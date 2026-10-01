use super::*;
use crate::product::ResearchReadSnapshotPort;
use std::sync::Arc;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_production_ports::ProductionResearchPort;
use jftrade_integration_marketdata_helper::HelperClient;
use std::io::{Read, Write};
use std::net::TcpListener as StdTcpListener;
use std::time::Duration;
use serde_json::json;

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:97
/// TestProviderCorporateActionProjectionFormatsStatements
///
/// Go renders a plain-language statement per event: dividends use
/// "每股派息 <amount>", splits use "1 拆 <ratio>", an amount-less dividend
/// omits the statement entirely, and the envelope keeps the embedded
/// provider attribution with `total`/`hasMore`.
#[test]
fn corporate_action_projection_formats_statements() {
    let payload = json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "source": "yfinance-actions",
        "events": [
            {"kind": "dividend", "ex_date": "2026-08-10", "amount": 0.5},
            {"kind": "split", "ex_date": "2026-08-01", "ratio": 4},
            {"kind": "dividend", "ex_date": "2026-07-01"}
        ]
    });
    let result = project_research_payload(
        "corporate-actions",
        payload,
        "US",
        "AAPL",
        "yfinance",
        None,
    )
    .expect("project corporate actions");
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 3);
    assert_eq!(entries[0]["statement"], "每股派息 0.5");
    assert_eq!(entries[0]["exDate"], "2026-08-10");
    assert_eq!(entries[0]["kind"], "dividend");
    assert_eq!(entries[1]["statement"], "1 拆 4");
    assert!(entries[2].get("statement").is_none());
    assert_eq!(result["total"], 3);
    assert_eq!(result["hasMore"], false);
    assert_eq!(
        result["provider"]["selectionReason"],
        "embedded-market-data-provider"
    );
    assert_eq!(result["provider"]["brokerId"], "yfinance");
    assert_eq!(result["provider"]["featureId"], "research.corporate_actions");
    assert_eq!(result["metadata"]["source"], "yfinance-actions");
}

/// Go's dividend projection only renders a statement when an amount is
/// present, and every non-null amount is rendered with Go's shortest
/// round-trip float formatting.
#[test]
fn corporate_action_statements_use_go_number_rendering() {
    for (amount, expected) in [(0.5_f64, "每股派息 0.5"), (1.0, "每股派息 1")] {
        let payload = json!({
            "market": "US",
            "symbol": "AAPL",
            "instrument_id": "US.AAPL",
            "source": "yfinance-actions",
            "events": [{"kind": "dividend", "ex_date": "2026-08-10", "amount": amount}]
        });
        let result = project_research_payload(
            "corporate-actions",
            payload,
            "US",
            "AAPL",
            "yfinance",
            None,
        )
        .expect("project corporate actions");
        assert_eq!(result["entries"][0]["statement"], expected);
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:371
/// TestProviderCompanyProfileProjectionMapsFrontendKeys
///
/// Grouped profile fields flatten into the entry stream the console reads: a
/// `{fieldType:"title",name}` row opens a group and `{fieldType:"text",name,
/// value}` rows fill it. Fully empty rows and title-less groups keep the same
/// treatment as the embedded facade.
#[test]
fn provider_company_profile_projection_maps_frontend_keys() {
    let payload = json!({
        "instrument_id": "US.AAPL",
        "market": "US",
        "symbol": "AAPL",
        "source": "yfinance-profile",
        "groups": [
            {
                "title": "公司概要",
                "fields": [
                    {"name": "行业", "value": "消费电子"},
                    {"name": "", "value": ""}
                ]
            },
            {"title": "", "fields": [{"name": "员工数", "value": "164000"}]}
        ]
    });
    let result = project_research_payload("profile", payload, "US", "AAPL", "yfinance", None)
        .expect("profile projection");
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 3);
    assert_eq!(entries[0]["fieldType"], "title");
    assert_eq!(entries[0]["name"], "公司概要");
    assert!(entries[0].get("value").is_none());
    assert_eq!(entries[1]["fieldType"], "text");
    assert_eq!(entries[1]["name"], "行业");
    assert_eq!(entries[1]["value"], "消费电子");
    assert_eq!(entries[2]["name"], "员工数");
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "US.AAPL");
    assert_eq!(result["resolvedInstrument"]["code"], "AAPL");
    assert_eq!(result["metadata"]["source"], "yfinance-profile");
    assert_eq!(result["total"], 3);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:421
/// TestProviderFinancialStatementsProjectionMapsFrontendKeys
///
/// Statement rows project `metadata.structureList` plus one entry per period
/// whose `itemList` carries the field cells; `yoy`/`qoq` stay absent when the
/// feed publishes no comparison, and an empty period still projects an empty
/// list.
#[test]
fn provider_financial_statements_projection_maps_frontend_keys() {
    let payload = json!({
        "instrument_id": "US.AAPL",
        "market": "US",
        "symbol": "AAPL",
        "statement": "income",
        "currency": "USD",
        "source": "yfinance-financials",
        "fields": [
            {"field_id": "total_revenue", "display_name": "总营收"},
            {"field_id": "net_income", "display_name": "净利润"}
        ],
        "periods": [
            {
                "period_text": "2025财年",
                "values": {
                    "total_revenue": {"data": 416161000000_i64, "yoy": 0.02},
                    "net_income": {"data": 112010000000_i64}
                }
            },
            {"period_text": "2024财年", "values": {}}
        ]
    });
    let result = project_research_payload(
        "financials",
        payload,
        "US",
        "AAPL",
        "yfinance",
        Some("income"),
    )
    .expect("financials projection");
    let structure = result["metadata"]["structureList"]
        .as_array()
        .expect("structureList");
    assert_eq!(structure.len(), 2);
    assert_eq!(structure[0]["fieldId"], "total_revenue");
    assert_eq!(structure[0]["displayName"], "总营收");

    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[0]["periodText"], "2025财年");
    assert_eq!(entries[0]["currencyCode"], "USD");
    let items = entries[0]["itemList"].as_array().expect("itemList");
    assert_eq!(items.len(), 2);
    assert_eq!(items[0]["fieldId"], "total_revenue");
    assert_eq!(items[0]["data"], json!(416161000000_i64));
    assert_eq!(items[0]["yoy"], json!(0.02));
    assert!(items[0].get("qoq").is_none(), "nil qoq must be omitted");
    assert!(items[1].get("yoy").is_none(), "nil yoy must be omitted");
    assert_eq!(entries[1]["itemList"], json!([]));
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:486
/// TestProviderAnalystConsensusProjectionMapsFrontendKeys
///
/// The consensus projects exactly one entry with the rating dashboard's keys;
/// distribution buckets the feed did not publish stay absent.
#[test]
fn provider_analyst_consensus_projection_maps_frontend_keys() {
    let payload = json!({
        "instrument_id": "HK.00700",
        "market": "HK",
        "symbol": "00700",
        "source": "yfinance-analyst",
        "rating": 4,
        "analyst_count": 38,
        "target_price": {"lowest": 520, "average": 700.5, "highest": 860},
        "distribution": {"strong_buy": 45, "buy": 30, "hold": 20},
        "update_time": "2026-08-15"
    });
    let result = project_research_payload("analyst", payload, "HK", "00700", "yfinance", None)
        .expect("analyst projection");
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 1);
    let entry = &entries[0];
    assert_eq!(entry["rating"], json!(4));
    assert_eq!(entry["analystCount"], json!(38));
    assert_eq!(entry["lowest"], json!(520));
    assert_eq!(entry["average"], json!(700.5));
    assert_eq!(entry["highest"], json!(860));
    assert_eq!(entry["strongBuy"], json!(45));
    assert_eq!(entry["buy"], json!(30));
    assert_eq!(entry["hold"], json!(20));
    assert_eq!(entry["updateTimeStr"], "2026-08-15");
    for key in ["underperform", "sell"] {
        assert!(
            entry.get(key).is_none(),
            "nil bucket {key} must be omitted: {entry}"
        );
    }
    assert_eq!(result["total"], 1);
    assert_eq!(result["hasMore"], false);
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:530
/// TestProviderAnalystConsensusProjectionOmitsAbsentSections
#[test]
fn provider_analyst_consensus_projection_omits_absent_sections() {
    let payload = json!({
        "instrument_id": "US.AAPL",
        "market": "US",
        "symbol": "AAPL",
        "source": "yfinance-analyst"
    });
    let result = project_research_payload("analyst", payload, "US", "AAPL", "yfinance", None)
        .expect("consensus projection");
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 1);
    assert!(
        entries[0].as_object().expect("entry").is_empty(),
        "fully null consensus must project one empty entry: {entries:?}"
    );
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:547
/// TestProviderOwnershipProjectionMapsFrontendKeys
///
/// Ownership splits groups into the metadata lists the holder panels read:
/// `mainHolderInfoList` for major holders and `holderTypeInfoList` for holder
/// types, while the entry stream itself stays empty.
#[test]
fn provider_ownership_projection_maps_frontend_keys() {
    let payload = json!({
        "instrument_id": "SH.600519",
        "market": "SH",
        "symbol": "600519",
        "source": "akshare-ownership",
        "groups": [
            {
                "kind": "major_holders",
                "static_date": "2026-06-30",
                "items": [
                    {"name": "中国贵州茅台酒厂(集团)", "holder_pct": 54.07},
                    {"name": "香港中央结算有限公司"}
                ]
            },
            {
                "kind": "holder_types",
                "items": [
                    {"name": "国有法人", "holder_pct": 60.1},
                    {"name": "流通A股", "holder_pct": 39.9}
                ]
            }
        ]
    });
    let result = project_research_payload("ownership", payload, "SH", "600519", "akshare", None)
        .expect("ownership projection");
    assert!(result["entries"].as_array().expect("entries").is_empty());
    assert_eq!(result["total"], 0);

    let main = result["metadata"]["mainHolderInfoList"]
        .as_array()
        .expect("mainHolderInfoList");
    assert_eq!(main.len(), 1);
    assert_eq!(main[0]["staticDateStr"], "2026-06-30");
    let main_items = main[0]["itemList"].as_array().expect("itemList");
    assert_eq!(main_items.len(), 2);
    assert_eq!(main_items[0]["name"], "中国贵州茅台酒厂(集团)");
    assert_eq!(main_items[0]["holderPct"], json!(54.07));
    assert!(
        main_items[1].get("holderPct").is_none(),
        "nil holderPct must be omitted: {}",
        main_items[1]
    );

    let holder_types = result["metadata"]["holderTypeInfoList"]
        .as_array()
        .expect("holderTypeInfoList");
    assert_eq!(holder_types.len(), 1);
    assert!(holder_types[0].get("staticDateStr").is_none());
    let type_items = holder_types[0]["itemList"].as_array().expect("itemList");
    assert_eq!(type_items.len(), 2);
    assert_eq!(type_items[1]["name"], "流通A股");
}

/// Parity: go:452dea11:internal/productfeatures/provider_projection_test.go:186
/// TestEmbeddedResearchInstrumentDerivesMarketAndSymbol
///
/// The helper request parser is the Rust owner of Go's
/// `embeddedResearchInstrument`: an explicit market wins, a `MARKET.CODE`
/// instrument prefix derives it otherwise, and an instrument without either
/// must not be served.
#[test]
fn embedded_research_instrument_derives_market_and_symbol() {
    let (operation, market, symbol, _) =
        research_helper_request("/api/v1/research/instruments/us.aapl", "").expect("explicit market");
    assert_eq!(operation, "profile");
    assert_eq!(market, "US");
    assert_eq!(symbol, "AAPL");

    let (_, market, symbol, _) =
        research_helper_request("/api/v1/research/instruments/hk.00700", "").expect("prefix market");
    assert_eq!(market, "HK");
    assert_eq!(symbol, "00700");

    assert!(matches!(
        research_helper_request("/api/v1/research/instruments/AAPL", ""),
        Err(ResearchReadSnapshotError::Invalid(_))
    ));
    assert!(matches!(
        research_helper_request("/api/v1/research/instruments/", ""),
        Err(ResearchReadSnapshotError::Invalid(_))
    ));
}


/// Loopback helper fixture for the company-research routes.
///
/// The production port performs its helper call on a nested Tokio runtime from
/// a blocking thread, so the peer lives on a plain OS thread and is joined by
/// the test after the call returns.
struct CompanyResearchFixture {
    client: HelperClient,
    server: std::thread::JoinHandle<Vec<String>>,
}

impl CompanyResearchFixture {
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

    fn ok(body: &str) -> Self {
        Self::new(vec![("200 OK".to_owned(), body.to_owned())])
    }

    fn join(self) -> Vec<String> {
        self.server.join().expect("server")
    }
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:51
/// TestEmbeddedProviderServesCompanyResearchDefaultOperations
///
/// Each company-research feature has one embedded operation (profile,
/// statements, consensus, overview). Serving them forwards the instrument and
/// resolves it for the caller, with no broker hop.
#[test]
fn company_research_default_operations_project_on_the_wire() {
    // profile -> /providers/yfinance/profile/US/AAPL
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","market":"US","symbol":"AAPL","source":"yfinance-profile","groups":[{"title":"Company","fields":[{"name":"Sector","value":"Technology"}]}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "operation=profile")
        .expect("profile");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/yfinance/profile/US/AAPL "),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["fieldType"], "title");
    assert_eq!(result["entries"][1]["name"], "Sector");
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "US.AAPL");
    assert_eq!(result["provider"]["featureId"], "research.instrument");

    // statements -> /providers/yfinance/financials/US/AAPL
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","statement":"income","source":"yfinance-financials","fields":[{"field_id":"revenue","display_name":"Revenue"}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read(
            "/api/v1/research/financials/US.AAPL",
            "operation=statements&statement=income",
        )
        .expect("financials");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/yfinance/financials/US/AAPL?statement=income "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["metadata"]["structureList"][0]["fieldId"], "revenue");
    assert_eq!(result["provider"]["featureId"], "research.financials");

    // consensus -> /providers/yfinance/analyst/US/AAPL
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","source":"yfinance-analyst","rating":4}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/analyst/US.AAPL", "operation=consensus")
        .expect("analyst");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/yfinance/analyst/US/AAPL "),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["entries"][0]["rating"], 4);
    assert_eq!(result["provider"]["featureId"], "research.analyst");

    // overview -> /providers/yfinance/ownership/US/AAPL
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","source":"yfinance-ownership","groups":[{"kind":"major_holders","items":[{"name":"Vanguard","holder_pct":8.6}]}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/ownership/US.AAPL", "operation=overview")
        .expect("ownership");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/yfinance/ownership/US/AAPL "),
        "request = {}",
        requests[0]
    );
    assert_eq!(
        result["metadata"]["mainHolderInfoList"][0]["itemList"][0]["name"],
        "Vanguard"
    );
    assert_eq!(
        result["metadata"]["mainHolderInfoList"][0]["itemList"][0]["holderPct"],
        8.6
    );
    assert_eq!(result["provider"]["featureId"], "research.ownership");
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:57 TestRuntimeCompanyResearchForwarding
/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:148
/// TestEmbeddedProviderCompanyResearchForwardsMarketSymbolAndStatement
///
// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:69 TestProviderCompanyProfileConvertsGroupsAndSkipsEmptyFields
#[test]
fn yfinance_profile_route_converts_groups_and_currency() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","market":"US","symbol":"AAPL","currency":" USD ","groups":[{"title":" Company ","fields":[{"name":" Sector ","value":" Technology "},{"name":"","value":""}]},{"title":"Listing","fields":[{"name":"Exchange","value":"NASDAQ"}]}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "operation=profile")
        .expect("profile");
    let requests = fixture.join();
    assert!(requests[0].starts_with("GET /providers/yfinance/profile/US/AAPL "));
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "US.AAPL");
    assert_eq!(result["metadata"]["source"], "yfinance-profile");
    assert_eq!(result["entries"].as_array().map(Vec::len), Some(4));
    assert_eq!(result["entries"][0]["name"], "Company");
    assert_eq!(result["entries"][1]["name"], "Sector");
    assert_eq!(result["entries"][1]["value"], "Technology");
    assert_eq!(result["entries"][2]["name"], "Listing");
    assert_eq!(result["entries"][3]["value"], "NASDAQ");
}

// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:100 TestProviderFinancialStatementsConvertsFieldsPeriodsAndNullableRatios
#[test]
fn yfinance_financials_route_converts_periods_and_nullable_ratios() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"HK.00700","statement":"income","currency":null,"fields":[{"field_id":"revenue","display_name":"Revenue"}],"periods":[{"period_text":"2025FY","values":{"revenue":{"data":660000000000.0,"yoy":8.4,"qoq":null}}},{"period_text":"2024FY","values":{"revenue":{"data":609000000000.0,"yoy":null,"qoq":null}}}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/financials/HK.00700", "operation=statements&statement=income")
        .expect("financials");
    let requests = fixture.join();
    assert!(requests[0].starts_with(
        "GET /providers/yfinance/financials/HK/00700?statement=income "
    ));
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "HK.00700");
    assert_eq!(result["metadata"]["structureList"][0]["fieldId"], "revenue");
    assert_eq!(result["entries"].as_array().map(Vec::len), Some(2));
    assert_eq!(result["entries"][0]["itemList"][0]["data"], 660000000000.0);
    assert_eq!(result["entries"][0]["itemList"][0]["yoy"], 8.4);
    assert!(result["entries"][0]["itemList"][0].get("qoq").is_none());
    assert!(result["entries"][1]["itemList"][0].get("yoy").is_none());
}

// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:134 TestProviderAnalystConsensusConvertsRatingTargetAndDistribution
#[test]
fn yfinance_analyst_route_converts_rating_targets_and_distribution() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","rating":4,"analyst_count":38,"target_price":{"lowest":180.5,"average":240.1,"highest":300},"distribution":{"strong_buy":42.1,"buy":31.6,"hold":21.1,"underperform":5.2,"sell":0},"update_time":" 2026-08-15T20:00:00Z "}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/analyst/US.AAPL", "operation=consensus")
        .expect("analyst consensus");
    let requests = fixture.join();
    assert!(requests[0].starts_with("GET /providers/yfinance/analyst/US/AAPL "));
    let entry = &result["entries"][0];
    assert_eq!(entry["rating"], 4);
    assert_eq!(entry["analystCount"], 38);
    assert_eq!(entry["average"], 240.1);
    assert_eq!(entry["strongBuy"], 42.1);
    assert_eq!(entry["sell"], 0.0);
    assert_eq!(entry["updateTimeStr"], "2026-08-15T20:00:00Z");
}

// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:161 TestProviderOwnershipConvertsGroupsAndValidatesKind
#[test]
fn yfinance_ownership_route_converts_groups_and_nullable_dates() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","groups":[{"kind":"major_holders","static_date":"2026-06-30","items":[{"name":"Vanguard","holder_pct":8.6}]},{"kind":"holder_types","static_date":null,"items":[{"name":"Institutions","holder_pct":61.2}]}]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read("/api/v1/research/ownership/US.AAPL", "operation=overview")
        .expect("ownership");
    let requests = fixture.join();
    assert!(requests[0].starts_with("GET /providers/yfinance/ownership/US/AAPL "));
    assert_eq!(result["metadata"]["mainHolderInfoList"][0]["staticDateStr"], "2026-06-30");
    assert_eq!(result["metadata"]["mainHolderInfoList"][0]["itemList"][0]["name"], "Vanguard");
    assert_eq!(result["metadata"]["mainHolderInfoList"][0]["itemList"][0]["holderPct"], 8.6);
    assert_eq!(result["metadata"]["holderTypeInfoList"][0]["itemList"][0]["name"], "Institutions");
    assert_eq!(result["metadata"]["holderTypeInfoList"][0]["itemList"][0]["holderPct"], 61.2);
    assert!(result["metadata"]["holderTypeInfoList"][0].get("staticDateStr").is_none());
}

/// The financials read forwards the request's market, symbol, and statement
/// selection verbatim to the provider path.
#[test]
fn company_financials_forwards_market_symbol_and_statement() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"SH.600519","statement":"cashflow","source":"akshare-financials","fields":[]}"#,
    );
    let result = production_research_client(&fixture.client)
        .read(
            "/api/v1/research/financials/SH.600519",
            "operation=statements&statement=cashflow",
        )
        .expect("financials");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with(
            "GET /providers/yfinance/financials/SH/600519?statement=cashflow "
        ),
        "request = {}",
        requests[0]
    );
    assert_eq!(result["resolvedInstrument"]["instrumentId"], "SH.600519");
}

// Parity: go:452dea11:internal/marketdata/company_research_facade_test.go:109
// TestServiceFinancialStatementsValidatesStatementAndDefaultsToIncome.
#[test]
fn company_financials_reject_invalid_statement_before_helper_access() {
    let fixture = CompanyResearchFixture::new(Vec::new());
    let error = production_research_client(&fixture.client)
        .read(
            "/api/v1/research/financials/US.AAPL",
            "operation=statements&statement=annual",
        )
        .expect_err("annual is not a supported statement kind");
    assert!(matches!(error, ResearchReadSnapshotError::Invalid(_)));
    assert!(fixture.join().is_empty(), "invalid statement must not reach helper");
}

// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:192 TestProviderCompanyResearchRejectsUnsupportedMarketsWithoutSidecarCall
#[test]
fn yfinance_company_research_rejects_unsupported_markets_without_helper_calls() {
    for (path, operation) in [
        ("/api/v1/research/instruments/SH.600519", "profile"),
        ("/api/v1/research/financials/CN.SH.600519", "statements&statement=income"),
        ("/api/v1/research/analyst/SZ.000001", "consensus"),
        ("/api/v1/research/ownership/BJ.430047", "overview"),
    ] {
        let fixture = CompanyResearchFixture::new(Vec::new());
        let error = production_research_client(&fixture.client)
            .read(path, &format!("operation={operation}"))
            .expect_err("unsupported market must fail closed");
        assert!(matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "BROKER_CAPABILITY_UNAVAILABLE"
        ));
        assert!(fixture.join().is_empty(), "{path} reached helper");
    }
}

// Parity: go:452dea11:internal/integration/yfinance/provider_company_research_test.go:220 TestProviderCompanyResearchMapsSidecarUnsupportedMarket
#[test]
fn yfinance_profile_route_maps_sidecar_unsupported_market() {
    let fixture = CompanyResearchFixture::new(vec![(
        "400 Bad Request".to_owned(),
        r#"{"error":{"code":"unsupported_market","message":"HK profile is not covered"}}"#.to_owned(),
    )]);
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/HK.00700", "operation=profile")
        .expect_err("unsupported market");
    assert!(matches!(
        error,
        ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
            if code == "BROKER_CAPABILITY_UNAVAILABLE"
    ));
    assert_eq!(fixture.join().len(), 1);
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:170
/// TestEmbeddedProviderCompanyResearchAcceptsOmittedOperation
///
/// An absent operation falls back to the feature's embedded default instead of
/// being rejected.
#[test]
fn company_research_accepts_an_omitted_operation() {
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","market":"US","symbol":"AAPL","source":"yfinance-profile","groups":[]}"#,
    );
    production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "")
        .expect("profile without an explicit operation");
    let requests = fixture.join();
    assert!(
        requests[0].starts_with("GET /providers/yfinance/profile/US/AAPL "),
        "request = {}",
        requests[0]
    );
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:183
/// TestEmbeddedProviderRejectsNonDefaultCompanyOperations
///
/// Non-default operations for the embedded company-research features are
/// capability errors and never reach the helper.
// Parity: go:452dea11:internal/api/productfeatures/provider_research_routes_test.go:458 TestEmbeddedProviderCompanyResearchRejectsUnsupportedOperations
#[test]
fn company_research_rejects_non_default_operations() {
    for (path, operation) in [
        ("/api/v1/research/instruments/US.AAPL", "deep_dive"),
        ("/api/v1/research/financials/US.AAPL", "ratios"),
        ("/api/v1/research/analyst/US.AAPL", "estimate_trend"),
        ("/api/v1/research/ownership/US.AAPL", "history"),
    ] {
        let fixture = CompanyResearchFixture::new(Vec::new());
        let error = production_research_client(&fixture.client)
            .read(path, &format!("operation={operation}"))
            .expect_err("non-default operations must fail closed");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                    if code == "BROKER_CAPABILITY_UNAVAILABLE"
            ),
            "{path} {operation} => {error:?}"
        );
        // No queued response: the join only returns because no helper read ran.
        assert!(fixture.join().is_empty(), "{path} reached the helper");
    }
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:95 TestRuntimeCompanyResearchPropagatesError
/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:207
/// TestEmbeddedProviderPropagatesCompanyResearchCapabilityErrors
///
/// A helper capability rejection keeps its code, and the warming sentinel keeps
/// its identity so the transport can answer 503 instead of 409.
#[test]
fn company_research_propagates_capability_and_lifecycle_errors() {
    let fixture = CompanyResearchFixture::new(vec![(
        "409 Conflict".to_owned(),
        r#"{"error":{"code":"CAPABILITY_UNSUPPORTED","message":"analyst consensus unsupported"}}"#
            .to_owned(),
    )]);
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/analyst/US.AAPL", "operation=consensus")
        .expect_err("unsupported analyst consensus");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "CAPABILITY_UNSUPPORTED"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);

    let fixture = CompanyResearchFixture::new(vec![(
        "503 Service Unavailable".to_owned(),
        r#"{"error":{"code":"AKSHARE_RUNTIME_WARMING","message":"runtime loading"}}"#.to_owned(),
    )]);
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "operation=profile")
        .expect_err("warming profile");
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

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_company_forwarding_test.go:108 TestRuntimeCompanyResearchCapabilityUnsupported
/// Parity: go:452dea11:internal/productfeatures/provider_facade_company_test.go:226
/// TestEmbeddedProviderCompanyResearchStaysOnBrokerPathForFutu
///
/// With Futu active the embedded reader is never consulted for company
/// research; the route fails closed before any helper read.
#[test]
fn company_research_stays_off_the_helper_for_futu() {
    let fixture = CompanyResearchFixture::new(Vec::new());
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(true, true, true);
    let port = ProductionResearchPort {
        active_provider_state: state,
        helper: Some(fixture.client.clone()),
        trade_runtime: None,
    };
    let error = port
        .read("/api/v1/research/analyst/US.AAPL", "operation=consensus")
        .expect_err("Futu must keep company research on the broker path");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "BROKER_CAPABILITY_UNAVAILABLE"
        ),
        "error = {error:?}"
    );
    assert!(fixture.join().is_empty());
}

/// Build the production research port with a yfinance helper client, matching
/// the Go fixture's active descriptor.
fn production_research_client(
    client: &HelperClient,
) -> ProductionResearchPort {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Yfinance,
    )));
    state.set_readiness(true, false, false);
    ProductionResearchPort {
        active_provider_state: state,
        helper: Some(client.clone()),
        trade_runtime: None,
    }
}

/// The company-research projection is fail-closed on provider identity: a
/// helper payload whose instrument/market/symbol does not match the request is
/// a 502 instead of a silently mislabeled result.
///
/// This guard had no Rust test before this batch: removing it kept every other
/// company-research test green, so it is asserted here rather than assumed.
#[test]
fn company_research_rejects_provider_identity_drift() {
    // instrument_id belongs to a different instrument than the request.
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.MSFT","market":"US","symbol":"MSFT","source":"yfinance-profile","groups":[]}"#,
    );
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "operation=profile")
        .expect_err("identity drift must fail closed");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 502, ref code, .. }
                if code == "BAD_GATEWAY"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);

    // Profile's own market/symbol fields must agree with the request too.
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"US.AAPL","market":"US","symbol":"MSFT","source":"yfinance-profile","groups":[]}"#,
    );
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/instruments/US.AAPL", "operation=profile")
        .expect_err("symbol drift must fail closed");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 502, ref code, .. }
                if code == "BAD_GATEWAY"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);

    // A non-instrument operation is checked against the request identity as
    // well, so analyst/ownership reads cannot be relabeled either.
    let fixture = CompanyResearchFixture::ok(
        r#"{"instrument_id":"SH.600519","source":"yfinance-analyst","rating":4}"#,
    );
    let error = production_research_client(&fixture.client)
        .read("/api/v1/research/analyst/US.AAPL", "operation=consensus")
        .expect_err("cross-market identity drift must fail closed");
    assert!(
        matches!(
            error,
            ResearchReadSnapshotError::Failed { status: 502, ref code, .. }
                if code == "BAD_GATEWAY"
        ),
        "error = {error:?}"
    );
    assert_eq!(fixture.join().len(), 1);
}
