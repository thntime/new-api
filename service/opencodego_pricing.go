package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// ConvertOpenCodeGoToRatioData 解析 models.opencode.ai api.json 并转换为本地
// 定价数据：billing_mode = tiered_expr 与对应 billing_expr。表达式系数直接
// 采用 api.json 的 USD/1M tokens 单价，不再先换算成 ratio。
//
// 函数名保留历史命名，调用方（同步抓取）无需改动。
func ConvertOpenCodeGoToRatioData(reader io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return convertOpenCodeGoRatioData(data, 0)
}

// maxRatioBound 是 api.json 单条价格字段（USD/1M tokens）能进入计费的上界
// （建议值 1e6，覆盖当前全部已知模型价格三个数量级）。上游 api.json 由
// 第三方维护，病态输入（如 1e999 解析出的 +Inf 或恶意超大数）若直接进入
// 表达式会让单价无界放大并污染真实计费，因此按计费安全不变式统一拒绝。
const maxRatioBound = 1e6

// openCodeGoDeepSeekModels 使用内置的闲时/忙时规则，而不是目录中的单一
// cost：目录只发布 off-peak 价，peak 按 2× 生成。
var openCodeGoDeepSeekModels = map[string]struct{}{
	"deepseek-v4.1-flash": {},
	"deepseek-v4-pro":     {},
}

// convertOpenCodeGoRatioData 将 api.json 定价（USD/1M tokens）转换为计费表达式：
// input→p、output→c、cache_read→cr、cache_write→cc；存在 tiers 时按 size 升序
// 生成 len 阶梯。目录缺省的字段不生成对应项，避免把未知价格当成 0；全部价格
// 为 0 的免费模型生成 tier("base", fixed(0))。
//
// 病态价格（NaN/±Inf/负值/超 maxRatioBound）不会进入计费：任一字段异常都跳过
// 整个模型并告警，避免异常值被写成低价或免费。第二个参数是历史 ratio 换算
// 使用的汇率，现已不再参与计算，仅为兼容既有调用保留。
func convertOpenCodeGoRatioData(data []byte, _ float64) (map[string]any, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("failed to decode opencode api.json response: %w", err)
	}
	if len(upstream) == 0 {
		return nil, fmt.Errorf("empty opencode api.json response")
	}

	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("no valid opencode pricing entries found")
	}

	modes := make(map[string]any)
	expressions := make(map[string]any)
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		expression, ok := openCodeGoBillingExpr(name, models[name].Cost)
		if !ok {
			continue
		}
		modes[name] = billing_setting.BillingModeTieredExpr
		expressions[name] = expression
	}
	if len(modes) == 0 {
		return map[string]any{}, nil
	}
	return map[string]any{
		billing_setting.BillingModeField: modes,
		billing_setting.BillingExprField: expressions,
	}, nil
}

// openCodeGoBillingExpr 为单个模型生成计费表达式。缺少 input 价格时不能按 0
// 处理：那会把未知价格当成免费模型，直接改变真实计费。
func openCodeGoBillingExpr(name string, cost openCodeGoCost) (string, bool) {
	if cost.Input == nil {
		logger.LogWarn(context.Background(),
			"opencode-go pricing: skip model %q without input cost", name)
		return "", false
	}
	// A missing output price must not become a free output charge: reuse the
	// legacy completion-ratio resolution (including model-family defaults) so
	// the expression matches the old engine's output price. Cache read/write
	// stay optional because their tokens remain in p when omitted.
	if cost.Output == nil {
		completionRatio := ratio_setting.ResolveCompletionRatio(name, nil).Ratio
		output := *cost.Input * completionRatio
		logger.LogWarn(context.Background(),
			"opencode-go pricing: model %q has no output cost; using input price × completion ratio %g", name, completionRatio)
		cost.Output = &output
	}
	if _, ok := openCodeGoDeepSeekModels[name]; ok {
		return openCodeGoDeepSeekExpr(name, cost)
	}
	return openCodeGoContextExpr(name, cost)
}

// openCodeGoDeepSeekExpr 以目录中的 input/output/cache_read 作为 off-peak 单价，
// peak 固定为 2×，时间窗口复用内置 DeepSeek 规则。
func openCodeGoDeepSeekExpr(name string, cost openCodeGoCost) (string, bool) {
	input, output := *cost.Input, *cost.Output
	if !isValidOpenCodeGoCost(input) || !isValidOpenCodeGoCost(output) {
		logger.LogWarn(context.Background(),
			"opencode-go pricing: skip DeepSeek model %q for invalid cost (input=%v output=%v, bound=%g)",
			name, input, output, maxRatioBound)
		return "", false
	}
	cacheRead := cost.CacheRead
	if cacheRead != nil && !isValidOpenCodeGoCost(*cacheRead) {
		logger.LogWarn(context.Background(),
			"opencode-go pricing: skip DeepSeek model %q for invalid cache_read cost %v (bound=%g)",
			name, *cacheRead, maxRatioBound)
		return "", false
	}
	if input == 0 && output == 0 && (cacheRead == nil || *cacheRead == 0) {
		return `tier("base", fixed(0))`, true
	}
	terms := func(scale float64) string {
		parts := []string{"p * " + formatOpenCodeGoPrice(input*scale)}
		if cacheRead != nil {
			parts = append(parts, "cr * "+formatOpenCodeGoPrice(*cacheRead*scale))
		}
		parts = append(parts, "c * "+formatOpenCodeGoPrice(output*scale))
		return strings.Join(parts, " + ")
	}
	return billing_setting.DeepSeekPeakCondition +
		` ? tier("peak", ` + terms(2) + `) : tier("off_peak", ` + terms(1) + `)`, true
}

// openCodeGoContextExpr 生成基础档与上下文阶梯。多个 tiers 按 size 升序串联，
// 阶梯价格只取该阶梯自身声明的字段。
func openCodeGoContextExpr(name string, cost openCodeGoCost) (string, bool) {
	baseBody, baseZero, ok := openCodeGoTokenBody(cost.openCodeGoPrices)
	if !ok {
		logger.LogWarn(context.Background(),
			"opencode-go pricing: skip model %q for invalid cost (bound=%g)", name, maxRatioBound)
		return "", false
	}

	tiers := make([]openCodeGoCostTier, 0, len(cost.Tiers))
	for _, tier := range cost.Tiers {
		if tier.Tier == nil || tier.Tier.Size <= 0 || tier.Input == nil {
			logger.LogWarn(context.Background(),
				"opencode-go pricing: skip model %q tier without input or size", name)
			continue
		}
		if tier.Output == nil {
			tier.Output = cost.Output
		}
		tiers = append(tiers, tier)
	}
	if len(tiers) == 0 {
		if baseZero {
			return `tier("base", fixed(0))`, true
		}
		return `tier("standard", ` + baseBody + `)`, true
	}
	sort.SliceStable(tiers, func(i, j int) bool {
		return tiers[i].Tier.Size < tiers[j].Tier.Size
	})

	last := tiers[len(tiers)-1]
	lastBody, lastZero, ok := openCodeGoTokenBody(last.openCodeGoPrices)
	if !ok {
		logger.LogWarn(context.Background(),
			"opencode-go pricing: skip model %q for invalid tier cost (size=%d, bound=%g)",
			name, last.Tier.Size, maxRatioBound)
		return "", false
	}
	expression := `tier("long_context", ` + openCodeGoTierBody(lastBody, lastZero) + `)`
	for i := len(tiers) - 1; i >= 1; i-- {
		body, zero, ok := openCodeGoTokenBody(tiers[i-1].openCodeGoPrices)
		if !ok {
			logger.LogWarn(context.Background(),
				"opencode-go pricing: skip model %q for invalid tier cost (size=%d, bound=%g)",
				name, tiers[i].Tier.Size, maxRatioBound)
			return "", false
		}
		expression = fmt.Sprintf(`len <= %d ? tier("long_context", %s) : %s`,
			tiers[i].Tier.Size, openCodeGoTierBody(body, zero), expression)
	}
	return fmt.Sprintf(`len <= %d ? tier("standard", %s) : %s`,
		tiers[0].Tier.Size, openCodeGoTierBody(baseBody, baseZero), expression), true
}

// openCodeGoTokenBody 按 p、c、cr、cc 顺序拼接价格项。返回 allZero 表示所有
// 已声明字段都为 0（由调用方决定用 fixed(0) 表达免费）。
func openCodeGoTokenBody(prices openCodeGoPrices) (body string, allZero bool, ok bool) {
	fields := []struct {
		variable string
		value    *float64
	}{
		{"p", prices.Input},
		{"c", prices.Output},
		{"cr", prices.CacheRead},
		{"cc", prices.CacheWrite},
	}
	terms := make([]string, 0, len(fields))
	allZero = true
	for _, field := range fields {
		if field.value == nil {
			continue
		}
		if !isValidOpenCodeGoCost(*field.value) {
			return "", false, false
		}
		if *field.value != 0 {
			allZero = false
		}
		terms = append(terms, field.variable+" * "+formatOpenCodeGoPrice(*field.value))
	}
	if allZero {
		return "", true, true
	}
	return strings.Join(terms, " + "), false, true
}

func openCodeGoTierBody(body string, allZero bool) string {
	if allZero {
		return "fixed(0)"
	}
	return body
}

// formatOpenCodeGoPrice 用最短可精确还原的十进制表示，避免 0.1+0.2 之类的
// 浮点尾数进入表达式。
func formatOpenCodeGoPrice(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// isValidOpenCodeGoCost 校验单个价格字段为有限、非负且不超 maxRatioBound 的
// 正常值（allFinite 校验，风格同 controller/model_rich.go allFiniteNonNegative）。
// 防止 NaN/±Inf/负值/超大值（如 1e999 解析出的 +Inf 或恶意大数）进入计费换算。
func isValidOpenCodeGoCost(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= maxRatioBound
}
