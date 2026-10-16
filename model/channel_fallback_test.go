package model

import (
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const fallbackTestGroup = "fallback-pool-test"
const fallbackTestModel = "fallback-pool-model"

type fallbackTestChannel struct {
	id          int
	priority    int64
	weight      uint
	channelType int
	disabled    bool
}

// setFallbackTestCache fills the in-memory candidate cache directly, keeping the
// previous cache and MemoryCacheEnabled flag for restore.
func setFallbackTestCache(t *testing.T, channels []fallbackTestChannel) {
	t.Helper()
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true

	sorted := make([]fallbackTestChannel, len(channels))
	copy(sorted, channels)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].priority > sorted[j].priority })

	ids := make([]int, 0, len(sorted))
	byID := make(map[int]*Channel, len(sorted))
	for _, item := range sorted {
		priority := item.priority
		weight := item.weight
		status := common.ChannelStatusEnabled
		if item.disabled {
			status = common.ChannelStatusManuallyDisabled
		}
		channel := &Channel{Id: item.id, Priority: &priority, Weight: &weight, Type: item.channelType, Status: status}
		ids = append(ids, item.id)
		byID[item.id] = channel
	}

	channelSyncLock.Lock()
	previousGroup2model2channels := group2model2channels
	previousChannelsIDM := channelsIDM
	channelSyncLock.Unlock()

	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = previousGroup2model2channels
		channelsIDM = previousChannelsIDM
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = previousMemoryCache
	})

	channelSyncLock.Lock()
	group2model2channels = map[string]map[string][]int{
		fallbackTestGroup: {fallbackTestModel: ids},
	}
	channelsIDM = byID
	channelSyncLock.Unlock()
}

// markFallbackChannels flags the given channels as fallback channels in the
// in-memory cache. Ids absent from the cache (disabled or another group/model)
// are ignored, mirroring production where only enabled candidates are reachable.
func markFallbackChannels(t *testing.T, ids []int) {
	t.Helper()
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	for _, id := range ids {
		if channel, ok := channelsIDM[id]; ok {
			channel.ChannelInfo.IsFallback = true
		}
	}
}

// setFallbackEnabled overrides the global fallback switch for the test and
// restores the previous value on cleanup.
func setFallbackEnabled(t *testing.T, enabled bool) {
	t.Helper()
	setting := operation_setting.GetChannelAffinitySetting()
	previous := setting.FallbackEnabled
	setting.FallbackEnabled = enabled
	t.Cleanup(func() { setting.FallbackEnabled = previous })
}

// insertFallbackTestChannels inserts the standard DB-path channels for the
// fallback fixture, marking fallbackIDs as fallback channels.
func insertFallbackTestChannels(t *testing.T, fallbackIDs []int) {
	t.Helper()
	ids := []int{50, 40, 30, 20, 10, 66, 55}
	priorities := []int64{50, 40, 30, 20, 10, 6, 5}
	for i, id := range ids {
		priority := priorities[i]
		weight := uint(1)
		channel := Channel{
			Id:          id,
			Type:        constant.ChannelTypeOpenAI,
			Status:      common.ChannelStatusEnabled,
			Name:        "fallback-db",
			Models:      fallbackTestModel,
			Group:       fallbackTestGroup,
			Priority:    &priority,
			Weight:      &weight,
			ChannelInfo: ChannelInfo{IsFallback: slices.Contains(fallbackIDs, id)},
		}
		require.NoError(t, channel.Insert())
	}
}

// insertFallbackTestChannel inserts one DB-path channel with the given status and
// fallback marker.
func insertFallbackTestChannel(t *testing.T, id, status int, isFallback bool) {
	t.Helper()
	priority := int64(6)
	weight := uint(1)
	channel := Channel{
		Id:          id,
		Type:        constant.ChannelTypeOpenAI,
		Status:      status,
		Name:        "fallback-db",
		Models:      fallbackTestModel,
		Group:       fallbackTestGroup,
		Priority:    &priority,
		Weight:      &weight,
		ChannelInfo: ChannelInfo{IsFallback: isFallback},
	}
	require.NoError(t, channel.Insert())
}

func TestNormalSelectionExcludesFallbackChannels(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 40, priority: 40, weight: 1},
		{id: 30, priority: 30, weight: 1},
		{id: 20, priority: 20, weight: 1},
		{id: 10, priority: 10, weight: 1},
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})
	markFallbackChannels(t, []int{66, 55})

	// Five normal layers remain; retries below the lowest layer clamp to it.
	expectedByRetry := []int{50, 40, 30, 20, 10, 10, 10}
	for retry, expected := range expectedByRetry {
		for range 20 {
			channel, err := GetRandomSatisfiedChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, expected, channel.Id, "retry %d", retry)
		}
	}
}

func TestNormalSelectionUnchangedWithoutFallbackPool(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 40, priority: 40, weight: 1},
		{id: 30, priority: 30, weight: 1},
		{id: 20, priority: 20, weight: 1},
		{id: 10, priority: 10, weight: 1},
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})

	// No marked channels leaves every candidate selectable, in the historical order.
	expectedByRetry := []int{50, 40, 30, 20, 10, 66, 55}
	for retry, expected := range expectedByRetry {
		channel, err := GetRandomSatisfiedChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, expected, channel.Id, "retry %d", retry)
	}
}

func TestFallbackChannelInSamePriorityLayerIsNotSelectedNormally(t *testing.T) {
	// The fallback channel dominates its layer by weight; only its removal
	// before the single-candidate fast path can keep it out of normal selection.
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 66, priority: 50, weight: 1000},
	})
	markFallbackChannels(t, []int{66})

	for range 50 {
		channel, err := GetRandomSatisfiedChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 50, channel.Id)
	}
}

func TestDatabaseSelectionExcludesFallbackChannels(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	insertFallbackTestChannels(t, []int{66, 55})

	expectedByRetry := []int{50, 40, 30, 20, 10, 10, 10}
	for retry, expected := range expectedByRetry {
		channel, err := GetChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, expected, channel.Id, "retry %d", retry)
	}
}

func TestGetFallbackChannelDescendsByOwnPriority(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 40, priority: 40, weight: 1},
		{id: 30, priority: 30, weight: 1},
		{id: 20, priority: 20, weight: 1},
		{id: 10, priority: 10, weight: 1},
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})
	markFallbackChannels(t, []int{66, 55})

	channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)

	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 55, channel.Id)

	// Only two fallback layers exist; unlike the normal path there is no wrap-around.
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 2, nil)
	require.NoError(t, err)
	assert.Nil(t, channel)

	// Negative indexes are treated as the top fallback layer.
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, -1, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)

	// The DB path mirrors the layer order and the non-wrapping bound.
	truncateTables(t)
	common.MemoryCacheEnabled = false
	insertFallbackTestChannels(t, []int{66, 55})
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 55, channel.Id)
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 2, nil)
	require.NoError(t, err)
	assert.Nil(t, channel)
}

func TestGetFallbackChannelPicksWeightedWithinLayer(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1},
		{id: 77, priority: 6, weight: 1},
	})
	markFallbackChannels(t, []int{66, 77})

	seen := map[int]bool{}
	for range 200 {
		channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		seen[channel.Id] = true
	}
	assert.True(t, seen[66], "weighted random must reach the first channel")
	assert.True(t, seen[77], "weighted random must reach the second channel")
}

func TestGetFallbackChannelSkipsUnqualifiedChannels(t *testing.T) {
	// 55 fails the request constraint (advanced-custom channel with no matching
	// route); 44 and 33 are marked but absent from the candidate cache, standing
	// in for disabled channels or another group/model.
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1, channelType: constant.ChannelTypeAdvancedCustom},
	})
	markFallbackChannels(t, []int{66, 55, 44, 33})

	filters := []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"}}

	channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, filters)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)

	// The only other fallback candidate is filtered out, so the next layer is empty.
	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 1, filters)
	require.NoError(t, err)
	assert.Nil(t, channel)

	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, -1, filters)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)
}

func TestGetFallbackChannelWithoutPoolReturnsNil(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})

	channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err)
	assert.Nil(t, channel)
}

func TestGetFallbackChannelSingleChannel(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 7},
	})
	markFallbackChannels(t, []int{66})

	channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id)

	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 1, nil)
	require.NoError(t, err)
	assert.Nil(t, channel)
}

// The pool is defined by the channel-level marker, not the legacy global option.
func TestIsFallbackChannelReadsChannelMarker(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})
	markFallbackChannels(t, []int{66})

	assert.True(t, IsFallbackChannel(66))
	assert.False(t, IsFallbackChannel(55), "an unmarked channel is not a fallback channel")
	assert.False(t, IsFallbackChannel(999), "an unknown channel is not a fallback channel")
	assert.False(t, IsFallbackChannel(0), "a non-positive id is never a fallback channel")
	assert.False(t, IsFallbackChannel(-1))

	setFallbackEnabled(t, false)
	assert.False(t, IsFallbackChannel(66), "the global switch off disables every fallback channel")
}

func TestHasFallbackChannelsReflectsMarkedChannels(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	})
	assert.False(t, HasFallbackChannels(), "no marked channel means no fallback pool")

	markFallbackChannels(t, []int{66})
	assert.True(t, HasFallbackChannels())

	setFallbackEnabled(t, false)
	assert.False(t, HasFallbackChannels(), "the global switch off hides the pool")
}

// The availability guard counts only enabled channels, so a marker on a disabled
// channel does not open the fallback stage. The marker itself is still reported:
// marker detection and availability are separate questions.
func TestHasFallbackChannelsIgnoresDisabledChannels(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 66, priority: 6, weight: 1, disabled: true},
	})
	markFallbackChannels(t, []int{66})
	assert.True(t, IsFallbackChannel(66), "the marker is independent of channel status")
	assert.False(t, HasFallbackChannels(), "a disabled fallback channel is not usable")

	truncateTables(t)
	common.MemoryCacheEnabled = false
	insertFallbackTestChannel(t, 66, common.ChannelStatusManuallyDisabled, true)
	assert.True(t, IsFallbackChannel(66), "the DB path reports the marker too")
	assert.False(t, HasFallbackChannels(), "the DB path also ignores the disabled channel")
}

func TestFallbackSwitchOffKeepsMarkedChannelsSelectableNormally(t *testing.T) {
	setFallbackTestCache(t, []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 66, priority: 6, weight: 1},
	})
	markFallbackChannels(t, []int{66})
	setFallbackEnabled(t, false)

	channel, err := GetRandomSatisfiedChannel(fallbackTestGroup, fallbackTestModel, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id, "with fallback disabled the marked channel stays in normal selection")

	channel, err = GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err)
	assert.Nil(t, channel, "with fallback disabled there is no fallback pool")
}

func TestDatabaseFallbackSelectionMatchesMemoryCache(t *testing.T) {
	fixture := []fallbackTestChannel{
		{id: 50, priority: 50, weight: 1},
		{id: 40, priority: 40, weight: 1},
		{id: 30, priority: 30, weight: 1},
		{id: 20, priority: 20, weight: 1},
		{id: 10, priority: 10, weight: 1},
		{id: 66, priority: 6, weight: 1},
		{id: 55, priority: 5, weight: 1},
	}

	setFallbackTestCache(t, fixture)
	markFallbackChannels(t, []int{66, 55})
	for retry, expected := range []int{50, 40, 30, 20, 10, 10, 10} {
		channel, err := GetRandomSatisfiedChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, expected, channel.Id, "memory normal selection, retry %d", retry)
	}
	memoryFallback := make([]int, 0, 2)
	for layer := range 2 {
		channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, layer, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		memoryFallback = append(memoryFallback, channel.Id)
	}
	assert.Equal(t, []int{66, 55}, memoryFallback)

	truncateTables(t)
	common.MemoryCacheEnabled = false
	insertFallbackTestChannels(t, []int{66, 55})
	for retry, expected := range []int{50, 40, 30, 20, 10, 10, 10} {
		channel, err := GetChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, expected, channel.Id, "db normal selection, retry %d", retry)
	}
	dbFallback := make([]int, 0, 2)
	for layer := range 2 {
		channel, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, layer, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		dbFallback = append(dbFallback, channel.Id)
	}
	assert.Equal(t, memoryFallback, dbFallback, "both paths must select the same fallback channels")
}

// Failure Paths (design): a failed batch channel read on the DB direct path is
// treated as "no usable fallback channels", never as a request failure.
func TestDatabaseFallbackFlagLookupFailureDegrades(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	insertFallbackTestChannels(t, []int{66, 55})

	// Fail only the fallback flag lookup: it is the sole query selecting
	// channel_info, so the earlier ability and constraint queries stay intact.
	callbackName := "test:inject_fallback_flag_failure"
	injectedErr := errors.New("injected fallback flag lookup failure")
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if slices.Contains(tx.Statement.Selects, "channel_info") {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	// Normal selection must survive the failure and skip the fallback exclusion,
	// so the marked channel stays selectable in its own priority layer.
	channel, err := GetChannel(fallbackTestGroup, fallbackTestModel, 5, nil)
	require.NoError(t, err, "a flag lookup failure must not fail normal selection")
	require.NotNil(t, channel)
	assert.Equal(t, 66, channel.Id, "the failed lookup skips exclusion instead of aborting")

	// The fallback path degrades to "no usable fallback channel".
	fallback, err := GetFallbackChannel(fallbackTestGroup, fallbackTestModel, 0, nil)
	require.NoError(t, err, "a flag lookup failure must not surface as a relay error")
	assert.Nil(t, fallback, "a failed lookup yields no fallback channel")
}

func TestIsFallbackChannelAndHasFallbackChannelsDatabasePath(t *testing.T) {
	truncateTables(t)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	insertFallbackTestChannels(t, []int{66})

	assert.True(t, IsFallbackChannel(66))
	assert.False(t, IsFallbackChannel(55))
	assert.False(t, IsFallbackChannel(0))
	assert.True(t, HasFallbackChannels())

	setFallbackEnabled(t, false)
	assert.False(t, IsFallbackChannel(66))
	assert.False(t, HasFallbackChannels())
}
