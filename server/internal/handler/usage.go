package handler

import (
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

// userTimeZone 从 session 获取当前用户的时区偏好。
// 用户未设置时区时回退到服务器本地时区（time.Local，由 AG_TIME_ZONE 配置）。
func userTimeZone(c *gin.Context) *time.Location {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID != nil {
		var user model.User
		if err := model.DB.Select("time_zone").First(&user, userID).Error; err == nil && user.TimeZone != "" {
			if loc, err := time.LoadLocation(user.TimeZone); err == nil {
				return loc
			}
		}
	}
	return time.Local
}

type UsageHandler struct{}

type dailyStat struct {
	Date    string `json:"date"`
	Count   int64  `json:"count"`
	Success int64  `json:"success"`
}

type tokenDailyStat struct {
	Date        string `json:"date"`
	TotalTokens int64  `json:"total_tokens"`
}

func NewUsageHandler() *UsageHandler {
	return &UsageHandler{}
}

func (h *UsageHandler) Dashboard(c *gin.Context) {
	nDays := 7
	// 按当前用户时区计算日边界，与下方 groupByDate 的 Go 端时区处理一致。
	// 数据库存储 UTC，此处用用户时区转换，避免 SQLite/PostgreSQL DATE() 行为差异。
	userLoc := userTimeZone(c)
	now := time.Now().In(userLoc)
	nDaysAgo := now.AddDate(0, 0, -nDays).Format("2006-01-02")
	lastNDays := generateLastNDaysIn(nDays+1, userLoc)

	uid := GetCurrentUserID(c)
	isAdmin := IsAdmin(c)

	// 非管理员：获取当前用户的 key ID 列表，用于过滤日志
	var userKeyIDs []uint
	if !isAdmin {
		var keys []model.Key
		model.DB.Where("user_id = ?", uid).Select("id").Find(&keys)
		for _, k := range keys {
			userKeyIDs = append(userKeyIDs, k.ID)
		}
	}

	// 资产统计 — 非管理员仅统计自己有权限的资源
	var totalProviders, activeProviders int64
	var totalModels, activeModels int64
	var totalProviderModels, activeProviderModels int64
	var totalMCPs, activeMCPs int64
	var totalKeys, activeKeys int64
	var totalMCPTools, totalMCPResources, totalMCPPrompts int64
	var activeMCPTools, activeMCPResources, activeMCPPrompts int64

	if isAdmin {
		model.DB.Model(&model.Provider{}).Count(&totalProviders)
		model.DB.Model(&model.Provider{}).Where("enabled = ?", true).Count(&activeProviders)
		model.DB.Model(&model.Model{}).Count(&totalModels)
		model.DB.Model(&model.Model{}).Where("enabled = ?", true).Count(&activeModels)
		model.DB.Model(&model.ProviderModel{}).Count(&totalProviderModels)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT pm.id)
			FROM provider_models pm
			JOIN providers p ON pm.provider_id = p.id
			WHERE pm.is_available = ? AND p.enabled = ?
				AND p.deleted_at IS NULL
		`, true, true).Scan(&activeProviderModels)
		model.DB.Model(&model.MCP{}).Count(&totalMCPs)
		model.DB.Model(&model.MCP{}).Where("enabled = ?", true).Count(&activeMCPs)
		model.DB.Model(&model.Key{}).Count(&totalKeys)
		model.DB.Model(&model.Key{}).Where("enabled = ?", true).Count(&activeKeys)
		model.DB.Model(&model.MCPTool{}).Count(&totalMCPTools)
		model.DB.Model(&model.MCPResource{}).Count(&totalMCPResources)
		model.DB.Model(&model.MCPPrompt{}).Count(&totalMCPPrompts)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mt.id)
			FROM mcp_tools mt
			JOIN mcps m ON mt.mcp_id = m.id
			WHERE mt.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPTools)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mr.id)
			FROM mcp_resources mr
			JOIN mcps m ON mr.mcp_id = m.id
			WHERE mr.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPResources)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mp.id)
			FROM mcp_prompts mp
			JOIN mcps m ON mp.mcp_id = m.id
			WHERE mp.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPPrompts)
	} else {
		// 非管理员：统计其有权限的厂商
		model.DB.Model(&model.Provider{}).
			Joins("JOIN user_providers up ON up.provider_id = providers.id AND up.user_id = ?", uid).
			Count(&totalProviders)
		model.DB.Model(&model.Provider{}).
			Joins("JOIN user_providers up ON up.provider_id = providers.id AND up.user_id = ?", uid).
			Where("providers.enabled = ?", true).
			Count(&activeProviders)
		// 统计其有权限的模型映射
		model.DB.Model(&model.Model{}).
			Joins("JOIN user_models um ON um.model_id = models.id AND um.user_id = ?", uid).
			Count(&totalModels)
		model.DB.Model(&model.Model{}).
			Joins("JOIN user_models um ON um.model_id = models.id AND um.user_id = ?", uid).
			Where("models.enabled = ?", true).
			Count(&activeModels)
		// 非管理员的厂商模型数沿用权限厂商下的模型数
		model.DB.Raw(`
			SELECT COUNT(DISTINCT pm.id)
			FROM provider_models pm
			JOIN providers p ON pm.provider_id = p.id
			JOIN user_providers up ON up.provider_id = p.id AND up.user_id = ?
			WHERE p.enabled = ? AND p.deleted_at IS NULL
		`, uid, true).Scan(&totalProviderModels)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT pm.id)
			FROM provider_models pm
			JOIN providers p ON pm.provider_id = p.id
			JOIN user_providers up ON up.provider_id = p.id AND up.user_id = ?
			WHERE pm.is_available = ? AND p.enabled = ? AND p.deleted_at IS NULL
		`, uid, true, true).Scan(&activeProviderModels)
		// MCP 暂不按用户过滤
		model.DB.Model(&model.MCP{}).Count(&totalMCPs)
		model.DB.Model(&model.MCP{}).Where("enabled = ?", true).Count(&activeMCPs)
		// 只统计当前用户的 Key
		model.DB.Model(&model.Key{}).Where("user_id = ?", uid).Count(&totalKeys)
		model.DB.Model(&model.Key{}).Where("user_id = ? AND enabled = ?", uid, true).Count(&activeKeys)
		model.DB.Model(&model.MCPTool{}).Count(&totalMCPTools)
		model.DB.Model(&model.MCPResource{}).Count(&totalMCPResources)
		model.DB.Model(&model.MCPPrompt{}).Count(&totalMCPPrompts)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mt.id)
			FROM mcp_tools mt
			JOIN mcps m ON mt.mcp_id = m.id
			WHERE mt.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPTools)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mr.id)
			FROM mcp_resources mr
			JOIN mcps m ON mr.mcp_id = m.id
			WHERE mr.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPResources)
		model.DB.Raw(`
			SELECT COUNT(DISTINCT mp.id)
			FROM mcp_prompts mp
			JOIN mcps m ON mp.mcp_id = m.id
			WHERE mp.enabled = ? AND m.enabled = ?
				AND m.deleted_at IS NULL
		`, true, true).Scan(&activeMCPPrompts)
	}

	// Model API 统计 (过去N天) — 非管理员仅统计自己的 Key 产生的日志
	modelLogQuery := model.DB.Model(&model.ModelLog{}).Where("created_at >= ?", nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		modelLogQuery = modelLogQuery.Where("key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		// 用户没有 key，直接返回零值
		modelLogQuery = modelLogQuery.Where("1 = 0")
	}

	var modelTotalRequests int64
	modelLogQuery.Count(&modelTotalRequests)

	var modelSuccessCount int64
	modelLogQuery.Where("status = ?", "success").Count(&modelSuccessCount)

	var modelTotalTokens int64
	modelLogQuery.Select("COALESCE(SUM(total_tokens), 0)").Scan(&modelTotalTokens)

	var modelAvgLatency float64
	modelLogQuery.Select("COALESCE(AVG(latency_ms), 0)").Scan(&modelAvgLatency)

	// 按本地时区在 Go 端分组，避免 SQLite/PostgreSQL DATE() 时区行为差异
	var modelDailyRows []struct {
		CreatedAt time.Time
		Status    string
	}
	modelLogQuery.Select("created_at, status").Scan(&modelDailyRows)
	modelDailyStats := groupByDate(lastNDays, modelDailyRows,
		func(r struct {
			CreatedAt time.Time
			Status    string
		}) string {
			return r.CreatedAt.In(userLoc).Format("2006-01-02")
		},
		func(date string) dailyStat { return dailyStat{Date: date} },
		func(acc *dailyStat, r struct {
			CreatedAt time.Time
			Status    string
		}) {
			acc.Count++
			if r.Status == "success" {
				acc.Success++
			}
		})

	var modelTokenRows []struct {
		CreatedAt   time.Time
		TotalTokens int64
	}
	modelLogQuery.Select("created_at, total_tokens").Scan(&modelTokenRows)
	modelTokenDailyStats := groupByDate(lastNDays, modelTokenRows,
		func(r struct {
			CreatedAt   time.Time
			TotalTokens int64
		}) string {
			return r.CreatedAt.In(userLoc).Format("2006-01-02")
		},
		func(date string) tokenDailyStat { return tokenDailyStat{Date: date} },
		func(acc *tokenDailyStat, r struct {
			CreatedAt   time.Time
			TotalTokens int64
		}) {
			acc.TotalTokens += r.TotalTokens
		})

	var providerStats []struct {
		Provider   string  `json:"provider"`
		Count      int64   `json:"count"`
		Tokens     int64   `json:"tokens"`
		AvgLatency float64 `json:"avg_latency"`
	}
	provSQL := model.DB.Raw(`
		SELECT 
			p.name as provider, 
			COUNT(*) as count,
			COALESCE(SUM(ml.total_tokens), 0) as tokens,
			COALESCE(AVG(ml.latency_ms), 0) as avg_latency
		FROM model_logs ml
		JOIN providers p ON ml.provider_id = p.id
		WHERE ml.created_at >= ?
	`, nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		provSQL = provSQL.Where("ml.key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		provSQL = provSQL.Where("1 = 0")
	}
	provSQL.Group("p.name").Order("count DESC").Scan(&providerStats)

	var modelStats []struct {
		Model string `json:"model"`
		Count int64  `json:"count"`
	}
	modelSQL := model.DB.Raw(`
		SELECT model, COUNT(*) as count
		FROM model_logs
		WHERE created_at >= ?
	`, nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		modelSQL = modelSQL.Where("key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		modelSQL = modelSQL.Where("1 = 0")
	}
	modelSQL.Group("model").Order("count DESC").Limit(10).Scan(&modelStats)

	// MCP 服务统计 (过去7天) — 非管理员仅统计自己的 Key 产生的日志
	mcpLogQuery := model.DB.Model(&model.MCPLog{}).Where("created_at >= ?", nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		mcpLogQuery = mcpLogQuery.Where("key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		mcpLogQuery = mcpLogQuery.Where("1 = 0")
	}

	var mcpTotalRequests int64
	mcpLogQuery.Count(&mcpTotalRequests)

	var mcpSuccessCount int64
	mcpLogQuery.Where("status = ?", "success").Count(&mcpSuccessCount)

	var mcpTotalSize int64
	mcpLogQuery.Select("COALESCE(SUM(input_size + output_size), 0)").Scan(&mcpTotalSize)

	var mcpAvgLatency float64
	mcpLogQuery.Select("COALESCE(AVG(latency_ms), 0)").Scan(&mcpAvgLatency)

	// 按本地时区在 Go 端分组
	var mcpDailyRows []struct {
		CreatedAt time.Time
		Status    string
	}
	mcpLogQuery.Select("created_at, status").Scan(&mcpDailyRows)
	mcpDailyStats := groupByDate(lastNDays, mcpDailyRows,
		func(r struct {
			CreatedAt time.Time
			Status    string
		}) string {
			return r.CreatedAt.In(userLoc).Format("2006-01-02")
		},
		func(date string) dailyStat { return dailyStat{Date: date} },
		func(acc *dailyStat, r struct {
			CreatedAt time.Time
			Status    string
		}) {
			acc.Count++
			if r.Status == "success" {
				acc.Success++
			}
		})

	var mcpTypeStats []struct {
		MCPType string `json:"mcp_type"`
		Count   int64  `json:"count"`
	}
	mcpTypeSQL := model.DB.Raw(`
		SELECT mcp_type, COUNT(*) as count
		FROM mcp_logs
		WHERE created_at >= ?
	`, nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		mcpTypeSQL = mcpTypeSQL.Where("key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		mcpTypeSQL = mcpTypeSQL.Where("1 = 0")
	}
	mcpTypeSQL.Group("mcp_type").Order("count DESC").Scan(&mcpTypeStats)

	var mcpServiceStats []struct {
		MCPName string `json:"mcp_name"`
		Count   int64  `json:"count"`
	}
	mcpSvcSQL := model.DB.Raw(`
		SELECT mcp_name, COUNT(*) as count
		FROM mcp_logs
		WHERE created_at >= ?
	`, nDaysAgo)
	if !isAdmin && len(userKeyIDs) > 0 {
		mcpSvcSQL = mcpSvcSQL.Where("key_id IN ?", userKeyIDs)
	} else if !isAdmin && len(userKeyIDs) == 0 {
		mcpSvcSQL = mcpSvcSQL.Where("1 = 0")
	}
	mcpSvcSQL.Group("mcp_name").Order("count DESC").Limit(10).Scan(&mcpServiceStats)

	c.JSON(http.StatusOK, gin.H{
		"days": nDays,
		"assets": gin.H{
			"totalProviders":       totalProviders,
			"activeProviders":      activeProviders,
			"totalModels":          totalModels,
			"activeModels":         activeModels,
			"totalProviderModels":  totalProviderModels,
			"activeProviderModels": activeProviderModels,
			"totalMCPs":            totalMCPs,
			"activeMCPs":           activeMCPs,
			"totalKeys":            totalKeys,
			"activeKeys":           activeKeys,
			"totalMCPTools":        totalMCPTools,
			"activeMCPTools":       activeMCPTools,
			"totalMCPResources":    totalMCPResources,
			"activeMCPResources":   activeMCPResources,
			"totalMCPPrompts":      totalMCPPrompts,
			"activeMCPPrompts":     activeMCPPrompts,
		},
		"modelUsage": gin.H{
			"totalRequests":   modelTotalRequests,
			"successCount":    modelSuccessCount,
			"totalTokens":     modelTotalTokens,
			"avgLatency":      modelAvgLatency,
			"dailyStats":      modelDailyStats,
			"tokenDailyStats": modelTokenDailyStats,
			"providerStats":   providerStats,
			"modelStats":      modelStats,
		},
		"mcpUsage": gin.H{
			"totalRequests": mcpTotalRequests,
			"successCount":  mcpSuccessCount,
			"totalSize":     mcpTotalSize,
			"avgLatency":    mcpAvgLatency,
			"dailyStats":    mcpDailyStats,
			"typeStats":     mcpTypeStats,
			"serviceStats":  mcpServiceStats,
		},
	})
}

// generateLastNDaysIn 生成最近 n 天的日期序列（YYYY-MM-DD），按指定时区计算。
func generateLastNDaysIn(n int, loc *time.Location) []string {
	now := time.Now().In(loc)
	days := make([]string, n)
	for i := 0; i < n; i++ {
		offset := -n + 1 + i
		days[i] = now.AddDate(0, 0, offset).Format("2006-01-02")
	}
	return days
}

// groupByDate 按日期字符串（由 getDate 从每行提取，通常用本地时区格式化）将日志行
// 聚合到 days 序列对应的桶中。缺失日期用 createEmpty 填充。
//
// 这样做是为了避免 SQLite DATE()（按 UTC）与 PostgreSQL DATE()（按服务器时区）
// 的行为差异——所有日期归属统一在 Go 端按 time.Local（由 AG_TIME_ZONE 配置）计算。
func groupByDate[T any, Acc any](
	days []string,
	rows []T,
	getDate func(T) string,
	createEmpty func(string) Acc,
	accumulate func(acc *Acc, row T),
) []Acc {
	result := make([]Acc, len(days))
	idx := make(map[string]int, len(days))
	for i, day := range days {
		result[i] = createEmpty(day)
		idx[day] = i
	}
	for _, r := range rows {
		d := getDate(r)
		if i, ok := idx[d]; ok {
			accumulate(&result[i], r)
		}
	}
	return result
}

type modelLogResponse struct {
	ID              uint      `json:"id"`
	Source          string    `json:"source"`
	ClientIPs       string    `json:"client_ips"`
	KeyID           uint      `json:"key_id"`
	KeyName         string    `json:"key_name"`
	Model           string    `json:"model"`
	ProviderID      uint      `json:"provider_id"`
	ProviderName    string    `json:"provider_name"`
	ActualModelID   string    `json:"actual_model_id"`
	ActualModelName string    `json:"actual_model_name"`
	CallMethod      string    `json:"call_method"`
	CachedTokens    int       `json:"cached_tokens"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	LatencyMs       int       `json:"latency_ms"`
	Status          string    `json:"status"`
	ErrorMsg        string    `json:"error_msg"`
	ConvStatus      string    `json:"conv_status"`
	CreatedAt       time.Time `json:"created_at"`
}

func (h *UsageHandler) ModelLogs(c *gin.Context) {
	userLoc := userTimeZone(c)
	startDate := c.DefaultQuery("start_date", time.Now().In(userLoc).Format("2006-01-02 00:00:00"))
	endDate := c.DefaultQuery("end_date", time.Now().In(userLoc).AddDate(0, 0, 1).Format("2006-01-02 00:00:00"))

	startTime, err := time.ParseInLocation("2006-01-02 15:04:05", startDate, userLoc)
	if err != nil {
		startTime, err = time.ParseInLocation("2006-01-02", startDate, userLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start date format"})
			return
		}
	}
	endTime, err := time.ParseInLocation("2006-01-02 15:04:05", endDate, userLoc)
	if err != nil {
		endTime, err = time.ParseInLocation("2006-01-02", endDate, userLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end date format"})
			return
		}
	}

	var modelLogs []model.ModelLog
	if err := model.DB.Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Order("created_at DESC").
		Find(&modelLogs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logsResponses := make([]modelLogResponse, len(modelLogs))
	for i, log := range modelLogs {
		logsResponses[i] = modelLogResponse{
			ID:              log.ID,
			Source:          log.Source,
			ClientIPs:       log.ClientIPs,
			KeyID:           log.KeyID,
			KeyName:         log.KeyName,
			Model:           log.Model,
			ProviderID:      log.ProviderID,
			ProviderName:    log.ProviderName,
			ActualModelID:   log.ActualModelID,
			ActualModelName: log.ActualModelName,
			CallMethod:      log.CallMethod,
			CachedTokens:    log.CachedTokens,
			InputTokens:     log.InputTokens,
			OutputTokens:    log.OutputTokens,
			TotalTokens:     log.TotalTokens,
			LatencyMs:       log.LatencyMs,
			Status:          log.Status,
			ErrorMsg:        log.ErrorMsg,
			ConvStatus:      log.ConvStatus,
			CreatedAt:       log.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"logs": logsResponses})
}

func NewMCPLog(source string, clientIPs string, keyID uint, keyName string, mcpID uint, mcpName string, mcpType string, callType string, callTarget string, callMethod string, inputSize int, outputSize int, latencyMs int, status string, errorMsg string) *model.MCPLog {
	return &model.MCPLog{
		Source:     source,
		ClientIPs:  clientIPs,
		KeyID:      keyID,
		KeyName:    keyName,
		MCPID:      mcpID,
		MCPName:    mcpName,
		MCPType:    mcpType,
		CallType:   callType,
		CallTarget: callTarget,
		CallMethod: callMethod,
		InputSize:  inputSize,
		OutputSize: outputSize,
		LatencyMs:  latencyMs,
		Status:     status,
		ErrorMsg:   errorMsg,
	}
}

type mcpLogResponse struct {
	ID         uint      `json:"id"`
	Source     string    `json:"source"`
	ClientIPs  string    `json:"client_ips"`
	KeyID      uint      `json:"key_id"`
	KeyName    string    `json:"key_name"`
	MCPID      uint      `json:"mcp_id"`
	MCPName    string    `json:"mcp_name"`
	MCPType    string    `json:"mcp_type"`
	CallType   string    `json:"call_type"`
	CallMethod string    `json:"call_method"`
	CallTarget string    `json:"call_target"`
	InputSize  int       `json:"input_size"`
	OutputSize int       `json:"output_size"`
	LatencyMs  int       `json:"latency_ms"`
	Status     string    `json:"status"`
	ErrorMsg   string    `json:"error_msg"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *UsageHandler) MCPLogs(c *gin.Context) {
	userLoc := userTimeZone(c)
	startDate := c.DefaultQuery("start_date", time.Now().In(userLoc).Format("2006-01-02 00:00:00"))
	endDate := c.DefaultQuery("end_date", time.Now().In(userLoc).AddDate(0, 0, 1).Format("2006-01-02 00:00:00"))

	startTime, err := time.ParseInLocation("2006-01-02 15:04:05", startDate, userLoc)
	if err != nil {
		startTime, err = time.ParseInLocation("2006-01-02", startDate, userLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start date format"})
			return
		}
	}
	endTime, err := time.ParseInLocation("2006-01-02 15:04:05", endDate, userLoc)
	if err != nil {
		endTime, err = time.ParseInLocation("2006-01-02", endDate, userLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end date format"})
			return
		}
	}

	var mcpLogs []model.MCPLog
	if err := model.DB.Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Order("created_at DESC").
		Find(&mcpLogs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logsResponses := make([]mcpLogResponse, len(mcpLogs))
	for i, log := range mcpLogs {
		logsResponses[i] = mcpLogResponse{
			ID:         log.ID,
			Source:     log.Source,
			ClientIPs:  log.ClientIPs,
			KeyID:      log.KeyID,
			KeyName:    log.KeyName,
			MCPID:      log.MCPID,
			MCPName:    log.MCPName,
			MCPType:    log.MCPType,
			CallType:   log.CallType,
			CallMethod: log.CallMethod,
			CallTarget: log.CallTarget,
			InputSize:  log.InputSize,
			OutputSize: log.OutputSize,
			LatencyMs:  log.LatencyMs,
			Status:     log.Status,
			ErrorMsg:   log.ErrorMsg,
			CreatedAt:  log.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"logs": logsResponses})
}
