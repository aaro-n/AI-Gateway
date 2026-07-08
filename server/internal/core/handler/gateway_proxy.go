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

	"ai-gateway/internal/core/conversion"
	coreErrors "ai-gateway/internal/core/errors"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	"ai-gateway/internal/monitor"
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
	traceID := middleware.GetTraceID(c)

	coreErrors.TraceDebugKVs(traceID, "gateway_enter",
		"protocol", protocolName,
		"method", c.Request.Method,
		"path", c.Request.URL.Path)

	// 1. 获取入口协议描述符
	entryDesc, ok := registry.Get(protocolName)
	if !ok {
		coreErrors.TraceWarn(traceID, "unsupported protocol: %s", protocolName)
		c.JSON(http.StatusNotFound, gin.H{"error": "unsupported protocol: " + protocolName})
		return
	}

	// 2. 提取并验证 API Key
	rawKey := entryDesc.AuthExtractor(c)
	if rawKey == "" {
		coreErrors.TraceWarn(traceID, "missing API key")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		return
	}

	var kf model.KeyFormat
	if err := model.DB.Where("formatted_key = ?", rawKey).First(&kf).Error; err != nil {
		coreErrors.TraceWarn(traceID, "invalid API key")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}
	if kf.Format != protocolName {
		coreErrors.TraceWarn(traceID, "key_format_mismatch key_format=%s entry_protocol=%s key_id=%d", kf.Format, protocolName, kf.KeyID)
		c.JSON(http.StatusForbidden, gin.H{"error": "key format not allowed on this endpoint"})
		return
	}

	var apiKey model.Key
	if err := model.DB.Where("id = ? AND enabled = ?", kf.KeyID, true).First(&apiKey).Error; err != nil {
		coreErrors.TraceWarn(traceID, "invalid API key (disabled or deleted) key_id=%d", kf.KeyID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		coreErrors.TraceWarn(traceID, "API key expired key_id=%d expires_at=%s", apiKey.ID, apiKey.ExpiresAt.String())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "API key expired"})
		return
	}

	coreErrors.TraceDebugKVs(traceID, "key_authenticated",
		"key_id", fmt.Sprintf("%d", apiKey.ID),
		"key_name", apiKey.Name,
		"access_mode", apiKey.AccessMode)

	// 3. 读取请求体（ToUnified 需要）
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		coreErrors.TraceError(traceID, "read_body_failed err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	coreErrors.TraceDebug(traceID, "request_body_size=%d", len(body))

	// 4. 提取模型名
	modelName := h.extractModel(c, body)
	if modelName == "" {
		coreErrors.TraceWarn(traceID, "missing model identifier in request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model identifier"})
		return
	}

	coreErrors.TraceDebugKVs(traceID, "model_extracted",
		"model", modelName)

	// 5. 路由（支持 mapping/direct/hybrid 三种 AccessMode）
	result, isDirectCall, routeErr := h.route(apiKey, modelName)
	if routeErr != nil {
		coreErrors.TraceError(traceID, "route_failed model=%s err=%v", modelName, routeErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": routeErr.Error()})
		return
	}
	if result == nil {
		coreErrors.TraceWarn(traceID, "no_model_mapping_or_provider model=%s access_mode=%s", modelName, apiKey.AccessMode)
		c.JSON(http.StatusNotFound, gin.H{"error": "model mapping not found or no available provider"})
		return
	}

	coreErrors.TraceDebugKVs(traceID, "route_success",
		"provider", result.Provider.Name,
		"provider_model", result.ProviderModel.ModelID,
		"is_direct", fmt.Sprintf("%v", isDirectCall),
		"base_url", result.Provider.OpenAIBaseURL)

	// 5.1 获取所有候选项（用于 failover）
	allResults, isDirectCall, err := h.routeAll(apiKey, modelName)
	if err != nil || len(allResults) == 0 {
		// 回退到单路由
		allResults = []*router.RouteResult{result}
	}

	// 5.2 重复检查（双保险）
	if conflict := h.checkKeyConflict(apiKey.ID); conflict != "" {
		coreErrors.TraceError(traceID, "key_model_conflict key_id=%d conflict=%s", apiKey.ID, conflict)
		c.JSON(http.StatusForbidden, gin.H{"error": "API key has model ID conflict: " + conflict})
		return
	}

	// 6. 权限检查（mapping 模式下校验 key_models）
	if !isDirectCall {
		if err := h.verifyKeyID(apiKey.ID, modelName); err != nil {
			coreErrors.TraceWarn(traceID, "permission_denied model=%s key_id=%d err=%v", modelName, apiKey.ID, err)
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
	}

	// 7. 入口协议 ToUnified
	entryProv := entryDesc.NewProvider(&registry.Config{})
	unifiedReq, err := entryProv.ToUnified(body, result.ProviderModel.ModelID)
	if err != nil {
		coreErrors.TraceError(traceID, "to_unified_failed from=%s err=%v", protocolName, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "parse request: " + err.Error()})
		return
	}
	unifiedReq.SourceProtocol = protocolName
	unifiedReq.Ctx = c.Request.Context()

	coreErrors.TraceDebugKVs(traceID, "to_unified_done",
		"stream", fmt.Sprintf("%v", unifiedReq.Stream),
		"has_tools", fmt.Sprintf("%v", len(unifiedReq.Tools) > 0))

	// 协议特定的流式检测（Gemini 通过 URL 区分流式，body 中无 stream 字段）
	if hintProv, ok := entryProv.(registry.StreamHintProvider); ok {
		if hintProv.IsStreamRequest(c) {
			unifiedReq.Stream = true
			coreErrors.TraceDebug(traceID, "stream_detected via StreamHintProvider for protocol=%s", protocolName)
		}
	}

	// 8. 选择上游协议并执行
	c.Set("key_id", apiKey.ID)
	c.Set("key_name", apiKey.Name)

	originalPath := c.Request.URL.Path
	prefix := "/gateway/" + protocolName
	if strings.HasPrefix(originalPath, prefix) {
		c.Request.URL.Path = strings.TrimPrefix(originalPath, prefix)
	}

	// 8. Failover 循环：逐一尝试候选项
	var upstreamProtocol string
	var execErr error
	var usage registry.Usage

	start := time.Now()
	status := "success"
	errMsg := ""
	finalResult := allResults[0]

	for i, cand := range allResults {
		if i > 0 {
			coreErrors.TraceWarn(traceID, "failover_retry attempt=%d provider=%s model=%s",
				i, cand.Provider.Name, cand.ProviderModel.ModelID)
			// 重置 usage（每个候选项独立计数）
			usage = registry.Usage{}
		}

		upstreamProtocol, execErr = h.execute(c, unifiedReq, cand, &usage, isDirectCall)
		if execErr == nil {
			finalResult = cand
			h.modelRouter.RecordSuccess(cand.Provider.ID, cand.ProviderModel.ID)
			break
		}

		// 记录错误到熔断器
		if httpErr, ok := execErr.(*registry.HTTPError); ok && httpErr.IsRateLimit() {
			h.modelRouter.RecordRateLimit(cand.Provider.ID, cand.ProviderModel.ID)
			coreErrors.TraceWarn(traceID, "rate_limited provider=%s model=%s", cand.Provider.Name, cand.ProviderModel.ModelID)
		} else {
			h.modelRouter.RecordError(cand.Provider.ID, cand.ProviderModel.ID)
		}
		coreErrors.TraceError(traceID, "upstream_failed upstream=%s model=%s provider_model=%s err=%s",
			upstreamProtocol, modelName, cand.ProviderModel.ModelID, execErr.Error())
	}

	if execErr != nil {
		status = "error"
		errMsg = execErr.Error()
	}

	c.Request.URL.Path = originalPath
	latencyMs := time.Since(start).Milliseconds()

	// 10. 日志
	matched := upstreamProtocol == protocolName
	clientIPs := utils.GetClientIPInfo(c)
	convStatusStr := "ok"
	if convResult, exists := c.Get("conv_result"); exists {
		if cr, ok := convResult.(*conversion.ConversionResult); ok && cr.Status != 0 {
			convStatusStr = conversion.DecodeSummary(cr.Status)
		}
	}
	modelLog := h.newModelLog(
		protocolName, clientIPs, apiKey.ID, apiKey.Name, modelName,
		finalResult, matched, &usage, int(latencyMs), status, errMsg, convStatusStr,
	)
	model.DB.Create(modelLog)

	// ── 指标记录 (Prometheus / OpenTelemetry) ──
	monitor.GlobalRecorder.RecordRelayRequest(start, modelName, finalResult.ProviderModel.ModelID,
		protocolName, upstreamProtocol, execErr == nil,
		usage.InputTokens, usage.OutputTokens, usage.CachedTokens)

	callType := classifyCall(matched, isDirectCall)
	if status == "success" {
		coreErrors.TraceInfoKVs(traceID, "gateway_success",
			"entry_protocol", protocolName,
			"upstream_protocol", upstreamProtocol,
			"model", modelName,
			"provider_model", finalResult.ProviderModel.ModelID,
			"provider", finalResult.Provider.Name,
			"call_type", callType,
			"tokens", fmt.Sprintf("%d", usage.TotalTokens()),
			"latency_ms", fmt.Sprintf("%d", latencyMs),
			"key_id", fmt.Sprintf("%d", apiKey.ID),
			"key_name", apiKey.Name,
			"conv_status", convStatusStr)
	} else {
		coreErrors.TraceError(traceID, "gateway_failed entry=%s upstream=%s model=%s provider=%s call_type=%s conv_status=%s err=%s latency_ms=%d",
			protocolName, upstreamProtocol, modelName, finalResult.Provider.Name,
			callType, convStatusStr, errMsg, latencyMs)
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

// routeAll 返回所有候选项，用于 failover 循环
func (h *UnifiedGatewayHandler) routeAll(apiKey model.Key, modelName string) ([]*router.RouteResult, bool, error) {
	if apiKey.AccessMode == "direct" {
		results, err := h.modelRouter.RouteAllDirect(modelName, apiKey.ID)
		if err != nil {
			return nil, false, err
		}
		if len(results) > 0 {
			return results, true, nil
		}
		return nil, false, fmt.Errorf("direct model not found: %s", modelName)
	}

	if apiKey.AccessMode == "hybrid" {
		directResults, _ := h.modelRouter.RouteAllDirect(modelName, apiKey.ID)
		mappedResults, _ := h.modelRouter.RouteAll(modelName)
		results := make([]*router.RouteResult, 0, len(directResults)+len(mappedResults))
		for _, r := range directResults {
			results = append(results, r)
		}
		for _, r := range mappedResults {
			results = append(results, r)
		}
		if len(results) > 0 {
			// 确定首选项类型
			_, isDirect, _ := h.route(apiKey, modelName)
			return results, isDirect, nil
		}
		return nil, false, fmt.Errorf("no available provider for model: %s", modelName)
	}

	// mapping mode
	results, err := h.modelRouter.RouteAll(modelName)
	if err != nil {
		return nil, false, err
	}
	return results, false, nil
}

// checkKeyConflict 检查该 key 的直通白名单与映射白名单是否有 model_id 重复。
// 返回非空字符串表示有冲突（含冲突的 model_id 列表）。
func (h *UnifiedGatewayHandler) checkKeyConflict(keyID uint) string {
	// 直通白名单中的 model_id 集合（仅 enabled）
	var directPMIDs []uint
	model.DB.Model(&model.KeyProviderModel{}).Where("key_id = ? AND enabled = ?", keyID, true).Pluck("provider_model_id", &directPMIDs)
	if len(directPMIDs) == 0 {
		return ""
	}
	var directModelIDs []string
	model.DB.Model(&model.ProviderModel{}).Where("id IN ?", directPMIDs).Pluck("model_id", &directModelIDs)
	if len(directModelIDs) == 0 {
		return ""
	}

	// 映射白名单中的虚拟模型名集合（仅 enabled）
	var mappingModelIDs []uint
	model.DB.Model(&model.KeyModel{}).Where("key_id = ? AND enabled = ?", keyID, true).Pluck("model_id", &mappingModelIDs)
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
	result *router.RouteResult, usage *registry.Usage, isDirectCall bool) (string, error) {

	traceID := middleware.GetTraceID(c)

	// 选择上游协议：优先同协议直通
	providerProtos := result.Provider.SupportedProtocols()
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

	// 直通模式不允许跨协议转换
	if isDirectCall && upstreamProto == "" {
		return "", fmt.Errorf(
			"direct API key does not support cross-protocol conversion: "+
				"client protocol=%s, provider only supports %v",
			req.SourceProtocol, providerProtos)
	}

	if upstreamProto == "" {
		upstreamProto = providerProtos[0]
	}

	coreErrors.TraceDebugKVs(traceID, "upstream_selected",
		"upstream_protocol", upstreamProto,
		"source_protocol", req.SourceProtocol,
		"is_conversion", fmt.Sprintf("%v", upstreamProto != req.SourceProtocol),
		"provider_protocols", strings.Join(providerProtos, ","))

	// 跨协议转换损失检测（声明式能力差集）
	if upstreamProto != req.SourceProtocol {
		entryDesc, entryOK := registry.Get(req.SourceProtocol)
		upDesc, upOK := registry.Get(upstreamProto)
		if entryOK && upOK &&
			entryDesc.Capabilities != nil && upDesc.Capabilities != nil {
			convResult := conversion.Compare(entryDesc.Capabilities, upDesc.Capabilities)
			c.Set("conv_result", convResult)
			convStatus := conversion.BuildStatusCode(
				conversion.ProtocolID(req.SourceProtocol),
				conversion.ProtocolID(upstreamProto),
				convResult.Status,
				false,
			)
			c.Set("conv_status", convStatus)
			if len(convResult.Warnings) > 0 {
				c.Header("X-Gateway-Warnings", strings.Join(convResult.Warnings, "; "))
			}
			coreErrors.TraceDebugKVs(traceID, "conversion_detected",
				"conv_status", conversion.DecodeSummary(convStatus),
				"lost_fields", strings.Join(convResult.LostFields, ","))
		}
	} else {
		// 同协议直通，记录 conv_status 方便日志统计
		c.Set("conv_status", conversion.BuildStatusCode(
			conversion.ProtocolID(req.SourceProtocol),
			conversion.ProtocolID(upstreamProto),
			0,
			true,
		))
	}

	upDesc, ok := registry.Get(upstreamProto)
	if !ok {
		return upstreamProto, fmt.Errorf("unsupported upstream protocol: %s", upstreamProto)
	}

	baseURL := h.getBaseURL(result.Provider, upstreamProto)
	if baseURL == "" {
		return upstreamProto, fmt.Errorf("no base URL for protocol %s", upstreamProto)
	}

	coreErrors.TraceDebug(traceID, "upstream_base_url=%s", baseURL)

	cfg := &registry.Config{BaseURL: baseURL, APIKey: result.Provider.APIKey}
	upProv := upDesc.NewProvider(cfg)

	// 上游执行：优先使用 Outbound 接口
	var resp *unified.Response
	var events <-chan unified.StreamEvent
	var err error

	if outbound, ok := upProv.(registry.Outbound); ok {
		coreErrors.TraceDebug(traceID, "using_outbound_interface")
		resp, events, err = outbound.BuildRequest(req)
	} else {
		coreErrors.TraceDebug(traceID, "using_from_unified")
		resp, events, err = upProv.FromUnified(req)
	}

	if err != nil {
		coreErrors.TraceError(traceID, "upstream_build_request_failed err=%v", err)
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
		coreErrors.TraceDebug(traceID, "using_inbound_interface for response formatting")
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
	url := p.EndpointFor(protocol)
	if url != "" {
		return strings.TrimSuffix(url, "/")
	}
	return ""
}

// verifyKeyID 校验 key 是否有权访问指定虚拟模型（key_models 为空表示全部允许）。
// 只检查 enabled=true 的映射关联。
func (h *UnifiedGatewayHandler) verifyKeyID(keyID uint, modelName string) error {
	var count int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ? AND enabled = ?", keyID, true).Count(&count)
	if count == 0 {
		return nil
	}
	var m model.Model
	if err := model.DB.Where("name = ?", modelName).First(&m).Error; err != nil {
		return fmt.Errorf("model not allowed for this API key")
	}
	var modelCount int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ? AND model_id = ? AND enabled = ?", keyID, m.ID, true).Count(&modelCount)
	if modelCount == 0 {
		return fmt.Errorf("model not allowed for this API key")
	}
	return nil
}

// newModelLog 构造模型调用日志
func (h *UnifiedGatewayHandler) newModelLog(source, clientIPs string, keyID uint, keyName, modelName string,
	result *router.RouteResult, matched bool, usage *registry.Usage,
	latencyMs int, status, errMsg, convStatus string) *model.ModelLog {

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
		ConvStatus:      convStatus,
		ErrorMsg:        errMsg,
	}
}
