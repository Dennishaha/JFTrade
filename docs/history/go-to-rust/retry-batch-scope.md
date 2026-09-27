# 重试与退避域对齐批次（第 104 批）

本文件记录 Go 基线 `internal/retry`（指数退避重试助手 `Do`、`ShouldRetry`/`Notify` 钩子、
Futu 限流判定）迁移到 Rust 的逐测试核对结论，涉及
`crates/jftrade-engine/tests/sidecar_subprocesses_crash_recovery_resilience.rs`、
`crates/jftrade-engine/src/product_runtime_helper_health.rs`（退避阶梯）、
`crates/jftrade-integration-futu/src/{basic_quote_query.rs,runtime_task.rs,...}`
（重放循环、重连退避、限流分类）与 `crates/jftrade-integration-pine/src/{process.rs,readiness.rs}`。

## 第一百零四批：internal/retry 收口（6 条）

### 范围与分片

`internal/retry` 共 6 条 `missing`，两个文件：

- P1 `retry_test.go` 4：确定性翻倍+封顶（:10）、零退避不 sleep（:40）、
  不可重试错误立即返回（:57）、Futu 限流判定（:76）。
- P1 `do_attempts_test.go` 2：负 MaxRetries 归一 + nil 不可重试（:8）、
  失败一次后重试成功（:26）。

分类：3 条 `[x]`/function_exact + 3 条 partial；`internal/retry` 域内 `missing` 归零。

### 关键事实（本批 recon 实测）

- Go 语义（复核后确认）：`MaxDelay<=0` 归一为 30s、`MaxRetries<0` 归一为 0；总尝试 =
  `MaxRetries+1`；退避 `BaseDelay << (attempt-1)` 确定性翻倍并以 `MaxDelay` 封顶（另有
  `delay < BaseDelay` 的溢出保护）；`BaseDelay==0` 表示不 sleep；`ShouldRetry` 为 false 时
  立即返回原错误；`Notify` 只在每次重试 sleep 前调用；耗尽后返回
  `retry exhausted after N retries: %w`；`FutuRateLimitShouldRetry` 是文本谓词
  （含「频率太高」或「retType=-1」即 true，nil 为 false）。
- **`internal/retry` 在冻结基线里没有生产调用方**：`git grep -n "internal/retry" go` 与
  `git grep -n "\bretry[A-Za-z]*\.Do(" go` 均只命中该包自身文件，没有任何业务包 import 它。
  因此 Rust 侧不复制一个无人调用的共享 `Do` 助手（仓库规则也不提前引入未使用 API），
  而是登记「同一策略在具体 owner 的实现 + 差异」。
- Rust 策略分布：退避阶梯在 `compute_helper_backoff`（engine）/`compute_pine_backoff`
  （pine）/`PineReadinessPolicy::retry_delay`/`retry_delay_ms`（ADK）等；尝试上限与重放在
  futu `OpenDBasicQuoteExecutor::query_with_retry`（2 次、零退避）、backtest sync
  `fetch_futu_page_with_retry`（4 次、线性延迟、`futu_error_retryable` 判定）、运行时任务
  重连循环（`reconnect_delay` + `runtime.status().reconnects`）；限流分类在
  `classify_security_snapshot_fetch_error` / `margin_ratio_rate_limited_error` / 结构化 errCode。

### 结果

- 全局：4451 = function_exact **1305** + partial **2542** + boundary 573 + module_only 4 +
  missing **27**（前批 1302 / 2539 / 573 / 4 / 33）。
- Rust 测试 3035 → **3036**（本批新增 1 条；其余 5 条复用所属 owner 的既有测试）。
- 锚点对账：1305 → **1309** 唯一引用（已记账 1250 → **1254**、unrecorded **0**、
  unknown 55、stale **0**）；未锚定 function_exact 告警保持 **199**（本批 3 条 `[x]` 行全部带锚点）。

### 本批新增/加强的测试

Rust（1 条新增 + 4 个锚点）：

- `crates/jftrade-integration-futu/src/basic_quote_query.rs::basic_quote_query_returns_non_recoverable_rejection_without_replaying`
  （`retry_test.go:57`）：mock OpenD 返回 `ret_type=-1/err_code=1000/retMsg=no permission`，
  断言返回的正是该 `Rejected{ret_type:-1,error_code:1000,message}`、注入的 reconnect 闭包
  调用次数为 0、服务端只接受一次查询 → 非可重试错误立即返回且不重放。
- 新增锚点（不新增测试）：`retry_test.go:40` → futu
  `basic_quote_query_replays_once_after_recoverable_session_timeout`（断言
  `BASIC_QUOTE_QUERY_ATTEMPTS == 2` 与 `BASIC_QUOTE_RETRY_BACKOFF.is_zero()`）；
  `do_attempts_test.go:26` → futu
  `runtime_task_backoff_replays_after_a_failed_reconnect_attempt`；
  `retry_test.go:10`（partial 证据）→ engine
  `test_exponential_backoff_progression_and_upper_bound_capping`。

### 探针（改坏→转红→按字节回滚）

1. `crates/jftrade-integration-futu/src/basic_quote_query.rs`：把
   `is_recoverable_session_error` 的非会话分支改成返回 true → 新测试转红（错误变成连接失败、
   重连被调用）；回滚后 shasum `c0b6fb19d81c3a028df3168f91463dacc2e0cdd5897dafc33cdd0446dbb40ef4`。
2. 同文件：把 `BASIC_QUOTE_RETRY_BACKOFF` 从 `Duration::ZERO` 改成 1ms →
   `basic_quote_query_replays_once_after_recoverable_session_timeout` 红
   （`assert!(BASIC_QUOTE_RETRY_BACKOFF.is_zero())` 失败）；回滚到同一 shasum。
3. `crates/jftrade-integration-futu/src/runtime_task.rs`：把 `reconnect_delay` 改成恒返回
   `Duration::ZERO` → `runtime_task_backoff_replays_after_a_failed_reconnect_attempt` 红
   （`recovered_accept - failed_accept >= 30ms` 失败）；回滚后 shasum
   `2b87942a890de566d5e93126cf466ca26ebf6b3849d35ad21e01635210d5e6e3`。
4. `crates/jftrade-engine/src/product_runtime_helper_health.rs`：把 `compute_helper_backoff`
   改成 `attempt >= 2` 恒返回 initial（阶梯不再翻倍）→
   `test_exponential_backoff_progression_and_upper_bound_capping` 红；回滚后 shasum
   `a5ba59d0c1399ea4ea2754ab3ebc6a169a7ff0b26803383ee6b13c1ba46512b8`。

### 保留差异与边界

- 没有共享 `Do(operation, cfg)`：`MaxRetries/BaseDelay/MaxDelay/ShouldRetry/Notify` 五个可配置项
  在 Rust 被拆成「owner 常量 + 分类器 + 监控计数」，因此 `retry_test.go:10`（翻倍封顶以外的
  尝试上限、Notify 回调、`retry exhausted` 包装错误）与 `do_attempts_test.go:8`
  （负 MaxRetries 归一）记为 partial，而不是新增无人调用的迁移候选 API。
- 零退避：Go 用 `BaseDelay=0` 表达不 sleep；Rust 没有该旋钮，具体 owner 用显式
  `Duration::ZERO` 常量并在代码里跳过 sleep（futu BasicQot 重放），测试通过断言常量与分支
  语义而非计时来钉住契约。
- Futu 限流：Go 把「含 `retType=-1` 的任意错误文本」视为可重试；Rust 按 errCode/配额文案
  细分（`retType=-1` 是 OpenD 业务拒绝的通用前缀，例如权限错误 errCode=9/1000，必须不可重放）。
- 尝试上限的 owner 差异：futu BasicQot 重放 2 次、backtest sync 4 次（线性延迟）、
  ADK 恢复扫描按 durable marker 退避（最长 60s）、sidecar 监控按崩溃次数重启。
- `internal/retry` 无生产调用方这一事实本身也登记在本批结论里：这是「Go 侧遗留工具」
  而非 Rust 需要补齐的功能面，后续批次不应据此新增共享助手。

### 后续待办

- 下一批按域余量：`internal/jftsettings` 5 → `internal/frontendassets` 4（锚点例外）→
  `internal/research` 4 → `internal/security/passwordhash` 3 → …，直到 4451 条全部完成。
- 待办：`crates/jftrade-engine/src/product_production_ports_backtest_sync.rs::futu_error_retryable`
  与 `fetch_futu_page_with_retry` 目前只有端点级夹具测试；若要提升到 function_exact，
  需要在 backtest 同步域补一条「429 重试、Decode 立即失败」的定向测试。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-integration-futu
  -p jftrade-engine --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine
  --all-targets --locked --no-fail-fast`：2290 passed / 1 skipped / 0 failed。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3036 Rust、`[x]` 1305、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 199（本批 3 条新 `[x]`
  行全部带 `// Parity:` 锚点）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1309 唯一引用（已记账 1254、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2909 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` 首次 EXIT=1：唯一失败阶段是 `pnpm run check:rust:target-health`
  （`target/debug/deps` 中间 `.rcgu.o` ≥ 50000），按约定确认无 cargo/rustc 进程后执行
  `pnpm run clean:rust:artifacts`（移除 152012 个文件 / 40.6GiB）并重跑；第二次 EXIT=0：
  affected 计划含 policy（脚本 124 + 兼容 19 + Python 59 全绿）、contracts、target-health、
  nextest（desktop + engine + futu）2318 passed / 1 skipped、fmt/clippy、兼容回放与 desktop 检查。
- `pnpm run check:rust` EXIT=1：唯一阻塞点仍是 `pnpm run check:rust:policy`（`cargo deny check`）
  的 RUSTSEC-2026-0285（rustls 0.23.44，修复需 >=0.23.45）与 8 条
  `warning[advisory-not-detected]` 陈旧 ignore；run 停在静态阶段、未执行 workspace/all-targets
  阶段，等价测试面由上面的 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致。

### 2026-09-26 parity baseline correction（历史批次不回写）

本文件前述审计数字和门禁收据属于 retry 批次完成时的历史快照。后续 assistant provider probe、strategy activity 与 workflow invalid-input 批次新增 7 条 `function_exact`，并同步减少 3 条 `partial`、4 条 `boundary`；当前全局基线为 Go 4451、Rust 3366、`function_exact` 1491、`partial` 2332、`boundary` 628，Parity 锚点 1780/1734/0/0/46。retry 域自身的历史结论（包括 `cargo deny` advisory 阻塞）保持不变，不应与当前工作树门禁结果混用。
