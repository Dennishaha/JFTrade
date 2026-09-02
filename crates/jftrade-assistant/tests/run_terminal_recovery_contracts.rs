use jftrade_assistant::{AssistantRuntime, RunStatus, RuntimeError, Session};
use jftrade_kernel::WireTimestamp;

const RUN_ID: &str = "run-terminal-contract";

fn timestamp() -> WireTimestamp {
    "2026-08-19T00:00:00Z".parse().expect("fixture timestamp")
}

fn later_timestamp() -> WireTimestamp {
    "2026-08-19T00:00:01Z".parse().expect("fixture timestamp")
}

fn runtime_with_run() -> AssistantRuntime {
    let now = timestamp();
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(Session {
        id: "session-terminal-contract".to_owned(),
        agent_id: "agent-terminal-contract".to_owned(),
        title: "Terminal run recovery contract".to_owned(),
        workflow_id: None,
        created_at: now,
        updated_at: now,
    });
    runtime
        .create_run(
            RUN_ID,
            "session-terminal-contract",
            "agent-terminal-contract",
            now,
        )
        .expect("fixture run");
    runtime
}

#[test]
fn cancelled_run_persists_terminal_fields_and_rejects_resume_after_restore() {
    let cancelled_at = later_timestamp();
    let mut runtime = runtime_with_run();
    let transition = runtime
        .transition(RUN_ID, RunStatus::Cancelled, cancelled_at)
        .expect("cancel run");

    assert_eq!(transition.previous, RunStatus::Running);
    assert_eq!(transition.current, RunStatus::Cancelled);
    assert!(transition.changed);
    let run = &runtime.checkpoint().runs[RUN_ID];
    assert_eq!(run.status, RunStatus::Cancelled);
    assert_eq!(run.completed_at, Some(cancelled_at));
    assert_eq!(run.cancelled_at, Some(cancelled_at));

    let checkpoint = runtime.checkpoint_json().expect("checkpoint");
    let mut restored = AssistantRuntime::restore(&checkpoint).expect("restore checkpoint");
    assert_eq!(restored.checkpoint().runs[RUN_ID], *run);
    let audit_count = restored.checkpoint().audit.len();

    let replay = restored
        .transition(RUN_ID, RunStatus::Cancelled, later_timestamp())
        .expect("idempotent cancellation replay");
    assert!(!replay.changed);
    assert_eq!(restored.checkpoint().audit.len(), audit_count);
    assert_eq!(
        restored.transition(RUN_ID, RunStatus::Running, later_timestamp()),
        Err(RuntimeError::InvalidTransition {
            from: RunStatus::Cancelled,
            to: RunStatus::Running,
        })
    );
}

#[test]
fn timed_out_run_remains_terminal_and_preserves_audit_order_after_restore() {
    let now = timestamp();
    let timeout_at = later_timestamp();
    let mut runtime = runtime_with_run();
    runtime
        .transition(RUN_ID, RunStatus::TimedOut, timeout_at)
        .expect("time out run");

    let checkpoint = runtime.checkpoint_json().expect("checkpoint");
    let mut restored = AssistantRuntime::restore(&checkpoint).expect("restore checkpoint");
    let run = &restored.checkpoint().runs[RUN_ID];
    assert_eq!(run.status, RunStatus::TimedOut);
    assert_eq!(run.completed_at, Some(timeout_at));
    assert!(run.cancelled_at.is_none());

    let audit = &restored.checkpoint().audit;
    assert!(
        audit
            .windows(2)
            .all(|events| events[0].sequence < events[1].sequence)
    );
    let audit_count = audit.len();
    assert_eq!(
        restored.transition(RUN_ID, RunStatus::Cancelled, now),
        Err(RuntimeError::InvalidTransition {
            from: RunStatus::TimedOut,
            to: RunStatus::Cancelled,
        })
    );
    assert_eq!(restored.checkpoint().audit.len(), audit_count);
    assert_eq!(
        restored.transition(RUN_ID, RunStatus::Running, now),
        Err(RuntimeError::InvalidTransition {
            from: RunStatus::TimedOut,
            to: RunStatus::Running,
        })
    );
}

#[test]
fn transient_provider_failure_round_trips_and_clears_on_recovery() {
    let failure_at = later_timestamp();
    let mut runtime = runtime_with_run();
    runtime
        .record_provider_failure(
            RUN_ID,
            "NETWORK",
            "temporary provider outage",
            true,
            failure_at,
        )
        .expect("record transient provider failure");
    let checkpoint = runtime.checkpoint_json().expect("checkpoint");
    let mut restored = AssistantRuntime::restore(&checkpoint).expect("restore checkpoint");

    let degraded = &restored.checkpoint().runs[RUN_ID];
    assert_eq!(degraded.status, RunStatus::Running);
    assert!(degraded.degraded);
    assert_eq!(degraded.error_code, "NETWORK");
    assert_eq!(degraded.failure_reason, "temporary provider outage");

    restored
        .clear_provider_failure(RUN_ID, later_timestamp())
        .expect("provider recovery");
    let recovered = &restored.checkpoint().runs[RUN_ID];
    assert_eq!(recovered.status, RunStatus::Running);
    assert!(recovered.degraded);
    assert!(recovered.error_code.is_empty());
    assert!(recovered.failure_reason.is_empty());
}
