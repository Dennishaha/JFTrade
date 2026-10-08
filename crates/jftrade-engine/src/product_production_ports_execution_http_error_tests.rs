//! Real HTTP error propagation through the durable execution owner.
use super::*;
use crate::product::{ProductConfig, ProductHandle, start_product};
use jftrade_integration_futu::{
    OpenDManagedSessionError, OpenDSessionCoordinatorError, ResponseError,
};

async fn http_request(
    handle: &ProductHandle,
    method: reqwest::Method,
    path: &str,
    body: &str,
) -> (u16, Value) {
    let response = reqwest::Client::builder()
        .no_proxy()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .expect("HTTP client")
        .request(
            method,
            format!("http://{}{path}", handle.startup_record().address),
        )
        .header("Content-Type", "application/json")
        .body(body.to_owned())
        .send()
        .await
        .expect("HTTP response");
    let status = response.status().as_u16();
    assert!(
        response.headers()["content-type"]
            .to_str()
            .unwrap()
            .starts_with("application/json")
    );
    let envelope: Value = response.json().await.expect("JSON envelope");
    assert!(
        envelope["timestamp"]
            .as_str()
            .is_some_and(|v| !v.is_empty())
    );
    (status, envelope)
}

#[derive(Debug, Default)]
struct ErrorTradeWriter {
    next: Mutex<Option<TradeSessionError>>,
    placed: Mutex<Vec<TradePlaceOrderRequest>>,
}

impl TradeWritePort for ErrorTradeWriter {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        self.placed.lock().unwrap().push(request);
        Err(self
            .next
            .lock()
            .unwrap()
            .take()
            .expect("configured upstream error"))
    }
    fn place_combo_order(
        &self,
        _: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        panic!("single order must not submit a combo")
    }
    fn modify_order(
        &self,
        _: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        panic!("single order must not cancel")
    }
    fn unlock_trade(&self, _: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        panic!("single order must not unlock")
    }
    fn subscribe_trade_accounts(
        &self,
        _: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        panic!("single order must not subscribe")
    }
}

// Parity: go:452dea11:internal/api/trading/execution_test.go:19 TestExecutionCommandErrorMapsRequestAndBrokerFailures
#[tokio::test]
async fn execution_http_upstream_errors_preserve_codes_and_fence_duplicate_submissions() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (store, directory) = execution_store();
    let writer = Arc::new(ErrorTradeWriter::default());
    let port = Arc::new(write_port_on_store(store.clone(), Some(writer.clone())));
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        directory.path().join("settings.json"),
    )
    .unwrap()
    .with_execution_write_port(port);
    let handle = start_product(config).await.unwrap();
    let errors = [
        (
            TradeSessionError::Response(ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 1001,
                message: "account not found".to_owned(),
            }),
            400,
            "BAD_REQUEST",
        ),
        (
            TradeSessionError::Session(OpenDManagedSessionError::RequestTimeout {
                protocol: 2202,
                serial: 7,
            }),
            504,
            "BROKER_TIMEOUT",
        ),
        (TradeSessionError::RateLimited, 429, "BROKER_RATE_LIMITED"),
        (
            TradeSessionError::Coordinator(OpenDSessionCoordinatorError::Closed),
            502,
            "BROKER_NOT_CONNECTED",
        ),
        (
            TradeSessionError::Response(ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 9001,
                message: "order rejected".to_owned(),
            }),
            502,
            "BROKER_COMMAND_FAILED",
        ),
        (
            TradeSessionError::Response(ResponseError::Decode {
                operation: "place",
                message: "plain".to_owned(),
            }),
            502,
            "BROKER_COMMAND_FAILED",
        ),
    ];
    for (index, (error, expected_status, code)) in errors.into_iter().enumerate() {
        *writer.next.lock().unwrap() = Some(error);
        let client_id = format!("http-broker-error-{index}");
        let payload = cancel_contract_payload(&client_id);
        let (status, failed) = http_request(
            &handle,
            reqwest::Method::POST,
            "/api/v1/execution/orders",
            &payload.to_string(),
        )
        .await;
        assert_eq!(status, expected_status, "{failed}");
        assert_eq!(failed["ok"], false);
        assert_eq!(failed["error"]["code"], code);
        assert!(failed.get("data").is_none_or(Value::is_null));
        let rows = store.list_orders().unwrap();
        let row = rows
            .iter()
            .find(|r| r.client_order_id.as_deref() == Some(client_id.as_str()))
            .unwrap()
            .clone();
        assert_eq!(row.status, "UNKNOWN");
        assert_eq!(row.last_error_code.as_deref(), Some(code));
        assert_eq!(row.last_error_source.as_deref(), Some("opend"));
        assert!(row.broker_order_id.is_none());
        let events = store.list_order_events(&row.internal_order_id).unwrap();
        assert_eq!(events.len(), 1);
        assert_eq!(events[0].event_type, "submission_failed");
        let (status, replayed) = http_request(
            &handle,
            reqwest::Method::POST,
            "/api/v1/execution/orders",
            &payload.to_string(),
        )
        .await;
        assert_eq!(status, 200, "{replayed}");
        assert_eq!(replayed["data"]["status"], "UNKNOWN");
        assert_eq!(replayed["data"]["internalOrderId"], row.internal_order_id);
        assert_eq!(writer.placed.lock().unwrap().len(), index + 1);
        assert_eq!(store.get_order(&row.internal_order_id).unwrap(), Some(row));
        assert_eq!(
            store
                .list_order_events(&events[0].internal_order_id)
                .unwrap(),
            events
        );
    }
    handle.shutdown().await.unwrap();
    let requests = writer.placed.lock().unwrap();
    assert_eq!(requests.len(), 6);
    for request in requests.iter() {
        assert_eq!(request.header.acc_id, 42);
        assert_eq!(request.header.trd_env, 0);
        assert_eq!(request.header.trd_market, 1);
        assert_eq!(request.code, "00700");
        assert_eq!(request.trd_side, 1);
        assert_eq!(request.order_type, 1);
        assert_eq!(request.quantity, 100.0);
        assert_eq!(request.price, Some(320.5));
    }
}

// Parity: go:452dea11:internal/api/trading/execution_test.go:47 TestHandleExecutionPlaceReturnsRiskRejectionEnvelope
// Parity: go:452dea11:internal/api/trading/execution_test.go:19 TestExecutionCommandErrorMapsRequestAndBrokerFailures
#[tokio::test]
async fn execution_http_real_risk_rejection_keeps_native_admission_and_zero_side_effects() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (store, directory) = execution_store();
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = write_port_on_store(store.clone(), Some(writer.clone()));
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        directory.path().join("real-trade-control.json"),
    )));
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        directory.path().join("settings.json"),
    )
    .unwrap()
    .with_execution_write_port(Arc::new(port));
    let handle = start_product(config).await.unwrap();
    let original = json!({"tradingEnvironment":"REAL","market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100});
    let (status, body) = http_request(
        &handle,
        reqwest::Method::POST,
        "/api/v1/execution/orders",
        &original.to_string(),
    )
    .await;
    assert_eq!(status, 400, "{body}");
    assert_eq!(body["error"]["code"], "BAD_REQUEST");
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("accountId is required")
    );
    let mut numeric = original;
    numeric["accountId"] = json!("42");
    let (status, body) = http_request(
        &handle,
        reqwest::Method::POST,
        "/api/v1/execution/orders",
        &numeric.to_string(),
    )
    .await;
    assert_eq!(status, 403, "{body}");
    assert_eq!(body["error"]["code"], "REAL_TRADING_DISABLED");
    assert_eq!(body["ok"], false);
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("real trading is disabled")
    );
    assert!(store.list_orders().unwrap().is_empty());
    assert!(writer.placed.lock().unwrap().is_empty());
    assert!(writer.modified.lock().unwrap().is_empty());
    handle.shutdown().await.unwrap();
}

// Parity: go:452dea11:internal/api/trading/routes_broker_contracts_test.go:214 TestBrokerWriteRoutesMapBodyBrokerAndOperationOutcomes
#[tokio::test]
async fn broker_http_writes_validate_original_bodies_resolve_cancel_and_forward_unlock() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (store, directory) = execution_store();
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = Arc::new(write_port_on_store(store.clone(), Some(writer.clone())));
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        directory.path().join("settings.json"),
    )
    .unwrap()
    .with_brokers_write_port(port.clone());
    let handle = start_product(config).await.unwrap();
    for (method, path, body, status) in [
        (
            reqwest::Method::POST,
            "/api/v1/brokers/futu/orders",
            r#"{"symbol":"#,
            400,
        ),
        (
            reqwest::Method::DELETE,
            "/api/v1/brokers/futu/orders",
            r#"{"orders":"#,
            400,
        ),
        (
            reqwest::Method::POST,
            "/api/v1/brokers/futu/unlock",
            r#"{"unlock":"#,
            400,
        ),
        (
            reqwest::Method::POST,
            "/api/v1/brokers/ib/orders",
            r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#,
            404,
        ),
        (
            reqwest::Method::POST,
            "/api/v1/brokers/futu/funds",
            "{}",
            404,
        ),
    ] {
        let (actual, envelope) = http_request(&handle, method, path, body).await;
        assert_eq!(actual, status, "{path}: {envelope}");
        assert_eq!(envelope["ok"], false);
    }
    // Unlike the Go callback, native cancellation requires a durable order.
    let (status, failed) = http_request(
        &handle,
        reqwest::Method::DELETE,
        "/api/v1/brokers/futu/orders",
        r#"{"orders":[{"brokerOrderId":"fail","symbol":"US.AAPL"}]}"#,
    )
    .await;
    assert_eq!(status, 404, "{failed}");
    assert_eq!(failed["error"]["code"], "EXECUTION_ORDER_NOT_FOUND");
    assert!(store.list_orders().unwrap().is_empty());
    assert!(writer.placed.lock().unwrap().is_empty());
    assert!(writer.modified.lock().unwrap().is_empty());
    assert!(writer.unlocked.lock().unwrap().is_empty());
    let placed = port
        .place_order(&cancel_contract_payload("broker-http-cancel-seven"))
        .unwrap();
    let id = placed["internalOrderId"].as_str().unwrap();
    let mut row = store.get_order(id).unwrap().unwrap();
    row.broker_order_id = Some("7".to_owned());
    row.broker_order_id_ex = None;
    row.market = "US".to_owned();
    row.symbol = Some("US.AAPL".to_owned());
    store.save_order(row, "2026-10-08T00:00:00Z").unwrap();
    let (status, cancelled) = http_request(
        &handle,
        reqwest::Method::DELETE,
        "/api/v1/brokers/futu/orders",
        r#"{"orders":[{"orderId":7,"brokerOrderId":"ok","symbol":"US.AAPL"}]}"#,
    )
    .await;
    assert_eq!(status, 200, "{cancelled}");
    assert_eq!(cancelled["ok"], true);
    assert_eq!(cancelled["data"]["cancelled"], 1);
    assert_eq!(cancelled["data"]["orders"][0]["internalOrderId"], id);
    assert_eq!(
        store.get_order(id).unwrap().unwrap().status,
        "CANCEL_SUBMITTED"
    );
    {
        let modified = writer.modified.lock().unwrap();
        assert_eq!(modified.len(), 1);
        assert_eq!(modified[0].order_id, 7);
        assert_eq!(modified[0].operation, 2);
        assert_eq!(modified[0].header.acc_id, 42);
        assert_eq!(modified[0].header.trd_market, 2);
        assert!(modified[0].order_id_ex.is_none());
    }
    let before = store.list_order_events(id).unwrap();
    let (status, unlocked) = http_request(
        &handle,
        reqwest::Method::POST,
        "/api/v1/brokers/futu/unlock",
        r#"{"unlock":true,"passwordMd5":"abc"}"#,
    )
    .await;
    assert_eq!(status, 200, "{unlocked}");
    assert_eq!(unlocked["ok"], true);
    assert_eq!(unlocked["data"]["unlocked"], true);
    assert_eq!(store.list_order_events(id).unwrap(), before);
    handle.shutdown().await.unwrap();
    let unlocks = writer.unlocked.lock().unwrap();
    assert_eq!(unlocks.len(), 1);
    assert!(unlocks[0].unlock);
    assert_eq!(unlocks[0].password_md5.as_deref(), Some("abc"));
}
