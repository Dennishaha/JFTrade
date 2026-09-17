//! Socket-level regressions for the OpenD market-read boundaries.
//!
//! Parity: go:pkg/futu/opend/market_read_boundaries_test.go. Go runs these
//! against a scripted server so the client's real framing, request encoding
//! and response projection are exercised together. The same is done here with
//! a framed loopback server; nothing is asserted through private helpers.

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDHistoricalKlineReader, OpenDInstrumentSearchReader, OpenDSessionCoordinator,
    OpenDSubscriptionLifecycle, OpenDTcpProbeConfig, PROTO_GET_KL, PROTO_GET_STATIC_INFO,
    PROTO_INIT_CONNECT, PROTO_REQUEST_HISTORY_KL, decode_frame, encode_frame,
};
use jftrade_marketdata::MarketDataRuntimeRecorder;
use prost::Message;

#[derive(Clone, PartialEq, Message)]
struct InitResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
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

fn read_framed_frame(stream: &mut TcpStream) -> Option<Frame> {
    let mut header = [0u8; 44];
    stream.read_exact(&mut header).ok()?;
    let mut body_len_bytes = [0u8; 4];
    body_len_bytes.copy_from_slice(&header[12..16]);
    let body_len = u32::from_le_bytes(body_len_bytes) as usize;
    let mut packet = vec![0u8; 44 + body_len];
    packet[..44].copy_from_slice(&header);
    stream.read_exact(&mut packet[44..]).ok()?;
    decode_frame(&packet).ok()
}

fn write_framed_response(stream: &mut TcpStream, proto_id: u32, serial_no: u32, body: &[u8]) {
    let packet = encode_frame(proto_id, serial_no, body).expect("encode frame");
    stream.write_all(&packet).expect("write frame");
}

/// Serve the InitConnect handshake and then answer each expected protocol with
/// the supplied body, recording the decoded request frames for assertions.
fn scripted_server(
    expected: Vec<(u32, Vec<u8>)>,
) -> (
    std::net::SocketAddr,
    Arc<Mutex<Vec<Frame>>>,
    thread::JoinHandle<()>,
) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let requests = Arc::new(Mutex::new(Vec::new()));
    let recorded = Arc::clone(&requests);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 7,
                }),
            }
            .encode_to_vec(),
        );
        for (proto_id, body) in expected {
            let frame = read_framed_frame(&mut stream).expect("request frame");
            assert_eq!(frame.header.proto_id, proto_id, "request protocol order");
            recorded.lock().expect("requests").push(frame.clone());
            write_framed_response(&mut stream, proto_id, frame.header.serial_no, &body);
        }
        // The scripted exchange is complete; close the socket so the client
        // observes a peer close instead of the test joining a blocked reader.
        let _ = stream.shutdown(std::net::Shutdown::Both);
    });
    (address, requests, server)
}

/// Serve the InitConnect handshake, then answer every subsequent request with a
/// generic success response, recording frames until the client closes.
fn scripted_server_until_close() -> (
    std::net::SocketAddr,
    Arc<Mutex<Vec<Frame>>>,
    thread::JoinHandle<()>,
) {
    #[derive(Clone, PartialEq, Message)]
    struct AnyResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let requests = Arc::new(Mutex::new(Vec::new()));
    let recorded = Arc::clone(&requests);
    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        let init = read_framed_frame(&mut stream).expect("init frame");
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 7,
                }),
            }
            .encode_to_vec(),
        );
        while let Some(frame) = read_framed_frame(&mut stream) {
            recorded.lock().expect("requests").push(frame.clone());
            write_framed_response(
                &mut stream,
                frame.header.proto_id,
                frame.header.serial_no,
                &AnyResponse { ret_type: Some(0) }.encode_to_vec(),
            );
        }
    });
    (address, requests, server)
}

fn coordinator(address: std::net::SocketAddr) -> Arc<Mutex<OpenDSessionCoordinator>> {
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        Vec::new(),
        0,
    )
    .expect("connect");
    Arc::new(Mutex::new(coordinator))
}

#[test]
fn history_optional_fields_round_trip_and_missing_s2c_is_an_empty_result() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:77
    // TestRequestHistoryKLEncodesOptionalFieldsAndHandlesEmptyResult
    use jftrade_integration_futu::{
        HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort,
    };

    #[derive(Clone, PartialEq, Message)]
    struct Response {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<S2c>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct S2c {
        #[prost(message, optional, tag = "1")]
        security: Option<jftrade_integration_futu::trade_proto::qot_common::Security>,
        #[prost(message, repeated, tag = "2")]
        kl_list: Vec<jftrade_integration_futu::trade_proto::qot_common::KLine>,
        #[prost(bytes, optional, tag = "3")]
        next_req_key: Option<Vec<u8>>,
        #[prost(string, optional, tag = "4")]
        name: Option<String>,
    }

    let empty = Response {
        ret_type: Some(0),
        ret_msg: None,
        err_code: None,
        s2c: None,
    }
    .encode_to_vec();
    let (address, requests, server) = scripted_server(vec![(PROTO_REQUEST_HISTORY_KL, empty)]);
    let reader = OpenDHistoricalKlineReader::new(coordinator(address));
    let result = reader
        .query(&HistoricalKlineQuery {
            market: 1,
            symbol: "00700".to_owned(),
            period: "15m".to_owned(),
            adjustment: 1,
            begin_time: "2026-06-01".to_owned(),
            end_time: "2026-06-30".to_owned(),
            max_ack_kl_num: Some(250),
            next_req_key: b"page-2".to_vec(),
            extended_time: Some(true),
            session: Some(1),
        })
        .expect("payload-less ack is an empty page");
    assert_eq!(result.security.code, "00700");
    assert!(result.name.is_none());
    assert!(result.klines.is_empty());
    assert!(result.next_req_key.is_empty());

    // The optional history fields still reach the wire unchanged.
    #[derive(Clone, PartialEq, Message)]
    struct Request {
        #[prost(message, optional, tag = "1")]
        c2s: Option<C2s>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct C2s {
        #[prost(int32, optional, tag = "1")]
        rehab_type: Option<i32>,
        #[prost(int32, optional, tag = "2")]
        kl_type: Option<i32>,
        #[prost(message, optional, tag = "3")]
        security: Option<jftrade_integration_futu::trade_proto::qot_common::Security>,
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
    }
    let requests = requests.lock().expect("requests");
    let request = Request::decode(requests[0].body.as_slice())
        .expect("history request")
        .c2s
        .expect("c2s");
    assert_eq!(request.security.expect("security").code, "00700");
    assert_eq!(request.begin_time.as_deref(), Some("2026-06-01"));
    assert_eq!(request.end_time.as_deref(), Some("2026-06-30"));
    assert_eq!(request.max_ack_kl_num, Some(250));
    assert_eq!(request.next_req_key.as_deref(), Some(b"page-2".as_slice()));
    assert_eq!(request.extended_time, Some(true));
    assert_eq!(request.session, Some(1));
    drop(requests);
    server.join().expect("server");

    // A non-zero retType keeps the OpenD business error instead of an empty page.
    let rejected = Response {
        ret_type: Some(-2),
        ret_msg: Some("history rate limited".to_owned()),
        err_code: Some(429),
        s2c: None,
    }
    .encode_to_vec();
    let (address, _, server) = scripted_server(vec![(PROTO_REQUEST_HISTORY_KL, rejected)]);
    let reader = OpenDHistoricalKlineReader::new(coordinator(address));
    let error = reader
        .query(&HistoricalKlineQuery {
            market: 1,
            symbol: "00700".to_owned(),
            period: "1d".to_owned(),
            adjustment: 0,
            begin_time: String::new(),
            end_time: String::new(),
            max_ack_kl_num: None,
            next_req_key: Vec::new(),
            extended_time: None,
            session: None,
        })
        .expect_err("rejection must stay an error");
    assert!(matches!(
        error,
        HistoricalKlineError::Rejected {
            ret_type: -2,
            err_code: 429,
            ..
        }
    ));
    server.join().expect("server");
}

#[test]
fn security_info_methods_return_empty_collections_for_a_payload_less_ack() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:192
    // TestSecurityInfoMethodsReturnEmptyCollectionsForEmptyOpenDResults
    use jftrade_integration_futu::InstrumentSearchReadPort;

    #[derive(Clone, PartialEq, Message)]
    struct StaticInfoResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct SnapshotResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }

    let (address, _, server) = scripted_server(vec![
        (
            PROTO_GET_STATIC_INFO,
            StaticInfoResponse { ret_type: Some(0) }.encode_to_vec(),
        ),
        (
            jftrade_integration_futu::PROTO_GET_SECURITY_SNAPSHOT,
            SnapshotResponse { ret_type: Some(0) }.encode_to_vec(),
        ),
    ]);
    let handle = coordinator(address);
    let reader = OpenDInstrumentSearchReader::new(Arc::clone(&handle));
    let entries = reader
        .lookup("HK", "00700")
        .expect("missing s2c is an empty collection");
    assert!(entries.is_empty());

    let snapshots = jftrade_integration_futu::OpenDSecuritySnapshotReader::new(handle)
        .query(&["HK.00700".to_owned()])
        .expect("missing s2c is an empty collection");
    assert!(snapshots.is_empty());
    server.join().expect("server");
}

#[test]
fn stale_or_malformed_push_updates_never_reach_the_lifecycle() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:284
    // TestMarketPushSubscribersIgnoreMalformedAndUnsuccessfulUpdates
    use jftrade_integration_futu::{
        OpenDSessionEvent, OpenDSessionEventPump, PROTO_UPDATE_BASIC_QOT, QuotePushDecodeError,
    };
    use jftrade_kernel::WireTimestamp;

    let (address, _, server) = scripted_server(Vec::new());
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        Vec::new(),
        0,
    )
    .expect("connect");
    let session = coordinator.session_clone().expect("session");
    let lifecycle = Arc::new(OpenDSubscriptionLifecycle::new(
        coordinator.recorder(),
        60_000,
    ));
    let pump = OpenDSessionEventPump::new(session);

    #[derive(Clone, PartialEq, Message)]
    struct BasicResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
    }
    let rejected = jftrade_integration_futu::encode_frame(
        PROTO_UPDATE_BASIC_QOT,
        0,
        &BasicResponse {
            ret_type: Some(-1),
            ret_msg: Some("stale subscription".to_owned()),
        }
        .encode_to_vec(),
    )
    .expect("frame");
    assert!(matches!(
        jftrade_integration_futu::decode_quote_push(
            &jftrade_integration_futu::decode_frame(&rejected).expect("decode")
        ),
        Ok(None)
    ));

    // A malformed known push is surfaced as a typed decode error so the stream
    // owner can reconnect, never as a successful push.
    let malformed =
        jftrade_integration_futu::encode_frame(PROTO_UPDATE_BASIC_QOT, 0, &[0xff]).expect("frame");
    assert!(matches!(
        jftrade_integration_futu::decode_quote_push(
            &jftrade_integration_futu::decode_frame(&malformed).expect("decode")
        ),
        Err(QuotePushDecodeError::Decode {
            protocol: PROTO_UPDATE_BASIC_QOT,
            ..
        })
    ));

    // The event pump drops payload-less updates instead of emitting a push.
    let now = WireTimestamp::from_offset_datetime(
        ::time::OffsetDateTime::from_unix_timestamp(1_786_000_000).expect("timestamp"),
    );
    let event = OpenDSessionEvent::UnsolicitedFrame {
        generation: lifecycle.generation(),
        frame: jftrade_integration_futu::decode_frame(
            &jftrade_integration_futu::encode_frame(
                PROTO_UPDATE_BASIC_QOT,
                0,
                &BasicResponse {
                    ret_type: Some(0),
                    ret_msg: None,
                }
                .encode_to_vec(),
            )
            .expect("frame"),
        )
        .expect("decode frame"),
    };
    let push = lifecycle
        .ingest_session_event(&event, now)
        .expect("lifecycle ingests the frame");
    assert!(
        push.is_none(),
        "payload-less update must not surface a push"
    );
    let _ = pump;
    let mut coordinator = coordinator;
    let _ = coordinator.close();
    server.join().expect("server");
}

#[test]
fn market_read_business_errors_keep_opend_return_details() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:129
    // TestMarketReadMethodsPropagateOpenDBusinessErrors
    use jftrade_integration_futu::{
        CurrentKlineError, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort,
        InstrumentSearchReadPort, SecuritySnapshotQueryError,
    };

    // Current kline entitlement (Qot_GetKL 3006).
    #[derive(Clone, PartialEq, Message)]
    struct GetKlResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
    }
    let (address, _, server) = scripted_server(vec![(
        PROTO_GET_KL,
        GetKlResponse {
            ret_type: Some(-1),
            ret_msg: Some("kline entitlement denied".to_owned()),
            err_code: Some(9),
        }
        .encode_to_vec(),
    )]);
    let handle = coordinator(address);
    let session = handle
        .lock()
        .expect("coordinator")
        .session_clone()
        .expect("session");
    let error = jftrade_integration_futu::query_current_klines(
        &session,
        &jftrade_integration_futu::CurrentKlineQuery::new(1, "00700", "1d"),
        Duration::from_secs(2),
    )
    .expect_err("entitlement rejection must surface");
    assert!(matches!(
        error,
        CurrentKlineError::Rejected {
            ret_type: -1,
            err_code: 9,
            ref message,
        } if message == "kline entitlement denied"
    ));
    server.join().expect("server");

    // Historical rate limit (Qot_RequestHistoryKL 3103).
    #[derive(Clone, PartialEq, Message)]
    struct HistoryResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
    }
    let (address, _, server) = scripted_server(vec![(
        PROTO_REQUEST_HISTORY_KL,
        HistoryResponse {
            ret_type: Some(-2),
            ret_msg: Some("history rate limited".to_owned()),
            err_code: Some(429),
        }
        .encode_to_vec(),
    )]);
    let reader = OpenDHistoricalKlineReader::new(coordinator(address));
    let error = reader
        .query(&HistoricalKlineQuery {
            market: 1,
            symbol: "00700".to_owned(),
            period: "1d".to_owned(),
            adjustment: 0,
            begin_time: String::new(),
            end_time: String::new(),
            max_ack_kl_num: None,
            next_req_key: Vec::new(),
            extended_time: None,
            session: None,
        })
        .expect_err("rate limit must surface");
    assert!(matches!(
        error,
        HistoricalKlineError::Rejected {
            ret_type: -2,
            err_code: 429,
            ref message,
        } if message == "history rate limited"
    ));
    server.join().expect("server");

    // Snapshot entitlement (Qot_GetSecuritySnapshot 3203).
    #[derive(Clone, PartialEq, Message)]
    struct SnapshotResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
    }
    let (address, _, server) = scripted_server(vec![(
        jftrade_integration_futu::PROTO_GET_SECURITY_SNAPSHOT,
        SnapshotResponse {
            ret_type: Some(-1),
            ret_msg: Some("snapshot entitlement denied".to_owned()),
            err_code: Some(403),
        }
        .encode_to_vec(),
    )]);
    let error = jftrade_integration_futu::OpenDSecuritySnapshotReader::new(coordinator(address))
        .query(&["HK.00700".to_owned()])
        .expect_err("snapshot rejection must surface");
    match error {
        SecuritySnapshotQueryError::Rejected {
            ret_type,
            err_code,
            message,
        } => {
            assert_eq!(ret_type, -1);
            assert_eq!(err_code, 403);
            assert!(message.contains("snapshot entitlement denied"), "{message}");
        }
        other => panic!("unexpected snapshot error: {other:?}"),
    }
    server.join().expect("server");

    // Static-info invalid security (Qot_GetStaticInfo 3202).
    #[derive(Clone, PartialEq, Message)]
    struct StaticInfoResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(string, optional, tag = "2")]
        ret_msg: Option<String>,
        #[prost(int32, optional, tag = "3")]
        err_code: Option<i32>,
    }
    let (address, _, server) = scripted_server(vec![(
        PROTO_GET_STATIC_INFO,
        StaticInfoResponse {
            ret_type: Some(-1),
            ret_msg: Some("unknown security".to_owned()),
            err_code: Some(400),
        }
        .encode_to_vec(),
    )]);
    let error = OpenDInstrumentSearchReader::new(coordinator(address))
        .lookup("HK", "bad")
        .expect_err("invalid security must surface");
    match error {
        jftrade_integration_futu::InstrumentSearchError::Rejected {
            ret_type,
            err_code,
            message,
        } => {
            assert_eq!(ret_type, -1);
            assert_eq!(err_code, 400);
            assert_eq!(message, "unknown security");
        }
        other => panic!("unexpected static-info error: {other:?}"),
    }
    server.join().expect("server");
}

#[test]
fn market_read_methods_reject_a_disconnected_session() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:232
    // TestMarketReadMethodsRejectDisconnectedSession. An unconnected client is
    // `Closed`; the typed readers must fail closed instead of fabricating an
    // empty page. This is asserted on a coordinator that was never connected to
    // a live OpenD port.
    use jftrade_integration_futu::{
        CurrentKlineError, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort,
    };

    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("address");
    drop(listener);
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let error = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_millis(150)),
        recorder,
        Vec::new(),
        0,
    )
    .expect_err("a closed port must not yield a connected coordinator");
    let rendered = error.to_string();
    assert!(
        rendered.to_lowercase().contains("failed")
            || rendered.to_lowercase().contains("connection"),
        "unexpected connect error: {rendered}"
    );

    // The same fail-closed contract for readers built over a closed
    // coordinator: querying must return an error, never an empty result.
    let (address, _, server) = scripted_server(Vec::new());
    let handle = coordinator(address);
    {
        let mut coordinator = handle.lock().expect("coordinator");
        coordinator.close().expect("close");
    }
    let reader = OpenDHistoricalKlineReader::new(Arc::clone(&handle));
    let error = reader
        .query(&HistoricalKlineQuery {
            market: 1,
            symbol: "00700".to_owned(),
            period: "1d".to_owned(),
            adjustment: 0,
            begin_time: String::new(),
            end_time: String::new(),
            max_ack_kl_num: None,
            next_req_key: Vec::new(),
            extended_time: None,
            session: None,
        })
        .expect_err("closed coordinator must reject history reads");
    assert!(matches!(error, HistoricalKlineError::Session(_)));

    let current = jftrade_integration_futu::OpenDHistoricalKlineReader::new(Arc::clone(&handle))
        .query_current(&jftrade_integration_futu::CurrentKlineQuery::new(
            1, "00700", "1d",
        ))
        .expect_err("closed coordinator must reject current kline reads");
    assert!(matches!(current, CurrentKlineError::Session(_)));
    server.join().expect("server");
}

#[test]
fn quote_subscribe_encodes_advanced_market_data_options_on_the_wire() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:21
    // TestSubscribeQuotesEncodesAdvancedMarketDataOptions. Go's low-level
    // QuoteSubRequest exposes every Qot_Sub flag; Rust builds one request per
    // demand-owned subscription kind, so this asserts the flags that each kind
    // actually emits, plus the US intraday session routing, over a real framed
    // socket.
    use jftrade_integration_futu::{OpenDSubscriptionExecutor, PROTO_QOT_SUB, SubscriptionKind};
    use jftrade_marketdata::InstrumentRef;

    #[derive(Clone, PartialEq, Message)]
    struct SubRequest {
        #[prost(message, optional, tag = "1")]
        c2s: Option<SubC2s>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct SubC2s {
        #[prost(message, repeated, tag = "1")]
        security_list: Vec<Security>,
        #[prost(int32, repeated, tag = "2")]
        sub_type_list: Vec<i32>,
        #[prost(bool, optional, tag = "3")]
        is_sub_or_un_sub: Option<bool>,
        #[prost(bool, optional, tag = "4")]
        is_reg_or_un_reg_push: Option<bool>,
        #[prost(int32, repeated, tag = "5")]
        reg_push_rehab_type_list: Vec<i32>,
        #[prost(bool, optional, tag = "6")]
        is_first_push: Option<bool>,
        #[prost(bool, optional, tag = "7")]
        is_unsub_all: Option<bool>,
        #[prost(bool, optional, tag = "8")]
        is_sub_order_book_detail: Option<bool>,
        #[prost(bool, optional, tag = "9")]
        extended_time: Option<bool>,
        #[prost(int32, optional, tag = "10")]
        session: Option<i32>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct Security {
        #[prost(int32, optional, tag = "1")]
        market: Option<i32>,
        #[prost(string, optional, tag = "2")]
        code: Option<String>,
    }
    #[derive(Clone, PartialEq, Message)]
    struct SubResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }

    let _ = SubResponse { ret_type: Some(0) };
    let (address, requests, server) = scripted_server_until_close();
    let desired = vec![
        InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: None,
        },
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: Some("1m".to_owned()),
        },
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: None,
        },
    ];
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        Vec::new(),
        0,
    )
    .expect("connect");
    coordinator
        .reconcile_topology(&desired, 1)
        .expect("reconcile demand");
    let _ = OpenDSubscriptionExecutor::from_session(coordinator.session_clone().expect("session"));
    let _ = coordinator.close();
    server.join().expect("server");

    let records = requests.lock().expect("requests");
    let decoded: Vec<SubC2s> = records
        .iter()
        .filter(|frame| frame.header.proto_id == PROTO_QOT_SUB)
        .map(|frame| {
            SubRequest::decode(frame.body.as_slice())
                .expect("sub request")
                .c2s
                .expect("c2s")
        })
        .collect();
    // Go's adapter subscribes Basic quotes for the snapshot channel, and the
    // K-line channel adds its own Basic + KLINE pair (pkg/futu/exchange_kline.go),
    // so the demand set below emits four Qot_Sub requests.
    assert_eq!(decoded.len(), 4, "one subscribe per physical stream pair");

    let basic = decoded
        .iter()
        .find(|c2s| c2s.sub_type_list == [1] && c2s.security_list[0].market == Some(1))
        .expect("HK basic quote subscribe");
    assert_eq!(basic.security_list[0].code.as_deref(), Some("00700"));
    assert_eq!(basic.is_sub_or_un_sub, Some(true));
    assert_eq!(basic.is_reg_or_un_reg_push, Some(true));
    assert_eq!(basic.is_first_push, Some(true));
    assert_eq!(basic.is_unsub_all, Some(false));
    let kline_basic = decoded
        .iter()
        .find(|c2s| c2s.sub_type_list == [1] && c2s.security_list[0].market == Some(11))
        .expect("US kline companion basic subscribe");
    assert_eq!(kline_basic.is_reg_or_un_reg_push, Some(true));

    let kline = decoded
        .iter()
        .find(|c2s| c2s.sub_type_list == [11])
        .expect("US intraday kline subscribe");
    assert_eq!(kline.security_list[0].market, Some(11));
    assert_eq!(
        kline.extended_time,
        Some(true),
        "US intraday sessions need extended routing"
    );
    assert_eq!(
        kline.session,
        Some(jftrade_integration_futu::SESSION_ALL),
        "US intraday K-line requests Session_ALL"
    );
    assert_eq!(kline.is_reg_or_un_reg_push, Some(false));

    let order_book = decoded
        .iter()
        .find(|c2s| c2s.sub_type_list == [2])
        .expect("HK order-book subscribe");
    assert_eq!(order_book.is_reg_or_un_reg_push, Some(true));
    assert_eq!(order_book.is_sub_order_book_detail, Some(true));
    let _ = SubscriptionKind::OrderBook;
}

#[test]
fn get_kl_returns_empty_result_when_opend_omits_s2c() {
    // Parity: go:pkg/futu/opend/market_read_boundaries_test.go:212
    // TestGetKLReturnsEmptyResultWhenOpenDOmitsS2C. `new_methods_test.go:768`
    // covers the same policy through the pure decoder; this asserts the real
    // reader path over a framed socket so the coordinator/session plumbing is
    // included.
    #[derive(Clone, PartialEq, Message)]
    struct GetKlResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
    }
    let (address, requests, server) = scripted_server(vec![(
        PROTO_GET_KL,
        GetKlResponse { ret_type: Some(0) }.encode_to_vec(),
    )]);
    let reader = OpenDHistoricalKlineReader::new(coordinator(address));
    let result = reader
        .query_current(&jftrade_integration_futu::CurrentKlineQuery::new(
            1, "00700", "1d",
        ))
        .expect("payload-less ack is an empty current-kline result");
    assert!(result.klines.is_empty(), "no candles must be fabricated");
    assert!(result.name.is_none(), "no name must be fabricated");

    let recorded = requests.lock().expect("requests");
    assert_eq!(recorded[0].header.proto_id, PROTO_GET_KL);
    drop(recorded);
    server.join().expect("server");
}
