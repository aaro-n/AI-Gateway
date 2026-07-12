package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
)

// =============================================================================
// POST /api/v1/debug/test-providers
// =============================================================================

func (h *DebugHandler) TestProviders(c *gin.Context) {
	// 仅 admin 可以测试厂商
	if !IsAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return
	}
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
	now := func() string { return time.Now().Format(time.RFC3339Nano) }

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

			// 执行测试（1024 tokens 确保思考模型有足够空间）
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
