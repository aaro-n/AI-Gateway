package provider

import (
	"ai-gateway/internal/model"
	"fmt"

	"github.com/gin-gonic/gin"
)

type AutomatedProvider struct {
	providers map[string]Provider
}

func NewAutomatedProvider(openAIBaseURL string, anthropicBaseUrl string, geminiBaseURL string, apiKey string) *AutomatedProvider {
	p := AutomatedProvider{
		providers: make(map[string]Provider),
	}

	endpoints := map[string]string{
		"openai":    openAIBaseURL,
		"anthropic": anthropicBaseUrl,
		"gemini":    geminiBaseURL,
	}

	for name, url := range endpoints {
		if url != "" {
			if desc, ok := GetProtocol(name); ok && desc.NewProvider != nil {
				p.providers[name] = desc.NewProvider(&Config{
					APIKey:  apiKey,
					BaseURL: url,
				})
			}
		}
	}
	return &p
}

func (p *AutomatedProvider) SyncModels(providerID uint) ([]model.ProviderModel, error) {
	order := []string{"gemini", "anthropic", "openai"}
	for _, name := range order {
		if prov, ok := p.providers[name]; ok && prov != nil {
			if models, err := prov.SyncModels(providerID); err == nil && models != nil {
				return models, nil
			}
		}
	}

	for _, prov := range p.providers {
		if prov != nil {
			if models, err := prov.SyncModels(providerID); err == nil && models != nil {
				return models, nil
			}
		}
	}
	return nil, fmt.Errorf("no valid models found")
}

func (p *AutomatedProvider) ExecuteOpenAIRequest(ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	finalProvider := p.providers["openai"]
	if finalProvider == nil {
		finalProvider = p.providers["anthropic"]
	}
	if finalProvider == nil {
		finalProvider = p.providers["gemini"]
	}
	if finalProvider == nil {
		return fmt.Errorf("no available provider to handle OpenAI request")
	}
	return finalProvider.ExecuteOpenAIRequest(ctx, pm, usage)
}

func (p *AutomatedProvider) ExecuteAnthropicRequest(ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	finalProvider := p.providers["anthropic"]
	if finalProvider == nil {
		finalProvider = p.providers["openai"]
	}
	if finalProvider == nil {
		finalProvider = p.providers["gemini"]
	}
	if finalProvider == nil {
		return fmt.Errorf("no available provider to handle Anthropic request")
	}
	return finalProvider.ExecuteAnthropicRequest(ctx, pm, usage)
}

func (p *AutomatedProvider) ExecuteGeminiRequest(ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	finalProvider := p.providers["gemini"]
	if finalProvider == nil {
		finalProvider = p.providers["openai"]
	}
	if finalProvider == nil {
		finalProvider = p.providers["anthropic"]
	}
	if finalProvider == nil {
		return fmt.Errorf("no available provider to handle Gemini request")
	}
	return finalProvider.ExecuteGeminiRequest(ctx, pm, usage)
}

func (p *AutomatedProvider) ExecuteRequest(protocol string, ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	switch protocol {
	case "openai":
		return p.ExecuteOpenAIRequest(ctx, pm, usage)
	case "anthropic":
		return p.ExecuteAnthropicRequest(ctx, pm, usage)
	case "gemini":
		return p.ExecuteGeminiRequest(ctx, pm, usage)
	default:
		return fmt.Errorf("protocol %s not supported by AutomatedProvider", protocol)
	}
}
