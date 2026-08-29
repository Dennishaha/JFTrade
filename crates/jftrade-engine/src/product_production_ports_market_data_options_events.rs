use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option event runtime is not configured".to_owned(),
        )
    })?;
    let operation = operation(query)?;
    if operation == "zero_dte" {
        return read_zero_dte(runtime, query);
    }
    if operation == "earnings" {
        return read_earnings(runtime, query);
    }
    if operation == "seller" {
        return read_seller(runtime, query);
    }
    if operation != "unusual" {
        return Err(bad_request(
            "operation must be unusual, zero_dte, earnings, or seller",
        ));
    }
    let request = parse_request(query)?;
    if !runtime.option_events_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option event reader is not ready".to_owned(),
        ));
    }
    let page = runtime.option_events(&request).map_err(map_error)?;
    let entries = page
        .events
        .into_iter()
        .map(|event| {
            serde_json::to_value(event).map_err(|error| {
                MarketDataOptionsReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD option event: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let total = page.all_count.unwrap_or(entries.len() as i32);
    let has_more = page.next_page.is_some();
    let mut result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_events",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": has_more,
        "total": total,
    });
    if let Some(next_page) = page.next_page {
        result["nextCursor"] = Value::String(next_page);
    }
    Ok(result)
}

fn operation(query: &str) -> Result<String, MarketDataOptionsReadSnapshotError> {
    let map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    Ok(map
        .get_first("operation")
        .map(|value| value.trim().to_ascii_lowercase())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "unusual".to_owned()))
}

fn read_zero_dte(
    runtime: &super::super::product_production_ports_trade::SharedTradeReadRuntime,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_zero_dte_screener_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu 0DTE screener reader is not ready".to_owned(),
        ));
    }
    let (option_market, sort_type, is_asc, count, page, filters) = parse_screener_common(query, true)?;
    let request = jftrade_integration_futu::OptionZeroDteScreenerQuery { option_market, sort_type, is_asc, count, page, filters };
    let page = runtime
        .option_zero_dte_screener(&request)
        .map_err(map_zero_dte_error)?;
    let mut entries = Vec::with_capacity(page.items.len());
    for item in page.items {
        let mut value = serde_json::to_value(item).map_err(|error| bad_gateway(error.to_string()))?;
        if let Some(chain) = value.get("chainInfo").cloned() {
            let underlying = value
                .get("owner")
                .and_then(|owner| owner.get("instrumentId"))
                .cloned()
                .unwrap_or(Value::Null);
            let context = json!({
                "underlyingInstrumentId": underlying,
                "expiryTimestamp": chain.get("strikeDateTimestamp").cloned().unwrap_or(Value::Null),
                "chain": {
                    "productCode": chain.get("productCode").cloned().unwrap_or(Value::Null),
                    "multiplier": chain.get("multiplier").cloned().unwrap_or(Value::Null),
                    "contractSize": chain.get("contractShareSize").cloned().unwrap_or(Value::Null),
                    "expirationType": chain.get("expirationType").cloned().unwrap_or(Value::Null),
                },
            });
            value["drilldownContext"] = context;
        }
        entries.push(value);
    }
    screener_result(entries, page.next_page, page.update_timestamp, None)
}

fn read_earnings(
    runtime: &super::super::product_production_ports_trade::SharedTradeReadRuntime,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_earnings_screener_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu earnings screener reader is not ready".to_owned(),
        ));
    }
    let (option_market, sort_type, is_asc, count, page, filters) = parse_screener_common(query, false)?;
    let request = jftrade_integration_futu::OptionEarningsScreenerQuery { option_market, sort_type, is_asc, count, page, filters };
    let page = runtime
        .option_earnings_screener(&request)
        .map_err(map_earnings_error)?;
    let entries = page
        .items
        .into_iter()
        .map(|item| serde_json::to_value(item).map_err(|error| bad_gateway(error.to_string())))
        .collect::<Result<Vec<_>, _>>()?;
    screener_result(entries, page.next_page, page.update_timestamp, page.all_count)
}

fn read_seller(
    runtime: &super::super::product_production_ports_trade::SharedTradeReadRuntime,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_seller_screener_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu seller screener reader is not ready".to_owned(),
        ));
    }
    let (option_market, seller_type, sort_type, is_asc, filters) = parse_seller_query(query)?;
    let request = jftrade_integration_futu::OptionSellerScreenerQuery {
        option_market,
        seller_type,
        sort_type,
        is_asc,
        filters,
    };
    let entries = runtime
        .option_seller_screener(&request)
        .map_err(map_seller_error)?
        .into_iter()
        .map(|item| serde_json::to_value(item).map_err(|error| bad_gateway(error.to_string())))
        .collect::<Result<Vec<_>, _>>()?;
    screener_result(entries, None, None, None)
}

fn parse_seller_query(
    query: &str,
) -> Result<
    (
        i32,
        i32,
        Option<i32>,
        Option<bool>,
        Vec<jftrade_integration_futu::EventIndicator>,
    ),
    MarketDataOptionsReadSnapshotError,
> {
    let map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    let market = map
        .get_first("market")
        .map(|value| value.trim().to_ascii_uppercase())
        .unwrap_or_else(|| "US".to_owned());
    let option_market = match market.as_str() {
        "US" => 1,
        "HK" => 3,
        _ => return Err(bad_request("seller option market must be US or HK")),
    };
    let product_class = map
        .get_first("underlyingProductClass")
        .map(|value| value.trim().to_ascii_lowercase());
    if product_class
        .as_deref()
        .is_some_and(|value| !value.is_empty() && value != "equity" && value != "option")
    {
        return Err(bad_request(
            "seller screener supports security options only",
        ));
    }
    let seller_type = match map
        .get_first("sellerStrategy")
        .or_else(|| map.get_first("sellerType"))
        .map(|value| value.trim().to_ascii_lowercase())
        .as_deref()
    {
        None | Some("") | Some("covered_call") | Some("coveredcall") | Some("1") => 1,
        Some("cash_secured_put") | Some("cashsecuredput") | Some("2") => 2,
        Some(_) => return Err(bad_request("unsupported sellerStrategy")),
    };
    let sort_type = map
        .get_first("sortType")
        .or_else(|| map.get_first("sort"))
        .map(|value| {
            if let Ok(number) = value.trim().parse::<i32>() {
                return Ok(number);
            }
            match value.trim().to_ascii_lowercase().as_str() {
                "annualized_return" | "annualized" | "return" => Ok(1),
                "interval_return" | "interval" => Ok(2),
                "itm_probability" | "itm" | "probability" => Ok(3),
                "premium" => Ok(4),
                _ => Err(bad_request("unsupported seller sort")),
            }
        })
        .transpose()?;
    let is_asc = map
        .get_first("isAsc")
        .or_else(|| map.get_first("sortAsc"))
        .map(|value| match value.trim().to_ascii_lowercase().as_str() {
            "true" | "1" | "asc" => Ok(true),
            "false" | "0" | "desc" => Ok(false),
            _ => Err(bad_request("isAsc must be true or false")),
        })
        .transpose()?;
    let owner = map
        .get_first("underlying")
        .or_else(|| map.get_first("instrumentId"))
        .or_else(|| map.get_first("code"));
    let filters = owner
        .filter(|value| !value.trim().is_empty())
        .map(|value| {
            parse_owner(value, &market).map(|owner| {
                vec![jftrade_integration_futu::EventIndicator {
                    indicator_type: 1,
                    value: Some(jftrade_integration_futu::EventIndicatorValue {
                        value_list: Vec::new(),
                        value_interval: None,
                        string_value_list: Vec::new(),
                        security_list: vec![owner],
                    }),
                }]
            })
        })
        .transpose()?
        .unwrap_or_default();
    Ok((option_market, seller_type, sort_type, is_asc, filters))
}

fn screener_result(
    entries: Vec<Value>,
    next_page: Option<String>,
    update_timestamp: Option<f64>,
    all_count: Option<i32>,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let total = all_count.unwrap_or(entries.len() as i32);
    let has_more = next_page.is_some();
    let as_of = super::super::provider_now_rfc3339();
    let mut result = json!({
        "provider": { "brokerId": "futu", "securityFirm": "Futu/Moomoo via OpenD", "featureId": "derivatives.option_events", "capability": "available", "selectionReason": "adapter_request", "resolvedAt": as_of, "asOf": as_of },
        "asOf": as_of,
        "entries": entries,
        "hasMore": has_more,
        "total": total,
    });
    if let Some(next) = next_page { result["nextCursor"] = Value::String(next); }
    if let Some(timestamp) = update_timestamp { result["updateTimestamp"] = json!(timestamp); }
    Ok(result)
}

fn parse_screener_common(
    query: &str,
    zero_dte: bool,
) -> Result<(i32, Option<i32>, Option<bool>, i32, Option<String>, Vec<jftrade_integration_futu::EventIndicator>), MarketDataOptionsReadSnapshotError> {
    let map = crate::product::product_query::QueryMap::parse(query).map_err(|_| bad_request("invalid URL escape"))?;
    let market = map.get_first("market").map(|value| value.trim().to_ascii_uppercase()).unwrap_or_else(|| "US".to_owned());
    let product_class = map.get_first("underlyingProductClass").map(|value| value.trim().to_ascii_lowercase()).unwrap_or_else(|| "equity".to_owned());
    let option_market = match (market.as_str(), product_class.as_str(), zero_dte) {
        ("US", "equity" | "option" | "", true) => 1,
        ("US", "index", true) => 2,
        ("US", "equity" | "option" | "", false) => 1,
        ("HK", "equity" | "option" | "", false) => 3,
        _ if zero_dte => return Err(bad_request("0DTE option research is available only in US market")),
        _ => return Err(bad_request("earnings screener supports US/HK security options only")),
    };
    let count = map.get_first("pageSize").or_else(|| map.get_first("count")).map(|value| parse_i32(value, "pageSize")).transpose()?.unwrap_or(50);
    if !(1..=500).contains(&count) { return Err(bad_request("pageSize must be between 1 and 500")); }
    let page = map.get_first("cursor").map(str::trim).filter(|value| !value.is_empty()).map(ToOwned::to_owned);
    let sort_type = map
        .get_first("sortType")
        .or_else(|| map.get_first("sort"))
        .map(|value| {
            if let Ok(number) = value.trim().parse::<i32>() {
                return Ok(number);
            }
            let key = value.trim().to_ascii_lowercase();
            let number = if zero_dte {
                match key.as_str() {
                    "volume" => 1,
                    "iv" => 2,
                    "change_rate" | "change" => 3,
                    "open_interest" | "oi" => 4,
                    "market_cap" => 5,
                    _ => return Err(bad_request("unsupported 0DTE sort")),
                }
            } else {
                match key.as_str() {
                    "earnings_date" | "date" => 1,
                    "volume" => 2,
                    "iv" => 3,
                    "market_cap" => 4,
                    "change_rate" | "change" => 5,
                    "price" => 6,
                    "iv_rank" => 7,
                    "iv_percentile" => 8,
                    "hv" => 9,
                    "open_interest" | "oi" => 10,
                    "last_report_iv_crush" => 11,
                    "history_report_iv_crush" => 12,
                    "last_report_chg_rate" => 13,
                    "history_report_chg_rate" => 14,
                    "estimate_eps_yoy" => 15,
                    "estimate_revenue_yoy" => 16,
                    "expected_move_ratio" => 17,
                    _ => return Err(bad_request("unsupported earnings sort")),
                }
            };
            Ok(number)
        })
        .transpose()?;
    let is_asc = map.get_first("isAsc").map(|value| match value.trim().to_ascii_lowercase().as_str() { "true" | "1" => Ok(true), "false" | "0" => Ok(false), _ => Err(bad_request("isAsc must be true or false")) }).transpose()?;
    let owner = map.get_first("underlying").or_else(|| map.get_first("instrumentId")).or_else(|| map.get_first("code"));
    let filters = owner.filter(|value| !value.trim().is_empty()).map(|value| parse_owner(value, &market)).transpose()?.map(|owner| vec![jftrade_integration_futu::EventIndicator { indicator_type: 1, value: Some(jftrade_integration_futu::EventIndicatorValue { value_list: Vec::new(), value_interval: None, string_value_list: Vec::new(), security_list: vec![owner] }) }]).unwrap_or_default();
    Ok((option_market, sort_type, is_asc, count, page, filters))
}

fn parse_request(
    query: &str,
) -> Result<jftrade_integration_futu::OptionEventQuery, MarketDataOptionsReadSnapshotError> {
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    let market = query_map
        .get_first("market")
        .map(|value| value.trim().to_ascii_uppercase())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "US".to_owned());
    let (market_code, market_label) = match market.as_str() {
        "US" => (11, "US"),
        "HK" => (1, "HK"),
        _ => return Err(bad_request("option event market must be HK or US")),
    };
    let underlying_product_class = parse_product_class(&query_map)?;
    let owner_value = query_map
        .get_first("underlying")
        .or_else(|| query_map.get_first("instrumentId"))
        .or_else(|| query_map.get_first("code"));
    let owner = owner_value
        .filter(|value| !value.trim().is_empty())
        .map(|value| parse_owner(value, market_label))
        .transpose()?;
    let count = query_map
        .get_first("pageSize")
        .or_else(|| query_map.get_first("count"))
        .map(|value| parse_i32(value, "pageSize"))
        .transpose()?
        .unwrap_or(100);
    if !(1..=300).contains(&count) {
        return Err(bad_request("pageSize must be between 1 and 300"));
    }
    let page = query_map
        .get_first("cursor")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    let sort = parse_sort(&query_map)?;
    Ok(jftrade_integration_futu::OptionEventQuery {
        market: option_market(market_code, underlying_product_class),
        underlying_product_class: Some(underlying_product_class),
        owner,
        count,
        page,
        filters: Vec::new(),
        sort,
    })
}

fn parse_product_class(
    query: &crate::product::product_query::QueryMap,
) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    match query
        .get_first("underlyingProductClass")
        .map(|value| value.trim().to_ascii_lowercase())
        .as_deref()
    {
        None | Some("") | Some("equity") | Some("option") => Ok(1),
        Some("index") => Ok(2),
        Some(_) => Err(bad_request(
            "underlyingProductClass must be equity or index",
        )),
    }
}

fn parse_owner(
    value: &str,
    expected_market: &str,
) -> Result<jftrade_integration_futu::OptionEventSecurity, MarketDataOptionsReadSnapshotError> {
    let (market, code) = value
        .trim()
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("underlying must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    if market != expected_market {
        return Err(bad_request("underlying market does not match market"));
    }
    let code = code.trim();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(bad_request("underlying code is invalid"));
    }
    Ok(jftrade_integration_futu::OptionEventSecurity {
        market: market.clone(),
        code: code.to_ascii_uppercase(),
        quote_market: market.clone(),
        trade_market: market.clone(),
        instrument_id: format!("{market}.{}", code.to_ascii_uppercase()),
    })
}

fn parse_sort(
    query: &crate::product::product_query::QueryMap,
) -> Result<Option<jftrade_integration_futu::EventSort>, MarketDataOptionsReadSnapshotError> {
    let Some(value) = query.get_first("sort").filter(|value| !value.trim().is_empty()) else {
        return Ok(None);
    };
    let indicator_type = match value.trim().to_ascii_lowercase().as_str() {
        "time" | "fill_time" => 305,
        "price" => 304,
        "volume" => 302,
        "turnover" => 303,
        "dte" | "expiry_days" => 204,
        "iv" => 504,
        "delta" => 601,
        "gamma" => 602,
        "vega" => 603,
        "theta" => 604,
        "rho" => 605,
        _ => return Err(bad_request("unsupported option event sort")),
    };
    let is_asc = match query.get_first("sortAsc") {
        None | Some("") => false,
        Some(value) => match value.trim().to_ascii_lowercase().as_str() {
            "true" | "1" | "asc" => true,
            "false" | "0" | "desc" => false,
            _ => return Err(bad_request("sortAsc must be true or false")),
        },
    };
    Ok(Some(jftrade_integration_futu::EventSort {
        indicator_type,
        is_asc,
    }))
}

fn option_market(market: i32, product_class: i32) -> i32 {
    match (market, product_class) {
        (11, 2) => 2,
        (1, 2) => 4,
        (11, _) => 1,
        (1, _) => 3,
        _ => 0,
    }
}

fn parse_i32(value: &str, key: &str) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    value
        .trim()
        .parse::<i32>()
        .map_err(|_| bad_request(&format!("{key} must be an integer")))
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionEventQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionEventQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_zero_dte_error(
    error: jftrade_integration_futu::OptionZeroDteScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionZeroDteScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

fn map_earnings_error(
    error: jftrade_integration_futu::OptionEarningsScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionEarningsScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

fn map_seller_error(
    error: jftrade_integration_futu::OptionSellerScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionSellerScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

fn bad_gateway(message: String) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message,
    }
}
