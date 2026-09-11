use super::*;
use crate::product::product_research_screen_write_port::{
    ResearchScreenColumn, ResearchScreenWritePort, ResearchScreenWriteQuery,
};
use std::io::{Read, Write};
use std::net::TcpListener as StdTcpListener;
use std::sync::Arc;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

fn helper(base_url: String) -> HelperClient {
    HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
        base_url,
        bearer_token: None,
        // Keep the fixture timeout above the scheduler noise of the full
        // engine suite; production timeout/error mapping is tested by the
        // helper integration tests with their own configured budgets.
        request_timeout: Duration::from_secs(5),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client")
}

#[derive(Debug)]
struct FutuScreenFixture {
    page: jftrade_integration_futu::StockScreenPage,
}

impl jftrade_integration_futu::StockScreenReadPort for FutuScreenFixture {
    fn query(
        &self,
        _query: &jftrade_integration_futu::StockScreenQuery,
    ) -> Result<jftrade_integration_futu::StockScreenPage, jftrade_integration_futu::StockScreenQueryError>
    {
        Ok(self.page.clone())
    }
}

#[test]
fn futu_stock_screen_projects_exact_mainland_rows_and_omits_combined_total() {
    use jftrade_integration_futu::{
        StockScreenProperty, StockScreenPropertyParams, StockScreenResult, StockScreenSecurity,
        StockScreenValue,
    };
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_stock_screen_reader(Some(Arc::new(FutuScreenFixture {
        page: jftrade_integration_futu::StockScreenPage {
            last_page: false,
            all_count: Some(20),
            items: vec![
                jftrade_integration_futu::StockScreenItem {
                    stock_id: 101,
                    security: Some(StockScreenSecurity {
                        market: "SH".to_owned(),
                        code: "600519".to_owned(),
                        instrument_id: "SH.600519".to_owned(),
                    }),
                    results: vec![
                        StockScreenResult {
                            factor_key: "basic.code".to_owned(),
                            property: StockScreenProperty {
                                category: "basic".to_owned(),
                                provider_id: 1101,
                                factor_key: "basic.code".to_owned(),
                                params: StockScreenPropertyParams::default(),
                            },
                            value: StockScreenValue::String {
                                value: "600519".to_owned(),
                            },
                            enum_type_name: None,
                            enum_name: None,
                            end_time: None,
                        },
                        StockScreenResult {
                            factor_key: "basic.name".to_owned(),
                            property: StockScreenProperty {
                                category: "basic".to_owned(),
                                provider_id: 1102,
                                factor_key: "basic.name".to_owned(),
                                params: StockScreenPropertyParams::default(),
                            },
                            value: StockScreenValue::String {
                                value: "贵州茅台".to_owned(),
                            },
                            enum_type_name: None,
                            enum_name: None,
                            end_time: None,
                        },
                        StockScreenResult {
                            factor_key: "simple.price".to_owned(),
                            property: StockScreenProperty {
                                category: "simple".to_owned(),
                                provider_id: 2201,
                                factor_key: "simple.price".to_owned(),
                                params: StockScreenPropertyParams::default(),
                            },
                            value: StockScreenValue::Number { value: 1_700.0 },
                            enum_type_name: None,
                            enum_name: None,
                            end_time: None,
                        },
                    ],
                },
                jftrade_integration_futu::StockScreenItem {
                    stock_id: 202,
                    security: Some(StockScreenSecurity {
                        market: "SZ".to_owned(),
                        code: "000001".to_owned(),
                        instrument_id: "SZ.000001".to_owned(),
                    }),
                    results: Vec::new(),
                },
            ],
        },
    })));
    let port = ProductionResearchScreenHelperPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: Some(runtime),
    };
    let request = ResearchScreenWriteQuery {
        broker_id: "futu".to_owned(),
        account_id: String::new(),
        trading_environment: String::new(),
        market: "SH".to_owned(),
        offset: 10,
        limit: 10,
        definition: json!({
            "catalogVersion": "futu-stock-screen-v1",
            "querySchemaVersion": 2,
            "market": "SH",
            "pool": {},
            "conditions": [],
            "columns": [{"columnId": "price", "factor": {"instanceId": "price", "factorKey": "simple.price", "params": {}}}],
            "sorts": []
        }),
        columns: vec![ResearchScreenColumn {
            column_id: "price".to_owned(),
            instance_id: "price".to_owned(),
            factor_key: "simple.price".to_owned(),
            label: "最新价".to_owned(),
            unit: "currency".to_owned(),
        }],
    };
    let value = port.query(&request).expect("Futu stock screen");
    assert_eq!(value["entries"].as_array().map(Vec::len), Some(1));
    assert_eq!(value["entries"][0]["instrumentId"], "SH.600519");
    assert_eq!(value["entries"][0]["quoteCurrency"], "CNY");
    assert_eq!(value["entries"][0]["cells"]["price"]["value"]["number"], 1_700.0);
    assert!(value.get("total").is_none());
    assert_eq!(
        value["warnings"][0],
        "OpenD reports only a combined A-share total; total is omitted for exact SH/SZ results."
    );
    assert_eq!(value["nextOffset"], 12);
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

#[test]
fn production_research_port_preserves_helper_http_errors() {
    // Keep the fixture listener on its own OS thread.  The production port is
    // synchronous and performs its reqwest call on a spawned Tokio runtime;
    // hosting the peer on the test runtime lets a busy full-suite scheduler
    // starve the response long enough to turn a deterministic 404 into a
    // connection/timeout error.
    let listener = StdTcpListener::bind("127.0.0.1:0").expect("listen");
    let address = listener.local_addr().expect("address");
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        stream
            .set_read_timeout(Some(Duration::from_secs(5)))
            .expect("read timeout");
        let mut request = Vec::with_capacity(4096);
        let mut chunk = [0_u8; 1024];
        while !request.windows(4).any(|window| window == b"\r\n\r\n") {
            let read = stream.read(&mut chunk).expect("read");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
            if request.len() > 16 * 1024 {
                break;
            }
        }
        let body = r#"{"error":{"code":"NOT_FOUND","message":"financials not found"}}"#;
        let response = format!(
            "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: {}\r\nRetry-After: 3\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).expect("write");
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
    // The fixture envelope describes the financials operation; exercise that
    // canonical route so the request and expected remote error stay aligned.
    let result = port.read("/api/v1/research/financials/US.AAPL", "");
    assert!(matches!(
        result,
        Err(ResearchReadSnapshotError::Failed {
            status: 404,
            ref code,
            ref message,
            retry_after_seconds: Some(3),
        }) if code == "NOT_FOUND" && message == "financials not found"
    ));
    server.join().expect("server");
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
        trade_runtime: None,
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
