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
// MCP Tools 代理处理
// =============================================================================

// handleToolsList 处理 tools/list 请求
func (h *MCPProxyHandler) handleToolsList(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	apiKey := c.MustGet("api_key").(*model.Key)

	var tools []interface{}

	if h.hasFullToolAccess(apiKey.ID) {
		var allTools []model.MCPTool
		model.DB.Preload("MCP").Where("enabled = ?", true).Find(&allTools)
		for _, t := range allTools {
			if t.MCP != nil && t.MCP.Enabled {
				tools = append(tools, map[string]interface{}{
					"name":        t.MCP.Name + "." + t.Name,
					"description": t.Description,
					"inputSchema": json.RawMessage(t.InputSchema),
				})
			}
		}
	} else {
		var keyTools []model.KeyMCPTool
		model.DB.Preload("Tool.MCP").Where("key_id = ?", apiKey.ID).Find(&keyTools)
		for _, kt := range keyTools {
			if kt.Tool != nil && kt.Tool.MCP != nil && kt.Tool.Enabled && kt.Tool.MCP.Enabled {
				tools = append(tools, map[string]interface{}{
					"name":        kt.Tool.MCP.Name + "." + kt.Tool.Name,
					"description": kt.Tool.Description,
					"inputSchema": json.RawMessage(kt.Tool.InputSchema),
				})
			}
		}
	}

	return mcp.NewResponse(req.ID, map[string]interface{}{
		"tools": tools,
	})
}

// handleToolsCall 处理 tools/call 请求
func (h *MCPProxyHandler) handleToolsCall(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
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
			Message: "invalid tool name format, expected: mcp_name.tool_name",
		})
	}

	mcpName := parts[0]
	toolName := parts[1]

	var m model.MCP
	if err := model.DB.Where("name = ? AND enabled = ?", mcpName, true).First(&m).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("MCP not found: %s", mcpName),
		})
	}

	var tool model.MCPTool
	if err := model.DB.Where("mcp_id = ? AND name = ? AND enabled = ?", m.ID, toolName, true).First(&tool).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("tool not found: %s", params.Name),
		})
	}

	if !h.hasFullToolAccess(apiKey.ID) {
		var keyTool model.KeyMCPTool
		if err := model.DB.Where("key_id = ? AND tool_id = ?", apiKey.ID, tool.ID).First(&keyTool).Error; err != nil {
			return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
				Code:    mcp.ErrInvalidParams.Code,
				Message: "permission denied",
			})
		}
	}

	resp, err := mcpManager.CallTool(&m, toolName, params.Arguments)
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
		"tool",
		toolName,
		"call",
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
