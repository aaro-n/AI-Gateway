## Why

多个厂商（DeepSeek、Gemini、OpenAI）的模型管理存在 bug：
1. DeepSeek 删除模型失败：`key_provider_models` 有外键引用，删除时未级联清理
2. Gemini/OpenAI 添加模型报重复键错误：`Create` handler 使用严格 INSERT，重复 model_id 触发 unique constraint
3. Anthropic API 模型 ID 带日期后缀（如 `claude-opus-4-8-20250709`），同步时无法匹配本地模型

此外 Keys API 返回完整 models 数组导致列表页请求过大，需要优化。

## What Changes

### Bug Fixes
- `provider_model.go` Delete handler: 添加 `KeyProviderModel` 级联删除，修复 DeepSeek 删除失败
- `provider_model.go` Create handler: 检测 duplicate key 错误后执行 upsert，修复 Gemini/OpenAI 重复添加
- `gateway_proxy.go`: 修正 hybrid/mapping 模式的 routing 逻辑

### API 改进
- `key.go`: 用 `direct_count`/`mapping_count` 替代 `models` 数组，减少列表 API 响应体积
- `model.go`: Provider 响应新增 `Endpoints` map 字段

### 协议同步改进
- `anthropic/sync.go`: 新增 `normalizeModelID()` 去除日期后缀，支持匹配本地模型
- 新增各厂商 `models.json` 本地模型缓存（Anthropic、DeepSeek、Gemini、OpenAI、OpenRouter）

### 基础设施
- `docker-compose.yml` + `.env.example`: PostgreSQL 16 Docker 部署
- `deploy-pg.sh`: 重置 PG 容器的辅助脚本
- `.gitignore`: 排除 `pgdata/` 和 `.env`

### 前端
- `Keys/index.vue`: 展示 `direct_count`/`mapping_count` 替代完整模型列表
- i18n 翻译更新

### 文档
- `AGENTS.md`: 新增构建顺序说明、Auth 和 Key 管理参考

## Capabilities

### Modified Capabilities
- `model-management`: 修复删除/添加模型的 bug，新增 upsert 逻辑
- `api-key-management`: Keys 列表 API 优化，用计数替代完整列表
- `provider-plugin-refactor`: Anthropic 同步新增日期后缀规范化
