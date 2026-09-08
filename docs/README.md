# JFTrade 维护者文档导航

本文只负责“这个问题先看哪里”，不维护另一份架构、工具链版本或发布状态快照。启动项目看 [根 README](../README.md)，使用控制台看 [用户首页](index.md) 和 [快速开始](quick-start.md)。

## 事实源与阅读顺序

1. 开发任务先读根 [AGENTS.md](../AGENTS.md) 和目标路径沿途的局部指令；从 [模块表](../scripts/module-map.json) 定位源码入口和影响范围。
2. 需要理解运行边界时读 [系统架构](architecture.md)；需要图形总览时读 [架构图](architecture-mermaid.md)。
3. 根据下表选择相关专题。只有核对协议原文或历史设计时，才进入 reference / history。

| 要确认的事实 | 唯一维护位置 |
| --- | --- |
| 开发规则与局部上下文 | [根 AGENTS.md](../AGENTS.md) 及其链接的局部指令 |
| 命令、Node / pnpm、Rust 工具链 | [package.json](../package.json)、[rust-toolchain.toml](../rust-toolchain.toml)；依赖解析以已提交锁文件为准 |
| 模块入口和 affected 归属 | [scripts/module-map.json](../scripts/module-map.json) |
| 当前运行架构、API 与写入所有权 | [architecture.md](architecture.md) 和对应领域专题 |
| 产品门禁、CI 与 affected 规则 | [quality-gates.md](architecture/quality-gates.md)；执行语义以其中对应脚本/workflow 为准 |
| 候选、正式发布和发布后证明 | [release-qualification.md](architecture/release-qualification.md) 与绑定 commit SHA 的实际 receipt |
| 尚未完成的项目级工作 | [roadmap.md](roadmap.md) |
| 公开 HTTP wire contract | [contracts/openapi/openapi.json](../contracts/openapi/openapi.json)；[生成参考](reference/README.md) 不是手工编辑入口 |

## 按任务进入专题

### 运行、桌面与诊断

- 启动、端口、配置、数据目录：[系统架构](architecture.md)、[配置](configuration.md)、[启动与端口排障](troubleshooting/startup-ports.md)。
- Tauri profile、窗口、IPC、安装包：[桌面局部指令](../apps/desktop/src-tauri/AGENTS.md)、[桌面构建与发布](troubleshooting/desktop-release.md)。
- 冷启动与性能定位：[桌面启动性能](troubleshooting/desktop-startup-performance.md)。
- 启动失败、OpenD、实时连接：[排障入口](troubleshooting.md)、[实时流连接](troubleshooting/live-stream-connection.md)。
- 跨 HTTP、OpenD、ADK、回测和 PineTS 的诊断：[可观测性与排障](operations/observability-troubleshooting.md)。

### Rust、行情与交易

- Rust 分层、port、store 和 integration：[Rust 局部指令](../crates/AGENTS.md)、[后端编码规范](architecture/backend-coding-standards.md)。
- Provider 选择、Futu/yfinance/AKShare 能力：[行情数据源](market-data-providers.md)、[helper 排障](troubleshooting/marketdata-sidecar.md)。
- 行情 helper 的安装、内部 API 与测试：[helper README](../workers/marketdata-sidecar/README.md)。
- 研究数据源扩展与资格：[数据源资格门槛](market-data-provider-qualification.md)。
- 自选分组、星标、券商导入和快照：[自选系统](watchlist.md)。
- broker capability、默认选择和 adapter 扩展：[券商集成指南](new-broker-integration-guide.md)。
- Futu/OpenD 协议和映射：[协议参考导航](reference/README.md)。

### 策略、回测与 Assistant

- Pine 编辑、结构指令与 visual model：[策略创作](frontend/strategy-authoring.md)。
- PineTS worker / worker pool：[Worker 局部指令](../workers/AGENTS.md)、[PineTS 契约](pinets-contract-audit.md)。
- PineTS embedded assets 与非 mock smoke：[worker 发布排障](troubleshooting/pinets-worker-release.md)。
- 撮合、成交语义、executionModel 与实盘差异：[回测执行模型](backtest-execution-model.md)。
- ADK、agent、approval、provider 和 tools：[ADK 控制面](adk.md)。

### Vue 控制台

- 页面、组件、composable 的开发入口：[Web 局部指令](../apps/web/AGENTS.md)。
- OpenAPI 类型、wire / view model、请求封装：[API 契约](frontend/api-contracts.md)。
- Vue Query、页面 context、singleton 所有权：[状态管理](frontend/state-management.md)。
- 实时行情和 K 线：[前端 K 线](frontend-kline.md)。
- Vuetify / Tailwind、tokens、scoped CSS：[样式规范](frontend/styling-guide.md)。
- 首屏与异步 chunk、重依赖懒加载：[bundle 预算](frontend/bundle-budget.md)。

### 验证与发布

- 本地门禁、PR/main、affected planner：[质量门禁](architecture/quality-gates.md)。
- 行为测试、覆盖率、fixture 和 live 边界：[测试策略](testing-strategy.md)。
- 候选、签名、安装升级回滚、SBOM 和发布后验收：[发布资格](architecture/release-qualification.md)。
- 滚动升级输入：[升级基线](../tests/fixtures/release/upgrade-baselines.json)。不得从历史源码重建已发布基线。
- deprecated、tombstone 和有意保留端点：[API 生命周期](reference/api-lifecycle.md)。
- 开源许可与第三方依赖：[Third-Party Notices](legal/third-party-notices.md)。

## 历史与上游参考

以下资料不是当前生产架构、开发路径或门禁依据：

- [Go → Rust 历史资料](history/go-to-rust/README.md) 和 [SQLite 查询计划审计](history/go-to-rust/sqlite-query-plan-audit.md)。
- [历史异步生命周期审计](architecture/goroutine-lifecycle-audit.md)、[历史公开包治理](architecture/public-package-policy.md)。
- [bbgo 上游参考](reference/bbgo-doc/README.md)。当前控制台不以 bbgo 原生 API 运行。

历史发布状态和验收结论通过对应 Git commit、tag、Release 和证据查询；不要根据文档更新时间推断当前发布资格。

## 文档维护规则

- 根 `README.md` 回答“是什么、怎么跑”；本文回答“去哪里找”；专题页维护规则与实现边界；reference 保存协议或生成参考。
- 命令发布前核对对应 `package.json` 的脚本名、工作目录和参数；只读检查与会写资产的生成/构建命令分开说明。
- 公开 HTTP 契约有意变化时，先改规范源，再运行 `pnpm run generate:docs`；验证用 `pnpm run check:generated`，不手改生成类型或 `docs/reference/generated/*`。
- 变更影响运行边界时更新对应架构/专题和模块表，再补导航链接；不要把整段事实复制到多个入口。
- 完成项从 roadmap 移除，稳定规则归入专题；一次性进度、迁移报告和覆盖率冲刺记录不进入架构事实页。
- 修改文档/AI 指令后核对本地链接与示例命令，执行 `pnpm run check:ai-context` 和 `pnpm run check:quick`。不因是文档改动绕过 planner 的全量兜底。
