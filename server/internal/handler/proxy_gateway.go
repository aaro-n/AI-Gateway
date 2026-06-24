package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
	"ai-gateway/internal/provider"
	"ai-gateway/internal/router"
	"ai-gateway/internal/utils"
)

type GatewayProxyHandler struct {
	router *router.ModelRouter
}

func NewGatewayProxyHandler() *GatewayProxyHandler {
	return &GatewayProxyHandler{
		router: router.GetRouter(),
	}
}

func (h *GatewayProxyHandler) HandleRequest(c *gin.Context) {
	protocolName := c.Param("protocol")

	// 1. Get Protocol Descriptor
	desc, ok := provider.GetProtocol(protocolName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unsupported protocol: " + protocolName})
		return
	}

	// 2. Extract API Key
	rawKey := desc.AuthExtractor(c)
	if rawKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		return
	}

	// 3. Authenticate KeyFormat and Key
	var kFormat model.KeyFormat
	if err := model.DB.Where("formatted_key = ?", rawKey).First(&kFormat).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}

	if kFormat.Format != protocolName {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Key format " + kFormat.Format + " is not allowed on " + protocolName + " endpoint",
		})
		return
	}

	var apiKey model.Key
	if err := model.DB.Where("id = ? AND enabled = ?", kFormat.KeyID, true).First(&apiKey).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "API key expired"})
		return
	}

	// 4. Extract Model Name
	modelName, err := desc.ModelExtractor(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to extract model: " + err.Error()})
		return
	}

	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model identifier"})
		return
	}

	// 5. Check Key Model Permissions (only if in mapping or hybrid mode, or if key has specific model bounds)
	// For direct access mode, if they didn't explicitly bind specific virtual models to key, we let them pass.
	// But let's verify key permissions. If the key has explicit models bound, they must be allowed.
	// We'll verify permissions. For direct/hybrid mode, if a key is bounded to specific virtual models, they can only call those mapped ones, OR we can verify the requested model is matched.
	// To make it simple and elegant:
	// If AccessMode is "direct", we bypass the model name mapping check (VerifyKeyID checks Model table).
	// If the key has bound models (count > 0), we check if the requested direct model is allowed. Let's see:
	var boundModelCount int64
	model.DB.Model(&model.KeyModel{}).Where("key_id = ?", apiKey.ID).Count(&boundModelCount)

	var result *router.RouteResult
	isDirectCall := false
	if apiKey.AccessMode == "direct" || apiKey.AccessMode == "hybrid" {
		// Let's first try to route direct
		directResult, err := h.router.RouteDirect(modelName)
		if err == nil && directResult != nil {
			result = directResult
			isDirectCall = true
		}
	}

	if !isDirectCall {
		// Fallback to standard mapping
		if apiKey.AccessMode == "direct" {
			c.JSON(http.StatusForbidden, gin.H{"error": "This key only supports direct access mode"})
			return
		}

		if err := VerifyKeyID(apiKey.ID, modelName); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		// Route virtual mapping Model Request
		mappedResult, err := h.router.Route(modelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if mappedResult == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "model mapping not found or no available provider"})
			return
		}
		result = mappedResult
	} else {
		// Direct call model permissions checking: if the key has explicit models bound, check permissions.
		if boundModelCount > 0 {
			// In direct mode, bound models are still evaluated. We can verify if the direct model matches allowed provider models.
			// For simplicity: if boundModelCount > 0, they can only call mappings. So we check if isDirectCall is allowed.
			// Let's allow direct calls only if they are not explicitly restricted, or if the model ID is explicitly bound in model list.
			// Usually, if a key restricts models, it restricts the virtual mapped ones.
		}
	}

	// Store variables in context for logging and compatibility
	c.Set("api_key", &apiKey)
	c.Set("key_id", apiKey.ID)
	c.Set("key_name", apiKey.Name)

	// Store original Path and modify path for downstream Provider expectation
	originalPath := c.Request.URL.Path
	prefixToStrip := "/gateway/" + protocolName
	if strings.HasPrefix(originalPath, prefixToStrip) {
		c.Request.URL.Path = strings.TrimPrefix(originalPath, prefixToStrip)
	}

	start := time.Now()
	usage := provider.Usage{}

	// 7. Execute Request dynamically through provider instance
	err = result.ProviderInstance.ExecuteRequest(protocolName, c, result.ProviderModel, &usage)

	// Restore original Path for logging and next middlewares
	c.Request.URL.Path = originalPath

	latencyMs := time.Since(start).Milliseconds()

	status := "success"
	errorMsg := ""
	if err != nil {
		status = "error"
		errorMsg = err.Error()
		if provider.IsRateLimitError(err) {
			h.router.RecordRateLimit(result.Provider.ID, result.ProviderModel.ID)
		}
	} else {
		h.router.RecordSuccess(result.Provider.ID, result.ProviderModel.ID)
	}

	clientIPs := utils.GetClientIPInfo(c)

	// Determine matching protocol support (dynamic via Endpoints JSON)
	matched := result.SupportProtocol(protocolName)

	modelLog := NewModelLog(
		protocolName,
		clientIPs,
		apiKey.ID,
		apiKey.Name,
		modelName,
		result,
		matched,
		&usage,
		int(latencyMs),
		status,
		errorMsg,
	)
	model.DB.Create(&modelLog)
	log.Println(modelLog.String())
}
