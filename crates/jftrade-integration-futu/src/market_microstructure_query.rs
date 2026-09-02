//! Typed OpenD readers for market microstructure and company-profile routes.
//!
//! This adapter deliberately keeps protobuf details at the Futu boundary. The
//! engine receives a validated JSON feature result and must never manufacture
//! an empty success when OpenD is unavailable or returns malformed data.

use std::sync::{Arc, Mutex};

use prost::Message;
use serde_json::{Value, json};
use thiserror::Error;

use crate::OpenDSessionCoordinator;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum MarketMicrostructureOperation {
    Depth,
    Ticks,
    BrokerQueue,
    CapitalFlow,
    CapitalDistribution,
    Intraday,
    Profile,
}

pub trait MarketMicrostructureReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        operation: MarketMicrostructureOperation,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError>;
}

#[derive(Debug, Error)]
pub enum MarketMicrostructureError {
    #[error("invalid market microstructure request: {0}")]
    Invalid(String),
    #[error("OpenD session unavailable: {0}")]
    Session(String),
    #[error("OpenD {operation} request rejected retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("decode OpenD {operation} response: {message}")]
    Decode {
        operation: &'static str,
        message: String,
    },
}

#[derive(Clone)]
pub struct OpenDMarketMicrostructureReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDMarketMicrostructureReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDMarketMicrostructureReader")
            .finish_non_exhaustive()
    }
}

impl OpenDMarketMicrostructureReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }

    fn call<R: Message + Default>(
        &self,
        protocol: u32,
        operation: &'static str,
        body: Vec<u8>,
    ) -> Result<R, MarketMicrostructureError> {
        let coordinator = self.coordinator.lock().map_err(|_| {
            MarketMicrostructureError::Session("coordinator lock poisoned".to_owned())
        })?;
        let session = coordinator
            .session()
            .map_err(|error| MarketMicrostructureError::Session(error.to_string()))?;
        let bytes = session
            .managed_session()
            .call(protocol, &body)
            .map_err(|error| MarketMicrostructureError::Session(error.to_string()))?;
        R::decode(bytes.as_slice()).map_err(|error| MarketMicrostructureError::Decode {
            operation,
            message: error.to_string(),
        })
    }

    fn security(
        instrument_id: &str,
    ) -> Result<crate::trade_proto::qot_common::Security, MarketMicrostructureError> {
        let (market, code) = instrument_id.trim().split_once('.').ok_or_else(|| {
            MarketMicrostructureError::Invalid("instrumentId must be MARKET.CODE".to_owned())
        })?;
        let market = match market.trim().to_ascii_uppercase().as_str() {
            "HK" => 1,
            "US" => 11,
            "SH" => 21,
            "SZ" => 22,
            "CN" => {
                return Err(MarketMicrostructureError::Invalid(
                    "CN requires SH or SZ prefix".to_owned(),
                ));
            }
            _ => {
                return Err(MarketMicrostructureError::Invalid(
                    "unsupported market".to_owned(),
                ));
            }
        };
        let code = code.trim().to_ascii_uppercase();
        if code.is_empty()
            || code.len() > 64
            || code.chars().any(|c| c.is_whitespace() || c.is_control())
        {
            return Err(MarketMicrostructureError::Invalid(
                "instrument code is invalid".to_owned(),
            ));
        }
        Ok(crate::trade_proto::qot_common::Security { market, code })
    }

    fn result(feature: &str, instrument_id: &str, entries: Vec<Value>, metadata: Value) -> Value {
        let now = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        json!({
            "asOf": now,
            "entries": entries,
            "hasMore": false,
            "total": entries.len(),
            "provider": {"brokerId": "futu", "securityFirm": "Futu/Moomoo via OpenD", "featureId": feature, "capability": "available", "selectionReason": "adapter_request", "resolvedAt": now},
            "metadata": metadata,
            "resolvedInstrument": instrument_id,
        })
    }
}

impl MarketMicrostructureReadPort for OpenDMarketMicrostructureReader {
    fn query(
        &self,
        operation: MarketMicrostructureOperation,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        let security = Self::security(instrument_id)?;
        match operation {
            MarketMicrostructureOperation::Depth => self.depth(security, instrument_id, params),
            MarketMicrostructureOperation::Ticks => self.ticks(security, instrument_id, params),
            MarketMicrostructureOperation::BrokerQueue => {
                self.broker_queue(security, instrument_id)
            }
            MarketMicrostructureOperation::CapitalFlow => {
                self.capital_flow(security, instrument_id, params)
            }
            MarketMicrostructureOperation::CapitalDistribution => {
                self.capital_distribution(security, instrument_id)
            }
            MarketMicrostructureOperation::Intraday => self.intraday(security, instrument_id),
            MarketMicrostructureOperation::Profile => self.profile(security, instrument_id),
        }
    }
}

impl OpenDMarketMicrostructureReader {
    fn depth(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_order_book::{C2s, Request, Response};
        let num = params
            .get("num")
            .and_then(Value::as_i64)
            .unwrap_or(10)
            .clamp(1, 50) as i32;
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_order_book::PROTOCOL_ID,
            "Qot_GetOrderBook",
            (Request {
                c2s: C2s {
                    security,
                    num,
                    order_book_type: None,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetOrderBook",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetOrderBook", "s2c"))?;
        let levels = |items: Vec<crate::trade_proto::qot_common::OrderBook>| {
            items
                .into_iter()
                .map(|item| {
                    finite(item.price, "depth price")?;
                    if item.volume < 0 {
                        return Err(MarketMicrostructureError::Decode {
                            operation: "Qot_GetOrderBook",
                            message: "negative depth volume".to_owned(),
                        });
                    }
                    let mut value = json!({
                        "price": item.price,
                        "volume": item.volume as f64,
                        "orderCount": item.oreder_count,
                    });
                    if !item.detail_list.is_empty() {
                        value["detailList"] = json!(item
                            .detail_list
                            .into_iter()
                            .map(|detail| {
                                json!({"orderId": detail.order_id, "volume": detail.volume as f64})
                            })
                            .collect::<Vec<_>>());
                    }
                    Ok(value)
                })
                .collect::<Result<Vec<_>, MarketMicrostructureError>>()
        };
        let asks = levels(s2c.order_book_ask_list)?;
        let bids = levels(s2c.order_book_bid_list)?;
        let (market, symbol) = instrument_id.split_once('.').unwrap_or(("", instrument_id));
        let resolved_at = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        let mut depth = json!({
            "symbol": instrument_id,
            "bids": bids,
            "asks": asks,
        });
        if let Some(name) = s2c.name.filter(|name| !name.trim().is_empty()) {
            depth["name"] = json!(name);
        }
        if let Some(value) = s2c
            .svr_recv_time_bid
            .filter(|value| !value.trim().is_empty())
        {
            depth["svrRecvTimeBid"] = json!(value);
        }
        if let Some(value) = s2c
            .svr_recv_time_ask
            .filter(|value| !value.trim().is_empty())
        {
            depth["svrRecvTimeAsk"] = json!(value);
        }
        Ok(json!({
            "request": {
                "market": market,
                "symbol": symbol,
                "instrumentId": instrument_id,
                "num": num,
            },
            "depth": depth,
            "meta": {
                "instrumentId": instrument_id,
                "source": "bbgo:futu",
                "resolvedAt": resolved_at,
                "fromCache": false,
            },
        }))
    }

    fn ticks(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_ticker::{C2s, Request, Response};
        let max_ret_num = params
            .get("pageSize")
            .or_else(|| params.get("limit"))
            .and_then(Value::as_i64)
            .unwrap_or(100)
            .clamp(1, 1000) as i32;
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_ticker::PROTOCOL_ID,
            "Qot_GetTicker",
            (Request {
                c2s: C2s {
                    security,
                    max_ret_num,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetTicker",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetTicker", "s2c"))?;
        let entries = s2c.ticker_list.into_iter().map(|item| {
            finite(item.price, "tick price")?;
            if item.volume < 0 || !item.turnover.is_finite() { return Err(MarketMicrostructureError::Decode { operation: "Qot_GetTicker", message: "invalid tick numeric value".to_owned() }); }
            Ok(json!({"time": item.time, "sequence": item.sequence, "direction": item.dir, "price": item.price.to_string(), "volume": item.volume.to_string(), "turnover": item.turnover.to_string(), "timestamp": item.timestamp}))
        }).collect::<Result<Vec<_>, _>>()?;
        Ok(Self::result(
            "market.ticks",
            instrument_id,
            entries,
            json!({"name": s2c.name}),
        ))
    }

    fn broker_queue(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_broker::{C2s, Request, Response};
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_broker::PROTOCOL_ID,
            "Qot_GetBroker",
            (Request {
                c2s: C2s {
                    security,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetBroker",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetBroker", "s2c"))?;
        let map = |item: crate::trade_proto::qot_common::Broker| json!({"id": item.id, "name": item.name, "position": item.pos, "orderId": item.order_id, "volume": item.volume});
        let entries = vec![
            json!({"asks": s2c.broker_ask_list.into_iter().map(map).collect::<Vec<_>>(), "bids": s2c.broker_bid_list.into_iter().map(map).collect::<Vec<_>>(), "name": s2c.name}),
        ];
        Ok(Self::result(
            "market.broker_queue",
            instrument_id,
            entries,
            json!({}),
        ))
    }

    fn capital_flow(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_capital_flow::{C2s, Request, Response};
        let period_type = optional_i32(params, "periodType")?;
        let begin_time = optional_string(params, "beginTime")?;
        let end_time = optional_string(params, "endTime")?;
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_capital_flow::PROTOCOL_ID,
            "Qot_GetCapitalFlow",
            (Request {
                c2s: C2s {
                    security,
                    period_type,
                    begin_time,
                    end_time,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetCapitalFlow",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetCapitalFlow", "s2c"))?;
        let entries = s2c
            .flow_item_list
            .into_iter()
            .map(|item| {
                finite(item.in_flow, "capital flow")?;
                for (label, value) in [
                    ("main capital flow", item.main_in_flow),
                    ("super capital flow", item.super_in_flow),
                    ("big capital flow", item.big_in_flow),
                    ("mid capital flow", item.mid_in_flow),
                    ("small capital flow", item.sml_in_flow),
                ] {
                    if let Some(value) = value {
                        finite(value, label)?;
                    }
                }
                Ok(json!({
                    "inFlow": item.in_flow,
                    "time": item.time,
                    "timestamp": item.timestamp,
                    "mainInFlow": item.main_in_flow,
                    "superInFlow": item.super_in_flow,
                    "bigInFlow": item.big_in_flow,
                    "midInFlow": item.mid_in_flow,
                    "smallInFlow": item.sml_in_flow,
                }))
            })
            .collect::<Result<Vec<_>, MarketMicrostructureError>>()?;
        Ok(Self::result(
            "market.capital_flow",
            instrument_id,
            entries,
            json!({"lastValidTime": s2c.last_valid_time, "lastValidTimestamp": s2c.last_valid_timestamp}),
        ))
    }

    fn capital_distribution(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_capital_distribution::{C2s, Request, Response};
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_capital_distribution::PROTOCOL_ID,
            "Qot_GetCapitalDistribution",
            (Request {
                c2s: C2s {
                    security,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetCapitalDistribution",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetCapitalDistribution", "s2c"))?;
        for (label, value) in [
            ("capital in big", s2c.capital_in_big),
            ("capital in mid", s2c.capital_in_mid),
            ("capital in small", s2c.capital_in_small),
            ("capital out big", s2c.capital_out_big),
            ("capital out mid", s2c.capital_out_mid),
            ("capital out small", s2c.capital_out_small),
        ] {
            finite(value, label)?;
        }
        for (label, value) in [
            ("capital in super", s2c.capital_in_super),
            ("capital out super", s2c.capital_out_super),
            ("capital distribution timestamp", s2c.update_timestamp),
        ] {
            if let Some(value) = value {
                finite(value, label)?;
            }
        }
        let mut entry = json!({
            "capitalInBig": s2c.capital_in_big,
            "capitalInMid": s2c.capital_in_mid,
            "capitalInSmall": s2c.capital_in_small,
            "capitalOutBig": s2c.capital_out_big,
            "capitalOutMid": s2c.capital_out_mid,
            "capitalOutSmall": s2c.capital_out_small,
        });
        if let Some(value) = s2c.capital_in_super {
            entry["capitalInSuper"] = json!(value);
        }
        if let Some(value) = s2c.capital_out_super {
            entry["capitalOutSuper"] = json!(value);
        }
        if let Some(value) = s2c.update_time {
            entry["updateTime"] = json!(value);
        }
        if let Some(value) = s2c.update_timestamp {
            entry["updateTimestamp"] = json!(value);
        }
        Ok(Self::result(
            "market.capital_flow",
            instrument_id,
            vec![entry],
            json!({}),
        ))
    }

    fn intraday(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_rt::{C2s, Request, Response};
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_rt::PROTOCOL_ID,
            "Qot_GetRT",
            (Request {
                c2s: C2s {
                    security,
                    header: None,
                },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetRT",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetRT", "s2c"))?;
        let entries = s2c
            .rt_list
            .into_iter()
            .map(|item| {
                if let Some(price) = item.price {
                    finite(price, "intraday price")?;
                }
                if item.volume.unwrap_or_default() < 0 {
                    return Err(MarketMicrostructureError::Decode {
                        operation: "Qot_GetRT",
                        message: "negative intraday volume".to_owned(),
                    });
                }
                Ok(json!({
                    "time": item.time,
                    "minute": item.minute,
                    "isBlank": item.is_blank,
                    "price": item.price,
                    "lastClosePrice": item.last_close_price,
                    "avgPrice": item.avg_price,
                    "volume": item.volume,
                    "turnover": item.turnover,
                    "timestamp": item.timestamp
                }))
            })
            .collect::<Result<Vec<_>, _>>()?;
        Ok(Self::result(
            "market.intraday",
            instrument_id,
            entries,
            json!({"name": s2c.name}),
        ))
    }

    fn profile(
        &self,
        security: crate::trade_proto::qot_common::Security,
        instrument_id: &str,
    ) -> Result<Value, MarketMicrostructureError> {
        use crate::trade_proto::qot_get_company_profile::{C2s, Request, Response};
        let response = self.call::<Response>(
            crate::trade_proto::qot_get_company_profile::PROTOCOL_ID,
            "Qot_GetCompanyProfile",
            (Request {
                c2s: C2s { security },
            })
            .encode_to_vec(),
        )?;
        ensure_ok(
            "Qot_GetCompanyProfile",
            response.ret_type,
            response.err_code,
            response.ret_msg,
        )?;
        let s2c = response
            .s2c
            .ok_or_else(|| decode_missing("Qot_GetCompanyProfile", "s2c"))?;
        let entries = s2c.item_list.into_iter().map(|item| json!({"name": item.name, "value": item.value, "fieldType": item.field_type})).collect::<Vec<_>>();
        if entries.is_empty() {
            return Err(MarketMicrostructureError::Rejected {
                operation: "Qot_GetCompanyProfile",
                ret_type: 0,
                err_code: 0,
                message: "OpenD returned no company profile fields".to_owned(),
            });
        }
        Ok(Self::result(
            "market.instrument_profile",
            instrument_id,
            entries,
            json!({}),
        ))
    }
}

fn ensure_ok(
    operation: &'static str,
    ret_type: i32,
    err_code: Option<i32>,
    ret_msg: Option<String>,
) -> Result<(), MarketMicrostructureError> {
    if ret_type == 0 {
        return Ok(());
    }
    Err(MarketMicrostructureError::Rejected {
        operation,
        ret_type,
        err_code: err_code.unwrap_or_default(),
        message: ret_msg.unwrap_or_else(|| "OpenD request failed".to_owned()),
    })
}

fn decode_missing(operation: &'static str, field: &str) -> MarketMicrostructureError {
    MarketMicrostructureError::Decode {
        operation,
        message: format!("response missing {field}"),
    }
}

fn finite(value: f64, label: &str) -> Result<(), MarketMicrostructureError> {
    if value.is_finite() {
        Ok(())
    } else {
        Err(MarketMicrostructureError::Decode {
            operation: "market-microstructure",
            message: format!("{label} is not finite"),
        })
    }
}

fn optional_i32(params: &Value, key: &str) -> Result<Option<i32>, MarketMicrostructureError> {
    let Some(value) = params.get(key) else {
        return Ok(None);
    };
    let parsed = match value {
        Value::Number(number) => number.as_i64().and_then(|value| i32::try_from(value).ok()),
        Value::String(value) => value.trim().parse::<i32>().ok(),
        _ => None,
    };
    parsed
        .map(Some)
        .ok_or_else(|| MarketMicrostructureError::Invalid(format!("{key} must be an integer")))
}

fn optional_string(params: &Value, key: &str) -> Result<Option<String>, MarketMicrostructureError> {
    let Some(value) = params.get(key) else {
        return Ok(None);
    };
    match value {
        Value::String(value) if !value.trim().is_empty() => Ok(Some(value.clone())),
        Value::String(_) | Value::Null => Ok(None),
        _ => Err(MarketMicrostructureError::Invalid(format!(
            "{key} must be a string"
        ))),
    }
}
