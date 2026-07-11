## ADDED Requirements

### Requirement: 插件化自注册

系统 SHALL 通过 Go `init()` 函数在每个协议包中自注册，新增厂商无需修改 Handler/Model/Router/Core 任何代码。

#### Scenario: 新增 Gemini 协议
- **WHEN** 开发者在 `protocols/gemini/register.go` 中调用 `registry.Register()`
- **AND** 在 `cmd/server/main.go` 添加 `_ "ai-gateway/internal/protocols/gemini"`
- **THEN** 系统自动识别 Gemini 协议并用于路由、测试、同步模型列表

#### Scenario: 新增协议不影响已有代码
- **WHEN** 新增任意协议包
- **THEN** `handler/provider.go` 无需修改字段
- **AND** `handler/model_testing.go` 无需添加 if-else 分支
- **AND** `router/` 无需修改路由逻辑

---

### Requirement: Provider.Endpoints JSON 替代扁平列

系统 SHALL 使用 `endpoints` JSON 列（`{"gemini":"https://...","openai":"https://..."}`）统一管理多协议端点，废弃 `openai_base_url` / `anthropic_base_url` 等 5 个扁平列。

#### Scenario: 创建 Provider 时使用 Endpoints
- **WHEN** 管理员创建 Provider，设置 `endpoints={"openai":"https://api.openai.com/v1"}`
- **THEN** 系统将 endpoints 序列化为 JSON 存入数据库
- **AND** 不再写入 `openai_base_url` 列

#### Scenario: 查询 Provider 时返回 Endpoints
- **WHEN** 管理员查询 Provider 列表
- **THEN** 返回 `"endpoints":{"openai":"https://...","gemini":"https://..."}`
- **AND** 同时兼容旧的扁平列（fallback）

#### Scenario: 读取兼容旧数据
- **WHEN** 旧 Provider 只有 `openai_base_url` 列有值
- **THEN** `EndpointsMap()` 返回 `{"openai":"https://..."}`
- **AND** 新创建的 Provider 优先使用 `endpoints` JSON 列

---

### Requirement: 协议目录标准化

每个协议包 SHALL 放在 `protocols/{name}/` 下，包含标准文件结构。

#### Scenario: 协议包文件完整性
- **WHEN** 检查 `protocols/gemini/`
- **THEN** 必须包含 `register.go`（自注册）
- **AND** `provider.go`（Provider 实现）
- **AND** `request_parse.go`（ToUnified）
- **AND** `request_build.go`（FromUnified）
- **AND** `response_parse.go`（响应解析）
- **AND** `response_format.go`（FormatUnified）
- **AND** `sync.go`（SyncModels）
- **AND** `capabilities.go`（协议能力声明）

---

### Requirement: 直通模式协议一致性校验

系统 SHALL 在直通模式（AccessMode=direct）下校验客户端入口协议与上游协议一致，禁止跨协议转换。

#### Scenario: 直通协议一致通过
- **WHEN** Key AccessMode=direct，客户端用 OpenAI 格式，Provider 支持 OpenAI
- **THEN** 路由成功，`upstreamProto == sourceProtocol`
- **AND** `callMethod = "direct"`

#### Scenario: 直通协议不一致拒绝
- **WHEN** Key AccessMode=direct，客户端用 OpenAI 格式，Provider 仅支持 Gemini
- **THEN** 返回 403 "direct API key does not support cross-protocol conversion"
- **AND** 不会降级为跨协议转换

---

### Requirement: Handler 通用循环消除协议分支

系统 SHALL 使用 `registry.All()` 遍历已注册协议，替代硬编码的 5 路 if-else/switch。

#### Scenario: 测试 Provider 时遍历所有协议
- **WHEN** 调用 `TestProvider(p, pm)`
- **THEN** 遍历 `registry.All()` 中所有协议描述符
- **AND** 对有端点配置的协议执行测试
- **AND** 无需为每个协议写 case 分支

#### Scenario: 新增协议自动被循环覆盖
- **WHEN** 新增协议 XYZ 并注册
- **THEN** 所有 `registry.All()` 循环自动包含 XYZ
- **AND** 无需修改 Handler 代码
