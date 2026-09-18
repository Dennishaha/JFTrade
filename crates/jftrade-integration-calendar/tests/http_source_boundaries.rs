//! Parity tests for Go's `internal/exchangecalendar/http_source_boundaries_test.go`.
//!
//! The Go file pins the failure taxonomy of the HTTP adapter and the
//! parser/validator boundaries. Rust has no nil receiver and no partially
//! constructed adapter — identity, URL, parse function and validator are all
//! constructor arguments — so the reachable failure modes are transport, parse
//! and validation, and those three stay distinguishable.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use jftrade_calendar::{
    CalendarCancellationToken, CalendarSourceError, CalendarSourcePort, TradingDaySchedule,
};
use jftrade_integration_calendar::{
    CalendarHttpClient, HttpCalendarSource, ParseFn, ValidateFn, default_holiday_override_parser,
    hong_kong_holiday_ical_parser, minimum_anchor_year_schedules_validator,
    nyse_holiday_schedule_parser, sse_trading_schedule_parser,
};
use jftrade_kernel::WireTimestamp;

/// Transport stub that can fail before a body exists at all.
struct StubClient {
    outcome: StubOutcome,
    calls: AtomicUsize,
}

enum StubOutcome {
    Body(&'static str),
    Transport(&'static str),
}

impl StubClient {
    fn ok(body: &'static str) -> Arc<Self> {
        Arc::new(Self {
            outcome: StubOutcome::Body(body),
            calls: AtomicUsize::new(0),
        })
    }

    fn transport(message: &'static str) -> Arc<Self> {
        Arc::new(Self {
            outcome: StubOutcome::Transport(message),
            calls: AtomicUsize::new(0),
        })
    }
}

impl CalendarHttpClient for StubClient {
    fn get(
        &self,
        _url: &str,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<Vec<u8>, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        match self.outcome {
            StubOutcome::Body(body) => Ok(body.as_bytes().to_vec()),
            StubOutcome::Transport(message) => Err(message.to_owned()),
        }
    }
}

fn timestamp(value: &str) -> WireTimestamp {
    value.parse().expect("valid fixture timestamp")
}

fn source(
    id: &str,
    markets: &[&str],
    client: Arc<dyn CalendarHttpClient>,
    parse: ParseFn,
) -> HttpCalendarSource {
    HttpCalendarSource::new(
        jftrade_calendar::CalendarSourceDescriptor {
            id: id.to_owned(),
            kind: "fixture".to_owned(),
            authority: "fixture".to_owned(),
            markets: markets.iter().map(|market| (*market).to_owned()).collect(),
        },
        "https://example.test/fixture",
        parse,
        time::Duration::days(1),
        client,
    )
}

fn fetch(
    source: &HttpCalendarSource,
    market: &str,
    from: &str,
    to: &str,
) -> Result<jftrade_calendar::CalendarSnapshot, CalendarSourceError> {
    source.fetch(
        market,
        timestamp(from),
        timestamp(to),
        &CalendarCancellationToken::default(),
    )
}

fn parse_error(
    _market: &str,
    _body: &[u8],
    _from: Option<WireTimestamp>,
    _to: Option<WireTimestamp>,
) -> Result<Vec<TradingDaySchedule>, CalendarSourceError> {
    Err(CalendarSourceError::Failed(
        "malformed authority document".to_owned(),
    ))
}

const US_FIXTURE: &str =
    "<table><tr><td>Juneteenth</td><td>June 19, 2026</td><td>Closed</td></tr></table>";

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:30
/// TestHTTPCalendarSourceFetchPreservesDistinctTransportAndParsingFailures.
///
/// Go checks six distinct failure identities: nil receiver, invalid URL,
/// transport error, body-read error, parse error and validator error, and each
/// must keep its own message (`errors.Is` for the wrapping cases, plus the
/// closed body). Rust's adapter has no nil receiver and builds its URL through
/// the typed client, so the reachable identities are transport/parse/validate;
/// the read-failure case is the transport seam reporting a body error.
#[test]
fn fetch_preserves_distinct_transport_parse_and_validation_failures() {
    let transport = source(
        "transport-failure",
        &["US"],
        StubClient::transport("calendar endpoint unreachable"),
        default_holiday_override_parser,
    );
    let error = fetch(
        &transport,
        "US",
        "2026-01-01T00:00:00Z",
        "2026-12-31T00:00:00Z",
    )
    .expect_err("a transport error is surfaced");
    assert!(
        error.to_string().contains("calendar endpoint unreachable"),
        "the transport message survives: {error}"
    );

    let read = source(
        "read-failure",
        &["US"],
        StubClient::transport("response body interrupted"),
        default_holiday_override_parser,
    );
    let error = fetch(&read, "US", "2026-01-01T00:00:00Z", "2026-12-31T00:00:00Z")
        .expect_err("a body read failure is surfaced");
    assert!(
        error.to_string().contains("interrupted"),
        "the body-read message survives: {error}"
    );

    let parse = source(
        "parse-failure",
        &["US"],
        StubClient::ok("fixture"),
        parse_error,
    );
    let error = fetch(&parse, "US", "2026-01-01T00:00:00Z", "2026-12-31T00:00:00Z")
        .expect_err("a parse error is surfaced");
    assert_eq!(
        error.to_string(),
        "malformed authority document",
        "the parser error is passed through unchanged"
    );

    let validate: ValidateFn = Arc::new(
        |_market: &str,
         _schedules: &[TradingDaySchedule],
         _from: Option<WireTimestamp>,
         _to: Option<WireTimestamp>| {
            Err(CalendarSourceError::Failed(
                "schedule failed authority validation".to_owned(),
            ))
        },
    );
    let validate_source = source(
        "validate-failure",
        &["US"],
        StubClient::ok(US_FIXTURE),
        default_holiday_override_parser,
    )
    .with_validate(validate);
    let error = fetch(
        &validate_source,
        "US",
        "2026-01-01T00:00:00Z",
        "2026-12-31T00:00:00Z",
    )
    .expect_err("a validator error is surfaced");
    assert_eq!(
        error.to_string(),
        "schedule failed authority validation",
        "the validator error is passed through unchanged"
    );

    // Cancellation is not a provider outage: the adapter keeps it recognisable.
    let cancelled = CalendarCancellationToken::default();
    cancelled.cancel();
    let cancelling = source(
        "cancelled",
        &["US"],
        StubClient::transport("calendar request was cancelled"),
        default_holiday_override_parser,
    );
    let error = cancelling
        .fetch(
            "US",
            timestamp("2026-01-01T00:00:00Z"),
            timestamp("2026-12-31T00:00:00Z"),
            &cancelled,
        )
        .expect_err("a cancelled request fails");
    assert!(
        matches!(error, CalendarSourceError::Cancelled),
        "cancellation stays a distinct error kind: {error:?}"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:89
/// TestCalendarParserHelpersHandleMalformedAndPartialAuthorityDocuments.
///
/// The helper level is exercised through the provider parsers that own it (the
/// helpers are crate-private, and Go's own test reaches them through the same
/// parser entry points for the observable cases): malformed iCal folding,
/// missing field separators, unknown markets, deduplication, non-date cells,
/// embedded years and out-of-window dates. Assertions that name a private Rust
/// helper directly are recorded in the module tests of `parser_helpers.rs`.
#[test]
fn parsers_tolerate_malformed_and_partial_authority_documents() {
    let from = Some(timestamp("2026-01-01T00:00:00Z"));
    let to = Some(timestamp("2026-12-31T00:00:00Z"));

    // A folded SUMMARY line must survive as one field, not two events.
    let folded = [
        "BEGIN:VCALENDAR",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20260102",
        "SUMMARY:National",
        " Day",
        "END:VEVENT",
        "END:VCALENDAR",
    ]
    .join("\r\n");
    let schedules = hong_kong_holiday_ical_parser("HK", folded.as_bytes(), from, to).expect("fold");
    assert_eq!(schedules.len(), 1, "schedules = {schedules:?}");
    assert_eq!(
        schedules[0].reason, "nationalday",
        "Go's unfoldICalLines strips only the fold whitespace: `SUMMARY:National` +          ` Day` becomes `NationalDay`, and the reason is normalised from that"
    );

    // Unknown market: Go's `resolveParseTemplate` returns ok=false and the
    // parser answers an empty document rather than inventing schedules.
    for schedules in [
        default_holiday_override_parser("MARS", b"Closed January 1, 2026", from, to)
            .expect("default parser"),
        nyse_holiday_schedule_parser(
            "MARS",
            br#"<table><tr><th>Holiday</th><th>2026</th></tr><tr><td>X</td><td>January 1</td></tr></table>"#,
            from,
            to,
        )
        .expect("NYSE parser"),
        hong_kong_holiday_ical_parser(
            "MARS",
            b"BEGIN:VEVENT\nDTSTART;VALUE=DATE:20260101\nEND:VEVENT",
            from,
            to,
        )
        .expect("HK parser"),
        sse_trading_schedule_parser("MARS", b"2026\nJanuary 1", from, to).expect("SSE parser"),
    ] {
        assert!(schedules.is_empty(), "unknown market stays empty");
    }

    // Deduplication: the same date twice is a single schedule.
    let duplicated =
        "<div>Juneteenth June 19, 2026 Closed</div><p>Juneteenth June 19, 2026 Closed</p>";
    let schedules =
        default_holiday_override_parser("US", duplicated.as_bytes(), from, to).expect("dedup");
    assert_eq!(schedules.len(), 1, "schedules = {schedules:?}");

    // A non-date cell is discarded, and an embedded year in prose is not a
    // standalone year header for the SSE parser.
    let sse = sse_trading_schedule_parser("CN", b"calendar 2026\nOctober 1 - October 8", from, to)
        .expect("SSE without a standalone year");
    assert!(
        sse.is_empty(),
        "only a standalone `## 2026` line opens a new year block: {sse:?}"
    );

    // Out-of-window dates are dropped rather than clamped.
    let outside = default_holiday_override_parser(
        "US",
        b"<div>Friday, July 3, 2026 - 1:00 p.m. early close</div>",
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2026-06-30T00:00:00Z")),
    )
    .expect("out-of-window");
    assert!(outside.is_empty(), "out-of-window rows are dropped");

    // An empty DTSTART is not a date, so the event never lands.
    let missing_date =
        hong_kong_holiday_ical_parser("HK", b"BEGIN:VEVENT\nSUMMARY:No date\nEND:VEVENT", from, to)
            .expect("missing DTSTART");
    assert!(
        missing_date.is_empty(),
        "an event without DTSTART is dropped"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:156
/// TestCalendarAuthorityValidatorHandlesMissingAnchorsAndSparseYears.
///
/// Four validator cases: a zero threshold disables the check, a missing anchor
/// year is an error, a sparse year is an error, and the first parsed schedule
/// supplies the anchor when the window has no usable year. Go's validator is a
/// closure over the threshold, so `2` and `1` are real cases.
#[test]
fn authority_validator_handles_missing_anchors_and_sparse_years() {
    let disabled = minimum_anchor_year_schedules_validator(0);
    assert!(
        disabled("US", &[], None, None).is_ok(),
        "a zero threshold disables the check"
    );

    let sparse = minimum_anchor_year_schedules_validator(2);
    let error = sparse("US", &[], None, None).expect_err("no anchor year is available");
    assert!(
        error.to_string().contains("no anchor year"),
        "the missing anchor is named: {error}"
    );

    let one = schedule("US", "2026-01-01T00:00:00-05:00");
    let error = sparse("US", std::slice::from_ref(&one), None, None)
        .expect_err("one schedule is below the threshold");
    assert!(
        error.to_string().contains("too few"),
        "the sparse year is named: {error}"
    );
    assert!(
        error.to_string().contains("want at least 2"),
        "the requested threshold is carried by the validator: {error}"
    );

    let single = minimum_anchor_year_schedules_validator(1);
    assert!(
        single(
            "US",
            std::slice::from_ref(&one),
            Some(timestamp("2026-01-01T00:00:00-05:00")),
            None,
        )
        .is_ok(),
        "one matching schedule satisfies a threshold of one"
    );
    assert!(
        single("US", std::slice::from_ref(&one), None, None).is_ok(),
        "the first parsed schedule supplies the anchor when the window has none"
    );

    // The anchor comes from the requested window, so a feed that answers with
    // a later year is measured against the requested year's coverage.
    let later = schedule("US", "2027-06-19T00:00:00-04:00");
    let error = sparse(
        "US",
        std::slice::from_ref(&later),
        Some(timestamp("2026-01-01T00:00:00-05:00")),
        None,
    )
    .expect_err("the requested year has no coverage");
    assert!(
        error.to_string().contains("2026"),
        "the anchor year is the requested one: {error}"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:179
/// TestCalendarParsersDiscardIncompleteOrOutOfRangeAuthorityRows.
///
/// Every malformed row shape must be discarded: an NYSE row with too few cells,
/// a blank holiday name, an impossible civil date, and a date outside the
/// window; an iCal event without a date; an SSE body that reaches the
/// `Trading hours` marker before the holiday block.
#[test]
fn parsers_discard_incomplete_or_out_of_range_authority_rows() {
    let from = Some(timestamp("2026-01-01T00:00:00Z"));
    let to = Some(timestamp("2026-12-31T00:00:00Z"));

    let nyse_body = br#"
        <table>
            <tr><th>Holiday</th><th>2026</th></tr>
            <tr><td>Incomplete row</td></tr>
            <tr><td></td><td>June 19</td></tr>
            <tr><td>Invalid date</td><td>February 30</td></tr>
            <tr><td>Outside range</td><td>July 3</td></tr>
        </table>"#;
    let schedules = nyse_holiday_schedule_parser(
        "US",
        nyse_body,
        from,
        Some(timestamp("2026-06-30T00:00:00Z")),
    )
    .expect("NYSE malformed rows");
    assert!(
        schedules.is_empty(),
        "every malformed NYSE row is discarded: {schedules:?}"
    );

    let incomplete =
        hong_kong_holiday_ical_parser("HK", b"BEGIN:VEVENT\nSUMMARY:No date\nEND:VEVENT", from, to)
            .expect("HK incomplete event");
    assert!(incomplete.is_empty(), "an event without DTSTART is dropped");

    let sse_body = [
        "notice before year",
        "## 2026",
        "plain text without a month",
        "Reverse span December 31, 2026 - January 2, 2026",
        "Trading hours",
        "National Day October 1 - October 8",
    ]
    .join("\n");
    let schedules =
        sse_trading_schedule_parser("CN", sse_body.as_bytes(), from, to).expect("SSE stop marker");
    assert!(
        schedules.is_empty(),
        "the `Trading hours` marker stops the parser before the holiday block: {schedules:?}"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:278
/// TestCalendarParsersHonorNarrowFetchWindowsAndDiscardImpossibleDates.
///
/// A narrow window clips the expanded SSE range to the requested days, and an
/// impossible civil date is rejected both as a single date and as either end
/// of a span.
#[test]
fn parsers_honor_narrow_fetch_windows_and_discard_impossible_dates() {
    let schedules = sse_trading_schedule_parser(
        "CN",
        b"## 2026\nNational Day October 1 - October 8",
        Some(timestamp("2026-10-03T00:00:00+08:00")),
        Some(timestamp("2026-10-04T00:00:00+08:00")),
    )
    .expect("narrow window");
    let days = schedules
        .iter()
        .map(|schedule| schedule.date.into_inner().date().to_string())
        .collect::<Vec<_>>();
    assert_eq!(
        days,
        ["2026-10-03", "2026-10-04"],
        "only the requested days inside the closed range are materialised"
    );

    let from = Some(timestamp("2026-01-01T00:00:00Z"));
    let to = Some(timestamp("2026-12-31T00:00:00Z"));
    let impossible_cell = nyse_holiday_schedule_parser(
        "US",
        br#"<table><tr><th>Holiday</th><th>2026</th></tr><tr><td>Impossible</td><td>February 30</td></tr></table>"#,
        from,
        to,
    )
    .expect("impossible cell");
    assert!(
        impossible_cell.is_empty(),
        "February 30 is not a real NYSE date: {impossible_cell:?}"
    );

    for body in ["Holiday February 30", "Holiday January 1 - February 30"] {
        let schedules =
            sse_trading_schedule_parser("CN", format!("## 2026\n{body}").as_bytes(), from, to)
                .expect("impossible SSE span");
        assert!(
            schedules.is_empty(),
            "an impossible span end must not expand: {body:?} => {schedules:?}"
        );
    }
}

fn schedule(market: &str, date: &str) -> TradingDaySchedule {
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: timestamp(date),
        status: "closed".to_owned(),
        sessions: Vec::new(),
        reason: String::new(),
        source_id: String::new(),
        observed: false,
        updated_at: None,
    }
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:119
/// TestHTTPCalendarSourceValidateSnapshotBoundary.
///
/// Go's adapter-level `ValidateSnapshot` returns nil for a nil source and for a
/// source without a validator, and otherwise calls the validator with the exact
/// `market`/`schedules`/`from`/`to` it was given, propagating the error. Rust's
/// `HttpCalendarSource` carries `validate: Option<ValidateFn>` with the same
/// semantics, so this pins the pass-through shape of the validator contract: no
/// validator means the fetched snapshot is accepted, and a validator sees one
/// schedule in a non-empty window for the requested market.
#[test]
fn adapter_validate_snapshot_boundary_passes_market_schedules_and_window_through() {
    // No validator installed: the fetch is accepted, mirroring Go's
    // `(&HTTPCalendarSource{}).ValidateSnapshot(...) == nil`.
    let unvalidated = source(
        "no-validator",
        &["US"],
        StubClient::ok(US_FIXTURE),
        default_holiday_override_parser,
    );
    let snapshot = fetch(
        &unvalidated,
        "US",
        "2026-01-01T00:00:00Z",
        "2026-12-31T23:59:59Z",
    )
    .expect("a source without a validator accepts its parse");
    assert_eq!(snapshot.schedules.len(), 1);

    // A validator installed: it receives the market, the parsed schedules and
    // the window, and its error is the fetch error.
    let seen: Arc<std::sync::Mutex<Vec<String>>> = Arc::new(std::sync::Mutex::new(Vec::new()));
    let recorder = Arc::clone(&seen);
    let validate: ValidateFn = Arc::new(
        move |market: &str,
              schedules: &[TradingDaySchedule],
              from: Option<WireTimestamp>,
              to: Option<WireTimestamp>| {
            recorder.lock().expect("validator recording").push(format!(
                "{market}|{}|{}|{}",
                schedules.len(),
                from.map(|value| value.to_string()).unwrap_or_default(),
                to.map(|value| value.to_string()).unwrap_or_default()
            ));
            Err(CalendarSourceError::Failed(
                "not enough official holidays".to_owned(),
            ))
        },
    );
    let validated = source(
        "validated",
        &["US"],
        StubClient::ok(US_FIXTURE),
        default_holiday_override_parser,
    )
    .with_validate(validate);
    let error = fetch(
        &validated,
        "US",
        "2026-01-01T00:00:00Z",
        "2026-12-31T23:59:59Z",
    )
    .expect_err("the validator error propagates");
    assert_eq!(error.to_string(), "not enough official holidays");
    assert_eq!(
        seen.lock().expect("validator recording").as_slice(),
        ["US|1|2026-01-01T00:00:00Z|2026-12-31T23:59:59Z"],
        "the validator sees the requested market, the parsed schedule count and both bounds"
    );
}
