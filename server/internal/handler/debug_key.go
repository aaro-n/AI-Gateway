package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/gin-gonic/gin"

	coreErrors "ai-gateway/internal/core/errors"
	"ai-gateway/internal/model"
)

// =============================================================================
// POST /api/v1/debug/test-key
// =============================================================================

func (h *DebugHandler) TestKey(c *gin.Context) {
	var req testKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := func() string { return time.Now().Format(time.RFC3339Nano) }
	logs := make([]debugLogEntry, 0)

	// Step 1: 查询 Key 信息
	var k model.Key
	if err := model.DB.Preload("Formats").First(&k, req.KeyID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	// 非 admin 只能测试自己的 key
	if !IsAdmin(c) {
		uid := GetCurrentUserID(c)
		if k.UserID == nil || *k.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: can only test your own keys"})
			return
		}
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
	resp.Body = io.NopCloser(bytes.NewReader(respDump))
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

		if mlog.CallMethod == "direct" && mlog.Source == protocol {
			isDirect = true
			entryProtocol = protocol
			upstreamProtocol = protocol
		} else if mlog.CallMethod == "convert" {
			isDirect = false
			entryProtocol = mlog.Source
			upstreamProtocol = inferUpstreamProtocol(&mlog)
			if convStatusStr == "" || convStatusStr == "ok" {
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
			CreatedAt:       log.CreatedAt,
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
