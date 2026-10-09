# Assistant、Workflow、ADK 领域对齐批次

本批复核冻结 `internal/assistant/engine/skill_reg_test.go:115`。
真实 URL 下载 owner 新增缺失 `SKILL.md`、多根歧义、损坏 ZIP、超过 4 MiB
archive 与超过 512 KiB 文档的逐项失败断言，并保持 rows/audit/skills 目录零写入。
下载层读取 4 MiB+1 后交给 archive/document owner，恢复 Go 的 archive/document
错误分类；定向 nextest 4/4 passed，receipt
`sha256:6cfb60ffb253cdc5c9eb55994bbde17be1bc1f634239e7f048cc5cb25a8112cf`。
映射由 partial 升 `function_exact`，当前 **1676 exact / 2138 partial / 637 boundary**；
reuse 保持单引用 `allowed=false`，source/diff/anchor 现场复核通过。quick 门禁已通过；
两次现场 `CARGO_INCREMENTAL=0 pnpm run check:rust` 均仅在 `cargo deny` 拉取
RustSec advisory DB 时因 GitHub 网络不可达退出，失败证据保留。整体未完成，继续 ADK
重连写失败、预取消和断连终态 owner 对齐。

原安装owner现保留所有catalog内置Skill ID，外部单文档/ZIP下载在文件、SQLite前返回400重复安装并保持目录投影。九条原函数分类仍1675/2139/637；补充builtin collision不升级原函数。定向273、quick2481 Rust/98 Pine/desktop11+48、现场完整Rust4085 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。三份receipt、两份gate、1061指纹与anchor/reuse核验；整体未完成，继续archive边界优先级和资源投影。详见[积压清单](parity-evidence-backlog.md)。

缺失Skill卸载由原registry owner保留typed NotFound源错误，原port继续500/code/message；missing/builtin拒绝保持完整rows/audit/文档bytes。七条原函数复核，skill_reg286整组闭合升exact；fs89旧archive错误归因换为真实下载后file-root拒绝，仍partial。当前1675/2139/637。最终定向272、quick2480 Rust/98 Pine/desktop11+48、现场完整Rust4084 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。五份receipt、三份gate、1061指纹与reuse/anchor复核；DNS、目录/nil、内置资源与跨系统恢复仍开放，整体未完成，继续内置skill ID安装保护。详见[积压清单](parity-evidence-backlog.md)。

Skill生产下载总截止时间由30秒对齐冻结Go的20秒。真实TCP握手与暂停时钟验证headers/body/跨redirect超时及19秒成功，400分类与完整rows/audit/文件保持；同一异步下载/安装owner由原同步mutation驱动。定向271、quick2479 Rust/98 Pine/desktop11+48及现场完整Rust4083 passed/0 failed/2 skipped、static/七类replay通过，无LEAK。五份receipt、两份gate、1061指纹与六条原函数/reuse/anchor核验，分类1674/2140/637保持；编译、旧30秒、首次timeout分类和工具截断失败留证。blocking DNS shutdown不能保证整个mutation的20秒返回上限，typed missing等缺口保持；整体未完成，继续缺失卸载类型化源错误。详见[积压清单](parity-evidence-backlog.md)。

真实URL下载修复安全302停止与409/502安装错误，原owner逐跳验证地址、固定连接、保留最初source，失败统一400；private/localhost/metadata/FTP在解析/连接前拒绝。六条冻结原函数复核，235/224完整闭合升exact，当前1674/2140/637；162目录/nil、286 typed os.ErrNotExist仍partial。定向267、quick2475 Rust/98 Pine/desktop11+48与现场完整Rust4079 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。五份receipt、两份gate、1060指纹及六处reuse/anchor核验，编译/行为红留证。Go20秒/Rust30秒、DNS取消与跨系统恢复继续开放；整体未完成，继续生产下载默认超时。详见[积压清单](parity-evidence-backlog.md)。

Skill卸载权限失败的真实DELETE从200修复为500，原writer事务回滚记录删除；SQL DELETE拒绝控制保持文件及完整rows/audit，故障移除后均可200重试。九条原函数分类与reuse不变，当前1672/2142/637；新失败控制不用于升级exact。定向261、quick2676 Rust/98 Pine/desktop11+48、现场完整Rust4073 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。三份receipt、两份gate、1059指纹及九条原函数/anchor核验，红测试及脚本失败留证。文件系统部分删除、SQL commit后跨系统恢复仍有缺口；整体未完成，继续真实URL下载、重复安装与重定向owner。详见[积压清单](parity-evidence-backlog.md)。

真实POST/重连与文件Skill发现修复运行后外部文件无法列表/卸载，GET不写SQLite，原mutation owner删除目录。六条冻结原函数复核，routes_resource410完整闭合后恢复exact，当前1669/2145/637；两个builtin hash、八份资源、三项publish工具及刷新owner仍有缺口。定向253、quick全量4065 Rust/2435 Web/98 Pine/9结构语料/337 Python、现场完整Rust4065 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。七份receipt、两份gate、1054指纹及九处reuse/anchor核验，失败及核验脚本错误保留；整体未完成，继续source排序及未知工具WARNING。 详见[积压清单](parity-evidence-backlog.md)。

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

其中 `adk_workflow_routes_test.go:262` 的三条 malformed-body 文案现由
`adk_mutations_compatibility::adk_workflow_routes_reject_invalid_payloads_with_reference_messages`
在真实 mutation wire boundary 逐条断言：workflow create 为
`invalid workflow payload`、workflow run 为 `invalid workflow inputs`、trigger
create 为 `invalid workflow trigger payload`；与既有 read-resource miss、mutation
not-found 两条 owner 测试合并后，映射不再保留未覆盖断言。

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

## 第二十四批：ADK run 生命周期审计与终态投影（runner_chat_test.go/store_test.go/input_request_test.go 共 7 条 [x]，含 2 个真实功能缺口）

本批把 `internal/assistant/engine/runner_chat_test.go` 的终态/审计条目与两条被遗漏的审计契约一起结清，并修掉两处真实功能缺失。

### 缺口 1：Rust 从不写 ADK audit 行

Go 的 `Runtime.audit` 在每个生命周期转换写一行 `AuditEvent`（`run.completed`、`run.failed`、`run.timed_out`、`run.cancelled`、`run.denied`、`run.awaiting_approval`、`run.awaiting_input`），`GET /api/v1/adk/audit` 直接回读这些行。Rust 的 store 与读路由都已接好，但生产 runtime 一行都没写，控制台审计视图恒为空。

Rust 修复（`crates/jftrade-engine`）：

- 新增 `product_adk_model_runtime_audit.rs`：`RunAuditEvent`、`record_run_audit`（best-effort，唯一键冲突视为已被更早的 fenced 尝试写过而保留首行，其余错误只打日志）、`lifecycle_audit_kind`、`terminal_audit_message`、`terminal_audit_fields`。用 `include!` 挂在 `product_adk_model_runtime.rs` 模块作用域，保持生产片段在 800 行上限内。
- `product_adk_model_runtime_stream.rs`：`persist_success` 写 `run.completed`，`persist_failure` 按状态派生 `run.failed`/`run.timed_out` 并携带 `errorCode`/`failureReason`。
- `product_adk_model_runtime_lifecycle.rs`：`persist_cancelled` 写 `run.cancelled`（`RUN_CANCELLED`）。
- `product_adk_model_runtime_tool_persistence.rs`：审批停写 `run.awaiting_approval`（含 `pendingApprovals` 计数）。
- `product_adk_model_runtime_input_call.rs`：输入停写 `run.awaiting_input`（含 `requestId` 与 `decisionKind`，不泄漏 `blockingReason`）。

### 缺口 2：审批拒绝没有 `approval_denied` 与 `run.resumed`

Go 的 `markDeniedResumedRun` 把被拒的续跑投影成 `status=DENIED`、`resumeState="approval_denied"`、`message="approval denied"`（清空 `errorCode`/`failureReason`），`auditResumedRun` 再写 `run.resumed` + 终态 kind 两行，二者都带 `resumeState`。Rust 原先只写 `status=DENIED` 与一句自造消息，`resumeState` 与两条审计都缺失。

Rust 修复：新增 `record_resumed_run_audit`（`run.resumed` 后接 `lifecycle_audit_kind(status)`，共用幂等插入），`product_adk_model_runtime_events.rs` 的 `resume_approval` denied 分支改用 Go 的字段与消息。

### 本批结清条目

| Go 测试 | Rust 测试 | 结论 |
| --- | --- | --- |
| `runner_chat_test.go:181` `TestPersistRunTerminalStateWritesRunAndAudit` | `a_failed_run_persists_its_terminal_state_and_audit_row` | `[x]` 终态 run + `run.failed` 审计行 |
| `runner_chat_test.go:267` `TestFinishPendingApprovalRunPersistsPendingStateAndAssistantPrompt` | `a_gated_call_parks_the_run_and_audits_awaiting_approval` | `[x]` `PENDING`/`waiting_approval` + `run.awaiting_approval` |
| `runner_chat_test.go:325` `TestCompleteChatRunFailurePersistsUserFacingErrorReply` | `a_failed_run_persists_the_provider_error_and_audit_row` | `[x]` 终态落盘半；**wire 差异另列为 P0 未关闭（见下）** |
| `runner_chat_test.go:376` `TestCompleteChatRunSuccessPersistsCompletedRunAndAssistantReply` | `a_completed_run_persists_the_reply_and_audits_run_completed` | `[x]` `COMPLETED`/`completed` + `run.completed` |
| `runner_chat_test.go:1124` `TestCancelRunOnTerminalStateIsNoop` | `a_cancelled_run_audits_run_cancelled_and_terminates_once` | `[x]` 幂等取消 + 单条 `run.cancelled` |
| `store_test.go:720` `TestApprovalDenialRecordsResumedAndDeniedAuditEvents` | `a_denied_approval_audits_run_resumed_and_run_denied_with_the_denied_state` | `[x]` `approval_denied` + `run.resumed`/`run.denied` |
| `input_request_test.go:584` `TestRequestUserToolPausesAndResumesChatRun` | `a_pending_input_run_audits_awaiting_input_with_the_decision_kind` | `[x]` `PENDING_INPUT`/`waiting_input` + `run.awaiting_input` |

另有 `lifecycle_audit_helpers_match_the_reference_table` 冻结 helper 表（kind、detail、omitempty 字段）。

批准续跑同样补了审计：Go 的 `auditResumedRun` 在 `PersistRunTerminalState` 之前执行，因此 `persist_success` 的 `chat.resumed` 分支先写 `run.resumed`（`resumeState=adk_confirmation_resolved`）再写 `run.completed`；`store_ops_test.go:565` 的既有回归新增了这两行的断言。

### 未关闭的 P0：provider 失败在 chat 路由的 wire 行为

- Go 行为（冻结证据）：provider HTTP 500 对 `POST /api/v1/adk/chat` 与 `/stream` 都是 **HTTP 200 + failed run**，`reply = userFacingADKError(err)`、`run.finalMessageId` 非空、`degraded=true`，SSE 以 `final` 事件结束。见 `tests/fixtures/compatibility/api-transport/adk-chat-stream.json` 的 `chat-provider-failure`/`stream-provider-failure` 与 `docs/history/go-to-rust/route-ledgers/adk-chat-stream.md` 的 go-behavior quirk。
- Rust 现状：可重试 provider 故障走 `product_adk_model_runtime_retry.rs`，run 保持 `RUNNING` + `providerRetry`/`resumeState=provider_waiting` 以便恢复，路由回 502；`finalMessageId` 在 Rust 中全仓没有实现（仅前端类型与 openapi 声明存在）。
- 为什么本批不直接改：该行为是 Rust 产品刻意的 durable-retry 设计（提交 5853c5d4），与 provider 失败即终态的 Go 语义冲突，属于公开 API 契约变更（`ADR`/release gate 范围），且 `contracts/openapi/openapi.json` 的 chat 路由只声明 200/400。先登记复现条件、预期、修复位置与回归要求，交由下一批按“保 durable 还是按 Go 投影”决策后再改。
- 复现条件：active provider 返回 5xx（fixture 用 500），body 带合法 `clientRequestId`/`agentId`/`message`，请求 `POST /api/v1/adk/chat`（或 `/stream`）。
- 预期（Go）：`200 ok=true` + `data.run.status=FAILED`、`errorCode=MODEL_CALL_FAILED`、`reply` 为用户可见错误文本、`finalMessageId` 非空；SSE 以 `final` 事件收尾。
- 修复位置：`product_adk_model_runtime_retry.rs` 的 `is_provider_retryable_error`/`persist_provider_retry` 终态化策略，以及 `product_adk_model_runtime_stream.rs` 的 `finish_chat`/`persist_failure` 投影（含 `finalMessageId`）。
- 回归要求：先把 fixture 的两个 provider-failure 用例写成 Rust 失败回归（200 + FAILED + reply + finalMessageId + SSE `final`），再改生产实现；若最终决定保留 durable retry，必须在 `adk-chat-stream.md` 登记为有意的行为分歧并同步 openapi/前端契约。

### 文件规模

`product_adk_model_runtime_stream.rs` 增行后达到 811 行，超过 workspace 的 800 行生产上限（`pnpm run check:rust:architecture` 拦截）。按既有 `include!` 约定把 `persist_failure` 抽到新文件 `crates/jftrade-engine/src/product_adk_model_runtime_failure.rs`（181 行，文本包含回同一模块作用域），主片段回到 640 行。

### 验证

- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`：通过（0 警告）。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1479 passed / 0 failed**（上批基线 1470，本批 +9）。
- `pnpm run check:quick`（含 1499 条引擎测试 + 兼容 replay + 桌面 release 测试）、`pnpm run check:compatibility`、`pnpm run check:zero-go`、`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`、`git diff --check`：全部通过。
- 中途 `check:quick` 报 `.rcgu.o ≥ 50000`（本机累积 114197 个中间对象，无 cargo/rustc 进程在跑），按门禁提示清理 `target/debug/deps/*.rcgu.o` 后通过。
- 探针 A（`record_run_audit` 入口短路）：6 条审计回归全部转红，`cp` 备份恢复后通过。
- 探针 B（`record_resumed_run_audit` 短路）：`a_denied_approval_audits_run_resumed_and_run_denied_with_the_denied_state` 转红（`audit rows must contain run.resumed`），恢复后通过。
- 映射清单：`[x]` 805 → **812**，全部 `function_exact`，0 重复 rust_entry，key 集合仍为 4451，0 U+FFFD。

### 下批目标

`internal/assistant/engine/runner_chat_test.go` 余 17 条 `[~]`：`:56`（`ADK_INPUT_UNSUPPORTED`）、`:69`（`HydrateRunExecutionResult` 字段投影）、`:102`（顶层 followUp 不提升为 pending input）、`:127`（tool-only run 合成 final reply）、`:156`（`markFailedChatRun` context→终态映射）、`:225`（`AttachFinalAssistantMessage` 与 `finalMessageId`）、`:490`（`ProjectedChatResponse` 投影）、`:541`（非法 permissionMode override）、`:552`（run 冻结 resolved model 快照）、`:609`（provider override 不改 agent）、`:737`（projection 持久化边界）、`:846`（已 resolve 的 approval 不回流）、`:891`（stream 关闭后后台 resume）、`:1043`（`resolveSession` 复用/标题裁剪）、`:1079`（run handle 生命周期）。优先级：先把 P0 的 provider-failure wire 决策连同 `finalMessageId` 一起处理（`:225`/`:325`/`:490` 归属同一决策面），再按 `:156`/`:127`/`:69` 顺序补齐终态与投影语义。

## 第二十五批：runner_chat_test.go 终态映射与审计投影（runner_chat_test.go:156 结清、:56 保留边界）

批次范围：`internal/assistant/engine/runner_chat_test.go:156`（`TestMarkFailedChatRunMapsContextToTerminalState`，`[x]`）与 `:56`（`TestRequestedInputEventFailsWithUnsupportedInputCode`，`[~]` 保留边界 + 映射契约已补）。

Go 行为（go:452dea11）：
- `internal/assistant/model/runner_lifecycle.go:16` `RunStatusForContext` + `:33` `RunErrorCode`：deadline → `TIMED_OUT`/`RUN_TIMED_OUT`，canceled → `CANCELLED`/`RUN_CANCELLED`，其他 → `FAILED`/`MODEL_CALL_FAILED`；`ErrADKInputUnsupported` 在 status switch **之前**先判，因此 `FAILED` 也取 `ADK_INPUT_UNSUPPORTED`。
- `markFailedChatRun` 把 `err.Error()` 原样同时写入 `Message` 与 `FailureReason`；`CompletedAt` 必写，取消时另写 `CancelledAt`（同一时刻），且 `Degraded = true`。

Rust 修复前差异：
1. `persist_failure` 把 provider 的 `MODEL_CALL_TIMEOUT` 直接当 run 级 `errorCode`（Go 是 `RUN_TIMED_OUT`）；
2. `message` 写成 `"{code}: {message}"`（Go 是原始错误文本，fixture `chat-provider-failure`/`stream-provider-failure` 亦然）；
3. 不写 `failureReason` 与 `completedAt`；
4. `persist_cancelled` 缺 `failureReason`/`degraded`/`completedAt`/`cancelledAt`；
5. 没有 `ADK_INPUT_UNSUPPORTED` 分支。

修复位置：
- `crates/jftrade-engine/src/product_adk_model_runtime_failure.rs`：新增 `run_terminal_state(error) -> (status, error_code)` 冻结 Go 的判定优先级；`persist_failure` 改写 `message`/`failureReason` = 原始错误文本、`errorCode` = 分类码、`completedAt`、`degraded=true`，并清 `providerRetry`。
- `crates/jftrade-engine/src/product_adk_model_runtime_lifecycle.rs`：`persist_cancelled` 按 Go 补 `failureReason`/`degraded`/`completedAt`/`cancelledAt`。
- 新回归（`product_adk_model_runtime_terminal_audit_tests.rs`）：`run_terminal_state_classifies_unsupported_input_before_the_status_switch`（三条优先级 + `Unavailable` 默认分支）、`terminal_failure_mapping_matches_the_reference_table`（取消/超时/普通失败走真实 store + runtime 断言全部投影字段）。

探针：把 `run_terminal_state` 的 `RUN_TIMED_OUT` 改回 `MODEL_CALL_TIMEOUT` → `terminal_failure_mapping_matches_the_reference_table` 转红（`left: MODEL_CALL_TIMEOUT` / `right: RUN_TIMED_OUT`），用 `cp` 备份恢复后通过。

`:56` 为什么只能保留 partial：该测试的输入是 vendored Google ADK 的 `adksession.Event.RequestedInput` 事件（`consumeEvent` → `errADKInputUnsupported`），Rust 不走 ADK 事件循环、直接调 Responses API，因此没有 `RequestedInput`/`InterruptID` 事件面。可移植的只有终态映射契约（先判 `ADK_INPUT_UNSUPPORTED` 再进 status switch），已由 `run_terminal_state_classifies_unsupported_input_before_the_status_switch` 断言；不伪造事件源。若未来 Rust 接入 ADK 事件循环，需补事件级回归。

验证：
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1481 passed / 0 failed**（上批 1479）。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`（修掉一处 `needless_borrow`）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`、`git diff --check`：全部通过。
- `python3 scripts/compatibility/audit_test_parity.py`：`[x]` 812 → **813**、`partial` 123 → 124、`missing` 3364 → 3362；Rust 测试总数 2638 → **2640**；0 重复 `[x]`、0 U+FFFD、4451 key 不变。
- `pnpm run check:quick`：首次因 target-health（`target/debug/deps` ≥ 50000 个 `.rcgu.o`）失败，确认 `pgrep -fl "cargo|rustc"` 为空后 `find target/debug/deps -maxdepth 1 -name '*.rcgu.o' -delete`，重跑通过。
- 未关闭 P0 原样保留：provider 5xx 的 chat wire 行为（HTTP 200 + failed run + `reply` + `finalMessageId` vs Rust 的 502 + durable retry），本批未触碰该决策。

### 下批目标

`internal/assistant/engine/runner_chat_test.go` 余 14 条 `[~]`：`:56`（保留边界）、`:69`（`HydrateRunExecutionResult` 字段投影）、`:102`（顶层 followUp 不提升为 pending input）、`:127`（tool-only run 合成 final reply）、`:225`（`AttachFinalAssistantMessage` 消息与 run link）、`:490`（`ProjectedChatResponse` 投影字段）、`:541`（非法 permissionMode override）、`:552`（run 冻结 resolved model 快照）、`:609`（provider override 不改 agent）、`:737`（projection 持久化边界）、`:846`（已 resolve 的 approval 不回流）、`:891`（stream 关闭后后台 resume）、`:1043`（`resolveSession` 复用/标题裁剪）、`:1079`（run handle 生命周期）。
优先级：先落 P0 的 provider-failure wire 决策（连同 `finalMessageId`；`:225`/`:490` 与其同一决策面）：先把冻结 fixture 用例写成 Rust 失败回归，再决定「按 Go 收敛为 200 + FAILED + reply + finalMessageId」或「保留 durable retry 并在 route-ledger 登记为有意分歧 + 同步 OpenAPI/前端」。随后按 `:127` → `:69` 的顺序补齐投影语义，再接 `store_test.go`（20）→ `session_context_test.go`（19）。

另有 `runner_continuation_boundaries_test.go:87` 未结清分支（本批如实降回 `[~]` partial）：missing-run continuation 在 Go 返回 nil 而 Rust 回 `Unavailable`；foreign unexpired lease 不被 steal（Rust 走 takeover 等待）；`stageResolvedApproval(missing)`/`markApprovalContinuationFailed(missing)`/`attachParentWorkflowResolution(empty)`/nil-runtime reconcile 的无副作用分支。这三项按 P1 与 `store_test.go` 同批消化。

## 第二十五批补充：审批 continuation 竞态修复（`runner_continuation_boundaries_test.go:136` 结清）

`check:quick` 在批次收尾时稳定复现了一条此前被 nextest 顺序掩盖的 P0 竞态：`adk_approval_deny_returns_the_resolution_envelope_without_a_sync_message` 间歇性失败，报 `503 ADK_CONTINUATION_UNAVAILABLE: assistant chat run is already DENIED`。

根因：`resume_approval` 对任何「非 RUNNING 且非 `PENDING_INPUT`+`input_resume_pending`」的 run 都伪造 `Unavailable`。审批/输入路由与 durable recovery scanner 会为同一 continuation 竞争，输的一方把赢家已经写好的终态当成自己的失败，并据此回滚已 staged 的审批 → 路由回 503。Go 相反：`continueResolvedApprovalRun`/`continueResolvedInput` 对缺失或不可续跑的 run 返回 nil，`runCanContinueResolvedApproval` 只放行 PENDING 或处于 `approval_resuming` 的 RUNNING leaf。

修复（`crates/jftrade-engine/src/product_adk_model_runtime_events.rs`）：该分支改为 `return Ok(())` 静默 no-op，与并发赢家保持一致，不再回滚别人的 durable 写入。

新增/加强回归：
- `a_denied_approval_audits_run_resumed_and_run_denied_with_the_denied_state`：连续两次 resume 同一已 DENIED run 都必须 Ok，且 `run.resumed`/`run.denied` 审计行保持唯一。
- `test_adk_resume_approval_cas_rejection`：原断言「非可续跑状态必须报错且含 `already PENDING_INPUT`」是 Rust 自造契约，改为断言返回 Ok 且 durable 状态逐字段不被改写。
- `adk_denied_approval_closes_siblings_and_unrelated_resolution_keeps_the_projection`（新）：锚定 `runner_continuation_boundaries_test.go:136` 的 denial 关闭 sibling、unrelated approval 不替换 embedded projection 两条子用例。

探针：
- 探针 A（`resolve_and_stage_approval` 的 denial 分支改为 `if false && denied`）：新 sibling 回归转红（`call 0 was closed by the denial: left PENDING_APPROVAL / right DENIED`），`cp` 备份恢复后通过。
- 探针 B（把 `resume_approval` 的 no-op 改回伪造 `Unavailable`）：terminal-audit 回归与 `test_adk_resume_approval_cas_rejection` 同时转红，恢复后通过。

验证：
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1482 passed / 0 failed**。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`python3 scripts/compatibility/audit_test_parity.py`：通过（814 function_exact，0 重复 `rust_entry`，0 U+FFFD，4451 key 不变）。
- `:87` 如实降回 `[~]` partial：missing-run/foreign-lease/empty-projection 等分支仍未覆盖。

## 第二十六批：runner_continuation_boundaries_test.go 结清（12 条，含 1 个真实缺口）

批次范围：`internal/assistant/engine/runner_continuation_boundaries_test.go` 全部 12 条测试。本批结清 `[x]` 7 条、保留边界 1 条、如实登记功能缺失 4 条。

真实缺口（本批修复）：`continueResolvedApprovalRun`/`continueResolvedInput` 在 run 行已被删除时必须返回 store 的 `nil`，让 `ResolveApprovalAsync` 照常回 resolution envelope。Rust `resume_approval` 之前对缺失 run 返回 `Unavailable("persisted ADK run disappeared")`，于是审批/输入路由会回滚已 staged 的审批并回 `503 ADK_CONTINUATION_UNAVAILABLE`——与上一批那条竞态同源的第二半。修复位置：`crates/jftrade-engine/src/product_adk_model_runtime_events.rs`，改为 `let Some(run) = ... else { return Ok(()) }`。回归：`a_continuation_for_a_missing_run_is_a_silent_no_op`（断言返回 Ok、不物化 run、不写审计行）。探针：恢复旧的 `ok_or_else(...)` 后该回归转红（`Unavailable("persisted ADK run disappeared")`），`cp` 备份恢复后通过。

新增回归（`product_adk_model_runtime_fencing_tests.rs`，全部断言 continuation claim 与关闭语义）：
- `challenge_continuation_supervisor_claims_are_exclusive_and_released` —— 同一 run 的第二次 spawn 必须 `Conflict("assistant continuation is already running")`，无关 run 不受影响，任务退出后 claim 释放且可重新 spawn。
- `challenge_continuation_supervisor_releases_claims_without_extra_initialization` —— 默认 supervisor 无需惰性初始化即可用，任务与 no-op continuation 结束后 `tasks` 表都不再持有该 run id。
- `challenge_continuation_supervisor_rejects_new_work_once_stopping` —— shutdown 前可 spawn，之后必须 `Unavailable(... is stopping)`，再次尝试仍被拒。
- `challenge_continuation_supervisor_shutdown_cancels_in_flight_and_rejects_new_work` —— Close 语义：shutdown 返回时在途任务已观察到取消，且此后拒绝新工作。

映射结论：
- `[x]` 7 条：`:11`（claim 互斥 + closing 不留残余 claim）、`:63`（零值可用 + 完成即释放）、`:87`（missing-run / 不可续跑 / foreign lease / empty state 四点全结清）、`:136`（上一批已结清，本批复核）、`:207`（不偷 foreign lease）、`:314`（closing 拒绝新工作）、`:347`（Close 等待在途 + 拒绝新工作）。
- `boundary` 1 条：`:406`（Rust 无 runtime 级 backgroundCtx，nil 兜底分支不适用）。
- `missing` 4 条（如实登记，不伪造测试）：
  - `:188` `RUN_LEASE_CLAIM_FAILED` —— Rust 把 run 行、初始事件与租约插入放在同一个 SQLite 事务（`create_run_with_event_idempotent`），租约失败即回滚，不留 RUNNING run，比 Go 更强，因此没有该错误码与回写路径。需决策保留还是补等价可观测性。
  - `:242` 租约存储故障传播 —— Rust 的 `reconcile_orphaned_pending_runs` 与 recovery scanner 用 `let Ok(...) else { return }` 吞掉存储错误只打印日志；store 关闭后的 Pause/Resume/UpdateObjective 有 `storage_mutation_failed` 映射但缺确定性回归。
  - `:275` / `:427` goal-run 后台续跑 —— Rust 没有 goal workflow 执行引擎：`ResumeRun` 路由只把 run 投影成 `RUNNING`/`user_resuming`/`workflowStatus=RUNNING` 并清暂停字段，不申请运行租约、不启动后台 worker，因此 fresh-foreign-lease 让位、租约存储错误落 FAILED、resume 失败落 FAILED 等分支在 Rust 都不存在。这是本领域最大的待设计缺口。

验证：
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1487 passed / 0 failed**。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`python3 scripts/compatibility/audit_test_parity.py`：通过（820 function_exact、0 重复 `rust_entry`、4451 key 不变、1 条 boundary 新增）。

### 下批目标

`runner_chat_test.go` 余 14 条 `[~]`（`:56`、`:69`、`:102`、`:127`、`:225`、`:490`、`:541`、`:552`、`:609`、`:737`、`:846`、`:891`、`:1043`、`:1079`）。先做 P0：provider-failure wire 决策（HTTP 200 + FAILED + reply + finalMessageId vs Rust 的 502 + durable retry），它与 `:225`/`:490` 同一决策面，必须先写失败回归再落实现或正式登记分歧并同步 OpenAPI/前端。随后按 `:127` tool-only run 合成 final reply → `:69` 的 `HydrateRunExecutionResult` 字段投影 → 其余投影/边界项推进。另需把 `:188`/`:242` 两条决策项与本批新增的 goal-run 执行引擎缺口纳入 store_test.go 同批评估。

## 第二十七批：provider 失败终态收敛（关闭上一批登记的 P0 wire 差异）

范围：`internal/assistant/engine/runner_chat_test.go:325 TestCompleteChatRunFailurePersistsUserFacingErrorReply`。

该行在第二十四批就已标 `[x]`（function_exact 终态落盘半），但 conclusion 里如实登记了一个**未关闭的 P0 wire 差异**。本批把这个已声明的缺口真正修掉并把 conclusion 改写为完整结论，因此 `[x]` 总数不变（820 → 820），变化在证据质量而非计数。

### 冻结证据（先取证，再改实现）

- Go `CompleteChatRun`（`git show go:internal/assistant/engine/runner_chat.go:174`）：`adkErr != nil` 时 `markFailedChatRun` → `PersistRunTerminalState` → `replyResult = assistantExecutionResult{Reply: userFacingADKError(adkErr), SyntheticKind: "provider_error"}` → `AttachFinalAssistantMessage`（写 transcript 条目并令 `run.FinalMessageID = message.ID`）→ 返回 `ProjectedChatResponse`。因此 `/api/v1/adk/chat` 对外是 **HTTP 200 + `ok=true` + `data.run.status=FAILED` + `data.reply` 非空 + `data.run.finalMessageId` 非空**。
- `git grep providerRetry` / `provider_waiting` 在 `go` 分支全仓 **0 命中**：Rust 之前的 durable retry（`RUNNING` + `providerRetry` + `resumeState=provider_waiting` + 502）是 Rust 发明物，不是基线行为。
- 冻结 fixture `tests/fixtures/compatibility/api-transport/adk-chat-stream.json` 的 `chat-provider-failure`（`status=200`、`run.status=FAILED`、`errorCode=MODEL_CALL_FAILED`、`failureReason`=原始 provider 文本、`finalMessageId=message-fixture`、`reply` 与 `failureReason` 同文本）与 `stream-provider-failure`（`type=final` 帧携带同一投影）与上述实现一致。
- 路线台账 `docs/history/go-to-rust/route-ledgers/adk-chat-stream.md` 早已登记该 quirk 并给出处置：**Reproduce the 200 projection; do not "fix" the Go error precedence**。
- 运行真实 Go 参考（`/opt/homebrew/bin/go test ./internal/assistant/engine/ -run TestCompleteChatRunFailure...`）并加临时探针确认：`run.FinalMessageID` 与 timeline 里 `kind=assistant_message/status=final` 条目的 `id` **相等**（`jftrade-run-probe-provider_error-0c118294ccd0a38f`），且该 id 由 `sha256("<kind>\0<reasoning>\0<reply>")` 前 8 字节决定。探针已删除，scratch 校验目录未纳入仓库。

### Rust 修复

- `crates/jftrade-engine/src/product_adk_model_runtime_failure.rs`：
  - 新增 `synthetic_assistant_message_id(run_id, kind, reasoning, reply)`，精确复刻 Go `syntheticAssistantMessageID`（SHA-256 前 8 字节 hex，`kind` 缺省 `local`，provider 失败传 `provider_error`）；用 Go 探针的期望值 `jftrade-run-probe-provider_error-0c118294ccd0a38f` 逐字节核对。
  - 新增 `user_facing_adk_error(error)`，复刻 Go `UserFacingADKError` 的两条中文映射（`wrote more than the declared content-length`、`database is locked`/`sqlite_busy`）与原文兜底。
  - 新增 `attach_terminal_failure_projection`：写 `finalMessageId`、按 `GO_RUN_PROJECTION_FIELDS` 收敛 wire 字段（避免把 `streamEvents`/`providerEvents`/`route`/`toolResults` 等 durable 内部键泄漏到 `run`）、拼装 `{reply, session, run, pendingApprovals, timeline}` 投影并落 `response`，同时把 assistant 消息作为 `AdkRunEvent` 写进 session transcript；stream 路由补 `final` 帧（Go `publishTerminalError` → `RecoverTerminalChatResponse` 行为）。`persist_failure` 结尾调用它。
- `crates/jftrade-engine/src/product_adk_model_runtime_stream.rs`：`finish_chat` 的 `Err` 分支不再分流 `is_provider_retryable_error`，改为 `persist_failure` 后回读持久投影并返回（chat → `Json`，stream → `stream_from_payload`）；`run_live_stream` 的 `Err` 分支同样只走 `persist_failure`，保留取消/断线优先。
- `crates/jftrade-engine/src/product_adk_model_runtime_tool_loop.rs`：工具循环内的 provider 失败同步改为终态。
- `crates/jftrade-engine/src/product_adk_model_runtime_retry.rs`：删除已无调用方的 `persist_provider_retry`；保留 `persist_provider_retry_with_lease` 并加说明——它只服务**历史** `provider_waiting` 行（恢复扫描需要识别、重新申请 fenced lease 探活、按退避避免热循环），新写入路径不可能再进入该状态。

### 回归与断言更新

- 新增 `crates/jftrade-engine/src/product_adk_chat_stream_product_tests.rs::production_chat_provider_failure_projects_go_failed_run_with_reply`：真实 `ProductionAdkChatRuntime` + 关闭的 loopback provider，断言 `200`、`ok=true`、`run.status=FAILED`、`errorCode=MODEL_CALL_FAILED`、`degraded=true`、`completedAt` 落戳、`reply` 与 `finalMessageId` 非空、**无 `providerRetry`**、timeline 含 `id == finalMessageId` 的 `assistant_message/final` 且文本等于 `reply`。
- 同步更新 3 个此前钉住旧 Rust 行为、且与 Go 基线冲突的断言：
  - `product_adk_chat_stream_product_tests.rs::adk_chat_idempotency_contract_matches_the_go_routes`：首次请求由 `502` 改为 `200 + FAILED`，并新增「重放复用同一 run（`finalMessageId` 相同）」断言。
  - `product_production_ports_adk_tests.rs::adk_chat_route_reports_the_go_error_classification`：fallback provider 调用失败改为断言返回投影（`FAILED`/`MODEL_CALL_FAILED`/失败原因含 fixture endpoint/`reply == failureReason`）。
  - `product_adk_model_runtime_tool_deadline_tests.rs::an_expired_tool_deadline_is_projected_onto_the_tool_call`：工具级 `TIMED_OUT`/`TIMEOUT` 契约不变，但删掉「run 级 `failureReason` 必空」这一 Rust 发明断言，改为要求 run 级失败**不得**归因到工具超时（用 Go 探针确认：工作 provider 下 Go 为 `COMPLETED` + `degraded=true`，本 fixture 的 provider 为关闭端口时经 provider 边界终态）。
- `product_adk_model_runtime_terminal_audit_tests.rs` 的过时注释改写为指向新回归；`docs/history/go-to-rust/manual-test-mappings.json` 的 `runner_chat_test.go:325` 行 conclusion 重写为完整结论（冻结证据、`providerRetry` 0 命中、修复位置、回归名、验证结果），`status` 保持 `[x]`、`rust_entry` 保持唯一。

### 探针（失败 → 恢复）

在 `persist_failure` 末尾短路 `attach_terminal_failure_projection`（并让 helper 提前 `Ok(())`）：
`production_chat_provider_failure_projects_go_failed_run_with_reply` 立即转红——`left: 502 / right: 200`，与收敛前行为一致；用 `cp` 备份恢复后重新通过。全过程未使用 `git checkout --`。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1488 passed / 0 failed**。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`：通过。
- `pnpm run check:zero-go`（2845 文件 / 0 产物）、`pnpm run check:compatibility`（含 api-transport 278 operations、18 route groups、19 probes）、`pnpm run check:rust:architecture`、`pnpm run check:quick`（EXIT=0）：全部通过。
- `python3 scripts/compatibility/audit_test_parity.py`：820 `function_exact`、0 重复 `rust_entry`、4451 key 不变、1 条 helper-based 提示复核。

### 下批目标

`runner_chat_test.go` 当前余 12 条 `[~]`：`:69`、`:102`、`:127`、`:225`、`:490`、`:541`、`:552`、`:609`、`:737`、`:846`、`:891`、`:1043`、`:1079` 中除去本批已复核项后仍待推进的投影/边界项（`:56` 维持边界保留，`:156`/`:325` 已结清）。建议顺序：

1. `:127` tool-only run 合成 final reply（Rust `persist_success` 目前没有空文本合成路径）。
2. `:69` `HydrateRunExecutionResult` 字段投影（`toolCalls`/`toolSummaries`/`preToolContent`/`preToolReasoning`/`optimizationTaskId`/`pendingApprovals`/`usage.toolCallsTotal`）。
3. `:102` 顶层 follow-up 不升级为 pending input。
4. `:225` + `:490`：`AttachFinalAssistantMessage` 的 `finalMessageId` 链接与 `ProjectedChatResponse` 投影字段。本批已把 `finalMessageId`、timeline 与合成消息 id 的实现落地，这两条不再被 wire 决策阻塞，可直接按 Go 逐字段核对。
5. 其余 `:541`/`:552`/`:609`/`:737`/`:846`/`:891`/`:1043`/`:1079`。

决策项仍需与本领域同批评估：`runner_continuation_boundaries_test.go:188`（`RUN_LEASE_CLAIM_FAILED` 语义）、`:242`（租约存储错误传播与关闭 store 回归）、`:275`/`:427`（goal-run 后台续跑引擎缺口）。

随后按 `store_test.go`（20）→ `session_context_test.go`（19）推进；`internal/app/apiserver`（349）与 `pkg/backtest`（174）保持为后续大领域批次。

## 第二十八批：chat 覆盖校验、run 快照字段与成功路径 finalMessageId 链接（runner_chat_test.go 结清 3 条）

范围：`internal/assistant/engine/runner_chat_test.go` 的 `:225`、`:541`、`:552`。三行均为 Rust 真实功能缺口，不是计数对齐。`[x]` 由 820 → **823**。

### :541 `TestRunChatRejectsInvalidPermissionModeOverride`（fail-open 修复）

冻结证据：Go `ValidateChatOverrides`（`internal/assistant/model/runner_state.go:78`）在 `runChat` 中紧跟 `prepareChatRequest`、且**在 `resolveAgentDefinition` 之前**执行：
`workModeOverride` 非空且不属于 `{chat,loop}` → `invalid work mode %q`；`permissionModeOverride` 去空格后非空且不属于 `{approval,less_approval,all}` → `invalid permission mode %q`；`reasoningEffortOverride` 不属于 `{low,medium,high,xhigh,max}` → `invalid reasoning effort %q`；空白视为缺省。

Rust 修复前的问题：完全没有 override 校验。请求里写 `permissionModeOverride: "root"` 会静默沿用 agent 自身模式——即 fail-open，比 Go 更宽松。

修复：`product_adk_model_runtime_lifecycle.rs` 新增 `validate_permission_mode_override` / `validate_work_mode_override` / `validate_reasoning_effort_override` 与 `PERMISSION_MODE_OVERRIDE_FIELD`；`product_adk_model_runtime_events.rs` 的 `prepare_chat` 在指纹与 agent 解析之前调用（复刻 Go 顺序），校验后的规范化权限模式经 `request_object` 传给 `resolve_provider` 并冻结进 run 快照。

回归 `chat_rejects_invalid_permission_work_mode_and_reasoning_overrides`：三类非法值对真实 agentId 与**不存在的** agentId 都返回 `(400, ADK_CHAT_FAILED, 参考原文)`（同时锁住"校验先于 agent 解析"的顺序）；合法值 `all`/`"  approval  "`/`loop`/空串/`xhigh`/`MAX` 全部接受；带 override 的 run 快照 `permissionMode=less_approval`。探针：把 `validate_permission_mode_override` 短路成 `Ok(None)` 后回归转红（回 200 投影而非 400），`cp` 恢复后通过。

### :552 `TestRunStoresResolvedModelSnapshot`（快照字段缺失）

冻结证据：Go `startRun` 把解析后的 provider display name、model 与有效 permissionMode 冻结到 run 行；之后重命名 provider 或改其 model **不得**改写已存在 run 的历史。

Rust 修复前的问题：run payload 只写 `providerId`/`model`，缺 `providerName` 与 `permissionMode`，控制台拿不到与 Go 相同的快照字段。

修复：`ResolvedProvider` 增加 `name`/`permission_mode`，由 `resolve_provider` 从 provider payload 的 `displayName` 与（override 优先 / 否则 agent 的 `normalize_permission_mode`）填充；初始 payload 写入 `providerName` 与 `permissionMode`。

回归 `run_snapshot_freezes_provider_name_model_and_permission_mode`：快照 `model=snapshot-model-v1`、`providerName=Snapshot Provider`、`permissionMode=approval`；改名为 `Snapshot Provider Renamed` 且 model 改 v2 后，落盘 run 仍保持 v1 与原名。

### :225 `TestAttachFinalAssistantMessagePersistsMessageAndRunLink`（成功路径从不写 finalMessageId）

冻结证据：Go `AttachFinalAssistantMessage` 追加 assistant transcript 条目、写 `run.FinalMessageID = message.ID`、`SaveRun` 后返回更新过的 run；消息 id 由 `syntheticAssistantMessageID(runID, replyResult)` 决定（kind 缺省 `local`，摘要 = `sha256(kind\0reasoning\0reply)` 前 8 字节 hex）。上一批只修了**失败**路径的 finalMessageId，成功路径仍是自造的 `"{run}:{agent}"` session event id，且 `finalMessageId` 完全缺失，控制台无法把 reply 反查回 transcript 行。

修复：`persist_success` 改用 `synthetic_assistant_message_id(run_id, "local", "", text)`，把同一个 id 同时写到 `run_value.finalMessageId`、`payload.finalMessageId`、timeline 条目 id 与 `AdkRunEvent.id`，四者共享一个 id。摘要里 reasoning 槽留空并写明原因：Rust 的 `ModelResponse` 只带可见文本，没有 Go `googleADKExecution.result()` 那种 reasoning buffer 可参与摘要；这是如实记录的能力差异，不是遗漏。

回归 `a_completed_run_links_its_final_assistant_message_across_the_transcript`：断言 `run.finalMessageId` 非空且等于 `timeline[0].id`、timeline 为 `assistant_message`/`final` 且文本等于回复；落盘 payload 同 id；用 `session_store.list_events` 找到该 id 并断言 `content`/`invocationId`/`author` 与 run 对应。探针：短路 `payload["finalMessageId"]` 写入后回归转红，`cp` 恢复后通过。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1490 passed / 0 failed**（上批 1488）。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`pnpm run check:zero-go`、`pnpm run check:compatibility`（EXIT=0）、`pnpm run check:rust:architecture`：通过。
- `python3 scripts/compatibility/audit_test_parity.py`：820 → **823 function_exact**、0 重复 `rust_entry`、4451 key 不变、0 U+FFFD、Rust 测试总数 2647 → 2650。

### 下批目标

`runner_chat_test.go` 余 9 条 `[~]`：`:56`（边界保留）、`:69`、`:102`、`:127`、`:490`、`:609`、`:737`、`:846`、`:891`、`:1043`、`:1079`。建议顺序：

1. `:69` `HydrateRunExecutionResult` 字段投影（`toolCalls`/`toolSummaries`/`preToolContent`/`preToolReasoning`/`optimizationTaskId`/`pendingApprovals`/`usage.toolCallsTotal`）——与 `:490` 同一投影面，可同批。
2. `:490` `ProjectedChatResponse` 需要从 session transcript 反推 `preToolContent` 与 `reply`（Go 用 `ProjectedAssistantMessageForRun` 读投影），Rust 目前只有 run payload，缺口可能比 `:69` 大，先取证再决定实现或如实登记。
3. `:127` tool-only run 合成 final reply——注意 Go 的 `testProviderFinalReply` 是**测试替身**（假 provider 的第二轮回复），不是生产合成路径；真正的生产合成只有 `tool_failure` 分支。需先确认 Rust 的工具循环是否已复刻该轮次，再判定 `[x]` 还是 `boundary`。
4. `:102` 顶层 follow-up 不升级为 pending input、`:609` provider override 不改 agent、`:737` projection 持久化边界、`:846` 已 resolve 的 approval 不回流、`:891` stream 关闭后后台 resume、`:1043` resolveSession 复用/标题裁剪 28 字符、`:1079` run handle 生命周期。

决策项仍需与本领域同批评估：`runner_continuation_boundaries_test.go:188`（`RUN_LEASE_CLAIM_FAILED`）、`:242`（租约存储错误传播）、`:275`/`:427`（goal-run 后台续跑引擎缺口）。

随后按 `store_test.go`（20）→ `session_context_test.go`（19）推进；`internal/app/apiserver`（349）与 `pkg/backtest`（174）保持为后续大领域批次。

## 第二十九批：ProjectedChatResponse 运行投影、会话复用与 run 快照（runner_chat_test.go 结清 6 条）

范围：`internal/assistant/engine/runner_chat_test.go` 的 `:69`、`:102`、`:490`、`:846`、`:1043`、`:1079`。六行都是真实功能缺口（投影字段丢失、会话被改名、快照字段缺失），不是计数对齐。`[x]` 由 823 → **829**，Rust 测试总数 2650 → **2657**。

### 共同根因：完成的 chat envelope 是临时 JSON，而不是 Go 的 `ProjectedChatResponse`

冻结证据：`tests/fixtures/compatibility/api-transport/adk-chat-stream.json` 的 `chat-success` 与 `chat-provider-failure` 两个用例（Go 录制）给出完成的 `run` 字段集：`agentId/createdAt/finalMessageId/id/maxDurationMs/message/model/pendingApprovals/permissionMode/providerId/providerName/sessionId/startedAt/status/toolCalls/updatedAt/usage/userMessage/workMode`，且 `degraded=false`、空的 `errorCode`/`failureReason`、`reply` 都**不在** run 上（`encoding/json` 的 `omitempty` 与 `Run` 本身没有 `reply` 字段）；`timeline` 是**会话投影**（`user_message` + `assistant_message`），不是单条合成条目。

Rust 修复前的实现恰好相反：`persist_success` 手写一个 JSON 字面量，缺 `userMessage`/`usage`/`workMode`/`maxDurationMs`/`startedAt`，多出 `reply`、`degraded:false`、`errorCode:""`、`failureReason:""`，`timeline` 只有一条 assistant 条目；失败路径虽然按 `GO_RUN_PROJECTION_FIELDS` 过滤，但同样缺这些字段与用户 timeline 条目。

修复：新增 `crates/jftrade-engine/src/product_adk_model_runtime_projection.rs`（生产模块，≤800 行）承载 Go 的投影语义：

- `summarize_tool_output` / `tool_summaries_for_run`（`SummarizeToolOutput`+`ToolSummariesForRun`，1800 字节按字符边界截断 + `...(truncated)`；SUCCEEDED/FAILED/DENIED 三种终态）。
- `optimization_task_id`（首个 SUCCEEDED 的 `strategy.optimize` 的 `output.taskId`）。
- `merge_projected_text`（`mergeProjectedText`：partial 追加、前缀替换、重复后缀去重）。
- `usage_wire_value`（`RunUsage`：`modelCalls`/`toolCallsTotal` 恒在，`durationMs` 由 `startedAt`→`completedAt` 计算，>0 才写）。
- `run_wire_fields` / `go_run_wire` / `drop_go_zero_fields`（`GO_RUN_PROJECTION_FIELDS` + Go `omitempty` 语义）。
- `session_timeline`（`SessionTimeline` 投影：过滤 `assistant.stream`/`assistant.tool` 内部事件，并把调用方**即将写入**的 assistant 条目按 id 幂等补入）。

`persist_success` 与 `attach_terminal_failure_projection` 都改为走同一投影；成功路径先把 `preToolContent` 与最终文本合并成 `reply`（并据此计算 `finalMessageId`），再一次性把 `toolCalls`/`toolSummaries`/`optimizationTaskId`/`usage` 写回 durable payload，保证重连与 `clientRequestId` 重放能重建同一 envelope。

### :69 `TestHydrateRunExecutionResultPopulatesRunFields`

Go 的 `HydrateRunExecutionResult` 把 tool context 投影到 run：calls、summaries、`preToolContent`/`preToolReasoning`、`optimizationTaskId`、`pendingApprovals`、`usage.toolCallsTotal`。Rust 之前只回传最后一次模型文本。

回归 `the_run_projection_derives_tool_summaries_optimization_task_and_usage_totals`：预置三条终态 call（SUCCEEDED 带 `{taskId: opt-123}`、FAILED `disk full`、DENIED）后断言 `optimizationTaskId=opt-123`、`usage.toolCallsTotal=3`、`modelCalls=3`（两个 tool round + 开场调用）、summary 三条分别为 `strategy.optimize => {...}` / `strategy.save_draft failed: disk full` / `trade denied by user`。

### :490 `TestProjectedChatResponseAppliesProjectionToRunFields`

Go 的期望：`reply` = 整轮 assistant 文本合并（`先说明一下。优化已启动。`）、`run.preToolContent=先说明一下。`、`toolCalls[0].toolName=strategy.optimize`、`toolSummaries` 含该工具、`optimizationTaskId=opt-999`、`usage.toolCallsTotal=1`、`finalMessageId` 非空、`timeline` 非空。

回归 `a_tool_round_projects_the_pre_tool_reply_and_session_timeline`：按 Go 的注入方式准备持久状态（`preToolContent` + 一条 SUCCEEDED 的 optimize call + 会话里的 user 事件），走 `persist_success` 后逐项断言，并额外断言 timeline 为 `[user_message, assistant_message]` 且 assistant 条目 id == `run.finalMessageId`。

配套回归 `staging_a_tool_round_freezes_the_pre_tool_assistant_text`：`persist_tool_calls` 首轮把文本去空格后冻结到 `preToolContent`（`state.preToolCaptured` 语义），第二轮不再覆盖；同时锁定低风险读（`system.status` 在 `less_approval`）保持 `RUNNING` 释放、不进入审批。

**已登记的差异（不是本行遗漏）**：Rust 生产 tool catalog 未注册 `strategy.optimize`（Go 见 `internal/assistant/assembly/tool_catalog.go:574`：`optimize_strategy` + `RequiresApprovalIn=[approval]`），因此该 id 在 Rust 侧因 `requires_approval` 查不到而 fail-closed 为审批，且 `ProductionAdkToolExecutor::supports` 也不含它。本行验证的是投影契约；catalog/executor 缺口作为 follow-up 记在下方“下批目标”。

### :846 `TestProjectedChatResponseDoesNotExposeResolvedApprovals`

回归 `a_completed_projection_hides_resolved_approvals`：run payload 预置一条 `APPROVED` 审批后走完成投影，断言 `response.pendingApprovals` 与 `response.run.pendingApprovals` 均为空数组，且 timeline 内没有 `approval_group`（Rust 的 timeline 来自会话事件投影，不合成审批组）。

### :102 `TestCompleteChatRunDoesNotPromoteTopLevelFollowUpToPendingInput`

回归 `a_top_level_follow_up_reply_keeps_its_run_completed`：提问型顶层回复仍是 `COMPLETED`、`reply` 原样保留、无 `inputRequest`、`pendingApprovals` 空。Rust 从未实现该启发式，回归把它锁定为契约，防止后续误加“提问即等待输入”的推断。

### :1043 `TestResolveSessionReusesExistingRejectsMismatchAndCreatesTrimmedSession`

冻结证据：Go `resolveSession` 只在**创建**时写 title（前 28 个 rune），复用显式 `sessionId` 时原样返回既有行。Rust 之前每次 chat 都 `upsert_session` 当次消息生成的 title，会话会被自己的第二条消息改名；ADK session 行也被同步重写 state。

修复：`prepare_chat` 先读 `get_session_agent_id`，仅在该行为空时创建 app 层 session；Google-ADK session 行改为 `get_session_by_id` 缺失时才创建（按需创建、不重写）。

回归 `chat_creates_the_session_once_and_reuses_its_stored_title`：40 rune 中文消息创建出 28 rune title；第二条消息复用显式 sessionId 后 title 不变、`list_sessions()` 仍只有一行。mismatch/not-found 由 `gate_tests::chat_rejects_a_session_owned_by_a_different_agent` 覆盖。

### :1079 `TestStartRunPersistsRunAndFinishRemovesActiveHandle`

冻结证据：Go `startRun` 落盘 `status=RUNNING`、`message="running"`、`userMessage`、`startedAt`、`MaxDurationMs=DefaultRunTimeout(30min=1800000)`、`workMode`、`usage={modelCalls:0,toolCallsTotal:0}`，并注册 active handle；`finish()` 释放。Rust 之前只有内部 `requestMessage`，没有 Go 的 Run 快照字段。

修复：`prepare_chat` 写入上述快照字段（`RUN_TIMEOUT_MS = 1_800_000`、`normalize_work_mode` 对应 `NormalizeWorkMode`）。

回归 `a_started_run_serves_its_snapshot_and_drops_the_active_handle`：逐项断言快照六字段；注册 live 句柄时可取消，终态后 `cancellation_registry.cancel()` 返回 false（无泄漏句柄），run 落 `COMPLETED`。

### 验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked --no-fail-fast`：**1198 passed / 0 failed**（含本批 7 条新回归）。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`：**1498 passed / 0 failed**。
- `cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`：通过；`git diff --check` 干净。
- `python3 scripts/compatibility/audit_test_parity.py`：823 → **829 function_exact**、0 重复 `rust_entry`、4451 key 不变、0 U+FFFD、Rust 测试总数 2650 → 2657。
- 本批同时把两条早期断言纠正回 Go 形态：`a_completed_run_persists_the_reply_and_audits_run_completed`（干净完成不再带 `degraded:false`）、`persist_success_marks_a_run_degraded_from_its_failed_tool_calls`（run 不再带 `reply`，改用 response 级 `reply` + `toolSummaries`）。

### 下批目标

`runner_chat_test.go` 余 5 条 `[~]`：`:56`（边界保留，Rust 无 ADK 事件循环）、`:127`、`:609`、`:737`、`:891`。建议顺序：

1. `:609` provider override 不改 agent：确认 `resolve_provider` 的 override 路径只影响当次 run（agent 行保持 `providerId`/`model`）。
2. `:737` `RunnerChatProjectionPersistenceAndAssistantBoundaries`：`persistRunActivitySnapshot`/`AuthoritativeRunSnapshot`/`appendAssistantMessageEvent`/`EnsureAssistantMessage` 与 `applySessionProjectionToRun` 的边界集合，Rust 目前只有部分对应物（`MergeRunActivitySnapshot` 语义、assistant 事件幂等）。
3. `:891` `ResolveApprovalAsyncDetachesClosedStreamBeforeBackgroundResume`：需要证明后台 resume 不会再向已关闭的 SSE 回调投递 delta。
4. `:127` tool-only final reply：先确认真实生产合成路径（只有 `tool_failure` 分支），再判定 `[x]` 或边界。
5. 新登记的 catalog 缺口：把 `strategy.optimize` 加入 `PRODUCTION_TOOL_DEFINITIONS`（`optimize_strategy` + `RequiresApprovalIn=[approval]`），并在 `ProductionAdkToolExecutor` 绑定优化任务适配器；回归要求覆盖“审批模式下 gated、`less_approval` 下释放、执行后产出 `taskId` 并让 `optimizationTaskId` 出现在 envelope”。

决策项仍需与本领域同批评估：`runner_continuation_boundaries_test.go:188`（`RUN_LEASE_CLAIM_FAILED`）、`:242`（租约存储错误传播）、`:275`/`:427`（goal-run 后台续跑引擎缺口）。

随后按 `store_test.go`（20）→ `session_context_test.go`（19）推进；`internal/app/apiserver`（349）与 `pkg/backtest`（174）保持为后续大领域批次。

## 第三十批：chat 覆盖隔离、工具轮回复与审批续跑 detach（runner_chat_test.go 结清 4 条 + 1 条边界复核）

范围：`internal/assistant/engine/runner_chat_test.go` 的 `:127`、`:609`、`:737`、`:891` 与边界复核 `:56`。四条 `[x]` 全部由端到端回归证明（真实 runtime + scripted loopback 模型端点），`[x]` 由 829 → **833**，Rust 测试 2657 → **2661**；`:56` 由 `partial` 明确收敛为 `boundary`（附探针证据），不再挂着“部分覆盖”。

### 新增回归

- 新文件 `product_adk_model_runtime_chat_turn_tests.rs`（3 条 + 共享 scripted provider 夹具）：
  - `:609` `a_provider_override_runs_the_turn_without_editing_the_stored_agent`：provider 请求带 `override-model`、run 快照冻结 override provider 的 id/name/model、store 的 agent 行保持 `provider-default`/`agent-model`。
  - `:127` `a_tool_only_turn_returns_the_second_round_reply_that_names_the_tool`：两轮 provider，第二轮请求必须携带 `function_call_output(call-status)`，run 终止 COMPLETED、`toolCalls[0].status=SUCCEEDED`、reply 非空且包含工具名。
  - `:737` `a_gated_tool_call_parks_the_run_with_only_pending_approvals`：`approval` 模式 + `http.fetch`（medium 风险）停泊，`run.status=PENDING`、只发布待审批条目、tool call 为 `PENDING_APPROVAL`、回复为审批提示，且执行器未被调用。
- `product_adk_chat_stream_product_tests.rs` 新增 `:891` `production_approval_resume_after_the_stream_closed_completes_without_late_frames`：把停泊的 SSE 响应保持连接，在其上 `POST /api/v1/adk/approvals/{id}/approve`，再把该连接读到 EOF；断言没有额外 `data:` 帧、run 落 `COMPLETED` 且 `resumeState=adk_confirmation_resolved`、reply 含第二轮文本、工具恰好执行一次。为注入夹具执行器，`with_tool_executor_for_test` 的测试可见性由 `pub(super)` 放宽为 `pub(crate)`（`#[cfg(test)]` 专用，无生产影响），并新增 `ProductionAdkPort::new_for_test` + `chat_runtime` 的审批路由夹具。

### 探针证据（四条均为真实守卫）

1. `:127`：清空第二次 `execute_model` 前的 `tool_context` 赋值 → 测试失败（第二轮请求缺少 `function_call_output`）。
2. `:609`：在 `prepare_chat` 里把 override providerId 写回 agent 行 → 测试失败（`stored agent providerId=override-provider`）。
3. `:891`：在停泊分支 final 帧后补一次 400ms 延迟的 delta 发送 → 测试失败（同一连接读到 `{"probe":"late frame after terminal"}`）。
4. `:56`：把 `run_terminal_state` 的 `ADK_INPUT_UNSUPPORTED` 分支改成 `("FAILED","MODEL_CALL_FAILED")` → 既有 Rust 测试失败，证明终态映射契约有真实守卫；Go 的 `adksession.Event.RequestedInput` 触发面在 Rust 不存在，故保留边界。

### 仍留边界（写明理由，不静默忽略）

- `:56` 的触发面（ADK 事件循环的 `RequestedInput`）不迁移；只保留已覆盖的终态映射。
- `:737` 中 `persistRunActivitySnapshot` / `AuthoritativeRunSnapshot` / `appendAssistantMessageEvent` / `EnsureAssistantMessage` / `shouldPreferProjectedToolCalls` / `terminalToolCallCount` / `pendingApprovalToolCallCount` 属于 Go「run 快照 + session 投影」双来源内部 api；Rust 的 run 行由 runtime 唯一写者持有，没有等价 api，硬造会引入第二写者，故记为 boundary。

### 本批新登记的功能缺口（下一批目标）

`strategy.optimize` 在生产 catalog 中完全缺失：`PRODUCTION_TOOL_DEFINITIONS`、`MODEL_EXPOSED_TOOLS` 与 `ProductionAdkToolExecutor` 都没有它，模型无法调用该工具（Go `internal/assistant/assembly/tool_catalog.go:574` 注册了它：permission `optimize_strategy`、`RequiresApprovalIn=[approval]`、按 `definitionIds` 批量入队真实回测任务并落 `OptimizationTask`）。下一批实现该 catalog 条目、策略与执行器，并补「approval 模式 gated / `less_approval` 释放 / 产出 `taskId` 让 `optimizationTaskId` 进 envelope」回归。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked`（1202 passed）、`pnpm run check:zero-go`、`check:compatibility`、`check:rust:architecture`、`git diff --check`、`pnpm run check:quick`；审计 4451 keys、833 function_exact、0 重复 `rust_entry`、0 非 function_exact 的 `[x]`、0 U+FFFD。

随后按 `store_test.go`（20）→ `session_context_test.go`（19）推进；`internal/app/apiserver`（574 待办）与 `pkg/backtest`（237 待办）保持为后续大领域批次。

## 第三十一批：补齐生产 catalog 缺失的 `strategy.optimize`（assembly 领域 1 条 [x] + 3 条 partial 收窄）

范围：关闭第三十批登记的真实功能缺口 —— 生产 catalog 完全没有 `strategy.optimize`，模型无法调用它。`[x]` 由 833 → **834**，Rust 测试 2661 → **2671**。

### 冻结证据（Go）

`internal/assistant/assembly/tool_catalog.go:574` 的 `registerADKStrategyOptimizationTools`：`DisplayName=策略优化`、`Category=strategy`、`Permission=optimize_strategy`、低风险、`RequiresApprovalIn=[approval]`、`RequiredSkills=[publish builtin]`；处理器按 `definitionIds`（或单个 `definitionId`）为每个候选起真实异步回测，失败时回滚已入队候选，成功落 `OptimizationTask{ID:"opt-<UTC>",Status:"queued",Objective,Runs:[{DefinitionID,RunID}]}`，返回 `{taskId,status,objective,runs,message}`。`internal/assistant/engine/sqlite_tools_test.go:261` 给出该工具 schema 的 required 集合 `definitionIds/market/symbol/startTime/endTime`。

### 实现

- `PRODUCTION_TOOL_DEFINITIONS` 新增 `strategy.optimize`（adapter 复用 `BacktestStart` 的就绪判定）；`tool_access_policy` 新增 `OPTIMIZE_APPROVAL_MODES = ["approval"]` 的显式审批列表（不能沿用 `APPROVAL_MODES`，Go 只列 approval）。
- `MODEL_EXPOSED_TOOLS` 暴露该工具；`ProductionAdkToolExecutor.supports/execute` 绑定新模块 `product_strategy_optimize_execution.rs`（≤250 行）。
- 新模块：候选解析（列表优先、单 `definitionId` 兜底、trim 去空）、12 上限、逐候选 `BacktestsWriteInput::Start`（payload 去掉 `definitionIds/objective`、写入单个 `definitionId`、规范化 `marketDataProvider`）、失败回滚、`OptimizationTask` 落库（复用既有 `adk_optimization_tasks` 与读/取消路由）、Go 形状的 `opt-YYYYMMDDTHHMMSS.nnnnnnnnn` 任务 id。
- `BacktestsWriteInput` 新增 `Cancel { run_id }`（无公开路由，供多候选工具回滚；生产实现复用此前 dead-code 的 `cancel_backtest`），并同步 test-cutover/两个夹具端口。
- `product_mcp_schema_catalog_dispatch.rs` 新增 `strategy.optimize` 的 strict schema（required 与 Go 一致，`definitionIds.maxItems=12`）。

### 回归与探针

- `strategy_optimize_enqueues_every_candidate_and_persists_the_task`、`strategy_optimize_rolls_back_candidates_and_validates_the_request`、`strategy_optimize_is_gated_in_approval_mode_only`（assembly 领域）、`candidate_*`/`optimization_task_ids_use_go_timestamp_shape`/`the_tool_schema_requires_the_candidate_list_and_execution_window`（引擎）、runtime 侧 `a_released_optimizer_call_reaches_the_production_executor`（模型可见 + 释放执行）与 `the_optimizer_is_gated_in_approval_mode`（approval 停泊、执行器不被调用）。
- 探针：①把 `requires_approval_in` 改回 `None` → 策略回归失败；②从 `MODEL_EXPOSED_TOOLS` 删除该工具 → 模型可见性回归失败；③删掉失败分支的 `cancel_candidates` → 回滚回归失败。三者回滚后全部通过。

### 收窄为 partial 的三行（写明剩余差异）

1. `adk_runtime_contracts_test.go:113`：注册与 queued run 引用已覆盖；Go 的 `EnsureBacktestData` 预检（未 ready 返回 readiness 负载且不入队）在 Rust 无等价依赖，数据缺失由写端 fail-closed，待评估是否需要工具层预检。
2. `adk_strategy_test.go:699`：请求侧 `marketDataProvider` 规范化已实现；默认 provider 由写端按候选解析（Go 在循环前冻结一次），并发改写默认值时理论上可能分裂，登记为待评估项。
3. `sqlite_tools_test.go:194`：`strategy.optimize` schema 已对齐；该 Go 测试覆盖的其余 18 个工具 schema 仍按各自领域批次核对。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`pnpm run check:zero-go`、`check:compatibility`、`check:rust:architecture`、`git diff --check`、`pnpm run check:quick`；审计 4451 keys、834 function_exact、0 重复 `rust_entry`、0 非 function_exact 的 `[x]`。

随后按 `store_test.go`（20）→ `session_context_test.go`（19）推进；`internal/app/apiserver`（574 待办）与 `pkg/backtest`（237 待办）保持为后续大领域批次。

## 第三十二批：`internal/assistant/engine/store_test.go` 全量结清（13 条 [x] + 3 条 Go-skip 边界 + 2 条登记缺口 + 1 条可达子集 partial）

范围：`internal/assistant/engine/store_test.go` 的 19 条待办逐行核对，先跑参考夹具确认真实基线（19 条中 16 条通过、3 条为 `t.Skip`）。`[x]` 由 834 → **847**，Rust 测试 2671 → **2682**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run '<本批 19 个 Test*>' -count=1`：16 条通过；`TestStoreMigrationNormalizesHiddenAgentWorkflowDefaults`、`TestStoreMigrationRepairsOrphanTasksAndDuplicateConfirmations`、`TestStoreMigrationReopensCompletedWorkflowWithRecoverablePendingApproval` 打印 `incremental ADK migrations were intentionally removed; strict incompatibility is covered below` 后 SKIP，故这三行记为 boundary 而不是未覆盖。

### 本批真实功能修复（3 处）

1. **run 超时来自持久化 settings（`TestStartRunUsesConfiguredRuntimeTimeout`、`TestResumeGoalRunAllowsTimedOutGoalWithFreshTimeoutWindow`）**：Rust 原先把 `maxDurationMs` 写死 1800000。新增 `crate::product::product_adk_run_timeout::assistant_run_timeout_ms(settings_path)`（读 settings 文档 `adk.runTimeoutMs`，缺失/不可读回退 Go 默认 1800000），chat 起始 payload 与 `ResumeRun`（TIMED_OUT 分支）都改用它；`ProductionAdkChatRuntime` 新增 `settings_path` 字段并沿用到 facade（stream/dispatch 分支）。回归：`run_start_freezes_the_configured_run_timeout_from_settings`、`resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window`。
2. **provider 请求超时保存即归一化（`TestProviderSecretIsNotEchoed`、`TestProviderRequestTimeoutDefaultsAndClamp`）**：Go 的 `StoreCore.SaveProvider` 用 `NormalizeProviderRequestTimeoutMs` 落库（<=0 → 180000，区间 [15000,600000]），Rust 原先只存原值、执行期兜底还是 120000。修复：保存路径新增 `normalize_provider_request_timeout_ms`，执行期兜底常量改为 180000。
3. **拒绝审批的本地答复文本（`TestApprovalDenialCreatesAssistantSummary`）**：Go 用 `model.ApprovalResolutionSummary` 渲染 `已拒绝工具调用 \`<tool>\`。本次 run 已结束，未执行该操作。`，Rust 之前把 transcript 内容写成固定 `approval denied`。修复：拒绝分支新增 `denied_approval_summary`（优先 `pendingApprovals[0].toolName`，回退 DENIED 工具调用名），run.Message 仍保持 `approval denied`、errorCode/failureReason 置空。

另：`product_adk_model_runtime.rs` 因新增字段触到 800 行上限，把 `RunCancellationRegistry` 抽到 `product_adk_model_runtime_cancellation.rs`（`include!`，纯机械搬移），生产文件回到 743 行。

### 新增回归（9 条 Rust 测试）

- `crates/jftrade-store-sqlite/tests/adk_store_contracts.rs`：`adk_store_fences_a_second_writer_and_serializes_concurrent_access`、`adk_store_rejects_a_legacy_database_without_rewriting_it`（逐字节未变 + 无 schema metadata）、`adk_store_schema_has_no_legacy_message_tables`、`adk_tool_call_staging_admits_one_confirmation_winner_under_concurrency`（24 线程同一 confirmation，恰好 1 次 staging 成功、恰好 1 行）、`adk_approval_resolution_restages_a_stale_embedded_approval`。
- `crates/jftrade-engine`：`run_start_freezes_the_configured_run_timeout_from_settings`、`a_hanging_provider_is_bounded_by_the_request_timeout`（只接受不回包的 loopback provider + 150ms 超时 → 504 `MODEL_CALL_TIMEOUT`，实测 0.17s）、`resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window`、`saved_provider_hides_the_credential_from_the_row_and_projection`、`saved_provider_normalizes_the_request_timeout_on_write`、`denied_approval_summary_renders_the_go_denial_reply_text`。

### 探针（改坏 → 转红 → 回滚）

1. `"maxDurationMs": self.run_timeout_ms()` 改回常量 → `run_start_freezes...` 转红（1800000 vs 660000）。
2. `ResumeRun` 的 `maxDurationMs` 改回常量 → resume 用例转红（1800000 vs 2700000）。
3. 拒绝事件内容改回 `"approval denied"` → 审计用例转红。
4. provider 归一化替换成直存原值 → 超时用例在 180000 断言处转红。
5. 让 `WriterLease` 忽略 `try_lock` 冲突 → 单写者用例在第二所有者断言处转红。
6. 把 staging 的 `status/revision` 条件改成恒真 → 确认并发用例转红（重复插入被唯一索引拒绝）。
7. `should_continue = !has_pending && changed` → stale-embedded 用例转红。
8. client timeout 放大 100 倍 → 挂起 provider 用例在“必须及时返回”断言处转红（15s）。

### 保留为 partial / boundary（写明剩余差异，不计入通过）

- `:607`（partial）：Go 的 `ReconcileResolvedApprovals` 在读路径上修复“审批行已解析但 run 仍等待”的运行；Rust 的审批解析与 run 状态迁移在同一事务内，读路径不持有 runner 也不做 reconcile，因此可达状态（RUNNING + approval_resuming）由 `recover_approval_continuations` + `resume_approval` 恢复（已有测试），手工编辑/历史库中的 PENDING+已解析状态不会被读请求治愈（再次解析会重新 staging）。
- `:947`、`:1005`（partial，登记为下一批 P1 缺口）：Rust 没有 run 级到期回收 `ReconcileExpiredRuns`（按 run 自身 `maxDurationMs` 扫描 RUNNING run、取消活跃 run、把 RUNNING 工具调用改 FAILED、落 TIMED_OUT + `run.timed_out` 审计，并在 runs/run 详情/取消入口调用）。现有覆盖仅是单次模型调用超时分类（`MODEL_CALL_TIMEOUT` → `RUN_TIMED_OUT`）与启动期 orphan/approval 扫描。
- `:75`、`:127`、`:198`（boundary）：参考基线自身 `t.Skip`（增量 ADK 迁移已被刻意移除），Rust 用严格 schema fail-closed 取代，不迁移这些修复逻辑。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --all-targets --locked`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`、`git diff --check`、`pnpm run check:quick`。

随后按 `session_context_test.go`（19）→ 本批登记的 `ReconcileExpiredRuns` 缺口（`store_test.go:947/:1005`）→ `store_lifecycle_test.go`（17）推进；`internal/app/apiserver`（574 待办）与 `pkg/backtest`（237 待办）保持为后续大领域批次。

## 第三十三批：run 级到期回收落地 + 会话上下文窗口按有效 provider 解析（`store_test.go:947/:1005` 结清，`session_context_test.go` 4 条结清）

范围：结清上一批登记的 P1 缺口 `ReconcileExpiredRuns`（`store_test.go:947`、`:1005`），并开始 `internal/assistant/engine/session_context_test.go`（19 条待办中的 4 条）。`[x]` 由 847 → **853**，Rust 测试 2682 → **2691**，待办 `[~]` 减少 6 条。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestReconcileExpiredRuns|TestSessionContextCompactionShrinksSessionView|TestSessionContextUsesSessionProviderOverrideWindow|TestSessionContextCompactionCreatesCurrentRevision|TestSessionContextViewDoesNotAutoCompact' -count=1`：6 条全部通过。

### 本批真实功能修复（2 处）

1. **run 级到期回收（`store_test.go:947/:1005`）**：新增 `crates/jftrade-engine/src/product_adk_model_runtime_expiry.rs`（`include!` 片段，保持 `product_adk_model_runtime.rs` / `..._events.rs` 在 800 行内），实现 `ProductionAdkChatRuntime::reconcile_expired_runs`：扫描 RUNNING run → 取 `startedAt`（缺省 `createdAt`）+ 每个 run 自己的 `maxDurationMs`（<=0 回退 1800000）→ 跳过休眠的 workflow 子 run 与持有新鲜外部 lease 的 run → 取消活跃 provider 调用、把 RUNNING 工具调用置 FAILED（带 `run timed out while waiting for model or tool completion`、`completedAt`/`updatedAt`/`durationMs`）→ run 落 TIMED_OUT（`message=run timed out`、`failureReason="run exceeded maximum duration of <go duration>"`、`errorCode=RUN_TIMED_OUT`、`degraded=true`、FinalizeRunUsage 写 `usage.durationMs`）→ `status+revision` CAS 落库 → 写 `run.timed_out` 审计（detail `Agent run timed out.`，id `<run>:audit:run.timed_out`）。`AdkChatStreamPort` 新增默认 `reconcile_expired_runs`（夹具端口保持 no-op），生产 runtime 覆写；`GET /api/v1/adk/runs`、`GET /api/v1/adk/runs/{id}` 与 `CancelRun` 入口在读取前先回收（对齐 `internal/api/assistant/session_run.go:188/290` 与 `runner.go` 的 CancelRun）。`failureReason` 的时长文本由 `go_duration_string` 复刻 Go `time.Duration.String()`（`1ms`/`1.5s`/`1m0s`/`1h0m0s`）。
2. **会话上下文窗口按有效 provider 解析（`session_context_test.go:120`）**：Rust 之前只读 session 行的 `contextWindowTokens`，而 `CreateSession` 从不写该字段，等于永远 0/unknown。新增 `crates/jftrade-engine/src/product_production_ports_adk_context_window.rs`，按 Go `Runtime.resolveSessionContextAgent` + `SessionContextManager.contextWindowTokens` 解析 composer `providerIdOverride` → agent `providerId` → provider `contextWindowTokens`；读取已落库投影时用 `patch_context_window` 重新解析窗口/`usageRatio`/`status`/`recentUserWindow`（只改响应，不改行）；压缩路由同样改用该解析，并把缺省保留窗口从写死的 10 改为 Go `NormalizeRecentUserWindow`（<=0→6、<2→2、>100→100）作用于会话有效 agent（请求体的 `recentUserWindow` 仍可覆盖）。`context_status_for_read` 随之上移到共享模块，读路径与压缩路径不再各有一套阈值。

### 新增回归（9 条 Rust 测试）

- `crates/jftrade-engine/src/product_adk_model_runtime_expiry_tests.rs`（5 条）：`expired_running_run_is_reconciled_to_timed_out_with_failed_tool_calls`（31 分钟前的 RUNNING run + RUNNING 工具调用 → 全部终态字段、工具调用 FAILED、`usage.durationMs>0`、审计行）、`expired_runs_use_each_runs_own_timeout_window`（60000ms 到期 / 300000ms 保持 RUNNING，`failureReason` 含 `1m0s`）、`a_fresh_foreign_run_lease_shields_a_run_from_expiry`（外部 runtime 持新鲜 lease → 不回收、无审计）、`the_run_read_routes_reconcile_expired_runs_before_serving`（真实 `ProductionAdkPort` + `/api/v1/adk/runs?status=TIMED_OUT` 与 run 详情）、`go_duration_string_matches_the_reference_formatting`（11 组值与 Go 实测输出逐一对齐）。
- `crates/jftrade-engine/src/product_adk_session_context_tests.rs`（4 条）：`context_compaction_shrinks_the_projected_session_view`、`session_context_window_follows_the_composer_provider_override`、`each_context_compaction_creates_the_next_current_revision`、`session_context_read_reports_pressure_without_compacting`。

### 探针（改坏 → 转红 → 回滚）

1. `timeout_ms` 强制为 DEFAULT 常量 → `expired_runs_use_each_runs_own_timeout_window` 转红（300000ms 的 run 被误判）。
2. 跳过 `fresh_foreign_run_lease` → `a_fresh_foreign_run_lease_shields_a_run_from_expiry` 转红（RUNNING vs TIMED_OUT）。
3. `finish_running_tool_calls` 跳过 RUNNING 调用 → `expired_running_run_is_reconciled_to_timed_out_with_failed_tool_calls` 转红（RUNNING vs FAILED）。
4. 移除读路由入口的 `reconcile_expired_runs` → `the_run_read_routes_reconcile_expired_runs_before_serving` 转红（`page.total` 0 vs 1）。
5. 窗口解析恒返回 0 → `session_context_window_follows_the_composer_provider_override` 与 `session_context_read_reports_pressure_without_compacting` 均转红（窗口 0、status unknown）。
6. 移除读取分支的 `patch_context_window` → 窗口回落到压缩时的 1000（期望 200000），override 用例转红。
7. `breakdown.handoffTokens` 改为合并全部 active 段 → `each_context_compaction_creates_the_next_current_revision` 转红（54 vs 84）。

### 仍未结清（下一批继续）

- `session_context_test.go` 剩余 15 条 `[~]`：上下文提示（`contextCompactionStartedText/DoneText` timeline 通知与 `SessionNotices` 持久化）、`maybeAutoCompactSession` 自动压缩 deltas、会话级压缩 gate、workflow 期间允许活跃父 run、模型上下文读取前自动压缩、`protectedTailStart` 的未解析审批锚点（Go 依赖 `toolconfirmation` 结构化事件，Rust 事件为纯文本/工具 JSON，需要先定 Rust 的等价锚点语义），以及 `AppendADKEventWithStaleRetryRefreshesSession` 等 Go ADK 库表面用例。
- `session_context_test.go` 中 `TestHasActiveRunDoesNotTreatPendingApprovalAsExecuting`、`TestCompactedSessionPreservesOriginalCallForPendingApproval` 亦未结清。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1657 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2691 Rust / 853 `[x]`）、`git diff --check`、`pnpm run check:quick`。

下一批：继续 `session_context_test.go`（先做上下文提示 + 自动压缩 deltas，再评估 `protectedTailStart` 的 Rust 锚点语义）。
## 第三十四批：会话压缩 gate 与上下文通知落地（`session_context_test.go:287` 结清）

范围：第三十三批登记的下一批首组第一项——上下文压缩通知与 gate。`[x]` 由 853 → **854**，Rust 测试 2691 → **2694**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestCompactSessionContextWritesContextNotice|TestMaybeAutoCompactSessionSkipsWhenSessionCompactionAlreadyRunning|TestSessionServiceAutoCompactionUsesSessionGate' -count=1`：3 条通过。

### 本批实现（3 处）

1. **store 侧通知读写**：`crates/jftrade-store-sqlite/src/adk.rs` 新增 `save_session_notice`（按 id upsert，保留首次 `created_at`）与 `list_session_notices`（`ORDER BY created_at ASC, id ASC`），对应 Go `StoreCore.SaveSessionNotice` / `SessionNotices`。
2. **会话级压缩 gate**：`crates/jftrade-engine/src/product_adk_session_compaction_gate.rs` 复刻 Go `Runtime.beginSessionCompaction`——按 session 键控的进程内集合，guard drop 即释放；空 session id 直接放行。手动压缩路由在 gate 被占用时返回 `500 ADK_SESSION_CONTEXT_COMPACT_FAILED` / `session context compaction already running`（Go 的错误文本，handler 对 active run 之外的错误映射 500）。
3. **上下文通知**：新增 `crates/jftrade-engine/src/product_production_ports_adk_notices.rs`，复刻 Go `context_notice.go` 的三段文本与 `streaming → final/error` 生命周期、id 形状 `notice-<session>-<timestamp>`；`compact_session_context` 拆成外层（gate + 通知）与 `compact_session_context_locked`（活跃 run 检查 → 投影重建 → durable 写入），失败路径同样落 error 通知（与 Go 在活跃 run 409 时仍写错误通知一致）；`GET /api/v1/adk/sessions/{id}` 的 timeline 现在按 Go `BuildSessionTimeline` 的语义把通知与消息合并后重排 `sequence`。

### 新增回归（3 条 Rust 测试）

- `manual_context_compaction_writes_the_done_notice_into_the_timeline`：手动压缩后 timeline 出现唯一 `context_notice`，`status=final`、文本为「已压缩上下文，继续使用最新摘要。」，且转录条目仍在同一 timeline。
- `a_second_compaction_is_rejected_while_the_session_gate_is_held`：持锁时路由返回 500 且**不**写通知；释放后同一请求成功并落 final 通知。
- `a_rejected_compaction_records_the_failed_notice`：存在 RUNNING run 时 409，timeline 落 `error` 通知与失败文本。

### 探针（改坏 → 转红 → 回滚）

1. 不创建通知 → 手动压缩与失败通知两条用例转红（timeline 无 context_notice）。
2. 成功分支不更新为 final → 手动压缩用例转红（status `streaming` vs `final`）。
3. gate 恒成功（跳过重复插入检查）→ 持锁用例转红（第二次压缩被执行，返回快照而非 500）。

### 新增锚点

- 为新 `[x]` 引用的 36 个 Rust 测试函数补 `// Parity: go:452dea11:<go 文件>:<行> <TestName>` 锚点（14 个文件：`crates/jftrade-assistant` 3 个源文件 + 1 个集成测试、`crates/jftrade-engine` 6 个、`crates/jftrade-store-sqlite` 3 个、`crates/jftrade-engine/tests` 1 个）。
- 未锚定 `function_exact` 告警从 229 回到 **193**（本批新增 36 条批准全部有代码侧锚点，未增加历史欠账）。

### 仍未结清（下一批）

- `:341`、`:446`、`:506`、`:569`、`:703`：Rust 仍缺 `maybeAutoCompactSession(DuringWorkflow)` 与 `AutoCompactForModelContext`——阈值（0.85 auto / 0.93 aggressive）、pending user text 投影、streaming→final 通知 delta 与 context delta、活跃 RUNNING run 跳过（workflow 入口允许）、模型上下文读取前自动压缩。gate 与通知基础设施本批已就绪，下一批可直接接线；`:446`、`:506` 的结论已同步更新为“gate 已实现、自动压缩入口待补”。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1660 passed）、`pnpm run check:rust:architecture`、`git diff --check`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2694 Rust / 854 `[x]`）。
## 第三十五批：自动压缩入口与模型上下文前置压缩（`session_context_test.go` 再结清 5 条）

范围：接上第三十四批已就绪的 gate 与通知设施，落地 Go 的自动压缩入口与模型上下文前置压缩。`[x]` 由 854 → **859**，Rust 测试 2694 → **2700**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestMaybeAutoCompactSessionEmitsContextNoticeDeltas|TestMaybeAutoCompactSessionSkipsWhenSessionCompactionAlreadyRunning|TestSessionServiceAutoCompactionUsesSessionGate|TestMaybeAutoCompactSessionDuringWorkflowAllowsActiveParent|TestModelContextReadAutoCompactsBeforeProviderPayload' -count=1`：5 条通过。

### 本批实现（4 处）

1. **压缩核心改为可复用的 store 级函数**：`compact_session_context_locked` 改名 `compact_session_projection`，签名改为 `(&AdkStore, &AdkSessionStore, session_id, session, mode, trigger, reason, recent_window, require_no_active_run)` 并放宽到 `pub(crate)`，手动路由与运行时共用同一份实现（避免在 runtime 里复制压缩逻辑）。
2. **自动压缩入口**：新增 `crates/jftrade-engine/src/product_adk_model_runtime_auto_compaction.rs`——`projected_context_projection`（durable 投影 + pending user text token）、`auto_compaction_mode`（0.85 / 0.93）、`session_has_running_run`（仅 RUNNING 阻塞，对齐 Go `HasActiveRun` 注释）、`maybe_auto_compact_session(allow_active_run, on_delta)`（gate → streaming 通知 delta → 压缩 → final 通知 delta + context delta；失败走 error 通知并返回错误）、`auto_compact_for_model_context`（无通知、取 gate、`... before model call` 理由文本）。
3. **接线**：`prepare_chat` 在创建 run 之前调用常规入口（Go `RunChat` 的顺序：run 行尚不存在，因此不会看到自己的 RUNNING run 而跳过——这是本批踩到的顺序陷阱）；两个 `durable_context_items` 调用点（新建与恢复路径）之前调用 `auto_compact_for_model_context`，使 provider payload 基于压缩后的投影。
4. **修复真实缺陷**：`autoCompacted` 原先写成 `last_mode == "auto"`，导致 aggressive+auto 的自动压缩被报告为 `false`；Go 的规则是 `state.AutoCompacted = trigger == "auto"`（`LastCompactionMode` 仍按 `compactionModeLabel` 保留 `aggressive`）。由新测试暴露并修正。

### 新增回归（6 条 Rust 测试）

- `auto_compaction_emits_streaming_then_final_notice_and_context_delta`（:341）：2 条 timeline delta 同 id streaming→final + 1 条 context delta（currentInputTokens 收缩、autoCompacted=true、compactedEventCount>0），并核对 `adk_session_notices` 恰好 1 行。
- `auto_compaction_skips_while_another_compaction_holds_the_session_gate`（:446）。
- `workflow_auto_compaction_proceeds_under_an_active_run_while_chat_waits`（:569）。
- `model_context_read_compacts_only_when_the_session_gate_is_free`（:506）。
- `model_context_autocompacts_before_the_provider_payload`（:703）：`durable_context_items` 由 80 条收缩。
- `a_chat_turn_autocompacts_the_session_before_the_provider_payload`：端到端跑 `prepare_chat`（带凭据 provider + 80 token 窗口），断言会话落 autoCompacted。

### 探针（改坏 → 转红 → 回滚）

1. `auto_compaction_mode` 恒返回 None → 6 条用例转红（压缩不再发生）。
2. gate 恒成功 → 持锁自动压缩用例转红（gated 调用直接压缩并发布 delta）。
3. active-run 跳过条件恒 false → workflow 用例转红（常规入口在存在 RUNNING run 时仍压缩）。

### 仍未结清（下一批）

- `:773`/`:787`/`:800`/`:813`（保护尾部锚点，需先定 Rust 结构化锚点语义）、`:827`、`:868`、`:922`、`:977`、`:1009`。
- SSE 帧转发：自动压缩产生的 notice/context delta 目前只由 runtime 方法返回（Go 测试同样直接调用该方法），尚未转发进 chat SSE 流；workflow canvas 的 `MaybeAutoCompactSessionDuringWorkflow` 调用点接线同样待补（Rust 已提供 `allow_active_run=true` 入口）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1666 passed）、`pnpm run check:rust:architecture`（`product_adk_model_runtime_events.rs` 触限后把 include 移到 lifecycle 片段并压缩注释，回到 800 行）、`git diff --check`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2700 Rust / 859 `[x]`）。
## 第三十六批：保护尾锚点与 session context 读取边界（`session_context_test.go` 结清 9 条）

范围：把 `internal/assistant/engine/session_context_test.go` 剩余 9 行逐条映射到 Rust，并修复读取重建路径的真实差异。`[x]` 由 859 → **867**，Rust 测试 2700 → **2709**（engine+store nextest 1675 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestProtectedTail|TestSessionContextIgnoresHandoffSegmentsWithoutRevision|TestAppendADKEventWithStaleRetryRefreshesSession|TestCompactedSessionViewTracksEventsAppendedDuringInvocation|TestHasActiveRunDoesNotTreatPendingApprovalAsExecuting|TestCompactedSessionPreservesOriginalCallForPendingApproval' -count=1`：9 条通过；`TestProtectedTail` 单独复跑（4 条）与文件内 helper 定义逐条核对。

### 本批实现（3 处）

1. **结构化保护尾锚点**：`protected_context_event_start` 从「content 子串包含 approval」改为解析真实写入的 `assistant.tool_call` 信封：`status=PENDING_APPROVAL` 的调用是锚点，`assistant.tool` 结果（同名 `callId`）或 `{runId}:denied` 终止事件表示审批已解决；信封带 `functionCallId`/`originalCallId` 时回退到原始调用事件索引（对应 Go `OriginalCallFrom` + `functionCallEventIndex`）。
2. **读取与压缩共享同一锚点**：`product_production_ports_adk_mutation_context.rs` 删除本地 `protected_tail_start` 副本，`/sessions/{id}/context` 快照与压缩 cutoff 共用 read 侧实现，避免两条路径分裂。
3. **修复无 revision 旧 handoff 段被采纳**：`rebuild_context_snapshot` 原先在「无 context-state 行」时把所有 active 段当成当前链，导致无 `contextRevisionId` 的旧段重新出现在 `summaryPreview`。现在只有携带当前 revision 的段参与投影；`/sessions/{id}/context` 首次读取时像 Go `ensureSessionContextRevision` 那样锚定并持久化新 revision（`ctx-<uuid>`），后续读取稳定。

### 新增回归（9 条 Rust 测试）

- `protected_tail_starts_at_the_earliest_unresolved_approval`（:773）
- `protected_tail_rewinds_a_pending_approval_to_its_original_call`（:787）
- `protected_tail_ignores_an_approval_with_a_durable_tool_outcome`（:800）
- `protected_tail_keeps_only_the_approval_that_is_still_pending`（:813）
- `protected_tail_ignores_approvals_closed_by_a_denied_run`（denied run 关闭全部待审批的补充用例）
- `session_context_ignores_handoff_segments_without_a_revision`（:827）
- `session_context_tracks_events_appended_after_a_compaction`（:922）
- `a_run_waiting_for_approval_does_not_block_chat_auto_compaction`（:977，同一用例同时断言 RUNNING 仍阻塞 chat 入口）
- `compaction_preserves_the_call_of_a_pending_approval`（:1009）

### 探针（改坏 → 转红 → 回滚）

1. 去掉「已解决审批跳过」分支 → `:800`、`:813`、denied 三条转红（已解决审批仍锚定保护尾）。
2. 忽略 `functionCallId` 回退 → `:787` 转红（起点停在审批信封索引 3 而非原始调用索引 1）。
3. 读取重建不再锚定 revision → `:827` 转红（`contextRevisionId` 为空，legacy 段仍被采纳）。

### 仍未结清（下一批）

- `:868`（Go `appendADKEventWithStaleRetry`）标为边界保留：Rust 按 session 主键直接追加事件，没有 ADK session 句柄与 append 锁表层。
- 已知差异（本批未修，登记为 P1）：`/sessions/{id}/context` 在已有 context-state 行时直接返回已存 payload，压缩后追加事件不会刷新 `rawEventCount`/`currentInputTokens`（Go 每次 Snapshot 重算并保存）；Rust 的 timeline 读取本身是实时的。
- 自动压缩 notice/context delta 仍未转发进 chat SSE 帧；workflow canvas 的 `allow_active_run=true` 调用点待接线（承自第三十五批）。
- 下一批进入 `store_lifecycle_test.go`（17 行），随后 `input_request_test.go`（15）、`mcp_server_test.go`（13）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1675 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`（`product_production_ports_adk_read.rs` 触限后压缩注释回到 798 行）、`git diff --check`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2709 Rust / 867 `[x]`）。
## 第三十七批：context 快照重算、自动压缩 SSE 帧与 store 生命周期首批（`store_lifecycle_test.go` 结清 4 条）

范围：三块——(1) 修第三十六批登记的 P1：`/sessions/{id}/context` 在已有 context-state 行时也按 Go Snapshot 语义重算；(2) 把自动压缩 notice/context delta 转发进 chat SSE 流；(3) `store_lifecycle_test.go` 首批 4 条结清 + 1 条功能缺失登记。`[x]` 867 → **871**，Rust 测试 2709 → **2713**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestProvidersMaintainDefaultSelectionAndCreatedOrder|TestRejectUnsafeHost|TestInternalSkillCannotBeUninstalled|TestExternalSkillUninstallRemovesInstallDir' -count=1`：4 条通过；本批范围内 17 条一起跑亦通过（余下 13 条已完成语义分析，见「仍未结清」）。

### 本批实现（4 处）

1. **context 快照实时重算**：新增 `refresh_context_snapshot`，`/sessions/{id}/context` 在存在 context-state 行时用持久 revision 重建投影并覆盖派生指标（`rawEventCount`/`currentInputTokens`/breakdown 等），同时保留 state 独有的 `previousContextRevisionId`、`lastCompactionTrigger`、`autoCompacted`；读路径不写库，避免与压缩 CAS 竞态。`/context` 分支整体移入 `session_context_snapshot`，`product_production_ports_adk_read.rs` 回到 751 行。
2. **自动压缩 delta 收集**：`SessionContextDelta::sse_frame()`（timeline/context 帧）与 `collect_auto_compaction_deltas`；`ChatExecution.context_deltas` 携带 Go `onDelta` 在 run 建立前发布的压缩 delta。
3. **SSE 接线**：`start_live_stream` 在 `retry:` 之后、session/run 帧之前把 deltas 作为 SSE 帧发出，顺序对齐 Go `adkChatStreamExecution.handleDelta`。
4. **store 层原子替换证据**：新增 store 测试固定 `delete_provider_with_replacement_atomic` 的 payload 写入、默认优先排序与唯一默认归一化。

### 新增回归（4 条 Rust 测试 + 2 处既有测试扩展）

- `provider_delete_promotes_the_replacement_and_keeps_one_default`（jftrade-store-sqlite，:45）
- `reject_unsafe_host_blocks_the_reference_host_table`（:701）
- `builtin_skill_uninstall_is_refused_and_the_projection_keeps_it`（:711）
- `production_live_chat_stream_emits_auto_compaction_frames_before_the_run`（端到端走真实 `/api/v1/adk/chat/stream`）
- `provider_default_contract_orders_the_default_first_and_keeps_the_route_code` 扩展「删除默认后仍有唯一默认排第一」断言；`session_context_tracks_events_appended_after_a_compaction` 扩展「快照指标跟随追加事件」断言。

### 探针（改坏 → 转红 → 回滚）

1. `session_context_snapshot` 直接返回已存 payload → 快照指标跟随断言转红。
2. `start_live_stream` 不发送 `context_deltas` → 端到端 SSE 用例转红（缺 context_notice/context 帧）。
3. `delete_provider_with_replacement_atomic` 跳过 replacement 写入 → store 用例转红（replacement payload 未落库）。
   补充记录：引擎层「删除默认后提升」断言单独探针不会转红，因为 `list_providers` 的默认归一化会兜底；因此 :45 的替换证据落在 store 层测试。

### 仍未结清（下一批）

- `store_lifecycle_test.go` 余量 13 条：`:110`（删除会话级联 approvals/runs/tasks/messages，需核对 `adk_cascade_session_cleanup` 覆盖范围）、`:146`/`:250`/`:281`/`:328`（Go `SaveRun` 生命周期单调性——Rust 没有全量 `SaveRun` 入口，只有 CAS 状态更新，需逐条给出等价证据或边界结论）、`:466`（sessions 分页过滤）、`:501`（composer state 持久化/截断/级联删除）、`:553`（删除会话空白/缺失语义）、`:565`（approvals 分页排序）、`:609`（optimization tasks 排序）、`:641`（**功能缺失**：Rust 无 `<execute-tool>` 标签解析路径，已登记 P1）、`:765`（prepared agent 只加载 enabled 绑定）、`:792`（skill registry 元数据）。
- workflow 侧 Go `MaybeAutoCompactSessionDuringWorkflow(allowActiveRun=true)` 的调用点（final synthesis / resumed execution）在 Rust 没有直接对应：Rust 每个 workflow agent 节点使用独立 session 并经普通 chat 入口压缩，且没有独立的 final synthesis 阶段；按结构差异记录，不再按「缺失接线」处理。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1679 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / 2713 Rust / 871 `[x]`）。

观测记录：本批首次全量 nextest 出现 1 条偶发失败 `adk_session_detail_omits_resolved_approval_groups`（单跑、engine lib 全量与第二次全量均通过），失败与本批改动无直接关联；如再次复现需按并发时序专项排查。

## 第三十八批：`store_lifecycle_test.go` 全量结清（列表排序修复、run 生命周期 CAS 证据与内置 skill 工具分层）

范围：`store_lifecycle_test.go` 余下 13 条逐条结清——6 条 `[x]`、7 条带明确结论的 `[~]`；同时修 3 处真实行为差异（approvals 排序键、optimization task 排序键、两个策略内置 skill 的工具分层），并更正第三十七批对 `:641` 的登记口径。`[x]` 871 → **877**，Rust 测试 2713 → **2725**（engine+store nextest 1679 → **1691**）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run 'TestDeleteProviderFailsWhenReferencedByAgent|TestProvidersMaintainDefaultSelectionAndCreatedOrder|TestDeleteSessionRemovesApprovals|TestSaveRunDoesNotRegressTerminalLifecycle|TestSaveRunReopensCompletedRunForFreshPendingApproval|TestSaveRunAllowsPausedWorkflowLifecycleUpdates|TestSaveRunPreservesUserGoalPauseLifecycle|TestListSessionsPageFiltersQueryAndPaginates|TestSessionComposerStatePersistsAndDeletesWithSession|TestDeleteSessionMissingAndBlankAreNotFound|TestListApprovalsPageFiltersAndSortsNewestFirst|TestListOptimizationTasksSortsByUpdatedAtDesc|TestExecuteToolTagInvokesCanonicalToolWithParameters|TestRejectUnsafeHost|TestInternalSkillCannotBeUninstalled|TestExternalSkillUninstallRemovesInstallDir|TestPreparedAgentLoadsOnlyEnabledBoundSkillsAndTools|TestSkillRegistryReportsMetadataAndAllowedTools' -count=1`：18 条全通过（1.251s）。

### 本批修复（3 处生产行为）

1. **approvals 排序键**：`AdkStore::list_approvals` 由 `created_at DESC` 改为 `updated_at DESC, id ASC`，对齐 Go `StoreCore.ListApprovals`。此前“先创建、后被解决”的审批不会前移，`/api/v1/adk/approvals` 与 Go 队列顺序不一致。
2. **optimization task 排序键**：`list_optimization_tasks` 由通用 `created_at DESC` 改为 `updated_at DESC, id ASC`（新增 `list_entities_ordered` 辅助，通用 `list_simple_entities` 顺序不变），对齐 Go `StoreCore.ListOptimizationTasks`。
3. **内置策略 skill 工具分层**：新增 `curated_skill_tools`，`jftrade-strategy-research` / `jftrade-strategy-publish` 改用 Go `pkg/strategy/pinespec` 的 curated `allowed-tools` 清单（∩ 当前已注册工具，保持目录顺序）。修复前两个 skill 都按 `strategy+backtest` 类别派生，导致研究 skill 暴露 `strategy.optimize` 等写工具、发布 skill 暴露 `strategy.research_backtest`，与 Go 的最小权限契约相反。

### 新增回归（12 条 Rust 测试）

- `crates/jftrade-store-sqlite/tests/adk_run_lifecycle_cas.rs`（5 条）：`run_terminal_state_cannot_be_regressed_by_a_stale_running_snapshot`（:146）、`completed_run_reopens_only_for_a_fresh_durable_approval`（:250）、`paused_workflow_run_keeps_accepting_progress_and_terminal_updates`（:281）、`user_goal_pause_fields_survive_a_stale_writer_and_clear_on_explicit_resume`（:328）、`session_delete_missing_is_idempotent_and_blank_ids_are_rejected`（:553）。
- `crates/jftrade-engine/src/product_adk_store_parity_tests.rs`（7 条，作为 `product_production_ports_adk_tests` 的子模块 `store_parity`）：`adk_session_page_filters_by_agent_and_title_and_paginates`（:466）、`adk_approval_page_orders_by_latest_update_and_counts_filtered_rows`（:565）、`adk_optimization_task_page_orders_by_latest_update`（:609）、`adk_composer_state_truncates_trim_and_rejects_invalid_modes`（:501）、`adk_session_delete_missing_is_reported_with_the_session_error_code`（:553）、`adk_agent_write_requires_registered_skills_and_keeps_declared_tools`（:765）、`adk_builtin_strategy_skills_publish_the_curated_tool_split`（:792）。
- `:110` 复用既有 `crates/jftrade-store-sqlite/tests/adk_cascade_session_cleanup.rs::test_adk_cascade_cleanup_removes_all_entities_across_three_databases`：三库 14 表计数归零（含 approvals/runs/tasks/events/composer）＋删除边界后的 stale 写入全部被 fence，是 Go 断言的超集，本批登记为 `[x]`。

### 探针（改坏 → 转红 → 回滚）

1. `curated_skill_tools` 返回 `None`（回退类别派生）→ `adk_builtin_strategy_skills_publish_the_curated_tool_split` 转红：research tools 变为 `["strategy.definitions","strategy.validate_pine","strategy.research_backtest","strategy.optimize",...]`，缺 `workflow.wait`/`market.*` 且含写工具。
2. `list_approvals` 回退 `created_at DESC` → `adk_approval_page_orders_by_latest_update_and_counts_filtered_rows` 转红：刷新较老审批后仍返回 `["approval-newer","approval-older"]`。
3. `list_optimization_tasks` 回退 `created_at DESC` → `adk_optimization_task_page_orders_by_latest_update` 转红：更新较老任务后仍返回 `["opt-newer","opt-older"]`。

### 结论登记（7 条 `[~]` 的边界与缺口）

- **`:146`/`:250`/`:281`/`:328`（Go `SaveRun` 生命周期）**：Rust 没有全行 `SaveRun`，run 写入是 `status+revision` CAS（必要时再叠加 run lease）。四条测试固定了可达等价语义：stale RUNNING 快照无法回归终态、CANCELLED 接受 `finalMessageId` enrichment、COMPLETED+`workflowStatus=RUNNING` 中间态不可回退但接受终态纠正、COMPLETED 可在其 revision 上带新 pending approval 重开且重放被 fence、paused workflow 继续接受进度与终态更新、stale 写入无法清空 `pauseRequestedAt`/`resumeState` 而显式 resume 可以清空。**未对齐子项（P1 follow-up）**：Go 的 `SaveRun` SQL 谓词还禁止“已有终态→另一终态”并强制 `COMPLETED→PENDING` 必须带新 pending approval；Rust CAS 在持有当前 revision 时允许这些组合。实际 HTTP 路径不可达（`CancelRun` 对终态已 no-op，`PauseRun`/`ResumeRun` 只接受 RUNNING/PAUSED/TIMED_OUT），要逐字对齐需在 CAS 增加 Go 谓词与两条例外，并逐调用点验证。
- **`:553`（删除会话空白/缺失）**：Go store 语义为 `""→os.ErrNotExist`、`missing→nil`；Rust `delete_session("")`（与保留 `user`）返回 `AdkStoreError::Validation`，缺失 id 返回 `Ok(false)`（幂等，等价 nil）。HTTP 契约一致：删除缺失会话 `404 ADK_SESSION_NOT_FOUND`（对齐 Go runtime 的 `session not found`），空白 id 在两侧都不是可路由请求。
- **`:641`（登记口径更正）**：`<execute-tool>` **不是 Go 生产语法**——`crates`/Go 生产代码全量搜索均无该字符串，只有 `internal/assistant/engine/test_helpers_test.go:testProviderExecuteToolCalls` 在测试假 provider 里把标签翻译成 tool call。真正被该测试覆盖的生产能力是 `skillsruntime.NormalizeToolAlias`（`"jftrade portfolio summary" → portfolio.summary`）＋参数/审批/结果链路，而 Rust **没有 alias 归一化**，模型必须精确返回目录 id。复现条件：模型返回 `jftrade.portfolio.summary`、`@portfolio.summary` 或 `portfolio summary`。预期行为：按 Go 规则（小写、去 `@`/`jftrade.` 前缀、空白与 `-`/`:`/`/` 转 `.`、折叠 `..`、修剪首尾 `.`）归一后解析到唯一目录工具；修复位置为 tool call 名称解析/持久化入口；回归测试需覆盖别名命中、未知工具、parameters JSON 非法、需要审批的别名调用。
- **`:792`（skill registry 资产层）**：本批已对齐 id/version/source/builtin/validationStatus 与两个策略 skill 的工具分层；**未对齐（P2 follow-up）**：Go 断言 `contentHash` 非空且 skills 目录下存在 `references/*.md`，Rust 不落盘内置 skill bundle（无 SKILL.md/资源/内容哈希）；Go 研究 skill 还允许 `backtest.cancel` 与 save/instantiate 系列工具，Rust 目录尚未注册这些工具（缺口归 strategy 领域）。

### 仍未结清（下一批）

- `internal/assistant/engine/input_request_test.go`（15 条）→ `mcp_server_test.go`（13）→ `adk_edges_test.go`（12）→ `workflow_tools_test.go`（11）；随后按 backlog 进入 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`internal/assistant/assembly`（102）等。
- 跨批 follow-up 汇总：P1 = Go `SaveRun` 终态谓词逐字对齐（`:146`/`:250` 引出的 CAS 缺口）、tool alias 归一化（`:641`）；P2 = 内置 skill bundle 落盘/内容哈希与缺失工具注册（`:792`）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**1691 passed**）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2725 Rust** / **877 `[x]`**，0 重复 `rust_entry`、0 非 `function_exact` 的 `[x]`）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（先按 target-health 提示执行 `pnpm run clean:rust:artifacts`）。

## 第三十九批：`input_request_test.go` 全量结清（输入续跑状态、并行提问冲突与 approval 过渡）

范围：`input_request_test.go` 16 条逐条结清——本批新增 13 条 `[x]`（`:584` 已在早前批次结清）、2 条带明确结论的 `[~]`（`:225`、`:530`）；同时修 4 处真实行为差异（输入续跑终态 `input_resolved`、输入续跑失败 `input_resume_failed`、同轮并行提问冲突、`allowOther` 缺省值）。`[x]` 877 → **890**，Rust 测试 2725 → **2738**（engine lib 测试 1268 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run '<16 条 input_request 测试名>' -count=1`：16 条全通过（1.179s）。

### 本批修复（4 处生产行为）

1. **输入续跑成功终态**：Go `completeInputContinuation` 写 `resumeState=input_resolved`；Rust 过去对所有续跑统一写 `adk_confirmation_resolved`。新增 `ChatExecution.resumed_from_input`（由 payload 的 `input_resuming`/`input_resume_pending` 判定），`persist_success` 与持久化 payload 两处投影按续跑类型分流；同时补齐 Go 的 `run.input_resolved` 审计行（输入续跑不再写 `run.resumed`）。
2. **输入续跑失败终态**：Go `failInputContinuation` 在 `markFailedChatRun` 之后写 `resumeState=input_resume_failed`；Rust 过去保留 `provider_executing`。`persist_failure` 对输入续跑改写为 `input_resume_failed`，保持 `FAILED` + 分类 errorCode + degraded + completedAt。
3. **同轮并行提问冲突**：Go `PendingInputRequests` 对同一 run 的两个阻塞提问返回 `errInputRequestConflict`（`simultaneous input requests are not supported for run <id>`），`CompleteChatRun` 投影为 200 + FAILED run。Rust 过去只停泊第一个、丢弃第二个 function call；现在 `persist_tool_calls` 检测到多个 `interaction.request_user` 调用即返回 `ADK_INPUT_REQUEST_CONFLICT`，`execute_chat` 按 Go 语义返回终态投影（FAILED/MODEL_CALL_FAILED/degraded，无 inputRequest、无 toolCalls）。
4. **`allowOther` 缺省值**：Go `buildInputRequest` 原样拷贝 `source.AllowOther`（缺省 false），Rust 过去 `unwrap_or(true)`，会让控制台对模型未允许的题目提供自由回答；改为 false，并在构建用例中断言。

### 新增回归（9 条 Rust 测试）

- `crates/jftrade-engine/src/product_adk_input_response_parity_tests.rs`（7 条）：`input_request_questions_publish_the_reference_ids_labels_and_defaults`（:16 构建半边）、`input_response_payload_anchors_the_resumed_run_to_the_original_request`（:773）、`simultaneous_input_request_calls_fail_the_run_instead_of_parking`（:305）、`an_unrecoverable_input_continuation_fails_the_run_with_the_reference_resume_state`（:413）、`an_answered_input_request_can_transition_into_an_approval_wait`（:694）、`sequential_questions_in_one_run_keep_both_answered_requests`（:647）、`a_restarted_runtime_resumes_a_pending_input_run`（:738，补 `input_resolved` 断言）；既有 `input_answers_are_canonicalized_by_question_order_and_invalid_answers_are_rejected`（:16 答案半边）、`cancelling_a_pending_input_run_cancels_the_request_and_rejects_a_late_answer`（:490）、`responding_to_a_missing_or_corrupt_input_run_reports_the_store_error`（:225）、`a_resumed_input_run_replays_the_original_request_anchor_to_the_provider`（:802）同批登记。
- `crates/jftrade-engine/src/product_adk_model_runtime_chat_turn_tests.rs`：`an_invalid_request_user_call_returns_correctable_feedback_before_parking`（:72）；`crates/jftrade-engine/src/product_adk_input_request_parity_tests.rs`：`request_user_arguments_accept_valid_calls_and_report_the_reference_errors`（:114、:72 校验半边）、`request_user_tool_declaration_publishes_the_two_or_three_option_budget`（:556）；`:449` 复用既有 `product_production_ports_adk_tests.rs::adk_respond_to_input_strict_validation_idempotency_and_conflict`。

### 探针（改坏 → 转红 → 回滚）

1. `persist_tool_calls` 的并行提问检测短路 → `simultaneous_input_request_calls_fail_the_run_instead_of_parking` 转红（run 停泊在第一个问题）。
2. `resumed_from_input` 判定恒 false → 续跑恢复用例转红（终态回落到 `adk_confirmation_resolved`/`provider_executing`）。
3. `allow_other` 回退 `unwrap_or(true)` → 构建用例转红（`questions[0].allowOther` 期望 false）。

### 结论登记（`[~]` 行与 `[x]` 行内的边界）

- **`:225`（store 错误面）**：缺失 run 的 404 与损坏 payload 已锁定；Go 的 `nil store`、空 id `errInputRequestInvalid`、关闭库与 SQLite trigger 注入在 Rust 无等价入口（端口先做必填校验，存储错误统一 500），保留为 Go 内部边界。
- **`:305`（事件对账分支）**：`nil execution`、`missing session`、`filters irrelevant and invalid events`、`existing pending request` 依赖 Go 的「事件重放 → PendingInputRequests 对账」架构；Rust 在落盘时直接判定并已用 correctable feedback 拦截非法调用，不再重建该清单，登记为架构差异。
- **`:530`（timeline primitive）**：Go 断言 `TimelinePrimitivesForRunActivity` + `GroupTimelinePrimitives` 的排序/合并行为；Rust 没有 primitive 投影层，输入历史由 run payload 的 `inputRequests` 承载（`session_timeline` 只投影会话事件并过滤 `assistant.tool`/`assistant.stream`）。若控制台后续要渲染 input_request 条目，需先定义 Rust 侧 primitive 语义再补测。

### 仍未结清（下一批）

- `internal/assistant/engine/mcp_server_test.go`（13 条）→ `adk_edges_test.go`（12）→ `workflow_tools_test.go`（11）；随后按 backlog 进入 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`internal/assistant/assembly`（102）等。
- 跨批 follow-up 汇总：P1 = Go `SaveRun` 终态谓词逐字对齐（`:146`/`:250` 引出的 CAS 缺口）、tool alias 归一化（`:641`）、审批续跑失败 `resumeState=approval_continuation_failed`（Go `markApprovalContinuationFailed`，Rust 目前仍是 `approval_resuming`）；P2 = 内置 skill bundle 落盘/内容哈希与缺失工具注册（`:792`）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2738 Rust** / **890 `[x]`**，0 重复 `rust_entry`、0 非 `function_exact` 的 `[x]`）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十批：`mcp_server_test.go` 全量结清（engine 13 条 + assembly 7 条；runtime status 脱敏落地）

范围：`internal/assistant/engine/mcp_server_test.go`（13 条）与 `internal/assistant/assembly/mcp_server_test.go`（7 条）逐条结清——本批 20 条全部落为 `[x]`，其中 6 条为带明确结论的边界（Go registry/通知面在 Rust 不存在）。`[x]` 890 → **910**，Rust 测试 2738 → **2751**（engine+store nextest 1717 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ ./internal/assistant/assembly/ -run '<20 条 mcp_server 测试名>' -count=1`：20 条全通过（engine 13 条 + assembly 7 条）。

### 本批修复（2 处生产缺口）

1. **runtime status 资源缺失**：Go `sanitizedMCPRuntimeStatus`（providers/agents/skills 摘要 + `storeConfigured` + `snapshotError`）在 Rust 完全没有实现，`resources/read` 只回空对象。新增 `McpRequestContext.status_store: Option<Arc<AdkStore>>`（`new`/`from_production_ports` 注入，`with_executor` 为 None）、`runtime_status_value`、`sanitized_tool_descriptors`、`sanitized_provider/agent/skill`、`normalize_tool_access_mode`；store 读取失败统一投影 `snapshotError="runtime snapshot unavailable"`，无 store 时 `providers/agents/skills` 显式空数组（对齐 Go 的“未配置”与“未知”区分）。
2. **`safe_capabilities` 可见性**：脱敏 provider 摘要需要复用 capabilities 过滤，`product_mcp_protocol.rs` 的 `safe_capabilities` 由私有提升为 `pub(crate)`。

### 新增回归（13 条 Rust 测试）

- `crates/jftrade-engine/src/product_mcp_server_tests.rs`（13 条）：`tool_failures_are_returned_as_mcp_tool_errors`（engine :162）、`runtime_status_resource_reports_store_and_reviewed_tools`（:293）、`runtime_status_resource_includes_configured_providers_agents_and_skills`（:333）、`runtime_status_resource_serializes_sanitized_descriptors`（:358）、`runtime_status_subscription_validation_rejects_unknown_uris`（:367）、`rotating_the_token_rejects_the_previous_secret`（assembly :22）、`mcp_tools_list_exposes_only_reviewed_read_tools`（engine :17）、`write_capable_names_are_never_reachable_through_the_reviewed_allowlist`（:119）、`stateless_post_only_requests_never_issue_a_session_header`（:194）、`tool_calls_resolve_the_current_executor_on_every_request`（:440）、`runtime_status_resource_reflects_live_dependency_availability`（:382）、`replacement_disable_and_close_release_each_listener_owner`（assembly :203）、`cold_start_listener_failure_records_the_reason_and_recovery_clears_it`（assembly :257）。
- 复用并强化证据：`disabled_runtime_has_stopped_status_and_releases_listener`（assembly :76，补 running + `endpoint=http://127.0.0.1:<port>/mcp`）、`port_conflict_keeps_previous_listener_and_reset_rebinds`（assembly :181，补 `last_error` 断言）、`shutdown_is_idempotent_and_closed_runtime_rejects_rebind`（engine :136）、`reviewed_mcp_catalog_reports_native_and_fail_closed_counts`（:129）、`host_rebinding_and_missing_host_are_rejected`（:266）、`loopback_policy_rejects_non_loopback_peer_addresses`（assembly :280）、`token_auth_and_tools_list_use_reviewed_catalog`（assembly :106）。
- 辅助断言：`advertised_tool_names`、`advertised_availability` 两个只读 helper；`with_executor` 标注为 `#[cfg(test)]`（生产仅用 `from_production_ports`）后 clippy 无 dead_code 告警。

### 探针（改坏 → 转红 → 回滚）

1. `sanitized_provider` 改为直接回传原始 `capabilities` → `runtime_status_resource_serializes_sanitized_descriptors` 转红（输出泄漏 `apiKey: sk-leak`），回滚后恢复。
2. `runtime_status_value` 的无 store 分支去掉 `providers/agents/skills` 空数组 → `runtime_status_resource_reports_store_and_reviewed_tools` 转红（`left: Null, right: []`），回滚后恢复。
3. `call_tool` 去掉 `REVIEWED_READ_ONLY_TOOLS` 允许列表闸门（改为只拦 `\0` 前缀）→ `write_capable_names_are_never_reachable_through_the_reviewed_allowlist` 转红（错误文案从 `unknown tool "..."` 变为 `tool "..." is unavailable in the Rust MCP runtime`），回滚后恢复；该用例因此锁定“拒绝来自 reviewed 白名单而非能力图”。

### 文档修正（`docs/adk.md`）

旧文档把 Go 的行为写成了 Rust 现状：一是把 MCP 服务描述为“使用 go-sdk 提供 transport”，二是声称“运行时工具目录变化会发送 `tools/list_changed` 和 `resources/updated` 通知”。后者在 Rust 监听器里没有实现面：`rg -n 'list_changed|resources/updated' crates/` 全仓 0 命中，`notifications/*` 仅接受 `notifications/initialized` 与 `notifications/cancelled` 且不回推。因此改为：transport 是“与 go-sdk v1.7.0 线协议兼容的 Rust 实现”，资源内容按 `sanitizedMCPRuntimeStatus` 逐字段列出，并明确写出「无主动通知、需重新 `tools/list` 或重读资源」以及按请求投影 `ready`/`fail-closed` 的事实。

### 结论登记（`[x]` 行内的边界）

- **engine `:382`（registry 变更同步）**：Go 断言 registry 变更后客户端收到 `tools/list_changed` 与 `resources/updated`；Rust 无 registry 变更/主动 SSE 通知面。等价证据是工具面每次请求从运行期能力图重投影（绑定 SystemRead → `ready`，移除 adapter → `fail-closed` 且 `tools/call` 得 `-32602`）。通知通道本身不迁移。
- **engine `:440`（替换 handler 刷新）**：Go 重注册同名工具后下一次 call 走新 handler；Rust 无可替换 registry，等价不变式为每次调用重新解析当前 executor（计数 executor 连续两次返回 `version=1`、`2`）。
- **engine `:119`（write-capable 替换被拒）**：Go 在 `NewLocalMCPHandler` 构造期报错；Rust 无构造期注册面，改为断言 reviewed 白名单全部 `read_*` + 写工具不可达。
- **engine `:129`（至少一个 reviewed 工具）**：Go 对空 registry 报错；Rust 的 reviewed 名单是 69 项常量，不存在空目录状态，登记为结构不变式。
- **engine `:136`（Close 注销 registry listener）**：Rust 无 registry listener；等价契约为关闭幂等 + 关闭后拒绝重绑 + 端口释放。
- **assembly `:257`（意外退出释放 handler）**：Rust 的意外退出分支存在（worker guard 写 `MCP listener stopped unexpectedly` 并翻转 running），但 Go 依赖可注入的 `manager.listen` seam；以冷启动绑定失败路径 + 恢复清空 `last_error` 作为可达证据，serve 异常注入登记为边界。
- 通用差异：Go 测试通过 `httptest` + MCP SDK 客户端驱动，Rust 用原始 socket 请求（2026-07-28 协议要求 `Mcp-Protocol-Version`/`Mcp-Method`，且 `resources/read` 必须携带匹配的 `Mcp-Name`），`-32602` 在 Rust wire 上映射为 HTTP 400——与既有 `modern_unknown_tool_is_json_rpc_invalid_params_not_http_success` 一致。

### 仍未结清（下一批）

- `internal/assistant/engine/adk_edges_test.go`（12 条）→ `workflow_tools_test.go`（11 条）；随后按 backlog 进入 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`internal/assistant/assembly`（余量）等。
- 跨批 follow-up 汇总：P1 = Go `SaveRun` 终态谓词逐字对齐（`:146`/`:250` 引出的 CAS 缺口）、tool alias 归一化（`:641`）、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 内置 skill bundle 落盘/内容哈希与缺失工具注册（`:792`）、MCP registry 变更通知通道（engine `:382`/`:440` 提出的 SSE 通知面，若控制台需要动态刷新再单独立项）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1717 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2751 Rust** / **910 `[x]`**，0 重复 `rust_entry`、0 非 `function_exact` 的 `[x]`）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十一批：`adk_edges_test.go` 全量结清（12 条边界用例；context 覆盖回落修复）

范围：`internal/assistant/engine/adk_edges_test.go` 12 条逐条结清——全部落为 `[x]`，其中 8 条带明确结构边界。`[x]` 910 → **922**，Rust 测试 2751 → **2763**（engine+store nextest 1729 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run '<12 条 adk_edges 测试名>' -count=1`：12 条全通过（2.528s）。

### 本批修复（1 处生产差异）

1. **session context 的 composer provider 覆盖回落**：Go `Runtime.resolveSessionContextAgent` 在 override 解析失败且 base agent 有 provider 时回落 base；Rust `session_context_agent_payload`（`crates/jftrade-engine/src/product_production_ports_adk_context_window.rs`）过去无条件把 `providerIdOverride` 写进 agent，导致不存在的 provider 让 `contextWindowTokens` 退化为 0、`status=unknown`。现在按 Go 语义回落：override 生效且 override provider 不可读、base provider 可读时返回 base agent 投影。

### 新增回归（12 条 Rust 测试）

- `product_adk_model_runtime_gate_tests.rs`：`agent_memory_prompt_scopes_workspace_rows_and_fails_closed`（:25）、`chat_resolution_reports_missing_agents_and_unusable_providers`（:532）。
- `product_mcp_server_tests.rs`：`models_list_tool_projects_providers_with_reference_boolean_rules`（:61）。
- `product_production_ports_adk_notices.rs`（新增 `#[cfg(test)] mod tests`）：`context_notices_are_best_effort_and_keep_one_identity_across_updates`（:105）。
- `product_adk_store_parity_tests.rs`：`goal_pause_and_resume_mutations_own_the_pause_lifecycle_fields`（:153）、`snapshot_and_provider_test_boundaries_fail_closed`（:684）。
- `product_production_ports_adk_tests.rs`：`tool_declarations_stay_complete_for_every_callable_tool`（:203）。
- `product_adk_run_timeout.rs` / `product_adk_session_compaction_gate.rs`（均新增 `#[cfg(test)] mod tests`）：`assistant_run_timeout_falls_back_to_the_reference_default_window`（:294）、`compaction_gate_serializes_one_compactor_per_session`（:351）。
- `product_adk_session_context_tests.rs`：`session_context_overrides_fall_back_to_the_agent_provider_window`（:416，本批修复的守卫）。
- `crates/jftrade-store-sqlite/tests/adk_store_contracts.rs`：`adk_transaction_boundaries_commit_roll_back_and_report_missing_tables`（:209）。
- `crates/jftrade-store-sqlite/tests/maintenance_cleanup_candidates.rs`（新增文件）：`soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute`（:237）。

### 探针（改坏 → 转红 → 回滚）

1. 去掉 context window 的 base 回落 → `session_context_overrides_fall_back_to_the_agent_provider_window` 转红（窗口 0/unknown）。
2. `update_context_compaction_notice` 去掉空 id 守卫 → `context_notices_are_best_effort_and_keep_one_identity_across_updates` 转红（空 id 会新增第二行）。
3. `verify_execute` 去掉 fingerprint 比对 → `soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute` 转红（`CandidatesChanged` 不再返回）。
4. `optional_bool` 未知字符串回退改为 false → `models_list_tool_projects_providers_with_reference_boolean_rules` 转红（`callableOnly` 默认被破坏）。

### 结论登记（`[x]` 行内的边界）

- **:25**：无 Google ADK memory service，也无按 query token 匹配的 `googleADKMemoryMatches`；以 workspace/agent 私有行投影 + 表缺失报错作为可达契约。
- **:153**：无 `preserveUserGoalPauseLifecycle` 纯函数；pause 生命周期由 `PauseRun`/`ResumeRun` + revision CAS 持有，`different run`/`non-loop candidate`/`terminal candidate` 无对应输入面。
- **:203**：Rust schema 是 `serde_json::Value`，Go 的 “func 值 encode 失败” 分支不可达；以“每个 callable 工具都有完整声明”作为契约。
- **:209**：无 gorm pool 表层；以事务原子性（run+event 同提交/失败回滚）与表缺失报错替代。
- **:237**：无 `(*Store)(nil)` 接收者；以软删除候选集 + `CandidatesChanged` + `Busy("active run")` 覆盖。
- **:294**：无 nil runtime/`SetRuntimeLimitsProvider`；以 settings 缺省/零/负/坏 JSON 回退 1800000 与配置值胜出覆盖。
- **:351**：artifact service 回退（非法 session DB → in-memory）与 `Runtime.Close()` 取消 active run 属 Go 构造面；Rust store 在组合根一次构造、失败即拒绝启动。
- **:532**：session 标题 28 字符、技能缺失、关闭 store/表缺失分支由既有用例与 store 契约用例覆盖；本批锁定 agent/provider 解析错误分类。
- **:684**：skill registry 注入与“坏 chat provider”网络失败属 Go 注入面；以 404/503 分类与 snapshot fail-closed 覆盖。

### 仍未结清（下一批）

- `internal/assistant/engine/workflow_tools_test.go`（11 条）；随后按 backlog 进入 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`internal/assistant/assembly`（余量）等。
- 跨批 follow-up 汇总：P1 = Go `SaveRun` 终态谓词逐字对齐、tool alias 归一化、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 内置 skill bundle 落盘/内容哈希与缺失工具注册、MCP registry 变更通知通道、chat 路径的 composer provider/model 覆盖（本批仅修了 session context 读取路径，chat 执行路径不读 composer 覆盖，需要 Go 侧用例确认后再立项）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1729 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2763 Rust** / **922 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十二批：`workflow_tools_test.go` 结构化工具失败落库修复（11 条；2 条 `[x]`）

范围：`internal/assistant/engine/workflow_tools_test.go` 11 条逐条结清——2 条落为 `[x]`（`:230`、`:263`），5 条 `partial`（`:28`、`:140`、`:285`、`:559` 等），4 条 `boundary`（`:206`、`:222`、`:438`、`:474`、`:488`）。`[x]` 922 → **924**，Rust 测试 2763 → **2768**（engine+store nextest 1734 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/engine/ -run '<11 条 workflow_tools 测试名>' -count=1 -v`：11 条全通过（0.968s，checkout `/tmp/go452dea11.niwD1G`）。

### 本批修复（1 处生产差异）

1. **结构化工具失败被记为 SUCCEEDED**：Go 的 `googleADKTool.executeAndMap` 把 handler 返回的 `{"success":false}`（新契约）或 `{"error":"..."}`（旧契约）重新投影为 `structuredToolErrorEnvelope`，`consumeFunctionResponse` 据此把 `ToolCall` 记为 `FAILED`/`TIMED_OUT`/`CANCELLED`，run 继续本轮并在终态以 `degraded` 暴露；`{"result": …}` 包装裸标量。Rust 过去只要 executor 返回 `Ok(value)` 就落 `SUCCEEDED`，结构化失败对控制台、`FirstToolCallFailure`/`degraded` 与 metrics 全部不可见，裸标量也不包装。

   修复落在新片段 `crates/jftrade-engine/src/product_adk_model_runtime_tool_result.rs`（`map_tool_output` / `tool_error_envelope` / `structured_tool_failure_message` / `structured_tool_failure_metadata` / `classify_tool_error_text` / `prefixed_tool_error`），由 `product_adk_model_runtime.rs` 以模块级 `include!` 引入；`product_adk_model_runtime_tool_loop.rs` 的 `Ok(output)` 分支改为投影后按 `(status, error_text)` 落库，`persist_tool_result` 增加 `error_text` 覆写参数（`ToolCall.Error` 记 Go 的模型侧文本）。切片保持在 800 行以内的既有约定下：tool loop 761 → 764 行、`product_adk_model_runtime.rs` 770 行、新片段 162 行。

   刻意差异（已在清单行内登记）：嵌套 `error` 缺 `code` 时 Rust 用 `TOOL_EXECUTION_FAILED` 默认值，不复制 Go `<nil>` 的 `fmt.Sprint` 产物。

### 新增回归（5 条 Rust 测试）

- `product_adk_model_runtime_tool_failure_tests.rs`：`structured_tool_response_failures_match_the_reference_helpers`（`:230`，`structuredToolError`/`isToolResponseError`/`toolResponseErrorMessage` 矩阵：空 map、success:true、success:false 缺 message、trim、旧式 error、`"<nil>"`/空串/null、裸标量包装）。
- `product_adk_model_runtime_tool_failure_tests.rs`：`tool_error_envelopes_classify_timeout_cancellation_and_structured_metadata`（`:263`，TIMEOUT/retryable、嵌套镜像、结构化 RATE_LIMITED 保留、CANCELLED/非 retryable、缺 code 回退）。
- `product_adk_model_runtime_tool_failure_tests.rs`：`a_structured_tool_failure_is_persisted_as_a_failed_call_and_the_loop_continues`（`:28`/`:488` 侧，端到端 `run_tool_loop`：FAILED 调用 + 后续 SUCCEEDED 兄弟调用 + `{"result":"AAPL"}` 包装 + `FirstToolCallFailure`）。
- `product_mcp_server_tests.rs`：`strict_tool_schemas_reject_invalid_arguments_before_the_executor_runs`（`:140`，合法调用命中 executor 一次；invalid type／missing required／additional property 全部 `-32602` 且 executor 计数保持 1）。
- `crates/jftrade-store-sqlite/tests/adk_store_contracts.rs`：`workflow_trigger_soft_delete_and_log_lookup_keep_the_reference_boundaries`（`:559`，软删除 DISABLED+deletedAt、revision fence 二次删除 false、due/enabled 列表剔除、trigger log Some/None 与日志留存）。

### 探针（改坏 → 转红 → 回滚）

1. `map_tool_output` 的失败检测短路为恒 `SUCCEEDED` → 三条 tool-failure 新用例全部转红（实测 `toolResults[0].status` 回到 `SUCCEEDED`，`calls[0].status` 断言失败）。
2. 删除 `product_mcp_server_dispatch.rs::call_tool` 的 `validate_tool_arguments` 调用 → `strict_tool_schemas_reject_invalid_arguments_before_the_executor_runs` 转红（被拒参数返回 200 并进入 executor）。
3. `soft_delete_workflow_trigger_if_revision` 的 SQL `status='DISABLED'` 改回 `'ENABLED'` → store 新用例转红（`deleted.status` ENABLED != DISABLED）。

### 结论登记（partial / boundary 行内的边界）

- **:28 partial**：Rust 无 ADK `ProcessRequest` 的声明追加／`duplicate tool` 拒绝（probe：`rg "ProcessRequest" crates/ apps/ workers/` 无命中，声明每次由 catalog 生成）；无 `unexpected args type`（参数是 `serde_json::Value`，非 object 在 MCP 边界以 `-32602` 拒绝）；`ErrConfirmationRequired` 原样传播改为 `requiresUser`/`PENDING_APPROVAL` staging。
- **:140 partial**：Go 的 `execution.descriptorForTool` 无 descriptor registry seam；无 InputSchema 的工具 Go 放行任意参数，Rust 对未评审工具回退 generic object schema + `additionalProperties:false`（刻意 fail-closed）。
- **:206 / :222 boundary**：`serde_json::Value` 无法表示 `make(chan int)`，`"convert GO-ADK product tool schema"` 无命中；也没有“按名字选集返回 nil toolset”的 seam，Rust 由 catalog 绑定 + `allowedModes` 决定声明集合。
- **:285 partial**：Rust 只有 skill allowed-tools 投影（`adk_builtin_strategy_skills_publish_the_curated_tool_split`）；`agentFilteredSkillSource` / `skillAllowedForAgent` / `ErrSkillNotFound` 与 `"skill not found"` 均无命中，per-agent 技能授权过滤未实现。
- **:438 / :488 boundary（功能差异）**：Rust 有 artifact 持久化（`AdkArtifactStore`）但没有 artifact toolset，`load_artifacts` 无命中；`ToolAccessModeNone` 的无工具面语义由 `AgentToolScope::None` 覆盖。
- **:474 boundary（功能差异）**：`preload_memory`/`load_memory` 无命中；Rust 在 `memoryEnabled` 时把 durable memory 预注入 instruction（`agent_memory_prompt` + `JFTrade memory:` 块，见 `chat_injects_memory_into_the_instruction_only_when_enabled`），模型可调用的 memory 工具未实现。
- **:559 partial**：Rust 无 `ListActiveWorkflowTriggerLogs` 与 `ListWorkflowTriggerLogsPage`（status 过滤 + 分页）同形 store API，只有全量 `list_workflow_trigger_logs` + read port `page(...)`；id/type/status 归一由 mutation port 的 `required_identifier`/`normalize_trigger_type`/`normalize_trigger_status` 承担，未在同一用例断言。

### 仍未结清（下一批）

- `internal/assistant/assembly/workflow_tools_test.go`（7 条：`:15` catalog/approval matrix、`:68` bounded wait envelope、`:86` deadline/cancel、`:132` patch 语义、`:183` list/create/delete、`:240` interactive session、`:285` unavailable manager fail-closed）。
- 跨批 follow-up 汇总：P1 = Go `SaveRun` 终态谓词逐字对齐、tool alias 归一化、审批续跑失败 `resumeState=approval_continuation_failed`、工作流触发日志的 active/page 过滤（本批 `:559` 引出）；P2 = 内置 skill bundle 落盘/内容哈希与 per-agent 技能授权过滤（本批 `:285` 引出）、模型侧 memory/artifact 直接工具（本批 `:474`/`:488` 引出）、MCP registry 变更通知通道。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1734 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2768 Rust** / **924 `[x]`**，0 破坏引用、0 重复 `rust_entry`）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十三批：assembly `workflow_tools_test.go` 全量结清（7 条；工具层 vs REST 表面边界）

范围：`internal/assistant/assembly/workflow_tools_test.go` 7 条逐条结清——本批 0 条新 `[x]`：5 条 `partial`（`:15`、`:86`、`:132`、`:183`、`:285`），2 条 `boundary`（`:68`、`:240`）。`[x]` 保持 **924**，Rust 测试 2768 → **2772**（engine+store nextest 1738 passed）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run '<7 条 workflow_tools 测试名>' -count=1 -v`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（无生产改动；4 条新增回归）

Go 这 7 条用例几乎全部作用于 `RegisterWorkflowManagementTools` 注册的 15 个 `workflows.*` / `workflow_triggers.*` / `workflow_runs.*` 模型工具。Rust **没有**该工具注册表（probe：`rg "RegisterWorkflowManagementTools|workflows.list|workflow_runs.list" crates/ apps/ workers/` 无命中）：工作流管理在 Rust 由控制台走 REST `/api/v1/adk/workflows*`，模型侧只保留 `workflow.wait`。因此本批把「可复现的 REST 契约」补成测试，「工具层专有」的部分登记为边界。

新增回归（`crates/jftrade-engine/src/product_adk_store_parity_tests.rs`，4 条）：

- `workflow_management_catalog_keeps_the_skill_and_approval_boundaries`（`:15`）：`jftrade-workflow-management` builtin skill 投影 workflow/interaction 分类工具；`workflow.wait` 在三种 permission mode 下都不需要审批；catalog 之外的名字 `requires_approval` fail-closed；console-only 的 `workflows.*`/`workflow_runs.wait` 名字从不进入模型 catalog。
- `workflow_updates_keep_omitted_fields_and_apply_explicit_clears`（`:132`）：省略字段保值（name/promptTemplate/objectiveTemplate/defaultInputs/agentId）、显式清空生效（`description:""`、`tags:[]`、`canvasGraph:null`，读投影一致）、webhook 触发器更新保留 type、替换 title、清空 config、未带 `resetSecret` 时不返回 secret 且 `hasSecret` 为 true。
- `workflow_and_trigger_lists_hide_deleted_rows_after_create_and_delete`（`:183`）：创建后可列出、删除返回 `{"deleted": true}` 并从列表消失、触发器日志在定义删除后仍可读（列表 + store `get_workflow_trigger_log`）。
- `workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation`（`:285`）：缺少 assistant model runtime 时运行被拒 503 `ADK_WORKFLOW_RUNTIME_UNAVAILABLE`，已认领 invocation 持久化为 `FAILED` 并保留 errorCode/error 原文（不产生假排队）。

### 探针（改坏 → 转红 → 回滚）

1. 从 patch 键列表移除 `"canvasGraph"` → `workflow_updates_keep_omitted_fields_and_apply_explicit_clears` 转红（创建与更新都丢 graph）。
2. 去掉 workflow 列表投影的 deleted 过滤（`product_production_ports_adk_read.rs`）→ `workflow_and_trigger_lists_hide_deleted_rows_after_create_and_delete` 转红（删除后仍在列）。
3. 把 `finalize_workflow_failure` 的落库状态 `FAILED` 改成 `QUEUED` → `workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation` 转红。
4. `requires_approval` 的 catalog-miss 默认值 `true` → `false` → `workflow_management_catalog_keeps_the_skill_and_approval_boundaries` 转红。
5. workflow-management skill 的 `categories` 去掉 `"workflow"` → 同一用例转红（tools 只剩 `interaction.request_user`）。

### 结论登记（partial / boundary 行内的边界）

- **:15 partial**：15 个 workflow 工具的 descriptor category/InputSchema/required-skills 断言无对应注册表；可达半边是 builtin skill 投影 + `workflow.wait` 审批决策 + catalog fail-closed。
- **:68 boundary**：`workflow_runs.wait` 轮询工具不存在（`nextPollMs` 无实现；probe 无命中），等待由前端 composable `adkRunContinuation.ts` 轮询承担；Rust 的 `workflow.wait` 是 Go `workflowWaitTool` 的对应物（`{waitedMs, reason}`）。
- **:86 partial**：覆盖 25s cap 拒绝与取消返回 499（另有 tool-loop deadline/取消），无 `pollIntervalMs`/轮询输入面。
- **:132 partial**：Go 工具层禁止用工具创建 webhook 触发器、禁止改变 webhook 的 type（错误含 `UI/API`，`workflow_tools.go:79/:384` 与 schema enum 排除 webhook）；Rust REST 就是该错误指向的 UI/API 表面，故允许，属结构差异。Rust 更新 schedule 触发器时会校验新 config（清空被拒），Go 用例用 webhook 表达空 config。
- **:183 partial**：`workflow_runs.list` 的 workflowId/triggerId/status 过滤与 `WorkflowToolPage` 形状在 Rust 列表投影里没有对应参数（`workflow_logs` 只做 `page()`）。
- **:240 boundary**：无 session 来源门禁（无 `cannot start`/`resolvable` 文案，ADK sessions 无 source 列）；可达相邻契约是 disabled → 409、缺资源 → 路由 code 404、runtime 缺失 → 503。
- **:285 partial**：无 `unavailableWorkflowToolManager` 的 12 方法接口；REST `page()` 只保证默认 limit=100，对显式 limit 不做上限夹取（Go 工具层 clamp 500→100 属另一表面，为避免改动公开 HTTP 契约故不改）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量：`adk_workflow_tools`（若存在）之外按 backlog 继续，随后 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 工作流触发日志的 active/page 过滤（第 42 批 `:559`）、`workflow_runs.*` 过滤参数（本批 `:183`）、Go `SaveRun` 终态谓词逐字对齐、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = per-agent 技能授权过滤（第 42 批 `:285`）、模型侧 memory/artifact 直接工具（`:474`/`:488`）、ADK read `page()` 是否引入显式 limit 上限（需先确认公开 HTTP 契约意愿）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（1738 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2772 Rust** / **924 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十四批：assembly `application_adapter_test.go` 全量结清（11 条；6 条新 `[x]`，4 条 `partial`，1 条 `boundary`）

范围：`internal/assistant/assembly/application_adapter_test.go` 11 条逐条结清——本批新增 **6 条 `[x]`**（`:16`、`:58`、`:81`、`:123`、`:138`、`:226`；`:164` 已在早前结清），`partial` 4 条（`:185`、`:278`、`:308` 与既有 `[~]` 行保持一致），`boundary` 1 条（`:253`）。`[x]` 计数 924 → **930**，Rust 测试 2772 → **2781**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestApplicationAdapter|TestApplicationWorkflowSnapshotRejectsInvalidInstrument' -count=1 -v`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（2 处生产修复 + 9 条新增回归）

Go 这 11 条用例作用于 Wails 助手适配器 `ApplicationAdapter`：把领域服务（trading/strategy/backtest/marketdata/system）绑成工具依赖。Rust 的等价 owner 是 **生产 MCP 执行器 + 生产端口 bundle**（`ProductionMcpToolExecutor` / `ProductionPortBundle` / `BacktestReadSnapshotPort`），因此本批按「路由 → 端口 → wire 错误」逐段建立证据。

生产修复：

1. `crates/jftrade-engine/src/product_mcp_production_executor_market_data.rs`：MCP `market.candles` 的 adjustment 现在与 Go 的 `MarketCandlesAdvanced` 一致先 trim + 小写再转发（此前原样透传、靠路由侧 `parse_candle_adjustment` 兜底）。
2. `crates/jftrade-engine/src/product_mcp_production_executor_helpers.rs` + `product_mcp_production_executor.rs`：修复 `backtest.runs` 过滤 —— 生产读端口把策略请求放在 `request.*` 下（`request.definitionId` / `request.definitionVersion`），而旧的 `matches_filter` 只比较顶层键，导致按 definition 过滤永远返回空。新 `matches_run_filter`/`filter_backtest_runs` 同时解析顶层与 `request.*`，`status`/`marketDataProvider` 保持顶层语义，行为对齐 Go 的扁平摘要过滤。

新增回归：

- `crates/jftrade-engine/src/product_mcp_server_tests.rs::production_tools_fail_closed_before_any_domain_service_is_configured`（`:16`）：只注入 catalog/store（等价 `ApplicationPorts{}`）时，`execution.order_events`、`broker.orders`、`strategy.definitions`、`backtest.runs`、`backtest.kline_sync_status`、`research.screen_catalog`、`market.providers`、`system.runtime_dependencies` 全部 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`。
- `crates/jftrade-engine/src/product_execution_read_tests.rs::execution_read_routes_preserve_the_port_failure_code_and_message`（`:58`）：端口 `Failed{code,message}` 的 code/message 原样进入 wire error（500），不被折叠。
- `crates/jftrade-engine/src/product_production_ports_trade_tests.rs::order_scope_defaults_to_current_and_status_lists_merge_case_insensitively`（`:81`）：空/`scope=`/` cUrReNt ` → CURRENT、` history ` → HISTORY、status+statuses 合并去重 → `[5,10]`。
- `crates/jftrade-engine/src/product_mcp_production_executor_market_data.rs::tests::market_candle_instrument_ids_split_once_and_require_market_and_symbol`（`:81`）：` us.brk.b ` → US/BRK.B（单次 split 保点号），`US.`/`AAPL`/` .AAPL` 在 provider 之前 400。
- 同文件 `market_candles_compact_tool_forwards_market_symbol_period_and_limit`（`:123`）与 `market_candles_advanced_tool_normalizes_session_and_adjustment_labels`（`:138`）：period/limit/游标/sessions/adjustment 的转发与归一化。
- `crates/jftrade-research/tests/catalog_parameter_availability_boundaries.rs::screen_catalog_normalizes_padded_lowercase_markets_and_rejects_unsupported_labels`（`:226`）：` futu `/` us ` 归一化命中 `futu|US` 目录，` cn ` 对 futu 拒绝。
- `crates/jftrade-engine/src/product_backtest_sync_start_tests.rs::production_backtest_cancel_reports_false_without_a_cancellable_run`（`:226`）：缺失/终态运行 `cancelled=false` 且状态不改写，queued 运行 `cancelled=true` 并落库 `status=cancelled`。
- `crates/jftrade-engine/src/product_mcp_production_executor_tests.rs::backtest_runs_filters_resolve_nested_request_fields_and_top_level_status`（`:278` 的记录半边）：嵌套 `request.definitionId`/`definitionVersion` 与顶层 status/provider 过滤、limit 截断与 `truncated` 标记（第 45 批把 MCP executor 单元测试拆到 `product_mcp_production_executor_tests.rs`，以满足 800 行生产文件预算）。

### 探针（改坏 → 转红 → 回滚）

1. MCP period 输出改大写 → `market_candles_compact_tool_forwards_market_symbol_period_and_limit` 转红。
2. 去掉 adjustment 归一化 → `market_candles_advanced_tool_normalizes_session_and_adjustment_labels` 转红。
3. `instrument()` 的 `split_once` 改 `rsplit_once` → `market_candle_instrument_ids_split_once_and_require_market_and_symbol` 转红。
4. `screen_catalog` 去掉 market trim/upper → `screen_catalog_normalizes_padded_lowercase_markets_and_rejects_unsupported_labels` 转红。
5. `matches_run_filter` 去掉 `request.*` 回退 → `backtest_runs_filters_resolve_nested_request_fields_and_top_level_status` 转红。
6. `execution_read_snapshot_failure` 的 Failed 分支改成固定 `EXECUTION_FAILED` → `execution_read_routes_preserve_the_port_failure_code_and_message` 转红。
7. `history_scope` 把空 scope 默认改为 HISTORY → `order_scope_defaults_to_current_and_status_lists_merge_case_insensitively` 转红。
8. `cancel_backtest` 对缺失运行返回 true → `production_backtest_cancel_reports_false_without_a_cancellable_run` 转红。
9. `ports()` 错误码改成 `MCP_PRODUCTION_EXECUTOR_MISSING` → `production_tools_fail_closed_before_any_domain_service_is_configured` 转红。

### 结论登记（partial / boundary 行内的边界）

- **:185 partial**：MarketProviders/RuntimeDependencies 可达半边由 `GET /api/v1/market-data/provider` 的 fail-closed 503 与 `GET /api/v1/system/runtime-dependencies` 的 `allRequiredSatisfied` + node 投影覆盖；Go 的 `openD` 合并键（`status=error` + error 文本）与 `SelectMarketProvider` 助手包装在 Rust 无对应物 —— OpenD 健康是独立路由/工具 `/api/v1/system/futu-opend`（失败投影 `status=unavailable`/`offline` + reason），provider 选择是设置写入路由 `PUT /api/v1/settings/market-data-provider`。
- **:253 boundary**：`strategyVisualModelFromInput`（engine 默认 `logic-flow`、version=1、edge 默认 `polyline`、拒绝字符串与 legacy `blockKind`）是 Wails 侧助手归一化；Rust 写入端口把 `visualModel` 原样存进 `visual_model_json`，等价默认值归 Vue/TS 可视化构建器（`apps/web/src/features/strategy-builder/*` 自带 vitest）。探针：`rg -n "logic-flow|strategyVisualModelFromInput|blockKind" crates/ workers/` 无命中（仅 apps/web 命中）。
- **:278 partial**：同步态投影由 `test_research_backtest_data_readiness_and_sync_lifecycle`（`status=syncing_data`、`dataSync.*`、`nextTool=backtest.kline_sync_status`）与 `production_sync_read_projects_persisted_task`（status/completedIntervals）覆盖；未对齐：Go 的 nil→零值 helper 与 `intervals=["1m"]` 字面量断言在 Rust 缺少同名 helper（P2 待补），`backtestRunSummaryFromService` 的扁平摘要（provider 默认 "futu"）无对应 DTO —— Rust 返回存储投影并靠本批修复的过滤器解析嵌套 request。
- **:308 partial**：Rust 的非法 instrument 拒绝 owner 是行情路由 `parse_market_symbol_path`（`live_read_routes_reject_malformed_instruments_before_any_provider_access` 断言 400 且不触达 provider）；Go 的 `WorkflowMarketSnapshot` 助手在 Rust 无对应物（探针：`rg -n "WorkflowMarketSnapshot|workflow_market_snapshot" crates/ apps/ workers/` 0 命中），workflow 轮询把无点号 id 当作 `US/<id>` 并在读取失败时跳过该轮事件。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量：`adk_strategy_test.go`（10 条）、`portfolio_tools_test.go`（8 条）、`tool_catalog_test.go`（7 条）、`product_adapters_test.go`（6 条）等，随后 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 工作流触发日志的 active/page 过滤（第 42 批 `:559`）、`workflow_runs.*` 过滤参数（第 43 批 `:183`）、Go `SaveRun` 终态谓词逐字对齐、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `backtest.kline_sync_status` 的 intervals 字面量投影断言（本批 `:278` 引出）、per-agent 技能授权过滤（第 42 批 `:285`）、模型侧 memory/artifact 直接工具（`:474`/`:488`）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1755 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2781 Rust** / **930 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:ai-context`、`pnpm run check:quick`。

## 第四十五批：assembly `adk_strategy_test.go` 全量结清（10 条；3 条新 `[x]`，6 条 `partial`）

范围：`internal/assistant/assembly/adk_strategy_test.go` 10 条逐条结清——本批新增 **3 条 `[x]`**（`:19`、`:308`、`:397`），`partial` 6 条（`:32`、`:77`、`:194`、`:449`、`:699`、`:774`），`:605` 保持既有 `[x]`。`[x]` 计数 930 → **933**，Rust 测试 2781 → **2786**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADK' -count=1 -v`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（2 处生产修复 + 5 条新增回归）

Go 这 10 条用例横跨 ADK 工具层（注册表 + ToolDeps）与领域服务：订阅错误面、任务/记忆 CRUD 与审计、策略定义版本快照、backtest.runs 过滤、provider 冻结与并发隔离。Rust 的等价 owner 是 `ProductionAdkToolExecutor` + `ProductionMcpToolExecutor` + 生产端口。

生产修复：

1. `crates/jftrade-engine/src/product_production_ports_strategy.rs`：`ProductionStrategyDefinitionPort::versions` 先按 `get_definition(id, include_deleted=true)` 判定存在性，未知 definitionId 现在返回 `None`（路由/MCP 404），与冻结 fixture（`versions-missing` → 404）和 Go 的 `found=false` 对齐；此前生产路径会回答 200 + 空列表（fixture replay 用 fixture port，掩盖了该差异）。软删除定义仍返回历史（`versions-soft-deleted` → 200）。
2. `crates/jftrade-engine/src/product_mcp_production_executor_helpers.rs`：`filter_backtest_runs` 在未过滤/空列表时也注入 `runCount`，恢复 Go legacy 两键形状（此前直接返回路由 payload，模型侧看不到计数）。

新增回归：

- `product_mcp_server_tests.rs::market_subscriptions_surface_quote_port_failures_without_fixture_payloads`（`:19`）：失败订阅端口 → 503 `MARKET_DATA_QUOTE_READ_UNAVAILABLE` + 原始 message。
- `product_mcp_server_tests.rs::backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets`（`:194`/`:308`）：result_view 缺 runId、kline_sync_status 缺 taskId、version list 缺 definitionId 均 400；未知 taskId → 404 `BACKTEST_SYNC_TASK_NOT_FOUND`；未知 definitionId → 404 `STRATEGY_DEFINITION_NOT_FOUND`。
- `product_production_ports_strategy_tests.rs::strategy_definition_versions_report_unknown_ids_and_keep_deleted_history`（`:308`）：missing→None、两版历史、软删除后仍 2 条、未知版本→None、0.1.0 快照保留首次脚本。
- `product_mcp_production_executor_tests.rs::backtest_runs_filters_match_the_go_definition_version_status_and_limit_matrix`（`:397`）：未过滤 `runCount=4`、definitionId+version+status+limit 组合 `runCount=1/totalMatched=2/truncated=true` 且 newest 优先、version-only、provider 大小写不敏感。
- `product_production_ports_adk_tests.rs::detached_adk_tool_executor_reports_domain_tools_as_unavailable`（`:449` 不可用半边）：未挂载端口时 research/optimize/portfolio/kline/result_view 全部 fail-closed。

### 探针（改坏 → 转红 → 回滚）

1. 去掉 `versions` 存在性判定 → `strategy_definition_versions_report_unknown_ids_and_keep_deleted_history` 与 `backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets` 同时转红。
2. 去掉 `filter_backtest_runs` 的 `runCount` 注入 → `backtest_runs_filters_match_the_go_definition_version_status_and_limit_matrix` 转红。
3. `quote_error` 的 Unavailable message 固定为 "quote failed" → `market_subscriptions_surface_quote_port_failures_without_fixture_payloads` 转红。
4. 分离态执行器错误文案改成 "not configured" → `detached_adk_tool_executor_reports_domain_tools_as_unavailable` 转红。

### 结论登记（partial 行内的边界）

- **:32 partial**：`recordADKWorkflowAudit` 无对应物（probe：`rg "recordADKWorkflowAudit" crates/ apps/ workers/` 0 命中）；`brokerReadQuery` 的 brokerId 默认由路由路径固定 `/api/v1/brokers/futu/{resource}`，空 market 默认 HK 在 `resolve_account` 内；scope/merge 半边由第 44 批 `order_scope_defaults_to_current_and_status_lists_merge_case_insensitively` 覆盖。
- **:77 partial**：任务/记忆 CRUD 由 `adk_task_and_memory_crud_contracts_match_go` 覆盖；`system.status` 的 `adk.enabled` 块与审计 kind（`task.saved`/`memory.deleted`）无对应写入（probe 0 命中）。
- **:194 partial**：`strategy.save_draft` / `strategy.save_definition` / `strategy.update_instance_mode` 不是 Rust 助手工具（REST 写入承担），`strategy.definitions` 不返回 instanceCount。
- **:449 partial**：不可用/派发/视图投影三半各有证据；未在同一用例断言 research_backtest 内嵌 `resultView` 入参归一与 `readyToRetry=true`（P2）。
- **:699 partial**：显式 provider 归一 + 冻结值写进候选 payload 有既有测试；Go 的 `freezeBacktestProviderID(value, default)` 在 default 变化时的 prepare/queue 双点断言无同名 helper/用例。
- **:774 partial**：Rust 的 provider 解析只读请求 payload（`requested_provider` 白名单校验），显式 override 天然隔离；缺少 Go 式双 goroutine 通道用例（ADK 工具执行器为同步接口，P2）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量：`portfolio_tools_test.go`（8 条）、`tool_catalog_test.go`（7 条）、`product_adapters_test.go`（6 条）…；随后 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 策略定义版本/快照的生产 wire 形状（本批 `:308` 引出：生产返回 store 原始行含 `visualModelJson`、无 `isCurrent`，冻结 fixture 为 `{definitionId,version,name,savedAt,isCurrent}` 与 `visualModel`）、工作流触发日志 active/page 过滤（第 42 批 `:559`）、`workflow_runs.*` 过滤参数（第 43 批 `:183`）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言（第 44/45 批）、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2786 Rust** / **933 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第四十六批：assembly `portfolio_tools_test.go` 全量结清（8 条；4 条新 `[x]`，4 条 `partial`）

范围：`internal/assistant/assembly/portfolio_tools_test.go` 8 条逐条结清——本批新增 **4 条 `[x]`**（`:85`、`:255`、`:338`、`:444`），`partial` 4 条（`:15`、`:155`、`:288`、`:376`）。`[x]` 计数 933 → **937**，Rust 测试 2786 → **2794**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'Portfolio|AccountOrders' -count=1`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（2 处行为修正 + 1 处复用重构 + 8 条新增回归）

Go 这 8 条用例覆盖三层组合读取（accounts/overview/positions）、多账户摘要（summary）、账户解析矩阵（exact/后缀/环境与市场隔离）与 `account.orders` 的账户+环境+市场+activeOnly 过滤。Rust 的等价 owner 是 `ProductionAdkToolExecutor` + `ProductionMcpToolExecutor` + `product_portfolio_projection`。

行为修正：

1. `crates/jftrade-engine/src/product_portfolio_projection.rs`：`resolve_portfolio_selection` 的 `not_found` mode 对齐 Go `resolvePortfolioAccounts`——带 accountId 未命中 → `account_id`，无 accountId 且无候选 → `all_matching_accounts`（此前统一返回 `none`）；`discovery_failed` 仍为 `none`。`selection_value` / `discovery_failed_selection` 提取为 `pub(crate)`，`discovery_failed_payload` 改为复用同一构造。
2. `crates/jftrade-engine/src/product_mcp_production_executor.rs::account_orders`：请求 `accountId` 时先经 `TradeReadPort` discovery 解析（exact → 唯一后缀），命中后把完整账户 id 交给执行层过滤，并回填 `selection`/`discoveredAccounts`/`partial`；未命中或 discovery 失败返回空 `orders` + `partial=true` + `warnings`（对应 Go 的 `emptyAccountOrdersResult`），且不再访问执行库。此前 Rust 把短 id 原样透传给「精确相等」的执行过滤，`accountId=240` 会静默返回空列表。未挂 trade reader 的部署保留透传边界（见 `:338` 的 partial 说明）。

复用重构：后缀解析只保留在 `product_portfolio_projection`，工具层通过 `pub(crate)` 入口复用，避免在 MCP/ADK 层复制账户匹配规则。

新增回归（8 条）：

- `product_portfolio_parity_tests.rs::portfolio_layered_tools_keep_discovery_overview_and_positions_separate`（`:85`）：三层读取账本（accounts 只 discovery；overview 每账户 funds+positions+orders；positions 只读 positions 且不读 funds），并断言 overview/positions item 不泄漏 funds、payload 无顶层 funds。
- `product_portfolio_parity_tests.rs::portfolio_account_resolution_matches_the_go_exact_suffix_and_isolation_matrix`（`:255`）：六例解析矩阵 + 无 accountId 的 `all_matching_accounts` + 无匹配市场的 mode 标签。
- `product_portfolio_parity_tests.rs::account_orders_resolves_requested_account_suffixes_through_discovery`（`:338`）：后缀 240→8240 的执行 query、`selection.mode=unique_suffix`、未命中空结果与不读执行库、无 accountId 的 `all_matching_orders`、discovery 失败空结果。
- `product_portfolio_parity_tests.rs::portfolio_overview_reports_partial_reads_without_classifying_empty_funds_as_assets`（`:444`）：零资金不归类资产、currency balance / market asset 归类资产、单读失败与三读失败的 partial/errors/positionCount。
- `product_portfolio_parity_tests.rs::portfolio_layered_tools_report_discovery_failure_and_partial_read_states`（`:155`/`:288`）：三个工具的 discovery_failed + partial + warnings，以及逐能力失败时的 errors/warnings。
- `product_mcp_server_tests.rs::account_orders_forwards_scope_account_environment_and_market_filters`（`:338` 透传边界）：无 trade reader 时 query 原样带 `accountId`/`market`，`count`/`activeOnly`/orders 透传。
- `product_mcp_server_tests.rs::portfolio_tools_require_trading_environment_and_fail_closed_without_a_broker_reader`（`:155`）：三个工具缺 `tradingEnvironment` 报错、缺 reader 时 fail-closed。
- `product_mcp_server_tests.rs::portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers`（`:15`/`:288` 现状固化）：`portfolio.summary` 合并 positions/balances/orders + connectivity/checkedAt 一致性、无顶层 funds、`brokerId` 非 futu 报 BAD_REQUEST。
- 新 fixture `product_portfolio_parity_tests.rs`（`AdkPortfolioFixtureRead` + `AdkPortfolioFixtureOrders`）：记录型 `TradeReadPort`，支持 discovery 失败与按 funds/positions/orders 单点失败，并由 `product_production_ports_adk_tests.rs` 以 `#[path]` 引入。

### 探针（改坏 → 转红 → 回滚）

1. 两处 `not_found` mode 改回 `"none"` → `portfolio_account_resolution_matches_the_go_exact_suffix_and_isolation_matrix` 与 `account_orders_resolves_requested_account_suffixes_through_discovery` 同时转红。
2. `resolve_account_orders_scope` 短路为透传 → `account_orders_resolves_requested_account_suffixes_through_discovery` 转红（`selection` 为 Null）。
3. 删除 `funds_have_assets` 的 `market_info_list` 分支 → `portfolio_overview_reports_partial_reads_without_classifying_empty_funds_as_assets` 转红。
4. `read_account_overview_item` 把 `partial` 固定为 `false` → `portfolio_layered_tools_report_discovery_failure_and_partial_read_states` 转红。
5. 三处 `accountId`/`market` 过滤参数删除（`account.orders`）→ `account_orders_forwards_scope_account_environment_and_market_filters` 转红。
6. 缺 `tradingEnvironment` 的校验与缺 reader 的 fail-closed 分别改坏 → `portfolio_tools_require_trading_environment_and_fail_closed_without_a_broker_reader` 转红。

### 结论登记（partial 行内的边界）

- **:15 partial**：Rust `portfolio.summary` 是三个单账户快照的合并，不含 Go 的 `accountSummaries` 多账户聚合、`hasAssetsOrPositions` 排序与 `queryMarket` 逐账户投影；排序规则在 overview 层有等价测试，缺口登记 P1。
- **:155 partial**：Go 第 (3) 子用例的 broker runtime `lastError` 警告在 Rust 无来源——`portfolio.*` 经 `TradeReadPort` 直连 discovery，base payload 固定 `connectivity="connected"`/`lastError=""`，bundle 的 `/api/v1/brokers/{brokerId}/runtime` 端口未被该投影消费；其余三个子场景已闭环。
- **:288 partial**：Go 的 summary partial/warnings/discovery 语义在 Rust summary 上不存在（同 :15 缺口），只能由分层工具的等价行为间接证明。
- **:376 partial**：读取记账（funds=1/positions=1 → funds=1/positions=2 的 Rust 拆分等价）与 runtime/session 投影均已覆盖，但 Rust 的 runtime 形状是 `session/connection`，不是 Go 的 `connectivity/lastError/accounts` 三元组；Go 的 runtime nil → "empty response" 在 Rust 对应端口不可用错误。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量：`tool_catalog_test.go`（7 条）、`product_adapters_test.go`（6 条）…；随后 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = `portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial（本批 `:15`/`:288` 引出）、`portfolio.*` 无法投影 broker runtime `lastError`（本批 `:155` 引出）、策略定义版本/快照的生产 wire 形状（第 45 批 `:308`）、工作流触发日志 active/page 过滤（第 42 批 `:559`）、`workflow_runs.*` 过滤参数（第 43 批 `:183`）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言（第 44/45 批）、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1768 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2794 Rust** / **937 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。


## 第四十七批：assembly `tool_catalog_test.go` 全量结清（7 条；1 条新 `[x]`，6 条 `partial`）

范围：`internal/assistant/assembly/tool_catalog_test.go` 7 条逐条结清——本批新增 **1 条 `[x]`**（`:707`），`partial` 6 条（`:16`、`:159`、`:272`、`:415`、`:550`、`:672`）。`[x]` 计数 937 → **938**，Rust 测试 2794 → **2801**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKRuntimeHelperInputNormalization|TestADKRuntimePollingAndPayloadHelpers|TestADKReadToolsNormalizeInputsAndExposeBusinessHandlers|TestADKRuntimeMiscHelpersAndMetadata|TestADKCoreToolHandlersNormalizeMarketAndPortfolioFlows|TestWatchlistListToolDefaultsToReadOnlyMetadataAndNormalizesPaging|TestExecutionReadToolsPropagateProjectionFailures' -count=1`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（4 处行为修正 + 7 条新增回归）

1. `crates/jftrade-strategy/src/pinespec/mod.rs`：`metadata_payload`/`risk_payload` 新增 Go `strategyMetadataPayload` 的 risk 投影——`allowedEntryDirection`、`maxDrawdown`、`maxIntradayLoss`、`maxIntradayFilledOrders`、`maxPositionSize`、`maxConsLossDays` 只在脚本声明正值时出现，类型/告警字段 trim，整数按 Go 的 `float64` 输出不带小数。此前 `strategy.validate_pine` 恒定返回 `risk: {}`，脚本声明的风控在验证答案里完全不可见。
2. `crates/jftrade-engine/src/product_mcp_production_executor.rs::watchlist_list`：market 参数改为先 trim 再大写（Go `strings.ToUpper`）。此前 `market=us` 会原样进入 `/api/v1/watchlist/items` 查询串。
3. `crates/jftrade-engine/src/product_production_ports_execution_orders.rs`：`/orders/{id}` 与 `/orders/{id}/events` 改为对路径段做 percent-decode（拒绝空段与含 `/` 的 id），修复 MCP `execution.order_events` 转发编码 id 时 404 的问题。
4. `crates/jftrade-engine/src/product_mcp_production_executor_helpers.rs`：新增 `wait_for_kline_sync_progress`/`kline_sync_status_is_terminal`，`backtest.kline_sync_status` 真正遵守 `waitForCompletionMs`（50ms 轮询、终态提前返回、任务消失 → 404 `BACKTEST_SYNC_TASK_NOT_FOUND`）；此前该参数被解析后丢弃。

新增回归（7 条）：

- `product_tool_catalog_parity_tests.rs::broker_read_tools_normalize_scope_filters_and_identifier_lists`（`:272`）：五个 broker 读工具在 recorder 上断言完整 query（scope=CURRENT/HISTORY 大小写、tradingEnvironment、accountId、symbol、startTime/endTime、标识符列表），缺 `orderIdEx`/`symbols` 与未知 scope 均 BAD_REQUEST 且不触达端口。
- `product_tool_catalog_parity_tests.rs::system_plugin_and_risk_tools_project_port_payloads`（`:550`）：`system.futu_opend`/`plugins.catalog`/`risk.state`/`risk.events` 的薄投影与四条系统读路径顺序。
- `product_tool_catalog_parity_tests.rs::watchlist_list_normalizes_filters_and_rejects_out_of_range_limits`（`:672`）：group/query/cursor 去空格、market 大写、默认 limit=50 的分组路径、limit 0/201 BAD_REQUEST、`includeQuotes=true` 的 503 失败关闭且不触达端口。
- `product_tool_catalog_parity_tests.rs::execution_order_events_lists_orders_and_propagates_projection_failures`（`:707`）：列表成功路径、百分号编码明细路径（`/orders/order%2D1/events`）、三处失败传播（列表/明细/`account.orders`）映射为 503 `EXECUTION_UNAVAILABLE` 并保留端口消息。
- `product_tool_catalog_parity_tests.rs::kline_sync_status_waits_for_a_terminal_status_within_the_requested_window`（`:159`）：脚本化进度序列断言轮询到 completed、终态提前返回、任务消失 404。
- `product_production_ports_execution_preview_tests.rs::execution_read_decodes_percent_encoded_order_ids`：`order%2D1` 在生产读端口解码为 `order-1` 后命中订单与事件。
- `crates/jftrade-strategy/tests/pine_mcp_contract.rs::validation_metadata_projects_declared_risk_limits_like_go`（`:415`）：默认 `risk: {}`、`defaultQtyMode/defaultQtyValue/pyramiding` 归一化、`hooks=["on_kline_close"]`，以及声明后的六个 risk 字段完整结构。

### 探针（改坏 → 转红 → 回滚）

1. `risk_payload` 短路为空对象 → `validation_metadata_projects_declared_risk_limits_like_go` 转红（`risk` 为 `{}`）。
2. `decode_order_id` 改为返回原始路径段 → `execution_read_decodes_percent_encoded_order_ids` 转红（`order%2D1` ≠ `order-1`）。
3. `wait_for_kline_sync_progress` 立即返回 → `kline_sync_status_waits_for_a_terminal_status_within_the_requested_window` 转红（`running` ≠ `completed`）。
4. watchlist limit 上界 200 → 5000 → `watchlist_list_normalizes_filters_and_rejects_out_of_range_limits` 转红（201 被接受）。
5. `broker_query` 把 scope 小写化 → `broker_read_tools_normalize_scope_filters_and_identifier_lists` 转红（`scope=current`）。
6. `plugins_catalog` 包一层 `{"catalog": ...}` → `system_plugin_and_risk_tools_project_port_payloads` 转红（`plugins[0].id` 为 Null）。
7. 去掉 watchlist market 大写 → `watchlist_list_normalizes_filters_and_rejects_out_of_range_limits` 转红（`market=us`）。探针 2/3/4/5/6/7 均在本批内执行并已回滚。

### 结论登记（partial 行内的边界与差异）

- **:16 partial**：`brokerReadInput`（默认市场/scope/账号/环境/状态列表）与 `taskPatchFromInput` 的任务补丁归一化有 Rust 断言；`optionalBoolInput`、`intPtrFromInput`、`stringPtrFromInput`、`stringSliceFromPresentInput`、`backtestResultViewInputFromNested` 是 `map[string]any` 适配层强转，Rust 以 serde 强类型参数取代，无对应 owner。
- **:159 partial**：K 线同步轮询与终态判定已闭环；`backtestDataReadinessPayload`、`waitForADKBacktestStatus`、`statusFromBacktestResultView`、`isTerminalBacktestStatus` 在 Rust 无 helper（模型侧直接轮询 `backtest.result_view`），`klineSyncProgressPayload(nil)` 的 readyToRetry 由 retry hint 投影覆盖。
- **:272 partial**：五个 broker 读工具、`risk.*`、`execution.order_events` 两条路径均已覆盖；缺口是 `market.depth` 的自然语言 `query` 推断与 `num` 字符串强转——Rust 工具 schema 只接受 `instrumentId` 或 `market`+`symbol`，缺标的直接 BAD_REQUEST，登记 P2。
- **:415 partial**：risk 投影已修复；`BuildCompiledHookKinds`/`BuildCompiledRequirementsPayload`/`pageEnvelope`/`SourceFormatPineV6` 各有 Rust owner；`inferMarketSymbol`、`floatValue`、`boolInputValue(Default)`、`summarizeADKText`、`lastString`/`lastBacktestTrade`/`lastBacktestCandle`、`callMap`/`callBool`、`nowStringRFC3339Nano` 为 Go 侧 helper，Rust 无对应实现。
- **:550 partial**：`system.futu_opend`/`plugins.catalog`/`risk.state`/`risk.events` 已断言；`market.subscriptions`/`market.snapshot`/`market.candles`、托管账户、`portfolio.summary`、`account.orders` 由第 44-46 批证据覆盖，但不在本批测试内。
- **:672 partial**：过滤归一化与 limit 边界已断言；差异是 Go 会把 `includeQuotes` 传给依赖返回行情，Rust 生产 bundle 无行情富化，`includeQuotes=true` 失败关闭为 503 `WATCHLIST_QUOTES_UNAVAILABLE`，登记 P2。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量：`product_adapters_test.go`（6 条）、`adk_product_catalog_test.go`（4 条）；随后 `internal/app/apiserver`（574）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = `portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial（第 46 批 `:15`/`:288`）、`portfolio.*` 无法投影 broker runtime `lastError`（第 46 批 `:155`）、策略定义版本/快照的生产 wire 形状（第 45 批 `:308`）、工作流触发日志 active/page 过滤（第 42 批 `:559`）、`workflow_runs.*` 过滤参数（第 43 批 `:183`）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `market.depth` 自由文本 instrument 推断（本批 `:272` 引出）、`watchlist.list includeQuotes` 行情富化（本批 `:672` 引出）、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1819 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2801 Rust** / **938 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。


## 第四十八批：assembly `product_adapters_test.go` 全量结清（6 条；2 条新 `[x]`，4 条 `partial`）

范围：`internal/assistant/assembly/product_adapters_test.go` 6 条逐条结清——本批新增 **2 条 `[x]`**（`:15`、`:155`），`partial` 4 条（`:26`、`:83`、`:186`、`:217`）。`[x]` 计数 938 → **940**，Rust 测试 2801 → **2806**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestCustomizationToolsMapToOpenDOperations|TestProductToolInputHelpersCompleteBranches|TestProductAndExecutionDispatchFailureBoundaries|TestProductExecutionAdapterNormalizesScreenAndCalendarV2Inputs|TestProductExecutionAdapterRejectsInvalidScreenPageAndValue|TestProductExecutionAdapterCoversSpecialDispatchFailuresAndSnapshots' -count=1`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（5 条新增回归，无生产改动）

Go 这 6 条覆盖 Go 侧产品/执行适配器的输入强转 helper、分发失败边界，以及 research.screen / research.calendar 的 V2 归一。Rust 侧对应 owner 已是强类型端口（`ProductionMcpToolExecutor` + `product_research_screen_write_port` + capability catalog），本轮把「同名行为」补成断言，不改生产代码：

- `product_broker_capabilities_projection_tests.rs::customization_tools_map_to_their_single_opend_action`（`:15`）：`alerts.price.set`→`set`/`Qot_SetPriceReminder`、`alerts.option_event.set`→`set`/`Qot_SetOptionEventAlert`、`watchlist.remote.modify`→`modify`/`Qot_ModifyUserSecurity`，并断言每个工具只暴露一个动作与 `kind=request`。
- `product_research_screen_write_port_tests.rs::screen_query_defaults_the_page_and_keeps_catalog_columns`（`:155` 屏幕半）：默认 limit=50、offset=2 保留、响应回填 `catalogVersion` 与 `columns[0].factorKey=simple.last_close`。
- `product_research_screen_write_port_tests.rs::screen_query_rejects_wrong_catalog_and_schema_versions`（`:186`）：`page.limit=101`、`catalogVersion="wrong"`、`querySchemaVersion=1` 均在 provider 之前 400；用 `UnreachablePort`（被调用即 panic）证明校验先于端口。
- `product_mcp_server_tests.rs::research_calendar_forwards_the_advanced_filter_query`（`:155` 日历半）：`sort/stockScope/marketCapMin/optionVolumeMax/ivMin/ivRankMax/ivPercentileMin` 全部进入 `/api/v1/research/calendars` 查询串。
- `product_mcp_server_tests.rs::product_dispatch_rejects_unknown_tools_and_missing_instruments`（`:83`/`:217`）：未知工具 → `MCP_TOOL_UNAVAILABLE`、缺 symbols 的 snapshot → `BAD_REQUEST`、buying-power 缺类型化字段 → `BAD_REQUEST`、`research.screen` 非对象输入 → `BAD_REQUEST`、`research.calendar` 缺 operation → `CAPABILITY_UNAVAILABLE`、`market.snapshot` 成功路径转发 `/api/v1/market-data/snapshots/US/AAPL`、失败 quote 端口 → 503 `MARKET_DATA_QUOTE_READ_UNAVAILABLE` 且保留端口消息。

### 探针（改坏 → 转红 → 回滚）

1. `normalize_query` 默认 limit 50 → 20 → `screen_query_defaults_the_page_and_keeps_catalog_columns` 转红（left 20 / right 50）。
2. `catalogVersion` 白名单校验放开 → `screen_query_rejects_wrong_catalog_and_schema_versions` 转红（`UnreachablePort` 被调用并 panic，证明校验先于 provider）。
3. `market_research_request` 把 `sort` 加入排除列表 → `research_calendar_forwards_the_advanced_filter_query` 转红（`sort=iv%5Fdesc` 缺失，并打印实际查询串）。
4. `alerts.option_event.set` 动作改成 `"write"` → `customization_tools_map_to_their_single_opend_action` 转红。
5. `snapshot_request` 缺 symbols 时默认补 `US.AAPL` → `product_dispatch_rejects_unknown_tools_and_missing_instruments` 转红（错误码从 `BAD_REQUEST` 变成 `MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE`）。
   5 处探针均在本批内执行并已回滚，`git diff` 只留三份测试文件。

### 结论登记（partial 行内的边界与差异）

- **:26 partial**：`toolInstrumentID` 的 trim+大写由 candle 路由测试证明；`decodeToolInput`（channel/nil）、`toolMapString/Int/Strings`（nil/数字/interface 切片）、`cloneToolInput` 是 `map[string]any` 适配层强转，Rust 以 serde 强类型参数取代。
- **:83 partial**：未知工具、缺 symbols、buying-power 缺字段已断言；Go 用 `make(chan int)` 构造的不可 marshal 输入在 JSON 边界不存在，保留为边界。
- **:186 partial**：三处非法取值/版本拒绝已断言；同用例的 `decodeToolInputValue` channel/nil 分支无 Rust owner。
- **:217 partial**：快照成功/失败、malformed screen、缺 operation 均已断言；差异是 Go 的 `research.calendar(nil)` 返回默认输入并成功，而 Rust fail-closed 要求对象 + 显式 operation；Go 的 plain-service typed dispatch 探针在 Rust 无对应概念（端口为强类型 trait）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量 45 条：`runtime_test.go`（5）、`adk_product_catalog_test.go`（4）、`application_adapter_boundaries_test.go`/`application_strategy_lifecycle_test.go`/`market_index_constituents_tools_test.go`/`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3）…；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = `portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `market.depth` 自由文本 instrument 推断（第 47 批 `:272`）、`watchlist.list includeQuotes` 行情富化（第 47 批 `:672`）、`research.calendar` 缺省输入与 operation 的 fail-closed 差异（本批 `:217` 引出）、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2806 Rust** / **940 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。


## 第四十九批：assembly `runtime_test.go` 全量结清（5 条；2 条新 `[x]`，3 条 `partial`）

范围：`internal/assistant/assembly/runtime_test.go` 5 条逐条结清——本批新增 **2 条 `[x]`**（`:45`、`:55`），`partial` 3 条（`:14`、`:74`、`:131`）。`[x]` 计数 940 → **942**，Rust 测试 2806 → **2808**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestOpenBuildsToolsServiceAndIdempotentLifecycle|TestRuntimeDatabaseProbesUseProvidedLayout|TestOpenOwnsApplicationToolRegistration|TestHandleExposesNarrowAuditAndToolOperations|TestNilHandleLifecycleIsSafe' -count=1`（checkout `/tmp/go452dea11.niwD1G`）。

### 本批结论（2 条新增回归，无生产改动）

Go 这 5 条描述 `assembly.Open` 拥有的运行时句柄：构建 tools/service、幂等 Close、按注入布局探测 runtime/session 数据库、注册应用工具、窄审计与工具存取、以及 nil 句柄的生命周期安全。Rust 对应 owner 是 composition root（`start_product_runtime`/`ProductRuntimeHandle`、`initialize_production_databases`、`ProductionToolCatalog`、`AdkStore`/`AdkSessionStore`），本轮把缺口补成断言：

- `product_data_management_batch_atomic_startup_tests.rs::database_probes_use_the_provided_layout`（`:45`）：用 `JFTRADE_ADK_DB`/`JFTRADE_ADK_SESSION_DB` 注入自定义布局，断言 descriptor 保持注入路径、两个文件存在于该路径、`AdkStore::open` 与 `AdkSessionStore::open` 探针通过、runtime 库带固定 schema 版本。
- `product_mcp_server_tests.rs::production_catalog_registers_application_tools_from_the_composition_root`（`:55`）：断言 composition root 的 `PRODUCTION_TOOL_DEFINITIONS` 同时登记 `system.status` 与 `strategy.research_backtest`，且所有端口 Ready 时两者都在 `callable_tools()` 中。

### 探针（改坏 → 转红 → 回滚）

1. `database_descriptors` 忽略 `JFTRADE_ADK_DB` 覆盖 → `database_probes_use_the_provided_layout` 转红（路径回退到默认 root `/…/adk.db`，不再是 `/…/provided-layout/adk.db`）。
2. `from_bindings_with_research` 跳过 `system.status` 定义 → `production_catalog_registers_application_tools_from_the_composition_root` 转红并打印实际可调用列表（其中可见 `strategy.research_backtest` 仍在）。
   2 处探针均在本批内执行并已回滚。

### 结论登记（partial 行内的边界与差异）

- **:14 partial**：`start_product_runtime` 的启动 + 干净停机与显式/Drop 停机顺序有断言；差异是 Rust 没有 `Available()`/`Service()`/`HasTool()`/`MCPStatus()` 访问器，`shutdown(self)` 消耗 handle，重复 Close 在类型层不可表达，Drop 兜底同步停机。
- **:74 partial**：审计写入 + 按 kind/subjectId/limit 的窄查询已断言（2 条 agent.saved + 1 条 provider.saved → total=2/returned=1）；Go 的 `RegisterTool`/`Tool()` 运行期动态注册在 Rust 无 owner（目录由 composition root 静态拥有）。
- **:131 partial**：Rust 无可空 `*Handle`，等价约束是缺依赖失败关闭（缺 Futu/OpenD 或类型化 reader 时研究工具 unavailable，`system.runtime_dependencies` fail-closed）；Go 的 nil-safe 生命周期方法在 Rust 由 supervisor/HTTP 层持有，无同名 owner。

### 仍未结清（下一批）

- `internal/assistant/assembly` 余量 40 条：`adk_product_catalog_test.go`（4）、`application_adapter_boundaries_test.go`/`application_strategy_lifecycle_test.go`/`market_index_constituents_tools_test.go`/`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3）…；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = `portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `market.depth` 自由文本 instrument 推断（第 47 批）、`watchlist.list includeQuotes` 行情富化（第 47 批）、`research.calendar` 缺省输入 fail-closed 差异（第 48 批）、运行期动态工具注册与可空句柄（本批 `:74`/`:131` 引出）、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2808 Rust** / **942 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十批：assembly `adk_product_catalog_test.go` 全量结清（4 条；2 条新 `[x]`，2 条 `partial`）

范围：`internal/assistant/assembly/adk_product_catalog_test.go` 4 条逐条结清——本批新增 **2 条 `[x]`**（`:13`、`:138`），`partial` 2 条（`:29`、`:77`）。`[x]` 计数 942 → **944**，Rust 测试 2808 → **2812**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestDefaultBuiltinAgentToolsExistInAssembledRegistry|TestCapabilityCatalogSurfacesAreRegisteredAndMCPBounded|TestProductToolRegistryAndOperationSchemasAreCatalogBacked|TestProductReadSchemasRejectInvalidRoutingAndFreeTextFields' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（3 条新增回归，无生产改动）

Go 这 4 条把「内置 Agent 工具目录」和「券商能力目录」钉在一起：默认助手引用的工具必须在组装 registry 中；每个能力 feature 的工具必须按访问级别带 permission/risk 且只读面只放 reviewed 读；product 工具与操作 schema 必须由 `CapabilityCatalog` 供数；读 schema 不接受自由文本 `query` 或越权的 `tradingEnvironment`。Rust 的对应 owner 是 `ProductionToolCatalog`/`PRODUCTION_TOOL_DEFINITIONS`（模型目录）、`validate_agent_tools`（成员校验）、`product_broker_capabilities_projection`（能力目录）与 `product_mcp_schema_catalog`（reviewed schema），本轮把缺口补成断言：

- `product_production_ports_adk_tests.rs::builtin_default_agent_tools_all_resolve_in_the_assembled_catalog`（`:13`）：组装目录非空；内置默认模板用完整目录创建且保留全部工具（membership bypass 只在「集合相等」时生效）；同一目录在无 bypass 的普通 Agent 上仍全部解析；未知工具 `not.an.assembled.tool` 返回 400 `unknown ADK tool: not.an.assembled.tool`。
- `product_broker_capabilities_projection_tests.rs::capability_access_classes_bound_the_reviewed_read_only_surface`（`:29`）：50 个 feature 都必须有 tool 映射；读类 `read_only`/`none`/`surface.readOnlyMcp=true` + 在 `REVIEWED_READ_ONLY_TOOLS` 内 + 在 `PRODUCTION_TOOL_DEFINITIONS` 注册且 `tool_access_policy=read_internal`；写类 `write_external`/high、交易类 `live_trading`/critical 都不得进入只读 MCP 面；6 个 execution feature 另断言 approval/less_approval/all 三模式全部要求审批；目录只保留 read/trade/write 三类。
- `product_broker_capabilities_projection_tests.rs::reviewed_tool_operation_schemas_are_catalog_backed`（`:77`）：每个 feature 的 tool 至少被一个 operation 声明；`market.capabilities` 是唯一注册但无 broker operation 的目录项；`REVIEWED_READ_ONLY_TOOLS` 全部注册且 permission 为 `read_internal`；每个声明 `operation` enum 的 reviewed schema 与该工具的能力目录操作集合完全相等，并用 `market.candles`/`research.rankings`/`research.screen`/`prediction.history`/`derivatives.option_analysis` 守卫「比较非空」。
- `product_mcp_protocol_tests.rs::reviewed_read_schemas_keep_structured_routing_without_free_text_fields`（`:138`）：`market.capabilities` 有 `tradingEnvironment` 且无 `query`；`market.snapshot`/`research.news`/`research.calendar` 都不含 `tradingEnvironment`；`account.orders` 保留 `activeOnly` 且无 `query`。

### 探针（改坏 → 转红 → 回滚）

1. `catalog_feature` 把写类 permission 改成 `write_internal` → `capability_access_classes_bound_the_reviewed_read_only_surface` 转红（left `write_internal` / right `write_external`，定位 `alerts.price.set`）。
2. `typed_instrument_schema` 删掉 `research.valuation` 的 `constituents` → `reviewed_tool_operation_schemas_are_catalog_backed` 转红（left `{detail}` / right `{constituents, detail}`）。
3. `common_capability_properties` 加回 `query` → `reviewed_read_schemas_keep_structured_routing_without_free_text_fields` 转红（打印 properties 含 `query`）。
4. `validate_agent_tools` 的 `known` 置空 → `builtin_default_agent_tools_all_resolve_in_the_assembled_catalog` 转红（400 `unknown ADK tool: interaction.request_user`）。
   4 处探针均在本批内执行并已回滚；`git diff` 只留三份测试文件与清单/报告。

### 结论登记（partial 行内的边界与差异）

- **`:29 partial`**：能力目录层的三类访问级别 + 只读 MCP 面 membership + 交易类三模式审批已全部断言；不可迁移的一半是 Go 在 registry 中注册 `ProductTradeToolDefinitions()`/`ProductWriteToolDefinitions()` 的 9 个外部写/交易工具，Rust 的模型目录 `PRODUCTION_TOOL_DEFINITIONS`（79 条）刻意不含 `execution.order_*`/`execution.combo_*`/`alerts.*.set`/`watchlist.remote.modify`，这些工具的 descriptor permission/`riskLevel`/`RequiresApprovalIn` 在 Rust 无 owner。列 P1 待办：若要让模型可直接下单/改提醒，必须先有对应 production port 与审批/幂等 owner，再补注册。
- **`:77 partial`**：目录到操作的关系与 operation schema 等价已断言（其余 reviewed 工具保持无 enum 的单一操作语义，与 Go 一致）；同 `:29` 的原因，Go 的 `productTools` 还含 6 个交易与 3 个外部写工具，Rust 未注册，故整行记为部分覆盖。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 38 条：`application_adapter_boundaries_test.go`/`application_strategy_lifecycle_test.go`/`market_index_constituents_tools_test.go`/`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3），`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`（各 2）等；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（本批 `:29`/`:77` 引出）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `market.depth` 自由文本 instrument 推断（第 47 批）、`watchlist.list includeQuotes` 行情富化（第 47 批）、`research.calendar` 缺省输入 fail-closed 差异（第 48 批）、运行期动态工具注册与可空句柄（第 49 批）、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1832 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2812 Rust** / **944 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十一批：assembly `application_adapter_boundaries_test.go` 全量结清（3 条；2 条新 `[x]`，1 条 `partial`）

范围：`internal/assistant/assembly/application_adapter_boundaries_test.go` 3 条逐条结清——本批新增 **2 条 `[x]`**（`:20`、`:71`），`partial` 1 条（`:128`）。`[x]` 计数 944 → **946**，Rust 测试 2812 → **2813**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestApplicationAdapterKeepsNilPortsCallable|TestApplicationAdapterUsesConfiguredRuntimeAndSettings|TestApplicationAdapterValidatesDomainInputsBeforeDelegation' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 条新增回归，无生产改动）

Go 这三条描述 `ApplicationAdapter` 的边界：nil 端口时全部领域调用仍可调用但报不可用、配置端口时把 runtime/设置投影出来、委托前先校验领域输入。Rust 没有可空 adapter，等价 owner 分裂成 composition root 的端口装配、设置文件的运行时限制投影与 reviewed MCP schema 校验，本轮把缺口补成断言：

- `product_mcp_server_tests.rs::unwired_production_bundle_keeps_every_reviewed_tool_callable_without_payloads`（`:20`）：只装配 catalog/store（未注入 `ProductionPortBundle`）的 executor 上遍历 `REVIEWED_READ_ONLY_TOOLS` 全部 69 个工具，断言每个都返回 400..503 的结构化失败且 `code` 非空、不返回伪造 payload；唯一能回答空请求的是进程内 `strategy.pine_spec`（`strategy.validate_pine` 同样不依赖端口，但空请求以 400 拒绝）；另对 `execution.order_events`/`broker.orders`/`strategy.definitions`/`backtest.runs`/`backtest.kline_sync_status`/`research.screen_catalog`/`market.providers`/`risk.state` 断言 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`。
- `:71` 引用既有证据：`product_production_assembly_tests.rs::adk_snapshot_and_tools_routes_return_the_composed_catalog`（`runTimeoutMs=660000`/`streamIdleTimeoutMs=420000` 经设置文件回环，`GET /api/v1/adk` 的 `runtimeSettings` 原样读出）、`product_adk_run_timeout.rs::tests::assistant_run_timeout_falls_back_to_the_reference_default_window`（默认 1_800_000、配置 660_000 胜出）、`product_adk_read_tests.rs::adk_audit_route_filters_by_kind_and_subject_id`（按 kind/subjectId/limit 取回审计）、`product_system_control_read_tests.rs::system_status_matches_go_stable_fields_without_claiming_migration_ownership`（`name=JFTrade`）、以及无 runtime 时 `workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation` 的 503 关闭。

### 探针（改坏 → 转红 → 回滚）

1. `risk.state` 在 `ports.is_none()` 时返回 `{"killSwitch":{"status":"unavailable"}}`（复刻 Go 的 fallback）→ `unwired_production_bundle_keeps_every_reviewed_tool_callable_without_payloads` 转红（left `[risk.state, strategy.pine_spec]` / right `[strategy.pine_spec]`）。
2. 可用性错误码从 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE` 改成 `MCP_EXECUTOR_UNAVAILABLE` → 同一测试在 503 断言转红（定位 `execution.order_events`）。
   2 处探针均在本批内执行并已回滚；`git diff` 只留测试文件与清单/报告。

### 结论登记（partial 行内的边界与差异）

- **`:20` 差异**：Go 保留三个不失败 fallback（`FutuOpenDHealth`→`status=unavailable`、`RiskState()` 非 nil、`BacktestKLineSyncProgress`→`found=false`），Rust 对这三者一律 fail-closed，因此本行按「结构化失败」而非某个具体 fallback 值断言，差异已写进结论。
- **`:128 partial`**：可达半边已断言 —— execution 读在缺端口时失败关闭、broker 读的 scope/标识符校验先于端口（archive 非法、缺 `orderIdEx`/`symbols` → BAD_REQUEST）、research backtest 请求派发保留 interval/useExtendedHours/chartType。未迁移：Go 的 `strategyInstanceSummary`/`strategySummaryDefinitionID`（InstanceView→摘要 trim 与 definitionId 优先级）、`applicationOptimizationRuns` 的空实现 `Get`/`Cancel` no-op、`validationInstrument`（`Program.Metadata` symbol/interval trim）在 Rust 没有同名 owner（助手目录不生产 instance summary，校验入口是 `jftrade-strategy` typed pipeline）。列 P2 待补。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 35 条：`application_strategy_lifecycle_test.go`/`market_index_constituents_tools_test.go`/`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3），`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`（各 2）等；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 本批 `:128` 的 instance summary/optimization no-op/validationInstrument 三个无 owner 断言、`market.depth` 自由文本 instrument 推断（第 47 批）、`watchlist.list includeQuotes` 行情富化（第 47 批）、`research.calendar` 缺省输入 fail-closed 差异（第 48 批）、运行期动态工具注册与可空句柄（第 49 批）、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1833 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2813 Rust** / **946 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十二批：assembly `application_strategy_lifecycle_test.go` 全量结清（3 条；1 条新 `[x]`，2 条 `partial`）

范围：`internal/assistant/assembly/application_strategy_lifecycle_test.go` 3 条逐条结清——本批新增 **1 条 `[x]`**（`:84`），`partial` 2 条（`:122`、`:141`）。`[x]` 计数 946 → **947**，Rust 测试 2813 → **2814**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestApplicationAdapterStrategyInstanceLifecyclePorts|TestApplicationAdapterStrategyInstanceLifecycleRejectsInvalidInputs|TestApplicationAdapterStrategyInstanceLifecycleRejectsDefinitionAndActivityBoundaries' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 条新增回归，无生产改动）

Go 这三条把策略实例生命周期钉在助手工具依赖上：instantiate/start/stop(pause,stop)/refresh/update-risk/activity 各有端口调用与计数断言，空 id、未知定义、非法动作、未知实例分别在委托前或读取时失败。Rust 的 owner 分裂成 MCP `strategy.instance_activity`（读）、策略运行时写路由（生命周期写）与策略定义写路由（instantiate），本轮补上缺失的活动读断言：

- `product_mcp_server_tests.rs::strategy_instance_activity_tool_normalizes_kind_and_paging_before_the_read_port`（`:84` 活动半边 + `:122`/`:141` 边界半边）：断言 `" instance-1 "` 被 trim 后走 `/api/v1/strategies/instance-1/logs`、首读 query 恰为 `limit=50&offset=0`；`kind=AUDIT` 折叠大小写后走 `/audit` 并携带 `limit=10`/`offset=2`/`kind=pause`/`fromTime`/`toTime`，且 logs-only 的 `level` 不进入 audit 读；非法 `kind`/`limit=0`/`limit=201`/`offset=-1`/缺 `instanceId` 一律 400 `BAD_REQUEST` 且消息精确匹配；端口缺实例 404 `STRATEGY_INSTANCE_NOT_FOUND`、端口不可用 503 `STRATEGY_ACTIVITY_UNAVAILABLE`。
- `:84` 其余半边引用既有证据：`strategy_runtime_write_product_replays_browser_failure_recovery_and_restart`（start/update/update-runtime-risk/pause/stop/refresh-definition + 失败恢复与重启重放，及浏览器 CSRF/来源校验）、`instantiate_persists_the_same_normalized_binding_as_runtime_update`（instantiate 绑定归一化与持久化）。

### 探针（改坏 → 转红 → 回滚）

1. `strategy_instance_activity` 的默认 `kind` 从 `logs` 改成 `audit` → `strategy_instance_activity_tool_normalizes_kind_and_paging_before_the_read_port` 转红（left `/api/v1/strategies/instance-1/audit` / right `…/logs`）。
2. 同一工具的 `limit` 下界从 1 改成 0 → 该测试在 `limit=0` 用例转红（`expect_err` 收到成功 payload）。
   2 处探针均在本批内执行并已回滚；`git diff` 只留测试文件与清单/报告。

### 结论登记（partial 行内的边界与差异）

- **`:84` 差异（P2）**：Go 的 `StrategyInstanceActivity` 对 `limit=0`/`offset=-1` 做钳制（0→1、-1→0），Rust 按 reviewed schema（`limit` 1..200、`offset`≥0）直接 400 拒绝，属 fail-closed 收紧；如需完全对齐要么放宽 schema 要么在 schema 里写明默认值，记 P2 复核。
- **`:122 partial`**：可达半边为 `strategy.instance_activity` 缺 `instanceId` → 400、`strategy.definition_versions.*` 缺 `definitionId` → 400（`backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets`）。不可迁移：Rust 模型目录没有 instantiate/start/stop/refresh/update-risk 这些生命周期写工具（与第 50 批登记的「模型目录不含外部写/交易工具」同类边界），控制台经 `/api/v1/strategies/{instanceId}/{action}` 写入，空 id 不匹配路由形态（404 `NOT_FOUND`），不存在「空 id 传给服务」的等价路径。
- **`:141 partial`**：可达半边为缺失实例 404、未知定义 404、instantiate 空 body 200 / 畸形 JSON 400（`instantiate_accepts_empty_body_but_rejects_malformed_json`）。未迁移：Go 的「空 definition id」与「`stop action=restart` 非法」在 Rust 由路由形态与动作枚举承担（空 id → 404；只有 pause/stop 两个路由，restart 不存在 → 404），无等价的「服务内非法动作」分支。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 32 条：`market_index_constituents_tools_test.go`/`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3），`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`（各 2）等；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（本批 `:122`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 活动读分页钳制差异（本批 `:84`）、第 51 批 `:128` 的 instance summary/optimization no-op/validationInstrument 三个无 owner 断言、`market.depth` 自由文本 instrument 推断、`watchlist.list includeQuotes` 行情富化、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1834 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2814 Rust** / **947 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十三批：assembly `market_index_constituents_tools_test.go` 全量结清（3 条；3 条新 `[x]`，含生产实现）

范围：`internal/assistant/assembly/market_index_constituents_tools_test.go` 3 条逐条结清——本批新增 **3 条 `[x]`**（`:14`、`:60`、`:69`），并补齐 Go 有、Rust 之前完全没有的生产能力 `market.index_constituents`。`[x]` 计数 947 → **950**，Rust 测试 2814 → **2825**（新增 11 条：端口级 6 条、executor 级 5 条）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKMarketIndexConstituentsToolForwardsNormalizedInputs|TestADKMarketIndexConstituentsToolFailsClosedWithoutPort|TestADKMarketIndexConstituentsToolSurfacesProviderCapabilityAsClearMessage' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（生产缺口：新增 tool-only 能力）

参考面上 `market.index_constituents` 只存在于 ADK 工具注册表（`internal/assistant/assembly/market_capability_tools.go`）与 `skillsruntime` 严格 schema：既不在 `LocalMCPReadOnlyToolNames`（69 个 reviewed 名字里只有 `research.news`/`research.corporate_actions`），也没有公开 HTTP 路由。Rust 侧此前只有 Python sidecar 的 `GET /providers/akshare/index-constituents/{market}/{symbol}`，engine 既无端口也无目录条目，属真实功能缺失，故本批按「先失败回归、再补生产」闭环实现：

- `crates/jftrade-engine/src/product_market_index_constituents_read_port.rs`（新）：`MarketIndexConstituentsReadPort` + `MarketIndexConstituentsReadError`（`Unavailable`/`Failed{status,code,message,retry_after_seconds}`），消费方是生产 MCP executor，不是 wire handler。
- `crates/jftrade-engine/src/product_production_ports_market_index_constituents.rs`（新）：AKShare 所有权校验（非 AKShare → 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED` + `futu-opend`/`yahoo-finance` 标签）、`CN` 聚合仅接受 `SH.<code>`/`SZ.<code>` 并解析到交易所叶子、helper 未 ready/未配置 → `Unavailable`、helper 转发 `GET /providers/akshare/index-constituents/{market}/{symbol}?limit=`、identity 漂移与空 `code` → 502 `BAD_GATEWAY`、payload 无 `constituents` 数组 → 502（不允许把畸形 payload 当成空列表）、`AKSHARE_UNSUPPORTED` → 409 capability、`weight` 为 null 原样保留。
- 目录与绑定：`ProductionRouteAdapter::MarketIndexConstituentsRead` + 启动矩阵（仅 `akshare` 且 helper ready 为 `Ready`，其余 `ExternalUnavailable`）+ `PRODUCTION_TOOL_DEFINITIONS` 条目（category `market`，79 → 80）+ 动态 readiness 分支（Futu/yfinance 保持 unavailable）+ executor 派发（`limit` 1..1000 默认 200，经 `instrument()` 归一化 market/symbol）+ reviewed schema（strict object，required `[market,symbol]`，`limit` default 200）+ 模型/重放白名单。HTTP 传输层显式 404：该能力刻意不暴露公开路由。
- 测试：`product_production_ports_market_index_constituents_tests.rs` 6 条（真实 TCP helper fixture 断言请求行与 `limit`、CN 聚合解析、identity/空 code/缺数组拒绝、provider 与 readiness 失败关闭、非 CN/裸 CN 拒绝、helper capability 折叠）；`product_mcp_index_constituents_tool_tests.rs` 5 条（输入归一化与默认值、descriptor 策略与 `jftrade-market` 技能归属、缺 owner 端口双层 fail-closed、provider capability 直传、真实激活 Futu 的端到端 409）。

### 探针（改坏 → 转红 → 回滚）

1. `resolve_cn_index` 不再大写 market（`" sh "` 原样下发）→ `index_constituents_read_forwards_the_normalized_leaf_and_limit` 转红（helper 请求行断言失败）。
2. executor 默认 `limit` 200 → 100 → `market_index_constituents_tool_normalizes_its_inputs_before_the_read_port` 转红（left `[("SH","000300",300),("SH","000300",100)]` / right `…,200`）。
3. capability 错误码 `MARKET_DATA_CAPABILITY_UNSUPPORTED` → `MARKET_DATA_CAPABILITY_UNAVAILABLE` → `index_constituents_read_requires_akshare_and_a_ready_helper` 转红（Futu 分支）。
4. 提供商缺失时回退为 AKShare（`unwrap_or(Akshare)`）→ 同一测试的「未配置 provider」断言转红（错误变成 `market-data helper is not ready`）。
   4 处探针均在本批内执行并已回滚；`git diff` 只留实现、测试、清单与报告。

### 结论登记（边界与差异）

- **`:60` 差异**：Go 用 nil 依赖返回字符串错误；Rust 拆成两层结构化失败关闭（未装配 bundle → 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`；已装配但无 provider/helper → 503 `MARKET_INDEX_CONSTITUENTS_UNAVAILABLE`），两半都已断言。
- **`:69` 差异**：Go 断言 `errors.Is(err, ErrCapabilityUnsupported)` 且消息含 `futu-opend`；Rust 以 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED` + 同一 provider 标签表达，helper 侧 `AKSHARE_UNSUPPORTED` 折叠进同类。
- **P2 差异**：Go `intValue` 对 float64 截断（300.7 → 300），Rust `bounded_integer` 直接 400 拒绝（fail-closed 收紧）；Go `inferMarketSymbol` 还能从 `query` 自由文本推断 instrument，Rust 走 `instrumentId`/`market`+`symbol`，模型 schema 仅公开 market/symbol。
- **归属后续批次**：`CN` 聚合与指数符号的其他启发式（如裸代码补前缀）归 `internal/integration/akshare`/`internal/marketdata` 批次；本批只实现 `CN.SH.xxxxxx`/`CN.SZ.xxxxxx` 叶子解析。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 29 条：`market_news_tools_test.go`/`mcp_server_lifecycle_authorization_test.go`（各 3），`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）等；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 活动读分页钳制差异（第 52 批 `:84`）、第 51 批 `:128` 的 instance summary/optimization no-op/validationInstrument 三个无 owner 断言、本批的 float 截断与 `query` 文本推断差异、`market.depth` 自由文本 instrument 推断、`watchlist.list includeQuotes` 行情富化、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1845 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2825 Rust** / **950 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十四批：assembly `market_news_tools_test.go` 全量结清（3 条；1 条新 `[x]`，2 条 `partial`）

范围：`internal/assistant/assembly/market_news_tools_test.go` 3 条逐条结清——本批新增 **1 条 `[x]`**（`:93`），`partial` 2 条（`:15`、`:104`）。`[x]` 计数 950 → **951**，Rust 测试 2825 → **2830**（新增 5 条：`product_mcp_market_news_tool_tests.rs`）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKMarketNewsAndCorporateActionsToolsForwardNormalizedInputs|TestADKMarketNewsAndCorporateActionsToolsFailClosedWithoutPorts|TestADKMarketNewsToolSurfacesProviderCapabilityAsClearMessage' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 个新测试文件，无生产改动）

Go 的 ADK 工具 `market.news`/`market.corporate_actions` 在 Rust 的等价名字是 `research.news`/`research.corporate_actions`（reviewed MCP 目录、catalog category=research、`jftrade-research` 技能），能力本体由生产新闻/公司行动端口提供。本轮把缺口补成 5 条断言（新文件 `crates/jftrade-engine/src/product_mcp_market_news_tool_tests.rs`，声明在 `product_mcp_server.rs` 的 `market_news_tool` 模块）：

- `market_news_tool_normalizes_market_and_limit_before_the_read_port`（`:15` 归一化半边）：缺 instrument → 400 `BAD_REQUEST`；`{"market":"us","symbol":"msft","limit":5}` 归一化为 `instrumentId=US.MSFT&limit=5` 后到达 `/api/v1/market-data/news`（夹具记录 query，`instrumentId=US%2EMSFT`）；两个工具的 catalog category=`research`、policy `read_internal`/`low`/无 per-mode 审批列表、归属内建技能 `jftrade-research`。
- `news_search_read_applies_the_reference_default_limit`（`:15` 默认值半边）：未给 `limit` 时 helper 收到 `GET /providers/yfinance/news/US/MSFT?limit=10`（`search_limit` 缺省 10、范围 1..50）。
- `corporate_actions_read_forwards_the_utc_normalized_window`（`:15` 公司行动半边）：`from=2025-01-01T08:00:00+08:00&to=2026-01-01T00:00:00Z` 转发为 `from=2025-01-01T00%3A00%3A00Z&to=2026-01-01T00%3A00%3A00Z`（等价 Go `optionalToolTime` 的 UTC 归一），身份保留 `HK.00700`；倒置/非法窗口由既有 `corporate_actions_query_requires_rfc3339_and_ascending_range` 断言 400 与精确消息。
- `market_news_and_corporate_actions_fail_closed_without_owner_ports`（`:93`）：未装配 bundle → 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`；已装配但未选提供商 → 503 `MARKET_DATA_NEWS_SEARCH_UNAVAILABLE` 与公司行动读 `Unavailable(active market-data provider is not configured)`，都不返回空列表。
- `corporate_actions_surface_the_capability_class_for_a_foreign_broker`（`:104`）：活跃 yfinance + `brokerId=futu` → 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED`「requested broker \"futu\" does not match active provider」；Futu 未就绪 → 503 且消息含 `Futu`/`not ready`（不借用 helper 的公司行动数据）。

### 探针（改坏 → 转红 → 回滚）

1. `research_news` 把 `instrumentId` 写回归一化前的 symbol → `market_news_tool_normalizes_market_and_limit_before_the_read_port` 转红（left `instrumentId=MSFT&limit=5` / right `instrumentId=US%2EMSFT&limit=5`）。
2. `parse_corporate_action_time` 直接回传原始串（不做 UTC 归一）→ `corporate_actions_read_forwards_the_utc_normalized_window` 转红（请求行变成 `from=2025-01-01T08%3A00%3A00%2B08%3A00`）。
3. 公司行动路由删掉 `provider_request_matches` 早退 → `corporate_actions_surface_the_capability_class_for_a_foreign_broker` 转红（不再是 409 capability）。
   3 处探针均在本批内执行并已回滚；`git diff` 只留测试文件、清单与报告。

### 结论登记（partial 行内的边界与差异）

- **`:15 partial`**：未迁移三点 —— ①Go 工具层拒绝 `limit>50`，Rust `search_limit` 按冻结 fixture 钳制 1..50（拒绝语义只在路由 `news_actions_helper_request`）；②Go `inferMarketSymbol` 支持 `query` 自由文本推断（`check us.msft headlines`），Rust 只接受 instrumentId 或 market+symbol；③Go 公司行动工具入参 `from`/`to` 在 Rust 无对应 ADK 工具，等价能力只在 market-data 路由上。
- **`:104 partial`**：能力类（409）与「指名提供商」都保留，但 Rust 措辞为 `futu`/`Futu` + `does not match active provider`/`not ready`，与 Go 的 `active provider "futu-opend" does not support instrument news` 不逐字一致；`news_actions_capability("Futu corporate actions market is unsupported")` 分支在路由层不可达（`normalize_news_actions_identity` 先以 400 拒绝非 HK/US/SH/SZ 市场）。已列 P2。
- **技能归属差异**：Go 把两个工具放进 `jftrade-market`（RequiredSkills），Rust 因 id 为 `research.*` 归入 `jftrade-research`；能力与权限类一致。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 26 条：`mcp_server_lifecycle_authorization_test.go`（3），`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）等；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 活动读分页钳制差异（第 52 批 `:84`）、第 51 批 `:128` 的三个无 owner 断言、第 53 批的 float 截断与 `query` 文本推断差异、本批的 `limit>50` 钳制/工具入参差异与 capability 措辞差异、`market.depth` 自由文本 instrument 推断、`watchlist.list includeQuotes` 行情富化、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1850 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2830 Rust** / **951 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十五批：assembly `mcp_server_lifecycle_authorization_test.go` 全量结清（3 条；2 条新 `[x]`，1 条 `partial`）

范围：`internal/assistant/assembly/mcp_server_lifecycle_authorization_test.go` 3 条逐条结清——本批新增 **2 条 `[x]`**（`:74`、`:95`），`partial` 1 条（`:27`）。`[x]` 计数 951 → **953**，Rust 测试 2830 → **2833**（新增 3 条，另含 1 处生产重构）。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestMCPServerManagerRemainingLifecycleBoundaries|TestMCPServerManagerRemainingServeFailureStates|TestMCPAuthorizedHandlerRemainingRequestBoundaries' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 处生产重构 + 3 条新回归）

Go 用可注入 `failingMCPListener` 断言 serve 失败状态；Rust 的 `axum::serve` 没有该 seam，于是把 worker 的错误发布判定抽成生产函数 `publish_listener_failure(state, generation, error)`（`crates/jftrade-engine/src/product_mcp_server.rs`），worker 与测试共用同一实现，行为不变：

- `unexpected_serve_failure_publishes_only_for_the_current_generation`（`:74`）：当前 generation 的失败写入 `MCP listener stopped unexpectedly: accept failed`；被替换的监听器（generation 已递增）失败既不写 `last_error` 也不替换当前 owner。
- `lifecycle_boundaries_report_the_missing_token_and_keep_same_port_applies_idempotent`（`:27` 可达半边）：token 模式缺 hash → 报错含 `token` 且 `last_error` 记录、`running=false`；未配置端口的状态投影端点为 `http://127.0.0.1:6697/mcp`；同端口重复 apply 幂等且清空 `last_error`。
- `authorization_boundaries_challenge_blank_bearer_and_reject_foreign_paths`（`:95`）：空 `Authorization: Bearer` → 401 且响应头 `www-authenticate: Bearer`；非 `/mcp` 路径 → 404；`/mcp` 未授权 → 401。
- 既有证据引用：`disabled_runtime_has_stopped_status_and_releases_listener`、`shutdown_is_idempotent_and_closed_runtime_rejects_rebind`、`port_conflict_keeps_previous_listener_and_reset_rebinds`、`loopback_policy_rejects_non_loopback_peer_addresses`、`host_rebinding_and_missing_host_are_rejected`、`cold_start_listener_failure_records_the_reason_and_recovery_clears_it`。

### 探针（改坏 → 转红 → 回滚）

1. token 校验改成恒假（`if false && …`）→ `lifecycle_boundaries_report_the_missing_token_and_keep_same_port_applies_idempotent` 转红（缺 hash 的 apply 不再报错）。
2. `publish_listener_failure` 去掉 generation 比较 → `unexpected_serve_failure_publishes_only_for_the_current_generation` 转红（陈旧失败写进 `last_error`）。
3. 401 响应删除 `WWW-Authenticate: Bearer` → `authorization_boundaries_challenge_blank_bearer_and_reject_foreign_paths` 转红（challenge 断言失败）。
   3 处探针均在本批内执行并已回滚；`git diff` 只留 `product_mcp_server.rs` 的重构、测试文件、清单与报告。

### 结论登记（partial 行内的边界与差异）

- **`:27 partial`**：未迁移的是「nil 接收者」三连（nil manager 的 Reconfigure/Status/Close、nil runtime 的 Reconfigure、`closeMCPHTTPServer(nil)`）——Rust 的 composition root 总是用 catalog 构造 runtime，没有可空 handle 路径；其余生命周期分支（缺 token、同端口幂等、禁用释放、Close 幂等 + closed 拒绝、端口冲突回退）均有断言。
- **`:74 差异**：Go 在 matched 失败时把 `server`/`listener` 置 nil；Rust 保留 owner 直到下一次 apply/shutdown，由 worker 翻 `running=false` 并保留 `last_error`，`status` 因此报告未运行且带原因。
- **`:95 差异**：Go 在「已禁用但 handler 仍在」的 manager 上断言 503；Rust 禁用即停止并释放监听器（`disabled_runtime_has_stopped_status_and_releases_listener`），不存在该 503 路径，属刻意收紧。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 23 条：`adk_capability_contracts_test.go`/`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）等。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 活动读分页钳制差异（第 52 批 `:84`）、第 51 批 `:128` 的三个无 owner 断言、第 53 批 float 截断与 `query` 文本推断、第 54 批 `limit>50` 钳制与 capability 措辞、第 55 批的禁用 503 路径差异、`market.depth` 自由文本 instrument 推断、`watchlist.list includeQuotes` 行情富化、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-strategy -p jftrade-store-sqlite -p jftrade-research --all-targets --locked --no-fail-fast`（1853 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2833 Rust** / **953 `[x]`**）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十六批：assembly `adk_capability_contracts_test.go` 全量结清（2 条 partial；新增 4 条回归 + 1 处生产校验修复）

范围：`internal/assistant/assembly/adk_capability_contracts_test.go` 2 条逐条结清。本批新增 **0 条 `[x]`**（两条 Go 测试的不可迁移半边是模型目录缺失的写工具，见下），`partial` 2 条；Rust 测试 2833 → **2837**，`[x]` 953 保持不变。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKNewCapabilityToolsForwardInputsAndApprovalBoundaries|TestADKNewCapabilityHandlersRejectUnavailableAndMalformedInputs' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 处生产修复 + 4 条新回归）

Go 这两条把新能力工具钉在「注册 → 入参转发 → 审批边界 → 缺端口/畸形输入一律报错」上：

- `strategy.research_backtest` 与 `strategy.optimize` 都经 `backtestStartInputFromMap` 解析 `tradingCosts`，字段存在但非对象时报 `tradingCosts must be a valid object`（Go `internal/assistant/assembly/tool_catalog.go:456`）。Rust 此前把 `tradingCosts` 原样克隆转发，既不看形状也不报错，属真实缺口，本批补齐：
  - `product_research_backtest_execution.rs::validate_trading_costs`（新）：缺失/`null`/对象接受（等价 Go `json.Unmarshal` 对 `null` 的折叠语义），其余类型返回 `tradingCosts must be a valid object`；
  - `execute_research_backtest` 在 script 校验之后、准备/启动 run 之前调用；
  - `execute_strategy_optimize` 在候选数量校验之后、入队第一个候选之前调用，保证拒绝时不排候选也不落 optimization task。
- 新增回归：
  - `product_research_backtest_execution.rs::research_script_validation_rejects_free_text`（Go `:94` 的 `script="not pine"` 半边）；
  - `product_research_backtest_execution.rs::research_trading_costs_stay_optional_but_must_be_an_object`（可选性与集合语义）；
  - `product_production_ports_adk_tests.rs::research_backtest_rejects_free_text_scripts_and_non_object_trading_costs`（工具级：自由文本脚本 + 非对象 costs）；
  - `product_production_ports_adk_tests.rs::strategy_optimize_rejects_non_object_trading_costs_before_enqueuing`（工具级：拒绝后 `started_definitions()` 为空、optimization task 表为空）。
- 既有证据引用（不重复造测试）：`product_mcp_server_tests.rs::strategy_instance_activity_tool_normalizes_kind_and_paging_before_the_read_port`（`:12` 的 activity kind/limit 归一化）、`product_production_ports_adk_tests.rs::strategy_optimize_is_gated_in_approval_mode_only`（`RequiresApprovalIn=[approval]` 且 `less_approval`/`all` 放行）、`product_mcp_server_tests.rs::production_mcp_local_tools_use_the_real_bundle_ports` 与 `production_tools_fail_closed_before_any_domain_service_is_configured`（注册 + 缺端口 fail-closed 503）、`backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets`（缺 id 400）。

### 探针（改坏 → 转红 → 回滚）

1. `validate_trading_costs` 的 `Some(_)` 分支改成 `Ok(())` → 3 条测试转红（`research_trading_costs_stay_optional_but_must_be_an_object`、`research_backtest_rejects_free_text_scripts_and_non_object_trading_costs`、`strategy_optimize_rejects_non_object_trading_costs_before_enqueuing`，最后一条打印出 queued 载荷里照抄的 `"tradingCosts": "cheap"`）。
2. 删掉 `execute_research_backtest` 的 `validate_trading_costs(arguments)?` 调用 → 工具级 research 测试转红。
3. 删掉 `execute_strategy_optimize` 的同一调用 → 工具级 optimize 测试转红。
4. `validate_research_script` 的 `if !validation.ok` 改成 `if false && !validation.ok` → 自由文本两条测试转红。
   4 处探针均在本批内执行并已回滚；`git diff` 只留两个生产文件的校验与三处测试、清单与审计产物。

### 结论登记（两条 partial 的边界）

- **`:12 partial`**：可达半边（目录注册、端口供数、activity 归一化、research/optimize 的 script+costs 校验与审批门）已断言；不可迁移半边是 Rust 模型目录 `PRODUCTION_TOOL_DEFINITIONS`（80 条）不含 `market.provider.select`、`strategy.instantiate`、`strategy.instance_start`、`strategy.instance_stop`、`strategy.instance_refresh_definition`、`strategy.instance_risk.update`、`backtest.cancel`——Go 在同一 registry 注册它们并断言 port 转发与 `RequiresApprovalIn` 非空，Rust 这些 id 无 descriptor/owner（`market.provider.select` 仅在 `tool_access_policy` 保留 write_settings/high/全模式审批元数据）。控制台经 `/api/v1/strategies/{instanceId}/{action}`、`/api/v1/backtest…` 等 HTTP 写路由完成同样的用户可见动作，模型侧不可调用。维持第 50/52 批登记的 P1 待办。
- **`:94 partial`**：可达半边（脚本/`tradingCosts` 畸形拒绝、缺 id 400、缺端口 503）已断言；差异①Go 空 `ToolDeps` 时 `system.runtime_dependencies` 返回 `{"status":"unavailable"}` 空载荷且无错误，Rust 端口未装配即 503、装配后返回 runtime-dependency 快照（`checkedAt`/`allRequiredSatisfied`/`dependencies`），无等价空载荷分支；②provider.select 的 scope/providerId、instantiate 的 binding、instance_risk.update 的 risk 取值校验在 Rust 无 owner；③P2：Go 逐字段解码 `tradingCosts`（字段类型错误即拒绝），Rust 只校验顶层对象形状后原样转发，字段级类型差异记入 follow-up。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 21 条：`adk_closure_contracts_test.go`/`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2）、`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`，本批再次确认含 `market.provider.select`/`backtest.cancel`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 本批 `tradingCosts` 字段级类型解码差异、活动读分页钳制差异（第 52 批）、第 51 批 `:128` 的 instance summary/optimization no-op/validationInstrument 三个无 owner 断言、`market.depth` 自由文本 instrument 推断、`watchlist.list includeQuotes` 行情富化、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1664 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2837 Rust** / 953 `[x]`，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十七批：assembly `adk_closure_contracts_test.go` 全量结清（2 条 partial；新增 3 条回归 + 1 处生产行为补齐）

范围：`internal/assistant/assembly/adk_closure_contracts_test.go` 2 条逐条结清。本批新增 **0 条 `[x]`**（两条的可迁移半边都有差异面，见下），`partial` 2 条；Rust 测试 2837 → **2840**，`[x]` 953 保持不变。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKToolDependencyClosuresForwardNormalizedOwnerPorts|TestADKToolDependencyClosuresFailClosedWhenPortsAreMissing' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 处生产行为补齐 + 3 条新回归 + 1 条既有回归扩容）

Go 这两条把「闭包工具」钉在两件事上：端口入参归一化后转发，以及 owner 端口缺失/报错时一律 fail-closed。

- 生产行为补齐：Go 的 `system.status` 工具在 `ToolDeps.ADKEnabled()` 为真时注入 `status["adk"] = {"module":"google.golang.org/adk/v2","enabled":true}`（`tool_catalog.go:199-203`；`ADKEnabled` 来自 `ApplicationAdapter.assistantEnabled = runtime != nil && runtime.Available()`）。Rust 的 `/api/v1/system/status` HTTP 投影刻意不提 ADK，因此本批在 ADK 工具层实现同样的合并：
  - `product_adk_tool_executor.rs` 新增 `system_status_with_adk_module(status, runtime_ready)`（纯投影，便于逐分支断言）与 `adk_runtime_ready()`（读 `AdkChatStreamPort::runtime_ready`，即 Rust 的 runtime Available 语义），并把 `system.status` 提为显式分支、把原先 fallback 里的 MCP 调用抽成 `mcp_execute` 供两处复用。
- 新增回归：
  - `product_adk_model_runtime::tool_executor::tests::system_status_publishes_the_adk_module_block_only_for_a_ready_runtime`（ready → 注入 module/enabled 且保留 `status`/`workers`；未 ready → 无 `adk` 键）；
  - `product_production_ports_adk_tests.rs::system_status_tool_stays_plain_without_a_configured_assistant_runtime`（端到端：生产 bundle 未配置可用模型 runtime 时不发布 `adk` 块）；
  - `product_mcp_production_executor_tests.rs::owner_port_failures_map_to_tool_failures_for_the_closure_readers`（`quote_error`/`watchlist_error`/`system_error` 对 `Unavailable` 的 503 + 稳定错误码 + 保留 owner 原因）。
- 既有回归扩容：`product_mcp_server_tests.rs::production_tools_fail_closed_before_any_domain_service_is_configured` 的待测清单加入 `system.futu_opend`/`market.snapshot`/`market.candles`/`watchlist.list`，未装配端口时四者都必须 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`。
- 既有证据引用（归一化半边）：`product_query.rs::candle_period_normalizes_aliases_and_rejects_unsupported`（60m/60min/k_60m → 1h，与 Go `pkg/broker/candle_period.go` 逐条一致）、`market_candles_compact_tool_forwards_market_symbol_period_and_limit`（工具层按设计原样转发，归一化落在读取端口）、`watchlist_list_normalizes_filters_and_rejects_out_of_range_limits`（groupName trim / market 大写 / limit 边界）。

### 探针（改坏 → 转红 → 回滚）

1. `system_status_with_adk_module` 的注入条件加 `false &&` → 单元测试转红（ready 分支不再写 `adk`）。
2. `adk_runtime_ready()` 恒返回 true → 端到端测试转红（未配置 runtime 也发布 `adk` 块）。
3. `watchlist_error` 的 `Unavailable` 错误码改成 `WATCHLIST_READ_FAILED` → 映射测试转红。
4. `market_candles` 在无端口时返回 `{"candles": []}` → 扩容后的 fail-closed 测试转红（打印 `domain tool without production ports must fail closed: Object {"candles": Array []}`）。
   4 处探针均在本批内执行并已回滚；中途一次「给 bundle 注入失败读取器」的尝试因生产 bundle 的读取器是具体适配器（`ProductionMarketDataQuotePort`/`ProductionWatchlistPort`，`with_market_data_quote_read_snapshot_port` 只喂 HTTP 路由）而撤回，回滚后 `git diff` 只留三处生产/测试改动、清单与审计产物。

### 结论登记（两条 partial 的边界与差异）

- **`:12 partial`**：`system.status` 的 ADK 块、`market.snapshot`/`market.candles`/`watchlist.list` 的归一化都已逐条对上；未迁移的是 ①`watchlist.list` 的 `includeQuotes=true` 转发——Rust 返回 `WATCHLIST_QUOTES_UNAVAILABLE`（第 47 批 P2，行情富化无 owner）；②`RecordWorkflowAudit` 回调——Rust 由 `product_adk_model_runtime_audit.rs::record_audit_event` 直接写 ADK 审计行，没有 ToolDeps 回调句柄。
- **`:73 partial`**：端口错误映射（本批新测试）与「未装配即 503」（扩容后的 fail-closed 循环）都已断言；未闭合的是「已装配但返回 error 的读取器 + 工具」这条组合路径不可注入；另有第 51 批已登记的刻意收紧：Go 在 `FutuOpenDHealth == nil` 时返回 `{"status":"unavailable"}` 载荷且无错误，Rust 选择 fail-closed。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 19 条：`adk_runtime_contracts_test.go`/`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2）、`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`，第 56 批确认含 `market.provider.select`/`backtest.cancel`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 第 56 批的 `tradingCosts` 字段级类型解码差异、`watchlist.list includeQuotes` 行情富化（第 47 批，本批再次确认）、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1667 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2840 Rust** / 953 `[x]`，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十八批：assembly `adk_runtime_contracts_test.go` 结清余量 2 条（`:156` 判 `[x]`；`:13` partial）

范围：`internal/assistant/assembly/adk_runtime_contracts_test.go` 未复核的 2 条（`:13`、`:156`；该文件的 `:113` 早在 strategy optimize 批次已记 partial）。本批新增 **1 条 `[x]`**，`[x]` 953 → **954**，Rust 测试 2840 → **2842**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKRuntimeStrategyToolsPreserveOwnerContracts|TestADKRuntimeResearchToolStopsBeforeRunWhenDataSyncIsPending' -count=1`（checkout `/tmp/452dea11.niwD1G` 同源 checkout，`ok`）。

### 本批结论（2 条新回归，无生产改动）

- `product_production_ports_adk_tests.rs::research_backtest_pending_data_sync_returns_the_sync_task_without_starting_a_run`（`:156` → `[x]`）：先跑一条 HK.00700 的真实研究回测记住 run 集合，再对缺数据的 HK.00001 调用同一工具，断言 `status=syncing_data`、`nextAction=wait_kline_sync`、`runId` 缺失/为 null，且 `ports.backtest_read.list()` 的 run 集合调用前后完全不变——等价 Go 的 `started` 标志（readiness 为 syncing 时在启动 run 前返回）。Go 的 `dataSync.taskId`/`nextTool` 等 payload 细节由同文件既有 `test_research_backtest_data_readiness_and_sync_lifecycle` 覆盖。
- `product_production_ports_adk_tests.rs::pine_validation_and_backtest_result_view_keep_their_owner_contracts`（`:13` 可达半边）：`strategy.validate_pine` 对骨架脚本返回 `ok=true` 且 `normalizedScript` 非空；`backtest.result_view` 返回的投影描述的就是请求的 run（`view.run.id == runId`、`view.view == "summary"`）。

### 探针（改坏 → 转红 → 回滚）

1. 把 `execute_research_backtest` 的 `EnsureDataOutcome::Syncing(syncing_resp) => return Ok(syncing_resp)` 改成继续 `start_research_backtest_run(...)` → 新 `:156` 测试转红（`failed to start research backtest: ... backtest K-line data is not ready`）。
2. 把 `backtest_result_view` 传给端口的 `run_id` 换成固定 `"probe-run"` → `:13` 的 result-view 断言转红（`backtest run was not found`）。
   2 处探针均在本批内执行并已回滚；`git diff` 只留测试文件与清单/报告。

### 结论登记（`:13 partial` 的边界与差异）

可达半边：`strategy.validate_pine`（本批新测试）、`strategy.definitions`（`production_mcp_local_tools_use_the_real_bundle_ports` 的 `definitions`/`definitionCount`）、`backtest.runs` 过滤（`backtest_runs_filters_resolve_nested_request_fields_and_top_level_status`、`backtest_runs_filters_match_the_go_definition_version_status_and_limit_matrix` 的 definitionId/status/definitionVersion/limit 与 `totalMatched`）、`backtest.result_view`（本批新测试）。
不可迁移/差异：①Go 的 `strategy.save_draft`（依赖收到 `Validation.Program`/`NormalizedScript`）、`strategy.save_definition`（`operation="created"` 且转发 `VisualModel`）、`strategy.update_instance_mode`（`updatedFields[0]="executionMode"`）三个模型写工具在 Rust 模型目录不存在（第 50/52 批登记的 P1「模型目录缺写工具」，只有 HTTP 写路由 owner）；②Go 的 `BacktestResultView` 是注入依赖并以 `runId` 回显，Rust 无注入依赖，生产投影把 run 身份放在 `run.id`（另带 `view`/`window`/`series`）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 17 条：`adk_summary_contracts_test.go`/`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2）、`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；本批再次确认 `strategy.save_draft`/`save_definition`/`update_instance_mode`）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `watchlist.list includeQuotes`（第 47 批）与 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1669 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2842 Rust** / **954 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第五十九批：assembly `adk_summary_contracts_test.go` 全量结清（1 条 `[x]` + 1 条 partial；新增 2 条回归）

范围：`internal/assistant/assembly/adk_summary_contracts_test.go` 2 条逐条结清。本批新增 **1 条 `[x]`**（`:33`），`partial` 1 条（`:9`）；Rust 测试 2842 → **2844**，`[x]` 954 → **955**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKStrategySummariesHideSourceDetailsAndCountLinkedInstances|TestADKBacktestSummariesRetainCountsWithoutEmbeddingRawSeries' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（2 条新回归）

Go 这两条把 ADK 摘要契约钉在两件事上：策略定义/实例摘要必须隐藏源码并统计关联实例数；回测 run 摘要必须保留计数、丢弃原始序列，并按定义/状态/limit 过滤。

- `:33 [x]`：新回归 `product_mcp_production_executor_tests.rs::adk_backtest_run_summaries_keep_counts_without_raw_series` 直接采用 Go 的 fixture（`initialBalance=100000`、`PnL=1500`、`TotalTrades=3`、1 根 candle、`logs=[started,completed]`、`runtimeErrors=[warning]`、`useExtendedHours=true`）：摘要投影产出 `tradeCount=3`/`candlesCount=1`/`latestLog=completed`/`totalReturn=0.015`、run 行带 `useExtendedHours=true`，并且 `run` 与顶层都不含 `result`、summary 不含 `candles`、`series` 为空；同一测试用 `filter_backtest_runs(definitionId=definition-1, status=queued, limit=1)` 断言 `runCount=1`/`totalMatched=1`/`truncated=false`/`runs[0].id=a`。
- `:9 partial`：新回归 `product_production_ports_adk_tests.rs::adk_strategy_definition_catalog_counts_the_seeded_definitions` 先经生产定义写端口（`strategy_definition_write`，Create）播种 `definition-1`，再走模型目录工具 `strategy.definitions`，断言 `definitionCount=1` 且 `definitions[0].id`/`name` 命中；空目录与 `definitionCount=0` 由既有 `product_mcp_server_tests.rs::production_mcp_local_tools_use_the_real_bundle_ports` 覆盖。

### 探针（改坏 → 转红 → 回滚）

1. `product_research_backtest_projection.rs::enrich_summary_payload` 的 `totalReturn = pnl / initial_balance` 改成常量 `0.05` → `:33` 测试转红（打印 `"totalReturn":0.05`）。
2. 同文件 `extract_case_summary` 的 `"candlesCount": lengths.candles` 改成 `0` → 同一测试转红（`left: Number(0)` / `right: 1`）。
3. `product_mcp_production_executor.rs::strategy_definitions` 的 `definitionCount` 改成常量 `0` → `:9` 测试转红（payload 里 `definitions` 仍有 1 条、完整 `script` 也在其中）。
   3 处探针均在本批内执行并已回滚，`git diff` 只留两条新测试、清单与审计产物。

### 结论登记（`:9 partial` 的边界与差异）

- 可达半边：`strategy.definitions` 的 `definitions` + `definitionCount`（本批新回归 + 空目录既有回归）。
- 未迁移/差异：①`scriptPreview`（Go 截断 280 字符）与 `scriptBytes`/`visualNodeCount` 在 Rust 生产响应中不存在，`strategy.definitions` 直接回传存储行，连完整 `script`、`visualModelJson` 一起给到模型面——即模型面没有隐藏源码；②`linkedInstanceCount`/`instances`/`instanceCount`/`activeSymbolCount`/`latestLog` 在 Rust 模型目录没有 owner：策略实例只有 HTTP `GET /api/v1/strategies` 读取端口（`strategy_read`）与单实例活动工具 `strategy.instance_activity`（logs/audit），没有实例列表/摘要的模型工具。
- 已登记为 P1 差异，与第 50/51/52 批同一 follow-up（模型目录缺策略实例读写工具、定义摘要 wire 形状），非本批可结清范围。
- `:33` 与 Go 的表面差异（已写入清单结论）：Go 的 `SummarizeADKBacktestRuns` 在列表层逐 run 补计数；Rust 的列表 owner（`backtest.runs` → 端口 `list()`）只回传不含 `result` 的 run 行加 `runCount`/`totalMatched`，逐 run 计数只在 result_view 投影上生成，两边都不把原始 `result` 交给调用方。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 15 条：`typed_product_capabilities_test.go`/`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；本批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `watchlist.list includeQuotes`（第 47 批）与 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1671 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2844 Rust** / **955 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十批：assembly `typed_product_capabilities_test.go` 全量结清（2 条 `[x]`；新增 2 条回归）

范围：`internal/assistant/assembly/typed_product_capabilities_test.go` 2 条逐条结清。本批新增 **2 条 `[x]`**（`:11`/`:45`）；Rust 测试 2844 → **2846**，`[x]` 955 → **957**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestTypedProductCapabilitiesDriveFeatureAndAssistantSchemas|TestAssistantSchemasCoverProviderAndResearchExtensions' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`；`:11` 的 16 个子测试逐工具 PASS）。

### 本批结论（2 条新回归，均为既有实现的行为守卫）

这两条把「typed product capability 表」钉在四件事上：工具→feature id 一致、schema 闭合、operation 枚举一致、按 SchemaKind 的必填路由字段；外加 Provider/Research 扩展 schema 的字段边界。Rust 已经实现全部行为，本批补齐的是把清单钉死的回归，不需要改生产代码。

- `:11 [x]`：新增 `product_broker_capabilities_projection_tests.rs::typed_product_capabilities_drive_feature_ids_and_reviewed_schemas`。遍历与 Go 相同的 16 个 typed 工具，断言每个工具恰好命中一条 `FEATURE_SPECS` 且 `spec.id == tool`（Go 的 `broker.FeatureID` 常量值就是工具名，Rust 的 id/tool 同表同源）、`schema.additionalProperties == false`、operation 枚举逐项有序相等（`prediction.snapshot`/`prediction.depth`/`prediction.combo_eligible` 无 operation 属性）、Instrument→必填 `instrumentId`、PredictionDiscovery→`operation`、PredictionQuote→`accountId`/`mvc`/`legs`。
- `:45 [x]`：新增 `product_mcp_protocol_tests.rs::provider_and_research_extension_schemas_keep_their_reviewed_fields`。`research.screen_catalog`/`market.candles`/`market.depth`/`research.calendar` 四者 `additionalProperties=false`；`market.candles` 保留 `sessions`/`beforeTime`/`adjustment`/`startTime`/`endTime`；`research.calendar` 保留 `sort`/`stockScope`/`marketCapMin`/`optionVolumeMax`/`ivMin`/`ivRankMax`/`ivPercentileMin`，`sort` 枚举 [hot,market_cap,option_volume,iv,iv_rank,iv_percentile]、`stockScope` 枚举 [all,watchlist,position,special]，`marketCapMin.anyOf[0]` 为 minimum=0 且无 maximum、`ivMax.anyOf[0]` 为 minimum=0/maximum=100；`research.screen` 保留 `conditions` 且 operation 只允许 `stock_v2`。

### 探针（改坏 → 转红 → 回滚）

1. `FEATURE_SPECS` 里 `research.calendar` 的 id 改成 `research.calendar.v2` → `:11` 测试转红（`left: "research.calendar.v2"` / `right: "research.calendar"`）。
2. `prediction_quote_schema` 的 required 去掉 `"legs"` → 同一测试转红（`prediction.combo_quote must require legs` 并打印完整 schema）。
3. `calendar_numeric_filter_schema` 屏蔽 maximum 注入 → `:45` 测试转红（`{"minimum":0,"type":"number"}`，`left: Null` / `right: 100`）。
   3 处探针均在本批内执行并已回滚，`git diff` 只留两条新测试、清单与审计产物。

### 结论登记（与 Go 的表面差异）

- `:11` 的映射断言落在 Rust 的单一事实源上：Go 用 `productToolFeatureIDs`（init 里由描述表合并）与 `TypedCapabilityDescriptions()` 两处表，Rust 只有 `FEATURE_SPECS` 一张 `id`/`tool` 同源表，因此断言是「每个工具唯一命中一条 spec 且 `id == tool`」，没有为了照抄 Go 再建第二套映射。
- `:45` 的字段与数值边界逐字段相等；Rust 的 `calendar_numeric_filter_schema` 把 `minimum: 0` 放在 `anyOf[0]`（number 分支）并把上限只加在 `iv*` 家族上，与 Go 的 `anyOf[0].minimum/maximum` 读法一致。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 13 条：`watchlist_adapter_test.go`/`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例读写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；第 59 批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `watchlist.list includeQuotes`（第 47 批）与 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1673 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2846 Rust** / **957 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十一批：assembly `watchlist_adapter_test.go` 全量结清（1 条 `[x]` + 1 条 partial；新增 2 条回归 + 1 处生产行为补齐）

范围：`internal/assistant/assembly/watchlist_adapter_test.go` 2 条逐条结清。本批新增 **1 条 `[x]`**（`:72`），`partial` 1 条（`:24`），并补齐一处真实生产行为（分组解析）；Rust 测试 2846 → **2848**，`[x]` 957 → **958**。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKWatchlistListReturnsRealDataWithoutImplicitQuoteCalls|TestWatchlistToolAdapterUnavailableAndMissingGroupBoundaries' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`）。

### 本批结论（1 处生产行为补齐 + 2 条新回归 + 1 条既有回归同步）

Go 的助手适配器在列成员之前会先 `ListGroups`，再用 `resolveWatchlistGroup` 按 id 或大小写不敏感名字解析调用方的分组引用；解析不到就报 `watchlist group %q not found`，绝不会拿未知分组去列一个空页。

- 生产行为补齐：`watchlist_list`（product_mcp_production_executor.rs）此前把调用方的分组字符串原样当作 `groupId` 传给读取端口，Rust 的 store 只按 `member.group_id = ?` 精确匹配，因此带分组**名字**（Go 的常规用法）会静默返回空 items，未知分组也不会报错。现在改为先读 `/api/v1/watchlist/groups`，用新增的 `product_mcp_production_executor_helpers.rs::resolve_watchlist_group_id`（精确 id 或 `name.eq_ignore_ascii_case`）解析出存储 id，再以该 id 查成员；解析不到返回 404 `WATCHLIST_NOT_FOUND` 且不再触发 items 读取。工具层的错误映射表与读端口语义未动。
- `:72 [x]`：新增 `product_mcp_watchlist_tool_tests.rs::watchlist_list_fails_closed_without_ports_and_for_unknown_groups`——未装配端口的 executor → 503 `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`（等价 Go 的 `watchlist is unavailable`）；`group="missing"` → 404 `WATCHLIST_NOT_FOUND`，记录型端口只看到 1 次 groups 读取、没有 items 读取（等价 `resolveWatchlistGroup` 返回 false 后不 ListItems）。
- `:24 partial`：新增 `product_mcp_watchlist_tool_tests.rs::watchlist_list_serves_stored_groups_and_members_without_quote_enrichment`——production bundle 里播种分组 `US Tech` 与成员 `US.AAPL`，默认调用返回该分组、`group="us tech"` 返回 1 条成员，两者都不含 quotes/quoteErrors（不隐式取行情）；`includeQuotes:true` 以 503 `WATCHLIST_QUOTES_UNAVAILABLE` 作为已登记边界断言。
- 既有回归同步：`product_tool_catalog_parity_tests.rs::watchlist_list_normalizes_filters_and_rejects_out_of_range_limits` 的期望读序列更新为「groups(limit=20) 解析 → items(groupId=group%2Dtech&...)」，并新增未知分组 404 与 `groups[0].groupId` 断言；limit 越界与 includeQuotes 仍在触达端口前被拒（读计数 3 不变）。

### 探针（改坏 → 转红 → 回滚）

1. 解析结果改回原样透传（`let resolved = Some(reference.to_owned());`）→ `:72` 测试转红（未知分组不再 404，而是去列 items）。
2. `resolve_watchlist_group_id` 的名字比较改成大小写敏感（`name == wanted`）→ `:24` 测试转红（`group="us tech"` 解析不到 `US Tech`）。
3. `include_quotes` 分支改成 `false && include_quotes` → `:24` 测试转红（`includeQuotes:true` 不再 503）。
   3 处探针均在本批内执行并已回滚，`git diff` 只留生产补齐、两条新测试、既有回归同步、清单与审计产物。

### 结论登记（`:24 partial` 的边界与差异，均为 P2）

- ①`includeQuotes:true` 无行情富化 owner：Go 会走 BatchQuotes 并回传 quotes/quoteErrors/quotesObservedAt（快照源调用 1 次），Rust 直接 503 `WATCHLIST_QUOTES_UNAVAILABLE`（第 47/57 批同一 P2，本批不重复登记）。
- ②载荷字段形状：Go 默认路径带 `includeQuotes`/`sources`/`recentImports`/`checkedAt`，成员路径带 `group`/`nextCursor`；Rust 只回传读取端口的 `groups`/`items`(`nextCursor`)。字段级补齐归入同一条 P2，未在本批扩面。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 11 条：`workflow_bridge_contracts_test.go`/`workflow_tools_error_boundaries_test.go`/`maintenance_test.go`（各 2），`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与策略实例读写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；第 59 批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤、`workflow_runs.*` 过滤参数、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `watchlist.list` 的 includeQuotes 行情富化与载荷字段形状（第 47/57 批 + 本批 ②）、第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1675 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2848 Rust** / **958 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十二批：assembly `workflow_bridge_contracts_test.go` 全量结清（1 条 `[x]` + 1 条 partial；新增 2 条回归）

范围：`internal/assistant/assembly/workflow_bridge_contracts_test.go` 2 条逐条结清。本批新增 **1 条 `[x]`**（`:103`），`partial` 1 条（`:14`）；Rust 测试 2848 → **2850**，`[x]` 958 → **959**，未改生产代码。

### 冻结证据（Go）

`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestWorkflowManagerProjectsServiceCRUDAndRuns|TestWorkflowManagerRejectsUnavailableServicesAcrossOperations' -count=1`（checkout `/tmp/go452dea11.niwD1G`，`ok`；`:103` 的 12 个操作与 nil-service/closed-facade 子测试全部 PASS）。

### 本批结论（2 条新回归）

Go 这两条把 workflow bridge 钉在两件事上：manager 的 CRUD/分页/run 读取/缺失目标启动失败；以及服务缺失时**每个**操作都要报 unavailable。Rust 的对应 owner 不是模型工具（模型目录只有 `workflow.wait`），而是 `/api/v1/adk/workflows*` 读路由 + `AdkReadSnapshotPort`/`AdkMutationPort`。

- `:14 partial`：新增 `product_adk_store_parity_tests.rs::workflow_bridge_pages_lists_and_rejects_unknown_run_targets`。补齐 Go 的 ListWorkflows 分页信封（`page.limit=5`/`offset=0`/`total=1` + 单条 items）、GetWorkflow 单条读取、`ListWorkflowRuns`/`GetWorkflowRun` 对应的 durable log 投影（`logs[0].id/workflowId/triggerId/runId`、`page.limit=10`/`total=1`）、以及 `RunWorkflow("missing-workflow")`/`RunWorkflowTrigger("missing-trigger")` 的 404 `ADK_WORKFLOW_RUN_FAILED`/`ADK_WORKFLOW_TRIGGER_RUN_FAILED`（等价 Go 的 error + `Accepted=false`）。create/update/delete 与删除后隐藏仍由既有 `workflow_and_trigger_lists_hide_deleted_rows_after_create_and_delete`、`workflow_updates_keep_omitted_fields_and_apply_explicit_clears` 覆盖。
- `:103 [x]`：新增 `product_adk_workflow_bridge_tests.rs::workflow_bridge_operations_fail_closed_without_their_ports`（挂在 `product.rs`，因为 `product_adk_mutation_port.rs` 被集成测试 `tests/adk_mutations_compatibility.rs` 复用，挂在其下会让集成 crate 也编译该文件）。断言 4 条 workflow 读路由无端口 → 503 `ADK_READ_UNAVAILABLE`（message=`ADK read snapshot port is not configured`），8 条写路由无端口 → 503 `ADK_MUTATIONS_UNAVAILABLE`（message=`ADK mutation port is unavailable`）。

### 探针（改坏 → 转红 → 回滚）

1. `dispatch_adk_read` 的缺失端口错误码改成 `ADK_READ_FAILED` → `:103` 测试转红（`left: "ADK_READ_FAILED"`）。
2. `page()` 的 `"limit": limit` 改成 `"limit": default_limit` → `:14` 测试转红（`left: Number(100)` vs `5`）。
3. `run_workflow` 的缺失 workflow 分支从 404 改成 409 → `:14` 测试转红（`left: 409`）。
   3 处探针均在本批内执行并已回滚，`git diff` 只留两条新测试、清单与审计产物。

### 结论登记（`:14 partial` 的边界与差异）

- ①`GetWorkflowTrigger`/`GetWorkflowRun` 在 Rust 没有独立读路由（`ADK_READ_ROUTES` 无 `.../triggers/{triggerId}`、无 `.../runs/{runId}`）；store 级 `get_workflow_trigger`/`get_workflow_trigger_log` 存在且被既有回归直接断言。
- ②`ListWorkflowRuns` 的 workflowId/triggerId/status 过滤与「触发日志 active/page 过滤」在 `page()`/`workflow_logs()` 未实现（第 47/57 批同一 P1，本批不重复登记）。
- ③删除响应是 `{"deleted": true}`，Go 是带 `DeletedAt` 的实体。
- ④Go 的 workflow CRUD 模型工具族在 Rust 不存在（第 50 批同一 P1；模型目录只有 `workflow.wait`）。
- `:103` 的 closed-facade 半边由既有 `workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation` 覆盖（runtime 缺失 → 503 `ADK_WORKFLOW_RUNTIME_UNAVAILABLE`，调用落库 FAILED）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 9 条：`workflow_tools_error_boundaries_test.go` 2、`maintenance_test.go` 2、`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go` 各 1；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与 workflow CRUD 工具族（本批再次确认）、策略实例读写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；第 59 批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤与 `workflow_runs.*` 过滤参数（本批 ②）、`workflow.*` 单条读路由缺口（本批 ①）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = `watchlist.list` 的 includeQuotes 行情富化与载荷字段形状（第 47/57 批 + 第 61 批 ②）、第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1677 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2850 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十三批：`internal/assistant/assembly/workflow_tools_error_boundaries_test.go`（2 条）

### 范围与基线

- 目标文件：`internal/assistant/assembly/workflow_tools_error_boundaries_test.go`（`:52` `TestWorkflowToolsRemainingManagerErrorPropagation`、`:103` `TestWorkflowToolsRemainingSessionAndPayloadErrors`）。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestWorkflowToolsRemainingManagerErrorPropagation|TestWorkflowToolsRemainingSessionAndPayloadErrors' -count=1`，两条均 PASS。
- Rust 侧入口核对：`product_adk_mutation_port_parse.rs`（路由/载荷解析）、`product_production_ports_adk_mutation_workflows.rs`（workflow/trigger 写入）、`product_production_ports_adk_mutation_workflow_runtime.rs`（运行期 canvas 校验）、`product_adk_read_api.rs`（读端口失败映射）；挂载点选在 `product_production_ports_adk_tests.rs`（与 `product_adk_store_parity_tests.rs` 同父模块，可使用 `agent_validation_port`），未挂在 `product_adk_mutation_port.rs`（该文件被集成测试 `tests/adk_mutations_compatibility.rs` 以 `#[path]` 复用）。
- 新增测试文件：`crates/jftrade-engine/src/product_adk_workflow_tool_error_tests.rs`（4 条测试）。

### `:52 [~]`（管理器错误原样传播）

- 新增 `product_adk_workflow_tool_error_tests.rs::workflow_manager_failures_surface_verbatim_on_every_workflow_route`：读端口返回 `Failed{502, WORKFLOW_MANAGER_FAILED, workflow manager failed, retryAfter=3}` 时，4 条 workflow 读路由（workflows / workflow / triggers / workflow-trigger-logs）逐字带出 status/code/message/retryAfter；mutation 端口返回 `Failed{422, WORKFLOW_MANAGER_FAILED, workflow manager failed}` 时，8 条 workflow 写路由逐字带出 status/code/message 且 `ok=false`。零改写即为 Go「工具把管理器错误 `errors.Is` 原样返回」的端口级等价断言。
- nil 管理器 fail closed 半边不重复登记，由批次 62 的 `product_adk_workflow_bridge_tests.rs::workflow_bridge_operations_fail_closed_without_their_ports` 覆盖（4 条读 503 `ADK_READ_UNAVAILABLE`、8 条写 503 `ADK_MUTATIONS_UNAVAILABLE`）。
- 仍为 partial：Go 注册的 14 个 workflow 模型工具（`workflows.*` / `workflow_triggers.*` / `workflow_runs.*` / `workflows.run` / `workflow_triggers.run`）在 Rust 无对应工具族（模型目录只有 `workflow.wait`，第 50/62 批同一 P1），因此「工具 handler 透传」这一层无对象可断言。

### `:103 [~]`（会话与载荷错误）：本轮生产修正

- 生产缺口：Rust 的 workflow create/update 把 body 的 `canvasGraph` 原样写入 SQLite，Go 在写前就会失败——工具层 `decodeWorkflowCanvasGraph`（`internal/assistant/assembly/workflow_tools.go:358`）与 REST 绑定 `*WorkflowCanvasGraph`（`internal/api/assistant/workflow.go:58`，失败 → 400 `BAD_REQUEST` `invalid workflow payload`）。因此 `canvasGraph:"invalid"` 在 Rust 会被持久化，直到运行期才以 400 `invalid workflow canvasGraph: …` 失败，且已落库的坏图会污染读投影。
- 修正位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_workflows.rs` 新增 `validate_request_canvas_graph(body.get("canvasGraph"))`（create/update 分支入口）与同名私有函数——仅校验请求值（非 null 时用 canvas 运行时同一 `jftrade_assistant::WorkflowCanvasGraph` 反序列化），失败映射 400 `BAD_REQUEST` / `invalid workflow canvasGraph: {serde error}`；`null` 仍表示清空、省略仍保值，历史行不被回写重判。

### `:103 [~]`（其余三段）

- `product_adk_workflow_tool_error_tests.rs::workflow_writes_reject_malformed_canvas_graphs_before_storing`：create 收到 `"invalid"`、`{"version":"v1","nodes":"invalid"}`、数组 → 400 `BAD_REQUEST` 且列表不出现被拒 workflow；省略 `canvasGraph` 的更新不报错、读投影无 graph；`{"version":"v1"}` 正常保存；对已存图的 workflow 提交 `canvasGraph:"invalid"` → 400 且存图仍是 `version=v1`。等价 Go 的 `decodeWorkflowCanvasGraph` 错误分支 + 省略不报错 + 合法图写入 `payload.CanvasGraph`。
- `product_adk_workflow_tool_error_tests.rs::workflow_writes_apply_the_documented_write_fields`：一次 create 带齐 `name/description/status/agentId/workMode/providerId/model/permissionMode/promptTemplate/objectiveTemplate/defaultInputs/tags`，断言落库与读投影逐字段一致（等价 `applyWorkflowWriteFields` 的 12 个拷贝点）。
- `product_adk_workflow_tool_error_tests.rs::workflow_trigger_update_switches_a_manual_trigger_to_a_schedule`：manual → `{"type":"schedule","config":{"cron":"* * * * *"}}` 后 type=schedule、config.cron 保留、省略 title 保值、enabled schedule 重算 `nextRunAt`（等价 `workflowTriggerUpdateRequest` 的非 webhook 分支；Go 的 webhook type 改动限制属工具层，REST 面是它指向的 UI/API，不迁移——同第 62 批 `workflow_tools_test.go:132` 登记）。
- Go-only 边界：`requireInteractiveWorkflowToolSession`（nil store / 缺失 session / 已关闭 store 必须报错）只存在于模型工具层；Rust 的 `workflows.run`/`workflow-triggers.run` 路由不要求交互式会话，因此这三段断言无对应对象，不作迁移实现。

### 探针（改坏 → 转红 → 回滚）

1. 删掉 `validate_request_canvas_graph(body.get("canvasGraph"))?;` 调用 → `workflow_writes_reject_malformed_canvas_graphs_before_storing` 转红（exit 100）。
2. `product_adk_read_api.rs::snapshot_failure` 的 `Failed` 分支把 `retry_after_seconds` 写成 `None` → `workflow_manager_failures_surface_verbatim_on_every_workflow_route` 转红（exit 100）。
3. 触发器更新把 `normalize_trigger_type(body.get("type"), …)` 改成 `normalize_trigger_type(None, …)` → `workflow_trigger_update_switches_a_manual_trigger_to_a_schedule` 转红（exit 100）。
4. 从 create/update 的 patch 键列表移除 `"providerId"` → `workflow_writes_apply_the_documented_write_fields` 转红（exit 100）。
   4 处探针均在本批内执行并已按字节回滚（脚本断言恢复后字节一致），suite 复测全绿。

### 结论登记（本批新增/确认的边界与差异）

- ①写期 canvas 校验比 Go 绑定更严一档：Go 的 `WorkflowCanvasNode` 允许缺 `id`/`type`（零值继续 normalize），Rust `WorkflowCanvasGraph` 反序列化要求 `id`/`type`，因此 `{"nodes":[{"data":{}}]}` 现在保存即 400（此前是保存成功、运行期 400）。判定为可接受收紧：Rust 运行期解析同一类型，坏图在存储前拒绝可避免污染读投影；登记为 P2 边界，若后续需要精确复刻 Go 的零值容忍，应在 `WorkflowCanvasNode` 加 `#[serde(default)]` 并在 normalize 阶段处理空 id/type。
- ②`:52` 的模型工具族缺口与第 50/62 批同源（P1），本批不重复展开。
- ③`:103` 的交互式会话守卫为工具层结构差异（P2，不迁移）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 7 条：`maintenance_test.go`（2）、`adk_backtest_adapter_test.go`/`adk_strategy_input_validation_test.go`/`adk_tool_failure_contracts_test.go`/`product_execution_contracts_test.go`/`workflow_execution_injection_test.go`（各 1）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest 分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与 workflow CRUD 工具族（第 62 批 + 本批 `:52`）、策略实例读写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；第 59 批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 的多账户 `accountSummaries` 聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照的生产 wire 形状、工作流触发日志 active/page 过滤与 `workflow_runs.*` 过滤参数（第 62 批 ②）、`workflow.*` 单条读路由缺口（第 62 批 ①）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`；P2 = 本批 ①（写期 canvas 校验比 Go 绑定更严）与本批 ③（交互式会话守卫不迁移）、`watchlist.list` 的 includeQuotes 行情富化与载荷字段形状（第 47/57/61 批）、第 56 批的 `tradingCosts` 字段级类型解码差异、第 57 批的 `RecordWorkflowAudit` 回调无 owner、`market.depth` 自由文本 instrument 推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` 的 intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 入参归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批的 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1680 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2854 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十四批：`internal/assistant/assembly/maintenance_test.go`（2 条）

### 范围与基线

- 目标文件：`internal/assistant/assembly/maintenance_test.go`（`:12` `TestDatabaseMaintenanceOwnsADKBusyPurgeAndCompactPaths`、`:77` `TestDatabaseMaintenanceFailsClosedWithoutOwnedRuntime`）。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestDatabaseMaintenanceOwnsADKBusyPurgeAndCompactPaths|TestDatabaseMaintenanceFailsClosedWithoutOwnedRuntime' -count=1`，两条均 PASS。
- Go 语义（读 `internal/assistant/assembly/maintenance.go`）：`DatabaseMaintenance` 是 engine `runtime` 的 owner-side 适配器——①`MaintenanceBusyReason` 由 `runtime.HasDatabaseActivity`（活动/暂停/等待审批的 run）派生，`"无法确认 ADK 运行状态"` 表示探测失败；②`PurgeMaintenanceCandidates` 只在 runtime 资源上生效，把分类 `智能体/工作流/触发器` 映射成 `DeletedConfigIDs`，未知分类不映射任何 id，`deleted != len(candidates)` 或 runtime 返回 `ErrCleanupCandidatesChanged` 都归一到 `dmsrv.ErrCleanupCandidatesChanged`；③`CompactMaintenanceResource` 对 runtime/session/artifact 分别走 `CompactDatabase`/`CompactSessionDatabase`/`CompactArtifactDatabase`，未知资源报错；④nil handle 时 busy reason 为空、compact/purge 报错。
- Rust 对应 owner：`crates/jftrade-store-sqlite/src/maintenance.rs`（`ManagedDatabaseMaintenanceStore`，写前取 `WriterLease`）、`data_management.rs`（`maintenance_candidates`：ADK 软删 agent/workflow/trigger + 被软删 workflow 的级联 trigger 查询）、engine 装配 `crates/jftrade-engine/src/product_data_management.rs`。既有测试：`crates/jftrade-store-sqlite/tests/maintenance_cleanup_candidates.rs`（仅候选列举 + 指纹 + busy 字符串注入）。
- 新增测试文件：`crates/jftrade-store-sqlite/tests/maintenance_adk_resources.rs`（2 条测试）。

### `:12 [~]`（purge + compact 已覆盖，busy owner 缺口登记 P1）

- 新增 `maintenance_adk_resources.rs::adk_soft_deleted_configs_purge_only_for_the_approved_candidate_set`：ADK 库播种 active agent、软删 agent、软删 workflow、live workflow、软删 trigger、挂在软删 workflow 下的 trigger；候选集恰为 `agent-deleted(智能体)`、`workflow-deleted(工作流)`、`trigger-deleted(触发器)`、`trigger-cascade(触发器)`；`execute_cleanup` → `deletedCount=4`、`compacted=true`，四行消失而 active/live 行保留；未知分类 `未来类型` 的候选 → `Err(Stale)`（等价 Go 的 `ErrCleanupCandidatesChanged`）且不删除背后的真实候选 `agent-late`。
- 新增 `maintenance_adk_resources.rs::adk_maintenance_compacts_every_owned_database_and_requires_the_writer_lease`：`DATABASE_ADK`/`adk-session`/`adk-artifact` 三个资源各自 `compact` → `compacted=true` 且 `databaseId` 回显。
- **P1 缺口（busy owner）**：Go 在 preview 与 execute 都因活跃 run 拒绝维护；Rust 生产装配不给 `CleanupPreviewService` 传 busy reason（`product_data_management.rs::cleanup_preview_service` 无 busy 端口），preview 不感知 run 状态，靠 `ManagedDatabaseMaintenanceStore` 的 `WriterLease` 兜底——引擎常驻持有 `AdkStore`（`_writer_lease` 字段）时，maintenance store 以独立 owner 取同一 lock 文件必然 `Conflict`。复现条件：open `AdkStore` 后对同一文件 `compact`/`execute_cleanup` → `Err(Conflict)`（本批测试断言）；`drop(store)` 后可成功。影响：①preview 行为与 Go 不同（Go 拒绝）；②引擎常驻的生产进程内 compact/purge 始终不可用（Go 通过 engine-owned 适配器在无活跃 run 时仍可维护）。修复方向：由 engine 暴露 owner-side maintenance 端口（busy reason 来自 ADK run 状态；compact/purge 走引擎自身连接与 lease），或实现显式 lease handoff；属于跨 crate 设计变更，本批只立测试与证据，不改生产行为。

### `:77 [~]`（nil handle 无同名对象，fail-closed 面已覆盖）

- Rust 生产对象没有 nil handle/nil runtime 形态，`unknown` 资源与「无 owner 不得维护」两面由 `adk_maintenance_compacts_every_owned_database_and_requires_the_writer_lease` 断言：`compact("unknown")` → `Err(Rejected)`；owner 存活时 `compact`/`execute_cleanup` → `Err(Conflict)`；owner 释放后恢复成功。
- Go 的 nil busy reason（空串）在 Rust 无对应物——busy reason 概念本身缺失，归入 `:12` 的 P1，不重复登记。

### 探针（改坏 → 转红 → 回滚）

1. `("adk","智能体")` 的删除语句改指 `adk_sessions` → `adk_soft_deleted_configs_purge_only_for_the_approved_candidate_set` 转红（exit 100）。
2. 删掉触发器候选 SQL 的 `OR workflow_id IN (SELECT id FROM adk_workflows WHERE ... deletedAt ...)` 级联子句 → 同上测试转红（exit 100）。
3. 把 `compact` 的 `let _lease = self.lease(descriptor)?;` 移到 `open_ready` 之后 → `adk_maintenance_compacts_every_owned_database_and_requires_the_writer_lease` 转红（exit 100）。
4. `verify_candidates` 直接返回 `Ok(())`（跳过 `verify_execute`）→ 未知分类用例转红（exit 101）。
   4 处探针均在本批内执行并按字节回滚，回滚后 suite 复测全绿。

### 校正

第六十三批小结与 `:52` 行结论里写的「1680 条 engine 测试」是当时 4 条新测试中第 4 条尚未加入时的实跑数；本批复核 engine 全量为 **1681**（无 engine 源码变更，仅为计数校正）。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 3 条：`adk_backtest_adapter_test.go` 1、`adk_strategy_input_validation_test.go` 1、`adk_tool_failure_contracts_test.go` 1；随后 `product_execution_contracts_test.go` 1、`workflow_execution_injection_test.go` 1；再进入 `internal/app/apiserver`（574，按 servercore/servercoretest/datamigration 等子域分片，注意 `internal/app/apiserver/datamigration/maintenance_test.go` 8 条）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与 workflow CRUD 工具族（第 62/63 批）、策略实例读写工具（第 52 批 `:122`；第 56 批确认含 `market.provider.select`/`backtest.cancel`；第 59 批补充策略实例读/摘要工具与定义摘要 wire 形状）、`portfolio.summary` 多账户聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照生产 wire 形状、工作流触发日志 active/page 过滤与 `workflow_runs.*` 过滤参数（第 62 批）、`workflow.*` 单条读路由缺口（第 62 批）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`、**ADK 维护缺 owner-side busy/lease handoff（本批新增）**；P2 = 写期 canvas 校验比 Go 绑定更严与交互式会话守卫不迁移（第 63 批）、`watchlist.list` includeQuotes 富化与载荷字段形状（第 47/57/61 批）、第 56 批 `tradingCosts` 类型解码差异、第 57 批 `RecordWorkflowAudit` 回调无 owner、`market.depth` instrument 文本推断、`research.calendar` 缺省输入 fail-closed 差异、运行期动态工具注册与可空句柄、`backtest.kline_sync_status` intervals/readyToRetry 字面量断言、research_backtest 内嵌 resultView 归一、per-agent 技能授权过滤、模型侧 memory/artifact 直接工具、第 53/54 批 float 截断/query 文本推断/limit>50 钳制/capability 措辞、第 55 批禁用 503 路径差异。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（141 passed）、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1681 passed）、`node scripts/quality/cargo-nextest.mjs run -p jftrade-datamanagement --all-targets --locked --no-fail-fast`（6 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2856 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十五批：`internal/assistant/assembly/adk_backtest_adapter_test.go`（1 条）

### 范围与基线

- 目标文件：`internal/assistant/assembly/adk_backtest_adapter_test.go`（`:8` `TestADKStrategyValidationAndVisualModelBoundaries`）。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKStrategyValidationAndVisualModelBoundaries' -count=1`，PASS。
- Go 语义（`strategy_tools.go:69` / `application_strategy.go:258` / `internal/strategy/visual_model.go:16`）：①`ValidateADKStrategyScript(tool, script)` 用 `strategypine.Compile` 校验，返回 TrimSpace 后的 `NormalizedScript`、`Program`、`Requirements`、`Warnings`；②`SourceFormatPineV6() == "pine-v6"`；③`strategyVisualModelFromInput` 把对象归一（engine 缺省 `logic-flow`、version 缺省 1、nodes/edges 补空数组、node `properties` 缺省 `{}`、edge `type` 缺省 `polyline`）；④非对象 → `visualModel must be a valid object`；⑤`blockKind` 为 `codeBlock`/`technicalIndicator` → `ErrUnsupportedLegacyDefinition`。
- Rust 侦察：`crates/jftrade-strategy/src/pinespec/mod.rs::validate_script`（`SOURCE_FORMAT`/`RUNTIME`）、`crates/jftrade-engine/src/strategy_pine_mcp.rs`（`strategy.validate_pine`/`strategy.pine_spec` 叶）、`crates/jftrade-engine/src/strategy_pine.rs`（`PINE_V6_SOURCE_FORMAT` 与 `/api/v1/strategy-pine/analyze`）、写入端口 `crates/jftrade-engine/src/product_production_ports_strategy.rs`（`visualModel`/`visualModelJson` 原样落库，无归一/拒绝）。

### 本批改动（补强既有回归，不新增测试文件）

- `crates/jftrade-strategy/tests/pine_mcp_contract.rs::validation_payload_matches_go_owner_field_set_and_defaults_requirements` 增加四条断言：`source_format == "pine-v6"`（字面量，对应 ②）、`normalized_script == EMA_SCRIPT.trim()`（对应 ①的归一回显）、`metadata.name == "EMA"` 与 `metadata.pyramiding == 2`（证明 program 已被编译，对应 ①的 `Program != nil`）；既有的 `requirements`/`saveHint`/payload 键集断言保留。
- 引擎侧模型工具面引用既有证据：`production_mcp_pine_leaves_execute_native_spec_and_validation`、`production_mcp_pine_validation_maps_bad_arguments_and_rejects_unsupported_scripts`、`tests/strategy_pine_compatibility.rs::strategy_pine_applies_input_validation_and_error_precedence_before_the_port`。

### `:8 [~]`（③④⑤ 为缺口，登记 P1）

- Rust 无视觉模型归一/legacy 校验 owner：写入端口把 `visualModel`（任意 JSON）序列化进 `visual_model_json`，或直接透传 `visualModelJson` 字符串，不补默认值、不拒绝 `blockKind: codeBlock/technicalIndicator`、不拒绝非对象；缺省值等价物在 Vue/TS 构建器（第 47 批 `:253` 已登记 boundary）。
- **新增 P1（公开写入契约校验缺口）**：Go 在 store 归一化层（`internal/store/strategy/normalize.go` → `NormalizeVisualModel`）对*任意*写入拒绝非对象 visual model 与 legacy blockKind（`ErrUnsupportedLegacyDefinition`/400），Rust 同等请求 200 落库。复现：POST/PUT `/api/v1/strategy-definitions` 带 `visualModel:"not-an-object"`，或 `visualModel:{nodes:[{id:"n1",type:"note",properties:{blockKind:"codeBlock"}}]}`。修复方向：在策略定义写入 owner（engine 端口或 jftrade-strategy 领域校验）补归一（engine/version/properties/edge type 缺省）+ legacy 拒绝并映射错误码，回归断言按本行 ③④⑤ 编写。
- 另注：Go 的 `strategy.save_draft` 工具在 Rust 不发布（`product_adk_store_parity_tests.rs:496` 明确禁止研究技能发布 `strategy.save_draft`/`strategy.save_definition`），因此「工具名参数化的校验入口」无同名对象；等价能力由 `strategy.validate_pine` 叶承担，已在上节覆盖。

### 探针（改坏 → 转红 → 回滚）

1. `validate_script` 的 `source.trim()` 改回 `source` → 该测试转红（exit 100，normalizedScript 含尾随换行）。
2. `SOURCE_FORMAT` 常量改成 `"pine-v6-legacy"` → 该测试转红（exit 100）。
3. `validate_script` 的 metadata 分支改成恒 `default_metadata()` → 该测试转红（exit 100，metadata.name 为空）。
   3 处探针均在本批内执行并按字节回滚，回滚后 suite 复测全绿。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 3 条：`adk_strategy_input_validation_test.go` 1、`adk_tool_failure_contracts_test.go` 1、`product_execution_contracts_test.go` 1、`workflow_execution_injection_test.go` 1（共 4 条，见下批目标）；随后 `internal/app/apiserver`（574，按 servercore/servercoretest/datamigration 等子域分片，注意 `internal/app/apiserver/datamigration/maintenance_test.go` 8 条）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 模型目录缺 9 个外部写/交易工具（第 50 批 `:29`/`:77`）与 workflow CRUD 工具族（第 62/63 批）、策略实例读写工具（第 52 批 `:122`；第 56/59 批补充）、**策略定义写入缺视觉模型归一与 legacy 拒绝（本批新增）**、`portfolio.summary` 多账户聚合/排序/partial、`portfolio.*` 无法投影 broker runtime `lastError`、策略定义版本/快照生产 wire 形状（含 `visualModelJson` 直出）、工作流触发日志 active/page 过滤与 `workflow_runs.*` 过滤参数（第 62 批）、`workflow.*` 单条读路由缺口（第 62 批）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState=approval_continuation_failed`、ADK 维护缺 owner-side busy/lease handoff（第 64 批）；P2 同前批（写期 canvas 校验更严、交互式会话守卫不迁移、`watchlist.list` includeQuotes、`tradingCosts` 类型解码、`RecordWorkflowAudit` 回调、`market.depth` 推断、`research.calendar` fail-closed、动态工具注册、`backtest.kline_sync_status` 字面量、resultView 归一、per-agent 技能过滤、模型侧 memory/artifact 工具、第 53/54/55 批各项）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-strategy --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast`（45 passed）、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1681 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2856 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十六批：`internal/assistant/assembly/adk_strategy_input_validation_test.go`（1 条）

### 范围与基线

- 目标文件：`internal/assistant/assembly/adk_strategy_input_validation_test.go`（`:10` `TestStrategyADKInputsAndSummariesEnforceBusinessBoundaries`，四个子测试）。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestStrategyADKInputsAndSummariesEnforceBusinessBoundaries' -count=1`，PASS。
- Go 语义：①`ValidateADKStrategyDraftScript` 对空白草稿 no-op；`ValidateADKStrategyScript("strategy.validate_pine","\t")` 报错含「非空」；`StrategyValidatePineToolPayload({"script":" "})` → ok=false、errors 恰 1 条含「必填」、saveHint.message 非空；②合法脚本 + `includeRequirements:false` → ok=true、requirements nil、normalizedScript 非空；③`StrategyMetadataPayload` 暴露 allowedEntryDirection/maxPositionSize/maxIntradayLoss{value,alertMessage}/maxConsLossDays{count,alertMessage}；④`SummarizeADKBacktestRuns` 对 QUEUED run 保留显式 `useExtendedHours:true` 且 `totalReturn` 为 nil（`summarizeADKBacktestRun` 在 `run.Result == nil` 时只回身份字段；`internal/backtest/result_view.go::resultViewSummaryPayload` 对无 result 的 run 直接回 `{}`）。

### 本批改动

- **生产修正**：`crates/jftrade-engine/src/product_research_backtest_projection.rs::enrich_summary_payload` 增加 missing-result 早退——run 没有非空 `result` 且当前摘要为空时原样返回 `{}`，不再用 request 的 initialBalance 造出 `quoteCurrency`/`totalReturn:0.0`（legacy 顶层摘要仍走原富化路径，避免影响既有兼容用例）。
- 新增回归 `product_mcp_production_executor_tests.rs::queued_backtest_summaries_keep_explicit_extended_hours_without_return_metrics`：queued run（无 result、显式 `useExtendedHours:true`）→ `run.useExtendedHours == true`、`summary == {}`、`series == {}`；列表路径 `filter_backtest_runs` 保留 `request.useExtendedHours == true` 且无 `totalReturn`。
- 补强既有回归：`crates/jftrade-strategy/tests/pine_mcp_contract.rs::validation_payload_matches_go_owner_field_set_and_defaults_requirements` 增加 ② 的 false 分支（ok/requirements None/normalized/metadata 四条）；`product_mcp_server_tests.rs::production_mcp_pine_leaves_execute_native_spec_and_validation` 增加 ① 的模型工具面空白脚本断言（ok=false、errors 含「必填」、saveHint 为对象）。
- ③ 由既有 `validation_metadata_projects_declared_risk_limits_like_go` 覆盖（逐字段核对一致）。

### `:10 [~]`（partial 边界）

- Go-only：`ValidateADKStrategyDraftScript` 的空草稿 no-op 与严格校验「报错含非空」属于未发布的 `strategy.save_draft` 工具层（Rust 明确不发布 `strategy.save_draft`/`strategy.save_definition`，见 `product_adk_store_parity_tests.rs:496`），Rust 对应物是 `strategy.validate_pine` 的校验载荷（errors/saveHint），文案为「必填」而非「非空」，无同名错误对象。
- 结构差异（前批已登记）：Go 的 `backtest.runs` 列表会对每个 run summarize（提升 `useExtendedHours`、附计数、缺 result 时不带 totalReturn），Rust 该工具返回原始存储行 + `runCount`；本批新增回归断言的是原始行语义。

### 探针（改坏 → 转红 → 回滚）

1. 删掉 `enrich_summary_payload` 的 missing-result 早退 → `queued_backtest_summaries_keep_explicit_extended_hours_without_return_metrics` 转红（exit 100，summary 出现 quoteCurrency/totalReturn）。
2. `validate_script` 的 requirements 恒为 `Some(...)`（忽略 include_requirements）→ strategy 契约测试转红（exit 100）。
3. 空脚本分支的 `errors` 清空 → `production_mcp_pine_leaves_execute_native_spec_and_validation` 转红（exit 100）。
   3 处探针均在本批内执行并按字节回滚，回滚后 engine 1682 / strategy 45 全绿。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 3 条：`adk_tool_failure_contracts_test.go` 1、`product_execution_contracts_test.go` 1、`workflow_execution_injection_test.go` 1；随后 `internal/app/apiserver`（574，按 servercore/servercoretest/datamigration 等子域分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 模型目录缺 9 个外部写/交易工具（第 50 批）与 workflow CRUD 工具族（第 62/63 批）、策略实例读写工具（第 52/56/59 批）、策略定义写入缺视觉模型归一与 legacy 拒绝（第 65 批）、`portfolio.summary` 多账户聚合、`portfolio.*` broker runtime `lastError`、策略定义版本/快照生产 wire 形状、工作流触发日志与 `workflow_runs.*` 过滤（第 62 批）、`workflow.*` 单条读路由（第 62 批）、Go `SaveRun` 终态谓词、审批续跑失败 `resumeState`、ADK 维护缺 owner-side busy/lease handoff（第 64 批）；P2 同前批，另加本批「`backtest.runs` 列表未 summarize」沿用既有登记。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1682 passed）、`node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast`（45 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2857 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十七批：`internal/assistant/assembly/adk_tool_failure_contracts_test.go`（1 条）

### 范围与基线

- 目标文件：`internal/assistant/assembly/adk_tool_failure_contracts_test.go`（`:19` `TestADKToolFailuresPreserveBusinessErrorContracts`，3 个子测试）。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestADKToolFailuresPreserveBusinessErrorContracts' -count=1`，PASS。
- Go 语义：①注入的 `MarketCandles`/`WatchlistList` 依赖在非法输入（`{}`、非法 `period`、`limit:0`）时绝不能被调用；watchlist 无依赖时报 "unavailable"、非法 limit 报 "between 1 and 200"；②`strategy.research_backtest` 在 readiness 出错时直接报错且不得入队，入队出错时报 "research queue unavailable"；`strategy.optimize` 在 `EnsureBacktestData` 出错时报 "optimization data source unavailable"；③`store.Close()` 后 `tasks.list`/`tasks.delete`/`memory.list`/`memory.forget` 四个工具都必须报错。
- Rust 侦察：`product_mcp_production_executor_helpers.rs::instrument`（缺键 "market is required"、空片段 "market and symbol are required"）、`product_production_ports_market_data_quote_reads.rs::read_candles`（period 在 provider 读取前归一/拒绝）、`product_mcp_production_executor.rs::watchlist_list`（limit 1..=200，无端口 503）、`product_research_backtest_execution.rs::start_research_backtest_run`、`product_strategy_optimize_execution.rs::execute_strategy_optimize`、ADK read/mutation 端口。

### 本批改动

- 新增模块 `crates/jftrade-engine/src/product_adk_tool_failure_boundary_tests.rs`（挂载于 `product_mcp_server.rs`）：
  - `market_candles_stop_missing_instrument_and_unsupported_period_before_provider_reads`：把真实 `ProductionMarketDataQuotePort`（provider history 为记录桩）装进生产 bundle，`{}` → 400/BAD_REQUEST，`period:"not-a-period"` → 400/BAD_REQUEST，且 history 端口 0 次请求、当前 K 线 0 次读取，落点等价 Go「依赖不得被调用」。
  - `research_backtest_surfaces_queue_failure_without_answering_a_run`：coverage 恒 true + 失败队列端口（`Failed("research queue unavailable")`）→ 工具报错含 "failed to start research backtest"/"research queue unavailable"，Start 恰好 1 次、Cancel 0 次。
- 补强既有回归：`product_tool_catalog_parity_tests.rs::watchlist_list_normalizes_filters_and_rejects_out_of_range_limits` 增加 limit 0/201 报文含 "between 1 and 200" 的断言（对齐 Go 子串）；`product_production_ports_adk_tests.rs::adk_routes_surface_durable_store_failures_instead_of_empty_success` 在故障前补种 `adk_tasks`/`adk_memory` 行并新增 `DeleteTask`/`DeleteMemory`（Go `tasks.delete`/`memory.forget`）断言：只能暴露存储故障，不得伪造 404。
- 无生产代码改动：①② 的 Rust owner 已满足 Go 语义，③ 由新增断言闭环。

### `:19 [~]`（partial 边界）

- 差异（P2 文案）：Go 缺键报 "market and symbol are required"，Rust 缺键报 "market is required"（仅空 `instrumentId` 片段才用 Go 文案）；无端口 watchlist 报文为 "production MCP ports are not configured"（Go 为 "unavailable"）。两者都在 provider/端口读取前 fail-closed。
- **本批新增 P1**：Go `strategy.optimize` 入队前执行 `EnsureBacktestData`（`tool_catalog.go:592` → `internal/backtest/data.go:38 EnsureDefinitionsData`：按候选定义解析、要求同 symbol/interval、取最小 queryStart/最大 endTime 与最大 warmup 后查覆盖并去重同步）；Rust `execute_strategy_optimize` 直接入队，缺 definition 级 readiness owner。复现：`strategy.optimize` + 无覆盖数据 → Go 回 readiness/syncing 载荷不建 run，Rust 直接建 run。修复方向：在 `product_strategy_optimize_execution.rs` 前补 definitions readiness（复用 `backtest_sync.check_coverage` 与同步去重）；回归测试按本批队列失败用例的 readiness 对应面补。

### 探针（改坏 → 转红 → 回滚）

1. `read_candles` 的 period 归一改成 `unwrap_or("1m")` → `market_candles_stop_missing_instrument_and_unsupported_period_before_provider_reads` 转红（exit 100，provider 端口被读取）。
2. `start_research_backtest_run` 的队列错误改成吞掉并返回伪 run → `research_backtest_surfaces_queue_failure_without_answering_a_run` 转红（exit 100）。
   2 处探针均在本批内执行并按字节回滚，回滚后 engine 1684 全量复测通过。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 2 条：`product_execution_contracts_test.go` 1、`workflow_execution_injection_test.go` 1；随后进入 `internal/app/apiserver`（574，按 servercore/servercoretest/datamigration 等子域分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 前批清单 + **`strategy.optimize` 缺 definition 级数据 readiness 门（本批新增）**；P2 = 前批清单 + 本批 `market.candles` 缺键文案与无端口 watchlist 报文措辞。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1684 passed）、`pnpm run check:zero-go`（2880 tracked files / 0 release artifact）、`pnpm run check:compatibility`（desktop runtime 3 profiles / 6 link cases / 10 facade commands / 4 events）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2859 Rust** / **959 `[x]`**，0 破坏引用，5 条已登记 partial 引用缺失 + 2 条既有空断言告警）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（含 `check:rust:target-health`，先按既有流程清理 `target/debug/deps/*.rcgu.o` 后通过）。

## 第六十八批：P1 修复——`strategy.optimize` 的 definition 级数据 readiness 门（驱动行 `adk_tool_failure_contracts_test.go:19` 子测试②）

### 范围与基线

- 目标：闭环第六十七批登记的 P1——Go `strategy.optimize` 在入队前跑 `EnsureBacktestData`，Rust 直接入队。
- Go 语义：`internal/assistant/assembly/tool_catalog.go:585-596`（`EnsureBacktestData(definitionIDs, startInput)`；错误 → 直接失败；`!Ready` → 返回 `backtestDataReadinessPayload` 且不建 run）→ `application_backtest.go:13 ensureBacktestData` → `internal/backtest/data.go:38 EnsureDefinitionsData`（逐候选取定义，缺失 → `ErrStrategyDefinitionNotFound`；`prepareResolvedBacktest` + `combinePreparedBacktests` 要求同 symbol/interval 并取 min queryStart/max endTime 与最大 warmup；覆盖齐 → Ready，缺失 → `ensureMissingCoverage` 以 `providerDataSyncKey` 去重起同步并回 `syncing_data`）；`data.go:267 backtestReadSessionScope` 由 `UseExtendedHours` 推出会话范围。
- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/backtest/ -run 'TestEnsureDefinitionsData' -count=1`（`service_test.go:406` 最大 warmup 语义）。

### 生产修正

- `crates/jftrade-engine/src/product_research_backtest_readiness.rs`：
  - `ReadinessContext` 拆出 `build`/`from_payload`/`assemble`，新增 `ensure_definitions_data_readiness`（无单一脚本的 readiness：候选最大 warmup + 复用 coverage/`SyncStateTracker`/去重同步）；
  - 既有 `format_syncing_response` 保留，新增 `format_definitions_syncing_response` 与 `syncing_payload` 调度（定义路径不带 research 专属 `scriptHash`/`validation`，保留 `marketDataProvider`/`dataSync`/`nextTool`/`nextAction`/`suggestedArguments`/`message` 与运行元数据）；
  - 会话范围改为 Go 语义：显式 `sessionScope` 优先，否则由 `useExtendedHours`（start payload 或 arguments）推出 `extended`/`regular`。
- `crates/jftrade-engine/src/product_research_backtest_execution.rs`：抽出 `apply_instrument_defaults`，新增 `prepare_derived_start_payload`（定义类工具复用 Go 的 symbol/market 缺省投影，原 `prepare_start_payload` 行为不变）。
- `crates/jftrade-engine/src/product_strategy_optimize_execution.rs`：`execute_strategy_optimize` 在候选循环前解析候选定义（`optimization_candidate_warmup_bars`：向 `StrategyDefinitionSnapshotPort::get` 传 `StrategyDefinitionPreview{symbol, interval, use_extended_hours}`，取 `derivedWarmupBars` 最大值，缺定义 → `strategy definition not found`）并调用 `ensure_definitions_data_readiness`；`Syncing` 时直接返回 readiness 载荷、不建任何 run。`optimization_start_payload` 补 `sessionScope`。

### 回归与既有用例

- 新增（`crates/jftrade-engine/src/product_adk_tool_failure_boundary_tests.rs`）：`strategy_optimize_stops_at_readiness_with_the_widest_candidate_warmup`（缺覆盖 → `status=syncing_data`/`nextAction=wait_kline_sync`、覆盖请求 warmup=600、`dataSync.since` 与同步请求 `since` 都等于 600 bar 推导值、Start 0 次、Cancel 0 次）、`strategy_optimize_queues_every_candidate_when_the_window_is_covered`（覆盖齐 → 2 个 run、无额外 Sync）、`strategy_optimize_reports_an_unresolved_candidate_before_queueing`（未知定义 → 报错且 0 入队/0 同步）。
- 既有用例适配：`product_production_ports_adk_tests.rs::optimize_bundle` 补装 `OptimizeCoverageReady`（覆盖恒 true）与 `OptimizeDefinitionSnapshot`（解析任意候选定义）——对齐 Go 测试里 `EnsureBacktestData` 由测试注入并回答 Ready 的接线；`strategy_optimize_enqueues_every_candidate_and_persists_the_task` / `strategy_optimize_rolls_back_candidates_and_validates_the_request` 语义不变。

### 探针（改坏 → 转红 → 回滚）

1. `ensure_context_data_readiness` 的覆盖短路改成恒 Ready → readiness/syncing 两测试转红（exit 100）。
2. `optimization_candidate_warmup_bars` 的 `max` 改成「首个非零生效」→ 最大 warmup 测试转红（exit 100）。
3. 缺定义的 `ok_or_else` 改成 `unwrap_or_else(默认 0)` → 未解析候选测试转红（exit 100）。
   3 处探针均在本批内执行并按字节回滚，回滚后 engine 1687 全量复测全绿。

### 仍未结清（下一批）

- `internal/assistant/assembly` 未复核余量 2 条：`product_execution_contracts_test.go` 1、`workflow_execution_injection_test.go` 1；随后进入 `internal/app/apiserver`（574，按 servercore/servercoretest/datamigration 等子域分片）、`pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 前批清单去除本批已闭环的 `strategy.optimize` readiness 门，其余（模型目录缺 9 个外部写/交易工具、workflow CRUD 工具族、策略实例/定义工具族、`portfolio.summary` 多账户聚合、`portfolio.*` broker runtime `lastError`、策略定义版本/快照 wire 形状、工作流触发日志与 `workflow_runs.*` 过滤、`workflow.*` 单条读路由、Go `SaveRun` 终态谓词、审批续跑 `resumeState`、ADK 维护 busy/lease handoff）不变；P2 = 前批清单 + 本批 `market.candles` 缺键文案、无端口 watchlist 报文措辞、就绪终态载荷形状（Go `backtestDataReadinessPayload` 含 error/nextAction 中文指令 vs Rust `nextAction=wait_kline_sync` 且终态直接报错）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1687 passed）、`pnpm run check:zero-go`、`pnpm run check:compatibility`、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2862 Rust** / **959 `[x]`**，0 破坏引用）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 第六十九批：`internal/assistant/assembly` 收尾 2 条（`product_execution_contracts_test.go:84`、`workflow_execution_injection_test.go:60`）

### 范围与基线

- Go 基线：`GOFLAGS=-mod=mod /opt/homebrew/bin/go test ./internal/assistant/assembly/ -run 'TestProductExecutionAdapterPreservesProductAndExecutionBoundaries|TestRuntimeUsesInjectedWorkflowExecutionForLoopChat' -count=1`，PASS。
- Go 语义：①`ProductExecutionAdapter` 的产品/执行边界——`market.capabilities` 用大写 market 查 capability；`research.news` 的旧路由字段 `tradingEnvironment` 不得进 provider params；`market.search` 读 schema 的 `pageSize`（默认 20、钳 1..100）并把 `query` 映射为 `keyword`；`market.snapshots` 保留符号原始大小写；`alerts.price.set` → customization `set` + payload；`execution.order_*/combo_*/buying_power` 走执行服务且 buying power 用 `FeatureExecutionBuyingPower`。②注入的 workflow executor 必须被 loop 模式 ChatStream 使用，其 sentinel 错误原样上抛。

### 生产修正（本批 2 处真实缺口）

- `crates/jftrade-engine/src/product_mcp_production_executor.rs::market_search`：原先只读 `limit`，而评审 schema 与 `normalize_legacy_mcp_arguments` 使用 `pageSize`（服务器把 `limit` 改写成 `pageSize`），导致调用方页大小被丢弃、回落默认 20；现优先 `pageSize`、缺省兼容 `limit`、默认 20、范围 1..100。
- `crates/jftrade-engine/src/product_mcp_production_executor_research.rs::research_news`：Go 把路由字段放在 typed query、provider params 只带业务参数；Rust 原先把全部剩余参数拼进 provider query，现排除 `tradingEnvironment`/`accountId`/`featureId`/`cursor`（brokerId/market/instrumentId/limit/pageSize 仍由 Rust 新闻路由消费）。

### 回归

- `product_tool_catalog_parity_tests.rs::market_search_honors_the_schema_page_size_and_keeps_the_limit_alias`：pageSize=25 → `limit=25`、limit=7 → `limit=7`、缺省 → `limit=20`，且路径固定 `/api/v1/market-data/instruments`、`query=apple` 保留、不出现 `pageSize=`。
- `product_mcp_market_news_tool_tests.rs::research_news_never_forwards_the_legacy_routing_field`：`instrumentId=US.AAPL` 与 `limit=10` 保留，`tradingEnvironment` 不得出现。
- `product_adk_model_runtime_chat_turn_tests.rs::a_failed_workflow_execution_surfaces_on_the_loop_chat_path`：loop 模式脚本化模型调用 `workflow.wait`，注入失败执行器返回 sentinel → 执行器恰被调用一次，`toolCalls[0].status=FAILED`、`error` 等于 sentinel、`errorCode=TOOL_EXECUTION_FAILED`；同文件 `runtime_with_production_catalog` 参数改为 `Arc<dyn AdkToolExecutor>`（5 处调用点改 `executor.clone()`），语义不变。
- `[~]` 结论：`product_execution_contracts_test.go:84` 的 `market.capabilities` 在 Rust 是 provider readiness 投影（端口忽略 query），broker 能力矩阵另有 owner（`product_broker_capabilities_projection.rs` + `/api/v1/broker-capabilities`）→ 结构差异（P2）；`market.snapshots` 归一位置不同（Rust 在执行器归一大写、Go 交服务）→ 终态等价（P2）。`workflow_execution_injection_test.go:60` 的注入缝在 Rust 不适用（组合根启动接线，无运行期 `SetWorkflowExecutor`；`workflow.run` 不在模型工具目录，P1 已登记），行为面按上条 loop 路径固定。

### 探针（改坏 → 转红 → 回滚）

1. `market_search` 改回只读 `limit` → 页大小测试转红（exit 100）。
2. `research_news` 的排除表清空 → 路由字段测试转红（exit 100）。
   2 处探针均在本批内执行并按字节回滚，回滚后 engine 1690 全量复测全绿。

### 仍未结清（下一批）

- `internal/assistant/assembly` 已全部结清（0 条余量）；下一批进入 `internal/app/apiserver`（574 条），先 recon 分片（servercore/servercoretest/datamigration 等，`internal/app/apiserver/datamigration/maintenance_test.go` 8 条），再按 P0/P1 顺序推进；其后 `pkg/strategy`（342）、`pkg/backtest`（237）、`pkg/bbgo`（145）、`pkg/futu`（118，live_opend 放最后）、`internal/integration/akshare`（73）、`internal/integration/yfinance`（68）、`pkg/market`（56）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 前批清单（模型目录缺 9 个外部写/交易工具、workflow CRUD 工具族、策略实例/定义工具族、`portfolio.summary` 多账户聚合、`portfolio.*` broker runtime `lastError`、策略定义版本/快照 wire 形状、工作流触发日志与 `workflow_runs.*` 过滤、`workflow.*` 单条读路由、Go `SaveRun` 终态谓词、审批续跑 `resumeState`、ADK 维护 busy/lease handoff）；P2 = 前批清单 + 本批 `market.capabilities` 结构差异、`market.snapshots` 归一位置、`strategy.optimize` readiness 终态载荷形状（第 68 批）。

验证：`cargo fmt --all`、`cargo clippy -p jftrade-engine --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（1690 passed）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2865 Rust** / **959 `[x]`**，0 破坏引用、0 重复 rust_entry）、`pnpm run check:compatibility`、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

> 干扰说明（客观记录，不计为通过）：本批执行期间工作树出现了**非本批**的并发改动——`scripts/compatibility/audit_test_parity.py` 与 `scripts/compatibility/test_audit_test_parity.py`（11:47 落盘，域矩阵重分类 + 自测）以及随之重生成的 `docs/history/go-to-rust/test-parity-report.md`/`test-parity-inventory.md`。这些改动属于其他进行中的工作，本批**未纳入提交**、也未回退。因此 `pnpm run check:zero-go` 在本批末尾转红，且 `pnpm run check:quick` 未复跑至绿；转红原因经核对完全来自上述脚本中新引入的字面量（`internal/pineworkerassets`、`internal/marketdataassets`，命中 zero-go 的资产目录规则），与本批 crates 改动无关。本批 crates 侧证据（fmt/clippy/nextest 1690 全绿、审计 0 破坏引用、架构检查、diff --check）在上述并发改动出现前完成；`check:zero-go`/`check:quick` 需在并发工作落地后复跑确认。

## 第七十六批：`internal/assistant/engine` 执行声明/租约/恢复族与 `workflowexec/**` 全量收口（165 条）

### 范围与结果

- 范围：`internal/assistant/engine` 剩余 165 条 `missing`——执行声明/租约/恢复族（`exec_bounds` 8、`execution_claims` 7、`execution_claim_failure_boundaries` 4、`execution_state_projection_contracts` 4、`persistence/execution_claims` 4、`persistence_failure_boundaries` 7、`runtime_execution_lease_boundaries` 7、`store_recover` 4、`runner_approval_concurrency` 4、`runner_continuation_boundaries` 4、`runner_lifecycle_reconciliation` 4、`session_context_stale` 7）与工作流/目标族（`workflow_agent` 9、`workflow_goal` 9、`workflow_compiler` 4、`workflow_reconcile` 4、`workflow_canvas` 5、`workflow_agent_native_integration` 4、`workflowexec/**` 66）。
- 结果：`[x]` 1154 → **1190**、`missing` 2597 → **2432**、`partial` 470 → **568**、`boundary` 226 → **257**。本批 165 条 = **36 `[x]`（全部 function_exact）+ 98 partial + 31 boundary**；`internal/assistant/engine` 的未核对余量从 433 条降到 **268 条**（119 个文件）。
- 本批**未改任何 crate 实现或断言**，只在 14 个 Rust 文件里为新 `[x]` 补 36 行 `// Parity:` 注释锚点（见下），因此没有"先红后改"探针；差异以结论形式登记在清单与本节。

### 执行声明/租约/恢复族（99 条：36 `[x]` + 59 partial + 4 boundary）

- 主要证据面：`crates/jftrade-assistant/tests/assistant_claims_runtime_contracts.rs`（run/tool 租约的过期、围栏、接管、心跳与重放；输入/审批结清的幂等与冲突语义）、`crates/jftrade-assistant/src/claims.rs` 与 `src/runtime.rs` 的模块内用例（fail-closed 陈旧声明、sibling 审批续跑一次、终态不可恢复）、`crates/jftrade-engine/src/product_adk_model_runtime_*`（工具取消 join、失败工具调用持久化、会话上下文/压缩、终态审计、租约接管围栏、continuation supervisor 的 claim/shutdown 语义）、`crates/jftrade-store-sqlite/tests/adk_run_lifecycle_cas.rs` 与 `adk_atomic_projection_events.rs`（终态单调、暂停字段经陈旧写者存活、投影与会话事件原子提交）。
- `[x]` 代表性锚点：`run_lease_expiry_fencing_and_stale_release_are_rejected`、`completed_tool_output_replays_after_checkpoint_restore`、`stale_tool_completion_is_rejected_after_keyed_takeover`、`input_resolution_is_idempotent_and_conflict_safe`、`approval_resolution_is_idempotent_and_conflict_safe`、`sibling_approvals_resume_once_after_every_decision`、`terminal_run_cannot_be_resumed`、`a_cancelled_tool_call_reports_the_context_cancellation`、`challenge_continuation_supervisor_concurrent_spawn_shutdown_race`、`a_cancelled_run_audits_run_cancelled_and_terminates_once`、`paused_workflow_run_keeps_accepting_progress_and_terminal_updates`、`user_goal_pause_fields_survive_a_stale_writer_and_clear_on_explicit_resume`、`production_adk_goal_pause_and_resume_are_persisted_atomically`、`workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation`。
- 记 `partial`/`boundary` 的共同原因：Go 的这些用例驱动的是 engine 内部函数粒度（`persistRunTerminalState`、`startRun` 的租约声明失败、reconcile 子运行、`sessionContextProjection` 的小响应阈值等），Rust 把同一不变量分散到"运行生命周期 CAS + 端口错误分类 + ADK 路由错误矩阵"三个 owner 上，无法逐条一一映射；差异已在每行结论中写明 Rust 已覆盖什么、为何不同。

### 工作流/目标编排族（66 条 workflowexec，全部 `[~]`：39 partial + 27 boundary）

- Go 的 `WorkflowExecutor` 是一层**Go 专属编排**：goal 决策（`goal.complete`/`goal.continue`）、子运行启动/收口（`StartWorkflowChildRuns`/`RunChild`/`EnsureWorkflowChildrenFinalReplies`）、workflow plan 持久化与最终广播（`PrepareWorkflowParent`/`FinalizePlannedWorkflow`）、workflow task toolset（`claim`/`complete`/`delegate`/`merge`、runtime task 上限 `maxRuntimeWorkflowTasks`）、`reconcileWorkflowChildren` 与审批阻塞聚合 `workflowCompletionBlockers`。
- Rust 没有同形 owner：loop/goal 运行由 ADK 模型运行时执行，运行状态由生命周期 CAS 拥有，暂停/恢复由 pause/resume mutation 拥有，任务由 ADK 任务 CRUD 拥有，画布子运行由 `adk_workflow_*` 端口拥有。因此 66 条按"Rust 已覆盖的运行/审批/画布语义 + 缺失的编排语义"逐条记为 partial（39）或 boundary（27），没有一条被强行批准为等价。
- 代表性 Rust 证据面：`product_adk_mutation_product_tests.rs::goal_pause_rejects_child_runs_and_resume_reports_missing_runs`、`product_adk_store_parity_tests.rs::goal_pause_and_resume_mutations_own_the_pause_lifecycle_fields`、`product_adk_model_runtime_catalog_policy_tests.rs::gated_call_persists_the_go_approval_projection`、`product_production_ports_adk_tests.rs::adk_routes_surface_durable_store_failures_instead_of_empty_success` / `resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window`、`adk_workflow_canvas_contracts.rs::approval_and_running_nodes_suspend_then_resume_the_same_durable_request`、`crates/jftrade-assistant/src/workflow.rs::TaskGraph`（依赖/环/ready-claim-complete）。
- 明确保留的 Go-only 语义（已在清单逐行写明）：`iteration_limit` 暂停与持久化、中断内部工具调用剪枝（Rust 以 FAILED 标记替代）、runtime task 上限、goal decision 工具族、workflow completion blockers、task.delegate/merge 与父运行暂停投影、native task graph 的 prepare/compile/runner 装配层。

### 清单定义修正（审计驱动，本批内完成）

- 本批前段曾把同一 Rust 测试同时批准给两条 Go 行，触发审计 `[x] mappings must use unique Rust test entries; duplicate references=18`，违反"[x] 必须引用真实且唯一的 Rust 测试"这一清单定义。修正方式：16 个重复组各保留 1 条 `[x]`，其余改为 `[~] partial` 并在结论中写明"共享哪条证据、差异在哪"。
- 其中两条**第七十五批已批准**的行（`internal/assistant/engine/runner_chat_test.go:490`、`runner_continuation_boundaries_test.go:11`）在本批被误改到冲突锚点，已按 HEAD 字节恢复原结论与锚点；对应的两条本批行改记 partial（`execution_state_projection_contracts_test.go:44`、`runner_approval_concurrency_test.go:154`）。
- 修正后审计：`OK: 1190 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry；`partial` 中 8 条无解析引用全部是前批已登记的缺口（本批新增 0 条）。

### 仍未结清（下一批）

- `internal/assistant/engine` 余量 **268 条 / 119 文件**：根目录 211（`runtime_store` 10、`adk_store_edges` 8、`adk_tool_edges` 7、`completion_review` 7、`skill_reg*` 13、`runner_*` 15、`session_context_projection` 5、`sqlite_tools`/`store_business` 8 …），`persistence/` 22、`providers/` 15、`skillsruntime/` 8、`completionreview/` 4、`adk22regression/` 3、`usageprojection/` 3、`workflowruntime/` 2。下一批按 `providers`+`skillsruntime`+`persistence`（45 条）先分片，再收根目录余量。
- 跨批 follow-up 汇总：P0 无新增。P1 = 前批清单 + 本批新增 **Go workflow executor 编排层整体未迁移**（goal decision 工具、子运行计划/收口、workflow task toolset、runtime task 上限、审批 reconcile 与 completion blockers）——需要产品决策是否在 Rust 重建该层，或在清单中永久保留为边界；同时登记 `iteration_limit` 暂停、中断内部工具调用剪枝、`RUN_LEASE_CLAIM_FAILED` 错误码、跨连接声明级并发用例四项缺口。P2 = 前批清单 + workflowexec 内部 helper（plan/描述裁剪、任务排序、modelsList 包装层、`resultSummary` 回退文案）与"部分结果 + 错误并存"的任务工具返回形态。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run --workspace --all-targets --locked --no-fail-fast`（**3042 passed / 2 skipped**，锚点插入前）、补锚点后复跑 `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**1930 passed / 0 skipped**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1190 `[x]`**，missing 2432、partial 568、boundary 257；0 破坏引用、0 重复 rust_entry、未锚定告警 193）、`pnpm run check:compatibility`（278 OpenAPI operations / 18 route groups / 19 probes；desktop runtime 3 profiles / 6 link cases / 10 facade commands / 4 events）、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`，最后 `pnpm run check:quick`。

## 第七十七批：`internal/assistant/engine` 全量收口（268 条，engine 余量清零）

### 范围与结果

- 范围：`internal/assistant/engine` 全部剩余 268 条 `missing`（119 个文件）——`providers/` 15、`skillsruntime/` 8、`persistence/` 22（技能注册与运行时 schema、provider responses 模型/probe、SQLite session schema 与 artifact 持久化），根目录 211（`runtime_store` 10、`adk_store_edges` 8、`adk_tool_edges` 7、`completion_review` 7、`skill_reg*` 13、`runner_*` 15、`session_context_projection` 5、`sqlite_tools`/`store_*` 25、`workflow_*` 20 等），以及 `completionreview/` 4、`adk22regression/` 3、`usageprojection/` 3、`workflowruntime/` 2。
- 结果：`[x]` 1190 → **1196**、`missing` 2432 → **2164**（本批结清 268 条）、`partial` 568 → **768**、`boundary` 257 → **319**。本批 268 条 = **6 `[x]`（全部 function_exact）+ 200 partial + 62 boundary**。
- **`internal/assistant/engine` 未核对余量归零**：域内 626 条 = 205 `[x]` + 312 partial + 109 boundary，无 `missing`。至此 `internal/assistant` 的 `assembly`、`engine`、`workflowexec/**` 三个子域全部收口，剩余仅 `internal/assistant` 非 engine/assembly/workflowexec 的 79 条（根目录 64、`model/` 10、`workflow/` 5）。
- 本批只改证据清单与 5 个 Rust 文件的 6 行 `// Parity:` 锚点注释（见下），**未改任何生产实现或断言**，因此没有"先红后改"探针；差异以结论形式登记在清单与本节的缺口列表。

### 分片执行

- **A（45 条）**：`providers/` 15 + `skillsruntime/` 8 + `persistence/` 22。证据面为 `crates/jftrade-store-sqlite/tests/adk_session_store_contracts.rs`、`adk_artifact_store_contracts.rs`、`schema_migrations.rs`、`sqlite_query_plan_and_migrations_audit.rs` 与 `crates/jftrade-engine/src/product_production_ports_adk_mutation_provider.rs`。
- **B（100 条）**：`adk22regression`、`adk_runner_edges`、`adk_schema`、`adk_skill_edges`、`adk_store_edges`、`adk_tool_edges`、`approval_*`（含 `approval_state_guard`）、canvas provider overrides、chat idempotency、`completion_review`、`context_cache`、`error_identity`、`event_projection*`、`exec_state_bounds`、`goal_state_boundaries`、`google_exec_concurrency`、`google_execution_replay_guards`、`google_memory`、`google_runner_failure_diagnostics`、`handoff_notice`、`input_continuation_*`、`input_workflow`、`lifecycle_reconciliation_failures`、`normalize`、`observability`、`planner_identity`、`planner_toolset`、`projection_canvas_memory_contracts`、`provider_base_url`、`provider_headers`、`reasoning_effort_lifecycle`、`responses_model_runtime`、`responses_stream_projection`、`resumed_execution_recovery_boundaries`。
- **C（123 条）**：engine 根目录余量——`run_timeline`、`runner_chat_callbacks`、`runner_chat_continuation_signal`、`runner_chat_runtime_branches`、`runner_goal`、`runner_lifecycle_*`、`runner_plugin`、`runtime_store`、`session_compaction_boundaries`、`session_context_*`、`session_skill`、`session_wrap`、`skill_recover`、`skill_reg_fs`、`skill_reg`、`skill_registry_archives`、`skill_registry_http_sources`、`sqlite_dialector_boundaries`、`sqlite_tools`、`store_approve`、`store_async`、`store_audit_query`、`store_business`、`store_entity_lifecycle_edges`、`store_failure_normalization_boundaries`、`store_identity`、`store_lifecycle`、`store_maintenance_*`、`task_runner`、`taskset_biz`、`timeline_projection_helpers`、`tool_artifact_materialization`、`tool_registry_change`、`tool_schema_workflow`、`tools_net_transport_boundaries`、`tools_security`、`usageprojection/`、`workflow_agent_runtime_branches`、`workflow_approval*`、`workflow_child`、`workflow_execution_persistence`、`workflow_finalization_contracts`、`workflow_helpers_provider_failures`、`workflow_observation_projection`、`workflow_persistence`、`workflow_plan_boundaries`、`workflow_planner_runtime`、`workflow_resume`、`workflow_store*`、`workflowruntime/`。

### 6 条新 `[x]` 与锚点

- `adk22regression/native_runtime_test.go:86` → `adk_workflow_canvas_contracts.rs::restart_keeps_completed_nodes_and_the_inflight_request_identity`
- `chat_request_idempotency_test.go:48` → `product_adk_model_runtime_lifecycle_tests.rs::concurrent_first_delivery_creates_one_durable_run_and_event`
- `google_exec_concurrency_test.go:12` → `product_adk_model_runtime_fencing_tests.rs::fail_closed_lease_takeover_blocks_duplicate_tool_execution_and_stale_commit`
- `persistence/google_artifact_test.go:186` → `adk_artifact_store_contracts.rs::adk_artifact_store_lifecycle_and_restart_durability`
- `persistence/session_sqlite_schema_test.go:15` → `adk_session_store_contracts.rs::adk_session_store_rejects_missing_drifted_and_corrupted_go_databases`
- `persistence/session_sqlite_test.go:58` → `adk_session_store_contracts.rs::adk_session_store_lifecycle_and_restart_durability`

锚点写入 5 个文件共 6 行 `// Parity: go:452dea11:<go 文件>:<行> <测试名>`，写入后审计的"未锚定 function_exact"告警从 199 回到第七十六批基线 193（即本批 0 新增未锚定）。

### 新增缺口登记（功能缺失，保留）

- **P1｜provider 工具探测回写**：Go `TestProvider` 在连通性探测后再做一次带工具请求；失败时把 `capabilities.tools=false` 写回 provider 记录并在响应返回。Rust `product_production_ports_adk_mutation_runtime.rs::test_provider` 只做一次不带工具的连通性请求，返回存储中的 capabilities 且不写回。复现：对带工具请求返回 502 的 provider 调用 `POST /api/v1/adk/providers/{id}/test`。修复位置：该函数增加第二次带工具请求与 `store.upsert_provider` 回写；回归要求：新断言响应与落库 capabilities 均为 `tools=false`。驱动行 `runtime_store_test.go:362`。
- **P2｜工具输出 artifact 物化**：Go 把超阈值的研究类工具输出物化为 artifact 并回填引用，且在没有 artifact store 或保存失败时回退保留原输出。Rust 有 ADK artifact store（版本化、生命周期、重启持久，见 `adk_artifact_store_contracts.rs`）但没有物化/回填与回退路径。驱动行 `tool_artifact_materialization_test.go:24/52/73`。
- **P2｜ADK task runner 有界扇出**：Go 的 task set 以有界并发执行每个任务，且已取消批次仍带原上下文执行。Rust 无 task-set 执行器（工具调用按序，并发只在运行/审批围栏层）。驱动行 `task_runner_test.go:13/79`。
- **P2｜ADK planner runtime**：Go 的 `planWorkflowWithADK` 具备内存会话回退、会话查找/创建错误前置上抛、provider 执行失败上抛。Rust 计划由画布图与运行入口承担，无 ADK 规划会话层。驱动行 `workflow_planner_runtime_test.go:12`。
- **P2｜continuation-only 信号**：Go 按消息文本识别"仅继续"输入并写 `run.continuation_only` 审计；Rust 续跑只有显式 `request_input` 答复通道，无该分类器与审计种类。驱动行 `runner_chat_continuation_signal_test.go:9/22/52`。
- **P2｜内置目录启动刷新**：Go 重开旧库时刷新内置 agent 受保护字段并保留用户模型选择与 `CreatedAt`；Rust 内置 agent/技能为编译期常量目录，无迁移刷新步骤。驱动行 `runtime_store_test.go:117`。
- **结构差异（不迁移）**：GORM sqlite dialector 层（类型映射、clause 构造器、版本比较）在 Rust 由 rusqlite + 显式迁移与查询计划审计取代（`sqlite_dialector_boundaries_test.go`、`sqlite_tools_test.go` 4 条）；`workflowruntime` facade 装配由 engine composition root 承担（`workflowruntime/runtime_test.go` 2 条）；"非工作流父忽略子回调"的类型区分在 Rust 由运行负载 workflow 标识判定（`workflow_child_test.go:137`，按边界登记）。

### 仍未结清（下一批）

- 下一批（第七十八批）范围：`internal/assistant` 除已结清的 engine/assembly/workflowexec 之外全部余量 **79 条**——根目录 64（`workflow_crud` 10、`workflows` 7、`workflows_extended` 6、`service_business_helpers` 4、`service_business` 4、`service_contract_boundaries` 4、`service_lifecycle_boundaries` 4、`service_test` 4、`service_skill_state_recovery` 3、`workflow_async_tools` 3、`workflow_lifecycle` 3、`workflows_resource_recovery` 3、`service_audit_pagination`/`service_builtin_agent_edit`/`service_recovery`/`workflow_store_failures` 各 1）、`model/` 10（`provider_reasoning_config` 3、`workflow_plan` 3、`timeline_helper`、`timeline_reply_ordering`、`workflow_graph_resume_identity`、`workflow_task_tools`）、`workflow/` 5（`rules_test.go`）。该批结清后 `internal/assistant/**` 全域归零。
- 其后按域余量排序：`pkg/strategy` 333、`pkg/backtest` 237、`internal/store` 206、`internal/api` 180、`internal/strategy` 169、`pkg/bbgo` 145、`internal/integration` 141、`internal/marketdata` 112、`pkg/futu` 86、`internal/trading` 80（`internal/app/apiserver` 已在此前批次结清）。
- 跨批 follow-up 汇总：P0 无新增。P1 = 前批清单 + 本批 provider 工具探测回写。P2 = 前批清单 + 本批 artifact 物化、task runner 有界扇出、planner runtime、continuation-only 信号、内置目录启动刷新。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**1930 passed / 0 skipped**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1196 `[x]`**；missing 2164、partial 768、boundary 319、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线）、`pnpm run check:compatibility`（278 OpenAPI operations / 18 route groups / 19 probes；desktop runtime 3 profiles / 6 link cases / 10 facade commands / 4 events）、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（含 compatibility 全套与 pineworker 10 files / 98 tests）。

## 第七十八批：`internal/assistant` 全域收尾（79 条，助理域归零）

### 范围与结果

- 范围：`internal/assistant` 除已结清的 `engine/`、`assembly/`、`workflowexec/` 之外全部余量 **79 条**——根目录 64（`workflow_crud` 10、`workflows` 7、`workflows_extended` 6、`service_business_helpers`/`service_business`/`service_contract_boundaries`/`service_lifecycle_boundaries`/`service_test` 各 4、`service_skill_state_recovery`/`workflow_async_tools`/`workflow_lifecycle`/`workflows_resource_recovery` 各 3、`service_audit_pagination`/`service_builtin_agent_edit`/`service_recovery`/`workflow_store_failures` 各 1）、`model/` 10、`workflow/` 5。
- 结果：`missing` 2164 → **2085**（本批结清 79 条）、`partial` 768 → **846**、`boundary` 319 → **320**、`[x]` 保持 **1196**。本批 79 条 = **0 `[x]` + 78 partial + 1 boundary**（无新批准项，因此本批没有新增 `// Parity:` 锚点，未锚定告警保持 193 的前批基线）。
- **`internal/assistant/**` 全域归零**：域内 810 条 = **250 `[x]` + 447 partial + 113 boundary，0 `missing`**。至此助理/ADK 域（assembly、engine、workflowexec、service、workflow、model）全部完成逐项核对。

### 分片执行

- **A1（14 条）**：`service_test` 4、`service_audit_pagination` 1、`service_builtin_agent_edit` 1、`service_business_helpers` 4、`service_business` 4。证据面为 `product_production_ports_adk_tests.rs`（时间线错误码、未就绪运行时拒绝、agent 写入重校验、技能安装码）、`product_production_assembly_tests.rs`（内置目录投影、指标聚合）、`product_adk_read_tests.rs`（审计/运行过滤、快照 fail-closed）。
- **A2（17 条）**：`service_contract_boundaries` 4、`service_lifecycle_boundaries` 4、`service_persistence_runtime_boundaries` 5、`service_recovery` 1、`service_skill_state_recovery` 3。证据面为 `jftrade-settings::assistant_runtime`（设置缺省/边界）、`product_adk_store_parity_tests.rs`、`adk_workflow_canvas_contracts.rs`、`product_adk_model_runtime_tool_failure_tests.rs`（终态流终帧恢复）、`product_adk_mutation_product_tests.rs`（优化任务取消/负路由）。
- **B（35 条）**：`workflow_crud` 10、`workflows` 7、`workflows_extended` 6、`workflow_async_tools` 3、`workflow_lifecycle` 3、`workflow_store_failures` 1、`workflow/rules` 5。证据面为 `adk_workflow_canvas_contracts.rs`、`adk_workflow_canvas_adversarial.rs`、`adk_workflow_scheduler_contracts.rs`（worker 启停、计划创建与 tick、阈值上/下穿与冷却、行情错误韧性）、`product_workflow_cron.rs`、`product_workflow_threshold.rs`、`product_workflow_jobs.rs`、`workflow_cron_go_semantics.rs`、`product_adk_workflow_tool_error_tests.rs`、`product_adk_store_parity_tests.rs`、`product_production_ports_adk_mutation_workflow_runtime.rs`、`workflow_queue_atomicity.rs`、`crates/jftrade-assistant/src/workflow_canvas.rs`。
- **C（13 条）**：`model/` 10（provider 推理配置 3、时间线助手 1、时间线回复顺序 1、工作流图指纹 1、工作流计划 3、目标决策与工具契约 1）+ `workflows_resource_recovery` 3。证据面为 `product_production_ports_adk_mutation_provider.rs`、`product_adk_model_runtime_run_projection_tests.rs`、`adk_workflow_canvas_contracts.rs`、`product_production_ports_adk_mutation_workflow_runtime.rs` 与 `jftrade-assistant` 的 `workflow.rs`/`workflow_canvas.rs`/`model.rs`。

### 新增缺口登记（功能缺失，保留，均为 P2）

- **provider 推理配置解析层缺失**：Go 的 `provider_reasoning_config` 有三层行为——responses 预设只写 `reasoning.effort` 且不假设映射、显式空映射视为"不支持"、自定义映射按大小写保真校验并拒绝未知档位；另要求可选档位不得取 `default`。Rust 的 provider 写入只做超时/凭据归一化（`saved_provider_normalizes_the_request_timeout_on_write`、`saved_provider_hides_the_credential_from_the_row_and_projection`），运行期直接透传 `reasoningEffortOverride`，没有配置解析与校验层。驱动行 `model/provider_reasoning_config_test.go:8/27/65`。修复位置建议：`crates/jftrade-engine/src/product_production_ports_adk_mutation_provider.rs` 写入校验 + 运行时映射解析；回归要求：新增断言拒绝 `default`、未知档位报错、空映射解析为不支持。
- **目标决策工具层**（承接第 76 批 P1 登记）：`model/workflow_task_tools_test.go` 的目标决策状态机（nil 决策惰性、复位/阶段进入、目标提示保真）与 Go workflowexec 的 goal decision 工具族同源，Rust 无对应实现，本批以 partial 记录。
- 其余 76 条均为 service/workflow 服务层包装与 Go 内部助手函数：Rust 以端口 + 编译期目录表达同一职责，逐条差异已写入清单结论。

### 跨批 follow-up 汇总

- P0 无新增。P1 = 前批清单 + 第 77 批登记项（provider 工具探测回写 `capabilities.tools`）不变。
- P2 = 前批清单 + 本批 provider 推理配置解析层、可选推理档位 `default` 拒绝；工作流 cron/阈值/调度面的逐条边界断言已由 `adk_workflow_scheduler_contracts.rs`、`product_workflow_cron.rs`、`product_workflow_threshold.rs` 覆盖，无需新增修复项。

### 仍未结清（下一批）

- 下一批（第七十九批）范围：按域余量排序的下一块 **`pkg/strategy` 333 条**——先按文件分组 recon（`pkg/strategy/*` 与 `pkg/strategy/pine*` 等），再按 P0（策略实例/定义写入所有权、运行期状态与取消）→ P1（Pine 编译/校验、回测分页与超时）→ P2 顺序分片。其后：`pkg/backtest` 237、`internal/store` 206、`internal/api` 180、`internal/strategy` 169、`pkg/bbgo` 145、`internal/integration` 141、`internal/marketdata` 112、`pkg/futu` 86、`internal/trading` 80，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-assistant -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**1930 passed / 0 skipped**）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1196 `[x]`**；missing 2085、partial 846、boundary 320、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`。

## 2026-09-28 P1：workflow CRUD 分页与标签归一化

针对 `internal/assistant/workflow_crud_test.go:14:TestWorkflowResourceCrudPaginationAndLogs`，先红后修补齐四个可迁移行为：workflow `tags` 现在 trim、去空、去重并排序；ADK 列表请求的 `limit=200` 统一收敛到 100；workflow trigger logs 默认 limit 从 100 改为 20；空 type/title/status 的手动 trigger 回退为 `manual`/`手动触发`/`ERROR` 且列表投影不暴露 `secretHash`。新增 `workflow_resource_crud_normalizes_tags_and_pagination_defaults` 回归，修复后与 workflow bridge/软删除 owner 测试合计 nextest 3/3 通过，receipt 为 `sha256:98e9c06f94e261d4b0c53c0a7141ab81d861de627f5435ea45c07550092bee09`（[`p1-workflow-pagination-2026-09-28T081500Z.json`](verification-receipts/p1-workflow-pagination-2026-09-28T081500Z.json)）。

## 2026-09-28 P1：workflow CRUD 删除 wire 与日志过滤

对同一 Go 测试继续逐断言核对：先红确认 `workflow-trigger-logs` 忽略 `workflowId`/`triggerId`/`status` 过滤，随后在生产 read owner 按过滤条件缩小集合再分页；新增 workflow 更新后的 `status/name/workMode/tags` 与负 offset 归一断言；删除 workflow/trigger 逐字段确认 `DISABLED`、`deletedAt`、重复删除失败以及删除后单条 workflow 读取失败；补齐 schedule 非法 cron 与 market-threshold 缺 numeric value 的 400 分支。定向 engine nextest 3/3 通过，receipt 为 `sha256:1a5f94840bd784348b30d979083cac9e64b2bee2be0a9bdb7b7bef961d1cb4df`（[`p1-workflow-crud-filter-delete-2026-09-28T090634Z.json`](verification-receipts/p1-workflow-crud-filter-delete-2026-09-28T090634Z.json)）。映射继续保持 `partial`：webhook/manual 运行错误矩阵与完整删除 wire 仍需更多逐字段证据，未机械升级 exact。
## 2026-09-26 provider probe completion

本节更新第七十八批的历史 follow-up：当时登记为 P1 的 provider 工具探测回写缺口现已由真实生产端口与 loopback provider 回归关闭；此前批次中关于 ProbeProvider quick/full 未实现的表述仅保留为历史状态。

- `TestProbeProviderQuickAndFullRequestCounts`、`TestProbeProviderWithoutMappingsSendsNoReasoningField` 与 `TestProviderProbeTimeoutCapsConfiguredRequestTimeout` 现为 `[x]`/`function_exact`；Rust 覆盖 full 的 low/medium/high 采样、quick 的 canonical effort、空 mappings 不发送 reasoning，以及 30 秒 timeout cap。
- `TestRuntimeTestProviderMarksToolsUnsupportedWhenSelectionFails` 现为 `[x]`/`function_exact`；TestProvider 将 streaming/tools/reasoning 能力投影并持久化，工具探测失败时 `capabilities.tools=false` 不再停留在旧值。
- 受影响 engine 回归经 nextest wrapper 运行 8/8 通过（1959 项跳过）。当前审计基线为 Go 4451、Rust 3366、`function_exact` 1491、`partial` 2332、`boundary` 628；Parity 锚点 1780/1734/0/0/46。

仍未完成的 provider safe-HTTP、completion-review 与 workflow executor 编排层条目不受本节影响，继续按清单中的 P1/P2 owner 与边界结论推进。

## 2026-09-27 strict evidence batch

本批人工复核 Assistant/ADK 的 11 条单引用 `function_exact`：approval resolution、metrics query、optimization cancellation、legacy route、resolved approval projection、pagination、approval idempotency、malformed query、missing mutation targets 与 catalog fault contracts。每条均引用已有 Parity anchor 和真实 workspace receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`；session-negative、空数组与 stream transport 因子断言未覆盖而未纳入。

## 2026-10-09 请求取消与 durable 字段解码

独立请求取消批次以真实 prepared HTTP router 与生产 SSE reader 补齐 disconnect110 的预取消零 body、活动 listener 和单 reader 取消边界，升级 exact；其他原测试保持分类。stream durable 字段解码批次先红复现旧缓存掩盖 `message:42`，领域校验与生产准入保证字段错误优先于重放和冲突。核对9种结构/124字段，保留null、合法重复、未知扩展；缺失/null history的POST终态恢复只修改响应投影。8条冻结原测试逐项复核，不以补充控制推断其未测组合闭合，当前1666 exact/2147 partial/638 boundary。

两批失败原日志、源码快照和receipt均保留。最终字段解码 [生产owner receipt](verification-receipts/adk-typed-run-production-owner-verified-2026-10-09.json) 对应定向227 passed；[quick](gate-runs/adk-typed-run-quick-verified-2026-10-09.json) 与 [完整Rust](gate-runs/adk-typed-run-rust-verified-2026-10-09.json) 绑定同一1041文件冻结指纹。同步chat的400解码返回、GET缺失history/typed解码、请求重复与大小写解码、known-context、context首次revision、原生partial/tool投影及lease启动仍为活动缺口；历史LEAK未定位，整体目标未完成。

### 2026-10-09 同步 durable 字段解码与冻结原函数分类复核

生产`prepare_existing_run`在identity比较及保存response前校验persisted Run；实际HTTP8类损坏×同/冲突body均400/ADK_CHAT_FAILED，无data、零provider连接，run/audit/native sessions保持。有效null/重复/未知扩展返回保存回复或409。冻结Go同步handler错误400与stream500分别保留。恢复checkout红0/1、控制11、[最终定向receipt](verification-receipts/adk-sync-typed-production-owner-verified-2026-10-09.json)229 passed/0 failed；红源码与指纹独立保留。

本批8条原函数逐项核对：routes301、identity17、concurrent48、runner_chat737、persistence31、service_business169、recovery41、helpers216。runner_chat737注册停泊测试未构造混合resolved approvals，快照/原生消息/投影/计数/audit仍缺，exact退回partial。persistence31私有provider reasoning wire字段已有保存/恢复owner，旧ModelResponse文本slot的boundary解释错误；改partial，明确值/公开隐私未验证并撤销旧passed手工review。两条以外mapping与reuse保持，1665 exact/2149 partial/637 boundary。

原外置盘quick中断不计通过，完整Rust当时未启动；[中断证据](gate-runs/adk-sync-typed-external-volume-interrupted-2026-10-09.json)保留。内置盘恢复同分支/基线，现场[quick](gate-runs/adk-sync-typed-quick-verified-2026-10-09.json)2442 Rust/98 Pine/desktop11+48及七类replay通过；[完整Rust](gate-runs/adk-sync-typed-rust-verified-2026-10-09.json)4042 passed/0 failed/2 skipped、static/七类replay退出0，无LEAK。1041文件冻结，三份receipt/raw SHA、两份gate、8条Go函数/blob、17处reuse/anchor与diff核验，strict/context通过、unrecorded/stale0、既有unknown48。失败与最终源码/日志备份在`/Users/jiangfan/.cache/jftrade/parity-sync-typed-recovery-20261009`。私有reasoning保存/恢复值及公开隐私、GET typed解码/history、完整chat投影等继续开放；整体目标未完成。

### 2026-10-09 私有 reasoning 保存/读取与公开 run owner

生产红0/2确认GET run与同步RUNNING重放泄露reasoningEffortField/Value；追加真实HTTP终态cancel红0/1确认mutation输出也需同一公开投影。engine读取、重放和mutation输出仅移除两个私有字段，保存/恢复输入保留原值，公开effort保持。直接prepare/finish持久化与读取field/value，实际HTTP三条GET、RUNNING重放和终态cancel验证隐私及完整row/audit/native sessions、零provider连接；provider改名/model=v2后的session过滤列表仍恰好一个run且model=v1。

9条冻结原函数及blob复核：persistence31、reasoning10/23、config8/27/65、runner_chat552、routes29、adk_approval335。persistence31由完整原保存/读取/隐私断言升exact；runner_chat552补齐此前缺少的SessionRuns断言，exact保持。reasoning10旧session三级优先级不属于原函数，残余改为agent medium继承与request max覆盖缺直接owner证据；reasoning23实际chat/workflow恢复仍partial。1666 exact/2148 partial/637 boundary，新主测试的双引用经审核，只闭合两个原函数的不同断言。

编译trait装配101、TLS0/2、GET红与idle shutdown174秒/SIGTERM、有界关闭诊断0/1及mutation误接线6/1失败均保留原源码、指纹和日志。测试明确connection close并先停runtime owner、释放lease，再等待listener；不把idle关闭等待当作已修复。最终控制7、[生产owner receipt](verification-receipts/adk-private-reasoning-production-owner-verified-2026-10-09.json)236 passed/0 failed，无LEAK；[quick](gate-runs/adk-private-reasoning-quick-verified-2026-10-09.json)2445 Rust/98 Pine/desktop11+48与[完整Rust](gate-runs/adk-private-reasoning-rust-verified-2026-10-09.json)4045 passed/0 failed/2 skipped通过，static/七类replay退出0。1042文件冻结，十份receipt/raw SHA、两份gate、9条Go函数、10处reuse/anchor与diff核验；strict/context通过、unrecorded/stale0、既有unknown48。verifier的单引用allowed误判原错误保留，按仓库单/多引用规则校正，未放宽多引用审核条件或改旧关系。内置盘证据备份`/Users/jiangfan/.cache/jftrade/parity-private-reasoning-20261009`；整体未完成，继续实际恢复和GET边界。

### 2026-10-09 reasoning 恢复配置漂移

生产resume红1/1确认旧max快照被现行low-only provider映射拒绝。resolver的内部恢复参数只接收durable run的完整field/value；恢复时先采用该映射，新请求及不完整快照仍按现行支持矩阵校验。四条控制与最终定向243通过：实际mock provider发送snapshot-model/reasoning_effort=MAX_V1、原durable effort/field/value保留，终态重试零二次调用、row/audit/native events保持；新请求不能通过私有字段绕过校验。shutdown owner明确返回成功。

7条Go原函数及blob复核：reasoning10/23、config8/27/65、persistence31、runner_chat552。medium继承/max覆盖/extreme拒绝由直接生产resolver闭合，reasoning10升exact；reasoning23的workflowResumeContext仍缺真实owner证据，保持partial。旧composer保存不是恢复快照断言，解除该一处引用，保留其他四条复用。config27旧exact缺字段语法和完整mapping矩阵断言，降partial，当前1666 exact/2148 partial/637 boundary；[quick](gate-runs/adk-reasoning-snapshot-quick-verified-2026-10-09.json) 2449 Rust/98 Pine/desktop11+48及[现场完整Rust](gate-runs/adk-reasoning-snapshot-rust-verified-2026-10-09.json) 4049 passed/0 failed/2 skipped通过，static/七类replay退出0，无LEAK。1043文件冻结，四份receipt/raw SHA、两份gate、7条原函数及8处reuse/anchor/diff核验通过；strict-verified退出0，anchor unrecorded/stale0、既有unknown48。strict-final辅助lane误派发四条控制，其真实nextest命令与receipt保留，不计作strict；只有实际strict audit计入通过。证据备份`/Users/jiangfan/.cache/jftrade/parity-reasoning-snapshot-20261009`。整体未完成。 最终verifier发现reasoning10旧generic门禁无物理anchor，失败日志保留；解除该错误引用，专属owner完整断言保留，permission原引用独立保留，不放宽anchor要求。

### 2026-10-09 provider reasoning 完整原断言矩阵

两条实际生产owner测试补齐config27上一批明确的证据缺口：真实HTTP拒绝六种原非法配置且完整provider/audit/native sessions与secret保持、零provider连接；合法low=LOW及high/max共用balanced保存一致，生产resolver保持大小写值并拒绝medium未映射/extreme非法。原生产逻辑无需修改，两条owner控制与最终定向246通过，真实receipt及源码指纹保存。

逐项复核7条Go原函数/blob：model config8/27/65、persistence/provider10/47、reasoning10/23；仅config27完整断言闭合后partial→exact，当前1667 exact/2147 partial/637 boundary。reasoning10/23只刷新移动的Rust anchor和现场receipt，workflow分支保持partial。新两个单引用证据不允许未来未经审核复用，旧reuse关系保持。夹具在seed前shutdown/join启动scanner，并将accepted mock socket显式设回blocking，保留原超时及wire/终态/一次执行断言。nextest已知Apple并发capture pipe缺陷由统一pin0.9.145修复，官方五个archive SHA现场核验；旧LEAK、scanner竞争、HTTP/2 fetch、bootstrap旧pin及mock读取失败均留证。最终定向246 passed；quick全量preflight 4051 Rust/2435 Web/98 Pine/337 Python、现场完整Rust 4051 passed/0 failed/2 skipped通过，static与七类replay通过，无LEAK，1044文件和runner源码冻结；workflow恢复等其余缺口保持，整体未完成。详细现场结果与失败记录见[证据积压清单](parity-evidence-backlog.md)。

### 2026-10-09 known-context preview 生产 owner

直接生产 `emit_preview_session` 对已持有context的执行仅发布session、sequence1并EOF；durable context腐坏后的独立读取返回500/ADK_STORAGE_CORRUPT，而preview成功、完整context/session/audit保持，零provider连接。控制结合owner源码验证本次调用不重载context，不声称记录读取次数。已有Go式无response终态run经两路真实HTTP重连恰好run/final、正确response.run.id、无error且EOF，完整row/native/audit保持。

逐项复核6条冻结原函数/blob：recovery41、helpers167/151、routes136、disconnect110、runner_chat1124。只将recovery41 partial→exact，当前1668 exact/2146 partial/637 boundary；其余分类与缺口保持。两个旧terminal引用补物理anchor和mapping anchor，新owner单引用，旧reuse关系保持。首次编译、strict缺anchor及quick格式失败均留证；源码冻结结束后才修正。最终定向156 passed，quick2452 Rust/98 Pine/desktop11+48与七类replay通过，无LEAK；现场完整Rust4052 passed/0 failed/2 skipped、static与七类replay通过，无LEAK；1045 Rust文件和runner源码冻结。四份receipt、两份通过gate、六条原函数及15处reuse/anchor/diff核验通过。workflow reasoning恢复、delta分类/narrative组合、过期RUNNING清理及GET typed/history继续开放，整体未完成，详细证据见[积压清单](parity-evidence-backlog.md)。

### 2026-10-09 workflow 真实模型准入与旧请求重放

真实ProductionAdkPort/ProductionAdkChatRuntime先红复现新节点session not found。新节点不传伪造已有sessionId，改由原chat owner创建domain/native session；模型snapshot-model/MAX_V1、COMPLETED回复、原生消息及终态重复恢复一次provider调用闭合。旧跨调用前完整request、暂停后投影inputs两种checkpoint在provider drift后保留原显式session/canonical identity；完整child/native/session/audit保持且零provider连接，重复resume不写终态。

七条冻结原函数/blob复核：reasoning10/23、workflow_canvas133/167/223、config27/65；分类1668 exact/2146 partial/637 boundary保持。新证据单引用，旧reuse不改；Go parent聚合投影与非终态workflow reasoning context继续partial。最终定向236、quick2454 Rust/98 Pine/desktop11+48及现场完整Rust4054 passed/0 failed/2 skipped通过，static/七类replay通过，无LEAK，1046文件与runner源码冻结。六份receipt、两份gate、7条原函数及11处reuse/anchor/diff核验通过；既有partial的一个多引用allowed=false保持，不视为exact复用批准。编译、生产准入红、legacy图夹具及两个verifier错误均留证，详细证据见[积压清单](parity-evidence-backlog.md)。整体未完成，提交后继续非终态workflow快照。

### 2026-10-09 workflow 阻塞 child 的 reasoning 恢复

直接生产Canvas owner重放仍PENDING_INPUT的child，provider drift后完整child/audit/native events及durable max/reasoning_effort/MAX_V1保持、零provider连接；真实RespondToInput恢复实际发送旧snapshot-model/MAX_V1恰好一次。既有completion barrier同步最终audit后捕获终态；重复workflow/runtime恢复保持完整run/log/audit/native events，重放完成才shutdown/join。

复核7条冻结原函数：reasoning10/23、workflow_canvas133/167、input_request584、config27/65。新增child证据不闭合Go parent workflowResumeContext语义，reasoning23保持partial并精准保留该缺口。input584原exact仅引用park/audit半，未闭合两问、推荐项和完整回答后恢复，纠正为partial，当前1667/2147/637；新单引用与旧reuse边界不变。三个夹具失败及源码快照保留，最终定向281 passed/0 failed；quick2455 Rust/98 Pine与七类replay通过、无LEAK，desktop Node检查未安排。现场完整Rust4055 passed/0 failed/2 skipped，static及七类replay通过、无LEAK；1047文件与runner源码冻结，五份receipt、两份gate、7条原函数及12处reuse/anchor/diff核验通过，strict/context/anchor通过。整体未完成，继续input584真实chat两问与完整回答恢复，见[积压清单](parity-evidence-backlog.md)。

### 2026-10-09 request_user 真实 chat 与完整回答恢复

后续过期RUNNING流批次详见[积压清单](parity-evidence-backlog.md)：两个生产GET红复现200→预期404后修复，定向231通过；routes136升级exact，routes_resource410完整原函数复核降为partial。首次quick的805行架构失败留证，重连职责拆分后quick2715 Rust/98 Pine/desktop11+48、现场完整Rust4060 passed/0 failed/2 skipped及static/七类replay通过，无LEAK。四份receipt、三份gate、1051份源码指纹和六条原函数核验；记录级终态TTL与无合法时间回退保持缺口，当前1668/2146/637。整体未完成，继续真实POST/skill链路。

真实chat/model工具调用返回两问、q1-o1推荐项与PENDING_INPUT；Balanced/q2-o1回答后同run COMPLETED、同inputRequest ANSWERED且两条answers完整相等。审计隐私、原生final回复、重复回答与恢复的完整终态保持、provider恰好两次闭合。单问控制的durable response与实际wire保留唯一Conservative answer、中文原请求、非空instruction和正确call身份/名称；执行错误明确FAILED，重复恢复不写入或调用模型。

复核7条原函数input530/556/584/773/802、reasoning23、config65；584完整闭合后恢复exact，当前1668/2146/637。773/802的新共用owner按不同durable/wire断言审核，584新证据仅单引用；parent reasoning与timeline分组缺口保持。FAILED row早于同owner附件写入的夹具失败保留，capture在既有completion barrier后重新读取，最终定向282 passed/0 failed。quick2457 Rust/98 Pine与现场完整Rust4057 passed/0 failed/2 skipped、static/七类replay通过，无LEAK；desktop Node检查未安排。1048 Rust文件与runner源码冻结，四份receipt、两份gate、7条原函数及9处reuse/anchor/diff核验通过，strict/context/anchor通过。整体未完成，继续超时RUNNING stream的重连保留期控制，详见[积压清单](parity-evidence-backlog.md)。
