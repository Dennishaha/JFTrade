//! ADK preparation and queueing share one provider snapshot per tool operation.
use super::tests::production_bundle;
use crate::product::product_adk_model_runtime::{AdkToolExecutor, ProductionAdkToolExecutor};
use crate::product::product_backtests_write_port::{
    BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError, BacktestsWritePortResult,
};
use crate::product::{
    BacktestDataCoverageRequest, BacktestSyncReadSnapshotError, BacktestSyncReadSnapshotPort,
    StrategyDefinitionPreview, StrategyDefinitionSnapshotError, StrategyDefinitionSnapshotPort,
};
use serde_json::{Value, json};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Condvar, Mutex, mpsc};
use std::time::Duration;

// Frozen Go pinespec.Skeleton(), used unchanged by the reference tools.
const SCRIPT: &str = r#"//@version=6
strategy("Minimal Draft", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

log.info("ready")"#;

#[derive(Debug)]
struct ProviderState {
    current: Mutex<Option<String>>,
    default_reads: AtomicUsize,
}

impl ProviderState {
    fn new(provider: Option<&str>) -> Self {
        Self {
            current: Mutex::new(provider.map(str::to_owned)),
            default_reads: AtomicUsize::new(0),
        }
    }
}

#[derive(Debug)]
struct Queue {
    state: Arc<ProviderState>,
    starts: Mutex<Vec<(Value, Option<String>)>>,
    change_after_start: bool,
    entered: Option<mpsc::Sender<String>>,
    release: Arc<(Mutex<bool>, Condvar)>,
}

impl Queue {
    fn new(state: Arc<ProviderState>) -> Self {
        Self {
            state,
            starts: Mutex::new(Vec::new()),
            change_after_start: false,
            entered: None,
            release: Arc::new((Mutex::new(true), Condvar::new())),
        }
    }
}

impl BacktestsWritePort for Queue {
    fn default_market_data_provider(&self) -> Option<String> {
        self.state.default_reads.fetch_add(1, Ordering::SeqCst);
        self.state.current.lock().expect("default").clone()
    }

    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let BacktestsWriteInput::Start { payload } = input else {
            return Err(BacktestsWritePortError::Failed(
                "unexpected mutation".into(),
            ));
        };
        let provider = payload
            .get("marketDataProvider")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_ascii_lowercase)
            .or_else(|| self.state.current.lock().expect("default").clone());
        self.starts
            .lock()
            .expect("starts")
            .push((payload.clone(), provider.clone()));
        if self.change_after_start {
            *self.state.current.lock().expect("default") = Some("yfinance".into());
        }
        if let Some(entered) = &self.entered {
            entered
                .send(provider.clone().expect("explicit provider"))
                .expect("notify entry");
            let (released, signal) = &*self.release;
            let guard = released.lock().expect("release");
            let (guard, timed) = signal
                .wait_timeout_while(guard, Duration::from_secs(5), |done| !*done)
                .expect("wait");
            assert!(*guard && !timed.timed_out(), "bounded queue release");
        }
        let suffix = payload
            .get("definitionId")
            .and_then(Value::as_str)
            .unwrap_or_else(|| provider.as_deref().unwrap_or("unset"));
        Ok(BacktestsWritePortResult::Data(
            json!({"id":format!("run-{suffix}"),"status":"queued"}),
        ))
    }
}

#[derive(Debug)]
struct Coverage {
    state: Arc<ProviderState>,
    requests: Mutex<Vec<BacktestDataCoverageRequest>>,
    change_default: bool,
}

impl Coverage {
    fn new(state: Arc<ProviderState>, change_default: bool) -> Self {
        Self {
            state,
            requests: Mutex::new(Vec::new()),
            change_default,
        }
    }
}

impl BacktestSyncReadSnapshotPort for Coverage {
    fn progress(&self, _: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
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
            .expect("coverage")
            .push(request.clone());
        if self.change_default {
            *self.state.current.lock().expect("default") = Some("akshare".into());
        }
        Ok(true)
    }
}

#[derive(Debug, Default)]
struct Definitions {
    requests: Mutex<Vec<String>>,
}
impl StrategyDefinitionSnapshotPort for Definitions {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        Ok(Vec::new())
    }
    fn get(
        &self,
        id: &str,
        _: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        self.requests.lock().expect("definitions").push(id.into());
        Ok(Some(json!({"id":id,"derivedWarmupBars":0})))
    }
    fn versions(&self, _: &str) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        Ok(None)
    }
    fn version(&self, _: &str, _: &str) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        Ok(None)
    }
}

fn executor(
    ports: Arc<crate::product::product_production_ports::ProductionPortBundle>,
) -> ProductionAdkToolExecutor {
    ProductionAdkToolExecutor::with_ports(ports.mcp_catalog.clone(), ports.mcp_store.clone(), ports)
}

fn research_arguments() -> Value {
    json!({"script":SCRIPT,"market":"US","symbol":"US.AAPL"})
}
fn optimize_arguments() -> Value {
    json!({"definitionIds":["def-a","def-b"],"market":"US","symbol":"US.AAPL"})
}

// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:699 TestADKBacktestProviderFreezesDefaultAcrossPreparationAndQueue
#[test]
fn provider_freeze_without_an_owner_default_preserves_an_unset_request() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(None));
    ports.backtests_write = Arc::new(Queue::new(state.clone()));
    for arguments in [json!({}), json!({"marketDataProvider":""})] {
        assert_eq!(
            crate::product::product_research_backtest_execution::frozen_market_data_provider(
                &ports, &arguments,
            ),
            None,
        );
        assert_eq!(
            crate::product::product_research_backtest_execution::freeze_backtest_arguments(
                &ports, &arguments,
            ),
            arguments,
        );
    }
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 4);
    assert_eq!(state.current.lock().expect("default").as_deref(), None);
}

// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:699 TestADKBacktestProviderFreezesDefaultAcrossPreparationAndQueue
#[test]
fn research_default_provider_is_frozen_before_preparation_changes_the_queue_default() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(Some("yfinance")));
    let queue = Arc::new(Queue::new(state.clone()));
    let coverage = Arc::new(Coverage::new(state.clone(), true));
    ports.backtests_write = queue.clone();
    ports.backtest_sync = coverage.clone();
    let result = executor(Arc::new(ports))
        .execute("strategy.research_backtest", &research_arguments())
        .expect("research");
    let starts = queue.starts.lock().expect("starts");
    assert_eq!(
        coverage.requests.lock().expect("coverage")[0].provider,
        "yfinance"
    );
    assert_eq!(starts.len(), 1);
    assert_eq!(starts[0].0["marketDataProvider"], "yfinance");
    assert_eq!(starts[0].1.as_deref(), Some("yfinance"));
    assert_eq!(result["marketDataProvider"], "yfinance");
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 1);
    assert_eq!(
        state.current.lock().expect("default").as_deref(),
        Some("akshare")
    );
}

// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:699 TestADKBacktestProviderFreezesDefaultAcrossPreparationAndQueue
#[test]
fn optimization_default_provider_is_shared_by_preparation_and_every_candidate_queue_entry() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(Some("futu")));
    let mut queue = Queue::new(state.clone());
    queue.change_after_start = true;
    let queue = Arc::new(queue);
    let coverage = Arc::new(Coverage::new(state.clone(), true));
    let definitions = Arc::new(Definitions::default());
    ports.backtests_write = queue.clone();
    ports.backtest_sync = coverage.clone();
    ports.strategy_definition = definitions.clone();
    let ports = Arc::new(ports);
    let result = executor(ports.clone())
        .execute("strategy.optimize", &optimize_arguments())
        .expect("optimization");
    assert_eq!(
        coverage.requests.lock().expect("coverage")[0].provider,
        "futu"
    );
    let starts = queue.starts.lock().expect("starts");
    assert_eq!(starts.len(), 2);
    for (payload, provider) in starts.iter() {
        assert_eq!(payload["marketDataProvider"], "futu");
        assert_eq!(provider.as_deref(), Some("futu"));
    }
    assert_eq!(starts[0].0["definitionId"], "def-a");
    assert_eq!(starts[1].0["definitionId"], "def-b");
    assert_eq!(
        *definitions.requests.lock().expect("definitions"),
        ["def-a", "def-b"]
    );
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 1);
    assert_eq!(
        state.current.lock().expect("default").as_deref(),
        Some("yfinance")
    );
    assert_eq!(result["runs"].as_array().expect("runs").len(), 2);
    for run in result["runs"].as_array().expect("runs") {
        assert_eq!(run["marketDataProvider"], "futu");
    }
    let task = ports
        .mcp_store
        .get_optimization_task(result["taskId"].as_str().expect("task id"))
        .expect("stored task")
        .expect("persisted");
    let task: Value = serde_json::from_str(&task.payload_json).expect("task JSON");
    assert_eq!(
        task["runs"],
        json!([{"definitionId":"def-a","runId":"run-def-a"},{"definitionId":"def-b","runId":"run-def-b"}])
    );
}

// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:699 TestADKBacktestProviderFreezesDefaultAcrossPreparationAndQueue
#[test]
fn research_explicit_provider_is_normalized_without_reading_or_mutating_the_default() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(Some("futu")));
    let queue = Arc::new(Queue::new(state.clone()));
    let coverage = Arc::new(Coverage::new(state.clone(), false));
    ports.backtests_write = queue.clone();
    ports.backtest_sync = coverage.clone();
    let mut args = research_arguments();
    args["marketDataProvider"] = json!(" YFINANCE ");
    let result = executor(Arc::new(ports))
        .execute("strategy.research_backtest", &args)
        .expect("explicit override");
    assert_eq!(
        coverage.requests.lock().expect("coverage")[0].provider,
        "yfinance"
    );
    assert_eq!(
        queue.starts.lock().expect("starts")[0].0["marketDataProvider"],
        "yfinance"
    );
    assert_eq!(result["marketDataProvider"], "yfinance");
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 0);
    assert_eq!(
        state.current.lock().expect("default").as_deref(),
        Some("futu")
    );
}

// Parity: go:452dea11:internal/assistant/assembly/adk_strategy_test.go:774 TestADKConcurrentResearchBacktestOverridesStayIsolated
#[test]
fn concurrent_research_provider_overrides_reach_the_queue_together_without_default_reads() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(Some("futu")));
    let (entered, seen) = mpsc::channel();
    let mut queue = Queue::new(state.clone());
    queue.entered = Some(entered);
    *queue.release.0.lock().expect("release") = false;
    let queue = Arc::new(queue);
    let coverage = Arc::new(Coverage::new(state.clone(), false));
    ports.backtests_write = queue.clone();
    ports.backtest_sync = coverage.clone();
    let executor = Arc::new(executor(Arc::new(ports)));
    let (results, completed) = mpsc::channel();
    std::thread::scope(|scope| {
        for provider in ["yfinance", "akshare"] {
            let executor = executor.clone();
            let results = results.clone();
            scope.spawn(move || {
                let mut args = research_arguments();
                args["marketDataProvider"] = json!(provider);
                args["startTime"] = json!("2026-01-02T00:00:00Z");
                args["endTime"] = json!("2026-01-03T00:00:00Z");
                results
                    .send(executor.execute("strategy.research_backtest", &args))
                    .expect("completion");
            });
        }
        let mut providers = Vec::new();
        for _ in 0..2 {
            providers.push(seen.recv_timeout(Duration::from_secs(2)));
        }
        *queue.release.0.lock().expect("release") = true;
        queue.release.1.notify_all();
        let mut providers: Vec<_> = providers
            .into_iter()
            .map(|v| v.expect("concurrent queue entry"))
            .collect();
        providers.sort();
        assert_eq!(providers, ["akshare", "yfinance"]);
        for _ in 0..2 {
            let result = completed
                .recv_timeout(Duration::from_secs(2))
                .expect("bounded result")
                .expect("successful research");
            assert_eq!(result["status"], "queued");
        }
    });
    assert_eq!(queue.starts.lock().expect("starts").len(), 2);
    let mut providers: Vec<_> = coverage
        .requests
        .lock()
        .expect("coverage")
        .iter()
        .map(|r| r.provider.clone())
        .collect();
    providers.sort();
    assert_eq!(providers, ["akshare", "yfinance"]);
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 0);
    assert_eq!(
        state.current.lock().expect("default").as_deref(),
        Some("futu")
    );
}

#[test]
fn backtest_tools_reject_malformed_inputs_before_reading_the_provider_or_queue() {
    let (_directory, mut ports) = production_bundle();
    let state = Arc::new(ProviderState::new(Some("futu")));
    let queue = Arc::new(Queue::new(state.clone()));
    let coverage = Arc::new(Coverage::new(state.clone(), false));
    ports.backtests_write = queue.clone();
    ports.backtest_sync = coverage.clone();
    let executor = executor(Arc::new(ports));
    assert_eq!(
        executor
            .execute("strategy.research_backtest", &json!({}))
            .expect_err("missing script"),
        "script is required"
    );
    assert_eq!(
        executor
            .execute("strategy.optimize", &json!({}))
            .expect_err("missing candidates"),
        "definitionIds is required"
    );
    let mut args = research_arguments();
    args["tradingCosts"] = json!(42);
    assert_eq!(
        executor
            .execute("strategy.research_backtest", &args)
            .expect_err("bad costs"),
        "tradingCosts must be a valid object"
    );
    assert_eq!(state.default_reads.load(Ordering::SeqCst), 0);
    assert!(queue.starts.lock().expect("starts").is_empty());
    assert!(coverage.requests.lock().expect("coverage").is_empty());
}
