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
