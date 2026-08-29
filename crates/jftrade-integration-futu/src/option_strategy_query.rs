//! Typed OpenD option-strategy reader (`Qot_GetOptionStrategy/3256`).
//!
//! The generated protobuf messages remain private to this crate.  This module
//! exposes broker-neutral strategy rows and validates the owner, strategy,
//! filter, and combination-leg boundaries before a request reaches OpenD.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const SUPPORTED_STRATEGIES: [i32; 13] = [1, 2, 4, 6, 7, 8, 9, 11, 13, 14, 15, 16, 100];
const DIAGONAL_SPREAD: i32 = 16;

#[derive(Clone, Debug, PartialEq)]
pub struct OptionStrategyQuery {
    /// Public quote market: HK (1) or US (11).
    pub market: i32,
    pub code: String,
    pub option_strategy: i32,
    pub expire_time: Option<String>,
    pub far_expire_time: Option<String>,
    pub spread: Option<f64>,
    pub option_type: Option<i32>,
    pub strike_price: Option<f64>,
    pub index_option_type: Option<i32>,
}

impl OptionStrategyQuery {
    pub fn validate(&self) -> Result<(), OptionStrategyQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategySecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

/// Broker-neutral representation of an OpenD `Qot_Common.ComboLeg`.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategyLeg {
    pub security: OptionStrategySecurity,
    pub side: Option<i32>,
    pub qty_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub position_id: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pred_side: Option<i32>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategyItem {
    pub code: String,
    pub name: String,
    pub option_strategy: i32,
    pub stock_owner: OptionStrategySecurity,
    pub multi_legs: Vec<OptionStrategyLeg>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionStrategySnapshot {
    pub items: Vec<OptionStrategyItem>,
}

pub trait OptionStrategyReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionStrategyQuery,
    ) -> Result<OptionStrategySnapshot, OptionStrategyQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionStrategyReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionStrategyReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionStrategyReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionStrategyReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionStrategyReadPort for OpenDOptionStrategyReader {
    fn query(
        &self,
        query: &OptionStrategyQuery,
    ) -> Result<OptionStrategySnapshot, OptionStrategyQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OptionStrategyQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_strategy::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(query: &OptionStrategyQuery) -> Result<(), OptionStrategyQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionStrategyQueryError::InvalidQuery(
            "option strategy market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    validate_code(&query.code).ok_or_else(|| {
        OptionStrategyQueryError::InvalidQuery(format!(
            "option strategy code must be a {market} underlying code"
        ))
    })?;
    if !SUPPORTED_STRATEGIES.contains(&query.option_strategy) {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "optionStrategy is not a supported OptionStrategyType".to_owned(),
        ));
    }
    let expire = query
        .expire_time
        .as_deref()
        .map(parse_date)
        .transpose()
        .map_err(|_| {
            OptionStrategyQueryError::InvalidQuery("expireTime must be YYYY-MM-DD".to_owned())
        })?;
    let far_expire = query
        .far_expire_time
        .as_deref()
        .map(parse_date)
        .transpose()
        .map_err(|_| {
            OptionStrategyQueryError::InvalidQuery("farExpireTime must be YYYY-MM-DD".to_owned())
        })?;
    if query.option_strategy == DIAGONAL_SPREAD && far_expire.is_none() {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "farExpireTime is required for diagonal spread".to_owned(),
        ));
    }
    if let (Some(expire), Some(far_expire)) = (expire, far_expire)
        && far_expire < expire
    {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "farExpireTime must not precede expireTime".to_owned(),
        ));
    }
    for (field, value) in [
        ("spread", query.spread),
        ("strikePrice", query.strike_price),
    ] {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(OptionStrategyQueryError::InvalidQuery(format!(
                "{field} must be finite"
            )));
        }
    }
    if query.spread.is_some_and(|value| value < 0.0) {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "spread must not be negative".to_owned(),
        ));
    }
    if query.strike_price.is_some_and(|value| value < 0.0) {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "strikePrice must not be negative".to_owned(),
        ));
    }
    for (field, value) in [
        ("optionType", query.option_type),
        ("indexOptionType", query.index_option_type),
    ] {
        if value.is_some_and(|value| !(0..=2).contains(&value)) {
            return Err(OptionStrategyQueryError::InvalidQuery(format!(
                "{field} must be 0, 1, or 2"
            )));
        }
    }
    Ok(())
}

fn encode_request(query: &OptionStrategyQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_strategy::{C2s, Request};
    Request {
        c2s: C2s {
            owner: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
            option_strategy: query.option_strategy,
            expire_time: query
                .expire_time
                .as_deref()
                .map(|value| value.trim().to_owned()),
            far_expire_time: query
                .far_expire_time
                .as_deref()
                .map(|value| value.trim().to_owned()),
            spread: query.spread,
            option_type: query.option_type,
            strike_price: query.strike_price,
            index_option_type: query.index_option_type,
            header: None,
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionStrategyQuery,
) -> Result<OptionStrategySnapshot, OptionStrategyQueryError> {
    use crate::trade_proto::qot_get_option_strategy::Response;
    let response = Response::decode(body).map_err(OptionStrategyQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionStrategyQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option strategy request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionStrategyQueryError::MissingS2c);
    };
    let owner_market = market_label(query.market).expect("query validation ensures market");
    let owner_code = validate_code(&query.code).expect("query validation ensures code");
    let items = s2c
        .strategy_list
        .into_iter()
        .map(|item| map_item(item, owner_market, &owner_code))
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionStrategySnapshot { items })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_strategy::OptionStrategyItem,
    expected_market: &str,
    expected_code: &str,
) -> Result<OptionStrategyItem, OptionStrategyQueryError> {
    let code = validate_strategy_code(&item.code).ok_or_else(|| {
        OptionStrategyQueryError::InvalidResponse(
            "option strategy response code must be non-empty and valid".to_owned(),
        )
    })?;
    let name = item.name.trim().to_owned();
    if name.is_empty() {
        return Err(OptionStrategyQueryError::InvalidResponse(
            "option strategy response name must not be empty".to_owned(),
        ));
    }
    if !SUPPORTED_STRATEGIES.contains(&item.option_strategy) {
        return Err(OptionStrategyQueryError::InvalidResponse(
            "option strategy response has an unsupported strategy type".to_owned(),
        ));
    }
    let owner =
        map_security(item.stock_owner).map_err(OptionStrategyQueryError::InvalidResponse)?;
    if owner.market != expected_market || owner.code != expected_code {
        return Err(OptionStrategyQueryError::InvalidResponse(
            "option strategy response stock owner does not match query".to_owned(),
        ));
    }
    if item.multi_legs.is_empty() {
        return Err(OptionStrategyQueryError::InvalidResponse(
            "option strategy response must contain at least one combo leg".to_owned(),
        ));
    }
    let multi_legs = item
        .multi_legs
        .into_iter()
        .map(map_leg)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionStrategyItem {
        code,
        name,
        option_strategy: item.option_strategy,
        stock_owner: owner,
        multi_legs,
    })
}

pub(crate) fn encode_combo_leg(
    leg: &OptionStrategyLeg,
) -> Result<crate::trade_proto::qot_common::ComboLeg, String> {
    validate_leg(leg).map_err(|error| error.to_string())?;
    let market = market_number(&leg.security.market)
        .ok_or_else(|| "option strategy combo leg market must be HK or US".to_owned())?;
    Ok(crate::trade_proto::qot_common::ComboLeg {
        security: crate::trade_proto::qot_common::Security {
            market,
            code: leg.security.code.trim().to_ascii_uppercase(),
        },
        side: leg.side,
        qty_ratio: leg.qty_ratio,
        position_id: leg.position_id,
        pred_side: leg.pred_side,
    })
}

fn map_leg(
    leg: crate::trade_proto::qot_common::ComboLeg,
) -> Result<OptionStrategyLeg, OptionStrategyQueryError> {
    validate_wire_leg(&leg).map_err(OptionStrategyQueryError::InvalidResponse)?;
    Ok(OptionStrategyLeg {
        security: map_security(leg.security).map_err(OptionStrategyQueryError::InvalidResponse)?,
        side: leg.side,
        qty_ratio: leg.qty_ratio,
        position_id: leg.position_id,
        pred_side: leg.pred_side,
    })
}

fn validate_wire_leg(leg: &crate::trade_proto::qot_common::ComboLeg) -> Result<(), String> {
    let _ = map_security(leg.security.clone()).map_err(|error| error.to_string())?;
    if !leg.side.is_some_and(|side| (1..=4).contains(&side)) {
        return Err("option strategy combo leg side must be 1, 2, 3, or 4".to_owned());
    }
    if !leg
        .qty_ratio
        .is_some_and(|ratio| ratio.is_finite() && ratio > 0.0)
    {
        return Err("option strategy combo leg qtyRatio must be finite and positive".to_owned());
    }
    Ok(())
}

fn validate_leg(leg: &OptionStrategyLeg) -> Result<(), OptionStrategyQueryError> {
    let _ = market_number(&leg.security.market).ok_or_else(|| {
        OptionStrategyQueryError::InvalidQuery(
            "option strategy combo leg market must be HK or US".to_owned(),
        )
    })?;
    if validate_code(&leg.security.code).is_none() {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "option strategy combo leg code is invalid".to_owned(),
        ));
    }
    if !leg.side.is_some_and(|side| (1..=4).contains(&side)) {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "option strategy combo leg side must be 1, 2, 3, or 4".to_owned(),
        ));
    }
    if !leg
        .qty_ratio
        .is_some_and(|ratio| ratio.is_finite() && ratio > 0.0)
    {
        return Err(OptionStrategyQueryError::InvalidQuery(
            "option strategy combo leg qtyRatio must be finite and positive".to_owned(),
        ));
    }
    Ok(())
}

fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionStrategySecurity, String> {
    let market = market_label(security.market)
        .ok_or_else(|| "option strategy security market must be HK or US".to_owned())?;
    let code = validate_code(&security.code)
        .ok_or_else(|| "option strategy security code is invalid".to_owned())?;
    Ok(security_from_wire(market, &code))
}

fn security_from_wire(market: &str, code: &str) -> OptionStrategySecurity {
    OptionStrategySecurity {
        market: market.to_owned(),
        code: code.to_owned(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    }
}

fn validate_code(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty()
        || value.chars().any(|character| {
            character.is_whitespace() || (!character.is_ascii_alphanumeric() && character != '-')
        })
    {
        return None;
    }
    Some(value.to_ascii_uppercase())
}

fn validate_strategy_code(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty()
        || value.chars().any(|character| {
            character.is_whitespace()
                || (!character.is_ascii_alphanumeric()
                    && !matches!(character, '-' | '/' | '_'))
        })
    {
        return None;
    }
    Some(value.to_ascii_uppercase())
}

fn parse_date(value: &str) -> Result<Date, ()> {
    let format =
        time::format_description::parse_borrowed::<2>("[year]-[month]-[day]").map_err(|_| ())?;
    Date::parse(value.trim(), &format).map_err(|_| ())
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

fn market_number(value: &str) -> Option<i32> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionStrategyQueryError {
    #[error("invalid OpenD option strategy query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option strategy session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionStrategy response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionStrategy retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionStrategy response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option strategy response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_strategy::{
        OptionStrategyItem as WireItem, Response, S2c,
    };

    fn query() -> OptionStrategyQuery {
        OptionStrategyQuery {
            market: 11,
            code: " aapl ".to_owned(),
            option_strategy: 4,
            expire_time: Some("2026-09-18".to_owned()),
            far_expire_time: None,
            spread: Some(5.0),
            option_type: Some(1),
            strike_price: Some(100.0),
            index_option_type: Some(1),
        }
    }

    fn wire_leg(code: &str) -> crate::trade_proto::qot_common::ComboLeg {
        crate::trade_proto::qot_common::ComboLeg {
            security: crate::trade_proto::qot_common::Security {
                market: 11,
                code: code.to_owned(),
            },
            side: Some(1),
            qty_ratio: Some(1.0),
            ..Default::default()
        }
    }

    #[test]
    fn request_uses_all_strategy_filters() {
        let request = crate::trade_proto::qot_get_option_strategy::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.owner.code, "AAPL");
        assert_eq!(request.c2s.option_strategy, 4);
        assert_eq!(request.c2s.expire_time.as_deref(), Some("2026-09-18"));
        assert_eq!(request.c2s.spread, Some(5.0));
        assert_eq!(request.c2s.option_type, Some(1));
        assert_eq!(request.c2s.strike_price, Some(100.0));
        assert_eq!(request.c2s.index_option_type, Some(1));
    }

    #[test]
    fn response_maps_owner_and_combo_legs() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                strategy_list: vec![WireItem {
                    code: "AAPL260918C/P100".to_owned(),
                    name: "AAPL vertical".to_owned(),
                    option_strategy: 4,
                    stock_owner: crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL".to_owned(),
                    },
                    multi_legs: vec![wire_leg("AAPL260918C100")],
                }],
            }),
        }
        .encode_to_vec();
        let snapshot = decode_response(&body, &query()).expect("snapshot");
        assert_eq!(snapshot.items[0].stock_owner.instrument_id, "US.AAPL");
        assert_eq!(snapshot.items[0].multi_legs[0].qty_ratio, Some(1.0));
    }

    #[test]
    fn rejects_invalid_filters_and_malformed_rows() {
        let mut invalid = query();
        invalid.option_strategy = 3;
        assert!(matches!(
            invalid.validate(),
            Err(OptionStrategyQueryError::InvalidQuery(_))
        ));
        invalid = query();
        invalid.strike_price = Some(f64::NAN);
        assert!(matches!(
            invalid.validate(),
            Err(OptionStrategyQueryError::InvalidQuery(_))
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
            Err(OptionStrategyQueryError::MissingS2c)
        ));
        let empty_legs = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                strategy_list: vec![WireItem {
                    code: "AAPL".to_owned(),
                    name: "vertical".to_owned(),
                    option_strategy: 4,
                    stock_owner: crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL".to_owned(),
                    },
                    multi_legs: Vec::new(),
                }],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&empty_legs, &query()),
            Err(OptionStrategyQueryError::InvalidResponse(_))
        ));
    }
}
