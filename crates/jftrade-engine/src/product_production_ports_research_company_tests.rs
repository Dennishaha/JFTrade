use super::*;
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
