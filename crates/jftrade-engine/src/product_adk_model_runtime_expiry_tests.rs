//! Regressions for Go's `Runtime.ReconcileExpiredRuns`.
//!
//! Reference: go:452dea11:internal/assistant/engine/store_test.go
//! `TestReconcileExpiredRunsMarksHungRunTimedOut` (line 947) and
//! `TestReconcileExpiredRunsUsesRunSpecificTimeout` (line 1005).

use std::fs::File;
use std::sync::Arc;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current,
};

use super::{ProductionAdkChatRuntime, RunCancellationRegistry};
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use crate::product::product_production_ports::{ProductionAdkPort, ProductionToolCatalog};
use crate::product::{AdkReadSnapshot, AdkReadSnapshotPort};

fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    (
        directory,
        Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
    )
}

fn runtime_for(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
) -> Arc<ProductionAdkChatRuntime> {
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    ProductionAdkChatRuntime::new(
        Arc::clone(store),
        Arc::clone(session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    )
}

/// RFC3339 timestamp `minutes` before now, matching Go's `NowString` shape.
fn minutes_ago(minutes: i64) -> String {
    (time::OffsetDateTime::now_utc() - time::Duration::minutes(minutes))
        .format(&time::format_description::well_known::Rfc3339)
        .expect("format timestamp")
}

fn seed_running_run(
    store: &AdkStore,
    id: &str,
    started_at: &str,
    max_duration_ms: Option<i64>,
    tool_calls: Value,
) {
    let mut payload = json!({
        "id": id,
        "sessionId": format!("session-{id}"),
        "agentId": "agent-expiry",
        "status": "RUNNING",
        "message": "running",
        "maxDurationMs": max_duration_ms,
        "startedAt": started_at,
        "createdAt": started_at,
        "updatedAt": started_at,
        "pendingApprovals": [],
        "toolCalls": tool_calls,
        "usage": {"modelCalls": 1, "toolCallsTotal": 0},
    });
    if max_duration_ms.is_none() {
        payload
            .as_object_mut()
            .expect("payload object")
            .remove("maxDurationMs");
    }
    store
        .create_run(CreateAdkRunParams {
            id,
            session_id: &format!("session-{id}"),
            agent_id: "agent-expiry",
            status: "RUNNING",
            client_request_id: &format!("request-{id}"),
            request_fingerprint: &format!("fingerprint-{id}"),
            payload_json: &payload.to_string(),
        })
        .expect("seed running run");
}

fn run_payload(store: &AdkStore, id: &str) -> Value {
    let run = store.get_run(id).expect("read run").expect("run row");
    serde_json::from_str(&run.payload_json).expect("decode run payload")
}

fn audit_rows(store: &AdkStore) -> Vec<(String, String, Value)> {
    store
        .list_audit_events()
        .expect("list audit events")
        .into_iter()
        .map(|row| {
            let payload: Value =
                serde_json::from_str(&row.payload_json).expect("decode audit payload");
            (row.kind, row.subject_id, payload)
        })
        .collect()
}

/// Go's `ReconcileExpiredRuns` turns a hung `RUNNING` run into `TIMED_OUT`:
/// the still-RUNNING tool call fails with the expiry reason, the run records
/// `run timed out`, `run exceeded maximum duration of 30m0s`,
/// `RUN_TIMED_OUT`, `degraded=true` and `completedAt`, and the lifecycle audit
/// row lands alongside.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:947
/// `TestReconcileExpiredRunsMarksHungRunTimedOut`.
#[test]
fn expired_running_run_is_reconciled_to_timed_out_with_failed_tool_calls() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    let started_at = minutes_ago(31);
    seed_running_run(
        &store,
        "run-hung",
        &started_at,
        None,
        json!([{
            "id": "tool-1",
            "runId": "run-hung",
            "name": "account.orders",
            "status": "RUNNING",
            "startedAt": started_at,
            "updatedAt": started_at,
        }]),
    );

    runtime
        .reconcile_expired_runs()
        .expect("reconcile expired runs");

    let stored = store
        .get_run("run-hung")
        .expect("read run")
        .expect("run row");
    assert_eq!(stored.status, "TIMED_OUT");
    let payload = run_payload(&store, "run-hung");
    assert_eq!(payload["status"], "TIMED_OUT");
    assert_eq!(payload["message"], "run timed out");
    assert_eq!(
        payload["failureReason"],
        "run exceeded maximum duration of 30m0s"
    );
    assert_eq!(payload["errorCode"], "RUN_TIMED_OUT");
    assert_eq!(payload["degraded"], true);
    assert!(payload["completedAt"].is_string(), "{payload}");
    let calls = payload["toolCalls"].as_array().expect("tool calls");
    assert_eq!(calls[0]["status"], "FAILED", "{payload}");
    assert_eq!(
        calls[0]["error"],
        "run timed out while waiting for model or tool completion"
    );
    assert!(calls[0]["completedAt"].is_string(), "{payload}");
    assert!(
        payload["usage"]["durationMs"]
            .as_i64()
            .is_some_and(|ms| ms > 0),
        "the terminal projection finalizes the run duration: {payload}"
    );

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = rows
        .iter()
        .find(|(kind, _, _)| kind == "run.timed_out")
        .unwrap_or_else(|| panic!("expected a run.timed_out audit row: {rows:?}"));
    assert_eq!(kind, "run.timed_out");
    assert_eq!(subject_id, "run-hung");
    assert_eq!(audit["detail"], "Agent run timed out.");
    assert_eq!(audit["metadata"]["status"], "TIMED_OUT");
    assert_eq!(audit["metadata"]["errorCode"], "RUN_TIMED_OUT");
    assert_eq!(
        audit["metadata"]["failureReason"],
        "run exceeded maximum duration of 30m0s"
    );
    runtime.shutdown();
}

/// Each run is measured against its own frozen budget: the 60s run expires
/// while the 300s run started at the same instant stays RUNNING.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:1005
/// `TestReconcileExpiredRunsUsesRunSpecificTimeout`.
/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_child_finalization_boundaries_test.go:66
/// `TestWorkflowChildrenSkipIdleOrApprovalBlockedFinalization`.
/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:69 TestTaskResumeUsesStoredRunningChildBeforeCompletingParent
/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_executor_boundaries_test.go:110 TestTaskResumeTerminatesParentForStoredTerminalChild
/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_reconcile_ignore_boundaries_test.go:9 TestReconcileWorkflowChildrenIgnoresMissingAndForeignRuns
#[test]
fn expired_runs_use_each_runs_own_timeout_window() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    let started_at = minutes_ago(2);
    seed_running_run(
        &store,
        "run-short-timeout",
        &started_at,
        Some(60_000),
        json!([]),
    );
    seed_running_run(
        &store,
        "run-long-timeout",
        &started_at,
        Some(300_000),
        json!([]),
    );

    runtime
        .reconcile_expired_runs()
        .expect("reconcile expired runs");

    let short = store
        .get_run("run-short-timeout")
        .expect("read short run")
        .expect("short run row");
    assert_eq!(short.status, "TIMED_OUT");
    assert_eq!(
        run_payload(&store, "run-short-timeout")["failureReason"],
        "run exceeded maximum duration of 1m0s"
    );
    let long = store
        .get_run("run-long-timeout")
        .expect("read long run")
        .expect("long run row");
    assert_eq!(
        long.status, "RUNNING",
        "a run inside its own budget must stay live"
    );
    runtime.shutdown();
}

/// A live lease owned by another executor shields the run: the owning worker
/// still heartbeats it, so expiring the row here would double-write state.
#[test]
fn a_fresh_foreign_run_lease_shields_a_run_from_expiry() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    let started_at = minutes_ago(31);
    seed_running_run(&store, "run-leased-elsewhere", &started_at, None, json!([]));
    store
        .claim_run_lease(
            "run-leased-elsewhere",
            "other-runtime",
            std::time::Duration::from_secs(300),
        )
        .expect("claim foreign run lease");

    runtime
        .reconcile_expired_runs()
        .expect("reconcile expired runs");

    let stored = store
        .get_run("run-leased-elsewhere")
        .expect("read run")
        .expect("run row");
    assert_eq!(
        stored.status, "RUNNING",
        "a fresh foreign lease must keep the run alive"
    );
    assert!(
        !audit_rows(&store)
            .iter()
            .any(|(kind, _, _)| kind == "run.timed_out"),
        "a shielded run must not be audited as timed out"
    );
    runtime.shutdown();
}

/// Go's `handleADKRuns` and `handleADKRun` reconcile before serving, so the
/// console never receives the stale `RUNNING` projection of an expired run.
#[test]
fn the_run_read_routes_reconcile_expired_runs_before_serving() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    let artifact_path = directory.path().join("adk-artifact.db");
    initialize_current(
        &Connection::open(&artifact_path).expect("create artifact database"),
        "adk-artifact",
    )
    .expect("initialize artifact schema");
    let port = ProductionAdkPort {
        store: Arc::clone(&store),
        session_store: Arc::clone(&session_store),
        artifact_store: Arc::new(
            AdkArtifactStore::open(&artifact_path).expect("open artifact store"),
        ),
        tool_catalog: Arc::new(ProductionToolCatalog::empty_for_test()),
        settings_path: directory.path().join("settings.json"),
        chat_runtime: Some(Arc::clone(&runtime) as Arc<dyn AdkChatStreamPort>),
    };
    let started_at = minutes_ago(31);
    seed_running_run(&store, "run-route-expiry", &started_at, None, json!([]));

    let AdkReadSnapshot::Json(listing) = port
        .read("/api/v1/adk/runs", "status=TIMED_OUT")
        .expect("runs listing must reconcile first")
    else {
        panic!("the runs route answers JSON");
    };
    assert_eq!(
        listing["page"]["total"], 1,
        "the expired run is filtered as TIMED_OUT: {listing}"
    );
    assert_eq!(listing["runs"][0]["id"], "run-route-expiry");
    assert_eq!(listing["runs"][0]["status"], "TIMED_OUT");

    // A second seed proves the detail route reconciles too (the listing already
    // consumed the first run's expiry).
    seed_running_run(
        &store,
        "run-route-expiry-detail",
        &minutes_ago(31),
        None,
        json!([]),
    );
    let AdkReadSnapshot::Json(detail) = port
        .read("/api/v1/adk/runs/run-route-expiry-detail", "")
        .expect("run detail must reconcile first")
    else {
        panic!("the run detail route answers JSON");
    };
    assert_eq!(detail["status"], "TIMED_OUT", "{detail}");
    assert_eq!(detail["message"], "run timed out");
    runtime.shutdown();
}

/// Go's `time.Duration.String()` for the budgets frozen on `maxDurationMs`.
#[test]
fn go_duration_string_matches_the_reference_formatting() {
    for (ms, expected) in [
        (1, "1ms"),
        (100, "100ms"),
        (1_500, "1.5s"),
        (5_000, "5s"),
        (30_000, "30s"),
        (60_000, "1m0s"),
        (90_000, "1m30s"),
        (300_000, "5m0s"),
        (1_800_000, "30m0s"),
        (2_700_000, "45m0s"),
        (3_600_000, "1h0m0s"),
    ] {
        assert_eq!(
            super::go_duration_string(ms),
            expected,
            "{ms}ms must render like Go's time.Duration.String()"
        );
    }
}
