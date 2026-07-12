package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
)

// providerWithStatus Provider 列表项（含 selected 标记）
type providerWithStatus struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	ModelCount int64  `json:"model_count"`
	Config     string `json:"config"`
	Protocol   string `json:"protocol"`
	KeyPrefix  string `json:"key_prefix"`
	Selected   bool   `json:"selected"`
}

// providerModelWithStatusResponse ProviderModel 列表项（含 selected/enabled 标记）
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
	Enabled       bool   `json:"enabled"`
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
	case "openrouter":
		return "openrouter"
	default:
		return ""
	}
}

// =============================================================================
// Provider（模型厂商）管理
// =============================================================================

// ListProviders 列出某 key 可访问的模型厂商
// GET /keys/:id/providers
func (h *KeyHandler) ListProviders(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
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
	var allProviders []model.Provider
	model.DB.Where("enabled = ?", true).Find(&allProviders)
	providers := make([]model.Provider, 0, len(allProviders))
	for _, p := range allProviders {
		if p.EndpointFor(protocol) != "" {
			providers = append(providers, p)
		}
	}

	// 对于非 admin 用户，只展示已授权的厂商
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		authorizedIDs, err := GetUserProviderIDs(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		authSet := make(map[uint]bool)
		for _, pid := range authorizedIDs {
			authSet[pid] = true
		}
		filtered := make([]model.Provider, 0)
		for _, p := range providers {
			if authSet[p.ID] {
				filtered = append(filtered, p)
			}
		}
		providers = filtered
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
			Selected:   selectedIDs[p.ID],
		})
	}

	c.JSON(http.StatusOK, gin.H{"providers": result})
}

// AddProvider 添加模型厂商
// POST /keys/:id/providers/:provider_id
func (h *KeyHandler) AddProvider(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	providerID, err := strconv.ParseUint(c.Param("provider_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	key := h.checkKeyOwnership(c, keyID)
	if key == nil {
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

	model.DB.Create(&model.KeyProvider{KeyID: uint(keyID), ProviderID: uint(providerID)})
	c.JSON(http.StatusOK, gin.H{"message": "provider added"})
}

// RemoveProvider 移除模型厂商
// DELETE /keys/:id/providers/:provider_id
func (h *KeyHandler) RemoveProvider(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
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

// ClearProviders 清除所有模型厂商
// DELETE /keys/:id/providers
func (h *KeyHandler) ClearProviders(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyProvider{})
	c.JSON(http.StatusOK, gin.H{"message": "all providers cleared"})
}

// =============================================================================
// ProviderModel（模型 ID 级别直通白名单）管理
// =============================================================================

// ListProviderModels 列出某 key 可直通的模型
// GET /keys/:id/provider-models
func (h *KeyHandler) ListProviderModels(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	// 获取白名单记录
	var kpmRows []model.KeyProviderModel
	model.DB.Where("key_id = ?", keyID).Find(&kpmRows)
	kpmMap := make(map[uint]model.KeyProviderModel)
	for _, r := range kpmRows {
		kpmMap[r.ProviderModelID] = r
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

	// 按协议过滤
	var allPms []model.ProviderModel
	model.DB.Preload("Provider").
		Joins("JOIN key_provider_models kpm ON kpm.provider_model_id = provider_models.id AND kpm.key_id = ?", uint(keyID)).
		Joins("JOIN providers ON providers.id = provider_models.provider_id AND providers.enabled = ?", true).
		Order("provider_models.id ASC").
		Find(&allPms)

	pms := make([]model.ProviderModel, 0, len(allPms))
	for _, pm := range allPms {
		if pm.Provider != nil && pm.Provider.EndpointFor(protocol) != "" {
			pms = append(pms, pm)
		}
	}

	result := make([]providerModelWithStatusResponse, 0, len(pms))
	for _, pm := range pms {
		providerName := ""
		if pm.Provider != nil {
			providerName = pm.Provider.Name
		}
		row, exists := kpmMap[pm.ID]
		result = append(result, providerModelWithStatusResponse{
			ID:            pm.ID,
			ProviderID:    pm.ProviderID,
			ProviderName:  providerName,
			ModelID:       pm.ModelID,
			DisplayName:   pm.DisplayName,
			OwnedBy:       pm.OwnedBy,
			ContextWindow: pm.ContextWindow,
			MaxOutput:     pm.MaxOutput,
			Selected:      exists,
			Enabled:       exists && row.Enabled,
		})
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

// AddProviderModel 添加直通模型（upsert）
// POST /keys/:id/provider-models/:pmid
func (h *KeyHandler) AddProviderModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if h.checkKeyOwnership(c, keyID) == nil {
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

	if conflict := h.checkModelIDConflict(uint(keyID), pm.ModelID); conflict != "" {
		c.JSON(http.StatusConflict, gin.H{"error": conflict})
		return
	}

	var existing model.KeyProviderModel
	if err := model.DB.Where("key_id = ? AND provider_model_id = ?", keyID, pmid).First(&existing).Error; err == nil {
		model.DB.Model(&existing).Update("enabled", true)
		c.JSON(http.StatusOK, gin.H{"message": "provider model enabled"})
		return
	}

	model.DB.Create(&model.KeyProviderModel{KeyID: uint(keyID), ProviderModelID: uint(pmid), Enabled: true})
	c.JSON(http.StatusOK, gin.H{"message": "provider model added"})
}

// RemoveProviderModel 从直通白名单移除
// DELETE /keys/:id/provider-models/:pmid
func (h *KeyHandler) RemoveProviderModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
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

// ClearProviderModels 批量禁用直通白名单
// DELETE /keys/:id/provider-models
func (h *KeyHandler) ClearProviderModels(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", keyID).Update("enabled", false)
	c.JSON(http.StatusOK, gin.H{"message": "all provider models disabled"})
}

// EnableAllProviderModels 批量启用直通白名单
// PUT /keys/:id/provider-models
func (h *KeyHandler) EnableAllProviderModels(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", keyID).Update("enabled", true)
	c.JSON(http.StatusOK, gin.H{"message": "all provider models enabled"})
}

// ToggleProviderModel 切换直通模型启用状态
// PUT /keys/:id/provider-models/:pmid
func (h *KeyHandler) ToggleProviderModel(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}
	pmid, err := strconv.ParseUint(c.Param("pmid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider model id"})
		return
	}

	var kpm model.KeyProviderModel
	if err := model.DB.Where("key_id = ? AND provider_model_id = ?", keyID, pmid).First(&kpm).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider model not in whitelist"})
		return
	}
	newVal := !kpm.Enabled
	model.DB.Model(&kpm).Update("enabled", newVal)
	c.JSON(http.StatusOK, gin.H{"message": "toggled", "enabled": newVal})
}
