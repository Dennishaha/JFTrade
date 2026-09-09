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
