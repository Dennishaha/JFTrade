//! Typed OpenD option-underlying ranking reader
//! (`Qot_GetOptionUnderlyingRank/3305`).
//!
//! The wire protocol uses option-market identifiers (US security = 1 and HK
//! security = 3), while the public product contract uses the normal quote
//! market identifiers (US = 11 and HK = 1). This adapter keeps that translation
//! and all generated protobuf types inside the Futu boundary.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionUnderlyingRankQuery {
    /// Public quote market: HK (1) or US (11). The protocol maps these to 3
    /// and 1 respectively because it ranks security underlyings.
    pub market: i32,
    pub sort_type: i32,
    pub is_asc: Option<bool>,
    pub count: Option<i32>,
    pub trading_date: Option<String>,
    pub page: Option<String>,
}

impl OptionUnderlyingRankQuery {
    pub fn validate(&self) -> Result<(), OptionUnderlyingRankQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingRankSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingRankItem {
    pub security: OptionUnderlyingRankSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_rank: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_percentile: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_change: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv_change: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingRankSnapshot {
    pub market: String,
    pub sort_type: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trading_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trading_timestamp: Option<f64>,
    pub items: Vec<OptionUnderlyingRankItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
}

pub trait OptionUnderlyingRankReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionUnderlyingRankQuery,
    ) -> Result<OptionUnderlyingRankSnapshot, OptionUnderlyingRankQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionUnderlyingRankReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionUnderlyingRankReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionUnderlyingRankReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionUnderlyingRankReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionUnderlyingRankReadPort for OpenDOptionUnderlyingRankReader {
    fn query(
        &self,
        query: &OptionUnderlyingRankQuery,
    ) -> Result<OptionUnderlyingRankSnapshot, OptionUnderlyingRankQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionUnderlyingRankQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_underlying_rank::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &OptionUnderlyingRankQuery) -> Result<(), OptionUnderlyingRankQueryError> {
    market_label(query.market).ok_or_else(|| {
        OptionUnderlyingRankQueryError::InvalidQuery(
            "option underlying rank market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    if !(1..=13).contains(&query.sort_type) {
        return Err(OptionUnderlyingRankQueryError::InvalidQuery(
            "option underlying rank sortType must be between 1 and 13".to_owned(),
        ));
    }
    if let Some(count) = query.count
        && !(1..=200).contains(&count)
    {
        return Err(OptionUnderlyingRankQueryError::InvalidQuery(
            "option underlying rank count must be between 1 and 200".to_owned(),
        ));
    }
    if let Some(trading_date) = query.trading_date.as_deref()
        && !is_date(trading_date.trim())
    {
        return Err(OptionUnderlyingRankQueryError::InvalidQuery(
            "option underlying rank tradingDate must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(page) = query.page.as_deref()
        && (page.len() > 1024 || page.chars().any(char::is_control))
    {
        return Err(OptionUnderlyingRankQueryError::InvalidQuery(
            "option underlying rank page token is invalid".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionUnderlyingRankQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_underlying_rank::{C2s, Request};
    Request {
        c2s: C2s {
            option_market: option_market(query.market),
            sort_type: query.sort_type,
            is_asc: query.is_asc,
            count: query.count,
            trading_date: query
                .trading_date
                .as_deref()
                .map(str::trim)
                .map(ToOwned::to_owned),
            filter_list: Vec::new(),
            page: query
                .page
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned),
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionUnderlyingRankQuery,
) -> Result<OptionUnderlyingRankSnapshot, OptionUnderlyingRankQueryError> {
    use crate::trade_proto::qot_get_option_underlying_rank::Response;
    let response = Response::decode(body).map_err(OptionUnderlyingRankQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionUnderlyingRankQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option underlying rank request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionUnderlyingRankQueryError::MissingS2c);
    };
    let expected_option_market = option_market(query.market);
    if s2c.option_market != expected_option_market || s2c.sort_type != query.sort_type {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank response scope does not match query".to_owned(),
        ));
    }
    let trading_date = s2c
        .trading_date
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    if let Some(value) = trading_date.as_deref()
        && !is_date(value)
    {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank tradingDate must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(value) = s2c.trading_timestamp
        && !value.is_finite()
    {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank tradingTimestamp must be finite".to_owned(),
        ));
    }
    if let Some(value) = s2c.all_count
        && (value < 0 || value < s2c.rank_list.len() as i32)
    {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank allCount is inconsistent with rankList".to_owned(),
        ));
    }
    let items = s2c
        .rank_list
        .into_iter()
        .map(|item| map_item(item, query.market))
        .collect::<Result<Vec<_>, _>>()?;
    let next_page = s2c
        .next_page
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    if next_page
        .as_deref()
        .is_some_and(|value| value.len() > 1024 || value.chars().any(char::is_control))
    {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank nextPage token is invalid".to_owned(),
        ));
    }
    let market = market_label(query.market).expect("query validation ensures market");
    Ok(OptionUnderlyingRankSnapshot {
        market: market.to_owned(),
        sort_type: query.sort_type,
        trading_date,
        trading_timestamp: s2c.trading_timestamp,
        items,
        next_page,
        all_count: s2c.all_count,
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_underlying_rank::UnderlyingRankItem,
    expected_market: i32,
) -> Result<OptionUnderlyingRankItem, OptionUnderlyingRankQueryError> {
    if item.owner.market != expected_market || item.owner.code.trim().is_empty() {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank owner does not match query market".to_owned(),
        ));
    }
    if item.owner.code.chars().any(char::is_whitespace) {
        return Err(OptionUnderlyingRankQueryError::InvalidResponse(
            "option underlying rank owner code is invalid".to_owned(),
        ));
    }
    let owner = security_from_wire(item.owner.market, item.owner.code.trim());
    for (field, value) in [
        ("volumeRatio", item.volume_ratio),
        ("openInterestRatio", item.open_interest_ratio),
        ("iv", item.iv),
        ("ivRank", item.iv_rank),
        ("ivPercentile", item.iv_percentile),
        ("price", item.price),
        ("changeRate", item.change_rate),
        ("ivChange", item.iv_change),
        ("hv", item.hv),
        ("hvChange", item.hv_change),
        ("marketCap", item.market_cap),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionUnderlyingRankQueryError::InvalidResponse(format!(
                "option underlying rank {field} must be finite"
            )));
        }
    }
    for (field, value) in [
        ("totalVolume", item.total_volume),
        ("totalOpenInterest", item.total_open_interest),
    ] {
        if let Some(value) = value
            && value < 0
        {
            return Err(OptionUnderlyingRankQueryError::InvalidResponse(format!(
                "option underlying rank {field} must be non-negative"
            )));
        }
    }
    Ok(OptionUnderlyingRankItem {
        security: owner,
        name: optional_text(item.name),
        total_volume: item.total_volume,
        total_open_interest: item.total_open_interest,
        volume_ratio: item.volume_ratio,
        open_interest_ratio: item.open_interest_ratio,
        iv: item.iv,
        iv_rank: item.iv_rank,
        iv_percentile: item.iv_percentile,
        price: item.price,
        change_rate: item.change_rate,
        iv_change: item.iv_change,
        hv: item.hv,
        hv_change: item.hv_change,
        market_cap: item.market_cap,
    })
}

fn security_from_wire(market: i32, code: &str) -> OptionUnderlyingRankSecurity {
    let market_label = market_label(market).expect("response validation ensures market");
    let code = code.to_ascii_uppercase();
    OptionUnderlyingRankSecurity {
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

fn is_date(value: &str) -> bool {
    let Ok(format) = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]") else {
        return false;
    };
    Date::parse(value, &format).is_ok()
}

fn option_market(market: i32) -> i32 {
    match market {
        1 => 3,
        11 => 1,
        _ => unreachable!("query validation ensures a supported market"),
    }
}

fn market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionUnderlyingRankQueryError {
    #[error("invalid OpenD option underlying rank query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option underlying rank session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionUnderlyingRank response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionUnderlyingRank retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionUnderlyingRank response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option underlying rank response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_underlying_rank::{Response, S2c, UnderlyingRankItem};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionUnderlyingRankQuery {
        OptionUnderlyingRankQuery {
            market: 11,
            sort_type: 7,
            is_asc: Some(true),
            count: Some(25),
            trading_date: Some("2026-08-29".to_owned()),
            page: Some("next".to_owned()),
        }
    }

    fn owner() -> crate::trade_proto::qot_common::Security {
        crate::trade_proto::qot_common::Security {
            market: 11,
            code: "AAPL".to_owned(),
        }
    }

    #[test]
    fn request_maps_public_market_and_pagination() {
        let request = crate::trade_proto::qot_get_option_underlying_rank::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.option_market, 1);
        assert_eq!(request.c2s.sort_type, 7);
        assert_eq!(request.c2s.is_asc, Some(true));
        assert_eq!(request.c2s.count, Some(25));
        assert_eq!(request.c2s.trading_date.as_deref(), Some("2026-08-29"));
        assert_eq!(request.c2s.page.as_deref(), Some("next"));
    }

    #[test]
    fn hk_scope_maps_option_market_while_preserving_security_market() {
        let query = OptionUnderlyingRankQuery {
            market: 1,
            sort_type: 1,
            is_asc: None,
            count: None,
            trading_date: None,
            page: None,
        };
        let request = crate::trade_proto::qot_get_option_underlying_rank::Request::decode(
            encode_request(&query).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.option_market, 3);
        let security = security_from_wire(1, "0700");
        assert_eq!(security.market, "HK");
        assert_eq!(security.instrument_id, "HK.0700");
    }

    #[test]
    fn framed_response_preserves_rank_metrics_and_metadata() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                sort_type: 7,
                trading_date: Some("2026-08-29".to_owned()),
                trading_timestamp: Some(1_756_000_000.0),
                rank_list: vec![UnderlyingRankItem {
                    owner: owner(),
                    name: Some("Apple".to_owned()),
                    total_volume: Some(1200),
                    total_open_interest: Some(900),
                    volume_ratio: Some(80.0),
                    iv: Some(25.0),
                    iv_rank: Some(60.0),
                    price: Some(225.0),
                    ..Default::default()
                }],
                next_page: Some("next-2".to_owned()),
                all_count: Some(42),
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_underlying_rank::PROTOCOL_ID,
            8,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3305);
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(snapshot.market, "US");
        assert_eq!(snapshot.items[0].security.instrument_id, "US.AAPL");
        assert_eq!(snapshot.items[0].total_volume, Some(1200));
        assert_eq!(snapshot.next_page.as_deref(), Some("next-2"));
        assert_eq!(snapshot.all_count, Some(42));
    }

    #[test]
    fn rejects_invalid_query_scope_and_response_values() {
        assert!(matches!(
            validate_query(&OptionUnderlyingRankQuery {
                market: 11,
                sort_type: 14,
                is_asc: None,
                count: Some(201),
                trading_date: Some("2026-02-30".to_owned()),
                page: None,
            }),
            Err(OptionUnderlyingRankQueryError::InvalidQuery(_))
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
            Err(OptionUnderlyingRankQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                sort_type: 7,
                rank_list: vec![UnderlyingRankItem {
                    owner: owner(),
                    total_volume: Some(-1),
                    iv: Some(f64::NAN),
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionUnderlyingRankQueryError::InvalidResponse(_))
        ));
    }
}
