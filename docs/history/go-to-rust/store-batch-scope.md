# Store 领域对齐批次

## 第 130 批 08 切片一：sqliteconn 26 条 recon（2026-09-24）

范围：`internal/store/sqliteconn/conn_test.go`（10）+
`coordinator_test.go`（7）+ `db_api_test.go`（4）+
`db_concurrency_test.go`（4）+ `maintenance_test.go`（1），
共 26 条（partial 9 + boundary 17）。

方法：Go 体逐条核对结论；9 条 partial 与 17 条 boundary 的 Rust 引用逐一
rg 命中（0 缺失）；结构断言：Rust 无连接池/写协调器/读屏障/DSN 拼接层
（仅 owner-lock 测试内的 Parity 锚点注释命中同名关键词，无实现），
单 rusqlite 连接互斥 + WriterLease + busy_timeout=10s（多处 `busy_timeout`
10s 设置在位），boundary 口径属实；关键语义抽查 Go `:12`（池统计 +
busy_timeout 10000 + foreign_keys）与 Go `:11`（写者 FIFO + 读屏障门控），
结论如实记录了不可表达项与缺口。

结论：26 条 verdict 全部成立，无判定变更、无代码变更。本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、
`check:migration-manifest`、`check:zero-go`、
定向 nextest（store-sqlite 维护/失败恢复/查询计划 + owner-lock + settings-file 契约）、
`check:quick`（单实例）、`git diff --check`。

下一片：store 域按文件继续（sqliteschema 系 30 条）。

## 第 130 批 08 切片二：sqliteschema 30 条 recon（2026-09-24）

范围：`internal/store/sqliteschema/catalog_test.go`（17）+
`schema_boundaries_test.go`（8）+ `schema_fault_driver_test.go`（3）+
`schema_test.go`（2），共 30 条（partial 29 + boundary 1）。

方法：Go 体逐条核对结论；30 条 Rust 引用逐一 rg 命中（0 缺失）；
关键语义抽查 Go `:170`（延迟外键提交失败→零表残留）与 Rust
`test_p1_06_migration_syntax_error_triggers_atomic_rollback`
（语法错误→丢弃事务→版本保持）：两者同断言初始化期原子回滚，
延迟约束注入专项无对应断言，partial 诚实；唯一 boundary `:57`
（database/sql 行集关闭错误合并）属实——rusqlite 无行集关闭错误合并层。

结论：30 条 verdict 全部成立，无判定变更、无代码变更。本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、
`check:migration-manifest`、`check:zero-go`、
定向 nextest（store-sqlite 整 crate）、`check:quick`（单实例）、`git diff --check`。

下一片：store 域按文件继续（settingsfile 系 38 条）。

## 第 130 批 08 切片三：settingsfile 38 条 recon（2026-09-24）

范围：`internal/store/settingsfile/` 全域 8 文件 38 条：
legacy 1、market_data 5、normalization 6、persist_failures 1、
rollback 3、persistence_contracts 9、recovery 6、store 7
（partial 37 + `[x]` 1）。

方法：Go 体逐条核对结论；38 条 Rust 引用逐一 rg 命中（0 缺失）；
唯一 `[x]`（`store:205` US 默认远端源 NYSE 列表）逐项等价，
preferred/enabled 两列表同值断言 + `// Parity:` 锚点在位，成立。
37 partial 均明确写出缺口（显式 false 保留、enabled 列表重写、
scope ID 格式、未知字段不重写、逐日矩阵等），口径诚实。
附带发现（非判定变更）：`test_failed_setting_saves_rollback_all_runtime_state`
名实不符——函数名承诺失败回滚，函数体仅做 appearance 成功保存可读；
台账结论已如实描述现状（partial），改名/补断言留待后续回归。

结论：38 条 verdict 全部成立，无判定变更、无代码变更。本片为纯 recon。

验证：`audit_test_parity.py --write-report` exit 0；`parity_anchor_reconcile.py` 过；
`cargo fmt -p jftrade-engine -- --check`、`check:ai-context`、
`check:migration-manifest`、`check:zero-go`、
定向 nextest（store-settings-file + settings 整 crate）、
`check:quick`（单实例）、`git diff --check`。

下一片：store 域按文件继续（trading 系约 50 条）。
