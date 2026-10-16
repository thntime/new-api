package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fallback marker is written through the ordinary channel update request
// body as part of channel_info and must merge with the stored object instead of
// replacing it, so unrelated keys such as is_multi_key survive.
func TestUpdateChannelPersistsFallbackMarkerAndPreservesChannelInfoKeys(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Name:   "fallback-marking",
		Key:    "sk",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, channel.Insert())

	body := fmt.Sprintf(
		`{"id":%d,"type":1,"name":"fallback-marking","key":"sk","models":"gpt-4o","group":"default","channel_info":{"is_fallback":true}}`,
		channel.Id,
	)
	recorder := putUpdateChannel(t, 1, common.RoleRootUser, body)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.True(t, stored.ChannelInfo.IsFallback, "is_fallback must be persisted")
	assert.True(t, stored.ChannelInfo.IsMultiKey, "unrelated channel_info keys must be preserved")
	assert.Equal(t, constant.MultiKeyModePolling, stored.ChannelInfo.MultiKeyMode)
}

// Clearing the marker must persist too: the resulting channel_info marshals to
// its zero JSON value, which a plain struct update would silently skip.
func TestUpdateChannelClearsFallbackMarker(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	channel := model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Name:        "fallback-unmark",
		Key:         "sk",
		Models:      "gpt-4o",
		Group:       "default",
		Status:      common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{IsFallback: true},
	}
	require.NoError(t, channel.Insert())

	body := fmt.Sprintf(
		`{"id":%d,"type":1,"name":"fallback-unmark","key":"sk","models":"gpt-4o","group":"default","channel_info":{"is_fallback":false}}`,
		channel.Id,
	)
	recorder := putUpdateChannel(t, 1, common.RoleRootUser, body)
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.ChannelInfo.IsFallback, "clearing is_fallback must persist")
}
