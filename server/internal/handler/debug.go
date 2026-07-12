package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-gateway/internal/core/conversion"
	"ai-gateway/internal/model"
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
	Detail    string `json:"detail"` // 额外详情（JSON 字符串）
}

type providerTestResult struct {
	ProviderID      uint            `json:"provider_id"`
	ProviderName    string          `json:"provider_name"`
	Protocol        string          `json:"protocol"`
	TestModel       string          `json:"test_model"`
	AvailableModels []string        `json:"available_models"`
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
	ConvStatus    string          `json:"conv_status"`
	ConvDetail    json.RawMessage `json:"conv_detail"`
	IsDirect      bool            `json:"is_direct"`
	LostFeatures  []string        `json:"lost_features"`
	EntryProtocol string          `json:"entry_protocol"`
	UpstreamProto string          `json:"upstream_protocol"`
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
	CreatedAt       time.Time       `json:"created_at"`
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
			"max_completion_tokens": 1024,
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
		sb.WriteString(" \\\n  -H \"anthropic-version: 2023-06-01\"")
	}

	bodyPretty := compactJSON(reqBody)
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
