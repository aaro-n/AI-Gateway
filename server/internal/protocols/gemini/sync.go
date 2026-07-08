package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-gateway/internal/core/registry"
)

// SyncModels 从 Gemini API 拉取模型列表
func (p *GeminiProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	url := fmt.Sprintf("%s/models?key=%s", p.cfg.BaseURL, p.cfg.APIKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpPool.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API error: %s", string(body))
	}

	var result struct {
		Models []geminiRawModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Models))
	for _, m := range result.Models {
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate || m.Name == "" {
			continue
		}
		modelID := strings.TrimPrefix(m.Name, "models/")
		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        modelID,
			DisplayName:    m.DisplayName,
			OwnedBy:        "google",
			ContextWindow:  m.InputTokenLimit,
			MaxOutput:      m.OutputTokenLimit,
			SupportsVision: true,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

type geminiRawModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}
