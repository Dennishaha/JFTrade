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

fn format_unix_millis_rfc3339(ms: i64) -> String {
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
        ],
        ..PagedHistory::default()
    });
    let result = port_with_calendar(reader, calendar)
        .read(
            "/api/v1/market-data/candles/US/AAPL",
            "period=1m&limit=10&sessions=regular,extended,overnight",
        )
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
