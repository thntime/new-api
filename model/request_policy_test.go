package model

import (
	"errors"
	"maps"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// descent 必须同时被全局与规则级白名单接受并原样保留，未知取值仍被拒（fail-closed）。
func TestRequestPolicyAcceptsDescentSessionMode(t *testing.T) {
	snapshot, err := BuildRequestPolicy(map[string]string{"channel_affinity_setting.session_mode": "descent"})
	require.NoError(t, err)
	require.Equal(t, "descent", snapshot.Affinity.SessionMode)

	rules := `[{"name":"ring","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":"descent"}]`
	snapshot, err = BuildRequestPolicy(map[string]string{
		"channel_affinity_setting.session_mode": "prefer",
		"channel_affinity_setting.rules":        rules,
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Affinity.Rules, 1)
	require.Equal(t, "descent", snapshot.Affinity.Rules[0].SessionMode)

	_, err = BuildRequestPolicy(map[string]string{"channel_affinity_setting.session_mode": "unknown"})
	require.Error(t, err)
	_, err = BuildRequestPolicy(map[string]string{
		"channel_affinity_setting.session_mode": "prefer",
		"channel_affinity_setting.rules":        `[{"name":"ring","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":"unknown"}]`,
	})
	require.Error(t, err)
}

func TestRequestPolicyDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "policy_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType, previousSnapshot := DB, common.MainDatabaseType(), CurrentRequestPolicy()
			common.OptionMapRWMutex.Lock()
			previousOptions := maps.Clone(common.OptionMap)
			common.OptionMap = maps.Clone(previousSnapshot.Options)
			common.OptionMapRWMutex.Unlock()
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Option{}))
				for k, v := range previousSnapshot.Options {
					require.NoError(t, updateOptionMap(k, v))
				}
				requestPolicySnapshot.Store(previousSnapshot)
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Option{}))
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			rules := `[{"name":"session","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"ttl_seconds":0,"skip_retry_on_failure":false,"include_using_group":false,"param_override_template":{"temperature":0},"future_field":{"enabled":false}}]`
			require.NoError(t, UpdateRequestPolicyOptions(map[string]string{"RetryTimes": "2", "channel_affinity_setting.rules": rules}))
			assert.Equal(t, 2, CurrentRequestPolicy().RetryTimes)
			assert.Equal(t, 2, common.RetryTimes, "the runtime global follows the same write")
			require.NoError(t, UpdateOption("AutomaticRetryStatusCodes", "429,500-503"))
			assert.Equal(t, "429,500-503", CurrentRequestPolicy().Options["AutomaticRetryStatusCodes"])
			assert.Equal(t, "429,500-503", operation_setting.AutomaticRetryStatusCodesToString())
			assert.Error(t, UpdateRequestPolicyOptions(map[string]string{"request_policy_setting.mode": "off"}), "the removed mode switch is not a policy option")
			assert.False(t, CurrentRequestPolicy().Affinity.Rules[0].SkipRetryOnFailure)
			assert.Equal(t, rules, CurrentRequestPolicy().Options["channel_affinity_setting.rules"])
			loadOptionsFromDatabase()
			loadOptionsFromDatabase()
			assert.Equal(t, rules, CurrentRequestPolicy().Options["channel_affinity_setting.rules"], "legacy rule JSON survives reloads without dropping extension fields")
			require.NoError(t, UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.session_mode": "strict"}))
			loadOptionsFromDatabase()
			loadOptionsFromDatabase()
			assert.Equal(t, "strict", CurrentRequestPolicy().Affinity.SessionMode)
			assert.Equal(t, rules, CurrentRequestPolicy().Options["channel_affinity_setting.rules"], "a global mode never rewrites rule fields")
			snapshot := CurrentRequestPolicy()
			assert.Error(t, UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.session_mode": "unknown"}))
			assert.Same(t, snapshot, CurrentRequestPolicy())
			for _, value := range []string{"-1", "1.5", "bad", strconv.Itoa(math.MaxInt)} {
				assert.Error(t, UpdateRequestPolicyOptions(map[string]string{"RetryTimes": value}))
				assert.Same(t, snapshot, CurrentRequestPolicy())
			}
			assert.Error(t, UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.rules": "[", "RetryTimes": "8"}))
			assert.Same(t, snapshot, CurrentRequestPolicy())
			var option Option
			require.NoError(t, db.Where(map[string]any{"key": "RetryTimes"}).First(&option).Error)
			assert.Equal(t, "2", option.Value)
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("policy_fail", func(tx *gorm.DB) {
				if tx.Statement.Table == "policy_test_options" {
					tx.AddError(errors.New("save rejected"))
				}
			}))
			assert.Error(t, UpdateRequestPolicyOptions(map[string]string{"RetryTimes": "8", "channel_affinity_setting.session_mode": "off"}))
			assert.Same(t, snapshot, CurrentRequestPolicy())
			require.NoError(t, db.Callback().Update().Remove("policy_fail"))
			option = Option{}
			require.NoError(t, db.Where(map[string]any{"key": "RetryTimes"}).First(&option).Error)
			assert.Equal(t, "2", option.Value)
			loadOptionsFromDatabase()
			assert.Equal(t, "strict", CurrentRequestPolicy().Affinity.SessionMode, "a failed save keeps the persisted global mode")
		})
	}
}

// Global fallback switch: on by default, explicit true/false accepted, non-boolean
// values rejected, and the legacy pool key stays accepted.
func TestRequestPolicyFallbackEnabled(t *testing.T) {
	snapshot, err := BuildRequestPolicy(nil)
	require.NoError(t, err)
	require.True(t, snapshot.Affinity.FallbackEnabled, "fallback is enabled by default")

	snapshot, err = BuildRequestPolicy(map[string]string{"channel_affinity_setting.fallback_enabled": "false"})
	require.NoError(t, err)
	require.False(t, snapshot.Affinity.FallbackEnabled)

	snapshot, err = BuildRequestPolicy(map[string]string{
		"channel_affinity_setting.fallback_enabled":     "true",
		"channel_affinity_setting.fallback_channel_ids": "[66,55]",
	})
	require.NoError(t, err)
	require.True(t, snapshot.Affinity.FallbackEnabled)
	require.Equal(t, []int{66, 55}, snapshot.Affinity.FallbackChannelIDs, "the legacy pool key stays accepted")

	for _, value := range []string{"yes", "1", "", "TRUE"} {
		_, err := BuildRequestPolicy(map[string]string{"channel_affinity_setting.fallback_enabled": value})
		require.Error(t, err, "value %q must be rejected", value)
	}
}

func TestRequestPolicyAcceptsFallbackChannelIDs(t *testing.T) {
	options := map[string]string{"channel_affinity_setting.fallback_channel_ids": "[66,55]"}
	snapshot, err := BuildRequestPolicy(options)
	require.NoError(t, err)
	require.Equal(t, []int{66, 55}, snapshot.Affinity.FallbackChannelIDs)
}

func TestRequestPolicyRejectsMalformedFallbackChannelIDs(t *testing.T) {
	options := map[string]string{"channel_affinity_setting.fallback_channel_ids": `["a"]`}
	_, err := BuildRequestPolicy(options)
	require.Error(t, err)
}

func TestFallbackOptionsRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "policy_test_"}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousSnapshot := DB, CurrentRequestPolicy()
	common.OptionMapRWMutex.Lock()
	previousOptions := maps.Clone(common.OptionMap)
	common.OptionMap = maps.Clone(previousSnapshot.Options)
	common.OptionMapRWMutex.Unlock()
	DB = db
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(&Option{}))
		for k, v := range previousSnapshot.Options {
			require.NoError(t, updateOptionMap(k, v))
		}
		requestPolicySnapshot.Store(previousSnapshot)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Option{}))

	require.NoError(t, UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.fallback_channel_ids": "[66,55]"}))
	require.Equal(t, []int{66, 55}, CurrentRequestPolicy().Affinity.FallbackChannelIDs)

	require.NoError(t, UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.fallback_enabled": "false"}))
	require.False(t, CurrentRequestPolicy().Affinity.FallbackEnabled)

	loadOptionsFromDatabase()
	loadOptionsFromDatabase()
	require.Equal(t, []int{66, 55}, CurrentRequestPolicy().Affinity.FallbackChannelIDs, "fallback pool survives a reload")
	require.False(t, CurrentRequestPolicy().Affinity.FallbackEnabled, "the fallback switch survives a reload")
	require.True(t, operation_setting.IsFallbackChannel(66))
	require.False(t, operation_setting.IsFallbackChannel(999))
}
