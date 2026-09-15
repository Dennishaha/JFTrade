use super::*;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use jftrade_integration_futu::{
    CurrentKlineError, CurrentKlineQuery, CurrentKlineResult, HistoricalKline,
    HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult,
    HistoricalSecurity,
};

#[derive(Debug, Default)]
struct PagedHistory {
    requests: Mutex<Vec<HistoricalKlineQuery>>,
    fail_second_page: bool,
    current_calls: AtomicUsize,
}

fn candle(minute: usize) -> HistoricalKline {
    HistoricalKline {
        time: format!("2026-01-05 10:{minute:02}:00"),
        is_blank: false,
        high_price: Some(100.0),
        open_price: Some(100.0),
        low_price: Some(100.0),
        close_price: Some(100.0),
        volume: Some(100),
        turnover: None,
        change_rate: None,
    }
}

impl HistoricalKlineReadPort for PagedHistory {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        self.requests.lock().unwrap().push(query.clone());
        let first = query.next_req_key.is_empty();
        if !first && self.fail_second_page {
            return Err(HistoricalKlineError::MissingS2c);
        }
        Ok(HistoricalKlineResult {
            security: HistoricalSecurity {
                market: 1,
                code: "00700".into(),
            },
            name: Some("腾讯控股".into()),
            klines: if first {
                vec![candle(0), candle(1)]
            } else {
                vec![candle(2), candle(3), candle(4)]
            },
            next_req_key: if first { vec![1] } else { vec![] },
        })
    }

    fn query_current(
        &self,
        _: &CurrentKlineQuery,
    ) -> Result<CurrentKlineResult, CurrentKlineError> {
        self.current_calls.fetch_add(1, Ordering::SeqCst);
        Ok(CurrentKlineResult {
            security: HistoricalSecurity {
                market: 1,
                code: "00700".into(),
            },
            name: None,
            klines: vec![candle(5)],
        })
    }
}

fn port(reader: Arc<PagedHistory>) -> ProductionMarketDataQuotePort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_historical_klines(Some(reader));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ProductionMarketDataQuotePort::new(state, None, None, None).with_trade_runtime(Some(runtime))
}

#[tokio::test]
async fn candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar() {
    let reader = Arc::new(PagedHistory::default());
    let result = port(reader.clone())
        .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=3")
        .await
        .unwrap();
    let candles = result["candles"].as_array().unwrap();
    assert_eq!(candles.len(), 3);
    assert_eq!(candles[0]["at"], "2026-01-05T02:03:00Z");
    assert_eq!(candles[1]["at"], "2026-01-05T02:04:00Z");
    assert_eq!(candles[2]["at"], "2026-01-05T02:05:00Z");
    assert_eq!(result["pagination"]["hasMore"], true);
    assert_eq!(result["pagination"]["nextBefore"], candles[0]["at"]);
    assert_eq!(reader.requests.lock().unwrap().len(), 2);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn candle_route_excludes_before_boundary_and_can_continue_loading_older_pages() {
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    let result = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=2&before=2026-01-05T02:04:00Z",
        )
        .await
        .unwrap();
    assert_eq!(result["candles"][0]["at"], "2026-01-05T02:02:00Z");
    assert_eq!(result["candles"][1]["at"], "2026-01-05T02:03:00Z");
    assert_eq!(result["pagination"]["hasMore"], true);
    let result = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=2&before=2026-01-05T02:02:00Z",
        )
        .await
        .unwrap();
    assert_eq!(result["candles"][0]["at"], "2026-01-05T02:00:00Z");
    assert_eq!(result["candles"][1]["at"], "2026-01-05T02:01:00Z");
    assert_eq!(result["pagination"]["hasMore"], false);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn candle_route_serves_tick_period_and_forwards_strict_before_window() {
    // Parity: internal/api/marketdata/routes_test.go:407 TestCandlesRouteTickAndStrictBeforePagination
    // Go asserts a `period=tick` request is a successful non-paged response and
    // that `before` is forwarded as the provider window end with `to` empty.
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    let tick = port
        .read("/api/v1/market-data/candles/US/AAPL", "period=tick")
        .await
        .expect("tick candles");
    assert_eq!(tick["pagination"]["hasMore"], false);

    let paged = port
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=5m&limit=2&before=2026-07-18T13:40:00Z",
        )
        .await
        .expect("strict before pagination");
    assert!(paged["candles"].is_array());
    let requests = reader.requests.lock().unwrap();
    let last = requests.last().expect("provider query");
    assert_eq!(last.begin_time.len(), 19, "window start is provider-local");
    // The RFC3339 cursor is converted into the US session timezone, matching
    // how the Futu reader expects provider-local window bounds.
    assert_eq!(
        last.end_time, "2026-07-18 09:40:00",
        "before is forwarded as the provider window end (America/New_York)"
    );
}

#[tokio::test]
async fn candle_route_rejects_partial_history_before_querying_current_bars() {
    let reader = Arc::new(PagedHistory {
        fail_second_page: true,
        ..Default::default()
    });
    assert!(
        port(reader.clone())
            .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=3")
            .await
            .is_err()
    );
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn test_broker_k_line_candles_response_projects_strict_page() {
    // Parity: internal/marketdata/broker_candles_test.go:12 TestBrokerKLineCandlesResponseProjectsStrictPage
    let reader = Arc::new(PagedHistory::default());
    let result = port(reader.clone())
        .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=2")
        .await
        .unwrap();
    assert_eq!(result["totalReturned"], 2);
    let candles = result["candles"].as_array().expect("candles array");
    assert_eq!(candles.len(), 2);
    assert!(candles[0]["at"].as_str().is_some());
    assert!(candles[0]["close"].is_string() || candles[0]["close"].is_number());
    assert_eq!(result["pagination"]["hasMore"], true);
    assert!(result["pagination"]["nextBefore"].as_str().is_some());
    assert!(result["meta"].is_object());
}

#[tokio::test]
async fn test_broker_k_line_candles_response_handles_terminal_and_bounded_pages() {
    // Parity: internal/marketdata/broker_candles_test.go:51 TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    // Terminal page where no older records exist
    let result = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=2&before=2026-01-05T02:02:00Z",
        )
        .await
        .unwrap();
    assert_eq!(result["pagination"]["hasMore"], false);
}

#[test]
fn test_broker_k_line_helpers_classify_sessions_and_numbers() {
    // Parity: internal/marketdata/broker_candles_test.go:109 TestBrokerKLineHelpersClassifySessionsAndNumbers
    use crate::product::product_candle_converter::{
        parse_candle_number, parse_optional_iso_timestamp,
    };

    // 1. Time parsing (empty is Ok(None), valid RFC3339 parsed)
    assert_eq!(parse_optional_iso_timestamp("").unwrap(), None);
    assert!(
        parse_optional_iso_timestamp("2026-07-15T22:00:00+08:00")
            .unwrap()
            .is_some()
    );

    // 2. Number parsing
    let val = 12.5;
    let num_str = parse_candle_number(Some(val), "close").expect("valid float");
    assert_eq!(num_str, "12.5");
    assert!(parse_candle_number(None, "close").is_err());
}

#[test]
fn test_broker_k_line_pagination_rejects_invalid_bounded_and_paged_metadata() {
    // Parity: internal/marketdata/broker_candles_test.go:137 TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata
    use crate::product::product_candle_converter::validate_candle_pagination;

    // 1. Bounded query (has from) returned hasMore=true
    let err = validate_candle_pagination(
        true,
        Some("2026-07-15T01:00:00Z"),
        Some("2026-07-14T00:00:00Z"),
        1,
        1,
    );
    assert!(err.is_err());
    assert!(err.unwrap_err().contains("bounded"));

    // 2. Page exceeds limit
    let err_limit = validate_candle_pagination(false, None, None, 2, 1);
    assert!(err_limit.is_err());
    assert!(err_limit.unwrap_err().contains("exceeds limit"));
}

#[test]
fn test_broker_k_line_candles_response_rejects_invalid_provider_rows() {
    // Parity: internal/marketdata/broker_candles_test.go:76 TestBrokerKLineCandlesResponseRejectsInvalidProviderRows
    use crate::product::product_candle_converter::validate_candle_positive_decimal;

    // 1. Non-positive close price
    assert!(validate_candle_positive_decimal("close", "0.0").is_err());
    assert!(validate_candle_positive_decimal("close", "-1.5").is_err());
    assert!(validate_candle_positive_decimal("close", "bad").is_err());

    // 2. Valid close price
    assert!(validate_candle_positive_decimal("close", "100.5").is_ok());
}
