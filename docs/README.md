# JFTrade 维护者文档导航

这份 README 面向仓库维护者、协作者和后续 AI。它不重复介绍项目本身，只负责把你引到正确的事实来源。

如果只看一篇，请先看本文和 [architecture.md](architecture.md)。

## 当前版本快照

更新时间：2026-07-04。当前代码基线为 `704a02e`，提交说明是 `Support Pine live percent sizing and cancel commands`。

JFTrade 当前是 **Futu-first 的本地量化策略研发与半自动执行工作台**。它以 `cmd/jftrade-api` 这一条 API sidecar 入口为核心，前端控制台、文档站、Futu/OpenD 接入、行情、交易、策略、回测、ADK 和系统诊断都围绕 `/api/v1/*` 组织。

当前主线事实：

- 后端入口：`cmd/jftrade-api`，只支持 API sidecar 模式。
- 前端入口：`apps/web`，Vue 3 + Vite；文档站使用 VitePress。
- 开发端口：API `127.0.0.1:3000`，Web `127.0.0.1:5173`，Docs `127.0.0.1:3001`。
- 发布端口：GUI `127.0.0.1:6688`，API gateway `127.0.0.1:6699`。
- Pine 主路径：`sourceFormat=pine-v6` + `runtime=pine-pinets`。
- PineTS worker：Node ESM `worker.mjs`，Go 通过 localhost gRPC 管理 worker pool。
- 回测和实盘权威边界：PineTS 产出信号、图形输出和 order intents；Go 负责撮合、成交、资金曲线、风控、账户刷新和券商下单。
- 许可证注意：`workers/pineworker` 精确依赖 `pinets@0.9.27`，当前 npm license 为 `AGPL-3.0-only`。

当前发布和验收入口：

```bash
go test ./...
npm run test:web
npm run typecheck:web
npm run test:pineworker
npm run typecheck:pineworker
npm run check:pinets-release
```

发布脚本没有写死语义化版本号，会按 `JFTRADE_VERSION`、`git describe --tags --always --dirty`、`dev` 的顺序解析版本。正式发布建议显式设置 `JFTRADE_VERSION` 或先打 git tag。

## 推荐阅读顺序

### 1. 先确认系统边界

- [architecture.md](architecture.md)：当前系统架构、单一 API 入口、请求链路和职责边界。
- [architecture/backend-coding-standards.md](architecture/backend-coding-standards.md)：后端分层约束、依赖方向和常见禁区。
- [architecture/high-roi-tech-stack-refactor-plan.md](architecture/high-roi-tech-stack-refactor-plan.md)：高收益技术栈重构计划，拆分 API 契约、状态层、实时事件、性能、领域组件和观测能力。
- [architecture/high-value-optimization-implementation-plan.md](architecture/high-value-optimization-implementation-plan.md)：高价值优化实施路线，覆盖回测执行模型、券商适配、行情 provider 和开源工程化。

### 2. 再按问题类型进入专题

- [troubleshooting.md](troubleshooting.md)：启动、端口、实时连接、OpenD、回测性能的排障入口。
- [adk.md](adk.md)：GO-ADK / Agent 控制面、权限模式、内置 tools 和运行时文件。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口。
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：策略定义、Logic Flow、Pine 编辑与 visual model 同步。
- [pinets-hardcut-migration.md](pinets-hardcut-migration.md)：PineTS 硬切替换 Go Pine runtime 的执行计划、进度、测试覆盖和性能门禁。
- [pinets-contract-audit.md](pinets-contract-audit.md)：PineTS 切换后的 Go/API/worker/前端契约矩阵和 visual output 边界。
- [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)：PineTS worker 发布、运行配置、embedded asset 和非 mock smoke 放行清单。
- [operations/observability-troubleshooting.md](operations/observability-troubleshooting.md)：从 SystemPage 的错误、慢请求和 OpenD 摘要进入结构化日志及 ADK/回测运行记录。
- [reference/README.md](reference/README.md)：协议细节、OpenD 资料和上游参考。

### 3. 最后再看历史收口记录

以下文档保留为历史背景，不是当前默认入口：

- [review-boundaries-2026-06.md](review-boundaries-2026-06.md)
- [release-closeout-2026-06.md](release-closeout-2026-06.md)
- [release-pine-v08-closeout.md](release-pine-v08-closeout.md)

## 快速路由

- 改启动方式、端口、运行时目录：先看 [architecture.md](architecture.md) 和 [troubleshooting/startup-ports.md](troubleshooting/startup-ports.md)
- 改前端默认接口、系统状态、设置：先看 [architecture.md](architecture.md)、[configuration.md](configuration.md)、[troubleshooting.md](troubleshooting.md)
- 改 ADK、agent、approval、provider、tools：先看 [adk.md](adk.md)
- 改实时行情、K 线、SSE、WS：先看 [frontend-kline.md](frontend-kline.md) 和 [troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md)
- 改 PineTS worker、worker pool、embedded asset、发布验收：先看 [pinets-contract-audit.md](pinets-contract-audit.md)、[pinets-hardcut-migration.md](pinets-hardcut-migration.md) 和 [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)
- 改回测撮合、订单成交语义或 executionModel：先看 [architecture/high-value-optimization-implementation-plan.md](architecture/high-value-optimization-implementation-plan.md)
- 改 Futu / OpenD 协议和映射：先看 [reference/README.md](reference/README.md)
- 查 HTTP、OpenD、ADK、回测或 PineTS 跨链路问题：先看 [operations/observability-troubleshooting.md](operations/observability-troubleshooting.md)

## 文档职责边界

- 根仓库 `README.md`：仓库级入口，回答“项目现在怎么跑”
- 本文档：维护者导航和当前版本快照，回答“现在是什么状态、遇到这个问题先看哪篇”
- [index.md](index.md)：VitePress 用户文档首页，面向控制台使用者

不要把实现细节、长篇回归记录或协议原文继续堆回入口文档；它们应留在专题页或 reference 层。

## AI 协作入口

后续 AI 在动手前建议按下面顺序取上下文：

1. 读 [architecture.md](architecture.md)，先判断问题属于 sidecar、前端、Futu 集成还是底层 bbgo 公共能力。
2. 读对应专题页，而不是直接在根目录全仓库盲搜。
3. 只有需要协议原文或上游背景时，才进入 [reference/README.md](reference/README.md) 或 `reference/bbgo-doc/`。
