package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
