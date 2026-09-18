//! Parity coverage for `pkg/futu/snapshot_fallback_test.go` and
//! `pkg/futu/snapshot_fallback_parsing_test.go`.
//!
//! Go drives these cases through a scripted OpenD server plus the adapter's
//! `broker.SnapshotFallbackSource` capability. Rust keeps the same split: the
//! projection/batching/cache rules run against a fake fetch port here, while
//! `tests/snapshot_fallback_protocol.rs` drives the real framed OpenD path.

use super::*;
use crate::{OpenDTcpProbeConfig, SecuritySnapshotCancelToken, decode_frame};
use std::sync::atomic::{AtomicUsize, Ordering};

fn symbols(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| (*value).to_owned()).collect()
}

fn screen_row(stock_id: u64, price: f64, extra: &[(i32, f64)]) -> ScreenRow {
    let mut simple = BTreeMap::from([(2201, price)]);
    let mut cumulative = BTreeMap::new();
    for (property, value) in extra {
        if *property == CUMULATIVE_CHANGE_PCT {
            cumulative.insert(*property, *value);
        } else {
            simple.insert(*property, *value);
        }
    }
    ScreenRow {
        stock_id,
        simple,
        cumulative,
    }
}

/// Fake delayed OpenD reader: records the requested batches so the batching
/// rules and the "no subscription" contract stay observable.
#[derive(Default)]
struct FakeFetchPort {
    identities: Vec<StockIdentity>,
    pages: Mutex<Vec<(i64, Vec<u64>)>>,
    rows: Mutex<HashMap<i64, Vec<ScreenRow>>>,
    static_error: Option<String>,
    page_error: Option<String>,
    static_calls: AtomicUsize,
    screen_calls: AtomicUsize,
}

impl std::fmt::Debug for FakeFetchPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("FakeFetchPort")
            .finish_non_exhaustive()
    }
}

impl FakeFetchPort {
    fn new(identities: Vec<StockIdentity>) -> Self {
        Self {
            identities,
            ..Default::default()
        }
    }

    fn with_page(self, market_value: i64, rows: Vec<ScreenRow>) -> Self {
        self.rows
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .insert(market_value, rows);
        self
    }

    fn requested_pages(&self) -> Vec<(i64, Vec<u64>)> {
        self.pages
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }
}

impl SnapshotFallbackFetchPort for FakeFetchPort {
    fn static_info_ids(&self, _symbols: &[String]) -> Result<Vec<StockIdentity>, String> {
        self.static_calls.fetch_add(1, Ordering::SeqCst);
        match &self.static_error {
            Some(error) => Err(error.clone()),
            None => Ok(self.identities.clone()),
        }
    }

    fn stock_screen_page(
        &self,
        market_value: i64,
        stock_ids: &[u64],
    ) -> Result<Vec<ScreenRow>, String> {
        validate_snapshot_page(stock_ids).map_err(|error| error.to_string())?;
        self.screen_calls.fetch_add(1, Ordering::SeqCst);
        self.pages
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push((market_value, stock_ids.to_vec()));
        if let Some(error) = &self.page_error {
            return Err(error.clone());
        }
        let rows = self
            .rows
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .get(&market_value)
            .cloned()
            .unwrap_or_default();
        Ok(rows
            .into_iter()
            .filter(|row| stock_ids.contains(&row.stock_id))
            .collect())
    }
}

/// Loopback OpenD server used only by the case that drives the real adapter
/// entry point: it completes the handshake and then stops answering, because
/// the fake fetch port owns the reads in that test.
fn loopback_coordinator() -> (
    Arc<Mutex<OpenDSessionCoordinator>>,
    std::thread::JoinHandle<()>,
) {
    use prost::Message;
    use std::io::{Read, Write};
    use std::net::TcpListener;

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("address");
    let task = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let mut header = [0_u8; 44];
        stream.read_exact(&mut header).expect("init header");
        let body_len = u32::from_le_bytes(header[12..16].try_into().expect("len")) as usize;
        let mut packet = vec![0_u8; 44 + body_len];
        packet[..44].copy_from_slice(&header);
        stream.read_exact(&mut packet[44..]).expect("init body");
        let frame = decode_frame(&packet).expect("init frame");
        let body = InitResponse {
            ret_type: Some(0),
            s2c: Some(InitState {
                server_ver: 1009,
                conn_id: 7,
            }),
        }
        .encode_to_vec();
        stream
            .write_all(
                &crate::encode_frame(frame.header.proto_id, frame.header.serial_no, &body)
                    .expect("encode init response"),
            )
            .expect("write init response");
    });
    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
            Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default()),
            Vec::new(),
            0,
        )
        .expect("managed OpenD coordinator"),
    ));
    (coordinator, task)
}

fn identity(symbol: &str, stock_id: u64, name: Option<&str>) -> StockIdentity {
    StockIdentity {
        symbol: symbol.to_owned(),
        stock_id,
        name: name.map(str::to_owned),
    }
}

#[test]
fn snapshot_fallback_params_use_strict_delayed_quote_fields() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_test.go:18
    // TestStockScreenSnapshotParamsUseStrictDelayedQuoteFields
    let request = screen_wire::Request::decode(encode_snapshot_page(3, &[101, 202]).as_slice())
        .expect("decode snapshot page request");
    let c2s = request.c2s;

    assert_eq!(c2s.watchlist_stock_ids, vec![101, 202]);
    assert_eq!(c2s.page_count, Some(2));
    assert_eq!(c2s.page_from, None);
    assert_eq!(c2s.sort, None);
    assert!(c2s.sort_list.is_empty());

    assert_eq!(c2s.filter_list.len(), 2);
    for (index, (field, value)) in [(1_i32, 3_i64), (4, 1)].into_iter().enumerate() {
        let query = c2s.filter_list[index]
            .simple_field_query
            .as_ref()
            .expect("simple field query");
        assert_eq!(query.simple_field, Some(field));
        assert_eq!(query.screen_value_list, vec![value]);
    }
    // Neither filter may carry a property query: Go builds them from
    // `simpleFieldQuery` only.
    assert!(
        c2s.filter_list
            .iter()
            .all(|filter| filter.simple_property_query.is_none()
                && filter.cumulative_property_query.is_none())
    );

    let simple = c2s
        .retrieve_list
        .iter()
        .filter_map(|query| query.simple_property.as_ref().and_then(|p| p.name))
        .collect::<Vec<_>>();
    let cumulative = c2s
        .retrieve_list
        .iter()
        .filter_map(|query| query.cumulative_property.as_ref())
        .collect::<Vec<_>>();
    assert_eq!(simple, vec![2201, 2202, 2203, 2204, 2205, 2207, 2208]);
    assert_eq!(cumulative.len(), 1);
    assert_eq!(cumulative[0].name, Some(3101));
    assert_eq!(cumulative[0].days, Some(1));
    assert_eq!(c2s.retrieve_list.len(), SIMPLE_PROPERTIES.len() + 1);
}

#[test]
fn futu_stock_screen_snapshot_fallback_uses_static_ids_without_subscription() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_test.go:58
    // TestFutuStockScreenSnapshotFallbackUsesStaticIDsWithoutSubscription
    let port = FakeFetchPort::new(vec![
        identity("SH.600519", 101, Some("Kweichow Moutai")),
        identity("SZ.000001", 202, Some("Ping An Bank")),
        identity("US.AAPL", 303, Some("Apple")),
    ])
    .with_page(
        3,
        vec![
            screen_row(101, 1500.0, &[(2203, 1490.0)]),
            screen_row(202, 12.3, &[(CUMULATIVE_CHANGE_PCT, 0.3)]),
        ],
    );

    let items = fetch_delayed_snapshots(&port, &symbols(&["SH.600519", "SZ.000001", "US.AAPL"]))
        .expect("delayed snapshots");

    assert_eq!(
        items.len(),
        2,
        "a missing StockScreen row must not be synthesized"
    );
    let moutai = items.get("SH.600519").expect("SH.600519");
    assert_eq!(moutai.source, STOCK_SCREEN_SNAPSHOT_SOURCE);
    assert_eq!(moutai.name.as_deref(), Some("Kweichow Moutai"));
    assert_eq!(moutai.last_price, Some("1500".parse().expect("decimal")));
    assert_eq!(
        moutai.previous_close,
        Some("1490".parse().expect("decimal"))
    );
    let ping_an = items.get("SZ.000001").expect("SZ.000001");
    assert_eq!(ping_an.name.as_deref(), Some("Ping An Bank"));
    assert_eq!(ping_an.last_price, Some("12.3".parse().expect("decimal")));
    // `previousClose` is derived from `lastPrice - 3101` when 2203 is absent.
    assert_eq!(ping_an.previous_close, Some("12".parse().expect("decimal")));
    assert!(
        !items.contains_key("US.AAPL"),
        "US.AAPL had no StockScreen row but appeared in the result"
    );

    // Markets are read in ascending order and each market is one page: US.AAPL
    // resolves through static info even though the delayed page returns no row
    // for it, so the US page is real work and the A-share page follows at 3.
    assert_eq!(
        port.requested_pages(),
        vec![(2, vec![303]), (3, vec![101, 202])]
    );
    assert_eq!(
        port.screen_calls.load(Ordering::SeqCst),
        2,
        "one page per market with static info"
    );
    // The delayed read is read-only: it never creates a Qot_Sub. The fake
    // exposes no subscribe entry point at all, which is the Rust equivalent of
    // Go's `subCallCount() == 0`.
    assert_eq!(port.static_calls.load(Ordering::SeqCst), 1);
}

#[test]
fn stock_screen_snapshot_coordinator_caches_rows_and_negative_results() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_test.go:98
    // TestStockScreenSnapshotCoordinatorCachesRowsAndNegativeResults
    let clock = Arc::new(Mutex::new(
        std::time::SystemTime::UNIX_EPOCH + Duration::from_secs(1_000),
    ));
    let advance = {
        let clock = Arc::clone(&clock);
        move |delta: Duration| {
            let mut now = clock.lock().unwrap_or_else(|error| error.into_inner());
            *now += delta;
        }
    };
    let read_now = {
        let clock = Arc::clone(&clock);
        move || *clock.lock().unwrap_or_else(|error| error.into_inner())
    };
    let coordinator = StockScreenSnapshotCoordinator::with_settings(
        Arc::new(read_now),
        STOCK_SCREEN_SNAPSHOT_CACHE_TTL,
    );

    let calls = AtomicUsize::new(0);
    let requests = Mutex::new(Vec::new());
    let fetch = |batch: &[String]| {
        calls.fetch_add(1, Ordering::SeqCst);
        requests
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push(batch.to_vec());
        Ok(HashMap::from([(
            "SH.600519".to_owned(),
            DelayedSnapshotItem {
                symbol: "SH.600519".to_owned(),
                source: STOCK_SCREEN_SNAPSHOT_SOURCE.to_owned(),
                name: Some("Moutai".to_owned()),
                last_price: Some("1500".parse().expect("decimal")),
                ..Default::default()
            },
        )]))
    };

    let first = coordinator
        .query(&symbols(&["SZ.000001", "SH.600519"]), fetch)
        .expect("first query");
    assert_eq!(first.len(), 1);
    assert_eq!(calls.load(Ordering::SeqCst), 1);
    assert_eq!(
        requests.lock().unwrap_or_else(|error| error.into_inner())[0],
        symbols(&["SH.600519", "SZ.000001"]),
        "the fetch sees the canonical sorted batch"
    );

    // The cache is handed out as clones: mutating the caller's copy must not
    // leak into the next reader (Go asserts the same with clone helpers).
    let mut mutated = first.get("SH.600519").cloned().expect("row");
    mutated.name = Some("mutated".to_owned());
    let second = coordinator
        .query(&symbols(&["SH.600519", "SZ.000001"]), fetch)
        .expect("cached query");
    assert_eq!(
        calls.load(Ordering::SeqCst),
        1,
        "second query hit the cache"
    );
    assert_eq!(
        second
            .get("SH.600519")
            .and_then(|item| item.name.as_deref()),
        Some("Moutai")
    );

    // Go's freshness test is `expiresAt.After(now)`, so the TTL boundary itself
    // is already stale. Half a TTL later the entry is still served from cache.
    advance(STOCK_SCREEN_SNAPSHOT_CACHE_TTL / 2);
    coordinator
        .query(&symbols(&["SH.600519", "SZ.000001"]), fetch)
        .expect("mid-ttl query");
    assert_eq!(calls.load(Ordering::SeqCst), 1, "mid-TTL read stays cached");
    advance(STOCK_SCREEN_SNAPSHOT_CACHE_TTL / 2 + Duration::from_nanos(1));
    coordinator
        .query(&symbols(&["SH.600519", "SZ.000001"]), fetch)
        .expect("expired query");
    assert_eq!(calls.load(Ordering::SeqCst), 2, "expired entry was re-read");
    assert_eq!(
        requests.lock().unwrap_or_else(|error| error.into_inner())[1],
        symbols(&["SH.600519", "SZ.000001"])
    );

    // A failure is never cached, so the next call retries immediately.
    let failing = StockScreenSnapshotCoordinator::new();
    let attempts = AtomicUsize::new(0);
    let error = failing
        .query(&symbols(&["SH.600519"]), |_| {
            attempts.fetch_add(1, Ordering::SeqCst);
            Err(SnapshotFallbackError::Fetch(
                "StockScreen unavailable".to_owned(),
            ))
        })
        .expect_err("failed delayed fetch");
    assert_eq!(error.to_string(), "StockScreen unavailable");
    let recovered = failing
        .query(&symbols(&["SH.600519"]), |_| {
            attempts.fetch_add(1, Ordering::SeqCst);
            Ok(HashMap::new())
        })
        .expect("retry after failure");
    assert!(recovered.is_empty());
    assert_eq!(
        attempts.load(Ordering::SeqCst),
        2,
        "the failure was not cached"
    );

    // A negative result *is* cached: one physical read covers both lookups.
    let negative = StockScreenSnapshotCoordinator::new();
    let negative_calls = AtomicUsize::new(0);
    let miss = negative
        .query(&symbols(&["US.AAPL"]), |_| {
            negative_calls.fetch_add(1, Ordering::SeqCst);
            Ok(HashMap::new())
        })
        .expect("negative query");
    assert!(miss.is_empty());
    negative
        .query(&symbols(&["US.AAPL"]), |_| {
            negative_calls.fetch_add(1, Ordering::SeqCst);
            Ok(HashMap::new())
        })
        .expect("cached negative query");
    assert_eq!(
        negative_calls.load(Ordering::SeqCst),
        1,
        "negative results are cached for the TTL"
    );
}

#[test]
fn futu_stock_screen_snapshot_fallback_reports_screen_errors() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_test.go:153
    // TestFutuStockScreenSnapshotFallbackReportsScreenErrors
    let port = FakeFetchPort {
        identities: vec![identity("SH.600519", 101, Some("Kweichow Moutai"))],
        page_error: Some(
            "OpenD Qot_StockScreen returned retType=-1 errCode=9: StockScreen unavailable"
                .to_owned(),
        ),
        ..Default::default()
    };

    let error =
        fetch_delayed_snapshots(&port, &symbols(&["SH.600519"])).expect_err("screen rejection");
    let message = error.to_string();
    assert!(
        message.contains("Qot_StockScreen"),
        "error must name the OpenD protocol: {message}"
    );
    assert!(message.contains("StockScreen unavailable"));
    // The failure happens before (and instead of) any subscription work.
    assert_eq!(port.screen_calls.load(Ordering::SeqCst), 1);
    assert_eq!(port.static_calls.load(Ordering::SeqCst), 1);
}

#[test]
fn stock_screen_fallback_wire_value_helpers_match_go_coercions() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:14
    // TestStockScreenFallbackWireValueHelpers
    // Rust decodes the protobuf into typed fields instead of Go's
    // `map[string]any`, so the coercions collapse to the typed result_number /
    // finite filters exercised here.
    assert_eq!(result_number(Some(1.5), Some(9)), Some(1.5));
    assert_eq!(result_number(Some(f64::NAN), Some(9)), Some(9.0));
    assert_eq!(result_number(Some(f64::INFINITY), Some(9)), Some(9.0));
    assert_eq!(result_number(None, Some(4)), Some(4.0));
    assert_eq!(result_number(None, None), None);
    assert_eq!(result_number(Some(f64::NAN), None), None);

    let values = BTreeMap::from([(1, f64::INFINITY), (2, 3.0)]);
    assert_eq!(
        optional(&values, 1),
        None,
        "non-finite optionals are dropped"
    );
    assert_eq!(optional(&values, 2), Some("3".parse().expect("decimal")));
    assert_eq!(optional(&values, 3), None);
    assert!(!positive(0.0));
    assert!(!positive(-1.0));
    assert!(!positive(f64::NAN));
    assert!(positive(0.5));

    // previousClose = lastPrice - 3101, and only when the result is positive.
    let derived = project_screen_page_at(
        &[screen_row(1, 10.0, &[(CUMULATIVE_CHANGE_PCT, 2.0)])],
        &[identity("US.AAPL", 1, None)],
        None,
    );
    assert_eq!(
        derived["US.AAPL"].previous_close,
        Some("8".parse().expect("decimal"))
    );
    let non_positive = project_screen_page_at(
        &[screen_row(1, 1.0, &[(CUMULATIVE_CHANGE_PCT, 2.0)])],
        &[identity("US.AAPL", 1, None)],
        None,
    );
    assert_eq!(
        non_positive["US.AAPL"].previous_close, None,
        "a non-positive derived previous close is dropped"
    );
}

#[test]
fn stock_screen_fallback_parses_rows_and_market_groups() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:93
    // TestStockScreenFallbackParsesRowsAndMarketGroups
    for (market, expected) in [
        ("HK", Some(1)),
        ("US", Some(2)),
        ("CN", Some(3)),
        ("SH", Some(3)),
        ("SZ", Some(3)),
        ("SG", Some(4)),
        ("CA", Some(5)),
        ("AU", Some(6)),
        ("JP", Some(7)),
        ("MY", Some(8)),
        ("OTHER", None),
        (" us ", Some(2)),
    ] {
        assert_eq!(screen_market_value(market), expected, "market {market}");
    }

    // Rows without a usable 2201 price, unknown stock ids and malformed rows
    // are dropped; the surviving row keeps every retrieved field.
    let rows = vec![
        screen_row(2, 0.0, &[]),
        screen_row(3, 0.0, &[]),
        screen_row(
            1,
            100.0,
            &[
                (2202, 99.0),
                (2204, 101.0),
                (2205, 98.0),
                (2207, 99.5),
                (2208, 100.5),
                (CUMULATIVE_CHANGE_PCT, 2.0),
            ],
        ),
    ];
    let identities = vec![
        identity("US.AAPL", 1, Some("Apple")),
        identity("US.ZERO", 2, None),
        // stock id 3 is not part of the request, so its row is ignored.
    ];
    // 2026-08-07T14:00:00Z == 10:00 New York, inside the US regular session.
    let items = project_screen_page_at(&rows, &identities, Some(1_786_111_200_000));

    assert_eq!(items.len(), 1, "only the priced, requested row survives");
    let apple = items.get("US.AAPL").expect("US.AAPL");
    assert_eq!(apple.source, STOCK_SCREEN_SNAPSHOT_SOURCE);
    assert_eq!(apple.name.as_deref(), Some("Apple"));
    assert_eq!(apple.last_price, Some("100".parse().expect("decimal")));
    // 2203 is absent, so the 3101 change derives the previous close.
    assert_eq!(apple.previous_close, Some("98".parse().expect("decimal")));
    assert_eq!(apple.open_price, Some("99".parse().expect("decimal")));
    assert_eq!(apple.high_price, Some("101".parse().expect("decimal")));
    assert_eq!(apple.low_price, Some("98".parse().expect("decimal")));
    assert_eq!(apple.bid_price, Some("99.5".parse().expect("decimal")));
    assert_eq!(apple.ask_price, Some("100.5".parse().expect("decimal")));
    assert_eq!(apple.session.as_deref(), Some("regular"));

    // A row whose stock id is absent from the identity map is skipped instead
    // of being attributed to another symbol.
    let unmatched = project_screen_page_at(&[screen_row(9, 10.0, &[])], &identities, None);
    assert!(unmatched.is_empty());
}

#[test]
fn stock_screen_fallback_coordinates_copies_and_errors() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:146
    // TestStockScreenFallbackCoordinatesCopiesAndErrors
    let coordinator = StockScreenSnapshotCoordinator::new();
    assert!(
        coordinator
            .query(&[], |_| Ok(HashMap::new()))
            .expect("empty query")
            .is_empty()
    );
    assert!(matches!(
        coordinator.query(&symbols(&["invalid"]), |_| Ok(HashMap::new())),
        Err(SnapshotFallbackError::InvalidInstrument(_))
    ));
    assert!(matches!(
        coordinator.query(&symbols(&["MARS.AAPL"]), |_| Ok(HashMap::new())),
        Err(SnapshotFallbackError::InvalidInstrument(_))
    ));
    assert_eq!(
        canonical_fallback_symbols(&symbols(&["HK.00700", " hk.00700 ", "US.AAPL"]))
            .expect("canonical symbols"),
        symbols(&["HK.00700", "US.AAPL"])
    );
    assert!(matches!(
        canonical_fallback_symbols(&symbols(&["BROKEN"])),
        Err(SnapshotFallbackError::InvalidInstrument(_))
    ));
    assert!(matches!(
        canonical_fallback_symbols(["HK.   ".to_owned()].as_slice()),
        Err(SnapshotFallbackError::InvalidInstrument(_))
    ));

    // The page owner rejects empty and oversized pages before any OpenD call.
    let port = FakeFetchPort::default();
    assert!(matches!(
        port.stock_screen_page(1, &[]),
        Err(message) if message == "futu stock-screen snapshot page requires 1..300 stock ids"
    ));
    let oversized = vec![7_u64; STOCK_SCREEN_SNAPSHOT_PAGE_SIZE + 1];
    assert!(port.stock_screen_page(1, &oversized).is_err());
    assert_eq!(
        port.screen_calls.load(Ordering::SeqCst),
        0,
        "rejected locally"
    );

    // Errors stay typed so the caller can distinguish an unusable adapter from
    // a rejected instrument.
    assert_eq!(
        SnapshotFallbackError::Unavailable.to_string(),
        "futu stock-screen snapshot fallback is unavailable"
    );
    assert_eq!(
        SnapshotFallbackError::EmptySymbols.to_string(),
        "futu: QuerySnapshotFallback requires at least one symbol"
    );
    assert_eq!(
        SnapshotFallbackError::InvalidResult.to_string(),
        "futu stock-screen snapshot fallback returned an invalid result"
    );
}

#[test]
fn stock_screen_fallback_pages_and_sorts_markets_like_go() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback.go:40-320
    // (`stockScreenSnapshotPageSize` paging plus ascending market values).
    let identities = vec![
        identity("US.B", 2, None),
        identity("HK.A", 1, Some("HK A")),
        identity("US.A", 3, None),
    ];
    let port = FakeFetchPort::new(identities)
        .with_page(1, vec![screen_row(1, 10.0, &[])])
        .with_page(2, vec![screen_row(2, 20.0, &[]), screen_row(3, 30.0, &[])]);
    let items = fetch_delayed_snapshots(&port, &symbols(&["US.B", "HK.A", "US.A"]))
        .expect("delayed snapshots");
    assert_eq!(items.len(), 3);
    assert_eq!(
        port.requested_pages(),
        vec![(1, vec![1]), (2, vec![2, 3])],
        "market values ascend and each market is one page"
    );

    // More than 300 ids in one market must be split at the page size.
    let mut identities = Vec::new();
    let mut rows = Vec::new();
    for index in 0..(STOCK_SCREEN_SNAPSHOT_PAGE_SIZE + 2) {
        let stock_id = index as u64 + 1;
        identities.push(identity(&format!("US.S{index:04}"), stock_id, None));
        rows.push(screen_row(stock_id, 10.0 + index as f64, &[]));
    }
    let port = FakeFetchPort::new(identities).with_page(2, rows);
    let requested = port
        .identities
        .iter()
        .map(|identity| identity.symbol.clone())
        .collect::<Vec<_>>();
    let items = fetch_delayed_snapshots(&port, &requested).expect("paged snapshots");
    let pages = port.requested_pages();
    assert_eq!(pages.len(), 2);
    assert_eq!(pages[0].1.len(), STOCK_SCREEN_SNAPSHOT_PAGE_SIZE);
    assert_eq!(pages[1].1.len(), 2);
    assert_eq!(items.len(), STOCK_SCREEN_SNAPSHOT_PAGE_SIZE + 2);

    // A static-info failure is surfaced instead of silently returning nothing.
    let failing = FakeFetchPort {
        static_error: Some("Qot_GetStaticInfo rejected".to_owned()),
        ..Default::default()
    };
    let error = fetch_delayed_snapshots(&failing, &symbols(&["SH.600519"]))
        .expect_err("static-info failure");
    assert_eq!(error.to_string(), "Qot_GetStaticInfo rejected");
}

#[test]
fn stock_screen_fallback_cancellation_and_adapter_entry_point() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback_parsing_test.go:146
    // (nil-adapter / empty-query / cancellation branches) and
    // `pkg/futu/snapshot_fallback.go:185` `QuerySnapshotFallback` ordering.
    let failing = FakeFetchPort {
        identities: vec![identity("HK.00700", 1, None)],
        page_error: Some("StockScreen unavailable".to_owned()),
        ..Default::default()
    };

    // A failed physical read surfaces to the caller and is not cached, so the
    // next read retries.
    let reader: Arc<dyn SnapshotFallbackFetchPort> = Arc::new(failing);
    let cache = StockScreenSnapshotCoordinator::new();
    let error = cache
        .query(&symbols(&["HK.00700"]), |batch| {
            fetch_delayed_snapshots(reader.as_ref(), batch)
        })
        .expect_err("screen failure");
    assert_eq!(error.to_string(), "StockScreen unavailable");

    // The adapter-level entry point rejects an empty request the way Go does,
    // and returns rows in the caller's requested order.
    let port = FakeFetchPort::new(vec![
        identity("HK.00700", 1, Some("Tencent")),
        identity("US.AAPL", 2, Some("Apple")),
    ])
    .with_page(1, vec![screen_row(1, 100.0, &[])])
    .with_page(2, vec![screen_row(2, 200.0, &[])]);
    let reader: Arc<dyn SnapshotFallbackFetchPort> = Arc::new(port);
    let clock = Arc::new(|| std::time::SystemTime::UNIX_EPOCH);
    let (coordinator, server) = loopback_coordinator();
    let fallback = StockScreenSnapshotFallback::with_reader(
        reader,
        StockScreenSnapshotCoordinator::with_settings(clock, STOCK_SCREEN_SNAPSHOT_CACHE_TTL),
        Arc::clone(&coordinator),
    );
    assert!(matches!(
        fallback.query(&[]),
        Err(SnapshotFallbackError::EmptySymbols)
    ));
    assert!(matches!(
        fallback.query(&symbols(&["NOT-A-SYMBOL"])),
        Err(SnapshotFallbackError::InvalidInstrument(_))
    ));
    let items = fallback
        .query(&symbols(&["US.AAPL", "HK.00700", "US.MISSING"]))
        .expect("adapter query");
    assert_eq!(
        items
            .iter()
            .map(|item| item.symbol.as_str())
            .collect::<Vec<_>>(),
        vec!["US.AAPL", "HK.00700"],
        "rows follow the requested order and unknown symbols are absent"
    );
    assert_eq!(items[0].source, STOCK_SCREEN_SNAPSHOT_SOURCE);

    coordinator.lock().expect("lock").close().expect("close");
    server.join().expect("loopback server");
}

#[test]
fn stock_screen_fallback_waiter_can_be_canceled_before_the_read_completes() {
    // Parity: go:452dea11:pkg/futu/snapshot_fallback.go:57-104
    // (`select` on `ctx.Done()` while parked on the single-flight result).
    let coordinator = StockScreenSnapshotCoordinator::new();
    let cancel = SecuritySnapshotCancelToken::new();
    cancel.cancel();
    let error = coordinator
        .query_with_cancel(&symbols(&["HK.00700"]), Some(&cancel), |_| {
            Ok(HashMap::new())
        })
        .expect_err("canceled wait");
    assert!(matches!(error, SnapshotFallbackError::Canceled));
}
