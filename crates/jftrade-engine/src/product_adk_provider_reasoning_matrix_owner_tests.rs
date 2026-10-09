use super::*;

async fn matrix_product() -> (
    tempfile::TempDir, Arc<ProductionAdkPort>, std::net::TcpListener, crate::product::ProductHandle,
) {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (directory, port) = reconnect_port();
    let provider = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    provider.set_nonblocking(true).unwrap();
    let config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(), &port.settings_path,
    ).unwrap().with_adk_read_snapshot_port(port.clone()).with_adk_mutation_port(port.clone());
    let state = crate::product_runtime::ProductRuntimeState::product_only(&config);
    let prepared = crate::product::prepare_product_with_runtime_state(config, state, None).await.unwrap();
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    (directory, port, provider, handle)
}

async fn create_provider(
    address: std::net::SocketAddr, provider: &std::net::TcpListener, id: &str, config: Value,
) -> (reqwest::StatusCode, Value) {
    let response = reqwest::Client::builder().timeout(Duration::from_secs(3)).build().unwrap()
        .post(format!("http://{address}/api/v1/adk/providers"))
        .header("connection", "close")
        .json(&json!({"id":id,"displayName":"Reasoning Matrix", "enabled":true,
            "baseUrl":format!("http://{}/v1",provider.local_addr().unwrap()),
            "model":"matrix-model", "apiKey":"fixture-key", "reasoningConfig":config}))
        .send().await.unwrap();
    (response.status(), response.json::<Value>().await.unwrap())
}

fn valid_config() -> Value {
    json!({"requestField":"reasoning.level","mappings":[
        {"effort":"low","value":"LOW"}, {"effort":"high","value":"balanced"},
        {"effort":"max","value":"balanced"},
    ]})
}

// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27 TestProviderReasoningValidationAndCustomMapping
#[tokio::test]
async fn production_provider_reasoning_http_rejects_all_invalid_reference_configs_without_writes() {
    let (_directory, port, provider, handle) = matrix_product().await;
    let rows = port.store.list_providers().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let native = port.session_store.list_sessions().unwrap();
    let mappings = valid_config()["mappings"].clone();
    let configs = [
        json!({"requestField":"model.reasoning","mappings":mappings}),
        json!({"requestField":"reasoning[0]","mappings":mappings}),
        json!({"requestField":"reasoning..level","mappings":mappings}),
        json!({"requestField":"reasoning.level","mappings":[{"effort":"low","value":"x"},{"effort":"low","value":"y"}]}),
        json!({"requestField":"reasoning.level","mappings":[{"effort":"low","value":"  "}]}),
        json!({"requestField":"reasoning.level","mappings":[{"effort":"default","value":"x"}]}),
    ];
    let mut responses = Vec::new();
    for (index, config) in configs.into_iter().enumerate() {
        responses.push(create_provider(handle.startup_record().address, &provider,
            &format!("provider-invalid-{index}"), config).await);
        assert_eq!(port.store.list_providers().unwrap(), rows);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
        assert_eq!(port.session_store.list_sessions().unwrap(), native);
        assert!(!port.settings_path.parent().unwrap().join("secrets/adk-secrets.json").exists());
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(), io::ErrorKind::WouldBlock);
    for (status, response) in responses {
        assert_eq!(status, 400, "{response}");
        assert_eq!(response["error"]["code"], "BAD_REQUEST");
        assert!(!response["error"]["message"].as_str().unwrap().is_empty());
        assert!(response.get("data").is_none());
    }
}

// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27 TestProviderReasoningValidationAndCustomMapping
#[tokio::test]
async fn production_provider_reasoning_http_persists_shared_values_and_resolver_matches_reference_matrix() {
    let (_directory, port, provider, handle) = matrix_product().await;
    let config = valid_config();
    let (status, saved) = create_provider(handle.startup_record().address, &provider, "provider-matrix", config.clone()).await;
    assert_eq!(status, 200, "{saved}");
    assert_eq!(saved["data"]["reasoningConfig"], config);
    port.store.upsert_agent("agent-matrix", &json!({"id":"agent-matrix","status":"ENABLED",
        "providerId":"provider-matrix","reasoningEffort":"low"}).to_string()).unwrap();
    let runtime = ProductionAdkChatRuntime::new(port.store.clone(),port.session_store.clone(),
        &port.settings_path,Arc::new(RunCancellationRegistry::default()),port.tool_catalog.clone());
    let rows = port.store.list_providers().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let mut request = serde_json::Map::from_iter([("agentId".to_owned(),json!("agent-matrix"))]);
    for (effort, value) in [("low","LOW"),("high","balanced"),("max","balanced")] {
        request.insert("reasoningEffortOverride".to_owned(),json!(effort));
        let resolved = runtime.resolve_provider(&request).unwrap();
        assert_eq!(resolved.reasoning_effort.as_deref(), Some(effort));
        assert_eq!(resolved.reasoning, Some(("reasoning.level".to_owned(),value.to_owned())));
    }
    request.insert("reasoningEffortOverride".to_owned(),json!("medium"));
    match runtime.resolve_provider(&request) {
        Err(AdkChatPortError::Failed {status,code,message}) => {
            assert_eq!(status,400);assert_eq!(code,"ADK_CHAT_FAILED");
            assert_eq!(message,"provider reasoning unsupported: medium");
        }
        _ => panic!("unmapped medium must return unsupported"),
    }
    request.insert("reasoningEffortOverride".to_owned(),json!("extreme"));
    assert!(matches!(runtime.resolve_provider(&request),Err(AdkChatPortError::Failed {status:400,..})));
    runtime.shutdown_with_error().unwrap();
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_providers().unwrap(),rows);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert!(port.store.list_runs().unwrap().is_empty());
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}
