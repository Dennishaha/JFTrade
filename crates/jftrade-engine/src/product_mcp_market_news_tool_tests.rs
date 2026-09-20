//! Executor and port coverage for the market news / corporate actions tools.
//!
//! Owner: `internal/assistant/assembly/market_news_tools_test.go` on the `go`
//! branch.  The reference registers `market.news` and
//! `market.corporate_actions` in the ADK registry; Rust exposes the same
//! capabilities as `research.news` and `research.corporate_actions`
//! (market-data news/actions base URLs) plus the news `limit` and corporate
//! action `from`/`to` validation on the production read ports.

use super::tests::production_bundle;
use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::{
    ProductionMarketDataNewsPort, ProductionPortBundle, SharedTradeReadRuntime,
};
use crate::product::{
    ActiveProviderState, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotError, MarketDataNewsSearchReadSnapshotPort,
};
use jftrade_integration_marketdata_helper::HelperClient;
use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

#[derive(Debug, Default)]
struct RecordingNewsSearch {
    reads: Mutex<Vec<(String, String)>>,
}

impl RecordingNewsSearch {
    fn recorded(&self) -> Vec<(String, String)> {
        self.reads.lock().expect("news reads").clone()
    }
}

impl MarketDataNewsSearchReadSnapshotPort for RecordingNewsSearch {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
        self.reads
            .lock()
            .expect("news reads")
            .push((path.to_owned(), query.to_owned()));
        Ok(json!({
            "instrumentId": "US.MSFT",
            "entries": [],
            "metadata": {"source": "yfinance-news"},
        }))
    }
}

fn bundle_with_news_search(
    port: Arc<dyn MarketDataNewsSearchReadSnapshotPort>,
) -> (tempfile::TempDir, Arc<ProductionPortBundle>) {
    let (directory, mut ports) = production_bundle();
    ports.market_data_news_search = port;
    (directory, Arc::new(ports))
}

/// Parity: go:452dea11:internal/assistant/assembly/market_news_tools_test.go:15
/// `TestADKMarketNewsAndCorporateActionsToolsForwardNormalizedInputs`: both
/// tools keep the reviewed `read_internal`/low policy with no per-mode
/// confirmation, require an instrument, and forward the normalized
/// market/symbol pair (news keeps its `limit`).
#[test]
fn market_news_tool_normalizes_market_and_limit_before_the_read_port() {
    let recorder = Arc::new(RecordingNewsSearch::default());
    let (_directory, ports) = bundle_with_news_search(recorder.clone());
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let missing = executor
        .execute_production("research.news", &json!({}))
        .expect_err("a news call without an instrument must fail");
    assert_eq!(missing.status, 400);
    assert_eq!(missing.code, "BAD_REQUEST");

    executor
        .execute_production(
            "research.news",
            &json!({"market": "us", "symbol": "msft", "limit": 5}),
        )
        .expect("normalized news read");
    assert_eq!(
        recorder.recorded(),
        [(
            "/api/v1/market-data/news".to_owned(),
            "instrumentId=US%2EMSFT&limit=5".to_owned(),
        )]
    );

    let definitions =
        crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS;
    for name in ["research.news", "research.corporate_actions"] {
        let definition = definitions
            .iter()
            .find(|definition| definition.id == name)
            .unwrap_or_else(|| panic!("{name} definition"));
        assert_eq!(definition.category, "research", "{name}");
        let policy =
            crate::product::product_production_ports::product_production_ports_adk::tool_access_policy(name);
        assert_eq!(policy.permission, "read_internal", "{name}");
        assert_eq!(policy.risk_level, "low", "{name}");
        assert_eq!(policy.requires_approval_in, None, "{name}");
    }

    let (_, skill_ports) = production_bundle();
    let skills = match skill_ports
        .adk_read
        .read("/api/v1/adk/skills", "")
        .expect("read builtin skills")
    {
        crate::product::AdkReadSnapshot::Json(value) => {
            value["skills"].as_array().expect("skills array").clone()
        }
        crate::product::AdkReadSnapshot::Stream(_) => panic!("expected skills JSON"),
    };
    // Rust names this capability `research.news`/`research.corporate_actions`
    // (the reviewed MCP catalog) and therefore groups it with the research
    // skill instead of Go's `jftrade-market` attribution.
    let research_skill = skills
        .iter()
        .find(|skill| skill["id"] == "jftrade-research")
        .expect("jftrade-research builtin skill");
    let skill_tools = research_skill["tools"]
        .as_array()
        .expect("research skill tools");
    for name in ["research.news", "research.corporate_actions"] {
        assert!(
            skill_tools.iter().any(|tool| tool == name),
            "{name} belongs to the research skill: {}",
            research_skill["tools"]
        );
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/market_news_tools_test.go:15
/// (news default half): a news read without an explicit bound keeps the
/// reference default of ten entries, which the search port applies before the
/// provider call.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn news_search_read_applies_the_reference_default_limit() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with("GET /providers/yfinance/news/US/MSFT?limit=10 HTTP/1.1\r\n"),
            "request = {request}"
        );
        let body = r#"{"market":"US","symbol":"MSFT","instrument_id":"US.MSFT","entries":[],"source":"yfinance-news"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });

    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: Some(
            HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{address}"),
                bearer_token: None,
                request_timeout: Duration::from_secs(1),
                max_attempts: 1,
                retry_delay: Duration::ZERO,
            })
            .expect("helper client"),
        ),
        trade_runtime: None,
    };

    let value = MarketDataNewsSearchReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/news",
        "instrumentId=US.MSFT",
    )
    .expect("default news limit read");
    assert!(
        value["entries"].as_array().is_some(),
        "the projected news payload keeps an entry list: {value}"
    );
    server.await.expect("server");
}

/// Parity: go:452dea11:internal/assistant/assembly/market_news_tools_test.go:15
/// (corporate actions half): the reference converts an offset window to UTC
/// before the provider read and preserves the requested identity, so the
/// helper must see `from=2025-01-01T00:00:00Z`.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn corporate_actions_read_forwards_the_utc_normalized_window() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with(
                "GET /providers/yfinance/corporate-actions/HK/00700?from=2025-01-01T00%3A00%3A00Z&to=2026-01-01T00%3A00%3A00Z HTTP/1.1\r\n"
            ),
            "request = {request}"
        );
        let body = r#"{"market":"HK","symbol":"00700","instrument_id":"HK.00700","events":[],"source":"yfinance-actions"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });

    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: Some(
            HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{address}"),
                bearer_token: None,
                request_timeout: Duration::from_secs(1),
                max_attempts: 1,
                retry_delay: Duration::ZERO,
            })
            .expect("helper client"),
        ),
        trade_runtime: None,
    };

    let value = MarketDataNewsActionsReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/corporate-actions/HK/00700",
        "from=2025-01-01T08:00:00%2B08:00&to=2026-01-01T00:00:00Z",
    )
    .expect("corporate actions window");
    assert_eq!(value["instrumentId"], "HK.00700");
    assert!(value["events"].as_array().expect("events").is_empty());
    server.await.expect("server");
}

/// Parity: go:452dea11:internal/assistant/assembly/market_news_tools_test.go:93
/// `TestADKMarketNewsAndCorporateActionsToolsFailClosedWithoutPorts`: both
/// capabilities answer a structured failure instead of an empty payload when
/// the owner port is missing.
#[test]
fn market_news_and_corporate_actions_fail_closed_without_owner_ports() {
    let (_directory, ports) = production_bundle();
    let ports = Arc::new(ports);
    let unwired = ProductionMcpToolExecutor::new(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
    );
    let failure = unwired
        .execute_production("research.news", &json!({"instrumentId": "US.AAPL"}))
        .expect_err("a listener without the production bundle must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE");

    let wired = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    let failure = wired
        .execute_production("research.news", &json!({"instrumentId": "US.AAPL"}))
        .expect_err("an unconfigured provider must fail closed");
    assert_eq!(failure.status, 503);
    assert_eq!(failure.code, "MARKET_DATA_NEWS_SEARCH_UNAVAILABLE");

    let failure = ports
        .market_data_news_actions
        .read("/api/v1/market-data/corporate-actions/US/AAPL", "")
        .expect_err("corporate actions without a provider must fail closed");
    match failure {
        crate::product::MarketDataNewsActionsReadSnapshotError::Unavailable(message) => {
            assert!(message.contains("provider"), "message = {message}");
        }
        other => panic!("expected the unavailable failure, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/market_news_tools_test.go:104
/// `TestADKMarketNewsToolSurfacesProviderCapabilityAsClearMessage`: a provider
/// that cannot serve the feed answers a capability rejection that names the
/// requested provider instead of a generic failure.
#[test]
fn corporate_actions_surface_the_capability_class_for_a_foreign_broker() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };

    let failure = MarketDataNewsActionsReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/corporate-actions/US/AAPL",
        "brokerId=futu",
    )
    .expect_err("the requested broker must not borrow the active provider");
    match failure {
        crate::product::MarketDataNewsActionsReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
            assert!(
                message.contains("futu") && message.contains("does not match active provider"),
                "message = {message}"
            );
        }
        other => panic!("expected the capability rejection, got {other:?}"),
    }

    // Futu without a ready OpenD session fails closed instead of borrowing the
    // helper's corporate action feed.
    let futu_state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    futu_state.set_readiness(true, false, false);
    let futu_port = ProductionMarketDataNewsPort {
        active_provider_state: futu_state,
        helper: None,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    let failure = MarketDataNewsActionsReadSnapshotPort::read(
        &futu_port,
        "/api/v1/market-data/corporate-actions/HK/00700",
        "",
    )
    .expect_err("Futu without a ready OpenD session must fail closed");
    match failure {
        crate::product::MarketDataNewsActionsReadSnapshotError::Unavailable(message) => {
            assert!(message.contains("Futu"), "message = {message}");
            assert!(message.contains("not ready"), "message = {message}");
        }
        other => panic!("expected the Futu readiness failure, got {other:?}"),
    }
}
