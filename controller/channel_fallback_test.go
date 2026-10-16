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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func countReasons(reasons []string, want string) int {
	count := 0
	for _, reason := range reasons {
		if reason == want {
			count++
		}
	}
	return count
}
