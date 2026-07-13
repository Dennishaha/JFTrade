# 高价值优化实施计划

本文记录 JFTrade 后续高价值优化的实施顺序。目标不是“照搬”某个开源项目，而是把 vn.py、QuantConnect LEAN、OpenBB、NautilusTrader 等项目中已验证的工程边界，转成 JFTrade 自己可合并、可测试、可回滚的切片。

## 当前优先级

1. 回测执行正确性：先让订单生命周期、成交模型、部分成交、手续费和结果记录有明确版本。
2. 券商适配边界：再把 Futu 单券商假设收束为可验收的 broker conformance。
3. 行情数据提供者：再引入 OpenBB 风格的 provider descriptor、typed DTO 和可发现能力。
4. 开源工程化：最后补齐贡献、安全、发布、许可证核查与第三方 notice gate。

## Slice 1：回测执行模型版本化

状态：已开始落地。

### 参考来源

- QuantConnect LEAN / NautilusTrader：参考“回测与实盘共享订单语义、明确撮合模型版本”的方向。
- JFTrade 当前约束：仍保持 closed-bar Pine 策略，不追求完整 TradingView broker emulator。

### 已落地边界

- `executionModel` 成为回测请求和结果的一等字段。
- 默认且唯一模型：`conservative-bar-v1`，直接替换 bbgo 原生整单撮合，但继续复用 bbgo session、account、stream 和结果收集基础设施。
- 未知模型在业务服务层返回请求错误，不静默降级。

### `conservative-bar-v1` 当前规则

- 默认 closed-bar 信号在下一根 Bar open 成交。
- Pine metadata `process_orders_on_close=true` 时，允许同根信号 Bar close 成交。
- 每个 symbol 每根 Bar 共享 10% volume 流动性预算。
- 订单按 FIFO 消耗流动性；超出预算的数量保留为 pending，后续 Bar 继续尝试。
- market / stop-market 使用 open 或 close 基准价，并按 Pine slippage tick 做不利滑点。
- limit 单支持跳空价格改善，但成交价不会劣于 limit。
- stop-limit 先触发，再从后续 Bar 按 limit 逻辑撮合。
- fee engine 继续按每笔 trade update 计费，避免整单一次性计费造成部分成交失真。

### 主要文件

- `pkg/backtest/execution_model.go`
- `pkg/backtest/conservative_bar_executor.go`
- `pkg/backtest/result_collector.go`
- `pkg/backtest/pineworker_runner.go`
- `internal/backtest/service.go`
- `apps/web/src/contracts/index.ts`
- `apps/web/src/composables/useBacktestRuns.ts`

### 验收

- `go test ./pkg/backtest ./internal/backtest ./internal/app/apiserver/servercore -count=1 -timeout 300s`
- `pnpm --filter @jftrade/web run test backtestRequestPayload`
- `pnpm --filter @jftrade/web run typecheck`

## Slice 2：券商适配验收套件

状态：待实施。

### 参考来源

- vn.py / StockSharp：参考 gateway/connector 的能力声明、状态映射和 conformance fake。

### 实施目标

- broker registry 支持显式 default broker ID，不再依赖 map 首项。
- 下单链路去除 `brokerId=futu` 硬编码。
- fake broker 覆盖 place / cancel / query / push / partial fill / disconnect / unsupported capability。
- 原始券商状态保留，同时输出 JFTrade canonical status。

### 主要文件

- `pkg/broker/registry.go`
- `pkg/broker/broker.go`
- `internal/trading/execution.go`
- `internal/app/apiserver/servercore/execution_store.go`
- `pkg/futu/exchange_trade_read_convert.go`

## Slice 3：行情 provider 可发现化

状态：待实施。

### 参考来源

- OpenBB：参考 provider registry、provider descriptor、数据能力分层。

### 实施目标

- 将行情能力拆成 Security / Snapshot / Candles / Depth / Streaming 可选接口。
- 增加 provider descriptor：`id`、`displayName`、`capabilities`、`markets`、`status`。
- 增加 `GET /api/v1/market-data/providers`。
- 现有 Futu closure 迁移到 integration adapter，但保持现有 JSON 兼容。

### 主要文件

- `internal/marketdata`
- `internal/api/marketdata/routes.go`
- `internal/app/apiserver/servercore/marketdata_adapters.go`
- `pkg/futu/adapter_marketdata_reader.go`

## Slice 4：开源工程化

状态：待实施；顶层许可证仍需项目方确认。

### 实施目标

- 增加 `CONTRIBUTING.md`、`SECURITY.md`、`CODE_OF_CONDUCT.md`、`CHANGELOG.md`。
- 增加 `scripts/check-oss-release.sh`，做发布前人工 gate，不放进默认 CI。
- 开源前逐项复核第三方依赖许可证和 notice，尤其是 `docs/legal/third-party-notices.md` 已记录的 PineTS AGPL 边界。

### 不自动决策

- 不自动选择顶层开源许可证。
- 不把上游项目代码复制进本仓库。
- 不把商业券商密钥、OpenD 配置、用户本地数据纳入示例。

## 维护规则

- 每个 slice 独立合并，禁止把 broker/provider/OSS 三件事混在同一个提交里。
- 涉及交易或回测语义时，测试必须覆盖业务状态变化，不只覆盖 DTO 字段。
- 单个新增核心文件保持可审阅；大文件需要继续拆分。
