package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
)

// modelWithStatusResponse 模型映射列表响应（含 selected/enabled 标记）
type modelWithStatusResponse struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	MappingCount     int    `json:"mapping_count"`
	MinContextWindow int    `json:"min_context_window"`
	MinMaxOutput     int    `json:"min_max_output"`
	SupportsVision   bool   `json:"supports_vision"`
	SupportsTools    bool   `json:"supports_tools"`
	SupportsStream   bool   `json:"supports_stream"`
	Selected         bool   `json:"selected"`
	Enabled          bool   `json:"enabled"`
}

// =============================================================================
// 模型映射管理
// =============================================================================

// ListModels 列出某 key 的可用模型映射（全量 + selected/enabled 标记）
// GET /keys/:id/models
func (h *KeyHandler) ListModels(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 非 admin 检查 key 所有权
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		var key model.Key
		if err := model.DB.First(&key, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
			return
		}
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	// 获取白名单记录
	var kmRows []model.KeyModel
	model.DB.Where("key_id = ?", id).Find(&kmRows)
	kmMap := make(map[uint]model.KeyModel)
	for _, r := range kmRows {
		kmMap[r.ModelID] = r
	}

	// 对于非 admin 用户，只展示已授权的模型
	modelQuery := model.DB.Where("enabled = ?", true)
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		modelIDs, err := GetUserModelIDs(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(modelIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"models": []modelWithStatusResponse{}})
			return
		}
		modelQuery = modelQuery.Where("id IN ?", modelIDs)
	}

	var allModels []model.Model
	if err := modelQuery.Order("id ASC").Find(&allModels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]modelWithStatusResponse, len(allModels))
	for i, m := range allModels {
		var mappings []model.ModelMapping
		model.DB.Preload("Provider").
			Joins("JOIN providers ON providers.id = model_mappings.provider_id AND providers.enabled = ?", true).
			Where("model_id = ? AND model_mappings.enabled = ?", m.ID, true).
			Order("weight DESC").
			Find(&mappings)

		minContext, minOutput := calculateMinTokens(mappings)
		supportsVision, supportsTools, supportsStream := calculateCapabilitiesIntersection(mappings)

		row, exists := kmMap[m.ID]

		result[i] = modelWithStatusResponse{
			ID:               m.ID,
			Name:             m.Name,
			MappingCount:     len(mappings),
			MinContextWindow: minContext,
			MinMaxOutput:     minOutput,
			SupportsVision:   supportsVision,
			SupportsTools:    supportsTools,
			SupportsStream:   supportsStream,
			Selected:         exists,
			Enabled:          exists && row.Enabled,
		}
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

// AddModel 添加模型映射（upsert）
// POST /keys/:id/models/:model_id
func (h *KeyHandler) AddModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	modelID, err := strconv.ParseUint(c.Param("model_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	key := h.checkKeyOwnership(c, keyID)
	if key == nil {
		return
	}

	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	if conflict := h.checkMappingModelConflict(uint(keyID), m.Name); conflict != "" {
		c.JSON(http.StatusConflict, gin.H{"error": conflict})
		return
	}

	var existing model.KeyModel
	if err := model.DB.Where("key_id = ? AND model_id = ?", keyID, modelID).First(&existing).Error; err == nil {
		model.DB.Model(&existing).Update("enabled", true)
		c.JSON(http.StatusOK, gin.H{"message": "model association enabled"})
		return
	}

	if err := model.DB.Create(&model.KeyModel{KeyID: uint(keyID), ModelID: uint(modelID), Enabled: true}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model association added"})
}

// RemoveModel 从映射白名单移除
// DELETE /keys/:id/models/:model_id
func (h *KeyHandler) RemoveModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	modelID, err := strconv.ParseUint(c.Param("model_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	model.DB.Where("key_id = ? AND model_id = ?", keyID, modelID).Delete(&model.KeyModel{})
	c.JSON(http.StatusOK, gin.H{"message": "model association removed"})
}

// ClearModels 批量禁用映射白名单
// DELETE /keys/:id/models
func (h *KeyHandler) ClearModels(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Update("enabled", false)
	c.JSON(http.StatusOK, gin.H{"message": "all model associations disabled"})
}

// EnableAllModels 批量启用映射白名单
// PUT /keys/:id/models
func (h *KeyHandler) EnableAllModels(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Update("enabled", true)
	c.JSON(http.StatusOK, gin.H{"message": "all model associations enabled"})
}

// ToggleModel 切换映射模型启用状态
// PUT /keys/:id/models/:model_id
func (h *KeyHandler) ToggleModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}
	modelID, err := strconv.ParseUint(c.Param("model_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var km model.KeyModel
	if err := model.DB.Where("key_id = ? AND model_id = ?", keyID, modelID).First(&km).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not in whitelist"})
		return
	}
	newVal := !km.Enabled
	model.DB.Model(&km).Update("enabled", newVal)
	c.JSON(http.StatusOK, gin.H{"message": "toggled", "enabled": newVal})
}
