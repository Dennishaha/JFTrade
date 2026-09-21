# 尽力而为错误边界批次（第 111 批）

本文件记录 Go 基线 `pkg/besteffort`（尽力而为错误日志助手）的 2 条 `missing` 测试迁移到 Rust 的
逐条核对结论。Rust 没有同名通用包，等价契约落在 ADK 审计的 best-effort 插入边界上，涉及
`crates/jftrade-engine/src/product_adk_model_runtime_audit.rs`（新增可测试的三态分类）与
`crates/jftrade-engine/src/product_adk_model_runtime_terminal_audit_tests.rs`（本批新增 2 条测试）。

## 第一百一十一批：pkg/besteffort 收口（2 条）

### 范围与分片

Go 侧 2 个文件、2 条 `missing`：

- P1 `besteffort_test.go:11`：`TestLogError`（非 nil 错误输出恰好一行 `best-effort operation failed: <err>`）。
- P1 `besteffort_test.go:33`：`TestLogErrorNoError`（nil 错误零输出；`LogResult` 丢弃主值只看错误）。

分类：2 条 `[x]`/function_exact；`pkg/besteffort` 域内 `missing` 归零。

### 关键事实（本批 recon 与实测）

- **Rust 无同名通用包**：`rg -n -i "best-effort|besteffort|LogError|LogResult" crates` 只有注释里的
  “best-effort” 描述（ADK 审计、执行订单 fence、futu 关闭路径），没有生产符号。Rust 把“继续执行”
  作为每个 owner 的显式决定，而不是抽一个共享 `LogError`。
- **契约相同的最近 owner**：`product_adk_model_runtime_audit.rs` 的模块注释即契约原文——
  “A store fault is logged, never propagated, so an audit write can never turn a successful run into
  a failure”；插入路径按 `Ok`／UNIQUE 冲突／真实故障三分支处理。
- **本批改动（行为不变、加可测试接缝）**：把原先内联的 `match` 抽成
  `classify_audit_insert(...) -> AuditInsertOutcome { Recorded, AlreadyAudited, Failed(String) }`，
  生产调用方 `record_audit_event` 仍只对 `Failed` 打印一行
  `failed to record ADK audit event <id>: <error>` 后继续，无公开 API 变更、错误文本不变。
- **故障注入方式**：测试用临时目录建库后 `DROP TABLE adk_audit_events`，让审计插入真实失败，
  同时 run 投影仍应提交（对应 Go “错误被记录后吞掉、调用方继续”）。

### 逐条结论

- `besteffort_test.go:11` → `crates/jftrade-engine/src/product_adk_model_runtime_terminal_audit_tests.rs::best_effort_audit_faults_are_reported_once_and_never_fail_the_run`：
  表被删后分类必须是 `Failed(非空)`（恰好一个待报告项），且 `persist_failure` 仍返回 Ok、
  run 仍以 `FAILED` 落库。
- `besteffort_test.go:33` → `::best_effort_audit_reports_nothing_when_the_insert_succeeds_or_was_already_recorded`：
  健康库上同一事件连续分类依次为 `Recorded`、`AlreadyAudited`（两次都不产生报告），
  audit 行数保持 1（fenced 重试不写第二行、不误报故障）。

### 探针记录（破 → 红 → 按字节回滚）

- 把所有错误都归为 `AlreadyAudited`（等价于吞掉真实故障、永不报告）：故障测试转红
  （`got AlreadyAudited`）。
- 把 UNIQUE 冲突分支改成 `Failed(...)`（等价于把幂等重试当故障、产生多余报告）：静默测试转红
  （`left: Failed("query adk database: UNIQUE constraint failed: adk_audit_events.id")`,
  `right: AlreadyAudited`）。

两次探针回滚后 shasum 恒定：`product_adk_model_runtime_audit.rs` 为
`95feb90ac095a871700f9345f55d6f65bbe4612c239cea2d1a35021e855aac07`，
`product_adk_model_runtime_terminal_audit_tests.rs` 为
`03d0476b220d1ce01eb39e092543b021339dc01492d850b809e5317062e22134`。

### 保留差异

- Go 的报告通道是 `log.Printf`（测试用 `log.SetOutput` 捕获文本断言）；Rust 在 ADK 边界用
  `eprintln!` 固定前缀，本批断言的是“分类恰好一次 + 不返回错误”的可观察契约，日志文本本身
  不在断言里（Rust 测试无法进程内捕获 stderr，故以分类结果承载“是否会被报告”）。
- Go 用 nil 判断，Rust 用三态枚举（多出 `AlreadyAudited`）：这是 Rust 为 fenced 重试保留首行而
  引入的更严格语义，Go 没有对应概念。
- `LogResult[T]` 的泛型丢值语义在 Rust 无同名 API，由“插入返回值被忽略、只看分类结果”承担。
- Rust 的 best-effort 报告点分散（ADK 审计、执行订单 durable fence、futu 关闭清理等），
  本批以 ADK 审计这一最完整、已文档化契约的边界收口；统一 helper 属可选重构，未在本批引入。

### 后续待办

- 下一批按域余量：`internal/app/apiserver/servercoretest` 1 → `internal/app/apiserver/webaccess` 1 →
  `cmd/jftrade-desktop` 1 → `pkg/chart` 1 → `scripts/archive_frontend_assets_test.go` 1 →
  直至 4451 条全部完成。
- 既有独立项：修复审计脚本生成 markdown 时的尖括号占位符（`<go 文件>:<行>`），使
  `build:docs:generated` 不再被 markdown-it 判为未闭合 HTML 标签。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-engine --all-targets --locked`
  EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
  --no-fail-fast`：1758 passed / 0 skipped / 0 failed（含本批新增 2 条；聚焦运行 2/2 passed）。
- 两次探针各自转红并已按字节回滚（所有错误归为 AlreadyAudited、UNIQUE 冲突归为 Failed），
  回滚后两个文件 shasum 与探测前一致（见上文探针记录）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3054 Rust、`[x]` 1320 → 1322、
  `missing` 7 → 5、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；
  未锚定 function_exact 保持 202（本批 2 条均写锚点）、partial 无解析引用 7（既有基线）、
  无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1323 唯一引用（已记账 1268，+2；
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2919 tracked files，含本批新文档）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`），报
  RUSTSEC-2026-0285 漏洞与陈旧 advisory 告警，与本仓既有基线同源。**不记为通过**；该 run 停在
  静态阶段，等价测试面由上面的 nextest 结果覆盖。首跑曾在 `check:rust:target-health` 因
  `.rcgu.o` ≥50000 失败，确认无 cargo/rustc 后执行 `pnpm run clean:rust:artifacts` 再跑得到上述
  结论（环境阈值问题，与本批改动无关）。
