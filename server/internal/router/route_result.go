package router

import (
	"encoding/json"

	"ai-gateway/internal/model"
	"ai-gateway/internal/provider"
)

type RouteResult struct {
	Provider         *model.Provider
	ProviderModel    *model.ProviderModel
	ProviderInstance provider.Provider
}

// SupportProtocol 动态检查是否支持指定协议（优先用 Endpoints JSON）
func (r *RouteResult) SupportProtocol(protocol string) bool {
	if r.Provider.Endpoints != "" {
		var eps map[string]string
		if json.Unmarshal([]byte(r.Provider.Endpoints), &eps) == nil {
			if url, ok := eps[protocol]; ok && url != "" {
				return true
			}
		}
	}
	// 兼容旧字段
	switch protocol {
	case "openai":
		return r.Provider.OpenAIBaseURL != ""
	case "anthropic":
		return r.Provider.AnthropicBaseURL != ""
	case "gemini":
		return r.Provider.GeminiBaseURL != ""
	}
	return false
}

// GetProviderProtocols 获取所有支持的协议列表
func (r *RouteResult) GetProviderProtocols() []string {
	protocols := []string{}
	if r.Provider.Endpoints != "" {
		var eps map[string]string
		if json.Unmarshal([]byte(r.Provider.Endpoints), &eps) == nil {
			for name, url := range eps {
				if url != "" {
					protocols = append(protocols, name)
				}
			}
			return protocols
		}
	}
	// 兼容旧字段
	if r.Provider.OpenAIBaseURL != "" {
		protocols = append(protocols, "openai")
	}
	if r.Provider.AnthropicBaseURL != "" {
		protocols = append(protocols, "anthropic")
	}
	if r.Provider.GeminiBaseURL != "" {
		protocols = append(protocols, "gemini")
	}
	return protocols
}

func (r *RouteResult) SupportOpenAI() bool {
	return r.Provider.OpenAIBaseURL != ""
}

func (r *RouteResult) SupportAnthropic() bool {
	return r.Provider.AnthropicBaseURL != ""
}

func (r *RouteResult) SupportGemini() bool {
	return r.Provider.GeminiBaseURL != ""
}
