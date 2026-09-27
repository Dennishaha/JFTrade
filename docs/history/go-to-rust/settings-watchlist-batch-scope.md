# Settings、Watchlist 领域对齐批次

本批按 `Go 文件路径:行号:测试名` 复合键核对设置服务与 Futu/watchlist
边界，优先处理 provider 选择回滚、唯一字段所有权、输入校验和速率限制。
证据只引用真实 Rust 测试函数；测试装配或断言范围不同的条目保持
`[~]`/`partial`，不把相近 helper 当成完整覆盖。

| 状态 | Go 测试 | Rust 证据 | 差异结论 | 精确验证 |
| --- | --- | --- | --- | --- |
| [~] | `internal/settings/market_data_test.go:56:TestMarketDataProviderSettingsNormalizeAndApply` | `jftrade-settings::tests::provider_normalization_matches_current_go_defaults` | Rust 覆盖 provider 规范化、默认值、保存和非法输入；缺 Go 的 side-effect callback/idempotent restart 断言。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(provider_normalization_matches_current_go_defaults)'` |
| [~] | `internal/settings/market_data_test.go:203:TestMarketDataProviderRuntimeRollback` | `jftrade-settings::tests::active_failure_rolls_back_but_backtest_failure_never_persists` | Rust 覆盖 activation failure 后 active/backtest 持久值恢复；Go 只测 active provider，错误类型/返回值组合不同。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(active_failure_rolls_back_but_backtest_failure_never_persists)'` |
| [~] | `internal/settings/service_managed_accounts_test.go:13:TestServiceCreateManagedAccountNormalizesClientFields` | `jftrade-settings::managed_account_validation::create_account_clears_client_owned_identity_and_timestamps` | Rust 断言 server-owned id/timestamps 清空和 accountId trim；Go 还检查 backing array 与 fake store 形状。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(create_account_clears_client_owned_identity_and_timestamps)'` |
| [x] | `internal/settings/service_managed_accounts_test.go:111:TestServiceCreateManagedAccountRejectsBlankAccountID` | `jftrade-settings::managed_account_validation::blank_account_id_is_rejected_before_persistence` | 函数级等价：空 accountId 返回同一错误消息，且不会触发持久化。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -E 'test(blank_account_id_is_rejected_before_persistence)'` |
| [~] | `internal/watchlist/futu/source_boundaries_test.go:137:TestFutuSnapshotRemainingProviderRateLimitAndSplitPaths` | `jftrade-integration-futu::test_opend_trade_read_client_burst_and_10th_call_preemption` | Rust 覆盖真实 framed OpenD rate-limit preemption；Go 还覆盖 split、nil provider 与 invalid item。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --test trade_session_gcra_rate_limit_challenge -E 'test(test_opend_trade_read_client_burst_and_10th_call_preemption)'` |
| [x] | `internal/watchlist/quote_preview_boundaries_test.go:37:TestQuoteCacheAndImportHelpersKeepAbsentDataExplicit` | `jftrade-watchlist::tests::test_quote_cache_and_import_helpers_keep_absent_data_explicit` | 已有函数级证据：limit、group trimming/dedup 和 absent data 边界逐项断言。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist -E 'test(test_quote_cache_and_import_helpers_keep_absent_data_explicit)'` |

## 未完成边界

Futu remote watchlist reader 的重名组 ambiguous/cache/fresh 读取、订阅配额满时
delayed fallback、extended-session quote 选择及时区未知值保护，已在 **第九十二批**
逐条核对：`internal/watchlist/**` 31 条 `missing` 全部转为 `[x]`/`partial`（见下节），
其中 delayed fallback 与 extended-session 选择记 `partial`（Rust 由
`security_snapshot_coordinator` / `snapshot_fallback` 承担，缺 watchlist 端口级端到端断言）。
Settings 的并发 security save、listener 失败后完整 rollback 也不能由 provider rollback 证据替代。

## 第九十二批：`internal/watchlist` 全域收口（31 条）

### 范围与分片

`internal/watchlist/**` 剩余 **31 条 `missing`**（5 个文件），分两片：

- P0 Futu 数据源与 provider 边界 14 条（`futu/source_test.go` 11、`futu/source_boundaries_test.go` 3）；
- P1 服务层 quote/导入/预览 17 条（`service_quotes_test.go` 8、`service_test.go` 8、
  `quote_preview_boundaries_test.go` 1）。

### 结果

- 31 条全部给出结论：**2 条 `[x]`/`function_exact` + 29 条 `partial`**，`internal/watchlist/**`
  归零（35 条 = 5 `[x]` + 30 `partial`，0 `missing`）；另把既有 `futu/source_test.go:83`
  从 `partial` 核验升级为 `[x]`（本批共新增 3 条 `[x]`）。
- 全局：4451 = function_exact **1224** + partial 2472 + boundary 541 + module_only 4 +
  missing **210**（前批为 1221 / 2444 / 541 / 4 / 241），Rust 测试 2936 → **2940**。

### 本批生产修复（先红后改）

Go 的 watchlist quote 缓存是**读穿透缓存**，并以 `ChangeQuoteProvider`/`ResetQuoteCache`
对 provider 换代做失效与 in-flight 围栏。Rust 的 `handle_batch_quotes`
（`crates/jftrade-engine/src/product_production_ports_watchlist_quotes.rs`）此前有两个真实缺口：

1. **缓存不生效（每次请求都访问快照源）**：缓存只在“本轮取数无结果”时兜底，未命中即回源。
   修复：请求开始先用 `WATCHLIST_QUOTE_CACHE_TTL`（30s）+ provider generation 预填缓存，
   只对未命中/异代的 id 取数，且只有**本轮真实取到**的 quote 才刷新 `cached_at`
   （避免重复读把 TTL 无限续期）。
2. **provider 换代后旧数据仍可用**：缓存条目没有来源代际，切换 provider 后 30 秒内仍会命中旧
   provider 的报价；切换期间在飞的读取也会把旧 provider 结果写回缓存。
   修复：`WatchlistQuoteCacheEntry` 增加 `provider_generation`（取 `ActiveProviderState` 的
   generation），命中要求与当前代际一致；写回前再次比对代际，代际变化时丢弃在飞结果（Go 的
   in-flight fence）。失败切换（激活回调报错）generation 不变，缓存继续有效（Go 保留缓存语义）。

新增 4 条回归测试（同一文件 `#[cfg(test)] mod tests`）：
`quote_cache_serves_repeat_reads_within_one_provider_generation`、
`provider_switch_drops_quotes_cached_under_the_previous_provider`、
`in_flight_snapshot_does_not_repopulate_the_cache_after_a_provider_switch`、
`rejected_provider_switch_preserves_the_current_quote_cache`；
测试通过 `SecuritySnapshotReadPort` + `SharedTradeReadRuntime` + `ActiveProviderState` +
按固定 schema 初始化的 `WatchlistStore` 组装真实端口，`CountingSnapshotReader` 统计物理读取次数。
探针（改坏→转红→按字节回滚）：摘除缓存预填后第 1、3 条转红；仅摘除缓存读取处的代际校验后第 2 条转红
（第 1 条仍绿）；`shasum` 比对确认回滚为字节级还原，4 条测试恢复全绿。

### 新增 `[x]` 证据

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `service_quotes_test.go:174 TestChangeQuoteProviderRejectsPreviousProviderInflightResults` | `jftrade-engine/src/product_production_ports_watchlist_quotes.rs::tests::in_flight_snapshot_does_not_repopulate_the_cache_after_a_provider_switch` | `[x]`：provider 在快照读取期间完成切换时，旧代际结果不得回填缓存；配套测试断言切换后必须重新读取源而不是命中旧代际缓存。 |
| `service_quotes_test.go:210 TestChangeQuoteProviderFailurePreservesCurrentCache` | `jftrade-engine/src/product_production_ports_watchlist_quotes.rs::tests::rejected_provider_switch_preserves_the_current_quote_cache` | `[x]`：激活失败时 generation 不变，缓存继续命中且不再访问源（源调用次数保持 1）。 |
| `futu/source_test.go:83 TestRemoteMembersKeepBrokerCodeAndSecurityIDAsSeparateAliases` | `jftrade-integration-futu/src/watchlist_reader_tests.rs::tests::watchlist_member_conversion_preserves_canonical_id_and_broker_alias` | `[x]`：canonical instrumentId 与 brokerCode/brokerSecurityId 两个独立别名保持分离且互不相等（既有 `partial` 核验后升级，并补该 Go 行的锚点）。 |

### 保留差异候选（保持 partial 的理由）

- **P0 Futu 数据源 14 条**：Go 在 watchlist 层维护“主源按 symbol 单元素批次 → 配额满时同批次
  delayed fallback → 逐市场权限/未知/OTC 单项隔离”的专属策略；Rust 拆成
  `security_snapshot_coordinator`（HK 20 / 其他 400 分片、限流分类、取消、并发合并）+
  `snapshot_fallback`（延迟 StockScreen 回退）+ `jftrade-broker` 的 symbol-scoped 错误分类，
  没有 Go 那种 watchlist 端口级端到端断言；`source.Source/Status/Error` 字符串投影在 Rust 不存在，
  就绪状态由 `ProviderRuntimeSnapshot(helper_ready/opend_ready/router_ready)` 表达。
- **P1 quote 服务 8 条**：Rust 未实现的两项 Go 能力已在结论中登记——
  ① **single-flight**：并发同 id 请求会各自访问源（Go 只发 1 次并让后来者等待）；
  ② **provider 协商 TTL**：Rust 固定 30s（Go 用 `QuoteCacheTTL()`，fixture 为 15s）；
  另外 Rust 的 batch-quotes **不写 instrument metadata**（Go 会把 name/type 回写基础信息库），
  因此 `ResetQuoteCache` 的“陈旧 provider 不得更新元数据”断言在 Rust 无对应副作用可断言。
- **P1 服务层 8 条**：Go 断言 service 层逐项委托与 `ErrValidation`/`IsConflict` 统一分类，
  Rust 以 `WatchlistError` + `WatchlistStoreError` 状态码 + revision fence 表达；
  导入 preview/commit 的等价断言落在 `jftrade-store-sqlite` 的
  `watchlist_membership_mutations_and_preview_commit_lifecycle` 与
  `concurrent_group_updates_commit_with_revision_fence`，不含 Go 的 fresh-connector 读取要求。
- **`quote_preview_boundaries_test.go:12`**：Rust 入口校验（1–500 条、非法 instrumentId 拒绝）
  已覆盖，single-flight 预约/取消等待未实现（同 P1 缺口）。

### 后续待办（下一个 watchlist 批次候选）

1. **已完成（本批）**：quote single-flight 的并发同标的折叠（`service_quotes_test.go:62`）。
2. **已完成（本批）**：按 provider polling policy 选择 quote TTL（`service_quotes_test.go:117`）；
   instrument metadata 回写（`:145`）仍因 Rust 没有同形副作用而保持 partial。
3. watchlist 端口级的 delayed fallback / 逐市场权限 / 未知与 OTC 单项隔离端到端测试
   （`futu/source_test.go:221`、`:267`、`:316`、`:357`），以及 SG.D05 未知市场时区保护
   （`futu/source_test.go:396`）仍保持 partial，等待 Rust 端口级 seam。

## 第九十三批：Watchlist quote session、single-flight 与 provider TTL（2026-09-27）

本批聚焦 3 条 P1/high 映射，均先以重叠请求、扩展 session 与 TTL 边界回归复现，再修复
`ProductionWatchlistPort` 的真实行为：

- Futu `pre`/`after`/`overnight` session 现在优先投影扩展价，change 仍以 regular
  previous close 计算，volume/turnover 保留 regular snapshot 语义；
- overlapping batch quote 请求在物理 snapshot read 前使用共享 gate，后续请求观察首个
  请求写入的缓存，不再重复访问源；
- Futu 默认缓存 TTL 为 2.5 秒，helper provider（Akshare/Yfinance）按 15 秒 polling
  policy 缓存；provider generation 仍构成缓存命中与迟到写回的 fence。

新增并通过 `batch_quotes_selects_extended_session_price_and_change`、
`overlapping_batch_quotes_share_one_snapshot_read`、
`batch_quote_cache_uses_provider_ttl_before_refetching`（nextest 3/3）；Watchlist 相关
nextest 56/56 通过。对应 `source_test.go:418`、`service_quotes_test.go:62`、`:117`
升级为 `[x]`/`function_exact`，均引用 receipt
`sha256:ae6267da865221f941864dca401b67519a51c0a38b9c0069be36bd4010bad7e3`。

本批未升级 SG.D05 unknown-market timezone、metadata 回写、delayed fallback 或逐市场
permission/unknown/OTC 隔离；这些是真实结构差异，继续保留 partial，不以相邻测试代替。

验证：`cargo fmt --all -- --check`；`env NEXTEST_EXPERIMENTAL_LIBTEST_JSON=1 node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked --message-format libtest-json-plus --no-fail-fast -E 'test(batch_quotes_selects_extended_session_price_and_change) or test(overlapping_batch_quotes_share_one_snapshot_read) or test(batch_quote_cache_uses_provider_ttl_before_refetching)'`（3/3）；`python3 scripts/compatibility/audit_test_parity.py --write-report`（Go 4451、Rust 3380、function_exact 1499、partial 2318、boundary 634、missing 0）；`python3 scripts/compatibility/parity_anchor_reconcile.py`（1791/1745/0/0/46）；`node scripts/check-zero-go.mjs`；`pnpm run check:ai-context`；`git diff --check`。`audit_test_parity.py --strict` 仍真实失败 3927 个历史 evidence/receipt gaps，不宣称严格审计完成。

验证：`cargo fmt --all`；`cargo clippy -p jftrade-engine -p jftrade-integration-futu --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist -p jftrade-engine -p jftrade-integration-futu -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked --no-fail-fast`（**1418 passed / 0 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2940 Rust** / **1224 `[x]`**；
missing 210、partial 2472、boundary 541、module_only 4；0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；
未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（1222 唯一锚点引用，本批 3 条新锚点全部已记账，
未记账 6 条仍为 `internal/settings` 前批基线）。

`pnpm run check:quick` **EXIT=0**（工作区 **2275 passed / 1 skipped**；policy/contracts/target-health/format/clippy/desktop 均通过）。

`pnpm run check:rust` **EXIT=1**，唯一失败项为环境级 `check:rust:policy`（`cargo deny check` → advisories FAILED），保留原始证据不记为通过：
**RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；根 `Cargo.toml` 精确锁定 `=0.23.44`，解除需一次显式的依赖升级批次并复核 `deny.toml`），
另有 8 条 `warning[advisory-not-detected]`（deny.toml 陈旧 ignore）为警告而非错误；
同次运行的 target-health、architecture、production-policy、format:rust:check、clippy 均已通过。
`node scripts/check-zero-go.mjs`（2894 tracked files / 0 release artifact）；`git diff --check` 通过。
