//! Typed OpenD option-underlying overview reader
//! (`Qot_GetOptionUnderlyingOverview/3303`).
//!
//! OpenD returns the latest option statistics for one or more underlying
//! securities. This bounded adapter queries one underlying and maps generated
//! protobuf messages into broker-neutral DTOs.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionUnderlyingOverviewQuery {
    pub market: i32,
    pub code: String,
    pub index_option_type: Option<i32>,
}

impl OptionUnderlyingOverviewQuery {
    pub fn validate(&self) -> Result<(), OptionUnderlyingOverviewQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingOverviewSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHvItem {
    pub time_range: i32,
    pub hv: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv_percentile: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingOverviewItem {
    pub security: OptionUnderlyingOverviewSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub call_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub put_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub call_open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub put_open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_rank: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_percentile: Option<f64>,
    pub hv_list: Vec<OptionUnderlyingHvItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pre_iv: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingOverviewSnapshot {
    pub items: Vec<OptionUnderlyingOverviewItem>,
}

pub trait OptionUnderlyingOverviewReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionUnderlyingOverviewQuery,
    ) -> Result<OptionUnderlyingOverviewSnapshot, OptionUnderlyingOverviewQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionUnderlyingOverviewReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionUnderlyingOverviewReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionUnderlyingOverviewReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionUnderlyingOverviewReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionUnderlyingOverviewReadPort for OpenDOptionUnderlyingOverviewReader {
    fn query(
        &self,
        query: &OptionUnderlyingOverviewQuery,
    ) -> Result<OptionUnderlyingOverviewSnapshot, OptionUnderlyingOverviewQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionUnderlyingOverviewQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_underlying_overview::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(
    query: &OptionUnderlyingOverviewQuery,
) -> Result<(), OptionUnderlyingOverviewQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionUnderlyingOverviewQueryError::InvalidQuery(
            "option underlying overview market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(OptionUnderlyingOverviewQueryError::InvalidQuery(format!(
            "option underlying overview code must be a {market} underlying code"
        )));
    }
    if let Some(index_option_type) = query.index_option_type
        && !matches!(index_option_type, 0..=2)
    {
        return Err(OptionUnderlyingOverviewQueryError::InvalidQuery(
            "option underlying overview indexOptionType must be 0, 1, or 2".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionUnderlyingOverviewQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_underlying_overview::{C2s, Request};
    Request {
        c2s: C2s {
            owner_list: vec![crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            }],
            index_option_type: query.index_option_type,
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionUnderlyingOverviewQuery,
) -> Result<OptionUnderlyingOverviewSnapshot, OptionUnderlyingOverviewQueryError> {
    use crate::trade_proto::qot_get_option_underlying_overview::Response;
    let response = Response::decode(body).map_err(OptionUnderlyingOverviewQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionUnderlyingOverviewQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option underlying overview request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionUnderlyingOverviewQueryError::MissingS2c);
    };
    let items = s2c
        .underlying_data_list
        .into_iter()
        .map(|item| map_item(item, query))
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionUnderlyingOverviewSnapshot { items })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_underlying_overview::UnderlyingData,
    query: &OptionUnderlyingOverviewQuery,
) -> Result<OptionUnderlyingOverviewItem, OptionUnderlyingOverviewQueryError> {
    let owner = item.owner;
    if owner.market != query.market || owner.code.trim().is_empty() {
        return Err(OptionUnderlyingOverviewQueryError::InvalidResponse(
            "option underlying overview owner does not match query".to_owned(),
        ));
    }
    let security = security_from_wire(owner.market, owner.code.trim());
    let code = optional_text(item.code);
    let name = optional_text(item.name);
    let hv_list = item
        .hv_list
        .into_iter()
        .map(map_hv_item)
        .collect::<Result<Vec<_>, _>>()?;
    for (field, value) in [
        ("iv", item.iv),
        ("ivRank", item.iv_rank),
        ("ivPercentile", item.iv_percentile),
        ("preIV", item.pre_iv),
    ] {
        validate_finite(field, value)?;
    }
    Ok(OptionUnderlyingOverviewItem {
        security,
        code,
        name,
        call_volume: item.call_volume,
        put_volume: item.put_volume,
        call_open_interest: item.call_open_interest,
        put_open_interest: item.put_open_interest,
        iv: item.iv,
        iv_rank: item.iv_rank,
        iv_percentile: item.iv_percentile,
        hv_list,
        pre_iv: item.pre_iv,
    })
}

fn map_hv_item(
    item: crate::trade_proto::qot_get_option_underlying_overview::HvItem,
) -> Result<OptionUnderlyingHvItem, OptionUnderlyingOverviewQueryError> {
    if !(1..=5).contains(&item.time_range) {
        return Err(OptionUnderlyingOverviewQueryError::InvalidResponse(
            "option underlying overview HV timeRange is unsupported".to_owned(),
        ));
    }
    validate_finite_required("hv", item.hv)?;
    validate_finite("hvPercentile", item.hv_percentile)?;
    Ok(OptionUnderlyingHvItem {
        time_range: item.time_range,
        hv: item.hv,
        hv_percentile: item.hv_percentile,
    })
}

fn security_from_wire(market: i32, code: &str) -> OptionUnderlyingOverviewSecurity {
    let market_label = market_label(market).expect("response validation ensures market");
    let code = code.to_ascii_uppercase();
    OptionUnderlyingOverviewSecurity {
        market: market_label.to_owned(),
        code: code.clone(),
        quote_market: market_label.to_owned(),
        trade_market: market_label.to_owned(),
        instrument_id: format!("{market_label}.{code}"),
    }
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn validate_finite(
    field: &str,
    value: Option<f64>,
) -> Result<(), OptionUnderlyingOverviewQueryError> {
    if let Some(value) = value {
        validate_finite_required(field, value)?;
    }
    Ok(())
}

fn validate_finite_required(
    field: &str,
    value: f64,
) -> Result<(), OptionUnderlyingOverviewQueryError> {
    if !value.is_finite() {
        return Err(OptionUnderlyingOverviewQueryError::InvalidResponse(
            format!("option underlying overview {field} must be finite"),
        ));
    }
    Ok(())
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionUnderlyingOverviewQueryError {
    #[error("invalid OpenD option underlying overview query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option underlying overview session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionUnderlyingOverview response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error(
        "OpenD Qot_GetOptionUnderlyingOverview retType={ret_type} errCode={err_code}: {message}"
    )]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionUnderlyingOverview response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option underlying overview response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_underlying_overview::{
        HvItem, Response, S2c, UnderlyingData,
    };
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionUnderlyingOverviewQuery {
        OptionUnderlyingOverviewQuery {
            market: 11,
            code: " aapl ".to_owned(),
            index_option_type: Some(1),
        }
    }

    fn owner() -> crate::trade_proto::qot_common::Security {
        crate::trade_proto::qot_common::Security {
            market: 11,
            code: "AAPL".to_owned(),
        }
    }

    #[test]
    fn request_uses_owner_list_and_index_option_type() {
        let request = crate::trade_proto::qot_get_option_underlying_overview::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.owner_list[0].code, "AAPL");
        assert_eq!(request.c2s.index_option_type, Some(1));
    }

    #[test]
    fn framed_response_preserves_overview_metrics() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                underlying_data_list: vec![UnderlyingData {
                    owner: owner(),
                    code: Some("AAPL".to_owned()),
                    name: Some("Apple".to_owned()),
                    call_volume: Some(120),
                    put_volume: Some(80),
                    call_open_interest: Some(900),
                    put_open_interest: Some(700),
                    iv: Some(25.0),
                    iv_rank: Some(60.0),
                    iv_percentile: Some(55.0),
                    hv_list: vec![HvItem {
                        time_range: 1,
                        hv: 20.0,
                        hv_percentile: Some(45.0),
                    }],
                    pre_iv: Some(24.0),
                }],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_underlying_overview::PROTOCOL_ID,
            6,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3303);
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(snapshot.items[0].security.instrument_id, "US.AAPL");
        assert_eq!(snapshot.items[0].name.as_deref(), Some("Apple"));
        assert_eq!(snapshot.items[0].hv_list[0].hv, 20.0);
    }

    #[test]
    fn rejects_invalid_query_owner_and_non_finite_metrics() {
        assert!(matches!(
            validate_query(&OptionUnderlyingOverviewQuery {
                market: 11,
                code: "AAPL".to_owned(),
                index_option_type: Some(3),
            }),
            Err(OptionUnderlyingOverviewQueryError::InvalidQuery(_))
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
            Err(OptionUnderlyingOverviewQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                underlying_data_list: vec![UnderlyingData {
                    owner: owner(),
                    iv: Some(f64::NAN),
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionUnderlyingOverviewQueryError::InvalidResponse(_))
        ));
    }
}
