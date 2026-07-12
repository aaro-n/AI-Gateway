# Thinking 管道 + 配置热加载

## 动机

### Thinking 管道

当前 `UnifiedRequest` 中存在 `ReasoningEffort`/`ReasoningBudget` 字段，但：
- OpenAI 的 `ToUnified` 正确解析 `reasoning_effort`
- Anthropic 的 `ToUnified` 正确解析 `thinking.budget_tokens`
- **但跨协议转换时**，thinking 参数未经过校验和转换就透传，导致：
  - Anthropic thinking (`budget_tokens`) 转到 OpenAI 时 `reasoning_effort` 可能不兼容
  - Gemini `thinkingBudget` 转到 Anthropic 时可能超出 `budget_tokens` 范围
  - DeepSeek `reasoning_effort` 无实际作用（DeepSeek 不支持该字段的标准含义）

### 配置热加载

当前 `DebugConfig` 变更需重启服务（Gin/GORM/Provider/MCP debug 日志开关），
多节点部署时需协调重启，运维体验差。

## 方案

### 1. Thinking 管道 (`internal/core/unified/thinking/`)

新增 `thinking` 包，提供统一的思考配置处理管道：

- **`types.go`**：`ThinkingConfig`、`ThinkingMode`（None/Auto/Budget/Level）、`ModelThinkingCap` 模型能力声明
- **`convert.go`**：`LevelToBudget()` / `BudgetToLevel()` / `ClampBudget()` / `ValidateLevel()`
- **`validate.go`**：`ValidateAndConvert()` — 自动进行 Level↔Budget 互转并校验模型能力

管道流程：
```
客户端请求 → ToUnified (解析 thinking → ReasoningEffort/Budget)
    → 网关 7.5: ThinkingConfig.FromUnified() → LookupCap(上游模型)
    → ValidateAndConvert() → unifiedReq.ThkConfig
    → FromUnified (ThkConfig 优先 → 注入协议原生 thinking 参数)
```

### 2. 配置热加载 (`internal/config/reloader.go`)

- **`Reload()`**：重新读取 `config.yaml` + 环境变量，仅更新可热重载字段
  - 热重载：`debug.*`、`test_concurrency`、`time_zone`
  - 忽略（需重启）：数据库配置、server port、session、auth、pprof、monitor
- **SIGHUP 信号处理**：`kill -HUP <pid>` 触发热加载
- **Admin API**：`POST /api/v1/admin/reload-config`（需 session 认证）
- **线程安全**：`sync.RWMutex` 保护 `cfg` 读写

## 修改文件

| 文件 | 变更 |
|---|---|
| `server/internal/core/unified/thinking/types.go` | 新增：ThinkingConfig、ThinkingMode、ModelThinkingCap、KnownModelCaps |
| `server/internal/core/unified/thinking/convert.go` | 新增：Level↔Budget 转换、Clamp、ValidateLevel |
| `server/internal/core/unified/thinking/validate.go` | 新增：ValidateAndConvert 管道核心逻辑 |
| `server/internal/core/unified/types.go` | `Request` 新增 `ThkConfig *thinking.ThinkingConfig` 字段 |
| `server/internal/core/handler/gateway_proxy.go` | 步骤 7.5：注入 Thinking 管道（构建→校验→注入） |
| `server/internal/protocols/anthropic/request_build.go` | FromUnified：ThkConfig 优先，回退到旧字段 |
| `server/internal/protocols/gemini/request_build.go` | FromUnified：ThkConfig → thinkingConfig |
| `server/internal/protocols/openai/request_build.go` | FromUnified：ThkConfig → reasoning_effort/budget |
| `server/internal/protocols/deepseek/request_build.go` | FromUnified：ThkConfig → reasoning_effort/budget |
| `server/internal/protocols/openrouter/request_build.go` | FromUnified：ThkConfig → reasoning_effort/budget |
| `server/internal/config/reloader.go` | 新增：Reload()、RWMutex 保护 Get() |
| `server/internal/config/config.go` | Load() 使用 Mutex、移除旧 Get() |
| `server/internal/handler/admin.go` | 新增：POST /api/v1/admin/reload-config |
| `server/cmd/server/main.go` | SIGHUP 信号处理 + admin 路由注册 |