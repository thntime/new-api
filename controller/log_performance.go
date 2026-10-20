package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetLogPerformance returns one filter snapshot of log-derived performance and
// availability metrics for the admin performance-health page.
func GetLogPerformance(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if endTimestamp == 0 {
		endTimestamp = common.GetTimestamp()
	}
	if startTimestamp == 0 {
		startTimestamp = endTimestamp - 24*60*60
	}

	granularity, granularitySeconds, err := model.ResolveLogPerformanceGranularity(startTimestamp, endTimestamp, c.Query("granularity"))
	if err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}

	channel := 0
	if raw := strings.TrimPrefix(strings.TrimSpace(c.Query("channel")), "#"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			common.ApiErrorT(c, "Invalid channel ID")
			return
		}
		channel = parsed
	}
	var isStream *bool
	switch c.Query("is_stream") {
	case "true":
		value := true
		isStream = &value
	case "false":
		value := false
		isStream = &value
	}

	result, err := model.QueryLogPerformance(model.LogPerformanceFilter{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
		Channel:        channel,
		Group:          c.Query("group"),
		TokenName:      c.Query("token_name"),
		Username:       c.Query("username"),
		IsStream:       isStream,
	}, granularitySeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result.Granularity = granularity
	result.ErrorLogEnabled = constant.ErrorLogEnabled

	channelIds := make([]int, 0, len(result.Channels)+len(result.SlowRequests)+len(result.Errors))
	seen := make(map[int]bool, cap(channelIds))
	for _, channel := range result.Channels {
		if channel.ChannelId != 0 && !seen[channel.ChannelId] {
			seen[channel.ChannelId] = true
			channelIds = append(channelIds, channel.ChannelId)
		}
	}
	for _, slow := range result.SlowRequests {
		if slow.ChannelId != 0 && !seen[slow.ChannelId] {
			seen[slow.ChannelId] = true
			channelIds = append(channelIds, slow.ChannelId)
		}
	}
	for _, logError := range result.Errors {
		if logError.ChannelId != 0 && !seen[logError.ChannelId] {
			seen[logError.ChannelId] = true
			channelIds = append(channelIds, logError.ChannelId)
		}
	}
	channelNames, err := model.GetChannelNameMap(channelIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range result.Channels {
		result.Channels[i].ChannelName = channelNames[result.Channels[i].ChannelId]
	}
	for i := range result.SlowRequests {
		result.SlowRequests[i].ChannelName = channelNames[result.SlowRequests[i].ChannelId]
	}
	for i := range result.Errors {
		result.Errors[i].ChannelName = channelNames[result.Errors[i].ChannelId]
	}

	common.ApiSuccess(c, result)
}
