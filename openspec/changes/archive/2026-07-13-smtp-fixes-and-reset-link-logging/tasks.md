# Tasks

## 已完成

- [x] `config/reloader.go` — Reload() 新增 SMTP 全部字段热重载
- [x] `config/config.go` — SMTPConfig 添加 yaml 标签 (enabled, host, port, username, password, from, use_tls, log_reset_link)
- [x] `email/smtp.go` — extractEmail() 提取纯邮箱地址用于 MAIL FROM
- [x] `handler/admin.go` — TestSMTP 密码占位符回退逻辑
- [x] `handler/auth.go` — ForgotPassword 异步发邮件 + LogResetLink 日志输出
- [x] `web/src/api/index.ts` — 401 拦截器白名单加入 /forgot-password, /reset-password
- [x] `web/src/views/Settings/index.vue` — 移除保存 SMTP 卡片、saveLoading、handleSaveSMTP
- [x] `docker-compose.yml` — 新增 SMTP 8 个环境变量 + AG_SERVER_DOMAIN
- [x] `config.yaml.example` — 新增 log_reset_link 字段文档
- [x] 前端重建 (make build) + Go 二进制重建 (go build)

## 验证

- [x] SMTP 测试邮件发送成功（占位符密码回退正常）
- [x] 忘记密码 API 响应时间 < 20ms（异步发送）
- [x] 重置密码页面不再跳转 /login
- [x] LogResetLink 开启后终端可见重置链接
