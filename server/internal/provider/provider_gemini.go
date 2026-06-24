package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

type GeminiProvider struct {
	cfg *Config
}

func NewGeminiProvider(cfg *Config) *GeminiProvider {
	return &GeminiProvider{cfg: cfg}
}

// SyncModels synchronizes Gemini models
func (m *GeminiProvider) SyncModels(providerID uint) ([]model.ProviderModel, error) {
	baseURL := m.cfg.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("Gemini base URL is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := fmt.Sprintf("%s/models?key=%s", baseURL, m.cfg.APIKey)

	// DEBUG LOG
	fmt.Printf("[Gemini Debug] SyncModels URL: %s, APIKey length: %d\n", url, len(m.cfg.APIKey))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[Gemini Debug] SyncModels request error: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[Gemini Debug] SyncModels status failed: %d, response: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			Version                    string   `json:"version"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := []model.ProviderModel{}
	for _, rawModel := range result.Models {
		if rawModel.Name == "" {
			continue
		}
		// Skip if does not support generateContent
		supportsGenerate := false
		for _, method := range rawModel.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}

		modelID := strings.TrimPrefix(rawModel.Name, "models/")
		models = append(models, model.ProviderModel{
			ProviderID:     providerID,
			ModelID:        modelID,
			DisplayName:    rawModel.DisplayName,
			OwnedBy:        "google",
			ContextWindow:  rawModel.InputTokenLimit,
			MaxOutput:      rawModel.OutputTokenLimit,
			SupportsVision: true,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}

	return models, nil
}

// ExecuteRequest routes requests of any protocol format supported by Gemini provider
func (m *GeminiProvider) ExecuteRequest(protocol string, ctx *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	switch protocol {
	case "gemini":
		return m.ExecuteGeminiRequest(ctx, pm, usage)
	case "openai":
		return m.ExecuteOpenAIRequest(ctx, pm, usage)
	case "anthropic":
		return m.ExecuteAnthropicRequest(ctx, pm, usage)
	default:
		return fmt.Errorf("protocol %s not supported by Gemini provider", protocol)
	}
}
