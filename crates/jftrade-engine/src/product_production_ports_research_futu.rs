//! Futu/OpenD projections for the advanced research routes.
//!
//! The integration crate owns protobuf framing and typed response validation.
//! This module only translates the public HTTP query into those typed ports
//! and projects their provider-neutral values into the existing
//! `broker.FeatureResult` envelope.

use std::sync::Arc;

use jftrade_integration_futu::{
    FutuIndicatorInput, FutuIndicatorListQuery, FutuInstitutionOperation,
    FutuInstitutionQuery, FutuInstitutionQueryError, FutuInstitutionSecurityQuery,
    FutuShortInterestQuery, IndicatorCalcQuery, IndicatorKline, ShortInterestOperation,
};
use serde_json::{Map, Value, json};

use crate::product::product_production_ports::SharedTradeReadRuntime;
use crate::product::product_query::{QueryMap, decode_query_component};
use crate::product::ResearchReadSnapshotError;

const COMMON_QUERY_KEYS: &[&str] = &[
    "brokerId",
    "providerBrokerId",
    "accountId",
    "tradingEnvironment",
    "market",
    "operation",
    "cursor",
    "pageSize",
    "refresh",
];

/// Read the collection-scoped institution and ARK operations.
pub(super) fn read_institutions(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    if path != "/api/v1/research/institutions" {
        return Err(ResearchReadSnapshotError::Invalid(
            "unsupported institutions route".to_owned(),
        ));
    }
    let query_map = parse_query(query)?;
    ensure_query_keys(
        query,
        &[
            COMMON_QUERY_KEYS,
            &[
                "institutionId",
                "instrumentId",
                "changeType",
                "holdingType",
                "cycleType",
                "sortField",
                "sortDir",
                "count",
                "page",
                "namePart",
                "keyword",
            ],
        ]
        .concat(),
    )?;
    let operation = parse_institution_operation(query_map.get_first("operation"))?;
    let market = parse_market(
        query_map.get_first("market"),
        "US",
        &["HK", "US", "SH", "SZ"],
        "institution",
    )?;
    let institution_id = parse_optional_i32(&query_map, "institutionId")?;
    let security = query_map
        .get_first("instrumentId")
        .map(parse_instrument_identity)
        .transpose()?;
    if security.as_ref().is_some_and(|security| security.0 != market) {
        return Err(ResearchReadSnapshotError::Invalid(
            "instrumentId market does not match market".to_owned(),
        ));
    }
    let count = parse_optional_i32(&query_map, "pageSize")?
        .or(parse_optional_i32(&query_map, "count")?)
        .or(Some(20));
    let request = FutuInstitutionQuery {
        operation,
        market: market_code(&market).expect("validated institution market"),
        institution_id,
        security: security.map(|(_, code)| FutuInstitutionSecurityQuery {
            market: market_code(&market).expect("validated institution market"),
            code,
        }),
        change_type: parse_optional_i32(&query_map, "changeType")?,
        holding_type: parse_optional_i32(&query_map, "holdingType")?,
        cycle_type: parse_optional_i32(&query_map, "cycleType")?,
        sort_field: parse_optional_i32(&query_map, "sortField")?,
        sort_dir: parse_optional_i32(&query_map, "sortDir")?,
        count,
        page: first_non_empty(&query_map, "cursor", "page"),
        name_part: optional_text(&query_map, "namePart")?,
        keyword: optional_text(&query_map, "keyword")?,
    };
    let runtime = institution_runtime(runtime)?;
    let result = runtime.institution(&request).map_err(map_institution_error)?;
    project_institution_result(result)
}

/// Read a security-scoped short-interest operation.
pub(super) fn read_short_interest(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let (market, code) = parse_instrument_path(path, "/api/v1/research/short-interest/", &[
        "HK", "US",
    ])?;
    let query_map = parse_query(query)?;
    ensure_query_keys(
        query,
        &[COMMON_QUERY_KEYS, &["instrumentId", "symbol"]].concat(),
    )?;
    ensure_query_market(&query_map, &market)?;
    let operation = match query_map
        .get_first("operation")
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        None => ShortInterestOperation::DailyVolume,
        Some("daily_volume") => ShortInterestOperation::DailyVolume,
        Some("short_interest") => ShortInterestOperation::ShortInterest,
        Some(value) => {
            return Err(ResearchReadSnapshotError::Invalid(format!(
                "unsupported short-interest operation {value:?}"
            )));
        }
    };
    let limit = parse_optional_i32(&query_map, "pageSize")?.unwrap_or(50);
    let request = FutuShortInterestQuery {
        market: market_code(&market).expect("validated short-interest market"),
        code,
        operation,
        next_key: optional_text(&query_map, "cursor")?,
        limit,
    };
    let runtime = short_interest_runtime(runtime)?;
    let result = runtime.short_interest(&request).map_err(map_short_interest_error)?;
    project_short_interest_result(result)
}

/// Read an indicator catalogue or acknowledge an asynchronous calculation.
pub(super) fn read_technical_indicators(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let (market, code) = parse_instrument_path(
        path,
        "/api/v1/research/technical-indicators/",
        &["HK", "US", "SH", "SZ", "SG", "JP", "AU", "MY", "CA"],
    )?;
    let query_map = parse_query(query)?;
    ensure_query_keys(
        query,
        &[
            COMMON_QUERY_KEYS,
            &[
                "instrumentId",
                "symbol",
                "searchKey",
                "langType",
                "searchMode",
                "shortName",
                "klType",
                "kLine",
                "num",
                "inputs",
            ],
        ]
        .concat(),
    )?;
    ensure_query_market(&query_map, &market)?;
    let operation = query_map
        .get_first("operation")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("list");
    let runtime = technical_indicator_runtime(runtime)?;
    match operation {
        "list" => {
            let list = runtime
                .technical_indicator_list(&FutuIndicatorListQuery {
                    search_key: optional_text(&query_map, "searchKey")?,
                    lang_type: parse_optional_i32(&query_map, "langType")?,
                    search_mode: parse_optional_i32(&query_map, "searchMode")?,
                })
                .map_err(map_indicator_error)?;
            project_indicator_list(list, &market, &code)
        }
        "calculate" => {
            let short_name = required_text(&query_map, "shortName")?;
            let lang_type = required_i32(&query_map, "langType")?;
            let kl_type = required_i32(&query_map, "klType")?;
            let k_line = parse_klines(&query_map)?;
            let inputs = parse_indicator_inputs(&query_map)?;
            let calculation = runtime
                .technical_indicator_calculate(&IndicatorCalcQuery {
                    short_name,
                    lang_type,
                    market: market_code(&market).expect("validated indicator market"),
                    code: code.clone(),
                    kl_type,
                    k_line,
                    num: parse_optional_i32(&query_map, "num")?,
                    inputs,
                })
                .map_err(map_indicator_error)?;
            project_indicator_calculation(calculation, &market, &code)
        }
        value => Err(ResearchReadSnapshotError::Invalid(format!(
            "unsupported technical-indicators operation {value:?}"
        ))),
    }
}

fn parse_query(query: &str) -> Result<QueryMap, ResearchReadSnapshotError> {
    QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))
}

fn ensure_query_keys(
    raw_query: &str,
    allowed: &[&str],
) -> Result<(), ResearchReadSnapshotError> {
    for key in raw_query
        .trim()
        .trim_start_matches('?')
        .split('&')
        .filter(|pair| !pair.is_empty())
        .map(|pair| pair.split_once('=').map_or(pair, |(key, _)| key))
        .map(decode_query_component)
    {
        let key = key
            .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
        if !allowed.contains(&key.as_str()) {
            return Err(ResearchReadSnapshotError::Invalid(format!(
                "unsupported research query parameter {key}"
            )));
        }
    }
    Ok(())
}

fn ensure_query_market(query: &QueryMap, market: &str) -> Result<(), ResearchReadSnapshotError> {
    if let Some(requested) = query
        .get_first("market")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        && !requested.eq_ignore_ascii_case(market)
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "market does not match instrumentId".to_owned(),
        ));
    }
    Ok(())
}

fn parse_institution_operation(
    value: Option<&str>,
) -> Result<FutuInstitutionOperation, ResearchReadSnapshotError> {
    match value.map(str::trim).filter(|value| !value.is_empty()) {
        None | Some("list") => Ok(FutuInstitutionOperation::List),
        Some("profile") => Ok(FutuInstitutionOperation::Profile),
        Some("distribution") => Ok(FutuInstitutionOperation::Distribution),
        Some("holding_changes") => Ok(FutuInstitutionOperation::HoldingChanges),
        Some("holdings") => Ok(FutuInstitutionOperation::Holdings),
        Some("ark_fund_holdings") => Ok(FutuInstitutionOperation::ArkFundHoldings),
        Some("ark_stock_activity") => Ok(FutuInstitutionOperation::ArkStockActivity),
        Some("ark_transactions") => Ok(FutuInstitutionOperation::ArkTransactions),
        Some(value) => Err(ResearchReadSnapshotError::Invalid(format!(
            "unsupported institutions operation {value:?}"
        ))),
    }
}

fn parse_market(
    value: Option<&str>,
    default: &str,
    allowed: &[&str],
    family: &str,
) -> Result<String, ResearchReadSnapshotError> {
    let market = value
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(default)
        .to_ascii_uppercase();
    if !allowed.contains(&market.as_str()) {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "{family} market is unsupported"
        )));
    }
    Ok(market)
}

fn parse_instrument_path(
    path: &str,
    prefix: &str,
    allowed_markets: &[&str],
) -> Result<(String, String), ResearchReadSnapshotError> {
    let suffix = path
        .strip_prefix(prefix)
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| ResearchReadSnapshotError::Invalid("instrument path is invalid".to_owned()))?;
    let (market, code) = parse_instrument_identity(suffix)?;
    if !allowed_markets.contains(&market.as_str()) {
        return Err(ResearchReadSnapshotError::Invalid(
            "instrument market is unsupported".to_owned(),
        ));
    }
    Ok((market, code))
}

fn parse_instrument_identity(
    value: &str,
) -> Result<(String, String), ResearchReadSnapshotError> {
    let (raw_market, raw_code) = value
        .trim()
        .split_once('.')
        .ok_or_else(|| ResearchReadSnapshotError::Invalid("instrumentId must be MARKET.CODE".to_owned()))?;
    let market = raw_market.trim().to_ascii_uppercase();
    let code = raw_code.trim().to_ascii_uppercase();
    if market.is_empty()
        || code.is_empty()
        || market.chars().any(|value| value.is_whitespace() || value.is_control())
        || code
            .chars()
            .any(|value| value.is_whitespace() || value.is_control() || value == '/')
    {
        return Err(ResearchReadSnapshotError::Invalid(
            "instrumentId must be MARKET.CODE".to_owned(),
        ));
    }
    Ok((market, code))
}

fn market_code(market: &str) -> Option<i32> {
    match market {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" => Some(21),
        "SZ" => Some(22),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        _ => None,
    }
}

fn parse_optional_i32(
    query: &QueryMap,
    key: &str,
) -> Result<Option<i32>, ResearchReadSnapshotError> {
    let Some(value) = query.get_first(key) else {
        return Ok(None);
    };
    if value.trim().is_empty() {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "{key} must be an integer"
        )));
    }
    value
        .trim()
        .parse::<i32>()
        .map(Some)
        .map_err(|_| ResearchReadSnapshotError::Invalid(format!("{key} must be an integer")))
}

fn required_i32(query: &QueryMap, key: &str) -> Result<i32, ResearchReadSnapshotError> {
    parse_optional_i32(query, key)?.ok_or_else(|| {
        ResearchReadSnapshotError::Invalid(format!("{key} is required for indicator calculation"))
    })
}

fn optional_text(
    query: &QueryMap,
    key: &str,
) -> Result<Option<String>, ResearchReadSnapshotError> {
    let Some(value) = query.get_first(key) else {
        return Ok(None);
    };
    let value = value.trim();
    if value.is_empty() {
        return Ok(None);
    }
    if value.chars().any(char::is_control) {
        return Err(ResearchReadSnapshotError::Invalid(format!(
            "{key} contains control characters"
        )));
    }
    Ok(Some(value.to_owned()))
}

fn required_text(query: &QueryMap, key: &str) -> Result<String, ResearchReadSnapshotError> {
    optional_text(query, key)?.ok_or_else(|| {
        ResearchReadSnapshotError::Invalid(format!("{key} is required for indicator calculation"))
    })
}

fn first_non_empty(query: &QueryMap, first: &str, second: &str) -> Option<String> {
    query
        .get_first(first)
        .or_else(|| query.get_first(second))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn institution_runtime(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
) -> Result<&SharedTradeReadRuntime, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu institution research runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.institution_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu institution research reader is not ready".to_owned(),
        ));
    }
    Ok(runtime)
}

fn short_interest_runtime(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
) -> Result<&SharedTradeReadRuntime, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu short-interest research runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.short_interest_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu short-interest research reader is not ready".to_owned(),
        ));
    }
    Ok(runtime)
}

fn technical_indicator_runtime(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
) -> Result<&SharedTradeReadRuntime, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu technical-indicator research runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.technical_indicator_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu technical-indicator research reader is not ready".to_owned(),
        ));
    }
    Ok(runtime)
}

fn project_institution_result(
    result: jftrade_integration_futu::FutuInstitutionResult,
) -> Result<Value, ResearchReadSnapshotError> {
    let mut value = serde_json::to_value(result).map_err(serialize_error)?;
    let object = value
        .as_object_mut()
        .ok_or_else(|| invalid_payload("institution result is not an object"))?;
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let next_cursor = object
        .get("nextPage")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty() && *value != "-1")
        .map(str::to_owned);
    let total = object
        .get("allCount")
        .and_then(Value::as_i64)
        .or_else(|| object.get("entries").and_then(Value::as_array).map(|v| v.len() as i64))
        .unwrap_or(0);
    object.insert("provider".to_owned(), futu_provider("research.institutions", &as_of));
    object.insert("asOf".to_owned(), Value::String(as_of));
    object.insert("hasMore".to_owned(), Value::Bool(next_cursor.is_some()));
    object.insert("total".to_owned(), json!(total));
    if let Some(next_cursor) = next_cursor {
        object.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
    Ok(value)
}

fn project_short_interest_result(
    result: jftrade_integration_futu::FutuShortInterestResult,
) -> Result<Value, ResearchReadSnapshotError> {
    let mut value = serde_json::to_value(result).map_err(serialize_error)?;
    let object = value
        .as_object_mut()
        .ok_or_else(|| invalid_payload("short-interest result is not an object"))?;
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    let entries = object.remove("items").unwrap_or_else(|| Value::Array(Vec::new()));
    let total = entries.as_array().map_or(0, Vec::len);
    object.insert("entries".to_owned(), entries);
    if let Some(security) = object.get("security").cloned() {
        object.insert("resolvedInstrument".to_owned(), security);
    }
    let next_cursor = object
        .get("nextKey")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty() && *value != "-1")
        .map(str::to_owned);
    object.insert("provider".to_owned(), futu_provider("research.short_interest", &as_of));
    object.insert("asOf".to_owned(), Value::String(as_of));
    object.insert("hasMore".to_owned(), Value::Bool(next_cursor.is_some()));
    object.insert("total".to_owned(), json!(total));
    if let Some(next_cursor) = next_cursor {
        object.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
    Ok(value)
}

fn project_indicator_list(
    result: jftrade_integration_futu::FutuIndicatorList,
    market: &str,
    code: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let entries = serde_json::to_value(result.indicators).map_err(serialize_error)?;
    let total = entries.as_array().map_or(0, Vec::len);
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    Ok(json!({
        "provider": futu_provider("research.technical_indicators", &as_of),
        "resolvedInstrument": resolved_instrument(market, code),
        "asOf": as_of,
        "operation": "list",
        "entries": entries,
        "hasMore": false,
        "total": total,
    }))
}

fn project_indicator_calculation(
    result: jftrade_integration_futu::FutuIndicatorCalculation,
    market: &str,
    code: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let entries = serde_json::to_value(&result).map_err(serialize_error)?;
    let as_of = crate::product::product_production_ports::provider_now_rfc3339();
    Ok(json!({
        "provider": futu_provider("research.technical_indicators", &as_of),
        "resolvedInstrument": resolved_instrument(market, code),
        "asOf": as_of,
        "operation": "calculate",
        "entries": [entries],
        "hasMore": false,
        "total": 1,
    }))
}

fn futu_provider(feature_id: &str, as_of: &str) -> Value {
    json!({
        "brokerId": "futu",
        "securityFirm": "Futu/Moomoo via OpenD",
        "featureId": feature_id,
        "capability": "available",
        "selectionReason": "adapter_request",
        "resolvedAt": as_of,
        "asOf": as_of,
    })
}

fn resolved_instrument(market: &str, code: &str) -> Value {
    json!({
        "instrumentId": format!("{market}.{code}"),
        "code": code,
        "productClass": "unknown",
        "marketSegment": "securities",
        "quoteMarket": market,
        "tradeMarket": market,
        "quantityMode": "units",
    })
}

fn parse_klines(query: &QueryMap) -> Result<Vec<IndicatorKline>, ResearchReadSnapshotError> {
    let Some(raw) = query.get_first("kLine") else {
        return Ok(Vec::new());
    };
    let value: Value = serde_json::from_str(raw)
        .map_err(|error| ResearchReadSnapshotError::Invalid(format!("kLine must be JSON: {error}")))?;
    let values = value.as_array().ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("kLine must be a JSON array".to_owned())
    })?;
    values.iter().map(parse_kline).collect()
}

fn parse_kline(value: &Value) -> Result<IndicatorKline, ResearchReadSnapshotError> {
    let object = value.as_object().ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("kLine entries must be JSON objects".to_owned())
    })?;
    let time = object
        .get("time")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .ok_or_else(|| ResearchReadSnapshotError::Invalid("kLine.time is required".to_owned()))?;
    let is_blank = match object.get("isBlank") {
        None | Some(Value::Null) => false,
        Some(Value::Bool(value)) => *value,
        Some(_) => return Err(ResearchReadSnapshotError::Invalid("kLine.isBlank must be boolean".to_owned())),
    };
    Ok(IndicatorKline {
        time,
        is_blank,
        high_price: optional_f64(object, "highPrice")?,
        open_price: optional_f64(object, "openPrice")?,
        low_price: optional_f64(object, "lowPrice")?,
        close_price: optional_f64(object, "closePrice")?,
        last_close_price: optional_f64(object, "lastClosePrice")?,
        volume: optional_i64(object, "volume")?,
        turnover: optional_f64(object, "turnover")?,
        turnover_rate: optional_f64(object, "turnoverRate")?,
        pe: optional_f64(object, "pe")?,
        change_rate: optional_f64(object, "changeRate")?,
        timestamp: optional_f64(object, "timestamp")?,
        hp_volume: optional_f64(object, "hpVolume")?,
    })
}

fn optional_f64(
    object: &Map<String, Value>,
    key: &str,
) -> Result<Option<f64>, ResearchReadSnapshotError> {
    match object.get(key) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::Number(value)) => value
            .as_f64()
            .filter(|value| value.is_finite())
            .map(Some)
            .ok_or_else(|| ResearchReadSnapshotError::Invalid(format!("kLine.{key} must be finite"))),
        Some(_) => Err(ResearchReadSnapshotError::Invalid(format!("kLine.{key} must be a number"))),
    }
}

fn optional_i64(
    object: &Map<String, Value>,
    key: &str,
) -> Result<Option<i64>, ResearchReadSnapshotError> {
    match object.get(key) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::Number(value)) => value
            .as_i64()
            .map(Some)
            .ok_or_else(|| ResearchReadSnapshotError::Invalid(format!("kLine.{key} must be an integer"))),
        Some(_) => Err(ResearchReadSnapshotError::Invalid(format!("kLine.{key} must be an integer"))),
    }
}

fn parse_indicator_inputs(query: &QueryMap) -> Result<Vec<FutuIndicatorInput>, ResearchReadSnapshotError> {
    let Some(raw) = query.get_first("inputs") else {
        return Ok(Vec::new());
    };
    let value: Value = serde_json::from_str(raw)
        .map_err(|error| ResearchReadSnapshotError::Invalid(format!("inputs must be JSON: {error}")))?;
    let values = value.as_array().ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("inputs must be a JSON array".to_owned())
    })?;
    values
        .iter()
        .map(|value| {
            let object = value.as_object().ok_or_else(|| {
                ResearchReadSnapshotError::Invalid("inputs entries must be JSON objects".to_owned())
            })?;
            let index = object
                .get("index")
                .and_then(Value::as_i64)
                .and_then(|value| i32::try_from(value).ok())
                .ok_or_else(|| ResearchReadSnapshotError::Invalid("inputs.index must be an integer".to_owned()))?;
            let value = match object.get("value") {
                None | Some(Value::Null) => None,
                Some(Value::String(value)) => Some(value.clone()),
                Some(_) => return Err(ResearchReadSnapshotError::Invalid("inputs.value must be a string".to_owned())),
            };
            Ok(FutuIndicatorInput { index, value })
        })
        .collect()
}

fn map_institution_error(error: FutuInstitutionQueryError) -> ResearchReadSnapshotError {
    match error {
        FutuInstitutionQueryError::InvalidQuery(message)
            if message.contains("runtime is unavailable") => ResearchReadSnapshotError::Unavailable(message),
        FutuInstitutionQueryError::InvalidQuery(message) => ResearchReadSnapshotError::Invalid(message),
        FutuInstitutionQueryError::Rejected { operation, ret_type, err_code, message } => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "FUTU_INSTITUTION_REJECTED".to_owned(),
            message: format!("OpenD institution {operation} retType={ret_type} errCode={err_code}: {message}"),
            retry_after_seconds: None,
        },
        FutuInstitutionQueryError::Session(error) => failed_error(error.to_string()),
        FutuInstitutionQueryError::Decode(error) => failed_error(error.to_string()),
        FutuInstitutionQueryError::MissingS2c | FutuInstitutionQueryError::InvalidResponse(_) => {
            failed_error(error.to_string())
        }
    }
}

fn map_short_interest_error(
    error: jftrade_integration_futu::FutuShortInterestQueryError,
) -> ResearchReadSnapshotError {
    match error {
        jftrade_integration_futu::FutuShortInterestQueryError::InvalidQuery(message)
            if message.contains("runtime is unavailable") => ResearchReadSnapshotError::Unavailable(message),
        jftrade_integration_futu::FutuShortInterestQueryError::InvalidQuery(message) => ResearchReadSnapshotError::Invalid(message),
        jftrade_integration_futu::FutuShortInterestQueryError::Rejected { operation, ret_type, err_code, message } => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "FUTU_SHORT_INTEREST_REJECTED".to_owned(),
            message: format!("OpenD {operation} retType={ret_type} errCode={err_code}: {message}"),
            retry_after_seconds: None,
        },
        other => failed_error(other.to_string()),
    }
}

fn map_indicator_error(
    error: jftrade_integration_futu::FutuIndicatorQueryError,
) -> ResearchReadSnapshotError {
    match error {
        jftrade_integration_futu::FutuIndicatorQueryError::InvalidQuery(message)
            if message.contains("runtime is unavailable") => ResearchReadSnapshotError::Unavailable(message),
        jftrade_integration_futu::FutuIndicatorQueryError::InvalidQuery(message) => ResearchReadSnapshotError::Invalid(message),
        jftrade_integration_futu::FutuIndicatorQueryError::Rejected { operation, ret_type, err_code, message } => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "FUTU_INDICATOR_REJECTED".to_owned(),
            message: format!("OpenD {operation} retType={ret_type} errCode={err_code}: {message}"),
            retry_after_seconds: None,
        },
        other => failed_error(other.to_string()),
    }
}

fn failed_error(message: String) -> ResearchReadSnapshotError {
    ResearchReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message,
        retry_after_seconds: None,
    }
}

fn serialize_error(error: serde_json::Error) -> ResearchReadSnapshotError {
    failed_error(format!("serialize Futu research response: {error}"))
}

fn invalid_payload(message: &str) -> ResearchReadSnapshotError {
    failed_error(message.to_owned())
}
