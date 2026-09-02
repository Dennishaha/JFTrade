use jftrade_assistant::{
    Approval, ApprovalStatus, AssistantRuntime, ClaimError, ClaimStore, InputAnswer,
    InputDecisionKind, InputOptionDraft, InputQuestionDraft, InputRequestDraft, InputRequestStatus,
    RunStatus, RuntimeError, Session, ToolCall, ToolCallStatus, ToolClaimRequest,
    ToolIdempotencyMode, ToolInvocationStatus,
};
use jftrade_kernel::WireTimestamp;
use serde_json::{Value, json};

const RUN_ID: &str = "run-contract";
const OWNER_A: &str = "executor-a";
const OWNER_B: &str = "executor-b";

fn timestamp() -> WireTimestamp {
    "2026-08-19T00:00:00Z".parse().expect("fixture timestamp")
}

fn later_timestamp() -> WireTimestamp {
    "2026-08-19T00:00:01Z".parse().expect("fixture timestamp")
}

fn session() -> Session {
    let now = timestamp();
    Session {
        id: "session-contract".to_owned(),
        agent_id: "agent-contract".to_owned(),
        title: "Assistant contract test".to_owned(),
        workflow_id: None,
        created_at: now,
        updated_at: now,
    }
}

fn runtime_with_run() -> AssistantRuntime {
    let session = session();
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(session.clone());
    runtime
        .create_run(RUN_ID, session.id, session.agent_id, timestamp())
        .expect("fixture run");
    runtime
}

fn tool_request(
    lease: &jftrade_assistant::RunLease,
    key: &str,
    tool_name: &str,
    input: Value,
    mode: ToolIdempotencyMode,
    now_unix_ms: i64,
    ttl_ms: i64,
) -> ToolClaimRequest {
    ToolClaimRequest {
        run_id: lease.run_id.clone(),
        idempotency_key: key.to_owned(),
        tool_name: tool_name.to_owned(),
        owner_id: lease.owner_id.clone(),
        run_lease_token: lease.fencing_token,
        input,
        mode,
        now_unix_ms,
        ttl_ms,
    }
}

fn input_draft() -> InputRequestDraft {
    InputRequestDraft {
        decision_kind: InputDecisionKind::MaterialTradeoff,
        blocking_reason: "The execution mode changes the requested result.".to_owned(),
        title: "Choose execution mode".to_owned(),
        questions: vec![InputQuestionDraft {
            question: "Which execution mode should be used?".to_owned(),
            options: vec![
                InputOptionDraft {
                    label: "Paper".to_owned(),
                    description: "No broker write".to_owned(),
                    recommended: true,
                },
                InputOptionDraft {
                    label: "Live".to_owned(),
                    description: "Requires approval".to_owned(),
                    recommended: false,
                },
            ],
            allow_other: false,
        }],
    }
}

fn approval_fixture(approved: bool) -> Approval {
    let now = timestamp();
    Approval {
        id: "approval-contract".to_owned(),
        run_id: RUN_ID.to_owned(),
        agent_id: "agent-contract".to_owned(),
        tool_name: "trade.submit".to_owned(),
        input: json!({"symbol": "AAPL", "quantity": "1"}),
        status: ApprovalStatus::Pending,
        reason: if approved {
            "The tool may place an order.".to_owned()
        } else {
            "The requested write was denied.".to_owned()
        },
        function_call_id: "function-call-contract".to_owned(),
        confirmation_call_id: "confirmation-contract".to_owned(),
        created_at: now,
        updated_at: now,
    }
}

fn approval_tool_call() -> ToolCall {
    let now = timestamp();
    ToolCall {
        id: "function-call-contract".to_owned(),
        run_id: RUN_ID.to_owned(),
        tool_name: "trade.submit".to_owned(),
        permission: "write".to_owned(),
        status: ToolCallStatus::Pending,
        input: json!({"symbol": "AAPL", "quantity": "1"}),
        output: None,
        error: None,
        requires_user: false,
        idempotency_key: "approval-order-contract".to_owned(),
        created_at: now,
        updated_at: now,
        completed_at: None,
    }
}

#[test]
fn run_lease_expiry_fencing_and_stale_release_are_rejected() {
    let mut store = ClaimStore::default();
    let first = store
        .claim_run(RUN_ID, OWNER_A, 100, 40)
        .expect("initial run lease");
    assert_eq!(first.fencing_token, 1);
    assert_eq!(first.expires_at_unix_ms, 140);
    assert_eq!(
        store.claim_run(RUN_ID, OWNER_B, 139, 40),
        Err(ClaimError::RunLeaseHeld)
    );

    let takeover = store
        .claim_run(RUN_ID, OWNER_B, 140, 40)
        .expect("expired lease takeover");
    assert_eq!(takeover.owner_id, OWNER_B);
    assert_eq!(takeover.fencing_token, 2);
    assert_eq!(
        store.heartbeat_run(&first, 140, 40),
        Err(ClaimError::RunLeaseLost)
    );
    assert!(!store.release_run(&first, 140));
    assert_eq!(store.checkpoint().run_leases[RUN_ID], takeover);
}

#[test]
fn same_owner_heartbeat_renews_without_advancing_fence() {
    let mut store = ClaimStore::default();
    let first = store
        .claim_run(RUN_ID, OWNER_A, 100, 40)
        .expect("initial run lease");
    let claimed_again = store
        .claim_run(RUN_ID, OWNER_A, 120, 50)
        .expect("same-owner claim refresh");
    assert_eq!(claimed_again.fencing_token, first.fencing_token);
    assert_eq!(claimed_again.heartbeat_at_unix_ms, 120);
    assert_eq!(claimed_again.expires_at_unix_ms, 170);

    let heartbeat = store
        .heartbeat_run(&first, 130, 60)
        .expect("same-owner heartbeat");
    assert_eq!(heartbeat.fencing_token, first.fencing_token);
    assert_eq!(heartbeat.heartbeat_at_unix_ms, 130);
    assert_eq!(heartbeat.expires_at_unix_ms, 190);
    assert_eq!(
        store.claim_run(RUN_ID, OWNER_B, 189, 40),
        Err(ClaimError::RunLeaseHeld)
    );
}

#[test]
fn completed_tool_output_replays_after_checkpoint_restore() {
    let mut store = ClaimStore::default();
    let lease = store
        .claim_run(RUN_ID, OWNER_A, 100, 1_000)
        .expect("run lease");
    let request = tool_request(
        &lease,
        "read-aapl",
        "marketdata.quote",
        json!({"symbol": "AAPL"}),
        ToolIdempotencyMode::ReplaySafe,
        100,
        100,
    );
    let ticket = store.claim_tool(request.clone()).expect("first claim");
    assert!(ticket.execute);
    assert!(!ticket.replayed);
    assert_eq!(
        store.claim_tool(request.clone()),
        Err(ClaimError::ToolInvocationInFlight)
    );

    let output = json!({"price": "123.50", "source": "fixture"});
    store
        .complete_tool(&ticket, output.clone(), 150)
        .expect("complete invocation");
    let checkpoint = store.checkpoint_json().expect("claim checkpoint");
    let mut restored = ClaimStore::restore(&checkpoint).expect("restore claim checkpoint");
    let replay = restored.claim_tool(ToolClaimRequest {
        now_unix_ms: 151,
        ..request
    });
    let replay = replay.expect("completed replay");
    assert!(!replay.execute);
    assert!(replay.replayed);
    assert_eq!(replay.output, Some(output));
    assert_eq!(
        restored
            .checkpoint()
            .tool_invocations
            .values()
            .next()
            .map(|invocation| invocation.status),
        Some(ToolInvocationStatus::Completed)
    );
}

#[test]
fn stale_tool_completion_is_rejected_after_keyed_takeover() {
    let mut store = ClaimStore::default();
    let first_lease = store
        .claim_run(RUN_ID, OWNER_A, 100, 30)
        .expect("initial run lease");
    let first_ticket = store
        .claim_tool(tool_request(
            &first_lease,
            "submit-aapl",
            "orders.submit",
            json!({"symbol": "AAPL", "quantity": "1"}),
            ToolIdempotencyMode::Keyed,
            100,
            20,
        ))
        .expect("initial keyed claim");

    let second_lease = store
        .claim_run(RUN_ID, OWNER_B, 131, 100)
        .expect("expired run lease takeover");
    let second_ticket = store
        .claim_tool(tool_request(
            &second_lease,
            "submit-aapl",
            "orders.submit",
            json!({"symbol": "AAPL", "quantity": "1"}),
            ToolIdempotencyMode::Keyed,
            131,
            20,
        ))
        .expect("keyed invocation takeover");
    assert_eq!(second_ticket.fencing_token, first_ticket.fencing_token + 1);
    assert_eq!(second_ticket.run_lease_token, second_lease.fencing_token);
    assert_eq!(
        store.complete_tool(&first_ticket, json!({"stale": true}), 131),
        Err(ClaimError::RunLeaseLost)
    );
    store
        .complete_tool(&second_ticket, json!({"accepted": true}), 132)
        .expect("current owner completion");
}

#[test]
fn expired_tool_invocation_takeover_fences_old_ticket_with_live_run_lease() {
    let mut store = ClaimStore::default();
    let lease = store
        .claim_run(RUN_ID, OWNER_A, 100, 1_000)
        .expect("long-lived run lease");
    let first_ticket = store
        .claim_tool(tool_request(
            &lease,
            "read-expired",
            "marketdata.quote",
            json!({"symbol": "AAPL"}),
            ToolIdempotencyMode::Keyed,
            100,
            20,
        ))
        .expect("initial invocation claim");
    let replacement = store
        .claim_tool(tool_request(
            &lease,
            "read-expired",
            "marketdata.quote",
            json!({"symbol": "AAPL"}),
            ToolIdempotencyMode::Keyed,
            121,
            20,
        ))
        .expect("expired invocation takeover");
    assert!(replacement.execute);
    assert_eq!(replacement.owner_id, lease.owner_id);
    assert_eq!(replacement.fencing_token, first_ticket.fencing_token + 1);
    assert_eq!(replacement.run_lease_token, lease.fencing_token);
    assert_eq!(
        store.complete_tool(&first_ticket, json!({"stale": true}), 122),
        Err(ClaimError::ToolInvocationLost)
    );
    store
        .complete_tool(&replacement, json!({"price": "123.50"}), 123)
        .expect("replacement completion");
    assert_eq!(
        store
            .checkpoint()
            .tool_invocations
            .values()
            .next()
            .map(|invocation| invocation.status),
        Some(ToolInvocationStatus::Completed)
    );
}

#[test]
fn fail_closed_expiry_becomes_indeterminate_and_keyed_expiry_can_take_over() {
    let mut store = ClaimStore::default();
    let first_lease = store
        .claim_run(RUN_ID, OWNER_A, 100, 30)
        .expect("initial run lease");
    let fail_closed = tool_request(
        &first_lease,
        "submit-fail-closed",
        "orders.submit",
        json!({"symbol": "AAPL", "quantity": "1"}),
        ToolIdempotencyMode::FailClosed,
        100,
        20,
    );
    store
        .claim_tool(fail_closed.clone())
        .expect("first write claim");
    assert_eq!(
        store.claim_tool(ToolClaimRequest {
            now_unix_ms: 121,
            ..fail_closed.clone()
        }),
        Err(ClaimError::ToolOutcomeUnknown)
    );
    assert_eq!(
        store.claim_tool(ToolClaimRequest {
            now_unix_ms: 122,
            ..fail_closed.clone()
        }),
        Err(ClaimError::ToolOutcomeUnknown)
    );
    assert_eq!(
        store
            .checkpoint()
            .tool_invocations
            .values()
            .next()
            .map(|invocation| invocation.status),
        Some(ToolInvocationStatus::Indeterminate)
    );

    let keyed = tool_request(
        &first_lease,
        "submit-keyed",
        "orders.submit.keyed",
        json!({"symbol": "AAPL", "quantity": "1"}),
        ToolIdempotencyMode::Keyed,
        100,
        20,
    );
    let keyed_ticket = store.claim_tool(keyed.clone()).expect("keyed claim");
    let second_lease = store
        .claim_run(RUN_ID, OWNER_B, 131, 100)
        .expect("takeover after run lease expiry");
    let takeover = store
        .claim_tool(ToolClaimRequest {
            owner_id: second_lease.owner_id.clone(),
            run_lease_token: second_lease.fencing_token,
            now_unix_ms: 131,
            ..keyed
        })
        .expect("keyed stale invocation takeover");
    assert_eq!(takeover.fencing_token, keyed_ticket.fencing_token + 1);
}

#[test]
fn input_resolution_is_idempotent_and_conflict_safe() {
    let now = timestamp();
    let mut runtime = runtime_with_run();
    let request = runtime
        .request_input(
            RUN_ID,
            "input-contract",
            "function-call-input",
            input_draft(),
            now,
        )
        .expect("input request");
    assert_eq!(request.status, InputRequestStatus::Pending);
    assert_eq!(
        runtime.checkpoint().runs[RUN_ID].status,
        RunStatus::PendingInput
    );

    let invalid = vec![InputAnswer {
        question_id: "q1".to_owned(),
        option_id: "missing-option".to_owned(),
        other_text: String::new(),
    }];
    assert_eq!(
        runtime.answer_input(RUN_ID, &request.id, invalid, later_timestamp()),
        Err(RuntimeError::InvalidInputAnswers)
    );
    assert_eq!(
        runtime.checkpoint().runs[RUN_ID].input_requests[0].status,
        InputRequestStatus::Pending
    );

    let answers = vec![InputAnswer {
        question_id: "q1".to_owned(),
        option_id: "q1-o1".to_owned(),
        other_text: String::new(),
    }];
    assert!(
        runtime
            .answer_input(RUN_ID, &request.id, answers.clone(), now)
            .expect("answer")
    );
    assert!(
        !runtime
            .answer_input(RUN_ID, &request.id, answers, later_timestamp())
            .expect("idempotent replay")
    );
    assert_eq!(
        runtime.answer_input(
            RUN_ID,
            &request.id,
            vec![InputAnswer {
                question_id: "q1".to_owned(),
                option_id: "q1-o2".to_owned(),
                other_text: String::new(),
            }],
            later_timestamp(),
        ),
        Err(RuntimeError::InputConflict)
    );
    let run = &runtime.checkpoint().runs[RUN_ID];
    assert_eq!(run.status, RunStatus::Running);
    assert!(run.input_request.is_none());
    assert_eq!(run.input_requests[0].answers[0].option_id, "q1-o1");
}

#[test]
fn approval_resolution_is_idempotent_and_conflict_safe() {
    for approved in [true, false] {
        let now = timestamp();
        let mut runtime = runtime_with_run();
        runtime
            .request_approval(approval_fixture(approved), approval_tool_call(), now)
            .expect("approval request");
        assert!(
            runtime
                .resolve_approval(RUN_ID, "approval-contract", approved, now)
                .expect("first resolution")
        );
        assert!(
            !runtime
                .resolve_approval(RUN_ID, "approval-contract", approved, later_timestamp(),)
                .expect("idempotent resolution")
        );
        assert_eq!(
            runtime.resolve_approval(RUN_ID, "approval-contract", !approved, later_timestamp(),),
            Err(RuntimeError::ApprovalConflict)
        );
        let run = &runtime.checkpoint().runs[RUN_ID];
        assert_eq!(
            run.pending_approvals[0].status,
            if approved {
                ApprovalStatus::Approved
            } else {
                ApprovalStatus::Denied
            }
        );
        assert_eq!(run.status, RunStatus::Running);
        assert_eq!(
            run.tool_calls[0].status,
            if approved {
                ToolCallStatus::Running
            } else {
                ToolCallStatus::Denied
            }
        );
    }
}

const RUN_STATUSES: [RunStatus; 9] = [
    RunStatus::Running,
    RunStatus::Completed,
    RunStatus::PendingApproval,
    RunStatus::PendingInput,
    RunStatus::Failed,
    RunStatus::Denied,
    RunStatus::Cancelled,
    RunStatus::TimedOut,
    RunStatus::Paused,
];

fn runtime_at_status(status: RunStatus) -> AssistantRuntime {
    let now = timestamp();
    let mut runtime = runtime_with_run();
    match status {
        RunStatus::Running => {}
        RunStatus::PendingApproval => {
            runtime
                .request_approval(approval_fixture(true), approval_tool_call(), now)
                .expect("pending approval setup");
        }
        RunStatus::PendingInput => {
            runtime
                .request_input(
                    RUN_ID,
                    "input-transition",
                    "call-transition",
                    input_draft(),
                    now,
                )
                .expect("pending input setup");
        }
        RunStatus::Paused => {
            runtime
                .transition(RUN_ID, RunStatus::Paused, now)
                .expect("paused setup");
        }
        RunStatus::Denied => {
            runtime
                .request_approval(approval_fixture(false), approval_tool_call(), now)
                .expect("denied setup approval");
            runtime
                .transition(RUN_ID, RunStatus::Denied, now)
                .expect("denied setup");
        }
        RunStatus::Completed | RunStatus::Failed | RunStatus::Cancelled | RunStatus::TimedOut => {
            runtime
                .transition(RUN_ID, status, now)
                .expect("terminal setup");
        }
    }
    runtime
}

fn transition_allowed(from: RunStatus, to: RunStatus) -> bool {
    if from == to {
        return true;
    }
    match from {
        RunStatus::Running => matches!(
            to,
            RunStatus::Completed
                | RunStatus::PendingApproval
                | RunStatus::PendingInput
                | RunStatus::Failed
                | RunStatus::Cancelled
                | RunStatus::TimedOut
                | RunStatus::Paused
        ),
        RunStatus::PendingApproval => matches!(
            to,
            RunStatus::Running | RunStatus::Denied | RunStatus::Cancelled | RunStatus::TimedOut
        ),
        RunStatus::PendingInput => {
            matches!(
                to,
                RunStatus::Running | RunStatus::Cancelled | RunStatus::TimedOut
            )
        }
        RunStatus::Paused => {
            matches!(
                to,
                RunStatus::Running | RunStatus::Cancelled | RunStatus::TimedOut
            )
        }
        RunStatus::Completed
        | RunStatus::Failed
        | RunStatus::Denied
        | RunStatus::Cancelled
        | RunStatus::TimedOut => false,
    }
}

#[test]
fn run_transition_matrix_rejects_invalid_edges_and_keeps_self_transitions_idempotent() {
    for from in RUN_STATUSES {
        for to in RUN_STATUSES {
            let mut runtime = runtime_at_status(from);
            let result = runtime.transition(RUN_ID, to, later_timestamp());
            if transition_allowed(from, to) {
                let result = result.expect("allowed transition");
                assert_eq!(result.previous, from);
                assert_eq!(result.current, to);
                assert_eq!(result.changed, from != to);
            } else {
                assert_eq!(result, Err(RuntimeError::InvalidTransition { from, to }));
            }
        }
    }
}

#[test]
fn checkpoint_restore_rejects_malformed_audit_and_unknown_fields() {
    let mut runtime = runtime_with_run();
    runtime
        .transition(RUN_ID, RunStatus::Completed, later_timestamp())
        .expect("audit-producing transition");
    let checkpoint = runtime.checkpoint_json().expect("runtime checkpoint");
    let base: Value = serde_json::from_slice(&checkpoint).expect("checkpoint JSON");

    let mut malformed_audit = base.clone();
    let audit = malformed_audit["audit"]
        .as_array_mut()
        .expect("audit array");
    let first_sequence = audit[0]["sequence"].as_u64().expect("sequence");
    audit[1]["sequence"] = json!(first_sequence);
    let malformed_bytes = serde_json::to_vec(&malformed_audit).expect("malformed JSON");
    assert!(matches!(
        AssistantRuntime::restore(&malformed_bytes),
        Err(RuntimeError::InvalidCheckpoint(message)) if message.contains("strictly increasing")
    ));

    let mut unknown_runtime_field = base;
    unknown_runtime_field
        .as_object_mut()
        .expect("checkpoint object")
        .insert("unknownField".to_owned(), json!(true));
    let unknown_bytes = serde_json::to_vec(&unknown_runtime_field).expect("unknown JSON");
    assert!(matches!(
        AssistantRuntime::restore(&unknown_bytes),
        Err(RuntimeError::InvalidCheckpoint(message)) if message.contains("unknown field")
    ));

    let claims = ClaimStore::default()
        .checkpoint_json()
        .expect("claim checkpoint");
    let mut unknown_claim_field: Value = serde_json::from_slice(&claims).expect("claim JSON");
    unknown_claim_field
        .as_object_mut()
        .expect("claim object")
        .insert("unknownField".to_owned(), json!(true));
    let unknown_claim_bytes = serde_json::to_vec(&unknown_claim_field).expect("unknown claim JSON");
    assert!(matches!(
        ClaimStore::restore(&unknown_claim_bytes),
        Err(ClaimError::InvalidCheckpoint(message)) if message.contains("unknown field")
    ));
}
