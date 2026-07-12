# AI Gateway Web 前端

基于 Vue 3 + TypeScript + Vite + Element Plus 的管理控制台。

## 页面结构

| 路由 | 页面 | 说明 |
|------|------|------|
| `/` | 仪表盘 | 请求量、Token 用量概览 |
| `/providers` | 模型厂商 | 管理 AI 厂商配置 |
| `/providers/:id` | 厂商详情 | 编辑厂商端点、模型列表 |
| `/models` | 模型映射 | 虚拟模型 → 实际模型路由 |
| `/models/:id` | 映射详情 | 编辑映射规则 |
| `/keys` | API 密钥 | 管理网关访问密钥 |
| `/keys/:id` | 密钥详情 | 模型/MCP 权限、用量统计 |
| `/mcps` | MCP 服务 | MCP 服务管理 |
| `/mcps/:id` | MCP 详情 | 编辑 MCP 服务配置 |
| `/model_usage` | 模型用量 | 按模型/密钥/时间的用量图表 |
| `/mcp_usage` | MCP 用量 | MCP 调用统计 |
| `/protocol_compare` | 协议对比 | 多协议响应格式对比 |
| `/debug` | 调试 | API 测试、日志查看、模型测试 |
| `/settings` | 系统设置 | SMTP 测试邮件、密码重置域名 |
| `/profile` | 个人设置 | 个人信息、偏好、安全 |
| `/login` | 登录 | 用户登录 |
| `/forgot-password` | 忘记密码 | 发送密码重置邮件 |
| `/reset-password` | 重置密码 | 通过 token 重置密码 |

## 技术栈

- **Vue 3** `<script setup>` Composition API
- **TypeScript** 严格模式
- **Vite** 构建工具
- **Element Plus** UI 组件库
- **Vue Router** 路由管理
- **Pinia** 状态管理
- **Axios** HTTP 客户端
- **Vue I18n** 国际化（中文/英文）
- **ECharts** 用量图表
- **@codemirror** 代码编辑器（调试页）

## 开发

```bash
# 安装依赖
npm install

# 启动开发服务器（默认 5173，自动代理到后端 18080）
npm run dev

# 类型检查
npx vue-tsc -b --noEmit

# 构建（输出到 ../server/res/web/）
npm run build
```

## 构建产物

前端构建输出到 `../server/res/web/`，Go 后端通过 `//go:embed` 嵌入二进制。

**必须按顺序构建：**

```bash
# 1. 构建前端（输出到 server/res/web/）
cd web && make build

# 2. 构建后端（嵌入前端资源）
cd ../server && go build -o server ./cmd/server/
```

只运行 `go build` 而不重建前端，二进制内嵌的将是上一次的旧前端。

## 目录结构

```
web/
├── public/                 # 静态资源
├── src/
│   ├── api/                # Axios 实例 + 拦截器
│   │   └── index.ts        # 401 处理、错误追踪
│   ├── assets/             # 图标、图片
│   ├── components/         # 全局组件
│   │   ├── CopyButton.vue  # 复制按钮
│   │   └── layout/         # 布局组件
│   ├── composables/        # 组合式函数（暗色模式、排序等）
│   ├── core/               # 核心模块
│   │   ├── apiErrorTracker.ts
│   │   └── collapsibleSidebar.ts
│   ├── locales/            # i18n 翻译文件
│   │   ├── en.ts
│   │   └── zh.ts
│   ├── plugins/            # 插件（i18n 初始化）
│   ├── router/             # 路由配置
│   ├── stores/             # Pinia stores
│   │   ├── user.ts         # 认证、用户信息、SMTP 操作
│   │   ├── keys.ts         # API 密钥
│   │   ├── models.ts       # 模型映射
│   │   ├── providers.ts    # 模型厂商
│   │   └── mcps.ts         # MCP 服务
│   ├── styles/             # 全局样式
│   ├── types/              # TypeScript 类型定义
│   ├── utils/              # 工具函数（格式化、时区）
│   └── views/              # 页面组件
│       ├── Dashboard/
│       ├── Debug/
│       ├── Keys/
│       ├── Login/          # 登录、忘记密码、重置密码
│       ├── MCPs/
│       ├── MCPUsage/
│       ├── ModelUsage/
│       ├── Models/
│       ├── Profile/        # 个人设置
│       ├── ProtocolCompare/
│       ├── Providers/
│       └── Settings/       # SMTP 测试邮件、域名配置
├── index.html
├── package.json
├── Makefile
├── tsconfig.json
└── vite.config.ts
```

## 环境变量

开发代理配置在 `vite.config.ts`：

```ts
server: {
  proxy: {
    '/api': 'http://localhost:18080',
    '/gateway': 'http://localhost:18080',
    '/mcp': 'http://localhost:18080'
  }
}
```

