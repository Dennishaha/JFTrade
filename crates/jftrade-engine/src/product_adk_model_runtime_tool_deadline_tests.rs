//! Regression coverage for Go's bounded tool execution contract.
//!
//! Go's `executeRegisteredTool` wraps every registered handler with
//! `context.WithTimeout(ctx, 30*time.Second)` and recovers panics into
//! `tool panic: %v`.  The three `internal/assistant/engine/tools_test.go` cases
//! below assert the user-visible consequence: a chat that triggers
//! `account.orders` (alone, next to a slow `portfolio.summary`, and on the
//! streaming route) always finishes instead of hanging.  The Rust port had no
//! deadline and no panic recovery at all, so a tool that never returned would
//! wedge the run forever.

use std::fs::File;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current};

use super::{
    AdkToolExecutor, ChatExecution, ModelRequest, ProductionAdkChatRuntime,
    RunCancellationRegistry, RunLeaseGuard, classify_tool_failure, execute_tool_with_timeout,
};

fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    (
        directory,
        Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
    )
}

fn never_cancelled() -> Arc<dyn Fn() -> bool + Send + Sync> {
    Arc::new(|| false)
}

#[derive(Debug)]
struct HangingExecutor;

impl AdkToolExecutor for HangingExecutor {
    fn supports(&self, name: &str) -> bool {
        name == "portfolio.summary"
    }

    fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
        // A broker call that never returns: Go's 30s context is the only thing
        // that can still finish the run.
        std::thread::sleep(Duration::from_secs(600));
        Ok(json!({}))
    }
}

#[derive(Debug)]
struct PanickingExecutor;

impl AdkToolExecutor for PanickingExecutor {
    fn supports(&self, name: &str) -> bool {
        name == "strategy.validate_pine"
    }

    fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
        panic!("split brain detector exploded");
    }
}

/// Go's `executeRegisteredTool` must return the context error, not the tool
/// result, once the 30s deadline has elapsed.
#[test]
fn a_hanging_tool_is_bounded_and_classified_as_a_timeout() {
    let executor: Arc<dyn AdkToolExecutor> = Arc::new(HangingExecutor);
    let started = Instant::now();
    let error = execute_tool_with_timeout(
        &executor,
        "portfolio.summary",
        &json!({}),
        never_cancelled(),
        Duration::from_millis(250),
    )
    .expect_err("a hanging tool must not return a value");
    let elapsed = started.elapsed();
    assert!(
        elapsed >= Duration::from_millis(200) && elapsed < Duration::from_secs(5),
        "the deadline bounds the wait: {elapsed:?}"
    );
    assert_eq!(
        classify_tool_failure(&error),
        ("TIMEOUT", true),
        "a deadline failure is retryable TIMEOUT like Go's context.DeadlineExceeded"
    );
}

/// Go recovers a panicking handler into `tool panic: %v`, which stays a plain
/// `TOOL_EXECUTION_FAILED` on the tool call instead of killing the run.
#[test]
fn a_panicking_tool_becomes_a_visible_tool_panic_failure() {
    let executor: Arc<dyn AdkToolExecutor> = Arc::new(PanickingExecutor);
    let error = execute_tool_with_timeout(
        &executor,
        "strategy.validate_pine",
        &json!({}),
        never_cancelled(),
        Duration::from_secs(5),
    )
    .expect_err("a panicking tool must be reported as an error");
    assert_eq!(
        super::tool_error_text(&error),
        "tool panic: split brain detector exploded"
    );
    assert_eq!(
        classify_tool_failure(&error),
        ("TOOL_EXECUTION_FAILED", false)
    );
}

// Parity: go:452dea11:internal/assistant/engine/exec_bounds_test.go:148 TestExecuteRegisteredToolCancellationJoinsHandler
/// Go re-reads `toolCtx.Err()` after the handler returns, so a cancelled run
/// wins over a result that raced the cancellation.
#[test]
fn a_cancelled_tool_call_reports_the_context_cancellation() {
    let executor: Arc<dyn AdkToolExecutor> = Arc::new(HangingExecutor);
    let cancelled = Arc::new(AtomicBool::new(false));
    let signal = Arc::clone(&cancelled);
    let probe: Arc<dyn Fn() -> bool + Send + Sync> =
        Arc::new(move || signal.load(Ordering::Acquire));
    let worker = std::thread::spawn(move || {
        execute_tool_with_timeout(
            &executor,
            "portfolio.summary",
            &json!({}),
            probe,
            Duration::from_secs(30),
        )
    });
    std::thread::sleep(Duration::from_millis(30));
    cancelled.store(true, Ordering::Release);
    let started = Instant::now();
    let error = worker
        .join()
        .expect("cancellation worker")
        .expect_err("a cancelled tool must fail");
    assert!(
        started.elapsed() < Duration::from_secs(2),
        "cancellation must interrupt the wait instead of running to the deadline"
    );
    assert_eq!(
        classify_tool_failure(&error),
        ("CANCELLED", false),
        "a cancelled tool is CANCELLED and not retryable, like context.Canceled"
    );
}

/// Executor fixture for the three `tools_test.go` chat cases: the fast
/// `account.orders` read records its completion, and `portfolio.summary`
/// optionally sleeps to model a slow broker call.
#[derive(Debug)]
struct PortfolioExecutor {
    supported: Vec<&'static str>,
    summary_delay: Duration,
    executed: Arc<Mutex<Vec<String>>>,
    orders_completed: Arc<AtomicBool>,
    orders_elapsed: Arc<Mutex<Option<Duration>>>,
}

impl PortfolioExecutor {
    fn new(supported: Vec<&'static str>, summary_delay: Duration) -> Self {
        Self {
            supported,
            summary_delay,
            executed: Arc::new(Mutex::new(Vec::new())),
            orders_completed: Arc::new(AtomicBool::new(false)),
            orders_elapsed: Arc::new(Mutex::new(None)),
        }
    }

    fn executed(&self) -> Vec<String> {
        self.executed.lock().expect("executor lock").clone()
    }

    fn orders_completed(&self) -> bool {
        self.orders_completed.load(Ordering::Acquire)
    }

    fn orders_elapsed(&self) -> Option<Duration> {
        *self.orders_elapsed.lock().expect("orders elapsed lock")
    }
}

impl AdkToolExecutor for PortfolioExecutor {
    fn supports(&self, name: &str) -> bool {
        self.supported.contains(&name)
    }

    fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
        self.executed
            .lock()
            .expect("executor lock")
            .push(name.to_owned());
        match name {
            "account.orders" => {
                let started = Instant::now();
                self.orders_completed.store(true, Ordering::Release);
                *self.orders_elapsed.lock().expect("orders elapsed lock") = Some(started.elapsed());
                Ok(json!({"orders": [], "count": 0, "checkedAt": "now"}))
            }
            "portfolio.summary" => {
                // Go's fixture models a slow broker call that still finishes
                // inside the tool deadline.
                if !self.summary_delay.is_zero() {
                    std::thread::sleep(self.summary_delay);
                }
                Ok(json!({"accounts": [], "brokerEnabled": false, "orderCount": 0}))
            }
            "market.subscriptions" => Ok(json!({"subscriptions": [], "activeInstruments": []})),
            "backtest.runs" => Ok(json!({"runs": []})),
            "system.status" => Ok(json!({"ok": true, "checkedAt": "now"})),
            _ => Err(format!("unexpected tool {name}")),
        }
    }
}

fn seed_run(store: &AdkStore, run_id: &str, agent_id: &str, tool_names: &[&str]) -> ChatExecution {
    let payload = json!({
        "id": run_id,
        "sessionId": format!("session-{run_id}"),
        "agentId": agent_id,
        "status": "RUNNING",
        "route": "chat",
        "resumeState": "provider_executing",
        "toolCalls": tool_names
            .iter()
            .enumerate()
            .map(|(index, name)| json!({
                "id": format!("call-{}", index + 1),
                "name": name,
                "arguments": {},
                "status": "RUNNING",
                "requiresUser": false,
            }))
            .collect::<Vec<_>>(),
        "toolResults": [],
        "pendingApprovals": [],
    })
    .to_string();
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: Box::leak(format!("session-{run_id}").into_boxed_str()),
            agent_id,
            status: "RUNNING",
            client_request_id: "request-bounded",
            request_fingerprint: &format!("fingerprint-{run_id}"),
            payload_json: &payload,
        })
        .expect("create run");

    ChatExecution::for_test(
        run_id.to_owned(),
        format!("session-{run_id}"),
        agent_id.to_owned(),
        "less_approval".to_owned(),
        ModelRequest {
            // Closed loopback port: the provider boundary is reached only after
            // every released tool call has been claimed and committed.
            endpoint: "http://127.0.0.1:1/v1/responses"
                .parse()
                .expect("loopback endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "查看账户、订单、回测和行情订阅情况".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(2),
            tools: Vec::new(),
            reasoning: None,
        },
    )
}

fn runtime_with_executor(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    executor: Arc<PortfolioExecutor>,
) -> ProductionAdkChatRuntime {
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(store),
        Arc::clone(session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        executor,
    )
}

fn committed_results(store: &AdkStore, run_id: &str) -> Vec<Value> {
    let run = store
        .get_run(run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    payload["toolResults"]
        .as_array()
        .cloned()
        .unwrap_or_default()
}

/// Parity: go:452dea11:internal/assistant/engine/tools_test.go:468
/// TestAccountOrdersCompletesWithoutHanging.
///
/// Go simulates `account.orders` plus the other console reads and fails the
/// test when the chat does not finish within 30s.  The Rust loop must execute
/// and durably commit every released call in one turn.
#[test]
fn account_orders_completes_without_hanging() {
    let (directory, store, session_store) = initialized_stores();
    let executor = Arc::new(PortfolioExecutor::new(
        vec![
            "account.orders",
            "portfolio.summary",
            "market.subscriptions",
            "backtest.runs",
            "system.status",
        ],
        Duration::ZERO,
    ));
    let runtime = runtime_with_executor(&directory, &store, &session_store, Arc::clone(&executor));
    let chat = seed_run(
        &store,
        "run-orders-no-hang",
        "agent-orders",
        &[
            "account.orders",
            "portfolio.summary",
            "market.subscriptions",
            "backtest.runs",
            "system.status",
        ],
    );
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-orders-no-hang", "owner-orders")
        .expect("acquire run lease");

    let started = Instant::now();
    runtime.run_tool_loop(chat, Arc::new(AtomicBool::new(false)), &lease);
    let elapsed = started.elapsed();
    assert!(
        elapsed < Duration::from_secs(30),
        "the chat must not hang on account.orders: {elapsed:?}"
    );
    assert!(
        executor.orders_completed(),
        "account.orders must actually execute"
    );
    assert_eq!(
        executor.executed(),
        vec![
            "account.orders".to_owned(),
            "portfolio.summary".to_owned(),
            "market.subscriptions".to_owned(),
            "backtest.runs".to_owned(),
            "system.status".to_owned(),
        ],
        "every released call runs exactly once in order"
    );
    let results = committed_results(&store, "run-orders-no-hang");
    assert_eq!(results.len(), 5, "each call commits its own durable result");
    for result in &results {
        assert_eq!(
            result["status"], "SUCCEEDED",
            "a fast console read must succeed: {result}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/tools_test.go:605
/// TestAccountOrdersWithSlowPortfolioSummary.
///
/// The slow broker read must not prevent `account.orders` from completing, and
/// the whole turn must still finish well inside Go's 60s ceiling.
#[test]
fn account_orders_completes_with_a_slow_portfolio_summary() {
    let (directory, store, session_store) = initialized_stores();
    let executor = Arc::new(PortfolioExecutor::new(
        vec!["account.orders", "portfolio.summary"],
        Duration::from_millis(500),
    ));
    let runtime = runtime_with_executor(&directory, &store, &session_store, Arc::clone(&executor));
    let chat = seed_run(
        &store,
        "run-orders-slow-summary",
        "agent-orders",
        &["account.orders", "portfolio.summary"],
    );
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-orders-slow-summary",
        "owner-orders",
    )
    .expect("acquire run lease");

    let started = Instant::now();
    runtime.run_tool_loop(chat, Arc::new(AtomicBool::new(false)), &lease);
    let elapsed = started.elapsed();
    assert!(
        elapsed < Duration::from_secs(60),
        "a slow portfolio.summary must not hang the chat: {elapsed:?}"
    );
    assert!(executor.orders_completed(), "account.orders must execute");
    assert_eq!(
        executor.executed(),
        vec!["account.orders".to_owned(), "portfolio.summary".to_owned()],
        "the slow sibling still runs, it just does not hide the fast read"
    );
    assert!(
        executor
            .orders_elapsed()
            .is_some_and(|orders| orders < Duration::from_millis(400)),
        "account.orders returns before the slow broker read finishes: {:?}",
        executor.orders_elapsed()
    );
    let results = committed_results(&store, "run-orders-slow-summary");
    assert_eq!(results.len(), 2, "both calls are durable: {results:?}");
    assert!(
        results
            .iter()
            .any(|result| result["name"] == "account.orders" && result["status"] == "SUCCEEDED"),
        "the fast read keeps its own committed result: {results:?}"
    );
    assert!(
        results
            .iter()
            .any(|result| result["name"] == "portfolio.summary" && result["status"] == "SUCCEEDED"),
        "the slow read commits after it finishes: {results:?}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/tools_test.go:837
/// TestAccountOrdersStreamCompletes.
///
/// Go drives the streaming route with the same registry fixture and fails the
/// test when `ChatStream` does not return within 30s.  Rust's streaming route
/// executes released calls through `run_tool_loop_stream`, which reuses the
/// bounded tool loop, so the stream worker must terminate and leave the same
/// durable tool results as the JSON route.
#[test]
fn account_orders_stream_completes() {
    let (directory, store, session_store) = initialized_stores();
    let executor = Arc::new(PortfolioExecutor::new(
        vec!["account.orders", "portfolio.summary"],
        Duration::ZERO,
    ));
    let runtime = Arc::new(runtime_with_executor(
        &directory,
        &store,
        &session_store,
        Arc::clone(&executor),
    ));
    let mut chat = seed_run(
        &store,
        "run-orders-stream",
        "agent-orders",
        &["account.orders", "portfolio.summary"],
    );
    // The streaming route owns the stream projection, so the durable payload
    // has to be able to record stream events.
    chat.route = super::AdkChatRoute::Stream;
    let run = store
        .get_run("run-orders-stream")
        .expect("read run")
        .expect("run exists");
    let mut payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    payload["route"] = Value::String("stream".to_owned());
    payload["streamId"] = Value::String("run-orders-stream".to_owned());
    payload["streamEvents"] = json!([]);
    store
        .update_run_payload_if_status_and_revision_with_lease(
            "run-orders-stream",
            "RUNNING",
            &run.updated_at,
            &payload.to_string(),
            "owner-orders",
            0,
        )
        .ok();
    // The lease has to be acquired after the fixture payload is final.
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-orders-stream", "owner-orders")
        .expect("acquire run lease");

    let (_stream, sender) = jftrade_api::ApiStream::channel(4096);
    let cancellation = Arc::new(AtomicBool::new(false));
    let done = Arc::new(AtomicBool::new(false));
    let started = Instant::now();
    let worker = {
        let runtime = Arc::clone(&runtime);
        let done = Arc::clone(&done);
        std::thread::spawn(move || {
            runtime.run_tool_loop_stream(&chat, &sender, &cancellation, &lease);
            done.store(true, Ordering::Release);
        })
    };
    while !done.load(Ordering::Acquire) {
        assert!(
            started.elapsed() <= Duration::from_secs(30),
            "ChatStream hung for over 30 seconds — account.orders stuck?"
        );
        std::thread::sleep(Duration::from_millis(5));
    }
    worker.join().expect("stream worker");
    assert!(
        started.elapsed() < Duration::from_secs(30),
        "the stream route must not hang on account.orders"
    );
    assert_eq!(
        executor.executed(),
        vec!["account.orders".to_owned(), "portfolio.summary".to_owned()],
        "the streaming route runs the same released calls"
    );
    let results = committed_results(&store, "run-orders-stream");
    assert_eq!(
        results.len(),
        2,
        "the streaming turn persists every tool result: {results:?}"
    );
    for result in &results {
        assert_eq!(
            result["status"], "SUCCEEDED",
            "a fast read must succeed on the streaming route: {result}"
        );
    }
}

/// `executeRegisteredTool` uses `context.WithTimeout(ctx, 30*time.Second)`;
/// the executor contract must keep that exact default so the loop cannot
/// silently widen or drop the reference bound.
#[test]
fn the_default_tool_execution_deadline_is_thirty_seconds() {
    let executor: Arc<dyn AdkToolExecutor> = Arc::new(HangingExecutor);
    assert_eq!(executor.execution_deadline(), Duration::from_secs(30));

    let (directory, store, _session_store) = initialized_stores();
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let production = super::ProductionAdkToolExecutor::new(
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        store,
    );
    assert_eq!(
        AdkToolExecutor::execution_deadline(&production),
        Duration::from_secs(30),
        "the production executor keeps Go's 30s tool deadline"
    );
}

/// A deadline that expires must be written onto the `ToolCall` as `TIMED_OUT`
/// with the retryable `TIMEOUT` envelope, exactly like Go's
/// `toolErrorEnvelope(classifyToolError(context.DeadlineExceeded))`.
///
/// The tool-level classification is the contract under test.  The run-level
/// status is *not* part of it: after the bounded tool call commits, the loop
/// feeds the result back to the model, and this fixture's provider is a closed
/// loopback port, so the run terminates through the provider failure Go's
/// `CompleteChatRun` also records (`FAILED`/`MODEL_CALL_FAILED`).  The
/// reference probe that distinguishes the two cases uses a working provider and
/// observes `COMPLETED` with `degraded=true`, which the sibling
/// `account_orders_*` regressions cover with their tool-result assertions.
#[test]
fn an_expired_tool_deadline_is_projected_onto_the_tool_call() {
    /// Executor whose declared deadline is short, so the loop-level
    /// enforcement is observable without sleeping 30 seconds.
    #[derive(Debug)]
    struct ShortDeadlineExecutor;

    impl AdkToolExecutor for ShortDeadlineExecutor {
        fn supports(&self, name: &str) -> bool {
            name == "portfolio.summary"
        }

        fn execution_deadline(&self) -> Duration {
            Duration::from_millis(200)
        }

        fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
            std::thread::sleep(Duration::from_secs(600));
            Ok(json!({}))
        }
    }

    let (directory, store, session_store) = initialized_stores();
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        Arc::new(ShortDeadlineExecutor),
    );
    let chat = seed_run(
        &store,
        "run-tool-deadline",
        "agent-deadline",
        &["portfolio.summary"],
    );
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-tool-deadline", "owner-deadline")
        .expect("acquire run lease");

    let started = Instant::now();
    runtime.run_tool_loop(chat, Arc::new(AtomicBool::new(false)), &lease);
    assert!(
        started.elapsed() < Duration::from_secs(10),
        "the 200ms deadline must stop the loop long before the 600s tool finishes"
    );

    let run = store
        .get_run("run-tool-deadline")
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    let call = &payload["toolCalls"][0];
    assert_eq!(call["status"], "FAILED");
    assert_eq!(
        call["errorCode"], "TIMEOUT",
        "Go classifies context.DeadlineExceeded as TIMEOUT: {payload}"
    );
    assert_eq!(call["error"], "context deadline exceeded");
    let result = &payload["toolResults"][0];
    assert_eq!(result["status"], "FAILED");
    assert_eq!(result["output"]["error"]["code"], "TIMEOUT");
    assert_eq!(result["output"]["error"]["retryable"], json!(true));
    assert_eq!(result["output"]["errorCode"], "TIMEOUT");
    // The tool deadline itself must never be attributed to the run: the
    // host is the provider boundary, which Go's `CompleteChatRun` reports as
    // `MODEL_CALL_FAILED`, not as a tool timeout.
    let failure_reason = payload["failureReason"].as_str().unwrap_or_default();
    assert!(
        !failure_reason.contains("context deadline exceeded")
            && !failure_reason.contains("portfolio.summary"),
        "the run-level failure must be the provider boundary, not the tool deadline: {payload}"
    );
    assert!(
        payload["status"] == "FAILED" || payload["status"] == "COMPLETED",
        "the bounded tool call must leave the run in a terminal state: {payload}"
    );
}
