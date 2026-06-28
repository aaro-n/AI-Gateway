package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/provider/capabilities"
)

// ProtocolCompareHandler 协议对比 API
type ProtocolCompareHandler struct{}

func NewProtocolCompareHandler() *ProtocolCompareHandler {
	return &ProtocolCompareHandler{}
}

// GetAllProtocols 返回所有协议及其能力
// GET /api/v1/protocols/compare
func (h *ProtocolCompareHandler) GetAllProtocols(c *gin.Context) {
	all := capabilities.GetAll()
	c.JSON(http.StatusOK, gin.H{
		"protocols": all,
	})
}

// GetProtocolCaps 返回单个协议的能力
// GET /api/v1/protocols/compare/:protocol
func (h *ProtocolCompareHandler) GetProtocolCaps(c *gin.Context) {
	protocol := c.Param("protocol")
	caps := capabilities.Get(protocol)
	if caps == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "protocol not found: " + protocol})
		return
	}
	c.JSON(http.StatusOK, caps)
}

// Compare 对比两个协议间的能力差异
// GET /api/v1/protocols/compare/:from/:to
func (h *ProtocolCompareHandler) Compare(c *gin.Context) {
	from := c.Param("from")
	to := c.Param("to")

	result := capabilities.Compare(from, to)
	c.JSON(http.StatusOK, result)
}

// CompareAll 返回所有协议间的两两对比
// GET /api/v1/protocols/compare-all
func (h *ProtocolCompareHandler) CompareAll(c *gin.Context) {
	results := capabilities.CompareAll()
	c.JSON(http.StatusOK, gin.H{
		"comparisons": results,
	})
}
