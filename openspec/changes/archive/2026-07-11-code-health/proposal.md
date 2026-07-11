# 代码健康：TypeScript baseUrl 弃用 & Go 冗余代码清理

## 动机

全面检查项目后发现以下问题：

- `web/tsconfig.app.json` 中 `baseUrl` 选项已被 TypeScript 标记为弃用，将在 TS 7.0 中停止工作
- `server/internal/handler/debug.go` 存在无格式化动词的 `fmt.Sprintf` 冗余调用
- `server/go.mod` 存在未被任何代码导入的 `go-openai` 间接依赖
- `server/bin/` 积压多个过时构建产物（共 ~240MB）
- `server/server` 根目录残留 43MB 二进制文件
- `web/node_modules` 缺失，前端从未构建

## 方案

| 问题 | 修复 |
|------|------|
| `baseUrl` 弃用 | 移除 `baseUrl`，`paths` 改为相对路径 `"./src/*"` |
| 冗余 `fmt.Sprintf` | 无格式动词的场景改用普通字符串字面量 |
| 未使用依赖 | `go mod tidy` 清理 `go-openai` |
| 过时构建产物 | 删除旧版二进制，只保留最新 |
| 根目录残留 | 删除 `server/server` |
| 前端缺依赖 | `npm install` + `npm run build` |

## 影响范围

- **前端**: `web/tsconfig.app.json` — 仅配置变更，无运行时影响
- **后端**: `server/internal/handler/debug.go` — curl 命令生成逻辑不变
- **依赖**: `server/go.mod` / `server/go.sum` — 仅移除未使用项
- **构建**: 前端重新构建，`server/res/web/` 静态资源更新
