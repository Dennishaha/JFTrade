use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value, json};
use thiserror::Error;

pub const RESEARCH_SCREEN_PATH: &str = "/api/v1/research/screens";
pub const RESEARCH_SCREEN_WRITE_ROUTES: [(&str, &str); 1] = [("POST", RESEARCH_SCREEN_PATH)];

const FUTU_CATALOG_VERSION: &str = "futu-stock-screen-v1";
const EMBEDDED_CATALOG_VERSION: &str = "embedded-stock-screen-v1";
const INVALID_REQUEST_MESSAGE: &str = "invalid stock-screen request";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResearchScreenWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct ResearchScreenColumn {
    pub column_id: String,
    pub instance_id: String,
    pub factor_key: String,
    pub label: String,
    pub unit: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct ResearchScreenWriteQuery {
    pub broker_id: String,
    pub account_id: String,
    pub trading_environment: String,
    pub market: String,
    pub offset: i64,
    pub limit: i64,
    pub definition: Value,
    pub columns: Vec<ResearchScreenColumn>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum ResearchScreenWritePortError {
    #[error("research screen query port is unavailable")]
    Unavailable,
    #[error("{message}")]
    RateLimited { message: String, retry_after: u64 },
    #[error("{0}")]
    Capability(String),
    #[error("market data provider is warming")]
    ProviderWarming,
    #[error("market data provider is busy")]
    ProviderBusy,
    #[error("{0}")]
    Failed(String),
}

/// Consumer-owned research-screen query boundary.
///
/// The Go service remains the production owner of broker routing, caching,
/// provider lifecycle, and all side effects. A test-cutover adapter may return
/// a Go-shaped `ResearchScreenResult` document, but this leaf never opens a
/// provider, database, or durable state store.
pub trait ResearchScreenWritePort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ResearchScreenWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn research_screen_write_routes() -> &'static [(&'static str, &'static str); 1] {
    &RESEARCH_SCREEN_WRITE_ROUTES
}

pub fn dispatch_research_screen_write(
    request: &ResearchScreenWriteRequest,
    port: Option<&dyn ResearchScreenWritePort>,
    timestamp: &str,
) -> ResearchScreenWriteResponse {
    let (path, _) = split_path_query(&request.path);
    if request.method != "POST" || path != RESEARCH_SCREEN_PATH {
        return error_response(404, "NOT_FOUND", "resource not found", timestamp, None);
    }
    let query = match parse_query(request.body.as_deref()) {
        Ok(query) => query,
        Err(ParseQueryError::InvalidRequest) => {
            return error_response(400, "BAD_REQUEST", INVALID_REQUEST_MESSAGE, timestamp, None);
        }
        Err(ParseQueryError::InvalidDefinition(message)) => {
            return error_response(400, "BAD_REQUEST", &message, timestamp, None);
        }
    };
    let Some(port) = port else {
        return error_response(
            503,
            "RESEARCH_SCREEN_UNAVAILABLE",
            "research screen query port is unavailable",
            timestamp,
            None,
        );
    };
    match port.query(&query) {
        Ok(data) => match project_result(data, &query) {
            Ok(data) => success_response(data, timestamp),
            Err(message) => error_response(502, "BROKER_FEATURE_FAILED", &message, timestamp, None),
        },
        Err(error) => port_error_response(error, timestamp),
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
enum ParseQueryError {
    InvalidRequest,
    InvalidDefinition(String),
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireQuery {
    #[serde(default)]
    broker_id: Option<String>,
    #[serde(default)]
    market: Option<String>,
    #[serde(default)]
    pool: Option<WirePool>,
    #[serde(default)]
    conditions: Option<Vec<WireCondition>>,
    #[serde(default)]
    columns: Option<Vec<WireColumn>>,
    #[serde(default)]
    sorts: Option<Vec<WireSort>>,
    #[serde(default)]
    catalog_version: Option<String>,
    #[serde(default)]
    query_schema_version: Option<i64>,
    #[serde(default)]
    account_id: Option<String>,
    #[serde(default)]
    trading_environment: Option<String>,
    #[serde(default)]
    page: Option<WirePage>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePool {
    #[serde(default)]
    watchlist_stock_ids: Option<Vec<String>>,
    #[serde(default)]
    plates: Option<Vec<WirePlate>>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePlate {
    #[allow(dead_code)]
    #[serde(default)]
    parent_plate_id: Option<String>,
    #[serde(default)]
    plate_ids: Option<Vec<String>>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireFactorRef {
    #[serde(default)]
    instance_id: Option<String>,
    #[serde(default)]
    factor_key: Option<String>,
    #[serde(default)]
    params: Option<WireFactorParams>,
}

#[derive(Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireFactorParams {
    #[serde(default)]
    days: Option<i64>,
    #[serde(default)]
    period_average: Option<i64>,
    #[serde(default)]
    term: Option<i64>,
    #[serde(default)]
    duration: Option<i64>,
    #[serde(default)]
    year: Option<i64>,
    #[serde(default)]
    future_duration: Option<i64>,
    #[serde(default)]
    period: Option<i64>,
    #[serde(default)]
    range_period: Option<i64>,
    #[serde(default)]
    first_custom_param: Option<i64>,
    #[serde(default)]
    indicator_params: Option<Vec<i64>>,
    #[serde(default)]
    broker_param: Option<String>,
    #[serde(default)]
    option_param_type: Option<i64>,
    #[serde(default)]
    option_param_string: Option<String>,
    #[serde(default)]
    option_param_integer: Option<i64>,
    #[serde(default)]
    option_param_integers: Option<Vec<i64>>,
    #[serde(default)]
    option_hv_period: Option<i64>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireCondition {
    #[serde(default)]
    id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    operator: Option<String>,
    #[serde(default)]
    value: Option<Value>,
    #[serde(default)]
    second_factor: Option<WireFactorRef>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireColumn {
    #[serde(default)]
    column_id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    label: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireSort {
    #[serde(default)]
    sort_id: Option<String>,
    #[serde(default)]
    column_id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    direction: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePage {
    #[serde(default)]
    offset: Option<i64>,
    #[serde(default)]
    limit: Option<i64>,
}

fn parse_query(body: Option<&[u8]>) -> Result<ResearchScreenWriteQuery, ParseQueryError> {
    let Some(body) = body else {
        return Err(ParseQueryError::InvalidRequest);
    };
    if body.iter().all(u8::is_ascii_whitespace) {
        return Err(ParseQueryError::InvalidRequest);
    }
    let raw: Value = serde_json::from_slice(body).map_err(|_| ParseQueryError::InvalidRequest)?;
    let (raw, wire) = if raw.is_null() {
        (Value::Object(Map::new()), WireQuery::default())
    } else {
        let wire = serde_json::from_value::<WireQuery>(raw.clone())
            .map_err(|_| ParseQueryError::InvalidRequest)?;
        (raw, wire)
    };
    normalize_query(raw, wire)
}

fn normalize_query(
    mut raw: Value,
    wire: WireQuery,
) -> Result<ResearchScreenWriteQuery, ParseQueryError> {
    let page = wire.page.unwrap_or_default();
    let offset = page.offset.unwrap_or(0);
    if offset < 0 {
        return Err(ParseQueryError::InvalidDefinition(
            "page.offset must be non-negative".to_owned(),
        ));
    }
    let raw_limit = page.limit.unwrap_or(0);
    let limit = if raw_limit == 0 { 50 } else { raw_limit };
    if !(1..=100).contains(&limit) {
        return Err(ParseQueryError::InvalidDefinition(
            "page.limit must be between 1 and 100".to_owned(),
        ));
    }

    let broker_id = normalized_or_default(wire.broker_id.as_deref(), "futu", false);
    let market = normalized_or_default(wire.market.as_deref(), "", true);
    let catalog_version = wire.catalog_version.unwrap_or_default();
    if wire.query_schema_version.unwrap_or(0) != 2 {
        return Err(ParseQueryError::InvalidDefinition(
            "querySchemaVersion: must be 2".to_owned(),
        ));
    }
    if catalog_version != FUTU_CATALOG_VERSION && catalog_version != EMBEDDED_CATALOG_VERSION {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "catalogVersion: catalog {catalog_version:?} is not executable"
        )));
    }
    validate_market(&market, &catalog_version)?;
    validate_pool(wire.pool.as_ref())?;
    let normalized_conditions = normalize_conditions(
        wire.conditions.as_deref().unwrap_or(&[]),
        &catalog_version,
        &market,
    )?;
    let (normalized_columns, columns) = normalize_columns(
        wire.columns.as_deref().unwrap_or(&[]),
        &catalog_version,
        &market,
    )?;
    let normalized_sorts = normalize_sorts(
        wire.sorts.as_deref().unwrap_or(&[]),
        &catalog_version,
        &market,
    )?;

    let object = raw.as_object_mut().ok_or(ParseQueryError::InvalidRequest)?;
    object.insert("brokerId".to_owned(), Value::String(broker_id.clone()));
    object.insert("market".to_owned(), Value::String(market.clone()));
    object.insert(
        "catalogVersion".to_owned(),
        Value::String(catalog_version.clone()),
    );
    object.insert("querySchemaVersion".to_owned(), Value::Number(2.into()));
    object.insert(
        "conditions".to_owned(),
        if normalized_conditions.is_empty() {
            Value::Null
        } else {
            Value::Array(normalized_conditions)
        },
    );
    object.insert(
        "columns".to_owned(),
        if normalized_columns.is_empty() {
            Value::Null
        } else {
            Value::Array(normalized_columns)
        },
    );
    object.insert(
        "sorts".to_owned(),
        if normalized_sorts.is_empty() {
            Value::Null
        } else {
            Value::Array(normalized_sorts)
        },
    );
    object.remove("accountId");
    object.remove("tradingEnvironment");
    object.remove("page");

    Ok(ResearchScreenWriteQuery {
        broker_id,
        account_id: wire.account_id.unwrap_or_default().trim().to_owned(),
        trading_environment: wire
            .trading_environment
            .unwrap_or_default()
            .trim()
            .to_ascii_uppercase(),
        market,
        offset,
        limit,
        definition: raw,
        columns,
    })
}

fn normalized_or_default(value: Option<&str>, default: &str, uppercase: bool) -> String {
    let value = value.unwrap_or_default().trim();
    if value.is_empty() {
        return default.to_owned();
    }
    if uppercase {
        value.to_ascii_uppercase()
    } else {
        value.to_ascii_lowercase()
    }
}

fn validate_market(market: &str, catalog: &str) -> Result<(), ParseQueryError> {
    let supported = if catalog == EMBEDDED_CATALOG_VERSION {
        ["HK", "US", "SH", "SZ", "CN"].as_slice()
    } else {
        ["HK", "US", "SH", "SZ"].as_slice()
    };
    if !supported.contains(&market) {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "market: must be one of {}",
            if catalog == EMBEDDED_CATALOG_VERSION {
                "HK, US, SH, SZ or CN"
            } else {
                "HK, US, SH or SZ"
            }
        )));
    }
    Ok(())
}

fn validate_pool(pool: Option<&WirePool>) -> Result<(), ParseQueryError> {
    let Some(pool) = pool else {
        return Ok(());
    };
    if let Some(ids) = &pool.watchlist_stock_ids {
        for (index, id) in ids.iter().enumerate() {
            let id = id.trim();
            if id.is_empty() {
                return Err(ParseQueryError::InvalidDefinition(format!(
                    "pool.watchlistStockIds[{index}]: stock id is required"
                )));
            }
            if id.parse::<u64>().is_err() {
                return Err(ParseQueryError::InvalidDefinition(format!(
                    "pool.watchlistStockIds[{index}]: must be an unsigned integer string"
                )));
            }
        }
    }
    if let Some(plates) = &pool.plates {
        for (index, plate) in plates.iter().enumerate() {
            if plate
                .plate_ids
                .as_ref()
                .is_none_or(|ids| ids.iter().map(|id| id.trim()).all(str::is_empty))
            {
                return Err(ParseQueryError::InvalidDefinition(format!(
                    "pool.plates[{index}].plateIds: at least one plate id is required"
                )));
            }
        }
    }
    Ok(())
}

fn normalize_conditions(
    conditions: &[WireCondition],
    catalog: &str,
    market: &str,
) -> Result<Vec<Value>, ParseQueryError> {
    let mut result = Vec::with_capacity(conditions.len());
    for (index, condition) in conditions.iter().enumerate() {
        let path = format!("conditions[{index}]");
        let factor = condition.factor.as_ref().ok_or_else(|| {
            ParseQueryError::InvalidDefinition(format!(
                "{path}.factor.factorKey: factor key is required"
            ))
        })?;
        let (normalized_factor, factor_key) = normalize_factor(factor, &path, catalog, market)?;
        let operator = condition
            .operator
            .as_deref()
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase();
        let operator = if operator.is_empty() {
            infer_operator(condition.value.as_ref())
        } else {
            operator
        };
        validate_condition_value(&path, &operator, condition.value.as_ref(), &factor_key)?;
        let mut value = Map::new();
        value.insert(
            "id".to_owned(),
            Value::String(normalized_id(condition.id.as_deref(), "condition", index)),
        );
        value.insert("factor".to_owned(), normalized_factor);
        value.insert("operator".to_owned(), Value::String(operator));
        if let Some(condition_value) = &condition.value {
            value.insert("value".to_owned(), condition_value.clone());
        }
        if let Some(second) = &condition.second_factor {
            let (normalized_second, _) =
                normalize_factor(second, &format!("{path}.secondFactor"), catalog, market)?;
            value.insert("secondFactor".to_owned(), normalized_second);
        }
        result.push(Value::Object(value));
    }
    Ok(result)
}

fn normalize_columns(
    columns: &[WireColumn],
    catalog: &str,
    market: &str,
) -> Result<(Vec<Value>, Vec<ResearchScreenColumn>), ParseQueryError> {
    let mut normalized = Vec::with_capacity(columns.len());
    let mut result = Vec::with_capacity(columns.len());
    for (index, column) in columns.iter().enumerate() {
        let path = format!("columns[{index}]");
        let factor = column.factor.as_ref().ok_or_else(|| {
            ParseQueryError::InvalidDefinition(format!(
                "{path}.factor.factorKey: factor key is required"
            ))
        })?;
        let (normalized_factor, factor_key) = normalize_factor(factor, &path, catalog, market)?;
        let column_id = normalized_id(column.column_id.as_deref(), "column", index);
        let label = column.label.as_deref().unwrap_or_default().to_owned();
        let unit = catalog_factor_unit(catalog, market, &factor_key);
        let mut value = Map::new();
        value.insert("columnId".to_owned(), Value::String(column_id.clone()));
        value.insert("factor".to_owned(), normalized_factor.clone());
        if !label.is_empty() {
            value.insert("label".to_owned(), Value::String(label.clone()));
        }
        normalized.push(Value::Object(value));
        result.push(ResearchScreenColumn {
            column_id,
            instance_id: normalized_factor["instanceId"]
                .as_str()
                .unwrap_or_default()
                .to_owned(),
            factor_key,
            label,
            unit,
        });
    }
    Ok((normalized, result))
}

fn normalize_sorts(
    sorts: &[WireSort],
    catalog: &str,
    market: &str,
) -> Result<Vec<Value>, ParseQueryError> {
    let mut result = Vec::with_capacity(sorts.len());
    for (index, sort) in sorts.iter().enumerate() {
        let path = format!("sorts[{index}]");
        let factor = sort.factor.as_ref().ok_or_else(|| {
            ParseQueryError::InvalidDefinition(format!(
                "{path}.factor.factorKey: factor key is required"
            ))
        })?;
        let (normalized_factor, _) = normalize_factor(factor, &path, catalog, market)?;
        let direction = sort
            .direction
            .as_deref()
            .unwrap_or("desc")
            .trim()
            .to_ascii_lowercase();
        if !matches!(direction.as_str(), "asc" | "desc" | "abs_asc" | "abs_desc") {
            return Err(ParseQueryError::InvalidDefinition(format!(
                "{path}.direction: must be asc, desc, abs_asc or abs_desc"
            )));
        }
        let mut value = Map::new();
        value.insert(
            "sortId".to_owned(),
            Value::String(normalized_id(sort.sort_id.as_deref(), "sort", index)),
        );
        if let Some(column_id) = sort.column_id.as_deref().filter(|value| !value.is_empty()) {
            value.insert(
                "columnId".to_owned(),
                Value::String(column_id.trim().to_owned()),
            );
        }
        value.insert("factor".to_owned(), normalized_factor);
        value.insert("direction".to_owned(), Value::String(direction));
        result.push(Value::Object(value));
    }
    Ok(result)
}

fn normalize_factor(
    factor: &WireFactorRef,
    path: &str,
    catalog: &str,
    market: &str,
) -> Result<(Value, String), ParseQueryError> {
    let factor_key = factor
        .factor_key
        .as_deref()
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase();
    if factor_key.is_empty() {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "{path}.factor.factorKey: factor key is required"
        )));
    }
    if !known_factor(catalog, &factor_key) {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "{path}.factor.factorKey: unknown research screen factor {factor_key:?}"
        )));
    }
    let instance_id = factor
        .instance_id
        .as_deref()
        .unwrap_or_default()
        .trim()
        .to_owned();
    let mut value = Map::new();
    value.insert("instanceId".to_owned(), Value::String(instance_id));
    value.insert("factorKey".to_owned(), Value::String(factor_key.clone()));
    if let Some(params) = &factor.params {
        let params = serde_json::to_value(params).unwrap_or_else(|_| json!({}));
        if params.as_object().is_some_and(|object| !object.is_empty()) {
            value.insert("params".to_owned(), params);
        }
    }
    let _ = market;
    Ok((Value::Object(value), factor_key))
}

fn known_factor(catalog: &str, factor: &str) -> bool {
    if catalog == EMBEDDED_CATALOG_VERSION {
        return matches!(
            factor,
            "basic.code"
                | "basic.name"
                | "basic.industry"
                | "simple.price"
                | "simple.change_pct"
                | "simple.volume"
                | "simple.market_cap"
                | "simple.pe_ttm"
                | "simple.pb"
        );
    }
    matches!(
        factor,
        "basic.code" | "basic.name" | "basic.industry" | "simple.price" | "simple.market_cap"
    )
}

fn validate_condition_value(
    path: &str,
    operator: &str,
    value: Option<&Value>,
    _factor: &str,
) -> Result<(), ParseQueryError> {
    let Some(value) = value else {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "{path}.value: condition value is required"
        )));
    };
    if operator != "between" {
        return Ok(());
    }
    let Some(object) = value.as_object() else {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "{path}.value: between requires an object with min/max"
        )));
    };
    let has_min = object.get("min").is_some_and(Value::is_number);
    let has_max = object.get("max").is_some_and(Value::is_number);
    let has_intervals = object
        .get("intervals")
        .and_then(Value::as_array)
        .is_some_and(|values| !values.is_empty());
    if !has_min && !has_max && !has_intervals {
        return Err(ParseQueryError::InvalidDefinition(format!(
            "{path}.value: at least one of min, max or intervals is required"
        )));
    }
    if has_min && has_max {
        let min = object["min"].as_f64().unwrap_or_default();
        let max = object["max"].as_f64().unwrap_or_default();
        if min > max {
            return Err(ParseQueryError::InvalidDefinition(format!(
                "{path}.value: min must not exceed max"
            )));
        }
    }
    Ok(())
}

fn infer_operator(value: Option<&Value>) -> String {
    match value {
        Some(Value::Object(_)) => "between".to_owned(),
        Some(Value::Array(_)) => "in".to_owned(),
        _ => "eq".to_owned(),
    }
}

fn normalized_id(value: Option<&str>, prefix: &str, index: usize) -> String {
    let value = value.unwrap_or_default().trim();
    if value.is_empty() {
        format!("{prefix}-{}", index + 1)
    } else {
        value.to_owned()
    }
}

fn catalog_factor_unit(catalog: &str, market: &str, key: &str) -> String {
    let broker = if catalog == EMBEDDED_CATALOG_VERSION {
        "yfinance"
    } else {
        "futu"
    };
    let Ok(catalog) = jftrade_research::screen_catalog(broker, market) else {
        return String::new();
    };
    catalog["factors"]
        .as_array()
        .and_then(|factors| {
            factors
                .iter()
                .find(|factor| factor["key"].as_str() == Some(key))
        })
        .and_then(|factor| factor["unit"].as_str())
        .unwrap_or_default()
        .to_owned()
}

fn project_result(mut data: Value, query: &ResearchScreenWriteQuery) -> Result<Value, String> {
    let object = data
        .as_object_mut()
        .ok_or_else(|| "broker returned an invalid stock-screen result".to_owned())?;
    let entries = object
        .get("entries")
        .cloned()
        .unwrap_or_else(|| Value::Array(Vec::new()));
    let entries = entries
        .as_array()
        .ok_or_else(|| "broker returned an invalid stock-screen row".to_owned())?;
    let mut projected_entries = Vec::with_capacity(entries.len());
    for entry in entries {
        let mut row = entry
            .as_object()
            .cloned()
            .ok_or_else(|| "broker returned an invalid stock-screen row".to_owned())?;
        match row.get("cells") {
            None | Some(Value::Null) => {
                row.insert("cells".to_owned(), Value::Object(Map::new()));
            }
            Some(Value::Object(_)) => {}
            Some(_) => return Err("broker returned an invalid stock-screen row".to_owned()),
        }
        projected_entries.push(Value::Object(row));
    }
    object.insert("entries".to_owned(), Value::Array(projected_entries));
    object.insert(
        "catalogVersion".to_owned(),
        query.definition["catalogVersion"].clone(),
    );
    object.insert("columns".to_owned(), columns_value(&query.columns));
    if object.get("hasMore").is_none() || object["hasMore"].is_null() {
        object.insert("hasMore".to_owned(), Value::Bool(false));
    }
    Ok(data)
}

fn columns_value(columns: &[ResearchScreenColumn]) -> Value {
    Value::Array(
        columns
            .iter()
            .map(|column| {
                let mut value = Map::new();
                value.insert(
                    "columnId".to_owned(),
                    Value::String(column.column_id.clone()),
                );
                value.insert(
                    "instanceId".to_owned(),
                    Value::String(column.instance_id.clone()),
                );
                value.insert(
                    "factorKey".to_owned(),
                    Value::String(column.factor_key.clone()),
                );
                if !column.label.is_empty() {
                    value.insert("label".to_owned(), Value::String(column.label.clone()));
                }
                if !column.unit.is_empty() {
                    value.insert("unit".to_owned(), Value::String(column.unit.clone()));
                }
                Value::Object(value)
            })
            .collect(),
    )
}

fn port_error_response(
    error: ResearchScreenWritePortError,
    timestamp: &str,
) -> ResearchScreenWriteResponse {
    match error {
        ResearchScreenWritePortError::Unavailable => error_response(
            503,
            "RESEARCH_SCREEN_UNAVAILABLE",
            "research screen query port is unavailable",
            timestamp,
            None,
        ),
        ResearchScreenWritePortError::RateLimited {
            message,
            retry_after,
        } => error_response(
            429,
            "RESEARCH_SCREEN_RATE_LIMITED",
            &message,
            timestamp,
            Some(("Retry-After", retry_after.to_string())),
        ),
        ResearchScreenWritePortError::Capability(message) => error_response(
            409,
            "BROKER_CAPABILITY_UNAVAILABLE",
            &message,
            timestamp,
            None,
        ),
        ResearchScreenWritePortError::ProviderWarming => error_response(
            503,
            "MARKET_DATA_PROVIDER_WARMING",
            "行情服务正在预热，请稍后重试",
            timestamp,
            Some(("Retry-After", "1".to_owned())),
        ),
        ResearchScreenWritePortError::ProviderBusy => error_response(
            503,
            "MARKET_DATA_PROVIDER_BUSY",
            "行情服务当前繁忙，请稍后重试",
            timestamp,
            Some(("Retry-After", "2".to_owned())),
        ),
        ResearchScreenWritePortError::Failed(message) => {
            error_response(502, "BROKER_FEATURE_FAILED", &message, timestamp, None)
        }
    }
}

fn success_response(data: Value, timestamp: &str) -> ResearchScreenWriteResponse {
    ResearchScreenWriteResponse {
        status: 200,
        headers: json_headers(None),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
    extra_header: Option<(&str, String)>,
) -> ResearchScreenWriteResponse {
    ResearchScreenWriteResponse {
        status,
        headers: json_headers(extra_header),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers(extra_header: Option<(&str, String)>) -> BTreeMap<String, String> {
    let mut headers = BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )]);
    if let Some((key, value)) = extra_header {
        headers.insert(key.to_owned(), value);
    }
    headers
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug)]
    struct TestPort;

    impl ResearchScreenWritePort for TestPort {
        fn query(
            &self,
            _request: &ResearchScreenWriteQuery,
        ) -> Result<Value, ResearchScreenWritePortError> {
            Ok(json!({"entries": [], "hasMore": false}))
        }
    }

    #[test]
    fn research_screen_route_inventory_has_only_the_post_route() {
        assert_eq!(
            research_screen_write_routes(),
            &[("POST", RESEARCH_SCREEN_PATH)]
        );
    }

    #[test]
    fn decoder_rejects_unknown_and_trailing_json_before_the_port() {
        for body in [
            br#"{"brokerId":"api-test","unknown":true}"#.as_slice(),
            br#"{"brokerId":"api-test"} {}"#.as_slice(),
        ] {
            let response = dispatch_research_screen_write(
                &ResearchScreenWriteRequest {
                    method: "POST".to_owned(),
                    path: RESEARCH_SCREEN_PATH.to_owned(),
                    body: Some(body.to_vec()),
                },
                Some(&TestPort),
                "2026-08-23T12:00:00Z",
            );
            assert_eq!(response.status, 400);
            assert_eq!(response.body["error"]["message"], INVALID_REQUEST_MESSAGE);
        }
    }

    #[test]
    fn valid_request_fails_closed_without_a_query_port() {
        let response = dispatch_research_screen_write(
            &ResearchScreenWriteRequest {
                method: "POST".to_owned(),
                path: RESEARCH_SCREEN_PATH.to_owned(),
                body: Some(
                    br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#.to_vec(),
                ),
            },
            None,
            "2026-08-23T12:00:00Z",
        );
        assert_eq!(response.status, 503);
        assert_eq!(
            response.body["error"]["code"],
            "RESEARCH_SCREEN_UNAVAILABLE"
        );
    }

    #[test]
    fn route_and_page_validation_precede_provider_calls() {
        let request = ResearchScreenWriteRequest {
            method: "POST".to_owned(),
            path: format!("{RESEARCH_SCREEN_PATH}?market=HK"),
            body: Some(
                br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"page":{"offset":-1}}"#.to_vec(),
            ),
        };
        let response = dispatch_research_screen_write(&request, Some(&TestPort), "fixture-time");
        assert_eq!(response.status, 400);
        assert_eq!(
            response.body["error"]["message"],
            "page.offset must be non-negative"
        );
        let wrong_method = ResearchScreenWriteRequest {
            method: "GET".to_owned(),
            ..request
        };
        assert_eq!(
            dispatch_research_screen_write(&wrong_method, None, "fixture-time").status,
            404
        );
    }
}
