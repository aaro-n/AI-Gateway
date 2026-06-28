package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
)

type ProviderModelHandler struct{}

type createProviderModelRequest struct {
	ModelID        string  `json:"model_id"`
	DisplayName    string  `json:"display_name"`
	OwnedBy        string  `json:"owned_by"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	InputPrice     float64 `json:"input_price"`
	OutputPrice    float64 `json:"output_price"`
	SupportsVision bool    `json:"supports_vision"`
	SupportsTools  bool    `json:"supports_tools"`
	SupportsStream bool    `json:"supports_stream"`
	Source         string  `json:"source"`
}

// updateProviderModelRequest 使用指针类型，前端可只传需要更新的字段，零值不会被误覆盖
type updateProviderModelRequest struct {
	ModelID        *string  `json:"model_id"`
	DisplayName    *string  `json:"display_name"`
	OwnedBy        *string  `json:"owned_by"`
	ContextWindow  *int     `json:"context_window"`
	MaxOutput      *int     `json:"max_output"`
	InputPrice     *float64 `json:"input_price"`
	OutputPrice    *float64 `json:"output_price"`
	SupportsVision *bool    `json:"supports_vision"`
	SupportsTools  *bool    `json:"supports_tools"`
	SupportsStream *bool    `json:"supports_stream"`
}

type providerModelResponse struct {
	ID             uint    `json:"id"`
	ProviderID     uint    `json:"provider_id"`
	ModelID        string  `json:"model_id"`
	DisplayName    string  `json:"display_name"`
	OwnedBy        string  `json:"owned_by"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	InputPrice     float64 `json:"input_price"`
	OutputPrice    float64 `json:"output_price"`
	SupportsVision bool    `json:"supports_vision"`
	SupportsTools  bool    `json:"supports_tools"`
	SupportsStream bool    `json:"supports_stream"`
	IsAvailable    bool    `json:"is_available"`
	Source         string  `json:"source"`
	CreatedAt      string  `json:"created_at"`
}

func NewProviderModelHandler() *ProviderModelHandler {
	return &ProviderModelHandler{}
}

func toProviderModelResponse(m model.ProviderModel) providerModelResponse {
	return providerModelResponse{
		ID:             m.ID,
		ProviderID:     m.ProviderID,
		ModelID:        m.ModelID,
		DisplayName:    m.DisplayName,
		OwnedBy:        m.OwnedBy,
		ContextWindow:  m.ContextWindow,
		MaxOutput:      m.MaxOutput,
		InputPrice:     m.InputPrice,
		OutputPrice:    m.OutputPrice,
		SupportsVision: m.SupportsVision,
		SupportsTools:  m.SupportsTools,
		SupportsStream: m.SupportsStream,
		IsAvailable:    m.IsAvailable,
		Source:         m.Source,
		CreatedAt:      m.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func (h *ProviderModelHandler) List(c *gin.Context) {
	providerID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	var models []model.ProviderModel
	query := model.DB.Where("provider_id = ?", providerID)

	if c.Query("available_only") == "true" {
		query = query.Where("is_available = ?", true)
	}

	if err := query.Find(&models).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]providerModelResponse, len(models))
	for i, m := range models {
		result[i] = toProviderModelResponse(m)
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

func (h *ProviderModelHandler) Create(c *gin.Context) {
	providerID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	var req createProviderModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	source := req.Source
	if source == "" {
		source = "manual"
	}

	contextWindow := req.ContextWindow
	maxOutput := req.MaxOutput
	if source == "manual" {
		if contextWindow <= 0 {
			contextWindow = 8192 // 默认一个合理的上下文窗口，防止出错
		}
		if maxOutput <= 0 {
			maxOutput = 4096 // 默认最大输出，防止出错
		}
	}

	pm := model.ProviderModel{
		ProviderID:     uint(providerID),
		ModelID:        req.ModelID,
		DisplayName:    req.DisplayName,
		OwnedBy:        req.OwnedBy,
		ContextWindow:  contextWindow,
		MaxOutput:      maxOutput,
		InputPrice:     req.InputPrice,
		OutputPrice:    req.OutputPrice,
		SupportsVision: req.SupportsVision,
		SupportsTools:  req.SupportsTools,
		SupportsStream: req.SupportsStream,
		IsAvailable:    true,
		Source:         source,
	}

	if err := model.DB.Create(&pm).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"model": toProviderModelResponse(pm)})
}

func (h *ProviderModelHandler) Update(c *gin.Context) {
	providerID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	modelID, err := strconv.ParseUint(c.Param("mid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var pm model.ProviderModel
	if err := model.DB.Where("id = ? AND provider_id = ?", modelID, providerID).First(&pm).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req updateProviderModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.OwnedBy != nil {
		updates["owned_by"] = *req.OwnedBy
	}
	if req.ContextWindow != nil {
		if pm.Source == "sync" && *req.ContextWindow != pm.ContextWindow {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify context window of synced models"})
			return
		}
		updates["context_window"] = *req.ContextWindow
	}
	if req.MaxOutput != nil {
		if pm.Source == "sync" && *req.MaxOutput != pm.MaxOutput {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify max output of synced models"})
			return
		}
		updates["max_output"] = *req.MaxOutput
	}
	if req.InputPrice != nil {
		updates["input_price"] = *req.InputPrice
	}
	if req.OutputPrice != nil {
		updates["output_price"] = *req.OutputPrice
	}
	if req.SupportsVision != nil {
		updates["supports_vision"] = *req.SupportsVision
	}
	if req.SupportsTools != nil {
		updates["supports_tools"] = *req.SupportsTools
	}
	if req.SupportsStream != nil {
		updates["supports_stream"] = *req.SupportsStream
	}

	if req.ModelID != nil && *req.ModelID != "" && *req.ModelID != pm.ModelID {
		var existing model.ProviderModel
		if err := model.DB.Where("provider_id = ? AND model_id = ? AND id != ?", providerID, *req.ModelID, pm.ID).First(&existing).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model_id already exists"})
			return
		}
		updates["model_id"] = *req.ModelID
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"model": toProviderModelResponse(pm)})
		return
	}

	if err := model.DB.Model(&pm).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update model"})
		return
	}

	if err := model.DB.First(&pm, pm.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read updated model"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": toProviderModelResponse(pm)})
}

func (h *ProviderModelHandler) Delete(c *gin.Context) {
	providerID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	modelID, err := strconv.ParseUint(c.Param("mid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var pm model.ProviderModel
	if err := model.DB.Where("id = ? AND provider_id = ?", modelID, providerID).First(&pm).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	// 使用事务确保数据一致性
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_model_id = ?", pm.ID).Delete(&model.ModelMapping{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&pm).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete model"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

func (h *ProviderModelHandler) Sync(c *gin.Context) {
	providerID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	var provider model.Provider
	if err := model.DB.First(&provider, providerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	models, err := protocolsPkg.AutoSyncModels(provider.ID,
		provider.OpenAIBaseURL,
		provider.AnthropicBaseURL,
		provider.GeminiBaseURL,
		provider.APIKey,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	added := 0
	updated := 0
	syncedModelIDs := make([]string, 0, len(models))

	for _, pm := range models {
		syncedModelIDs = append(syncedModelIDs, pm.ModelID)
		var existing model.ProviderModel
		res := model.DB.Where("provider_id = ? AND model_id = ?", provider.ID, pm.ModelID).First(&existing)

		if res.Error != nil {
			if err := model.DB.Create(&pm).Error; err != nil {
				log.Printf("[Sync] Failed to create model %s: %v", pm.ModelID, err)
				continue
			}
			added++
		} else if existing.Source != "manual" {
			model.DB.Model(&existing).Updates(map[string]interface{}{
				"display_name":    pm.DisplayName,
				"owned_by":        pm.OwnedBy,
				"context_window":  pm.ContextWindow,
				"max_output":      pm.MaxOutput,
				"supports_vision": pm.SupportsVision,
				"supports_tools":  pm.SupportsTools,
				"is_available":    true,
			})
			updated++
		}
	}

	// Disable provider models that are NO LONGER returned during sync, but only if they were added via sync
	var deactivatedCount int64 = 0
	if len(syncedModelIDs) > 0 {
		var deactivatedModels []model.ProviderModel
		model.DB.Where("provider_id = ? AND source = ? AND model_id NOT IN ? AND is_available = ?", provider.ID, "sync", syncedModelIDs, true).Find(&deactivatedModels)
		deactivatedCount = int64(len(deactivatedModels))
		if deactivatedCount > 0 {
			model.DB.Model(&model.ProviderModel{}).Where("provider_id = ? AND source = ? AND model_id NOT IN ?", provider.ID, "sync", syncedModelIDs).Updates(map[string]interface{}{
				"is_available": false,
			})
		}
	}

	now := time.Now()
	model.DB.Model(&provider).Update("last_sync_at", &now)

	c.JSON(http.StatusOK, gin.H{
		"message":     fmt.Sprintf("%s models synced", provider.Name),
		"added":       added,
		"updated":     updated,
		"deactivated": deactivatedCount,
		"total":       len(models),
	})
}
