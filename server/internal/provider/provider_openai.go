package provider

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

type openAIUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u openAIUsage) toUsage(usage *Usage) {
	usage.CachedTokens = u.PromptTokensDetails.CachedTokens
	usage.InputTokens = u.PromptTokens - u.PromptTokensDetails.CachedTokens
	usage.OutputTokens = u.CompletionTokens
}

type OpenAIProvider struct {
	cfg *Config
}

func NewOpenAIProvider(cfg *Config) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg}
}

func (m *OpenAIProvider) SyncModels(providerID uint) ([]model.ProviderModel, error) {
	baseURL := m.cfg.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("OpenAI base URL is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

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
			ID            string    `json:"id"`
			Name          string    `json:"name"`
			Created       time.Time `json:"created"`
			Description   time.Time `json:"description"`
			ContextLength int       `json:"context_length"`
			Object        string    `json:"object"`
			OwnedBy       string    `json:"owned_by"`
			Pricing       struct {
				Completion float64 `json:"completion"`
				Prompt     float64 `json:"prompt"`
				WebSearch  float64 `json:"web_search"`
			} `json:"pricing"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := []model.ProviderModel{}
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		models = append(models, model.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    m.ID,
			OwnedBy:        m.OwnedBy,
			ContextWindow:  m.ContextLength,
			MaxOutput:      0,
			InputPrice:     m.Pricing.Prompt,
			OutputPrice:    m.Pricing.Completion,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}

	return models, nil
}

func init() {
	Register(ProtocolDescriptor{
		Name:      "openai",
		KeyPrefix: "sk-",
		KeyLength: 24,
		KeyEncoder: func(b []byte) string {
			return hex.EncodeToString(b)
		},
		AuthExtractor: func(c *gin.Context) string {
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				return ""
			}
			return strings.TrimPrefix(authHeader, "Bearer ")
		},
		ModelExtractor: func(c *gin.Context) (string, error) {
			// Read the model from JSON body
			body, err := c.GetRawData()
			if err != nil {
				return "", err
			}
			// Restore the body for downstream use
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			var req struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				return "", err
			}
			return req.Model, nil
		},
		DefaultBaseURL: "https://api.openai.com/v1",
		NewProvider: func(cfg *Config) Provider {
			return NewOpenAIProvider(cfg)
		},
	})
}

func (m *OpenAIProvider) generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

func (m *OpenAIProvider) ExecuteRequest(protocol string, ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	switch protocol {
	case "openai":
		return m.ExecuteOpenAIRequest(ctx, pm, usage)
	case "anthropic":
		return m.ExecuteAnthropicRequest(ctx, pm, usage)
	case "gemini":
		return m.ExecuteGeminiRequest(ctx, pm, usage)
	default:
		return fmt.Errorf("protocol %s not supported by OpenAI provider", protocol)
	}
}
