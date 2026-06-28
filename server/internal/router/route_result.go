package router

import (
	"encoding/json"

	"ai-gateway/internal/model"
)

// RouteResult 路由结果。
// 注意：不再持有 ProviderInstance（旧 provider 包的实例），
// 上游协议的执行由 UnifiedGatewayHandler 通过 registry 动态创建。
type RouteResult struct {
	Provider      *model.Provider
	ProviderModel *model.ProviderModel
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
