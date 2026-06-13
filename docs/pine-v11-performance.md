# Pine v6 v1.1 Support and Performance

v1.1 focuses on practical TradingView strategy migration and measurable execution efficiency. It does not add arrays, maps, matrices, imports, dynamic `request.security`, or a full intrabar broker emulator.

## Backtest Semantics

- `strategy()` supports `initial_capital`, `commission_type`, `commission_value`, `slippage`, and `process_orders_on_close`.
- Initial balance precedence is API `initialBalance`, then Pine `initial_capital`, then the JFTrade default.
- Percent and cash commission plus slippage ticks affect backtests only. Live broker orders are not modified.
- `strategy.close` and `strategy.close_all` accept `immediately=true`.
- `comment` is recorded in strategy logs. `alert_message` emits a strategy notification unless `disable_alert=true`.
- OCA, partial fills, stop-limit combinations, and intrabar recalculation remain unsupported and return explicit diagnostics.

## Benchmark Layers

1. Compiler benchmarks measure tokenize, AST, lowering, planning, and complete analysis.
2. Runtime benchmarks push 2K or 10K K-lines directly without SQLite.
3. End-to-end benchmarks execute `pinespec.GoldenExamples()` through the backtest runner.

Run locally:

```bash
go test ./pkg/strategy/pine -run '^$' -bench 'BenchmarkPine' -benchmem
go test ./pkg/strategy/pineruntime -run '^$' -bench 'BenchmarkPineRuntimePushKLines' -benchmem
go test ./pkg/backtest -run '^$' -bench 'BenchmarkRunExecutesPineGoldenMatrix' -benchmem -benchtime 3x
```

The performance workflow runs base and head eight times on the same self-hosted runner and publishes benchstat, CPU, `alloc_space`, and `alloc_objects` artifacts. The old absolute JSON baseline is optional historical reference rather than the default gate.
