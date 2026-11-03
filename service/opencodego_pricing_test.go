package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertOpenCodeGoPricingExpressions 锁定 opencode-go 同步直接产出的计费
// 表达式：billing_mode 全部为 tiered_expr，系数等于目录中的 USD/百万 token
// 单价，input→p、output→c、cache_read→cr、cache_write→cc，tiers 生成 len 阶梯。
func TestConvertOpenCodeGoPricingExpressions(t *testing.T) {
	apiJSON := `{
  "opencode-go": {
    "models": {
      "plain-model": {
        "id": "plain-model",
        "cost": {"input": 1.0, "output": 4.0, "cache_read": 0.5, "cache_write": 0.25}
      },
      "no-cache-model": {
        "id": "no-cache-model",
        "cost": {"input": 2.0, "output": 6.0}
      },
      "explicit-zero-cache-model": {
        "id": "explicit-zero-cache-model",
        "cost": {"input": 1.0, "output": 2.0, "cache_write": 0}
      },
      "tiered-model": {
        "id": "tiered-model",
        "cost": {
          "input": 0.2, "output": 1.2, "cache_read": 0.02, "cache_write": 0.25,
          "tiers": [
            {"input": 0.4, "output": 1.8, "cache_read": 0.04, "cache_write": 0.5, "tier": {"type": "context", "size": 272000}}
          ]
        }
      },
      "multi-tier-model": {
        "id": "multi-tier-model",
        "cost": {
          "input": 1.0, "output": 2.0,
          "tiers": [
            {"input": 3.0, "output": 6.0, "tier": {"type": "context", "size": 400000}},
            {"input": 2.0, "output": 4.0, "tier": {"type": "context", "size": 200000}}
          ]
        }
      },
      "free-model": {
        "id": "free-model",
        "cost": {"input": 0, "output": 0, "cache_read": 0}
      },
      "deepseek-v4.1-flash": {
        "id": "deepseek-v4.1-flash",
        "cost": {"input": 0.15, "output": 0.6, "cache_read": 0.003}
      },
      "deepseek-v4-pro": {
        "id": "deepseek-v4-pro",
        "cost": {"input": 0.66, "output": 1.98, "cache_read": 0.022}
      }
    }
  }
}`

	converted, err := convertOpenCodeGoRatioData([]byte(apiJSON), 0)
	require.NoError(t, err)

	// 不再产出 ratio 字段。
	assert.NotContains(t, converted, "model_ratio")
	assert.NotContains(t, converted, "completion_ratio")
	assert.NotContains(t, converted, "cache_ratio")

	modes, ok := converted[billing_setting.BillingModeField].(map[string]any)
	require.True(t, ok, "billing_mode must be present")
	expressions, ok := converted[billing_setting.BillingExprField].(map[string]any)
	require.True(t, ok, "billing_expr must be present")

	deepSeekFlash := billing_setting.DeepSeekPeakCondition +
		` ? tier("peak", p * 0.3 + cr * 0.006 + c * 1.2) : tier("off_peak", p * 0.15 + cr * 0.003 + c * 0.6)`
	deepSeekPro := billing_setting.DeepSeekPeakCondition +
		` ? tier("peak", p * 1.32 + cr * 0.044 + c * 3.96) : tier("off_peak", p * 0.66 + cr * 0.022 + c * 1.98)`

	want := map[string]string{
		"plain-model":               `tier("standard", p * 1 + c * 4 + cr * 0.5 + cc * 0.25)`,
		"no-cache-model":            `tier("standard", p * 2 + c * 6)`,
		"explicit-zero-cache-model": `tier("standard", p * 1 + c * 2 + cc * 0)`,
		"tiered-model":              `len <= 272000 ? tier("standard", p * 0.2 + c * 1.2 + cr * 0.02 + cc * 0.25) : tier("long_context", p * 0.4 + c * 1.8 + cr * 0.04 + cc * 0.5)`,
		"multi-tier-model":          `len <= 200000 ? tier("standard", p * 1 + c * 2) : len <= 400000 ? tier("long_context", p * 2 + c * 4) : tier("long_context", p * 3 + c * 6)`,
		"free-model":                `tier("base", fixed(0))`,
		"deepseek-v4.1-flash":       deepSeekFlash,
		"deepseek-v4-pro":           deepSeekPro,
	}
	for name, wantExpr := range want {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, billing_setting.BillingModeTieredExpr, modes[name])
			expr, ok := expressions[name].(string)
			require.True(t, ok, "billing_expr missing for %s", name)
			assert.Equal(t, wantExpr, expr)
			_, err := billingexpr.CompileFromCache(expr)
			require.NoError(t, err, expr)
		})
	}

	// 没有对应价格时不得凭空生成 0 价项。
	assert.Equal(t, `tier("standard", p * 2 + c * 6)`, expressions["no-cache-model"])

	// DeepSeek 两档必须都出现，且 peak 是 off-peak 的 2 倍。
	assert.Contains(t, expressions["deepseek-v4.1-flash"], `tier("peak", p * 0.3 + cr * 0.006 + c * 1.2)`)
	assert.Contains(t, expressions["deepseek-v4.1-flash"], `tier("off_peak", p * 0.15 + cr * 0.003 + c * 0.6)`)
}

// TestConvertOpenCodeGoSkipsPathologicalPrices 验证计费换算的输入防御不变式：
// 病态价格（NaN/±Inf/负值/超 maxRatioBound）不得进入真实计费。任一价格字段
// 异常（含阶梯价格）都跳过该模型并告警，避免把异常值写成低价或免费。
func TestConvertOpenCodeGoMissingPriceFallbacks(t *testing.T) {
	apiJSON := `{
  "opencode-go": {
    "models": {
      "missing-output": {"id": "missing-output", "cost": {"input": 3}},
      "gpt-5.6-luna": {"id": "gpt-5.6-luna", "cost": {"input": 3}},
      "empty-tier": {"id": "empty-tier", "cost": {"input": 1, "output": 2, "tiers": [{"tier": {"type": "context", "size": 100000}}]}},
      "tier-missing-output": {"id": "tier-missing-output", "cost": {"input": 1, "output": 2, "tiers": [{"input": 3, "tier": {"type": "context", "size": 100000}}]}},
      "deepseek-v4-pro": {"id": "deepseek-v4-pro", "cost": {"input": 0.66, "output": 1.98}}
    }
  }
}`
	converted, err := convertOpenCodeGoRatioData([]byte(apiJSON), 0)
	require.NoError(t, err)
	expressions, ok := converted[billing_setting.BillingExprField].(map[string]any)
	require.True(t, ok, "billing_expr must be present")

	// 缺少 output 时沿用旧 completion ratio 解析：无前缀模型回退 1，gpt-5.x 回退 6。
	assert.Equal(t, `tier("standard", p * 3 + c * 3)`, expressions["missing-output"])
	assert.Equal(t, `tier("standard", p * 3 + c * 18)`, expressions["gpt-5.6-luna"])
	// 空阶梯没有任何价格字段时必须忽略，不能生成 fixed(0) 的长上下文免费档。
	assert.Equal(t, `tier("standard", p * 1 + c * 2)`, expressions["empty-tier"])
	// 阶梯缺少 output 时沿用基础 output 价。
	assert.Equal(t, `len <= 100000 ? tier("standard", p * 1 + c * 2) : tier("long_context", p * 3 + c * 2)`, expressions["tier-missing-output"])
	// DeepSeek 缺少 cache_read 时省略 cr，但仍保留 peak/off_peak 两档。
	deepSeek, ok := expressions["deepseek-v4-pro"].(string)
	require.True(t, ok)
	assert.NotContains(t, deepSeek, "cr *")
	assert.Contains(t, deepSeek, `tier("peak", p * 1.32 + c * 3.96)`)
	assert.Contains(t, deepSeek, `tier("off_peak", p * 0.66 + c * 1.98)`)
}

func TestConvertOpenCodeGoSkipsPathologicalPrices(t *testing.T) {
	apiJSON := `{
  "opencode-go": {
    "models": {
      "norm-model": {
        "id": "norm-model",
        "cost": {"input": 1.0, "output": 4.0, "cache_read": 0.5}
      },
      "free-model": {
        "id": "free-model",
        "cost": {"input": 0.0, "output": 0.0}
      },
      "huge-input-model": {
        "id": "huge-input-model",
        "cost": {"input": 2000000.0, "output": 1.0}
      },
      "negative-input-model": {
        "id": "negative-input-model",
        "cost": {"input": -1.0}
      },
      "nil-input-model": {
        "id": "nil-input-model",
        "cost": {"output": 2.0}
      },
      "huge-output-model": {
        "id": "huge-output-model",
        "cost": {"input": 1.0, "output": 3000000.0}
      },
      "negative-cache-write-model": {
        "id": "negative-cache-write-model",
        "cost": {"input": 1.0, "cache_write": -0.5}
      },
      "huge-tier-model": {
        "id": "huge-tier-model",
        "cost": {"input": 1.0, "tiers": [{"input": 3000000.0, "tier": {"type": "context", "size": 100000}}]}
      }
    }
  }
}`

	converted, err := convertOpenCodeGoRatioData([]byte(apiJSON), 0)
	require.NoError(t, err)
	modes, ok := converted[billing_setting.BillingModeField].(map[string]any)
	require.True(t, ok, "billing_mode must be present")
	expressions, ok := converted[billing_setting.BillingExprField].(map[string]any)
	require.True(t, ok, "billing_expr must be present")

	assert.Equal(t, `tier("standard", p * 1 + c * 4 + cr * 0.5)`, expressions["norm-model"])
	assert.Equal(t, `tier("base", fixed(0))`, expressions["free-model"])

	for _, name := range []string{
		"huge-input-model",
		"negative-input-model",
		"nil-input-model",
		"huge-output-model",
		"negative-cache-write-model",
		"huge-tier-model",
	} {
		assert.NotContains(t, modes, name, name)
		assert.NotContains(t, expressions, name, name)
	}

	for name, expr := range expressions {
		_, err := billingexpr.CompileFromCache(expr.(string))
		require.NoError(t, err, name)
	}
}

// TestIsValidOpenCodeGoCostBounds 锁定 isValidOpenCodeGoCost 的边界：合法价格
// 必须是有限、非负且不超 maxRatioBound；NaN/±Inf/负值/超上界一律拒绝。
func TestIsValidOpenCodeGoCostBounds(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "zero", value: 0, want: true},
		{name: "normal", value: 1.25, want: true},
		{name: "at upper bound", value: maxRatioBound, want: true},
		{name: "above upper bound", value: maxRatioBound + 1, want: false},
		{name: "huge", value: 1e12, want: false},
		{name: "negative", value: -0.1, want: false},
		{name: "NaN", value: math.NaN(), want: false},
		{name: "+Inf", value: math.Inf(1), want: false},
		{name: "-Inf", value: math.Inf(-1), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidOpenCodeGoCost(tt.value))
		})
	}
}
