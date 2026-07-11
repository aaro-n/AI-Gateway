package openrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-gateway/internal/core/registry"
)

// =============================================================================
// SyncModels — OpenRouter 禁止自动同步（模型太多）
// =============================================================================

func (p *OpenRouterProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	return nil, fmt.Errorf("OpenRouter does not support auto-sync: too many models. Please add models manually and use the model lookup endpoint to fetch individual model capabilities")
}

// =============================================================================
// LookupModel — 从 OpenRouter API 查询单个模型的详细信息
// =============================================================================

// OpenRouterModelInfo OpenRouter 模型信息 API 返回的数据结构
type OpenRouterModelInfo struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ContextLength       int    `json:"context_length"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
	Pricing             struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Architecture struct {
		Modality string `json:"modality"`
	} `json:"architecture"`
}

// LookupModel 查询指定模型的详细信息。
// 从 OpenRouter /models 列表接口获取数据并过滤指定模型。
func (p *OpenRouterProvider) LookupModel(modelID string) (*registry.ProviderModel, error) {
	allModels, err := p.listModels()
	if err != nil {
		return nil, err
	}

	for _, info := range allModels {
		if info.ID == modelID {
			return info.toProviderModel(), nil
		}
	}
	return nil, fmt.Errorf("model not found: %s", modelID)
}

// listModels 获取 OpenRouter 完整模型列表（内部方法）
func (p *OpenRouterProvider) listModels() ([]OpenRouterModelInfo, error) {
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.httpPool.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []OpenRouterModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse models list: %w", err)
	}
	return result.Data, nil
}

// toProviderModel 将 OpenRouter 模型信息转换为统一的 ProviderModel
func (info *OpenRouterModelInfo) toProviderModel() *registry.ProviderModel {
	// 解析价格（OpenRouter 价格是字符串格式，如 "0.000005"）
	inputPrice := 0.0
	outputPrice := 0.0
	if info.Pricing.Prompt != "" {
		fmt.Sscanf(info.Pricing.Prompt, "%f", &inputPrice)
	}
	if info.Pricing.Completion != "" {
		fmt.Sscanf(info.Pricing.Completion, "%f", &outputPrice)
	}

	displayName := info.Name
	if displayName == "" {
		displayName = info.ID
	}

	contextWindow := info.ContextLength
	if contextWindow == 0 {
		contextWindow = 8192
	}

	maxOutput := info.MaxCompletionTokens
	if maxOutput == 0 {
		maxOutput = 4096
	}

	// 从 architecture.modality 判断能力
	// OpenRouter 的 modality 格式如 "text+image->text", "multimodal", "text"
	modality := strings.ToLower(info.Architecture.Modality)
	supportsVision := strings.Contains(modality, "image") || strings.Contains(modality, "multimodal")
	// 几乎所有 OpenRouter 上的对话模型都支持工具调用，无法从 API 精确判断时默认 true
	supportsTools := true

	return &registry.ProviderModel{
		ModelID:        info.ID,
		DisplayName:    displayName,
		OwnedBy:        extractOwnedBy(info.ID),
		ContextWindow:  contextWindow,
		MaxOutput:      maxOutput,
		InputPrice:     inputPrice,
		OutputPrice:    outputPrice,
		SupportsVision: supportsVision,
		SupportsTools:  supportsTools,
		SupportsStream: true,
		IsAvailable:    true,
		Source:         "manual",
	}
}

// extractOwnedBy 从 OpenRouter model ID 提取提供商名称
// 例如 "openai/gpt-4o" → "openai", "anthropic/claude-3" → "anthropic"
func extractOwnedBy(modelID string) string {
	if idx := strings.Index(modelID, "/"); idx > 0 {
		return modelID[:idx]
	}
	return "openrouter"
}

// LookupModelStatic 静态查找：不依赖 Provider 实例，直接调用 OpenRouter API
func LookupModelStatic(baseURL, apiKey, modelID string) (*registry.ProviderModel, error) {
	prov := NewOpenRouterProvider(&registry.Config{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
	})
	return prov.LookupModel(modelID)
}

// LookupModelsBatch 批量查找：一次获取模型列表，过滤多个模型 ID
func LookupModelsBatch(baseURL, apiKey string, modelIDs []string) ([]registry.ProviderModel, []map[string]string) {
	prov := NewOpenRouterProvider(&registry.Config{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
	})

	allModels, err := prov.listModels()
	if err != nil {
		// 全部失败
		errs := make([]map[string]string, len(modelIDs))
		for i, id := range modelIDs {
			errs[i] = map[string]string{"model_id": id, "error": err.Error()}
		}
		return nil, errs
	}

	// 建立索引
	modelMap := make(map[string]*OpenRouterModelInfo, len(allModels))
	for i := range allModels {
		modelMap[allModels[i].ID] = &allModels[i]
	}

	results := make([]registry.ProviderModel, 0, len(modelIDs))
	errors := make([]map[string]string, 0)

	for _, modelID := range modelIDs {
		info, ok := modelMap[modelID]
		if !ok {
			errors = append(errors, map[string]string{
				"model_id": modelID,
				"error":    fmt.Sprintf("model not found in OpenRouter catalog: %s", modelID),
			})
			continue
		}
		results = append(results, *info.toProviderModel())
	}

	return results, errors
}
