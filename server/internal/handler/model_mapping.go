package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	"ai-gateway/internal/protocols/capabilities"
)

// ── 映射相关请求类型 ──

type createMappingRequest struct {
	ProviderID      uint `json:"provider_id" binding:"required"`
	ProviderModelID uint `json:"provider_model_id" binding:"required"`
	Weight          int  `json:"weight"`
	Enabled         bool `json:"enabled"`
}

type updateMappingRequest struct {
	ProviderID      *uint `json:"provider_id"`
	ProviderModelID *uint `json:"provider_model_id"`
	Weight          *int  `json:"weight"`
	Enabled         *bool `json:"enabled"`
}

type updateMappingsOrderRequest struct {
	Order []uint `json:"order" binding:"required"`
}

// ── 映射相关响应类型 ──

type mappingResponse struct {
	ID                uint                   `json:"id"`
	ProviderID        uint                   `json:"provider_id"`
	ProviderModelID   uint                   `json:"provider_model_id"`
	ProviderModelName string                 `json:"provider_model_name"`
	Weight            int                    `json:"weight"`
	Enabled           bool                   `json:"enabled"`
	Provider          *providerBasicResponse `json:"provider,omitempty"`
	ModelInfo         *modelInfoResponse     `json:"model_info,omitempty"`
}

type modelInfoResponse struct {
	ContextWindow  int  `json:"context_window"`
	MaxOutput      int  `json:"max_output"`
	SupportsVision bool `json:"supports_vision"`
	SupportsTools  bool `json:"supports_tools"`
	SupportsStream bool `json:"supports_stream"`
}

type mappingCapabilitiesResponse struct {
	MappingID       uint                           `json:"mapping_id"`
	ProviderID      uint                           `json:"provider_id"`
	ProviderName    string                         `json:"provider_name"`
	ProviderModelID string                         `json:"provider_model_id"`
	Weight          int                            `json:"weight"`
	Enabled         bool                           `json:"enabled"`
	Protocols       []string                       `json:"protocols"`
	Capabilities    *capabilities.ComparisonResult `json:"capabilities,omitempty"`
}

// =============================================================================
// 模型映射 CRUD
// =============================================================================

// ListMappings 列出某模型的所有映射
func (h *ModelHandler) ListMappings(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var m model.Model
	if err := model.DB.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var mappings []model.ModelMapping
	model.DB.Preload("Provider").Preload("ProviderModel").
		Joins("JOIN providers ON providers.id = model_mappings.provider_id AND providers.enabled = ?", true).
		Where("model_id = ?", m.ID).
		Order("weight DESC").
		Find(&mappings)

	result := make([]mappingResponse, len(mappings))
	for i, mm := range mappings {
		result[i] = toMappingResponse(mm)
	}

	c.JSON(http.StatusOK, gin.H{"mappings": result})
}

// CreateMapping 创建模型映射
func (h *ModelHandler) CreateMapping(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var m model.Model
	if err := model.DB.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req createMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pm model.ProviderModel
	if err := model.DB.First(&pm, req.ProviderModelID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider model not found"})
		return
	}

	if pm.ProviderID != req.ProviderID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider mismatch"})
		return
	}

	var existingCount int64
	model.DB.Model(&model.ModelMapping{}).
		Where("model_id = ? AND provider_id = ? AND provider_model_id = ?", m.ID, req.ProviderID, req.ProviderModelID).
		Count(&existingCount)
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "provider model already mapped"})
		return
	}

	mapping := model.ModelMapping{
		ModelID:         m.ID,
		ProviderID:      req.ProviderID,
		ProviderModelID: req.ProviderModelID,
		Weight:          req.Weight,
		Enabled:         true,
	}
	if mapping.Weight == 0 {
		mapping.Weight = 1
	}
	if !req.Enabled {
		mapping.Enabled = false
	}

	if err := model.DB.Create(&mapping).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	model.DB.Preload("Provider").Preload("ProviderModel").First(&mapping, mapping.ID)
	c.JSON(http.StatusCreated, gin.H{"mapping": toMappingResponse(mapping)})
}

// UpdateMapping 更新模型映射
func (h *ModelHandler) UpdateMapping(c *gin.Context) {
	modelID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	mappingID, err := strconv.ParseUint(c.Param("mid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mapping id"})
		return
	}

	var mapping model.ModelMapping
	if err := model.DB.Where("id = ? AND model_id = ?", mappingID, modelID).First(&mapping).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mapping not found"})
		return
	}

	var req updateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.ProviderID != nil {
		var provider model.Provider
		if err := model.DB.First(&provider, *req.ProviderID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
			return
		}
		updates["provider_id"] = *req.ProviderID
	}
	if req.ProviderModelID != nil {
		var pm model.ProviderModel
		if err := model.DB.First(&pm, *req.ProviderModelID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider model not found"})
			return
		}
		providerID := mapping.ProviderID
		if req.ProviderID != nil {
			providerID = *req.ProviderID
		}
		if pm.ProviderID != providerID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider mismatch"})
			return
		}
		updates["provider_model_id"] = *req.ProviderModelID
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if len(updates) > 0 {
		if err := model.DB.Model(&mapping).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	model.DB.Preload("Provider").Preload("ProviderModel").First(&mapping, mappingID)
	c.JSON(http.StatusOK, gin.H{"mapping": toMappingResponse(mapping)})
}

// DeleteMapping 删除模型映射
func (h *ModelHandler) DeleteMapping(c *gin.Context) {
	modelID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	mappingID, err := strconv.ParseUint(c.Param("mid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mapping id"})
		return
	}

	if err := model.DB.Where("id = ? AND model_id = ?", mappingID, modelID).Delete(&model.ModelMapping{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "mapping deleted"})
}

// UpdateMappingsOrder 批量更新映射排序
func (h *ModelHandler) UpdateMappingsOrder(c *gin.Context) {
	modelID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req updateMappingsOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Order) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order array is empty"})
		return
	}

	totalMappings := len(req.Order)
	for i, mappingID := range req.Order {
		weight := totalMappings - 1 - i
		if err := model.DB.Model(&model.ModelMapping{}).
			Where("id = ? AND model_id = ?", mappingID, modelID).
			Update("weight", weight).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update mapping %d: %v", mappingID, err)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "mappings order updated"})
}

// =============================================================================
// 映射辅助函数
// =============================================================================

func toMappingResponse(m model.ModelMapping) mappingResponse {
	var providerResp *providerBasicResponse
	if m.Provider != nil {
		providerResp = &providerBasicResponse{
			ID:               m.Provider.ID,
			Name:             m.Provider.Name,
			Endpoints:        m.Provider.EndpointsMap(),
			OpenAIBaseURL:    m.Provider.OpenAIBaseURL,
			AnthropicBaseURL: m.Provider.AnthropicBaseURL,
			GeminiBaseURL:    m.Provider.GeminiBaseURL,
			DeepSeekBaseURL:  m.Provider.DeepSeekBaseURL,
		}
	}

	var modelInfoResp *modelInfoResponse
	var providerModelName string
	if m.ProviderModel != nil {
		providerModelName = m.ProviderModel.ModelID
		modelInfoResp = &modelInfoResponse{
			ContextWindow:  m.ProviderModel.ContextWindow,
			MaxOutput:      m.ProviderModel.MaxOutput,
			SupportsVision: m.ProviderModel.SupportsVision,
			SupportsTools:  m.ProviderModel.SupportsTools,
			SupportsStream: m.ProviderModel.SupportsStream,
		}
	}

	return mappingResponse{
		ID:                m.ID,
		ProviderID:        m.ProviderID,
		ProviderModelID:   m.ProviderModelID,
		ProviderModelName: providerModelName,
		Weight:            m.Weight,
		Enabled:           m.Enabled,
		Provider:          providerResp,
		ModelInfo:         modelInfoResp,
	}
}

func calculateEnabledCount(mappings []model.ModelMapping) int {
	enabledCount := 0
	for _, m := range mappings {
		if m.Enabled {
			enabledCount++
		}
	}
	return enabledCount
}
