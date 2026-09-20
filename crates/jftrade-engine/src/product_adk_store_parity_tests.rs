//! Behavior tests for the Go `internal/assistant/engine/store_lifecycle_test.go`
//! rows that are observable through the production ADK read and mutation
//! ports: session listing filters, approval and optimization ordering, session
//! composer state, session deletion and the builtin strategy skill split.

use std::collections::BTreeMap;
use std::thread::sleep;
use std::time::Duration;

use serde_json::{Value, json};

use jftrade_store_sqlite::CreateAdkRunParams;

use crate::product::AdkReadSnapshotPort;

use super::*;

fn read_json(port: &ProductionAdkPort, path: &str, query: &str) -> Value {
    match port.read(path, query).expect("read snapshot") {
        AdkReadSnapshot::Json(value) => value,
        other => panic!("expected a JSON snapshot for {path}, got {other:?}"),
    }
}

fn mutate(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> Value {
    port.mutate(&AdkMutationInput {
        operation,
        identifiers: identifiers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect::<BTreeMap<_, _>>(),
        body,
        webhook_secret: None,
    })
    .expect("mutation succeeds")
}

fn create_session(port: &ProductionAdkPort, agent_id: &str, title: &str) -> String {
    mutate(
        port,
        AdkMutationOperation::CreateSession,
        &[],
        json!({"agentId": agent_id, "title": title}),
    )["id"]
        .as_str()
        .expect("session id")
        .to_owned()
}

fn session_ids(snapshot: &Value) -> Vec<String> {
    snapshot["sessions"]
        .as_array()
        .expect("sessions array")
        .iter()
        .filter_map(|session| session["id"].as_str().map(str::to_owned))
        .collect()
}

fn approval_ids(snapshot: &Value) -> Vec<String> {
    snapshot["approvals"]
        .as_array()
        .expect("approvals array")
        .iter()
        .filter_map(|approval| approval["id"].as_str().map(str::to_owned))
        .collect()
}

fn optimization_task_ids(snapshot: &Value) -> Vec<String> {
    snapshot["tasks"]
        .as_array()
        .expect("tasks array")
        .iter()
        .filter_map(|task| task["id"].as_str().map(str::to_owned))
        .collect()
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:466
/// TestListSessionsPageFiltersQueryAndPaginates.
#[test]
fn adk_session_page_filters_by_agent_and_title_and_paginates() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    for agent in ["agent-a", "agent-b"] {
        store
            .upsert_agent(
                agent,
                &json!({"id": agent, "name": agent, "status": "ENABLED"}).to_string(),
            )
            .expect("seed agent");
    }

    let older = create_session(&port, "agent-a", "Alpha Review");
    sleep(Duration::from_millis(15));
    let newer = create_session(&port, "agent-a", "alpha Deep Dive");
    sleep(Duration::from_millis(15));
    create_session(&port, "agent-b", "Alpha Other Agent");
    sleep(Duration::from_millis(15));
    create_session(&port, "agent-a", "Gamma Notes");

    let first = read_json(
        &port,
        "/api/v1/adk/sessions",
        "agentId=agent-a&query=ALPHA&limit=1&offset=0",
    );
    assert_eq!(first["page"]["total"], 2);
    assert_eq!(first["page"]["returned"], 1);
    assert_eq!(session_ids(&first), vec![newer.clone()]);

    let second = read_json(
        &port,
        "/api/v1/adk/sessions",
        "agentId=agent-a&query=alpha&limit=1&offset=1",
    );
    assert_eq!(second["page"]["total"], 2);
    assert_eq!(session_ids(&second), vec![older]);

    // The other agent's matching session and the non-matching title stay out
    // of the filtered total.
    let unscoped = read_json(&port, "/api/v1/adk/sessions", "query=alpha&limit=10");
    assert_eq!(unscoped["page"]["total"], 3);
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:565
/// TestListApprovalsPageFiltersAndSortsNewestFirst.
#[test]
fn adk_approval_page_orders_by_latest_update_and_counts_filtered_rows() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    for (run_id, agent_id) in [
        ("run-approval-older", "agent-a"),
        ("run-approval-newer", "agent-a"),
        ("run-approval-other-agent", "agent-b"),
        ("run-approval-other-status", "agent-a"),
    ] {
        store
            .create_run(CreateAdkRunParams {
                id: run_id,
                session_id: "session-approval-page",
                agent_id,
                status: "PENDING",
                client_request_id: "",
                request_fingerprint: "",
                payload_json: "{}",
            })
            .expect("seed run");
    }
    let approval = |id: &str, run_id: &str, agent_id: &str, status: &str| {
        json!({
            "id": id,
            "runId": run_id,
            "agentId": agent_id,
            "toolName": "strategy.optimize",
            "status": status,
        })
        .to_string()
    };
    store
        .create_approval(
            "approval-older",
            "run-approval-older",
            "agent-a",
            "PENDING",
            &approval("approval-older", "run-approval-older", "agent-a", "PENDING"),
        )
        .expect("older approval");
    sleep(Duration::from_millis(15));
    store
        .create_approval(
            "approval-newer",
            "run-approval-newer",
            "agent-a",
            "PENDING",
            &approval("approval-newer", "run-approval-newer", "agent-a", "PENDING"),
        )
        .expect("newer approval");
    sleep(Duration::from_millis(15));
    store
        .create_approval(
            "approval-other-agent",
            "run-approval-other-agent",
            "agent-b",
            "PENDING",
            &approval(
                "approval-other-agent",
                "run-approval-other-agent",
                "agent-b",
                "PENDING",
            ),
        )
        .expect("other agent approval");
    sleep(Duration::from_millis(15));
    store
        .create_approval(
            "approval-other-status",
            "run-approval-other-status",
            "agent-a",
            "APPROVED",
            &approval(
                "approval-other-status",
                "run-approval-other-status",
                "agent-a",
                "APPROVED",
            ),
        )
        .expect("other status approval");

    let page = read_json(
        &port,
        "/api/v1/adk/approvals",
        "status=PENDING&agentId=agent-a&limit=10&offset=0",
    );
    assert_eq!(page["page"]["total"], 2);
    assert_eq!(
        approval_ids(&page),
        vec!["approval-newer".to_owned(), "approval-older".to_owned()]
    );

    // Go orders the page by `updated_at DESC`, so resolving the older approval
    // moves it in front of the approval created after it.
    sleep(Duration::from_millis(15));
    assert!(
        store
            .update_approval_status("approval-older", "PENDING")
            .expect("refresh older approval")
    );
    let page = read_json(
        &port,
        "/api/v1/adk/approvals",
        "status=pending&agentId=agent-a&limit=10&offset=0",
    );
    assert_eq!(page["page"]["total"], 2);
    assert_eq!(
        approval_ids(&page),
        vec!["approval-older".to_owned(), "approval-newer".to_owned()]
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:609
/// TestListOptimizationTasksSortsByUpdatedAtDesc.
#[test]
fn adk_optimization_task_page_orders_by_latest_update() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    let older = store
        .upsert_optimization_task(
            "opt-older",
            &json!({"id": "opt-older", "objective": "older"}).to_string(),
        )
        .expect("older task");
    sleep(Duration::from_millis(15));
    store
        .upsert_optimization_task(
            "opt-newer",
            &json!({"id": "opt-newer", "objective": "newer"}).to_string(),
        )
        .expect("newer task");

    let page = read_json(&port, "/api/v1/adk/optimization-tasks", "");
    assert_eq!(
        optimization_task_ids(&page),
        vec!["opt-newer".to_owned(), "opt-older".to_owned()]
    );

    // Go sorts by `updated_at DESC, id ASC`, so touching the older task puts
    // it back at the head of the queue.
    sleep(Duration::from_millis(15));
    assert!(
        store
            .update_optimization_task_if_revision(
                "opt-older",
                &older.updated_at,
                &json!({"id": "opt-older", "objective": "older", "status": "RUNNING"})
                    .to_string(),
            )
            .expect("refresh older task")
    );
    let page = read_json(&port, "/api/v1/adk/optimization-tasks", "");
    assert_eq!(
        optimization_task_ids(&page),
        vec!["opt-older".to_owned(), "opt-newer".to_owned()]
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:501
/// TestSessionComposerStatePersistsAndDeletesWithSession.
#[test]
fn adk_composer_state_truncates_trim_and_rejects_invalid_modes() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    let session = create_session(&port, "jftrade-default", "composer");
    assert!(
        store
            .get_session_composer_state(&session)
            .expect("empty composer state")
            .is_none()
    );

    let saved = mutate(
        &port,
        AdkMutationOperation::UpdateSessionComposerState,
        &[("sessionId", &session)],
        json!({
            "chatDraft": "x".repeat(50_020),
            "providerIdOverride": " provider-session ",
            "modelOverride": " model-session ",
            "reasoningEffortOverride": "high",
            "workModeOverride": "loop",
            "permissionModeOverride": "less_approval",
            "goalObjectiveDraft": "目标草稿",
            "goalObjectiveTouched": true,
        }),
    );
    assert_eq!(saved["sessionId"], session);
    assert_eq!(saved["chatDraft"].as_str().map(str::len), Some(50_000));
    assert_eq!(saved["providerIdOverride"], "provider-session");
    assert_eq!(saved["modelOverride"], "model-session");
    assert_eq!(saved["reasoningEffortOverride"], "high");
    assert_eq!(saved["workModeOverride"], "loop");
    assert_eq!(saved["permissionModeOverride"], "less_approval");
    assert_eq!(saved["goalObjectiveDraft"], "目标草稿");
    assert_eq!(saved["goalObjectiveTouched"], true);

    for (key, value) in [
        ("workModeOverride", "sequential"),
        ("permissionModeOverride", "root"),
        ("reasoningEffortOverride", "extreme"),
    ] {
        let error = port
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::UpdateSessionComposerState,
                identifiers: BTreeMap::from([("sessionId".to_owned(), session.clone())]),
                body: json!({key: value}),
                webhook_secret: None,
            })
            .expect_err("invalid composer override must fail");
        match error {
            AdkMutationPortError::Failed { status, code, .. } => {
                assert_eq!(status, 400, "{key} status");
                assert_eq!(code, "ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED", "{key} code");
            }
            other => panic!("expected a failed composer write for {key}, got {other:?}"),
        }
    }

    mutate(
        &port,
        AdkMutationOperation::DeleteSession,
        &[("sessionId", &session)],
        Value::Null,
    );
    assert!(
        store
            .get_session_composer_state(&session)
            .expect("composer state after delete")
            .is_none(),
        "session deletion must cascade the composer state row"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:553
/// TestDeleteSessionMissingAndBlankAreNotFound.
#[test]
fn adk_session_delete_missing_is_reported_with_the_session_error_code() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);

    // Go's runtime answers `session not found` for a missing session, which the
    // HTTP edge maps to `404 ADK_SESSION_NOT_FOUND`.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteSession,
            identifiers: BTreeMap::from([(
                "sessionId".to_owned(),
                "session-missing".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("missing session delete must fail");
    match error {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("expected 404 ADK_SESSION_NOT_FOUND, got {other:?}"),
    }

    // The store keeps Go's idempotent delete for an absent row and rejects a
    // blank id as a not-found classification instead of deleting nothing.
    assert!(
        !store
            .delete_session("session-missing")
            .expect("store delete of a missing session is idempotent")
    );
    assert!(matches!(
        store.delete_session(""),
        Err(jftrade_store_sqlite::AdkStoreError::Validation(_))
    ));
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:765
/// TestPreparedAgentLoadsOnlyEnabledBoundSkillsAndTools.
#[test]
fn adk_agent_write_requires_registered_skills_and_keeps_declared_tools() {
    let (port, _directory) = agent_validation_port();
    let error = create_agent_error(
        &port,
        json!({
            "id": "agent-missing-skill",
            "name": "Agent",
            "skills": ["missing-skill"],
        }),
    );
    assert_bad_request(error, "unknown ADK skill: missing-skill");

    let created = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({
            "id": "agent-bound-skill",
            "name": "Agent",
            "instruction": "Base instruction.",
            "providerId": "provider-enabled",
            "tools": ["http.fetch", "system.status"],
            "skills": ["jftrade-strategy-research"],
            "permissionMode": "less_approval",
            "status": "ENABLED",
        }),
    );
    assert_eq!(created["instruction"], "Base instruction.");
    assert_eq!(
        created["tools"],
        json!(["http.fetch", "system.status"]),
        "binding a skill must not rewrite the declared tool list"
    );
    assert_eq!(created["skills"], json!(["jftrade-strategy-research"]));

    let read_back = read_json(&port, "/api/v1/adk/agents", "");
    let stored = read_back["agents"]
        .as_array()
        .expect("agents array")
        .iter()
        .find(|agent| agent["id"] == "agent-bound-skill")
        .expect("stored agent");
    assert_eq!(stored["tools"], json!(["http.fetch", "system.status"]));
    assert_eq!(stored["instruction"], "Base instruction.");
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:792
/// TestSkillRegistryReportsMetadataAndAllowedTools.
#[test]
fn adk_builtin_strategy_skills_publish_the_curated_tool_split() {
    let (port, _directory) = agent_validation_port();
    let snapshot = read_json(&port, "/api/v1/adk/skills", "");
    let skills = snapshot["skills"].as_array().expect("skills array");
    let find = |id: &str| {
        skills
            .iter()
            .find(|skill| skill["id"] == id)
            .unwrap_or_else(|| panic!("builtin skill {id} must be projected"))
    };
    let tools_of = |skill: &Value| {
        skill["tools"]
            .as_array()
            .expect("skills tools array")
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_owned)
            .collect::<Vec<_>>()
    };

    let research = find("jftrade-strategy-research");
    assert_eq!(research["builtin"], true);
    assert_eq!(research["source"], "builtin");
    assert_eq!(research["validationStatus"], "VALID");
    assert_eq!(research["version"], "12");
    let research_tools = tools_of(research);
    for tool in [
        "strategy.pine_spec",
        "strategy.validate_pine",
        "strategy.definition_versions.list",
        "strategy.definition_versions.get",
        "strategy.research_backtest",
        "backtest.runs",
        "backtest.result_view",
        "workflow.wait",
        "market.snapshot",
        "market.candles",
    ] {
        assert!(
            research_tools.iter().any(|candidate| candidate == tool),
            "research skill must publish {tool}: {research_tools:?}"
        );
    }
    for forbidden in ["strategy.save_draft", "strategy.save_definition", "strategy.optimize"] {
        assert!(
            !research_tools.iter().any(|candidate| candidate == forbidden),
            "research skill must not publish {forbidden}: {research_tools:?}"
        );
    }

    let publish = find("jftrade-strategy-publish");
    assert_eq!(publish["builtin"], true);
    assert_eq!(publish["source"], "builtin");
    assert_eq!(publish["validationStatus"], "VALID");
    assert_eq!(publish["version"], "12");
    let publish_tools = tools_of(publish);
    for tool in [
        "strategy.validate_pine",
        "strategy.definition_versions.list",
        "strategy.definition_versions.get",
        "strategy.optimize",
        "backtest.runs",
    ] {
        assert!(
            publish_tools.iter().any(|candidate| candidate == tool),
            "publish skill must publish {tool}: {publish_tools:?}"
        );
    }
    assert!(
        !publish_tools
            .iter()
            .any(|candidate| candidate == "strategy.research_backtest"),
        "publish skill must not publish research_backtest: {publish_tools:?}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:153
/// `TestPauseGuardBoundaryBranches`: Rust has no `preserveUserGoalPauseLifecycle`
/// pure function, so the pause lifecycle is owned by the run mutations plus the
/// revision CAS.  A running loop goal gains `pauseRequestedAt` and
/// `resumeState=user_pause_requested`, a stale writer cannot clear them, an
/// explicit resume drops every pause field, and a chat run can never take the
/// goal-pause path.
#[test]
fn goal_pause_and_resume_mutations_own_the_pause_lifecycle_fields() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    let seed = |id: &str, work_mode: &str| {
        let payload = json!({
            "id": id,
            "sessionId": "session-pause-guard",
            "agentId": "agent-pause-guard",
            "status": "RUNNING",
            "workMode": work_mode,
            "objective": "推进目标",
            "workflowStatus": "RUNNING",
            "message": "goal running",
        })
        .to_string();
        store
            .create_run(CreateAdkRunParams {
                id,
                session_id: "session-pause-guard",
                agent_id: "agent-pause-guard",
                status: "RUNNING",
                client_request_id: "",
                request_fingerprint: "",
                payload_json: &payload,
            })
            .expect("seed run");
    };
    seed("run-goal-guard", "loop");
    seed("run-chat-guard", "chat");

    let paused = mutate(
        port.as_ref(),
        AdkMutationOperation::PauseRun,
        &[("runId", "run-goal-guard")],
        json!({}),
    );
    assert_eq!(paused["status"], "RUNNING");
    assert_eq!(paused["resumeState"], "user_pause_requested");
    assert_eq!(paused["message"], "目标将在当前轮结束后暂停。");
    assert!(paused["pauseRequestedAt"].is_string());
    assert!(
        paused.get("pausedAt").is_none(),
        "a pause request is not a pause yet: {paused}"
    );

    let failure = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::PauseRun,
            identifiers: [("runId".to_owned(), "run-chat-guard".to_owned())]
                .into_iter()
                .collect(),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("chat runs cannot be paused");
    match failure {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_RUN_PAUSE_FAILED");
            assert!(
                message.contains("only loop goal runs can be paused"),
                "message = {message}"
            );
        }
        other => panic!("unexpected pause failure {other:?}"),
    }

    // A stale snapshot written against the pre-pause revision cannot clear the
    // pending pause request.
    let current = store.get_run("run-goal-guard").expect("read run").expect("run");
    let stale_payload = json!({
        "id": "run-goal-guard",
        "sessionId": "session-pause-guard",
        "agentId": "agent-pause-guard",
        "status": "RUNNING",
        "workMode": "loop",
        "objective": "推进目标",
        "workflowStatus": "RUNNING",
        "message": "goal running",
    })
    .to_string();
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-goal-guard",
                "RUNNING",
                "stale-revision",
                "RUNNING",
                &stale_payload,
            )
            .expect("stale pause guard")
    );
    let guarded: Value =
        serde_json::from_str(&store.get_run("run-goal-guard").unwrap().unwrap().payload_json)
            .expect("guarded payload");
    assert_eq!(guarded["resumeState"], "user_pause_requested");
    assert_eq!(
        guarded["pauseRequestedAt"], paused["pauseRequestedAt"],
        "the pending pause request survives the stale writer"
    );

    // The loop parks the goal: PAUSED + pausedReason=user is the state the
    // resume path accepts.
    let parked_payload = json!({
        "id": "run-goal-guard",
        "sessionId": "session-pause-guard",
        "agentId": "agent-pause-guard",
        "status": "PAUSED",
        "workMode": "loop",
        "objective": "推进目标",
        "workflowStatus": "PAUSED",
        "message": "paused by user",
        "pauseRequestedAt": paused["pauseRequestedAt"],
        "pausedAt": "2026-09-20T00:05:00Z",
        "pausedReason": "user",
        "resumeState": "user_paused",
    })
    .to_string();
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-goal-guard",
                "RUNNING",
                &current.updated_at,
                "PAUSED",
                &parked_payload,
            )
            .expect("park paused goal")
    );

    let resumed = mutate(
        port.as_ref(),
        AdkMutationOperation::ResumeRun,
        &[("runId", "run-goal-guard")],
        json!({}),
    );
    assert_eq!(resumed["status"], "RUNNING");
    assert_eq!(resumed["resumeState"], "user_resuming");
    assert_eq!(resumed["message"], "goal resumed");
    for cleared in ["pauseRequestedAt", "pausedAt", "pausedReason"] {
        assert!(
            resumed.get(cleared).is_none_or(Value::is_null),
            "{cleared} must be cleared by the explicit resume: {resumed}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:684
/// `TestRuntimeSnapshotAndProviderTestBoundaryBranches`: a snapshot read fails
/// closed once the agent table is gone, `TestProvider` reports a missing
/// provider as 404, and the same operation without a chat port reports 503
/// instead of pretending the provider answered.
#[test]
fn snapshot_and_provider_test_boundaries_fail_closed() {
    let (port, store, directory) = setup_test_adk_mutation_port(None);

    let missing = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::TestProvider,
            identifiers: [("providerId".to_owned(), "missing-provider".to_owned())]
                .into_iter()
                .collect(),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("a missing provider cannot be tested");
    match missing {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404, "message = {message}");
            assert_eq!(code, "ADK_PROVIDER_NOT_FOUND");
        }
        other => panic!("unexpected provider test failure {other:?}"),
    }

    store
        .upsert_provider(
            "provider-test-boundary",
            &json!({
                "id": "provider-test-boundary",
                "displayName": "Provider test boundary",
                "baseUrl": "http://127.0.0.1:1/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    let unavailable = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::TestProvider,
            identifiers: [("providerId".to_owned(), "provider-test-boundary".to_owned())]
                .into_iter()
                .collect(),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("a runtime without a chat port cannot test providers");
    match unavailable {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 503, "message = {message}");
            assert_eq!(code, "ADK_PROVIDER_TEST_UNAVAILABLE");
        }
        other => panic!("unexpected provider test failure {other:?}"),
    }

    // A snapshot whose agent table disappeared reports the failure instead of an
    // empty console payload.
    let connection =
        rusqlite::Connection::open(directory.path().join("adk.db")).expect("open ADK database");
    connection
        .execute("DROP TABLE adk_agents", [])
        .expect("drop agents table");
    drop(connection);
    assert!(
        port.read("/api/v1/adk", "").is_err(),
        "a snapshot without the agent table must fail closed"
    );
}

fn workflow_ids(snapshot: &Value) -> Vec<String> {
    snapshot["workflows"]
        .as_array()
        .expect("workflows array")
        .iter()
        .filter_map(|workflow| workflow["id"].as_str().map(str::to_owned))
        .collect()
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_test.go:15
/// `TestWorkflowManagementToolCatalogAndApprovalMatrix`.
///
/// Go registers fifteen `workflows.*` / `workflow_triggers.*` /
/// `workflow_runs.*` tools in the `workflow` category, requires the
/// `jftrade-workflow-management` skill on every descriptor, and gates only the
/// mutation tools in approval mode.  Rust has no workflow-management tool
/// registry — the console owns those operations over
/// `/api/v1/adk/workflows*` — so the reachable half of the matrix is the
/// builtin skill that publishes the workflow-category tools and the approval
/// decision of the one workflow tool the model can call.
#[test]
fn workflow_management_catalog_keeps_the_skill_and_approval_boundaries() {
    let (port, _directory) = agent_validation_port();
    let skills = read_json(&port, "/api/v1/adk/skills", "");
    let skill = skills["skills"]
        .as_array()
        .expect("skills array")
        .iter()
        .find(|skill| skill["id"] == "jftrade-workflow-management")
        .unwrap_or_else(|| panic!("the workflow management skill must be projected: {skills}"));
    assert_eq!(skill["builtin"], true);
    assert_eq!(skill["source"], "builtin");
    let tools = skill["tools"].as_array().expect("skill tools array");
    let names = tools
        .iter()
        .filter_map(Value::as_str)
        .collect::<Vec<_>>();
    assert!(
        names.contains(&"workflow.wait"),
        "the skill publishes the workflow-category tool: {skill}"
    );
    assert!(
        names.contains(&"interaction.request_user"),
        "the skill publishes the interaction-category tool: {skill}"
    );

    // Go's mutation tools wait for the operator in approval mode.  Rust's only
    // workflow tool is a read-only wait, so no permission mode gates it, and a
    // name outside the catalog still fails closed.
    for mode in ["approval", "less_approval", "all"] {
        assert!(
            !port.tool_catalog.requires_approval("workflow.wait", mode),
            "workflow.wait must not require approval in {mode}"
        );
    }
    assert!(
        port.tool_catalog
            .requires_approval("workflows.create", "all"),
        "a tool outside the catalog fails closed"
    );
    let advertised = port.tool_catalog.ids();
    for console_only in [
        "workflows.list",
        "workflows.create",
        "workflows.delete",
        "workflow_triggers.run",
        "workflow_runs.wait",
    ] {
        assert!(
            !advertised.iter().any(|id| id == console_only),
            "the console-only workflow tool {console_only} is never advertised to the model: {advertised:?}"
        );
    }
}

fn trigger_ids(snapshot: &Value) -> Vec<String> {
    snapshot["triggers"]
        .as_array()
        .expect("triggers array")
        .iter()
        .filter_map(|trigger| trigger["id"].as_str().map(str::to_owned))
        .collect()
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_test.go:132
/// `TestWorkflowManagementToolUpdatesUsePatchSemantics`.
///
/// Go's `workflows.update` / `workflow_triggers.update` tools patch instead of
/// replacing: an omitted field keeps its stored value, while an explicitly
/// empty value clears it (`description:""`, `tags:[]`, `clearCanvasGraph`).
/// The Rust mutation port expresses the same contract through the REST body —
/// the field has to be present, and `null`/`[]`/`""` clear it — so the
/// reachable assertions are the preserved fields, the applied clears and the
/// trigger's retained type.  Go's tool-only webhook restrictions (`webhooks
/// are managed through the UI/API`) do not exist here because the Rust REST
/// route is that UI/API surface and never manages webhook secrets implicitly.
#[test]
fn workflow_updates_keep_omitted_fields_and_apply_explicit_clears() {
    let (port, _directory) = agent_validation_port();
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "Patch Agent", "instruction": "patch", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();
    let created = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "name": "Keep Name",
            "description": "old",
            "agentId": agent_id,
            "promptTemplate": "keep prompt",
            "objectiveTemplate": "keep objective",
            "defaultInputs": {"symbol": "US.AAPL"},
            "tags": ["old"],
            "canvasGraph": {
                "version": "v1",
                "nodes": [{"id": "start", "type": "start", "title": "Start", "data": {}}],
                "edges": [],
            },
        }),
    );
    let workflow_id = created["id"].as_str().expect("workflow id").to_owned();
    assert_eq!(created["description"], "old");
    assert!(
        created.get("canvasGraph").is_some_and(|graph| !graph.is_null()),
        "the created workflow keeps its canvas graph: {created}"
    );

    let updated = mutate(
        &port,
        AdkMutationOperation::UpdateWorkflow,
        &[("workflowId", workflow_id.as_str())],
        json!({"description": "", "tags": [], "canvasGraph": null}),
    );
    assert_eq!(updated["name"], "Keep Name", "an omitted name is preserved");
    assert_eq!(
        updated["promptTemplate"], "keep prompt",
        "an omitted prompt template is preserved"
    );
    assert_eq!(
        updated["objectiveTemplate"], "keep objective",
        "an omitted objective template is preserved"
    );
    assert_eq!(
        updated["defaultInputs"]["symbol"], "US.AAPL",
        "omitted default inputs are preserved"
    );
    assert_eq!(updated["agentId"], agent_id);
    assert_eq!(updated["description"], "", "the empty description clears it");
    assert_eq!(updated["tags"], json!([]), "the empty tag list clears tags");
    assert!(
        updated
            .get("canvasGraph")
            .is_none_or(Value::is_null),
        "the null canvas graph clears the stored graph: {updated}"
    );

    // The same patch semantics survive the read projection.
    let listed = read_json(&port, "/api/v1/adk/workflows", "");
    let stored = listed["workflows"]
        .as_array()
        .expect("workflows array")
        .iter()
        .find(|workflow| workflow["id"] == workflow_id.as_str())
        .unwrap_or_else(|| panic!("patched workflow must stay listed: {listed}"));
    assert_eq!(stored["name"], "Keep Name");
    assert_eq!(stored["promptTemplate"], "keep prompt");
    assert_eq!(stored["description"], "");
    assert_eq!(stored["tags"], json!([]));

    let trigger = mutate(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[("workflowId", workflow_id.as_str())],
        json!({
            "type": "webhook",
            "title": "Old Webhook",
            "config": {"source": "old"},
        }),
    );
    let trigger_id = trigger["trigger"]["id"]
        .as_str()
        .expect("trigger id")
        .to_owned();
    assert!(
        trigger["secret"].as_str().is_some_and(|secret| !secret.is_empty()),
        "creating a webhook trigger returns its one-time secret: {trigger}"
    );
    assert_eq!(trigger["trigger"]["hasSecret"], true);
    let updated_trigger = mutate(
        &port,
        AdkMutationOperation::UpdateWorkflowTrigger,
        &[
            ("workflowId", workflow_id.as_str()),
            ("triggerId", trigger_id.as_str()),
        ],
        json!({"title": "Renamed Webhook", "config": {}}),
    );
    assert_eq!(
        updated_trigger["trigger"]["type"], "webhook",
        "an omitted type keeps the stored trigger type: {updated_trigger}"
    );
    assert_eq!(updated_trigger["trigger"]["title"], "Renamed Webhook");
    assert_eq!(
        updated_trigger["trigger"]["config"],
        json!({}),
        "an explicit empty config clears the stored one"
    );
    assert!(
        updated_trigger.get("secret").is_none(),
        "an update without `resetSecret` never returns a secret: {updated_trigger}"
    );
    assert_eq!(updated_trigger["trigger"]["hasSecret"], true);
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_test.go:183
/// `TestWorkflowManagementToolListsCreatesAndDeletes`.
///
/// Go's workflow tools list, create and delete definitions, triggers and run
/// logs.  The Rust REST surface keeps the same boundaries: a created workflow
/// or trigger is listed, deleting one answers `{"deleted": true}` and removes
/// it from the list projection, and a trigger log stays readable after the
/// definition it belongs to is gone.
#[test]
fn workflow_and_trigger_lists_hide_deleted_rows_after_create_and_delete() {
    let (port, _directory) = agent_validation_port();
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "CRUD Agent", "instruction": "crud", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();
    let workflow_id = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "name": "CRUD Workflow",
            "agentId": agent_id,
            "promptTemplate": "Run {{symbol}}",
        }),
    )["id"]
        .as_str()
        .expect("workflow id")
        .to_owned();
    assert!(
        workflow_ids(&read_json(&port, "/api/v1/adk/workflows", "")).contains(&workflow_id),
        "a created workflow is listed"
    );

    let trigger_id = mutate(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[("workflowId", workflow_id.as_str())],
        json!({"type": "manual", "title": "Manual run", "config": {}}),
    )["trigger"]["id"]
        .as_str()
        .expect("trigger id")
        .to_owned();
    let trigger_path = format!("/api/v1/adk/workflows/{workflow_id}/triggers");
    assert!(
        trigger_ids(&read_json(&port, &trigger_path, "")).contains(&trigger_id),
        "a created trigger is listed"
    );

    // A durable invocation log stays readable after its definition is removed.
    port.store
        .create_workflow_trigger_log(
            "log-crud",
            &workflow_id,
            &trigger_id,
            "manual",
            "SUCCEEDED",
            "run-crud",
            r#"{"id":"log-crud","status":"SUCCEEDED","runId":"run-crud"}"#,
        )
        .expect("create trigger log");
    let logs = read_json(&port, "/api/v1/adk/workflow-trigger-logs", "");
    assert!(
        logs["logs"]
            .as_array()
            .expect("logs array")
            .iter()
            .any(|log| log["id"] == "log-crud"),
        "the run log is listed: {logs}"
    );
    assert_eq!(
        port.store
            .get_workflow_trigger_log("log-crud")
            .expect("read trigger log")
            .expect("log row")
            .run_id,
        "run-crud",
        "`workflow_runs.get` reads the stored log"
    );

    let deleted_trigger = mutate(
        &port,
        AdkMutationOperation::DeleteWorkflowTrigger,
        &[
            ("workflowId", workflow_id.as_str()),
            ("triggerId", trigger_id.as_str()),
        ],
        json!({}),
    );
    assert_eq!(deleted_trigger["deleted"], true);
    assert!(
        !trigger_ids(&read_json(&port, &trigger_path, "")).contains(&trigger_id),
        "a deleted trigger disappears from the list"
    );

    let deleted_workflow = mutate(
        &port,
        AdkMutationOperation::DeleteWorkflow,
        &[("workflowId", workflow_id.as_str())],
        json!({}),
    );
    assert_eq!(deleted_workflow["deleted"], true);
    assert!(
        !workflow_ids(&read_json(&port, "/api/v1/adk/workflows", "")).contains(&workflow_id),
        "a deleted workflow disappears from the list"
    );
    assert_eq!(
        port.store
            .get_workflow_trigger_log("log-crud")
            .expect("read trigger log after delete")
            .map(|log| log.status)
            .as_deref(),
        Some("SUCCEEDED"),
        "deleting the definition never rewrites its run history"
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_test.go:285
/// `TestUnavailableWorkflowToolManagerFailsClosed`.
///
/// Go's unavailable manager fails closed on every workflow call instead of
/// answering an empty page or a synthetic start result.  The Rust equivalent
/// boundary is a runtime without the assistant model port: the run is rejected
/// with `503 ADK_WORKFLOW_RUNTIME_UNAVAILABLE` and the already-claimed
/// invocation is durably finalised `FAILED` with that code, so no console
/// reader can mistake an unavailable runtime for a queued run.
#[test]
fn workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation() {
    let (mut port, _directory) = agent_validation_port();
    port.chat_runtime = None;
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "Unavailable Agent", "instruction": "fail closed", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();
    let workflow_id = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "name": "Unavailable Runtime Workflow",
            "agentId": agent_id,
            "promptTemplate": "Run {{symbol}}",
        }),
    )["id"]
        .as_str()
        .expect("workflow id")
        .to_owned();

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RunWorkflow,
            identifiers: BTreeMap::from([("workflowId".to_owned(), workflow_id.clone())]),
            body: json!({"inputs": {"symbol": "US.AAPL"}}),
            webhook_secret: None,
        })
        .expect_err("a runtime without a model port must not accept a workflow run");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 503, "message = {message}");
            assert_eq!(code, "ADK_WORKFLOW_RUNTIME_UNAVAILABLE");
            assert_eq!(message, "assistant model runtime is unavailable");
        }
        other => panic!("expected 503 ADK_WORKFLOW_RUNTIME_UNAVAILABLE, got {other:?}"),
    }

    let logs = port
        .store
        .list_workflow_trigger_logs()
        .expect("list trigger logs");
    assert_eq!(logs.len(), 1, "the rejected run is still audited: {logs:?}");
    assert_eq!(logs[0].status, "FAILED");
    assert_eq!(logs[0].workflow_id, workflow_id);
    let payload: Value =
        serde_json::from_str(&logs[0].payload_json).expect("trigger log payload");
    assert_eq!(payload["errorCode"], "ADK_WORKFLOW_RUNTIME_UNAVAILABLE");
    assert_eq!(
        payload["error"], "assistant model runtime is unavailable",
        "the durable log keeps the reference failure text: {payload}"
    );
}
