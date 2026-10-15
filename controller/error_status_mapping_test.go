package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newErrorMappingContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	return c, recorder
}

func TestRenderErrorResponseOverride(t *testing.T) {
	override := &types.ErrorResponseOverride{
		StatusCode: http.StatusTooManyRequests,
		Headers:    map[string]string{"x-should-retry": "true", "Retry-After": "30"},
		Message:    "rate limit exceeded",
		Type:       "rate_limit_error",
		Code:       "rate_limit_exceeded",
	}

	t.Run("openai body and headers", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatOpenAI)

		require.True(t, written)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "true", recorder.Header().Get("x-should-retry"))
		require.Equal(t, "30", recorder.Header().Get("Retry-After"))
		require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

		var body struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, "rate limit exceeded", body.Error.Message)
		require.Equal(t, "rate_limit_error", body.Error.Type)
		require.Equal(t, "rate_limit_exceeded", body.Error.Code)
	})

	t.Run("claude body without request id suffix", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatClaude)

		require.True(t, written)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)

		var body struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, "error", body.Type)
		require.Equal(t, "rate_limit_error", body.Error.Type)
		require.Equal(t, "rate limit exceeded", body.Error.Message)
	})

	t.Run("overrides a pre-set event-stream content type", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Transfer-Encoding", "chunked")

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatOpenAI)

		require.True(t, written)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
		require.Equal(t, "true", recorder.Header().Get("x-should-retry"))
		require.Empty(t, recorder.Header().Get("Transfer-Encoding"))
	})

	t.Run("nil override is not rendered", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{}, types.RelayFormatOpenAI)

		require.False(t, written)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("committed response is left untouched", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		c.String(http.StatusOK, "stream-started")

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatOpenAI)

		require.False(t, written)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Empty(t, recorder.Header().Get("x-should-retry"))
		require.Equal(t, "stream-started", recorder.Body.String())
	})
}

// Mirrors the preset rate limit rule (design D6), enabled for these tests.
const rateLimitStatusMappingRulesJSON = `[{"name":"Rate limit","enabled":true,"match":{"status_code":0,"message_regex":"exhausted|tpm/rpm|rps|rate limit"},"output":{"status_code":429,"message":"rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded","headers":[{"key":"x-should-retry","value":"true"},{"key":"Retry-After","value":"30"}]}}]`

// Tokens that make a downstream client give up instead of retrying.
var statusMappingForbiddenTokens = []string{
	"insufficient_quota",
	"usage_not_included",
	"billing_error",
	"gousagelimiterror",
	"freeusagelimiterror",
	"creditlimitexceeded",
	"insufficient balance",
	"quota exceeded",
	"budget exceeded",
	"usage limit",
	"limit exhausted",
	"package has expired",
	"plan does not yet include",
}

func enableRateLimitStatusMapping(t *testing.T) {
	t.Helper()
	previous := operation_setting.ErrorStatusMappingRules
	t.Cleanup(func() { operation_setting.ErrorStatusMappingRules = previous })
	rules, err := operation_setting.ParseErrorStatusMappingRules(rateLimitStatusMappingRulesJSON)
	require.NoError(t, err)
	operation_setting.ErrorStatusMappingRules = rules
}

// upstreamRelayError builds the NewAPIError the relay derives from an upstream
// response, using the parser production uses for that status: OpenaiHandler for
// a 200 body that carries an error object, RelayErrorHandler otherwise.
//
// channel_inference_test.go only covers VLLM status probing and assembles no
// relay channel/user/token, so a full Relay() HTTP round trip is not reachable
// from the existing fixtures; per the plan this degrades to the production
// parse -> service.ResetStatusCode -> renderErrorResponseOverride chain.
func upstreamRelayError(t *testing.T, c *gin.Context, statusCode int, body string) *types.NewAPIError {
	t.Helper()
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if statusCode == http.StatusOK {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
		_, newAPIError := openai.OpenaiHandler(c, info, resp)
		require.NotNil(t, newAPIError, "a 200 body carrying an error object must become a NewAPIError")
		return newAPIError
	}
	newAPIError := service.RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, newAPIError)
	return newAPIError
}

func TestRelayErrorStatusMapping(t *testing.T) {
	enableRateLimitStatusMapping(t)

	for _, tc := range []struct {
		name           string
		upstreamStatus int
		upstreamBody   string
	}{
		{
			name:           "upstream 200 with an OpenAI error body",
			upstreamStatus: http.StatusOK,
			upstreamBody:   `{"error":{"message":"inference exceeds tpm/rpm limit, usage limit reached","type":"rate_limit_error","code":"insufficient_quota"}}`,
		},
		{
			name:           "upstream 429 quota report",
			upstreamStatus: http.StatusTooManyRequests,
			upstreamBody:   `{"error":{"message":"usage limit exhausted and insufficient balance: rate limit applies","type":"insufficient_quota","code":"creditlimitexceeded"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := newErrorMappingContext(t)
			newAPIError := upstreamRelayError(t, c, tc.upstreamStatus, tc.upstreamBody)
			service.ResetStatusCode(newAPIError, "")

			require.True(t, renderErrorResponseOverride(c, newAPIError, types.RelayFormatOpenAI))

			assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
			assert.Equal(t, "true", recorder.Header().Get("x-should-retry"), "the client must be told to retry")
			assert.Equal(t, "30", recorder.Header().Get("Retry-After"))
			assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

			body := recorder.Body.String()
			for _, token := range statusMappingForbiddenTokens {
				assert.NotContains(t, body, token)
			}
			var parsed struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &parsed))
			assert.Equal(t, "rate limit exceeded", parsed.Error.Message)
			assert.Equal(t, "rate_limit_error", parsed.Error.Type)
			assert.Equal(t, "rate_limit_exceeded", parsed.Error.Code)
		})
	}

	t.Run("unmatched error report is not rewritten", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		newAPIError := upstreamRelayError(t, c, http.StatusTooManyRequests, `{"error":{"message":"model is temporarily unavailable","type":"server_error"}}`)
		service.ResetStatusCode(newAPIError, "")

		assert.Nil(t, newAPIError.ResponseOverride)
		assert.Equal(t, http.StatusTooManyRequests, newAPIError.StatusCode, "the original status code stays on the error object")
		assert.False(t, renderErrorResponseOverride(c, newAPIError, types.RelayFormatOpenAI))
		assert.Empty(t, recorder.Header().Get("x-should-retry"))
		assert.Empty(t, recorder.Body.String())
	})

	t.Run("successful upstream body stays a successful response", func(t *testing.T) {
		// Review Focus #1: a 200 body of normal content must never be rewritten.
		// Driven through the production handler (OpenaiHandler) instead of a
		// hand-rolled detector call, so a regression that turns normal content
		// into an error is caught here.
		c, recorder := newErrorMappingContext(t)
		successBody := `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(successBody)),
		}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
		usage, newAPIError := openai.OpenaiHandler(c, info, resp)

		assert.Nil(t, newAPIError, "a normal 200 body must not become a NewAPIError")
		assert.NotNil(t, usage, "the successful path returns usage")

		// The mapping rule is enabled, but a nil error cannot match it: the exit
		// chain must neither panic nor have an override to render.
		service.ResetStatusCode(nil, "")
		assert.False(t, renderErrorResponseOverride(c, newAPIError, types.RelayFormatOpenAI))

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Empty(t, recorder.Header().Get("x-should-retry"))
		assert.Empty(t, recorder.Header().Get("Retry-After"))
		assert.Equal(t, successBody, recorder.Body.String(), "the successful body must be forwarded verbatim")
		for _, token := range statusMappingForbiddenTokens {
			assert.NotContains(t, successBody, token)
		}
	})
}

func TestRelayErrorStatusMappingStream(t *testing.T) {
	enableRateLimitStatusMapping(t)

	t.Run("committed SSE response is left untouched", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		newAPIError := upstreamRelayError(t, c, http.StatusTooManyRequests, `{"error":{"message":"usage limit exhausted: rate limit applies","type":"insufficient_quota","code":"insufficient_quota"}}`)
		service.ResetStatusCode(newAPIError, "")
		require.NotNil(t, newAPIError.ResponseOverride, "the rule must have matched")

		helper.SetEventStreamHeaders(c)
		_, err := c.Writer.WriteString("data: {\"type\":\"response.created\"}\n\n")
		require.NoError(t, err)
		require.True(t, c.Writer.Written())

		assert.False(t, renderErrorResponseOverride(c, newAPIError, types.RelayFormatOpenAI))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Empty(t, recorder.Header().Get("x-should-retry"))
		assert.Empty(t, recorder.Header().Get("Retry-After"))
		assert.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
		assert.Equal(t, "data: {\"type\":\"response.created\"}\n\n", recorder.Body.String())
	})

	t.Run("stream failure before the first chunk follows the rules", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		newAPIError := upstreamRelayError(t, c, http.StatusTooManyRequests, `{"error":{"message":"tpm/rpm limit reached","type":"rate_limit_error"}}`)
		service.ResetStatusCode(newAPIError, "")

		helper.SetEventStreamHeaders(c)
		require.False(t, c.Writer.Written())

		assert.True(t, renderErrorResponseOverride(c, newAPIError, types.RelayFormatOpenAI))
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
		assert.Empty(t, recorder.Header().Get("Transfer-Encoding"))
		assert.Equal(t, "true", recorder.Header().Get("x-should-retry"))
		assert.Equal(t, "30", recorder.Header().Get("Retry-After"))
	})
}

func TestUpdateOptionRejectsInvalidErrorStatusMappingRules(t *testing.T) {
	db := modelManagementDB(t, "sqlite", "")

	const before = `[{"name":"before","enabled":false,"match":{"message_regex":"exhausted"},"output":{"status_code":429}}]`
	require.NoError(t, db.Create(&model.Option{Key: "ErrorStatusMappingRules", Value: before}).Error)
	common.OptionMap["ErrorStatusMappingRules"] = before

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	modelManagementRequest(t, UpdateOption, http.MethodPut, "/api/option/", OptionUpdateRequest{
		Key:   "ErrorStatusMappingRules",
		Value: `[{"name":"bad","enabled":true,"match":{"message_regex":"["},"output":{"status_code":429}}]`,
	}, &response)

	assert.False(t, response.Success, "invalid rules must be rejected")
	assert.NotEmpty(t, response.Message)
	assert.Equal(t, before, common.OptionMap["ErrorStatusMappingRules"], "a rejected save must not touch the option map")

	var saved model.Option
	require.NoError(t, db.Where(&model.Option{Key: "ErrorStatusMappingRules"}).First(&saved).Error)
	assert.Equal(t, before, saved.Value, "a rejected save must not touch the database")
}
