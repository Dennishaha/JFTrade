# JFTrade

JFTrade 是一个面向 Futu OpenD 的交易研发控制台。它把行情查看、历史数据同步、策略编写、回测、运行时设置和 ADK 助手放在同一个本地工作台里。

前端与构建工具要求 Node.js `>=22.13` 和仓库固定的 pnpm `11.12.0`；依赖安装统一使用根目录 `pnpm-lock.yaml`。

## 快速开始

日常使用和桌面联调优先启动 `JFTrade Dev`。它保持桌面免登录，也提供配置可选 Web 访问的可信设置入口：

```bash
pnpm install --frozen-lockfile
pnpm run desktop:dev
```

只有进行纯浏览器前端开发时，才需要另外开两个终端。先在 `JFTrade Dev` 的“设置 → Web 访问”中设置密码并主动开启；独立 API 默认不会开放浏览器控制台。

终端 1：启动后端服务：

```bash
go run ./cmd/jftrade-api
```

终端 2：启动前端：

```bash
pnpm install --frozen-lockfile
pnpm run dev:web
```

Web 已开启后可打开这些地址：

- 控制台：`http://127.0.0.1:3003/`
- 文档：`http://127.0.0.1:3003/docs/`
- Swagger UI：`http://127.0.0.1:3000/swagger/`
- Swagger JSON：`http://127.0.0.1:3000/swagger/doc.json`

前端开发服务器在 `3003`，会把 `/api` 和 `/swagger` 转发到后端 `3000`。`cmd/jftrade-api` 是独立 API 入口；Wails 产品由 `cmd/jftrade-desktop` 启动同一套应用服务。

## 常用入口

只看文档站：

```bash
pnpm run generate:docs
pnpm run dev:docs
```

VitePress 文档站默认在 `http://127.0.0.1:3001/`。

本地按可选 Web 发布包的方式验收：

```bash
./start.sh
```

Windows:

```cmd
start.cmd
```

Web 已在 `JFTrade Dev` 中开启后的入口：

- 前端 + API：`http://127.0.0.1:6688/`

生成发布产物：

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

发布脚本会构建 `cmd/jftrade-api`，并把前端静态资源和文档站一起放进 `dist/`。

Wails 正式产品使用 `vX.Y.Z` tag 作为唯一版本源。macOS 只发布 Apple Silicon ARM64 无签名 DMG：

```bash
JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3 pnpm run desktop:release:darwin
```

Windows x64 无签名 NSIS 安装器使用 `pnpm run desktop:release:windows`；tag CI 还会在原生 ARM64 runner 上生成 Windows ARM64 preview 无签名 NSIS 安装器。完整发布约束见 [桌面发布与通道隔离](docs/troubleshooting/desktop-release.md)。

推送正式桌面 tag 会自动触发 GitHub Actions，在全部平台构建通过后创建或更新同名 GitHub Release，并上传可下载二进制、SBOM 和 `SHA256SUMS`：

```bash
git tag v1.2.3
git push origin v1.2.3
```

## 常用命令

```bash
go test ./...
pnpm run test:web
pnpm run typecheck:web
pnpm run build:web
pnpm run build:docs
```

文档和接口说明生成：

```bash
pnpm run generate:openapi
pnpm run generate:reference
pnpm run generate:docs
```

其中：

- `generate:openapi` 从 `cmd/jftrade-api` 扫描 Swagger 注释，生成 `docs/swagger/*`
- `generate:reference` 生成 `docs/reference/generated/*`
- `generate:docs` 顺序执行上面两步

`docs/swagger/*` 和 `docs/reference/generated/*` 是生成产物，不要手工改。

Protobuf Go 代码生成使用跨平台 Go 命令，并要求本机安装 `protoc 34.1`：

```bash
go run ./cmd/generate-futu-proto -source /path/to/FTAPIProtoFiles_10.5.6508
go run ./cmd/generate-pineworker-proto
```

命令会校验并按需安装固定版本的 Go Protobuf 插件；生成失败时不会替换仓库内已有产物。

## 运行时文件

浏览器开发、独立 API 入口和 `JFTrade Dev` 默认把运行时文件放在仓库内的 `var/jftrade-api/`。首次启动后通常会看到：

- `settings.json`：本地设置
- `backtest.db`：回测和历史数据相关存储
- `watchlists.db`：本地多分组自选、券商导入绑定与审计

配置优先级是：环境变量 > `settings.json` > 内置默认值。配置细节见 [docs/configuration.md](docs/configuration.md)，端口和启动排障见 [docs/troubleshooting/startup-ports.md](docs/troubleshooting/startup-ports.md)。

正式 Wails 产品不迁移开发数据，也不在安装目录写入业务数据，默认使用系统用户数据目录：

- macOS：`~/Library/Application Support/JFTrade`
- Windows：`%LOCALAPPDATA%/JFTrade`
- Linux：`${XDG_DATA_HOME:-~/.local/share}/jftrade`

常见端口：

- `127.0.0.1:3003`：前端开发服务器
- `127.0.0.1:3000`：开发态后端服务
- `127.0.0.1:3001`：文档开发服务器
- `127.0.0.1:3008`：`JFTrade Dev` 桌面 sidecar
- `127.0.0.1:6688`：默认的可选 Web 入口；端口可在设置中修改，桌面 Web 关闭时不创建该监听器
- `127.0.0.1:6699`：正式 Wails 产品 sidecar；始终仅限本机桌面 WebView，不作为浏览器入口
- `127.0.0.1:11110`：Futu OpenD API
- `127.0.0.1:11111`：Futu OpenD WebSocket

## 接下来读什么

- 想确认当前版本状态、发布形态和验收基线：读 [docs/README.md](docs/README.md)
- 想快速使用控制台：读 [docs/quick-start.md](docs/quick-start.md)
- 想改启动、端口或可选 Web 访问：读 [docs/configuration.md](docs/configuration.md)
- 想构建或排查 Wails 桌面产品：读 [docs/troubleshooting/desktop-release.md](docs/troubleshooting/desktop-release.md)
- 启动失败、OpenD 连不上、实时行情异常：读 [docs/troubleshooting.md](docs/troubleshooting.md)
- 想理解模块边界和数据流：读 [docs/architecture.md](docs/architecture.md)
- 想管理或扩展自选、Futu 分组导入和自选快照：读 [docs/watchlist.md](docs/watchlist.md)
- 想改 ADK 助手、agent、approval 或工具权限：读 [docs/adk.md](docs/adk.md)
- 想查 Futu/OpenD、bbgo 或生成接口参考：读 [docs/reference/README.md](docs/reference/README.md)

## 目录导览

```text
cmd/jftrade-api/           后端入口
cmd/jftrade-desktop/       Wails v3 桌面入口与桌面专属服务
internal/app/apiserver/    后端启动、装配、运行时目录
internal/desktop/          正式产品系统用户数据目录解析
internal/api/              /api/v1/*、SSE、WebSocket
internal/{system,settings,marketdata,trading,strategy,backtest,assistant,watchlist}/
                           控制台业务能力
internal/integration/futu/ 后端内部的 Futu/OpenD 集成
pkg/futu/                  Futu exchange 适配层
pkg/strategy/              Pine 和策略运行能力
pkg/backtest/              回测与历史数据存储
apps/web/                  Vue 3 控制台
docs/                      用户文档、维护者导航与参考资料
scripts/                   文档生成、打包和辅助脚本
build/                     Wails 配置、平台 Taskfile 与桌面资源
```
