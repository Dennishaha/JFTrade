use serde_json::{Value, json};
use std::fs;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};

use jftrade_engine::product::ProductConfig;
use jftrade_engine::product_runtime::{ProductRuntimeConfig, start_product_runtime};
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};

const TEST_DESKTOP_TOKEN: &str = "test-desktop-token-entropy-32-chars-long";

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

async fn request_json(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&Value>,
) -> (u16, Value) {
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
    let value: Value = serde_json::from_str(body_part).unwrap_or_else(|_| {
        serde_json::json!({
            "raw": body_part
        })
    });
    (status, value)
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
    })
    .await
    .expect("runtime start");
    let (status, actual) = request_json(
        runtime.startup_record().address,
        expected["method"].as_str().unwrap_or("GET"),
        expected["requestPath"].as_str().unwrap(),
        None,
    )
    .await;
    assert_eq!(status, expected["expectedStatus"].as_u64().unwrap() as u16);
    assert_eq!(actual["data"], expected["data"]);
    runtime.shutdown().await.expect("clean shutdown");
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
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Initially 0 subscriptions
    let (status, sub_json) =
        request_json(addr, "GET", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(status, 200);
    assert_eq!(sub_json["data"]["totalActiveSubscriptions"], 0);

    // 2. Acquire with omitted providerBrokerId -> operates on shared DemandBook
    let (status, acq_json) = request_json(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "NVDA"}]
        })),
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(acq_json["data"]["totalActiveSubscriptions"], 1);

    // 3. GET reflects the new subscription in the shared snapshot
    let (status, sub_json2) =
        request_json(addr, "GET", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(status, 200);
    assert_eq!(sub_json2["data"]["totalActiveSubscriptions"], 1);
    assert_eq!(sub_json2["data"]["brokerState"]["desiredCount"], 1);
    assert_eq!(sub_json2["data"]["entries"][0]["instrumentId"], "US.NVDA");

    // 4. Heartbeat with omitted providerBrokerId -> operates on shared DemandBook
    let (status, hb_json) = request_json(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions/heartbeat",
        Some(&json!({
            "consumerId": "chart-view"
        })),
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(hb_json["data"]["totalActiveSubscriptions"], 1);

    // 5. Release with omitted providerBrokerId -> releases from shared DemandBook
    let (status, rel_json) = request_json(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions/release",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "NVDA"}]
        })),
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(rel_json["data"]["released"], true);
    assert_eq!(rel_json["data"]["totalActiveSubscriptions"], 0);

    // 6. Acquire again, then DELETE /api/v1/market-data/subscriptions -> clears shared DemandBook
    let (status, _) = request_json(
        addr,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(&json!({
            "consumerId": "chart-view",
            "instruments": [{"market": "US", "symbol": "TSLA"}]
        })),
    )
    .await;
    assert_eq!(status, 200);

    let (status, clear_json) =
        request_json(addr, "DELETE", "/api/v1/market-data/subscriptions", None).await;
    assert_eq!(status, 200);
    assert_eq!(clear_json["data"]["cleared"], true);
    assert_eq!(clear_json["data"]["totalActiveSubscriptions"], 0);

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
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Initial provider is yfinance
    let (status, prov_json) = request_json(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(status, 200);
    assert_eq!(prov_json["data"]["descriptor"]["selectionId"], "yfinance");

    // 2. Mutate provider to akshare via PUT /api/v1/settings/market-data-provider
    let (status, put_json) = request_json(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({
            "activeProvider": "akshare"
        })),
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(put_json["data"]["activeProvider"], "akshare");

    // 3. GET /api/v1/market-data/provider immediately reflects akshare without restarting
    let (status, prov_json2) =
        request_json(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(status, 200);
    assert_eq!(prov_json2["data"]["descriptor"]["selectionId"], "akshare");

    // 4. Invalid mutation rejected, state retained
    let (status, err_json) = request_json(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({
            "activeProvider": "invalid_provider_name"
        })),
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(err_json["error"]["code"], "MARKET_DATA_PROVIDER_INVALID");

    // Provider state is still akshare
    let (status, prov_json3) =
        request_json(addr, "GET", "/api/v1/market-data/provider", None).await;
    assert_eq!(status, 200);
    assert_eq!(prov_json3["data"]["descriptor"]["selectionId"], "akshare");

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
                } else if req_str.contains("out_of_order") {
                    json!({
                        "market": "US",
                        "symbol": "MSFT",
                        "instrumentId": "US.MSFT",
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
                } else {
                    json!({
                        "error": "not found"
                    })
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
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. yfinance intraday session & volume classification
    let (status, candle_json) = request_json(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/AAPL?period=1m&sessions=regular,extended",
        None,
    )
    .await;
    assert_eq!(status, 200);
    let candles = candle_json["data"]["candles"].as_array().unwrap();
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
    assert_eq!(candle_json["data"]["meta"]["extendedHours"], true);
    assert_eq!(candle_json["data"]["meta"]["session"], "all");

    // 2. AKShare candles -> session is null
    let _ = request_json(
        addr,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(&json!({ "activeProvider": "akshare" })),
    )
    .await;

    let (status, ak_json) = request_json(
        addr,
        "GET",
        "/api/v1/market-data/candles/CN/600519?period=1d",
        None,
    )
    .await;
    assert_eq!(status, 200);
    let ak_candles = ak_json["data"]["candles"].as_array().unwrap();
    assert_eq!(ak_candles.len(), 1);
    assert!(ak_candles[0]["session"].is_null());
    assert_eq!(ak_json["data"]["meta"]["extendedHours"], false);

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
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;

    // 1. Out of order timestamps rejected with 502 OPEND_CANDLES_FAILED
    let (status, err1) = request_json(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/OUT_OF_ORDER?period=1d",
        None,
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(err1["error"]["code"], "OPEND_CANDLES_FAILED");

    // 2. Invalid OHLC bounds (high < low) rejected with 502 OPEND_CANDLES_FAILED
    let (status, err2) = request_json(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/INVALID_BOUNDS?period=1d",
        None,
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(err2["error"]["code"], "OPEND_CANDLES_FAILED");

    // 3. Bounded query (from/to) with has_more rejected with 502 OPEND_CANDLES_FAILED
    let (status, err3) = request_json(
        addr,
        "GET",
        "/api/v1/market-data/candles/US/BOUNDED_HAS_MORE?period=1d&from=2026-08-01T00:00:00Z&to=2026-08-28T00:00:00Z",
        None,
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(err3["error"]["code"], "OPEND_CANDLES_FAILED");

    runtime.shutdown().await.expect("clean shutdown");
    server_task.abort();
}
