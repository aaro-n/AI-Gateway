// Package protocols 提供按优先级自动回退的智能模型同步。
package protocols

import (
	"fmt"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/model"
)

// AutoSyncModels 按优先级遍历已配置端点，使用核心注册表的新 Provider 接口执行 SyncModels。
func AutoSyncModels(providerID uint, endpoints map[string]string, apiKey string) ([]model.ProviderModel, error) {
	type endpoint struct {
		name    string
		baseURL string
	}

	// 优先级: Gemini → Anthropic → OpenAI → DeepSeek
	priorityOrder := []string{"gemini", "anthropic", "openai", "deepseek"}

	var lastErr error

	// 1. 优先尝试用户已配置的自定义端点
	for _, name := range priorityOrder {
		baseURL, ok := endpoints[name]
		if !ok || baseURL == "" {
			continue
		}
		desc, ok2 := registry.Get(name)
		if !ok2 || desc.NewProvider == nil {
			continue
		}
		prov := desc.NewProvider(&registry.Config{
			BaseURL: baseURL,
			APIKey:  apiKey,
		})
		registryModels, err := prov.SyncModels(providerID)
		if err != nil {
			lastErr = err
			continue
		}
		if len(registryModels) == 0 {
			lastErr = fmt.Errorf("provider %s returned empty model list", name)
			continue
		}
		return convertModels(registryModels), nil
	}

	// 2. 兜底：对已配置的协议尝试使用 DefaultBaseURL
	for _, name := range priorityOrder {
		if _, ok := endpoints[name]; !ok || endpoints[name] == "" {
			continue // 只 fallback 用户明确配置了的协议
		}
		desc, ok := registry.Get(name)
		if !ok || desc.NewProvider == nil || desc.DefaultBaseURL == "" {
			continue
		}
		prov := desc.NewProvider(&registry.Config{
			BaseURL: desc.DefaultBaseURL,
			APIKey:  apiKey,
		})
		registryModels, err := prov.SyncModels(providerID)
		if err != nil {
			lastErr = err
			continue
		}
		if len(registryModels) == 0 {
			lastErr = fmt.Errorf("provider %s returned empty model list (default URL)", name)
			continue
		}
		return convertModels(registryModels), nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no valid models found: %v", lastErr)
	}
	return nil, fmt.Errorf("no valid models found")
}

func convertModels(registryModels []registry.ProviderModel) []model.ProviderModel {
	models := make([]model.ProviderModel, len(registryModels))
	for i, rm := range registryModels {
		models[i] = model.ProviderModel{
			ProviderID:     rm.ProviderID,
			ModelID:        rm.ModelID,
			DisplayName:    rm.DisplayName,
			OwnedBy:        rm.OwnedBy,
			ContextWindow:  rm.ContextWindow,
			MaxOutput:      rm.MaxOutput,
			InputPrice:     rm.InputPrice,
			OutputPrice:    rm.OutputPrice,
			SupportsVision: rm.SupportsVision,
			SupportsTools:  rm.SupportsTools,
			SupportsStream: rm.SupportsStream,
			IsAvailable:    rm.IsAvailable,
			Source:         rm.Source,
		}
	}
	return models
}
