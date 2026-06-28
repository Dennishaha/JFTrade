# 回测性能排查

本文只回答一类问题：回测为什么慢，以及应该先查 sync 还是 replay。

## 先分两段，不要混着看

- sync：`pkg/backtest/internal/storage` 从 OpenD 拉历史 K 线并落 SQLite。
- replay：`pkg/backtest.RunWithPineWorker` 把 SQLite 里的 K 线回放给 bbgo backtest.Exchange，并把 Pine 源码、参数和 K 线窗口交给 PineTS worker 计算 signals/plots/debug。

如果不先把两段拆开，只看一份混合 benchmark，容易把 OpenD/SQLite 成本和策略运行时成本混在一起。

## 当前推荐的真实链路检查入口

当前硬切方向是 Go 主引擎 + PineTS worker。真实链路排障时先确认三件事：

- Go 能从 storage 读取目标区间 K 线。
- Pine worker 健康检查通过，并能在超时内完成脚本计算。
- Go 撮合、资金曲线和指标统计没有被 worker 错误短路。

### 1. 先跑 Go 回测与 worker 单测

```bash
go test ./pkg/backtest ./pkg/strategy/pineworker -run Test
```

重点看：

- worker 是否返回 runtime error。
- replay 是否产生符合预期的 trades/equity/metrics。
- `0 trades` 是否来自真实无信号，而不是 worker error 或 warmup 不足。

### 2. 再单独跑 Pine worker 性能门槛

```bash
go test ./pkg/strategy/pineworker \
  -bench BenchmarkCheckPerformanceGate \
  -run '^$' -benchmem
```

这条 benchmark 是 worker 边界的轻量守卫。它不能替代真实历史数据 replay，但能在日常提交里快速发现协议、health check 或基础执行路径的明显回归。

### 3. 如需真实历史复测，复用同一份 DB

真实历史复测要避免把下载历史数据和 replay 混成一个数字。建议流程是：

1. 先通过 backtest service 或 storage 同步任务准备 SQLite。
2. 固定同一份 DB、同一段 symbol/interval/session scope。
3. 只改变策略、参数或 worker 版本，再比较 replay 结果。

如果后续重新加入真实链路 benchmark，测试名应明确区分：

- sync benchmark：只覆盖 OpenD 查询和 SQLite 落盘。
- replay benchmark：只覆盖 `RunWithPineWorker`、worker 执行和 Go 撮合。
- end-to-end smoke：只用于发现集成断裂，不用于判断 replay 性能。

## 当前推荐的回归守卫

如果改动会影响共享的 replay 执行路径，例如：

- `pkg/backtest.RunWithPineWorker`
- `pkg/strategy/pineworker`
- worker proto / request / response DTO
- 前端策略设计器当前支持的指标/价格判断/通知/下单/风控图块

优先跑当前可维护的 focused suite：

```bash
go test ./pkg/backtest
go test ./pkg/strategy/pineworker -run Test -cover
go test ./pkg/strategy/pineworker \
  -bench BenchmarkCheckPerformanceGate \
  -run '^$' -benchmem
```

历史上的 synthetic 图块矩阵和真实链路 benchmark 已随 Go Pine runtime 硬切清理。重新建立 performance gate 时，应围绕 PineTS worker 的输入规模、worker pool 并发、gRPC 序列化成本和 Go 撮合结果收集来建基线。

## 结果怎么解释

- 如果 sync 明显慢：先看 OpenD 查询窗口、分页、重试和 SQLite 落盘。
- 如果 replay 明显慢：优先看 `pkg/strategy/pineworker` 的请求/响应大小、worker pool 并发、PineTS 脚本执行耗时、gRPC 序列化成本和 `pkg/backtest/result_collector.go`。
- `trades=0` 不等于性能问题；它只说明这段真实数据下模板没有成交，或 warmup/信号本身不满足。
- `backtest prepare warning: no kline data found for symbol ... 1m before start time` 目前在这条真实链路里只是告警，不会阻止 5m replay 跑完。
- 现有 US.TME 2026-03 diagnostics 已验证：`oneMinuteRowsBeforeStart=0`，因为这条 harness 只同步了 `5m`；同时 `crossoverCount=0`、`crossunderCount=0`，所以当前 `0 trades` 是真实“无信号”，不是 replay 漏单。

## 为什么引入 regular/extended 和重定义 day 之后会变慢

这轮回测语义修正，本质上是在修两类旧错配：

- 旧版 store 读取侧没有真正把 `useExtendedHours` 下沉到数据版本选择，extended replay 仍可能读到只按 regular 口径准备的 higher-period 数据。
- 旧版 day/week/month moving-average requirement 与保护窗口本质上只是 `resolveBarCount` 的固定 bar 数换算；例如 US `5m` 上的日线 MA20，旧语义等价于固定 `20 * 390 / 5 = 1560` 根 bar，而不是“20 个真实 trading day”。

现在的设计变成了：

- SQLite 同一 symbol/interval/rehabType 下区分 `legacy` / `regular` / `extended` 三套 session scope。
- `useExtendedHours` 不只影响 replay 数据版本，也会影响 `1d/1w/1mo` 与 `2h/4h/6h/12h` 的聚合边界。
- indicatorruntime 的 `day/week/month` window 改为按 `TradingPeriodLabelStart` 和 trading profile 来算，US extended 口径下一个 trading day 当前就是 24 小时连续交易日。

性能下降主要来自三层叠加：

- store 侧多了一层 session-scope 选表和 boundary probe，避免 regular/extended 数据串读。
- higher-period 查询多了 session-aware / trading-period aggregation，不能再把 `2h`、`1d`、`1w` 简化成“读现成表”或“固定 bar 数凑够”。
- indicatorruntime 的 `day/week/month` snapshot 不再是固定长度 rolling window；extended warmup 也会同步放大。例如 US `5m` 上 `20 day`，旧 regular 口径只要约 `1560` 根 bar，当前 extended 口径会扩大到约 `5760` 根 bar。

所以这里面有一部分是语义正确性必须支付的成本，不能简单回滚；另一部分才是实现上的额外损耗，值得继续压。

## 历史 Go runtime 热点记录

硬切 PineTS worker 之前，US.TME 真实 replay 曾验证过一类高概率热点：

- `pkg/strategy/indicatorruntime` 的 trading-window moving-average path
- 具体表现是每根 bar 反复构造 day/week/month window 的 indices/values/volumes
- 如果 period key 走字符串格式化，也会放大 `time.Time.Format` 分配

这些记录只用于理解旧性能数据，不再是当前 PineTS worker 的调优入口。当前调优优先看：

- worker 请求 candle 数量、params 数量和 response payload 大小
- PineTS 脚本执行时间与 worker RSS
- worker pool 的并发度、排队时间、超时和重启
- gRPC encode/decode 和 localhost 调用成本
- Go replay pump、bbgo matching 和 result collector 的剩余成本

旧三年 extended-hours 场景的剩余热点仍有参考价值，尤其是：

- `modernc.org/sqlite.(*rows).Next` / `columnText` / `pkg/backtest/internal/storage.scanStoredKLineRow`
- 上游 `github.com/c9s/bbgo/pkg/backtest.(*Exchange).ConsumeKLine` 的 `klineCache` map churn 与 `EmitKLineClosed`

这意味着 PineTS 硬切后，Go 侧仍值得继续压 SQLite 扫描、bbgo replay/matching 结构和结果收集的数据表示；但 Pine 语言执行本身的热点应回到 worker 侧衡量。
