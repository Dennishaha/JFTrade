//! Futu market-code conversions shared by execution-order parsing and previews.

pub(crate) fn trade_market(value: &str) -> i32 {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 2,
        "CN" | "SH" | "SZ" | "CNSH" | "CNSZ" => 3,
        "SG" => 6,
        _ => 0,
    }
}

/// Futu quote-market identifiers used by combo-leg securities.  They are not
/// the trading-header identifiers above (US is 11 here, not 2).
pub(crate) fn quote_market(value: &str) -> i32 {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 11,
        "SH" | "CNSH" => 21,
        "SZ" | "CNSZ" => 22,
        "SG" => 31,
        "JP" => 41,
        "AU" => 51,
        "MY" => 61,
        "CA" => 71,
        "US_EVENT" | "EVENT" => 101,
        _ => 0,
    }
}

pub(crate) fn quote_market_from_trade_market(value: i32) -> i32 {
    match value {
        1 => 1,
        2 => 11,
        3 => 0,
        6 => 31,
        _ => 0,
    }
}

pub(crate) fn quote_market_label(value: i32) -> &'static str {
    match value {
        1 => "HK",
        11 | 101 => "US",
        21 => "SH",
        22 => "SZ",
        31 => "SG",
        41 => "JP",
        51 => "AU",
        61 => "MY",
        71 => "CA",
        _ => "UNKNOWN",
    }
}

pub(crate) fn sec_market(trd_market: i32) -> i32 {
    match trd_market {
        1 => 1,
        2 => 2,
        3 => 31,
        _ => 0,
    }
}

pub(crate) fn market_label(value: i32) -> String {
    match value {
        1 => "HK",
        2 => "US",
        3 => "CN",
        6 => "SG",
        _ => "UNKNOWN",
    }
    .to_owned()
}
