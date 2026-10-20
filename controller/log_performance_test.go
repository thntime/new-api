package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupLogPerformanceControllerTest builds the handler with the real
// AdminAuth middleware and two legacy dashboard tokens (admin and common).
func setupLogPerformanceControllerTest(t *testing.T) (*gin.Engine, string, string, int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemory, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousMaster := common.IsMasterNode

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "log-perf.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Log{}, &model.AuditLog{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.IsMasterNode = true
	require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, name) VALUES (7, 'primary')").Error)

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.MemoryCacheEnabled = previousMemory
		common.RedisEnabled = previousRedis
		common.IsMasterNode = previousMaster
	})

	adminToken := "log-perf-admin-token"
	require.NoError(t, db.Create(&model.User{
		Username: "log-perf-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
		Group: "default", AccessToken: &adminToken, AuthVersion: 1, AffCode: "log-perf-admin",
	}).Error)
	userToken := "log-perf-user-token"
	require.NoError(t, db.Create(&model.User{
		Username: "log-perf-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AccessToken: &userToken, AuthVersion: 1, AffCode: "log-perf-user",
	}).Error)

	base := int64(1_700_000_000)
	base -= base % 3600
	for range 2 {
		require.NoError(t, db.Create(&model.Log{
			CreatedAt: base, Type: model.LogTypeConsume, ModelName: "gpt-a", ChannelId: 7,
			Group: "vip", TokenName: "tok-a", Username: "alice", IsStream: true,
			UseTime: 2, CompletionTokens: 20,
		}).Error)
	}
	for range 2 {
		require.NoError(t, db.Create(&model.Log{
			CreatedAt: base, Type: model.LogTypeConsume, ModelName: "gpt-b", ChannelId: 7,
			Group: "default", TokenName: "tok-b", Username: "bob", IsStream: false,
			UseTime: 3, CompletionTokens: 30,
		}).Error)
	}
	require.NoError(t, db.Create(&model.Log{
		CreatedAt: base, Type: model.LogTypeError, ModelName: "gpt-a", ChannelId: 7,
		Group: "vip", TokenName: "tok-a", Username: "alice", Content: "upstream failed",
	}).Error)

	engine := gin.New()
	engine.GET("/api/log/performance", middleware.AdminAuth(), GetLogPerformance)
	return engine, adminToken, userToken, base
}

func logPerformanceRequest(engine http.Handler, token, query string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/log/performance?"+query, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestLogPerformanceRequiresAdmin(t *testing.T) {
	engine, adminToken, userToken, base := setupLogPerformanceControllerTest(t)
	query := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", base, base+3599)

	forbidden := logPerformanceRequest(engine, userToken, query)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.NotContains(t, forbidden.Body.String(), `"requests"`)

	allowed := logPerformanceRequest(engine, adminToken, query)
	require.Equal(t, http.StatusOK, allowed.Code)
	var body struct {
		Success bool                       `json:"success"`
		Data    model.LogPerformanceResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(allowed.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "minute", body.Data.Granularity)
	assert.Equal(t, constant.ErrorLogEnabled, body.Data.ErrorLogEnabled)
	assert.EqualValues(t, 5, body.Data.Summary.Requests)
	assert.EqualValues(t, 4, body.Data.Summary.Successes)
	assert.EqualValues(t, 1, body.Data.Summary.Failures)
	require.NotNil(t, body.Data.Summary.SuccessRate)
	assert.InDelta(t, 80, *body.Data.Summary.SuccessRate, 1e-9)

	require.Len(t, body.Data.Channels, 1)
	assert.Equal(t, 7, body.Data.Channels[0].ChannelId)
	assert.Equal(t, "primary", body.Data.Channels[0].ChannelName)
	require.Len(t, body.Data.SlowRequests, 4)
	assert.Equal(t, "primary", body.Data.SlowRequests[0].ChannelName)
	require.Len(t, body.Data.Errors, 1)
	assert.Equal(t, "primary", body.Data.Errors[0].ChannelName)
	assert.Equal(t, "upstream failed", body.Data.Errors[0].Content)
}

func TestLogPerformanceParsesFilters(t *testing.T) {
	engine, adminToken, _, base := setupLogPerformanceControllerTest(t)

	cases := []struct {
		name  string
		query string
		want  int64
	}{
		{"model_name", "model_name=gpt-a", 3},
		{"channel", "channel=7", 5},
		{"channel hash prefix", "channel=%237", 5},
		{"group", "group=vip", 3},
		{"token_name", "token_name=tok-b", 2},
		{"username", "username=alice", 3},
		{"is_stream", "is_stream=true", 2},
		{"is_stream false", "is_stream=false", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d&%s", base, base+3599, tc.query)
			response := logPerformanceRequest(engine, adminToken, query)
			require.Equal(t, http.StatusOK, response.Code)
			var body struct {
				Success bool                       `json:"success"`
				Data    model.LogPerformanceResult `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			require.True(t, body.Success, response.Body.String())
			assert.EqualValues(t, tc.want, body.Data.Summary.Requests)
		})
	}

	defaulted := logPerformanceRequest(engine, adminToken, "")
	require.Equal(t, http.StatusOK, defaulted.Code)
	var body struct {
		Data model.LogPerformanceResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(defaulted.Body.Bytes(), &body))
	assert.Equal(t, "hour", body.Data.Granularity)
}

func TestLogPerformanceRejectsInvalidRanges(t *testing.T) {
	engine, adminToken, _, base := setupLogPerformanceControllerTest(t)

	for _, tc := range []struct {
		name  string
		query string
	}{
		{"31 days", fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", base, base+31*24*3600)},
		{"end before start", fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", base+3600, base)},
		{"bad granularity", fmt.Sprintf("start_timestamp=%d&end_timestamp=%d&granularity=week", base, base+3599)},
		{"too many points", fmt.Sprintf("start_timestamp=%d&end_timestamp=%d&granularity=minute", base, base+24*3600)},
		{"invalid channel", fmt.Sprintf("start_timestamp=%d&end_timestamp=%d&channel=abc", base, base+3599)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := logPerformanceRequest(engine, adminToken, tc.query)
			require.Equal(t, http.StatusOK, response.Code)
			var body struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.False(t, body.Success)
			assert.NotEmpty(t, body.Message)
		})
	}
}
