package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
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

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestAffinityAdjustedRetry(t *testing.T) {
	// 未命中亲和 → 不偏移
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	require.Equal(t, 1, AffinityAdjustedRetry(ctx, 1))
	require.Equal(t, 7, AffinityAdjustedRetry(ctx, 7))

	// 亲和命中 → retry>=1 时偏移 -1，retry=0 不偏移（0 为首轮快捷路径，不进入此分支）
	ctx2 := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r2"})
	MarkChannelAffinityUsed(ctx2, "default", 6)
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 1))
	require.Equal(t, 6, AffinityAdjustedRetry(ctx2, 7))
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 0))

	// nil ctx → 不偏移
	require.Equal(t, 1, AffinityAdjustedRetry(nil, 1))
}

func TestAffinityRetrySelectsTopPriorityChannel(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	// 构造渠道：priority 20（渠道 1）与 19（渠道 2），同一 model/group
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	// InitChannelCache 用 Ability 表的 group 集合初始化候选池的二级 map，
	// 测试环境需先为该 group 建立一条 Ability 记录，否则 nil map 赋值会 panic。
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20),
	}).Error)
	model.InitChannelCache()

	// 未命中亲和：retry=1 选中层 1 → 渠道 2（priority 19）
	param1 := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch1, _, err1 := CacheGetRandomSatisfiedChannel(param1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "无亲和时 retry=1 应选中层 1 的渠道 2")

	// 亲和命中：缓存渠道 6（无实际 6 渠道影响，仅设置标志）→ retry=1 偏移为 0 → 选中最高优先级渠道 1
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective, "亲和命中时 retry=1 应偏移到 0")
	// 与 controller/relay.go getChannel 一致：偏移值经 SetRetry 注入后再选择渠道
	param2 := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	param2.SetRetry(effective)
	ch2, _, err2 := CacheGetRandomSatisfiedChannel(param2)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中时重试应能选中最高优先级渠道 1")
}

// createPriorityLayeredChannels 为同一分组/模型建立「每层单渠道」的确定性候选池，
// priorities 按降序给出（下标 0 = 最高层）。返回的渠道 id 与层序一一对应。
func createPriorityLayeredChannels(t *testing.T, db *gorm.DB, group, modelName string, firstID int, priorities []int64) []int {
	t.Helper()
	ids := make([]int, 0, len(priorities))
	for i, priority := range priorities {
		id := firstID + i
		weight := uint(100)
		p := priority
		require.NoError(t, db.Create(&model.Channel{
			Id:       id,
			Type:     constant.ChannelTypeOpenAI,
			Key:      fmt.Sprintf("key-%d", id),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("channel-%d", id),
			Weight:   &weight,
			Models:   modelName,
			Group:    group,
			Priority: &p,
		}).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group:     group,
			Model:     modelName,
			ChannelId: id,
			Enabled:   true,
			Priority:  &p,
			Weight:    weight,
		}).Error)
		ids = append(ids, id)
	}
	return ids
}

func descentTestContext(t *testing.T, modelName, tokenGroup string) *RetryParam {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	RequestPolicy(ctx).SessionMode = "descent"
	return &RetryParam{Ctx: ctx, TokenGroup: tokenGroup, ModelName: modelName, Retry: intPtr(0)}
}

// 环序 = 锚点层 + retry 逐层向下并在最低层回绕最高层。锚点层在中间与最低层两种输入。
func TestDescentRingFollowsPriorityLayers(t *testing.T) {
	const modelName = "descent-ring-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 7101, []int64{30, 20, 10})
	model.InitChannelCache()

	cases := []struct {
		name       string
		seedRetry  int
		seedWantID int
		wantRing   []int // priorityRetry 1、2、3 依次选中的渠道 id
	}{
		{
			name:       "anchor in middle layer",
			seedRetry:  1,
			seedWantID: ids[1],
			wantRing:   []int{ids[2], ids[0], ids[1]},
		},
		{
			name:       "anchor in lowest layer",
			seedRetry:  2,
			seedWantID: ids[2],
			wantRing:   []int{ids[0], ids[1], ids[2]},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			param := descentTestContext(t, modelName, "default")
			seed, _, err := CacheGetRandomSatisfiedChannelWithPriority(param, tc.seedRetry)
			require.NoError(t, err)
			require.NotNil(t, seed)
			require.Equal(t, tc.seedWantID, seed.Id, "锚点层由首次选中的渠道决定")

			for i, wantID := range tc.wantRing {
				got, _, err := CacheGetRandomSatisfiedChannelWithPriority(param, i+1)
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, wantID, got.Id, "retry=%d 应落在环序第 %d 个渠道", i+1, i+1)
			}
		})
	}
}

// 锚点在最高层时环序与常规降序逐位一致（无亲和把首轮拉低的情形）。
func TestDescentFromTopMatchesPlainDescending(t *testing.T) {
	const modelName = "descent-top-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 7201, []int64{30, 20, 10})
	model.InitChannelCache()

	descent := descentTestContext(t, modelName, "default")
	descentIDs := make([]int, 0, len(ids))
	for retry := range len(ids) {
		got, _, err := CacheGetRandomSatisfiedChannelWithPriority(descent, retry)
		require.NoError(t, err)
		require.NotNil(t, got)
		descentIDs = append(descentIDs, got.Id)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	baseline := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}
	baselineIDs := make([]int, 0, len(ids))
	for retry := range len(ids) {
		got, _, err := CacheGetRandomSatisfiedChannelWithPriority(baseline, retry)
		require.NoError(t, err)
		require.NotNil(t, got)
		baselineIDs = append(baselineIDs, got.Id)
	}

	assert.Equal(t, baselineIDs, descentIDs, "锚点在最高层时环序必须逐位等于常规降序")
	assert.Equal(t, ids, descentIDs)
}

// descent 的层下标是「锚点层 + retry」，retry 前移会重选原层，因此必须跳过偏移。
func TestAffinityAdjustedRetrySkipsShiftForDescent(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "descent"})
	MarkChannelAffinityUsed(ctx, "default", 6)
	RequestPolicy(ctx).SessionMode = "descent"
	require.Equal(t, 1, AffinityAdjustedRetry(ctx, 1), "descent 模式不得前移 retry")

	preferCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "prefer"})
	MarkChannelAffinityUsed(preferCtx, "default", 6)
	RequestPolicy(preferCtx).SessionMode = "prefer"
	require.Equal(t, 0, AffinityAdjustedRetry(preferCtx, 1), "prefer 模式仍前移 retry（无回归）")
}

// 同一请求内 auto 跨组时，新分组不沿用锚点所在分组的环游标（D3 的 false 分支）。
func TestDescentRingDoesNotCrossGroupsWithinSingleRequest(t *testing.T) {
	const modelName = "descent-cross-group-model"
	db := setupChannelSelectAutoGroupsTest(t)
	vipPriorities := []int64{30, 20, 10}
	createPriorityLayeredChannels(t, db, "vip", modelName, 7301, vipPriorities)
	defaultIDs := createPriorityLayeredChannels(t, db, "default", modelName, 7311, []int64{30, 20, 10})
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","default"]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	// 跳过 vip，本组从 default 开始搜索：锚点仍钉在 vip 的中间层。
	common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 1)
	RequestPolicy(ctx).SessionMode = "descent"
	ctx.Set(ginKeyChannelAffinityDescent, descentAnchorState{group: "vip", priority: vipPriorities[1]})

	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: modelName, Retry: intPtr(1)}
	got, group, err := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "default", group)
	assert.Equal(t, defaultIDs[1], got.Id, "同一请求内跨组时新组从最高层起算，不沿用锚点环游标")
}

// 亲和命中时首轮复用原渠道，且锚点由亲和渠道的优先级决定；重试从该层的下一层（环序）推进。
func TestSelectChannelForRequestDescentAnchorFromAffinityHit(t *testing.T) {
	const modelName = "descent-affinity-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 7501, []int64{30, 20, 10})
	model.InitChannelCache()

	affinityValue := fmt.Sprintf("descent-affinity-%d", time.Now().UnixNano())
	rule := operation_setting.ChannelAffinityRule{
		Name:        "descent-affinity",
		ModelRegex:  []string{".*"},
		KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Key"}},
		SessionMode: "descent",
	}
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, "default", affinityValue)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, ids[1], time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKeySuffix}) })

	setting := operation_setting.GetChannelAffinitySetting()
	originalRules := setting.Rules
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() { setting.Rules = originalRules })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Affinity-Key", affinityValue)

	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}
	first, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, selectErr)
	require.NotNil(t, first)
	require.Equal(t, ids[1], first.Id, "首轮必须复用亲和渠道（非最高层）")
	require.Equal(t, "default", group)

	anchorGroup, anchorPriority, ok := descentAnchor(ctx)
	require.True(t, ok, "亲和命中后必须写入 descent 锚点")
	assert.Equal(t, "default", anchorGroup)
	assert.Equal(t, int64(20), anchorPriority, "锚点优先级来自亲和渠道")
	_, keyExists := ctx.Get(ginKeyChannelAffinityDescent)
	assert.True(t, keyExists, "gin key channel_affinity_descent_anchor 必须已写入")

	param.SetRetry(1)
	next, _, err := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, ids[2], next.Id, "重试应从锚点层的下一层推进，而不是回到最高层")
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

func TestNextFallbackChannelAdvancesLayerPointer(t *testing.T) {
	const modelName = "fallback-advance-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 8101, []int64{30, 20, 10})
	markFallbackChannels(t, ids)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, HasFallbackChannels())
	require.False(t, FallbackStageActive(ctx))

	first := NextFallbackChannel(ctx, "default", modelName)
	require.NotNil(t, first)
	assert.Equal(t, ids[0], first.Id, "首次必须返回优先级最高层的兜底渠道")
	assert.True(t, FallbackStageActive(ctx), "取得兜底渠道后请求应处于兜底阶段")

	second := NextFallbackChannel(ctx, "default", modelName)
	require.NotNil(t, second)
	assert.Equal(t, ids[1], second.Id, "再次调用必须推进到下一层")
}

func TestNextFallbackChannelReturnsNilWhenExhausted(t *testing.T) {
	const modelName = "fallback-exhausted-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 8201, []int64{30, 20})
	markFallbackChannels(t, ids)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	first := NextFallbackChannel(ctx, "default", modelName)
	second := NextFallbackChannel(ctx, "default", modelName)
	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.NotEqual(t, first.Id, second.Id, "不得重复返回同一层")

	assert.Nil(t, NextFallbackChannel(ctx, "default", modelName), "层耗尽后返回 nil")
	assert.Nil(t, NextFallbackChannel(ctx, "default", modelName), "耗尽后不得回绕到已用过的层")
}

func TestNextFallbackChannelWithoutPoolReturnsNil(t *testing.T) {
	const modelName = "fallback-no-pool-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 8301, []int64{30})
	require.NotEmpty(t, ids)
	markFallbackChannels(t, nil)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, HasFallbackChannels())
	assert.Nil(t, NextFallbackChannel(ctx, "default", modelName))
	assert.False(t, FallbackStageActive(ctx), "未配置兜底池时不得进入兜底阶段")
	assert.Nil(t, NextFallbackChannel(nil, "default", modelName), "nil 上下文必须返回 nil")
}

// The pool is open only when the global switch is on and at least one enabled
// channel carries the fallback marker.
func TestHasFallbackChannelsFollowsMarkerAndGlobalSwitch(t *testing.T) {
	const modelName = "has-fallback-marker-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 8351, []int64{30})
	require.NotEmpty(t, ids)

	markFallbackChannels(t, nil)
	assert.False(t, HasFallbackChannels(), "no marked channel must not open the pool")

	markFallbackChannels(t, ids)
	assert.True(t, HasFallbackChannels())

	setting := operation_setting.GetChannelAffinitySetting()
	previous := setting.FallbackEnabled
	setting.FallbackEnabled = false
	t.Cleanup(func() { setting.FallbackEnabled = previous })
	assert.False(t, HasFallbackChannels(), "the global fallback switch off must close the pool")
}

func TestFallbackGroupResolution(t *testing.T) {
	const modelName = "fallback-group-model"
	db := setupChannelSelectAutoGroupsTest(t)
	vipIDs := createPriorityLayeredChannels(t, db, "vip", modelName, 8401, []int64{30})
	defaultIDs := createPriorityLayeredChannels(t, db, "default", modelName, 8411, []int64{30})
	markFallbackChannels(t, slices.Concat(vipIDs, defaultIDs))
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := NextFallbackChannel(ctx, "vip", modelName)
	require.NotNil(t, channel)
	assert.Equal(t, vipIDs[0], channel.Id, "非 auto 必须使用令牌分组")

	selectedCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(selectedCtx, constant.ContextKeyAutoGroup, "default")
	channel = NextFallbackChannel(selectedCtx, "auto", modelName)
	require.NotNil(t, channel)
	assert.Equal(t, defaultIDs[0], channel.Id, "auto 必须优先使用本请求已选中的分组")

	autoCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(autoCtx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(autoCtx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	channel = NextFallbackChannel(autoCtx, "auto", modelName)
	require.NotNil(t, channel)
	assert.Equal(t, vipIDs[0], channel.Id, "未选中分组时取第一个 auto 分组")
}

// installStrictAffinityRule installs a strict-mode affinity rule whose cached
// channel is preferChannelID, so the first attempt's affinity binding is
// unavailable when that id cannot be used. The returned context carries the
// matching affinity header.
func installStrictAffinityRule(t *testing.T, modelName, ruleName, affinityValue string, preferChannelID int) *gin.Context {
	t.Helper()
	rule := operation_setting.ChannelAffinityRule{
		Name:        ruleName,
		ModelRegex:  []string{".*"},
		KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Key"}},
		SessionMode: "strict",
	}
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, "default", affinityValue)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, preferChannelID, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKeySuffix}) })

	affinitySetting := operation_setting.GetChannelAffinitySetting()
	originalRules := affinitySetting.Rules
	affinitySetting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() { affinitySetting.Rules = originalRules })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Affinity-Key", affinityValue)
	return ctx
}

func TestSelectChannelForRequestUsesFallbackWhenStrictAffinityUnavailable(t *testing.T) {
	const modelName = "strict-fallback-model"
	db := setupChannelSelectAutoGroupsTest(t)
	fallbackIDs := createPriorityLayeredChannels(t, db, "default", modelName, 8601, []int64{30})
	markFallbackChannels(t, fallbackIDs)
	model.InitChannelCache()

	ctx := installStrictAffinityRule(t, modelName, "strict-fallback", fmt.Sprintf("strict-fallback-%d", time.Now().UnixNano()), 999999)
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

	channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, selectErr, "有合格兜底渠道时 strict 亲和不可用不得返回 503")
	require.NotNil(t, channel)
	assert.Equal(t, fallbackIDs[0], channel.Id)
	assert.Equal(t, "default", group)
	assert.True(t, FallbackStageActive(ctx))
}

func TestSelectChannelForRequestUsesFallbackWhenNormalPoolEmpty(t *testing.T) {
	const modelName = "empty-pool-fallback-model"
	db := setupChannelSelectAutoGroupsTest(t)
	fallbackIDs := createPriorityLayeredChannels(t, db, "default", modelName, 8701, []int64{30})
	markFallbackChannels(t, fallbackIDs)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

	channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, selectErr, "正常候选池为空且有兜底渠道时不得返回无可用渠道")
	require.NotNil(t, channel)
	assert.Equal(t, fallbackIDs[0], channel.Id, "池内仅 1 台兜底渠道仍必须被尝试")
	assert.Equal(t, "default", group)
	assert.True(t, FallbackStageActive(ctx))
	assert.Empty(t, RequestPolicy(ctx).Events(), "使用兜底渠道不得记录 fallback_exhausted")
}

func TestSelectChannelForRequestContinuesFallbackStage(t *testing.T) {
	const modelName = "continue-fallback-model"
	db := setupChannelSelectAutoGroupsTest(t)
	fallbackIDs := createPriorityLayeredChannels(t, db, "default", modelName, 8801, []int64{30, 20})
	normalIDs := createPriorityLayeredChannels(t, db, "default", modelName, 8851, []int64{30})
	markFallbackChannels(t, fallbackIDs)
	model.InitChannelCache()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

	require.NotNil(t, NextFallbackChannel(ctx, "default", modelName))
	require.True(t, FallbackStageActive(ctx))

	second, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, selectErr)
	require.NotNil(t, second)
	assert.Equal(t, fallbackIDs[1], second.Id, "已处于兜底阶段时必须返回下一兜底层")
	assert.Equal(t, "default", group)
	require.NotEqual(t, normalIDs[0], second.Id)

	exhausted, exhaustedGroup, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, exhausted)
	require.NotNil(t, selectErr, "兜底层耗尽后必须返回无可用渠道错误，不得回落到正常候选")
	assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
	assert.Equal(t, i18n.MsgDistributorNoAvailableChannel, selectErr.MessageID)
	assert.True(t, selectErr.NoAvailableChannel)
	assert.Equal(t, "", exhaustedGroup)
}

func TestSelectChannelForRequestKeeps503WithoutFallbackPool(t *testing.T) {
	const modelName = "strict-no-fallback-model"
	db := setupChannelSelectAutoGroupsTest(t)
	createPriorityLayeredChannels(t, db, "default", modelName, 8901, []int64{30})
	markFallbackChannels(t, nil)
	model.InitChannelCache()

	ctx := installStrictAffinityRule(t, modelName, "strict-no-fallback", fmt.Sprintf("strict-no-fallback-%d", time.Now().UnixNano()), 999999)
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

	channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
	require.Nil(t, channel)
	require.NotNil(t, selectErr)
	assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
	assert.Equal(t, "strict_session_binding_unavailable", selectErr.Message)
	assert.Equal(t, "", group)
	assert.False(t, FallbackStageActive(ctx), "未配置兜底池时不得进入兜底阶段")
}

// The spec scenario 候选池为空且无兜底渠道 requires one fallback_exhausted decision
// event when a configured pool cannot serve an otherwise empty normal pool. An
// unconfigured pool must stay byte-for-byte unchanged and record nothing (D10).
func TestFallbackExhaustedEventRecorded(t *testing.T) {
	t.Run("configured pool with no eligible layer", func(t *testing.T) {
		const modelName = "fallback-exhausted-event-model"
		db := setupChannelSelectAutoGroupsTest(t)
		// The pool member exists but serves another model, so HasFallbackChannels
		// is true while no fallback layer is eligible for this request.
		createPriorityLayeredChannels(t, db, "default", "fallback-other-model", 9101, []int64{30})
		markFallbackChannels(t, []int{9101})
		model.InitChannelCache()

		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

		channel, _, selectErr := SelectChannelForRequest(ctx, modelName, param)
		require.Nil(t, channel)
		require.NotNil(t, selectErr)
		assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
		assert.True(t, selectErr.NoAvailableChannel)

		events := RequestPolicy(ctx).Events()
		require.Len(t, events, 1, "exactly one fallback_exhausted event must be recorded")
		assert.Equal(t, PolicyDecision{Action: "stop", Reason: "fallback_exhausted", Source: "global"}, events[0].Decision)
	})

	t.Run("no configured pool records nothing", func(t *testing.T) {
		const modelName = "fallback-exhausted-no-pool-model"
		db := setupChannelSelectAutoGroupsTest(t)
		createPriorityLayeredChannels(t, db, "default", "fallback-other-model", 9201, []int64{30})
		markFallbackChannels(t, nil)
		model.InitChannelCache()

		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

		channel, _, selectErr := SelectChannelForRequest(ctx, modelName, param)
		require.Nil(t, channel)
		require.NotNil(t, selectErr)
		assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
		assert.True(t, selectErr.NoAvailableChannel)
		assert.Empty(t, RequestPolicy(ctx).Events(), "an unconfigured pool must not record events")
	})
}

// The strict affinity-unavailable exit also represents "normal candidates
// unavailable" (spec 正常候选池为空时进入兜底阶段). A configured pool that cannot
// serve the request must record one fallback_exhausted event; an unconfigured
// pool must record nothing (D10).
func TestStrictAffinityUnavailableRecordsFallbackExhausted(t *testing.T) {
	countExhausted := func(events []PolicyEvent) int {
		n := 0
		for _, event := range events {
			if event.Decision.Reason == "fallback_exhausted" {
				n++
			}
		}
		return n
	}

	t.Run("configured pool with no eligible layer", func(t *testing.T) {
		const modelName = "strict-fallback-exhausted-model"
		db := setupChannelSelectAutoGroupsTest(t)
		// The pool member serves another model, so HasFallbackChannels is true
		// while no fallback layer is eligible for this request.
		createPriorityLayeredChannels(t, db, "default", "strict-other-model", 9401, []int64{30})
		markFallbackChannels(t, []int{9401})
		model.InitChannelCache()

		ctx := installStrictAffinityRule(t, modelName, "strict-exhausted", fmt.Sprintf("strict-exhausted-%d", time.Now().UnixNano()), 999999)
		param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

		channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
		require.Nil(t, channel)
		require.NotNil(t, selectErr)
		assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
		assert.Equal(t, "strict_session_binding_unavailable", selectErr.Message)
		assert.Equal(t, "", group)
		assert.Equal(t, 1, countExhausted(RequestPolicy(ctx).Events()), "exactly one fallback_exhausted event must be recorded")
	})

	t.Run("no configured pool records nothing", func(t *testing.T) {
		const modelName = "strict-no-pool-exhausted-model"
		db := setupChannelSelectAutoGroupsTest(t)
		createPriorityLayeredChannels(t, db, "default", "strict-other-model", 9501, []int64{30})
		markFallbackChannels(t, nil)
		model.InitChannelCache()

		ctx := installStrictAffinityRule(t, modelName, "strict-no-pool-exhausted", fmt.Sprintf("strict-no-pool-exhausted-%d", time.Now().UnixNano()), 999999)
		param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}

		channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
		require.Nil(t, channel)
		require.NotNil(t, selectErr)
		assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
		assert.Equal(t, "strict_session_binding_unavailable", selectErr.Message)
		assert.Equal(t, "", group)
		assert.Zero(t, countExhausted(RequestPolicy(ctx).Events()), "an unconfigured pool must not record fallback_exhausted")
	})
}

// Without a configured fallback pool the descent ring must be unchanged: the
// pool must not alter layer order or wrap-around.
func TestDescentRingUnchangedWithoutFallbackPool(t *testing.T) {
	const modelName = "descent-no-fallback-model"
	db := setupChannelSelectAutoGroupsTest(t)
	ids := createPriorityLayeredChannels(t, db, "default", modelName, 9301, []int64{30, 20, 10})
	markFallbackChannels(t, nil)
	model.InitChannelCache()

	param := descentTestContext(t, modelName, "default")
	seed, _, err := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err)
	require.NotNil(t, seed)
	require.Equal(t, ids[1], seed.Id, "锚点层由首次选中的渠道决定")

	ring := make([]int, 0, len(ids))
	for retry := 1; retry <= len(ids); retry++ {
		got, _, err := CacheGetRandomSatisfiedChannelWithPriority(param, retry)
		require.NoError(t, err)
		require.NotNil(t, got)
		ring = append(ring, got.Id)
	}
	assert.Equal(t, []int{ids[2], ids[0], ids[1]}, ring, "未配置兜底池时 descent 环序必须逐位不变")
}

// Design D3: with the global fallback switch off, channels carrying the marker
// must leave request selection identical to an unmarked deployment. The two
// exits that consult the pool are covered: the strict affinity-unavailable
// branch must not record fallback_exhausted, and an empty normal candidate pool
// must fail exactly like an unmarked pool. Each subtest compares switch-off with
// marked channels against switch-on with nothing marked.
func TestFallbackSwitchOffMatchesUnmarkedPool(t *testing.T) {
	type selectionOutcome struct {
		channelID int
		group     string
		status    int
		message   string
		messageID string
		noChannel bool
		stage     bool
		reasons   []string
		exhausted int
	}
	record := func(ctx *gin.Context, channel *model.Channel, group string, selectErr *ChannelSelectError) selectionOutcome {
		outcome := selectionOutcome{channelID: -1, group: group, stage: FallbackStageActive(ctx)}
		for _, event := range RequestPolicy(ctx).Events() {
			outcome.reasons = append(outcome.reasons, event.Decision.Reason)
			if event.Decision.Reason == "fallback_exhausted" {
				outcome.exhausted++
			}
		}
		if channel != nil {
			outcome.channelID = channel.Id
		}
		if selectErr != nil {
			outcome.status = selectErr.StatusCode
			outcome.message = selectErr.Message
			outcome.messageID = selectErr.MessageID
			outcome.noChannel = selectErr.NoAvailableChannel
		}
		return outcome
	}

	affinitySetting := operation_setting.GetChannelAffinitySetting()
	previousFallbackEnabled := affinitySetting.FallbackEnabled
	t.Cleanup(func() { affinitySetting.FallbackEnabled = previousFallbackEnabled })

	t.Run("strict affinity unavailable", func(t *testing.T) {
		const modelName = "switch-off-strict-model"
		db := setupChannelSelectAutoGroupsTest(t)
		ids := createPriorityLayeredChannels(t, db, "default", modelName, 9601, []int64{30})
		require.NotEmpty(t, ids)

		run := func(marked []int, fallbackEnabled bool) selectionOutcome {
			affinitySetting.FallbackEnabled = fallbackEnabled
			markFallbackChannels(t, marked)
			ctx := installStrictAffinityRule(t, modelName, "switch-off-strict", fmt.Sprintf("switch-off-strict-%d", time.Now().UnixNano()), 999999)
			param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}
			channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
			return record(ctx, channel, group, selectErr)
		}

		markedOff := run(ids, false)
		unmarkedOn := run(nil, true)

		require.Equal(t, -1, markedOff.channelID, "switch off must not select the marked channel through the pool")
		require.Equal(t, http.StatusServiceUnavailable, markedOff.status)
		require.Equal(t, "strict_session_binding_unavailable", markedOff.message)
		require.Zero(t, markedOff.exhausted, "switch off must not record fallback_exhausted")
		require.False(t, markedOff.stage, "switch off must not enter the fallback stage")
		assert.Equal(t, unmarkedOn, markedOff, "switch off must match an unmarked pool")
	})

	t.Run("empty normal candidate pool", func(t *testing.T) {
		const modelName = "switch-off-empty-pool-model"
		db := setupChannelSelectAutoGroupsTest(t)
		// The pool member serves another model, so this request finds neither a
		// normal candidate nor an eligible pool layer for its own model.
		ids := createPriorityLayeredChannels(t, db, "default", "switch-off-other-model", 9701, []int64{30})
		require.NotEmpty(t, ids)

		run := func(marked []int, fallbackEnabled bool) selectionOutcome {
			affinitySetting.FallbackEnabled = fallbackEnabled
			markFallbackChannels(t, marked)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: intPtr(0)}
			channel, group, selectErr := SelectChannelForRequest(ctx, modelName, param)
			return record(ctx, channel, group, selectErr)
		}

		markedOff := run(ids, false)
		unmarkedOn := run(nil, true)

		require.Equal(t, -1, markedOff.channelID)
		require.Equal(t, http.StatusServiceUnavailable, markedOff.status)
		require.True(t, markedOff.noChannel)
		require.Zero(t, markedOff.exhausted, "switch off must not record fallback_exhausted")
		require.False(t, markedOff.stage)
		assert.Equal(t, unmarkedOn, markedOff, "switch off must match an unmarked pool")
	})
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestCacheGetRandomWithPriorityKeepsCrossGroupRetryState(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	// 组 default：渠道 1(pri 20)、渠道 2(pri 19)；组 vip：渠道 3(pri 18)、渠道 4(pri 17)
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
		{Id: 3, Type: 1, Key: "k3", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(18)},
		{Id: 4, Type: 1, Key: "k4", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(17)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	// 为两个组播种 Ability（group 集合）
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: "m1", ChannelId: 3, Enabled: true, Priority: int64Ptr(18)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "m1", Retry: intPtr(1)}
	// 未命中亲和：WithPriority(param, 1) 与 CacheGetRandomSatisfiedChannel(param) 等价
	ch1, _, err1 := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "未命中亲和 retry=1 应选中 default 组层 1 的渠道 2")

	// 关键断言：priorityRetry 偏移只影响层索引、不写回 param；内部跨组状态机变异
	// （SetRetry(0) + ResetRetryNextTry）仍作用在原始 param 上（复现既有行为）。
	// fixture 中 common.RetryTimes=0 且 crossGroupRetry=true，auto 命中路径
	// priorityRetry(=1) >= RetryTimes(=0) 恒成立，因此调用后 param.Retry 被状态机
	// 置 0、resetNextTry 置位——这正是 clone 方案会丢失的变异（回归根因）。
	// 对照实证：原入口 CacheGetRandomSatisfiedChannel 同一场景下事后状态完全相同
	// （GetRetry()=0, resetNextTry=true），二者等价。
	require.Equal(t, 0, param.GetRetry(), "跨组状态机 SetRetry(0) 必须作用在原始 param 上")
	require.True(t, param.resetNextTry, "ResetRetryNextTry 必须在原始 param 上置位")
}

func TestCacheGetWithPriorityAffinityOffsetKeepsStateMachine(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))

	// 亲和命中：retry=1 偏移为 0 → 选中 highest priority（渠道 1）
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective)
	param := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch2, _, err2 := CacheGetRandomSatisfiedChannelWithPriority(param, effective)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中偏移后应选中最高优先级渠道 1")
	require.Equal(t, 1, param.GetRetry(), "循环计数不被偏移污染")
}
