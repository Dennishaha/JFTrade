use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::{Arc, Mutex, mpsc};
use std::time::Duration;

use jftrade_integration_futu::{
    Frame, OpenDSessionCoordinator, OpenDSessionRuntime, OpenDSessionRuntimeConfig,
    OpenDTcpProbeConfig, PROTO_INIT_CONNECT, PROTO_QOT_SUB, decode_frame, encode_frame,
};
use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};
use prost::Message;

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

#[derive(Clone, PartialEq, Message)]
struct SubRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<SubState>,
}

#[derive(Clone, PartialEq, Message)]
struct SubState {
    #[prost(message, repeated, tag = "1")]
    securities: Vec<Security>,
}

#[derive(Clone, PartialEq, Message)]
struct Security {
    #[prost(string, optional, tag = "2")]
    code: Option<String>,
}

#[test]
fn latest_demand_replaces_stale_replay_while_reconnect_is_pending() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
    let address = listener.local_addr().expect("address");
    let (release_initial_tx, release_initial_rx) = mpsc::channel();
    let (failed_attempt_tx, failed_attempt_rx) = mpsc::channel();
    let (replayed_code_tx, replayed_code_rx) = mpsc::channel();
    let server = std::thread::spawn(move || {
        let (mut initial, _) = listener.accept().expect("initial accept");
        respond_to_init(&mut initial, 1);
        respond_to_subscription(&mut initial, "initial subscription");
        release_initial_rx.recv().expect("release initial");
        drop(initial);

        let (mut failed, _) = listener.accept().expect("failed reconnect accept");
        let init = read_frame(&mut failed, "failed init");
        assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
        failed_attempt_tx.send(()).expect("failed attempt observed");
        drop(failed);

        let (mut recovered, _) = listener.accept().expect("recovery accept");
        respond_to_init(&mut recovered, 2);
        recovered
            .set_read_timeout(Some(Duration::from_secs(2)))
            .expect("recovery timeout");
        let subscription = read_frame(&mut recovered, "replayed subscription");
        assert_eq!(subscription.header.proto_id, PROTO_QOT_SUB);
        let request =
            SubRequest::decode(subscription.body.as_slice()).expect("decode replayed subscription");
        let code = request
            .c2s
            .and_then(|state| state.securities.into_iter().next())
            .and_then(|security| security.code)
            .expect("replayed security code");
        replayed_code_tx.send(code).expect("replayed code");
        write_subscription_response(&mut recovered, &subscription);
        let mut byte = [0_u8; 1];
        let _ = recovered.read(&mut byte);
    });

    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let coordinator = Arc::new(Mutex::new(
        OpenDSessionCoordinator::connect(
            OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
            Arc::clone(&recorder),
            vec![snapshot_demand("AAPL")],
            0,
        )
        .expect("coordinator"),
    ));
    let mut runtime = OpenDSessionRuntime::start(
        Arc::clone(&coordinator),
        OpenDSessionRuntimeConfig {
            poll_interval: Duration::from_millis(5),
            event_timeout: Duration::from_millis(1),
            reconnect_initial_delay: Duration::from_millis(100),
            reconnect_max_delay: Duration::from_millis(100),
            ..OpenDSessionRuntimeConfig::default()
        },
    )
    .expect("runtime task");
    release_initial_tx.send(()).expect("release initial");
    failed_attempt_rx
        .recv_timeout(Duration::from_secs(1))
        .expect("failed reconnect attempt");
    runtime.set_demand(vec![snapshot_demand("MSFT")]);

    assert_eq!(
        replayed_code_rx
            .recv_timeout(Duration::from_secs(2))
            .expect("latest demand replay"),
        "MSFT"
    );
    wait_for_reconnect(&runtime);
    assert_eq!(runtime.demand(), vec![snapshot_demand("MSFT")]);
    runtime.shutdown().expect("shutdown");
    server.join().expect("server");
}

fn respond_to_init(stream: &mut TcpStream, connection_id: u64) {
    let init = read_frame(stream, "init");
    assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
    let body = InitResponse {
        ret_type: Some(0),
        s2c: Some(InitState {
            server_ver: 1009,
            conn_id: connection_id,
        }),
    }
    .encode_to_vec();
    stream
        .write_all(
            &encode_frame(PROTO_INIT_CONNECT, init.header.serial_no, &body).expect("init response"),
        )
        .expect("write init response");
}

fn respond_to_subscription(stream: &mut TcpStream, context: &str) {
    let request = read_frame(stream, context);
    assert_eq!(request.header.proto_id, PROTO_QOT_SUB);
    write_subscription_response(stream, &request);
}

fn write_subscription_response(stream: &mut TcpStream, request: &Frame) {
    stream
        .write_all(
            &encode_frame(PROTO_QOT_SUB, request.header.serial_no, &[8, 0])
                .expect("subscription response"),
        )
        .expect("write subscription response");
}

fn read_frame(stream: &mut TcpStream, context: &str) -> Frame {
    let mut header = [0_u8; 44];
    stream
        .read_exact(&mut header)
        .unwrap_or_else(|error| panic!("read {context} header: {error}"));
    let body_len = u32::from_le_bytes(header[12..16].try_into().expect("body length")) as usize;
    let mut body = vec![0_u8; body_len];
    stream
        .read_exact(&mut body)
        .unwrap_or_else(|error| panic!("read {context} body: {error}"));
    decode_frame(&[header.as_slice(), body.as_slice()].concat())
        .unwrap_or_else(|error| panic!("decode {context}: {error}"))
}

fn snapshot_demand(symbol: &str) -> InstrumentRef {
    InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: symbol.to_owned(),
        interval: None,
    }
}

fn wait_for_reconnect(runtime: &OpenDSessionRuntime) {
    for _ in 0..100 {
        if runtime.status().reconnects > 0 {
            return;
        }
        std::thread::sleep(Duration::from_millis(5));
    }
    panic!("runtime did not report a successful reconnect");
}
