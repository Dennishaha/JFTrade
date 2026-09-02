//! Typed OpenD option-underlying historical statistic reader
//! (`Qot_GetOptionUnderlyingHisStatistic/3302`).
//!
//! Generated protobuf messages stay private to this integration crate. The
//! engine receives a neutral daily volume/open-interest series for one
//! underlying plus an opaque pagination cursor.

use std::sync::{Arc, Mutex};

use base64::{Engine as _, engine::general_purpose::STANDARD};
use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const MAX_PAGE_KEY_BYTES: usize = 768;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionUnderlyingHisStatisticQuery {
    /// Public quote market: HK = 1 or US = 11.
    pub market: i32,
    pub code: String,
    pub index_option_type: Option<i32>,
    pub begin_time: String,
    pub end_time: String,
    /// Raw OpenD `nextPageKey`; an empty value requests the first page.
    pub next_page_key: Vec<u8>,
}

impl OptionUnderlyingHisStatisticQuery {
    pub fn validate(&self) -> Result<(), OptionUnderlyingHisStatisticQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisStatisticSecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisStatisticItem {
    pub time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_volume: Option<i64>,
    pub call_volume: i64,
    pub put_volume: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub put_call_volume_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub option_open_interest: Option<i64>,
    pub call_open_interest: i64,
    pub put_open_interest: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub put_call_open_interest_ratio: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub underlying_price: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisStatisticSnapshot {
    pub security: OptionUnderlyingHisStatisticSecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub items: Vec<OptionUnderlyingHisStatisticItem>,
    /// Raw OpenD `nextPageKey`; an empty value means there is no next page.
    #[serde(skip)]
    pub next_page_key: Vec<u8>,
}

pub trait OptionUnderlyingHisStatisticReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionUnderlyingHisStatisticQuery,
    ) -> Result<OptionUnderlyingHisStatisticSnapshot, OptionUnderlyingHisStatisticQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionUnderlyingHisStatisticReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionUnderlyingHisStatisticReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionUnderlyingHisStatisticReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionUnderlyingHisStatisticReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionUnderlyingHisStatisticReadPort for OpenDOptionUnderlyingHisStatisticReader {
    fn query(
        &self,
        query: &OptionUnderlyingHisStatisticQuery,
    ) -> Result<OptionUnderlyingHisStatisticSnapshot, OptionUnderlyingHisStatisticQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionUnderlyingHisStatisticQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_underlying_his_statistic::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(
    query: &OptionUnderlyingHisStatisticQuery,
) -> Result<(), OptionUnderlyingHisStatisticQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            format!("option underlying historical statistic code must be a {market} code"),
        ));
    }
    if let Some(index_option_type) = query.index_option_type
        && !matches!(index_option_type, 0..=2)
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic indexOptionType must be 0, 1, or 2".to_owned(),
        ));
    }
    let begin = parse_date(&query.begin_time).ok_or_else(|| {
        OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "beginTime must be YYYY-MM-DD".to_owned(),
        )
    })?;
    let end = parse_date(&query.end_time).ok_or_else(|| {
        OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "endTime must be YYYY-MM-DD".to_owned(),
        )
    })?;
    if end < begin {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic date range must be ordered".to_owned(),
        ));
    }
    if query.next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic nextPageKey is too large".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionUnderlyingHisStatisticQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_underlying_his_statistic::{C2s, Request};
    Request {
        c2s: C2s {
            owner: crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.code.trim().to_ascii_uppercase(),
            },
            index_option_type: query.index_option_type,
            begin_time: query.begin_time.trim().to_owned(),
            end_time: query.end_time.trim().to_owned(),
            next_page_key: (!query.next_page_key.is_empty()).then(|| query.next_page_key.clone()),
        },
    }
    .encode_to_vec()
}

fn decode_response(
    body: &[u8],
    query: &OptionUnderlyingHisStatisticQuery,
) -> Result<OptionUnderlyingHisStatisticSnapshot, OptionUnderlyingHisStatisticQueryError> {
    use crate::trade_proto::qot_get_option_underlying_his_statistic::Response;
    let response =
        Response::decode(body).map_err(OptionUnderlyingHisStatisticQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionUnderlyingHisStatisticQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response.ret_msg.unwrap_or_else(|| {
                "OpenD option underlying historical statistic request failed".to_owned()
            }),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionUnderlyingHisStatisticQueryError::MissingS2c);
    };
    let expected_code = query.code.trim().to_ascii_uppercase();
    if s2c.owner.market != query.market
        || s2c.owner.code.trim().to_ascii_uppercase() != expected_code
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic owner does not match query".to_owned(),
        ));
    }
    let owner_code = validate_code(&s2c.owner.code).ok_or_else(|| {
        OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic owner code is invalid".to_owned(),
        )
    })?;
    let code = optional_text(s2c.code);
    if let Some(value) = code.as_deref()
        && value.to_ascii_uppercase() != expected_code
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic response code does not match query".to_owned(),
        ));
    }
    let next_page_key = s2c.next_page_key.unwrap_or_default();
    if next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic nextPageKey is too large".to_owned(),
        ));
    }
    let items = s2c
        .statistic_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    let market = market_label(query.market).expect("query validation ensures market");
    Ok(OptionUnderlyingHisStatisticSnapshot {
        security: security_from_wire(market, &owner_code),
        code,
        name: optional_text(s2c.name),
        items,
        next_page_key,
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_underlying_his_statistic::DailyStatistic,
) -> Result<OptionUnderlyingHisStatisticItem, OptionUnderlyingHisStatisticQueryError> {
    let time = item.time.trim().to_owned();
    if parse_date(&time).is_none() {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic item time must be YYYY-MM-DD".to_owned(),
        ));
    }
    for (field, value) in [
        ("optionVolume", item.option_volume),
        ("callVolume", Some(item.call_volume)),
        ("putVolume", Some(item.put_volume)),
        ("optionOpenInterest", item.option_open_interest),
        ("callOpenInterest", Some(item.call_open_interest)),
        ("putOpenInterest", Some(item.put_open_interest)),
    ] {
        if value.is_some_and(|value| value < 0) {
            return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
                format!("option underlying historical statistic {field} must be non-negative"),
            ));
        }
    }
    if let Some(value) = item.option_volume
        && value < item.call_volume.saturating_add(item.put_volume)
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic optionVolume is inconsistent with callVolume and putVolume"
                .to_owned(),
        ));
    }
    if let Some(value) = item.option_open_interest
        && value
            < item
                .call_open_interest
                .saturating_add(item.put_open_interest)
    {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
            "option underlying historical statistic optionOpenInterest is inconsistent with callOpenInterest and putOpenInterest"
                .to_owned(),
        ));
    }
    for (field, value) in [
        ("timestamp", item.timestamp),
        ("putCallVolumeRatio", item.put_call_volume_ratio),
        (
            "putCallOpenInterestRatio",
            item.put_call_open_interest_ratio,
        ),
        ("underlyingPrice", item.underlying_price),
    ] {
        if let Some(value) = value
            && (!value.is_finite() || ((field.starts_with("putCall")) && value < 0.0))
        {
            return Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(
                format!(
                    "option underlying historical statistic {field} must be finite and non-negative"
                ),
            ));
        }
    }
    Ok(OptionUnderlyingHisStatisticItem {
        time,
        timestamp: item.timestamp,
        option_volume: item.option_volume,
        call_volume: item.call_volume,
        put_volume: item.put_volume,
        put_call_volume_ratio: item.put_call_volume_ratio,
        option_open_interest: item.option_open_interest,
        call_open_interest: item.call_open_interest,
        put_open_interest: item.put_open_interest,
        put_call_open_interest_ratio: item.put_call_open_interest_ratio,
        underlying_price: item.underlying_price,
    })
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

fn security_from_wire(market: &str, code: &str) -> OptionUnderlyingHisStatisticSecurity {
    OptionUnderlyingHisStatisticSecurity {
        market: market.to_owned(),
        code: code.to_owned(),
        quote_market: market.to_owned(),
        trade_market: market.to_owned(),
        instrument_id: format!("{market}.{code}"),
    }
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn parse_date(value: &str) -> Option<Date> {
    let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]").ok()?;
    Date::parse(value.trim(), &format).ok()
}

fn market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        _ => None,
    }
}

/// Decode the standard protojson representation of a `bytes nextPageKey`.
pub fn decode_next_page_key(
    value: &str,
) -> Result<Vec<u8>, OptionUnderlyingHisStatisticQueryError> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(Vec::new());
    }
    if value.len() > 1024 {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic cursor is too large".to_owned(),
        ));
    }
    let bytes = STANDARD.decode(value).map_err(|_| {
        OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic cursor must be base64".to_owned(),
        )
    })?;
    if bytes.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(
            "option underlying historical statistic cursor is too large".to_owned(),
        ));
    }
    Ok(bytes)
}

/// Encode a raw OpenD `nextPageKey` using the standard protojson base64 form.
pub fn encode_next_page_key(value: &[u8]) -> Option<String> {
    (!value.is_empty()).then(|| STANDARD.encode(value))
}

#[derive(Debug, Error)]
pub enum OptionUnderlyingHisStatisticQueryError {
    #[error("invalid OpenD option underlying historical statistic query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option underlying historical statistic session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionUnderlyingHisStatistic response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error(
        "OpenD Qot_GetOptionUnderlyingHisStatistic retType={ret_type} errCode={err_code}: {message}"
    )]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionUnderlyingHisStatistic response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option underlying historical statistic response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_underlying_his_statistic::{
        DailyStatistic, Response, S2c,
    };
    use prost::Message;

    fn query() -> OptionUnderlyingHisStatisticQuery {
        OptionUnderlyingHisStatisticQuery {
            market: 11,
            code: " aapl ".to_owned(),
            index_option_type: Some(1),
            begin_time: "2025-08-29".to_owned(),
            end_time: "2026-08-29".to_owned(),
            next_page_key: vec![1, 2, 3],
        }
    }

    fn owner() -> crate::trade_proto::qot_common::Security {
        crate::trade_proto::qot_common::Security {
            market: 11,
            code: "AAPL".to_owned(),
        }
    }

    #[test]
    fn request_uses_owner_date_range_and_wire_cursor() {
        let request = crate::trade_proto::qot_get_option_underlying_his_statistic::Request::decode(
            encode_request(&query()).as_slice(),
        )
        .expect("request");
        assert_eq!(request.c2s.owner.market, 11);
        assert_eq!(request.c2s.owner.code, "AAPL");
        assert_eq!(request.c2s.index_option_type, Some(1));
        assert_eq!(request.c2s.begin_time, "2025-08-29");
        assert_eq!(request.c2s.end_time, "2026-08-29");
        assert_eq!(request.c2s.next_page_key, Some(vec![1, 2, 3]));
    }

    #[test]
    fn response_maps_daily_statistics_and_cursor() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                owner: owner(),
                code: Some("AAPL".to_owned()),
                name: Some("Apple".to_owned()),
                statistic_list: vec![DailyStatistic {
                    time: "2026-08-29".to_owned(),
                    timestamp: Some(1_756_000_000.0),
                    option_volume: Some(180),
                    call_volume: 100,
                    put_volume: 80,
                    put_call_volume_ratio: Some(0.8),
                    option_open_interest: Some(1600),
                    call_open_interest: 900,
                    put_open_interest: 700,
                    put_call_open_interest_ratio: Some(0.777),
                    underlying_price: Some(225.0),
                }],
                next_page_key: Some(vec![4, 5, 6]),
            }),
        }
        .encode_to_vec();
        let snapshot = decode_response(&body, &query()).expect("snapshot");
        assert_eq!(snapshot.security.instrument_id, "US.AAPL");
        assert_eq!(snapshot.items[0].option_volume, Some(180));
        assert_eq!(snapshot.next_page_key, vec![4, 5, 6]);
    }

    #[test]
    fn rejects_invalid_scope_dates_values_and_missing_s2c() {
        assert!(matches!(
            validate_query(&OptionUnderlyingHisStatisticQuery {
                market: 11,
                code: "AAPL".to_owned(),
                index_option_type: Some(3),
                begin_time: "2026-09-01".to_owned(),
                end_time: "2026-08-01".to_owned(),
                next_page_key: Vec::new(),
            }),
            Err(OptionUnderlyingHisStatisticQueryError::InvalidQuery(_))
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
            Err(OptionUnderlyingHisStatisticQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                owner: owner(),
                statistic_list: vec![DailyStatistic {
                    time: "2026-08-29".to_owned(),
                    call_volume: -1,
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionUnderlyingHisStatisticQueryError::InvalidResponse(_))
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
