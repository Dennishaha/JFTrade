//! Typed OpenD corporate-action readers.
//!
//! OpenD exposes dividends, buybacks and stock splits as separate RPCs and
//! does not provide a common date filter.  The adapter therefore normalizes
//! the three responses to one neutral event type and applies the requested
//! date window locally.  No event is fabricated when OpenD omits its date or
//! numeric fields.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};

use prost::Message;
use serde::Serialize;
use thiserror::Error;
use time::Date;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CorporateActionKind {
    Dividends,
    Buybacks,
    StockSplits,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FutuCorporateActionsQuery {
    pub market: i32,
    pub code: String,
    pub kind: CorporateActionKind,
    pub from: Option<Date>,
    pub to: Option<Date>,
    pub next_key: Option<String>,
    pub limit: i32,
}

impl FutuCorporateActionsQuery {
    pub fn validate(&self) -> Result<(), FutuCorporateActionsQueryError> {
        if !matches!(self.market, 1 | 11 | 21 | 22) {
            return Err(FutuCorporateActionsQueryError::InvalidQuery(
                "corporate actions market must be HK, US, SH, or SZ".to_owned(),
            ));
        }
        let code = self.code.trim();
        if code.is_empty()
            || code.len() > 128
            || code.chars().any(|value| {
                value.is_whitespace()
                    || value.is_control()
                    || matches!(value, '.' | '/' | '\\' | '?' | '#')
            })
        {
            return Err(FutuCorporateActionsQueryError::InvalidQuery(
                "corporate actions code is invalid".to_owned(),
            ));
        }
        if self.from.zip(self.to).is_some_and(|(from, to)| from > to) {
            return Err(FutuCorporateActionsQueryError::InvalidQuery(
                "corporate actions from must not be after to".to_owned(),
            ));
        }
        if !(1..=50).contains(&self.limit) {
            return Err(FutuCorporateActionsQueryError::InvalidQuery(
                "corporate actions limit must be between 1 and 50".to_owned(),
            ));
        }
        if self
            .next_key
            .as_deref()
            .is_some_and(|key| key.len() > 256 || key.chars().any(char::is_control))
        {
            return Err(FutuCorporateActionsQueryError::InvalidQuery(
                "corporate actions nextKey is invalid".to_owned(),
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuCorporateAction {
    pub kind: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ex_date: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub amount: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ratio: Option<f64>,
    pub metadata: BTreeMap<String, String>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FutuCorporateActionsResult {
    pub events: Vec<FutuCorporateAction>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_key: Option<String>,
}

pub trait FutuCorporateActionsReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &FutuCorporateActionsQuery,
    ) -> Result<FutuCorporateActionsResult, FutuCorporateActionsQueryError>;
}

#[derive(Clone)]
pub struct OpenDCorporateActionsReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDCorporateActionsReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDCorporateActionsReader")
            .finish_non_exhaustive()
    }
}

impl OpenDCorporateActionsReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl FutuCorporateActionsReadPort for OpenDCorporateActionsReader {
    fn query(
        &self,
        query: &FutuCorporateActionsQuery,
    ) -> Result<FutuCorporateActionsResult, FutuCorporateActionsQueryError> {
        query.validate()?;
        let coordinator = self.coordinator.lock().map_err(|_| {
            FutuCorporateActionsQueryError::Session(OpenDSessionCoordinatorError::Closed)
        })?;
        let session = coordinator.session()?;
        match query.kind {
            CorporateActionKind::Dividends => {
                let body = session.managed_session().call(
                    crate::trade_proto::qot_get_corporate_actions_dividends::PROTOCOL_ID,
                    &encode_dividends(query),
                )?;
                decode_dividends(&body, query)
            }
            CorporateActionKind::Buybacks => {
                let body = session.managed_session().call(
                    crate::trade_proto::qot_get_corporate_actions_buybacks::PROTOCOL_ID,
                    &encode_buybacks(query),
                )?;
                decode_buybacks(&body, query)
            }
            CorporateActionKind::StockSplits => {
                let body = session.managed_session().call(
                    crate::trade_proto::qot_get_corporate_actions_stock_splits::PROTOCOL_ID,
                    &encode_splits(query),
                )?;
                decode_splits(&body, query)
            }
        }
    }
}

fn security(market: i32, code: &str) -> crate::trade_proto::qot_common::Security {
    crate::trade_proto::qot_common::Security {
        market,
        code: code.trim().to_ascii_uppercase(),
    }
}

fn encode_dividends(query: &FutuCorporateActionsQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_corporate_actions_dividends::{C2s, Request};
    Request {
        c2s: C2s {
            security: security(query.market, &query.code),
        },
    }
    .encode_to_vec()
}

fn encode_buybacks(query: &FutuCorporateActionsQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_corporate_actions_buybacks::{C2s, Request};
    Request {
        c2s: C2s {
            security: security(query.market, &query.code),
            next_key: query.next_key.clone(),
            num: Some(query.limit),
        },
    }
    .encode_to_vec()
}

fn encode_splits(query: &FutuCorporateActionsQuery) -> Vec<u8> {
    use crate::trade_proto::qot_get_corporate_actions_stock_splits::{C2s, Request};
    Request {
        c2s: C2s {
            security: security(query.market, &query.code),
            next_key: query.next_key.clone(),
            num: Some(query.limit),
        },
    }
    .encode_to_vec()
}

fn decode_dividends(
    body: &[u8],
    query: &FutuCorporateActionsQuery,
) -> Result<FutuCorporateActionsResult, FutuCorporateActionsQueryError> {
    use crate::trade_proto::qot_get_corporate_actions_dividends::Response;
    let response = Response::decode(body).map_err(FutuCorporateActionsQueryError::Decode)?;
    ensure_success(
        response.ret_type,
        response.err_code,
        response.ret_msg,
        "dividends",
    )?;
    let Some(s2c) = response.s2c else {
        return Err(FutuCorporateActionsQueryError::MissingS2c);
    };
    let events = s2c
        .dividend_list
        .into_iter()
        .map(map_dividend)
        .collect::<Result<Vec<_>, _>>()?
        .into_iter()
        .filter(|event| in_range(event, query))
        .collect();
    Ok(FutuCorporateActionsResult {
        events,
        next_key: None,
    })
}

fn decode_buybacks(
    body: &[u8],
    query: &FutuCorporateActionsQuery,
) -> Result<FutuCorporateActionsResult, FutuCorporateActionsQueryError> {
    use crate::trade_proto::qot_get_corporate_actions_buybacks::Response;
    let response = Response::decode(body).map_err(FutuCorporateActionsQueryError::Decode)?;
    ensure_success(
        response.ret_type,
        response.err_code,
        response.ret_msg,
        "buybacks",
    )?;
    let Some(s2c) = response.s2c else {
        return Err(FutuCorporateActionsQueryError::MissingS2c);
    };
    let mut events = Vec::new();
    events.extend(s2c.hk_buy_back_list.into_iter().map(map_hk_buyback));
    events.extend(s2c.a_buy_back_list.into_iter().map(map_a_buyback));
    events.retain(|event| in_range(event, query));
    let next_key = s2c
        .next_key
        .filter(|value| !value.is_empty() && value != "-1");
    Ok(FutuCorporateActionsResult { events, next_key })
}

fn decode_splits(
    body: &[u8],
    query: &FutuCorporateActionsQuery,
) -> Result<FutuCorporateActionsResult, FutuCorporateActionsQueryError> {
    use crate::trade_proto::qot_get_corporate_actions_stock_splits::Response;
    let response = Response::decode(body).map_err(FutuCorporateActionsQueryError::Decode)?;
    ensure_success(
        response.ret_type,
        response.err_code,
        response.ret_msg,
        "stock splits",
    )?;
    let Some(s2c) = response.s2c else {
        return Err(FutuCorporateActionsQueryError::MissingS2c);
    };
    let events = s2c
        .split_item_list
        .into_iter()
        .map(map_split)
        .collect::<Result<Vec<_>, _>>()?
        .into_iter()
        .filter(|event| in_range(event, query))
        .collect();
    let next_key = s2c
        .next_key
        .filter(|value| !value.is_empty() && value != "-1");
    Ok(FutuCorporateActionsResult { events, next_key })
}

fn ensure_success(
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<String>,
    operation: &str,
) -> Result<(), FutuCorporateActionsQueryError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(FutuCorporateActionsQueryError::Rejected {
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or_else(|| format!("OpenD {operation} request failed")),
    })
}

fn map_dividend(
    item: crate::trade_proto::qot_get_corporate_actions_dividends::DividendItem,
) -> Result<FutuCorporateAction, FutuCorporateActionsQueryError> {
    let mut metadata = BTreeMap::new();
    insert_text(&mut metadata, "pubDate", item.pub_date)?;
    insert_text(&mut metadata, "statement", item.statement)?;
    insert_text(&mut metadata, "process", item.process)?;
    insert_text(&mut metadata, "recordDate", item.record_date)?;
    insert_text(
        &mut metadata,
        "dividendPayableDate",
        item.dividend_payable_date,
    )?;
    insert_text(&mut metadata, "fiscalYear", item.fiscal_year)?;
    let ex_date = normalize_date(item.ex_date)?;
    Ok(FutuCorporateAction {
        kind: "dividend".to_owned(),
        ex_date,
        amount: None,
        ratio: None,
        metadata,
    })
}

fn map_hk_buyback(
    item: crate::trade_proto::qot_get_corporate_actions_buybacks::HkBuyBackItem,
) -> FutuCorporateAction {
    let mut metadata = BTreeMap::new();
    insert_optional_text(&mut metadata, "publDate", item.publ_date_str.clone());
    insert_optional_text(&mut metadata, "endDate", item.end_date_str);
    insert_optional_text(&mut metadata, "shareType", item.share_type);
    insert_optional_number(&mut metadata, "buyBackSum", item.buy_back_sum);
    FutuCorporateAction {
        kind: "buyback".to_owned(),
        ex_date: normalize_date_optional(item.publ_date_str),
        amount: finite_optional(item.buy_back_money, "buyBackMoney"),
        ratio: finite_optional(item.percentage, "percentage"),
        metadata,
    }
}

fn map_a_buyback(
    item: crate::trade_proto::qot_get_corporate_actions_buybacks::ABuyBackItem,
) -> FutuCorporateAction {
    let mut metadata = BTreeMap::new();
    insert_optional_text(&mut metadata, "changeDate", item.change_date_str.clone());
    insert_optional_text(&mut metadata, "eventProceDesc", item.event_proce_desc);
    insert_optional_text(&mut metadata, "shareType", item.share_type);
    insert_optional_number(&mut metadata, "buyBackSum", item.buy_back_sum);
    FutuCorporateAction {
        kind: "buyback".to_owned(),
        ex_date: normalize_date_optional(item.change_date_str)
            .or_else(|| normalize_date_optional(item.advance_date_str)),
        amount: finite_optional(item.buy_back_money, "buyBackMoney"),
        ratio: finite_optional(item.percentage, "percentage"),
        metadata,
    }
}

fn map_split(
    item: crate::trade_proto::qot_get_corporate_actions_stock_splits::StockSplitItem,
) -> Result<FutuCorporateAction, FutuCorporateActionsQueryError> {
    let mut metadata = BTreeMap::new();
    insert_text(
        &mut metadata,
        "announcementDate",
        item.dir_deci_pub_date_str,
    )?;
    insert_text(&mut metadata, "reformType", item.reform_type)?;
    insert_text(&mut metadata, "decisionDate", item.sm_deci_date_str)?;
    insert_text(&mut metadata, "eventStatus", item.event_status)?;
    insert_text(&mut metadata, "temporaryShareCode", item.temp_share_code)?;
    insert_text(
        &mut metadata,
        "temporaryShareName",
        item.temp_share_abbr_name,
    )?;
    let ex_date = normalize_date(item.ex_date_str)?;
    let ratio = item.rate.as_deref().and_then(parse_ratio).or_else(|| {
        item.shares_after_effect
            .zip(item.new_par_value)
            .and_then(|_| None)
    });
    Ok(FutuCorporateAction {
        kind: "split".to_owned(),
        ex_date,
        amount: finite_optional(item.new_par_value, "newParValue"),
        ratio,
        metadata,
    })
}

fn in_range(event: &FutuCorporateAction, query: &FutuCorporateActionsQuery) -> bool {
    let Some(ex_date) = event.ex_date.as_deref().and_then(|value| {
        Date::parse(
            value,
            &time::format_description::parse_borrowed::<3>("[year]-[month]-[day]").ok()?,
        )
        .ok()
    }) else {
        // The route contract only promises date-filtered events when OpenD
        // provides a usable date.  Keep undated rows for an unbounded query,
        // but never claim they match an explicit date window.
        return query.from.is_none() && query.to.is_none();
    };
    query.from.is_none_or(|from| ex_date >= from) && query.to.is_none_or(|to| ex_date <= to)
}

fn normalize_date(value: Option<String>) -> Result<Option<String>, FutuCorporateActionsQueryError> {
    let Some(value) = value else { return Ok(None) };
    let value = value.trim();
    if value.is_empty() {
        return Ok(None);
    }
    let normalized = value.replace('/', "-");
    let format = time::format_description::parse_borrowed::<3>("[year]-[month]-[day]")
        .map_err(|error| FutuCorporateActionsQueryError::InvalidResponse(error.to_string()))?;
    Date::parse(&normalized, &format)
        .map_err(|_| {
            FutuCorporateActionsQueryError::InvalidResponse(
                "OpenD corporate action date is not YYYY-MM-DD".to_owned(),
            )
        })?
        .format(&format)
        .map(Some)
        .map_err(|error| FutuCorporateActionsQueryError::InvalidResponse(error.to_string()))
}

fn normalize_date_optional(value: Option<String>) -> Option<String> {
    normalize_date(value).ok().flatten()
}

fn insert_text(
    target: &mut BTreeMap<String, String>,
    key: &str,
    value: Option<String>,
) -> Result<(), FutuCorporateActionsQueryError> {
    if let Some(value) = value {
        let value = value.trim().to_owned();
        if !value.is_empty() {
            target.insert(key.to_owned(), value);
        }
    }
    Ok(())
}

fn insert_optional_text(target: &mut BTreeMap<String, String>, key: &str, value: Option<String>) {
    if let Some(value) = value.filter(|value| !value.trim().is_empty()) {
        target.insert(key.to_owned(), value.trim().to_owned());
    }
}

fn insert_optional_number<T: ToString>(
    target: &mut BTreeMap<String, String>,
    key: &str,
    value: Option<T>,
) {
    if let Some(value) = value {
        target.insert(key.to_owned(), value.to_string());
    }
}

fn finite_optional(value: Option<f64>, field: &str) -> Option<f64> {
    value.filter(|value| value.is_finite()).or_else(|| {
        let _ = field;
        None
    })
}

fn parse_ratio(value: &str) -> Option<f64> {
    let value = value.trim();
    if let Ok(number) = value.parse::<f64>() {
        return number.is_finite().then_some(number);
    }
    let (left, right) = value.split_once(':')?;
    let left = left.trim().parse::<f64>().ok()?;
    let right = right.trim().parse::<f64>().ok()?;
    if left.is_finite() && right.is_finite() && right != 0.0 {
        Some(left / right)
    } else {
        None
    }
}

#[derive(Debug, Error)]
pub enum FutuCorporateActionsQueryError {
    #[error("invalid OpenD corporate actions query: {0}")]
    InvalidQuery(String),
    #[error("OpenD corporate actions session: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD corporate actions session call failed: {0}")]
    Call(#[from] crate::OpenDManagedSessionError),
    #[error("decode OpenD corporate actions response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD corporate actions retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD corporate actions response missing s2c")]
    MissingS2c,
    #[error("invalid OpenD corporate actions response: {0}")]
    InvalidResponse(String),
}
