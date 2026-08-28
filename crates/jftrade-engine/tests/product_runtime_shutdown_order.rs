use jftrade_engine::product::ProductConfig;
use jftrade_engine::product_runtime::{
    ProductRuntimeConfig, ShutdownEventRecorder, start_product_runtime,
};

const TEST_DESKTOP_TOKEN: &str = "test-desktop-token-entropy-32-chars-long";

fn assert_shutdown_order(events: &[&str]) {
    let order = [
        "http_join",
        "provider",
        "opend",
        "helper_pine",
        "sqlite_lease",
    ];
    let mut last_idx = 0;
    for &event in events {
        let current_idx = order
            .iter()
            .position(|&expected| expected == event)
            .unwrap_or_else(|| panic!("unexpected shutdown event: {event}"));
        assert!(
            current_idx >= last_idx,
            "shutdown event {event} violated order in sequence: {events:?}"
        );
        last_idx = current_idx;
    }
}

#[tokio::test]
async fn test_product_runtime_ordered_shutdown_lifecycle() {
    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "futu",
        "marketData": {
            "futu": {
                "host": "127.0.0.1",
                "port": 11111,
                "autoConnect": false
            }
        }
    });
    std::fs::write(
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

    let recorder = ShutdownEventRecorder::new();
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
        shutdown_recorder: Some(recorder.clone()),
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;
    assert!(addr.port() > 0);

    // Explicit shutdown completes cleanly and records teardown events
    let result = runtime.shutdown().await;
    assert!(
        result.is_ok(),
        "runtime shutdown should succeed cleanly: {:?}",
        result
    );

    let events = recorder.events();
    assert!(!events.is_empty(), "shutdown events must be recorded");
    assert!(events.contains(&"http_join"));
    assert!(events.contains(&"sqlite_lease"));
    assert_shutdown_order(&events);
}

#[tokio::test]
async fn test_product_runtime_startup_failure_rollback() {
    let temp_dir = tempfile::tempdir().unwrap();
    let corrupted_path = temp_dir.path().join("corrupted.json");
    std::fs::write(&corrupted_path, "{ broken json").unwrap();

    let product_config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().unwrap(),
        &corrupted_path,
        TEST_DESKTOP_TOKEN,
    )
    .expect("config");

    let recorder = ShutdownEventRecorder::new();
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
        shutdown_recorder: Some(recorder.clone()),
    };

    // Startup should fail closed without panicking or leaking open resources
    let result = start_product_runtime(runtime_config).await;
    assert!(
        result.is_err(),
        "startup with corrupted settings must fail closed"
    );

    let events = recorder.events();
    assert_shutdown_order(&events);
}

#[tokio::test]
async fn test_product_runtime_drop_cleanup() {
    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "futu",
        "marketData": {
            "futu": {
                "host": "127.0.0.1",
                "port": 11111,
                "autoConnect": false
            }
        }
    });
    std::fs::write(
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

    let recorder = ShutdownEventRecorder::new();
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
        shutdown_recorder: Some(recorder.clone()),
    };

    let runtime = start_product_runtime(runtime_config)
        .await
        .expect("runtime start");
    let addr = runtime.startup_record().address;
    assert!(addr.port() > 0);

    // Drop runtime synchronously; execution_sync_drop terminates and cleans up
    drop(runtime);

    let events = recorder.events();
    assert!(!events.is_empty(), "drop events must be recorded");
    assert!(events.contains(&"http_join"));
    assert!(events.contains(&"sqlite_lease"));
    assert_shutdown_order(&events);

    // Verify resources released
    assert!(!temp_dir.path().join("main.db-journal").exists());
}

#[test]
fn test_product_runtime_tokio_runtime_exit_synchronous() {
    let temp_dir = tempfile::tempdir().unwrap();
    let settings_path = temp_dir.path().join("settings.json");
    let settings_content = serde_json::json!({
        "activeMarketDataProvider": "yfinance",
        "marketData": {}
    });
    std::fs::write(
        &settings_path,
        serde_json::to_string_pretty(&settings_content).unwrap(),
    )
    .unwrap();

    let rt = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap();

    let recorder = ShutdownEventRecorder::new();
    let recorder_clone = recorder.clone();

    let runtime = rt.block_on(async {
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
            shutdown_recorder: Some(recorder_clone),
        };

        start_product_runtime(runtime_config)
            .await
            .expect("runtime start")
    });

    // Drop rt while runtime is still alive; then drop runtime
    drop(rt);
    drop(runtime);

    let events = recorder.events();
    assert!(!events.is_empty(), "events after rt exit must be recorded");
    assert!(events.contains(&"http_join"));
    assert!(events.contains(&"sqlite_lease"));
    assert_shutdown_order(&events);
}
