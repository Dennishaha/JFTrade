use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_brokers_write_port::test_cutover::BrokersWriteSqliteTestCutoverPort;
use super::super::product_brokers_write_port::{
    BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation, BrokersWritePort,
    BrokersWritePortError, BrokersWriteQuery,
};
use super::*;

#[derive(Debug)]
struct FixtureBrokersWritePort;

impl BrokersWritePort for FixtureBrokersWritePort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
            "brokerId": input.query.broker_id,
        }))
    }
}

#[derive(Debug)]
struct SequencedBrokersWritePort {
    responses: Mutex<VecDeque<Result<Value, BrokersWritePortError>>>,
    inputs: Mutex<Vec<BrokersWriteInput>>,
}

impl SequencedBrokersWritePort {
    fn new(responses: impl IntoIterator<Item = Result<Value, BrokersWritePortError>>) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            inputs: Mutex::new(Vec::new()),
        }
    }
}

impl BrokersWritePort for SequencedBrokersWritePort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        self.inputs
            .lock()
            .expect("brokers write product inputs lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("brokers write product responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(BrokersWritePortError::Unavailable(
                    "fixture brokers writer response missing".to_owned(),
                ))
            })
    }
}

fn browser_access_policy() -> AccessPolicy {
    AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()])
}

#[tokio::test]
async fn brokers_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_brokers_write_port(Arc::new(FixtureBrokersWritePort));
    let handle = start_product(config)
        .await
        .expect("start brokers write product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/brokers/{brokerId}/orders" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "place-order");
    handle
        .shutdown()
        .await
        .expect("shutdown brokers write product");
}

#[tokio::test]
async fn brokers_write_product_requires_private_bearer_and_internal_protocol() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_brokers_write_port(Arc::new(FixtureBrokersWritePort));
    config.access = AccessPolicy {
        desktop_token: Some("fixture-private-token".to_owned()),
        internal_proxy_protocol: Some("jftrade-product-rehearsal.v1".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    };
    let handle = start_product(config).await.expect("start private product");
    assert_eq!(handle.startup_record().owned_routes, 51);

    let unauthorized = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(r#"{"symbol":"US.AAPL"}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    assert_eq!(
        unauthorized.1["error"]["code"],
        "INTERNAL_PROXY_AUTH_REQUIRED"
    );

    let response = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders?accountId=acct-1&market=US",
        Some(r#"{"symbol":"US.AAPL"}"#),
        &[
            ("Authorization", "Bearer fixture-private-token"),
            ("x-jftrade-internal-proxy", "jftrade-product-rehearsal.v1"),
            ("x-jftrade-access-surface", "web"),
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/brokers"),
            ("X-CSRF-Token", "fixture-csrf"),
        ],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["accepted"], true);
    assert_eq!(response.1["data"]["operation"], "place-order");
    handle.shutdown().await.expect("shutdown private product");
}

#[tokio::test]
async fn brokers_write_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"brokers-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let port = Arc::new(SequencedBrokersWritePort::new([
        Err(BrokersWritePortError::Unavailable(
            "fixture broker writer unavailable".to_owned(),
        )),
        Ok(json!({"accepted": true, "operation": "place-order"})),
        Ok(json!({"accepted": true, "operation": "place-order"})),
        Err(BrokersWritePortError::Failed {
            status: 502,
            code: "PLACE_ORDER_FAILED".to_owned(),
            message: "fixture broker place failed".to_owned(),
        }),
        Ok(json!({"accepted": true, "operation": "place-order"})),
        Ok(json!({"cancelled": 1})),
        Err(BrokersWritePortError::Failed {
            status: 502,
            code: "CANCEL_FAILED".to_owned(),
            message: "context deadline exceeded".to_owned(),
        }),
        Ok(json!({"cancelled": 1})),
        Ok(json!({"unlocked": true})),
        Err(BrokersWritePortError::Failed {
            status: 502,
            code: "UNLOCK_FAILED".to_owned(),
            message: "context canceled".to_owned(),
        }),
        Ok(json!({"unlocked": true})),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_brokers_write_port(port.clone());
    config.access = browser_access_policy();
    let handle = start_product(config).await.expect("start brokers product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/brokers"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "brokers-write-fixture"),
    ];
    let place_path =
        "/api/v1/brokers/futu/orders?tradingEnvironment=REAL&accountId=acct-1&market=US";
    let place_body = r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#;
    let cancel_path =
        "/api/v1/brokers/futu/orders?tradingEnvironment=REAL&accountId=acct-1&market=US";
    let cancel_body = r#"{"orders":[{"orderId":7,"brokerOrderId":"broker-7","symbol":"US.AAPL"}]}"#;
    let unlock_path =
        "/api/v1/brokers/futu/unlock?tradingEnvironment=REAL&accountId=acct-1&market=US";
    let unlock_body = r#"{"unlock":true,"passwordMd5":"fixture"}"#;

    let unauthorized =
        request_json_with_status(address, "POST", place_path, Some(place_body), &[]).await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        place_path,
        Some(place_body),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/brokers"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);
    assert_eq!(csrf_missing.1["error"]["code"], "CSRF_FAILED");

    let unavailable = request_json_with_status(
        address,
        "POST",
        place_path,
        Some(place_body),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(unavailable.1["error"]["code"], "BROKERS_WRITE_UNAVAILABLE");
    for _ in 0..2 {
        let (status, response) = request_json_with_status(
            address,
            "POST",
            place_path,
            Some(place_body),
            &browser_headers,
        )
        .await;
        assert_eq!(status, 200);
        assert_eq!(response["data"]["operation"], "place-order");
    }
    let failed = request_json_with_status(
        address,
        "POST",
        place_path,
        Some(place_body),
        &browser_headers,
    )
    .await;
    assert_eq!(failed.0, 502);
    assert_eq!(failed.1["error"]["code"], "PLACE_ORDER_FAILED");
    let recovered = request_json_with_status(
        address,
        "POST",
        place_path,
        Some(place_body),
        &browser_headers,
    )
    .await;
    assert_eq!(recovered.0, 200);

    let (status, response) = request_json_with_status(
        address,
        "DELETE",
        cancel_path,
        Some(cancel_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["cancelled"], 1);
    let (status, response) = request_json_with_status(
        address,
        "DELETE",
        cancel_path,
        Some(cancel_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(response["error"]["code"], "CANCEL_FAILED");
    let (status, response) = request_json_with_status(
        address,
        "DELETE",
        cancel_path,
        Some(cancel_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["cancelled"], 1);

    let (status, response) = request_json_with_status(
        address,
        "POST",
        unlock_path,
        Some(unlock_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["unlocked"], true);
    let (status, response) = request_json_with_status(
        address,
        "POST",
        unlock_path,
        Some(unlock_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(response["error"]["code"], "UNLOCK_FAILED");
    let (status, response) = request_json_with_status(
        address,
        "POST",
        unlock_path,
        Some(unlock_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["unlocked"], true);

    let inputs = port
        .inputs
        .lock()
        .expect("brokers write product inputs")
        .clone();
    assert_eq!(inputs.len(), 11);
    assert_eq!(
        inputs
            .iter()
            .filter(|input| input.operation.name() == "place-order")
            .count(),
        5
    );
    assert_eq!(
        inputs
            .iter()
            .filter(|input| input.operation.name() == "cancel-orders")
            .count(),
        3
    );
    assert_eq!(
        inputs
            .iter()
            .filter(|input| input.operation.name() == "unlock")
            .count(),
        3
    );
    assert!(inputs.iter().all(|input| {
        input.context == super::super::product_brokers_write_port::BrokersWriteContext::Normal
    }));
    handle.shutdown().await.expect("shutdown brokers product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings"),
        settings_before
    );

    let mut restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_brokers_write_port(Arc::new(FixtureBrokersWritePort));
    restarted_config.access = browser_access_policy();
    let restarted = start_product(restarted_config)
        .await
        .expect("restart brokers product");
    assert_eq!(restarted.startup_record().owned_routes, 51);
    let (status, response) = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        unlock_path,
        Some(unlock_body),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["accepted"], true);
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read restarted settings"),
        settings_before
    );
}

#[tokio::test]
async fn brokers_write_sqlite_test_cutover_replays_transport_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("brokers-test-cutover.db");
    std::fs::write(&settings_path, b"{\"seed\":\"brokers-durable\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("settings before replay");
    let port = Arc::new(
        BrokersWriteSqliteTestCutoverPort::open(&database_path).expect("open durable adapter"),
    );
    let place = brokers_durable_input(
        BrokersWriteOperation::PlaceOrder,
        json!({"symbol":"US.AAPL","side":"BUY","quantity":1}),
        BrokersWriteContext::Normal,
    );
    port.reject_next_event().expect("install event rejection");
    assert!(matches!(
        port.mutate(&place),
        Err(BrokersWritePortError::Failed { .. })
    ));
    assert_eq!(port.order_count().expect("orders after rollback"), 0);
    assert_eq!(
        port.event_count("place-order")
            .expect("events after rollback"),
        0
    );

    let first = port.mutate(&place).expect("first place");
    let second = port.mutate(&place).expect("duplicate place");
    assert_eq!(first["orderId"], 1);
    assert_eq!(second["orderId"], 2);

    let cancel = brokers_durable_input(
        BrokersWriteOperation::CancelOrders,
        json!({"orders":[{"orderId":1}]}),
        BrokersWriteContext::Normal,
    );
    let attempts = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let cancel = cancel.clone();
            std::thread::spawn(move || port.mutate(&cancel))
        })
        .collect::<Vec<_>>();
    let mut transitioned = 0;
    let mut fenced = 0;
    for attempt in attempts {
        let response = attempt.join().expect("join cancel").expect("cancel result");
        if response["cancelled"] == 1 {
            transitioned += 1;
        } else {
            fenced += 1;
        }
    }
    assert_eq!((transitioned, fenced), (1, 1));
    assert_eq!(
        port.order_status(1).expect("cancelled order status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(port.event_count("cancel-orders").expect("cancel events"), 1);

    let unlock = brokers_durable_input(
        BrokersWriteOperation::Unlock,
        json!({"unlock":false}),
        BrokersWriteContext::Normal,
    );
    assert_eq!(port.mutate(&unlock).expect("unlock")["unlocked"], true);
    assert_eq!(
        port.session_unlocked("fixture-broker")
            .expect("session state"),
        Some(true)
    );

    let canceled = brokers_durable_input(
        BrokersWriteOperation::PlaceOrder,
        json!({"symbol":"US.AAPL"}),
        BrokersWriteContext::Canceled,
    );
    assert!(matches!(
        port.mutate(&canceled),
        Err(BrokersWritePortError::Failed { status: 499, .. })
    ));
    assert_eq!(port.order_count().expect("orders after cancellation"), 2);

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_brokers_write_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let place_body = r#"{"symbol":"US.AAPL","side":"BUY","quantity":1}"#;
    let response = request_json_with_status(
        address,
        "POST",
        "/api/v1/brokers/futu/orders?accountId=acct-1&market=US",
        Some(place_body),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["orderId"], 3);
    let cancel_body = r#"{"orders":[{"orderId":3}]}"#;
    let response = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/brokers/futu/orders?accountId=acct-1&market=US",
        Some(cancel_body),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["cancelled"], 1);
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after replay"),
        settings_before
    );
    drop(port);

    let reopened = Arc::new(
        BrokersWriteSqliteTestCutoverPort::open(&database_path).expect("reopen durable adapter"),
    );
    assert_eq!(
        reopened.order_status(3).expect("reopened order status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(
        reopened
            .session_unlocked("fixture-broker")
            .expect("reopened session"),
        Some(true)
    );
    let restarted = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().expect("restart address"),
            &settings_path,
        )
        .expect("restart config")
        .with_brokers_write_port(reopened.clone()),
    )
    .await
    .expect("start restarted product");
    let response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(place_body),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["orderId"], 4);
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after restart"),
        settings_before
    );
}

fn brokers_durable_input(
    operation: BrokersWriteOperation,
    payload: Value,
    context: BrokersWriteContext,
) -> BrokersWriteInput {
    BrokersWriteInput {
        operation,
        query: BrokersWriteQuery {
            broker_id: "fixture-broker".to_owned(),
            account_id: "acct-1".to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            market: "US".to_owned(),
        },
        payload,
        context,
    }
}
