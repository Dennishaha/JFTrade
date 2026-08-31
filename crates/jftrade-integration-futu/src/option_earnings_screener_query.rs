//! Typed OpenD earnings screener reader (Qot_GetOptionEarningsScreener/3313).

use crate::{
    EventIndicator, EventIndicatorValue, OpenDSessionCoordinator, OpenDSessionCoordinatorError,
    OptionEventSecurity,
};
use prost::Message;
use serde::Serialize;
use std::sync::{Arc, Mutex};
use thiserror::Error;

#[derive(Clone, Debug, PartialEq)]
pub struct OptionEarningsScreenerQuery {
    pub option_market: i32,
    pub sort_type: Option<i32>,
    pub is_asc: Option<bool>,
    pub count: i32,
    pub page: Option<String>,
    pub filters: Vec<EventIndicator>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEarningsScreenerItem {
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
    pub earnings_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_time: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_pub_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_quarter: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_report_iv_crush: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub history_report_iv_crush: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_report_chg_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub history_report_chg_rate: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub estimate_eps_yoy: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub estimate_revenue_yoy: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expected_move_ratio: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionEarningsScreenerPage {
    pub items: Vec<OptionEarningsScreenerItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_page: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub update_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub all_count: Option<i32>,
}

pub trait OptionEarningsScreenerReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionEarningsScreenerQuery,
    ) -> Result<OptionEarningsScreenerPage, OptionEarningsScreenerQueryError>;
}
#[derive(Clone)]
pub struct OpenDOptionEarningsScreenerReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}
impl std::fmt::Debug for OpenDOptionEarningsScreenerReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDOptionEarningsScreenerReader")
            .finish_non_exhaustive()
    }
}
impl OpenDOptionEarningsScreenerReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}
impl OptionEarningsScreenerReadPort for OpenDOptionEarningsScreenerReader {
    fn query(
        &self,
        query: &OptionEarningsScreenerQuery,
    ) -> Result<OptionEarningsScreenerPage, OptionEarningsScreenerQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionEarningsScreenerQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_earnings_screener::PROTOCOL_ID,
                &encode_request(query)?,
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}
fn validate_query(
    query: &OptionEarningsScreenerQuery,
) -> Result<(), OptionEarningsScreenerQueryError> {
    if !matches!(query.option_market, 1 | 3) {
        return Err(OptionEarningsScreenerQueryError::InvalidQuery(
            "earnings optionMarket must be US (1) or HK (3) security".into(),
        ));
    }
    if !(1..=500).contains(&query.count) {
        return Err(OptionEarningsScreenerQueryError::InvalidQuery(
            "earnings count must be between 1 and 500".into(),
        ));
    }
    if let Some(page) = &query.page
        && (page.len() > 1024 || page.chars().any(char::is_control))
    {
        return Err(OptionEarningsScreenerQueryError::InvalidQuery(
            "earnings page token is invalid".into(),
        ));
    }
    if let Some(sort) = query.sort_type
        && !(1..=17).contains(&sort)
    {
        return Err(OptionEarningsScreenerQueryError::InvalidQuery(
            "earnings sortType is unsupported".into(),
        ));
    }
    for filter in &query.filters {
        if filter.indicator_type == 0 || !(1..=20).contains(&filter.indicator_type) {
            return Err(OptionEarningsScreenerQueryError::InvalidQuery(
                "earnings indicatorType is unsupported".into(),
            ));
        }
    }
    Ok(())
}
fn encode_request(
    query: &OptionEarningsScreenerQuery,
) -> Result<Vec<u8>, OptionEarningsScreenerQueryError> {
    use crate::trade_proto::qot_get_option_earnings_screener::{C2s, Request};
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
) -> Result<
    crate::trade_proto::qot_option_common::EarningsIndicator,
    OptionEarningsScreenerQueryError,
> {
    Ok(crate::trade_proto::qot_option_common::EarningsIndicator {
        indicator_type: indicator.indicator_type,
        indicator_value: indicator.value.as_ref().map(encode_value).transpose()?,
    })
}
fn encode_value(
    value: &EventIndicatorValue,
) -> Result<crate::trade_proto::qot_option_common::IndicatorValue, OptionEarningsScreenerQueryError>
{
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
) -> Result<crate::trade_proto::qot_common::Security, OptionEarningsScreenerQueryError> {
    let market = match security.market.to_ascii_uppercase().as_str() {
        "US" => 11,
        "HK" => 1,
        _ => {
            return Err(OptionEarningsScreenerQueryError::InvalidQuery(
                "earnings security market must be US or HK".into(),
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
) -> Result<OptionEarningsScreenerPage, OptionEarningsScreenerQueryError> {
    use crate::trade_proto::qot_get_option_earnings_screener::Response;
    let response = Response::decode(body).map_err(OptionEarningsScreenerQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionEarningsScreenerQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD earnings screener request failed".into()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionEarningsScreenerQueryError::MissingS2c);
    };
    let items = s2c
        .item_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    if s2c.all_count.is_some_and(|count| count < 0) {
        return Err(OptionEarningsScreenerQueryError::InvalidResponse(
            "earnings allCount must be non-negative".into(),
        ));
    }
    Ok(OptionEarningsScreenerPage {
        items,
        next_page: s2c.next_page.filter(|p| !p.is_empty()),
        update_timestamp: finite(s2c.update_timestamp, "updateTimestamp")?,
        all_count: s2c.all_count,
    })
}
fn map_item(
    item: crate::trade_proto::qot_get_option_earnings_screener::EarningsItem,
) -> Result<OptionEarningsScreenerItem, OptionEarningsScreenerQueryError> {
    let owner = map_security(item.owner)?;
    Ok(OptionEarningsScreenerItem {
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
        earnings_timestamp: finite(item.earnings_timestamp, "earningsTimestamp")?,
        earnings_time: clean(item.earnings_time),
        earnings_pub_type: item.earnings_pub_type,
        earnings_quarter: clean(item.earnings_quarter),
        last_report_iv_crush: finite(item.last_report_iv_crush, "lastReportIvCrush")?,
        history_report_iv_crush: finite(item.history_report_iv_crush, "historyReportIvCrush")?,
        last_report_chg_rate: finite(item.last_report_chg_rate, "lastReportChgRate")?,
        history_report_chg_rate: finite(item.history_report_chg_rate, "historyReportChgRate")?,
        estimate_eps_yoy: finite(item.estimate_eps_yoy, "estimateEpsYoy")?,
        estimate_revenue_yoy: finite(item.estimate_revenue_yoy, "estimateRevenueYoy")?,
        expected_move_ratio: finite(item.expected_move_ratio, "expectedMoveRatio")?,
    })
}
fn map_security(
    security: crate::trade_proto::qot_common::Security,
) -> Result<OptionEventSecurity, OptionEarningsScreenerQueryError> {
    let (market, wire) = match security.market {
        11 => ("US", "US"),
        1 => ("HK", "HK"),
        _ => {
            return Err(OptionEarningsScreenerQueryError::InvalidResponse(
                "earnings security market is unsupported".into(),
            ));
        }
    };
    let code = security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(OptionEarningsScreenerQueryError::InvalidResponse(
            "earnings security code is empty".into(),
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
fn finite(
    value: Option<f64>,
    field: &str,
) -> Result<Option<f64>, OptionEarningsScreenerQueryError> {
    if value.is_some_and(|v| !v.is_finite()) {
        return Err(OptionEarningsScreenerQueryError::InvalidResponse(format!(
            "earnings {field} must be finite"
        )));
    }
    Ok(value)
}
fn non_negative(
    value: Option<i64>,
    field: &str,
) -> Result<Option<i64>, OptionEarningsScreenerQueryError> {
    if value.is_some_and(|v| v < 0) {
        return Err(OptionEarningsScreenerQueryError::InvalidResponse(format!(
            "earnings {field} must be non-negative"
        )));
    }
    Ok(value)
}

#[derive(Debug, Error)]
pub enum OptionEarningsScreenerQueryError {
    #[error("invalid earnings screener query: {0}")]
    InvalidQuery(String),
    #[error("decode OpenD earnings screener response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD earnings screener response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD earnings screener response: {0}")]
    InvalidResponse(String),
    #[error("OpenD session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_only_security_option_markets() {
        for option_market in [1, 3] {
            let query = OptionEarningsScreenerQuery {
                option_market,
                sort_type: None,
                is_asc: None,
                count: 50,
                page: None,
                filters: Vec::new(),
            };
            assert!(validate_query(&query).is_ok());
        }
        let query = OptionEarningsScreenerQuery {
            option_market: 2,
            sort_type: None,
            is_asc: None,
            count: 50,
            page: None,
            filters: Vec::new(),
        };
        assert!(validate_query(&query).is_err());
    }

    #[test]
    fn decodes_earnings_fields_and_total() {
        use crate::trade_proto::qot_get_option_earnings_screener::{EarningsItem, Response, S2c};
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![EarningsItem {
                    owner: crate::trade_proto::qot_common::Security {
                        market: 1,
                        code: "00700".into(),
                    },
                    name: Some("Tencent".into()),
                    price: Some(400.0),
                    change_rate: None,
                    market_cap: None,
                    iv: Some(25.0),
                    iv_rank: None,
                    iv_percentile: None,
                    hv: None,
                    volume: Some(8),
                    open_interest: None,
                    earnings_timestamp: Some(1.0),
                    earnings_time: Some("2026-09-01".into()),
                    earnings_pub_type: Some(1),
                    earnings_quarter: Some("2026Q2".into()),
                    last_report_iv_crush: None,
                    history_report_iv_crush: None,
                    last_report_chg_rate: None,
                    history_report_chg_rate: None,
                    estimate_eps_yoy: None,
                    estimate_revenue_yoy: None,
                    expected_move_ratio: Some(4.0),
                }],
                next_page: Some("next".into()),
                update_timestamp: Some(3.0),
                all_count: Some(7),
            }),
        };
        let page = decode_response(&body.encode_to_vec()).expect("decode");
        assert_eq!(page.items[0].owner.instrument_id, "HK.00700");
        assert_eq!(page.items[0].earnings_quarter.as_deref(), Some("2026Q2"));
        assert_eq!(page.all_count, Some(7));
    }
}
