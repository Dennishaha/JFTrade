use super::*;
use crate::product::product_production_route_registry::ProductionRouteAdapter;
use axum::http::{HeaderValue, header};

fn headers(content_type: &str, accept: &str, protocol: Option<&str>) -> HeaderMap {
    let mut headers = HeaderMap::new();
    headers.insert(
        header::CONTENT_TYPE,
        HeaderValue::from_str(content_type).unwrap(),
    );
    headers.insert(header::ACCEPT, HeaderValue::from_str(accept).unwrap());
    if let Some(protocol) = protocol {
        headers.insert(
            "Mcp-Protocol-Version",
            HeaderValue::from_str(protocol).unwrap(),
        );
    }
    headers
}

#[test]
fn headers_match_go_streamable_contract() {
    let parsed = validate_headers(&headers(
        "application/json; charset=utf-8",
        "application/json, text/event-stream",
        Some("2025-06-18"),
    ))
    .unwrap();
    assert_eq!(parsed, "2025-06-18");
    assert_eq!(
        validate_headers(&headers("text/plain", "*/*", None))
            .unwrap_err()
            .status,
        415
    );
    assert_eq!(
        validate_headers(&headers("application/json", "application/json", None))
            .unwrap_err()
            .status,
        400
    );
}

#[test]
fn future_versions_are_allowed_but_old_unknown_versions_are_rejected() {
    assert!(validate_headers(&headers("application/json", "*/*", Some("2027-01-01"))).is_ok());
    assert_eq!(
        validate_headers(&headers("application/json", "*/*", Some("2025-01-01")))
            .unwrap_err()
            .status,
        400
    );
}

#[test]
fn batching_and_envelope_validation_follow_version_boundary() {
    let body = br#"[{"jsonrpc":"2.0","id":1,"method":"ping"}]"#;
    assert_eq!(decode_messages(body, "2025-03-26").unwrap().len(), 1);
    assert!(decode_messages(body, "2025-06-18").is_err());
    assert!(validate_message(&serde_json::json!({"jsonrpc":"1.0","method":"ping"})).is_err());
    assert!(
        validate_message(&serde_json::json!({"jsonrpc":"2.0","method":"ping","params":null}))
            .is_ok()
    );
}

#[test]
fn modern_standard_headers_must_match_method_and_target() {
    let message = serde_json::json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {"_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}, "name": "models.list", "arguments": {}}
    });
    let mut headers = headers("application/json", "*/*", Some("2026-07-28"));
    headers.insert("Mcp-Method", HeaderValue::from_static("tools/call"));
    headers.insert("Mcp-Name", HeaderValue::from_static("models.list"));
    assert!(validate_standard_headers(&headers, &message, "2026-07-28").is_ok());
    headers.insert("Mcp-Name", HeaderValue::from_static("tools.search"));
    assert!(validate_standard_headers(&headers, &message, "2026-07-28").is_err());
}

#[test]
fn modern_protocol_requires_matching_meta_version() {
    let message = serde_json::json!({
        "jsonrpc": "2.0", "id": 1, "method": "ping",
        "params": {"_meta": {META_PROTOCOL_VERSION_KEY: MODERN_PROTOCOL_VERSION}}
    });
    let mut headers = headers("application/json", "*/*", Some(MODERN_PROTOCOL_VERSION));
    headers.insert("Mcp-Method", HeaderValue::from_static("ping"));
    assert!(validate_standard_headers(&headers, &message, MODERN_PROTOCOL_VERSION).is_ok());
    let missing = serde_json::json!({"jsonrpc": "2.0", "id": 1, "method": "ping"});
    let error = validate_standard_headers(&headers, &missing, MODERN_PROTOCOL_VERSION).unwrap_err();
    assert_eq!(error.code, -32602);
    headers.insert(
        "Mcp-Protocol-Version",
        HeaderValue::from_static("2025-11-25"),
    );
    let error = validate_standard_headers(&headers, &message, "2025-11-25").unwrap_err();
    assert_eq!(error.code, CODE_HEADER_MISMATCH);
}

#[test]
fn params_null_is_optional_but_required_methods_are_invalid_requests() {
    assert!(
        validate_message(
            &serde_json::json!({"jsonrpc": "2.0", "id": 1, "method": "ping", "params": null})
        )
        .is_ok()
    );
    assert!(
        validate_call_shape(
            &serde_json::json!({"jsonrpc": "2.0", "id": 1, "method": "tools/call"})
        )
        .is_ok()
    );
    assert!(requires_object_params("tools/call"));
    assert!(!requires_object_params("ping"));
    assert!(
        validate_call_shape(
            &serde_json::json!({"jsonrpc": "2.0", "method": "notifications/unknown"})
        )
        .is_ok()
    );
    assert!(
        validate_call_shape(
            &serde_json::json!({"jsonrpc": "2.0", "id": 1, "method": "notifications/unknown"})
        )
        .is_err()
    );
    assert!(
        validate_call_shape(&serde_json::json!({"jsonrpc": "2.0", "id": null, "method": "ping"}))
            .is_ok()
    );
    assert!(validate_call_shape(&serde_json::json!({"jsonrpc": "2.0", "method": "ping"})).is_err());
}

#[test]
fn initialize_negotiates_legacy_fallback_for_modern_clients() {
    assert_eq!(
        negotiate_initialize_version(
            &serde_json::json!({"protocolVersion": "2026-07-28"}),
            "2026-07-28"
        ),
        "2025-11-25"
    );
    assert_eq!(
        negotiate_initialize_version(
            &serde_json::json!({"protocolVersion": "2025-06-18"}),
            "2025-06-18"
        ),
        "2025-06-18"
    );
}

#[test]
fn model_projection_is_allowlisted_and_searchable() {
    let row = jftrade_store_sqlite::StoredAdkEntity {
            id: "provider-a".to_owned(),
            payload_json: r#"{"displayName":"Provider A","baseUrl":"https://example.test","model":"model-a","enabled":true,"default":true,"apiKey":"secret","capabilities":{"vision":true,"token":"bad"},"unexpected":"omit"}"#.to_owned(),
            created_at: "created".to_owned(),
            updated_at: "updated".to_owned(),
        };
    let model = provider_model(row).expect("valid provider projection");
    assert_eq!(model["providerId"], "provider-a");
    assert_eq!(model["callable"], true);
    assert_eq!(model["hasApiKey"], true);
    assert_eq!(model["capabilities"]["vision"], true);
    assert!(model.get("apiKey").is_none());
    assert!(model.get("unexpected").is_none());
    assert!(!model.to_string().contains("secret"));
    assert!(model_search_text(&model).contains("vision"));
}

#[test]
fn malformed_provider_payload_fails_closed() {
    let row = jftrade_store_sqlite::StoredAdkEntity {
        id: "provider-a".to_owned(),
        payload_json: "[]".to_owned(),
        created_at: String::new(),
        updated_at: String::new(),
    };
    assert!(provider_model(row).is_err());
}

#[test]
fn route_backed_microstructure_tools_have_exact_production_adapters() {
    let expected = [
        (
            "market.instrument_profile",
            ProductionRouteAdapter::MarketDataProfileRead,
        ),
        (
            "market.intraday",
            ProductionRouteAdapter::MarketDataIntradayRead,
        ),
        ("market.ticks", ProductionRouteAdapter::MarketDataTicksRead),
        ("market.depth", ProductionRouteAdapter::MarketDataDepthRead),
        (
            "market.broker_queue",
            ProductionRouteAdapter::MarketDataBrokerQueueRead,
        ),
        (
            "market.capital_flow",
            ProductionRouteAdapter::MarketDataCapitalFlowRead,
        ),
    ];
    for (name, adapter) in expected {
        assert!(PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name));
        assert_eq!(mcp_tool_adapter(name), Some(adapter));
    }
}

#[test]
fn derivatives_and_prediction_tools_have_exact_production_adapters() {
    let expected = [
        (
            "derivatives.warrants",
            ProductionRouteAdapter::MarketDataDerivativeRead,
        ),
        (
            "derivatives.futures",
            ProductionRouteAdapter::MarketDataFuturesRead,
        ),
        (
            "derivatives.option_chain",
            ProductionRouteAdapter::MarketDataOptionsChainRead,
        ),
        (
            "derivatives.option_screen",
            ProductionRouteAdapter::MarketDataOptionsScreenRead,
        ),
        (
            "derivatives.option_analysis",
            ProductionRouteAdapter::MarketDataOptionsAnalysisRead,
        ),
        (
            "derivatives.option_events",
            ProductionRouteAdapter::MarketDataOptionsEventsRead,
        ),
        (
            "prediction.discover",
            ProductionRouteAdapter::MarketDataPredictionRead,
        ),
        (
            "prediction.snapshot",
            ProductionRouteAdapter::MarketDataPredictionRead,
        ),
        (
            "prediction.depth",
            ProductionRouteAdapter::MarketDataPredictionRead,
        ),
        (
            "prediction.history",
            ProductionRouteAdapter::MarketDataPredictionRead,
        ),
        (
            "prediction.combo_eligible",
            ProductionRouteAdapter::MarketDataPredictionRead,
        ),
        (
            "prediction.combo_quote",
            ProductionRouteAdapter::MarketDataPredictionCombosWrite,
        ),
    ];
    for (name, adapter) in expected {
        assert!(PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name));
        assert_eq!(mcp_tool_adapter(name), Some(adapter));
    }
}

#[test]
fn alerts_and_research_tools_have_exact_production_adapters() {
    let expected = [
        ("alerts.price.list", ProductionRouteAdapter::AlertsRead),
        (
            "alerts.option_event.list",
            ProductionRouteAdapter::AlertsRead,
        ),
        ("research.instrument", ProductionRouteAdapter::ResearchRead),
        ("research.financials", ProductionRouteAdapter::ResearchRead),
        ("research.analyst", ProductionRouteAdapter::ResearchRead),
        ("research.ownership", ProductionRouteAdapter::ResearchRead),
        (
            "research.corporate_actions",
            ProductionRouteAdapter::ResearchRead,
        ),
        ("research.valuation", ProductionRouteAdapter::ResearchRead),
        ("research.rankings", ProductionRouteAdapter::ResearchRead),
        ("research.industry", ProductionRouteAdapter::ResearchRead),
        ("research.calendar", ProductionRouteAdapter::ResearchRead),
        ("research.macro", ProductionRouteAdapter::ResearchRead),
        (
            "research.news",
            ProductionRouteAdapter::MarketDataNewsSearchRead,
        ),
        (
            "research.screen",
            ProductionRouteAdapter::ResearchScreenWrite,
        ),
        (
            "research.screen_catalog",
            ProductionRouteAdapter::ResearchCatalog,
        ),
    ];
    for (name, adapter) in expected {
        assert!(PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name));
        assert_eq!(mcp_tool_adapter(name), Some(adapter));
    }
    for name in [
        "research.institutions",
        "research.short_interest",
        "research.technical_indicators",
        "strategy.pine_spec",
        "strategy.validate_pine",
    ] {
        assert!(!PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name));
        assert_eq!(mcp_tool_adapter(name), None);
    }
}
