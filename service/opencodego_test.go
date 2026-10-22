package service

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const openCodeGoAPIJSONFixture = `{
  "opencode-go": {
    "models": {
      "gpt-5.6-luna": {"id": "gpt-5.6-luna", "name": "GPT 5.6 Luna", "description": "GPT 5.6 Luna official model", "provider": {"npm": "@ai-sdk/openai"}, "status": "", "cost": {"input": 15, "output": 60, "cache_read": 7.5}},
      "no-cap-model": {"description": "A model without usage cap", "status": "", "cost": {"input": 1, "output": 2, "cache_read": 0.5}},
      "deepseek-v4-pro": {"name": "DeepSeek V4 Pro", "description": "DeepSeek V4 Pro model", "status": "", "cost": {"input": 0.435, "output": 0.87, "cache_read": 0.003625}},
      "shared-free": {"name": "Shared", "description": "shared paid description", "provider": {"npm": "@ai-sdk/openai"}, "status": "", "cost": {"input": 10, "output": 20}},
      "fill-model": {"description": "opencode-go description", "status": "", "cost": {"input": 3}},
      "nil-cost-free": {"name": "Nil Cost", "description": "paid entry", "status": "", "cost": {"input": 9}},
      "deprecated-model": {"name": "Deprecated", "description": "deprecated model", "status": "deprecated", "cost": {"input": 1, "output": 2}}
    }
  },
  "aaa-provider": {
    "models": {
      "only-elsewhere": {"name": "Only Elsewhere", "description": "from aaa", "provider": {"npm": "@ai-sdk/google"}, "status": "", "cost": {"input": 2, "output": 4}},
      "fill-model": {"name": "Fill Model", "provider": {"npm": "@ai-sdk/deepseek"}, "status": "", "limit": {"context": 64000, "output": 4096}, "cost": {"input": 99}},
      "gpt-5.6-luna": {"name": "WRONG", "description": "wrong description", "provider": {"npm": "@ai-sdk/anthropic"}, "status": "", "cost": {"input": 99, "output": 99}}
    }
  },
  "zzz-provider": {
    "models": {
      "only-elsewhere": {"name": "ZZZ", "description": "from zzz", "status": "", "cost": {"input": 8, "output": 16}},
      "Kimi-K3": {"name": "Case", "status": "", "cost": {"input": 1, "output": 2}},
      "missing-price": {"name": "Missing price", "status": ""}
    }
  },
  "opencode": {
    "models": {
      "hy3-free": {"name": "Hy3 Free", "description": "Hy3 free tier", "provider": {"npm": "@ai-sdk/anthropic"}, "status": "", "cost": {"input": 0, "output": 0}},
      "shared-free": {"name": "Shared Free", "description": "shared free description", "status": "", "cost": {"input": 0, "output": 0}},
      "paid-x": {"name": "Paid X", "description": "paid model", "status": "", "cost": {"input": 5, "output": 10}},
      "nil-cost-free": {"name": "Nil Cost Free", "description": "free entry without cost", "status": ""},
      "dep-free": {"name": "Dep Free", "description": "deprecated free", "status": "deprecated", "cost": {"input": 0, "output": 0}}
    }
  }
}`

func TestParseOpenCodeGoModels(t *testing.T) {
	t.Run("valid api.json", func(t *testing.T) {
		ids, err := parseOpenCodeGoModels([]byte(openCodeGoAPIJSONFixture))
		require.NoError(t, err)
		// opencode-go 优先；其余 provider 段按精确 id 补入；deprecated 剔除；
		// -free 免费模型合并。Kimi-K3 与 kimi-k3 视为不同 id。
		assert.Equal(t, []string{"Kimi-K3", "deepseek-v4-pro", "fill-model", "gpt-5.6-luna", "hy3-free", "missing-price", "nil-cost-free", "no-cap-model", "only-elsewhere", "paid-x", "shared-free"}, ids)
		assert.NotContains(t, ids, "kimi-k3")
		assert.NotContains(t, ids, "deprecated-model")
		assert.NotContains(t, ids, "dep-free")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseOpenCodeGoModels([]byte("{not json"))
		require.Error(t, err)
	})

	t.Run("no valid models", func(t *testing.T) {
		_, err := parseOpenCodeGoModels([]byte(`{"opencode-go": {"models": {"x": {"status": "deprecated", "cost": {"input": 1, "output": 1}}}}}`))
		require.Error(t, err)
	})

	t.Run("stable across repeated parses", func(t *testing.T) {
		first, err := parseOpenCodeGoModels([]byte(openCodeGoAPIJSONFixture))
		require.NoError(t, err)
		for range 3 {
			next, err := parseOpenCodeGoModels([]byte(openCodeGoAPIJSONFixture))
			require.NoError(t, err)
			assert.Equal(t, first, next)
		}
	})
}

func TestParseOpenCodeGoModelEntries(t *testing.T) {
	t.Run("valid api.json", func(t *testing.T) {
		entries, err := parseOpenCodeGoModelEntries([]byte(openCodeGoAPIJSONFixture))
		require.NoError(t, err)
		require.Len(t, entries, 11)
		byID := make(map[string]OpenCodeGoModelEntry, len(entries))
		for _, entry := range entries {
			byID[entry.ID] = entry
		}

		// 大小写不同不算同一模型
		assert.Equal(t, "Case", byID["Kimi-K3"].Name)
		assert.NotContains(t, byID, "kimi-k3")

		// opencode-go 条目优先：aaa-provider 的同 id 条目不得覆盖
		assert.Equal(t, "GPT 5.6 Luna", byID["gpt-5.6-luna"].Name)
		assert.Equal(t, "GPT 5.6 Luna official model", byID["gpt-5.6-luna"].Description)
		assert.Equal(t, "@ai-sdk/openai", byID["gpt-5.6-luna"].Provider)

		// -free 模型描述优先取 -free 条目
		assert.Equal(t, "Hy3 Free", byID["hy3-free"].Name)
		assert.Equal(t, "Hy3 free tier", byID["hy3-free"].Description)
		assert.Equal(t, "@ai-sdk/anthropic", byID["hy3-free"].Provider)

		// name 缺失时回退 id；provider 缺失时为空
		assert.Equal(t, "no-cap-model", byID["no-cap-model"].Name)
		assert.Equal(t, "A model without usage cap", byID["no-cap-model"].Description)
		assert.Equal(t, "", byID["no-cap-model"].Provider)

		// 条目级回退按段名升序：aaa-provider 先于 zzz-provider
		assert.Equal(t, "Only Elsewhere", byID["only-elsewhere"].Name)
		assert.Equal(t, "from aaa", byID["only-elsewhere"].Description)
		assert.Equal(t, "@ai-sdk/google", byID["only-elsewhere"].Provider)

		// 字段级补齐：opencode-go 缺失的 name/provider/limit 由同 id 条目补齐，
		// 已有的 description 不被覆盖
		assert.Equal(t, "Fill Model", byID["fill-model"].Name)
		assert.Equal(t, "opencode-go description", byID["fill-model"].Description)
		assert.Equal(t, "@ai-sdk/deepseek", byID["fill-model"].Provider)
		require.NotNil(t, byID["fill-model"].Limit)
		assert.Equal(t, 64000, byID["fill-model"].Limit.Context)

		// 免费条目覆盖同 id 条目时保留自身 name
		require.NotNil(t, byID["nil-cost-free"])
		assert.Equal(t, "Nil Cost Free", byID["nil-cost-free"].Name)

		// 缺少价格的回退条目仍进入模型目录，价格为 nil
		require.NotNil(t, byID["missing-price"])
		assert.Equal(t, "Missing price", byID["missing-price"].Name)

		// 同 id 时 -free 免费条目覆盖（含 name/description）；provider 缺失时回退原条目值
		assert.Equal(t, "Shared Free", byID["shared-free"].Name)
		assert.Equal(t, "shared free description", byID["shared-free"].Description)
		assert.Equal(t, "@ai-sdk/openai", byID["shared-free"].Provider)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseOpenCodeGoModelEntries([]byte("{not json"))
		require.Error(t, err)
	})

	t.Run("no valid models", func(t *testing.T) {
		_, err := parseOpenCodeGoModelEntries([]byte(`{"opencode-go": {"models": {"x": {"status": "deprecated", "cost": {"input": 1, "output": 1}}}}}`))
		require.Error(t, err)
	})
}

func TestConvertOpenCodeGoRatioData(t *testing.T) {
	converted, err := convertOpenCodeGoRatioData([]byte(openCodeGoAPIJSONFixture), 500)
	require.NoError(t, err)

	modelRatios, ok := converted["model_ratio"].(map[string]any)
	require.True(t, ok, "model_ratio missing")
	completionRatios, ok := converted["completion_ratio"].(map[string]any)
	require.True(t, ok, "completion_ratio missing")
	cacheRatios, ok := converted["cache_ratio"].(map[string]any)
	require.True(t, ok, "cache_ratio missing")

	t.Run("no cap multiplier, api.json price used as-is", func(t *testing.T) {
		// input 15 × USD/1000 = 15 × 0.5 = 7.5（无额度系数）
		assert.InDelta(t, 7.5, modelRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 4.0, completionRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 0.5, cacheRatios["gpt-5.6-luna"].(float64), 1e-9)
	})

	t.Run("cache ratio precision preserves restored price", func(t *testing.T) {
		// deepseek-v4-pro: cache_read 0.003625 / input 0.435 = 0.00833333...(循环小数)
		// 前端还原缓存价 = model_ratio × cache_ratio × 1000，必须接近
		// 0.003625 × USD/1000 × 1000 = 1.8125，不能因比值舍入产生可见误差
		modelRatio := modelRatios["deepseek-v4-pro"].(float64)
		assert.InDelta(t, 0.2175, modelRatio, 1e-9)
		cacheRatio := cacheRatios["deepseek-v4-pro"].(float64)
		restored := modelRatio * cacheRatio * 1000
		assert.InDelta(t, 1.8125, restored, 1e-6)
		// 若按 6 位舍入，restored 会差 2.9e-7 < 1e-6 仍通过，
		// 因此再断言比值本身保留 12 位精度（0.008333333333 而非 0.008333）
		assert.InDelta(t, 0.008333333333, cacheRatio, 1e-11)
	})

	t.Run("fallback entries priced from their own provider", func(t *testing.T) {
		// only-elsewhere 来自 aaa-provider（2 USD/1M）→ 2 × 0.5 = 1
		assert.InDelta(t, 1.0, modelRatios["only-elsewhere"].(float64), 1e-9)
		// paid-x 现由条目级回退纳入：5 × 0.5 = 2.5
		assert.InDelta(t, 2.5, modelRatios["paid-x"].(float64), 1e-9)
		assert.InDelta(t, 0.5, modelRatios["Kimi-K3"].(float64), 1e-9)
		// fill-model 的 cost 来自 opencode-go（3 × 0.5 = 1.5），未被回退条目的 99 覆盖
		assert.InDelta(t, 1.5, modelRatios["fill-model"].(float64), 1e-9)
	})

	t.Run("free model ratio is zero without NaN", func(t *testing.T) {
		// -free 合并模型与免费覆盖条目均应为 0
		assert.InDelta(t, 0.0, modelRatios["hy3-free"].(float64), 1e-9)
		assert.InDelta(t, 0.0, modelRatios["shared-free"].(float64), 1e-9)
		_, hasCompletion := completionRatios["hy3-free"]
		assert.False(t, hasCompletion)
		_, hasCache := cacheRatios["hy3-free"]
		assert.False(t, hasCache)
	})

	t.Run("no NaN or Inf values", func(t *testing.T) {
		for _, m := range []map[string]any{modelRatios, completionRatios, cacheRatios} {
			for _, v := range m {
				f, ok := v.(float64)
				require.True(t, ok)
				assert.False(t, math.IsNaN(f), "unexpected NaN")
				assert.False(t, math.IsInf(f, 0), "unexpected Inf")
			}
		}
	})

	t.Run("deprecated excluded and missing price skipped", func(t *testing.T) {
		assert.NotContains(t, modelRatios, "deprecated-model")
		assert.NotContains(t, modelRatios, "dep-free")
		// 缺少 input 价格的条目不得按 0 写入（否则未知价格会变成免费）
		assert.NotContains(t, modelRatios, "missing-price")
		// 免费条目的 cost 为 nil 时保持 nil，不继承付费价格，也不按 0 计费
		assert.NotContains(t, modelRatios, "nil-cost-free")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := convertOpenCodeGoRatioData([]byte("{not json"), 500)
		require.Error(t, err)
	})
}

// TestCollectOpenCodeGoModelsCostSemantics 验证 cost 的两种语义：
// 条目级补齐会填充缺失的 cost；-free 覆盖不继承同 id 付费条目的价格。
func TestCollectOpenCodeGoModelsCostSemantics(t *testing.T) {
	var upstream map[string]openCodeGoProvider
	require.NoError(t, common.Unmarshal([]byte(openCodeGoAPIJSONFixture), &upstream))
	models := collectOpenCodeGoModels(upstream)

	// 条目级回退补齐 cost：fill-model 取 opencode-go 的 3，而不是 aaa 的 99
	require.Contains(t, models, "fill-model")
	require.NotNil(t, models["fill-model"].Cost.Input)
	assert.InDelta(t, 3.0, *models["fill-model"].Cost.Input, 1e-9)

	// 显式 0 的免费条目保持 0
	require.Contains(t, models, "shared-free")
	require.NotNil(t, models["shared-free"].Cost.Input)
	assert.InDelta(t, 0.0, *models["shared-free"].Cost.Input, 1e-9)

	// cost 为 nil 的免费条目不得从付费条目继承价格
	require.Contains(t, models, "nil-cost-free")
	assert.Nil(t, models["nil-cost-free"].Cost.Input)
}

func TestOpenCodeGoCatalogSharedByMetadataAndPricing(t *testing.T) {
	entries, err := parseOpenCodeGoModelEntries([]byte(openCodeGoAPIJSONFixture))
	require.NoError(t, err)
	converted, err := convertOpenCodeGoRatioData([]byte(openCodeGoAPIJSONFixture), 500)
	require.NoError(t, err)
	modelRatios, ok := converted["model_ratio"].(map[string]any)
	require.True(t, ok, "model_ratio missing")

	entryIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entryIDs[entry.ID] = struct{}{}
	}
	// 价格入口是同一解析结果的子集：只额外剔除价格非法或缺失的模型。
	require.NotEmpty(t, modelRatios)
	for id := range modelRatios {
		assert.Contains(t, entryIDs, id)
	}
	assert.Contains(t, entryIDs, "missing-price")
	assert.NotContains(t, modelRatios, "missing-price")
}

func TestParseOpenCodeGoUsage(t *testing.T) {
	futureReset := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	tests := []struct {
		name            string
		body            string
		wantWindows     []OpenCodeGoWindow
		wantErrContains string
	}{
		{
			name: "three windows in fixed order",
			body: fmt.Sprintf(`{"usage":{"rolling":{"status":"ok","percent":4,"resetsAt":%q},"weekly":{"status":"ok","percent":3,"resetsAt":%q},"monthly":{"status":"ok","percent":1,"resetsAt":%q}}}`, futureReset, futureReset, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "session", Status: "ok", UsedPercent: 4, RemainingPercent: 96, ResetAt: futureReset},
				{Period: "weekly", Status: "ok", UsedPercent: 3, RemainingPercent: 97, ResetAt: futureReset},
				{Period: "monthly", Status: "ok", UsedPercent: 1, RemainingPercent: 99, ResetAt: futureReset},
			},
		},
		{
			name: "missing weekly window still returns others",
			body: fmt.Sprintf(`{"usage":{"rolling":{"status":"ok","percent":50,"resetsAt":%q},"monthly":{"status":"ok","percent":70,"resetsAt":%q}}}`, futureReset, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "session", Status: "ok", UsedPercent: 50, RemainingPercent: 50, ResetAt: futureReset},
				{Period: "monthly", Status: "ok", UsedPercent: 70, RemainingPercent: 30, ResetAt: futureReset},
			},
		},
		{
			name: "non-ok status still returned with valid percent",
			body: fmt.Sprintf(`{"usage":{"weekly":{"status":"degraded","percent":80,"resetsAt":%q}}}`, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "weekly", Status: "degraded", UsedPercent: 80, RemainingPercent: 20, ResetAt: futureReset},
			},
		},
		{
			name: "non-numeric percent uses -1 sentinel",
			body: fmt.Sprintf(`{"usage":{"monthly":{"status":"ok","percent":"abc","resetsAt":%q}}}`, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "monthly", Status: "ok", UsedPercent: -1, RemainingPercent: -1, ResetAt: futureReset},
			},
		},
		{
			name: "missing percent uses -1 sentinel",
			body: fmt.Sprintf(`{"usage":{"monthly":{"status":"ok","resetsAt":%q}}}`, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "monthly", Status: "ok", UsedPercent: -1, RemainingPercent: -1, ResetAt: futureReset},
			},
		},
		{
			name: "percent clamped to 100",
			body: fmt.Sprintf(`{"usage":{"weekly":{"status":"ok","percent":123.7,"resetsAt":%q}}}`, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "weekly", Status: "ok", UsedPercent: 100, RemainingPercent: 0, ResetAt: futureReset},
			},
		},
		{
			name: "alias fallback for window key and field names",
			body: fmt.Sprintf(`{"usage":{"rolling_usage":{"status":"ok","usage_percent":12,"reset_at":%q}}}`, futureReset),
			wantWindows: []OpenCodeGoWindow{
				{Period: "session", Status: "ok", UsedPercent: 12, RemainingPercent: 88, ResetAt: futureReset},
			},
		},
		{
			name:            "no windows errors",
			body:            `{"usage":{}}`,
			wantErrContains: "未返回任何用量窗口",
		},
		{
			name:            "invalid json errors",
			body:            `{not json`,
			wantErrContains: "无法解析 opencode usage api 响应",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseOpenCodeGoUsage([]byte(tt.body))
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.Len(t, info.Windows, len(tt.wantWindows))
			for i, want := range tt.wantWindows {
				got := info.Windows[i]
				assert.Equal(t, want.Period, got.Period, "window %d period", i)
				assert.Equal(t, want.Status, got.Status, "window %d status", i)
				assert.InDelta(t, want.UsedPercent, got.UsedPercent, 1e-9, "window %d used", i)
				assert.InDelta(t, want.RemainingPercent, got.RemainingPercent, 1e-9, "window %d remaining", i)
				assert.Equal(t, want.ResetAt, got.ResetAt, "window %d reset_at", i)
				if want.ResetAt != "" {
					// resetInSec 应约等于 now 到 resetAt 的剩余秒数（宽松断言避免 flaky）
					resetAt, err := time.Parse(time.RFC3339, want.ResetAt)
					require.NoError(t, err, "window %d reset_at parse", i)
					assert.InDelta(t, resetAt.Unix()-time.Now().Unix(), got.ResetInSec, 5, "window %d reset_in_sec", i)
				}
			}
		})
	}
}
