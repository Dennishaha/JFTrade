//! Shared 3203 snapshot coordinator: cache, market batching, call budget.
//!
//! Parity source: `go:452dea11:pkg/futu/security_snapshot_coordinator.go`.
//! Go keeps a per-*Exchange* coordinator in front of the raw
//! `Qot_GetSecuritySnapshot` (3203) call because that protocol is the most
//! heavily consumed market read in the product: the watchlist, quote routes,
//! order helpers and the reconciliation reads all ask for overlapping symbol
//! sets within the same seconds.  OpenD answers the whole request with a
//! quota error once the operator exceeds roughly 60 calls per 30 seconds, so
//! the coordinator owns four behaviors the raw reader must not:
//!
//! * a 3s TTL cache of successful per-symbol snapshots, handed out as clones
//!   so a caller cannot mutate what the next caller receives;
//! * market-shaped batching (HK 20 per call, every other market 400) so one
//!   logical query cannot burn the quota with a per-symbol fan-out;
//! * single-flight coalescing keyed by the batch, so concurrent identical
//!   requests share one physical read;
//! * a sliding 54-calls-per-30s local gate that fails closed *before* the
//!   socket, reports the exact retry delay, and never caches a failure.

use std::collections::HashMap;
use std::sync::{Arc, Condvar, Mutex};
use std::time::{Duration, Instant, SystemTime};

use jftrade_marketdata::BrokerSecuritySnapshot;
use thiserror::Error;

use crate::OpenDSessionCoordinator;
use crate::security_snapshot_query::market_code;

/// Go `securitySnapshotCacheTTL`.
pub const SECURITY_SNAPSHOT_CACHE_TTL: Duration = Duration::from_secs(3);
/// Go `securitySnapshotCallLimit`.
pub const SECURITY_SNAPSHOT_CALL_LIMIT: usize = 54;
/// Go `securitySnapshotCallWindow`.
pub const SECURITY_SNAPSHOT_CALL_WINDOW: Duration = Duration::from_secs(30);
/// Go `securitySnapshotHKBatchSize`.
pub const SECURITY_SNAPSHOT_HK_BATCH_SIZE: usize = 20;
/// Go `securitySnapshotOtherBatchSize`.
pub const SECURITY_SNAPSHOT_OTHER_BATCH_SIZE: usize = 400;
/// Re-check interval for an observed (cancellable) coalesced wait.
const CANCEL_POLL_INTERVAL: Duration = Duration::from_millis(2);

/// Clock seam so TTL and the sliding window can be exercised deterministically.
pub type SecuritySnapshotClock = Arc<dyn Fn() -> SystemTime + Send + Sync>;

/// Cancellation handle for a coalesced wait.
///
/// Go cancels a snapshot read by canceling the `context.Context`, which can
/// happen at any moment from another goroutine while this caller is parked on
/// the single-flight result. Rust has no context object, so the same contract
/// is expressed as a token any thread may flip; a waiter then returns
/// [`SecuritySnapshotCoordinatorError::Canceled`] promptly. The leader's
/// physical read is deliberately *not* interrupted, exactly as in Go.
#[derive(Clone, Debug, Default)]
pub struct SecuritySnapshotCancelToken {
    canceled: Arc<std::sync::atomic::AtomicBool>,
}

impl SecuritySnapshotCancelToken {
    pub fn new() -> Self {
        Self::default()
    }

    /// Requests cancellation; safe to call from any thread at any time.
    pub fn cancel(&self) {
        self.canceled
            .store(true, std::sync::atomic::Ordering::SeqCst);
    }

    pub fn is_canceled(&self) -> bool {
        self.canceled.load(std::sync::atomic::Ordering::SeqCst)
    }
}

/// Coordinator outcome. `RateLimited` is the only variant the API layer maps
/// to 429/`MARKET_SNAPSHOT_RATE_LIMITED`; everything else fails closed.
#[derive(Clone, Debug, Error, PartialEq, Eq)]
pub enum SecuritySnapshotCoordinatorError {
    /// Go wraps this in `broker.NewSymbolScopedSnapshotError`, so a bad symbol
    /// must not be mistaken for a transport outage.
    #[error("invalid OpenD security snapshot instrument: {0}")]
    InvalidInstrument(String),
    #[error("futu security snapshot coordinator is unavailable")]
    Unavailable,
    #[error("futu security snapshot coordinator returned an invalid result")]
    InvalidResult,
    /// Go's `ctx.Err()` branch: the caller abandoned a coalesced wait. The
    /// leader's physical read keeps running and its outcome is never cached.
    #[error("futu security snapshot query was canceled before it completed")]
    Canceled,
    #[error("{message}")]
    RateLimited {
        retry_after: Duration,
        message: String,
    },
    #[error("{0}")]
    Fetch(String),
}

impl SecuritySnapshotCoordinatorError {
    /// Mirrors `broker.is_symbol_scoped_snapshot_error` for the Rust port.
    pub fn is_symbol_scoped(&self) -> bool {
        matches!(self, Self::InvalidInstrument(_))
    }

    /// Retry hint for the 429 wire response.
    pub fn retry_after(&self) -> Option<Duration> {
        match self {
            Self::RateLimited { retry_after, .. } => Some(*retry_after),
            _ => None,
        }
    }
}

#[derive(Clone, Debug)]
struct CachedSnapshot {
    snapshot: BrokerSecuritySnapshot,
    expires_at: SystemTime,
}

#[derive(Default)]
struct Flight {
    state: Mutex<Option<Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>>>,
    ready: Condvar,
    /// Callers currently sharing this physical read, exposed through
    /// [`SecuritySnapshotCoordinator::coalesced_waiters`] so the single-flight
    /// contract can be asserted without timing assumptions.
    waiters: Mutex<usize>,
}

struct CoordinatorInner {
    cache: Mutex<HashMap<String, CachedSnapshot>>,
    calls: Mutex<Vec<SystemTime>>,
    flights: Mutex<HashMap<String, Arc<Flight>>>,
    clock: SecuritySnapshotClock,
    cache_ttl: Duration,
    call_limit: usize,
    call_window: Duration,
}

impl std::fmt::Debug for CoordinatorInner {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("SecuritySnapshotCoordinator")
            .finish_non_exhaustive()
    }
}

/// Front door for every 3203 read in the product runtime.
#[derive(Clone, Debug)]
pub struct SecuritySnapshotCoordinator {
    inner: Arc<CoordinatorInner>,
}

impl Default for SecuritySnapshotCoordinator {
    fn default() -> Self {
        Self::with_settings(
            Arc::new(SystemTime::now),
            SECURITY_SNAPSHOT_CACHE_TTL,
            SECURITY_SNAPSHOT_CALL_LIMIT,
            SECURITY_SNAPSHOT_CALL_WINDOW,
        )
    }
}

impl SecuritySnapshotCoordinator {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_settings(
        clock: SecuritySnapshotClock,
        cache_ttl: Duration,
        call_limit: usize,
        call_window: Duration,
    ) -> Self {
        Self {
            inner: Arc::new(CoordinatorInner {
                cache: Mutex::new(HashMap::new()),
                calls: Mutex::new(Vec::new()),
                flights: Mutex::new(HashMap::new()),
                clock,
                cache_ttl,
                call_limit,
                call_window,
            }),
        }
    }

    /// Resolves `symbols` from cache and, for everything missing, through
    /// `fetch` in at most one physical read per market batch.
    ///
    /// The returned vector follows Go's canonical order (upper-cased,
    /// de-duplicated, sorted), which is the order the Go adapter re-derives
    /// after the coordinator returns its map.
    pub fn query<F>(
        &self,
        symbols: &[String],
        fetch: F,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        self.query_with_deadline(symbols, fetch, None)
    }

    /// Same as [`Self::query`], but a coalesced caller may abandon its wait at
    /// `deadline` with [`SecuritySnapshotCoordinatorError::Canceled`].
    pub fn query_with_deadline<F>(
        &self,
        symbols: &[String],
        fetch: F,
        deadline: Option<Instant>,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        self.query_observed(symbols, fetch, None, deadline)
    }

    /// Same as [`Self::query`], but a coalesced caller returns
    /// [`SecuritySnapshotCoordinatorError::Canceled`] as soon as `cancel` is
    /// flipped. This is the Rust seam for Go's `context.Context` cancellation.
    pub fn query_with_cancel<F>(
        &self,
        symbols: &[String],
        fetch: F,
        cancel: &SecuritySnapshotCancelToken,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        self.query_observed(symbols, fetch, Some(cancel), None)
    }

    fn query_observed<F>(
        &self,
        symbols: &[String],
        fetch: F,
        cancel: Option<&SecuritySnapshotCancelToken>,
        deadline: Option<Instant>,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        let canonical = canonical_snapshot_symbols(symbols)?;
        if canonical.is_empty() {
            return Ok(Vec::new());
        }
        let (mut result, missing) = self.cached(&canonical);
        for batch in snapshot_batches(&missing) {
            let fetched = self.fetch_batch(&batch, &fetch, cancel, deadline)?;
            for snapshot in fetched {
                let Some(symbol) = snapshot.symbol.as_deref() else {
                    continue;
                };
                let symbol = symbol.trim().to_ascii_uppercase();
                if !symbol.is_empty() {
                    result.insert(symbol, snapshot);
                }
            }
        }
        Ok(canonical
            .into_iter()
            .filter_map(|symbol| result.remove(&symbol))
            .collect())
    }

    /// Number of physical reads admitted inside the current window; used by
    /// the parity tests to prove cache hits never consume the budget.
    pub fn admitted_calls(&self) -> usize {
        self.inner
            .calls
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .len()
    }

    /// Number of coalesced callers currently waiting on an in-progress read.
    pub fn coalesced_waiters(&self) -> usize {
        self.inner
            .flights
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .values()
            .map(|flight| {
                *flight
                    .waiters
                    .lock()
                    .unwrap_or_else(|error| error.into_inner())
            })
            .sum()
    }

    fn cached(&self, symbols: &[String]) -> (HashMap<String, BrokerSecuritySnapshot>, Vec<String>) {
        let now = (self.inner.clock)();
        let mut cache = self
            .inner
            .cache
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let mut result = HashMap::with_capacity(symbols.len());
        let mut missing = Vec::new();
        for symbol in symbols {
            match cache.get(symbol) {
                Some(entry) if entry.expires_at > now => {
                    result.insert(symbol.clone(), entry.snapshot.clone());
                }
                _ => {
                    // Go deletes the expired entry while collecting misses, so
                    // an expired snapshot can never be served again.
                    cache.remove(symbol);
                    missing.push(symbol.clone());
                }
            }
        }
        (result, missing)
    }

    fn fetch_batch<F>(
        &self,
        batch: &[String],
        fetch: &F,
        cancel: Option<&SecuritySnapshotCancelToken>,
        deadline: Option<Instant>,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        let key = batch.join("\u{0}");
        let (flight, leader) = {
            let mut flights = self
                .inner
                .flights
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            match flights.get(&key) {
                Some(existing) => (Arc::clone(existing), false),
                None => {
                    let flight = Arc::new(Flight::default());
                    flights.insert(key.clone(), Arc::clone(&flight));
                    (flight, true)
                }
            }
        };

        if !leader {
            // Coalesced caller: Go's `DoChan` shares the leader's result, so
            // the physical read and the quota slot are consumed exactly once.
            *flight
                .waiters
                .lock()
                .unwrap_or_else(|error| error.into_inner()) += 1;
            let mut state = flight
                .state
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            while state.is_none() {
                if cancel.is_some_and(SecuritySnapshotCancelToken::is_canceled) {
                    Self::release_waiter(&flight);
                    return Err(SecuritySnapshotCoordinatorError::Canceled);
                }
                // A wait is bounded whenever an observer exists, so a token
                // flipped by another thread wakes this caller promptly instead
                // of pinning it until the leader finishes.
                let wait_for = match (cancel, deadline) {
                    (_, Some(deadline)) => {
                        let now = Instant::now();
                        if now >= deadline {
                            Self::release_waiter(&flight);
                            return Err(SecuritySnapshotCoordinatorError::Canceled);
                        }
                        deadline - now
                    }
                    (Some(_), None) => CANCEL_POLL_INTERVAL,
                    (None, None) => {
                        state = flight
                            .ready
                            .wait(state)
                            .unwrap_or_else(|error| error.into_inner());
                        continue;
                    }
                };
                let (next, timeout) = flight
                    .ready
                    .wait_timeout(state, wait_for)
                    .unwrap_or_else(|error| error.into_inner());
                state = next;
                // A timeout only means "re-check the observation"; the caller
                // keeps waiting unless the token or deadline actually fired.
                if timeout.timed_out() && state.is_none() {
                    if cancel.is_some_and(SecuritySnapshotCancelToken::is_canceled) {
                        Self::release_waiter(&flight);
                        return Err(SecuritySnapshotCoordinatorError::Canceled);
                    }
                    if deadline.is_some_and(|deadline| Instant::now() >= deadline) {
                        Self::release_waiter(&flight);
                        return Err(SecuritySnapshotCoordinatorError::Canceled);
                    }
                }
            }
            Self::release_waiter(&flight);
            return state
                .clone()
                .unwrap_or(Err(SecuritySnapshotCoordinatorError::InvalidResult));
        }

        let result = self.run_batch(batch, fetch);
        {
            let mut state = flight
                .state
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            *state = Some(result.clone());
        }
        flight.ready.notify_all();
        self.inner
            .flights
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .remove(&key);
        result
    }

    fn release_waiter(flight: &Flight) {
        let mut waiters = flight
            .waiters
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        *waiters = waiters.saturating_sub(1);
    }

    fn run_batch<F>(
        &self,
        batch: &[String],
        fetch: &F,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>
    where
        F: Fn(&[String]) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError>,
    {
        if let Some(retry_after) = self.take_call() {
            return Err(SecuritySnapshotCoordinatorError::RateLimited {
                retry_after,
                message: format!(
                    "futu security snapshot rate limited; retry after {}ms",
                    retry_after.as_millis()
                ),
            });
        }
        let snapshots = fetch(batch)?;
        // Go stores only after a successful fetch, so failures are retried on
        // the next call instead of being replayed from cache.
        self.store(&snapshots);
        Ok(snapshots)
    }

    fn store(&self, snapshots: &[BrokerSecuritySnapshot]) {
        if snapshots.is_empty() {
            return;
        }
        let expires_at = (self.inner.clock)() + self.inner.cache_ttl;
        let mut cache = self
            .inner
            .cache
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        for snapshot in snapshots {
            let Some(symbol) = snapshot.symbol.as_deref() else {
                continue;
            };
            let symbol = symbol.trim().to_ascii_uppercase();
            if symbol.is_empty() {
                continue;
            }
            cache.insert(
                symbol,
                CachedSnapshot {
                    snapshot: snapshot.clone(),
                    expires_at,
                },
            );
        }
    }

    /// Sliding-window reservation. Returns the retry delay when this call must
    /// be refused without touching OpenD.
    fn take_call(&self) -> Option<Duration> {
        let now = (self.inner.clock)();
        let mut calls = self
            .inner
            .calls
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let cutoff = now
            .checked_sub(self.inner.call_window)
            .unwrap_or(SystemTime::UNIX_EPOCH);
        calls.retain(|call| *call > cutoff);
        if self.inner.call_limit == 0
            || self.inner.call_window.is_zero()
            || calls.len() >= self.inner.call_limit
        {
            let retry_after = calls
                .first()
                .and_then(|first| first.checked_add(self.inner.call_window))
                .and_then(|release| release.duration_since(now).ok())
                .unwrap_or(Duration::from_secs(1));
            return Some(retry_after.max(Duration::from_millis(1)));
        }
        calls.push(now);
        None
    }
}

/// Go `canonicalSecuritySnapshotSymbols`: trim + upper-case, drop duplicates,
/// sort, and reject anything OpenD cannot address.
pub fn canonical_snapshot_symbols(
    symbols: &[String],
) -> Result<Vec<String>, SecuritySnapshotCoordinatorError> {
    let mut canonical = Vec::with_capacity(symbols.len());
    for symbol in symbols {
        let trimmed = symbol.trim();
        let Some((market, code)) = trimmed.split_once('.') else {
            return Err(SecuritySnapshotCoordinatorError::InvalidInstrument(
                symbol.clone(),
            ));
        };
        let Some(_) = market_code(market) else {
            return Err(SecuritySnapshotCoordinatorError::InvalidInstrument(
                symbol.clone(),
            ));
        };
        let code = code.trim().to_ascii_uppercase();
        if code.is_empty() || code.contains('.') {
            return Err(SecuritySnapshotCoordinatorError::InvalidInstrument(
                symbol.clone(),
            ));
        }
        canonical.push(format!("{}.{}", market.trim().to_ascii_uppercase(), code));
    }
    canonical.sort();
    canonical.dedup();
    Ok(canonical)
}

/// Go `securitySnapshotBatches`: HK is split at 20 so a single unusable quote
/// right cannot poison a large batch, every other market is split at 400.
pub fn snapshot_batches(symbols: &[String]) -> Vec<Vec<String>> {
    let mut hk = Vec::new();
    let mut other = Vec::new();
    for symbol in symbols {
        if symbol.starts_with("HK.") {
            hk.push(symbol.clone());
        } else {
            other.push(symbol.clone());
        }
    }
    let mut batches = Vec::new();
    batches.extend(
        hk.chunks(SECURITY_SNAPSHOT_HK_BATCH_SIZE)
            .map(<[String]>::to_vec),
    );
    batches.extend(
        other
            .chunks(SECURITY_SNAPSHOT_OTHER_BATCH_SIZE)
            .map(<[String]>::to_vec),
    );
    batches
}

/// Go `classifySecuritySnapshotError`: OpenD reports its own quota as a plain
/// business error, so the coordinator must promote it to the typed rate limit
/// the API layer already understands.
pub fn classify_security_snapshot_fetch_error(message: &str) -> SecuritySnapshotCoordinatorError {
    let lowered = message.to_lowercase();
    let rate_limited = lowered.contains("每30秒最多60次")
        || lowered.contains("频率太高")
        || lowered.contains("frequency too high")
        || lowered.contains("too many requests");
    if rate_limited {
        return SecuritySnapshotCoordinatorError::RateLimited {
            retry_after: SECURITY_SNAPSHOT_CALL_WINDOW,
            message: message.to_owned(),
        };
    }
    SecuritySnapshotCoordinatorError::Fetch(message.to_owned())
}

/// One physical 3203 read for exactly the symbols it is handed.
///
/// This is Go's `querySecuritySnapshotListDirect` seam: the *coordinator* owns
/// market batching, so the inner reader must not chunk a batch again. Keeping
/// them separate is what makes the quota accounting honest — a 400-symbol
/// non-HK batch is one booked call and one socket write, exactly like Go.
pub trait SecuritySnapshotBatchReader: Send + Sync {
    fn query_batch(&self, symbols: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String>;
}

/// Cached front door for one physical 3203 batch reader.
///
/// The coordinator owns cache, batching, coalescing and the quota gate; the
/// inner reader stays a thin single-call codec.
#[derive(Clone)]
pub struct CachedSecuritySnapshotReader {
    coordinator: SecuritySnapshotCoordinator,
    inner: Arc<dyn SecuritySnapshotBatchReader>,
}

impl std::fmt::Debug for CachedSecuritySnapshotReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("CachedSecuritySnapshotReader")
            .finish_non_exhaustive()
    }
}

impl CachedSecuritySnapshotReader {
    pub fn new(inner: Arc<dyn SecuritySnapshotBatchReader>) -> Self {
        Self {
            coordinator: SecuritySnapshotCoordinator::new(),
            inner,
        }
    }

    pub fn with_coordinator(
        coordinator: SecuritySnapshotCoordinator,
        inner: Arc<dyn SecuritySnapshotBatchReader>,
    ) -> Self {
        Self { coordinator, inner }
    }

    pub fn coordinator(&self) -> &SecuritySnapshotCoordinator {
        &self.coordinator
    }

    pub fn query(
        &self,
        instruments: &[String],
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError> {
        let inner = Arc::clone(&self.inner);
        self.coordinator.query(instruments, move |batch| {
            inner
                .query_batch(batch)
                .map_err(|error| classify_security_snapshot_fetch_error(&error))
        })
    }

    /// Same as [`Self::query`], but a coalesced caller may abandon its wait at
    /// `deadline` with [`SecuritySnapshotCoordinatorError::Canceled`].
    pub fn query_with_deadline(
        &self,
        instruments: &[String],
        deadline: Option<Instant>,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError> {
        let inner = Arc::clone(&self.inner);
        self.coordinator.query_with_deadline(
            instruments,
            move |batch| {
                inner
                    .query_batch(batch)
                    .map_err(|error| classify_security_snapshot_fetch_error(&error))
            },
            deadline,
        )
    }

    /// Same as [`Self::query`], but `cancel` aborts a coalesced wait promptly.
    pub fn query_with_cancel(
        &self,
        instruments: &[String],
        cancel: &SecuritySnapshotCancelToken,
    ) -> Result<Vec<BrokerSecuritySnapshot>, SecuritySnapshotCoordinatorError> {
        let inner = Arc::clone(&self.inner);
        self.coordinator.query_with_cancel(
            instruments,
            move |batch| {
                inner
                    .query_batch(batch)
                    .map_err(|error| classify_security_snapshot_fetch_error(&error))
            },
            cancel,
        )
    }
}

impl crate::SecuritySnapshotReadPort for CachedSecuritySnapshotReader {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        CachedSecuritySnapshotReader::query(self, instruments).map_err(|error| error.to_string())
    }
}

/// Adapter that exposes [`OpenDSecuritySnapshotReader`] as a single-batch
/// 3203 reader, i.e. Go's `querySecuritySnapshotListDirect`.
pub struct OpenDSecuritySnapshotBatchReader {
    reader: crate::OpenDSecuritySnapshotReader,
}

impl std::fmt::Debug for OpenDSecuritySnapshotBatchReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDSecuritySnapshotBatchReader")
            .finish_non_exhaustive()
    }
}

impl OpenDSecuritySnapshotBatchReader {
    pub fn new(coordinator: Arc<Mutex<OpenDSessionCoordinator>>) -> Self {
        Self {
            reader: crate::OpenDSecuritySnapshotReader::new(coordinator),
        }
    }
}

impl SecuritySnapshotBatchReader for OpenDSecuritySnapshotBatchReader {
    fn query_batch(&self, symbols: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        self.reader
            .query_batch(symbols)
            .map_err(|error| error.to_string())
    }
}

#[cfg(test)]
#[path = "security_snapshot_coordinator_tests.rs"]
mod tests;
