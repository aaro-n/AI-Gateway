package anthropic

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"time"

	_ "embed"

	"ai-gateway/internal/core/registry"
)

// =============================================================================
// SyncModels — API 优先，本地补齐缺失字段，API 不可用时纯本地兜底
// =============================================================================

//go:embed models.json
var localModelsJSON []byte

// modelEntry 对应 models.json 中的单条模型配置
type modelEntry struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	OwnedBy        string `json:"owned_by"`
	ContextWindow  int    `json:"context_window"`
	MaxOutput      int    `json:"max_output"`
	SupportsVision bool   `json:"supports_vision"`
	SupportsTools  bool   `json:"supports_tools"`
	SupportsStream bool   `json:"supports_stream"`
}

func loadLocalModels() (map[string]modelEntry, error) {
	var entries []modelEntry
	if err := json.Unmarshal(localModelsJSON, &entries); err != nil {
		return nil, err
	}
	m := make(map[string]modelEntry, len(entries))
	for _, e := range entries {
		m[e.ID] = e
	}
	return m, nil
}

func (p *AnthropicProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	localMap, err := loadLocalModels()
	if err != nil {
		return nil, err
	}

	// 尝试 API
	apiSpecs, apiErr := p.fetchModelSpecs()
	if apiErr != nil {
		log.Printf("[Anthropic] Models API unavailable (%v), using local models only (%d)", apiErr, len(localMap))
		return buildFromLocal(providerID, localMap), nil
	}
	log.Printf("[Anthropic] API=%d models, local=%d models", len(apiSpecs), len(localMap))

	// --- API 成功：遍历 API 为主线，本地补齐缺失 + 本地独有的追加 ---
	// 注意：Anthropic API 返回带日期后缀的 ID（如 claude-haiku-4-5-20251001），
	// 需要归一化为短 ID（claude-haiku-4-5）才能与本地 models.json 匹配。
	// 多个日期版本会去重，只保留第一个。
	type apiWithNorm struct {
		api  *anthropicModelSpec
		norm string
	}
	result := make([]registry.ProviderModel, 0, len(apiSpecs)+len(localMap))
	seen := map[string]bool{}

	for id, api := range apiSpecs {
		normID := normalizeModelID(id)
		if seen[normID] {
			continue
		}
		local, hasLocal := localMap[normID]

		// API 值优先；API 未返回（== 0 或 ""）时用本地补齐
		ctx := api.MaxInput
		out := api.MaxOutput
		vis := api.Capabilities.Vision.Supported
		name := api.DisplayName
		ownedBy := "anthropic"

		if hasLocal {
			if ctx == 0 {
				ctx = local.ContextWindow
			}
			if out == 0 {
				out = local.MaxOutput
			}
			if !vis {
				vis = local.SupportsVision
			}
			if name == "" {
				name = local.DisplayName
			}
			ownedBy = local.OwnedBy
		}
		if name == "" {
			name = normID
		}

		seen[normID] = true
		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        normID,
			DisplayName:    name,
			OwnedBy:        ownedBy,
			ContextWindow:  ctx,
			MaxOutput:      out,
			SupportsVision: vis,
			SupportsTools:  localBool(hasLocal, local.SupportsTools, true),
			SupportsStream: localBool(hasLocal, local.SupportsStream, true),
			IsAvailable:    true,
			Source:         "sync",
		})
	}

	// 本地有但 API 没有的模型 → 追加
	for id, local := range localMap {
		if seen[id] {
			continue
		}
		result = append(result, toProviderModel(providerID, local))
	}

	return result, nil
}

// =============================================================================
// 兜底 & 工具函数
// =============================================================================

func buildFromLocal(providerID uint, localMap map[string]modelEntry) []registry.ProviderModel {
	result := make([]registry.ProviderModel, 0, len(localMap))
	for _, e := range localMap {
		result = append(result, toProviderModel(providerID, e))
	}
	return result
}

func toProviderModel(providerID uint, e modelEntry) registry.ProviderModel {
	return registry.ProviderModel{
		ProviderID:     providerID,
		ModelID:        e.ID,
		DisplayName:    e.DisplayName,
		OwnedBy:        e.OwnedBy,
		ContextWindow:  e.ContextWindow,
		MaxOutput:      e.MaxOutput,
		SupportsVision: e.SupportsVision,
		SupportsTools:  e.SupportsTools,
		SupportsStream: e.SupportsStream,
		IsAvailable:    true,
		Source:         "sync",
	}
}

// localBool: if local exists use its value, otherwise fallback.
func localBool(hasLocal bool, localVal, fallback bool) bool {
	if hasLocal {
		return localVal
	}
	return fallback
}

// Anthropic API 返回带日期后缀的模型 ID（如 claude-haiku-4-5-20251001），
// 归一化为短 ID（claude-haiku-4-5）以匹配本地 models.json。
var dateSuffixRe = regexp.MustCompile(`-\d{8}$`)

func normalizeModelID(id string) string {
	return dateSuffixRe.ReplaceAllString(id, "")
}

// =============================================================================
// Anthropic Models API
// =============================================================================

type anthropicModelSpec struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	MaxInput     int    `json:"max_input_tokens"`
	MaxOutput    int    `json:"max_tokens"`
	Capabilities struct {
		Vision struct {
			Supported bool `json:"supported"`
		} `json:"image_input"`
	} `json:"capabilities"`
}

func (p *AnthropicProvider) fetchModelSpecs() (map[string]*anthropicModelSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Data []anthropicModelSpec `json:"data"`
	}
	if err := p.sdk.Get(ctx, "/v1/models", nil, &result); err != nil {
		return nil, err
	}

	specs := make(map[string]*anthropicModelSpec, len(result.Data))
	for i := range result.Data {
		specs[result.Data[i].ID] = &result.Data[i]
	}
	return specs, nil
}
