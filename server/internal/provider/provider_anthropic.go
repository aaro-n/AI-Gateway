package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) total() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u anthropicUsage) toUsage(usage *Usage) {
	usage.CachedTokens = u.CacheReadInputTokens
	usage.InputTokens = u.InputTokens
	usage.OutputTokens = u.OutputTokens
}

type AnthropicProvider struct {
	cfg *Config
}

func NewAnthropicProvider(cfg *Config) *AnthropicProvider {
	return &AnthropicProvider{cfg: cfg}
}

func (m *AnthropicProvider) SyncModels(providerID uint) ([]model.ProviderModel, error) {
	baseURL := m.cfg.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("Anthropic base URL is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", m.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s", string(body))
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			Type          string `json:"type"`
			DisplayName   string `json:"display_name"`
			CreatedAt     string `json:"created_at"`
			MaxInputToken int    `json:"max_input_tokens"`
			MaxTokens     int    `json:"max_tokens"`
			Capabilities  struct {
				ImageInput struct {
					Supported bool `json:"supported"`
				} `json:"image_input"`
				Thinking struct {
					Supported bool `json:"supported"`
				} `json:"thinking"`
			} `json:"capabilities"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := []model.ProviderModel{}
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}

		supportsVision := m.Capabilities.ImageInput.Supported
		supportsTools := true

		models = append(models, model.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        "anthropic",
			ContextWindow:  m.MaxInputToken,
			MaxOutput:      m.MaxTokens,
			SupportsVision: supportsVision,
			SupportsTools:  supportsTools,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}

	return models, nil
}

func (m *AnthropicProvider) isStreaming(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return len(resp.Header["Transfer-Encoding"]) > 0 ||
		(len(contentType) > 0 && len(contentType) >= 17 && contentType[:17] == "text/event-stream")
}

func (m *AnthropicProvider) ExecuteRequest(protocol string, ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	switch protocol {
	case "openai":
		return m.ExecuteOpenAIRequest(ctx, pm, usage)
	case "anthropic":
		return m.ExecuteAnthropicRequest(ctx, pm, usage)
	case "gemini":
		return m.ExecuteGeminiRequest(ctx, pm, usage)
	default:
		return fmt.Errorf("protocol %s not supported by Anthropic provider", protocol)
	}
}
