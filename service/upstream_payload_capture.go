package service

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	upstreamPayloadMaxRequestBytes  = 256 << 10 // 256 KiB
	upstreamPayloadMaxResponseBytes = 1 << 20   // 1 MiB
)

// UpstreamPayloadCapture observes the bytes the gateway relays upstream without
// changing them, and writes one row at the end of the request.
type UpstreamPayloadCapture struct {
	ctx               context.Context
	requestId         string
	mu                sync.Mutex
	requestBody       bytes.Buffer
	responseBody      bytes.Buffer
	requestTruncated  bool
	responseTruncated bool
	once              sync.Once
}

// NewUpstreamPayloadCapture returns nil when capture is off or the request has no
// correlation key, so callers take the original path with no extra allocation.
func NewUpstreamPayloadCapture(c *gin.Context, info *relaycommon.RelayInfo) *UpstreamPayloadCapture {
	if !operation_setting.GetLogSetting().CaptureUpstreamPayload {
		return nil
	}
	if info == nil || info.RequestId == "" {
		return nil
	}
	capture := &UpstreamPayloadCapture{requestId: info.RequestId, ctx: context.Background()}
	if c != nil && c.Request != nil {
		capture.ctx = c.Request.Context()
	}
	return capture
}

// WrapRequestBody wraps the outgoing request body; the caller must call Finish once
// the upstream call is done, because the request body may not be read to EOF.
func (c *UpstreamPayloadCapture) WrapRequestBody(rc io.ReadCloser) io.ReadCloser {
	return &captureReadCloser{rc: rc, sink: c.appendRequest}
}

// WrapResponseBody wraps the upstream response body and finishes the capture when
// the body is read to EOF or closed.
func (c *UpstreamPayloadCapture) WrapResponseBody(rc io.ReadCloser) io.ReadCloser {
	return &captureReadCloser{rc: rc, sink: c.appendResponse, onDone: c.Finish}
}

// The request body is written by the transport goroutine while Finish runs on the
// response goroutine, so both buffer updates and the Finish snapshot take mu.
func (c *UpstreamPayloadCapture) appendRequest(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.requestTruncated {
		return
	}
	n := min(len(p), upstreamPayloadMaxRequestBytes-c.requestBody.Len())
	if n > 0 {
		c.requestBody.Write(p[:n])
	}
	if n < len(p) {
		c.requestTruncated = true
	}
}

func (c *UpstreamPayloadCapture) appendResponse(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.responseTruncated {
		return
	}
	n := min(len(p), upstreamPayloadMaxResponseBytes-c.responseBody.Len())
	if n > 0 {
		c.responseBody.Write(p[:n])
	}
	if n < len(p) {
		c.responseTruncated = true
	}
}

// Finish persists the captured bodies at most once; a failed write is logged and
// never affects the relayed request. The snapshot is taken under mu and the write
// happens outside it, so a slow log database never blocks the transport goroutine.
func (c *UpstreamPayloadCapture) Finish() {
	c.once.Do(func() {
		c.mu.Lock()
		payload := &model.UpstreamPayload{
			RequestId:         c.requestId,
			CreatedAt:         time.Now().Unix(),
			RequestBody:       model.LongText(c.requestBody.String()),
			ResponseBody:      model.LongText(c.responseBody.String()),
			RequestTruncated:  c.requestTruncated,
			ResponseTruncated: c.responseTruncated,
		}
		c.mu.Unlock()

		if err := model.SaveUpstreamPayload(payload); err != nil {
			logger.LogError(c.ctx, common.LogText("failed to save upstream payload: %s", err.Error()))
		}
	})
}

var _ io.ReadCloser = (*captureReadCloser)(nil)

type captureReadCloser struct {
	rc       io.ReadCloser
	sink     func([]byte)
	onDone   func()
	doneOnce sync.Once
}

func (b *captureReadCloser) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.sink(p[:n])
	}
	if err == io.EOF {
		b.fire()
	}
	return n, err
}

func (b *captureReadCloser) Close() error {
	err := b.rc.Close()
	b.fire()
	return err
}

// fire runs onDone at most once. Close is reachable from both the caller and the
// http transport, hence sync.Once rather than a plain bool.
func (b *captureReadCloser) fire() {
	if b.onDone == nil {
		return
	}
	b.doneOnce.Do(b.onDone)
}
