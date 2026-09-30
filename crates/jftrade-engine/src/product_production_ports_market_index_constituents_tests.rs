use super::*;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

fn helper(base_url: String) -> HelperClient {
    HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
        base_url,
        bearer_token: None,
        request_timeout: Duration::from_secs(1),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client")
}

fn port(
    provider: Option<MarketDataProvider>,
    helper_ready: bool,
    helper: Option<HelperClient>,
) -> ProductionMarketIndexConstituentsPort {
    let state = Arc::new(ActiveProviderState::new(provider));
    state.set_readiness(helper_ready, false, false);
    ProductionMarketIndexConstituentsPort {
        active_provider_state: state,
        helper,
    }
}

fn akshare_port(base_url: String) -> ProductionMarketIndexConstituentsPort {
    port(
        Some(MarketDataProvider::Akshare),
        true,
        Some(helper(base_url)),
    )
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_index_constituents_forwarding_test.go:35 TestRuntimeForwardsIndexConstituentsToCapableActiveProvider
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
// Parity: go:452dea11:internal/marketdata/index_constituents_facade_test.go:41 TestServiceIndexConstituentsValidatesLimitAndForwardsArguments
// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:14 TestADKMarketIndexConstituentsToolForwardsNormalizedInputs
async fn index_constituents_read_forwards_the_normalized_leaf_and_limit() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with(
                "GET /providers/akshare/index-constituents/SH/000300?limit=300 HTTP/1.1\r\n"
            ),
            "request = {request}"
        );
        let body = r#"{"market":"SH","symbol":"000300","instrument_id":"SH.000300","constituents":[{"code":"600519","name":"贵州茅台","weight":null},{"code":"300750","name":"宁德时代","weight":3.21}],"source":"akshare-index-constituents"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let value = MarketIndexConstituentsReadPort::read(
        &akshare_port(format!("http://{address}")),
        " sh ",
        " 000300 ",
        300,
    )
    .expect("index constituents response");
    assert_eq!(value["market"], "SH");
    assert_eq!(value["symbol"], "000300");
    assert_eq!(value["instrumentId"], "SH.000300");
    assert_eq!(value["source"], "akshare-index-constituents");
    assert!(
        value["constituents"][0]["weight"].is_null(),
        "a missing upstream weight stays null: {value}"
    );
    assert_eq!(value["constituents"][1]["weight"], 3.21);
    server.await.expect("server");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
// Parity: go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:23 TestProviderIndexConstituentsConvertsEntriesAndAppliesDefaultLimit
async fn index_constituents_read_applies_default_limit_and_projects_weights() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with(
                "GET /providers/akshare/index-constituents/SH/000300?limit=200 HTTP/1.1\r\n"
            ),
            "request = {request}"
        );
        let body = r#"{"market":"SH","symbol":"000300","instrument_id":"SH.000300","constituents":[{"code":"600519","name":"贵州茅台","weight":null},{"code":"300750","name":"宁德时代","weight":3.21}],"source":"akshare-index-constituents"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(), body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let value = MarketIndexConstituentsReadPort::read(
        &akshare_port(format!("http://{address}")),
        "cn",
        "SH.000300",
        0,
    )
    .expect("index constituents response");
    assert_eq!(value["instrumentId"], "SH.000300");
    assert_eq!(value["source"], "akshare-index-constituents");
    assert!(value["constituents"][0]["weight"].is_null());
    assert_eq!(value["constituents"][1]["weight"], 3.21);
    server.await.expect("server");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
// Parity: go:452dea11:internal/marketdata/index_constituents_facade_test.go:73 TestServiceIndexConstituentsResolvesChinaAggregateToExchangeLeaf
async fn index_constituents_read_accepts_the_cn_aggregate_prefix() {
    for (market, symbol, limit) in [("SZ", "399001", 200), ("SH", "000300", 50)] {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with(
                &format!("GET /providers/akshare/index-constituents/{market}/{symbol}?limit={limit} HTTP/1.1\r\n")
            ),
            "request = {request}"
        );
        let body = json!({"market": market, "symbol": symbol, "instrument_id": format!("{market}.{symbol}"), "constituents": [], "source": "akshare-index-constituents"}).to_string();
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let value = MarketIndexConstituentsReadPort::read(
        &akshare_port(format!("http://{address}")),
        "cn",
        &format!("{market}.{symbol}"),
        limit,
    )
    .expect("CN aggregate resolves to the SZ leaf");
    assert_eq!(value["instrumentId"], format!("{market}.{symbol}"));
    assert!(
        value["constituents"].as_array().expect("array").is_empty(),
        "an empty member list is still a valid projection: {value}"
    );
    server.await.expect("server");
    }
}

#[test]
// Parity: go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:48 TestProviderIndexConstituentsRejectsMalformedPayloads
// Parity: go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:65 TestProviderIndexConstituentsRejectsIdentityMismatch
fn index_constituents_projection_rejects_identity_drift_and_blank_codes() {
    let drifted = json!({
        "market": "SH",
        "symbol": "000001",
        "instrument_id": "SH.000001",
        "constituents": [],
        "source": "akshare-index-constituents"
    });
    match project_constituents(&drifted, "SH", "000300") {
        Err(MarketIndexConstituentsReadError::Failed {
            status, code, message, ..
        }) => {
            assert_eq!(status, 502);
            assert_eq!(code, "BAD_GATEWAY");
            assert!(
                message.contains("SH.000300"),
                "identity drift names the requested index: {message}"
            );
        }
        other => panic!("expected the identity rejection, got {other:?}"),
    }

    let blank_code = json!({
        "market": "SH",
        "symbol": "000300",
        "instrument_id": "SH.000300",
        "constituents": [{"code": "  ", "name": "无名", "weight": null}],
        "source": "akshare-index-constituents"
    });
    match project_constituents(&blank_code, "SH", "000300") {
        Err(MarketIndexConstituentsReadError::Failed { status, code, message, .. }) => {
            assert_eq!((status, code.as_str()), (502, "BAD_GATEWAY"));
            assert!(message.contains("code"), "message = {message}");
        }
        other => panic!("expected the blank-code rejection, got {other:?}"),
    }

    let missing_members = json!({
        "market": "SH",
        "symbol": "000300",
        "instrument_id": "SH.000300",
        "constituents": null,
        "source": "akshare-index-constituents"
    });
    assert!(
        project_constituents(&missing_members, "SH", "000300").is_err(),
        "a payload without a constituents array must not project as empty"
    );
}

#[test]
// Parity: go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:65 TestProviderIndexConstituentsRejectsIdentityMismatch
fn index_constituents_projection_rejects_identity_mismatch() {
    let payload = json!({
        "market": "SH",
        "symbol": "000001",
        "instrument_id": "SH.000001",
        "constituents": [],
        "source": "akshare-index-constituents"
    });
    match project_constituents(&payload, "SH", "000300") {
        Err(MarketIndexConstituentsReadError::Failed {
            status, code, message, ..
        }) => {
            assert_eq!((status, code.as_str()), (502, "BAD_GATEWAY"));
            assert!(message.contains("SH.000300"), "message = {message}");
        }
        other => panic!("expected identity rejection, got {other:?}"),
    }
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_index_constituents_forwarding_test.go:61 TestRuntimeIndexConstituentsRejectsProvidersWithoutCapability
#[test]
// Parity: go:452dea11:internal/marketdata/index_constituents_facade_test.go:31 TestServiceIndexConstituentsRejectsProvidersWithoutCapability
// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:60 TestADKMarketIndexConstituentsToolFailsClosedWithoutPort
// Parity: go:452dea11:internal/assistant/assembly/market_index_constituents_tools_test.go:69 TestADKMarketIndexConstituentsToolSurfacesProviderCapabilityAsClearMessage
fn index_constituents_read_requires_akshare_and_a_ready_helper() {
    let unconfigured = port(None, false, None);
    match MarketIndexConstituentsReadPort::read(&unconfigured, "SH", "000300", 200) {
        Err(MarketIndexConstituentsReadError::Unavailable(message)) => {
            assert!(message.contains("provider"), "message = {message}");
        }
        other => panic!("expected the unconfigured provider failure, got {other:?}"),
    }

    for provider in [MarketDataProvider::Futu, MarketDataProvider::Yfinance] {
        let unsupported = port(Some(provider), true, None);
        match MarketIndexConstituentsReadPort::read(&unsupported, "SH", "000300", 200) {
            Err(MarketIndexConstituentsReadError::Failed {
                status, code, message, ..
            }) => {
                assert_eq!(status, 409, "{provider:?}");
                assert_eq!(code, "MARKET_DATA_CAPABILITY_UNSUPPORTED", "{provider:?}");
                let label = if provider == MarketDataProvider::Futu {
                    "futu-opend"
                } else {
                    "yahoo-finance"
                };
                assert!(
                    message.contains(label) && message.contains("index constituents"),
                    "{provider:?} message = {message}"
                );
            }
            other => panic!("expected the capability failure for {provider:?}, got {other:?}"),
        }
    }

    let not_ready = port(
        Some(MarketDataProvider::Akshare),
        false,
        Some(helper("http://127.0.0.1:9".to_owned())),
    );
    match MarketIndexConstituentsReadPort::read(&not_ready, "SH", "000300", 200) {
        Err(MarketIndexConstituentsReadError::Unavailable(message)) => {
            assert!(message.contains("not ready"), "message = {message}");
        }
        other => panic!("expected the warming helper failure, got {other:?}"),
    }
}

#[test]
// Parity: go:452dea11:internal/integration/akshare/provider_index_constituents_test.go:80 TestProviderIndexConstituentsSurfacesUnsupportedMarkets
// Parity: go:452dea11:internal/marketdata/index_constituents_facade_test.go:31 TestServiceIndexConstituentsRejectsProvidersWithoutCapability
fn index_constituents_read_rejects_non_cn_indices_without_touching_the_helper() {
    let port = port(Some(MarketDataProvider::Akshare), true, None);
    for (market, symbol) in [("US", "SPX"), ("HK", "HSI")] {
        match MarketIndexConstituentsReadPort::read(&port, market, symbol, 200) {
            Err(MarketIndexConstituentsReadError::Failed {
                status, code, message, ..
            }) => {
                assert_eq!(status, 409);
                assert_eq!(code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
                assert!(message.contains("akshare"), "message = {message}");
                assert!(message.contains(market), "message = {message}");
            }
            other => panic!("expected the non-CN capability failure, got {other:?}"),
        }
    }
    match MarketIndexConstituentsReadPort::read(&port, "CN", "000300", 200) {
        Err(MarketIndexConstituentsReadError::Failed {
            status, code, message, ..
        }) => {
            assert_eq!((status, code.as_str()), (400, "BAD_REQUEST"));
            assert!(
                message.contains("SH.<code> or SZ.<code>"),
                "message = {message}"
            );
        }
        other => panic!("expected the bare CN rejection, got {other:?}"),
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn index_constituents_read_maps_helper_capability_rejection_to_409() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let _ = stream.read(&mut request).await.expect("read");
        let body = r#"{"error":{"code":"AKSHARE_UNSUPPORTED","message":"AKShare index constituents are only available for CN indices"}}"#;
        let response = format!(
            "HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    match MarketIndexConstituentsReadPort::read(
        &akshare_port(format!("http://{address}")),
        "SH",
        "000300",
        200,
    ) {
        Err(MarketIndexConstituentsReadError::Failed {
            status, code, message, ..
        }) => {
            assert_eq!(status, 409);
            assert_eq!(code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
            assert!(message.contains("CN indices"), "message = {message}");
        }
        other => panic!("expected the helper capability failure, got {other:?}"),
    }
    server.await.expect("server");
}
