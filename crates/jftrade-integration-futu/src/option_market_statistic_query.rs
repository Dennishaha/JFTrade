//! Typed OpenD option-market statistic reader
//! (`Qot_GetOptionMarketStatistic/3301`).
//!
//! The generated protobuf messages remain private to the Futu integration
//! crate. Callers receive a broker-neutral daily call/put aggregate series and
//! an opaque pagination cursor.

use std::sync::{Arc, Mutex};

use base64::{Engine as _, engine::general_purpose::STANDARD};
use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const MAX_PAGE_KEY_BYTES: usize = 768;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionMarketStatisticQuery {
    /// OpenD option market: US security/index = 1/2, HK security/index = 3/4.
    pub option_market: i32,
    /// OpenD statistic data type: volume = 0, open interest = 1.
    pub data_type: i32,
    pub begin_time: String,
    pub end_time: String,
    /// Raw OpenD `nextPageKey`; an empty value requests the first page.
    pub next_page_key: Vec<u8>,
}

impl OptionMarketStatisticQuery {
    pub fn validate(&self) -> Result<(), OptionMarketStatisticQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionMarketStatisticItem {
    pub time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<f64>,
    pub call_value: i64,
    pub put_value: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub total_value: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ratio: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionMarketStatisticSnapshot {
    pub option_market: i32,
    pub market: String,
    pub data_type: i32,
    pub items: Vec<OptionMarketStatisticItem>,
    /// Raw OpenD `nextPageKey`; an empty value means there is no next page.
    #[serde(skip)]
    pub next_page_key: Vec<u8>,
}

pub trait OptionMarketStatisticReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionMarketStatisticQuery,
    ) -> Result<OptionMarketStatisticSnapshot, OptionMarketStatisticQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionMarketStatisticReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionMarketStatisticReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionMarketStatisticReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionMarketStatisticReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionMarketStatisticReadPort for OpenDOptionMarketStatisticReader {
    fn query(
        &self,
        query: &OptionMarketStatisticQuery,
    ) -> Result<OptionMarketStatisticSnapshot, OptionMarketStatisticQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionMarketStatisticQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_market_statistic::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(
    query: &OptionMarketStatisticQuery,
) -> Result<(), OptionMarketStatisticQueryError> {
    market_label(query.option_market).ok_or_else(|| {
        OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic optionMarket must be 1, 2, 3, or 4".to_owned(),
        )
    })?;
    if !matches!(query.data_type, 0 | 1) {
        return Err(OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic dataType must be volume (0) or open interest (1)".to_owned(),
        ));
    }
    let begin = parse_date(&query.begin_time).ok_or_else(|| {
        OptionMarketStatisticQueryError::InvalidQuery("beginTime must be YYYY-MM-DD".to_owned())
    })?;
    let end = parse_date(&query.end_time).ok_or_else(|| {
        OptionMarketStatisticQueryError::InvalidQuery("endTime must be YYYY-MM-DD".to_owned())
    })?;
    if end < begin {
        return Err(OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic date range must be ordered".to_owned(),
        ));
    }
    if query.next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic nextPageKey is too large".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionMarketStatisticQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_market_statistic::{C2s, Request};
    Request {
        c2s: C2s {
            option_market: query.option_market,
            data_type: query.data_type,
            begin_time: query.begin_time.trim().to_owned(),
            end_time: query.end_time.trim().to_owned(),
            next_page_key: (!query.next_page_key.is_empty()).then(|| query.next_page_key.clone()),
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionMarketStatisticQuery,
) -> Result<OptionMarketStatisticSnapshot, OptionMarketStatisticQueryError> {
    use crate::trade_proto::qot_get_option_market_statistic::Response;
    let response = Response::decode(body).map_err(OptionMarketStatisticQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionMarketStatisticQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD option market statistic request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionMarketStatisticQueryError::MissingS2c);
    };
    if s2c.option_market != query.option_market || s2c.data_type != query.data_type {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic response scope does not match query".to_owned(),
        ));
    }
    let items = s2c
        .statistic_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    let next_page_key = s2c.next_page_key.unwrap_or_default();
    if next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic nextPageKey is too large".to_owned(),
        ));
    }
    let market = market_label(query.option_market).expect("query validation ensures market");
    Ok(OptionMarketStatisticSnapshot {
        option_market: query.option_market,
        market: market.to_owned(),
        data_type: query.data_type,
        items,
        next_page_key,
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_market_statistic::StatisticItem,
) -> Result<OptionMarketStatisticItem, OptionMarketStatisticQueryError> {
    let time = item.time.trim().to_owned();
    if parse_date(&time).is_none() {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic item time must be YYYY-MM-DD".to_owned(),
        ));
    }
    if item.call_value < 0 || item.put_value < 0 {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic callValue and putValue must be non-negative".to_owned(),
        ));
    }
    if let Some(value) = item.total_value
        && (value < 0 || value < item.call_value.saturating_add(item.put_value))
    {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic totalValue is inconsistent with callValue and putValue"
                .to_owned(),
        ));
    }
    if let Some(value) = item.timestamp
        && !value.is_finite()
    {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic timestamp must be finite".to_owned(),
        ));
    }
    if let Some(value) = item.ratio
        && (!value.is_finite() || value < 0.0)
    {
        return Err(OptionMarketStatisticQueryError::InvalidResponse(
            "option market statistic ratio must be finite and non-negative".to_owned(),
        ));
    }
    Ok(OptionMarketStatisticItem {
        time,
        timestamp: item.timestamp,
        call_value: item.call_value,
        put_value: item.put_value,
        total_value: item.total_value,
        ratio: item.ratio,
    })
}

fn parse_date(value: &str) -> Option<Date> {
    let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]").ok()?;
    Date::parse(value.trim(), &format).ok()
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 | 2 => Some("US"),
        3 | 4 => Some("HK"),
        _ => None,
    }
}

/// Decode the standard protojson representation of a `bytes nextPageKey`.
pub fn decode_next_page_key(value: &str) -> Result<Vec<u8>, OptionMarketStatisticQueryError> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(Vec::new());
    }
    if value.len() > 1024 {
        return Err(OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic cursor is too large".to_owned(),
        ));
    }
    let bytes = STANDARD.decode(value).map_err(|_| {
        OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic cursor must be base64".to_owned(),
        )
    })?;
    if bytes.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionMarketStatisticQueryError::InvalidQuery(
            "option market statistic cursor is too large".to_owned(),
        ));
    }
    Ok(bytes)
}

/// Encode a raw OpenD `nextPageKey` using the standard protojson base64 form.
pub fn encode_next_page_key(value: &[u8]) -> Option<String> {
    (!value.is_empty()).then(|| STANDARD.encode(value))
}

#[derive(Debug, Error)]
pub enum OptionMarketStatisticQueryError {
    #[error("invalid OpenD option market statistic query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option market statistic session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionMarketStatistic response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetOptionMarketStatistic retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionMarketStatistic response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option market statistic response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_market_statistic::{Response, S2c, StatisticItem};
    use prost::Message;

    fn query() -> OptionMarketStatisticQuery {
        OptionMarketStatisticQuery {
            option_market: 1,
            data_type: 0,
            begin_time: "2026-08-01".to_owned(),
            end_time: "2026-08-29".to_owned(),
            next_page_key: vec![1, 2, 3],
        }
    }

    #[test]
    fn request_encodes_scope_dates_and_cursor() {
        let request = crate::trade_proto::qot_get_option_market_statistic::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.option_market, 1);
        assert_eq!(request.c2s.data_type, 0);
        assert_eq!(request.c2s.begin_time, "2026-08-01");
        assert_eq!(request.c2s.end_time, "2026-08-29");
        assert_eq!(request.c2s.next_page_key, Some(vec![1, 2, 3]));
    }

    #[test]
    fn response_maps_items_and_cursor() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                data_type: 0,
                statistic_list: vec![StatisticItem {
                    time: "2026-08-29".to_owned(),
                    timestamp: Some(1_756_000_000.0),
                    call_value: 100,
                    put_value: 50,
                    total_value: Some(150),
                    ratio: Some(0.5),
                }],
                next_page_key: Some(vec![4, 5, 6]),
            }),
        }
        .encode_to_vec();
        let snapshot = decode_response(&body, &query()).expect("snapshot");
        assert_eq!(snapshot.market, "US");
        assert_eq!(snapshot.items[0].total_value, Some(150));
        assert_eq!(snapshot.next_page_key, vec![4, 5, 6]);
    }

    #[test]
    fn rejects_invalid_query_and_malformed_response() {
        assert!(matches!(
            validate_query(&OptionMarketStatisticQuery {
                option_market: 9,
                data_type: 0,
                begin_time: "2026-08-01".to_owned(),
                end_time: "2026-08-29".to_owned(),
                next_page_key: Vec::new(),
            }),
            Err(OptionMarketStatisticQueryError::InvalidQuery(_))
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
            Err(OptionMarketStatisticQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                option_market: 1,
                data_type: 0,
                statistic_list: vec![StatisticItem {
                    time: "2026-08-29".to_owned(),
                    call_value: -1,
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionMarketStatisticQueryError::InvalidResponse(_))
        ));
    }

    #[test]
    fn cursor_uses_protojson_base64() {
        let encoded = encode_next_page_key(&[1, 2, 3]).expect("cursor");
        assert_eq!(encoded, "AQID");
        assert_eq!(
            decode_next_page_key(&encoded).expect("decoded"),
            vec![1, 2, 3]
        );
        assert!(decode_next_page_key("not base64!").is_err());
    }
}
