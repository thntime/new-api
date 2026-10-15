package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newErrorMappingContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	return c, recorder
}

func TestRenderErrorResponseOverride(t *testing.T) {
	override := &types.ErrorResponseOverride{
		StatusCode: http.StatusTooManyRequests,
		Headers:    map[string]string{"x-should-retry": "true", "Retry-After": "30"},
		Message:    "rate limit exceeded",
		Type:       "rate_limit_error",
		Code:       "rate_limit_exceeded",
	}

	t.Run("openai body and headers", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatOpenAI)

		require.True(t, written)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "true", recorder.Header().Get("x-should-retry"))
		require.Equal(t, "30", recorder.Header().Get("Retry-After"))
		require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

		var body struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, "rate limit exceeded", body.Error.Message)
		require.Equal(t, "rate_limit_error", body.Error.Type)
		require.Equal(t, "rate_limit_exceeded", body.Error.Code)
	})

	t.Run("claude body without request id suffix", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatClaude)

		require.True(t, written)
		require.Equal(t, http.StatusTooManyRequests, recorder.Code)

		var body struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, "error", body.Type)
		require.Equal(t, "rate_limit_error", body.Error.Type)
		require.Equal(t, "rate limit exceeded", body.Error.Message)
	})

	t.Run("nil override is not rendered", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)

		written := renderErrorResponseOverride(c, &types.NewAPIError{}, types.RelayFormatOpenAI)

		require.False(t, written)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("committed response is left untouched", func(t *testing.T) {
		c, recorder := newErrorMappingContext(t)
		c.String(http.StatusOK, "stream-started")

		written := renderErrorResponseOverride(c, &types.NewAPIError{ResponseOverride: override}, types.RelayFormatOpenAI)

		require.False(t, written)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Empty(t, recorder.Header().Get("x-should-retry"))
		require.Equal(t, "stream-started", recorder.Body.String())
	})
}

func TestUpdateOptionRejectsInvalidErrorStatusMappingRules(t *testing.T) {
	db := modelManagementDB(t, "sqlite", "")

	const before = `[{"name":"before","enabled":false,"match":{"message_regex":"exhausted"},"output":{"status_code":429}}]`
	require.NoError(t, db.Create(&model.Option{Key: "ErrorStatusMappingRules", Value: before}).Error)
	common.OptionMap["ErrorStatusMappingRules"] = before

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	modelManagementRequest(t, UpdateOption, http.MethodPut, "/api/option/", OptionUpdateRequest{
		Key:   "ErrorStatusMappingRules",
		Value: `[{"name":"bad","enabled":true,"match":{"message_regex":"["},"output":{"status_code":429}}]`,
	}, &response)

	assert.False(t, response.Success, "invalid rules must be rejected")
	assert.NotEmpty(t, response.Message)
	assert.Equal(t, before, common.OptionMap["ErrorStatusMappingRules"], "a rejected save must not touch the option map")

	var saved model.Option
	require.NoError(t, db.Where(&model.Option{Key: "ErrorStatusMappingRules"}).First(&saved).Error)
	assert.Equal(t, before, saved.Value, "a rejected save must not touch the database")
}
