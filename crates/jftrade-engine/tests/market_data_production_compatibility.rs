use prost::Message;
use serde_json::{Value, json};
use std::collections::BTreeMap;
use std::fs;
use std::io::Read as _;
use std::net::{SocketAddr, TcpListener as StdTcpListener, TcpStream as StdTcpStream};
use std::path::PathBuf;
use std::thread;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};

use jftrade_engine::product::ProductConfig;
use jftrade_engine::product_runtime::{ProductRuntimeConfig, start_product_runtime};
use jftrade_integration_futu::{
    Frame, PROTO_GET_GLOBAL_STATE, PROTO_GET_SUB_INFO, PROTO_INIT_CONNECT, PROTO_QOT_SUB,
    decode_frame, encode_frame,
};
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};

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
struct SubResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
}

fn fixture_path(name: &str) -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/rust-migration/stage9")
        .join(name)
}

fn load_fixture(name: &str) -> Value {
    let path = fixture_path(name);
    let content = fs::read_to_string(&path)
        .unwrap_or_else(|err| panic!("failed to read fixture {path:?}: {err}"));
    serde_json::from_str(&content)
        .unwrap_or_else(|err| panic!("failed to parse fixture {path:?}: {err}"))
}

#[derive(Clone, Debug)]
struct ApiResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Value,
}

async fn request_api(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&Value>,
) -> ApiResponse {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let body_str = body
        .map(|b| serde_json::to_string(b).unwrap())
        .unwrap_or_default();
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nAuthorization: Bearer {TEST_DESKTOP_TOKEN}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body_str}",
        body_str.len()
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut raw_response = Vec::new();
    stream
        .read_to_end(&mut raw_response)
        .await
        .expect("read response");
    let response = String::from_utf8(raw_response).expect("UTF-8 response");
    let (head, body_part) = response.split_once("\r\n\r\n").expect("HTTP body");
    let mut lines = head.lines();
    let status = lines
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");

    let mut headers = BTreeMap::new();
    for line in lines {
        if let Some((name, val)) = line.split_once(':') {
            headers.insert(name.trim().to_owned(), val.trim().to_owned());
        }
    }

    let value: Value = serde_json::from_str(body_part).unwrap_or_else(|_| {
        serde_json::json!({
            "raw": body_part
        })
    });
    ApiResponse {
        status,
        headers,
        body: value,
    }
}

fn assert_differential_match(actual: &ApiResponse, expected_case: &Value) {
    let expected_status = expected_case["expectedStatus"]
        .as_u64()
        .expect("expectedStatus in fixture") as u16;
    assert_eq!(
        actual.status, expected_status,
        "Status mismatch for case '{}': expected {}, got {}",
        expected_case["name"], expected_status, actual.status
    );

    if let Some(expected_headers) = expected_case["headers"].as_object() {
        for (header_k, header_v) in expected_headers {
            let actual_v = actual
                .headers
                .get(header_k)
                .or_else(|| {
                    actual.headers.iter().find_map(|(k, v)| {
                        if k.eq_ignore_ascii_case(header_k) {
                            Some(v)
                        } else {
                            None
                        }
                    })
                })
                .map(String::as_str);
            assert_eq!(
                actual_v,
                header_v.as_str(),
                "Header '{}' mismatch for case '{}'",
                header_k,
                expected_case["name"]
            );
        }
    }

    if let Some(expected_data) = expected_case.get("data") {
        assert_eq!(
            &actual.body["data"], expected_data,
            "Data mismatch for case '{}'",
            expected_case["name"]
        );
    }

    if let Some(expected_code) = expected_case.get("errorCode") {
        assert_eq!(
            &actual.body["error"]["code"], expected_code,
            "ErrorCode mismatch for case '{}'",
            expected_case["name"]
        );
    }

    if let Some(expected_message) = expected_case.get("errorMessage") {
        assert_eq!(
            &actual.body["error"]["message"], expected_message,
            "ErrorMessage mismatch for case '{}'",
            expected_case["name"]
        );
    }
}

fn read_framed_packet(stream: &mut StdTcpStream) -> Option<Frame> {
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

fn write_framed_packet(stream: &mut StdTcpStream, proto_id: u32, serial_no: u32, body: &[u8]) {
    let packet = encode_frame(proto_id, serial_no, body).expect("encode frame");
    std::io::Write::write_all(stream, &packet).expect("write frame");
}

#[tokio::test]
async fn test_market_data_canonical_fixture_subscription_matches_production() {
    let fixture = load_fixture("market-data-quote-read.json");
    let expected = fixture["cases"]
        .as_array()
        .and_then(|cases| {
            cases
                .iter()
                .find(|case| case["name"] == "subscriptions-empty")
        })
        .expect("subscriptions fixture case");

    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&json!({
            "activeMarketDataProvider": "yfinance",
            "marketData": {}
        }))
        .unwrap(),
    )
    .unwrap();

    let product = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .unwrap();

    let runtime = start_product_runtime(ProductRuntimeConfig {
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
    })
    .await
    .expect("runtime start");

    let response = request_api(
        runtime.startup_record().address,
        expected["method"].as_str().unwrap_or("GET"),
        expected["requestPath"].as_str().unwrap(),
        None,
    )
    .await;

    assert_differential_match(&response, expected);
    runtime.shutdown().await.expect("clean shutdown");
}

#[tokio::test]
async fn test_futu_quota_and_subscriptions_through_real_composition_runtime() {
    let listener = StdTcpListener::bind(("127.0.0.1", 0)).expect("bind framed mock server");
    let opend_addr = listener.local_addr().expect("local_addr");

    let server_handle = thread::spawn(move || {
        while let Ok((mut stream, _)) = listener.accept() {
            thread::spawn(move || {
                while let Some(frame) = read_framed_packet(&mut stream) {
                    match frame.header.proto_id {
                        PROTO_INIT_CONNECT => {
                            write_framed_packet(
                                &mut stream,
                                PROTO_INIT_CONNECT,
                                frame.header.serial_no,
                                &InitResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                    s2c: Some(InitState {
                                        server_ver: 1009,
                                        conn_id: 1234,
                                    }),
                                }
                                .encode_to_vec(),
                            );
                        }
                        PROTO_GET_GLOBAL_STATE => {
                            write_framed_packet(
                                &mut stream,
                                PROTO_GET_GLOBAL_STATE,
                                frame.header.serial_no,
                                &GetGlobalStateResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                    s2c: Some(GetGlobalStateS2c {
                                        market_hk: 3,
                                        market_us: 4,
                                        market_sh: 5,
                                        market_sz: 6,
                                        qot_logined: true,
                                        trd_logined: true,
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
                        }
                        PROTO_GET_SUB_INFO => {
                            write_framed_packet(
                                &mut stream,
                                PROTO_GET_SUB_INFO,
                                frame.header.serial_no,
                                &GetSubInfoResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                    s2c: Some(GetSubInfoS2c {
                                        total_used_quota: Some(20),
                                        remain_quota: Some(80),
                                        own_used_quota: Some(6),
                                    }),
                                }
                                .encode_to_vec(),
                            );
                        }
                        PROTO_QOT_SUB => {
                            write_framed_packet(
                                &mut stream,
                                PROTO_QOT_SUB,
                                frame.header.serial_no,
                                &SubResponse {
                                    ret_type: Some(0),
                                    ret_msg: Some("ok".to_owned()),
                                }
                                .encode_to_vec(),
                            );
                        }
                        _ => {}
                    }
                }
            });
        }
    });

    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "futu",
        "marketData": {},
        "integration": {
            "enabled": true,
            "config": {
                "host": "127.0.0.1",
                "apiPort": opend_addr.port(),
                "websocketPort": 11111
            }
        }
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let runtime = start_product_runtime(ProductRuntimeConfig {
        product: product_config,
        pine_workers: Vec::new(),
        marketdata_helper: None,
        market_data_router: None,
        market_data_runtime_recorder: None,
        market_data_opend: None,
        market_data_opend_task: None,
        market_data_opend_provider: None,
        strategy_runtime_registry: None,
        shutdown_recorder: None,
    })
    .await
    .expect("runtime start");

    let addr = runtime.startup_record().address;

    // Refresh quota through coordinator
    if let Some(coordinator) = runtime.market_data_opend() {
        let mut guard = coordinator.lock().unwrap();
        let _ = guard.refresh_quota(1_700_000_000_000);
    }

    // 1. Check subscriptions response reflects real OpenD quota values
    let sub_resp = request_api(addr, "GET", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(sub_resp.status, 200);
    assert_eq!(sub_resp.body["data"]["brokerState"]["totalUsedQuota"], 20);
    assert_eq!(sub_resp.body["data"]["brokerState"]["remainQuota"], 80);
    assert_eq!(sub_resp.body["data"]["brokerState"]["ownUsedQuota"], 6);
    assert_eq!(sub_resp.body["data"]["brokerState"]["fallbackCount"], 0);

    // 2. Acquire subscription via POST
    let acq_resp = request_api(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "AAPL"}]
        })),
    )
    .await;
    assert_eq!(acq_resp.status, 200);
    assert_eq!(acq_resp.body["data"]["totalActiveSubscriptions"], 1);

    runtime.shutdown().await.expect("clean shutdown");
    drop(server_handle);
}

#[tokio::test]
async fn test_omitted_broker_subscription_lifecycle_and_single_demand_owner() {
    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "yfinance",
        "marketData": {}
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let runtime_config = ProductRuntimeConfig {
        product: product_config,
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

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Initially 0 subscriptions
    let sub_resp = request_api(addr, "GET", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(sub_resp.status, 200);
    assert_eq!(sub_resp.body["data"]["totalActiveSubscriptions"], 0);

    // 2. Acquire with omitted providerBrokerId -> operates on shared DemandBook
    let acq_resp = request_api(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "NVDA"}]
        })),
    )
    .await;
    assert_eq!(acq_resp.status, 200);
    assert_eq!(acq_resp.body["data"]["totalActiveSubscriptions"], 1);

    // 3. GET reflects the new subscription in the shared snapshot
    let sub_resp2 = request_api(addr, "GET", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(sub_resp2.status, 200);
    assert_eq!(sub_resp2.body["data"]["totalActiveSubscriptions"], 1);
    assert_eq!(sub_resp2.body["data"]["brokerState"]["desiredCount"], 1);
    assert_eq!(
        sub_resp2.body["data"]["entries"][0]["instrumentId"],
        "US.NVDA"
    );

    // 4. Heartbeat with omitted providerBrokerId -> operates on shared DemandBook
    let hb_resp = request_api(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions/heartbeat",
        Some(&json!({
            "consumerId": "chart-view"
        })),
    )
    .await;
    assert_eq!(hb_resp.status, 200);
    assert_eq!(hb_resp.body["data"]["totalActiveSubscriptions"], 1);

    // 5. Release with omitted providerBrokerId -> releases from shared DemandBook
    let rel_resp = request_api(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions/release",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "NVDA"}]
        })),
    )
    .await;
    assert_eq!(rel_resp.status, 200);
    assert_eq!(rel_resp.body["data"]["released"], true);
    assert_eq!(rel_resp.body["data"]["totalActiveSubscriptions"], 0);

    // 6. Acquire again, then DELETE /api/v1/market-data/subscriptions -> clears shared DemandBook
    let _ = request_api(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "TSLA"}]
        })),
    )
    .await;

    let clear_resp = request_api(addr, "DELETE", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(clear_resp.status, 200);
    assert_eq!(clear_resp.body["data"]["cleared"], true);
    assert_eq!(clear_resp.body["data"]["totalActiveSubscriptions"], 0);

    runtime.shutdown().await.expect("clean shutdown");
}

#[tokio::test]
async fn test_active_provider_mutation_atomic_single_state_source() {
    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "yfinance",
        "marketData": {}
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let runtime_config = ProductRuntimeConfig {
        product: product_config,
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

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Initial provider is yfinance
    let prov_resp = request_api(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(prov_resp.status, 200);
    assert_eq!(
        prov_resp.body["data"]["descriptor"]["selectionId"],
        "yfinance"
    );

    // 2. Mutate provider to akshare via PUT /api/v1/settings/market-data-provider
    let put_resp = request_api(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({
            "activeProvider": "akshare"
        })),
    )
    .await;
    assert_eq!(put_resp.status, 200);
    assert_eq!(put_resp.body["data"]["activeProvider"], "akshare");

    // 3. GET /api/v1/market-data/provider immediately reflects akshare without restarting
    let prov_resp2 = request_api(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(prov_resp2.status, 200);
    assert_eq!(
        prov_resp2.body["data"]["descriptor"]["selectionId"],
        "akshare"
    );

    // 4. Invalid mutation rejected, state retained
    let err_resp = request_api(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({
            "activeProvider": "invalid_provider_name"
        })),
    )
    .await;
    assert_eq!(err_resp.status, 400);
    assert_eq!(
        err_resp.body["error"]["code"],
        "MARKET_DATA_PROVIDER_INVALID"
    );

    // Provider state is still akshare
    let prov_resp3 = request_api(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(prov_resp3.status, 200);
    assert_eq!(
        prov_resp3.body["data"]["descriptor"]["selectionId"],
        "akshare"
    );

    runtime.shutdown().await.expect("clean shutdown");
}

#[tokio::test]
async fn test_go_compatible_candle_conversion_and_session_classification() {
    let mock_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let mock_addr = mock_listener.local_addr().unwrap();

    let server_task = tokio::spawn(async move {
        while let Ok((mut stream, _)) = mock_listener.accept().await {
            tokio::spawn(async move {
                let mut buf = vec![0u8; 4096];
                let n = stream.read(&mut buf).await.unwrap_or(0);
                let req_str = String::from_utf8_lossy(&buf[..n]);

                let response_body = if req_str.contains("/providers/yfinance/candles/US/AAPL") {
                    json!({
                        "market": "US",
                        "symbol": "AAPL",
                        "instrumentId": "US.AAPL",
                        "period": "1m",
                        "extendedHours": true,
                        "totalReturned": 4,
                        "hasMore": false,
                        "source": "yfinance",
                        "candles": [
                            {
                                "at": "2026-08-28T08:00:00-04:00",
                                "open": "150.0",
                                "high": "151.0",
                                "low": "149.5",
                                "close": "150.5",
                                "volume": "1000"
                            },
                            {
                                "at": "2026-08-28T10:00:00-04:00",
                                "open": "150.5",
                                "high": "152.0",
                                "low": "150.0",
                                "close": "151.5",
                                "volume": "5000"
                            },
                            {
                                "at": "2026-08-28T17:00:00-04:00",
                                "open": "151.5",
                                "high": "153.0",
                                "low": "151.0",
                                "close": "152.5",
                                "volume": "2000"
                            },
                            {
                                "at": "2026-08-28T22:00:00-04:00",
                                "open": "152.5",
                                "high": "153.0",
                                "low": "152.0",
                                "close": "152.8",
                                "volume": "500"
                            }
                        ]
                    })
                } else if req_str.contains("/providers/akshare/candles/CN/600519") {
                    json!({
                        "market": "CN",
                        "symbol": "600519",
                        "instrumentId": "CN.600519",
                        "period": "1d",
                        "extendedHours": false,
                        "totalReturned": 1,
                        "hasMore": false,
                        "source": "akshare",
                        "candles": [
                            {
                                "at": "2026-08-28T15:00:00+08:00",
                                "open": "1800.0",
                                "high": "1850.0",
                                "low": "1790.0",
                                "close": "1820.0",
                                "volume": "25000"
                            }
                        ]
                    })
                } else {
                    json!({ "error": "not found" })
                };

                let body_str = serde_json::to_string(&response_body).unwrap();
                let resp = format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body_str.len(),
                    body_str
                );
                let _ = stream.write_all(resp.as_bytes()).await;
            });
        }
    });

    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "yfinance",
        "marketData": {}
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let helper_client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{mock_addr}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(2),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .unwrap();

    let product_config = product_config.with_market_data_helper(helper_client);

    let runtime_config = ProductRuntimeConfig {
        product: product_config,
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

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. yfinance intraday session & volume classification
    let candle_resp = request_api(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/AAPL?period=1m&sessions=regular,extended",
        None,
    )
    .await;
    assert_eq!(candle_resp.status, 200);
    let candles = candle_resp.body["data"]["candles"].as_array().unwrap();
    // Overnight candle (22:00 EDT) filtered out -> 3 candles remain
    assert_eq!(candles.len(), 3);
    // Candle 0: Pre-market -> session "pre", volume null
    assert_eq!(candles[0]["session"], "pre");
    assert!(candles[0]["volume"].is_null());
    // Candle 1: Regular -> session "regular", volume "5000"
    assert_eq!(candles[1]["session"], "regular");
    assert_eq!(candles[1]["volume"], "5000");
    // Candle 2: After-hours -> session "after", volume null
    assert_eq!(candles[2]["session"], "after");
    assert!(candles[2]["volume"].is_null());
    // Meta session & extendedHours
    assert_eq!(candle_resp.body["data"]["meta"]["extendedHours"], true);
    assert_eq!(candle_resp.body["data"]["meta"]["session"], "all");

    // 2. AKShare candles -> session is null
    let _ = request_api(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({ "activeProvider": "akshare" })),
    )
    .await;

    let ak_resp = request_api(
        addr,
        "GET",
        "/api/v1/market-data/candles/CN/600519?period=1d",
        None,
    )
    .await;
    assert_eq!(ak_resp.status, 200);
    let ak_candles = ak_resp.body["data"]["candles"].as_array().unwrap();
    assert_eq!(ak_candles.len(), 1);
    assert!(ak_candles[0]["session"].is_null());
    assert_eq!(ak_resp.body["data"]["meta"]["extendedHours"], false);

    runtime.shutdown().await.expect("clean shutdown");
    server_task.abort();
}

#[tokio::test]
async fn test_candle_converter_strict_rejection_boundaries() {
    let mock_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let mock_addr = mock_listener.local_addr().unwrap();

    let server_task = tokio::spawn(async move {
        while let Ok((mut stream, _)) = mock_listener.accept().await {
            tokio::spawn(async move {
                let mut buf = vec![0u8; 4096];
                let n = stream.read(&mut buf).await.unwrap_or(0);
                let req_str = String::from_utf8_lossy(&buf[..n]);

                let response_body = if req_str
                    .contains("/providers/yfinance/candles/US/OUT_OF_ORDER")
                {
                    json!({
                        "market": "US",
                        "symbol": "OUT_OF_ORDER",
                        "instrumentId": "US.OUT_OF_ORDER",
                        "period": "1d",
                        "totalReturned": 2,
                        "hasMore": false,
                        "source": "yfinance",
                        "candles": [
                            {
                                "at": "2026-08-28T16:00:00-04:00",
                                "open": "400.0", "high": "405.0", "low": "395.0", "close": "402.0", "volume": "1000"
                            },
                            {
                                "at": "2026-08-27T16:00:00-04:00",
                                "open": "395.0", "high": "401.0", "low": "390.0", "close": "399.0", "volume": "1200"
                            }
                        ]
                    })
                } else if req_str.contains("/providers/yfinance/candles/US/INVALID_BOUNDS") {
                    json!({
                        "market": "US",
                        "symbol": "INVALID_BOUNDS",
                        "instrumentId": "US.INVALID_BOUNDS",
                        "period": "1d",
                        "totalReturned": 1,
                        "hasMore": false,
                        "source": "yfinance",
                        "candles": [
                            {
                                "at": "2026-08-28T16:00:00-04:00",
                                "open": "400.0", "high": "350.0", "low": "410.0", "close": "402.0", "volume": "1000"
                            }
                        ]
                    })
                } else if req_str.contains("/providers/yfinance/candles/US/BOUNDED_HAS_MORE") {
                    json!({
                        "market": "US",
                        "symbol": "BOUNDED_HAS_MORE",
                        "instrumentId": "US.BOUNDED_HAS_MORE",
                        "period": "1d",
                        "totalReturned": 1,
                        "hasMore": true,
                        "source": "yfinance",
                        "candles": [
                            {
                                "at": "2026-08-28T16:00:00-04:00",
                                "open": "400.0", "high": "405.0", "low": "395.0", "close": "402.0", "volume": "1000"
                            }
                        ]
                    })
                } else {
                    json!({ "error": "not found" })
                };

                let body_str = serde_json::to_string(&response_body).unwrap();
                let resp = format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body_str.len(),
                    body_str
                );
                let _ = stream.write_all(resp.as_bytes()).await;
            });
        }
    });

    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "yfinance",
        "marketData": {}
    });
    fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &settings_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let helper_client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{mock_addr}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(2),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .unwrap();

    let product_config = product_config.with_market_data_helper(helper_client);

    let runtime_config = ProductRuntimeConfig {
        product: product_config,
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

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Out of order timestamps rejected with 502 OPEND_CANDLES_FAILED
    let err1 = request_api(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/OUT_OF_ORDER?period=1d",
        None,
    )
    .await;
    assert_eq!(err1.status, 502);
    assert_eq!(err1.body["error"]["code"], "OPEND_CANDLES_FAILED");

    // 2. Invalid OHLC bounds (high < low) rejected with 502 OPEND_CANDLES_FAILED
    let err2 = request_api(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/INVALID_BOUNDS?period=1d",
        None,
    )
    .await;
    assert_eq!(err2.status, 502);
    assert_eq!(err2.body["error"]["code"], "OPEND_CANDLES_FAILED");

    // 3. Bounded query (from/to) with has_more rejected with 502 OPEND_CANDLES_FAILED
    let err3 = request_api(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/BOUNDED_HAS_MORE?period=1d&from=2026-08-01T00:00:00Z&to=2026-08-28T00:00:00Z",
        None,
    )
    .await;
    assert_eq!(err3.status, 502);
    assert_eq!(err3.body["error"]["code"], "OPEND_CANDLES_FAILED");

    runtime.shutdown().await.expect("clean shutdown");
    server_task.abort();
}
