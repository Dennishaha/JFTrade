#![forbid(unsafe_code)]

//! Behavior contracts for the ADK run lifecycle writes that Go implements in
//! `StoreCore.SaveRun` / `StoreCore.DeleteSession`.
//!
//! Rust has no full-row `SaveRun`: every run mutation is a
//! status/revision compare-and-swap that only the writer holding the current
//! revision can win.  These tests pin the lifecycle guarantees the Go
//! `store_lifecycle_test.go` rows assert (terminal runs cannot regress, a
//! completed run reopens only with a fresh durable approval, paused workflows
//! keep accepting progress, and user goal pause fields survive a stale
//! snapshot) on top of that CAS boundary.

use std::path::Path;
use std::thread::sleep;
use std::time::Duration;

use jftrade_store_sqlite::{AdkStore, AdkStoreError, CreateAdkRunParams, initialize_current};
use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

/// A revision written in the same millisecond as its predecessor is not a
/// distinguishable fencing token, so tests that compare revisions sleep past
/// the store timestamp resolution instead of relying on wall-clock luck.
const REVISION_TICK: Duration = Duration::from_millis(15);

fn open_store() -> (AdkStore, tempfile::TempDir) {
    let directory = tempdir().expect("temp dir");
    let path = directory.path().join("adk.db");
    initialize_database(&path, "adk");
    (AdkStore::open(&path).expect("open adk store"), directory)
}

fn initialize_database(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

fn seed_run(
    store: &AdkStore,
    id: &str,
    session_id: &str,
    status: &str,
    payload: &Value,
) -> jftrade_store_sqlite::StoredAdkRun {
    store
        .create_run(CreateAdkRunParams {
            id,
            session_id,
            agent_id: "agent-lifecycle",
            status,
            client_request_id: "",
            request_fingerprint: "",
            payload_json: &payload.to_string(),
        })
        .expect("create run")
}

fn run_payload(run: &jftrade_store_sqlite::StoredAdkRun) -> Value {
    serde_json::from_str(&run.payload_json).expect("decode run payload")
}

fn reload(store: &AdkStore, id: &str) -> jftrade_store_sqlite::StoredAdkRun {
    store
        .get_run(id)
        .expect("read run")
        .expect("run must still exist")
}

// Parity: go:452dea11:internal/assistant/engine/workflow_goal_test.go:342 TestGoalWorkflowActivitySnapshotDoesNotDowngradeUserPausedParent
#[test]
fn run_terminal_state_cannot_be_regressed_by_a_stale_running_snapshot() {
    let (store, _directory) = open_store();
    let cancelled = seed_run(
        &store,
        "run-terminal-monotonic",
        "session-terminal-monotonic",
        "CANCELLED",
        &json!({
            "id": "run-terminal-monotonic",
            "status": "CANCELLED",
            "message": "cancelled",
            "errorCode": "RUN_CANCELLED",
            "cancelledAt": "2026-09-20T00:00:00Z",
            "completedAt": "2026-09-20T00:00:00Z",
        }),
    );

    // A stale running snapshot cannot win the status CAS.
    let stale = json!({"id": "run-terminal-monotonic", "status": "RUNNING"});
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-terminal-monotonic",
                "RUNNING",
                &cancelled.updated_at,
                "RUNNING",
                &stale.to_string(),
            )
            .expect("stale running snapshot")
    );
    let stored = reload(&store, "run-terminal-monotonic");
    assert_eq!(stored.status, "CANCELLED");
    let payload = run_payload(&stored);
    assert_eq!(payload["errorCode"], "RUN_CANCELLED");
    assert!(payload["cancelledAt"].is_string());

    // Go lets a cancelled run accept a late final-message enrichment without
    // reviving it; the Rust payload write keeps the CANCELLED status column.
    let mut enriched = payload.clone();
    enriched["finalMessageId"] = Value::String("message-final-after-cancel".to_owned());
    assert!(
        store
            .update_run_payload("run-terminal-monotonic", &enriched.to_string())
            .expect("enrich cancelled run")
    );
    let stored = reload(&store, "run-terminal-monotonic");
    assert_eq!(stored.status, "CANCELLED");
    assert_eq!(
        run_payload(&stored)["finalMessageId"],
        "message-final-after-cancel"
    );
    sleep(REVISION_TICK);

    // A completed run whose workflow is still running is the one intermediate
    // state Go treats as correctable: it cannot fall back to RUNNING, but a
    // terminal correction is accepted.
    let intermediate = seed_run(
        &store,
        "run-terminal-completed-running-workflow",
        "session-terminal-monotonic",
        "COMPLETED",
        &json!({
            "id": "run-terminal-completed-running-workflow",
            "status": "COMPLETED",
            "workMode": "loop",
            "workflowStatus": "RUNNING",
            "message": "completed intermediate workflow state",
            "completedAt": "2026-09-20T00:00:01Z",
        }),
    );
    let stale_running = json!({
        "id": "run-terminal-completed-running-workflow",
        "status": "RUNNING",
        "message": "stale workflow running snapshot",
    });
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-terminal-completed-running-workflow",
                "RUNNING",
                &intermediate.updated_at,
                "RUNNING",
                &stale_running.to_string(),
            )
            .expect("stale completed-running workflow snapshot")
    );
    let corrected = json!({
        "id": "run-terminal-completed-running-workflow",
        "status": "FAILED",
        "workMode": "loop",
        "workflowStatus": "FAILED",
        "message": "workflow max iterations exceeded",
        "errorCode": "WORKFLOW_GOAL_MAX_ITERATIONS_EXCEEDED",
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-terminal-completed-running-workflow",
                "COMPLETED",
                &intermediate.updated_at,
                "FAILED",
                &corrected.to_string(),
            )
            .expect("terminal correction")
    );
    let stored = reload(&store, "run-terminal-completed-running-workflow");
    assert_eq!(stored.status, "FAILED");
    assert_eq!(
        run_payload(&stored)["errorCode"],
        "WORKFLOW_GOAL_MAX_ITERATIONS_EXCEEDED"
    );
}

// Parity: go:452dea11:internal/assistant/engine/workflow_reconcile_test.go:73 TestPendingChildCanReopenCompletedRunningParentWorkflow
#[test]
fn completed_run_reopens_only_for_a_fresh_durable_approval() {
    let (store, _directory) = open_store();
    let completed = seed_run(
        &store,
        "run-reopen-fresh-approval",
        "session-reopen-fresh-approval",
        "COMPLETED",
        &json!({
            "id": "run-reopen-fresh-approval",
            "status": "COMPLETED",
            "completedAt": "2026-09-20T00:00:02Z",
        }),
    );
    store
        .create_approval(
            "approval-reopen-fresh",
            "run-reopen-fresh-approval",
            "agent-lifecycle",
            "PENDING",
            &json!({
                "id": "approval-reopen-fresh",
                "runId": "run-reopen-fresh-approval",
                "agentId": "agent-lifecycle",
                "toolName": "strategy.research_backtest",
                "functionCallId": "function-reopen-fresh",
                "confirmationCallId": "confirmation-reopen-fresh",
                "status": "PENDING",
            })
            .to_string(),
        )
        .expect("stage fresh approval");

    let reopened = json!({
        "id": "run-reopen-fresh-approval",
        "status": "PENDING",
        "resumeState": "waiting_approval",
        "pendingApprovals": [{
            "id": "approval-reopen-fresh",
            "runId": "run-reopen-fresh-approval",
            "agentId": "agent-lifecycle",
            "toolName": "strategy.research_backtest",
            "status": "PENDING",
        }],
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-reopen-fresh-approval",
                "COMPLETED",
                &completed.updated_at,
                "PENDING",
                &reopened.to_string(),
            )
            .expect("reopen completed run")
    );
    let stored = reload(&store, "run-reopen-fresh-approval");
    assert_eq!(stored.status, "PENDING");
    let payload = run_payload(&stored);
    assert!(payload.get("completedAt").is_none());
    assert_eq!(payload["resumeState"], "waiting_approval");
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(1)
    );
    let approvals = store.list_approvals().expect("list approvals");
    assert_eq!(approvals.len(), 1);
    assert_eq!(approvals[0].id, "approval-reopen-fresh");
    assert_eq!(approvals[0].status, "PENDING");

    // The reopen consumed the completed revision, so a replay of the same
    // snapshot is fenced instead of staging the approval twice.
    sleep(REVISION_TICK);
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-reopen-fresh-approval",
                "COMPLETED",
                &completed.updated_at,
                "PENDING",
                &reopened.to_string(),
            )
            .expect("replayed reopen")
    );
    assert_eq!(
        reload(&store, "run-reopen-fresh-approval").status,
        "PENDING"
    );
}

// Parity: go:452dea11:internal/assistant/engine/persistence_failure_boundaries_test.go:111 TestNativeTaskGraphPersistsCompletedAndPendingInputOutcomes
#[test]
fn paused_workflow_run_keeps_accepting_progress_and_terminal_updates() {
    let (store, _directory) = open_store();
    let paused = seed_run(
        &store,
        "run-paused-workflow-updatable",
        "session-paused-workflow-updatable",
        "RUNNING",
        &json!({
            "id": "run-paused-workflow-updatable",
            "status": "RUNNING",
            "workMode": "loop",
            "workflowStatus": "PAUSED",
            "message": "waiting for child approval",
        }),
    );

    let resumed = json!({
        "id": "run-paused-workflow-updatable",
        "status": "RUNNING",
        "workMode": "loop",
        "workflowStatus": "RUNNING",
        "message": "workflow resumed after approval",
        "iteration": 2,
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-paused-workflow-updatable",
                "RUNNING",
                &paused.updated_at,
                "RUNNING",
                &resumed.to_string(),
            )
            .expect("resume paused workflow")
    );
    let stored = reload(&store, "run-paused-workflow-updatable");
    assert_eq!(stored.status, "RUNNING");
    let payload = run_payload(&stored);
    assert_eq!(payload["workflowStatus"], "RUNNING");
    assert_eq!(payload["message"], "workflow resumed after approval");
    assert_eq!(payload["iteration"], 2);

    let completed_at = "2026-09-20T00:01:00Z";
    let completed = json!({
        "id": "run-paused-workflow-updatable",
        "status": "COMPLETED",
        "workMode": "loop",
        "workflowStatus": "COMPLETE",
        "message": "workflow completed after approval",
        "completedAt": completed_at,
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-paused-workflow-updatable",
                "RUNNING",
                &stored.updated_at,
                "COMPLETED",
                &completed.to_string(),
            )
            .expect("complete paused workflow")
    );
    let stored = reload(&store, "run-paused-workflow-updatable");
    assert_eq!(stored.status, "COMPLETED");
    let payload = run_payload(&stored);
    assert_eq!(payload["workflowStatus"], "COMPLETE");
    assert_eq!(payload["completedAt"], completed_at);

    // The completed revision rejects the pre-completion snapshot.
    sleep(REVISION_TICK);
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-paused-workflow-updatable",
                "RUNNING",
                &stored.updated_at,
                "RUNNING",
                &resumed.to_string(),
            )
            .expect("stale paused workflow update")
    );
    assert_eq!(
        reload(&store, "run-paused-workflow-updatable").status,
        "COMPLETED"
    );
}

// Parity: go:452dea11:internal/assistant/engine/workflow_goal_test.go:222 TestGoalWorkflowPauseRequestedBeforeCompleteDecisionPausesInsteadOfCompleting
#[test]
fn user_goal_pause_fields_survive_a_stale_writer_and_clear_on_explicit_resume() {
    let (store, _directory) = open_store();
    let initial = seed_run(
        &store,
        "run-goal-user-resuming",
        "session-goal-pause",
        "RUNNING",
        &json!({
            "id": "run-goal-user-resuming",
            "status": "RUNNING",
            "workMode": "loop",
            "objective": "推进目标",
            "workflowStatus": "RUNNING",
            "message": "goal running",
        }),
    );

    // A writer that does not hold the current revision cannot clear the pause
    // request Go preserves in `preserveUserGoalPauseLifecycle`.
    let paused = json!({
        "id": "run-goal-user-resuming",
        "status": "RUNNING",
        "workMode": "loop",
        "objective": "推进目标",
        "workflowStatus": "RUNNING",
        "message": "目标将在当前轮结束后暂停。",
        "pauseRequestedAt": "2026-09-20T00:02:00Z",
        "resumeState": "user_pause_requested",
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-goal-user-resuming",
                "RUNNING",
                &initial.updated_at,
                "RUNNING",
                &paused.to_string(),
            )
            .expect("request pause")
    );
    let paused_run = reload(&store, "run-goal-user-resuming");
    let paused_payload = run_payload(&paused_run);
    assert_eq!(paused_payload["resumeState"], "user_pause_requested");
    assert!(paused_payload["pauseRequestedAt"].is_string());

    sleep(REVISION_TICK);
    let stale = json!({
        "id": "run-goal-user-resuming",
        "status": "RUNNING",
        "workMode": "loop",
        "objective": "推进目标",
        "workflowStatus": "RUNNING",
        "message": "goal running",
    });
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-goal-user-resuming",
                "RUNNING",
                &initial.updated_at,
                "RUNNING",
                &stale.to_string(),
            )
            .expect("stale snapshot clearing pause fields")
    );
    let stored = reload(&store, "run-goal-user-resuming");
    let payload = run_payload(&stored);
    assert_eq!(payload["resumeState"], "user_pause_requested");
    assert!(payload["pauseRequestedAt"].is_string());
    assert_eq!(payload["message"], "目标将在当前轮结束后暂停。");

    // An explicit resume from the current revision clears the pause fields.
    let resumed = json!({
        "id": "run-goal-user-resuming",
        "status": "RUNNING",
        "workMode": "loop",
        "objective": "推进目标",
        "workflowStatus": "RUNNING",
        "message": "goal resumed",
        "resumeState": "user_resuming",
    });
    assert!(
        store
            .update_run_state_if_status_and_revision(
                "run-goal-user-resuming",
                "RUNNING",
                &stored.updated_at,
                "RUNNING",
                &resumed.to_string(),
            )
            .expect("explicit resume")
    );
    let stored = reload(&store, "run-goal-user-resuming");
    assert_eq!(stored.status, "RUNNING");
    let payload = run_payload(&stored);
    assert!(payload.get("pauseRequestedAt").is_none());
    assert!(payload.get("pausedAt").is_none());
    assert_eq!(payload["resumeState"], "user_resuming");
    assert_eq!(payload["message"], "goal resumed");
}

#[test]
fn session_delete_missing_is_idempotent_and_blank_ids_are_rejected() {
    let (store, _directory) = open_store();

    let blank = store.delete_session("");
    assert!(
        matches!(blank, Err(AdkStoreError::Validation(_))),
        "blank session id must fail closed: {blank:?}"
    );
    let reserved = store.delete_session("user");
    assert!(
        matches!(reserved, Err(AdkStoreError::Validation(_))),
        "reserved shared session must fail closed: {reserved:?}"
    );
    assert!(
        !store
            .delete_session("session-missing")
            .expect("missing session delete is idempotent"),
        "a missing session reports no deleted rows"
    );
}
