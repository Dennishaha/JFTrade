use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;

const REQUEST: &str = "abcdefab-cdef-4abc-8def-abcdefabcdef";

fn equivalent_body(agent: &str) -> String {
    format!(
        r#"{{"message":"  hello  ","agentId":" {agent} ","clientRequestId":" URN:UUID:ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF ","workModeOverride":" CHAT ","permissionModeOverride":" APPROVAL ","reasoningEffortOverride":null,"objective":"hello","runOptions":{{"loopMaxIterations":5}},"ignored":"not part of identity"}}"#
    )
}

// Parity: go:452dea11:internal/api/assistant/routes_test.go:301 TestChatRequestIdempotencyContracts
#[tokio::test]
async fn production_retained_error_reuses_equivalent_chat_payload_after_configuration_change() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let url = format!(
        "http://{}/api/v1/adk/chat/stream",
        handle.startup_record().address
    );
    let client = reqwest::Client::new();
    let first = client
        .post(&url)
        .json(&json!({"clientRequestId":REQUEST,"agentId":"missing-agent","message":"hello"}))
        .send()
        .await
        .unwrap();
    assert_eq!(first.status(), 200);
    let id = first.headers()["x-adk-stream-id"]
        .to_str()
        .unwrap()
        .to_owned();
    let initial = first.text().await.unwrap();
    retained_readiness::set_provider(&port, &provider, true);
    assert!(port.runtime_ready());
    let replay = client
        .post(&url)
        .header("content-type", "application/json")
        .body(equivalent_body("missing-agent"))
        .send()
        .await
        .unwrap();
    assert_eq!(
        replay.status(),
        200,
        "semantic identity survives readiness change"
    );
    assert_eq!(replay.headers()["x-adk-stream-id"], id);
    let replay = replay.text().await.unwrap();
    let mut expected: Value = serde_json::from_str(
        initial
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    expected["replay"] = json!(true);
    let actual: Value = serde_json::from_str(
        replay
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(actual, expected);
    for changed in [
        json!({"reasoningEffortOverride":"high"}),
        json!({"objective":"different"}),
        json!({"runOptions":{"loopMaxIterations":6}}),
    ] {
        let mut body =
            json!({"clientRequestId":REQUEST,"agentId":"missing-agent","message":"hello"});
        body.as_object_mut()
            .unwrap()
            .extend(changed.as_object().unwrap().clone());
        let response = client.post(&url).json(&body).send().await.unwrap();
        assert_eq!(response.status(), 409);
        assert_eq!(
            response.json::<Value>().await.unwrap()["error"]["code"],
            "ADK_CHAT_IDEMPOTENCY_CONFLICT"
        );
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert!(port.store.list_runs().unwrap().is_empty());
    assert!(port.store.list_audit_events().unwrap().is_empty());
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

// Parity: go:452dea11:internal/assistant/engine/chat_request_idempotency_test.go:17 TestChatRequestIdentityValidationAndFingerprintConflict
#[tokio::test]
async fn production_durable_chat_reuses_equivalent_payload_without_second_provider_call() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    port.store
        .upsert_agent(
            "agent-canonical",
            &json!({"id":"agent-canonical","name":"Canonical Agent",
        "providerId":"provider-readiness","model":"fixture-model"})
            .to_string(),
        )
        .unwrap();
    let calls = Arc::new(AtomicUsize::new(0));
    let observed = calls.clone();
    let mock=axum::Router::new().route("/v1/responses",axum::routing::post(move || {
        let observed=observed.clone();async move {
            observed.fetch_add(1,Ordering::SeqCst);
            axum::Json(json!({"output":[{"type":"message","content":[{"type":"output_text","text":"single answer"}]}]}))
        }
    }));
    let socket = TcpListener::from_std(provider).unwrap();
    let (stop, stopped) = oneshot::channel();
    let server = tokio::spawn(
        axum::serve(socket, mock)
            .with_graceful_shutdown(async {
                let _ = stopped.await;
            })
            .into_future(),
    );
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let url = format!("http://{}/api/v1/adk/chat", handle.startup_record().address);
    let client = reqwest::Client::new();
    let first = client
        .post(&url)
        .json(&json!({"clientRequestId":REQUEST,"agentId":"agent-canonical","message":"hello"}))
        .send()
        .await
        .unwrap();
    assert_eq!(first.status(), 200);
    let first: Value = first.json().await.unwrap();
    assert_eq!(first["data"]["run"]["status"], "COMPLETED");
    assert_eq!(first["data"]["reply"], "single answer");
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let run = &runs[0];
    let canonical = crate::product::product_adk_chat_identity::ChatRequestIdentity::decode(
        equivalent_body("agent-canonical").as_bytes(),
    ).unwrap();
    assert_eq!(run.request_fingerprint, canonical.canonical);
    let events = port.session_store.list_events(&run.session_id).unwrap();
    let replay = client
        .post(&url)
        .header("content-type", "application/json")
        .body(equivalent_body("agent-canonical"))
        .send()
        .await
        .unwrap();
    assert_eq!(replay.status(), 200, "canonical durable identity");
    assert_eq!(replay.json::<Value>().await.unwrap()["data"], first["data"]);
    let changed=client.post(&url).json(&json!({"clientRequestId":REQUEST,"agentId":"agent-canonical","message":"hello","reasoningEffortOverride":"high"})).send().await.unwrap();
    assert_eq!(changed.status(), 409);
    assert_eq!(
        changed.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_CHAT_IDEMPOTENCY_CONFLICT"
    );
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    stop.send(()).unwrap();
    server.await.unwrap().unwrap();
    assert_eq!(calls.load(Ordering::SeqCst), 1);
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        port.session_store.list_events(&run.session_id).unwrap(),
        events
    );
}

#[tokio::test]
async fn production_legacy_raw_chat_fingerprint_replays_without_rewriting_the_durable_owner() {
    use sha2::Digest;
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    let original = json!({"clientRequestId":REQUEST,"agentId":"agent-legacy","message":"hello"}).to_string();
    let legacy = sha2::Sha256::digest(original.as_bytes()).iter().map(|byte| format!("{byte:02x}")).collect::<String>();
    let response = json!({"reply":"retained legacy answer","run":{"id":"run-legacy","status":"COMPLETED"}});
    port.store.create_run(CreateAdkRunParams {
        id:"run-legacy",session_id:"session-legacy",agent_id:"agent-legacy",status:"COMPLETED",
        client_request_id:REQUEST,request_fingerprint:&legacy,
        payload_json:&json!({"route":"chat","response":response}).to_string(),
    }).unwrap();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}/api/v1/adk",handle.startup_record().address);
    let client = reqwest::Client::new();
    let replay = client.post(format!("{base}/chat")).header("content-type","application/json").body(original.clone()).send().await.unwrap();
    assert_eq!(replay.status(),200);
    assert_eq!(replay.json::<Value>().await.unwrap()["data"],response);
    for (path,body) in [("chat",equivalent_body("agent-legacy")),("chat/stream",original),
        ("chat",json!({"clientRequestId":REQUEST,"agentId":"agent-legacy","message":"changed"}).to_string())] {
        let conflict = client.post(format!("{base}/{path}")).header("content-type","application/json").body(body).send().await.unwrap();
        assert_eq!(conflict.status(),409);
        assert_eq!(conflict.json::<Value>().await.unwrap()["error"]["code"],"ADK_CHAT_IDEMPOTENCY_CONFLICT");
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_runs().unwrap(),runs);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert!(port.session_store.list_events("session-legacy").unwrap().is_empty());
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}
