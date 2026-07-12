# Phase 5 — 用户管理系统（RBAC）

## 动机

实现基于角色的访问控制（RBAC），支持：
1. 管理员可手动创建普通用户
2. 管理员授权普通用户可访问的模型厂商和模型映射
3. 普通用户只能管理自己的 API 密钥
4. 普通用户只能根据已授权厂商/模型创建密钥、查看数据
5. 调试界面仅允许普通用户测试自己创建的密钥
6. 模型厂商、模型映射、MCP 服务仅管理员可管理

## 方案

### 后端

#### 数据模型
- **Key 表新增 `user_id` 字段**：可为 NULL（管理员创建/旧数据），有值时关联用户
- **新增 `user_providers` 表**：用户-厂商授权（user_id + provider_id 唯一）
- **新增 `user_models` 表**：用户-模型映射授权（user_id + model_id 唯一）

#### Handler
- **user.go（新建）**：
  - `ListUsers` / `CreateUser` / `UpdateUser` / `DeleteUser` — CRUD
  - `GetUserPermissions` / `UpdateUserPermissions` — 权限管理（事务替换）
  - 辅助函数 `GetCurrentUserID()` / `IsAdmin()` / `GetUserProviderIDs()` / `GetUserModelIDs()`
- **key.go（大量修改）**：
  - 所有 30+ 方法添加 `checkKeyOwnership()` 校验
  - `Create` / `List` 按用户过滤
- **provider.go / model.go**：
  - `List` 按用户授权过滤，未授权返回空列表
- **debug.go**：
  - `TestKey` 非 admin 只能测试自己的 key
  - `TestProviders` 仅 admin 可调用

#### 中间件
- `RequireAuth` 新增 role 传递
- 新增 `RequireAdmin()` 中间件
- admin 路由组使用 `RequireAdmin` 保护

#### 路由
```
/admin/users                    GET     (admin)
/admin/users                    POST    (admin)
/admin/users/:id                PUT     (admin)
/admin/users/:id                DELETE  (admin)
/admin/users/:id/permissions    GET     (admin)
/admin/users/:id/permissions    PUT     (admin)
```

### 前端

#### 系统设置 → 用户管理（仅管理员可见）
- Tab 布局：SMTP | 用户管理
- 用户列表：ID / 用户名 / 显示名 / 角色标签 / 操作（编辑/权限/删除）
- 创建/编辑用户对话框：用户名 / 显示名 / 密码 / 启用开关
- 权限对话框：复选框选择可访问的厂商 + 模型映射

#### 侧边栏权限控制
- 模型厂商、模型映射、MCP 服务 → 仅管理员可见
- 路由守卫 `requiresAdmin` 保护直接 URL 访问

#### 调试页面
- "测试模型供应商" → 仅管理员可见
- "测试 API 密钥" → 仅允许测试自己的密钥（后端校验）

#### Store
- `useUserStore` 新增 `isAdmin` computed

### 关键细节
- 管理员用户不可删除
- 删除用户时级联清理 UserProvider / UserModel / 所有密钥
- 登录时 session 存储 role，前端根据 role 控制 UI
- 旧数据（user_id = NULL）视为管理员创建，不受限制
