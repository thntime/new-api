package model

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if constant.IsAdvancedCustomChannelType(channel.Type) {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.SplitSeq(channel.Group, ",")
		for group := range groups {
			models := channel.GetModels()
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog(common.LogText("channels synced from database"))
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog(common.LogText("syncing channels from database"))
		InitChannelCache()
	}
}

// descentLayerIndex 返回目标优先级层下标。anchorPriority 为 nil 时保持历史语义
// （retry 即降序层下标，越界 clamp 到最低层）；否则以第一个不高于 anchorPriority
// 的层为锚点，按 retry 环形向下推进。
func descentLayerIndex(prioritiesDesc []int64, anchorPriority *int64, retry int) int {
	if anchorPriority == nil {
		return min(retry, len(prioritiesDesc)-1)
	}
	above := sort.Search(len(prioritiesDesc), func(i int) bool { return prioritiesDesc[i] <= *anchorPriority })
	// shortcut: 锚点低于全部候选层时回退为最高层起算；锚点恒来自同组已选中渠道，故生产不可达
	return (above + retry) % len(prioritiesDesc)
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	return getRandomSatisfiedChannel(group, model, retry, filters, nil)
}

// GetRandomSatisfiedChannelDescending 与 GetRandomSatisfiedChannel 相同，但从
// anchorPriority 所在的优先级层起，按 retry 环形向下推进（最低层后回绕最高层）。
func GetRandomSatisfiedChannelDescending(
	group string,
	model string,
	anchorPriority int64,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	return getRandomSatisfiedChannel(group, model, retry, filters, &anchorPriority)
}

// GetFallbackChannel selects from the global fallback pool for a single request.
// layerIndex indexes the pool's own priority layers in descending order and is
// never wrapped: an index at or beyond the last layer yields no channel. The
// pool is intersected with the request's group, model and channel constraints.
func GetFallbackChannel(
	group string,
	model string,
	layerIndex int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return getFallbackChannel(group, model, layerIndex, filters)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if layerIndex < 0 {
		layerIndex = 0
	}

	// Same candidate source as normal selection: the exact model key first,
	// then the normalized model name when the exact key yields nothing.
	candidateIDs, _ := filterCandidateIDs(fallbackPoolMembers(group2model2channels[group][model]), model, filters)
	if len(candidateIDs) == 0 {
		normalizedModel := ratio_setting.RoutingMatchModelName(model)
		candidateIDs, _ = filterCandidateIDs(fallbackPoolMembers(group2model2channels[group][normalizedModel]), model, filters)
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	uniquePriorities := make(map[int64]bool)
	for _, channelId := range candidateIDs {
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", channelId)
		}
		uniquePriorities[channel.GetPriority()] = true
	}
	sortedUniquePriorities := make([]int64, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Slice(sortedUniquePriorities, func(i, j int) bool { return sortedUniquePriorities[i] > sortedUniquePriorities[j] })

	if layerIndex >= len(sortedUniquePriorities) {
		return nil, nil
	}
	targetPriority := sortedUniquePriorities[layerIndex]

	targetChannels := make([]*Channel, 0, len(candidateIDs))
	for _, channelId := range candidateIDs {
		if channel, ok := channelsIDM[channelId]; ok && channel.GetPriority() == targetPriority {
			targetChannels = append(targetChannels, channel)
		}
	}
	return pickWeightedChannel(targetChannels), nil
}

// excludeFallbackChannels drops fallback-marked channels from normal candidates.
// The input slice is never mutated. Callers must hold channelSyncLock.
func excludeFallbackChannels(ids []int) []int {
	if len(ids) == 0 {
		return ids
	}
	kept := make([]int, 0, len(ids))
	for _, id := range ids {
		if !isFallbackChannelLocked(id) {
			kept = append(kept, id)
		}
	}
	return kept
}

// fallbackPoolMembers keeps only fallback-marked channels. The input slice is
// never mutated. Callers must hold channelSyncLock.
func fallbackPoolMembers(ids []int) []int {
	return slices.DeleteFunc(slices.Clone(ids), func(id int) bool {
		return !isFallbackChannelLocked(id)
	})
}

func fallbackEnabled() bool {
	return operation_setting.GetChannelAffinitySetting().FallbackEnabled
}

// isFallbackChannelLocked reports whether the cached channel is an active
// fallback channel. Callers must hold channelSyncLock (read or write).
func isFallbackChannelLocked(channelID int) bool {
	if !fallbackEnabled() {
		return false
	}
	channel, ok := channelsIDM[channelID]
	return ok && channel.ChannelInfo.IsFallback
}

// IsFallbackChannel reports whether the channel is marked as a fallback channel.
// A non-positive id, the global fallback switch off, or an unknown channel all
// yield false. The marker lives in the channel's own ChannelInfo, so the global
// option array is no longer consulted.
func IsFallbackChannel(channelID int) bool {
	if channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		if !fallbackEnabled() {
			return false
		}
		flags, err := fallbackFlagsByIDs([]int{channelID})
		if err != nil {
			return false
		}
		return flags[channelID]
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return isFallbackChannelLocked(channelID)
}

// HasFallbackChannels reports whether the global switch is on and at least one
// channel carries the fallback marker.
func HasFallbackChannels() bool {
	if !fallbackEnabled() {
		return false
	}
	if !common.MemoryCacheEnabled {
		// shortcut: without the memory cache this scans every enabled channel once.
		// HasFallbackChannels is a low-frequency guard, not a per-request path, so
		// the scan is acceptable; add an indexed marker column if that changes.
		var channels []Channel
		if err := DB.Select("id", "channel_info").Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
			return false
		}
		for i := range channels {
			if channels[i].ChannelInfo.IsFallback {
				return true
			}
		}
		return false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	for _, channel := range channelsIDM {
		if channel.ChannelInfo.IsFallback {
			return true
		}
	}
	return false
}

// fallbackFlagsByIDs reports which of the given channel ids carry the fallback
// marker, using a single IN query. Ids absent from the table are absent from the
// map, which reads as false.
func fallbackFlagsByIDs(ids []int) (map[int]bool, error) {
	flags := make(map[int]bool, len(ids))
	if len(ids) == 0 {
		return flags, nil
	}
	var channels []Channel
	if err := DB.Select("id", "channel_info").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		flags[channels[i].Id] = channels[i].ChannelInfo.IsFallback
	}
	return flags, nil
}

func getRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
	anchorPriority *int64,
) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return getChannel(group, model, retry, filters, anchorPriority)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels, _ := filterCandidateIDs(group2model2channels[group][model], model, filters)
	channels = excludeFallbackChannels(channels)

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.RoutingMatchModelName(model)
		channels, _ = filterCandidateIDs(group2model2channels[group][normalizedModel], model, filters)
		channels = excludeFallbackChannels(channels)
	}

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", channels[0])
	}

	uniquePriorities := make(map[int64]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[channel.GetPriority()] = true
		} else {
			return nil, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", channelId)
		}
	}
	sortedUniquePriorities := make([]int64, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Slice(sortedUniquePriorities, func(i, j int) bool { return sortedUniquePriorities[i] > sortedUniquePriorities[j] })

	targetPriority := sortedUniquePriorities[descentLayerIndex(sortedUniquePriorities, anchorPriority, retry)]

	// get the priority for the given retry number
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("database consistency error: channel #%d does not exist, please contact the administrator", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	if channel := pickWeightedChannel(targetChannels); channel != nil {
		return channel, nil
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

// pickWeightedChannel picks one channel from a single priority layer by weighted
// random, applying the historical smoothing rules. It returns nil when the draw
// matches no channel.
func pickWeightedChannel(targetChannels []*Channel) *Channel {
	if len(targetChannels) == 0 {
		return nil
	}
	sumWeight := 0
	for _, channel := range targetChannels {
		sumWeight += channel.GetWeight()
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel
		}
	}
	return nil
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("channel #%d no longer exists", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("channel #%d no longer exists", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if constant.IsAdvancedCustomChannelType(channel.Type) {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}
