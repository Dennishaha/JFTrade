# Go → Rust 对齐成果摘要

更新时间：2026-09-25。本文是迁移期工作摘要，不替代架构事实、门禁结果或发布资格。

## 当前基线

| 项目 | 数值 | 解释 |
| --- | ---: | --- |
| Go 测试候选 | 4451 | 冻结基线 `go:452dea11` |
| Rust 测试 | 3320 | 数量不代表行为等价 |
| `function_exact` | 1472 | 有真实且唯一的 Rust 测试证据 |
| `partial` | 2343 | 只覆盖部分断言，不能视为完成 |
| `boundary` | 636 | 当前架构边界或没有同形对象 |
| 重复映射 | 0 | 审计脚本结果 |
| Parity 锚点 | 1745 / 1699 / 0 / 0 / 46 | unique / recorded / unrecorded / stale / unknown |

## 已完成的工作

- 建立了 4451 条 Go 测试逐项映射清单，按文件、行号、测试名记录，避免只按同名测试判断。
- `strategy_pine` 的 385 条 `partial` 已逐条复核完；结论仍是 `partial`，没有把聚合测试冒充 exact。
- `api_transport` P1 的 72 条 `partial` 已逐条复核完；结论仍是 `partial`。
- `internal/marketdata` partial 第 1–30 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`，精准 Rust 证据 38/38 通过，没有新增生产修复。
- `internal/marketdata` partial 第 31–60 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`/`boundary`，并新增一个先红后修的 resolver 回归测试，锁定 provider full-window 后再应用公开 limit。
- `internal/marketdata` façade 剩余 34 条已逐项读取 Go 断言并核对 Rust owner；34/34 保持 `partial`，本批受影响 crate nextest 1993/1993 通过。helper provider 的 `snapshot-poll-delayed`/`snapshot-poll-fallback` 状态已由生产 owner 回归测试锁定。
- resolver limit 差异已在 `crates/jftrade-engine/src/product_production_ports_market_data_catalog_futu.rs` 修复：避免 provider 在 CN/SH/SZ 过滤前按公开 limit 截断候选；TTL/singleflight 仍保留为架构边界，不宣称等价。
- 近期真正修改过 Rust 生产代码的批次包括：交易默认市场注入、下单前名义金额回退、市日边界、策略运行时及若干行情/路由边界；这些改动均配有回归测试或兼容性证据。
- 最近的 strategy/API 批次主要是证据审查和文档落账，没有新增 Rust 生产代码，必须与“功能已完成”分开看待。

## 仍未闭环的部分

- `partial` 仍有 2343 条，说明 Go 断言与 Rust 证据尚未达到同层等价。
- `api_transport`、`backtest_calendar`、`assistant` 等领域仍有大量 partial；下一步应优先选择一个真实功能缺口，补 production owner、失败回归测试和实现，再更新清单。
- Watchlist/Futu remote reader、TickCandles/跨交易日 `VolumeDelta` 等历史上已识别为架构能力缺口，不能靠继续增加映射文档闭环。
- `boundary` 不是“通过”，而是明确记录当前 Rust 架构没有对应 Go helper、wrapper 或旧入口。

## 调度收敛规则

- 当前只保留一个持续队列上下文；本批完成后下一片为 `internal/marketdata/subscriptions_test.go` 4 条、`quote_availability_test.go` 2 条，再处理 `lifecycle_boundaries_test.go` 余量。
- 不创建子任务、不创建第二个 heartbeat、不重复复核已经完成的切片。
- 每批先读 Go 实现和 Rust owner；只有发现真实行为差异才先写失败回归测试并修改生产代码。
- 仅证据不足时维持 `partial` 或 `boundary`，不得为了提高数字升级为 `function_exact`。
- 每批只运行对应证据的精准测试，按批次提交；完成后只更新同一个 heartbeat 的下一批范围。

## 最近提交

- `00f89290`：marketdata façade 剩余 34 条逐项复核，补 helper provider polling mode owner 回归测试并修复状态投影。
- `47bf7b1a`：注入配置的默认交易市场并补交易读取测试。
- `917d1534`：API transport P1 partial 第 1–30 条核对。
- `952a8993`：API transport P1 partial 第 31–60 条核对。
- `7fd577c7`：API transport P1 partial 第 61–72 条核对并收尾。
- `15f3f2d1`：strategy/Pine partial 第 281–300 条核对并修复 Pine request/indicator 差异。

`917d1534`、`952a8993`、`7fd577c7` 主要是 parity 清单和批次文档，不应被解读为新增 Rust 功能已经完成；`15f3f2d1` 则同时包含 Pine 生产修复与回归测试。
