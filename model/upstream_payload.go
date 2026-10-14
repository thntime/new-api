package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpstreamPayload keeps the raw bodies the gateway actually sent upstream and
// received back, keyed by the usage-log request_id.
type UpstreamPayload struct {
	Id                int64  `gorm:"primaryKey"`
	RequestId         string `gorm:"type:varchar(64);uniqueIndex:idx_upstream_payload_request_id;not null"`
	CreatedAt         int64  `gorm:"index"`
	RequestBody       LongText
	ResponseBody      LongText
	RequestTruncated  bool
	ResponseTruncated bool
}

// MigrateUpstreamPayloads creates the payload table. SQLite/MySQL/PostgreSQL use
// AutoMigrate; a ClickHouse log store needs an explicit MergeTree table.
func MigrateUpstreamPayloads() error {
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.AutoMigrate(&UpstreamPayload{})
	}
	return LOG_DB.Exec(clickHouseUpstreamPayloadCreateTableSQL(clickHouseLogTTLDays())).Error
}

func clickHouseUpstreamPayloadCreateTableSQL(ttlDays int) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS upstream_payloads (
	id Int64, request_id String, created_at Int64,
	request_body String, response_body String,
	request_truncated Bool, response_truncated Bool
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)%s`, clickHouseLogTTLClause(ttlDays))
}

// clickHouseUpstreamPayloadTTLSQL 根据当前建表语句与期望 TTL 天数决定要执行的语句：
// 正数天数改写 TTL，置零且表上仍有 TTL 时移除，其余情况返回空串表示无需执行。
// 与 syncClickHouseLogTTL 的语义保持一致，避免日志与报文 TTL 不一致导致报文被静默删除。
func clickHouseUpstreamPayloadTTLSQL(createTableSQL string, ttlDays int) string {
	if expression := clickHouseLogTTLExpression(ttlDays); expression != "" {
		return "ALTER TABLE upstream_payloads MODIFY TTL " + expression
	}
	if clickHouseCreateTableHasTTL(createTableSQL) {
		return "ALTER TABLE upstream_payloads REMOVE TTL"
	}
	return ""
}

// syncClickHouseUpstreamPayloadTTL refreshes the table TTL on restart, mirroring
// syncClickHouseLogTTL (including the REMOVE TTL path when the TTL is switched off).
func syncClickHouseUpstreamPayloadTTL(ttlDays int) error {
	var createTableSQL string
	if err := LOG_DB.Raw("SHOW CREATE TABLE upstream_payloads").Scan(&createTableSQL).Error; err != nil {
		return err
	}
	sql := clickHouseUpstreamPayloadTTLSQL(createTableSQL, ttlDays)
	if sql == "" {
		return nil
	}
	return LOG_DB.Exec(sql).Error
}

// SaveUpstreamPayload keeps one row per request_id. ClickHouse has no upsert, so
// it appends and readers take the newest created_at instead.
func SaveUpstreamPayload(p *UpstreamPayload) error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.Create(p).Error
	}
	return LOG_DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "request_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"created_at", "request_body", "response_body", "request_truncated", "response_truncated"}),
	}).Create(p).Error
}

// GetUpstreamPayloadByRequestId returns (nil, nil) when there is no record.
func GetUpstreamPayloadByRequestId(requestId string) (*UpstreamPayload, error) {
	if requestId == "" {
		return nil, nil
	}
	var payload UpstreamPayload
	query := LOG_DB.Where("request_id = ?", requestId)
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse leaves id at its zero value because the table declares no
		// DEFAULT and GORM omits the primary key from INSERT, so ordering by id
		// would not break created_at ties; created_at is the only useful sort key.
		query = query.Order("created_at DESC")
	}
	err := query.Take(&payload).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &payload, nil
}

func CountOldUpstreamPayload(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	if err := LOG_DB.WithContext(ctx).Model(&UpstreamPayload{}).
		Where("created_at < ?", targetTimestamp).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func DeleteOldUpstreamPayloadBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if nil != ctx.Err() {
		return 0, ctx.Err()
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse DELETE is a heavy synchronous mutation; remove every
		// matching row in one pass regardless of limit, mirroring DeleteOldLogBatch.
		total, err := CountOldUpstreamPayload(ctx, targetTimestamp)
		if err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE upstream_payloads DELETE WHERE created_at < ? SETTINGS mutations_sync = 1",
			targetTimestamp,
		).Error; err != nil {
			return 0, err
		}
		return total, nil
	}
	result := LOG_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&UpstreamPayload{})
	if nil != result.Error {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
