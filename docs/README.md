# JFTrade 维护者文档导航

这份 README 面向仓库维护者、协作者和后续 AI。它不重复介绍项目本身，只负责把你引到正确的事实来源。

如果只看一篇，请先看本文和 [architecture.md](architecture.md)。需要图形化总览时看 [architecture-mermaid.md](architecture-mermaid.md)。

## 当前版本快照

更新时间：2026-08-21。本文描述当前工作树的运行边界；提交版本以仓库实际 HEAD 和 `vX.Y.Z` 发布 tag 为准。

JFTrade 当前是 **Futu-first 的本地量化策略研发与半自动执行工作台**。交易链路仍由 Futu/OpenD 管理；新安装的行情默认使用内置 AKShare 延迟数据源，支持美股、港股和沪深，也可以选择 Futu OpenD。系统以同一套 API sidecar 为核心，可由 `cmd/jftrade-api` 独立启动，也可由 `cmd/jftrade-desktop` 管理；前端控制台、行情、交易、策略、回测、ADK 和系统诊断都围绕 `/api/v1/*` 组织。

当前主线事实：

- 独立后端入口：`cmd/jftrade-api`，只支持 API sidecar 模式。
- 桌面入口：`cmd/jftrade-desktop`，使用 Wails `v3.0.0-beta.8`，先显示窗口、再异步启动内置 API；仍通过 HTTP/SSE/WebSocket 访问 sidecar，并将启动状态、链接、日志和更新检查暴露为桌面 bindings。
- 前端入口：`apps/web`，Vue 3 + Vite；文档站使用 VitePress。
- 开发端口：API `127.0.0.1:3000`，Web `127.0.0.1:3003`，Docs `127.0.0.1:3001`。
- 桌面内部端口：`JFTrade Dev` sidecar 为 `127.0.0.1:3008`，正式 `JFTrade` sidecar 为 `127.0.0.1:6699`；两者仅供 Wails 使用且可同时运行。
- 可选 Web 端口：默认 `127.0.0.1:6688`，可在桌面设置中修改；Web 关闭时桌面产品不创建该监听器。
- 内置 Python 行情 helper：发布版随 `release_assets` 嵌入并由 JFTrade 自动启动，yfinance 与 AKShare 在同一进程中隔离运行；提供 `US`、`HK`、`SH`、`SZ` 的延迟查询与历史 K 线，不提供实时推流、Level 2 或实盘策略行情。发布资产必须显式准备；Wails 开发启动不再自动选择或构建 Python helper。
- 数据隔离：桌面开发版继续使用仓库 `var/jftrade-api`；正式产品使用系统用户数据目录，不扫描或迁移开发数据。
- 自选系统：`watchlists.db` 是本地唯一主数据，支持多分组、Futu 只读预览导入、可见行快照行情和 ADK 只读查询。
- Pine 主路径：`sourceFormat=pine-v6` + `runtime=pine-pinets`。
- PineTS worker：Node ESM `worker.mjs`，Go 通过 localhost gRPC 管理 worker pool。
- 回测和实盘权威边界：PineTS 产出信号、图形输出和 order intents；Go 负责撮合、成交、资金曲线、风控、账户刷新和券商下单。
- Rust 迁移：阶段 1 共存 bridge，以及阶段 2 codec/SQLite、阶段 3 回测、阶段 4 行情/worker 生命周期、阶段 5 交易/策略、阶段 6 Assistant/Rig、阶段 7 API/control-plane、阶段 8 Tauri desktop facade 的本地 shadow 证据已建立；阶段 9 已验证受鉴权的 Rust read-only API shadow、calendar manager control-plane、watchlist/plugin/portfolio/research/execution/broker/system/backtest/strategy read projections 及 Go fixture differential、Tauri macOS RC，以及 settings/SQLite 的 Go/Rust 跨进程 writer lease。route ledger v2 逐 operation 登记并由门禁派生为 26 个 GET shadow、88 个 cutover-test-only、164 个 remaining、0 个 Rust production owner。Vue 已通过单一 facade 支持 Wails/Tauri adapter，但正式产品仍只运行 Wails，Go/Wails 继续拥有公开 API、全部业务写入、生产数据库、Futu/Assistant 与桌面发布。Rust RC 启动的 Node/Python 仅用于隔离发布 smoke；全量 route group、真实 live、持久化恢复、签名 updater、四平台 RC、hard-cut readiness、签名 rollback artifact 与备份恢复演练仍阻断产品切换；正式硬切不设置生产观察窗口，也不保留 Go runtime fallback。正式关闭证据可先用 `pnpm run report:rust:stage9:closeout` 只读查看，`pnpm run check:rust:stage9:closeout` 在所有条件满足前 fail-closed。完整阶段、目录约束和切换条件见 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)。
- 当前 data-management maintenance 增量只在临时数据库和 `cutover-test-only.v1` 启用：cleanup execute、backup、compact、rebuild 均经过 writer lease、候选指纹或可验证备份 fencing。calendar、read projection、backtest run/sync read、strategy-instance-read、research-preset-read 与 execution-read 组继续只在显式 snapshot port 下登记，最新门禁派生为 26 shadow、88 cutover-test-only、164 remaining、0 Rust production owner；正式 Go/Wails owner 不变。
- Rust calendar persistence 已兼容 Go owner 的 `MARKET/YYYY/source.json`、RFC3339Nano JSON、`0755/0644` 权限与同目录 fsync 原子替换；加载不会创建路径，并会在保留有效 snapshot 的同时逐文件报告遍历、权限、截断和 JSON 损坏。它已由 fixture-backed manager lifecycle 消费，但尚未接入正式 launcher 或扩大 route owner。
- Rust calendar manager 已在 fixture source 边界内接入 registry、builtin/manual policy、snapshot restore/cache、source health/backoff、settings reload、start/close/cancel；sources/status 及 probe/refresh 四个控制操作由同一 manager 在 test-cutover profile 提供，覆盖未知市场、全失败/部分成功、超时、取消、缓存恢复和持久化失败。当前未复制真实 HTTP Provider，尚未接入正式 launcher 或生产 owner。
- 许可证注意：`workers/pineworker` 精确依赖 `pinets@0.9.31`，当前 npm license 为 `AGPL-3.0-only`。

当前发布和验收入口：

```bash
go test ./...
pnpm run test:web
pnpm run typecheck:web
pnpm run test:pineworker
pnpm run typecheck:pineworker
pnpm run check:rust
workers/marketdata-sidecar/.venv/bin/python -m pytest workers/marketdata-sidecar/tests
pnpm run check:pinets-release
pnpm run check:wails-bindings
go test -tags release_assets ./cmd/jftrade-desktop ./internal/desktop -count=1
```

独立 API 发行脚本仍按 `JFTRADE_VERSION`、`git describe --tags --always --dirty`、`dev` 解析版本。Wails 正式产品只接受 `vX.Y.Z`，并把版本、提交号和构建时间注入 Go buildinfo 与平台资源；`dev` 与 `v0.0.0` 禁止进入桌面 release。

## 推荐阅读顺序

### 1. 先确认系统边界

- [architecture.md](architecture.md)：当前系统架构、单一 API 入口、请求链路和职责边界。
- [architecture-mermaid.md](architecture-mermaid.md)：项目架构、主要运行链路和开发/发布链路的 Mermaid 图。
- [architecture/backend-coding-standards.md](architecture/backend-coding-standards.md)：后端分层约束、依赖方向和常见禁区。
- [architecture/goroutine-lifecycle-audit.md](architecture/goroutine-lifecycle-audit.md)：61 个异步启动面的 owner、取消、join、风险与修复账本。
- [architecture/sqlite-query-plan-audit.md](architecture/sqlite-query-plan-audit.md)：9 个 SQLite 数据库的生产查询计划、索引决策与迁移阻断项。
- [architecture/public-package-policy.md](architecture/public-package-policy.md)：`pkg/*` 的公开契约、保留/内移标准和当前决策。
- [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)：Go/Wails → Rust/Tauri 完整迁移方案、守则、依赖和阶段门禁。
- [architecture/rust-migration-execution-playbook.md](architecture/rust-migration-execution-playbook.md)：迁移 harness 的先读协议、广度优先并行调度、route group 工作包、bug/quirks 收集、验证和 Go 删除准入。
- [testing-strategy.md](testing-strategy.md)：覆盖率分层、PR/main 门禁和真实外部依赖的运行边界。
- [roadmap.md](roadmap.md)：唯一活动计划入口，只记录尚未完成的高价值事项与验收标准。

### 2. 再按问题类型进入专题

- [troubleshooting.md](troubleshooting.md)：启动、端口、实时连接、OpenD、回测性能的排障入口。
- [market-data-providers.md](market-data-providers.md)：Futu/yfinance 行情能力、内置 helper、进程生命周期与设置边界。
- [market-data-provider-qualification.md](market-data-provider-qualification.md)：研究中心后续数据源资格门槛与扩展候选。
- [adk.md](adk.md)：ADK Go v2 / Agent 控制面、权限模式、内置 tools 和运行时文件。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口。
- [watchlist.md](watchlist.md)：自选系统的使用方式、数据主权、Futu 导入、快照行情、API、ADK 和扩展边界。
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：策略定义、结构指令、Pine 编辑与 visual model 投影。
- [frontend/api-contracts.md](frontend/api-contracts.md)：OpenAPI 生成类型、前端 view model/mapper、typed API 与原生请求边界。
- [frontend/state-management.md](frontend/state-management.md)：Vue Query、页面 composable、context 与受控 singleton 的状态归属规则。
- [frontend/bundle-budget.md](frontend/bundle-budget.md)：首屏与异步 chunk 的 gzip 基线、重依赖懒加载约束和本地报告命令。
- [frontend/styling-guide.md](frontend/styling-guide.md)：Vuetify、Tailwind、全局 primitive 与 scoped CSS 的职责边界。
- [backtest-execution-model.md](backtest-execution-model.md)：`conservative-bar-v1` 的成交规则、职责边界和实盘差异。
- [pinets-contract-audit.md](pinets-contract-audit.md)：PineTS 切换后的 Go/API/worker/前端契约矩阵和 visual output 边界。
- [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)：PineTS worker 发布、运行配置、embedded asset 和非 mock smoke 放行清单。
- [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md)：Wails v3 开发/产品通道隔离、系统数据目录、版本注入、ARM64-only macOS 无签名 DMG、Windows 无签名安装器与发布产物。
- [troubleshooting/desktop-startup-performance.md](troubleshooting/desktop-startup-performance.md)：开发/正式桌面冷启动、缓存命中、窗口和 API 墙钟基准。
- [troubleshooting/marketdata-sidecar.md](troubleshooting/marketdata-sidecar.md)：内置 helper、开发态路径、上游错误和延迟行情排障。
- [operations/observability-troubleshooting.md](operations/observability-troubleshooting.md)：从设置页“开发者工具”的错误、慢请求和 OpenD 摘要进入结构化日志及 ADK/回测运行记录。
- [reference/README.md](reference/README.md)：协议细节、OpenD 资料和上游参考。
- [new-broker-integration-guide.md](new-broker-integration-guide.md)：当前 broker capability、注册和验收约束。

### 3. 计划与契约治理

- [roadmap.md](roadmap.md)：尚未完成的项目级工作；完成项应从路线图删除并写入专题事实文档。
- [reference/api-lifecycle.md](reference/api-lifecycle.md)：deprecated、tombstone 与有意保留端点的治理记录。
- 历史迁移、发布收口和 review 边界通过 Git 提交、tag 与 GitHub Release 查询，不在 `docs/` 重复保留过期快照。

## 快速路由

- 改启动方式、端口、运行时目录：先看 [architecture.md](architecture.md) 和 [troubleshooting/startup-ports.md](troubleshooting/startup-ports.md)
- 改 Wails profile、bindings、菜单、窗口状态或桌面发布：先看 [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md) 和 `cmd/jftrade-desktop`
- 改前端默认接口、系统状态、设置：先看 [architecture.md](architecture.md)、[configuration.md](configuration.md)、[troubleshooting.md](troubleshooting.md)
- 改 HTTP wire contract、前端 API 类型或请求封装：先看 [frontend/api-contracts.md](frontend/api-contracts.md)
- 改 ADK、agent、approval、provider、tools：先看 [adk.md](adk.md)
- 改行情数据源选择、yfinance 或统一 Provider：先看 [market-data-providers.md](market-data-providers.md)、[architecture.md](architecture.md) 和 [troubleshooting/marketdata-sidecar.md](troubleshooting/marketdata-sidecar.md)
- 改实时行情、K 线、SSE、WS：先看 [frontend-kline.md](frontend-kline.md) 和 [troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md)
- 改自选分组、星标、券商导入或自选快照：先看 [watchlist.md](watchlist.md)
- 改 PineTS worker、worker pool、embedded asset、发布验收：先看 [pinets-contract-audit.md](pinets-contract-audit.md) 和 [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)
- 改回测撮合、订单成交语义或 executionModel：先看 [backtest-execution-model.md](backtest-execution-model.md)
- 改 Rust workspace、Go/Rust bridge、迁移 owner 或候选依赖：先看 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)
- 让 AI/harness 规划或执行 Go → Rust 迁移：先看 [architecture/rust-migration-execution-playbook.md](architecture/rust-migration-execution-playbook.md)，再看 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md) 和对应局部 AGENTS.md
- 改 broker capability、默认选择或新增 adapter：先看 [new-broker-integration-guide.md](new-broker-integration-guide.md) 和 [roadmap.md](roadmap.md)
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
