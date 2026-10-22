package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const openCodeGoModelsURL = "https://models.opencode.ai/api.json"

// openCodeGoProvider 是 models.opencode.ai api.json 中单个 provider 段的结构。
type openCodeGoProvider struct {
	Models map[string]openCodeGoModel `json:"models"`
}

// openCodeGoProviderRef 是 api.json 模型条目中 provider 对象（如 {"npm": "..."}）。
type openCodeGoProviderRef struct {
	NPM string `json:"npm"`
}

type openCodeGoModel struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	Status           string                `json:"status"`
	Family           string                `json:"family"`
	Attachment       *bool                 `json:"attachment"`
	Reasoning        *bool                 `json:"reasoning"`
	ReasoningOptions []map[string]any      `json:"reasoning_options"`
	ToolCall         *bool                 `json:"tool_call"`
	StructuredOutput *bool                 `json:"structured_output"`
	Temperature      *bool                 `json:"temperature"`
	OpenWeights      *bool                 `json:"open_weights"`
	ReleaseDate      string                `json:"release_date"`
	LastUpdated      string                `json:"last_updated"`
	Modalities       *openCodeGoModalities `json:"modalities"`
	Limit            *openCodeGoLimit      `json:"limit"`
	Cost             openCodeGoCost        `json:"cost"`
	Provider         openCodeGoProviderRef `json:"provider"`
}

type openCodeGoModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type openCodeGoLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type openCodeGoCost struct {
	Input     *float64 `json:"input"`
	Output    *float64 `json:"output"`
	CacheRead *float64 `json:"cache_read"`
}

// collectOpenCodeGoModels 解析 api.json 的模型集合：
//  1. opencode-go 段的非 deprecated 条目优先入表；
//  2. 其余 provider 段按段名升序遍历：id 不在结果中时补入该条目，已存在时
//     按字段粒度用该条目补齐空值（opencode-go 已有的值不被覆盖）；
//  3. opencode 段中免费的 -free 模型（cost 全 0 且非 deprecated）覆盖同 id
//     条目，缺失字段由原条目补齐。
//
// 第 2 步按段名升序而非 map 遍历顺序，保证同 id 的回退来源确定。
func collectOpenCodeGoModels(upstream map[string]openCodeGoProvider) map[string]openCodeGoModel {
	models := make(map[string]openCodeGoModel)
	if provider, ok := upstream["opencode-go"]; ok {
		for name, m := range provider.Models {
			if m.Status == "deprecated" {
				continue
			}
			models[name] = m
		}
	}

	// 来源优先级：opencode-go（已在上面写入）> opencode 官方段 > 其余段（段名升序）。
	providers := make([]string, 0, len(upstream))
	for name := range upstream {
		if name == "opencode-go" || name == "opencode" {
			continue
		}
		providers = append(providers, name)
	}
	sort.Strings(providers)
	if _, ok := upstream["opencode"]; ok {
		providers = append([]string{"opencode"}, providers...)
	}
	for _, providerName := range providers {
		provider := upstream[providerName]
		names := make([]string, 0, len(provider.Models))
		for name := range provider.Models {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry := provider.Models[name]
			if entry.Status == "deprecated" {
				continue
			}
			if existing, ok := models[name]; ok {
				models[name] = fillOpenCodeGoModel(existing, entry)
				continue
			}
			models[name] = entry
		}
	}

	if provider, ok := upstream["opencode"]; ok {
		for name, m := range provider.Models {
			if m.Status == "deprecated" || !strings.HasSuffix(name, "-free") {
				continue
			}
			if isOpenCodeGoZeroCost(m.Cost) {
				// 免费模型覆盖同 id 条目；缺失的元数据字段回退原条目的值，
				// 避免免费条目（通常只含价格信息）抹掉付费条目的富元数据。
				// Cost 不参与补齐：免费语义以 -free 条目为准，cost 为 nil 的
				// 免费条目不得从同 id 付费条目继承价格。
				if existing, ok := models[name]; ok {
					cost := m.Cost
					m = fillOpenCodeGoModel(m, existing)
					m.Cost = cost
				}
				models[name] = m
			}
		}
	}
	return models
}

// fillOpenCodeGoModel 用 fallback 的非空字段补齐 base 的空值字段，base 已有
// 的非空值保持不变。ID 与 Status 不参与补齐：Status 的空串表示启用，不是缺失。
func fillOpenCodeGoModel(base, fallback openCodeGoModel) openCodeGoModel {
	if base.Name == "" {
		base.Name = fallback.Name
	}
	if base.Description == "" {
		base.Description = fallback.Description
	}
	if base.Family == "" {
		base.Family = fallback.Family
	}
	if base.ReleaseDate == "" {
		base.ReleaseDate = fallback.ReleaseDate
	}
	if base.LastUpdated == "" {
		base.LastUpdated = fallback.LastUpdated
	}
	if base.Provider.NPM == "" {
		base.Provider = fallback.Provider
	}
	if base.Attachment == nil {
		base.Attachment = fallback.Attachment
	}
	if base.Reasoning == nil {
		base.Reasoning = fallback.Reasoning
	}
	if len(base.ReasoningOptions) == 0 {
		base.ReasoningOptions = fallback.ReasoningOptions
	}
	if base.ToolCall == nil {
		base.ToolCall = fallback.ToolCall
	}
	if base.StructuredOutput == nil {
		base.StructuredOutput = fallback.StructuredOutput
	}
	if base.Temperature == nil {
		base.Temperature = fallback.Temperature
	}
	if base.OpenWeights == nil {
		base.OpenWeights = fallback.OpenWeights
	}
	if base.Modalities == nil {
		base.Modalities = fallback.Modalities
	}
	if base.Limit == nil {
		base.Limit = fallback.Limit
	}
	if base.Cost.Input == nil {
		base.Cost.Input = fallback.Cost.Input
	}
	if base.Cost.Output == nil {
		base.Cost.Output = fallback.Cost.Output
	}
	if base.Cost.CacheRead == nil {
		base.Cost.CacheRead = fallback.Cost.CacheRead
	}
	return base
}

func isOpenCodeGoZeroCost(cost openCodeGoCost) bool {
	input, output := 0.0, 0.0
	if cost.Input != nil {
		input = *cost.Input
	}
	if cost.Output != nil {
		output = *cost.Output
	}
	return input == 0 && output == 0
}

// parseOpenCodeGoModels 解析 models.opencode.ai api.json 的模型 id 列表。
func parseOpenCodeGoModels(data []byte) ([]string, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("invalid opencode api.json response: %w", err)
	}
	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("opencode api.json response contains no valid models")
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// fetchOpenCodeGoAPIJSON 拉取 models.opencode.ai api.json 原始字节。
func fetchOpenCodeGoAPIJSON() ([]byte, error) {
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openCodeGoModelsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// FetchOpenCodeGoModels 拉取 models.opencode.ai 的官方模型列表。
func FetchOpenCodeGoModels() ([]string, error) {
	body, err := fetchOpenCodeGoAPIJSON()
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoModels(body)
}

// OpenCodeGoModelEntry 是 opencode-go 模型库同步使用的结构化条目。
type OpenCodeGoModelEntry struct {
	ID          string
	Name        string
	Description string
	// Status 是 api.json 条目中的 status 字段（""、beta、preview 等；deprecated 已被过滤）。
	Status string
	// Provider 是 api.json 条目中 provider.npm 值（如 "@ai-sdk/openai"），无则空。
	Provider         string
	Family           string
	Attachment       *bool
	Reasoning        *bool
	ReasoningOptions []map[string]any
	ToolCall         *bool
	StructuredOutput *bool
	Temperature      *bool
	OpenWeights      *bool
	ReleaseDate      string
	LastUpdated      string
	Modalities       *ModalitiesEntry
	Limit            *LimitEntry
}

// ModalitiesEntry 输入/输出模态
type ModalitiesEntry struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// LimitEntry 上下文/输出限额（tokens）
type LimitEntry struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// parseOpenCodeGoModelEntries 解析 models.opencode.ai api.json 的结构化条目
// 列表，集合规则与 parseOpenCodeGoModels 完全一致（同 id 时 -free 免费条目覆盖）。
func parseOpenCodeGoModelEntries(data []byte) ([]OpenCodeGoModelEntry, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("invalid opencode api.json response: %w", err)
	}
	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("opencode api.json response contains no valid models")
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]OpenCodeGoModelEntry, 0, len(names))
	for _, name := range names {
		m := models[name]
		entry := OpenCodeGoModelEntry{ID: name, Description: m.Description, Status: m.Status, Provider: m.Provider.NPM}
		entry.Name = m.Name
		if entry.Name == "" {
			entry.Name = name
		}
		entry.Family = m.Family
		entry.Attachment = m.Attachment
		entry.Reasoning = m.Reasoning
		entry.ReasoningOptions = m.ReasoningOptions
		entry.ToolCall = m.ToolCall
		entry.StructuredOutput = m.StructuredOutput
		entry.Temperature = m.Temperature
		entry.OpenWeights = m.OpenWeights
		entry.ReleaseDate = m.ReleaseDate
		entry.LastUpdated = m.LastUpdated
		if m.Modalities != nil {
			entry.Modalities = &ModalitiesEntry{Input: m.Modalities.Input, Output: m.Modalities.Output}
		}
		if m.Limit != nil {
			entry.Limit = &LimitEntry{Context: m.Limit.Context, Output: m.Limit.Output}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// FetchOpenCodeGoModelEntries 拉取 models.opencode.ai 的结构化模型条目。
func FetchOpenCodeGoModelEntries() ([]OpenCodeGoModelEntry, error) {
	body, err := fetchOpenCodeGoAPIJSON()
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoModelEntries(body)
}
