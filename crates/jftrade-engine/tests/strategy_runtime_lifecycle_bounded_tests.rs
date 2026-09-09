#![forbid(unsafe_code)]

//! Lifecycle, bounded graceful shutdown, and resource normalization tests for
//! strategy runtime (Milestone 6 / Requirement R5).
//!
//! Verifies:
//! 1. Normal single stop terminates promptly (<500ms).
//! 2. Bounded 3-second stop timeout on simulated hung/unresponsive strategy task without blocking caller.
//! 3. Bounded 3-second stop timeout on synchronously blocking strategy task (thread detachment).
//! 4. Bounded 5-second global shutdown deadline across multiple instances with hung tasks.
//! 5. Clean immediate shutdown (<500ms) when all tasks terminate normally without waiting for deadline.
//! 6. Resource normalization: verify `new_current_thread` allocates strictly 1 thread per instance
//!    without multi-threaded `num_cpus` thread pool explosion.
//! 7. Idempotent cancel and shutdown operations.

#[path = "../src/product_strategy_runtime_write_port.rs"]
pub mod product_strategy_runtime_write_port;

mod product {
    pub use super::product_strategy_runtime_write_port;
    pub use jftrade_engine::product::*;

    pub mod product_production_ports {
        use std::sync::Arc;

        #[derive(Clone, Debug, Default)]
        pub struct SharedTradeReadRuntime;

        impl SharedTradeReadRuntime {
            pub fn snapshot(&self) -> TradeRuntimeReadSnapshot {
                TradeRuntimeReadSnapshot::default()
            }
        }

        #[derive(Clone, Default)]
        pub struct TradeRuntimeReadSnapshot {
            pub client: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
            pub trade_logged_in: Option<bool>,
        }
    }
}

#[allow(dead_code)]
#[path = "../src/strategy_runtime.rs"]
mod strategy_runtime;

use std::collections::HashSet;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use product::product_active_provider_state::ActiveProviderState;
use product::{MarketDataQuoteReadFuture, MarketDataQuoteReadSnapshotPort};
use rusqlite::Connection;
use serde_json::{Value, json};

use jftrade_integration_pine::{GrpcPineExecutionPort, PineExecutionConfig};
use jftrade_store_sqlite::StrategyRuntimeStore;
use strategy_runtime::{STRATEGY_SHUTDOWN_TIMEOUT, STRATEGY_STOP_TIMEOUT, StrategyRuntimeManager};

/// Mock quote snapshot port that can simulate normal responses, async delay hangs,
/// or synchronous OS thread blocks based on requested symbols.
#[derive(Debug)]
struct MockQuotePort {
    hang_symbols: Arc<Mutex<HashSet<String>>>,
    blocking_symbols: Arc<Mutex<HashSet<String>>>,
    delay: Duration,
    call_count: Arc<Mutex<usize>>,
}

impl MockQuotePort {
    fn new(delay: Duration) -> Self {
        Self {
            hang_symbols: Arc::new(Mutex::new(HashSet::new())),
            blocking_symbols: Arc::new(Mutex::new(HashSet::new())),
            delay,
            call_count: Arc::new(Mutex::new(0)),
        }
    }

    fn add_hang_symbol(&self, symbol: &str) {
        self.hang_symbols.lock().unwrap().insert(symbol.to_owned());
    }

    fn add_blocking_symbol(&self, symbol: &str) {
        self.blocking_symbols
            .lock()
            .unwrap()
            .insert(symbol.to_owned());
    }
}

impl MarketDataQuoteReadSnapshotPort for MockQuotePort {
    fn read<'a>(&'a self, path: &'a str, _query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        *self.call_count.lock().unwrap() += 1;
        let is_hang = {
            let set = self.hang_symbols.lock().unwrap();
            set.iter()
                .any(|s| path.contains(s.as_str()) || path.contains(&s.replace('.', "/")))
        };
        let is_blocking = {
            let set = self.blocking_symbols.lock().unwrap();
            set.iter()
                .any(|s| path.contains(s.as_str()) || path.contains(&s.replace('.', "/")))
        };
        let delay = self.delay;

        if is_blocking {
            // Synchronous block: halts the OS thread without yielding to Tokio
            std::thread::sleep(delay);
            Box::pin(std::future::ready(Ok(json!({ "candles": [] }))))
        } else if is_hang {
            // Asynchronous hang: sleeps inside async future
            Box::pin(async move {
                tokio::time::sleep(delay).await;
                Ok(json!({ "candles": [] }))
            })
        } else {
            // Immediate normal response
            Box::pin(std::future::ready(Ok(json!({ "candles": [] }))))
        }
    }
}

fn seed_test_database(path: &std::path::Path) {
    let conn = Connection::open(path).expect("open test db");
    jftrade_store_sqlite::initialize_current(&conn, "strategy")
        .expect("initialize strategy schema");
}

fn create_test_worker() -> Arc<GrpcPineExecutionPort> {
    Arc::new(
        GrpcPineExecutionPort::new(PineExecutionConfig {
            endpoint: "http://127.0.0.1:9".to_owned(),
            connect_timeout: Duration::from_millis(50),
            request_timeout: Duration::from_millis(100),
            ..PineExecutionConfig::default()
        })
        .expect("pine worker client"),
    )
}

fn create_test_harness(
    quote: Arc<MockQuotePort>,
) -> (
    StrategyRuntimeManager,
    Arc<StrategyRuntimeStore>,
    tempfile::TempDir,
) {
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let db_path = temp_dir.path().join("strategy.db");
    seed_test_database(&db_path);

    let store = Arc::new(StrategyRuntimeStore::open(&db_path).expect("open store"));
    let worker = create_test_worker();
    let provider = Arc::new(ActiveProviderState::default());

    let manager = StrategyRuntimeManager::new(
        None,
        Some(worker),
        Some(quote as Arc<dyn MarketDataQuoteReadSnapshotPort>),
        None,
        provider,
    );

    (manager, store, temp_dir)
}

fn strategy_binding(symbol: &str) -> Value {
    json!({
        "script": "//@version=5\nindicator('lifecycle_test')\nplot(close)",
        "symbols": [symbol],
        "interval": "1m",
        "market": "US",
        "executeOrders": false,
    })
}

/// Helper to measure process thread count on Unix / macOS / Linux.
fn count_process_threads() -> Option<usize> {
    #[cfg(target_os = "linux")]
    {
        if let Ok(entries) = std::fs::read_dir("/proc/self/task") {
            return Some(entries.count());
        }
    }
    #[cfg(target_os = "macos")]
    {
        let pid = std::process::id().to_string();
        if let Ok(output) = std::process::Command::new("ps").args(["-M", &pid]).output() {
            let text = String::from_utf8_lossy(&output.stdout);
            let count = text.lines().count().saturating_sub(1);
            if count > 0 {
                return Some(count);
            }
        }
    }
    None
}

// =========================================================================
// Test 1: Normal Stop Terminates Promptly (<500ms)
// =========================================================================

#[test]
fn test_strategy_normal_stop_immediate() {
    let quote = Arc::new(MockQuotePort::new(Duration::from_millis(10)));
    let (manager, store, _dir) = create_test_harness(quote);

    let instance_id = "test-normal-stop";
    store
        .seed_instance_with_binding(
            instance_id,
            "RUNNING",
            strategy_binding("US.AAPL"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed instance");

    manager
        .spawn_task(
            instance_id.to_owned(),
            strategy_binding("US.AAPL"),
            Arc::clone(&store),
        )
        .expect("spawn task");

    // Wait a brief moment for the thread to enter its poll loop
    std::thread::sleep(Duration::from_millis(60));

    // Cancel and measure duration
    let start = Instant::now();
    manager.cancel(instance_id);
    let elapsed = start.elapsed();

    // Normal termination must join promptly (<500ms, typically <100ms)
    assert!(
        elapsed < Duration::from_millis(500),
        "normal stop took too long: {elapsed:?}"
    );

    // Redundant cancel should succeed cleanly as a safe no-op
    manager.cancel(instance_id);
}

// =========================================================================
// Test 2: Bounded 3-Second Stop Timeout on Async-Hung Strategy Task
// =========================================================================

#[test]
fn asynchronous_quote_wait_is_cancelled_without_waiting_for_timeout() {
    // Hangs for 15 seconds, well beyond the 3s stop timeout
    let quote = Arc::new(MockQuotePort::new(Duration::from_secs(15)));
    quote.add_hang_symbol("US.HUNG");

    let (manager, store, _dir) = create_test_harness(quote);

    let instance_id = "test-async-hung";
    store
        .seed_instance_with_binding(
            instance_id,
            "RUNNING",
            strategy_binding("US.HUNG"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed instance");

    manager
        .spawn_task(
            instance_id.to_owned(),
            strategy_binding("US.HUNG"),
            Arc::clone(&store),
        )
        .expect("spawn task");

    // Give the thread time to enter the hanging read_strategy_candles call
    std::thread::sleep(Duration::from_millis(100));

    // Async quote I/O receives cancellation and must join promptly.
    let start = Instant::now();
    assert!(manager.cancel(instance_id));
    let elapsed = start.elapsed();

    assert!(
        elapsed < Duration::from_millis(500),
        "cancel must join the asynchronous task promptly; actual: {elapsed:?}"
    );
}

// =========================================================================
// Test 3: Bounded 3-Second Stop Timeout on Synchronously Blocking Task
// =========================================================================

#[test]
fn test_strategy_stop_bounded_timeout_on_synchronously_blocking_task() {
    // Synchronously blocks OS thread for 15s without yielding to Tokio
    let quote = Arc::new(MockQuotePort::new(Duration::from_secs(15)));
    quote.add_blocking_symbol("US.BLOCKING");

    let (manager, store, _dir) = create_test_harness(quote);

    let instance_id = "test-sync-blocking";
    store
        .seed_instance_with_binding(
            instance_id,
            "RUNNING",
            strategy_binding("US.BLOCKING"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed instance");

    manager
        .spawn_task(
            instance_id.to_owned(),
            strategy_binding("US.BLOCKING"),
            Arc::clone(&store),
        )
        .expect("spawn task");

    // Allow thread to enter synchronous sleep
    std::thread::sleep(Duration::from_millis(100));

    let start = Instant::now();
    manager.cancel(instance_id);
    let elapsed = start.elapsed();

    assert!(
        elapsed >= Duration::from_millis(2900),
        "cancel must respect bounded stop timeout ~3s; actual: {elapsed:?}"
    );
    assert!(
        elapsed < Duration::from_millis(3800),
        "synchronous thread block must be abandoned after 3s; actual: {elapsed:?}"
    );
}

// =========================================================================
// Test 4: Bounded 5-Second Shutdown Deadline Across Multiple Tasks
// =========================================================================

#[test]
fn test_strategy_shutdown_bounded_global_deadline() {
    let quote = Arc::new(MockQuotePort::new(Duration::from_secs(15)));
    // Task 2 hangs on quote read
    quote.add_blocking_symbol("US.HUNG2");

    let (manager, store, _dir) = create_test_harness(quote);

    // Instance 1: normal, fast
    store
        .seed_instance_with_binding(
            "inst-normal-1",
            "RUNNING",
            strategy_binding("US.AAPL"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed 1");
    manager
        .spawn_task(
            "inst-normal-1".to_owned(),
            strategy_binding("US.AAPL"),
            Arc::clone(&store),
        )
        .expect("spawn 1");

    // Instance 2: hung
    store
        .seed_instance_with_binding(
            "inst-hung-2",
            "RUNNING",
            strategy_binding("US.HUNG2"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed 2");
    manager
        .spawn_task(
            "inst-hung-2".to_owned(),
            strategy_binding("US.HUNG2"),
            Arc::clone(&store),
        )
        .expect("spawn 2");

    // Instance 3: normal, fast
    store
        .seed_instance_with_binding(
            "inst-normal-3",
            "RUNNING",
            strategy_binding("US.MSFT"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed 3");
    manager
        .spawn_task(
            "inst-normal-3".to_owned(),
            strategy_binding("US.MSFT"),
            Arc::clone(&store),
        )
        .expect("spawn 3");

    std::thread::sleep(Duration::from_millis(100));

    // Global shutdown must unblock within STRATEGY_SHUTDOWN_TIMEOUT (~5s)
    let start = Instant::now();
    manager.shutdown();
    let elapsed = start.elapsed();

    assert!(
        elapsed >= Duration::from_millis(4800),
        "shutdown must wait against the global deadline (~5s); actual: {elapsed:?}"
    );
    assert!(
        elapsed < Duration::from_millis(5900),
        "shutdown must not block beyond global deadline (~5.5s); actual: {elapsed:?}"
    );
}

// =========================================================================
// Test 5: Immediate Shutdown (<500ms) When All Tasks Normal
// =========================================================================

#[test]
fn test_strategy_shutdown_all_normal_tasks_immediate() {
    let quote = Arc::new(MockQuotePort::new(Duration::from_millis(10)));
    let (manager, store, _dir) = create_test_harness(quote);

    for i in 1..=3 {
        let id = format!("inst-fast-{i}");
        let symbol = format!("US.SYM{i}");
        store
            .seed_instance_with_binding(
                &id,
                "RUNNING",
                strategy_binding(&symbol),
                "2026-09-08T00:00:00Z",
            )
            .expect("seed");
        manager
            .spawn_task(id, strategy_binding(&symbol), Arc::clone(&store))
            .expect("spawn");
    }

    std::thread::sleep(Duration::from_millis(60));

    let start = Instant::now();
    manager.shutdown();
    let elapsed = start.elapsed();

    // Fast path: clean tasks join promptly without waiting for the 5s deadline
    assert!(
        elapsed < Duration::from_millis(500),
        "fast shutdown should terminate promptly; actual: {elapsed:?}"
    );
}

// =========================================================================
// Test 6: Thread Normalization (No num_cpus Worker Pool Explosion)
// =========================================================================

#[test]
fn test_strategy_runtime_current_thread_no_thread_explosion() {
    let num_cpus = std::thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(4);

    let quote = Arc::new(MockQuotePort::new(Duration::from_millis(10)));
    let (manager, store, _dir) = create_test_harness(quote);

    let baseline_threads = count_process_threads();

    const TASK_COUNT: usize = 4;
    for i in 1..=TASK_COUNT {
        let id = format!("inst-norm-{i}");
        let symbol = format!("US.NORM{i}");
        store
            .seed_instance_with_binding(
                &id,
                "RUNNING",
                strategy_binding(&symbol),
                "2026-09-08T00:00:00Z",
            )
            .expect("seed");
        manager
            .spawn_task(id, strategy_binding(&symbol), Arc::clone(&store))
            .expect("spawn");
    }

    // Wait for all 4 strategy runner threads to be active
    std::thread::sleep(Duration::from_millis(150));

    if let (Some(before), Some(after)) = (baseline_threads, count_process_threads()) {
        let delta = after.saturating_sub(before);
        // If Runtime::new() multi-threaded runtime were used:
        // Expected delta >= TASK_COUNT * (1 + num_cpus) (e.g. 4 * 9 = 36 threads on 8 cores).
        // With Builder::new_current_thread():
        // Exactly 1 OS thread per task, delta should be TASK_COUNT (at most TASK_COUNT + 1).
        assert!(
            delta <= TASK_COUNT + 2,
            "Thread explosion detected! Delta: {delta}, Task count: {TASK_COUNT}, Cores: {num_cpus}. \
             Expected ~{TASK_COUNT} OS threads with current_thread runtime."
        );
    }

    // Verify constants match the specification
    assert_eq!(STRATEGY_STOP_TIMEOUT, Duration::from_secs(3));
    assert_eq!(STRATEGY_SHUTDOWN_TIMEOUT, Duration::from_secs(5));

    manager.shutdown();
}

// =========================================================================
// Test 7: Idempotent Cancel and Shutdown Operations
// =========================================================================

#[test]
fn test_strategy_runtime_idempotent_lifecycle_operations() {
    let quote = Arc::new(MockQuotePort::new(Duration::from_millis(10)));
    let (manager, store, _dir) = create_test_harness(quote);

    let instance_id = "test-idempotent";
    store
        .seed_instance_with_binding(
            instance_id,
            "RUNNING",
            strategy_binding("US.AAPL"),
            "2026-09-08T00:00:00Z",
        )
        .expect("seed");

    // Cancel before task spawned does nothing and does not panic
    manager.cancel(instance_id);

    manager
        .spawn_task(
            instance_id.to_owned(),
            strategy_binding("US.AAPL"),
            Arc::clone(&store),
        )
        .expect("spawn");

    std::thread::sleep(Duration::from_millis(50));

    // First cancel stops the task
    manager.cancel(instance_id);

    // Second cancel is a safe no-op
    manager.cancel(instance_id);

    // Multiple shutdowns are safe no-ops
    manager.shutdown();
    manager.shutdown();
}
