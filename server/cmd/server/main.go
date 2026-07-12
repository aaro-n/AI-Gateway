package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"ai-gateway/internal/config"
	coreHandler "ai-gateway/internal/core/handler"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/debug"
	"ai-gateway/internal/handler"
	"ai-gateway/internal/mcp"
	"ai-gateway/internal/middleware"
	"ai-gateway/internal/model"
	"ai-gateway/internal/monitor"
	"ai-gateway/internal/telemetry"
	"ai-gateway/res"

	// 错误日志（终端输出）
	coreErrors "ai-gateway/internal/core/errors"

	// 协议插件（init() 自注册）
	_ "ai-gateway/internal/protocols/anthropic"
	_ "ai-gateway/internal/protocols/deepseek"
	_ "ai-gateway/internal/protocols/gemini"
	_ "ai-gateway/internal/protocols/openai"
	_ "ai-gateway/internal/protocols/openrouter"
)

func main() {
	// 日志级别（环境变量 AG_LOG_LEVEL=debug|info|warn|error）
	if lv := os.Getenv("AG_LOG_LEVEL"); lv != "" {
		coreErrors.SetLevel(coreErrors.ParseLevel(lv))
		log.Printf("[Config] Log level set to: %s", lv)
	}

	cfg := config.Load()

	log.Printf("AI Gateway %s", res.Version)

	// ── 日志文件输出 ──
	if cfg.Debug.LogFile != "" {
		f, err := os.OpenFile(cfg.Debug.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("[Log] Warning: cannot open log file %s: %v (falling back to stdout)", cfg.Debug.LogFile, err)
		} else {
			coreErrors.SetOutput(f)
			log.Printf("[Log] Writing logs to %s", cfg.Debug.LogFile)
		}
	}

	// ── 内存环形日志缓冲区（供调试页面查看运行时日志）──
	coreErrors.EnableRingBuffer(500)

	// ── OpenTelemetry 初始化 ──
	ctx := context.Background()
	otelProviders, err := telemetry.InitOpenTelemetry(ctx, telemetry.Config{
		Enabled:     cfg.Monitor.Otel.Enabled,
		Endpoint:    cfg.Monitor.Otel.Endpoint,
		ServiceName: cfg.Monitor.Otel.ServiceName,
	})
	if err != nil {
		log.Fatalf("Failed to init OpenTelemetry: %v", err)
	}
	if otelProviders != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := telemetry.Shutdown(shutdownCtx, otelProviders); err != nil {
				log.Printf("[OTel] Shutdown error: %v", err)
			}
		}()
		log.Printf("[OTel] OpenTelemetry initialized, endpoint=%s", cfg.Monitor.Otel.Endpoint)
	}

	// ── 监控初始化 (Prometheus / OpenTelemetry 指标) ──
	if cfg.Monitor.Prometheus.Enabled || cfg.Monitor.Otel.Enabled {
		if err := monitor.InitMonitoring(monitor.Config{
			EnablePrometheus: cfg.Monitor.Prometheus.Enabled,
			EnableOtel:       cfg.Monitor.Otel.Enabled,
		}); err != nil {
			log.Fatalf("Failed to init monitoring: %v", err)
		}
	}

	if err := model.InitDB(
		cfg.Database.Type,
		cfg.Database.Path,
		cfg.Database.URL,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.Pool.MaxOpen,
		cfg.Database.Pool.MaxIdle,
		cfg.Database.Pool.MaxLifetime,
		cfg.Database.Pool.MaxIdleTime,
		cfg.Debug.Gorm,
	); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	if err := model.InitDefaultAdmin(cfg.Auth.DefaultAdmin.Username, cfg.Auth.DefaultAdmin.Password); err != nil {
		log.Fatalf("Failed to init default admin: %v", err)
	}

	debug.SetEnabled(cfg.Debug.Provider)
	mcp.SetDebugMode(cfg.Debug.MCP)

	if cfg.Debug.Gin {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Printf("Warning: Failed to set trusted proxies: %v, using default configuration", err)
	}

	r.Use(middleware.CORS())
	r.Use(middleware.TraceID()) // 请求级 Trace ID（第一个中间件，后续都需要）
	r.Use(middleware.RequestLogger())

	// ── 可观测性中间件 ──
	if otelProviders != nil {
		r.Use(otelgin.Middleware(cfg.Monitor.Otel.ServiceName))
	}
	if cfg.Monitor.Prometheus.Enabled {
		r.Use(middleware.PrometheusMiddleware())
	}

	r.Use(middleware.SetupSessionStore(
		cfg.Server.Session.Secret,
		cfg.Server.Session.MaxAge,
		cfg.Server.Session.Secure,
		cfg.Server.Session.HttpOnly,
		cfg.Server.Session.SameSite,
	))

	// 错误日志中间件（panic 恢复 + HTTP 状态上报 → 终端输出）
	r.Use(coreErrors.GinRecovery())
	r.Use(coreErrors.GinErrorReporter())

	authHandler := handler.NewAuthHandler()
	providerHandler := handler.NewProviderHandler()
	providerModelHandler := handler.NewProviderModelHandler()
	modelHandler := handler.NewModelHandler()
	keyHandler := handler.NewKeyHandler()
	usageHandler := handler.NewUsageHandler()
	mcpProxyHandler := handler.NewMCPProxyHandler()
	mcpHandler := handler.NewMCPHandler()
	modelTestHandler := handler.NewModelTestHandler()
	protocolCompareHandler := handler.NewProtocolCompareHandler()
	debugHandler := handler.NewDebugHandler()

	// Unified Gateway（基于 Registry + Unified 中间表示，轴辐式协议转换）
	unifiedGatewayHandler := coreHandler.NewUnifiedGatewayHandler()
	gateway := r.Group("/gateway/:protocol")
	{
		gateway.POST("/*path", unifiedGatewayHandler.Handle)
		gateway.GET("/*path", unifiedGatewayHandler.Handle)
		gateway.DELETE("/*path", unifiedGatewayHandler.Handle)
	}

	mcp := r.Group("/mcp/v1")
	mcp.Use(middleware.RequireAPIKeyForMCP())
	{
		mcp.GET("", mcpProxyHandler.Handle)
		mcp.POST("", mcpProxyHandler.Handle)
		mcp.DELETE("", mcpProxyHandler.Handle)
	}

	api := r.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/logout", authHandler.Logout)
		}

		protected := api.Group("")
		protected.Use(middleware.RequireAuth())
		protected.Use(middleware.ResolveSlug(""))
		{
			protected.GET("/auth/me", authHandler.Me)
			protected.PUT("/auth/password", authHandler.ChangePassword)

			protected.GET("/providers", providerHandler.List)
			protected.GET("/providers/meta/protocols", providerHandler.GetProtocolsMeta)
			protected.POST("/providers/test-connection", providerHandler.TestConnection)
			protected.POST("/providers/test-model", modelTestHandler.TestUnsavedProviderModel)
			protected.GET("/providers/:id", providerHandler.Get)
			protected.POST("/providers", providerHandler.Create)
			protected.PUT("/providers/:id", providerHandler.Update)
			protected.DELETE("/providers/:id", providerHandler.Delete)
			protected.POST("/providers/:id/test", providerHandler.Test)
			protected.POST("/providers/:id/test-custom", modelTestHandler.TestCustomModel)

			protected.GET("/providers/:id/models", providerModelHandler.List)
			protected.POST("/providers/:id/models", providerModelHandler.Create)
			protected.PUT("/providers/:id/models/:mid", providerModelHandler.Update)
			protected.DELETE("/providers/:id/models/:mid", providerModelHandler.Delete)
			protected.POST("/providers/:id/sync", providerModelHandler.Sync)
			protected.POST("/providers/:id/models/lookup", providerModelHandler.Lookup)
			protected.POST("/providers/models/lookup-batch", providerModelHandler.LookupBatch)
			protected.POST("/providers/:id/models/:mid/test", modelTestHandler.TestProviderModel)
			protected.GET("/providers/:id/test-results", modelTestHandler.GetTestResults)

			protected.GET("/models", modelHandler.List)
			protected.POST("/models", modelHandler.Create)
			protected.GET("/models/:id", modelHandler.Get)
			protected.PUT("/models/:id", modelHandler.Update)
			protected.DELETE("/models/:id", modelHandler.Delete)

			protected.GET("/models/:id/mappings", modelHandler.ListMappings)
			protected.POST("/models/:id/mappings", modelHandler.CreateMapping)
			protected.PUT("/models/:id/mappings/order", modelHandler.UpdateMappingsOrder)
			protected.PUT("/models/:id/mappings/:mid", modelHandler.UpdateMapping)
			protected.DELETE("/models/:id/mappings/:mid", modelHandler.DeleteMapping)
			protected.POST("/models/:id/test", modelTestHandler.TestModel)
			protected.GET("/models/:id/capabilities", modelHandler.GetCapabilities)

			protected.GET("/keys", keyHandler.List)
			protected.GET("/keys/:id", keyHandler.Get)
			protected.POST("/keys", keyHandler.Create)
			protected.PUT("/keys/:id", keyHandler.Update)
			protected.DELETE("/keys/:id", keyHandler.Delete)
			protected.POST("/keys/:id/reset", keyHandler.Reset)
			protected.GET("/keys/:id/models", keyHandler.ListModels)
			protected.POST("/keys/:id/models/:model_id", keyHandler.AddModel)
			protected.DELETE("/keys/:id/models/:model_id", keyHandler.RemoveModel)
			protected.PUT("/keys/:id/models/:model_id", keyHandler.ToggleModel)
			protected.DELETE("/keys/:id/models", keyHandler.ClearModels)
			protected.PUT("/keys/:id/models", keyHandler.EnableAllModels)
			protected.GET("/keys/:id/providers", keyHandler.ListProviders)
			protected.POST("/keys/:id/providers/:provider_id", keyHandler.AddProvider)
			protected.DELETE("/keys/:id/providers/:provider_id", keyHandler.RemoveProvider)
			protected.DELETE("/keys/:id/providers", keyHandler.ClearProviders)
			protected.GET("/keys/:id/provider-models", keyHandler.ListProviderModels)
			protected.POST("/keys/:id/provider-models/:pmid", keyHandler.AddProviderModel)
			protected.DELETE("/keys/:id/provider-models/:pmid", keyHandler.RemoveProviderModel)
			protected.PUT("/keys/:id/provider-models/:pmid", keyHandler.ToggleProviderModel)
			protected.DELETE("/keys/:id/provider-models", keyHandler.ClearProviderModels)
			protected.PUT("/keys/:id/provider-models", keyHandler.EnableAllProviderModels)
			protected.GET("/keys/:id/mcp-tools", keyHandler.GetMCPTools)
			protected.POST("/keys/:id/mcp-tools/:tool_id", keyHandler.AddMCPTool)
			protected.DELETE("/keys/:id/mcp-tools/:tool_id", keyHandler.RemoveMCPTool)
			protected.DELETE("/keys/:id/mcp-tools", keyHandler.ClearMCPTools)
			protected.PUT("/keys/:id/mcp-tools", keyHandler.UpdateMCPTools)
			protected.GET("/keys/:id/mcp-resources", keyHandler.GetMCPResources)
			protected.POST("/keys/:id/mcp-resources/:resource_id", keyHandler.AddMCPResource)
			protected.DELETE("/keys/:id/mcp-resources/:resource_id", keyHandler.RemoveMCPResource)
			protected.DELETE("/keys/:id/mcp-resources", keyHandler.ClearMCPResources)
			protected.PUT("/keys/:id/mcp-resources", keyHandler.UpdateMCPResources)
			protected.GET("/keys/:id/mcp-prompts", keyHandler.GetMCPPrompts)
			protected.POST("/keys/:id/mcp-prompts/:prompt_id", keyHandler.AddMCPPrompt)
			protected.DELETE("/keys/:id/mcp-prompts/:prompt_id", keyHandler.RemoveMCPPrompt)
			protected.DELETE("/keys/:id/mcp-prompts", keyHandler.ClearMCPPrompts)
			protected.PUT("/keys/:id/mcp-prompts", keyHandler.UpdateMCPPrompts)

			protected.GET("/usage/dashboard", usageHandler.Dashboard)
			protected.GET("/usage/model-logs", usageHandler.ModelLogs)
			protected.GET("/usage/mcp-logs", usageHandler.MCPLogs)

			protected.GET("/mcps", mcpHandler.List)
			protected.POST("/mcps", mcpHandler.Create)
			protected.GET("/mcps/:id", mcpHandler.Get)
			protected.PUT("/mcps/:id", mcpHandler.Update)
			protected.DELETE("/mcps/:id", mcpHandler.Delete)
			protected.POST("/mcps/:id/test", mcpHandler.TestConnection)
			protected.POST("/mcps/:id/sync", mcpHandler.Sync)
			protected.GET("/mcps/:id/tools", mcpHandler.ListTools)
			protected.PUT("/mcps/tools/:id", mcpHandler.UpdateTool)
			protected.GET("/mcps/:id/resources", mcpHandler.ListResources)
			protected.PUT("/mcps/resources/:id", mcpHandler.UpdateResource)
			protected.GET("/mcps/:id/prompts", mcpHandler.ListPrompts)
			protected.PUT("/mcps/prompts/:id", mcpHandler.UpdatePrompt)

			// ── 协议元数据（前端动态渲染）──
			protected.GET("/protocols", func(c *gin.Context) {
				all := registry.All()
				result := make([]gin.H, 0, len(all))
				for _, desc := range all {
					result = append(result, gin.H{
						"name":             desc.Name,
						"label":            desc.Label,
						"key_prefix":       desc.KeyPrefix,
						"default_base_url": desc.DefaultBaseURL,
						"form_schema":      desc.FormSchema,
					})
				}
				c.JSON(http.StatusOK, gin.H{
					"protocols":        result,
					"test_concurrency": cfg.Server.TestConcurrency,
				})
			})

			// ── 协议对比：查看各协议能力及两两对比差异 ──
			protected.GET("/protocols/compare", protocolCompareHandler.GetAllProtocols)
			protected.GET("/protocols/compare/:protocol", protocolCompareHandler.GetProtocolCaps)
			protected.GET("/protocols/compare-between/:from/:to", protocolCompareHandler.Compare)
			protected.GET("/protocols/compare-all", protocolCompareHandler.CompareAll)

			// ── 调试工具：测试供应商、测试密钥、运行日志 ──
			protected.POST("/debug/test-providers", debugHandler.TestProviders)
			protected.POST("/debug/test-key", debugHandler.TestKey)
			protected.GET("/debug/recent-logs", debugHandler.RecentLogs)
			protected.GET("/debug/server-logs", debugHandler.ServerLogs)
			// ── Prometheus Metrics 端点 ──
			if cfg.Monitor.Prometheus.Enabled {
				if token := cfg.Monitor.Prometheus.MetricsToken; token != "" {
					r.GET("/metrics", middleware.MetricsAuth(token), gin.WrapH(promhttp.Handler()))
				} else {
					r.GET("/metrics", gin.WrapH(promhttp.Handler()))
				}
				log.Printf("[Metrics] Prometheus metrics endpoint: /metrics")
			}

		}
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": res.Version})
	})

	r.NoRoute(middleware.Static(res.WebFS))

	go func() {
		pprofAddr := fmt.Sprintf("localhost:%d", cfg.Pprof.Port)
		log.Printf("[Pprof] Performance profiling server starting on http://%s/debug/pprof/", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			log.Printf("[Pprof] Failed to start pprof server: %v", err)
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// 优雅关闭：SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
