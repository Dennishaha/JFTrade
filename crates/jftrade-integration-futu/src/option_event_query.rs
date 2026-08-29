//! Typed OpenD option-event reader (Qot_GetOptionEvent/3307).
//!
//! The public options-events route currently exposes the unusual-events
//! operation only.  This adapter keeps the generated protobuf messages behind
//! the Futu boundary while retaining the optional fields OpenD may omit.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq)]
pub struct OptionEventQuery {
    /// OpenD option market: 1/2 are US security/index, 3/4 are HK
    /// security/index.
    pub market: i32,
    /// The underlying product class selects security versus index option
    /// market. `None` preserves the Go adapter's equity default.
    pub underlying_product_class: Option<i32>,
    pub owner: Option<OptionEventSecurity>,
    pub count: i32,
    pub page: Option<String>,
    pub filters: Vec<EventIndicator>,
    pub sort: Option<EventSort>,
}

impl OptionEventQuery {
    pub fn validate(&self) -> Result<(), OptionEventQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEventSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EventBoundary {
    pub value: f64,
    pub includes: bool,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EventInterval {
    pub filter_min: Option<EventBoundary>,
    pub filter_max: Option<EventBoundary>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EventIndicatorValue {
    pub value_list: Vec<i64>,
    pub value_interval: Option<EventInterval>,
    pub string_value_list: Vec<String>,
    pub security_list: Vec<OptionEventSecurity>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EventIndicator {
    pub indicator_type: i32,
    pub value: Option<EventIndicatorValue>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct EventSort {
    pub indicator_type: i32,
    pub is_asc: bool,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEventCorporateAction {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action_timestamp: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEvent {
    pub option: OptionEventSecurity,
    pub owner: OptionEventSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub symbol: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fill_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fill_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ticker_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub turnover: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dte: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub underlying_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub otm: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bid_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ask_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub vo_ratio: Option<f64>,
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
    pub sentiment: Option<i32>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub order_type_list: Vec<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strategy_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_pub_type: Option<i32>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub corporate_action_list: Vec<OptionEventCorporateAction>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub industry_plate_list: Vec<OptionEventSecurity>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub concept_plate_list: Vec<OptionEventSecurity>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEventPage {
    pub events: Vec<OptionEvent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub update_timestamp: Option<f64>,
}

pub trait OptionEventReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &OptionEventQuery) -> Result<OptionEventPage, OptionEventQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionEventReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionEventReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionEventReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionEventReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionEventReadPort for OpenDOptionEventReader {
    fn query(&self, query: &OptionEventQuery) -> Result<OptionEventPage, OptionEventQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OptionEventQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_event::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(query: &OptionEventQuery) -> Result<(), OptionEventQueryError> {
    if !(1..=4).contains(&query.market) {
        return Err(OptionEventQueryError::InvalidQuery(
            "option event market must be US/HK security or index (1..4)".to_owned(),
        ));
    }
    if let Some(product_class) = query.underlying_product_class
        && !matches!(product_class, 1 | 2)
    {
        return Err(OptionEventQueryError::InvalidQuery(
            "underlying product class must be equity (1) or index (2)".to_owned(),
        ));
    }
    if !(1..=300).contains(&query.count) {
        return Err(OptionEventQueryError::InvalidQuery(
            "option event count must be between 1 and 300".to_owned(),
        ));
    }
    if let Some(page) = query.page.as_deref()
        && (page.len() > 1024 || page.chars().any(char::is_control))
    {
        return Err(OptionEventQueryError::InvalidQuery(
            "option event page token is invalid".to_owned(),
        ));
    }
    if let Some(owner) = query.owner.as_ref() {
        validate_query_security(owner, "option event owner")?;
    }
    for indicator in &query.filters {
        validate_indicator(indicator)?;
    }
    if let Some(sort) = query.sort
        && !is_indicator_type(sort.indicator_type)
    {
        return Err(OptionEventQueryError::InvalidQuery(
            "option event sort indicator is unsupported".to_owned(),
        ));
    }
    Ok(())
}

fn validate_indicator(indicator: &EventIndicator) -> Result<(), OptionEventQueryError> {
    if !is_indicator_type(indicator.indicator_type) {
        return Err(OptionEventQueryError::InvalidQuery(format!(
            "option event indicator {} is unsupported",
            indicator.indicator_type
        )));
    }
    if let Some(value) = indicator.value.as_ref() {
        if value.value_list.is_empty()
            && value.value_interval.is_none()
            && value.string_value_list.is_empty()
            && value.security_list.is_empty()
        {
            return Err(OptionEventQueryError::InvalidQuery(
                "option event indicator value is empty".to_owned(),
            ));
        }
        if let Some(interval) = value.value_interval.as_ref() {
            for boundary in [interval.filter_min.as_ref(), interval.filter_max.as_ref()]
                .into_iter()
                .flatten()
            {
                if !boundary.value.is_finite() {
                    return Err(OptionEventQueryError::InvalidQuery(
                        "option event indicator boundary must be finite".to_owned(),
                    ));
                }
            }
            if let (Some(min), Some(max)) =
                (interval.filter_min.as_ref(), interval.filter_max.as_ref())
                && min.value > max.value
            {
                return Err(OptionEventQueryError::InvalidQuery(
                    "option event indicator minimum must not exceed maximum".to_owned(),
                ));
            }
        }
        for security in &value.security_list {
            validate_query_security(security, "option event indicator security")?;
        }
    }
    Ok(())
}

fn is_indicator_type(value: i32) -> bool {
    matches!(
        value,
        101..=105
            | 201..=205
            | 301..=306
            | 401..=403
            | 501..=504
            | 601..=605
    )
}

fn encode_request(query: &OptionEventQuery) -> Result<Vec<u8>, OptionEventQueryError> {
    use crate::trade_proto::qot_get_option_event::{C2s, EventIndicator, EventSort, Request};
    let mut filters = query
        .filters
        .iter()
        .map(encode_indicator)
        .collect::<Result<Vec<_>, _>>()?;
    if let Some(owner) = query.owner.as_ref() {
        filters.push(EventIndicator {
            indicator_type: 101,
            indicator_value: Some(encode_indicator_value(&EventIndicatorValue {
                value_list: Vec::new(),
                value_interval: None,
                string_value_list: Vec::new(),
                security_list: vec![owner.clone()],
            })?),
        });
    }
    Ok(Request {
        c2s: C2s {
            option_market: query.market,
            count: Some(query.count),
            page: query
                .page
                .as_deref()
                .map(str::trim)
                .filter(|page| !page.is_empty())
                .map(ToOwned::to_owned),
            filter_list: filters,
            sort: query.sort.map(|sort| EventSort {
                indicator_type: sort.indicator_type,
                is_asc: sort.is_asc,
            }),
        },
    }
    .encode_to_vec())
}

fn encode_indicator(
    indicator: &EventIndicator,
) -> Result<crate::trade_proto::qot_get_option_event::EventIndicator, OptionEventQueryError> {
    Ok(crate::trade_proto::qot_get_option_event::EventIndicator {
        indicator_type: indicator.indicator_type,
        indicator_value: indicator
            .value
            .as_ref()
            .map(encode_indicator_value)
            .transpose()?,
    })
}

fn encode_indicator_value(
    value: &EventIndicatorValue,
) -> Result<crate::trade_proto::qot_option_common::IndicatorValue, OptionEventQueryError> {
    Ok(crate::trade_proto::qot_option_common::IndicatorValue {
        value_list: value.value_list.clone(),
        value_interval: value.value_interval.as_ref().map(|interval| {
            crate::trade_proto::qot_option_common::Interval {
                filter_min: interval.filter_min.as_ref().map(|boundary| {
                    crate::trade_proto::qot_option_common::Boundary {
                        value: boundary.value,
                        includes: boundary.includes,
                    }
                }),
                filter_max: interval.filter_max.as_ref().map(|boundary| {
                    crate::trade_proto::qot_option_common::Boundary {
                        value: boundary.value,
                        includes: boundary.includes,
                    }
                }),
            }
        }),
        string_value_list: value.string_value_list.clone(),
        security_list: value
            .security_list
            .iter()
            .map(encode_security)
            .collect::<Result<Vec<_>, _>>()?,
    })
}

fn encode_security(
    security: &OptionEventSecurity,
) -> Result<crate::trade_proto::qot_common::Security, OptionEventQueryError> {
    let market = match security.market.trim().to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 11,
        _ => {
            return Err(OptionEventQueryError::InvalidQuery(
                "option event security market must be HK or US".to_owned(),
            ));
        }
    };
    Ok(crate::trade_proto::qot_common::Security {
        market,
        code: security.code.trim().to_ascii_uppercase(),
    })
}

fn decode_response(body: &[u8]) -> Result<OptionEventPage, OptionEventQueryError> {
    use crate::trade_proto::qot_get_option_event::Response;
    let response = Response::decode(body).map_err(OptionEventQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionEventQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option event request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionEventQueryError::MissingS2c);
    };
    if let Some(all_count) = s2c.all_count
        && (all_count < 0 || all_count < s2c.event_list.len() as i32)
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event allCount is inconsistent with eventList".to_owned(),
        ));
    }
    if let Some(timestamp) = s2c.update_timestamp
        && !timestamp.is_finite()
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event updateTimestamp must be finite".to_owned(),
        ));
    }
    let next_page = s2c.next_page.filter(|page| !page.trim().is_empty());
    let all_count = s2c.all_count;
    let update_timestamp = s2c.update_timestamp;
    let events = s2c
        .event_list
        .into_iter()
        .map(map_event)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionEventPage {
        events,
        next_page,
        all_count,
        update_timestamp,
    })
}

fn map_event(
    event: crate::trade_proto::qot_get_option_event::EventItem,
) -> Result<OptionEvent, OptionEventQueryError> {
    let option = event.option.clone().ok_or_else(|| {
        OptionEventQueryError::InvalidResponse("option event item missing option".to_owned())
    })?;
    let owner = event.owner.clone().ok_or_else(|| {
        OptionEventQueryError::InvalidResponse("option event item missing owner".to_owned())
    })?;
    let option = map_security(option)?;
    let owner = map_security(owner)?;
    if let Some(fill_time) = event.fill_time.as_deref()
        && !fill_time.trim().is_empty()
        && !is_datetime(fill_time.trim())
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event fillTime must be YYYY-MM-DD HH:MM:SS".to_owned(),
        ));
    }
    if let Some(strike_time) = event.strike_time.as_deref()
        && !strike_time.trim().is_empty()
        && !is_date(strike_time.trim())
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event strikeTime must be YYYY-MM-DD".to_owned(),
        ));
    }
    if let Some(earnings_time) = event.earnings_time.as_deref()
        && !earnings_time.trim().is_empty()
        && !is_date(earnings_time.trim())
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event earningsTime must be YYYY-MM-DD".to_owned(),
        ));
    }
    for (name, value) in [
        ("fillTimestamp", event.fill_timestamp),
        ("price", event.price),
        ("turnover", event.turnover),
        ("strikePrice", event.strike_price),
        ("strikeTimestamp", event.strike_timestamp),
        ("underlyingPrice", event.underlying_price),
        ("otm", event.otm),
        ("bidPrice", event.bid_price),
        ("askPrice", event.ask_price),
        ("iv", event.iv),
        ("voRatio", event.vo_ratio),
        ("delta", event.delta),
        ("gamma", event.gamma),
        ("vega", event.vega),
        ("theta", event.theta),
        ("rho", event.rho),
    ] {
        if value.is_some_and(|value| !value.is_finite()) {
            return Err(OptionEventQueryError::InvalidResponse(format!(
                "option event {name} must be finite"
            )));
        }
    }
    for (name, value) in [
        ("volume", event.volume),
        ("totalVolume", event.total_volume),
        ("totalOpenInterest", event.total_open_interest),
    ] {
        if value.is_some_and(|value| value < 0) {
            return Err(OptionEventQueryError::InvalidResponse(format!(
                "option event {name} must be non-negative"
            )));
        }
    }
    validate_event_enums(&event)?;
    let corporate_action_list = event
        .corporate_action_list
        .into_iter()
        .map(map_corporate_action)
        .collect::<Result<Vec<_>, _>>()?;
    let industry_plate_list = event
        .industry_plate_list
        .into_iter()
        .map(map_security)
        .collect::<Result<Vec<_>, _>>()?;
    let concept_plate_list = event
        .concept_plate_list
        .into_iter()
        .map(map_security)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionEvent {
        option,
        owner,
        symbol: clean_string(event.symbol),
        fill_time: clean_string(event.fill_time),
        fill_timestamp: event.fill_timestamp,
        ticker_type: event.ticker_type,
        price: event.price,
        volume: event.volume,
        turnover: event.turnover,
        option_type: event.option_type,
        strike_price: event.strike_price,
        strike_time: clean_string(event.strike_time),
        strike_timestamp: event.strike_timestamp,
        dte: event.dte,
        underlying_price: event.underlying_price,
        otm: event.otm,
        bid_price: event.bid_price,
        ask_price: event.ask_price,
        iv: event.iv,
        total_volume: event.total_volume,
        total_open_interest: event.total_open_interest,
        vo_ratio: event.vo_ratio,
        delta: event.delta,
        gamma: event.gamma,
        vega: event.vega,
        theta: event.theta,
        rho: event.rho,
        sentiment: event.sentiment,
        order_type_list: event.order_type_list,
        strategy_type: event.strategy_type,
        earnings_time: clean_string(event.earnings_time),
        earnings_pub_type: event.earnings_pub_type,
        corporate_action_list,
        industry_plate_list,
        concept_plate_list,
    })
}

fn validate_event_enums(
    event: &crate::trade_proto::qot_get_option_event::EventItem,
) -> Result<(), OptionEventQueryError> {
    for (name, value) in [
        ("tickerType", event.ticker_type),
        ("optionType", event.option_type),
        ("sentiment", event.sentiment),
        ("strategyType", event.strategy_type),
        ("earningsPubType", event.earnings_pub_type),
    ] {
        if value.is_some_and(|value| !valid_enum(name, value)) {
            return Err(OptionEventQueryError::InvalidResponse(format!(
                "option event {name} is unsupported"
            )));
        }
    }
    for value in &event.order_type_list {
        if !matches!(value, 0 | 1 | 2 | 4) {
            return Err(OptionEventQueryError::InvalidResponse(
                "option event orderTypeList contains an unsupported value".to_owned(),
            ));
        }
    }
    Ok(())
}

fn valid_enum(name: &str, value: i32) -> bool {
    match name {
        "tickerType" | "sentiment" => (1..=3).contains(&value),
        "optionType" | "strategyType" | "earningsPubType" => matches!(value, 1 | 2),
        _ => false,
    }
}

fn map_corporate_action(
    action: crate::trade_proto::qot_get_option_event::CorporateAction,
) -> Result<OptionEventCorporateAction, OptionEventQueryError> {
    if let Some(action_type) = action.action_type
        && !(0..=9).contains(&action_type)
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event corporate action type is unsupported".to_owned(),
        ));
    }
    if let Some(action_time) = action.action_time.as_deref()
        && !action_time.trim().is_empty()
        && !is_date(action_time.trim())
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event actionTime must be YYYY-MM-DD".to_owned(),
        ));
    }
    if action
        .action_timestamp
        .is_some_and(|value| !value.is_finite())
    {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event actionTimestamp must be finite".to_owned(),
        ));
    }
    Ok(OptionEventCorporateAction {
        action_type: action.action_type,
        action_time: clean_string(action.action_time),
        action_timestamp: action.action_timestamp,
    })
}

fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionEventSecurity, OptionEventQueryError> {
    let market = market_label(security.market).ok_or_else(|| {
        OptionEventQueryError::InvalidResponse(
            "option event security market must be HK or US".to_owned(),
        )
    })?;
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(OptionEventQueryError::InvalidResponse(
            "option event security code is invalid".to_owned(),
        ));
    }
    Ok(OptionEventSecurity {
        market: market.to_owned(),
        code: code.clone(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    })
}

fn validate_query_security(
    security: &OptionEventSecurity,
    context: &str,
) -> Result<(), OptionEventQueryError> {
    if !matches!(
        security.market.trim().to_ascii_uppercase().as_str(),
        "HK" | "US"
    ) || security.code.trim().is_empty()
        || security.code.chars().any(char::is_whitespace)
    {
        return Err(OptionEventQueryError::InvalidQuery(format!(
            "{context} is invalid"
        )));
    }
    Ok(())
}

fn clean_string(value: Option<String>) -> Option<String> {
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

fn is_datetime(value: &str) -> bool {
    let Ok(format) = time::format_description::parse_borrowed::<2>(
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    ) else {
        return false;
    };
    time::PrimitiveDateTime::parse(value, &format).is_ok()
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

#[derive(Debug, Error)]
pub enum OptionEventQueryError {
    #[error("invalid OpenD option event query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option event session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionEvent response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionEvent retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionEvent response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option event response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_event::{EventItem, Response, S2c};
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionEventQuery {
        OptionEventQuery {
            market: 1,
            underlying_product_class: None,
            owner: Some(OptionEventSecurity {
                market: "US".to_owned(),
                code: "AAPL".to_owned(),
                quote_market: "US".to_owned(),
                trade_market: "US".to_owned(),
                instrument_id: "US.AAPL".to_owned(),
            }),
            count: 50,
            page: Some("next".to_owned()),
            filters: Vec::new(),
            sort: Some(EventSort {
                indicator_type: 305,
                is_asc: false,
            }),
        }
    }

    #[test]
    fn request_uses_option_market_pagination_owner_filter_and_sort() {
        let request = crate::trade_proto::qot_get_option_event::Request::decode(
            encode_request(&query()).expect("request").as_slice(),
        )
        .expect("decode request");
        assert_eq!(request.c2s.option_market, 1);
        assert_eq!(request.c2s.count, Some(50));
        assert_eq!(request.c2s.page.as_deref(), Some("next"));
        assert_eq!(request.c2s.filter_list.len(), 1);
        assert_eq!(request.c2s.filter_list[0].indicator_type, 101);
        let security = &request.c2s.filter_list[0]
            .indicator_value
            .as_ref()
            .expect("indicator value")
            .security_list[0];
        assert_eq!(security.market, 11);
        assert_eq!(security.code, "AAPL");
        assert_eq!(request.c2s.sort.as_ref().expect("sort").indicator_type, 305);
    }

    #[test]
    fn framed_response_maps_event_and_preserves_protocol_identity() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                event_list: vec![EventItem {
                    option: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL260918C00100000".to_owned(),
                    }),
                    owner: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL".to_owned(),
                    }),
                    fill_time: Some("2026-08-29 14:30:00".to_owned()),
                    fill_timestamp: Some(1_756_000_000.123),
                    option_type: Some(1),
                    price: Some(1.25),
                    volume: Some(100),
                    ..Default::default()
                }],
                next_page: Some("next".to_owned()),
                all_count: Some(1),
                update_timestamp: Some(1_756_000_001.0),
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_event::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        assert_eq!(decoded.header.proto_id, 3307);
        let page = decode_response(&decoded.body).expect("page");
        assert_eq!(page.events.len(), 1);
        assert_eq!(
            page.events[0].option.instrument_id,
            "US.AAPL260918C00100000"
        );
        assert_eq!(page.events[0].price, Some(1.25));
        assert_eq!(page.next_page.as_deref(), Some("next"));
    }

    #[test]
    fn rejects_invalid_query_missing_s2c_and_inconsistent_or_non_finite_response() {
        assert!(matches!(
            validate_query(&OptionEventQuery {
                market: 9,
                count: 1,
                ..query()
            }),
            Err(OptionEventQueryError::InvalidQuery(_))
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
            Err(OptionEventQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                event_list: vec![EventItem {
                    option: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL260918C00100000".to_owned(),
                    }),
                    owner: Some(crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL".to_owned(),
                    }),
                    price: Some(f64::NAN),
                    ..Default::default()
                }],
                all_count: Some(0),
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid),
            Err(OptionEventQueryError::InvalidResponse(_))
        ));
    }
}
