//! Behavior tests mirroring `go:pkg/futu/adapter_earnings_calendar_test.go`.
//!
//! Each Rust test maps to one Go `Test*` and asserts the same business
//! semantics: parameter translation, OpenD 7-day chunking, whole-range
//! failure, exact segment order, validation edges and deduplication.

use super::*;
use jftrade_integration_futu::{
    EarningsCalendarPage, EarningsCalendarQueryError, EarningsCalendarReadPort,
    EarningsCalendarSecurity,
};
use std::sync::Mutex;

/// A port fixture that records every chunk request and can fail one segment,
/// standing in for the Go `collectEarningsCalendarChunks` callback.
#[derive(Debug)]
struct RecordingEarningsCalendarReader {
    calls: Mutex<Vec<EarningsCalendarQuery>>,
    fail_on_call: Option<usize>,
    items: Vec<EarningsCalendarItem>,
}

impl RecordingEarningsCalendarReader {
    fn new(fail_on_call: Option<usize>) -> Self {
        Self {
            calls: Mutex::new(Vec::new()),
            fail_on_call,
            items: Vec::new(),
        }
    }

    fn calls(&self) -> Vec<EarningsCalendarQuery> {
        self.calls.lock().expect("calls lock").clone()
    }
}

impl EarningsCalendarReadPort for RecordingEarningsCalendarReader {
    fn query(
        &self,
        query: &EarningsCalendarQuery,
    ) -> Result<EarningsCalendarPage, EarningsCalendarQueryError> {
        let mut calls = self.calls.lock().expect("calls lock");
        calls.push(query.clone());
        if Some(calls.len()) == self.fail_on_call {
            return Err(EarningsCalendarQueryError::Rejected {
                ret_type: -1,
                err_code: 1001,
                message: "second OpenD segment failed".to_owned(),
            });
        }
        Ok(EarningsCalendarPage {
            items: self.items.clone(),
        })
    }
}

fn query_map(raw: &str) -> QueryMap {
    QueryMap::parse(raw).expect("query map")
}

fn translated(raw: &str, market: &str) -> TranslatedEarningsParams {
    translate_earnings_calendar_params(&query_map(raw), market).expect("translated params")
}

fn item(date: &str, instrument_id: &str, name: &str) -> EarningsCalendarItem {
    let (market, code) = instrument_id.split_once('.').expect("market.code");
    EarningsCalendarItem {
        security: EarningsCalendarSecurity {
            market: market.to_owned(),
            code: code.to_owned(),
            instrument_id: instrument_id.to_owned(),
        },
        name: Some(name.to_owned()),
        earnings_date: Some(date.to_owned()),
        earnings_timestamp: None,
        pub_type: None,
        period_text: None,
        estimate_list: Vec::new(),
        option_volume: None,
        iv: None,
        iv_rank: None,
        iv_percentile: None,
        market_cap: None,
        price: None,
    }
}

#[test]
fn translate_earnings_calendar_params_maps_business_semantics() {
    let params = translated(
        "sort=iv_percentile&stockScope=watchlist&marketCapMin=1000000000\
         &optionVolumeMax=20000&ivMin=10&ivMax=80&ivRankMin=5\
         &ivPercentileMax=95&beginDate=2026-07-01&endDate=2026-07-31",
        "US",
    );
    assert_eq!(params.sort_type, Some(6));
    assert_eq!(params.filters.len(), 6);
    assert_eq!(params.filters[0].indicator_type, 4);
    assert_eq!(params.filters[0].value_list, vec![1]);
    assert!(params.filters[0].interval.is_none());
    assert_eq!(params.filters[1].indicator_type, 3);
    assert_eq!(params.filters[2].indicator_type, 5);
    assert_eq!(params.filters[3].indicator_type, 6);
    assert_eq!(params.filters[4].indicator_type, 7);
    assert_eq!(params.filters[5].indicator_type, 8);

    let iv = params.filters[3].interval.as_ref().expect("iv interval");
    assert_eq!(iv.min.as_ref().map(|value| value.value), Some(10.0));
    assert_eq!(iv.max.as_ref().map(|value| value.value), Some(80.0));
    assert_eq!(iv.min.as_ref().map(|value| value.includes), Some(true));
    assert_eq!(iv.max.as_ref().map(|value| value.includes), Some(true));

    // The translated dates stay available to the chunker verbatim.
    let dates = query_map("beginDate=2026-07-01&endDate=2026-07-31");
    assert_eq!(dates.get_first("beginDate"), Some("2026-07-01"));
    assert_eq!(dates.get_first("endDate"), Some("2026-07-31"));
}

#[test]
fn translate_earnings_calendar_params_rejects_unsupported_market_conditions() {
    assert!(matches!(
        translate_earnings_calendar_params(&query_map("sort=iv"), "SH"),
        Err(ResearchReadSnapshotError::Invalid(message)) if message.contains("HK/US")
    ));
    assert!(matches!(
        translate_earnings_calendar_params(&query_map("ivMin=10"), "SZ"),
        Err(ResearchReadSnapshotError::Invalid(message)) if message.contains("ivMin/ivMax")
    ));
}

#[test]
fn earnings_calendar_date_chunks_limit_every_opend_call_to_seven_days() {
    let chunks =
        earnings_calendar_date_chunks(&query_map("beginDate=2026-06-28&endDate=2026-08-08"))
            .expect("chunks");
    let rendered = chunks
        .iter()
        .map(|chunk| format!("{}..{}", chunk.begin, chunk.end))
        .collect::<Vec<_>>();
    assert_eq!(
        rendered,
        vec![
            "2026-06-28..2026-07-04",
            "2026-07-05..2026-07-11",
            "2026-07-12..2026-07-18",
            "2026-07-19..2026-07-25",
            "2026-07-26..2026-08-01",
            "2026-08-02..2026-08-08",
        ]
    );
}

#[test]
fn earnings_calendar_date_chunks_supports_thirty_five_day_grid_and_rejects_longer_than_forty_two()
{
    let chunks =
        earnings_calendar_date_chunks(&query_map("beginDate=2026-02-01&endDate=2026-03-07"))
            .expect("35-day grid");
    assert_eq!(chunks.len(), 5);
    assert_eq!(chunks[4].begin, "2026-03-01");
    assert_eq!(chunks[4].end, "2026-03-07");

    assert!(matches!(
        earnings_calendar_date_chunks(&query_map("beginDate=2026-01-01&endDate=2026-02-12")),
        Err(ResearchReadSnapshotError::Invalid(message)) if message.contains("42")
    ));
}

#[test]
fn deduplicate_earnings_calendar_entries_uses_date_and_security() {
    let entries = vec![
        item("2026-07-22", "US.AAPL", "Apple"),
        item("2026-07-22", "US.AAPL", "Apple duplicate"),
        item("2026-07-23", "US.AAPL", "Apple next day"),
    ];
    let result = deduplicate_earnings_calendar_entries(entries);
    assert_eq!(result.len(), 2);
    assert_eq!(result[0].name.as_deref(), Some("Apple"));
    assert_eq!(result[1].name.as_deref(), Some("Apple next day"));
}

#[test]
fn collect_earnings_calendar_chunks_fails_the_whole_range_when_one_chunk_fails() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(RecordingEarningsCalendarReader::new(Some(2)));
    runtime.set_earnings_calendar_reader(Some(reader.clone()));
    let error = read_futu_earnings_calendar(
        Some(&runtime),
        "market=US&operation=earnings&beginDate=2026-07-01&endDate=2026-07-21",
    )
    .expect_err("second segment fails");
    // The whole range fails with the provider error; partial rows from the
    // first successful chunk are never merged into a result.
    assert!(matches!(
        &error,
        ResearchReadSnapshotError::Failed { status: 502, code, message, .. }
            if code == "OPEND_EARNINGS_CALENDAR_FAILED"
                && message.contains("second OpenD segment failed")
    ));
    // The failing batch stops at the second call instead of continuing.
    assert_eq!(reader.calls().len(), 2);
}

#[test]
fn collect_earnings_calendar_chunks_uses_every_exact_segment_in_order() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(RecordingEarningsCalendarReader::new(None));
    runtime.set_earnings_calendar_reader(Some(reader.clone()));
    let value = read_futu_earnings_calendar(
        Some(&runtime),
        "market=US&operation=earnings&sort=market_cap&beginDate=2026-07-01&endDate=2026-07-14",
    )
    .expect("whole range");
    let calls = reader.calls();
    let segments = calls
        .iter()
        .map(|call| format!("{}..{}", call.begin_date, call.end_date))
        .collect::<Vec<_>>();
    assert_eq!(segments, vec!["2026-07-01..2026-07-07", "2026-07-08..2026-07-14"]);
    // The translated sort survives every chunk request.
    assert!(calls.iter().all(|call| call.sort_type == Some(2)));
    assert!(calls.iter().all(|call| call.market == 11));
    assert_eq!(value["metadata"]["rangeChunks"], 2);
    assert_eq!(value["metadata"]["beginDate"], "2026-07-01");
    assert_eq!(value["metadata"]["endDate"], "2026-07-14");
    assert_eq!(value["total"], 0);
    assert_eq!(value["entries"].as_array().map(Vec::len), Some(0));
}

#[test]
fn earnings_calendar_parameter_validation_edges() {
    let cases = [
        ("sort=unknown", "sort"),
        ("stockScope=unknown", "stockScope"),
        ("marketCapMin=many", "marketCapMin"),
        ("marketCapMax=inf", "marketCapMax"),
        ("marketCapMin=-1", "marketCapMin"),
        ("ivMax=101", "percentage"),
        ("marketCapMin=2&marketCapMax=1", "marketCapMin"),
    ];
    for (query, needle) in cases {
        let error =
            translate_earnings_calendar_params(&query_map(query), "US").expect_err(query);
        assert!(
            error.to_string().contains(needle),
            "query {query:?} error {error} must mention {needle:?}"
        );
    }

    // An explicitly empty parameter is the documented "not provided" form and
    // produces no filter instead of a stale or zero-valued one.
    let empty = translated("marketCapMin=&marketCapMax=", "US");
    assert!(empty.filters.is_empty());
    assert_eq!(empty.sort_type, None);
}

#[test]
fn earnings_calendar_date_validation_edges() {
    let default_chunks = earnings_calendar_date_chunks(&query_map("")).expect("default");
    assert_eq!(default_chunks.len(), 1);
    assert_eq!(default_chunks[0].begin, default_chunks[0].end);

    for query in [
        "endDate=2026-07-01",
        "beginDate=07/01/2026",
        "beginDate=2026-07-01&endDate=07/02/2026",
        "beginDate=2026-07-02&endDate=2026-07-01",
        // Multi-byte input must fail as invalid input instead of panicking.
        "beginDate=éééééééééé",
    ] {
        assert!(
            earnings_calendar_date_chunks(&query_map(query)).is_err(),
            "query {query:?} must be rejected"
        );
    }

    let chunks =
        earnings_calendar_date_chunks(&query_map("beginDate=2026-07-01&endDate=2026-07-08"))
            .expect("two chunks");
    assert_eq!(chunks.len(), 2);
    assert_eq!(chunks[1].end, "2026-07-08");
}

#[test]
fn deduplicate_earnings_calendar_entries_falls_back_for_anonymous_rows() {
    // Adapter rows always carry a non-empty code, but the shared dedup helper
    // still mirrors the Go fallback for rows without any identity.
    let anonymous = EarningsCalendarItem {
        security: EarningsCalendarSecurity {
            market: String::new(),
            code: String::new(),
            instrument_id: String::new(),
        },
        name: Some("anonymous".to_owned()),
        earnings_date: None,
        earnings_timestamp: None,
        pub_type: None,
        period_text: None,
        estimate_list: Vec::new(),
        option_volume: None,
        iv: None,
        iv_rank: None,
        iv_percentile: None,
        market_cap: None,
        price: None,
    };
    let mut other = anonymous.clone();
    other.name = Some("other".to_owned());
    let result =
        deduplicate_earnings_calendar_entries(vec![anonymous.clone(), anonymous, other]);
    assert_eq!(result.len(), 2);
    assert_eq!(result[0].name.as_deref(), Some("anonymous"));
    assert_eq!(result[1].name.as_deref(), Some("other"));
}

/// Parity: go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:11
/// TestValidateResearchCalendarQueryAcceptsSupportedBusinessParameters
///
/// The Rust owner splits the Go validator across
/// `translate_earnings_calendar_params` (sort/scope/range rules) and
/// `earnings_calendar_date_chunks` (window rules), so the accepted business
/// table must clear both halves: sort `iv_percentile` on a US listing, the
/// watchlist scope, every range key and a 39-day window inside the 42-day cap.
#[test]
fn earnings_calendar_accepts_the_go_business_parameter_table() {
    let map = query_map(
        "operation=earnings&sort=iv_percentile&stockScope=watchlist\
         &marketCapMin=1000000000&optionVolumeMax=20000&ivMin=10&ivMax=80\
         &ivRankMin=5&ivPercentileMax=100&beginDate=2026-07-01&endDate=2026-08-08",
    );
    let params =
        translate_earnings_calendar_params(&map, "US").expect("business parameters");
    assert_eq!(params.sort_type, Some(6));
    assert_eq!(params.filters.len(), 6);
    assert_eq!(params.filters[0].indicator_type, 4);
    assert_eq!(params.filters[0].value_list, vec![1]);

    let chunks = earnings_calendar_date_chunks(&map).expect("39-day window");
    assert_eq!(
        chunks.first().map(|chunk| chunk.begin.as_str()),
        Some("2026-07-01")
    );
    assert_eq!(
        chunks.last().map(|chunk| chunk.end.as_str()),
        Some("2026-08-08")
    );
}

/// Parity: go:452dea11:internal/productfeatures/earnings_calendar_query_test.go:35
/// TestValidateResearchCalendarQueryRejectsInvalidParameters
///
/// The Go table mixes sort, scope, window, range-shape and market-capability
/// rejections in one validator. Each row must be rejected by whichever Rust
/// owner handles it instead of being accepted by the route.
#[test]
fn earnings_calendar_rejects_the_go_invalid_parameter_table() {
    let cases = [
        ("US", "sort=unknown"),
        ("US", "stockScope=mine"),
        ("US", "endDate=2026-07-01"),
        ("US", "beginDate=2026/07/01"),
        ("US", "beginDate=2026-07-02&endDate=2026-07-01"),
        ("US", "beginDate=2026-07-01&endDate=2026-08-12"),
        ("US", "marketCapMin=-1"),
        ("US", "ivMin=not-a-number"),
        ("US", "ivMax=101"),
        ("US", "optionVolumeMin=20&optionVolumeMax=10"),
        ("SH", "sort=iv"),
        ("SZ", "ivRankMin=1"),
    ];
    for (market, query) in cases {
        let map = query_map(query);
        let rejected = translate_earnings_calendar_params(&map, market).is_err()
            || earnings_calendar_date_chunks(&map).is_err();
        assert!(
            rejected,
            "market {market} query {query:?} must be rejected like the Go validator"
        );
    }
}
