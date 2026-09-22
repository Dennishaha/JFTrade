use super::*;
use jftrade_integration_pine::{
    PineExecutionFuture, PineExecutionPort, PineOrderIntent, PineRunResult,
};
use std::sync::Condvar;
use std::time::{Duration, Instant};

#[derive(Debug, Default)]
struct Quotes {
    rows: Mutex<Value>,
}
impl MarketDataQuoteReadSnapshotPort for Quotes {
    fn read<'a>(&'a self, _: &'a str, _: &'a str) -> crate::product::MarketDataQuoteReadFuture<'a> {
        Box::pin(std::future::ready(Ok(self.rows.lock().unwrap().clone())))
    }
}

#[derive(Debug, Default)]
struct Worker {
    calls: Mutex<Vec<PineRunRequest>>,
    fail_at: Mutex<Option<i64>>,
}
impl PineExecutionPort for Worker {
    fn run<'a>(&'a self, request: PineRunRequest) -> PineExecutionFuture<'a> {
        let at = request
            .candles
            .last()
            .map(|c| c.open_time)
            .unwrap_or_default();
        let append = request.session_operation == "append";
        let mut fail = self.fail_at.lock().unwrap();
        let should_fail = append && *fail == Some(at);
        if should_fail {
            *fail = None;
        }
        self.calls.lock().unwrap().push(request.clone());
        Box::pin(std::future::ready(if should_fail {
            Err(PineExecutionError::Timeout)
        } else {
            Ok(PineRunResult {
                session_revision: request.expected_revision + 1,
                order_intents: if append {
                    vec![PineOrderIntent {
                        kind: "entry".into(),
                        id: "L".into(),
                        direction: "long".into(),
                        time: at,
                        quantity: 1.0,
                        has_quantity: true,
                        ..Default::default()
                    }]
                } else {
                    vec![]
                },
                ..Default::default()
            })
        }))
    }
}

fn bar(minute: u32, closed: bool) -> Value {
    json!({"at":format!("2026-09-08T10:{minute:02}:00Z"),"open":100,"close":100,"high":101,"low":99,"closed":closed})
}
fn binding() -> Value {
    json!({"script":"//@version=5\nindicator('owner')\nplot(close)","symbols":["US.AAPL"],"interval":"1m","executeOrders":false})
}

#[derive(Debug, Default)]
struct Notifications {
    delivered: Mutex<Vec<ProductNotificationRequest>>,
}

impl ProductNotificationPort for Notifications {
    fn deliver(
        &self,
        request: ProductNotificationRequest,
    ) -> crate::product::ProductNotificationDelivery {
        self.delivered.lock().unwrap().push(request);
        crate::product::ProductNotificationDelivery {
            delivered: true,
            status: "sent".to_owned(),
            message: String::new(),
        }
    }
}

/// Pine worker that fails the session-open request: the reference "runtime
/// died before it could trade" path that must reconcile to STOPPED.
#[derive(Debug, Default)]
struct FailingOpenWorker;

impl PineExecutionPort for FailingOpenWorker {
    fn run<'a>(&'a self, _request: PineRunRequest) -> PineExecutionFuture<'a> {
        Box::pin(std::future::ready(Err(PineExecutionError::Remote(
            "pine worker crashed".to_owned(),
        ))))
    }
}
fn store() -> (tempfile::TempDir, Arc<StrategyRuntimeStore>) {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("strategy.db");
    let conn = rusqlite::Connection::open(&path).unwrap();
    jftrade_store_sqlite::initialize_current(&conn, "strategy").unwrap();
    drop(conn);
    let store = Arc::new(StrategyRuntimeStore::open(path).unwrap());
    store
        .seed_instance_with_binding("one", "RUNNING", binding(), "2026-09-08T00:00:00Z")
        .unwrap();
    (dir, store)
}
fn wait_for(mut condition: impl FnMut() -> bool) {
    let deadline = Instant::now() + Duration::from_secs(5);
    while !condition() {
        assert!(Instant::now() < deadline, "runtime did not make progress");
        std::thread::sleep(Duration::from_millis(10));
    }
}

#[test]
fn production_loop_reopens_at_checkpoint_and_replays_every_unprocessed_bar() {
    let (_dir, store) = store();
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":[bar(0,true),bar(1,false)]})),
    });
    let worker = Arc::new(Worker::default());
    let mut manager = StrategyRuntimeManager::new(
        None,
        None,
        Some(quotes.clone()),
        None,
        Arc::new(ActiveProviderState::default()),
    );
    manager.worker = Some(worker.clone());
    manager
        .spawn_task("one".into(), binding(), store.clone())
        .unwrap();
    wait_for(|| {
        store
            .list_audit_events("one")
            .unwrap()
            .iter()
            .any(|e| e.kind == "PINE_SESSION_CHECKPOINT")
    });
    assert_eq!(worker.calls.lock().unwrap()[0].candles.len(), 1);
    let at1 = time::OffsetDateTime::parse(
        "2026-09-08T10:01:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .unwrap()
    .unix_timestamp()
        * 1000;
    *worker.fail_at.lock().unwrap() = Some(at1);
    *quotes.rows.lock().unwrap() =
        json!({"candles":[bar(0,true),bar(1,true),bar(2,true),bar(3,false)]});
    wait_for(|| {
        store
            .list_audit_events("one")
            .unwrap()
            .iter()
            .filter(|e| e.kind == "SIGNAL_DETECTED")
            .count()
            == 2
    });
    assert!(manager.cancel("one"));
    let calls = worker.calls.lock().unwrap();
    let opens: Vec<_> = calls
        .iter()
        .filter(|r| r.session_operation == "open")
        .collect();
    assert_eq!(opens.len(), 2);
    assert_eq!(
        opens[1].candles.len(),
        1,
        "reopen must not consume unprocessed bars as warmup"
    );
    let appends: Vec<_> = calls
        .iter()
        .filter(|r| r.session_operation == "append")
        .map(|r| r.candles[0].open_time)
        .collect();
    assert_eq!(appends, vec![at1, at1, at1 + 60000]);
    drop(calls);
    *quotes.rows.lock().unwrap() =
        json!({"candles":[bar(0,true),bar(1,true),bar(2,true),bar(3,true),bar(4,false)]});
    manager
        .spawn_task("one".into(), binding(), store.clone())
        .unwrap();
    wait_for(|| {
        store
            .list_audit_events("one")
            .unwrap()
            .iter()
            .filter(|e| e.kind == "SIGNAL_DETECTED")
            .count()
            == 3
    });
    assert!(manager.cancel("one"));
}

#[derive(Debug, Default)]
struct BlockingQuotes {
    state: Mutex<(bool, bool)>,
    cv: Condvar,
}
impl MarketDataQuoteReadSnapshotPort for BlockingQuotes {
    fn read<'a>(&'a self, _: &'a str, _: &'a str) -> crate::product::MarketDataQuoteReadFuture<'a> {
        let mut s = self.state.lock().unwrap();
        s.0 = true;
        self.cv.notify_all();
        while !s.1 {
            s = self.cv.wait(s).unwrap();
        }
        Box::pin(std::future::ready(Ok(json!({"candles":[]}))))
    }
}
#[test]
fn timed_out_stop_keeps_owner_and_releases_store_during_blocking_quote_io() {
    let (dir, store) = store();
    let quote = Arc::new(BlockingQuotes::default());
    let mut manager = StrategyRuntimeManager::new(
        None,
        None,
        Some(quote.clone()),
        None,
        Arc::new(ActiveProviderState::default()),
    );
    manager.worker = Some(Arc::new(Worker::default()));
    manager
        .spawn_task("one".into(), binding(), store.clone())
        .unwrap();
    {
        let mut s = quote.state.lock().unwrap();
        while !s.0 {
            s = quote.cv.wait(s).unwrap();
        }
    }
    assert!(!manager.cancel("one"));
    assert!(manager.is_task_alive("one"));
    drop(store);
    let reopened = StrategyRuntimeStore::open(dir.path().join("strategy.db"))
        .expect("blocked quote must not hold the WriterLease");
    drop(reopened);
    {
        let mut s = quote.state.lock().unwrap();
        s.1 = true;
        quote.cv.notify_all();
    }
    wait_for(|| manager.cancel("one"));
    assert!(!manager.is_task_alive("one"));
}

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_observation_test.go:176 TestStrategyRuntimePanicAutoReconcilesToStopped
// Parity: go:452dea11:internal/strategy/catalog/runtime_reconciliation_business_test.go:52 TestCatalogRuntimeFailureReconcilesOnlyRunningInstance
#[test]
fn runtime_exit_converges_to_stopped_with_audit_notification_and_error_log() {
    let (_dir, store) = store();
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":[bar(0,true),bar(1,false)]})),
    });
    let notifications = Arc::new(Notifications::default());
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    router
        .lock()
        .unwrap()
        .acquire_demand(
            "one",
            [InstrumentRef {
                channel: "KLINE".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: Some("1m".to_owned()),
            }],
            false,
            1_700_000_000_000,
        )
        .expect("seed runtime demand");
    assert_eq!(router.lock().unwrap().demand().logical_count, 1);
    let mut manager = StrategyRuntimeManager::new(
        Some(Arc::clone(&router)),
        None,
        Some(quotes),
        None,
        Arc::new(ActiveProviderState::default()),
    );
    manager.worker = Some(Arc::new(FailingOpenWorker));
    manager.notification = Some(notifications.clone());
    manager
        .spawn_task("one".into(), binding(), store.clone())
        .unwrap();

    wait_for(|| {
        store
            .get_instance("one")
            .unwrap()
            .is_some_and(|instance| instance.status == "STOPPED")
    });

    let observation = store
        .get_observation("one")
        .unwrap()
        .expect("observation after runtime exit");
    assert_eq!(observation.actual_status, "STOPPED");
    assert!(
        observation
            .last_error
            .as_deref()
            .is_some_and(|error| error.contains("pine worker crashed")),
        "observation last error = {:?}",
        observation.last_error
    );

    let audits = store.list_audit_events("one").unwrap();
    let exit = audits
        .iter()
        .find(|event| event.kind == "RUNTIME_EXITED")
        .expect("runtime exit audit entry");
    assert!(
        exit.detail.contains("pine worker crashed"),
        "runtime exit detail = {:?}",
        exit.detail
    );
    let logs = store.list_log_events("one").unwrap();
    assert!(
        logs.iter().any(|event| {
            event.level.eq_ignore_ascii_case("error")
                && event.raw.contains("strategy runtime exited unexpectedly: ")
                && event.raw.contains("pine worker crashed")
        }),
        "runtime exit logs = {logs:?}"
    );

    let delivered = notifications.delivered.lock().unwrap();
    assert_eq!(
        delivered.len(),
        1,
        "runtime exit notifications = {delivered:?}"
    );
    assert_eq!(delivered[0].title, "策略运行异常退出");
    assert!(
        delivered[0].body.contains("pine worker crashed"),
        "notification body = {:?}",
        delivered[0].body
    );
    drop(delivered);
    assert_eq!(
        router.lock().unwrap().demand().logical_count,
        0,
        "a dead runtime must not keep the instance's market-data demand"
    );

    // A second convergence attempt must not rewrite a non-RUNNING instance:
    // the reference `ReconcileRuntimeFailure` returns without saving or
    // appending another audit entry.
    fail_strategy_task(&store, &None, None, "one", &[], "late duplicate".to_owned());
    let audits = store.list_audit_events("one").unwrap();
    assert_eq!(
        audits
            .iter()
            .filter(|event| event.kind == "RUNTIME_EXITED")
            .count(),
        1,
        "runtime exit audits = {audits:?}"
    );
    assert_eq!(
        store.get_instance("one").unwrap().unwrap().status,
        "STOPPED"
    );
}

/// Router that can hold managed leases: an activated streaming Futu provider
/// mirrors the production descriptor the runtime is composed with.
fn lease_router() -> ProviderRouter {
    use jftrade_marketdata::{
        ActivationMode, HealthStatus, ProviderCapabilities, ProviderConstraints,
        ProviderDescriptor, ProviderReadiness, ProviderRouter,
    };
    let mut router = ProviderRouter::new(32);
    router
        .register(
            ProviderDescriptor {
                selection_id: "futu".to_owned(),
                provider_id: "futu-opend".to_owned(),
                display_name: "Futu OpenD".to_owned(),
                broker_id: Some("futu".to_owned()),
                source: "bbgo:futu".to_owned(),
                default_market: "HK".to_owned(),
                supported_markets: vec!["HK".to_owned(), "US".to_owned()],
                transports: vec!["opend-tcp".to_owned()],
                capabilities: ProviderCapabilities {
                    snapshots: true,
                    streaming_quotes: true,
                    streaming_candles: true,
                    streaming_depth: true,
                    historical_candles: true,
                    tick_candles: true,
                    order_book_depth: true,
                    ..ProviderCapabilities::default()
                },
                constraints: ProviderConstraints::default(),
                notes: Vec::new(),
            },
            HealthStatus {
                connected: true,
                stream_mode: "push-stream".to_owned(),
                readiness: ProviderReadiness::Ready,
                ..HealthStatus::default()
            },
        )
        .expect("register futu descriptor");
    router
        .activate("futu", ActivationMode::Explicit)
        .expect("activate futu");
    router
}

/// Parity: go:452dea11:internal/app/apiserver/servercore/strategy_subscription_lifecycle_test.go:12 TestStrategyRuntimeHoldsExactKLineLeasesUntilStopAndClose
///
/// Go starts one runtime over `US.AAPL` and `HK.00700` at `5m`, keeps both
/// exact KLINE leases while the runtime is live (a web-only clear must not
/// drop them), and only releases them on stop and manager close.  Rust's
/// manager runs the same acquire-then-release sequence against the shared
/// demand book, so assert the exact lease set here; the web-clear half is
/// pinned by `clear_route_preserves_running_strategy_lease`.
#[test]
fn strategy_runtime_holds_exact_kline_demand_until_stop_and_shutdown() {
    let (_dir, store) = store();
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":[bar(0,true)]})),
    });
    let worker = Arc::new(Worker::default());
    let router = Arc::new(Mutex::new(lease_router()));
    let mut manager = StrategyRuntimeManager::new(
        Some(Arc::clone(&router)),
        None,
        Some(quotes),
        None,
        Arc::new(ActiveProviderState::default()),
    );
    manager.worker = Some(worker.clone());
    let binding = json!({
        "script": "//@version=5\nindicator('lease')\nplot(close)",
        "symbols": ["US.AAPL", "HK.00700"],
        "interval": "5m",
        "executeOrders": false
    });

    manager
        .acquire_demand("one", &binding)
        .expect("acquire exact K-line demand");
    manager
        .spawn_task("one".to_owned(), binding.clone(), Arc::clone(&store))
        .expect("spawn runtime task");
    wait_for(|| !worker.calls.lock().unwrap().is_empty());

    let lease_keys = {
        let router = router.lock().unwrap();
        let demand = router.demand();
        assert_eq!(demand.logical_count, 2, "strategy demand = {demand:?}");
        let mut keys: Vec<_> = demand
            .entries
            .iter()
            .map(|entry| {
                assert_eq!(entry.consumers, vec!["one".to_owned()]);
                assert_eq!(entry.channel, "KLINE");
                assert_eq!(entry.interval.as_deref(), Some("5m"));
                entry.key.clone()
            })
            .collect();
        keys.sort();
        keys
    };
    assert_eq!(lease_keys, vec!["KLINE:HK:00700:5m", "KLINE:US:AAPL:5m"]);

    // The mutation owner releases the leases when the runtime stops; a
    // stopped runtime must not keep its exact demand alive.
    assert!(manager.cancel("one"));
    manager.release_demand("one");
    assert_eq!(router.lock().unwrap().demand().logical_count, 0);

    // Restarting re-acquires the same exact set, and a manager close releases
    // every lease it still owns.
    manager
        .acquire_demand("one", &binding)
        .expect("re-acquire exact K-line demand");
    manager
        .spawn_task("one".to_owned(), binding, Arc::clone(&store))
        .expect("restart runtime task");
    wait_for(|| worker.calls.lock().unwrap().len() >= 2);
    assert_eq!(router.lock().unwrap().demand().logical_count, 2);
    manager.shutdown();
    wait_for(|| router.lock().unwrap().demand().logical_count == 0);
}
