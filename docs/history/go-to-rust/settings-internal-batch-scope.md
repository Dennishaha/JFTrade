# Settings 领域（`internal/settings`）对齐批次

本文件记录 Go 基线 `internal/settings`（Service 委托、security/MCP 监听器回滚、
行情 provider 选择与回滚、托管账户与通知访问器）迁移到 Rust `jftrade-settings`
（security/mcp_server/market_data_provider/broker/notifications 等模块内测试与
`tests/settings_failure_propagation.rs`）及 `jftrade-engine`（设置写入端口、
通知测试路由、下单缺省交易环境）的逐测试核对结论。

## 第九十五批：`internal/settings` 全域收口（24 条）

### 范围与分片

`internal/settings/**` 剩余 **24 条 `missing`**（4 个文件），分三片：
P0 settings service 与持久化/失败路径 14 条（`service_test.go` 11、
`persistence_and_mcp_failures_test.go` 3）→ P1 行情数据设置 5 条（`market_data_test.go`）→
P2 托管账户、通知与选项 5 条（`service_managed_accounts_test.go`）。

### 结果

- 24 条全部给出结论：**18 条 `[x]`/`function_exact` + 6 条 `partial`**。
- `internal/settings/**` 归零：28 条 = 19 `[x]` + 9 `partial`，0 `missing`。
- 全局：4451 = function_exact **1259** + partial **2513** + boundary **542** + module_only 4 +
  missing **133**（前批为 1241 / 2507 / 542 / 4 / 157），Rust 测试 2948 → **2962**。
- 锚点对账：`unrecorded` 由 6 条（全部为 `internal/settings` 历史锚点）降到 **0**，
  新增/迁移锚点 14 个唯一 Go 引用（1240 → 1254）。

### 生产修复（2 处真实缺口，先红后改）

1. **回测 provider 必须先 prepare 再原子持久化**（P0，`market_data_provider.rs`）：
   Go 的 `SaveBacktestMarketDataProvider` 在落盘前调用 `OnBacktestProviderChanged`，
   准备失败时不做任何持久化；Rust 原实现先写 store 再 `prepare_backtest`，靠回滚掩盖，
   于是准备回调运行期间持久值已经是新 provider，与 Go「准备前旧值可见」的契约相反。
   修复为 prepare → persist，落盘失败不再触发运行时的隐式改动。
2. **provider 读写 fencing**（P1，`market_data_provider.rs`）：
   Go 用 `marketDataProviderMu`/`backtestProviderMu` 的写锁串行化保存、读锁保护
   `GetActiveMarketDataProvider`，保证「运行时不接受 → 回滚」窗口内读不返回中间值；
   Rust 原先无锁。本批给两个服务加 `Arc<RwLock<()>>`（save 持写锁、active_provider 持读锁），
   并把内部读取拆成 `stored_provider()` 避免重入死锁。

两次探针均改坏 → 转红 → 按字节回滚并 `shasum -a 256` 校验一致（`867394d6…`）：
把回测顺序改回「先持久化后 prepare」→ `backtest_provider_is_prepared_before_atomic_persistence`
转红（prepare 读到的已是 akshare）；去掉 `active_provider` 读锁 →
`reads_wait_for_the_runtime_rollback_window` 转红（读立即返回并破坏后续断言）。

### 本批新增与补强测试（14 条）

- `crates/jftrade-settings/src/market_data_provider.rs`（新增 5 条）：
  `degraded_current_selection_is_reactivated_only_while_degraded`、
  `akshare_selection_is_accepted_and_applied`、
  `backtest_provider_is_prepared_before_atomic_persistence`、
  `persistence_and_rollback_failures_are_reported`、
  `reads_wait_for_the_runtime_rollback_window`（线程 + 通道复刻 Go 的 50ms 阻塞断言）。
- `crates/jftrade-settings/src/security.rs`：新增
  `concurrent_writes_serialize_and_keep_the_newest_password`（阻塞且计数的密码端口证明
  第二次写入读到第一次的 verifier）；把 `service_test.go:232` 锚点从 listener 测试
  移到 `writes_validate_password_port_and_public_access_like_go`，并给
  `web_access_password_boundaries_match_go` 补 `service_managed_accounts_test.go:99` 锚点。
- `crates/jftrade-settings/src/mcp_server.rs`：补强
  `token_reset_returns_secret_once_and_persists_only_its_verifier`（轮换密钥端口 + 两次 reset +
  公开投影不泄漏 token/verifier）；锚点迁移：`:344` 移到
  `writes_validate_like_go_and_never_accept_a_caller_supplied_token`，`:90` 让位给新的
  集成测试，`:84` 挂到快照与密钥两条测试。
- `crates/jftrade-settings/src/broker.rs`：新增 `empty_store_projects_an_empty_broker_surface`
  与 `structured_integration_save_leaves_the_process_environment_untouched`
  （结构化 Config 原样返回 + 进程环境变量前后一致）。
- `crates/jftrade-settings/tests/settings_failure_propagation.rs`（新增文件，4 条）：
  `rollback_and_disabled_fallbacks_preserve_the_stored_projection`、
  `store_and_rollback_failures_propagate_across_settings_writes`、
  `execution_and_security_write_failures_surface_to_callers`、
  `default_mcp_settings_match_the_go_service_defaults`。
- `crates/jftrade-engine/src/product_tests.rs`：新增
  `system_notification_test_route_fails_closed_without_a_publisher`（未装配发布者 →
  503 `SYSTEM_NOTIFICATION_UNAVAILABLE`）。
- `crates/jftrade-engine/src/product_production_ports_execution_order_validation_tests.rs`：新增
  `configured_default_trading_environment_fills_orders_that_omit_it`
  （缺省 REAL 生效、显式 SIMULATE 优先）。

### 新增 `[x]` 锚点（18 条）

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `service_test.go:232 TestSaveSecuritySettingsRejectsInvalidWebPort` | `security.rs::writes_validate_password_port_and_public_access_like_go` | `[x]`：port=80 → InvalidPort（1024..=65535 之外拒绝）。 |
| `service_test.go:242 TestSaveSecuritySettingsRollsBackWhenRuntimeListenerUpdateFails` | `security.rs::listener_failure_rolls_back_password_and_port_together` | `[x]`：监听器失败 → Runtime 错误且端口/校验器一并回滚。 |
| `service_test.go:268 TestMCPServerTokenResetDoesNotLeakAndInvalidatesPreviousToken` | `mcp_server.rs::token_reset_returns_secret_once_and_persists_only_its_verifier`（补强） | `[x]`：两次 reset 的 token/verifier 不同，公开投影不泄漏 token 与哈希。 |
| `service_test.go:318 TestSaveMCPServerSettingsRollsBackWhenListenerUpdateFails` | `mcp_server.rs::listener_failure_rolls_back_the_persisted_settings` | `[x]`：监听器失败 → Runtime 错误且持久记录恢复。 |
| `service_test.go:344 TestSaveMCPServerSettingsValidatesTokenAndPort` | `mcp_server.rs::writes_validate_like_go_and_never_accept_a_caller_supplied_token` | `[x]`：InvalidPort / TokenRequired / InvalidAuthMode 三态。 |
| `service_test.go:361 TestConcurrentSecuritySavesPreserveNewestPasswordAndCallbackOrder` | `security.rs::concurrent_writes_serialize_and_keep_the_newest_password`（新增） | `[x]`：并发保存串行化、最新密码落盘、公开访问顺序正确。 |
| `service_test.go:435 TestDefaultCallbacksReturnEmptyMaps` | `onboarding.rs::default_onboarding_inputs_and_settings_return_empty_values`；`broker.rs::empty_store_projects_an_empty_broker_surface`（新增） | `[x]`：空投影 = 无 onboarding 完成态、无 broker 集成/账户。 |
| `service_test.go:446 TestSaveIntegrationPassesStructuredConfigWithoutChangingRuntimeEnv` | `broker.rs::structured_integration_save_leaves_the_process_environment_untouched`（新增） | `[x]`：结构化 host/apiPort/websocketPort 原样返回、进程环境变量不变。 |
| `market_data_test.go:99 TestMarketDataProviderRetriesDegradedCurrentSelection` | `market_data_provider.rs::degraded_current_selection_is_reactivated_only_while_degraded`（新增） | `[x]`：降级重试、健康幂等、失败保值。 |
| `market_data_test.go:143 TestMarketDataProviderSettingsAcceptAKShare` | `...::akshare_selection_is_accepted_and_applied`（新增） | `[x]`：`" AKSHARE "` → akshare 落盘 + 一次 activate。 |
| `market_data_test.go:161 TestBacktestProviderIsPreparedBeforeAtomicPersistence` | `...::backtest_provider_is_prepared_before_atomic_persistence`（新增 + 顺序修复） | `[x]`：prepare 期间仍读旧值、失败不落盘、成功才落盘。 |
| `market_data_test.go:224 TestMarketDataProviderReportsPersistenceAndRollbackFailures` | `...::persistence_and_rollback_failures_are_reported`（新增） | `[x]`：落盘错误原样上抛；回滚失败信息含 rollback failed。 |
| `market_data_test.go:252 TestMarketDataProviderReadsWaitForRuntimeRollback` | `...::reads_wait_for_the_runtime_rollback_window`（新增 + 加锁修复） | `[x]`：副作用进行中读被阻塞，回滚后读回旧 provider。 |
| `service_managed_accounts_test.go:58 TestServiceSystemNotificationTestUsesNarrowPublisherAndFailsClosed` | `product_tests.rs::system_notification_test_route_fails_closed_without_a_publisher`（新增）；`product_tests.rs::product_server_persists_ui_settings_and_reports_actual_port` | `[x]`：发布者存在时返回 event+delivery，缺失时 503 fail-closed。 |
| `service_managed_accounts_test.go:84 TestServiceDefaultMCPStatusAndTokenGeneration` | `mcp_server.rs::snapshot_normalizes_public_settings_and_reports_unowned_listener_stopped`；`...::system_secret_uses_go_compatible_token_and_argon2id_verifier` | `[x]`：endpoint 由存储端口派生；token 前缀 `jft_mcp_`、定长 51、argon2id 校验器。 |
| `service_managed_accounts_test.go:99 TestValidateWebAccessPasswordBoundaries` | `security.rs::web_access_password_boundaries_match_go` | `[x]`：15 字符下限、1024 字节上限、400 个「界」拒绝。 |
| `service_managed_accounts_test.go:120 TestServiceOptionsCaptureBrokerDescriptorAndDefaultTradingEnvironment` | `product_production_ports_execution_order_validation_tests.rs::configured_default_trading_environment_fills_orders_that_omit_it`（新增）；`jftrade-integration-futu::broker_descriptor_matches_current_go_wire_fixture` | `[x]`：descriptor 由 wire fixture 锁定；缺省 REAL 生效、显式值优先。 |
| `persistence_and_mcp_failures_test.go:90 TestServiceRollsBackMCPOnSaveFailure` | `tests/settings_failure_propagation.rs::store_and_rollback_failures_propagate_across_settings_writes`（新增） | `[x]`：MCP 回滚失败、token reset 落盘失败、pine/calendar/integration 落盘失败逐项上抛。 |

### 保留差异候选（保持 `partial` 的理由）

- `service_test.go:153 TestSaveSettingsTriggersSideEffects`：Go 的 `SideEffects` 回调结构在 Rust
  不存在，save() 直接返回规范化投影由 engine 消费；等价部分有 execution/pine
  归一化与 engine 落盘断言（含 `$argon2id$v=19$m=65536,t=3,p=1$`），但无「回调收到同一结构体」断言。
- `service_test.go:480 TestServiceDelegatesGettersAndSimpleSavers`：Rust 拆分为各领域服务，
  无集中式 getter/saver 委托点；appearance/execution/broker 三条测试覆盖其中三块。
- `service_test.go:565 TestServiceDelegatesProvidersAndLifecycle`：托管账户 create/update/delete
  已覆盖；**边界**是 `EnsureBootstrap`——Rust 无 bootstrap 端口，settings-file 缺失文件按空文档处理，
  不再写 bootstrap 文件（bind 由 engine/desktop 配置提供）。
- `service_managed_accounts_test.go:35 TestServiceNotificationAndMCPStatusAccessors`：通知归一化与
  MCP 快照成对投影已覆盖，但无集中式 accessor 或保存返回值双向对照断言。
- `persistence_and_mcp_failures_test.go:55 TestServiceReportsPersistenceAndMCPFailures`：execution/security
  错误上抛与 MCP 默认投影（6697/token）已覆盖；**边界**是 `ErrMCPServerStoreUnavailable`——
  Rust 服务构造即要求 `Arc<dyn Store>`，「无 store」在类型层面不可表达。
- `persistence_and_mcp_failures_test.go:125 TestServicePreservesSecurityAndMCPFallbacks`：
  PasswordRequired、RuntimeRollback 文案、禁用更新保留 port/authMode、密钥生成失败已覆盖；
  **边界**是无 store 的内部 helper `saveMCPServerSettingsLocked` 无对应分支。

### 后续待办

- 全量清单仍余 **133 条 missing**，按域余量下一批为 `cmd/generate-futu-proto`（18）→
  `internal/system`（14）→ `cmd/generate-pineworker-proto`（12）→ `internal/live`（12）→
  `internal/productfeatures`（12）→ `pkg/observability`（10）→ `cmd/internal`（9）→
  `internal/pineworkerassets`（7，锚点例外）→ `internal/desktop`（6）→ `internal/retry`（6）。
- 未锚定 `[x]` 196 条与 34 条 stale anchor 仍是历史基线，需要按域逐批补锚点或改入口。
- `internal/settings` 的 9 条 `partial` 中，4 条属于「Rust 结构不同（无回调/no-store 状态）」，
  若未来 engine 引入集中式 settings facade，可重新评估升级为 `[x]`。

验证：`cargo fmt --all -- --check`；`cargo clippy -p jftrade-settings -p jftrade-engine -p jftrade-api -p jftrade-store-sqlite --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -p jftrade-engine -p jftrade-api -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**2007 passed / 0 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2962 Rust** / **1259 `[x]`**；missing 133、partial 2513、boundary 542、module_only 4；`OK: 1259 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors 1254；already recorded 1165、**unrecorded 0**、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0，六组 replay 全通过）；
`node scripts/check-zero-go.mjs`（2896 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`。

`pnpm run check:quick` **EXIT=0**（受影响计划 **1809 passed / 0 skipped**；target-health、generated、route contracts、web contract 与 desktop 检查全通过）。

`pnpm run check:rust` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，保留原始证据不记为通过：
`check:rust:policy` 的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；
根 `Cargo.toml` 精确锁定 `=0.23.44`），另有 8 条 `warning[advisory-not-detected]`（陈旧 ignore）。
同一次运行中 target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 均通过；本批未改 `Cargo.lock` / `deny.toml`。
