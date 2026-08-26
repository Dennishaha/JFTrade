use std::collections::VecDeque;
use std::sync::Arc;
use std::sync::Mutex;

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_market_data_subscription_mutation_port::test_cutover::MarketDataSubscriptionMutationSqliteTestCutoverPort;
use super::super::product_market_data_subscription_mutation_port::{
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
};
use super::*;

#[derive(Debug)]
struct FixtureMarketDataSubscriptionMutationPort;

impl MarketDataSubscriptionMutationPort for FixtureMarketDataSubscriptionMutationPort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        Ok(json!({
            "accepted": true,
            "method": request.method,
            "path": request.path,
        }))
    }
}

#[derive(Debug)]
struct SequencedMarketDataSubscriptionMutationPort {
    responses: Mutex<VecDeque<Result<Value, MarketDataSubscriptionMutationPortError>>>,
    calls: Mutex<Vec<MarketDataSubscriptionMutationRequest>>,
}

impl SequencedMarketDataSubscriptionMutationPort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, MarketDataSubscriptionMutationPortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<MarketDataSubscriptionMutationRequest> {
        self.calls
            .lock()
            .expect("market-data subscription call lock")
            .clone()
    }
}

impl MarketDataSubscriptionMutationPort for SequencedMarketDataSubscriptionMutationPort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        self.calls
            .lock()
            .expect("market-data subscription call lock")
            .push(request.clone());
        self.responses
            .lock()
            .expect("market-data subscription response lock")
            .pop_front()
            .expect("market-data subscription rehearsal response")
    }
}

#[tokio::test]
async fn market_data_subscription_mutations_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_subscription_mutation_port(Arc::new(
                FixtureMarketDataSubscriptionMutationPort,
            ));
    let handle = start_product(config)
        .await
        .expect("start market-data subscription product");
    assert_eq!(handle.startup_record().owned_routes, 54);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/market-data/subscriptions" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(
        response["data"]["path"],
        "/api/v1/market-data/subscriptions"
    );
    handle
        .shutdown()
        .await
        .expect("shutdown market-data subscription product");
}

#[tokio::test]
async fn market_data_subscription_mutations_replay_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(
        &settings_path,
        b"{\"seed\":\"market-data-subscriptions\"}\n",
    )
    .expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let port = Arc::new(SequencedMarketDataSubscriptionMutationPort::new([
        Err(MarketDataSubscriptionMutationPortError::Unavailable(
            "fixture provider unavailable".to_owned(),
        )),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Err(MarketDataSubscriptionMutationPortError::Failed {
            status: 502,
            code: "BROKER_FEATURE_FAILED".to_owned(),
            message: "fixture provider failed".to_owned(),
            retry_after_seconds: None,
        }),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Ok(json!({"accepted": true, "source": "rust-product"})),
        Ok(json!({"accepted": true, "source": "rust-product"})),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_subscription_mutation_port(port.clone());
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
        ("X-Request-ID", "market-data-subscription-fixture"),
    ];

    let unauthorized = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);

    let unavailable = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(
        unavailable.1["error"]["code"],
        "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE"
    );

    let success = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart","instruments":[]}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(success.0, 200);
    assert_eq!(success.1["data"]["source"], "rust-product");

    let failed = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
        Some(r#"{"dataTypes":["ORDER_BOOK"]}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(failed.0, 502);
    assert_eq!(failed.1["error"]["code"], "BROKER_FEATURE_FAILED");

    for (method, path, body) in [
        (
            "POST",
            "/api/v1/market-data/subscriptions/release",
            r#"{"consumerId":"chart","instruments":[]}"#,
        ),
        (
            "POST",
            "/api/v1/market-data/subscriptions/heartbeat",
            r#"{"consumerId":"chart"}"#,
        ),
        (
            "DELETE",
            "/api/v1/market-data/subscriptions?consumerId=chart",
            "",
        ),
        (
            "DELETE",
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/lease-1",
            "",
        ),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, Some(body), &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(
            response["data"]["source"], "rust-product",
            "{method} {path}"
        );
    }

    let calls = port.calls();
    assert_eq!(calls.len(), 7);
    assert_eq!(calls[0].path, "/api/v1/market-data/subscriptions");
    assert_eq!(calls[1].path, "/api/v1/market-data/subscriptions");
    assert_eq!(
        calls[2].path,
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions"
    );
    assert_eq!(calls[3].path, "/api/v1/market-data/subscriptions/release");
    assert_eq!(calls[4].path, "/api/v1/market-data/subscriptions/heartbeat");
    assert_eq!(calls[5].query, "consumerId=chart");
    assert_eq!(
        calls[6].path,
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/lease-1"
    );
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_port = Arc::new(FixtureMarketDataSubscriptionMutationPort);
    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_market_data_subscription_mutation_port(restarted_port);
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "DELETE",
        "/api/v1/market-data/subscriptions?consumerId=chart",
        None,
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

#[tokio::test]
async fn market_data_subscription_mutations_sqlite_test_cutover_replays_transport_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("market-data-subscriptions.db");
    std::fs::write(&settings_path, b"{\"seed\":\"subscription-durable\"}\n")
        .expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("settings before replay");
    let port = Arc::new(
        MarketDataSubscriptionMutationSqliteTestCutoverPort::open(&database_path)
            .expect("open durable adapter"),
    );
    let acquire_body = br#"{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}"#;
    let acquire = mutation_request(
        "POST",
        "/api/v1/market-data/subscriptions",
        "",
        acquire_body,
    );
    port.reject_next_event().expect("install event rejection");
    assert!(matches!(
        port.dispatch(&acquire),
        Err(MarketDataSubscriptionMutationPortError::Failed { .. })
    ));
    assert_eq!(port.active_subscription_count().expect("rollback count"), 0);
    assert_eq!(port.event_count("acquire").expect("rollback events"), 0);

    let acquired = port.dispatch(&acquire).expect("acquire subscription");
    assert_eq!(acquired["source"], "sqlite-test");
    assert_eq!(port.active_subscription_count().expect("active count"), 1);
    let heartbeat = mutation_request(
        "POST",
        "/api/v1/market-data/subscriptions/heartbeat",
        "",
        br#"{"consumerId":"chart"}"#,
    );
    port.dispatch(&heartbeat).expect("heartbeat");

    let prediction_acquire = mutation_request(
        "POST",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
        "",
        br#"{"dataTypes":["ORDER_BOOK"]}"#,
    );
    let lease = port
        .dispatch(&prediction_acquire)
        .expect("prediction acquire");
    assert_eq!(lease["leaseId"], "lease-test-1");
    let prediction_release = mutation_request(
        "DELETE",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/lease-test-1",
        "",
        b"",
    );
    let attempts = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let request = prediction_release.clone();
            std::thread::spawn(move || port.dispatch(&request))
        })
        .collect::<Vec<_>>();
    let mut transitioned = 0;
    let mut fenced = 0;
    for attempt in attempts {
        let response = attempt
            .join()
            .expect("join lease release")
            .expect("release");
        if response["transitioned"] == true {
            transitioned += 1;
        } else {
            fenced += 1;
        }
    }
    assert_eq!((transitioned, fenced), (1, 1));
    assert_eq!(
        port.lease_status("lease-test-1").expect("lease status"),
        Some("released".to_owned())
    );

    let release = mutation_request(
        "POST",
        "/api/v1/market-data/subscriptions/release",
        "",
        br#"{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}"#,
    );
    port.dispatch(&release).expect("release subscription");
    assert_eq!(port.active_subscription_count().expect("released count"), 0);

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_subscription_mutation_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let response = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(std::str::from_utf8(acquire_body).expect("acquire body")),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["source"], "sqlite-test");
    let response = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions/heartbeat",
        Some(r#"{"consumerId":"chart"}"#),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    let response = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/subscriptions/release",
        Some(r#"{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}"#),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    let response = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/market-data/subscriptions?consumerId=chart",
        None,
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    let response = request_json_with_status(
        address,
        "POST",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
        Some(r#"{"dataTypes":["ORDER_BOOK"]}"#),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["leaseId"], "lease-test-2");
    let response = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/lease-test-2",
        None,
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after replay"),
        settings_before
    );
    drop(port);

    let reopened = Arc::new(
        MarketDataSubscriptionMutationSqliteTestCutoverPort::open(&database_path)
            .expect("reopen durable adapter"),
    );
    assert_eq!(
        reopened
            .lease_status("lease-test-2")
            .expect("reopened lease"),
        Some("released".to_owned())
    );
    assert_eq!(
        reopened
            .active_subscription_count()
            .expect("reopened count"),
        0
    );
    let restarted = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().expect("restart address"),
            &settings_path,
        )
        .expect("restart config")
        .with_market_data_subscription_mutation_port(reopened.clone()),
    )
    .await
    .expect("start restarted product");
    let response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(std::str::from_utf8(acquire_body).expect("restart acquire body")),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(
        reopened
            .active_subscription_count()
            .expect("post-restart count"),
        1
    );
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after restart"),
        settings_before
    );
}

fn mutation_request(
    method: &str,
    path: &str,
    query: &str,
    body: &[u8],
) -> MarketDataSubscriptionMutationRequest {
    MarketDataSubscriptionMutationRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: query.to_owned(),
        body: body.to_vec(),
    }
}
