package model

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fallbackTestGroup = "fallback-pool-test"
const fallbackTestModel = "fallback-pool-model"

type fallbackTestChannel struct {
	id       int
	priority int64
	weight   uint
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
		channel := &Channel{Id: item.id, Priority: &priority, Weight: &weight}
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

func setFallbackChannelPool(t *testing.T, ids []int) {
	t.Helper()
	setting := operation_setting.GetChannelAffinitySetting()
	previous := setting.FallbackChannelIDs
	setting.FallbackChannelIDs = ids
	t.Cleanup(func() { setting.FallbackChannelIDs = previous })
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
	setFallbackChannelPool(t, []int{66, 55})

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
	setFallbackChannelPool(t, nil)

	// An empty pool leaves every candidate selectable, in the historical order.
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
	setFallbackChannelPool(t, []int{66})

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
	setFallbackChannelPool(t, []int{66, 55})

	ids := []int{50, 40, 30, 20, 10, 66, 55}
	priorities := []int64{50, 40, 30, 20, 10, 6, 5}
	for i, id := range ids {
		priority := priorities[i]
		weight := uint(1)
		channel := Channel{
			Id:       id,
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "fallback-db",
			Models:   fallbackTestModel,
			Group:    fallbackTestGroup,
			Priority: &priority,
			Weight:   &weight,
		}
		require.NoError(t, channel.Insert())
	}

	expectedByRetry := []int{50, 40, 30, 20, 10, 10, 10}
	for retry, expected := range expectedByRetry {
		channel, err := GetChannel(fallbackTestGroup, fallbackTestModel, retry, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, expected, channel.Id, "retry %d", retry)
	}
}
