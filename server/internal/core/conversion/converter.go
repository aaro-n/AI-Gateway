package conversion

import (
	"encoding/json"

	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/protocols/capabilities"
)

// ConversionResult 描述一次跨协议转换的损失分析结果。
type ConversionResult struct {
	IsConversion bool     `json:"is_conversion"` // 是否发生了跨协议转换
	FromProtocol string   `json:"from_protocol"` // 客户端协议
	ToProtocol   string   `json:"to_protocol"`   // 上游协议
	Status       uint64   `json:"status"`        // 损失位掩码 (conv_status)
	Warnings     []string `json:"warnings"`      // 人类可读的警告列表
	LostFields   []string `json:"lost_fields"`   // 被丢弃的具体字段名
}

// Compare 对比客户端协议与上游协议，返回转换损失分析。
//
// 参数：
//   - sourceProtocol: 客户端使用的协议 ("openai", "anthropic", "gemini" 等)
//   - targetProtocol: 上游供应商使用的协议
//   - req: 客户端请求的 Unified 中间表示
//
// 如果 sourceProtocol == targetProtocol，返回 StatusOK（同协议无损失）。
func Compare(sourceProtocol, targetProtocol string, req *unified.Request) *ConversionResult {
	result := &ConversionResult{
		FromProtocol: sourceProtocol,
		ToProtocol:   targetProtocol,
		IsConversion: sourceProtocol != targetProtocol,
	}

	// 同协议无损失
	if sourceProtocol == targetProtocol {
		return result
	}

	// 1. 获取能力对比
	capsResult := capabilities.Compare(sourceProtocol, targetProtocol)

	// 2. 根据客户端实际请求检测具体损失
	var warnings []string
	var lostFields []string
	var status uint64

	// 2.1 工具调用
	if len(req.Tools) > 0 && !targetHasCap(capsResult, "function_calling") {
		status |= StatusToolsLost
		lostFields = append(lostFields, "tools")
		warnings = append(warnings, "工具调用 (function calling) 在上游协议中不受支持，已被丢弃")
	}

	// 2.2 视觉/多模态
	if requestHasVision(req) && !targetHasCap(capsResult, "vision") {
		status |= StatusVisionLost
		lostFields = append(lostFields, "image inputs")
		warnings = append(warnings, "图像/多模态输入在上游协议中不受支持，已被丢弃")
	}

	// 2.3 思考/推理
	if req.ReasoningEffort != "" || req.ReasoningBudget != nil {
		if !targetHasCap(capsResult, "thinking") {
			status |= StatusThinkingLost
			lostFields = append(lostFields, "reasoning_effort/reasoning_budget")
			warnings = append(warnings, "思考/推理参数在上游协议中不受支持，已被丢弃")
		} else {
			status |= StatusFieldDegraded
			warnings = append(warnings, "思考/推理参数可能降级，不同协议的推理机制存在差异")
		}
	}

	// 2.4 流式模式
	if req.Stream && !targetHasCap(capsResult, "streaming") {
		status |= StatusStreamLost
		lostFields = append(lostFields, "stream")
		warnings = append(warnings, "流式模式在上游协议中不受支持，响应将降级为非流式")
	}

	// 2.5 Prompt Caching (通过 TransformerMetadata 中的 cache_control)
	if hasCacheControl(req) && !targetHasCap(capsResult, "prompt_caching") {
		status |= StatusCacheLost
		lostFields = append(lostFields, "cache_control")
		warnings = append(warnings, "Prompt Caching 在上游协议中不受支持，缓存标记已被丢弃")
	}

	// 2.6 系统提示降级
	if req.SystemPrompt != "" && !targetHasCap(capsResult, "system_prompt") {
		status |= StatusSystemPromptDegraded
		warnings = append(warnings, "系统提示在上游协议中的位置/格式不同，已做适配转换")
	}

	// 2.7 JSON 模式
	if len(req.ResponseFormat) > 0 && !targetHasCap(capsResult, "json_mode") {
		status |= StatusJSONModeLost
		lostFields = append(lostFields, "response_format")
		warnings = append(warnings, "JSON 模式/结构化输出在上游协议中不受支持，已被丢弃")
	}

	// 2.8 logprobs（不在 unified.Request 中单独存在，通过工具/响应格式推断）
	// 注：unified.Response 不包含 logprobs，仅做协议级检测
	if sourceHasCap(capsResult, "logprobs") && !targetHasCap(capsResult, "logprobs") {
		status |= StatusLogprobsLost
		warnings = append(warnings, "对数概率 (logprobs) 在上游协议中不可用")
	}

	// 2.9 频率/存在惩罚
	if req.FrequencyPenalty != nil || req.PresencePenalty != nil {
		if !targetHasCap(capsResult, "frequency_penalty") && !targetHasCap(capsResult, "presence_penalty") {
			status |= StatusPenaltyDegraded
			lostFields = append(lostFields, "frequency_penalty/presence_penalty")
			warnings = append(warnings, "频率/存在惩罚参数在上游协议中不受支持，已被丢弃")
		} else if !targetHasCap(capsResult, "frequency_penalty") || !targetHasCap(capsResult, "presence_penalty") {
			status |= StatusFieldDegraded
			warnings = append(warnings, "部分惩罚参数在上游协议中不受支持")
		}
	}

	// 2.10 通用字段丢弃检测：检查 capsResult 中的完整损失列表
	for _, loss := range capsResult.Losses {
		switch loss.CapabilityKey {
		case "function_calling", "vision", "streaming", "thinking",
			"prompt_caching", "system_prompt", "json_mode",
			"logprobs", "frequency_penalty", "presence_penalty":
			// 已在上方精确检测
			continue
		default:
			if status == StatusOK {
				status |= StatusFieldDropped
			}
			lostFields = append(lostFields, loss.CapabilityKey)
			warnings = append(warnings, loss.Note)
		}
	}

	result.Status = status
	result.Warnings = warnings
	result.LostFields = lostFields
	return result
}

// targetHasCap 检查目标协议是否有指定能力（在 ComparisonResult 中未出现在 Losses 中 = 有）。
func targetHasCap(result capabilities.ComparisonResult, capKey string) bool {
	for _, loss := range result.Losses {
		if loss.CapabilityKey == capKey && loss.LossLevel == "full" {
			return false
		}
	}
	return true
}

// sourceHasCap 检查源协议是否有指定能力。
func sourceHasCap(result capabilities.ComparisonResult, capKey string) bool {
	// 若 capKey 在 losses 中，说明源协议有但目标没有 → 源协议肯定有
	for _, loss := range result.Losses {
		if loss.CapabilityKey == capKey {
			return true
		}
	}
	// 不在 losses 中 → 可能两个都有，也可能两个都没有。
	// 通过检查 capabilities 包中源协议的能力列表来判断。
	caps := capabilities.Get(result.FromProtocol)
	if caps == nil {
		return false
	}
	for _, c := range caps.Capabilities {
		if c.Key == capKey {
			return true
		}
	}
	return false
}

// requestHasVision 检测请求中是否包含视觉（图片）内容。
func requestHasVision(req *unified.Request) bool {
	for _, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}
		var blocks []unified.ContentBlock
		if len(msg.Content) > 0 && json.Unmarshal(msg.Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "image_url" || b.Type == "image" {
					return true
				}
			}
		}
	}
	return false
}

// hasCacheControl 检测请求中是否包含 Prompt Caching 标记。
func hasCacheControl(req *unified.Request) bool {
	if req.TransformerMetadata != nil {
		if _, ok := req.TransformerMetadata["cache_control"]; ok {
			return true
		}
	}
	for _, msg := range req.Messages {
		if msg.TransformerMetadata != nil {
			if _, ok := msg.TransformerMetadata["cache_control"]; ok {
				return true
			}
		}
		// 检查 ContentBlocks 中的 cache_control
		for _, b := range unified.ContentBlocks(msg.Content) {
			if b.TransformerMetadata != nil {
				if _, ok := b.TransformerMetadata["cache_control"]; ok {
					return true
				}
			}
		}
	}
	return false
}
