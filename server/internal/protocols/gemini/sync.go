package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	_ "embed"

	"ai-gateway/internal/core/registry"
)

// =============================================================================
// SyncModels — API 优先，本地补齐缺失字段，API 不可用时纯本地兜底
// =============================================================================

//go:embed models.json
var localModelsJSON []byte

// modelEntry 对应 models.json 中的单条模型配置
type modelEntry struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	OwnedBy        string `json:"owned_by"`
	ContextWindow  int    `json:"context_window"`
	MaxOutput      int    `json:"max_output"`
	SupportsVision bool   `json:"supports_vision"`
	SupportsTools  bool   `json:"supports_tools"`
	SupportsStream bool   `json:"supports_stream"`
}

func loadLocalModels() (map[string]modelEntry, error) {
	var entries []modelEntry
	if err := json.Unmarshal(localModelsJSON, &entries); err != nil {
		return nil, err
	}
	m := make(map[string]modelEntry, len(entries))
	for _, e := range entries {
		m[e.ID] = e
	}
	return m, nil
}

func (p *GeminiProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	localMap, err := loadLocalModels()
	if err != nil {
		return nil, err
	}

	// 尝试 API
	apiModels, apiErr := p.fetchModels()
	if apiErr != nil {
		log.Printf("[Gemini] Models API unavailable (%v), using local models only (%d)", apiErr, len(localMap))
		return buildFromLocal(providerID, localMap), nil
	}
	log.Printf("[Gemini] API=%d models, local=%d models", len(apiModels), len(localMap))

	// --- API 成功：遍历 API 为主线，本地补齐缺失 + 本地独有的追加 ---
	result := make([]registry.ProviderModel, 0, len(apiModels)+len(localMap))
	seen := map[string]bool{}

	for _, api := range apiModels {
		if !supportsGenerate(api) || api.Name == "" {
			continue
		}
		id := strings.TrimPrefix(api.Name, "models/")
		local, hasLocal := localMap[id]

		// API 值优先；API 未返回（== 0 或 \"\"）时用本地补齐
		ctx := api.InputTokenLimit
		out := api.OutputTokenLimit
		name := api.DisplayName
		vis := hasLocal && local.SupportsVision // Gemini API 不返回 vision 字段，信任本地
		ownedBy := "google"

		if hasLocal {
			if ctx == 0 {
				ctx = local.ContextWindow
			}
			if out == 0 {
				out = local.MaxOutput
			}
			if name == "" {
				name = local.DisplayName
			}
			ownedBy = local.OwnedBy
		}
		if name == "" {
			name = id
		}

		seen[id] = true
		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        id,
			DisplayName:    name,
			OwnedBy:        ownedBy,
			ContextWindow:  ctx,
			MaxOutput:      out,
			SupportsVision: vis,
			SupportsTools:  localBool(hasLocal, local.SupportsTools, true),
			SupportsStream: localBool(hasLocal, local.SupportsStream, true),
			IsAvailable:    true,
			Source:         "sync",
		})
	}

	// 本地有但 API 没有的模型 → 追加
	for id, local := range localMap {
		if seen[id] {
			continue
		}
		result = append(result, toProviderModel(providerID, local))
	}

	return result, nil
}

// =============================================================================
// 兜底 & 工具函数
// =============================================================================

func (p *GeminiProvider) fetchModels() ([]geminiRawModel, error) {
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
	return result.Models, nil
}

func supportsGenerate(m geminiRawModel) bool {
	for _, method := range m.SupportedGenerationMethods {
		if method == "generateContent" {
			return true
		}
	}
	return false
}

func buildFromLocal(providerID uint, localMap map[string]modelEntry) []registry.ProviderModel {
	result := make([]registry.ProviderModel, 0, len(localMap))
	for _, e := range localMap {
		result = append(result, toProviderModel(providerID, e))
	}
	return result
}

func toProviderModel(providerID uint, e modelEntry) registry.ProviderModel {
	return registry.ProviderModel{
		ProviderID:     providerID,
		ModelID:        e.ID,
		DisplayName:    e.DisplayName,
		OwnedBy:        e.OwnedBy,
		ContextWindow:  e.ContextWindow,
		MaxOutput:      e.MaxOutput,
		SupportsVision: e.SupportsVision,
		SupportsTools:  e.SupportsTools,
		SupportsStream: e.SupportsStream,
		IsAvailable:    true,
		Source:         "sync",
	}
}

func localBool(hasLocal bool, localVal, fallback bool) bool {
	if hasLocal {
		return localVal
	}
	return fallback
}

type geminiRawModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}
