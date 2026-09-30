use super::*;

#[test]
fn test_board_kind_defaults_empty_to_industry() {
    // Parity: go:452dea11:internal/marketdata/rankings_facade_test.go:184 TestServiceIndustriesDefaultsEmptyKindToIndustry
    let empty_query = QueryMap::parse("").expect("empty query");
    assert_eq!(board_kind(&empty_query).expect("kind"), "industry");

    let explicit_industry = QueryMap::parse("plateType=industry").expect("query");
    assert_eq!(board_kind(&explicit_industry).expect("kind"), "industry");

    let uppercase_industry = QueryMap::parse("plateType=INDUSTRY").expect("query");
    assert_eq!(board_kind(&uppercase_industry).expect("kind"), "industry");

    let concept = QueryMap::parse("plateType=concept").expect("query");
    assert_eq!(board_kind(&concept).expect("kind"), "concept");

    let invalid = QueryMap::parse("plateType=unsupported").expect("query");
    assert!(board_kind(&invalid).is_err());
}

#[test]
fn rankings_limit_prefers_positive_page_size() {
    let query = QueryMap::parse("pageSize=40&limit=5").expect("query");
    assert_eq!(request_limit(&query).expect("limit"), 40);
}

#[test]
fn rankings_limit_falls_back_to_legacy_limit_and_default() {
    let zero = QueryMap::parse("pageSize=0&limit=5").expect("query");
    assert_eq!(request_limit(&zero).expect("legacy fallback"), 5);

    let malformed = QueryMap::parse("pageSize=bad&limit=-2").expect("query");
    assert_eq!(request_limit(&malformed).expect("default"), DEFAULT_LIMIT);

    let empty = QueryMap::parse("").expect("query");
    assert_eq!(request_limit(&empty).expect("default"), DEFAULT_LIMIT);
}

#[test]
fn rankings_limit_clamps_to_provider_bounds() {
    let query = QueryMap::parse("pageSize=10000").expect("query");
    assert_eq!(request_limit(&query).expect("clamp"), MAX_LIMIT);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:351
/// TestEmbeddedRankingsLimitPrecedenceAndClamp
///
/// The embedded rankings/industry facade receives pageSize as an integer: zero,
/// negative, and malformed values count as "not supplied", the legacy `limit`
/// query is the next fallback, and the provider default/bounds apply last.
#[test]
fn embedded_rankings_limit_precedence_and_clamp() {
    let explicit = QueryMap::parse("pageSize=12").expect("query");
    assert_eq!(request_limit(&explicit).expect("explicit pageSize"), 12);

    let clamped = QueryMap::parse("pageSize=500").expect("query");
    assert_eq!(request_limit(&clamped).expect("clamped pageSize"), MAX_LIMIT);

    let legacy = QueryMap::parse("limit=9").expect("query");
    assert_eq!(request_limit(&legacy).expect("legacy limit"), 9);

    let default = QueryMap::parse("").expect("query");
    assert_eq!(request_limit(&default).expect("default"), DEFAULT_LIMIT);

    let zero_page_size = QueryMap::parse("pageSize=0&limit=4").expect("query");
    assert_eq!(
        request_limit(&zero_page_size).expect("zero pageSize falls back to limit"),
        4
    );

    let negative_page_size = QueryMap::parse("pageSize=-3&limit=4").expect("query");
    assert_eq!(
        request_limit(&negative_page_size).expect("negative pageSize falls back to limit"),
        4
    );

    let malformed = QueryMap::parse("pageSize=bad&limit=4").expect("query");
    assert_eq!(
        request_limit(&malformed).expect("malformed pageSize falls back to limit"),
        4
    );
}

#[test]
// Parity: go:452dea11:internal/integration/yfinance/provider_rankings_test.go:42 TestProviderRankingsConvertsEntriesAndAppliesDefaultLimit
fn provider_rankings_conversion_and_default_limit() {
    let query = QueryMap::parse("market=us&kind= Gainers &limit=0").expect("query");
    assert_eq!(request_limit(&query).expect("default limit"), DEFAULT_LIMIT);
    let payload = serde_json::json!({
        "market": "US", "kind": "gainers", "entries": [{
            "instrument_id": "us.aapl", "name": " Apple Inc. ", "price": 232.1,
            "change_rate": 1.25
        }], "source": ""
    });
    let (entries, source, market) = ranking_entries(&payload, "US", "gainers").expect("projection");
    assert_eq!(market, "US");
    assert_eq!(source, "market-data-rankings");
    assert_eq!(entries.len(), 1);
    assert_eq!(entries[0]["instrumentId"], "US.AAPL");
    assert_eq!(entries[0]["name"], "Apple Inc.");
    assert_eq!(entries[0]["changeRate"], serde_json::json!(1.25));
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:226
/// TestProviderRankingsProjectionMapsFrontendKeys
///
/// Ranking rows expose the camelCase keys the research tables read, omit every
/// null metric, uppercase the instrument id, and derive market/symbol from its
/// prefix.
#[test]
fn provider_rankings_projection_maps_frontend_keys() {
    let payload = serde_json::json!({
        "market": "CN",
        "kind": "gainers",
        "source": "akshare-rankings",
        "entries": [
            {
                "instrument_id": "sh.600519",
                "name": "贵州茅台",
                "price": 1680.5,
                "change_rate": 5.42,
                "change_amount": 86.4,
                "volume": 123456,
                "turnover": 207000000.0,
                "turnover_ratio": 0.98,
                "pe_ttm": 24.6,
                "market_cap": 2110000000000.0
            },
            {"instrument_id": "SZ.000001", "name": "平安银行"}
        ]
    });
    let (entries, source, market) =
        ranking_entries(&payload, "CN", "gainers").expect("rankings projection");
    assert_eq!(market, "CN");
    assert_eq!(source, "akshare-rankings");
    assert_eq!(entries.len(), 2);

    let first = &entries[0];
    for key in [
        "instrumentId",
        "market",
        "symbol",
        "name",
        "price",
        "changeRate",
        "changeAmount",
        "volume",
        "turnover",
        "turnoverRatio",
        "peTTM",
        "marketCap",
    ] {
        assert!(first.get(key).is_some(), "first entry missing key {key}: {first}");
    }
    assert_eq!(first["instrumentId"], "SH.600519");
    assert_eq!(first["market"], "SH");
    assert_eq!(first["symbol"], "600519");
    assert_eq!(first["name"], "贵州茅台");
    assert_eq!(first["changeRate"], serde_json::json!(5.42));
    assert_eq!(first["peTTM"], serde_json::json!(24.6));

    let second = &entries[1];
    for key in ["price", "changeRate", "turnover", "marketCap"] {
        assert!(
            second.get(key).is_none(),
            "nil field {key} must be omitted: {second}"
        );
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:290
/// TestProviderIndustryBoardsProjectionMapsFrontendKeys
#[test]
fn provider_industry_boards_projection_maps_frontend_keys() {
    let payload = serde_json::json!({
        "market": "CN",
        "kind": "concept",
        "source": "akshare-industries",
        "boards": [{
            "name": "人工智能",
            "change_rate": 2.31,
            "turnover": 1500000000.0,
            "leading_stock_name": "宁德时代",
            "leading_stock_change_rate": 7.02
        }]
    });
    let (entries, source, market) =
        industry_board_entries(&payload, "CN", "concept").expect("board projection");
    assert_eq!(market, "CN");
    assert_eq!(source, "akshare-industries");
    assert_eq!(entries.len(), 1);
    let entry = &entries[0];
    assert_eq!(entry["instrumentId"], "CN.人工智能");
    assert_eq!(entry["market"], "CN");
    assert_eq!(entry["name"], "人工智能");
    assert_eq!(entry["productClass"], "plate");
    assert_eq!(entry["changeRate"], serde_json::json!(2.31));
    assert_eq!(entry["turnover"], serde_json::json!(1500000000.0));
    assert_eq!(entry["leadingStockName"], "宁德时代");
    assert_eq!(entry["leadingStockChangeRate"], serde_json::json!(7.02));
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:326
/// TestProviderIndustryMembersProjectionUsesRankingKeys
#[test]
fn provider_industry_members_projection_uses_ranking_keys() {
    let payload = serde_json::json!({
        "market": "CN",
        "kind": "industry",
        "board": "半导体",
        "source": "akshare-industries",
        "entries": [{"instrument_id": "SH.688981", "name": "中芯国际", "price": 92.4}]
    });
    let (entries, source, market) =
        member_entries(&payload, "半导体", "CN", Some("industry")).expect("member projection");
    assert_eq!(market, "CN");
    assert_eq!(source, "akshare-industries");
    assert_eq!(entries.len(), 1);
    let entry = &entries[0];
    assert_eq!(entry["instrumentId"], "SH.688981");
    assert_eq!(entry["symbol"], "688981");
    assert_eq!(entry["name"], "中芯国际");
    assert_eq!(entry["price"], serde_json::json!(92.4));
}

/// The projection owns identity checks: a response that echoes a different
/// market/kind/board than the request must fail closed instead of being served
/// as the caller's data.
#[test]
fn market_research_projection_rejects_identity_drift() {
    let drifted = serde_json::json!({
        "market": "US",
        "kind": "gainers",
        "source": "akshare-rankings",
        "entries": []
    });
    assert!(matches!(
        ranking_entries(&drifted, "CN", "gainers"),
        Err(ResearchReadSnapshotError::Failed { status: 502, .. })
    ));

    let wrong_kind = serde_json::json!({
        "market": "CN",
        "kind": "losers",
        "source": "akshare-rankings",
        "entries": []
    });
    assert!(matches!(
        ranking_entries(&wrong_kind, "CN", "gainers"),
        Err(ResearchReadSnapshotError::Failed { status: 502, .. })
    ));

    let wrong_board = serde_json::json!({
        "market": "CN",
        "kind": "industry",
        "board": "白酒",
        "source": "akshare-industries",
        "entries": []
    });
    assert!(matches!(
        member_entries(&wrong_board, "半导体", "CN", Some("industry")),
        Err(ResearchReadSnapshotError::Failed { status: 502, .. })
    ));
}

use std::io::{Read, Write};
use std::net::TcpListener as StdTcpListener;
use std::time::Duration;

/// Loopback helper fixture for the market research routes.
///
/// The production code performs its helper call on a nested Tokio runtime from
/// a blocking thread, so the peer lives on a plain OS thread with its own
/// listener and is joined by the test after the call returns. The fixture
/// answers the queued responses in order and records each request line so a
/// test can assert the exact provider-neutral wire request.
struct MarketResearchFixture {
    client: HelperClient,
    server: std::thread::JoinHandle<Vec<String>>,
}

impl MarketResearchFixture {
    fn new(responses: Vec<(String, String)>) -> Self {
        let listener = StdTcpListener::bind("127.0.0.1:0").expect("listen");
        let address = listener.local_addr().expect("address");
        let server = std::thread::spawn(move || {
            let mut requests = Vec::new();
            for (status, body) in responses {
                let (mut stream, _) = listener.accept().expect("accept");
                stream
                    .set_read_timeout(Some(Duration::from_secs(5)))
                    .expect("read timeout");
                let mut request = Vec::new();
                let mut chunk = [0_u8; 1024];
                while !request.windows(4).any(|window| window == b"\r\n\r\n") {
                    let read = Read::read(&mut stream, &mut chunk).expect("read");
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
                Write::write_all(&mut stream, response.as_bytes()).expect("write");
            }
            requests
        });
        let client = HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
            base_url: format!("http://{address}"),
            bearer_token: None,
            request_timeout: Duration::from_secs(5),
            max_attempts: 1,
            retry_delay: Duration::ZERO,
        })
        .expect("helper client");
        Self { client, server }
    }

    /// Fixture that answers a single 200 with the supplied body.
    fn ok(body: String) -> Self {
        Self::new(vec![("200 OK".to_owned(), body)])
    }

    /// Fixture with queued responses; an empty list means "no helper call may
    /// happen", because the read would otherwise block on the join.
    fn with_responses(responses: Vec<(String, String)>) -> Self {
        Self::new(responses)
    }

    fn join(self) -> Vec<String> {
        self.server.join().expect("server")
    }
}

fn rankings_body(kind: &str, market: &str) -> String {
    format!(
        r#"{{"market":"{market}","kind":"{kind}","source":"akshare-rankings","entries":[{{"instrument_id":"SH.600519","name":"贵州茅台","price":1680.5,"change_rate":5.42}}]}}"#
    )
}

fn boards_body(kind: &str, market: &str) -> String {
    format!(
        r#"{{"market":"{market}","kind":"{kind}","source":"akshare-industries","boards":[{{"name":"人工智能"}}]}}"#
    )
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:66 TestRuntimeForwardsRankingsToCapableActiveProvider
/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:53
/// TestEmbeddedProviderMapsRankingsOperationsToKinds
///
/// The console sends `top_movers` with `direction=up|down` (default up) and
/// `hot`; the facade maps those to the provider kinds gainers/losers/active and
/// forwards market, kind, and the page limit verbatim.
#[test]
fn rankings_operations_map_to_provider_kinds_on_the_wire() {
    for (query, expected) in [
        ("operation=top_movers&direction=up&pageSize=30", "gainers"),
        ("operation=top_movers&pageSize=30", "gainers"),
        ("operation=top_movers&direction=down&pageSize=30", "losers"),
        ("operation=hot&pageSize=30", "active"),
    ] {
        // The fixture echoes the kind the route asked for, so a wrong kind in
        // the request is a projection identity failure rather than a pass.
        let fixture = MarketResearchFixture::ok(rankings_body(expected, "CN"));
        let result = read_market_research(
            MarketDataProvider::Akshare,
            true,
            Some(&fixture.client),
            "/api/v1/research/rankings",
            &format!("market=CN&{query}"),
        )
        .expect("rankings read");
        let requests = fixture.join();
        assert_eq!(requests.len(), 1);
        assert!(
            requests[0].starts_with(&format!(
                "GET /providers/akshare/rankings?market=CN&kind={expected}&limit=30 "
            )),
            "request = {}",
            requests[0]
        );
        assert_eq!(result["entries"][0]["instrumentId"], "SH.600519");
        assert_eq!(result["provider"]["brokerId"], "akshare");
        assert_eq!(
            result["provider"]["selectionReason"],
            "embedded-market-data-provider"
        );
        assert_eq!(result["provider"]["featureId"], "research.rankings");
    }
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:125 TestRuntimeRankingsAndIndustriesRejectProvidersWithoutCapability
/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:101
/// TestEmbeddedProviderRejectsUnmappedRankingsOperations
///
/// Futu-only ranking operations (and an empty operation) must fail closed with
/// the capability error and never reach the helper.
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:335 TestEmbeddedProviderRankingsRouteMapsUnsupportedOperations
#[test]
fn rankings_reject_unmapped_operations_without_a_helper_call() {
    for operation in [
        "pre_market",
        "after_hours",
        "overnight",
        "high_dividend_state",
        "fund_catalog",
        "",
    ] {
        // No queued response: any helper call would block the join, so the
        // capability error proves the read never left the process.
        let fixture = MarketResearchFixture::with_responses(Vec::new());
        let error = read_market_research(
            MarketDataProvider::Yfinance,
            true,
            Some(&fixture.client),
            "/api/v1/research/rankings",
            &format!("market=US&operation={operation}"),
        )
        .expect_err("unmapped operations must not be served");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                    if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "operation {operation:?} => {error:?}"
        );
        assert!(
            fixture.join().is_empty(),
            "operation {operation:?} reached the helper"
        );
    }
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_rankings_industry_forwarding_test.go:92 TestRuntimeForwardsIndustryReadsToCapableActiveProvider
/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:127
/// TestEmbeddedProviderMapsIndustryBoardOperations
///
/// `plate_list` (industry feature) and `heatmap` (rankings feature) both render
/// the industry board feed; `plateType` defaults to industry and swaps to
/// concept, and the heatmap keeps the rankings feature id.
#[test]
fn industry_board_operations_map_to_provider_kinds_on_the_wire() {
    for (path, query, expected_plate, expected_feature) in [
        (
            "/api/v1/research/industries",
            "market=CN&operation=plate_list&plateType=concept",
            "concept",
            "research.industry",
        ),
        (
            "/api/v1/research/industries",
            "market=CN&operation=plate_list",
            "industry",
            "research.industry",
        ),
        (
            "/api/v1/research/rankings",
            "market=CN&operation=heatmap&plateType=industry",
            "industry",
            "research.rankings",
        ),
        (
            "/api/v1/research/rankings",
            "market=CN&operation=heatmap&plateType=concept",
            "concept",
            "research.rankings",
        ),
    ] {
        let fixture = MarketResearchFixture::ok(boards_body(expected_plate, "CN"));
        let result = read_market_research(
            MarketDataProvider::Akshare,
            true,
            Some(&fixture.client),
            path,
            query,
        )
        .expect("industry boards");
        let requests = fixture.join();
        assert!(
            requests[0].starts_with(&format!(
                "GET /providers/akshare/industries?kind={expected_plate}&market=CN "
            )),
            "request = {}",
            requests[0]
        );
        assert_eq!(result["entries"][0]["instrumentId"], "CN.人工智能");
        assert_eq!(result["entries"][0]["productClass"], "plate");
        assert_eq!(result["provider"]["featureId"], expected_feature);
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:173
/// TestEmbeddedProviderServesPlateMembersFromInstrumentID
///
/// `plate_members` derives the board from the `CN.<board>` instrumentId and
/// reuses the ranking entry keys for the member rows.
#[test]
fn industry_plate_members_read_the_board_from_the_instrument_id() {
    let fixture = MarketResearchFixture::ok(
        // Rust validates the members identity (market/board/kind) before projecting,
        // so the fixture echoes them. Go projects `Entries` without checking the
        // response envelope, which is the identity-drift hole the Rust owner closes.
        r#"{"market":"CN","kind":"industry","board":"半导体","source":"akshare-industries","entries":[{"instrument_id":"SH.688981","name":"中芯国际"}]}"#
            .to_owned(),
    );
    let result = read_market_research(
        MarketDataProvider::Akshare,
        true,
        Some(&fixture.client),
        "/api/v1/research/industries",
        "market=CN&instrumentId=CN.半导体&operation=plate_members&pageSize=50",
    )
    .expect("plate members");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/akshare/industries/%E5%8D%8A%E5%AF%BC%E4%BD%93/members?limit=50&market=CN "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["symbol"], "688981");
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "CN.半导体");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:199
/// TestEmbeddedProviderRejectsUnsupportedIndustryOperationsAndPlateTypes
///
/// Industry-chain operations are Futu-only and region/theme plate types have no
/// embedded feed, so both are capability errors rather than empty results.
#[test]
fn industry_rejects_unsupported_operations_and_plate_types() {
    for operation in [
        "chains",
        "chain_detail",
        "chains_by_plate",
        "plate",
        "plate_stocks",
    ] {
        let fixture = MarketResearchFixture::with_responses(Vec::new());
        let error = read_market_research(
            MarketDataProvider::Akshare,
            true,
            Some(&fixture.client),
            "/api/v1/research/industries",
            &format!("market=CN&operation={operation}"),
        )
        .expect_err("unsupported industry operation");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                    if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "operation {operation:?} => {error:?}"
        );
        assert!(fixture.join().is_empty());
    }
    for plate_type in ["region", "theme"] {
        let fixture = MarketResearchFixture::with_responses(Vec::new());
        let error = read_market_research(
            MarketDataProvider::Akshare,
            true,
            Some(&fixture.client),
            "/api/v1/research/industries",
            &format!("market=CN&operation=plate_list&plateType={plate_type}"),
        )
        .expect_err("unsupported plate type");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                    if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "plateType {plate_type:?} => {error:?}"
        );
        assert!(fixture.join().is_empty());
    }
}

// Parity: go:452dea11:internal/integration/yfinance/provider_rankings_test.go:71 TestProviderRankingsRejectsNonUSMarketsWithoutSidecarCall
#[test]
fn yfinance_rankings_reject_non_us_markets_without_helper_calls() {
    for market in ["HK", "SH", "SZ", "CN"] {
        let fixture = MarketResearchFixture::with_responses(Vec::new());
        let error = read_market_research(
            MarketDataProvider::Yfinance,
            true,
            Some(&fixture.client),
            "/api/v1/research/rankings",
            &format!("market={market}&operation=hot"),
        )
        .expect_err("YFinance rankings must reject non-US markets");
        assert!(matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "BROKER_CAPABILITY_UNAVAILABLE"
        ));
        assert!(fixture.join().is_empty(), "market={market}");
    }

    let fixture = MarketResearchFixture::with_responses(Vec::new());
    let error = read_market_research(
        MarketDataProvider::Yfinance,
        true,
        Some(&fixture.client),
        "/api/v1/research/rankings",
        "market=US&operation=breakout",
    )
    .expect_err("unknown rankings operation must be rejected");
    assert!(matches!(
        error,
        ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
            if code == "BROKER_CAPABILITY_UNAVAILABLE"
    ));
    assert!(fixture.join().is_empty());
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:235
/// TestEmbeddedProviderPropagatesRankingsCapabilityErrors
///
/// A helper-side capability rejection keeps its own code instead of being
/// rewritten into an empty result, and the warming lifecycle sentinel keeps its
/// identity plus Retry-After so the transport can answer 503.
#[test]
fn rankings_propagate_capability_and_lifecycle_errors() {
    let fixture = MarketResearchFixture::with_responses(vec![(
        "409 Conflict".to_owned(),
        r#"{"error":{"code":"CAPABILITY_UNSUPPORTED","message":"market rankings unsupported for akshare"}}"#
            .to_owned(),
    )]);
    let error = read_market_research(
        MarketDataProvider::Akshare,
        true,
        Some(&fixture.client),
        "/api/v1/research/rankings",
        "market=HK&operation=hot",
    )
    .expect_err("unsupported rankings");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "CAPABILITY_UNSUPPORTED"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);

    let fixture = MarketResearchFixture::with_responses(vec![(
        "503 Service Unavailable".to_owned(),
        r#"{"error":{"code":"AKSHARE_RUNTIME_WARMING","message":"runtime loading"}}"#.to_owned(),
    )]);
    let error = read_market_research(
        MarketDataProvider::Akshare,
        true,
        Some(&fixture.client),
        "/api/v1/research/industries",
        "market=CN&operation=plate_list",
    )
    .expect_err("warming industry boards");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed {
                status: 503,
                ref code,
                retry_after_seconds: Some(1),
                ..
            } if code == "MARKET_DATA_PROVIDER_WARMING"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:262
/// TestEmbeddedProviderRankingsStayOnBrokerPathForFutu
///
/// Futu never serves the embedded market-research feed, so the route fails
/// closed with the capability error and performs no helper read.
#[test]
fn market_research_stays_off_the_helper_for_futu() {
    for path in [
        "/api/v1/research/rankings",
        "/api/v1/research/industries",
    ] {
        let fixture = MarketResearchFixture::with_responses(Vec::new());
        let error = read_market_research(
            MarketDataProvider::Futu,
            true,
            Some(&fixture.client),
            path,
            "market=US&operation=top_movers",
        )
        .expect_err("Futu must not be served by the embedded feed");
        // Observable contract: Futu never reaches the helper and never gets an
        // empty result; the rejection names the provider family as the
        // unsupported operand. The family-level early return in
        // `read_market_research` and the per-operation `helper_provider` guard
        // render the same answer, so this asserts behavior rather than one line.
        match error {
            ResearchReadSnapshotError::Failed {
                status,
                ref code,
                ref message,
                ..
            } => {
                assert_eq!(status, 409, "path {path}");
                assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE", "path {path}");
                assert!(
                    message.contains("\"futu\""),
                    "path {path} message = {message}"
                );
            }
            other => panic!("path {path} => {other:?}"),
        }
        assert!(fixture.join().is_empty());
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_rankings_test.go:278
/// TestEmbeddedProviderDefaultsEmptyMarketToProviderDefault
///
/// Market-wide reads default an absent market by provider: yfinance falls back
/// to US, AKShare to CN.
#[test]
fn market_research_defaults_an_absent_market_per_provider() {
    let fixture = MarketResearchFixture::ok(
        r#"{"market":"US","kind":"active","source":"yfinance-rankings","entries":[]}"#.to_owned(),
    );
    read_market_research(
        MarketDataProvider::Yfinance,
        true,
        Some(&fixture.client),
        "/api/v1/research/rankings",
        "operation=hot",
    )
    .expect("yfinance default market");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/yfinance/rankings?market=US&kind=active&"),
        "request = {}",
        requests[0]
    );

    let fixture = MarketResearchFixture::ok(boards_body("industry", "CN"));
    read_market_research(
        MarketDataProvider::Akshare,
        true,
        Some(&fixture.client),
        "/api/v1/research/industries",
        "operation=plate_list",
    )
    .expect("akshare default market");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/akshare/industries?kind=industry&market=CN "),
        "request = {}",
        requests[0]
    );
}
