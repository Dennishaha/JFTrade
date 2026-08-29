//! Typed OpenD option-contract ranking reader (`Qot_GetOptionRank/3306`).
//!
//! This adapter translates the public HK/US market identifiers to OpenD's
//! option-market identifiers and keeps generated protobuf messages out of the
//! engine. Contract-level filters are intentionally bounded to the common
//! sorting, date, and pagination controls exposed by the product route.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionContractRankQuery {
    /// Public quote market: HK (1) or US (11).
    pub market: i32,
    pub sort_type: i32,
    pub count: Option<i32>,
    pub trading_date: Option<String>,
    pub is_asc: Option<bool>,
    pub page: Option<String>,
}

impl OptionContractRankQuery {
    pub fn validate(&self) -> Result<(), OptionContractRankQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContractRankSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContractRankItem {
    pub security: OptionContractRankSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub oi_increment: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub oi_decrement: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub oi_market_cap_increment: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub oi_market_cap_decrement: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest_market_cap: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mid_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gamma: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub theta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vega: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rho: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContractRankSnapshot {
    pub market: String,
    pub sort_type: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trading_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub trading_timestamp: Option<f64>,
    pub items: Vec<OptionContractRankItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
}

pub trait OptionContractRankReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionContractRankQuery,
    ) -> Result<OptionContractRankSnapshot, OptionContractRankQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionContractRankReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionContractRankReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionContractRankReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionContractRankReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionContractRankReadPort for OpenDOptionContractRankReader {
    fn query(
        &self,
        query: &OptionContractRankQuery,
    ) -> Result<OptionContractRankSnapshot, OptionContractRankQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionContractRankQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_rank::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &OptionContractRankQuery) -> Result<(), OptionContractRankQueryError> {
    market_label(query.market).ok_or_else(|| {
        OptionContractRankQueryError::InvalidQuery(
            "option contract rank market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    if !(1..=10).contains(&query.sort_type) {
        return Err(OptionContractRankQueryError::InvalidQuery(
            "option contract rank sortType must be between 1 and 10".to_owned(),
        ));
    }
    if let Some(count) = query.count
        && !(1..=200).contains(&count)
    {
        return Err(OptionContractRankQueryError::InvalidQuery(
            "option contract rank count must be between 1 and 200".to_owned(),
        ));
    }
    if let Some(trading_date) = query.trading_date.as_deref()
        && !is_date(trading_date.trim())
    {
        return Err(OptionContractRankQueryError::InvalidQuery(
            "option contract rank tradingDate must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(page) = query.page.as_deref()
        && (page.len() > 1024 || page.chars().any(char::is_control))
    {
        return Err(OptionContractRankQueryError::InvalidQuery(
            "option contract rank page token is invalid".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionContractRankQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_rank::{C2s, Request};
    Request {
        c2s: C2s {
            option_market: option_market(query.market),
            sort_type: query.sort_type,
            count: query.count,
            trading_date: query
                .trading_date
                .as_deref()
                .map(str::trim)
                .map(ToOwned::to_owned),
            is_asc: query.is_asc,
            page: query
                .page
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned),
            filter_list: Vec::new(),
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionContractRankQuery,
) -> Result<OptionContractRankSnapshot, OptionContractRankQueryError> {
    use crate::trade_proto::qot_get_option_rank::Response;
    let response = Response::decode(body).map_err(OptionContractRankQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionContractRankQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option contract rank request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionContractRankQueryError::MissingS2c);
    };
    if s2c.option_market != option_market(query.market) || s2c.sort_type != query.sort_type {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank response scope does not match query".to_owned(),
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
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank tradingDate must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(value) = s2c.trading_timestamp
        && !value.is_finite()
    {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank tradingTimestamp must be finite".to_owned(),
        ));
    }
    if let Some(value) = s2c.all_count
        && (value < 0 || value < s2c.rank_list.len() as i32)
    {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank allCount is inconsistent with rankList".to_owned(),
        ));
    }
    let items = s2c
        .rank_list
        .into_iter()
        .map(map_item)
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
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank nextPage token is invalid".to_owned(),
        ));
    }
    let market = market_label(query.market).expect("query validation ensures market");
    Ok(OptionContractRankSnapshot {
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
    item: crate::trade_proto::qot_get_option_rank::OptionRankItem,
) -> Result<OptionContractRankItem, OptionContractRankQueryError> {
    let option = item.option;
    if !matches!(option.market, 1 | 11) || option.code.trim().is_empty() {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank option security is invalid".to_owned(),
        ));
    }
    if option.code.chars().any(char::is_whitespace) {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank option code is invalid".to_owned(),
        ));
    }
    if let Some(option_type) = item.option_type
        && !matches!(option_type, 1 | 2)
    {
        return Err(OptionContractRankQueryError::InvalidResponse(
            "option contract rank optionType is unsupported".to_owned(),
        ));
    }
    for (field, value) in [
        ("oiMarketCapIncrement", item.oi_market_cap_increment),
        ("oiMarketCapDecrement", item.oi_market_cap_decrement),
        ("turnover", item.turnover),
        ("openInterestMarketCap", item.open_interest_market_cap),
        ("iv", item.iv),
        ("optionPrice", item.option_price),
        ("changeRate", item.change_rate),
        ("midPrice", item.mid_price),
        ("bidPrice", item.bid_price),
        ("askPrice", item.ask_price),
        ("delta", item.delta),
        ("gamma", item.gamma),
        ("theta", item.theta),
        ("vega", item.vega),
        ("rho", item.rho),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionContractRankQueryError::InvalidResponse(format!(
                "option contract rank {field} must be finite"
            )));
        }
    }
    for (field, value) in [
        ("oiIncrement", item.oi_increment),
        ("oiDecrement", item.oi_decrement),
        ("volume", item.volume),
        ("openInterest", item.open_interest),
        ("bidVolume", item.bid_volume),
        ("askVolume", item.ask_volume),
    ] {
        if let Some(value) = value
            && value < 0
        {
            return Err(OptionContractRankQueryError::InvalidResponse(format!(
                "option contract rank {field} must be non-negative"
            )));
        }
    }
    for (field, value) in [
        ("oiMarketCapIncrement", item.oi_market_cap_increment),
        ("oiMarketCapDecrement", item.oi_market_cap_decrement),
    ] {
        if let Some(value) = value
            && value < 0.0
        {
            return Err(OptionContractRankQueryError::InvalidResponse(format!(
                "option contract rank {field} must be non-negative"
            )));
        }
    }
    Ok(OptionContractRankItem {
        security: security_from_wire(option.market, option.code.trim()),
        name: optional_text(item.name),
        option_type: item.option_type,
        oi_increment: item.oi_increment,
        oi_decrement: item.oi_decrement,
        oi_market_cap_increment: item.oi_market_cap_increment,
        oi_market_cap_decrement: item.oi_market_cap_decrement,
        volume: item.volume,
        turnover: item.turnover,
        open_interest: item.open_interest,
        open_interest_market_cap: item.open_interest_market_cap,
        iv: item.iv,
        option_price: item.option_price,
        change_rate: item.change_rate,
        mid_price: item.mid_price,
        bid_price: item.bid_price,
        bid_volume: item.bid_volume,
        ask_price: item.ask_price,
        ask_volume: item.ask_volume,
        delta: item.delta,
        gamma: item.gamma,
        theta: item.theta,
        vega: item.vega,
        rho: item.rho,
    })
}

fn security_from_wire(market: i32, code: &str) -> OptionContractRankSecurity {
    let market_label = market_label(market).expect("response validation ensures market");
    let code = code.to_ascii_uppercase();
    OptionContractRankSecurity {
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
pub enum OptionContractRankQueryError {
    #[error("invalid OpenD option contract rank query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option contract rank session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionRank response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionRank retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionRank response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option contract rank response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_rank::{OptionRankItem, Response, S2c};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionContractRankQuery {
        OptionContractRankQuery {
            market: 11,
            sort_type: 10,
            count: Some(25),
            trading_date: Some("2026-08-29".to_owned()),
            is_asc: Some(true),
            page: Some("next".to_owned()),
        }
    }

    fn option() -> crate::trade_proto::qot_common::Security {
        crate::trade_proto::qot_common::Security {
            market: 11,
            code: "AAPL260918C00100000".to_owned(),
        }
    }

    #[test]
    fn request_maps_market_and_pagination() {
        let request = crate::trade_proto::qot_get_option_rank::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.option_market, 1);
        assert_eq!(request.c2s.sort_type, 10);
        assert_eq!(request.c2s.count, Some(25));
        assert_eq!(request.c2s.trading_date.as_deref(), Some("2026-08-29"));
        assert_eq!(request.c2s.is_asc, Some(true));
        assert_eq!(request.c2s.page.as_deref(), Some("next"));
    }

    #[test]
    fn framed_response_preserves_contract_metrics_and_metadata() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                sort_type: 10,
                trading_date: Some("2026-08-29".to_owned()),
                trading_timestamp: Some(1_756_000_000.0),
                rank_list: vec![OptionRankItem {
                    option: option(),
                    name: Some("AAPL Call".to_owned()),
                    option_type: Some(1),
                    volume: Some(1200),
                    open_interest: Some(900),
                    iv: Some(25.0),
                    option_price: Some(1.25),
                    delta: Some(0.5),
                    ..Default::default()
                }],
                next_page: Some("next-2".to_owned()),
                all_count: Some(42),
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_rank::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3306);
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(snapshot.market, "US");
        assert_eq!(
            snapshot.items[0].security.instrument_id,
            "US.AAPL260918C00100000"
        );
        assert_eq!(snapshot.items[0].option_price, Some(1.25));
        assert_eq!(snapshot.next_page.as_deref(), Some("next-2"));
    }

    #[test]
    fn rejects_invalid_query_and_response_values() {
        assert!(matches!(
            validate_query(&OptionContractRankQuery {
                market: 11,
                sort_type: 11,
                count: Some(201),
                trading_date: Some("2026-02-30".to_owned()),
                is_asc: None,
                page: None,
            }),
            Err(OptionContractRankQueryError::InvalidQuery(_))
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
            Err(OptionContractRankQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                sort_type: 10,
                rank_list: vec![OptionRankItem {
                    option: option(),
                    volume: Some(-1),
                    iv: Some(f64::NAN),
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionContractRankQueryError::InvalidResponse(_))
        ));
    }
}
