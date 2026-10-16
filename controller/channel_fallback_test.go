package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	originalFallbackIDs := operation_setting.GetChannelAffinitySetting().FallbackChannelIDs

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
		operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = originalFallbackIDs
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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = nil

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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = []int{55}
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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = []int{66, 55}
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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = []int{66, 55}
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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = []int{66, 55}
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
	operation_setting.GetChannelAffinitySetting().FallbackChannelIDs = []int{66, 55}
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
	fallbackPool := []int{66, 55}
	affinity.FallbackChannelIDs = fallbackPool

	// One channel per normal priority layer, so each layer selects deterministically.
	for _, priority := range []int64{50, 40, 30, 20, 10} {
		createFallbackStageChannel(t, db, int(priority), "default", modelName, priority)
	}
	createFallbackStageChannel(t, db, 66, "default", modelName, 6)
	createFallbackStageChannel(t, db, 55, "default", modelName, 5)
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

func countReasons(reasons []string, want string) int {
	count := 0
	for _, reason := range reasons {
		if reason == want {
			count++
		}
	}
	return count
}
