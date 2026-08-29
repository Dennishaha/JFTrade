use super::*;
use super::product_backtest_sync_request::parse_sync_request;
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{StoredBacktestCandle, StoredBacktestSyncTask};
use crate::product::product_backtest_execution::BacktestExecutionTaskRegistry;
use crate::product::{BacktestExecutionError, BacktestExecutionPort, BacktestExecutionRequest};

#[derive(Debug)]
struct FixtureExecution;

impl BacktestExecutionPort for FixtureExecution {
    fn execute(
        &self,
        request: BacktestExecutionRequest,
    ) -> Result<Value, BacktestExecutionError> {
        Ok(json!({
            "runId": request.run_id,
            "bars": request.candles.len(),
            "marketDataProvider": request.market_data_provider,
        }))
    }
}

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
    let strategy_path = directory.path().join("strategy-definitions.db");
    let connection = rusqlite::Connection::open(&strategy_path).expect("create strategy database");
    jftrade_store_sqlite::initialize_current(&connection, "strategy")
        .expect("initialize strategy database");
    drop(connection);
    let strategy_definitions = std::sync::Arc::new(
        jftrade_store_sqlite::StrategyDefinitionStore::open_existing(
            &strategy_path,
            jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .expect("open strategy store"),
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
            sync_workers: std::sync::Arc::new(BacktestSyncWorkerRegistry::default()),
            execution: None,
            execution_workers: std::sync::Arc::new(BacktestExecutionTaskRegistry::default()),
            strategy_definitions,
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

#[test]
fn production_backtest_read_routes_project_store_state() {
    let (port, _directory) = production_port();
    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-production".to_owned(),
                status: "completed".to_owned(),
                request_json: r#"{"symbol":"US.AAPL","period":"1d"}"#.to_owned(),
                result_json: r#"{"pnl":12.5,"marketDataProvider":"yfinance"}"#.to_owned(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:01:00Z".to_owned(),
            },
            "2026-08-29T00:01:00Z",
        )
        .expect("persist run");

    let listed = port.list().expect("list runs");
    assert_eq!(listed["runs"][0]["id"], "run-production");
    assert_eq!(listed["runs"][0]["marketDataProvider"], "yfinance");

    let status = port
        .status("run-production")
        .expect("status")
        .expect("run exists");
    assert_eq!(status["status"], "completed");

    let result = port
        .result("run-production")
        .expect("result")
        .expect("run exists");
    assert_eq!(result["result"]["pnl"], 12.5);

    let deleted = port
        .mutate(&BacktestsWriteInput::Delete {
            run_id: "run-production".to_owned(),
        })
        .expect("delete run");
    assert_eq!(
        deleted,
        BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted)
    );
    assert!(port.status("run-production").expect("status").is_none());
}

#[test]
fn production_backtest_read_rejects_corrupted_request_json() {
    let (port, _directory) = production_port();
    port.store
        .save_run(
            jftrade_store_sqlite::StoredBacktestRun {
                id: "run-corrupt".to_owned(),
                status: "queued".to_owned(),
                request_json: "{not-json".to_owned(),
                result_json: String::new(),
                created_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:00:00Z".to_owned(),
            },
            "2026-08-29T00:00:00Z",
        )
        .expect("persist corrupt fixture");
    let error = port.list().expect_err("corrupt request must fail closed");
    assert!(error.to_string().contains("invalid JSON"));
}

#[test]
fn production_backtest_start_without_worker_fails_before_persisting_run() {
    let (port, _directory) = production_port();
    let result = port.mutate(&BacktestsWriteInput::Start {
        payload: json!({"symbol": "US.AAPL", "period": "1d"}),
    });
    assert!(matches!(
        result,
        Err(BacktestsWritePortError::Unavailable(message))
            if message.contains("worker runtime")
    ));
    assert_eq!(port.store.run_count().expect("run count"), 0);
}

#[tokio::test]
async fn production_backtest_start_executes_fixture_and_persists_terminal_result() {
    let (mut port, _directory) = production_port();
    port._market_data_store
        .insert_candles(
            "yfinance",
            "US.AAPL",
            "1m",
            "forward",
            "regular",
            &[StoredBacktestCandle {
                start_time: 1_750_683_600_000,
                end_time: 1_750_683_659_999,
                open: "100".to_owned(),
                high: "101".to_owned(),
                low: "99".to_owned(),
                close: "100".to_owned(),
                volume: "10".to_owned(),
            }],
        )
        .expect("seed candles");
    port.execution = Some(std::sync::Arc::new(FixtureExecution));
    let response = port
        .mutate(&BacktestsWriteInput::Start {
            payload: json!({
                "definitionId": "fixture-definition",
                "strategyScript": "strategy('fixture')",
                "symbol": "US.AAPL",
                "interval": "1m",
                "startTime": "2025-06-23T13:00:00Z",
                "endTime": "2025-06-23T13:01:00Z",
                "rehabType": "forward"
            }),
        })
        .expect("start fixture backtest");
    let BacktestsWritePortResult::Data(data) = response else {
        panic!("unexpected start response");
    };
    let run_id = data["id"].as_str().expect("run id").to_owned();
    for _ in 0..50 {
        if let Some(run) = port.store.get_run(&run_id).expect("load run")
            && run.status == "completed"
        {
            let result: Value = serde_json::from_str(&run.result_json).expect("result json");
            assert_eq!(result["bars"], 1);
            assert_eq!(result["marketDataProvider"], "yfinance");
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
    }
    panic!("fixture backtest did not complete");
}

#[tokio::test]
async fn production_backtest_start_rejects_missing_history_without_queuing() {
    let (mut port, _directory) = production_port();
    port.execution = Some(std::sync::Arc::new(FixtureExecution));
    let result = port.mutate(&BacktestsWriteInput::Start {
        payload: json!({
            "definitionId": "fixture-definition",
            "strategyScript": "strategy('fixture')",
            "symbol": "US.MISSING",
            "interval": "1m",
            "startTime": "2025-06-23T13:00:00Z",
            "endTime": "2025-06-23T13:01:00Z",
            "rehabType": "forward"
        }),
    });
    assert!(matches!(
        result,
        Err(BacktestsWritePortError::Unavailable(message))
            if message.contains("K-line data")
    ));
    assert_eq!(port.store.run_count().expect("run count"), 0);
}
