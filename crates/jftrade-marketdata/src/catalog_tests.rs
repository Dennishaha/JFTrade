use super::*;
use rust_decimal::Decimal;

#[test]
fn market_rules_ssot_contains_all_core_markets_with_decimal_tick_sizes() {
    let markets = default_markets();
    assert_eq!(markets.len(), 5);

    let hk = find_market_rule("HK").expect("HK rule");
    assert_eq!(hk.code, "HK");
    assert_eq!(hk.resolved_market, "HK");
    assert_eq!(hk.precision.price, 3);
    assert_eq!(hk.precision.quote, 3);
    assert_eq!(hk.tick_size, Decimal::new(1, 3)); // 0.001
    assert_eq!(hk.quote_currency, "HKD");
    assert_eq!(hk.timezone, "Asia/Hong_Kong");
    assert!(!hk.requires_exchange_prefix);

    let us = find_market_rule("US").expect("US rule");
    assert_eq!(us.code, "US");
    assert_eq!(us.resolved_market, "US");
    assert_eq!(us.precision.price, 2);
    assert_eq!(us.tick_size, Decimal::new(1, 2)); // 0.01
    assert!(us.supports_extended_hours);

    let cn = find_market_rule("CN").expect("CN rule");
    assert_eq!(cn.code, "CN");
    assert_eq!(cn.display_name, "沪深");
    assert_eq!(cn.resolved_market, "CN");
    assert_eq!(cn.aliases, vec!["SH", "SZ", "CNSH", "CNSZ"]);
    assert!(cn.requires_exchange_prefix);
    assert_eq!(cn.child_markets, vec!["SH", "SZ"]);
    assert_eq!(cn.tick_size, Decimal::new(1, 2)); // 0.01

    let sh = find_market_rule("SH").expect("SH rule");
    assert_eq!(sh.code, "SH");
    assert_eq!(sh.resolved_market, "CN");
    assert_eq!(sh.parent_market, Some("CN".to_owned()));
    assert!(sh.requires_exchange_prefix);

    let sz = find_market_rule("SZ").expect("SZ rule");
    assert_eq!(sz.code, "SZ");
    assert_eq!(sz.resolved_market, "CN");
    assert_eq!(sz.parent_market, Some("CN".to_owned()));
    assert!(sz.requires_exchange_prefix);
}

#[test]
fn market_rule_to_api_value_preserves_expected_schema() {
    let cn = cn_market_rule();
    let api_val = cn.to_api_value();
    assert_eq!(api_val["code"], "CN");
    assert_eq!(api_val["market"], "CN");
    assert_eq!(api_val["resolvedMarket"], "CN");
    assert_eq!(api_val["displayName"], "沪深");
    assert_eq!(api_val["requiresExchangePrefix"], true);
    assert_eq!(api_val["tickSize"], 0.01);
    assert_eq!(api_val["precision"]["price"], 2);

    let hk = hk_market_rule();
    let hk_val = hk.to_api_value();
    assert_eq!(hk_val["tickSize"], 0.001);
}

#[test]
fn decimal_price_tick_alignment() {
    let hk = hk_market_rule();
    let aligned = hk.align_price_to_step(Decimal::new(3202347, 4)); // 320.2347
    assert_eq!(aligned, Decimal::new(320235, 3)); // 320.235

    let us = us_market_rule();
    let aligned_us = us.align_price_to_step(Decimal::new(150256, 3)); // 150.256
    assert_eq!(aligned_us, Decimal::new(15026, 2)); // 150.26
}

#[test]
fn normalize_instrument_handles_cn_market_and_prefixes() {
    // Qualified SH symbol
    let n1 = normalize_instrument(None, Some("SH.600519")).unwrap();
    assert_eq!(n1.market, "CN");
    assert_eq!(n1.resolved_market, "CN");
    assert_eq!(n1.prefix, "SH");
    assert_eq!(n1.code, "600519");
    assert_eq!(n1.symbol, "SH.600519");
    assert_eq!(n1.instrument_id, "SH.600519");

    // Qualified SZ symbol
    let n2 = normalize_instrument(None, Some("SZ.000001")).unwrap();
    assert_eq!(n2.market, "CN");
    assert_eq!(n2.resolved_market, "CN");
    assert_eq!(n2.prefix, "SZ");
    assert_eq!(n2.code, "000001");
    assert_eq!(n2.symbol, "SZ.000001");

    // Market CN with 6-prefix code -> infers SH
    let n3 = normalize_instrument(Some("CN"), Some("600519")).unwrap();
    assert_eq!(n3.market, "CN");
    assert_eq!(n3.resolved_market, "CN");
    assert_eq!(n3.prefix, "SH");
    assert_eq!(n3.code, "600519");
    assert_eq!(n3.symbol, "SH.600519");

    // Market CN with 0-prefix code -> infers SZ
    let n4 = normalize_instrument(Some("CN"), Some("000001")).unwrap();
    assert_eq!(n4.market, "CN");
    assert_eq!(n4.resolved_market, "CN");
    assert_eq!(n4.prefix, "SZ");
    assert_eq!(n4.code, "000001");
    assert_eq!(n4.symbol, "SZ.000001");

    // Market CN with 3-prefix code (ChiNext) -> infers SZ
    let n5 = normalize_instrument(Some("CN"), Some("300750")).unwrap();
    assert_eq!(n5.prefix, "SZ");
    assert_eq!(n5.symbol, "SZ.300750");

    // Market CN with already-qualified symbol
    let n6 = normalize_instrument(Some("CN"), Some("SH.600519")).unwrap();
    assert_eq!(n6.prefix, "SH");
    assert_eq!(n6.resolved_market, "CN");
    assert_eq!(n6.symbol, "SH.600519");

    // Legacy colon format
    let n7 = normalize_instrument(None, Some("cnsh:600519")).unwrap();
    assert_eq!(n7.prefix, "SH");
    assert_eq!(n7.resolved_market, "CN");
    assert_eq!(n7.symbol, "SH.600519");
}

#[test]
fn normalize_instrument_handles_us_and_hk() {
    let us = normalize_instrument(Some("us"), Some("aapl")).unwrap();
    assert_eq!(us.market, "US");
    assert_eq!(us.resolved_market, "US");
    assert_eq!(us.prefix, "US");
    assert_eq!(us.code, "AAPL");
    assert_eq!(us.symbol, "US.AAPL");

    let hk = normalize_instrument(Some("HK"), Some("700")).unwrap();
    assert_eq!(hk.market, "HK");
    assert_eq!(hk.resolved_market, "HK");
    assert_eq!(hk.prefix, "HK");
    assert_eq!(hk.code, "00700");
    assert_eq!(hk.symbol, "HK.00700");
}

#[test]
fn normalize_instrument_rejects_missing_inputs() {
    assert_eq!(
        normalize_instrument(None, None),
        Err(MarketCatalogError::MissingSymbolOrCode)
    );
    assert_eq!(
        normalize_instrument(Some("US"), None),
        Err(MarketCatalogError::MissingSymbolOrCode)
    );
    assert_eq!(
        normalize_instrument(Some("US"), Some("")),
        Err(MarketCatalogError::MissingSymbolOrCode)
    );
    assert_eq!(
        normalize_instrument(None, Some("")),
        Err(MarketCatalogError::MissingSymbolOrCode)
    );
}

#[test]
fn normalize_instrument_rejects_unsupported_market_and_market_mismatch() {
    assert_eq!(
        normalize_instrument(Some("INVALID"), Some("AAPL")),
        Err(MarketCatalogError::UnsupportedMarket("INVALID".to_owned()))
    );
    assert_eq!(
        normalize_instrument(None, Some("INVALID.AAPL")),
        Err(MarketCatalogError::UnsupportedMarket("INVALID".to_owned()))
    );
    assert_eq!(
        normalize_instrument(Some("US"), Some("SH.600519")),
        Err(MarketCatalogError::MarketMismatch(
            "US".to_owned(),
            "SH.600519".to_owned()
        ))
    );
    assert_eq!(
        normalize_instrument(Some("HK"), Some("US.AAPL")),
        Err(MarketCatalogError::MarketMismatch(
            "HK".to_owned(),
            "US.AAPL".to_owned()
        ))
    );

    // Explicit cross-exchange contradiction within CN must be rejected
    assert_eq!(
        normalize_instrument(Some("SH"), Some("SZ.000001")),
        Err(MarketCatalogError::MarketMismatch(
            "SH".to_owned(),
            "SZ.000001".to_owned()
        ))
    );
    assert_eq!(
        normalize_instrument(Some("SZ"), Some("SH.600519")),
        Err(MarketCatalogError::MarketMismatch(
            "SZ".to_owned(),
            "SH.600519".to_owned()
        ))
    );
    assert_eq!(
        normalize_instrument(Some("CNSH"), Some("SZ.000001")),
        Err(MarketCatalogError::MarketMismatch(
            "CNSH".to_owned(),
            "SZ.000001".to_owned()
        ))
    );
    assert_eq!(
        normalize_instrument(Some("CNSZ"), Some("SH.600519")),
        Err(MarketCatalogError::MarketMismatch(
            "CNSZ".to_owned(),
            "SH.600519".to_owned()
        ))
    );
}

#[test]
fn normalize_instrument_supports_suffix_formats_and_aliases() {
    // Suffix notation (CODE.EXCHANGE)
    let n1 = normalize_instrument(None, Some("600519.SH")).unwrap();
    assert_eq!(n1.market, "CN");
    assert_eq!(n1.resolved_market, "CN");
    assert_eq!(n1.prefix, "SH");
    assert_eq!(n1.code, "600519");
    assert_eq!(n1.symbol, "SH.600519");

    let n2 = normalize_instrument(None, Some("000001.SZ")).unwrap();
    assert_eq!(n2.market, "CN");
    assert_eq!(n2.prefix, "SZ");
    assert_eq!(n2.symbol, "SZ.000001");

    let n3 = normalize_instrument(None, Some("00700.HK")).unwrap();
    assert_eq!(n3.market, "HK");
    assert_eq!(n3.prefix, "HK");
    assert_eq!(n3.symbol, "HK.00700");

    let n4 = normalize_instrument(None, Some("AAPL.US")).unwrap();
    assert_eq!(n4.market, "US");
    assert_eq!(n4.prefix, "US");
    assert_eq!(n4.symbol, "US.AAPL");

    // Yahoo Finance suffix alias .SS
    let n5 = normalize_instrument(None, Some("600519.SS")).unwrap();
    assert_eq!(n5.market, "CN");
    assert_eq!(n5.prefix, "SH");
    assert_eq!(n5.symbol, "SH.600519");

    // Suffix with explicit parent market CN
    let n6 = normalize_instrument(Some("CN"), Some("600519.SH")).unwrap();
    assert_eq!(n6.prefix, "SH");
    assert_eq!(n6.symbol, "SH.600519");

    // Market alias SS
    let ss_rule = find_market_rule("SS").expect("SS alias rule");
    assert_eq!(ss_rule.code, "SH");
    assert_eq!(ss_rule.resolved_market, "CN");
    let n7 = normalize_instrument(Some("SS"), Some("600519")).unwrap();
    assert_eq!(n7.prefix, "SH");
    assert_eq!(n7.symbol, "SH.600519");
}

#[test]
fn infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes() {
    // Bare codes
    assert_eq!(infer_cn_prefix("600519"), "SH");
    assert_eq!(infer_cn_prefix("000001"), "SZ");
    assert_eq!(infer_cn_prefix("300750"), "SZ");
    assert_eq!(infer_cn_prefix("730001"), "SH");

    // Explicit prefixes must NOT be overridden by code heuristics
    // (e.g. Shanghai Composite Index SH.000001 must stay SH)
    assert_eq!(infer_cn_prefix("SH.000001"), "SH");
    assert_eq!(infer_cn_prefix("SH:000001"), "SH");
    assert_eq!(infer_cn_prefix("CNSH:000001"), "SH");
    assert_eq!(infer_cn_prefix("SZ.600519"), "SZ");
    assert_eq!(infer_cn_prefix("SZ:600519"), "SZ");
    assert_eq!(infer_cn_prefix("CNSZ:600519"), "SZ");

    // Suffix formats
    assert_eq!(infer_cn_prefix("600519.SH"), "SH");
    assert_eq!(infer_cn_prefix("000001.SZ"), "SZ");
    assert_eq!(infer_cn_prefix("600519.SS"), "SH");
    assert_eq!(infer_cn_prefix("000001.SH"), "SH");

    // Generic CN prefix
    assert_eq!(infer_cn_prefix("CN.600519"), "SH");
    assert_eq!(infer_cn_prefix("CN.000001"), "SZ");
}
