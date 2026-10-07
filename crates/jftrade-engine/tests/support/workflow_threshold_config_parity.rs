use super::*;

#[derive(Debug)]
struct RecordingQuotes {
    inner: Arc<MockQuotePort>,
    requests: Mutex<Vec<String>>,
}

impl MarketDataQuoteReadSnapshotPort for RecordingQuotes {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        self.requests
            .lock()
            .expect("requests")
            .push(path.to_owned());
        self.inner.read(path, query)
    }
}

#[tokio::test]
async fn threshold_scheduler_polls_normalized_scalar_ids_once_and_persists_each_match() {
    let cluster = TestCluster::new();
    let agent = cluster.create_agent("scalar-threshold");
    let workflow = cluster.create_workflow(&agent, "scalar-threshold");
    let trigger = cluster.create_trigger(&workflow, json!({"type":"market_threshold",
        "config":{"instrumentIds":[" US.AAPL ",700,"us.aapl",""],"value":100,"edge":"above","cooldownSec":0}}));
    for path in [
        "/api/v1/market-data/snapshots/US/700",
        "/api/v1/market-data/snapshots/US/AAPL",
    ] {
        cluster
            .mock_quote
            .set_quote(path, json!({"snapshot":{"price":101}}));
    }
    let quotes = Arc::new(RecordingQuotes {
        inner: Arc::clone(&cluster.mock_quote),
        requests: Mutex::new(Vec::new()),
    });
    let scheduler = WorkflowScheduler::new_for_test(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(quotes.clone()),
    );
    let now = OffsetDateTime::parse("2026-07-01T01:00:00Z", &Rfc3339).expect("time");
    let tick = scheduler.tick(now).await;
    assert!(
        scheduler.join_shutdown(Duration::from_secs(2)).await,
        "queued owner joined"
    );
    assert!(tick.errors.is_empty(), "{:?}", tick.errors);
    assert_eq!(
        *quotes.requests.lock().expect("requests"),
        [
            "/api/v1/market-data/snapshots/US/700",
            "/api/v1/market-data/snapshots/US/AAPL"
        ]
    );
    assert_eq!(tick.threshold_triggers_evaluated, 1);
    assert_eq!(tick.threshold_triggers_fired, 2);
    let stored = cluster
        .store
        .get_workflow_trigger(&trigger)
        .expect("trigger")
        .expect("row");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("payload");
    assert_eq!(
        payload["config"]["state"]["lastValues"],
        json!({"700":101.0,"US.AAPL":101.0})
    );
    assert_eq!(
        payload["config"]["state"]["lastTriggeredAt"],
        json!({"700":"2026-07-01T01:00:00Z","US.AAPL":"2026-07-01T01:00:00Z"})
    );
    let logs = cluster.store.list_workflow_trigger_logs().expect("logs");
    assert_eq!(logs.len(), 2);
    assert!(
        logs.iter()
            .all(|log| log.workflow_id == workflow && log.trigger_id == trigger)
    );
}
