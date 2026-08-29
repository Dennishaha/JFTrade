//! Typed OpenD seller screener reader (Qot_GetOptionSellerScreener/3314).
use crate::{
    EventIndicator, EventIndicatorValue, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    OptionEventSecurity,
};
use prost::Message;
use serde::Serialize;
use std::sync::{Arc, Mutex};
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub struct OptionSellerScreenerQuery {
    pub option_market: i32,
    pub seller_type: i32,
    pub sort_type: Option<i32>,
    pub is_asc: Option<bool>,
    pub filters: Vec<EventIndicator>,
}
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionSellerScreenerItem {
    pub option: OptionEventSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub strike_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub left_days: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stock_price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub premium: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub otm_degree: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub interval_return: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub annualized_return: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub itm_probability: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub striked_interval_return: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub striked_annualized_return: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub owner: Option<OptionEventSecurity>,
}
pub trait OptionSellerScreenerReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionSellerScreenerQuery,
    ) -> Result<Vec<OptionSellerScreenerItem>, OptionSellerScreenerQueryError>;
}
#[derive(Clone)]
pub struct OpenDOptionSellerScreenerReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}
impl std::fmt::Debug for OpenDOptionSellerScreenerReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDOptionSellerScreenerReader")
            .finish_non_exhaustive()
    }
}
impl OpenDOptionSellerScreenerReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}
impl OptionSellerScreenerReadPort for OpenDOptionSellerScreenerReader {
    fn query(
        &self,
        query: &OptionSellerScreenerQuery,
    ) -> Result<Vec<OptionSellerScreenerItem>, OptionSellerScreenerQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionSellerScreenerQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_seller_screener::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}
fn validate_query(query: &OptionSellerScreenerQuery) -> Result<(), OptionSellerScreenerQueryError> {
    if !matches!(query.option_market, 1 | 3) {
        return Err(OptionSellerScreenerQueryError::InvalidQuery(
            "seller optionMarket must be US (1) or HK (3) security".into(),
        ));
    }
    if !matches!(query.seller_type, 1 | 2) {
        return Err(OptionSellerScreenerQueryError::InvalidQuery(
            "seller sellerType must be covered call (1) or cash secured put (2)".into(),
        ));
    }
    if let Some(sort) = query.sort_type {
        if !(1..=4).contains(&sort) {
            return Err(OptionSellerScreenerQueryError::InvalidQuery(
                "seller sortType is unsupported".into(),
            ));
        }
    }
    for filter in &query.filters {
        if !(1..=26).contains(&filter.indicator_type) {
            return Err(OptionSellerScreenerQueryError::InvalidQuery(
                "seller indicatorType is unsupported".into(),
            ));
        }
    }
    Ok(())
}
fn encode_request(
    query: &OptionSellerScreenerQuery,
) -> Result<Vec<u8>, OptionSellerScreenerQueryError> {
    use crate::trade_proto::qot_get_option_seller_screener::{C2s, Request};
    let filters = query
        .filters
        .iter()
        .map(encode_indicator)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(Request {
        c2s: C2s {
            option_market: query.option_market,
            seller_type: query.seller_type,
            sort_type: query.sort_type,
            is_asc: query.is_asc,
            filter_list: filters,
        },
    }
    .encode_to_vec())
}
fn encode_indicator(
    indicator: &EventIndicator,
) -> Result<crate::trade_proto::qot_option_common::SellerIndicator, OptionSellerScreenerQueryError>
{
    Ok(crate::trade_proto::qot_option_common::SellerIndicator {
        indicator_type: indicator.indicator_type,
        indicator_value: indicator.value.as_ref().map(encode_value).transpose()?,
    })
}
fn encode_value(
    value: &EventIndicatorValue,
) -> Result<crate::trade_proto::qot_option_common::IndicatorValue, OptionSellerScreenerQueryError> {
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
) -> Result<crate::trade_proto::qot_common::Security, OptionSellerScreenerQueryError> {
    let market = match security.market.to_ascii_uppercase().as_str() {
        "US" => 11,
        "HK" => 1,
        _ => {
            return Err(OptionSellerScreenerQueryError::InvalidQuery(
                "seller security market must be US or HK".into(),
            ));
        }
    };
    Ok(crate::trade_proto::qot_common::Security {
        market,
        code: security.code.trim().to_ascii_uppercase(),
    })
}
fn decode_response(
    body: &[u8],
) -> Result<Vec<OptionSellerScreenerItem>, OptionSellerScreenerQueryError> {
    use crate::trade_proto::qot_get_option_seller_screener::Response;
    let response = Response::decode(body).map_err(OptionSellerScreenerQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionSellerScreenerQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD seller screener request failed".into()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionSellerScreenerQueryError::MissingS2c);
    };
    s2c.item_list.into_iter().map(map_item).collect()
}
fn map_item(
    item: crate::trade_proto::qot_get_option_seller_screener::SellerItem,
) -> Result<OptionSellerScreenerItem, OptionSellerScreenerQueryError> {
    Ok(OptionSellerScreenerItem {
        option: map_security(item.option)?,
        name: clean(item.name),
        option_type: item.option_type,
        strike_price: finite(item.strike_price, "strikePrice")?,
        strike_time: clean(item.strike_time),
        strike_timestamp: finite(item.strike_timestamp, "strikeTimestamp")?,
        left_days: non_negative_i32(item.left_days, "leftDays")?,
        option_price: finite(item.option_price, "optionPrice")?,
        stock_price: finite(item.stock_price, "stockPrice")?,
        premium: finite(item.premium, "premium")?,
        otm_degree: finite(item.otm_degree, "otmDegree")?,
        iv: finite(item.iv, "iv")?,
        interval_return: finite(item.interval_return, "intervalReturn")?,
        annualized_return: finite(item.annualized_return, "annualizedReturn")?,
        itm_probability: finite(item.itm_probability, "itmProbability")?,
        striked_interval_return: finite(item.striked_interval_return, "strikedIntervalReturn")?,
        striked_annualized_return: finite(
            item.striked_annualized_return,
            "strikedAnnualizedReturn",
        )?,
        owner: item.owner.map(map_security).transpose()?,
    })
}
fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionEventSecurity, OptionSellerScreenerQueryError> {
    let (market, wire) = match security.market {
        11 => ("US", "US"),
        1 => ("HK", "HK"),
        _ => {
            return Err(OptionSellerScreenerQueryError::InvalidResponse(
                "seller security market is unsupported".into(),
            ));
        }
    };
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(OptionSellerScreenerQueryError::InvalidResponse(
            "seller security code is empty".into(),
        ));
    }
    Ok(OptionEventSecurity {
        market: market.into(),
        code: code.clone(),
        quote_market: wire.into(),
        trade_market: wire.into(),
        instrument_id: format!("{market}.{code}"),
    })
}
fn clean(value: Option<String>) -> Option<String> {
    value.and_then(|value| (!value.trim().is_empty()).then(|| value.trim().to_owned()))
}
fn finite(value: Option<f64>, field: &str) -> Result<Option<f64>, OptionSellerScreenerQueryError> {
    if value.is_some_and(|v| !v.is_finite()) {
        return Err(OptionSellerScreenerQueryError::InvalidResponse(format!(
            "seller {field} must be finite"
        )));
    }
    Ok(value)
}
fn non_negative_i32(
    value: Option<i32>,
    field: &str,
) -> Result<Option<i32>, OptionSellerScreenerQueryError> {
    if value.is_some_and(|v| v < 0) {
        return Err(OptionSellerScreenerQueryError::InvalidResponse(format!(
            "seller {field} must be non-negative"
        )));
    }
    Ok(value)
}
#[derive(Debug, Error)]
pub enum OptionSellerScreenerQueryError {
    #[error("invalid seller screener query: {0}")]
    InvalidQuery(String),
    #[error("decode OpenD seller screener response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD seller screener response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD seller screener response: {0}")]
    InvalidResponse(String),
    #[error("OpenD session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}
