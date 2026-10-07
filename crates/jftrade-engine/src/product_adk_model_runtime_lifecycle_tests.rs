// Behavior tests for the ADK model runtime lifecycle.
//
// Split out of `product_adk_model_runtime_lifecycle.rs` so the production
// fragment stays under the workspace 800-line architecture limit. The
// module is pulled in with `#[path]` from the same parent module, so the
// visibility and imports are unchanged.

use super::*;

#[path = "product_adk_workflow_shutdown_tests.rs"]
mod workflow_shutdown;

#[path = "product_adk_stream_usage_tests.rs"]
mod stream_usage;

#[path = "product_adk_reconnect_live_tests.rs"]
mod reconnect_live;

fn read_http_json_body(stream: &mut std::net::TcpStream) -> Value {
    let mut request = Vec::new();
    let mut chunk = [0_u8; 4096];
    let mut expected = None;
    loop {
        let count = std::io::Read::read(stream, &mut chunk).expect("read probe request");
        if count == 0 {
            break;
        }
        request.extend_from_slice(&chunk[..count]);
        if expected.is_none()
            && let Some(headers_end) = request.windows(4).position(|window| window == b"\r\n\r\n")
        {
            let headers_end = headers_end + 4;
            let headers = String::from_utf8_lossy(&request[..headers_end]).to_ascii_lowercase();
            let content_length = headers
                .lines()
                .find_map(|line| line.strip_prefix("content-length:"))
                .and_then(|value| value.trim().parse::<usize>().ok())
                .expect("content length");
            expected = Some(headers_end + content_length);
        }
        if expected.is_some_and(|length| request.len() >= length) {
            break;
        }
    }
    let headers_end = request
        .windows(4)
        .position(|window| window == b"\r\n\r\n")
        .map(|index| index + 4)
        .expect("probe request headers");
    serde_json::from_slice(&request[headers_end..]).expect("decode probe request")
}
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

// Parity: go:452dea11:internal/assistant/engine/providers/probe_test.go:105 TestProviderProbeTimeoutCapsConfiguredRequestTimeout
#[test]
fn provider_probe_timeout_caps_configured_request_timeout() {
    assert_eq!(provider_probe_timeout_ms(0), 30_000);
    assert_eq!(provider_probe_timeout_ms(15_000), 15_000);
    assert_eq!(provider_probe_timeout_ms(600_000), 30_000);
}

// Parity: go:452dea11:internal/assistant/engine/providers/probe_test.go:13 TestProbeProviderQuickAndFullRequestCounts
#[test]
fn provider_probe_quick_and_full_modes_send_expected_responses_requests() {
    let (directory, store, session_store) = initialized_stores();
    let secrets_dir = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets_dir).expect("create secrets directory");
    std::fs::write(
        secrets_dir.join("adk-secrets.json"),
        br#"{"probe-provider":"probe-secret"}"#,
    )
    .expect("write provider key");

    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind probe provider");
    let address = listener.local_addr().expect("probe provider address");
    let server = thread::spawn(move || {
        let mut requests = Vec::new();
        for (index, incoming) in listener.incoming().take(13).enumerate() {
            let mut stream = incoming.expect("accept provider probe");
            let body = read_http_json_body(&mut stream);
            let failed_reasoning = body.pointer("/reasoning/effort") == Some(&json!("DEEP"));
            let failed_tools = body
                .get("tools")
                .and_then(Value::as_array)
                .is_some_and(|tools| !tools.is_empty());
            let (status, response) = if failed_tools {
                ("502 Bad Gateway", r#"{"error":{"message":"tool calling unavailable"}}"#)
            } else if failed_reasoning {
                ("400 Bad Request", r#"{"error":{"message":"deep unavailable"}}"#)
            } else {
                ("200 OK", r#"{"output_text":"health check ok"}"#)
            };
            let header = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                response.len()
            );
            std::io::Write::write_all(&mut stream, header.as_bytes()).expect("write headers");
            std::io::Write::write_all(&mut stream, response.as_bytes()).expect("write response");
            requests.push((index, body));
        }
        requests
    });

    let provider = |mappings: Value| {
        store
            .upsert_provider(
                "probe-provider",
                &json!({
                    "displayName": "Probe Provider",
                    "baseUrl": format!("http://{}:{}/v1", address.ip(), address.port()),
                    "model": "probe-model",
                    "enabled": true,
                    "reasoningConfig": {
                        "requestField": "reasoning.effort",
                        "mappings": mappings,
                    },
                })
                .to_string(),
            )
            .expect("persist probe provider");
    };
    provider(json!([
        {"effort":"high","value":"DEEP"},
        {"effort":"low","value":"FAST"},
        {"effort":"medium","value":"BALANCED"},
    ]));
    store
        .upsert_agent(
            "probe-agent",
            &json!({"providerId":"probe-provider","model":"probe-model","status":"ENABLED"})
                .to_string(),
        )
        .expect("persist probe agent");

    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    let run_probe = |mode: &str, request_id: &str| {
        let body = json!({
            "clientRequestId": request_id,
            "providerId": "probe-provider",
            "agentId": "probe-agent",
            "providerProbe": true,
            "providerTestMode": mode,
        });
        runtime
            .dispatch_inner(
                AdkChatRoute::Chat,
                &AdkChatInput {
                    body: body.to_string().into_bytes(),
                    client_request_id: request_id.to_owned(),
                },
            )
            .expect("provider probe succeeds")
    };

    let full = run_probe("full", "provider-probe-full");
    let full = match full {
        AdkChatPortOutput::Json(value) => value,
        other => panic!("provider probe must return JSON: {other:?}"),
    };
    assert_eq!(full["reasoning"]["mode"], "full");
    assert_eq!(full["reasoning"]["results"].as_array().unwrap().len(), 3);
    assert_eq!(full["reasoning"]["results"][0]["effort"], "low");
    assert_eq!(full["reasoning"]["results"][1]["effort"], "medium");
    assert_eq!(full["reasoning"]["results"][2]["effort"], "high");
    assert_eq!(full["reasoning"]["results"][2]["ok"], false);
    assert_eq!(full["capabilities"]["reasoning"], false);
    assert_eq!(full["capabilities"]["tools"], false);
    assert_eq!(full["ok"], false);

    let quick = run_probe("quick", "provider-probe-quick");
    let quick = match quick {
        AdkChatPortOutput::Json(value) => value,
        other => panic!("provider probe must return JSON: {other:?}"),
    };
    assert_eq!(quick["reasoning"]["mode"], "quick");
    assert_eq!(quick["reasoning"]["results"].as_array().unwrap().len(), 1);
    assert_eq!(quick["reasoning"]["results"][0]["effort"], "medium");
    assert_eq!(quick["capabilities"]["reasoning"], true);

    provider(json!([
        {"effort":"high","value":"DEEP"},
        {"effort":"low","value":"FAST"},
    ]));
    let canonical = run_probe("quick", "provider-probe-canonical");
    let canonical = match canonical {
        AdkChatPortOutput::Json(value) => value,
        other => panic!("provider probe must return JSON: {other:?}"),
    };
    assert_eq!(canonical["reasoning"]["results"].as_array().unwrap().len(), 1);
    assert_eq!(canonical["reasoning"]["results"][0]["effort"], "low");
    assert_eq!(canonical["reasoning"]["results"][0]["ok"], true);
    assert_eq!(canonical["capabilities"]["reasoning"], true);

    provider(json!([]));
    let empty = run_probe("quick", "provider-probe-empty");
    let empty = match empty {
        AdkChatPortOutput::Json(value) => value,
        other => panic!("provider probe must return JSON: {other:?}"),
    };
    assert_eq!(empty["reasoning"]["results"], json!([]));
    assert_eq!(empty["capabilities"]["reasoning"], false);
    assert_eq!(store.list_runs().expect("list runs"), Vec::new());

    let requests = server.join().expect("join provider test server");
    assert_eq!(requests.len(), 13);
    for (_, body) in &requests {
        assert_eq!(body["model"], "probe-model");
        assert_eq!(body["stream"], false);
        assert_eq!(body["input"][0]["content"], "JFTrade ADK provider connectivity test.");
    }
    assert!(requests[0].1.get("reasoning").is_none());
    assert!(requests[1].1["tools"].is_array());
    assert_eq!(requests[2].1.pointer("/reasoning/effort"), Some(&json!("FAST")));
    assert_eq!(requests[3].1.pointer("/reasoning/effort"), Some(&json!("BALANCED")));
    assert_eq!(requests[4].1.pointer("/reasoning/effort"), Some(&json!("DEEP")));
    assert_eq!(requests[7].1.pointer("/reasoning/effort"), Some(&json!("BALANCED")));
    assert_eq!(requests[10].1.pointer("/reasoning/effort"), Some(&json!("FAST")));
    assert!(requests[9].1.get("reasoning").is_none());
    assert!(requests[11].1.get("reasoning").is_none());
    assert!(requests[12].1.get("reasoning").is_none());
    runtime.shutdown();
}

// Parity: go:452dea11:internal/assistant/engine/providers/probe_test.go:81 TestProbeProviderWithoutMappingsSendsNoReasoningField
#[test]
fn provider_probe_without_mappings_sends_no_reasoning_field() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind probe provider");
    let address = listener.local_addr().expect("probe provider address");
    let server = thread::spawn(move || {
        let mut requests = Vec::new();
        for incoming in listener.incoming().take(2) {
            let mut stream = incoming.expect("accept provider probe");
            let body = read_http_json_body(&mut stream);
            assert!(body.get("reasoning").is_none(), "unexpected reasoning: {body}");
            requests.push(body);
            let response = r#"{"output_text":"health check ok"}"#;
            let header = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                response.len()
            );
            std::io::Write::write_all(&mut stream, header.as_bytes()).expect("write headers");
            std::io::Write::write_all(&mut stream, response.as_bytes()).expect("write response");
        }
        requests
    });
    let provider = ResolvedProvider {
        id: "probe-provider".to_owned(),
        name: "Probe Provider".to_owned(),
        agent_id: "probe-agent".to_owned(),
        agent_payload: json!({}),
        endpoint: Url::parse(&format!("http://{}:{}/v1", address.ip(), address.port()))
            .expect("probe endpoint"),
        api_key: "probe-secret".to_owned(),
        model: "probe-model".to_owned(),
        agent_model: None,
        permission_mode: "all".to_owned(),
        instruction: None,
        timeout: Duration::from_secs(1),
        reasoning: None,
        reasoning_effort: None,
    };
    provider_probe_model(&provider, Vec::new(), None).expect("baseline probe");
    provider_probe_model(&provider, provider_probe_tools(), None).expect("tool probe");
    let requests = server.join().expect("join provider test server");
    assert_eq!(requests.len(), 2);
    assert!(requests[0].get("reasoning").is_none());
    assert!(requests[1].get("reasoning").is_none());
}

// Parity: go:452dea11:internal/assistant/engine/chat_request_idempotency_test.go:48 TestConcurrentResponsesRequestReusesOneRunAndNativeAssistantEvent
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
                r#"{"contextRevisionId":"revision-1","endEventIndex":2,"summary":"handoff summary"}"#,
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
        reasoning: None,
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

/// Go's `startRun` freezes `Runtime.runtimeLimits().RunTimeout` (wired from the
/// persisted `assistantRuntime.runTimeoutMs` settings) onto `run.MaxDurationMs`,
/// so an operator edit applies to the next run without a restart while a
/// missing settings document keeps `assistantmodel.DefaultRunTimeout`.
///
/// Reference: go:452dea11:internal/assistant/engine/store_test.go
/// `TestStartRunUsesConfiguredRuntimeTimeout`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:881
/// `TestStartRunUsesConfiguredRuntimeTimeout`.
#[test]
fn run_start_freezes_the_configured_run_timeout_from_settings() {
    let (directory, store, session_store) = initialized_stores();
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"provider-timeout-fixture":"fixture-key"}"#,
    )
    .expect("write provider credential");
    let settings_path = directory.path().join("settings.json");
    // 11 minutes: inside the accepted window, distinct from the 30-minute
    // default so a hardcoded constant cannot satisfy the assertion.
    std::fs::write(&settings_path, r#"{"adk":{"runTimeoutMs":660000}}"#)
        .expect("write assistant runtime settings");

    store
        .upsert_provider(
            "provider-timeout-fixture",
            &json!({
                "displayName": "Timeout fixture",
                "baseUrl": "http://127.0.0.1:9/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-timeout-fixture",
            &json!({
                "id": "agent-timeout-fixture",
                "name": "Timeout fixture",
                "providerId": "provider-timeout-fixture",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let chat = |client_request_id: &str| AdkChatInput {
        body: json!({
            "agentId": "agent-timeout-fixture",
            "message": "hello",
        })
        .to_string()
        .into_bytes(),
        client_request_id: client_request_id.to_owned(),
    };

    // The provider endpoint is a closed loopback port, so the run is created
    // (and frozen) before the model call fails.
    let _ = runtime.dispatch(AdkChatRoute::Chat, &chat("timeout-configured"));
    let configured = store
        .get_run("run-timeout-configured")
        .expect("read configured run")
        .expect("configured run row");
    let payload: Value = serde_json::from_str(&configured.payload_json).expect("run payload");
    assert_eq!(
        payload["maxDurationMs"], 660_000,
        "the run freezes the configured timeout: {payload}"
    );

    // An edit applies to the next run, and a missing settings document falls
    // back to Go's 30-minute default.
    std::fs::write(&settings_path, "{}").expect("reset assistant runtime settings");
    let _ = runtime.dispatch(AdkChatRoute::Chat, &chat("timeout-default"));
    let defaulted = store
        .get_run("run-timeout-default")
        .expect("read defaulted run")
        .expect("defaulted run row");
    let payload: Value = serde_json::from_str(&defaulted.payload_json).expect("run payload");
    assert_eq!(
        payload["maxDurationMs"], 1_800_000,
        "a settings document without an ADK section keeps the default: {payload}"
    );
    runtime.shutdown();
}

/// Go's `googleADKExecution.Run` returns `context.DeadlineExceeded` promptly
/// even when the blocking model callback never returns, so a stalled runner can
/// never pin a run.  Rust bounds the same call with the frozen request timeout
/// plus the cancellation poll and reports `504 MODEL_CALL_TIMEOUT`.
///
/// Reference: go:452dea11:internal/assistant/engine/store_test.go
/// `TestGoogleADKExecutionRunHonorsContextDeadline`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:862
/// `TestGoogleADKExecutionRunHonorsContextDeadline`.
#[test]
fn a_hanging_provider_is_bounded_by_the_request_timeout() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind hanging provider");
    let endpoint = format!(
        "http://127.0.0.1:{}/v1/responses",
        listener.local_addr().expect("provider address").port()
    );
    // The server accepts the request and never answers.  It returns as soon as
    // the client gives up and closes the socket, so the test joins promptly.
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept model request");
        let mut buffer = [0_u8; 64];
        let _ = std::io::Read::read(&mut stream, &mut buffer);
        // Hold the connection open without answering until the client gives up
        // and closes it, so the bounded request (not the server) ends the call.
        while let Ok(read) = std::io::Read::read(&mut stream, &mut buffer) {
            if read == 0 {
                break;
            }
        }
    });

    let started = std::time::Instant::now();
    let error = execute_model(
        ModelRequest {
            endpoint: reqwest::Url::parse(&endpoint).expect("endpoint url"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "查看系统状态".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_millis(150),
            tools: Vec::new(),
            reasoning: None,
        },
        Arc::new(std::sync::atomic::AtomicBool::new(false)),
    )
    .expect_err("a stalled provider must not pin the run");
    match error {
        AdkChatPortError::Failed {
            status,
            ref code,
            ref message,
        } => {
            assert_eq!(status, 504);
            assert_eq!(code, "MODEL_CALL_TIMEOUT");
            assert!(
                message.contains("timed out"),
                "unexpected timeout message: {message}"
            );
        }
        other => panic!("expected 504 MODEL_CALL_TIMEOUT, got {other:?}"),
    }
    assert!(
        started.elapsed() < Duration::from_secs(5),
        "the deadline guard must return promptly, took {:?}",
        started.elapsed()
    );
    server.join().expect("join hanging provider");
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

/// Parity: go:452dea11:internal/assistant/engine/providers/reasoning_effort_transport_test.go:64
/// Parity: go:452dea11:internal/assistant/engine/providers/reasoning_effort_transport_test.go:14
/// TestResponsesCustomReasoningMappingInjectsNestedField.
///
/// Provider mappings are written into the nested Responses request path before
/// transport. The production adapter previously ignored the resolved mapping,
/// so the loopback endpoint saw no `provider.reasoning.level` field.
#[test]
fn responses_request_injects_provider_reasoning_mapping() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind reasoning provider");
    let address = listener.local_addr().expect("reasoning provider address");
    let (captured_sender, captured_receiver) = std::sync::mpsc::channel();
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept reasoning request");
        let mut request = Vec::new();
        let mut chunk = [0_u8; 4096];
        let mut expected = None;
        loop {
            let count = std::io::Read::read(&mut stream, &mut chunk).expect("read reasoning request");
            if count == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..count]);
            if expected.is_none()
                && let Some(headers_end) = request.windows(4).position(|window| window == b"\r\n\r\n")
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
            if expected.is_some_and(|length| request.len() >= length) {
                break;
            }
        }
        let headers_end = request
            .windows(4)
            .position(|window| window == b"\r\n\r\n")
            .map(|index| index + 4)
            .expect("reasoning request headers");
        let body: Value = serde_json::from_slice(&request[headers_end..]).expect("reasoning body");
        captured_sender.send(body).expect("send captured reasoning body");
        let response = b"{\"output_text\":\"ok\"}";
        let header = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
            response.len()
        );
        std::io::Write::write_all(&mut stream, header.as_bytes()).expect("write reasoning headers");
        std::io::Write::write_all(&mut stream, response).expect("write reasoning response");
    });

    let result = execute_model(
        ModelRequest {
            endpoint: Url::parse(&format!("http://{}:{}/v1/responses", address.ip(), address.port()))
                .expect("reasoning endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "hello".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(2),
            tools: Vec::new(),
            reasoning: Some((
                "provider.reasoning.level".to_owned(),
                "BALANCED".to_owned(),
            )),
        },
        Arc::new(AtomicBool::new(false)),
    )
    .expect("reasoning request succeeds");
    assert_eq!(result.text, "ok");
    let body = captured_receiver.recv().expect("captured request body");
    assert_eq!(body.pointer("/provider/reasoning/level"), Some(&json!("BALANCED")));
    server.join().expect("join reasoning provider");
}

/// Parity: go:452dea11:internal/assistant/engine/providers/reasoning_effort_transport_test.go:14
/// TestResponsesReasoningEffortRequestField.
#[test]
fn responses_request_reasoning_effort_matrix_matches_go_contract() {
    for (label, input, expected) in [
        ("model default", None, None),
        ("low", Some("low"), Some("low")),
        ("medium", Some("medium"), Some("medium")),
        ("high", Some("high"), Some("high")),
        ("xhigh", Some("xhigh"), Some("xhigh")),
        ("max", Some("max"), Some("max")),
    ] {
        let reasoning = input.map(|effort| ("reasoning.effort".to_owned(), effort.to_owned()));
        let body = capture_responses_request(reasoning);
        let actual = body.pointer("/reasoning/effort").and_then(Value::as_str);
        assert_eq!(actual, expected, "reasoning matrix case {label}");
        assert_eq!(body.get("reasoning").is_some(), expected.is_some(), "reasoning presence case {label}");
    }
}

fn capture_responses_request(reasoning: Option<(String, String)>) -> Value {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind reasoning matrix provider");
    let address = listener.local_addr().expect("reasoning matrix provider address");
    let (captured_sender, captured_receiver) = std::sync::mpsc::channel();
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept reasoning matrix request");
        let mut request = Vec::new();
        let mut chunk = [0_u8; 4096];
        let mut expected = None;
        loop {
            let count = std::io::Read::read(&mut stream, &mut chunk).expect("read reasoning matrix request");
            if count == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..count]);
            if expected.is_none()
                && let Some(headers_end) = request.windows(4).position(|window| window == b"\r\n\r\n")
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
            if expected.is_some_and(|length| request.len() >= length) {
                break;
            }
        }
        let headers_end = request
            .windows(4)
            .position(|window| window == b"\r\n\r\n")
            .map(|index| index + 4)
            .expect("reasoning matrix request headers");
        let body: Value = serde_json::from_slice(&request[headers_end..]).expect("reasoning matrix body");
        captured_sender.send(body).expect("send reasoning matrix body");
        let response = b"{\"output_text\":\"ok\"}";
        let header = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
            response.len()
        );
        std::io::Write::write_all(&mut stream, header.as_bytes()).expect("write reasoning matrix headers");
        std::io::Write::write_all(&mut stream, response).expect("write reasoning matrix response");
    });

    execute_model(
        ModelRequest {
            endpoint: Url::parse(&format!("http://{}:{}/v1/responses", address.ip(), address.port()))
                .expect("reasoning matrix endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "hello".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(2),
            tools: Vec::new(),
            reasoning,
        },
        Arc::new(AtomicBool::new(false)),
    )
    .expect("reasoning matrix request succeeds");
    let body = captured_receiver.recv().expect("captured reasoning matrix body");
    server.join().expect("join reasoning matrix provider");
    body
}

/// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:8
/// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27
/// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:65
/// TestOptionalReasoningEffortRejectsDefault.
#[test]
// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:8 TestProviderReasoningPresetsAndExplicitEmptyMappings
// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27 TestProviderReasoningValidationAndCustomMapping
fn provider_reasoning_resolution_matches_mapping_and_unsupported_effort_contract() {
    let provider = json!({
        "reasoningConfig": {
            "requestField": "provider.reasoning.level",
            "mappings": [
                {"effort": " high ", "value": " BALANCED "},
                {"effort": "low", "value": "FAST"}
            ]
        }
    });
    let (effort, resolved) =
        resolve_provider_reasoning(&provider, Some(" HIGH ")).expect("custom mapping");
    assert_eq!(effort.as_deref(), Some("high"));
    assert_eq!(
        resolved,
        Some((
            "provider.reasoning.level".to_owned(),
            "BALANCED".to_owned()
        ))
    );

    let empty = json!({"reasoningConfig": {"requestField": "reasoning.effort", "mappings": []}});
    let error = resolve_provider_reasoning(&empty, Some("high")).expect_err("unsupported effort");
    assert!(format_adk_error(&error).contains("unsupported"));
    assert_eq!(resolve_provider_reasoning(&provider, None).expect("blank effort"), (None, None));
    assert!(resolve_provider_reasoning(&provider, Some("default")).is_err());
}

#[test]
// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27 TestProviderReasoningValidationAndCustomMapping
fn resolve_provider_freezes_agent_reasoning_mapping_and_request_override() {
    let (directory, store, session_store) = initialized_stores();
    let secrets = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets).expect("create secrets directory");
    std::fs::write(
        secrets.join("adk-secrets.json"),
        r#"{"provider-reasoning-fixture":"sk-fixture"}"#,
    )
    .expect("write provider secret");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    store
        .upsert_provider(
            "provider-reasoning-fixture",
            &json!({
                "displayName": "Reasoning fixture",
                "baseUrl": "https://reasoning.example/v1",
                "model": "fixture-model",
                "enabled": true,
                "reasoningConfig": {
                    "requestField": "provider.reasoning.level",
                    "mappings": [
                        {"effort": "low", "value": "FAST"},
                        {"effort": "high", "value": "BALANCED"}
                    ]
                }
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-reasoning-fixture",
            &json!({
                "id": "agent-reasoning-fixture",
                "providerId": "provider-reasoning-fixture",
                "reasoningEffort": "high",
                "status": "ENABLED"
            })
            .to_string(),
        )
        .expect("persist agent");
    let runtime = ProductionAdkChatRuntime::new(
        store,
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let request = serde_json::Map::from_iter([(
        "agentId".to_owned(),
        Value::String("agent-reasoning-fixture".to_owned()),
    )]);
    let resolved = runtime.resolve_provider(&request).expect("resolve agent mapping");
    assert_eq!(resolved.reasoning_effort.as_deref(), Some("high"));
    assert_eq!(
        resolved.reasoning,
        Some((
            "provider.reasoning.level".to_owned(),
            "BALANCED".to_owned()
        ))
    );

    let override_request = serde_json::Map::from_iter([
        (
            "agentId".to_owned(),
            Value::String("agent-reasoning-fixture".to_owned()),
        ),
        (
            "reasoningEffortOverride".to_owned(),
            Value::String("low".to_owned()),
        ),
    ]);
    let overridden = runtime
        .resolve_provider(&override_request)
        .expect("resolve request override");
    assert_eq!(overridden.reasoning_effort.as_deref(), Some("low"));
    assert_eq!(
        overridden.reasoning,
        Some(("provider.reasoning.level".to_owned(), "FAST".to_owned()))
    );
    runtime.shutdown();
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
// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:565 TestPendingApprovalResumesThroughGoogleADKAfterRuntimeRestart
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
