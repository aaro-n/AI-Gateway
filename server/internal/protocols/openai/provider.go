package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
)

type OpenAIProvider struct {
	cfg *registry.Config
}

func NewOpenAIProvider(cfg *registry.Config) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg}
}

// =============================================================================
// SyncModels — 从 OpenAI API 同步模型列表
// =============================================================================

type openAIModelEntry struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
	Pricing struct {
		Completion float64 `json:"completion"`
		Prompt     float64 `json:"prompt"`
	} `json:"pricing"`
}

func (p *OpenAIProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var result struct {
		Data []openAIModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    m.ID,
			OwnedBy:        m.OwnedBy,
			InputPrice:     m.Pricing.Prompt,
			OutputPrice:    m.Pricing.Completion,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================
// HandleNative — OpenAI 直通（只替换模型名）
// =============================================================================

func (p *OpenAIProvider) HandleNative(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	// 替换模型名
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return fmt.Errorf("parse body: %w", err)
	}
	bodyMap["model"] = modelID
	body, _ = json.Marshal(bodyMap)

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST",
		p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		ctx.Status(resp.StatusCode)
		ctx.Writer.Write(respBody)
		return fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if p.isStreaming(resp) {
		ctx.Status(http.StatusOK)
		ctx.Header("Content-Type", "text/event-stream")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Header("Connection", "keep-alive")
		return p.copyStream(ctx.Request.Context(), ctx.Writer, resp.Body, usage)
	}

	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "application/json")
	return p.copyResponse(ctx.Writer, resp.Body, usage)
}

// =============================================================================
// FromOpenAI — OpenAI 入口 → 本协上游（同协议，等同 HandleNative）
// =============================================================================

func (p *OpenAIProvider) FromOpenAI(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	return p.HandleNative(ctx, modelID, usage)
}

// =============================================================================
// 内部工具
// =============================================================================

type openAIUsageRaw struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsageRaw) toUsage(usage *registry.Usage) {
	usage.CachedTokens = u.PromptTokensDetails.CachedTokens
	usage.InputTokens = u.PromptTokens - u.PromptTokensDetails.CachedTokens
	usage.OutputTokens = u.CompletionTokens
}

func (p *OpenAIProvider) isStreaming(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return len(resp.Header["Transfer-Encoding"]) > 0 ||
		(len(contentType) >= 17 && contentType[:17] == "text/event-stream")
}

func (p *OpenAIProvider) copyStream(ctx context.Context, dst io.Writer, src io.Reader, usage *registry.Usage) error {
	reader := bufio.NewReader(src)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if _, err := fmt.Fprint(dst, line); err != nil {
			return err
		}
		if flusher, ok := dst.(http.Flusher); ok {
			flusher.Flush()
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Usage openAIUsageRaw `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			chunk.Usage.toUsage(usage)
		}
	}
	return nil
}

func (p *OpenAIProvider) copyResponse(dst io.Writer, src io.Reader, usage *registry.Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	dst.Write(body)

	var resp struct {
		Usage openAIUsageRaw `json:"usage"`
	}
	json.Unmarshal(body, &resp)
	resp.Usage.toUsage(usage)
	return nil
}
