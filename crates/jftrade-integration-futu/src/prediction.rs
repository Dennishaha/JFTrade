//! Typed OpenD readers for the event-contract (prediction) market surface.
//!
//! The prediction API is deliberately kept behind this adapter.  The engine
//! receives ordinary JSON values, while this module owns protobuf encoding,
//! response validation and the OpenD session boundary.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};

use prost::Message;
use serde_json::{Value, json};
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError};

const EVENT_MARKET: i32 = 101;

#[derive(Clone, Debug, Error, PartialEq, Eq)]
pub enum PredictionMarketReadError {
    #[error("invalid prediction request: {0}")]
    InvalidQuery(String),
    #[error("prediction OpenD request rejected: {operation}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("decode prediction OpenD {operation} response: {message}")]
    Decode {
        operation: &'static str,
        message: String,
    },
    #[error("prediction OpenD session unavailable: {0}")]
    Session(String),
    #[error("prediction OpenD request failed: {0}")]
    Transport(String),
}

pub trait PredictionMarketReadPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str, query: &str) -> Result<Value, PredictionMarketReadError>;
}

pub trait PredictionMarketSubscriptionPort: Send + Sync + std::fmt::Debug {
    fn subscribe(
        &self,
        code: &str,
        data_types: &[String],
    ) -> Result<Value, PredictionMarketReadError>;
    fn unsubscribe(&self, code: &str) -> Result<Value, PredictionMarketReadError>;
}

pub trait PredictionComboQuotePort: Send + Sync + std::fmt::Debug {
    fn quote(&self, payload: &Value) -> Result<Value, PredictionMarketReadError>;
}

#[derive(Clone)]
pub struct OpenDPredictionMarketReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDPredictionMarketReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDPredictionMarketReader")
            .finish_non_exhaustive()
    }
}

impl OpenDPredictionMarketReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }

    fn call(&self, protocol: u32, operation: &'static str, body: Vec<u8>) -> Result<Vec<u8>, PredictionMarketReadError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| PredictionMarketReadError::Session("coordinator lock poisoned".to_owned()))?;
        let session = coordinator
            .session()
            .map_err(|error| PredictionMarketReadError::Session(error.to_string()))?;
        session
            .managed_session()
            .call(protocol, &body)
            .map_err(|error| PredictionMarketReadError::Transport(error.to_string()))
            .map(|body| {
                // Keep the operation in one place for all decoders while
                // allowing protocol-specific response types below.
                let _ = operation;
                body
            })
    }

    fn call_response<R: Message + Default>(
        &self,
        protocol: u32,
        operation: &'static str,
        body: Vec<u8>,
    ) -> Result<R, PredictionMarketReadError> {
        let response = self.call(protocol, operation, body)?;
        R::decode(response.as_slice()).map_err(|error| PredictionMarketReadError::Decode {
            operation,
            message: error.to_string(),
        })
    }
}

impl PredictionMarketReadPort for OpenDPredictionMarketReader {
    fn read(&self, path: &str, query: &str) -> Result<Value, PredictionMarketReadError> {
        let query = Query::parse(query)?;
        let (operation, code_or_event) = route_operation(path)?;
        match operation {
            "categories" => self.categories(&query),
            "competitions" => self.competitions(&query),
            "series" => self.series(&query),
            "events" => self.events(&query),
            "contracts" => self.contracts(code_or_event, &query),
            "milestones" => self.milestones(code_or_event, &query),
            "snapshot" => self.snapshot(code_or_event),
            "order_book" => self.order_book(code_or_event, &query),
            "candles" => self.candles(code_or_event, &query),
            "historical" => self.historical(code_or_event, &query),
            "ticks" => self.ticks(code_or_event, &query),
            "eligible_events" => self.combo_events(&query),
            _ => Err(PredictionMarketReadError::InvalidQuery(
                "unsupported prediction operation".to_owned(),
            )),
        }
    }
}

impl PredictionMarketSubscriptionPort for OpenDPredictionMarketReader {
    fn subscribe(
        &self,
        code: &str,
        data_types: &[String],
    ) -> Result<Value, PredictionMarketReadError> {
        self.update_subscription(code, data_types, true)
    }

    fn unsubscribe(&self, code: &str) -> Result<Value, PredictionMarketReadError> {
        self.update_subscription(code, &[], false)
    }
}

impl PredictionComboQuotePort for OpenDPredictionMarketReader {
    fn quote(&self, payload: &Value) -> Result<Value, PredictionMarketReadError> {
        let request = ComboQuoteRequest::parse(payload)?;
        let response = self.call_response::<crate::trade_proto::qot_get_event_contract_combo_rfq::Response>(
            crate::trade_proto::qot_get_event_contract_combo_rfq::PROTOCOL_ID,
            "Qot_GetEventContractComboRfq",
            request.encode(),
        )?;
        ensure_ok(
            "Qot_GetEventContractComboRfq",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let s2c = response.s2c.ok_or_else(|| PredictionMarketReadError::Decode {
            operation: "Qot_GetEventContractComboRfq",
            message: "response missing s2c".to_owned(),
        })?;
        let legs = s2c
            .combo_leg_list
            .iter()
            .map(combo_leg_value)
            .collect::<Vec<_>>();
        Ok(json!({
            "entries": legs,
            "metadata": {
                "quoteId": s2c.quote_id,
                "bidPrice": s2c.bid_price,
                "askPrice": s2c.ask_price,
                "shouldRetry": s2c.should_retry,
                "mvc": request.mvc,
            }
        }))
    }
}

#[derive(Debug, Default)]
struct Query {
    values: BTreeMap<String, String>,
}

impl Query {
    fn parse(raw: &str) -> Result<Self, PredictionMarketReadError> {
        let mut values = BTreeMap::new();
        for pair in raw.split('&').filter(|value| !value.is_empty()) {
            let (key, value) = pair.split_once('=').unwrap_or((pair, ""));
            let key = decode(key)?;
            let value = decode(value)?;
            if key.len() > 64 || value.len() > 4096 {
                return Err(invalid("prediction query value is too long"));
            }
            values.insert(key, value);
        }
        Ok(Self { values })
    }

    fn get(&self, key: &str) -> Option<&str> {
        self.values.get(key).map(String::as_str)
    }

    fn number(&self, key: &str, default: i32, min: i32, max: i32) -> Result<i32, PredictionMarketReadError> {
        let Some(value) = self.get(key).filter(|value| !value.trim().is_empty()) else {
            return Ok(default);
        };
        let parsed = value
            .trim()
            .parse::<i32>()
            .map_err(|_| invalid(&format!("{key} must be an integer")))?;
        if !(min..=max).contains(&parsed) {
            return Err(invalid(&format!("{key} must be between {min} and {max}")));
        }
        Ok(parsed)
    }
}

fn decode(value: &str) -> Result<String, PredictionMarketReadError> {
    let mut bytes = Vec::with_capacity(value.len());
    let mut chars = value.as_bytes().iter().copied();
    while let Some(byte) = chars.next() {
        match byte {
            b'+' => bytes.push(b' '),
            b'%' => {
                let hi = chars.next().and_then(hex_digit);
                let lo = chars.next().and_then(hex_digit);
                let (Some(hi), Some(lo)) = (hi, lo) else {
                    return Err(invalid("invalid prediction query encoding"));
                };
                bytes.push((hi << 4) | lo);
            }
            value if value.is_ascii() => bytes.push(value),
            _ => return Err(invalid("invalid prediction query encoding")),
        }
    }
    String::from_utf8(bytes).map_err(|_| invalid("invalid prediction query encoding"))
}

fn hex_digit(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        b'A'..=b'F' => Some(value - b'A' + 10),
        _ => None,
    }
}

fn route_operation(path: &str) -> Result<(&'static str, Option<&str>), PredictionMarketReadError> {
    if path == "/api/v1/market-data/prediction/categories" {
        return Ok(("categories", None));
    }
    if path == "/api/v1/market-data/prediction/competitions" {
        return Ok(("competitions", None));
    }
    if path == "/api/v1/market-data/prediction/series" {
        return Ok(("series", None));
    }
    if path == "/api/v1/market-data/prediction/events" {
        return Ok(("events", None));
    }
    if path == "/api/v1/market-data/prediction/combos/eligible-events" {
        return Ok(("eligible_events", None));
    }
    let event_prefix = "/api/v1/market-data/prediction/events/";
    if let Some(value) = path.strip_prefix(event_prefix) {
        let code = value
            .strip_suffix("/contracts")
            .filter(|value| !value.is_empty() && !value.contains('/'))
            .ok_or_else(|| invalid("eventId is invalid"))?;
        return Ok(("contracts", Some(code)));
    }
    let contract_prefix = "/api/v1/market-data/prediction/contracts/";
    let Some(value) = path.strip_prefix(contract_prefix) else {
        return Err(invalid("unsupported prediction read route"));
    };
    let (code, operation) = value
        .split_once('/')
        .filter(|(code, operation)| !code.is_empty() && !operation.is_empty())
        .ok_or_else(|| invalid("contract code is invalid"))?;
    let operation = match operation {
        "snapshot" => "snapshot",
        "order-book" => "order_book",
        "candles" => "candles",
        "candles/history" => "historical",
        "ticks" => "ticks",
        "milestones" => "milestones",
        _ => return Err(invalid("unsupported prediction read route")),
    };
    Ok((operation, Some(code)))
}

fn code(value: Option<&str>, label: &str) -> Result<String, PredictionMarketReadError> {
    let value = value
        .ok_or_else(|| invalid(&format!("{label} is required")))?
        .trim();
    let value = value.strip_prefix("US.").unwrap_or(value);
    if value.is_empty()
        || value.len() > 512
        || value
            .chars()
            .any(|ch| ch.is_whitespace() || ch.is_control() || matches!(ch, '/' | '\\' | '?' | '#'))
    {
        return Err(invalid(&format!("{label} is invalid")));
    }
    Ok(value.to_ascii_uppercase())
}

fn security(value: &str) -> crate::trade_proto::qot_common::Security {
    crate::trade_proto::qot_common::Security {
        market: EVENT_MARKET,
        code: value.to_owned(),
    }
}

fn invalid(message: &str) -> PredictionMarketReadError {
    PredictionMarketReadError::InvalidQuery(message.to_owned())
}

fn ensure_ok(
    operation: &'static str,
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<&str>,
) -> Result<(), PredictionMarketReadError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(PredictionMarketReadError::Rejected {
        operation,
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or("OpenD request failed").to_owned(),
    })
}

impl OpenDPredictionMarketReader {
    fn categories(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_category::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3434,
            "Qot_GetEventContractCategory",
            Request {
                c2s: C2s {
                    category: query.get("category").filter(|value| !value.is_empty()).map(str::to_owned),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractCategory", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.category_list.into_iter().map(|item| json!({
            "category": item.category,
            "categoryName": item.category_name,
            "tags": item.tags,
        })).collect()).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn competitions(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_filter_competition::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3435,
            "Qot_FilterCompetition",
            Request { c2s: C2s {
                category: query.get("category").filter(|value| !value.is_empty()).map(str::to_owned),
                tag: query.get("tag").filter(|value| !value.is_empty()).map(str::to_owned),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_FilterCompetition", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.tag_filter_list.into_iter().map(|item| json!({
            "category": item.category,
            "tag": item.tag,
            "competitionList": item.competition_list,
            "scopeList": item.scope_list,
        })).collect()).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn series(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_series_list::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3436,
            "Qot_GetEventContractSeriesList",
            Request { c2s: C2s {
                category: query.get("category").filter(|value| !value.is_empty()).map(str::to_owned),
                tag: query.get("tag").filter(|value| !value.is_empty()).map(str::to_owned),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractSeriesList", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.series_list.into_iter().map(|item| json!({
            "seriesSecurity": security_value(&item.series_security),
            "seriesName": item.series_name,
            "category": item.category,
            "tags": item.tags,
            "frequency": item.frequency,
        })).collect()).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn events(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_event_list::{C2s, Request, Response};
        let series = code(query.get("seriesId").or_else(|| query.get("series")), "seriesId")?;
        let response = self.call_response::<Response>(
            3437,
            "Qot_GetEventContractEventList",
            Request { c2s: C2s {
                series: security(&series),
                status: query.get("status").and_then(parse_status),
                next_page: query.get("cursor").filter(|value| !value.is_empty()).map(str::to_owned),
                count: Some(query.number("pageSize", 100, 1, 300)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractEventList", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let (entries, next) = response.s2c.map(|s2c| (
            s2c.event_list.into_iter().map(event_value).collect(),
            s2c.next_page,
        )).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn contracts(&self, event_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract::{C2s, Request, Response};
        let event = code(event_id, "eventId")?;
        let response = self.call_response::<Response>(
            3438,
            "Qot_GetEventContract",
            Request { c2s: C2s {
                event: security(&event),
                next_page: query.get("cursor").filter(|value| !value.is_empty()).map(str::to_owned),
                count: Some(query.number("pageSize", 100, 1, 1000)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContract", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let (entries, next) = response.s2c.map(|s2c| (
            s2c.contract_list.into_iter().map(contract_value).collect(),
            s2c.next_page,
        )).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn milestones(&self, contract_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_milestone_list::{C2s, Request, Response};
        let event = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3439,
            "Qot_GetEventContractMilestoneList",
            Request { c2s: C2s {
                category: query.get("category").filter(|value| !value.is_empty()).map(str::to_owned),
                competition: query.get("competition").filter(|value| !value.is_empty()).map(str::to_owned),
                related_event: Some(security(&event)),
                next_page: query.get("cursor").filter(|value| !value.is_empty()).map(str::to_owned),
                count: Some(query.number("pageSize", 100, 1, 300)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractMilestoneList", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let (entries, next) = response.s2c.map(|s2c| (
            s2c.milestone_list.into_iter().map(milestone_value).collect(),
            s2c.next_page,
        )).unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn snapshot(&self, contract_id: Option<&str>) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_snapshot::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3445,
            "Qot_GetEventContractSnapshot",
            Request { c2s: C2s { security_list: vec![security(&contract)] }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractSnapshot", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.snapshot_list.into_iter().map(snapshot_value).collect()).unwrap_or_default();
        Ok(feature_result("prediction.snapshot", entries, None))
    }

    fn order_book(&self, contract_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_order_book::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3446,
            "Qot_GetEventContractOrderBook",
            Request { c2s: C2s {
                security: security(&contract),
                num: query.number("depth", 10, 1, 100)?,
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractOrderBook", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.order_book_list.into_iter().map(order_book_value).collect()).unwrap_or_default();
        Ok(feature_result("prediction.depth", entries, None))
    }

    fn candles(&self, contract_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_kline::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3447,
            "Qot_GetEventContractKline",
            Request { c2s: C2s {
                security: security(&contract),
                kline_source: Some(1),
                pre_side: None,
                ktype: Some(parse_kline_type(query.get("period").or_else(|| query.get("interval")))?),
                max_count: Some(query.number("pageSize", 100, 1, 1000)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractKline", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.kline_list.into_iter().map(kline_value).collect()).unwrap_or_default();
        Ok(feature_result("prediction.history", entries, None))
    }

    fn historical(&self, contract_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_request_history_event_contract_kl::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let begin = query.get("from").or_else(|| query.get("beginTime")).ok_or_else(|| invalid("from is required"))?;
        let end = query.get("to").or_else(|| query.get("endTime")).ok_or_else(|| invalid("to is required"))?;
        if begin.is_empty() || end.is_empty() || begin.len() > 64 || end.len() > 64 {
            return Err(invalid("historical time range is invalid"));
        }
        let response = self.call_response::<Response>(
            3456,
            "Qot_RequestHistoryEventContractKL",
            Request { c2s: C2s {
                security: security(&contract),
                kline_source: Some(1),
                pre_side: None,
                kl_type: parse_kline_type(query.get("period").or_else(|| query.get("interval")))?,
                begin_time: begin.to_owned(),
                end_time: end.to_owned(),
                max_ack_kl_num: Some(query.number("pageSize", 100, 1, 1000)?),
                next_req_key: None,
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_RequestHistoryEventContractKL", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let (entries, next) = response.s2c.map(|s2c| (
            s2c.kline_list.into_iter().map(kline_value).collect(),
            s2c.next_req_key.map(|key| bytes_to_hex(&key)),
        )).unwrap_or_default();
        Ok(feature_result("prediction.history", entries, next))
    }

    fn ticks(&self, contract_id: Option<&str>, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_ticker::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3448,
            "Qot_GetEventContractTicker",
            Request { c2s: C2s {
                security: security(&contract),
                count: Some(query.number("pageSize", 30, 1, 1000)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractTicker", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let entries = response.s2c.map(|s2c| s2c.ticker_list.into_iter().map(ticker_value).collect()).unwrap_or_default();
        Ok(feature_result("prediction.history", entries, None))
    }

    fn combo_events(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_combo_list::{C2s, Request, Response};
        let series = query.get("seriesId").or_else(|| query.get("series")).map(|value| code(Some(value), "seriesId")).transpose()?;
        let response = self.call_response::<Response>(
            3453,
            "Qot_GetEventContractComboList",
            Request { c2s: C2s {
                category: query.get("category").filter(|value| !value.is_empty()).map(str::to_owned),
                competition: query.get("competition").filter(|value| !value.is_empty()).map(str::to_owned),
                series: series.as_deref().map(security),
                next_page: query.get("cursor").filter(|value| !value.is_empty()).map(str::to_owned),
                count: Some(query.number("pageSize", 100, 1, 300)?),
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_GetEventContractComboList", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        let (entries, next) = response.s2c.map(|s2c| (
            s2c.combo_event_list.into_iter().map(combo_event_value).collect(),
            s2c.next_page,
        )).unwrap_or_default();
        Ok(feature_result("prediction.combo_eligible", entries, next))
    }

    fn update_subscription(&self, code_value: &str, data_types: &[String], subscribe: bool) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_sub_event_contract::{C2s, Request, Response};
        let contract = code(Some(code_value), "code")?;
        let mut types = Vec::new();
        for data_type in data_types {
            match data_type.trim().to_ascii_uppercase().as_str() {
                "ORDER_BOOK" => types.push(2),
                "KLINE" => types.push(11),
                "TICKER" | "TICKS" => types.push(4),
                value => return Err(invalid(&format!("unsupported event contract subscription type {value:?}"))),
            }
        }
        if subscribe && types.is_empty() {
            return Err(invalid("dataTypes must not be empty"));
        }
        let response = self.call_response::<Response>(
            3455,
            "Qot_SubEventContract",
            Request { c2s: C2s {
                security_list: vec![security(&contract)],
                sub_type_list: types,
                is_sub_or_un_sub: subscribe,
                is_reg_or_un_reg_push: Some(subscribe),
                is_first_push: Some(subscribe),
                is_unsub_all: None,
                kline_source: Vec::new(),
                header: None,
            }}.encode_to_vec(),
        )?;
        ensure_ok("Qot_SubEventContract", response.ret_type, response.err_code, response.ret_msg.as_deref())?;
        Ok(json!({"instrumentId": format!("US.{contract}"), "dataTypes": data_types, "subscribed": subscribe}))
    }
}

fn parse_status(value: &str) -> Option<i32> {
    match value.trim().to_ascii_lowercase().as_str() {
        "open" | "active" => Some(1),
        "closed" | "settled" => Some(2),
        "all" | "" => None,
        value => value.parse().ok(),
    }
}

fn parse_kline_type(value: Option<&str>) -> Result<i32, PredictionMarketReadError> {
    match value.unwrap_or("1m").trim().to_ascii_lowercase().as_str() {
        "1m" | "1min" => Ok(1),
        "5m" | "5min" => Ok(6),
        "1h" | "60m" | "60min" => Ok(9),
        "1d" | "day" => Ok(2),
        _ => Err(invalid("prediction candles period must be 1m, 5m, 1h, or 1d")),
    }
}

fn feature_result(feature: &str, entries: Vec<Value>, next: Option<String>) -> Value {
    let has_more = next.is_some();
    json!({
        "asOf": now_rfc3339(),
        "entries": entries,
        "nextCursor": next,
        "hasMore": has_more,
        "total": entries.len(),
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": feature,
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": now_rfc3339(),
        },
    })
}

fn now_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn security_value(value: &crate::trade_proto::qot_common::Security) -> Value {
    json!({"market": value.market, "code": value.code, "instrumentId": format!("US.{}", value.code)})
}

fn event_value(item: crate::trade_proto::qot_get_event_contract_event_list::EventItem) -> Value {
    json!({
        "eventSecurity": security_value(&item.event_security),
        "eventName": item.event_name,
        "eventSubName": item.event_sub_name,
        "status": item.status,
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "startDate": item.start_date,
        "endDate": item.end_date,
        "category": item.category,
        "tags": item.tags,
        "mutuallyExclusive": item.mutually_exclusive,
        "competition": item.competition,
        "competitionScope": item.competition_scope,
    })
}

fn contract_value(item: crate::trade_proto::qot_get_event_contract::ContractItem) -> Value {
    json!({
        "contractSecurity": security_value(&item.contract_security),
        "eventSecurity": item.event_security.as_ref().map(security_value),
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "contractType": item.contract_type,
        "title": item.title,
        "yesSubTitle": item.yes_sub_title,
        "openTime": item.open_time,
        "closeTime": item.close_time,
        "determinationTime": item.determination_time,
        "settledTime": item.settled_time,
        "latestExpirationTime": item.latest_expiration_time,
        "status": item.status,
        "result": item.result,
        "settlementValue": item.settlement_value,
        "expirationValue": item.expiration_value,
        "volume": item.volume,
        "canCloseEarly": item.can_close_early,
        "tickSize": item.tick_size,
        "category": item.category,
        "tag": item.tag,
    })
}

fn milestone_value(item: crate::trade_proto::qot_get_event_contract_milestone_list::MilestoneItem) -> Value {
    json!({
        "milestoneSecurity": security_value(&item.milestone_security),
        "title": item.title,
        "category": item.category,
        "type": item.r#type,
        "startDate": item.start_date,
        "endDate": item.end_date,
        "primaryEventSecurity": item.primary_event_security.as_ref().map(security_value),
        "relatedEventList": item.related_event_list.iter().map(security_value).collect::<Vec<_>>(),
        "notificationMessage": item.notification_message,
    })
}

fn snapshot_value(item: crate::trade_proto::qot_get_event_contract_snapshot::SnapshotItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "name": item.name,
        "eventCode": item.event_code.as_ref().map(security_value),
        "yesSubTitle": item.yes_sub_title,
        "noSubTitle": item.no_sub_title,
        "status": item.status,
        "price": item.price,
        "cumulativeVolume": item.cumulative_volume,
        "yesBid": item.yes_bid,
        "yesBidSize": item.yes_bid_size,
        "yesAsk": item.yes_ask,
        "yesAskSize": item.yes_ask_size,
        "noBid": item.no_bid,
        "noBidSize": item.no_bid_size,
        "noAsk": item.no_ask,
        "noAskSize": item.no_ask_size,
        "lastTradeTime": item.last_trade_time,
        "volume24h": item.volume_24h,
        "openInterest": item.open_interest,
    })
}

fn order_book_value(item: crate::trade_proto::qot_get_event_contract_order_book::OrderBookItem) -> Value {
    let levels = |items: Vec<crate::trade_proto::qot_get_event_contract_order_book::OrderBookLevel>| {
        items.into_iter().map(|level| json!({"price": level.price, "size": level.size})).collect::<Vec<_>>()
    };
    json!({
        "code": security_value(&item.code),
        "yesBids": levels(item.yes_bids),
        "yesAsks": levels(item.yes_asks),
        "noBids": levels(item.no_bids),
        "noAsks": levels(item.no_asks),
    })
}

fn kline_value(item: crate::trade_proto::qot_get_event_contract_kline::KlineItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "preSide": item.pre_side,
        "name": item.name,
        "klineList": item.kline_list.into_iter().map(|point| json!({
            "timeKey": point.time_key,
            "open": point.open,
            "high": point.high,
            "low": point.low,
            "close": point.close,
            "volume": point.volume,
        })).collect::<Vec<_>>(),
    })
}

fn ticker_value(item: crate::trade_proto::qot_get_event_contract_ticker::TickerItem) -> Value {
    json!({
        "code": security_value(&item.code),
        "tickerList": item.ticker_list.into_iter().map(|point| json!({
            "time": point.time,
            "yesPrice": point.yes_price,
            "noPrice": point.no_price,
            "volume": point.volume,
            "side": point.side,
            "sequence": point.sequence,
        })).collect::<Vec<_>>(),
    })
}

fn combo_event_value(item: crate::trade_proto::qot_get_event_contract_combo_list::ComboEvent) -> Value {
    json!({
        "eventSecurity": security_value(&item.event_security),
        "eventName": item.event_name,
        "comboContracts": item.combo_contracts.iter().map(security_value).collect::<Vec<_>>(),
        "seriesSecurity": item.series_security.as_ref().map(security_value),
        "category": item.category,
        "competition": item.competition,
        "competitionScope": item.competition_scope,
    })
}

fn combo_leg_value(leg: &crate::trade_proto::qot_common::ComboLeg) -> Value {
    json!({
        "security": security_value(&leg.security),
        "side": leg.side,
        "ratio": leg.qty_ratio,
        "predSide": leg.pred_side,
    })
}

#[derive(Debug)]
struct ComboQuoteRequest {
    mvc: String,
    legs: Vec<crate::trade_proto::qot_common::ComboLeg>,
}

impl ComboQuoteRequest {
    fn parse(value: &Value) -> Result<Self, PredictionMarketReadError> {
        let object = value.as_object().ok_or_else(|| invalid("prediction combo quote payload must be an object"))?;
        let mvc = object.get("mvc").and_then(Value::as_str).map(str::trim).filter(|v| !v.is_empty()).ok_or_else(|| invalid("mvc is required"))?;
        if mvc.len() > 256 || mvc.chars().any(char::is_control) {
            return Err(invalid("mvc is invalid"));
        }
        let legs = object.get("legs").or_else(|| object.get("comboLegList")).and_then(Value::as_array).ok_or_else(|| invalid("legs are required"))?;
        if legs.is_empty() || legs.len() > 20 {
            return Err(invalid("legs must contain between 1 and 20 items"));
        }
        let mut encoded = Vec::with_capacity(legs.len());
        for leg in legs {
            let item = leg.as_object().ok_or_else(|| invalid("prediction combo leg is invalid"))?;
            let instrument = item.get("instrumentId").and_then(Value::as_str).ok_or_else(|| invalid("prediction combo leg instrumentId is required"))?;
            let security_code = code(Some(instrument), "instrumentId")?;
            let side = item.get("side").and_then(Value::as_str).map(|v| match v.to_ascii_uppercase().as_str() { "BUY" => Ok(1), "SELL" => Ok(2), _ => Err(()) }).transpose().map_err(|_| invalid("prediction combo leg side must be BUY or SELL"))?.unwrap_or(1);
            let pred_side = item.get("predictionSide").and_then(Value::as_str).map(|v| match v.to_ascii_uppercase().as_str() { "YES" => Ok(1), "NO" => Ok(2), _ => Err(()) }).transpose().map_err(|_| invalid("predictionSide must be YES or NO"))?.unwrap_or(1);
            let ratio = item.get("ratio").and_then(Value::as_i64).unwrap_or(1);
            if !(1..=100).contains(&ratio) {
                return Err(invalid("prediction combo leg ratio must be between 1 and 100"));
            }
            encoded.push(crate::trade_proto::qot_common::ComboLeg {
                security: security(&security_code),
                side: Some(side),
                qty_ratio: Some(ratio as f64),
                position_id: None,
                pred_side: Some(pred_side),
            });
        }
        Ok(Self { mvc: mvc.to_owned(), legs: encoded })
    }

    fn encode(&self) -> Vec<u8> {
        crate::trade_proto::qot_get_event_contract_combo_rfq::Request {
            c2s: crate::trade_proto::qot_get_event_contract_combo_rfq::C2s {
                combo_leg_list: self.legs.clone(),
                mvc: self.mvc.clone(),
            },
        }
        .encode_to_vec()
    }
}

fn bytes_to_hex(value: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(value.len() * 2);
    for byte in value {
        output.push(HEX[(byte >> 4) as usize] as char);
        output.push(HEX[(byte & 0x0f) as usize] as char);
    }
    output
}

impl From<OpenDSessionCoordinatorError> for PredictionMarketReadError {
    fn from(error: OpenDSessionCoordinatorError) -> Self {
        Self::Session(error.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn prediction_query_rejects_invalid_period_and_malformed_combo() {
        assert!(parse_kline_type(Some("2m")).is_err());
        assert!(ComboQuoteRequest::parse(&json!({"mvc":"US.MVC","legs":[]})).is_err());
        assert!(ComboQuoteRequest::parse(&json!({
            "mvc":"US.MVC",
            "legs":[{"instrumentId":"US.EC.ONE","side":"BUY","ratio":1,"predictionSide":"YES"}]
        })).is_ok());
    }

    #[test]
    fn prediction_route_parsing_preserves_contract_operation_boundaries() {
        assert_eq!(route_operation("/api/v1/market-data/prediction/contracts/US.EC-1/snapshot").unwrap(), ("snapshot", Some("US.EC-1")));
        assert!(route_operation("/api/v1/market-data/prediction/contracts/US.EC-1/snapshot/extra").is_err());
    }
}
