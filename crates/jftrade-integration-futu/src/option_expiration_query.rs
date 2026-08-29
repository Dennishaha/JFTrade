//! Typed OpenD option-chain expiration-date reader (Qot_GetOptionExpirationDate/3224).
//!
//! The adapter owns only the wire request/response mapping.  It does not
//! invent dates when OpenD is unavailable or returns an incomplete payload.

use std::sync::{Arc, Mutex};

use prost::Message;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

/// OpenD market/security identity for the option-chain owner.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OptionExpirationQuery {
    pub market: i32,
    pub symbol: String,
    pub index_option_type: Option<i32>,
}

/// One expiration returned by OpenD.
#[derive(Clone, Debug, PartialEq)]
pub struct OptionExpirationDate {
    pub strike_time: Option<String>,
    pub strike_timestamp: Option<f64>,
    pub expiry_date_distance: i32,
    pub cycle: Option<i32>,
}

/// Consumer-owned port for broker-neutral option expiration dates.
pub trait OptionExpirationReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionExpirationQuery,
    ) -> Result<Vec<OptionExpirationDate>, OptionExpirationQueryError>;
}

/// Read-only adapter over the coordinator's authenticated managed OpenD
/// session.  The coordinator remains the owner of connection/reconnect state.
#[derive(Clone)]
pub struct OpenDOptionExpirationReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionExpirationReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionExpirationReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionExpirationReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionExpirationReadPort for OpenDOptionExpirationReader {
    fn query(
        &self,
        query: &OptionExpirationQuery,
    ) -> Result<Vec<OptionExpirationDate>, OptionExpirationQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionExpirationQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let body = encode_request(query);
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_expiration_date::PROTOCOL_ID,
                &body,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &OptionExpirationQuery) -> Result<(), OptionExpirationQueryError> {
    if !matches!(query.market, 1 | 11) {
        return Err(OptionExpirationQueryError::InvalidQuery(
            "option expiration market must be HK (1) or US (11)".to_owned(),
        ));
    }
    if query.symbol.trim().is_empty()
        || query.symbol.contains('.')
        || query.symbol.trim().chars().any(char::is_whitespace)
    {
        return Err(OptionExpirationQueryError::InvalidQuery(
            "option expiration symbol must be a non-empty code".to_owned(),
        ));
    }
    if let Some(index_option_type) = query.index_option_type
        && !matches!(index_option_type, 0..=2)
    {
        return Err(OptionExpirationQueryError::InvalidQuery(
            "index option type must be 0, 1, or 2".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionExpirationQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_expiration_date::{C2s, Request};
    Request {
        c2s: C2s {
            owner: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.symbol.trim().to_ascii_uppercase(),
            },
            index_option_type: query.index_option_type,
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(body: &[u8]) -> Result<Vec<OptionExpirationDate>, OptionExpirationQueryError> {
    use crate::trade_proto::qot_get_option_expiration_date::Response;
    let response = Response::decode(body).map_err(OptionExpirationQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionExpirationQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option expiration request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionExpirationQueryError::MissingS2c);
    };
    let mut dates = Vec::with_capacity(s2c.date_list.len());
    for date in s2c.date_list {
        if date.option_expiry_date_distance < -3660 {
            return Err(OptionExpirationQueryError::InvalidResponse(
                "option expiration distance is outside the supported range".to_owned(),
            ));
        }
        if let Some(timestamp) = date.strike_timestamp
            && !timestamp.is_finite()
        {
            return Err(OptionExpirationQueryError::InvalidResponse(
                "option expiration timestamp must be finite".to_owned(),
            ));
        }
        dates.push(OptionExpirationDate {
            strike_time: date.strike_time.filter(|value| !value.trim().is_empty()),
            strike_timestamp: date.strike_timestamp,
            expiry_date_distance: date.option_expiry_date_distance,
            cycle: date.cycle,
        });
    }
    Ok(dates)
}

#[derive(Debug, Error)]
pub enum OptionExpirationQueryError {
    #[error("invalid OpenD option expiration query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option expiration session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionExpirationDate response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionExpirationDate retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionExpirationDate response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option expiration response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_expiration_date::{
        OptionExpirationDate as WireDate, Response, S2c,
    };
    use crate::{decode_frame, encode_frame};

    #[test]
    fn request_uses_owner_and_index_option_type_wire_fields() {
        let query = OptionExpirationQuery {
            market: 11,
            symbol: " aapl ".to_owned(),
            index_option_type: Some(2),
        };
        let request = crate::trade_proto::qot_get_option_expiration_date::Request::decode(
            encode_request(&query).as_slice(),
        )
        .expect("request");
        let c2s = request.c2s;
        let owner = c2s.owner;
        assert_eq!(owner.market, 11);
        assert_eq!(owner.code, "AAPL");
        assert_eq!(c2s.index_option_type, Some(2));
    }

    #[test]
    fn framed_response_decodes_dates_and_preserves_protocol_identity() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                date_list: vec![WireDate {
                    strike_time: Some("2026-09-18".to_owned()),
                    strike_timestamp: Some(1_789_000_000.0),
                    option_expiry_date_distance: 21,
                    cycle: Some(1),
                }],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_expiration_date::PROTOCOL_ID,
            7,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3224);
        assert_eq!(decoded.header.serial_no, 7);
        let dates = decode_response(&decoded.body).expect("dates");
        assert_eq!(dates[0].strike_time.as_deref(), Some("2026-09-18"));
        assert_eq!(dates[0].expiry_date_distance, 21);
    }

    #[test]
    fn rejects_missing_s2c_and_non_finite_timestamps() {
        let missing = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&missing),
            Err(OptionExpirationQueryError::MissingS2c)
        ));

        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                date_list: vec![WireDate {
                    strike_time: None,
                    strike_timestamp: Some(f64::NAN),
                    option_expiry_date_distance: 0,
                    cycle: None,
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid),
            Err(OptionExpirationQueryError::InvalidResponse(_))
        ));
    }
}
