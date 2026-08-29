//! Typed OpenD valid option-strategy spread reader
//! (`Qot_GetOptionStrategySpread/3258`).
//!
//! The protocol returns the effective strike spreads for one underlying,
//! strategy type, and expiry selection. Generated protobuf messages remain
//! private to this crate; callers receive a neutral list of spread values.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const SUPPORTED_STRATEGIES: [i32; 8] = [4, 7, 8, 9, 11, 13, 14, 16];
const DIAGONAL_SPREAD: i32 = 16;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionStrategySpreadQuery {
    /// Public quote market: HK (1) or US (11).
    pub market: i32,
    pub code: String,
    pub option_strategy: i32,
    pub expire_time: String,
    pub far_expire_time: Option<String>,
    pub index_option_type: Option<i32>,
}

impl OptionStrategySpreadQuery {
    pub fn validate(&self) -> Result<(), OptionStrategySpreadQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategySpreadItem {
    pub spread: f64,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategySpreadSnapshot {
    pub items: Vec<OptionStrategySpreadItem>,
}

pub trait OptionStrategySpreadReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionStrategySpreadQuery,
    ) -> Result<OptionStrategySpreadSnapshot, OptionStrategySpreadQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionStrategySpreadReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionStrategySpreadReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionStrategySpreadReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionStrategySpreadReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionStrategySpreadReadPort for OpenDOptionStrategySpreadReader {
    fn query(
        &self,
        query: &OptionStrategySpreadQuery,
    ) -> Result<OptionStrategySpreadSnapshot, OptionStrategySpreadQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionStrategySpreadQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_strategy_spread::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &OptionStrategySpreadQuery) -> Result<(), OptionStrategySpreadQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionStrategySpreadQueryError::InvalidQuery(
            "option strategy spread market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(OptionStrategySpreadQueryError::InvalidQuery(format!(
            "option strategy spread code must be a {market} underlying code"
        )));
    }
    if !SUPPORTED_STRATEGIES.contains(&query.option_strategy) {
        return Err(OptionStrategySpreadQueryError::InvalidQuery(
            "optionStrategy must be one of 4, 7, 8, 9, 11, 13, 14, or 16".to_owned(),
        ));
    }
    let expire = parse_date(&query.expire_time).ok_or_else(|| {
        OptionStrategySpreadQueryError::InvalidQuery("expireTime must be YYYY-MM-DD".to_owned())
    })?;
    let far_expire = query
        .far_expire_time
        .as_deref()
        .map(|value| {
            parse_date(value).ok_or_else(|| {
                OptionStrategySpreadQueryError::InvalidQuery(
                    "farExpireTime must be YYYY-MM-DD".to_owned(),
                )
            })
        })
        .transpose()?;
    if query.option_strategy == DIAGONAL_SPREAD && far_expire.is_none() {
        return Err(OptionStrategySpreadQueryError::InvalidQuery(
            "farExpireTime is required for diagonal spread".to_owned(),
        ));
    }
    if let Some(far_expire) = far_expire
        && far_expire < expire
    {
        return Err(OptionStrategySpreadQueryError::InvalidQuery(
            "farExpireTime must not precede expireTime".to_owned(),
        ));
    }
    if let Some(index_option_type) = query.index_option_type
        && !matches!(index_option_type, 0..=2)
    {
        return Err(OptionStrategySpreadQueryError::InvalidQuery(
            "indexOptionType must be 0, 1, or 2".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionStrategySpreadQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_strategy_spread::{C2s, Request};
    Request {
        c2s: C2s {
            owner: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
            option_strategy: query.option_strategy,
            expire_time: Some(query.expire_time.trim().to_owned()),
            far_expire_time: query
                .far_expire_time
                .as_deref()
                .map(|value| value.trim().to_owned()),
            index_option_type: query.index_option_type,
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
) -> Result<OptionStrategySpreadSnapshot, OptionStrategySpreadQueryError> {
    use crate::trade_proto::qot_get_option_strategy_spread::Response;
    let response = Response::decode(body).map_err(OptionStrategySpreadQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionStrategySpreadQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option strategy spread request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionStrategySpreadQueryError::MissingS2c);
    };
    let mut items = Vec::with_capacity(s2c.spread_list.len());
    for spread in s2c.spread_list {
        if !spread.is_finite() || spread <= 0.0 {
            return Err(OptionStrategySpreadQueryError::InvalidResponse(
                "option strategy spread values must be finite and positive".to_owned(),
            ));
        }
        items.push(OptionStrategySpreadItem { spread });
    }
    Ok(OptionStrategySpreadSnapshot { items })
}

fn parse_date(value: &str) -> Option<Date> {
    let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]").ok()?;
    Date::parse(value.trim(), &format).ok()
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionStrategySpreadQueryError {
    #[error("invalid OpenD option strategy spread query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option strategy spread session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionStrategySpread response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionStrategySpread retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionStrategySpread response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option strategy spread response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_strategy_spread::{Response, S2c};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionStrategySpreadQuery {
        OptionStrategySpreadQuery {
            market: 11,
            code: " aapl ".to_owned(),
            option_strategy: 4,
            expire_time: "2026-09-18".to_owned(),
            far_expire_time: None,
            index_option_type: Some(1),
        }
    }

    #[test]
    fn request_uses_owner_strategy_expiry_and_index_type() {
        let request = crate::trade_proto::qot_get_option_strategy_spread::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.owner.market, 11);
        assert_eq!(request.c2s.owner.code, "AAPL");
        assert_eq!(request.c2s.option_strategy, 4);
        assert_eq!(request.c2s.expire_time.as_deref(), Some("2026-09-18"));
        assert_eq!(request.c2s.index_option_type, Some(1));
        assert!(request.c2s.far_expire_time.is_none());
    }

    #[test]
    fn framed_response_preserves_positive_spreads_and_protocol() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                spread_list: vec![10.0, 20.0, 30.0],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_strategy_spread::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        let snapshot = decode_response(&decoded.body).expect("snapshot");
        assert_eq!(decoded.header.proto_id, 3258);
        assert_eq!(snapshot.items[1].spread, 20.0);
    }

    #[test]
    fn rejects_invalid_query_values_and_malformed_response() {
        let mut invalid = query();
        invalid.option_strategy = 6;
        assert!(matches!(
            invalid.validate(),
            Err(OptionStrategySpreadQueryError::InvalidQuery(_))
        ));
        let mut diagonal = query();
        diagonal.option_strategy = DIAGONAL_SPREAD;
        assert!(matches!(
            diagonal.validate(),
            Err(OptionStrategySpreadQueryError::InvalidQuery(_))
        ));
        let missing = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&missing),
            Err(OptionStrategySpreadQueryError::MissingS2c)
        ));
        let invalid_response = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                spread_list: vec![f64::NAN],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid_response),
            Err(OptionStrategySpreadQueryError::InvalidResponse(_))
        ));
    }

    #[test]
    fn diagonal_query_requires_ordered_far_expiry() {
        let mut query = query();
        query.option_strategy = DIAGONAL_SPREAD;
        query.far_expire_time = Some("2026-09-25".to_owned());
        assert!(query.validate().is_ok());
        query.far_expire_time = Some("2026-09-01".to_owned());
        assert!(matches!(
            query.validate(),
            Err(OptionStrategySpreadQueryError::InvalidQuery(_))
        ));
    }
}
