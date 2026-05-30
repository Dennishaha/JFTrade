# 回测性能排查

本文只回答一类问题：回测为什么慢，以及应该先查 sync 还是 replay。

## 先分两段，不要混着看

- sync：`pkg/backtest/internal/storage` 从 OpenD 拉历史 K 线并落 SQLite。
- replay：`pkg/backtest.Run` 把 SQLite 里的 K 线回放给 bbgo backtest.Exchange 和 DSL runtime。

如果不先把两段拆开，只看一份混合 benchmark，容易把 OpenD/SQLite 成本和策略运行时成本混在一起。

## 当前推荐的真实链路 profiling 入口

仓库里已经有真实链路 harness：`pkg/backtest/real_chain_profile_test.go`。

它的用途是：

- 用真实 OpenD 拉 US.TME 的 2026-03 K 线
- 用前端双均线模板的真实 DSL 语义回放
- 支持一次性全链路 test
- 支持复用已有 SQLite，只测 replay benchmark
- 支持单独诊断“0 trades 是无信号，还是 runtime 漏单”以及 `1m` prepare warning 的来源

### 1. 先跑一次全链路

```bash
JFTRADE_REAL_CHAIN_PROFILE=1 \
JFTRADE_REAL_CHAIN_DB_PATH=/tmp/jftrade-real-us-tme-202603.db \
go test ./pkg/backtest -run '^TestRealUSMarch2026DoubleMATemplateProfile$' -v
```

重点看两段日志：

- `real-chain sync profile`：确认 syncRange、rows、completedBatches、retries
- `real-chain run profile`：确认 replay duration、candles、trades、runtimeErrors

### 2. 再复用同一个 DB，只测 replay

```bash
JFTRADE_REAL_CHAIN_PROFILE=1 \
JFTRADE_REAL_CHAIN_DB_PATH=/tmp/jftrade-real-us-tme-202603.db \
go test ./pkg/backtest -run '^$' \
  -bench '^BenchmarkRealUSMarch2026DoubleMATemplateReplay$' \
  -benchtime 1x -benchmem \
  -cpuprofile /tmp/jftrade-real-us-tme-replay.cpu.out \
  -memprofile /tmp/jftrade-real-us-tme-replay.mem.out
```

这样 replay benchmark 就不会再把 sync setup 混进去。

### 2b. 如果要复测“已保存双均线策略 + 三年 extended-hours”

`pkg/backtest/real_chain_profile_test.go` 还新增了这组场景：

- `TestRealUSTME2023To2026SavedDoubleMAStrategyProfileExtended`
- `BenchmarkRealUSTME2023To2026SavedDoubleMAStrategyReplayExtended`

默认配置是：

- symbol：`US.TME`
- interval：`5m`
- replay range：`2023-01-01` 到 `2026-01-01`
- `useExtendedHours=true`
- 策略定义：默认从工作区 `var/jftrade-api/strategy-runtime.db` 读取 `dsl-double-moving-average`

如需覆盖默认定义库或 definition id，可额外传：

- `JFTRADE_REAL_CHAIN_STRATEGY_DB_PATH`
- `JFTRADE_REAL_CHAIN_STRATEGY_DEFINITION_ID`

对应 replay-only benchmark 命令：

```bash
JFTRADE_REAL_CHAIN_PROFILE=1 \
JFTRADE_REAL_CHAIN_DB_PATH=/tmp/jftrade-real-us-tme-202301-202601-extended.db \
go test ./pkg/backtest -run '^$' \
  -bench '^BenchmarkRealUSTME2023To2026SavedDoubleMAStrategyReplayExtended$' \
  -benchtime 1x -benchmem
```

### 3. 如果要区分“没信号”还是“跑漏了”，再跑 diagnostics

```bash
JFTRADE_REAL_CHAIN_PROFILE=1 \
JFTRADE_REAL_CHAIN_DB_PATH=/tmp/jftrade-real-us-tme-202603.db \
go test ./pkg/backtest -run '^TestRealUSMarch2026DoubleMATemplateDiagnostics$' -v
```

这条诊断会额外记录：

- `oneMinuteRowsBeforeStart`：确认 `backtest prepare warning` 是不是因为当前真实库压根没有 `1m` 数据
- `crossoverCount` / `crossunderCount`：确认这段真实数据里 5/20 day 双均线到底有没有信号

## 当前推荐的 synthetic 图块回归守卫

如果改动会影响共享的 replay 执行路径，例如：

- `pkg/backtest.Run`
- `pkg/strategy/dslruntime`
- `pkg/strategy/indicatorruntime`
- 前端策略设计器当前支持的指标/价格判断/通知/下单/风控图块

优先再跑一套 synthetic 图块矩阵，避免只盯着单一双均线场景：

```bash
go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkCasesSmoke$'

go test ./pkg/backtest -run '^$' \
  -bench '^BenchmarkRunExecutesStrategyBlockMatrix$' \
  -benchtime 1x -benchmem

JFTRADE_UPDATE_STRATEGY_BLOCK_BASELINE=1 \
  go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkBaseline$' -count 1

JFTRADE_ENFORCE_STRATEGY_BLOCK_BASELINE=1 \
  go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkBaseline$' -count 1
```

其中 baseline 会落盘到 `pkg/backtest/testdata/strategy_block_benchmark_baseline.json`。
当前 compare 模式按每个 case 取 `testing.Benchmark` 5 次采样，并使用其中的保守上界再用以下阈值判定回归：

- `ns/op` 不超过基线的 `1.40x`
- `B/op` 不超过基线的 `1.15x`
- `allocs/op` 不超过基线的 `1.10x`

如果希望把这套门槛挂进一个手动触发的 job，现在可以直接用 [.github/workflows/backtest-performance-gate.yml](.github/workflows/backtest-performance-gate.yml)。

说明：

- 它只提供 `workflow_dispatch`，不会自动在每个 PR 上跑。
- `runs-on` 固定为 `self-hosted + macOS + ARM64`，因为当前 baseline 来自 Apple Silicon，本地真实 protect 复测还依赖 runner 自己持有的 SQLite DB。
- `baseline_mode` 支持 `skip` / `enforce` / `update`。
- `run_real_tme=true` 时会追加跑 `BenchmarkRealUSTME2023To2026ProtectSessionReplayExtended`，并从 `real_chain_db_path` 读取本机 DB。
- job 会把 smoke、protect matrix、baseline compare/update、真实 TME protect benchmark 输出和 baseline JSON 一起上传成 artifact。
- 本地验证时不要把 `JFTRADE_UPDATE_STRATEGY_BLOCK_BASELINE=1 ... && JFTRADE_ENFORCE_STRATEGY_BLOCK_BASELINE=1 ...` 串成一条命令；第二段 compare 会跑在第一段长时间矩阵压测后的热机状态里，容易把瞬时 CPU 偏差也带进结果。手动 workflow 也是按 `baseline_mode` 分开跑的。

这套矩阵在 `pkg/backtest/strategy_block_benchmark_test.go`，当前覆盖：

- `movingAverage`（含 `hour` timeUnit，专门压 time-bound window path）
- `rsi`
- `macd`
- `kdj`
- `bollinger`
- `atr`
- `cci`
- `williams_r`
- `ifCloseAbove` / `ifCloseBelow`
- `log` / `notify`
- `placeOrder`
- `stopLoss` / `takeProfit` / `trailingStop`

说明：

- 它故意固定在 synthetic `US.AAPL / 1m / useExtendedHours=true` 连续输入上，避免把 US regular session filter 的时段缺口混进图块 replay 基线。
- 指标型场景加了短路保护，warmup 阶段不会因为快照尚未就绪把 benchmark 污染成 runtime error。
- 已废弃的 `codeBlock` 没有单独列入矩阵，因为当前 DSL 生成器会把它降级成普通 `log` 语句，它不再是主 authoring path。

当前这套矩阵在 Apple A18 Pro 的基线量级以 `pkg/backtest/testdata/strategy_block_benchmark_baseline.json` 为准，当前一轮参考值约为：

- `moving_average_windowed`：约 `10.36ms/op`、`2.96MB/op`
- `macd_momentum`：约 `14.44ms/op`、`3.77MB/op`
- `rsi_reversion`：约 `10.40ms/op`、`3.54MB/op`
- `bollinger_reversion`：约 `10.90ms/op`、`3.44MB/op`
- `cci_reversion`：约 `11.50ms/op`、`3.47MB/op`
- `kdj_reversion`：约 `10.31ms/op`、`4.20MB/op`
- `williamsr_reversion`：约 `12.21ms/op`、`3.45MB/op`
- `atr_volatility`：约 `10.72ms/op`、`2.95MB/op`
- `breakout_notify`：约 `16.79ms/op`、`4.07MB/op`
- `mean_reversion_price`：约 `8.84ms/op`、`4.12MB/op`
- `protect_session_risk`：约 `43.77ms/op`、`13.64MB/op`

从这个矩阵看，当前最该盯的共享热点不是普通价格判断块，而是风险保护块：如果后续改动 `protect` / risk snapshot / session-aware window 逻辑，优先比较 `protect_session_risk` 这条基线有没有明显回升。注意这里落盘的是保守 gate 值，不等于日常一轮直跑的典型值；这轮直接跑 `BenchmarkRunExecutesStrategyBlockMatrix/protect_session_risk -benchtime 3x` 时，量级约为 `35.19ms/op`、`13.57MB/op`、`24.9万 allocs/op`，而持久化 gate 当前按保守上界记为约 `43.77ms/op`、`13.64MB/op`。

## 结果怎么解释

- 如果 sync 明显慢：先看 OpenD 查询窗口、分页、重试和 SQLite 落盘。
- 如果 replay 明显慢：优先看 `pkg/strategy/dslruntime`、`pkg/strategy/indicatorruntime`、`pkg/backtest/result_collector.go`。
- `trades=0` 不等于性能问题；它只说明这段真实数据下模板没有成交，或 warmup/信号本身不满足。
- `backtest prepare warning: no kline data found for symbol ... 1m before start time` 目前在这条真实链路里只是告警，不会阻止 5m replay 跑完。
- 现有 US.TME 2026-03 diagnostics 已验证：`oneMinuteRowsBeforeStart=0`，因为这条 harness 只同步了 `5m`；同时 `crossoverCount=0`、`crossunderCount=0`，所以当前 `0 trades` 是真实“无信号”，不是 replay 漏单。

## 为什么引入 regular/extended 和重定义 day 之后会变慢

这轮回测语义修正，本质上是在修两类旧错配：

- 旧版 store 读取侧没有真正把 `useExtendedHours` 下沉到数据版本选择，extended replay 仍可能读到只按 regular 口径准备的 higher-period 数据。
- 旧版 `ma(..., day/week/month)`、`stopLoss(..., day/week/month)` 本质上只是 `resolveBarCount` 的固定 bar 数换算；例如 US `5m` 上的 `20 day`，旧语义等价于固定 `20 * 390 / 5 = 1560` 根 bar，而不是“20 个真实 trading day”。

现在的设计变成了：

- SQLite 同一 symbol/interval/rehabType 下区分 `legacy` / `regular` / `extended` 三套 session scope。
- `useExtendedHours` 不只影响 replay 数据版本，也会影响 `1d/1w/1mo` 与 `2h/4h/6h/12h` 的聚合边界。
- indicatorruntime 的 `day/week/month` window 改为按 `TradingPeriodLabelStart` 和 trading profile 来算，US extended 口径下一个 trading day 当前就是 24 小时连续交易日。

性能下降主要来自三层叠加：

- store 侧多了一层 session-scope 选表和 boundary probe，避免 regular/extended 数据串读。
- higher-period 查询多了 session-aware / trading-period aggregation，不能再把 `2h`、`1d`、`1w` 简化成“读现成表”或“固定 bar 数凑够”。
- indicatorruntime 的 `day/week/month` snapshot 不再是固定长度 rolling window；extended warmup 也会同步放大。例如 US `5m` 上 `20 day`，旧 regular 口径只要约 `1560` 根 bar，当前 extended 口径会扩大到约 `5760` 根 bar。

所以这里面有一部分是语义正确性必须支付的成本，不能简单回滚；另一部分才是实现上的额外损耗，值得继续压。

## 已验证过的一类 replay 热点

在 US.TME 2026-03 + 双均线模板 + `ma(..., day)` 的真实 replay 上，已经验证过一类高概率热点：

- `pkg/strategy/indicatorruntime` 的 trading-window moving-average path
- 具体表现是每根 bar 反复构造 day/week/month window 的 indices/values/volumes
- 如果 period key 走字符串格式化，也会放大 `time.Time.Format` 分配

当前实现已经做了两层减压：

- moving-average trading-window 复用 snapshot cache buffer，避免反复新建 indices/values/volumes
- trading-period label start 改成直接按本地交易日推导，不再让 replay path 依赖字符串 key 往返
- 在此基础上，trading-window MA 主路径又进一步改成 online aggregation：`SMA/MA/BOLL`、`LWMA`、`VWMA` 现在在倒序扫描 `endTimes` 时直接聚合，不再构造 `selected []int` 或继续物化 values/volumes；`EMA/EXPMA`、`SMMA`、`TMA`、`HMA` 仍保留 materialize fallback 以降低语义风险
- 在 online aggregation 之上，`current` / `previous` snapshot 又进一步合成了单趟扫描，避免为同一根 bar 的 snapshot 计算重复调用一遍 `TradingPeriodLabelStart`
- 在单趟 snapshot 扫描之上，runtime 又开始在 `push()` 时增量维护 `day/week/month` 的 trading-period label key，让 fast/slow 两个 `ma(..., day)` 在 snapshot 时直接复用，不再重复调用 `TradingPeriodLabelStart`
- `indicatorRuntime` 的核心 price/session/label 序列现在会按 `seriesLimit` 预分配，减少 replay 期间 append 扩容带来的额外 alloc

如果后续 replay 再次回升，优先回看：

- `pkg/strategy/indicatorruntime/indicator_runtime.go`
- `pkg/futu/trading_profile.go`

## 真实 TME protect 复测

如果要在已下载的 `US.TME / 2023-01-01 到 2026-01-01 / 5m / extended-hours` 数据上复测 protect session 路径，现在可以直接跑：

```bash
JFTRADE_REAL_CHAIN_PROFILE=1 \
JFTRADE_REAL_CHAIN_DB_PATH=/tmp/jftrade-real-us-tme-202301-202601-extended.db \
go test ./pkg/backtest -run '^$' \
  -bench '^BenchmarkRealUSTME2023To2026ProtectSessionReplayExtended$' \
  -benchtime 1x -benchmem
```

对应场景实现在 `pkg/backtest/real_chain_profile_test.go`，它复用本地 SQLite DB，只测 replay，不重新下载。

当前这条真实 protect 场景在 Apple A18 Pro 上最新一轮 one-shot 复测量级约为：

- 约 `2.60s/op`
- 约 `743MB/op`
- 约 `1653万 allocs/op`

这轮复测说明新一刀主要打在 `getPosition` / runtime 内部诊断日志相关分配上：相对前一版约 `1.15GB/op`、`2620万 allocs/op`，内存和分配显著下降，但 `ns/op` 仍会受宿主机状态影响，建议以后以手动 workflow artifact 和同机多次结果一起判断，而不是只看单次 wall-clock。

## 更长范围场景的剩余热点

对“US.TME / 2023-01-01 到 2026-01-01 / 5m / extended-hours / 已保存 `dsl-double-moving-average`”这条真实 replay，当前量级大约是：

- 约 `5.16s/op` 到 `5.23s/op`
- 约 `323.8MB/op`
- 约 `793.96万 allocs/op`

这条场景已经加过一刀本地优化：`pkg/backtest.Run` 在单会话 replay 上优先走 direct-stream fast path，绕开 `QueryKLinesCh` 的 goroutine + channel 逐根转发。复用同一份已同步 DB、连续 3 次 replay-only benchmark 复测，量级从优化前约 `5.87s/op` 压到约 `5.16s/op` 到 `5.23s/op`。

优化后剩余热点主要是：

- `modernc.org/sqlite.(*rows).Next` / `columnText` / `pkg/backtest/internal/storage.scanStoredKLineRow`
- 上游 `github.com/c9s/bbgo/pkg/backtest.(*Exchange).ConsumeKLine` 的 `klineCache` map churn 与 `EmitKLineClosed`
- `pkg/strategy/indicatorruntime` 的 `tradingWindowMovingAverageState.push` 与 `calculateTradingWindowMovingAverageSnapshotFromKeys`

这意味着这条三年 extended-hours 场景里，下一轮大收益大概率不再是“小范围本地改一两个 helper”，而是要继续压 SQLite 扫描路径、考虑结果收集的数据表示，或者直接处理上游 bbgo 的 replay/matching 结构。

## 当前这一刀继续优化打掉了什么

这次没有回退任何 session/day 语义，而是只压 store 读路径里的一个实现性回归：session-scope 选表时，原来每个候选表都要分别做两次 exact boundary 查询来确认请求窗口左右端点是否命中；现在合并成了一次 `COUNT(DISTINCT end_time)` 查询。

Apple A18 Pro 本地复测量级：

- `BenchmarkFutuKLineStoreQueryBackwardSessionAwareTwoHour`：约 `1.19ms/op` 降到约 `0.63ms/op`，`325KB/op -> 324KB/op`，`6489 allocs/op -> 6458 allocs/op`
- `BenchmarkRunExecutesIndicatorHeavyDSLBacktest`：约 `11.02ms/op` 降到约 `10.56ms/op`，`2.99MB/op` 基本持平，`73579 allocs/op -> 73553 allocs/op`

这说明当前确实有一部分 slowdown 来自实现上的重复 probe，而不是新语义本身；但 end-to-end 只拿回了几百分之几，也说明更大的账仍在 SQLite scan、bbgo replay 和 trading-period 指标路径上。

## 一次真实复测的量级参考

同一份持久 DB、同一条真实 benchmark，在这次优化前后大致量级如下：

- 优化前：约 `1.02s/op`、`248MB/op`、`5381944 allocs/op`
- 第一轮优化后：约 `0.80s/op`、`35.5MB/op`、`96741 allocs/op`
- direct aggregation 继续下压后：约 `0.78s/op`、`14.9MB/op`、`85416 allocs/op`
- online aggregation 再继续下压后：约 `0.80s/op`、`4.49MB/op`、`83835 allocs/op`
- current/previous 单趟 snapshot 扫描后：约 `0.39s/op`、`4.49MB/op`、`83847 allocs/op`
- runtime 增量 label key + series 预分配后：约 `0.24s/op`、`4.46MB/op`、`83777 allocs/op`
- 已保存双均线策略 + 2023-2026 extended-hours + direct-stream fast path 后：约 `5.16s/op` 到 `5.23s/op`、`323.8MB/op`、`793.96万 allocs/op`

这说明在这条真实链路里，主要瓶颈确实是 replay 的 trading-window 指标路径，不是 sync。