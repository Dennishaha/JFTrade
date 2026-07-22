# 回测执行模型

JFTrade 当前只接受 `executionModel=conservative-bar-v1`。请求省略该字段时也会归一为这个值；未知模型会在业务层直接报错，不会静默切换。

## 职责边界

- PineTS worker 执行 Pine 脚本，产出信号、图形输出和 order intents。
- Go 负责撮合、订单/成交事件、账户余额、费用、资金曲线、结果持久化和风险边界。
- 回测结果记录 `executionModel`，用于区分未来可能出现的其他成交模型。

主要实现位于：

- `pkg/backtest/execution_model.go`
- `pkg/backtest/conservative_bar_executor.go`
- `pkg/backtest/pineworker_runner.go`
- `internal/backtest/run.go`

## `conservative-bar-v1` 规则

- 默认 closed-bar 信号在下一根 bar 参与撮合。
- Pine metadata 启用 `process_orders_on_close=true` 时，允许订单在当前信号 bar 的 close 点尝试成交。
- 每个 symbol、每根 bar 共享按成交量计算的流动性预算；订单按提交顺序消耗预算。
- 超出预算的数量保持 pending，后续 bar 继续撮合；零成交量 bar 不会成交。
- 数量会按市场 quantity step 和最小数量约束归一，预算不足时保留 pending 并记录 warning。
- market 与 stop-market 按 open/close 或触发价规则成交，并应用 Pine slippage tick。
- limit 单允许跳空价格改善，但成交价不会劣于 limit。
- stop-limit 先进入 triggered 状态，再从后续撮合机会按 limit 规则处理。
- 不支持的订单类型保持 pending，并在结果中记录 warning，不伪造成已成交。
- 费用按 trade update 计算，因此部分成交会逐笔进入费用明细。

## 不能推断的实盘语义

该模型是基于 OHLCV bar 的保守模拟，不是 Futu 或交易所撮合器。它无法还原：

- bar 内真实价格路径和排队顺序；
- 订单簿深度、撮合优先级和券商路由延迟；
- 行情权限、停牌、价格笼子、账户购买力及全部市场微观规则；
- 实盘网络中断、提交结果未知和 broker update 乱序。

因此，回测的 partial fill、成交价和滑点只说明该执行模型下的结果，不能当作实盘成交保证。

## 验证入口

```bash
go test ./pkg/backtest ./internal/backtest -count=1
pnpm --filter @jftrade/web run test -- backtestRequestPayload
pnpm --filter @jftrade/web run typecheck
```

涉及撮合规则的修改必须同时覆盖 market、limit、stop-market、stop-limit、部分成交、取消、零成交量、数量步进、手续费和 `process_orders_on_close`。
