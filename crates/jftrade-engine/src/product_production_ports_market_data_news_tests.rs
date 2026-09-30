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
// Parity: go:452dea11:internal/api/marketdata/routes_news_actions_test.go:76 TestNewsRouteValidatesLimitAndForwardsToService
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

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_news_forwarding_test.go:52 TestRuntimeForwardsNewsAndCorporateActionsToCapableActiveProvider
// Parity: go:452dea11:internal/api/marketdata/routes_news_actions_test.go:115 TestCorporateActionsRouteValidatesRangeAndForwardsToService
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
// Parity: go:452dea11:internal/api/marketdata/routes_news_actions_test.go:76 TestNewsRouteValidatesLimitAndForwardsToService
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
    for invalid_limit in ["0", "abc", "51"] {
        let result = MarketDataNewsActionsReadSnapshotPort::read(
            &port(format!("http://{address}")),
            "/api/v1/market-data/news/US/AAPL",
            &format!("limit={invalid_limit}"),
        )
        .expect_err("invalid limit");
        assert!(
            matches!(
                result,
                MarketDataNewsActionsReadSnapshotError::Failed {
                    status: 400,
                    ref code,
                    ..
                } if code == "BAD_REQUEST"
            ),
            "limit {invalid_limit}"
        );
    }
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

/// Parity: go:452dea11:internal/productfeatures/provider_facade_interception_test.go:180
/// TestEmbeddedProviderServesNewsForExplicitBrokerID
///
/// An explicit `brokerId` that names the active embedded provider is served by
/// the helper instead of falling through to the broker registry: the request
/// reaches `/providers/yfinance/news/US/AAPL` with the resolved `limit`, and
/// the helper payload is projected for the console.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn news_route_serves_an_explicit_broker_id_for_the_active_provider() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with("GET /providers/yfinance/news/US/AAPL?limit=5 HTTP/1.1\r\n"),
            "request = {request}"
        );
        let body = r#"{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","entries":[{"title":"results","published_at":"2026-08-15T21:30:00Z"}],"source":"yfinance-news"}"#;
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
        "brokerId=yfinance&limit=5",
    )
    .expect("news for the active provider's explicit id");
    assert_eq!(value["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["title"], "results");
    assert_eq!(value["entries"][0]["publishedAt"], "2026-08-15T21:30:00Z");
    server.await.expect("server");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_interception_test.go:213
/// TestEmbeddedProviderServesCorporateActionsForActiveProvider
///
/// Corporate actions are served by the active embedded provider with no broker
/// hop. Go's reader derives the default window (`now.AddDate(-2, 0, 0)` .. now),
/// which the Python sidecar owns here (`action_window`: last two years), so the
/// Rust contract is "forward no bounds" and let the sidecar default; an explicit
/// window is forwarded verbatim.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn corporate_actions_route_uses_the_sidecar_default_window() {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
    let address = listener.local_addr().expect("address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept");
        let mut request = vec![0_u8; 4096];
        let read = stream.read(&mut request).await.expect("read");
        let request = String::from_utf8_lossy(&request[..read]);
        assert!(
            request.starts_with("GET /providers/yfinance/corporate-actions/SH/600519 HTTP/1.1\r\n"),
            "request = {request}"
        );
        assert!(
            !request.contains("from=") && !request.contains("to="),
            "the sidecar owns the default two-year window: {request}"
        );
        let body = r#"{"market":"SH","symbol":"600519","instrument_id":"SH.600519","events":[{"kind":"dividend","ex_date":"2026-06-30","amount":1.2}],"source":"yfinance-actions"}"#;
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
        "",
    )
    .expect("corporate actions with the sidecar window");
    assert_eq!(value["instrumentId"], "SH.600519");
    assert_eq!(value["events"][0]["kind"], "dividend");
    server.await.expect("server");
}

/// Parity: go:452dea11:internal/productfeatures/provider_facade_interception_test.go:248
/// TestEmbeddedProviderLeavesFutuQueriesOnBrokerPath
///
/// News queries never enter the embedded facade when they belong to the broker
/// path: an explicit `brokerId=futu` while yfinance is active, and Futu being
/// the active provider with no explicit id. Both fail closed on the broker side
/// (capability / OpenD readiness) instead of borrowing yfinance's helper.
#[test]
fn futu_news_queries_stay_on_the_broker_path() {
    // yfinance active, explicit futu: the active-provider guard rejects before
    // any helper read even though the helper is "ready".
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };
    let error = MarketDataNewsActionsReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/news/US/AAPL",
        "brokerId=futu&limit=5",
    )
    .expect_err("explicit futu must not be intercepted by the embedded facade");
    assert!(
        matches!(
            error,
            MarketDataNewsActionsReadSnapshotError::Failed { status: 409, ref code, .. }
                if code == "MARKET_DATA_CAPABILITY_UNSUPPORTED"
        ),
        "error = {error:?}"
    );

    // Futu active with no explicit id: the broker path owns the read and fails
    // closed on OpenD readiness, so no helper is consulted.
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };
    let error = MarketDataNewsActionsReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/news/US/AAPL",
        "limit=5",
    )
    .expect_err("futu-active news must stay on the OpenD path");
    assert!(
        matches!(
            error,
            MarketDataNewsActionsReadSnapshotError::Unavailable(ref message)
                if message.contains("Futu OpenD")
        ),
        "error = {error:?}"
    );
}

#[test]
// Parity: go:452dea11:internal/integration/yfinance/provider_news_actions_test.go:67 TestProviderCorporateActionsSortsEventsByExDateAndKind
fn corporate_actions_projection_sorts_events_by_ex_date_and_kind() {
    let payload = serde_json::json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "source": "yfinance-actions",
        "events": [
            {"kind": "split", "ex_date": "2026-05-11", "amount": null, "ratio": 2},
            {"kind": "dividend", "ex_date": "2026-05-11", "amount": 1.2, "ratio": null},
            {"kind": "dividend", "ex_date": "2026-08-10", "amount": 0.5, "ratio": null}
        ]
    });
    let value = super::product_production_ports_market_data_news_actions::validate_news_actions_payload(
        payload,
        "corporate-actions",
        "US",
        "AAPL",
    )
    .expect("corporate actions projection");
    assert_eq!(
        value["events"],
        serde_json::json!([
            {"kind": "dividend", "exDate": "2026-05-11", "amount": 1.2, "ratio": null},
            {"kind": "split", "exDate": "2026-05-11", "amount": null, "ratio": 2},
            {"kind": "dividend", "exDate": "2026-08-10", "amount": 0.5, "ratio": null}
        ])
    );
}

#[test]
fn corporate_actions_projection_rejects_missing_events() {
    let payload = serde_json::json!({
        "market": "US",
        "symbol": "AAPL",
        "instrument_id": "US.AAPL",
        "source": "yfinance-actions"
    });
    let error = super::product_production_ports_market_data_news_actions::validate_news_actions_payload(
        payload,
        "corporate-actions",
        "US",
        "AAPL",
    )
    .expect_err("missing events must not project as an empty success");
    assert!(matches!(
        error,
        MarketDataNewsActionsReadSnapshotError::Failed {
            status: 502,
            ref code,
            ref message,
            ..
        } if code == "BAD_GATEWAY" && message == "market-data helper response is missing events"
    ));
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_news_forwarding_test.go:89 TestRuntimeNewsAndCorporateActionsRejectProvidersWithoutCapability
/// Parity: go:452dea11:internal/productfeatures/service_test.go:30
/// TestQueryDoesNotFallbackWhenBrokerIsExplicit
///
/// Go resolves the request against the explicit broker and returns
/// `ErrCapabilityUnavailable` when that broker cannot serve the feature; it
/// never falls back to another registered broker. The Rust owner is the
/// production port's active-provider guard: an explicit `brokerId` that names a
/// different provider must answer 409 before any helper/OpenD read, which is
/// the fail-closed equivalent of "no fallback".
///
/// Two Go transports render that sentinel: the path-style market-data routes
// Parity: go:452dea11:internal/api/marketdata/routes_test.go:147 TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback
/// (`/api/v1/market-data/news/{market}/{symbol}`) use
/// `MARKET_DATA_CAPABILITY_UNSUPPORTED`, while the product-feature routes use
/// `BROKER_CAPABILITY_UNAVAILABLE`. This port serves the path-style family, so
/// it must report the market-data code; the product-feature code is asserted by
/// `product_production_ports_market_data_news_search_tests.rs`. Only the code
/// differs — the no-fallback decision and the 409 status are shared.
#[test]
fn explicit_broker_that_is_not_the_active_provider_is_rejected_without_fallback() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    state.set_readiness(true, false, false);
    let port = ProductionMarketDataNewsPort {
        active_provider_state: state,
        helper: None,
        trade_runtime: None,
    };
    for requested in ["akshare", "futu"] {
        let error = MarketDataNewsActionsReadSnapshotPort::read(
            &port,
            "/api/v1/market-data/news/US/AAPL",
            &format!("brokerId={requested}&limit=5"),
        )
        .expect_err("a non-active explicit broker must not fall back");
        match error {
            MarketDataNewsActionsReadSnapshotError::Failed {
                status,
                code,
                message,
                ..
            } => {
                assert_eq!(status, 409);
                assert_eq!(code, "MARKET_DATA_CAPABILITY_UNSUPPORTED");
                assert!(
                    message.contains("does not match active provider"),
                    "message = {message}"
                );
            }
            other => panic!("expected the capability rejection, got {other:?}"),
        }
    }

    // The provider's own id is accepted and reaches the readiness check, which
    // proves the guard keys on the provider identity rather than rejecting any
    // explicit broker.
    let error = MarketDataNewsActionsReadSnapshotPort::read(
        &port,
        "/api/v1/market-data/news/US/AAPL",
        "brokerId=yfinance&limit=5",
    )
    .expect_err("helper is unavailable in this fixture");
    assert!(matches!(
        error,
        MarketDataNewsActionsReadSnapshotError::Unavailable(message)
            if message == "market-data helper is not configured"
    ));
}

#[test]
// Parity: go:452dea11:internal/api/marketdata/routes_news_actions_test.go:115 TestCorporateActionsRouteValidatesRangeAndForwardsToService
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

// Parity: go:452dea11:internal/api/marketdata/routes_news_actions_test.go:55 TestNewsAndCorporateActionsRoutesRequireInstrumentURI
// The reference handlers bind the {market}/{symbol} URI exactly and answer 400
// BAD_REQUEST when the route hand-off carries no instrument; the Rust owner
// validates the same path shape before any helper call. A request that cannot
// match the template route at all is a transport-level 404 instead.
#[test]
fn news_actions_helper_request_rejects_a_missing_instrument_uri() {
    for (path, expected_message) in [
        ("/api/v1/market-data/news", "unsupported news/actions path"),
        ("/api/v1/market-data/news/", "invalid instrument"),
        ("/api/v1/market-data/news/US", "invalid instrument"),
        (
            "/api/v1/market-data/corporate-actions/",
            "invalid instrument",
        ),
        (
            "/api/v1/market-data/corporate-actions/US/AAPL/extra",
            "invalid instrument",
        ),
    ] {
        let error = news_actions_helper_request(path, "").expect_err("missing instrument URI");
        assert!(
            matches!(
                error,
                MarketDataNewsActionsReadSnapshotError::Failed {
                    status: 400,
                    ref code,
                    ref message,
                    ..
                } if code == "BAD_REQUEST" && message == expected_message
            ),
            "path {path} -> expected {expected_message}"
        );
    }
}
