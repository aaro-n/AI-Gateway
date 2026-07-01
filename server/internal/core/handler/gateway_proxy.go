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
	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/model"
	"ai-gateway/internal/router"
	"ai-gateway/internal/utils"
)

// UnifiedGatewayHandler 统一网关代理（基于 Registry + Unified 中间表示）。
//
// 流程：
//  1. 入口协议 ToUnified(body) → *unified.Request
//  2. 路由选择上游 Provider + ProviderModel
//  3. 上游协议 FromUnified(req) → unified.Response 或 <-chan unified.StreamEvent
//  4. 入口协议 FormatUnified(resp/events) → 客户端格式
//
// 中间件本身永远不随协议数量变化；新增协议只需在 protocols/ 下新建文件夹。
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

	// 1. 获取入口协议描述符
	entryDesc, ok := registry.Get(protocolName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unsupported protocol: " + protocolName})
		return
	}

	// 2. 提取并验证 API Key
	rawKey := entryDesc.AuthExtractor(c)
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

	// 3. 读取请求体（ToUnified 需要）
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	// 4. 提取模型名
	modelName := h.extractModel(c, body)
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model identifier"})
		return
	}

	// 5. 路由（支持 mapping/direct/hybrid 三种 AccessMode）
	result, isDirectCall, routeErr := h.route(apiKey, modelName)
	if routeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": routeErr.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model mapping not found or no available provider"})
		return
	}

	// 5.1 重复检查（双保险）：该 key 的直通白名单与映射白名单不应有相同 model_id
	if conflict := h.checkKeyConflict(apiKey.ID); conflict != "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "API key has model ID conflict: " + conflict})
		return
	}

	// 6. 权限检查（mapping 模式下校验 key_models）
	if !isDirectCall {
		if err := h.verifyKeyID(apiKey.ID, modelName); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
	}

	// 7. 入口协议 ToUnified
	entryProv := entryDesc.NewProvider(&registry.Config{})
	unifiedReq, err := entryProv.ToUnified(body, result.ProviderModel.ModelID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parse request: " + err.Error()})
		return
	}
	unifiedReq.SourceProtocol = protocolName

	// Gemini Stream 检测（Gemini 通过 URL 区分流式，body 中无 Stream 字段）
	if protocolName == "gemini" && strings.Contains(c.Request.URL.Path, "streamGenerateContent") {
		unifiedReq.Stream = true
	}

	// 8. 选择上游协议并执行
	c.Set("key_id", apiKey.ID)
	c.Set("key_name", apiKey.Name)

	originalPath := c.Request.URL.Path
	prefix := "/gateway/" + protocolName
	if strings.HasPrefix(originalPath, prefix) {
		c.Request.URL.Path = strings.TrimPrefix(originalPath, prefix)
	}

	start := time.Now()
	usage := registry.Usage{}
	upstreamProtocol, execErr := h.execute(c, unifiedReq, result, &usage)

	c.Request.URL.Path = originalPath
	latencyMs := time.Since(start).Milliseconds()

	// 9. 冷却管理
	status := "success"
	errMsg := ""
	if execErr != nil {
		status = "error"
		errMsg = execErr.Error()
		if httpErr, ok := execErr.(*registry.HTTPError); ok && httpErr.IsRateLimit() {
			h.modelRouter.RecordRateLimit(result.Provider.ID, result.ProviderModel.ID)
		}
		coreErrors.Error("gateway error: %s | %s %s | model=%s→%s | %dms",
			errMsg, c.Request.Method, originalPath,
			modelName, upstreamProtocol, latencyMs)
	} else {
		h.modelRouter.RecordSuccess(result.Provider.ID, result.ProviderModel.ID)
	}

	// 10. 日志
	matched := upstreamProtocol == protocolName
	clientIPs := utils.GetClientIPInfo(c)
	modelLog := h.newModelLog(
		protocolName, clientIPs, apiKey.ID, apiKey.Name, modelName,
		result, matched, &usage, int(latencyMs), status, errMsg,
	)
	model.DB.Create(modelLog)

	if status == "success" {
		coreErrors.Info("%s %s → %s | %s | %dt %dms",
			protocolName, modelName, upstreamProtocol,
			classifyCall(matched, isDirectCall), usage.TotalTokens(), latencyMs)
	}
}

func classifyCall(matched, isDirect bool) string {
	if isDirect {
		return "direct"
	}
	if !matched {
		return "convert"
	}
	return "direct"
}

// route 根据 AccessMode 选择路由方式
func (h *UnifiedGatewayHandler) route(apiKey model.Key, modelName string) (*router.RouteResult, bool, error) {
	// direct / hybrid 模式先尝试直通
	if apiKey.AccessMode == "direct" || apiKey.AccessMode == "hybrid" {
		directResult, err := h.modelRouter.RouteDirect(modelName, apiKey.ID)
		if err == nil && directResult != nil {
			return directResult, true, nil
		}
	}

	if apiKey.AccessMode == "direct" {
		return nil, false, fmt.Errorf("direct model not found: %s", modelName)
	}

	// mapping 模式
	mappedResult, err := h.modelRouter.Route(modelName)
	if err != nil {
		return nil, false, err
	}
	return mappedResult, false, nil
}

// checkKeyConflict 检查该 key 的直通白名单与映射白名单是否有 model_id 重复。
// 返回非空字符串表示有冲突（含冲突的 model_id 列表）。
func (h *UnifiedGatewayHandler) checkKeyConflict(keyID uint) string {
	// 直通白名单中的 model_id 集合
	var directPMIDs []uint
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ?", keyID).Pluck("provider_model_id", &directPMIDs)
	if len(directPMIDs) == 0 {
		return ""
	}
	var directModelIDs []string
	model.DB.Model(&model.ProviderModel{}).Where("id IN ?", directPMIDs).Pluck("model_id", &directModelIDs)
	if len(directModelIDs) == 0 {
		return ""
	}

	// 映射白名单中的虚拟模型名集合
	var mappingModelIDs []uint
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Pluck("model_id", &mappingModelIDs)
	if len(mappingModelIDs) == 0 {
		return ""
	}
	var mappingNames []string
	model.DB.Model(&model.Model{}).Where("id IN ?", mappingModelIDs).Pluck("name", &mappingNames)
	if len(mappingNames) == 0 {
		return ""
	}

	// 求交集
	directSet := make(map[string]bool)
	for _, id := range directModelIDs {
		directSet[id] = true
	}
	var conflicts []string
	for _, name := range mappingNames {
		if directSet[name] {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 {
		return "以下模型同时出现在「模型厂商」直通和「模型映射」中，请从一侧移除：" + strings.Join(conflicts, "、")
	}
	return ""
}

// execute 执行 Unified 请求，返回使用的上游协议名。
//
// 优先使用 Outbound 接口（AxonHub 风格），回退到 Provider.FromUnified。
func (h *UnifiedGatewayHandler) execute(c *gin.Context, req *unified.Request,
	result *router.RouteResult, usage *registry.Usage) (string, error) {

	// 选择上游协议：优先同协议直通，否则任选一个支持的协议
	providerProtos := result.GetProviderProtocols()
	if len(providerProtos) == 0 {
		return "", fmt.Errorf("no protocol configured for provider")
	}

	upstreamProto := ""
	for _, p := range providerProtos {
		if p == req.SourceProtocol {
			upstreamProto = p
			break
		}
	}
	if upstreamProto == "" {
		upstreamProto = providerProtos[0]
	}

	upDesc, ok := registry.Get(upstreamProto)
	if !ok {
		return upstreamProto, fmt.Errorf("unsupported upstream protocol: %s", upstreamProto)
	}

	baseURL := h.getBaseURL(result.Provider, upstreamProto)
	if baseURL == "" {
		return upstreamProto, fmt.Errorf("no base URL for protocol %s", upstreamProto)
	}

	cfg := &registry.Config{BaseURL: baseURL, APIKey: result.Provider.APIKey}
	upProv := upDesc.NewProvider(cfg)

	// 上游执行：优先使用 Outbound 接口
	var resp *unified.Response
	var events <-chan unified.StreamEvent
	var err error

	if outbound, ok := upProv.(registry.Outbound); ok {
		resp, events, err = outbound.BuildRequest(req)
	} else {
		resp, events, err = upProv.FromUnified(req)
	}

	if err != nil {
		if httpErr, ok := err.(*registry.HTTPError); ok {
			c.Status(httpErr.StatusCode)
			c.Header("Content-Type", "application/json")
			c.Writer.Write(httpErr.Body)
		}
		return upstreamProto, err
	}

	// 用入口协议把 Unified 响应/流格式化为客户端格式
	entryDesc, _ := registry.Get(req.SourceProtocol)
	entryProv := entryDesc.NewProvider(&registry.Config{})

	// 优先使用 Inbound 接口
	if inbound, ok := entryProv.(registry.Inbound); ok {
		return upstreamProto, inbound.FormatResponse(resp, events, c, usage)
	}
	return upstreamProto, entryProv.FormatUnified(resp, events, c, usage)
}

// extractModel 从请求中提取模型名
func (h *UnifiedGatewayHandler) extractModel(c *gin.Context, body []byte) string {
	// 优先从 body 的 model 字段提取
	var req map[string]interface{}
	if json.Unmarshal(body, &req) == nil {
		if m, ok := req["model"].(string); ok && m != "" {
			return m
		}
	}
	// Gemini 风格 URL 提取: /models/{id}:generateContent
	path := c.Request.URL.Path
	if parts := strings.Split(path, "/models/"); len(parts) >= 2 {
		return strings.Split(parts[1], ":")[0]
	}
	return ""
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
	case "deepseek":
		return strings.TrimSuffix(p.DeepSeekBaseURL, "/")
	}
	return ""
}

// verifyKeyID 校验 key 是否有权访问指定虚拟模型（key_models 为空表示全部允许）
func (h *UnifiedGatewayHandler) verifyKeyID(keyID uint, modelName string) error {
	var count int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", keyID).Count(&count)
	if count == 0 {
		return nil
	}
	var m model.Model
	if err := model.DB.Where("name = ?", modelName).First(&m).Error; err != nil {
		return fmt.Errorf("model not allowed for this API key")
	}
	var modelCount int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ? AND model_id = ?", keyID, m.ID).Count(&modelCount)
	if modelCount == 0 {
		return fmt.Errorf("model not allowed for this API key")
	}
	return nil
}

// newModelLog 构造模型调用日志
func (h *UnifiedGatewayHandler) newModelLog(source, clientIPs string, keyID uint, keyName, modelName string,
	result *router.RouteResult, matched bool, usage *registry.Usage,
	latencyMs int, status, errMsg string) *model.ModelLog {

	actualModelName := result.ProviderModel.DisplayName
	if actualModelName == "" {
		actualModelName = result.ProviderModel.ModelID
	}
	callMethod := "direct"
	if !matched {
		callMethod = "convert"
	}
	return &model.ModelLog{
		Source:          source,
		ClientIPs:       clientIPs,
		KeyID:           keyID,
		KeyName:         keyName,
		Model:           modelName,
		ProviderID:      result.Provider.ID,
		ProviderName:    result.Provider.Name,
		ActualModelID:   result.ProviderModel.ModelID,
		ActualModelName: actualModelName,
		CallMethod:      callMethod,
		CachedTokens:    usage.CachedTokens,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens(),
		LatencyMs:       latencyMs,
		Status:          status,
		ErrorMsg:        errMsg,
	}
}
