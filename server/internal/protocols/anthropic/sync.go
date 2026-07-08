package anthropic

import (
	"context"
	"log"
	"time"

	"ai-gateway/internal/core/registry"
)

// =============================================================================
// SyncModels
// =============================================================================

func (p *AnthropicProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	// 尝试从 Anthropic Models API 获取模型规格，失败则回退硬编码
	apiSpecs, err := p.fetchModelSpecs()
	if err != nil {
		log.Printf("[Anthropic] Models API unavailable (%v), using hardcoded specs as fallback", err)
		return p.knownModels(providerID), nil
	}
	log.Printf("[Anthropic] Models API returned %d models, merging with hardcoded IDs", len(apiSpecs))
	return p.buildModels(providerID, apiSpecs), nil
}

// anthropicModelSpec represents a model's specs from the Anthropic Models API.
type anthropicModelSpec struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	MaxInput     int    `json:"max_input_tokens"`
	MaxOutput    int    `json:"max_tokens"`
	Capabilities struct {
		Vision struct {
			Supported bool `json:"supported"`
		} `json:"image_input"`
	} `json:"capabilities"`
}

func (p *AnthropicProvider) fetchModelSpecs() (map[string]*anthropicModelSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Data []anthropicModelSpec `json:"data"`
	}
	if err := p.sdk.Get(ctx, "/v1/models", nil, &result); err != nil {
		return nil, err
	}

	specs := make(map[string]*anthropicModelSpec, len(result.Data))
	for i := range result.Data {
		specs[result.Data[i].ID] = &result.Data[i]
	}
	return specs, nil
}

// knownModelIDs lists all Claude model IDs (正版无日期格式).
var knownModelIDs = []struct {
	id      string
	display string
}{
	{"claude-opus-4-8", "Claude Opus 4.8"},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6"},
	{"claude-haiku-4-5", "Claude Haiku 4.5"},
	{"claude-opus-4-7", "Claude Opus 4.7"},
	{"claude-opus-4-6", "Claude Opus 4.6"},
	{"claude-sonnet-4-5", "Claude Sonnet 4.5"},
	{"claude-opus-4-5", "Claude Opus 4.5"},
}

// hardcodedFallbacks: only for the 3 old 4.5-gen models that the API may not return.
var hardcodedFallbacks = map[string]struct {
	ctx int
	out int
	vis bool
}{
	"claude-haiku-4-5":  {200000, 64000, true},
	"claude-sonnet-4-5": {200000, 64000, true},
	"claude-opus-4-5":   {200000, 64000, true},
}

// buildModels merges known IDs with API specs. Newer models (4.6+) come from API only;
// the 3 old 4.5-gen models fall back to official hardcoded values if not in API.
func (p *AnthropicProvider) buildModels(providerID uint, specs map[string]*anthropicModelSpec) []registry.ProviderModel {
	result := make([]registry.ProviderModel, 0, len(knownModelIDs))
	for _, m := range knownModelIDs {
		var ctx, out int
		var vis bool
		name := m.display

		if s, ok := specs[m.id]; ok {
			ctx, out, vis = s.MaxInput, s.MaxOutput, s.Capabilities.Vision.Supported
			if s.DisplayName != "" {
				name = s.DisplayName
			}
		} else if fb, ok := hardcodedFallbacks[m.id]; ok {
			ctx, out, vis = fb.ctx, fb.out, fb.vis
		} else {
			continue
		}

		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.id,
			DisplayName:    name,
			OwnedBy:        "anthropic",
			ContextWindow:  ctx,
			MaxOutput:      out,
			SupportsVision: vis,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return result
}

// knownModels is the offline fallback when the API is unreachable.
// It includes ALL models with their best-known specs from official docs.
func (p *AnthropicProvider) knownModels(providerID uint) []registry.ProviderModel {
	models := []struct {
		id   string
		name string
		ctx  int
		out  int
		vis  bool
	}{
		{"claude-opus-4-8", "Claude Opus 4.8", 1000000, 128000, true},
		{"claude-sonnet-4-6", "Claude Sonnet 4.6", 1000000, 128000, true},
		{"claude-haiku-4-5", "Claude Haiku 4.5", 200000, 64000, true},
		{"claude-opus-4-7", "Claude Opus 4.7", 1000000, 128000, true},
		{"claude-opus-4-6", "Claude Opus 4.6", 1000000, 128000, true},
		{"claude-sonnet-4-5", "Claude Sonnet 4.5", 200000, 64000, true},
		{"claude-opus-4-5", "Claude Opus 4.5", 200000, 64000, true},
	}
	result := make([]registry.ProviderModel, 0, len(models))
	for _, m := range models {
		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.id,
			DisplayName:    m.name,
			OwnedBy:        "anthropic",
			ContextWindow:  m.ctx,
			MaxOutput:      m.out,
			SupportsVision: m.vis,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return result
}
