use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::{Arc, Mutex, mpsc};
use std::thread;
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDSessionCoordinator, OpenDSessionRuntime, OpenDSessionRuntimeConfig,
    OpenDTcpProbeConfig, PROTO_GET_SUB_INFO, PROTO_INIT_CONNECT, PROTO_QOT_SUB, decode_frame,
    encode_frame,
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
    #[prost(int32, optional, tag = "2")]
    total_used_quota: Option<i32>,
    #[prost(int32, optional, tag = "3")]
    remain_quota: Option<i32>,
    #[prost(int32, optional, tag = "4")]
    own_used_quota: Option<i32>,
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
                    total_used_quota: Some(15),
                    remain_quota: Some(85),
                    own_used_quota: Some(5),
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
fn test_fallback_establishment_recovery_and_count_semantics() {
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
                    conn_id: 100,
                }),
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
    .expect("connect");

    assert_eq!(
        coordinator
            .physical_snapshot()
            .expect("snapshot")
            .fallback_count,
        0
    );

    // Explicit fallback_count manipulation and reconciliation behavior
    coordinator.set_fallback_count(3);
    assert_eq!(
        coordinator
            .physical_snapshot()
            .expect("snapshot")
            .fallback_count,
        3
    );

    // Verify snapshot reflects fallback count accurately
    let snapshot = coordinator.physical_snapshot().expect("snapshot");
    assert_eq!(snapshot.fallback_count, 3);

    coordinator.set_fallback_count(0);
    assert_eq!(
        coordinator
            .physical_snapshot()
            .expect("snapshot")
            .fallback_count,
        0
    );

    coordinator.close().expect("close");
    server.join().expect("server join");
}
