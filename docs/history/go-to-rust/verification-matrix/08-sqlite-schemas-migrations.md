# 领域 8：九个 SQLite 数据库演进与兼容

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.8 领域 8：九个 SQLite 数据库演进与兼容（9 SQLite Schemas & Migrations）

#### 2.8.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `internal/store/sqliteschema/catalog.go:27-35`
  - `internal/store/sqliteschema/schema.go:127`
- **关键符号**: `Database*`, `*Version`, `IncompatibleError`
- **历史行为**:
  Go 基线在启动时若检测到本地 SQLite 数据库的元数据版本与代码期望版本不一致，直接抛出 `IncompatibleError` 报错退出。Go 没有实现任何运行时自动在线 DDL 迁移机制，要求用户通过数据管理控制台进行手动冷备份与冷重建。

#### 2.8.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-store-sqlite/src/schema_migrations.rs:20-48, 74-239`
  - `crates/jftrade-store-sqlite/src/schema_manifest.rs:168-212`
  - `crates/jftrade-engine/src/product_data_management.rs:79-113, 236-315`
  - `tests/fixtures/compatibility/storage/sqlite-schema-definitions.json`
- **关键机制**:
  1. **9 个数据库清单与版本**:
     - `backtest` (v3), `backtest-runs` (v1), `strategy` (v2), `execution-orders` (v5), `adk` (v4), `adk-session` (v4), `adk-artifact` (v1), `watchlist` (v1), `research` (v1)。
  2. **显式支持的迁移跳跃**:
     - `backtest (v2 -> v3)`: 规范化表名带入 Provider 与 FNV-1a 哈希。
     - `strategy (v1 -> v2)`: 建立版本表 `strategy_definition_versions` 与防篡改触发器。
     - `adk (v2 -> v3)`: 增加客户端请求 ID 与请求指纹列。
     - `adk (v3 -> v4)`: 显式空跳跃递增版本号。
  3. **在线备份与双重完整性校验**: 迁移前使用 SQLite Online Backup API 生成临时备份，执行强制刷盘，并以只读方式打开执行 `PRAGMA quick_check` 与 `foreign_key_check`，校验通过后原子重命名为 `{path}.pre-migration.bak`。迁移出错时执行全局原子回滚。

#### 2.8.3 微观差异与破坏性边界失效推演
1. **缺失的迁移路径断崖 (P1-06 缺陷推演)**:
   - `execution-orders` 现为 v5，但代码中**完全没有从 v1-v4 升级到 v5 的迁移逻辑**！
   - `adk-session` 现为 v4，但**完全没有从 v1-v3 升级到 v4 的迁移逻辑**！
   - 若用户从较旧版本的 JFTrade 升级，由于迁移跳跃缺失，系统直接报错崩溃拒启。
2. **不可逆降级绝对硬阻断**:
   - `schema_migrations.rs:20` 中严禁 `from_version >= expected_version`。升级新版后若想回滚旧版本二进制，旧程序检测到版本过高直接拒绝运行，用户只能手动还原 `.pre-migration.bak` 备份文件，导致升级后产生的新交易流水全部丢失。
3. **adk-session.events 历史 P0 慢查询索引缺口遗留 (P1-05 缺陷推演)**:
   - 历史审计文件 `docs/architecture/sqlite-query-plan-audit.md` 早在 2026-07-29 即指出：
     `events` 表主键为 `(id, app_name, user_id, session_id)`，但核心业务查询为 `WHERE session_id = ? ORDER BY timestamp ASC`。
   - 主键首列为 `id` 导致索引前缀完全失效，所有会话查询退化为 **全表扫描 (FULL TABLE SCAN) + USE TEMP B-TREE 临时排序**！
   - 在 Rust 主线中，该表**依然未添加任何索引**。用户若手工在数据库加索引，启动时的 `validate_current` 会因表定义不匹配而直接报错拒绝启动。

#### 2.8.4 Release Qualification 验证清单
- [x] **RQ-MIG-01（正常流 / P1-06 闭环）**: 使用旧版 `backtest.db` (v2) 与 `strategy.db` (v1) 启动引擎，核验成功生成 `.pre-migration.bak`，备份通过 `PRAGMA quick_check`，新表建立与数据回填正常，重复打开幂等。
  - 验证用例：`test_p1_06_supported_legacy_migrations_upgrade_and_repeated_open` (PASS)
- [x] **RQ-MIG-02（异常流 / P1-06 闭环）**: 在迁移 DDL 中注入语法错误，验证事务自动回滚，数据库文件版本与无损状态完好保留。
  - 验证用例：`test_p1_06_migration_syntax_error_triggers_atomic_rollback` (PASS)
- [x] **RQ-MIG-03（降级阻断 / P1-06 闭环）**: 使用较旧期望版本加载高版本数据库，核验系统多层绝对阻断（`validate_current` 报 `incompatible`，`migrate_legacy_schema` 报 `unsupported migration range`）拒绝启动。
  - 验证用例：`test_p1_06_downgrade_strictly_rejected_at_all_layers` (PASS)
- [x] **RQ-MIG-04（性能压测与索引审计 / P1-05 闭环）**:
  - **历史提议反证**: 证明旧 Go 提议索引 `(app_name, user_id, session_id, timestamp DESC)` 因前缀缺失无法被 Rust `WHERE session_id = ?` 命中，依然触发全表扫描与临时 B-Tree 排序。
  - **最优索引验证**: 证明瘦索引 `(session_id, timestamp ASC, id ASC)` 消除全表扫描与临时排序（转为 `SEARCH`），带来 14.4x~48.7x 查询加速，且避免了宽覆盖索引大文本字段引起的页溢出与写放大。
  - 验证用例：
    * `test_p1_05_adk_session_events_query_plan_without_index_shows_scan_and_temp_btree` (PASS)
    * `test_p1_05_adk_session_events_refutes_go_historical_index_proposal` (PASS)
    * `test_p1_05_adk_session_events_optimal_index_achieves_search_and_zero_temp_btree` (PASS)
    * `test_p1_05_adk_session_events_performance_benchmark_gain` (PASS)

