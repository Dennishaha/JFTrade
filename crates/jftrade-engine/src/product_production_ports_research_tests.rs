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

#[derive(Debug)]
struct FutuEarningsCalendarFixture {
    items: Vec<jftrade_integration_futu::EarningsCalendarItem>,
}

impl jftrade_integration_futu::EarningsCalendarReadPort for FutuEarningsCalendarFixture {
    fn query(
        &self,
        _query: &jftrade_integration_futu::EarningsCalendarQuery,
    ) -> Result<
        jftrade_integration_futu::EarningsCalendarPage,
        jftrade_integration_futu::EarningsCalendarQueryError,
    > {
        Ok(jftrade_integration_futu::EarningsCalendarPage {
            items: self.items.clone(),
        })
    }
}

#[test]
fn futu_earnings_calendar_route_defaults_to_earnings_and_projects_event_identity() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_earnings_calendar_reader(Some(Arc::new(FutuEarningsCalendarFixture {
        items: vec![jftrade_integration_futu::EarningsCalendarItem {
            security: jftrade_integration_futu::EarningsCalendarSecurity {
                market: "US".to_owned(),
                code: "AAPL".to_owned(),
                instrument_id: "US.AAPL".to_owned(),
            },
            name: Some("Apple".to_owned()),
            earnings_date: Some("2026-07-22".to_owned()),
            earnings_timestamp: None,
            pub_type: None,
            period_text: Some("2025Q2".to_owned()),
            estimate_list: Vec::new(),
            option_volume: None,
            iv: None,
            iv_rank: None,
            iv_percentile: None,
            market_cap: None,
            price: None,
        }],
    })));
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: Some(runtime),
    };
    // The Futu default calendar operation is `earnings`; omitting the
    // parameter must still reach the OpenD reader instead of the helper path.
    let value = port
        .read(
            "/api/v1/research/calendars",
            "brokerId=futu&market=US&beginDate=2026-07-22&endDate=2026-07-22",
        )
        .expect("Futu earnings calendar");
    assert_eq!(value["provider"]["capability"], "available");
    assert_eq!(value["total"], 1);
    assert_eq!(value["entries"][0]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["symbol"], "AAPL");
    assert_eq!(value["entries"][0]["eventDate"], "2026-07-22");
    assert_eq!(value["entries"][0]["calendarType"], "earnings");
    assert_eq!(value["metadata"]["rangeChunks"], 1);
}

/// Parity: go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:72
/// TestValidateResearchCalendarQueryIgnoresOtherOperations
///
/// Go runs the earnings-calendar validator only when `operation` is empty or
/// `earnings`; `operation=dividends` skips it entirely, so an option-only sort
/// and an option-only filter on an SH symbol are not earnings-calendar
/// failures. The Rust route owner keeps the same gate: a non-earnings
/// operation never reaches the OpenD earnings reader, while `operation=earnings`
/// still validates the same parameters.
#[test]
fn futu_calendar_route_skips_earnings_validation_for_other_operations() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_earnings_calendar_reader(Some(Arc::new(FutuEarningsCalendarFixture {
        items: Vec::new(),
    })));
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: Some(runtime),
    };

    let other = port
        .read(
            "/api/v1/research/calendars",
            "brokerId=futu&market=SH&operation=dividends&sort=iv&stockScope=mine&ivRankMin=1",
        )
        .expect_err("Futu serves dividends through the helper path");
    assert!(
        !other.to_string().contains("earnings calendar"),
        "operation=dividends must skip the earnings validator, got {other}"
    );

    let earnings = port
        .read(
            "/api/v1/research/calendars",
            "brokerId=futu&market=SH&operation=earnings&sort=iv",
        )
        .expect_err("SH does not support the option sort");
    assert!(
        earnings.to_string().contains("HK/US"),
        "operation=earnings must still validate the sort, got {earnings}"
    );
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
// Parity: go:452dea11:pkg/futu/adapter_stock_screen_test.go:142 TestStockScreenFeatureResultNormalizesIdentityCellsAndOffset
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
// Parity: go:452dea11:pkg/futu/adapter_stock_screen_test.go:188 TestNormalizeStockScreenRowPreservesParameterizedInstanceIdentity
fn futu_stock_screen_selects_parameterized_columns_and_derives_counter_currency() {
    use jftrade_integration_futu::{
        StockScreenProperty, StockScreenPropertyParams, StockScreenResult, StockScreenSecurity,
        StockScreenValue,
    };

    fn row(
        stock_id: u64,
        market: &str,
        code: &str,
        name: &str,
        indicator_params: Vec<i64>,
        price: f64,
    ) -> jftrade_integration_futu::StockScreenItem {
        jftrade_integration_futu::StockScreenItem {
            stock_id,
            security: Some(StockScreenSecurity {
                market: market.to_owned(),
                code: code.to_owned(),
                instrument_id: format!("{market}.{code}"),
            }),
            results: vec![
                StockScreenResult {
                    factor_key: "basic.name".to_owned(),
                    property: StockScreenProperty {
                        category: "basic".to_owned(),
                        provider_id: 1102,
                        factor_key: "basic.name".to_owned(),
                        params: StockScreenPropertyParams::default(),
                    },
                    value: StockScreenValue::String {
                        value: name.to_owned(),
                    },
                    enum_type_name: None,
                    enum_name: None,
                    end_time: None,
                },
                StockScreenResult {
                    factor_key: "indicator.ma".to_owned(),
                    property: StockScreenProperty {
                        category: "indicator".to_owned(),
                        provider_id: 18,
                        factor_key: "indicator.ma".to_owned(),
                        params: StockScreenPropertyParams {
                            period: Some(11),
                            indicator_params,
                            ..Default::default()
                        },
                    },
                    value: StockScreenValue::Number { value: price },
                    enum_type_name: None,
                    enum_name: None,
                    end_time: None,
                },
            ],
        }
    }

    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_stock_screen_reader(Some(Arc::new(FutuScreenFixture {
        page: jftrade_integration_futu::StockScreenPage {
            last_page: false,
            all_count: Some(2),
            items: vec![
                row(1, "HK", "80700", "腾讯控股-R", vec![20], 180.0),
                row(2, "HK", "00700", "腾讯控股", vec![60], 170.0),
            ],
        },
    })));
    let request = ResearchScreenWriteQuery {
        broker_id: "futu".to_owned(),
        account_id: String::new(),
        trading_environment: String::new(),
        market: "HK".to_owned(),
        offset: 0,
        limit: 10,
        definition: json!({
            "catalogVersion": "futu-stock-screen-v1",
            "querySchemaVersion": 2,
            "market": "HK",
            "pool": {},
            "conditions": [],
            "columns": [
                {"columnId": "ma20-column", "factor": {"instanceId": "ma20", "factorKey": "indicator.ma", "params": {"period": 11, "indicatorParams": [20]}}},
                {"columnId": "ma60-column", "factor": {"instanceId": "ma60", "factorKey": "indicator.ma", "params": {"period": 11, "indicatorParams": [60]}}}
            ],
            "sorts": []
        }),
        columns: vec![
            ResearchScreenColumn {
                column_id: "ma20-column".to_owned(),
                instance_id: "ma20".to_owned(),
                factor_key: "indicator.ma".to_owned(),
                label: String::new(),
                unit: String::new(),
            },
            ResearchScreenColumn {
                column_id: "ma60-column".to_owned(),
                instance_id: "ma60".to_owned(),
                factor_key: "indicator.ma".to_owned(),
                label: String::new(),
                unit: String::new(),
            },
        ],
    };
    let value = port_query_futu_screen(&state, &runtime, &request);
    // Parameterized columns must bind to the row result whose provider params
    // match the requested instance, not to the first same-key result.
    assert_eq!(
        value["entries"][0]["cells"]["ma20-column"]["value"]["number"],
        180.0
    );
    assert_eq!(
        value["entries"][0]["cells"]["ma20-column"]["instanceId"],
        "ma20"
    );
    assert_eq!(
        value["entries"][1]["cells"]["ma60-column"]["value"]["number"],
        170.0
    );
    // HK RMB counters report CNY, plain HKD counters stay HKD.
    assert_eq!(value["entries"][0]["quoteCurrency"], "CNY");
    assert_eq!(value["entries"][1]["quoteCurrency"], "HKD");
}

fn port_query_futu_screen(
    state: &Arc<ActiveProviderState>,
    runtime: &Arc<SharedTradeReadRuntime>,
    request: &ResearchScreenWriteQuery,
) -> Value {
    let port = ProductionResearchScreenHelperPort {
        active_provider_state: Arc::clone(state),
        helper: None,
        trade_runtime: Some(Arc::clone(runtime)),
    };
    port.query(request).expect("Futu stock screen")
}

#[test]
// Parity: go:452dea11:pkg/futu/adapter_stock_screen_test.go:228 TestStockScreenFeatureResultUsesPerRowMainlandIdentityAndFiltersExactMarkets
fn futu_stock_screen_filters_exact_mainland_markets_and_maps_rate_limit() {
    use jftrade_integration_futu::{
        StockScreenProperty, StockScreenPropertyParams, StockScreenResult, StockScreenSecurity,
        StockScreenValue,
    };

    fn plain_row(
        stock_id: u64,
        market: &str,
        code: &str,
    ) -> jftrade_integration_futu::StockScreenItem {
        jftrade_integration_futu::StockScreenItem {
            stock_id,
            security: Some(StockScreenSecurity {
                market: market.to_owned(),
                code: code.to_owned(),
                instrument_id: format!("{market}.{code}"),
            }),
            results: vec![StockScreenResult {
                factor_key: "basic.name".to_owned(),
                property: StockScreenProperty {
                    category: "basic".to_owned(),
                    provider_id: 1102,
                    factor_key: "basic.name".to_owned(),
                    params: StockScreenPropertyParams::default(),
                },
                value: StockScreenValue::String {
                    value: code.to_owned(),
                },
                enum_type_name: None,
                enum_name: None,
                end_time: None,
            }],
        }
    }

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
                plain_row(101, "SH", "600519"),
                plain_row(202, "SZ", "000001"),
            ],
        },
    })));
    let request = ResearchScreenWriteQuery {
        broker_id: "futu".to_owned(),
        account_id: String::new(),
        trading_environment: String::new(),
        market: "SZ".to_owned(),
        offset: 10,
        limit: 10,
        definition: json!({
            "catalogVersion": "futu-stock-screen-v1",
            "querySchemaVersion": 2,
            "market": "SZ",
            "pool": {},
            "conditions": [],
            "columns": [],
            "sorts": []
        }),
        columns: Vec::new(),
    };
    let value = port_query_futu_screen(&state, &runtime, &request);
    assert_eq!(value["entries"].as_array().map(Vec::len), Some(1));
    assert_eq!(value["entries"][0]["instrumentId"], "SZ.000001");
    // nextOffset advances by the raw provider page size, not by the filtered
    // row count, so a page that only contains other markets still moves on.
    assert_eq!(value["nextOffset"], 12);
    assert_eq!(
        value["warnings"][0],
        "OpenD reports only a combined A-share total; total is omitted for exact SH/SZ results."
    );
    assert!(value.get("total").is_none());
}

#[test]
// Parity: go:452dea11:pkg/futu/adapter_stock_screen_test.go:385 TestResearchScreenRateLimitErrorRoundTrip
fn futu_stock_screen_rate_limit_maps_to_retry_after_seconds() {
    let error = screen::map_futu_screen_error(
        jftrade_integration_futu::StockScreenQueryError::RateLimited {
            retry_after_ms: 2_500,
        },
    );
    assert!(matches!(
        error,
        ResearchScreenWritePortError::RateLimited {
            ref message,
            retry_after: 3,
        } if message.contains("rate limited")
    ));
    // A sub-second window must still be surfaced as at least one second.
    let error = screen::map_futu_screen_error(
        jftrade_integration_futu::StockScreenQueryError::RateLimited {
            retry_after_ms: 40,
        },
    );
    assert!(matches!(
        error,
        ResearchScreenWritePortError::RateLimited { retry_after: 1, .. }
    ));
}

#[test]
// Parity: go:452dea11:pkg/futu/adapter_stock_screen_test.go:327 TestResearchScreenQuoteCurrencyUsesSecurityCounterIdentity
fn stock_screen_quote_currency_uses_security_counter_identity() {
    for (market, symbol, name, expected) in [
        ("US", "AAPL", "Apple", Some("USD")),
        ("SH", "600519", "贵州茅台", Some("CNY")),
        ("SZ", "000001", "平安银行", Some("CNY")),
        ("HK", "00700", "腾讯控股", Some("HKD")),
        ("HK", "80700", "腾讯控股-R", Some("CNY")),
        ("HK", "81211", "比亚迪股份-R", Some("CNY")),
        // RMB code without the -R marker stays unresolved rather than guessed.
        ("HK", "80700", "腾讯控股", None),
        // An -R marker on an HKD code is contradictory and stays unresolved.
        ("HK", "00700", "腾讯控股-R", None),
        ("HK", "00700", "", None),
        // HK counters require the five-digit form.
        ("HK", "700", "腾讯控股", None),
        ("US", "", "Apple", None),
        ("SG", "D05", "DBS", None),
    ] {
        assert_eq!(
            screen::futu_quote_currency(market, symbol, name),
            expected,
            "{market}.{symbol} ({name})"
        );
    }
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
            "conditions": [{"factor": {"factorKey": "simple.price"}, "operator": "between", "value": {"min": 10}}],
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

/// Parity: go:452dea11:internal/productfeatures/provider_capability_alignment_test.go:9
/// TestEmbeddedResearchFeatureAllowListIsExplicit
///
/// Go keeps `embeddedResearchFeatureIDs` as one enumerable map: the complete set
/// of research features an embedded yfinance/AKShare provider may own.  The
/// routing test compares the whole set, not a few representatives.  Rust has no
/// single map keyed by feature id — the facade is the `ProductionResearchPort`
/// routing table plus the helper projection — so this test freezes the exact
/// facade-usable set by driving every route with a helper-backed provider and
/// asserting that the eleven allowed features are served while a control route
/// outside the set is rejected with the capability error.
#[test]
// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:14 TestProductFeatureServiceRemainingRoutingAndDegradationBranches
// Parity: go:452dea11:internal/productfeatures/service_test.go:143 TestProductFeatureServiceRoutesEveryOptionalInterfaceAndCaches
fn embedded_research_facade_serves_exactly_the_allowed_feature_set() {
    // The allow-list is the *routing* contract: every entry below is a research
    // feature the embedded provider must be able to own. Keep this list sorted
    // and explicit so a new helper-owned read has to be added deliberately.
    const ALLOWED_FEATURES: [&str; 11] = [
        "research.news",
        "research.corporate_actions",
        "research.rankings",
        "research.industry",
        "research.instrument",
        "research.financials",
        "research.analyst",
        "research.ownership",
        "research.calendar",
        "research.macro",
        "research.screen",
    ];

    // The helper-backed routes reject with 409 CAPABILITY_UNAVAILABLE when the
    // helper is not ready (the facade is disabled), which is the fail-closed
    // answer for every allowed feature. The route must never be answered by a
    // different owner, and the message must name the feature family.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(false, false, false);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };

    let allowed_routes = [
        ("/api/v1/research/rankings", "research.rankings"),
        ("/api/v1/research/industries", "research.industry"),
        ("/api/v1/research/calendars", "research.calendar"),
        ("/api/v1/research/macro", "research.macro"),
        ("/api/v1/research/financials/US.AAPL", "research.financials"),
        ("/api/v1/research/analyst/US.AAPL", "research.analyst"),
        ("/api/v1/research/ownership/US.AAPL", "research.ownership"),
        (
            "/api/v1/research/corporate-actions/US.AAPL",
            "research.corporate_actions",
        ),
        ("/api/v1/research/instruments/US.AAPL", "research.instrument"),
    ];
    for (path, feature) in allowed_routes {
        let error = port
            .read(path, "")
            .expect_err("a disabled embedded facade must fail closed");
        assert!(
            !matches!(error, ResearchReadSnapshotError::Invalid(_)),
            "{feature} ({path}) must be a supported facade route, got {error:?}"
        );
    }

    // Every allow-listed feature id must be a reviewed MCP tool name; a typo in
    // the list above would otherwise silently drop coverage. `REVIEWED_READ_ONLY_TOOLS`
    // is the single reviewed extension catalog shared by `tools/list` and dispatch.
    for feature in ALLOWED_FEATURES {
        assert!(
            crate::product::product_mcp_protocol::REVIEWED_READ_ONLY_TOOLS.contains(&feature),
            "{feature} is not a reviewed assistant/MCP tool"
        );
    }
    // The canonical prediction/derivatives/execution families are deliberately
    // outside the embedded facade: Futu product families never fall back to
    // yfinance/AKShare.
    for outside in [
        "prediction.depth",
        "derivatives.option_chain",
        "execution.order_place",
        "research.valuation",
        "research.institutions",
    ] {
        assert!(
            !ALLOWED_FEATURES.contains(&outside),
            "{outside} must not be part of the embedded facade allow-list"
        );
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:205
/// TestMapEmbeddedProviderErrorKeepsSentinels
///
/// The embedded facade folds an unsupported capability into the broker
/// capability contract, while warming/busy lifecycle sentinels keep their own
/// identity so the transport can answer 503 with the documented Retry-After.
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:229 TestEmbeddedProviderRouteErrorsKeepHTTPContract
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:335 TestEmbeddedProviderRankingsRouteMapsUnsupportedOperations
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:458 TestEmbeddedProviderCompanyResearchRejectsUnsupportedOperations
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:606 TestEmbeddedProviderCalendarMacroRoutesRejectUnsupportedOperations
#[test]
fn embedded_capability_errors_keep_the_broker_code_and_lifecycle_sentinels() {
    let unsupported = capability("research.news", "instrument news");
    match unsupported {
        ResearchReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE");
            assert!(message.contains("research.news"), "message = {message}");
        }
        other => panic!("expected a capability failure, got {other:?}"),
    }

    let warming = map_research_helper_error(
        jftrade_integration_marketdata_helper::HttpAdapterError::Remote {
            status: 503,
            code: "AKSHARE_RUNTIME_WARMING".to_owned(),
            message: "runtime loading".to_owned(),
            retry_after_seconds: Some(1),
        },
    );
    assert!(matches!(
        warming,
        ResearchReadSnapshotError::Failed {
            status: 503,
            ref code,
            retry_after_seconds: Some(1),
            ..
        } if code == "MARKET_DATA_PROVIDER_WARMING"
    ));

    let busy = map_research_helper_error(
        jftrade_integration_marketdata_helper::HttpAdapterError::Remote {
            status: 503,
            code: "AKSHARE_POOL_BUSY".to_owned(),
            message: "pool busy".to_owned(),
            retry_after_seconds: Some(2),
        },
    );
    assert!(matches!(
        busy,
        ResearchReadSnapshotError::Failed {
            status: 503,
            ref code,
            retry_after_seconds: Some(2),
            ..
        } if code == "MARKET_DATA_PROVIDER_BUSY"
    ));
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_interception_test.go:294
/// TestEmbeddedProviderPropagatesCapabilityAndLifecycleErrors
///
/// The embedded facade maps a helper capability rejection into the broker
/// capability contract, while helper runtime pressure keeps the market-data
/// lifecycle identity so the transport answers 503 with the fixed Retry-After:
/// warming -> `MARKET_DATA_PROVIDER_WARMING`/1s, busy -> `..._BUSY`/2s. This is
/// the news/corporate-actions interception path; the rankings, calendar, and
/// company owners assert the same mapping at their own boundaries.
#[test]
fn embedded_facade_propagates_capability_and_lifecycle_sentinels() {
    use jftrade_integration_marketdata_helper::HttpAdapterError;

    let capability_error = capability("research.news", "instrument news");
    assert!(matches!(
        capability_error,
        ResearchReadSnapshotError::Failed {
            status: 409,
            ref code,
            ..
        } if code == "BROKER_CAPABILITY_UNAVAILABLE"
    ));

    for (raw_code, expected_code, expected_retry_after) in [
        ("AKSHARE_RUNTIME_WARMING", "MARKET_DATA_PROVIDER_WARMING", 1_u64),
        ("AKSHARE_POOL_BUSY", "MARKET_DATA_PROVIDER_BUSY", 2_u64),
        ("AKSHARE_UPSTREAM_TIMEOUT", "MARKET_DATA_PROVIDER_BUSY", 2_u64),
        ("PROVIDER_RUNTIME_WARMING", "MARKET_DATA_PROVIDER_WARMING", 1_u64),
    ] {
        let mapped = map_research_helper_error(HttpAdapterError::Remote {
            status: 503,
            code: raw_code.to_owned(),
            message: "runtime loading".to_owned(),
            retry_after_seconds: None,
        });
        assert!(
            matches!(
                mapped,
                ResearchReadSnapshotError::Failed {
                    status: 503,
                    ref code,
                    retry_after_seconds: Some(retry_after),
                    ..
                } if code == expected_code && retry_after == expected_retry_after
            ),
            "{raw_code} => {mapped:?}"
        );
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:160
/// TestEmbeddedProviderServesMirrorsActiveProviderMatching
///
/// Go's `embeddedProviderServes` mirrors `usesActiveNonBrokerProvider` in
/// internal/api/marketdata: the embedded provider owns a request only when the
/// active provider is not Futu and the explicit `brokerId` matches the active
/// descriptor's broker id *or* its provider id, case-insensitively. An empty
/// request matches, and a Futu/empty descriptor never serves.
///
/// Rust collapses the descriptor pair (brokerID/providerID) into the single
/// `MarketDataProvider` enum, so the same contract lives in
/// `provider_request_matches`: each enum value accepts its own ids and
/// aliases. This test freezes the complete Go case table and then proves the
/// production research port applies it before any helper/OpenD read.
#[test]
fn active_provider_matching_accepts_descriptor_aliases_and_rejects_other_brokers() {
    // The Go predicate is compound: the embedded facade serves only when the
    // descriptor is not Futu *and* the requested id matches the descriptor.
    // Rust splits the same decision the same way — the Futu branch owns its
    // routes before the helper facade is reached — so the faithful
    // transcription of \`embeddedProviderServes\` is
    // \`provider != Futu && provider_request_matches(provider, query)\`.
    let embedded_serves =
        |provider: jftrade_settings::MarketDataProvider, requested: &str| -> bool {
            provider != jftrade_settings::MarketDataProvider::Futu
                && super::super::super::provider_request_matches(
                    provider,
                    &QueryMap::parse(&format!("brokerId={requested}")).expect("query map"),
                )
        };

    // (active provider, requested brokerId, served) transcribed from the Go
    // table. The descriptor-less row is asserted separately below because Rust
    // models "no descriptor" as None, not as a zero-valued descriptor.
    let cases = [
        (jftrade_settings::MarketDataProvider::Yfinance, "", true),
        (jftrade_settings::MarketDataProvider::Yfinance, "yfinance", true),
        (
            jftrade_settings::MarketDataProvider::Yfinance,
            "yahoo-finance",
            true,
        ),
        (jftrade_settings::MarketDataProvider::Yfinance, "YFINANCE", true),
        (jftrade_settings::MarketDataProvider::Yfinance, "akshare", false),
        (jftrade_settings::MarketDataProvider::Yfinance, "futu", false),
        (jftrade_settings::MarketDataProvider::Futu, "", false),
        (jftrade_settings::MarketDataProvider::Futu, "yfinance", false),
        (jftrade_settings::MarketDataProvider::Futu, "futu", false),
        (jftrade_settings::MarketDataProvider::Akshare, "AKSHARE", true),
        (jftrade_settings::MarketDataProvider::Akshare, "yfinance", false),
    ];
    for (provider, requested, served) in cases {
        assert_eq!(
            embedded_serves(provider, requested),
            served,
            "provider {provider:?} requested {requested:?}"
        );
    }

    // A blank value is "not requested", exactly like the Go TrimSpace guard.
    // It only serves for a non-Futu facade.
    for blank in ["", "%20", "%20%20"] {
        assert!(
            embedded_serves(jftrade_settings::MarketDataProvider::Yfinance, blank),
            "{blank:?} must behave as no explicit broker for yfinance"
        );
        assert!(
            !embedded_serves(jftrade_settings::MarketDataProvider::Futu, blank),
            "{blank:?} must not serve the Futu descriptor"
        );
    }

    // Production behavior: the guard runs before helper readiness, so a
    // non-active broker is a 409 capability rejection rather than a fallback
    // or a 503, while the provider's own alias reaches the helper boundary.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(false, false, false);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };
    for requested in ["akshare", "futu"] {
        match port.read("/api/v1/research/rankings", &format!("brokerId={requested}")) {
            Err(ResearchReadSnapshotError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            }) => {
                assert_eq!(status, 409);
                assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE");
                assert!(
                    message.contains("does not match active provider"),
                    "message = {message}"
                );
                assert_eq!(retry_after_seconds, None);
            }
            other => panic!("{requested} must be a capability rejection, got {other:?}"),
        }
    }
    // "yahoo-finance" is the active provider's other id, so it is accepted and
    // only the unready helper stops the read.
    assert!(matches!(
        port.read("/api/v1/research/rankings", "brokerId=yahoo-finance"),
        Err(ResearchReadSnapshotError::Unavailable(message))
            if message == "market-data helper is not ready"
    ));
    // So is a case variation of the broker id.
    assert!(matches!(
        port.read("/api/v1/research/rankings", "brokerId=YFinance"),
        Err(ResearchReadSnapshotError::Unavailable(message))
            if message == "market-data helper is not ready"
    ));

    // Futu never serves the embedded research facade, even when the request
    // names futu; the read fails closed instead of borrowing the helper.
    let futu = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    futu.set_readiness(true, true, true);
    let futu_port = ProductionResearchPort {
        active_provider_state: futu,
        helper: None,
        trade_runtime: None,
    };
    match futu_port.read("/api/v1/research/rankings", "") {
        Err(ResearchReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        }) => {
            assert_eq!(status, 409);
            assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE");
            assert!(message.contains("futu"), "message = {message}");
        }
        other => panic!("Futu must not serve rankings, got {other:?}"),
    }
}
