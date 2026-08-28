use std::io::{Read, Write};
use std::net::{Ipv4Addr, SocketAddr, TcpListener, TcpStream};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use jftrade_api::AccessPolicy;
use jftrade_integration_futu::{
    OpenDProviderRuntimeConfig, OpenDTcpProbeConfig, PROTO_GET_GLOBAL_STATE, PROTO_INIT_CONNECT,
    PROTO_QOT_SUB, decode_frame, encode_frame, provider_descriptor,
};
use jftrade_marketdata::{InstrumentRef, ProviderReadiness, ProviderRouter};
use tempfile::tempdir;

use super::*;

#[tokio::test]
async fn product_runtime_without_optional_workers_starts_and_stops_cleanly() {
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::new(
        SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
        directory.path().join("settings.json"),
        AccessPolicy::default(),
    )
    .expect("product config");
    let config = ProductRuntimeConfig {
        product,
        pine_workers: Vec::new(),
        marketdata_helper: None,
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: None,
        market_data_opend_provider: None,
        strategy_runtime_registry: None,
        shutdown_recorder: None,
    };
    let snapshot = ProductRuntimeState::configured(&config).snapshot();
    let runtime = start_product_runtime(config).await.expect("start runtime");
    assert_eq!(runtime.startup_record().owned_routes, 26);
    assert_eq!(snapshot.resources.len(), 11);
    assert_eq!(snapshot.resources[0].id, "settings-file");
    assert_eq!(snapshot.resources[1].id, "backtest-kline-db");
    assert_eq!(snapshot.resources[9].id, "research-db");
    assert_eq!(snapshot.resources[10].id, "real-trade-control");
    assert!(
        snapshot.resources[1..10]
            .iter()
            .all(|resource| resource.kind == "sqlite")
    );
    runtime.shutdown().await.expect("shutdown");
}

#[tokio::test]
async fn opend_runtime_task_requires_explicit_session_composition() {
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::new(
        SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
        directory.path().join("settings.json"),
        AccessPolicy::default(),
    )
    .expect("product config");
    let config = ProductRuntimeConfig {
        product,
        pine_workers: Vec::new(),
        marketdata_helper: None,
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: Some(OpenDSessionRuntimeConfig::default()),
        market_data_opend_provider: None,
        strategy_runtime_registry: None,
        shutdown_recorder: None,
    };
    assert!(matches!(
        start_product_runtime(config).await,
        Err(ProductRuntimeError::MissingOpenDSession)
    ));
}

#[tokio::test]
async fn product_runtime_composes_opend_provider_and_fences_shutdown_ownership() {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("mock OpenD listener");
    let address = listener.local_addr().expect("mock OpenD address");
    let server = thread::spawn(move || {
        let (mut probe, _) = listener.accept().expect("accept health probe");
        let init = read_mock_frame(&mut probe);
        assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
        write_mock_response(&mut probe, &init, init_response());
        let global = read_mock_frame(&mut probe);
        assert_eq!(global.header.proto_id, PROTO_GET_GLOBAL_STATE);
        write_mock_response(&mut probe, &global, global_state_response());

        let (mut session, _) = listener.accept().expect("accept long-lived session");
        let init = read_mock_frame(&mut session);
        assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
        write_mock_response(&mut session, &init, init_response());
        let subscription = read_mock_frame(&mut session);
        assert_eq!(subscription.header.proto_id, PROTO_QOT_SUB);
        write_mock_response(&mut session, &subscription, field(1, 0));
        session
            .set_read_timeout(Some(Duration::from_secs(2)))
            .expect("set session timeout");
        let mut byte = [0_u8; 1];
        let _ = session.read(&mut byte);
    });

    let router = Arc::new(std::sync::Mutex::new(ProviderRouter::new(2)));
    let mut provider = OpenDProviderRuntimeConfig::with_defaults(
        Arc::clone(&router),
        provider_descriptor(),
        OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
        vec![InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }],
        0,
    );
    provider.task.poll_interval = Duration::from_secs(10);
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("product address"),
        directory.path().join("settings.json"),
        "a".repeat(32),
    )
    .expect("product config");
    let config = ProductRuntimeConfig::desktop(product, DesktopRetainedRuntimeConfig::default())
        .expect("runtime config")
        .with_opend_provider_runtime(provider);

    let runtime = start_product_runtime(config)
        .await
        .expect("start product runtime");
    assert_eq!(runtime.startup_record().provider_status, "ready");
    assert_eq!(runtime.startup_record().opend_status, "ready");
    assert_eq!(runtime.startup_record().worker_status, "unavailable");
    assert!(runtime.market_data_opend().is_some());
    assert!(runtime.market_data_opend_runtime_status().is_some());
    {
        let state = router.lock().expect("router lock").runtime().clone();
        assert_eq!(state.active_provider, "futu");
        assert_eq!(state.readiness, ProviderReadiness::Ready);
        assert!(state.connected);
        assert_eq!(state.active_demand, 1);
    }
    assert!(runtime.set_market_data_opend_demand(vec![InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: "MSFT".to_owned(),
        interval: None,
    }]));
    assert_eq!(
        router
            .lock()
            .expect("router lock")
            .demand()
            .active
            .first()
            .map(|instrument| instrument.symbol.as_str()),
        Some("MSFT")
    );
    assert!(!runtime.set_market_data_opend_demand(vec![InstrumentRef {
        channel: "INVALID".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: None,
    }]));
    assert_eq!(
        router
            .lock()
            .expect("router lock")
            .demand()
            .active
            .first()
            .map(|instrument| instrument.symbol.as_str()),
        Some("MSFT")
    );
    assert!(runtime.set_market_data_opend_demand(Vec::new()));
    assert!(
        router
            .lock()
            .expect("router lock")
            .demand()
            .active
            .is_empty()
    );
    runtime.shutdown().await.expect("shutdown product runtime");
    let state = router.lock().expect("router lock").runtime().clone();
    assert!(state.active_provider.is_empty());
    assert!(
        router
            .lock()
            .expect("router lock")
            .demand()
            .active
            .is_empty()
    );
    server.join().expect("mock OpenD server");
}

fn read_mock_frame(stream: &mut TcpStream) -> jftrade_integration_futu::Frame {
    let mut header = [0_u8; 44];
    stream.read_exact(&mut header).expect("read frame header");
    let body_len = u32::from_le_bytes(header[12..16].try_into().expect("body length")) as usize;
    let mut packet = header.to_vec();
    let mut body = vec![0_u8; body_len];
    stream.read_exact(&mut body).expect("read frame body");
    packet.extend(body);
    decode_frame(&packet).expect("decode frame")
}

fn write_mock_response(
    stream: &mut TcpStream,
    request: &jftrade_integration_futu::Frame,
    body: Vec<u8>,
) {
    stream
        .write_all(
            &encode_frame(request.header.proto_id, request.header.serial_no, &body)
                .expect("encode response"),
        )
        .expect("write response");
}

fn varint(mut value: u64) -> Vec<u8> {
    let mut bytes = Vec::new();
    loop {
        let mut byte = (value & 0x7f) as u8;
        value >>= 7;
        if value != 0 {
            byte |= 0x80;
        }
        bytes.push(byte);
        if value == 0 {
            return bytes;
        }
    }
}

fn field(tag: u8, value: u64) -> Vec<u8> {
    let mut bytes = varint(u64::from(tag) << 3);
    bytes.extend(varint(value));
    bytes
}

fn message_field(tag: u8, message: Vec<u8>) -> Vec<u8> {
    let mut bytes = varint((u64::from(tag) << 3) | 2);
    bytes.extend(varint(message.len() as u64));
    bytes.extend(message);
    bytes
}

fn init_response() -> Vec<u8> {
    let mut s2c = field(1, 1009);
    s2c.extend(field(3, 1));
    let mut response = field(1, 0);
    response.extend(message_field(4, s2c));
    response
}

fn global_state_response() -> Vec<u8> {
    let mut state = field(1, 3);
    state.extend(field(2, 4));
    state.extend(field(3, 5));
    state.extend(field(4, 6));
    state.extend(field(6, 1));
    state.extend(field(8, 1009));
    state.extend(field(9, 7000));
    state.extend(field(10, 1_754_000_000));
    let mut response = field(1, 0);
    response.extend(message_field(4, state));
    response
}
