//! Live health evidence for the managed market-data helper.
//!
//! Startup readiness is proven by `HelperProcess::start_until_ready` with a
//! real `/healthz` round trip.  This monitor keeps that evidence current: it
//! polls `/healthz` on a fixed interval, records `last_success_at`,
//! `last_error` and `checked_at`, and treats readiness as stale once no
//! successful check landed within the configured TTL.  The dynamic provider
//! readiness and the market-data capability matrix consume the same snapshot,
//! so a failing or stale helper dynamically downgrades the runtime instead of
//! masquerading as healthy.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use jftrade_integration_marketdata_helper::{HelperClient, HelperProcess};
use serde::Serialize;
use tokio::sync::Mutex as AsyncMutex;

fn unix_millis_now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .unwrap_or_default()
}

/// Health evidence snapshot for one managed helper process.  Timestamps are
/// unix epoch milliseconds; `staleness_ms` is evaluated at read time against
/// the last successful check.
#[derive(Clone, Debug, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HelperHealthSnapshot {
    pub healthy: bool,
    pub last_success_at: Option<i64>,
    pub last_error: Option<String>,
    pub checked_at: Option<i64>,
    pub staleness_ms: Option<i64>,
    #[serde(default)]
    pub restarts: u32,
    #[serde(default)]
    pub consecutive_failures: u32,
}

/// Restart and backoff policy for a supervised market-data helper process.
#[derive(Clone, Debug)]
pub struct HelperRestartPolicy {
    pub initial_backoff: Duration,
    pub max_backoff: Duration,
    pub multiplier: f64,
    pub startup_timeout: Duration,
    pub initial_retry_delay: Duration,
    pub max_retry_delay: Duration,
}

impl Default for HelperRestartPolicy {
    fn default() -> Self {
        Self {
            initial_backoff: Duration::from_millis(500),
            max_backoff: Duration::from_millis(10000),
            multiplier: 2.0,
            startup_timeout: Duration::from_secs(5),
            initial_retry_delay: Duration::from_millis(50),
            max_retry_delay: Duration::from_millis(250),
        }
    }
}

/// Compute exponential backoff clamped between initial and max.
pub fn compute_helper_backoff(
    initial: Duration,
    max: Duration,
    multiplier: f64,
    attempt: u32,
) -> Duration {
    let min_bound = initial.min(max);
    let max_bound = initial.max(max);
    if attempt <= 1 {
        return min_bound;
    }
    let factor = multiplier.powi((attempt - 1).min(30) as i32);
    let millis = (initial.as_millis() as f64 * factor) as u64;
    Duration::from_millis(millis).clamp(min_bound, max_bound)
}

async fn sleep_with_cancellation(duration: Duration, stop: &AtomicBool) -> bool {
    let tick = Duration::from_millis(50);
    let deadline = tokio::time::Instant::now() + duration;
    while tokio::time::Instant::now() < deadline {
        if stop.load(Ordering::Acquire) {
            return true;
        }
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        tokio::time::sleep(remaining.min(tick)).await;
    }
    stop.load(Ordering::Acquire)
}

pub struct HelperHealthMonitor {
    client: HelperClient,
    interval: Duration,
    ttl: Duration,
    snapshot: RwLock<HelperHealthSnapshot>,
    refresh_guard: AsyncMutex<()>,
    stop: Arc<AtomicBool>,
    managed_process: Option<Arc<Mutex<Option<HelperProcess>>>>,
    restart_policy: HelperRestartPolicy,
    task: Mutex<Option<tokio::task::JoinHandle<()>>>,
}

impl std::fmt::Debug for HelperHealthMonitor {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("HelperHealthMonitor")
            .field("interval", &self.interval)
            .field("ttl", &self.ttl)
            .field("snapshot", &self.snapshot())
            .finish()
    }
}

impl HelperHealthMonitor {
    pub fn new(client: HelperClient, interval: Duration, ttl: Duration) -> Self {
        Self {
            client,
            interval,
            ttl,
            snapshot: RwLock::new(HelperHealthSnapshot::default()),
            refresh_guard: AsyncMutex::new(()),
            stop: Arc::new(AtomicBool::new(false)),
            managed_process: None,
            restart_policy: HelperRestartPolicy::default(),
            task: Mutex::new(None),
        }
    }

    pub fn with_managed_process(
        client: HelperClient,
        interval: Duration,
        ttl: Duration,
        managed_process: Arc<Mutex<Option<HelperProcess>>>,
        restart_policy: HelperRestartPolicy,
    ) -> Self {
        Self {
            client,
            interval,
            ttl,
            snapshot: RwLock::new(HelperHealthSnapshot::default()),
            refresh_guard: AsyncMutex::new(()),
            stop: Arc::new(AtomicBool::new(false)),
            managed_process: Some(managed_process),
            restart_policy,
            task: Mutex::new(None),
        }
    }

    /// Record the real successful `/healthz` probe performed by the startup
    /// readiness gate, so the monitor starts from actual evidence instead of
    /// an unknown state.
    pub fn seed_success(&self) {
        let now = unix_millis_now();
        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.healthy = true;
        snapshot.last_success_at = Some(now);
        snapshot.last_error = None;
        snapshot.checked_at = Some(now);
    }

    pub fn record_restart(&self) {
        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.restarts = snapshot.restarts.saturating_add(1);
    }

    /// Perform one real `/healthz` round trip and update the snapshot with
    /// its outcome.  A failed check marks the helper unhealthy immediately;
    /// only a later successful check clears the failure.
    pub async fn refresh(&self) {
        let _guard = self.refresh_guard.lock().await;
        let now = unix_millis_now();
        let proc_dead = if let Some(ref managed) = self.managed_process {
            let mut guard = managed
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            if let Some(ref mut proc) = *guard {
                !proc.is_alive()
            } else {
                true
            }
        } else {
            false
        };

        let outcome = if proc_dead {
            Err("helper process exited".to_string())
        } else {
            self.client
                .healthz()
                .await
                .map(|_| ())
                .map_err(|error| error.to_string())
        };

        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.checked_at = Some(now);
        match outcome {
            Ok(_) => {
                snapshot.healthy = true;
                snapshot.last_success_at = Some(now);
                snapshot.last_error = None;
                snapshot.consecutive_failures = 0;
            }
            Err(error) => {
                snapshot.healthy = false;
                snapshot.last_error = Some(error);
                snapshot.consecutive_failures = snapshot.consecutive_failures.saturating_add(1);
            }
        }
    }

    pub fn snapshot(&self) -> HelperHealthSnapshot {
        let mut snapshot = self
            .snapshot
            .read()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .clone();
        let now = unix_millis_now();
        snapshot.staleness_ms = snapshot
            .last_success_at
            .map(|at| (now.saturating_sub(at)).max(0));
        snapshot
    }

    /// The helper counts as ready only when the last `/healthz` check
    /// succeeded and that success is still within the staleness TTL.
    pub fn is_ready(&self) -> bool {
        let snapshot = self
            .snapshot
            .read()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.healthy
            && snapshot.last_error.is_none()
            && snapshot.last_success_at.is_some_and(|at| {
                let staleness = unix_millis_now().saturating_sub(at);
                staleness <= i64::try_from(self.ttl.as_millis()).unwrap_or(i64::MAX)
            })
    }
}

struct OrphanGuard {
    proc: Option<HelperProcess>,
    managed: Arc<Mutex<Option<HelperProcess>>>,
    stopped: Arc<AtomicBool>,
}

impl Drop for OrphanGuard {
    fn drop(&mut self) {
        if let Some(mut proc) = self.proc.take() {
            if self.stopped.load(Ordering::Acquire) {
                proc.terminate();
            }
            let mut guard = self
                .managed
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            *guard = Some(proc);
        }
    }
}

impl HelperHealthMonitor {
    /// Run the monitor loop until [`HelperHealthMonitor::stop`] or
    /// [`HelperHealthMonitor::shutdown`] is requested.
    pub fn spawn(self: &Arc<Self>) -> tokio::task::JoinHandle<()> {
        let monitor = Arc::clone(self);
        let (completion_tx, completion_rx) = tokio::sync::oneshot::channel();
        let task = tokio::spawn(async move {
            let mut failure_count = 0_u32;
            let mut consecutive_successes = 0_u32;
            while !monitor.stop.load(Ordering::Acquire) {
                monitor.refresh().await;
                let is_healthy = monitor.is_ready();
                if is_healthy {
                    consecutive_successes = consecutive_successes.saturating_add(1);
                    if consecutive_successes >= 6 {
                        failure_count = 0;
                    }
                    if sleep_with_cancellation(monitor.interval, &monitor.stop).await {
                        break;
                    }
                } else {
                    consecutive_successes = 0;
                    if let Some(ref managed) = monitor.managed_process {
                        failure_count = failure_count.saturating_add(1);
                        let backoff = compute_helper_backoff(
                            monitor.restart_policy.initial_backoff,
                            monitor.restart_policy.max_backoff,
                            monitor.restart_policy.multiplier,
                            failure_count,
                        );
                        if sleep_with_cancellation(backoff, &monitor.stop).await {
                            break;
                        }
                        if monitor.stop.load(Ordering::Acquire) {
                            break;
                        }
                        let mut proc_opt = {
                            let mut guard = managed
                                .lock()
                                .unwrap_or_else(|poisoned| poisoned.into_inner());
                            guard.take()
                        };
                        if let Some(proc) = proc_opt.take() {
                            let mut orphan_guard = OrphanGuard {
                                proc: Some(proc),
                                managed: Arc::clone(managed),
                                stopped: Arc::clone(&monitor.stop),
                            };
                            let restart_res = orphan_guard
                                .proc
                                .as_mut()
                                .unwrap()
                                .restart_until_ready(
                                    &monitor.client,
                                    monitor.restart_policy.startup_timeout,
                                    monitor.restart_policy.initial_retry_delay,
                                    monitor.restart_policy.max_retry_delay,
                                )
                                .await;
                            if monitor.stop.load(Ordering::Acquire) {
                                if let Some(ref mut p) = orphan_guard.proc {
                                    p.terminate();
                                }
                                break;
                            }
                            let proc = orphan_guard.proc.take().unwrap();
                            let restarts = proc.restarts();
                            {
                                let mut guard = managed
                                    .lock()
                                    .unwrap_or_else(|poisoned| poisoned.into_inner());
                                *guard = Some(proc);
                            }
                            match restart_res {
                                Ok(_) => {
                                    monitor.seed_success();
                                    let mut snap =
                                        monitor.snapshot.write().unwrap_or_else(|p| p.into_inner());
                                    snap.restarts = restarts;
                                    snap.consecutive_failures = 0;
                                    // Do not reset failure_count on initial restart probe success.
                                    // Flapping/crash-looping sidecars must retain backoff progression;
                                    // failure_count is only reset after sustained healthy execution (consecutive_successes >= 6).
                                    consecutive_successes = 1;
                                }
                                Err(err) => {
                                    let mut snap =
                                        monitor.snapshot.write().unwrap_or_else(|p| p.into_inner());
                                    snap.healthy = false;
                                    snap.last_error = Some(format!("auto-restart failed: {err}"));
                                    snap.consecutive_failures = failure_count;
                                }
                            }
                        }
                    } else if sleep_with_cancellation(monitor.interval, &monitor.stop).await {
                        break;
                    }
                }
            }
            let _ = completion_tx.send(());
        });

        *self.task.lock().unwrap_or_else(|p| p.into_inner()) = Some(task);

        tokio::spawn(async move {
            let _ = completion_rx.await;
        })
    }

    /// Request the monitor loop to finish; in-flight checks complete without
    /// further effects beyond the snapshot update.
    pub fn stop(&self) {
        self.stop.store(true, Ordering::Release);
    }

    /// Abort and await the owned background task, guaranteeing that no in-flight
    /// restart loop continues executing or leaves an orphan child process behind.
    pub async fn shutdown(&self) {
        self.stop();
        let task = self
            .task
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take();
        if let Some(task) = task {
            task.abort();
            let _ = task.await;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn monitor_on(endpoint: &str) -> HelperHealthMonitor {
        HelperHealthMonitor::new(
            HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
                base_url: endpoint.to_owned(),
                bearer_token: None,
                request_timeout: Duration::from_millis(300),
                max_attempts: 1,
                retry_delay: Duration::from_millis(10),
            })
            .expect("client"),
            Duration::from_millis(50),
            Duration::from_millis(200),
        )
    }

    #[tokio::test]
    async fn refresh_records_success_and_failure_with_timestamps() {
        let monitor = monitor_on("http://127.0.0.1:1");
        monitor.refresh().await;
        let failed = monitor.snapshot();
        assert!(!failed.healthy);
        assert!(failed.last_error.is_some());
        assert!(failed.checked_at.is_some());
        assert!(failed.last_success_at.is_none());

        monitor.seed_success();
        let seeded = monitor.snapshot();
        assert!(seeded.healthy);
        assert!(seeded.last_error.is_none());
        assert!(monitor.is_ready());
    }

    #[tokio::test]
    async fn readiness_expires_once_the_success_is_stale() {
        let monitor = monitor_on("http://127.0.0.1:1");
        monitor.seed_success();
        assert!(monitor.is_ready());
        // Beyond the 200ms TTL the seeded success must no longer count.
        tokio::time::sleep(Duration::from_millis(220)).await;
        assert!(!monitor.is_ready());
        assert!(monitor.snapshot().last_error.is_none());
    }

    #[test]
    fn compute_helper_backoff_doubles_and_caps_at_max() {
        let initial = Duration::from_millis(500);
        let max = Duration::from_millis(10000);
        let multiplier = 2.0;

        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 0),
            Duration::from_millis(500)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 1),
            Duration::from_millis(500)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 2),
            Duration::from_millis(1000)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 3),
            Duration::from_millis(2000)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 4),
            Duration::from_millis(4000)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 5),
            Duration::from_millis(8000)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 6),
            Duration::from_millis(10000)
        );
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, 10),
            Duration::from_millis(10000)
        );
    }

    #[test]
    fn record_restart_increments_snapshot_counter() {
        let monitor = monitor_on("http://127.0.0.1:1");
        assert_eq!(monitor.snapshot().restarts, 0);
        monitor.record_restart();
        assert_eq!(monitor.snapshot().restarts, 1);
        monitor.record_restart();
        assert_eq!(monitor.snapshot().restarts, 2);
    }
}
