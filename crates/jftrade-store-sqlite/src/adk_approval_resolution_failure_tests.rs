#![forbid(unsafe_code)]

use std::path::Path;

use super::{AdkStore, AdkStoreError, CreateAdkRunParams};
use crate::initialize_current;
use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::{TempDir, tempdir};

fn fixture(count: usize) -> (TempDir, AdkStore) {
    let directory = tempdir().expect("directory");
    let path = directory.path().join("adk.db");
    let connection = Connection::open(&path).expect("database");
    initialize_current(&connection, "adk").expect("production schema");
    drop(connection);
    let store = AdkStore::open(&path).expect("production store");
    let approvals = (0..count).map(approval).collect::<Vec<_>>();
    let calls = (0..count)
        .map(|index| {
            json!({
                "id": format!("call-{index}"), "name": format!("write.{index}"),
                "status": "PENDING_APPROVAL", "requiresUser": true,
            })
        })
        .collect::<Vec<_>>();
    store.create_run(CreateAdkRunParams {
        id: "run", session_id: "session", agent_id: "agent", status: "PENDING",
        client_request_id: "request", request_fingerprint: "fingerprint",
        payload_json: &json!({
            "id": "run", "agentId": "agent", "sessionId": "session", "status": "PENDING",
            "resumeState": "waiting_approval", "pendingApprovals": approvals, "toolCalls": calls,
        }).to_string(),
    }).expect("seed run");
    for index in 0..count {
        store
            .create_approval(
                &format!("approval-{index}"),
                "run",
                "agent",
                "PENDING",
                &approval(index).to_string(),
            )
            .expect("seed approval");
    }
    (directory, store)
}

fn approval(index: usize) -> Value {
    json!({
        "id": format!("approval-{index}"), "runId": "run", "agentId": "agent",
        "status": "PENDING", "toolName": format!("write.{index}"),
        "functionCallId": format!("call-{index}"),
        "confirmationCallId": format!("confirmation-{index}"),
    })
}

fn inject(path: &Path, sql: &str) {
    Connection::open(path)
        .expect("fixture connection")
        .execute_batch(sql)
        .expect("inject fixture fault");
}

fn snapshot(store: &AdkStore) -> Value {
    let mut approvals = store.list_approvals().expect("approvals");
    approvals.sort_by(|a, b| a.id.cmp(&b.id));
    let run = store.get_run("run").expect("run query");
    json!({
        "approvals": approvals.iter().map(|row| json!({
            "id": row.id, "status": row.status, "payload": row.payload_json,
            "createdAt": row.created_at, "updatedAt": row.updated_at,
        })).collect::<Vec<_>>(),
        "run": run.map(|row| json!({
            "status": row.status, "payload": row.payload_json,
            "createdAt": row.created_at, "updatedAt": row.updated_at,
        })),
    })
}

fn corrupted_resolution(sql: &str) -> Result<(), AdkStoreError> {
    let (directory, store) = fixture(2);
    drop(store);
    let path = directory.path().join("adk.db");
    inject(&path, sql);
    let store = AdkStore::open(&path).expect("reopen corrupted payload fixture");
    let before = snapshot(&store);
    let result = store.resolve_and_stage_approval("approval-0", "APPROVED");
    assert_eq!(
        snapshot(&store),
        before,
        "resolution and staging roll back together"
    );
    result.map(|_| ())
}

// Parity: go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:75 TestResolveAndStageApprovalRejectsCorruptDurablePayloads
#[test]
fn approval_resolution_rejects_a_corrupt_target_identity_without_mutation() {
    let result = corrupted_resolution(
        "UPDATE adk_approvals SET payload_json = '{\"id\":[],\"status\":\"PENDING\"}' WHERE id = 'approval-0';",
    );
    assert!(
        matches!(result, Err(AdkStoreError::Validation(_))),
        "{result:?}"
    );
}

// Parity: go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:75 TestResolveAndStageApprovalRejectsCorruptDurablePayloads
#[test]
fn approval_resolution_rejects_a_corrupt_run_json_without_mutation() {
    let result = corrupted_resolution("UPDATE adk_runs SET payload_json = '{' WHERE id = 'run';");
    assert!(
        matches!(result, Err(AdkStoreError::Validation(_))),
        "{result:?}"
    );
}

// Parity: go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:75 TestResolveAndStageApprovalRejectsCorruptDurablePayloads
#[test]
fn approval_resolution_rejects_a_corrupt_sibling_identity_without_mutation() {
    let result = corrupted_resolution(
        "UPDATE adk_approvals SET payload_json = '{\"id\":[],\"status\":\"PENDING\"}' WHERE id = 'approval-1';",
    );
    assert!(
        matches!(result, Err(AdkStoreError::Validation(_))),
        "{result:?}"
    );
}

#[test]
fn approval_payload_decoder_rejects_wrong_types_and_accepts_nullable_fields() {
    for field in [
        "id",
        "runId",
        "agentId",
        "toolName",
        "status",
        "reason",
        "functionCallId",
        "confirmationCallId",
        "createdAt",
        "updatedAt",
    ] {
        for invalid in [json!([]), json!({}), json!(1), json!(false)] {
            assert!(super::decode_approval_payload(&json!({field: invalid}).to_string()).is_err());
        }
        for valid in [Value::Null, json!("value")] {
            assert!(super::decode_approval_payload(&json!({field: valid}).to_string()).is_ok());
        }
    }
    assert!(super::decode_approval_payload("{}").is_ok());
    for valid in [Value::Null, json!({"argument": [1, true]})] {
        assert!(super::decode_approval_payload(&json!({"input": valid}).to_string()).is_ok());
    }
    for invalid in [json!([]), json!(1), json!(false), json!("value")] {
        assert!(super::decode_approval_payload(&json!({"input": invalid}).to_string()).is_err());
    }
}

// Parity: go:452dea11:internal/assistant/engine/approval_persistence_failures_test.go:9 TestApprovalPersistenceFailuresRemainObservable
#[test]
fn approval_staging_write_errors_preserve_retryable_approval_and_run_snapshots() {
    for (count, verdict) in [(2, "APPROVED"), (1, "APPROVED"), (2, "DENIED")] {
        let (directory, store) = fixture(count);
        let path = directory.path().join("adk.db");
        store.lock_connection().expect("owner connection").execute_batch(
            "CREATE TEMP TRIGGER reject_run_stage BEFORE UPDATE ON adk_runs BEGIN SELECT RAISE(ABORT, 'reject_run_stage'); END;",
        ).expect("inject write failure on the owner connection");
        let before = snapshot(&store);
        let error = store
            .resolve_and_stage_approval("approval-0", verdict)
            .expect_err("run write fails");
        assert!(matches!(error, AdkStoreError::Query(_)));
        assert!(error.to_string().contains("reject_run_stage"), "{error}");
        assert_eq!(snapshot(&store), before, "count={count}, verdict={verdict}");
        store
            .lock_connection()
            .expect("owner connection")
            .execute_batch("DROP TRIGGER reject_run_stage;")
            .expect("remove write failure");
        drop(store);
        let retry = AdkStore::open(&path).expect("retry store");
        let result = retry
            .resolve_and_stage_approval("approval-0", verdict)
            .expect("retry resolution")
            .expect("approval");
        assert!(result.changed);
        assert_eq!(result.approval.status, verdict);
        assert_eq!(result.should_continue, count == 1 || verdict == "DENIED");
        let run = result.run.expect("staged run");
        assert_eq!(
            run.status,
            if result.should_continue {
                "RUNNING"
            } else {
                "PENDING"
            }
        );
        let payload: Value = serde_json::from_str(&run.payload_json).expect("payload");
        assert_eq!(payload["pendingApprovals"][0]["status"], verdict);
        if verdict == "DENIED" {
            assert!(
                retry
                    .list_approvals()
                    .expect("approvals")
                    .iter()
                    .all(|row| row.status == "DENIED")
            );
            for call in payload["toolCalls"].as_array().expect("calls") {
                assert_eq!(call["status"], "DENIED");
                assert_eq!(call["requiresUser"], false);
            }
        }
    }
}

// Parity: go:452dea11:internal/assistant/engine/approval_stage_boundaries_test.go:8 TestResolveAndStageApprovalBoundaryStates
#[test]
fn approval_resolution_boundary_states_do_not_stage_unrelated_runs() {
    let (directory, store) = fixture(1);
    assert!(
        store
            .resolve_and_stage_approval("missing", "APPROVED")
            .expect("missing")
            .is_none()
    );
    drop(store);
    let path = directory.path().join("adk.db");
    for (fault, changed, returns_run) in [
        (
            "UPDATE adk_approvals SET status='APPROVED' WHERE id='approval-0';",
            false,
            false,
        ),
        ("DELETE FROM adk_runs WHERE id='run';", true, false),
        (
            "UPDATE adk_runs SET status='COMPLETED' WHERE id='run';",
            true,
            false,
        ),
        (
            "UPDATE adk_runs SET payload_json='{\"id\":\"run\",\"status\":\"PENDING\"}' WHERE id='run';",
            true,
            true,
        ),
    ] {
        let (case_dir, case_store) = fixture(1);
        drop(case_store);
        let case_path = case_dir.path().join("adk.db");
        inject(&case_path, fault);
        let case_store = AdkStore::open(&case_path).expect("boundary store");
        let before_run = case_store.get_run("run").expect("run");
        let result = case_store
            .resolve_and_stage_approval("approval-0", if changed { "APPROVED" } else { "DENIED" })
            .expect("boundary resolution")
            .expect("approval");
        assert_eq!(result.changed, changed);
        assert_eq!(result.run.is_some(), returns_run);
        assert!(!result.should_continue);
        assert_eq!(result.approval.status, "APPROVED");
        assert_eq!(
            case_store.get_run("run").expect("run").map(|r| (
                r.status,
                r.payload_json,
                r.updated_at
            )),
            before_run.map(|r| (r.status, r.payload_json, r.updated_at))
        );
    }
    assert!(path.exists());
}

// Parity: go:452dea11:internal/assistant/engine/approval_persistence_failures_test.go:75 TestWorkflowApprovalReconcilerRestagesPersistedResolution
#[test]
fn persisted_approval_resolution_restages_once_after_store_recreation() {
    let (directory, store) = fixture(1);
    store
        .update_approval_status("approval-0", "APPROVED")
        .expect("persist resolution without stage");
    drop(store);
    let store = AdkStore::open(directory.path().join("adk.db")).expect("recreated store");
    let result = store
        .resolve_and_stage_approval("approval-0", "APPROVED")
        .expect("restage")
        .expect("approval");
    assert!(!result.changed);
    assert!(result.should_continue);
    let run = result.run.expect("run");
    assert_eq!(run.status, "RUNNING");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("payload");
    assert_eq!(payload["resumeState"], "approval_resuming");
    assert_eq!(payload["pendingApprovals"][0]["status"], "APPROVED");
    let before = snapshot(&store);
    let duplicate = store
        .resolve_and_stage_approval("approval-0", "APPROVED")
        .expect("duplicate")
        .expect("approval");
    assert!(!duplicate.changed);
    assert!(!duplicate.should_continue);
    assert_eq!(snapshot(&store), before);
    // Parent workflow aggregation is owned by the engine and is not proven here.
}
