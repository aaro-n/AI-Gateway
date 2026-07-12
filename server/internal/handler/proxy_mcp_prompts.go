package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/mcp"
	"ai-gateway/internal/model"
	"ai-gateway/internal/utils"
)

// =============================================================================
// MCP Prompts 代理处理
// =============================================================================

// handlePromptsList 处理 prompts/list 请求
func (h *MCPProxyHandler) handlePromptsList(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	apiKey := c.MustGet("api_key").(*model.Key)

	var prompts []interface{}

	if h.hasFullPromptAccess(apiKey.ID) {
		var allPrompts []model.MCPPrompt
		model.DB.Preload("MCP").Where("enabled = ?", true).Find(&allPrompts)
		for _, p := range allPrompts {
			if p.MCP != nil && p.MCP.Enabled {
				prompts = append(prompts, map[string]interface{}{
					"name":        p.MCP.Name + "." + p.Name,
					"description": p.Description,
					"arguments":   json.RawMessage(p.Arguments),
				})
			}
		}
	} else {
		var keyPrompts []model.KeyMCPPrompt
		model.DB.Preload("Prompt.MCP").Where("key_id = ?", apiKey.ID).Find(&keyPrompts)
		for _, kp := range keyPrompts {
			if kp.Prompt != nil && kp.Prompt.MCP != nil && kp.Prompt.Enabled && kp.Prompt.MCP.Enabled {
				prompts = append(prompts, map[string]interface{}{
					"name":        kp.Prompt.MCP.Name + "." + kp.Prompt.Name,
					"description": kp.Prompt.Description,
					"arguments":   json.RawMessage(kp.Prompt.Arguments),
				})
			}
		}
	}

	return mcp.NewResponse(req.ID, map[string]interface{}{
		"prompts": prompts,
	})
}

// handlePromptsGet 处理 prompts/get 请求
func (h *MCPProxyHandler) handlePromptsGet(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	apiKey := c.MustGet("api_key").(*model.Key)
	startTime := time.Now()
	clientIPs := utils.GetClientIPInfo(c)

	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.ErrInvalidParams)
	}

	parts := strings.SplitN(params.Name, ".", 2)
	if len(parts) != 2 {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: "invalid prompt name format, expected: mcp_name.prompt_name",
		})
	}

	mcpName := parts[0]
	promptName := parts[1]

	var m model.MCP
	if err := model.DB.Where("name = ? AND enabled = ?", mcpName, true).First(&m).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("MCP not found: %s", mcpName),
		})
	}

	var prompt model.MCPPrompt
	if err := model.DB.Where("mcp_id = ? AND name = ? AND enabled = ?", m.ID, promptName, true).First(&prompt).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("prompt not found: %s", params.Name),
		})
	}

	if !h.hasFullPromptAccess(apiKey.ID) {
		var keyPrompt model.KeyMCPPrompt
		if err := model.DB.Where("key_id = ? AND prompt_id = ?", apiKey.ID, prompt.ID).First(&keyPrompt).Error; err != nil {
			return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
				Code:    mcp.ErrInvalidParams.Code,
				Message: "permission denied",
			})
		}
	}

	resp, err := mcpManager.GetPrompt(&m, promptName, params.Arguments)
	latencyMs := int(time.Since(startTime).Milliseconds())

	status := "success"
	errorMsg := ""
	inputSize := 0
	outputSize := 0

	if err != nil {
		status = "error"
		errorMsg = err.Error()
	} else {
		status = "success"
		if resp != nil {
			respBytes, _ := json.Marshal(resp)
			outputSize = len(respBytes)
		}
	}

	argsBytes, _ := json.Marshal(params.Arguments)
	inputSize = len(argsBytes)

	mcpLog := NewMCPLog(
		"default",
		clientIPs,
		apiKey.ID,
		apiKey.Name,
		m.ID,
		m.Name,
		m.Type,
		"prompt",
		promptName,
		"get",
		inputSize,
		outputSize,
		latencyMs,
		status,
		errorMsg,
	)
	model.DB.Create(&mcpLog)
	log.Println(mcpLog.String())

	if err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInternalError.Code,
			Message: err.Error(),
		})
	}

	resp.ID = req.ID
	return resp
}
