//! Parity tests for Go's `internal/exchangecalendar/http_source_test.go`.
//!
//! Every test name records the Go case it answers. Transport is stubbed through
//! [`CalendarHttpClient`], so no test touches the network.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use jftrade_calendar::{
    CalendarCancellationToken, CalendarSourceError, CalendarSourcePort, CalendarSourceRegistry,
};
use jftrade_integration_calendar::{
    CalendarHttpClient, HttpCalendarSource, default_holiday_override_parser, default_registry,
    hong_kong_holiday_ical_parser, minimum_anchor_year_schedules_validator,
    nyse_holiday_schedule_parser, sse_trading_schedule_parser,
};
use jftrade_kernel::WireTimestamp;

/// A transport that replays one canned response and records every request.
struct StubClient {
    status_ok: bool,
    body: Vec<u8>,
    calls: AtomicUsize,
}

impl StubClient {
    fn ok(body: &str) -> Arc<Self> {
        Arc::new(Self {
            status_ok: true,
            body: body.as_bytes().to_vec(),
            calls: AtomicUsize::new(0),
        })
    }

    fn failing(body: &str) -> Arc<Self> {
        Arc::new(Self {
            status_ok: false,
            body: body.as_bytes().to_vec(),
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
        if self.status_ok {
            Ok(self.body.clone())
        } else {
            Err("request returned status 502".to_owned())
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
    parse: jftrade_integration_calendar::ParseFn,
    valid_for_days: i64,
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
        time::Duration::days(valid_for_days),
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

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:21
/// TestDefaultRegistryRegistersExpectedSources.
#[test]
fn default_registry_registers_the_four_official_providers_with_their_markets() {
    let registry: CalendarSourceRegistry = default_registry(StubClient::ok("")).expect("registry");
    let by_id = registry
        .descriptors()
        .into_iter()
        .map(|descriptor| (descriptor.id, descriptor.markets))
        .collect::<std::collections::BTreeMap<_, _>>();
    assert_eq!(by_id["nyse_official"], ["US"]);
    assert_eq!(by_id["nasdaq_verifier"], ["US"]);
    assert_eq!(by_id["hk_gov_1823_ical"], ["HK"]);
    assert_eq!(by_id["mainland_official_notice"], ["CN", "SH", "SZ"]);
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:40
/// TestDefaultRegistryUsesCalendarFetchTimeout.
#[test]
fn default_registry_client_uses_the_calendar_fetch_timeout() {
    assert_eq!(
        jftrade_integration_calendar::DEFAULT_HTTP_TIMEOUT,
        std::time::Duration::from_secs(15),
        "Go's defaultHTTPTimeout is 15s"
    );
    // The production client must build with that timeout so a hung provider
    // cannot pin a calendar worker indefinitely.
    jftrade_integration_calendar::ReqwestCalendarClient::with_default_timeout()
        .expect("default client builds");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:55
/// TestHTTPCalendarSourceFetchBuildsSnapshotMetadata.
#[test]
fn fetch_builds_snapshot_metadata_with_checksum_and_validity() {
    let client = StubClient::ok(
        r#"<table><tr><td>Juneteenth</td><td>June 19, 2026</td><td>Closed</td></tr></table>"#,
    );
    let source = source(
        "nyse_official",
        &["US"],
        client,
        default_holiday_override_parser,
        1,
    );
    let snapshot = fetch(
        &source,
        "US",
        "2026-01-01T00:00:00Z",
        "2026-12-31T00:00:00Z",
    )
    .expect("fetch");

    assert_eq!(snapshot.source_id, "nyse_official");
    assert_eq!(snapshot.market_code, "US");
    assert_eq!(snapshot.schedules.len(), 1);
    assert_eq!(snapshot.schedules[0].status, "closed");
    assert!(!snapshot.checksum.is_empty(), "checksum is always present");
    assert!(
        snapshot.fetched_at.into_inner().year() > 2000,
        "fetchedAt is stamped"
    );
    let fetched = snapshot.fetched_at.into_inner();
    let until = snapshot.valid_until.into_inner();
    assert!(
        until > fetched,
        "validUntil is derived from fetchedAt + validFor"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:88
/// TestHTTPCalendarSourceFetchReturnsStatusErrors.
#[test]
fn fetch_surfaces_a_provider_status_error() {
    let client = StubClient::failing("bad gateway");
    let source = source(
        "hk_gov_1823_ical",
        &["HK"],
        client,
        hong_kong_holiday_ical_parser,
        1,
    );
    let error = fetch(
        &source,
        "HK",
        "0001-01-01T00:00:00Z",
        "0001-01-01T00:00:00Z",
    )
    .expect_err("a non-2xx response is an error");
    assert!(
        error.to_string().contains("502"),
        "the status is preserved: {error}"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:109
/// TestHTTPCalendarSourceFetchRejectsSparseAnnualSchedules.
#[test]
fn fetch_rejects_a_sparse_annual_schedule_via_the_anchor_validator() {
    let client = StubClient::ok(
        "<div>Holiday Notice: Securities Market will be closed on 19 June 2026.</div>",
    );
    let source = source(
        "hk_gov_1823_ical",
        &["HK"],
        client,
        default_holiday_override_parser,
        1,
    )
    .with_validate(minimum_anchor_year_schedules_validator(8));
    let error = fetch(
        &source,
        "HK",
        "2026-01-01T00:00:00Z",
        "2027-12-31T23:59:59Z",
    )
    .expect_err("one row cannot satisfy the eight-per-year floor");
    assert!(
        error.to_string().contains("too few anchor-year schedules"),
        "the structure change is named: {error}"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:133
/// TestAnchorYearSchedulesValidatorAllowsMissingFutureYearCoverage.
#[test]
fn anchor_year_validator_allows_missing_future_year_coverage() {
    let body = [
        "BEGIN:VCALENDAR",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270101",
        "SUMMARY:The first day of January",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270405",
        "SUMMARY:Ching Ming Festival",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270406",
        "SUMMARY:The day following Ching Ming Festival",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270501",
        "SUMMARY:Labour Day",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270519",
        "SUMMARY:Buddha's Birthday",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270614",
        "SUMMARY:Tuen Ng Festival",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20270701",
        "SUMMARY:HKSAR Establishment Day",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20271001",
        "SUMMARY:National Day",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20271225",
        "SUMMARY:Christmas Day",
        "END:VEVENT",
        "END:VCALENDAR",
    ]
    .join("\r\n");
    let client = StubClient::ok(&body);
    let source = source(
        "hk_gov_1823_ical",
        &["HK"],
        client,
        hong_kong_holiday_ical_parser,
        1,
    )
    .with_validate(minimum_anchor_year_schedules_validator(8));
    // The requested window spans two years but the feed only covers 2027.
    let snapshot = fetch(
        &source,
        "HK",
        "2027-01-01T00:00:00Z",
        "2028-12-31T23:59:59Z",
    )
    .expect("2027 coverage alone satisfies the anchor year");
    assert_eq!(snapshot.schedules.len(), 9);
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:201
/// TestDefaultHolidayOverrideParserUSParsesTableRows.
#[test]
fn us_holiday_override_parser_reads_closed_and_early_close_rows() {
    let body = r#"
        <table>
            <tr><td>Juneteenth National Independence Day</td><td>June 19, 2026</td><td>Closed</td></tr>
            <tr><td>Black Friday</td><td>November 27, 2026</td><td>1:00 p.m. early close</td></tr>
        </table>
    "#;
    let schedules = default_holiday_override_parser(
        "US",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2026-12-31T00:00:00Z")),
    )
    .expect("parse");
    assert_eq!(schedules.len(), 2, "rows = {schedules:?}");
    assert_eq!(schedules[0].status, "closed");
    assert_eq!(schedules[1].status, "early_close");
    // The early close keeps the bundled session windows, not an empty list.
    assert!(
        !schedules[1].sessions.is_empty(),
        "the builtin early-close windows are preserved"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:224
/// TestNYSEHolidayScheduleParserParsesMultiYearTableAndFootnotes.
#[test]
fn nyse_parser_reads_multi_year_table_and_footnote_early_closes() {
    let body = r#"
        <table>
            <tr>
                <th>Holiday</th>
                <th>2026</th>
                <th>2027</th>
                <th>2028</th>
            </tr>
            <tr>
                <td>Juneteenth National Independence Day</td>
                <td>Friday, June 19</td>
                <td>Friday, June 18 (Juneteenth National Independence Day observed)</td>
                <td>Monday, June 19</td>
            </tr>
            <tr>
                <td>Independence Day</td>
                <td>Friday, July 3 (Independence Day observed)</td>
                <td>Monday, July 5 (Independence Day observed)</td>
                <td>Tuesday, July 4**</td>
            </tr>
        </table>
        <div>*** Each market will close early at 1:00 p.m. (1:15 p.m. for eligible options) on Friday, November 27, 2026, Friday, November 26, 2027, and Friday, November 24, 2028 (the day after Thanksgiving).</div>
    "#;
    let schedules = nyse_holiday_schedule_parser(
        "US",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2027-12-31T23:59:59Z")),
    )
    .expect("parse");
    let by_day = schedules
        .iter()
        .map(|schedule| {
            (
                schedule.date.into_inner().date().to_string(),
                schedule.status.clone(),
            )
        })
        .collect::<std::collections::BTreeMap<_, _>>();

    assert_eq!(by_day["2026-06-19"], "closed");
    assert_eq!(by_day["2026-07-03"], "closed");
    assert_eq!(by_day["2026-11-27"], "early_close");
    assert_eq!(by_day["2027-06-18"], "closed");
    assert_eq!(by_day["2027-07-05"], "closed");
    assert_eq!(by_day["2027-11-26"], "early_close");
    // 2028 rows are outside the requested window and must not leak in.
    assert!(
        !by_day.contains_key("2028-07-03"),
        "out-of-range rows are discarded: {by_day:?}"
    );
    // The "observed" marker is preserved from the table cell.
    assert!(
        schedules
            .iter()
            .find(|schedule| schedule.date.into_inner().date().to_string() == "2027-06-18")
            .is_some_and(|schedule| schedule.observed),
        "observed holidays keep their marker"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:276
/// TestDefaultHolidayOverrideParserHKParsesEnglishList.
#[test]
fn hk_holiday_override_parser_reads_english_list_items() {
    let body = r#"<ul><li>National Day - 1 October 2026 - Closed</li></ul>"#;
    let schedules = default_holiday_override_parser(
        "HK",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2026-12-31T00:00:00Z")),
    )
    .expect("parse");
    assert_eq!(schedules.len(), 1, "schedules = {schedules:?}");
    assert_eq!(schedules[0].status, "closed");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:288
/// TestHongKongHolidayICalParserParsesClosedDays.
#[test]
fn hk_ical_parser_reads_closed_days_from_events() {
    let body = [
        "BEGIN:VCALENDAR",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20260101",
        "SUMMARY:The first day of January",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "DTSTART;VALUE=DATE:20271001",
        "SUMMARY:National Day",
        "END:VEVENT",
        "END:VCALENDAR",
    ]
    .join("\r\n");
    let schedules = hong_kong_holiday_ical_parser(
        "HK",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2027-12-31T23:59:59Z")),
    )
    .expect("parse");
    let by_day = schedules
        .iter()
        .map(|schedule| {
            (
                schedule.date.into_inner().date().to_string(),
                schedule.status.clone(),
            )
        })
        .collect::<std::collections::BTreeMap<_, _>>();
    assert_eq!(by_day["2026-01-01"], "closed");
    assert_eq!(by_day["2027-10-01"], "closed");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:320
/// TestSSETradingScheduleParserExpandsRangesAndSkipsMakeupDays.
#[test]
fn sse_parser_expands_closed_ranges_and_skips_makeup_days() {
    let body = [
        "## 2026",
        "Chinese New Year January 28 (Wednesday) - February 4 (Wednesday), plus January 25 (Sunday) and February 7 (Saturday)",
        "National Day October 1 (Thursday) - October 8 (Thursday), plus September 27 (Sunday) and October 10 (Saturday)",
        "Trading hours",
    ]
    .join("\n");
    let schedules = sse_trading_schedule_parser(
        "CN",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2026-12-31T23:59:59Z")),
    )
    .expect("parse");
    let days = schedules
        .iter()
        .map(|schedule| schedule.date.into_inner().date().to_string())
        .collect::<std::collections::BTreeSet<_>>();

    assert!(days.contains("2026-01-28") && days.contains("2026-02-04"));
    assert!(days.contains("2026-10-01") && days.contains("2026-10-08"));
    // "plus ..." makeup days are working days and must stay out of the closure.
    assert!(!days.contains("2026-01-25"), "makeup day excluded");
    assert!(!days.contains("2026-10-10"), "makeup day excluded");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:351
/// TestSSETradingScheduleParserInfersCrossYearRange.
#[test]
fn sse_parser_infers_a_cross_year_range() {
    let body = [
        "## 2026",
        "New Year holiday December 31 - January 2",
        "Trading hours",
    ]
    .join("\n");
    let schedules = sse_trading_schedule_parser(
        "CN",
        body.as_bytes(),
        Some(timestamp("2026-12-01T00:00:00Z")),
        Some(timestamp("2027-01-31T23:59:59Z")),
    )
    .expect("parse");
    let days = schedules
        .iter()
        .map(|schedule| schedule.date.into_inner().date().to_string())
        .collect::<Vec<_>>();
    assert_eq!(days, ["2026-12-31", "2027-01-01", "2027-01-02"]);
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:378
/// TestDefaultHolidayOverrideParserCNParsesChineseDateLine.
#[test]
fn cn_holiday_override_parser_reads_a_chinese_date_line() {
    let body = "<div>国庆节休市安排：2026年10月1日 休市</div>";
    let schedules = default_holiday_override_parser(
        "CN",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2026-12-31T00:00:00Z")),
    )
    .expect("parse");
    assert_eq!(schedules.len(), 1, "schedules = {schedules:?}");
    assert_eq!(schedules[0].status, "closed");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_test.go:390
/// TestDefaultHolidayOverrideParserRejectsOutOfRangeDates.
#[test]
fn holiday_override_parser_drops_dates_outside_the_fetch_window() {
    let body = "<div>Friday, July 3, 2028 - 1:00 p.m. early close</div>";
    let schedules = default_holiday_override_parser(
        "US",
        body.as_bytes(),
        Some(timestamp("2026-01-01T00:00:00Z")),
        Some(timestamp("2027-12-31T23:59:59Z")),
    )
    .expect("parse");
    assert!(
        schedules.is_empty(),
        "out-of-range dates are discarded: {schedules:?}"
    );
}
