package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
	"ai-gateway/internal/router"
)

type ProviderHandler struct{}

type createProviderRequest struct {
	Name             string `json:"name" binding:"required"`
	OpenAIBaseURL    string `json:"openai_base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	GeminiBaseURL    string `json:"gemini_base_url"`
	DeepSeekBaseURL  string `json:"deepseek_base_url"`
	APIKey           string `json:"api_key" binding:"required"`
	Priority         int    `json:"priority"`
}

type updateProviderRequest struct {
	Name             string  `json:"name"`
	OpenAIBaseURL    *string `json:"openai_base_url"`
	AnthropicBaseURL *string `json:"anthropic_base_url"`
	GeminiBaseURL    *string `json:"gemini_base_url"`
	DeepSeekBaseURL  *string `json:"deepseek_base_url"`
	APIKey           string  `json:"api_key"`
	Enabled          *bool   `json:"enabled"`
	Priority         *int    `json:"priority"`
}

type providerResponse struct {
	ID               uint                    `json:"id"`
	Slug             string                  `json:"slug"`
	Name             string                  `json:"name"`
	OpenAIBaseURL    string                  `json:"openai_base_url"`
	AnthropicBaseURL string                  `json:"anthropic_base_url"`
	GeminiBaseURL    string                  `json:"gemini_base_url"`
	DeepSeekBaseURL  string                  `json:"deepseek_base_url"`
	APIKeyMasked     string                  `json:"api_key_masked"`
	Enabled          bool                    `json:"enabled"`
	Priority         int                     `json:"priority"`
	Models           []providerModelResponse `json:"models,omitempty"`
	CreatedAt        string                  `json:"created_at"`
}

func NewProviderHandler() *ProviderHandler {
	return &ProviderHandler{}
}

func (h *ProviderHandler) List(c *gin.Context) {
	var providers []model.Provider
	if err := model.DB.Preload("Models").Find(&providers).Error; err != nil {
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
			ID:               p.ID,
			Slug:             p.Slug,
			Name:             p.Name,
			OpenAIBaseURL:    p.OpenAIBaseURL,
			AnthropicBaseURL: p.AnthropicBaseURL,
			GeminiBaseURL:    p.GeminiBaseURL,
			DeepSeekBaseURL:  p.DeepSeekBaseURL,
			APIKeyMasked:     maskAPIKey(p.APIKey),
			Enabled:          p.Enabled,
			Priority:         p.Priority,
			Models:           models,
			CreatedAt:        p.CreatedAt.Format("2006-01-02 15:04:05"),
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
		ID:               provider.ID,
		Name:             provider.Name,
		OpenAIBaseURL:    provider.OpenAIBaseURL,
		AnthropicBaseURL: provider.AnthropicBaseURL,
		GeminiBaseURL:    provider.GeminiBaseURL,
		DeepSeekBaseURL:  provider.DeepSeekBaseURL,
		APIKeyMasked:     maskAPIKey(provider.APIKey),
		Enabled:          provider.Enabled,
		Priority:         provider.Priority,
		Models:           models,
		CreatedAt:        provider.CreatedAt.Format("2006-01-02 15:04:05"),
	}})
}

func (h *ProviderHandler) Create(c *gin.Context) {
	var req createProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	provider := model.Provider{
		Name:             req.Name,
		OpenAIBaseURL:    strings.TrimSuffix(req.OpenAIBaseURL, "/"),
		AnthropicBaseURL: strings.TrimSuffix(req.AnthropicBaseURL, "/"),
		GeminiBaseURL:    strings.TrimSuffix(req.GeminiBaseURL, "/"),
		DeepSeekBaseURL:  strings.TrimSuffix(req.DeepSeekBaseURL, "/"),
		APIKey:           req.APIKey,
		Enabled:          true,
		Priority:         req.Priority,
	}

	if provider.OpenAIBaseURL == "" && provider.AnthropicBaseURL == "" && provider.GeminiBaseURL == "" && provider.DeepSeekBaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one base URL is required"})
		return
	}

	if err := model.DB.Create(&provider).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"provider": providerResponse{
		ID:               provider.ID,
		Name:             provider.Name,
		OpenAIBaseURL:    provider.OpenAIBaseURL,
		AnthropicBaseURL: provider.AnthropicBaseURL,
		GeminiBaseURL:    provider.GeminiBaseURL,
		DeepSeekBaseURL:  provider.DeepSeekBaseURL,
		APIKeyMasked:     maskAPIKey(provider.APIKey),
		Enabled:          provider.Enabled,
		Priority:         provider.Priority,
		CreatedAt:        provider.CreatedAt.Format("2006-01-02 15:04:05"),
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

	// 允许更新 BaseURL（包括清空）
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

	if req.APIKey != "" {
		updates["api_key"] = req.APIKey
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}

	if req.OpenAIBaseURL != nil || req.AnthropicBaseURL != nil || req.GeminiBaseURL != nil || req.DeepSeekBaseURL != nil || req.APIKey != "" || req.Enabled != nil {
		router.ClearAllCooldownsForProvider(provider.ID)
	}

	// 验证更新后至少有一个 BaseURL
	newOpenAIBaseURL := provider.OpenAIBaseURL
	newAnthropicBaseURL := provider.AnthropicBaseURL
	newGeminiBaseURL := provider.GeminiBaseURL
	newDeepSeekBaseURL := provider.DeepSeekBaseURL
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
	if newOpenAIBaseURL == "" && newAnthropicBaseURL == "" && newGeminiBaseURL == "" && newDeepSeekBaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one base URL is required"})
		return
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
		ID:               provider.ID,
		Name:             provider.Name,
		OpenAIBaseURL:    provider.OpenAIBaseURL,
		AnthropicBaseURL: provider.AnthropicBaseURL,
		GeminiBaseURL:    provider.GeminiBaseURL,
		DeepSeekBaseURL:  provider.DeepSeekBaseURL,
		APIKeyMasked:     maskAPIKey(provider.APIKey),
		Enabled:          provider.Enabled,
		Priority:         provider.Priority,
		Models:           models,
		CreatedAt:        provider.CreatedAt.Format("2006-01-02 15:04:05"),
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
	OpenAIBaseURL    string `json:"openai_base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	GeminiBaseURL    string `json:"gemini_base_url"`
	DeepSeekBaseURL  string `json:"deepseek_base_url"`
	APIKey           string `json:"api_key"`
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
		}
		if existing.ID > 0 {
			apiKey = existing.APIKey
		}
	}

	models, err := protocolsPkg.AutoSyncModels(0, openaiURL, anthropicURL, geminiURL, deepseekURL, apiKey)
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

	models, err := protocolsPkg.AutoSyncModels(provider.ID,
		provider.OpenAIBaseURL,
		provider.AnthropicBaseURL,
		provider.GeminiBaseURL,
		provider.DeepSeekBaseURL,
		provider.APIKey,
	)
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
