# JFTrade

JFTrade 当前的默认心智不是单一的 bbgo 插件仓库，而是一个以前端控制台和 API sidecar 为主、以 bbgo 运行时为扩展路径的交易研发工作台。

仓库里同时保留两条运行路径：

- `cmd/jftrade-api` / `cmd/jftrade api`：启动 JFTrade sidecar，提供控制台默认使用的 `/api/v1/*`、SSE、WS、Swagger 和运行时设置。
- `cmd/jftrade run --config ./config/jftrade.yaml`：在 sidecar 之外进入 bbgo runtime，用于策略执行、交易运行时和 bbgo 扩展点集成。

系统边界和数据流以 [docs/architecture.md](docs/architecture.md) 为准；本 README 只负责告诉仓库协作者“项目现在怎么跑”。

## 常用启动方式

### 开发态：前端 + sidecar

前端开发服务器默认在 `127.0.0.1:5173`，会把 `/api`、`/swagger` 代理到 `127.0.0.1:3000`，并把 `/docs` 代理到 VitePress 文档站 `127.0.0.1:3001`。

```bash
npm install
go run ./cmd/jftrade api
npm run dev:web
```

常用入口：

- 控制台：`http://127.0.0.1:5173/`
- 用户文档：`http://127.0.0.1:5173/docs/`
- Swagger UI：`http://127.0.0.1:3000/swagger/`
- Swagger JSON：`http://127.0.0.1:3000/swagger/doc.json`

### 开发态：只看文档站

```bash
npm install
npm run generate:docs
npm run dev:docs
```

VitePress 开发站默认在 `http://127.0.0.1:3001/`。

### 本地一键验收

`start.sh` / `start.cmd` 会安装依赖、生成 Swagger、执行前端类型检查和构建，然后以 release-style 端口启动内嵌前端与 sidecar。

```bash
./start.sh
```

默认入口：

- GUI：`http://127.0.0.1:6688/`
- API gateway：`http://127.0.0.1:6699/`

### 完整 bbgo 运行时

```bash
go run ./cmd/jftrade run --config ./config/jftrade.yaml
```

这条路径用于策略运行和交易链路，不应与“前端默认后端就是 bbgo 原生 `/api/*`”混为一谈。控制台默认仍然面向 JFTrade sidecar 的 `/api/v1/*`。

### 发布构建

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

当前发布脚本构建的是 `cmd/jftrade-api` 的 API-only 发行版，并把前端静态资源和文档站一起打入发布产物。生成产物位于 `dist/`。

## 文档与生成链路

- `npm run generate:openapi`：从 `cmd/jftrade-api` 的 Swagger 扫描入口重新生成 `docs/swagger/*`
- `npm run generate:reference`：从现有契约和 Swagger 生成 `docs/reference/generated/*`
- `npm run generate:docs`：顺序执行上面两步
- `npm run build:docs`：生成 VitePress 静态文档站

以下内容是生成产物，不应手工维护：

- `docs/swagger/*`
- `docs/reference/generated/*`

## 运行时与配置

默认运行时目录是 `var/jftrade-api/`。首次启动后会生成：

- `settings.json`
- `backtest.db`
- `secrets/admin.key`

常见端口约定：

- `127.0.0.1:5173`：前端开发服务器
- `127.0.0.1:3000`：开发态 sidecar
- `127.0.0.1:3001`：VitePress 开发站
- `127.0.0.1:6688`：发布态 GUI / 同源 docs / Swagger
- `127.0.0.1:6699`：发布态 API gateway
- `127.0.0.1:11110`：Futu OpenD API
- `127.0.0.1:11111`：Futu OpenD WebSocket

启动优先级为：环境变量 > `settings.json` > 内置默认值。配置细节见 [docs/configuration.md](docs/configuration.md)，启动与端口分诊见 [docs/troubleshooting/startup-ports.md](docs/troubleshooting/startup-ports.md)。

## 常用验证命令

```bash
go test ./...
npm run typecheck:web
npm run build:web
npm run build:docs
```

## 先读哪些文档

开发者和维护者建议按下面顺序取上下文：

1. [docs/README.md](docs/README.md)：维护者文档导航
2. [docs/architecture.md](docs/architecture.md)：当前系统架构与职责边界
3. [docs/troubleshooting.md](docs/troubleshooting.md)：排障入口
4. [docs/adk.md](docs/adk.md)：ADK / Agent 控制面
5. [docs/reference/README.md](docs/reference/README.md)：协议和上游参考资料

## 关键目录

```text
cmd/jftrade-api/          API-only 发布入口
cmd/jftrade/              sidecar + 可选 bbgo runtime 入口
internal/app/apiserver/   sidecar 生命周期、装配、运行时目录
internal/api/             /api/v1/* transport
internal/{system,settings,marketdata,trading,strategy,backtest,assistant}/
                          业务 service
internal/integration/futu/ sidecar 内部 Futu/OpenD 集成
pkg/futu/                 共享 Futu exchange 适配层
pkg/strategy/             Pine / strategy runtime 能力
pkg/backtest/             回测与历史数据存储能力
apps/web/                 Vue 3 控制台
docs/                     用户文档、维护者导航与参考资料
scripts/                  文档生成、打包和辅助脚本
```
