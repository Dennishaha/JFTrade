//! Typed OpenD historical underlying-volatility reader
//! (`Qot_GetOptionUnderlyingHisVolatility/3304`).
//!
//! The protocol returns one underlying's daily implied and historical
//! volatility series.  Generated protobuf messages stay inside this crate;
//! callers receive broker-neutral security, item, and pagination DTOs.

use std::sync::{Arc, Mutex};

use base64::{Engine as _, engine::general_purpose::STANDARD};
use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const MAX_PAGE_KEY_BYTES: usize = 768;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct OptionUnderlyingHisVolatilityQuery {
    /// Public quote market: HK (1) or US (11).
    pub market: i32,
    pub code: String,
    pub index_option_type: Option<i32>,
    pub begin_time: String,
    pub end_time: String,
    /// Raw OpenD `nextPageKey`; an empty value requests the first page.
    pub next_page_key: Vec<u8>,
}

impl OptionUnderlyingHisVolatilityQuery {
    pub fn validate(&self) -> Result<(), OptionUnderlyingHisVolatilityQueryError> {
        validate_query(self)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisVolatilitySecurity {
    pub market: String,
    pub code: String,
    pub quote_market: String,
    pub trade_market: String,
    pub instrument_id: String,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisVolatilityItem {
    pub time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timestamp: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hv: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub underlying_price: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OptionUnderlyingHisVolatilitySnapshot {
    pub security: OptionUnderlyingHisVolatilitySecurity,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub items: Vec<OptionUnderlyingHisVolatilityItem>,
    /// Raw OpenD `nextPageKey`; an empty value means there is no next page.
    #[serde(skip)]
    pub next_page_key: Vec<u8>,
}

pub trait OptionUnderlyingHisVolatilityReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &OptionUnderlyingHisVolatilityQuery,
    ) -> Result<OptionUnderlyingHisVolatilitySnapshot, OptionUnderlyingHisVolatilityQueryError>;
}

#[derive(Clone)]
pub struct OpenDOptionUnderlyingHisVolatilityReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDOptionUnderlyingHisVolatilityReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDOptionUnderlyingHisVolatilityReader")
            .finish_non_exhaustive()
    }
}

impl OpenDOptionUnderlyingHisVolatilityReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl OptionUnderlyingHisVolatilityReadPort for OpenDOptionUnderlyingHisVolatilityReader {
    fn query(
        &self,
        query: &OptionUnderlyingHisVolatilityQuery,
    ) -> Result<OptionUnderlyingHisVolatilitySnapshot, OptionUnderlyingHisVolatilityQueryError>
    {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            OptionUnderlyingHisVolatilityQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        let response = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_option_underlying_his_volatility::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&response, query)
    }
}

fn validate_query(
    query: &OptionUnderlyingHisVolatilityQuery,
) -> Result<(), OptionUnderlyingHisVolatilityQueryError> {
    let market = market_label(query.market).ok_or_else(|| {
        OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility market must be HK (1) or US (11)".to_owned(),
        )
    })?;
    let code = query.code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            format!(
                "option underlying historical volatility code must be a {market} underlying code"
            ),
        ));
    }
    if let Some(index_option_type) = query.index_option_type
        && !matches!(index_option_type, 0..=2)
    {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility indexOptionType must be 0, 1, or 2".to_owned(),
        ));
    }
    let begin = parse_date(&query.begin_time).ok_or_else(|| {
        OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "beginTime must be YYYY-MM-DD".to_owned(),
        )
    })?;
    let end = parse_date(&query.end_time).ok_or_else(|| {
        OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "endTime must be YYYY-MM-DD".to_owned(),
        )
    })?;
    if end < begin {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility date range must be ordered".to_owned(),
        ));
    }
    if query.next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility nextPageKey is too large".to_owned(),
        ));
    }
    Ok(())
}

fn encode_request(query: &OptionUnderlyingHisVolatilityQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_option_underlying_his_volatility::{C2s, Request};
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
    query: &OptionUnderlyingHisVolatilityQuery,
) -> Result<OptionUnderlyingHisVolatilitySnapshot, OptionUnderlyingHisVolatilityQueryError> {
    use crate::trade_proto::qot_get_option_underlying_his_volatility::Response;
    let response =
        Response::decode(body).map_err(OptionUnderlyingHisVolatilityQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(OptionUnderlyingHisVolatilityQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response.ret_msg.unwrap_or_else(|| {
                "OpenD option underlying historical volatility request failed".to_owned()
            }),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(OptionUnderlyingHisVolatilityQueryError::MissingS2c);
    };
    let expected_code = query.code.trim().to_ascii_uppercase();
    if s2c.owner.market != query.market
        || s2c.owner.code.trim().to_ascii_uppercase() != expected_code
    {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
            "option underlying historical volatility owner does not match query".to_owned(),
        ));
    }
    let owner_code = validate_code(&s2c.owner.code).ok_or_else(|| {
        OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
            "option underlying historical volatility owner code is invalid".to_owned(),
        )
    })?;
    let code = optional_text(s2c.code);
    if let Some(value) = code.as_deref()
        && value.to_ascii_uppercase() != expected_code
    {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
            "option underlying historical volatility response code does not match query".to_owned(),
        ));
    }
    let next_page_key = s2c.next_page_key.unwrap_or_default();
    if next_page_key.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
            "option underlying historical volatility nextPageKey is too large".to_owned(),
        ));
    }
    let items = s2c
        .volatility_list
        .into_iter()
        .map(map_item)
        .collect::<Result<Vec<_>, _>>()?;
    let market = market_label(query.market).expect("query validation ensures market");
    Ok(OptionUnderlyingHisVolatilitySnapshot {
        security: security_from_wire(market, &owner_code),
        code,
        name: optional_text(s2c.name),
        items,
        next_page_key,
    })
}

fn map_item(
    item: crate::trade_proto::qot_get_option_underlying_his_volatility::VolatilityItem,
) -> Result<OptionUnderlyingHisVolatilityItem, OptionUnderlyingHisVolatilityQueryError> {
    let time = item.time.trim().to_owned();
    if parse_date(&time).is_none() {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
            "option underlying historical volatility item time must be YYYY-MM-DD".to_owned(),
        ));
    }
    for (field, value) in [
        ("timestamp", item.timestamp),
        ("iv", item.iv),
        ("hv", item.hv),
        ("underlyingPrice", item.underlying_price),
    ] {
        if let Some(value) = value
            && !value.is_finite()
        {
            return Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(
                format!("option underlying historical volatility {field} must be finite"),
            ));
        }
    }
    Ok(OptionUnderlyingHisVolatilityItem {
        time,
        timestamp: item.timestamp,
        iv: item.iv,
        hv: item.hv,
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

fn security_from_wire(market: &str, code: &str) -> OptionUnderlyingHisVolatilitySecurity {
    OptionUnderlyingHisVolatilitySecurity {
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
/// Empty cursors represent the first page and are intentionally accepted.
pub fn decode_next_page_key(
    value: &str,
) -> Result<Vec<u8>, OptionUnderlyingHisVolatilityQueryError> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(Vec::new());
    }
    if value.len() > 1024 {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility cursor is too large".to_owned(),
        ));
    }
    let bytes = STANDARD.decode(value).map_err(|_| {
        OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility cursor must be base64".to_owned(),
        )
    })?;
    if bytes.len() > MAX_PAGE_KEY_BYTES {
        return Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            "option underlying historical volatility cursor is too large".to_owned(),
        ));
    }
    Ok(bytes)
}

/// Encode a raw OpenD `nextPageKey` using the standard protojson base64 form.
pub fn encode_next_page_key(value: &[u8]) -> Option<String> {
    (!value.is_empty()).then(|| STANDARD.encode(value))
}

#[derive(Debug, Error)]
pub enum OptionUnderlyingHisVolatilityQueryError {
    #[error("invalid OpenD option underlying historical volatility query: {0}")]
    InvalidQuery(String),
    #[error("OpenD option underlying historical volatility session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetOptionUnderlyingHisVolatility response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error(
        "OpenD Qot_GetOptionUnderlyingHisVolatility retType={ret_type} errCode={err_code}: {message}"
    )]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetOptionUnderlyingHisVolatility response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD option underlying historical volatility response: {0}")]
    InvalidResponse(String),
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trade_proto::qot_get_option_underlying_his_volatility::{
        Response, S2c, VolatilityItem,
    };
    use crate::{decode_frame, encode_frame};

    fn query() -> OptionUnderlyingHisVolatilityQuery {
        OptionUnderlyingHisVolatilityQuery {
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
        let request =
            crate::trade_proto::qot_get_option_underlying_his_volatility::Request::decode(
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
    fn framed_response_preserves_series_metadata_and_cursor() {
        let body = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                owner: owner(),
                code: Some("AAPL".to_owned()),
                name: Some("Apple".to_owned()),
                volatility_list: vec![VolatilityItem {
                    time: "2026-08-29".to_owned(),
                    timestamp: Some(1_756_000_000.0),
                    iv: Some(25.0),
                    hv: Some(20.0),
                    underlying_price: Some(225.0),
                }],
                next_page_key: Some(vec![4, 5, 6]),
            }),
        }
        .encode_to_vec();
        let frame = encode_frame(
            crate::trade_proto::qot_get_option_underlying_his_volatility::PROTOCOL_ID,
            9,
            &body,
        )
        .expect("frame");
        let decoded = decode_frame(&frame).expect("decode frame");
        let snapshot = decode_response(&decoded.body, &query()).expect("snapshot");
        assert_eq!(decoded.header.proto_id, 3304);
        assert_eq!(snapshot.security.instrument_id, "US.AAPL");
        assert_eq!(snapshot.code.as_deref(), Some("AAPL"));
        assert_eq!(snapshot.items[0].iv, Some(25.0));
        assert_eq!(snapshot.next_page_key, vec![4, 5, 6]);
    }

    #[test]
    fn rejects_invalid_scope_dates_values_and_missing_s2c() {
        assert!(matches!(
            validate_query(&OptionUnderlyingHisVolatilityQuery {
                market: 11,
                code: "AAPL".to_owned(),
                index_option_type: Some(3),
                begin_time: "2026-09-01".to_owned(),
                end_time: "2026-08-01".to_owned(),
                next_page_key: Vec::new(),
            }),
            Err(OptionUnderlyingHisVolatilityQueryError::InvalidQuery(_))
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
            Err(OptionUnderlyingHisVolatilityQueryError::MissingS2c)
        ));
        let invalid = Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(S2c {
                owner: owner(),
                volatility_list: vec![VolatilityItem {
                    time: "2026-08-29".to_owned(),
                    iv: Some(f64::NAN),
                    ..Default::default()
                }],
                ..Default::default()
            }),
        }
        .encode_to_vec();
        assert!(matches!(
            decode_response(&invalid, &query()),
            Err(OptionUnderlyingHisVolatilityQueryError::InvalidResponse(_))
        ));
    }

    #[test]
    fn cursor_uses_protojson_standard_base64() {
        let encoded = encode_next_page_key(&[1, 2, 3]).expect("cursor");
        assert_eq!(encoded, "AQID");
        assert_eq!(
            decode_next_page_key(&encoded).expect("decoded"),
            vec![1, 2, 3]
        );
        assert!(decode_next_page_key("not base64!").is_err());
    }

    #[test]
    fn date_parser_rejects_invalid_calendar_days() {
        assert!(parse_date("2026-02-30").is_none());
        assert_eq!(
            parse_date("2026-08-29").expect("date") - parse_date("2026-08-28").expect("date"),
            time::Duration::days(1)
        );
    }
}
