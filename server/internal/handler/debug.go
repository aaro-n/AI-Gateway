package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/conversion"
	coreErrors "ai-gateway/internal/core/errors"
	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
	"ai-gateway/internal/protocols/capabilities"
)

// DebugHandler 调试工具 API
type DebugHandler struct{}

func NewDebugHandler() *DebugHandler {
	return &DebugHandler{}
}

// =============================================================================
// 请求/响应类型
// =============================================================================

type testProvidersRequest struct {
	ProviderID *uint  `json:"provider_id"` // 可选：指定单个 Provider，为空则测试所有已启用
	Model      string `json:"model"`       // 可选：指定要测试的模型 ID，为空则自动选取第一个可用模型
}

type testKeyRequest struct {
	KeyID uint   `json:"key_id" binding:"required"`
	Model string `json:"model"` // 可选：指定模型，为空则自动选择
}

type debugLogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"` // "info", "success", "error", "warn"
	Message   string `json:"message"`
	Detail    string `json:"detail"` // 额外详情（JSON 字符串）；不使用 omitempty，空串也需透传
}

type providerTestResult struct {
	ProviderID      uint            `json:"provider_id"`
	ProviderName    string          `json:"provider_name"`
	Protocol        string          `json:"protocol"`
	TestModel       string          `json:"test_model"`       // 实际使用的测试模型 ID
	AvailableModels []string        `json:"available_models"` // 该 Provider 可用的模型列表
	Success         bool            `json:"success"`
	LatencyMs       int64           `json:"latency_ms"`
	InputTokens     int             `json:"input_tokens"`
	OutputTokens    int             `json:"output_tokens"`
	Response        string          `json:"response"`
	Error           string          `json:"error"`
	Logs            []debugLogEntry `json:"logs"`
}

type keyTestResult struct {
	KeyID         uint            `json:"key_id"`
	KeyName       string          `json:"key_name"`
	Protocol      string          `json:"protocol"`
	Model         string          `json:"model"`
	Success       bool            `json:"success"`
	HTTPStatus    int             `json:"http_status"`
	LatencyMs     int64           `json:"latency_ms"`
	ResponseBody  string          `json:"response_body"`
	Error         string          `json:"error"`
	ConvStatus    string          `json:"conv_status"`       // 转换损失摘要
	ConvDetail    json.RawMessage `json:"conv_detail"`       // 转换损失详细信息
	IsDirect      bool            `json:"is_direct"`         // 是否同协议直通
	LostFeatures  []string        `json:"lost_features"`     // 丢失的功能列表
	EntryProtocol string          `json:"entry_protocol"`    // 客户端入口协议
	UpstreamProto string          `json:"upstream_protocol"` // 上游实际协议
	Logs          []debugLogEntry `json:"logs"`
}

type recentLogEntry struct {
	ID              uint            `json:"id"`
	KeyName         string          `json:"key_name"`
	Model           string          `json:"model"`
	ProviderName    string          `json:"provider_name"`
	ActualModelName string          `json:"actual_model_name"`
	CallMethod      string          `json:"call_method"`
	InputTokens     int             `json:"input_tokens"`
	OutputTokens    int             `json:"output_tokens"`
	TotalTokens     int             `json:"total_tokens"`
	LatencyMs       int             `json:"latency_ms"`
	Status          string          `json:"status"`
	ErrorMsg        string          `json:"error_msg"`
	Source          string          `json:"source"`
	ConvStatus      string          `json:"conv_status"`
	ConvDetail      json.RawMessage `json:"conv_detail,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

// =============================================================================
// POST /api/v1/debug/test-providers
// =============================================================================

func (h *DebugHandler) TestProviders(c *gin.Context) {
	var req testProvidersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body（测试所有 provider）
	}

	var providers []model.Provider
	if req.ProviderID != nil {
		var p model.Provider
		if err := model.DB.Preload("Models").First(&p, *req.ProviderID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
			return
		}
		providers = []model.Provider{p}
	} else {
		model.DB.Where("enabled = ?", true).Preload("Models").Order("name ASC").Find(&providers)
	}

	results := make([]providerTestResult, 0)
	now := func() string { return time.Now().Format("15:04:05.000") }

	for _, p := range providers {
		protocols := p.SupportedProtocols()
		if len(protocols) == 0 {
			continue
		}

		// 收集该 Provider 所有可用的 Model ID
		availableModelIDs := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			if m.ModelID != "" {
				availableModelIDs = append(availableModelIDs, m.ModelID)
			}
		}

		for _, proto := range protocols {
			logs := make([]debugLogEntry, 0)
			logs = append(logs, debugLogEntry{
				Timestamp: now(),
				Level:     "info",
				Message:   fmt.Sprintf("开始测试厂商 [%s] 协议 [%s]", p.Name, proto),
			})

			baseURL := p.EndpointFor(proto)
			if baseURL == "" {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "warn",
					Message:   fmt.Sprintf("协议 [%s] 未配置端点，跳过", proto),
				})
				results = append(results, providerTestResult{
					ProviderID:      p.ID,
					ProviderName:    p.Name,
					Protocol:        proto,
					Success:         false,
					Error:           "no endpoint configured",
					AvailableModels: availableModelIDs,
					Logs:            logs,
				})
				continue
			}

			// 选取测试模型：优先用请求中指定的，否则取第一个可用模型
			testModel := req.Model
			if testModel == "" && len(availableModelIDs) > 0 {
				testModel = availableModelIDs[0]
			}
			if testModel == "" {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "warn",
					Message:   "没有可用的模型（请先在「模型厂商」页面同步模型列表）",
				})
				results = append(results, providerTestResult{
					ProviderID:      p.ID,
					ProviderName:    p.Name,
					Protocol:        proto,
					Success:         false,
					Error:           "no models available - please sync models first",
					AvailableModels: availableModelIDs,
					Logs:            logs,
				})
				continue
			}

			logs = append(logs, debugLogEntry{
				Timestamp: now(),
				Level:     "info",
				Message:   fmt.Sprintf("端点: %s", baseURL),
			})
			logs = append(logs, debugLogEntry{
				Timestamp: now(),
				Level:     "info",
				Message:   fmt.Sprintf("测试模型: %s", testModel),
			})

			// 构建测试请求体
			reqBody := buildDebugRequestBody(proto, testModel)
			// 生成 curl 风格请求日志
			curlCmd := buildCurlCommand(proto, baseURL, p.APIKey, testModel, reqBody)
			logs = append(logs, debugLogEntry{
				Timestamp: now(),
				Level:     "info",
				Message:   "发送测试请求...",
				Detail:    curlCmd,
			})

			// 执行测试（调试页用 1024 tokens 以获得完整响应）
			tr := protocolsPkg.RunTest(proto, baseURL, p.APIKey, testModel, 1024)

			if tr.Success {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "success",
					Message:   fmt.Sprintf("✓ 测试成功 | 耗时: %dms | 输入Token: %d | 输出Token: %d", tr.LatencyMs, tr.InputTokens, tr.OutputTokens),
				})
				// 直接展示上游原始 JSON 响应
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "info",
					Message:   "响应内容",
					Detail:    safePrefix(tr.RawResponse, 3000),
				})
			} else {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "error",
					Message:   fmt.Sprintf("✗ 测试失败 | 耗时: %dms", tr.LatencyMs),
				})
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "error",
					Message:   "错误详情",
					Detail:    tr.Error,
				})
			}

			results = append(results, providerTestResult{
				ProviderID:      p.ID,
				ProviderName:    p.Name,
				Protocol:        proto,
				TestModel:       testModel,
				Success:         tr.Success,
				LatencyMs:       tr.LatencyMs,
				InputTokens:     tr.InputTokens,
				OutputTokens:    tr.OutputTokens,
				Response:        tr.Response,
				Error:           tr.Error,
				AvailableModels: availableModelIDs,
				Logs:            logs,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// =============================================================================
// POST /api/v1/debug/test-key
// =============================================================================

func (h *DebugHandler) TestKey(c *gin.Context) {
	var req testKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := func() string { return time.Now().Format("15:04:05.000") }
	logs := make([]debugLogEntry, 0)

	// Step 1: 查询 Key 信息
	var k model.Key
	if err := model.DB.Preload("Formats").First(&k, req.KeyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	if !k.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is disabled"})
		return
	}

	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   fmt.Sprintf("密钥: %s (ID=%d, AccessMode=%s)", k.Name, k.ID, k.AccessMode),
	})

	// Step 2: 获取 Key 的协议
	if len(k.Formats) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key has no formats configured"})
		return
	}

	primaryFormat := k.Formats[0]
	protocol := primaryFormat.Format
	formattedKey := primaryFormat.FormattedKey

	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   fmt.Sprintf("协议: %s | 格式化Key前缀: %s...", protocol, safePrefix(formattedKey, 12)),
	})

	// Step 3: 确定测试模型
	modelName := req.Model
	if modelName == "" {
		modelName = "gpt-4o" // 默认
	}
	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   fmt.Sprintf("测试模型: %s", modelName),
	})

	// Step 4: 构建 HTTP 请求
	host := c.Request.Host
	gatewayPath := buildGatewayPath(protocol, modelName)
	gatewayURL := fmt.Sprintf("http://%s%s", host, gatewayPath)
	reqBody := buildDebugRequestBody(protocol, modelName)

	httpReq, err := http.NewRequest("POST", gatewayURL, bytes.NewReader(reqBody))
	if err != nil {
		logs = append(logs, debugLogEntry{
			Timestamp: now(),
			Level:     "error",
			Message:   fmt.Sprintf("构造请求失败: %v", err),
		})
		c.JSON(http.StatusOK, gin.H{
			"key_id":   k.ID,
			"key_name": k.Name,
			"protocol": protocol,
			"model":    modelName,
			"success":  false,
			"error":    err.Error(),
			"logs":     logs,
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+formattedKey)
	httpReq.Header.Set("User-Agent", "ai-gateway-debug/1.0")

	// ── Dump 请求（curl -v 风格）──
	reqDump, _ := httputil.DumpRequestOut(httpReq, true)
	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   "────────── 请求 Dump（curl 风格）──────────",
		Detail:    string(reqDump),
	})

	// Step 5: 发送 HTTP 请求
	start := time.Now()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		logs = append(logs, debugLogEntry{
			Timestamp: now(),
			Level:     "error",
			Message:   fmt.Sprintf("HTTP 请求失败 (耗时 %dms): %v", latencyMs, err),
		})
		c.JSON(http.StatusOK, gin.H{
			"key_id":     k.ID,
			"key_name":   k.Name,
			"protocol":   protocol,
			"model":      modelName,
			"success":    false,
			"latency_ms": latencyMs,
			"error":      err.Error(),
			"logs":       logs,
		})
		return
	}
	defer resp.Body.Close()

	// ── Dump 响应（curl -v 风格）──
	respDump, _ := httputil.DumpResponse(resp, true)
	// DumpResponse 已消费 body，需要重建 resp.Body 以供后续读取
	resp.Body = io.NopCloser(bytes.NewReader(respDump))
	// 重新读取 body 从 dump 中（跳过 headers 部分）
	// httputil.DumpResponse 输出格式: "HTTP/1.1 200 OK\r\nHeader: value\r\n\r\nbody..."
	// 我们需要单独获取 body 内容
	respBody, _ := io.ReadAll(resp.Body)
	respBodyStr := string(respBody)

	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   "────────── 响应 Dump（curl 风格）──────────",
		Detail:    string(respDump),
	})

	// 从 dump 中提取纯 body 部分
	bodyOnly := extractBodyFromDump(respDump)
	if bodyOnly == "" {
		bodyOnly = respBodyStr
	}
	if len(bodyOnly) > 2000 {
		bodyOnly = bodyOnly[:2000] + "...(truncated)"
	}

	logs = append(logs, debugLogEntry{
		Timestamp: now(),
		Level:     "info",
		Message:   fmt.Sprintf("HTTP 响应状态: %d | 耗时: %dms", resp.StatusCode, latencyMs),
	})

	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	// Step 6: 查询转换损失信息
	var convDetail json.RawMessage
	var convStatusStr string
	var lostFeatures []string
	var isDirect bool
	var entryProtocol, upstreamProtocol string

	if success {
		// 从最近的 model_log 获取 conv_status
		var mlog model.ModelLog
		if err := model.DB.Where("key_id = ?", k.ID).
			Order("created_at DESC").
			First(&mlog).Error; err == nil {
			convStatusStr = mlog.ConvStatus
			if convStatusStr != "" && convStatusStr != "ok" {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "warn",
					Message:   fmt.Sprintf("检测到协议转换损失: %s", convStatusStr),
				})
			} else if convStatusStr == "ok" || convStatusStr == "" {
				logs = append(logs, debugLogEntry{
					Timestamp: now(),
					Level:     "success",
					Message:   "协议直通，无功能损失 ✓",
				})
			}
		}

		// 解码 conv_status（如果是数值型编码）
		// 尝试将 CallMethod 和 Provider 信息组合判断
		if mlog.CallMethod == "direct" && mlog.Source == protocol {
			isDirect = true
			entryProtocol = protocol
			upstreamProtocol = protocol
		} else if mlog.CallMethod == "convert" {
			isDirect = false
			entryProtocol = mlog.Source
			// 尝试从 provider 推断上游协议
			upstreamProtocol = inferUpstreamProtocol(&mlog)
			if convStatusStr == "" || convStatusStr == "ok" {
				// 从 capabilities 计算损失
				lossInfo := computeConversionLoss(entryProtocol, upstreamProtocol)
				if lossInfo != nil {
					detailBytes, _ := json.Marshal(lossInfo)
					convDetail = detailBytes
					lostFeatures = lossInfo.LostFields
					if len(lostFeatures) > 0 {
						convStatusStr = fmt.Sprintf("%d项丢失", len(lostFeatures))
					}
				}
			}
		} else {
			isDirect = true
			entryProtocol = protocol
			upstreamProtocol = protocol
		}
	} else {
		// 失败时也设置默认值
		isDirect = false
		entryProtocol = protocol
		upstreamProtocol = protocol
	}

	// Step 7: 最终状态日志
	if success {
		logs = append(logs, debugLogEntry{
			Timestamp: now(),
			Level:     "success",
			Message:   fmt.Sprintf("✓ 测试通过 | HTTP %d | %dms", resp.StatusCode, latencyMs),
		})
	} else {
		logs = append(logs, debugLogEntry{
			Timestamp: now(),
			Level:     "error",
			Message:   fmt.Sprintf("✗ 测试失败 | HTTP %d | %dms", resp.StatusCode, latencyMs),
		})
	}

	result := keyTestResult{
		KeyID:         k.ID,
		KeyName:       k.Name,
		Protocol:      protocol,
		Model:         modelName,
		Success:       success,
		HTTPStatus:    resp.StatusCode,
		LatencyMs:     latencyMs,
		ResponseBody:  bodyOnly,
		ConvStatus:    convStatusStr,
		ConvDetail:    convDetail,
		IsDirect:      isDirect,
		LostFeatures:  lostFeatures,
		EntryProtocol: entryProtocol,
		UpstreamProto: upstreamProtocol,
		Logs:          logs,
	}

	c.JSON(http.StatusOK, result)
}

// =============================================================================
// GET /api/v1/debug/recent-logs
// =============================================================================

func (h *DebugHandler) RecentLogs(c *gin.Context) {
	var logs []model.ModelLog
	if err := model.DB.Order("created_at DESC").Limit(50).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	entries := make([]recentLogEntry, 0, len(logs))
	for _, log := range logs {
		entry := recentLogEntry{
			ID:              log.ID,
			KeyName:         log.KeyName,
			Model:           log.Model,
			ProviderName:    log.ProviderName,
			ActualModelName: log.ActualModelName,
			CallMethod:      log.CallMethod,
			InputTokens:     log.InputTokens,
			OutputTokens:    log.OutputTokens,
			TotalTokens:     log.TotalTokens,
			LatencyMs:       log.LatencyMs,
			Status:          log.Status,
			ErrorMsg:        log.ErrorMsg,
			Source:          log.Source,
			ConvStatus:      log.ConvStatus,
			CreatedAt:       log.CreatedAt.Format("2006-01-02 15:04:05"),
		}

		// 如果有 conv_status，解析出转换详情
		if log.ConvStatus != "" && log.ConvStatus != "ok" {
			entryProtocol := log.Source
			upstreamProtocol := inferUpstreamProtocol(&log)
			lossInfo := computeConversionLoss(entryProtocol, upstreamProtocol)
			if lossInfo != nil {
				detailBytes, _ := json.Marshal(lossInfo)
				entry.ConvDetail = detailBytes
			}
		}

		entries = append(entries, entry)
	}

	c.JSON(http.StatusOK, gin.H{"logs": entries})
}

// =============================================================================
// GET /api/v1/debug/server-logs  ——  服务端运行时日志（内存环形缓冲区）
// =============================================================================

func (h *DebugHandler) ServerLogs(c *gin.Context) {
	since := c.Query("since")
	var entries []coreErrors.LogEntry
	if since != "" {
		entries = coreErrors.GetRingBufferEntriesSince(since)
	} else {
		entries = coreErrors.GetRingBufferEntries(50)
	}
	if entries == nil {
		entries = make([]coreErrors.LogEntry, 0)
	}
	c.JSON(http.StatusOK, gin.H{"logs": entries})
}

// =============================================================================
// 辅助函数
// =============================================================================

// buildGatewayPath 根据协议和模型名构建网关请求路径
func buildGatewayPath(protocol, model string) string {
	switch protocol {
	case "openai", "deepseek", "openrouter":
		return fmt.Sprintf("/gateway/%s/v1/chat/completions", protocol)
	case "anthropic":
		return fmt.Sprintf("/gateway/%s/v1/messages", protocol)
	case "gemini":
		return fmt.Sprintf("/gateway/%s/v1/models/%s:generateContent", protocol, model)
	default:
		return fmt.Sprintf("/gateway/%s/v1/chat/completions", protocol)
	}
}

// buildDebugRequestBody 构建测试请求体
func buildDebugRequestBody(protocol, modelID string) []byte {
	var body map[string]interface{}
	if protocol == "gemini" {
		body = map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]interface{}{{"text": "Say 'hello' in one word."}}},
			},
			"generationConfig": map[string]interface{}{
				"maxOutputTokens": 1024,
			},
		}
	} else {
		body = map[string]interface{}{
			"model":                 modelID,
			"messages":              []map[string]string{{"role": "user", "content": "Say 'hello' in one word."}},
			"max_completion_tokens": 5,
			"stream":                false,
		}
	}
	b, _ := json.Marshal(body)
	return b
}

// buildCurlCommand 生成 curl 风格的请求日志（包含 URL + headers + body）
func buildCurlCommand(protocol, baseURL, apiKey, modelID string, reqBody []byte) string {
	var apiURL string
	switch protocol {
	case "gemini":
		apiURL = fmt.Sprintf("%s/models/%s:generateContent?key=%s", strings.TrimSuffix(baseURL, "/"), modelID, maskKey(apiKey))
	case "anthropic":
		apiURL = fmt.Sprintf("%s/v1/messages", strings.TrimSuffix(baseURL, "/"))
	default:
		apiURL = fmt.Sprintf("%s/v1/chat/completions", strings.TrimSuffix(baseURL, "/"))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("curl -X POST %q", apiURL))
	sb.WriteString(" \\\n  -H \"Content-Type: application/json\"")

	switch protocol {
	case "anthropic":
		sb.WriteString(fmt.Sprintf(" \\\n  -H \"x-api-key: %s\"", maskKey(apiKey)))
		sb.WriteString(fmt.Sprintf(" \\\n  -H \"anthropic-version: 2023-06-01\""))
	}

	// 格式化 body 为 multiline curl 风格
	bodyPretty := compactJSON(reqBody)
	// 将 body 缩进对齐到 -d 后面
	indentedBody := indentJSON(bodyPretty, "  ")
	sb.WriteString(fmt.Sprintf(" \\\n  -d '%s'", indentedBody))
	return sb.String()
}

// maskKey 遮蔽 API Key 中间部分
func maskKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// indentJSON 给 JSON 字符串每行添加缩进
func indentJSON(s string, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// compactJSON 格式化 JSON 为紧凑字符串
func compactJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

// safePrefix 安全截取前缀
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// truncateStr 截断字符串
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// extractBodyFromDump 从 httputil.DumpResponse 输出中提取纯响应 body。
// dump 格式: "HTTP/1.x Status\r\nHeaders...\r\n\r\nbody..."
func extractBodyFromDump(dump []byte) string {
	s := string(dump)
	idx := bytes.Index(dump, []byte("\r\n\r\n"))
	if idx == -1 {
		return s
	}
	return s[idx+4:]
}

// inferUpstreamProtocol 从 ModelLog 推断上游协议
func inferUpstreamProtocol(mlog *model.ModelLog) string {
	if mlog.ProviderID > 0 {
		var p model.Provider
		if err := model.DB.First(&p, mlog.ProviderID).Error; err == nil {
			protos := p.SupportedProtocols()
			if len(protos) > 0 {
				return protos[0]
			}
		}
	}
	return mlog.Source
}

// conversionLossDetail 转换损失详情
type conversionLossDetail struct {
	EntryProtocol    string   `json:"entry_protocol"`
	UpstreamProtocol string   `json:"upstream_protocol"`
	IsDirect         bool     `json:"is_direct"`
	LostFields       []string `json:"lost_fields"`
	Warnings         []string `json:"warnings"`
}

// computeConversionLoss 计算协议转换损失
func computeConversionLoss(entry, upstream string) *conversionLossDetail {
	if entry == "" || upstream == "" || entry == upstream {
		return nil
	}

	entryCaps := capabilities.Get(entry)
	upCaps := capabilities.Get(upstream)
	if entryCaps == nil || upCaps == nil {
		return nil
	}

	convResult := conversion.Compare(entryCaps, upCaps)
	if convResult == nil || !convResult.IsConversion {
		return nil
	}

	return &conversionLossDetail{
		EntryProtocol:    entry,
		UpstreamProtocol: upstream,
		IsDirect:         false,
		LostFields:       convResult.LostFields,
		Warnings:         convResult.Warnings,
	}
}
