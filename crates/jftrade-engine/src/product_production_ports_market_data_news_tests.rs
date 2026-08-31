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

fn port(base_url: String) -> ProductionMarketDataNewsPort {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: Some(helper(base_url)),
        trade_runtime: None,
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_news_actions_port_forwards_yfinance_news_request() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(request.starts_with("GET /providers/yfinance/news/US/AAPL?limit=5 HTTP/1.1\r\n"));
        let body = r#"{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","entries":[{"title":"Headline","published_at":"2026-08-15T14:30:00Z"}],"source":"yfinance-news"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let value = MarketDataNewsActionsReadSnapshotPort::read(
        &port(format!("http://{address}")),
        "/api/v1/market-data/news/US/AAPL",
        "limit=5",
    )
    .expect("news response");
    assert_eq!(value["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["publishedAt"], "2026-08-15T14:30:00Z");
    server.await.expect("server");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_news_actions_port_forwards_corporate_actions_window() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(request.starts_with(
            "GET /providers/yfinance/corporate-actions/SH/600519?from=2026-01-01T00%3A00%3A00Z&to=2026-01-31T00%3A00%3A00Z HTTP/1.1\r\n"
        ));
        let body = r#"{"market":"SH","symbol":"600519","instrument_id":"SH.600519","events":[{"kind":"dividend","ex_date":"2026-01-10","amount":1.2,"ratio":null}],"source":"yfinance-actions"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let value = MarketDataNewsActionsReadSnapshotPort::read(
        &port(format!("http://{address}")),
        "/api/v1/market-data/corporate-actions/SH/600519",
        "from=2026-01-01T00:00:00Z&to=2026-01-31T00:00:00Z",
    )
    .expect("corporate actions response");
    assert_eq!(value["events"][0]["kind"], "dividend");
    server.await.expect("server");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn production_news_actions_port_maps_helper_failure_and_rejects_bad_limit() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = [0_u8; 4096];
        let _ = stream.read(&mut request).await.expect("read");
        let body = r#"{"error":{"code":"upstream_error","message":"Yahoo unavailable"}}"#;
        let response = format!(
            "HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        stream.write_all(response.as_bytes()).await.expect("write");
    });
    let result = MarketDataNewsActionsReadSnapshotPort::read(
        &port(format!("http://{address}")),
        "/api/v1/market-data/news/US/AAPL",
        "limit=0",
    )
    .expect_err("invalid limit");
    assert!(matches!(
        result,
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 400,
            ref code,
            ..
        } if code == "BAD_REQUEST"
    ));
    let result = MarketDataNewsActionsReadSnapshotPort::read(
        &port(format!("http://{address}")),
        "/api/v1/market-data/news/US/AAPL",
        "limit=5",
    )
    .expect_err("helper failure");
    assert!(matches!(
        result,
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            ref code,
            ..
        } if code == "upstream_error"
    ));
    server.await.expect("server");
}

#[test]
fn corporate_actions_query_requires_rfc3339_and_ascending_range() {
    let error = news_actions_helper_request(
        "/api/v1/market-data/corporate-actions/US/AAPL",
        "from=not-a-time",
    )
    .expect_err("invalid from");
    assert!(matches!(
        error,
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 400,
            ref message,
            ..
        } if message == "from must be a valid timestamp"
    ));

    let error = news_actions_helper_request(
        "/api/v1/market-data/corporate-actions/US/AAPL",
        "from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z",
    )
    .expect_err("descending range");
    assert!(matches!(
        error,
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 400,
            ref message,
            ..
        } if message == "from must not be after to"
    ));
}
