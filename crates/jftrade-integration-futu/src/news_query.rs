//! Typed OpenD news search reader (`Qot_GetSearchNews/3263`).
//!
//! The generated protobuf messages remain private to the Futu integration
//! crate.  This module validates the request/response boundary and exposes a
//! small provider-neutral result consumed by the Rust product adapter.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FutuNewsQuery {
    pub keyword: String,
    pub max_count: i32,
    pub news_sub_type: Option<i32>,
}

impl Default for FutuNewsQuery {
    fn default() -> Self {
        Self {
            keyword: String::new(),
            max_count: 10,
            news_sub_type: None,
        }
    }
}

impl FutuNewsQuery {
    pub fn validate(&self) -> Result<(), FutuNewsQueryError> {
        let keyword = self.keyword.trim();
        if keyword.is_empty() || keyword.len() > 128 {
            return Err(FutuNewsQueryError::InvalidQuery(
                "news keyword must be between 1 and 128 characters".to_owned(),
            ));
        }
        if keyword
            .chars()
            .any(|value| value.is_control() || value == '\r' || value == '\n')
        {
            return Err(FutuNewsQueryError::InvalidQuery(
                "news keyword contains invalid characters".to_owned(),
            ));
        }
        if !(1..=50).contains(&self.max_count) {
            return Err(FutuNewsQueryError::InvalidQuery(
                "news maxCount must be between 1 and 50".to_owned(),
            ));
        }
        if self
            .news_sub_type
            .is_some_and(|value| !(0..=3).contains(&value))
        {
            return Err(FutuNewsQueryError::InvalidQuery(
                "news subType must be between 0 and 3".to_owned(),
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuNewsEntry {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub news_sub_type: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub source: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub published_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub view_count: Option<i64>,
    pub related_securities: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuNewsResult {
    pub entries: Vec<FutuNewsEntry>,
}

pub trait FutuNewsReadPort: Send + Sync + std::fmt::Debug {
    fn query(&self, query: &FutuNewsQuery) -> Result<FutuNewsResult, FutuNewsQueryError>;
}

#[derive(Clone)]
pub struct OpenDNewsReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDNewsReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDNewsReader")
            .finish_non_exhaustive()
    }
}

impl OpenDNewsReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl FutuNewsReadPort for OpenDNewsReader {
    fn query(&self, query: &FutuNewsQuery) -> Result<FutuNewsResult, FutuNewsQueryError> {
        query.validate()?;
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| FutuNewsQueryError::Session(OpenDSessionCoordinatorError::Closed))?;
        let session = coordinator.session()?;
        let body = session
            .managed_session()
            .call(
                crate::trade_proto::qot_get_search_news::PROTOCOL_ID,
                &encode_request(query),
            )
            .map_err(crate::OpenDSessionCoordinatorError::from)?;
        decode_response(&body)
    }
}

fn encode_request(query: &FutuNewsQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_search_news::{C2s, Request};
    Request {
        c2s: C2s {
            keyword: query.keyword.trim().to_owned(),
            max_count: Some(query.max_count),
            news_sub_type: query.news_sub_type,
        },
    }
    .encode_to_vec()
}

fn decode_response(body: &[u8]) -> Result<FutuNewsResult, FutuNewsQueryError> {
    use crate::trade_proto::qot_get_search_news::Response;
    let response = Response::decode(body).map_err(FutuNewsQueryError::Decode)?;
    if response.ret_type != 0 {
        return Err(FutuNewsQueryError::Rejected {
            ret_type: response.ret_type,
            err_code: response.err_code.unwrap_or_default(),
            message: response
                .ret_msg
                .unwrap_or_else(|| "OpenD news search request failed".to_owned()),
        });
    }
    let Some(s2c) = response.s2c else {
        return Err(FutuNewsQueryError::MissingS2c);
    };
    let entries = s2c
        .search_news_list
        .into_iter()
        .map(map_entry)
        .collect::<Result<Vec<_>, _>>()?;
    Ok(FutuNewsResult { entries })
}

fn map_entry(
    entry: crate::trade_proto::qot_get_search_news::SearchNews,
) -> Result<FutuNewsEntry, FutuNewsQueryError> {
    if entry
        .news_sub_type
        .is_some_and(|value| !(0..=3).contains(&value))
    {
        return Err(FutuNewsQueryError::InvalidResponse(
            "OpenD news entry has unsupported newsSubType".to_owned(),
        ));
    }
    if entry.view_count.is_some_and(|value| value < 0) {
        return Err(FutuNewsQueryError::InvalidResponse(
            "OpenD news entry viewCount must be non-negative".to_owned(),
        ));
    }
    let published_at = entry
        .publish_time
        .map(|value| normalize_publish_time(&value))
        .transpose()?
        .flatten();
    let related_securities = entry
        .related_securities
        .into_iter()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .collect();
    Ok(FutuNewsEntry {
        title: optional_text(entry.title),
        news_sub_type: entry.news_sub_type,
        source: optional_text(entry.source),
        published_at,
        view_count: entry.view_count,
        related_securities,
        url: optional_text(entry.url),
    })
}

fn optional_text(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

fn normalize_publish_time(value: &str) -> Result<Option<String>, FutuNewsQueryError> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(None);
    }
    if let Ok(timestamp) =
        time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339)
    {
        return timestamp
            .to_offset(time::UtcOffset::UTC)
            .format(&time::format_description::well_known::Rfc3339)
            .map(Some)
            .map_err(|error| FutuNewsQueryError::InvalidResponse(error.to_string()));
    }
    for format in [
        "[year]-[month]-[day] [hour]:[minute]:[second]",
        "[year]/[month]/[day] [hour]:[minute]:[second]",
        "[year]-[month]-[day]",
    ] {
        let Ok(description) = time::format_description::parse_borrowed::<3>(format) else {
            continue;
        };
        if let Ok(value) = time::PrimitiveDateTime::parse(value, &description) {
            return value
                .assume_utc()
                .format(&time::format_description::well_known::Rfc3339)
                .map(Some)
                .map_err(|error| FutuNewsQueryError::InvalidResponse(error.to_string()));
        }
        if let Ok(value) = time::Date::parse(value, &description) {
            return value
                .midnight()
                .assume_utc()
                .format(&time::format_description::well_known::Rfc3339)
                .map(Some)
                .map_err(|error| FutuNewsQueryError::InvalidResponse(error.to_string()));
        }
    }
    Err(FutuNewsQueryError::InvalidResponse(
        "OpenD news publishTime is not a valid timestamp".to_owned(),
    ))
}

#[derive(Debug, Error)]
pub enum FutuNewsQueryError {
    #[error("invalid OpenD news query: {0}")]
    InvalidQuery(String),
    #[error("OpenD news session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_GetSearchNews response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_GetSearchNews retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_GetSearchNews response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD news response: {0}")]
    InvalidResponse(String),
}
