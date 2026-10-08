use super::*;
use crate::product::product_adk_chat_stream_port::{AdkChatInput, AdkChatRoute};

fn prepare_reasoning_snapshot() -> (
    tempfile::TempDir, Arc<ProductionAdkPort>, std::net::TcpListener,
    Arc<ProductionAdkChatRuntime>, ChatExecution, RunLeaseGuard, RunGateGuard, AdkChatInput,
) {
    let _=rustls::crypto::ring::default_provider().install_default();
    let (directory, mut port) = reconnect_port();
    let provider=std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    provider.set_nonblocking(true).unwrap();
    port.store.upsert_provider("provider-readiness", &json!({
        "baseUrl":format!("http://{}/v1",provider.local_addr().unwrap()),
        "model":"snapshot-model-v1","apiKey":"fixture-key","enabled":true,
        "reasoningConfig":{"requestField":"provider.reasoning.level",
            "mappings":[{"effort":"high","value":"DEEP"}]},
    }).to_string()).unwrap();
    port.store.upsert_agent("agent-private", &json!({
        "id":"agent-private","providerId":"provider-readiness",
        "status":"ENABLED","reasoningEffort":"high",
    }).to_string()).unwrap();
    let input = AdkChatInput {
        client_request_id:"11111111-1111-4111-8111-111111111120".to_owned(),
        body:br#"{"clientRequestId":"11111111-1111-4111-8111-111111111120","agentId":"agent-private","message":"hello"}"#.to_vec(),
    };
    let runtime=ProductionAdkChatRuntime::new(port.store.clone(),port.session_store.clone(),
        &port.settings_path,Arc::new(RunCancellationRegistry::default()),port.tool_catalog.clone());
    Arc::get_mut(&mut port).unwrap().chat_runtime=Some(runtime.clone());
    let PreparedChat::New(chat,lease,slot) = runtime.prepare_chat(AdkChatRoute::Chat,&input).unwrap() else {
        panic!("new production snapshot")
    };
    assert_eq!(chat.request.reasoning,Some(("provider.reasoning.level".to_owned(),"DEEP".to_owned())));
    let stored = port.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload:Value=serde_json::from_str(&stored.payload_json).unwrap();
    assert_eq!(payload["reasoningEffort"],"high");
    assert_eq!(payload["reasoningEffortField"],"provider.reasoning.level");
    assert_eq!(payload["reasoningEffortValue"],"DEEP");
    (directory,port,provider,runtime,chat,lease,slot,input)
}

fn assert_public_snapshot(body:&Value) {
    let encoded=body.to_string();
    assert!(!encoded.contains("reasoningEffortField"),"private field leaked: {body}");
    assert!(!encoded.contains("reasoningEffortValue"),"private value leaked: {body}");
}

#[tokio::test]
async fn production_terminal_cancel_projection_hides_private_reasoning_snapshot() {
    let (_directory,port,provider,runtime,chat,lease,slot,_input)=prepare_reasoning_snapshot();
    runtime.finish_chat(&chat,Ok(ModelResponse {
        text:"done".to_owned(),tool_calls:Vec::new(),usage_metadata:None,
    }),&lease).unwrap();
    drop((lease,slot));
    let stored=port.store.get_run(&chat.run_id).unwrap().unwrap();
    let audit=port.store.list_audit_events().unwrap();
    let config=crate::product::ProductConfig::test_cutover("127.0.0.1:0".parse().unwrap(),&port.settings_path)
        .unwrap().with_adk_read_snapshot_port(port.clone()).with_adk_mutation_port(port.clone());
    let state=crate::product_runtime::ProductRuntimeState::product_only(&config);
    let prepared=crate::product::prepare_product_with_runtime_state(config,state,None).await.unwrap();
    let handle=crate::product::expose_prepared_product(prepared).unwrap();
    let response=reqwest::Client::new().post(format!("http://{}/api/v1/adk/runs/{}/cancel",
        handle.startup_record().address,chat.run_id)).header("connection","close").send().await.unwrap();
    let status=response.status();let body=response.json::<Value>().await.unwrap();
    handle.shutdown().await.unwrap();port.shutdown_with_error().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
    assert_eq!(port.store.get_run(&chat.run_id).unwrap().unwrap(),stored);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert_eq!(status,200,"{body}");
    assert_eq!(body["data"]["status"],"COMPLETED");
    assert_eq!(body["data"]["reasoningEffort"],"high");
    assert_public_snapshot(&body);
}

// Parity: go:452dea11:internal/assistant/engine/persistence/store_run_test.go:31 TestRunReasoningSnapshotIsPrivateAndRestored
// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:552 TestRunStoresResolvedModelSnapshot
#[tokio::test]
async fn production_private_reasoning_snapshot_is_persisted_restored_and_hidden_in_http_reads() {
    let (_directory,port,provider,runtime,chat,lease,slot,_input)=prepare_reasoning_snapshot();
    let output=runtime.finish_chat(&chat,Ok(ModelResponse {
        text:"done".to_owned(),tool_calls:Vec::new(),usage_metadata:None,
    }),&lease).unwrap();
    let AdkChatPortOutput::Json(response)=output else {panic!("sync response")};
    assert_public_snapshot(&response);
    drop((lease,slot));
    let stored=port.store.get_run(&chat.run_id).unwrap().unwrap();
    let restored:Value=serde_json::from_str(&stored.payload_json).unwrap();
    assert_eq!(restored["reasoningEffortField"],"provider.reasoning.level");
    assert_eq!(restored["reasoningEffortValue"],"DEEP");
    port.store.upsert_provider("provider-readiness",&json!({
        "baseUrl":format!("http://{}/v1",provider.local_addr().unwrap()),
        "model":"snapshot-model-v2","displayName":"renamed","apiKey":"fixture-key","enabled":true,
        "reasoningConfig":{"requestField":"vendor.current","mappings":[{"effort":"low","value":"LOW_V2"}]},
    }).to_string()).unwrap();
    let audit=port.store.list_audit_events().unwrap();
    let native=port.session_store.list_sessions().unwrap();
    let prepared=prepared_router_with_chat(port.clone(),true).await;
    let handle=crate::product::expose_prepared_product(prepared).unwrap();
    let base=format!("http://{}",handle.startup_record().address);
    let mut results=Vec::new();
    for path in [format!("/api/v1/adk/runs/{}",chat.run_id),
        format!("/api/v1/adk/runs?sessionId={}",chat.session_id),
        format!("/api/v1/adk/sessions/{}",chat.session_id)] {
        let response=reqwest::Client::new().get(format!("{base}{path}")).send().await.unwrap();
        results.push((path,response.status(),response.json::<Value>().await.unwrap()));
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
    assert_eq!(port.store.get_run(&chat.run_id).unwrap().unwrap(),stored);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert_eq!(port.session_store.list_sessions().unwrap(),native);
    for (path,status,body) in results {
        assert_eq!(status,200,"{path}: {body}");
        assert_public_snapshot(&body);
        if path.starts_with("/api/v1/adk/runs?") {
            let runs=body["data"]["runs"].as_array().unwrap();
            assert_eq!(runs.len(),1);
            assert_eq!(runs[0]["model"],"snapshot-model-v1");
            assert_eq!(runs[0]["reasoningEffort"],"high");
        }
    }
}

#[tokio::test]
async fn production_sync_running_replay_hides_private_reasoning_without_changing_owner() {
    let (_directory,port,provider,_runtime,chat,lease,slot,input)=prepare_reasoning_snapshot();
    let stored=port.store.get_run(&chat.run_id).unwrap().unwrap();
    let audit=port.store.list_audit_events().unwrap();
    let native=port.session_store.list_sessions().unwrap();
    let prepared=prepared_router_with_chat(port.clone(),true).await;
    let handle=crate::product::expose_prepared_product(prepared).unwrap();
    let response=reqwest::Client::builder().timeout(Duration::from_secs(3)).build().unwrap()
        .post(format!("http://{}/api/v1/adk/chat",handle.startup_record().address))
        .header("content-type","application/json").header("connection","close")
        .body(input.body).send().await.unwrap();
    let status=response.status();let body=response.json::<Value>().await.unwrap();
    assert_eq!(port.store.get_run(&chat.run_id).unwrap().unwrap(),stored);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert_eq!(port.session_store.list_sessions().unwrap(),native);
    assert_eq!(port.store.get_run_lease(&chat.run_id).unwrap().unwrap().fencing_token,lease.token());
    port.shutdown_with_error().unwrap();
    drop((lease,slot));
    tokio::time::timeout(Duration::from_secs(3),handle.shutdown()).await.unwrap().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
    assert_eq!(status,200,"{body}");
    assert_eq!(body["data"]["run"]["id"],chat.run_id);
    assert_eq!(body["data"]["run"]["reasoningEffort"],"high");
    assert_public_snapshot(&body);
}
