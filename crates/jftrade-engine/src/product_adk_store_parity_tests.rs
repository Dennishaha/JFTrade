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
