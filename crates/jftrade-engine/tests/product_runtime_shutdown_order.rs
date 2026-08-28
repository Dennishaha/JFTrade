use std::fs;
use std::io::{Read, Write};
use std::net::{IpAddr, Ipv4Addr, SocketAddr, TcpListener};
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tempfile::TempDir;

use jftrade_engine::product::ProductConfig;
use jftrade_engine::product_runtime::{
    MarketDataHelperRuntimeConfig, ProductRuntimeConfig, ShutdownEventRecorder,
    start_product_runtime,
};
use jftrade_integration_futu::{
    OpenDProviderRuntimeConfig, OpenDTcpProbeConfig, PROTO_GET_GLOBAL_STATE, PROTO_INIT_CONNECT,
    decode_frame, encode_frame, provider_descriptor,
};
use jftrade_integration_marketdata_helper::HelperProcessConfig;
use jftrade_marketdata::ProviderRouter;
use prost::Message;

const TEST_DESKTOP_TOKEN: &str = "test-desktop-token-entropy-32-chars-long";

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
}

fn build_mock_opend_server() -> (SocketAddr, std::thread::JoinHandle<()>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind opend");
    let addr = listener.local_addr().expect("addr");
    let handle = std::thread::spawn(move || {
        while let Ok((mut stream, _)) = listener.accept() {
            std::thread::spawn(move || {
                let mut header = [0u8; 44];
                while stream.read_exact(&mut header).is_ok() {
                    let mut body_len_bytes = [0u8; 4];
                    body_len_bytes.copy_from_slice(&header[12..16]);
                    let body_len = u32::from_le_bytes(body_len_bytes) as usize;
                    let mut packet = vec![0u8; 44 + body_len];
                    packet[..44].copy_from_slice(&header);
                    if stream.read_exact(&mut packet[44..]).is_err() {
                        break;
                    }
                    if let Ok(frame) = decode_frame(&packet) {
                        match frame.header.proto_id {
                            PROTO_INIT_CONNECT => {
                                let resp = InitResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                    s2c: Some(InitState {
                                        server_ver: 1009,
                                        conn_id: 12345,
                                    }),
                                };
                                let mut body = Vec::new();
                                resp.encode(&mut body).unwrap();
                                let resp_bytes =
                                    encode_frame(PROTO_INIT_CONNECT, frame.header.serial_no, &body)
                                        .unwrap();
                                let _ = stream.write_all(&resp_bytes);
                            }
                            PROTO_GET_GLOBAL_STATE => {
                                let resp = GetGlobalStateResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                    s2c: Some(GetGlobalStateS2c {
                                        market_hk: 1,
                                        market_us: 1,
                                        market_sh: 1,
                                        market_sz: 1,
                                        qot_logined: true,
                                        trd_logined: true,
                                        server_ver: 1009,
                                        server_build_no: 7000,
                                        time: 1_700_000_000,
                                    }),
                                };
                                let mut body = Vec::new();
                                resp.encode(&mut body).unwrap();
                                let resp_bytes = encode_frame(
                                    PROTO_GET_GLOBAL_STATE,
                                    frame.header.serial_no,
                                    &body,
                                )
                                .unwrap();
                                let _ = stream.write_all(&resp_bytes);
                            }
                            _ => {}
                        }
                    }
                }
            });
        }
    });
    (addr, handle)
}

fn build_mock_helper_server() -> (SocketAddr, std::thread::JoinHandle<()>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind helper");
    let addr = listener.local_addr().expect("addr");
    let handle = std::thread::spawn(move || {
        while let Ok((mut stream, _)) = listener.accept() {
            let mut buf = [0u8; 1024];
            let _ = stream.read(&mut buf);
            let body = b"{\"status\":\"ready\",\"ok\":true}";
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                body.len()
            );
            let _ = stream.write_all(response.as_bytes());
            let _ = stream.write_all(body);
        }
    });
    (addr, handle)
}

async fn build_runtime_config(
    temp_dir: &TempDir,
    recorder: ShutdownEventRecorder,
    fail_listen_port: Option<u16>,
) -> (
    ProductRuntimeConfig,
    SocketAddr,
    (SocketAddr, std::thread::JoinHandle<()>),
    (SocketAddr, std::thread::JoinHandle<()>),
) {
    let (opend_addr, opend_handle) = build_mock_opend_server();
    let (helper_addr, helper_handle) = build_mock_helper_server();

    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "futu",
        "marketData": {
            "futu": {
                "host": "127.0.0.1",
                "port": opend_addr.port(),
                "autoConnect": true
            }
        }
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let listen_addr = if let Some(port) = fail_listen_port {
        SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), port)
    } else {
        SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 0)
    };

    let product_config =
        ProductConfig::desktop_production(listen_addr, &settings_path, TEST_DESKTOP_TOKEN)
            .expect("product config");

    let router = Arc::new(Mutex::new(ProviderRouter::new(100)));
    let provider_config = OpenDProviderRuntimeConfig::with_defaults(
        router,
        provider_descriptor(),
        OpenDTcpProbeConfig::new(opend_addr, Duration::from_secs(1)),
        Vec::new(),
        1_700_000_000_000,
    );

    let helper_config = MarketDataHelperRuntimeConfig {
        process: HelperProcessConfig {
            executable: PathBuf::from("/bin/sh"),
            host: helper_addr.ip(),
            port: helper_addr.port(),
            bearer_token: None,
            prefix_args: Vec::new(),
            extra_args: vec!["-c".to_owned(), "sleep 30".to_owned()],
            environment: Default::default(),
            log_path: None,
            stop_timeout: Duration::from_millis(500),
        },
        startup_timeout: Duration::from_secs(5),
        initial_retry_delay: Duration::from_millis(50),
        max_retry_delay: Duration::from_millis(100),
        request_timeout: Duration::from_secs(2),
    };

    let config = ProductRuntimeConfig {
        product: product_config,
        pine_workers: Vec::new(),
        marketdata_helper: Some(helper_config),
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: None,
        market_data_opend_provider: Some(provider_config),
        strategy_runtime_registry: None,
        shutdown_recorder: Some(recorder),
    };

    (
        config,
        listen_addr,
        (opend_addr, opend_handle),
        (helper_addr, helper_handle),
    )
}

#[tokio::test]
async fn test_product_runtime_ordered_shutdown_explicit() {
    let temp_dir = TempDir::new().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, _, _, _) = build_runtime_config(&temp_dir, recorder.clone(), None).await;

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");
    let result = handle.shutdown().await;
    assert!(result.is_ok(), "shutdown must succeed");

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "provider",
            "opend",
            "helper_pine",
            "sqlite_lease"
        ],
        "explicit shutdown must follow complete 5-stage order"
    );
}

#[tokio::test]
async fn test_product_runtime_startup_failure_rollback() {
    let temp_dir = TempDir::new().unwrap();
    let recorder = ShutdownEventRecorder::new();

    // Pre-bind a port to cause startup failure during listener binding in start_product_runtime
    let conflict_listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind conflict");
    let conflict_port = conflict_listener.local_addr().expect("port").port();

    let (config, _, _, _) =
        build_runtime_config(&temp_dir, recorder.clone(), Some(conflict_port)).await;

    // Must return Err because listener could not bind!
    let result = start_product_runtime(config).await;
    assert!(result.is_err(), "startup must fail due to conflicting port");

    // Supervisor rollback inside start_product_runtime must execute the reverse teardown of all assembled resources!
    assert_eq!(
        recorder.events(),
        vec!["provider", "opend", "helper_pine"],
        "startup failure rollback must execute ordered teardown of initialized stages"
    );
}

#[tokio::test]
async fn test_product_runtime_ordered_shutdown_direct_drop() {
    let temp_dir = TempDir::new().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, _, _, _) = build_runtime_config(&temp_dir, recorder.clone(), None).await;

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");
    drop(handle);

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "provider",
            "opend",
            "helper_pine",
            "sqlite_lease"
        ],
        "direct drop must follow complete 5-stage order"
    );
}

#[test]
fn test_product_runtime_ordered_shutdown_tokio_runtime_exit_drop() {
    let temp_dir = TempDir::new().unwrap();
    let recorder = ShutdownEventRecorder::new();

    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap();

    let (handle, recorder) = rt.block_on(async {
        let (config, _, _, _) = build_runtime_config(&temp_dir, recorder.clone(), None).await;
        let handle = start_product_runtime(config)
            .await
            .expect("start product runtime");
        let rec = handle.shutdown_recorder();
        (handle, rec)
    });

    // Drop Tokio runtime first
    drop(rt);

    // Drop handle outside of Tokio runtime
    drop(handle);

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "provider",
            "opend",
            "helper_pine",
            "sqlite_lease"
        ],
        "tokio exit drop must follow complete 5-stage order"
    );
}
