package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

// ResolveSlug 将路由 :id 参数（slug 或数字 ID）解析为数字 ID。
// table 为空时从请求 URL 自动提取表名。
// 闭包内用 t:=table 局部副本，避免跨请求变量污染。
func ResolveSlug(table string) gin.HandlerFunc {
	return func(c *gin.Context) {
		t := table
		param := c.Param("id")
		if param == "" {
			c.Next()
			return
		}
		if t == "" {
			t = detectTable(c.Request.URL.Path)
		}
		if t == "" {
			if n, err := strconv.ParseUint(param, 10, 32); err == nil {
				c.Set("resolved_id", uint(n))
			}
			c.Next()
			return
		}

		id, ok := model.ResolveID(param, "slug", t)
		if ok {
			c.Set("resolved_id", id)
			c.Next()
			return
		}

		if n, err := strconv.ParseUint(param, 10, 32); err == nil {
			c.Set("resolved_id", uint(n))
			c.Next()
			return
		}

		c.JSON(http.StatusNotFound, gin.H{"error": t + " not found: " + param})
		c.Abort()
	}
}

func detectTable(urlPath string) string {
	for _, prefix := range []string{"/api/v1/providers", "/api/v1/models", "/api/v1/keys", "/api/v1/mcps"} {
		if strings.HasPrefix(urlPath, prefix) {
			return strings.TrimPrefix(prefix, "/api/v1/")
		}
	}
	return ""
}

func GetID(c *gin.Context) (uint, error) {
	if v, exists := c.Get("resolved_id"); exists {
		return v.(uint), nil
	}
	return 0, strconv.ErrSyntax
}

func GetIDParam(c *gin.Context, name string) (uint, error) {
	if v, exists := c.Get("resolved_" + name); exists {
		return v.(uint), nil
	}
	return parseUint(c.Param(name))
}

func parseUint(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	return uint(n), err
}
