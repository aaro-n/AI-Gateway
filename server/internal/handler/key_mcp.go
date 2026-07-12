package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
)

// =============================================================================
// MCP 相关响应类型
// =============================================================================

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

type toolWithStatusResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MCPID       uint   `json:"mcp_id"`
	MCPName     string `json:"mcp_name"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
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

type promptWithStatusResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MCPID       uint   `json:"mcp_id"`
	MCPName     string `json:"mcp_name"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
}

// =============================================================================
// MCP 工具管理
// =============================================================================

// GetMCPTools 列出某 key 的 MCP 工具
// GET /keys/:id/mcp-tools
func (h *KeyHandler) GetMCPTools(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.checkKeyOwnership(c, id) == nil {
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

// AddMCPTool 添加 MCP 工具
// POST /keys/:id/mcp-tools/:tool_id
func (h *KeyHandler) AddMCPTool(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool id"})
		return
	}

	key := h.checkKeyOwnership(c, keyID)
	if key == nil {
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

	if err := model.DB.Create(&model.KeyMCPTool{KeyID: uint(keyID), ToolID: uint(toolID)}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tool association added"})
}

// RemoveMCPTool 移除 MCP 工具
// DELETE /keys/:id/mcp-tools/:tool_id
func (h *KeyHandler) RemoveMCPTool(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
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

// ClearMCPTools 清除所有 MCP 工具
// DELETE /keys/:id/mcp-tools
func (h *KeyHandler) ClearMCPTools(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPTool{})
	c.JSON(http.StatusOK, gin.H{"message": "all tool associations cleared"})
}

// UpdateMCPTools 批量替换 MCP 工具
// PUT /keys/:id/mcp-tools
func (h *KeyHandler) UpdateMCPTools(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	key := h.checkKeyOwnership(c, id)
	if key == nil {
		return
	}

	var req struct {
		ToolIDs []uint `json:"tool_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPTool{})

	for _, toolID := range req.ToolIDs {
		var tool model.MCPTool
		if err := model.DB.First(&tool, toolID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tool not found: " + strconv.FormatUint(uint64(toolID), 10)})
			return
		}
		model.DB.Create(&model.KeyMCPTool{KeyID: key.ID, ToolID: toolID})
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP tools updated"})
}

// =============================================================================
// MCP 资源管理
// =============================================================================

// GetMCPResources 列出某 key 的 MCP 资源
// GET /keys/:id/mcp-resources
func (h *KeyHandler) GetMCPResources(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.checkKeyOwnership(c, id) == nil {
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

// AddMCPResource 添加 MCP 资源
// POST /keys/:id/mcp-resources/:resource_id
func (h *KeyHandler) AddMCPResource(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resource id"})
		return
	}

	key := h.checkKeyOwnership(c, keyID)
	if key == nil {
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

	if err := model.DB.Create(&model.KeyMCPResource{KeyID: uint(keyID), ResourceID: uint(resourceID)}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "resource association added"})
}

// RemoveMCPResource 移除 MCP 资源
// DELETE /keys/:id/mcp-resources/:resource_id
func (h *KeyHandler) RemoveMCPResource(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
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

// ClearMCPResources 清除所有 MCP 资源
// DELETE /keys/:id/mcp-resources
func (h *KeyHandler) ClearMCPResources(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPResource{})
	c.JSON(http.StatusOK, gin.H{"message": "all resource associations cleared"})
}

// UpdateMCPResources 批量替换 MCP 资源
// PUT /keys/:id/mcp-resources
func (h *KeyHandler) UpdateMCPResources(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	key := h.checkKeyOwnership(c, id)
	if key == nil {
		return
	}

	var req struct {
		ResourceIDs []uint `json:"resource_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPResource{})

	for _, resourceID := range req.ResourceIDs {
		var resource model.MCPResource
		if err := model.DB.First(&resource, resourceID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resource not found: " + strconv.FormatUint(uint64(resourceID), 10)})
			return
		}
		model.DB.Create(&model.KeyMCPResource{KeyID: key.ID, ResourceID: resourceID})
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP resources updated"})
}

// =============================================================================
// MCP 提示管理
// =============================================================================

// GetMCPPrompts 列出某 key 的 MCP 提示
// GET /keys/:id/mcp-prompts
func (h *KeyHandler) GetMCPPrompts(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.checkKeyOwnership(c, id) == nil {
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

// AddMCPPrompt 添加 MCP 提示
// POST /keys/:id/mcp-prompts/:prompt_id
func (h *KeyHandler) AddMCPPrompt(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	promptID, err := strconv.ParseUint(c.Param("prompt_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prompt id"})
		return
	}

	key := h.checkKeyOwnership(c, keyID)
	if key == nil {
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

	if err := model.DB.Create(&model.KeyMCPPrompt{KeyID: uint(keyID), PromptID: uint(promptID)}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "prompt association added"})
}

// RemoveMCPPrompt 移除 MCP 提示
// DELETE /keys/:id/mcp-prompts/:prompt_id
func (h *KeyHandler) RemoveMCPPrompt(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
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

// ClearMCPPrompts 清除所有 MCP 提示
// DELETE /keys/:id/mcp-prompts
func (h *KeyHandler) ClearMCPPrompts(c *gin.Context) {
	keyID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}
	if h.checkKeyOwnership(c, keyID) == nil {
		return
	}

	model.DB.Where("key_id = ?", keyID).Delete(&model.KeyMCPPrompt{})
	c.JSON(http.StatusOK, gin.H{"message": "all prompt associations cleared"})
}

// UpdateMCPPrompts 批量替换 MCP 提示
// PUT /keys/:id/mcp-prompts
func (h *KeyHandler) UpdateMCPPrompts(c *gin.Context) {
	id, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	key := h.checkKeyOwnership(c, id)
	if key == nil {
		return
	}

	var req struct {
		PromptIDs []uint `json:"prompt_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model.DB.Where("key_id = ?", id).Delete(&model.KeyMCPPrompt{})

	for _, promptID := range req.PromptIDs {
		var prompt model.MCPPrompt
		if err := model.DB.First(&prompt, promptID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt not found: " + strconv.FormatUint(uint64(promptID), 10)})
			return
		}
		model.DB.Create(&model.KeyMCPPrompt{KeyID: key.ID, PromptID: promptID})
	}

	c.JSON(http.StatusOK, gin.H{"message": "MCP prompts updated"})
}
