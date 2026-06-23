# JFTrade

JFTrade 是一个面向 Futu OpenD 的交易研发控制台。它把行情查看、历史数据同步、策略编写、回测、运行时设置和 ADK 助手放在同一个本地工作台里。

## 快速开始

开发时通常开两个终端。

终端 1：启动后端服务：

```bash
go run ./cmd/jftrade-api
```

终端 2：启动前端：

```bash
npm install
npm run dev:web
```

打开这些地址：

- 控制台：`http://127.0.0.1:5173/`
- 文档：`http://127.0.0.1:5173/docs/`
- Swagger UI：`http://127.0.0.1:3000/swagger/`
- Swagger JSON：`http://127.0.0.1:3000/swagger/doc.json`

前端开发服务器在 `5173`，会把 `/api` 和 `/swagger` 转发到后端 `3000`。当前仓库只保留 `cmd/jftrade-api` 这一条后端入口。

## 常用入口

只看文档站：

```bash
npm run generate:docs
npm run dev:docs
```

VitePress 文档站默认在 `http://127.0.0.1:3001/`。

本地按接近发布包的方式验收：

```bash
./start.sh
```

Windows:

```cmd
start.cmd
```

默认入口：

- GUI：`http://127.0.0.1:6688/`
- API gateway：`http://127.0.0.1:6699/`

生成发布产物：

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

发布脚本会构建 `cmd/jftrade-api`，并把前端静态资源和文档站一起放进 `dist/`。

## 常用命令

```bash
go test ./...
npm run test:web
npm run typecheck:web
npm run build:web
npm run build:docs
```

文档和接口说明生成：

```bash
npm run generate:openapi
npm run generate:reference
npm run generate:docs
```

其中：

- `generate:openapi` 从 `cmd/jftrade-api` 扫描 Swagger 注释，生成 `docs/swagger/*`
- `generate:reference` 生成 `docs/reference/generated/*`
- `generate:docs` 顺序执行上面两步

`docs/swagger/*` 和 `docs/reference/generated/*` 是生成产物，不要手工改。

## 运行时文件

后端默认把运行时文件放在 `var/jftrade-api/`。首次启动后通常会看到：

- `settings.json`：本地设置
- `backtest.db`：回测和历史数据相关存储
- `secrets/admin.key`：本地管理密钥

配置优先级是：环境变量 > `settings.json` > 内置默认值。配置细节见 [docs/configuration.md](docs/configuration.md)，端口和启动排障见 [docs/troubleshooting/startup-ports.md](docs/troubleshooting/startup-ports.md)。

常见端口：

- `127.0.0.1:5173`：前端开发服务器
- `127.0.0.1:3000`：开发态后端服务
- `127.0.0.1:3001`：文档开发服务器
- `127.0.0.1:6688`：发布态 GUI、文档和 Swagger 的同源入口
- `127.0.0.1:6699`：发布态 API gateway
- `127.0.0.1:11110`：Futu OpenD API
- `127.0.0.1:11111`：Futu OpenD WebSocket

## 接下来读什么

- 想快速使用控制台：读 [docs/quick-start.md](docs/quick-start.md)
- 想改启动、端口、配置或管理密钥：读 [docs/configuration.md](docs/configuration.md)
- 启动失败、OpenD 连不上、实时行情异常：读 [docs/troubleshooting.md](docs/troubleshooting.md)
- 想理解模块边界和数据流：读 [docs/architecture.md](docs/architecture.md)
- 想改 ADK 助手、agent、approval 或工具权限：读 [docs/adk.md](docs/adk.md)
- 想查 Futu/OpenD、bbgo 或生成接口参考：读 [docs/reference/README.md](docs/reference/README.md)

## 目录导览

```text
cmd/jftrade-api/           后端入口
internal/app/apiserver/    后端启动、装配、运行时目录
internal/api/              /api/v1/*、SSE、WebSocket
internal/{system,settings,marketdata,trading,strategy,backtest,assistant}/
                           控制台业务能力
internal/integration/futu/ 后端内部的 Futu/OpenD 集成
pkg/futu/                  Futu exchange 适配层
pkg/strategy/              Pine 和策略运行能力
pkg/backtest/              回测与历史数据存储
apps/web/                  Vue 3 控制台
docs/                      用户文档、维护者导航与参考资料
scripts/                   文档生成、打包和辅助脚本
```
