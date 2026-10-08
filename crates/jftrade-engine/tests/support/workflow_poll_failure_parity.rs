use super::*;

#[derive(Debug, Default)]
struct PollingQuotesWithFailure {
    paths: Mutex<Vec<String>>,
    numeric_price: bool,
}

impl MarketDataQuoteReadSnapshotPort for PollingQuotesWithFailure {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        assert!(query.is_empty());
        self.paths.lock().unwrap().push(path.to_owned());
        let result = match path {
            "/api/v1/market-data/snapshots/US/BAD" => Err(
                MarketDataQuoteReadSnapshotError::Unavailable("quote canceled".to_owned()),
            ),
            "/api/v1/market-data/snapshots/US/AAPL" => Ok(if self.numeric_price {
                json!({"snapshot":{"price":99.0}})
            } else {
                json!({"snapshot":{}})
            }),
            _ => panic!("unexpected market read {path}"),
        };
        Box::pin(std::future::ready(result))
    }
}

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:208 TestWorkflowSchedulerTickAndMarketPollingStablePaths
#[tokio::test]
async fn market_poll_persists_read_failure_without_discarding_successful_instruments() {
    assert_poll_failure_is_durable(false).await;
}

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:208 TestWorkflowSchedulerTickAndMarketPollingStablePaths
#[tokio::test]
async fn market_poll_persists_read_failure_when_threshold_values_are_unchanged() {
    assert_poll_failure_is_durable(true).await;
}

async fn assert_poll_failure_is_durable(unchanged: bool) {
    let cluster = TestCluster::new();
    let agent = cluster.create_agent("market polling agent");
    let workflow = cluster.create_workflow(&agent, "workflow-scheduler-market");
    let mut config = json!({
        "instrumentIds":["US.BAD","US.AAPL"],
        "snapshotPath":"snapshot.price", "value":100, "edge":"cross_up"
    });
    if unchanged {
        config["state"] = json!({"lastValues":{"US.AAPL":99.0}});
    }
    let trigger = cluster.create_trigger(
        &workflow,
        json!({
            "type":"market_threshold", "status":"ENABLED", "title":"Market poll", "config":config
        }),
    );
    let before: Value = serde_json::from_str(
        &cluster
            .store
            .get_workflow_trigger(&trigger)
            .unwrap()
            .unwrap()
            .payload_json,
    )
    .unwrap();
    let quote = Arc::new(PollingQuotesWithFailure {
        numeric_price: !unchanged,
        ..Default::default()
    });
    let scheduler = WorkflowScheduler::new_for_test(
        cluster.store.clone(),
        cluster.port.clone(),
        Some(quote.clone()),
    );
    let now = OffsetDateTime::parse("2026-07-01T00:00:05Z", &Rfc3339).unwrap();
    let tick = scheduler.tick(now).await;
    assert_eq!(tick.threshold_triggers_evaluated, 1);
    assert_eq!(tick.threshold_triggers_fired, 0);
    assert!(
        tick.errors.is_empty(),
        "ordinary quote failure must not stop scheduler: {:?}",
        tick.errors
    );
    assert_eq!(
        *quote.paths.lock().unwrap(),
        [
            "/api/v1/market-data/snapshots/US/AAPL",
            "/api/v1/market-data/snapshots/US/BAD"
        ]
    );
    assert!(
        cluster
            .store
            .list_workflow_trigger_logs()
            .unwrap()
            .is_empty()
    );
    let row = cluster
        .store
        .get_workflow_trigger(&trigger)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&row.payload_json).unwrap();
    assert!(
        payload["lastError"]
            .as_str()
            .unwrap_or_default()
            .contains("quote canceled"),
        "{payload}"
    );
    assert_eq!(payload["config"]["state"]["lastValues"]["US.AAPL"], 99.0);
    if unchanged {
        assert_eq!(
            payload["config"], before["config"],
            "threshold config must remain unchanged"
        );
    }
    assert_eq!(payload["lastRunAt"].as_str().unwrap_or_default(), "");
    assert!(scheduler.join_invocations(Duration::from_secs(1)));
}
