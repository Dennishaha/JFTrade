// Behavior tests for the ADK model runtime lifecycle.
//
// Split out of `product_adk_model_runtime_lifecycle.rs` so the production
// fragment stays under the workspace 800-line architecture limit. The
// module is pulled in with `#[path]` from the same parent module, so the
// visibility and imports are unchanged.

use super::*;
use jftrade_store_sqlite::RecordAdkEventParams;
use jftrade_store_sqlite::initialize_current;
use rusqlite::Connection;
use std::fs::File;
use std::sync::Barrier;
use std::thread;
use tempfile::tempdir;

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

#[test]
fn concurrent_first_delivery_creates_one_durable_run_and_event() {
    let (_directory, store, session_store) = initialized_stores();
    session_store
        .upsert_session("jftrade", "local", "session-race", "{}")
        .expect("seed session");
    let barrier = Arc::new(Barrier::new(2));
    let workers = ["run-race-a", "run-race-b"].map(|run_id| {
        let store = Arc::clone(&store);
        let session_store = Arc::clone(&session_store);
        let barrier = Arc::clone(&barrier);
        thread::spawn(move || {
            barrier.wait();
            let event_id = format!("{run_id}:user");
            store
                .create_run_with_event_idempotent(
                    CreateAdkRunParams {
                        id: run_id,
                        session_id: "session-race",
                        agent_id: "agent-race",
                        status: "RUNNING",
                        client_request_id: "request-race",
                        request_fingerprint: "fingerprint-race",
                        payload_json: "{\"status\":\"RUNNING\"}",
                    },
                    session_store.as_ref(),
                    &AdkRunEvent {
                        id: &event_id,
                        session_id: "session-race",
                        invocation_id: run_id,
                        author: "user",
                        content: "hello",
                    },
                    run_id,
                    Duration::from_secs(1),
                )
                .expect("create or load run")
        })
    });
    let outcomes = workers.map(|worker| worker.join().expect("join first delivery"));
    assert_eq!(
        outcomes.iter().filter(|(_, lease)| lease.is_some()).count(),
        1
    );
    assert_eq!(outcomes[0].0.id, outcomes[1].0.id);
    assert_eq!(
        session_store
            .list_events("session-race")
            .expect("list initial events")
            .len(),
        1
    );
}

#[test]
fn tool_claim_heartbeat_is_live_then_becomes_fenced_takeover() {
    let (_directory, store, _session_store) = initialized_stores();
    let run = store
        .create_run(CreateAdkRunParams {
            id: "run-claim",
            session_id: "session-claim",
            agent_id: "agent-claim",
            status: "RUNNING",
            client_request_id: "request-claim",
            request_fingerprint: "fingerprint-claim",
            payload_json: "{\"status\":\"RUNNING\"}",
        })
        .expect("create claim run");
    let first_lease = store
        .claim_run_lease("run-claim", "owner-first", Duration::from_secs(1))
        .expect("claim first run lease");
    let first_claim = match store
        .claim_tool_invocation_if_status_and_revision(
            "run-claim",
            "call-claim",
            "tools.search",
            "{}",
            "RUNNING",
            &run.updated_at,
            "owner-first",
            first_lease.fencing_token,
            Duration::from_millis(100),
            false,
        )
        .expect("claim first tool invocation")
    {
        AdkToolInvocationClaim::Execute(invocation) => invocation,
        other => panic!("unexpected first claim: {other:?}"),
    };
    let first_claim = store
        .heartbeat_tool_invocation(&first_claim, Duration::from_millis(250))
        .expect("heartbeat tool claim");
    assert!(
        store
            .release_run_lease(&first_lease)
            .expect("release first lease")
    );
    let second_lease = store
        .claim_run_lease("run-claim", "owner-second", Duration::from_secs(1))
        .expect("claim second run lease");
    assert!(matches!(
        store
            .claim_tool_invocation_if_status_and_revision(
                "run-claim",
                "call-claim",
                "tools.search",
                "{}",
                "RUNNING",
                &run.updated_at,
                "owner-second",
                second_lease.fencing_token,
                Duration::from_millis(100),
                false,
            )
            .expect("observe live claim"),
        AdkToolInvocationClaim::Live(_)
    ));
    let remaining = first_claim
        .lease_expires_at_unix_ms
        .saturating_sub(unix_now_ms())
        .max(0) as u64;
    thread::sleep(Duration::from_millis(remaining.saturating_add(25)));
    let takeover = store
        .claim_tool_invocation_if_status_and_revision(
            "run-claim",
            "call-claim",
            "tools.search",
            "{}",
            "RUNNING",
            &run.updated_at,
            "owner-second",
            second_lease.fencing_token,
            Duration::from_millis(100),
            false,
        )
        .expect("take over expired claim");
    let AdkToolInvocationClaim::Execute(takeover) = takeover else {
        panic!("expired claim was not executable");
    };
    assert!(takeover.fencing_token > first_claim.fencing_token);
    assert_eq!(takeover.run_lease_token, second_lease.fencing_token);
}

#[test]
fn durable_error_classification_keeps_invariants_fatal() {
    assert_eq!(
        classify_durable_store_error(&AdkStoreError::LeaseLost("lost".to_owned())),
        DurableErrorClass::LeaseHeldOrLost
    );
    assert_eq!(
        classify_durable_store_error(&AdkStoreError::Invariant("mismatch".to_owned())),
        DurableErrorClass::InvariantViolation
    );
    assert!(matches!(
        runtime_store_error(AdkStoreError::Invariant("mismatch".to_owned())),
        AdkChatPortError::Failed { ref code, .. } if code == "ADK_STORAGE_CORRUPT"
    ));
}

#[test]
fn cancellation_registry_fans_out_and_unregisters_exact_token() {
    let registry = RunCancellationRegistry::default();
    let first = registry.register("run-fanout");
    let second = registry.register("run-fanout");

    registry.unregister("run-fanout", &first);
    assert!(registry.cancel("run-fanout"));
    assert!(!first.load(Ordering::Acquire));
    assert!(second.load(Ordering::Acquire));

    registry.unregister("run-fanout", &second);
    assert!(!registry.cancel("run-fanout"));
}

#[test]
fn compacted_context_survives_restart_and_precedes_current_user_message() {
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

    {
        let store = AdkStore::open(&adk_path).expect("open ADK store");
        let session_store = AdkSessionStore::open(&session_path).expect("open session store");
        session_store
            .upsert_session("jftrade", "local", "session-1", "{}")
            .expect("seed session");
        for (id, invocation_id, author, content) in [
            ("event-01", "run-old-1", "user", "first question"),
            ("event-02", "run-old-1", "assistant", "first answer"),
            ("event-03", "run-old-2", "user", "latest durable question"),
            ("event-04", "run-current", "user", "current request"),
        ] {
            session_store
                .record_event(RecordAdkEventParams {
                    id,
                    app_name: "jftrade",
                    user_id: "local",
                    session_id: "session-1",
                    invocation_id,
                    author,
                    content,
                })
                .expect("seed session event");
        }
        store
            .upsert_session_context(
                "session-1",
                r#"{"contextRevisionId":"revision-1","compactedEventCount":2,"summaryPreview":"compacted summary"}"#,
            )
            .expect("persist compacted context");
        store
            .save_handoff_segment(
                "session-1",
                "handoff-1",
                1,
                r#"{"endEventIndex":2,"summary":"handoff summary"}"#,
            )
            .expect("persist handoff segment");
    }

    // Reopen both stores to prove the model payload is rebuilt from the
    // durable compaction rows rather than process-local state.
    let store = AdkStore::open(&adk_path).expect("reopen ADK store");
    let session_store = AdkSessionStore::open(&session_path).expect("reopen session store");
    let context =
        durable_context_items(&store, &session_store, "session-1", Some("run-current"))
            .expect("build durable context");
    let request = ModelRequest {
        endpoint: Url::parse("https://example.test/responses").expect("endpoint"),
        api_key: "secret".to_owned(),
        model: "fixture-model".to_owned(),
        instruction: Some("system instruction".to_owned()),
        message: "current request".to_owned(),
        durable_context: context,
        tool_context: Vec::new(),
        timeout: Duration::from_secs(1),
        tools: Vec::new(),
    };

    let input = model_input(&request);
    assert_eq!(
        input[0],
        json!({"role":"system","content":"system instruction"})
    );
    assert_eq!(
        input[1],
        json!({
            "role":"system",
            "content":"Durable session context:\nhandoff summary\n\ncompacted summary"
        })
    );
    assert_eq!(
        input[2],
        json!({"role":"user","content":"latest durable question"})
    );
    assert_eq!(input[3], json!({"role":"user","content":"current request"}));
    assert_eq!(input.len(), 4, "current event must not be duplicated");
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:745
/// TestUnrecoverablePendingApprovalRunIsMarkedOrphanedOnRestart.
///
/// The reference `reconcileStaleRuns` fails a stale `PENDING` run whose
/// approval rows carry no ADK confirmation identifiers as
/// `FAILED/RUN_ORPHANED` with `resumeState=approval_context_missing`; a
/// pending run that still has the identifiers stays resumable and is left for
/// the approval-continuation path.  The Rust startup path previously left the
/// first run pending forever.
#[test]
fn orphaned_pending_approval_runs_are_failed_on_startup_reconcile() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    store
        .create_run(CreateAdkRunParams {
            id: "run-orphaned-pending",
            session_id: "session-orphaned",
            agent_id: "agent-orphaned",
            status: "PENDING",
            client_request_id: "request-orphaned",
            request_fingerprint: "fingerprint-orphaned",
            payload_json: r#"{
                "id":"run-orphaned-pending",
                "sessionId":"session-orphaned",
                "agentId":"agent-orphaned",
                "status":"PENDING",
                "message":"waiting approval",
                "pendingApprovals":[{
                    "id":"approval-orphaned",
                    "runId":"run-orphaned-pending",
                    "agentId":"agent-orphaned",
                    "toolName":"strategy.save_draft",
                    "status":"PENDING",
                    "reason":"needs approval"
                }],
                "toolCalls":[{
                    "id":"call-orphaned",
                    "name":"strategy.save_draft",
                    "status":"PENDING_APPROVAL",
                    "requiresUser":true
                }]
            }"#,
        })
        .expect("seed unrecoverable pending run");
    // A resumable sibling keeps its confirmation identifiers and must stay
    // pending through the same reconcile pass.
    store
        .create_run(CreateAdkRunParams {
            id: "run-resumable-pending",
            session_id: "session-resumable",
            agent_id: "agent-resumable",
            status: "PENDING",
            client_request_id: "request-resumable",
            request_fingerprint: "fingerprint-resumable",
            payload_json: r#"{
                "id":"run-resumable-pending",
                "sessionId":"session-resumable",
                "agentId":"agent-resumable",
                "status":"PENDING",
                "pendingApprovals":[{
                    "id":"approval-resumable",
                    "runId":"run-resumable-pending",
                    "agentId":"agent-resumable",
                    "toolName":"strategy.save_draft",
                    "status":"PENDING",
                    "functionCallId":"call-resumable",
                    "confirmationCallId":"call-resumable:confirmation"
                }]
            }"#,
        })
        .expect("seed resumable pending run");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let orphaned = store
        .get_run("run-orphaned-pending")
        .expect("read orphaned run")
        .expect("orphaned run exists");
    assert_eq!(
        orphaned.status, "FAILED",
        "an unrecoverable pending run becomes terminal: {orphaned:?}"
    );
    let payload: Value = serde_json::from_str(&orphaned.payload_json).expect("payload");
    assert_eq!(payload["errorCode"], "RUN_ORPHANED");
    assert_eq!(payload["resumeState"], "approval_context_missing");
    assert_eq!(
        payload["message"], "pending approval run lost its resumable approval context",
        "the orphan projection carries its own message: {payload}"
    );

    let resumable = store
        .get_run("run-resumable-pending")
        .expect("read resumable run")
        .expect("resumable run exists");
    assert_eq!(
        resumable.status, "PENDING",
        "a pending run with the ADK confirmation ids stays resumable"
    );
    runtime.shutdown();
}

/// A loopback AI Platform-compatible endpoint that answers one non-streaming
/// Responses request with a plain text completion.  Serves exactly one
/// connection, then returns; the caller joins the thread.
fn spawn_loopback_json_provider(text: &'static str) -> (String, std::thread::JoinHandle<()>) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind loopback provider");
    let address = listener.local_addr().expect("provider address");
    let handle = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept model request");
        let mut request = Vec::new();
        let mut chunk = [0_u8; 4096];
        let mut expected = None;
        loop {
            let count = std::io::Read::read(&mut stream, &mut chunk).expect("read model request");
            if count == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..count]);
            if expected.is_none()
                && let Some(headers_end) = request.windows(4).position(|w| w == b"\r\n\r\n")
            {
                let headers_end = headers_end + 4;
                let headers = String::from_utf8_lossy(&request[..headers_end]).to_ascii_lowercase();
                let length = headers
                    .lines()
                    .find_map(|line| line.strip_prefix("content-length:"))
                    .and_then(|value| value.trim().parse::<usize>().ok())
                    .unwrap_or_default();
                expected = Some(headers_end + length);
            }
            if expected.is_some_and(|expected| request.len() >= expected) {
                break;
            }
        }
        let body = format!("{{\"output_text\":\"{text}\"}}");
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        let _ = std::io::Write::write_all(&mut stream, response.as_bytes());
        let _ = std::io::Write::flush(&mut stream);
        std::thread::sleep(std::time::Duration::from_millis(200));
    });
    (
        format!("http://{}:{}/v1/responses", address.ip(), address.port()),
        handle,
    )
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:621
/// TestApprovalResumingRunIsRecoveredAfterRuntimeRestart.
///
/// Go persists `status=RUNNING` with `resumeState=approval_resuming` and the
/// released tool call, then rebuilds the runtime.  `ReconcileResolvedApprovals`
/// re-claims the run, executes the released tool exactly once, and finishes
/// `COMPLETED`.  The Rust startup path must recover the same durable state
/// without a second execution and without re-grouping the resolved approval on
/// the session timeline.
#[test]
fn an_approval_resuming_run_is_recovered_after_a_runtime_restart() {
    let (endpoint, provider) = spawn_loopback_json_provider("recovered after restart");
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    session_store
        .upsert_session("jftrade", "local", "session-restart-recovery", "{}")
        .expect("seed session");
    store
        .upsert_provider(
            "provider-restart",
            &json!({
                "id": "provider-restart",
                "displayName": "Restart Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "enabled": true,
                // The inline key keeps the fixture self-contained; the
                // production path reads the same value from the secret store.
                "apiKey": "sk-fixture",
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-restart",
            &json!({
                "id": "agent-restart",
                "name": "Restart Agent",
                "providerId": "provider-restart",
                "permissionMode": "approval",
                "status": "ENABLED",
                "tools": ["tools.search"],
                "toolAccessMode": "selected",
            })
            .to_string(),
        )
        .expect("persist agent");
    store
        .create_run(CreateAdkRunParams {
            id: "run-restart-recovery",
            session_id: "session-restart-recovery",
            agent_id: "agent-restart",
            status: "RUNNING",
            client_request_id: "request-restart-recovery",
            request_fingerprint: "fingerprint-restart-recovery",
            payload_json: &json!({
                "id": "run-restart-recovery",
                "sessionId": "session-restart-recovery",
                "agentId": "agent-restart",
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "approval_resuming",
                "requestMessage": "search the catalog",
                "toolCalls": [{
                    "id": "call-restart",
                    "name": "tools.search",
                    "arguments": {"query": "pine"},
                    "status": "RUNNING",
                    "requiresUser": false,
                }],
                "toolResults": [],
                "pendingApprovals": [{
                    "id": "approval-restart",
                    "runId": "run-restart-recovery",
                    "agentId": "agent-restart",
                    "toolName": "tools.search",
                    "status": "APPROVED",
                    "functionCallId": "call-restart",
                    "confirmationCallId": "call-restart:confirmation",
                }],
            })
            .to_string(),
        })
        .expect("seed resumed run");
    store
        .create_approval(
            "approval-restart",
            "run-restart-recovery",
            "agent-restart",
            "APPROVED",
            &json!({
                "id": "approval-restart",
                "runId": "run-restart-recovery",
                "agentId": "agent-restart",
                "toolName": "tools.search",
                "status": "APPROVED",
                "functionCallId": "call-restart",
                "confirmationCallId": "call-restart:confirmation",
            })
            .to_string(),
        )
        .expect("seed resolved approval");

    // Restarting the runtime is the whole trigger: the constructor runs the
    // startup reconcile that re-claims every resumable run.
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let deadline = std::time::Instant::now() + Duration::from_secs(10);
    let completed = loop {
        let run = store
            .get_run("run-restart-recovery")
            .expect("read recovered run")
            .expect("run row");
        if run.status.eq_ignore_ascii_case("COMPLETED") {
            break run;
        }
        assert!(
            std::time::Instant::now() < deadline,
            "the recovered continuation must finish the run, last status {status}: {run:?}",
            status = run.status
        );
        std::thread::sleep(Duration::from_millis(25));
    };
    provider.join().expect("loopback provider thread");

    let payload: Value = serde_json::from_str(&completed.payload_json).expect("run payload");
    assert_eq!(payload["status"], "COMPLETED");
    assert_eq!(
        payload["resumeState"], "adk_confirmation_resolved",
        "the recovered continuation records its resume state: {payload}"
    );
    let results = payload["toolResults"].as_array().expect("tool results");
    assert_eq!(
        results.len(),
        1,
        "the released tool executes exactly once: {payload}"
    );
    assert_eq!(results[0]["callId"], "call-restart");
    assert_eq!(results[0]["name"], "tools.search");
    assert_eq!(results[0]["status"], "SUCCEEDED");
    assert_eq!(
        payload["toolCalls"]
            .as_array()
            .expect("tool calls")
            .iter()
            .filter(|call| call["status"] == "SUCCEEDED")
            .count(),
        1,
        "only the released call completes: {payload}"
    );

    // The resolved approval must not resurface as a timeline approval group.
    // The Rust session projection only carries user/assistant messages, so the
    // guarantee is that no approval-group kind is ever emitted for this run.
    let events = session_store
        .list_events("session-restart-recovery")
        .expect("session events");
    assert!(
        !events.iter().any(|event| {
            event.content.contains("approvalGroup") || event.content.contains("approval_group")
        }),
        "a resolved approval is omitted from the timeline: {events:?}"
    );
    runtime.shutdown();
}
