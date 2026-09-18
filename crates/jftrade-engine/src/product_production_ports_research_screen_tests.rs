//! Embedded stock-screen facade tests.
//!
//! The Go facade (`internal/productfeatures/provider_facade_screen.go`) owns the
//! translation from a normalized screen definition to the provider-neutral
//! helper request, including the catalog gate and the shapes the embedded
//! catalog cannot execute. These tests drive the Rust owner
//! (`ProductionResearchScreenHelperPort`) end to end against a loopback helper
//! so the request the sidecar actually receives is the one under test.

use super::*;
use crate::product::product_research_screen_write_port::ResearchScreenColumn;
use jftrade_integration_marketdata_helper::HelperClient;
use serde_json::json;
use std::io::{Read, Write};
use std::net::TcpListener as StdTcpListener;
use std::sync::Arc;
use std::time::Duration;

const EMBEDDED_CATALOG: &str = "embedded-stock-screen-v1";
const FUTU_CATALOG: &str = "futu-stock-screen-v1";

/// The Go fixture definition: a price range plus a PE cap, an explicit sort,
/// and the six columns the console renders.
fn embedded_definition(broker: &str, market: &str) -> Value {
    json!({
        "brokerId": broker,
        "market": market,
        "catalogVersion": EMBEDDED_CATALOG,
        "querySchemaVersion": 2,
        "conditions": [
            {"factor": {"factorKey": "simple.price"}, "operator": "between",
             "value": {"min": 100.0, "max": 300.0}},
            {"factor": {"factorKey": "simple.pe_ttm"}, "operator": "between",
             "value": {"max": 40}}
        ],
        "columns": [
            {"id": "col-code", "factor": {"factorKey": "basic.code"}},
            {"id": "col-name", "factor": {"factorKey": "basic.name"}},
            {"id": "col-industry", "factor": {"factorKey": "basic.industry"}},
            {"id": "col-price", "factor": {"factorKey": "simple.price"}},
            {"id": "col-volume", "factor": {"factorKey": "simple.volume"}},
            {"id": "col-pb", "factor": {"factorKey": "simple.pb"}}
        ],
        "sorts": [
            {"factor": {"factorKey": "simple.market_cap"}, "direction": "desc"}
        ]
    })
}

fn embedded_columns() -> Vec<ResearchScreenColumn> {
    [
        ("col-code", "basic.code"),
        ("col-name", "basic.name"),
        ("col-industry", "basic.industry"),
        ("col-price", "simple.price"),
        ("col-volume", "simple.volume"),
        ("col-pb", "simple.pb"),
    ]
    .into_iter()
    .map(|(column_id, factor_key)| ResearchScreenColumn {
        column_id: column_id.to_owned(),
        instance_id: column_id.to_owned(),
        factor_key: factor_key.to_owned(),
        label: String::new(),
        unit: String::new(),
    })
    .collect()
}

fn screen_query(definition: Value, offset: i64, limit: i64) -> ResearchScreenWriteQuery {
    ResearchScreenWriteQuery {
        broker_id: definition
            .get("brokerId")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned(),
        account_id: String::new(),
        trading_environment: String::new(),
        market: definition
            .get("market")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned(),
        offset,
        limit,
        definition,
        columns: embedded_columns(),
    }
}

/// Loopback helper fixture that records the request line and answers one
/// queued response with the supplied status/body.
struct ScreenFixture {
    client: HelperClient,
    server: std::thread::JoinHandle<(String, String)>,
}

impl ScreenFixture {
    fn new(status: &str, body: &str) -> Self {
        let listener = StdTcpListener::bind("127.0.0.1:0").expect("listen");
        let address = listener.local_addr().expect("address");
        let body = body.to_owned();
        let status = status.to_owned();
        let server = std::thread::spawn(move || {
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
            let head = String::from_utf8_lossy(&request)
                .lines()
                .next()
                .unwrap_or_default()
                .to_owned();
            // Anything after the headers is the JSON body (the helper path is a
            // POST), so split it out to assert the translated definition.
            let payload = String::from_utf8_lossy(&request)
                .split_once("\r\n\r\n")
                .map(|(_, body)| body.to_owned())
                .unwrap_or_default();
            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            Write::write_all(&mut stream, response.as_bytes()).expect("write");
            (head, payload)
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

    fn join(self) -> (String, String) {
        self.server.join().expect("server")
    }
}

fn screen_port(
    client: Option<HelperClient>,
    provider: jftrade_settings::MarketDataProvider,
    helper_ready: bool,
) -> ProductionResearchScreenHelperPort {
    let state = Arc::new(crate::product::product_active_provider_state::ActiveProviderState::new(
        Some(provider),
    ));
    state.set_readiness(helper_ready, false, false);
    ProductionResearchScreenHelperPort {
        active_provider_state: state,
        helper: client,
        trade_runtime: None,
    }
}

const SCREEN_ROWS: &str = r#"{
    "entries": [
        {"instrument_id": "US.AAPL", "name": "Apple", "industry": "Technology",
         "quote_currency": "USD",
         "values": {"simple.price": 189.25, "simple.volume": 1234567}},
        {"instrument_id": "US.MSFT", "name": "Microsoft", "quote_currency": "USD",
         "values": {}}
    ],
    "total": 7,
    "has_more": true,
    "next_offset": 75,
    "as_of": "2026-08-15T20:00:00Z",
    "source": "yfinance-screen-us"
}"#;

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:83
/// TestEmbeddedProviderServesScreenAndProjectsRows
///
/// The facade folds the normalized definition into the provider-neutral helper
/// request (conditions, sorts, offset, limit), then projects identity and typed
/// cells for the console, including a missing value rendering as a missing cell.
#[test]
fn embedded_screen_projects_rows_and_forwards_the_definition() {
    let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let result = port
        .query(&screen_query(embedded_definition("yfinance", "US"), 50, 25))
        .expect("embedded screen query");
    let (head, payload) = fixture.join();
    assert!(
        head.starts_with("POST /providers/yfinance/screen "),
        "request = {head}"
    );
    let sent: Value = serde_json::from_str(&payload).expect("helper request body");
    assert_eq!(sent["market"], "US");
    assert_eq!(sent["offset"], 50);
    assert_eq!(sent["limit"], 25);
    assert_eq!(sent["conditions"][0]["factor_key"], "simple.price");
    // The Go facade renders bounds as bare JSON numbers (json.Number), so the
    // request keeps 100/300 rather than a float-rendered 100.0.
    assert_eq!(sent["conditions"][0]["min"], 100);
    assert_eq!(sent["conditions"][0]["max"], 300);
    assert!(sent["conditions"][1].get("min").is_none());
    assert_eq!(sent["conditions"][1]["max"], 40);
    assert_eq!(sent["sorts"][0]["factor_key"], "simple.market_cap");
    assert_eq!(sent["sorts"][0]["direction"], "desc");

    assert_eq!(result["provider"]["brokerId"], "yfinance");
    assert_eq!(
        result["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    assert_eq!(result["total"], 7);
    assert_eq!(result["hasMore"], true);
    assert_eq!(result["nextOffset"], 75);
    assert_eq!(result["asOf"], "2026-08-15T20:00:00Z");
    let rows = result["entries"].as_array().expect("rows");
    assert_eq!(rows.len(), 2);

    let row = &rows[0];
    assert_eq!(row["stockId"], "US.AAPL");
    assert_eq!(row["instrumentId"], "US.AAPL");
    assert_eq!(row["market"], "US");
    assert_eq!(row["symbol"], "AAPL");
    assert_eq!(row["name"], "Apple");
    assert_eq!(row["industry"], "Technology");
    assert_eq!(row["quoteCurrency"], "USD");
    assert_eq!(row["productClass"], "equity");
    assert_eq!(row["cells"]["col-code"]["value"]["type"], "string");
    assert_eq!(row["cells"]["col-code"]["value"]["string"], "AAPL");
    assert_eq!(row["cells"]["col-industry"]["value"]["string"], "Technology");
    assert_eq!(row["cells"]["col-price"]["value"]["type"], "number");
    assert_eq!(row["cells"]["col-price"]["value"]["number"], 189.25);
    assert_eq!(row["cells"]["col-price"]["value"]["unit"], "currency");
    assert_eq!(row["cells"]["col-volume"]["value"]["type"], "integer");
    assert_eq!(row["cells"]["col-volume"]["value"]["integer"], 1234567);
    assert_eq!(row["cells"]["col-volume"]["value"]["unit"], "shares");
    assert_eq!(row["cells"]["col-pb"]["value"]["type"], "missing");

    // A row without the industry string keeps the row but reports a missing
    // cell rather than an invented empty string value.
    let second = &rows[1];
    assert_eq!(second["instrumentId"], "US.MSFT");
    assert_eq!(second["cells"]["col-industry"]["value"]["type"], "missing");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:168
/// TestEmbeddedProviderRejectsFutuCatalogScreenWith409
///
/// A definition pinned to the Futu catalog must not be answered by the embedded
/// provider, so an old preset cannot silently switch providers.
#[test]
fn embedded_screen_rejects_a_futu_catalog_definition() {
    // No queued response: a helper call would block the join, so returning
    // before the reader proves the gate runs first.
    let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let mut definition = embedded_definition("yfinance", "US");
    definition["catalogVersion"] = json!(FUTU_CATALOG);
    let error = port
        .query(&screen_query(definition, 0, 25))
        .expect_err("futu catalog must fail closed");
    assert!(
        matches!(error, ResearchScreenWritePortError::Capability(_)),
        "error = {error:?}"
    );
    let _ = fixture;
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:186
/// TestEmbeddedProviderRejectsNonExecutableScreenShapes
///
/// Absolute-value sorts and multi-interval conditions have no provider-neutral
/// helper form, so they stay capability-unavailable instead of being silently
/// approximated.
#[test]
fn embedded_screen_rejects_shapes_the_helper_cannot_execute() {
    for direction in ["abs_desc", "abs_asc"] {
        let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
        let port = screen_port(
            Some(fixture.client.clone()),
            jftrade_settings::MarketDataProvider::Yfinance,
            true,
        );
        let mut definition = embedded_definition("yfinance", "US");
        definition["sorts"][0]["direction"] = json!(direction);
        let error = port
            .query(&screen_query(definition, 0, 25))
            .expect_err("absolute sorts are not executable");
        assert!(
            matches!(error, ResearchScreenWritePortError::Capability(ref m)
                if m.contains("direction")),
            "direction {direction} => {error:?}"
        );
        let _ = fixture;
    }

    let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let mut definition = embedded_definition("yfinance", "US");
    definition["conditions"][0]["value"] =
        json!({"intervals": [{"min": 1.0, "max": 2.0}]});
    let error = port
        .query(&screen_query(definition, 0, 25))
        .expect_err("multi-interval conditions are not executable");
    // The multi-interval draft has no provider-neutral form: it must be
    // rejected as an unsupported shape, not folded into an empty-bounds
    // condition error, so the message has to name the interval form.
    assert!(
        matches!(error, ResearchScreenWritePortError::Capability(ref message)
            if message.contains("multi-interval")),
        "error = {error:?}"
    );
    let _ = fixture;
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:214
/// TestEmbeddedProviderMapsScreenCapabilityErrors
///
/// A helper capability rejection becomes the broker capability contract, while
/// the warming sentinel keeps its own identity so the transport answers 503.
#[test]
fn embedded_screen_maps_capability_and_lifecycle_errors() {
    let fixture = ScreenFixture::new(
        "409 Conflict",
        r#"{"error":{"code":"CAPABILITY_UNSUPPORTED","message":"stock screen market HK"}}"#,
    );
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let error = port
        .query(&screen_query(embedded_definition("yfinance", "US"), 0, 25))
        .expect_err("unsupported market");
    assert!(
        matches!(error, ResearchScreenWritePortError::Capability(ref m)
            if m.contains("stock screen market")),
        "error = {error:?}"
    );
    let _ = fixture.join();

    let fixture = ScreenFixture::new(
        "503 Service Unavailable",
        r#"{"error":{"code":"AKSHARE_RUNTIME_WARMING","message":"runtime loading"}}"#,
    );
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let error = port
        .query(&screen_query(embedded_definition("yfinance", "US"), 0, 25))
        .expect_err("warming helper");
    assert!(
        matches!(error, ResearchScreenWritePortError::ProviderWarming),
        "error = {error:?}"
    );
    let _ = fixture.join();
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:236
/// TestEmbeddedScreenDecodesMapDefinitionAndDefaultsPaging
///
/// A definition handed in as a map (not the typed struct) still executes, and
/// an empty page keeps offset 0 with the request limit. The Go defaulting of
/// pageSize to 100 happens in the service before the facade, so this asserts
/// the request envelope the facade owns.
#[test]
fn embedded_screen_accepts_a_map_definition_and_defaults_paging() {
    let fixture = ScreenFixture::new(
        "200 OK",
        r#"{"entries":[],"total":0,"has_more":false,"as_of":"2026-08-15T20:00:00Z"}"#,
    );
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Yfinance,
        true,
    );
    let query = screen_query(embedded_definition("yfinance", "US"), 0, 100);
    let result = port.query(&query).expect("map definition query");
    let (_, payload) = fixture.join();
    let sent: Value = serde_json::from_str(&payload).expect("helper request body");
    assert_eq!(sent["market"], "US");
    assert_eq!(sent["offset"], 0);
    assert_eq!(sent["limit"], 100);
    assert_eq!(result["hasMore"], false);
    // No next offset from the provider means no cursor is invented, matching
    // the flat result the console expects.
    assert!(result.get("nextOffset").is_none());
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_screen_test.go:267
/// TestEmbeddedProviderServesUSScreenViaAkshare
///
/// AKShare can serve the embedded catalog for a US screen; the facade keys on
/// the active provider rather than the definition's market.
#[test]
fn embedded_screen_serves_a_us_screen_via_akshare() {
    let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
    let port = screen_port(
        Some(fixture.client.clone()),
        jftrade_settings::MarketDataProvider::Akshare,
        true,
    );
    let result = port
        .query(&screen_query(embedded_definition("akshare", "US"), 0, 25))
        .expect("akshare US screen");
    let (head, payload) = fixture.join();
    assert!(
        head.starts_with("POST /providers/akshare/screen "),
        "request = {head}"
    );
    let sent: Value = serde_json::from_str(&payload).expect("helper request body");
    assert_eq!(sent["market"], "US");
    assert_eq!(result["provider"]["brokerId"], "akshare");
    assert_eq!(result["entries"].as_array().expect("rows").len(), 2);
}

/// The catalog gate also protects the reverse direction: a Futu-pinned
/// definition must not be served even when AKShare is the active provider.
#[test]
fn embedded_screen_rejects_futu_catalog_for_every_embedded_provider() {
    for (provider, broker) in [
        (jftrade_settings::MarketDataProvider::Yfinance, "yfinance"),
        (jftrade_settings::MarketDataProvider::Akshare, "akshare"),
    ] {
        let fixture = ScreenFixture::new("200 OK", SCREEN_ROWS);
        let port = screen_port(Some(fixture.client.clone()), provider, true);
        let mut definition = embedded_definition(broker, "US");
        definition["catalogVersion"] = json!(FUTU_CATALOG);
        let error = port
            .query(&screen_query(definition, 0, 25))
            .expect_err("futu catalog must fail closed");
        assert!(
            matches!(error, ResearchScreenWritePortError::Capability(ref m)
                if m.contains("requires the futu broker")),
            "provider {provider:?} => {error:?}"
        );
        let _ = fixture;
    }
}
