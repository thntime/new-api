package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupFallbackStageTest installs an isolated in-memory channel cache and a
// fresh gin context, restoring every global it touches on cleanup.
func setupFallbackStageTest(t *testing.T) (*gorm.DB, *gin.Context) {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		if originalMemoryCacheEnabled && originalDB != nil && originalDB.Migrator().HasTable(&model.Channel{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return db, c
}

func createFallbackStageChannel(t *testing.T, db *gorm.DB, id int, group, modelName string, priority int64) {
	t.Helper()
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

// markFallbackChannels marks exactly the given channels as fallback channels and
// refreshes the memory channel cache; nil or an empty slice marks none. The full
// ChannelInfo is loaded before the marker is written so other channel_info keys
// survive.
func markFallbackChannels(t *testing.T, ids []int) {
	t.Helper()
	var channels []model.Channel
	require.NoError(t, model.DB.Find(&channels).Error)
	for i := range channels {
		marked := slices.Contains(ids, channels[i].Id)
		if channels[i].ChannelInfo.IsFallback == marked {
			continue
		}
		channels[i].ChannelInfo.IsFallback = marked
		require.NoError(t, channels[i].SaveChannelInfo())
	}
	model.InitChannelCache()
}

func fallbackPolicyReasons(events []service.PolicyEvent) []string {
	reasons := make([]string, 0, len(events))
	for _, event := range events {
		reasons = append(reasons, event.Decision.Reason)
	}
	return reasons
}

// TestRunFallbackStageWithoutPoolSkipsEverything covers the unchanged behaviour
// when no fallback pool is configured: no attempt, no event, no stage entry.
func TestRunFallbackStageWithoutPoolSkipsEverything(t *testing.T) {
	_, c := setupFallbackStageTest(t)
	markFallbackChannels(t, nil)

	calls := 0
	result := runFallbackStage(c, "default", "fallback-empty-pool-model", func(channel *model.Channel) (*types.NewAPIError, bool) {
		calls++
		return nil, false
	})

	require.Nil(t, result)
	assert.Zero(t, calls, "an unconfigured pool must not attempt any channel")
	assert.Empty(t, service.RequestPolicy(c).Events(), "an unconfigured pool must not record events")
	assert.False(t, service.FallbackStageActive(c), "an unconfigured pool must not enter the fallback stage")
}

// TestRunFallbackStageRecordsExhaustedWhenNoEligibleLayer covers a configured
// pool with no eligible layer for this request: no attempt and one
// fallback_exhausted event.
func TestRunFallbackStageRecordsExhaustedWhenNoEligibleLayer(t *testing.T) {
	db, c := setupFallbackStageTest(t)
	createFallbackStageChannel(t, db, 55, "default", "fallback-other-model", 5)
	markFallbackChannels(t, []int{55})
	model.InitChannelCache()

	calls := 0
	result := runFallbackStage(c, "default", "fallback-missing-model", func(channel *model.Channel) (*types.NewAPIError, bool) {
		calls++
		return nil, false
	})

	require.Nil(t, result)
	assert.Zero(t, calls, "an ineligible pool must not attempt any channel")
	assert.Equal(t, []string{"fallback_exhausted"}, fallbackPolicyReasons(service.RequestPolicy(c).Events()))
}

// TestRunFallbackStageTriesLayersInPriorityOrder covers two failing fallback
// layers: they are attempted highest priority first and the last error is
// returned.
func TestRunFallbackStageTriesLayersInPriorityOrder(t *testing.T) {
	db, c := setupFallbackStageTest(t)
	const modelName = "fallback-order-model"
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	markFallbackChannels(t, []int{66, 55})
	model.InitChannelCache()

	var tried []int
	upstreamErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	result := runFallbackStage(c, "default", modelName, func(channel *model.Channel) (*types.NewAPIError, bool) {
		tried = append(tried, channel.Id)
		return upstreamErr, false
	})

	require.NotNil(t, result)
	assert.Equal(t, []int{66, 55}, tried, "fallback layers must be attempted in descending priority")
	reasons := fallbackPolicyReasons(service.RequestPolicy(c).Events())
	assert.Equal(t, 2, countReasons(reasons, "fallback_retry"))
	assert.NotContains(t, reasons, "fallback_exhausted")
}

// TestRunFallbackStageStopsAfterFirstSuccess covers a first-layer success: the
// stage returns nil and the remaining layer is never attempted.
func TestRunFallbackStageStopsAfterFirstSuccess(t *testing.T) {
	db, c := setupFallbackStageTest(t)
	const modelName = "fallback-success-model"
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	markFallbackChannels(t, []int{66, 55})
	model.InitChannelCache()

	var tried []int
	result := runFallbackStage(c, "default", modelName, func(channel *model.Channel) (*types.NewAPIError, bool) {
		tried = append(tried, channel.Id)
		return nil, false
	})

	require.Nil(t, result)
	assert.Equal(t, []int{66}, tried, "a successful layer must stop the fallback stage")
	assert.NotContains(t, fallbackPolicyReasons(service.RequestPolicy(c).Events()), "fallback_retry")
}

// TestRunFallbackStageStopsOnAbortWithoutRetryEvent covers a layer that reports
// a hard stop (billing/request-body failure): the stage returns that error
// immediately, records no fallback_retry, and never attempts the next layer.
func TestRunFallbackStageStopsOnAbortWithoutRetryEvent(t *testing.T) {
	db, c := setupFallbackStageTest(t)
	const modelName = "fallback-abort-model"
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	markFallbackChannels(t, []int{66, 55})
	model.InitChannelCache()

	calls := 0
	abortErr := types.NewOpenAIError(errors.New("body read failed"), types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest)
	result := runFallbackStage(c, "default", modelName, func(channel *model.Channel) (*types.NewAPIError, bool) {
		calls++
		return abortErr, true
	})

	require.NotNil(t, result)
	assert.Same(t, abortErr, result, "a stop result must be returned as-is")
	assert.Equal(t, 1, calls, "a stop result must not attempt further layers")
	reasons := fallbackPolicyReasons(service.RequestPolicy(c).Events())
	assert.NotContains(t, reasons, "fallback_retry")
	assert.NotContains(t, reasons, "fallback_exhausted")
}

// TestTaskFallbackFailureReportsLastAttemptError pins the task submission
// fallback wiring: when every fallback layer fails, the reported error is the
// last fallback attempt (agreeing with relayInfo.LastError and the HTTP path),
// not the normal-stage error.
func TestTaskFallbackFailureReportsLastAttemptError(t *testing.T) {
	db, c := setupFallbackStageTest(t)
	const modelName = "task-fallback-wiring-model"
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	markFallbackChannels(t, []int{66, 55})
	model.InitChannelCache()

	c.Set("channel_id", 999)
	c.Set("channel_type", constant.ChannelTypeOpenAI)
	c.Set("channel_name", "normal-channel")

	relayInfo := &relaycommon.RelayInfo{
		TokenGroup:      "default",
		OriginModelName: modelName,
		UsingGroup:      "default",
		UserGroup:       "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	// The submit failure carries the selected channel id, so the reported error
	// identifies which attempt produced it. LocalError keeps processChannelError
	// out of the test; it runs after the taskErr assignment under test.
	submit := func(c *gin.Context, info *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *taskdto.TaskError) {
		code := fmt.Sprintf("channel-%d-failed", common.GetContextKeyInt(c, constant.ContextKeyChannelId))
		return nil, &taskdto.TaskError{Code: code, Message: code, StatusCode: http.StatusInternalServerError, LocalError: true}
	}

	outcome, taskErr := executeTaskSubmissionWith(c, relayInfo, submit)

	require.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "channel-55-failed", taskErr.Code, "the reported error must be the last fallback attempt")
	require.NotNil(t, relayInfo.LastError)
	assert.Equal(t, "channel-55-failed", relayInfo.LastError.Error(), "relayInfo.LastError must agree with the reported error")
}

// TestFallbackSequenceMatchesSpecExample drives one fully failing request
// through the real relay loop and asserts the spec's example attempt order:
// 50 -> 40 -> 30 -> 20 -> 10 -> 50 (descent wrap-around) -> 66 -> 55 (fallback
// pool, its own priorities). The order is only reachable when the descent anchor
// is established by the production selection path, so the request is primed
// exactly like the distributor does.
func TestFallbackSequenceMatchesSpecExample(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-sequence-model"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	db, c := setupFallbackStageTest(t)
	common.RetryTimes = 5

	// A free model keeps billing preparation from reserving quota, and disabling
	// the log paths keeps the fixture to channels and abilities only.
	originalRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(fmt.Sprintf(`{%q:0}`, modelName)))
	originalLogConsume, originalErrorLog, originalCountToken := common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken
	common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken = false, false, false
	originalFreeModelPreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios))
		common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken = originalLogConsume, originalErrorLog, originalCountToken
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = originalFreeModelPreConsume
	})

	// descent is resolved into the request only through a matching affinity rule,
	// so the global mode needs an inheriting rule and a key source value.
	affinity := operation_setting.GetChannelAffinitySetting()
	originalAffinity := *affinity
	t.Cleanup(func() { *affinity = originalAffinity })
	affinity.Enabled = true
	affinity.SessionMode = "descent"
	affinity.Rules = []operation_setting.ChannelAffinityRule{{
		Name:        "session",
		ModelRegex:  []string{".*"},
		KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
		SessionMode: "inherit",
	}}
	// One channel per normal priority layer, so each layer selects deterministically.
	for _, priority := range []int64{50, 40, 30, 20, 10} {
		createFallbackStageChannel(t, db, int(priority), "default", modelName, priority)
	}
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	markFallbackChannels(t, []int{66, 55})
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)
	model.InitChannelCache()

	body := `{"model":"` + modelName + `","messages":[{"role":"user","content":"hi"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Session", t.Name())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)

	// Production selection: this remembers the descent anchor (priority 50) before
	// the relay loop starts, which is what makes the wrap-around 10 -> 50 happen.
	selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
		Ctx: c, ModelName: modelName, TokenGroup: "default", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	})
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	require.Equal(t, 50, selected.Id)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, []string{"50", "40", "30", "20", "10", "50", "66", "55"}, c.GetStringSlice("use_channel"))
}

// prepareFreeModelRelay makes modelName free and silences the log paths so the
// relay loop can run against an in-memory channel cache, restoring every global
// on cleanup.
func prepareFreeModelRelay(t *testing.T, modelName string) {
	t.Helper()
	originalRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(fmt.Sprintf(`{%q:0}`, modelName)))
	originalLogConsume, originalErrorLog, originalCountToken := common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken
	common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken = false, false, false
	originalFreeModelPreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios))
		common.LogConsumeEnabled, constant.ErrorLogEnabled, constant.CountToken = originalLogConsume, originalErrorLog, originalCountToken
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = originalFreeModelPreConsume
	})
}

// prepareFallbackRelayRequest primes c exactly like the distributor does for the
// shared fallback relay tests.
func prepareFallbackRelayRequest(c *gin.Context, modelName string) {
	body := `{"model":"` + modelName + `","messages":[{"role":"user","content":"hi"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)
}

// A successful normal-layer attempt must never reach the fallback pool: the
// pool channel is not attempted and no fallback decision event is recorded.
func TestNormalSuccessNeverTouchesFallback(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-normal-success-model"

	successBody := `{"id":"chatcmpl-1","object":"chat.completion","model":"` + modelName + `","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(successBody))
	}))
	t.Cleanup(upstream.Close)

	db, c := setupFallbackStageTest(t)
	common.RetryTimes = 5
	prepareFreeModelRelay(t, modelName)

	createFallbackStageChannel(t, db, 50, "default", modelName, 50)
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	markFallbackChannels(t, []int{66})
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)
	model.InitChannelCache()
	prepareFallbackRelayRequest(c, modelName)

	selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
		Ctx: c, ModelName: modelName, TokenGroup: "default", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	})
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	require.Equal(t, 50, selected.Id, "a normal channel must win over the pool")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, []string{"50"}, c.GetStringSlice("use_channel"), "a successful normal attempt must not touch the pool")
	assert.False(t, service.FallbackStageActive(c))
	reasons := fallbackPolicyReasons(service.RequestPolicy(c).Events())
	assert.NotContains(t, reasons, "fallback_retry")
	assert.NotContains(t, reasons, "fallback_exhausted")
}

// A channel pin means "this channel only": after the pinned attempt fails the
// request must stop without entering the fallback pool (design D8).
func TestPinnedRequestDoesNotEnterFallback(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-pinned-model"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	db, c := setupFallbackStageTest(t)
	common.RetryTimes = 5
	prepareFreeModelRelay(t, modelName)

	createFallbackStageChannel(t, db, 50, "default", modelName, 50)
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	markFallbackChannels(t, []int{66})
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)
	model.InitChannelCache()
	prepareFallbackRelayRequest(c, modelName)

	service.GetChannelConstraints(c).AddPin(taskdto.ChannelPin{
		ChannelId: 50, Source: taskdto.PinSourceToken, Rank: taskdto.PinRankToken, RetryMode: taskdto.PinRetrySingleAttempt,
	})

	selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
		Ctx: c, ModelName: modelName, TokenGroup: "default", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	})
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	require.Equal(t, 50, selected.Id)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, []string{"50"}, c.GetStringSlice("use_channel"), "a pinned request must not try the fallback pool")
	assert.False(t, service.FallbackStageActive(c))
	reasons := fallbackPolicyReasons(service.RequestPolicy(c).Events())
	assert.NotContains(t, reasons, "fallback_retry")
	assert.NotContains(t, reasons, "fallback_exhausted")
}

// A fallback pool holding a single channel is still tried once the normal
// attempts stop; the pool is not treated as empty.
func TestSingleFallbackChannelIsTried(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-single-channel-model"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	db, c := setupFallbackStageTest(t)
	common.RetryTimes = 0
	prepareFreeModelRelay(t, modelName)

	createFallbackStageChannel(t, db, 50, "default", modelName, 50)
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	markFallbackChannels(t, []int{66})
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)
	model.InitChannelCache()
	prepareFallbackRelayRequest(c, modelName)

	selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
		Ctx: c, ModelName: modelName, TokenGroup: "default", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	})
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	require.Equal(t, 50, selected.Id)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, []string{"50", "66"}, c.GetStringSlice("use_channel"), "a single-channel pool must still be tried")
}

// Through the real request path a configured-but-ineligible pool must record
// exactly one fallback_exhausted event: the distributor aborts on the 503 and
// the deferred termination recorder must not append a second stop event.
func TestDistributorEmptyNormalPoolRecordsSingleFallbackExhausted(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-distributor-exhausted-model"
	db, c := setupFallbackStageTest(t)
	// The pool member serves another model, so the pool cannot serve this request.
	createFallbackStageChannel(t, db, 55, "default", "fallback-distributor-other-model", 5)
	markFallbackChannels(t, []int{55})
	model.InitChannelCache()
	prepareFallbackRelayRequest(c, modelName)

	gin.SetMode(gin.TestMode)
	var captured *gin.Context
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		captured = ctx
		common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
		ctx.Next()
	})
	engine.POST("/v1/chat/completions", middleware.Distribute(), func(ctx *gin.Context) {
		Relay(ctx, types.RelayFormatOpenAI)
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, c.Request)

	require.NotNil(t, captured)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.True(t, captured.IsAborted(), "the request must be aborted before the relay handler")

	events := service.RequestPolicy(captured).Events()
	require.Len(t, events, 1, "the request path must record exactly one decision event")
	assert.Equal(t, "fallback_exhausted", events[0].Decision.Reason)
	assert.NotContains(t, fallbackPolicyReasons(events), "fallback_retry")
}

// TestAutoGroupsRunFallbackOnceAfterAllGroups pins the auto-group fallback
// timing: once every auto group has run its normal priority layers and failed,
// the pool is entered exactly once, filtered by the group current when the
// request reached the fallback stage (design D1/D6). Two groups run with
// cross-group retry and the pool lives in the second group, so a pool entered
// after the first group would be filtered out and a pool that runs early would
// break the normal segment's order.
func TestAutoGroupsRunFallbackOnceAfterAllGroups(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "auto-groups-fallback-model"
	const (
		groupFirst  = "fb-g1"
		groupSecond = "fb-g2"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	db, _ := setupFallbackStageTest(t)
	common.RetryTimes = 1
	prepareFreeModelRelay(t, modelName)

	// Auto group membership needs the group to be user-usable and priced.
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(fmt.Sprintf(`[%q,%q]`, groupFirst, groupSecond)))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(fmt.Sprintf(`{"default":"Default",%q:"A",%q:"B"}`, groupFirst, groupSecond)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(`{"default":1,%q:1,%q:1}`, groupFirst, groupSecond)))

	// One channel per priority layer: group 1 has layers 50/40, group 2 has
	// layers 30/20 and hosts both pool layers 6/5. The pool's own priorities are
	// below the layers the retry budget reaches, so a pool member never shows up
	// in the normal segment whether or not the pool is configured.
	createFallbackStageChannel(t, db, 2001, groupFirst, modelName, 50)
	createFallbackStageChannel(t, db, 2002, groupFirst, modelName, 40)
	createFallbackStageChannel(t, db, 3001, groupSecond, modelName, 30)
	createFallbackStageChannel(t, db, 3002, groupSecond, modelName, 20)
	createFallbackStageChannel(t, db, 66, groupSecond, modelName, 6)
	createFallbackStageChannel(t, db, 55, groupSecond, modelName, 5)
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)

	// Affinity rules left by other tests must not steer the auto selection.
	affinity := operation_setting.GetChannelAffinitySetting()
	originalAffinity := *affinity
	t.Cleanup(func() { *affinity = originalAffinity })
	affinity.Enabled = false

	run := func(pool []int) []string {
		markFallbackChannels(t, pool)

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := `{"model":"` + modelName + `","messages":[{"role":"user","content":"hi"}]}`
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyTokenId, 1)

		// Production selection, exactly like the distributor: it makes the first
		// attempt, primes the auto-group state and gives the relay loop a
		// ChannelMeta so the remaining attempts select for real.
		selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
			Ctx: c, ModelName: modelName, TokenGroup: "auto", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
		})
		require.Nil(t, selectErr)
		require.NotNil(t, selected)
		require.Equal(t, 2001, selected.Id, "the first attempt must be group 1's top layer")
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

		Relay(c, types.RelayFormatOpenAI)
		return c.GetStringSlice("use_channel")
	}

	withPool := run([]int{66, 55})
	assert.Equal(t, []string{"2001", "2002", "3001", "3002", "66", "55"}, withPool)

	// ① each fallback priority layer is attempted exactly once; ② no fallback
	// channel runs before the last normal group channel.
	count66, count55 := 0, 0
	lastNormal, firstFallback := -1, len(withPool)
	for i, id := range withPool {
		if id == "66" || id == "55" {
			if id == "66" {
				count66++
			} else {
				count55++
			}
			if i < firstFallback {
				firstFallback = i
			}
			continue
		}
		lastNormal = i
	}
	assert.Equal(t, 1, count66, "pool layer 6 must be attempted exactly once")
	assert.Equal(t, 1, count55, "pool layer 5 must be attempted exactly once")
	require.NotEqual(t, -1, lastNormal, "the normal segment must run")
	require.NotEqual(t, len(withPool), firstFallback, "the pool must be attempted")
	assert.Greater(t, firstFallback, lastNormal, "the pool must run only after every normal group channel")

	// ③ disabling the pool leaves the normal segment bit-for-bit identical,
	// proving a single group's budget running out does not advance the pool nor
	// reorder the normal segment.
	withoutPool := run(nil)
	narrowed := make([]string, 0, len(withPool))
	for _, id := range withPool {
		if id != "66" && id != "55" {
			narrowed = append(narrowed, id)
		}
	}
	assert.Equal(t, narrowed, withoutPool, "the normal segment must not depend on the pool")
}

// Design D3: with the global fallback switch off the whole request path must be
// identical to a deployment where no channel carries the marker: same attempt
// order, same policy events, no fallback stage and never any fallback_* event.
// The pool-active run proves the fixture can tell the two apart.
func TestFallbackSwitchOffMatchesUnmarkedPool(t *testing.T) {
	require.NoError(t, i18n.Init())

	const modelName = "fallback-switch-off-model"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	db, _ := setupFallbackStageTest(t)
	common.RetryTimes = 5
	prepareFreeModelRelay(t, modelName)

	// descent is resolved into the request only through a matching affinity rule.
	affinity := operation_setting.GetChannelAffinitySetting()
	originalAffinity := *affinity
	t.Cleanup(func() { *affinity = originalAffinity })
	affinity.Enabled = true
	affinity.SessionMode = "descent"
	affinity.Rules = []operation_setting.ChannelAffinityRule{{
		Name:        "session",
		ModelRegex:  []string{".*"},
		KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
		SessionMode: "inherit",
	}}

	for _, priority := range []int64{50, 40, 30, 20, 10} {
		createFallbackStageChannel(t, db, int(priority), "default", modelName, priority)
	}
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
	require.NoError(t, db.Model(&model.Channel{}).Where("1 = 1").Update("base_url", upstream.URL).Error)

	run := func(marked []int, fallbackEnabled bool, session string) ([]string, []string, bool) {
		affinity.FallbackEnabled = fallbackEnabled
		markFallbackChannels(t, marked)

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := `{"model":"` + modelName + `","messages":[{"role":"user","content":"hi"}]}`
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("X-Session", session)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserId, 1)
		common.SetContextKey(c, constant.ContextKeyTokenId, 1)

		selected, _, selectErr := service.SelectChannelForRequest(c, modelName, &service.RetryParam{
			Ctx: c, ModelName: modelName, TokenGroup: "default", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
		})
		require.Nil(t, selectErr)
		require.NotNil(t, selected)
		require.Equal(t, 50, selected.Id, "the first attempt must be the top normal layer")
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, selected, modelName))

		Relay(c, types.RelayFormatOpenAI)
		return c.GetStringSlice("use_channel"), fallbackPolicyReasons(service.RequestPolicy(c).Events()), service.FallbackStageActive(c)
	}

	poolOn, poolOnEvents, poolOnStage := run([]int{66, 55}, true, t.Name()+"-pool")
	markedOff, markedOffEvents, markedOffStage := run([]int{66, 55}, false, t.Name()+"-off")
	unmarkedOn, unmarkedOnEvents, unmarkedOnStage := run(nil, true, t.Name()+"-unmarked")

	// Guard against a vacuous equality: with the switch on the fixture must enter
	// the pool and differ from the unmarked run.
	assert.True(t, poolOnStage, "the pool-active run must enter the fallback stage")
	assert.Contains(t, poolOnEvents, "fallback_retry", "the pool-active run must record fallback attempts")
	assert.NotEqual(t, unmarkedOn, poolOn, "the fixture must be able to enter the pool")

	assert.Equal(t, unmarkedOn, markedOff, "switch off must not change the attempt order")
	assert.Equal(t, unmarkedOnEvents, markedOffEvents, "switch off must not change the decision events")
	assert.Equal(t, unmarkedOnStage, markedOffStage, "switch off must not change the stage state")
	assert.False(t, markedOffStage, "switch off must never enter the fallback stage")
	assert.NotContains(t, markedOffEvents, "fallback_retry")
	assert.NotContains(t, markedOffEvents, "fallback_exhausted")
}

func countReasons(reasons []string, want string) int {
	count := 0
	for _, reason := range reasons {
		if reason == want {
			count++
		}
	}
	return count
}
