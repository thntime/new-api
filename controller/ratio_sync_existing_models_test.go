package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRatioSyncTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Model{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

// TestFetchUpstreamRatiosFiltersToExistingModels 验证价格同步只保留模型库中
// 已存在的模型：上游独有模型不得出现在 differences 与 prices 中。
func TestFetchUpstreamRatiosFiltersToExistingModels(t *testing.T) {
	setupRatioSyncTestDB(t)
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "gpt-5.6-luna"}).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-5.6-luna":2.5,"kimi-k3":1.0},"completion_ratio":{"gpt-5.6-luna":4,"kimi-k3":2}}}`))
	}))
	defer upstream.Close()

	body, err := common.Marshal(dto.UpstreamRequest{
		Upstreams: []dto.UpstreamDTO{{
			Name:     "test-upstream",
			BaseURL:  upstream.URL,
			Endpoint: upstream.URL + "/api/ratio_config",
		}},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/fetch", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Differences map[string]map[string]dto.DifferenceItem `json:"differences"`
			Prices      map[string]any                           `json:"prices"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	assert.Contains(t, resp.Data.Differences, "gpt-5.6-luna")
	assert.NotContains(t, resp.Data.Differences, "kimi-k3")
	assert.Contains(t, resp.Data.Prices, "gpt-5.6-luna")
	assert.NotContains(t, resp.Data.Prices, "kimi-k3")
}
