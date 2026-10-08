use super::*;

fn builtin_payload(port: &ProductionAdkPort) -> Value {
    json!({
        "id": "jftrade-default", "name": "默认助手", "builtin": true,
        "instruction": "Keep the approved instruction", "status": "ENABLED",
        "tools": port.tool_catalog.ids(), "skills": ["jftrade-market"],
        "toolAccessMode": "selected", "permissionMode": "all",
        "memoryEnabled": true, "recentUserWindow": 12,
        "workMode": "chat", "loopMaxIterations": 10,
        "providerId": "", "model": "", "reasoningEffort": "",
    })
}

async fn builtin_product() -> (
    Arc<ProductionAdkPort>,
    tempfile::TempDir,
    crate::product::ProductHandle,
    Value,
) {
    let (port, directory) = agent_validation_port();
    port.store.upsert_provider("builtin-reasoning-provider", &json!({
        "id": "builtin-reasoning-provider", "displayName": "Reasoning Provider",
        "baseUrl": "http://127.0.0.1:1/v1", "model": "reasoning-model", "enabled": true,
        "reasoningConfig": {"requestField":"reasoning_effort", "mappings":[{"effort":"max","value":"max"}]},
    }).to_string()).expect("provider");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        br#"{"provider-enabled":"sk-fixture","builtin-reasoning-provider":"sk-test"}"#,
    )
    .expect("keys");
    let current = builtin_payload(&port);
    port.store
        .upsert_agent("jftrade-default", &current.to_string())
        .expect("current builtin");
    let port = Arc::new(port);
    let config = ProductConfig::test_cutover("127.0.0.1:0".parse().unwrap(), &port.settings_path)
        .unwrap()
        .with_adk_read_snapshot_port(port.clone())
        .with_adk_mutation_port(port.clone());
    let handle = start_product(config).await.expect("HTTP product");
    (port, directory, handle, current)
}

fn stored_builtin(port: &ProductionAdkPort) -> jftrade_store_sqlite::StoredAdkEntity {
    port.store
        .get_agent("jftrade-default")
        .unwrap()
        .expect("builtin row")
}

fn assert_unchanged(port: &ProductionAdkPort, before: &jftrade_store_sqlite::StoredAdkEntity) {
    let after = stored_builtin(port);
    assert_eq!(after.payload_json, before.payload_json);
    assert_eq!(after.created_at, before.created_at);
    assert_eq!(after.updated_at, before.updated_at);
}

/// Parity: go:452dea11:internal/assistant/service_builtin_agent_edit_test.go:10 TestPrimaryBuiltinAgentAllowsOnlyProviderReasoningSettings
#[tokio::test]
async fn builtin_agent_http_updates_provider_reasoning_and_keeps_protected_fields() {
    let (port, directory, handle, mut request) = builtin_product().await;
    request["providerId"] = json!("builtin-reasoning-provider");
    request["model"] = json!("reasoning-model");
    request["reasoningEffort"] = json!("max");
    let path = "/api/v1/adk/agents/jftrade-default";
    let (status, updated) = request_json_with_status(
        handle.startup_record().address,
        "PUT",
        path,
        Some(&request.to_string()),
    )
    .await;
    assert_eq!(status, 200, "{updated}");
    for key in ["providerId", "model", "reasoningEffort"] {
        assert_eq!(updated["data"][key], request[key], "{key}");
    }
    let before = stored_builtin(&port);
    for (key, value) in [
        ("instruction", json!("replace protected instruction")),
        ("status", json!("DISABLED")),
    ] {
        let mut denied = request.clone();
        denied[key] = value;
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            "PUT",
            path,
            Some(&denied.to_string()),
        )
        .await;
        assert_eq!(status, 409, "{key}: {response}");
        assert_eq!(response["error"]["code"], "ADK_AGENT_PROTECTED");
        assert_unchanged(&port, &before);
    }
    let persisted: Value = serde_json::from_str(&before.payload_json).unwrap();
    for key in [
        "name",
        "instruction",
        "status",
        "tools",
        "skills",
        "toolAccessMode",
        "permissionMode",
        "memoryEnabled",
        "recentUserWindow",
        "workMode",
        "loopMaxIterations",
        "builtin",
    ] {
        assert_eq!(persisted[key], request[key], "protected {key}");
    }
    handle.shutdown().await.unwrap();
    drop(port);
    let reopened = AdkStore::open(directory.path().join("adk.db")).expect("reopen");
    let restored = reopened.get_agent("jftrade-default").unwrap().unwrap();
    assert_eq!(restored.payload_json, before.payload_json);
}

// Supplemental create-path regression: the same builtin ID must not bypass protection.
/// Parity: go:452dea11:internal/assistant/service_builtin_agent_edit_test.go:10 TestPrimaryBuiltinAgentAllowsOnlyProviderReasoningSettings
#[tokio::test]
async fn builtin_agent_http_create_cannot_overwrite_protected_behavior() {
    let (port, _directory, handle, current) = builtin_product().await;
    let before = stored_builtin(&port);
    for (key, value) in [
        ("instruction", json!("replace protected instruction")),
        ("status", json!("DISABLED")),
        ("name", json!("Changed identity")),
        ("tools", json!([])),
        ("permissionMode", json!("none")),
        ("builtin", json!(false)),
    ] {
        let mut denied = current.clone();
        denied[key] = value;
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            "POST",
            "/api/v1/adk/agents",
            Some(&denied.to_string()),
        )
        .await;
        assert_eq!(status, 409, "{key}: {response}");
        assert_eq!(response["error"]["code"], "ADK_AGENT_PROTECTED");
        assert_unchanged(&port, &before);
    }
    handle.shutdown().await.unwrap();
}

/// Parity: go:452dea11:internal/assistant/service_persistence_runtime_boundaries_test.go:228 TestAgentValidationAndSchedulerBoundariesProtectRuntimeResources
/// Parity: go:452dea11:internal/api/assistant/chat_helpers_test.go:260 TestAssistantRequestHelpersCoverInvalidAndBoundaryInputs
#[tokio::test]
async fn builtin_agent_http_rejects_invalid_provider_configuration_without_rewriting_the_row() {
    let (port, _directory, handle, current) = builtin_product().await;
    let before = stored_builtin(&port);
    for patch in [
        json!({"providerId":"provider-missing"}),
        json!({"providerId":"provider-disabled"}),
        json!({"providerId":"provider-no-key"}),
        json!({"providerId":"builtin-reasoning-provider","reasoningEffort":"unsupported"}),
        json!({"providerId":7}),
        json!({"model":["reasoning-model"]}),
        json!({"reasoningEffort":true}),
    ] {
        let mut request = current.clone();
        for (key, value) in patch.as_object().unwrap() {
            request[key] = value.clone();
        }
        for (method, path) in [
            ("PUT", "/api/v1/adk/agents/jftrade-default"),
            ("POST", "/api/v1/adk/agents"),
        ] {
            let (status, response) = request_json_with_status(
                handle.startup_record().address,
                method,
                path,
                Some(&request.to_string()),
            )
            .await;
            assert_eq!(status, 400, "{patch}: {response}");
            assert_eq!(response["error"]["code"], "BAD_REQUEST");
            assert_unchanged(&port, &before);
        }
    }
    for (method, path) in [
        ("PUT", "/api/v1/adk/agents/jftrade-default"),
        ("POST", "/api/v1/adk/agents"),
    ] {
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            method,
            path,
            Some(r#"{"id":"jftrade-default"}"#),
        )
        .await;
        assert_eq!(status, 409, "{response}");
        assert!(
            response["error"]["message"]
                .as_str()
                .unwrap()
                .contains("only provider")
        );
        assert_unchanged(&port, &before);
    }
    handle.shutdown().await.unwrap();
}

// Supplemental sparse editor update: normalization does not rewrite protected behavior.
/// Parity: go:452dea11:internal/assistant/service_builtin_agent_edit_test.go:10 TestPrimaryBuiltinAgentAllowsOnlyProviderReasoningSettings
#[tokio::test]
async fn builtin_agent_http_saves_sparse_configuration_and_preserves_normalized_protected_fields() {
    let (port, _directory, handle, current) = builtin_product().await;
    let mut tools = current["tools"].as_array().unwrap().clone();
    tools.reverse();
    tools.push(tools[0].clone());
    let request = json!({
        "providerId":"builtin-reasoning-provider", "model":"reasoning-model", "reasoningEffort":"max",
        "name":" 默认助手 ", "instruction":" Keep the approved instruction ",
        "status":" enabled ", "permissionMode":" ALL ", "toolAccessMode":" SELECTED ",
        "workMode":" CHAT ", "tools":tools,
    });
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "PUT",
        "/api/v1/adk/agents/jftrade-default",
        Some(&request.to_string()),
    )
    .await;
    assert_eq!(status, 200, "{response}");
    let persisted: Value = serde_json::from_str(&stored_builtin(&port).payload_json).unwrap();
    for (key, value) in current.as_object().unwrap() {
        if !["providerId", "model", "reasoningEffort"].contains(&key.as_str()) {
            assert_eq!(&persisted[key], value, "protected {key}");
        }
    }
    handle.shutdown().await.unwrap();
}
