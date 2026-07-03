package middleware

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ── 缓存策略常量 ──

const (
	// hashedAssetMaxAge 带 hash 的静态资源缓存 7 天
	hashedAssetMaxAge = 7 * 24 * 60 * 60 // 604800s
)

var (
	// buildTime 编译时间，作为 embed 文件的统一 Last-Modified
	buildTime = time.Now()
)

// Static 提供前端静态文件服务，内置缓存校验：
//
//   - index.html / 非 hash 文件 → Cache-Control: no-cache + ETag + Last-Modified
//   - /assets/* 带 hash 的文件    → Cache-Control: public, max-age=604800 + ETag + Last-Modified
//
// ETag 基于文件内容 MD5+大小生成；Last-Modified 使用编译时间。
// 支持 If-None-Match / If-Modified-Since 条件请求返回 304。
func Static(fsys fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 跳过 API / 健康检查路由
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/openai") ||
			strings.HasPrefix(path, "/mcp") || path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		// SPA fallback: 静态文件不存在 → index.html
		name := strings.TrimPrefix(path, "/")
		stat, err := fs.Stat(fsys, name)
		if err != nil {
			path = "/"
			name = "index.html"
			stat, _ = fs.Stat(fsys, name)
			c.Request.URL.Path = "/"
		}

		// 打开文件计算 ETag
		file, err := fsys.Open(name)
		if err != nil {
			c.Next()
			return
		}

		hash := md5.New()
		size, _ := io.Copy(hash, file)
		file.Close()

		etag := fmt.Sprintf(`W/"%x-%x"`, hash.Sum(nil)[:8], size)

		// 设置 Last-Modified
		c.Header("Last-Modified", buildTime.UTC().Format(http.TimeFormat))
		c.Header("ETag", etag)

		// 缓存策略
		if isHashedAsset(path) {
			c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", hashedAssetMaxAge))
		} else {
			c.Header("Cache-Control", "no-cache")
		}

		// 条件请求：If-None-Match
		if match := c.GetHeader("If-None-Match"); match != "" {
			if match == etag || match == "*" {
				c.Status(http.StatusNotModified)
				c.Abort()
				return
			}
		}

		// 条件请求：If-Modified-Since
		if since := c.GetHeader("If-Modified-Since"); since != "" {
			if t, err := time.Parse(http.TimeFormat, since); err == nil && !buildTime.Truncate(time.Second).After(t) {
				c.Status(http.StatusNotModified)
				c.Abort()
				return
			}
		}

		// 重新打开文件写入响应
		file2, err := fsys.Open(name)
		if err != nil {
			c.Next()
			return
		}
		defer file2.Close()

		// 设置 Content-Type
		if stat != nil && !stat.IsDir() {
			setContentType(c, name)
		}

		// 显式设 200 —— NoRoute 中间件预置 404，io.Copy 不会覆盖
		c.Status(http.StatusOK)
		io.Copy(c.Writer, file2)
		c.Abort()
	}
}

// isHashedAsset 判断路径是否指向带内容指纹的静态资源。
//
// Vite 文件命名格式：name-[8位hash].ext，hash 为 base64 编码（A-Za-z0-9_-）
func isHashedAsset(path string) bool {
	if !strings.HasPrefix(path, "/assets/") {
		return false
	}

	name := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		name = path[idx+1:]
	}
	if idx := strings.LastIndexByte(name, '.'); idx > 0 {
		name = name[:idx]
	}
	idx := strings.LastIndexByte(name, '-')
	if idx < 0 {
		return false
	}
	hash := name[idx+1:]
	return isBase64Hash(hash)
}

func isBase64Hash(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// setContentType 根据文件扩展名设置 Content-Type
func setContentType(c *gin.Context, name string) {
	switch {
	case strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs"):
		c.Header("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		c.Header("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".html"):
		c.Header("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".svg"):
		c.Header("Content-Type", "image/svg+xml")
	case strings.HasSuffix(name, ".json"):
		c.Header("Content-Type", "application/json")
	case strings.HasSuffix(name, ".png"):
		c.Header("Content-Type", "image/png")
	case strings.HasSuffix(name, ".ico"):
		c.Header("Content-Type", "image/x-icon")
	case strings.HasSuffix(name, ".woff2"):
		c.Header("Content-Type", "font/woff2")
	case strings.HasSuffix(name, ".woff"):
		c.Header("Content-Type", "font/woff")
	}
}
