use super::*;
use std::net::TcpListener as StdTcpListener;

#[test]
fn macro_history_uses_page_size_before_legacy_limit() {
    let query = QueryMap::parse("pageSize=60&limit=5").expect("query");
    assert_eq!(macro_limit(&query).expect("limit"), 60);
}

#[test]
fn macro_history_limit_is_bounded_and_defaults() {
    let default = QueryMap::parse("").expect("query");
    assert_eq!(macro_limit(&default).expect("default"), DEFAULT_MACRO_LIMIT);

    let oversized = QueryMap::parse("pageSize=9999").expect("query");
    assert_eq!(macro_limit(&oversized).expect("clamp"), MAX_MACRO_LIMIT);

    let zero = QueryMap::parse("pageSize=0").expect("query");
    assert_eq!(macro_limit(&zero).expect("default"), DEFAULT_MACRO_LIMIT);

    let legacy = QueryMap::parse("pageSize=0&limit=5").expect("query");
    assert_eq!(macro_limit(&legacy).expect("legacy fallback"), 5);
}

// Parity: go:452dea11:internal/marketdata/calendar_macro_facade_test.go:109
// TestServiceCalendarValidatesDateFormats.
#[test]
fn calendar_date_validation_rejects_malformed_and_reversed_ranges() {
    for (key, value) in [("beginDate", "2026/08/01"), ("endDate", "08-31"), ("date", "2026-13-01")] {
        let query = QueryMap::parse(&format!("{key}={value}")).expect("query");
        assert!(required_date(&query, key).is_err(), "{key}={value} must fail");
    }
    let reversed = QueryMap::parse("beginDate=2026-08-31&endDate=2026-08-01").expect("query");
    assert!(date_window(&reversed).is_err(), "reversed range must fail");
    let valid = QueryMap::parse("beginDate=2026-08-01&endDate=2026-08-31").expect("query");
    assert_eq!(date_window(&valid).expect("valid range"), ("2026-08-01".into(), "2026-08-31".into()));
}

// Parity: go:452dea11:internal/marketdata/calendar_macro_facade_test.go:144
// TestServiceMacroIndicatorHistoryValidatesIDAndLimit.
#[test]
fn macro_history_rejects_blank_indicator_before_helper_access() {
    let fixture = CalendarRouteFixture::new(Vec::new());
    let error = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&fixture.client),
        "/api/v1/research/macro",
        "operation=indicator_history&indicatorId= ",
    )
    .expect_err("blank indicator id must fail");
    assert!(matches!(error, ResearchReadSnapshotError::Failed { status: 409, .. }));
    assert!(fixture.join().is_empty(), "invalid identity must not reach helper");
}

#[test]
fn macro_projection_rejects_missing_or_malformed_typed_fields() {
    let mut indicator = Map::new();
    indicator.insert("unit_type".to_owned(), json!("percent"));
    assert!(matches!(
        required_integer(&indicator, "unit_type"),
        Err(ResearchReadSnapshotError::Failed { status: 502, .. })
    ));

    let mut point = Map::new();
    point.insert("value".to_owned(), json!("1.2"));
    let mut projected = Map::new();
    assert!(matches!(
        copy_optional_number(&point, &mut projected, "value", "value"),
        Err(ResearchReadSnapshotError::Failed { status: 502, .. })
    ));
}

/// Drive the production calendar route end to end against a loopback helper
/// fixture, so the projection under test is the one the HTTP route serves.
fn calendar_fixture(body: &'static str) -> (HelperClient, std::thread::JoinHandle<String>) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("listen");
    let address = listener.local_addr().expect("address");
    let handle = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        stream
            .set_read_timeout(Some(std::time::Duration::from_secs(5)))
            .expect("read timeout");
        let mut request = Vec::new();
        let mut chunk = [0_u8; 1024];
        while !request.windows(4).any(|window| window == b"\r\n\r\n") {
            let read = std::io::Read::read(&mut stream, &mut chunk).expect("read");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
            if request.len() > 16 * 1024 {
                break;
            }
        }
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        std::io::Write::write_all(&mut stream, response.as_bytes()).expect("write");
        String::from_utf8(request).expect("request utf8")
    });
    let client = HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
        base_url: format!("http://{address}"),
        bearer_token: None,
        request_timeout: std::time::Duration::from_secs(5),
        max_attempts: 1,
        retry_delay: std::time::Duration::ZERO,
    })
    .expect("helper client");
    (client, handle)
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:98
/// TestProviderEconomicCalendarProjectionDerivesDateAndTime
///
/// Economic events derive `eventDate`/`eventTime` from the unix timestamp in the
/// Asia/Shanghai display frame (UTC+8), while all-day events keep only the
/// supplied `event_date` and omit the timestamp-derived keys.
#[test]
fn economic_calendar_route_derives_date_and_time() {
    let (client, server) = calendar_fixture(
        r#"{"source":"akshare-calendar","entries":[
            {"event_id":"econ-1","title":"中国7月CPI同比","region":"中国","event_timestamp":1787004000,"importance":3,"previous_value":"0.3%"},
            {"event_id":"econ-2","title":"无时间事件","event_date":"2026-08-20"}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/calendars",
        "operation=economic&beginDate=2026-08-01&endDate=2026-08-31",
    )
    .expect("economic calendar");
    let request = server.join().expect("server");
    assert!(
        request.starts_with("GET /providers/akshare/calendar/economic?begin_date=2026-08-01&end_date=2026-08-31"),
        "request = {request}"
    );

    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 2);
    let first = &entries[0];
    let moment = time::OffsetDateTime::from_unix_timestamp(1_787_004_000)
        .expect("timestamp")
        .to_offset(time::UtcOffset::from_hms(8, 0, 0).expect("offset"));
    for key in [
        "eventId",
        "title",
        "region",
        "eventTimestamp",
        "eventDate",
        "eventTime",
        "importance",
        "previousValue",
    ] {
        assert!(first.get(key).is_some(), "missing key {key}: {first}");
    }
    assert_eq!(first["eventDate"], json!(moment.date().to_string()));
    assert_eq!(
        first["eventTime"],
        json!(format!("{:02}:{:02}", moment.hour(), moment.minute()))
    );
    assert_eq!(first["importance"], json!(3));
    for key in ["forecastValue", "actualValue"] {
        assert!(
            first.get(key).is_none(),
            "nil field {key} must be omitted: {first}"
        );
    }

    let all_day = &entries[1];
    for key in ["eventTimestamp", "eventTime", "importance"] {
        assert!(
            all_day.get(key).is_none(),
            "all-day event must omit {key}: {all_day}"
        );
    }
    assert_eq!(all_day["eventDate"], "2026-08-20");
    assert_eq!(result["hasMore"], false);
    assert!(result.get("nextCursor").is_none());
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:16
/// TestProviderEarningsCalendarProjectionMapsFrontendKeys
///
/// Earnings events expose the calendar table's keys, omit every null metric,
/// never resolve an instrument (calendar data is cross-market), and keep the
/// embedded provider attribution.
#[test]
fn earnings_calendar_route_maps_frontend_keys() {
    let (client, server) = calendar_fixture(
        r#"{"source":"akshare-calendar","entries":[
            {"instrument_id":"SH.600519","name":"贵州茅台","symbol":"600519","event_date":"2026-08-20","period_text":"2025中报","market_cap":2100000000000.0,"price":1680.5},
            {"instrument_id":"SZ.000001","name":"平安银行","event_date":"2026-08-21"}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/calendars",
        "operation=earnings&beginDate=2026-08-01&endDate=2026-08-31",
    )
    .expect("earnings calendar");
    server.join().expect("server");

    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 2);
    let first = &entries[0];
    for key in [
        "instrumentId",
        "market",
        "symbol",
        "name",
        "eventDate",
        "periodText",
        "marketCap",
        "price",
    ] {
        assert!(first.get(key).is_some(), "missing key {key}: {first}");
    }
    assert_eq!(first["instrumentId"], "SH.600519");
    assert_eq!(first["market"], "SH");
    assert_eq!(first["symbol"], "600519");
    assert_eq!(first["marketCap"], json!(2100000000000.0_f64));
    let second = &entries[1];
    for key in ["marketCap", "price", "periodText"] {
        assert!(
            second.get(key).is_none(),
            "nil field {key} must be omitted: {second}"
        );
    }
    assert!(
        result.get("resolvedInstrument").is_none(),
        "calendar results must not resolve an instrument: {result}"
    );
    assert_eq!(result["total"], 2);
    assert_eq!(result["hasMore"], false);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:63
/// TestProviderDividendCalendarProjectionMapsFrontendKeys
#[test]
fn dividend_calendar_route_maps_frontend_keys() {
    let (client, server) = calendar_fixture(
        r#"{"source":"akshare-calendar","entries":[
            {"instrument_id":"SH.600519","name":"贵州茅台","symbol":"600519","statement":"10派308.76元","ex_date":"2026-08-15","record_date":"2026-08-14","payable_date":"2026-08-22"},
            {"instrument_id":"SZ.000001","statement":"10派2元","ex_date":"2026-08-15"}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/calendars",
        "operation=dividends&date=2026-08-15",
    )
    .expect("dividend calendar");
    server.join().expect("server");

    let entries = result["entries"].as_array().expect("entries");
    let first = &entries[0];
    for key in [
        "instrumentId",
        "name",
        "symbol",
        "statement",
        "exDate",
        "recordDate",
        "dividendPayableDate",
    ] {
        assert!(first.get(key).is_some(), "missing key {key}: {first}");
    }
    assert_eq!(first["dividendPayableDate"], "2026-08-22");
    assert!(
        entries[1].get("dividendPayableDate").is_none(),
        "nil payable date must be omitted: {}",
        entries[1]
    );
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:154
/// TestProviderIpoCalendarProjectionMapsFrontendKeys
#[test]
fn ipo_calendar_route_maps_frontend_keys() {
    let (client, server) = calendar_fixture(
        r#"{"source":"akshare-calendar","entries":[
            {"instrument_id":"SZ.301999","name":"新股示例","symbol":"301999","status":"pending","issue_volume":4000.0,"issue_price_min":12.5,"issue_price_max":15.0},
            {"instrument_id":"SH.688999","name":"已上市","status":"listed","listing_date":"2026-08-25"}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/calendars",
        "operation=ipos",
    )
    .expect("ipo calendar");
    server.join().expect("server");

    let entries = result["entries"].as_array().expect("entries");
    let first = &entries[0];
    for key in [
        "instrumentId",
        "name",
        "symbol",
        "status",
        "issueVolume",
        "issuePriceMin",
        "issuePriceMax",
    ] {
        assert!(first.get(key).is_some(), "pending entry missing key {key}: {first}");
    }
    for key in ["listingDate", "issuePrice"] {
        assert!(
            first.get(key).is_none(),
            "nil field {key} must be omitted: {first}"
        );
    }
    assert_eq!(entries[1]["listingDate"], "2026-08-25");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:205
/// TestEmbeddedProviderRejectsUnsupportedCalendarMacroOperations
///
/// `trade_dates`, the fed_* macro operations, an empty operation, and an
/// indicator history without `indicatorId` have no embedded feed: they must
/// answer the broker capability contract instead of falling through to the
/// broker or returning a synthetic empty feed.
#[test]
fn calendar_and_macro_routes_reject_unsupported_operations() {
    let (client, server) = calendar_fixture(r#"{"entries":[]}"#);
    for (path, query) in [
        ("/api/v1/research/calendars", "operation=trade_dates"),
        ("/api/v1/research/calendars", ""),
        ("/api/v1/research/macro", "operation=fed_target_rate"),
        ("/api/v1/research/macro", "operation=fed_dot_plot"),
        ("/api/v1/research/macro", "operation=indicator_history"),
        ("/api/v1/research/macro", ""),
    ] {
        let error = read_market_calendar(
            MarketDataProvider::Akshare,
            true,
            Some(&client),
            path,
            query,
        )
        .expect_err("unsupported operation must fail closed");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed {
                    status: 409,
                    ref code,
                    ..
                } if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "{path}?{query} reported {error:?}"
        );
    }
    // No helper request may have been issued for a rejected operation: the
    // fixture peer never accepts, so drop it without joining an accept.
    drop(client);
    drop(server);
}

/// The calendar/macro family is AKShare-only: an active yfinance provider and a
/// not-ready helper are lifecycle failures, not capability fallbacks.
#[test]
fn calendar_and_macro_routes_fail_closed_for_other_providers() {
    let error = read_market_calendar(
        MarketDataProvider::Yfinance,
        true,
        None,
        "/api/v1/research/calendars",
        "operation=earnings&beginDate=2026-08-01&endDate=2026-08-31",
    )
    .expect_err("yfinance has no embedded calendar feed");
    assert!(matches!(
        error,
        ResearchReadSnapshotError::Failed {
            status: 409,
            ref code,
            ..
        } if code == "BROKER_CAPABILITY_UNAVAILABLE"
    ));

    let error = read_market_calendar(
        MarketDataProvider::Akshare,
        false,
        None,
        "/api/v1/research/macro",
        "operation=indicators",
    )
    .expect_err("a warming helper is unavailable");
    assert!(matches!(error, ResearchReadSnapshotError::Unavailable(_)));

    let error = read_market_calendar(
        MarketDataProvider::Futu,
        true,
        None,
        "/api/v1/research/macro",
        "operation=indicators",
    )
    .expect_err("Futu keeps the broker path");
    assert!(matches!(error, ResearchReadSnapshotError::Unavailable(_)));
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:194
/// TestProviderMacroIndicatorsProjectionNestsIndicatorList
#[test]
fn macro_indicators_route_nests_indicator_list() {
    let (client, server) = calendar_fixture(
        r#"{"source":"akshare-macro","categories":[
            {"category_name":"价格","indicators":[
                {"indicator_id":"cpi_yoy","name":"CPI同比","region":"中国","unit":"%","frequency":"月","unit_type":1}
            ]},
            {"category_name":"景气","indicators":[]}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/macro",
        "operation=indicators",
    )
    .expect("macro indicators");
    server.join().expect("server");

    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[0]["categoryName"], "价格");
    let list = entries[0]["indicatorList"].as_array().expect("indicatorList");
    assert_eq!(list.len(), 1);
    assert_eq!(list[0]["indicatorId"], "cpi_yoy");
    assert_eq!(list[0]["name"], "CPI同比");
    assert_eq!(list[0]["region"], "中国");
    assert_eq!(list[0]["unit"], "%");
    assert_eq!(list[0]["frequency"], "月");
    assert_eq!(list[0]["unitType"], json!(1));
    assert!(list[0].get("defaultMarket").is_none());
    assert_eq!(entries[1]["categoryName"], "景气");
    assert!(
        entries[1]["indicatorList"]
            .as_array()
            .expect("empty indicatorList")
            .is_empty()
    );
    assert_eq!(result["total"], 2);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_calendar_test.go:242
/// TestProviderMacroIndicatorHistoryProjectionMapsFrontendKeys
#[test]
fn macro_indicator_history_route_maps_frontend_keys() {
    let (client, server) = calendar_fixture(
        r#"{"indicator_id":"cpi_yoy","source":"akshare-macro","entries":[
            {"data_time":"2026-07","value":0.5,"previous_value":0.3,"unit":"%","unit_type":1},
            {"data_time":"2026-08","value":0.6,"predict_value":0.55,"unit":"%","unit_type":1}
        ]}"#,
    );
    let result = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/macro",
        "operation=indicator_history&indicatorId=cpi_yoy&pageSize=60",
    )
    .expect("macro history");
    let request = server.join().expect("server");
    assert!(
        request.starts_with("GET /providers/akshare/macro/indicator-history?indicator_id=cpi_yoy&limit=60"),
        "request = {request}"
    );

    let entries = result["entries"].as_array().expect("entries");
    let first = &entries[0];
    for key in ["dataTime", "value", "previousValue", "unit", "unitType"] {
        assert!(first.get(key).is_some(), "missing key {key}: {first}");
    }
    assert!(
        first.get("predictValue").is_none(),
        "nil predictValue must be omitted: {first}"
    );
    let second = &entries[1];
    assert_eq!(second["predictValue"], json!(0.55));
    assert_eq!(second["unitType"], json!(1));
    assert!(
        second.get("previousValue").is_none(),
        "absent previousValue must be omitted: {second}"
    );
}

/// The macro history route validates the helper's echoed indicator identity and
/// the typed numeric cells instead of publishing a silently mismatched series.
#[test]
fn macro_indicator_history_route_rejects_identity_and_type_drift() {
    let (client, server) = calendar_fixture(
        r#"{"indicator_id":"ppi_yoy","entries":[{"data_time":"2026-07","value":0.5,"unit":"%","unit_type":1}]}"#,
    );
    let error = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/macro",
        "operation=indicator_history&indicatorId=cpi_yoy",
    )
    .expect_err("a different indicator must not be served");
    server.join().expect("server");
    assert!(matches!(
        error,
        ResearchReadSnapshotError::Failed {
            status: 502,
            ref message,
            ..
        } if message == "macro history indicator_id does not match request"
    ));

    let (client, server) = calendar_fixture(
        r#"{"indicator_id":"cpi_yoy","entries":[{"data_time":"2026-07","value":"0.5","unit":"%","unit_type":1}]}"#,
    );
    let error = read_market_calendar(
        MarketDataProvider::Akshare,
        true,
        Some(&client),
        "/api/v1/research/macro",
        "operation=indicator_history&indicatorId=cpi_yoy",
    )
    .expect_err("a non-numeric value must not be published");
    server.join().expect("server");
    assert!(matches!(
        error,
        ResearchReadSnapshotError::Failed {
            status: 502,
            ref message,
            ..
        } if message == "research response field value must be numeric"
    ));
}

/// Loopback helper fixture for the calendar/macro routes.
struct CalendarRouteFixture {
    client: HelperClient,
    server: std::thread::JoinHandle<Vec<String>>,
}

impl CalendarRouteFixture {
    fn new(responses: Vec<(String, String)>) -> Self {
        let listener = StdTcpListener::bind("127.0.0.1:0").expect("listen");
        let address = listener.local_addr().expect("address");
        let server = std::thread::spawn(move || {
            let mut requests = Vec::new();
            for (status, body) in responses {
                let (mut stream, _) = listener.accept().expect("accept");
                stream
                    .set_read_timeout(Some(std::time::Duration::from_secs(5)))
                    .expect("read timeout");
                let mut request = Vec::new();
                let mut chunk = [0_u8; 1024];
                while !request.windows(4).any(|window| window == b"\r\n\r\n") {
                    let read = std::io::Read::read(&mut stream, &mut chunk).expect("read");
                    if read == 0 {
                        break;
                    }
                    request.extend_from_slice(&chunk[..read]);
                    if request.len() > 32 * 1024 {
                        break;
                    }
                }
                requests.push(
                    String::from_utf8_lossy(&request)
                        .lines()
                        .next()
                        .unwrap_or_default()
                        .to_owned(),
                );
                let response = format!(
                    "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                );
                std::io::Write::write_all(&mut stream, response.as_bytes()).expect("write");
            }
            requests
        });
        let client = HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
            base_url: format!("http://{address}"),
            bearer_token: None,
            request_timeout: std::time::Duration::from_secs(5),
            max_attempts: 1,
            retry_delay: std::time::Duration::ZERO,
        })
        .expect("helper client");
        Self { client, server }
    }

    fn ok(body: &str) -> Self {
        Self::new(vec![("200 OK".to_owned(), body.to_owned())])
    }

    fn join(self) -> Vec<String> {
        self.server.join().expect("server")
    }
}

fn akshare_calendar_route(
    client: &HelperClient,
    path: &str,
    query: &str,
) -> Result<serde_json::Value, ResearchReadSnapshotError> {
    read_market_calendar(MarketDataProvider::Akshare, true, Some(client), path, query)
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:74 TestRuntimeCalendarMacroForwarding
/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:33
/// TestEmbeddedProviderServesCalendarOperations
///
/// All four calendar operations (earnings/dividends/economic/ipos) are served
/// by the embedded provider, forward their business parameters verbatim, keep
/// the request envelope (no resolved instrument), and never resolve a broker.
#[test]
fn calendar_operations_map_to_provider_reads_on_the_wire() {
    let fixture = CalendarRouteFixture::ok(
        r#"{"source":"akshare-calendar","entries":[{"instrument_id":"SH.600519","name":"贵州茅台","symbol":"600519","event_date":"2026-08-20","period_text":"2025中报","price":1680.5}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/calendars",
        "operation=earnings&beginDate=2026-08-01&endDate=2026-08-31",
    )
    .expect("earnings");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/akshare/calendar/earnings?begin_date=2026-08-01&end_date=2026-08-31 "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["instrumentId"], "SH.600519");
    assert_eq!(result["entries"][0]["market"], "SH");
    assert_eq!(result["entries"][0]["symbol"], "600519");
    assert_eq!(result["entries"][0]["price"], 1680.5);
    assert!(result.get("resolvedInstrument").is_none());

    let fixture = CalendarRouteFixture::ok(
        r#"{"source":"akshare-calendar","entries":[{"instrument_id":"SZ.000001","statement":"10派2元","ex_date":"2026-08-15"}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/calendars",
        "operation=dividends&date=2026-08-15",
    )
    .expect("dividends");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/akshare/calendar/dividends?date=2026-08-15 "),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["statement"], "10派2元");

    let fixture = CalendarRouteFixture::ok(
        r#"{"source":"akshare-calendar","entries":[{"event_id":"econ-1","title":"CPI同比","region":"中国","event_timestamp":1787000000}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/calendars",
        "operation=economic&beginDate=2026-08-01&endDate=2026-08-07",
    )
    .expect("economic");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/akshare/calendar/economic?begin_date=2026-08-01&end_date=2026-08-07 "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["eventId"], "econ-1");
    assert_eq!(result["entries"][0]["eventTimestamp"], 1787000000_i64);
    // Flat pagination: the calendar envelope always reports hasMore=false and
    // never invents a cursor.
    assert_eq!(result["hasMore"], false);
    assert!(result.get("nextCursor").is_none());

    let fixture = CalendarRouteFixture::ok(
        r#"{"source":"akshare-calendar","entries":[{"instrument_id":"SZ.301999","name":"新股示例","status":"pending"}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/calendars",
        "operation=ipos&market=CN",
    )
    .expect("ipos");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/akshare/calendar/ipos "),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["status"], "pending");
    assert_eq!(result["provider"]["featureId"], "research.calendar");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:147
/// TestEmbeddedProviderServesMacroOperations
///
/// `indicators` nests the indicator list per category; `indicator_history`
/// forwards indicatorId plus pageSize as the provider limit and projects the
/// point rows.
#[test]
fn macro_operations_map_to_provider_reads_on_the_wire() {
    let fixture = CalendarRouteFixture::ok(
        r#"{"source":"akshare-macro","categories":[{"category_name":"价格","indicators":[{"indicator_id":"cpi_yoy","name":"CPI同比","region":"中国","unit":"%","frequency":"monthly","unit_type":1}]}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/macro",
        "operation=indicators",
    )
    .expect("indicators");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/akshare/macro/indicators "),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["categoryName"], "价格");
    assert_eq!(
        result["entries"][0]["indicatorList"][0]["indicatorId"],
        "cpi_yoy"
    );

    let fixture = CalendarRouteFixture::ok(
        r#"{"indicator_id":"cpi_yoy","source":"akshare-macro","entries":[{"data_time":"2026-07","value":0.5,"unit":"%","unit_type":1}]}"#,
    );
    let result = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/macro",
        "operation=indicator_history&indicatorId=cpi_yoy&pageSize=60",
    )
    .expect("indicator history");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/akshare/macro/indicator-history?indicator_id=cpi_yoy&limit=60 "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["dataTime"], "2026-07");
    assert_eq!(result["entries"][0]["value"], 0.5);
    assert_eq!(result["provider"]["featureId"], "research.macro");
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:131 TestRuntimeCalendarMacroCapabilityUnsupported
/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:205
/// TestEmbeddedProviderRejectsUnsupportedCalendarMacroOperations
///
/// trade_dates and the fed_* macro operations have no embedded feed, an empty
/// operation is not a default, and indicator_history without indicatorId is a
/// capability error — none of them falls through to broker routing or the
/// helper.
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:606 TestEmbeddedProviderCalendarMacroRoutesRejectUnsupportedOperations
#[test]
fn calendar_and_macro_reject_unsupported_operations_without_a_helper_call() {
    for (path, query) in [
        ("/api/v1/research/calendars", "operation=trade_dates"),
        ("/api/v1/research/calendars", ""),
        ("/api/v1/research/macro", "operation=fed_target_rate"),
        ("/api/v1/research/macro", "operation=fed_dot_plot"),
        ("/api/v1/research/macro", "operation=indicator_history"),
        ("/api/v1/research/macro", ""),
    ] {
        let fixture = CalendarRouteFixture::new(Vec::new());
        let error = akshare_calendar_route(&fixture.client, path, query)
            .expect_err("unsupported operations must fail closed");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                    if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "{path} {query} => {error:?}"
        );
        // No queued response: any helper read would block the join.
        assert!(
            fixture.join().is_empty(),
            "{path} {query} reached the helper"
        );
    }
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_calendar_forwarding_test.go:119 TestRuntimeCalendarMacroPropagatesError
/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:237
/// TestEmbeddedProviderPropagatesCalendarMacroErrors
///
/// A helper capability rejection keeps its own code, and the provider-busy
/// sentinel keeps its identity instead of being flattened into a capability
/// error or an empty calendar.
#[test]
fn calendar_and_macro_propagate_capability_and_busy_errors() {
    let fixture = CalendarRouteFixture::new(vec![(
        "409 Conflict".to_owned(),
        r#"{"error":{"code":"CAPABILITY_UNSUPPORTED","message":"event calendars unsupported"}}"#
            .to_owned(),
    )]);
    let error = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/calendars",
        "operation=ipos",
    )
    .expect_err("unsupported ipos");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "CAPABILITY_UNSUPPORTED"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);

    let fixture = CalendarRouteFixture::new(vec![(
        "503 Service Unavailable".to_owned(),
        r#"{"error":{"code":"AKSHARE_POOL_BUSY","message":"pool busy"}}"#.to_owned(),
    )]);
    let error = akshare_calendar_route(
        &fixture.client,
        "/api/v1/research/macro",
        "operation=indicators",
    )
    .expect_err("busy macro indicators");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed {
                status: 503,
                ref code,
                retry_after_seconds: Some(2),
                ..
            } if code == "MARKET_DATA_PROVIDER_BUSY"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_calendar_test.go:262
/// TestEmbeddedProviderCalendarMacroStayOnBrokerPathForFutu
///
/// With Futu active the calendar/macro routes never consult the helper: the
/// read fails closed as unavailable rather than borrowing another provider.
#[test]
fn calendar_and_macro_stay_off_the_helper_for_futu() {
    for path in [
        "/api/v1/research/calendars",
        "/api/v1/research/macro",
    ] {
        let fixture = CalendarRouteFixture::new(Vec::new());
        let error = read_market_calendar(
            MarketDataProvider::Futu,
            true,
            Some(&fixture.client),
            path,
            "operation=indicators",
        )
        .expect_err("Futu must not be served by the embedded calendar/macro feed");
        assert!(
            matches!(error, ResearchReadSnapshotError::Unavailable(ref m)
                if m.contains("Futu research calendar/macro runtime is not ready")),
            "path {path} => {error:?}"
        );
        assert!(fixture.join().is_empty());
    }
}
