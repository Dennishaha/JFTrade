//! Typed OpenD option-volatility reader (Qot_GetOptionVolatility/3250).
//!
//! OpenD accepts the underlying stock (not an option contract) and returns a
//! time series of implied/history volatility.  The generated protobuf stays
//! inside this crate; callers receive broker-neutral security, item, and
//! summary DTOs.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionVolatilityQuery {
    pub market: i32,
    pub code: String,
    pub query_time_period: Option<i32>,
    pub hv_time_period: Option<i32>,
}

impl OptionVolatilityQuery {
    pub fn validate(&self) -> Result<(), OptionVolatilityQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionVolatilitySecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionVolatilityItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub implied_volatility: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub history_volatility: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volatility_premium: Option<f64>,
}

/// Complete volatility response, including summary values returned by OpenD.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionVolatilitySnapshot {
    pub security: OptionVolatilitySecurity,
    pub items: Vec<OptionVolatilityItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub average_impvol: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub impvol_status: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub analysis: Option<String>,
}

pub trait OptionVolatilityReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionVolatilityQuery,
    ) -> Result<OptionVolatilitySnapshot, OptionVolatilityQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionVolatilityReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionVolatilityReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionVolatilityReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionVolatilityReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionVolatilityReadPort for OpenDOptionVolatilityReader {
    fn query(
        &self,
        query: &OptionVolatilityQuery,
    ) -> Result<OptionVolatilitySnapshot, OptionVolatilityQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionVolatilityQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_volatility::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &OptionVolatilityQuery) -> Result<(), OptionVolatilityQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionVolatilityQueryError::InvalidQuery(
            "option volatility market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(OptionVolatilityQueryError::InvalidQuery(format!(
            "option volatility code must be a {market} underlying code"
        )));
    }
    if let Some(period) = query.query_time_period
        && !(1..=5).contains(&period)
    {
        return Err(OptionVolatilityQueryError::InvalidQuery(
            "option volatility queryTimePeriod must be 1 (week) through 5 (year)".to_owned(),
        ));
    }
    if let Some(period) = query.hv_time_period
        && !(5..=250).contains(&period)
    {
        return Err(OptionVolatilityQueryError::InvalidQuery(
            "option volatility hvTimePeriod must be between 5 and 250 days".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionVolatilityQuery) -> Result<Vec<u8>, OptionVolatilityQueryError> {
    use crate::trade_proto::qot_get_option_volatility::{C2s, Request};
    let query_time_period = query
        .query_time_period
        .map(|value| {
            crate::trade_proto::qot_common::OptionVolatilityTimePeriodType::try_from(value)
                .map_err(|_| {
                    OptionVolatilityQueryError::InvalidQuery(
                        "option volatility queryTimePeriod is unsupported".to_owned(),
                    )
                })
                .map(i32::from)
        })
        .transpose()?;
    Ok(Request {
        c2s: C2s {
            security: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
            query_time_period,
            hv_time_period: query.hv_time_period,
        },
    }
    .encode_to_vec())
}

fn decode_response(
    body: &[u8],
    query: &OptionVolatilityQuery,
) -> Result<OptionVolatilitySnapshot, OptionVolatilityQueryError> {
    use crate::trade_proto::qot_get_option_volatility::Response;
    let response = Response::decode(body).map_err(OptionVolatilityQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionVolatilityQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option volatility request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionVolatilityQueryError::MissingS2c);
    };
    let mut items = Vec::with_capacity(s2c.item_list.len());
    for item in s2c.item_list {
        items.push(map_item(item)?);
    }
    if let Some(value) = s2c.average_impvol
        && !value.is_finite()
    {
        return Err(OptionVolatilityQueryError::InvalidResponse(
            "option volatility averageImpvol must be finite".to_owned(),
        ));
    }
    let impvol_status = s2c
        .impvol_status
        .map(|status| match status {
            0 => Ok("ImpvolFluctuating".to_owned()),
            1 => Ok("ImpvolOvervalued".to_owned()),
            2 => Ok("ImpvolUndervalued".to_owned()),
            _ => Err(OptionVolatilityQueryError::InvalidResponse(
                "option volatility impvolStatus is unsupported".to_owned(),
            )),
        })
        .transpose()?;
    let market = market_label(query.market).expect("query validation ensures market");
    let code = query.code.trim().to_ascii_uppercase();
    Ok(OptionVolatilitySnapshot {
        security: OptionVolatilitySecurity {
            market: market.to_owned(),
            code: code.clone(),
            quote_market: market.to_owned(),
            trade_market: market.to_owned(),
            instrument_id: format!("{market}.{code}"),
        },
        items,
        average_impvol: s2c.average_impvol,
        impvol_status,
        analysis: s2c
            .analysis
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty()),
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_volatility::VolatilityItem,
) -> Result<OptionVolatilityItem, OptionVolatilityQueryError> {
    let date = item
        .timestamp_str
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    if let Some(value) = date.as_deref()
        && !is_date(value)
    {
        return Err(OptionVolatilityQueryError::InvalidResponse(
            "option volatility timestampStr must be YYYY-MM-DD".to_owned(),
        ));
    }
    for (name, value) in [
        ("impliedVolatility", item.implied_volatility),
        ("historyVolatility", item.history_volatility),
        ("volatilityPremium", item.volatility_premium),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionVolatilityQueryError::InvalidResponse(format!(
                "option volatility {name} must be finite"
            )));
        }
    }
    Ok(OptionVolatilityItem {
        timestamp: item.timestamp,
        timestamp_str: date,
        implied_volatility: item.implied_volatility,
        history_volatility: item.history_volatility,
        volatility_premium: item.volatility_premium,
    })
}

fn is_date(value: &str) -> bool {
    let Ok(format) = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]") else {
        return false;
    };
    Date::parse(value, &format).is_ok()
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionVolatilityQueryError {
    #[error("invalid OpenD option volatility query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option volatility session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionVolatility response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionVolatility retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionVolatility response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option volatility response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_volatility::{Response, S2c, VolatilityItem};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionVolatilityQuery {
        OptionVolatilityQuery {
            market: 11,
            code: " aapl ".to_owned(),
            query_time_period: Some(2),
            hv_time_period: Some(30),
        }
    }

    #[test]
    fn request_uses_underlying_and_bounded_periods() {
        let request = crate::trade_proto::qot_get_option_volatility::Request::decode(
            encode_request(&query()).expect("request").as_slice(),
        )
        .expect("decode request");
        let c2s = request.c2s;
        assert_eq!(c2s.security.code, "AAPL");
        assert_eq!(c2s.query_time_period, Some(2));
        assert_eq!(c2s.hv_time_period, Some(30));
    }

    #[test]
    fn framed_response_preserves_metrics_and_summary() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![VolatilityItem {
                    timestamp: Some(1_756_000_000),
                    timestamp_str: Some("2026-08-29".to_owned()),
                    implied_volatility: Some(25.0),
                    history_volatility: Some(20.0),
                    volatility_premium: Some(5.0),
                }],
                average_impvol: Some(25.0),
                impvol_status: Some(
                    crate::trade_proto::qot_common::OptionImpvolStatusType::ImpvolOvervalued.into(),
                ),
                analysis: Some("elevated".to_owned()),
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_volatility::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3250);
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(snapshot.security.instrument_id, "US.AAPL");
        assert_eq!(snapshot.items[0].implied_volatility, Some(25.0));
        assert_eq!(snapshot.average_impvol, Some(25.0));
        assert_eq!(snapshot.impvol_status.as_deref(), Some("ImpvolOvervalued"));
    }

    #[test]
    fn rejects_invalid_period_date_non_finite_and_missing_s2c() {
        assert!(matches!(
            validate_query(&OptionVolatilityQuery {
                market: 11,
                code: "AAPL".to_owned(),
                query_time_period: Some(0),
                hv_time_period: None,
            }),
            Err(OptionVolatilityQueryError::InvalidQuery(_))
        ));
        let missing = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&missing, &query()),
            Err(OptionVolatilityQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![VolatilityItem {
                    timestamp_str: Some("2026-02-30".to_owned()),
                    implied_volatility: Some(f64::NAN),
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionVolatilityQueryError::InvalidResponse(_))
        ));
    }
}
