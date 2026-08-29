use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option analysis runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.option_quotes_available()
        && !runtime.option_volatility_available()
        && !runtime.option_exercise_probability_available()
        && !runtime.option_underlying_overview_available()
        && !runtime.option_underlying_rank_available()
        && !runtime.option_contract_rank_available()
    {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option analysis readers are not ready".to_owned(),
        ));
    }
    let operation = parse_operation(query)?;
    if operation == "volatility" {
        return read_volatility(runtime, path, query);
    }
    if operation == "exercise_probability" {
        return read_exercise_probability(runtime, path, query);
    }
    if operation == "underlying_overview" {
        return read_underlying_overview(runtime, path, query);
    }
    if operation == "underlying_rank" {
        return read_underlying_rank(runtime, path, query);
    }
    if operation == "contract_rank" {
        return super::product_production_ports_market_data_options_contract_rank::read(
            Some(runtime),
            path,
            query,
        );
    }
    if operation != "quote" {
        return Err(bad_request(
            "operation must be quote, volatility, exercise_probability, underlying_overview, underlying_rank, or contract_rank",
        ));
    }
    if !runtime.option_quotes_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option quote reader is not ready".to_owned(),
        ));
    }
    let request = parse_quote_request(path, query)?;
    let quotes = runtime.option_quotes(&request).map_err(map_quote_error)?;
    if quotes.is_empty() {
        return Err(MarketDataOptionsReadSnapshotError::Failed {
            status: 404,
            code: "OPTION_QUOTE_NOT_FOUND".to_owned(),
            message: "OpenD returned no option quote".to_owned(),
        });
    }
    let entries = quotes.into_iter().map(serialize_quote).collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
    }))
}

fn parse_quote_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionQuoteQuery, MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| {
            !market.is_empty() && !code.is_empty() && !code.contains('.')
        })
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(bad_request("option quote market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(bad_request("option quote code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let request = jftrade_integration_futu::OptionQuoteQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn read_volatility(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_volatility_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option volatility reader is not ready".to_owned(),
        ));
    }
    let request = parse_volatility_request(path, query)?;
    let snapshot = runtime
        .option_volatility(&request)
        .map_err(map_volatility_error)?;
    let jftrade_integration_futu::OptionVolatilitySnapshot {
        security: _security,
        items,
        average_impvol,
        impvol_status,
        analysis,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| serde_json::to_value(item).map_err(|error| serialization_error("volatility", error)))
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    if let Some(value) = average_impvol {
        metadata.insert("averageImpvol".to_owned(), json!(value));
    }
    if let Some(value) = impvol_status {
        metadata.insert("impvolStatus".to_owned(), json!(value));
    }
    if let Some(value) = analysis {
        metadata.insert("analysis".to_owned(), json!(value));
    }
    let mut result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
    });
    if !metadata.is_empty() {
        result["metadata"] = Value::Object(metadata);
    }
    Ok(result)
}

fn read_exercise_probability(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_exercise_probability_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option exercise probability reader is not ready".to_owned(),
        ));
    }
    let request = parse_exercise_probability_request(path, query)?;
    let snapshot = runtime
        .option_exercise_probability(&request)
        .map_err(map_exercise_probability_error)?;
    let jftrade_integration_futu::OptionExerciseProbabilitySnapshot {
        security: _security,
        items,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item)
                .map_err(|error| serialization_error("exercise probability", error))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
    }))
}

fn read_underlying_overview(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_underlying_overview_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option underlying overview reader is not ready".to_owned(),
        ));
    }
    let request = parse_underlying_overview_request(path, query)?;
    let snapshot = runtime
        .option_underlying_overview(&request)
        .map_err(map_underlying_overview_error)?;
    let entries = snapshot
        .items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item)
                .map_err(|error| serialization_error("underlying overview", error))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": false,
        "total": total,
    }))
}

fn read_underlying_rank(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_underlying_rank_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option underlying rank reader is not ready".to_owned(),
        ));
    }
    let request = parse_underlying_rank_request(path, query)?;
    let snapshot = runtime
        .option_underlying_rank(&request)
        .map_err(map_underlying_rank_error)?;
    let jftrade_integration_futu::OptionUnderlyingRankSnapshot {
        market,
        sort_type,
        trading_date,
        trading_timestamp,
        items,
        next_page,
        all_count,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item)
                .map_err(|error| serialization_error("underlying rank", error))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let has_more = next_page.is_some();
    let total = all_count.unwrap_or(entries.len() as i32);
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    metadata.insert("market".to_owned(), json!(market));
    metadata.insert("sortType".to_owned(), json!(sort_type));
    if let Some(value) = trading_date {
        metadata.insert("tradingDate".to_owned(), json!(value));
    }
    if let Some(value) = trading_timestamp {
        metadata.insert("tradingTimestamp".to_owned(), json!(value));
    }
    let mut result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
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
    if !metadata.is_empty() {
        result["metadata"] = Value::Object(metadata);
    }
    if let Some(next_page) = next_page {
        result["nextCursor"] = Value::String(next_page);
    }
    Ok(result)
}

fn parse_operation(query: &str) -> Result<String, MarketDataOptionsReadSnapshotError> {
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    query_map
        .get_first("operation")
        .filter(|value| !value.trim().is_empty())
        .map(str::to_owned)
        .ok_or_else(|| bad_request("operation is required"))
}

fn parse_volatility_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionVolatilityQuery, MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(bad_request("option volatility market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty()
        || code.chars().any(char::is_whitespace)
        || code.chars().any(|value| !value.is_ascii_alphanumeric() && value != '-')
    {
        return Err(bad_request("option volatility code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let query_time_period = query_map
        .get_first("queryTimePeriod")
        .map(parse_query_time_period)
        .transpose()?;
    let hv_time_period = query_map
        .get_first("hvTimePeriod")
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("hvTimePeriod must be an integer"))
        })
        .transpose()?;
    let request = jftrade_integration_futu::OptionVolatilityQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
        query_time_period,
        hv_time_period,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_exercise_probability_request(
    path: &str,
    query: &str,
) -> Result<
    jftrade_integration_futu::OptionExerciseProbabilityQuery,
    MarketDataOptionsReadSnapshotError,
> {
    let request = parse_quote_request(path, query)?;
    let request = jftrade_integration_futu::OptionExerciseProbabilityQuery {
        market: request.market,
        code: request.code,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_underlying_overview_request(
    path: &str,
    query: &str,
) -> Result<
    jftrade_integration_futu::OptionUnderlyingOverviewQuery,
    MarketDataOptionsReadSnapshotError,
> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(bad_request("option underlying overview market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty()
        || code.chars().any(|value| {
            value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-')
        })
    {
        return Err(bad_request("option underlying overview code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let index_option_type = query_map
        .get_first("indexOptionType")
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("indexOptionType must be an integer"))
        })
        .transpose()?;
    let request = jftrade_integration_futu::OptionUnderlyingOverviewQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
        index_option_type,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_underlying_rank_request(
    path: &str,
    query: &str,
) -> Result<
    jftrade_integration_futu::OptionUnderlyingRankQuery,
    MarketDataOptionsReadSnapshotError,
> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(bad_request("option underlying rank market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(bad_request("option underlying rank code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let sort_type = query_map
        .get_first("sortType")
        .or_else(|| query_map.get_first("sort"))
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("sortType must be an integer"))
        })
        .transpose()?
        .unwrap_or(1);
    let is_asc = query_map
        .get_first("isAsc")
        .map(parse_rank_bool)
        .transpose()?;
    let count = query_map
        .get_first("count")
        .or_else(|| query_map.get_first("pageSize"))
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("count must be an integer"))
        })
        .transpose()?
        .or(Some(200));
    let trading_date = query_map
        .get_first("tradingDate")
        .map(|value| value.trim().to_owned());
    let page = query_map
        .get_first("page")
        .or_else(|| query_map.get_first("cursor"))
        .map(|value| value.trim().to_owned());
    let request = jftrade_integration_futu::OptionUnderlyingRankQuery {
        market: market_code,
        sort_type,
        is_asc,
        count,
        trading_date,
        page,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_rank_bool(value: &str) -> Result<bool, MarketDataOptionsReadSnapshotError> {
    match value.trim().to_ascii_lowercase().as_str() {
        "true" | "1" => Ok(true),
        "false" | "0" => Ok(false),
        _ => Err(bad_request("isAsc must be true or false")),
    }
}

fn parse_query_time_period(
    value: &str,
) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    match value.trim().to_ascii_lowercase().as_str() {
        "week" | "1w" | "1" => Ok(1),
        "month" | "1m" | "2" => Ok(2),
        "quarter" | "3m" | "3" => Ok(3),
        "half_year" | "half-year" | "6m" | "4" => Ok(4),
        "year" | "1y" | "5" => Ok(5),
        _ => Err(bad_request("queryTimePeriod is unsupported")),
    }
}

fn serialize_quote(
    quote: jftrade_integration_futu::OptionQuote,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    serde_json::to_value(quote).map_err(|error| serialization_error("quote", error))
}

fn serialization_error(
    operation: &str,
    error: serde_json::Error,
) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: format!("failed to serialize OpenD option {operation}: {error}"),
    }
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_quote_error(
    error: jftrade_integration_futu::OptionQuoteQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionQuoteQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_volatility_error(
    error: jftrade_integration_futu::OptionVolatilityQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionVolatilityQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_exercise_probability_error(
    error: jftrade_integration_futu::OptionExerciseProbabilityQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionExerciseProbabilityQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_underlying_overview_error(
    error: jftrade_integration_futu::OptionUnderlyingOverviewQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionUnderlyingOverviewQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_underlying_rank_error(
    error: jftrade_integration_futu::OptionUnderlyingRankQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionUnderlyingRankQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
