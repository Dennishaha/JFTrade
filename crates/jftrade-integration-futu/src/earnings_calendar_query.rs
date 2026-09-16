//! Typed OpenD earnings-calendar reader (`Qot_GetEarningsCalendar`, protocol 3401).
//!
//! The engine owns the public parameter translation, the 7-day chunking and the
//! merged-entry deduplication. This adapter only encodes one already-chunked
//! request into protobuf, validates the response and projects provider-neutral
//! rows, so the same wire contract cannot diverge between callers.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

pub const EARNINGS_CALENDAR_PROTOCOL_ID: u32 = 3401;

/// One OpenD page for an already-chunked date range. `begin_date`/`end_date`
/// are validated by the engine before they reach this boundary.
#[derive(Clone, Debug, PartialEq)]
pub struct EarningsCalendarQuery {
    pub market: i32,
    pub sort_type: Option<i32>,
    pub begin_date: String,
    pub end_date: String,
    pub filters: Vec<EarningsCalendarFilter>,
}

/// A single AND-ed filter. Exactly one of `value_list` / `interval` is set.
#[derive(Clone, Debug, PartialEq)]
pub struct EarningsCalendarFilter {
    pub indicator_type: i32,
    pub value_list: Vec<i64>,
    pub interval: Option<EarningsCalendarInterval>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EarningsCalendarInterval {
    pub min: Option<EarningsCalendarBoundary>,
    pub max: Option<EarningsCalendarBoundary>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct EarningsCalendarBoundary {
    pub value: f64,
    pub includes: bool,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EarningsCalendarEstimate {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub estimate_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub actual_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub predict_value: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub period_type: Option<i32>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EarningsCalendarItem {
    pub security: EarningsCalendarSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub earnings_timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pub_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub period_text: Option<String>,
    pub estimate_list: Vec<EarningsCalendarEstimate>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_volume: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_rank: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv_percentile: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub market_cap: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub price: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EarningsCalendarSecurity {
    pub market: String,
    pub code: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EarningsCalendarPage {
    pub items: Vec<EarningsCalendarItem>,
}

pub trait EarningsCalendarReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &EarningsCalendarQuery,
    ) -> Result<EarningsCalendarPage, EarningsCalendarQueryError>;
}

#[derive(Clone)]
pub struct OpenDEarningsCalendarReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDEarningsCalendarReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDEarningsCalendarReader")
            .finish_non_exhaustive()
    }
}

impl OpenDEarningsCalendarReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl EarningsCalendarReadPort for OpenDEarningsCalendarReader {
    fn query(
        &self,
        query: &EarningsCalendarQuery,
    ) -> Result<EarningsCalendarPage, EarningsCalendarQueryError> {
        validate_query(query)?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            EarningsCalendarQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(EARNINGS_CALENDAR_PROTOCOL_ID, &encode_request(query))
            .map_err(OpenDSessionCoordinatorError::from)?;
        decode_response(&response)
    }
}

#[derive(Debug, Error)]
pub enum EarningsCalendarQueryError {
    #[error("invalid earnings calendar query: {0}")]
    InvalidQuery(String),
    #[error("decode OpenD earnings-calendar response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD earnings-calendar request rejected ({ret_type}/{err_code}): {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD earnings-calendar response is missing s2c")]
    MissingS2c,
    #[error("invalid OpenD earnings-calendar response: {0}")]
    InvalidResponse(String),
    #[error("OpenD earnings-calendar session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
}

/// Market codes accepted by `Qot_GetEarningsCalendar`. The Go baseline
/// translates HK/US/SH/SZ/SG/JP/AU/CA; SH and SZ both map to the combined
/// A-share market value.
pub fn earnings_calendar_market_value(market: &str) -> Option<i32> {
    match market.trim().to_ascii_uppercase().as_str() {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" | "SZ" | "CN" => Some(21),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "CA" => Some(71),
        _ => None,
    }
}

fn validate_query(query: &EarningsCalendarQuery) -> Result<(), EarningsCalendarQueryError> {
    if !matches!(query.market, 1 | 11 | 21 | 22 | 31 | 41 | 51 | 71) {
        return Err(EarningsCalendarQueryError::InvalidQuery(
            "earnings calendar market is unsupported".into(),
        ));
    }
    if query.begin_date.trim().is_empty() || query.end_date.trim().is_empty() {
        return Err(EarningsCalendarQueryError::InvalidQuery(
            "earnings calendar date range is required".into(),
        ));
    }
    for filter in &query.filters {
        if filter.value_list.is_empty()
            && filter
                .interval
                .as_ref()
                .is_none_or(|interval| interval.min.is_none() && interval.max.is_none())
        {
            return Err(EarningsCalendarQueryError::InvalidQuery(
                "earnings calendar filter has no value".into(),
            ));
        }
        if let Some(interval) = &filter.interval {
            for boundary in [interval.min.as_ref(), interval.max.as_ref()]
                .into_iter()
                .flatten()
            {
                if !boundary.value.is_finite() {
                    return Err(EarningsCalendarQueryError::InvalidQuery(
                        "earnings calendar filter boundary must be finite".into(),
                    ));
                }
            }
        }
    }
    Ok(())
}

fn encode_request(query: &EarningsCalendarQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_earnings_calendar as wire;
    wire::Request {
        c2s: wire::C2s {
            market: query.market,
            sort_type: query.sort_type,
            begin_date: Some(query.begin_date.clone()),
            end_date: Some(query.end_date.clone()),
            filter_list: query
                .filters
                .iter()
                .map(|filter| wire::EarningsCalendarIndicator {
                    indicator_type: filter.indicator_type,
                    indicator_value: Some(crate::trade_proto::qot_option_common::IndicatorValue {
                        value_list: filter.value_list.clone(),
                        value_interval: filter.interval.as_ref().map(|interval| {
                            crate::trade_proto::qot_option_common::Interval {
                                filter_min: interval.min.as_ref().map(boundary),
                                filter_max: interval.max.as_ref().map(boundary),
                            }
                        }),
                        string_value_list: Vec::new(),
                        security_list: Vec::new(),
                    }),
                })
                .collect(),
        },
    }
    .encode_to_vec()
}

fn boundary(value: &EarningsCalendarBoundary) -> crate::trade_proto::qot_option_common::Boundary {
    crate::trade_proto::qot_option_common::Boundary {
        value: value.value,
        includes: value.includes,
    }
}

fn decode_response(body: &[u8]) -> Result<EarningsCalendarPage, EarningsCalendarQueryError> {
    use crate::trade_proto::qot_get_earnings_calendar::Response;
    let response = Response::decode(body)?;
    if response.ret_type != 0 {
        return Err(EarningsCalendarQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD earnings-calendar request failed".into()),
        });
    }
    let s2c = response.s2c.ok_or(EarningsCalendarQueryError::MissingS2c)?;
    let mut items = Vec::with_capacity(s2c.item_list.len());
    for item in s2c.item_list {
        items.push(map_item(item)?);
    }
    Ok(EarningsCalendarPage { items })
}

/// Fail closed on non-finite numerics: a `NaN`/`inf` from OpenD would otherwise
/// serialize into invalid JSON.
fn finite(value: Option<f64>, field: &str) -> Result<Option<f64>, EarningsCalendarQueryError> {
    match value {
        Some(value) if !value.is_finite() => Err(EarningsCalendarQueryError::InvalidResponse(
            format!("earnings calendar {field} must be finite"),
        )),
        other => Ok(other),
    }
}

fn map_item(
    item: crate::trade_proto::qot_get_earnings_calendar::EarningsCalendarItem,
) -> Result<EarningsCalendarItem, EarningsCalendarQueryError> {
    let market = earnings_calendar_market_label(item.security.market).ok_or_else(|| {
        EarningsCalendarQueryError::InvalidResponse(
            "earnings calendar item has an unsupported market".into(),
        )
    })?;
    let code = item.security.code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(EarningsCalendarQueryError::InvalidResponse(
            "earnings calendar item has an empty code".into(),
        ));
    }
    let instrument_id = format!("{market}.{code}");
    let estimate_list = item
        .estimate_list
        .into_iter()
        .map(|estimate| {
            Ok(EarningsCalendarEstimate {
                estimate_type: estimate.estimate_type,
                actual_value: finite(estimate.actual_value, "actualValue")?,
                predict_value: finite(estimate.predict_value, "predictValue")?,
                currency: estimate.currency.filter(|value| !value.trim().is_empty()),
                period_type: estimate.period_type,
            })
        })
        .collect::<Result<Vec<_>, EarningsCalendarQueryError>>()?;
    Ok(EarningsCalendarItem {
        security: EarningsCalendarSecurity {
            market: market.to_owned(),
            code,
            instrument_id,
        },
        name: item.name.filter(|value| !value.trim().is_empty()),
        earnings_date: item.earnings_date.filter(|value| !value.trim().is_empty()),
        earnings_timestamp: finite(item.earnings_timestamp, "earningsTimestamp")?,
        pub_type: item.pub_type,
        period_text: item.period_text.filter(|value| !value.trim().is_empty()),
        estimate_list,
        option_volume: item.option_volume,
        iv: finite(item.iv, "iv")?,
        iv_rank: finite(item.iv_rank, "ivRank")?,
        iv_percentile: finite(item.iv_percentile, "ivPercentile")?,
        market_cap: finite(item.market_cap, "marketCap")?,
        price: finite(item.price, "price")?,
    })
}

/// Market labels used by the public wire. SH and SZ are distinct even though
/// they share one OpenD A-share market value.
fn earnings_calendar_market_label(market: i32) -> Option<&'static str> {
    match market {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        71 => Some("CA"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_earnings_calendar::{EstimateData, Request, Response, S2c};

    fn item() -> crate::trade_proto::qot_get_earnings_calendar::EarningsCalendarItem {
        use crate::trade_proto::qot_get_earnings_calendar::EarningsCalendarItem as WireItem;
        WireItem {
            security: crate::trade_proto::qot_common::Security {
                market: 11,
                code: "aapl".to_owned(),
            },
            name: Some("Apple".to_owned()),
            earnings_date: Some("2026-07-22".to_owned()),
            earnings_timestamp: Some(1_784_000_000.0),
            pub_type: Some(2),
            period_text: Some("2025Q2".to_owned()),
            estimate_list: vec![EstimateData {
                estimate_type: Some(1),
                actual_value: None,
                predict_value: Some(1.5),
                currency: Some("USD".to_owned()),
                period_type: Some(1),
            }],
            option_volume: Some(12_000),
            iv: Some(42.5),
            iv_rank: Some(11.0),
            iv_percentile: Some(30.0),
            market_cap: Some(1_000_000_000.0),
            price: Some(190.25),
        }
    }

    #[test]
    fn encodes_market_sort_dates_and_both_filter_shapes() {
        let query = EarningsCalendarQuery {
            market: 11,
            sort_type: Some(6),
            begin_date: "2026-07-01".to_owned(),
            end_date: "2026-07-07".to_owned(),
            filters: vec![
                EarningsCalendarFilter {
                    indicator_type: 4,
                    value_list: vec![1],
                    interval: None,
                },
                EarningsCalendarFilter {
                    indicator_type: 6,
                    value_list: Vec::new(),
                    interval: Some(EarningsCalendarInterval {
                        min: Some(EarningsCalendarBoundary {
                            value: 10.0,
                            includes: true,
                        }),
                        max: Some(EarningsCalendarBoundary {
                            value: 80.0,
                            includes: true,
                        }),
                    }),
                },
            ],
        };
        let request = Request::decode(encode_request(&query).as_slice()).expect("decode request");
        let c2s = request.c2s;
        assert_eq!(c2s.market, 11);
        assert_eq!(c2s.sort_type, Some(6));
        assert_eq!(c2s.begin_date.as_deref(), Some("2026-07-01"));
        assert_eq!(c2s.end_date.as_deref(), Some("2026-07-07"));
        assert_eq!(c2s.filter_list.len(), 2);
        assert_eq!(c2s.filter_list[0].indicator_type, 4);
        assert_eq!(
            c2s.filter_list[0]
                .indicator_value
                .as_ref()
                .map(|value| value.value_list.as_slice()),
            Some([1_i64].as_slice())
        );
        let interval = c2s.filter_list[1]
            .indicator_value
            .as_ref()
            .and_then(|value| value.value_interval.as_ref())
            .expect("interval");
        assert_eq!(
            interval.filter_min.as_ref().map(|value| value.value),
            Some(10.0)
        );
        assert_eq!(
            interval.filter_max.as_ref().map(|value| value.value),
            Some(80.0)
        );
        assert_eq!(
            interval.filter_min.as_ref().map(|value| value.includes),
            Some(true)
        );
    }

    #[test]
    fn projects_items_and_rejects_non_finite_or_unusable_rows() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![item()],
            }),
        }
        .encode_to_vec();
        let page = decode_response(&body).expect("decode");
        assert_eq!(page.items.len(), 1);
        let row = &page.items[0];
        assert_eq!(row.security.instrument_id, "US.AAPL");
        assert_eq!(row.security.market, "US");
        assert_eq!(row.security.code, "AAPL");
        assert_eq!(row.name.as_deref(), Some("Apple"));
        assert_eq!(row.earnings_date.as_deref(), Some("2026-07-22"));
        assert_eq!(row.estimate_list.len(), 1);
        assert_eq!(row.estimate_list[0].predict_value, Some(1.5));
        assert_eq!(row.option_volume, Some(12_000));
        assert_eq!(row.iv, Some(42.5));
        assert_eq!(row.price, Some(190.25));

        // Non-finite numerics must fail closed rather than serialize as JSON.
        let mut broken = item();
        broken.iv = Some(f64::NAN);
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![broken],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body),
            Err(EarningsCalendarQueryError::InvalidResponse(message)) if message.contains("finite")
        ));

        // An unsupported market or empty code is rejected, never relabelled.
        let mut broken = item();
        broken.security.market = 999;
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![broken],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body),
            Err(EarningsCalendarQueryError::InvalidResponse(message)) if message.contains("market")
        ));

        let mut broken = item();
        broken.security.code = "   ".to_owned();
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                item_list: vec![broken],
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body),
            Err(EarningsCalendarQueryError::InvalidResponse(message)) if message.contains("code")
        ));
    }

    #[test]
    fn rejects_rejection_missing_s2c_and_unsupported_markets() {
        let body = Response {
            ret_type: -1,
            ret_msg: Some("denied".to_owned()),
            err_code: Some(1001),
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body),
            Err(EarningsCalendarQueryError::Rejected {
                ret_type: -1,
                err_code: 1001,
                message,
            }) if message == "denied"
        ));

        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: None,
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&body),
            Err(EarningsCalendarQueryError::MissingS2c)
        ));

        assert_eq!(earnings_calendar_market_value(" hk "), Some(1));
        assert_eq!(earnings_calendar_market_value("us"), Some(11));
        assert_eq!(earnings_calendar_market_value("SH"), Some(21));
        assert_eq!(earnings_calendar_market_value("SZ"), Some(21));
        assert_eq!(earnings_calendar_market_value("SG"), Some(31));
        assert_eq!(earnings_calendar_market_value("JP"), Some(41));
        assert_eq!(earnings_calendar_market_value("AU"), Some(51));
        assert_eq!(earnings_calendar_market_value("CA"), Some(71));
        assert_eq!(earnings_calendar_market_value("EU"), None);
    }

    #[test]
    fn query_validation_rejects_incomplete_filters_and_dates() {
        let base = EarningsCalendarQuery {
            market: 11,
            sort_type: None,
            begin_date: "2026-07-01".to_owned(),
            end_date: "2026-07-07".to_owned(),
            filters: Vec::new(),
        };
        assert!(validate_query(&base).is_ok());

        let mut invalid = base.clone();
        invalid.begin_date = "  ".to_owned();
        assert!(matches!(
            validate_query(&invalid),
            Err(EarningsCalendarQueryError::InvalidQuery(_))
        ));

        let mut invalid = base.clone();
        invalid.market = 999;
        assert!(matches!(
            validate_query(&invalid),
            Err(EarningsCalendarQueryError::InvalidQuery(_))
        ));

        let mut invalid = base.clone();
        invalid.filters = vec![EarningsCalendarFilter {
            indicator_type: 3,
            value_list: Vec::new(),
            interval: None,
        }];
        assert!(matches!(
            validate_query(&invalid),
            Err(EarningsCalendarQueryError::InvalidQuery(_))
        ));

        let mut invalid = base;
        invalid.filters = vec![EarningsCalendarFilter {
            indicator_type: 6,
            value_list: Vec::new(),
            interval: Some(EarningsCalendarInterval {
                min: Some(EarningsCalendarBoundary {
                    value: f64::INFINITY,
                    includes: true,
                }),
                max: None,
            }),
        }];
        assert!(matches!(
            validate_query(&invalid),
            Err(EarningsCalendarQueryError::InvalidQuery(_))
        ));
    }
}
