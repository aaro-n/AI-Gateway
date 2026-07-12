# 任务清单

## 已完成

### 后端 — 数据模型
- [x] `model.Key` 新增 `UserID *uint` 字段（gorm index）
- [x] 新建 `model.UserProvider` struct + AutoMigrate
- [x] 新建 `model.UserModel` struct + AutoMigrate

### 后端 — Handler
- [x] 新建 `handler/user.go`：ListUsers / CreateUser / UpdateUser / DeleteUser / GetUserPermissions / UpdateUserPermissions
- [x] 新建辅助函数：GetCurrentUserID / IsAdmin / GetUserProviderIDs / GetUserModelIDs
- [x] 修改 `handler/key.go`：所有 30+ 方法添加所有权校验
- [x] 修改 `handler/provider.go`：List 按用户权限过滤
- [x] 修改 `handler/model.go`：List 按用户权限过滤
- [x] 修改 `handler/debug.go`：TestKey 校验所有权，TestProviders 仅 admin

### 后端 — 中间件
- [x] 修改 `middleware/auth.go`：RequireAuth 传递 role，新增 RequireAdmin
- [x] 修改 `cmd/server/main.go`：admin 路由组 + /admin/users 路由

### 前端 — 用户管理页面
- [x] `stores/user.ts` 新增 `isAdmin` computed
- [x] `views/Settings/index.vue` 新增用户管理 Tab
  - 用户列表表格
  - 创建/编辑用户对话框
  - 权限编辑对话框（厂商 + 模型映射复选框）
- [x] 页面宽度从 760px 增加到 960px

### 前端 — 权限过滤
- [x] `components/layout/MainLayout.vue`：侧边栏隐藏 admin 菜单项（v-if="isAdmin"）
- [x] `router/index.ts`：路由守卫 requiresAdmin 保护 /providers /models /mcps
- [x] `views/Debug/index.vue`：测试厂商区域仅管理员可见
- [x] `views/Debug/index.vue`：导入 isAdmin / useUserStore

### 构建
- [x] `cd web && make build` → 前端构建到 server/res/web/
- [x] `go build -o server ./cmd/server/` → 后端编译
- [x] 全栈构建验证通过

## 构建注意事项

Go 使用 `//go:embed` 嵌入 `res/web/`，构建顺序：
```bash
cd web && make build    # → ../server/res/web/
cd ../server && go build -o server ./cmd/server/
```

不要使用 `npm run build`（不含 --outDir）否则输出到 web/dist/ 不会被嵌入。
