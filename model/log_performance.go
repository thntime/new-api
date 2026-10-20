package model

import (
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	logPerformanceMaxRangeSeconds int64 = 30 * 24 * 60 * 60
	logPerformanceMaxBuckets            = 120
	// Auto targets at most 60 points so a 24h range resolves to hourly buckets
	// instead of the 96 points a 15-minute granularity would produce.
	logPerformanceAutoBucketLimit   = 60
	logPerformanceMaxUseTimeSeconds = 600
	logPerformanceBreakdownLimit    = 50
	logPerformanceAnomalyLimit      = 10
)

var (
	ErrLogPerformanceTimeOrder     = errors.New("The end time must not be before the start time")
	ErrLogPerformanceRange         = errors.New("The time range cannot exceed 30 days")
	ErrLogPerformanceGranularity   = errors.New("Invalid granularity")
	ErrLogPerformanceTooManyPoints = errors.New("The selected granularity produces too many data points")
)

type logPerformanceGranularity struct {
	name    string
	seconds int64
}

var logPerformanceGranularities = []logPerformanceGranularity{
	{"minute", 60},
	{"5min", 300},
	{"15min", 900},
	{"hour", 3600},
	{"day", 86400},
}

// ResolveLogPerformanceGranularity validates the requested range and returns
// the effective granularity name and bucket size in seconds.
func ResolveLogPerformanceGranularity(startTimestamp int64, endTimestamp int64, requested string) (string, int64, error) {
	if endTimestamp < startTimestamp {
		return "", 0, ErrLogPerformanceTimeOrder
	}
	span := endTimestamp - startTimestamp
	if span > logPerformanceMaxRangeSeconds {
		return "", 0, ErrLogPerformanceRange
	}
	bucketCount := func(seconds int64) int64 {
		return (span + seconds - 1) / seconds
	}
	if requested == "" || requested == "auto" {
		for _, granularity := range logPerformanceGranularities {
			if bucketCount(granularity.seconds) <= logPerformanceAutoBucketLimit {
				return granularity.name, granularity.seconds, nil
			}
		}
		last := logPerformanceGranularities[len(logPerformanceGranularities)-1]
		return last.name, last.seconds, nil
	}
	for _, granularity := range logPerformanceGranularities {
		if granularity.name != requested {
			continue
		}
		if bucketCount(granularity.seconds) > logPerformanceMaxBuckets {
			return "", 0, ErrLogPerformanceTooManyPoints
		}
		return granularity.name, granularity.seconds, nil
	}
	return "", 0, ErrLogPerformanceGranularity
}

// LogPerformanceFilter is the single filter snapshot shared by every block of
// the performance-health response.
type LogPerformanceFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Channel        int
	Group          string
	TokenName      string
	Username       string
	IsStream       *bool
}

// logPerformanceGroupColumn returns the dialect-quoted group column. A test
// that swaps database types without opening a database still gets a usable
// value instead of an empty column name.
func logPerformanceGroupColumn() string {
	if logGroupCol == "" {
		initCol()
	}
	return logGroupCol
}

func (f LogPerformanceFilter) apply(tx *gorm.DB) (*gorm.DB, error) {
	tx = tx.Table("logs").Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError})
	if f.StartTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", f.StartTimestamp)
	}
	if f.EndTimestamp != 0 {
		tx = tx.Where("logs.created_at < ?", f.EndTimestamp)
	}
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", f.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", f.Username); err != nil {
		return nil, err
	}
	if f.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", f.TokenName)
	}
	if f.Channel != 0 {
		tx = tx.Where("logs.channel_id = ?", f.Channel)
	}
	if f.Group != "" {
		tx = tx.Where("logs."+logPerformanceGroupColumn()+" = ?", f.Group)
	}
	if f.IsStream != nil {
		tx = tx.Where("logs.is_stream = ?", *f.IsStream)
	}
	return tx, nil
}

// LogPerformanceMetrics is one row of aggregate metrics. Nil pointers mean the
// metric cannot be computed (empty denominator or no samples).
type LogPerformanceMetrics struct {
	Requests         int64    `json:"requests"`
	Successes        int64    `json:"successes"`
	Failures         int64    `json:"failures"`
	SuccessRate      *float64 `json:"success_rate"`
	AvgTps           *float64 `json:"avg_tps"`
	MaxTps           *float64 `json:"max_tps"`
	MinTps           *float64 `json:"min_tps"`
	AvgLatencyMs     *float64 `json:"avg_latency_ms"`
	P95LatencyMs     *float64 `json:"p95_latency_ms"`
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	TotalTokens      int64    `json:"total_tokens"`
	CacheTokens      int64    `json:"cache_tokens"`
}

type LogPerformanceSeriesPoint struct {
	Ts int64 `json:"ts"`
	LogPerformanceMetrics
}

type LogPerformanceGroup struct {
	Group string `json:"group"`
	LogPerformanceMetrics
}

type LogPerformanceModel struct {
	ModelName string `json:"model_name"`
	LogPerformanceMetrics
}

type LogPerformanceChannel struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	LogPerformanceMetrics
}

type LogPerformanceSlowRequest struct {
	CreatedAt        int64  `json:"created_at" gorm:"column:created_at"`
	ModelName        string `json:"model_name" gorm:"column:model_name"`
	ChannelId        int    `json:"channel_id" gorm:"column:channel_id"`
	ChannelName      string `json:"channel_name" gorm:"-"`
	TokenName        string `json:"token_name" gorm:"column:token_name"`
	UseTime          int    `json:"use_time" gorm:"column:use_time"`
	CompletionTokens int    `json:"completion_tokens" gorm:"column:completion_tokens"`
	RequestId        string `json:"request_id" gorm:"column:request_id"`
}

type LogPerformanceError struct {
	CreatedAt   int64  `json:"created_at" gorm:"column:created_at"`
	ModelName   string `json:"model_name" gorm:"column:model_name"`
	ChannelId   int    `json:"channel_id" gorm:"column:channel_id"`
	ChannelName string `json:"channel_name" gorm:"-"`
	TokenName   string `json:"token_name" gorm:"column:token_name"`
	Content     string `json:"content" gorm:"column:content"`
	RequestId   string `json:"request_id" gorm:"column:request_id"`
}

type LogPerformanceResult struct {
	Granularity     string                      `json:"granularity"`
	ErrorLogEnabled bool                        `json:"error_log_enabled"`
	Summary         LogPerformanceMetrics       `json:"summary"`
	Series          []LogPerformanceSeriesPoint `json:"series"`
	Groups          []LogPerformanceGroup       `json:"groups"`
	Models          []LogPerformanceModel       `json:"models"`
	Channels        []LogPerformanceChannel     `json:"channels"`
	SlowRequests    []LogPerformanceSlowRequest `json:"slow_requests"`
	Errors          []LogPerformanceError       `json:"errors"`
}

type logPerformanceMetricsRow struct {
	Requests         int64    `gorm:"column:requests"`
	Successes        int64    `gorm:"column:successes"`
	Failures         int64    `gorm:"column:failures"`
	AvgUseTime       *float64 `gorm:"column:avg_use_time"`
	AvgTps           *float64 `gorm:"column:avg_tps"`
	MaxTps           *float64 `gorm:"column:max_tps"`
	MinTps           *float64 `gorm:"column:min_tps"`
	PromptTokens     int64    `gorm:"column:prompt_tokens"`
	CompletionTokens int64    `gorm:"column:completion_tokens"`
	CacheTokens      int64    `gorm:"column:cache_tokens"`
}

type logPerformanceBucketRow struct {
	Bucket  int64                    `gorm:"column:bucket"`
	Metrics logPerformanceMetricsRow `gorm:"embedded"`
}

type logPerformanceGroupRow struct {
	GroupKey string                   `gorm:"column:group_key"`
	Metrics  logPerformanceMetricsRow `gorm:"embedded"`
}

type logPerformanceChannelRow struct {
	ChannelId int                      `gorm:"column:channel_id"`
	Metrics   logPerformanceMetricsRow `gorm:"embedded"`
}

type logPerformanceHistogramRow struct {
	Bucket int64 `gorm:"column:bucket"`
	T      int64 `gorm:"column:t"`
	Count  int64 `gorm:"column:cnt"`
}

const logPerformanceMetricsSelectTemplate = `
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN logs.type = %d THEN 1 ELSE 0 END), 0) AS successes,
	COALESCE(SUM(CASE WHEN logs.type = %d THEN 1 ELSE 0 END), 0) AS failures,
	AVG(CASE WHEN logs.use_time > 0 THEN logs.use_time END) AS avg_use_time,
	AVG(CASE WHEN logs.use_time > 0 AND logs.completion_tokens > 0
		THEN 1.0 * logs.completion_tokens / logs.use_time END) AS avg_tps,
	MAX(CASE WHEN logs.use_time > 0 AND logs.completion_tokens > 0
		THEN 1.0 * logs.completion_tokens / logs.use_time END) AS max_tps,
	MIN(CASE WHEN logs.use_time > 0 AND logs.completion_tokens > 0
		THEN 1.0 * logs.completion_tokens / logs.use_time END) AS min_tps,
	COALESCE(SUM(logs.prompt_tokens), 0) AS prompt_tokens,
	COALESCE(SUM(logs.completion_tokens), 0) AS completion_tokens`

// logPerformanceMetricsSelect inlines its constants instead of using
// placeholders so every log-database driver binds the same way.
func logPerformanceMetricsSelect() string {
	return fmt.Sprintf(logPerformanceMetricsSelectTemplate, LogTypeConsume, LogTypeError)
}

// logPerformanceCacheTokensExpr extracts cached prompt tokens from the log's
// JSON metadata. Each log database exposes a different JSON function, and the
// guard keeps rows with empty or malformed metadata from failing the query.
func logPerformanceCacheTokensExpr() string {
	switch common.LogDatabaseType() {
	case common.DatabaseTypeMySQL:
		return "COALESCE(SUM(CASE WHEN JSON_VALID(logs.other) THEN CAST(JSON_UNQUOTE(JSON_EXTRACT(logs.other, '$.cache_tokens')) AS SIGNED) ELSE 0 END), 0) AS cache_tokens"
	case common.DatabaseTypePostgreSQL:
		return "COALESCE(SUM(CASE WHEN logs.other LIKE '{%' THEN COALESCE((logs.other::jsonb->>'cache_tokens')::numeric::bigint, 0) ELSE 0 END), 0) AS cache_tokens"
	case common.DatabaseTypeClickHouse:
		return "COALESCE(SUM(JSONExtractInt(logs.other, 'cache_tokens')), 0) AS cache_tokens"
	default:
		return "COALESCE(SUM(CASE WHEN json_valid(logs.other) THEN json_extract(logs.other, '$.cache_tokens') ELSE 0 END), 0) AS cache_tokens"
	}
}

func (r logPerformanceMetricsRow) metrics() LogPerformanceMetrics {
	metrics := LogPerformanceMetrics{
		Requests:         r.Requests,
		Successes:        r.Successes,
		Failures:         r.Failures,
		AvgTps:           r.AvgTps,
		MaxTps:           r.MaxTps,
		MinTps:           r.MinTps,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		TotalTokens:      r.PromptTokens + r.CompletionTokens,
		CacheTokens:      r.CacheTokens,
	}
	if attempts := r.Successes + r.Failures; attempts > 0 {
		rate := float64(r.Successes) / float64(attempts) * 100
		metrics.SuccessRate = &rate
	}
	if r.AvgUseTime != nil {
		latency := *r.AvgUseTime * 1000
		metrics.AvgLatencyMs = &latency
	}
	return metrics
}

// logPerformanceP95 returns the nearest-rank 95th percentile of the latency
// histogram, in milliseconds. Rows may be unsorted; the function sorts a copy.
func logPerformanceP95(rows []logPerformanceHistogramRow) *float64 {
	var total int64
	for _, row := range rows {
		total += row.Count
	}
	if total == 0 {
		return nil
	}
	sorted := append([]logPerformanceHistogramRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].T < sorted[j].T })
	target := (total*95 + 99) / 100
	var cumulative int64
	for _, row := range sorted {
		cumulative += row.Count
		if cumulative >= target {
			value := float64(row.T) * 1000
			return &value
		}
	}
	value := float64(sorted[len(sorted)-1].T) * 1000
	return &value
}

func logPerformanceBucketExpr(granularitySeconds int64) string {
	return fmt.Sprintf("logs.created_at - (logs.created_at %% %d)", granularitySeconds)
}

func logPerformanceHistogramExpr(granularitySeconds int64) string {
	return fmt.Sprintf(
		"logs.created_at - (logs.created_at %% %d) AS bucket, CASE WHEN logs.use_time > %d THEN %d ELSE logs.use_time END AS t, COUNT(*) AS cnt",
		granularitySeconds, logPerformanceMaxUseTimeSeconds, logPerformanceMaxUseTimeSeconds,
	)
}

// QueryLogPerformance runs one filter snapshot through every aggregation the
// performance-health page needs.
func QueryLogPerformance(filter LogPerformanceFilter, granularitySeconds int64) (*LogPerformanceResult, error) {
	if granularitySeconds <= 0 {
		return nil, ErrLogPerformanceGranularity
	}
	result := &LogPerformanceResult{
		Series:       []LogPerformanceSeriesPoint{},
		Groups:       []LogPerformanceGroup{},
		Models:       []LogPerformanceModel{},
		Channels:     []LogPerformanceChannel{},
		SlowRequests: []LogPerformanceSlowRequest{},
		Errors:       []LogPerformanceError{},
	}

	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	var summary logPerformanceMetricsRow
	if err := tx.Select(logPerformanceMetricsSelect() + ", " + logPerformanceCacheTokensExpr()).Scan(&summary).Error; err != nil {
		return nil, err
	}
	result.Summary = summary.metrics()

	tx, err = filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	var bucketRows []logPerformanceBucketRow
	if err := tx.Select(logPerformanceBucketExpr(granularitySeconds) + " AS bucket, " + logPerformanceMetricsSelect()).
		Group("bucket").Order("bucket ASC").Scan(&bucketRows).Error; err != nil {
		return nil, err
	}

	tx, err = filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	var histogramRows []logPerformanceHistogramRow
	if err := tx.Where("logs.use_time > 0").
		Select(logPerformanceHistogramExpr(granularitySeconds)).
		Group("bucket, t").Order("bucket ASC, t ASC").Scan(&histogramRows).Error; err != nil {
		return nil, err
	}
	result.Summary.P95LatencyMs = logPerformanceP95(histogramRows)

	histogramByBucket := make(map[int64][]logPerformanceHistogramRow, len(bucketRows))
	for _, row := range histogramRows {
		histogramByBucket[row.Bucket] = append(histogramByBucket[row.Bucket], row)
	}
	for _, row := range bucketRows {
		metrics := row.Metrics.metrics()
		metrics.P95LatencyMs = logPerformanceP95(histogramByBucket[row.Bucket])
		result.Series = append(result.Series, LogPerformanceSeriesPoint{Ts: row.Bucket, LogPerformanceMetrics: metrics})
	}

	groupRows, err := queryLogPerformanceBreakdown(filter, logPerformanceGroupColumn())
	if err != nil {
		return nil, err
	}
	for _, row := range groupRows {
		result.Groups = append(result.Groups, LogPerformanceGroup{Group: row.GroupKey, LogPerformanceMetrics: row.Metrics.metrics()})
	}

	modelRows, err := queryLogPerformanceBreakdown(filter, "model_name")
	if err != nil {
		return nil, err
	}
	for _, row := range modelRows {
		result.Models = append(result.Models, LogPerformanceModel{ModelName: row.GroupKey, LogPerformanceMetrics: row.Metrics.metrics()})
	}

	channelRows, err := queryLogPerformanceChannels(filter)
	if err != nil {
		return nil, err
	}
	for _, row := range channelRows {
		result.Channels = append(result.Channels, LogPerformanceChannel{ChannelId: row.ChannelId, LogPerformanceMetrics: row.Metrics.metrics()})
	}

	tx, err = filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	if err := tx.Where("logs.type = ?", LogTypeConsume).Where("logs.use_time > 0").
		Select("logs.created_at, logs.model_name, logs.channel_id, logs.token_name, logs.use_time, logs.completion_tokens, logs.request_id").
		Order("logs.use_time DESC").Limit(logPerformanceAnomalyLimit).
		Find(&result.SlowRequests).Error; err != nil {
		return nil, err
	}

	tx, err = filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	if err := tx.Where("logs.type = ?", LogTypeError).
		Select("logs.created_at, logs.model_name, logs.channel_id, logs.token_name, logs.content, logs.request_id").
		Order("logs.created_at DESC").Limit(logPerformanceAnomalyLimit).
		Find(&result.Errors).Error; err != nil {
		return nil, err
	}

	return result, nil
}

func queryLogPerformanceBreakdown(filter LogPerformanceFilter, column string) ([]logPerformanceGroupRow, error) {
	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	var rows []logPerformanceGroupRow
	if err := tx.Select("logs." + column + " AS group_key, " + logPerformanceMetricsSelect()).
		Group("logs." + column).Order("requests DESC, group_key ASC").Limit(logPerformanceBreakdownLimit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func queryLogPerformanceChannels(filter LogPerformanceFilter) ([]logPerformanceChannelRow, error) {
	tx, err := filter.apply(LOG_DB)
	if err != nil {
		return nil, err
	}
	var rows []logPerformanceChannelRow
	if err := tx.Select("logs.channel_id AS channel_id, " + logPerformanceMetricsSelect()).
		Group("logs.channel_id").Order("requests DESC, channel_id ASC").Limit(logPerformanceBreakdownLimit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
