package gemini

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/httpclient"
	"ai-gateway/internal/core/registry"
)

// GeminiProvider Gemini 协议实现
type GeminiProvider struct {
	cfg      *registry.Config
	httpPool *http.Client
}

// NewGeminiProvider 创建 Gemini Provider
func NewGeminiProvider(cfg *registry.Config) *GeminiProvider {
	return &GeminiProvider{cfg: cfg, httpPool: httpclient.Pool()}
}

// IsStreamRequest 通过 URL 路径判断是否为流式请求。
// Gemini 通过 URL 区分流式（streamGenerateContent）和非流式（generateContent），
// 请求体中没有 stream 字段。
func (p *GeminiProvider) IsStreamRequest(c *gin.Context) bool {
	return strings.Contains(c.Request.URL.Path, "streamGenerateContent")
}
