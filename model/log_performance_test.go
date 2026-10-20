package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupLogPerformanceTestDB gives each test its own in-memory SQLite log
// database and restores the package globals afterwards.
func setupLogPerformanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemory := common.MemoryCacheEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	initCol()

	dsn := fmt.Sprintf("file:log_perf_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.MemoryCacheEnabled = previousMemory
		initCol()
	})
	return db
}

func createLogPerformanceLogs(t *testing.T, db *gorm.DB, logs ...*Log) {
	t.Helper()
	for _, log := range logs {
		require.NoError(t, db.Create(log).Error)
	}
}

func alignedLogPerfBase(alignment int64) int64 {
	const seed = int64(1_700_000_000)
	return seed - seed%alignment
}

func TestLogPerfCacheTokens(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	createLogPerformanceLogs(t, db,
		&Log{CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-4o", PromptTokens: 1000, Other: `{"cache_tokens": 400}`},
		&Log{CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-4o", PromptTokens: 500, Other: `{"cache_tokens": 100}`},
		&Log{CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-4o", PromptTokens: 200, Other: `{}`},
		&Log{CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-4o", PromptTokens: 100, Other: ``},
	)

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	assert.EqualValues(t, 500, result.Summary.CacheTokens)
	assert.EqualValues(t, 1800, result.Summary.PromptTokens)
}

func TestLogPerfSuccessRate(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	for range 90 {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-4o",
			UseTime: 2, CompletionTokens: 20,
		})
	}
	for range 10 {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeError, ModelName: "gpt-4o",
		})
	}

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	assert.EqualValues(t, 100, result.Summary.Requests)
	assert.EqualValues(t, 90, result.Summary.Successes)
	assert.EqualValues(t, 10, result.Summary.Failures)
	require.NotNil(t, result.Summary.SuccessRate)
	assert.InDelta(t, 90, *result.Summary.SuccessRate, 1e-9)

	empty, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599, ModelName: "absent"}, 3600)
	require.NoError(t, err)
	assert.EqualValues(t, 0, empty.Summary.Requests)
	assert.Nil(t, empty.Summary.SuccessRate)
	assert.Nil(t, empty.Summary.AvgTps)
	assert.Nil(t, empty.Summary.MaxTps)
	assert.Nil(t, empty.Summary.MinTps)
	assert.Nil(t, empty.Summary.AvgLatencyMs)
	assert.Nil(t, empty.Summary.P95LatencyMs)
	assert.Empty(t, empty.Series)
}

func TestLogPerfTPSAndLatency(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	createLogPerformanceLogs(t, db,
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 4, CompletionTokens: 200, PromptTokens: 10},
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 2, CompletionTokens: 60},
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 5, CompletionTokens: 50},
		// Excluded from TPS: zero use time, zero completion tokens, and errors.
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 0, CompletionTokens: 100},
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 3, CompletionTokens: 0},
		&Log{CreatedAt: base, Type: LogTypeError, UseTime: 0, CompletionTokens: 1000},
	)

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	require.NotNil(t, result.Summary.AvgTps)
	require.NotNil(t, result.Summary.MaxTps)
	require.NotNil(t, result.Summary.MinTps)
	assert.InDelta(t, 30, *result.Summary.AvgTps, 1e-9)
	assert.InDelta(t, 50, *result.Summary.MaxTps, 1e-9)
	assert.InDelta(t, 10, *result.Summary.MinTps, 1e-9)

	require.NotNil(t, result.Summary.AvgLatencyMs)
	assert.InDelta(t, 3500, *result.Summary.AvgLatencyMs, 1e-6)
	require.NotNil(t, result.Summary.P95LatencyMs)
	assert.InDelta(t, 5000, *result.Summary.P95LatencyMs, 1e-9)

	assert.EqualValues(t, 10, result.Summary.PromptTokens)
	assert.EqualValues(t, 1410, result.Summary.CompletionTokens)
	assert.EqualValues(t, 1420, result.Summary.TotalTokens)

	require.Len(t, result.Series, 1)
	assert.Equal(t, base, result.Series[0].Ts)
	assert.InDelta(t, 30, *result.Series[0].AvgTps, 1e-9)
}

func TestLogPerfP95SingleBucket(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	for i := 1; i <= 100; i++ {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeConsume, UseTime: i, CompletionTokens: i,
		})
	}

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	require.NotNil(t, result.Summary.P95LatencyMs)
	assert.InDelta(t, 95000, *result.Summary.P95LatencyMs, 1e-9)
	require.Len(t, result.Series, 1)
	require.NotNil(t, result.Series[0].P95LatencyMs)
	assert.InDelta(t, 95000, *result.Series[0].P95LatencyMs, 1e-9)
}

func TestLogPerfP95AcrossBuckets(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	for i := 1; i <= 50; i++ {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeConsume, UseTime: i, CompletionTokens: i,
		})
	}
	for i := 51; i <= 100; i++ {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base + 3600, Type: LogTypeConsume, UseTime: i, CompletionTokens: i,
		})
	}

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 7199}, 3600)
	require.NoError(t, err)
	require.NotNil(t, result.Summary.P95LatencyMs)
	assert.InDelta(t, 95000, *result.Summary.P95LatencyMs, 1e-9)

	require.Len(t, result.Series, 2)
	assert.Equal(t, base, result.Series[0].Ts)
	assert.Equal(t, base+3600, result.Series[1].Ts)
	require.NotNil(t, result.Series[0].P95LatencyMs)
	require.NotNil(t, result.Series[1].P95LatencyMs)
	assert.InDelta(t, 48000, *result.Series[0].P95LatencyMs, 1e-9)
	assert.InDelta(t, 98000, *result.Series[1].P95LatencyMs, 1e-9)
}

func TestLogPerfBucketing(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(60)

	createLogPerformanceLogs(t, db,
		&Log{CreatedAt: base, Type: LogTypeConsume, UseTime: 1},
		&Log{CreatedAt: base + 59, Type: LogTypeConsume, UseTime: 1},
		&Log{CreatedAt: base + 60, Type: LogTypeConsume, UseTime: 1},
		&Log{CreatedAt: base + 119, Type: LogTypeConsume, UseTime: 1},
	)

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 120}, 60)
	require.NoError(t, err)
	require.Len(t, result.Series, 2)
	assert.Equal(t, base, result.Series[0].Ts)
	assert.EqualValues(t, 2, result.Series[0].Requests)
	assert.Equal(t, base+60, result.Series[1].Ts)
	assert.EqualValues(t, 2, result.Series[1].Requests)
}

func TestLogPerfFilters(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	for range 2 {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-a", ChannelId: 1,
			Group: "vip", TokenName: "tok-a", Username: "alice", IsStream: true,
			UseTime: 2, CompletionTokens: 20,
		})
	}
	for range 2 {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base, Type: LogTypeConsume, ModelName: "gpt-b", ChannelId: 2,
			Group: "default", TokenName: "tok-b", Username: "bob", IsStream: false,
			UseTime: 2, CompletionTokens: 20,
		})
	}

	streamTrue := true
	streamFalse := false
	cases := []struct {
		name   string
		filter LogPerformanceFilter
		want   int64
	}{
		{"all", LogPerformanceFilter{}, 4},
		{"model_name", LogPerformanceFilter{ModelName: "gpt-a"}, 2},
		{"channel", LogPerformanceFilter{Channel: 2}, 2},
		{"group", LogPerformanceFilter{Group: "vip"}, 2},
		{"token_name", LogPerformanceFilter{TokenName: "tok-b"}, 2},
		{"username", LogPerformanceFilter{Username: "alice"}, 2},
		{"is_stream true", LogPerformanceFilter{IsStream: &streamTrue}, 2},
		{"is_stream false", LogPerformanceFilter{IsStream: &streamFalse}, 2},
		{"combined", LogPerformanceFilter{ModelName: "gpt-a", Channel: 2}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.filter.StartTimestamp = base
			tc.filter.EndTimestamp = base + 3599
			result, err := QueryLogPerformance(tc.filter, 3600)
			require.NoError(t, err)
			assert.EqualValues(t, tc.want, result.Summary.Requests)
		})
	}

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	require.Len(t, result.Groups, 2)
	require.Len(t, result.Models, 2)
	require.Len(t, result.Channels, 2)
	assert.Equal(t, "default", result.Groups[0].Group)
	assert.Equal(t, "gpt-a", result.Models[0].ModelName)
	assert.Equal(t, 1, result.Channels[0].ChannelId)
}

func TestLogPerfGranularity(t *testing.T) {
	cases := []struct {
		name      string
		start     int64
		end       int64
		requested string
		want      string
		wantSecs  int64
		wantErr   error
	}{
		{"24h auto is hourly", 0, 24 * 3600, "auto", "hour", 3600, nil},
		{"30d auto is daily", 0, 30 * 24 * 3600, "auto", "day", 86400, nil},
		{"31d rejected", 0, 31 * 24 * 3600, "auto", "", 0, ErrLogPerformanceRange},
		{"invalid granularity", 0, 3600, "week", "", 0, ErrLogPerformanceGranularity},
		{"too many points", 0, 24 * 3600, "minute", "", 0, ErrLogPerformanceTooManyPoints},
		{"end before start", 100, 50, "auto", "", 0, ErrLogPerformanceTimeOrder},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, seconds, err := ResolveLogPerformanceGranularity(tc.start, tc.end, tc.requested)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, name)
			assert.Equal(t, tc.wantSecs, seconds)
		})
	}
}

func TestLogPerfSlowRequestsAndErrors(t *testing.T) {
	db := setupLogPerformanceTestDB(t)
	base := alignedLogPerfBase(3600)

	for i := 1; i <= 12; i++ {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base + int64(i), Type: LogTypeConsume, ModelName: "gpt-4o",
			ChannelId: 7, UseTime: i, CompletionTokens: i, RequestId: fmt.Sprintf("slow-%d", i),
		})
	}
	for i := 1; i <= 12; i++ {
		createLogPerformanceLogs(t, db, &Log{
			CreatedAt: base + int64(i), Type: LogTypeError, ModelName: "gpt-4o",
			ChannelId: 7, Content: fmt.Sprintf("error-%d", i), RequestId: fmt.Sprintf("err-%d", i),
		})
	}

	result, err := QueryLogPerformance(LogPerformanceFilter{StartTimestamp: base, EndTimestamp: base + 3599}, 3600)
	require.NoError(t, err)
	require.Len(t, result.SlowRequests, 10)
	assert.Equal(t, 12, result.SlowRequests[0].UseTime)
	assert.Equal(t, "slow-12", result.SlowRequests[0].RequestId)
	assert.Equal(t, 3, result.SlowRequests[9].UseTime)

	require.Len(t, result.Errors, 10)
	assert.Equal(t, "error-12", result.Errors[0].Content)
	assert.Equal(t, "err-12", result.Errors[0].RequestId)
	assert.Equal(t, base+12, result.Errors[0].CreatedAt)
}
