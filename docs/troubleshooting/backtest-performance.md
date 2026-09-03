# 回测性能排查

## 先拆分两段

- sync：`jftrade-engine` 通过 Provider port 拉取历史 K 线，由 `jftrade-store-sqlite` 持久化。
- replay：`jftrade-backtest` 读取规范化 candles，Rust 撮合/统计并通过 `jftrade-integration-pine` 调用 PineTS worker。

先分别记录下载/写库与 replay 的墙钟、数据量和峰值内存，避免把网络、SQLite、gRPC 和 Pine 执行混成一个指标。

## 最窄验证

```bash
cargo test -p jftrade-backtest -p jftrade-integration-pine --all-targets
cargo test -p jftrade-engine --test backtests_write_compatibility
pnpm run smoke:pinets-backtest
```

`smoke:pinets-backtest` 会构建真实 Node worker，并由 Rust `PineProcess`/gRPC client 完成 readiness、native PineTS `RunScript` 和 shutdown；它不是 mock executor。

## 排查顺序

1. sync 慢：检查 Provider 查询窗口、分页、重试、取消、SQLite transaction 批量和索引。
2. replay 慢：检查 candle 数、request/response bytes、worker pool 并发、PineTS duration 和 gRPC 序列化。
3. 内存高：分开观察 Rust API、Node worker 和 Python helper RSS；确认 session close 后资源回落。
4. 无成交：先验证策略信号、warmup、session 和 execution model；`trades=0` 本身不是性能失败。
5. extended-hours：确认 session scope、日历 source 和 higher-period aggregation 使用同一 `jftrade-calendar` 规则。

性能结论必须记录固定 fixture/数据范围、commit、release/debug profile、平台、CPU/内存、迭代次数和统计方法。一次墙钟变化不能直接解释为回归；先用相同输入复现，再定位到 sync、Rust matching 或 PineTS worker。

变更共享 replay 路径后至少运行：

```bash
pnpm run check:quick
pnpm run check:rust
pnpm run check:zero-go
```

历史迁移 benchmark 只作趋势参考，不得继续引用已删除的 Go/bbgo 热点作为当前实现位置。
