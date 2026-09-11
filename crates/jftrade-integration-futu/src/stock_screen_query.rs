//! Typed OpenD stock-screen reader (`Qot_StockScreen`, protocol 3252).
//!
//! The research domain owns the normalized V2 definition.  This adapter is the
//! only place that turns that definition into OpenD protobuf messages and
//! turns the response back into provider-neutral rows.  In particular, A-share
//! rows are resolved through `Qot_GetStaticInfo`; the request market is never
//! used as a substitute for an authoritative SH/SZ identity.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use prost::Message;
use serde::Serialize;
use serde_json::{Map, Value};
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

use crate::stock_screen_factors::FACTORS;

pub const STOCK_SCREEN_PROTOCOL_ID: u32 = 3252;
pub const STOCK_SCREEN_LIMIT: usize = 10;
pub const STOCK_SCREEN_WINDOW: Duration = Duration::from_secs(30);

/// A normalized V2 definition plus the page requested by the public route.
/// The definition is validated by `jftrade-research` before it reaches this
/// boundary; the adapter still validates bounds and protocol-critical fields.
#[derive(Clone, Debug, PartialEq)]
pub struct StockScreenQuery {
    pub market: String,
    pub definition: Value,
    pub page_from: i32,
    pub page_count: i32,
}

impl StockScreenQuery {
    pub fn new(
        market: impl Into<String>,
        definition: Value,
        page_from: i64,
        page_count: i64,
    ) -> Result<Self, StockScreenQueryError> {
        let page_from = i32::try_from(page_from).map_err(|_| {
            StockScreenQueryError::InvalidQuery("stock-screen pageFrom is out of range".into())
        })?;
        let page_count = i32::try_from(page_count).map_err(|_| {
            StockScreenQueryError::InvalidQuery("stock-screen pageCount is out of range".into())
        })?;
        let query = Self {
            market: market.into().trim().to_ascii_uppercase(),
            definition,
            page_from,
            page_count,
        };
        validate_query(&query)?;
        Ok(query)
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenSecurity {
    pub market: String,
    pub code: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenPropertyParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub days: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub period_average: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub term: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duration: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub year: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub future_duration: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub period: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub range_period: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub first_custom_param: Option<i64>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub indicator_params: Vec<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub broker_param: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_param: Option<StockScreenParam>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_hv_period: Option<i32>,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenParam {
    #[serde(skip_serializing_if = "Option::is_none", rename = "type")]
    pub value_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub string_value: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub integer_value: Option<i64>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub integer_values: Vec<i64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenProperty {
    pub category: String,
    pub provider_id: i32,
    pub factor_key: String,
    pub params: StockScreenPropertyParams,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "snake_case", tag = "type")]
pub enum StockScreenValue {
    Missing,
    String { value: String },
    Integer { value: i64 },
    IntegerArray { values: Vec<i64> },
    Number { value: f64 },
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenResult {
    pub factor_key: String,
    pub property: StockScreenProperty,
    pub value: StockScreenValue,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub enum_type_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub enum_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub end_time: Option<u32>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenItem {
    pub stock_id: u64,
    pub results: Vec<StockScreenResult>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub security: Option<StockScreenSecurity>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StockScreenPage {
    pub last_page: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
    pub items: Vec<StockScreenItem>,
}

pub trait StockScreenReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &StockScreenQuery) -> Result<StockScreenPage, StockScreenQueryError>;
}

#[derive(Clone, Debug, Default)]
pub struct StockScreenLimiter {
    requests: Vec<Instant>,
}

impl StockScreenLimiter {
    pub fn retry_after(&mut self) -> Option<Duration> {
        let now = Instant::now();
        self.requests
            .retain(|request| now.duration_since(*request) < STOCK_SCREEN_WINDOW);
        if self.requests.len() >= STOCK_SCREEN_LIMIT {
            return Some(
                STOCK_SCREEN_WINDOW
                    .saturating_sub(now.duration_since(self.requests[0]))
                    .max(Duration::from_millis(1)),
            );
        }
        self.requests.push(now);
        None
    }
}

#[derive(Clone)]
pub struct OpenDStockScreenReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    limiter: Arc<Mutex<StockScreenLimiter>>,
}

impl std::fmt::Debug for OpenDStockScreenReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDStockScreenReader")
            .finish_non_exhaustive()
    }
}

impl OpenDStockScreenReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self {
            coordinator,
            limiter: Arc::new(Mutex::new(StockScreenLimiter::default())),
        }
    }
}

impl StockScreenReadPort for OpenDStockScreenReader {
    fn query(&self, query: &StockScreenQuery) -> Result<StockScreenPage, StockScreenQueryError> {
        validate_query(query)?;
        let request = encode_request(query)?;
        let retry_after = self
            .limiter
            .lock()
            .map_err(|_| {
                StockScreenQueryError::InvalidQuery("stock-screen limiter is closed".into())
            })?
            .retry_after();
        if let Some(delay) = retry_after {
            return Err(StockScreenQueryError::RateLimited {
                retry_after_ms: delay.as_millis() as u64,
            });
        }
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| StockScreenQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(STOCK_SCREEN_PROTOCOL_ID, &request)
            .map_err(OpenDSessionCoordinatorError::from)?;
        let mut page = decode_response(&response)?;
        resolve_identities(&coordinator, query, &mut page)?;
        Ok(page)
    }
}

#[derive(Debug, Error)]
pub enum StockScreenQueryError {
    #[error("invalid stock-screen query: {0}")]
    InvalidQuery(String),
    #[error("stock-screen request is rate limited; retry after {retry_after_ms}ms")]
    RateLimited { retry_after_ms: u64 },
    #[error("decode OpenD stock-screen response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD stock-screen request rejected ({ret_type}/{err_code}): {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD stock-screen response is missing s2c")]
    MissingS2c,
    #[error("invalid OpenD stock-screen response: {0}")]
    InvalidResponse(String),
    #[error("OpenD stock-screen session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}

#[derive(Clone, Debug)]
struct FactorSpec {
    key: String,
    category: &'static str,
    provider_id: i32,
    params: Map<String, Value>,
}

fn validate_query(query: &StockScreenQuery) -> Result<(), StockScreenQueryError> {
    if !matches!(query.market.as_str(), "HK" | "US" | "SH" | "SZ" | "CN") {
        return Err(StockScreenQueryError::InvalidQuery(
            "stock-screen market is unsupported".into(),
        ));
    }
    if query.page_from < 0 {
        return Err(StockScreenQueryError::InvalidQuery(
            "stock-screen pageFrom must be non-negative".into(),
        ));
    }
    if !(1..=300).contains(&query.page_count) {
        return Err(StockScreenQueryError::InvalidQuery(
            "stock-screen pageCount must be between 1 and 300".into(),
        ));
    }
    query.definition.as_object().ok_or_else(|| {
        StockScreenQueryError::InvalidQuery("stock-screen definition must be an object".into())
    })?;
    Ok(())
}

fn encode_request(query: &StockScreenQuery) -> Result<Vec<u8>, StockScreenQueryError> {
    use crate::trade_proto::qot_stock_screen::{C2s, Request};
    let definition = query.definition.as_object().ok_or_else(|| {
        StockScreenQueryError::InvalidQuery("stock-screen definition must be an object".into())
    })?;
    let mut filters = Vec::new();
    let mut has_market = false;
    if let Some(conditions) = definition.get("conditions").and_then(Value::as_array) {
        for (index, condition) in conditions.iter().enumerate() {
            let (filter, is_market) =
                encode_condition(condition, &query.market).map_err(|error| {
                    StockScreenQueryError::InvalidQuery(format!("conditions[{index}]: {error}"))
                })?;
            has_market |= is_market;
            filters.push(filter);
        }
    }
    if !has_market {
        filters.insert(0, market_filter(&query.market)?);
    }
    let pool = definition.get("pool").and_then(Value::as_object);
    if let Some(plates) = pool
        .and_then(|pool| pool.get("plates"))
        .and_then(Value::as_array)
    {
        let mut plate_list = Vec::new();
        for (index, plate) in plates.iter().enumerate() {
            let plate = plate.as_object().ok_or_else(|| {
                StockScreenQueryError::InvalidQuery(format!("pool.plates[{index}] is invalid"))
            })?;
            let ids = plate
                .get("plateIds")
                .and_then(Value::as_array)
                .ok_or_else(|| {
                    StockScreenQueryError::InvalidQuery(format!(
                        "pool.plates[{index}].plateIds is required"
                    ))
                })?
                .iter()
                .map(|value| {
                    value
                        .as_str()
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                        .map(ToOwned::to_owned)
                })
                .collect::<Option<Vec<_>>>()
                .filter(|ids| !ids.is_empty())
                .ok_or_else(|| {
                    StockScreenQueryError::InvalidQuery(format!(
                        "pool.plates[{index}].plateIds is required"
                    ))
                })?;
            plate_list.push(crate::trade_proto::qot_stock_screen::Plate {
                parent_plate_id: plate
                    .get("parentPlateId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(ToOwned::to_owned),
                plate_id_list: ids,
            });
        }
        if !plate_list.is_empty() {
            filters.push(crate::trade_proto::qot_stock_screen::ScreenQuery {
                simple_field_query: None,
                plate_query: Some(crate::trade_proto::qot_stock_screen::QueryPlate { plate_list }),
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            });
        }
    }
    let watchlist_stock_ids = pool
        .and_then(|pool| pool.get("watchlistStockIds"))
        .and_then(Value::as_array)
        .map_or(&[] as &[Value], |values| values.as_slice())
        .iter()
        .map(|value| {
            value
                .as_str()
                .ok_or_else(|| {
                    StockScreenQueryError::InvalidQuery(
                        "watchlistStockIds must contain strings".into(),
                    )
                })?
                .parse::<u64>()
                .map_err(|_| {
                    StockScreenQueryError::InvalidQuery(
                        "watchlistStockIds contains an invalid id".into(),
                    )
                })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let mut retrieve_list = Vec::new();
    let mut seen_retrieve = BTreeSet::new();
    let mut has_basic_code = false;
    if let Some(columns) = definition.get("columns").and_then(Value::as_array) {
        for (index, column) in columns.iter().enumerate() {
            let factor = column
                .as_object()
                .and_then(|object| object.get("factor"))
                .ok_or_else(|| {
                    StockScreenQueryError::InvalidQuery(format!(
                        "columns[{index}]: factor is required"
                    ))
                })?;
            let spec = factor_spec(factor).map_err(|error| {
                StockScreenQueryError::InvalidQuery(format!("columns[{index}]: {error}"))
            })?;
            has_basic_code |= spec.key == "basic.code";
            let key = format!(
                "{}:{}",
                spec.key,
                serde_json::to_string(&spec.params).unwrap_or_default()
            );
            if seen_retrieve.insert(key) {
                retrieve_list.push(encode_retrieve(column).map_err(|error| {
                    StockScreenQueryError::InvalidQuery(format!("columns[{index}]: {error}"))
                })?);
            }
        }
    }
    // A stock-screen row only carries the provider stockId.  Always request
    // basic.code internally so the identity resolver can attach a stable
    // instrumentId even when the caller only asks for numeric columns.  The
    // implicit value is not exposed as an extra public cell.
    if !has_basic_code {
        retrieve_list.insert(
            0,
            crate::trade_proto::qot_stock_screen::RetrieveQuery {
                basic_property: Some(crate::trade_proto::qot_stock_screen::PropertyBasic {
                    name: Some(1101),
                }),
                simple_property: None,
                cumulative_property: None,
                financial_property: None,
                indicator_property: None,
                featured_property: None,
                broker_property: None,
                option_property: None,
                kline_shape_property: None,
            },
        );
    }
    let sort_list = definition
        .get("sorts")
        .and_then(Value::as_array)
        .map_or(&[] as &[Value], |values| values.as_slice())
        .iter()
        .enumerate()
        .map(|(index, sort)| {
            encode_sort(sort).map_err(|error| {
                StockScreenQueryError::InvalidQuery(format!("sorts[{index}]: {error}"))
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    Ok(Request {
        c2s: C2s {
            filter_list: filters,
            retrieve_list,
            watchlist_stock_ids,
            sort: None,
            page_from: Some(query.page_from),
            page_count: Some(query.page_count),
            sort_list,
        },
    }
    .encode_to_vec())
}

fn factor_spec(value: &Value) -> Result<FactorSpec, String> {
    let object = value
        .as_object()
        .ok_or_else(|| "factor must be an object".to_owned())?;
    let key = object
        .get("factorKey")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| "factorKey is required".to_owned())?
        .to_ascii_lowercase();
    let (category, provider_id) = FACTORS
        .iter()
        .find(|(candidate, _, _)| *candidate == key)
        .map(|(_, category, provider_id)| (*category, *provider_id))
        .ok_or_else(|| format!("unknown stock-screen factor {key:?}"))?;
    let params = object
        .get("params")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    Ok(FactorSpec {
        key,
        category,
        provider_id,
        params,
    })
}

fn factor_from_condition(condition: &Value) -> Result<FactorSpec, String> {
    let object = condition
        .as_object()
        .ok_or_else(|| "condition must be an object".to_owned())?;
    factor_spec(
        object
            .get("factor")
            .ok_or_else(|| "factor is required".to_owned())?,
    )
}

fn market_filter(
    market: &str,
) -> Result<crate::trade_proto::qot_stock_screen::ScreenQuery, StockScreenQueryError> {
    let value = market_value(market)?;
    Ok(crate::trade_proto::qot_stock_screen::ScreenQuery {
        simple_field_query: Some(crate::trade_proto::qot_stock_screen::QuerySimpleField {
            simple_field: Some(1),
            screen_value_list: vec![value],
        }),
        plate_query: None,
        simple_property_query: None,
        cumulative_property_query: None,
        financial_property_query: None,
        indicator_positional_query: None,
        indicator_pattern_query: None,
        featured_property_query: None,
        broker_holdings_query: None,
        kline_shape_query: None,
        option_query: None,
    })
}

fn market_value(market: &str) -> Result<i64, StockScreenQueryError> {
    match market {
        "HK" => Ok(1),
        "US" => Ok(2),
        "SH" | "SZ" | "CN" => Ok(3),
        _ => Err(StockScreenQueryError::InvalidQuery(
            "stock-screen market is unsupported".into(),
        )),
    }
}

fn encode_condition(
    condition: &Value,
    market: &str,
) -> Result<(crate::trade_proto::qot_stock_screen::ScreenQuery, bool), String> {
    use crate::trade_proto::qot_stock_screen as wire;
    let object = condition
        .as_object()
        .ok_or_else(|| "condition must be an object".to_owned())?;
    let spec = factor_from_condition(condition)?;
    let value = object.get("value").cloned().unwrap_or(Value::Null);
    match spec.category {
        "field" => {
            let values = integer_values(&value)?;
            if values.is_empty() {
                return Err("field factor requires at least one value".into());
            }
            let is_market = spec.key == "field.market";
            if is_market {
                let expected = market_value(market).map_err(|error| error.to_string())?;
                if values.len() != 1 || values[0] != expected {
                    return Err(format!("market factor must match request market {market}"));
                }
            }
            Ok((
                wire::ScreenQuery {
                    simple_field_query: Some(wire::QuerySimpleField {
                        simple_field: Some(spec.provider_id),
                        screen_value_list: values,
                    }),
                    plate_query: None,
                    simple_property_query: None,
                    cumulative_property_query: None,
                    financial_property_query: None,
                    indicator_positional_query: None,
                    indicator_pattern_query: None,
                    featured_property_query: None,
                    broker_holdings_query: None,
                    kline_shape_query: None,
                    option_query: None,
                },
                is_market,
            ))
        }
        "simple" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: Some(wire::QueryPropertySimple {
                    property: simple_property(&spec)?,
                    filter_min: boundary(&value, "min")?,
                    filter_max: boundary(&value, "max")?,
                    unit: None,
                }),
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "cumulative" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: Some(wire::QueryPropertyCumulative {
                    property: cumulative_property(&spec)?,
                    filter_min: boundary(&value, "min")?,
                    filter_max: boundary(&value, "max")?,
                    continuous_period: int_param(&value, "continuousPeriod")?,
                    unit: None,
                }),
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "financial" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: Some(wire::QueryPropertyFinancial {
                    property: financial_property(&spec)?,
                    filter_min: boundary(&value, "min")?,
                    filter_max: boundary(&value, "max")?,
                    continuous_period: int_param(&value, "continuousPeriod")?,
                    unit: None,
                }),
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "indicator" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: Some(indicator_condition(&spec, object)?),
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "pattern" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: Some(pattern_condition(&spec, &value)?),
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "featured" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: Some(wire::QueryPropertyFeatured {
                    property: Some(featured_property(&spec)?),
                    filter_min: None,
                    filter_max: None,
                    intervals: intervals(&value)?,
                    value_set: integer_values(&value).unwrap_or_default(),
                }),
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "broker" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: Some(wire::QueryPropertyBroker {
                    property: Some(broker_property(&spec)?),
                    intervals: intervals(&value)?,
                }),
                kline_shape_query: None,
                option_query: None,
            },
            false,
        )),
        "option" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: None,
                option_query: Some(wire::QueryPropertyOption {
                    property: Some(option_property(&spec)?),
                    intervals: intervals(&value)?,
                }),
            },
            false,
        )),
        "kline_shape" => Ok((
            wire::ScreenQuery {
                simple_field_query: None,
                plate_query: None,
                simple_property_query: None,
                cumulative_property_query: None,
                financial_property_query: None,
                indicator_positional_query: None,
                indicator_pattern_query: None,
                featured_property_query: None,
                broker_holdings_query: None,
                kline_shape_query: Some(wire::QueryKlineShape {
                    property: Some(kline_shape_property(&spec)?),
                    value_set: integer_values(&value)?,
                }),
                option_query: None,
            },
            false,
        )),
        category => Err(format!("unsupported factor category {category:?}")),
    }
}

fn encode_retrieve(
    column: &Value,
) -> Result<crate::trade_proto::qot_stock_screen::RetrieveQuery, String> {
    use crate::trade_proto::qot_stock_screen as wire;
    let object = column
        .as_object()
        .ok_or_else(|| "column must be an object".to_owned())?;
    let spec = factor_spec(
        object
            .get("factor")
            .ok_or_else(|| "factor is required".to_owned())?,
    )?;
    Ok(match spec.category {
        "basic" => wire::RetrieveQuery {
            basic_property: Some(wire::PropertyBasic {
                name: Some(spec.provider_id),
            }),
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "simple" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: Some(simple_property(&spec)?),
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "cumulative" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: Some(cumulative_property(&spec)?),
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "financial" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: Some(financial_property(&spec)?),
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "indicator" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: Some(indicator_property(&spec)?),
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "featured" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: Some(featured_property(&spec)?),
            broker_property: None,
            option_property: None,
            kline_shape_property: None,
        },
        "broker" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: Some(broker_property(&spec)?),
            option_property: None,
            kline_shape_property: None,
        },
        "option" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: Some(option_property(&spec)?),
            kline_shape_property: None,
        },
        "kline_shape" => wire::RetrieveQuery {
            basic_property: None,
            simple_property: None,
            cumulative_property: None,
            financial_property: None,
            indicator_property: None,
            featured_property: None,
            broker_property: None,
            option_property: None,
            kline_shape_property: Some(kline_shape_property(&spec)?),
        },
        category => return Err(format!("unsupported factor category {category:?}")),
    })
}

fn encode_sort(sort: &Value) -> Result<crate::trade_proto::qot_stock_screen::Sort, String> {
    use crate::trade_proto::qot_stock_screen as wire;
    let object = sort
        .as_object()
        .ok_or_else(|| "sort must be an object".to_owned())?;
    let spec = factor_spec(
        object
            .get("factor")
            .ok_or_else(|| "factor is required".to_owned())?,
    )?;
    let direction = match object
        .get("direction")
        .and_then(Value::as_str)
        .unwrap_or("desc")
        .trim()
        .to_ascii_lowercase()
        .as_str()
    {
        "asc" => 1,
        "desc" | "" => 2,
        "abs_asc" => 3,
        "abs_desc" => 4,
        value => return Err(format!("unsupported sort direction {value:?}")),
    };
    let mut result = wire::Sort {
        direction,
        basic_property: None,
        simple_property: None,
        cumulative_property: None,
        financial_property: None,
        indicator_property: None,
        featured_property: None,
        broker_property: None,
        option_property: None,
        kline_shape_property: None,
    };
    match spec.category {
        "basic" => {
            result.basic_property = Some(wire::PropertyBasic {
                name: Some(spec.provider_id),
            })
        }
        "simple" => result.simple_property = Some(simple_property(&spec)?),
        "cumulative" => result.cumulative_property = Some(cumulative_property(&spec)?),
        "financial" => result.financial_property = Some(financial_property(&spec)?),
        "indicator" => result.indicator_property = Some(indicator_property(&spec)?),
        "featured" => result.featured_property = Some(featured_property(&spec)?),
        "broker" => result.broker_property = Some(broker_property(&spec)?),
        "option" => result.option_property = Some(option_property(&spec)?),
        "kline_shape" => result.kline_shape_property = Some(kline_shape_property(&spec)?),
        category => return Err(format!("unsupported factor category {category:?}")),
    }
    Ok(result)
}

fn property_params(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertySimple, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertySimple {
        name: Some(spec.provider_id),
    })
}
fn simple_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertySimple, String> {
    property_params(spec)
}
fn cumulative_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyCumulative, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyCumulative {
        name: Some(spec.provider_id),
        days: Some(u32_param(&spec.params, "days").unwrap_or(1)),
        period_average: i32_param_map(&spec.params, "periodAverage")?,
    })
}
fn financial_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyFinancial, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyFinancial {
        name: Some(spec.provider_id),
        term: i32_param_map(&spec.params, "term")?,
        duration: int_param_map(&spec.params, "duration")?,
        year: int_param_map(&spec.params, "year")?.map(|value| value as i32),
        period_average: i32_param_map(&spec.params, "periodAverage")?,
        future_duration: int_param_map(&spec.params, "futureDuration")?.map(|value| value as i32),
    })
}
fn indicator_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyIndicator, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyIndicator {
        name: Some(spec.provider_id),
        period: Some(i32_param_map(&spec.params, "period")?.unwrap_or(11)),
        indicator_params: int_array_param(&spec.params, "indicatorParams")?,
    })
}
fn featured_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyFeatured, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyFeatured {
        name: Some(spec.provider_id),
        period: int_param_map(&spec.params, "period")?.map(|value| value as i32),
        range_period: int_param_map(&spec.params, "rangePeriod")?.map(|value| value as i32),
        first_custom_param: int_param_map(&spec.params, "firstCustomParam")?,
    })
}
fn broker_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyBroker, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyBroker {
        name: Some(spec.provider_id),
        days: int_param_map(&spec.params, "days")?.map(|value| value as i32),
        param: spec
            .params
            .get("brokerParam")
            .and_then(Value::as_str)
            .map(ToOwned::to_owned),
    })
}
fn option_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyOption, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyOption {
        name: Some(spec.provider_id),
        param: option_param(&spec.params)?,
        period: int_param_map(&spec.params, "optionHvPeriod")?.map(|value| value as i32),
    })
}
fn kline_shape_property(
    spec: &FactorSpec,
) -> Result<crate::trade_proto::qot_stock_screen::PropertyKlineShape, String> {
    Ok(crate::trade_proto::qot_stock_screen::PropertyKlineShape {
        name: Some(spec.provider_id),
        period: int_param_map(&spec.params, "period")?.map(|value| value as i32),
    })
}

fn option_param(
    params: &Map<String, Value>,
) -> Result<Option<crate::trade_proto::qot_stock_screen::Param>, String> {
    let value_type = int_param_map(params, "optionParamType")?.map(|value| value as i32);
    let string_value = params
        .get("optionParamString")
        .and_then(Value::as_str)
        .map(ToOwned::to_owned);
    let integer_value = int_param_map(params, "optionParamInteger")?;
    let integer_values = int_array_param(params, "optionParamIntegers")?;
    if value_type.is_none()
        && string_value.is_none()
        && integer_value.is_none()
        && integer_values.is_empty()
    {
        return Ok(None);
    }
    Ok(Some(crate::trade_proto::qot_stock_screen::Param {
        r#type: value_type,
        sval: string_value,
        ival: integer_value,
        aval: integer_values,
    }))
}

fn indicator_condition(
    spec: &FactorSpec,
    object: &Map<String, Value>,
) -> Result<crate::trade_proto::qot_stock_screen::QueryIndicatorPositional, String> {
    let position = required_i32(object.get("value").and_then(Value::as_object), "position")?;
    if !(1..=4).contains(&position) {
        return Err("indicator position must be 1..4".into());
    }
    let period = i32_param_map(&spec.params, "period")?.unwrap_or(11);
    let second = object.get("secondFactor").map(factor_spec).transpose()?;
    if second
        .as_ref()
        .is_some_and(|factor| factor.category != "indicator")
    {
        return Err("second factor must be an indicator".into());
    }
    let value = object.get("value").and_then(Value::as_object);
    Ok(
        crate::trade_proto::qot_stock_screen::QueryIndicatorPositional {
            position,
            period,
            first_indicator: spec.provider_id,
            second_indicator: second.as_ref().map(|factor| factor.provider_id),
            second_value: value
                .and_then(|value| integer_value(value.get("secondValue")).ok())
                .flatten(),
            first_indicator_params: int_array_param(&spec.params, "indicatorParams")?,
            second_indicator_params: second
                .map(|factor| int_array_param(&factor.params, "indicatorParams"))
                .transpose()?
                .unwrap_or_default(),
            continuous_period: value
                .and_then(|value| int_param_map(value, "continuousPeriod").ok())
                .flatten()
                .map(|value| value as i32),
            intervals: intervals(&object.get("value").cloned().unwrap_or(Value::Null))?,
            first_indicator_name: Some(spec.provider_id),
            period_type: Some(period),
        },
    )
}

fn pattern_condition(
    spec: &FactorSpec,
    value: &Value,
) -> Result<crate::trade_proto::qot_stock_screen::QueryIndicatorPattern, String> {
    let object = value.as_object();
    let period = i32_param_map(&spec.params, "period")?.unwrap_or(11);
    Ok(
        crate::trade_proto::qot_stock_screen::QueryIndicatorPattern {
            pattern: Some(spec.provider_id),
            period,
            continuous_period: object
                .and_then(|value| int_param_map(value, "continuousPeriod").ok())
                .flatten()
                .map(|value| value as i32),
            name: Some(spec.provider_id),
            is_matching: Some(
                object
                    .and_then(|value| value.get("match"))
                    .and_then(Value::as_bool)
                    .unwrap_or(true),
            ),
            sub_patterns: object
                .and_then(|value| value.get("values"))
                .map(integer_values)
                .transpose()?
                .unwrap_or_default()
                .into_iter()
                .map(|value| value as i32)
                .collect(),
            period_type: Some(period),
        },
    )
}

fn required_i32(object: Option<&Map<String, Value>>, key: &str) -> Result<i32, String> {
    object
        .and_then(|object| object.get(key))
        .and_then(|value| integer_value(Some(value)).ok())
        .flatten()
        .and_then(|value| i32::try_from(value).ok())
        .ok_or_else(|| format!("{key} is required"))
}
fn int_param(value: &Value, key: &str) -> Result<Option<i32>, String> {
    let Some(object) = value.as_object() else {
        return Ok(None);
    };
    i32_param_map(object, key)
}
fn int_param_map(map: &Map<String, Value>, key: &str) -> Result<Option<i64>, String> {
    map.get(key)
        .map(|value| integer_value(Some(value)))
        .transpose()
        .map(|value| value.flatten())
}
fn i32_param_map(map: &Map<String, Value>, key: &str) -> Result<Option<i32>, String> {
    int_param_map(map, key)?
        .map(|value| i32::try_from(value).map_err(|_| format!("{key} is out of range")))
        .transpose()
}
fn u32_param(map: &Map<String, Value>, key: &str) -> Option<u32> {
    int_param_map(map, key)
        .ok()
        .flatten()
        .and_then(|value| u32::try_from(value).ok())
}
fn int_array_param(map: &Map<String, Value>, key: &str) -> Result<Vec<i64>, String> {
    map.get(key)
        .map(integer_values)
        .transpose()
        .map(|value| value.unwrap_or_default())
}
fn integer_value(value: Option<&Value>) -> Result<Option<i64>, String> {
    let Some(value) = value else {
        return Ok(None);
    };
    if let Some(number) = value.as_i64() {
        return Ok(Some(number));
    }
    if let Some(number) = value.as_f64()
        && number.is_finite()
        && number.fract() == 0.0
    {
        return Ok(Some(number as i64));
    }
    value
        .as_str()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(|value| {
            value
                .parse::<i64>()
                .map_err(|_| format!("{value:?} is not an integer"))
        })
        .transpose()
}
fn integer_values(value: &Value) -> Result<Vec<i64>, String> {
    value
        .as_array()
        .ok_or_else(|| "value must be an integer array".to_owned())?
        .iter()
        .map(|value| {
            integer_value(Some(value))
                .map(|value| value.ok_or_else(|| "value contains a non-integer".to_owned()))
                .and_then(|value| value)
        })
        .collect()
}
fn number_value(value: Option<&Value>) -> Result<Option<f64>, String> {
    let Some(value) = value else {
        return Ok(None);
    };
    let number = value
        .as_f64()
        .or_else(|| value.as_str().and_then(|value| value.trim().parse().ok()))
        .ok_or_else(|| "boundary must be numeric".to_owned())?;
    if !number.is_finite() {
        return Err("boundary must be finite".into());
    }
    Ok(Some(number))
}
fn boundary(
    value: &Value,
    key: &str,
) -> Result<Option<crate::trade_proto::qot_stock_screen::Boundary>, String> {
    let Some(object) = value.as_object() else {
        return Ok(None);
    };
    let Some(number) = number_value(object.get(key))? else {
        return Ok(None);
    };
    let includes = object
        .get(&format!("{key}Includes"))
        .and_then(Value::as_bool)
        .unwrap_or(true);
    Ok(Some(crate::trade_proto::qot_stock_screen::Boundary {
        value: number,
        includes,
    }))
}
fn intervals(value: &Value) -> Result<Vec<crate::trade_proto::qot_stock_screen::Interval>, String> {
    let object = value.as_object();
    let values: Vec<Value> = object
        .and_then(|object| object.get("intervals"))
        .and_then(Value::as_array)
        .cloned()
        .filter(|values| !values.is_empty())
        .unwrap_or_else(|| {
            object
                .filter(|object| object.contains_key("min") || object.contains_key("max"))
                .map(|object| vec![Value::Object(object.clone())])
                .unwrap_or_default()
        });
    values
        .iter()
        .map(|value| {
            Ok(crate::trade_proto::qot_stock_screen::Interval {
                filter_min: boundary(value, "min")?,
                filter_max: boundary(value, "max")?,
                unit: value
                    .as_object()
                    .and_then(|value| int_param_map(value, "unit").ok())
                    .flatten()
                    .map(|value| value as i32),
            })
        })
        .collect()
}

fn decode_response(body: &[u8]) -> Result<StockScreenPage, StockScreenQueryError> {
    use crate::trade_proto::qot_stock_screen::Response;
    let response = Response::decode(body)?;
    if response.ret_type != 0 {
        return Err(StockScreenQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD stock-screen request failed".into()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(StockScreenQueryError::MissingS2c);
    };
    if s2c.all_count.is_some_and(|count| count < 0) {
        return Err(StockScreenQueryError::InvalidResponse(
            "stock-screen allCount must be non-negative".into(),
        ));
    }
    let last_page = s2c.last_page.unwrap_or(1) != 0;
    let items = s2c
        .data_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    if s2c
        .all_count
        .is_some_and(|count| count < items.len() as i32)
    {
        return Err(StockScreenQueryError::InvalidResponse(
            "stock-screen allCount is inconsistent with dataList".into(),
        ));
    }
    Ok(StockScreenPage {
        last_page,
        all_count: s2c.all_count,
        items,
    })
}

fn map_item(
    item: crate::trade_proto::qot_stock_screen::StockScreenItem,
) -> Result<StockScreenItem, StockScreenQueryError> {
    let stock_id = item.stock_id.ok_or_else(|| {
        StockScreenQueryError::InvalidResponse("stock-screen item is missing stockId".into())
    })?;
    if stock_id == 0 {
        return Err(StockScreenQueryError::InvalidResponse(
            "stock-screen item stockId must be non-zero".into(),
        ));
    }
    let mut results = Vec::new();
    for result in item.results {
        if let Some(value) = result.basic_property_result
            && let Some(mapped) = map_result(
                "basic",
                value.property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
            )?
        {
            results.push(mapped);
        }
        if let Some(value) = result.simple_property_result
            && let Some(mapped) = map_result(
                "simple",
                value.property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
            )?
        {
            results.push(mapped);
        }
        if let Some(value) = result.cumulative_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "cumulative",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_cumulative),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.financial_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "financial",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                value.end_time,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_financial),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.indicator_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "indicator",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_indicator),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.featured_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "featured",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_featured),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.broker_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "broker",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_broker),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.option_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "option",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_option),
            )? {
                results.push(mapped);
            }
        }
        if let Some(value) = result.kline_shape_property_result {
            let property = value.property.as_ref();
            if let Some(mapped) = map_result_with_params(
                "kline_shape",
                property.and_then(|p| p.name),
                value.value_type,
                value.sval,
                value.ival,
                value.aval,
                value.dval,
                None,
                value.enum_type_name,
                value.enum_name,
                property.map(params_from_kline_shape),
            )? {
                results.push(mapped);
            }
        }
    }
    Ok(StockScreenItem {
        stock_id,
        results,
        security: None,
    })
}

#[allow(clippy::too_many_arguments)]
fn map_result(
    category: &str,
    provider_id: Option<i32>,
    value_type: Option<i32>,
    sval: Option<String>,
    ival: Option<i64>,
    aval: Vec<i64>,
    dval: Option<f64>,
    end_time: Option<u32>,
    enum_type_name: Option<String>,
    enum_name: Option<String>,
) -> Result<Option<StockScreenResult>, StockScreenQueryError> {
    map_result_with_params(
        category,
        provider_id,
        value_type,
        sval,
        ival,
        aval,
        dval,
        end_time,
        enum_type_name,
        enum_name,
        None,
    )
}
#[allow(clippy::too_many_arguments)]
fn map_result_with_params(
    category: &str,
    provider_id: Option<i32>,
    value_type: Option<i32>,
    sval: Option<String>,
    ival: Option<i64>,
    aval: Vec<i64>,
    dval: Option<f64>,
    end_time: Option<u32>,
    enum_type_name: Option<String>,
    enum_name: Option<String>,
    params: Option<StockScreenPropertyParams>,
) -> Result<Option<StockScreenResult>, StockScreenQueryError> {
    let Some(provider_id) = provider_id else {
        return Ok(None);
    };
    let Some((factor_key, _, _)) = FACTORS
        .iter()
        .find(|(_, candidate, id)| *candidate == category && *id == provider_id)
    else {
        return Ok(None);
    };
    let value = match value_type.unwrap_or_default() {
        1 => sval
            .map(|value| StockScreenValue::String { value })
            .unwrap_or(StockScreenValue::Missing),
        2 => ival
            .map(|value| StockScreenValue::Integer { value })
            .unwrap_or(StockScreenValue::Missing),
        3 => StockScreenValue::IntegerArray { values: aval },
        4 => match dval {
            Some(value) if value.is_finite() => StockScreenValue::Number { value },
            Some(_) => {
                return Err(StockScreenQueryError::InvalidResponse(format!(
                    "{category} value must be finite"
                )));
            }
            None => StockScreenValue::Missing,
        },
        _ => StockScreenValue::Missing,
    };
    Ok(Some(StockScreenResult {
        factor_key: (*factor_key).to_owned(),
        property: StockScreenProperty {
            category: category.to_owned(),
            provider_id,
            factor_key: (*factor_key).to_owned(),
            params: params.unwrap_or_default(),
        },
        value,
        enum_type_name: clean(enum_type_name),
        enum_name: clean(enum_name),
        end_time,
    }))
}
fn clean(value: Option<String>) -> Option<String> {
    value.filter(|value| !value.trim().is_empty())
}
fn params_from_cumulative(
    value: &crate::trade_proto::qot_stock_screen::PropertyCumulative,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        days: value.days,
        period_average: value.period_average,
        ..Default::default()
    }
}
fn params_from_financial(
    value: &crate::trade_proto::qot_stock_screen::PropertyFinancial,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        term: value.term,
        duration: value.duration,
        year: value.year,
        period_average: value.period_average,
        future_duration: value.future_duration,
        ..Default::default()
    }
}
fn params_from_indicator(
    value: &crate::trade_proto::qot_stock_screen::PropertyIndicator,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        period: value.period,
        indicator_params: value.indicator_params.clone(),
        ..Default::default()
    }
}
fn params_from_featured(
    value: &crate::trade_proto::qot_stock_screen::PropertyFeatured,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        period: value.period,
        range_period: value.range_period,
        first_custom_param: value.first_custom_param,
        ..Default::default()
    }
}
fn params_from_broker(
    value: &crate::trade_proto::qot_stock_screen::PropertyBroker,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        days: value.days.map(|value| value as u32),
        broker_param: clean(value.param.clone()),
        ..Default::default()
    }
}
fn params_from_option(
    value: &crate::trade_proto::qot_stock_screen::PropertyOption,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        option_hv_period: value.period,
        option_param: value.param.as_ref().map(|value| StockScreenParam {
            value_type: value.r#type,
            string_value: value.sval.clone(),
            integer_value: value.ival,
            integer_values: value.aval.clone(),
        }),
        ..Default::default()
    }
}
fn params_from_kline_shape(
    value: &crate::trade_proto::qot_stock_screen::PropertyKlineShape,
) -> StockScreenPropertyParams {
    StockScreenPropertyParams {
        period: value.period,
        ..Default::default()
    }
}

fn resolve_identities(
    coordinator: &OpenDSessionCoordinator,
    query: &StockScreenQuery,
    page: &mut StockScreenPage,
) -> Result<(), StockScreenQueryError> {
    let mainland = matches!(query.market.as_str(), "SH" | "SZ" | "CN");
    if !mainland {
        for item in &mut page.items {
            item.security = code_from_item(item).map(|code| StockScreenSecurity {
                market: query.market.clone(),
                instrument_id: format!("{}.{}", query.market, code),
                code,
            });
        }
        return Ok(());
    }
    if page.items.is_empty() {
        return Ok(());
    }
    let mut codes = Vec::new();
    for item in &page.items {
        if let Some(code) = code_from_item(item)
            && !codes.contains(&code)
        {
            codes.push(code);
        }
    }
    if codes.is_empty() {
        return Err(StockScreenQueryError::InvalidResponse(
            "mainland stock-screen rows contain no security codes".into(),
        ));
    }
    use crate::trade_proto::{qot_common::Security, qot_get_static_info as wire};
    let request = wire::Request {
        c2s: wire::C2s {
            market: None,
            sec_type: None,
            header: None,
            security_list: codes
                .iter()
                .flat_map(|code| {
                    [21, 22].into_iter().map(|market| Security {
                        market,
                        code: code.clone(),
                    })
                })
                .collect(),
        },
    }
    .encode_to_vec();
    let body = coordinator
        .session()?
        .managed_session()
        .call(wire::PROTOCOL_ID, &request)
        .map_err(OpenDSessionCoordinatorError::from)?;
    let response = wire::Response::decode(body.as_slice())?;
    if response.ret_type != 0 {
        return Err(StockScreenQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD static-info request failed".into()),
        });
    }
    let infos = response
        .s2c
        .ok_or_else(|| {
            StockScreenQueryError::InvalidResponse("OpenD static-info response missing s2c".into())
        })?
        .static_info_list;
    for item in &mut page.items {
        let identity = infos
            .iter()
            .find(|info| info.basic.id > 0 && info.basic.id as u64 == item.stock_id)
            .map(|info| {
                let market = match info.basic.security.market {
                    21 => "SH",
                    22 => "SZ",
                    _ => "",
                };
                (market, info.basic.security.code.trim().to_ascii_uppercase())
            })
            .filter(|(market, code)| !market.is_empty() && !code.is_empty());
        let Some((market, code)) = identity else {
            return Err(StockScreenQueryError::InvalidResponse(format!(
                "could not resolve mainland stock-screen identity {}",
                item.stock_id
            )));
        };
        item.security = Some(StockScreenSecurity {
            market: market.to_owned(),
            code: code.clone(),
            instrument_id: format!("{market}.{code}"),
        });
    }
    Ok(())
}
fn code_from_item(item: &StockScreenItem) -> Option<String> {
    item.results
        .iter()
        .find(|result| result.factor_key == "basic.code")
        .and_then(|result| match &result.value {
            StockScreenValue::String { value } => Some(value.trim().to_ascii_uppercase()),
            StockScreenValue::Integer { value } => Some(value.to_string()),
            _ => None,
        })
        .filter(|value| !value.is_empty())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_stock_screen::{
        PropertyBasic, PropertySimple, Request, Response, ResultPropertyBasic,
        ResultPropertySimple, RspItemResult, S2c, StockScreenItem as WireItem,
    };

    fn query() -> StockScreenQuery {
        StockScreenQuery::new("US", serde_json::json!({"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"value":{"min":10.5,"max":200}}],"columns":[{"factor":{"factorKey":"basic.code"}},{"factor":{"factorKey":"simple.price"}}],"sorts":[{"factor":{"factorKey":"simple.market_cap"},"direction":"desc"}],"pool":{"watchlistStockIds":["7"]}}), 50, 25).expect("query")
    }

    #[test]
    fn encodes_market_filter_properties_retrieve_sort_and_page() {
        let request =
            Request::decode(encode_request(&query()).expect("encode").as_slice()).expect("decode");
        let c2s = request.c2s;
        assert_eq!(c2s.page_from, Some(50));
        assert_eq!(c2s.page_count, Some(25));
        assert_eq!(c2s.watchlist_stock_ids, vec![7]);
        assert_eq!(c2s.filter_list.len(), 2);
        assert_eq!(
            c2s.filter_list[0]
                .simple_field_query
                .as_ref()
                .and_then(|query| query.simple_field),
            Some(1)
        );
        assert_eq!(
            c2s.filter_list[0]
                .simple_field_query
                .as_ref()
                .map(|query| query.screen_value_list.as_slice()),
            Some([2_i64].as_slice())
        );
        assert_eq!(c2s.retrieve_list.len(), 2);
        assert_eq!(
            c2s.sort_list[0]
                .simple_property
                .as_ref()
                .and_then(|property| property.name),
            Some(2301)
        );
    }

    #[test]
    fn encodes_an_implicit_code_retrieve_for_identity_resolution() {
        let query = StockScreenQuery::new(
            "SH",
            serde_json::json!({
                "columns": [{"factor": {"factorKey": "simple.price"}}]
            }),
            0,
            25,
        )
        .expect("query");
        let request =
            Request::decode(encode_request(&query).expect("encode").as_slice()).expect("decode");
        assert_eq!(
            request.c2s.retrieve_list[0]
                .basic_property
                .as_ref()
                .and_then(|p| p.name),
            Some(1101)
        );
        assert_eq!(request.c2s.retrieve_list.len(), 2);
    }

    #[test]
    fn decodes_values_and_rejects_bad_totals() {
        let basic = ResultPropertyBasic {
            property: Some(PropertyBasic { name: Some(1101) }),
            value_type: Some(1),
            sval: Some("AAPL".into()),
            ..Default::default()
        };
        let simple = ResultPropertySimple {
            property: Some(PropertySimple { name: Some(2201) }),
            value_type: Some(4),
            dval: Some(190.25),
            ..Default::default()
        };
        let result = RspItemResult {
            basic_property_result: Some(basic),
            simple_property_result: Some(simple),
            cumulative_property_result: None,
            financial_property_result: None,
            indicator_property_result: None,
            featured_property_result: None,
            broker_property_result: None,
            option_property_result: None,
            kline_shape_property_result: None,
        };
        let item = WireItem {
            stock_id: Some(7),
            results: vec![result],
        };
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                data_list: vec![item],
                last_page: Some(0),
                all_count: Some(2),
            }),
        };
        let page = decode_response(&body.encode_to_vec()).expect("decode");
        assert!(!page.last_page);
        assert_eq!(page.items[0].results[0].factor_key, "basic.code");
        assert!(
            matches!(page.items[0].results[1].value, StockScreenValue::Number { value } if value == 190.25)
        );
    }

    #[test]
    fn limiter_allows_ten_and_rejects_eleventh() {
        let mut limiter = StockScreenLimiter::default();
        for _ in 0..STOCK_SCREEN_LIMIT {
            assert!(limiter.retry_after().is_none());
        }
        assert!(limiter.retry_after().is_some());
    }
}
