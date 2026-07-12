package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

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

// openAIModelEntry API 返回的模型结构
type openAIModelEntry struct {
	ID                  string `json:"id"`
	OwnedBy             string `json:"owned_by"`
	DisplayName         string `json:"display_name"`
	ContextLength       int    `json:"context_length"`
	MaxInputTokens      int    `json:"max_input_tokens"`
	MaxTokens           int    `json:"max_tokens"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
	Pricing             struct {
		Completion float64 `json:"completion"`
		Prompt     float64 `json:"prompt"`
	} `json:"pricing"`
	Capabilities *struct {
		Vision    bool `json:"vision"`
		Streaming bool `json:"streaming"`
	} `json:"capabilities,omitempty"`
}

func (p *OpenAIProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	localMap, err := loadLocalModels()
	if err != nil {
		return nil, err
	}

	// 尝试 API
	apiModels, apiErr := p.fetchModels()
	if apiErr != nil {
		log.Printf("[OpenAI] Models API unavailable (%v), using local models only (%d)", apiErr, len(localMap))
		return buildFromLocal(providerID, localMap), nil
	}
	log.Printf("[OpenAI] API=%d models, local=%d models", len(apiModels), len(localMap))

	// --- API 成功：遍历 API 为主线，本地补齐缺失 + 本地独有的追加 ---
	result := make([]registry.ProviderModel, 0, len(apiModels)+len(localMap))
	seen := map[string]bool{}

	for _, api := range apiModels {
		id := api.ID
		if id == "" || isNonChat(id) {
			continue
		}
		local, hasLocal := localMap[id]

		// API 值优先；API 未返回（== 0 或 \"\"）时用本地补齐
		ctx := api.ContextLength
		if ctx == 0 {
			ctx = api.MaxInputTokens
		}
		out := api.MaxCompletionTokens
		if out == 0 {
			out = api.MaxTokens
		}
		vis := false
		if api.Capabilities != nil {
			vis = api.Capabilities.Vision
		}
		name := api.DisplayName
		ownedBy := api.OwnedBy

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
			if ownedBy == "" {
				ownedBy = local.OwnedBy
			}
		}
		if name == "" {
			name = id
		}
		if ownedBy == "" {
			ownedBy = "openai"
		}

		// 没有 context_window 和 max_output 的跳过
		if ctx == 0 && out == 0 {
			continue
		}

		seen[id] = true
		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        id,
			DisplayName:    name,
			OwnedBy:        ownedBy,
			ContextWindow:  ctx,
			MaxOutput:      out,
			InputPrice:     api.Pricing.Prompt,
			OutputPrice:    api.Pricing.Completion,
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

func (p *OpenAIProvider) fetchModels() ([]openAIModelEntry, error) {
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.httpPool.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var result struct {
		Data []openAIModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func isNonChat(id string) bool {
	return strings.Contains(id, "embedding") || strings.Contains(id, "moderation") ||
		strings.Contains(id, "whisper") || strings.Contains(id, "tts") ||
		strings.Contains(id, "dall-e") || strings.Contains(id, "davinci")
}

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

func localBool(hasLocal bool, localVal, fallback bool) bool {
	if hasLocal {
		return localVal
	}
	return fallback
}
