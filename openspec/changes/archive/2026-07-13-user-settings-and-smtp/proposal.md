# 个人设置页面 + SMTP 配置页

## 动机

1. **用户管理界面重构**：原有的 /settings 页面包含个人信息、偏好设置、账号安全三个卡片，现在将这些移动到独立页面 `/profile`，通过右上角用户名下拉访问。

2. **SMTP 邮件配置**：为忘记密码功能提供邮件服务配置界面，支持 SMTP 参数设置和测试邮件发送。

3. **密码重置域名**：支持通过 AG_SERVER_DOMAIN 环境变量设置重置链接的服务器域名，未设置时自动使用浏览器当前域名作为保底。

4. **Phase 3 - 用户体验优化**：
   - 用户设置不再使用弹窗，直接跳转到 `/profile` 路由
   - SMTP 设置拆分为「保存设置」和「发送测试邮件」两个独立卡片
   - 测试邮件允许自定义收件人、主题和正文
   - SMTP 设置保存到 config.yaml，无需重启即可生效

## 方案

### 前端架构
- **Profile/index.vue**（新建）：独立页面，包含三个卡片（个人信息/偏好设置/账号安全）
- **MainLayout.vue**：用户名下拉"用户设置"点击跳转到 `/profile`
- **Settings/index.vue**（重写）：三卡片布局
  - 卡片1：保存 SMTP 设置（表单 + 保存按钮）
  - 卡片2：发送测试邮件（收件人/主题/正文 + 发送按钮）
  - 卡片3：密码重置链接域名展示
- **user store**：新增 fetchSMTPConfig()、saveSMTPConfig()、testSMTP() 方法
- **i18n**：zh.ts / en.ts 新增 SMTP 保存/测试分离相关键

### 后端
- **config.go**：ServerConfig 新增 Domain 字段，AG_SERVER_DOMAIN 环境变量
- **admin.go**：新增 GetSMTP（GET /admin/smtp）、SaveSMTP（POST /admin/smtp）、TestSMTP（POST /admin/smtp/test）
  - SaveSMTP 写入 config.yaml（yaml.v3 marshal/unmarshal）
  - 密码占位符 `••••••••` 保留原有密码
  - TestSMTP 接受 `to`/`subject`/`body` 参数，从 config.yaml 读取 SMTP 配置后发送
- **smtp.go**：新增 SendCustom() 用于发送自定义内容邮件
- **auth.go**：getFrontendURL() 优先使用 Domain 配置
- **main.go**：注册 /admin/smtp GET+POST、/admin/smtp/test POST 路由

### 关键细节
- Settings 页保留 `/settings` 路由不变，侧边栏 `menu.settings` = "系统设置"
- 个人设置通过用户名下拉 → `/profile` 独立页面访问
- SMTP 测试邮件默认收件人为当前用户邮箱，主题和正文可自定义
- `getFrontendURL()` 优先级：AG_SERVER_DOMAIN > 浏览器 Origin
- SMTP 保存立即写入 config.yaml，但邮件客户端需重新 Init 生效
