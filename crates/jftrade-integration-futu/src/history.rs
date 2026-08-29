//! Typed OpenD historical K-line (Qot_RequestHistoryKL/3103) adapter.

use std::sync::{Arc, Mutex};

use prost::Message;
use thiserror::Error;

use crate::{OpenDSessionCoordinator, OpenDSessionCoordinatorError, PROTO_REQUEST_HISTORY_KL};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct HistoricalSecurity {
    pub market: i32,
    pub code: String,
}

#[derive(Clone, Debug, PartialEq)]
pub struct HistoricalKline {
    pub time: String,
    pub is_blank: bool,
    pub high_price: Option<f64>,
    pub open_price: Option<f64>,
    pub low_price: Option<f64>,
    pub close_price: Option<f64>,
    pub volume: Option<i64>,
    pub turnover: Option<f64>,
    pub change_rate: Option<f64>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct HistoricalKlineQuery {
    pub market: i32,
    pub symbol: String,
    pub period: String,
    pub adjustment: i32,
    pub begin_time: String,
    pub end_time: String,
    pub max_ack_kl_num: Option<i32>,
    pub next_req_key: Vec<u8>,
    pub extended_time: Option<bool>,
    pub session: Option<i32>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct HistoricalKlineResult {
    pub security: HistoricalSecurity,
    pub name: Option<String>,
    pub klines: Vec<HistoricalKline>,
    pub next_req_key: Vec<u8>,
}

pub trait HistoricalKlineReadPort: Send + Sync + std::fmt::Debug {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError>;
}

#[derive(Debug, Error)]
pub enum HistoricalKlineError {
    #[error("OpenD session unavailable: {0}")]
    Session(#[from] OpenDSessionCoordinatorError),
    #[error("decode OpenD Qot_RequestHistoryKL response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_RequestHistoryKL retType={ret_type} errCode={err_code}: {message}")]
    Rejected {
        ret_type: i32,
        err_code: i32,
        message: String,
    },
    #[error("OpenD Qot_RequestHistoryKL response missing s2c")]
    MissingS2c,
}

/// Read-only adapter over the coordinator's authenticated managed OpenD session.
#[derive(Clone)]
pub struct OpenDHistoricalKlineReader {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl std::fmt::Debug for OpenDHistoricalKlineReader {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("OpenDHistoricalKlineReader")
            .finish_non_exhaustive()
    }
}

impl OpenDHistoricalKlineReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self { coordinator }
    }
}

impl HistoricalKlineReadPort for OpenDHistoricalKlineReader {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|_| OpenDSessionCoordinatorError::Closed)?;
        let session = coordinator.session()?;
        let body = encode_request(query);
        let body = session
            .managed_session()
            .call(PROTO_REQUEST_HISTORY_KL, &body)
            .map_err(OpenDSessionCoordinatorError::Session)?;
        let response = HistoryResponse::decode(body.as_slice())?;
        let ret_type = response.ret_type.unwrap_or(-400);
        if ret_type != 0 {
            return Err(HistoricalKlineError::Rejected {
                ret_type,
                err_code: response.err_code.unwrap_or_default(),
                message: response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD historical K-line request failed".to_owned()),
            });
        }
        let s2c = response.s2c.ok_or(HistoricalKlineError::MissingS2c)?;
        Ok(HistoricalKlineResult {
            security: s2c
                .security
                .map(|security| HistoricalSecurity {
                    market: security.market,
                    code: security.code,
                })
                .unwrap_or_else(|| query_security(query)),
            name: s2c.name,
            klines: s2c
                .kl_list
                .into_iter()
                .map(|kline| HistoricalKline {
                    time: kline.time,
                    is_blank: kline.is_blank,
                    high_price: kline.high_price,
                    open_price: kline.open_price,
                    low_price: kline.low_price,
                    close_price: kline.close_price,
                    volume: kline.volume,
                    turnover: kline.turnover,
                    change_rate: kline.change_rate,
                })
                .collect(),
            next_req_key: s2c.next_req_key.unwrap_or_default(),
        })
    }
}

fn query_security(query: &HistoricalKlineQuery) -> HistoricalSecurity {
    HistoricalSecurity {
        market: query.market,
        code: query.symbol.clone(),
    }
}

fn period_code(period: &str) -> i32 {
    match period {
        "1m" => 1,
        "1d" => 2,
        "1w" => 3,
        "1mo" => 4,
        "3m" => 10,
        "5m" => 6,
        "10m" => 12,
        "15m" => 7,
        "30m" => 8,
        "1h" => 9,
        _ => 2,
    }
}

fn encode_request(query: &HistoricalKlineQuery) -> Vec<u8> {
    HistoryRequest {
        c2s: Some(HistoryC2s {
            rehab_type: Some(query.adjustment),
            kl_type: Some(period_code(&query.period)),
            security: Some(crate::trade_proto::qot_common::Security {
                market: query.market,
                code: query.symbol.clone(),
            }),
            begin_time: Some(query.begin_time.clone()),
            end_time: Some(query.end_time.clone()),
            max_ack_kl_num: query.max_ack_kl_num,
            next_req_key: (!query.next_req_key.is_empty()).then(|| query.next_req_key.clone()),
            extended_time: query.extended_time,
            session: query.session,
            need_kl_fields_flag: None,
            header: None,
        }),
    }
    .encode_to_vec()
}

#[derive(Clone, PartialEq, Message)]
struct HistoryRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<HistoryC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct HistoryC2s {
    #[prost(int32, optional, tag = "1")]
    rehab_type: Option<i32>,
    #[prost(int32, optional, tag = "2")]
    kl_type: Option<i32>,
    #[prost(message, optional, tag = "3")]
    security: Option<crate::trade_proto::qot_common::Security>,
    #[prost(string, optional, tag = "4")]
    begin_time: Option<String>,
    #[prost(string, optional, tag = "5")]
    end_time: Option<String>,
    #[prost(int32, optional, tag = "6")]
    max_ack_kl_num: Option<i32>,
    #[prost(int64, optional, tag = "7")]
    need_kl_fields_flag: Option<i64>,
    #[prost(bytes, optional, tag = "8")]
    next_req_key: Option<Vec<u8>>,
    #[prost(bool, optional, tag = "9")]
    extended_time: Option<bool>,
    #[prost(int32, optional, tag = "10")]
    session: Option<i32>,
    #[prost(message, optional, tag = "100")]
    header: Option<crate::trade_proto::qot_common::QotHeader>,
}

#[derive(Clone, PartialEq, Message)]
struct HistoryResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<HistoryS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct HistoryS2c {
    #[prost(message, optional, tag = "1")]
    security: Option<crate::trade_proto::qot_common::Security>,
    #[prost(message, repeated, tag = "2")]
    kl_list: Vec<crate::trade_proto::qot_common::KLine>,
    #[prost(bytes, optional, tag = "3")]
    next_req_key: Option<Vec<u8>>,
    #[prost(string, optional, tag = "4")]
    name: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn history_request_preserves_wire_pagination_and_extended_session_fields() {
        let query = HistoricalKlineQuery {
            market: 11,
            symbol: "AAPL".to_owned(),
            period: "5m".to_owned(),
            adjustment: 1,
            begin_time: "2026-08-01 00:00:00".to_owned(),
            end_time: "2026-08-02 00:00:00".to_owned(),
            max_ack_kl_num: Some(200),
            next_req_key: vec![1, 2, 3],
            extended_time: Some(true),
            session: Some(3),
        };
        let decoded = HistoryRequest::decode(encode_request(&query).as_slice()).expect("request");
        let c2s = decoded.c2s.expect("c2s");
        assert_eq!(c2s.rehab_type, Some(1));
        assert_eq!(c2s.kl_type, Some(6));
        assert_eq!(c2s.security.expect("security").code, "AAPL");
        assert_eq!(c2s.next_req_key, Some(vec![1, 2, 3]));
        assert_eq!(c2s.extended_time, Some(true));
        assert_eq!(c2s.session, Some(3));
    }

    #[test]
    fn history_response_maps_rejection_without_fabricating_s2c() {
        let response = HistoryResponse {
            ret_type: Some(-1),
            ret_msg: Some("history rate limited".to_owned()),
            err_code: Some(429),
            s2c: None,
        };
        let decoded =
            HistoryResponse::decode(response.encode_to_vec().as_slice()).expect("response");
        assert_eq!(decoded.ret_type, Some(-1));
        assert!(decoded.s2c.is_none());
    }

    #[test]
    fn history_wire_frame_keeps_protocol_and_serial_for_mock_opend() {
        let body = HistoryResponse {
            ret_type: Some(0),
            ret_msg: None,
            err_code: None,
            s2c: Some(HistoryS2c {
                security: None,
                kl_list: Vec::new(),
                next_req_key: Some(vec![9]),
                name: Some("Apple".to_owned()),
            }),
        }
        .encode_to_vec();
        let frame = crate::decode_frame(
            &crate::encode_frame(PROTO_REQUEST_HISTORY_KL, 7, &body).expect("frame"),
        )
        .expect("decode frame");
        assert_eq!(frame.header.proto_id, PROTO_REQUEST_HISTORY_KL);
        assert_eq!(frame.header.serial_no, 7);
        let decoded = HistoryResponse::decode(frame.body.as_slice()).expect("response");
        assert_eq!(decoded.s2c.expect("s2c").next_req_key, Some(vec![9]));
    }
}
