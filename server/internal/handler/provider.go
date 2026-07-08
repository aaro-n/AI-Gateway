package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
	"ai-gateway/internal/protocols/openrouter"
	"ai-gateway/internal/router"
)

type ProviderHandler struct{}

type createProviderRequest struct {
	Name      string            `json:"name" binding:"required"`
	Endpoints map[string]string `json:"endpoints"`
	APIKey    string            `json:"api_key" binding:"required"`
	Priority  int               `json:"priority"`
}

type updateProviderRequest struct {
	Name              string            `json:"name"`
	OpenAIBaseURL     *string           `json:"openai_base_url"`
	AnthropicBaseURL  *string           `json:"anthropic_base_url"`
	GeminiBaseURL     *string           `json:"gemini_base_url"`
	DeepSeekBaseURL   *string           `json:"deepseek_base_url"`
	OpenRouterBaseURL *string           `json:"openrouter_base_url"`
	Endpoints         map[string]string `json:"endpoints"` // 新：多协议端点统一管理
	APIKey            string            `json:"api_key"`
	Enabled           *bool             `json:"enabled"`
	Priority          *int              `json:"priority"`
}

type providerResponse struct {
	ID           uint                    `json:"id"`
	Slug         string                  `json:"slug"`
	Name         string                  `json:"name"`
	Endpoints    map[string]string       `json:"endpoints"`
	APIKeyMasked string                  `json:"api_key_masked"`
	Enabled      bool                    `json:"enabled"`
	Priority     int                     `json:"priority"`
	Models       []providerModelResponse `json:"models,omitempty"`
	CreatedAt    string                  `json:"created_at"`
}

func NewProviderHandler() *ProviderHandler {
	return &ProviderHandler{}
}

func buildEndpoints(eps map[string]string) string {
	if eps == nil {
		eps = make(map[string]string)
	}
	trimmed := make(map[string]string, len(eps))
	for k, v := range eps {
		trimmed[k] = strings.TrimSuffix(v, "/")
	}
	if len(trimmed) == 0 {
		return ""
	}
	b, _ := json.Marshal(trimmed)
	return string(b)
}

func (h *ProviderHandler) List(c *gin.Context) {
	var providers []model.Provider
	if err := model.DB.Preload("Models").Order("name ASC").Find(&providers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]providerResponse, len(providers))
	for i, p := range providers {
		models := make([]providerModelResponse, len(p.Models))
		for j, m := range p.Models {
			models[j] = toProviderModelResponse(m)
		}

		result[i] = providerResponse{
			ID:           p.ID,
			Slug:         p.Slug,
			Name:         p.Name,
			Endpoints:    p.EndpointsMap(),
			APIKeyMasked: maskAPIKey(p.APIKey),
			Enabled:      p.Enabled,
			Priority:     p.Priority,
			Models:       models,
			CreatedAt:    p.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{"providers": result})
}

func (h *ProviderHandler) Get(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var provider model.Provider
	if err := model.DB.Preload("Models").First(&provider, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	models := make([]providerModelResponse, len(provider.Models))
	for j, m := range provider.Models {
		models[j] = toProviderModelResponse(m)
	}

	c.JSON(http.StatusOK, gin.H{"provider": providerResponse{
		ID:           provider.ID,
		Slug:         provider.Slug,
		Name:         provider.Name,
		Endpoints:    provider.EndpointsMap(),
		APIKeyMasked: maskAPIKey(provider.APIKey),
		Enabled:      provider.Enabled,
		Priority:     provider.Priority,
		Models:       models,
		CreatedAt:    provider.CreatedAt.Format("2006-01-02 15:04:05"),
	}})
}

func (h *ProviderHandler) Create(c *gin.Context) {
	var req createProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	provider := model.Provider{
		Name:      req.Name,
		Endpoints: buildEndpoints(req.Endpoints),
		APIKey:    req.APIKey,
		Enabled:   true,
		Priority:  req.Priority,
	}

	if provider.Endpoints == "" || provider.Endpoints == "null" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one base URL is required"})
		return
	}

	if err := model.DB.Create(&provider).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"provider": providerResponse{
		ID:           provider.ID,
		Slug:         provider.Slug,
		Name:         provider.Name,
		Endpoints:    provider.EndpointsMap(),
		APIKeyMasked: maskAPIKey(provider.APIKey),
		Enabled:      provider.Enabled,
		Priority:     provider.Priority,
		CreatedAt:    provider.CreatedAt.Format("2006-01-02 15:04:05"),
	}})
}

func (h *ProviderHandler) Update(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var provider model.Provider
	if err := model.DB.First(&provider, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	var req updateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}

	// 新 Endpoints JSON 统一管理（优先于扁平列）
	if req.Endpoints != nil {
		updates["endpoints"] = buildEndpoints(req.Endpoints)
	}

	// 允许更新 BaseURL（包括清空）— 向后兼容旧的扁平列
	if req.OpenAIBaseURL != nil {
		updates["openai_base_url"] = strings.TrimSuffix(*req.OpenAIBaseURL, "/")
	}
	if req.AnthropicBaseURL != nil {
		updates["anthropic_base_url"] = strings.TrimSuffix(*req.AnthropicBaseURL, "/")
	}
	if req.GeminiBaseURL != nil {
		updates["gemini_base_url"] = strings.TrimSuffix(*req.GeminiBaseURL, "/")
	}
	if req.DeepSeekBaseURL != nil {
		updates["deepseek_base_url"] = strings.TrimSuffix(*req.DeepSeekBaseURL, "/")
	}
	if req.OpenRouterBaseURL != nil {
		updates["openrouter_base_url"] = strings.TrimSuffix(*req.OpenRouterBaseURL, "/")
	}

	if req.APIKey != "" {
		updates["api_key"] = req.APIKey
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}

	if req.OpenAIBaseURL != nil || req.AnthropicBaseURL != nil || req.GeminiBaseURL != nil || req.DeepSeekBaseURL != nil || req.OpenRouterBaseURL != nil || req.APIKey != "" || req.Enabled != nil {
		router.ClearAllCooldownsForProvider(provider.ID)
	}

	// 验证更新后至少有一个 BaseURL（检查 endpoints JSON 和扁平列）
	hasEndpoints := false

	// 检查新的 endpoints JSON
	if endpointsJSON, ok := updates["endpoints"].(string); ok && endpointsJSON != "" && endpointsJSON != "null" {
		var eps map[string]string
		if json.Unmarshal([]byte(endpointsJSON), &eps) == nil {
			for _, url := range eps {
				if url != "" {
					hasEndpoints = true
					break
				}
			}
		}
	}

	// 检查扁平列（向后兼容）
	newOpenAIBaseURL := provider.OpenAIBaseURL
	newAnthropicBaseURL := provider.AnthropicBaseURL
	newGeminiBaseURL := provider.GeminiBaseURL
	newDeepSeekBaseURL := provider.DeepSeekBaseURL
	newOpenRouterBaseURL := provider.OpenRouterBaseURL
	if openaiURL, ok := updates["openai_base_url"].(string); ok {
		newOpenAIBaseURL = openaiURL
	}
	if anthropicURL, ok := updates["anthropic_base_url"].(string); ok {
		newAnthropicBaseURL = anthropicURL
	}
	if geminiURL, ok := updates["gemini_base_url"].(string); ok {
		newGeminiBaseURL = geminiURL
	}
	if deepseekURL, ok := updates["deepseek_base_url"].(string); ok {
		newDeepSeekBaseURL = deepseekURL
	}
	if openrouterURL, ok := updates["openrouter_base_url"].(string); ok {
		newOpenRouterBaseURL = openrouterURL
	}
	if newOpenAIBaseURL == "" && newAnthropicBaseURL == "" && newGeminiBaseURL == "" && newDeepSeekBaseURL == "" && newOpenRouterBaseURL == "" && !hasEndpoints {
		// 如果 endpoints JSON 也没被更新，检查现有 provider 的 endpoints
		if provider.Endpoints == "" || provider.Endpoints == "null" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one base URL is required"})
			return
		}
	}

	if err := model.DB.Model(&provider).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	model.DB.Preload("Models").First(&provider, id)

	models := make([]providerModelResponse, len(provider.Models))
	for j, m := range provider.Models {
		models[j] = toProviderModelResponse(m)
	}

	c.JSON(http.StatusOK, gin.H{"provider": providerResponse{
		ID:           provider.ID,
		Slug:         provider.Slug,
		Name:         provider.Name,
		Endpoints:    provider.EndpointsMap(),
		APIKeyMasked: maskAPIKey(provider.APIKey),
		Enabled:      provider.Enabled,
		Priority:     provider.Priority,
		Models:       models,
		CreatedAt:    provider.CreatedAt.Format("2006-01-02 15:04:05"),
	}})
}

func (h *ProviderHandler) Delete(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 使用事务确保数据一致性
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var providerModelIDs []uint
		tx.Model(&model.ProviderModel{}).Where("provider_id = ?", id).Pluck("id", &providerModelIDs)

		if len(providerModelIDs) > 0 {
			if err := tx.Where("provider_model_id IN ?", providerModelIDs).Delete(&model.ModelMapping{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("provider_id = ?", id).Delete(&model.ProviderModel{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&model.Provider{}, id).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete provider"})
		return
	}

	router.ClearAllCooldownsForProvider(uint(id))

	c.JSON(http.StatusOK, gin.H{"message": "provider deleted"})
}

type testConnectionRequest struct {
	OpenAIBaseURL     string            `json:"openai_base_url"`
	AnthropicBaseURL  string            `json:"anthropic_base_url"`
	GeminiBaseURL     string            `json:"gemini_base_url"`
	DeepSeekBaseURL   string            `json:"deepseek_base_url"`
	OpenRouterBaseURL string            `json:"openrouter_base_url"`
	Endpoints         map[string]string `json:"endpoints"`
	APIKey            string            `json:"api_key"`
}

func (h *ProviderHandler) TestConnection(c *gin.Context) {
	var req testConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	openaiURL := strings.TrimSuffix(req.OpenAIBaseURL, "/")
	anthropicURL := strings.TrimSuffix(req.AnthropicBaseURL, "/")
	geminiURL := strings.TrimSuffix(req.GeminiBaseURL, "/")
	deepseekURL := strings.TrimSuffix(req.DeepSeekBaseURL, "/")
	openrouterURL := strings.TrimSuffix(req.OpenRouterBaseURL, "/")
	apiKey := req.APIKey

	if apiKey == "DUMMY_KEY_FOR_EDIT" || apiKey == "" {
		var existing model.Provider
		if openaiURL != "" {
			model.DB.Where("openai_base_url = ?", openaiURL).First(&existing)
		} else if anthropicURL != "" {
			model.DB.Where("anthropic_base_url = ?", anthropicURL).First(&existing)
		} else if geminiURL != "" {
			model.DB.Where("gemini_base_url = ?", geminiURL).First(&existing)
		} else if deepseekURL != "" {
			model.DB.Where("deepseek_base_url = ?", deepseekURL).First(&existing)
		} else if openrouterURL != "" {
			model.DB.Where("openrouter_base_url = ?", openrouterURL).First(&existing)
		}
		if existing.ID > 0 {
			apiKey = existing.APIKey
		}
	}

	// OpenRouter 不支持批量同步，单独处理连接测试
	if openrouterURL != "" && openaiURL == "" && anthropicURL == "" && geminiURL == "" && deepseekURL == "" {
		_, err := openrouter.LookupModelStatic(openrouterURL, apiKey, "openai/gpt-3.5-turbo")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Connection test failed: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"models": []interface{}{}, "total": 0, "note": "OpenRouter connection successful. Use model lookup to fetch individual model details."})
		return
	}

	models, err := protocolsPkg.AutoSyncModels(0, map[string]string{"openai": openaiURL, "anthropic": anthropicURL, "gemini": geminiURL, "deepseek": deepseekURL}, apiKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Connection test failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Connection test successful!",
		"models":  models,
	})
}

func (h *ProviderHandler) Test(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var provider model.Provider
	if err := model.DB.First(&provider, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	models, err := protocolsPkg.AutoSyncModels(provider.ID, provider.EndpointsMap(), provider.APIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Connection test failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Connection test successful!",
		"models":  models,
	})
}

type protocolMetaResponse struct {
	Name           string `json:"name"`
	KeyPrefix      string `json:"key_prefix"`
	DefaultBaseURL string `json:"default_base_url"`
}

func (h *ProviderHandler) GetProtocolsMeta(c *gin.Context) {
	all := registry.All()
	result := make([]protocolMetaResponse, 0, len(all))
	for _, p := range all {
		result = append(result, protocolMetaResponse{
			Name:           p.Name,
			KeyPrefix:      p.KeyPrefix,
			DefaultBaseURL: p.DefaultBaseURL,
		})
	}
	c.JSON(http.StatusOK, gin.H{"protocols": result})
}

func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
