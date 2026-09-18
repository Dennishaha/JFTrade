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
