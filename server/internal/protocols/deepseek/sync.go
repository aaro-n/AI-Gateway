package deepseek

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-gateway/internal/core/registry"
)

// =============================================================================
// SyncModels — 从 DeepSeek API 同步模型列表
// =============================================================================

type deepseekModelEntry struct {
	ID                  string `json:"id"`
	OwnedBy             string `json:"owned_by"`
	DisplayName         string `json:"display_name"`
	ContextLength       int    `json:"context_length"`
	MaxInputTokens      int    `json:"max_input_tokens"`
	MaxTokens           int    `json:"max_tokens"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
}

func (p *DeepSeekProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.httpPool.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DeepSeek API error: %s", string(body))
	}

	var result struct {
		Data []deepseekModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		// 跳过非 chat 模型
		if strings.Contains(m.ID, "embedding") || strings.Contains(m.ID, "moderation") ||
			strings.Contains(m.ID, "whisper") || strings.Contains(m.ID, "tts") {
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}

		contextWindow := m.ContextLength
		if contextWindow == 0 {
			contextWindow = m.MaxInputTokens
		}

		maxOutput := m.MaxCompletionTokens
		if maxOutput == 0 {
			maxOutput = m.MaxTokens
		}

		// 兜底：API 未返回时使用已知模型默认值
		if contextWindow == 0 || maxOutput == 0 {
			if def, ok := deepseekModelDefaults[m.ID]; ok {
				if contextWindow == 0 {
					contextWindow = def.contextWindow
				}
				if maxOutput == 0 {
					maxOutput = def.maxOutput
				}
			}
		}

		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        m.OwnedBy,
			ContextWindow:  contextWindow,
			MaxOutput:      maxOutput,
			SupportsVision: detectDeepSeekVision(m.ID),
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}


