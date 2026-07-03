# Changelog

## [Unreleased] — 2026-07-04

### Added
- **DeepSeek 协议支持**: 新增 `/gateway/deepseek/v1/chat/completions` 端点，完整支持 reasoning_content
- **OpenRouter 协议支持**: 新增 `/gateway/openrouter/v1/chat/completions` 端点
- **Gemini 协议支持**: 新增 `/gateway/gemini/v1beta/models/:model:generateContent` 和 `streamGenerateContent` 端点
- **轴辐式协议转换** (Hub-and-Spoke): 以 OpenAI 格式为统一中间表示 (Unified)，任意协议间双向自动转换
- **Hybrid 路由模式**: API Key 新增 hybrid 模式，优先 Direct 穿透，fallback Mapping 映射
- **可观测性中间件**: OpenTelemetry 集成 + Prometheus `/metrics` 端点 + 请求级 Trace ID
- **Slug 路由**: Key/Provider/Model/MCP 统一使用 6 位短 Hash Slug 替代数字 ID
- **协议对比页面**: Web UI 新增协议对比功能，可视化各协议能力差异
- **表格排序**: 所有列表页支持按列排序
- **Key 重置**: 支持一键重置 API Key 生成新密钥
- **模型测试**: 支持对虚拟模型和 Provider Model 发起测试调用

### Changed
- **API 端点重构**: `/openai/v1/*` + `/anthropic/v1/*` 分离路由 → 统一 `/gateway/:protocol/*`（轴辐式，所有协议共用统一网关处理器）
- **厂商列表页**: 显示各厂商支持的协议格式标签
- **Key 详情页**: 新增协议标签、直通白名单 Tab、映射白名单 Tab 分离
- **模型路由**: `Route()` / `RouteDirect()` 添加 Provider nil 空指针保护

### Fixed
- 修复模型测试后 `is_available=false` 导致路由永久跳过 Provider Model
- 修复 `updateProviderModelRequest` 缺少 `IsAvailable` 字段
- 修复删除模型后 localStorage 持久化隐藏 ID 导致刷新恢复
- 修复 toggle 按钮 v-model/@change 时序问题
- 修复 slug 路由 Number(param) 导致 NaN
- 修复中间件闭包变量污染问题
- 修复 Gemini/DeepSeek 厂商协议转换中流式 thinking/tool_use 字段

### Security
- 协议端点强制 Key Format 校验：跨协议 Key 返回 403（如 OpenAI Key 不可用于 Anthropic 端点）
- API Key 直通白名单与映射白名单冲突检测：同一 model_id 不可同时存在于 key_provider_models 和 key_models（双保险）
- 路由层双重检查：添加模型时检查 + 请求时运行时检查

---

## Previous

### v0.x (before Hub-and-Spoke refactor)
- OpenAI ↔ Anthropic 双向协议转换
- MCP 协议代理 (JSON-RPC 2.0)
- Web 管理控制台 (Vue 3 + i18n + 暗色模式)
- SQLite / PostgreSQL 双数据库支持
- Provider 故障转移 & 冷却机制 (Cooldown Manager)
- 用量统计仪表盘 & 请求日志
