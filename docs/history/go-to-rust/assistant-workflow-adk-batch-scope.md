# Assistant、Workflow、ADK 领域对齐批次

本批逐项核对 6 条 Assistant/ADK 高风险 Go 测试：空集合序列化、chat stream
事件 wire、非法 session 路由、超大 offset 分页钳制、断线后的 stream 重放，
以及 legacy assistant route 边界。

| Go 测试 | Rust 证据 | 结论 |
| --- | --- | --- |
| `internal/api/assistant/adk_normalize_test.go:15` | `product_adk_read_tests::adk_read_fixture_preserves_empty_collections_as_json_arrays` | `[~]`/`partial`：只覆盖 runs/sessions 数组，缺 run detail 中 toolCalls/artifacts 空数组和真实 HTTP route |
| `internal/api/assistant/adk_routes_test.go:245` | `product_adk_chat_stream_product_tests::adk_chat_stream_routes_register_only_with_explicit_test_port` | `[~]`/`partial`：SSE frame 有证据，但 idle timeout 与完整 session/run/final 事件集合不同 |
| `internal/api/assistant/adk_routes_test.go:787` | `product_adk_read_tests::adk_read_dynamic_routes_validate_suffixes_and_identifiers` | `[~]`/`partial`：非法标识符矩阵有证据，缺 session 创建/缺失 404/malformed rename route |
| `internal/api/assistant/routes_payload_pagination_test.go:65` | `product_adk_read_tests::adk_read_routes_clamp_pagination_beyond_available_items` | `[~]`/`partial`：三个集合均断言空页 metadata；Go 通过 HTTP router，装配不同 |
| `internal/api/assistant/chat_transport_disconnect_test.go:55` | `product_adk_chat_stream_product_tests::adk_chat_stream_replays_retained_terminal_events_through_adk_read_after_restart` | `[~]`/`partial`：断线后重放与终态保留有证据，缺 transport 写失败/goroutine 清理细节 |
| `internal/api/assistant/adk_ops_test.go:448` | `jftrade-api::transport_contracts::legacy_assistant_chat_route_is_strictly_not_found` | `[~]`/`partial`：Rust 额外锁定 JSON content-type 与无 port dispatch，Go 仅断言 404 |

验证命令均写入 `manual-test-mappings.json`；本批没有把部分 SSE、fixture 或
route 证据升级为完整等价。后续优先补 approval/session workflow lease、取消
恢复和断线写失败回归测试。

## 第二批：Assistant/ADK 路由、审批、会话与流式传输

批次范围（`internal/api/assistant/`，go:452dea11）：`adk_routes_test.go`（16）、
`adk_approval_test.go`（6）、`adk_sessions_test.go`（1）、`adk_ops_test.go`（7）、
`adk_transport_contracts_test.go`（1）、`adk_workflow_routes_test.go`（2）、
`adk_normalize_test.go`（1）、`adk_integration_test.go`（1）、
`chat_stream_lifecycle_test.go`（1）、`chat_transport_disconnect_test.go`（2），共 38 条。
逐条对照后：9 条升级为 `[x]`/`function_exact`，29 条保留 `[~]`（其中 1 条为
live-provider 边界保留），并新增 4 条 Rust 回归测试。

### 真实功能差异与修复（P0：公开 API 查询语义）

ADK 读路由此前把所有行都按分页切片返回，忽略了 Go 在持久化层执行的过滤谓词，
导致 `page.total` 与集合内容都与 Go 不一致：

1. **audit kind/subjectId 过滤缺失**
   - 复现：写入两条 `agent.saved`/`agent-audit` 与一条
     `provider.saved`/`provider-audit`，请求
     `GET /api/v1/adk/audit?kind=agent.saved&subjectId=agent-audit&limit=1`。
   - Go 预期：`page.total=2`、`page.returned=1`，仅返回 `agent.saved` 事件。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_adk_read.rs::audit`。
   - 回归测试：`product_adk_read_tests::adk_audit_route_filters_by_kind_and_subject_id`。

2. **runs status/agentId/sessionId 过滤缺失**
   - 复现：一条 `CANCELLED`/`agent-1`、一条 `COMPLETED`/`agent-2`，请求
     `GET /api/v1/adk/runs?status=CANCELLED&agentId=agent-1&limit=1`。
   - Go 预期：`page.total=1`，只返回 `run-cancel`。
   - 修复位置：同一文件的 `runs`。
   - 回归测试：`product_adk_read_tests::adk_runs_route_filters_by_status_and_agent_id`。

3. **approvals status/agentId 过滤缺失**
   - 复现：`PENDING`/`agent-a`、`PENDING`/`agent-b`、`APPROVED`/`agent-b`
     三条审批，请求 `GET /api/v1/adk/approvals?status=PENDING&agentId=agent-a`。
   - Go 预期：仅 `approval-a`（`ListApprovalsPage` 谓词）。
   - 修复位置：同一文件的 `approvals`。
   - 回归测试：`product_adk_read_tests::adk_approvals_route_filters_by_status_and_agent_id`。

4. **metrics 噪声查询参数**
   - 复现：`GET /api/v1/adk/metrics?limit=abc&offset=-1&status=FAILED`。
   - Go 预期：metrics 不接受分页参数，必须忽略并返回成功 envelope。
   - 回归测试：`product_adk_read_tests::adk_metrics_route_ignores_unexpected_query_params`
     （锁定 read owner 不得对 metrics 施加分页校验）。

### 边界与保留原因（部分覆盖）

- `TestADKProviderDeleteRejectsReferencedProvider`：已补测并升级为 `[x]`。
  Go 的 `DeleteProvider` 把 `ErrProviderInUse` 包装成引用 agent 名，
  `handleADKDeleteProvider` 以 `409 ADK_PROVIDER_DELETE_FAILED` 投影。
  Rust 原先返回 `409 ADK_PROVIDER_IN_USE` / "provider is referenced by an
  agent"，与 Go 不一致，已修正为
  `ADK_PROVIDER_DELETE_FAILED` / "provider is used by agent \"<name>\""；
  回归测试 `adk_provider_delete_reports_the_in_use_agent_and_keeps_the_go_projection`。
  同时补充 Go 的幂等删除语义（未知 provider 返回 `200 {"deleted":true,"id":...}`，
  而非 404）与 `ADK_PROVIDER_DELETE_FAILED` 成功 envelope 只含 `deleted`/`id`
  两个字段，回归测试
  `adk_provider_delete_is_idempotent_and_matches_the_go_success_envelope`。
- `TestADKAgentSaveValidationFailures`：已补测并升级为 `[x]`。新增
  `product_production_ports_adk_mutation_agent_validation.rs::validate_agent_write`
  承接 Go `service.validateAgent` 的 status/workMode/toolAccessMode 词表、
  `loopMaxIterations` 1..20 范围、provider 生命周期与 tool/skill 目录成员校验；
  回归测试 `adk_agent_write_reports_the_go_validation_messages` 逐条断言
  `400 BAD_REQUEST` 文案矩阵，`adk_agent_update_revalidates_the_merged_payload`
  断言更新路径复用同一 owner 且拒写不落库。
- `TestADKSkillInstallAndUninstallFailureRoutes`：已补测并升级为 `[x]`。
  install 非 URL 为 `400 ADK_SKILL_INSTALL_FAILED`，uninstall builtin 为
  `500 ADK_SKILL_UNINSTALL_FAILED`，缺失 skill 保留 Go 的
  `"file does not exist"` 投影；回归测试
  `adk_skill_install_and_uninstall_failures_keep_the_go_codes` 与
  `adk_skill_uninstall_removes_external_installs_and_reports_missing_files`。

### 本批次顺带修正的 wire 偏差

- ADK mutation 的通用 400 失败码原为 `ADK_INVALID_REQUEST`，该字符串在 Go
  代码库中并不存在。Go 各 handler 在 payload、路由标识与业务规则拒绝上统一
  返回 `400 BAD_REQUEST`，因此 Rust 的 `invalid_mutation_input` 已对齐为
  `BAD_REQUEST`。
- `value["id"] = ...` 在 `serde_json::Map` 上对缺失键会 panic。
  `jftrade-store-sqlite` 的 `set_default_provider_atomic` 与
  `delete_provider_with_replacement_atomic` 已改用 `Map::insert`，历史
  provider 行缺少 `id`/`default` 字段时不再中断写入事务。
- `TestADKChatReturnsCompletedEnvelopeWithVisibleToolFailure` 系列：工具失败以
  `FAILED` + `errorCode` 持久化有证据，但 Go 断言的
  `degraded=true` + `failureReason/errorCode` 为空 + `error` 含 "disk full"
  组合未逐条对齐。
- `TestHandlerCloseCancelsAndJoinsBackgroundExecutions`：取消有证据，
  “Close 必须 join 全部后台执行”的汇合语义无等价测试。
- `TestChatStreamReconnectAndReplayRespectClientDisconnect`：重放与重启保留有证据，
  重连期间再次断线的时序未覆盖。
- `TestRealADKChatStreamWithSavedProvider`：真实模型 Provider 端到端调用，
  按仓库规则只允许在显式 live workflow 验证，标记为边界保留。

### 结构整理

`product_production_ports_adk_read.rs` 因新增过滤逻辑超过 800 行生产文件上限，
按既有 `#[path = ...] mod` 模式把上下文估算、事件分类与 provider 密钥脱敏等
投影辅助函数拆分到
`crates/jftrade-engine/src/product_production_ports_adk_read_helpers.rs`
（119 行），主文件回到 720 行。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`
（1114 passed）、`jftrade-assistant`（33 passed）、`jftrade-api`（54 passed）、
`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`
（4451 Go / 1840 Rust，0 重复 `[x]`，0 非 function_exact 的 `[x]`）。

## 第三批：Assistant/ADK workflow 路由错误码与 webhook 认证

本批以 `go:452dea11` 的
`internal/api/assistant/adk_workflow_routes_test.go` 与
`internal/api/assistant/workflow_routes_test.go` 作为行为基线，冻结的
`tests/fixtures/compatibility/api-transport/adk-mutations.json` /
`adk-read.json` 作为 wire 证据，逐条核对 Go handler 的
`writeWorkflowError` 分类规则。

### 真实功能差异与修复

- workflow run / trigger run / webhook 三条运行时路由原先只复用
  `invalid_mutation_input` 与 `not_found_mutation`，错误码为
  `ADK_WORKFLOW_TRIGGER_NOT_FOUND` / `ADK_WORKFLOW_NOT_FOUND`。Go 的
  `writeWorkflowError` 保持路由自身 code，只决定状态：
  "not found" -> 404，"disabled"/"active" -> 409，其余 400。已改为
  `ADK_WORKFLOW_RUN_FAILED` / `ADK_WORKFLOW_TRIGGER_RUN_FAILED` /
  `ADK_WORKFLOW_WEBHOOK_FAILED`，并修正 webhook 三种投影：
  missing/non-webhook -> `404 workflow webhook not found`、disabled ->
  `400 workflow webhook is disabled`、错误 secret -> `401 invalid workflow
  webhook secret`（Go `handleADKWorkflowWebhook` 只把 secret 失败升到 401）。
  回归：`workflow_run_and_trigger_routes_keep_the_go_error_codes`、
  `disabled_workflow_and_webhook_routes_keep_the_go_error_codes`。
- workflow CRUD 与 trigger CRUD 的 not-found 投影原先统一返回
  `ADK_WORKFLOW_NOT_FOUND` / `ADK_WORKFLOW_TRIGGER_NOT_FOUND`，而 Go 的
  `handleADKSaveWorkflow` / `handleADKDeleteWorkflow` /
  `handleADKSaveWorkflowTrigger` / `handleADKDeleteWorkflowTrigger` 全部经
  `writeWorkflowError` 传出路由级 code。已改为按 operation 选择
  `ADK_WORKFLOW_SAVE_FAILED`、`ADK_WORKFLOW_DELETE_FAILED`、
  `ADK_WORKFLOW_TRIGGER_SAVE_FAILED`、`ADK_WORKFLOW_TRIGGER_DELETE_FAILED`。
  回归：`workflow_mutation_routes_keep_the_go_not_found_codes`。
- ADK read 投影把 missing task / workflow / trigger-list / session-context
  的资源级 404 压成通用 `NOT_FOUND`。Go 分别返回
  `ADK_TASK_NOT_FOUND`、`ADK_WORKFLOW_GET_FAILED`、
  `ADK_WORKFLOW_TRIGGER_LIST_FAILED`、`ADK_SESSION_CONTEXT_FAILED`
  （`handleADKSessionContext` 同样保留路由 code）。新增
  `product_production_ports_adk_projection.rs::not_found_with_code` 后逐条对齐。
  回归：`adk_read_resource_misses_keep_the_go_route_error_codes`。
- webhook 一次性 secret 生命周期补齐为函数级断言：
  `CreateWorkflowTrigger` 返回的 `trigger` 只带 `hasSecret=true` 且不泄露
  `secretHash`；错误 secret 为 `401 ADK_WORKFLOW_WEBHOOK_FAILED`；正确
  secret 运行成功并写入 `triggerType=webhook` 的触发日志。回归：
  `workflow_webhook_secret_lifecycle_stays_sanitized_and_authenticated`。

### 探针验证

临时改坏实现确认测试真实守卫后回滚：把 task 404 改回 `not_found` 后
`adk_read_resource_misses_keep_the_go_route_error_codes` 失败；把 disabled
webhook 改回 `invalid_mutation_input` 后
`disabled_workflow_and_webhook_routes_keep_the_go_error_codes` 失败；把
`route_code` 换成常量 `ADK_WORKFLOW_NOT_FOUND` 后
`workflow_mutation_routes_keep_the_go_not_found_codes` 失败。

### 清单状态

`manual-test-mappings.json` 本批升级 4 条 `[~]` -> `[x]`（均为
`function_exact`）：`adk_workflow_routes_test.go:15`、`:262`、
`workflow_routes_test.go:14`、`workflow_routes_test.go:185`。

验证：`cargo fmt --all`；`node scripts/quality/cargo-nextest.mjs run -p
jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`
（1485 passed）；`-p jftrade-api`（54 passed）；`pnpm run check:quick`；
`pnpm run check:zero-go`；`pnpm run check:compatibility`；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2500 Rust，
705 function_exact，0 重复 `[x]`，0 非 function_exact 的 `[x]`）；
`git diff --check`。`pnpm run check:rust` 的 advisories 阶段仍在干净 HEAD
上因既有 `RUSTSEC-2026-0285`（`rustls 0.23.44`）失败，未修改 `Cargo.lock`
规避。

### 边界与保留

- workflow 事件/调度触发与 background run 生命周期（`workflows_extended_test.go`、
  `workflow_async_tools_test.go`、`workflow_lifecycle_test.go` 等）仍属
  Go-owned Assistant runtime，本批只覆盖到达 Rust 生产端口的路由投影。
- `internal/assistant/engine` 的 497 条 `[~]` 尚未逐条映射，是
  Assistant/ADK 领域下一批的主要缺口。

## 第四批：ADK run input-response / cancel 路由错误码与标识符、查询编码边界

范围（`internal/api/assistant`，共 5 条 `[~]` → `[x]`）：

- `input_response_test.go:12 TestRunInputResponseErrorAndRetryContracts`
- `routes_test.go:101 TestRunInputResponseContract`
- `adk_routes_test.go:858 TestADKRunNegativeRoutes`
- `routes_identifier_validation_test.go:12 TestAssistantRoutesRejectBlankDecodedIdentifiers`
- `query_encoding_contracts_test.go:13 TestAssistantQueryRoutesRejectMalformedEncoding`

### 本轮发现并修复的真实功能差异（3 处）

1. `respond_to_input` 的 run 缺失分支原先返回
   `404 ADK_INPUT_REQUEST_NOT_FOUND`；Go 的 `ResolveRunInput` 把缺失 run 包成
   `ErrInputRequestNotFound`，而 `InputRequestErrorKind` 归类为 `not_found`，
   handler 写成 `404 NOT_FOUND` + 包装消息。现改为
   `404 NOT_FOUND` / `input request not found: {runId}`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_runtime.rs::respond_to_input`。
2. 同一函数的 `requestId` 与 run 上任何 input request 都不匹配时，原先返回
   `404 ADK_INPUT_REQUEST_NOT_FOUND`；Go 的 `resolveRunInputState` 用
   `ErrInputRequestConflict`，handler 映射为 `409 ADK_INPUT_RESPONSE_CONFLICT`。
   现已改为 `409 ADK_INPUT_RESPONSE_CONFLICT` / `input request conflict: request does not match run`。
3. `CancelRun` 的 run 缺失分支原先返回 `404 NOT_FOUND`；Go 的
   `handleADKCancelRun` 把所有 runtime 错误统一包装为
   `404 ADK_RUN_CANCEL_FAILED`（与 `GET /runs/{runId}` 保持的 `NOT_FOUND` 不同）。
   现改为 `404 ADK_RUN_CANCEL_FAILED` / `run not found`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_runs.rs`。

### 新增 Rust 测试（本轮）

- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs::adk_respond_to_input_maps_the_go_error_codes_and_retries`
- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs::adk_run_input_response_route_accepts_then_conflicts`
- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs::adk_cancel_run_missing_uses_the_go_cancel_error_code`
- `crates/jftrade-engine/src/product_adk_read_tests.rs::adk_read_routes_reject_blank_decoded_identifiers`
- `crates/jftrade-engine/src/product_adk_read_tests.rs::adk_read_routes_reject_malformed_query_encoding`
- `crates/jftrade-engine/tests/adk_mutations_compatibility.rs::adk_mutation_routes_reject_blank_decoded_identifiers`

### 探针（临时改坏实现 → 测试转红 → 还原）

- `decode_query(query).map_err(|_| query_failure(route))?` →
  `decode_query(query).unwrap_or_default()`：`adk_read_routes_reject_malformed_query_encoding` 转红。
- `validate_path` 去掉 `decoded.trim().is_empty()`：`adk_read_routes_reject_blank_decoded_identifiers` 转红。
- `decode_identifier` 去掉 `decoded.trim().is_empty()`：`adk_mutation_routes_reject_blank_decoded_identifiers` 转红（首轮探针误改 `raw.is_empty()` 分支，未命中，已重做）。
- `respond_to_input` 两处错误码分支各自回滚：`adk_respond_to_input_maps_the_go_error_codes_and_retries` 转红。
- `CancelRun` 回退为 `NOT_FOUND`：`adk_cancel_run_missing_uses_the_go_cancel_error_code` 转红。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：1368 passed。
- `pnpm run check:quick`（首轮因 `target/debug/deps` 的 `.rcgu.o` ≥ 50000 报 target-health 失败；确认无 cargo 进程后清理并重跑通过）
- `pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`：全部通过。
- 审计：710 function_exact / 2506 Rust tests；0 条非 function_exact 的 `[x]`，0 条重复 `[x]`。

### 边界与保留

- `internal/api/assistant/adk_integration_test.go:20 TestRealADKChatStreamWithSavedProvider`
  仍需真实模型 Provider，保持 boundary。
- `internal/api/assistant` 剩余 `[~]` 行（session/chat/SSE 流式与 approval 负路径等）
  继续由下一批处理。

## 第五批：ADK 读/写路由错误分类、session 更新业务码与 goal objective

范围（`internal/api/assistant`，共 2 条 `[~]` → `[x]`）：

- `routes_error_contracts_test.go:14 TestAssistantRoutesRejectInvalidQueriesPayloadsAndMissingResources`
- `routes_error_contracts_test.go:98 TestAssistantRoutesEnforceBusinessValidationOnUpdates`

### 本轮发现并修复的真实功能差异（4 处）

1. `GET /api/v1/adk/tasks?status=<未知>` 原返回 `400 BAD_REQUEST`
   (`invalid tasks query`)；Go 的 `ListTasks` 用 `ErrInvalidTaskStatus`，
   `handleADKTasks` 写 `400 ADK_TASK_LIST_FAILED` + `invalid task status "..."`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_read.rs`。
2. `PUT /api/v1/adk/sessions/{id}` 的空标题原返回 `400 BAD_REQUEST`；Go 的
   `handleADKRenameSession` 把所有失败写成 `400 ADK_SESSION_RENAME_FAILED`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_tasks.rs`。
3. `PATCH /api/v1/adk/sessions/{id}/composer-state` 的非法 workMode
   （`parallel` / `task`）原返回 `400 BAD_REQUEST`；Go 写成
   `400 ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation.rs::validate_optional_composer_mode`。
4. `PATCH /api/v1/adk/runs/{id}/objective` 的空 objective 原返回
   `400 BAD_REQUEST`；Go 写成 `400 ADK_RUN_OBJECTIVE_UPDATE_FAILED`。
   位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_tasks.rs`。

### 新增 Rust 测试（本轮）

- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs::adk_read_and_mutation_routes_keep_the_go_error_classification`
- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs::adk_session_and_run_update_routes_keep_the_go_business_error_codes`

### 探针（临时改坏实现 → 测试转红 → 还原）

- 未知 task status 回退为 `BAD_REQUEST`：读路由错误分类测试转红。
- rename 业务码回退：session/run 更新测试转红（`RenameSession` 行）。
- composer 业务码回退：同一测试转红（`UpdateSessionComposerState` 行）。
- objective 业务码回退：同一测试转红（`UpdateRunObjective` 行）。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`
  首次出现 1 条与本次改动无关的失败：
  `product::tests::market_data_quote_read_tests::candle_pagination_tests::us_regular_only_request_drops_an_extended_hours_current_bucket`，
  原因是该测试用 `now_utc()` 选锚点，在美东盘后窗口（当前 UTC 00:0x）会踩到日历无法分类的时段。
  把本次源码改动 `git stash` 后在干净 HEAD 上复现同样失败，确认是既有 clock-dependent flake，
  记入下一批 MarketData 范围处理，不计入本批通过项。
- `pnpm run check:zero-go`、`python3 scripts/compatibility/audit_test_parity.py`：通过（712 function_exact / 2508 Rust tests）。
- ADK 相关子集 124 passed。

## 批次小结：ADK chat 准入闸门与 Provider 默认回退（8 条 [x]）

本批把 `internal/api/assistant` 的 chat 错误分类家族与
`internal/assistant/engine` 的 Provider 解析家族一起收尾，共关闭 8 条 `[~]`。

### 两个真实功能缺口（先用 Go 探针取得 ground truth，再修 Rust）

1. **Provider 默认选择从未修复**（P0，影响 chat 可用性）
   - Go：`StoreCore.ListProviders` 按 `created_at ASC, id ASC` 取行 →
     `NormalizeDefaultProviderSelection`（非空表恰好一个 `default`，重复收敛为第一个，
     缺失则把第一行补为 default）→ 变更时 `saveProviderDefaultSelection` 持久化 →
     `SortProvidersDefaultFirst`（default 在前，其余按 `created_at ASC, id ASC`）。
     `DefaultProvider` 直接取 `ListProviders()[0]`。
   - Rust 原实现：`list_providers` 用 `ORDER BY created_at DESC`，既不修复也不排序；
     `resolve_provider` 只在 payload 显式带 `"default": true` 时才选中，
     否则报 `default agent provider is not configured`。
   - 影响：agent 未绑定 provider（`providerId` 为空）时，只要表里没有 default 标记，
     chat 直接 400；Go 会回退到第一个 provider 正常执行。
   - 探针（`/tmp/go452dea11.niwD1G` 实跑，probe 文件已删除）：
     - 单 provider 且无 default 标记 + 有 key → Go `200 ok=true`，
      `providerId="test-provider"`（probe provider_api3）；
     - 两 provider 且都无标记 → Go 列表 `default=true` 落在第一行，
      再 `default` 落到它（probe provider_api4）；
     - 清空 default 后单 provider 列表本身就会写成 `default=true`（probe provider）；
     - 无 provider → Go `400 ADK_CHAT_FAILED / default agent provider is not configured`
      （probe provider_api2，与 Rust 原行为一致，作为真实边界保留）。
   - 修复位置：`crates/jftrade-store-sqlite/src/adk.rs`
     （`list_providers` / `list_providers_created_first` /
     `selected_default_provider_id` / `persist_provider_default_selection` /
     `sort_providers_default_first`，写入走 `Immediate` 事务，未变更时不写库）。

2. **chat 并发闸门完全缺失**（P0，run 生命周期/资源保护）
   - Go：`Runtime.runSem = make(chan struct{}, MaxConcurrentRuns /* 10 */)`，
     `prepareChatRequest` 用非阻塞 `select` 取槽，满员报
     `maximum concurrent runs (10) reached, please try again later`；
     槽在 run 结束时 `defer <-r.runSem` 释放。
   - Rust 原实现：`rg 'Semaphore|max_concurrent|MAX_CONCURRENT'` 在 `crates/` 下无命中，
     即没有等价闸门，并发 run 数不受限。
   - 修复位置：`crates/jftrade-engine/src/product_adk_model_runtime.rs`
     （`MAX_CONCURRENT_RUNS`、`RunGate`/`RunGateGuard`，由 runtime 持有并随 facade 共享，
     `PreparedChat::New` 携带 guard 直到 run 结束；
     准入点在幂等 replay 判定之后、provider/agent 解析之前，与 Go `runChat` 一致）。

### 本批新增/修正的 Rust 测试

- `crates/jftrade-engine/src/product_adk_model_runtime_gate_tests.rs`（新文件）
  - `run_gate_rejects_the_eleventh_concurrent_run_and_releases_on_drop`
  - `run_gate_is_shared_across_runtime_facades`
  - `provider_list_reports_the_repaired_default_selection`
  - `resolve_provider_follows_the_default_selection_and_its_repair`
  - `agent_unavailable_reason_prefers_status_over_the_delete_marker`
- `crates/jftrade-store-sqlite/tests/adk_store_contracts.rs`
  - `provider_list_normalizes_default_selection_and_orders_default_first`
- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs`
  - `adk_chat_route_reports_the_go_error_classification`（在上一批基础上修正 fixture：
    provider 改为 closed loopback 端口，使 Provider 回退可观察；
    每次 dispatch 使用独立 `clientRequestId`，避免 idempotency 冲突掩盖分支）

### 探针（退化实现 → 测试转红 → 还原）

- probe A（移除 chat 消息长度校验）：`adk_chat_route_reports_the_go_error_classification`
  转红（实际得到 `agent provider API keys is not configured`，期望长度错误）。
- probe B（移除 `RunGate` 满员判定）：两个 `run_gate_*` 测试同时转红
  （第 11 次 acquire 竟然成功）。
- probe C（把 `list_providers` 退回 `created_at DESC` 且不做 default 修复）：
  store 契约测试与 `provider_list_reports_the_repaired_default_selection` 同时转红
  （顺序变成 `["provider-second","provider-first"]`）。

### 验证

- `cargo fmt --all`
- `cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`：通过
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1375 passed
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`：
  123 passed
- `pnpm run check:zero-go`、`pnpm run check:compatibility`：通过
- `python3 scripts/compatibility/audit_test_parity.py`：720 function_exact / 2516 Rust tests，
  0 条非 function_exact 的 `[x]`，0 条重复 `[x]`，0 条不存在的 crate 引用

### 状态

- `internal/api/assistant`：本批关闭 2 条（`routes_payload_pagination_test.go`），
  该目录剩余 `[~]` 49 条。
- `internal/assistant/engine`：本批关闭 5 条，`persistence/provider_selection_test.go` 2 条全部关闭。

## 第六批：工具失败不终止 run（degraded 语义，5 条 [x]）

范围：`internal/api/assistant/adk_routes_test.go:359/420/481`、
`internal/assistant/engine/runner_chat_test.go:423`、
`internal/assistant/engine/tools_test.go:775`，共 5 个 Go 复合键
（其中 359/420/481 原共享一条 Rust 测试，本批拆分为 3 条唯一命名测试，
故本批 `[x]` 由 720 增至 725，且 0 条重复 `rust_entry`）。

对照的 Go 行为基线：

- `googleADKExecution.afterToolCallback` 记录 `ToolCall.Error` 后吞掉工具错误，
  模型继续该轮；run 不会因单个工具失败而进入 FAILED。
- `MarkCompletedChatRun` 写入固定字面量 `message="completed"`，清空
  `FailureReason`/`ErrorCode`，并以 `FirstToolCallFailure(run) != ""` 推导
  `Degraded`。
- chat 路由直接回放存储信封；stream 路由把同一终态 run 投影成末帧 `final`，
  且不会伪造 `error` 帧；`publishTerminalError`/`RecoverTerminalChatResponse`
  在 final append 失败时从 durable run 重建终态。
- `classifyToolError` 分类表：`TOOL_EXECUTION_FAILED`、`TIMEOUT`、
  `CANCELLED`、`SUBMISSION_UNKNOWN`、`RUN_LEASE_LOST`，`ToolCall.Error`
  保留原始文本（`disk full`）而非前缀化的模型可见错误。

Rust 侧落点：

- `crates/jftrade-engine/src/product_adk_model_runtime_replay.rs`（新文件，
  经 `product_adk_model_runtime_adapters.rs` 的 `include!(...)` 保持同一模块
  作用域，以满足 product* 生产文件 800 行硬上限）：
  新增 `first_tool_call_failure`（等价 `FirstToolCallByStatus` +
  `ToolCallFailureMessage`）；`stream_from_payload` 在 durable 终态缺少
  final/error 帧时按存储 `response` 合成 `final` 帧。
- `crates/jftrade-engine/src/product_adk_model_runtime_tool_loop.rs`：
  新增 `tool_error_text`、`tool_failed`、`tool_result_error_code`/
  `tool_result_error_message` 与 `classify_tool_failure`。
- `crates/jftrade-engine/src/product_adk_model_runtime_stream.rs`：
  `persist_success` 写 `message="completed"`、保留 `toolCalls`、清空 run 级
  失败投影并置 `degraded = first_tool_call_failure(...)`；`persist_failure`
  置 `degraded=true`。
- `crates/jftrade-engine/src/product_adk_model_runtime_tool_loop.rs`：
  工具失败只落到 `ToolCall`（`FAILED` + 原始 error + errorCode），不再把 run
  整体置为失败；`tool_failed` 包装模型可见错误文本。
- `crates/jftrade-engine/src/product_adk_model_runtime_tool_failure_tests.rs`
  （新增测试模块，经 `product_adk_model_runtime.rs` 的
  `#[cfg(test)] #[path = ...]` 挂载）：
  `a_failed_tool_result_is_persisted_on_the_call_not_the_run`、
  `a_completed_run_with_a_failed_tool_replays_as_the_chat_envelope`、
  `a_completed_run_with_a_failed_tool_replays_as_a_stream_final_frame`、
  `a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame`、
  `persist_success_marks_a_run_degraded_from_its_failed_tool_calls`、
  `tool_failure_classification_matches_the_reference_table`。

功能差异与边界：

1. 权限模式（记录，不顺手改）：Go 的 `less_approval`/`all` 权限模式内联执行
   工具，Rust 目前一律把模型请求的调用落为 `PENDING_APPROVAL`。尝试端到端移植
   `TestChatContinuesAfterToolFailure`（真实 HTTP fixture）会因审批挂起超时，
   故本批删除该端到端测试并保留手工构建的 durable 形态测试。复现条件：以
   `less_approval` agent 发起带工具调用的 chat；预期行为：工具内联执行、失败只
   记在 `ToolCall` 上；修复位置：
   `product_adk_model_runtime_events.rs::persist_tool_calls`；回归要求：新增
   覆盖 `less_approval` 内联执行 + 失败后 run 仍 COMPLETED/degraded 的端到端测试。
2. `adk_routes_test.go:481` 的注入点：Go 通过
   `failingAppendSessionService` 让 ADK session `AppendEvent` 失败；Rust 的
   stream 持久化不经过 ADK session AppendEvent，因此不存在同构注入点。等价边界
   取「durable 终态 run 缺少 final 帧」这一可观察形态，见
   `a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame`。

探针结论（改坏实现 → 转红 → 还原）：

- 令 `stream_from_payload` 的终帧合成分支恒不进入，则
  `a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame` 失败
  （frames 长度 1 ≠ 2），证明该测试真实守卫恢复路径。
- 令 `first_tool_call_failure` 返回 `None`，则
  `persist_success_marks_a_run_degraded_from_its_failed_tool_calls` 失败。
- 令 `persist_success` 不清空 `failureReason`/`errorCode`/`errorStatus`，则同一
  测试失败。

本批验证命令（结果记录于本轮会话日志与 automation 更新）：

```bash
cargo fmt --all
node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast
pnpm run check:zero-go
pnpm run check:compatibility
python3 scripts/compatibility/audit_test_parity.py
git diff --check
pnpm run check:quick
```

`check:rust` 的 advisories 阶段在干净 HEAD 上亦失败（既有
RUSTSEC-2026-0285 / rustls 0.23.44），本批未改 `Cargo.lock` 规避。

附带修复：`pnpm run check:zero-go` 在上一批提交（47583d80）后失败，命中项全部
来自 `scripts/compatibility/parity_anchor_reconcile.py`、
`parity_gap_triage.py`、`test_parity_anchor_reconcile.py` 注释里的
"Go test" 相邻措辞。本批把这些注释改写为 "reference test"，不改任何脚本逻辑，
门禁恢复通过（2832 tracked files, 0 release artifact）。

剩余 follow-up（未在本批闭环，保持 `[~]`）：`adk_ops_test.go:394`、
`adk_approval_test.go:450`、`adk_ops_test.go:468`，以及
`internal/api/assistant` 其余 `[~]` 行；下一批进入
`internal/assistant/engine`。

## 第七批：ADK 负路径 follow-up 闭环（optimization task / child run / snapshot 组合，3 条 [x]）

范围：上一批挂账的三条 follow-up，逐条决断为「修实现 + 回归测试」：

1. `internal/api/assistant/adk_ops_test.go:394 TestADKOptimizationTaskNegativeRoutes`
   - Go 行为（`internal/api/assistant/observability.go`）：读取与取消两个
     handler 都把 service 的 "not found" 归一化为
     `404 NOT_FOUND` / "optimization task not found"；`%zz` 这种无法解码的
     `:taskId` 在 handler 入口即 `400 BAD_REQUEST` / "taskId is invalid"。
   - 发现的真实差异：Rust cancel 分支返回 `ADK_OPTIMIZATION_TASK_NOT_FOUND`，
     与同一条 Go handler 的通用 `NOT_FOUND` 不一致。
     修复位置：`product_production_ports_adk_mutation_tasks.rs` 的
     `AdkMutationOperation::CancelOptimizationTask` 三个 not-found 分支。
   - 回归测试：`product_adk_mutation_product_tests.rs::optimization_task_negative_routes_match_the_reference_matrix`，
     在真实产品装配下单测试断言 GET 与 cancel 两条错误矩阵（400/404 × code/message）。
   - 探针：把 cancel 的 code 改回 `ADK_OPTIMIZATION_TASK_NOT_FOUND` → 测试转红（缺任务断言），还原后通过。

2. `internal/api/assistant/adk_approval_test.go:450 TestADKRunPauseResumeRoutesRejectInvalidRuns`
   - Go 行为：loop 模式 child run（`parentRunId` 非空）pause 返回 400；
     缺失 run 的 resume 返回 `404 NOT_FOUND`。
   - Rust 此前只在 `validate_goal_run` 中实现 child 分支，没有独立回归测试。
   - 回归测试：`product_adk_mutation_product_tests.rs::goal_pause_rejects_child_runs_and_resume_reports_missing_runs`，
     种子 child run 走真实 HTTP 路由，断言
     `400 ADK_RUN_PAUSE_FAILED` / "only root goal runs can be paused"
     与 `404 NOT_FOUND` / "run not found"。
   - 探针：令 `validate_goal_run` 的 `parentRunId` 判定恒不成立 → 测试转红（期望 400 实得 200），还原后通过。

3. `internal/api/assistant/adk_ops_test.go:468 TestADKSnapshotAndToolsRoutesReturnCatalogData`
   - Go 行为：`/api/v1/adk` 一次返回 providers/agents/skills/tools 与持久化
     runtimeSettings，`/api/v1/adk/tools` 返回同一工具目录。
   - 回归测试：`product_production_assembly_tests.rs::adk_snapshot_and_tools_routes_return_the_composed_catalog`，
     在生产端口装配下同时断言两个路由：snapshot 四个集合非空、种子 provider
     的 `requestTimeoutMs=240000` 保留、`PUT /api/v1/settings/adk` 写入的
     `runTimeoutMs=660000`/`streamIdleTimeoutMs=420000` 回读一致、
     以及 `/api/v1/adk/tools` 与 `snapshot.tools` 的 id 集合完全相等。
   - 探针：令 snapshot 的 `tools` 恒为空数组 → 测试转红（非空数组断言），还原后通过。

### 验证

- `cargo fmt --all`
- `python3 scripts/compatibility/audit_test_parity.py`：728 function_exact，
  0 条非 function_exact 的 `[x]`，0 条重复 `[x]`
- 三条回归测试在还原实现后全部通过（见上）

### 状态

- `internal/api/assistant`：本批关闭 3 条 follow-up；`adk_ops_test.go` 与
  `adk_approval_test.go` 的已知挂账已清空，该目录剩余 `[~]` 43 条。

## 第八批：`internal/api/assistant/routes_test.go` 剩余 8 条 + 重连契约（9 条 [x]）

批次范围（`internal/api/assistant/`，go:452dea11）：`routes_test.go` 全量 9 条
（`TestCatalogSessionRunAndObservabilityContracts`、
`TestAgentSaveErrorClassification`、`TestChatStreamHubReplayAndCleanupBoundaries`、
`TestSessionTimelineFailureKeepsLegacyErrorCode`、`TestChatAndSSEContracts`、
`TestChatRequestIdempotencyContracts`、`TestChatRequestUsesDeclaredMessageFieldOnly`、
`TestApprovalContract`）以及 `routes_resource_contracts_test.go:410`
`TestStreamReconnectAndSkillContracts`。本批全部升为 `[x]`/`function_exact`，
`internal/api/assistant/routes_test.go` 已无 `[~]` 残留。

### 功能差异与修复

1. **stream 重连缺少 `replay:true` 标记（新增 2 条回归测试）**
   - Go 行为：`streamADKChatRecord(..., replay=true)` 给重连客户端收到的每个
     retained 帧打 `replay:true`；`GET /api/v1/adk/streams/{streamId}?after=` 与
     `GET /api/v1/adk/runs/{runId}/stream?after=` 都走这条路径，未知流/run 返回 404。
   - Rust 此前两条重连路径都只返回裸事件，丢掉该标记。
   - 修复位置：`product_adk_model_runtime_replay.rs`（`mark_replayed`，含
     `stream_from_payload` 合成终帧路径）与
     `product_production_ports_adk_read.rs`（`stream_snapshot` 的 `replayed_event`）。
   - 回归测试：
     `product_adk_model_runtime_recovery_tests.rs::replayed_stream_payloads_carry_the_go_replay_marker`、
     `product_adk_model_runtime_recovery_tests.rs::recovered_terminal_stream_frame_carries_the_replay_marker`、
     `product_production_ports_adk_tests.rs::adk_stream_reconnect_routes_carry_replay_markers_and_fail_closed`。
   - 探针：令两个标记 helper 直接 `clone()` 返回 → 读取路由测试与两条投影测试转红，还原后通过。

2. **live `/chat/stream` 缺少首帧 `session` 事件（新增 1 条端到端测试）**
   - Go 行为：`executeADKChatStream` 在跑模型前先 `previewSession()` 发 `session`
     帧，再发 `run` 快照，最后 `final`；控制台用首帧的 session id 绑定 transcript。
   - Rust 此前 live 流只发 `run`。
   - 修复位置：`product_adk_model_runtime_stream.rs` 的 `start_live_stream`
     （抽出 `emit_preview_session` 到 `product_adk_model_runtime_stream_events.rs`）。
   - 回归测试：
     `product_adk_chat_stream_product_tests.rs::production_live_chat_stream_emits_session_before_run_and_terminal_frame`
     驱动真实 `ProductionAdkChatRuntime` + 真实 HTTP 路由（模型端点是已关闭的
     loopback 端口，失败快且确定），断言首帧为 `session` 且携带持久化 session id。
   - 探针：把首帧 `"type"` 改成别的字符串 → 测试转红（顺序断言），还原后通过。

3. **agent 写失败被误分类为 400**
   - Go 行为：`isADKAgentValidationError` 只把识别到的验证文案归为 400，存储失败
     必须是 500。
   - 修复位置：`product_production_ports_adk_mutation_entities.rs` 的三处 agent
     create/update/delete 写失败改为 `agent_write_store_failure`（内部
     `storage_mutation_failed`）。
   - 回归测试：`adk_agent_save_storage_failure_is_not_client_classified`（`DROP TABLE`
     制造真实写失败，断言 500 `ADK_MUTATION_FAILED`，同时保留 `provider not found` 的 400）。

4. **session timeline 读取失败的错误码**
   - Go 行为：`TestSessionTimelineFailureKeepsLegacyErrorCode` 要求
     `500 ADK_MESSAGES_GET_FAILED`，而不是通用的读不可用。
   - 修复位置：`product_production_ports_adk_read.rs` 的 session detail timeline
     读取改为 `AdkReadSnapshotError::Failed{status:500,code:"ADK_MESSAGES_GET_FAILED"}`。
   - 回归测试：`adk_session_timeline_failure_keeps_the_legacy_messages_error_code`。

### 新增行为断言（无生产差异，仅补证据）

- `catalog_session_run_and_observability_routes_answer_ok`：同一次装配下逐条断言 Go
  的 12 条读路由 + `DeleteProvider` 成功。
- `adk_chat_idempotency_contract_matches_the_go_routes`：missing/invalid
  `clientRequestId` → 400 `BAD_REQUEST`/"clientRequestId must be a valid UUID"；
  相同 body 重放持久化 run（不新建）、payload 变化 → 409
  `ADK_CHAT_IDEMPOTENCY_CONFLICT`；stream 路由复用同一 `X-ADK-Stream-ID` 并同样 409。
- `adk_chat_request_uses_only_the_declared_message_field`：仅 `prompt`、仅 `text`、
  两者同时存在、以及 `message` 为空白并带 `prompt` 的 payload 全部按声明字段判空，
  返回 400 `ADK_CHAT_FAILED`/"message is required"。
- `adk_chat_approval_is_listed_as_pending_and_denied_with_ok_envelope`：真实 runtime
  下 `approvals?status=PENDING` 只返回该 pending 项，`POST /approvals/{id}/deny`
  走 `ResolveAndStageApproval("DENIED")` 返回解析结果，deny 后不再出现在 PENDING。
- `internal/api/assistant` 内 `TestChatStreamHubReplayAndCleanupBoundaries` 判定为
  「结构边界保留」：Go 的 hub 是进程内生命周期 owner，Rust 的等价模型是
  durable replay（只服务持久化历史、终态后可重放、重启后一致），故映射到上述
  retained-replay 证据而不是复制内存 hub。

### 重建与拆分

- `product_adk_model_runtime_stream.rs` 触及 800 行上限，把 `tail_existing_stream`
  与新的 `emit_preview_session` 迁到同模块的
  `product_adk_model_runtime_stream_events.rs`；`runner` 与 `tail` 仍在同一模块
  作用域内，未改变所有权或锁顺序。
- `product_adk_model_runtime_recovery_tests.rs` 承接两条 replay 标记回归测试，
  生产文件保持 800 行以内。

### 验证

- `cargo fmt --all`
- `cargo clippy -p jftrade-engine --all-targets --locked`：通过
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1397 passed / 0 failed
- `pnpm run check:zero-go`、`pnpm run check:compatibility`、
  `pnpm run check:rust:architecture`：通过
- `python3 scripts/compatibility/audit_test_parity.py`：4451 条基线映射中 737 条
  `function_exact`，0 条缺 `function_exact` 的 `[x]`，0 条重复 `rust_entry`，
  Rust 测试总数 2536
- `pnpm run check:quick`：通过（含 1417 条 Rust 测试与 98 条前端测试）

### 状态

- `internal/api/assistant/routes_test.go`：9/9 条升为 `[x]`，本批关闭该文件全部挂账。
- `internal/api/assistant`：剩余 `[~]` 34 条，集中在 `adk_ops_test.go`、
  `adk_sessions_test.go`、`chat_stream_recovery_contracts_test.go`、
  `routes_resource_contracts_test.go` 其余资源契约与 `service_*_test.go`。

## 第九批：stream 成功路径 P0 修复 + 边界/恢复契约（6 条 [x]）

批次范围（`internal/api/assistant/`，go:452dea11）：`adk_routes_test.go:245`、
`adk_approval_test.go` 之外的首批 P0/P1 边界项，以及
`routes_boundary_contracts_test.go:15`、`chat_transport_disconnect_test.go:55`、
`chat_stream_recovery_contracts_test.go:41`。

### 功能差异与修复

1. **成功 live 流在 `run` 之后中止，永不发 `final`（P0，本批最重要）**
   - 复现：真实 provider（loopback AI Platform 兼容端点返回
     `response.output_text.delta` + `response.completed`）驱动
     `POST /api/v1/adk/chat/stream`，SSE 只到 `session`、`run` 两帧就结束。
   - 根因：`jftrade-api` 的 `ApiStreamSender::send` 无条件调用
     `blocking_send`。provider 读线程运行在 Tokio 运行时内（嵌套 current-thread
     runtime），`blocking_send` 在该上下文 panic，生产者的终帧写入被 panic 吞掉。
   - 预期行为：`session → run → final` 三帧齐全，`final.response.run.agentId`
     为已解析 agent、`reply` 携带模型文本。
   - 修复位置：`crates/jftrade-api/src/ports.rs`。发送改为 `try_send` 优先；
     运行时**外**仍用 `blocking_send` 保留背压契约；运行时**内**改为让渡重试，
     上限 30s，超时或通道关闭返回 `Err` 以取消生产 run，而不是 panic 或永久阻塞。
   - 回归测试：
     `product_adk_chat_stream_product_tests.rs::production_live_chat_stream_emits_session_run_and_final_events`
     （新增 `spawn_loopback_model_provider`，按 `Content-Length` 读完请求再回 SSE，
     `join()` provider 线程）。
   - 探针：把 `send()` 还原为 `blocking_send` → 测试转红（缺 `final`），还原后通过。

2. **已注入但不可用的 ADK 端口必须全线失败关闭**
   - Go 在 runtime 为 nil 时仍注册全部 ADK 路由并统一回 503。
   - Rust 组合根按端口注入决定注册（未注入=未注册），等价保证落在「已注入但不可用」：
     新增 `wired_but_unavailable_adk_ports_fail_closed_on_every_route`，对 Go 矩阵中
     Rust 拥有的 24 条读路由逐条断言 `503 ADK_READ_UNAVAILABLE`，并对 9 条写路由经
     真实 mutation wire 分发断言 `503 ADK_MUTATIONS_UNAVAILABLE`。
   - 探针：把 `snapshot_failure` 的 Unavailable 状态码改成 200 → 测试转红，还原后通过。

3. **持久化终态 run 必须以 `final` 恢复、不得再发第二个 `error`**
   - 新增
     `product_adk_model_runtime_recovery_tests.rs::persisted_terminal_run_is_published_as_final_instead_of_a_second_error`，
     断言终态投影中 `types` 恰有一个 `final`、无 `error`、末帧 `response.run.id` 正确。
   - 探针：禁用 `stream_from_payload` 的终帧合成分支 → 测试转红，还原后通过。

### 边界判定

- `TestChatStreamHubKeepsEventAndTimelineContracts` 的「不可序列化 tool output 不得
  丢弃事件」判定为边界保留：Go 侧 `cloneADKChatStreamEvent` 是进程内 hub 的
  JSON 往返克隆兜底，Rust 的事件在写入时已是 `serde_json::Value`（无 Go 的
  `func()` 字段），不存在同等结构；对应保证由 durable 事件写入路径覆盖。

### 验证

- `cargo fmt --all`；`cargo clippy -p jftrade-engine -p jftrade-api --all-targets --locked` 通过
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-api --all-targets --locked --no-fail-fast`：
  1454 + 54 passed / 0 failed
- `python3 scripts/compatibility/audit_test_parity.py`：741 function_exact，
  0 条缺 `function_exact` 的 `[x]`，0 条重复 `rust_entry`
- `git diff --check` 通过

### 并发 wakeup 修复（本轮追加）

`pnpm run check:rust:workspace` 并行跑 engine 时暴露出
`adk_approval_negative_and_idempotent_routes_match_the_go_envelopes` 的间歇失败，
根因是真实功能差异而不是测试抖动：

- 复现条件：`resolve_and_stage_approval` 已把 run 从 `PENDING` 提交为
  `RUNNING`，但路由尚未调用 `runtime.resume_approval`；此时 durable recovery
  scanner（或浏览器重试）先 claim 了同一条 continuation。
- 预期行为（Go `ResolveApprovalAsync`）：`claimApprovalContinuation` 失败即
  “已有 owner 在飞”，属于幂等 no-op，路由仍返回 resolution envelope；在飞
  owner 重读已 resolve 的 run 并执行释放的 tool call，approved tool 只执行一次。
- Rust 现状（修复前）：`resume_approval` 返回 `Conflict`，approve/deny 分支
  无条件 `rollback_staged_approval` 后回 `503 ADK_CONTINUATION_UNAVAILABLE`，
  把正在继续的 run 误报为不可用。
- 修复位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_runs.rs`
  的 approve/deny 分支——仅对真正的启动失败（`Unavailable`/`Failed`）回滚，
  `Conflict`（已有 in-flight claim）照常投影 resolution envelope。
- 回归测试：`product_production_ports_adk_tests.rs::adk_approval_wakeup_accepts_an_already_claimed_continuation`
  （冲突 runtime + 真实 store，断言 envelope 与 approval 不回退 PENDING）。
  探针：把 `Conflict` 分支恢复为无条件回滚，该测试转红。
- 同时把 `adk_approval_approve_returns_the_running_resolution_envelope` 的
  fixture 改成记录型 continuation runtime，消除真实 provider 调用覆写
  `resumeState` 造成的抖动；真实 provider 端到端仍由
  `production_live_chat_stream_emits_session_run_and_final_events` 覆盖。

### 状态

- `internal/api/assistant` 剩余 30 条 `[~]`（adk_approval 2、adk_integration 1、
  adk_routes 3、catalog_failure 1、chat_helpers 8、chat_stream_lifecycle 1、
  chat_stream_recovery 1、chat_transport_disconnect 1、routes_boundary 5、
  routes_payload_pagination 3、routes_resource_contracts 4）。
- 本轮追加关闭 `internal/assistant/engine/runner_approval_concurrency_test.go:119`
  `TestConcurrentSiblingAsyncApprovalsEnqueueOneContinuation`（同批修复的并发
  wakeup 语义）。

## 第十一批：internal/assistant/engine/store_ops_test.go 首批（7 条 [x]，含 3 个真实功能缺口）

批次范围：`internal/assistant/engine/store_ops_test.go:281/294/320/363/402/939/961`
（resolveSession 归属、agent 软删除生命周期、取消联动 deny、memory 注入、
agent 工具可见范围）。所有 `[x]` 的 `rust_entry` 均全局唯一。

### 修复的真实功能差异

1. **agent 删除是软删除（P0，历史记录与恢复）**
   - Go：`StoreCore.DeleteAgent` 保留行、`Status=DISABLED`、写 `deletedAt`；
     `ListAgents` 过滤 `DeletedAt != nil`，`ListAllAgents` 保留；`SaveAgent`
     重建整行从而清除 `deletedAt`。
   - Rust 现状（修复前）：`crates/jftrade-store-sqlite/src/adk.rs::delete_agent`
     执行 `DELETE FROM adk_agents` 物理删除，读路由也不过滤 `deletedAt`。
   - 修复：`delete_agent` 改为事务内读 payload→写 `DISABLED`/`deletedAt`/
     `updatedAt` 的 UPDATE（缺失时仍 `Ok(false)`）；读 owner 新增
     `active_agent_rows()` 在 `snapshot()` 与 `agents()` 过滤软删行；
     `UpdateAgent`/`CreateAgent` 合并 payload 前移除 `deletedAt` 实现恢复；
     `DeleteProvider` 的引用检查也不再被已软删 agent 永久占用。
   - 回归：`adk_agent_delete_soft_deletes_the_historical_row`、
     `adk_agent_listing_excludes_soft_deleted_rows_but_keeps_history`、
     `adk_agent_save_restores_a_soft_deleted_row`。
2. **chat 的 session 归属校验（P0，身份/数据隔离）**
   - Go：`Runtime.resolveSession` 对显式 `sessionId` 要求已存在且 `AgentID`
     相同，否则 `session not found` / `session belongs to a different agent`。
   - Rust 现状（修复前）：`prepare_chat` 无条件 `upsert_session`，会把已有
     session 静默改绑到新 agent，缺失也会被创建。
   - 修复：`product_adk_model_runtime_events.rs` 在写入前读取
     `get_session_agent_id`，不匹配即 `400 ADK_CHAT_FAILED`；显式 id 缺失同样
     报错。
   - 回归：`chat_rejects_a_session_owned_by_a_different_agent`。
     探针：临时禁用 guard → 测试转红（断言 status 400 失败），恢复后通过。
3. **取消 run 必须 deny 其 pending approvals（P0，资金/审批安全）**
   - Go：`Runtime.cancelRunTree` 通过 `SaveRunAndDenyPendingApprovals` 在
     同一事务写终态 run 并把该 run 的 `PENDING` approval 置 `DENIED`。
   - Rust 现状（修复前）：取消路由只 CAS 写 run 行，approval 仍是 `PENDING`
     且可在终态 run 上被 approve。
   - 修复：新增
     `AdkStore::cancel_run_and_deny_pending_approvals`（run CAS + approval
     `UPDATE ... WHERE run_id=? AND status='PENDING'`，含 payload
     `json_set`），取消路由改走该原子路径；`run_state_result` 的重复实现
     `run_state_result_if_status` 删除。
   - 回归：`cancelling_a_run_denies_its_pending_approvals`。
     探针：把取消路由还原为 `run_state_result`（仅写 run） → 测试转红
     （approval 仍为 PENDING），恢复后通过。
4. **memory 注入（P1，上下文正确性）**
   - Go：`Runtime.prepareAgent` 仅在 `MemoryEnabled` 时追加
     `

JFTrade memory:
` + `agentMemoryPrompt`（workspace + 本 agent 行，
     4000 rune 截断）。
   - Rust 现状（修复前）：完全没有 memory prompt 注入路径。
   - 修复：`product_adk_model_runtime_lifecycle.rs` 新增
     `agent_memory_prompt()` 并在 `resolve_provider` 组装 instruction 时按
     `memoryEnabled` 注入。
   - 回归：`chat_injects_memory_into_the_instruction_only_when_enabled`。
5. **agent 工具可见范围（P1，权限边界）**
   - Go：`ToolDescriptorsForAgent` 按 `toolAccessMode`/`tools` 归一化
     （selected 允许列表、none 全隐藏、all/空列表全放行），
     `strategy.research_backtest` 与 `strategy.optimize` 额外隐含
     `backtest.kline_sync_status`。
   - Rust 现状（修复前）：`prepare_chat` 与 resume 路径把整个 catalog 下发
     给模型，agent 的 `tools`/`toolAccessMode` 完全不影响可见工具。
   - 修复：新增 `AgentToolScope` 与 `agent_tool_scope(payload)`，两条
     `openai_tools()` 构造路径都按 scope 过滤；`ResolvedProvider` 携带已解析
     agent payload，避免二次读库。
   - 回归：`agent_tool_scope_follows_the_go_access_mode_normalization`
     （selected/none/all/显式 all/隐式 selected/optimize 六个分支）。
     过程回归：该改动最初让 `adk_chat_route_reports_the_go_error_classification`
     转红（scope 查询假定 agent 必然存在，而该夹具只有 provider），据此把
     scope 改为纯函数式归一化，不再回查 store。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`：
  1549 passed（修改前基线 1543；含 6 条新增回归）。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(adk) | test(chat)'`：
  176 passed（改动过程中一度 1 failed，定位为 scope 回查 store 的回归，已修）。
- `cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`：通过。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 条，`[x]` 771
  （本批净增 7），0 非 function_exact 的 `[x]`，0 重复引用，
  `function_exact` 引用全部可解析。
- `pnpm run check:rust:architecture`：通过。过程中 `product_adk_model_runtime_events.rs`
  与 `product_production_ports_adk_read.rs` 因新增逻辑超过 800 行，已抽出
  `product_adk_model_runtime_tool_persistence.rs` 与
  `product_production_ports_adk_read_listings.rs` 两个 include 片段。
- `pnpm run check:zero-go`、`pnpm run check:compatibility`、`git diff --check`：通过。

### 状态

- `internal/assistant/engine/store_ops_test.go` 剩余 19 条 `[~]`；下一批继续该
  文件（run/approval 恢复族与 task/memory/tool 审批族），随后转入
  `store_test.go` / `runner_chat_test.go` / `tools_test.go`。

## 第十二批：store_ops_test.go 收尾（26 条全部结清；14 条 [x]、5 条边界/partial、7 条复检）

范围：`internal/assistant/engine/store_ops_test.go` 剩余 19 条 `[~]`，外加本批修完
功能缺口后顺带结清的 `internal/assistant/engine/tools_test.go` 5 条审批策略用例。
本批结束时该文件的 26 条全部有结论，`[x]` 从 771 → 790（净增 19）。

### 真实功能缺口与修复（4 处）

1. **approval continuation 的终局 resumeState（P1，控制台/指标正确性）**
   - Go：`hydrateResumedRun` 把续跑收尾的 run 标成 `resumeState=adk_confirmation_resolved`
     并写 `completedAt`；`service_metrics.go` 用该字段统计 `resumed`。
   - Rust 现状（修复前）：`persist_success` 完全丢弃 resume state，续跑与普通 chat
     在控制台和指标里无法区分。修复：`ChatExecution` 新增 `resumed: bool`
     （`product_adk_model_runtime.rs`），新 run 传 `false`、resume 路径传 `true`
     （`product_adk_model_runtime_events.rs`），`persist_success` 据此写
     `resumeState` 与 `completedAt`（`product_adk_model_runtime_stream.rs`）。
   - 回归：`product_adk_model_runtime_tool_failure_tests.rs::a_resumed_approval_run_completes_with_the_confirmation_resolved_state`。

2. **孤立 pending run 的启动期 reconcile（P0，恢复/回滚）**
   - Go：`reconcileStaleRuns` 把“PENDING 但审批行没有 ADK confirmation id”的 run
     标成 `FAILED/RUN_ORPHANED` + `resumeState=approval_context_missing`；带 id 的
     pending run 继续留给审批续跑。
   - Rust 现状（修复前）：这类 run 永远停在 PENDING，控制台显示为“等待审批”，
     实际再也没有唤醒路径。修复：`product_adk_model_runtime_lifecycle.rs` 新增
     `reconcile_orphaned_pending_runs()`，在构造函数里于
     `recover_approval_continuations()` **之前**调用；写状态走既有
     `update_run_state_if_status_and_revision` CAS，不引入第二个写入者。
   - 回归：`product_adk_model_runtime_lifecycle_tests.rs::orphaned_pending_approval_runs_are_failed_on_startup_reconcile`
     （同时断言可恢复的兄弟 run 保持 PENDING）。
   - 探针：移除该 reconcile 调用 → 孤立 run 停在 PENDING，测试转红。

3. **task 依赖/planner warnings 的规范化（P1，数据一致性）**
   - Go：`SaveTask` 对 `DependsOn` 与 `PlannerWarnings` 走 `NormalizeStringSlice`
     （trim、丢空、去重、排序），create 与 patch 都适用。
   - Rust 现状（修复前）：原样保存调用方列表，并且**拒绝**空白项——与 Go 的
     “静默丢空”相反。修复：新增 `normalized_string_slice()` 并在 create/patch 的
     `dependsOn`/`plannerWarnings` 两处调用；`string_slice` 不再把空串当错误。
   - 回归：`product_production_ports_adk_tests.rs::adk_task_normalization_and_validation_match_go`。
   - 探针：把 create 的 `depends_on` 改回 `string_slice` → 断言 `["task-a","task-z"]`
     得到 `["task-z","task-a","task-z",""]`，测试转红。

4. **builtin skill 被外部安装隐藏（P1，目录投影）**
   - Go：`ListSkills` 始终把内置 bundle 与外部安装一起返回。
   - Rust 现状（修复前）：`skills()` 只在 store 为空时投影 builtins，任何一次外部
     安装都会让内置技能从 `GET /api/v1/adk/skills` 消失（Agent 面板随即丢失
     `jftrade-market` 等条目）。修复：按 id 合并两个来源，并按 Go 的顺序
     （builtin 优先、组内按 displayName）排序。
   - 回归：`product_production_ports_adk_tests.rs::builtin_skill_catalog_stays_registered_alongside_external_installs`。
   - 探针：恢复修复前的 `if skills.is_empty()` 分支 → `an external install must not
     hide jftrade-market: ["neodata-financial-search"]`，测试转红。

### 补齐的纯函数：ToolRequiresApproval（P1，审批策略）

`internal/assistant/engine/store_ops_test.go:998` 与 `tools_test.go` 的 5 条审批策略
用例都锚定 `assistantmodel.ToolRequiresApproval`。Rust 侧此前**没有这个函数**，
`permissionMode` 在整个 runtime 里没有任何读取点。

本批把它按 Go 语义补进 `crates/jftrade-assistant`（assistant 领域唯一 owner）的
`model::tool_policy`：`normalize_permission_mode`、`tool_requires_approval`、
`tool_allowed_in_mode`，覆盖显式豁免名单、`RequiresApprovalIn`、medium/high/critical
风险、五类写权限、`create_strategy_instance`、`live_trading`，并对未知/空模式
归一化为 `approval`（fail-closed）。7 条单元测试逐项冻结。

**遗留接线（下一批 P0）**：runtime 的 `prepare_chat`/`resume_approval` 目前仍对所有
`tool_executor.supports()` 的工具一律 stage 成 `PENDING_APPROVAL`，尚未读取
`provider.agent_payload["permissionMode"]` 与 descriptor 的 risk/permission 做分流。
因此 `docs/adk.md` 承诺的“低风险读取自动执行 / `less_approval` 减少普通写入审批 /
`all` 尽量自动执行”在 Rust 侧还没有生效——这是功能缺口而不是测试缺口，需要把
`tool_policy` 接进 `product_adk_model_runtime_tool_persistence.rs` 的 staging 判定，
并按 Go 在无审批时直接进入工具执行循环（`run_approval_continuation` 的等价路径）。

### 边界/partial 结论（5 条）

- `TestStoreBuiltinSkillsSplitStrategySkill`、`TestBuiltinSkillStoreMetadataComesFromBundleRegistry`、
  `TestBuiltinStrategySkillRefreshesOutdatedBundle`、`TestBuiltinRefreshDoesNotOverrideNonBuiltinSkill`
  记为 boundary：Go 的 builtin 技能是文件系统 bundle registry 物化出的持久行
  （有 `contentHash`/`InstallPath`，会依据版本刷新磁盘文件），Rust builtin 是
  `BUILTIN_SKILL_DEFINITIONS` 的纯投影、没有磁盘副本，因此“store 里有该行”
  “刷新过期 bundle”“外部文件不被覆盖”这三类断言在 Rust 结构上不适用。可迁移的
  部分（内置目录存在、source/builtin/validationStatus/version、tools 来自 bundle
  分类映射、外部安装是追加而非替换）已由 `builtin_skill_catalog_stays_registered_alongside_external_installs`
  覆盖。
- `TestInstallSkillURLInstallsNeodataFinancialSearch` 记为 partial：Rust 覆盖
  frontmatter 解析、URL 形状校验、`SKILL.md` 落盘与 catalog 注册；Go 额外用
  httptest 服务器 + 临时清空 `SkillInstallHostValidator` 验证真实 HTTP 下载，
  Rust 没有在测试里起回环 HTTP 服务器（下载前半由 `parsed_for_download_host`
  纯函数测试承担）。

### 新增/迁移的 Rust 测试

- `crates/jftrade-engine/src/product_adk_model_runtime_lifecycle_tests.rs`（新文件）：
  从 `product_adk_model_runtime_lifecycle.rs` 拆出原 `mod tests`，新增
  `orphaned_pending_approval_runs_are_failed_on_startup_reconcile` 与
  `an_approval_resuming_run_is_recovered_after_a_runtime_restart`；后者用 loopback
  provider 驱动真实 provider 调用，断言被释放的工具恰好执行一次、run 收尾为
  `COMPLETED`/`adk_confirmation_resolved`，且已解析审批不再出现在 session timeline。
- `crates/jftrade-engine/src/product_adk_model_runtime_tool_failure_tests.rs`：
  `a_resumed_approval_run_completes_with_the_confirmation_resolved_state`。
- `crates/jftrade-engine/src/product_production_ports_adk_tests.rs`：
  `adk_resolve_approval_missing_returns_the_idempotent_empty_envelope`、
  `adk_run_listing_filters_and_sorts_newest_first`、
  `adk_duplicate_approval_resolution_is_a_noop`、
  `adk_multiple_approvals_continue_only_after_all_are_approved`、
  `adk_task_normalization_and_validation_match_go`、
  `adk_memory_filters_and_agent_validation_match_go`、
  `adk_cancel_run_missing_is_the_dedicated_cancel_failure`、
  `builtin_skill_catalog_stays_registered_alongside_external_installs`。
- `crates/jftrade-engine/src/product_production_ports_adk_mutation_skill_tests.rs`（新文件）：
  `skill_archive_install_preserves_resources_and_metadata`、
  `skill_document_install_registers_the_parsed_document`、
  `skill_install_is_exclusive_per_id`、
  `skill_archive_rejects_unsafe_paths_and_symlinks`、
  `skill_install_projection_is_durable`；`install_skill` 拆出纯下载半段
  `install_skill_document` 以便独立测试。
- `crates/jftrade-store-sqlite/tests/adk_store_contracts.rs`：
  `adk_approval_resolution_missing_and_non_pending_rows_are_idempotent`（store 层
  缺失 → `Ok(None)`、终态不回退、重复 resolve 不二次 stage）。
- `crates/jftrade-assistant/src/model.rs`：`tool_policy` 模块 + 7 条策略测试。

### 结构整理

- `product_adk_model_runtime_lifecycle.rs` 因新增 reconcile 逻辑超过 800 行，
  测试模块按仓库既有 `#[path]` 约定拆到
  `product_adk_model_runtime_lifecycle_tests.rs`。
- `product_production_ports_adk_mutation.rs` 超过 800 行，`normalized_string_slice` /
  `string_slice` / `reject_self_dependency` 三个字符串规范化辅助迁移到已有的
  `product_production_ports_adk_mutation_helpers.rs`。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite -p jftrade-assistant --all-targets --locked --no-fail-fast`：
  1606 passed，0 failed（修改前基线 1564）。
- `cargo clippy -p jftrade-engine -p jftrade-store-sqlite -p jftrade-assistant --all-targets --locked`：通过。
- `cargo fmt --all -- --check`：通过。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 条，`[x]` 790
  （本批净增 19），0 非 function_exact 的 `[x]`，0 重复引用，`function_exact`
  引用全部可解析。
- `pnpm run check:rust:architecture`：通过。
- `pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:quick`、
  `git diff --check`：通过。

### 状态

- `internal/assistant/engine/store_ops_test.go`：26/26 已结清（21 `[x]`、4 boundary、
  1 partial）。
- `internal/assistant/engine/tools_test.go`：`[x]` 6/21（本批新增 5 条审批策略），
  剩余 15 条集中在 `workflow.wait`、`http.fetch` 安全分类、task schema 字段、
  account.orders 慢端口与 stream 边界，下一批继续该文件后再转入 `store_test.go` /
  `runner_chat_test.go`。
- 下一批 P0：把 `tool_policy` 接进 `product_adk_model_runtime_tool_persistence.rs`，
  让 `permissionMode` 真正决定哪些工具直接执行、哪些进入审批。

## 第十九批：把 tool_policy 真正接进 ADK 运行时（permissionMode 生效）

本批把上批只存在于纯函数层的 `ToolRequiresApproval` 接到运行时，修复一个真实功能缺口：修前
`persist_tool_calls` 对任何 `tool_executor.supports()` 命中的工具一律 stage 成
`PENDING_APPROVAL`，完全不读 `provider.agent_payload["permissionMode"]`，因此 approval 模式下
73/75 个本应自动执行的读取工具都会要求操作员审批，与 `docs/adk.md` 承诺的权限模式语义不符。

### 功能修复

- `product_adk_model_runtime_tool_persistence.rs`：新增 `ToolCallStaging::{Pending,Released}`；
  staging 结果按 `tool_catalog.requires_approval(name, permission_mode)` 区分挂起与释放。
  非审批调用写 `RUNNING` 并原样继续工具循环，只有真正门控的调用才写
  `PENDING_APPROVAL` + `pendingApprovals` 并把 run 置 `PENDING`。
- 修复 `known` 的语义错误：可用性只看 `tool_executor.supports()`，`replay_safe_tool()` 只决定
  claim 的 `fail_closed`。修前把两者合并，导致任何非 replay-safe 工具（含 `live_trading` 类）
  都会以 503 `ADK_TOOL_UNAVAILABLE` 被拒，永远到不了审批队列。
- 审批挂起投影对齐 Go `finishPendingApprovalRun`：`resumeState=waiting_approval`、
  `message=等待用户审批后继续执行。`、`reply=我已经准备好执行需要授权的操作，请先在 ADK 审批队列里确认或拒绝。`
- `product_adk_model_runtime_tool_loop.rs`：抽出 `run_tool_loop(&self, chat, cancellation, run_lease)`
  复用同一执行循环；`run_approval_continuation` 自行取租约后调用，chat/stream 路径借用调用方已持有的
  租约（fencing token 不改变）。循环内 `Released` 继续下一轮、`Pending` 停止。
- `product_adk_model_runtime_stream.rs` / `product_adk_model_runtime_events.rs`：chat 与 live stream
  在 `Released` 时继续执行并回传终态投影，不再把 release 当作终态 `final`。
- `product_production_ports_adk_catalog.rs` + `product_production_ports_adk_policy.rs`：目录输出真实
  `permission`/`riskLevel`/`requiresApprovalIn`；新增 `requires_approval()`（缺描述符 fail-closed）；
  策略片段拆到独立 include 文件以守住 product* 800 行上限（主文件 800 行）。

### 新增回归（`product_adk_model_runtime_catalog_policy_tests.rs`，7 条）

- `read_tool_runs_without_approval_in_approval_mode`：approval 模式读工具 `Released`，run 保持
  `RUNNING`，`pendingApprovals` 为空，`toolCall.status=RUNNING`。
- `blank_permission_mode_still_releases_reads`：缺省/空 `permissionMode` 归一化为 `approval`。
- `unknown_permission_mode_normalizes_to_approval`：`NormalizePermissionMode` 等价性（含 `all` 与
  `less_approval` 保留）。
- `unknown_tool_stays_unavailable`：目录外工具仍以 503 `ADK_TOOL_UNAVAILABLE` fail-closed。
- `released_calls_are_executed_by_the_tool_loop`：释放后的调用被工具循环真实执行且写入 `toolResults`。
- `gated_call_persists_the_go_approval_projection`：门控调用写入 Go 的
  `waiting_approval`/中文 message/审批提示语/`pendingApprovals` 投影。
- `live_trading_call_is_gated_in_every_mode`：`live_trading` 在 approval/less_approval/all 都要求审批，
  运行时写成 `PENDING_APPROVAL`。

### 探针证据

- 探针 A：把 staging 判定改回「一律 gated」→ `read_tool_runs_without_approval_in_approval_mode`、
  `blank_permission_mode_still_releases_reads`、`released_calls_are_executed_by_the_tool_loop` 三条转红。
- 探针 B：把 `known` 改回 `replay_safe_tool(name) && supports(name)` →
  `gated_call_persists_the_go_approval_projection` 转红（503 而非审批挂起）。
- 两次探针均为临时替换生产实现，验证后立即从备份恢复，`git diff` 只剩有意变更。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1448 passed，0 failed（基线 1446，新增 2 条净计测试）。
- `cargo clippy -p jftrade-engine --all-targets --locked`：通过。
- `cargo fmt --all`、`pnpm run check:rust:architecture`（product* 800 行以内）：通过。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go 测试 / `[x]` 790、
  0 重复 `[x]`、0 断裂引用；本批把 `internal/assistant/engine/tools_test.go` 的两条映射从纯策略
  证据升级为运行时行为证据（`read_tool_runs_without_approval_in_approval_mode`、
  `live_trading_call_is_gated_in_every_mode`）。
- 说明：一次全量运行出现 `adk_lease_fencing_takeover_edge_conditions::
  test_lease_expiration_boundary_before_and_after_expiry` 的时间边界抖动（该用例与本次改动无关），
  单独复跑与随后两次全量复跑均通过。

### 下一批

- `internal/assistant/engine/tools_test.go` 剩余 14 条 `[~]`：`workflow.wait` 4 条、`http.fetch`
  安全分类 3 条、task schema 1 条、`account.orders` 慢端口/stream 3 条、`models.list` schema 1 条、
  backtest companion 1 条、descriptor access mode 1 条。
- 之后转 `internal/assistant/engine/runner_chat_test.go`（20 条）→ `store_test.go`（20 条）→
  `session_context_test.go`（19 条）。

## 第二十批：workflow.wait 生产适配器与可取消等待（tools_test.go 剩余 4 条结清）

Go 的 `workflow.wait` 是注册在工具目录里的 `read_internal`/low 工具（
`internal/assistant/engine/tools.go:126` 注册、`tool_net.go:90` 实现）。Rust 侧此前只有
`PRODUCTION_TOOL_DEFINITIONS` 目录项与 `product_wire.rs` 名单，没有执行适配器，模型一旦调用就会
失败关闭为不可用——属于真实功能缺失而非测试缺口。

### 功能修复

- `product_mcp_production_dispatch.rs`：新增 `workflow.wait` 到 `supports()` 与
  `execute_production()`；实现 `workflow_wait_duration()`（`durationMs` 优先，`seconds` 接受
  int/float/数字字符串；正值校验与 25s 上限，错误文本对齐 Go）与
  `workflow_wait_cancellable()`（分片等待并轮询取消信号，取消返回
  `499 MCP_TOOL_CANCELLED`）。
- `product_adk_tool_executor.rs`：`AdkToolExecutor` 新增带默认实现的 `execute_cancellable()`；
  只有 `workflow.wait` 走可取消实现，其余适配器保持原行为不变。
- `product_adk_model_runtime_tool_loop.rs`：工具执行改用 `execute_cancellable()`，把 run 的取消
  信号与 durable CANCELLED 状态一起传入，等价于 Go handler 观察 `ctx.Done()`。

### 新增回归（`product_mcp_server_tests.rs`，4 条）

- `workflow_wait_tool_waits_and_does_not_require_approval`：ADK 目录投影 `read_internal`/low、
  approval 模式 `requires_approval=false`；执行 `durationMs=10` 返回 `reason` 与 `waitedMs>=10`。
- `workflow_wait_tool_rejects_too_long_duration`：`seconds=26` 返回 400 且文案含 `25s`。
- `workflow_wait_tool_returns_context_cancellation`：取消后立即返回 `MCP_TOOL_CANCELLED`，
  断言 5s 等待在取消后远早于满额时间结束。
- `workflow_wait_duration_parses_multiple_input_forms`：`durationMs` 优先、1.5/2/`"0.25"` 解析、
  25s 边界、空串/`later`/0/25001ms 拒绝。

### 探针证据

把 `workflow_wait_cancellable` 的两处 `cancelled()` 判定改成恒假后，
`workflow_wait_tool_returns_context_cancellation` 转红并报告等待跑满 7048ms 才返回；
从备份恢复后 4 条全绿。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1452 passed，0 failed（上一批 1448，本批 +4）。
- `cargo clippy -p jftrade-engine --all-targets --locked`、`cargo fmt --all`、
  `pnpm run check:rust:architecture`：全部通过。
- `python3 scripts/compatibility/audit_test_parity.py`：`[x]` 790 → **794**，0 重复 `[x]`，
  0 断裂引用，0 缺失 evidence_type。

### 下一批

`internal/assistant/engine/tools_test.go` 剩余 10 条 `[~]`：`http.fetch` 安全分类 3 条、
task schema 1 条、`models.list` schema 1 条、tool registry 序列化 1 条、`account.orders`
慢端口/stream 3 条、backtest companion 1 条、descriptor access mode 1 条。

## 第二十一批：http.fetch 生产适配器与 SSRF 防护（tools_test.go 再结清 3 条）

Go 的 `http.fetch`（`internal/assistant/engine/tool_net.go:14`）是模型可见的
`read_external`/medium 工具，且被内置 `external-http` skill 绑定。Rust 侧目录、执行器与安全策略
全部缺失，模型调用会直接不可用；内置 `external-http` skill 也因此解析出空工具集。

### 功能修复

- `product_mcp_production_dispatch.rs`：新增 `http.fetch` 到 `supports()` 与
  `execute_production()`；实现 `reject_unsafe_host()`（空 host / localhost / `.localhost` /
  私网、回环、link-local、multicast、unspecified 与 169.254.169.254 全部拒绝，文案对齐 Go）、
  `unsafe_address()`（IPv4 与 IPv6 的 fc00::/7、fe80::/10 等范围）、`http_fetch()`（http/https
  限定、12s 超时、User-Agent `JFTrade-ADK/1.0`、最多 5 次重定向且每次重定向都重新做主机安全校验、
  text/json/xml/rss 内容类型白名单、1MiB 默认上限）与 `http_fetch_envelope()`。
- `build_http_fetch_client()` 在构建前显式安装 rustls ring provider：engine 把 reqwest 固定为
  `rustls-no-provider`，缺这一步会在 client 构建时 panic 而不是 fail-closed（探针时发现）。
- 目录登记：`http.fetch` 进入 `PRODUCTION_TOOL_DEFINITIONS`、`MODEL_EXPOSED_TOOLS` 与
  `REPLAY_SAFE_TOOL_ALLOWLIST`，权限策略为 Go 的 `read_external`/medium（无显式模式列表）。
- 更新 `production_builtin_skills_project_bound_tool_catalog`：`external-http` 现在解析出
  `["http.fetch"]`，与 Go `BuildSingleFileBuiltinSkill("external-http", ..., []string{"http.fetch"}, ...)`
  一致（此前断言空数组是功能缺失时的产物）。

### 新增回归（`product_mcp_server_tests.rs`，6 条）

- `http_fetch_tool_rejects_invalid_and_unsafe_targets`：9 组非法/不安全输入与文案。
- `reject_unsafe_host_and_unsafe_addr_classification`：空 host、公网放行、10 组 IPv4 分类。
- `http_fetch_tool_handles_responses_without_real_network`：响应封装与截断语义。
- `http_fetch_redirect_guard_blocks_unsafe_hosts`：重定向目标安全校验。
- `http_fetch_catalog_registration_matches_the_reference`：read_external/medium 与审批模式差异。
- `http_fetch_installs_the_rustls_provider_before_building_the_client`：确定性客户端构建（无网络 I/O）。

### 探针证据

把 `unsafe_address` 的判定恒假、并让 localhost 分支恒不触发后，3 条安全测试同时转红；
从备份恢复后 6 条全绿。探针过程中还暴露了 rustls provider 缺失导致的 reqwest panic，
已按仓库既有模式（skill 下载客户端）在构建前安装 ring provider 修复。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1458 passed，0 failed（上一批 1452，本批 +6）。
- `cargo clippy -p jftrade-engine --all-targets --locked`、`cargo fmt --all`、
  `pnpm run check:rust:architecture`：全部通过。
- `python3 scripts/compatibility/audit_test_parity.py`：`[x]` 794 → **797**，0 重复 `[x]`，
  0 断裂引用。

### 下一批

`internal/assistant/engine/tools_test.go` 剩余 7 条 `[~]`：task schema 1 条、`models.list`
schema 1 条、tool registry 序列化 1 条、`account.orders` 慢端口/stream 3 条、
backtest companion 1 条、descriptor access mode 1 条。

## 第二十二批：本地工具 schema 与访问模式投影（tools_test.go 再结清 4 条）

### 功能修复

- `product_mcp_schema_catalog_dispatch.rs`：为本地工具补审查 schema——`models.list`
  （query/providerId/callableOnly/limit）、`workflow.wait`（seconds/durationMs/reason，含 25s 与
  25000ms 上限）、`http.fetch`（url 必填、maxBytes 1..1MiB）。此前 ADK 投影对这些工具退回通用
  占位 schema，缺具体字段且不体现上限。
- 三个 `[x]` 复用已有运行时断言（目录 `requiresApprovalIn` 为数组、`workflow.wait`/`system.status`
  为 `[]`；toolAccessMode 三态投影；backtest companion 规则），另补两条独立回归避免同一
  Rust 入口被多条 Go 测试引用。

### 新增回归

- `product_mcp_protocol_tests.rs::models_list_schema_is_safe_and_complete`：四个字段齐备、
  `additionalProperties=false`、schema 文本不含 apiKey。
- `product_mcp_protocol_tests.rs::local_tool_schemas_are_reviewed`：workflow.wait/http.fetch 的
  字段与上限。
- `product_adk_model_runtime_gate_tests.rs::explicit_access_modes_project_their_declared_tool_sets`：
  all/selected/none 三态投影矩阵。
- `product_adk_model_runtime_gate_tests.rs::selected_backtest_tools_gain_the_kline_sync_companion`：
  research_backtest/optimize 隐含 companion，无关选择不隐含，none 不隐含。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：
  1462 passed，0 failed（上一批 1458，本批 +4）。
- `cargo fmt --all`、`pnpm run check:rust:architecture`：通过。
- `python3 scripts/compatibility/audit_test_parity.py`：`[x]` 797 → **801**，0 重复 `[x]`，
  0 断裂引用。

### 下一批

`internal/assistant/engine/tools_test.go` 剩余 3 条 `[~]`，全部围绕 30 秒工具超时与不挂起契约：
`TestAccountOrdersCompletesWithoutHanging`(468) / `TestAccountOrdersWithSlowPortfolioSummary`(605) /
`TestAccountOrdersStreamCompletes`(837)。已确认 Rust 侧缺 Go `executeRegisteredTool` 的
30s `context.WithTimeout` 与 panic 恢复（`product_adk_tool_executor.rs` 无超时包装、
`classify_tool_failure` 只有 MODEL_CALL_TIMEOUT 映射到 TIMEOUT）。实现要点：在工具执行边界加
30s 超时（结果映射 `TIMEOUT`/retryable）与 panic 捕获（`tool panic: ...`），并让
`account.orders` 与慢 `portfolio.summary` 并发时互不阻塞；补超时/panic/慢端口回归后用探针验证。

## 第二十三批：internal/assistant/engine/tools_test.go 结清（3 条超时/不挂起 + 1 条 schema）

批次范围：`internal/assistant/engine/tools_test.go` 剩余 4 条 `[~]`，全部结清。`tools_test.go` 现为 21/21 `[x]`。

### 功能缺口与修复

Go `executeRegisteredTool`（`internal/assistant/engine/tools.go:373`）对每个注册 handler 做两件 Rust 此前完全缺失的事：

1. `context.WithTimeout(ctx, 30*time.Second)`：handler 超过 30s 必须返回 `context.DeadlineExceeded`。
2. `recover()` → `tool panic: %v`：handler panic 不能杀死 run，只能变成 tool call 上的错误文本。

`classifyToolError`（`internal/assistant/engine/runner_tools.go:485`）再把 `DeadlineExceeded` 映射为 `("TIMEOUT", true)`、`Canceled` 映射为 `("CANCELLED", false)`。

Rust 修复（`crates/jftrade-engine`）：

- 新增 `AdkToolExecutor::execution_deadline()`（默认 30s），生产实现沿用该默认值，工具循环按执行器声明的 deadline 运行，避免把常量埋在调用点。
- 新增 `execute_tool_with_timeout()`（`product_adk_model_runtime_tool_loop.rs`）：把 handler 放到具名工作线程执行，`catch_unwind` 捕获 panic 转 `tool panic: <text>`；主循环以 50ms 片轮询，同时观察取消信号与 deadline，deadline 到期返回 `context deadline exceeded`/`TOOL_EXECUTION_TIMEOUT`，取消返回 `context canceled`/`TOOL_EXECUTION_CANCELLED`；handler 返回后再复查一次 context（对齐 Go 的“返回后重新读 ctx.Err()”语义）。超时路径故意 detach worker（Go 同样无法杀死 goroutine），持久 claim fencing 已阻止陈旧 owner 提交迟到结果。
- `classify_tool_failure` 增加 `TOOL_EXECUTION_TIMEOUT → ("TIMEOUT", true)`、`TOOL_EXECUTION_CANCELLED → ("CANCELLED", false)`；`RUN_CANCELLED`/`CLIENT_DISCONNECTED` 原映射保持不变。
- `run_tool_loop_stream` 提权为 `pub(super)`，供本批流式回归直接驱动。
- 超时/panic 辅助拆到 `product_adk_model_runtime_tool_timeout.rs` 并以 `include!` 引入，`product_adk_model_runtime_tool_loop.rs` 保持 762 行（低于 800 行生产上限）。

### 新增回归（8 条，`product_adk_model_runtime_tool_deadline_tests.rs`）

| Rust 测试 | 对应 Go 测试 | 断言要点 |
| --- | --- | --- |
| `account_orders_completes_without_hanging` | `tools_test.go:468` | 5 个 console 读工具在一次 `run_tool_loop` 内全部执行、各自落盘 `SUCCEEDED`，整体 <30s |
| `account_orders_completes_with_a_slow_portfolio_summary` | `tools_test.go:605` | 慢 `portfolio.summary`（500ms）不阻塞 `account.orders`（fast read elapsed <400ms），两条调用都落盘 |
| `account_orders_stream_completes` | `tools_test.go:837` | 流式路径复用有界 tool loop，stream run 的两条调用落盘且 worker <30s 结束 |
| `a_hanging_tool_is_bounded_and_classified_as_a_timeout` | 派生（30s 语义） | 200ms deadline 下挂起工具被截断，分类 `("TIMEOUT", true)` |
| `a_panicking_tool_becomes_a_visible_tool_panic_failure` | 派生（recover 语义） | 错误文本为 `tool panic: split brain detector exploded`，分类 `TOOL_EXECUTION_FAILED` |
| `a_cancelled_tool_call_reports_the_context_cancellation` | 派生（ctx 取消） | 取消在 <2s 内打断等待，分类 `("CANCELLED", false)` |
| `the_default_tool_execution_deadline_is_thirty_seconds` | 派生（常量冻结） | executor 与生产 executor 的 deadline 均为 30s |
| `an_expired_tool_deadline_is_projected_onto_the_tool_call` | 派生（落盘投影） | deadline 到期写入 `status=FAILED`、`errorCode=TIMEOUT`、`error="context deadline exceeded"`、`error.retryable=true`，run 级 `failureReason` 保持空 |

### 探针证据

- 探针 A（把 deadline 分支改成固定 30s 循环）：6 条测试中的超时/取消用例挂住不返回，进程需外部终止 → 说明 deadline 分支是唯一的时间边界，测试确实依赖它。
- 探针 B（panic payload 文本丢弃）：`a_panicking_tool_becomes_a_visible_tool_panic_failure` 转红（`left: "tool failed"`，`right: "tool panic: split brain detector exploded"`）。
- 探针 C（删除循环内与返回后的取消复查）：`a_cancelled_tool_call_reports_the_context_cancellation` 转红并跑满 30s（`cancellation must interrupt the wait instead of running to the deadline`）。
- 探针均以临时备份 `cp` 恢复，恢复后 `grep` 复核 `if remaining.is_zero()`、`panic_text(payload)`、双重取消复查均存在。

### 第 4 条：tasks schema（`tools_test.go:44`）

Go 的 `skillsruntime.DefaultToolInputSchema("tasks.create"/"tasks.update")` 必须含 10 个 planner 投影字段（order/modeHint/agentRole/plannerStepId/planSource/workflowMode/objective/childProviderId/childModel/plannerWarnings）。

Rust 侧 `tasks.create/update` 属未实现的写工具（不在 `PRODUCTION_TOOL_DEFINITIONS`，因此没有 registry descriptor 可断言），但字段语义由 mutation 路径守护：`product_production_ports_adk_mutation_tasks.rs` 逐字段读取/规范化，`product_production_ports_adk_tests.rs::adk_task_normalization_and_validation_match_go` 断言 `order`、`modeHint`、`agentRole`、`plannerStepId`、`planSource`、`workflowMode`、`objective` 原样往返，`plannerWarnings` 与 `dependsOn` 一同 trim/dedupe/sort；`childProviderId`/`childModel` 由 `TestTaskAndMemoryCRUDContracts` 对应回归断言。本行以 `local_tool_schemas_are_reviewed` + `adk_task_normalization_and_validation_match_go` 记为 function_exact，并在结论中写明“Rust 无 tasks descriptor，字段等价由 mutation 契约守护”。

### 验证

- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`：通过（clippy 0 错误）。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1470 passed / 0 failed**（上批基线 1462，本批 +8）。
- `pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`、`pnpm run check:rust:architecture`、`git diff --check`：通过。
- 映射清单：`[x]` 801 → **805**，`function_exact` 805，0 重复 rust_entry，key 集合仍为 4451。

### 下批目标

`internal/assistant/engine/runner_chat_test.go`（24 条，余 20 条 `[~]`）：先读 `runner_chat_test.go:56/69/102/127/156/181` 与 `internal/assistant/engine/runner_chat.go`，逐条比对 `hydrateRunExecutionResult`、`completeChatRun`、`markFailedChatRun`、`persistRunTerminalState`、`attachFinalAssistantMessage` 与 Rust 的 `persist_success`/`persist_failure`/`persisted_turn_response`。重点核对：tool-only run 合成 final reply、run 级终态与审计事件的原子写入、pending approval 的 assistant prompt 落盘、run handle 生命周期（`startRun`/`finishRun`/`cancelRun` 终态 noop）。
