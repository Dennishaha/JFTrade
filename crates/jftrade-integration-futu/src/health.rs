use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;

use prost::Message;
use thiserror::Error;

use crate::{
    OpenDManagedSession, OpenDManagedSessionError, OpenDProbe, PROTO_GET_GLOBAL_STATE,
    PROTO_INIT_CONNECT, TcpTransportError, TransportError, WireGlobalState,
};
use jftrade_marketdata::{HealthStatus, ProviderReadiness};

const RET_TYPE_SUCCEED: i32 = 0;

#[derive(Clone, Debug)]
pub struct OpenDTcpProbeConfig {
    pub address: SocketAddr,
    pub timeout: Duration,
    pub client_id: String,
    pub programming_language: String,
}

impl OpenDTcpProbeConfig {
    pub fn new(address: SocketAddr, timeout: Duration) -> Self {
        Self {
            address,
            timeout,
            client_id: "jftrade-rust".to_owned(),
            programming_language: "Rust".to_owned(),
        }
    }
}

#[derive(Debug, Error)]
pub enum OpenDTcpProbeError {
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("connect to OpenD: {0}")]
    Connect(#[from] TcpTransportError),
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("OpenD protocol exchange: {0}")]
    Exchange(#[from] TransportError),
    #[error("OpenD managed session: {0}")]
    Session(#[from] OpenDManagedSessionError),
    #[error("decode OpenD {operation} response: {source}")]
    Decode {
        operation: &'static str,
        source: prost::DecodeError,
    },
    #[error("OpenD {operation} returned retType={ret_type}: {message}")]
    Rejected {
        operation: &'static str,
        ret_type: i32,
        message: String,
    },
    #[error("OpenD InitConnect response omitted S2C state")]
    MissingInitState,
    #[error("OpenD GetGlobalState response omitted S2C state")]
    MissingGlobalState,
}

pub struct OpenDTcpProbe;

/// An authenticated OpenD session whose single reader can be shared by health,
/// subscription RPC and push consumers without competing socket reads.
#[derive(Clone)]
pub struct OpenDInitializedSession {
    session: Arc<OpenDManagedSession>,
}

impl OpenDInitializedSession {
    pub fn connect(
        config: &OpenDTcpProbeConfig,
        generation: u64,
    ) -> Result<Self, OpenDTcpProbeError> {
        let session = Arc::new(OpenDManagedSession::connect(
            config.address,
            config.timeout,
            generation,
        )?);
        initialize_session(&session, config, false)?;
        Ok(Self { session })
    }

    /// Connects the long-lived market-data role with unsolicited quote
    /// notifications enabled. The health-probe config remains source
    /// compatible; role-specific InitConnect behavior is selected by this
    /// constructor rather than by adding a public config field.
    pub fn connect_with_push_notifications(
        config: &OpenDTcpProbeConfig,
        generation: u64,
    ) -> Result<Self, OpenDTcpProbeError> {
        let session = Arc::new(OpenDManagedSession::connect(
            config.address,
            config.timeout,
            generation,
        )?);
        initialize_session(&session, config, true)?;
        Ok(Self { session })
    }

    pub fn managed_session(&self) -> &OpenDManagedSession {
        &self.session
    }

    pub(crate) fn managed_session_handle(&self) -> Arc<OpenDManagedSession> {
        Arc::clone(&self.session)
    }
}

/// Converts the protocol-neutral OpenD result into the broker-neutral health
/// contract consumed by an explicit `ProviderRouter` composition.
pub fn market_data_health_from_probe(enabled: bool, probe: &OpenDProbe) -> HealthStatus {
    let mut health = HealthStatus {
        readiness: ProviderReadiness::Failed,
        ..HealthStatus::default()
    };
    let error = if !enabled {
        Some("Futu OpenD integration is disabled".to_owned())
    } else if let Some(error) = &probe.last_error {
        Some(error.trim().to_owned())
    } else if probe.connectivity != "connected" || probe.status != "healthy" {
        Some("Futu OpenD is not connected".to_owned())
    } else if probe.quote_logged_in.is_none() {
        Some("Futu OpenD quote session status is unavailable".to_owned())
    } else if probe.quote_logged_in == Some(false) {
        Some("Futu OpenD quote session is not logged in".to_owned())
    } else {
        None
    };
    if let Some(error) = error {
        health.last_error = (!error.is_empty()).then_some(error);
    } else {
        health.connected = true;
        health.readiness = ProviderReadiness::Ready;
    }
    health
}

impl OpenDTcpProbe {
    pub fn probe(config: OpenDTcpProbeConfig) -> Result<OpenDProbe, OpenDTcpProbeError> {
        let session = OpenDInitializedSession::connect(&config, 1)?;
        Self::probe_initialized(&session)
    }

    pub fn probe_initialized(
        session: &OpenDInitializedSession,
    ) -> Result<OpenDProbe, OpenDTcpProbeError> {
        let global_body = GetGlobalStateRequest {
            c2s: Some(GetGlobalStateC2s { user_id: 0 }),
        }
        .encode_to_vec();
        let global_response = session
            .managed_session()
            .call(PROTO_GET_GLOBAL_STATE, &global_body)?;
        let global_response =
            GetGlobalStateResponse::decode(global_response.as_slice()).map_err(|source| {
                OpenDTcpProbeError::Decode {
                    operation: "GetGlobalState",
                    source,
                }
            })?;
        ensure_success(
            "GetGlobalState",
            global_response.ret_type,
            global_response.ret_msg,
        )?;
        let state = global_response
            .s2c
            .ok_or(OpenDTcpProbeError::MissingGlobalState)?;
        let server_version = format_version(state.server_ver, state.server_build_no);
        let version_supported =
            state.server_ver > 1009 || (state.server_ver == 1009 && state.server_build_no >= 6908);
        Ok(OpenDProbe::from_global_state(
            Some(WireGlobalState {
                qot_logged_in: Some(state.qot_logined),
                trade_logged_in: Some(state.trd_logined),
                server_version: Some(server_version),
                program_status: Some(program_status(state.program_status)),
                program_timestamp: Some(format_timestamp(state.time)),
                markets: vec![
                    crate::MarketState {
                        market: "HK".to_owned(),
                        state: state.market_hk,
                    },
                    crate::MarketState {
                        market: "US".to_owned(),
                        state: state.market_us,
                    },
                    crate::MarketState {
                        market: "SH".to_owned(),
                        state: state.market_sh,
                    },
                    crate::MarketState {
                        market: "SZ".to_owned(),
                        state: state.market_sz,
                    },
                ],
            }),
            version_supported,
        ))
    }
}

fn initialize_session(
    session: &OpenDManagedSession,
    config: &OpenDTcpProbeConfig,
    recv_notify: bool,
) -> Result<(), OpenDTcpProbeError> {
    let init_request = InitConnectRequest {
        c2s: Some(InitConnectC2s {
            client_ver: 101,
            client_id: config.client_id.clone(),
            recv_notify: Some(recv_notify),
            programming_language: Some(config.programming_language.clone()),
        }),
    };
    let init_body = init_request.encode_to_vec();
    let init_response = session.call(PROTO_INIT_CONNECT, &init_body)?;
    let init_response =
        InitConnectResponse::decode(init_response.as_slice()).map_err(|source| {
            OpenDTcpProbeError::Decode {
                operation: "InitConnect",
                source,
            }
        })?;
    ensure_success("InitConnect", init_response.ret_type, init_response.ret_msg)?;
    if init_response.s2c.is_none() {
        return Err(OpenDTcpProbeError::MissingInitState);
    }
    Ok(())
}

fn ensure_success(
    operation: &'static str,
    ret_type: Option<i32>,
    ret_msg: Option<String>,
) -> Result<(), OpenDTcpProbeError> {
    let ret_type = ret_type.unwrap_or(-400);
    if ret_type == RET_TYPE_SUCCEED {
        return Ok(());
    }
    Err(OpenDTcpProbeError::Rejected {
        operation,
        ret_type,
        message: ret_msg.unwrap_or_else(|| "OpenD request failed".to_owned()),
    })
}

fn format_version(server_ver: i32, build_no: i32) -> String {
    let major = server_ver / 100;
    let minor = server_ver % 100;
    if build_no > 0 {
        format!("{major}.{minor}.{build_no}")
    } else {
        format!("{major}.{minor}")
    }
}

fn format_timestamp(seconds: i64) -> String {
    time::OffsetDateTime::from_unix_timestamp(seconds)
        .map(|value| {
            value
                .to_offset(time::UtcOffset::UTC)
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_default()
        })
        .unwrap_or_default()
}

fn program_status(status: Option<ProgramStatus>) -> String {
    let Some(status) = status else {
        return "Unavailable".to_owned();
    };
    let label = match status.r#type {
        1 => "Loaded",
        2 => "Loging",
        3 => "NeedPicVerifyCode",
        4 => "NeedPhoneVerifyCode",
        5 => "LoginFailed",
        6 => "ForceUpdate",
        7 => "NessaryDataPreparing",
        8 => "NessaryDataMissing",
        9 => "UnAgreeDisclaimer",
        10 => "Ready",
        11 => "ForceLogout",
        12 => "DisclaimerPullFailed",
        _ => "Unavailable",
    };
    status
        .str_ext_desc
        .filter(|value| !value.trim().is_empty())
        .map_or_else(
            || label.to_owned(),
            |value| format!("{label}: {}", value.trim()),
        )
}

#[derive(Clone, PartialEq, Message)]
struct InitConnectRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<InitConnectC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct InitConnectC2s {
    #[prost(int32, tag = "1")]
    client_ver: i32,
    #[prost(string, tag = "2")]
    client_id: String,
    #[prost(bool, optional, tag = "3")]
    recv_notify: Option<bool>,
    #[prost(string, optional, tag = "6")]
    programming_language: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct InitConnectResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<InitConnectS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct InitConnectS2c {
    #[prost(int32, tag = "1")]
    server_ver: i32,
    #[prost(uint64, tag = "3")]
    conn_id: u64,
}

#[derive(Clone, PartialEq, Message)]
struct GetGlobalStateRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<GetGlobalStateC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct GetGlobalStateC2s {
    #[prost(uint64, tag = "1")]
    user_id: u64,
}

#[derive(Clone, PartialEq, Message)]
struct GetGlobalStateResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<GetGlobalStateS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct GetGlobalStateS2c {
    #[prost(int32, tag = "1")]
    market_hk: i32,
    #[prost(int32, tag = "2")]
    market_us: i32,
    #[prost(int32, tag = "3")]
    market_sh: i32,
    #[prost(int32, tag = "4")]
    market_sz: i32,
    #[prost(bool, tag = "6")]
    qot_logined: bool,
    #[prost(bool, tag = "7")]
    trd_logined: bool,
    #[prost(int32, tag = "8")]
    server_ver: i32,
    #[prost(int32, tag = "9")]
    server_build_no: i32,
    #[prost(int64, tag = "10")]
    time: i64,
    #[prost(message, optional, tag = "12")]
    program_status: Option<ProgramStatus>,
}

#[derive(Clone, PartialEq, Message)]
struct ProgramStatus {
    #[prost(int32, tag = "1")]
    r#type: i32,
    #[prost(string, optional, tag = "2")]
    str_ext_desc: Option<String>,
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::thread;

    use super::*;
    use crate::frame::HEADER_LEN;
    use crate::{Frame, decode_frame, encode_frame};

    #[test]
    fn tcp_probe_maps_login_global_state_and_market_readiness() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init_request = read_request(&mut stream);
            assert_eq!(init_request.header.proto_id, PROTO_INIT_CONNECT);
            let init =
                InitConnectRequest::decode(init_request.body.as_slice()).expect("init request");
            assert_eq!(
                init.c2s.and_then(|state| state.recv_notify),
                Some(false),
                "the short-lived Go-compatible health probe does not receive pushes"
            );
            let init_response = InitConnectResponse {
                ret_type: Some(RET_TYPE_SUCCEED),
                ret_msg: None,
                s2c: Some(InitConnectS2c {
                    server_ver: 1009,
                    conn_id: 7,
                }),
            };
            write_response(&mut stream, &init_request, init_response.encode_to_vec());

            let global_request = read_request(&mut stream);
            assert_eq!(global_request.header.proto_id, PROTO_GET_GLOBAL_STATE);
            let global_response = GetGlobalStateResponse {
                ret_type: Some(RET_TYPE_SUCCEED),
                ret_msg: None,
                s2c: Some(GetGlobalStateS2c {
                    market_hk: 3,
                    market_us: 4,
                    market_sh: 5,
                    market_sz: 6,
                    qot_logined: true,
                    trd_logined: false,
                    server_ver: 1009,
                    server_build_no: 7000,
                    time: 1_754_000_000,
                    program_status: Some(ProgramStatus {
                        r#type: 10,
                        str_ext_desc: None,
                    }),
                }),
            };
            write_response(
                &mut stream,
                &global_request,
                global_response.encode_to_vec(),
            );
        });

        let probe = OpenDTcpProbe::probe(OpenDTcpProbeConfig::new(address, Duration::from_secs(1)))
            .expect("probe");
        assert_eq!(probe.connectivity, "connected");
        assert_eq!(probe.status, "healthy");
        assert_eq!(probe.server_version.as_deref(), Some("10.9.7000"));
        assert_eq!(probe.program_status.as_deref(), Some("Ready"));
        assert_eq!(probe.quote_logged_in, Some(true));
        assert!(probe.market_data_ready());
        assert_eq!(probe.markets.len(), 4);
        server.join().expect("server thread");
    }

    #[test]
    fn initialized_probe_and_subscription_rpc_share_one_managed_reader() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init_request = read_request(&mut stream);
            let init =
                InitConnectRequest::decode(init_request.body.as_slice()).expect("init request");
            assert_eq!(
                init.c2s.and_then(|state| state.recv_notify),
                Some(true),
                "the long-lived data session must receive pushes"
            );
            write_response(
                &mut stream,
                &init_request,
                InitConnectResponse {
                    ret_type: Some(RET_TYPE_SUCCEED),
                    ret_msg: None,
                    s2c: Some(InitConnectS2c {
                        server_ver: 1009,
                        conn_id: 17,
                    }),
                }
                .encode_to_vec(),
            );
            let global_request = read_request(&mut stream);
            let push = encode_frame(crate::PROTO_UPDATE_BASIC_QOT, 0, b"push").expect("push frame");
            stream.write_all(&push).expect("write push");
            write_response(
                &mut stream,
                &global_request,
                GetGlobalStateResponse {
                    ret_type: Some(RET_TYPE_SUCCEED),
                    ret_msg: None,
                    s2c: Some(GetGlobalStateS2c {
                        market_hk: 3,
                        market_us: 4,
                        market_sh: 5,
                        market_sz: 6,
                        qot_logined: true,
                        trd_logined: false,
                        server_ver: 1009,
                        server_build_no: 7000,
                        time: 1_754_000_000,
                        program_status: Some(ProgramStatus {
                            r#type: 10,
                            str_ext_desc: None,
                        }),
                    }),
                }
                .encode_to_vec(),
            );
            let subscription_request = read_request(&mut stream);
            assert_eq!(subscription_request.header.proto_id, crate::PROTO_QOT_SUB);
            write_response(&mut stream, &subscription_request, vec![0x08, 0x00]);
        });

        let config = OpenDTcpProbeConfig::new(address, Duration::from_secs(1));
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 7)
            .expect("initialized session");
        let probe = OpenDTcpProbe::probe_initialized(&session).expect("probe");
        assert!(probe.market_data_ready());
        assert!(matches!(
            session
                .managed_session()
                .receive_event_timeout(Duration::from_secs(1))
                .expect("push event"),
            crate::OpenDSessionEvent::UnsolicitedFrame { generation: 7, frame }
                if frame.header.proto_id == crate::PROTO_UPDATE_BASIC_QOT
                    && frame.body == b"push"
        ));
        let mut executor = crate::OpenDSubscriptionExecutor::from_session(session.clone());
        executor
            .execute(&crate::ReconcileAction::Subscribe {
                subscription: crate::PhysicalSubscription {
                    key: "BASIC:US.AAPL".to_owned(),
                    kind: crate::SubscriptionKind::Basic,
                    instrument_id: "US.AAPL".to_owned(),
                    interval: None,
                },
            })
            .expect("subscription RPC");
        server.join().expect("server thread");
    }

    #[test]
    fn tcp_probe_preserves_opend_rejection_and_unsupported_version() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init_request = read_request(&mut stream);
            let init_response = InitConnectResponse {
                ret_type: Some(-1),
                ret_msg: Some("login required".to_owned()),
                s2c: None,
            };
            write_response(&mut stream, &init_request, init_response.encode_to_vec());
        });
        let error = OpenDTcpProbe::probe(OpenDTcpProbeConfig::new(address, Duration::from_secs(1)))
            .expect_err("rejection should not be swallowed");
        assert!(matches!(
            error,
            OpenDTcpProbeError::Rejected {
                operation: "InitConnect",
                ..
            }
        ));
        server.join().expect("server thread");

        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init_request = read_request(&mut stream);
            write_response(
                &mut stream,
                &init_request,
                InitConnectResponse {
                    ret_type: Some(RET_TYPE_SUCCEED),
                    ret_msg: None,
                    s2c: Some(InitConnectS2c {
                        server_ver: 1009,
                        conn_id: 7,
                    }),
                }
                .encode_to_vec(),
            );
            let global_request = read_request(&mut stream);
            write_response(
                &mut stream,
                &global_request,
                GetGlobalStateResponse {
                    ret_type: Some(RET_TYPE_SUCCEED),
                    ret_msg: None,
                    s2c: Some(GetGlobalStateS2c {
                        market_hk: 0,
                        market_us: 0,
                        market_sh: 0,
                        market_sz: 0,
                        qot_logined: false,
                        trd_logined: false,
                        server_ver: 1009,
                        server_build_no: 6000,
                        time: 1_754_000_000,
                        program_status: None,
                    }),
                }
                .encode_to_vec(),
            );
        });
        let probe = OpenDTcpProbe::probe(OpenDTcpProbeConfig::new(address, Duration::from_secs(1)))
            .expect("unsupported version probe");
        assert_eq!(probe.status, "degraded");
        assert_eq!(
            probe.issue_code.as_deref(),
            Some("OPEND_VERSION_UNSUPPORTED")
        );
        server.join().expect("server thread");
    }

    #[test]
    fn probe_health_projection_drives_an_explicit_router_without_default_owner() {
        let healthy_probe = OpenDProbe::from_global_state(
            Some(WireGlobalState {
                qot_logged_in: Some(true),
                trade_logged_in: Some(false),
                server_version: Some("10.9.7000".to_owned()),
                program_status: Some("Ready".to_owned()),
                program_timestamp: None,
                markets: Vec::new(),
            }),
            true,
        );
        let healthy = market_data_health_from_probe(true, &healthy_probe);
        assert!(healthy.is_ready());

        let mut router = jftrade_marketdata::ProviderRouter::new(2);
        router
            .register(crate::provider_descriptor(), healthy)
            .expect("Futu descriptor");
        router
            .activate("futu", jftrade_marketdata::ActivationMode::Explicit)
            .expect("activate healthy provider");

        let disconnected = OpenDProbe::disconnected("mock socket closed");
        let failed = market_data_health_from_probe(true, &disconnected);
        assert!(!failed.connected);
        assert_eq!(failed.readiness, ProviderReadiness::Failed);
        router.update_health("futu", failed).expect("update health");
        assert!(!router.runtime().connected);
        assert_eq!(router.runtime().readiness, ProviderReadiness::Failed);
    }

    fn read_request(stream: &mut TcpStream) -> Frame {
        let mut header = [0_u8; HEADER_LEN];
        stream.read_exact(&mut header).expect("request header");
        let body_len = u32::from_le_bytes(header[12..16].try_into().expect("body length")) as usize;
        let mut packet = Vec::with_capacity(HEADER_LEN + body_len);
        packet.extend_from_slice(&header);
        packet.resize(HEADER_LEN + body_len, 0);
        stream
            .read_exact(&mut packet[HEADER_LEN..])
            .expect("request body");
        decode_frame(&packet).expect("request frame")
    }

    fn write_response(stream: &mut TcpStream, request: &Frame, body: Vec<u8>) {
        let packet = encode_frame(request.header.proto_id, request.header.serial_no, &body)
            .expect("response frame");
        stream.write_all(&packet).expect("response");
    }
}
