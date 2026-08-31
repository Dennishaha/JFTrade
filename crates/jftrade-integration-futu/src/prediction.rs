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

#[path = "prediction_values.rs"]
mod prediction_values;
use prediction_values::{
    ComboQuoteRequest, bytes_to_hex, combo_event_value, combo_leg_value, contract_value,
    event_value, feature_result, kline_value, milestone_value, order_book_value, security_value,
    snapshot_value, ticker_value,
};

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

    fn call(
        &self,
        protocol: u32,
        operation: &'static str,
        body: Vec<u8>,
    ) -> Result<Vec<u8>, PredictionMarketReadError> {
        let coordinator = self.coordinator.lock().map_err(|_| {
            PredictionMarketReadError::Session("coordinator lock poisoned".to_owned())
        })?;
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
        let response = self
            .call_response::<crate::trade_proto::qot_get_event_contract_combo_rfq::Response>(
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
        let s2c = response
            .s2c
            .ok_or_else(|| PredictionMarketReadError::Decode {
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

    fn number(
        &self,
        key: &str,
        default: i32,
        min: i32,
        max: i32,
    ) -> Result<i32, PredictionMarketReadError> {
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
                    category: query
                        .get("category")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractCategory",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| {
                s2c.category_list
                    .into_iter()
                    .map(|item| {
                        json!({
                            "category": item.category,
                            "categoryName": item.category_name,
                            "tags": item.tags,
                        })
                    })
                    .collect()
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn competitions(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_filter_competition::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3435,
            "Qot_FilterCompetition",
            Request {
                c2s: C2s {
                    category: query
                        .get("category")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    tag: query
                        .get("tag")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_FilterCompetition",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| {
                s2c.tag_filter_list
                    .into_iter()
                    .map(|item| {
                        json!({
                            "category": item.category,
                            "tag": item.tag,
                            "competitionList": item.competition_list,
                            "scopeList": item.scope_list,
                        })
                    })
                    .collect()
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn series(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_series_list::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3436,
            "Qot_GetEventContractSeriesList",
            Request {
                c2s: C2s {
                    category: query
                        .get("category")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    tag: query
                        .get("tag")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractSeriesList",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| {
                s2c.series_list
                    .into_iter()
                    .map(|item| {
                        json!({
                            "seriesSecurity": security_value(&item.series_security),
                            "seriesName": item.series_name,
                            "category": item.category,
                            "tags": item.tags,
                            "frequency": item.frequency,
                        })
                    })
                    .collect()
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, None))
    }

    fn events(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_event_list::{C2s, Request, Response};
        let series = code(
            query.get("seriesId").or_else(|| query.get("series")),
            "seriesId",
        )?;
        let response = self.call_response::<Response>(
            3437,
            "Qot_GetEventContractEventList",
            Request {
                c2s: C2s {
                    series: security(&series),
                    status: query.get("status").and_then(parse_status),
                    next_page: query
                        .get("cursor")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    count: Some(query.number("pageSize", 100, 1, 300)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractEventList",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let (entries, next) = response
            .s2c
            .map(|s2c| {
                (
                    s2c.event_list.into_iter().map(event_value).collect(),
                    s2c.next_page,
                )
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn contracts(
        &self,
        event_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract::{C2s, Request, Response};
        let event = code(event_id, "eventId")?;
        let response = self.call_response::<Response>(
            3438,
            "Qot_GetEventContract",
            Request {
                c2s: C2s {
                    event: security(&event),
                    next_page: query
                        .get("cursor")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    count: Some(query.number("pageSize", 100, 1, 1000)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContract",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let (entries, next) = response
            .s2c
            .map(|s2c| {
                (
                    s2c.contract_list.into_iter().map(contract_value).collect(),
                    s2c.next_page,
                )
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn milestones(
        &self,
        contract_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_milestone_list::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        // OpenD's milestone endpoint is keyed by the owning *event*, not by
        // the contract code exposed in the route. Resolve that relationship
        // through the authoritative contract snapshot first; sending the
        // contract itself here returns plausible-looking but incorrect data.
        let event = self.resolve_owning_event(&contract)?;
        let response = self.call_response::<Response>(
            3439,
            "Qot_GetEventContractMilestoneList",
            Request {
                c2s: C2s {
                    category: query
                        .get("category")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    competition: query
                        .get("competition")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    related_event: Some(security(&event)),
                    next_page: query
                        .get("cursor")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    count: Some(query.number("pageSize", 100, 1, 300)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractMilestoneList",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let (entries, next) = response
            .s2c
            .map(|s2c| {
                (
                    s2c.milestone_list
                        .into_iter()
                        .map(milestone_value)
                        .collect(),
                    s2c.next_page,
                )
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.discover", entries, next))
    }

    fn resolve_owning_event(&self, contract: &str) -> Result<String, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_snapshot::{C2s, Request, Response};
        let response = self.call_response::<Response>(
            3445,
            "Qot_GetEventContractSnapshot",
            Request {
                c2s: C2s {
                    security_list: vec![security(contract)],
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractSnapshot",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let snapshots = response
            .s2c
            .ok_or_else(|| prediction_decode_error("response missing s2c"))?
            .snapshot_list;
        owning_event_code(&snapshots, contract)
    }

    fn snapshot(&self, contract_id: Option<&str>) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_snapshot::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3445,
            "Qot_GetEventContractSnapshot",
            Request {
                c2s: C2s {
                    security_list: vec![security(&contract)],
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractSnapshot",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| s2c.snapshot_list.into_iter().map(snapshot_value).collect())
            .unwrap_or_default();
        Ok(feature_result("prediction.snapshot", entries, None))
    }

    fn order_book(
        &self,
        contract_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_order_book::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3446,
            "Qot_GetEventContractOrderBook",
            Request {
                c2s: C2s {
                    security: security(&contract),
                    num: query.number("depth", 10, 1, 100)?,
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractOrderBook",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| {
                s2c.order_book_list
                    .into_iter()
                    .map(order_book_value)
                    .collect()
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.depth", entries, None))
    }

    fn candles(
        &self,
        contract_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_kline::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3447,
            "Qot_GetEventContractKline",
            Request {
                c2s: C2s {
                    security: security(&contract),
                    kline_source: Some(1),
                    pre_side: None,
                    ktype: Some(parse_kline_type(
                        query.get("period").or_else(|| query.get("interval")),
                    )?),
                    max_count: Some(query.number("pageSize", 100, 1, 1000)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractKline",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| s2c.kline_list.into_iter().map(kline_value).collect())
            .unwrap_or_default();
        Ok(feature_result("prediction.history", entries, None))
    }

    fn historical(
        &self,
        contract_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_request_history_event_contract_kl::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let begin = query
            .get("from")
            .or_else(|| query.get("beginTime"))
            .ok_or_else(|| invalid("from is required"))?;
        let end = query
            .get("to")
            .or_else(|| query.get("endTime"))
            .ok_or_else(|| invalid("to is required"))?;
        if begin.is_empty() || end.is_empty() || begin.len() > 64 || end.len() > 64 {
            return Err(invalid("historical time range is invalid"));
        }
        let response = self.call_response::<Response>(
            3456,
            "Qot_RequestHistoryEventContractKL",
            Request {
                c2s: C2s {
                    security: security(&contract),
                    kline_source: Some(1),
                    pre_side: None,
                    kl_type: parse_kline_type(
                        query.get("period").or_else(|| query.get("interval")),
                    )?,
                    begin_time: begin.to_owned(),
                    end_time: end.to_owned(),
                    max_ack_kl_num: Some(query.number("pageSize", 100, 1, 1000)?),
                    next_req_key: None,
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_RequestHistoryEventContractKL",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let (entries, next) = response
            .s2c
            .map(|s2c| {
                (
                    s2c.kline_list.into_iter().map(kline_value).collect(),
                    s2c.next_req_key.map(|key| bytes_to_hex(&key)),
                )
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.history", entries, next))
    }

    fn ticks(
        &self,
        contract_id: Option<&str>,
        query: &Query,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_ticker::{C2s, Request, Response};
        let contract = code(contract_id, "code")?;
        let response = self.call_response::<Response>(
            3448,
            "Qot_GetEventContractTicker",
            Request {
                c2s: C2s {
                    security: security(&contract),
                    count: Some(query.number("pageSize", 30, 1, 1000)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractTicker",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let entries = response
            .s2c
            .map(|s2c| s2c.ticker_list.into_iter().map(ticker_value).collect())
            .unwrap_or_default();
        Ok(feature_result("prediction.history", entries, None))
    }

    fn combo_events(&self, query: &Query) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_get_event_contract_combo_list::{C2s, Request, Response};
        let series = query
            .get("seriesId")
            .or_else(|| query.get("series"))
            .map(|value| code(Some(value), "seriesId"))
            .transpose()?;
        let response = self.call_response::<Response>(
            3453,
            "Qot_GetEventContractComboList",
            Request {
                c2s: C2s {
                    category: query
                        .get("category")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    competition: query
                        .get("competition")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    series: series.as_deref().map(security),
                    next_page: query
                        .get("cursor")
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned),
                    count: Some(query.number("pageSize", 100, 1, 300)?),
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetEventContractComboList",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        let (entries, next) = response
            .s2c
            .map(|s2c| {
                (
                    s2c.combo_event_list
                        .into_iter()
                        .map(combo_event_value)
                        .collect(),
                    s2c.next_page,
                )
            })
            .unwrap_or_default();
        Ok(feature_result("prediction.combo_eligible", entries, next))
    }

    fn update_subscription(
        &self,
        code_value: &str,
        data_types: &[String],
        subscribe: bool,
    ) -> Result<Value, PredictionMarketReadError> {
        use crate::trade_proto::qot_sub_event_contract::{C2s, Request, Response};
        let contract = code(Some(code_value), "code")?;
        let mut types = Vec::new();
        for data_type in data_types {
            match data_type.trim().to_ascii_uppercase().as_str() {
                "ORDER_BOOK" => types.push(2),
                "KLINE" => types.push(11),
                "TICKER" | "TICKS" => types.push(4),
                value => {
                    return Err(invalid(&format!(
                        "unsupported event contract subscription type {value:?}"
                    )));
                }
            }
        }
        if subscribe && types.is_empty() {
            return Err(invalid("dataTypes must not be empty"));
        }
        let response = self.call_response::<Response>(
            3455,
            "Qot_SubEventContract",
            Request {
                c2s: C2s {
                    security_list: vec![security(&contract)],
                    sub_type_list: types,
                    is_sub_or_un_sub: subscribe,
                    is_reg_or_un_reg_push: Some(subscribe),
                    is_first_push: Some(subscribe),
                    is_unsub_all: None,
                    kline_source: Vec::new(),
                    header: None,
                },
            }
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_SubEventContract",
            response.ret_type,
            response.err_code,
            response.ret_msg.as_deref(),
        )?;
        Ok(
            json!({"instrumentId": format!("US.{contract}"), "dataTypes": data_types, "subscribed": subscribe}),
        )
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

fn prediction_decode_error(message: impl Into<String>) -> PredictionMarketReadError {
    PredictionMarketReadError::Decode {
        operation: "Qot_GetEventContractSnapshot",
        message: message.into(),
    }
}

fn owning_event_code(
    snapshots: &[crate::trade_proto::qot_get_event_contract_snapshot::SnapshotItem],
    contract: &str,
) -> Result<String, PredictionMarketReadError> {
    let item = snapshots
        .iter()
        .find(|item| {
            item.code.market == EVENT_MARKET && item.code.code.eq_ignore_ascii_case(contract)
        })
        .ok_or_else(|| prediction_decode_error(format!("contract {contract} has no snapshot")))?;
    let event = item.event_code.as_ref().ok_or_else(|| {
        prediction_decode_error(format!("contract {contract} has no owning event"))
    })?;
    if event.market != EVENT_MARKET || event.code.trim().is_empty() {
        return Err(prediction_decode_error(format!(
            "contract {contract} has an invalid owning event"
        )));
    }
    code(Some(event.code.as_str()), "eventId").map_err(|_| {
        prediction_decode_error(format!("contract {contract} has an invalid owning event"))
    })
}

fn parse_kline_type(value: Option<&str>) -> Result<i32, PredictionMarketReadError> {
    match value.unwrap_or("1m").trim().to_ascii_lowercase().as_str() {
        "1m" | "1min" => Ok(1),
        "5m" | "5min" => Ok(6),
        "1h" | "60m" | "60min" => Ok(9),
        "1d" | "day" => Ok(2),
        _ => Err(invalid(
            "prediction candles period must be 1m, 5m, 1h, or 1d",
        )),
    }
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
        assert!(
            ComboQuoteRequest::parse(&json!({
                "mvc":"US.MVC",
                "legs":[{"instrumentId":"US.EC.ONE","side":"BUY","ratio":1,"predictionSide":"YES"}]
            }))
            .is_ok()
        );
    }

    #[test]
    fn prediction_route_parsing_preserves_contract_operation_boundaries() {
        assert_eq!(
            route_operation("/api/v1/market-data/prediction/contracts/US.EC-1/snapshot").unwrap(),
            ("snapshot", Some("US.EC-1"))
        );
        assert!(
            route_operation("/api/v1/market-data/prediction/contracts/US.EC-1/snapshot/extra")
                .is_err()
        );
    }

    #[test]
    fn milestones_resolve_the_owning_event_and_fail_closed_when_missing() {
        let snapshots = vec![
            crate::trade_proto::qot_get_event_contract_snapshot::SnapshotItem {
                code: security("EC.ONE"),
                event_code: Some(security("EVENT.ONE")),
                ..Default::default()
            },
        ];
        assert_eq!(
            owning_event_code(&snapshots, "EC.ONE").expect("owning event"),
            "EVENT.ONE"
        );
        let no_event = vec![
            crate::trade_proto::qot_get_event_contract_snapshot::SnapshotItem {
                code: security("EC.NO.EVENT"),
                event_code: None,
                ..Default::default()
            },
        ];
        assert!(matches!(
            owning_event_code(&no_event, "EC.NO.EVENT"),
            Err(PredictionMarketReadError::Decode { .. })
        ));
    }
}
