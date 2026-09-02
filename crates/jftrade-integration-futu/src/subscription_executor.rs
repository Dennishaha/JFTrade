use std::net::SocketAddr;
use std::time::Duration;

use prost::Message;
use thiserror::Error;

use crate::{
    OpenDInitializedSession, OpenDManagedSessionError, OpenDTcpProbeConfig, ReconcileAction,
    SubscriptionKind, TcpTransportError, TransportError,
};

const RET_TYPE_SUCCEED: i32 = 0;

/// Executes one physical subscription action against an already authenticated
/// OpenD TCP session. The executor owns only protocol I/O; demand ownership,
/// generation fencing and retry policy remain in `OpenDSubscriptionLifecycle`.
pub struct OpenDSubscriptionExecutor {
    session: OpenDInitializedSession,
}

impl OpenDSubscriptionExecutor {
    pub fn connect(
        address: SocketAddr,
        timeout: Duration,
    ) -> Result<Self, SubscriptionExecutorError> {
        let config = OpenDTcpProbeConfig::new(address, timeout);
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)?;
        Ok(Self { session })
    }

    pub fn from_session(session: OpenDInitializedSession) -> Self {
        Self { session }
    }

    pub fn session(&self) -> &OpenDInitializedSession {
        &self.session
    }

    pub fn execute(&mut self, action: &ReconcileAction) -> Result<(), SubscriptionExecutorError> {
        let request = QotSubRequest {
            c2s: Some(qot_sub_request(action)?),
        };
        let response = self
            .session
            .managed_session()
            .call(crate::PROTO_QOT_SUB, &request.encode_to_vec())?;
        let response = QotSubResponse::decode(response.as_slice())
            .map_err(SubscriptionExecutorError::Decode)?;
        let ret_type = response.ret_type.unwrap_or(-400);
        if ret_type != RET_TYPE_SUCCEED {
            return Err(SubscriptionExecutorError::Rejected {
                ret_type,
                error_code: response.err_code.unwrap_or_default(),
                message: response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD Qot_Sub request failed".to_owned()),
            });
        }
        Ok(())
    }
}

#[derive(Debug, Error)]
pub enum SubscriptionExecutorError {
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("connect to OpenD: {0}")]
    Connect(#[from] TcpTransportError),
    #[error("OpenD InitConnect handshake: {0}")]
    Handshake(#[from] crate::OpenDTcpProbeError),
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("OpenD Qot_Sub exchange: {0}")]
    Exchange(#[from] TransportError),
    #[error("OpenD managed session: {0}")]
    Session(#[from] OpenDManagedSessionError),
    #[error("decode OpenD Qot_Sub response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_Sub returned retType={ret_type} errCode={error_code}: {message}")]
    Rejected {
        ret_type: i32,
        error_code: i32,
        message: String,
    },
    #[error("invalid OpenD subscription instrument {0:?}")]
    InvalidInstrument(String),
    #[error("unsupported OpenD subscription interval {0:?}")]
    UnsupportedInterval(String),
}

fn qot_sub_request(action: &ReconcileAction) -> Result<QotSubC2s, SubscriptionExecutorError> {
    let (subscription, subscribe) = match action {
        ReconcileAction::Subscribe { subscription } => (subscription, true),
        ReconcileAction::Unsubscribe { subscription } => (subscription, false),
    };
    let (market, code) = split_instrument(&subscription.instrument_id)?;
    let (sub_type, register_push) = match subscription.kind {
        SubscriptionKind::Basic => (1, None),
        SubscriptionKind::Kline => (
            kline_sub_type(subscription.interval.as_deref())?,
            Some(false),
        ),
        SubscriptionKind::OrderBook => (2, Some(false)),
    };
    Ok(QotSubC2s {
        security_list: vec![QotSecurity {
            market: Some(market),
            code: Some(code),
        }],
        sub_type_list: vec![sub_type],
        is_sub_or_un_sub: Some(subscribe),
        is_reg_or_un_reg_push: register_push,
    })
}

pub(crate) fn split_instrument(value: &str) -> Result<(i32, String), SubscriptionExecutorError> {
    let (market, code) = value
        .trim()
        .split_once('.')
        .ok_or_else(|| SubscriptionExecutorError::InvalidInstrument(value.to_owned()))?;
    let market = match market.to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 11,
        "SH" | "CNSH" => 21,
        "SZ" | "CNSZ" => 22,
        "SG" => 31,
        "JP" => 41,
        "AU" => 51,
        "MY" => 61,
        "CA" => 71,
        _ => {
            return Err(SubscriptionExecutorError::InvalidInstrument(
                value.to_owned(),
            ));
        }
    };
    let code = code.trim();
    if code.is_empty() || code.contains('.') {
        return Err(SubscriptionExecutorError::InvalidInstrument(
            value.to_owned(),
        ));
    }
    Ok((market, code.to_ascii_uppercase()))
}

fn kline_sub_type(interval: Option<&str>) -> Result<i32, SubscriptionExecutorError> {
    let interval = interval.unwrap_or_default().trim().to_ascii_lowercase();
    let sub_type = match interval.as_str() {
        "1d" | "day" => 6,
        "5m" => 7,
        "15m" => 8,
        "30m" => 9,
        "60m" | "1h" => 10,
        "1m" => 11,
        "1w" | "week" => 12,
        "1mo" | "month" => 13,
        "3mo" | "quarter" => 15,
        "1y" | "year" => 16,
        "3m" => 17,
        "10m" => 18,
        "120m" => 19,
        "180m" => 20,
        "240m" => 21,
        _ => return Err(SubscriptionExecutorError::UnsupportedInterval(interval)),
    };
    Ok(sub_type)
}

#[derive(Clone, PartialEq, Message)]
struct QotSubRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<QotSubC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSubC2s {
    #[prost(message, repeated, tag = "1")]
    security_list: Vec<QotSecurity>,
    #[prost(int32, repeated, tag = "2")]
    sub_type_list: Vec<i32>,
    #[prost(bool, optional, tag = "3")]
    is_sub_or_un_sub: Option<bool>,
    #[prost(bool, optional, tag = "4")]
    is_reg_or_un_reg_push: Option<bool>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSecurity {
    #[prost(int32, optional, tag = "1")]
    market: Option<i32>,
    #[prost(string, optional, tag = "2")]
    code: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSubResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::Arc;
    use std::thread;

    use super::*;
    use crate::{
        OpenDSessionEvent, OpenDSubscriptionLifecycle, PROTO_UPDATE_BASIC_QOT,
        PhysicalSubscription, decode_frame, encode_frame,
    };
    use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};

    #[derive(Clone, PartialEq, Message)]
    struct InitRequest {
        #[prost(message, optional, tag = "1")]
        c2s: Option<InitRequestState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitRequestState {
        #[prost(bool, optional, tag = "3")]
        recv_notify: Option<bool>,
    }

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

    fn action(
        kind: SubscriptionKind,
        instrument_id: &str,
        interval: Option<&str>,
    ) -> ReconcileAction {
        ReconcileAction::Subscribe {
            subscription: PhysicalSubscription {
                key: "test".to_owned(),
                kind,
                instrument_id: instrument_id.to_owned(),
                interval: interval.map(str::to_owned),
            },
        }
    }

    #[test]
    fn qot_sub_mapping_matches_go_market_and_interval_values() {
        let request = match action(SubscriptionKind::Kline, "HK.00700", Some("1m")) {
            ReconcileAction::Subscribe { subscription } => {
                qot_sub_request(&ReconcileAction::Subscribe { subscription }).expect("request")
            }
            _ => unreachable!(),
        };
        assert_eq!(request.security_list[0].market, Some(1));
        assert_eq!(request.security_list[0].code.as_deref(), Some("00700"));
        assert_eq!(request.sub_type_list, [11]);
        assert_eq!(request.is_sub_or_un_sub, Some(true));
        assert_eq!(request.is_reg_or_un_reg_push, Some(false));

        let basic = qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("basic request");
        assert_eq!(basic.security_list[0].market, Some(11));
        assert_eq!(basic.sub_type_list, [1]);
        assert_eq!(basic.is_reg_or_un_reg_push, None);
    }

    #[test]
    fn executor_sends_subscribe_and_unsubscribe_over_one_framed_session() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            for exchange in 0..3 {
                let mut header = [0_u8; crate::frame::HEADER_LEN];
                stream.read_exact(&mut header).expect("header");
                let body_len =
                    u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
                let mut body = vec![0_u8; body_len];
                stream.read_exact(&mut body).expect("body");
                let frame =
                    decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("frame");
                let response = if exchange == 0 {
                    assert_eq!(frame.header.proto_id, crate::PROTO_INIT_CONNECT);
                    let init = InitRequest::decode(frame.body.as_slice()).expect("init request");
                    assert_eq!(
                        init.c2s.and_then(|state| state.recv_notify),
                        Some(true),
                        "the subscription data session must receive pushes"
                    );
                    InitResponse {
                        ret_type: Some(0),
                        s2c: Some(InitState {
                            server_ver: 1009,
                            conn_id: 1,
                        }),
                    }
                    .encode_to_vec()
                } else {
                    assert_eq!(frame.header.proto_id, crate::PROTO_QOT_SUB);
                    let request = QotSubRequest::decode(frame.body.as_slice()).expect("request");
                    assert_eq!(
                        request.c2s.as_ref().expect("c2s").is_sub_or_un_sub,
                        Some(exchange == 1)
                    );
                    if exchange == 1 {
                        let push =
                            encode_frame(PROTO_UPDATE_BASIC_QOT, 0, b"push").expect("push frame");
                        stream.write_all(&push).expect("write push");
                    }
                    QotSubResponse {
                        ret_type: Some(0),
                        ret_msg: None,
                        err_code: None,
                    }
                    .encode_to_vec()
                };
                let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                    .expect("response");
                stream.write_all(&packet).expect("write response");
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        executor
            .execute(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("subscribe");
        let push = executor
            .session()
            .managed_session()
            .receive_event_timeout(Duration::from_secs(1))
            .expect("push event");
        assert!(matches!(
            push,
            OpenDSessionEvent::UnsolicitedFrame { generation: 1, frame }
                if frame.header.proto_id == PROTO_UPDATE_BASIC_QOT && frame.body == b"push"
        ));
        executor
            .execute(&ReconcileAction::Unsubscribe {
                subscription: PhysicalSubscription {
                    key: "test".to_owned(),
                    kind: SubscriptionKind::Basic,
                    instrument_id: "US.AAPL".to_owned(),
                    interval: None,
                },
            })
            .expect("unsubscribe");
        server.join().expect("server");
    }

    #[test]
    fn executor_maps_qot_sub_rejection_without_reporting_success() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            for exchange in 0..2 {
                let mut header = [0_u8; crate::frame::HEADER_LEN];
                stream.read_exact(&mut header).expect("header");
                let body_len =
                    u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
                let mut body = vec![0_u8; body_len];
                stream.read_exact(&mut body).expect("body");
                let frame =
                    decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("frame");
                let response = if exchange == 0 {
                    InitResponse {
                        ret_type: Some(0),
                        s2c: Some(InitState {
                            server_ver: 1009,
                            conn_id: 2,
                        }),
                    }
                    .encode_to_vec()
                } else {
                    assert_eq!(frame.header.proto_id, crate::PROTO_QOT_SUB);
                    QotSubResponse {
                        ret_type: Some(-3),
                        ret_msg: Some("subscription denied".to_owned()),
                        err_code: Some(1001),
                    }
                    .encode_to_vec()
                };
                let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                    .expect("response");
                stream.write_all(&packet).expect("write response");
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        let error = executor
            .execute(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect_err("rejected subscription");
        assert!(matches!(
            error,
            SubscriptionExecutorError::Rejected {
                ret_type: -3,
                error_code: 1001,
                message
            } if message == "subscription denied"
        ));
        server.join().expect("server");
    }

    #[test]
    fn lifecycle_rejects_stale_executor_before_qot_sub_io() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let mut header = [0_u8; crate::frame::HEADER_LEN];
            stream.read_exact(&mut header).expect("init header");
            let body_len = u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
            let mut body = vec![0_u8; body_len];
            stream.read_exact(&mut body).expect("init body");
            let frame =
                decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("init frame");
            assert_eq!(frame.header.proto_id, crate::PROTO_INIT_CONNECT);
            let response = InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 3,
                }),
            }
            .encode_to_vec();
            let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                .expect("init response");
            stream.write_all(&packet).expect("write init response");

            stream
                .set_read_timeout(Some(Duration::from_millis(200)))
                .expect("read timeout");
            let mut unexpected = [0_u8; 1];
            match stream.read(&mut unexpected) {
                Ok(0) => false,
                Ok(_) => true,
                Err(error)
                    if matches!(
                        error.kind(),
                        std::io::ErrorKind::WouldBlock | std::io::ErrorKind::TimedOut
                    ) =>
                {
                    false
                }
                Err(error) => panic!("read after init: {error}"),
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        assert_eq!(executor.session().managed_session().generation(), 1);

        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(recorder, 0);
        let desired = [InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }];
        lifecycle.reconcile_demand(&desired, 0);
        let replay = lifecycle.reconfigure_for_reconnect(&desired);
        let generation = lifecycle.generation();
        assert_eq!(generation, 2);
        assert_eq!(replay.len(), 1);

        assert!(
            !lifecycle
                .execute_action(&replay[0], 0, generation, &mut executor)
                .expect("stale executor is ignored")
        );
        assert!(
            !server.join().expect("server"),
            "stale executor sent Qot_Sub"
        );
    }

    #[test]
    fn executor_rejects_invalid_instrument_and_unsupported_interval() {
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Basic, "AAPL", None)),
            Err(SubscriptionExecutorError::InvalidInstrument(_))
        ));
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL.EXTRA", None)),
            Err(SubscriptionExecutorError::InvalidInstrument(_))
        ));
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Kline, "US.AAPL", Some("2m"))),
            Err(SubscriptionExecutorError::UnsupportedInterval(_))
        ));
    }
}
