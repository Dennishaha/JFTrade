//! Typed OpenD short-interest readers.
//!
//! `Qot_GetDailyShortVolume` and `Qot_GetShortInterest` have nearly identical
//! request envelopes but return different US/HK row types.  The generated
//! protobuf messages stay private to this crate; this module exposes one
//! provider-neutral row projection and keeps the OpenD error boundary strict.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

/// The two OpenD short-interest operations exposed by the Futu capability.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum ShortInterestOperation {
    /// Daily short-sale volume (`Qot_GetDailyShortVolume/3248`).
    DailyVolume,
    /// Reported short interest (`Qot_GetShortInterest/3249`).
    #[default]
    ShortInterest,
}

/// A validated short-interest query.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ShortInterestQuery {
    pub market: i32,
    pub code: String,
    pub operation: ShortInterestOperation,
    pub next_key: Option<String>,
    pub limit: i32,
}

impl Default for ShortInterestQuery {
    fn default() -> Self {
        Self {
            market: 11,
            code: String::new(),
            operation: ShortInterestOperation::default(),
            next_key: None,
            limit: 50,
        }
    }
}

impl ShortInterestQuery {
    pub fn validate(&self) -> Result<(), ShortInterestQueryError> {
        if !matches!(self.market, 1 | 11) {
            return Err(ShortInterestQueryError::InvalidQuery(
                "short interest market must be HK or US".to_owned(),
            ));
        }
        let code = self.code.trim();
        if code.is_empty()
            || code.len() > 128
            || code.chars().any(|value| {
                value.is_whitespace()
                    || value.is_control()
                    || matches!(value, '.' | '/' | '\\' | '?' | '#')
            })
        {
            return Err(ShortInterestQueryError::InvalidQuery(
                "short interest security code is invalid".to_owned(),
            ));
        }
        if !(1..=50).contains(&self.limit) {
            return Err(ShortInterestQueryError::InvalidQuery(
                "short interest limit must be between 1 and 50".to_owned(),
            ));
        }
        if self
            .next_key
            .as_deref()
            .is_some_and(|value| value.len() > 256 || value.chars().any(char::is_control))
        {
            return Err(ShortInterestQueryError::InvalidQuery(
                "short interest nextKey is invalid".to_owned(),
            ));
        }
        Ok(())
    }
}

/// Identity attached to every projected row/result.
#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ShortInterestSecurity {
    pub market: String,
    pub code: String,
    pub instrument_id: String,
}

/// One US or HK short-interest row.  Fields not supplied by the selected
/// OpenD operation remain `None`; no values are inferred or fabricated.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ShortInterestItem {
    pub security: ShortInterestSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shares_short: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub short_percent: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub avg_daily_share_volume: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub days_to_cover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub short_sell_shares_traded: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub short_sell_turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub close_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_close_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub daily_trade_avg_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub aggregated_short: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub aggregated_short_ratio: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ShortInterestResult {
    pub security: ShortInterestSecurity,
    pub operation: String,
    pub items: Vec<ShortInterestItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_key: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub aggregated_short: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub aggregated_short_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub latest_time: Option<String>,
}

pub trait ShortInterestReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &ShortInterestQuery,
    ) -> Result<ShortInterestResult, ShortInterestQueryError>;
}

#[derive(Clone)]
pub struct OpenDShortInterestReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDShortInterestReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDShortInterestReader")
            .finish_non_exhaustive()
    }
}

impl OpenDShortInterestReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl ShortInterestReadPort for OpenDShortInterestReader {
    fn query(
        &self,
        query: &ShortInterestQuery,
    ) -> Result<ShortInterestResult, ShortInterestQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| ShortInterestQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let body = match query.operation {
            ShortInterestOperation::DailyVolume => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_daily_short_volume::PROTOCOL_ID,
                    &encode_daily_volume(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
            ShortInterestOperation::ShortInterest => session
                .managed_session()
                .call(
                    crate::trade_proto::qot_get_short_interest::PROTOCOL_ID,
                    &encode_short_interest(query),
                )
                .map_err(OpenDSessionCoordinatorError::from)?,
        };
        decode_response(query, &body)
    }
}

fn security(market: i32, code: &str) -> crate::trade_proto::qot_common::Security {
    crate::trade_proto::qot_common::Security {
        market,
        code: code.trim().to_ascii_uppercase(),
    }
}

fn encode_daily_volume(query: &ShortInterestQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_daily_short_volume::{C2s, Request};
    Request {
        c2s: C2s {
            security: security(query.market, &query.code),
            next_key: query.next_key.clone(),
            num: Some(query.limit),
        },
    }
    .encode_to_vec()
}

fn encode_short_interest(query: &ShortInterestQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_short_interest::{C2s, Request};
    Request {
        c2s: C2s {
            security: security(query.market, &query.code),
            next_key: query.next_key.clone(),
            num: Some(query.limit),
        },
    }
    .encode_to_vec()
}

fn decode_response(
    query: &ShortInterestQuery,
    body: &[u8],
) -> Result<ShortInterestResult, ShortInterestQueryError> {
    match query.operation {
        ShortInterestOperation::DailyVolume => {
            use crate::trade_proto::qot_get_daily_short_volume::Response;
            let response = Response::decode(body).map_err(ShortInterestQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                "daily volume",
            )?;
            let s2c = response.s2c.ok_or(ShortInterestQueryError::MissingS2c)?;
            let mut items = Vec::with_capacity(s2c.us_item_list.len() + s2c.hk_item_list.len());
            items.extend(
                s2c.us_item_list
                    .into_iter()
                    .map(|item| map_us_volume(item, query)),
            );
            items.extend(
                s2c.hk_item_list
                    .into_iter()
                    .map(|item| map_hk_volume(item, query)),
            );
            let aggregated_short = non_negative_i64(s2c.aggregated_short, "aggregatedShort")?;
            validate_optional_finite(s2c.aggregated_short_ratio, "aggregatedShortRatio")?;
            Ok(ShortInterestResult {
                security: security_projection(query),
                operation: "daily_volume".to_owned(),
                items: items.into_iter().collect::<Result<Vec<_>, _>>()?,
                next_key: normalize_next_key(s2c.next_key),
                aggregated_short: aggregated_short.map(|value| value as u64),
                aggregated_short_ratio: s2c.aggregated_short_ratio,
                latest_time: optional_text(s2c.new_time_str),
            })
        }
        ShortInterestOperation::ShortInterest => {
            use crate::trade_proto::qot_get_short_interest::Response;
            let response = Response::decode(body).map_err(ShortInterestQueryError::Decode)?;
            ensure_success(
                response.ret_type,
                response.err_code,
                response.ret_msg,
                "short interest",
            )?;
            let s2c = response.s2c.ok_or(ShortInterestQueryError::MissingS2c)?;
            let mut items = Vec::with_capacity(s2c.us_item_list.len() + s2c.hk_item_list.len());
            items.extend(
                s2c.us_item_list
                    .into_iter()
                    .map(|item| map_us_interest(item, query)),
            );
            items.extend(
                s2c.hk_item_list
                    .into_iter()
                    .map(|item| map_hk_interest(item, query)),
            );
            Ok(ShortInterestResult {
                security: security_projection(query),
                operation: "short_interest".to_owned(),
                items: items.into_iter().collect::<Result<Vec<_>, _>>()?,
                next_key: normalize_next_key(s2c.next_key),
                aggregated_short: None,
                aggregated_short_ratio: None,
                latest_time: None,
            })
        }
    }
}

fn map_us_volume(
    value: crate::trade_proto::qot_get_daily_short_volume::UsDailyShortVolumeItem,
    query: &ShortInterestQuery,
) -> Result<ShortInterestItem, ShortInterestQueryError> {
    validate_timestamp(value.timestamp, "timestamp")?;
    validate_finite_fields([
        ("shortPercent", value.short_percent),
        ("closePrice", value.close_price),
        ("lastClosePrice", value.last_close_price),
        ("dailyTradeAvgRatio", value.daily_trade_avg_ratio),
    ])?;
    Ok(ShortInterestItem {
        security: security_projection(query),
        timestamp: value.timestamp,
        timestamp_str: optional_text(value.timestamp_str),
        shares_short: value.total_shares_short,
        short_percent: value.short_percent,
        avg_daily_share_volume: None,
        days_to_cover: None,
        volume: value.volume,
        turnover: None,
        short_sell_shares_traded: None,
        short_sell_turnover: None,
        open_price: None,
        close_price: value.close_price,
        last_close_price: value.last_close_price,
        daily_trade_avg_ratio: value.daily_trade_avg_ratio,
        aggregated_short: None,
        aggregated_short_ratio: None,
    })
}

fn map_hk_volume(
    value: crate::trade_proto::qot_get_daily_short_volume::HkDailyShortVolumeItem,
    query: &ShortInterestQuery,
) -> Result<ShortInterestItem, ShortInterestQueryError> {
    validate_timestamp(value.timestamp, "timestamp")?;
    validate_finite_fields([
        ("turnover", value.turnover),
        ("shortSellTurnover", value.short_sell_turnover),
        ("openPrice", value.open_price),
        ("closePrice", value.close_price),
        ("lastClosePrice", value.last_close_price),
        ("dailyTradeAvgRatio", value.daily_trade_avg_ratio),
    ])?;
    Ok(ShortInterestItem {
        security: security_projection(query),
        timestamp: value.timestamp,
        timestamp_str: optional_text(value.timestamp_str),
        shares_short: None,
        short_percent: None,
        avg_daily_share_volume: None,
        days_to_cover: None,
        volume: value.shares_traded,
        turnover: value.turnover,
        short_sell_shares_traded: value.short_sell_shares_traded,
        short_sell_turnover: value.short_sell_turnover,
        open_price: value.open_price,
        close_price: value.close_price,
        last_close_price: value.last_close_price,
        daily_trade_avg_ratio: value.daily_trade_avg_ratio,
        aggregated_short: None,
        aggregated_short_ratio: None,
    })
}

fn map_us_interest(
    value: crate::trade_proto::qot_get_short_interest::UsShortInterestItem,
    query: &ShortInterestQuery,
) -> Result<ShortInterestItem, ShortInterestQueryError> {
    validate_timestamp(value.timestamp, "timestamp")?;
    validate_finite_fields([
        ("shortPercent", value.short_percent),
        ("daysToCover", value.days_to_cover),
        ("closePrice", value.close_price),
        ("lastClosePrice", value.last_close_price),
    ])?;
    Ok(ShortInterestItem {
        security: security_projection(query),
        timestamp: value.timestamp,
        timestamp_str: optional_text(value.timestamp_str),
        shares_short: value.shares_short,
        short_percent: value.short_percent,
        avg_daily_share_volume: value.avg_daily_share_volume,
        days_to_cover: value.days_to_cover,
        volume: None,
        turnover: None,
        short_sell_shares_traded: None,
        short_sell_turnover: None,
        open_price: None,
        close_price: value.close_price,
        last_close_price: value.last_close_price,
        daily_trade_avg_ratio: None,
        aggregated_short: None,
        aggregated_short_ratio: None,
    })
}

fn map_hk_interest(
    value: crate::trade_proto::qot_get_short_interest::HkShortInterestItem,
    query: &ShortInterestQuery,
) -> Result<ShortInterestItem, ShortInterestQueryError> {
    validate_timestamp(value.timestamp, "timestamp")?;
    validate_finite_fields([
        ("closePrice", value.close_price),
        ("lastClosePrice", value.last_close_price),
        ("aggregatedShortRatio", value.aggregated_short_ratio),
    ])?;
    Ok(ShortInterestItem {
        security: security_projection(query),
        timestamp: value.timestamp,
        timestamp_str: optional_text(value.timestamp_str),
        shares_short: None,
        short_percent: None,
        avg_daily_share_volume: None,
        days_to_cover: None,
        volume: None,
        turnover: None,
        short_sell_shares_traded: None,
        short_sell_turnover: None,
        open_price: None,
        close_price: value.close_price,
        last_close_price: value.last_close_price,
        daily_trade_avg_ratio: None,
        aggregated_short: value.aggregated_short,
        aggregated_short_ratio: value.aggregated_short_ratio,
    })
}

fn security_projection(query: &ShortInterestQuery) -> ShortInterestSecurity {
    let market = market_label(query.market).expect("query validation ensures market");
    let code = query.code.trim().to_ascii_uppercase();
    ShortInterestSecurity {
        market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
        code,
    }
}

fn market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

fn ensure_success(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<String>,
    operation: &'static str,
) -> Result<(), ShortInterestQueryError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(ShortInterestQueryError::Rejected {
        operation,
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or_else(|| format!("OpenD {operation} request failed")),
    })
}

fn normalize_next_key(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty() && value != "-1")
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn validate_timestamp(value: Option<i64>, field: &str) -> Result<(), ShortInterestQueryError> {
    if value.is_some_and(|value| value < 0) {
        return Err(ShortInterestQueryError::InvalidResponse(format!(
            "OpenD short interest {field} must be non-negative"
        )));
    }
    Ok(())
}

fn non_negative_i64(
    value: Option<i64>,
    field: &str,
) -> Result<Option<i64>, ShortInterestQueryError> {
    if value.is_some_and(|value| value < 0) {
        return Err(ShortInterestQueryError::InvalidResponse(format!(
            "OpenD short interest {field} must be non-negative"
        )));
    }
    Ok(value)
}

fn validate_finite_fields<const N: usize>(
    values: [(&str, Option<f64>); N],
) -> Result<(), ShortInterestQueryError> {
    for (field, value) in values {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(ShortInterestQueryError::InvalidResponse(format!(
                "OpenD short interest {field} must be finite"
            )));
        }
    }
    Ok(())
}

fn validate_optional_finite(
    value: Option<f64>,
    field: &str,
) -> Result<(), ShortInterestQueryError> {
    validate_finite_fields([(field, value)])
}

#[derive(Debug, Error)]
pub enum ShortInterestQueryError {
    #[error("invalid OpenD short interest query: {0}")]
    InvalidQuery(String),
    #[error("OpenD short interest session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD short interest response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD {operation} retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD short interest response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD short interest response: {0}")]
    InvalidResponse(String),
}

// Compatibility aliases make the capability name explicit to callers while
// retaining the concise names used by the other typed readers.
pub type FutuShortInterestQuery = ShortInterestQuery;
pub type FutuShortInterestResult = ShortInterestResult;
pub type FutuShortInterestQueryError = ShortInterestQueryError;
pub type FutuShortInterestItem = ShortInterestItem;
pub type FutuShortInterestSecurity = ShortInterestSecurity;
pub use ShortInterestReadPort as FutuShortInterestReadPort;
