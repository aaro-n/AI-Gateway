# SMTP 多项修复 + 重置链接日志 + 公开页面 401 修复

## 动机

1. **SMTP 配置热重载丢失**：`config.Reload()` 未包含 SMTP 字段，刷新后配置丢失。
2. **SMTPConfig yaml 标签缺失**：`UseTLS` 字段无 `yaml:"use_tls"` 标签，YAML 序列化/反序列化不匹配导致 `use_tls` 无法持久化。
3. **501 Invalid from address**：SMTP MAIL FROM 命令不接受 `"Name" <email>` 格式，需要提取纯邮箱地址。
4. **测试邮件 535 认证失败**：`GetSMTP` API 返回脱敏密码 `••••••••`，前端直接传给 `TestSMTP` 而不做占位符回退。
5. **忘记密码响应时间泄漏**：同步发送 SMTP 邮件导致已注册/未注册邮箱响应时间差异明显（3秒 vs 1ms），可被用户枚举攻击。
6. **公开页面 401 重定向**：`/reset-password` 和 `/forgot-password` 访问时 axios 拦截器强跳 `/login`。
7. **重置链接日志输出**：新增 `AG_SMTP_LOG_RESET_LINK` 环境变量，在 SMTP 不可用时管理员可从日志提取重置链接。

## 方案

### SMTP 修复
- `config/reloader.go`：Reload() 新增 7 个 SMTP 字段热重载
- `config/config.go`：SMTPConfig 所有字段添加 yaml 标签
- `email/smtp.go`：新增 `extractEmail()` 函数，Send() 中 envelope from 使用纯邮箱
- `handler/admin.go`：TestSMTP 新增占位符 `••••••••` 检测，回退到服务器已保存密码

### 安全加固
- `handler/auth.go`：ForgotPassword 改为 API 立即返回 + goroutine 异步发邮件
- `handler/auth.go`：新增 `AG_SMTP_LOG_RESET_LINK` 环境变量支持，开启后 log.Printf 输出重置链接

### 前端修复
- `web/src/api/index.ts`：401 拦截器白名单新增 `/forgot-password`、`/reset-password`
- `web/src/views/Settings/index.vue`：移除「保存 SMTP 设置」卡片，保留「发送测试邮件」

### 部署配置
- `docker-compose.yml`：新增 SMTP 环境变量 + `AG_SERVER_DOMAIN` + `AG_SMTP_LOG_RESET_LINK`
- `config.yaml.example`：新增 `log_reset_link` 字段文档 + `domain` 字段文档
