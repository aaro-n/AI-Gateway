## 1. 修复 TypeScript baseUrl 弃用

- [x] 修改 `web/tsconfig.app.json`，移除 `baseUrl`，`paths` 改用 `"./src/*"`
- [x] TypeScript 编译通过（`vue-tsc -b` 无错误）

## 2. 修复 Go 冗余代码

- [x] `debug.go` 第 648 行：`fmt.Sprintf(" \\\n  -H \"anthropic-version: 2023-06-01\"")` → 普通字符串
- [x] 编译通过

## 3. 清理依赖

- [x] `go mod tidy` 移除未使用的 `github.com/sashabaranov/go-openai`
- [x] `go build` 成功

## 4. 清理构建产物

- [x] 删除 `server/bin/ai-gateway-server-6052b14`
- [x] 删除 `server/bin/ai-gateway-server-c738404`
- [x] 删除 `server/server`（根目录残留）
- [x] 只保留 `server/bin/ai-gateway-server`

## 5. 前端构建

- [x] `npm install` — 恢复 node_modules
- [x] `npm run build` — Vite 构建成功（15s）
- [x] 复制 `web/dist/*` → `server/res/web/`
- [x] 后端编译嵌入最新前端资源

## 6. 推送到远程

- [x] Git commit `a2dc41c`
- [x] Push to `github.com:aaro-n/AI-Gateway.git` master
