# API 入口域对齐批次（第 109 批）

本文件记录 Go 基线 `cmd/jftrade-api`（`main.go`、`main_test.go`）的 2 条 `missing` 测试迁移到
Rust 的逐条核对结论，涉及 `crates/jftrade-engine/src/bin/jftrade-api-rust.rs`（进程入口与停止信号）、
`crates/jftrade-engine/src/product_production_profile.rs`（`ProductConfig::from_process_env`）与本次新增的
`crates/jftrade-engine/tests/product_api_launcher_lifecycle.rs`（端到端子进程测试）。

## 第一百零九批：cmd 侧 API 入口收口（2 条）

### 范围与分片

Go 侧 2 个文件、2 条 `missing`：

- P0 `main_test.go:86`：`TestRunAPICommandStartsAndStopsAPI`（缓存开关恰好写一次、runAPI 收到信号
  派生的 ctx、返回前必须 `stop()`）。
- P0 `main_test.go:121`：`TestRunAPICommandPreservesConfiguredCacheAndWrapsStartupErrors`
  （既有配置不得被覆盖、startup 错误必须可追溯并带前缀）。

分类：2 条 `[x]`/function_exact；`cmd/jftrade-api` 域内 `missing` 归零。

### 关键事实（本批 recon 与实测）

- **真实缺口（已修）**：Rust 入口原先只 `await tokio::signal::ctrl_c()`，仅处理 SIGINT。实测
  `kill -TERM` 时进程以退出码 143 被杀（`wait` 返回 143、`ExitStatus::code()==None`），
  `handle.shutdown()` 从不执行；而 Go 基线用
  `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` 同时注册 INT+TERM
  并 `defer stop()`。现改为 Unix 上 `select(ctrl_c, SIGTERM)`，实测 SIGTERM → 退出码 0、
  SIGINT → 退出码 0。
- **启动失败路径实测**：占用 loopback 端口后启动，进程退出码 1、stdout 为空、stderr 为
  `Error: Product(Bind(Os { code: 48, kind: AddrInUse, message: "Address already in use" }))`
  ——错误原因保留在返回链上（对应 Go 的 `errors.Is` + 包装前缀两种保证）。
- **结构差异**：Go 的 `runAPICommand` 把 `args/stdout/getenv/setenv/runAPI/notifyContext` 全部作为
  参数注入以便单测；Rust 入口没有该注入层（`ProductRuntimeBuilder::from_process_env()` 直接读
  进程环境），因此等价证据改为**端到端子进程测试**：真实二进制 + 临时目录 + 显式 bind。
  同域其余 `cmd` 行在此前批次已判为 boundary（无 CLI 参数/子命令兼容层）。
- **DISABLE_MARKETS_CACHE 无对应物**：该变量用于关闭 bbgo 的磁盘行情缓存；Rust 引擎不存在
  bbgo 缓存，行情缓存由 `ProviderRouter`/设置承担，启动过程不写任何进程环境变量。Go 的
  「不得覆盖既有配置」在 Rust 侧以「运维显式配置的 bind/settings 被逐字采用」承载。
- **锚点例外**：`cmd/jftrade-api` 字面量命中 `scripts/check-zero-go.mjs` 的 `activeTextPattern`，
  活跃文件（crates/、scripts/、apps/、.github/）一律不得出现，因此本批 2 条 `[x]` 行**不写**
  `// Parity:` 锚点（先例：第 106 批 internal/frontendassets），audit 未锚定 function_exact
  由 200 增至 202。

### 逐条结论

- `main_test.go:86` → `crates/jftrade-engine/tests/product_api_launcher_lifecycle.rs::api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal`：
  启动真实二进制 → 对配置地址建立 TCP 连接（API 已在服务）→ SIGTERM → 退出码 0，ready 记录的
  `address` 等于配置地址、`databaseLeaseStatus=acquired`。
- `main_test.go:121` → `::api_launcher_reports_startup_failure_when_the_configured_address_is_taken`：
  测试自己占用端口后启动，断言退出码 1、stdout 为空、stderr 含 `Error: ` 与 `AddrInUse`；
  释放端口后按同一设置目录重试必须重新 ready 并优雅停止（证明失败启动没有遗留 9 个
  SQLite WriterLease 的占用）。

### 探针记录（破 → 红 → 按字节回滚）

- 入口改回仅 `ctrl_c`：启动/停止测试转红（`left: None, right: Some(0)`，提示必须走 shutdown 路径）。
- 让配置 bind 被默认地址替换（`product_production_profile.rs` 直接用 `DEFAULT_PRODUCT_BIND`）：
  启动/停止测试转红（60s 内配置地址从未可连接）。
- 吞掉启动错误（`from_process_env()?.start().await?` 改为匹配后 `return Ok(())`）：
  启动失败测试转红（退出码 Some(0)≠Some(1)）。

三次探针回滚后 shasum 恒定：`jftrade-api-rust.rs` 为
`c49ccb69e728ae1c89418416b91947c05056b3efa01c17919c2068a95194dc63`，
`product_production_profile.rs` 为
`351162acfc26a248c02b9b68c6c39fb53ba9079874d303acd7af7c9be752b4e9`，
新增测试文件为 `ee7529e7b36b23864f57e8bef18e94dec7d3f47790650254dcdb88e63e5bbcfa`。

### 保留差异

- Go 在 `runAPICommand` 内注入 `getenv/setenv/notifyContext`；Rust 只有进程入口，等价证据为
  子进程端到端行为（含真实信号与退出码），覆盖面更大但更慢（两个测试合计约 5-6 秒）。
- Go 的缓存开关断言（`DISABLE_MARKETS_CACHE`）无 Rust 对应物：该缓存属于已删除的 Go 运行时，
  Rust 启动不写进程环境变量。
- Go 用 `fmt.Errorf("...: %w")` 包装后 `log.Fatalf` 打印；Rust 由 `main` 返回
  `Box<dyn Error>`，退出码 1 + stderr `Error: <链上错误>`，无固定前缀常量。
- 取消/超时语义：Go 由 `NotifyContext` 的 ctx 取消驱动；Rust 由信号等待结束驱动
  `handle.shutdown()`（有界关闭由既有 `test_product_runtime_ordered_shutdown_*` 覆盖）。

### 后续待办

- 下一批按域余量：`cmd/check-go-coverage` 2 → `pkg/besteffort` 2 →
  `internal/app/apiserver/servercoretest` 1 → `internal/app/apiserver/webaccess` 1 →
  `cmd/jftrade-desktop` 1 → `pkg/chart` 1 → `scripts/archive_frontend_assets_test.go` 1 →
  直至 4451 条全部完成。
- 既有独立项：修复审计脚本生成 markdown 时的尖括号占位符（`<go 文件>:<行>`），使
  `build:docs:generated` 不再被 markdown-it 判为未闭合 HTML 标签。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-engine --all-targets
  --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked
  --no-fail-fast`：1756 passed / 0 skipped / 0 failed（含本批新增 2 条；聚焦运行 2/2 passed，
  合计 5.6s）。
- 三次探针各自转红并已按字节回滚（仅 ctrl_c、忽略配置 bind、吞掉启动错误），回滚后三个文件
  shasum 与探测前一致（见上文探针记录）。
- 实测证据（本机手测，非测试）：修复前 SIGTERM → 退出码 143，修复后 SIGTERM/SIGINT → 退出码 0；
  端口占用时启动 → 退出码 1、stdout 空、stderr `Error: Product(Bind(Os { code: 48, kind:
  AddrInUse, ... }))`。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3052 Rust、`[x]` 1320（+2）、
  `missing` 11 → 9、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；
  未锚定 200 → 202（本批锚点例外，见上文）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1321 唯一引用（未锚定新增 2 条不计入；
  已记账 1266、unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2916 tracked files，含本批新文档；新测试文件不含被禁
  字面量）；`pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（首跑因 `check:rust:target-health` 报 `.rcgu.o` 59220 个失败，
  确认无 cargo/rustc 后执行 `pnpm run clean:rust:artifacts` 重跑全绿）。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`），报
  RUSTSEC-2026-0285 漏洞与陈旧 advisory 告警，与本仓既有基线同源（第 106 批已在干净 HEAD 用
  `git stash` 复现同一失败）。**不记为通过**；该 run 停在静态阶段，等价测试面由上面的 nextest
  结果覆盖。
