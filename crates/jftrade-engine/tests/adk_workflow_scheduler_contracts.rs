#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use jftrade_engine::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use jftrade_engine::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_engine::product::{
    MarketDataQuoteReadFuture, MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort,
};
use jftrade_engine::product_workflow_scheduler::WorkflowScheduler;
use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore, initialize_current};
use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

fn initialize_db(path: &Path, component: &str) {
    File::create(path).expect("create database file");
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

#[derive(Debug, Default)]
struct MockQuotePort {
    quotes: Mutex<BTreeMap<String, Value>>,
}

impl MockQuotePort {
    fn set_quote(&self, path: &str, snapshot: Value) {
        self.quotes
            .lock()
            .unwrap()
            .insert(path.to_owned(), snapshot);
    }
}

impl MarketDataQuoteReadSnapshotPort for MockQuotePort {
    fn read<'a>(&'a self, path: &'a str, _query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        let result = self
            .quotes
            .lock()
            .unwrap()
            .get(path)
            .cloned()
            .ok_or_else(|| {
                MarketDataQuoteReadSnapshotError::Unavailable(format!("snapshot not found: {path}"))
            });
        Box::pin(std::future::ready(result))
    }
}

struct TestCluster {
    _dir: tempfile::TempDir,
    store: Arc<AdkStore>,
    port: Arc<ProductionAdkPort>,
    mock_quote: Arc<MockQuotePort>,
}

impl TestCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");
        let settings_path = dir.path().join("settings.json");

        initialize_db(&adk_path, "adk");
        initialize_db(&session_path, "adk-session");
        initialize_db(&artifact_path, "adk-artifact");

        let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&artifact_path).expect("open artifact store"));

        let port = Arc::new(ProductionAdkPort::new_for_test(
            Arc::clone(&store),
            session_store,
            artifact_store,
            settings_path,
        ));

        let mock_quote = Arc::new(MockQuotePort::default());

        Self {
            _dir: dir,
            store,
            port,
            mock_quote,
        }
    }

    fn create_agent(&self, name: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({ "name": name }),
            webhook_secret: None,
        };
        let res = self.port.mutate(&input).expect("create agent");
        res["id"].as_str().expect("agent id").to_owned()
    }

    fn create_workflow(&self, agent_id: &str, name: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflow,
            identifiers: BTreeMap::new(),
            body: json!({
                "name": name,
                "agentId": agent_id,
                "promptTemplate": "execute workflow"
            }),
            webhook_secret: None,
        };
        let res = self.port.mutate(&input).expect("create workflow");
        res["id"].as_str().expect("workflow id").to_owned()
    }

    fn create_trigger(&self, workflow_id: &str, body: Value) -> String {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflowTrigger,
            identifiers,
            body,
            webhook_secret: None,
        };
        let res = self.port.mutate(&input).expect("create trigger");
        res.get("trigger")
            .and_then(|t| t.get("id"))
            .or_else(|| res.get("id"))
            .and_then(|v| v.as_str())
            .expect("trigger id")
            .to_owned()
    }

    fn update_trigger(&self, workflow_id: &str, trigger_id: &str, body: Value) {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id.to_owned());
        identifiers.insert("triggerId".to_owned(), trigger_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::UpdateWorkflowTrigger,
            identifiers,
            body,
            webhook_secret: None,
        };
        self.port.mutate(&input).expect("update trigger");
    }
}

#[test]
fn test_schedule_trigger_creation_and_next_run_calculation() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("cron-agent");
    let workflow_id = cluster.create_workflow(&agent_id, "daily-scan");

    // 1. Create schedule trigger: nextRunAt should be automatically computed
    let trigger_id = cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "schedule",
            "title": "every day noon",
            "config": {
                "cron": "0 12 * * *",
                "timezone": "Asia/Shanghai"
            }
        }),
    );

    let stored = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .expect("get trigger")
        .expect("trigger exists");
    assert!(
        !stored.next_run_at.is_empty(),
        "next_run_at should be computed on creation"
    );
    let payload: Value = serde_json::from_str(&stored.payload_json).unwrap();
    assert_eq!(payload["nextRunAt"].as_str().unwrap(), stored.next_run_at);

    // 2. Update trigger with different cron: nextRunAt should recompute
    cluster.update_trigger(
        &workflow_id,
        &trigger_id,
        json!({
            "config": {
                "cron": "*/15 * * * *",
                "timezone": "UTC"
            }
        }),
    );

    let updated = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .expect("get trigger")
        .unwrap();
    assert!(!updated.next_run_at.is_empty());

    // 3. Disable trigger: nextRunAt should be cleared
    cluster.update_trigger(
        &workflow_id,
        &trigger_id,
        json!({
            "status": "DISABLED"
        }),
    );

    let disabled = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .expect("get trigger")
        .unwrap();
    assert!(
        disabled.next_run_at.is_empty(),
        "next_run_at should be empty when disabled"
    );

    // 4. Re-enable trigger: nextRunAt should be restored
    cluster.update_trigger(
        &workflow_id,
        &trigger_id,
        json!({
            "status": "ENABLED"
        }),
    );

    let reenabled = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .expect("get trigger")
        .unwrap();
    assert!(
        !reenabled.next_run_at.is_empty(),
        "next_run_at should be restored when re-enabled"
    );
}

#[tokio::test]
async fn test_schedule_trigger_tick_advances_and_fires() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("cron-agent-2");
    let workflow_id = cluster.create_workflow(&agent_id, "hourly-report");

    let trigger_id = cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "schedule",
            "title": "daily noon",
            "config": {
                "cron": "0 12 * * *",
                "timezone": "Asia/Shanghai"
            }
        }),
    );

    // Artificially set next_run_at in the past
    let past_iso = "2026-09-08T04:00:00Z";
    cluster
        .store
        .update_workflow_trigger_run_state(&trigger_id, "", past_iso, None)
        .expect("set past run");

    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
    );

    // Tick at 2026-09-08T04:00:05Z (after past_iso)
    let tick_now = OffsetDateTime::parse("2026-09-08T04:00:05Z", &Rfc3339).unwrap();
    let tick_result = scheduler.tick(tick_now).await;

    assert_eq!(tick_result.schedule_triggers_evaluated, 1);
    assert_eq!(tick_result.schedule_triggers_fired, 1);
    assert!(tick_result.errors.is_empty());

    // Verify stored state in SQLite
    let row = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&row.payload_json).unwrap();

    assert_eq!(payload["lastRunAt"], "2026-09-08T04:00:05Z");
    assert_ne!(
        row.next_run_at, past_iso,
        "nextRunAt must advance to the next scheduled run"
    );
    assert!(row.next_run_at.as_str() > "2026-09-08T04:00:05Z");

    // Ticking again immediately at same time should find 0 due triggers
    let tick_result_2 = scheduler.tick(tick_now).await;
    assert_eq!(tick_result_2.schedule_triggers_evaluated, 0);
    assert_eq!(tick_result_2.schedule_triggers_fired, 0);
}

#[tokio::test]
async fn test_schedule_trigger_disabled_parent_workflow_safely_advances() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("cron-agent-3");
    let workflow_id = cluster.create_workflow(&agent_id, "disabled-wf");

    let trigger_id = cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "schedule",
            "title": "daily run",
            "config": {
                "cron": "0 12 * * *",
                "timezone": "Asia/Shanghai"
            }
        }),
    );

    // Disable parent workflow
    let mut identifiers = BTreeMap::new();
    identifiers.insert("workflowId".to_owned(), workflow_id.clone());
    cluster
        .port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateWorkflow,
            identifiers,
            body: json!({ "status": "DISABLED" }),
            webhook_secret: None,
        })
        .expect("disable workflow");

    // Set trigger next_run_at in the past
    let past_iso = "2026-09-08T04:00:00Z";
    cluster
        .store
        .update_workflow_trigger_run_state(&trigger_id, "", past_iso, None)
        .unwrap();

    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
    );

    let tick_now = OffsetDateTime::parse("2026-09-08T04:00:05Z", &Rfc3339).unwrap();
    let tick_result = scheduler.tick(tick_now).await;

    assert_eq!(tick_result.schedule_triggers_evaluated, 1);
    assert_eq!(
        tick_result.schedule_triggers_fired, 0,
        "must NOT fire invocation for disabled workflow"
    );

    // Check that lastError is recorded and nextRunAt advances so it doesn't loop
    let row = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&row.payload_json).unwrap();
    assert!(
        payload["lastError"]
            .as_str()
            .unwrap()
            .contains("disabled or missing")
    );
    assert!(row.next_run_at.as_str() > "2026-09-08T04:00:05Z");
}

#[tokio::test]
async fn test_market_threshold_trigger_cross_up_and_cooldown() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("market-agent");
    let workflow_id = cluster.create_workflow(&agent_id, "price-alert-wf");

    let trigger_id = cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "market_threshold",
            "title": "AAPL Crosses 150",
            "config": {
                "instrumentIds": ["US.AAPL"],
                "path": "lastPrice",
                "edge": "cross_up",
                "value": 150.0,
                "threshold": 150.0,
                "cooldownSec": 60
            }
        }),
    );

    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
    );

    let quote_path = "/api/v1/market-data/snapshots/US/AAPL";

    // Tick 1 at T0 (12:00:00): initial quote at 145.0 (below threshold)
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "lastPrice": 145.0 }));
    let t0 = OffsetDateTime::parse("2026-09-08T12:00:00Z", &Rfc3339).unwrap();
    let res1 = scheduler.tick(t0).await;

    assert_eq!(res1.threshold_triggers_evaluated, 1);
    assert_eq!(
        res1.threshold_triggers_fired, 0,
        "no previous value, cross_up should not fire on first observation"
    );

    // Verify stored lastValues in SQLite
    let row1 = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let p1: Value = serde_json::from_str(&row1.payload_json).unwrap();
    assert_eq!(p1["config"]["state"]["lastValues"]["US.AAPL"], 145.0);

    // Tick 2 at T0 + 10s (12:00:10): price rises to 155.0 (cross_up!)
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "lastPrice": 155.0 }));
    let t1 = OffsetDateTime::parse("2026-09-08T12:00:10Z", &Rfc3339).unwrap();
    let res2 = scheduler.tick(t1).await;

    assert_eq!(res2.threshold_triggers_evaluated, 1);
    assert_eq!(
        res2.threshold_triggers_fired, 1,
        "price crossed up 145 -> 155, must fire"
    );

    let row2 = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let p2: Value = serde_json::from_str(&row2.payload_json).unwrap();
    assert_eq!(p2["config"]["state"]["lastValues"]["US.AAPL"], 155.0);
    assert_eq!(
        p2["config"]["state"]["lastTriggeredAt"]["US.AAPL"],
        "2026-09-08T12:00:10Z"
    );
    assert_eq!(p2["lastRunAt"], "2026-09-08T12:00:10Z");

    // Tick 3 at T0 + 20s (12:00:20): price rises further to 160.0, but within 60s cooldown!
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "lastPrice": 160.0 }));
    let t2 = OffsetDateTime::parse("2026-09-08T12:00:20Z", &Rfc3339).unwrap();
    let res3 = scheduler.tick(t2).await;

    assert_eq!(res3.threshold_triggers_evaluated, 1);
    assert_eq!(
        res3.threshold_triggers_fired, 0,
        "cooldown of 60s must prevent firing"
    );

    // Tick 4 at T0 + 80s (12:01:20): past 60s cooldown. Price dropped back to 148.0 (below).
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "lastPrice": 148.0 }));
    let t3 = OffsetDateTime::parse("2026-09-08T12:01:20Z", &Rfc3339).unwrap();
    let res4 = scheduler.tick(t3).await;
    assert_eq!(
        res4.threshold_triggers_fired, 0,
        "drop below threshold does not fire cross_up"
    );

    // Tick 5 at T0 + 90s (12:01:30): price crosses up again to 152.0!
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "lastPrice": 152.0 }));
    let t4 = OffsetDateTime::parse("2026-09-08T12:01:30Z", &Rfc3339).unwrap();
    let res5 = scheduler.tick(t4).await;
    assert_eq!(
        res5.threshold_triggers_fired, 1,
        "crossed up again after cooldown, must fire"
    );
}

#[tokio::test]
async fn test_market_threshold_cross_down_and_operators() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("cross-down-agent");
    let workflow_id = cluster.create_workflow(&agent_id, "cross-down-wf");

    let trigger_id = cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "market_threshold",
            "title": "AAPL Crosses Down 100",
            "config": {
                "instrumentIds": ["US.AAPL"],
                "path": "nested.price",
                "edge": "cross_down",
                "value": 100.0,
                "threshold": 100.0,
                "cooldownSec": 30
            }
        }),
    );

    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
    );

    let quote_path = "/api/v1/market-data/snapshots/US/AAPL";

    // 1. Initial price: 105.0
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "nested": { "price": 105.0 } }));
    let t0 = OffsetDateTime::parse("2026-09-08T10:00:00Z", &Rfc3339).unwrap();
    scheduler.tick(t0).await;

    // 2. Price falls to 95.0 -> cross_down
    cluster
        .mock_quote
        .set_quote(quote_path, json!({ "nested": { "price": 95.0 } }));
    let t1 = OffsetDateTime::parse("2026-09-08T10:00:10Z", &Rfc3339).unwrap();
    let res = scheduler.tick(t1).await;
    assert_eq!(res.threshold_triggers_fired, 1);

    let row = cluster
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let p: Value = serde_json::from_str(&row.payload_json).unwrap();
    assert_eq!(p["config"]["state"]["lastValues"]["US.AAPL"], 95.0);
}

#[tokio::test]
async fn test_market_threshold_quote_error_resilience() {
    let cluster = TestCluster::new();
    let agent_id = cluster.create_agent("resilient-agent");
    let workflow_id = cluster.create_workflow(&agent_id, "resilient-wf");

    // Trigger references symbol that mock quote does not have
    cluster.create_trigger(
        &workflow_id,
        json!({
            "type": "market_threshold",
            "title": "Missing Quote",
            "config": {
                "instrumentIds": ["US.UNKNOWN"],
                "path": "lastPrice",
                "edge": "above",
                "value": 50.0,
                "threshold": 50.0
            }
        }),
    );

    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
    );

    let t0 = OffsetDateTime::now_utc();
    let res = scheduler.tick(t0).await;

    assert_eq!(res.threshold_triggers_evaluated, 1);
    assert_eq!(res.threshold_triggers_fired, 0);
    assert!(
        res.errors.is_empty(),
        "missing quotes should be safely skipped without errors"
    );
}

#[tokio::test]
async fn test_workflow_scheduler_worker_start_stop_and_status() {
    let cluster = TestCluster::new();

    let scheduler = WorkflowScheduler::start(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(Arc::clone(&cluster.mock_quote) as Arc<dyn MarketDataQuoteReadSnapshotPort>),
        Duration::from_millis(40),
    );

    let status = scheduler.status();
    assert_eq!(status.state, "ready");

    // Allow background loop to tick
    tokio::time::sleep(Duration::from_millis(150)).await;

    let updated_status = scheduler.status();
    assert!(
        updated_status.ticks >= 2,
        "scheduler background worker must have ticked at least twice, got {}",
        updated_status.ticks
    );

    scheduler.stop();
    assert!(scheduler.join_invocations(Duration::from_secs(1)));

    let stopped_status = scheduler.status();
    assert_eq!(stopped_status.state, "stopped");
}
