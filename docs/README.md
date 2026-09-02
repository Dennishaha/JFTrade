# JFTrade 维护者文档导航

这份 README 面向仓库维护者、协作者和后续 AI。它不重复介绍项目本身，只负责把你引到正确的事实来源。

如果只看一篇，请先看本文和 [architecture.md](architecture.md)。需要图形化总览时看 [architecture-mermaid.md](architecture-mermaid.md)。

## 当前版本快照

更新时间：2026-09-02。本文描述当前工作树的运行边界；提交版本以仓库实际 HEAD 和 `vX.Y.Z` 发布 tag 为准。

JFTrade 当前是 **Futu-first 的本地量化策略研发与半自动执行工作台**。交易链路仍由 Futu/OpenD 管理；新安装的行情默认使用内置 AKShare 延迟数据源，支持美股、港股和沪深，也可以选择 Futu OpenD。生产系统以 Rust engine API 为核心，可独立启动，也可由 Tauri 桌面壳管理；前端控制台、行情、交易、策略、回测、ADK 和系统诊断都围绕 `/api/v1/*` 组织。

当前主线事实：

- 独立后端入口：`cargo run -p jftrade-engine --bin jftrade-api-rust`，默认绑定 `127.0.0.1:3000`。
- 桌面入口：`apps/desktop/src-tauri`（Tauri 2），先显示窗口，再注入受管 Rust API 的 loopback URL 和临时桌面 Bearer token；前端仍通过 HTTP/SSE/WebSocket 访问 API。
- 前端入口：`apps/web`，Vue 3 + Vite；文档站使用 VitePress。
- 开发端口：API `127.0.0.1:3000`，Web `127.0.0.1:3003`，Docs `127.0.0.1:3001`。
- 桌面内部端口：开发壳 API 为 `127.0.0.1:3008`，正式壳 API 为 `127.0.0.1:6699`；两者仅供 Tauri WebView 使用且可同时运行。
- 可选 Web 端口：默认 `127.0.0.1:6688`，可在桌面设置中修改；Web 关闭时桌面产品不创建该监听器。
- 内置 Python 行情 helper：发布版随受管 runtime 准备并由 Rust product runtime 启动，yfinance 与 AKShare 在同一进程中隔离运行；提供 `US`、`HK`、`SH`、`SZ` 的延迟查询与历史 K 线，不提供实时推流、Level 2 或实盘策略行情。发布资产必须显式准备。
- 数据隔离：桌面开发版继续使用仓库 `var/jftrade-api`；正式产品使用系统用户数据目录，不扫描或迁移开发数据。
- 自选系统：`watchlists.db` 是本地唯一主数据，支持多分组、Futu 只读预览导入、可见行快照行情和 ADK 只读查询。
- Pine 主路径：`sourceFormat=pine-v6` + `runtime=pine-pinets`。
- PineTS worker：Node ESM `worker.mjs`，Rust product runtime 通过 localhost gRPC 管理 worker pool 生命周期。
- 回测和实盘权威边界：PineTS 产出信号、图形输出和 order intents；Rust engine 负责撮合、成交、资金曲线、风控、账户刷新和券商下单调度。
- 零 Go 主线：278 条 `/api/v1/*` production route 均登记为 `cutover-qualified`、`productionOwner=rust`、`goRemovalStatus=removed`；Go/Wails 源码、模块、生成器、CI/构建入口和运行产物已删除，Tauri 是唯一桌面壳。
- `0.29.0` 是计划中的首个零 Go 版本；线上最后一个正式 Go 基线是原样发布的 `v0.27.0`，其 tag、commit、安装包地址和官方 checksum 记录在 `last-go-release-baseline.json`，不重建、不补发、不生成新的最终 Go corpus。
- 当前 closeout 尚未关闭：Stage 9 closeout 仍为 `in_progress`；四平台 package/install/upgrade/uninstall/rollback/runtime smoke、签名 updater、security review、SBOM、rollback artifact、backup/restore 和 post-release smoke 门禁未全部闭合。请用 `pnpm run report:rust:stage9:closeout` 查看当前证据，不要把 release/closeout 写成已完成。
- 生产 SQLite 由 Rust engine 统一初始化并以唯一 `WriterLease` 声明写属主；数据库损坏、schema drift 或租约冲突会 fail-closed。data-management cleanup/backup/compact/rebuild 走生产 fencing 流程。
- Rust calendar manager 在生产 composition 中提供持久化、settings reload、source health/backoff、snapshot/cache、start/close/cancel 与控制操作；外部 calendar source 不可用时按各自契约 fail-closed。
- 许可证注意：`workers/pineworker` 精确依赖 `pinets@0.9.31`，当前 npm license 为 `AGPL-3.0-only`。

当前发布和验收入口：

```bash
pnpm run test:web
pnpm run typecheck:web
pnpm run test:pineworker
pnpm run typecheck:pineworker
pnpm run check:quick              # 当前工作树快速反馈
pnpm run check:affected           # merge-base affected 集成门禁
pnpm run check:rust:workspace     # Rust workspace 质量与测试
pnpm run check:rust:differential  # Stage 2-9 完整 differential
pnpm run check:rust
pnpm run check:go-retirement     # 单调递减历史账本，禁止 Go/Wails 回流
pnpm run check:zero-go           # 当前源码、构建链与传入发布产物的零 Go 不变量
pnpm run check:rust:target-health # 检测中断编译遗留对象
pnpm run clean:rust:artifacts     # 确认无 Cargo 进程后显式清理 target
workers/marketdata-sidecar/.venv/bin/python -m pytest workers/marketdata-sidecar/tests
pnpm run check:pinets-release
pnpm run check:tauri-release-runtime
```

独立 Rust API 与 Tauri release launcher 仍按 `JFTRADE_VERSION`、`git describe --tags --always --dirty`、`dev` 解析版本，并把版本、提交号和构建时间锁入 Rust `/api/v1/system/status` build identity 与 Tauri bundle config；`dev` 与 `v0.0.0` 禁止进入桌面 release。

## 推荐阅读顺序

### 1. 先确认系统边界

- [architecture.md](architecture.md)：当前系统架构、单一 API 入口、请求链路和职责边界。
- [architecture-mermaid.md](architecture-mermaid.md)：项目架构、主要运行链路和开发/发布链路的 Mermaid 图。
- [architecture/backend-coding-standards.md](architecture/backend-coding-standards.md)：后端分层约束、依赖方向和常见禁区。
- [architecture/goroutine-lifecycle-audit.md](architecture/goroutine-lifecycle-audit.md)：历史异步生命周期审计；仅作迁移来源资料，不参与当前状态计算。
- [architecture/sqlite-query-plan-audit.md](architecture/sqlite-query-plan-audit.md)：9 个 SQLite 数据库的生产查询计划、索引决策与迁移阻断项。
- [architecture/public-package-policy.md](architecture/public-package-policy.md)：历史公开包治理记录；仓库当前没有 Go `pkg/*` 生产 API。
- [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)：已完成迁移事实、线上兼容基线和 `0.29.0` 放行边界。
- [architecture/rust-migration-execution-playbook.md](architecture/rust-migration-execution-playbook.md)：零 Go closeout、fixture replay 和发布资格的执行协议。
- [testing-strategy.md](testing-strategy.md)：覆盖率分层、PR/main 门禁和真实外部依赖的运行边界。
- [roadmap.md](roadmap.md)：唯一活动计划入口，只记录尚未完成的高价值事项与验收标准。

### 2. 再按问题类型进入专题

- [troubleshooting.md](troubleshooting.md)：启动、端口、实时连接、OpenD、回测性能的排障入口。
- [market-data-providers.md](market-data-providers.md)：Futu/yfinance 行情能力、内置 helper、进程生命周期与设置边界。
- [market-data-provider-qualification.md](market-data-provider-qualification.md)：研究中心后续数据源资格门槛与扩展候选。
- [adk.md](adk.md)：ADK/Agent 控制面、权限模式、内置 tools 和运行时文件。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口。
- [watchlist.md](watchlist.md)：自选系统的使用方式、数据主权、Futu 导入、快照行情、API、ADK 和扩展边界。
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：策略定义、结构指令、Pine 编辑与 visual model 投影。
- [frontend/api-contracts.md](frontend/api-contracts.md)：OpenAPI 生成类型、前端 view model/mapper、typed API 与原生请求边界。
- [frontend/state-management.md](frontend/state-management.md)：Vue Query、页面 composable、context 与受控 singleton 的状态归属规则。
- [frontend/bundle-budget.md](frontend/bundle-budget.md)：首屏与异步 chunk 的 gzip 基线、重依赖懒加载约束和本地报告命令。
- [frontend/styling-guide.md](frontend/styling-guide.md)：Vuetify、Tailwind、全局 primitive 与 scoped CSS 的职责边界。
- [backtest-execution-model.md](backtest-execution-model.md)：`conservative-bar-v1` 的成交规则、职责边界和实盘差异。
- [pinets-contract-audit.md](pinets-contract-audit.md)：PineTS 历史迁移契约和当前 worker/前端 visual output 边界。
- [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)：PineTS worker 发布、运行配置、embedded asset 和非 mock smoke 放行清单。
- [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md)：Tauri 2 开发/产品通道隔离、系统数据目录、版本注入、平台安装包与发布产物。
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
- 改 Tauri profile、IPC、菜单、窗口状态或桌面发布：先看 [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md) 和 `apps/desktop/src-tauri`
- 改前端默认接口、系统状态、设置：先看 [architecture.md](architecture.md)、[configuration.md](configuration.md)、[troubleshooting.md](troubleshooting.md)
- 改 HTTP wire contract、前端 API 类型或请求封装：先看 [frontend/api-contracts.md](frontend/api-contracts.md)
- 改 ADK、agent、approval、provider、tools：先看 [adk.md](adk.md)
- 改行情数据源选择、yfinance 或统一 Provider：先看 [market-data-providers.md](market-data-providers.md)、[architecture.md](architecture.md) 和 [troubleshooting/marketdata-sidecar.md](troubleshooting/marketdata-sidecar.md)
- 改实时行情、K 线、SSE、WS：先看 [frontend-kline.md](frontend-kline.md) 和 [troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md)
- 改自选分组、星标、券商导入或自选快照：先看 [watchlist.md](watchlist.md)
- 改 PineTS worker、worker pool、embedded asset、发布验收：先看 [pinets-contract-audit.md](pinets-contract-audit.md) 和 [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)
- 改回测撮合、订单成交语义或 executionModel：先看 [backtest-execution-model.md](backtest-execution-model.md)
- 改 Rust workspace、历史兼容 fixture、零 Go owner 或候选依赖：先看 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)
- 让 AI/harness 处理零 Go、fixture replay 或 `0.29.0` closeout：先看 [architecture/rust-migration-execution-playbook.md](architecture/rust-migration-execution-playbook.md)，再看 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md) 和对应局部 AGENTS.md
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
