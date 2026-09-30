use super::*;
use crate::product::product_adk_mutation_port::AdkMutationPortError;
use crate::product::{ProductConfig, start_product};
use std::net::SocketAddr;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

async fn request_json_with_status(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&str>,
) -> (u16, Value) {
    let body = body.unwrap_or_default();
    let mut stream = TcpStream::connect(address).await.expect("connect product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    stream.write_all(request.as_bytes()).await.expect("write request");
    let mut response = Vec::new();
    stream.read_to_end(&mut response).await.expect("read response");
    let response = String::from_utf8(response).expect("UTF-8 response");
    let (headers, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let status = headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    (status, serde_json::from_str(body).expect("JSON response"))
}

#[path = "product_adk_store_parity_tests.rs"]
mod store_parity;

#[path = "product_adk_workflow_tool_error_tests.rs"]
mod workflow_tool_error;

#[path = "product_adk_input_response_parity_tests.rs"]
mod input_response_parity;

#[path = "product_portfolio_parity_tests.rs"]
mod portfolio_parity;

impl ProductionToolCatalog {
    /// Test-only catalog seeded with explicit descriptor rows, so policy
    /// regressions can use a confirmation-gated tool without depending on a
    /// production adapter being installed.
    pub(crate) fn from_tool_rows(rows: Vec<Value>) -> Self {
        Self {
            tools: rows,
            bindings: BTreeMap::new(),
            research_bindings: BTreeMap::new(),
            active_provider_state: None,
            trade_runtime: None,
            backtest_execution_ready: false,
            pine_readiness: None,
        }
    }
}

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
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("complete tool bindings"));
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

/// Build a production ADK port whose chat runtime passes the readiness gate so
/// request-level business errors reach the model runtime instead of the
/// fail-closed `503 ADK_UNAVAILABLE` placeholder path.
///
/// The single stored provider is callable (keyed) but no agent points at it, so
/// an agent-less request exercises Go's `DefaultProvider` fallback: Go's
/// `StoreCore.ListProviders` repairs the missing `default` flag and
/// `DefaultProvider` returns the first provider.  Its endpoint is a closed
/// loopback port, so the fallback resolves provider settings and fails in the
/// model call instead of being rejected as "not configured".
fn ready_adk_port_with_fallback_provider() -> (Arc<ProductionAdkPort>, tempfile::TempDir) {
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
        let connection = rusqlite::Connection::open(path).expect("create ADK database");
        jftrade_store_sqlite::initialize_current(&connection, component)
            .expect("initialize ADK schema");
    }
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        br#"{"provider-ready":"test-key"}"#,
    )
    .expect("write provider secret");
    let adk_store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
    let session_store =
        Arc::new(AdkSessionStore::open(&session_path).expect("open adk session store"));
    let artifact_store =
        Arc::new(AdkArtifactStore::open(&artifact_path).expect("open adk artifact store"));
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("complete tool bindings"));
    // A configured provider is what makes the runtime ready; no agent points at
    // it, so the chat request itself still has to resolve an agent.
    // A closed loopback port keeps the fallback deterministic: the request
    // reaches the model call and is refused immediately, with no external
    // network dependency and no chance of a stray listener answering.
    let closed_port = std::net::TcpListener::bind("127.0.0.1:0")
        .expect("bind a loopback port")
        .local_addr()
        .expect("read the bound address")
        .port();
    adk_store
        .upsert_provider(
            "provider-ready",
            &json!({
                "displayName": "Ready Provider",
                "baseUrl": format!("http://127.0.0.1:{closed_port}/v1"),
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    let runtime = crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
        Arc::clone(&adk_store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(crate::product::product_adk_model_runtime::RunCancellationRegistry::default()),
        Arc::clone(&tool_catalog),
    );
    assert!(runtime.runtime_ready(), "fixture runtime must be ready");
    let port = ProductionAdkPort {
        store: adk_store,
        session_store,
        artifact_store,
        tool_catalog,
        settings_path,
        chat_runtime: Some(runtime),
    };
    (Arc::new(port), directory)
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

/// Parity: go:452dea11:internal/assistant/engine/tools_test.go:25
/// `TestToolRegistrySerializesEmptyApprovalModesAsArray`.
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
    for tool in catalog.values() {
        for field in [
            "name",
            "displayName",
            "description",
            "category",
            "permission",
        ] {
            assert!(
                tool[field].as_str().is_some_and(|value| !value.is_empty()),
                "{field}: {tool}"
            );
        }
        assert!(tool["allowedModes"].is_array());
        assert!(tool["requiresApprovalIn"].is_array());
        // Parity: internal/assistant/engine/tools_test.go:25 TestToolRegistrySerializesEmptyApprovalModesAsArray
        if tool["id"] == "workflow.wait" || tool["id"] == "system.status" {
            assert_eq!(tool["requiresApprovalIn"], json!([]));
        }
    }
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
fn research_catalog_does_not_fall_back_to_shared_adapter_readiness() {
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let research = BTreeMap::from([("instrument", ProductionAdapterBinding::Ready)]);
    let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
        .expect("catalog bindings");

    for id in [
        "research.financials",
        "research.analyst",
        "research.ownership",
        "research.corporate_actions",
        "research.rankings",
        "research.industry",
        "research.calendar",
        "research.macro",
    ] {
        let tool = catalog
            .tools
            .iter()
            .find(|tool| tool["id"] == id)
            .expect("research tool");
        assert_eq!(tool["allowedModes"], json!([]), "{id} must fail closed");
    }
}

#[test]
fn provider_tool_catalog_requires_concrete_futu_readers() {
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let research = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .filter_map(|definition| definition.research_operation)
        .map(|operation| (operation, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
        .expect("catalog bindings")
        .with_active_provider_state(state);

    let allowed_modes = |id: &str| {
        catalog
            .values()
            .into_iter()
            .find(|tool| tool["id"] == id)
            .and_then(|tool| tool["allowedModes"].as_array().cloned())
            .expect("tool descriptor")
    };

    for id in [
        "derivatives.futures",
        "derivatives.option_chain",
        "derivatives.option_analysis",
        "derivatives.option_events",
        "derivatives.option_screen",
        "alerts.price.list",
        "alerts.option_event.list",
        "watchlist.remote.list",
    ] {
        assert!(allowed_modes(id).is_empty(), "{id} must require its reader");
    }
}

#[test]
fn helper_research_catalog_matches_provider_operation_support() {
    use jftrade_settings::MarketDataProviderRuntimePort;

    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let research = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .filter_map(|definition| definition.research_operation)
        .map(|operation| (operation, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
        .expect("catalog bindings")
        .with_active_provider_state(Arc::clone(&state));
    let allowed_modes = |id: &str| {
        catalog
            .values()
            .into_iter()
            .find(|tool| tool["id"] == id)
            .and_then(|tool| tool["allowedModes"].as_array().cloned())
            .is_some_and(|modes| !modes.is_empty())
    };

    assert!(allowed_modes("research.rankings"));
    assert!(allowed_modes("research.analyst"));
    assert!(allowed_modes("research.ownership"));
    assert!(!allowed_modes("research.industry"));
    assert!(!allowed_modes("research.calendar"));
    assert!(!allowed_modes("research.macro"));

    state
        .activate(jftrade_settings::MarketDataProvider::Akshare)
        .expect("provider activation");
    assert!(allowed_modes("research.rankings"));
    assert!(allowed_modes("research.industry"));
    assert!(allowed_modes("research.calendar"));
    assert!(allowed_modes("research.macro"));
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

    let cancellation_registry =
        Arc::new(crate::product::product_adk_model_runtime::RunCancellationRegistry::default());
    let adk_chat_runtime = crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
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

    let tool_results = payload["toolResults"]
        .as_array()
        .expect("toolResults array");
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

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:449
/// `TestADKStrategyToolContractsCoverUnavailableAndSuccessfulViewScenarios`
/// (unavailable half). Go's registry without the backtest dependencies answers
/// "unavailable" for `strategy.research_backtest`, `backtest.result_view`, and
/// `backtest.kline_sync_status` instead of a fabricated payload. The Rust
/// owner is `ProductionAdkToolExecutor` before the production port bundle is
/// attached.
#[test]
fn detached_adk_tool_executor_reports_domain_tools_as_unavailable() {
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
    for name in [
        "strategy.research_backtest",
        "strategy.optimize",
        "portfolio.accounts",
        "portfolio.overview",
        "portfolio.positions",
        "backtest.kline_sync_status",
        "backtest.result_view",
    ] {
        assert!(
            !executor.supports(name),
            "{name} needs the production ports"
        );
        let error = executor
            .execute(name, &json!({"taskId": "sync-1", "runId": "run-1"}))
            .expect_err("a detached executor must not fabricate a payload");
        assert!(
            error.contains("unavailable"),
            "{name} error must report unavailable, got {error:?}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:449 TestResolveRunInputIsValidatedAndIdempotent
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
    let artifact_store = Arc::new(
        jftrade_store_sqlite::AdkArtifactStore::open(&artifact_path).expect("artifact store"),
    );
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
    let cancellation_registry =
        Arc::new(crate::product::product_adk_model_runtime::RunCancellationRegistry::default());
    let runtime = crate::product::product_adk_model_runtime::ProductionAdkChatRuntime::new(
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
    let err = port
        .mutate(&input)
        .expect_err("partial answers must be rejected");
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
    let err = port
        .mutate(&input)
        .expect_err("invalid option must be rejected");
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
    let err = port
        .mutate(&input)
        .expect_err("label as optionId must be rejected");
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
    let err = port
        .mutate(&input)
        .expect_err("otherText must be rejected when allowOther is false");
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
    let retry_result = port
        .mutate(&input)
        .expect("identical answers must be idempotent 200 OK");
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
    let err = port
        .mutate(&input)
        .expect_err("different answers must return 409 conflict");
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
    let artifact_store = Arc::new(AdkArtifactStore::open(&artifact_path).expect("artifact store"));
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
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
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
        json!(["script", "market"])
    );
    assert!(research_backtest_tool["parameters"]["anyOf"].is_array());

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
    ) -> Result<
        Vec<jftrade_integration_futu::TradeAccountSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
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
    ) -> Result<
        jftrade_integration_futu::TradeFundsSnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "funds unsupported".into(),
        ))
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeCashFlowSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "cash flows unsupported".into(),
        ))
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "fees unsupported".into(),
        ))
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "margin ratios unsupported".into(),
        ))
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<
        jftrade_integration_futu::TradeMaxTradeQuantitySnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "quantity unsupported".into(),
        ))
    }

    fn read_combo_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeComboMaxTradeQuantityRequest,
    ) -> Result<
        jftrade_integration_futu::TradeComboMaxTradeQuantitySnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "combo unsupported".into(),
        ))
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
    ) -> Result<
        Vec<jftrade_integration_futu::TradePositionSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Ok(Vec::new())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Ok(Vec::new())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeFillSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
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
    ) -> Result<
        jftrade_integration_futu::HistoricalKlineResult,
        jftrade_integration_futu::HistoricalKlineError,
    > {
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

fn setup_test_bundle_and_executor() -> (
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
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
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
        .execute(
            "portfolio.positions",
            &json!({"tradingEnvironment": "REAL"}),
        )
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
    assert!(
        valid_backtest.is_ok(),
        "research backtest start failed: {:?}",
        valid_backtest
    );
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
    use crate::product::product_research_backtest_readiness::{SyncStateTracker, build_sync_key};
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

    // State 3: Corrupted settings -> fail closed across all 3 tools: brokerEnabled false, partial true, warning present
    std::fs::write(&settings_path, b"{corrupted_invalid_json: true")
        .expect("write corrupt settings");

    let corrupted_acc = executor
        .execute("portfolio.accounts", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.accounts corrupted");
    assert_eq!(corrupted_acc["brokerEnabled"], false);
    assert_eq!(corrupted_acc["partial"], true);
    assert!(
        corrupted_acc["warnings"]
            .as_array()
            .unwrap()
            .iter()
            .any(|w| w
                .as_str()
                .unwrap_or("")
                .contains("failed to load broker settings"))
    );

    let corrupted_ovw = executor
        .execute("portfolio.overview", &json!({"tradingEnvironment": "REAL"}))
        .expect("portfolio.overview corrupted");
    assert_eq!(corrupted_ovw["brokerEnabled"], false);
    assert_eq!(corrupted_ovw["partial"], true);
    assert!(
        corrupted_ovw["warnings"]
            .as_array()
            .unwrap()
            .iter()
            .any(|w| w
                .as_str()
                .unwrap_or("")
                .contains("failed to load broker settings"))
    );

    let corrupted_pos = executor
        .execute(
            "portfolio.positions",
            &json!({"tradingEnvironment": "REAL"}),
        )
        .expect("portfolio.positions corrupted");
    assert_eq!(corrupted_pos["brokerEnabled"], false);
    assert_eq!(corrupted_pos["partial"], true);
    assert!(
        corrupted_pos["warnings"]
            .as_array()
            .unwrap()
            .iter()
            .any(|w| w
                .as_str()
                .unwrap_or("")
                .contains("failed to load broker settings"))
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
    let artifact_store = Arc::new(AdkArtifactStore::open(&artifact_path).expect("artifact store"));
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

    let (port, store, _dir) =
        setup_test_adk_mutation_port(Some(Arc::clone(&mock_runtime) as Arc<dyn AdkChatStreamPort>));
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
    let schema = crate::product::product_mcp_protocol::schema_for("strategy.research_backtest");
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
        assert!(
            props.contains_key(*field),
            "missing field in schema: {field}"
        );
    }

    let required = schema["required"].as_array().expect("required array");
    assert!(required.contains(&json!("script")));
    assert!(required.contains(&json!("market")));
    assert_eq!(required.len(), 2);

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

// Parity: go:452dea11:internal/backtest/result_view_test.go:12 TestResultViewRunPayloadPreservesProviderAndExecutionMetadata
#[test]
fn test_research_backtest_result_view_projection() {
    use crate::product::product_research_backtest_projection::project_result_view;

    // 1. Real CorpusOutput structure with cases[]
    let real_corpus_payload = json!({
        "id": "run-test-real-corpus",
        "status": "completed",
        "marketDataProvider": "longport",
        "request": {
            "chartType": "candlestick",
            "instrumentType": "stock",
            "useExtendedHours": true,
            "executionModel": "bar_close",
            "tradingCosts": {"brokerFees": {"feeSchedule": "fixed"}}
        },
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
    assert_eq!(summary_view["run"]["chartType"], "candlestick");
    assert_eq!(summary_view["run"]["instrumentType"], "stock");
    assert_eq!(summary_view["run"]["executionModel"], "bar_close");
    assert_eq!(summary_view["run"]["useExtendedHours"], true);
    assert_eq!(summary_view["run"]["tradingCosts"]["brokerFees"]["feeSchedule"], "fixed");
    assert_eq!(summary_view["summary"]["finalEquity"], "105000.0");
    assert_eq!(summary_view["summary"]["realizedPnl"], "5000.0");
    assert_eq!(summary_view["summary"]["totalTrades"], 2);
    assert_eq!(summary_view["summary"]["winRate"], "0.50");
    assert!(
        summary_view["summary"]["warnings"]
            .as_array()
            .unwrap()
            .iter()
            .any(|w| w == "slippage estimated")
    );

    // Orders view with limit 1
    let orders_options = json!({"view": "orders", "limit": 1});
    let orders_view = project_result_view(&real_corpus_payload, Some(&orders_options));
    assert_eq!(
        orders_view["series"]["orderBook"].as_array().unwrap().len(),
        1
    );

    // Chart view includes pnlCurve, drawdownCurve, and trades derived from fills
    let chart_options = json!({"view": "chart"});
    let chart_view = project_result_view(&real_corpus_payload, Some(&chart_options));
    assert_eq!(chart_view["series"]["trades"].as_array().unwrap().len(), 2);
    assert_eq!(
        chart_view["series"]["pnlCurve"].as_array().unwrap().len(),
        2
    );
    assert_eq!(
        chart_view["series"]["drawdownCurve"]
            .as_array()
            .unwrap()
            .len(),
        2
    );

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
    assert_eq!(
        legacy_chart["series"]["candles"].as_array().unwrap().len(),
        2
    );
    assert_eq!(
        legacy_chart["series"]["trades"].as_array().unwrap().len(),
        1
    );
}

/// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:207 TestResolvedApprovalDoesNotStealForeignExecutionLease
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
    let store = Arc::new(jftrade_store_sqlite::AdkStore::open(&adk_path).expect("open adk store"));
    let session_store = Arc::new(
        jftrade_store_sqlite::AdkSessionStore::open(&session_path).expect("open session store"),
    );
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let tool_catalog =
        Arc::new(ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings"));
    let cancellation_registry =
        Arc::new(crate::product::product_adk_model_runtime::RunCancellationRegistry::default());
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
    assert!(
        res.is_ok(),
        "cancelled run is treated as ok without continuation"
    );

    // A run that is no longer resumable is Go's silent no-op, not a failure:
    // `continueResolvedInput` returns nil unless the run is RUNNING with an
    // answered request, and `continueResolvedApprovalRun` returns nil unless
    // `runCanContinueResolvedApproval` holds.  The approval/input routes race
    // the durable recovery scanner for the same continuation, so the caller
    // that arrives late must not turn the winner's state into a fabricated
    // error.
    let run_id2 = "run-non-resumable-input-test";
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

    runtime
        .resume_approval(run_id2)
        .expect("a non-resumable run must be a silent no-op");

    // The no-op must not mutate the durable state either.
    let untouched = store
        .get_run(run_id2)
        .expect("read run 2")
        .expect("run 2 exists");
    assert_eq!(untouched.status, "PENDING_INPUT");
    let untouched_payload: Value =
        serde_json::from_str(&untouched.payload_json).expect("decode run 2 payload");
    assert_eq!(untouched_payload["resumeState"], "other_state");
}

struct AdkTestPortfolioFundsReadPort;

impl jftrade_integration_futu::TradeReadPort for AdkTestPortfolioFundsReadPort {
    fn read_accounts(
        &self,
        _: u64,
        market: Option<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeAccountSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
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
    ) -> Result<
        jftrade_integration_futu::TradeFundsSnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
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
    ) -> Result<
        Vec<jftrade_integration_futu::TradeCashFlowSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "cash flows unsupported".into(),
        ))
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "fees unsupported".into(),
        ))
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "margin ratios unsupported".into(),
        ))
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<
        jftrade_integration_futu::TradeMaxTradeQuantitySnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(jftrade_integration_futu::TradeSessionError::Unsupported(
            "quantity unsupported".into(),
        ))
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
    ) -> Result<
        Vec<jftrade_integration_futu::TradePositionSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Ok(Vec::new())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Ok(Vec::new())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeFillSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
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
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
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
    assert!(
        invalid_mkt_res
            .unwrap_err()
            .to_string()
            .contains("unsupported market")
    );

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

/// Build an ADK port whose on-disk provider/skill state matches the Go agent
/// validation fixture: one enabled keyed provider, one disabled provider, one
/// enabled provider without a key, and one external skill directory.
fn agent_validation_port() -> (ProductionAdkPort, tempfile::TempDir) {
    let (port, directory) = unready_adk_port();
    std::fs::write(&port.settings_path, b"{}").expect("write settings");
    let secrets = directory.path().join("secrets");
    std::fs::create_dir_all(&secrets).expect("create secrets directory");
    std::fs::write(
        secrets.join("adk-secrets.json"),
        br#"{"provider-enabled":"sk-fixture"}"#,
    )
    .expect("write adk secrets");
    port.store
        .upsert_provider(
            "provider-enabled",
            &json!({
                "displayName": "Enabled Provider",
                "baseUrl": "https://api.example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist enabled provider");
    port.store
        .upsert_provider(
            "provider-disabled",
            &json!({
                "displayName": "Disabled Provider",
                "baseUrl": "https://api.example.test/v1",
                "model": "fixture-model",
                "enabled": false,
            })
            .to_string(),
        )
        .expect("persist disabled provider");
    port.store
        .upsert_provider(
            "provider-no-key",
            &json!({
                "displayName": "No Key Provider",
                "baseUrl": "https://api.example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist keyless provider");
    (port, directory)
}

fn create_agent_error(port: &ProductionAdkPort, body: Value) -> AdkMutationPortError {
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body,
        webhook_secret: None,
    })
    .expect_err("agent write must be rejected")
}

fn assert_bad_request(error: AdkMutationPortError, expected: &str) {
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 400, "status for {message:?}");
            // Go's agent handler classifies every `isADKAgentValidationError`
            // failure as `400 BAD_REQUEST` carrying the service message.
            assert_eq!(code, "BAD_REQUEST", "code for {message:?}");
            assert_eq!(message, expected);
        }
        other => panic!("expected a failed mutation, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_persistence_test.go:96 TestGoalWorkflowFailsWhenIterationLimitPauseCannotBePersisted
/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:634 TestADKAgentSaveValidationFailures
/// Parity: go:452dea11:internal/assistant/service_business_test.go:12 TestServiceSaveAgentValidationScenarios
/// TestADKAgentSaveValidationFailures
///
/// The agent write contract classifies provider lifecycle, unknown catalogue
/// membership and vocabulary failures as `400 BAD_REQUEST` carrying the
/// service message. Every branch below is reachable from POST
/// /api/v1/adk/agents in production.
#[test]
fn adk_agent_write_reports_the_go_validation_messages() {
    let (port, _directory) = agent_validation_port();

    for (expected, body) in [
        (
            "invalid agent status",
            json!({"id": "agent-invalid-status", "name": "Agent", "status": "BROKEN"}),
        ),
        (
            "invalid agent work mode",
            json!({"id": "agent-invalid-mode", "name": "Agent", "workMode": "parallel"}),
        ),
        (
            "invalid tool access mode",
            json!({"id": "agent-invalid-access", "name": "Agent", "toolAccessMode": "some"}),
        ),
        (
            "loop max iterations must be between 1 and 20",
            json!({"id": "agent-invalid-loop", "name": "Agent", "loopMaxIterations": 21}),
        ),
        (
            "loop max iterations must be between 1 and 20",
            json!({"id": "agent-negative-loop", "name": "Agent", "loopMaxIterations": -1}),
        ),
        (
            "invalid agent payload",
            json!({"id": "agent-fractional-loop", "name": "Agent", "loopMaxIterations": 1.5}),
        ),
        (
            "invalid agent payload",
            json!({"id": "agent-string-loop", "name": "Agent", "loopMaxIterations": "5"}),
        ),
        (
            "provider not found",
            json!({
                "id": "agent-missing-provider",
                "name": "Agent",
                "providerId": "provider-missing",
            }),
        ),
        (
            "provider is disabled",
            json!({
                "id": "agent-disabled-provider",
                "name": "Agent",
                "providerId": "provider-disabled",
            }),
        ),
        (
            "provider API keys is not configured",
            json!({
                "id": "agent-no-key",
                "name": "Agent",
                "providerId": "provider-no-key",
            }),
        ),
        (
            "unknown ADK tool: tool.does_not_exist",
            json!({
                "id": "agent-bad-tool",
                "name": "Agent",
                "tools": ["tool.does_not_exist"],
            }),
        ),
        (
            "unknown ADK skill: skill-does-not-exist",
            json!({
                "id": "agent-bad-skill",
                "name": "Agent",
                "skills": ["skill-does-not-exist"],
            }),
        ),
    ] {
        assert_bad_request(create_agent_error(&port, body), expected);
    }

    // A disabled agent keeps its provider reference even when that provider is
    // disabled or has no key, and an enabled agent with a keyed provider saves.
    for body in [
        json!({
            "id": "agent-disabled-ok",
            "name": "Disabled OK",
            "status": "DISABLED",
            "providerId": "provider-disabled",
        }),
        json!({
            "id": "agent-no-key-disabled-ok",
            "name": "No Key Disabled OK",
            "status": "DISABLED",
            "providerId": "provider-no-key",
        }),
        json!({
            "id": "agent-enabled-ok",
            "name": "Enabled OK",
            "status": "ENABLED",
            "providerId": "provider-enabled",
            "workMode": "loop",
            "loopMaxIterations": 1,
        }),
    ] {
        let saved = port
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::CreateAgent,
                identifiers: BTreeMap::new(),
                body: body.clone(),
                webhook_secret: None,
            })
            .unwrap_or_else(|error| panic!("agent {body} must save: {error:?}"));
        assert_eq!(saved["id"], body["id"]);
    }
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:741 TestADKBindAgentWithPreinstalledNeodataFinancialSearch
/// TestADKBindAgentWithPreinstalledNeodataFinancialSearch
///
/// A preinstalled external skill is addressable by the agent write path, so a
/// valid binding saves and the stored agent keeps the skill reference. The
/// builtin projection supplies the remaining catalogue entries.
#[test]
fn adk_agent_write_accepts_preinstalled_external_and_builtin_skills() {
    let (port, _directory) = agent_validation_port();
    port.store
        .upsert_skill(
            "neodata-financial-search",
            &json!({
                "id": "neodata-financial-search",
                "displayName": "NeoData Financial Search",
                "source": "https://example.test/neodata.zip",
                "enabled": true,
                "builtin": false,
                "validationStatus": "VALID",
            })
            .to_string(),
        )
        .expect("persist installed skill");

    let saved = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "agent-neodata",
                "name": "Agent NeoData",
                "status": "ENABLED",
                "tools": ["research.instrument"],
                "skills": ["neodata-financial-search", "jftrade-market"],
            }),
            webhook_secret: None,
        })
        .expect("preinstalled and builtin skills must be bindable");
    assert_eq!(
        saved["skills"],
        json!(["neodata-financial-search", "jftrade-market"])
    );
}

/// Parity: go:452dea11:internal/assistant/service_business_test.go:12 TestServiceSaveAgentValidationScenarios
///
/// Update goes through the same validation owner, so a PUT that introduces an
/// unknown tool is rejected instead of persisting an unusable agent. The
/// create-path scenarios of the same reference test are covered by
/// `adk_agent_write_reports_the_go_validation_messages`.
#[test]
fn adk_agent_update_revalidates_the_merged_payload() {
    let (port, _directory) = agent_validation_port();
    let saved = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "agent-update",
                "name": "Agent Update",
                "providerId": "provider-enabled",
            }),
            webhook_secret: None,
        })
        .expect("create agent");
    assert_eq!(saved["providerId"], "provider-enabled");

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateAgent,
            identifiers: BTreeMap::from([("agentId".to_owned(), "agent-update".to_owned())]),
            body: json!({"tools": ["tool.does_not_exist"]}),
            webhook_secret: None,
        })
        .expect_err("unknown tool on update");
    assert_bad_request(error, "unknown ADK tool: tool.does_not_exist");

    // The rejected update is not persisted: the stored agent keeps its
    // provider reference and no partial tool list.
    let stored = port
        .store
        .get_agent("agent-update")
        .expect("read agent")
        .expect("agent row");
    let payload: Value =
        serde_json::from_str(&stored.payload_json).expect("decode stored agent payload");
    assert_eq!(payload["providerId"], "provider-enabled");
    assert!(payload.get("tools").is_none());
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_product_catalog_test.go:13
/// TestDefaultBuiltinAgentToolsExistInAssembledRegistry
///
/// Go walks `DefaultBuiltinToolNames()` and fails when the assembled registry
/// has no entry for one of them. The Rust builtin default template keeps its
/// membership bypass only while the supplied list equals the composed catalog,
/// so every id the default agent references has to resolve through the same
/// validation owner that rejects unknown tools.
#[test]
fn builtin_default_agent_tools_all_resolve_in_the_assembled_catalog() {
    let (port, _directory) = agent_validation_port();
    let assembled = port.tool_catalog.ids();
    assert!(
        !assembled.is_empty(),
        "the assembled catalog must expose at least one tool"
    );

    let default_agent = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "jftrade-default",
                "name": "JFTrade Default",
                "status": "ENABLED",
                "tools": assembled.clone(),
            }),
            webhook_secret: None,
        })
        .expect("the builtin default template must only reference assembled tools");
    assert_eq!(
        default_agent["tools"].as_array().map(Vec::len),
        Some(assembled.len()),
        "the builtin default keeps every assembled tool: {default_agent}"
    );

    let resolved = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "agent-catalog-probe",
                "name": "Catalog Probe",
                "status": "ENABLED",
                "tools": assembled.clone(),
            }),
            webhook_secret: None,
        })
        .expect("every assembled tool id must resolve without the builtin bypass");
    assert_eq!(
        resolved["tools"].as_array().map(Vec::len),
        Some(assembled.len())
    );

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "agent-unknown-tool",
                "name": "Unknown Tool",
                "status": "ENABLED",
                "tools": ["not.an.assembled.tool"],
            }),
            webhook_secret: None,
        })
        .expect_err("a tool outside the assembled registry must be rejected");
    assert_bad_request(error, "unknown ADK tool: not.an.assembled.tool");
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:707 TestADKSkillInstallAndUninstallFailureRoutes
/// TestADKSkillInstallAndUninstallFailureRoutes
///
/// Install failures are reported as `400 ADK_SKILL_INSTALL_FAILED` with the
/// registry message, and uninstalling a builtin skill is
/// `500 ADK_SKILL_UNINSTALL_FAILED`. Both codes are owned by the skill
/// transport rather than the generic mutation failure.
#[test]
fn adk_skill_install_and_uninstall_failures_keep_the_go_codes() {
    let (port, _directory) = agent_validation_port();

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::InstallSkill,
            identifiers: BTreeMap::new(),
            body: json!({"url": "not-a-valid-url"}),
            webhook_secret: None,
        })
        .expect_err("invalid skill URL");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_SKILL_INSTALL_FAILED");
            assert_eq!(message, "valid http/https skill URL is required");
        }
        other => panic!("expected a failed install, got {other:?}"),
    }

    // The builtin skill list is projected rather than stored, so the uninstall
    // path must resolve it from the projection and refuse removal.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteSkill,
            identifiers: BTreeMap::from([("skillId".to_owned(), "jftrade-market".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("builtin skill uninstall");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 500);
            assert_eq!(code, "ADK_SKILL_UNINSTALL_FAILED");
            assert!(
                message.to_lowercase().contains("builtin"),
                "uninstall message {message:?} must name the builtin protection"
            );
        }
        other => panic!("expected a failed uninstall, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:711
/// TestInternalSkillCannotBeUninstalled.
///
/// The builtin skill refuses uninstall with the reference message and stays
/// visible in the catalog projection afterwards.
#[test]
fn builtin_skill_uninstall_is_refused_and_the_projection_keeps_it() {
    let (port, _directory) = agent_validation_port();
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteSkill,
            identifiers: BTreeMap::from([("skillId".to_owned(), "jftrade-market".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("builtin skill uninstall");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 500);
            assert_eq!(code, "ADK_SKILL_UNINSTALL_FAILED");
            assert!(
                message.to_lowercase().contains("builtin"),
                "uninstall message {message:?} must name the builtin protection"
            );
        }
        other => panic!("expected a failed uninstall, got {other:?}"),
    }
    let AdkReadSnapshot::Json(listed) = port.read("/api/v1/adk/skills", "").expect("skills read")
    else {
        panic!("skills route must answer JSON");
    };
    assert!(
        listed["skills"]
            .as_array()
            .expect("skills array")
            .iter()
            .any(|skill| skill["id"] == "jftrade-market" && skill["source"] == "builtin"),
        "the builtin skill stays registered: {listed}"
    );
}

/// Parity: go:452dea11:internal/api/assistant/routes_error_contracts_test.go:14 TestAssistantRoutesRejectInvalidQueriesPayloadsAndMissingResources
/// TestAssistantRoutesRejectInvalidQueriesPayloadsAndMissingResources.
///
/// Go classifies the query/payload/missing-resource table by route: a malformed
/// pagination value is `400 BAD_REQUEST`, an unknown task status is
/// `400 ADK_TASK_LIST_FAILED`, a missing task/memory delete is
/// `404 ADK_TASK_NOT_FOUND` / `404 ADK_MEMORY_NOT_FOUND`, and every malformed
/// mutation payload stays `400 BAD_REQUEST`. The task-status row was previously
/// collapsed into the generic code, so this test pins the route code.
#[test]
fn adk_read_and_mutation_routes_keep_the_go_error_classification() {
    let (port, _directory) = unready_adk_port();

    for (path, query, code, message) in [
        (
            "/api/v1/adk/tasks",
            "limit=oops",
            "BAD_REQUEST",
            "invalid tasks query",
        ),
        (
            "/api/v1/adk/tasks",
            "status=NOT_A_STATUS",
            "ADK_TASK_LIST_FAILED",
            r#"invalid task status "NOT_A_STATUS""#,
        ),
        (
            "/api/v1/adk/sessions",
            "limit=oops",
            "BAD_REQUEST",
            "invalid sessions query",
        ),
        (
            "/api/v1/adk/runs",
            "limit=oops",
            "BAD_REQUEST",
            "invalid runs query",
        ),
        (
            "/api/v1/adk/approvals",
            "limit=oops",
            "BAD_REQUEST",
            "invalid approvals query",
        ),
    ] {
        let failure = crate::product::dispatch_adk_read(Some(&port), "GET", path, query)
            .expect_err("an invalid query must fail closed");
        assert_eq!(failure.status, 400, "path {path}?{query}");
        assert_eq!(failure.code, code, "path {path}?{query}");
        assert_eq!(failure.message, message, "path {path}?{query}");
    }

    // A missing task keeps the task-specific 404 instead of a generic code.
    let failure =
        crate::product::dispatch_adk_read(Some(&port), "GET", "/api/v1/adk/tasks/task-missing", "")
            .expect_err("a missing task must fail closed");
    assert_eq!(failure.status, 404);
    assert_eq!(failure.code, "ADK_TASK_NOT_FOUND");
    assert_eq!(failure.message, "task not found");

    // Deleting a missing memory keeps the memory-specific 404.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteMemory,
            identifiers: BTreeMap::from([("memoryId".to_owned(), "memory-missing".to_owned())]),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("a missing memory delete must fail closed");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_MEMORY_NOT_FOUND");
            assert_eq!(message, "memory not found");
        }
        other => panic!("expected 404 ADK_MEMORY_NOT_FOUND, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_error_contracts_test.go:98 TestAssistantRoutesEnforceBusinessValidationOnUpdates
/// TestAssistantRoutesEnforceBusinessValidationOnUpdates.
///
/// Go keeps a dedicated route code per update resource: a blank session title is
/// `400 ADK_SESSION_RENAME_FAILED`, an unsupported composer work mode is
/// `400 ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED`, and a blank objective is
/// `400 ADK_RUN_OBJECTIVE_UPDATE_FAILED`. Both session branches previously
/// collapsed into the generic `400 BAD_REQUEST`.
#[test]
fn adk_session_and_run_update_routes_keep_the_go_business_error_codes() {
    let (port, _directory) = unready_adk_port();
    port.store
        .upsert_session(
            "session-1",
            "agent-1",
            r#"{"id":"session-1","agentId":"agent-1","title":"t"}"#,
        )
        .expect("seed session");
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-objective",
            session_id: "session-1",
            agent_id: "agent-1",
            status: "RUNNING",
            client_request_id: "request-objective",
            request_fingerprint: "fingerprint-objective",
            payload_json: r#"{"id":"run-objective","sessionId":"session-1","agentId":"agent-1","status":"RUNNING","workMode":"loop","workflowStatus":"RUNNING","objective":"keep"}"#,
        })
        .expect("seed goal run");

    let cases = [
        (
            AdkMutationOperation::RenameSession,
            "sessionId",
            "session-1",
            json!({"title": "   "}),
            "ADK_SESSION_RENAME_FAILED",
            "session title is required",
        ),
        (
            AdkMutationOperation::UpdateSessionComposerState,
            "sessionId",
            "session-1",
            json!({"workModeOverride": "parallel"}),
            "ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED",
            "invalid composer state payload",
        ),
        (
            AdkMutationOperation::UpdateSessionComposerState,
            "sessionId",
            "session-1",
            json!({"workModeOverride": "task"}),
            "ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED",
            "invalid composer state payload",
        ),
        (
            AdkMutationOperation::UpdateRunObjective,
            "runId",
            "run-objective",
            json!({"objective": "   "}),
            "ADK_RUN_OBJECTIVE_UPDATE_FAILED",
            "objective is required",
        ),
    ];
    for (operation, id_field, id, body, code, message) in cases {
        let error = port
            .mutate(&AdkMutationInput {
                operation,
                identifiers: BTreeMap::from([(id_field.to_owned(), id.to_owned())]),
                body,
                webhook_secret: None,
            })
            .expect_err("the business rule must reject the update");
        match error {
            AdkMutationPortError::Failed {
                status,
                code: actual_code,
                message: actual_message,
            } => {
                assert_eq!(status, 400, "{operation:?}");
                assert_eq!(actual_code, code, "{operation:?}");
                assert_eq!(actual_message, message, "{operation:?}");
            }
            other => panic!("expected a 400 failure for {operation:?}, got {other:?}"),
        }
    }
}

/// Parity: go:452dea11:internal/api/assistant/workflow_routes_test.go:185 TestWorkflowRoutesClassifyInvalidPayloadsAndUnavailableRuns
/// TestWorkflowRoutesClassifyInvalidPayloadsAndUnavailableRuns and
/// internal/api/assistant/adk_workflow_routes_test.go:262
/// TestADKWorkflowRoutesRejectInvalidInputs.
/// Parity: go:452dea11:internal/api/assistant/adk_workflow_routes_test.go:262 TestADKWorkflowRoutesRejectInvalidInputs
///
/// Go read handlers keep the route's own error code on a missing resource
/// (`ADK_TASK_NOT_FOUND`, `ADK_WORKFLOW_GET_FAILED`,
/// `ADK_WORKFLOW_TRIGGER_LIST_FAILED`, `ADK_SESSION_CONTEXT_FAILED`) instead
/// of a generic `NOT_FOUND`.  Those resources have no Rust projection owner
/// yet, so the production read port must not collapse them into `NOT_FOUND`.
#[test]
fn adk_read_resource_misses_keep_the_go_route_error_codes() {
    let (port, _directory) = unready_adk_port();
    for (path, code, message) in [
        (
            "/api/v1/adk/tasks/missing-task",
            "ADK_TASK_NOT_FOUND",
            "task not found",
        ),
        (
            "/api/v1/adk/workflows/missing-workflow",
            "ADK_WORKFLOW_GET_FAILED",
            "workflow not found",
        ),
        (
            "/api/v1/adk/workflows/missing-workflow/triggers",
            "ADK_WORKFLOW_TRIGGER_LIST_FAILED",
            "workflow not found",
        ),
        (
            "/api/v1/adk/sessions/missing-session/context",
            "ADK_SESSION_CONTEXT_FAILED",
            "session not found",
        ),
    ] {
        match port.read(path, "") {
            Err(AdkReadSnapshotError::Failed {
                status,
                code: actual_code,
                message: actual_message,
                ..
            }) => {
                assert_eq!(status, 404, "path {path}");
                assert_eq!(actual_code, code, "path {path}");
                assert_eq!(actual_message, message, "path {path}");
            }
            other => panic!("expected a {code} failure for {path}, got {other:?}"),
        }
    }
}

/// Parity: go:452dea11:internal/api/assistant/adk_approval_test.go:282 TestADKProviderDeleteRejectsReferencedProvider
/// TestADKProviderDeleteRejectsReferencedProvider
///
/// Go's store wraps `ErrProviderInUse` with the referencing agent name and
/// `handleADKDeleteProvider` reports it as `409 ADK_PROVIDER_DELETE_FAILED`.
/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:17
/// `TestDeleteProviderFailsWhenReferencedByAgent`.
#[test]
fn adk_provider_delete_reports_the_in_use_agent_and_keeps_the_go_projection() {
    let (port, _directory) = agent_validation_port();
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body: json!({
            "id": "agent-holding-provider",
            "name": "Holding Agent",
            "providerId": "provider-enabled",
        }),
        webhook_secret: None,
    })
    .expect("create referencing agent");

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteProvider,
            identifiers: BTreeMap::from([("providerId".to_owned(), "provider-enabled".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("a referenced provider must not be deleted");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_PROVIDER_DELETE_FAILED");
            assert!(
                message.contains("used by agent") && message.contains("Holding Agent"),
                "provider-in-use message {message:?} must name the referencing agent"
            );
        }
        other => panic!("expected 409 ADK_PROVIDER_DELETE_FAILED, got {other:?}"),
    }

    // The rejected delete is not durable: the provider row survives.
    assert!(
        port.store
            .get_provider("provider-enabled")
            .expect("read provider")
            .is_some(),
        "the referenced provider must remain persisted"
    );
}

/// Go's store-level provider delete is idempotent: `DeleteProvider` on an
/// unknown id returns nil and the route still answers
/// `200 {"deleted":true,"id":...}`. The same response drops the removed
/// `replacementProviderId` field, which the Go handler never emits.
#[test]
fn adk_provider_delete_is_idempotent_and_matches_the_go_success_envelope() {
    let (port, _directory) = agent_validation_port();

    let missing = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteProvider,
            identifiers: BTreeMap::from([("providerId".to_owned(), "provider-missing".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("deleting an unknown provider is idempotent in Go");
    assert_eq!(missing, json!({"deleted": true, "id": "provider-missing"}));

    let deleted = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteProvider,
            identifiers: BTreeMap::from([(
                "providerId".to_owned(),
                "provider-disabled".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("delete existing provider");
    assert_eq!(deleted, json!({"deleted": true, "id": "provider-disabled"}));
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:101 TestRunInputResponseContract
/// TestRunInputResponseContract.
///
/// The route contract only needs a single question with `allowOther: true`:
/// an unknown option is `400 ADK_INPUT_RESPONSE_INVALID`, the accepted answer
/// is `200`, and a later different answer is `409 ADK_INPUT_RESPONSE_CONFLICT`.
#[test]
fn adk_run_input_response_route_accepts_then_conflicts() {
    #[derive(Debug)]
    struct ResumeRuntime;
    impl AdkChatStreamPort for ResumeRuntime {
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

    let (port, store, directory) = setup_test_adk_mutation_port(Some(Arc::new(ResumeRuntime)));
    let run_id = "run-input-contract";
    let payload = json!({
        "id": run_id,
        "agentId": "agent-input-contract",
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": "input-contract",
            "status": "PENDING",
            "questions": [{
                "id": "q1",
                "question": "Choose",
                "allowOther": true,
                "options": [
                    {"id": "q1-o1", "label": "A"},
                    {"id": "q1-o2", "label": "B"}
                ]
            }]
        }
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-input-contract",
            agent_id: "agent-input-contract",
            status: "PENDING_INPUT",
            client_request_id: "client-input-contract",
            request_fingerprint: "fingerprint-input-contract",
            payload_json: &payload.to_string(),
        })
        .expect("persist pending-input run");

    let respond = |body: Value| AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
        body,
        webhook_secret: None,
    };

    let error = port
        .mutate(&respond(
            json!({"requestId": "input-contract", "answers": [{"questionId": "q1", "optionId": "missing"}]}),
        ))
        .expect_err("an unknown option id must be rejected");
    match error {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_INPUT_RESPONSE_INVALID");
        }
        other => panic!("expected 400 ADK_INPUT_RESPONSE_INVALID, got {other:?}"),
    }

    let accepted = port
        .mutate(&respond(
            json!({"requestId": "input-contract", "answers": [{"questionId": "q1", "optionId": "q1-o2"}]}),
        ))
        .expect("a valid option must be accepted");
    assert_eq!(accepted["request"]["status"], "ANSWERED");

    let conflict = port
        .mutate(&respond(
            json!({"requestId": "input-contract", "answers": [{"questionId": "q1", "otherText": "different"}]}),
        ))
        .expect_err("a different answer must conflict");
    match conflict {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_INPUT_RESPONSE_CONFLICT");
        }
        other => panic!("expected 409 ADK_INPUT_RESPONSE_CONFLICT, got {other:?}"),
    }
    drop(directory);
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:858 TestADKRunNegativeRoutes
/// TestADKRunNegativeRoutes.
///
/// Go's run handlers use two different 404 shapes: `GET /runs/{runId}` answers
/// the generic `NOT_FOUND` / "run not found", while `POST /runs/{runId}/cancel`
/// wraps every runtime error under `ADK_RUN_CANCEL_FAILED`. The read route keeps
/// that `NOT_FOUND` projection; this test pins the cancel route's dedicated code.
#[test]
fn adk_cancel_run_missing_uses_the_go_cancel_error_code() {
    let (port, _directory) = unready_adk_port();

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CancelRun,
            identifiers: BTreeMap::from([("runId".to_owned(), "run-missing".to_owned())]),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("cancelling a missing run must fail");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_RUN_CANCEL_FAILED");
            assert_eq!(message, "run not found");
        }
        other => panic!("expected 404 ADK_RUN_CANCEL_FAILED, got {other:?}"),
    }

    // The read route keeps the generic notification code for the same run id.
    let read_error = port
        .read("/api/v1/adk/runs/run-missing", "")
        .expect_err("reading a missing run must fail");
    match read_error {
        AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
            assert_eq!(message, "run not found");
        }
        other => panic!("expected 404 NOT_FOUND, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/input_response_test.go:12 TestRunInputResponseErrorAndRetryContracts
/// TestRunInputResponseErrorAndRetryContracts and
/// internal/api/assistant/routes_test.go:101 TestRunInputResponseContract.
///
/// Go's `handleADKInputResponse` maps `InputRequestErrorKind` onto the wire:
/// `invalid` -> `400 ADK_INPUT_RESPONSE_INVALID`, `not_found` -> a plain
/// `404 NOT_FOUND` run error, `conflict` -> `409 ADK_INPUT_RESPONSE_CONFLICT`.
/// A repeated identical submission stays `200` for idempotency.
#[test]
fn adk_respond_to_input_maps_the_go_error_codes_and_retries() {
    #[derive(Debug)]
    struct ResumeRuntime;
    impl AdkChatStreamPort for ResumeRuntime {
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

    let (port, store, directory) = setup_test_adk_mutation_port(Some(Arc::new(ResumeRuntime)));

    // A missing run surfaces the wrapped `ErrInputRequestNotFound` message
    // under the generic code, exactly like Go's handler switch.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: BTreeMap::from([("runId".to_owned(), "missing-run".to_owned())]),
            body: json!({"requestId": "input-errors", "answers": []}),
            webhook_secret: None,
        })
        .expect_err("a missing run must not accept an input response");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
            assert_eq!(message, "input request not found: missing-run");
        }
        other => panic!("expected 404 NOT_FOUND, got {other:?}"),
    }

    // A run whose stored payload has no matching input request is an invalid
    // submission, not a missing resource.
    let run_id = "run-input-response-errors";
    let payload = json!({
        "id": run_id,
        "agentId": "fixture-agent",
        "status": "PENDING_INPUT",
        "inputRequest": {
            "id": "input-errors",
            "status": "PENDING",
            "questions": [{
                "id": "q1",
                "question": "Choose",
                "allowOther": false,
                "options": [
                    {"id": "q1-o1", "label": "A"},
                    {"id": "q1-o2", "label": "B"}
                ]
            }]
        }
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-input-errors",
            agent_id: "fixture-agent",
            status: "PENDING_INPUT",
            client_request_id: "client-input-errors",
            request_fingerprint: "fingerprint-input-errors",
            payload_json: &payload.to_string(),
        })
        .expect("persist pending-input run");

    // Mismatched requestId -> 409 conflict under the input-response code.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
            body: json!({
                "requestId": "other-input",
                "answers": [{"questionId": "q1", "optionId": "q1-o1"}],
            }),
            webhook_secret: None,
        })
        .expect_err("a mismatched request id must conflict");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_INPUT_RESPONSE_CONFLICT");
            assert!(
                message.contains("does not match"),
                "unexpected conflict message {message}"
            );
        }
        other => panic!("expected 409 ADK_INPUT_RESPONSE_CONFLICT, got {other:?}"),
    }

    // An answer the question does not allow -> 400 invalid.
    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RespondToInput,
            identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
            body: json!({
                "requestId": "input-errors",
                "answers": [{"questionId": "q1", "otherText": "custom"}],
            }),
            webhook_secret: None,
        })
        .expect_err("otherText must be rejected when allowOther is false");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_INPUT_RESPONSE_INVALID");
            assert_eq!(message, "q1 does not allow other text");
        }
        other => panic!("expected 400 ADK_INPUT_RESPONSE_INVALID, got {other:?}"),
    }

    // The accepted answer is idempotent on an identical retry.
    let valid = AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
        body: json!({
            "requestId": "input-errors",
            "answers": [{"questionId": "q1", "optionId": "q1-o2"}],
        }),
        webhook_secret: None,
    };
    let first = port.mutate(&valid).expect("valid input response");
    let retry = port.mutate(&valid).expect("identical retry stays 200");
    assert_eq!(first["request"]["status"], "ANSWERED");
    assert_eq!(retry["request"]["status"], "ANSWERED");
    assert_eq!(first, retry, "an identical retry must be idempotent");
    drop(directory);
}
/// An installed external skill is removed together with its install directory,
/// and a second uninstall reports the frozen missing-file projection instead of
/// a synthetic 404.
/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:735
/// `TestExternalSkillUninstallRemovesInstallDir`.
#[test]
fn adk_skill_uninstall_removes_external_installs_and_reports_missing_files() {
    let (port, directory) = agent_validation_port();
    let install_dir = directory.path().join("skills/neodata-financial-search");
    std::fs::create_dir_all(&install_dir).expect("create install directory");
    let skill_document = install_dir.join("SKILL.md");
    std::fs::write(
        &skill_document,
        "---\nname: neodata-financial-search\n---\n",
    )
    .expect("write skill");
    port.store
        .upsert_skill(
            "neodata-financial-search",
            &json!({
                "id": "neodata-financial-search",
                "displayName": "NeoData Financial Search",
                "source": "https://example.test/neodata.zip",
                "installPath": skill_document,
                "enabled": true,
                "builtin": false,
                "validationStatus": "VALID",
            })
            .to_string(),
        )
        .expect("persist installed skill");

    let removed = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteSkill,
            identifiers: BTreeMap::from([(
                "skillId".to_owned(),
                "neodata-financial-search".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("uninstall external skill");
    assert_eq!(removed["deleted"], true);
    assert!(!install_dir.exists(), "install directory must be removed");

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::DeleteSkill,
            identifiers: BTreeMap::from([(
                "skillId".to_owned(),
                "neodata-financial-search".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("second uninstall");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 500);
            assert_eq!(code, "ADK_SKILL_UNINSTALL_FAILED");
            assert_eq!(message, "file does not exist");
        }
        other => panic!("expected a failed uninstall, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:12 TestAssistantChatRoutesRejectMalformedOrUnresolvableRequests
/// TestAssistantChatRoutesRejectMalformedOrUnresolvableRequests and
/// go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:65
/// TestAssistantRoutesClampPaginationBeyondAvailableItems.
///
/// Go's chat handler writes every non-conflict `Service.Chat` error as
/// `400 ADK_CHAT_FAILED` (see `handleADKChat`), so agent resolution, provider
/// resolution, missing credentials and blank messages all share that status and
/// code. A blank message reports `message is required`; an unresolvable agent
/// reports `agent not found`. `BAD_REQUEST` is reserved for a payload the chat
/// wire port cannot decode or whose `clientRequestId` is not a UUID.
#[test]
// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:18 TestPrepareChatRequestValidationAndConcurrency
// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:978 TestResolveAgentCoversDefaultAndProviderValidation
fn adk_chat_route_reports_the_go_error_classification() {
    let (port, _directory) = ready_adk_port_with_fallback_provider();
    port.store
        .upsert_agent(
            "agent-disabled",
            &json!({
                "id": "agent-disabled",
                "name": "Disabled Agent",
                "providerId": "provider-1",
                "status": "DISABLED",
            })
            .to_string(),
        )
        .expect("persist disabled agent");
    port.store
        .upsert_agent(
            "agent-deleted",
            &json!({
                "id": "agent-deleted",
                "name": "Deleted Agent",
                "providerId": "provider-1",
                "status": "DISABLED",
                "deletedAt": "2026-09-19T00:00:00Z",
            })
            .to_string(),
        )
        .expect("persist soft-deleted agent");
    // A legacy row can carry the delete marker while still reading ENABLED.
    // Go reports "agent is deleted" for that shape because the status check
    // passes first.
    port.store
        .upsert_agent(
            "agent-deleted-marker-only",
            &json!({
                "id": "agent-deleted-marker-only",
                "name": "Deleted Marker Agent",
                "providerId": "provider-missing",
                "status": "ENABLED",
                "deletedAt": "2026-09-19T00:00:00Z",
            })
            .to_string(),
        )
        .expect("persist marker-only deleted agent");

    // Every dispatch claims its own `clientRequestId`, exactly like a fresh
    // browser request.  A shared id would be replayed as the same run: once the
    // provider fallback resolves, a repeated id conflicts on the fingerprint
    // before the branch under test is reached.
    let dispatch_counter = std::sync::atomic::AtomicU32::new(1);
    let dispatch_result = |body: &str| {
        let sequence = dispatch_counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        port.dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                body: body.as_bytes().to_vec(),
                client_request_id: format!("1111111{sequence}-1111-4111-8111-111111111111"),
            },
        )
    };
    let dispatch = |body: &str| {
        dispatch_result(body).expect_err("chat must fail closed without a ready provider")
    };
    let failed = |error: AdkChatPortError| match error {
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => (status, code, message),
        other => panic!("expected a failed chat port error, got {other:?}"),
    };

    // A request with no `agentId` falls back to the built-in agent, whose own
    // `providerId` is empty.  Go's `DefaultProvider` then returns the first
    // stored provider even when its `default` flag was never persisted, so the
    // request reaches the model call instead of reporting a missing default.
    // The fixture endpoint is a closed loopback port, so the fallback is
    // observable as a provider-level failure.  Go's `CompleteChatRun` turns
    // that failure into a terminal run plus a synthetic reply, so the dispatch
    // answers with the failed-run projection instead of a port error.
    let projection = match dispatch_result(r#"{"message":"hello"}"#) {
        Ok(AdkChatPortOutput::Json(projection)) => projection,
        other => {
            panic!("the fallback provider failure must project a chat response, got {other:?}")
        }
    };
    assert_eq!(projection["run"]["status"], "FAILED");
    assert_eq!(projection["run"]["errorCode"], "MODEL_CALL_FAILED");
    assert!(
        projection["run"]["failureReason"]
            .as_str()
            .is_some_and(|reason| reason.contains("127.0.0.1")),
        "the resolved provider endpoint must be the fixture fallback: {projection}"
    );
    assert_eq!(
        projection["reply"], projection["run"]["failureReason"],
        "Go replies with `userFacingADKError(adkErr)`"
    );

    assert_eq!(
        failed(dispatch(r#"{"agentId":"agent-1","message":"   "}"#)),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "message is required".to_owned()
        )
    );
    assert_eq!(
        failed(dispatch(r#"{"agentId":"agent-missing","message":"hello"}"#)),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent not found".to_owned()
        )
    );
    // Parity: go:452dea11:internal/api/assistant/routes_test.go:348 TestChatRequestUsesDeclaredMessageFieldOnly
    // TestChatRequestUsesDeclaredMessageFieldOnly.  Go decodes the declared
    // `ADKChatRequest` fields, so the legacy `prompt`/`text` aliases never
    // populate the message: a payload carrying only those aliases is a blank
    // message and is rejected exactly like `{"message":""}`.
    for legacy in [
        r#"{"agentId":"agent-1","prompt":"legacy"}"#,
        r#"{"agentId":"agent-1","text":"legacy-text"}"#,
        r#"{"agentId":"agent-1","prompt":"legacy","text":"legacy-text"}"#,
    ] {
        assert_eq!(
            failed(dispatch(legacy)),
            (
                400,
                "ADK_CHAT_FAILED".to_owned(),
                "message is required".to_owned()
            ),
            "legacy alias payload {legacy} must not populate the declared message field"
        );
    }
    assert_eq!(
        failed(dispatch(
            r#"{"agentId":"agent-disabled","message":"hello"}"#
        )),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent is disabled".to_owned()
        )
    );
    // Go checks the status before the soft-delete marker, so a row that is both
    // disabled and deleted reports "agent is disabled".
    assert_eq!(
        failed(dispatch(r#"{"agentId":"agent-deleted","message":"hello"}"#)),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent is disabled".to_owned()
        )
    );
    assert_eq!(
        failed(dispatch(
            r#"{"agentId":"agent-deleted-marker-only","message":"hello"}"#
        )),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent is deleted".to_owned()
        )
    );
    port.store
        .upsert_provider(
            "provider-disabled",
            &json!({
                "displayName": "Disabled Provider",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": false,
            })
            .to_string(),
        )
        .expect("persist disabled provider");
    port.store
        .upsert_provider(
            "provider-no-key",
            &json!({
                "displayName": "No Key Provider",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist keyless provider");
    for (agent_id, provider_id) in [
        ("agent-disabled-provider", "provider-disabled"),
        ("agent-no-key", "provider-no-key"),
    ] {
        port.store
            .upsert_agent(
                agent_id,
                &json!({
                    "id": agent_id,
                    "name": agent_id,
                    "providerId": provider_id,
                    "status": "ENABLED",
                })
                .to_string(),
            )
            .expect("persist provider-bound agent");
    }
    // Provider resolution keeps Go's two provider messages: a disabled provider
    // and a provider without stored credentials are both request-level chat
    // failures, not infrastructure outages.
    assert_eq!(
        failed(dispatch(
            r#"{"agentId":"agent-disabled-provider","message":"hello"}"#
        )),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent provider is unavailable".to_owned()
        )
    );
    assert_eq!(
        failed(dispatch(r#"{"agentId":"agent-no-key","message":"hello"}"#)),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "agent provider API keys is not configured".to_owned()
        )
    );

    // A message at or below the Go rune limit reaches provider resolution; one
    // rune over the limit is rejected first with the Go wording.
    let at_limit = "x".repeat(50_000);
    assert!(matches!(
        port.dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                body: format!(r#"{{"message":"{at_limit}"}}"#).into_bytes(),
                client_request_id: "22222222-2222-4222-8222-222222222222".to_owned(),
            },
        )
        .expect_err("a full-length message still needs a provider"),
        AdkChatPortError::Failed { .. }
    ));
    assert_eq!(
        failed(dispatch(&format!(
            r#"{{"message":"{}"}}"#,
            "x".repeat(50_001)
        ))),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "message exceeds maximum length of 50000 characters".to_owned()
        )
    );
    // The limit counts runes, not bytes: 50_001 multibyte runes exceed it even
    // though Go and Rust both measure the trimmed text.
    assert_eq!(
        failed(dispatch(&format!(
            r#"{{"message":"{}"}}"#,
            "中".repeat(50_001)
        ))),
        (
            400,
            "ADK_CHAT_FAILED".to_owned(),
            "message exceeds maximum length of 50000 characters".to_owned()
        )
    );
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:180 TestSessionTimelineFailureKeepsLegacyErrorCode
/// TestSessionTimelineFailureKeepsLegacyErrorCode.
///
/// Go wraps a transcript-read failure in `ErrSessionTimelineFailed`, and
/// `handleADKSession` maps that sentinel to the legacy
/// `500 ADK_MESSAGES_GET_FAILED` envelope before the generic
/// `ADK_SESSION_GET_FAILED` fallback.  The distinction matters: the console
/// keys its "reload the transcript" affordance off that code, so a timeline
/// failure must not collapse into the generic read-unavailable error.
#[test]
fn session_timeline_failure_keeps_the_legacy_messages_error_code() {
    let (port, directory) = unready_adk_port();
    port.store
        .upsert_agent(
            "agent-timeline-fail",
            r#"{"id":"agent-timeline-fail","name":"Timeline Fail","status":"ENABLED"}"#,
        )
        .expect("seed agent");
    port.store
        .upsert_session("session-timeline-fail", "agent-timeline-fail", "{}")
        .expect("seed session");
    // Go's fixture drops `adk_runs`, which is the table its SessionTimeline
    // reads.  The Rust session detail reads the transcript events table
    // through the session store, so dropping `events` is the equivalent
    // durable failure: the session row still exists, only the timeline read
    // fails.
    let session_path = directory.path().join("adk-session.db");
    let connection = rusqlite::Connection::open(&session_path).expect("open session database");
    connection
        .execute_batch("DROP TABLE events;")
        .expect("drop transcript table");
    drop(connection);

    let failure = port
        .read("/api/v1/adk/sessions/session-timeline-fail", "")
        .expect_err("a broken transcript table must fail the session detail read");
    match failure {
        AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 500, "message: {message}");
            assert_eq!(
                code, "ADK_MESSAGES_GET_FAILED",
                "the legacy timeline error code is a public contract"
            );
        }
        other => panic!("expected a classified read failure, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:92 TestAgentSaveErrorClassification
/// TestAgentSaveErrorClassification.
///
/// Go's `isADKAgentValidationError` splits an agent save failure into two
/// classes: recognised validation text ("provider not found", "provider is
/// disabled", "invalid agent ...", "unknown ADK tool/skill") that the edge
/// reports as `400 BAD_REQUEST`, and everything else - a storage failure in
/// particular - which must not be client-classified.  Rust performs the same
/// split at the source: `validate_agent_write` returns
/// `400 BAD_REQUEST`, while a store failure maps to
/// `500 ADK_MUTATION_FAILED`.  The validation half is covered by
/// `adk_agent_write_reports_the_go_validation_messages`; this test pins the
/// other half so a generic persistence error cannot silently become a 400.
#[test]
fn agent_save_storage_failure_is_not_client_classified() {
    let (port, directory) = unready_adk_port();
    // Drop the agents table so every write path fails at the store rather
    // than in validation.  Validation itself stays reachable because it only
    // consults the provider table.
    let adk_path = directory.path().join("adk.db");
    let connection = rusqlite::Connection::open(&adk_path).expect("open ADK database");
    connection
        .execute_batch("DROP TABLE adk_agents;")
        .expect("drop agents table");
    drop(connection);

    let error = create_agent_error(
        &port,
        json!({"id": "agent-storage-failure", "name": "Agent"}),
    );
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 500, "message: {message}");
            assert_eq!(
                code, "ADK_MUTATION_FAILED",
                "a persistence error must not be reported as a client validation error"
            );
        }
        other => panic!("expected a failed mutation, got {other:?}"),
    }

    // The recognised validation text still classifies as a 400 in the same
    // port, proving the split is the classifier rather than a blanket rule.
    assert_bad_request(
        create_agent_error(
            &port,
            json!({
                "id": "agent-missing-provider-after-failure",
                "name": "Agent",
                "providerId": "provider-missing",
            }),
        ),
        "provider not found",
    );
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:366 TestApprovalContract
/// TestApprovalContract.
///
/// Go registers an `approval`-gated write tool, chats `@contract.write save`,
/// and requires the chat envelope to carry exactly one `pendingApprovals`
/// entry.  The approvals list route then filters `status=PENDING`, and
/// `POST /api/v1/adk/approvals/{id}/deny` answers `200 ok=true`.  Rust must
/// project the staged approval on the chat envelope and keep the deny route on
/// the same optimistic-CAS path.
#[test]
fn adk_chat_approval_is_listed_as_pending_and_denied_with_ok_envelope() {
    // A real `ProductionAdkChatRuntime` is required: Go's route resolves and
    // stages the approval, then enqueues the continuation in the background
    // (`ResolveApprovalAsync`) and answers `200` immediately.  A fixture port
    // that cannot continue would instead report `503 ADK_CONTINUATION_UNAVAILABLE`
    // and roll the staged resolution back.
    let (port, _directory) = ready_adk_port_with_fallback_provider();
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-approval-contract",
            session_id: "session-approval-contract",
            agent_id: "agent-approval",
            status: "PENDING",
            client_request_id: "approval-contract-request",
            request_fingerprint: "approval-contract-fingerprint",
            payload_json: r#"{
                "id":"run-approval-contract",
                "sessionId":"session-approval-contract",
                "agentId":"agent-approval",
                "status":"PENDING",
                "workMode":"chat",
                "reply":"",
                "toolCalls":[{"id":"call-contract-write","name":"contract.write","status":"PENDING_APPROVAL"}],
                "pendingApprovals":[{"id":"approval-contract","runId":"run-approval-contract","agentId":"agent-approval","toolName":"contract.write","status":"PENDING"}]
            }"#,
        })
        .expect("seed pending approval run");
    port.store
        .create_approval(
            "approval-contract",
            "run-approval-contract",
            "agent-approval",
            "PENDING",
            r#"{"id":"approval-contract","runId":"run-approval-contract","agentId":"agent-approval","toolName":"contract.write","status":"PENDING"}"#,
        )
        .expect("seed pending approval");

    // The pending approval is visible on the filtered list route.
    let output = port
        .read("/api/v1/adk/approvals", "status=PENDING")
        .expect("approvals read");
    let AdkReadSnapshot::Json(value) = output else {
        panic!("approvals route must answer JSON");
    };
    let approvals = value["approvals"].as_array().expect("approvals array");
    assert_eq!(approvals.len(), 1, "one pending approval: {approvals:?}");
    assert_eq!(approvals[0]["id"], "approval-contract");

    // Denying resolves through the optimistic CAS and returns the resolution.
    let denied = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Deny,
            identifiers: BTreeMap::from([(
                "approvalId".to_owned(),
                "approval-contract".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("deny must succeed");
    assert_eq!(denied["approval"]["id"], "approval-contract");
    assert_eq!(denied["approval"]["status"], "DENIED");

    // The resolved approval leaves the PENDING filter.
    let output = port
        .read("/api/v1/adk/approvals", "status=PENDING")
        .expect("approvals read after deny");
    let AdkReadSnapshot::Json(value) = output else {
        panic!("approvals route must answer JSON");
    };
    let approvals = value["approvals"].as_array().expect("approvals array");
    assert!(
        approvals
            .iter()
            .all(|approval| approval["id"] != "approval-contract"),
        "a denied approval must not stay PENDING: {approvals:?}"
    );
}

/// Seed a `PENDING` run plus its staged approval so the approval-resolution
/// envelope can be exercised end to end.  The runtime is the real production
/// chat runtime (required: Go resolves and stages the approval, then enqueues
/// the background continuation before answering); its provider points at a
/// closed loopback port so the continuation fails deterministically without
/// touching the network.
fn seed_pending_approval_run(
    suffix: &str,
    tool_call_id: &str,
    tool_name: &str,
) -> (Arc<ProductionAdkPort>, tempfile::TempDir, String, String) {
    let (port, directory) = ready_adk_port_with_fallback_provider();
    let (run_id, approval_id) =
        seed_pending_approval_rows(&port.store, suffix, tool_call_id, tool_name);
    (port, directory, run_id, approval_id)
}

/// Persist the `PENDING` run, its staged approval, and the session/agent rows
/// the resolution routes resolve, so any runtime fixture can exercise the
/// approval envelope without depending on the production provider fixture.
fn seed_pending_approval_rows(
    store: &Arc<AdkStore>,
    suffix: &str,
    tool_call_id: &str,
    tool_name: &str,
) -> (String, String) {
    let run_id = format!("run-approval-{suffix}");
    let approval_id = format!("approval-{suffix}");
    let session_id = format!("session-approval-{suffix}");
    let payload = json!({
        "id": run_id,
        "sessionId": session_id,
        "agentId": "agent-approval",
        "status": "PENDING",
        "workMode": "chat",
        "requestMessage": "run the gated tool",
        "reply": "",
        "toolCalls": [{
            "id": tool_call_id,
            "runId": run_id,
            "name": tool_name,
            "toolName": tool_name,
            "status": "PENDING_APPROVAL",
            "requiresUser": true,
        }],
        "pendingApprovals": [{
            "id": approval_id,
            "runId": run_id,
            "agentId": "agent-approval",
            "toolName": tool_name,
            "status": "PENDING",
        }],
    });
    // The approved continuation resolves the agent + provider before it
    // enqueues the background model call, so the fixture needs a real enabled
    // agent row; the denial path short-circuits before provider resolution.
    store
        .upsert_agent(
            "agent-approval",
            &json!({
                "id": "agent-approval",
                "name": "Approval Agent",
                "providerId": "provider-ready",
                "status": "ENABLED",
                "permissionMode": "approval",
            })
            .to_string(),
        )
        .expect("seed approval agent");
    // The session row is what the session detail route resolves before it
    // reads the transcript, so the fixture has to create it explicitly.
    store
        .upsert_session(&session_id, "agent-approval", "{}")
        .expect("seed approval session");
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: &run_id,
            session_id: &session_id,
            agent_id: "agent-approval",
            status: "PENDING",
            client_request_id: &format!("{suffix}-request"),
            request_fingerprint: &format!("{suffix}-fingerprint"),
            payload_json: &payload.to_string(),
        })
        .expect("seed pending approval run");
    store
        .create_approval(
            &approval_id,
            &run_id,
            "agent-approval",
            "PENDING",
            &json!({
                "id": approval_id,
                "runId": run_id,
                "agentId": "agent-approval",
                "toolName": tool_name,
                "status": "PENDING",
            })
            .to_string(),
        )
        .expect("seed pending approval");
    (run_id, approval_id)
}

/// Parity: go:452dea11:internal/api/assistant/adk_approval_test.go:16 TestADKApprovalApproveRouteReturnsRunningResolutionEnvelope
/// TestADKApprovalApproveRouteReturnsRunningResolutionEnvelope.
///
/// Go answers `200 ok=true` with the resolution envelope while the approved
/// continuation runs in the background: the approval is `APPROVED`, the run is
/// `RUNNING` with `resumeState=approval_resuming`, its tool call is already
/// `RUNNING`, and the session detail exposes a running tool group for that run
/// before the tool itself finishes.
///
/// The route's own responsibility is the durable staging plus the continuation
/// wakeup.  The fixture therefore records the wakeup for the resolved run and
/// returns immediately, which keeps the staged `RUNNING` projection observable
/// (a real provider call would immediately overwrite `resumeState`); the live
/// model-call path is covered end to end by
/// `production_live_chat_stream_emits_session_run_and_final_events`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:471
/// `TestApprovalModeCreatesPendingApprovalForWriteTool`.
#[test]
fn adk_approval_approve_returns_the_running_resolution_envelope() {
    #[derive(Debug, Default)]
    struct RecordingContinuationRuntime {
        resumed: std::sync::Mutex<Vec<String>>,
    }

    impl AdkChatStreamPort for RecordingContinuationRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
            self.resumed
                .lock()
                .expect("resume log")
                .push(run_id.to_owned());
            Ok(())
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let runtime = Arc::new(RecordingContinuationRuntime::default());
    let (port, store, _directory) =
        setup_test_adk_mutation_port(Some(Arc::clone(&runtime) as Arc<dyn AdkChatStreamPort>));
    let (run_id, approval_id) = seed_pending_approval_rows(
        &store,
        "approve-running",
        "call-approve-running",
        "contract.write",
    );

    let resolution = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("approve must answer the resolution envelope");

    assert_eq!(resolution["approval"]["id"], approval_id);
    assert_eq!(resolution["approval"]["status"], "APPROVED");
    let run = &resolution["run"];
    assert_eq!(run["id"], run_id, "resolution envelope: {resolution}");
    assert_eq!(run["status"], "RUNNING");
    assert_eq!(run["resumeState"], "approval_resuming");
    let tool_calls = run["toolCalls"].as_array().expect("toolCalls array");
    assert_eq!(tool_calls.len(), 1, "resolution toolCalls: {tool_calls:?}");
    assert_eq!(tool_calls[0]["status"], "RUNNING");
    assert_eq!(tool_calls[0]["requiresUser"], false);
    assert_eq!(
        runtime.resumed.lock().expect("resume log").as_slice(),
        std::slice::from_ref(&run_id),
        "the route must wake exactly the resolved run's continuation"
    );

    // The durable run carries the same running projection, which is what the
    // session detail and run detail routes read back.
    let stored = port
        .store
        .get_run(&run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["status"], "RUNNING");
    assert_eq!(payload["resumeState"], "approval_resuming");
    assert_eq!(
        payload["toolCalls"][0]["status"], "RUNNING",
        "a released approval must not leave its tool call pending: {payload}"
    );

    // Once resolved the approval leaves the PENDING filter.
    let AdkReadSnapshot::Json(value) = port
        .read("/api/v1/adk/approvals", "status=PENDING")
        .expect("approvals read")
    else {
        panic!("approvals route must answer JSON");
    };
    assert!(
        value["approvals"]
            .as_array()
            .expect("approvals array")
            .iter()
            .all(|approval| approval["id"] != approval_id),
        "an approved approval must not stay PENDING: {value}"
    );
}

/// Parity: go:452dea11:internal/api/assistant/adk_approval_test.go:183 TestADKApprovalRouteReturnsResolutionEnvelope
/// TestADKApprovalRouteReturnsResolutionEnvelope.
///
/// Go's denial path keeps the same envelope shape: `200 ok=true`, approval
/// `DENIED`, run `RUNNING` with `resumeState=approval_resuming`, and no
/// synchronous assistant `message` (the background continuation owns the
/// terminal state).
#[test]
fn adk_approval_deny_returns_the_resolution_envelope_without_a_sync_message() {
    let (port, _directory, run_id, approval_id) =
        seed_pending_approval_run("deny-running", "call-deny-running", "contract.write");

    let resolution = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Deny,
            identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("deny must answer the resolution envelope");

    assert_eq!(resolution["approval"]["id"], approval_id);
    assert_eq!(resolution["approval"]["status"], "DENIED");
    assert!(
        resolution.get("message").is_none(),
        "Go does not emit a synchronous assistant summary: {resolution}"
    );
    let run = &resolution["run"];
    assert_eq!(run["id"], run_id, "resolution envelope: {resolution}");
    assert_eq!(run["status"], "RUNNING");
    assert_eq!(run["resumeState"], "approval_resuming");

    // The denial is durable: the run reaches DENIED and the tool call is
    // rejected rather than left pending.
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(10);
    loop {
        let stored = port
            .store
            .get_run(&run_id)
            .expect("read run")
            .expect("run exists");
        if stored.status == "DENIED" {
            let payload: Value =
                serde_json::from_str(&stored.payload_json).expect("denied run payload");
            assert_eq!(payload["toolCalls"][0]["status"], "DENIED");
            assert_eq!(payload["toolCalls"][0]["requiresUser"], false);
            break;
        }
        assert!(
            std::time::Instant::now() < deadline,
            "denied run did not converge: {stored:?}"
        );
        std::thread::sleep(std::time::Duration::from_millis(10));
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:261 TestProviderDefaultContract
/// TestProviderDefaultContract.
///
/// Go creates two providers, promotes the second to default, and requires the
/// list route to return the default first; promoting a missing provider is
/// `404 ADK_PROVIDER_DEFAULT_FAILED` (the route's own code, not the entity
/// not-found code used by provider updates).
#[test]
fn provider_default_contract_orders_the_default_first_and_keeps_the_route_code() {
    let (port, _directory) = agent_validation_port();
    for id in ["provider-default-a", "provider-default-b"] {
        port.mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": id,
                "displayName": id,
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            }),
            webhook_secret: None,
        })
        .expect("create provider");
    }

    let promoted = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::SetDefaultProvider,
            identifiers: BTreeMap::from([(
                "providerId".to_owned(),
                "provider-default-b".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("promote provider-default-b");
    assert_eq!(promoted["id"], "provider-default-b");
    assert_eq!(promoted["default"], true);

    let AdkReadSnapshot::Json(listed) = port
        .read("/api/v1/adk/providers", "")
        .expect("providers read")
    else {
        panic!("providers route must answer JSON");
    };
    let ids = listed["providers"]
        .as_array()
        .expect("providers array")
        .iter()
        .filter_map(|provider| provider["id"].as_str())
        .collect::<Vec<_>>();
    let default_index = ids
        .iter()
        .position(|id| *id == "provider-default-b")
        .expect("default provider must be listed");
    let other_index = ids
        .iter()
        .position(|id| *id == "provider-default-a")
        .expect("other provider must be listed");
    assert!(
        default_index < other_index,
        "the default provider must be listed first: {ids:?}"
    );

    // Deleting the selected default promotes the remaining provider, and the
    // promoted row is listed first again.
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteProvider,
        identifiers: BTreeMap::from([("providerId".to_owned(), "provider-default-b".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete the default provider");
    let AdkReadSnapshot::Json(relisted) = port
        .read("/api/v1/adk/providers", "")
        .expect("providers read after delete")
    else {
        panic!("providers route must answer JSON");
    };
    let providers = relisted["providers"].as_array().expect("providers array");
    assert_ne!(
        providers[0]["id"], "provider-default-b",
        "the deleted default is gone: {relisted}"
    );
    assert_eq!(
        providers[0]["default"], true,
        "a remaining provider is promoted and listed first: {relisted}"
    );
    assert_eq!(
        providers
            .iter()
            .filter(|provider| provider["default"] == true)
            .count(),
        1,
        "exactly one provider stays default: {relisted}"
    );

    let missing = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::SetDefaultProvider,
            identifiers: BTreeMap::from([(
                "providerId".to_owned(),
                "provider-default-missing".to_owned(),
            )]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect_err("promoting a missing provider must fail closed");
    match missing {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404);
            assert_eq!(
                code, "ADK_PROVIDER_DEFAULT_FAILED",
                "Go keeps the route's own code; message {message}"
            );
            assert_eq!(message, "provider not found");
        }
        other => panic!("expected 404 ADK_PROVIDER_DEFAULT_FAILED, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/engine/persistence/provider_reasoning_test.go:47 TestProviderReasoningPersistenceDefaultsToEmptyMappings
/// Providers always expose the normalized Responses reasoning configuration,
/// even when the caller omits it from the write payload.
#[test]
fn provider_reasoning_config_defaults_to_the_responses_field_and_empty_mappings() {
    let (port, _directory) = agent_validation_port();
    let saved = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "provider-reasoning-default",
                "displayName": "Reasoning Default",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
            }),
            webhook_secret: None,
        })
        .expect("create provider");
    assert_eq!(
        saved["reasoningConfig"]["requestField"],
        "reasoning.effort",
        "provider writes use Go's default reasoning request field"
    );
    assert_eq!(
        saved["reasoningConfig"]["mappings"],
        json!([]),
        "missing reasoning mappings normalize to an explicit empty array"
    );

    let explicit_empty = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "provider-reasoning-empty",
                "displayName": "Reasoning Empty",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
                "reasoningConfig": {"requestField": "provider.reasoning", "mappings": []}
            }),
            webhook_secret: None,
        })
        .expect("create provider with explicit empty mappings");
    assert_eq!(
        explicit_empty["reasoningConfig"],
        json!({"requestField": "provider.reasoning", "mappings": []})
    );
}

/// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:8 TestProviderReasoningPresetsAndExplicitEmptyMappings
/// Custom mappings are trimmed, normalized to stable effort names and sorted;
/// malformed provider reasoning configurations fail before persistence.
#[test]
// Parity: go:452dea11:internal/assistant/model/provider_reasoning_config_test.go:27 TestProviderReasoningValidationAndCustomMapping
fn provider_reasoning_config_normalizes_custom_mappings_and_rejects_duplicates() {
    let (port, _directory) = agent_validation_port();
    let saved = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "provider-reasoning-custom",
                "displayName": "Reasoning Custom",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
                "reasoningConfig": {
                    "requestField": " reasoning.level ",
                    "mappings": [
                        {"effort": " HIGH ", "value": " balanced "},
                        {"effort": "low", "value": " LOW "}
                    ]
                }
            }),
            webhook_secret: None,
        })
        .expect("create provider with custom reasoning mappings");
    assert_eq!(saved["reasoningConfig"]["requestField"], "reasoning.level");
    assert_eq!(saved["reasoningConfig"]["mappings"][0]["effort"], "low");
    assert_eq!(saved["reasoningConfig"]["mappings"][0]["value"], "LOW");
    assert_eq!(saved["reasoningConfig"]["mappings"][1]["effort"], "high");
    assert_eq!(saved["reasoningConfig"]["mappings"][1]["value"], "balanced");

    let duplicate = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateProvider,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "provider-reasoning-duplicate",
                "displayName": "Reasoning Duplicate",
                "baseUrl": "https://example.test/v1",
                "model": "fixture-model",
                "enabled": true,
                "reasoningConfig": {
                    "requestField": "reasoning.level",
                    "mappings": [
                        {"effort": "low", "value": "LOW"},
                        {"effort": "LOW", "value": "FAST"}
                    ]
                }
            }),
            webhook_secret: None,
        })
        .expect_err("duplicate reasoning efforts must be rejected");
    match duplicate {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
            assert!(message.contains("duplicate provider reasoning effort"));
        }
        other => panic!("expected invalid provider reasoning error, got {other:?}"),
    }

    for (id, field, mappings) in [
        (
            "provider-reasoning-reserved",
            "model.reasoning",
            json!([{"effort": "low", "value": "LOW"}]),
        ),
        (
            "provider-reasoning-effort",
            "reasoning.level",
            json!([{"effort": "default", "value": "DEFAULT"}]),
        ),
        (
            "provider-reasoning-value",
            "reasoning.level",
            json!([{"effort": "low", "value": "  "}]),
        ),
    ] {
        let error = port
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::CreateProvider,
                identifiers: BTreeMap::new(),
                body: json!({
                    "id": id,
                    "displayName": id,
                    "baseUrl": "https://example.test/v1",
                    "model": "fixture-model",
                    "enabled": true,
                    "reasoningConfig": {"requestField": field, "mappings": mappings}
                }),
                webhook_secret: None,
            })
            .expect_err("invalid reasoning mapping must be rejected");
        assert!(matches!(error, AdkMutationPortError::Failed { status: 400, .. }));
    }
}

/// Parity: go:452dea11:internal/assistant/engine/persistence/provider_reasoning_test.go:10
/// TestProviderReasoningPersistenceAllowsMappingChanges.
#[test]
fn provider_reasoning_mapping_changes_persist_and_gate_new_agent_efforts() {
    let (port, _directory) = agent_validation_port();
    port.store
        .upsert_provider(
            "provider-enabled",
            &json!({
                "displayName": "Enabled Provider",
                "baseUrl": "https://api.example.test/v1",
                "model": "fixture-model",
                "enabled": true,
                "reasoningConfig": {
                    "requestField": "reasoning.level",
                    "mappings": [
                        {"effort": "low", "value": "LOW"},
                        {"effort": "high", "value": "HIGH"}
                    ]
                }
            })
            .to_string(),
        )
        .expect("persist reasoning provider");
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body: json!({
            "id": "agent-reasoning-existing",
            "name": "Existing reasoning agent",
            "providerId": "provider-enabled",
            "reasoningEffort": "high",
            "status": "ENABLED"
        }),
        webhook_secret: None,
    })
    .expect("create agent with supported effort");

    let updated = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateProvider,
            identifiers: BTreeMap::from([(
                "providerId".to_owned(),
                "provider-enabled".to_owned(),
            )]),
            body: json!({
                "reasoningConfig": {
                    "requestField": "reasoning.level",
                    "mappings": [{"effort": "low", "value": "FAST"}]
                }
            }),
            webhook_secret: None,
        })
        .expect("update provider reasoning mappings");
    assert_eq!(updated["reasoningConfig"]["mappings"], json!([
        {"effort": "low", "value": "FAST"}
    ]));

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "agent-reasoning-unsupported",
                "name": "Unsupported reasoning agent",
                "providerId": "provider-enabled",
                "reasoningEffort": "high",
                "status": "ENABLED"
            }),
            webhook_secret: None,
        })
        .expect_err("unsupported effort must be rejected");
    assert_bad_request(error, "provider does not support reasoning effort: high");
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:787 TestADKSessionNegativeRoutes
/// TestADKSessionNegativeRoutes.
///
/// Go keeps a dedicated error envelope per session negative case: a missing or
/// disabled agent on create is `400 BAD_REQUEST` / "enabled agent is
/// required", a blank/undecodable `sessionId` is `400 BAD_REQUEST` /
/// "sessionId is invalid", a missing session detail is `404 NOT_FOUND` /
/// "session not found", and a malformed rename payload is
/// `400 BAD_REQUEST` / "invalid session payload".
#[test]
fn adk_session_negative_routes_keep_the_go_error_envelopes() {
    let (port, _directory) = unready_adk_port();

    // Missing agent on create.
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateSession,
        identifiers: BTreeMap::new(),
        body: json!({"agentId": "missing-agent", "title": "x"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
            assert_eq!(message, "enabled agent is required");
        }
        other => panic!("expected 400 BAD_REQUEST for missing agent, got {other:?}"),
    }

    // Missing session detail.
    match port.read("/api/v1/adk/sessions/session-missing", "") {
        Err(AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
            assert_eq!(message, "session not found");
        }
        other => panic!("expected 404 NOT_FOUND for a missing session, got {other:?}"),
    }

    // Malformed rename payload against an existing session.
    port.store
        .upsert_agent(
            "session-negative-agent",
            r#"{"id":"session-negative-agent","name":"Session Negative Agent","status":"ENABLED","permissionMode":"approval"}"#,
        )
        .expect("seed agent");
    port.store
        .upsert_session("session-negative", "session-negative-agent", "{}")
        .expect("seed session");
    let malformed = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
        &crate::product::product_adk_mutation_port::AdkMutationRequest {
            method: "PUT".to_owned(),
            path: "/api/v1/adk/sessions/session-negative".to_owned(),
            body: Some(br#"{"title":"#.to_vec()),
            headers: BTreeMap::new(),
        },
        Some(&port),
        "2026-09-19T00:00:00Z",
    );
    assert_eq!(malformed.status, 400, "malformed rename: {malformed:?}");
    assert_eq!(malformed.body["error"]["code"], "BAD_REQUEST");
    assert_eq!(
        malformed.body["error"]["message"],
        "invalid session payload"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:436
/// TestResolveApprovalMissingReturnsIdempotentEmptyResult.
///
/// `Runtime.ResolveApproval` answers an unknown approval id with the zero
/// resolution: `approval.id` and `approval.status` stay empty and both `run`
/// and `message` are unset.  The reference is explicit that this is *not* an
/// error, so the Rust port must not upgrade a missing row into a 404.  This is
/// the narrow missing-target companion to
/// `adk_approval_negative_and_idempotent_routes_match_the_go_envelopes` (which
/// also covers the resolved-twice path); the store side is frozen separately by
/// `adk_store_contracts::adk_approval_resolution_missing_and_non_pending_rows_are_idempotent`.
#[test]
fn adk_resolve_approval_missing_returns_the_idempotent_empty_envelope() {
    let (port, _directory) = unready_adk_port();

    for operation in [AdkMutationOperation::Approve, AdkMutationOperation::Deny] {
        let resolution = port
            .mutate(&AdkMutationInput {
                operation,
                identifiers: BTreeMap::from([(
                    "approvalId".to_owned(),
                    "approval-missing".to_owned(),
                )]),
                body: Value::Null,
                webhook_secret: None,
            })
            .unwrap_or_else(|error| panic!("{operation:?} on a missing approval: {error:?}"));
        assert_eq!(
            resolution,
            json!({"approval": {"id": ""}}),
            "a missing approval resolves to the zero envelope, not an error: {resolution}"
        );
        assert!(
            resolution.get("run").is_none() && resolution.get("message").is_none(),
            "the missing resolution carries no run or message: {resolution}"
        );
    }

    // The zero envelope is the whole response; neither branch may fabricate a
    // row or a continuation for an id that was never persisted.
    assert!(
        port.store
            .list_approvals()
            .expect("list approvals")
            .is_empty(),
        "resolving a missing approval must not create a row"
    );
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:911 TestADKApprovalNegativeAndIdempotentRoutes
/// TestADKApprovalNegativeAndIdempotentRoutes.
///
/// An unknown approval is idempotent in Go: approving a missing id answers
/// `200 ok=true` with an empty resolution (`approval.id`/`status` blank and no
/// run/message), approving the same real approval twice stays
/// `200`/`APPROVED`, and the blank-identifier branch answers
/// `400 BAD_REQUEST` / "approvalId is invalid".
#[test]
fn adk_approval_negative_and_idempotent_routes_match_the_go_envelopes() {
    let (port, _directory, _run_id, approval_id) =
        seed_pending_approval_run("idempotent", "call-idempotent", "contract.write");

    let missing = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), "approval-missing".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("approving an unknown approval is idempotent in Go");
    assert_eq!(missing["approval"]["id"], "");
    assert!(
        missing.get("run").is_none(),
        "missing resolution run: {missing}"
    );
    assert!(
        missing.get("message").is_none(),
        "missing resolution message: {missing}"
    );

    let first = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("first approve");
    assert_eq!(first["approval"]["status"], "APPROVED");

    let second = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("second approve must be idempotent");
    assert_eq!(
        second["approval"]["status"], "APPROVED",
        "a repeated approve keeps the resolved projection: {second}"
    );

    // The blank identifier is rejected at the wire edge before the port runs.
    let blank = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
        &crate::product::product_adk_mutation_port::AdkMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/adk/approvals/%20/approve".to_owned(),
            body: None,
            headers: BTreeMap::new(),
        },
        Some(port.as_ref()),
        "2026-09-19T00:00:00Z",
    );
    assert_eq!(blank.status, 400, "blank approvalId: {blank:?}");
    assert_eq!(blank.body["error"]["code"], "BAD_REQUEST");
    assert_eq!(blank.body["error"]["message"], "approvalId is invalid");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:119 TestConcurrentSiblingAsyncApprovalsEnqueueOneContinuation.
/// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:207 TestResolvedApprovalDoesNotStealForeignExecutionLease.
///
/// Go claims the continuation before starting it, so a second wakeup for a run
/// whose continuation is already in flight returns the resolution envelope
/// instead of an error: the owner in flight rereads the resolved run and
/// executes the released tool call exactly once.  Rust reaches the same
/// window through the durable recovery scanner (the `PENDING`->`RUNNING`
/// stage commits before the route asks the runtime to resume), so an
/// already-claimed continuation must answer the envelope rather than
/// `503 ADK_CONTINUATION_UNAVAILABLE`.
#[test]
fn adk_approval_wakeup_accepts_an_already_claimed_continuation() {
    #[derive(Debug)]
    struct ClaimedContinuationRuntime;

    impl AdkChatStreamPort for ClaimedContinuationRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
            Err(AdkChatPortError::Conflict(
                "assistant continuation is already running".to_owned(),
            ))
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let (port, store, _directory) =
        setup_test_adk_mutation_port(Some(Arc::new(ClaimedContinuationRuntime)));
    let (run_id, approval_id) =
        seed_pending_approval_rows(&store, "claimed", "call-claimed", "contract.write");

    let approval_resolution = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("an already-claimed continuation must still answer the envelope");

    assert_eq!(approval_resolution["approval"]["id"], approval_id);
    assert_eq!(approval_resolution["approval"]["status"], "APPROVED");
    let run = &approval_resolution["run"];
    assert_eq!(
        run["id"], run_id,
        "resolution envelope: {approval_resolution}"
    );
    assert_eq!(run["status"], "RUNNING");
    assert_eq!(run["resumeState"], "approval_resuming");
    assert_eq!(run["toolCalls"][0]["status"], "RUNNING");
    assert_eq!(run["toolCalls"][0]["requiresUser"], false);

    // The staged resolution stays durable: the in-flight owner (or the next
    // recovery pass) still finds the released tool call.
    let stored = store
        .get_run(&run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["status"], "RUNNING");
    assert_eq!(payload["toolCalls"][0]["status"], "RUNNING");

    let recovered = store
        .list_approvals()
        .expect("list approvals")
        .into_iter()
        .find(|approval| approval.id == approval_id)
        .expect("approval exists");
    assert_eq!(
        recovered.status, "APPROVED",
        "a claimed continuation must not roll the approval back to PENDING"
    );
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:26 TestADKSessionDetailOmitsResolvedApprovalGroups
/// TestADKSessionDetailOmitsResolvedApprovalGroups.
///
/// Once an approval is resolved, Go's session detail no longer carries an
/// approval-group timeline entry and the pending-approval filter no longer
/// lists it.  Rust's session timeline only projects user/assistant messages
/// (there is no `approvalGroup` kind), so the guarantee is asserted through
/// both surfaces: no timeline entry is an approval group, and the
/// `status=PENDING` list excludes it.
#[test]
fn adk_session_detail_omits_resolved_approval_groups() {
    let (port, _directory, run_id, approval_id) =
        seed_pending_approval_run("resolved-group", "call-resolved-group", "contract.write");

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::Approve,
        identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("approve the staged approval");

    let AdkReadSnapshot::Json(detail) = port
        .read("/api/v1/adk/sessions/session-approval-resolved-group", "")
        .expect("session detail")
    else {
        panic!("session detail must answer JSON");
    };
    let timeline = detail["timeline"].as_array().expect("timeline array");
    for entry in timeline {
        let kind = entry["kind"].as_str().unwrap_or_default();
        assert_ne!(
            kind, "approval_group",
            "a resolved approval must not leave an approval group: {entry}"
        );
        // Message IDs include the run/session fixture suffix, which also
        // occurs in approval IDs. That substring is not an approval reference.
        assert_ne!(entry["approvalId"], approval_id);
    }

    let AdkReadSnapshot::Json(pending) = port
        .read("/api/v1/adk/approvals", "status=PENDING")
        .expect("pending approvals")
    else {
        panic!("approvals route must answer JSON");
    };
    assert!(
        pending["approvals"]
            .as_array()
            .expect("approvals array")
            .iter()
            .all(|approval| approval["id"] != approval_id),
        "resolved approval {approval_id} must not stay pending: {pending}"
    );

    // The run itself is still reachable from the session detail.
    let runs = detail["runs"].as_array().expect("runs array");
    assert!(
        runs.iter().any(|run| run["id"] == run_id),
        "session detail must keep the resolved run: {runs:?}"
    );
}

/// Parity: go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:278 TestAssistantCatalogBoundaryStatusCodes
/// TestAssistantCatalogBoundaryStatusCodes.
///
/// The catalog boundary matrix keeps one status per branch: `400` for an
/// unknown task status or an invalid memory scope, `404` for missing task /
/// memory / default-provider targets, `409` for a provider still referenced by
/// an agent or for the protected built-in agent, and `400` for malformed
/// provider / agent / skill payloads.
#[test]
fn adk_catalog_boundary_status_codes_match_the_go_matrix() {
    let (port, _directory) = agent_validation_port();
    port.store
        .upsert_agent(
            "agent-uses-provider",
            r#"{"id":"agent-uses-provider","name":"Provider User","providerId":"provider-enabled","status":"ENABLED"}"#,
        )
        .expect("seed referencing agent");
    port.store
        .upsert_task(
            "task-invalid-patch",
            "TODO",
            "",
            "",
            r#"{"id":"task-invalid-patch","title":"Valid title","status":"TODO"}"#,
        )
        .expect("seed task");

    for (path, query, status, code) in [
        (
            "/api/v1/adk/tasks",
            "status=BAD",
            400,
            "ADK_TASK_LIST_FAILED",
        ),
        (
            "/api/v1/adk/memory",
            "scope=private",
            400,
            "ADK_MEMORY_LIST_FAILED",
        ),
    ] {
        match port.read(path, query) {
            Err(AdkReadSnapshotError::Failed {
                status: actual_status,
                code: actual_code,
                ..
            }) => {
                assert_eq!(actual_status, status, "{path}?{query}");
                assert_eq!(actual_code, code, "{path}?{query}");
            }
            other => panic!("expected {status} {code} for {path}?{query}, got {other:?}"),
        }
    }

    for (path, code, message) in [
        (
            "/api/v1/adk/tasks/missing-task",
            "ADK_TASK_NOT_FOUND",
            "task not found",
        ),
        (
            "/api/v1/adk/sessions/missing-session/context",
            "ADK_SESSION_CONTEXT_FAILED",
            "session not found",
        ),
    ] {
        match port.read(path, "") {
            Err(AdkReadSnapshotError::Failed {
                status,
                code: actual_code,
                message: actual_message,
                ..
            }) => {
                assert_eq!(status, 404, "{path}");
                assert_eq!(actual_code, code, "{path}");
                assert_eq!(actual_message, message, "{path}");
            }
            other => panic!("expected 404 {code} for {path}, got {other:?}"),
        }
    }

    // Task mutations keep the task-specific 404 and blank-title 400.
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::UpdateTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), "missing-task".to_owned())]),
        body: json!({"title": "patched"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_TASK_NOT_FOUND");
            assert_eq!(message, "task not found");
        }
        other => panic!("expected 404 ADK_TASK_NOT_FOUND, got {other:?}"),
    }
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateTask,
        identifiers: BTreeMap::new(),
        body: json!({"title": " "}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_TASK_SAVE_FAILED");
        }
        other => panic!("expected 400 ADK_TASK_SAVE_FAILED, got {other:?}"),
    }

    // Deleting a missing memory keeps the memory-specific 404.
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteMemory,
        identifiers: BTreeMap::from([("memoryId".to_owned(), "missing-memory".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_MEMORY_NOT_FOUND");
            assert_eq!(message, "memory not found");
        }
        other => panic!("expected 404 ADK_MEMORY_NOT_FOUND, got {other:?}"),
    }

    // A referenced provider cannot be deleted; the built-in agent is protected.
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteProvider,
        identifiers: BTreeMap::from([("providerId".to_owned(), "provider-enabled".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_PROVIDER_DELETE_FAILED");
        }
        other => panic!("expected 409 ADK_PROVIDER_DELETE_FAILED, got {other:?}"),
    }
    for operation in [
        AdkMutationOperation::DeleteAgent,
        AdkMutationOperation::UpdateAgent,
    ] {
        let body = if operation == AdkMutationOperation::UpdateAgent {
            json!({"status": "DISABLED"})
        } else {
            Value::Null
        };
        match port.mutate(&AdkMutationInput {
            operation,
            identifiers: BTreeMap::from([("agentId".to_owned(), "jftrade-default".to_owned())]),
            body,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed { status, code, .. }) => {
                assert_eq!(status, 409, "{operation:?}");
                assert_eq!(code, "ADK_AGENT_PROTECTED", "{operation:?}");
            }
            other => panic!("expected 409 ADK_AGENT_PROTECTED, got {other:?}"),
        }
    }

    // Malformed payloads stay 400 BAD_REQUEST on the mutation wire.
    for (method, path) in [
        ("POST", "/api/v1/adk/providers"),
        ("POST", "/api/v1/adk/agents"),
        ("POST", "/api/v1/adk/skills"),
    ] {
        let response = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
            &crate::product::product_adk_mutation_port::AdkMutationRequest {
                method: method.to_owned(),
                path: path.to_owned(),
                body: Some(b"{".to_vec()),
                headers: BTreeMap::new(),
            },
            Some(&port),
            "2026-09-19T00:00:00Z",
        );
        assert_eq!(response.status, 400, "{method} {path}: {response:?}");
        assert_eq!(response.body["error"]["code"], "BAD_REQUEST", "{path}");
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:329 TestAssistantSessionRunBoundaryStatusCodes
/// TestAssistantSessionRunBoundaryStatusCodes.
///
/// The session/run boundary matrix fixes one status per branch: `400` for a
/// missing create-agent, malformed payloads and an invalid `after`, `404` for
/// missing sessions, runs and streams, and `200` for the idempotent
/// approve-missing-approval branch.
#[test]
fn adk_session_run_boundary_status_codes_match_the_go_matrix() {
    let (port, _directory) = unready_adk_port();

    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateSession,
        identifiers: BTreeMap::new(),
        body: json!({"agentId": "missing-agent"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
        }
        other => panic!("expected 400 BAD_REQUEST, got {other:?}"),
    }

    for path in [
        "/api/v1/adk/sessions/missing-session",
        "/api/v1/adk/runs/missing-run",
        "/api/v1/adk/streams/missing-stream",
        "/api/v1/adk/runs/missing-run/stream",
    ] {
        match port.read(path, "") {
            Err(AdkReadSnapshotError::Failed { status, .. }) => {
                assert_eq!(status, 404, "{path}");
            }
            other => panic!("expected 404 for {path}, got {other:?}"),
        }
    }

    for path in [
        "/api/v1/adk/streams/missing-stream",
        "/api/v1/adk/runs/missing-run/stream",
    ] {
        let failure = crate::product::dispatch_adk_read(Some(&port), "GET", path, "after=abc")
            .expect_err("an invalid after must fail closed");
        assert_eq!(failure.status, 400, "{path}");
        assert_eq!(failure.code, "BAD_REQUEST", "{path}");
        assert_eq!(failure.message, "after is invalid", "{path}");
    }

    // Missing mutation targets keep their route statuses.
    for (operation, identifiers, status, code) in [
        (
            AdkMutationOperation::CancelRun,
            BTreeMap::from([("runId".to_owned(), "missing-run".to_owned())]),
            404,
            "ADK_RUN_CANCEL_FAILED",
        ),
        (
            AdkMutationOperation::PauseRun,
            BTreeMap::from([("runId".to_owned(), "missing-run".to_owned())]),
            404,
            "NOT_FOUND",
        ),
        (
            AdkMutationOperation::ResumeRun,
            BTreeMap::from([("runId".to_owned(), "missing-run".to_owned())]),
            404,
            "NOT_FOUND",
        ),
    ] {
        match port.mutate(&AdkMutationInput {
            operation,
            identifiers,
            body: Value::Null,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed {
                status: actual_status,
                code: actual_code,
                ..
            }) => {
                assert_eq!(actual_status, status, "{operation:?}");
                assert_eq!(actual_code, code, "{operation:?}");
            }
            other => panic!("expected {status} {code} for {operation:?}, got {other:?}"),
        }
    }

    // Missing approval approve is idempotent rather than 404.
    let missing = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::Approve,
            identifiers: BTreeMap::from([("approvalId".to_owned(), "missing-approval".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("approving a missing approval must answer the empty success envelope");
    assert_eq!(missing["approval"]["id"], "");

    // A valid payload against a missing session is 404, while malformed
    // payloads for the session/run surface stay 400 BAD_REQUEST (the body is
    // validated before the target is resolved, exactly as in Go).
    let missing_compact = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
        &crate::product::product_adk_mutation_port::AdkMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/adk/sessions/missing-session/context/compact".to_owned(),
            body: Some(br#"{"mode":"summary"}"#.to_vec()),
            headers: BTreeMap::new(),
        },
        Some(&port),
        "2026-09-19T00:00:00Z",
    );
    assert_eq!(missing_compact.status, 404, "{missing_compact:?}");

    for (method, path, body) in [
        (
            "POST",
            "/api/v1/adk/sessions",
            br#"{"agentId":"missing-agent"}"#.to_vec(),
        ),
        (
            "POST",
            "/api/v1/adk/sessions/missing-session/context/compact",
            b"{".to_vec(),
        ),
        ("PUT", "/api/v1/adk/sessions/missing-session", b"{".to_vec()),
        (
            "PATCH",
            "/api/v1/adk/sessions/missing-session/composer-state",
            b"{".to_vec(),
        ),
        (
            "PATCH",
            "/api/v1/adk/runs/missing-run/objective",
            b"{".to_vec(),
        ),
    ] {
        let response = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
            &crate::product::product_adk_mutation_port::AdkMutationRequest {
                method: method.to_owned(),
                path: path.to_owned(),
                body: Some(body),
                headers: BTreeMap::new(),
            },
            Some(&port),
            "2026-09-19T00:00:00Z",
        );
        assert_eq!(response.status, 400, "{method} {path}: {response:?}");
        assert_eq!(response.body["error"]["code"], "BAD_REQUEST", "{path}");
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:295 TestSessionRunAndOptimizationRouteContracts
/// TestSessionRunAndOptimizationRouteContracts.
///
/// Go seeds a disabled and an enabled agent, then walks the session/run/
/// optimization surface: creating a session for a disabled agent is
/// `400 BAD_REQUEST` / "enabled agent is required"; missing session detail,
/// context, composer state and compact targets are `404`; a compact against a
/// session with an active run is `409`; the objective update succeeds for a
/// loop run; missing pause/resume targets are `404`; a pending run cancels to
/// `CANCELLED`; and a missing optimization task is `404` on both get and
/// cancel.
#[test]
fn adk_session_run_and_optimization_route_contracts_match_go() {
    let (port, _directory) = unready_adk_port();

    // Disabled agent cannot own a session.
    port.store
        .upsert_agent(
            "agent-session-disabled",
            r#"{"id":"agent-session-disabled","name":"Disabled Agent","status":"DISABLED"}"#,
        )
        .expect("seed disabled agent");
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateSession,
        identifiers: BTreeMap::new(),
        body: json!({"agentId": "agent-session-disabled", "title": "should fail"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
            assert_eq!(message, "enabled agent is required");
        }
        other => panic!("expected 400 BAD_REQUEST, got {other:?}"),
    }

    // Enabled agent + session for the remaining branches.
    port.store
        .upsert_agent(
            "agent-session-enabled",
            r#"{"id":"agent-session-enabled","name":"Enabled Agent","status":"ENABLED","workMode":"loop"}"#,
        )
        .expect("seed enabled agent");
    port.store
        .upsert_session(
            "session-contract",
            "agent-session-enabled",
            r#"{"id":"session-contract","agentId":"agent-session-enabled","title":"Session Contract"}"#,
        )
        .expect("seed session");

    for path in [
        "/api/v1/adk/sessions/session-missing",
        "/api/v1/adk/sessions/session-missing/context",
    ] {
        match port.read(path, "") {
            Err(AdkReadSnapshotError::Failed { status, .. }) => {
                assert_eq!(status, 404, "{path}");
            }
            other => panic!("expected 404 for {path}, got {other:?}"),
        }
    }

    for (operation, identifiers, body, status) in [
        (
            AdkMutationOperation::UpdateSessionComposerState,
            BTreeMap::from([("sessionId".to_owned(), "session-missing".to_owned())]),
            json!({"workModeOverride": "loop"}),
            404,
        ),
        (
            AdkMutationOperation::CompactSessionContext,
            BTreeMap::from([("sessionId".to_owned(), "session-missing".to_owned())]),
            json!({"mode": "normal"}),
            404,
        ),
        (
            AdkMutationOperation::ResumeRun,
            BTreeMap::from([("runId".to_owned(), "run-missing".to_owned())]),
            Value::Null,
            404,
        ),
        (
            AdkMutationOperation::PauseRun,
            BTreeMap::from([("runId".to_owned(), "run-missing".to_owned())]),
            Value::Null,
            404,
        ),
    ] {
        match port.mutate(&AdkMutationInput {
            operation,
            identifiers,
            body,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed { status: actual, .. }) => {
                assert_eq!(actual, status, "{operation:?}")
            }
            other => panic!("expected {status} for {operation:?}, got {other:?}"),
        }
    }

    // Rename succeeds on the real session.
    let renamed = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RenameSession,
            identifiers: BTreeMap::from([("sessionId".to_owned(), "session-contract".to_owned())]),
            body: json!({"title": "Renamed Session Contract"}),
            webhook_secret: None,
        })
        .expect("rename existing session");
    assert_eq!(renamed["title"], "Renamed Session Contract");

    // Active run blocks compaction with the route's own 409 code.
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-active-compact",
            session_id: "session-contract",
            agent_id: "agent-session-enabled",
            status: "RUNNING",
            client_request_id: "active-compact-request",
            request_fingerprint: "active-compact-fingerprint",
            payload_json: r#"{"id":"run-active-compact","sessionId":"session-contract","agentId":"agent-session-enabled","status":"RUNNING","workMode":"loop","objective":"monitor market"}"#,
        })
        .expect("seed active run");
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CompactSessionContext,
        identifiers: BTreeMap::from([("sessionId".to_owned(), "session-contract".to_owned())]),
        body: json!({"mode": "normal", "reason": "manual"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_SESSION_CONTEXT_COMPACT_FAILED");
            assert!(
                message.contains("active run"),
                "the active-run reason is part of the envelope: {message}"
            );
        }
        other => panic!("expected 409 ADK_SESSION_CONTEXT_COMPACT_FAILED, got {other:?}"),
    }

    // The objective update succeeds on a loop run.
    let updated = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateRunObjective,
            identifiers: BTreeMap::from([("runId".to_owned(), "run-active-compact".to_owned())]),
            body: json!({"objective": "watch premarket liquidity"}),
            webhook_secret: None,
        })
        .expect("update objective on a loop run");
    assert_eq!(updated["objective"], "watch premarket liquidity");

    // A pending run cancels to CANCELLED.
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-cancel-route",
            session_id: "session-contract",
            agent_id: "agent-session-enabled",
            status: "PENDING",
            client_request_id: "cancel-route-request",
            request_fingerprint: "cancel-route-fingerprint",
            payload_json: r#"{"id":"run-cancel-route","sessionId":"session-contract","agentId":"agent-session-enabled","status":"PENDING"}"#,
        })
        .expect("seed pending run");
    let cancelled = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CancelRun,
            identifiers: BTreeMap::from([("runId".to_owned(), "run-cancel-route".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("cancel a pending run");
    assert_eq!(cancelled["status"], "CANCELLED");

    // Optimization list + missing-target routes.
    port.store
        .upsert_optimization_task(
            "optimization-route",
            r#"{"id":"optimization-route","status":"queued","objective":"maximize sharpe"}"#,
        )
        .expect("seed optimization task");
    let AdkReadSnapshot::Json(listed) = port
        .read("/api/v1/adk/optimization-tasks", "limit=1&offset=0")
        .expect("optimization list")
    else {
        panic!("optimization list must answer JSON");
    };
    assert!(
        listed["tasks"]
            .as_array()
            .expect("tasks array")
            .iter()
            .any(|task| task["id"] == "optimization-route"),
        "seeded optimization task must be listed: {listed}"
    );
    match port.read("/api/v1/adk/optimization-tasks/task-missing", "") {
        Err(AdkReadSnapshotError::Failed { status, .. }) => assert_eq!(status, 404),
        other => panic!("expected 404 for a missing optimization task, got {other:?}"),
    }
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CancelOptimizationTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), "task-missing".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
        }
        other => panic!("expected 404 NOT_FOUND for cancel, got {other:?}"),
    }

    // Deleting the session removes it from the detail read.
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteSession,
        identifiers: BTreeMap::from([("sessionId".to_owned(), "session-contract".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete session");
    match port.read("/api/v1/adk/sessions/session-contract", "") {
        Err(AdkReadSnapshotError::Failed { status, .. }) => assert_eq!(status, 404),
        other => panic!("expected 404 after delete, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:15 TestTaskAndMemoryCRUDContracts
/// TestTaskAndMemoryCRUDContracts.
///
/// Go creates a task with `childProviderId`/`childModel`, filters the list by
/// status, patches it to `DONE` with a new child model and a result summary,
/// reads it back, deletes it, and then sees `404`; memory is saved with a
/// Chinese key, filtered by agent + normalized key, deleted, and a second
/// delete is `404`.
#[test]
fn adk_task_and_memory_crud_contracts_match_go() {
    let (port, _directory) = unready_adk_port();
    port.store
        .upsert_agent(
            "agent-contract-crud",
            r#"{"id":"agent-contract-crud","name":"CRUD Agent","status":"ENABLED"}"#,
        )
        .expect("seed agent");

    let created = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateTask,
            identifiers: BTreeMap::new(),
            body: json!({
                "title": "检查盘前准备",
                "status": "IN_PROGRESS",
                "agentId": "agent-contract-crud",
                "message": "补齐观察清单",
                "childProviderId": "provider-child",
                "childModel": "model-child-a",
            }),
            webhook_secret: None,
        })
        .expect("create task");
    let task_id = created["id"].as_str().expect("task id").to_owned();
    assert!(!task_id.is_empty());
    assert_eq!(created["childProviderId"], "provider-child");
    assert_eq!(created["childModel"], "model-child-a");

    let AdkReadSnapshot::Json(listed) = port
        .read("/api/v1/adk/tasks", "status=IN_PROGRESS")
        .expect("task list")
    else {
        panic!("task list must answer JSON");
    };
    assert!(
        listed["tasks"]
            .as_array()
            .expect("tasks array")
            .iter()
            .any(|task| task["id"] == task_id.as_str()),
        "the created task must be listed under IN_PROGRESS: {listed}"
    );

    let patched = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateTask,
            identifiers: BTreeMap::from([("taskId".to_owned(), task_id.clone())]),
            body: json!({
                "status": "DONE",
                "resultSummary": "已完成",
                "childProviderId": "provider-child-updated",
                "childModel": "model-child-b",
            }),
            webhook_secret: None,
        })
        .expect("patch task");
    assert_eq!(patched["status"], "DONE");
    assert_eq!(patched["childModel"], "model-child-b");

    let AdkReadSnapshot::Json(read_back) = port
        .read(&format!("/api/v1/adk/tasks/{task_id}"), "")
        .expect("task read")
    else {
        panic!("task detail must answer JSON");
    };
    assert_eq!(read_back["resultSummary"], "已完成");
    assert_eq!(read_back["childProviderId"], "provider-child-updated");

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), task_id.clone())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete task");
    match port.read(&format!("/api/v1/adk/tasks/{task_id}"), "") {
        Err(AdkReadSnapshotError::Failed { status, code, .. }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_TASK_NOT_FOUND");
        }
        other => panic!("expected 404 after task delete, got {other:?}"),
    }

    // Memory: save a Chinese key, filter by agent + key, delete, then 404.
    let memory = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateMemory,
            identifiers: BTreeMap::new(),
            body: json!({
                "agentId": "agent-contract-crud",
                "key": "watch-note",
                "value": "关注开盘波动",
                "scope": "agent",
            }),
            webhook_secret: None,
        })
        .expect("save memory");
    let memory_id = memory["id"].as_str().expect("memory id").to_owned();
    assert!(!memory_id.is_empty());

    let AdkReadSnapshot::Json(memories) = port
        .read(
            "/api/v1/adk/memory",
            "agentId=agent-contract-crud&key=watch-note",
        )
        .expect("memory list")
    else {
        panic!("memory list must answer JSON");
    };
    assert!(
        memories["entries"]
            .as_array()
            .expect("entries array")
            .iter()
            .any(|entry| entry["id"] == memory_id.as_str()),
        "the saved memory must be listed: {memories}"
    );

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteMemory,
        identifiers: BTreeMap::from([("memoryId".to_owned(), memory_id.clone())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete memory");
    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteMemory,
        identifiers: BTreeMap::from([("memoryId".to_owned(), memory_id.clone())]),
        body: Value::Null,
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed { status, code, .. }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_MEMORY_NOT_FOUND");
        }
        other => panic!("expected 404 ADK_MEMORY_NOT_FOUND, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:134 TestAssistantRoutesClassifyMissingMutationTargets
/// TestAssistantRoutesClassifyMissingMutationTargets.
///
/// Go classifies three distinct missing-target failures at the route edge: a
/// missing task patch is `404 ADK_TASK_NOT_FOUND`, a missing run objective is
/// `404 NOT_FOUND`, and a blank title on an existing task is
/// `400 ADK_TASK_SAVE_FAILED`.  This witness keeps the exact three-way
/// classification separate from the broader read/mutation error table used by
/// `routes_error_contracts_test.go:14`.
#[test]
fn adk_missing_mutation_targets_keep_the_go_classification() {
    let (port, _directory) = unready_adk_port();
    port.store
        .upsert_task(
            "task-invalid-patch",
            "TODO",
            "",
            "",
            r#"{"id":"task-invalid-patch","title":"Valid title","status":"TODO"}"#,
        )
        .expect("seed task");

    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::UpdateTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), "task-missing".to_owned())]),
        body: json!({"status": "DONE"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_TASK_NOT_FOUND");
            assert_eq!(message, "task not found");
        }
        other => panic!("expected 404 ADK_TASK_NOT_FOUND, got {other:?}"),
    }

    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::UpdateRunObjective,
        identifiers: BTreeMap::from([("runId".to_owned(), "run-missing".to_owned())]),
        body: json!({"objective": "new objective"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
            assert_eq!(message, "run not found");
        }
        other => panic!("expected 404 NOT_FOUND, got {other:?}"),
    }

    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::UpdateTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), "task-invalid-patch".to_owned())]),
        body: json!({"title": "   "}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_TASK_SAVE_FAILED");
            assert_eq!(message, "task title is required");
        }
        other => panic!("expected 400 ADK_TASK_SAVE_FAILED, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:178 TestAssistantCatalogSessionAndObservabilitySuccessContracts
/// TestAssistantCatalogSessionAndObservabilitySuccessContracts.
///
/// Go seeds a provider, an enabled agent, a session, a loop run, a pending
/// approval, a task, a workspace memory, an optimization task and an audit
/// event, then requires the whole catalog / session / run / observability /
/// mutation success surface to answer `200`.  Rust reaches the same composed
/// state through the production port and asserts one success per route family
/// (the per-route detail is already pinned by
/// `catalog_session_run_and_observability_routes_answer_ok` for the read
/// surface and `adk_task_and_memory_crud_contracts_match_go` for mutations).
#[test]
fn adk_catalog_session_and_observability_success_contracts_hold() {
    let (port, _directory) = agent_validation_port();
    port.store
        .upsert_agent(
            "success-agent",
            r#"{"id":"success-agent","name":"Success Agent","providerId":"provider-enabled","status":"ENABLED","workMode":"loop"}"#,
        )
        .expect("seed agent");
    port.store
        .upsert_session(
            "success-session",
            "success-agent",
            r#"{"id":"success-session","agentId":"success-agent","title":"Success Session"}"#,
        )
        .expect("seed session");
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "success-run",
            session_id: "success-session",
            agent_id: "success-agent",
            status: "RUNNING",
            client_request_id: "success-run-request",
            request_fingerprint: "success-run-fingerprint",
            payload_json: r#"{"id":"success-run","sessionId":"success-session","agentId":"success-agent","status":"RUNNING","workMode":"loop","objective":"hold","toolCalls":[],"pendingApprovals":[]}"#,
        })
        .expect("seed run");
    port.store
        .create_approval(
            "success-approval",
            "success-run",
            "success-agent",
            "PENDING",
            r#"{"id":"success-approval","runId":"success-run","agentId":"success-agent","toolName":"contract.write","status":"PENDING"}"#,
        )
        .expect("seed approval");
    port.store
        .upsert_task(
            "success-task",
            "TODO",
            "success-agent",
            "success-run",
            r#"{"id":"success-task","title":"Do work","status":"TODO"}"#,
        )
        .expect("seed task");
    port.store
        .upsert_memory(
            "workspace-success-note",
            "",
            "workspace",
            "success-note",
            r#"{"id":"workspace-success-note","key":"success-note","value":"remember this","scope":"workspace"}"#,
        )
        .expect("seed memory");
    port.store
        .upsert_optimization_task(
            "success-optimization",
            r#"{"id":"success-optimization","status":"RUNNING","objective":"Improve returns"}"#,
        )
        .expect("seed optimization task");
    port.store
        .record_audit_event(
            "audit-success",
            "provider_saved",
            "provider-enabled",
            r#"{"id":"audit-success","detail":"saved"}"#,
        )
        .expect("seed audit event");

    for (path, query) in [
        ("/api/v1/adk", ""),
        ("/api/v1/adk/providers", ""),
        ("/api/v1/adk/agents", "status=ENABLED&limit=1&offset=0"),
        ("/api/v1/adk/skills", ""),
        (
            "/api/v1/adk/sessions",
            "agentId=success-agent&query=success",
        ),
        ("/api/v1/adk/sessions/success-session", ""),
        (
            "/api/v1/adk/runs",
            "status=RUNNING&agentId=success-agent&sessionId=success-session",
        ),
        ("/api/v1/adk/runs/success-run", ""),
        (
            "/api/v1/adk/approvals",
            "status=PENDING&agentId=success-agent",
        ),
        (
            "/api/v1/adk/tasks",
            "status=TODO&agentId=success-agent&runId=success-run",
        ),
        ("/api/v1/adk/tasks/success-task", ""),
        ("/api/v1/adk/memory", "scope=workspace&key=success-note"),
        (
            "/api/v1/adk/audit",
            "kind=provider_saved&subjectId=provider-enabled",
        ),
        ("/api/v1/adk/metrics", ""),
        ("/api/v1/adk/optimization-tasks", ""),
        ("/api/v1/adk/optimization-tasks/success-optimization", ""),
    ] {
        match port.read(path, query) {
            Ok(AdkReadSnapshot::Json(value)) => {
                assert!(
                    value.is_object(),
                    "GET {path}?{query} must answer a JSON object"
                );
            }
            other => panic!("GET {path}?{query} failed: {other:?}"),
        }
    }

    // Every mutation family in the Go success matrix answers a JSON body.
    for (operation, identifiers, body) in [
        (
            AdkMutationOperation::UpdateProvider,
            BTreeMap::from([("providerId".to_owned(), "provider-enabled".to_owned())]),
            json!({"displayName": "Updated Provider", "baseUrl": "https://example.test/v1", "model": "fixture-model", "enabled": true}),
        ),
        (
            AdkMutationOperation::UpdateAgent,
            BTreeMap::from([("agentId".to_owned(), "success-agent".to_owned())]),
            json!({"name": "Updated Agent", "providerId": "provider-enabled", "status": "ENABLED"}),
        ),
        (
            AdkMutationOperation::RenameSession,
            BTreeMap::from([("sessionId".to_owned(), "success-session".to_owned())]),
            json!({"title": "Renamed Session"}),
        ),
        (
            AdkMutationOperation::UpdateRunObjective,
            BTreeMap::from([("runId".to_owned(), "success-run".to_owned())]),
            json!({"objective": "new objective"}),
        ),
        (
            AdkMutationOperation::CreateTask,
            BTreeMap::new(),
            json!({"id": "created-task", "title": "Created Task", "status": "TODO", "agentId": "success-agent", "runId": "success-run"}),
        ),
        (
            AdkMutationOperation::UpdateTask,
            BTreeMap::from([("taskId".to_owned(), "success-task".to_owned())]),
            json!({"title": "Updated Task", "status": "IN_PROGRESS"}),
        ),
        (
            AdkMutationOperation::DeleteTask,
            BTreeMap::from([("taskId".to_owned(), "success-task".to_owned())]),
            Value::Null,
        ),
        (
            AdkMutationOperation::CreateMemory,
            BTreeMap::new(),
            json!({"key": "Created Note", "value": "created", "scope": "workspace"}),
        ),
        (
            AdkMutationOperation::DeleteMemory,
            BTreeMap::from([("memoryId".to_owned(), "workspace-success-note".to_owned())]),
            Value::Null,
        ),
        (
            AdkMutationOperation::DeleteProvider,
            BTreeMap::from([("providerId".to_owned(), "provider-disabled".to_owned())]),
            Value::Null,
        ),
        (
            AdkMutationOperation::DeleteSession,
            BTreeMap::from([("sessionId".to_owned(), "success-session".to_owned())]),
            Value::Null,
        ),
    ] {
        let result = port
            .mutate(&AdkMutationInput {
                operation,
                identifiers,
                body,
                webhook_secret: None,
            })
            .unwrap_or_else(|error| panic!("{operation:?} must succeed: {error:?}"));
        assert!(
            result.is_object(),
            "{operation:?} must answer a JSON object: {result}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/workflowexec/workflow_task_tools_persistence_test.go:13 TestWorkflowTaskToolsReturnParentPlanPersistenceFailures
/// Parity: go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:92 TestAssistantRoutesSurfaceStoreFailuresAfterRuntimeClose
/// TestAssistantRoutesSurfaceStoreFailuresAfterRuntimeClose.
///
/// Go closes the Assistant runtime while the router stays registered and then
/// requires every administrative route to surface the storage failure instead
/// of answering an apparently successful empty list.  Rust has no equivalent
/// "closed runtime" state - the composition root wires ports on startup - so
/// the same guarantee is pinned against the closest structural analogue: the
/// ADK schema is dropped after the port opened its connections and every route
/// family must fail closed (never `200 ok=true`).  The per-resource codes for
/// the catalog reads are pinned exactly by
/// `catalog_read_faults_expose_the_go_resource_error_codes`, and the agent
/// mutation code by `agent_save_storage_failure_is_not_client_classified`.
/// Go's `tasks.delete` / `memory.forget` tools are covered by seeded task and
/// memory rows whose deletes must surface the storage fault instead of a
/// fabricated "not found".
#[test]
fn adk_routes_surface_durable_store_failures_instead_of_empty_success() {
    let (port, directory) = unready_adk_port();
    port.store
        .upsert_agent(
            "agent-store-failure",
            r#"{"id":"agent-store-failure","name":"Store Failure","status":"ENABLED"}"#,
        )
        .expect("seed agent before the fault");
    port.store
        .upsert_session("session-store-failure", "agent-store-failure", "{}")
        .expect("seed session before the fault");
    port.store
        .upsert_provider(
            "provider-store-failure",
            r#"{"displayName":"Store Failure","baseUrl":"https://example.test/v1","model":"fixture-model","enabled":true}"#,
        )
        .expect("seed provider before the fault");
    port.store
        .upsert_task(
            "task-store-failure",
            "TODO",
            "agent-store-failure",
            "",
            r#"{"id":"task-store-failure","title":"Store Failure","status":"TODO"}"#,
        )
        .expect("seed task before the fault");
    port.store
        .upsert_memory(
            "workspace-store-failure",
            "agent-store-failure",
            "workspace",
            "store-failure-note",
            r#"{"id":"workspace-store-failure","key":"store-failure-note","value":"v"}"#,
        )
        .expect("seed memory before the fault");

    // Drop the tables the administrative reads depend on.  Reads and writes
    // both start failing at the store boundary from here on.
    let adk_path = directory.path().join("adk.db");
    let connection = rusqlite::Connection::open(&adk_path).expect("open ADK database");
    connection
        .execute_batch(
            "DROP TABLE adk_agents;
             DROP TABLE adk_providers;
             DROP TABLE adk_sessions;
             DROP TABLE adk_tasks;
             DROP TABLE adk_memory;
             DROP TABLE adk_runs;
             DROP TABLE adk_approvals;
             DROP TABLE adk_workflows;
             DROP TABLE adk_audit_events;
             DROP TABLE adk_optimization_tasks;
             DROP TABLE adk_workflow_trigger_logs;",
        )
        .expect("drop administrative ADK tables");
    drop(connection);

    for (path, query) in [
        ("/api/v1/adk", ""),
        ("/api/v1/adk/agents", ""),
        ("/api/v1/adk/providers", ""),
        ("/api/v1/adk/sessions", ""),
        ("/api/v1/adk/sessions/session-store-failure", ""),
        ("/api/v1/adk/runs", ""),
        ("/api/v1/adk/approvals", ""),
        ("/api/v1/adk/tasks", ""),
        ("/api/v1/adk/memory", ""),
        ("/api/v1/adk/audit", ""),
        ("/api/v1/adk/metrics", ""),
        ("/api/v1/adk/optimization-tasks", ""),
        ("/api/v1/adk/workflows", ""),
        ("/api/v1/adk/workflow-trigger-logs", ""),
    ] {
        match port.read(path, query) {
            Err(error) => {
                // Fail closed: never a fabricated success envelope.  The
                // concrete code is asserted by the per-resource witnesses.
                match error {
                    AdkReadSnapshotError::Unavailable(message)
                    | AdkReadSnapshotError::Failed { message, .. } => assert!(
                        !message.trim().is_empty(),
                        "GET {path} must carry a diagnostic message"
                    ),
                }
            }
            Ok(output) => {
                panic!("GET {path} must fail closed after the store fault, got {output:?}")
            }
        }
    }

    // Mutation families must fail closed as well rather than reporting a
    // fabricated success after the write target disappeared.
    for (operation, identifiers, body) in [
        (
            AdkMutationOperation::CreateAgent,
            BTreeMap::new(),
            json!({"id": "agent-after-fault", "name": "Agent After Fault"}),
        ),
        (
            AdkMutationOperation::DeleteAgent,
            BTreeMap::from([("agentId".to_owned(), "agent-store-failure".to_owned())]),
            Value::Null,
        ),
        (
            AdkMutationOperation::CreateTask,
            BTreeMap::new(),
            json!({"title": "Task After Fault"}),
        ),
        (
            AdkMutationOperation::CreateMemory,
            BTreeMap::new(),
            json!({"key": "after-fault", "value": "v", "scope": "workspace"}),
        ),
        (
            AdkMutationOperation::RenameSession,
            BTreeMap::from([("sessionId".to_owned(), "session-store-failure".to_owned())]),
            json!({"title": "Renamed After Fault"}),
        ),
        (
            AdkMutationOperation::CreateSession,
            BTreeMap::new(),
            json!({"agentId": "agent-store-failure", "title": "After Fault"}),
        ),
        (
            AdkMutationOperation::UpdateProvider,
            BTreeMap::from([("providerId".to_owned(), "provider-store-failure".to_owned())]),
            json!({"displayName": "After Fault"}),
        ),
    ] {
        match port.mutate(&AdkMutationInput {
            operation,
            identifiers,
            body,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed { .. })
            | Err(AdkMutationPortError::Unavailable(_)) => {}
            Ok(value) => {
                panic!("{operation:?} must fail closed after the store fault, got {value}")
            }
        }
    }

    // Go closes the Assistant store and then requires `tasks.delete` and
    // `memory.forget` to surface the closed storage.  Both rows exist before
    // the fault, so a delete may only answer the storage failure - never a
    // fabricated "not found" classification.
    for (operation, identifier_key, identifier, not_found_code) in [
        (
            AdkMutationOperation::DeleteTask,
            "taskId",
            "task-store-failure",
            "ADK_TASK_NOT_FOUND",
        ),
        (
            AdkMutationOperation::DeleteMemory,
            "memoryId",
            "workspace-store-failure",
            "ADK_MEMORY_NOT_FOUND",
        ),
    ] {
        match port.mutate(&AdkMutationInput {
            operation,
            identifiers: BTreeMap::from([(identifier_key.to_owned(), identifier.to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed { code, .. }) => assert_ne!(
                code, not_found_code,
                "{operation:?} must surface the storage fault for a seeded row"
            ),
            Err(AdkMutationPortError::Unavailable(message)) => assert!(
                !message.trim().is_empty(),
                "{operation:?} must carry a diagnostic message"
            ),
            Ok(value) => {
                panic!("{operation:?} must fail closed after the store fault, got {value}")
            }
        }
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:348 TestChatRequestUsesDeclaredMessageFieldOnly
/// TestChatRequestUsesDeclaredMessageFieldOnly.
///
/// Go decodes `ADKChatRequest` with its declared fields only, so the legacy
/// `prompt`/`text` aliases never populate `message`: a payload carrying only
/// those aliases is treated exactly like a blank message and rejected with
/// `400 ADK_CHAT_FAILED` / "message is required" before any run is created.
#[test]
fn adk_chat_request_uses_only_the_declared_message_field() {
    let (port, _directory) = ready_adk_port_with_fallback_provider();
    port.store
        .upsert_agent(
            "agent-message-field",
            &json!({
                "id": "agent-message-field",
                "name": "Message Field Agent",
                "providerId": "provider-ready",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");

    let counter = std::sync::atomic::AtomicU32::new(1);
    for legacy in [
        r#"{"agentId":"agent-message-field","prompt":"legacy"}"#,
        r#"{"agentId":"agent-message-field","text":"legacy-text"}"#,
        r#"{"agentId":"agent-message-field","prompt":"legacy","text":"legacy-text"}"#,
        // The declared field wins; the aliases are ignored even when present.
        r#"{"agentId":"agent-message-field","message":"   ","prompt":"legacy"}"#,
    ] {
        let sequence = counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        let error = port
            .dispatch(
                AdkChatRoute::Chat,
                &AdkChatInput {
                    body: legacy.as_bytes().to_vec(),
                    client_request_id: format!("4444444{sequence}-4444-4444-8444-444444444444"),
                },
            )
            .expect_err("a blank declared message must fail closed");
        match error {
            AdkChatPortError::Failed {
                status,
                code,
                message,
            } => {
                assert_eq!(status, 400, "payload {legacy}");
                assert_eq!(code, "ADK_CHAT_FAILED", "payload {legacy}");
                assert_eq!(message, "message is required", "payload {legacy}");
            }
            other => panic!("payload {legacy} produced {other:?}"),
        }
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:410 TestStreamReconnectAndSkillContracts
/// TestStreamReconnectAndSkillContracts.
///
/// Go's reconnect handlers stream retained history through
/// `streamADKChatRecord(..., replay=true)`, which stamps `replay:true` on the
/// frames the reconnecting client receives, and answer 404 for an unknown
/// stream or run.  The durable Rust read adapter owns both reconnect routes
/// (`GET /api/v1/adk/streams/{streamId}` and
/// `GET /api/v1/adk/runs/{runId}/stream`), so both must publish the marker and
/// fail closed on missing identifiers.
#[test]
fn adk_stream_reconnect_routes_carry_replay_markers_and_fail_closed() {
    let (port, _directory) = unready_adk_port();
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-reconnect",
            session_id: "session-reconnect",
            agent_id: "agent-reconnect",
            status: "RUNNING",
            client_request_id: "reconnect-request",
            request_fingerprint: "reconnect-fingerprint",
            payload_json: r#"{
                "id":"run-reconnect",
                "status":"RUNNING",
                "streamId":"run-reconnect",
                "streamEvents":[
                    {"type":"run","sequence":1,"streamId":"run-reconnect"},
                    {"type":"timeline","sequence":2,"streamId":"run-reconnect"}
                ]
            }"#,
        })
        .expect("seed streaming run");

    // `after=1` drops the first retained frame, matching Go's
    // `?after=` filter on the reconnect request.
    for path in [
        "/api/v1/adk/streams/run-reconnect",
        "/api/v1/adk/runs/run-reconnect/stream",
    ] {
        let snapshot = port
            .read(path, "after=1")
            .unwrap_or_else(|error| panic!("GET {path} failed: {error:?}"));
        let AdkReadSnapshot::Stream(stream) = snapshot else {
            panic!("GET {path} must stream retained events");
        };
        assert_eq!(
            stream.headers,
            vec![("X-ADK-Stream-ID".to_owned(), "run-reconnect".to_owned())],
            "GET {path} stream id header"
        );
        assert_eq!(stream.events.len(), 1, "GET {path} after=1 keeps event 2");
        assert_eq!(stream.events[0].id.as_deref(), Some("2"));
        assert_eq!(
            stream.events[0].data["replay"], true,
            "GET {path} retained history must carry the Go replay marker"
        );
    }

    for path in [
        "/api/v1/adk/streams/stream-missing",
        "/api/v1/adk/runs/run-missing/stream",
    ] {
        let error = port
            .read(path, "")
            .expect_err("missing stream must fail closed");
        assert!(
            error.to_string().contains("stream not found"),
            "GET {path} must fail closed: {error}"
        );
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_boundary_contracts_test.go:15 TestAssistantRoutesReturnUnavailableWhenRuntimeMissing
/// TestAssistantRoutesReturnUnavailableWhenRuntimeMissing.
///
/// Go registers every ADK route even when the runtime is nil and answers `503`
/// for all of them.  Rust's composition root gates registration on the port
/// being wired, so the equivalent guarantee is split: an unwired surface is
/// plainly not registered (404, covered by
/// `adk_read_routes_fail_closed_without_snapshot_port` and
/// `adk_chat_stream_routes_are_isolated_without_port`), and a *wired but
/// unavailable* surface must fail closed on every ADK route instead of
/// answering a success envelope.
#[test]
fn wired_but_unavailable_adk_ports_fail_closed_on_every_route() {
    #[derive(Debug)]
    struct UnavailableSnapshotPort;

    impl AdkReadSnapshotPort for UnavailableSnapshotPort {
        fn read(&self, _path: &str, _query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
            Err(AdkReadSnapshotError::Unavailable(
                "ADK runtime is unavailable".to_owned(),
            ))
        }
    }

    // Every read route in the Go matrix that Rust owns must answer 503 rather
    // than an unclassified error or a fabricated success.
    for path in [
        "/api/v1/adk",
        "/api/v1/adk/tools",
        "/api/v1/adk/audit",
        "/api/v1/adk/metrics",
        "/api/v1/adk/workflows",
        "/api/v1/adk/workflows/workflow-1",
        "/api/v1/adk/workflows/workflow-1/triggers",
        "/api/v1/adk/workflow-trigger-logs",
        "/api/v1/adk/tasks",
        "/api/v1/adk/tasks/task-1",
        "/api/v1/adk/memory",
        "/api/v1/adk/optimization-tasks",
        "/api/v1/adk/optimization-tasks/task-1",
        "/api/v1/adk/providers",
        "/api/v1/adk/agents",
        "/api/v1/adk/sessions",
        "/api/v1/adk/sessions/session-1",
        "/api/v1/adk/sessions/session-1/context",
        "/api/v1/adk/runs",
        "/api/v1/adk/runs/run-1",
        "/api/v1/adk/runs/run-1/stream",
        "/api/v1/adk/approvals",
        "/api/v1/adk/skills",
        "/api/v1/adk/streams/stream-1",
    ] {
        let failure =
            crate::product::dispatch_adk_read(Some(&UnavailableSnapshotPort), "GET", path, "")
                .expect_err("GET {path} must fail closed");
        assert_eq!(failure.status, 503, "GET {path}");
        assert_eq!(failure.code, "ADK_READ_UNAVAILABLE", "GET {path}");
    }

    // Mutations are the same shape: the port reports unavailable instead of
    // letting the handler synthesise a success envelope.
    #[derive(Debug)]
    struct UnavailableMutationPort;

    impl AdkMutationPort for UnavailableMutationPort {
        fn mutate(&self, _input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
            Err(AdkMutationPortError::Unavailable(
                "ADK runtime is unavailable".to_owned(),
            ))
        }
    }

    for (method, path, body) in [
        (
            "POST",
            "/api/v1/adk/workflows",
            r#"{"name":"Missing Runtime"}"#,
        ),
        (
            "POST",
            "/api/v1/adk/workflows/workflow-1/run",
            r#"{"symbol":"US.AAPL"}"#,
        ),
        ("POST", "/api/v1/adk/tasks", r#"{"title":"Task"}"#),
        (
            "POST",
            "/api/v1/adk/memory",
            r#"{"key":"note","value":"v"}"#,
        ),
        (
            "POST",
            "/api/v1/adk/providers",
            r#"{"displayName":"Provider"}"#,
        ),
        ("POST", "/api/v1/adk/agents", r#"{"name":"Agent"}"#),
        ("POST", "/api/v1/adk/sessions", r#"{"agentId":"agent-1"}"#),
        ("POST", "/api/v1/adk/approvals/approval-1/approve", ""),
        ("POST", "/api/v1/adk/skills", r#"{"source":"local"}"#),
    ] {
        let response = crate::product::product_adk_mutation_port::dispatch_adk_mutation(
            &crate::product::product_adk_mutation_port::AdkMutationRequest {
                method: method.to_owned(),
                path: path.to_owned(),
                body: (!body.is_empty()).then(|| body.as_bytes().to_vec()),
                headers: BTreeMap::new(),
            },
            Some(&UnavailableMutationPort),
            "2026-09-19T00:00:00Z",
        );
        assert_eq!(response.status, 503, "{method} {path}");
        assert_eq!(
            response.body["error"]["code"], "ADK_MUTATIONS_UNAVAILABLE",
            "{method} {path}"
        );
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:29 TestCatalogSessionRunAndObservabilityContracts
/// TestCatalogSessionRunAndObservabilityContracts.
///
/// Go seeds a provider, an enabled agent, a session, a completed run, one
/// audit event and one optimization task, then requires every catalog /
/// session / run / observability read route to answer `200 ok=true` - and the
/// provider delete route to succeed.  The Rust surface must reach the same
/// composed state through the production port rather than per-route stubs.
#[tokio::test]
async fn catalog_session_run_and_observability_routes_answer_ok() {
    let (port, directory) = unready_adk_port();
    let port = Arc::new(port);
    port.store
        .upsert_provider(
            "provider-disabled",
            &json!({"displayName": "Disabled", "enabled": false}).to_string(),
        )
        .expect("seed provider");
    port.store
        .upsert_agent(
            "agent-catalog",
            &json!({
                "id": "agent-catalog",
                "name": "Catalog Agent",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("seed agent");
    port.store
        .upsert_session("session-catalog", "agent-catalog", "{}")
        .expect("seed session");
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-contract",
            session_id: "session-catalog",
            agent_id: "agent-catalog",
            status: "COMPLETED",
            client_request_id: "catalog-run-request",
            request_fingerprint: "catalog-run-fingerprint",
            payload_json: r#"{"id":"run-contract","status":"COMPLETED","toolCalls":[],"pendingApprovals":[]}"#,
        })
        .expect("seed run");
    port.store
        .record_audit_event("audit-contract", "agent.saved", "agent-catalog", "{}")
        .expect("seed audit event");
    port.store
        .upsert_optimization_task(
            "optimization-contract",
            r#"{"id":"optimization-contract","status":"queued","objective":"return"}"#,
        )
        .expect("seed optimization task");

    for path in [
        "/api/v1/adk",
        "/api/v1/adk/providers",
        "/api/v1/adk/agents",
        "/api/v1/adk/skills",
        "/api/v1/adk/sessions",
        "/api/v1/adk/sessions/session-catalog",
        "/api/v1/adk/runs",
        "/api/v1/adk/runs/run-contract",
        "/api/v1/adk/audit",
        "/api/v1/adk/metrics",
        "/api/v1/adk/optimization-tasks",
        "/api/v1/adk/optimization-tasks/optimization-contract",
    ] {
        let output = port
            .read(path, "")
            .unwrap_or_else(|error| panic!("GET {path} failed: {error:?}"));
        match output {
            AdkReadSnapshot::Json(_) => {}
            AdkReadSnapshot::Stream(_) => panic!("GET {path} unexpectedly streamed"),
        }
    }

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteProvider,
        identifiers: BTreeMap::from([("providerId".to_owned(), "provider-disabled".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("DELETE provider must succeed");

    port.store
        .upsert_provider(
            "provider-disabled",
            &json!({"displayName": "Disabled", "enabled": false}).to_string(),
        )
        .expect("reseed provider for HTTP delete");

    let settings_path = directory.path().join("http-settings.json");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
    )
    .expect("config")
    .with_adk_read_snapshot_port(port.clone())
    .with_adk_mutation_port(port.clone());
    let handle = start_product(config).await.expect("start ADK product");
    for path in [
        "/api/v1/adk",
        "/api/v1/adk/providers",
        "/api/v1/adk/agents",
        "/api/v1/adk/skills",
        "/api/v1/adk/sessions",
        "/api/v1/adk/sessions/session-catalog",
        "/api/v1/adk/runs",
        "/api/v1/adk/runs/run-contract",
        "/api/v1/adk/audit",
        "/api/v1/adk/metrics",
        "/api/v1/adk/optimization-tasks",
        "/api/v1/adk/optimization-tasks/optimization-contract",
    ] {
        let (status, response) =
            request_json_with_status(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(status, 200, "GET {path}: {response}");
        assert_eq!(response["ok"], true, "GET {path}: {response}");
    }
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "DELETE",
        "/api/v1/adk/providers/provider-disabled",
        None,
    )
    .await;
    assert_eq!(status, 200, "DELETE provider: {response}");
    assert_eq!(response["ok"], true);
    handle.shutdown().await.expect("shutdown ADK product");
}

/// Parity: go:452dea11:internal/api/assistant/catalog_failure_contracts_test.go:17 TestCatalogReadFaultsExposeStableAPIContracts
/// TestCatalogReadFaultsExposeStableAPIContracts.
///
/// Go drops one ADK table at a time while the runtime stays up and requires
/// every administrative catalog read to keep its own error code instead of
/// masquerading as an empty list: `500 ADK_TASK_LIST_FAILED`,
/// `400 ADK_MEMORY_LIST_FAILED`, `500 ADK_AGENT_LIST_FAILED` and
/// `500 ADK_PROVIDER_LIST_FAILED`.  The Rust port previously folded all four
/// durable faults into the transport-level `503 ADK_READ_UNAVAILABLE`, which
/// hid a damaged database behind an unchanged-runtime signal.
#[test]
fn catalog_read_faults_expose_the_go_resource_error_codes() {
    for (table, path, status, code) in [
        (
            "adk_tasks",
            "/api/v1/adk/tasks",
            500,
            "ADK_TASK_LIST_FAILED",
        ),
        (
            "adk_memory",
            "/api/v1/adk/memory",
            400,
            "ADK_MEMORY_LIST_FAILED",
        ),
        (
            "adk_agents",
            "/api/v1/adk/agents",
            500,
            "ADK_AGENT_LIST_FAILED",
        ),
        (
            "adk_providers",
            "/api/v1/adk/providers",
            500,
            "ADK_PROVIDER_LIST_FAILED",
        ),
    ] {
        let (port, directory) = unready_adk_port();
        let adk_path = directory.path().join("adk.db");
        // Schema faults are injected after the port opened its connection,
        // matching an operator serving a damaged or partially migrated
        // database.
        let connection = rusqlite::Connection::open(&adk_path).expect("open ADK database");
        connection
            .execute_batch(&format!("DROP TABLE {table};"))
            .unwrap_or_else(|error| panic!("drop {table}: {error}"));
        drop(connection);

        match port.read(path, "") {
            Err(AdkReadSnapshotError::Failed {
                status: actual_status,
                code: actual_code,
                ..
            }) => {
                assert_eq!(actual_status, status, "path {path}");
                assert_eq!(actual_code, code, "path {path}");
            }
            other => panic!("expected {status} {code} for {path}, got {other:?}"),
        }
    }

    // The invalid-scope branch shares the memory listing code instead of the
    // generic `BAD_REQUEST` it used to return.
    let (port, _directory) = unready_adk_port();
    match port.read("/api/v1/adk/memory", "scope=private") {
        Err(AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_MEMORY_LIST_FAILED");
            assert_eq!(message, "memory scope must be workspace or agent");
        }
        other => panic!("expected 400 ADK_MEMORY_LIST_FAILED, got {other:?}"),
    }
}
/// Go's `StoreCore.DeleteAgent` is a soft delete: the row stays addressable,
/// `status` becomes `DISABLED` and `deletedAt` is stamped instead of removing
/// the SQLite row.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestDeleteAgentSoftDeletesHistoricalRecord`.
/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:294
/// `TestDeleteAgentSoftDeletesHistoricalRecord`.
#[test]
fn adk_agent_delete_soft_deletes_the_historical_row() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body: json!({"id": "agent-soft-delete", "name": "Soft Delete", "status": "ENABLED"}),
        webhook_secret: None,
    })
    .expect("create agent");

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteAgent,
        identifiers: BTreeMap::from([("agentId".to_owned(), "agent-soft-delete".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete agent");

    let stored = store
        .get_agent("agent-soft-delete")
        .expect("read historical row")
        .expect("a soft-deleted row must stay addressable");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("agent payload");
    assert_eq!(payload["status"], "DISABLED");
    assert!(
        payload["deletedAt"]
            .as_str()
            .is_some_and(|value| !value.trim().is_empty()),
        "deletedAt must be stamped: {payload}"
    );
    assert_eq!(
        store.list_agents().expect("raw store listing").len(),
        1,
        "the row is kept for history rather than removed"
    );
}

/// Go's `StoreCore.ListAgents` drops every `deletedAt != nil` row while
/// `ListAllAgents` keeps it, so the active catalogue and the historical view
/// diverge by exactly the soft-deleted records.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestListAgentsExcludesSoftDeletedWhileListAllIncludesThem`.
/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:320
/// `TestListAgentsExcludesSoftDeletedWhileListAllIncludesThem`.
#[test]
fn adk_agent_listing_excludes_soft_deleted_rows_but_keeps_history() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    for (id, name) in [
        ("agent-older", "Older Agent"),
        ("agent-newer", "Newer Agent"),
    ] {
        port.mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({"id": id, "name": name, "status": "ENABLED"}),
            webhook_secret: None,
        })
        .unwrap_or_else(|error| panic!("create {id}: {error:?}"));
    }

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteAgent,
        identifiers: BTreeMap::from([("agentId".to_owned(), "agent-older".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete agent-older");

    let active = port
        .read("/api/v1/adk/agents", "")
        .expect("list active agents");
    let AdkReadSnapshot::Json(active) = active else {
        panic!("agents listing must be JSON");
    };
    let active_ids = active["agents"]
        .as_array()
        .expect("agents array")
        .iter()
        .filter_map(|agent| agent["id"].as_str())
        .collect::<Vec<_>>();
    assert!(
        active_ids.contains(&"agent-newer"),
        "the active agent stays listed: {active}"
    );
    assert!(
        !active_ids.contains(&"agent-older"),
        "soft-deleted agent must leave the active list: {active}"
    );

    // The historical view (raw store listing) still carries both rows with the
    // delete marker preserved.
    let historical = store.list_agents().expect("historical listing");
    assert_eq!(historical.len(), 2, "history keeps the deleted row");
    let deleted = historical
        .iter()
        .find(|row| row.id == "agent-older")
        .expect("deleted row present");
    let deleted_payload: Value =
        serde_json::from_str(&deleted.payload_json).expect("deleted payload");
    assert_eq!(deleted_payload["status"], "DISABLED");
    assert!(deleted_payload["deletedAt"].is_string());
}

/// A later save of the same id restores the record: the rebuilt payload drops
/// `deletedAt`, so the agent reappears in the active catalogue.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestSaveAgentRestoresDeletedAgentRecord`.
/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:363
/// `TestSaveAgentRestoresDeletedAgentRecord`.
#[test]
fn adk_agent_save_restores_a_soft_deleted_row() {
    let (port, _store, _directory) = setup_test_adk_mutation_port(None);
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body: json!({"id": "agent-restore", "name": "Agent", "status": "ENABLED"}),
        webhook_secret: None,
    })
    .expect("create agent");
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteAgent,
        identifiers: BTreeMap::from([("agentId".to_owned(), "agent-restore".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete agent");

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateAgent,
        identifiers: BTreeMap::new(),
        body: json!({"id": "agent-restore", "name": "Agent Restored", "status": "ENABLED"}),
        webhook_secret: None,
    })
    .expect("save restores the soft-deleted agent");

    let restored = port
        .read("/api/v1/adk/agents", "")
        .expect("list restored agents");
    let AdkReadSnapshot::Json(restored) = restored else {
        panic!("agents listing must be JSON");
    };
    let restored_agent = restored["agents"]
        .as_array()
        .expect("agents array")
        .iter()
        .find(|agent| agent["id"] == "agent-restore")
        .unwrap_or_else(|| panic!("restored agent must reappear: {restored}"));
    assert_eq!(restored_agent["name"], "Agent Restored");
    assert!(
        restored_agent.get("deletedAt").is_none_or(Value::is_null),
        "restore must drop the delete marker: {restored_agent}"
    );
}
/// Go's `Runtime.CancelRun` -> `cancelRunTree` persists the cancelled run and
/// denies every still-pending approval of that run in the same write
/// (`StoreCore.SaveRunAndDenyPendingApprovals`).  The Rust cancel route used to
/// write only the run row, leaving `PENDING` approvals resolvable against a
/// terminal run.
///
/// Reference: go:452dea11:internal/assistant/engine/store_ops_test.go
/// `TestCancelPendingRunDeniesApprovals`.
/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:402
/// `TestCancelPendingRunDeniesApprovals`.
#[test]
fn cancelling_a_run_denies_its_pending_approvals() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-cancel-approval",
            session_id: "session-cancel",
            agent_id: "agent-cancel",
            status: "PENDING",
            client_request_id: "request-cancel-approval",
            request_fingerprint: "fingerprint-cancel-approval",
            payload_json: &json!({
                "id": "run-cancel-approval",
                "sessionId": "session-cancel",
                "agentId": "agent-cancel",
                "status": "PENDING",
                "pendingApprovals": [{
                    "id": "approval-cancel",
                    "runId": "run-cancel-approval",
                    "agentId": "agent-cancel",
                    "status": "PENDING",
                }],
            })
            .to_string(),
        })
        .expect("seed pending run");
    store
        .create_approval(
            "approval-cancel",
            "run-cancel-approval",
            "agent-cancel",
            "PENDING",
            &json!({
                "id": "approval-cancel",
                "runId": "run-cancel-approval",
                "agentId": "agent-cancel",
                "status": "PENDING",
            })
            .to_string(),
        )
        .expect("seed pending approval");

    let cancelled = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CancelRun,
            identifiers: BTreeMap::from([("runId".to_owned(), "run-cancel-approval".to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("cancel the pending run");
    assert_eq!(cancelled["status"], "CANCELLED");
    assert!(
        cancelled["cancelledAt"]
            .as_str()
            .is_some_and(|value| !value.trim().is_empty()),
        "cancelledAt must be stamped: {cancelled}"
    );

    // The durable approval row is denied, not just dropped from the payload.
    let approval = store
        .list_approvals()
        .expect("list approvals")
        .into_iter()
        .find(|approval| approval.id == "approval-cancel")
        .expect("approval row survives");
    assert_eq!(
        approval.status, "DENIED",
        "a cancelled run must deny its pending approvals"
    );
    let approval_payload: Value =
        serde_json::from_str(&approval.payload_json).expect("approval payload");
    assert_eq!(approval_payload["status"], "DENIED");

    // Resolving the denied approval must stay a no-op and never resurrect the
    // cancelled run.
    let resolution = store
        .resolve_and_stage_approval("approval-cancel", "APPROVED")
        .expect("resolve denied approval")
        .expect("resolution row present");
    assert!(
        !resolution.changed,
        "a denied approval cannot be re-approved"
    );
    assert_eq!(resolution.approval.status, "DENIED");
    let stored_run = store
        .get_run("run-cancel-approval")
        .expect("read cancelled run")
        .expect("run present");
    assert_eq!(stored_run.status, "CANCELLED");
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:844
/// TestADKTaskUpdateDeleteAndValidation.
///
/// Go's `SaveTask` funnels every write through `NormalizeTaskDependsOn` /
/// `NormalizeStringSlice`, which trims, drops blanks, deduplicates and sorts;
/// the same normalization applies to `PlannerWarnings` on create and patch.
/// The Rust port stored the caller's list verbatim, so `["task-b","task-b"]`
/// stayed duplicated and unsorted and a blank member was rejected instead of
/// dropped.
#[test]
fn adk_task_normalization_and_validation_match_go() {
    let (port, _directory) = unready_adk_port();
    port.store
        .upsert_agent(
            "agent-normalize",
            r#"{"id":"agent-normalize","name":"Normalize Agent","status":"ENABLED"}"#,
        )
        .expect("seed agent");

    let created = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateTask,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": "task-normalize",
                "title": "  Normalize  ",
                "status": "todo",
                "agentId": "agent-normalize",
                "dependsOn": ["task-z", "task-a", "task-z", "  "],
                "order": 2,
                "modeHint": "loop",
                "agentRole": "实现 Agent",
                "plannerStepId": "__planner_step_2",
                "planSource": "planner",
                "workflowMode": "loop",
                "objective": "完成目标",
                "plannerWarnings": ["警告 B", "警告 A", "警告 A"],
            }),
            webhook_secret: None,
        })
        .expect("create task");
    assert_eq!(created["status"], "TODO", "status normalizes to upper case");
    assert_eq!(
        created["dependsOn"],
        json!(["task-a", "task-z"]),
        "dependencies are trimmed, deduplicated and sorted: {created}"
    );
    assert_eq!(
        created["plannerWarnings"],
        json!(["警告 A", "警告 B"]),
        "planner warnings use the same normalization: {created}"
    );
    assert_eq!(created["order"], 2);
    assert_eq!(created["modeHint"], "loop");
    assert_eq!(created["agentRole"], "实现 Agent");
    assert_eq!(created["plannerStepId"], "__planner_step_2");
    assert_eq!(created["planSource"], "planner");
    assert_eq!(created["workflowMode"], "loop");
    assert_eq!(created["objective"], "完成目标");

    let patched = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateTask,
            identifiers: BTreeMap::from([("taskId".to_owned(), "task-normalize".to_owned())]),
            body: json!({
                "description": "kept details",
                "status": "in_progress",
                "order": 3,
                "agentRole": "验证 Agent",
                "plannerWarnings": ["planner warning"],
            }),
            webhook_secret: None,
        })
        .expect("patch task");
    assert_eq!(patched["title"], "Normalize", "patch preserves the title");
    assert_eq!(patched["description"], "kept details");
    assert_eq!(patched["status"], "IN_PROGRESS");
    assert_eq!(patched["order"], 3);
    assert_eq!(patched["agentRole"], "验证 Agent");
    assert_eq!(patched["plannerWarnings"], json!(["planner warning"]));

    // Invalid status and self-dependency are the frozen `SaveTask` failures.
    for (body, expected) in [
        (
            json!({"id": "bad-status", "title": "Bad", "status": "NOPE"}),
            "invalid task status",
        ),
        (
            json!({"id": "self", "title": "Self", "dependsOn": ["self"]}),
            "task cannot depend on itself",
        ),
    ] {
        match port.mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateTask,
            identifiers: BTreeMap::new(),
            body,
            webhook_secret: None,
        }) {
            Err(AdkMutationPortError::Failed {
                status, message, ..
            }) => {
                assert_eq!(status, 400, "{expected}");
                assert!(
                    message.contains(expected),
                    "message {message:?} must contain {expected:?}"
                );
            }
            other => panic!("expected 400 {expected}, got {other:?}"),
        }
    }

    // The filtered page narrows by the normalized status and agent.
    let AdkReadSnapshot::Json(listed) = port
        .read(
            "/api/v1/adk/tasks",
            "status=IN_PROGRESS&agentId=agent-normalize",
        )
        .expect("task list")
    else {
        panic!("task list must answer JSON");
    };
    let tasks = listed["tasks"].as_array().expect("tasks array");
    assert_eq!(tasks.len(), 1, "one IN_PROGRESS task: {listed}");
    assert_eq!(tasks[0]["id"], "task-normalize");

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteTask,
        identifiers: BTreeMap::from([("taskId".to_owned(), "task-normalize".to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete task");
    assert!(
        port.store
            .get_task("task-normalize")
            .expect("read deleted task")
            .is_none(),
        "a deleted task is gone"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:899
/// TestADKMemoryFiltersDeleteAndAgentValidation.
///
/// Go requires an `agentId` for `scope=agent` (`400 ADK_MEMORY_SAVE_FAILED`),
/// normalizes memory keys to lower case, keeps workspace rows in a per-agent
/// listing alongside that agent's own rows, and makes a deleted row vanish.
#[test]
fn adk_memory_filters_and_agent_validation_match_go() {
    let (port, _directory) = unready_adk_port();
    port.store
        .upsert_agent(
            "agent-normalize",
            r#"{"id":"agent-normalize","name":"Normalize Agent","status":"ENABLED"}"#,
        )
        .expect("seed agent");

    // Memory: workspace + agent scoping, the agentId requirement and delete.
    let workspace = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateMemory,
            identifiers: BTreeMap::new(),
            body: json!({"scope": "workspace", "key": "Market", "value": "HK"}),
            webhook_secret: None,
        })
        .expect("save workspace memory");
    let workspace_id = workspace["id"].as_str().expect("memory id").to_owned();
    assert_eq!(workspace["key"], "market", "memory keys normalize");
    let agent_entry = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateMemory,
            identifiers: BTreeMap::new(),
            body: json!({
                "scope": "agent",
                "agentId": "agent-normalize",
                "key": "Style",
                "value": "risk first",
            }),
            webhook_secret: None,
        })
        .expect("save agent memory");
    let agent_entry_id = agent_entry["id"].as_str().expect("memory id").to_owned();
    assert_eq!(agent_entry["key"], "style");

    match port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::CreateMemory,
        identifiers: BTreeMap::new(),
        body: json!({"scope": "agent", "key": "missing", "value": "bad"}),
        webhook_secret: None,
    }) {
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_MEMORY_SAVE_FAILED");
            assert_eq!(message, "agent memory requires agentId");
        }
        other => panic!("expected 400 ADK_MEMORY_SAVE_FAILED, got {other:?}"),
    }

    let AdkReadSnapshot::Json(filtered) = port
        .read(
            "/api/v1/adk/memory",
            "scope=agent&agentId=agent-normalize&key=style",
        )
        .expect("filtered memory list")
    else {
        panic!("memory list must answer JSON");
    };
    let entries = filtered["entries"].as_array().expect("entries array");
    assert_eq!(entries.len(), 1, "only the agent style entry: {filtered}");
    assert_eq!(entries[0]["id"], agent_entry_id.as_str());

    let AdkReadSnapshot::Json(prompt_entries) = port
        .read("/api/v1/adk/memory", "agentId=agent-normalize")
        .expect("prompt memory list")
    else {
        panic!("memory list must answer JSON");
    };
    let entries = prompt_entries["entries"].as_array().expect("entries array");
    assert_eq!(
        entries.len(),
        2,
        "an agent-scoped listing keeps workspace plus its own rows: {prompt_entries}"
    );
    assert!(
        entries
            .iter()
            .any(|entry| entry["id"] == workspace_id.as_str())
    );
    assert!(
        entries
            .iter()
            .any(|entry| entry["id"] == agent_entry_id.as_str())
    );

    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::DeleteMemory,
        identifiers: BTreeMap::from([("memoryId".to_owned(), workspace_id.clone())]),
        body: Value::Null,
        webhook_secret: None,
    })
    .expect("delete memory");
    assert!(
        port.store
            .get_memory(&workspace_id)
            .expect("read deleted memory")
            .is_none(),
        "a deleted memory entry is gone"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:788
/// TestMultipleApprovalsExecuteOnlyAfterAllApproved.
///
/// A run with two pending approvals stays `PENDING` until *both* are approved:
/// the first approval resolves and stages without continuing, the second one
/// flips the run to `RUNNING`/`approval_resuming` with both tool calls
/// released.
#[test]
fn adk_multiple_approvals_continue_only_after_all_are_approved() {
    let (_port, store, _directory) = setup_test_adk_mutation_port(None);
    store
        .upsert_agent(
            "agent-approvals",
            &json!({
                "id": "agent-approvals",
                "name": "Approval Agent",
                "providerId": "provider-approvals",
                "status": "ENABLED",
                "permissionMode": "approval",
            })
            .to_string(),
        )
        .expect("seed agent");
    let run_id = "run-approvals";
    let payload = json!({
        "id": run_id,
        "sessionId": "session-approvals",
        "agentId": "agent-approvals",
        "status": "PENDING",
        "workMode": "chat",
        "toolCalls": [
            {"id": "call-one", "name": "approval.required.one", "status": "PENDING_APPROVAL", "requiresUser": true},
            {"id": "call-two", "name": "approval.required.two", "status": "PENDING_APPROVAL", "requiresUser": true},
        ],
        "pendingApprovals": [
            {
                "id": "approval-one",
                "runId": run_id,
                "agentId": "agent-approvals",
                "toolName": "approval.required.one",
                "status": "PENDING",
                "functionCallId": "call-one",
                "confirmationCallId": "call-one:confirmation",
            },
            {
                "id": "approval-two",
                "runId": run_id,
                "agentId": "agent-approvals",
                "toolName": "approval.required.two",
                "status": "PENDING",
                "functionCallId": "call-two",
                "confirmationCallId": "call-two:confirmation",
            },
        ],
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-approvals",
            agent_id: "agent-approvals",
            status: "PENDING",
            client_request_id: "request-approvals",
            request_fingerprint: "fingerprint-approvals",
            payload_json: &payload.to_string(),
        })
        .expect("seed approval run");
    for (id, call_id, tool) in [
        ("approval-one", "call-one", "approval.required.one"),
        ("approval-two", "call-two", "approval.required.two"),
    ] {
        store
            .create_approval(
                id,
                run_id,
                "agent-approvals",
                "PENDING",
                &json!({
                    "id": id,
                    "runId": run_id,
                    "agentId": "agent-approvals",
                    "toolName": tool,
                    "status": "PENDING",
                    "functionCallId": call_id,
                    "confirmationCallId": format!("{call_id}:confirmation"),
                })
                .to_string(),
            )
            .expect("seed approval");
    }

    // First approval resolves, but the run must stay PENDING while the sibling
    // is still unresolved.
    let first = store
        .resolve_and_stage_approval("approval-one", "APPROVED")
        .expect("resolve first approval")
        .expect("first resolution");
    assert!(first.changed, "the first resolution commits");
    assert!(
        !first.should_continue,
        "a run with an unresolved sibling must not continue: {first:?}"
    );
    let staged = first.run.as_ref().expect("staged run");
    assert_eq!(staged.status, "PENDING");
    let staged_payload: Value = serde_json::from_str(&staged.payload_json).expect("run payload");
    assert_eq!(
        staged_payload["toolCalls"][0]["status"], "PENDING_APPROVAL",
        "a waiting sibling keeps its tool call gated: {staged_payload}"
    );

    // The second approval releases the continuation: the run flips to RUNNING
    // with `approval_resuming` and both tool calls become RUNNING.
    let second = store
        .resolve_and_stage_approval("approval-two", "APPROVED")
        .expect("resolve second approval")
        .expect("second resolution");
    assert!(second.changed);
    assert!(
        second.should_continue,
        "the last approval releases the continuation: {second:?}"
    );
    let resumed = second.run.as_ref().expect("resumed run");
    assert_eq!(resumed.status, "RUNNING");
    let resumed_payload: Value = serde_json::from_str(&resumed.payload_json).expect("run payload");
    assert_eq!(resumed_payload["resumeState"], "approval_resuming");
    for (index, call) in resumed_payload["toolCalls"]
        .as_array()
        .expect("tool calls")
        .iter()
        .enumerate()
    {
        assert_eq!(call["status"], "RUNNING", "call {index} was released");
        assert_eq!(
            call["requiresUser"], false,
            "call {index} clears requiresUser"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:136
/// `TestResolvedApprovalContinuationKeepsSiblingStateAtomic`.
///
/// Go keeps the run `PENDING` while an approved sibling is still unresolved,
/// denies every still-pending sibling when one approval is denied, refuses to
/// let an approval that is not embedded in the run's own `pendingApprovals`
/// replace the durable projection, and never restarts a terminal run from an
/// old approval.  The Rust store owns all four rules so the route layer only
/// forwards the resolution.
#[test]
fn adk_denied_approval_closes_siblings_and_unrelated_resolution_keeps_the_projection() {
    let (_port, store, _directory) = setup_test_adk_mutation_port(None);
    let run_id = "run-approval-siblings-deny";
    let payload = json!({
        "id": run_id,
        "sessionId": "session-approval-siblings",
        "agentId": "agent-approval-siblings",
        "status": "PENDING",
        "workMode": "chat",
        "toolCalls": [
            {"id": "call-sib-one", "name": "approval.required.one", "status": "PENDING_APPROVAL", "requiresUser": true},
            {"id": "call-sib-two", "name": "approval.required.two", "status": "PENDING_APPROVAL", "requiresUser": true},
        ],
        "pendingApprovals": [
            {
                "id": "approval-sib-one",
                "runId": run_id,
                "agentId": "agent-approval-siblings",
                "toolName": "approval.required.one",
                "status": "PENDING",
                "functionCallId": "call-sib-one",
                "confirmationCallId": "call-sib-one:confirmation",
            },
            {
                "id": "approval-sib-two",
                "runId": run_id,
                "agentId": "agent-approval-siblings",
                "toolName": "approval.required.two",
                "status": "PENDING",
                "functionCallId": "call-sib-two",
                "confirmationCallId": "call-sib-two:confirmation",
            },
        ],
    });
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-approval-siblings",
            agent_id: "agent-approval-siblings",
            status: "PENDING",
            client_request_id: "request-approval-siblings",
            request_fingerprint: "fingerprint-approval-siblings",
            payload_json: &payload.to_string(),
        })
        .expect("seed sibling approval run");
    for (id, call_id, tool) in [
        ("approval-sib-one", "call-sib-one", "approval.required.one"),
        ("approval-sib-two", "call-sib-two", "approval.required.two"),
    ] {
        store
            .create_approval(
                id,
                run_id,
                "agent-approval-siblings",
                "PENDING",
                &json!({
                    "id": id,
                    "runId": run_id,
                    "agentId": "agent-approval-siblings",
                    "toolName": tool,
                    "status": "PENDING",
                    "functionCallId": call_id,
                    "confirmationCallId": format!("{call_id}:confirmation"),
                })
                .to_string(),
            )
            .expect("seed sibling approval");
    }
    // An approval row that belongs to the run but is not embedded in its
    // `pendingApprovals` must never replace the durable projection.
    store
        .create_approval(
            "approval-sib-orphan",
            run_id,
            "agent-approval-siblings",
            "PENDING",
            &json!({
                "id": "approval-sib-orphan",
                "runId": run_id,
                "agentId": "agent-approval-siblings",
                "toolName": "approval.required.one",
                "status": "PENDING",
            })
            .to_string(),
        )
        .expect("seed orphan approval");

    // Denying one approval closes every still-pending sibling in the same
    // write: the sibling row and its tool call both become DENIED.
    let denied = store
        .resolve_and_stage_approval("approval-sib-one", "DENIED")
        .expect("deny the first approval")
        .expect("first resolution");
    assert!(denied.changed, "the denial commits");
    assert_eq!(denied.approval.status, "DENIED");
    let staged = denied.run.as_ref().expect("staged run");
    let staged_payload: Value = serde_json::from_str(&staged.payload_json).expect("run payload");
    for (index, call) in staged_payload["toolCalls"]
        .as_array()
        .expect("tool calls")
        .iter()
        .enumerate()
    {
        assert_eq!(
            call["status"], "DENIED",
            "call {index} was closed by the denial"
        );
        assert_eq!(
            call["requiresUser"], false,
            "call {index} clears requiresUser"
        );
    }
    let sibling = store
        .list_approvals()
        .expect("list approvals")
        .into_iter()
        .find(|approval| approval.id == "approval-sib-two")
        .expect("sibling row survives");
    assert_eq!(
        sibling.status, "DENIED",
        "a denial closes still-pending siblings before the continuation"
    );

    // An approval that is not embedded in `pendingApprovals` is dropped
    // instead of rewriting the run's own projection.
    let untouched_before = store
        .get_run(run_id)
        .expect("read run")
        .expect("run row")
        .payload_json;
    let unrelated = store
        .resolve_and_stage_approval("approval-sib-orphan", "APPROVED")
        .expect("resolve the orphan approval")
        .expect("orphan resolution");
    assert!(
        !unrelated.should_continue,
        "an unrelated approval must not release the continuation"
    );
    assert_eq!(
        store
            .get_run(run_id)
            .expect("read run")
            .expect("run row")
            .payload_json,
        untouched_before,
        "an unrelated approval must not replace the embedded projection"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:527
/// TestDuplicateApprovalResolutionDoesNotExecuteTwice.
///
/// Go resolves an approval through the store CAS before waking the
/// continuation, so a browser retry (or a duplicate worker) observing an
/// already-resolved row must be a pure no-op: `changed` stays false, the run
/// is not staged a second time, and the release happens exactly once.  The
/// route layer answers the same way for both the duplicate and the first
/// resolution.
#[test]
fn adk_duplicate_approval_resolution_is_a_noop() {
    #[derive(Debug, Default)]
    struct CountingContinuationRuntime {
        resumed: std::sync::Mutex<Vec<String>>,
    }

    impl AdkChatStreamPort for CountingContinuationRuntime {
        fn dispatch(
            &self,
            _: AdkChatRoute,
            _: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
        }
        fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
            self.resumed
                .lock()
                .expect("resume log")
                .push(run_id.to_owned());
            Ok(())
        }
        fn runtime_ready(&self) -> bool {
            true
        }
    }

    let runtime = Arc::new(CountingContinuationRuntime::default());
    let (port, store, _directory) =
        setup_test_adk_mutation_port(Some(runtime.clone() as Arc<dyn AdkChatStreamPort>));
    let (run_id, approval_id) =
        seed_pending_approval_rows(&store, "duplicate", "call-duplicate", "contract.write");

    for attempt in 0..2 {
        let resolution = port
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::Approve,
                identifiers: BTreeMap::from([("approvalId".to_owned(), approval_id.clone())]),
                body: Value::Null,
                webhook_secret: None,
            })
            .unwrap_or_else(|error| panic!("approve attempt {attempt}: {error:?}"));
        assert_eq!(resolution["approval"]["status"], "APPROVED");
    }

    let resumed = runtime.resumed.lock().expect("resume log").clone();
    assert_eq!(
        resumed,
        vec![run_id.clone()],
        "the continuation is woken exactly once across both approve calls"
    );
    let stored = store
        .get_run(&run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["status"], "RUNNING");
    assert_eq!(
        payload["toolCalls"]
            .as_array()
            .expect("tool calls")
            .iter()
            .filter(|call| call["status"] == "RUNNING")
            .count(),
        1,
        "the released tool call is staged once: {payload}"
    );
    assert!(
        payload["pendingApprovals"]
            .as_array()
            .expect("pending approvals")
            .iter()
            .all(|approval| approval["status"] == "APPROVED"),
        "a duplicate resolution cannot resurrect the pending row: {payload}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:486
/// TestListRunsPageFiltersAndSortsNewestFirst.
///
/// Go's `ListRunsPage` filters by status/agent/session and orders
/// `created_at DESC, id ASC`, so the newest matching run is first; the filtered
/// total and the returned page agree with the request.
#[test]
fn adk_run_listing_filters_and_sorts_newest_first() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    for (id, session, status, created_at) in [
        ("run-older", "session-a", "FAILED", "2024-01-01T00:00:00Z"),
        ("run-newer", "session-a", "FAILED", "2024-01-02T00:00:00Z"),
        (
            "run-other-session",
            "session-b",
            "FAILED",
            "2024-01-03T00:00:00Z",
        ),
        (
            "run-other-status",
            "session-a",
            "COMPLETED",
            "2024-01-04T00:00:00Z",
        ),
    ] {
        let payload = json!({
            "id": id,
            "sessionId": session,
            "agentId": "agent-a",
            "status": status,
            "createdAt": created_at,
            "updatedAt": created_at,
        });
        store
            .create_run(jftrade_store_sqlite::CreateAdkRunParams {
                id,
                session_id: session,
                agent_id: "agent-a",
                status,
                client_request_id: &format!("request-{id}"),
                request_fingerprint: &format!("fingerprint-{id}"),
                payload_json: &payload.to_string(),
            })
            .expect("seed run");
    }

    let AdkReadSnapshot::Json(listed) = port
        .read(
            "/api/v1/adk/runs",
            "status=FAILED&agentId=agent-a&sessionId=session-a",
        )
        .expect("filtered run list")
    else {
        panic!("run list must answer JSON");
    };
    let runs = listed["runs"].as_array().expect("runs array");
    let ids = runs
        .iter()
        .filter_map(|run| run["id"].as_str())
        .collect::<Vec<_>>();
    assert_eq!(
        ids,
        vec!["run-newer", "run-older"],
        "the store query narrows to the requested session and orders newest first: {listed}"
    );
    assert_eq!(listed["page"]["total"], 2);
    assert_eq!(listed["page"]["returned"], 2);
    assert_eq!(listed["page"]["hasMore"], false);

    // The store-level page applies the same filter/order contract.
    let page = store.list_runs().expect("list runs");
    let filtered = page
        .iter()
        .filter(|run| run.status == "FAILED" && run.session_id == "session-a")
        .map(|run| run.id.as_str())
        .collect::<Vec<_>>();
    assert_eq!(
        filtered,
        vec!["run-newer", "run-older"],
        "store listing keeps newest-first order"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:426
/// TestCancelRunMissingReturnsNotFound.
///
/// Go's `Runtime.CancelRun` reports a missing run as "run not found"; the
/// cancel route wraps that into `404 ADK_RUN_CANCEL_FAILED` while the read
/// route keeps the generic `404 NOT_FOUND`.
#[test]
fn adk_cancel_run_missing_is_the_dedicated_cancel_failure() {
    let (port, _directory) = unready_adk_port();

    let error = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CancelRun,
            identifiers: BTreeMap::from([("runId".to_owned(), "run-missing".to_owned())]),
            body: json!({}),
            webhook_secret: None,
        })
        .expect_err("cancelling a missing run must fail");
    match error {
        AdkMutationPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_RUN_CANCEL_FAILED");
            assert_eq!(message, "run not found");
        }
        other => panic!("expected 404 ADK_RUN_CANCEL_FAILED, got {other:?}"),
    }

    // The read route keeps the generic notification code for the same id.
    match port.read("/api/v1/adk/runs/run-missing", "") {
        Err(AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        }) => {
            assert_eq!(status, 404);
            assert_eq!(code, "NOT_FOUND");
            assert_eq!(message, "run not found");
        }
        other => panic!("expected 404 NOT_FOUND, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:21
/// TestStoreBuiltinSkillsSplitStrategySkill,
/// go:452dea11:internal/assistant/engine/store_ops_test.go:36
/// TestBuiltinSkillStoreMetadataComesFromBundleRegistry and
/// go:452dea11:internal/assistant/engine/store_ops_test.go:147
/// TestBuiltinRefreshDoesNotOverrideNonBuiltinSkill.
///
/// Go's store registry exposes every builtin bundle through `Skill` with
/// `builtin=true` / `source=builtin`, including the split strategy research and
/// publish skills, and the product skill set (`jftrade-market`, `-derivatives`,
/// `-research`, `-prediction`, `-trading`) is always registered.  An externally
/// installed skill is a registry row, not a replacement: `ListSkills` still
/// returns the builtins alongside it, and a non-builtin `SKILL.md` on disk is
/// never rewritten by `ensureBuiltins`.
#[test]
fn builtin_skill_catalog_stays_registered_alongside_external_installs() {
    let (port, _directory) = agent_validation_port();

    let AdkReadSnapshot::Json(only_builtins) = port.read("/api/v1/adk/skills", "").expect("skills")
    else {
        panic!("skills route must answer JSON");
    };
    let builtin_skills = only_builtins["skills"].as_array().expect("skills array");
    let builtin_ids = builtin_skills
        .iter()
        .filter_map(|skill| skill["id"].as_str())
        .collect::<Vec<_>>();
    for required in [
        "jftrade-market",
        "jftrade-derivatives",
        "jftrade-research",
        "jftrade-prediction",
        "jftrade-trading",
        "jftrade-strategy-research",
        "jftrade-strategy-publish",
    ] {
        assert!(
            builtin_ids.contains(&required),
            "the builtin registry must expose {required}: {builtin_ids:?}"
        );
    }
    for skill in builtin_skills {
        assert_eq!(
            skill["source"], "builtin",
            "a builtin projection reports its bundle source: {skill}"
        );
        assert_eq!(skill["builtin"], true);
        assert_eq!(
            skill["validationStatus"], "VALID",
            "a builtin bundle validates: {skill}"
        );
        assert!(
            skill["version"].as_str().is_some_and(|v| !v.is_empty()),
            "the bundle metadata carries a version: {skill}"
        );
    }
    let strategy_research = builtin_skills
        .iter()
        .find(|skill| skill["id"] == "jftrade-strategy-research")
        .expect("strategy research skill");
    assert!(
        strategy_research["tools"]
            .as_array()
            .is_some_and(|tools| !tools.is_empty()),
        "the strategy research bundle documents its tools: {strategy_research}"
    );

    // An external install is an additional row, not a replacement for the
    // builtin catalogue.
    port.store
        .upsert_skill(
            "neodata-financial-search",
            &json!({
                "id": "neodata-financial-search",
                "displayName": "NeoData Financial Search",
                "source": "https://example.test/neodata.zip",
                "enabled": true,
                "builtin": false,
                "validationStatus": "VALID",
            })
            .to_string(),
        )
        .expect("persist installed skill");
    let AdkReadSnapshot::Json(merged) = port.read("/api/v1/adk/skills", "").expect("skills") else {
        panic!("skills route must answer JSON");
    };
    let merged_skills = merged["skills"].as_array().expect("skills array");
    let merged_ids = merged_skills
        .iter()
        .filter_map(|skill| skill["id"].as_str())
        .collect::<Vec<_>>();
    assert!(
        merged_ids.contains(&"neodata-financial-search"),
        "the external install is listed: {merged_ids:?}"
    );
    for required in [
        "jftrade-market",
        "jftrade-strategy-research",
        "external-http",
    ] {
        assert!(
            merged_ids.contains(&required),
            "an external install must not hide {required}: {merged_ids:?}"
        );
    }
    for skill in merged_skills {
        if skill["id"] == "neodata-financial-search" {
            assert_eq!(skill["builtin"], false, "the external row stays external");
            assert_eq!(skill["source"], "https://example.test/neodata.zip");
        }
    }
}

use std::sync::Mutex as OptimizeWriterMutex;

/// Fixture backtest writer that records every mutation, so the candidate
/// enqueue order and the rollback calls of `strategy.optimize` are visible.
#[derive(Debug)]
struct OptimizeBacktestsWriter {
    calls:
        OptimizeWriterMutex<Vec<crate::product::product_backtests_write_port::BacktestsWriteInput>>,
    fail_definition: Option<&'static str>,
}

impl OptimizeBacktestsWriter {
    fn new(fail_definition: Option<&'static str>) -> Self {
        Self {
            calls: OptimizeWriterMutex::new(Vec::new()),
            fail_definition,
        }
    }

    fn calls(&self) -> Vec<crate::product::product_backtests_write_port::BacktestsWriteInput> {
        self.calls.lock().expect("optimize writer lock").clone()
    }

    fn started_definitions(&self) -> Vec<String> {
        self.calls()
            .iter()
            .filter_map(|call| match call {
                crate::product::product_backtests_write_port::BacktestsWriteInput::Start {
                    payload,
                } => payload
                    .get("definitionId")
                    .and_then(Value::as_str)
                    .map(str::to_owned),
                _ => None,
            })
            .collect()
    }

    fn cancelled_runs(&self) -> Vec<String> {
        self.calls()
            .iter()
            .filter_map(|call| match call {
                crate::product::product_backtests_write_port::BacktestsWriteInput::Cancel {
                    run_id,
                } => Some(run_id.clone()),
                _ => None,
            })
            .collect()
    }
}

impl crate::product::product_backtests_write_port::BacktestsWritePort for OptimizeBacktestsWriter {
    fn mutate(
        &self,
        input: &crate::product::product_backtests_write_port::BacktestsWriteInput,
    ) -> Result<
        crate::product::product_backtests_write_port::BacktestsWritePortResult,
        crate::product::product_backtests_write_port::BacktestsWritePortError,
    > {
        use crate::product::product_backtests_write_port::{
            BacktestsWriteInput, BacktestsWritePortError, BacktestsWritePortResult,
        };
        self.calls
            .lock()
            .expect("optimize writer lock")
            .push(input.clone());
        match input {
            BacktestsWriteInput::Start { payload } => {
                let definition_id = payload
                    .get("definitionId")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                if self.fail_definition == Some(definition_id.as_str()) {
                    return Err(BacktestsWritePortError::Failed("queue down".to_owned()));
                }
                Ok(BacktestsWritePortResult::Data(json!({
                    "id": format!("run-{definition_id}"),
                    "status": "queued",
                })))
            }
            BacktestsWriteInput::Cancel { run_id } => Ok(BacktestsWritePortResult::Data(json!({
                "id": run_id,
                "cancelled": true,
            }))),
            other => Err(BacktestsWritePortError::Failed(format!(
                "unexpected backtest mutation {other:?}"
            ))),
        }
    }
}

/// Go's optimize contract tests wire `EnsureBacktestData` to answer "ready"
/// for every candidate, so the Rust fixture installs the same two
/// collaborators: a coverage reader that reports the shared window as covered
/// and a definition projection that resolves every requested candidate id.
#[derive(Debug)]
struct OptimizeCoverageReady;

impl crate::product::BacktestSyncReadSnapshotPort for OptimizeCoverageReady {
    fn progress(
        &self,
        _task_id: &str,
    ) -> Result<Option<Value>, crate::product::BacktestSyncReadSnapshotError> {
        Ok(None)
    }

    fn active_tasks(&self) -> Result<Vec<Value>, crate::product::BacktestSyncReadSnapshotError> {
        Ok(Vec::new())
    }

    fn check_coverage(
        &self,
        _request: &crate::product::BacktestDataCoverageRequest,
    ) -> Result<bool, crate::product::BacktestSyncReadSnapshotError> {
        Ok(true)
    }
}

#[derive(Debug)]
struct OptimizeDefinitionSnapshot;

impl crate::product::StrategyDefinitionSnapshotPort for OptimizeDefinitionSnapshot {
    fn list(&self) -> Result<Vec<Value>, crate::product::StrategyDefinitionSnapshotError> {
        Ok(Vec::new())
    }

    fn get(
        &self,
        definition_id: &str,
        preview: &crate::product::StrategyDefinitionPreview,
    ) -> Result<Option<Value>, crate::product::StrategyDefinitionSnapshotError> {
        Ok(Some(json!({
            "id": definition_id,
            "sourceFormat": "pine-v6",
            "derivedWarmupBars": 0,
            "derivedWarmupInterval": preview
                .interval
                .clone()
                .unwrap_or_else(|| "1m".to_owned()),
        })))
    }

    fn versions(
        &self,
        _definition_id: &str,
    ) -> Result<Option<Vec<Value>>, crate::product::StrategyDefinitionSnapshotError> {
        Ok(None)
    }

    fn version(
        &self,
        _definition_id: &str,
        _version: &str,
    ) -> Result<Option<Value>, crate::product::StrategyDefinitionSnapshotError> {
        Ok(None)
    }
}

fn optimize_bundle(
    writer: Arc<OptimizeBacktestsWriter>,
) -> (
    Arc<crate::product::product_production_ports::ProductionPortBundle>,
    crate::product::product_adk_model_runtime::ProductionAdkToolExecutor,
    tempfile::TempDir,
) {
    let (ports, executor, directory) = setup_test_bundle_and_executor();
    let mut bundle = (*ports).clone();
    bundle.backtests_write = writer;
    bundle.backtest_sync = Arc::new(OptimizeCoverageReady);
    bundle.strategy_definition = Arc::new(OptimizeDefinitionSnapshot);
    let bundle = Arc::new(bundle);
    executor.attach_ports(Arc::clone(&bundle));
    (bundle, executor, directory)
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:605
/// `TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure`.
///
/// The success half: one real backtest run per candidate definition, and one
/// persisted `OptimizationTask` row referencing them.
#[test]
fn strategy_optimize_enqueues_every_candidate_and_persists_the_task() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let writer = Arc::new(OptimizeBacktestsWriter::new(None));
    let (bundle, executor, _directory) = optimize_bundle(Arc::clone(&writer));
    assert!(
        executor.supports("strategy.optimize"),
        "the production executor must own the optimize adapter"
    );

    let output = executor
        .execute(
            "strategy.optimize",
            &json!({
                "definitionIds": ["def-a", "def-b"],
                "market": "US",
                "symbol": "US.AAPL",
                "objective": "sharpe",
            }),
        )
        .expect("optimization output");

    assert_eq!(output["status"], "queued");
    assert_eq!(output["objective"], "sharpe");
    let task_id = output["taskId"].as_str().expect("task id");
    assert!(
        task_id.starts_with("opt-"),
        "the reference prefixes optimization task ids: {task_id}"
    );
    let runs = output["runs"].as_array().expect("candidate runs");
    assert_eq!(runs.len(), 2);
    assert_eq!(runs[0]["definitionId"], "def-a");
    assert_eq!(runs[0]["runId"], "run-def-a");
    assert_eq!(runs[0]["status"], "queued");
    assert_eq!(runs[1]["definitionId"], "def-b");
    assert_eq!(runs[1]["runId"], "run-def-b");
    assert_eq!(writer.started_definitions(), vec!["def-a", "def-b"]);
    assert!(
        writer.cancelled_runs().is_empty(),
        "a successful optimization rolls nothing back"
    );
    // Candidate payloads carry the single definition the run belongs to and
    // drop the tool-level selectors.
    let start_payloads = writer
        .calls()
        .into_iter()
        .filter_map(|call| match call {
            crate::product::product_backtests_write_port::BacktestsWriteInput::Start {
                payload,
            } => Some(payload),
            _ => None,
        })
        .collect::<Vec<_>>();
    assert!(start_payloads[0].get("definitionIds").is_none());
    assert!(start_payloads[0].get("objective").is_none());
    assert_eq!(start_payloads[0]["market"], "US");

    let tasks = bundle
        .mcp_store
        .list_optimization_tasks()
        .expect("list optimization tasks");
    assert_eq!(tasks.len(), 1, "the tool persists exactly one task");
    let stored: Value =
        serde_json::from_str(&tasks[0].payload_json).expect("decode optimization task");
    assert_eq!(stored["id"], task_id);
    assert_eq!(stored["status"], "queued");
    assert_eq!(stored["objective"], "sharpe");
    assert_eq!(stored["runs"].as_array().map(Vec::len), Some(2));
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:605
/// (failure half) and `:587` (single `definitionId` fallback plus the
/// candidate limit).
#[test]
fn strategy_optimize_rolls_back_candidates_and_validates_the_request() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let failing = Arc::new(OptimizeBacktestsWriter::new(Some("def-b")));
    let (bundle, executor, _directory) = optimize_bundle(Arc::clone(&failing));
    let error = executor
        .execute(
            "strategy.optimize",
            &json!({"definitionIds": ["def-a", "def-b"], "market": "US", "symbol": "US.AAPL"}),
        )
        .expect_err("a failed candidate must fail the call");
    assert!(
        error.contains("queue candidate") && error.contains("def-b"),
        "the wrapped queue failure must name the candidate: {error}"
    );
    assert_eq!(
        failing.cancelled_runs(),
        vec!["run-def-a"],
        "the candidate enqueued before the failure is rolled back"
    );
    assert!(
        bundle
            .mcp_store
            .list_optimization_tasks()
            .expect("list optimization tasks")
            .is_empty(),
        "a failed optimization must not persist a task row"
    );

    let writer = Arc::new(OptimizeBacktestsWriter::new(None));
    let (_bundle, executor, _directory) = optimize_bundle(Arc::clone(&writer));
    let missing = executor
        .execute("strategy.optimize", &json!({"market": "US"}))
        .expect_err("candidates are required");
    assert_eq!(missing, "definitionIds is required");
    let single = executor
        .execute(
            "strategy.optimize",
            &json!({"definitionId": "def-solo", "market": "US", "symbol": "US.AAPL"}),
        )
        .expect("single-definition fallback");
    assert_eq!(single["status"], "queued");
    assert_eq!(writer.started_definitions(), vec!["def-solo"]);

    let oversized = (1..=13)
        .map(|index| format!("def-{index}"))
        .collect::<Vec<_>>();
    let error = executor
        .execute("strategy.optimize", &json!({"definitionIds": oversized}))
        .expect_err("the candidate limit is enforced");
    assert_eq!(
        error, "at most 12 optimization candidates are allowed",
        "the reference caps one optimization call"
    );
    assert_eq!(
        writer.started_definitions(),
        vec!["def-solo"],
        "a rejected call must not enqueue anything"
    );
}

/// Go's runtime contract test keeps every strategy read tool on its owner
/// contract: Pine validation answers `ok` with the normalized script, and the
/// result view echoes the run it was asked about.
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_runtime_contracts_test.go:13
#[tokio::test]
async fn pine_validation_and_backtest_result_view_keep_their_owner_contracts() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let (ports, executor, _directory) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    let validation = executor
        .execute(
            "strategy.validate_pine",
            &json!({"script": "//@version=6\nstrategy(\"Owner\", overlay=true)\nplot(close)\n"}),
        )
        .expect("Pine validation is port-free");
    assert_eq!(validation["ok"], true, "{validation}");
    assert!(
        !validation["normalizedScript"]
            .as_str()
            .unwrap_or_default()
            .trim()
            .is_empty(),
        "the normalized script must survive validation: {validation}"
    );

    let started = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Owner\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00700",
                "startTime": "2026-01-01T00:00:00Z",
                "endTime": "2026-01-02T00:00:00Z",
                "waitForCompletionMs": 0,
            }),
        )
        .expect("start one research backtest");
    let run_id = started["runId"].as_str().expect("run id").to_owned();

    let view = executor
        .execute(
            "backtest.result_view",
            &json!({"runId": run_id, "view": "summary"}),
        )
        .expect("result view");
    // The production projection describes the requested run instead of
    // echoing the selector, so the identity lives on `run.id`.
    assert_eq!(view["run"]["id"], run_id, "{view}");
    assert_eq!(view["view"], "summary", "{view}");
}

/// Go's `TestADKStrategySummariesHideSourceDetailsAndCountLinkedInstances`
/// (reachable half): the model catalog publishes the seeded strategy
/// definitions together with their count. Go additionally hides the source
/// behind `scriptPreview` and links every definition to its instances; the
/// Rust model catalog has no owner for those fields yet (see the batch note).
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_summary_contracts_test.go:9
#[test]
fn adk_strategy_definition_catalog_counts_the_seeded_definitions() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    use crate::product::product_strategy_definition_write_port::{
        StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation,
    };

    let (ports, executor, _directory) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    ports
        .strategy_definition_write
        .mutate(&StrategyDefinitionWriteInput {
            operation: StrategyDefinitionWriteOperation::Create,
            definition_id: Some("definition-1".to_owned()),
            definition: Some(json!({
                "id": "definition-1",
                "name": "Mean revert",
                "version": "1.2.0",
                "runtime": "pine-plan",
                "sourceFormat": "pine-v6",
                "symbol": "US.AAPL",
                "interval": "1d",
                "script": "//@version=6\nstrategy(\"Mean revert\", overlay=true)\nplot(close)\n",
            })),
            binding: None,
            binding_error: None,
        })
        .expect("seed strategy definition");

    let payload = executor
        .execute("strategy.definitions", &json!({}))
        .expect("model catalog reads the seeded definitions");
    assert_eq!(payload["definitionCount"], 1, "{payload}");
    let definitions = payload["definitions"]
        .as_array()
        .expect("definitions array");
    assert_eq!(definitions.len(), 1, "{payload}");
    assert_eq!(definitions[0]["id"], "definition-1", "{payload}");
    assert_eq!(definitions[0]["name"], "Mean revert", "{payload}");
}

/// Read the backtest run ids the production store currently holds, so a test
/// can prove a tool call did or did not create a run.
fn backtest_run_ids(
    ports: &crate::product::product_production_ports::ProductionPortBundle,
) -> Vec<String> {
    let payload = ports.backtest_read.list().expect("list backtest runs");
    payload
        .get("runs")
        .and_then(Value::as_array)
        .map(|runs| {
            runs.iter()
                .filter_map(|run| run.get("id").and_then(Value::as_str).map(str::to_owned))
                .collect()
        })
        .unwrap_or_default()
}

/// Go's `TestADKRuntimeResearchToolStopsBeforeRunWhenDataSyncIsPending`: when
/// the readiness dependency answers `syncing_data`, the tool returns the sync
/// task reference and never starts a backtest run.
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_runtime_contracts_test.go:156
#[tokio::test]
async fn research_backtest_pending_data_sync_returns_the_sync_task_without_starting_a_run() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let (ports, executor, _directory) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    let ready = executor
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
        .expect("data ready must start one run");
    let run_id = ready["runId"].as_str().expect("started run id").to_owned();
    let before = backtest_run_ids(&ports);
    assert_eq!(
        before,
        vec![run_id.clone()],
        "one run exists before the sync"
    );

    let pending = executor
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
        .expect("a pending data sync is answered, not raised");

    assert_eq!(pending["status"], "syncing_data", "{pending}");
    assert_eq!(pending["nextAction"], "wait_kline_sync", "{pending}");
    assert!(
        pending.get("runId").is_none() || pending["runId"].is_null(),
        "a pending sync must not fabricate a run id: {pending}"
    );
    assert_eq!(
        backtest_run_ids(&ports),
        before,
        "a pending data sync must stop before starting another run"
    );
}

/// Go's `system.status` tool only enriches the status payload with the ADK
/// module block while the assistant runtime reports itself available.
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_closure_contracts_test.go:12
#[test]
fn system_status_tool_stays_plain_without_a_configured_assistant_runtime() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let (_ports, executor, _directory) = setup_test_bundle_and_executor();

    let status = executor
        .execute("system.status", &json!({}))
        .expect("system status through the production executor");
    assert!(status.get("status").is_some(), "{status}");
    assert!(
        status.get("adk").is_none(),
        "a bundle without a usable model runtime must not publish the ADK module: {status}"
    );
}

/// Go's `strategy.optimize` parses the backtest start input — including
/// `tradingCosts` — before it queues the first candidate, so a malformed cost
/// block fails the whole call and leaves no run or task behind.
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:94
#[test]
fn strategy_optimize_rejects_non_object_trading_costs_before_enqueuing() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let writer = Arc::new(OptimizeBacktestsWriter::new(None));
    let (bundle, executor, _directory) = optimize_bundle(Arc::clone(&writer));

    for malformed in [json!("cheap"), json!([0.0003]), json!(7)] {
        let error = executor
            .execute(
                "strategy.optimize",
                &json!({
                    "definitionIds": ["def-a"],
                    "market": "US",
                    "symbol": "US.AAPL",
                    "tradingCosts": malformed,
                }),
            )
            .expect_err("a non-object tradingCosts must fail the call");
        assert_eq!(
            error, "tradingCosts must be a valid object",
            "malformed costs {malformed}"
        );
    }
    assert!(
        writer.started_definitions().is_empty(),
        "a rejected call must not queue a candidate"
    );
    let tasks = bundle
        .mcp_store
        .list_optimization_tasks()
        .expect("list optimization tasks");
    assert!(tasks.is_empty(), "a rejected call must not persist a task");
}

/// The research backtest tool validates the script and the start input before
/// it reaches the readiness port, so free text that is not Pine and a
/// non-object `tradingCosts` both fail closed.
///
/// Parity: go:452dea11:internal/assistant/assembly/adk_capability_contracts_test.go:94
#[test]
fn research_backtest_rejects_free_text_scripts_and_non_object_trading_costs() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor as _;
    let (ports, executor, _directory) = setup_test_bundle_and_executor();
    executor.attach_ports(Arc::clone(&ports));

    let free_text = executor
        .execute("strategy.research_backtest", &json!({"script": "not pine"}))
        .expect_err("free text is not a Pine script");
    assert!(
        free_text.contains("strategy script validation failed"),
        "unexpected error: {free_text}"
    );

    let costs = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Costs\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00700",
                "tradingCosts": "cheap",
            }),
        )
        .expect_err("a non-object tradingCosts must fail the call");
    assert_eq!(costs, "tradingCosts must be a valid object");
}

/// The reference gates `strategy.optimize` in `approval` mode and releases it
/// in `less_approval`/`all` (`RequiresApprovalIn=[approval]`).
#[test]
fn strategy_optimize_is_gated_in_approval_mode_only() {
    use crate::product::product_production_ports::product_production_ports_adk::tool_access_policy;

    let policy = tool_access_policy("strategy.optimize");
    assert_eq!(policy.permission, "optimize_strategy");
    assert_eq!(policy.risk_level, "low");
    assert_eq!(policy.requires_approval_in, Some(&["approval"][..]));

    let (ports, _executor, _directory) = setup_test_bundle_and_executor();
    let catalog = Arc::clone(&ports.mcp_catalog);
    assert!(
        catalog.requires_approval("strategy.optimize", "approval"),
        "approval mode must park the optimizer"
    );
    for mode in ["less_approval", "all"] {
        assert!(
            !catalog.requires_approval("strategy.optimize", mode),
            "{mode} must release the optimizer"
        );
    }
}

/// Go's `ResumeGoalRun` hands a timed-out goal a *fresh* timeout window taken
/// from `RuntimeLimits().RunTimeout`, restarts `startedAt`, and clears every
/// terminal field while keeping the paused-run branches on the same CAS.
///
/// Reference: go:452dea11:internal/assistant/engine/store_test.go
/// `TestResumeGoalRunAllowsTimedOutGoalWithFreshTimeoutWindow`.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:898
/// `TestResumeGoalRunAllowsTimedOutGoalWithFreshTimeoutWindow`.
#[test]
fn resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window() {
    let (port, store, directory) = setup_test_adk_mutation_port(None);
    let run_id = "run-timed-out-goal";
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: run_id,
            session_id: "session-timed-out-goal",
            agent_id: "agent-timed-out-goal",
            status: "TIMED_OUT",
            client_request_id: "request-timed-out-goal",
            request_fingerprint: "fingerprint-timed-out-goal",
            payload_json: &json!({
                "id": run_id,
                "sessionId": "session-timed-out-goal",
                "agentId": "agent-timed-out-goal",
                "status": "TIMED_OUT",
                "workMode": "loop",
                "workflowStatus": "RUNNING",
                "message": "run timed out",
                "resumeState": "run_timed_out",
                "startedAt": "2026-09-19T00:00:00Z",
                "completedAt": "2026-09-19T00:45:00Z",
                "failureReason": "run exceeded maximum duration of 30m0s",
                "errorCode": "RUN_TIMED_OUT",
                "degraded": true,
                "maxDurationMs": 1_800_000,
            })
            .to_string(),
        })
        .expect("seed timed-out goal run");
    // 45 minutes: the operator raised the window after the run timed out.
    std::fs::write(
        directory.path().join("settings.json"),
        r#"{"adk":{"runTimeoutMs":2700000}}"#,
    )
    .expect("write assistant runtime settings");

    let resumed = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::ResumeRun,
            identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
            body: Value::Null,
            webhook_secret: None,
        })
        .expect("resume the timed-out goal");
    assert_eq!(resumed["status"], "RUNNING");
    assert_eq!(resumed["resumeState"], "user_resuming");
    assert_eq!(
        resumed["maxDurationMs"], 2_700_000,
        "the resumed goal gets the configured window: {resumed}"
    );
    assert_ne!(
        resumed["startedAt"], "2026-09-19T00:00:00Z",
        "the resumed goal restarts its clock: {resumed}"
    );
    assert!(resumed.get("completedAt").is_none() || resumed["completedAt"].is_null());
    // Go marks both fields `omitempty`, so a resumed run drops them instead of
    // republishing an empty string.
    assert!(resumed["errorCode"].is_null(), "{resumed}");
    assert!(resumed["failureReason"].is_null(), "{resumed}");
    assert_eq!(resumed["degraded"], false);
    assert_eq!(resumed["workflowStatus"], "RUNNING");

    // The same refresh is durable, not just projected.
    let stored = store.get_run(run_id).expect("read run").expect("run row");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["maxDurationMs"], 2_700_000);
    assert_eq!(payload["status"], "RUNNING");
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:203
/// `TestSchemaConversionReportsMarshalError`: Go encodes each tool schema map
/// through JSON and reports an encoding error for values JSON cannot represent
/// (a `func`).  Rust schemas are `serde_json::Value`, which cannot hold such a
/// value, so the reachable contract is that every callable tool still yields a
/// complete object-shaped declaration — the strict reviewed schema when one
/// exists, the generic fallback otherwise — instead of failing or dropping the
/// tool.
#[test]
fn tool_declarations_stay_complete_for_every_callable_tool() {
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    let catalog = ProductionToolCatalog::from_bindings(&bindings).expect("catalog bindings");
    let callable = catalog.callable_tools();
    let declarations = catalog.openai_tools();
    assert_eq!(
        declarations.len(),
        callable.len(),
        "every callable tool must reach the model declaration list"
    );

    let mut strict = 0;
    for declaration in &declarations {
        assert_eq!(
            declaration["type"], "function",
            "declaration = {declaration}"
        );
        assert!(
            declaration["name"]
                .as_str()
                .is_some_and(|name| !name.trim().is_empty()),
            "declaration = {declaration}"
        );
        assert_eq!(
            declaration["parameters"]["type"], "object",
            "declaration = {declaration}"
        );
        if declaration["parameters"].get("required").is_some() {
            strict += 1;
        }
    }
    assert!(
        strict > 0,
        "the strict per-tool schemas must survive the projection"
    );
}
