//! Unit parity tests for the trade request parser and its query helpers.

use super::*;

#[test]
// Parity: internal/api/trading/execution_test.go:175 TestTradingQueryHelpersNormalizeAndValidate
// Verifies trade query parsing, path splitting, market label mapping and query parameter validation
fn trade_query_parses_symbol_and_optional_helpers() {
    let req = TradeRequest::parse("/api/v1/brokers/futu/max-trade-quantity", "symbol=US.AAPL&orderType=NORMAL&price=150.5&tradingEnvironment=simulate").expect("parse request");
    assert_eq!(req.broker_id, "futu");
    assert_eq!(req.resource, "max-trade-quantity");
    assert_eq!(req.environment_code().unwrap(), Some(0));
    assert_eq!(req.query.get_first("price"), Some("150.5"));
    assert_eq!(trade_account_market_label(21), Some("CN"));
    assert_eq!(normalize_trade_account_market(" sh "), "CN");
}

#[test]
// Parity: go:452dea11:internal/api/trading/execution_test.go:175 TestTradingQueryHelpersNormalizeAndValidate
fn trade_query_normalizes_scope_merges_aliases_and_treats_blank_optionals_as_absent() {
    let history = TradeRequest::parse("/api/v1/brokers/futu/orders", "scope=%20history%20")
        .expect("parse history scope");
    assert!(history.history_scope().expect("history scope"));
    let current =
        TradeRequest::parse("/api/v1/brokers/futu/orders", "scope=current").expect("parse");
    assert!(!current.history_scope().expect("current scope"));
    let defaulted = TradeRequest::parse("/api/v1/brokers/futu/orders", "").expect("parse");
    assert!(!defaulted.history_scope().expect("default scope"));
    let invalid =
        TradeRequest::parse("/api/v1/brokers/futu/orders", "scope=invalid").expect("parse");
    assert_eq!(
        invalid.history_scope().expect_err("invalid scope"),
        "query parameter scope is invalid"
    );

    let fees = TradeRequest::parse(
        "/api/v1/brokers/futu/order-fees",
        "orderIdEx=ord-1&orderIdExList=ord-2,ord-1",
    )
    .expect("parse");
    assert_eq!(
        fees.order_id_ex_list().expect("merged order ids"),
        vec!["ord-1".to_owned(), "ord-2".to_owned()]
    );

    let securities = TradeRequest::parse(
        "/api/v1/brokers/futu/securities",
        "symbol=US.AAPL&symbols=US.MSFT,%20US.AAPL",
    )
    .expect("parse");
    assert_eq!(
        securities.securities().expect("merged securities"),
        vec![
            TradeSecurity {
                market: 11,
                code: "AAPL".to_owned(),
            },
            TradeSecurity {
                market: 11,
                code: "MSFT".to_owned(),
            },
        ]
    );

    let typed = TradeRequest::parse(
        "/api/v1/brokers/futu/max-trade-qtys",
        "symbol=US.AAPL&orderType=LIMIT&price=%20100.5%20&adjustSideAndLimit=%201.25%20&positionId=%2042%20&session=%20RTH%20&orderIdEx=%20order-1%20",
    )
    .expect("parse");
    let (_, code, sec_market) = typed.max_trade_symbol().expect("symbol");
    let parsed = typed
        .max_trade_quantity_request(trade_header(1, 42, 2), code, sec_market)
        .expect("typed request");
    assert_eq!(parsed.price, 100.5);
    assert_eq!(parsed.adjust_side_and_limit, Some(1.25));
    assert_eq!(parsed.position_id, Some(42));
    assert_eq!(parsed.session, Some(1));
    assert_eq!(parsed.order_id_ex.as_deref(), Some("order-1"));

    let blank = TradeRequest::parse(
        "/api/v1/brokers/futu/max-trade-qtys",
        "symbol=US.AAPL&orderType=LIMIT&price=100&adjustSideAndLimit=%20&positionId=%20&session=%20&orderIdEx=%20",
    )
    .expect("parse");
    let (_, code, sec_market) = blank.max_trade_symbol().expect("symbol");
    let parsed = blank
        .max_trade_quantity_request(trade_header(1, 42, 2), code, sec_market)
        .expect("blank optionals");
    assert_eq!(
        (
            parsed.adjust_side_and_limit,
            parsed.position_id,
            parsed.session,
            parsed.order_id_ex,
        ),
        (None, None, None, None)
    );

    let invalid_float = TradeRequest::parse(
        "/api/v1/brokers/futu/max-trade-qtys",
        "symbol=US.AAPL&orderType=LIMIT&price=100&adjustSideAndLimit=bad",
    )
    .expect("parse");
    let (_, code, sec_market) = invalid_float.max_trade_symbol().expect("symbol");
    assert_eq!(
        invalid_float
            .max_trade_quantity_request(trade_header(1, 42, 2), code, sec_market)
            .expect_err("invalid float"),
        "query parameter adjustSideAndLimit is invalid"
    );

    let invalid_uint = TradeRequest::parse(
        "/api/v1/brokers/futu/max-trade-qtys",
        "symbol=US.AAPL&orderType=LIMIT&price=100&positionId=bad",
    )
    .expect("parse");
    let (_, code, sec_market) = invalid_uint.max_trade_symbol().expect("symbol");
    assert_eq!(
        invalid_uint
            .max_trade_quantity_request(trade_header(1, 42, 2), code, sec_market)
            .expect_err("invalid uint"),
        "query parameter positionId is invalid"
    );
}

#[test]
// Parity: go:452dea11:internal/trading/broker_test.go:735 TestNormalizeSymbolsAndRuntimeDefaults
// An omitted market resolves from the configured Futu default trade market;
// an explicit market always wins and a missing/blank default keeps HK.
fn trade_request_applies_configured_default_market_only_when_omitted() {
    let defaulted = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/margin-ratios",
        "accountId=42&symbol=AAPL",
        Some("US"),
    )
    .expect("request");
    assert_eq!(defaulted.market_label(), "US");
    let securities = defaulted.securities().expect("securities");
    assert_eq!(securities.len(), 1);
    assert_eq!(securities[0].market, 11, "US quote market code");
    assert_eq!(securities[0].code, "AAPL");

    let explicit = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/margin-ratios",
        "accountId=42&market=HK&symbol=BAD",
        Some("US"),
    )
    .expect("request");
    assert_eq!(explicit.market_label(), "HK");
    let securities = explicit.securities().expect("securities");
    assert_eq!(securities[0].market, 1, "HK quote market code");

    for default in [None, Some(""), Some("   ")] {
        let fallback = TradeRequest::parse_with_default_market(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=42&symbol=AAPL",
            default,
        )
        .expect("request");
        assert_eq!(fallback.market_label(), "HK");
        assert_eq!(fallback.securities().expect("securities")[0].market, 1);
    }

    let empty_query = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/orders",
        "",
        Some("US"),
    )
    .expect("request");
    assert_eq!(empty_query.market_label(), "US");

    let blank_market = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/margin-ratios",
        "accountId=42&market=%20&symbol=AAPL",
        Some("US"),
    )
    .expect("request");
    assert_eq!(blank_market.securities().expect("securities")[0].market, 11);

    // A blank query market counts as omitted everywhere: without a
    // configured default it keeps the historical HK fallback instead of
    // surfacing an empty market name.
    let blank_no_default = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/margin-ratios",
        "accountId=42&market=%20&symbol=AAPL",
        None,
    )
    .expect("request");
    assert_eq!(blank_no_default.market_label(), "HK");
    assert_eq!(
        blank_no_default.securities().expect("securities")[0].market,
        1
    );

    let conflict = TradeRequest::parse_with_default_market(
        "/api/v1/brokers/futu/margin-ratios",
        "accountId=42&market=HK&symbol=US.AAPL",
        Some("US"),
    )
    .expect("request");
    assert!(
        conflict.securities().is_err(),
        "a qualified symbol contradicting the explicit market stays rejected"
    );

    assert!(
        TradeRequest::parse_with_default_market(
            "/api/v1/brokers/futu/margin-ratios",
            "accountId=%FF",
            Some("US"),
        )
        .is_err(),
        "malformed query encoding stays a client error"
    );

    let portfolio = TradeRequest::parse_with_prefix_and_default_market(
        "/api/v1/portfolio/futu/summary",
        "accountId=42",
        "/api/v1/portfolio/",
        Some("US"),
    )
    .expect("request");
    assert_eq!(portfolio.broker_id, "futu");
    assert_eq!(portfolio.market_label(), "US");
}
