//! Tool-catalog parity tests for the production MCP executor.
//!
//! Owner: `internal/assistant/assembly/tool_catalog_test.go` on the `go`
//! branch. Go's read tools normalize scope/market/identifier inputs before they
//! reach the business handler; the Rust owner is the same set of MCP tools
//! backed by the production snapshot ports.

use super::tests::production_bundle;
use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::ProductionPortBundle;
use crate::product::{
    BacktestSyncReadSnapshotError, BacktestSyncReadSnapshotPort, BrokerReadSnapshotError,
    BrokerReadSnapshotPort, ExecutionReadSnapshotError, ExecutionReadSnapshotPort,
    PluginSnapshotError, PluginSnapshotPort, SystemReadSnapshotError, SystemReadSnapshotPort,
    WatchlistReadSnapshotError, WatchlistReadSnapshotPort,
};
use serde_json::{Value, json};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tempfile::TempDir;

fn bundle_with<F: FnOnce(&mut ProductionPortBundle)>(
    configure: F,
) -> (TempDir, Arc<ProductionPortBundle>) {
    let (directory, mut ports) = production_bundle();
    configure(&mut ports);
    (directory, Arc::new(ports))
}

#[derive(Debug, Default)]
struct RecordingBrokerRead {
    reads: Mutex<Vec<(String, String)>>,
}

impl RecordingBrokerRead {
    fn recorded(&self) -> Vec<(String, String)> {
        self.reads.lock().expect("broker reads").clone()
    }
}

impl BrokerReadSnapshotPort for RecordingBrokerRead {
    fn read(&self, path: &str, query: &str) -> Result<Value, BrokerReadSnapshotError> {
        self.reads
            .lock()
            .expect("broker reads")
            .push((path.to_owned(), query.to_owned()));
        Ok(json!({"orders": [], "fills": [], "cashFlows": [], "fees": [], "ratios": []}))
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:272
/// `TestADKReadToolsNormalizeInputsAndExposeBusinessHandlers` (broker readers).
///
/// Go's `brokerReadInput` defaults the scope to CURRENT and copies the account,
/// environment, market, and symbol dimensions; each reader merges its
/// identifier list before calling the business handler. Rust forwards the same
/// dimensions to the broker snapshot port; order status lists are applied by
/// the trading service in both implementations, so they are not part of the
/// wire query asserted here.
#[test]
fn broker_read_tools_normalize_scope_filters_and_identifier_lists() {
    let recorder: Arc<RecordingBrokerRead> = Arc::new(RecordingBrokerRead::default());
    let (_directory, ports) = bundle_with(|ports| {
        let recorder_handle = Arc::clone(&recorder);

        let port: Arc<dyn BrokerReadSnapshotPort> = recorder_handle;
        ports.broker = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    executor
        .execute_production(
            "broker.orders",
            &json!({
                "tradingEnvironment": "REAL",
                "accountId": "acct-live",
                "symbol": "AAPL",
                "status": ["SUBMITTED"],
            }),
        )
        .expect("broker.orders");
    executor
        .execute_production(
            "broker.fills",
            &json!({
                "scope": "history",
                "symbol": "MSFT",
                "startTime": "2025-01-01T09:30:00Z",
                "endTime": "2025-01-01T16:00:00Z",
            }),
        )
        .expect("broker.fills");
    executor
        .execute_production(
            "broker.cash_flows",
            &json!({"market": "HK", "clearingDate": "2025-01-03", "direction": "IN"}),
        )
        .expect("broker.cash_flows");
    executor
        .execute_production(
            "broker.fees",
            &json!({"orderIdEx": ["oid-1"], "orderIdExList": ["oid-2", "oid-3"]}),
        )
        .expect("broker.fees");
    executor
        .execute_production(
            "broker.margin_ratios",
            &json!({"symbols": ["700", "3690"], "symbol": "AAPL"}),
        )
        .expect("broker.margin_ratios");

    assert_eq!(
        recorder.recorded(),
        [
            (
                "/api/v1/brokers/futu/orders".to_owned(),
                "scope=CURRENT&tradingEnvironment=REAL&accountId=acct%2Dlive&symbol=AAPL".to_owned(),
            ),
            (
                "/api/v1/brokers/futu/fills".to_owned(),
                "scope=HISTORY&symbol=MSFT&startTime=2025%2D01%2D01T09%3A30%3A00Z&endTime=2025%2D01%2D01T16%3A00%3A00Z"
                    .to_owned(),
            ),
            (
                "/api/v1/brokers/futu/cash-flows".to_owned(),
                "scope=CURRENT&market=HK&clearingDate=2025%2D01%2D03&direction=IN".to_owned(),
            ),
            (
                "/api/v1/brokers/futu/order-fees".to_owned(),
                "scope=CURRENT&orderIdEx=oid%2D1%2Coid%2D2%2Coid%2D3".to_owned(),
            ),
            (
                "/api/v1/brokers/futu/margin-ratios".to_owned(),
                "scope=CURRENT&symbol=AAPL&symbols=700%2C3690%2CAAPL".to_owned(),
            ),
        ]
    );

    for (name, arguments) in [
        ("broker.cash_flows", json!({})),
        ("broker.fees", json!({})),
        ("broker.margin_ratios", json!({})),
    ] {
        let failure = executor
            .execute_production(name, &arguments)
            .expect_err("missing identifiers must fail before the port");
        assert_eq!(failure.code, "BAD_REQUEST", "{name}");
    }
    assert_eq!(recorder.recorded().len(), 5);

    let invalid_scope = executor
        .execute_production("broker.fills", &json!({"scope": "tomorrow"}))
        .expect_err("unknown scope must fail closed");
    assert_eq!(invalid_scope.code, "BAD_REQUEST");
}

#[derive(Debug)]
struct RecordingSystemRead {
    paths: Mutex<Vec<String>>,
}

impl SystemReadSnapshotPort for RecordingSystemRead {
    fn read(&self, path: &str) -> Result<Value, SystemReadSnapshotError> {
        self.paths
            .lock()
            .expect("system reads")
            .push(path.to_owned());
        Ok(match path {
            "/api/v1/system/futu-opend" => json!({"connected": true}),
            "/api/v1/system/real-trade-kill-switch" => json!({"killSwitch": false}),
            "/api/v1/system/real-trade-risk-limits" => json!({"maxNotional": 1000}),
            "/api/v1/system/real-trade-risk-events" => json!({"events": []}),
            other => panic!("unexpected system read {other}"),
        })
    }
}

#[derive(Debug)]
struct RecordingPlugins {
    catalog_calls: Mutex<usize>,
}

impl PluginSnapshotPort for RecordingPlugins {
    fn catalog(&self) -> Result<Value, PluginSnapshotError> {
        *self.catalog_calls.lock().expect("plugin calls") += 1;
        Ok(json!({"plugins": [{"id": "alpha"}]}))
    }

    fn operation(&self, _operation_id: &str) -> Result<Option<Value>, PluginSnapshotError> {
        Ok(None)
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:550
/// `TestADKCoreToolHandlersNormalizeMarketAndPortfolioFlows` (payload handlers).
///
/// The Go registry exposes `system.futu_opend`, `plugins.catalog`, `risk.state`,
/// and `risk.events` as thin projections over the injected dependencies; Rust
/// keeps the same thin projection over the system/plugin snapshot ports.
#[test]
fn system_plugin_and_risk_tools_project_port_payloads() {
    let system: Arc<RecordingSystemRead> = Arc::new(RecordingSystemRead {
        paths: Mutex::new(Vec::new()),
    });
    let plugins: Arc<RecordingPlugins> = Arc::new(RecordingPlugins {
        catalog_calls: Mutex::new(0),
    });
    let (_directory, ports) = bundle_with(|ports| {
        let system_handle = Arc::clone(&system);

        let system_port: Arc<dyn SystemReadSnapshotPort> = system_handle;
        let plugins_handle = Arc::clone(&plugins);

        let plugin_port: Arc<dyn PluginSnapshotPort> = plugins_handle;
        ports.system_read = system_port;
        ports.plugins = plugin_port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let opend = executor
        .execute_production("system.futu_opend", &json!({}))
        .expect("system.futu_opend");
    assert_eq!(opend["connected"], true);

    let catalog = executor
        .execute_production("plugins.catalog", &json!({}))
        .expect("plugins.catalog");
    assert_eq!(catalog["plugins"][0]["id"], "alpha");
    assert_eq!(*plugins.catalog_calls.lock().expect("plugin calls"), 1);

    let risk = executor
        .execute_production("risk.state", &json!({}))
        .expect("risk.state");
    assert_eq!(risk["killSwitch"]["killSwitch"], false);
    assert_eq!(risk["riskLimits"]["maxNotional"], 1000);

    let events = executor
        .execute_production("risk.events", &json!({}))
        .expect("risk.events");
    assert_eq!(events["events"], json!([]));

    assert_eq!(
        system.paths.lock().expect("system reads").as_slice(),
        [
            "/api/v1/system/futu-opend",
            "/api/v1/system/real-trade-kill-switch",
            "/api/v1/system/real-trade-risk-limits",
            "/api/v1/system/real-trade-risk-events",
        ]
    );
}

#[derive(Debug, Default)]
struct RecordingWatchlistRead {
    reads: Mutex<Vec<(String, String)>>,
}

impl WatchlistReadSnapshotPort for RecordingWatchlistRead {
    fn read(&self, path: &str, query: &str) -> Result<Value, WatchlistReadSnapshotError> {
        self.reads
            .lock()
            .expect("watchlist reads")
            .push((path.to_owned(), query.to_owned()));
        if path == "/api/v1/watchlist/groups" {
            // The stored wire shape keeps `groupId`/`name`, so the tool's
            // group resolution can mirror Go's `resolveWatchlistGroup`.
            return Ok(json!({
                "groups": [{"groupId": "group-tech", "name": "科技", "isDefault": false}]
            }));
        }
        Ok(json!({"items": []}))
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:672
/// `TestWatchlistListToolDefaultsToReadOnlyMetadataAndNormalizesPaging`.
///
/// Go trims the group/query/cursor filters, upper-cases the market, defaults
/// the limit to a page size and rejects limits above 200 while keeping
/// `includeQuotes` opt-in. The adapter resolves the caller's group reference
/// (id or case-insensitive name) before it lists members, so an unknown group
/// fails closed; Rust keeps that resolution in the tool and the production
/// watchlist snapshot has no quote enrichment, so the opt-in fails closed with
/// a structured 503 instead of fabricating quotes.
#[test]
fn watchlist_list_normalizes_filters_and_rejects_out_of_range_limits() {
    let recorder: Arc<RecordingWatchlistRead> = Arc::new(RecordingWatchlistRead::default());
    let (_directory, ports) = bundle_with(|ports| {
        let recorder_handle = Arc::clone(&recorder);

        let port: Arc<dyn WatchlistReadSnapshotPort> = recorder_handle;
        ports.watchlist = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    executor
        .execute_production(
            "watchlist.list",
            &json!({
                "groupName": " 科技 ",
                "market": "us",
                "query": " apple ",
                "cursor": " next ",
                "limit": 20,
            }),
        )
        .expect("watchlist.list page");

    let defaulted = executor
        .execute_production("watchlist.list", &json!({}))
        .expect("watchlist.list groups");

    assert_eq!(
        recorder.reads.lock().expect("watchlist reads").as_slice(),
        [
            ("/api/v1/watchlist/groups".to_owned(), "limit=20".to_owned()),
            (
                "/api/v1/watchlist/items".to_owned(),
                "groupId=group%2Dtech&market=US&query=apple&cursor=next&limit=20".to_owned(),
            ),
            ("/api/v1/watchlist/groups".to_owned(), "limit=50".to_owned(),),
        ]
    );
    assert_eq!(defaulted["groups"][0]["groupId"], json!("group-tech"));

    for limit in [0, 201] {
        let failure = executor
            .execute_production("watchlist.list", &json!({"limit": limit}))
            .expect_err("out-of-range limits must fail before the port");
        assert_eq!(failure.code, "BAD_REQUEST", "limit {limit}");
        assert!(
            failure.message.contains("between 1 and 200"),
            "limit {limit} must name the accepted range: {failure:?}"
        );
    }

    let quotes = executor
        .execute_production("watchlist.list", &json!({"includeQuotes": true}))
        .expect_err("quote enrichment is unavailable in production");
    assert_eq!(quotes.code, "WATCHLIST_QUOTES_UNAVAILABLE");
    assert_eq!(quotes.status, 503);
    assert_eq!(
        recorder.reads.lock().expect("watchlist reads").len(),
        3,
        "the rejected limit/quotes inputs must not reach the watchlist port"
    );

    let unknown = executor
        .execute_production("watchlist.list", &json!({"group": "missing"}))
        .expect_err("an unknown group must fail closed");
    assert_eq!(unknown.status, 404, "{unknown:?}");
    assert_eq!(unknown.code, "WATCHLIST_NOT_FOUND");
    assert_eq!(
        recorder.reads.lock().expect("watchlist reads").len(),
        4,
        "an unknown group is only resolved, never listed"
    );
}

#[derive(Debug)]
struct TimelineExecutionRead {
    query_fails: bool,
}

impl ExecutionReadSnapshotPort for TimelineExecutionRead {
    fn read(&self, path: &str, query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if self.query_fails {
            return Err(ExecutionReadSnapshotError::Unavailable(
                "execution store unavailable".to_owned(),
            ));
        }
        if path == "/api/v1/execution/orders" {
            assert_eq!(query, "scope=CURRENT");
            return Ok(json!({"orders": [{"internalOrderId": "order-1"}]}));
        }
        assert_eq!(path, "/api/v1/execution/orders/order%2D1/events");
        assert_eq!(query, "");
        Ok(json!({"internalOrderId": "order-1", "events": [{"event": "accepted"}]}))
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:707
/// `TestExecutionReadToolsPropagateProjectionFailures`.
///
/// Go's `execution.order_events` answers with the execution order list when no
/// identifier is given and with the event timeline otherwise, and both shapes
/// propagate the projection error unchanged. Rust keeps the same two paths and
/// maps the unavailable port to a structured 503 carrying the port message.
#[test]
fn execution_order_events_lists_orders_and_propagates_projection_failures() {
    let (_directory, ports) = bundle_with(|ports| {
        let port: Arc<dyn ExecutionReadSnapshotPort> =
            Arc::new(TimelineExecutionRead { query_fails: false });
        ports.execution_read = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let list = executor
        .execute_production("execution.order_events", &json!({}))
        .expect("order list");
    assert_eq!(list["orders"][0]["internalOrderId"], "order-1");

    let timeline = executor
        .execute_production(
            "execution.order_events",
            &json!({"internalOrderId": "order-1"}),
        )
        .expect("order timeline");
    assert_eq!(timeline["events"][0]["event"], "accepted");

    let (_directory, ports) = bundle_with(|ports| {
        let port: Arc<dyn ExecutionReadSnapshotPort> =
            Arc::new(TimelineExecutionRead { query_fails: true });
        ports.execution_read = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    for arguments in [json!({}), json!({"internalOrderId": "order-1"})] {
        let failure = executor
            .execute_production("execution.order_events", &arguments)
            .expect_err("projection failures must propagate");
        assert_eq!(failure.code, "EXECUTION_UNAVAILABLE");
        assert_eq!(failure.status, 503);
        assert!(failure.message.contains("execution store unavailable"));
    }
    let account_failure = executor
        .execute_production("account.orders", &json!({"tradingEnvironment": "REAL"}))
        .expect_err("account order projection failures must propagate");
    assert_eq!(account_failure.code, "EXECUTION_UNAVAILABLE");
    assert!(
        account_failure
            .message
            .contains("execution store unavailable")
    );
}

#[derive(Debug)]
struct ScriptedKlineSync {
    statuses: Mutex<Vec<Option<Value>>>,
    reads: Mutex<usize>,
}

impl ScriptedKlineSync {
    fn new(statuses: Vec<Option<Value>>) -> Self {
        Self {
            statuses: Mutex::new(statuses),
            reads: Mutex::new(0),
        }
    }

    fn reads(&self) -> usize {
        *self.reads.lock().expect("sync reads")
    }
}

impl BacktestSyncReadSnapshotPort for ScriptedKlineSync {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        let mut reads = self.reads.lock().expect("sync reads");
        *reads += 1;
        let index = (*reads).saturating_sub(1);
        let statuses = self.statuses.lock().expect("sync statuses");
        let observed = if index < statuses.len() {
            statuses[index].clone()
        } else {
            statuses.last().cloned().flatten()
        };
        Ok(observed.map(|mut payload| {
            payload["taskId"] = json!(task_id);
            payload
        }))
    }

    fn active_tasks(&self) -> Result<Vec<Value>, BacktestSyncReadSnapshotError> {
        Ok(Vec::new())
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:159
/// `TestADKRuntimePollingAndPayloadHelpers` (K-line sync polling).
///
/// Go's `waitForADKKLineSyncProgress` polls every 250ms until the task reaches
/// `completed`/`failed`/`cancelled`, returns the last observation when the wait
/// budget expires, and reports the task as missing as soon as the lookup
/// disappears. The production readiness payload tells the model to call this
/// tool with `waitForCompletionMs: 25000`, so the wait cannot be ignored.
#[test]
fn kline_sync_status_waits_for_a_terminal_status_within_the_requested_window() {
    let scripted: Arc<ScriptedKlineSync> = Arc::new(ScriptedKlineSync::new(vec![
        Some(json!({"status": "running", "symbol": "US.AAPL"})),
        Some(json!({"status": "completed", "symbol": "US.AAPL"})),
    ]));
    let (_directory, ports) = bundle_with(|ports| {
        let scripted_handle = Arc::clone(&scripted);

        let port: Arc<dyn BacktestSyncReadSnapshotPort> = scripted_handle;
        ports.backtest_sync = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));

    let started = Instant::now();
    let waited = executor
        .execute_production(
            "backtest.kline_sync_status",
            &json!({"taskId": "sync-1", "waitForCompletionMs": 5_000}),
        )
        .expect("waited sync status");
    let elapsed = started.elapsed();
    assert_eq!(waited["status"], "completed");
    assert_eq!(waited["readyToRetry"], true);
    assert_eq!(waited["taskId"], "sync-1");
    assert!(scripted.reads() >= 2, "the wait must poll the port again");
    assert!(
        elapsed >= Duration::from_millis(50),
        "the wait must actually wait, elapsed = {elapsed:?}"
    );

    let terminal_first: Arc<ScriptedKlineSync> = Arc::new(ScriptedKlineSync::new(vec![Some(
        json!({"status": "cancelled"}),
    )]));
    let (_directory, ports) = bundle_with(|ports| {
        let terminal_first_handle = Arc::clone(&terminal_first);

        let port: Arc<dyn BacktestSyncReadSnapshotPort> = terminal_first_handle;
        ports.backtest_sync = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    let immediate = executor
        .execute_production(
            "backtest.kline_sync_status",
            &json!({"taskId": "sync-2", "waitForCompletionMs": 5_000}),
        )
        .expect("terminal sync status");
    assert_eq!(immediate["status"], "cancelled");
    assert_eq!(immediate["readyToRetry"], false);
    assert_eq!(terminal_first.reads(), 1);

    let bounded: Arc<ScriptedKlineSync> = Arc::new(ScriptedKlineSync::new(vec![Some(
        json!({"status": "running"}),
    )]));
    let (_directory, ports) = bundle_with(|ports| {
        let bounded_handle = Arc::clone(&bounded);

        let port: Arc<dyn BacktestSyncReadSnapshotPort> = bounded_handle;
        ports.backtest_sync = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    let started = Instant::now();
    let still_running = executor
        .execute_production(
            "backtest.kline_sync_status",
            &json!({"taskId": "sync-3", "waitForCompletionMs": 120}),
        )
        .expect("bounded sync status");
    assert_eq!(still_running["status"], "running");
    assert_eq!(still_running["readyToRetry"], false);
    assert!(started.elapsed() >= Duration::from_millis(100));
    let reads_after_wait = bounded.reads();
    assert!(reads_after_wait >= 2);

    let no_wait = executor
        .execute_production("backtest.kline_sync_status", &json!({"taskId": "sync-3"}))
        .expect("sync status without a wait");
    assert_eq!(no_wait["status"], "running");
    assert_eq!(
        bounded.reads(),
        reads_after_wait + 1,
        "a zero wait reads once and answers"
    );

    let disappears: Arc<ScriptedKlineSync> = Arc::new(ScriptedKlineSync::new(vec![
        Some(json!({"status": "running"})),
        None,
    ]));
    let (_directory, ports) = bundle_with(|ports| {
        let disappears_handle = Arc::clone(&disappears);

        let port: Arc<dyn BacktestSyncReadSnapshotPort> = disappears_handle;
        ports.backtest_sync = port;
    });
    let executor = ProductionMcpToolExecutor::from_production_ports(Arc::clone(&ports));
    let missing = executor
        .execute_production(
            "backtest.kline_sync_status",
            &json!({"taskId": "sync-1", "waitForCompletionMs": 5_000}),
        )
        .expect_err("a task that disappears mid-wait fails closed");
    assert_eq!(missing.code, "BACKTEST_SYNC_TASK_NOT_FOUND");
}
