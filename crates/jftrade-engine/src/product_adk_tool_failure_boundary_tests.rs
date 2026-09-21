//! Tool-boundary failure contracts for the Go
//! `internal/assistant/assembly/adk_tool_failure_contracts_test.go` row.
//!
//! Go registers the market, watchlist, research and optimization handlers with
//! injected dependencies and pins three contracts: a malformed request must be
//! rejected before the dependency runs, a research backtest must stop on a
//! data-readiness or queue error instead of starting a run, and a closed ADK
//! store must surface through the task and memory tools. Rust has no
//! model-tool registry, so the equivalent owners are the production MCP
//! executor over the production quote port and the ADK tool executor over the
//! same production port bundle. The closed-store half is pinned by
//! `product_production_ports_adk_tests::
//! adk_routes_surface_durable_store_failures_instead_of_empty_success`.
//!
//! Registered differences against the reference row:
//! - Go answers `market.candles` with `{}` with "market and symbol are
//!   required"; Rust's instrument parser reports the first missing field
//!   ("market is required") and keeps "market and symbol are required" for an
//!   empty half of `instrumentId`. Both fail closed before any provider read.
//! - Go's dependency-free watchlist tool reports "unavailable"; Rust's
//!   port-less executor reports `MCP_PRODUCTION_EXECUTOR_UNAVAILABLE`
//!   ("production MCP ports are not configured").
//! - Go's `strategy.optimize` runs `EnsureBacktestData` before queueing
//!   candidates. Rust has no definitions-data readiness owner yet, so the
//!   optimization path queues without that gate (registered P1 gap).

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};

use jftrade_integration_futu::{
    CurrentKlineError, CurrentKlineQuery, CurrentKlineResult, HistoricalKlineError,
    HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult,
};

use crate::product::product_adk_model_runtime::{AdkToolExecutor, ProductionAdkToolExecutor};
use crate::product::product_backtests_write_port::{
    BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError, BacktestsWritePortResult,
};
use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::{
    ProductionMarketDataQuotePort, SharedTradeReadRuntime,
};
use crate::product::{
    ActiveProviderState, BacktestDataCoverageRequest, BacktestSyncReadSnapshotError,
    BacktestSyncReadSnapshotPort,
};

use super::tests::production_bundle;

/// Records every provider history read so a rejected tool call can be proven
/// to stop before the market-data dependency runs.
#[derive(Debug, Default)]
struct RecordingHistoryRead {
    requests: Mutex<Vec<HistoricalKlineQuery>>,
    current_calls: AtomicUsize,
}

impl HistoricalKlineReadPort for RecordingHistoryRead {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        self.requests
            .lock()
            .expect("history reads")
            .push(query.clone());
        Err(HistoricalKlineError::Rejected {
            ret_type: -1,
            err_code: 0,
            message: "history must not be read".to_owned(),
        })
    }

    fn query_current(
        &self,
        _: &CurrentKlineQuery,
    ) -> Result<CurrentKlineResult, CurrentKlineError> {
        self.current_calls.fetch_add(1, Ordering::SeqCst);
        Err(CurrentKlineError::Rejected {
            ret_type: -1,
            err_code: 0,
            message: "current candles must not be read".to_owned(),
        })
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_tool_failure_contracts_test.go:22
/// "market and watchlist handlers reject malformed requests before any remote
/// call" (market half). Go rejects `market.candles` with `{}` and with an
/// unsupported period without ever invoking the injected candle dependency.
/// Rust validates the instrument in the MCP executor and the period in the
/// production quote-read owner, so the provider history port keeps zero
/// requests for both calls.
#[test]
fn market_candles_stop_missing_instrument_and_unsupported_period_before_provider_reads() {
    let (_directory, mut ports) = production_bundle();
    let history = Arc::new(RecordingHistoryRead::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let history_port: Arc<dyn HistoricalKlineReadPort> = history.clone();
    runtime.set_historical_klines(Some(history_port));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ports.market_data_quote = Arc::new(
        ProductionMarketDataQuotePort::new(state, None, None, None)
            .with_trade_runtime(Some(runtime)),
    );
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::new(ports));

    let missing = executor
        .execute_production("market.candles", &json!({}))
        .expect_err("a candle request without an instrument must fail closed");
    assert_eq!(missing.status, 400, "{missing:?}");
    assert_eq!(missing.code, "BAD_REQUEST", "{missing:?}");
    assert!(
        missing.message.contains("required"),
        "the missing instrument must be named: {missing:?}"
    );

    let unsupported = executor
        .execute_production(
            "market.candles",
            &json!({"market": "HK", "symbol": "00700", "period": "not-a-period"}),
        )
        .expect_err("an unsupported candle period must fail closed");
    assert_eq!(unsupported.status, 400, "{unsupported:?}");
    assert_eq!(unsupported.code, "BAD_REQUEST", "{unsupported:?}");

    assert!(
        history.requests.lock().expect("history reads").is_empty(),
        "the rejected candle inputs must never reach the provider history port"
    );
    assert_eq!(
        history.current_calls.load(Ordering::SeqCst),
        0,
        "the rejected candle inputs must never read the current bucket"
    );
}

/// Reports every coverage question as satisfied so the research tool reaches
/// the queue step, and records each queued payload the owner tried to start.
#[derive(Debug)]
struct ReadyCoverage;

impl BacktestSyncReadSnapshotPort for ReadyCoverage {
    fn progress(&self, _task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        Ok(None)
    }

    fn active_tasks(&self) -> Result<Vec<Value>, BacktestSyncReadSnapshotError> {
        Ok(Vec::new())
    }

    fn check_coverage(
        &self,
        _request: &BacktestDataCoverageRequest,
    ) -> Result<bool, BacktestSyncReadSnapshotError> {
        Ok(true)
    }
}

#[derive(Debug, Default)]
struct FailingBacktestQueue {
    starts: Mutex<Vec<Value>>,
    cancels: AtomicUsize,
}

impl BacktestsWritePort for FailingBacktestQueue {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { payload } => {
                self.starts
                    .lock()
                    .expect("queued starts")
                    .push(payload.clone());
                Err(BacktestsWritePortError::Failed(
                    "research queue unavailable".to_owned(),
                ))
            }
            BacktestsWriteInput::Cancel { .. } => {
                self.cancels.fetch_add(1, Ordering::SeqCst);
                Ok(BacktestsWritePortResult::Data(json!({"cancelled": true})))
            }
            other => {
                let operation = other.operation();
                Err(BacktestsWritePortError::Failed(format!(
                    "unexpected backtest mutation {operation:?}"
                )))
            }
        }
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_tool_failure_contracts_test.go:60
/// "research and optimization stop on data-readiness or queue errors" (queue
/// half). Go surfaces a failing `StartResearchBacktest` dependency as the tool
/// error instead of answering a run summary. Rust answers the same way from
/// `start_research_backtest_run`, so the failing queue owner must see exactly
/// one start attempt and no rollback.
#[test]
fn research_backtest_surfaces_queue_failure_without_answering_a_run() {
    let (_directory, mut ports) = production_bundle();
    ports.backtest_sync = Arc::new(ReadyCoverage);
    let queue = Arc::new(FailingBacktestQueue::default());
    let queue_port: Arc<dyn BacktestsWritePort> = queue.clone();
    ports.backtests_write = queue_port;
    let ports = Arc::new(ports);
    let executor = ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(&ports),
    );

    let error = executor
        .execute(
            "strategy.research_backtest",
            &json!({
                "script": "//@version=6\nstrategy(\"Queue failure\", overlay=true)\nplot(close)\n",
                "market": "HK",
                "symbol": "HK.00700",
                "startTime": "2026-01-01T00:00:00Z",
                "endTime": "2026-01-02T00:00:00Z",
                "waitForCompletionMs": 0,
            }),
        )
        .expect_err("a failed queue must never answer a run summary");
    assert!(
        error.contains("failed to start research backtest"),
        "the queue failure must be surfaced: {error}"
    );
    assert!(
        error.contains("research queue unavailable"),
        "the owning failure message must survive: {error}"
    );
    assert_eq!(
        queue.starts.lock().expect("queued starts").len(),
        1,
        "the research tool must attempt the queue exactly once"
    );
    assert_eq!(
        queue.cancels.load(Ordering::SeqCst),
        0,
        "a queue that never accepted the run has nothing to roll back"
    );
}
