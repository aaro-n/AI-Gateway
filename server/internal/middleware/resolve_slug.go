package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

// ResolveSlug 根据指定表名将 :id slug 解析为数字 ID。
// table 为空字符串时，从路由路径自动检测（/keys/→keys, /providers/→providers...）。
func ResolveSlug(table string) gin.HandlerFunc {
	return func(c *gin.Context) {
		param := c.Param("id")
		if param == "" {
			c.Next()
			return
		}
		// 如果 param 是纯数字，直接跳过（不需要 slug 解析）
		if _, err := strconv.Atoi(param); err == nil {
			c.Next()
			return
		}
		// 自动检测表名
		if table == "" {
			path := c.Request.URL.Path
			for _, prefix := range []string{"/keys", "/providers", "/models", "/mcps"} {
				if strings.HasPrefix(path, "/api/v1"+prefix) || strings.HasPrefix(path, prefix) {
					table = strings.TrimPrefix(prefix, "/")
					break
				}
			}
		}
		if table == "" {
			c.Next()
			return
		}
		id, ok := model.ResolveID(param, "slug", table)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": table + " not found"})
			c.Abort()
			return
		}
		c.Set("resolved_id", id)
		c.Next()
	}
}

// GetID 从 context 获取 resolved_id，否则回退到 ParseUint。
func GetID(c *gin.Context) (uint, error) {
	if v, exists := c.Get("resolved_id"); exists {
		return v.(uint), nil
	}
	return parseUint(c.Param("id"))
}

// GetIDParam 解析任意路由参数（如 :model_id、:pmid）。
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
