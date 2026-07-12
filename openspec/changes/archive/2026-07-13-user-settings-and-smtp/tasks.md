# 任务清单

## 已完成

### 后端
- [x] ServerConfig 新增 Domain 字段（AG_SERVER_DOMAIN 环境变量）
- [x] AdminHandler 新增 GetSMTP、SaveSMTP、TestSMTP handler
- [x] SaveSMTP: POST /admin/smtp → 写入 config.yaml（yaml.v3）
- [x] TestSMTP: POST /admin/smtp/test → 接受 to/subject/body 参数
- [x] email 包新增 InitDirect()、SendTest()、SendCustom()
- [x] getFrontendURL() 优先使用 cfg.Server.Domain
- [x] main.go 注册 /admin/smtp GET+POST、/admin/smtp/test POST 路由
- [x] 修复 admin.go 中未使用的 `client` 变量

### 前端
- [x] ProfileSettingsDialog.vue → Profile/index.vue（独立页面，/profile 路由）
- [x] MainLayout.vue 用户名下拉"用户设置"改为 router.push('/profile')
- [x] Settings/index.vue 拆分为三卡片布局
  - 卡片1：保存 SMTP 设置（表单 + 保存按钮 → POST /admin/smtp）
  - 卡片2：发送测试邮件（收件人/主题/正文 + 发送按钮）
  - 卡片3：密码重置链接域名展示
- [x] user store 新增 saveSMTPConfig()，testSMTP() 参数改为 { to, subject, body }
- [x] zh.ts 和 en.ts 新增 SMTP 保存/测试分离 i18n 键
- [x] 注册 /profile 路由

### 删除的文件/引用
- [x] MainLayout 移除 ProfileSettingsDialog 导入和模板引用
- [x] MainLayout 移除 showProfileSettings ref（ref 导入也一并移除）

### 构建
- [x] 使用 `make build`（非 `npm run build`）正确输出到 server/res/web/
- [x] go build 嵌入前端资产
- [x] 重启服务验证

## 构建注意事项

Go 使用 `//go:embed` 嵌入 `res/web/`，构建顺序：
```bash
cd web && make build    # → ../server/res/web/
cd ../server && go build -o server ./cmd/server/
```

不要使用 `npm run build`（不含 --outDir）否则输出到 web/dist/ 不会被嵌入。
