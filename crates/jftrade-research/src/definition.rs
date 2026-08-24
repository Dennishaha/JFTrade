use std::collections::BTreeSet;

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};
use sha2::{Digest, Sha256};
use thiserror::Error;

use crate::catalog::normalization_catalog;

mod parameters;
use parameters::{normalize_factor_params, validate_params};

const FUTU_CATALOG_VERSION: &str = "futu-stock-screen-v1";
const EMBEDDED_CATALOG_VERSION: &str = "embedded-stock-screen-v1";
const DEFAULT_BROKER: &str = "futu";
const QUERY_SCHEMA_VERSION: i64 = 2;
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize, Error)]
#[error("{path}: {message}")]
#[serde(rename_all = "camelCase")]
pub struct DefinitionFieldError {
    pub path: String,
    pub code: String,
    pub message: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenDefinitionV2 {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    broker_id: String,
    #[serde(default)]
    market: String,
    #[serde(default)]
    pool: ScreenPool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    conditions: Vec<ScreenCondition>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    columns: Vec<ScreenColumn>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    sorts: Vec<ScreenSort>,
    #[serde(default)]
    catalog_version: String,
    #[serde(default)]
    query_schema_version: i64,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenPool {
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    watchlist_stock_ids: Vec<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    plates: Vec<ScreenPlate>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenPlate {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    parent_plate_id: String,
    #[serde(default)]
    plate_ids: Vec<String>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenCondition {
    #[serde(default)]
    id: String,
    #[serde(default)]
    factor: FactorRef,
    #[serde(default)]
    operator: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    value: Option<Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    second_factor: Option<FactorRef>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenColumn {
    #[serde(default, rename = "columnId")]
    id: String,
    #[serde(default)]
    factor: FactorRef,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    label: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScreenSort {
    #[serde(default, rename = "sortId", skip_serializing_if = "String::is_empty")]
    id: String,
    #[serde(default, rename = "columnId", skip_serializing_if = "String::is_empty")]
    column_id: String,
    #[serde(default)]
    factor: FactorRef,
    #[serde(default)]
    direction: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FactorRef {
    #[serde(default)]
    instance_id: String,
    #[serde(default)]
    factor_key: String,
    #[serde(default)]
    params: FactorParams,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FactorParams {
    #[serde(default, skip_serializing_if = "is_zero")]
    days: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    period_average: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    term: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    duration: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    year: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    future_duration: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    period: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    range_period: i64,
    #[serde(default, skip_serializing_if = "is_zero")]
    first_custom_param: i64,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    indicator_params: Vec<i64>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    broker_param: String,
    #[serde(default, skip_serializing_if = "is_zero")]
    option_param_type: i64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    option_param_string: String,
    #[serde(default, skip_serializing_if = "is_zero")]
    option_param_integer: i64,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    option_param_integers: Vec<i64>,
    #[serde(default, skip_serializing_if = "is_zero")]
    option_hv_period: i64,
}

fn is_zero(value: &i64) -> bool {
    *value == 0
}

pub fn normalize_definition_v2(input: Value) -> Result<Value, DefinitionFieldError> {
    let mut definition: ScreenDefinitionV2 = serde_json::from_value(input).map_err(|error| {
        issue(
            "definition",
            "invalid_type",
            format!("invalid screen definition: {error}"),
        )
    })?;
    normalize_header_and_pool(&mut definition)?;
    normalize_conditions(&mut definition)?;
    normalize_columns(&mut definition)?;
    normalize_sorts(&mut definition)?;
    serde_json::to_value(definition).map_err(|error| {
        issue(
            "definition",
            "invalid_type",
            format!("encode normalized screen definition: {error}"),
        )
    })
}

fn normalize_header_and_pool(
    definition: &mut ScreenDefinitionV2,
) -> Result<(), DefinitionFieldError> {
    definition.broker_id = definition.broker_id.trim().to_ascii_lowercase();
    if definition.broker_id.is_empty() {
        definition.broker_id = DEFAULT_BROKER.to_owned();
    }
    definition.market = definition.market.trim().to_ascii_uppercase();
    if definition.query_schema_version != QUERY_SCHEMA_VERSION {
        return Err(issue(
            "querySchemaVersion",
            "unsupported_schema",
            "must be 2",
        ));
    }
    if definition.catalog_version != FUTU_CATALOG_VERSION
        && !is_embedded_catalog(&definition.catalog_version)
    {
        return Err(issue(
            "catalogVersion",
            "unsupported_catalog",
            format!("catalog {:?} is not executable", definition.catalog_version),
        ));
    }
    validate_market(&definition.catalog_version, &definition.market)?;
    for (index, plate) in definition.pool.plates.iter_mut().enumerate() {
        plate.parent_plate_id = plate.parent_plate_id.trim().to_owned();
        let mut seen = BTreeSet::new();
        plate.plate_ids.retain_mut(|plate_id| {
            *plate_id = plate_id.trim().to_owned();
            !plate_id.is_empty() && seen.insert(plate_id.clone())
        });
        if plate.plate_ids.is_empty() {
            return Err(issue(
                format!("pool.plates[{index}].plateIds"),
                "required",
                "at least one plate id is required",
            ));
        }
    }
    for (index, stock_id) in definition.pool.watchlist_stock_ids.iter_mut().enumerate() {
        *stock_id = stock_id.trim().to_owned();
        if stock_id.is_empty() {
            return Err(issue(
                format!("pool.watchlistStockIds[{index}]"),
                "required",
                "stock id is required",
            ));
        }
        if stock_id.parse::<u64>().is_err() {
            return Err(issue(
                format!("pool.watchlistStockIds[{index}]"),
                "invalid_id",
                "must be an unsigned integer string",
            ));
        }
    }
    Ok(())
}

fn validate_market(catalog_version: &str, market: &str) -> Result<(), DefinitionFieldError> {
    let supported = if is_embedded_catalog(catalog_version) {
        matches!(market, "HK" | "US" | "SH" | "SZ" | "CN")
    } else {
        matches!(market, "HK" | "US" | "SH" | "SZ")
    };
    if supported {
        return Ok(());
    }
    let message = if is_embedded_catalog(catalog_version) {
        "must be one of HK, US, SH, SZ or CN"
    } else {
        "must be one of HK, US, SH or SZ"
    };
    Err(issue("market", "unsupported_market", message))
}

fn normalize_conditions(definition: &mut ScreenDefinitionV2) -> Result<(), DefinitionFieldError> {
    let mut seen_ids = BTreeSet::new();
    let mut seen_factors = BTreeSet::new();
    for (index, condition) in definition.conditions.iter_mut().enumerate() {
        let path = format!("conditions[{index}]");
        condition.id = condition.id.trim().to_owned();
        if condition.id.is_empty() {
            condition.id = stable_instance_id("condition", &condition.factor, index);
        }
        if !seen_ids.insert(condition.id.clone()) {
            return Err(issue(
                format!("{path}.id"),
                "duplicate",
                "condition id must be unique",
            ));
        }
        validate_ref(
            &format!("{path}.factor"),
            &definition.catalog_version,
            &definition.market,
            &condition.factor,
            FactorRole::Filter,
        )?;
        condition.factor.params = normalize_factor_params(&condition.factor);
        condition.factor.instance_id = normalized_instance_id(&condition.factor, index);
        if !seen_factors.insert(factor_configuration_key(&condition.factor)) {
            return Err(issue(
                format!("{path}.factor"),
                "duplicate_factor",
                "the same factor and parameters already exist",
            ));
        }
        condition.operator = condition.operator.trim().to_ascii_lowercase();
        if condition.operator.is_empty() {
            condition.operator = infer_operator(condition.value.as_ref()).to_owned();
        }
        validate_condition_value(
            &path,
            &definition.catalog_version,
            &definition.market,
            condition,
        )?;
        if let Some(second) = condition.second_factor.as_mut() {
            validate_ref(
                &format!("{path}.secondFactor"),
                &definition.catalog_version,
                &definition.market,
                second,
                FactorRole::Filter,
            )?;
            let first_category = factor_category(
                &definition.catalog_version,
                &definition.market,
                &condition.factor.factor_key,
            );
            let second_category = factor_category(
                &definition.catalog_version,
                &definition.market,
                &second.factor_key,
            );
            if first_category.as_deref() != Some("indicator")
                || second_category.as_deref() != Some("indicator")
            {
                return Err(issue(
                    format!("{path}.secondFactor.factorKey"),
                    "unsupported_factor",
                    "second factor comparisons require two indicators",
                ));
            }
            second.params = normalize_factor_params(second);
            second.instance_id = normalized_instance_id(second, index + 1);
        }
    }
    Ok(())
}

fn normalize_columns(definition: &mut ScreenDefinitionV2) -> Result<(), DefinitionFieldError> {
    let mut seen_ids = BTreeSet::new();
    let mut seen_factors = BTreeSet::new();
    for (index, column) in definition.columns.iter_mut().enumerate() {
        let path = format!("columns[{index}]");
        column.id = column.id.trim().to_owned();
        if column.id.is_empty() {
            column.id = format!("column-{}", index + 1);
        }
        if !seen_ids.insert(column.id.clone()) {
            return Err(issue(
                format!("{path}.id"),
                "duplicate",
                "column id must be unique",
            ));
        }
        validate_ref(
            &format!("{path}.factor"),
            &definition.catalog_version,
            &definition.market,
            &column.factor,
            FactorRole::Retrieve,
        )?;
        column.factor.params = normalize_factor_params(&column.factor);
        column.factor.instance_id = normalized_instance_id(&column.factor, index);
        if !seen_factors.insert(factor_configuration_key(&column.factor)) {
            return Err(issue(
                format!("{path}.factor"),
                "duplicate_factor",
                "the same factor and parameters already exist",
            ));
        }
    }
    Ok(())
}

fn normalize_sorts(definition: &mut ScreenDefinitionV2) -> Result<(), DefinitionFieldError> {
    for (index, sort) in definition.sorts.iter_mut().enumerate() {
        let path = format!("sorts[{index}]");
        sort.id = sort.id.trim().to_owned();
        if sort.id.is_empty() {
            sort.id = format!("sort-{}", index + 1);
        }
        sort.direction = sort.direction.trim().to_ascii_lowercase();
        if sort.direction.is_empty() {
            sort.direction = "desc".to_owned();
        }
        if !matches!(
            sort.direction.as_str(),
            "asc" | "desc" | "abs_asc" | "abs_desc"
        ) {
            return Err(issue(
                format!("{path}.direction"),
                "invalid_operator",
                "must be asc, desc, abs_asc or abs_desc",
            ));
        }
        validate_ref(
            &format!("{path}.factor"),
            &definition.catalog_version,
            &definition.market,
            &sort.factor,
            FactorRole::Sort,
        )?;
        sort.factor.params = normalize_factor_params(&sort.factor);
        sort.factor.instance_id = normalized_instance_id(&sort.factor, index);
    }
    Ok(())
}

#[derive(Clone, Copy)]
enum FactorRole {
    Filter,
    Retrieve,
    Sort,
}

fn validate_ref(
    path: &str,
    catalog_version: &str,
    market: &str,
    factor_ref: &FactorRef,
    role: FactorRole,
) -> Result<(), DefinitionFieldError> {
    let key = factor_ref.factor_key.trim().to_ascii_lowercase();
    if key.is_empty() {
        return Err(issue(
            format!("{path}.factorKey"),
            "required",
            "factor key is required",
        ));
    }
    let catalog = normalization_catalog(catalog_version, market)
        .map_err(|message| issue(format!("{path}.factorKey"), "unsupported_factor", message))?;
    let Some(factor) = find_factor(catalog, &key) else {
        let message = if is_embedded_catalog(catalog_version) {
            format!("unknown embedded research screen factor {key:?}")
        } else {
            format!("unknown research screen factor {key:?}")
        };
        return Err(issue(
            format!("{path}.factorKey"),
            "unsupported_factor",
            message,
        ));
    };
    let role_key = match role {
        FactorRole::Filter => "filter",
        FactorRole::Retrieve => "retrieve",
        FactorRole::Sort => "sort",
    };
    if factor.get(role_key).and_then(Value::as_bool) != Some(true) {
        let action = match role {
            FactorRole::Filter => "filtered",
            FactorRole::Retrieve => "retrieved",
            FactorRole::Sort => "sorted",
        };
        let embedded = if is_embedded_catalog(catalog_version) {
            "embedded "
        } else {
            ""
        };
        return Err(issue(
            format!("{path}.factorKey"),
            "unsupported_factor",
            format!("{embedded}research screen factor {key:?} cannot be {action}"),
        ));
    }
    if !is_embedded_catalog(catalog_version)
        && factor.get("availability").and_then(Value::as_str) == Some("unsupported")
    {
        let reason = factor
            .get("reason")
            .and_then(Value::as_str)
            .unwrap_or_default();
        return Err(issue(
            format!("{path}.factorKey"),
            "unsupported_factor",
            format!("research screen factor {key:?} is unavailable: {reason}"),
        ));
    }
    validate_params(
        &format!("{path}.params"),
        catalog,
        factor,
        &factor_ref.params,
    )
}

fn validate_condition_value(
    path: &str,
    catalog_version: &str,
    market: &str,
    condition: &ScreenCondition,
) -> Result<(), DefinitionFieldError> {
    validate_condition_operator(path, catalog_version, market, condition)?;
    let value = condition.value.as_ref().ok_or_else(|| {
        issue(
            format!("{path}.value"),
            "required",
            "condition value is required",
        )
    })?;
    match condition.operator.as_str() {
        "in" => validate_set_condition_value(path, value),
        "between" => validate_between_condition_value(path, value),
        "position" => validate_position_condition_value(path, condition, value),
        "pattern" => validate_pattern_condition_value(path, value),
        _ => Ok(()),
    }
}

fn validate_condition_operator(
    path: &str,
    catalog_version: &str,
    market: &str,
    condition: &ScreenCondition,
) -> Result<(), DefinitionFieldError> {
    if !matches!(
        condition.operator.as_str(),
        "" | "is"
            | "eq"
            | "ne"
            | "gt"
            | "gte"
            | "lt"
            | "lte"
            | "between"
            | "in"
            | "contains"
            | "crosses"
            | "position"
            | "pattern"
    ) {
        return Err(issue(
            format!("{path}.operator"),
            "unsupported_operator",
            "unsupported condition operator",
        ));
    }
    let Some(filter_kind) = factor_field(
        catalog_version,
        market,
        &condition.factor.factor_key,
        "filterKind",
    ) else {
        return Ok(());
    };
    let valid = match filter_kind.as_str() {
        "enum" | "set" => matches!(condition.operator.as_str(), "in" | "eq" | "is"),
        "interval" => condition.operator == "between",
        "interval_or_set" => matches!(condition.operator.as_str(), "between" | "in"),
        "position" => condition.operator == "position",
        "pattern" => condition.operator == "pattern",
        _ => true,
    };
    if valid {
        return Ok(());
    }
    let message = match filter_kind.as_str() {
        "enum" | "set" => "set factor requires in, eq or is",
        "interval" => "interval factor requires between",
        "interval_or_set" => "factor requires an interval or a value set",
        "position" => "indicator factor requires position",
        "pattern" => "pattern factor requires pattern",
        _ => return Ok(()),
    };
    Err(issue(
        format!("{path}.operator"),
        "unsupported_operator",
        message,
    ))
}

fn validate_set_condition_value(path: &str, value: &Value) -> Result<(), DefinitionFieldError> {
    if value
        .as_array()
        .is_some_and(|values| !values.is_empty() && values.iter().all(Value::is_number))
    {
        return Ok(());
    }
    Err(issue(
        format!("{path}.value"),
        "invalid_set",
        "value set must contain at least one integer",
    ))
}

fn validate_between_condition_value(path: &str, value: &Value) -> Result<(), DefinitionFieldError> {
    let Some(range) = value.as_object() else {
        return Err(issue(
            format!("{path}.value"),
            "invalid_range",
            "between requires an object with min/max",
        ));
    };
    let has_minimum = range.get("min").is_some_and(Value::is_number);
    let has_maximum = range.get("max").is_some_and(Value::is_number);
    if has_minimum || has_maximum {
        validate_range_value(&format!("{path}.value"), range)?;
    }
    let mut has_intervals = false;
    if let Some(intervals) = range.get("intervals") {
        let Some(intervals) = intervals.as_array() else {
            return Err(issue(
                format!("{path}.value.intervals"),
                "invalid_range",
                "intervals must be an array",
            ));
        };
        has_intervals = !intervals.is_empty();
        for (index, interval) in intervals.iter().enumerate() {
            let Some(interval) = interval.as_object() else {
                return Err(issue(
                    format!("{path}.value.intervals[{index}]"),
                    "invalid_range",
                    "interval must be an object",
                ));
            };
            validate_range_value(&format!("{path}.value.intervals[{index}]"), interval)?;
        }
    }
    if !has_minimum && !has_maximum && !has_intervals {
        return Err(issue(
            format!("{path}.value"),
            "invalid_range",
            "at least one of min, max or intervals is required",
        ));
    }
    Ok(())
}

fn validate_range_value(
    path: &str,
    range: &Map<String, Value>,
) -> Result<(), DefinitionFieldError> {
    let minimum = range.get("min").and_then(Value::as_f64);
    let maximum = range.get("max").and_then(Value::as_f64);
    if minimum.is_none() && maximum.is_none() {
        return Err(issue(
            path,
            "invalid_range",
            "at least one of min or max is required",
        ));
    }
    if minimum.is_some_and(|value| value.is_nan())
        || maximum.is_some_and(|value| value.is_nan())
        || minimum.zip(maximum).is_some_and(|(min, max)| min > max)
    {
        return Err(issue(path, "invalid_range", "min must not exceed max"));
    }
    Ok(())
}

fn validate_position_condition_value(
    path: &str,
    condition: &ScreenCondition,
    value: &Value,
) -> Result<(), DefinitionFieldError> {
    let Some(value) = value.as_object() else {
        return Err(issue(
            format!("{path}.value"),
            "invalid_position",
            "position requires an object",
        ));
    };
    let position = value.get("position").and_then(Value::as_f64);
    if position.is_none_or(|position| position.fract() != 0.0 || !(1.0..=4.0).contains(&position)) {
        return Err(issue(
            format!("{path}.value.position"),
            "invalid_position",
            "position must be an integer from 1 to 4",
        ));
    }
    if condition.second_factor.is_none()
        && value.get("secondValue").and_then(Value::as_f64).is_none()
    {
        return Err(issue(
            format!("{path}.value.secondValue"),
            "required",
            "secondValue or secondFactor is required",
        ));
    }
    Ok(())
}

fn validate_pattern_condition_value(path: &str, value: &Value) -> Result<(), DefinitionFieldError> {
    let Some(value) = value.as_object() else {
        return Err(issue(
            format!("{path}.value"),
            "invalid_pattern",
            "pattern requires an object",
        ));
    };
    if value.get("match").is_some_and(|value| !value.is_boolean()) {
        return Err(issue(
            format!("{path}.value.match"),
            "invalid_pattern",
            "match must be boolean",
        ));
    }
    Ok(())
}

fn find_factor<'a>(catalog: &'a Value, key: &str) -> Option<&'a Map<String, Value>> {
    catalog
        .get("factors")?
        .as_array()?
        .iter()
        .find(|factor| factor.get("key").and_then(Value::as_str) == Some(key))?
        .as_object()
}

fn factor_category(catalog_version: &str, market: &str, key: &str) -> Option<String> {
    factor_field(catalog_version, market, key, "category")
}

fn factor_field(catalog_version: &str, market: &str, key: &str, field: &str) -> Option<String> {
    let catalog = normalization_catalog(catalog_version, market).ok()?;
    find_factor(catalog, &key.trim().to_ascii_lowercase())?
        .get(field)?
        .as_str()
        .map(str::to_owned)
}

fn enum_contains(catalog: &Value, enum_name: &str, value: i64) -> bool {
    catalog
        .get("enums")
        .and_then(|enums| enums.get(enum_name))
        .and_then(Value::as_array)
        .is_some_and(|values| {
            values
                .iter()
                .any(|option| option.get("value").and_then(Value::as_i64) == Some(value))
        })
}

fn params_value(params: &FactorParams) -> Map<String, Value> {
    serde_json::to_value(params)
        .ok()
        .and_then(|value| value.as_object().cloned())
        .unwrap_or_default()
}

fn is_missing_json_value(value: &Value) -> bool {
    value.is_null() || value.as_str() == Some("") || value.as_array().is_some_and(Vec::is_empty)
}

fn infer_operator(value: Option<&Value>) -> &'static str {
    match value {
        Some(Value::Object(_)) => "between",
        Some(Value::Array(_)) => "in",
        _ => "eq",
    }
}

fn normalized_instance_id(factor_ref: &FactorRef, index: usize) -> String {
    let value = factor_ref.instance_id.trim();
    if value.is_empty() {
        stable_instance_id("factor", factor_ref, index)
    } else {
        value.to_owned()
    }
}

fn stable_instance_id(prefix: &str, factor_ref: &FactorRef, index: usize) -> String {
    #[derive(Serialize)]
    struct StableFactor<'a> {
        key: &'a str,
        params: &'a FactorParams,
    }
    let mut content = serde_json::to_vec(&StableFactor {
        key: &factor_ref.factor_key,
        params: &factor_ref.params,
    })
    .unwrap_or_default();
    content.extend_from_slice(index.to_string().as_bytes());
    let digest = Sha256::digest(content);
    let suffix = digest[..5]
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    format!(
        "{prefix}-{}-{suffix}",
        factor_ref.factor_key.trim_matches('.').to_ascii_lowercase()
    )
}

fn factor_configuration_key(factor_ref: &FactorRef) -> String {
    let params = serde_json::to_string(&factor_ref.params).unwrap_or_default();
    format!(
        "{}:{params}",
        factor_ref.factor_key.trim().to_ascii_lowercase()
    )
}

fn is_embedded_catalog(value: &str) -> bool {
    value.trim().eq_ignore_ascii_case(EMBEDDED_CATALOG_VERSION)
}

fn issue(
    path: impl Into<String>,
    code: impl Into<String>,
    message: impl Into<String>,
) -> DefinitionFieldError {
    DefinitionFieldError {
        path: path.into(),
        code: code.into(),
        message: message.into(),
    }
}
