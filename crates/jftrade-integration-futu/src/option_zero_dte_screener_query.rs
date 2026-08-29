//! Typed OpenD 0DTE screener reader (Qot_GetOptionZeroDteScreener/3311).

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{
    EventIndicator, EventIndicatorValue, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    OptionEventSecurity,
};

#[derive(Clone, Debug, PartialEq)]
pub struct OptionZeroDteScreenerQuery {
    pub option_market: i32,
    pub sort_type: Option<i32>,
    pub is_asc: Option<bool>,
    pub count: i32,
    pub page: Option<String>,
    pub filters: Vec<EventIndicator>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionZeroDteChainInfo {
    pub strike_date_timestamp: Option<i64>,
    pub product_code: Option<String>,
    pub multiplier: Option<f64>,
    pub contract_share_size: Option<f64>,
    pub expiration_type: Option<i32>,
    pub underlying: Option<OptionEventSecurity>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionZeroDteScreenerItem {
    pub owner: OptionEventSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub change_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_rank: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_percentile: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub open_interest: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_trading_time: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_timestamp: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_pub_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chain_info: Option<OptionZeroDteChainInfo>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionZeroDteScreenerPage {
    pub items: Vec<OptionZeroDteScreenerItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub update_timestamp: Option<f64>,
}

pub trait OptionZeroDteScreenerReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionZeroDteScreenerQuery,
    ) -> Result<OptionZeroDteScreenerPage, OptionZeroDteScreenerQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionZeroDteScreenerReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionZeroDteScreenerReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDOptionZeroDteScreenerReader")
            .finish_non_exhaustive()
    }
}
impl OpenDOptionZeroDteScreenerReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionZeroDteScreenerReadPort for OpenDOptionZeroDteScreenerReader {
    fn query(
        &self,
        query: &OptionZeroDteScreenerQuery,
    ) -> Result<OptionZeroDteScreenerPage, OptionZeroDteScreenerQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionZeroDteScreenerQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_zero_dte_screener::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

fn validate_query(
    query: &OptionZeroDteScreenerQuery,
) -> Result<(), OptionZeroDteScreenerQueryError> {
    if !matches!(query.option_market, 1 | 2) {
        return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
            "0DTE optionMarket must be US security (1) or index (2)".into(),
        ));
    }
    if !(1..=500).contains(&query.count) {
        return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
            "0DTE count must be between 1 and 500".into(),
        ));
    }
    if let Some(page) = &query.page {
        if page.len() > 1024 || page.chars().any(char::is_control) {
            return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
                "0DTE page token is invalid".into(),
            ));
        }
    }
    if let Some(sort) = query.sort_type {
        if !(1..=5).contains(&sort) {
            return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
                "0DTE sortType is unsupported".into(),
            ));
        }
    }
    for filter in &query.filters {
        if filter.indicator_type == 0 || !(1..=10).contains(&filter.indicator_type) {
            return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
                "0DTE indicatorType is unsupported".into(),
            ));
        }
    }
    Ok(())
}

fn encode_request(
    query: &OptionZeroDteScreenerQuery,
) -> Result<Vec<u8>, OptionZeroDteScreenerQueryError> {
    use crate::trade_proto::qot_get_option_zero_dte_screener::{C2s, Request};
    let filters = query
        .filters
        .iter()
        .map(encode_indicator)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(Request {
        c2s: C2s {
            option_market: query.option_market,
            sort_type: query.sort_type,
            is_asc: query.is_asc,
            count: Some(query.count),
            page: query.page.clone(),
            filter_list: filters,
        },
    }
    .encode_to_vec())
}

fn encode_indicator(
    indicator: &EventIndicator,
) -> Result<crate::trade_proto::qot_option_common::ZeroDteIndicator, OptionZeroDteScreenerQueryError>
{
    Ok(crate::trade_proto::qot_option_common::ZeroDteIndicator {
        indicator_type: indicator.indicator_type,
        indicator_value: indicator.value.as_ref().map(encode_value).transpose()?,
    })
}
fn encode_value(
    value: &EventIndicatorValue,
) -> Result<crate::trade_proto::qot_option_common::IndicatorValue, OptionZeroDteScreenerQueryError>
{
    Ok(crate::trade_proto::qot_option_common::IndicatorValue {
        value_list: value.value_list.clone(),
        value_interval: None,
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
) -> Result<crate::trade_proto::qot_common::Security, OptionZeroDteScreenerQueryError> {
    let market = match security.market.to_ascii_uppercase().as_str() {
        "US" => 11,
        _ => {
            return Err(OptionZeroDteScreenerQueryError::InvalidQuery(
                "0DTE security market must be US".into(),
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
) -> Result<OptionZeroDteScreenerPage, OptionZeroDteScreenerQueryError> {
    use crate::trade_proto::qot_get_option_zero_dte_screener::Response;
    let response = Response::decode(body).map_err(OptionZeroDteScreenerQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionZeroDteScreenerQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD 0DTE screener request failed".into()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionZeroDteScreenerQueryError::MissingS2c);
    };
    let items = s2c
        .item_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(OptionZeroDteScreenerPage {
        items,
        next_page: s2c.next_page.filter(|p| !p.is_empty()),
        update_timestamp: s2c.update_timestamp,
    })
}
fn map_item(
    item: crate::trade_proto::qot_get_option_zero_dte_screener::ZeroDteScreenerItem,
) -> Result<OptionZeroDteScreenerItem, OptionZeroDteScreenerQueryError> {
    let owner = map_security(item.owner)?;
    let chain_info = item.chain_info.map(map_chain).transpose()?;
    Ok(OptionZeroDteScreenerItem {
        owner,
        name: clean(item.name),
        price: finite(item.price, "price")?,
        change_rate: finite(item.change_rate, "changeRate")?,
        market_cap: finite(item.market_cap, "marketCap")?,
        iv: finite(item.iv, "iv")?,
        iv_rank: finite(item.iv_rank, "ivRank")?,
        iv_percentile: finite(item.iv_percentile, "ivPercentile")?,
        hv: finite(item.hv, "hv")?,
        volume: non_negative(item.volume, "volume")?,
        open_interest: non_negative(item.open_interest, "openInterest")?,
        last_trading_time: item.last_trading_time,
        earnings_timestamp: item.earnings_timestamp,
        earnings_time: clean(item.earnings_time),
        earnings_pub_type: item.earnings_pub_type,
        chain_info,
    })
}
fn map_chain(
    chain: crate::trade_proto::qot_get_option_zero_dte_screener::OptionChainInfo,
) -> Result<OptionZeroDteChainInfo, OptionZeroDteScreenerQueryError> {
    Ok(OptionZeroDteChainInfo {
        strike_date_timestamp: chain.strike_date_timestamp,
        product_code: clean(chain.product_code),
        multiplier: finite(chain.multiplier, "multiplier")?,
        contract_share_size: finite(chain.contract_share_size, "contractShareSize")?,
        expiration_type: chain.expiration_type,
        underlying: chain.underlying.map(map_security).transpose()?,
    })
}
fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionEventSecurity, OptionZeroDteScreenerQueryError> {
    let (market, label) = match security.market {
        11 => ("US", "US"),
        _ => {
            return Err(OptionZeroDteScreenerQueryError::InvalidResponse(
                "0DTE security market is unsupported".into(),
            ));
        }
    };
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(OptionZeroDteScreenerQueryError::InvalidResponse(
            "0DTE security code is empty".into(),
        ));
    }
    Ok(OptionEventSecurity {
        market: market.into(),
        code: code.clone(),
        quote_market: label.into(),
        trade_market: label.into(),
        instrument_id: format!("{market}.{code}"),
    })
}
fn clean(value: Option<String>) -> Option<String> {
    value.and_then(|value| (!value.trim().is_empty()).then(|| value.trim().to_owned()))
}
fn finite(value: Option<f64>, field: &str) -> Result<Option<f64>, OptionZeroDteScreenerQueryError> {
    if value.is_some_and(|v| !v.is_finite()) {
        return Err(OptionZeroDteScreenerQueryError::InvalidResponse(format!(
            "0DTE {field} must be finite"
        )));
    }
    Ok(value)
}
fn non_negative(
    value: Option<i64>,
    field: &str,
) -> Result<Option<i64>, OptionZeroDteScreenerQueryError> {
    if value.is_some_and(|v| v < 0) {
        return Err(OptionZeroDteScreenerQueryError::InvalidResponse(format!(
            "0DTE {field} must be non-negative"
        )));
    }
    Ok(value)
}

#[derive(Debug, Error)]
pub enum OptionZeroDteScreenerQueryError {
    #[error("invalid 0DTE screener query: {0}")]
    InvalidQuery(String),
    #[error("decode OpenD 0DTE screener response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD 0DTE screener response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD 0DTE screener response: {0}")]
    InvalidResponse(String),
    #[error("OpenD session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_non_us_or_out_of_range_page_size() {
        let query = OptionZeroDteScreenerQuery {
            option_market: 3,
            sort_type: None,
            is_asc: None,
            count: 50,
            page: None,
            filters: Vec::new(),
        };
        assert!(matches!(
            validate_query(&query),
            Err(OptionZeroDteScreenerQueryError::InvalidQuery(_))
        ));
        let query = OptionZeroDteScreenerQuery {
            option_market: 1,
            sort_type: None,
            is_asc: None,
            count: 501,
            page: None,
            filters: Vec::new(),
        };
        assert!(validate_query(&query).is_err());
    }

    #[test]
    fn decodes_owner_and_chain_context() {
        use crate::trade_proto::qot_get_option_zero_dte_screener::{
            OptionChainInfo, Response, S2c, ZeroDteScreenerItem,
        };
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![ZeroDteScreenerItem {
                    owner: crate::trade_proto::qot_common::Security {
                        market: 11,
                        code: "AAPL".into(),
                    },
                    name: Some("Apple".into()),
                    price: Some(1.0),
                    change_rate: None,
                    market_cap: None,
                    iv: None,
                    iv_rank: None,
                    iv_percentile: None,
                    hv: None,
                    volume: Some(4),
                    open_interest: None,
                    last_trading_time: None,
                    earnings_timestamp: None,
                    earnings_time: None,
                    earnings_pub_type: None,
                    chain_info: Some(OptionChainInfo {
                        strike_date_timestamp: Some(1),
                        product_code: Some("AAPL".into()),
                        multiplier: Some(100.0),
                        contract_share_size: Some(100.0),
                        expiration_type: Some(2),
                        underlying: None,
                    }),
                }],
                next_page: Some("next".into()),
                update_timestamp: Some(2.0),
            }),
        };
        let page = decode_response(&body.encode_to_vec()).expect("decode");
        assert_eq!(page.items[0].owner.instrument_id, "US.AAPL");
        assert_eq!(
            page.items[0]
                .chain_info
                .as_ref()
                .and_then(|chain| chain.product_code.as_deref()),
            Some("AAPL")
        );
        assert_eq!(page.next_page.as_deref(), Some("next"));
    }
}
