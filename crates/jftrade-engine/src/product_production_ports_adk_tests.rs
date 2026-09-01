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
