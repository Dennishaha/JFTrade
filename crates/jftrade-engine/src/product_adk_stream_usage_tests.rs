use super::*;

fn usage_provider() -> (String, thread::JoinHandle<Value>) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let address = listener.local_addr().unwrap();
    let owner = thread::spawn(move || {
        let deadline = Instant::now() + Duration::from_secs(5);
        let mut socket = loop {
            match listener.accept() {
                Ok((socket, _)) => break socket,
                Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                    assert!(Instant::now() < deadline, "model request must arrive");
                    thread::sleep(Duration::from_millis(1));
                }
                Err(error) => panic!("accept model request: {error}"),
            }
        };
        socket.set_nonblocking(false).unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(5)))
            .unwrap();
        let request = read_http_json_body(&mut socket);
        let body = concat!(
            "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_stream\",\"model\":\"test-model\"}}\n\n",
            "data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n\n",
            "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\",\"model\":\"test-model\",\"usage\":{\"input_tokens\":9,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":3,\"output_tokens_details\":{\"reasoning_tokens\":1},\"total_tokens\":12}}}\n\n",
            "data: [DONE]\n\n",
        );
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        std::io::Write::write_all(&mut socket, response.as_bytes()).unwrap();
        request
    });
    (format!("http://{address}/v1/responses"), owner)
}

// Parity: go:452dea11:internal/assistant/engine/providers/responses_model_test.go:48 TestResponsesModelRetainsStreamingUsageMetadata
#[test]
fn responses_stream_final_model_result_retains_input_output_and_total_usage_metadata() {
    let (endpoint, provider) = usage_provider();
    let request = ModelRequest {
        endpoint: endpoint.parse().unwrap(),
        api_key: "secret".to_owned(),
        model: "test-model".to_owned(),
        instruction: None,
        message: "look up AAPL".to_owned(),
        durable_context: Vec::new(),
        tool_context: vec![
            json!({"type":"function_call", "call_id":"call-1", "name":"market.data", "arguments":"{\"symbol\":\"AAPL\"}"}),
            json!({"type":"function_call_output", "call_id":"call-1", "output":"ready"}),
        ],
        timeout: Duration::from_secs(5),
        tools: vec![
            json!({"type":"function", "name":"market.data", "parameters":{"type":"object"}}),
        ],
        reasoning: None,
    };
    let mut events = Vec::new();
    let response = execute_model_stream(
        request,
        |event| {
            events.push(event.clone());
            Ok(())
        },
        || false,
    )
    .unwrap();
    let captured = provider.join().unwrap();
    assert_eq!(captured["stream"], true);
    assert_eq!(response.text, "done");
    assert!(response.tool_calls.is_empty());
    assert_eq!(events.last().unwrap()["type"], "response.completed");
    let usage = response
        .usage_metadata
        .expect("final model result retains streaming usage");
    assert_eq!(usage["input_tokens"], 9);
    assert_eq!(usage["output_tokens"], 3);
    assert_eq!(usage["total_tokens"], 12);
    assert_eq!(usage["input_tokens_details"]["cached_tokens"], 0);
    assert_eq!(usage["output_tokens_details"]["reasoning_tokens"], 1);
}

#[tokio::test]
async fn production_stream_final_run_retains_provider_input_and_output_usage() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (endpoint, provider) = usage_provider();
    let (directory, store, sessions) = initialized_stores();
    let settings = directory.path().join("settings.json");
    fs::write(&settings, "{}").unwrap();
    fs::create_dir(directory.path().join("secrets")).unwrap();
    fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"usage-provider":"secret"}"#,
    )
    .unwrap();
    store
        .upsert_provider(
            "usage-provider",
            &json!({"id":"usage-provider", "baseUrl": endpoint,
        "model":"test-model", "enabled":true})
            .to_string(),
        )
        .unwrap();
    store
        .upsert_agent(
            "usage-agent",
            &json!({"id":"usage-agent", "name":"usage-agent",
        "providerId":"usage-provider", "status":"ENABLED", "permissionMode":"all"})
            .to_string(),
        )
        .unwrap();
    let runtime = ProductionAdkChatRuntime::new(
        store.clone(),
        sessions,
        &settings,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let config =
        crate::product::ProductConfig::test_cutover("127.0.0.1:0".parse().unwrap(), &settings)
            .unwrap()
            .with_adk_chat_stream_port(runtime.clone());
    let handle = crate::product::start_product(config).await.unwrap();
    let response = reqwest::Client::new()
        .post(format!(
            "http://{}/api/v1/adk/chat/stream",
            handle.startup_record().address
        ))
        .timeout(Duration::from_secs(5))
        .json(
            &json!({"clientRequestId":"55555555-5555-4555-8555-555555555555",
            "agentId":"usage-agent", "message":"hello"}),
        )
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(response.headers()["content-type"], "text/event-stream");
    let body = response.text().await.unwrap();
    let request = provider.join().unwrap();
    assert_eq!(request["stream"], true);
    assert_eq!(request["model"], "test-model");
    let final_event = body
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .filter_map(|line| serde_json::from_str::<Value>(line).ok())
        .find(|event| event["type"] == "final")
        .expect("terminal final frame");
    handle.shutdown().await.unwrap();
    runtime.shutdown();
    assert_eq!(final_event["response"]["reply"], "done");
    assert_eq!(
        final_event["response"]["run"]["usage"]["tokensIn"], 9,
        "{final_event}"
    );
    assert_eq!(
        final_event["response"]["run"]["usage"]["tokensOut"], 3,
        "{final_event}"
    );
    let run_id = final_event["response"]["run"]["id"].as_str().unwrap();
    let stored = store.get_run(run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&stored.payload_json).unwrap();
    assert_eq!(payload["usage"]["tokensIn"], 9);
    assert_eq!(payload["usage"]["tokensOut"], 3);
}
