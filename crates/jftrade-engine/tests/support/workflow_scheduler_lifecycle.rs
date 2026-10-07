use super::*;

#[derive(Debug)]
struct CleanupQuotePort {
    entered: tokio::sync::Notify,
    cancelled: tokio::sync::Notify,
    release: Mutex<std::sync::mpsc::Receiver<()>>,
}

struct CleanupGuard<'a>(&'a CleanupQuotePort);

impl Drop for CleanupGuard<'_> {
    fn drop(&mut self) {
        self.0.cancelled.notify_one();
        self.0
            .release
            .lock()
            .unwrap()
            .recv_timeout(Duration::from_secs(2))
            .expect("release cancelled snapshot cleanup");
    }
}

impl MarketDataQuoteReadSnapshotPort for CleanupQuotePort {
    fn read<'a>(&'a self, _path: &'a str, _query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            let _cleanup = CleanupGuard(self);
            self.entered.notify_one();
            std::future::pending().await
        })
    }
}

fn start_blocked_scheduler(
    cluster: &TestCluster,
) -> (
    Arc<WorkflowScheduler>,
    Arc<CleanupQuotePort>,
    std::sync::mpsc::Sender<()>,
) {
    let agent = cluster.create_agent("scheduler-lifecycle");
    let workflow = cluster.create_workflow(&agent, "scheduler-lifecycle");
    cluster.create_trigger(
        &workflow,
        json!({
            "type": "market_threshold",
            "config": {
                "instrumentIds": ["US.AAPL"],
                "snapshotPath": "snapshot.price",
                "value": 100,
                "threshold": 100,
                "operator": "gt",
                "edge": "above"
            }
        }),
    );
    let (release, rx) = std::sync::mpsc::channel();
    let quote = Arc::new(CleanupQuotePort {
        entered: Default::default(),
        cancelled: Default::default(),
        release: Mutex::new(rx),
    });
    let scheduler = WorkflowScheduler::start(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        Some(quote.clone()),
        Duration::from_secs(3600),
    );
    (scheduler, quote, release)
}

// Parity: internal/assistant/workflow_lifecycle_test.go:98 TestWorkflowSchedulerStopCancelsAndJoinsInFlightTick
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn scheduler_join_reports_pending_cancelled_snapshot_cleanup() {
    let cluster = TestCluster::new();
    let (scheduler, quote, release) = start_blocked_scheduler(&cluster);
    tokio::time::timeout(Duration::from_secs(1), quote.entered.notified())
        .await
        .expect("scheduler entered snapshot");
    scheduler.stop();
    tokio::time::timeout(Duration::from_secs(1), quote.cancelled.notified())
        .await
        .expect("snapshot observed cancellation");
    let joined_before_cleanup = scheduler.join_invocations(Duration::from_millis(20));
    release.send(()).expect("release cleanup");
    assert!(scheduler.join_shutdown(Duration::from_secs(1)).await);
    assert!(
        !joined_before_cleanup,
        "join must retain the in-flight tick owner"
    );
    assert!(scheduler.join_invocations(Duration::from_secs(1)));
    assert_eq!(scheduler.status().state, "stopped");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn scheduler_async_shutdown_waits_for_cleanup_and_retains_timed_out_owner() {
    let cluster = TestCluster::new();
    let (scheduler, quote, release) = start_blocked_scheduler(&cluster);
    tokio::time::timeout(Duration::from_secs(1), quote.entered.notified())
        .await
        .unwrap();
    scheduler.stop();
    tokio::time::timeout(Duration::from_secs(1), quote.cancelled.notified())
        .await
        .unwrap();
    assert!(!scheduler.join_shutdown(Duration::from_millis(20)).await);
    assert_eq!(scheduler.status().state, "stopping");
    let mut joins = Vec::new();
    for _ in 0..8 {
        let scheduler = Arc::clone(&scheduler);
        joins.push(tokio::spawn(async move {
            scheduler.join_shutdown(Duration::from_secs(1)).await
        }));
    }
    tokio::time::sleep(Duration::from_millis(30)).await;
    let returned_before_cleanup = joins.iter().any(|task| task.is_finished());
    release.send(()).unwrap();
    for join in joins {
        assert!(join.await.unwrap());
    }
    assert!(!returned_before_cleanup);
    assert_eq!(scheduler.status().state, "stopped");
    let ticks = scheduler.status().ticks;
    assert_eq!(
        scheduler
            .tick(OffsetDateTime::now_utc())
            .await
            .threshold_triggers_evaluated,
        0
    );
    assert_eq!(scheduler.status().ticks, ticks);
}

#[tokio::test]
async fn scheduler_shutdown_yields_on_current_thread_and_is_idempotent() {
    let cluster = TestCluster::new();
    let scheduler = WorkflowScheduler::start(
        Arc::clone(&cluster.store),
        Arc::clone(&cluster.port),
        None,
        Duration::from_secs(3600),
    );
    scheduler.stop();
    assert!(
        !scheduler.join_invocations(Duration::ZERO),
        "abort has not yet been polled"
    );
    assert!(
        tokio::time::timeout(
            Duration::from_secs(1),
            scheduler.join_shutdown(Duration::from_secs(1))
        )
        .await
        .unwrap()
    );
    assert!(scheduler.join_shutdown(Duration::ZERO).await);
    assert_eq!(scheduler.status().state, "stopped");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn cancelling_a_scheduler_join_keeps_cleanup_owned_for_the_next_waiter() {
    let cluster = TestCluster::new();
    let (scheduler, quote, release) = start_blocked_scheduler(&cluster);
    tokio::time::timeout(Duration::from_secs(1), quote.entered.notified())
        .await
        .unwrap();
    let waiting = Arc::clone(&scheduler);
    let join = tokio::spawn(async move { waiting.join_shutdown(Duration::from_secs(1)).await });
    tokio::time::timeout(Duration::from_secs(1), quote.cancelled.notified())
        .await
        .unwrap();
    join.abort();
    assert!(join.await.unwrap_err().is_cancelled());
    assert!(!scheduler.join_invocations(Duration::ZERO));
    release.send(()).unwrap();
    assert!(scheduler.join_shutdown(Duration::from_secs(1)).await);
}
