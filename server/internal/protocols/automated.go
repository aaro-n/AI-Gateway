// Package protocols 提供按优先级自动回退的智能模型同步。
package protocols

import (
	"fmt"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/model"
)

// AutoSyncModels 按优先级 (Gemini → Anthropic → OpenAI) 遍历已配置端点，
// 使用核心注册表的新 Provider 接口执行 SyncModels，返回首个成功的结果。
func AutoSyncModels(providerID uint, openAIURL, anthropicURL, geminiURL, deepseekURL, apiKey string) ([]model.ProviderModel, error) {
	type endpoint struct {
		name    string
		baseURL string
	}

	endpoints := []endpoint{
		{"gemini", geminiURL},
		{"anthropic", anthropicURL},
		{"openai", openAIURL},
		{"deepseek", deepseekURL},
	}

	for _, ep := range endpoints {
		if ep.baseURL == "" {
			continue
		}
		desc, ok := registry.Get(ep.name)
		if !ok || desc.NewProvider == nil {
			continue
		}
		prov := desc.NewProvider(&registry.Config{
			BaseURL: ep.baseURL,
			APIKey:  apiKey,
		})
		registryModels, err := prov.SyncModels(providerID)
		if err != nil || len(registryModels) == 0 {
			continue
		}
		// 转换 registry.ProviderModel → model.ProviderModel
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
		return models, nil
	}

	// 兜底：遍历所有已注册端点
	for _, ep := range endpoints {
		if ep.baseURL == "" {
			continue
		}
		desc, ok := registry.Get(ep.name)
		if !ok || desc.NewProvider == nil {
			continue
		}
		prov := desc.NewProvider(&registry.Config{
			BaseURL: ep.baseURL,
			APIKey:  apiKey,
		})
		registryModels, err := prov.SyncModels(providerID)
		if err != nil {
			continue
		}
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
		return models, nil
	}

	return nil, fmt.Errorf("no valid models found")
}
