## 1. Thinking 管道 — 核心类型

- [x] `internal/core/unified/thinking/types.go`：ThinkingConfig、ThinkingMode、ModelThinkingCap、KnownModelCaps、LookupCap、FromUnified
- [x] `internal/core/unified/thinking/convert.go`：LevelToBudget、BudgetToLevel、ClampBudget、ValidateLevel
- [x] `internal/core/unified/thinking/validate.go`：ValidateAndConvert（Budget↔Level 互转 + clamp + 校验）

## 2. UnifiedRequest 扩展

- [x] `unified/types.go`：Request 新增 `ThkConfig *thinking.ThinkingConfig` 字段（json:"-"）

## 3. 协议插件 — FromUnified 注入

- [x] `anthropic/request_build.go`：ThkConfig 优先 → thinking type=budget_tokens，回退到旧字段
- [x] `gemini/request_build.go`：ThkConfig → thinkingConfig.thinkingBudget
- [x] `openai/request_build.go`：ThkConfig → reasoning_effort/budget
- [x] `deepseek/request_build.go`：ThkConfig → reasoning_effort/budget
- [x] `openrouter/request_build.go`：ThkConfig → reasoning_effort/budget

## 4. 网关注入

- [x] `gateway_proxy.go`：步骤 7.5 — FromUnified → LookupCap → ValidateAndConvert → 注入 ThkConfig

## 5. 配置热加载 — 核心

- [x] `internal/config/reloader.go`：Reload()、RWMutex 保护 Get()
- [x] `internal/config/config.go`：Load() 使用 Mutex、移除旧 Get()、configPath 包级变量

## 6. 配置热加载 — 触发点

- [x] `cmd/server/main.go`：SIGHUP 信号处理（goroutine 监听 quit channel）
- [x] `internal/handler/admin.go`：POST /api/v1/admin/reload-config

## 7. 构建验证

- [x] `cd web && make build` 前端构建通过
- [x] `cd server && go build -o server ./cmd/server/` 后端构建通过
- [x] `go vet ./...` 零警告