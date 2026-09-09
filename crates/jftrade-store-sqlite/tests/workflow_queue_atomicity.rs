use jftrade_store_sqlite::{AdkStore, initialize_current};
use rusqlite::Connection;
use serde_json::{Value, json};

#[test]
fn trigger_schedule_and_queue_commit_together_and_reject_stale_claims() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("adk.db");
    let c = Connection::open(&path).unwrap();
    initialize_current(&c, "adk").unwrap();
    drop(c);
    let store = AdkStore::open(&path).unwrap();
    store
        .upsert_workflow("wf", "ENABLED", r#"{"id":"wf","status":"ENABLED"}"#)
        .unwrap();
    let trigger = store
        .upsert_workflow_trigger(
            "tr",
            "wf",
            "schedule",
            "ENABLED",
            "2026-09-08T00:00:00Z",
            r#"{"id":"tr","status":"ENABLED","config":{"cron":"* * * * *"}}"#,
        )
        .unwrap();
    let next = json!({"id":"tr","status":"ENABLED","nextRunAt":"2026-09-08T00:01:00Z"});
    let c = Connection::open(&path).unwrap();
    c.execute_batch("CREATE TRIGGER reject_queue BEFORE INSERT ON adk_workflow_trigger_logs BEGIN SELECT RAISE(ABORT,'queue failed'); END;").unwrap();
    assert!(
        store
            .enqueue_workflow_trigger_invocations(
                &trigger,
                &next,
                "2026-09-08T00:01:00Z",
                &[("job".into(), json!({}))]
            )
            .is_err()
    );
    assert_eq!(store.get_workflow_trigger("tr").unwrap().unwrap(), trigger);
    assert!(store.list_workflow_trigger_logs().unwrap().is_empty());
    c.execute_batch("DROP TRIGGER reject_queue").unwrap();
    assert!(
        store
            .enqueue_workflow_trigger_invocations(
                &trigger,
                &next,
                "2026-09-08T00:01:00Z",
                &[("job".into(), json!({"scheduledAt":"2026-09-08T00:00:00Z"}))]
            )
            .unwrap()
    );
    assert!(
        !store
            .enqueue_workflow_trigger_invocations(
                &trigger,
                &next,
                "2026-09-08T00:01:00Z",
                &[("duplicate".into(), json!({}))]
            )
            .unwrap()
    );
    drop(store);
    let store = AdkStore::open(&path).unwrap();
    store.recover_orphaned_workflow_trigger_logs().unwrap();
    let rows = store.list_workflow_trigger_logs().unwrap();
    assert_eq!(rows.len(), 1);
    assert_eq!(rows[0].status, "QUEUED");
    let payload: Value = serde_json::from_str(&rows[0].payload_json).unwrap();
    assert!(payload["schedulerInvocation"].is_object());
}
