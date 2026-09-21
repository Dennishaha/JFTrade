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
    BacktestSyncReadSnapshotPort, StrategyDefinitionPreview, StrategyDefinitionSnapshotError,
    StrategyDefinitionSnapshotPort,
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

/// Answers the coverage question from a switch and records every coverage
/// request, so the readiness gate can be proven to use the candidate warmup.
#[derive(Debug)]
struct CoverageSwitch {
    covered: bool,
    requests: Mutex<Vec<Value>>,
}

impl CoverageSwitch {
    fn new(covered: bool) -> Self {
        Self {
            covered,
            requests: Mutex::new(Vec::new()),
        }
    }
}

impl BacktestSyncReadSnapshotPort for CoverageSwitch {
    fn progress(&self, _task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        Ok(None)
    }

    fn active_tasks(&self) -> Result<Vec<Value>, BacktestSyncReadSnapshotError> {
        Ok(Vec::new())
    }

    fn check_coverage(
        &self,
        request: &BacktestDataCoverageRequest,
    ) -> Result<bool, BacktestSyncReadSnapshotError> {
        self.requests
            .lock()
            .expect("coverage requests")
            .push(json!({
                "provider": request.provider,
                "symbol": request.symbol,
                "interval": request.interval,
                "rehabType": request.rehab_type,
                "sessionScope": request.session_scope,
                "warmupBars": request.warmup_bars,
            }));
        Ok(self.covered)
    }
}

/// Serves the definition projections the optimization gate resolves, and
/// records the preview the gate asked for.
#[derive(Debug)]
struct StubDefinitionSnapshot {
    warmups: Mutex<Vec<(String, i64)>>,
    previews: Mutex<Vec<(String, String, String, bool)>>,
}

impl StubDefinitionSnapshot {
    fn new(warmups: &[(&str, i64)]) -> Self {
        Self {
            warmups: Mutex::new(
                warmups
                    .iter()
                    .map(|(id, warmup)| ((*id).to_owned(), *warmup))
                    .collect(),
            ),
            previews: Mutex::new(Vec::new()),
        }
    }
}

impl StrategyDefinitionSnapshotPort for StubDefinitionSnapshot {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        Ok(Vec::new())
    }

    fn get(
        &self,
        definition_id: &str,
        preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        self.previews.lock().expect("definition previews").push((
            definition_id.to_owned(),
            preview.symbol.clone().unwrap_or_default(),
            preview.interval.clone().unwrap_or_default(),
            preview.use_extended_hours,
        ));
        let warmup = self
            .warmups
            .lock()
            .expect("definition warmups")
            .iter()
            .find(|(id, _)| id == definition_id)
            .map(|(_, warmup)| *warmup);
        Ok(warmup.map(|warmup| {
            json!({
                "id": definition_id,
                "runtime": "pinescript",
                "sourceFormat": "pine-v6",
                "derivedWarmupBars": warmup,
                "derivedWarmupInterval": preview.interval.clone().unwrap_or_else(|| "1m".to_owned()),
            })
        }))
    }

    fn versions(
        &self,
        _definition_id: &str,
    ) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        Ok(None)
    }

    fn version(
        &self,
        _definition_id: &str,
        _version: &str,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        Ok(None)
    }
}

#[derive(Debug, Default)]
struct RecordingBacktestMutations {
    starts: Mutex<Vec<Value>>,
    syncs: Mutex<Vec<Value>>,
    cancels: AtomicUsize,
}

impl BacktestsWritePort for RecordingBacktestMutations {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { payload } => {
                let mut starts = self.starts.lock().expect("queued starts");
                starts.push(payload.clone());
                Ok(BacktestsWritePortResult::Data(json!({
                    "id": format!("run-{}", starts.len()),
                    "status": "queued",
                })))
            }
            BacktestsWriteInput::Sync { payload } => {
                self.syncs
                    .lock()
                    .expect("sync starts")
                    .push(payload.clone());
                Ok(BacktestsWritePortResult::Data(json!({
                    "taskId": "sync-task-optimize",
                    "intervals": payload.get("intervals").cloned().unwrap_or_else(|| json!(["1m"])),
                })))
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

fn optimize_arguments(definition_ids: &[&str]) -> Value {
    json!({
        "definitionIds": definition_ids,
        "market": "HK",
        "symbol": "HK.00700",
        "interval": "1m",
        "startTime": "2026-03-01T00:00:00Z",
        "endTime": "2026-03-02T00:00:00Z",
        "waitForCompletionMs": 0,
    })
}

/// Parity: go:452dea11:internal/assistant/assembly/adk_tool_failure_contracts_test.go:60
/// (optimization half) and `internal/backtest/service_test.go:406`
/// `TestEnsureDefinitionsDataUsesMaximumCandidateWarmup`.  Go runs
/// `EnsureBacktestData` before it queues a single candidate: a missing window
/// answers the sync/readiness payload, the sync request carries the widest
/// candidate warmup, and no run is created.
#[test]
fn strategy_optimize_stops_at_readiness_with_the_widest_candidate_warmup() {
    let (_directory, mut ports) = production_bundle();
    let coverage = Arc::new(CoverageSwitch::new(false));
    ports.backtest_sync = coverage.clone();
    ports.strategy_definition = Arc::new(StubDefinitionSnapshot::new(&[
        ("def-fast", 60),
        ("def-slow", 600),
    ]));
    let queue = Arc::new(RecordingBacktestMutations::default());
    let queue_port: Arc<dyn BacktestsWritePort> = queue.clone();
    ports.backtests_write = queue_port;
    let ports = Arc::new(ports);
    let executor = ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(&ports),
    );

    let response = executor
        .execute(
            "strategy.optimize",
            &optimize_arguments(&["def-fast", "def-slow"]),
        )
        .expect("a missing window must answer the readiness payload");

    assert_eq!(response["ok"], true, "{response}");
    assert_eq!(response["status"], "syncing_data", "{response}");
    assert_eq!(response["nextAction"], "wait_kline_sync", "{response}");
    assert_eq!(
        response["nextTool"]["name"], "backtest.kline_sync_status",
        "{response}"
    );
    assert_eq!(
        response["dataSync"]["taskId"], "sync-task-optimize",
        "{response}"
    );
    assert_eq!(response["dataSync"]["symbol"], "HK.00700", "{response}");
    assert_eq!(
        response["dataSync"]["intervals"],
        json!(["1m"]),
        "{response}"
    );

    let requests = coverage.requests.lock().expect("coverage requests");
    assert_eq!(
        requests.len(),
        1,
        "exactly one coverage question: {requests:?}"
    );
    assert_eq!(
        requests[0]["warmupBars"], 600,
        "the widest candidate warmup must drive the query window: {requests:?}"
    );
    drop(requests);

    let expected_since =
        crate::product::product_research_backtest_execution::derive_effective_since_time(
            "2026-03-01T00:00:00Z",
            "1m",
            600,
        );
    assert_eq!(response["dataSync"]["since"], expected_since, "{response}");

    let syncs = queue.syncs.lock().expect("sync starts");
    assert_eq!(syncs.len(), 1, "exactly one sync start: {syncs:?}");
    assert_eq!(syncs[0]["symbol"], "HK.00700", "{syncs:?}");
    assert_eq!(syncs[0]["intervals"], json!(["1m"]), "{syncs:?}");
    assert_eq!(
        syncs[0]["since"], expected_since,
        "the sync request must use the combined warmup window: {syncs:?}"
    );
    assert_eq!(syncs[0]["sessionScope"], "regular", "{syncs:?}");
    drop(syncs);

    assert!(
        queue.starts.lock().expect("queued starts").is_empty(),
        "an uncovered window must never queue an optimization candidate"
    );
    assert_eq!(
        queue.cancels.load(Ordering::SeqCst),
        0,
        "nothing was queued, so nothing can be rolled back"
    );
}

/// Guard for the covered case: once the shared window is covered the tool
/// queues every candidate exactly like it did before the readiness gate.
#[test]
fn strategy_optimize_queues_every_candidate_when_the_window_is_covered() {
    let (_directory, mut ports) = production_bundle();
    let coverage = Arc::new(CoverageSwitch::new(true));
    ports.backtest_sync = coverage.clone();
    ports.strategy_definition =
        Arc::new(StubDefinitionSnapshot::new(&[("def-a", 60), ("def-b", 0)]));
    let queue = Arc::new(RecordingBacktestMutations::default());
    let queue_port: Arc<dyn BacktestsWritePort> = queue.clone();
    ports.backtests_write = queue_port;
    let ports = Arc::new(ports);
    let executor = ProductionAdkToolExecutor::with_ports(
        Arc::clone(&ports.mcp_catalog),
        Arc::clone(&ports.mcp_store),
        Arc::clone(&ports),
    );

    let response = executor
        .execute(
            "strategy.optimize",
            &optimize_arguments(&["def-a", "def-b"]),
        )
        .expect("a covered window must queue the candidates");
    assert_eq!(response["status"], "queued", "{response}");
    let runs = response["runs"].as_array().expect("runs array");
    assert_eq!(runs.len(), 2, "{response}");
    assert_eq!(runs[0]["definitionId"], "def-a", "{response}");
    assert_eq!(runs[1]["definitionId"], "def-b", "{response}");
    assert_eq!(
        queue.starts.lock().expect("queued starts").len(),
        2,
        "both candidates must reach the queue"
    );
    assert!(
        queue.syncs.lock().expect("sync starts").is_empty(),
        "a covered window must not start another sync"
    );
}

/// Go's `EnsureDefinitionsData` fails the call when a candidate definition
/// cannot be resolved (`ErrStrategyDefinitionNotFound`), so the tool must not
/// queue anything for an unknown id.
#[test]
fn strategy_optimize_reports_an_unresolved_candidate_before_queueing() {
    let (_directory, mut ports) = production_bundle();
    ports.backtest_sync = Arc::new(CoverageSwitch::new(true));
    ports.strategy_definition = Arc::new(StubDefinitionSnapshot::new(&[("def-known", 0)]));
    let queue = Arc::new(RecordingBacktestMutations::default());
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
            "strategy.optimize",
            &optimize_arguments(&["def-known", "def-missing"]),
        )
        .expect_err("an unresolved candidate must fail the call");
    assert!(
        error.contains("strategy definition not found"),
        "the unresolved definition must be surfaced: {error}"
    );
    assert!(
        queue.starts.lock().expect("queued starts").is_empty(),
        "an unresolved candidate set must never queue a run"
    );
    assert!(
        queue.syncs.lock().expect("sync starts").is_empty(),
        "an unresolved candidate set must not start a sync"
    );
}
