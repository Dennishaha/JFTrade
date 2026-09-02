//! Typed OpenD option-screen reader (Qot_OptionScreen/3253).
//!
//! Generated protobuf messages stay inside the Futu integration boundary. The
//! engine receives a broker-neutral page of option contracts and never needs
//! to know about OpenD enum names or pointer-backed protobuf fields.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::{Date, Month};

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct OptionScreenQuery {
    pub market_categories: Vec<i32>,
    pub page_from: Option<i32>,
    pub page_count: Option<i32>,
    pub option_retrieve_list: Vec<i32>,
    pub underlying_retrieve_list: Vec<i32>,
}

/// Broker-neutral option-screen row. Optional fields are retained as optional
/// because OpenD only returns fields requested by `*_retrieve_list`.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionScreenItem {
    pub security: OptionScreenSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_date: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exercise_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expiration_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub in_the_money: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub left_day: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mid_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_ask_spread: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_ask_volume_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest_market_cap: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vol_oi_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub premium: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub implied_volatility: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub delta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gamma: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vega: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub theta: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rho: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub leverage_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub effective_gearing: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub itm_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub underlying_info: Option<OptionScreenUnderlyingInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub history_volatility: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_hv_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub buy_to_bep: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sell_to_bep: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub buy_profit_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sell_profit_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub intrinsic_value_per: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub time_value_per: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub itm_degree: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub otm_degree: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub otm_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sell_annualized_return: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub interval_return: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionScreenSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionScreenUnderlyingInfo {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stock_id: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_rank: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_percentile: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
}

pub trait OptionScreenReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &OptionScreenQuery) -> Result<OptionScreenPage, OptionScreenQueryError>;
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionScreenPage {
    pub last_page: bool,
    pub all_count: i32,
    pub items: Vec<OptionScreenItem>,
}

#[derive(Clone)]
pub struct OpenDOptionScreenReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionScreenReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionScreenReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionScreenReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionScreenReadPort for OpenDOptionScreenReader {
    fn query(&self, query: &OptionScreenQuery) -> Result<OptionScreenPage, OptionScreenQueryError> {
        validate_query(query)?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OptionScreenQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_option_screen::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &OptionScreenQuery) -> Result<(), OptionScreenQueryError> {
    if query.market_categories.is_empty() || query.market_categories.len() > 7 {
        return Err(OptionScreenQueryError::InvalidQuery(
            "option screen requires one to seven market categories".to_owned(),
        ));
    }
    for (index, category) in query.market_categories.iter().enumerate() {
        if !(0..=6).contains(category) {
            return Err(OptionScreenQueryError::InvalidQuery(format!(
                "market category {category} is outside 0..6"
            )));
        }
        if query.market_categories[..index].contains(category) {
            return Err(OptionScreenQueryError::InvalidQuery(
                "market categories must be unique".to_owned(),
            ));
        }
    }
    if let Some(page_from) = query.page_from
        && page_from < 0
    {
        return Err(OptionScreenQueryError::InvalidQuery(
            "pageFrom must be non-negative".to_owned(),
        ));
    }
    if let Some(page_count) = query.page_count
        && !(1..=1000).contains(&page_count)
    {
        return Err(OptionScreenQueryError::InvalidQuery(
            "pageCount must be between 1 and 1000".to_owned(),
        ));
    }
    validate_retrieve_list(
        "optionRetrieveList",
        &query.option_retrieve_list,
        option_indicator,
    )?;
    validate_retrieve_list(
        "underlyingRetrieveList",
        &query.underlying_retrieve_list,
        underlying_indicator,
    )?;
    Ok(())
}

fn validate_retrieve_list(
    name: &str,
    values: &[i32],
    allowed: fn(i32) -> bool,
) -> Result<(), OptionScreenQueryError> {
    for (index, value) in values.iter().enumerate() {
        if !allowed(*value) {
            return Err(OptionScreenQueryError::InvalidQuery(format!(
                "{name} contains unsupported indicator {value}"
            )));
        }
        if values[..index].contains(value) {
            return Err(OptionScreenQueryError::InvalidQuery(format!(
                "{name} must not contain duplicates"
            )));
        }
    }
    Ok(())
}

fn option_indicator(value: i32) -> bool {
    matches!(
        value,
        1001..=1005
            | 1007
            | 2001..=2014
            | 2018
            | 2021
            | 3001..=3022
    )
}

fn underlying_indicator(value: i32) -> bool {
    matches!(value, 101 | 106 | 201..=209 | 401..=403)
}

fn encode_request(query: &OptionScreenQuery) -> Vec<u8> {
    use crate::trade_proto::qot_option_screen::{C2s, Request};
    Request {
        c2s: C2s {
            market_category_list: query.market_categories.clone(),
            filter_list: Vec::new(),
            sort_list: Vec::new(),
            page_from: query.page_from,
            page_count: query.page_count,
            option_retrieve_list: query.option_retrieve_list.clone(),
            underlying_retrieve_list: query.underlying_retrieve_list.clone(),
        },
    }
    .encode_to_vec()
}

fn decode_response(body: &[u8]) -> Result<OptionScreenPage, OptionScreenQueryError> {
    use crate::trade_proto::qot_option_screen::Response;
    let response = Response::decode(body).map_err(OptionScreenQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionScreenQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option screen request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionScreenQueryError::MissingS2c);
    };
    let last_page = s2c.last_page;
    let all_count = s2c.all_count;
    if all_count < 0 || all_count < s2c.data_list.len() as i32 {
        return Err(OptionScreenQueryError::InvalidResponse(
            "option screen allCount is inconsistent with dataList".to_owned(),
        ));
    }
    let items = s2c
        .data_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionScreenPage {
        last_page,
        all_count,
        items,
    })
}

fn map_item(
    item: crate::trade_proto::qot_option_screen::OptionScreenItem,
) -> Result<OptionScreenItem, OptionScreenQueryError> {
    let security = item
        .security
        .ok_or(OptionScreenQueryError::InvalidResponse(
            "option screen item missing security".to_owned(),
        ))?;
    let security = map_security(security)?;
    let result = OptionScreenItem {
        security,
        option_name: item.option_name,
        strike_price: item.strike_price,
        strike_date: item.strike_date,
        option_type: item.option_type,
        exercise_type: item.exercise_type,
        expiration_type: item.expiration_type,
        in_the_money: item.in_the_money,
        left_day: item.left_day,
        price: item.price,
        mid_price: item.mid_price,
        bid_price: item.bid_price,
        ask_price: item.ask_price,
        bid_ask_spread: item.bid_ask_spread,
        bid_volume: item.bid_volume,
        ask_volume: item.ask_volume,
        change_rate: item.change_rate,
        volume: item.volume,
        turnover: item.turnover,
        open_interest: item.open_interest,
        bid_ask_volume_ratio: item.bid_ask_volume_ratio,
        open_interest_market_cap: item.open_interest_market_cap,
        vol_oi_ratio: item.vol_oi_ratio,
        premium: item.premium,
        implied_volatility: item.implied_volatility,
        delta: item.delta,
        gamma: item.gamma,
        vega: item.vega,
        theta: item.theta,
        rho: item.rho,
        leverage_ratio: item.leverage_ratio,
        effective_gearing: item.effective_gearing,
        itm_probability: item.itm_probability,
        underlying_info: item.underlying_info.map(map_underlying_info).transpose()?,
        history_volatility: item.history_volatility,
        iv_hv_ratio: item.iv_hv_ratio,
        buy_to_bep: item.buy_to_bep,
        sell_to_bep: item.sell_to_bep,
        buy_profit_probability: item.buy_profit_probability,
        sell_profit_probability: item.sell_profit_probability,
        intrinsic_value_per: item.intrinsic_value_per,
        time_value_per: item.time_value_per,
        itm_degree: item.itm_degree,
        otm_degree: item.otm_degree,
        otm_probability: item.otm_probability,
        sell_annualized_return: item.sell_annualized_return,
        interval_return: item.interval_return,
    };
    validate_item(&result)?;
    Ok(result)
}

fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionScreenSecurity, OptionScreenQueryError> {
    let market = match security.market {
        1 => "HK",
        11 => "US",
        _ => {
            return Err(OptionScreenQueryError::InvalidResponse(
                "option screen security market is unsupported".to_owned(),
            ));
        }
    };
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(OptionScreenQueryError::InvalidResponse(
            "option screen security code is empty or invalid".to_owned(),
        ));
    }
    Ok(OptionScreenSecurity {
        market: market.to_owned(),
        code: code.clone(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    })
}

fn map_underlying_info(
    info: crate::trade_proto::qot_option_screen::UnderlyingInfo,
) -> Result<OptionScreenUnderlyingInfo, OptionScreenQueryError> {
    let result = OptionScreenUnderlyingInfo {
        stock_id: info.stock_id,
        iv: info.iv,
        hv: info.hv,
        iv_rank: info.iv_rank,
        iv_percentile: info.iv_percentile,
        market_cap: info.market_cap,
        price: info.price,
        change_rate: info.change_rate,
    };
    validate_floats(
        "underlyingInfo",
        [
            ("iv", result.iv),
            ("hv", result.hv),
            ("ivRank", result.iv_rank),
            ("ivPercentile", result.iv_percentile),
            ("marketCap", result.market_cap),
            ("price", result.price),
            ("changeRate", result.change_rate),
        ],
    )?;
    Ok(result)
}

fn validate_item(item: &OptionScreenItem) -> Result<(), OptionScreenQueryError> {
    if let Some(date) = item.strike_date {
        let date_string = date.to_string();
        if date_string.len() != 8 {
            return Err(OptionScreenQueryError::InvalidResponse(
                "option screen strikeDate must be YYYYMMDD".to_owned(),
            ));
        }
        let year = date_string[0..4].parse::<i32>().unwrap_or_default();
        let month = date_string[4..6].parse::<u8>().unwrap_or_default();
        let day = date_string[6..8].parse::<u8>().unwrap_or_default();
        let month = Month::try_from(month).map_err(|_| {
            OptionScreenQueryError::InvalidResponse(
                "option screen strikeDate has invalid month".to_owned(),
            )
        })?;
        Date::from_calendar_date(year, month, day).map_err(|_| {
            OptionScreenQueryError::InvalidResponse(
                "option screen strikeDate has invalid day".to_owned(),
            )
        })?;
    }
    if let Some(option_type) = item.option_type
        && !matches!(option_type, 1 | 2)
    {
        return Err(OptionScreenQueryError::InvalidResponse(
            "option screen optionType is unsupported".to_owned(),
        ));
    }
    if let Some(left_day) = item.left_day
        && left_day < 0
    {
        return Err(OptionScreenQueryError::InvalidResponse(
            "option screen leftDay must be non-negative".to_owned(),
        ));
    }
    validate_floats(
        "optionScreenItem",
        [
            ("strikePrice", item.strike_price),
            ("price", item.price),
            ("midPrice", item.mid_price),
            ("bidPrice", item.bid_price),
            ("askPrice", item.ask_price),
            ("bidAskSpread", item.bid_ask_spread),
            ("changeRate", item.change_rate),
            ("turnover", item.turnover),
            ("bidAskVolumeRatio", item.bid_ask_volume_ratio),
            ("openInterestMarketCap", item.open_interest_market_cap),
            ("volOIRatio", item.vol_oi_ratio),
            ("premium", item.premium),
            ("impliedVolatility", item.implied_volatility),
            ("delta", item.delta),
            ("gamma", item.gamma),
            ("vega", item.vega),
            ("theta", item.theta),
            ("rho", item.rho),
            ("leverageRatio", item.leverage_ratio),
            ("effectiveGearing", item.effective_gearing),
            ("itmProbability", item.itm_probability),
            ("historyVolatility", item.history_volatility),
            ("ivHvRatio", item.iv_hv_ratio),
            ("buyToBep", item.buy_to_bep),
            ("sellToBep", item.sell_to_bep),
            ("buyProfitProbability", item.buy_profit_probability),
            ("sellProfitProbability", item.sell_profit_probability),
            ("intrinsicValuePer", item.intrinsic_value_per),
            ("timeValuePer", item.time_value_per),
            ("itmDegree", item.itm_degree),
            ("otmDegree", item.otm_degree),
            ("otmProbability", item.otm_probability),
            ("sellAnnualizedReturn", item.sell_annualized_return),
            ("intervalReturn", item.interval_return),
        ],
    )
}

fn validate_floats<const N: usize>(
    context: &str,
    values: [(&str, Option<f64>); N],
) -> Result<(), OptionScreenQueryError> {
    for (name, value) in values {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionScreenQueryError::InvalidResponse(format!(
                "{context} {name} must be finite"
            )));
        }
    }
    Ok(())
}

#[derive(Debug, Error)]
pub enum OptionScreenQueryError {
    #[error("invalid OpenD option screen query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option screen session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_OptionScreen response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_OptionScreen retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_OptionScreen response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option screen response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_option_screen::{OptionScreenItem as WireItem, Response, S2c};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionScreenQuery {
        OptionScreenQuery {
            market_categories: vec![0],
            page_from: Some(0),
            page_count: Some(25),
            option_retrieve_list: vec![2002, 3004],
            underlying_retrieve_list: vec![402],
        }
    }

    #[test]
    fn request_uses_strict_market_category_and_pagination_fields() {
        let request = crate::trade_proto::qot_option_screen::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        let c2s = request.c2s;
        assert_eq!(c2s.market_category_list, vec![0]);
        assert_eq!(c2s.page_from, Some(0));
        assert_eq!(c2s.page_count, Some(25));
        assert!(c2s.filter_list.is_empty());
    }

    #[test]
    fn framed_response_maps_security_and_optional_metrics() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                last_page: true,
                all_count: 1,
                data_list: vec![WireItem {
                    security: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL260918C00100000".to_owned(),
                    }),
                    option_name: Some("AAPL Call".to_owned()),
                    strike_price: Some(10.0),
                    strike_date: Some(20260918),
                    option_type: Some(1),
                    price: Some(1.25),
                    delta: Some(0.5),
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(crate::trade_proto::qot_option_screen::PROTOCOL_ID, 9, &body)
            .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3253);
        let page = decode_response(&decoded.body).expect("page");
        assert!(page.last_page);
        assert_eq!(page.all_count, 1);
        assert_eq!(
            page.items[0].security.instrument_id,
            "US.AAPL260918C00100000"
        );
        assert_eq!(page.items[0].delta, Some(0.5));
    }

    #[test]
    fn rejects_missing_security_non_finite_and_bad_pagination() {
        let missing_security = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                last_page: true,
                all_count: 1,
                data_list: vec![WireItem::default()],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&missing_security),
            Err(OptionScreenQueryError::InvalidResponse(_))
        ));

        let non_finite = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                last_page: true,
                all_count: 1,
                data_list: vec![WireItem {
                    security: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL260918C00100000".to_owned(),
                    }),
                    price: Some(f64::NAN),
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&non_finite),
            Err(OptionScreenQueryError::InvalidResponse(_))
        ));

        let invalid = OptionScreenQuery {
            market_categories: vec![7],
            ..Default::default()
        };
        assert!(matches!(
            validate_query(&invalid),
            Err(OptionScreenQueryError::InvalidQuery(_))
        ));
    }
}
