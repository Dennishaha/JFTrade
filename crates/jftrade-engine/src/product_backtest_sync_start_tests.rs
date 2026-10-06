use super::product_backtest_sync_request::parse_sync_request;
use super::product_backtest_sync_request::validate_sync_lookback_window;
use super::product_backtest_sync_request::validate_sync_provider_capabilities;
use super::*;
use crate::product::product_backtest_execution::BacktestExecutionTaskRegistry;
use crate::product::{
    BacktestExecutionError, BacktestExecutionPort, BacktestExecutionRequest, ProductConfig,
    start_product,
};
use jftrade_integration_futu::{
    HistoricalKline, HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult,
    HistoricalSecurity,
};
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{StoredBacktestCandle, StoredBacktestSyncTask};
use std::sync::Mutex;

#[derive(Debug)]
struct FixtureExecution;

impl BacktestExecutionPort for FixtureExecution {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        Ok(json!({
            "runId": request.run_id,
            "bars": request.candles.len(),
            "marketDataProvider": request.market_data_provider,
        }))
    }
}

#[derive(Debug, Default)]
struct RecordingExecution {
    request: Mutex<Option<BacktestExecutionRequest>>,
}

impl BacktestExecutionPort for RecordingExecution {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        let bars = request.candles.len();
        self.request
            .lock()
            .expect("record execution request")
            .replace(request.clone());
        Ok(json!({
            "bars": bars,
            "marketDataProvider": request.market_data_provider,
        }))
    }
}

#[derive(Debug, Default)]
struct FutuHistoryFixture {
    calls: Mutex<Vec<Vec<u8>>>,
}

impl HistoricalKlineReadPort for FutuHistoryFixture {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, jftrade_integration_futu::HistoricalKlineError> {
        self.calls
            .lock()
            .expect("fixture calls")
            .push(query.next_req_key.clone());
        let first = query.next_req_key.is_empty();
        Ok(HistoricalKlineResult {
            security: HistoricalSecurity {
                market: query.market,
                code: query.symbol.clone(),
            },
            name: Some("Fixture security".to_owned()),
            klines: vec![HistoricalKline {
                time: if first {
                    "2026-08-01 08:00:00".to_owned()
                } else {
                    "2026-08-01 08:02:00".to_owned()
                },
                is_blank: false,
                high_price: Some(102.0),
                open_price: Some(100.0),
                low_price: Some(99.0),
                close_price: Some(101.0),
                volume: Some(12),
                turnover: None,
                change_rate: None,
            }],
            next_req_key: if first { vec![1, 2, 3] } else { Vec::new() },
        })
    }
}

#[test]
fn sync_request_plans_intervals_like_go() {
    // Parity: go:452dea11:internal/backtest/sync_test.go:367 TestPlanSyncIntervals
    let cases: [(&str, &str, Vec<&str>, Vec<&str>); 4] = [
        ("HK.00700", "", vec!["1m", "1m", "3d"], vec!["1m", "1d"]),
        ("HK.00700", "", vec!["3d", "2w", "2h"], vec!["1d", "1h"]),
        ("US.AAPL", "extended", vec!["1d", "3d", "1w"], vec!["1h"]),
        ("US.AAPL", "regular", vec!["1d"], vec!["1d"]),
    ];
    for (symbol, session_scope, requested, want) in cases {
        let payload = json!({
            "market": symbol.split('.').next().unwrap_or(""),
            "symbol": symbol,
            "intervals": requested,
            "since": "2026-08-01T00:00:00Z",
            "until": "2026-08-02T00:00:00Z",
            "sessionScope": session_scope,
        });
        let parsed = parse_sync_request(&payload).expect("planned sync intervals");
        let planned = parsed
            .intervals
            .iter()
            .map(String::as_str)
            .collect::<Vec<_>>();
        assert_eq!(planned, want, "{symbol} {session_scope}");
    }
}

#[test]
// Parity: go:452dea11:internal/api/backtest/routes_test.go:21 TestSyncRouteClassifiesRequestErrorsAsBadRequest
fn sync_request_rejects_invalid_ranges_and_intervals() {
    let invalid_symbol = json!({
        "symbol": "bad symbol",
        "since": "2026-08-01T00:00:00Z",
        "until": "2026-08-02T00:00:00Z"
    });
    assert!(matches!(
        parse_sync_request(&invalid_symbol),
        Err(BacktestsWritePortError::BadRequest(_))
    ));

    let malformed_since = json!({
        "market": "HK",
        "code": "00700",
        "since": "bad"
    });
    assert!(matches!(
        parse_sync_request(&malformed_since),
        Err(BacktestsWritePortError::BadRequest(_))
    ));

    let invalid_interval = json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["2m"],
        "since": "2026-08-01T00:00:00Z",
        "until": "2026-08-02T00:00:00Z"
    });
    assert!(matches!(
        parse_sync_request(&invalid_interval),
        Err(BacktestsWritePortError::BadRequest(_))
    ));

    let invalid_range = json!({
        "market": "US",
        "code": "AAPL",
        "since": "2026-08-02T00:00:00Z",
        "until": "2026-08-01T00:00:00Z"
    });
    assert!(matches!(
        parse_sync_request(&invalid_range),
        Err(BacktestsWritePortError::BadRequest(_))
    ));

    for payload in [
        json!({"market": "US", "code": "AAPL", "startDate": "2026-08-01"}),
        json!({"market": "US", "code": "AAPL", "endDate": "2026-08-02"}),
        json!({"market": "US", "code": "AAPL", "startDate": "2026-02-30", "endDate": "2026-03-01"}),
        json!({"market": "US", "code": "AAPL", "since": "2026-08-01T00:00:00Z", "until": "bad"}),
        json!({"market": "EU", "code": "ABC", "since": "2026-08-01T00:00:00Z", "until": "2026-08-02T00:00:00Z"}),
    ] {
        assert!(matches!(
            parse_sync_request(&payload),
            Err(BacktestsWritePortError::BadRequest(_))
        ), "payload must be rejected: {payload}");
    }
}

// Parity: go:452dea11:internal/api/backtest/routes_test.go:21 TestSyncRouteClassifiesRequestErrorsAsBadRequest
#[tokio::test]
async fn backtest_sync_http_rejects_request_errors_before_queuing() {
    let (port, directory) = production_port();
    let port = std::sync::Arc::new(port);
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("http-settings.json"),
    )
    .expect("config")
    .with_backtests_write_port(port.clone());
    let handle = start_product(config).await.expect("start product HTTP server");
    for body in [
        r#"{"symbol":"bad symbol"}"#,
        r#"{"market":"HK","code":"00700","since":"bad"}"#,
        r#"{"market":"HK","code":"00700","since":"2024-01-03T00:00:00Z","until":"2024-01-02T00:00:00Z"}"#,
    ] {
        let (status, headers, response) =
            crate::product::tests::request_json_with_status_and_headers(
                handle.startup_record().address,
                "POST",
                "/api/v1/backtests/sync",
                Some(body),
                &[],
            )
            .await;
        assert_eq!(status, 400, "request {body}: {response}");
        assert_eq!(headers["content-type"], "application/json; charset=utf-8");
        assert_eq!(response["ok"], false, "request {body}");
        assert_eq!(response["error"]["code"], "BAD_REQUEST", "request {body}");
        assert!(response.get("data").is_none(), "errors must not contain success data");
        assert!(port.sync_tasks.list_active().expect("active sync tasks").is_empty());
    }
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:55 TestBacktestSyncRejectsActualAKShareOneYearUSFiveMinuteRange
///
/// Go rejects a one-year AKShare US 5m sync through the provider capability
/// window (`HistoricalLookbackDays: {"US:5m": 5}`) before any provider work,
/// with the message `provider akshare limits 5m history to 5 days`. Rust now
/// applies the same guard on the parsed sync window and keeps other providers
/// and windows inside the limit working.
#[test]
fn akshare_sync_rejects_history_beyond_the_intraday_lookback_window() {
    let beyond = json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["5m"],
        "startDate": "2025-07-13",
        "endDate": "2026-07-13",
        "rehabType": "none",
        "sessionScope": "regular",
    });
    let request = parse_sync_request(&beyond).expect("parse akshare sync");
    let error = validate_sync_lookback_window("akshare", &request)
        .expect_err("one-year 5m AKShare sync must be rejected");
    match error {
        BacktestsWritePortError::BadRequest(message) => {
            assert_eq!(message, "provider akshare limits 5m history to 5 days")
        }
        other => panic!("expected 400 BAD_REQUEST, got {other:?}"),
    }

    let now = time::OffsetDateTime::now_utc();
    let within_limit = json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["5m"],
        "since": (now - time::Duration::days(3)).format(&time::format_description::well_known::Rfc3339).expect("since"),
        "until": now.format(&time::format_description::well_known::Rfc3339).expect("until"),
        "rehabType": "none",
        "sessionScope": "regular",
    });
    let request = parse_sync_request(&within_limit).expect("parse recent akshare sync");
    validate_sync_lookback_window("akshare", &request)
        .expect("a 3-day AKShare 5m window is inside the provider limit");
    validate_sync_lookback_window("yfinance", &request)
        .expect("yfinance has no AKShare intraday window");
}

#[test]
fn sync_request_defaults_match_public_contract_without_provider_success() {
    let request = parse_sync_request(&Value::Null).expect("null request uses documented defaults");
    assert_eq!(request.symbol, "HK.00700");
    assert_eq!(request.session_scope, "regular");
    assert_eq!(request.rehab_type, "forward");
    assert_eq!(
        request.intervals,
        ["1m", "5m", "15m", "30m", "1h", "1d", "1w"]
    );
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:141 TestProviderHistoricalSourceAppliesMarketScopedLookback
/// The AKShare intraday window is market scoped: the `US:5m` capability limits
/// only US five-minute history, while the same interval on HK stays inside the
/// provider limit and the one-minute rule applies to every market.
fn akshare_lookback_windows_are_scoped_to_the_declared_market() {
    let now = time::OffsetDateTime::now_utc();
    let since = (now - time::Duration::days(6))
        .format(&time::format_description::well_known::Rfc3339)
        .expect("since");
    let until = now
        .format(&time::format_description::well_known::Rfc3339)
        .expect("until");
    let request = |market: &str, code: &str, interval: &str| {
        parse_sync_request(&json!({
            "market": market,
            "code": code,
            "intervals": [interval],
            "since": since,
            "until": until,
            "rehabType": "none",
            "sessionScope": "regular",
        }))
        .expect("parse akshare sync request")
    };

    let us_5m = request("US", "AAPL", "5m");
    assert!(
        validate_sync_lookback_window("akshare", &us_5m).is_err(),
        "US five-minute history beyond five days must stay rejected"
    );

    let hk_5m = request("HK", "00700", "5m");
    validate_sync_lookback_window("akshare", &hk_5m)
        .expect("the US-only five-minute window must not constrain HK");

    let hk_1m = request("HK", "00700", "1m");
    assert!(
        validate_sync_lookback_window("akshare", &hk_1m).is_err(),
        "one-minute history is limited on every market"
    );
}

#[test]
// Parity: go:452dea11:internal/api/backtest/routes_boundaries_test.go:78 TestBacktestSyncRouteRejectsObsoleteSessionScope
fn sync_request_session_scope_parity_with_go() {
    // Parity: go:452dea11:internal/backtest/sync_test.go:412 TestParseSessionScope
    for (input, want) in [
        ("", "regular"),
        ("regular", "regular"),
        ("extended", "extended"),
    ] {
        let payload = json!({
            "market": "US",
            "code": "AAPL",
            "since": "2026-08-01T00:00:00Z",
            "until": "2026-08-02T00:00:00Z",
            "sessionScope": input,
        });
        let req = parse_sync_request(&payload).expect("valid session scope");
        assert_eq!(req.session_scope, want);
    }

    for invalid in ["legacy", "unknown", " regular ", "EXTENDED"] {
        let payload = json!({
            "market": "US",
            "code": "AAPL",
            "since": "2026-08-01T00:00:00Z",
            "until": "2026-08-02T00:00:00Z",
            "sessionScope": invalid,
        });
        assert!(matches!(
            parse_sync_request(&payload),
            Err(BacktestsWritePortError::BadRequest(_))
        ));
    }
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:87 TestProviderHistoricalSourceRejectsExtendedSessionsOutsideUSIntraday
fn sync_request_rejects_extended_session_scope_outside_us_intraday() {
    let payload = json!({
        "market": "HK",
        "code": "00700",
        "intervals": ["1m"],
        "since": "2026-08-01T00:00:00Z",
        "until": "2026-08-02T00:00:00Z",
        "sessionScope": "extended",
    });
    let error = parse_sync_request(&payload)
        .expect_err("HK extended session sync must be rejected before provider work");
    assert!(
        matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("US intraday"))
    );
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:108 TestProviderHistoricalSourceValidatesAdjustmentAndLookback
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:162 TestProviderHistoricalSourceEnforcesProviderAdjustmentMatrix
fn sync_request_validates_provider_adjustment_and_lookback_capabilities() {
    let backward = parse_sync_request(&json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["1d"],
        "since": "2026-09-29T00:00:00Z",
        "until": "2026-09-30T00:00:00Z",
        "rehabType": "backward",
        "sessionScope": "regular",
    }))
    .expect("parse yfinance adjustment request");
    let error = validate_sync_provider_capabilities("yfinance", &backward)
        .expect_err("yfinance must reject backward adjustment");
    assert!(
        matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("backward price adjustment"))
    );

    let beyond_lookback = parse_sync_request(&json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["1m"],
        "since": "2026-09-16T00:00:00Z",
        "until": "2026-09-25T00:00:00Z",
        "rehabType": "none",
        "sessionScope": "regular",
    }))
    .expect("parse yfinance lookback request");
    let error = validate_sync_provider_capabilities("yfinance", &beyond_lookback)
        .expect_err("yfinance 1m history beyond seven days must be rejected");
    assert!(
        matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("limits 1m history to 7 days"))
    );

    let window = recent_helper_sync_window();
    let supported = parse_sync_request(&json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["5m"],
        "since": window.since,
        "until": window.until,
        "rehabType": "forward",
        "sessionScope": "regular",
    }))
    .expect("parse supported yfinance request");
    validate_sync_provider_capabilities("yfinance", &supported)
        .expect("yfinance forward 5m request is supported");
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:206 TestKLineSyncPreflightRejectsStaticCapabilityMismatchAndUnknownProvider
fn production_sync_rejects_provider_capability_before_queuing() {
    let (mut port, _directory) = production_port();
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind helper probe");
    let address = listener.local_addr().expect("helper probe address");
    drop(listener);
    port.helper = Some(
        jftrade_integration_marketdata_helper::HelperClient::new(
            jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{address}"),
                bearer_token: None,
                request_timeout: std::time::Duration::from_secs(1),
                max_attempts: 1,
                retry_delay: std::time::Duration::ZERO,
            },
        )
        .expect("helper client"),
    );
    let error = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "US",
                "code": "AAPL",
                "intervals": ["1d"],
                "since": "2026-09-29T00:00:00Z",
                "until": "2026-09-30T00:00:00Z",
                "rehabType": "backward",
                "marketDataProvider": "yfinance",
            }),
        })
        .expect_err("unsupported yfinance adjustment must fail before queueing");
    assert!(
        matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("backward price adjustment"))
    );
    assert!(
        port.sync_tasks
            .list_active()
            .expect("active sync tasks")
            .is_empty()
    );

    let error = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({"marketDataProvider": "unknown"}),
        })
        .expect_err("unknown provider must fail before queueing");
    assert!(
        matches!(error, BacktestsWritePortError::BadRequest(message) if message.contains("unsupported marketDataProvider"))
    );
    assert!(
        port.sync_tasks
            .list_active()
            .expect("active sync tasks")
            .is_empty()
    );
}

// Keep provider-I/O scenarios inside the production rolling lookback window.
// Capture UTC midnight once so request, candle and pagination cursors share
// one window even if the test crosses midnight. Capability rejection cases
// above continue to assert that out-of-window requests are rejected.
struct RecentHelperSyncWindow {
    since: String,
    until: String,
    candle_at: String,
    forward_cursor: String,
}

fn recent_helper_sync_window() -> RecentHelperSyncWindow {
    let until = time::OffsetDateTime::now_utc().date().midnight().assume_utc();
    let since = until - time::Duration::days(1);
    let format = |at: time::OffsetDateTime| {
        at.format(&time::format_description::well_known::Rfc3339)
            .expect("helper fixture timestamp")
    };
    RecentHelperSyncWindow {
        since: format(since),
        until: format(until),
        candle_at: format(since + time::Duration::hours(12)),
        forward_cursor: format(until + time::Duration::days(1)),
    }
}

fn production_port() -> (ProductionBacktestPort, tempfile::TempDir) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let runs_path = directory.path().join("backtest-runs.db");
    let connection = rusqlite::Connection::open(&runs_path).expect("create runs database");
    jftrade_store_sqlite::initialize_current(&connection, "backtest-runs")
        .expect("initialize runs database");
    drop(connection);
    let runs = std::sync::Arc::new(
        jftrade_store_sqlite::BacktestRunStore::open_existing(
            &runs_path,
            jftrade_store_sqlite::BACKTEST_RUNS_PRODUCTION_PROFILE,
        )
        .expect("open runs store"),
    );
    let sync_tasks = std::sync::Arc::new(jftrade_store_sqlite::BacktestSyncTaskStore::new(
        std::sync::Arc::clone(&runs),
    ));
    let market_data_path = directory.path().join("backtest.db");
    let connection = rusqlite::Connection::open(&market_data_path).expect("create market database");
    jftrade_store_sqlite::initialize_current(&connection, "backtest")
        .expect("initialize market database");
    drop(connection);
    let market_data = std::sync::Arc::new(
        jftrade_store_sqlite::BacktestMarketDataStore::open_existing(
            &market_data_path,
            jftrade_store_sqlite::BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
        )
        .expect("open market store"),
    );
    let strategy_path = directory.path().join("strategy-definitions.db");
    let connection = rusqlite::Connection::open(&strategy_path).expect("create strategy database");
    jftrade_store_sqlite::initialize_current(&connection, "strategy")
        .expect("initialize strategy database");
    drop(connection);
    let strategy_definitions = std::sync::Arc::new(
        jftrade_store_sqlite::StrategyDefinitionStore::open_existing(
            &strategy_path,
            jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .expect("open strategy store"),
    );
    (
        ProductionBacktestPort {
            store: runs,
            sync_tasks,
            _market_data_store: market_data,
            helper: None,
            trade_runtime: None,
            backtest_market_data_provider_state: std::sync::Arc::new(
                crate::product::BacktestMarketDataProviderState::new(MarketDataProvider::Yfinance),
            ),
            sync_workers: std::sync::Arc::new(BacktestSyncWorkerRegistry::default()),
            execution: None,
            pine_readiness: None,
            execution_workers: std::sync::Arc::new(BacktestExecutionTaskRegistry::default()),
            strategy_definitions,
        },
        directory,
    )
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/servercoretest/backtest_provider_runtime_test.go:27 TestBacktestSyncUsesAssembledMarketDataRuntime
async fn production_futu_sync_uses_opend_reader_and_persists_candles() {
    let (mut port, _directory) = production_port();
    let runtime = std::sync::Arc::new(
        crate::product::product_production_ports::SharedTradeReadRuntime::default(),
    );
    let fixture = std::sync::Arc::new(FutuHistoryFixture::default());
    runtime.set_historical_klines(Some(fixture.clone()));
    port.trade_runtime = Some(runtime);
    port.backtest_market_data_provider_state
        .set(MarketDataProvider::Futu);
    let response = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "HK",
                "code": "00700",
                "intervals": ["1m"],
                "since": "2026-08-01T00:00:00Z",
                "until": "2026-08-01T00:03:00Z",
                "rehabType": "forward"
            }),
        })
        .expect("start Futu sync");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected sync response");
    };
    let task_id = data["taskId"].as_str().expect("task id").to_owned();
    for _ in 0..100 {
        if let Some(task) = port.sync_tasks.get(&task_id).expect("task")
            && matches!(task.status.as_str(), "completed" | "failed")
        {
            assert_eq!(task.status, "completed", "task error: {:?}", task.error);
            let candles = port
                ._market_data_store
                .read_candles(
                    "futu",
                    "HK.00700",
                    "1m",
                    "forward",
                    "regular",
                    1_785_542_400_000,
                    1_785_542_580_000,
                )
                .expect("read synced candles");
            assert_eq!(candles.len(), 2);
            assert_eq!(fixture.calls.lock().expect("fixture calls").len(), 2);
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("Futu sync did not complete");
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:226 TestProviderHistoricalSourceFetchesAndParsesProviderPage
async fn production_helper_sync_forwards_page_query_and_persists_provider_values() {
    let window = recent_helper_sync_window();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind helper page fixture");
    let helper_address = listener.local_addr().expect("helper page address");
    let request_capture = std::sync::Arc::new(std::sync::Mutex::new(None));
    let capture_for_server = std::sync::Arc::clone(&request_capture);
    let candle_at = window.candle_at.clone();
    let helper_task = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("helper page connection");
        let mut request = Vec::new();
        loop {
            let mut chunk = [0_u8; 1024];
            let read = tokio::io::AsyncReadExt::read(&mut stream, &mut chunk)
                .await
                .expect("read helper page request");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
            if request.windows(4).any(|window| window == b"\r\n\r\n") {
                break;
            }
        }
        *capture_for_server.lock().expect("request capture") =
            Some(String::from_utf8_lossy(&request).into_owned());
        let body = serde_json::to_string(&json!({
            "market": "US",
            "symbol": "AAPL",
            "instrumentId": "US.AAPL",
            "period": "1m",
            "extendedHours": false,
            "candles": [{
                "at": candle_at,
                "open": "100.25",
                "high": 102.5,
                "low": 99.5,
                "close": 101.75,
                "volume": 1200,
                "session": "regular"
            }],
            "totalReturned": 1,
            "hasMore": false,
            "source": "yfinance",
            "adjustment": "forward"
        }))
        .expect("serialize helper page");
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        tokio::io::AsyncWriteExt::write_all(&mut stream, response.as_bytes())
            .await
            .expect("write helper page response");
    });

    let (mut port, _directory) = production_port();
    port.helper = Some(
        jftrade_integration_marketdata_helper::HelperClient::new(
            jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{helper_address}"),
                bearer_token: None,
                request_timeout: std::time::Duration::from_secs(2),
                max_attempts: 1,
                retry_delay: std::time::Duration::ZERO,
            },
        )
        .expect("helper client"),
    );
    let response = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "US",
                "code": "AAPL",
                "intervals": ["1m"],
                "since": window.since,
                "until": window.until,
                "rehabType": "forward",
                "sessionScope": "regular",
                "marketDataProvider": "yfinance"
            }),
        })
        .expect("start helper sync");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected sync response");
    };
    let task_id = data["taskId"].as_str().expect("task id").to_owned();
    for _ in 0..100 {
        if let Some(task) = port.sync_tasks.get(&task_id).expect("task")
            && matches!(task.status.as_str(), "completed" | "failed")
        {
            assert_eq!(task.status, "completed", "task error: {:?}", task.error);
            let candles = port
                ._market_data_store
                .read_candles(
                    "yfinance",
                    "US.AAPL",
                    "1m",
                    "forward",
                    "regular",
                    0,
                    i64::MAX,
                )
                .expect("read synced candles");
            assert_eq!(candles.len(), 1);
            assert_eq!(candles[0].close, "101.75");
            assert_eq!(candles[0].volume, "1200");
            helper_task.await.expect("helper page task");
            let request = request_capture
                .lock()
                .expect("request capture")
                .clone()
                .expect("captured helper request");
            assert!(request.contains("period=1m"), "request = {request}");
            assert!(
                request.contains("adjustment=forward"),
                "request = {request}"
            );
            assert!(request.contains("limit=1000"), "request = {request}");
            assert!(request.contains("sessions=regular"), "request = {request}");
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("helper sync did not complete");
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/backtestapp/historical_source_test.go:226 TestProviderHistoricalSourceFetchesAndParsesProviderPage
async fn production_helper_sync_preserves_non_retryable_provider_fetch_error() {
    let window = recent_helper_sync_window();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind failing helper fixture");
    let helper_address = listener.local_addr().expect("failing helper address");
    let helper_task = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("failing helper connection");
        let mut request = [0_u8; 1024];
        let _ = tokio::io::AsyncReadExt::read(&mut stream, &mut request)
            .await
            .expect("read failing helper request");
        let body = "provider exploded";
        let response = format!(
            "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        tokio::io::AsyncWriteExt::write_all(&mut stream, response.as_bytes())
            .await
            .expect("write failing helper response");
    });

    let (mut port, _directory) = production_port();
    port.helper = Some(
        jftrade_integration_marketdata_helper::HelperClient::new(
            jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{helper_address}"),
                bearer_token: None,
                request_timeout: std::time::Duration::from_secs(2),
                max_attempts: 1,
                retry_delay: std::time::Duration::ZERO,
            },
        )
        .expect("helper client"),
    );
    let response = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "US",
                "code": "AAPL",
                "intervals": ["1m"],
                "since": window.since,
                "until": window.until,
                "rehabType": "forward",
                "sessionScope": "regular",
                "marketDataProvider": "yfinance"
            }),
        })
        .expect("start failing helper sync");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected sync response");
    };
    let task_id = data["taskId"].as_str().expect("task id").to_owned();
    for _ in 0..100 {
        if let Some(task) = port.sync_tasks.get(&task_id).expect("task")
            && task.status == "failed"
        {
            assert!(
                task.error
                    .as_deref()
                    .is_some_and(|error| error.contains("provider exploded")),
                "task error = {:?}",
                task.error
            );
            helper_task.await.expect("failing helper task");
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("failing helper sync did not reach failed state");
}

#[tokio::test]
// Parity: go:452dea11:internal/backtest/historical_source_test.go:147 TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight
async fn production_helper_sync_retries_transient_page_and_records_retry() {
    let window = recent_helper_sync_window();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind retry helper fixture");
    let helper_address = listener.local_addr().expect("retry helper address");
    let attempts = std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let attempts_for_server = std::sync::Arc::clone(&attempts);
    let candle_at = window.candle_at.clone();
    let helper_task = tokio::spawn(async move {
        for _ in 0..2 {
            let (mut stream, _) = listener.accept().await.expect("retry helper connection");
            let mut request = Vec::new();
            loop {
                let mut chunk = [0_u8; 1024];
                let read = tokio::io::AsyncReadExt::read(&mut stream, &mut chunk)
                    .await
                    .expect("read retry helper request");
                request.extend_from_slice(&chunk[..read]);
                if request.windows(4).any(|window| window == b"\r\n\r\n") {
                    break;
                }
            }
            let attempt = attempts_for_server.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            let (status, body) = if attempt == 0 {
                (
                    "503 Service Unavailable",
                    r#"{"error":{"code":"UPSTREAM","message":"temporary provider failure"}}"#
                        .to_owned(),
                )
            } else {
                (
                    "200 OK",
                    serde_json::to_string(&json!({
                        "market": "US",
                        "symbol": "AAPL",
                        "instrumentId": "US.AAPL",
                        "period": "1m",
                        "extendedHours": false,
                        "candles": [{
                            "at": candle_at,
                            "open": "100.25",
                            "high": 102.5,
                            "low": 99.5,
                            "close": 101.75,
                            "volume": 1200,
                            "session": "regular"
                        }],
                        "totalReturned": 1,
                        "hasMore": false,
                        "source": "yfinance",
                        "adjustment": "forward"
                    }))
                    .expect("serialize retry helper page"),
                )
            };
            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            tokio::io::AsyncWriteExt::write_all(&mut stream, response.as_bytes())
                .await
                .expect("write retry helper response");
        }
    });

    let (mut port, _directory) = production_port();
    port.helper = Some(
        jftrade_integration_marketdata_helper::HelperClient::new(
            jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{helper_address}"),
                bearer_token: None,
                request_timeout: std::time::Duration::from_secs(2),
                max_attempts: 1,
                retry_delay: std::time::Duration::ZERO,
            },
        )
        .expect("helper client"),
    );
    let response = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "US",
                "code": "AAPL",
                "intervals": ["1m"],
                "since": window.since,
                "until": window.until,
                "rehabType": "forward",
                "sessionScope": "regular",
                "marketDataProvider": "yfinance"
            }),
        })
        .expect("start retrying helper sync");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected sync response");
    };
    let task_id = data["taskId"].as_str().expect("task id").to_owned();
    for _ in 0..600 {
        if let Some(task) = port.sync_tasks.get(&task_id).expect("task")
            && matches!(task.status.as_str(), "completed" | "failed")
        {
            assert_eq!(task.status, "completed", "task error: {:?}", task.error);
            assert_eq!(task.retries, 1);
            assert_eq!(attempts.load(std::sync::atomic::Ordering::SeqCst), 2);
            helper_task.await.expect("retry helper task");
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("retrying helper sync did not complete");
}

#[tokio::test]
// Parity: go:452dea11:internal/backtest/historical_source_test.go:268 TestHistoricalKLineSyncerRejectsBrokenPagination
async fn production_helper_sync_rejects_broken_pagination_cursors() {
    let window = recent_helper_sync_window();
    let cases = [
        ("missing cursor", None, false),
        ("forward cursor", Some(window.forward_cursor.as_str()), false),
        ("cursor reaches boundary", Some(window.since.as_str()), true),
    ];

    for (name, next_before, expected_success) in cases {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind pagination helper fixture");
        let helper_address = listener.local_addr().expect("pagination helper address");
        let response_next_before = next_before.map(str::to_owned);
        let candle_at = window.candle_at.clone();
        let helper_task = tokio::spawn(async move {
            let (mut stream, _) = listener
                .accept()
                .await
                .expect("pagination helper connection");
            let mut request = Vec::new();
            loop {
                let mut chunk = [0_u8; 1024];
                let read = tokio::io::AsyncReadExt::read(&mut stream, &mut chunk)
                    .await
                    .expect("read pagination helper request");
                if read == 0 {
                    break;
                }
                request.extend_from_slice(&chunk[..read]);
                if request.windows(4).any(|window| window == b"\r\n\r\n") {
                    break;
                }
            }
            let body = serde_json::to_string(&json!({
                "market": "US",
                "symbol": "AAPL",
                "instrumentId": "US.AAPL",
                "period": "1m",
                "extendedHours": false,
                "candles": [{
                    "at": candle_at,
                    "open": "100.25",
                    "high": 102.5,
                    "low": 99.5,
                    "close": 101.75,
                    "volume": 1200,
                    "session": "regular"
                }],
                "totalReturned": 1,
                "hasMore": true,
                "nextBefore": response_next_before,
                "source": "yfinance",
                "adjustment": "forward"
            }))
            .expect("serialize pagination helper page");
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            tokio::io::AsyncWriteExt::write_all(&mut stream, response.as_bytes())
                .await
                .expect("write pagination helper response");
        });

        let (mut port, _directory) = production_port();
        port.helper = Some(
            jftrade_integration_marketdata_helper::HelperClient::new(
                jftrade_integration_marketdata_helper::HelperClientConfig {
                    base_url: format!("http://{helper_address}"),
                    bearer_token: None,
                    request_timeout: std::time::Duration::from_secs(2),
                    max_attempts: 1,
                    retry_delay: std::time::Duration::ZERO,
                },
            )
            .expect("helper client"),
        );
        let response = port
            .mutate(&BacktestsWriteInput::Sync {
                payload: json!({
                    "market": "US",
                    "code": "AAPL",
                    "intervals": ["1m"],
                    "since": window.since,
                    "until": window.until,
                    "rehabType": "forward",
                    "sessionScope": "regular",
                    "marketDataProvider": "yfinance"
                }),
            })
            .expect("start pagination sync");
        let BacktestsWritePortResult::Data(data) = response else {
            panic!("unexpected pagination sync response for {name}");
        };
        let task_id = data["taskId"].as_str().expect("pagination task id");
        let mut terminal = false;
        for _ in 0..600 {
            if let Some(task) = port.sync_tasks.get(task_id).expect("pagination task")
                && matches!(task.status.as_str(), "completed" | "failed")
            {
                terminal = true;
                assert_eq!(task.status == "completed", expected_success, "{name}: {task:?}");
                if expected_success {
                    assert_eq!(task.error, None, "{name}: successful task has an error");
                    let candles = port
                        ._market_data_store
                        .read_candles(
                            "yfinance",
                            "US.AAPL",
                            "1m",
                            "forward",
                            "regular",
                            0,
                            i64::MAX,
                        )
                        .expect("read boundary cursor candles");
                    assert_eq!(candles.len(), 1, "{name}: boundary page must persist one candle");
                } else {
                    assert!(
                        task.error.as_deref().is_some_and(|error| {
                            error.contains("nextBefore") || error.contains("pagination")
                        }),
                        "{name}: expected pagination error, got {:?}",
                        task.error
                    );
                }
                break;
            }
            tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        }
        assert!(terminal, "{name}: pagination sync did not reach a terminal state");
        helper_task.await.expect("pagination helper task");
    }
}

#[tokio::test]
// Parity: go:452dea11:internal/backtest/historical_source_test.go:111 TestHistoricalKLineSyncerCancelsInFlightProviderPage
async fn production_helper_sync_cancel_aborts_in_flight_request() {
    let window = recent_helper_sync_window();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind blocking helper fixture");
    let helper_address = listener.local_addr().expect("blocking helper address");
    let (request_started_tx, request_started_rx) = tokio::sync::oneshot::channel();
    let (connection_closed_tx, connection_closed_rx) = tokio::sync::oneshot::channel();
    let helper_task = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept helper request");
        let mut request = Vec::new();
        loop {
            let mut chunk = [0_u8; 1024];
            let read = tokio::io::AsyncReadExt::read(&mut stream, &mut chunk)
                .await
                .expect("read helper request");
            request.extend_from_slice(&chunk[..read]);
            if request.windows(4).any(|window| window == b"\r\n\r\n") {
                break;
            }
        }
        request_started_tx.send(()).expect("signal request start");
        let mut probe = [0_u8; 1];
        let read = tokio::io::AsyncReadExt::read(&mut stream, &mut probe)
            .await
            .expect("observe helper connection close");
        connection_closed_tx
            .send(read)
            .expect("signal helper connection close");
    });

    let (mut port, _directory) = production_port();
    port.helper = Some(
        jftrade_integration_marketdata_helper::HelperClient::new(
            jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: format!("http://{helper_address}"),
                bearer_token: None,
                request_timeout: std::time::Duration::from_secs(30),
                max_attempts: 1,
                retry_delay: std::time::Duration::ZERO,
            },
        )
        .expect("helper client"),
    );
    let response = port
        .mutate(&BacktestsWriteInput::Sync {
            payload: json!({
                "market": "US",
                "code": "AAPL",
                "intervals": ["1m"],
                "since": window.since,
                "until": window.until,
                "rehabType": "forward",
                "sessionScope": "regular",
                "marketDataProvider": "yfinance"
            }),
        })
        .expect("start helper sync");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected sync response");
    };
    let task_id = data["taskId"].as_str().expect("task id").to_owned();
    tokio::time::timeout(std::time::Duration::from_secs(2), request_started_rx)
        .await
        .expect("helper request did not start")
        .expect("request-start signal dropped");

    let cancelled = port
        .mutate(&BacktestsWriteInput::CancelSync {
            task_id: task_id.clone(),
        })
        .expect("cancel helper sync");
    assert_eq!(cancelled, BacktestsWritePortResult::SyncCancelled(true));
    let read = tokio::time::timeout(std::time::Duration::from_secs(2), connection_closed_rx)
        .await
        .expect("in-flight helper request was not aborted")
        .expect("connection-close signal dropped");
    assert_eq!(
        read, 0,
        "provider connection should close after cancellation"
    );
    helper_task.await.expect("helper task");
}

#[test]
// Parity: go:452dea11:internal/api/backtest/routes_boundaries_test.go:53 TestBacktestSyncRouteReturnsTaskForValidRequest
// Parity: go:452dea11:internal/api/backtest/routes_progress_test.go:71 TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound
fn production_sync_read_projects_persisted_task() {
    let (port, _directory) = production_port();
    port.sync_tasks
        .create(StoredBacktestSyncTask {
            task_id: "sync-production".to_owned(),
            status: "running".to_owned(),
            symbol: "US.AAPL".to_owned(),
            market_data_provider: "yfinance".to_owned(),
            total_intervals: 2,
            completed_intervals: 1,
            total_batches: 2,
            completed_batches: 1,
            current_interval: "1d".to_owned(),
            retries: 0,
            error: None,
            started_at: "2026-08-29T00:00:00Z".to_owned(),
            updated_at: "2026-08-29T00:01:00Z".to_owned(),
            revision: 0,
        })
        .expect("persist task");
    let projected = port
        .progress("sync-production")
        .expect("project task")
        .unwrap();
    assert_eq!(projected["status"], "running");
    assert_eq!(projected["completedIntervals"], 1);
    assert!(port.progress("missing").expect("missing task").is_none());
}

#[test]
// Parity: go:452dea11:internal/api/backtest/routes_progress_test.go:71 TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound
fn production_sync_cancel_matches_not_found_for_terminal_task() {
    let (port, _directory) = production_port();
    port.sync_tasks
        .create(StoredBacktestSyncTask {
            task_id: "sync-terminal".to_owned(),
            status: "completed".to_owned(),
            symbol: "US.AAPL".to_owned(),
            market_data_provider: "yfinance".to_owned(),
            total_intervals: 0,
            completed_intervals: 0,
            total_batches: 0,
            completed_batches: 0,
            current_interval: String::new(),
            retries: 0,
            error: None,
            started_at: "2026-08-29T00:00:00Z".to_owned(),
            updated_at: "2026-08-29T00:00:00Z".to_owned(),
            revision: 0,
        })
        .expect("persist terminal task");
    let result = port.mutate(&BacktestsWriteInput::CancelSync {
        task_id: "sync-terminal".to_owned(),
    });
    assert_eq!(result, Ok(BacktestsWritePortResult::SyncCancelled(false)));
}

/// Parity: go:452dea11:internal/assistant/assembly/application_adapter_test.go:226
/// `TestApplicationAdapterProvidesScreenCatalogAndCancelResult` (cancel half).
/// Go's `CancelBacktestResult` reports `false` when there is no backtest
/// service and when the service has no matching run, and only reports `true`
/// after it actually cancels a non-terminal run. The Rust owner is
/// `ProductionBacktestPort::cancel_backtest` through the backtests write port.
#[test]
fn production_backtest_cancel_reports_false_without_a_cancellable_run() {
    let (port, _directory) = production_port();
    let missed = port
        .mutate(&BacktestsWriteInput::Cancel {
            run_id: "missing".to_owned(),
        })
        .expect("missing run cancel");
    assert_eq!(
        missed,
        BacktestsWritePortResult::Data(json!({"id": "missing", "cancelled": false}))
    );

    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-terminal".to_owned(),
                status: "completed".to_owned(),
                request_json: r#"{"symbol":"US.AAPL"}"#.to_owned(),
                result_json: r#"{"pnl":1.0}"#.to_owned(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:01:00Z".to_owned(),
            },
            "2026-08-29T00:01:00Z",
        )
        .expect("persist terminal run");
    let terminal = port
        .mutate(&BacktestsWriteInput::Cancel {
            run_id: "run-terminal".to_owned(),
        })
        .expect("terminal run cancel");
    assert_eq!(
        terminal,
        BacktestsWritePortResult::Data(json!({"id": "run-terminal", "cancelled": false}))
    );
    let stored = port
        .store
        .get_run("run-terminal")
        .expect("load terminal run")
        .expect("run exists");
    assert_eq!(stored.status, "completed");

    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-queued".to_owned(),
                status: "queued".to_owned(),
                request_json: r#"{"symbol":"US.AAPL"}"#.to_owned(),
                result_json: String::new(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:00:00Z".to_owned(),
            },
            "2026-08-29T00:00:00Z",
        )
        .expect("persist queued run");
    let queued = port
        .mutate(&BacktestsWriteInput::Cancel {
            run_id: "run-queued".to_owned(),
        })
        .expect("queued run cancel");
    assert_eq!(
        queued,
        BacktestsWritePortResult::Data(json!({"id": "run-queued", "cancelled": true}))
    );
    let stored = port
        .store
        .get_run("run-queued")
        .expect("load cancelled run")
        .expect("run exists");
    assert_eq!(stored.status, "cancelled");
}

#[test]
fn production_sync_restart_recovery_marks_orphaned_task_failed() {
    let (port, _directory) = production_port();
    port.sync_tasks
        .create(StoredBacktestSyncTask {
            task_id: "sync-orphaned".to_owned(),
            status: "running".to_owned(),
            symbol: "HK.00700".to_owned(),
            market_data_provider: "futu".to_owned(),
            total_intervals: 1,
            completed_intervals: 0,
            total_batches: 0,
            completed_batches: 0,
            current_interval: "1m".to_owned(),
            retries: 1,
            error: None,
            started_at: "2026-08-29T00:00:00Z".to_owned(),
            updated_at: "2026-08-29T00:01:00Z".to_owned(),
            revision: 0,
        })
        .expect("persist orphaned task");
    port.recover_orphaned_sync_tasks()
        .expect("recover orphaned task");
    let task = port
        .sync_tasks
        .get("sync-orphaned")
        .expect("load recovered task")
        .expect("task exists");
    assert_eq!(task.status, "failed");
    assert_eq!(
        task.error.as_deref(),
        Some("sync task interrupted by process restart")
    );
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/servercoretest/backtest_runs_test.go:206 TestBacktestListReturnsLightweightRunsAndResultReturnsDetail
fn production_backtest_read_routes_project_store_state() {
    let (port, _directory) = production_port();
    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-production".to_owned(),
                status: "completed".to_owned(),
                request_json: r#"{"symbol":"US.AAPL","period":"1d"}"#.to_owned(),
                result_json: r#"{"pnl":12.5,"marketDataProvider":"yfinance"}"#.to_owned(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:01:00Z".to_owned(),
            },
            "2026-08-29T00:01:00Z",
        )
        .expect("persist run");

    let listed = port.list().expect("list runs");
    assert_eq!(listed["runs"][0]["id"], "run-production");
    assert_eq!(listed["runs"][0]["marketDataProvider"], "yfinance");
    // The list projection stays lightweight: the persisted result payload is
    // only reachable through the detail/result reads.
    assert!(listed["runs"][0].get("result").is_none());
    assert!(listed["runs"][0].get("results").is_none());

    let status = port
        .status("run-production")
        .expect("status")
        .expect("run exists");
    assert_eq!(status["status"], "completed");

    let result = port
        .result("run-production")
        .expect("result")
        .expect("run exists");
    assert_eq!(result["result"]["pnl"], 12.5);

    let deleted = port
        .mutate(&BacktestsWriteInput::Delete {
            run_id: "run-production".to_owned(),
        })
        .expect("delete run");
    assert_eq!(
        deleted,
        BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted)
    );
    assert!(port.status("run-production").expect("status").is_none());
}

#[test]
fn production_backtest_read_rejects_corrupted_request_json() {
    let (port, _directory) = production_port();
    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-corrupt".to_owned(),
                status: "queued".to_owned(),
                request_json: "{not-json".to_owned(),
                result_json: String::new(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:00:00Z".to_owned(),
            },
            "2026-08-29T00:00:00Z",
        )
        .expect("persist corrupt fixture");
    let error = port.list().expect_err("corrupt request must fail closed");
    assert!(error.to_string().contains("invalid JSON"));
}

#[test]
fn production_backtest_start_without_worker_fails_before_persisting_run() {
    let (port, _directory) = production_port();
    let result = port.mutate(&BacktestsWriteInput::Start {
        payload: json!({
            "definitionId": "fixture-definition",
            "strategyScript": "strategy('fixture')",
            "symbol": "US.AAPL",
            "interval": "1d",
            "startTime": "2025-06-23T13:00:00Z",
            "endTime": "2025-06-23T13:01:00Z"
        }),
    });
    assert!(matches!(
        result,
        Err(BacktestsWritePortError::Unavailable(message))
            if message.contains("worker runtime")
    ));
    assert_eq!(port.store.run_count().expect("run count"), 0);
}

#[tokio::test]
// Parity: go:452dea11:internal/api/backtest/routes_test.go:85 TestStartRoutePreservesQueuedResponseShape
async fn production_backtest_start_executes_fixture_and_persists_terminal_result() {
    let (mut port, _directory) = production_port();
    port._market_data_store
        .insert_candles(
            "yfinance",
            "US.AAPL",
            "1m",
            "forward",
            "regular",
            &[StoredBacktestCandle {
                start_time: 1_750_683_600_000,
                end_time: 1_750_683_659_999,
                open: "100".to_owned(),
                high: "101".to_owned(),
                low: "99".to_owned(),
                close: "100".to_owned(),
                volume: "10".to_owned(),
            }],
        )
        .expect("seed candles");
    port.execution = Some(std::sync::Arc::new(FixtureExecution));
    let response = port
        .mutate(&BacktestsWriteInput::Start {
            payload: json!({
                "definitionId": "fixture-definition",
                "strategyScript": "strategy('fixture')",
                "symbol": "US.AAPL",
                "interval": "1m",
                "startTime": "2025-06-23T13:00:00Z",
                "endTime": "2025-06-23T13:01:00Z",
                "rehabType": "forward"
            }),
        })
        .expect("start fixture backtest");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected start response");
    };
    let run_id = data["id"].as_str().expect("run id").to_owned();
    for _ in 0..50 {
        if let Some(run) = port.store.get_run(&run_id).expect("load run")
            && run.status == "completed"
        {
            let result: Value = serde_json::from_str(&run.result_json).expect("result json");
            assert_eq!(result["bars"], 1);
            assert_eq!(result["marketDataProvider"], "yfinance");
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("fixture backtest did not complete");
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/servercore/server_warmup_test.go:20 TestBacktestRouteUsesDerivedStrategyWarmup
async fn production_backtest_start_uses_derived_strategy_warmup_for_definition_route() {
    let (mut port, _directory) = production_port();
    let execution = std::sync::Arc::new(RecordingExecution::default());
    port.execution = Some(execution.clone());

    let script = r#"//@version=6
strategy("Auto Warmup Route", overlay=true)
slow = ta.sma(close, 20)
strategy.entry("Long", strategy.long, qty=1)
"#;
    port.strategy_definitions
        .save_definition(
            jftrade_store_sqlite::StoredStrategyDefinition {
                id: "dsl-auto-warmup-route".to_owned(),
                name: "Auto Warmup Route".to_owned(),
                version: "0.1.0".to_owned(),
                description: "definition-backed warmup route".to_owned(),
                runtime: "pine-pinets".to_owned(),
                source_format: "pine-v6".to_owned(),
                symbol: "US.AAPL".to_owned(),
                interval: "1m".to_owned(),
                script: script.to_owned(),
                visual_model_json: "{}".to_owned(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:00:00Z".to_owned(),
                deleted_at: None,
            },
            "2026-08-29T00:00:00Z",
        )
        .expect("save definition");

    let base_start = 1_780_272_000_000_i64;
    let candles = (0..25)
        .map(|index| {
            let start_time = base_start + index * 60_000;
            StoredBacktestCandle {
                start_time,
                end_time: start_time + 59_999,
                open: "100".to_owned(),
                high: "101".to_owned(),
                low: "99".to_owned(),
                close: "100".to_owned(),
                volume: "1000".to_owned(),
            }
        })
        .collect::<Vec<_>>();
    port._market_data_store
        .insert_candles("yfinance", "US.AAPL", "1m", "forward", "regular", &candles)
        .expect("seed route candles");

    let response = port
        .mutate(&BacktestsWriteInput::Start {
            payload: json!({
                "definitionId": "dsl-auto-warmup-route",
                "symbol": "US.AAPL",
                "interval": "1m",
                "startTime": "2026-06-01T00:20:00Z",
                "endTime": "2026-06-01T00:24:59Z",
                "initialBalance": 10_000,
                "rehabType": "forward",
            }),
        })
        .expect("start definition-backed backtest");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected start response");
    };
    let run_id = data["id"].as_str().expect("run id").to_owned();

    for _ in 0..100 {
        if let Some(run) = port.store.get_run(&run_id).expect("load run")
            && run.status == "completed"
        {
            let request = execution
                .request
                .lock()
                .expect("record execution request")
                .clone()
                .expect("worker request");
            assert_eq!(request.payload["warmupBars"], json!(20));
            assert_eq!(request.candles.len(), 25);
            assert_eq!(request.candles[0].start_time, base_start);
            assert_eq!(request.candles[19].start_time, base_start + 19 * 60_000);
            assert_eq!(request.candles[20].start_time, base_start + 20 * 60_000);
            assert_eq!(request.candles[24].start_time, base_start + 24 * 60_000);
            assert_eq!(request.payload["strategyScript"], json!(script));
            let result: Value = serde_json::from_str(&run.result_json).expect("result json");
            assert_eq!(result["bars"], json!(25));
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("definition-backed warmup backtest did not complete");
}

#[tokio::test]
async fn production_backtest_start_rejects_missing_history_without_queuing() {
    let (mut port, _directory) = production_port();
    port.execution = Some(std::sync::Arc::new(FixtureExecution));
    let result = port.mutate(&BacktestsWriteInput::Start {
        payload: json!({
            "definitionId": "fixture-definition",
            "strategyScript": "strategy('fixture')",
            "symbol": "US.MISSING",
            "interval": "1m",
            "startTime": "2025-06-23T13:00:00Z",
            "endTime": "2025-06-23T13:01:00Z",
            "rehabType": "forward"
        }),
    });
    assert!(matches!(
        result,
        Err(BacktestsWritePortError::Unavailable(message))
            if message.contains("K-line data")
    ));
    assert_eq!(port.store.run_count().expect("run count"), 0);
}
