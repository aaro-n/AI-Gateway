package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	protocolsPkg "ai-gateway/internal/protocols"
)

type ModelTestHandler struct{}

func NewModelTestHandler() *ModelTestHandler {
	return &ModelTestHandler{}
}

type protocolTestResult struct {
	Protocol     string `json:"protocol"`
	Success      bool   `json:"success"`
	CallMethod   string `json:"call_method"`
	LatencyMs    int64  `json:"latency_ms"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Response     string `json:"response"`
	Error        string `json:"error"`
}

type providerModelTestResponse struct {
	Provider struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	} `json:"provider"`
	Model struct {
		ModelID     string `json:"model_id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Tests []protocolTestResult `json:"tests"`
}

type customModelTestRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

type testProviderModelRequest struct {
	ProviderID uint              `json:"provider_id"`
	Endpoints  map[string]string `json:"endpoints"` // {"openai":"https://...","gemini":"https://..."}
	APIKey     string            `json:"api_key"`
	ModelID    string            `json:"model_id" binding:"required"`
}

type customModelTestResponse struct {
	Provider struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	} `json:"provider"`
	Model struct {
		ModelID     string `json:"model_id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Tests []protocolTestResult `json:"tests"`
}

func (h *ModelTestHandler) TestCustomModel(c *gin.Context) {
	providerID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	var req customModelTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_id is required"})
		return
	}

	var p model.Provider
	if err := model.DB.First(&p, providerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	pm := &model.ProviderModel{
		ModelID: req.ModelID,
	}

	protocols := p.SupportedProtocols()
	results := make([]protocolTestResult, len(protocols))
	var wg sync.WaitGroup

	for i, proto := range protocols {
		wg.Add(1)
		go func(idx int, pr string) {
			defer wg.Done()
			results[idx] = executeTest(&p, pm, pr)
		}(i, proto)
	}

	wg.Wait()

	tests := make([]protocolTestResult, 0, len(protocols))
	for _, r := range results {
		if r.Protocol != "" {
			tests = append(tests, r)
		}
	}

	// 保存测试结果
	saveTestResults(p.ID, 0, req.ModelID, tests)

	resp := customModelTestResponse{
		Provider: struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		}{ID: p.ID, Name: p.Name},
		Model: struct {
			ModelID     string `json:"model_id"`
			DisplayName string `json:"display_name"`
		}{ModelID: req.ModelID},
		Tests: tests,
	}

	c.JSON(http.StatusOK, resp)
}

func (h *ModelTestHandler) TestUnsavedProviderModel(c *gin.Context) {
	var req testProviderModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将 Endpoints map 序列化为 JSON 存入 Provider
	endpointsJSON, _ := json.Marshal(req.Endpoints)
	p := &model.Provider{
		Endpoints: string(endpointsJSON),
		APIKey:    req.APIKey,
	}

	// 如果 APIKey 是 DUMMY_KEY_FOR_EDIT 或为空，从已有 Provider 补齐
	if req.APIKey == "DUMMY_KEY_FOR_EDIT" || req.APIKey == "" {
		if req.ProviderID > 0 {
			var existing model.Provider
			if err := model.DB.First(&existing, req.ProviderID).Error; err == nil && existing.APIKey != "" {
				p.APIKey = existing.APIKey
				// 如果前端未传 Endpoints，从已有 Provider 补齐
				if len(req.Endpoints) == 0 {
					p.Endpoints = existing.Endpoints
				}
			}
		}
	}

	pm := &model.ProviderModel{
		ModelID: req.ModelID,
	}

	var wg sync.WaitGroup
	protocols := p.SupportedProtocols()
	results := make([]protocolTestResult, len(protocols))

	for i, proto := range protocols {
		wg.Add(1)
		go func(idx int, pr string) {
			defer wg.Done()
			results[idx] = executeTest(p, pm, pr)
		}(i, proto)
	}

	wg.Wait()

	tests := make([]protocolTestResult, 0, len(protocols))
	for _, r := range results {
		if r.Protocol != "" {
			tests = append(tests, r)
		}
	}

	// 保存测试结果（ProviderID 可能为 0，表示未保存的 provider）
	providerID := req.ProviderID
	saveTestResults(providerID, 0, req.ModelID, tests)

	c.JSON(http.StatusOK, gin.H{
		"model_id": req.ModelID,
		"tests":    tests,
	})
}

func (h *ModelTestHandler) TestProviderModel(c *gin.Context) {
	providerID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	modelDBID, err := strconv.ParseUint(c.Param("mid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var p model.Provider
	if err := model.DB.First(&p, providerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	var pm model.ProviderModel
	if err := model.DB.Where("id = ? AND provider_id = ?", modelDBID, providerID).First(&pm).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider model not found"})
		return
	}

	var wg sync.WaitGroup
	protocols := p.SupportedProtocols()
	results := make([]protocolTestResult, len(protocols))

	for i, proto := range protocols {
		wg.Add(1)
		go func(idx int, pr string) {
			defer wg.Done()
			results[idx] = executeTest(&p, &pm, pr)
		}(i, proto)
	}

	wg.Wait()

	tests := make([]protocolTestResult, 0, len(protocols))
	for _, r := range results {
		if r.Protocol != "" {
			tests = append(tests, r)
		}
	}

	// 保存测试结果并更新 is_available
	saveTestResults(p.ID, pm.ID, pm.ModelID, tests)

	resp := providerModelTestResponse{
		Provider: struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		}{ID: p.ID, Name: p.Name},
		Model: struct {
			ModelID     string `json:"model_id"`
			DisplayName string `json:"display_name"`
		}{ModelID: pm.ModelID, DisplayName: pm.DisplayName},
		Tests: tests,
	}

	c.JSON(http.StatusOK, resp)
}

type mappingTestResult struct {
	MappingID     uint                 `json:"mapping_id"`
	Weight        int                  `json:"weight"`
	Provider      providerBasicInfo    `json:"provider"`
	ProviderModel modelBasicInfo       `json:"provider_model"`
	ProtocolTests []protocolTestResult `json:"protocol_tests"`
}

type providerBasicInfo struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type modelBasicInfo struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
}

type virtualModelTestResponse struct {
	ModelName string              `json:"model_name"`
	Tests     []mappingTestResult `json:"tests"`
}

func (h *ModelTestHandler) TestModel(c *gin.Context) {
	modelID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return
	}

	var m model.Model
	if err := model.DB.First(&m, modelID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var mappings []model.ModelMapping
	model.DB.Preload("Provider").Preload("ProviderModel").
		Where("model_id = ? AND enabled = ?", m.ID, true).
		Order("weight DESC").
		Find(&mappings)

	type testJob struct {
		mapping model.ModelMapping
		pm      *model.ProviderModel
	}

	var jobs []testJob
	for _, mapping := range mappings {
		if mapping.Provider == nil || !mapping.Provider.Enabled {
			continue
		}

		if mapping.ProviderModel == nil || !mapping.ProviderModel.IsAvailable {
			continue
		}

		jobs = append(jobs, testJob{mapping: mapping, pm: mapping.ProviderModel})
	}

	results := make([]mappingTestResult, len(jobs))
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j testJob) {
			defer wg.Done()

			protocolTests := []protocolTestResult{}
			for _, proto := range j.mapping.Provider.SupportedProtocols() {
				result := executeTest(j.mapping.Provider, j.pm, proto)
				protocolTests = append(protocolTests, result)
			}

			results[idx] = mappingTestResult{
				MappingID: j.mapping.ID,
				Weight:    j.mapping.Weight,
				Provider: providerBasicInfo{
					ID:      j.mapping.Provider.ID,
					Name:    j.mapping.Provider.Name,
					Enabled: j.mapping.Provider.Enabled,
				},
				ProviderModel: modelBasicInfo{
					ModelID:     j.pm.ModelID,
					DisplayName: j.pm.DisplayName,
				},
				ProtocolTests: protocolTests,
			}

			// 保存测试结果并更新 is_available
			saveTestResults(j.mapping.Provider.ID, j.pm.ID, j.pm.ModelID, protocolTests)
		}(i, job)
	}

	wg.Wait()

	c.JSON(http.StatusOK, virtualModelTestResponse{
		ModelName: m.Name,
		Tests:     results,
	})
}

func executeTest(p *model.Provider, pm *model.ProviderModel, protocol string) protocolTestResult {
	baseURL := p.EndpointFor(protocol)
	result := protocolsPkg.RunTest(protocol, baseURL, p.APIKey, pm.ModelID, 5)
	return protocolTestResult{
		Protocol:     protocol,
		Success:      result.Success,
		CallMethod:   result.CallMethod,
		LatencyMs:    result.LatencyMs,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Response:     result.Response,
		Error:        result.Error,
	}
}

func extractResponseContent(body []byte, protocol string) string {
	if len(body) == 0 {
		return ""
	}

	if protocol == "openai" {
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		if len(resp.Choices) > 0 {
			return strings.TrimSpace(resp.Choices[0].Message.Content)
		}
	} else if protocol == "anthropic" {
		var resp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		for _, c := range resp.Content {
			if c.Type == "text" && c.Text != "" {
				return strings.TrimSpace(c.Text)
			}
		}
	} else if protocol == "gemini" {
		var resp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			return strings.TrimSpace(resp.Candidates[0].Content.Parts[0].Text)
		}
	}

	return ""
}

// saveTestResults 保存测试结果到数据库，并更新 ProviderModel 的可用状态
func saveTestResults(providerID uint, providerModelID uint, modelID string, tests []protocolTestResult) {
	// 只保存有 providerID 的测试结果，避免 provider_id=0 污染查询
	if providerID == 0 {
		return
	}
	for _, t := range tests {
		tr := model.TestResult{
			ProviderID:      providerID,
			ProviderModelID: providerModelID,
			ModelID:         modelID,
			Protocol:        t.Protocol,
			Success:         t.Success,
			CallMethod:      t.CallMethod,
			LatencyMs:       t.LatencyMs,
			InputTokens:     t.InputTokens,
			OutputTokens:    t.OutputTokens,
			Response:        t.Response,
			Error:           t.Error,
		}
		if err := model.DB.Create(&tr).Error; err != nil {
			log.Printf("[TestResult] Failed to save test result: %v", err)
		}
	}

	// 测试结果仅用于展示，不自动修改路由可用性 (is_available)。
	// 路由可用性由 cooldown manager 实时管理，测试结果不应产生副作用影响线上路由。
	_ = providerModelID // 保留参数兼容性
}

// getTestResultsResponse 某个模型的最近一次测试结果
type modelTestResultSummary struct {
	ModelID string `json:"model_id"`
	Success bool   `json:"success"`
	Latency int64  `gorm:"column:latency_ms" json:"latency_ms"`
	Error   string `json:"error"`
}

// GetTestResults 获取指定渠道所有模型的最近一次测试结果
func (h *ModelTestHandler) GetTestResults(c *gin.Context) {
	providerID, err := middleware.GetID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}

	// 获取该渠道下所有 provider_models 的 model_id
	var modelIDs []string
	model.DB.Model(&model.ProviderModel{}).Where("provider_id = ?", providerID).Pluck("model_id", &modelIDs)

	var results []modelTestResultSummary
	for _, mid := range modelIDs {
		var latest model.TestResult
		err := model.DB.Where("(provider_id = ? OR provider_id = 0) AND model_id = ?", providerID, mid).
			Order("created_at DESC").
			First(&latest).Error
		if err != nil {
			continue
		}
		results = append(results, modelTestResultSummary{
			ModelID: latest.ModelID,
			Success: latest.Success,
			Latency: latest.LatencyMs,
			Error:   latest.Error,
		})
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}
