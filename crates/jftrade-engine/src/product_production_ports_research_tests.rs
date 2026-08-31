use super::*;
use crate::product::product_research_screen_write_port::{
    ResearchScreenColumn, ResearchScreenWritePort, ResearchScreenWriteQuery,
};
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

fn helper(base_url: String) -> HelperClient {
    HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
        base_url,
        bearer_token: None,
        request_timeout: Duration::from_secs(1),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client")
}

#[test]
fn research_helper_request_rejects_unsupported_or_malformed_paths() {
    assert!(matches!(
        research_helper_request("/api/v1/research/technical-indicators/US.AAPL", ""),
        Err(ResearchReadSnapshotError::Unavailable(_))
    ));
    assert!(matches!(
        research_helper_request("/api/v1/research/financials/US/AAPL", ""),
        Err(ResearchReadSnapshotError::Invalid(_))
    ));
}

#[test]
fn research_helper_request_parses_canonical_instrument_ids() {
    for (path, operation) in [
        ("/api/v1/research/instruments/us.aapl", "profile"),
        ("/api/v1/research/financials/us.aapl", "financials"),
        ("/api/v1/research/analyst/us.aapl", "analyst"),
        ("/api/v1/research/ownership/us.aapl", "ownership"),
        (
            "/api/v1/research/corporate-actions/us.aapl",
            "corporate-actions",
        ),
    ] {
        let (actual_operation, market, symbol, query) =
            research_helper_request(path, "").expect("canonical instrument");
        assert_eq!(actual_operation, operation);
        assert_eq!(market, "US");
        assert_eq!(symbol, "AAPL");
        assert!(query.is_empty());
    }
    let (_, market, symbol, _) = research_helper_request("/api/v1/research/analyst/US.BRK.B", "")
        .expect("dot-qualified US symbols remain valid");
    assert_eq!(market, "US");
    assert_eq!(symbol, "BRK.B");
    assert!(matches!(
        research_helper_request("/api/v1/research/analyst/US/AAPL", ""),
        Err(ResearchReadSnapshotError::Invalid(_))
    ));
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_research_port_forwards_financials_to_helper() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(request.starts_with(
            "GET /providers/yfinance/financials/US/AAPL?statement=balance HTTP/1.1\r\n"
        ));
        let body = r#"{"instrumentId":"US.AAPL","statement":"balance","fields":[],"periods":[]}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: Some(helper(format!("http://{address}"))),
        trade_runtime: None,
    };
    let value = port
        .read("/api/v1/research/financials/US.AAPL", "statement=balance")
        .expect("research response");
    assert_eq!(value["statement"], "balance");
    server.await.expect("server");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_research_port_preserves_helper_http_errors() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let body = r#"{"error":{"code":"NOT_FOUND","message":"financials not found"}}"#;
        let response = format!(
            "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: {}\r\nRetry-After: 3\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
        let mut request = [0_u8; 1024];
        let _ = stream.read(&mut request).await;
    });
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: Some(helper(format!("http://{address}"))),
        trade_runtime: None,
    };
    let result = port.read("/api/v1/research/analyst/US.AAPL", "");
    assert!(matches!(
        result,
        Err(ResearchReadSnapshotError::Failed {
            status: 404,
            ref code,
            ref message,
            retry_after_seconds: Some(3),
        }) if code == "NOT_FOUND" && message == "financials not found"
    ));
    server.await.expect("server");
}

#[derive(Debug)]
struct FixtureValuationReader;

impl jftrade_integration_futu::ValuationDetailReadPort for FixtureValuationReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::ValuationDetailQuery,
    ) -> Result<
        jftrade_integration_futu::ValuationDetailSnapshot,
        jftrade_integration_futu::ValuationDetailQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.valuation_type, Some(1));
        assert_eq!(query.interval_type, Some(2));
        Ok(jftrade_integration_futu::ValuationDetailSnapshot {
            security: jftrade_integration_futu::ValuationDetailSecurity {
                market: "US".to_owned(),
                code: "AAPL".to_owned(),
                instrument_id: "US.AAPL".to_owned(),
            },
            valuation_type: Some(1),
            last_update_time: None,
            last_update_time_str: None,
            trend: None,
            market_distribution: None,
            plate_distribution: None,
            profit_growth_rate: None,
        })
    }
}

#[test]
fn futu_valuation_route_projects_typed_reader_and_query() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_valuation_detail(Some(Arc::new(FixtureValuationReader)));
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read(
            "/api/v1/research/valuation/US.AAPL",
            "brokerId=futu&operation=detail&valuationType=1&intervalType=2",
        )
        .expect("valuation response");
    assert_eq!(value["provider"]["brokerId"], "futu");
    assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["valuationType"], 1);
    assert_eq!(value["hasMore"], false);
}

#[test]
fn futu_valuation_route_fails_closed_when_reader_is_missing() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read("/api/v1/research/valuation/US.AAPL", ""),
        Err(ResearchReadSnapshotError::Unavailable(message))
            if message == "Futu valuation detail reader is not ready"
    ));
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn research_screen_helper_projects_rows_and_cells_without_fixture_defaults() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 8192];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(request.starts_with("POST /providers/yfinance/screen HTTP/1.1\r\n"));
        assert!(request.contains("\"factor_key\":\"simple.price\""));
        let body = r#"{"entries":[{"instrument_id":"US.AAPL","name":"Apple","symbol":"AAPL","industry":null,"quote_currency":"USD","values":{"simple.price":189.25}}],"total":1,"has_more":false,"next_offset":null,"as_of":"2026-08-31T12:00:00-04:00","source":"yfinance-screen"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    let port = ProductionResearchScreenHelperPort {
        active_provider_state: state,
        helper: Some(helper(format!("http://{address}"))),
    };
    let request = ResearchScreenWriteQuery {
        broker_id: "yfinance".to_owned(),
        account_id: String::new(),
        trading_environment: String::new(),
        market: "US".to_owned(),
        offset: 0,
        limit: 50,
        definition: json!({
            "conditions": [{"factor": {"factorKey": "simple.price"}, "operator": "gte", "value": 10}],
            "sorts": [{"factor": {"factorKey": "simple.price"}, "direction": "desc"}]
        }),
        columns: vec![ResearchScreenColumn {
            column_id: "price".to_owned(),
            instance_id: "price".to_owned(),
            factor_key: "simple.price".to_owned(),
            label: "Price".to_owned(),
            unit: "currency".to_owned(),
        }],
    };
    let value = port.query(&request).expect("screen response");
    assert_eq!(value["provider"]["brokerId"], "yfinance");
    assert_eq!(value["entries"][0]["instrumentId"], "US.AAPL");
    assert_eq!(
        value["entries"][0]["cells"]["price"]["value"]["number"],
        189.25
    );
    assert_eq!(value["total"], 1);
    assert_eq!(value["hasMore"], false);
    server.await.expect("server");
}
