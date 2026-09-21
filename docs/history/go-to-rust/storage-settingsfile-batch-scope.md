# SQLite、Storage、Settings File 领域对齐批次

本批逐项核对 5 条涉及 rollback、migration、唯一写入和设置持久化的 Go 测试：
规范化 fallback、atomic replace 失败回滚全部 runtime state、bootstrap/migration
失败回滚、managed account backing array 回滚，以及 Futu integration 持久化。

| Go 测试 | Rust 证据 | 结论 |
| --- | --- | --- |
| `internal/store/settingsfile/normalization_and_persistence_test.go:13` | `settings_file_contracts::test_settings_normalization_handles_fallbacks_and_boundaries` | `[~]`/`partial`：仅 execution fallback/clamp，缺 notification、Pine worker、颜色和 mode 边界 |
| `internal/store/settingsfile/rollback_test.go:13` | `settings_file_contracts::test_failed_setting_saves_rollback_all_runtime_state` | `[~]`/`partial`：Rust 仅验证 appearance 成功保存，缺 10 类设置 atomic replace 失败后的内存/磁盘回滚 |
| `internal/store/settingsfile/rollback_test.go:185` | `settings_file_contracts::test_failed_bootstrap_and_migration_rollback_runtime_state` | `[~]`/`partial`：缺 initial integration、bootstrap/migration replace 失败及文件不存在断言 |
| `internal/store/settingsfile/rollback_test.go:232` | `settings_file_contracts::test_failed_managed_account_crud_rolls_back_backing_array` | `[~]`/`partial`：仅初始空数组，缺 create/update/delete 失败注入和 backing array 不变 |
| `internal/store/settingsfile/store_test.go:83` | `settings_file_contracts::product_corpus_replays_frozen_compatibility_and_preserves_unknown_fields` | `[~]`/`partial`：frozen corpus 覆盖配置字段，缺 SaveIntegration runtime env 不变与 replace 流程 |

验证命令已写入 `manual-test-mappings.json`。本批不把 helper 或 corpus 级证据
升级为完整等价；下一步优先补 SettingsFileStore writer lease 冲突、原子替换
失败和 SQLite migration rollback 的独立 Rust 回归测试。

## 第八十一批：`internal/store` 全域收尾（206 条，存储域归零）

### 范围与结果

- 范围：`internal/store` 剩余 206 条 `missing`（53 个文件），按 P0（连接/协调器与 schema manifest 边界、
  交易执行订单账本与乱序对账）→ P1（settingsfile 持久化/恢复/归一化、watchlist 存储契约）→
  P2（strategy/research/backtest 存储读写、分页与失败边界）顺序分 4 片逐条核对。
- 结果：`missing` 1515 → **1309**（本批结清 206 条）、`partial` 1337 → **1507**、`boundary` 397 → **432**、
  `[x]` 1198 → **1199**。本批 206 条 = **1 `[x]` + 170 partial + 35 boundary**。
- **`internal/store/**` 全域归零**：域内 228 条 = **15 `[x]` + 178 partial + 35 boundary，0 `missing`**。

### 分片执行

- **P0a（56 条）**：`sqliteconn` 26（conn/coordinator/db_api/db_concurrency/maintenance）与 `sqliteschema` 30
  （catalog/schema_boundaries/schema_fault_driver/schema）。证据面为
  `crates/jftrade-store-sqlite/src/schema_manifest_invariants_tests.rs`、
  `crates/jftrade-store-sqlite/tests/{sqlite_query_plan_and_migrations_audit,schema_migrations,backtest_market_data_failure_recovery,backtest_snapshot}.rs`、
  `crates/jftrade-owner-lock/src/lib.rs::exclusive_lock_conflicts_and_file_survives_release` 与
  `crates/jftrade-store-settings-file/tests/settings_file_contracts.rs::read_only_shadow_loads_without_mutating_or_persisting_settings`。
- **P0b（44 条）**：`trading` 执行订单账本、broker 快照对账、乱序收敛、提交安全与启动兼容。证据面为
  `crates/jftrade-store-sqlite/tests/execution_order_store_contracts.rs`（生命周期/持久化、损坏库拒绝、
  预留重放不重试未知提交、预览消费幂等与过期栅栏）、`sqlite_query_plan_and_migrations_audit.rs` 与
  `maintenance_backup_and_rebuild_contracts.rs`。
- **P1（61 条）**：`settingsfile` 33 + `watchlist` 28。证据面为
  `crates/jftrade-store-settings-file/tests/settings_file_contracts.rs`、`crates/jftrade-settings/src/{exchange_calendar,pine_worker,notifications,market_data_provider,appearance,execution,mcp_server,broker,security,onboarding,futu_install}.rs`
  与 `crates/jftrade-store-sqlite/tests/watchlist_store_contracts.rs`。
- **P2（45 条）**：`strategy` 21 + `research` 6 + `backtest` 18。证据面为
  `strategy_definition_store_contracts.rs`、`strategy_runtime_store_contracts.rs`、
  `crates/jftrade-engine/src/{strategy_runtime_activity.rs,product_production_ports_strategy_tests.rs}`、
  `research_preset_store_contracts.rs`、`backtest_run_store_contracts.rs`、`backtest_sync_task_store_contracts.rs`
  与维护用例。

### 新增批准项（1 `[x]`）

- `internal/store/settingsfile/store_test.go:205 TestDefaultExchangeCalendarSettingsUseNYSEAsOnlyDefaultUSRemoteSource`
  → `crates/jftrade-settings/src/exchange_calendar.rs::default_exchange_calendar_settings_use_nyse_as_only_default_us_remote_source`：
  Go 断言 US 策略 `preferred=[nyse_official]`、`enabled=[nyse_official,builtin_rules]`，Rust 同名场景用例断言
  完全相同的两个列表；代码侧已有 `// Parity:` 锚点（未锚定告警保持 193 基线）。

### 新增缺口登记（保留，均为 P2）

- **连接池/读写池/写协调器/读屏障/DSN/语句分类**：Rust 是单 rusqlite 连接 + 互斥 + `WriterLease`
  （`busy_timeout` 10s，无连接池开关），因此 `sqliteconn` 的池配置、读写池分离、排队写可见性、
  读屏障取消与 DSN pragma 拼接等行为按 boundary/partial 记录。
- **查询计划断言缺失**：执行订单事件加载（订单索引）与 watchlist 成员表主键范围断言在 Rust 缺失；
  现有查询计划审计只覆盖 `adk_session_events`。
- **故障注入型回滚断言缺失**：watchlist 导入分阶段失败、成员差量失败、watchlist 删除事务部分失败、
  策略定义快照插入失败、backtest 存储关闭后写失败等 Go 用例依赖注入，Rust 侧写入虽在单事务内完成，
  但缺对应失败注入回归。
- **受管账户默认值与 scope ID**：Go 断言 `broker=futu`、环境 `SIMULATE`、市场 `HK`、展示名与
  `futu|SIMULATE|acc-1|HK` 形式的 scope ID；Rust 只断言空白 accountId 被拒与客户端字段被清除，
  未断言默认画像与 ID 生成格式。
- **Go 中间层不迁移**：执行订单的持久化 worker 队列、分腿快照合并、序号高水位恢复、策略设计的旧 JSON
  迁移与旧运行时迁移在 Rust 无对应实现（策略只存 SQLite、运行时只有 PineTS）。

### 跨批 follow-up 汇总

- P0 无新增；P1 无新增（本批未发现 P0/P1 级行为差异）。P2 = 前批清单 + 本批登记项：
  查询计划断言、故障注入回滚、受管账户默认值与 scope ID。

### 仍未结清（下一批）

- 下一批（第八十二批）范围：按域余量排序的下一块 **`internal/api` 180 条**，先按文件分组 recon 再按
  P0 → P1 → P2 分片；其后：`internal/strategy` 169、`pkg/bbgo` 145、`internal/integration` 141、
  `internal/marketdata` 112、`pkg/futu` 86、`internal/trading` 80、`internal/backtest` 63、`pkg/market` 56，
  直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-store-sqlite -p jftrade-store-settings-file -p jftrade-settings -p jftrade-owner-lock --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite -p jftrade-store-settings-file -p jftrade-settings -p jftrade-owner-lock --all-targets --locked --no-fail-fast`（**228 passed / 0 skipped**；本批引用的 engine 侧用例沿用第 80 批 6 crate 全量运行 2037 passed 的结果，期间无源码改动）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1199 `[x]`**；missing 1309、partial 1507、boundary 432、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`、`pnpm run check:ai-context`。
