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
	"ai-gateway/internal/model"
)

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
	Key        string             `json:"key"`
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	AccessMode string             `json:"access_mode"`
	ExpiresAt  *time.Time         `json:"expires_at"`
	CreatedAt  time.Time          `json:"created_at"`
	Models     []keyModelResponse `json:"models,omitempty"`
	Formats    map[string]string  `json:"formats,omitempty"`
}

type keyCreateResponse struct {
	Key     keyResponse       `json:"key"`
	RawKey  string            `json:"raw_key"`
	Formats map[string]string `json:"formats,omitempty"`
}

type keyMCPToolResponse struct {
	ID       uint   `json:"id"`
	ToolID   uint   `json:"tool_id"`
	ToolName string `json:"tool_name"`
	MCPID    uint   `json:"mcp_id"`
	MCPName  string `json:"mcp_name"`
}

type keyMCPResourceResponse struct {
	ID           uint   `json:"id"`
	ResourceID   uint   `json:"resource_id"`
	ResourceName string `json:"resource_name"`
	ResourceURI  string `json:"resource_uri"`
	MCPID        uint   `json:"mcp_id"`
	MCPName      string `json:"mcp_name"`
}

type keyMCPPromptResponse struct {
	ID         uint   `json:"id"`
	PromptID   uint   `json:"prompt_id"`
	PromptName string `json:"prompt_name"`
	MCPID      uint   `json:"mcp_id"`
	MCPName    string `json:"mcp_name"`
}

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

type keyListItemResponse struct {
	ID                uint               `json:"id"`
	Key               string             `json:"key"`
	Name              string             `json:"name"`
	Enabled           bool               `json:"enabled"`
	AccessMode        string             `json:"access_mode"`
	ExpiresAt         *time.Time         `json:"expires_at"`
	CreatedAt         time.Time          `json:"created_at"`
	Models            []keyModelResponse `json:"models,omitempty"`
	MCPToolsCount     int                `json:"mcp_tools_count"`
	MCPResourcesCount int                `json:"mcp_resources_count"`
	MCPPromptsCount   int                `json:"mcp_prompts_count"`
	Formats           map[string]string  `json:"formats,omitempty"`
}

func (h *KeyHandler) List(c *gin.Context) {
	var keys []model.Key
	if err := model.DB.Preload("Models.Model").Preload("Formats").Find(&keys).Error; err != nil {
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

		models := make([]keyModelResponse, len(k.Models))
		for j, m := range k.Models {
			modelName := ""
			if m.Model != nil {
				modelName = m.Model.Name
			}
			models[j] = keyModelResponse{
				ID:        m.ID,
				ModelID:   m.ModelID,
				ModelName: modelName,
			}
		}

		var mcpToolsCount, mcpResourcesCount, mcppromptsCount int64
		model.DB.Model(&model.KeyMCPTool{}).Where("key_id = ?", k.ID).Count(&mcpToolsCount)
		model.DB.Model(&model.KeyMCPResource{}).Where("key_id = ?", k.ID).Count(&mcpResourcesCount)
		model.DB.Model(&model.KeyMCPPrompt{}).Where("key_id = ?", k.ID).Count(&mcppromptsCount)

		result[i] = keyListItemResponse{
			ID:                k.ID,
			Key:               maskedKey,
			Name:              k.Name,
			Enabled:           k.Enabled,
			AccessMode:        k.AccessMode,
			ExpiresAt:         k.ExpiresAt,
			CreatedAt:         k.CreatedAt,
			Models:            models,
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
		t, err := time.Parse("2006-01-02 15:04:05", *req.ExpiresAt)
		if err == nil {
			expiresAt = &t
		}
	}

	accessMode := req.AccessMode
	if accessMode == "" {
		accessMode = "hybrid"
	}

	// 生成主密钥：如果指定了格式则使用对应格式，否则使用默认 sk- 前缀
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

	if err := model.DB.Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 创建格式化密钥：如果指定了 format 则只创建该格式，否则创建全部格式
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
		akm := model.KeyModel{
			KeyID:   key.ID,
			ModelID: modelID,
		}
		model.DB.Create(&akm)
	}

	model.DB.Preload("Models.Model").First(&key, key.ID)

	models := make([]keyModelResponse, len(key.Models))
	for j, m := range key.Models {
		modelName := ""
		if m.Model != nil {
			modelName = m.Model.Name
		}
		models[j] = keyModelResponse{
			ID:        m.ID,
			ModelID:   m.ModelID,
			ModelName: modelName,
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"key": keyResponse{
			ID:         key.ID,
			Key:        rawKey,
			Name:       key.Name,
			Enabled:    key.Enabled,
			AccessMode: key.AccessMode,
			ExpiresAt:  key.ExpiresAt,
			CreatedAt:  key.CreatedAt,
			Models:     models,
			Formats:    formats,
		},
		"raw_key": rawKey,
		"formats": formats,
	})
}

func (h *KeyHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 硬删除关联表
	model.DB.Where("key_id = ?", id).Delete(&model.KeyModel{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyProvider{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPTool{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPResource{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPPrompt{})
	model.DB.Where("key_id = ?", id).Delete(&model.KeyFormat{})

	// 软删除 Key
	if err := model.DB.Delete(&model.Key{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "key deleted"})
}

func (h *KeyHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
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
		if t, err := time.Parse("2006-01-02 15:04:05", *req.ExpiresAt); err == nil {
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
			akm := model.KeyModel{KeyID: key.ID, ModelID: modelID}
			model.DB.Create(&akm)
		}
	}

	model.DB.Preload("Models.Model").First(&key, id)

	models := make([]keyModelResponse, len(key.Models))
	for j, m := range key.Models {
		modelName := ""
		if m.Model != nil {
			modelName = m.Model.Name
		}
		models[j] = keyModelResponse{
			ID:        m.ID,
			ModelID:   m.ModelID,
			ModelName: modelName,
		}
	}

	c.JSON(http.StatusOK, gin.H{"key": keyResponse{
		ID:         key.ID,
		Key:        key.Key,
		Name:       key.Name,
		Enabled:    key.Enabled,
		AccessMode: key.AccessMode,
		ExpiresAt:  key.ExpiresAt,
		CreatedAt:  key.CreatedAt,
		Models:     models,
	}})
}

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
}

func (h *KeyHandler) ListModels(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var allModels []model.Model
	if err := model.DB.Where("enabled = ?", true).Find(&allModels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var keyModelIDs []uint
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", id).Pluck("model_id", &keyModelIDs)

	keyModelMap := make(map[uint]bool)
	for _, mid := range keyModelIDs {
		keyModelMap[mid] = true
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

		result[i] = modelWithStatusResponse{
			ID:               m.ID,
			Name:             m.Name,
			MappingCount:     len(mappings),
			MinContextWindow: minContext,
			MinMaxOutput:     minOutput,
			SupportsVision:   supportsVision,
			SupportsTools:    supportsTools,
			SupportsStream:   supportsStream,
			Selected:         keyModelMap[m.ID],
		}
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

func (h *KeyHandler) AddModel(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	modelID, err := strconv.ParseUint(c.Param("model_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	// 重复检查：该虚拟模型名是否已在该 key 的"模型厂商"直通白名单中
	if conflict := h.checkMappingModelConflict(uint(keyID), m.Name); conflict != "" {
		c.JSON(http.StatusConflict, gin.H{"error": conflict})
		return
	}

	var existing model.KeyModel
	if err := model.DB.Where("key_id = ? AND model_id = ?", keyID, modelID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "association already exists"})
		return
	}

	keyModel := model.KeyModel{
		KeyID:   uint(keyID),
		ModelID: uint(modelID),
	}
	if err := model.DB.Create(&keyModel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model association added"})
}

func (h *KeyHandler) RemoveModel(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
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

func (h *KeyHandler) ClearModels(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyModel{})

	c.JSON(http.StatusOK, gin.H{"message": "all model associations cleared"})
}

// ── Provider (模型厂商) handlers ──

type providerWithStatus struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	ModelCount int64  `json:"model_count"`
	Config     string `json:"config"`
	Protocol   string `json:"protocol"`
	KeyPrefix  string `json:"key_prefix"`
	Selected   bool   `json:"selected"`
}

func keyFormatToProtocol(format string) string {
	switch format {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "gemini":
		return "gemini"
	case "deepseek":
		return "deepseek"
	default:
		return ""
	}
}

func (h *KeyHandler) ListProviders(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	// 获取 key 格式确定协议类型
	var keyFormats []model.KeyFormat
	model.DB.Where("key_id = ?", keyID).Find(&keyFormats)

	keyFormat := "openai"
	for _, kf := range keyFormats {
		if kf.Format != "openai" {
			keyFormat = kf.Format
			break
		}
	}
	protocol := keyFormatToProtocol(keyFormat)

	// 获取该协议下所有启用的 Provider
	var providers []model.Provider
	fieldMap := map[string]string{
		"openai":    "openai_base_url != ''",
		"anthropic": "anthropic_base_url != ''",
		"gemini":    "gemini_base_url != ''",
		"deepseek":  "deepseek_base_url != ''",
	}
	if cond, ok := fieldMap[protocol]; ok {
		model.DB.Where("enabled = ?", true).Where(cond).Find(&providers)
	}

	// 获取已选中的 provider id
	var selected []model.KeyProvider
	model.DB.Where("key_id = ?", keyID).Find(&selected)
	selectedIDs := make(map[uint]bool)
	for _, s := range selected {
		selectedIDs[s.ProviderID] = true
	}

	result := make([]providerWithStatus, 0, len(providers))
	for _, p := range providers {
		var count int64
		model.DB.Model(&model.ProviderModel{}).Where("provider_id = ?", p.ID).Count(&count)

		result = append(result, providerWithStatus{
			ID:         p.ID,
			Name:       p.Name,
			ModelCount: count,
			Config:     p.Config,
			Protocol:   protocol,
			KeyPrefix:  "",
			Selected:   selectedIDs[p.ID],
		})
	}

	c.JSON(http.StatusOK, gin.H{"providers": result})
}

func (h *KeyHandler) AddProvider(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	providerID, err := strconv.ParseUint(c.Param("provider_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	var prov model.Provider
	if err := model.DB.First(&prov, providerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	var existing model.KeyProvider
	if err := model.DB.Where("key_id = ? AND provider_id = ?", keyID, providerID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "already added"})
		return
	}

	kp := model.KeyProvider{
		KeyID:      uint(keyID),
		ProviderID: uint(providerID),
	}
	model.DB.Create(&kp)

	c.JSON(http.StatusOK, gin.H{"message": "provider added"})
}

func (h *KeyHandler) RemoveProvider(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	providerID, err := strconv.ParseUint(c.Param("provider_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	model.DB.Where("key_id = ? AND provider_id = ?", keyID, providerID).Delete(&model.KeyProvider{})

	c.JSON(http.StatusOK, gin.H{"message": "provider removed"})
}

func (h *KeyHandler) ClearProviders(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyProvider{})

	c.JSON(http.StatusOK, gin.H{"message": "all providers cleared"})
}

// =============================================================================
// KeyProviderModel — 模型ID级别的直通白名单
// =============================================================================

type providerModelWithStatusResponse struct {
	ID            uint   `json:"id"`
	ProviderID    uint   `json:"provider_id"`
	ProviderName  string `json:"provider_name"`
	ModelID       string `json:"model_id"`
	DisplayName   string `json:"display_name"`
	OwnedBy       string `json:"owned_by"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	Selected      bool   `json:"selected"`
}

// ListProviderModels 列出某 key 可直通的模型（按厂商分组返回）
// GET /keys/:id/provider-models
func (h *KeyHandler) ListProviderModels(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	// 获取该 key 已选中的 provider_model_id
	var selectedIDs []uint
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", keyID).Pluck("provider_model_id", &selectedIDs)
	selectedMap := make(map[uint]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	// 获取 key 格式确定协议类型
	var keyFormats []model.KeyFormat
	model.DB.Where("key_id = ?", keyID).Find(&keyFormats)
	keyFormat := "openai"
	for _, kf := range keyFormats {
		if kf.Format != "openai" {
			keyFormat = kf.Format
			break
		}
	}
	protocol := keyFormatToProtocol(keyFormat)

	// 按协议过滤：只获取匹配协议的 provider_models
	fieldMap := map[string]string{
		"openai":    "providers.openai_base_url != ''",
		"anthropic": "providers.anthropic_base_url != ''",
		"gemini":    "providers.gemini_base_url != ''",
		"deepseek":  "providers.deepseek_base_url != ''",
	}

	var pms []model.ProviderModel
	query := model.DB.Preload("Provider").
		Joins("JOIN key_provider_models kpm ON kpm.provider_model_id = provider_models.id AND kpm.key_id = ?", uint(keyID)).
		Joins("JOIN providers ON providers.id = provider_models.provider_id AND providers.enabled = ?", true)

	if cond, ok := fieldMap[protocol]; ok {
		query = query.Where(cond)
	}
	query.Find(&pms)

	result := make([]providerModelWithStatusResponse, 0, len(pms))
	for _, pm := range pms {
		providerName := ""
		if pm.Provider != nil {
			providerName = pm.Provider.Name
		}
		result = append(result, providerModelWithStatusResponse{
			ID:            pm.ID,
			ProviderID:    pm.ProviderID,
			ProviderName:  providerName,
			ModelID:       pm.ModelID,
			DisplayName:   pm.DisplayName,
			OwnedBy:       pm.OwnedBy,
			ContextWindow: pm.ContextWindow,
			MaxOutput:     pm.MaxOutput,
			Selected:      selectedMap[pm.ID],
		})
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

// AddProviderModel 添加一个直通模型到白名单
// POST /keys/:id/provider-models/:pmid
func (h *KeyHandler) AddProviderModel(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	pmid, err := strconv.ParseUint(c.Param("pmid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider model id"})
		return
	}

	var pm model.ProviderModel
	if err := model.DB.First(&pm, pmid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider model not found"})
		return
	}

	// 重复检查：该 model_id 是否已在该 key 的"模型映射"白名单中
	if conflict := h.checkModelIDConflict(uint(keyID), pm.ModelID); conflict != "" {
		c.JSON(http.StatusConflict, gin.H{"error": conflict})
		return
	}

	var existing model.KeyProviderModel
	if err := model.DB.Where("key_id = ? AND provider_model_id = ?", keyID, pmid).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "already added"})
		return
	}

	kpm := model.KeyProviderModel{
		KeyID:           uint(keyID),
		ProviderModelID: uint(pmid),
	}
	if err := model.DB.Create(&kpm).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "provider model added"})
}

// RemoveProviderModel 从直通白名单移除一个模型
// DELETE /keys/:id/provider-models/:pmid
func (h *KeyHandler) RemoveProviderModel(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	pmid, err := strconv.ParseUint(c.Param("pmid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider model id"})
		return
	}
	model.DB.Where("key_id = ? AND provider_model_id = ?", keyID, pmid).Delete(&model.KeyProviderModel{})
	c.JSON(http.StatusOK, gin.H{"message": "provider model removed"})
}

// ClearProviderModels 清空直通白名单（= 全部允许）
// DELETE /keys/:id/provider-models
func (h *KeyHandler) ClearProviderModels(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyProviderModel{})
	c.JSON(http.StatusOK, gin.H{"message": "all provider models cleared"})
}

// checkModelIDConflict 检查某个 modelID 是否同时出现在该 key 的"模型映射"白名单中。
// 返回非空字符串表示有冲突（含冲突详情），空字符串表示无冲突。
func (h *KeyHandler) checkModelIDConflict(keyID uint, modelID string) string {
	// 该 provider_model 的 model_id 是否与某个虚拟模型(models.name)同名，
	// 且该虚拟模型已在该 key 的 key_models 白名单中。
	var kmIDs []uint
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Pluck("model_id", &kmIDs)
	if len(kmIDs) == 0 {
		return "" // 映射白名单为空 = 全部允许，不构成"重复"（直通优先即可）
	}
	var m model.Model
	if err := model.DB.Where("name = ? AND id IN ?", modelID, kmIDs).First(&m).Error; err != nil {
		return ""
	}
	return fmt.Sprintf("该模型已在「模型映射」白名单中，请先从「模型映射」中移除 %s，再添加为直通模型", modelID)
}

// checkMappingModelConflict 检查添加虚拟模型到映射白名单时，是否与直通白名单冲突。
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

func (h *KeyHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var key model.Key
	if err := model.DB.Preload("Formats").First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
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
			ID:        key.ID,
			Key:       maskedKey,
			Name:      key.Name,
			Enabled:   key.Enabled,
			ExpiresAt: key.ExpiresAt,
			CreatedAt: key.CreatedAt,
			Formats:   formats,
		},
	})
}

type resetKeyRequest struct {
	Format string `json:"format"` // "openai", "anthropic", "gemini"
}

func (h *KeyHandler) Reset(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req resetKeyRequest
	c.ShouldBindJSON(&req) // optional body

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	// 如果指定了format则使用对应格式，否则使用默认sk-前缀
	rawKey := generateKey()
	if req.Format != "" {
		rawKey = generateKeyFormat(req.Format)
	}

	if err := model.DB.Model(&key).Update("key", rawKey).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete existing key formats and generate new ones
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
		models[j] = keyModelResponse{
			ID:        m.ID,
			ModelID:   m.ModelID,
			ModelName: modelName,
		}
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
			ID:         key.ID,
			Key:        maskedKey,
			Name:       key.Name,
			Enabled:    key.Enabled,
			AccessMode: key.AccessMode,
			ExpiresAt:  key.ExpiresAt,
			CreatedAt:  key.CreatedAt,
			Models:     models,
			Formats:    maskedFormats,
		},
		"raw_key": rawKey,
		"formats": formats,
	})
}

type toolWithStatusResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MCPID       uint   `json:"mcp_id"`
	MCPName     string `json:"mcp_name"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
}

func (h *KeyHandler) GetMCPTools(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var allTools []model.MCPTool
	if err := model.DB.Preload("MCP", "enabled = ?", true).
		Joins("LEFT JOIN mcps ON mcps.id = mcp_tools.mcp_id").
		Where("mcps.enabled = ? AND mcp_tools.enabled = ?", true, true).
		Find(&allTools).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var keyToolIDs []uint
	model.DB.Model(&model.KeyMCPTool{}).Where("key_id = ?", id).Pluck("tool_id", &keyToolIDs)

	keyToolMap := make(map[uint]bool)
	for _, tid := range keyToolIDs {
		keyToolMap[tid] = true
	}

	result := make([]toolWithStatusResponse, len(allTools))
	for i, t := range allTools {
		mcpName := ""
		if t.MCP != nil {
			mcpName = t.MCP.Name
		}
		result[i] = toolWithStatusResponse{
			ID:          t.ID,
			Name:        t.Name,
			MCPID:       t.MCPID,
			MCPName:     mcpName,
			Description: t.Description,
			Selected:    keyToolMap[t.ID],
		}
	}

	c.JSON(http.StatusOK, gin.H{"tools": result})
}

func (h *KeyHandler) AddMCPTool(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	var tool model.MCPTool
	if err := model.DB.First(&tool, toolID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	var existing model.KeyMCPTool
	if err := model.DB.Where("key_id = ? AND tool_id = ?", keyID, toolID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "association already exists"})
		return
	}

	keyTool := model.KeyMCPTool{
		KeyID:  uint(keyID),
		ToolID: uint(toolID),
	}
	if err := model.DB.Create(&keyTool).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tool association added"})
}

func (h *KeyHandler) RemoveMCPTool(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool id"})
		return
	}

	model.DB.Where("key_id = ? AND tool_id = ?", keyID, toolID).Delete(&model.KeyMCPTool{})

	c.JSON(http.StatusOK, gin.H{"message": "tool association removed"})
}

func (h *KeyHandler) ClearMCPTools(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPTool{})

	c.JSON(http.StatusOK, gin.H{"message": "all tool associations cleared"})
}

func (h *KeyHandler) UpdateMCPTools(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		ToolIDs []uint `json:"tool_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPTool{})

	for _, toolID := range req.ToolIDs {
		var tool model.MCPTool
		if err := model.DB.First(&tool, toolID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tool not found: " + strconv.FormatUint(uint64(toolID), 10)})
			return
		}
		keyTool := model.KeyMCPTool{
			KeyID:  key.ID,
			ToolID: toolID,
		}
		model.DB.Create(&keyTool)
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP tools updated"})
}

type resourceWithStatusResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MCPID       uint   `json:"mcp_id"`
	MCPName     string `json:"mcp_name"`
	Description string `json:"description"`
	URI         string `json:"uri"`
	MimeType    string `json:"mime_type"`
	Selected    bool   `json:"selected"`
}

func (h *KeyHandler) GetMCPResources(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var allResources []model.MCPResource
	if err := model.DB.Preload("MCP", "enabled = ?", true).
		Joins("LEFT JOIN mcps ON mcps.id = mcp_resources.mcp_id").
		Where("mcps.enabled = ? AND mcp_resources.enabled = ?", true, true).
		Find(&allResources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var keyResourceIDs []uint
	model.DB.Model(&model.KeyMCPResource{}).Where("key_id = ?", id).Pluck("resource_id", &keyResourceIDs)

	keyResourceMap := make(map[uint]bool)
	for _, rid := range keyResourceIDs {
		keyResourceMap[rid] = true
	}

	result := make([]resourceWithStatusResponse, len(allResources))
	for i, r := range allResources {
		mcpName := ""
		if r.MCP != nil {
			mcpName = r.MCP.Name
		}
		result[i] = resourceWithStatusResponse{
			ID:          r.ID,
			Name:        r.Name,
			MCPID:       r.MCPID,
			MCPName:     mcpName,
			Description: r.Description,
			URI:         r.URI,
			MimeType:    r.MimeType,
			Selected:    keyResourceMap[r.ID],
		}
	}

	c.JSON(http.StatusOK, gin.H{"resources": result})
}

func (h *KeyHandler) AddMCPResource(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	var resource model.MCPResource
	if err := model.DB.First(&resource, resourceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	var existing model.KeyMCPResource
	if err := model.DB.Where("key_id = ? AND resource_id = ?", keyID, resourceID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "association already exists"})
		return
	}

	keyResource := model.KeyMCPResource{
		KeyID:      uint(keyID),
		ResourceID: uint(resourceID),
	}
	if err := model.DB.Create(&keyResource).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "resource association added"})
}

func (h *KeyHandler) RemoveMCPResource(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource id"})
		return
	}

	model.DB.Where("key_id = ? AND resource_id = ?", keyID, resourceID).Delete(&model.KeyMCPResource{})

	c.JSON(http.StatusOK, gin.H{"message": "resource association removed"})
}

func (h *KeyHandler) ClearMCPResources(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPResource{})

	c.JSON(http.StatusOK, gin.H{"message": "all resource associations cleared"})
}

func (h *KeyHandler) UpdateMCPResources(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		ResourceIDs []uint `json:"resource_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPResource{})

	for _, resourceID := range req.ResourceIDs {
		var resource model.MCPResource
		if err := model.DB.First(&resource, resourceID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resource not found: " + strconv.FormatUint(uint64(resourceID), 10)})
			return
		}
		keyResource := model.KeyMCPResource{
			KeyID:      key.ID,
			ResourceID: resourceID,
		}
		model.DB.Create(&keyResource)
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP resources updated"})
}

type promptWithStatusResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MCPID       uint   `json:"mcp_id"`
	MCPName     string `json:"mcp_name"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
}

func (h *KeyHandler) GetMCPPrompts(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var allPrompts []model.MCPPrompt
	if err := model.DB.Preload("MCP", "enabled = ?", true).
		Joins("LEFT JOIN mcps ON mcps.id = mcp_prompts.mcp_id").
		Where("mcps.enabled = ? AND mcp_prompts.enabled = ?", true, true).
		Find(&allPrompts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var keyPromptIDs []uint
	model.DB.Model(&model.KeyMCPPrompt{}).Where("key_id = ?", id).Pluck("prompt_id", &keyPromptIDs)

	keyPromptMap := make(map[uint]bool)
	for _, pid := range keyPromptIDs {
		keyPromptMap[pid] = true
	}

	result := make([]promptWithStatusResponse, len(allPrompts))
	for i, p := range allPrompts {
		mcpName := ""
		if p.MCP != nil {
			mcpName = p.MCP.Name
		}
		result[i] = promptWithStatusResponse{
			ID:          p.ID,
			Name:        p.Name,
			MCPID:       p.MCPID,
			MCPName:     mcpName,
			Description: p.Description,
			Selected:    keyPromptMap[p.ID],
		}
	}

	c.JSON(http.StatusOK, gin.H{"prompts": result})
}

func (h *KeyHandler) AddMCPPrompt(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	promptID, err := strconv.ParseUint(c.Param("prompt_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prompt id"})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, keyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	var prompt model.MCPPrompt
	if err := model.DB.First(&prompt, promptID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "prompt not found"})
		return
	}

	var existing model.KeyMCPPrompt
	if err := model.DB.Where("key_id = ? AND prompt_id = ?", keyID, promptID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "association already exists"})
		return
	}

	keyPrompt := model.KeyMCPPrompt{
		KeyID:    uint(keyID),
		PromptID: uint(promptID),
	}
	if err := model.DB.Create(&keyPrompt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "prompt association added"})
}

func (h *KeyHandler) RemoveMCPPrompt(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	promptID, err := strconv.ParseUint(c.Param("prompt_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prompt id"})
		return
	}

	model.DB.Where("key_id = ? AND prompt_id = ?", keyID, promptID).Delete(&model.KeyMCPPrompt{})

	c.JSON(http.StatusOK, gin.H{"message": "prompt association removed"})
}

func (h *KeyHandler) ClearMCPPrompts(c *gin.Context) {
	keyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPPrompt{})

	c.JSON(http.StatusOK, gin.H{"message": "all prompt associations cleared"})
}

func (h *KeyHandler) UpdateMCPPrompts(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		PromptIDs []uint `json:"prompt_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var key model.Key
	if err := model.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPPrompt{})

	for _, promptID := range req.PromptIDs {
		var prompt model.MCPPrompt
		if err := model.DB.First(&prompt, promptID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt not found: " + strconv.FormatUint(uint64(promptID), 10)})
			return
		}
		keyPrompt := model.KeyMCPPrompt{
			KeyID:    key.ID,
			PromptID: promptID,
		}
		model.DB.Create(&keyPrompt)
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP prompts updated"})
}
