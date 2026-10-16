package codebuddy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/channel/codebuddy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURL(t *testing.T) {
	cases := []struct {
		name      string
		format    types.RelayFormat
		relayMode int
		want      string
	}{
		{"claude messages", types.RelayFormatClaude, relayconstant.RelayModeChatCompletions, "https://api.codebuddy.example/v1/messages"},
		{"openai responses", types.RelayFormatOpenAI, relayconstant.RelayModeResponses, "https://api.codebuddy.example/v1/responses"},
		{"openai chat completions", types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "https://api.codebuddy.example/v1/chat/completions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &codebuddy.Adaptor{}
			info := &relaycommon.RelayInfo{
				RelayFormat: tc.format,
				RelayMode:   tc.relayMode,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: "https://api.codebuddy.example/",
				},
			}
			got, err := a.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetRequestURLEmptyBaseURL(t *testing.T) {
	a := &codebuddy.Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: ""},
	}
	got, err := a.GetRequestURL(info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "codebuddy")
	require.Empty(t, got)
}

func TestGetRequestURLUnsupportedRelayMode(t *testing.T) {
	a := &codebuddy.Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   9999,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.codebuddy.example"},
	}
	_, err := a.GetRequestURL(info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported relay mode")
}

func TestSetupRequestHeaderByFormat(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	t.Run("claude format uses x-api-key", func(t *testing.T) {
		a := &codebuddy.Adaptor{}
		info := &relaycommon.RelayInfo{
			RelayFormat: types.RelayFormatClaude,
			ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "sk-cb-test"},
		}
		header := http.Header{}
		require.NoError(t, a.SetupRequestHeader(c, &header, info))
		assert.Equal(t, "sk-cb-test", header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", header.Get("anthropic-version"))
	})

	t.Run("openai format uses bearer token", func(t *testing.T) {
		a := &codebuddy.Adaptor{}
		info := &relaycommon.RelayInfo{
			RelayFormat: types.RelayFormatOpenAI,
			ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "sk-cb-test"},
		}
		header := http.Header{}
		require.NoError(t, a.SetupRequestHeader(c, &header, info))
		assert.Equal(t, "Bearer sk-cb-test", header.Get("Authorization"))
		assert.Empty(t, header.Get("x-api-key"))
	})
}

func TestGetChannelName(t *testing.T) {
	a := &codebuddy.Adaptor{}
	require.Equal(t, "CodeBuddy", a.GetChannelName())
}
