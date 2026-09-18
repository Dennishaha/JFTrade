use super::*;

#[test]
fn test_board_kind_defaults_empty_to_industry() {
    // Parity: internal/marketdata/rankings_facade_test.go:193 TestServiceIndustriesDefaultsEmptyKindToIndustry
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
