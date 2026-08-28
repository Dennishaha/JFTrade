use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsFuture, MarketDataProviderActionsPort,
    MarketDataProviderActionsPortError, MarketDataProviderActionsRequest,
    NORMALIZE_INSTRUMENT_PATH, PREDICTION_COMBO_QUOTES_PATH, ZERO_DTE_CONTRACTS_PATH,
};
use super::*;

#[derive(Debug)]
struct FixtureMarketDataProviderActionsPort {
    source: &'static str,
}

impl MarketDataProviderActionsPort for FixtureMarketDataProviderActionsPort {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a> {
        let res = Ok(json!({
            "accepted": true,
            "source": self.source,
            "path": request.path,
        }));
        Box::pin(std::future::ready(res))
    }
}

#[derive(Debug)]
struct SequencedMarketDataProviderActionsPort {
    responses: Mutex<VecDeque<Result<Value, MarketDataProviderActionsPortError>>>,
    calls: Mutex<Vec<MarketDataProviderActionsRequest>>,
}

impl SequencedMarketDataProviderActionsPort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, MarketDataProviderActionsPortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<MarketDataProviderActionsRequest> {
        self.calls
            .lock()
            .expect("market-data provider actions call lock")
            .clone()
    }
}

impl MarketDataProviderActionsPort for SequencedMarketDataProviderActionsPort {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a> {
        self.calls
            .lock()
            .expect("market-data provider actions call lock")
            .push(request.clone());
        let res = self
            .responses
            .lock()
            .expect("market-data provider actions response lock")
            .pop_front()
            .expect("market-data provider actions rehearsal response");
        Box::pin(std::future::ready(res))
    }
}

#[tokio::test]
async fn market_data_provider_actions_replay_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"provider-actions\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let port = Arc::new(SequencedMarketDataProviderActionsPort::new([
        Err(MarketDataProviderActionsPortError::Unavailable(
            "fixture provider unavailable".to_owned(),
        )),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Err(MarketDataProviderActionsPortError::Failed {
            status: 429,
            code: "PROVIDER_RATE_LIMITED".to_owned(),
            message: "retry later".to_owned(),
            retry_after_seconds: Some(7),
        }),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Ok(json!({"accepted": true, "source": "rust-product"})),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_provider_actions_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/market-data"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "provider-actions-fixture"),
    ];

    let unauthorized = request_json_with_status(
        address,
        "POST",
        NORMALIZE_INSTRUMENT_PATH,
        Some(r#"{"symbol":"AAPL"}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        NORMALIZE_INSTRUMENT_PATH,
        Some(r#"{"symbol":"AAPL"}"#),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/market-data"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);

    let unavailable = request_json_with_status(
        address,
        "POST",
        NORMALIZE_INSTRUMENT_PATH,
        Some(r#"{"symbol":"AAPL"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(
        unavailable.1["error"]["code"],
        "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE"
    );

    let normalized = request_json_with_status(
        address,
        "POST",
        NORMALIZE_INSTRUMENT_PATH,
        Some(r#"{"symbol":"AAPL"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(normalized.0, 200);
    assert_eq!(normalized.1["data"]["source"], "rust-product");

    let (rate_limited_status, rate_limited_headers, rate_limited) = request_json_with_headers(
        address,
        "POST",
        "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&accountId=eligible",
        Some(r#"{"operation":"chain"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(rate_limited_status, 429);
    assert_eq!(rate_limited["error"]["code"], "PROVIDER_RATE_LIMITED");
    assert_eq!(
        rate_limited_headers.get("retry-after"),
        Some(&"7".to_owned())
    );

    for (path, body) in [
        (
            ZERO_DTE_CONTRACTS_PATH,
            r#"{"market":"US","underlying":"AAPL","expiry":"2026-08-25"}"#,
        ),
        (
            PREDICTION_COMBO_QUOTES_PATH,
            r#"{"legs":[{"side":"BUY","quantity":1}]}"#,
        ),
        (BATCH_SNAPSHOTS_PATH, r#"{"symbols":["AAPL"]}"#),
    ] {
        let (status, response) =
            request_json_with_status(address, "POST", path, Some(body), &browser_headers).await;
        assert_eq!(status, 200, "POST {path}");
        assert_eq!(response["data"]["source"], "rust-product", "POST {path}");
    }

    let calls = port.calls();
    assert_eq!(calls.len(), 6);
    assert_eq!(calls[0].path, NORMALIZE_INSTRUMENT_PATH);
    assert_eq!(calls[1].path, NORMALIZE_INSTRUMENT_PATH);
    assert_eq!(
        calls[2].path,
        "/api/v1/market-data/options/analysis/US.AAPL"
    );
    assert_eq!(calls[2].query, "brokerId=api-test&accountId=eligible");
    assert_eq!(calls[3].path, ZERO_DTE_CONTRACTS_PATH);
    assert_eq!(calls[4].path, PREDICTION_COMBO_QUOTES_PATH);
    assert_eq!(calls[5].path, BATCH_SNAPSHOTS_PATH);
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_market_data_provider_actions_port(Arc::new(FixtureMarketDataProviderActionsPort {
        source: "rust-restarted",
    }));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        BATCH_SNAPSHOTS_PATH,
        Some(r#"{"symbols":["AAPL"]}"#),
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["source"], "rust-restarted");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

async fn request_json_with_headers(
    address: std::net::SocketAddr,
    method: &str,
    path: &str,
    body: Option<&str>,
    headers: &[(&str, &str)],
) -> (u16, BTreeMap<String, String>, Value) {
    let body = body.unwrap_or_default();
    let mut stream = tokio::net::TcpStream::connect(address)
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
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut response = Vec::new();
    stream
        .read_to_end(&mut response)
        .await
        .expect("read response");
    let response = String::from_utf8(response).expect("UTF-8 response");
    let (header_text, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let status = header_text
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    let headers = header_text
        .lines()
        .skip(1)
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.to_ascii_lowercase(), value.trim().to_owned()))
        .collect();
    (
        status,
        headers,
        serde_json::from_str(body).expect("JSON response"),
    )
}
