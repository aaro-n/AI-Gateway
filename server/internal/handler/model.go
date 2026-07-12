package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	"ai-gateway/internal/protocols/capabilities"
)

type ModelHandler struct{}

type createModelRequest struct {
	Name    string `json:"name" binding:"required"`
	Enabled bool   `json:"enabled"`
}

type updateModelRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

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

type providerBasicResponse struct {
	ID               uint              `json:"id"`
	Name             string            `json:"name"`
	Endpoints        map[string]string `json:"endpoints,omitempty"`
	OpenAIBaseURL    string            `json:"openai_base_url,omitempty"`
	AnthropicBaseURL string            `json:"anthropic_base_url,omitempty"`
	GeminiBaseURL    string            `json:"gemini_base_url,omitempty"`
	DeepSeekBaseURL  string            `json:"deepseek_base_url,omitempty"`
}

func NewModelHandler() *ModelHandler {
	return &ModelHandler{}
}

func (h *ModelHandler) List(c *gin.Context) {
	var models []model.Model
	if err := model.DB.Order("name ASC").Find(&models).Error; err != nil {
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

func (h *ModelHandler) Create(c *gin.Context) {
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 清理已软删除的同名记录，释放唯一索引
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
		// 清理已软删除的同名记录，释放唯一索引（排除自己）
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

func (h *ModelHandler) Delete(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 硬删除关联的 ModelMapping
	if err := model.DB.Where("model_id = ?", id).Delete(&model.ModelMapping{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 软删除 Model
	if err := model.DB.Delete(&model.Model{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

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

	// 检查同一厂商下是否已有相同 provider_model_id 的映射
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

// ── 动态 Capabilities API ──

// modelCapabilitiesResponse 模型的能力视图
type modelCapabilitiesResponse struct {
	ModelID      uint                          `json:"model_id"`
	ModelName    string                        `json:"model_name"`
	KeyProtocol  string                        `json:"key_protocol"` // 客户端协议
	KeyFormat    string                        `json:"key_format"`   // key 的格式
	Mappings     []mappingCapabilitiesResponse `json:"mappings"`     // 每个映射的能力
	Intersection capabilityIntersection        `json:"intersection"` // 所有映射的能力交集
}

type mappingCapabilitiesResponse struct {
	MappingID       uint                           `json:"mapping_id"`
	ProviderID      uint                           `json:"provider_id"`
	ProviderName    string                         `json:"provider_name"`
	ProviderModelID string                         `json:"provider_model_id"`
	Weight          int                            `json:"weight"`
	Enabled         bool                           `json:"enabled"`
	Protocols       []string                       `json:"protocols"`              // 该 Provider 支持的协议
	Capabilities    *capabilities.ComparisonResult `json:"capabilities,omitempty"` // 客户端协议 vs 该 Provider 协议的对比
}

type capabilityIntersection struct {
	SupportsVision bool `json:"supports_vision"`
	SupportsTools  bool `json:"supports_tools"`
	SupportsStream bool `json:"supports_stream"`
}

// GetCapabilities 返回虚拟模型的动态能力视图。
// GET /api/v1/models/:id/capabilities?key_id=1
//
// 根据 key 的协议格式和模型的实际路由映射，返回跨协议转换的损失分析。
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

	// 获取模型
	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	// 确定客户端协议
	clientProtocol := "openai" // 默认
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

	// 获取模型的所有映射
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

		// 如果客户端协议与 provider 协议不同，做对比分析
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
