package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	"ai-gateway/internal/protocols/capabilities"
)

// ModelHandler 虚拟模型管理处理器
type ModelHandler struct{}

// ── 请求类型 ──

type createModelRequest struct {
	Name    string `json:"name" binding:"required"`
	Enabled bool   `json:"enabled"`
}

type updateModelRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

// ── 响应类型 ──

type modelResponse struct {
	ID               uint              `json:"id"`
	Slug             string            `json:"slug"`
	Model            string            `json:"model"`
	Enabled          bool              `json:"enabled"`
	MappingCount     int               `json:"mapping_count"`
	MinContextWindow int               `json:"min_context_window"`
	MinMaxOutput     int               `json:"min_max_output"`
	SupportsVision   bool              `json:"supports_vision"`
	SupportsTools    bool              `json:"supports_tools"`
	SupportsStream   bool              `json:"supports_stream"`
	CreatedAt        time.Time         `json:"created_at"`
	Mappings         []mappingResponse `json:"mappings,omitempty"`
}

type providerBasicResponse struct {
	ID               uint              `json:"id"`
	Name             string            `json:"name"`
	Endpoints        map[string]string `json:"endpoints,omitempty"`
	OpenAIBaseURL    string            `json:"openai_base_url,omitempty"`
	AnthropicBaseURL string            `json:"anthropic_base_url,omitempty"`
	GeminiBaseURL    string            `json:"gemini_base_url,omitempty"`
	DeepSeekBaseURL  string            `json:"deepseek_base_url,omitempty"`
}

// ── Capabilities 类型 ──

type modelCapabilitiesResponse struct {
	ModelID      uint                          `json:"model_id"`
	ModelName    string                        `json:"model_name"`
	KeyProtocol  string                        `json:"key_protocol"`
	KeyFormat    string                        `json:"key_format"`
	Mappings     []mappingCapabilitiesResponse `json:"mappings"`
	Intersection capabilityIntersection        `json:"intersection"`
}

type capabilityIntersection struct {
	SupportsVision bool `json:"supports_vision"`
	SupportsTools  bool `json:"supports_tools"`
	SupportsStream bool `json:"supports_stream"`
}

func NewModelHandler() *ModelHandler {
	return &ModelHandler{}
}

// =============================================================================
// 虚拟模型 CRUD
// =============================================================================

// List 列出所有虚拟模型
func (h *ModelHandler) List(c *gin.Context) {
	var models []model.Model
	query := model.DB.Order("name ASC")

	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		modelIDs, err := GetUserModelIDs(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(modelIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"models": []modelResponse{}})
			return
		}
		query = query.Where("id IN ?", modelIDs)
	}

	if err := query.Find(&models).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]modelResponse, len(models))
	for i, a := range models {
		var mappings []model.ModelMapping
		model.DB.Preload("Provider").Preload("ProviderModel").
			Joins("JOIN providers ON providers.id = model_mappings.provider_id AND providers.enabled = ?", true).
			Where("model_id = ?", a.ID).
			Order("weight DESC").
			Find(&mappings)

		mappingResponses := make([]mappingResponse, len(mappings))
		for j, mm := range mappings {
			mappingResponses[j] = toMappingResponse(mm)
		}

		enabledCount := calculateEnabledCount(mappings)
		minContext, minOutput := calculateMinTokens(mappings)
		supportsVision, supportsTools, supportsStream := calculateCapabilitiesIntersection(mappings)

		result[i] = modelResponse{
			ID:               a.ID,
			Slug:             a.Slug,
			Model:            a.Name,
			Enabled:          a.Enabled,
			MappingCount:     enabledCount,
			MinContextWindow: minContext,
			MinMaxOutput:     minOutput,
			SupportsVision:   supportsVision,
			SupportsTools:    supportsTools,
			SupportsStream:   supportsStream,
			CreatedAt:        a.CreatedAt,
			Mappings:         mappingResponses,
		}
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

// Get 获取单个虚拟模型
func (h *ModelHandler) Get(c *gin.Context) {
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

	mappingResponses := make([]mappingResponse, len(mappings))
	for j, mm := range mappings {
		mappingResponses[j] = toMappingResponse(mm)
	}

	c.JSON(http.StatusOK, gin.H{"model": modelResponse{
		ID:        m.ID,
		Slug:      m.Slug,
		Model:     m.Name,
		Enabled:   m.Enabled,
		CreatedAt: m.CreatedAt,
		Mappings:  mappingResponses,
	}})
}

// Create 创建虚拟模型
func (h *ModelHandler) Create(c *gin.Context) {
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model.DB.Unscoped().Where("name = ?", req.Name).Delete(&model.Model{})

	m := model.Model{
		Name:    req.Name,
		Enabled: true,
	}
	if !req.Enabled {
		m.Enabled = false
	}

	if err := model.DB.Create(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"model": modelResponse{
		ID:           m.ID,
		Slug:         m.Slug,
		Model:        m.Name,
		Enabled:      m.Enabled,
		MappingCount: 0,
		CreatedAt:    m.CreatedAt,
	}})
}

// Update 更新虚拟模型
func (h *ModelHandler) Update(c *gin.Context) {
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

	var req updateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		model.DB.Unscoped().Where("name = ? AND id != ?", *req.Name, m.ID).Delete(&model.Model{})
		updates["name"] = *req.Name
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if len(updates) > 0 {
		if err := model.DB.Model(&m).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	model.DB.First(&m, id)

	var mappings []model.ModelMapping
	model.DB.Preload("Provider").Preload("ProviderModel").
		Joins("JOIN providers ON providers.id = model_mappings.provider_id AND providers.enabled = ?", true).
		Where("model_id = ?", m.ID).
		Order("weight DESC").
		Find(&mappings)

	mappingResponses := make([]mappingResponse, len(mappings))
	for j, mm := range mappings {
		mappingResponses[j] = toMappingResponse(mm)
	}

	c.JSON(http.StatusOK, gin.H{"model": modelResponse{
		ID:        m.ID,
		Slug:      m.Slug,
		Model:     m.Name,
		Enabled:   m.Enabled,
		CreatedAt: m.CreatedAt,
		Mappings:  mappingResponses,
	}})
}

// Delete 删除虚拟模型（软删除 + 硬删除关联映射）
func (h *ModelHandler) Delete(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := model.DB.Where("model_id = ?", id).Delete(&model.ModelMapping{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := model.DB.Delete(&model.Model{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

// =============================================================================
// 能力计算辅助函数
// =============================================================================

// calculateMinTokens 计算已启用映射的最小上下文窗口和最大输出 token。
func calculateMinTokens(mappings []model.ModelMapping) (int, int) {
	minContext := 0
	minOutput := 0
	hasEnabled := false

	for _, m := range mappings {
		if !m.Enabled {
			continue
		}

		if m.Provider == nil || !m.Provider.Enabled {
			continue
		}

		if m.ProviderModel == nil {
			continue
		}

		hasEnabled = true
		pm := m.ProviderModel

		if pm.ContextWindow > 0 {
			if minContext == 0 || pm.ContextWindow < minContext {
				minContext = pm.ContextWindow
			}
		}
		if pm.MaxOutput > 0 {
			if minOutput == 0 || pm.MaxOutput < minOutput {
				minOutput = pm.MaxOutput
			}
		}
	}

	if !hasEnabled {
		return 0, 0
	}

	return minContext, minOutput
}

// calculateCapabilitiesIntersection 计算所有已启用映射的能力交集。
func calculateCapabilitiesIntersection(mappings []model.ModelMapping) (bool, bool, bool) {
	supportsVision := true
	supportsTools := true
	supportsStream := true
	hasEnabled := false

	for _, m := range mappings {
		if !m.Enabled {
			continue
		}

		if m.Provider == nil || !m.Provider.Enabled {
			continue
		}

		if m.ProviderModel == nil {
			supportsVision = false
			supportsTools = false
			supportsStream = false
			continue
		}

		hasEnabled = true
		pm := m.ProviderModel

		if !pm.SupportsVision {
			supportsVision = false
		}
		if !pm.SupportsTools {
			supportsTools = false
		}
		if !pm.SupportsStream {
			supportsStream = false
		}
	}

	if !hasEnabled {
		return false, false, false
	}

	return supportsVision, supportsTools, supportsStream
}

// =============================================================================
// 动态 Capabilities API
// =============================================================================

// GetCapabilities 返回虚拟模型的动态能力视图。
// GET /api/v1/models/:id/capabilities?key_id=1
func (h *ModelHandler) GetCapabilities(c *gin.Context) {
	modelID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	keyIDStr := c.Query("key_id")
	var keyID uint
	if keyIDStr != "" {
		id, err := strconv.ParseUint(keyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key_id"})
			return
		}
		keyID = uint(id)
	}

	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	clientProtocol := "openai"
	keyFormat := "openai"
	if keyID > 0 {
		var keyFormats []model.KeyFormat
		model.DB.Where("key_id = ?", keyID).Find(&keyFormats)
		for _, kf := range keyFormats {
			if kf.Format != "openai" {
				keyFormat = kf.Format
				clientProtocol = kf.Format
				break
			}
		}
	}

	var mappings []model.ModelMapping
	model.DB.Preload("Provider").Preload("ProviderModel").
		Joins("JOIN providers ON providers.id = model_mappings.provider_id AND providers.enabled = ?", true).
		Where("model_id = ?", m.ID).
		Order("weight DESC").
		Find(&mappings)

	mappingCaps := make([]mappingCapabilitiesResponse, 0, len(mappings))
	allVision := true
	allTools := true
	allStream := true
	hasEnabled := false

	for _, mm := range mappings {
		if !mm.Enabled {
			continue
		}
		if mm.Provider == nil || !mm.Provider.Enabled {
			continue
		}
		if mm.ProviderModel == nil {
			continue
		}

		hasEnabled = true
		pm := mm.ProviderModel
		if !pm.SupportsVision {
			allVision = false
		}
		if !pm.SupportsTools {
			allTools = false
		}
		if !pm.SupportsStream {
			allStream = false
		}

		providerProto := mm.Provider.SupportedProtocols()
		mc := mappingCapabilitiesResponse{
			MappingID:       mm.ID,
			ProviderID:      mm.Provider.ID,
			ProviderName:    mm.Provider.Name,
			ProviderModelID: pm.ModelID,
			Weight:          mm.Weight,
			Enabled:         mm.Enabled,
			Protocols:       providerProto,
		}

		if len(providerProto) > 0 && providerProto[0] != clientProtocol {
			result := capabilities.Compare(clientProtocol, providerProto[0])
			mc.Capabilities = &result
		}

		mappingCaps = append(mappingCaps, mc)
	}

	if !hasEnabled {
		allVision = false
		allTools = false
		allStream = false
	}

	resp := modelCapabilitiesResponse{
		ModelID:     m.ID,
		ModelName:   m.Name,
		KeyProtocol: clientProtocol,
		KeyFormat:   keyFormat,
		Mappings:    mappingCaps,
		Intersection: capabilityIntersection{
			SupportsVision: allVision,
			SupportsTools:  allTools,
			SupportsStream: allStream,
		},
	}

	c.JSON(http.StatusOK, gin.H{"capabilities": resp})
}
