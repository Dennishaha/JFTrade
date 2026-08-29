//! Typed OpenD future-contract catalogue reader (`Qot_GetFutureInfo/3218`).
//!
//! This module keeps generated protobuf messages inside the Futu boundary and
//! exposes a broker-neutral static contract catalogue to the engine. OpenD
//! accepts an optional list of securities; an empty list asks for the complete
//! catalogue.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

/// A Futu market/security pair used to scope a future-info request or result.
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutureInfoSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

/// A static exchange trading interval, represented in minutes after midnight.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutureTradeTime {
    pub begin: Option<f64>,
    pub end: Option<f64>,
}

/// One future contract row returned by OpenD.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutureInfo {
    pub name: String,
    pub security: FutureInfoSecurity,
    pub last_trade_time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_trade_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub owner: Option<FutureInfoSecurity>,
    pub owner_other: String,
    pub exchange: String,
    pub contract_type: String,
    pub contract_size: f64,
    pub contract_size_unit: String,
    pub quote_currency: String,
    pub min_var: f64,
    pub min_var_unit: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub quote_unit: Option<String>,
    pub trade_time: Vec<FutureTradeTime>,
    pub time_zone: String,
    pub exchange_format_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub origin: Option<FutureInfoSecurity>,
}

/// Request scope for `Qot_GetFutureInfo`.
///
/// OpenD treats an empty `securities` list as an unfiltered catalogue query.
/// `market` is retained for the engine projection, which may filter the
/// returned rows because the wire protocol has no standalone market field.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct FutureInfoQuery {
    pub market: Option<i32>,
    pub securities: Vec<FutureInfoSecurityQuery>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FutureInfoSecurityQuery {
    pub market: i32,
    pub code: String,
}

impl FutureInfoQuery {
    pub fn validate(&self) -> Result<(), FutureInfoQueryError> {
        validate_query(self)
    }
}

/// Read-only port for static Futu future-contract metadata.
pub trait FutureInfoReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &FutureInfoQuery) -> Result<Vec<FutureInfo>, FutureInfoQueryError>;
}

/// Adapter over an authenticated managed OpenD session.
#[derive(Clone)]
pub struct OpenDFutureInfoReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDFutureInfoReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDFutureInfoReader")
            .finish_non_exhaustive()
    }
}

impl OpenDFutureInfoReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl FutureInfoReadPort for OpenDFutureInfoReader {
    fn query(&self, query: &FutureInfoQuery) -> Result<Vec<FutureInfo>, FutureInfoQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            FutureInfoQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_future_info::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &FutureInfoQuery) -> Result<(), FutureInfoQueryError> {
    if let Some(market) = query.market {
        validate_market(market)?;
    }
    if query.securities.len() > 200 {
        return Err(FutureInfoQueryError::InvalidQuery(
            "future info security list cannot exceed 200 entries".to_owned(),
        ));
    }
    for security in &query.securities {
        validate_market(security.market)?;
        let code = security.code.trim();
        if code.is_empty() || code.len() > 128 || code.chars().any(|value| {
            value.is_whitespace() || value.is_control() || matches!(value, '.' | '/' | '\\' | '?' | '#')
        }) {
            return Err(FutureInfoQueryError::InvalidQuery(
                "future info security code is invalid".to_owned(),
            ));
        }
    }
    Ok(())
}

fn validate_market(market: i32) -> Result<(), FutureInfoQueryError> {
    if market_label(market).is_none() {
        return Err(FutureInfoQueryError::InvalidQuery(
            "future info market must be HK, US, SH, SZ, SG, JP, AU, MY, CA, FX, or crypto".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &FutureInfoQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_future_info::{C2s, Request};
    Request {
        c2s: C2s {
            security_list: query
                .securities
                .iter()
                .map(|security| crate::trade_proto::qot_common::Security {
                    market: security.market,
                    code: security.code.trim().to_ascii_uppercase(),
                })
                .collect(),
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(body: &[u8]) -> Result<Vec<FutureInfo>, FutureInfoQueryError> {
    use crate::trade_proto::qot_get_future_info::Response;
    let response = Response::decode(body).map_err(FutureInfoQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(FutureInfoQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD future info request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(FutureInfoQueryError::MissingS2c);
    };
    s2c.future_info_list
        .into_iter()
        .map(map_future_info)
        .collect()
}

fn map_future_info(
    item: crate::trade_proto::qot_get_future_info::FutureInfo,
) -> Result<FutureInfo, FutureInfoQueryError> {
    let security = map_security(item.security, "security")?;
    let owner = item
        .owner
        .map(|value| map_security(value, "owner"))
        .transpose()?;
    let origin = item
        .origin
        .map(|value| map_security(value, "origin"))
        .transpose()?;
    let name = required_text(item.name, "name")?;
    let last_trade_time = required_text(item.last_trade_time, "lastTradeTime")?;
    let owner_other = required_text(item.owner_other, "ownerOther")?;
    let exchange = required_text(item.exchange, "exchange")?;
    let contract_type = required_text(item.contract_type, "contractType")?;
    let contract_size_unit = required_text(item.contract_size_unit, "contractSizeUnit")?;
    let quote_currency = required_text(item.quote_currency, "quoteCurrency")?;
    let min_var_unit = required_text(item.min_var_unit, "minVarUnit")?;
    let time_zone = required_text(item.time_zone, "timeZone")?;
    let exchange_format_url = required_text(item.exchange_format_url, "exchangeFormatUrl")?;
    validate_finite_positive(item.contract_size, "contractSize")?;
    validate_finite_positive(item.min_var, "minVar")?;
    validate_optional_finite(item.last_trade_timestamp, "lastTradeTimestamp")?;
    let trade_time = item
        .trade_time
        .into_iter()
        .map(map_trade_time)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(FutureInfo {
        name,
        security,
        last_trade_time,
        last_trade_timestamp: item.last_trade_timestamp,
        owner,
        owner_other,
        exchange,
        contract_type,
        contract_size: item.contract_size,
        contract_size_unit,
        quote_currency,
        min_var: item.min_var,
        min_var_unit,
        quote_unit: optional_text(item.quote_unit),
        trade_time,
        time_zone,
        exchange_format_url,
        origin,
    })
}

fn map_trade_time(
    item: crate::trade_proto::qot_get_future_info::TradeTime,
) -> Result<FutureTradeTime, FutureInfoQueryError> {
    validate_optional_finite(item.begin, "tradeTime.begin")?;
    validate_optional_finite(item.end, "tradeTime.end")?;
    if item
        .begin
        .is_some_and(|begin| !(0.0..=24.0 * 60.0).contains(&begin))
        || item
            .end
            .is_some_and(|end| !(0.0..=24.0 * 60.0).contains(&end))
        || matches!((item.begin, item.end), (Some(begin), Some(end)) if begin > end)
    {
        return Err(FutureInfoQueryError::InvalidResponse(
            "future info tradeTime interval is invalid".to_owned(),
        ));
    }
    Ok(FutureTradeTime {
        begin: item.begin,
        end: item.end,
    })
}

fn map_security(
    value: crate::trade_proto::qot_common::Security,
    field: &'static str,
) -> Result<FutureInfoSecurity, FutureInfoQueryError> {
    let market = market_label(value.market).ok_or_else(|| {
        FutureInfoQueryError::InvalidResponse(format!("future info {field} market is unsupported"))
    })?;
    let code = required_text(value.code, field)?.to_ascii_uppercase();
    Ok(FutureInfoSecurity {
        market: market.to_owned(),
        code: code.clone(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    })
}

fn required_text(value: String, field: &'static str) -> Result<String, FutureInfoQueryError> {
    let value = value.trim();
    if value.is_empty() {
        return Err(FutureInfoQueryError::InvalidResponse(format!(
            "future info {field} is empty"
        )));
    }
    Ok(value.to_owned())
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn validate_finite_positive(value: f64, field: &'static str) -> Result<(), FutureInfoQueryError> {
    if !value.is_finite() || value <= 0.0 {
        return Err(FutureInfoQueryError::InvalidResponse(format!(
            "future info {field} must be finite and positive"
        )));
    }
    Ok(())
}

fn validate_optional_finite(
    value: Option<f64>,
    field: &'static str,
) -> Result<(), FutureInfoQueryError> {
    if value.is_some_and(|value| !value.is_finite()) {
        return Err(FutureInfoQueryError::InvalidResponse(format!(
            "future info {field} must be finite"
        )));
    }
    Ok(())
}

fn market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        61 => Some("MY"),
        71 => Some("CA"),
        81 => Some("FX"),
        91 => Some("CRYPTO"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum FutureInfoQueryError {
    #[error("invalid OpenD future info query: {0}")]
    InvalidQuery(String),
    #[error("OpenD future info session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetFutureInfo response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetFutureInfo retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetFutureInfo response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD future info response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_future_info::{
        FutureInfo as WireFutureInfo, Response, S2c, TradeTime as WireTradeTime,
    };
    use crate::trade_proto::qot_common::Security;

    fn wire_future() -> WireFutureInfo {
        WireFutureInfo {
            name: "E-mini S&P".to_owned(),
            security: Security { market: 11, code: "ESmain".to_owned() },
            last_trade_time: "2026-12-18".to_owned(),
            last_trade_timestamp: Some(1_797_667_200.0),
            owner: None,
            owner_other: "S&P 500".to_owned(),
            exchange: "CME".to_owned(),
            contract_type: "Main".to_owned(),
            contract_size: 50.0,
            contract_size_unit: "USD".to_owned(),
            quote_currency: "USD".to_owned(),
            min_var: 0.25,
            min_var_unit: "index point".to_owned(),
            quote_unit: Some("point".to_owned()),
            trade_time: vec![WireTradeTime { begin: Some(60.0), end: Some(1_380.0) }],
            time_zone: "America/Chicago".to_owned(),
            exchange_format_url: "https://www.cmegroup.com".to_owned(),
            origin: None,
        }
    }

    #[test]
    fn request_encodes_empty_security_list_for_catalogue() {
        let bytes = encode_request(&FutureInfoQuery::default());
        let request = crate::trade_proto::qot_get_future_info::Request::decode(bytes.as_slice())
            .expect("request");
        assert!(request.c2s.security_list.is_empty());
    }

    #[test]
    fn framed_response_decodes_static_contract_fields() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c { future_info_list: vec![wire_future()] }),
        }
        .encode_to_vec();
        let frame = crate::encode_frame(
            crate::trade_proto::qot_get_future_info::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = crate::decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3218);
        let values = decode_response(&decoded.body).expect("future info");
        assert_eq!(values[0].security.instrument_id, "US.ESMAIN");
        assert_eq!(values[0].trade_time[0].end, Some(1_380.0));
    }

    #[test]
    fn rejects_missing_security_non_finite_values_and_bad_return_code() {
        let mut missing = wire_future();
        missing.security = Security::default();
        let response = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c { future_info_list: vec![missing] }),
        }
        .encode_to_vec();
        assert!(matches!(decode_response(&response), Err(FutureInfoQueryError::InvalidResponse(_))));

        let mut invalid = wire_future();
        invalid.contract_size = f64::NAN;
        let response = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c { future_info_list: vec![invalid] }),
        }
        .encode_to_vec();
        assert!(matches!(decode_response(&response), Err(FutureInfoQueryError::InvalidResponse(_))));

        let response = Response {
            ret_type: 3,
            ret_msg: Some("denied".to_owned()),
            err_code: Some(42),
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(decode_response(&response), Err(FutureInfoQueryError::Rejected { ret_type: 3, err_code: 42, .. })));
    }

    #[test]
    fn validates_market_and_security_code() {
        assert!(FutureInfoQuery { market: Some(999), securities: Vec::new() }.validate().is_err());
        assert!(FutureInfoQuery {
            market: Some(11),
            securities: vec![FutureInfoSecurityQuery { market: 11, code: "ES/main".to_owned() }],
        }
        .validate()
        .is_err());
    }
}
