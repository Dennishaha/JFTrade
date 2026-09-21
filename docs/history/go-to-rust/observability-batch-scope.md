# observability 域对齐批次（第 100 批）

本文件记录 Go 基线 `pkg/observability` 的上下文关联、重要性阈值、有界摘要与 OpenD 健康
迁移到 Rust 的逐测试核对结论，涉及 `crates/jftrade-api/src/observability.rs`
（`TransportMetrics` / `RequestObservabilitySnapshot` / `sanitize_summary_text`）、
`crates/jftrade-kernel/src/open_d_observer.rs`（OpenD 上报 port）与
`crates/jftrade-engine/src/product_opend_call_observer.rs`（composition root 适配器）。

## 第一百批：pkg/observability 收口（10 条）

### 范围与分片

`pkg/observability` 共 10 条 `missing`，两组：

- P1 `observability_test.go` 8：结构化日志关联字段（:14）、空快照集合序列化（:45）、
  重要性阈值抑制（:62）、无 recorder 的全局阈值（:94）、有界错误/慢请求与 OpenD 健康
  （:113）、Detach 保留关联且不继承取消（:148）、边界默认值与全局 OpenD 日志（:163）、
  nil context 与 OpenD 成功边界（:215）。
- P1 `context_detach_and_importance_test.go` 2：background context 与重要性等级表（:10）、
  `Detach(background, parent)` 保留 Source/Recorder（:37）。

### 关键事实（本批 recon 实测）

- Go 的观测载体是 `context.WithValue`：`WithFields` 合并九字段（trim 后空值忽略）、
  `WithRecorder` 挂长生命周期 recorder、`Detach(base,parent)` 只搬观测状态并以 base 承担取消、
  nil context 归一为 `context.Background()`。Rust 不用 context 挂载观测状态：router 中间件
  生成/校验 request id 后按参数传给 `finish_request`，OpenD 关联由 kernel `OpenDCallRecord`
  显式携带 `operation/request_id/error`，取消由 tokio 任务结构承担。
- Go 的 `Recorder.RecordHTTPRequest` 与 `RecordOpenDCall` 是唯一写入方：慢请求进
  `recentSlowRequests`（importance=low，`latency >= slowThreshold` 且过阈值），5xx 进
  `recentErrors`（level=error、importance=high、message="api request failed"、error="HTTP N"），
  两者共用 `prependBounded`（最新在前 + 上限截断）。OpenD 调用无论成败都更新健康计数，
  失败时 `LastError/LastErrorAt/FailedCalls` 前进，成功时 `LastSuccessAt` 前进。
- **Go 的 OpenD 失败还会写入 recentErrors**：`Recorder.RecordOpenDCall` 失败分支调用
  `ErrorWithImportance(ctx, ImportanceHigh, "opend call failed", err, ...)`，其内部
  `RecorderFromContext(ctx).recordError(newEvent(...))` 把 `source=opend` + requestId +
  sanitized error 的事件按同一阈值写入 `recentErrors`（`operation`/`latency_ms` 只是 slog
  属性，不进入记录字段）。Go 侧独立证据：`pkg/futu/opend/client_test.go:77
  TestCallFailureRecordsRequestCorrelation` 断言 `RecentErrors[0].Source == "opend"`。
- Go 的重要性：`importanceByRank(0..3)` 映射低/常/高/严；`importanceRank(x)` 先
  `NormalizeImportance`（别名 debug/trace→low、info/default→normal、warn/warning/error→high、
  fatal/panic→critical，未知→normal）再排名；`NormalizeMinimumImportance` 未知→low；
  全局阈值是进程级 atomic（`SetMinimumImportance`）。Rust 把阈值放在
  `TransportMetrics::new(_,_,minimum_importance)`，全局值只在构造 `default()` 时读
  `JFTRADE_OBSERVABILITY_MIN_IMPORTANCE`。
- `sanitizeSummaryText` 折叠空白（`strings.Fields` 连接）后按 500 字节截断补 `...`；
  默认事件上限 20、默认慢阈值 750ms。

### 结果

- 10 条全部给出结论：**2 条 `[x]`/function_exact + 6 条 `partial` + 2 条 `boundary`**，
  `pkg/observability` 域内 `missing` 归零。
- 全局：4451 = function_exact **1289** + partial **2534** + boundary 569 + module_only 4 +
  missing **55**（前批 1287 / 2528 / 567 / 4 / 65），Rust 测试 2996 → **3005**。
- 锚点对账：anchors 1290 → **1298**（本批 8 个新锚点全部已记账），unrecorded 保持 **0**。

### 本批新增测试（9 条，均在 `crates/jftrade-api/src/observability.rs`）

`[x]` 行证据（2 条）：

- `request_observability_snapshot_serializes_empty_collections_as_arrays`（:45）：空快照
  serde 后 `recentErrors=[]`、`recentSlowRequests=[]`、`openD={totalCalls:0,failedCalls:0}`。
- `recorder_bounds_errors_slow_requests_and_open_d_correlation_like_go`（:113）：同一 Go
  序列下断言错误有界为 2 且最新在前、慢请求只剩 `/slow`、两类错误 importance 均为 high、
  OpenD 计数与 lastOperation/lastRequestId 正确。

`partial` 行证据（7 条）：

- `request_observability_events_trim_and_serialize_correlation_keys`（:14）：快照事件的
  camelCase 关联键（requestId/source）、trim 取值与空值省略，OpenD 事件键集合为
  `at/error/importance/level/message/requestId/source`。
- `importance_threshold_suppresses_slow_events_and_keeps_high_events`（:62）：minimum=high 下
  慢请求（low）被抑制，500 与 OpenD 失败（high）仍记录。
- `minimum_importance_aliases_normalize_like_go`（:94）：15 行别名/未知/空白表。
- `recorder_defaults_fall_back_to_go_limits_and_thresholds` 与
  `summary_text_collapses_whitespace_and_truncates_at_go_limit`（:163）：默认 20 条上限、
  750ms 阈值、未知最小值→low；空白折叠与 500 字节截断（含多字节边界）。
- `open_d_success_records_last_success_without_failure_counters`（:215）：成功路径计数、
  lastSuccessAt、空关联归一。
- `importance_rank_orders_go_levels_and_defaults_unknown_to_normal`（:10）：0..3 排序、
  别名归一与未知→normal。

### 功能修复（含先红后改）

- **OpenD 失败写入 recentErrors**（`observability_test.go:113`）：Rust 原先只更新 OpenD
  健康计数，Go 会把失败同时写进有界错误摘要（见上「关键事实」）。先在
  `crates/jftrade-api/src/observability.rs` 写复现测试与冻结语料修正，转红证据为
  `left: recentErrors[0].source="api"` / `right: "opend"` 与语料
  `left: [HTTP 500, HTTP 503]` / `right: [opend 失败, HTTP 500]`；随后新增
  `open_d_failure_event` 并在 `record_open_d_call` 内按同一阈值 `prepend_bounded`，转绿。
  冻结语料 `tests/fixtures/compatibility/api-transport/request-observability.json` 的
  `expected.recentErrors` 按 Go 语义修正（opend 失败事件顶掉最早的 503 事件；该文件不在
  `manifest.json` 的生成清单内，无生成器覆盖）。
- **重要性排名按 Go 归一化**（`context_detach_and_importance_test.go:10`）：`importance_rank`
  原按字面量匹配，别名（debug/warning/fatal）会被降为 normal；改为 Go 的
  「`NormalizeImportance` 后排名」，未知与空值仍为 normal。
- **既有 [x] 行被本批加强**：`pkg/futu/opend/client_test.go:77` 的 function_exact 证据原先
  只覆盖 port/适配器上报，未覆盖「失败事件出现在 recentErrors」；修复后该断言面由本批测试
  补齐（原行结论不变，属证据加强，不重复改行）。

### 探针（改坏 → 转红 → 按字节回滚）

1. `prepend_bounded` 去掉 `truncate(limit)` → `recorder_defaults_fall_back_to_go_limits_and_thresholds`
   转红（25 条 vs 20 条）→ 回滚，`observability.rs` 恢复
   `6d5f655a0b692d1118aff7d19e96c498732ba040b4c392bdd58ebf9d6f5b1119` 后转绿。
2. `MAX_SUMMARY_TEXT_BYTES` 500 → 499 → `summary_text_collapses_whitespace_and_truncates_at_go_limit`
   转红（截断长度/内容不等）→ 同法回滚并核对同一 shasum。
3. 删除 `record_open_d_call` 中的失败事件写入 → 有界用例与冻结语料回放同时转红
   （source 与 recentErrors 序列不符）→ 同法回滚并核对同一 shasum。

### 保留差异（记录在 partial/boundary 结论里）

- **结构化日志**：Go 用 slog JSON handler 断言九字段日志行；Rust 走
  `tracing_subscriber::fmt`（stderr 文本 + EnvFilter），没有把关联字段挂到日志事件的等价
  结构，快照事件只承载 `requestId` 与 `source`。session/run/task/broker/account/
  instrument/provider 在 Rust 传输层没有写入方。
- **context 语义**：`WithFields/WithRecorder/Detach/FieldsFromContext`、nil context 与
  `(*Recorder)(nil)` 的 no-op/Accepts 语义属 Go 运行时特有；Rust 关联字段按参数传递、
  recorder 始终存在（不可空句柄），因此 :148 与 `context_detach...:37` 记为 boundary，
  :10/:215 只保留可迁移的半边。
- **全局阈值与 critical 级**：Rust 无进程级可变全局与 critical 级记录生产者；全局旋钮是
  构造期环境变量，日志级别由 EnvFilter 控制。
- **OpenD 事件的 latency 字段**：Go 的 `Event` 有 `LatencyMS`，但 `operation`/`latency_ms`
  在 `RecordOpenDCall` 里只作为 slog 属性传给 `ErrorWithImportance`，不进入记录事件，
  因此 Rust 事件无需新增字段即可与 Go 的记录字段一致。

### 后续待办

- **锚点漂移（43 条 stale）**：第 97–99 批把测试模块抽到同目录 `*_tests.rs`（`#[path]`），
  代码锚点留在新文件而 `manual-test-mappings.json` 的 `rust_entry` 仍写生产文件名，导致
  `parity_anchor_reconcile.py` 报 stale 34 → 43（本批新增的 8 个锚点全部已记账，未计入）。
  修复方式：把对应行 `rust_entry` 补上 `*_tests.rs::<test>` 形式，或把锚点移回文件内的
  `mod tests` 声明处；建议作为下一批的机械清理项。
- `function_exact` 无锚点告警维持 196 条（本批新增 `[x]` 均已锚定），仍按批次登记增量。
- `partial` 无解析引用 7 条、无断言 `function_exact` 2 条为既有基线告警，本批未扩大。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-api --all-targets --locked`
  EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked`：
  66 passed / 0 skipped。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -p jftrade-kernel
  -p jftrade-integration-futu -p jftrade-engine --all-targets --locked --no-fail-fast`：
  2368 passed / 1 skipped / 0 failed（173s）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3005 Rust，`[x]` 1289、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、partial 无解析
  引用 7、无断言 2（均为既有基线，本批未扩大）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1298 唯一引用（已记账 1200、
  unrecorded 0、unknown 55、stale 43；本批 8 个锚点全部已记账，stale 增量来自第 97–99 批的
  `#[path]` 测试模块搬迁，见「后续待办」）。
- `pnpm run check:compatibility` EXIT=0（api-transport 278 operations / 18 route groups /
  19 probes，含 `request-observability.json` 冻结语料回放）。
- `pnpm run check:rust:architecture` EXIT=0；`node scripts/check-zero-go.mjs` EXIT=0
  （2905 tracked files）；`pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick`：首次因 `check:rust:target-health` 报 `target/debug/deps` 超过 5 万个
  `.rcgu.o`（实测 65829）失败；确认无 cargo/rustc 进程后执行 `pnpm run clean:rust:artifacts`
  （Removed 132943 files, 37.0GiB），重跑 EXIT=0（Node 套件 48 passed / 0 failed 等）。
- `pnpm run check:rust` EXIT=1，阻塞在 `check:rust:policy`（`cargo deny check`）：
  error[vulnerability] RUSTSEC-2026-0285（rustls TLS 1.3，修复需 >=0.23.45）与 8 条
  `warning[advisory-not-detected]` 陈旧 ignore。该 run 停在 policy，未执行 workspace 阶段；
  等价测试面由上面的 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致。
