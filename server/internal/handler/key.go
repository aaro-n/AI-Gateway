package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
)

// =============================================================================
// 类型定义
// =============================================================================

type KeyHandler struct{}

type createKeyRequest struct {
	Name       string  `json:"name" binding:"required"`
	Models     []uint  `json:"models"`
	ExpiresAt  *string `json:"expires_at"`
	AccessMode string  `json:"access_mode"` // "mapping", "direct", "hybrid"
	Format     string  `json:"format"`      // "openai", "anthropic", "gemini" - empty means all formats
}

type updateKeyRequest struct {
	Name       *string `json:"name"`
	Models     []uint  `json:"models"`
	ExpiresAt  *string `json:"expires_at"`
	Enabled    *bool   `json:"enabled"`
	AccessMode *string `json:"access_mode"`
}

type keyModelResponse struct {
	ID        uint   `json:"id"`
	ModelID   uint   `json:"model_id"`
	ModelName string `json:"model_name"`
}

type keyResponse struct {
	ID         uint               `json:"id"`
	Slug       string             `json:"slug"`
	Key        string             `json:"key"`
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	AccessMode string             `json:"access_mode"`
	ExpiresAt  *time.Time         `json:"expires_at"`
	CreatedAt  time.Time          `json:"created_at"`
	Format     string             `json:"format"` // 用户选择的主格式（openai/anthropic/gemini/deepseek/openrouter）
	Models     []keyModelResponse `json:"models,omitempty"`
	Formats    map[string]string  `json:"formats,omitempty"`
}

type keyListItemResponse struct {
	ID                uint              `json:"id"`
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Enabled           bool              `json:"enabled"`
	AccessMode        string            `json:"access_mode"`
	ExpiresAt         *time.Time        `json:"expires_at"`
	CreatedAt         time.Time         `json:"created_at"`
	Slug              string            `json:"slug"`
	DirectCount       int               `json:"direct_count"`  // 直通模型数量 (key_provider_models)
	MappingCount      int               `json:"mapping_count"` // 映射模型数量 (key_models)
	MCPToolsCount     int               `json:"mcp_tools_count"`
	MCPResourcesCount int               `json:"mcp_resources_count"`
	MCPPromptsCount   int               `json:"mcp_prompts_count"`
	Format            string            `json:"format"` // 主格式
	Formats           map[string]string `json:"formats,omitempty"`
}

type resetKeyRequest struct {
	Format string `json:"format"` // "openai", "anthropic", "gemini"
}

// =============================================================================
// 构造函数 & 生成函数
// =============================================================================

func NewKeyHandler() *KeyHandler {
	return &KeyHandler{}
}

func generateKey() string {
	bytes := make([]byte, 24)
	rand.Read(bytes)
	return "sk-" + hex.EncodeToString(bytes)
}

func generateKeyFormat(format string) string {
	desc, ok := registry.Get(format)
	if !ok {
		bytes := make([]byte, 24)
		rand.Read(bytes)
		return "sk-" + hex.EncodeToString(bytes)
	}

	bytes := make([]byte, desc.KeyLength)
	rand.Read(bytes)
	return desc.KeyPrefix + desc.KeyEncoder(bytes)
}

func createFormatsForKey(keyID uint) (map[string]string, error) {
	formats := make(map[string]string)
	for name := range registry.All() {
		formattedKey := generateKeyFormat(name)
		kf := model.KeyFormat{
			KeyID:        keyID,
			Format:       name,
			FormattedKey: formattedKey,
		}
		if err := model.DB.Create(&kf).Error; err != nil {
			return nil, err
		}
		formats[name] = formattedKey
	}
	return formats, nil
}

// createFormatForKey 创建单个指定格式的 KeyFormat
func createFormatForKey(keyID uint, format string) (map[string]string, error) {
	formats := make(map[string]string)
	formattedKey := generateKeyFormat(format)
	kf := model.KeyFormat{
		KeyID:        keyID,
		Format:       format,
		FormattedKey: formattedKey,
	}
	if err := model.DB.Create(&kf).Error; err != nil {
		return nil, err
	}
	formats[format] = formattedKey

	// 如果格式不是 openai，也需要创建openai格式作为兼容
	if format != "openai" {
		openaiKey := generateKeyFormat("openai")
		kf2 := model.KeyFormat{
			KeyID:        keyID,
			Format:       "openai",
			FormattedKey: openaiKey,
		}
		if err := model.DB.Create(&kf2).Error; err != nil {
			return nil, err
		}
		formats["openai"] = openaiKey
	}

	return formats, nil
}

// getPrimaryFormat 从 formats map 中提取主格式。
func getPrimaryFormat(formats map[string]string) string {
	if len(formats) == 0 {
		return ""
	}
	if _, ok := formats["openrouter"]; ok {
		return "openrouter"
	}
	for k := range formats {
		if k != "openai" {
			return k
		}
	}
	return "openai"
}

// =============================================================================
// 基础 CRUD
// =============================================================================

func (h *KeyHandler) List(c *gin.Context) {
	var keys []model.Key
	query := model.DB.Preload("Models.Model").Preload("Formats")

	// 普通用户只能看到自己的密钥
	if !IsAdmin(c) {
		userID := GetCurrentUserID(c)
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Order("name ASC").Find(&keys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]keyListItemResponse, len(keys))
	for i, k := range keys {
		maskedKey := k.Key
		if len(maskedKey) > 8 {
			maskedKey = maskedKey[:8] + "****" + maskedKey[len(maskedKey)-4:]
		}

		formats := make(map[string]string)
		for _, f := range k.Formats {
			masked := f.FormattedKey
			if len(masked) > 8 {
				masked = masked[:8] + "****" + masked[len(masked)-4:]
			}
			formats[f.Format] = masked
		}

		var mcpToolsCount, mcpResourcesCount, mcppromptsCount int64
		model.DB.Model(&model.KeyMCPTool{}).Where("key_id = ?", k.ID).Count(&mcpToolsCount)
		model.DB.Model(&model.KeyMCPResource{}).Where("key_id = ?", k.ID).Count(&mcpResourcesCount)
		model.DB.Model(&model.KeyMCPPrompt{}).Where("key_id = ?", k.ID).Count(&mcppromptsCount)

		var directCount, mappingCount int64
		model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", k.ID).Count(&directCount)
		model.DB.Model(&model.KeyModel{}).Where("key_id = ?", k.ID).Count(&mappingCount)

		result[i] = keyListItemResponse{
			ID:                k.ID,
			Key:               maskedKey,
			Name:              k.Name,
			Enabled:           k.Enabled,
			AccessMode:        k.AccessMode,
			ExpiresAt:         k.ExpiresAt,
			CreatedAt:         k.CreatedAt,
			Slug:              k.Slug,
			Format:            getPrimaryFormat(formats),
			DirectCount:       int(directCount),
			MappingCount:      int(mappingCount),
			MCPToolsCount:     int(mcpToolsCount),
			MCPResourcesCount: int(mcpResourcesCount),
			MCPPromptsCount:   int(mcppromptsCount),
			Formats:           formats,
		}
	}

	c.JSON(http.StatusOK, gin.H{"keys": result})
}

func (h *KeyHandler) Create(c *gin.Context) {
	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.ExpiresAt, time.Local)
		if err == nil {
			expiresAt = &t
		}
	}

	accessMode := req.AccessMode
	if accessMode == "" {
		accessMode = "hybrid"
	}

	rawKey := generateKey()
	if req.Format != "" {
		rawKey = generateKeyFormat(req.Format)
	}

	key := model.Key{
		Key:        rawKey,
		Name:       req.Name,
		ExpiresAt:  expiresAt,
		Enabled:    true,
		AccessMode: accessMode,
	}

	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		key.UserID = &uid
	}

	if err := model.DB.Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var formats map[string]string
	var fmtErr error
	if req.Format != "" {
		formats, fmtErr = createFormatForKey(key.ID, req.Format)
	} else {
		formats, fmtErr = createFormatsForKey(key.ID)
	}
	if fmtErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmtErr.Error()})
		return
	}

	for _, modelID := range req.Models {
		var m model.Model
		if err := model.DB.First(&m, modelID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model not found: " + strconv.FormatUint(uint64(modelID), 10)})
			return
		}
		akm := model.KeyModel{KeyID: key.ID, ModelID: modelID}
		model.DB.Create(&akm)
	}

	model.DB.Preload("Models.Model").First(&key, key.ID)
	models := make([]keyModelResponse, len(key.Models))
	for j, m := range key.Models {
		modelName := ""
		if m.Model != nil {
			modelName = m.Model.Name
		}
		models[j] = keyModelResponse{ID: m.ID, ModelID: m.ModelID, ModelName: modelName}
	}

	c.JSON(http.StatusCreated, gin.H{
		"key": keyResponse{
			ID: key.ID, Slug: key.Slug, Key: rawKey, Name: key.Name,
			Enabled: key.Enabled, AccessMode: key.AccessMode,
			ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt,
			Format: getPrimaryFormat(formats), Models: models, Formats: formats,
		},
		"raw_key": rawKey,
		"formats": formats,
	})
}

func (h *KeyHandler) Delete(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyModel{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyProvider{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPTool{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPResource{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPPrompt{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyFormat{})

	if err := model.DB.Delete(&model.Key{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "key deleted"})
}

func (h *KeyHandler) Update(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	var req updateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.ExpiresAt != nil {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.ExpiresAt, time.Local); err == nil {
			updates["expires_at"] = t
		}
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.AccessMode != nil {
		updates["access_mode"] = *req.AccessMode
	}

	if len(updates) > 0 {
		if err := model.DB.Model(&key).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if req.Models != nil {
		model.DB.Where("key_id = ?", key.ID).Delete(&model.KeyModel{})
		for _, modelID := range req.Models {
			var m model.Model
			if err := model.DB.First(&m, modelID).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "model not found: " + strconv.FormatUint(uint64(modelID), 10)})
				return
			}
			model.DB.Create(&model.KeyModel{KeyID: key.ID, ModelID: modelID})
		}
	}

	model.DB.Preload("Models.Model").Preload("Formats").First(&key, id)
	models := make([]keyModelResponse, len(key.Models))
	for j, m := range key.Models {
		modelName := ""
		if m.Model != nil {
			modelName = m.Model.Name
		}
		models[j] = keyModelResponse{ID: m.ID, ModelID: m.ModelID, ModelName: modelName}
	}

	updateFormats := make(map[string]string)
	for _, f := range key.Formats {
		updateFormats[f.Format] = f.FormattedKey
	}

	c.JSON(http.StatusOK, gin.H{"key": keyResponse{
		ID: key.ID, Slug: key.Slug, Key: key.Key, Name: key.Name,
		Enabled: key.Enabled, AccessMode: key.AccessMode,
		ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt,
		Format: getPrimaryFormat(updateFormats), Models: models,
	}})
}

// =============================================================================
// 辅助函数
// =============================================================================

// checkKeyOwnership 检查 key 是否存在且当前用户有权操作。
func (h *KeyHandler) checkKeyOwnership(c *gin.Context, keyID uint) *model.Key {
	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return nil
	}
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return nil
		}
	}
	return &key
}

// checkModelIDConflict 检查 modelID 是否与"模型映射"白名单冲突。
func (h *KeyHandler) checkModelIDConflict(keyID uint, modelID string) string {
	var kmIDs []uint
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Pluck("model_id", &kmIDs)
	if len(kmIDs) == 0 {
		return ""
	}
	var m model.Model
	if err := model.DB.Where("name = ? AND id IN ?", modelID, kmIDs).First(&m).Error; err != nil {
		return ""
	}
	return fmt.Sprintf("该模型已在「模型映射」白名单中，请先从「模型映射」中移除 %s，再添加为直通模型", modelID)
}

// checkMappingModelConflict 检查虚拟模型名是否与直通白名单冲突。
func (h *KeyHandler) checkMappingModelConflict(keyID uint, virtualModelName string) string {
	var kpmIDs []uint
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", keyID).Pluck("provider_model_id", &kpmIDs)
	if len(kpmIDs) == 0 {
		return ""
	}
	var pm model.ProviderModel
	if err := model.DB.Where("model_id = ? AND id IN ?", virtualModelName, kpmIDs).First(&pm).Error; err != nil {
		return ""
	}
	return fmt.Sprintf("该模型已在「模型厂商」直通白名单中，请先从「模型厂商」中移除 %s，再添加为映射模型", virtualModelName)
}

// =============================================================================
// Get / Reset
// =============================================================================

func (h *KeyHandler) Get(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var key model.Key
	if err := model.DB.Preload("Formats").First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	maskedKey := key.Key
	if len(maskedKey) > 8 {
		maskedKey = maskedKey[:8] + "****" + maskedKey[len(maskedKey)-4:]
	}

	formats := make(map[string]string)
	for _, f := range key.Formats {
		masked := f.FormattedKey
		if len(masked) > 8 {
			masked = masked[:8] + "****" + masked[len(masked)-4:]
		}
		formats[f.Format] = masked
	}

	c.JSON(http.StatusOK, gin.H{
		"key": keyResponse{
			ID: key.ID, Slug: key.Slug, Key: maskedKey, Name: key.Name,
			Enabled: key.Enabled, ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt,
			Format: getPrimaryFormat(formats), Formats: formats,
		},
	})
}

func (h *KeyHandler) Reset(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req resetKeyRequest
	c.ShouldBindJSON(&req)

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if key.UserID == nil || *key.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	rawKey := generateKey()
	if req.Format != "" {
		rawKey = generateKeyFormat(req.Format)
	}

	if err := model.DB.Model(&key).Update("key", rawKey).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyFormat{})
	var formats map[string]string
	if req.Format != "" {
		formats, err = createFormatForKey(key.ID, req.Format)
	} else {
		formats, err = createFormatsForKey(key.ID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new key formats: " + err.Error()})
		return
	}

	model.DB.Preload("Models.Model").First(&key, id)
	models := make([]keyModelResponse, len(key.Models))
	for j, m := range key.Models {
		modelName := ""
		if m.Model != nil {
			modelName = m.Model.Name
		}
		models[j] = keyModelResponse{ID: m.ID, ModelID: m.ModelID, ModelName: modelName}
	}

	maskedKey := key.Key
	if len(maskedKey) > 8 {
		maskedKey = maskedKey[:8] + "****" + maskedKey[len(maskedKey)-4:]
	}

	maskedFormats := make(map[string]string)
	for name, val := range formats {
		masked := val
		if len(masked) > 8 {
			masked = masked[:8] + "****" + masked[len(masked)-4:]
		}
		maskedFormats[name] = masked
	}

	c.JSON(http.StatusOK, gin.H{
		"key": keyResponse{
			ID: key.ID, Slug: key.Slug, Key: maskedKey, Name: key.Name,
			Enabled: key.Enabled, AccessMode: key.AccessMode,
			ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt,
			Format: getPrimaryFormat(formats), Models: models, Formats: maskedFormats,
		},
		"raw_key": rawKey,
		"formats": formats,
	})
}
