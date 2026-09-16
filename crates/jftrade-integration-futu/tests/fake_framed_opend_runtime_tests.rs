use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::{Arc, Mutex, mpsc};
use std::thread;
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDSessionCloseReason, OpenDSessionCoordinator, OpenDSessionEvent,
    OpenDSessionRuntime, OpenDSessionRuntimeConfig, OpenDTcpProbeConfig, PROTO_GET_SUB_INFO,
    PROTO_INIT_CONNECT, PROTO_QOT_SUB, decode_frame, encode_frame,
};
use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};
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

#[derive(Clone, PartialEq, Message)]
struct GetSubInfoResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(message, optional, tag = "4")]
    s2c: Option<GetSubInfoS2c>,
}

#[derive(Clone, PartialEq, Message)]
struct GetSubInfoS2c {
    #[prost(message, repeated, tag = "1")]
    conn_sub_info_list: Vec<ConnSubInfo>,
    #[prost(int32, optional, tag = "2")]
    total_used_quota: Option<i32>,
    #[prost(int32, optional, tag = "3")]
    remain_quota: Option<i32>,
    #[prost(int32, optional, tag = "4")]
    option_used_quota: Option<i32>,
}

#[derive(Clone, PartialEq, Message)]
struct ConnSubInfo {
    #[prost(int32, optional, tag = "2")]
    used_quota: Option<i32>,
    #[prost(bool, optional, tag = "3")]
    is_own_conn_data: Option<bool>,
}

#[derive(Clone, PartialEq, Message)]
struct SubRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<SubC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct SubC2s {
    #[prost(message, repeated, tag = "1")]
    securities: Vec<Security>,
    #[prost(int32, repeated, tag = "2")]
    sub_type_list: Vec<i32>,
    #[prost(bool, optional, tag = "3")]
    is_sub_or_unsub: Option<bool>,
    #[prost(bool, optional, tag = "4")]
    is_reg_or_un_reg_push: Option<bool>,
    /// Go `QuoteSubRequest.IsSubOrderBookDetail` (tag 8 on the wire).
    #[prost(bool, optional, tag = "8")]
    is_sub_order_book_detail: Option<bool>,
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
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
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

#[test]
fn test_quota_success_field_missing_protocol_error_and_preservation() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");

    let server = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept");
        // 1. Initial connect handshake
        let init_frame = read_framed_frame(&mut stream).expect("init frame");
        assert_eq!(init_frame.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream,
            PROTO_INIT_CONNECT,
            init_frame.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 42,
                }),
            }
            .encode_to_vec(),
        );

        // 2. First quota request: Success with full fields
        let quota1 = read_framed_frame(&mut stream).expect("quota frame 1");
        assert_eq!(quota1.header.proto_id, PROTO_GET_SUB_INFO);
        write_framed_response(
            &mut stream,
            PROTO_GET_SUB_INFO,
            quota1.header.serial_no,
            &GetSubInfoResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(GetSubInfoS2c {
                    conn_sub_info_list: vec![
                        ConnSubInfo {
                            used_quota: Some(3),
                            is_own_conn_data: Some(true),
                        },
                        ConnSubInfo {
                            used_quota: Some(2),
                            is_own_conn_data: Some(true),
                        },
                        ConnSubInfo {
                            used_quota: Some(7),
                            is_own_conn_data: Some(false),
                        },
                    ],
                    total_used_quota: Some(15),
                    remain_quota: Some(85),
                    option_used_quota: Some(1),
                }),
            }
            .encode_to_vec(),
        );

        // 3. Second quota request: ret_type = 0 but s2c is None (missing field)
        let quota2 = read_framed_frame(&mut stream).expect("quota frame 2");
        assert_eq!(quota2.header.proto_id, PROTO_GET_SUB_INFO);
        write_framed_response(
            &mut stream,
            PROTO_GET_SUB_INFO,
            quota2.header.serial_no,
            &GetSubInfoResponse {
                ret_type: Some(0),
                ret_msg: Some("missing s2c body".to_owned()),
                s2c: None,
            }
            .encode_to_vec(),
        );

        // 4. Third quota request: ret_type = -1 (error)
        let quota3 = read_framed_frame(&mut stream).expect("quota frame 3");
        assert_eq!(quota3.header.proto_id, PROTO_GET_SUB_INFO);
        write_framed_response(
            &mut stream,
            PROTO_GET_SUB_INFO,
            quota3.header.serial_no,
            &GetSubInfoResponse {
                ret_type: Some(-1),
                ret_msg: Some("Futu OpenD rate limit exceeded".to_owned()),
                s2c: None,
            }
            .encode_to_vec(),
        );
    });

    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        Vec::new(),
        1_700_000_000_000,
    )
    .expect("coordinator connect");

    // 1. Initial success
    coordinator
        .refresh_quota(1_700_000_000_100)
        .expect("refresh quota 1");
    let snap1 = coordinator.physical_snapshot().expect("snapshot 1");
    assert_eq!(snap1.total_used_quota, Some(15));
    assert_eq!(snap1.remain_quota, Some(85));
    assert_eq!(snap1.own_used_quota, Some(5));
    assert_eq!(snap1.last_error, None);

    // 2. Missing s2c: Preserves last success, records error in last_error
    coordinator
        .refresh_quota(1_700_000_000_200)
        .expect("refresh quota 2");
    let snap2 = coordinator.physical_snapshot().expect("snapshot 2");
    assert_eq!(snap2.total_used_quota, Some(15));
    assert_eq!(snap2.remain_quota, Some(85));
    assert_eq!(snap2.own_used_quota, Some(5));
    assert!(
        snap2
            .last_error
            .as_deref()
            .unwrap_or_default()
            .contains("missing s2c")
    );

    // 3. OpenD error ret_type != 0: Preserves last success, updates last_error
    coordinator
        .refresh_quota(1_700_000_000_300)
        .expect("refresh quota 3");
    let snap3 = coordinator.physical_snapshot().expect("snapshot 3");
    assert_eq!(snap3.total_used_quota, Some(15));
    assert_eq!(snap3.remain_quota, Some(85));
    assert_eq!(snap3.own_used_quota, Some(5));
    assert!(
        snap3
            .last_error
            .as_deref()
            .unwrap_or_default()
            .contains("rate limit")
    );

    coordinator.close().expect("close");
    server.join().expect("server join");
}

#[test]
fn test_reconnect_and_demand_replay_with_framed_opend() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let (first_sub_tx, first_sub_rx) = mpsc::channel();
    let (replayed_sub_tx, replayed_sub_rx) = mpsc::channel();

    let server = thread::spawn(move || {
        // First connection
        let (mut stream1, _) = listener.accept().expect("accept 1");
        let init1 = read_framed_frame(&mut stream1).expect("init 1");
        assert_eq!(init1.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream1,
            PROTO_INIT_CONNECT,
            init1.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 1,
                }),
            }
            .encode_to_vec(),
        );

        let sub1 = read_framed_frame(&mut stream1).expect("sub 1");
        assert_eq!(sub1.header.proto_id, PROTO_QOT_SUB);
        write_framed_response(
            &mut stream1,
            PROTO_QOT_SUB,
            sub1.header.serial_no,
            &SubResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
            }
            .encode_to_vec(),
        );
        first_sub_tx.send(()).expect("send first sub ack");

        // Sever connection
        drop(stream1);

        // Reconnect connection
        let (mut stream2, _) = listener.accept().expect("accept 2");
        let init2 = read_framed_frame(&mut stream2).expect("init 2");
        assert_eq!(init2.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream2,
            PROTO_INIT_CONNECT,
            init2.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 2,
                }),
            }
            .encode_to_vec(),
        );

        let sub2 = read_framed_frame(&mut stream2).expect("replayed sub");
        assert_eq!(sub2.header.proto_id, PROTO_QOT_SUB);
        let req = SubRequest::decode(sub2.body.as_slice()).expect("decode replayed sub");
        let code = req
            .c2s
            .and_then(|c| c.securities.into_iter().next())
            .and_then(|s| s.code)
            .expect("code");
        replayed_sub_tx.send(code).expect("send replayed code");
        write_framed_response(
            &mut stream2,
            PROTO_QOT_SUB,
            sub2.header.serial_no,
            &SubResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
            }
            .encode_to_vec(),
        );
    });

    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
            Arc::clone(&recorder),
            vec![InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "NVDA".to_owned(),
                interval: None,
            }],
            1_700_000_000_000,
        )
        .expect("coordinator connect"),
    ));

    let mut runtime = OpenDSessionRuntime::start(
        Arc::clone(&coordinator),
        OpenDSessionRuntimeConfig {
            poll_interval: Duration::from_millis(5),
            event_timeout: Duration::from_millis(1),
            reconnect_initial_delay: Duration::from_millis(50),
            reconnect_max_delay: Duration::from_millis(50),
            ..OpenDSessionRuntimeConfig::default()
        },
    )
    .expect("start session runtime");

    first_sub_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("wait for first sub");

    let replayed = replayed_sub_rx
        .recv_timeout(Duration::from_secs(3))
        .expect("replayed sub");
    assert_eq!(replayed, "NVDA");

    runtime.shutdown().expect("shutdown");
    server.join().expect("server join");
}

#[test]
fn test_order_book_reconcile_splits_hk_detail_and_deduplicates_replay() {
    // Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:237
    // TestEnsureOrderBookPushSubscriptionsSplitsDetailsAndDeduplicates and
    // :178 TestGroupOrderBookRequestsForPushSplitsHKAndNonHK. Go splits HK and
    // non-HK requests into separate Qot_Sub calls so only the HK batch carries
    // IsSubOrderBookDetail; Rust issues one physical subscribe per instrument,
    // so the flag must ride exactly on the HK request.
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");

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
                    conn_id: 21,
                }),
            }
            .encode_to_vec(),
        );

        let mut observed = Vec::new();
        for _ in 0..2 {
            let sub = read_framed_frame(&mut stream).expect("order-book sub frame");
            assert_eq!(sub.header.proto_id, PROTO_QOT_SUB);
            let request = SubRequest::decode(sub.body.as_slice()).expect("sub request");
            let c2s = request.c2s.expect("sub c2s");
            assert_eq!(c2s.is_sub_or_unsub, Some(true));
            assert_eq!(c2s.is_reg_or_un_reg_push, Some(true));
            assert_eq!(c2s.sub_type_list, vec![2]);
            assert_eq!(c2s.securities.len(), 1);
            let security = &c2s.securities[0];
            observed.push((
                security.market.expect("market"),
                security.code.clone().expect("code"),
                c2s.is_sub_order_book_detail,
            ));
            write_framed_response(
                &mut stream,
                PROTO_QOT_SUB,
                sub.header.serial_no,
                &SubResponse {
                    ret_type: Some(0),
                    ret_msg: Some("ok".to_owned()),
                }
                .encode_to_vec(),
            );
        }
        // Report whether a third Qot_Sub arrives after the repeated reconcile
        // pass so the assertion can distinguish "idle" from "resent".
        stream
            .set_read_timeout(Some(Duration::from_millis(400)))
            .expect("read timeout");
        let mut header = [0_u8; 44];
        let resent = match stream.read(&mut header) {
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
            Err(error) => panic!("unexpected read error: {error}"),
        };
        assert!(
            !resent,
            "a repeated reconcile pass must not resend order-book subscriptions"
        );
        assert_eq!(
            observed,
            vec![
                (1, "00700".to_owned(), Some(true)),
                (11, "AAPL".to_owned(), None),
            ]
        );
    });

    let demand = vec![
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: None,
        },
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        },
    ];
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        demand.clone(),
        1_700_000_000_000,
    )
    .expect("order-book reconcile");
    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.own_active_count, 2);
    assert_eq!(snapshot.entries.len(), 2);

    // The repeated pass is a no-op; the server asserts nothing is resent.
    coordinator
        .reconcile(&demand, 1_700_000_000_000 + 1_000)
        .expect("replayed reconcile");
    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.own_active_count, 2);

    coordinator.close().expect("close");
    server.join().expect("server join");
}

#[test]
fn test_basic_quote_availability_rejection_enters_delayed_fallback_and_reconcile_succeeds() {
    // Parity: internal/integration/futu/subscription_reconciler_test.go:346 TestSubscriptionReconcilerUsesDelayedFallbackForBasicQuoteAvailabilityFailures
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");

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
                    conn_id: 77,
                }),
            }
            .encode_to_vec(),
        );

        // First Qot_Sub: quota pressure -> fallback eligible.
        let sub = read_framed_frame(&mut stream).expect("sub frame");
        assert_eq!(sub.header.proto_id, PROTO_QOT_SUB);
        write_framed_response(
            &mut stream,
            PROTO_QOT_SUB,
            sub.header.serial_no,
            &SubResponse {
                ret_type: Some(-1),
                ret_msg: Some("OpenD subscription is full".to_owned()),
            }
            .encode_to_vec(),
        );

        // Retry after the 15s fallback window is acknowledged successfully.
        let retry = read_framed_frame(&mut stream).expect("retry frame");
        assert_eq!(retry.header.proto_id, PROTO_QOT_SUB);
        write_framed_response(
            &mut stream,
            PROTO_QOT_SUB,
            retry.header.serial_no,
            &SubResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
            }
            .encode_to_vec(),
        );
    });

    let demand = vec![InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "SH".to_owned(),
        symbol: "600519".to_owned(),
        interval: None,
    }];
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut coordinator = OpenDSessionCoordinator::connect(
        OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
        recorder,
        demand.clone(),
        1_700_000_000_000,
    )
    .expect("quota rejection must not fail the reconcile pass");

    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.fallback_count, 1);
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.desired_count, 1);
    assert_eq!(snapshot.entries.len(), 1);
    assert_eq!(snapshot.entries[0].broker_state, "fallback");
    assert_eq!(
        snapshot.entries[0].last_error.as_deref(),
        Some("OpenD Qot_Sub returned retType=-1 errCode=0: OpenD subscription is full")
    );

    // Retry before the fallback window stays deferred.
    coordinator
        .reconcile(&demand, 1_700_000_000_000 + 14_999)
        .expect("deferred reconcile");
    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.fallback_count, 1);

    // At the fallback deadline the retry succeeds and clears fallback state.
    coordinator
        .reconcile(&demand, 1_700_000_000_000 + 15_000)
        .expect("fallback recovery reconcile");
    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.fallback_count, 0);
    assert_eq!(snapshot.own_active_count, 1);
    assert_eq!(snapshot.entries[0].broker_state, "active");

    coordinator.close().expect("close");
    server.join().expect("server");
}

#[test]
fn test_fallback_establishment_recovery_and_count_semantics() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind");
    let address = listener.local_addr().expect("local_addr");
    let (first_connected_tx, first_connected_rx) = mpsc::channel();
    let (second_connected_tx, second_connected_rx) = mpsc::channel();
    let (replayed_sub_tx, replayed_sub_rx) = mpsc::channel();

    let server = thread::spawn(move || {
        // Gen 1: First connection - server accepts, handshakes, answers subscription, then drops stream (real EOF)
        let (mut stream1, _) = listener.accept().expect("accept 1");
        let init1 = read_framed_frame(&mut stream1).expect("init frame 1");
        assert_eq!(init1.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream1,
            PROTO_INIT_CONNECT,
            init1.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 101,
                }),
            }
            .encode_to_vec(),
        );

        let sub1 = read_framed_frame(&mut stream1).expect("sub1 frame");
        assert_eq!(sub1.header.proto_id, PROTO_QOT_SUB);
        write_framed_response(
            &mut stream1,
            PROTO_QOT_SUB,
            sub1.header.serial_no,
            &SubResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
            }
            .encode_to_vec(),
        );

        first_connected_tx.send(()).expect("send gen 1 ack");
        // Drop stream1 to simulate real EOF / disconnect
        drop(stream1);

        // Gen 2: Second connection - server accepts, answers replayed subscription, then drops stream (second reconnect)
        let (mut stream2, _) = listener.accept().expect("accept 2");
        let init2 = read_framed_frame(&mut stream2).expect("init frame 2");
        assert_eq!(init2.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream2,
            PROTO_INIT_CONNECT,
            init2.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 102,
                }),
            }
            .encode_to_vec(),
        );

        let sub2 = read_framed_frame(&mut stream2).expect("sub2 frame");
        assert_eq!(sub2.header.proto_id, PROTO_QOT_SUB);
        write_framed_response(
            &mut stream2,
            PROTO_QOT_SUB,
            sub2.header.serial_no,
            &SubResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
            }
            .encode_to_vec(),
        );

        second_connected_tx.send(()).expect("send gen 2 ack");
        drop(stream2);

        // Gen 3: Third connection - server handshakes, accepts replayed subscription and answers basic quote queries
        let (mut stream3, _) = listener.accept().expect("accept 3");
        let init3 = read_framed_frame(&mut stream3).expect("init frame 3");
        assert_eq!(init3.header.proto_id, PROTO_INIT_CONNECT);
        write_framed_response(
            &mut stream3,
            PROTO_INIT_CONNECT,
            init3.header.serial_no,
            &InitResponse {
                ret_type: Some(0),
                ret_msg: Some("ok".to_owned()),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 103,
                }),
            }
            .encode_to_vec(),
        );

        while let Some(frame) = read_framed_frame(&mut stream3) {
            match frame.header.proto_id {
                PROTO_QOT_SUB => {
                    let req =
                        SubRequest::decode(frame.body.as_slice()).expect("decode replayed sub");
                    let code = req
                        .c2s
                        .and_then(|c| c.securities.into_iter().next())
                        .and_then(|s| s.code)
                        .expect("code");
                    let _ = replayed_sub_tx.send(code);
                    write_framed_response(
                        &mut stream3,
                        PROTO_QOT_SUB,
                        frame.header.serial_no,
                        &SubResponse {
                            ret_type: Some(0),
                            ret_msg: Some("ok".to_owned()),
                        }
                        .encode_to_vec(),
                    );
                }
                jftrade_integration_futu::PROTO_GET_BASIC_QOT => {
                    write_framed_response(
                        &mut stream3,
                        jftrade_integration_futu::PROTO_GET_BASIC_QOT,
                        frame.header.serial_no,
                        &[0x08, 0x00, 0x22, 0x00],
                    );
                }
                _ => {}
            }
        }
    });

    let demand = vec![InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: None,
    }];

    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(2)),
            Arc::clone(&recorder),
            demand.clone(),
            1_700_000_000_000,
        )
        .expect("connect"),
    ));

    // Initial state after Gen 1 connection
    {
        let snap1 = coordinator
            .lock()
            .unwrap()
            .physical_snapshot()
            .expect("snapshot gen 1");
        assert_eq!(snap1.connection_generation, Some(1));
        assert_eq!(snap1.fallback_count, 0);
    }
    first_connected_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("gen 1 connected");

    let mut runtime = OpenDSessionRuntime::start(
        Arc::clone(&coordinator),
        OpenDSessionRuntimeConfig {
            poll_interval: Duration::from_millis(5),
            event_timeout: Duration::from_millis(1),
            reconnect_initial_delay: Duration::from_millis(20),
            reconnect_max_delay: Duration::from_millis(50),
            ..OpenDSessionRuntimeConfig::default()
        },
    )
    .expect("start session runtime");

    second_connected_rx
        .recv_timeout(Duration::from_secs(2))
        .expect("gen 2 connected and disconnected");

    let replayed = replayed_sub_rx
        .recv_timeout(Duration::from_secs(3))
        .expect("replayed sub gen 3");
    assert_eq!(replayed, "AAPL");

    // Wait for gen 3 to settle
    for _ in 0..100 {
        let snap = coordinator
            .lock()
            .unwrap()
            .physical_snapshot()
            .expect("snapshot");
        if snap.connection_generation == Some(3)
            && snap.fallback_count == 0
            && snap.own_active_count == 1
        {
            break;
        }
        thread::sleep(Duration::from_millis(10));
    }

    let snap_recovered = coordinator
        .lock()
        .unwrap()
        .physical_snapshot()
        .expect("snapshot recovered");
    assert_eq!(snap_recovered.connection_generation, Some(3));
    assert_eq!(snap_recovered.observed_connection_generation, Some(3));
    assert_eq!(snap_recovered.fallback_count, 0);
    assert_eq!(snap_recovered.last_error, None);
    assert_eq!(snap_recovered.own_active_count, 1);
    assert_eq!(snap_recovered.desired_count, 1);

    // Stale callback generation fencing assertion:
    // Ingesting a closed or unsolicited frame from Gen 1 or Gen 2 MUST be ignored and not mutate Gen 3 state.
    {
        let guard = coordinator.lock().unwrap();
        let stale_event = crate::OpenDSessionEvent::Closed {
            generation: 1,
            reason: crate::OpenDSessionCloseReason::PeerClosed,
        };
        let ingest_result = guard.lifecycle().ingest_session_event(
            &stale_event,
            "2026-08-28T00:00:00Z".parse().expect("timestamp"),
        );
        assert!(ingest_result.is_ok());
        let rec = guard.recorder().snapshot();
        assert_eq!(rec.generation, 3);
        assert_eq!(rec.stream_last_error, None);
    }

    runtime.shutdown().expect("shutdown");
    server.join().expect("server join");
}
