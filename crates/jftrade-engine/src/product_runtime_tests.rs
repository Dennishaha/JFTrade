use std::io::{Read, Write};
use std::net::{Ipv4Addr, SocketAddr, TcpListener, TcpStream};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;

use jftrade_api::AccessPolicy;
use jftrade_integration_futu::{
    OpenDProviderRuntimeConfig, OpenDTcpProbeConfig, PROTO_GET_GLOBAL_STATE, PROTO_INIT_CONNECT,
    PROTO_QOT_SUB, decode_frame, encode_frame, provider_descriptor,
};
use jftrade_marketdata::{InstrumentRef, ProviderReadiness, ProviderRouter};
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream as AsyncTcpStream;

use super::*;
use crate::product::product_production_ports::{self, ProductionAdapterBinding};
use crate::product_data_management;

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
        backtest_execution_port: None,
        strategy_pine_worker_port: None,
        shutdown_recorder: None,
        #[cfg(test)]
        inject_startup_failure: false,
    };
    let snapshot = ProductRuntimeState::configured(&config).snapshot();
    let runtime = start_product_runtime(config).await.expect("start runtime");
    assert_eq!(runtime.backtest_execution_ready(), None);
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
async fn production_runtime_rejects_multiple_pine_workers_without_failover() {
    let directory = tempdir().expect("temporary directory");
    let product = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("product address"),
        directory.path().join("settings.json"),
        "a".repeat(32),
    )
    .expect("product config");
    let retained = DesktopRetainedRuntimeConfig {
        pine: Some(DesktopPineRuntimeConfig {
            runtime_path: std::path::PathBuf::from("/unused/node"),
            bundle_path: std::path::PathBuf::from("/unused/worker.mjs"),
            proto_path: std::path::PathBuf::from("/unused/pine.proto"),
            bearer_token: "b".repeat(32),
            worker_count: 2,
            log_path: None,
        }),
        marketdata: None,
    };
    let config = ProductRuntimeConfig::desktop(product, retained).expect("runtime config");

    let result = start_product_runtime(config).await;
    let Err(error) = result else {
        panic!("production startup must reject unsupported Pine worker failover");
    };
    assert!(matches!(
        &error,
        ProductRuntimeError::PineWorkerFailoverUnsupported { configured: 2 }
    ));
    assert!(error.to_string().contains("failover"));
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
        backtest_execution_port: None,
        strategy_pine_worker_port: None,
        shutdown_recorder: None,
        #[cfg(test)]
        inject_startup_failure: false,
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
        let mut byte = [0_u8; 1];
        while session.read(&mut byte).is_ok_and(|read| read > 0) {}
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
    // This test owns the mock session only to prove composition and ordered
    // shutdown. Keep the unrelated periodic poll asleep until shutdown wakes
    // it, and let the runtime close the long-lived mock connection instead of
    // racing a short server-side timeout on contended CI runners.
    provider.task.poll_interval = Duration::from_secs(60 * 60);
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
                                write_mock_response(&mut stream, &frame, init_response());
                            }
                            PROTO_GET_GLOBAL_STATE => {
                                write_mock_response(&mut stream, &frame, global_state_response());
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

fn build_mock_helper_server() -> (SocketAddr, std::thread::JoinHandle<()>, Arc<AtomicBool>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind helper");
    let addr = listener.local_addr().expect("addr");
    let healthy = Arc::new(AtomicBool::new(true));
    let flag = Arc::clone(&healthy);
    let handle = std::thread::spawn(move || {
        while let Ok((mut stream, _)) = listener.accept() {
            let mut buf = [0u8; 1024];
            let _ = stream.read(&mut buf);
            // Controllable /healthz: false serves a real 500 so the health
            // monitor observes an actual failed round trip.
            let (status, body): (&str, &[u8]) = if flag.load(Ordering::Acquire) {
                ("200 OK", b"{\"status\":\"ready\",\"ok\":true}".as_slice())
            } else {
                ("500 Internal Server Error", b"{\"status\":\"error\"}")
            };
            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                body.len()
            );
            let _ = stream.write_all(response.as_bytes());
            let _ = stream.write_all(body);
        }
    });
    (addr, handle, healthy)
}

fn assert_all_9_databases_can_acquire_writer_leases(settings_path: &std::path::Path) {
    let descriptors = product_data_management::managed_database_runtime_descriptors(settings_path);
    assert_eq!(descriptors.len(), 9);
    for db_id in product_production_ports::PRODUCTION_DATABASE_IDS {
        let descriptor = descriptors
            .iter()
            .find(|d| d.id == db_id)
            .unwrap_or_else(|| panic!("missing descriptor for {db_id}"));
        let path = std::path::PathBuf::from(&descriptor.path);
        let open_res: Result<(), String> = match db_id {
            jftrade_datamanagement::DATABASE_WATCHLIST => {
                jftrade_store_sqlite::WatchlistStore::open_existing(
                    &path,
                    jftrade_store_sqlite::WATCHLIST_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_STRATEGY => {
                jftrade_store_sqlite::StrategyDefinitionStore::open_existing(
                    &path,
                    jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_ADK => jftrade_store_sqlite::AdkStore::open_existing(
                &path,
                jftrade_store_sqlite::ADK_PRODUCTION_PROFILE,
            )
            .map(|_| ())
            .map_err(|e| e.to_string()),
            jftrade_datamanagement::DATABASE_ADK_SESSION => {
                jftrade_store_sqlite::AdkSessionStore::open_existing(
                    &path,
                    jftrade_store_sqlite::ADK_SESSION_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_ADK_ARTIFACT => {
                jftrade_store_sqlite::AdkArtifactStore::open_existing(
                    &path,
                    jftrade_store_sqlite::ADK_ARTIFACT_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_BACKTEST_RUNS => {
                jftrade_store_sqlite::BacktestRunStore::open_existing(
                    &path,
                    jftrade_store_sqlite::BACKTEST_RUNS_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_BACKTEST => {
                jftrade_store_sqlite::BacktestMarketDataStore::open_existing(
                    &path,
                    jftrade_store_sqlite::BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_EXECUTION => {
                jftrade_store_sqlite::ExecutionOrderStore::open_existing(
                    &path,
                    jftrade_store_sqlite::EXECUTION_ORDERS_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            jftrade_datamanagement::DATABASE_RESEARCH => {
                jftrade_store_sqlite::ResearchPresetStore::open_existing(
                    &path,
                    jftrade_store_sqlite::RESEARCH_PRESET_PRODUCTION_PROFILE,
                )
                .map(|_| ())
                .map_err(|e| e.to_string())
            }
            other => panic!("unknown database {other}"),
        };
        open_res.unwrap_or_else(|err| {
            panic!("database {db_id} writer lease acquisition failed after shutdown: {err}")
        });
    }
}

async fn request_json_with_status(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&str>,
    headers: &[(&str, &str)],
) -> (u16, serde_json::Value) {
    let body = body.unwrap_or_default();
    let mut stream = AsyncTcpStream::connect(address)
        .await
        .expect("connect product API");
    let extra_headers = headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\n{extra_headers}Content-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write product request");
    let mut response = Vec::new();
    stream
        .read_to_end(&mut response)
        .await
        .expect("read product response");
    let response = String::from_utf8(response).expect("UTF-8 product response");
    let (headers, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let status = headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    (status, serde_json::from_str(body).expect("JSON response"))
}

/// External fixtures backing one composed runtime: real mock servers plus the
/// controllable helper health flag and the mock PineTS worker gRPC server.
struct TestRuntimeServers {
    _opend_handle: std::thread::JoinHandle<()>,
    _helper_handle: std::thread::JoinHandle<()>,
    helper_healthy: Arc<AtomicBool>,
    _pine_mock: jftrade_integration_pine::MockPineWorker,
}

async fn build_test_runtime_config(
    temp_dir: &tempfile::TempDir,
    recorder: ShutdownEventRecorder,
    fail_listen_port: Option<u16>,
    inject_fault: bool,
) -> (ProductRuntimeConfig, TestRuntimeServers) {
    let (opend_addr, opend_handle) = build_mock_opend_server();
    let (helper_addr, helper_handle, helper_healthy) = build_mock_helper_server();

    // Real controllable mock PineTS worker: a real loopback gRPC HealthCheck
    // server (bearer-token validating) proves the readiness probe path, while
    // the spawned fixture child proves the real process lifecycle.
    let pine_token = "b".repeat(32);
    let pine_mock =
        jftrade_integration_pine::spawn_mock_pine_worker("pineworker-1", Some(&pine_token))
            .await
            .expect("spawn mock pine worker");
    jftrade_integration_pine::wait_until_listening(pine_mock.address(), Duration::from_secs(2))
        .await
        .expect("mock pine worker listening");

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
    std::fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let listen_addr = if let Some(port) = fail_listen_port {
        SocketAddr::new(std::net::IpAddr::V4(Ipv4Addr::LOCALHOST), port)
    } else {
        SocketAddr::new(std::net::IpAddr::V4(Ipv4Addr::LOCALHOST), 0)
    };

    let product_config =
        ProductConfig::desktop_production(listen_addr, &settings_path, "a".repeat(32))
            .expect("product config");

    let router = Arc::new(std::sync::Mutex::new(ProviderRouter::new(100)));
    let provider_config = OpenDProviderRuntimeConfig::with_defaults(
        router,
        provider_descriptor(),
        OpenDTcpProbeConfig::new(opend_addr, Duration::from_secs(1)),
        Vec::new(),
        1_700_000_000_000,
    );

    // Real controllable mock helper child: `sh -c "exec sleep 300"` ignores
    // the appended --host/--port arguments and stays alive until the
    // supervisor really stops it; /healthz is answered by the loopback mock
    // server above.
    let helper_config = MarketDataHelperRuntimeConfig {
        process: jftrade_integration_marketdata_helper::HelperProcessConfig {
            executable: std::path::PathBuf::from("/bin/sh"),
            host: helper_addr.ip(),
            port: helper_addr.port(),
            bearer_token: None,
            prefix_args: vec!["-c".to_owned(), "exec sleep 300".to_owned()],
            extra_args: Vec::new(),
            environment: Default::default(),
            log_path: None,
            stop_timeout: Duration::from_millis(500),
        },
        startup_timeout: Duration::from_secs(5),
        initial_retry_delay: Duration::from_millis(50),
        max_retry_delay: Duration::from_millis(100),
        request_timeout: Duration::from_secs(2),
        health_interval: Duration::from_millis(50),
        // Wide enough to survive the synchronous production-port construction
        // window of the current-thread test runtime; staleness semantics are
        // covered by the monitor's own tests.
        // A full engine test run can spend several seconds waiting for other
        // production fixtures to release their leases before this assertion
        // executes.  Keep the seeded startup evidence valid for that bounded
        // construction window; the monitor's dedicated stale-TTL test still
        // verifies expiry semantics with a short TTL.
        health_ttl: Duration::from_secs(30),
    };

    // PineProcess appends worker arguments after the bundle path.  Use a
    // real temporary shell script here so `/bin/sh` receives a valid script
    // operand; passing `"-c exec sleep 300"` as one path makes macOS sh treat
    // it as an invalid option and can make the worker disappear before Drop.
    let pine_wrapper = temp_dir.path().join("pine-worker-wrapper.sh");
    std::fs::write(&pine_wrapper, "#!/bin/sh\nexec sleep 300\n").unwrap();

    let pine_config = PineWorkerRuntimeConfig {
        spec: pine_mock.spec("pineworker-1"),
        process: jftrade_integration_pine::PineProcessConfig {
            runtime: std::path::PathBuf::from("/bin/sh"),
            bundle_path: pine_wrapper,
            proto_path: None,
            max_message_bytes: None,
            pine_ts_version: None,
            bearer_token: Some(pine_token),
            environment: Default::default(),
            log_path: None,
            stop_timeout: Duration::from_secs(2),
        },
        readiness: jftrade_integration_pine::PineReadinessPolicy::go_compatibility(),
        connect_timeout: Duration::from_secs(1),
        request_timeout: Duration::from_secs(2),
    };

    let config = ProductRuntimeConfig {
        product: product_config,
        pine_workers: vec![pine_config],
        marketdata_helper: Some(helper_config),
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: None,
        market_data_opend_provider: Some(provider_config),
        strategy_runtime_registry: None,
        backtest_execution_port: None,
        strategy_pine_worker_port: None,
        shutdown_recorder: Some(recorder),
        #[cfg(test)]
        inject_startup_failure: inject_fault,
    };

    (
        config,
        TestRuntimeServers {
            _opend_handle: opend_handle,
            _helper_handle: helper_handle,
            helper_healthy,
            _pine_mock: pine_mock,
        },
    )
}

#[tokio::test]
async fn test_product_runtime_ordered_shutdown_explicit() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, _servers) =
        build_test_runtime_config(&temp_dir, recorder.clone(), None, false).await;
    let settings_path = config.product.settings_path().to_path_buf();

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");
    assert_eq!(handle.backtest_execution_ready(), Some(true));

    let lease_snapshot = handle.database_leases().expect("production leases");
    assert_eq!(lease_snapshot.expected, 9);
    assert_eq!(lease_snapshot.acquired, 9);
    assert_eq!(lease_snapshot.status, "acquired");
    assert_eq!(
        lease_snapshot.databases,
        product_production_ports::PRODUCTION_DATABASE_IDS
            .iter()
            .map(|s| s.to_string())
            .collect::<Vec<_>>()
    );

    let result = handle.shutdown().await;
    assert!(result.is_ok(), "shutdown must succeed");

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "execution_reconciliation_worker",
            "provider",
            "opend",
            "marketdata_helper",
            "pine_worker",
            "sqlite_lease"
        ],
        "explicit shutdown must follow complete reverse-of-construction order"
    );

    assert_all_9_databases_can_acquire_writer_leases(&settings_path);
}

#[tokio::test]
async fn test_product_runtime_startup_failure_rollback() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, _servers) =
        build_test_runtime_config(&temp_dir, recorder.clone(), None, true).await;
    let settings_path = config.product.settings_path().to_path_buf();

    let result = start_product_runtime(config).await;
    assert!(
        result.is_err(),
        "startup must fail due to injected fault before HTTP exposure"
    );

    assert_eq!(
        recorder.events(),
        vec![
            "execution_reconciliation_worker",
            "provider",
            "opend",
            "marketdata_helper",
            "pine_worker",
            "sqlite_lease"
        ],
        "startup failure rollback after lease acquisition must release provider, OpenD, \
         helper, Pine workers and the 9 SQLite WriterLeases in reverse order"
    );

    assert_all_9_databases_can_acquire_writer_leases(&settings_path);
}

#[tokio::test]
async fn test_product_runtime_ordered_shutdown_direct_drop() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, _servers) =
        build_test_runtime_config(&temp_dir, recorder.clone(), None, false).await;
    let settings_path = config.product.settings_path().to_path_buf();

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");
    drop(handle);

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "execution_reconciliation_worker",
            "provider",
            "opend",
            "marketdata_helper",
            "pine_worker",
            "sqlite_lease"
        ],
        "direct drop must follow complete reverse order"
    );

    assert_all_9_databases_can_acquire_writer_leases(&settings_path);
}

#[test]
fn test_product_runtime_ordered_shutdown_tokio_runtime_exit_drop() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();

    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap();

    let (handle, recorder, settings_path) = rt.block_on(async {
        let (config, _servers) =
            build_test_runtime_config(&temp_dir, recorder.clone(), None, false).await;
        let path = config.product.settings_path().to_path_buf();
        let handle = start_product_runtime(config)
            .await
            .expect("start product runtime");
        let rec = handle.shutdown_recorder();
        (handle, rec, path)
    });

    // Drop Tokio runtime first
    drop(rt);

    // Drop handle outside Tokio runtime
    drop(handle);

    assert_eq!(
        recorder.events(),
        vec![
            "http_join",
            "execution_reconciliation_worker",
            "provider",
            "opend",
            "marketdata_helper",
            "pine_worker",
            "sqlite_lease"
        ],
        "tokio exit drop must follow complete reverse order"
    );

    assert_all_9_databases_can_acquire_writer_leases(&settings_path);
}

/// The health monitor must be the single source of truth: when /healthz really
/// starts failing over HTTP the dynamic provider readiness and therefore the
/// capability matrix degrade; a recovered check restores them.
#[tokio::test]
async fn test_helper_health_failure_dynamically_downgrades_provider_readiness() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, servers) =
        build_test_runtime_config(&temp_dir, recorder.clone(), None, false).await;

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");

    let monitor = handle.helper_health().expect("helper health monitor");
    assert!(
        monitor.is_ready(),
        "seeded helper health must be ready, snapshot={:?}",
        monitor.snapshot()
    );
    let ready = handle
        .supervisor
        .active_provider_state
        .as_ref()
        .expect("state");
    assert!(
        ready.snapshot().helper_ready,
        "helper must be ready initially"
    );

    // Real failed /healthz round trips: the mock server serves 500.
    servers.helper_healthy.store(false, Ordering::Release);
    let mut degraded = false;
    for _ in 0..100 {
        if !monitor.is_ready() && !ready.snapshot().helper_ready {
            degraded = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    assert!(
        degraded,
        "failing /healthz must dynamically downgrade readiness"
    );
    let snapshot = monitor.snapshot();
    assert!(!snapshot.healthy);
    assert!(snapshot.last_error.is_some());
    assert!(snapshot.checked_at.is_some());

    // Recovery through real successful checks.
    servers.helper_healthy.store(true, Ordering::Release);
    let mut recovered = false;
    for _ in 0..100 {
        if monitor.is_ready() && ready.snapshot().helper_ready {
            recovered = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    assert!(recovered, "recovered /healthz must restore readiness");
    assert!(monitor.snapshot().last_error.is_none());

    handle.shutdown().await.expect("shutdown");
}

/// The Pine health monitor is the shared production readiness source: a real
/// failed HealthCheck downgrades both Pine-backed route bindings and the
/// execution port, while a later healthy probe restores them.  Stopping the
/// runtime then joins the monitor before tearing down the worker process.
#[tokio::test]
async fn pine_worker_health_failure_recovers_and_shutdown_joins_monitor() {
    let temp_dir = tempfile::tempdir().unwrap();
    let recorder = ShutdownEventRecorder::new();
    let (config, servers) =
        build_test_runtime_config(&temp_dir, recorder.clone(), None, false).await;

    let handle = start_product_runtime(config)
        .await
        .expect("start product runtime");
    let state = handle
        .supervisor
        .production_ports
        .as_ref()
        .and_then(|ports| ports.pine_readiness.clone())
        .expect("shared Pine readiness state");
    let ports = handle
        .supervisor
        .production_ports
        .as_ref()
        .expect("production ports");
    let backtest = product_production_ports::ProductionRouteAdapter::BacktestStart;
    let strategy_pine = product_production_ports::ProductionRouteAdapter::StrategyPine;

    assert!(state.is_ready(), "startup probe must seed readiness");
    assert_eq!(handle.backtest_execution_ready(), Some(true));
    assert_eq!(
        ports.adapter_binding(backtest),
        Some(ProductionAdapterBinding::Ready)
    );
    assert_eq!(
        ports.adapter_binding(strategy_pine),
        Some(ProductionAdapterBinding::Ready)
    );

    let token = "a".repeat(32);
    let auth_headers = [("Authorization", format!("Bearer {token}"))];
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 200, "system status response: {response}");
    assert!(
        response["data"]["observability"].get("workers").is_none(),
        "system status must preserve the Go-compatible observability shape: {response}"
    );

    // HealthCheck remains reachable but returns ok=false, matching a worker
    // that is alive yet cannot execute Pine requests.
    servers._pine_mock.set_ok(false);
    for _ in 0..100 {
        if !state.is_ready() {
            break;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    assert!(!state.is_ready(), "failed HealthCheck must clear readiness");
    assert_eq!(handle.backtest_execution_ready(), Some(false));
    assert_eq!(
        ports.adapter_binding(backtest),
        Some(ProductionAdapterBinding::ExternalUnavailable)
    );
    assert_eq!(
        ports.adapter_binding(strategy_pine),
        Some(ProductionAdapterBinding::ExternalUnavailable)
    );

    // The public routes stay registered while the shared worker is down, but
    // both execution surfaces fail closed before invoking or persisting work.
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 200, "degraded system status response: {response}");
    assert!(
        response["data"]["observability"].get("workers").is_none(),
        "degraded system status must preserve the Go-compatible observability shape: {response}"
    );

    let strategy_body = r#"{"script":"//@version=6\nindicator(\"unavailable\")"}"#;
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategy-pine/analyze",
        Some(strategy_body),
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 503, "strategy-pine response: {response}");
    assert_eq!(
        response["error"]["code"],
        "STRATEGY_PINE_ANALYZE_UNAVAILABLE"
    );

    let backtest_validation_cases = [
        ("malformed json", "{", 400, "BAD_REQUEST"),
        ("missing definition", "{}", 400, "BAD_REQUEST"),
        (
            "invalid interval",
            r#"{"definitionId":"inline","strategyScript":"//@version=6\nindicator(\"unavailable\")","symbol":"US.AAPL","interval":"2m","startTime":"2026-08-01T00:00:00Z","endTime":"2026-08-02T00:00:00Z"}"#,
            400,
            "BAD_REQUEST",
        ),
        (
            "missing definition resource",
            r#"{"definitionId":"missing","market":"US","symbol":"US.AAPL","startTime":"2026-08-01T00:00:00Z","endTime":"2026-08-02T00:00:00Z"}"#,
            404,
            "NOT_FOUND",
        ),
        (
            "definition version conflict",
            r#"{"definitionId":"missing","definitionVersion":"v1","version":"v2"}"#,
            409,
            "CONFLICT",
        ),
    ];
    for (name, body, expected_status, expected_code) in backtest_validation_cases {
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            "POST",
            "/api/v1/backtests",
            Some(body),
            &[("Authorization", auth_headers[0].1.as_str())],
        )
        .await;
        assert_eq!(
            status, expected_status,
            "backtest {name} response: {response}"
        );
        assert_eq!(
            response["error"]["code"], expected_code,
            "backtest {name} response: {response}"
        );
    }
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/backtests",
        None,
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 200, "backtest list response: {response}");
    assert!(
        response["runs"].is_null(),
        "unhealthy BacktestStart must not persist or queue a run: {response}"
    );
    assert!(
        !ports
            .mcp_catalog
            .callable_tools()
            .iter()
            .any(|tool| tool["id"] == "strategy.research_backtest"),
        "ADK catalog must hide Pine-backed backtest while worker is unhealthy"
    );

    // A recovered HealthCheck republishes ready evidence without recreating a
    // process or changing the immutable route catalog.
    servers._pine_mock.set_ok(true);
    for _ in 0..100 {
        if state.is_ready() {
            break;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    assert!(state.is_ready(), "healthy probe must restore readiness");
    assert_eq!(
        ports.adapter_binding(backtest),
        Some(ProductionAdapterBinding::Ready)
    );
    assert_eq!(
        ports.adapter_binding(strategy_pine),
        Some(ProductionAdapterBinding::Ready)
    );
    assert!(
        ports
            .mcp_catalog
            .callable_tools()
            .iter()
            .any(|tool| tool["id"] == "strategy.research_backtest"),
        "ADK catalog must restore Pine-backed backtest after recovery"
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 200, "recovered system status response: {response}");
    assert!(
        response["data"]["observability"].get("workers").is_none(),
        "recovered system status must preserve the Go-compatible observability shape: {response}"
    );
    // The readiness gate is no longer the reason for rejection after
    // recovery; malformed input reaches the normal backtest validation path.
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/backtests",
        Some("{}"),
        &[("Authorization", auth_headers[0].1.as_str())],
    )
    .await;
    assert_eq!(status, 400, "recovered backtest validation: {response}");
    assert_eq!(response["error"]["code"], "BAD_REQUEST");

    handle.shutdown().await.expect("shutdown product runtime");
    assert!(!state.is_ready(), "shutdown must clear shared readiness");
}

#[tokio::test]
async fn production_external_worker_failure_keeps_api_degraded() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("settings file");
    let product = ProductConfig::desktop_production(
        SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
        &settings_path,
        "a".repeat(32),
    )
    .expect("product config");
    let config = ProductRuntimeConfig::desktop(product, DesktopRetainedRuntimeConfig::default())
        .expect("runtime config");
    let mut config = config;
    config.marketdata_helper = Some(MarketDataHelperRuntimeConfig {
        process: jftrade_integration_marketdata_helper::HelperProcessConfig {
            executable: std::path::PathBuf::from("/definitely/missing/jftrade-helper"),
            host: Ipv4Addr::LOCALHOST.into(),
            port: 9,
            bearer_token: None,
            prefix_args: Vec::new(),
            extra_args: Vec::new(),
            environment: Default::default(),
            log_path: None,
            stop_timeout: Duration::from_millis(50),
        },
        startup_timeout: Duration::from_millis(50),
        initial_retry_delay: Duration::from_millis(1),
        max_retry_delay: Duration::from_millis(1),
        request_timeout: Duration::from_millis(20),
        health_interval: Duration::from_millis(20),
        health_ttl: Duration::from_millis(50),
    });

    let runtime = start_product_runtime(config)
        .await
        .expect("external helper failure must not abort production API");
    assert_eq!(runtime.startup_record().worker_status, "unavailable");
    assert_eq!(runtime.startup_record().runtime_readiness, "degraded");
    runtime.shutdown().await.expect("shutdown");
}
