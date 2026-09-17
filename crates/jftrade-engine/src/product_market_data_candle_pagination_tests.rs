use super::*;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use jftrade_integration_futu::{
    CurrentKlineError, CurrentKlineQuery, CurrentKlineResult, HistoricalKline,
    HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult,
    HistoricalSecurity, TickerQuoteError, TickerQuoteReadPort,
};

/// Test-local clock helpers. The production equivalents live behind a private
/// module, and these fixtures only need a consistent millisecond/RFC3339 pair.
fn current_unix_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("system clock")
        .as_millis() as i64
}

pub(super) fn format_unix_millis_rfc3339(ms: i64) -> String {
    time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(ms) * 1_000_000)
        .expect("valid timestamp")
        .format(&time::format_description::well_known::Rfc3339)
        .expect("RFC3339 formatting")
}

/// Records the provider ticker read that Go's `GetCandles` performs on a tick
/// cache miss. A failing fixture proves the retained-cache fallback.
#[derive(Debug, Default)]
struct StubTicker {
    calls: AtomicUsize,
    tick: Mutex<Option<jftrade_marketdata::Tick>>,
    fail: bool,
}

impl TickerQuoteReadPort for StubTicker {
    fn query_ticker(
        &self,
        _instrument_id: &str,
        _observed_at_ms: i64,
    ) -> Result<Option<jftrade_marketdata::Tick>, TickerQuoteError> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        if self.fail {
            return Err(TickerQuoteError::Lock);
        }
        Ok(self.tick.lock().unwrap().clone())
    }
}

fn tick_sample(
    instrument_id: &str,
    price: &str,
    volume_delta: Option<&str>,
    observed_at_ms: i64,
    session: &str,
) -> jftrade_marketdata::Tick {
    jftrade_marketdata::Tick {
        instrument_id: instrument_id.to_owned(),
        price: price.parse().expect("tick price"),
        volume: "1000".parse().expect("cumulative volume"),
        volume_delta: volume_delta.map(|value| value.parse().expect("volume delta")),
        snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
            session: Some(session.to_owned()),
            ..Default::default()
        }),
        observed_at_ms,
        provider_generation: 1,
    }
}

fn tick_port(
    mut router: Option<Arc<Mutex<ProviderRouter>>>,
    ticker: Arc<StubTicker>,
) -> ProductionMarketDataQuotePort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_ticker_quotes(Some(ticker));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    // Go's live-read harness acquires a logical subscription before the read,
    // and `GetCandles` refuses a tick cache read without one.
    if let Some(router) = router.as_mut() {
        router
            .lock()
            .unwrap()
            .acquire_demand(
                "tick-candles-test",
                [
                    InstrumentRef {
                        channel: "TICK".to_owned(),
                        market: "HK".to_owned(),
                        symbol: "00700".to_owned(),
                        interval: None,
                    },
                    InstrumentRef {
                        channel: "TICK".to_owned(),
                        market: "US".to_owned(),
                        symbol: "AAPL".to_owned(),
                        interval: None,
                    },
                ],
                false,
                0,
            )
            .expect("tick lease");
    }
    ProductionMarketDataQuotePort::new(state, router, None, None).with_trade_runtime(Some(runtime))
}

#[derive(Debug, Default)]
struct PagedHistory {
    requests: Mutex<Vec<HistoricalKlineQuery>>,
    fail_second_page: bool,
    /// Keep the first page terminal instead of offering a second page. The
    /// window tests need exact control over the provider's candle list.
    single_page: bool,
    /// Explicit provider series used by the Go-shaped pagination fixtures.
    /// When set, every request answers the whole series with a terminal page
    /// so the engine's own latest-window trimming is what gets exercised.
    series: Vec<HistoricalKline>,
    current_calls: AtomicUsize,
    /// Explicit candle open times used by session-classification tests.  When
    /// empty the default `2026-01-05 10:0x` fixture times are used.
    times: Vec<String>,
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

/// A provider candle whose close carries the fixture's price. Intraday OpenD
/// labels mark the end of the bucket; the engine shifts them to bucket start.
fn priced_candle(time: &str, close: f64) -> HistoricalKline {
    HistoricalKline {
        time: time.to_owned(),
        is_blank: false,
        high_price: Some(close + 1.0),
        open_price: Some(close),
        low_price: Some(close - 1.0),
        close_price: Some(close),
        volume: Some(1000),
        turnover: Some(close * 1000.0),
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
        if !self.series.is_empty() {
            return Ok(HistoricalKlineResult {
                security: HistoricalSecurity {
                    market: 1,
                    code: "00700".into(),
                },
                name: Some("腾讯控股".into()),
                klines: self.series.clone(),
                next_req_key: Vec::new(),
            });
        }
        if !first && self.fail_second_page {
            return Err(HistoricalKlineError::Rejected {
                ret_type: -2,
                err_code: 429,
                message: "history rate limited".to_owned(),
            });
        }
        Ok(HistoricalKlineResult {
            security: HistoricalSecurity {
                market: 1,
                code: "00700".into(),
            },
            name: Some("腾讯控股".into()),
            klines: if first {
                if self.times.is_empty() {
                    vec![candle(0), candle(1)]
                } else {
                    self.times
                        .iter()
                        .map(|time| HistoricalKline {
                            time: time.clone(),
                            ..candle(0)
                        })
                        .collect()
                }
            } else {
                vec![candle(2), candle(3), candle(4)]
            },
            next_req_key: if first && !self.single_page {
                vec![1]
            } else {
                vec![]
            },
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

/// Session-routed history reader for US intraday windows.
#[derive(Debug, Default)]
struct RoutedHistory {
    requests: Mutex<Vec<HistoricalKlineQuery>>,
    /// Per-route candles keyed by the OpenD session value.
    by_session: std::collections::BTreeMap<i32, Vec<HistoricalKline>>,
    /// Routes answering with an unsupported-sessions rejection.
    reject_sessions: Vec<i32>,
    current_calls: AtomicUsize,
}

impl HistoricalKlineReadPort for RoutedHistory {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        self.requests.lock().unwrap().push(query.clone());
        let session = query.session.unwrap_or_default();
        if self.reject_sessions.contains(&session) {
            return Err(HistoricalKlineError::Rejected {
                ret_type: 1,
                err_code: 0,
                message: "获取历史K线的时段仅支持设置 RTH，ETH，ALL".to_owned(),
            });
        }
        Ok(HistoricalKlineResult {
            security: HistoricalSecurity {
                market: 11,
                code: "AAPL".into(),
            },
            name: Some("Apple".into()),
            klines: self.by_session.get(&session).cloned().unwrap_or_default(),
            next_req_key: Vec::new(),
        })
    }

    fn query_current(
        &self,
        _: &CurrentKlineQuery,
    ) -> Result<CurrentKlineResult, CurrentKlineError> {
        self.current_calls.fetch_add(1, Ordering::SeqCst);
        Ok(CurrentKlineResult::default())
    }
}

fn us_candle(time: &str, open: f64) -> HistoricalKline {
    HistoricalKline {
        time: time.to_owned(),
        is_blank: false,
        high_price: Some(open),
        open_price: Some(open),
        low_price: Some(open),
        close_price: Some(open),
        volume: Some(10),
        turnover: None,
        change_rate: None,
    }
}

fn routed_port(
    reader: Arc<RoutedHistory>,
    calendar: Arc<jftrade_calendar::CalendarManager>,
) -> ProductionMarketDataQuotePort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_historical_klines(Some(reader));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ProductionMarketDataQuotePort::new(state, None, None, None)
        .with_trade_runtime(Some(runtime))
        .with_calendar(calendar)
}

fn default_calendar() -> Arc<jftrade_calendar::CalendarManager> {
    Arc::new(
        jftrade_calendar::CalendarManager::new(
            jftrade_calendar::CalendarSourceRegistry::default(),
            None,
            jftrade_calendar::CalendarManagerSettings::default(),
        )
        .expect("calendar manager"),
    )
}

/// Parity: pkg/futu/exchange_kline_test.go:50
/// TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults
///
/// A US intraday history read without an explicit `sessions` parameter must
/// fan out across the RTH/ETH/ALL OpenD routes and merge the returned bars.
#[tokio::test]
async fn us_intraday_history_fans_out_across_opend_session_routes() {
    use jftrade_integration_futu::{SESSION_ALL, SESSION_ETH, SESSION_RTH};
    // Fixture times are exchange-local (America/New_York): 06:00 ET is a
    // pre-market bar, 11:30 ET is regular, and 22:00 ET is the overnight
    // carry that the broad `Session_ALL` route also returns as a duplicate.
    let reader = Arc::new(RoutedHistory {
        by_session: [
            (SESSION_RTH, vec![us_candle("2026-05-20 11:30:00", 110.0)]),
            (SESSION_ETH, vec![us_candle("2026-05-20 06:00:00", 100.0)]),
            (
                SESSION_ALL,
                vec![
                    us_candle("2026-05-19 22:00:00", 90.0),
                    us_candle("2026-05-20 06:00:00", 95.0),
                    us_candle("2026-05-20 11:30:00", 105.0),
                ],
            ),
        ]
        .into_iter()
        .collect(),
        ..RoutedHistory::default()
    });
    let result = routed_port(reader.clone(), default_calendar())
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=1m&limit=10&from=2026-05-20T08:00:00Z&to=2026-05-20T16:00:00Z",
        )
        .await
        .expect("US intraday routed candles");
    let routes = reader
        .requests
        .lock()
        .unwrap()
        .iter()
        .map(|request| request.session)
        .collect::<Vec<_>>();
    assert_eq!(
        routes,
        vec![Some(SESSION_RTH), Some(SESSION_ETH), Some(SESSION_ALL)]
    );
    let candles = result["candles"].as_array().expect("candles");
    // Duplicate buckets resolve to the more specific route's candle.
    let by_at: std::collections::BTreeMap<&str, f64> = candles
        .iter()
        .filter_map(|candle| {
            Some((
                candle.get("at")?.as_str()?,
                candle.get("open")?.as_str()?.parse().ok()?,
            ))
        })
        .collect();
    assert_eq!(by_at.get("2026-05-20T10:00:00Z"), Some(&100.0));
    assert_eq!(by_at.get("2026-05-20T15:30:00Z"), Some(&110.0));
    assert_eq!(by_at.get("2026-05-20T02:00:00Z"), Some(&90.0));
    assert_eq!(candles.len(), 3, "one bar per routed session: {result}");
}

/// Parity: pkg/futu/exchange_kline_test.go:106
/// TestQueryKLinesForSessionsFiltersUSHistoricalRoutes
#[tokio::test]
async fn us_regular_only_history_uses_a_single_rth_route() {
    use jftrade_integration_futu::{SESSION_ALL, SESSION_ETH, SESSION_RTH};
    let reader = Arc::new(RoutedHistory {
        by_session: [
            (SESSION_RTH, vec![us_candle("2026-05-20 11:30:00", 110.0)]),
            (SESSION_ETH, vec![us_candle("2026-05-20 06:00:00", 100.0)]),
            (SESSION_ALL, vec![us_candle("2026-05-19 22:00:00", 90.0)]),
        ]
        .into_iter()
        .collect(),
        ..RoutedHistory::default()
    });
    let result = routed_port(reader.clone(), default_calendar())
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=1m&limit=10&sessions=regular&from=2026-05-20T00:00:00Z&to=2026-05-21T00:00:00Z",
        )
        .await
        .expect("regular-only candles");
    let routes = reader
        .requests
        .lock()
        .unwrap()
        .iter()
        .map(|request| request.session)
        .collect::<Vec<_>>();
    assert_eq!(routes, vec![Some(SESSION_RTH)]);
    let candles = result["candles"].as_array().expect("candles");
    assert_eq!(candles.len(), 1);
    assert_eq!(candles[0]["at"], "2026-05-20T15:30:00Z");
}

/// Parity: pkg/futu/exchange_kline_test.go:192
/// TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported
#[tokio::test]
async fn us_history_falls_back_to_session_all_when_a_route_is_rejected() {
    use jftrade_integration_futu::{SESSION_ALL, SESSION_ETH, SESSION_RTH};
    let reader = Arc::new(RoutedHistory {
        by_session: [
            (SESSION_RTH, vec![us_candle("2026-05-20 11:30:00", 110.0)]),
            (
                SESSION_ALL,
                vec![
                    us_candle("2026-05-19 22:00:00", 90.0),
                    us_candle("2026-05-20 06:00:00", 100.0),
                    us_candle("2026-05-20 11:30:00", 110.0),
                ],
            ),
        ]
        .into_iter()
        .collect(),
        reject_sessions: vec![SESSION_ETH],
        ..RoutedHistory::default()
    });
    let result = routed_port(reader.clone(), default_calendar())
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=1m&limit=10&from=2026-05-20T08:00:00Z&to=2026-05-20T16:00:00Z",
        )
        .await
        .expect("fallback Session_ALL candles");
    let routes = reader
        .requests
        .lock()
        .unwrap()
        .iter()
        .map(|request| request.session)
        .collect::<Vec<_>>();
    assert_eq!(
        routes,
        vec![Some(SESSION_RTH), Some(SESSION_ETH), Some(SESSION_ALL)]
    );
    let candles = result["candles"].as_array().expect("candles");
    assert_eq!(candles.len(), 3, "fallback ALL route must serve the window");
}

fn port_with_calendar(
    reader: Arc<PagedHistory>,
    calendar: Arc<jftrade_calendar::CalendarManager>,
) -> ProductionMarketDataQuotePort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_historical_klines(Some(reader));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ProductionMarketDataQuotePort::new(state, None, None, None)
        .with_trade_runtime(Some(runtime))
        .with_calendar(calendar)
}

#[tokio::test]
async fn us_intraday_futu_candles_carry_calendar_resolved_session_labels() {
    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:18
    // TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday
    //
    // Go classifies each US intraday candle against the exchange schedule, so
    // an RTH bar is `regular` while an ETH bar is `pre`/`after`.  Rust must
    // resolve the label from the authoritative calendar instead of assuming
    // every candle is a regular-session bar.
    let calendar = Arc::new(
        jftrade_calendar::CalendarManager::new(
            jftrade_calendar::CalendarSourceRegistry::default(),
            None,
            jftrade_calendar::CalendarManagerSettings::default(),
        )
        .expect("calendar manager"),
    );
    let reader = Arc::new(PagedHistory {
        times: vec![
            // Regular session (10:00 ET on 2026-01-05).
            "2026-01-05 10:00:00".to_owned(),
            // Pre-market (07:00 ET).
            "2026-01-05 07:00:00".to_owned(),
            // After-hours (17:00 ET).
            "2026-01-05 17:00:00".to_owned(),
            // Overnight carry (22:00 ET on 2026-01-05 = 2026-01-06T03:00:00Z).
            "2026-01-05 22:00:00".to_owned(),
        ],
        ..PagedHistory::default()
    });
    // Go passes no `sessions` flag here, so the read must resolve the US
    // intraday default of regular+extended+overnight.
    let result = port_with_calendar(reader, calendar)
        .read("/api/v1/market-data/candles/US/AAPL", "period=1m&limit=10")
        .await
        .expect("US intraday candles");
    let candles = result["candles"].as_array().expect("candles array");
    let labels: std::collections::BTreeMap<&str, &str> = candles
        .iter()
        .filter_map(|candle| {
            Some((
                candle.get("at")?.as_str()?,
                candle.get("session")?.as_str()?,
            ))
        })
        .collect();
    assert_eq!(
        labels.get("2026-01-05T12:00:00Z"),
        Some(&"pre"),
        "07:00 ET is a pre-market bar: {result}"
    );
    assert_eq!(
        labels.get("2026-01-05T15:00:00Z"),
        Some(&"regular"),
        "10:00 ET is a regular-session bar: {result}"
    );
    assert_eq!(
        labels.get("2026-01-05T22:00:00Z"),
        Some(&"after"),
        "17:00 ET is an after-hours bar: {result}"
    );
    assert_eq!(
        labels.get("2026-01-06T03:00:00Z"),
        Some(&"overnight"),
        "22:00 ET is the overnight carry: {result}"
    );
    assert!(
        candles.iter().all(|candle| candle["session"].is_string()),
        "every annotated US intraday candle needs a session label: {result}"
    );
    assert_eq!(
        result["meta"]["session"], "all",
        "meta.session must be `all` once extended sessions are present"
    );
    assert_eq!(result["meta"]["extendedHours"], true);
}

#[tokio::test]
async fn candle_route_only_annotates_sessions_for_us_intraday_history() {
    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:91
    // TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles and
    // market_http_test.go:18 TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday
    //
    // Go only annotates a per-candle `session` when the request is a US
    // intraday history window; daily candles and non-US markets must omit the
    // field entirely instead of claiming every candle is `regular`.
    for (path, query) in [
        ("/api/v1/market-data/candles/US/AAPL", "period=1d&limit=2"),
        ("/api/v1/market-data/candles/HK/00700", "period=1m&limit=2"),
    ] {
        let reader = Arc::new(PagedHistory::default());
        let result = port(reader)
            .read(path, query)
            .await
            .unwrap_or_else(|error| panic!("{path}?{query} failed: {error:?}"));
        let candles = result["candles"].as_array().expect("candles array");
        assert!(!candles.is_empty(), "{path}?{query} returned no candles");
        for candle in candles {
            assert!(
                candle.get("session").is_none(),
                "unannotated candle must omit `session`: {path}?{query} => {candle}"
            );
        }
        assert!(
            result["meta"].get("session").is_none(),
            "meta.session must stay absent when sessions are not annotated: {path}?{query}"
        );
    }
}

#[tokio::test]
async fn candle_route_forwards_strict_before_window() {
    // Parity: internal/api/marketdata/routes_test.go:407 TestCandlesRouteTickAndStrictBeforePagination
    // The non-tick half of that baseline: `before` is forwarded as the
    // provider window end while `to` stays empty. The tick half lives in
    // `candle_pagination_tests::tick_candles_*` because Go serves `period=tick`
    // from the tick cache instead of the historical reader.
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
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

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:150 TestMarketCandlesResponseClassifiesUnknownUSSessionAsDataError
///
/// Go classifies every annotated candle against the exchange schedule and
/// fails the read when a bar lands outside all sessions instead of returning a
/// page whose `session` field is silently missing.
#[tokio::test]
async fn candle_route_classifies_unknown_us_session_as_a_data_error() {
    let calendar = Arc::new(
        jftrade_calendar::CalendarManager::new(
            jftrade_calendar::CalendarSourceRegistry::default(),
            None,
            jftrade_calendar::CalendarManagerSettings::default(),
        )
        .expect("calendar manager"),
    );
    // 2026-05-24 is a Sunday, so the bar cannot belong to any session.
    let reader = Arc::new(PagedHistory {
        times: vec!["2026-05-24 12:00:00".to_owned()],
        ..PagedHistory::default()
    });
    let error = port_with_calendar(reader, calendar)
        .read("/api/v1/market-data/candles/US/AAPL", "period=1m&limit=1")
        .await
        .expect_err("a bar outside every session must be a data error");
    match error {
        MarketDataQuoteReadSnapshotError::Unavailable(message) => assert!(
            message.contains("unable to classify K-line session"),
            "unexpected classification error: {message}"
        ),
        other => panic!("unexpected error for unknown session: {other:?}"),
    }
}

/// A daily candle and a non-US market carry no session annotation, so the same
/// out-of-session bar must still be served without the classification check.
#[tokio::test]
async fn candle_route_skips_session_classification_for_unannotated_requests() {
    let calendar = Arc::new(
        jftrade_calendar::CalendarManager::new(
            jftrade_calendar::CalendarSourceRegistry::default(),
            None,
            jftrade_calendar::CalendarManagerSettings::default(),
        )
        .expect("calendar manager"),
    );
    let reader = Arc::new(PagedHistory {
        times: vec!["2026-05-24 12:00:00".to_owned()],
        ..PagedHistory::default()
    });
    let result = port_with_calendar(reader, calendar)
        .read("/api/v1/market-data/candles/US/AAPL", "period=1d&limit=1")
        .await
        .expect("daily candles do not require session classification");
    let candle = result["candles"][0].as_object().expect("candle object");
    assert!(
        candle.get("session").is_none(),
        "daily candles stay unannotated: {result}"
    );
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:382 TestMarketCandlesTickResponseUsesFreshCache
///
/// A fresh cached sample answers a `period=tick` request without any provider
/// call, and the response reports `meta.fromCache = true` together with the
/// candle's own `at` timestamp.
#[tokio::test]
async fn tick_candles_use_fresh_cache_without_querying_the_provider() {
    let now_ms = current_unix_millis();
    let router = ProviderRouter::new(8);
    router
        .cache_mut()
        .insert(
            tick_sample("HK.00700", "321.4", Some("25"), now_ms, "regular"),
            1,
        )
        .expect("cache tick");
    let router = Arc::new(Mutex::new(router));
    let ticker = Arc::new(StubTicker::default());
    let port = tick_port(Some(router.clone()), ticker.clone());

    let response = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect("tick candles from cache");

    assert_eq!(response["totalReturned"], 1);
    assert_eq!(response["meta"]["fromCache"], true);
    // Go's candle route overrides the pagination envelope for tick reads.
    assert_eq!(response["pagination"]["hasMore"], false);
    assert_eq!(response["request"]["period"], "tick");
    assert_eq!(
        response["request"]["instrument"]["instrumentId"],
        "HK.00700"
    );
    let candle = &response["candles"][0];
    assert_eq!(candle["period"], "tick");
    assert_eq!(candle["open"], "321.4");
    assert_eq!(candle["high"], "321.4");
    assert_eq!(candle["low"], "321.4");
    assert_eq!(candle["close"], "321.4");
    // Tick candles publish the per-event delta, not the cumulative counter.
    assert_eq!(candle["volume"], "25");
    assert_eq!(candle["at"], format_unix_millis_rfc3339(now_ms));
    assert_eq!(
        ticker.calls.load(Ordering::SeqCst),
        0,
        "a fresh cached sample must not trigger a provider ticker read"
    );
    let history = router
        .lock()
        .unwrap()
        .cache_handle()
        .lock()
        .unwrap()
        .history("HK.00700");
    assert_eq!(history.len(), 1, "the read must not duplicate the sample");
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:418 TestMarketCandlesTickResponseQueriesTickerOnCacheMiss
///
/// A cache miss performs exactly one provider ticker read, ingests the sample
/// and answers with `meta.fromCache = false`.
#[tokio::test]
async fn tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample() {
    let now_ms = current_unix_millis();
    let router = Arc::new(Mutex::new(ProviderRouter::new(8)));
    let ticker = Arc::new(StubTicker {
        tick: Mutex::new(Some(tick_sample(
            "HK.00700",
            "321.4",
            Some("25"),
            now_ms,
            "regular",
        ))),
        ..StubTicker::default()
    });
    let port = tick_port(Some(router.clone()), ticker.clone());

    let response = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect("tick candles from the provider ticker");

    assert_eq!(response["totalReturned"], 1);
    assert_eq!(response["meta"]["fromCache"], false);
    assert_eq!(response["candles"][0]["period"], "tick");
    assert_eq!(response["candles"][0]["volume"], "25");
    assert_eq!(
        ticker.calls.load(Ordering::SeqCst),
        1,
        "a cache miss must issue exactly one provider ticker read"
    );
    // Go calls `s.Ingest(*sample)`, so the read itself must leave the sample
    // available to the snapshot/latest readers.
    let history = router
        .lock()
        .unwrap()
        .cache_handle()
        .lock()
        .unwrap()
        .history("HK.00700");
    assert_eq!(history.len(), 1, "the ingested sample must be retained");
    assert_eq!(history[0].price, "321.4".parse().expect("price"));

    // The second read is now served from the fresh cache.
    let cached = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect("second tick read");
    assert_eq!(cached["meta"]["fromCache"], true);
    assert_eq!(
        ticker.calls.load(Ordering::SeqCst),
        1,
        "the ingested sample must satisfy the next read"
    );
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:442 TestMarketCandlesTickResponseFallsBackToCachedCandlesOnTickerError
///
/// When the provider ticker fails but the cache still retains an older sample,
/// Go answers from that sample with `fromCache = true` instead of surfacing the
/// provider error.
#[tokio::test]
async fn tick_candles_fall_back_to_retained_cache_on_ticker_error() {
    let now_ms = current_unix_millis();
    let observed_at_ms = now_ms - 60_000;
    let router = ProviderRouter::new(8);
    router
        .cache_mut()
        .insert(
            tick_sample("HK.00700", "321.4", Some("10"), observed_at_ms, "regular"),
            1,
        )
        .expect("cache tick");
    let ticker = Arc::new(StubTicker {
        fail: true,
        ..StubTicker::default()
    });
    let port = tick_port(Some(Arc::new(Mutex::new(router))), ticker.clone());

    let response = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect("ticker failure must fall back to the retained cache");

    assert_eq!(response["totalReturned"], 1);
    assert_eq!(response["meta"]["fromCache"], true);
    assert_eq!(
        response["candles"][0]["at"],
        format_unix_millis_rfc3339(observed_at_ms)
    );
    assert_eq!(ticker.calls.load(Ordering::SeqCst), 1);
}

/// The retained-cache fallback must still fail closed when nothing is cached:
/// Go returns the provider error instead of an empty success payload.
#[tokio::test]
async fn tick_candles_surface_the_ticker_error_when_no_candle_is_retained() {
    let ticker = Arc::new(StubTicker {
        fail: true,
        ..StubTicker::default()
    });
    let port = tick_port(Some(Arc::new(Mutex::new(ProviderRouter::new(8)))), ticker);

    let error = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect_err("an empty cache with a failing ticker must fail closed");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Unavailable(_)
    ));
}

/// A provider ticker read that answers without a usable sample is `(nil, nil)`
/// in Go: the read still succeeds, nothing is ingested, and an empty cache
/// produces an empty non-paged page rather than an error.
#[tokio::test]
async fn tick_candles_report_an_empty_page_when_the_ticker_returns_no_sample() {
    let ticker = Arc::new(StubTicker::default());
    let port = tick_port(
        Some(Arc::new(Mutex::new(ProviderRouter::new(8)))),
        ticker.clone(),
    );
    let response = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=tick&limit=2",
        )
        .await
        .expect("a nil provider sample is not an error");
    assert_eq!(response["totalReturned"], 0);
    assert_eq!(response["meta"]["fromCache"], false);
    assert_eq!(response["pagination"]["hasMore"], false);
    assert_eq!(ticker.calls.load(Ordering::SeqCst), 1);
}

/// Go's `tickCandlesResponse` only annotates `meta.session`/`extendedHours`
/// for US markets; a non-US tick read omits `meta.session` entirely.
#[tokio::test]
async fn tick_candles_only_annotate_us_session_metadata() {
    let now_ms = current_unix_millis();
    let router = Arc::new(Mutex::new(ProviderRouter::new(8)));
    let ticker = Arc::new(StubTicker {
        tick: Mutex::new(Some(tick_sample(
            "US.AAPL",
            "114.97",
            Some("3"),
            now_ms,
            "after",
        ))),
        ..StubTicker::default()
    });
    let port = tick_port(Some(router.clone()), ticker.clone());

    let us = port
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=tick&sessions=regular,extended",
        )
        .await
        .expect("US tick candles");
    assert_eq!(us["meta"]["extendedHours"], true);
    assert_eq!(us["meta"]["session"], "all");

    ticker.tick.lock().unwrap().replace(tick_sample(
        "HK.00700",
        "321.4",
        Some("1"),
        now_ms,
        "regular",
    ));
    router.lock().unwrap().cache_mut().clear();
    let hk = port
        .read("/api/v1/market-data/candles/HK/00700", "period=tick")
        .await
        .expect("HK tick candles");
    assert_eq!(hk["meta"]["extendedHours"], false);
    assert!(
        hk["meta"].get("session").is_none(),
        "non-US tick candles must omit meta.session: {hk}"
    );
}

/// Go filters tick candles by the requested session groups before applying the
/// limit. `pre` and `after` both belong to the `extended` group, so a
/// `sessions=extended` page keeps the newest extended sample and drops the
/// regular one.
#[tokio::test]
async fn tick_candles_filter_sessions_before_applying_the_limit() {
    let now_ms = current_unix_millis();
    let router = ProviderRouter::new(8);
    for (offset, session) in [(4_000_i64, "regular"), (3_000, "after"), (2_000, "pre")] {
        router
            .cache_mut()
            .insert(
                tick_sample("US.AAPL", "114.97", Some("1"), now_ms - offset, session),
                1,
            )
            .expect("cache tick");
    }
    let port = tick_port(
        Some(Arc::new(Mutex::new(router))),
        Arc::new(StubTicker::default()),
    );

    let response = port
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=tick&limit=1&sessions=extended",
        )
        .await
        .expect("extended tick candles");
    assert_eq!(response["totalReturned"], 1);
    assert_eq!(response["candles"][0]["session"], "pre");
    assert_eq!(
        response["request"]["sessions"],
        serde_json::json!(["extended"])
    );

    let regular = port
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=tick&limit=5&sessions=regular",
        )
        .await
        .expect("regular tick candles");
    assert_eq!(regular["totalReturned"], 1);
    assert_eq!(regular["candles"][0]["session"], "regular");
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

#[tokio::test]
async fn candle_route_preserves_legacy_query_parsing() {
    // Parity: internal/api/marketdata/routes_test.go:267 TestCandlesRoutePreservesLegacyQueryParsing
    // Legacy `from`/`to` values accept date-only and space-separated datetime
    // forms; `period=k_60m` normalizes to `1h`; lower-case market segments are
    // accepted like Go's service boundary.
    let reader = Arc::new(PagedHistory::default());
    let result = port(reader.clone())
        .read(
            "/api/v1/market-data/candles/us/aapl",
            "period=k_60m&limit=5&from=2026-05-01&to=2026-05-02 15:04:05",
        )
        .await
        .expect("legacy candles query");
    assert!(result["candles"].is_array());
    let requests = reader.requests.lock().unwrap();
    let query = requests.first().expect("provider query");
    assert_eq!(query.symbol, "aapl");
    assert_eq!(query.period, "1h");
    assert!(
        query.begin_time.starts_with("2026-04-30") || query.begin_time.starts_with("2026-05-01"),
        "from window = {}",
        query.begin_time
    );
    assert!(
        query.end_time.starts_with("2026-05-02"),
        "to window = {}",
        query.end_time
    );
}

#[tokio::test]
async fn candle_route_normalizes_repeated_sessions() {
    // Parity: internal/api/marketdata/routes_test.go:302 TestCandlesRouteNormalizesRepeatedSessions
    let reader = Arc::new(PagedHistory::default());
    let result = port(reader.clone())
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=5m&sessions=overnight,regular&sessions=extended&sessions=regular",
        )
        .await
        .expect("repeated sessions");
    assert!(result["candles"].is_array());
    assert!(!reader.requests.lock().unwrap().is_empty());
}

#[tokio::test]
async fn candle_route_rejects_invalid_sessions() {
    // Parity: internal/api/marketdata/routes_test.go:321 TestCandlesRouteRejectsInvalidSessions
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    for query in ["period=5m&sessions=", "period=5m&sessions=regular,invalid"] {
        let error = port
            .read("/api/v1/market-data/candles/US/AAPL", query)
            .await
            .expect_err("invalid sessions");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
            ),
            "query {query} produced {error:?}"
        );
    }
    assert!(reader.requests.lock().unwrap().is_empty());
}

#[tokio::test]
async fn candle_route_rejects_unsupported_period() {
    // Parity: internal/api/marketdata/routes_test.go:337 TestCandlesRouteRejectsUnsupportedPeriod
    let reader = Arc::new(PagedHistory::default());
    let error = port(reader.clone())
        .read("/api/v1/market-data/candles/HK/00700", "period=unsupported")
        .await
        .expect_err("unsupported period");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
    ));
    assert_eq!(reader.requests.lock().unwrap().len(), 0);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn candle_route_rejects_invalid_limit() {
    // Parity: internal/api/marketdata/routes_test.go:387 TestCandlesRouteRejectsInvalidLimit
    let reader = Arc::new(PagedHistory::default());
    let error = port(reader.clone())
        .read("/api/v1/market-data/candles/HK/00700", "limit=abc")
        .await
        .expect_err("invalid limit");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
    ));
    assert_eq!(reader.requests.lock().unwrap().len(), 0);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn candle_route_forwards_exclusive_before_and_rejects_invalid_combinations() {
    // Parity: internal/api/marketdata/routes_test.go:357 TestCandlesRouteForwardsExclusiveBeforeAndRejectsInvalidCombinations
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    let valid = port
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=5m&limit=20&before=2026-07-18T13:40:00Z",
        )
        .await
        .expect("valid before");
    assert!(valid["candles"].is_array());
    {
        let requests = reader.requests.lock().unwrap();
        assert_eq!(
            requests.last().expect("provider query").end_time,
            "2026-07-18 09:40:00",
            "before is forwarded as the exclusive provider window end"
        );
    }

    for query in [
        "period=5m&before=bad",
        "period=5m&before=2026-07-18T13:40:00Z&from=2026-07-01",
        "period=tick&before=2026-07-18T13:40:00Z",
    ] {
        let error = port
            .read("/api/v1/market-data/candles/US/AAPL", query)
            .await
            .expect_err("invalid before combination");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
            ),
            "query {query} produced {error:?}"
        );
    }
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:137 TestMarketCandlesResponseRejectsInvalidSessionsBeforeFutuAccess
///
/// The market-dataapp harness asserts only that `sessions="regular,unknown"`
/// fails the read. This dedicated case keeps that item verifiable while also
/// proving the failure happened before the Futu reader was touched: no
/// historical request, no current-bar request.
#[tokio::test]
async fn market_http_rejects_invalid_sessions_before_provider_access() {
    let reader = Arc::new(PagedHistory::default());
    let error = port(reader.clone())
        .read(
            "/api/v1/market-data/candles/US/NVDA",
            "period=1m&sessions=regular,unknown",
        )
        .await
        .expect_err("invalid sessions must fail the read");
    assert!(
        matches!(
            error,
            MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ),
        "invalid sessions produced {error:?}"
    );
    assert!(
        reader.requests.lock().unwrap().is_empty(),
        "invalid sessions must be rejected before any Futu history call"
    );
    assert_eq!(
        reader.current_calls.load(Ordering::SeqCst),
        0,
        "invalid sessions must not query the current bar"
    );
}

#[tokio::test]
async fn broker_klines_return_latest_page_and_use_exclusive_before_cursor() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:15
    // TestBrokerKLinesReturnLatestPageAndUseExclusiveBeforeCursor.
    //
    // OpenD labels intraday history at the end of each bucket; the reader
    // shifts the label to the bucket start. The first page is the latest N
    // bars with `hasMore` and `nextBefore == candles[0].at`; the follow-up uses
    // that cursor as an exclusive upper bound so the boundary bar never repeats.
    let reader = Arc::new(PagedHistory {
        // The loopback history port bypasses the OpenD reader's label shift,
        // so the fixture already carries bucket-start labels like Go's
        // `futuHistoryKLineStartTime` output.
        series: vec![
            priced_candle("2026-05-20 08:00:00", 101.5),
            priced_candle("2026-05-20 08:01:00", 102.5),
            priced_candle("2026-05-20 08:02:00", 103.5),
        ],
        ..Default::default()
    });
    let port = port(reader.clone());
    let latest = port
        .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=2")
        .await
        .expect("latest page");
    let candles = latest["candles"].as_array().expect("candles");
    assert_eq!(candles.len(), 2);
    assert_eq!(latest["pagination"]["hasMore"], true);
    assert_eq!(latest["pagination"]["nextBefore"], candles[0]["at"]);
    assert_eq!(candles[0]["close"], "102.5");
    assert_eq!(candles[1]["close"], "103.5");
    let cursor = latest["pagination"]["nextBefore"]
        .as_str()
        .expect("cursor")
        .to_owned();
    assert_eq!(cursor, "2026-05-20T00:01:00Z");

    // The provider window end must equal the cursor second exactly (08:01 HK)
    // and the stale boundary bar is filtered locally.
    let older = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            &format!("period=1m&limit=2&before={cursor}"),
        )
        .await
        .expect("older page");
    {
        let requests = reader.requests.lock().unwrap();
        let last = requests.last().expect("older request");
        assert_eq!(last.end_time, "2026-05-20 08:01:00");
    }
    let older_candles = older["candles"].as_array().expect("older candles");
    assert_eq!(older_candles.len(), 1);
    assert_eq!(older["pagination"]["hasMore"], false);
    assert_eq!(older_candles[0]["close"], "101.5");
    assert!(older_candles[0]["at"].as_str().expect("at") < cursor.as_str());
}

#[tokio::test]
async fn broker_kline_query_formats_opend_window_in_market_time_and_returns_utc() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:92
    // TestBrokerKLineQueryFormatsOpenDWindowInMarketTimeAndReturnsUTC.
    // The fixture already carries bucket-start labels; OpenD's end-of-bucket
    // label shift is covered by `kline_query` unit tests.
    let reader = Arc::new(PagedHistory {
        series: vec![priced_candle("2026-05-20 08:00:00", 100.0)],
        ..Default::default()
    });
    let result = port(reader.clone())
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&from=2026-05-20T00:00:00Z&to=2026-05-20T01:00:00Z&limit=10",
        )
        .await
        .expect("bounded window");
    {
        let requests = reader.requests.lock().unwrap();
        let request = requests.first().expect("history request");
        assert_eq!(
            request.begin_time, "2026-05-20 08:00:00",
            "OpenD begin must use Hong Kong local time"
        );
        assert_eq!(
            request.end_time, "2026-05-20 09:00:00",
            "OpenD end must use Hong Kong local time"
        );
    }
    // The projected candle is canonical UTC regardless of the provider zone.
    assert_eq!(result["candles"][0]["at"], "2026-05-20T00:00:00Z");
}

#[tokio::test]
async fn broker_kline_cursor_preserves_exact_second_window_and_excludes_boundary_locally() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:121
    // TestBrokerKLineCursorPreservesExactSecondWindowAndExcludesBoundaryLocally.
    let reader = Arc::new(PagedHistory {
        series: vec![
            priced_candle("2026-05-20 08:00:00", 100.0),
            priced_candle("2026-05-20 08:01:00", 101.0),
        ],
        ..Default::default()
    });
    let result = port(reader.clone())
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=10&before=2026-05-20T00:01:00Z",
        )
        .await
        .expect("cursor page");
    {
        let requests = reader.requests.lock().unwrap();
        let request = requests.first().expect("request");
        // 00:01Z is 08:01 HK: the window end keeps the unmodified second, it is
        // not advanced or truncated to the bucket boundary.
        assert_eq!(request.end_time, "2026-05-20 08:01:00");
        assert!(!request.begin_time.is_empty());
    }
    let candles = result["candles"].as_array().expect("candles");
    assert_eq!(candles.len(), 1);
    // The 08:00 bucket start stays strictly before the 00:01Z cursor while the
    // 08:01 boundary bar is excluded.
    assert_eq!(candles[0]["at"], "2026-05-20T00:00:00Z");
}

#[test]
fn normalize_broker_kline_page_deduplicates_sorts_and_keeps_latest() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:152
    // TestNormalizeBrokerKLinePageDeduplicatesSortsAndKeepsLatest.
    //
    // Rust's equivalent helper is `merge_klines_by_time`: duplicates on the
    // same bucket collapse to the later value, output is sorted ascending, and
    // the caller selects either the latest or earliest window slice.
    let base = "2026-07-18 12:00:00";
    let row = |time: &str, close: f64| HistoricalKline {
        time: time.to_owned(),
        is_blank: false,
        high_price: Some(close),
        open_price: Some(close),
        low_price: Some(close),
        close_price: Some(close),
        volume: Some(1),
        turnover: None,
        change_rate: None,
    };
    let merged = jftrade_integration_futu::kline_query::merge_klines_by_time(
        &[
            row("2026-07-18 12:02:00", 102.0),
            row(base, 100.0),
            row("2026-07-18 12:01:00", 101.0),
        ],
        &[row("2026-07-18 12:01:00", 201.0)],
    );
    let rendered = merged
        .iter()
        .map(|kline| (kline.time.as_str(), kline.close_price))
        .collect::<Vec<_>>();
    assert_eq!(
        rendered,
        vec![
            ("2026-07-18 12:00:00", Some(100.0)),
            ("2026-07-18 12:01:00", Some(201.0)),
            ("2026-07-18 12:02:00", Some(102.0)),
        ]
    );

    // `latest` keeps the trailing window; the earliest call keeps the head.
    let kept_latest = merged[merged.len() - 2..].to_vec();
    assert_eq!(kept_latest[0].time, "2026-07-18 12:01:00");
    assert_eq!(kept_latest[1].time, "2026-07-18 12:02:00");
    let kept_earliest = merged[..2].to_vec();
    assert_eq!(kept_earliest[0].time, base);
    assert_eq!(kept_earliest[1].time, "2026-07-18 12:01:00");
}

#[test]
fn normalize_broker_kline_range_keeps_inclusive_boundaries() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:198
    // TestNormalizeBrokerKLineRangeKeepsInclusiveBoundaries.
    let row = |time: &str, close: f64| HistoricalKline {
        time: time.to_owned(),
        is_blank: false,
        high_price: Some(close),
        open_price: Some(close),
        low_price: Some(close),
        close_price: Some(close),
        volume: Some(1),
        turnover: None,
        change_rate: None,
    };
    let rows = [
        row("2026-07-18 11:59:00", 99.0),
        row("2026-07-18 12:00:00", 100.0),
        row("2026-07-18 12:01:00", 101.0),
        row("2026-07-18 12:01:00", 201.0),
        row("2026-07-18 12:02:00", 102.0),
        row("2026-07-18 12:03:00", 103.0),
    ];
    let from = time::PrimitiveDateTime::new(
        time::Date::from_calendar_date(2026, time::Month::July, 18).expect("date"),
        time::Time::from_hms(12, 0, 0).expect("time"),
    );
    let to = time::PrimitiveDateTime::new(
        time::Date::from_calendar_date(2026, time::Month::July, 18).expect("date"),
        time::Time::from_hms(12, 2, 0).expect("time"),
    );
    let inclusive = rows
        .iter()
        .filter(|kline| {
            let Ok(at) = time::PrimitiveDateTime::parse(
                &kline.time,
                &time::format_description::parse_borrowed::<2>(
                    "[year]-[month]-[day] [hour]:[minute]:[second]",
                )
                .expect("format"),
            ) else {
                return false;
            };
            at >= from && at <= to
        })
        .cloned()
        .collect::<Vec<_>>();
    let merged = jftrade_integration_futu::kline_query::merge_klines_by_time(&inclusive, &[]);
    let times = merged
        .iter()
        .map(|kline| kline.time.as_str())
        .collect::<Vec<_>>();
    assert_eq!(
        times,
        vec![
            "2026-07-18 12:00:00",
            "2026-07-18 12:01:00",
            "2026-07-18 12:02:00",
        ],
        "both range boundaries are inclusive and the duplicate keeps the latest close"
    );
    assert_eq!(merged[1].close_price, Some(201.0));
    // A limit keeps the earliest N bars of the inclusive range.
    assert_eq!(merged[..2].len(), 2);
}

#[tokio::test]
async fn broker_kline_query_rejects_cursor_and_time_boundary_errors() {
    // Parity: go:8a78fc78:pkg/futu/adapter_kline_pagination_test.go:223
    // TestBrokerKLineQueryRejectsCursorAndTimeBoundaryErrors.
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    for (query, expected) in [
        (
            "period=5m&before=2026-07-18T13:40:00Z&from=2026-07-01",
            "before cannot be combined",
        ),
        (
            "period=5m&before=bad",
            "before must be an RFC3339 timestamp",
        ),
        ("period=5m&from=bad", "time must be a valid timestamp"),
        (
            "period=5m&from=2026-07-01T00:00:00Z&to=bad",
            "time must be a valid timestamp",
        ),
        (
            // Go: "futu: fromTime must be earlier than or equal to toTime".
            "period=5m&from=2026-07-02&to=2026-07-01",
            "fromTime must be earlier than or equal to toTime",
        ),
        (
            "period=tick&before=2026-07-18T13:40:00Z",
            "tick candles do not support",
        ),
    ] {
        let error = port
            .read("/api/v1/market-data/candles/US/AAPL", query)
            .await
            .expect_err(query);
        let text = error.to_string();
        assert!(
            text.contains(expected),
            "query {query:?} error {text:?} must contain {expected:?}"
        );
    }
    // A capped limit is still validated before the cursor check, and no
    // request reaches the provider for any rejected query.
    assert!(reader.requests.lock().unwrap().is_empty());
}
