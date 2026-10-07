//! Market-qualified Pine sessions in the real managed strategy loop.
use super::*;

fn original_bar(at: &str, price: f64) -> Value {
    json!({"at":at,"open":price,"close":price,"high":price+1.0,"low":price-1.0,"closed":true})
}

fn original_binding(symbols: &[&str], source: &str) -> Value {
    json!({"script":source,"symbols":symbols,"interval":"1m","executionMode":"notify_only"})
}

fn manager(
    quotes: Arc<dyn MarketDataQuoteReadSnapshotPort>,
    worker: Arc<dyn PineExecutionPort>,
) -> StrategyRuntimeManager {
    let mut manager = StrategyRuntimeManager::new(
        None,
        None,
        Some(quotes),
        None,
        Arc::new(ActiveProviderState::default()),
    );
    manager.worker = Some(worker);
    manager
}

// Parity: go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:226 TestPineWorkerLiveUsesStatefulSessionAfterWarmup
#[test]
fn pine_original_two_bar_warmup_appends_one_candle_and_closes_the_same_market_qualified_session() {
    let (_directory, store) = store();
    let id = "stateful-instance";
    let binding = original_binding(&["US.AAPL"], "//@version=6\nstrategy(\"Stateful\")");
    store
        .seed_instance_with_binding(id, "RUNNING", binding.clone(), "2026-05-28T09:58:00Z")
        .expect("instance");
    let warmup = [
        original_bar("2026-05-28T09:58:00Z", 100.0),
        original_bar("2026-05-28T09:59:00Z", 101.0),
    ];
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":warmup})),
    });
    let worker = Arc::new(Worker::default());
    let manager = manager(quotes.clone(), worker.clone());
    manager
        .spawn_task(id.into(), binding, store.clone())
        .expect("start owner");
    wait_for(|| {
        worker
            .calls
            .lock()
            .expect("calls")
            .iter()
            .any(|r| r.session_operation == "open")
    });
    *quotes.rows.lock().expect("quotes") =
        json!({"candles":[warmup[0],warmup[1],original_bar("2026-05-28T10:00:00Z",102.0)]});
    wait_for(|| {
        worker
            .calls
            .lock()
            .expect("calls")
            .iter()
            .any(|r| r.session_operation == "append")
    });
    assert!(manager.cancel(id), "join before assertions");
    let calls = worker.calls.lock().expect("calls");
    assert_eq!(
        calls
            .iter()
            .map(|r| r.session_operation.as_str())
            .collect::<Vec<_>>(),
        ["open", "append", "close"],
        "no full-history RunScript call or repeated append/close"
    );
    for call in calls.iter() {
        assert_eq!(call.session_id, "strategy:stateful-instance:US.AAPL");
        assert_eq!(call.symbol, "US.AAPL");
    }
    assert_eq!(calls[0].candles.len(), 2);
    assert_eq!(
        calls[0].candles.iter().map(|c| c.close).collect::<Vec<_>>(),
        [100.0, 101.0]
    );
    assert_eq!(calls[0].expected_revision, 0);
    assert_eq!(calls[1].candles.len(), 1);
    let closed_at = time::OffsetDateTime::parse(
        "2026-05-28T10:00:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("time")
    .unix_timestamp()
        * 1000;
    assert_eq!(calls[1].candles[0].open_time, closed_at);
    assert_eq!(calls[1].expected_revision, 1);
    assert_eq!(calls[2].expected_revision, 2);
    assert!(calls[2].candles.is_empty());
}

#[derive(Debug, Default)]
struct MarketSessions {
    calls: Mutex<Vec<PineRunRequest>>,
    sessions: Mutex<BTreeMap<String, (String, u64)>>,
}

impl PineExecutionPort for MarketSessions {
    fn run<'a>(&'a self, request: PineRunRequest) -> PineExecutionFuture<'a> {
        self.calls.lock().expect("calls").push(request.clone());
        let mut sessions = self.sessions.lock().expect("sessions");
        let result = match request.session_operation.as_str() {
            "open" if sessions.contains_key(&request.session_id) => Err(
                PineExecutionError::Remote("session already belongs to another market".into()),
            ),
            "open" => {
                sessions.insert(request.session_id.clone(), (request.symbol.clone(), 1));
                Ok(PineRunResult {
                    session_revision: 1,
                    ..Default::default()
                })
            }
            "close" => match sessions.remove(&request.session_id) {
                Some((symbol, revision))
                    if symbol == request.symbol && revision == request.expected_revision =>
                {
                    Ok(PineRunResult {
                        session_revision: revision + 1,
                        ..Default::default()
                    })
                }
                _ => Err(PineExecutionError::Remote(
                    "close lost its own session identity".into(),
                )),
            },
            _ => Err(PineExecutionError::Remote("unexpected request".into())),
        };
        Box::pin(std::future::ready(result))
    }
}

#[test]
fn pine_live_same_symbol_in_two_markets_opens_and_closes_independent_sessions() {
    let (_directory, store) = store();
    let binding = original_binding(
        &["US.AAPL", "HK.AAPL"],
        "//@version=6\nstrategy(\"Markets\")",
    );
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":[original_bar("2026-05-28T09:58:00Z",100.0)]})),
    });
    let worker = Arc::new(MarketSessions::default());
    let manager = manager(quotes, worker.clone());
    manager
        .spawn_task("one".into(), binding, store.clone())
        .expect("start");
    wait_for(|| {
        worker
            .calls
            .lock()
            .expect("calls")
            .iter()
            .filter(|r| r.session_operation == "open")
            .count()
            == 2
    });
    manager.shutdown_with_error().expect("join owner");
    let calls = worker.calls.lock().expect("calls");
    let opens: Vec<_> = calls
        .iter()
        .filter(|r| r.session_operation == "open")
        .collect();
    let closes: Vec<_> = calls
        .iter()
        .filter(|r| r.session_operation == "close")
        .collect();
    assert_eq!(opens.len(), 2);
    assert_eq!(closes.len(), 2);
    assert_ne!(opens[0].session_id, opens[1].session_id);
    for symbol in ["US.AAPL", "HK.AAPL"] {
        let open = opens
            .iter()
            .find(|r| r.symbol == symbol)
            .expect("market opened");
        let close = closes
            .iter()
            .find(|r| r.symbol == symbol)
            .expect("market closed");
        assert_eq!(open.session_id, format!("strategy:one:{symbol}"));
        assert_eq!(close.session_id, open.session_id);
        assert_eq!(close.expected_revision, 1);
    }
    assert!(worker.sessions.lock().expect("sessions").is_empty());
    assert!(
        !store
            .list_audit_events("one")
            .expect("audit")
            .iter()
            .any(|e| e.kind == "RUNTIME_EXITED")
    );
}

#[derive(Debug)]
struct FailedQuotes;
impl MarketDataQuoteReadSnapshotPort for FailedQuotes {
    fn read<'a>(&'a self, _: &'a str, _: &'a str) -> crate::product::MarketDataQuoteReadFuture<'a> {
        Box::pin(std::future::ready(Err(
            crate::product::MarketDataQuoteReadSnapshotError::Unavailable("klines failed".into()),
        )))
    }
}

// Parity: go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:270 TestPineWorkerLiveRemainingConstructorAndWarmupErrors
#[test]
fn pine_original_blank_source_and_warmup_query_failure_never_execute_worker_requests() {
    let (_directory, store) = store();
    let worker = Arc::new(Worker::default());
    let manager = manager(Arc::new(FailedQuotes), worker.clone());
    let before = store.list_audit_events("one").expect("audit");
    let error = manager
        .spawn_task(
            "one".into(),
            original_binding(&["US.AAPL"], " "),
            store.clone(),
        )
        .expect_err("blank script");
    assert!(
        matches!(error, StrategyRuntimeWritePortError::Failed {status:400, ref code,..} if code=="STRATEGY_SCRIPT_REQUIRED")
    );
    assert_eq!(store.list_audit_events("one").expect("audit"), before);
    assert!(worker.calls.lock().expect("calls").is_empty());
    manager
        .spawn_task(
            "one".into(),
            original_binding(&["US.AAPL"], "//@version=6\nstrategy(\"Coverage\")"),
            store.clone(),
        )
        .expect("start");
    wait_for(|| {
        store
            .get_instance("one")
            .expect("row")
            .expect("instance")
            .status
            == "STOPPED"
    });
    manager.shutdown_with_error().expect("join failed task");
    assert!(worker.calls.lock().expect("calls").is_empty());
    let audit = store.list_audit_events("one").expect("audit");
    let failures: Vec<_> = audit
        .iter()
        .filter(|event| event.kind == "RUNTIME_EXITED")
        .collect();
    assert_eq!(failures.len(), 1);
    assert!(failures[0].detail.contains("klines failed"));
}

// Parity: go:452dea11:internal/strategy/liveruntime/pineworker_live_business_test.go:404 TestPineWorkerLiveSessionFailureBoundaries
#[test]
fn pine_session_close_error_is_reported_after_join_and_repeated_shutdown_never_recloses() {
    let (_directory, store) = store();
    let worker = Arc::new(Worker::default());
    *worker.fail_close.lock().expect("fail close") = true;
    let quotes = Arc::new(Quotes {
        rows: Mutex::new(json!({"candles":[original_bar("2026-05-28T09:58:00Z",100.0)]})),
    });
    let manager = manager(quotes, worker.clone());
    manager
        .spawn_task(
            "one".into(),
            original_binding(&["US.AAPL"], "//@version=6\nstrategy(\"Failure\")"),
            store.clone(),
        )
        .expect("start");
    wait_for(|| {
        store
            .list_audit_events("one")
            .expect("audit")
            .iter()
            .any(|e| e.kind == "PINE_SESSION_CHECKPOINT")
    });
    let first = manager.shutdown_with_error().expect_err("close error");
    assert!(first.contains("close failed"));
    let second = manager
        .shutdown_with_error()
        .expect_err("cached close error");
    assert_eq!(first, second);
    let calls = worker.calls.lock().expect("calls");
    let closes: Vec<_> = calls
        .iter()
        .filter(|r| r.session_operation == "close")
        .collect();
    assert_eq!(closes.len(), 1);
    assert_eq!(closes[0].session_id, "strategy:one:US.AAPL");
    assert_eq!(closes[0].expected_revision, 1);
    assert_eq!(
        store
            .list_audit_events("one")
            .expect("audit")
            .iter()
            .filter(|e| e.kind == "SESSION_CLOSE_FAILED")
            .count(),
        1
    );
}
