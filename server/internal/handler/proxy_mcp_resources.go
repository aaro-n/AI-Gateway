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
// MCP Resources 代理处理
// =============================================================================

// handleResourcesList 处理 resources/list 请求
func (h *MCPProxyHandler) handleResourcesList(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	apiKey := c.MustGet("api_key").(*model.Key)

	var resources []interface{}

	if h.hasFullResourceAccess(apiKey.ID) {
		var allResources []model.MCPResource
		model.DB.Preload("MCP").Where("enabled = ?", true).Find(&allResources)
		for _, r := range allResources {
			if r.MCP != nil && r.MCP.Enabled {
				resources = append(resources, map[string]interface{}{
					"uri":         "mcp://" + r.MCP.Name + "/" + r.URI,
					"name":        r.Name,
					"description": r.Description,
					"mimeType":    r.MimeType,
				})
			}
		}
	} else {
		var keyResources []model.KeyMCPResource
		model.DB.Preload("Resource.MCP").Where("key_id = ?", apiKey.ID).Find(&keyResources)
		for _, kr := range keyResources {
			if kr.Resource != nil && kr.Resource.MCP != nil && kr.Resource.Enabled && kr.Resource.MCP.Enabled {
				resources = append(resources, map[string]interface{}{
					"uri":         "mcp://" + kr.Resource.MCP.Name + "/" + kr.Resource.URI,
					"name":        kr.Resource.Name,
					"description": kr.Resource.Description,
					"mimeType":    kr.Resource.MimeType,
				})
			}
		}
	}

	return mcp.NewResponse(req.ID, map[string]interface{}{
		"resources": resources,
	})
}

// handleResourcesRead 处理 resources/read 请求
func (h *MCPProxyHandler) handleResourcesRead(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	apiKey := c.MustGet("api_key").(*model.Key)
	startTime := time.Now()
	clientIPs := utils.GetClientIPInfo(c)

	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.ErrInvalidParams)
	}

	if !strings.HasPrefix(params.URI, "mcp://") {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: "invalid resource URI format, expected: mcp://mcp_name/original_uri",
		})
	}

	uriWithoutPrefix := strings.TrimPrefix(params.URI, "mcp://")
	parts := strings.SplitN(uriWithoutPrefix, "/", 2)
	if len(parts) != 2 {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: "invalid resource URI format, expected: mcp://mcp_name/original_uri",
		})
	}

	mcpName := parts[0]
	originalURI := parts[1]

	var m model.MCP
	if err := model.DB.Where("name = ? AND enabled = ?", mcpName, true).First(&m).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("MCP not found: %s", mcpName),
		})
	}

	var resource model.MCPResource
	if err := model.DB.Where("mcp_id = ? AND uri = ? AND enabled = ?", m.ID, originalURI, true).First(&resource).Error; err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
			Code:    mcp.ErrInvalidParams.Code,
			Message: fmt.Sprintf("resource not found: %s", params.URI),
		})
	}

	if !h.hasFullResourceAccess(apiKey.ID) {
		var keyResource model.KeyMCPResource
		if err := model.DB.Where("key_id = ? AND resource_id = ?", apiKey.ID, resource.ID).First(&keyResource).Error; err != nil {
			return mcp.NewErrorResponse(req.ID, &mcp.RPCError{
				Code:    mcp.ErrInvalidParams.Code,
				Message: "permission denied",
			})
		}
	}

	resp, err := mcpManager.ReadResource(&m, originalURI)
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

	mcpLog := NewMCPLog(
		"default",
		clientIPs,
		apiKey.ID,
		apiKey.Name,
		m.ID,
		m.Name,
		m.Type,
		"resource",
		originalURI,
		"read",
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

// handleResourcesSubscribe 处理 resources/subscribe 请求（暂不支持）
func (h *MCPProxyHandler) handleResourcesSubscribe(c *gin.Context, req *mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	return mcp.NewErrorResponse(req.ID, mcp.ErrMethodNotFound)
}
