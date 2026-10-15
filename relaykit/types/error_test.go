package types

import (
	"errors"
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIErrorResponseOverride(t *testing.T) {
	newAPIError := NewError(errors.New("upstream exhausted"), ErrorCodeBadResponseStatusCode)
	override := &ErrorResponseOverride{
		StatusCode: 429,
		Headers:    map[string]string{"x-should-retry": "true", "Retry-After": "30"},
		Message:    "override-sentinel-message",
		Type:       "rate_limit_error",
		Code:       "rate_limit_exceeded",
	}

	newAPIError.ResponseOverride = override
	require.Same(t, override, newAPIError.ResponseOverride)
	assert.Equal(t, 429, newAPIError.ResponseOverride.StatusCode)

	data, err := kitutil.Marshal(newAPIError)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "ResponseOverride")
	assert.NotContains(t, string(data), "override-sentinel-message")
}
