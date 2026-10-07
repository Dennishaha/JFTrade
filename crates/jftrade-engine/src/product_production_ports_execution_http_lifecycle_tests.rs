use super::*;
use crate::product::{ProductConfig, ProductHandle, start_product};

async fn request(
    handle: &ProductHandle,
    method: reqwest::Method,
    path: &str,
    payload: Option<&Value>,
) -> (u16, Value) {
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .unwrap();
    let mut request = client.request(
        method,
        format!("http://{}{path}", handle.startup_record().address),
    );
    if let Some(payload) = payload {
        request = request.json(payload);
    }
    let response = request.send().await.unwrap();
    let status = response.status().as_u16();
    (status, response.json().await.unwrap())
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/exec_routes_test.go:18 TestExecutionOrderRoutesPlaceListEventsAndCancel
#[tokio::test]
async fn execution_http_place_list_events_cancel_preserve_rust_admission_and_durable_fence() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (store, directory) = execution_store();
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = Arc::new(write_port_on_store(store.clone(), Some(writer.clone())));
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        directory.path().join("settings.json"),
    )
    .unwrap()
    .with_execution_read_snapshot_port(port.clone())
    .with_execution_write_port(port);
    let handle = start_product(config).await.unwrap();
    // Preserve the original input to expose the account admission difference.
    let original = json!({"market":"HK", "symbol":"00700", "side":"BUY", "orderType":"LIMIT",
        "timeInForce":"DAY", "quantity":100, "price":320.5, "env":"SIMULATE"});
    let (status, rejected) = request(
        &handle,
        reqwest::Method::POST,
        "/api/v1/execution/orders",
        Some(&original),
    )
    .await;
    assert_eq!(status, 400, "{rejected}");
    assert_eq!(rejected["ok"], false);
    assert_eq!(rejected["error"]["code"], "BAD_REQUEST");
    assert!(
        rejected["error"]["message"]
            .as_str()
            .unwrap()
            .contains("accountId is required")
    );
    assert!(writer.placed.lock().unwrap().is_empty());
    assert!(store.list_orders().unwrap().is_empty());

    let (status, placed) = request(
        &handle,
        reqwest::Method::POST,
        "/api/v1/execution/orders",
        Some(&cancel_contract_payload("http-lifecycle")),
    )
    .await;
    assert_eq!(status, 200, "{placed}");
    assert_eq!(placed["ok"], true);
    let id = placed["data"]["internalOrderId"]
        .as_str()
        .unwrap()
        .to_owned();
    assert!(!id.is_empty());
    assert_eq!(placed["data"]["brokerOrderId"], "9001");
    let (status, listed) = request(
        &handle,
        reqwest::Method::GET,
        "/api/v1/execution/orders",
        None,
    )
    .await;
    assert_eq!(status, 200, "{listed}");
    assert_eq!(listed["data"]["orders"].as_array().unwrap().len(), 1);
    assert_eq!(listed["data"]["orders"][0]["internalOrderId"], id);
    assert_eq!(listed["data"]["orders"][0]["symbol"], "HK.00700");
    assert_eq!(listed["data"]["orders"][0]["status"], "SUBMITTED");
    let event_path = format!("/api/v1/execution/orders/{id}/events");
    let (status, before) = request(&handle, reqwest::Method::GET, &event_path, None).await;
    assert_eq!(status, 200, "{before}");
    assert_eq!(before["data"]["internalOrderId"], id);
    let types = |body: &Value| {
        body["data"]["events"]
            .as_array()
            .unwrap()
            .iter()
            .map(|event| event["eventType"].as_str().unwrap().to_owned())
            .collect::<Vec<_>>()
    };
    // Reservation is durable but has no public PREPARED event in Rust.
    assert_eq!(types(&before), ["submitted"]);
    let (status, cancelled) = request(
        &handle,
        reqwest::Method::POST,
        &format!("/api/v1/execution/orders/{id}/cancel"),
        Some(&json!({})),
    )
    .await;
    assert_eq!(status, 200, "{cancelled}");
    assert_eq!(cancelled["data"]["status"], "CANCEL_SUBMITTED");
    let (status, after) = request(&handle, reqwest::Method::GET, &event_path, None).await;
    assert_eq!(status, 200, "{after}");
    handle.shutdown().await.unwrap();
    assert_eq!(types(&after), ["submitted", "cancel_submitted"]);
    assert_eq!(
        store.get_order(&id).unwrap().unwrap().status,
        "CANCEL_SUBMITTED"
    );
    let placed = writer.placed.lock().unwrap();
    assert_eq!(placed.len(), 1);
    assert_eq!(placed[0].header.acc_id, 42);
    let modified = writer.modified.lock().unwrap();
    assert_eq!(modified.len(), 1);
    assert_eq!(modified[0].order_id, 9001);
    assert_eq!(modified[0].operation, 2);
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:202 TestExecutionGatewayCancelOrderBoundaries
#[test]
fn execution_cancel_invalid_numeric_identity_and_absent_symbol_keep_rust_boundaries() {
    let (store, _directory) = execution_store();
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = write_port_on_store(store.clone(), Some(writer.clone()));
    let placed = port
        .place_order(&cancel_contract_payload("cancel-identity-boundaries"))
        .unwrap();
    let id = placed["internalOrderId"].as_str().unwrap();
    let mut order = store.get_order(id).unwrap().unwrap();
    order.broker_order_id = Some("not-a-number".to_owned());
    order.broker_order_id_ex = None;
    order = store.save_order(order, "2026-10-08T00:00:00Z").unwrap();
    let events_before = store.list_order_events(id).unwrap();
    let error = port.cancel_order(id).unwrap_err();
    assert!(
        matches!(error, ExecutionWritePortError::Failed { status:400, ref code, .. } if code == "BROKER_ORDER_ID_MISSING"),
        "{error:?}"
    );
    assert!(writer.modified.lock().unwrap().is_empty());
    assert_eq!(store.get_order(id).unwrap().unwrap(), order);
    assert_eq!(store.list_order_events(id).unwrap(), events_before);
    // OpenD cancellation identifies an order by id/header; symbol is unused.
    order.broker_order_id = Some("9001".to_owned());
    order.symbol = None;
    store.save_order(order, "2026-10-08T00:00:01Z").unwrap();
    let cancelled = port.cancel_order(id).unwrap();
    assert_eq!(cancelled["internalOrderId"], id);
    assert_eq!(cancelled["status"], "CANCEL_SUBMITTED");
    let modified = writer.modified.lock().unwrap();
    assert_eq!(modified.len(), 1);
    assert_eq!(modified[0].order_id, 9001);
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:298 TestExecutionGatewayCancelComboBoundaries
#[test]
fn execution_combo_cancel_keeps_missing_identity_gateway_and_extended_id_boundaries() {
    let (store, _directory) = execution_store();
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = write_port_on_store(store.clone(), Some(writer.clone()));
    let error = port.cancel_order("missing-combo").unwrap_err();
    assert!(
        matches!(error, ExecutionWritePortError::Failed { status:404, ref code, .. } if code == "EXECUTION_ORDER_NOT_FOUND"),
        "{error:?}"
    );
    let placed = port
        .place_order(&cancel_contract_payload("combo-cancel-boundaries"))
        .unwrap();
    let id = placed["internalOrderId"].as_str().unwrap();
    let mut combo = store.get_order(id).unwrap().unwrap();
    combo.order_kind = "option_combo".to_owned();
    combo.product_class = "option".to_owned();
    combo.market = "US".to_owned();
    combo.normalized_request = option_combo_fixture_payload().to_string();
    combo.broker_order_id = None;
    combo.broker_order_id_ex = None;
    store
        .save_order(combo.clone(), "2026-10-08T00:00:00Z")
        .unwrap();
    let error = port.cancel_order(id).unwrap_err();
    assert!(
        matches!(error, ExecutionWritePortError::Failed { status:400, ref code, .. } if code == "BROKER_ORDER_ID_MISSING"),
        "{error:?}"
    );
    combo.broker_order_id_ex = Some("C-1".to_owned());
    combo = store.save_order(combo, "2026-10-08T00:00:01Z").unwrap();
    let gatewayless = write_port_on_store(store.clone(), None);
    let events_before = store.list_order_events(id).unwrap();
    assert!(matches!(
        gatewayless.cancel_order(id),
        Err(ExecutionWritePortError::Unavailable(_))
    ));
    assert_eq!(store.get_order(id).unwrap().unwrap(), combo);
    assert_eq!(store.list_order_events(id).unwrap(), events_before);
    assert!(writer.modified.lock().unwrap().is_empty());
    let cancelled = port.cancel_order(id).unwrap();
    assert_eq!(cancelled["internalOrderId"], id);
    assert_eq!(cancelled["status"], "CANCEL_SUBMITTED");
    let modified = writer.modified.lock().unwrap();
    assert_eq!(modified.len(), 1);
    assert_eq!(modified[0].order_id, 0);
    assert_eq!(modified[0].order_id_ex.as_deref(), Some("C-1"));
}

#[test]
fn malformed_persisted_execution_orders_reject_before_cancel_fence_or_broker_call() {
    for invalid_combo in [false, true] {
        let (store, _directory) = execution_store();
        let writer = Arc::new(RecordingTradeWriter::default());
        let port = write_port_on_store(store.clone(), Some(writer.clone()));
        let placed = port
            .place_order(&cancel_contract_payload("malformed-cancel"))
            .unwrap();
        let id = placed["internalOrderId"].as_str().unwrap();
        let mut order = store.get_order(id).unwrap().unwrap();
        let (status, code) = if invalid_combo {
            order.order_kind = "option_combo".to_owned();
            (500, "EXECUTION_ORDER_DATA_INVALID")
        } else {
            order.account_id = "not-a-number".to_owned();
            (400, "BAD_REQUEST")
        };
        let before = store.save_order(order, "2026-10-08T00:00:00Z").unwrap();
        let events = store.list_order_events(id).unwrap();
        let error = port.cancel_order(id).unwrap_err();
        assert!(
            matches!(error, ExecutionWritePortError::Failed { status: actual, code: ref actual_code, .. }
            if actual == status && actual_code == code),
            "{error:?}"
        );
        assert!(
            writer.modified.lock().unwrap().is_empty(),
            "invalid order must not reach OpenD"
        );
        assert_eq!(
            store.get_order(id).unwrap().unwrap(),
            before,
            "invalid order must retain durable state"
        );
        assert_eq!(store.list_order_events(id).unwrap(), events);
    }
}
