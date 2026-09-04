use super::*;

#[derive(Debug)]
struct UnreadyChatRuntime;

impl AdkChatStreamPort for UnreadyChatRuntime {
    fn dispatch(
        &self,
        _: AdkChatRoute,
        _: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
    }

    fn runtime_ready(&self) -> bool {
        false
    }
}

fn unready_adk_port() -> (ProductionAdkPort, tempfile::TempDir) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let connection = rusqlite::Connection::open(path).expect("create ADK database");
        jftrade_store_sqlite::initialize_current(&connection, component)
            .expect("initialize ADK schema");
    }
    let adk_store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("open adk artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog = Arc::new(
        ProductionToolCatalog::from_bindings(&bindings).expect("complete tool bindings"),
    );
    let port = ProductionAdkPort {
        store: adk_store,
        session_store,
        artifact_store,
        tool_catalog,
        settings_path: directory.path().join("settings.json"),
        chat_runtime: Some(Arc::new(UnreadyChatRuntime)),
    };
    (port, directory)
}

#[test]
fn chat_dispatch_rejects_an_installed_but_unready_runtime() {
    let (port, _directory) = unready_adk_port();
    let input = AdkChatInput {
        body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111"}"#.to_vec(),
        client_request_id: "11111111-1111-4111-8111-111111111111".to_owned(),
    };
    let error = port
        .dispatch(AdkChatRoute::Stream, &input)
        .expect_err("unready runtime must fail closed");
    assert!(matches!(error, AdkChatPortError::Unavailable(_)));
}

#[test]
fn tool_catalog_marks_external_unavailable_tools_non_callable() {
    let mut bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    bindings.insert(
        ProductionRouteAdapter::MarketDataSearchRead,
        ProductionAdapterBinding::ExternalUnavailable,
    );

    let catalog = ProductionToolCatalog::from_bindings(&bindings).expect("complete bindings");
    let market_search = catalog
        .tools
        .iter()
        .find(|tool| tool["id"] == "market.search")
        .expect("market search tool");
    assert_eq!(market_search["allowedModes"], json!([]));

    let system_status = catalog
        .tools
        .iter()
        .find(|tool| tool["id"] == "system.status")
        .expect("system status tool");
    assert_eq!(
        system_status["allowedModes"],
        json!(["approval", "less_approval", "all"])
    );
}

#[test]
fn research_tools_use_operation_specific_readiness() {
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::Ready),
        ("financials", ProductionAdapterBinding::Ready),
        ("valuation", ProductionAdapterBinding::ExternalUnavailable),
        ("news", ProductionAdapterBinding::ExternalUnavailable),
    ]);
    let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
        .expect("complete bindings");

    for (id, callable) in [
        ("research.instrument", true),
        ("research.financials", true),
        ("research.valuation", false),
        ("research.news", false),
    ] {
        let tool = catalog
            .tools
            .iter()
            .find(|tool| tool["id"] == id)
            .expect("research tool");
        assert_eq!(
            tool["allowedModes"]
                .as_array()
                .is_some_and(|modes| !modes.is_empty()),
            callable,
            "{id}"
        );
    }
}

#[test]
fn tool_catalog_reprojects_provider_readiness_after_activation() {
    use jftrade_settings::MarketDataProviderRuntimePort;

    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let research = BTreeMap::from([
        ("instrument", ProductionAdapterBinding::Ready),
        ("financials", ProductionAdapterBinding::Ready),
        ("valuation", ProductionAdapterBinding::Ready),
        ("news", ProductionAdapterBinding::Ready),
    ]);
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
        .expect("complete bindings")
        .with_active_provider_state(Arc::clone(&state));

    let allowed_modes = |id: &str| {
        catalog
            .values()
            .into_iter()
            .find(|tool| tool["id"] == id)
            .and_then(|tool| tool["allowedModes"].as_array().cloned())
            .expect("tool descriptor")
    };

    assert!(!allowed_modes("market.search").is_empty());
    assert!(!allowed_modes("market.snapshot").is_empty());
    assert!(!allowed_modes("research.instrument").is_empty());
    assert!(!allowed_modes("research.screen").is_empty());

    // Provider transitions update the shared state while the catalog
    // remains the same Arc-owned object. A subsequent projection must
    // reflect Futu's OpenD/router prerequisites instead of the startup
    // yfinance snapshot.
    state
        .activate(jftrade_settings::MarketDataProvider::Futu)
        .expect("provider activation");
    assert!(allowed_modes("market.search").is_empty());
    assert!(allowed_modes("research.instrument").is_empty());
    assert!(allowed_modes("market.snapshot").is_empty());
    assert!(allowed_modes("research.screen").is_empty());
    // OpenD readiness alone does not provide a news reader.  Futu news
    // remains externally unavailable until the concrete trade-runtime
    // reader is installed, so the ADK catalog must not advertise it as
    // callable after provider activation.
    assert!(allowed_modes("research.news").is_empty());

    state.set_readiness(false, true, true);
    assert!(!allowed_modes("market.snapshot").is_empty());
    assert!(!allowed_modes("market.subscriptions").is_empty());
    assert!(!allowed_modes("research.valuation").is_empty());
    assert!(allowed_modes("research.news").is_empty());
    assert!(allowed_modes("research.screen").is_empty());
}

#[test]
fn native_pine_validation_remains_callable_when_worker_is_unhealthy() {
    let mut bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    bindings.insert(
        ProductionRouteAdapter::StrategyPine,
        ProductionAdapterBinding::ExternalUnavailable,
    );
    let provider_state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    let pine_readiness = jftrade_integration_pine::PineReadinessState::new("pineworker-1");
    let catalog = ProductionToolCatalog::from_bindings(&bindings)
        .expect("complete bindings")
        .with_active_provider_state(provider_state)
        .with_backtest_execution_ready(true)
        .with_pine_readiness(Some(pine_readiness));

    let callable_ids = catalog
        .callable_tools()
        .into_iter()
        .filter_map(|tool| tool["id"].as_str().map(str::to_owned))
        .collect::<Vec<_>>();

    assert!(callable_ids.iter().any(|id| id == "strategy.validate_pine"));
    assert!(
        !callable_ids
            .iter()
            .any(|id| id == "strategy.research_backtest")
    );
}

#[test]
fn adk_tool_catalog_exposes_typed_interaction_request_user_schema() {
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let catalog = ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings");
    let tools = catalog.openai_tools();
    let request_user_tool = tools
        .into_iter()
        .find(|tool| tool["name"] == "interaction.request_user")
        .expect("interaction.request_user must be present in openai_tools");

    assert_eq!(request_user_tool["type"], "function");
    let params = &request_user_tool["parameters"];
    assert_eq!(params["type"], "object");
    assert!(params["properties"]["decisionKind"].is_object());
    assert!(params["properties"]["blockingReason"].is_object());
    assert!(params["properties"]["questions"].is_object());
    let required = params["required"]
        .as_array()
        .expect("required array")
        .iter()
        .filter_map(|v| v.as_str())
        .collect::<Vec<_>>();
    assert!(required.contains(&"decisionKind"));
    assert!(required.contains(&"blockingReason"));
    assert!(required.contains(&"questions"));
}

#[test]
fn adk_respond_to_input_enriches_response_and_unblocks_run() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let connection = rusqlite::Connection::open(path).expect("create ADK database");
        jftrade_store_sqlite::initialize_current(&connection, component)
            .expect("initialize ADK schema");
    }
    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("open adk artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        br#"{"provider-test":"test-key"}"#,
    )
    .expect("write provider secret");

    store
        .upsert_provider(
            "provider-test",
            &json!({
                "displayName": "Test Provider",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");

    store
        .upsert_agent(
            "agent-1",
            &json!({
                "id": "agent-1",
                "name": "Test Agent",
                "providerId": "provider-test",
                "model": "fixture-model",
            })
            .to_string(),
        )
        .expect("persist agent");

    let run_id = "run-input-test-1";
    let request_id = "input-req-1";
    let call_id = "call-input-1";

    let initial_payload = json!({
        "id": run_id,
        "sessionId": "session-1",
        "agentId": "agent-1",
        "providerId": "provider-test",
        "model": "fixture-model",
        "status": "PENDING_INPUT",
        "resumeState": "waiting_input",
        "requestMessage": "请帮我诊断 AAPL 策略并在必要时问我",
        "message": "等待用户回答后继续执行。",
        "toolCalls": [
            {
                "id": call_id,
                "runId": run_id,
                "functionCallId": call_id,
                "name": "interaction.request_user",
                "toolName": "interaction.request_user",
                "arguments": {
                    "decisionKind": "material_tradeoff",
                    "blockingReason": "需要选择回测杠杆模式",
                    "questions": [
                        {
                            "id": "q1",
                            "question": "是否启用杠杆？",
                            "options": [
                                {"id": "q1-o1", "label": "不启用杠杆", "recommended": true},
                                {"id": "q1-o2", "label": "启用 2x 杠杆", "recommended": false}
                            ],
                            "allowOther": true
                        }
                    ]
                },
                "status": "PENDING_INPUT",
                "requiresUser": true,
            }
        ],
        "inputRequest": {
            "id": request_id,
            "runId": run_id,
            "agentId": "agent-1",
            "functionCallId": call_id,
            "title": "回测参数确认",
            "status": "PENDING",
            "decisionKind": "material_tradeoff",
            "blockingReason": "需要选择回测杠杆模式",
            "questions": [
                {
                    "id": "q1",
                    "question": "是否启用杠杆？",
                    "options": [
                        {"id": "q1-o1", "label": "不启用杠杆", "recommended": true},
                        {"id": "q1-o2", "label": "启用 2x 杠杆", "recommended": false}
                    ],
                    "allowOther": true
                }
            ],
            "answers": [],
            "createdAt": "2026-08-23T08:00:00Z",
            "updatedAt": "2026-08-23T08:00:00Z"
        },
        "inputRequests": [
            {
                "id": request_id,
                "runId": run_id,
                "agentId": "agent-1",
                "functionCallId": call_id,
                "title": "回测参数确认",
                "status": "PENDING",
                "decisionKind": "material_tradeoff",
                "blockingReason": "需要选择回测杠杆模式",
                "questions": [
                    {
                        "id": "q1",
                        "question": "是否启用杠杆？",
                        "options": [
                            {"id": "q1-o1", "label": "不启用杠杆", "recommended": true},
                            {"id": "q1-o2", "label": "启用 2x 杠杆", "recommended": false}
                        ],
                        "allowOther": true
                    }
                ],
                "answers": [],
                "createdAt": "2026-08-23T08:00:00Z",
                "updatedAt": "2026-08-23T08:00:00Z"
            }
        ],
        "toolResults": [],
    });

    use crate::product::product_adk_mutation_port::AdkMutationPort;

    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-1",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-req-1",
            request_fingerprint: "fingerprint-1",
            payload_json: &initial_payload.to_string(),
        })
        .expect("create initial run in PENDING_INPUT");

    let cancellation_registry = Arc::new(
        crate::product::product_adk_model_runtime::RunCancellationRegistry::default(),
    );
    let adk_chat_runtime =
        crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
            Arc::clone(&store),
            Arc::clone(&session_store),
            &settings_path,
            Arc::clone(&cancellation_registry),
            Arc::clone(&tool_catalog),
        );

    let adk_port = ProductionAdkPort {
        store: Arc::clone(&store),
        session_store: Arc::clone(&session_store),
        artifact_store,
        tool_catalog,
        settings_path,
        chat_runtime: Some(adk_chat_runtime),
    };

    let mut identifiers = BTreeMap::new();
    identifiers.insert("runId".to_owned(), run_id.to_owned());

    let mutation_input = crate::product::product_adk_mutation_port::AdkMutationInput {
        operation: crate::product::product_adk_mutation_port::AdkMutationOperation::RespondToInput,
        identifiers,
        body: json!({
            "requestId": request_id,
            "answers": [
                {
                    "questionId": "q1",
                    "optionId": "q1-o1"
                }
            ]
        }),
        webhook_secret: None,
    };

    let result = adk_port
        .mutate(&mutation_input)
        .expect("respond_to_input mutation succeeds");

    let updated_run = store.get_run(run_id).expect("get run").expect("run exists");
    let payload: serde_json::Value =
        serde_json::from_str(&updated_run.payload_json).expect("parse run payload");

    assert_eq!(payload["status"], "RUNNING");
    assert!(
        payload["resumeState"] == "input_resuming"
            || payload["resumeState"] == "provider_executing",
        "expected input_resuming or provider_executing, got {:?}",
        payload["resumeState"]
    );

    let input_response = &payload["inputResponse"];
    assert_eq!(input_response["requestId"], request_id);
    assert_eq!(
        input_response["originalRequest"],
        "请帮我诊断 AAPL 策略并在必要时问我"
    );
    assert!(
        input_response["continuationInstruction"]
            .as_str()
            .unwrap()
            .contains("用户已回答以上问题。回答只是解除阻塞")
    );

    let answers = input_response["answers"].as_array().expect("answers array");
    assert_eq!(answers.len(), 1);
    assert_eq!(answers[0]["questionId"], "q1");
    assert_eq!(answers[0]["question"], "是否启用杠杆？");
    assert_eq!(answers[0]["optionId"], "q1-o1");
    assert_eq!(answers[0]["answer"], "不启用杠杆");

    let tool_results = payload["toolResults"].as_array().expect("toolResults array");
    assert_eq!(tool_results.len(), 1);
    assert_eq!(tool_results[0]["name"], "interaction.request_user");
    assert_eq!(tool_results[0]["callId"], call_id);
    assert_eq!(tool_results[0]["output"]["requestId"], request_id);

    let tool_calls = payload["toolCalls"].as_array().expect("toolCalls array");
    assert_eq!(tool_calls[0]["status"], "COMPLETED");

    assert_eq!(result["request"]["status"], "ANSWERED");
}

#[test]
fn adk_tool_executor_supports_read_only_mcp_and_pine_validation() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let connection = rusqlite::Connection::open(&adk_path).expect("create ADK database");
    jftrade_store_sqlite::initialize_current(&connection, "adk").expect("initialize ADK schema");
    drop(connection);

    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));

    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let executor = crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::new(
        Arc::clone(&tool_catalog),
        Arc::clone(&store),
    );

    assert!(executor.supports("strategy.validate_pine"));
    assert!(executor.supports("strategy.pine_spec"));
    assert!(executor.supports("tools.search"));
    assert!(executor.supports("models.list"));
    assert!(!executor.supports("nonexistent.tool"));
}

#[test]
fn adk_respond_to_input_strict_validation_idempotency_and_conflict() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let artifact_path = directory.path().join("adk_artifact.db");
    let session_path = directory.path().join("adk_session.db");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let conn = rusqlite::Connection::open(path).expect("create database");
        jftrade_store_sqlite::initialize_current(&conn, component).expect("initialize schema");
    }

    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(jftrade_store_sqlite::AdkArtifactStore::open(&artifact_path).expect("artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));

    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        br#"{"provider-test":"test-key"}"#,
    )
    .expect("write provider secret");

    store
        .upsert_provider(
            "provider-test",
            &json!({
                "displayName": "Test Provider",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");

    store
        .upsert_agent(
            "agent-1",
            &json!({
                "id": "agent-1",
                "name": "Test Agent",
                "providerId": "provider-test",
                "model": "fixture-model",
            })
            .to_string(),
        )
        .expect("persist agent");
    let cancellation_registry = Arc::new(
        crate::product::product_adk_model_runtime::RunCancellationRegistry::default(),
    );
    let runtime =
        crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
            Arc::clone(&store),
            Arc::clone(&session_store),
            &settings_path,
            cancellation_registry,
            Arc::clone(&tool_catalog),
        );

    let run_id_str = format!(
        "test-strict-input-run-{}",
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    );
    let run_id = run_id_str.as_str();
    let request_id = "req-multi-question";
    let now = "2026-09-03T12:00:00Z";

    let payload = json!({
        "id": run_id,
        "sessionId": "session-1",
        "agentId": "agent-1",
        "providerId": "provider-test",
        "model": "fixture-model",
        "status": "PENDING_INPUT",
        "resumeState": "waiting_input",
        "requestMessage": "Please answer these questions.",
        "inputRequest": {
            "id": request_id,
            "status": "PENDING",
            "decisionKind": "missing_required_context",
            "blockingReason": "We need user input to proceed.",
            "questions": [
                {
                    "id": "q1",
                    "question": "Choose an option",
                    "options": [
                        {"id": "opt-1", "label": "Option 1"},
                        {"id": "opt-2", "label": "Option 2"}
                    ],
                    "allowOther": false
                },
                {
                    "id": "q2",
                    "question": "Any other notes?",
                    "options": [
                        {"id": "opt-default", "label": "Default"}
                    ],
                    "allowOther": true
                }
            ],
            "createdAt": now,
            "updatedAt": now
        },
        "toolCalls": [
            {
                "id": "call-input-1",
                "name": "interaction.request_user",
                "status": "RUNNING"
            }
        ]
    });

    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-1",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-req-strict-1",
            request_fingerprint: "fingerprint-strict-1",
            payload_json: &payload.to_string(),
        })
        .expect("save run");

    use crate::product::product_adk_mutation_port::{
        AdkMutationInput, AdkMutationOperation, AdkMutationPort,
    };

    let port = ProductionAdkPort {
        store: Arc::clone(&store),
        session_store: Arc::clone(&session_store),
        artifact_store: Arc::clone(&artifact_store),
        tool_catalog: Arc::clone(&tool_catalog),
        settings_path: settings_path.clone(),
        chat_runtime: Some(runtime),
    };

    let mut identifiers = BTreeMap::new();
    identifiers.insert("runId".to_owned(), run_id.to_owned());

    // 1. Missing answer for q2 -> Rejected (400)
    let partial_answers = json!({
        "requestId": request_id,
        "answers": [{"questionId": "q1", "optionId": "opt-1"}]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: partial_answers,
        webhook_secret: None,
    };
    let err = port.mutate(&input).expect_err("partial answers must be rejected");
    assert!(format!("{err}").contains("submitted 1 answers but request has 2 questions"));

    // 2. Invalid option for q1 -> Rejected (400)
    let invalid_opt_answers = json!({
        "requestId": request_id,
        "answers": [
            {"questionId": "q1", "optionId": "opt-invalid"},
            {"questionId": "q2", "optionId": "opt-default"}
        ]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: invalid_opt_answers,
        webhook_secret: None,
    };
    let err = port.mutate(&input).expect_err("invalid option must be rejected");
    assert!(format!("{err}").contains("invalid option for q1"));

    // 2b. Label used instead of optionId -> Rejected (400)
    let label_as_opt_answers = json!({
        "requestId": request_id,
        "answers": [
            {"questionId": "q1", "optionId": "Option 1"},
            {"questionId": "q2", "optionId": "opt-default"}
        ]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: label_as_opt_answers,
        webhook_secret: None,
    };
    let err = port.mutate(&input).expect_err("label as optionId must be rejected");
    assert!(format!("{err}").contains("invalid option for q1"));

    // 3. otherText on q1 which disallows other -> Rejected (400)
    let disallow_other_answers = json!({
        "requestId": request_id,
        "answers": [
            {"questionId": "q1", "otherText": "custom text"},
            {"questionId": "q2", "optionId": "opt-default"}
        ]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: disallow_other_answers,
        webhook_secret: None,
    };
    let err = port.mutate(&input).expect_err("otherText must be rejected when allowOther is false");
    assert!(format!("{err}").contains("q1 does not allow other text"));

    // 4. Valid answers: q1 with option, q2 with otherText -> Accepted (200)
    let valid_answers = json!({
        "requestId": request_id,
        "answers": [
            {"questionId": "q1", "optionId": "opt-1"},
            {"questionId": "q2", "otherText": "Special instructions"}
        ]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: valid_answers.clone(),
        webhook_secret: None,
    };
    let result = port.mutate(&input).expect("valid answers must succeed");
    assert_eq!(result["request"]["status"], "ANSWERED");
    assert_eq!(result["run"]["status"], "RUNNING");

    // 5. Idempotent retry with identical answers -> 200 OK
    let retry_result = port.mutate(&input).expect("identical answers must be idempotent 200 OK");
    assert_eq!(retry_result["request"]["status"], "ANSWERED");

    // 6. Second answer with conflicting answer -> 409 Conflict
    let conflict_answers = json!({
        "requestId": request_id,
        "answers": [
            {"questionId": "q1", "optionId": "opt-2"},
            {"questionId": "q2", "otherText": "Special instructions"}
        ]
    });
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: identifiers.clone(),
        body: conflict_answers,
        webhook_secret: None,
    };
    let err = port.mutate(&input).expect_err("different answers must return 409 conflict");
    assert!(format!("{err}").contains("ADK_INPUT_RESPONSE_CONFLICT"));

    // 7. Request with empty questions rejects non-empty answers (400)
    let empty_q_run_id = format!(
        "run-empty-q-{}",
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    );
    let empty_q_payload = json!({
        "id": empty_q_run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": "req-empty-q",
            "status": "PENDING",
            "questions": []
        }
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: &empty_q_run_id,
            session_id: "session-1",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-req-empty-q",
            request_fingerprint: "fingerprint-empty-q",
            payload_json: &empty_q_payload.to_string(),
        })
        .expect("save empty-q run");
    let mut empty_q_ident = BTreeMap::new();
    empty_q_ident.insert("runId".to_owned(), empty_q_run_id.clone());
    let empty_q_input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: empty_q_ident,
        body: json!({"requestId": "req-empty-q", "answers": [{"questionId": "any", "optionId": "o1"}]}),
        webhook_secret: None,
    };
    let err = port
        .mutate(&empty_q_input)
        .expect_err("empty questions must reject answers");
    assert!(format!("{err}").contains("submitted 1 answers but request has 0 questions"));

    // 8. Rollback when continuation worker spawn fails
    #[derive(Debug)]
    struct FailingChatRuntime;

    impl AdkChatStreamPort for FailingChatRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Err(AdkChatPortError::Unavailable(
                "spawn worker failed".to_owned(),
            ))
        }

        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            Err(AdkChatPortError::Unavailable(
                "continuation worker unavailable".to_owned(),
            ))
        }

        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let failing_port = ProductionAdkPort {
        store: Arc::clone(&store),
        session_store: Arc::clone(&session_store),
        artifact_store: Arc::clone(&artifact_store),
        tool_catalog: Arc::clone(&tool_catalog),
        settings_path: settings_path.clone(),
        chat_runtime: Some(Arc::new(FailingChatRuntime)),
    };
    let rollback_run_id = format!(
        "run-rollback-{}",
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    );
    let rollback_payload = json!({
        "id": rollback_run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": "req-rollback",
            "status": "PENDING",
            "questions": [{"id": "q1", "options": [{"id": "o1"}]}]
        },
        "toolCalls": [{"id": "tc-rollback", "name": "interaction.request_user", "status": "RUNNING"}]
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: &rollback_run_id,
            session_id: "session-1",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-req-rollback",
            request_fingerprint: "fingerprint-rollback",
            payload_json: &rollback_payload.to_string(),
        })
        .expect("save rollback run");
    let mut rollback_ident = BTreeMap::new();
    rollback_ident.insert("runId".to_owned(), rollback_run_id.clone());
    let rollback_input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: rollback_ident,
        body: json!({"requestId": "req-rollback", "answers": [{"questionId": "q1", "optionId": "o1"}]}),
        webhook_secret: None,
    };
    let err = failing_port
        .mutate(&rollback_input)
        .expect_err("continuation spawn failure must return 503");
    assert!(format!("{err}").contains("ADK_CONTINUATION_UNAVAILABLE"));
    let restored_run = store
        .get_run(&rollback_run_id)
        .expect("get run")
        .expect("run exists");
    assert_eq!(restored_run.status, "RUNNING");
    let restored_payload: Value =
        serde_json::from_str(&restored_run.payload_json).expect("parse json");
    assert_eq!(
        restored_payload.get("resumeState").and_then(Value::as_str),
        Some("input_resume_pending")
    );
    assert!(restored_payload.get("inputResumeCheckpoint").is_some());
    assert_eq!(
        restored_payload["inputResumeCheckpoint"]["requestId"],
        "req-rollback"
    );
    assert!(restored_payload.get("inputResponse").is_some());
}

#[test]
fn adk_respond_to_input_concurrent_cas_winner_loser_semantics() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let conn = rusqlite::Connection::open(path).expect("create database");
        jftrade_store_sqlite::initialize_current(&conn, component).expect("initialize schema");
    }

    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));
    use crate::product::product_adk_mutation_port::{
        AdkMutationInput, AdkMutationOperation, AdkMutationPort,
    };

    #[derive(Debug)]
    struct SuccessfulResumeChatRuntime;

    impl AdkChatStreamPort for SuccessfulResumeChatRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }

        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            Ok(())
        }

        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let runtime = Arc::new(SuccessfulResumeChatRuntime);

    let port = ProductionAdkPort {
        store: Arc::clone(&store),
        session_store,
        artifact_store,
        tool_catalog,
        settings_path,
        chat_runtime: Some(runtime),
    };

    let run_id = "run-cas-race-1";
    let request_id = "req-cas-race-1";
    let payload = json!({
        "id": run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": request_id,
            "status": "PENDING",
            "questions": [
                {"id": "q1", "options": [{"id": "opt-A"}, {"id": "opt-B"}]}
            ]
        },
        "toolCalls": [{"id": "tc1", "name": "interaction.request_user", "status": "RUNNING"}]
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-cas",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-cas-1",
            request_fingerprint: "fingerprint-cas-1",
            payload_json: &payload.to_string(),
        })
        .expect("save run");

    // Simulate CAS winner having already transitioned to RUNNING with opt-A
    let winner_payload = json!({
        "id": run_id,
        "status": "RUNNING",
        "inputRequest": {
            "id": request_id,
            "status": "ANSWERED",
            "answers": [{"questionId": "q1", "optionId": "opt-A"}],
            "questions": [
                {"id": "q1", "options": [{"id": "opt-A"}, {"id": "opt-B"}]}
            ]
        },
        "inputResponse": {
            "requestId": request_id,
            "answers": [{"questionId": "q1", "optionId": "opt-A"}]
        }
    });
    let initial_run = store.get_run(run_id).unwrap().unwrap();
    store
        .update_run_state_if_status_and_revision(
            run_id,
            "PENDING_INPUT",
            &initial_run.updated_at,
            "RUNNING",
            &winner_payload.to_string(),
        )
        .unwrap();

    let mut ident = BTreeMap::new();
    ident.insert("runId".to_owned(), run_id.to_owned());

    // Concurrent loser submits IDENTICAL answer -> CAS fails, but winner comparison returns 200 OK
    let same_answers_input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: ident.clone(),
        body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-A"}]}),
        webhook_secret: None,
    };
    let ok_res = port
        .mutate(&same_answers_input)
        .expect("CAS loser with same answer returns 200 OK");
    assert_eq!(ok_res["run"]["status"], "RUNNING");

    // Concurrent loser submits CONFLICTING answer -> CAS fails, winner comparison returns 409 Conflict
    let diff_answers_input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: ident,
        body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-B"}]}),
        webhook_secret: None,
    };
    let err = port
        .mutate(&diff_answers_input)
        .expect_err("CAS loser with differing answer returns 409 Conflict");
    assert!(format!("{err}").contains("ADK_INPUT_RESPONSE_CONFLICT"));
}

#[test]
fn adk_mcp_tool_executor_bundle_attachment_and_exact_schemas() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let connection = rusqlite::Connection::open(&adk_path).expect("create ADK database");
    jftrade_store_sqlite::initialize_current(&connection, "adk").expect("initialize ADK schema");
    drop(connection);

    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));

    use crate::product::product_adk_model_runtime::AdkToolExecutor;
    let executor = crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::new(
        Arc::clone(&tool_catalog),
        Arc::clone(&store),
    );

    // 1. Prior to attaching ports: leaf tools are supported, bundle tools are not
    assert!(executor.supports("strategy.validate_pine"));
    assert!(executor.supports("tools.search"));
    assert!(!executor.supports("system.status"));
    assert!(!executor.supports("portfolio.summary"));

    let pine_val = executor
        .execute(
            "strategy.validate_pine",
            &json!({
                "script": "//@version=6\nstrategy(\"T\")\nplot(close)"
            }),
        )
        .expect("validate pine leaf execution");
    assert_eq!(pine_val["ok"], true);

    let unavailable_err = executor
        .execute("system.status", &json!({}))
        .expect_err("system.status should fail without bundle");
    assert!(unavailable_err.contains("unavailable"));

    // 2. Build and attach bundle
    let bundle_dir = tempfile::tempdir().expect("bundle directory");
    let settings_path = bundle_dir.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    crate::product::product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");
    let settings = Arc::new(
        jftrade_store_settings_file::SettingsFileStore::open(&settings_path)
            .expect("settings store"),
    );
    #[derive(Debug)]
    struct ReadyRuntimeStatus;

    impl crate::product::MarketDataRuntimeStatusPort for ReadyRuntimeStatus {
        fn snapshot(&self) -> crate::product::MarketDataRuntimeState {
            crate::product::MarketDataRuntimeState {
                connected: true,
                generation: 1,
                ..Default::default()
            }
        }
    }

    let security = crate::product::SecuritySettingsService::new(settings);
    let active = Arc::new(crate::product::ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let runtime = Arc::new(
        crate::product::product_production_ports::SharedTradeReadRuntime::default(),
    );
    let mut config = crate::product::ProductConfig::new(
        "127.0.0.1:0".parse().expect("bind address"),
        &settings_path,
        crate::product::AccessPolicy::default(),
    )
    .expect("product config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime)
    .with_market_data_runtime_status_port(Arc::new(ReadyRuntimeStatus));
    config.capabilities = crate::product::ProductCapabilities::all();
    config.production = true;
    let ports = Arc::new(
        crate::product::product_production_ports::production_ports(&config, &security)
            .expect("production ports"),
    );

    executor.attach_ports(Arc::clone(&ports));

    // 3. After attaching bundle: bundle-backed tools are supported
    assert!(executor.supports("system.status"));
    assert!(executor.supports("portfolio.summary"));
    assert!(executor.supports("portfolio.accounts"));
    assert!(executor.supports("portfolio.overview"));
    assert!(executor.supports("portfolio.positions"));
    assert!(executor.supports("strategy.research_backtest"));
    assert!(executor.supports("market.snapshots"));

    let search_res = executor
        .execute("tools.search", &json!({"query": "pine"}))
        .expect("search tools");
    assert!(
        search_res["tools"]
            .as_array()
            .unwrap()
            .iter()
            .any(|t| t["id"] == "strategy.validate_pine")
    );

    // 4. Detach ports: bundle tools become unsupported again
    executor.detach_ports();
    assert!(!executor.supports("system.status"));
    assert!(!executor.supports("portfolio.summary"));
    assert!(!executor.supports("portfolio.accounts"));
    assert!(!executor.supports("portfolio.overview"));
    assert!(!executor.supports("portfolio.positions"));
    assert!(!executor.supports("strategy.research_backtest"));

    // 5. Verify openai_tools exposes exact schemas and does not include removed tools
    let openai_tools = tool_catalog.openai_tools();
    let snapshots_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("market.snapshots"))
        .expect("market.snapshots must be exposed in openai_tools");
    let params = &snapshots_tool["parameters"];
    assert_eq!(params["type"], "object");
    assert!(
        params["properties"].get("symbols").is_some(),
        "market.snapshots must have symbols property"
    );
    assert_eq!(params["required"], json!(["symbols"]));

    let portfolio_summary_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("portfolio.summary"))
        .expect("portfolio.summary must be exposed in openai_tools");
    assert_eq!(portfolio_summary_tool["parameters"]["type"], "object");

    let research_backtest_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("strategy.research_backtest"))
        .expect("strategy.research_backtest must be exposed in openai_tools");
    assert_eq!(
        research_backtest_tool["parameters"]["required"],
        json!(["script", "market", "startTime", "endTime"])
    );

    let portfolio_accounts_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("portfolio.accounts"))
        .expect("portfolio.accounts must be exposed in openai_tools");
    assert_eq!(portfolio_accounts_tool["parameters"]["type"], "object");
    assert_eq!(
        portfolio_accounts_tool["parameters"]["required"],
        json!(["tradingEnvironment"])
    );

    let portfolio_overview_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("portfolio.overview"))
        .expect("portfolio.overview must be exposed in openai_tools");
    assert_eq!(portfolio_overview_tool["parameters"]["type"], "object");
    assert_eq!(
        portfolio_overview_tool["parameters"]["required"],
        json!(["tradingEnvironment"])
    );

    let portfolio_positions_tool = openai_tools
        .iter()
        .find(|t| t.get("name").and_then(Value::as_str) == Some("portfolio.positions"))
        .expect("portfolio.positions must be exposed in openai_tools");
    assert_eq!(portfolio_positions_tool["parameters"]["type"], "object");
    assert_eq!(
        portfolio_positions_tool["parameters"]["required"],
        json!(["tradingEnvironment"])
    );
}

#[test]
fn test_canonical_input_answers_idempotency_and_conflict() {
    use crate::product::product_adk_input_canonical::CanonicalInputAnswers;

    let ans1 = json!([
        {
            "questionId": "q1",
            "optionId": "opt1",
            "question": "What is the capital?",
            "answer": "Beijing",
        },
        {
            "questionId": "q2",
            "otherText": "custom text",
            "question": "Other comments?",
        }
    ]);

    let ans2_different_enriched = json!([
        {
            "questionId": "q2",
            "otherText": "custom text",
            "question": "Different prompt text for q2?",
        },
        {
            "questionId": "q1",
            "optionId": "opt1",
            "question": "Another prompt text?",
            "answer": "Different label text",
        }
    ]);

    let ans3_different_option = json!([
        {
            "questionId": "q1",
            "optionId": "opt2",
        },
        {
            "questionId": "q2",
            "otherText": "custom text",
        }
    ]);

    let c1 = CanonicalInputAnswers::from_values(ans1.as_array().unwrap());
    let c2 = CanonicalInputAnswers::from_values(ans2_different_enriched.as_array().unwrap());
    let c3 = CanonicalInputAnswers::from_values(ans3_different_option.as_array().unwrap());

    // Ordering and enriched fields do not affect equality
    assert!(c1.matches(&c2));
    // Different option choice produces a conflict
    assert!(!c1.matches(&c3));
}

#[derive(Debug)]
struct AdkTestReadyRuntimeStatus;

impl crate::product::MarketDataRuntimeStatusPort for AdkTestReadyRuntimeStatus {
    fn snapshot(&self) -> crate::product::MarketDataRuntimeState {
        crate::product::MarketDataRuntimeState {
            connected: true,
            generation: 1,
            ..Default::default()
        }
    }
}

#[derive(Debug)]
struct AdkTestTradeReadPort;

impl jftrade_integration_futu::TradeReadPort for AdkTestTradeReadPort {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeAccountSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(vec![jftrade_integration_futu::TradeAccountSnapshot {
            trd_env: 1,
            acc_id: 42,
            trd_market_auth_list: vec![1, 2],
            acc_type: Some(2),
            card_num: None,
            security_firm: Some(1),
            sim_acc_type: None,
            uni_card_num: None,
            acc_status: Some(0),
            acc_role: Some(1),
            jp_acc_type: Vec::new(),
            competition_acc_name: None,
        }])
    }

    fn read_funds(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<jftrade_integration_futu::TradeFundsSnapshot, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("funds unsupported".into()))
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<jftrade_integration_futu::TradeCashFlowSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("cash flows unsupported".into()))
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("fees unsupported".into()))
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("margin ratios unsupported".into()))
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<jftrade_integration_futu::TradeMaxTradeQuantitySnapshot, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("quantity unsupported".into()))
    }

    fn read_combo_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeComboMaxTradeQuantityRequest,
    ) -> Result<jftrade_integration_futu::TradeComboMaxTradeQuantitySnapshot, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("combo unsupported".into()))
    }

    fn read_positions(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradePositionSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeFillSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }
}

#[derive(Debug)]
struct AdkTestBacktestExecution;

impl crate::product::BacktestExecutionPort for AdkTestBacktestExecution {
    fn execute(
        &self,
        request: crate::product::BacktestExecutionRequest,
    ) -> Result<Value, crate::product::BacktestExecutionError> {
        Ok(json!({
            "runId": request.run_id,
            "bars": request.candles.len(),
            "marketDataProvider": request.market_data_provider,
        }))
    }
}

#[derive(Debug)]
struct AdkTestHistoricalKlinePort;

impl jftrade_integration_futu::HistoricalKlineReadPort for AdkTestHistoricalKlinePort {
    fn query(
        &self,
        query: &jftrade_integration_futu::HistoricalKlineQuery,
    ) -> Result<jftrade_integration_futu::HistoricalKlineResult, jftrade_integration_futu::HistoricalKlineError> {
        Ok(jftrade_integration_futu::HistoricalKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity {
                market: query.market,
                code: query.symbol.clone(),
            },
            name: Some("Test security".to_owned()),
            klines: vec![],
            next_req_key: vec![],
        })
    }
}

fn setup_test_bundle_and_executor(
) -> (
    Arc<crate::product::product_production_ports::ProductionPortBundle>,
    crate::product::product_adk_model_runtime::ProductionAdkToolExecutor,
    tempfile::TempDir,
) {
    let bundle_dir = tempfile::tempdir().expect("bundle directory");
    let settings_path = bundle_dir.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    crate::product::product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");

    let backtest_data_path = bundle_dir.path().join("backtest.db");
    let market_data_store =
        jftrade_store_sqlite::BacktestMarketDataStore::open(&backtest_data_path)
            .expect("market data store");
    for provider in ["akshare", "futu"] {
        market_data_store
            .insert_candles(
                provider,
                "HK.00700",
                "1m",
                "forward",
                "regular",
                &[jftrade_store_sqlite::StoredBacktestCandle {
                    start_time: 1_767_225_600_000,
                    end_time: 1_767_225_659_999,
                    open: "300.0".to_owned(),
                    high: "305.0".to_owned(),
                    low: "295.0".to_owned(),
                    close: "302.0".to_owned(),
                    volume: "1000".to_owned(),
                }],
            )
            .expect("seed candles");
    }
    drop(market_data_store);

    let settings = Arc::new(
        jftrade_store_settings_file::SettingsFileStore::open(&settings_path)
            .expect("settings store"),
    );
    let security = crate::product::SecuritySettingsService::new(settings);
    let active = Arc::new(crate::product::ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let runtime = Arc::new(
        crate::product::product_production_ports::SharedTradeReadRuntime::default(),
    );
    runtime.set_historical_klines(Some(Arc::new(AdkTestHistoricalKlinePort)));
    let mut config = crate::product::ProductConfig::new(
        "127.0.0.1:0".parse().expect("bind address"),
        &settings_path,
        crate::product::AccessPolicy::default(),
    )
    .expect("product config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime)
    .with_trade_read_port(Some(Arc::new(AdkTestTradeReadPort)), Some(true))
    .with_market_data_runtime_status_port(Arc::new(AdkTestReadyRuntimeStatus))
    .with_backtest_execution_port(Arc::new(AdkTestBacktestExecution));
    config.capabilities = crate::product::ProductCapabilities::all();
    config.production = true;
    let ports = Arc::new(
        crate::product::product_production_ports::production_ports(&config, &security)
            .expect("production ports"),
    );

    let executor = crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(&ports),
    );

    (ports, executor, bundle_dir)
}

#[tokio::test]
async fn test_portfolio_and_research_backtest_execution_dispatch() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (ports, executor, _dir) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    // 1. Portfolio accounts query with explicit non-existent account returns partial result with not_found status and warnings
    let non_existent = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "accountId": "non-existent-999"}),
        )
        .expect("portfolio.accounts returns partial response on not_found");
    assert_eq!(non_existent["selection"]["status"], "not_found");
    assert_eq!(non_existent["partial"], true);
    assert!(!non_existent["warnings"].as_array().unwrap().is_empty());

    // 2. Portfolio accounts query with valid environment succeeds
    let accounts_res = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts");
    assert_eq!(accounts_res["selection"]["status"], "resolved");
    assert_eq!(accounts_res["brokerRuntime"]["connectivity"], "connected");

    // Portfolio overview query succeeds
    let overview_res = executor
        .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.overview");
    assert_eq!(overview_res["selection"]["status"], "resolved");
    assert!(overview_res["accountOverviews"].is_array());

    // Portfolio positions query succeeds
    let positions_res = executor
        .execute("portfolio.positions", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.positions");
    assert_eq!(positions_res["selection"]["status"], "resolved");
    assert!(positions_res["accountPositions"].is_array());

    // 3. Strategy research backtest with invalid script fails validation
    let invalid_backtest = executor.execute(
        "strategy.research_backtest",
        &json!({
            "script": "//@version=6\nstrategy(\"Broken\", broken=true",
            "market": "HK",
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-02T00:00:00Z",
        }),
    );
    assert!(invalid_backtest.is_err());
    assert!(invalid_backtest.unwrap_err().contains("validation failed"));

    // 4. Strategy research backtest with valid inline script and no definitionId
    let valid_backtest = executor.execute(
        "strategy.research_backtest",
        &json!({
            "script": "//@version=6\nstrategy(\"Valid Test\", overlay=true)\nplot(close)\n",
            "market": "HK",
            "symbol": "HK.00700",
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-02T00:00:00Z",
            "waitForCompletionMs": 0,
        }),
    );
    assert!(valid_backtest.is_ok(), "research backtest start failed: {:?}", valid_backtest);
    let backtest_res = valid_backtest.unwrap();
    assert_eq!(backtest_res["ok"], true);
    assert!(!backtest_res["runId"].as_str().unwrap().is_empty());
    assert_eq!(backtest_res["scriptHash"].as_str().unwrap().len(), 16);
    assert!(backtest_res.get("saveRecommendation").is_some());
}

#[tokio::test]
async fn test_research_backtest_data_readiness_and_sync_lifecycle() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (ports, executor, _dir) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    // 1. Data sufficient: starts immediately
    let ok_res = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Ready\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00700",
                "startTime": "2026-01-01T00:00:00Z",
                "endTime": "2026-01-02T00:00:00Z",
                "waitForCompletionMs": 0,
            }),
        )
        .expect("data ready must start immediately");
    assert_eq!(ok_res["ok"], true);
    assert!(ok_res.get("runId").is_some());

    // 2. Data missing: triggers sync task
    let sync_res = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Missing\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00001",
                "startTime": "2026-01-01T00:00:00Z",
                "endTime": "2026-01-02T00:00:00Z",
                "waitForCompletionMs": 0,
            }),
        )
        .expect("missing data triggers sync");
    assert_eq!(sync_res["ok"], true);
    assert_eq!(sync_res["status"], "syncing_data");
    assert_eq!(sync_res["nextAction"], "wait_kline_sync");
    assert_eq!(sync_res["nextTool"]["name"], "backtest.kline_sync_status");
    assert_eq!(sync_res["nextTool"]["input"]["waitForCompletionMs"], 25000);
    let task_id = sync_res["dataSync"]["taskId"].as_str().expect("task id");
    assert!(!task_id.is_empty());
    assert_eq!(sync_res["nextTool"]["input"]["taskId"], task_id);
    assert_eq!(sync_res["dataSync"]["symbol"], "HK.00001");
    assert_eq!(sync_res["dataSync"]["sessionScope"], "regular");
    assert_eq!(sync_res["dataSync"]["rehabType"], "forward");
    assert!(sync_res["dataSync"].get("since").is_some());
    assert!(sync_res["dataSync"].get("until").is_some());

    // 3. Reuses active sync task without redundant sync trigger
    let reuse_res = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Missing\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00001",
                "startTime": "2026-01-01T00:00:00Z",
                "endTime": "2026-01-02T00:00:00Z",
                "waitForCompletionMs": 0,
            }),
        )
        .expect("reuse active task");
    assert_eq!(reuse_res["ok"], true);
    assert_eq!(reuse_res["status"], "syncing_data");
    assert_eq!(reuse_res["nextAction"], "wait_kline_sync");
    assert_eq!(reuse_res["nextTool"]["name"], "backtest.kline_sync_status");
    assert_eq!(reuse_res["dataSync"]["taskId"], task_id);

    // 4. Execution model failure maps to error
    let fail_res = executor.execute(
        "strategy.research_backtest",
        &json!({
            "script": "//@version=6\nstrategy(\"InvalidModel\", overlay=true)\nplot(close)\n",
            "market": "HK",
            "symbol": "HK.00700",
            "executionModel": "unsupported-model-xyz",
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-02T00:00:00Z",
            "waitForCompletionMs": 0,
        }),
    );
    assert!(fail_res.is_err());

    // 5. Warmup span expands since boundary
    use crate::product::product_research_backtest_execution::derive_effective_since_time;
    let base = "2026-06-01T00:00:00Z";
    assert_eq!(derive_effective_since_time(base, "1d", 0), base);
    assert!(derive_effective_since_time(base, "1d", 10).as_str() < base);

    // 6. Terminal state tracking prevents infinite retries
    use crate::product::product_research_backtest_readiness::{build_sync_key, SyncStateTracker};
    let tracker = SyncStateTracker::global();
    let provider = sync_res["dataSync"]["marketDataProvider"]
        .as_str()
        .unwrap_or("futu");
    let since_str = derive_effective_since_time("2026-01-01T00:00:00Z", "1m", 0);
    let sync_key = build_sync_key(
        provider,
        "HK.00001",
        "1m",
        &since_str,
        "2026-01-02T00:00:00Z",
        "forward",
        "regular",
    );
    tracker.set_terminal(
        sync_key,
        "sync-failed-123".to_owned(),
        "failed".to_owned(),
        "network timeout".to_owned(),
    );
    let terminal_res = executor.execute(
        "strategy.research_backtest",
        &json!({
            "script": "//@version=6\nstrategy(\"Missing\", overlay=true)\nplot(close)\n",
            "market": "HK",
            "symbol": "HK.00001",
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-02T00:00:00Z",
            "waitForCompletionMs": 0,
        }),
    );
    assert!(terminal_res.is_err());
    let err_msg = terminal_res.unwrap_err().to_string();
    assert!(err_msg.contains("terminated with failed: network timeout"));
}

#[tokio::test]
async fn test_portfolio_broker_settings_states_fail_closed() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (ports, executor, bundle_dir) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));
    let settings_path = bundle_dir.path().join("settings.json");

    // State 1: Unconfigured settings ({}) -> brokerEnabled is false, no errors
    let unconfigured = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts unconfigured");
    assert_eq!(unconfigured["brokerEnabled"], false);
    assert_eq!(unconfigured["partial"], false);
    assert!(unconfigured["warnings"].as_array().unwrap().is_empty());

    // State 2: Normally configured settings -> brokerEnabled is true, accounts populated
    let valid_settings = json!({
        "integration": {
            "enabled": true,
            "brokerId": "futu",
            "updatedAt": "2026-08-19T00:00:00Z"
        },
        "accounts": [
            {
                "accountId": "1001",
                "name": "Main",
                "market": "HK",
                "trdEnv": "REAL",
                "enabled": true
            }
        ]
    });
    std::fs::write(&settings_path, serde_json::to_vec(&valid_settings).unwrap())
        .expect("write valid settings");

    let configured = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts configured");
    assert_eq!(configured["brokerEnabled"], true);
    assert_eq!(configured["partial"], false);
    assert!(configured["warnings"].as_array().unwrap().is_empty());
    assert_eq!(configured["managedAccounts"].as_array().unwrap().len(), 1);

    // State 3: Corrupted settings -> fail closed: brokerEnabled false, partial true, warning present
    std::fs::write(&settings_path, b"{corrupted_invalid_json: true").expect("write corrupt settings");

    let corrupted = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts corrupted");
    assert_eq!(corrupted["brokerEnabled"], false);
    assert_eq!(corrupted["partial"], true);
    let warnings = corrupted["warnings"].as_array().unwrap();
    assert!(!warnings.is_empty());
    assert!(
        warnings[0]
            .as_str()
            .unwrap()
            .contains("failed to load broker settings")
    );
}

use crate::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};

fn setup_test_adk_mutation_port(
    chat_runtime: Option<Arc<dyn AdkChatStreamPort>>,
) -> (Arc<ProductionAdkPort>, Arc<AdkStore>, tempfile::TempDir) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let artifact_path = directory.path().join("adk-artifact.db");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let conn = rusqlite::Connection::open(path).expect("create database");
        jftrade_store_sqlite::initialize_current(&conn, component).expect("initialize schema");
    }

    let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));

    let port = Arc::new(ProductionAdkPort {
        store: Arc::clone(&store),
        session_store,
        artifact_store,
        tool_catalog,
        settings_path,
        chat_runtime,
    });
    (port, store, directory)
}

#[test]
fn test_adk_respond_to_input_multithreaded_cas_race_same_answers() {
    #[derive(Debug)]
    struct MockResumeRuntime;
    impl AdkChatStreamPort for MockResumeRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            Ok(())
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let (port, store, _dir) = setup_test_adk_mutation_port(Some(Arc::new(MockResumeRuntime)));
    let run_id = "run-multithread-same";
    let request_id = "req-multithread-same";
    let payload = json!({
        "id": run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": request_id,
            "status": "PENDING",
            "questions": [{"id": "q1", "options": [{"id": "opt-1"}, {"id": "opt-2"}]}]
        },
        "toolCalls": [{"id": "tc1", "name": "interaction.request_user", "status": "RUNNING"}]
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-same",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-same",
            request_fingerprint: "fingerprint-same",
            payload_json: &payload.to_string(),
        })
        .expect("create run");

    let barrier = Arc::new(std::sync::Barrier::new(2));

    let t1 = {
        let port = Arc::clone(&port);
        let barrier = Arc::clone(&barrier);
        let mut ident = BTreeMap::new();
        ident.insert("runId".to_owned(), run_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: ident,
            body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-1"}]}),
            webhook_secret: None,
        };
        std::thread::spawn(move || {
            barrier.wait();
            port.mutate(&input)
        })
    };

    let t2 = {
        let port = Arc::clone(&port);
        let barrier = Arc::clone(&barrier);
        let mut ident = BTreeMap::new();
        ident.insert("runId".to_owned(), run_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: ident,
            body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-1"}]}),
            webhook_secret: None,
        };
        std::thread::spawn(move || {
            barrier.wait();
            port.mutate(&input)
        })
    };

    let res1 = t1.join().expect("thread 1 join");
    let res2 = t2.join().expect("thread 2 join");

    assert!(res1.is_ok(), "t1 result: {:?}", res1);
    assert!(res2.is_ok(), "t2 result: {:?}", res2);

    let run = store.get_run(run_id).unwrap().unwrap();
    assert_eq!(run.status, "RUNNING");
    let run_payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(run_payload["inputRequest"]["status"], "ANSWERED");
    assert_eq!(
        run_payload["inputResponse"]["answers"][0]["optionId"],
        "opt-1"
    );
}

#[test]
fn test_adk_respond_to_input_multithreaded_cas_race_conflicting_answers() {
    #[derive(Debug)]
    struct MockResumeRuntime;
    impl AdkChatStreamPort for MockResumeRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            Ok(())
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let (port, store, _dir) = setup_test_adk_mutation_port(Some(Arc::new(MockResumeRuntime)));
    let run_id = "run-multithread-conflict";
    let request_id = "req-multithread-conflict";
    let payload = json!({
        "id": run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": request_id,
            "status": "PENDING",
            "questions": [{"id": "q1", "options": [{"id": "opt-A"}, {"id": "opt-B"}]}]
        },
        "toolCalls": [{"id": "tc1", "name": "interaction.request_user", "status": "RUNNING"}]
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-conflict",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-conflict",
            request_fingerprint: "fingerprint-conflict",
            payload_json: &payload.to_string(),
        })
        .expect("create run");

    let barrier = Arc::new(std::sync::Barrier::new(2));

    let t1 = {
        let port = Arc::clone(&port);
        let barrier = Arc::clone(&barrier);
        let mut ident = BTreeMap::new();
        ident.insert("runId".to_owned(), run_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: ident,
            body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-A"}]}),
            webhook_secret: None,
        };
        std::thread::spawn(move || {
            barrier.wait();
            port.mutate(&input)
        })
    };

    let t2 = {
        let port = Arc::clone(&port);
        let barrier = Arc::clone(&barrier);
        let mut ident = BTreeMap::new();
        ident.insert("runId".to_owned(), run_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: ident,
            body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-B"}]}),
            webhook_secret: None,
        };
        std::thread::spawn(move || {
            barrier.wait();
            port.mutate(&input)
        })
    };

    let res1 = t1.join().expect("thread 1 join");
    let res2 = t2.join().expect("thread 2 join");

    let (ok_count, err_count) = match (&res1, &res2) {
        (Ok(_), Err(e)) => {
            assert!(format!("{e}").contains("CONFLICT") || format!("{e}").contains("409"));
            (1, 1)
        }
        (Err(e), Ok(_)) => {
            assert!(format!("{e}").contains("CONFLICT") || format!("{e}").contains("409"));
            (1, 1)
        }
        _ => panic!(
            "Expected one winner and one conflict loser, got {:?} and {:?}",
            res1, res2
        ),
    };
    assert_eq!(ok_count, 1);
    assert_eq!(err_count, 1);

    let run = store.get_run(run_id).unwrap().unwrap();
    assert_eq!(run.status, "RUNNING");
}

#[test]
fn test_adk_respond_to_input_resume_failure_error_propagation_and_recovery() {
    use std::sync::atomic::{AtomicBool, Ordering};

    #[derive(Debug)]
    struct ControllableResumeRuntime {
        fail_resume: AtomicBool,
    }

    impl AdkChatStreamPort for ControllableResumeRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            if self.fail_resume.load(Ordering::SeqCst) {
                Err(AdkChatPortError::Unavailable(
                    "engine worker pool unavailable".to_owned(),
                ))
            } else {
                Ok(())
            }
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let mock_runtime = Arc::new(ControllableResumeRuntime {
        fail_resume: AtomicBool::new(true),
    });

    let (port, store, _dir) = setup_test_adk_mutation_port(Some(
        Arc::clone(&mock_runtime) as Arc<dyn AdkChatStreamPort>
    ));
    let run_id = "run-fail-recovery";
    let request_id = "req-fail-recovery";
    let payload = json!({
        "id": run_id,
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": request_id,
            "status": "PENDING",
            "questions": [{"id": "q1", "options": [{"id": "opt-1"}]}]
        },
        "toolCalls": [{"id": "tc1", "name": "interaction.request_user", "status": "RUNNING"}]
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-fail",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-fail",
            request_fingerprint: "fingerprint-fail",
            payload_json: &payload.to_string(),
        })
        .expect("create run");

    let mut ident = BTreeMap::new();
    ident.insert("runId".to_owned(), run_id.to_owned());
    let input = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: ident,
        body: json!({"requestId": request_id, "answers": [{"questionId": "q1", "optionId": "opt-1"}]}),
        webhook_secret: None,
    };

    // 1. First submission fails due to runtime resume failure -> returns 503
    let err1 = port
        .mutate(&input)
        .expect_err("first resume must fail with 503");
    assert!(format!("{err1}").contains("ADK_CONTINUATION_UNAVAILABLE"));

    // Verify DB state: status remains RUNNING, resumeState is input_resume_pending
    let run = store.get_run(run_id).unwrap().unwrap();
    assert_eq!(run.status, "RUNNING");
    let payload_val: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(payload_val["resumeState"], "input_resume_pending");
    assert!(payload_val.get("inputResumeCheckpoint").is_some());

    // 2. Retry while runtime still fails -> error is NOT swallowed, returns 503
    let err2 = port
        .mutate(&input)
        .expect_err("retrying with failing runtime must return 503");
    assert!(format!("{err2}").contains("ADK_CONTINUATION_UNAVAILABLE"));

    // 3. Runtime recovers -> retry succeeds with 200 OK
    mock_runtime.fail_resume.store(false, Ordering::SeqCst);
    let ok_res = port
        .mutate(&input)
        .expect("retrying with recovered runtime must succeed");
    assert_eq!(ok_res["run"]["status"], "RUNNING");
}

#[tokio::test]
async fn test_portfolio_market_isolation_and_suffix_resolution() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (ports, executor, _dir) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    // 1. Market isolation: AdkTestTradeReadPort account 42 has authorities [1, 2] (HK, US)
    let hk_res = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "market": "HK"}),
        )
        .expect("portfolio.accounts for HK");
    assert_eq!(hk_res["selection"]["status"], "resolved");
    assert_eq!(hk_res["selection"]["selectedAccountIds"][0], "42");
    assert_eq!(hk_res["discoveredAccounts"][0]["accountId"], "42");
    assert_eq!(hk_res["selection"]["market"], "HK");

    let us_res = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "market": "US"}),
        )
        .expect("portfolio.accounts for US");
    assert_eq!(us_res["selection"]["status"], "resolved");
    assert_eq!(us_res["selection"]["selectedAccountIds"][0], "42");
    assert_eq!(us_res["discoveredAccounts"][0]["accountId"], "42");
    assert_eq!(us_res["selection"]["market"], "US");

    let cn_res = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "market": "CN"}),
        )
        .expect("portfolio.accounts for CN");
    assert_eq!(cn_res["selection"]["status"], "not_found");
    assert_eq!(cn_res["accounts"], json!([]));
    assert_eq!(cn_res["partial"], true);
    assert!(!cn_res["warnings"].as_array().unwrap().is_empty());

    // 2. Unique suffix matching: accountId "2" matches "42" uniquely
    let suffix_res = executor
        .execute(
            "portfolio.accounts",
            &json!({"tradingEnvironment": "REAL", "accountId": "2"}),
        )
        .expect("portfolio.accounts with suffix");
    assert_eq!(suffix_res["selection"]["status"], "resolved");
    assert_eq!(suffix_res["selection"]["mode"], "unique_suffix");
    assert_eq!(suffix_res["selection"]["selectedAccountIds"][0], "42");

    // 3. Overview and positions adhere to market isolation
    let cn_overview = executor
        .execute(
            "portfolio.overview",
            &json!({"tradingEnvironment": "REAL", "market": "CN"}),
        )
        .expect("portfolio.overview for CN");
    assert_eq!(cn_overview["selection"]["status"], "not_found");
    assert_eq!(cn_overview["accountOverviews"], json!([]));
    assert_eq!(cn_overview["partial"], true);

    let cn_positions = executor
        .execute(
            "portfolio.positions",
            &json!({"tradingEnvironment": "REAL", "market": "CN"}),
        )
        .expect("portfolio.positions for CN");
    assert_eq!(cn_positions["selection"]["status"], "not_found");
    assert_eq!(cn_positions["accountPositions"], json!([]));
    assert_eq!(cn_positions["partial"], true);
}

#[test]
fn test_strategy_research_backtest_schema_properties() {
    let schema = crate::product::product_mcp_protocol::schema_for(
        "strategy.research_backtest",
    );
    let props = schema
        .get("properties")
        .and_then(Value::as_object)
        .expect("properties object");

    let expected_fields = [
        "script",
        "market",
        "symbol",
        "code",
        "interval",
        "instrumentType",
        "startDate",
        "endDate",
        "startTime",
        "endTime",
        "initialBalance",
        "chartType",
        "rehabType",
        "useExtendedHours",
        "tradingCosts",
        "executionModel",
        "marketDataProvider",
        "waitForCompletionMs",
        "resultView",
    ];
    for field in &expected_fields {
        assert!(props.contains_key(*field), "missing field in schema: {field}");
    }

    let required = schema["required"].as_array().expect("required array");
    assert!(required.contains(&json!("script")));
    assert!(required.contains(&json!("market")));
    assert!(required.contains(&json!("startTime")));
    assert!(required.contains(&json!("endTime")));

    let costs = props.get("tradingCosts").unwrap();
    let cost_props = costs["properties"]
        .as_object()
        .expect("tradingCosts properties");
    assert!(cost_props.contains_key("brokerFees"));
    assert!(cost_props.contains_key("marketFees"));

    let provider_enum = props["marketDataProvider"]["enum"]
        .as_array()
        .expect("provider enum");
    assert!(provider_enum.iter().any(|v| v == "futu"));
    assert!(provider_enum.iter().any(|v| v == "yfinance"));
    assert!(provider_enum.iter().any(|v| v == "akshare"));

    let inst_enum = props["instrumentType"]["enum"]
        .as_array()
        .expect("instrumentType enum");
    assert!(inst_enum.iter().any(|v| v == "stock"));
    assert!(inst_enum.iter().any(|v| v == "etf"));
}

#[test]
fn test_research_backtest_result_view_projection() {
    use crate::product::product_research_backtest_projection::project_result_view;

    // 1. Real CorpusOutput structure with cases[]
    let real_corpus_payload = json!({
        "id": "run-test-real-corpus",
        "status": "completed",
        "marketDataProvider": "longport",
        "chartType": "candlestick",
        "instrumentType": "stock",
        "useExtendedHours": true,
        "executionModel": "bar_close",
        "tradingCosts": {"brokerFees": {"feeSchedule": "fixed"}},
        "result": {
            "executionModel": "bar_close",
            "cases": [
                {
                    "finalEquity": "105000.0",
                    "realizedPnl": "5000.0",
                    "cash": "95000.0",
                    "maxDrawdown": "0.035",
                    "currentDrawdown": "0.01",
                    "totalTrades": 2,
                    "winRate": "0.50",
                    "totalFees": "15.0",
                    "processedBars": 150,
                    "warnings": ["slippage estimated"],
                    "orders": [
                        {"id": "o1", "time": 1000, "symbol": "US.TSLA", "side": "BUY", "quantity": 10, "status": "FILLED"},
                        {"id": "o2", "time": 2000, "symbol": "US.TSLA", "side": "SELL", "quantity": 10, "status": "FILLED"}
                    ],
                    "fills": [
                        {"orderId": "o1", "symbol": "US.TSLA", "side": "BUY", "price": 10.0, "quantity": 10, "time": 1000},
                        {"orderId": "o2", "symbol": "US.TSLA", "side": "SELL", "price": 12.0, "quantity": 10, "time": 2000}
                    ],
                    "equityCurve": [
                        {"time": 1000, "equity": 100000.0},
                        {"time": 2000, "equity": 105000.0}
                    ],
                    "drawdownCurve": [
                        {"time": 1000, "drawdown": 0.0},
                        {"time": 2000, "drawdown": 0.02}
                    ]
                }
            ]
        },
        "logs": [
            {"timestamp": 1000, "level": "INFO", "message": "start"},
            {"timestamp": 2000, "level": "WARN", "message": "caution"}
        ]
    });

    let default_options = json!({});

    // Summary view projections from real CorpusOutput cases[0]
    let summary_view = project_result_view(&real_corpus_payload, Some(&default_options));
    assert_eq!(summary_view["run"]["marketDataProvider"], "longport");
    assert_eq!(summary_view["summary"]["finalEquity"], "105000.0");
    assert_eq!(summary_view["summary"]["realizedPnl"], "5000.0");
    assert_eq!(summary_view["summary"]["totalTrades"], 2);
    assert_eq!(summary_view["summary"]["winRate"], "0.50");
    assert!(summary_view["summary"]["warnings"].as_array().unwrap().iter().any(|w| w == "slippage estimated"));

    // Orders view with limit 1
    let orders_options = json!({"view": "orders", "limit": 1});
    let orders_view = project_result_view(&real_corpus_payload, Some(&orders_options));
    assert_eq!(orders_view["series"]["orderBook"].as_array().unwrap().len(), 1);

    // Chart view includes pnlCurve, drawdownCurve, and trades derived from fills
    let chart_options = json!({"view": "chart"});
    let chart_view = project_result_view(&real_corpus_payload, Some(&chart_options));
    assert_eq!(chart_view["series"]["trades"].as_array().unwrap().len(), 2);
    assert_eq!(chart_view["series"]["pnlCurve"].as_array().unwrap().len(), 2);
    assert_eq!(chart_view["series"]["drawdownCurve"].as_array().unwrap().len(), 2);

    // 2. Legacy run payload compatibility
    let legacy_run_payload = json!({
        "id": "run-test-proj",
        "status": "completed",
        "candles": [
            {"time": 1000, "open": 10.0, "high": 11.0, "low": 9.5, "close": 10.5, "volume": 100},
            {"time": 2000, "open": 10.5, "high": 12.0, "low": 10.0, "close": 11.5, "volume": 200}
        ],
        "trades": [
            {"time": 1000, "id": "t1", "action": "BUY", "price": 10.0, "quantity": 10}
        ],
        "equity": [
            {"time": 1000, "equity": 10000.0, "drawdown": 0.0}
        ],
        "orders": [
            {"id": "o1", "time": 1000, "symbol": "US.TSLA", "status": "FILLED"}
        ],
        "logs": [
            {"timestamp": 1000, "level": "INFO", "message": "start"}
        ],
        "marketDataProvider": "longport"
    });

    let legacy_chart = project_result_view(&legacy_run_payload, Some(&chart_options));
    assert_eq!(legacy_chart["series"]["candles"].as_array().unwrap().len(), 2);
    assert_eq!(legacy_chart["series"]["trades"].as_array().unwrap().len(), 1);
}

#[test]
fn test_adk_resume_approval_cas_rejection() {
    let bundle_dir = tempfile::tempdir().expect("tempdir");
    let settings_path = bundle_dir.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    let adk_path = bundle_dir.path().join("adk.db");
    let session_path = bundle_dir.path().join("adk-session.db");
    for (path, comp) in [(&adk_path, "adk"), (&session_path, "adk-session")] {
        let conn = rusqlite::Connection::open(path).expect("open db");
        jftrade_store_sqlite::initialize_current(&conn, comp).expect("init db");
    }
    let store = Arc::new(
        jftrade_store_sqlite::AdkStore::open(&adk_path).expect("open adk store"),
    );
    let session_store = Arc::new(
        jftrade_store_sqlite::AdkSessionStore::open(&session_path).expect("open session store"),
    );
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog = Arc::new(
        ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"),
    );
    let cancellation_registry = Arc::new(
        crate::product::product_adk_model_runtime::RunCancellationRegistry::default(),
    );
    let runtime = crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::clone(&cancellation_registry),
        Arc::clone(&tool_catalog),
    );

    let run_id = "run-cas-reject-test";
    let payload = json!({
        "id": run_id,
        "status": "PENDING_INPUT",
        "resumeState": "input_resume_pending",
        "inputRequest": {
            "id": "req-1",
            "status": "ANSWERED",
            "answers": [{"questionId": "q1", "optionId": "opt-1"}]
        }
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-cas",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-cas",
            request_fingerprint: "fingerprint-cas",
            payload_json: &payload.to_string(),
        })
        .expect("create run");

    // Concurrently mutate the run status in SQLite so CAS fails
    let conn = rusqlite::Connection::open(&adk_path).expect("open raw sqlite");
    conn.execute(
        "UPDATE adk_runs SET status = 'CANCELLED', updated_at = '2099-01-01T00:00:00Z' WHERE id = ?1",
        rusqlite::params![run_id],
    )
    .expect("concurrent update");
    drop(conn);

    // resume_approval must detect the status transition and NOT spawn continuation
    let res = runtime.resume_approval(run_id);
    assert!(res.is_ok(), "cancelled run is treated as ok without continuation");

    // Now test with status changed to RUNNING concurrently
    let run_id2 = "run-cas-running-test";
    let payload2 = json!({
        "id": run_id2,
        "status": "PENDING_INPUT",
        "resumeState": "other_state"
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id2,
            session_id: "session-cas2",
            agent_id: "agent-1",
            status: "PENDING_INPUT",
            client_request_id: "client-cas2",
            request_fingerprint: "fingerprint-cas2",
            payload_json: &payload2.to_string(),
        })
        .expect("create run 2");

    let res2 = runtime.resume_approval(run_id2);
    assert!(res2.is_err(), "non-resumable state must be rejected");
    match res2.unwrap_err() {
        crate::product::product_adk_chat_stream_port::AdkChatPortError::Unavailable(msg) => {
            assert!(msg.contains("already PENDING_INPUT"));
        }
        err => panic!("unexpected error variant: {:?}", err),
    }
}

struct AdkTestPortfolioFundsReadPort;

impl jftrade_integration_futu::TradeReadPort for AdkTestPortfolioFundsReadPort {
    fn read_accounts(
        &self,
        _: u64,
        market: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeAccountSnapshot>, jftrade_integration_futu::TradeSessionError> {
        let auth = match market {
            Some(m) => vec![m],
            None => vec![1, 2],
        };
        Ok(vec![
            jftrade_integration_futu::TradeAccountSnapshot {
                trd_env: 1,
                acc_id: 102,
                trd_market_auth_list: auth.clone(),
                acc_type: Some(2),
                card_num: None,
                security_firm: Some(1),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: Some(0),
                acc_role: Some(1),
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            },
            jftrade_integration_futu::TradeAccountSnapshot {
                trd_env: 1,
                acc_id: 100,
                trd_market_auth_list: auth.clone(),
                acc_type: Some(2),
                card_num: None,
                security_firm: Some(1),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: Some(0),
                acc_role: Some(1),
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            },
            jftrade_integration_futu::TradeAccountSnapshot {
                trd_env: 1,
                acc_id: 101,
                trd_market_auth_list: auth,
                acc_type: Some(2),
                card_num: None,
                security_firm: Some(1),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: Some(0),
                acc_role: Some(1),
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            },
        ])
    }

    fn read_funds(
        &self,
        header: jftrade_integration_futu::TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<jftrade_integration_futu::TradeFundsSnapshot, jftrade_integration_futu::TradeSessionError> {
        let cash = match header.acc_id {
            100 => 50000.0,
            101 => 10000.0,
            _ => 0.0,
        };
        Ok(jftrade_integration_futu::TradeFundsSnapshot {
            header,
            funds: jftrade_integration_futu::TradeFunds {
                power: cash,
                total_assets: cash,
                cash,
                market_val: 0.0,
                frozen_cash: 0.0,
                debt_cash: 0.0,
                avl_withdrawal_cash: cash,
                currency: Some(1),
                available_funds: Some(cash),
                unrealized_pl: None,
                realized_pl: None,
                risk_level: None,
                initial_margin: None,
                maintenance_margin: None,
                cash_info_list: Vec::new(),
                max_power_short: None,
                net_cash_power: None,
                long_mv: None,
                short_mv: None,
                pending_asset: None,
                max_withdrawal: None,
                risk_status: None,
                margin_call_margin: None,
                is_pdt: None,
                pdt_seq: None,
                beginning_dtbp: None,
                remaining_dtbp: None,
                dt_call_amount: None,
                dt_status: None,
                securities_assets: None,
                fund_assets: None,
                bond_assets: None,
                market_info_list: Vec::new(),
                crypto_mv: None,
                exposure_level: None,
                exposure_limit: None,
                used_limit: None,
                remaining_limit: None,
            },
        })
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<jftrade_integration_futu::TradeCashFlowSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("cash flows unsupported".into()))
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("fees unsupported".into()))
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("margin ratios unsupported".into()))
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<jftrade_integration_futu::TradeMaxTradeQuantitySnapshot, jftrade_integration_futu::TradeSessionError> {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported("quantity unsupported".into()))
    }

    fn read_positions(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradePositionSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeFillSnapshot>, jftrade_integration_futu::TradeSessionError> {
        Ok(Vec::new())
    }
}

#[tokio::test]
async fn test_portfolio_funds_overview_and_sorting_and_unsupported_market() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let bundle_dir = tempfile::tempdir().expect("bundle directory");
    let settings_path = bundle_dir.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    crate::product::product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");

    let settings = Arc::new(
        jftrade_store_settings_file::SettingsFileStore::open(&settings_path)
            .expect("settings store"),
    );
    let security = crate::product::SecuritySettingsService::new(settings);
    let active = Arc::new(crate::product::ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let runtime = Arc::new(
        crate::product::product_production_ports::SharedTradeReadRuntime::default(),
    );
    let mut config = crate::product::ProductConfig::new(
        "127.0.0.1:0".parse().expect("bind address"),
        &settings_path,
        crate::product::AccessPolicy::default(),
    )
    .expect("product config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime)
    .with_trade_read_port(Some(Arc::new(AdkTestPortfolioFundsReadPort)), Some(true))
    .with_market_data_runtime_status_port(Arc::new(AdkTestReadyRuntimeStatus))
    .with_backtest_execution_port(Arc::new(AdkTestBacktestExecution));
    config.capabilities = crate::product::ProductCapabilities::all();
    config.production = true;
    let ports = Arc::new(
        crate::product::product_production_ports::production_ports(&config, &security)
            .expect("production ports"),
    );

    let executor = crate::product::product_adk_model_runtime::ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(&ports),
    );

    // 1. Explicit unsupported market returns error
    let invalid_mkt_res = executor.execute(
        "portfolio.overview",
        &json!({"tradingEnvironment": "REAL", "market": "INVALID_MKT"}),
    );
    assert!(invalid_mkt_res.is_err());
    assert!(invalid_mkt_res.unwrap_err().to_string().contains("unsupported market"));

    // 2. Overview correctly detects funds and applies Go baseline stable sort
    let overview_res = executor
        .execute(
            "portfolio.overview",
            &json!({"tradingEnvironment": "REAL", "market": "HK"}),
        )
        .expect("portfolio.overview execution");

    assert_eq!(overview_res["selection"]["status"], "resolved");
    let overviews = overview_res["accountOverviews"].as_array().unwrap();
    assert_eq!(overviews.len(), 3);

    // Account 100: cash 50000 -> hasAssetsOrPositions = true
    assert_eq!(overviews[0]["account"]["accountId"], "100");
    assert_eq!(overviews[0]["hasAssetsOrPositions"], true);

    // Account 101: cash 10000 -> hasAssetsOrPositions = true
    assert_eq!(overviews[1]["account"]["accountId"], "101");
    assert_eq!(overviews[1]["hasAssetsOrPositions"], true);

    // Account 102: cash 0 -> hasAssetsOrPositions = false (sorted to end)
    assert_eq!(overviews[2]["account"]["accountId"], "102");
    assert_eq!(overviews[2]["hasAssetsOrPositions"], false);
}
