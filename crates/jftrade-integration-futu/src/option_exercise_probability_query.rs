//! Typed OpenD option-exercise probability reader
//! (`Qot_GetOptionExerciseProbability/3251`).
//!
//! OpenD accepts one concrete option contract and returns its historical
//! exercise-probability series. Generated protobuf types stay inside this
//! crate; callers receive broker-neutral security and item DTOs.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::{Date, Month};

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionExerciseProbabilityQuery {
    pub market: i32,
    pub code: String,
}

impl OptionExerciseProbabilityQuery {
    pub fn validate(&self) -> Result<(), OptionExerciseProbabilityQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionExerciseProbabilitySecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionExerciseProbabilityItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp_str: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub security_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_probability: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionExerciseProbabilitySnapshot {
    pub security: OptionExerciseProbabilitySecurity,
    pub items: Vec<OptionExerciseProbabilityItem>,
}

pub trait OptionExerciseProbabilityReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionExerciseProbabilityQuery,
    ) -> Result<OptionExerciseProbabilitySnapshot, OptionExerciseProbabilityQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionExerciseProbabilityReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionExerciseProbabilityReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionExerciseProbabilityReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionExerciseProbabilityReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionExerciseProbabilityReadPort for OpenDOptionExerciseProbabilityReader {
    fn query(
        &self,
        query: &OptionExerciseProbabilityQuery,
    ) -> Result<OptionExerciseProbabilitySnapshot, OptionExerciseProbabilityQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionExerciseProbabilityQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_exercise_probability::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(
    query: &OptionExerciseProbabilityQuery,
) -> Result<(), OptionExerciseProbabilityQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionExerciseProbabilityQueryError::InvalidQuery(
            "option exercise probability market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if !is_option_contract_code(code) {
        return Err(OptionExerciseProbabilityQueryError::InvalidQuery(format!(
            "option exercise probability code must be a concrete {market} option contract"
        )));
    }
    Ok(())
}

fn is_option_contract_code(code: &str) -> bool {
    let code = code.trim().to_ascii_uppercase();
    if code.is_empty()
        || code
            .chars()
            .any(|value| !value.is_ascii_alphanumeric() && value != '-')
    {
        return false;
    }
    let bytes = code.as_bytes();
    if bytes.len() < 8 || !bytes.first().is_some_and(u8::is_ascii_alphabetic) {
        return false;
    }
    for index in 0..=bytes.len().saturating_sub(7) {
        if !bytes[index..index + 6].iter().all(u8::is_ascii_digit)
            || !matches!(bytes[index + 6], b'C' | b'P')
            || !bytes[index + 7..].iter().all(u8::is_ascii_digit)
        {
            continue;
        }
        let year = code[index..index + 2].parse::<i32>().ok();
        let month = code[index + 2..index + 4].parse::<u8>().ok();
        let day = code[index + 4..index + 6].parse::<u8>().ok();
        if let (Some(year), Some(month), Some(day)) = (year, month, day)
            && let Ok(month) = Month::try_from(month)
            && Date::from_calendar_date(2000 + year, month, day).is_ok()
        {
            return true;
        }
    }
    false
}

fn encode_request(query: &OptionExerciseProbabilityQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_exercise_probability::{C2s, Request};
    Request {
        c2s: C2s {
            security: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionExerciseProbabilityQuery,
) -> Result<OptionExerciseProbabilitySnapshot, OptionExerciseProbabilityQueryError> {
    use crate::trade_proto::qot_get_option_exercise_probability::Response;
    let response = Response::decode(body).map_err(OptionExerciseProbabilityQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionExerciseProbabilityQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option exercise probability request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionExerciseProbabilityQueryError::MissingS2c);
    };
    let items = s2c
        .item_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    let market = market_label(query.market).expect("query validation ensures market");
    let code = query.code.trim().to_ascii_uppercase();
    Ok(OptionExerciseProbabilitySnapshot {
        security: OptionExerciseProbabilitySecurity {
            market: market.to_owned(),
            code: code.clone(),
            quote_market: market.to_owned(),
            trade_market: market.to_owned(),
            instrument_id: format!("{market}.{code}"),
        },
        items,
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_exercise_probability::StrikeProbabilityItem,
) -> Result<OptionExerciseProbabilityItem, OptionExerciseProbabilityQueryError> {
    let timestamp_str = item
        .timestamp_str
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    if let Some(value) = timestamp_str.as_deref()
        && !is_date(value)
    {
        return Err(OptionExerciseProbabilityQueryError::InvalidResponse(
            "option exercise probability timestampStr must be YYYY-MM-DD".to_owned(),
        ));
    }
    for (name, value) in [
        ("securityPrice", item.security_price),
        ("strikeProbability", item.strike_probability),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionExerciseProbabilityQueryError::InvalidResponse(
                format!("option exercise probability {name} must be finite"),
            ));
        }
    }
    if let Some(value) = item.strike_probability
        && !(0.0..=100.0).contains(&value)
    {
        return Err(OptionExerciseProbabilityQueryError::InvalidResponse(
            "option exercise probability strikeProbability must be between 0 and 100".to_owned(),
        ));
    }
    Ok(OptionExerciseProbabilityItem {
        timestamp: item.timestamp,
        timestamp_str,
        security_price: item.security_price,
        strike_probability: item.strike_probability,
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
pub enum OptionExerciseProbabilityQueryError {
    #[error("invalid OpenD option exercise probability query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option exercise probability session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionExerciseProbability response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error(
        "OpenD Qot_GetOptionExerciseProbability retType={ret_type} errCode={err_code}: {message}"
    )]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionExerciseProbability response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option exercise probability response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_exercise_probability::{
        Response, S2c, StrikeProbabilityItem,
    };
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionExerciseProbabilityQuery {
        OptionExerciseProbabilityQuery {
            market: 11,
            code: " AAPL260918C00100000 ".to_owned(),
        }
    }

    #[test]
    fn request_uses_concrete_option_contract() {
        let request = crate::trade_proto::qot_get_option_exercise_probability::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.security.market, 11);
        assert_eq!(request.c2s.security.code, "AAPL260918C00100000");
    }

    #[test]
    fn framed_response_preserves_probability_metrics() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![StrikeProbabilityItem {
                    timestamp: Some(1_756_000_000),
                    timestamp_str: Some("2026-08-29".to_owned()),
                    security_price: Some(225.0),
                    strike_probability: Some(41.869),
                }],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_exercise_probability::PROTOCOL_ID,
            4,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3251);
        assert_eq!(decoded.header.serial_no, 4);
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(snapshot.security.instrument_id, "US.AAPL260918C00100000");
        assert_eq!(snapshot.items[0].security_price, Some(225.0));
        assert_eq!(snapshot.items[0].strike_probability, Some(41.869));
    }

    #[test]
    fn rejects_invalid_contract_date_probability_and_missing_s2c() {
        assert!(matches!(
            validate_query(&OptionExerciseProbabilityQuery {
                market: 11,
                code: "AAPL".to_owned(),
            }),
            Err(OptionExerciseProbabilityQueryError::InvalidQuery(_))
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
            Err(OptionExerciseProbabilityQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![StrikeProbabilityItem {
                    timestamp_str: Some("2026-02-30".to_owned()),
                    security_price: Some(f64::NAN),
                    strike_probability: Some(101.0),
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionExerciseProbabilityQueryError::InvalidResponse(_))
        ));
    }
}
