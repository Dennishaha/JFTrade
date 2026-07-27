# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

JFTrade 是面向 Futu OpenD 的本地量化研发工作台（Go 后端 + Vue 3 控制台 + Wails v3 桌面壳 + Node PineTS worker）。仓库文档以中文为主，`docs/README.md` 是维护者导航，`docs/architecture.md` 是边界的事实来源。

## 环境要求

- Node `>=22.13`，pnpm 固定 `11.12.0`（`packageManager`），依赖统一走根 `pnpm-lock.yaml`：`pnpm install --frozen-lockfile`
- Go `1.26.5`；protobuf 生成额外要求本机 `protoc 34.1`
- pnpm workspace 只有两个包：`apps/web`（`@jftrade/web`）、`workers/pineworker`（`@jftrade/pineworker`）

## 常用命令

```bash
# 桌面联调（首选入口，免登录，含前端 + sidecar）
pnpm run desktop:dev

# 纯浏览器前端开发：两个终端
go run ./cmd/jftrade-api        # 后端 127.0.0.1:3000
pnpm run dev:web                # 前端 127.0.0.1:3003，代理 /api /swagger 到 3000

pnpm run dev:docs               # VitePress 文档站 127.0.0.1:3001
```

测试与检查：

```bash
pnpm run test:go                # go test ./... -count=1
pnpm run test:web               # vitest run（apps/web）
pnpm run test:pineworker
pnpm run typecheck              # web + pineworker
pnpm run lint:go                # golangci-lint v2.12.0
pnpm run vet:go
pnpm run check:arch-deps        # 分层依赖方向检查（需要 rg）
pnpm run test:coverage          # Go / Web / worker 三套覆盖率门禁
```

跑单个测试：

```bash
go test ./pkg/backtest -run TestPineworkerRunner -count=1 -v
go test ./internal/app/apiserver/servercore -run TestOpenAPISpecStable -count=1
pnpm --filter @jftrade/web run test BacktestPage           # vitest 按文件名过滤
pnpm --filter @jftrade/web run test:watch
```

分层门禁（`scripts/run-test-layer.mjs`，对应 CI 的 L0–L3）：

```bash
pnpm run test:preflight   # 提交前：test-policy/test-names/lint/vet/coverage/typecheck/arch-deps（别名 test:pr）
pnpm run test:ci-local    # 单机可跑的 Linux CI 核心门禁（含契约漂移检查、desktop 脚本测试、PineTS 合规）
pnpm run test:main        # ci-local + 完整 Go 回归 + desktop + 真实 PineTS smoke
```

增量覆盖率需要 base ref（CI 自动注入）：`JFTRADE_DIFF_BASE=origin/main pnpm run test:coverage`

## 生成产物（不要手工改）

`pnpm run generate:contracts` 串起：`go generate ./cmd/jftrade-api`（Swagger 注释 → `docs/swagger/*`）→ `scripts/generate-api-types.mjs`（→ `apps/web/src/generated/openapi.ts`）→ 以 `UPDATE_OPENAPI_SNAPSHOT=1` 刷新 `tests/fixtures/openapi-baseline.json`。`pnpm run generate:reference` 生成 `docs/reference/generated/*`，`pnpm run generate:docs` = 两者串联。

改任何公开 HTTP 契约后必须重跑 `pnpm run generate:docs`，否则 CI 的 `git diff --exit-code` 契约漂移检查会失败。

其余生成物：`pnpm run generate:wails-bindings` → `apps/web/src/wails/*`（`check:wails-bindings` 校验新鲜度）；`go run ./cmd/generate-futu-proto -source <FTAPIProtoFiles>` 与 `go run ./cmd/generate-pineworker-proto` → `pkg/futu/pb/*` 等。`.gitattributes` 把这些路径钉成 LF，Windows 上也要保持一致。

## 架构要点

**单一后端，两个进程入口。** `cmd/jftrade-api`（独立 API）和 `cmd/jftrade-desktop`（Wails v3）都装配同一个 `internal/app/apiserver`，不存在第二套业务 API。桌面壳不替换 transport：Vue 仍直接走 REST/SSE/WebSocket，bindings 只承载外部链接、桌面日志、更新检查。控制台只承诺 `/api/v1/*`；bbgo 原生 `/api/*` 不是本项目运行模式的一部分。

**后端分层（`docs/architecture/backend-coding-standards.md`，由 `scripts/check-arch-deps.sh` 强制）：**

- `internal/api/*`：只做参数绑定/校验、调 service、错误映射、DTO→JSON。禁止碰 `internal/store/*`、SQLite、`internal/integration/*`、Futu protobuf，禁止持有后台任务或跨步骤编排。
- `internal/{system,settings,marketdata,trading,strategy,backtest,assistant,watchlist}`：业务规则与状态机。禁止 import Gin / `net/http` handler 类型 / `internal/api/*`，禁止 import 具体 DB 驱动或 Futu protobuf。接口优先定义在使用方包内。
- `internal/store/*`：schema、codec、query、migration，只做存储模型 ↔ 业务模型转换。
- `internal/integration/*`：外部 SDK/协议封装（`internal/integration/futu` 是 sidecar 内部的 OpenD 适配层），不得持有调用方业务状态（demand、freshness、runtime lifecycle 属于 service）。
- `internal/app/apiserver`：`lifecycle`（sidecar 生命周期）、`runtime`（路径/环境变量/OpenD 注入）、`application`（应用资源序列）、`stores`（持久化资源句柄）、`runtimes`（运行时句柄与关闭顺序）、`futuapp`（Futu 应用编排与投影）、`servercore`（HTTP/security/frontend shell + 路由装配，是持续收口区，新逻辑别往这里堆）。
- `internal/assistant/assembly`：Assistant Runtime、ADK/MCP 生命周期和跨域工具投影；可依赖业务 service 的公开类型，但禁止反向依赖 `internal/app`、具体 store、integration 或 HTTP transport。
- `pkg/*` 只放需要被外部 module 复用的稳定能力（`pkg/futu` 实现 bbgo `types.Exchange`，同时服务 sidecar、策略 runtime 和回测——改动前先判断影响面）。

**策略/回测执行边界。** 主路径是 `sourceFormat=pine-v6` + `runtime=pine-pinets`：前端生成 Pine，Go（`pkg/strategy/{pine,pinespec,ir,pineworker}`）解析并规划，交由 Node ESM `worker.mjs`（`workers/pineworker`，固定 `pinets@0.9.29`）通过 localhost gRPC worker pool 执行。**PineTS 只产出信号、图形输出和 order intents；撮合、成交、资金曲线、风控、账户刷新和券商下单全部在 Go 侧**（`pkg/backtest`，撮合模型见 `docs/backtest-execution-model.md`）。Go 主进程不再维护自研 Pine 执行 runtime。

**实时行情链路：** `apps/web` → SSE `/api/v1/stream/live` 或 WS `/api/v1/ws/live` → `internal/api/live` → `internal/marketdata`（demand / tick cache / freshness / fallback polling / backoff 都归它）→ `internal/integration/futu` → `pkg/futu` → OpenD。

**自选：** `watchlists.db` 是唯一主数据。Futu 3213/3222 只做远端分组发现与预览导入，3203 `SecuritySnapshot` 只做可见行报价；自选行情不进入实时 collector demand 或 BasicQot 订阅。

**前端：** `apps/web/src` 下 `pages/` + `components/` + `composables/` + `features/`（纯逻辑模块，如 `strategyVisualBuilder*`、`pineSourceStructure*`）+ `charting/`（lightweight-charts）；Vuetify 4 + Tailwind 4 + `@tanstack/vue-query`；测试在 `apps/web/tests/`。

## 硬性约束

- **Lint**：`funlen` 限制函数 80 行 / 60 语句（测试文件豁免）；revive `filename-format` 约束 Go 文件名。CI 另跑 revive 文件长度门禁：生产 Go 文件 ≤ 800 行，测试 Go 文件 ≤ 1200 行（`.revive-file-length-{prod,test}.toml`）。
- **测试文件命名**：不得使用 `coverage_98`、`c95` 一类覆盖率数字命名，文件名要描述被验证的业务行为；`pnpm run check:test-names` 只检查相对 base 新增的文件。
- **覆盖率门禁**（`docs/testing-strategy.md`）：Go 业务总量 ≥90%、普通 package ≥85%、关键域 ≥95%；Web statements/lines ≥90%、branches/functions ≥85%，关键 Web 域 95/90。关键域 = 交易与订单、实盘行情、Futu/OpenD、回测与策略执行、安全认证、SQLite schema/migration。
- **真实外部依赖**：真实 Futu/OpenD 只在手动 `futu-live.yml`（`JFTRADE_FUTU_LIVE_TEST=1`，self-hosted runner）中跑；普通测试用 mock server 或协议 fixture，不得以 `skip` 充当验证结论。
- 接入 Futu 要求 OpenD `>= 10.9.6908`，低版本会被拒绝建立业务会话。

## 运行时与端口

配置优先级：环境变量 > `settings.json` > 内置默认值。开发态（浏览器开发、`cmd/jftrade-api`、`JFTrade Dev`）运行时文件落在仓库内 `var/jftrade-api/`（`settings.json`、`backtest.db`、`watchlists.db`）。正式 Wails 产品使用系统用户数据目录（macOS `~/Library/Application Support/JFTrade`、Windows `%LOCALAPPDATA%/JFTrade`、Linux `${XDG_DATA_HOME:-~/.local/share}/jftrade`），不迁移开发数据；开发版与产品版 Product/SingleInstance ID 不同，可同时运行。

| 端口 | 用途 |
| --- | --- |
| 3003 / 3000 / 3001 | 前端 dev server / 开发态后端 / 文档 dev server |
| 3008 / 6699 | `JFTrade Dev` sidecar / 正式产品 sidecar（仅 loopback，非浏览器入口） |
| 6688 | 可选 Web 入口，默认关闭，需在“设置 → Web 访问”设密码后主动开启 |
| 11110 / 11111 | Futu OpenD API / WebSocket |

## 发布

`./build-release.sh`（或 `build-release.ps1`）构建 `cmd/jftrade-api` 并把前端与文档站放进 `dist/`；版本按 `JFTRADE_VERSION` → `git describe` → `dev` 解析。Wails 正式产品以 `vX.Y.Z` tag 为唯一版本源，`dev`/`v0.0.0` 禁止进入桌面 release：`JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3 pnpm run desktop:release:darwin`（macOS 只发 ARM64 无签名 DMG）。推 tag 触发 `desktop-release.yml`。详见 `docs/troubleshooting/desktop-release.md`。

## 许可证

AGPL-3.0-only。`pnpm run check:oss-license` 会逐字节校验 `LICENSE`；新增依赖需过 `check:pinets-license` / `check:pinets-compliance` 与 `docs/legal/third-party-notices.md`。
