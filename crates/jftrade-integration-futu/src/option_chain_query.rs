//! Typed OpenD option-chain reader (Qot_GetOptionChain/3209).
//!
//! This adapter keeps the generated protobuf types inside the Futu boundary
//! and emits a broker-neutral static chain. Dynamic quotes are intentionally
//! left to the snapshot path so a chain read never creates subscriptions.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::format_description::{FormatItem, parse_borrowed};
use time::{Date, Duration};

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq)]
pub struct OptionChainQuery {
    pub market: i32,
    pub symbol: String,
    pub index_option_type: Option<i32>,
    pub option_type: Option<i32>,
    pub condition: Option<i32>,
    pub begin_time: String,
    pub end_time: String,
    pub data_filter: Option<OptionChainDataFilter>,
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct OptionChainDataFilter {
    pub implied_volatility_min: Option<f64>,
    pub implied_volatility_max: Option<f64>,
    pub delta_min: Option<f64>,
    pub delta_max: Option<f64>,
    pub gamma_min: Option<f64>,
    pub gamma_max: Option<f64>,
    pub vega_min: Option<f64>,
    pub vega_max: Option<f64>,
    pub theta_min: Option<f64>,
    pub theta_max: Option<f64>,
    pub rho_min: Option<f64>,
    pub rho_max: Option<f64>,
    pub net_open_interest_min: Option<f64>,
    pub net_open_interest_max: Option<f64>,
    pub open_interest_min: Option<f64>,
    pub open_interest_max: Option<f64>,
    pub vol_min: Option<f64>,
    pub vol_max: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionChainDate {
    pub strike_time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_timestamp: Option<f64>,
    #[serde(rename = "option")]
    pub options: Vec<OptionChainItem>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionChainItem {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub call: Option<OptionContract>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub put: Option<OptionContract>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContract {
    pub basic: OptionContractBasic,
    pub option_ex_data: OptionContractExData,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContractBasic {
    pub security: OptionSecurity,
    pub id: i64,
    pub lot_size: i32,
    pub sec_type: String,
    pub name: String,
    pub list_time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delisting: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub list_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exch_type: Option<i32>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionContractExData {
    #[serde(rename = "type")]
    pub option_type: String,
    pub owner: OptionSecurity,
    pub strike_time: String,
    pub strike_price: f64,
    pub suspend: bool,
    pub market: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub index_option_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expiration_cycle: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_standard_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_settlement_mode: Option<String>,
}

pub trait OptionChainReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionChainQuery,
    ) -> Result<Vec<OptionChainDate>, OptionChainQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionChainReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionChainReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionChainReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionChainReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionChainReadPort for OpenDOptionChainReader {
    fn query(
        &self,
        query: &OptionChainQuery,
    ) -> Result<Vec<OptionChainDate>, OptionChainQueryError> {
        validate_query(query)?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OptionChainQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_chain::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &OptionChainQuery) -> Result<(), OptionChainQueryError> {
    if !matches!(query.market, 1 | 11) {
        return Err(OptionChainQueryError::InvalidQuery(
            "option chain market must be HK (1) or US (11)".to_owned(),
        ));
    }
    if query.symbol.trim().is_empty()
        || query.symbol.contains('.')
        || query.symbol.trim().chars().any(char::is_whitespace)
    {
        return Err(OptionChainQueryError::InvalidQuery(
            "option chain symbol must be a non-empty code".to_owned(),
        ));
    }
    for (name, value) in [
        ("index option type", query.index_option_type),
        ("option type", query.option_type),
        ("option condition", query.condition),
    ] {
        if let Some(value) = value
            && !(0..=2).contains(&value)
        {
            return Err(OptionChainQueryError::InvalidQuery(format!(
                "{name} must be 0, 1, or 2"
            )));
        }
    }
    let format: &[FormatItem<'_>] = &parse_borrowed::<2>("[year]-[month]-[day]")
        .map_err(|_| OptionChainQueryError::InvalidQuery("invalid date format".to_owned()))?;
    let begin = Date::parse(query.begin_time.trim(), format).map_err(|_| {
        OptionChainQueryError::InvalidQuery("beginTime must be YYYY-MM-DD".to_owned())
    })?;
    let end = Date::parse(query.end_time.trim(), format).map_err(|_| {
        OptionChainQueryError::InvalidQuery("endTime must be YYYY-MM-DD".to_owned())
    })?;
    if end < begin || end - begin > Duration::days(31) {
        return Err(OptionChainQueryError::InvalidQuery(
            "option chain date range must be ordered and no more than 31 days".to_owned(),
        ));
    }
    validate_filter(query.data_filter.as_ref())
}

fn validate_filter(filter: Option<&OptionChainDataFilter>) -> Result<(), OptionChainQueryError> {
    let Some(filter) = filter else {
        return Ok(());
    };
    let values = [
        (
            "implied volatility",
            filter.implied_volatility_min,
            filter.implied_volatility_max,
        ),
        ("delta", filter.delta_min, filter.delta_max),
        ("gamma", filter.gamma_min, filter.gamma_max),
        ("vega", filter.vega_min, filter.vega_max),
        ("theta", filter.theta_min, filter.theta_max),
        ("rho", filter.rho_min, filter.rho_max),
        (
            "net open interest",
            filter.net_open_interest_min,
            filter.net_open_interest_max,
        ),
        (
            "open interest",
            filter.open_interest_min,
            filter.open_interest_max,
        ),
        ("volume", filter.vol_min, filter.vol_max),
    ];
    for (name, min, max) in values {
        for value in [min, max].into_iter().flatten() {
            if !value.is_finite() {
                return Err(OptionChainQueryError::InvalidQuery(format!(
                    "{name} filter must be finite"
                )));
            }
        }
        if let (Some(min), Some(max)) = (min, max)
            && min > max
        {
            return Err(OptionChainQueryError::InvalidQuery(format!(
                "{name} filter minimum must not exceed maximum"
            )));
        }
    }
    Ok(())
}

fn encode_request(query: &OptionChainQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_chain::{C2s, DataFilter, Request};
    Request {
        c2s: C2s {
            owner: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.symbol.trim().to_ascii_uppercase(),
            },
            index_option_type: query.index_option_type,
            r#type: query.option_type,
            condition: query.condition,
            begin_time: query.begin_time.trim().to_owned(),
            end_time: query.end_time.trim().to_owned(),
            data_filter: query.data_filter.as_ref().map(|filter| DataFilter {
                implied_volatility_min: filter.implied_volatility_min,
                implied_volatility_max: filter.implied_volatility_max,
                delta_min: filter.delta_min,
                delta_max: filter.delta_max,
                gamma_min: filter.gamma_min,
                gamma_max: filter.gamma_max,
                vega_min: filter.vega_min,
                vega_max: filter.vega_max,
                theta_min: filter.theta_min,
                theta_max: filter.theta_max,
                rho_min: filter.rho_min,
                rho_max: filter.rho_max,
                net_open_interest_min: filter.net_open_interest_min,
                net_open_interest_max: filter.net_open_interest_max,
                open_interest_min: filter.open_interest_min,
                open_interest_max: filter.open_interest_max,
                vol_min: filter.vol_min,
                vol_max: filter.vol_max,
            }),
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(body: &[u8]) -> Result<Vec<OptionChainDate>, OptionChainQueryError> {
    use crate::trade_proto::qot_get_option_chain::Response;
    let response = Response::decode(body).map_err(OptionChainQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionChainQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option chain request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionChainQueryError::MissingS2c);
    };
    s2c.option_chain.into_iter().map(map_chain_date).collect()
}

fn map_chain_date(
    chain: crate::trade_proto::qot_get_option_chain::OptionChain,
) -> Result<OptionChainDate, OptionChainQueryError> {
    let strike_time = chain.strike_time.trim().to_owned();
    if !is_date(&strike_time) {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain strikeTime must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(timestamp) = chain.strike_timestamp
        && !timestamp.is_finite()
    {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain strikeTimestamp must be finite".to_owned(),
        ));
    }
    let mut options = Vec::with_capacity(chain.option.len());
    for item in chain.option {
        if item.call.is_none() && item.put.is_none() {
            return Err(OptionChainQueryError::InvalidResponse(
                "option chain item must contain call or put".to_owned(),
            ));
        }
        options.push(OptionChainItem {
            call: item
                .call
                .map(|value| map_contract(value, 1, &strike_time))
                .transpose()?,
            put: item
                .put
                .map(|value| map_contract(value, 2, &strike_time))
                .transpose()?,
        });
    }
    Ok(OptionChainDate {
        strike_time,
        strike_timestamp: chain.strike_timestamp,
        options,
    })
}

fn map_contract(
    info: crate::trade_proto::qot_common::SecurityStaticInfo,
    expected_type: i32,
    chain_strike_time: &str,
) -> Result<OptionContract, OptionChainQueryError> {
    let basic = info.basic;
    let security = map_security(basic.security)?;
    if basic.sec_type != 8 {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain contract security type is not option".to_owned(),
        ));
    }
    let option = info.option_ex_data.ok_or_else(|| {
        OptionChainQueryError::InvalidResponse(
            "option chain contract missing optionExData".to_owned(),
        )
    })?;
    if option.r#type != expected_type {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain call/put type does not match its side".to_owned(),
        ));
    }
    if option.strike_time.trim() != chain_strike_time || !is_date(option.strike_time.trim()) {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain contract strikeTime disagrees with chain".to_owned(),
        ));
    }
    if !option.strike_price.is_finite() {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain strikePrice must be finite".to_owned(),
        ));
    }
    let owner = map_security(option.owner)?;
    for timestamp in [basic.list_timestamp, option.strike_timestamp]
        .into_iter()
        .flatten()
    {
        if !timestamp.is_finite() {
            return Err(OptionChainQueryError::InvalidResponse(
                "option chain timestamp must be finite".to_owned(),
            ));
        }
    }
    Ok(OptionContract {
        basic: OptionContractBasic {
            security,
            id: basic.id,
            lot_size: basic.lot_size,
            sec_type: "drvt".to_owned(),
            name: basic.name,
            list_time: basic.list_time,
            delisting: basic.delisting,
            list_timestamp: basic.list_timestamp,
            exch_type: basic.exch_type,
        },
        option_ex_data: OptionContractExData {
            option_type: option_type_label(option.r#type),
            owner,
            strike_time: option.strike_time,
            strike_price: option.strike_price,
            suspend: option.suspend,
            market: option.market,
            strike_timestamp: option.strike_timestamp,
            index_option_type: option.index_option_type.map(index_option_type_label),
            expiration_cycle: option.expiration_cycle.map(expiration_cycle_label),
            option_standard_type: option.option_standard_type.map(option_standard_type_label),
            option_settlement_mode: option
                .option_settlement_mode
                .map(option_settlement_mode_label),
        },
    })
}

fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionSecurity, OptionChainQueryError> {
    let market = market_label(security.market).ok_or_else(|| {
        OptionChainQueryError::InvalidResponse(
            "option chain security market is unsupported".to_owned(),
        )
    })?;
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(OptionChainQueryError::InvalidResponse(
            "option chain security code is empty".to_owned(),
        ));
    }
    Ok(OptionSecurity {
        market: market.to_owned(),
        code: code.clone(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    })
}

fn is_date(value: &str) -> bool {
    let Ok(format) = parse_borrowed::<2>("[year]-[month]-[day]") else {
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

fn option_type_label(value: i32) -> String {
    match value {
        1 => "call",
        2 => "put",
        _ => "unknown",
    }
    .to_owned()
}

fn index_option_type_label(value: i32) -> String {
    match value {
        1 => "normal",
        2 => "small",
        _ => "unknown",
    }
    .to_owned()
}

fn expiration_cycle_label(value: i32) -> String {
    match value {
        1 => "week",
        2 => "month",
        3 => "month_end",
        4 => "quarter",
        11 => "week_mon",
        12 => "week_tue",
        13 => "week_wed",
        14 => "week_thu",
        15 => "week_fri",
        _ => "unknown",
    }
    .to_owned()
}

fn option_standard_type_label(value: i32) -> String {
    match value {
        1 => "standard",
        2 => "non_standard",
        _ => "unknown",
    }
    .to_owned()
}

fn option_settlement_mode_label(value: i32) -> String {
    match value {
        1 => "am",
        2 => "pm",
        _ => "unknown",
    }
    .to_owned()
}

#[derive(Debug, Error)]
pub enum OptionChainQueryError {
    #[error("invalid OpenD option chain query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option chain session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionChain response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionChain retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionChain response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option chain response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_common::{
        OptionStaticExData, Security, SecurityStaticBasic, SecurityStaticInfo,
    };
    use crate::trade_proto::qot_get_option_chain::{OptionChain, OptionItem, Response, S2c};

    fn query() -> OptionChainQuery {
        OptionChainQuery {
            market: 11,
            symbol: " aapl ".to_owned(),
            index_option_type: Some(1),
            option_type: Some(0),
            condition: Some(2),
            begin_time: "2026-09-01".to_owned(),
            end_time: "2026-09-30".to_owned(),
            data_filter: None,
        }
    }

    #[test]
    fn request_preserves_owner_dates_and_filters() {
        let request = crate::trade_proto::qot_get_option_chain::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        let c2s = request.c2s;
        assert_eq!(c2s.owner.market, 11);
        assert_eq!(c2s.owner.code, "AAPL");
        assert_eq!(c2s.index_option_type, Some(1));
        assert_eq!(c2s.r#type, Some(0));
        assert_eq!(c2s.condition, Some(2));
        assert_eq!(c2s.begin_time, "2026-09-01");
        assert_eq!(c2s.end_time, "2026-09-30");
    }

    #[test]
    fn framed_response_maps_static_contracts_to_neutral_chain() {
        let call = SecurityStaticInfo {
            basic: SecurityStaticBasic {
                security: Security {
                    market: 11,
                    code: "AAPL260918C00100000".to_owned(),
                },
                id: 7,
                lot_size: 100,
                sec_type: 8,
                name: "AAPL Call".to_owned(),
                list_time: "2026-01-01".to_owned(),
                delisting: Some(false),
                list_timestamp: Some(1_767_225_600.0),
                exch_type: Some(8),
            },
            option_ex_data: Some(OptionStaticExData {
                r#type: 1,
                owner: Security {
                    market: 11,
                    code: "AAPL".to_owned(),
                },
                strike_time: "2026-09-18".to_owned(),
                strike_price: 100.0,
                suspend: false,
                market: "US".to_owned(),
                strike_timestamp: Some(1_789_000_000.0),
                index_option_type: Some(1),
                expiration_cycle: Some(2),
                option_standard_type: Some(1),
                option_settlement_mode: Some(2),
            }),
            warrant_ex_data: None,
            future_ex_data: None,
        };
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_chain: vec![OptionChain {
                    strike_time: "2026-09-18".to_owned(),
                    option: vec![OptionItem {
                        call: Some(call),
                        put: None,
                    }],
                    strike_timestamp: Some(1_789_000_000.0),
                }],
            }),
        }
        .encode_to_vec();
        let dates = decode_response(&body).expect("chain");
        let json = serde_json::to_value(&dates[0]).expect("json");
        assert_eq!(
            json["option"][0]["call"]["basic"]["security"]["instrumentId"],
            "US.AAPL260918C00100000"
        );
        assert_eq!(json["option"][0]["call"]["optionExData"]["type"], "call");
        assert_eq!(
            json["option"][0]["call"]["optionExData"]["strikePrice"],
            100.0
        );
    }

    #[test]
    fn rejects_bad_ranges_missing_s2c_and_invalid_contracts() {
        let mut invalid = query();
        invalid.end_time = "2026-11-01".to_owned();
        assert!(matches!(
            validate_query(&invalid),
            Err(OptionChainQueryError::InvalidQuery(_))
        ));
        let missing = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        };
        assert!(matches!(
            decode_response(&missing.encode_to_vec()),
            Err(OptionChainQueryError::MissingS2c)
        ));
        let empty = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_chain: vec![OptionChain {
                    strike_time: "".to_owned(),
                    option: vec![],
                    strike_timestamp: None,
                }],
            }),
        };
        assert!(matches!(
            decode_response(&empty.encode_to_vec()),
            Err(OptionChainQueryError::InvalidResponse(_))
        ));
    }
}
