## 1. 模型管理 Bug 修复

- [x] 1.1 修复 Delete handler 级联删除
  - 在事务中添加 `KeyProviderModel` 删除 (`tx.Where("provider_model_id = ?", pm.ID).Delete(&model.KeyProviderModel{})`)
  - 解决 DeepSeek 删除模型时外键约束冲突导致 500 错误
- [x] 1.2 修复 Create handler 重复键处理
  - 检测 `duplicate key` 错误后查询已存在记录
  - 执行 upsert 更新字段（保留 manual source 不被 sync 覆盖）
  - 解决 Gemini/OpenAI 重新添加模型时报 "模型 ID 添加失败"

## 2. Keys API 优化

- [x] 2.1 替换 models 数组为计数
  - `keyListItemResponse` 移除 `Models` 字段
  - 新增 `DirectCount` (key_provider_models 数量) 和 `MappingCount` (key_models 数量)
  - 值为 0 表示不限制
- [x] 2.2 前端适配
  - `Keys/index.vue`: 展示计数徽标替代完整模型列表

## 3. 协议同步改进

- [x] 3.1 Anthropic 模型 ID 规范化
  - `sync.go`: 新增 `normalizeModelID()` 使用正则 `-\d{8}$` 去除日期后缀
  - 支持从 API 拉取的带日期模型 ID 匹配本地 models.json
- [x] 3.2 创建各厂商 models.json
  - Anthropic: 7 个模型 (opus-4-8, sonnet-4-6, haiku-4-5 等)
  - DeepSeek: 已存在 + 更新
  - Gemini: 2 个模型 (gemini-2.5-pro, gemini-2.5-flash)
  - OpenAI: 多个 GPT 模型
  - OpenRouter: 路由模型

## 4. Access Mode Routing 修复

- [x] 4.1 修正 gateway_proxy.go routing 逻辑
  - `hybrid` 模式下不再无条件执行 direct 路由
  - 修复 `mapping` 模式的 fallback 行为

## 5. 基础设施

- [x] 5.1 Docker Compose PostgreSQL 配置
  - `docker-compose.yml`: postgres:16-alpine + healthcheck
  - `.env.example`: 默认 dev 凭据
  - `.gitignore`: 排除 `pgdata/` 数据目录和 `.env` 凭据文件
- [x] 5.2 部署辅助脚本
  - `deploy-pg.sh`: 清理旧容器 + 重建 PG

## 6. 其他

- [x] 6.1 debug.go: max_completion_tokens 调整为 1024（支持 thinking 模型测试）
- [x] 6.2 gemini/register.go: AuthExtractor 新增 `Authorization: Bearer` fallback
- [x] 6.3 model.go: `providerBasicResponse` 新增 `Endpoints` map
- [x] 6.4 AGENTS.md: 新增构建顺序和 Auth 参考文档
