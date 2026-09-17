//! Parity coverage for Go `pkg/futu/security_snapshot_coordinator_test.go`.

use super::*;
use std::sync::atomic::{AtomicUsize, Ordering};

fn at(seconds: u64) -> SystemTime {
    SystemTime::UNIX_EPOCH + Duration::from_secs(seconds)
}

fn snapshot(symbol: &str, name: &str) -> BrokerSecuritySnapshot {
    BrokerSecuritySnapshot {
        symbol: Some(symbol.to_owned()),
        name: Some(name.to_owned()),
        ..Default::default()
    }
}

fn symbols(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| (*value).to_owned()).collect()
}

/// One-shot latch used to sequence a leader thread and its observer without
/// relying on sleeps or on arrival order.
#[derive(Default)]
struct Gate {
    open: Mutex<bool>,
    signal: Condvar,
}

impl Gate {
    fn open(&self) {
        *self.open.lock().unwrap_or_else(|error| error.into_inner()) = true;
        self.signal.notify_all();
    }

    fn wait(&self) {
        let mut open = self.open.lock().unwrap_or_else(|error| error.into_inner());
        while !*open {
            open = self
                .signal
                .wait(open)
                .unwrap_or_else(|error| error.into_inner());
        }
    }

    fn is_open(&self) -> bool {
        *self.open.lock().unwrap_or_else(|error| error.into_inner())
    }
}

#[test]
fn security_snapshot_coordinator_caches_clones_and_uses_market_batches() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:17
    // TestSecuritySnapshotCoordinatorCachesClonesAndUsesMarketBatches.
    let coordinator = SecuritySnapshotCoordinator::with_settings(
        Arc::new(|| at(1_000)),
        SECURITY_SNAPSHOT_CACHE_TTL,
        SECURITY_SNAPSHOT_CALL_LIMIT,
        SECURITY_SNAPSHOT_CALL_WINDOW,
    );
    let batches = Mutex::new(Vec::new());
    let fetch = |batch: &[String]| {
        batches
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push(batch.to_vec());
        Ok(batch
            .iter()
            .map(|symbol| snapshot(symbol, symbol))
            .collect::<Vec<_>>())
    };

    let query = symbols(&[
        "US.AAPL",
        "HK.00700",
        "SH.600519",
        "SZ.000001",
        "HK.00941",
        "US.AAPL",
    ]);
    let first = coordinator.query(&query, fetch).expect("first query");
    assert_eq!(first.len(), 5, "one entry per canonical symbol");
    let recorded = batches
        .lock()
        .unwrap_or_else(|error| error.into_inner())
        .clone();
    assert_eq!(recorded.len(), 2, "HK and non-HK form two batches");
    // HK is isolated from other markets, and every batch is sorted because the
    // canonical symbol list is sorted before batching.
    assert_eq!(recorded[0], symbols(&["HK.00700", "HK.00941"]));
    assert_eq!(recorded[1], symbols(&["SH.600519", "SZ.000001", "US.AAPL"]));

    // A cached read must hand out a clone: mutating the caller's value cannot
    // change what the next caller receives, and it issues no physical read.
    let mut mutated = first;
    if let Some(entry) = mutated
        .iter_mut()
        .find(|s| s.symbol.as_deref() == Some("US.AAPL"))
    {
        entry.name = Some("mutated".to_owned());
    }
    let second = coordinator.query(&query, fetch).expect("cached query");
    assert_eq!(
        batches
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .len(),
        2,
        "cache hit must not reach the fetch closure"
    );
    assert_eq!(
        second
            .iter()
            .find(|s| s.symbol.as_deref() == Some("US.AAPL"))
            .and_then(|s| s.name.as_deref()),
        Some("US.AAPL"),
        "cached snapshot must not observe caller mutation"
    );
}

#[test]
fn security_snapshot_coordinator_cache_expires_at_the_ttl_boundary() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:17 —
    // Go advances the clock by exactly `securitySnapshotCacheTTL` and expects
    // a fresh read, i.e. the entry is expired at the boundary (`expiresAt`
    // must be *after* now).
    let now = Arc::new(Mutex::new(at(1_000)));
    let handle = Arc::clone(&now);
    let coordinator = SecuritySnapshotCoordinator::with_settings(
        Arc::new(move || *handle.lock().unwrap_or_else(|error| error.into_inner())),
        SECURITY_SNAPSHOT_CACHE_TTL,
        SECURITY_SNAPSHOT_CALL_LIMIT,
        SECURITY_SNAPSHOT_CALL_WINDOW,
    );
    let calls = AtomicUsize::new(0);
    let fetch = |batch: &[String]| {
        calls.fetch_add(1, Ordering::SeqCst);
        Ok(batch
            .iter()
            .map(|symbol| snapshot(symbol, symbol))
            .collect::<Vec<_>>())
    };
    let query = symbols(&["US.AAPL"]);
    coordinator.query(&query, fetch).expect("first query");

    // Last valid instant: still cached.
    *now.lock().unwrap_or_else(|error| error.into_inner()) = at(1_002);
    coordinator.query(&query, fetch).expect("cached query");
    assert_eq!(calls.load(Ordering::SeqCst), 1);

    *now.lock().unwrap_or_else(|error| error.into_inner()) = at(1_003);
    coordinator.query(&query, fetch).expect("expired query");
    assert_eq!(
        calls.load(Ordering::SeqCst),
        2,
        "the TTL boundary is a cache miss in Go"
    );
}

#[test]
fn security_snapshot_coordinator_coalesces_concurrent_requests() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:48
    // TestSecuritySnapshotCoordinatorCoalescesConcurrentRequests.
    //
    // Roles are explicit rather than arrival-ordered: the leader owns the
    // physical read and parks inside the fetch closure, the main thread waits
    // until the follower is provably registered as a coalesced waiter (via the
    // `coalesced_waiters` seam), and only then releases the leader. The
    // one-physical-read assertion therefore never depends on scheduler timing.
    let coordinator = SecuritySnapshotCoordinator::new();
    let calls = Arc::new(AtomicUsize::new(0));
    let read_started = Arc::new(Gate::default());
    let release = Arc::new(Gate::default());
    let query = Arc::new(symbols(&["US.AAPL"]));

    let leader = {
        let coordinator = coordinator.clone();
        let calls = Arc::clone(&calls);
        let read_started = Arc::clone(&read_started);
        let release = Arc::clone(&release);
        let query = Arc::clone(&query);
        std::thread::spawn(move || {
            coordinator.query(&query, |batch| {
                calls.fetch_add(1, Ordering::SeqCst);
                read_started.open();
                release.wait();
                Ok(batch
                    .iter()
                    .map(|symbol| snapshot(symbol, symbol))
                    .collect::<Vec<_>>())
            })
        })
    };

    // The leader now holds the in-flight batch open.
    read_started.wait();
    let follower = {
        let coordinator = coordinator.clone();
        let calls = Arc::clone(&calls);
        let query = Arc::clone(&query);
        std::thread::spawn(move || {
            coordinator.query(&query, |_| {
                calls.fetch_add(1, Ordering::SeqCst);
                panic!("the coalesced follower must not run its own fetch");
            })
        })
    };

    let deadline = Instant::now() + Duration::from_secs(10);
    while coordinator.coalesced_waiters() == 0 {
        assert!(
            Instant::now() < deadline,
            "the follower never joined the in-flight read"
        );
        std::thread::yield_now();
    }
    release.open();

    assert_eq!(
        leader
            .join()
            .expect("leader thread")
            .expect("leader query")
            .len(),
        1
    );
    assert_eq!(
        follower
            .join()
            .expect("follower thread")
            .expect("follower query")
            .len(),
        1
    );
    assert_eq!(
        calls.load(Ordering::SeqCst),
        1,
        "both callers must share one physical 3203 read"
    );
    assert_eq!(
        coordinator.coalesced_waiters(),
        0,
        "waiters must be released"
    );
}

#[test]
fn security_snapshot_coordinator_enforces_sliding_budget_and_does_not_cache_failures() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:83
    // TestSecuritySnapshotCoordinatorEnforcesSlidingBudgetAndDoesNotCacheFailures.
    let now = Arc::new(Mutex::new(at(1_000)));
    let handle = Arc::clone(&now);
    let coordinator = SecuritySnapshotCoordinator::with_settings(
        Arc::new(move || *handle.lock().unwrap_or_else(|error| error.into_inner())),
        SECURITY_SNAPSHOT_CACHE_TTL,
        2,
        SECURITY_SNAPSHOT_CALL_WINDOW,
    );
    let calls = AtomicUsize::new(0);
    let fetch = |batch: &[String]| {
        let attempt = calls.fetch_add(1, Ordering::SeqCst);
        if batch[0] == "US.FAIL" && attempt == 0 {
            return Err(SecuritySnapshotCoordinatorError::Fetch(
                "temporary failure".to_owned(),
            ));
        }
        Ok(batch
            .iter()
            .map(|symbol| snapshot(symbol, symbol))
            .collect::<Vec<_>>())
    };

    let failing = symbols(&["US.FAIL"]);
    assert!(
        coordinator.query(&failing, fetch).is_err(),
        "first failed query must surface"
    );
    // The failure was not cached, so the retry reaches the fetch closure again.
    coordinator
        .query(&failing, fetch)
        .expect("failure must not be cached");

    let third = symbols(&["US.THIRD"]);
    let error = coordinator
        .query(&third, fetch)
        .expect_err("third call must be rate limited");
    match error {
        SecuritySnapshotCoordinatorError::RateLimited { retry_after, .. } => {
            assert_eq!(retry_after, SECURITY_SNAPSHOT_CALL_WINDOW);
        }
        other => panic!("expected rate limit, got {other:?}"),
    }
    assert_eq!(
        calls.load(Ordering::SeqCst),
        2,
        "the rate-limited request must not reach OpenD"
    );

    // Releasing the window admits the next read.
    *now.lock().unwrap_or_else(|error| error.into_inner()) =
        at(1_000) + SECURITY_SNAPSHOT_CALL_WINDOW + Duration::from_millis(1);
    coordinator
        .query(&third, fetch)
        .expect("budget released after the window");
    assert_eq!(calls.load(Ordering::SeqCst), 3);
}

#[test]
fn security_snapshot_coordinator_classifies_remote_rate_limit() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:120
    // TestSecuritySnapshotCoordinatorClassifiesRemoteRateLimitAndHonorsCancellation
    // (rate-limit half).
    let coordinator = SecuritySnapshotCoordinator::new();
    let error = coordinator
        .query(&symbols(&["US.AAPL"]), |_| {
            Err(classify_security_snapshot_fetch_error(
                "获取市场快照频率太高，请求失败，每30秒最多60次",
            ))
        })
        .expect_err("remote quota error must be typed");
    assert!(
        matches!(
            error,
            SecuritySnapshotCoordinatorError::RateLimited { retry_after, .. }
                if retry_after == SECURITY_SNAPSHOT_CALL_WINDOW
        ),
        "OpenD quota text must become the typed rate limit: {error:?}"
    );
    assert_eq!(error.retry_after(), Some(SECURITY_SNAPSHOT_CALL_WINDOW));
}

#[test]
fn security_snapshot_coordinator_honors_cancellation_of_a_coalesced_wait() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:120
    // TestSecuritySnapshotCoordinatorClassifiesRemoteRateLimitAndHonorsCancellation
    // (cancellation half). Go starts the query in a goroutine, waits until the
    // physical read has *started*, calls `cancel()`, and then requires
    // `context.Canceled` from the caller while the leader's read is still in
    // flight. The same ordering is reproduced here with a cancel token, and it
    // also asserts the canceled result is never cached.
    let coordinator = SecuritySnapshotCoordinator::new();
    let read_started = Arc::new(Gate::default());
    let release = Arc::new(Gate::default());
    let canceled_token = SecuritySnapshotCancelToken::new();
    let leader_finished = Arc::new(AtomicUsize::new(0));
    let query = Arc::new(symbols(&["US.AAPL"]));

    // Watchdog: if the cancellation contract regresses, the waiter parks until
    // the leader finishes, which would otherwise hang this test forever.
    // Releasing the gate on a timer turns that into a clean assertion failure.
    // The thread is joined below so it can never outlive the test.
    let watchdog = {
        let release = Arc::clone(&release);
        std::thread::spawn(move || {
            let deadline = Instant::now() + Duration::from_secs(20);
            while !release.is_open() && Instant::now() < deadline {
                std::thread::sleep(Duration::from_millis(5));
            }
            release.open();
        })
    };

    // Leader: owns the physical read and stays inside the fetch closure until
    // the test releases it, mirroring Go's blocked `fetch`.
    let leader = {
        let coordinator = coordinator.clone();
        let read_started = Arc::clone(&read_started);
        let release = Arc::clone(&release);
        let leader_finished = Arc::clone(&leader_finished);
        let query = Arc::clone(&query);
        std::thread::spawn(move || {
            let result = coordinator.query_with_deadline(
                &query,
                |batch| {
                    read_started.open();
                    release.wait();
                    Ok(batch
                        .iter()
                        .map(|symbol| snapshot(symbol, symbol))
                        .collect::<Vec<_>>())
                },
                None,
            );
            leader_finished.store(1, Ordering::SeqCst);
            result
        })
    };
    read_started.wait();

    // Waiter: joins the in-flight batch, then the main thread cancels it.
    let waiter = {
        let coordinator = coordinator.clone();
        let token = canceled_token.clone();
        let query = Arc::clone(&query);
        // The main thread rendezvouses through `coalesced_waiters()` below
        // instead of this thread polling after the fact.
        std::thread::spawn(move || {
            coordinator.query_with_cancel(
                &query,
                |_| panic!("the coalesced waiter must not run its own fetch"),
                &token,
            )
        })
    };

    // Wait until the waiter is parked, exactly like Go waiting on `started`.
    let deadline = Instant::now() + Duration::from_secs(10);
    while coordinator.coalesced_waiters() == 0 {
        assert!(
            Instant::now() < deadline,
            "the waiter never joined the in-flight read"
        );
        std::thread::yield_now();
    }
    canceled_token.cancel();

    let canceled = waiter
        .join()
        .expect("waiter thread")
        .expect_err("a canceled waiter must fail");
    assert_eq!(canceled, SecuritySnapshotCoordinatorError::Canceled);
    assert!(!canceled.is_symbol_scoped());
    assert_eq!(canceled.retry_after(), None);
    assert_eq!(
        coordinator.coalesced_waiters(),
        0,
        "the canceled waiter must not leak a waiter slot"
    );
    // Go's strongest assertion: the caller observed cancellation *while* the
    // physical read was still in flight, not after it had already resolved.
    assert_eq!(
        leader_finished.load(Ordering::SeqCst),
        0,
        "the waiter must return before the leader's read completes"
    );

    // The leader is still running and completes normally; its success is
    // cached, which proves the cancellation did not corrupt shared state.
    release.open();
    assert_eq!(
        leader
            .join()
            .expect("leader thread")
            .expect("leader query")
            .len(),
        1
    );
    watchdog.join().expect("watchdog thread");
    let cached = coordinator
        .query(&query, |_| panic!("the leader's success must be cached"))
        .expect("cached after cancellation");
    assert_eq!(cached.len(), 1);
}

#[test]
fn security_snapshot_coordinator_rejects_invalid_and_unavailable_inputs() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:154
    // TestSecuritySnapshotCoordinatorRejectsUnavailableAndInvalidInputs.
    let coordinator = SecuritySnapshotCoordinator::new();
    // Illegible symbol: rejected before any physical read and flagged as
    // symbol-scoped, matching `broker.NewSymbolScopedSnapshotError`.
    let error = coordinator
        .query(&symbols(&["BAD"]), |_| {
            panic!("invalid input must not fetch")
        })
        .expect_err("invalid symbol must fail");
    assert!(
        matches!(
            error,
            SecuritySnapshotCoordinatorError::InvalidInstrument(_)
        ),
        "{error:?}"
    );
    assert!(error.is_symbol_scoped());
    assert_eq!(error.retry_after(), None);

    // Empty input is a successful empty read, not an error.
    let empty = coordinator
        .query(&[], |_| panic!("empty input must not fetch"))
        .expect("empty query");
    assert!(empty.is_empty());
}

#[test]
fn security_snapshot_coordinator_handles_empty_and_unexpected_results() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator_test.go:189
    // TestSecuritySnapshotCoordinatorHandlesEmptyAndUnexpectedResults.
    let coordinator = SecuritySnapshotCoordinator::new();
    // An empty fetch result is stored as nothing and still succeeds; Go's
    // `store(nil)`/`store({sym: nil})` guards have the same effect.
    let empty = coordinator
        .query(&symbols(&["US.AAPL"]), |_| Ok(Vec::new()))
        .expect("empty fetch result");
    assert!(empty.is_empty());
    assert_eq!(
        coordinator.admitted_calls(),
        1,
        "an empty successful read still consumes one quota slot"
    );

    // Results without a usable symbol are dropped instead of poisoning the
    // cache; the canonical symbol list still drives the response order.
    let nameless = coordinator
        .query(&symbols(&["US.MSFT"]), |_| {
            Ok(vec![BrokerSecuritySnapshot::default()])
        })
        .expect("snapshot without a symbol");
    assert!(nameless.is_empty());

    // Classification helpers keep their nil/replacement semantics.
    assert_eq!(
        classify_security_snapshot_fetch_error("plain transport failure"),
        SecuritySnapshotCoordinatorError::Fetch("plain transport failure".to_owned())
    );
    // Unsorted input still produces the canonical sorted order.
    assert_eq!(
        canonical_snapshot_symbols(&symbols(&[" us.aapl ", "HK.00700", "US.AAPL"]))
            .expect("canonical"),
        symbols(&["HK.00700", "US.AAPL"])
    );
}

#[test]
fn security_snapshot_batches_split_hk_and_other_markets_by_size() {
    // Parity: go:452dea11:pkg/futu/security_snapshot_coordinator.go:110
    // `securitySnapshotBatches`: HK at 20, every other market at 400.
    let hk = (0..21)
        .map(|index| format!("HK.{index:05}"))
        .collect::<Vec<_>>();
    let us = (0..401)
        .map(|index| format!("US.{index:05}"))
        .collect::<Vec<_>>();
    let batches = snapshot_batches(&hk);
    assert_eq!(batches.len(), 2);
    assert_eq!(batches[0].len(), SECURITY_SNAPSHOT_HK_BATCH_SIZE);
    assert_eq!(batches[1].len(), 1);
    let batches = snapshot_batches(&us);
    assert_eq!(batches.len(), 2);
    assert_eq!(batches[0].len(), SECURITY_SNAPSHOT_OTHER_BATCH_SIZE);
    assert_eq!(batches[1].len(), 1);
}
