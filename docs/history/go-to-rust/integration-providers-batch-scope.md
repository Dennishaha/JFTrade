# 外部数据集成域（internal/integration）对齐批次

本域覆盖 `internal/integration` 下的外部行情/研究提供商：`akshare`（A 股/港股/中国叶市场，
经 Python sidecar）与 `yfinance`（Yahoo Finance，经 sidecar）。两者的 Go 实现由
client + conversion + provider 三层组成；Rust 侧对应 `crates/jftrade-integration-marketdata-helper`
（sidecar 进程/HTTP 客户端/DTO）、`crates/jftrade-engine` 的研究与行情端口投影、
`crates/jftrade-integration-calendar`（日历/宏观）与 `crates/jftrade-marketdata`（标的归一）。

## 第八十五批：`internal/integration` 全域收尾（141 条，外部数据集成域归零）

### 范围与结果

- 范围：`internal/integration` 剩余 141 条 `missing`（18 个文件），按 P0（client/transport 与
  conversion 边界）→ P1（akshare provider 语义：公司研究/排名行业/日历宏观/新闻公司行动/筛选/指数成分）
  → P2（yfinance provider 与转换收尾）顺序分 3 片逐条核对。
- 结果：`missing` 815 → **674**（本批结清 141 条）、`partial` 1902 → **2043**、
  `boundary` 保持 526、`[x]` 保持 **1204**。本批 141 条 = **0 `[x]` + 141 partial**。
- **`internal/integration/**` 全域归零**：域内 194 条 = **29 `[x]`（前批）+ 143 partial + 22 boundary，
  0 `missing`**。

### 分片执行

- **P0（34 条，客户端与边界）**：`akshare/boundaries_test.go` 14、`akshare/client_index_constituents_test.go` 2、
  `akshare/client_news_actions_test.go` 6、`yfinance/client_test.go` 12。
  证据面：`crates/jftrade-integration-marketdata-helper/src/{client,dto,provider,process}.rs`
  （loopback/弱 token 拒绝、健康契约与 warmup 错误名、就绪重试、DTO snake_case 与数值有限性）与
  engine 端口错误映射（`calendar_and_macro_propagate_capability_and_busy_errors`、
  `test_non_futu_helper_candles_failure_uses_generic_market_code`、
  `test_research_screen_route_uses_production_helper_and_preserves_upstream_rate_limit`、
  `research_screen_leaf_maps_invalid_provider_rows_to_bad_gateway`）。
- **P1（51 条，akshare provider）**：`provider_test.go` 8、`provider_company_research_test.go` 12、
  `provider_rankings_industries_test.go` 8、`provider_calendar_macro_test.go` 6、
  `provider_news_actions_test.go` 6、`provider_screen_test.go` 6、`provider_index_constituents_test.go` 5。
  证据面：`product_production_ports_research_company_tests.rs`（概况/财报/分析师/持股投影）、
  `product_production_ports_research_market_tests.rs`（排名/行业 limit 归一）、
  `product_production_ports_research_calendar_tests.rs`（日历/宏观 wire 与错误传播）、
  `product_production_ports_market_data_news_tests.rs` 与
  `product_market_data_news_{search,actions}_read_tests.rs`（新闻/公司行动）、
  `product_research_screen_write_port_tests.rs` 与 `tests/research_screens_compatibility.rs`（筛选）、
  `product_production_ports_market_index_constituents_tests.rs`（指数成分）、
  `crates/jftrade-marketdata/src/catalog_tests.rs`（标的归一）与 `crates/jftrade-research/src/catalog.rs`。
- **P2（56 条，yfinance 收尾）**：`conversion_test.go` 18、`provider_test.go` 11、
  `provider_company_research_test.go` 8、`provider_news_actions_test.go` 6、`provider_screen_test.go` 5、
  `provider_rankings_test.go` 4、`client_news_actions_test.go` 4。
  证据面：`crates/jftrade-engine/src/product_market_data_quote_read_tests.rs`（快照盘前/盘后契约）、
  `product_market_data_candle_pagination_tests.rs`（K 线 session 分类与分页边界）、
  `crates/jftrade-integration-futu/src/basic_quote_tick.rs`（扩展时段/高精度成交量/非有限价）、
  `tests/market_data_production_compatibility.rs`（K 线转换与 helper 失败码）与
  `crates/jftrade-marketdata/src/catalog_tests.rs`（市场/别名归一）。

### 为什么本批没有新的 `[x]`

Go 的 `akshare`/`yfinance` 集成把“HTTP 客户端 + 字段转换 + provider 语义”全部实现在 Go 进程内；
Rust 把这些职责拆成 Python sidecar（`crates/jftrade-integration-marketdata-helper` 只保留进程
管理、健康契约、重试与 DTO 校验）加 engine 端口的目录/投影层。因此 Go 的
`decodeResponse`、`NewClient`、`convertSnapshot`、`convertCandles` 等函数在 Rust 没有逐函数对应物：
解码与转换为 Python 侧实现，Rust 只对“错误分类、能力协商、身份回显、分页边界、字段投影”负责。
逐条比对后没有任何一条断言与 Rust 用例逐项一致，故本批全部登记为 `partial`
（并逐条写清 Rust 已覆盖什么、差异为何），而不是把相近用例硬升级为 `[x]`。

### 保留差异与补测候选（P2）

- **客户端整体超时预算**：Go `yfinance/client_test.go:192` 断言超时覆盖所有重试尝试；Rust 只有
  就绪重试与 HTTP 客户端超时，未断言“整体预算”语义。
- **响应体大小上限**：Go `TestClientRejectsMalformedEmptyTrailingAndOversizedResponses` 中的超大响应；
  Rust 的大小守卫在帧/进程层，helper HTTP 客户端未单独断言。
- **批量分块**：Go `TestProviderChunksBatchSnapshotsAtContractLimit` 断言批次切分；Rust 的批量快照
  用例覆盖缓存复用，未断言分块大小常量。
- **akshare 复权/session 白名单**：Go 断言日线复权能力与扩展时段拒绝；Rust 的能力集合由
  adapter bindings 矩阵与市场目录表达，缺独立断言。
- **日历驱动的盘后窗口**：Go 用日历挑选最近已收盘时段并处理陈旧/越窗报价；Rust 由
  `basic_quote_tick` 的扩展时段窗口用例覆盖一部分，跨市场矩阵未全量断言。

### 仍未结清（下一批）

- 下一批（第八十六批）范围：按域余量排序的下一块 **`internal/marketdata` 112 条**，先按文件分组
  recon 再按 P0 → P1 → P2 分片；其后：`pkg/futu` 86、`internal/trading` 80、`internal/backtest` 63、
  `pkg/market` 56、`internal/marketdataassets` 36、`internal/watchlist` 31、`pkg/broker` 29、
  `pkg/researchscreen` 24、`internal/settings` 24，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-research --all-targets --locked --no-fail-fast`（**28 passed / 0 skipped**；本批只改文档与清单，未改任何 crate 代码）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1204 `[x]`**；missing 674、partial 2043、boundary 526、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`（EXIT=0）、`node scripts/check-zero-go.mjs`（2890 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（EXIT=0）、`pnpm run check:ai-context`。

### 2026-09-26 parity baseline correction（历史批次不回写）

本批 integration/provider 的逐域结论仍按原文保留；上方 2925 Rust / 1204 `[x]` 仅是该批完成时快照。后续 provider probe、strategy activity、workflow invalid-input 与 auth ABA evidence 已合并到当前工作树，最新全局基线为 Go 4451、Rust 3366、`function_exact` 1491、`partial` 2332、`boundary` 628，Parity 锚点 1780/1734/0/0/46。该基线更新不改变本文件关于 sidecar 解码/转换职责边界的结论。
