# JFTrade 活动路线图

更新时间：2026-07-22。

本文是仓库内唯一的活动计划入口，只记录尚未完成且仍值得推进的工作。已经落地的设计应写入对应专题文档；一次性迁移过程、发布冻结说明和验收日志由 Git 提交与发布 tag 保留，不继续作为维护文档存在。

## 当前基线

以下事项已经完成，不再列为待办：

- 后端已迁入 `internal/api/*`、`internal/*` service 与 `internal/app/apiserver`，旧 `pkg/jftradeapi` 兼容包已删除。
- Pine 执行主路径已硬切到 `runtime=pine-pinets`；Go 保留调度、撮合、风控和券商下单权威。
- ADK workflow 的计划、child run 与审批队列 UI 已落地。
- OpenAPI 类型生成、首批 TanStack Query server-state、统一 live event、重页面有界渲染、领域组件和请求观测已落地。
- 实盘前置风控、kill switch、hard stop、canonical order status、broker capability catalog 与行情 provider 状态已经进入当前代码。
- 项目许可证已确定为 `AGPL-3.0-only`，PineTS 第三方 notice 与发布合规检查已经存在。

当前事实分别维护在：

- [architecture.md](architecture.md)：运行架构与 ownership 现状。
- [backtest-execution-model.md](backtest-execution-model.md)：当前回测撮合模型与限制。
- [adk.md](adk.md)：ADK runtime、workflow、审批与 UI 投影。
- [pinets-contract-audit.md](pinets-contract-audit.md)：PineTS 执行契约。
- [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)：PineTS 构建、发布和排障门禁。
- [reference/generated/pine-v6-support.md](reference/generated/pine-v6-support.md)：自动生成的 Pine v6 支持边界。

## P0：继续收缩 `servercore` ownership

### 目标

让 `internal/app/apiserver/servercore` 逐步回到应用装配与兼容适配职责，业务 store、runtime 生命周期和领域状态由对应模块拥有。

### 建议切片

1. 将 strategy runtime/store 的 ownership 迁入 `internal/strategy` 与明确的 store 包，`servercore` 只注入端口。
2. 将 Pine worker pool 的启动、健康、关闭和配置归到独立 runtime service，避免根 `Server` 同时拥有 worker 细节和业务调用。
3. 将 ADK runtime/MCP 装配移入更窄的 assistant assembly，保留 `internal/api/assistant` 的 HTTP 边界。
4. 在每个切片完成后加强 `scripts/check-arch-deps.sh`，阻止业务包反向依赖 `servercore`。

### 验收

- 被迁移模块可用窄接口独立测试，不需要构造根 `Server`。
- 进程启动、关闭、数据库重建和 runtime resource 释放语义不变。
- `go test ./...`、`go vet ./...` 与 `bash scripts/check-arch-deps.sh` 通过。

## P1：把 broker/provider 扩展性变成可验证契约

### 当前边界

`pkg/broker` 已有 registry、capability catalog、feature router、运行时 capability evaluation 和 Futu adapter；`internal/marketdata` 已暴露当前 provider 的 descriptor/health。下一步重点不是继续增加抽象，而是证明第二实现可以安全接入。

### 建议切片

1. 建立可复用 broker conformance harness，覆盖查询、下单、撤单、推送、部分成交、断连、权限不足和 unsupported capability。
2. 让新增 adapter 的 capability 声明、实际接口、API/UI/tool surface 与测试映射同时受门禁约束。
3. 仅在出现真实第二数据源时引入多 provider registry；在此之前保留当前单 active-provider service。
4. 逐步移除产品中性流程里的 Futu 默认假设，但保留 Futu/OpenD 专属设置、诊断和协议实现。

### 验收

- fake adapter 可以跑完整 conformance suite，且不会访问 OpenD。
- 显式 broker 选择、默认 broker 与 fallback 的行为稳定且可观测。
- 未声明或未实现的 capability 在路由前失败，不在 handler 内静默降级。
- 新接入流程与 [new-broker-integration-guide.md](new-broker-integration-guide.md) 保持一致。

## P1：持续加固交易解释性

### 目标

在不夸大回测/实盘一致性的前提下，让用户始终知道订单作用域、风险决策和执行结果来自哪里。

### 建议切片

1. 对所有可交易入口持续校验环境、账户、broker、标的、策略版本和 execution model 展示是否一致。
2. 在策略启动前明确阻断 PineTS 或 live runtime 不支持的语义，不把 warning 当成可安全执行。
3. 让手工、策略与未来 ADK 交易动作共享同一个 pre-trade risk 证据和订单状态闭环。
4. 为关键拒绝、未知提交结果和 out-of-order broker update 保留可定位的审计信息。

### 验收

- REAL 下单在 risk gateway 不可用时 fail closed。
- kill switch/hard stop 与并发在途订单的语义有业务测试覆盖。
- 回测结果明确记录 `executionModel`，UI 不把 bar-based 撮合描述成券商真实成交。

## P2：补齐通用开源治理文件

### 当前边界

根许可证、第三方 notices 与 PineTS 合规门禁已经存在；仓库仍缺少贡献、安全响应、行为准则和变更记录入口。

### 建议切片

1. 增加 `CONTRIBUTING.md`、`SECURITY.md`、`CODE_OF_CONDUCT.md` 与 `CHANGELOG.md`。
2. 增加独立的 OSS release check，核对许可证、notice、生成产物、源码提供说明与敏感文件；是否进入默认 CI 由发布策略决定。
3. 将漏洞报告、支持范围和发布版本策略与 GitHub 仓库设置保持一致。

### 验收

- 新贡献者能从根 README 找到开发、测试、提交和安全报告流程。
- 发布检查不会把本地密钥、OpenD 配置或用户数据库带入产物。
- 第三方依赖或许可证变化会触发 notice 审查。

## 非目标

- 不恢复 Go Pine runtime，不追求完整 TradingView broker emulator 或笼统的“100% Pine v6”。
- 没有真实第二实现前，不为多券商或多 provider 做大规模预留式重构。
- 不替换现有 Go/Gin、Vue/Vite/Vuetify、PineTS worker 主栈。
- 不在计划文档中复制长测试日志、提交清单或已经完成的迁移阶段。

## 维护规则

1. 新计划先判断能否归入上述主题；能归入时更新本文，不新建平行 roadmap。
2. 每个事项必须包含当前边界、主要代码落点和可验证的完成标准。
3. 完成后在同一变更中更新专题事实文档，并从本文删除对应待办。
4. 一次性 release/review/closeout 说明写在 GitHub Release、PR 或 commit 中，不新增长期跟踪的日期型 Markdown。
