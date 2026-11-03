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
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func createAbility(t *testing.T, name string, channelID int, enabled bool) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     name,
		ChannelId: channelID,
		Enabled:   enabled,
	}).Error)
}

type ratioSyncResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Differences map[string]map[string]dto.DifferenceItem `json:"differences"`
		Prices      map[string]struct {
			Current   map[string]any            `json:"current"`
			Upstreams map[string]map[string]any `json:"upstreams"`
		} `json:"prices"`
	} `json:"data"`
}

func fetchUpstreamRatiosForTest(t *testing.T, upstreamURL string, includeUnchanged bool) ratioSyncResponse {
	t.Helper()

	body, err := common.Marshal(map[string]any{
		"upstreams": []dto.UpstreamDTO{{
			Name:     "test-upstream",
			BaseURL:  upstreamURL,
			Endpoint: upstreamURL + "/api/ratio_config",
		}},
		"include_unchanged": includeUnchanged,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/fetch", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var resp ratioSyncResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp), recorder.Body.String())
	require.True(t, resp.Success)
	return resp
}

// TestFetchUpstreamRatiosFiltersToEnabledModels 验证价格同步只保留当前启用渠道
// 提供的模型：上游独有模型与仅存在于模型库的模型不得出现在 differences 与 prices 中。
func TestFetchUpstreamRatiosFiltersToEnabledModels(t *testing.T) {
	setupRatioSyncTestDB(t)
	createAbility(t, "gpt-5.6-luna", 1, true)
	// 只存在于模型库、没有启用渠道提供的模型不属于同步范围。
	require.NoError(t, model.DB.Create(&model.Model{ModelName: "library-only"}).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-5.6-luna":2.5,"kimi-k3":1.0,"library-only":1.5},"completion_ratio":{"gpt-5.6-luna":4,"kimi-k3":2,"library-only":2}}}`))
	}))
	defer upstream.Close()

	for _, includeUnchanged := range []bool{false, true} {
		resp := fetchUpstreamRatiosForTest(t, upstream.URL, includeUnchanged)
		assert.Contains(t, resp.Data.Differences, "gpt-5.6-luna")
		assert.NotContains(t, resp.Data.Differences, "kimi-k3")
		assert.NotContains(t, resp.Data.Differences, "library-only")
		assert.Contains(t, resp.Data.Prices, "gpt-5.6-luna")
		assert.NotContains(t, resp.Data.Prices, "kimi-k3")
		assert.NotContains(t, resp.Data.Prices, "library-only")
	}
}

// TestFetchUpstreamRatiosExcludesDisabledOnlyModels 验证只存在于停用渠道的模型
// 不进入同步结果。
func TestFetchUpstreamRatiosExcludesDisabledOnlyModels(t *testing.T) {
	setupRatioSyncTestDB(t)
	createAbility(t, "gpt-5.6-luna", 1, true)
	createAbility(t, "disabled-model", 2, false)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"gpt-5.6-luna":2.5,"disabled-model":1.0},"completion_ratio":{"gpt-5.6-luna":4,"disabled-model":2}}}`))
	}))
	defer upstream.Close()

	for _, includeUnchanged := range []bool{false, true} {
		resp := fetchUpstreamRatiosForTest(t, upstream.URL, includeUnchanged)
		assert.Contains(t, resp.Data.Differences, "gpt-5.6-luna")
		assert.NotContains(t, resp.Data.Differences, "disabled-model")
		assert.Contains(t, resp.Data.Prices, "gpt-5.6-luna")
		assert.NotContains(t, resp.Data.Prices, "disabled-model")
	}
}
