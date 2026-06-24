package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	coreErrors "ai-gateway/internal/core/errors"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/model"
	"ai-gateway/internal/router"
)

// UnifiedGatewayHandler 统一网关代理（基于 Registry + 协议插件）
type UnifiedGatewayHandler struct {
	modelRouter *router.ModelRouter
}

func NewUnifiedGatewayHandler() *UnifiedGatewayHandler {
	return &UnifiedGatewayHandler{
		modelRouter: router.GetRouter(),
	}
}

// Handle 统一处理 /gateway/:protocol/*
func (h *UnifiedGatewayHandler) Handle(c *gin.Context) {
	protocolName := c.Param("protocol")

	// 1. 获取协议描述符
	protoDesc, ok := registry.Get(protocolName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unsupported protocol: " + protocolName})
		return
	}

	// 2. 提取并验证 API Key
	rawKey := protoDesc.AuthExtractor(c)
	if rawKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		return
	}

	var kf model.KeyFormat
	if err := model.DB.Where("formatted_key = ?", rawKey).First(&kf).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}
	if kf.Format != protocolName {
		c.JSON(http.StatusForbidden, gin.H{"error": "key format not allowed on this endpoint"})
		return
	}

	var apiKey model.Key
	if err := model.DB.Where("id = ? AND enabled = ?", kf.KeyID, true).First(&apiKey).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "API key expired"})
		return
	}

	// 3. 提取模型名
	modelName := h.extractModel(c)
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model identifier"})
		return
	}

	// 4. 权限检查
	if !h.checkModelAccess(apiKey.ID, modelName) {
		c.JSON(http.StatusForbidden, gin.H{"error": "model not allowed for this API key"})
		return
	}

	// 5. 路由
	result, routeErr := h.modelRouter.Route(modelName)
	if routeErr != nil {
		coreErrors.Error("route failed for model=%s: %v", modelName, routeErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": routeErr.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model mapping not found or no available provider"})
		return
	}

	// 6. 保存上下文
	c.Set("key_id", apiKey.ID)
	c.Set("key_name", apiKey.Name)

	originalPath := c.Request.URL.Path
	prefix := "/gateway/" + protocolName
	if strings.HasPrefix(originalPath, prefix) {
		c.Request.URL.Path = strings.TrimPrefix(originalPath, prefix)
	}

	start := time.Now()
	usage := registry.Usage{}

	// 7. 执行请求
	upstreamProtocol, execErr := h.execute(c, protocolName, result, &usage)

	c.Request.URL.Path = originalPath
	latencyMs := time.Since(start).Milliseconds()

	// 8. 日志
	status := "success"
	errMsg := ""
	if execErr != nil {
		status = "error"
		errMsg = execErr.Error()
		coreErrors.Error("gateway error: %s | %s %s | model=%s→%s | %dms",
			errMsg, c.Request.Method, originalPath,
			modelName, upstreamProtocol, latencyMs)
	}

	h.logUsage(protocolName, apiKey.ID, apiKey.Name, modelName, result,
		upstreamProtocol, &usage, int(latencyMs), status, errMsg)
}

// execute 执行请求，返回使用的上游协议
func (h *UnifiedGatewayHandler) execute(c *gin.Context, entryProtocol string,
	result *router.RouteResult, usage *registry.Usage) (string, error) {

	providerProtos := result.GetProviderProtocols()
	if len(providerProtos) == 0 {
		return "", fmt.Errorf("no protocol configured for provider")
	}

	// 同协议直通优先
	upstreamProto := ""
	for _, p := range providerProtos {
		if p == entryProtocol {
			upstreamProto = p
			break
		}
	}

	// 策略 A：非同协议且非 OpenAI 入口 → 拒绝
	if upstreamProto == "" && entryProtocol != "openai" {
		return "", fmt.Errorf("cross-protocol not supported from %s; use OpenAI endpoint", entryProtocol)
	}
	if upstreamProto == "" {
		upstreamProto = providerProtos[0]
	}

	upDesc, ok := registry.Get(upstreamProto)
	if !ok {
		return upstreamProto, fmt.Errorf("unsupported protocol: %s", upstreamProto)
	}

	baseURL := h.getBaseURL(result.Provider, upstreamProto)
	if baseURL == "" {
		return upstreamProto, fmt.Errorf("no base URL for protocol %s", upstreamProto)
	}

	prov := upDesc.NewProvider(&registry.Config{
		BaseURL: baseURL,
		APIKey:  result.Provider.APIKey,
	})

	if entryProtocol == upstreamProto {
		return upstreamProto, prov.HandleNative(c, result.ProviderModel.ModelID, usage)
	}
	return upstreamProto, prov.FromOpenAI(c, result.ProviderModel.ModelID, usage)
}

// extractModel 从请求中提取模型名
func (h *UnifiedGatewayHandler) extractModel(c *gin.Context) string {
	body, err := io.ReadAll(c.Request.Body)
	if err == nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var req map[string]interface{}
		if json.Unmarshal(body, &req) == nil {
			if m, ok := req["model"].(string); ok && m != "" {
				return m
			}
		}
	}
	// Gemini 风格 URL 提取
	path := c.Request.URL.Path
	if parts := strings.Split(path, "/models/"); len(parts) >= 2 {
		return strings.Split(parts[1], ":")[0]
	}
	return ""
}

func (h *UnifiedGatewayHandler) checkModelAccess(keyID uint, modelName string) bool {
	var count int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Count(&count)
	if count == 0 {
		return true
	}
	var m model.Model
	if err := model.DB.Where("name = ?", modelName).First(&m).Error; err != nil {
		return false
	}
	var modelCount int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ? AND model_id = ?", keyID, m.ID).Count(&modelCount)
	return modelCount > 0
}

func (h *UnifiedGatewayHandler) getBaseURL(p *model.Provider, protocol string) string {
	if p.Endpoints != "" {
		var eps map[string]string
		if json.Unmarshal([]byte(p.Endpoints), &eps) == nil {
			if url, ok := eps[protocol]; ok && url != "" {
				return strings.TrimSuffix(url, "/")
			}
		}
	}
	switch protocol {
	case "openai":
		return strings.TrimSuffix(p.OpenAIBaseURL, "/")
	case "anthropic":
		return strings.TrimSuffix(p.AnthropicBaseURL, "/")
	case "gemini":
		return strings.TrimSuffix(p.GeminiBaseURL, "/")
	}
	return ""
}

func (h *UnifiedGatewayHandler) logUsage(source string, keyID uint, keyName, modelName string,
	result *router.RouteResult, upstreamProto string, usage *registry.Usage,
	latencyMs int, status, errMsg string) {

	displayName := result.ProviderModel.DisplayName
	if displayName == "" {
		displayName = result.ProviderModel.ModelID
	}
	callMethod := "direct"
	if source != upstreamProto {
		callMethod = "convert"
	}
	mlog := model.ModelLog{
		Source:          source,
		KeyID:           keyID,
		KeyName:         keyName,
		Model:           modelName,
		ProviderID:      result.Provider.ID,
		ProviderName:    result.Provider.Name,
		ActualModelID:   result.ProviderModel.ModelID,
		ActualModelName: displayName,
		CallMethod:      callMethod,
		CachedTokens:    usage.CachedTokens,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens(),
		LatencyMs:       latencyMs,
		Status:          status,
		ErrorMsg:        errMsg,
	}
	model.DB.Create(&mlog)

	// 终端日志
	if status == "success" {
		coreErrors.Info("%s %s → %s | %s | %dt %dms",
			source, modelName, upstreamProto, callMethod, usage.TotalTokens(), latencyMs)
	}
}
