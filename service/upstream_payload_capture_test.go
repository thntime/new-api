package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useUpstreamPayloadLogDB installs an isolated in-memory SQLite log database for
// the upstream payload tests and restores the previous handles afterwards.
func useUpstreamPayloadLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.UpstreamPayload{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestSaveUpstreamPayloadOverwritesSameRequestId(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	createdAt := time.Now().Unix()
	require.NoError(t, model.SaveUpstreamPayload(&model.UpstreamPayload{
		RequestId: "req-overwrite", CreatedAt: createdAt, RequestBody: "first",
	}))
	require.NoError(t, model.SaveUpstreamPayload(&model.UpstreamPayload{
		RequestId: "req-overwrite", CreatedAt: createdAt + 1, RequestBody: "second", ResponseBody: "resp",
	}))

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.UpstreamPayload{}).
		Where("request_id = ?", "req-overwrite").Count(&count).Error)
	assert.EqualValues(t, 1, count)

	stored, err := model.GetUpstreamPayloadByRequestId("req-overwrite")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.LongText("second"), stored.RequestBody)
	assert.Equal(t, model.LongText("resp"), stored.ResponseBody)
}

func TestGetUpstreamPayloadByRequestIdMissingReturnsNil(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	stored, err := model.GetUpstreamPayloadByRequestId("missing")
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func TestDeleteOldUpstreamPayloadBatchOnlyRemovesExpired(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	now := time.Now().Unix()
	require.NoError(t, model.LOG_DB.Create(&model.UpstreamPayload{RequestId: "old", CreatedAt: now - 3600}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.UpstreamPayload{RequestId: "fresh", CreatedAt: now}).Error)

	remaining, err := model.CountOldUpstreamPayload(context.Background(), now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, remaining)

	deleted, err := model.DeleteOldUpstreamPayloadBatch(context.Background(), now, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	fresh, err := model.GetUpstreamPayloadByRequestId("fresh")
	require.NoError(t, err)
	assert.NotNil(t, fresh)
}

func TestLogSettingRegistersCaptureUpstreamPayloadDefaultOff(t *testing.T) {
	exported := config.GlobalConfig.ExportAllConfigs()
	value, ok := exported["log_setting.capture_upstream_payload"]
	require.True(t, ok, "the option key must be registered so it round-trips through /api/option/")
	assert.Equal(t, "false", value)
}

func wrapAndDrain(t *testing.T, wrapped io.ReadCloser) string {
	t.Helper()
	read, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	require.NoError(t, wrapped.Close())
	return string(read)
}

func TestRequestWrapperPassesBytesThroughUnchanged(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	body := strings.Repeat("abcdefghij", 5000) // 50 KiB, read in many chunks
	capture := &UpstreamPayloadCapture{requestId: "req-passthrough", ctx: context.Background()}
	wrapped := capture.WrapRequestBody(io.NopCloser(strings.NewReader(body)))
	got := wrapAndDrain(t, wrapped)
	assert.Equal(t, body, got)

	capture.Finish()
	stored, err := model.GetUpstreamPayloadByRequestId("req-passthrough")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.LongText(body), stored.RequestBody)
	assert.False(t, stored.RequestTruncated)
	var count int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestResponseWrapperTruncatesAtLimitButStillPassesThrough(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	body := strings.Repeat("x", upstreamPayloadMaxResponseBytes+4096)
	capture := &UpstreamPayloadCapture{requestId: "req-truncated", ctx: context.Background()}
	response := strings.NewReader(body)
	wrapped := capture.WrapResponseBody(io.NopCloser(response))
	got := wrapAndDrain(t, wrapped)
	assert.Equal(t, len(body), len(got), "downstream still receives every byte")

	stored, err := model.GetUpstreamPayloadByRequestId("req-truncated")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Len(t, string(stored.ResponseBody), upstreamPayloadMaxResponseBytes)
	assert.True(t, stored.ResponseTruncated)
}

func TestNewCaptureDisabledReturnsNil(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	logSetting := operation_setting.GetLogSetting()
	previous := *logSetting
	t.Cleanup(func() { *logSetting = previous })
	logSetting.CaptureUpstreamPayload = false

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{RequestId: "req-off"}
	assert.Nil(t, NewUpstreamPayloadCapture(c, info))

	logSetting.CaptureUpstreamPayload = true
	assert.NotNil(t, NewUpstreamPayloadCapture(c, info))
	assert.Nil(t, NewUpstreamPayloadCapture(c, &relaycommon.RelayInfo{}), "no request_id means no correlation key")

	var count int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.Zero(t, count, "a disabled switch must write no rows")
}

func TestRequestWrapperCloseDoesNotPersistCapture(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	capture := &UpstreamPayloadCapture{requestId: "req-close-only", ctx: context.Background()}
	wrapped := capture.WrapRequestBody(io.NopCloser(strings.NewReader("hello")))

	require.NoError(t, wrapped.Close())

	var count int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.Zero(t, count, "the request body must be finished by the caller after the upstream call")
}

func TestRequestWrapperTruncatesAtLimitButStillPassesThrough(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	body := strings.Repeat("y", upstreamPayloadMaxRequestBytes+4096)
	capture := &UpstreamPayloadCapture{requestId: "req-request-truncated", ctx: context.Background()}
	wrapped := capture.WrapRequestBody(io.NopCloser(strings.NewReader(body)))
	got := wrapAndDrain(t, wrapped)
	assert.Equal(t, len(body), len(got), "downstream still receives every byte")

	capture.Finish()
	stored, err := model.GetUpstreamPayloadByRequestId("req-request-truncated")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Len(t, string(stored.RequestBody), upstreamPayloadMaxRequestBytes)
	assert.True(t, stored.RequestTruncated)
}

func TestCaptureAppendAndFinishConcurrentlyIsRaceFree(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	capture := &UpstreamPayloadCapture{requestId: "req-race-append", ctx: context.Background()}
	const chunks = 2000
	written := strings.Repeat("captured-chunk-", chunks)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range chunks {
			capture.appendRequest([]byte("captured-chunk-"))
		}
	}()
	go func() {
		defer wg.Done()
		capture.Finish()
	}()
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "Finish must write exactly one row even when it races the appender")

	stored, err := model.GetUpstreamPayloadByRequestId("req-race-append")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.LessOrEqual(t, len(stored.RequestBody), upstreamPayloadMaxRequestBytes)
	assert.True(t, strings.HasPrefix(written, string(stored.RequestBody)),
		"the persisted snapshot must be a prefix of the bytes appended so far")
}

func TestResponseWrapperConcurrentCloseIsRaceFree(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	capture := &UpstreamPayloadCapture{requestId: "req-race-close", ctx: context.Background()}
	wrapped := capture.WrapResponseBody(io.NopCloser(strings.NewReader("payload")))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = wrapped.Close()
		}()
	}
	wg.Wait()

	stored, err := model.GetUpstreamPayloadByRequestId("req-race-close")
	require.NoError(t, err)
	require.NotNil(t, stored)
}

func TestRunLogCleanupTaskDeletesExpiredUpstreamPayloads(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UpstreamPayload{RequestId: "old", CreatedAt: now - 3600}).Error)
	require.NoError(t, db.Create(&model.UpstreamPayload{RequestId: "fresh", CreatedAt: now + 3600}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: now - 3600, RequestId: "log-old"}).Error)

	task, err := model.CreateSystemTask(
		model.SystemTaskTypeLogCleanup,
		LogCleanupPayload{TargetTimestamp: now, BatchSize: 100},
		LogCleanupState{},
	)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogCleanup, "runner-upstream-payload", now+60)
	require.NoError(t, err)
	require.True(t, ok)

	runLogCleanupTask(context.Background(), claimed, "runner-upstream-payload")

	var expired int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Where("request_id = ?", "old").Count(&expired).Error)
	assert.Zero(t, expired)
	fresh, err := model.GetUpstreamPayloadByRequestId("fresh")
	require.NoError(t, err)
	assert.NotNil(t, fresh, "records after the cutoff must survive")

	var expiredLogs int64
	require.NoError(t, db.Model(&model.Log{}).Where("request_id = ?", "log-old").Count(&expiredLogs).Error)
	assert.Zero(t, expiredLogs, "the task must actually delete expired log rows")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
}

func TestRunLogCleanupTaskSurvivesUpstreamPayloadCleanupFailure(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	// Capturing payloads is an add-on: dropping its table forces every payload
	// count/delete to fail, exactly like a ClickHouse mutation timeout or a
	// rolling upgrade where the log DB has not created the table yet.
	require.NoError(t, db.Migrator().DropTable(&model.UpstreamPayload{}))

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: now - 3600, RequestId: "log-old"}).Error)

	task, err := model.CreateSystemTask(
		model.SystemTaskTypeLogCleanup,
		LogCleanupPayload{TargetTimestamp: now, BatchSize: 100},
		LogCleanupState{},
	)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogCleanup, "runner-payload-failure", now+60)
	require.NoError(t, err)
	require.True(t, ok)

	runLogCleanupTask(context.Background(), claimed, "runner-payload-failure")

	var expiredLogs int64
	require.NoError(t, db.Model(&model.Log{}).Where("request_id = ?", "log-old").Count(&expiredLogs).Error)
	assert.Zero(t, expiredLogs, "log retention must still run when payload cleanup fails")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status,
		"a payload cleanup failure must not fail the log cleanup task")
}

func TestCaptureWritesExactlyOnceAfterReadThenClose(t *testing.T) {
	db := useUpstreamPayloadLogDB(t)
	capture := &UpstreamPayloadCapture{requestId: "req-once", ctx: context.Background()}
	wrapped := capture.WrapResponseBody(io.NopCloser(strings.NewReader("payload")))
	_, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	require.NoError(t, wrapped.Close()) // Close after EOF must not write a second row

	var count int64
	require.NoError(t, db.Model(&model.UpstreamPayload{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	stored, err := model.GetUpstreamPayloadByRequestId("req-once")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.LongText("payload"), stored.ResponseBody)
}

func TestCaptureFailureKeepsRequestOnlyRecord(t *testing.T) {
	useUpstreamPayloadLogDB(t)
	capture := &UpstreamPayloadCapture{requestId: "req-failed", ctx: context.Background()}
	request := capture.WrapRequestBody(io.NopCloser(strings.NewReader("request-only")))
	_, err := io.ReadAll(request)
	require.NoError(t, err)
	require.NoError(t, request.Close())
	// doRequest calls Finish() on a Do error / nil response: no response body yet.
	capture.Finish()

	stored, err := model.GetUpstreamPayloadByRequestId("req-failed")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.LongText("request-only"), stored.RequestBody)
	assert.Equal(t, model.LongText(""), stored.ResponseBody)
	assert.False(t, stored.ResponseTruncated)
}
