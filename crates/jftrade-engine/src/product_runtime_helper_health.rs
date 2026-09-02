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

use std::sync::Arc;
use std::sync::RwLock;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use jftrade_integration_marketdata_helper::HelperClient;
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
}

pub struct HelperHealthMonitor {
    client: HelperClient,
    interval: Duration,
    ttl: Duration,
    snapshot: RwLock<HelperHealthSnapshot>,
    refresh_guard: AsyncMutex<()>,
    stop: Arc<AtomicBool>,
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

    /// Perform one real `/healthz` round trip and update the snapshot with
    /// its outcome.  A failed check marks the helper unhealthy immediately;
    /// only a later successful check clears the failure.
    pub async fn refresh(&self) {
        let _guard = self.refresh_guard.lock().await;
        let now = unix_millis_now();
        let outcome = self.client.healthz().await;
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
            }
            Err(error) => {
                snapshot.healthy = false;
                snapshot.last_error = Some(error.to_string());
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

    /// Run the monitor loop until [`HelperHealthMonitor::stop`] is requested.
    pub fn spawn(self: &Arc<Self>) -> tokio::task::JoinHandle<()> {
        let monitor = Arc::clone(self);
        tokio::spawn(async move {
            while !monitor.stop.load(Ordering::Acquire) {
                monitor.refresh().await;
                tokio::time::sleep(monitor.interval).await;
            }
        })
    }

    /// Request the monitor loop to finish; in-flight checks complete without
    /// further effects beyond the snapshot update.
    pub fn stop(&self) {
        self.stop.store(true, Ordering::Release);
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
}
