use super::*;

#[path = "product_adk_skill_registry_owner_tests.rs"]
mod registry;

async fn skill_router(
    port: Arc<ProductionAdkPort>,
    include_chat: bool,
) -> crate::product::ProductHandle {
    let mut config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone())
    .with_adk_mutation_port(port.clone());
    if include_chat {
        config = config.with_adk_chat_stream_port(port);
    }
    let runtime = crate::product_runtime::ProductRuntimeState::product_only(&config);
    let prepared = crate::product::prepare_product_with_runtime_state(config, runtime, None)
        .await
        .unwrap();
    crate::product::expose_prepared_product(prepared).unwrap()
}

// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:410 TestStreamReconnectAndSkillContracts
// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:735 TestExternalSkillUninstallRemovesInstallDir
#[tokio::test]
async fn production_skill_directory_is_discovered_on_read_and_uninstalled_by_the_mutation_owner() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    let handle = skill_router(port.clone(), false).await;
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap();
    let builtins = client
        .get(format!("{base}/api/v1/adk/skills"))
        .send()
        .await
        .unwrap();
    assert_eq!(builtins.status(), 200);
    let builtins: Value = builtins.json().await.unwrap();
    assert!(
        builtins["data"]["skills"]
            .as_array()
            .unwrap()
            .iter()
            .any(|skill| skill["id"] == "jftrade-market")
    );
    let removed_update = client
        .put(format!("{base}/api/v1/adk/skills/jftrade-market"))
        .send()
        .await
        .unwrap();
    assert_eq!(removed_update.status(), 404);
    let invalid = client
        .post(format!("{base}/api/v1/adk/skills"))
        .json(&json!({"url":"ftp://invalid-skill"}))
        .send()
        .await
        .unwrap();
    assert_eq!(invalid.status(), 400);
    let invalid: Value = invalid.json().await.unwrap();
    assert!(
        invalid["error"]["message"]
            .as_str()
            .unwrap()
            .contains("valid http/https skill URL is required")
    );
    let protected = client
        .delete(format!("{base}/api/v1/adk/skills/jftrade-market"))
        .send()
        .await
        .unwrap();
    assert_eq!(protected.status(), 500);
    let protected: Value = protected.json().await.unwrap();
    assert!(
        protected["error"]["message"]
            .as_str()
            .unwrap()
            .contains("cannot be uninstalled")
    );
    let path = port
        .settings_path
        .parent()
        .unwrap()
        .join("skills/external-skill");
    fs::create_dir_all(&path).unwrap();
    fs::write(path.join("SKILL.md"), "---\nname: external-skill\ndescription: external skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nUse external references carefully.\n").unwrap();
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let external = client
        .get(format!("{base}/api/v1/adk/skills"))
        .send()
        .await
        .unwrap();
    assert_eq!(external.status(), 200);
    let external: Value = external.json().await.unwrap();
    assert_eq!(
        port.store.list_skills().unwrap(),
        rows,
        "directory projection must not acquire a second SQLite writer"
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    let deleted = client
        .delete(format!("{base}/api/v1/adk/skills/external-skill"))
        .send()
        .await
        .unwrap();
    let delete_status = deleted.status().as_u16();
    let delete_body: Value = deleted.json().await.unwrap();
    let after: Value = client
        .get(format!("{base}/api/v1/adk/skills"))
        .send()
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert!(
        external["data"]["skills"]
            .as_array()
            .unwrap()
            .iter()
            .any(|skill| skill["id"] == "external-skill"),
        "new directory missing from list: {external}; delete {delete_status}: {delete_body}"
    );
    assert_eq!(delete_status, 200, "{delete_body}");
    assert!(
        !path.exists(),
        "mutation owner must remove the external install directory"
    );
    assert!(
        !after["data"]["skills"]
            .as_array()
            .unwrap()
            .iter()
            .any(|skill| skill["id"] == "external-skill")
    );
}

// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:410 TestStreamReconnectAndSkillContracts
#[tokio::test]
async fn production_post_stream_completes_once_and_both_reconnect_routes_keep_final_history() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    port.store
        .upsert_agent(
            "agent-stream-skill",
            &json!({"id":"agent-stream-skill",
        "providerId":"provider-readiness", "model":"fixture-model"})
            .to_string(),
        )
        .unwrap();
    let calls = Arc::new(AtomicUsize::new(0));
    let observed = calls.clone();
    let mock = axum::Router::new().route("/v1/responses", axum::routing::post(move |axum::Json(body): axum::Json<Value>| {
        let calls = observed.clone();
        async move {
            assert_eq!(body["stream"], true);
            calls.fetch_add(1, Ordering::SeqCst);
            let data = [json!({"type":"response.output_text.delta", "delta":"stream skill answer"}),
                json!({"type":"response.completed", "response":{"id":"response-stream-skill", "model":"fixture-model"}})]
                .into_iter().map(|value| format!("data: {value}\n\n")).collect::<String>() + "data: [DONE]\n\n";
            ([("content-type", "text/event-stream")], data)
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
    let handle = skill_router(port.clone(), true).await;
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .unwrap();
    let posted = client.post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&json!({"clientRequestId":"77777777-7777-4777-8777-777777777741", "agentId":"agent-stream-skill", "message":"hello reconnect"}))
        .send().await.unwrap();
    assert_eq!(posted.status(), 200);
    let stream_id = posted.headers()["x-adk-stream-id"]
        .to_str()
        .unwrap()
        .to_owned();
    assert!(!stream_id.is_empty());
    let body = posted.text().await.unwrap();
    let final_event: Value = body
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .map(|data| serde_json::from_str::<Value>(data).unwrap())
        .find(|event| event["type"] == "final")
        .unwrap();
    assert_eq!(final_event["response"]["run"]["status"], "COMPLETED");
    port.shutdown_with_error().unwrap();
    let runs = port.store.list_runs().unwrap();
    assert_eq!(runs.len(), 1);
    let run_id = &runs[0].id;
    let audit = port.store.list_audit_events().unwrap();
    let native = port.session_store.list_events(&runs[0].session_id).unwrap();
    let invalid = client
        .get(format!("{base}/api/v1/adk/streams/{stream_id}?after=-1"))
        .send()
        .await
        .unwrap();
    assert_eq!(invalid.status(), 400);
    for path in [
        format!("/api/v1/adk/streams/{stream_id}?after=1"),
        format!("/api/v1/adk/runs/{run_id}/stream?after=1"),
    ] {
        let response = client.get(format!("{base}{path}")).send().await.unwrap();
        assert_eq!(response.status(), 200);
        assert_eq!(response.headers()["x-adk-stream-id"], stream_id);
        assert!(response.text().await.unwrap().contains("\"replay\":true"));
    }
    for path in [
        "/api/v1/adk/streams/stream-missing",
        "/api/v1/adk/runs/run-missing/stream",
    ] {
        assert_eq!(
            client
                .get(format!("{base}{path}"))
                .send()
                .await
                .unwrap()
                .status(),
            404
        );
    }
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    stop.send(()).unwrap();
    tokio::time::timeout(Duration::from_secs(3), server)
        .await
        .unwrap()
        .unwrap()
        .unwrap();
    assert_eq!(calls.load(Ordering::SeqCst), 1);
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        port.session_store.list_events(&runs[0].session_id).unwrap(),
        native
    );
}

#[tokio::test]
async fn production_skill_directory_reports_metadata_and_rejects_corrupt_frontmatter_without_writes()
 {
    use sha2::Digest;
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    let path = port
        .settings_path
        .parent()
        .unwrap()
        .join("skills/guard-skill");
    fs::create_dir_all(&path).unwrap();
    let raw = "---\nname: guard-skill\ndescription: guarded metadata\nallowed-tools: [missing.tool]\nmetadata:\n  source: https://example.com/guard/SKILL.md\n  version: '2'\n---\nInstructions.\n";
    fs::write(path.join("SKILL.md"), raw).unwrap();
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let handle = skill_router(port.clone(), false).await;
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap();
    let response = client
        .get(format!("{base}/api/v1/adk/skills"))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    let body: Value = response.json().await.unwrap();
    let skill = body["data"]["skills"]
        .as_array()
        .unwrap()
        .iter()
        .find(|skill| skill["id"] == "guard-skill")
        .unwrap();
    assert_eq!(skill["source"], "https://example.com/guard/SKILL.md");
    assert_eq!(skill["description"], "guarded metadata");
    assert_eq!(skill["version"], "2");
    assert_eq!(skill["tools"], json!(["missing.tool"]));
    assert_eq!(skill["validationStatus"], "WARNING");
    assert!(
        skill["validationError"]
            .as_str()
            .unwrap()
            .contains("missing.tool")
    );
    assert_eq!(skill["builtin"], false);
    assert_eq!(
        skill["installPath"],
        path.join("SKILL.md").to_str().unwrap()
    );
    let digest = sha2::Sha256::digest(raw)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    assert_eq!(skill["contentHash"], digest);
    fs::write(
        path.join("SKILL.md"),
        "---\nname: guard-skill\nname: duplicate\n---",
    )
    .unwrap();
    let corrupt = client
        .get(format!("{base}/api/v1/adk/skills"))
        .send()
        .await
        .unwrap();
    assert_eq!(corrupt.status(), 500);
    assert_eq!(
        corrupt.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_SKILL_LIST_FAILED"
    );
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}
