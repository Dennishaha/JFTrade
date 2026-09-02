//! Typed OpenD option-quote reader (Qot_GetOptionQuote/3255).
//!
//! The quote protocol accepts a list of combo legs.  This bounded adapter
//! intentionally exposes only one concrete option contract as a broker-
//! neutral quote, keeping generated protobuf types inside the Futu boundary.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::{Date, Month};

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionQuoteQuery {
    pub market: i32,
    pub code: String,
}

impl OptionQuoteQuery {
    pub fn validate(&self) -> Result<(), OptionQuoteQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionQuoteSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

/// Broker-neutral option quote. OpenD marks every quote field optional, so
/// absent fields remain None and are omitted from the JSON projection.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionQuote {
    pub security: OptionQuoteSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chg: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chg_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vol: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub high: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub low: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mid: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pre_close: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest: Option<i32>,
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
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expire_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub contract_size: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub contract_multiplier: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exercise_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub days_to_expiry: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub net_open_interest: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub contract_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub equal_underlying: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub index_option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub intrinsic_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub time_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub breakeven_point: Option<Vec<f64>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dist_to_breakeven: Option<Vec<f64>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prob_of_profit: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seller_roi: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mark_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub leverage_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub effective_gearing: Option<f64>,
}

pub trait OptionQuoteReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &OptionQuoteQuery) -> Result<Vec<OptionQuote>, OptionQuoteQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionQuoteReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionQuoteReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionQuoteReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionQuoteReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionQuoteReadPort for OpenDOptionQuoteReader {
    fn query(&self, query: &OptionQuoteQuery) -> Result<Vec<OptionQuote>, OptionQuoteQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OptionQuoteQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_quote::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &OptionQuoteQuery) -> Result<(), OptionQuoteQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionQuoteQueryError::InvalidQuery(
            "option quote market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if !is_option_contract_code(code) {
        return Err(OptionQuoteQueryError::InvalidQuery(format!(
            "option quote code must be a concrete {market} option contract"
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
        if !bytes[index..index + 6].iter().all(u8::is_ascii_digit) {
            continue;
        }
        if !matches!(bytes[index + 6], b'C' | b'P') {
            continue;
        }
        if bytes[index + 7..].iter().all(u8::is_ascii_digit) {
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
    }
    false
}

fn encode_request(query: &OptionQuoteQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_quote::{C2s, Request};
    Request {
        c2s: C2s {
            multi_legs: vec![crate::trade_proto::qot_common::ComboLeg {
                security: crate::trade_proto::qot_common::Security {
                    market: query.market,
                    code: query.code.trim().to_ascii_uppercase(),
                },
                side: Some(1),
                qty_ratio: Some(1.0),
                ..Default::default()
            }],
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionQuoteQuery,
) -> Result<Vec<OptionQuote>, OptionQuoteQueryError> {
    use crate::trade_proto::qot_get_option_quote::Response;
    let response = Response::decode(body).map_err(OptionQuoteQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionQuoteQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option quote request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionQuoteQueryError::MissingS2c);
    };
    if s2c.option_quote_list.len() > 1 {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote response contains more than one quote for a single leg".to_owned(),
        ));
    }
    s2c.option_quote_list
        .into_iter()
        .map(|quote| map_quote(quote, query))
        .collect()
}

fn map_quote(
    quote: crate::trade_proto::qot_get_option_quote::OptionQuote,
    query: &OptionQuoteQuery,
) -> Result<OptionQuote, OptionQuoteQueryError> {
    let expire_time = quote
        .expire_time
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    if let Some(expire_time) = expire_time.as_deref()
        && !is_date(expire_time)
    {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote expireTime must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(option_type) = quote.option_type
        && !matches!(option_type, 1 | 2)
    {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote optionType is unsupported".to_owned(),
        ));
    }
    if let Some(exercise_type) = quote.exercise_type
        && !matches!(exercise_type, 1..=3)
    {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote exerciseType is unsupported".to_owned(),
        ));
    }
    if let Some(index_option_type) = quote.index_option_type
        && !matches!(index_option_type, 1 | 2)
    {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote indexOptionType is unsupported".to_owned(),
        ));
    }
    if let Some(volume) = quote.vol
        && volume < 0
    {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote volume must be non-negative".to_owned(),
        ));
    }
    validate_floats(&quote)?;
    let market = market_label(query.market).expect("query validation ensures market");
    let code = query.code.trim().to_ascii_uppercase();
    Ok(OptionQuote {
        security: OptionQuoteSecurity {
            market: market.to_owned(),
            code: code.clone(),
            quote_market: market.to_owned(),
            trade_market: market.to_owned(),
            instrument_id: format!("{market}.{code}"),
        },
        price: quote.price,
        chg: quote.chg,
        chg_rate: quote.chg_rate,
        vol: quote.vol,
        turnover: quote.turnover,
        high: quote.high,
        low: quote.low,
        mid: quote.mid,
        open: quote.open,
        pre_close: quote.pre_close,
        open_interest: quote.open_interest,
        premium: quote.premium,
        implied_volatility: quote.iv,
        delta: quote.delta,
        gamma: quote.gamma,
        vega: quote.vega,
        theta: quote.theta,
        rho: quote.rho,
        option_type: quote.option_type,
        expire_time,
        strike: quote.strike,
        contract_size: quote.contract_size,
        contract_multiplier: quote.contract_multiplier,
        exercise_type: quote.exercise_type,
        days_to_expiry: quote.days_to_expiry,
        net_open_interest: quote.net_open_interest,
        contract_value: quote.contract_value,
        equal_underlying: quote.equal_underlying,
        index_option_type: quote.index_option_type,
        intrinsic_value: quote.intrinsic_value,
        time_value: quote.time_value,
        breakeven_point: (!quote.breakeven_point.is_empty()).then_some(quote.breakeven_point),
        dist_to_breakeven: (!quote.dist_to_breakeven.is_empty()).then_some(quote.dist_to_breakeven),
        prob_of_profit: quote.prob_of_profit,
        seller_roi: quote.seller_roi,
        mark_price: quote.mark_price,
        leverage_ratio: quote.leverage_ratio,
        effective_gearing: quote.effective_gearing,
    })
}

fn validate_floats(
    quote: &crate::trade_proto::qot_get_option_quote::OptionQuote,
) -> Result<(), OptionQuoteQueryError> {
    macro_rules! finite {
        ($($field:ident),+ $(,)?) => {
            $(if let Some(value) = quote.$field && !value.is_finite() {
                return Err(OptionQuoteQueryError::InvalidResponse(format!(
                    "option quote {} must be finite",
                    stringify!($field)
                )));
            })+
        };
    }
    finite!(
        price,
        chg,
        chg_rate,
        turnover,
        high,
        low,
        mid,
        open,
        pre_close,
        premium,
        iv,
        delta,
        gamma,
        vega,
        theta,
        rho,
        strike,
        contract_size,
        contract_multiplier,
        contract_value,
        equal_underlying,
        intrinsic_value,
        time_value,
        prob_of_profit,
        seller_roi,
        mark_price,
        leverage_ratio,
        effective_gearing
    );
    for (name, values) in [
        ("breakevenPoint", quote.breakeven_point.as_slice()),
        ("distToBreakeven", quote.dist_to_breakeven.as_slice()),
    ] {
        if values.iter().any(|value| !value.is_finite()) {
            return Err(OptionQuoteQueryError::InvalidResponse(format!(
                "option quote {name} must be finite"
            )));
        }
    }
    if quote.breakeven_point.len() != quote.dist_to_breakeven.len() {
        return Err(OptionQuoteQueryError::InvalidResponse(
            "option quote breakevenPoint and distToBreakeven lengths disagree".to_owned(),
        ));
    }
    Ok(())
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
pub enum OptionQuoteQueryError {
    #[error("invalid OpenD option quote query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option quote session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionQuote response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionQuote retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionQuote response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option quote response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_quote::{OptionQuote as WireQuote, Response, S2c};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionQuoteQuery {
        OptionQuoteQuery {
            market: 11,
            code: " aapl260918c00100000 ".to_owned(),
        }
    }

    #[test]
    fn request_uses_single_option_combo_leg_defaults() {
        let request = crate::trade_proto::qot_get_option_quote::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        let leg = &request.c2s.multi_legs[0];
        let security = &leg.security;
        assert_eq!(security.market, 11);
        assert_eq!(security.code, "AAPL260918C00100000");
        assert_eq!(leg.side, Some(1));
        assert_eq!(leg.qty_ratio, Some(1.0));
    }

    #[test]
    fn framed_response_preserves_optional_metrics_and_security() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_quote_list: vec![WireQuote {
                    price: Some(1.25),
                    iv: Some(0.2),
                    option_type: Some(1),
                    expire_time: Some("2026-09-18".to_owned()),
                    breakeven_point: vec![100.0],
                    dist_to_breakeven: vec![1.0],
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_quote::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3255);
        let quotes = decode_response(&decoded.body, &query()).expect("quotes");
        assert_eq!(quotes.len(), 1);
        assert_eq!(quotes[0].security.instrument_id, "US.AAPL260918C00100000");
        assert_eq!(quotes[0].price, Some(1.25));
        assert_eq!(quotes[0].implied_volatility, Some(0.2));
        assert_eq!(quotes[0].breakeven_point, Some(vec![100.0]));
    }

    #[test]
    fn rejects_bad_query_missing_s2c_and_non_finite_values() {
        assert!(matches!(
            validate_query(&OptionQuoteQuery {
                market: 11,
                code: "AAPL".to_owned()
            }),
            Err(OptionQuoteQueryError::InvalidQuery(_))
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
            Err(OptionQuoteQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_quote_list: vec![WireQuote {
                    price: Some(f64::NAN),
                    ..Default::default()
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionQuoteQueryError::InvalidResponse(_))
        ));
        let rejected = Response {
            ret_type: 3,
            ret_msg: Some("permission denied".to_owned()),
            err_code: Some(401),
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&rejected, &query()),
            Err(OptionQuoteQueryError::Rejected {
                ret_type: 3,
                err_code: 401,
                ..
            })
        ));
    }
}
