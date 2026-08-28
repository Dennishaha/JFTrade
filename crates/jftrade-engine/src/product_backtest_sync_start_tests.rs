use super::*;

#[test]
fn sync_request_rejects_invalid_ranges_and_intervals() {
    let invalid_interval = json!({
        "market": "US",
        "code": "AAPL",
        "intervals": ["2m"],
        "since": "2026-08-01T00:00:00Z",
        "until": "2026-08-02T00:00:00Z"
    });
    assert!(matches!(
        parse_sync_request(&invalid_interval),
        Err(BacktestsWritePortError::BadRequest(_))
    ));

    let invalid_range = json!({
        "market": "US",
        "code": "AAPL",
        "since": "2026-08-02T00:00:00Z",
        "until": "2026-08-01T00:00:00Z"
    });
    assert!(matches!(
        parse_sync_request(&invalid_range),
        Err(BacktestsWritePortError::BadRequest(_))
    ));
}

#[test]
fn sync_request_defaults_match_public_contract_without_provider_success() {
    let request = parse_sync_request(&Value::Null).expect("null request uses documented defaults");
    assert_eq!(request.symbol, "HK.00700");
    assert_eq!(request.session_scope, "regular");
    assert_eq!(request.rehab_type, "forward");
    assert_eq!(request.intervals, ["1m", "5m", "15m", "30m", "1h", "1d", "1w"]);
}

fn production_port() -> (ProductionBacktestPort, tempfile::TempDir) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let runs_path = directory.path().join("backtest-runs.db");
    let connection = rusqlite::Connection::open(&runs_path).expect("create runs database");
    jftrade_store_sqlite::initialize_current(&connection, "backtest-runs")
        .expect("initialize runs database");
    drop(connection);
    let runs = std::sync::Arc::new(
        jftrade_store_sqlite::BacktestRunStore::open_existing(
            &runs_path,
            jftrade_store_sqlite::BACKTEST_RUNS_PRODUCTION_PROFILE,
        )
        .expect("open runs store"),
    );
    let sync_tasks = std::sync::Arc::new(jftrade_store_sqlite::BacktestSyncTaskStore::new(
        std::sync::Arc::clone(&runs),
    ));
    let market_data_path = directory.path().join("backtest.db");
    let connection = rusqlite::Connection::open(&market_data_path).expect("create market database");
    jftrade_store_sqlite::initialize_current(&connection, "backtest")
        .expect("initialize market database");
    drop(connection);
    let market_data = std::sync::Arc::new(
        jftrade_store_sqlite::BacktestMarketDataStore::open_existing(
            &market_data_path,
            jftrade_store_sqlite::BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
        )
        .expect("open market store"),
    );
    (
        ProductionBacktestPort {
            store: runs,
            sync_tasks,
            _market_data_store: market_data,
            helper: None,
            active_provider_state: std::sync::Arc::new(
                ActiveProviderState::new(Some(MarketDataProvider::Yfinance)),
            ),
        },
        directory,
    )
}

#[test]
fn production_sync_read_projects_persisted_task() {
    let (port, _directory) = production_port();
    port.sync_tasks
        .create(StoredBacktestSyncTask {
            task_id: "sync-production".to_owned(),
            status: "running".to_owned(),
            symbol: "US.AAPL".to_owned(),
            market_data_provider: "yfinance".to_owned(),
            total_intervals: 2,
            completed_intervals: 1,
            total_batches: 2,
            completed_batches: 1,
            current_interval: "1d".to_owned(),
            retries: 0,
            error: None,
            started_at: "2026-08-29T00:00:00Z".to_owned(),
            updated_at: "2026-08-29T00:01:00Z".to_owned(),
            revision: 0,
        })
        .expect("persist task");
    let projected = port.progress("sync-production").expect("project task").unwrap();
    assert_eq!(projected["status"], "running");
    assert_eq!(projected["completedIntervals"], 1);
    assert!(port.progress("missing").expect("missing task").is_none());
}

#[test]
fn production_sync_cancel_matches_not_found_for_terminal_task() {
    let (port, _directory) = production_port();
    port.sync_tasks
        .create(StoredBacktestSyncTask {
            task_id: "sync-terminal".to_owned(),
            status: "completed".to_owned(),
            symbol: "US.AAPL".to_owned(),
            market_data_provider: "yfinance".to_owned(),
            total_intervals: 0,
            completed_intervals: 0,
            total_batches: 0,
            completed_batches: 0,
            current_interval: String::new(),
            retries: 0,
            error: None,
            started_at: "2026-08-29T00:00:00Z".to_owned(),
            updated_at: "2026-08-29T00:00:00Z".to_owned(),
            revision: 0,
        })
        .expect("persist terminal task");
    let result = port.mutate(&BacktestsWriteInput::CancelSync {
        task_id: "sync-terminal".to_owned(),
    });
    assert_eq!(result, Ok(BacktestsWritePortResult::SyncCancelled(false)));
}
