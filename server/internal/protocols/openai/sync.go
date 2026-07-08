package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-gateway/internal/core/registry"
)

// SyncModels — 从 OpenAI API 同步模型列表
// =============================================================================

type openAIModelEntry struct {
	ID                  string `json:"id"`
	OwnedBy             string `json:"owned_by"`
	DisplayName         string `json:"display_name"`          // 某些兼容 API 返回
	ContextLength       int    `json:"context_length"`        // 上下文窗口
	MaxInputTokens      int    `json:"max_input_tokens"`      // 别名
	MaxTokens           int    `json:"max_tokens"`            // 最大输出（别名）
	MaxCompletionTokens int    `json:"max_completion_tokens"` // 最大输出（OpenAI 标准）
	Pricing             struct {
		Completion float64 `json:"completion"`
		Prompt     float64 `json:"prompt"`
	} `json:"pricing"`
	Capabilities *struct {
		Vision    bool `json:"vision"`
		Streaming bool `json:"streaming"`
	} `json:"capabilities,omitempty"`
}

// openAIModelDefaults 为 OpenAI 标准 API 不返回 context_window 的情况提供默认值。
// 数据来源：https://platform.openai.com/docs/models (Core Generative Models)
// 不在本表中的模型同步时会被过滤舍弃（非核心对话模型如音频/图像/embedding 等）。
var openAIModelDefaults = map[string]struct {
	contextWindow int
	maxOutput     int
	vision        bool
}{
	// GPT-5.2 系列
	"gpt-5.2-pro": {400000, 128000, true},
	"gpt-5.2":     {400000, 128000, true},
	// GPT-5.1 系列
	"gpt-5.1":           {400000, 128000, true},
	"gpt-5.1-codex-max": {400000, 128000, true},
	"gpt-5.1-codex":     {400000, 128000, true},
	// GPT-5 系列
	"gpt-5":       {400000, 128000, true},
	"gpt-5-codex": {400000, 128000, true},
	"gpt-5-mini":  {400000, 128000, true},
	"gpt-5-nano":  {400000, 128000, true},
	// GPT-4.1 系列
	"gpt-4.1":      {1047576, 32768, true},
	"gpt-4.1-mini": {1047576, 32768, true},
	"gpt-4.1-nano": {1047576, 32768, true},
	// GPT-4o 系列
	"gpt-4o":      {128000, 16384, true},
	"gpt-4o-mini": {128000, 16384, true},
	// GPT-4 系列
	"gpt-4":       {8192, 8192, false},
	"gpt-4-turbo": {128000, 4096, true},
	// GPT-3.5
	"gpt-3.5-turbo": {16385, 4096, false},
	// o 系列推理模型
	"o1":         {200000, 100000, true},
	"o1-mini":    {128000, 65536, false},
	"o1-preview": {128000, 32768, false},
	"o1-pro":     {200000, 100000, true},
	"o3":         {200000, 100000, true},
	"o3-mini":    {200000, 100000, false},
	"o4-mini":    {200000, 100000, true},
}

func (p *OpenAIProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
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
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var result struct {
		Data []openAIModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		// 跳过非 chat 模型（embedding、moderation、tts 等）
		if strings.Contains(m.ID, "embedding") || strings.Contains(m.ID, "moderation") ||
			strings.Contains(m.ID, "whisper") || strings.Contains(m.ID, "tts") ||
			strings.Contains(m.ID, "dall-e") || strings.Contains(m.ID, "davinci") {
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}

		// 上下文窗口：多种字段名兼容
		contextWindow := m.ContextLength
		if contextWindow == 0 {
			contextWindow = m.MaxInputTokens
		}

		// 最大输出：优先 max_completion_tokens，其次 max_tokens
		maxOutput := m.MaxCompletionTokens
		if maxOutput == 0 {
			maxOutput = m.MaxTokens
		}

		// 视觉支持：从 capabilities 读取
		supportsVision := false
		if m.Capabilities != nil {
			supportsVision = m.Capabilities.Vision
		}

		// 核心对话模型白名单：用官方默认值填充 API 未返回的字段，
		// 不在白名单中的模型（音频/图像/embedding/旧版快照等）直接舍弃
		def, inDefaults := openAIModelDefaults[m.ID]
		if !inDefaults {
			continue
		}

		if contextWindow == 0 {
			contextWindow = def.contextWindow
		}
		if maxOutput == 0 {
			maxOutput = def.maxOutput
		}
		if !supportsVision {
			supportsVision = def.vision
		}

		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        m.OwnedBy,
			ContextWindow:  contextWindow,
			MaxOutput:      maxOutput,
			InputPrice:     m.Pricing.Prompt,
			OutputPrice:    m.Pricing.Completion,
			SupportsVision: supportsVision,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================

