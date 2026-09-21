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
        // prost cannot observe proto2 `required` presence, so enforce the same
        // `RequiredNotSet` contract Go's `proto.Unmarshal` applies before the
        // depth projection can publish defaulted levels.
        if protocol == crate::trade_proto::qot_get_order_book::PROTOCOL_ID {
            crate::order_book_wire::validate_order_book_response(bytes.as_slice())
                .map_err(|message| MarketMicrostructureError::Decode { operation, message })?;
        }
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
        let asks = order_book_levels(s2c.order_book_ask_list)?;
        let bids = order_book_levels(s2c.order_book_bid_list)?;
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

/// Projects OpenD order-book levels into the neutral depth JSON.
///
/// Parity: `go:452dea11:pkg/futu/adapter_new_methods.go`
/// `orderBookLevelFromPb`: price/volume/orderCount always project, the
/// optional detail list is present only when OpenD supplied entries, and
/// non-finite prices or negative volumes fail closed.
fn order_book_levels(
    items: Vec<crate::trade_proto::qot_common::OrderBook>,
) -> Result<Vec<Value>, MarketMicrostructureError> {
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
                value["detailList"] = json!(
                    item.detail_list
                        .into_iter()
                        .map(|detail| {
                            json!({"orderId": detail.order_id, "volume": detail.volume as f64})
                        })
                        .collect::<Vec<_>>()
                );
            }
            Ok(value)
        })
        .collect()
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

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::{Arc, Mutex};
    use std::time::Duration;

    use super::*;
    use crate::trade_proto::qot_common::{OrderBook, OrderBookDetail};

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    fn read_frame(stream: &mut std::net::TcpStream) -> crate::Frame {
        let mut header = [0_u8; crate::frame::HEADER_LEN];
        stream.read_exact(&mut header).expect("frame header");
        let body_len = u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
        let mut packet = Vec::from(header);
        let mut body = vec![0_u8; body_len];
        stream.read_exact(&mut body).expect("frame body");
        packet.extend(body);
        crate::decode_frame(&packet).expect("decoded frame")
    }

    /// Serves one Qot_GetOrderBook request over a loopback framed session and
    /// hands the client a coordinator bound to that session.
    fn depth_reader_with_response(
        response: crate::trade_proto::qot_get_order_book::Response,
    ) -> OpenDMarketMicrostructureReader {
        depth_reader_with_body(response.encode_to_vec())
    }

    /// Serves one Qot_GetOrderBook request with a raw response body so
    /// wire-level payloads reach the decoder exactly as OpenD sends them.
    fn depth_reader_with_body(body: Vec<u8>) -> OpenDMarketMicrostructureReader {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_frame(&mut stream);
            stream
                .write_all(
                    &crate::encode_frame(
                        init.header.proto_id,
                        init.header.serial_no,
                        &InitResponse {
                            ret_type: Some(0),
                            s2c: Some(InitState {
                                server_ver: 1009,
                                conn_id: 7,
                            }),
                        }
                        .encode_to_vec(),
                    )
                    .expect("init frame"),
                )
                .expect("init response");
            let request = read_frame(&mut stream);
            assert_eq!(
                request.header.proto_id,
                crate::trade_proto::qot_get_order_book::PROTOCOL_ID
            );
            let decoded =
                crate::trade_proto::qot_get_order_book::Request::decode(request.body.as_slice())
                    .expect("depth request");
            assert_eq!(decoded.c2s.security.market, 11);
            assert_eq!(decoded.c2s.security.code, "AAPL");
            stream
                .write_all(
                    &crate::encode_frame(request.header.proto_id, request.header.serial_no, &body)
                        .expect("response frame"),
                )
                .expect("response");
        });
        let coordinator = Arc::new(Mutex::new(
            OpenDSessionCoordinator::connect(
                crate::OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
                Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default()),
                Vec::new(),
                0,
            )
            .expect("coordinator"),
        ));
        OpenDMarketMicrostructureReader::new(coordinator)
    }

    fn depth_response(
        name: Option<String>,
        asks: Vec<OrderBook>,
        bids: Vec<OrderBook>,
    ) -> crate::trade_proto::qot_get_order_book::Response {
        crate::trade_proto::qot_get_order_book::Response {
            ret_type: 0,
            ret_msg: None,
            err_code: None,
            s2c: Some(crate::trade_proto::qot_get_order_book::S2c {
                security: crate::trade_proto::qot_common::Security {
                    market: 11,
                    code: "AAPL".to_owned(),
                },
                name,
                order_book_ask_list: asks,
                order_book_bid_list: bids,
                svr_recv_time_bid: Some("2025-01-01 10:00:00.000".to_owned()),
                svr_recv_time_bid_timestamp: None,
                svr_recv_time_ask: Some("2025-01-01 10:00:01.000".to_owned()),
                svr_recv_time_ask_timestamp: None,
                order_book_type: None,
            }),
        }
    }

    fn level(price: f64, volume: i64, order_count: i32) -> OrderBook {
        OrderBook {
            price,
            volume,
            oreder_count: order_count,
            detail_list: Vec::new(),
            hp_volume: None,
        }
    }

    fn wire_varint(mut value: u64) -> Vec<u8> {
        let mut out = Vec::new();
        loop {
            let byte = (value & 0x7f) as u8;
            value >>= 7;
            if value == 0 {
                out.push(byte);
                return out;
            }
            out.push(byte | 0x80);
        }
    }

    fn wire_key(field: u32, wire_type: u8) -> Vec<u8> {
        wire_varint((u64::from(field) << 3) | u64::from(wire_type))
    }

    fn wire_varint_field(field: u32, value: u64) -> Vec<u8> {
        let mut out = wire_key(field, 0);
        out.extend(wire_varint(value));
        out
    }

    fn wire_bytes_field(field: u32, payload: &[u8]) -> Vec<u8> {
        let mut out = wire_key(field, 2);
        out.extend(wire_varint(payload.len() as u64));
        out.extend_from_slice(payload);
        out
    }

    fn order_book_success_prefix() -> Vec<u8> {
        wire_varint_field(1, 0)
    }

    fn order_book_response_with_s2c(s2c: &[u8]) -> Vec<u8> {
        let mut body = order_book_success_prefix();
        body.extend(wire_bytes_field(4, s2c));
        body
    }

    fn depth_read_raw(body: Vec<u8>) -> Result<Value, MarketMicrostructureError> {
        depth_reader_with_body(body).query(
            MarketMicrostructureOperation::Depth,
            "US.AAPL",
            &json!({"num": 10}),
        )
    }

    #[test]
    fn depth_read_rejects_a_closed_session_before_any_projection() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:14
        // TestGetOrderBookRejectsDisconnectedSession. Go returns ErrClosed when
        // the client has no live connection; the Rust owner is the coordinator
        // session gate, which must reject the read instead of manufacturing an
        // empty success.
        let reader = depth_reader_with_response(depth_response(None, Vec::new(), Vec::new()));
        reader
            .coordinator
            .lock()
            .expect("coordinator lock")
            .close()
            .expect("close coordinator");
        let error = reader
            .query(
                MarketMicrostructureOperation::Depth,
                "US.AAPL",
                &json!({"num": 10}),
            )
            .expect_err("closed session must reject the depth read");
        assert!(
            matches!(error, MarketMicrostructureError::Session(ref message) if message.contains("closed")),
            "closed session must surface as a typed session error, got {error}"
        );
    }

    #[test]
    fn depth_read_rejects_malformed_top_level_wire_fields() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:22
        // TestParseOrderBookResponseRejectsMalformedTopLevelFields. Go's hand
        // written protowire parser rejects a truncated tag, a wrong wire type
        // on retType/retMsg/errCode/s2c, a truncated varint or bytes field, a
        // malformed s2c tag and a truncated unknown field. prost is the Rust
        // decoder owner, so the same bodies must surface as Decode errors
        // instead of a defaulted depth result.
        let ret_type_wrong_wire_type = wire_bytes_field(1, &[]);
        let ret_type_truncated_varint = {
            let mut body = wire_key(1, 0);
            body.push(0x80);
            body
        };
        let ret_msg_wrong_wire_type = wire_varint_field(2, 0);
        let ret_msg_truncated_bytes = {
            let mut body = wire_key(2, 2);
            body.push(0x80);
            body
        };
        let err_code_wrong_wire_type = wire_bytes_field(3, &[]);
        let err_code_truncated_varint = {
            let mut body = wire_key(3, 0);
            body.push(0x80);
            body
        };
        let s2c_wrong_wire_type = {
            let mut body = order_book_success_prefix();
            body.extend(wire_varint_field(4, 0));
            body
        };
        let s2c_malformed_tag = order_book_response_with_s2c(&[0x80]);
        let unknown_field_truncated = {
            let mut body = order_book_success_prefix();
            body.extend(wire_key(9, 2));
            body.push(0x80);
            body
        };
        let cases: [(&str, Vec<u8>); 10] = [
            ("truncated tag", vec![0x80]),
            ("retType wrong wire type", ret_type_wrong_wire_type),
            ("retType truncated varint", ret_type_truncated_varint),
            ("retMsg wrong wire type", ret_msg_wrong_wire_type),
            ("retMsg truncated bytes", ret_msg_truncated_bytes),
            ("errCode wrong wire type", err_code_wrong_wire_type),
            ("errCode truncated varint", err_code_truncated_varint),
            ("s2c wrong wire type", s2c_wrong_wire_type),
            ("s2c malformed tag", s2c_malformed_tag),
            ("unknown field truncated", unknown_field_truncated),
        ];
        for (name, body) in cases {
            let error = depth_read_raw(body).expect_err(name);
            assert!(
                matches!(error, MarketMicrostructureError::Decode { .. }),
                "{name} must surface as a typed Decode error, got {error}"
            );
        }
    }

    #[test]
    fn depth_read_rejects_malformed_s2c_wire_fields() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:50
        // TestParseOrderBookResponseRejectsMalformedS2CFields. Every typed S2C
        // field (security, ask, bid, receive times and name) must reject a
        // wrong wire type or truncated payload, and a truncated unknown S2C
        // field must not be silently dropped.
        let invalid_message = wire_bytes_field(1, &[0x80]);
        let cases: [(&str, Vec<u8>); 16] = [
            ("security wrong wire type", wire_varint_field(1, 0)),
            ("security truncated bytes", {
                let mut body = wire_key(1, 2);
                body.push(0x80);
                body
            }),
            (
                "security malformed message",
                wire_bytes_field(1, &invalid_message),
            ),
            ("ask wrong wire type", wire_varint_field(2, 0)),
            ("ask truncated bytes", {
                let mut body = wire_key(2, 2);
                body.push(0x80);
                body
            }),
            ("ask malformed level", wire_bytes_field(2, &invalid_message)),
            ("bid wrong wire type", wire_varint_field(3, 0)),
            ("bid truncated bytes", {
                let mut body = wire_key(3, 2);
                body.push(0x80);
                body
            }),
            ("bid malformed level", wire_bytes_field(3, &invalid_message)),
            ("bid receive time wrong wire type", wire_varint_field(4, 0)),
            ("bid receive time truncated", {
                let mut body = wire_key(4, 2);
                body.push(0x80);
                body
            }),
            ("ask receive time wrong wire type", wire_varint_field(6, 0)),
            ("ask receive time truncated", {
                let mut body = wire_key(6, 2);
                body.push(0x80);
                body
            }),
            ("name wrong wire type", wire_varint_field(8, 0)),
            ("name truncated", {
                let mut body = wire_key(8, 2);
                body.push(0x80);
                body
            }),
            ("unknown field truncated", {
                let mut body = wire_key(9, 2);
                body.push(0x80);
                body
            }),
        ];
        for (name, s2c) in cases {
            let error = depth_read_raw(order_book_response_with_s2c(&s2c)).expect_err(name);
            assert!(
                matches!(error, MarketMicrostructureError::Decode { .. }),
                "{name} must surface as a typed Decode error, got {error}"
            );
        }
    }

    #[test]
    fn depth_read_skips_valid_unknown_fields_and_keeps_known_projection() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:85
        // TestParseOrderBookResponseSkipsValidUnknownFields. Valid unknown
        // fields at both the S2C and response level must be skipped without
        // disturbing the known projection: name is kept and the empty ask/bid
        // lists stay empty.
        let mut s2c = wire_varint_field(9, 7);
        s2c.extend(wire_bytes_field(8, b"Tencent"));
        let mut body = order_book_response_with_s2c(&s2c);
        body.extend({
            let mut unknown = wire_key(10, 5);
            unknown.extend_from_slice(&9_u32.to_le_bytes());
            unknown
        });
        let value = depth_read_raw(body).expect("valid unknown fields are skipped");
        assert_eq!(value["depth"]["name"], "Tencent");
        assert_eq!(value["depth"]["asks"], json!([]));
        assert_eq!(value["depth"]["bids"], json!([]));
    }

    #[test]
    fn depth_read_accepts_a_wire_level_ask_payload_with_required_fields() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook_boundaries_test.go:101
        // TestParseOrderBookLevelAcceptsValidRequiredFields. A level carrying
        // price/volume/orderCount must project into the neutral depth payload
        // without loss; the Rust decoder reads the same proto2 wire layout.
        let mut ask = Vec::new();
        ask.extend(wire_key(1, 1));
        ask.extend_from_slice(&320.5_f64.to_le_bytes());
        ask.extend(wire_varint_field(2, 200));
        ask.extend(wire_varint_field(3, 3));
        let s2c = wire_bytes_field(2, &ask);
        let value = depth_read_raw(order_book_response_with_s2c(&s2c)).expect("valid level");
        assert_eq!(value["depth"]["asks"][0]["price"], 320.5);
        assert_eq!(value["depth"]["asks"][0]["volume"], 200.0);
        assert_eq!(value["depth"]["asks"][0]["orderCount"], 3);
    }

    #[test]
    fn depth_read_rejects_levels_missing_proto2_required_fields() {
        // Parity: go:452dea11:pkg/futu/opend/orderbook.go:97 (parseOrderBookLevel)
        // and go:pkg/futu/opend/orderbook_boundaries_test.go:50. Go decodes each
        // nested level with `proto.Unmarshal`, which fails with RequiredNotSet
        // when price/volume/orderCount are absent. prost cannot observe proto2
        // presence, so without an explicit wire check an empty level would be
        // published as a fake {price: 0, volume: 0, orderCount: 0} row.
        let cases: [(&str, Vec<u8>); 4] = [
            ("empty level", wire_bytes_field(2, &[])),
            ("price only", wire_bytes_field(2, &wire_key(1, 1))),
            (
                "missing order count",
                wire_bytes_field(2, &{
                    let mut level = wire_key(1, 1);
                    level.extend_from_slice(&320.5_f64.to_le_bytes());
                    level.extend(wire_varint_field(2, 200));
                    level
                }),
            ),
            ("empty security", wire_bytes_field(1, &[])),
        ];
        for (name, s2c) in cases {
            let error = depth_read_raw(order_book_response_with_s2c(&s2c)).expect_err(name);
            assert!(
                matches!(error, MarketMicrostructureError::Decode { .. }),
                "{name} must fail closed on a missing proto2 required field, got {error}"
            );
        }
    }

    #[test]
    fn order_book_levels_project_price_volume_count_and_details() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:431
        // TestOrderBookLevelFromPb.
        let levels = order_book_levels(vec![OrderBook {
            price: 175.5,
            volume: 5_000,
            oreder_count: 3,
            detail_list: vec![OrderBookDetail {
                order_id: 12_345,
                volume: 1_000,
            }],
            hp_volume: None,
        }])
        .expect("levels");
        assert_eq!(levels.len(), 1);
        assert_eq!(levels[0]["price"], 175.5);
        assert_eq!(levels[0]["volume"], 5_000.0);
        assert_eq!(levels[0]["orderCount"], 3);
        assert_eq!(levels[0]["detailList"][0]["orderId"], 12_345);
        assert_eq!(levels[0]["detailList"][0]["volume"], 1_000.0);
    }

    #[test]
    fn order_book_levels_return_empty_for_empty_input() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:465
        // TestOrderBookLevelFromPbNil.
        assert!(
            order_book_levels(Vec::new())
                .expect("nil levels")
                .is_empty()
        );
    }

    #[test]
    fn order_book_levels_omit_detail_list_when_absent() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:475
        // TestOrderBookLevelFromPbEmptyDetails.
        let levels = order_book_levels(vec![level(100.0, 200, 1)]).expect("empty details");
        assert_eq!(levels[0]["price"], 100.0);
        assert!(
            levels[0].get("detailList").is_none(),
            "Go omits the detail list when OpenD supplies no entries"
        );
    }

    #[test]
    fn depth_read_projects_name_times_and_levels_from_opend_s2c() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:492
        // TestOrderBookSnapshotFromOpendResult.
        let reader = depth_reader_with_response(depth_response(
            Some("Tencent".to_owned()),
            vec![level(320.0, 100, 1), level(321.0, 200, 2)],
            vec![level(319.0, 150, 1)],
        ));
        let value = reader
            .query(
                MarketMicrostructureOperation::Depth,
                "US.AAPL",
                &json!({"num": 10}),
            )
            .expect("depth");
        assert_eq!(value["depth"]["symbol"], "US.AAPL");
        assert_eq!(value["depth"]["name"], "Tencent");
        assert_eq!(value["depth"]["svrRecvTimeBid"], "2025-01-01 10:00:00.000");
        assert_eq!(value["depth"]["svrRecvTimeAsk"], "2025-01-01 10:00:01.000");
        assert_eq!(value["depth"]["asks"][0]["price"], 320.0);
        assert_eq!(value["depth"]["asks"][1]["price"], 321.0);
        assert_eq!(value["depth"]["bids"][0]["price"], 319.0);
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_depth_test.go:306 TestMarketDepthEmptyOrderBook
    #[test]
    fn depth_read_returns_empty_arrays_for_empty_s2c_lists() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:559
        // TestOrderBookSnapshotFromOpendResultEmptyResult.
        let reader = depth_reader_with_response(depth_response(None, Vec::new(), Vec::new()));
        let value = reader
            .query(
                MarketMicrostructureOperation::Depth,
                "US.AAPL",
                &json!({"num": 10}),
            )
            .expect("empty depth");
        assert_eq!(value["depth"]["bids"], json!([]));
        assert_eq!(value["depth"]["asks"], json!([]));
        assert!(value["depth"].get("name").is_none());
    }

    #[test]
    fn microstructure_reader_rejects_invalid_instruments_before_any_opend_call() {
        // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:17
        // `QueryOrderBook(invalid symbol)` must fail. Rust's single owner is
        // `OpenDMarketMicrostructureReader::security`, which validates the
        // instrument before the coordinator is locked or a frame is written.
        for instrument in ["BAD", "HK.", ".00700", "MARS.AAPL", "CN.600519"] {
            assert!(
                OpenDMarketMicrostructureReader::security(instrument).is_err(),
                "instrument {instrument:?} must be rejected"
            );
        }
        let security = OpenDMarketMicrostructureReader::security(" us.aapl ").expect("valid");
        assert_eq!(security.market, 11);
        assert_eq!(security.code, "AAPL");
    }

    #[test]
    fn order_book_levels_fail_closed_on_negative_volume_and_non_finite_price() {
        let negative = order_book_levels(vec![level(100.0, -1, 1)]);
        assert!(matches!(
            negative,
            Err(MarketMicrostructureError::Decode { message, .. })
                if message.contains("negative depth volume")
        ));
        let non_finite = order_book_levels(vec![level(f64::NAN, 1, 1)]);
        assert!(matches!(
            non_finite,
            Err(MarketMicrostructureError::Decode { .. })
        ));
    }
}
