//! Dynamic readiness evidence for a managed PineTS worker.
//!
//! Startup readiness is still established by [`PineProcess::wait_until_ready`]
//! with a real gRPC health probe.  The state in this module keeps that
//! evidence live afterwards: a failed health check (including a process exit
//! that makes the loopback endpoint disappear) immediately clears readiness,
//! and a later successful check restores it.  The monitor owns its task so the
//! composition root can join it during shutdown instead of leaving a detached
//! probe behind.

use std::sync::RwLock;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use std::{fmt, sync::Arc};

use serde::Serialize;
use tokio::task::JoinHandle;

use crate::pool::WorkerHealth;
use crate::process::{
    GrpcPineReadinessProbe, PineProcess, PineReadinessPolicy, PineReadinessProbe, WorkerProcessSpec,
};

/// Default interval for post-startup Pine worker health checks.
pub const DEFAULT_PINE_HEALTH_INTERVAL: Duration = Duration::from_secs(1);

fn unix_millis_now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .unwrap_or_default()
}

/// Shared, queryable health evidence for one Pine worker.
#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PineReadinessSnapshot {
    pub worker_id: String,
    pub healthy: bool,
    pub running: bool,
    pub last_success_at: Option<i64>,
    pub last_error: Option<String>,
    pub checked_at: Option<i64>,
    pub version: String,
    pub pine_ts_version: String,
    pub capabilities: Vec<String>,
    #[serde(default)]
    pub restarts: u32,
    #[serde(default)]
    pub consecutive_failures: u32,
}

/// Thread-safe readiness state shared by execution ports, route bindings and
/// status projections.  It starts unavailable and can only become ready from
/// a successful health response.
pub struct PineReadinessState {
    snapshot: RwLock<PineReadinessSnapshot>,
    stopped: AtomicBool,
}

impl fmt::Debug for PineReadinessState {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("PineReadinessState")
            .field("snapshot", &self.snapshot())
            .finish()
    }
}

impl PineReadinessState {
    /// Create an initially unavailable state for a worker identity.
    pub fn new(worker_id: impl Into<String>) -> Arc<Self> {
        Arc::new(Self {
            snapshot: RwLock::new(PineReadinessSnapshot {
                worker_id: worker_id.into(),
                ..PineReadinessSnapshot::default()
            }),
            stopped: AtomicBool::new(false),
        })
    }

    /// Seed the state with the health response already obtained by the
    /// startup readiness gate.
    pub fn seed_success(&self, health: WorkerHealth) {
        self.record_health(Ok(health));
    }

    /// Publish one health-check result.  Transport failures mark the worker
    /// as not running; an explicit `ok=false` response keeps it reachable but
    /// not ready.  Only a later `ok=true` response restores readiness.
    pub fn record_health(&self, result: Result<WorkerHealth, String>) {
        self.record_health_with_restarts(result, None);
    }

    /// Publish one health-check result and optionally update the restart counter.
    pub fn record_health_with_restarts(
        &self,
        result: Result<WorkerHealth, String>,
        restarts: Option<u32>,
    ) {
        let now = unix_millis_now();
        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        // Stop is terminal for a state instance.  A probe can be in flight
        // while the owning monitor is shut down; do not let that stale result
        // republish readiness after the stop transition.
        if self.stopped.load(Ordering::Acquire) {
            return;
        }
        if let Some(r) = restarts {
            snapshot.restarts = r;
        }
        snapshot.checked_at = Some(now);
        match result {
            Ok(health) if health.ok => {
                snapshot.healthy = true;
                snapshot.running = true;
                snapshot.last_success_at = Some(now);
                snapshot.last_error = None;
                snapshot.version = health.version;
                snapshot.pine_ts_version = health.pine_ts_version;
                snapshot.capabilities = health.capabilities;
                snapshot.consecutive_failures = 0;
            }
            Ok(health) => {
                snapshot.healthy = false;
                snapshot.running = true;
                snapshot.last_error = Some("worker health returned ok=false".to_owned());
                snapshot.version = health.version;
                snapshot.pine_ts_version = health.pine_ts_version;
                snapshot.capabilities = health.capabilities;
                snapshot.consecutive_failures = snapshot.consecutive_failures.saturating_add(1);
            }
            Err(error) => {
                snapshot.healthy = false;
                snapshot.running = false;
                snapshot.last_error = Some(error);
                snapshot.consecutive_failures = snapshot.consecutive_failures.saturating_add(1);
            }
        }
    }

    /// Increment the restart counter on a successful worker respawn.
    pub fn record_restart(&self) {
        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.restarts = snapshot.restarts.saturating_add(1);
    }

    /// Mark the worker unavailable when the monitor or owning runtime stops.
    pub fn mark_stopped(&self, reason: impl Into<String>) {
        let first_stop = !self.stopped.swap(true, Ordering::AcqRel);
        let mut snapshot = self
            .snapshot
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.healthy = false;
        snapshot.running = false;
        if first_stop {
            snapshot.last_error = Some(reason.into());
        }
        snapshot.checked_at = Some(unix_millis_now());
    }

    pub fn snapshot(&self) -> PineReadinessSnapshot {
        self.snapshot
            .read()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .clone()
    }

    /// Return true only while the last observed worker health is positive.
    pub fn is_ready(&self) -> bool {
        if self.stopped.load(Ordering::Acquire) {
            return false;
        }
        let snapshot = self
            .snapshot
            .read()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        snapshot.healthy && snapshot.running
    }

    pub fn unavailable_message(&self) -> String {
        self.snapshot()
            .last_error
            .unwrap_or_else(|| "pine worker is not ready".to_owned())
    }
}

/// Owned periodic health monitor.  Its task does not retain the monitor
/// itself, avoiding an `Arc` cycle between the monitor and its join handle.
pub struct PineReadinessMonitor {
    state: Arc<PineReadinessState>,
    stop: Arc<AtomicBool>,
    task: std::sync::Mutex<Option<JoinHandle<()>>>,
}

impl fmt::Debug for PineReadinessMonitor {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("PineReadinessMonitor")
            .field("state", &self.state)
            .field("stopped", &self.stop.load(Ordering::Acquire))
            .finish()
    }
}

/// Restart and backoff policy for a supervised PineTS worker process.
#[derive(Clone, Debug)]
pub struct PineRestartPolicy {
    pub initial_backoff: Duration,
    pub max_backoff: Duration,
    pub multiplier: f64,
    pub readiness_policy: PineReadinessPolicy,
}

impl Default for PineRestartPolicy {
    fn default() -> Self {
        Self {
            initial_backoff: Duration::from_millis(500),
            max_backoff: Duration::from_millis(10000),
            multiplier: 2.0,
            readiness_policy: PineReadinessPolicy::default(),
        }
    }
}

/// Compute exponential backoff clamped between initial and max.
pub fn compute_pine_backoff(
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

impl PineReadinessMonitor {
    /// Spawn a monitor using the standard post-startup interval.
    pub fn spawn(
        state: Arc<PineReadinessState>,
        probe: GrpcPineReadinessProbe,
        spec: WorkerProcessSpec,
    ) -> Arc<Self> {
        Self::spawn_with_interval(state, probe, spec, DEFAULT_PINE_HEALTH_INTERVAL)
    }

    /// Spawn a supervised monitor that automatically recovers a failed/crashed
    /// Pine worker process with exponential backoff.
    pub fn spawn_supervised(
        state: Arc<PineReadinessState>,
        probe: GrpcPineReadinessProbe,
        process: Arc<tokio::sync::Mutex<PineProcess>>,
        interval: Duration,
        policy: PineRestartPolicy,
    ) -> Arc<Self> {
        let interval = if interval.is_zero() {
            Duration::from_millis(1)
        } else {
            interval
        };
        let stop = Arc::new(AtomicBool::new(false));
        let stop_for_task = Arc::clone(&stop);
        let state_for_task = Arc::clone(&state);
        let task = tokio::spawn(async move {
            let mut failure_count = 0_u32;
            let mut consecutive_successes = 0_u32;
            while !stop_for_task.load(Ordering::Acquire) {
                let (spec, proc_dead) = {
                    let mut proc = process.lock().await;
                    let dead = !proc.is_alive();
                    (proc.spec().clone(), dead)
                };
                let result = if proc_dead {
                    Err("pine worker process exited".to_string())
                } else {
                    probe.health(&spec).await.map_err(|error| error.to_string())
                };
                let is_ok = matches!(&result, Ok(health) if health.ok);
                if is_ok {
                    consecutive_successes = consecutive_successes.saturating_add(1);
                    if consecutive_successes >= 30 {
                        failure_count = 0;
                    }
                    state_for_task.record_health(result);
                    if stop_for_task.load(Ordering::Acquire) {
                        break;
                    }
                    if sleep_with_cancellation(interval, &stop_for_task).await {
                        break;
                    }
                } else {
                    consecutive_successes = 0;
                    state_for_task.record_health(result);
                    if stop_for_task.load(Ordering::Acquire) {
                        break;
                    }
                    failure_count = failure_count.saturating_add(1);
                    let backoff = compute_pine_backoff(
                        policy.initial_backoff,
                        policy.max_backoff,
                        policy.multiplier,
                        failure_count,
                    );
                    if sleep_with_cancellation(backoff, &stop_for_task).await {
                        break;
                    }
                    let mut proc = process.lock().await;
                    if stop_for_task.load(Ordering::Acquire) {
                        break;
                    }
                    match proc
                        .restart_until_ready(&probe, policy.readiness_policy)
                        .await
                    {
                        Ok(health) => {
                            let restarts = proc.restarts();
                            state_for_task.record_health_with_restarts(Ok(health), Some(restarts));
                            // Do not reset failure_count on initial restart probe success.
                            // Flapping/crash-looping sidecars must retain backoff progression;
                            // failure_count is only reset after sustained healthy execution.
                            consecutive_successes = 1;
                        }
                        Err(error) => {
                            state_for_task
                                .record_health(Err(format!("auto-restart failed: {error}")));
                        }
                    }
                }
            }
        });
        Arc::new(Self {
            state,
            stop,
            task: std::sync::Mutex::new(Some(task)),
        })
    }

    /// Spawn a monitor with an explicit interval for deterministic tests and
    /// embedding profiles.
    pub fn spawn_with_interval(
        state: Arc<PineReadinessState>,
        probe: GrpcPineReadinessProbe,
        spec: WorkerProcessSpec,
        interval: Duration,
    ) -> Arc<Self> {
        let interval = if interval.is_zero() {
            Duration::from_millis(1)
        } else {
            interval
        };
        let stop = Arc::new(AtomicBool::new(false));
        let stop_for_task = Arc::clone(&stop);
        let state_for_task = Arc::clone(&state);
        let task = tokio::spawn(async move {
            while !stop_for_task.load(Ordering::Acquire) {
                let result = probe.health(&spec).await.map_err(|error| error.to_string());
                state_for_task.record_health(result);
                if stop_for_task.load(Ordering::Acquire) {
                    break;
                }
                if sleep_with_cancellation(interval, &stop_for_task).await {
                    break;
                }
            }
        });
        Arc::new(Self {
            state,
            stop,
            task: std::sync::Mutex::new(Some(task)),
        })
    }

    pub fn state(&self) -> Arc<PineReadinessState> {
        Arc::clone(&self.state)
    }

    /// Request stop and mark the shared state unavailable immediately.
    pub fn stop(&self) {
        self.stop.store(true, Ordering::Release);
        self.state
            .mark_stopped("pine worker health monitor stopped");
    }

    /// Abort and join the owned task.  Aborting is intentional: the probe is
    /// bounded by its request timeout, but shutdown must not wait for a full
    /// health interval or leave a background task behind.
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

    /// Synchronous Drop path used after the Tokio runtime has already gone
    /// away.  The task is aborted but cannot be awaited here.
    pub fn terminate(&self) {
        self.stop();
        if let Some(task) = self
            .task
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take()
        {
            task.abort();
        }
    }
}

impl Drop for PineReadinessMonitor {
    fn drop(&mut self) {
        // `stop`/`shutdown` already mark the state with their more useful
        // reason.  Only the direct-drop path needs to publish the fallback
        // reason here; this also keeps an explicit shutdown reason stable.
        if !self.stop.swap(true, Ordering::AcqRel) {
            self.state
                .mark_stopped("pine worker health monitor dropped");
        }
        if let Some(task) = self
            .task
            .get_mut()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take()
        {
            task.abort();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn healthy() -> WorkerHealth {
        WorkerHealth {
            ok: true,
            version: "worker-1".to_owned(),
            pine_ts_version: "pine-1".to_owned(),
            capabilities: vec!["run".to_owned()],
        }
    }

    #[test]
    fn readiness_transitions_from_initial_unavailable_through_failure_and_recovery() {
        let state = PineReadinessState::new("pineworker-1");
        assert!(!state.is_ready());

        state.seed_success(healthy());
        assert!(state.is_ready());

        state.record_health(Err("connection refused".to_owned()));
        assert!(!state.is_ready());
        assert_eq!(
            state.snapshot().last_error.as_deref(),
            Some("connection refused")
        );

        state.seed_success(healthy());
        assert!(state.is_ready());
        assert!(state.snapshot().last_error.is_none());
    }

    #[test]
    fn explicit_health_failure_keeps_reachable_worker_unready() {
        let state = PineReadinessState::new("pineworker-1");
        state.record_health(Ok(WorkerHealth {
            ok: false,
            ..healthy()
        }));
        let snapshot = state.snapshot();
        assert!(!snapshot.healthy);
        assert!(snapshot.running);
        assert!(!state.is_ready());
    }

    #[test]
    fn stopped_state_ignores_late_health_success() {
        let state = PineReadinessState::new("pineworker-1");
        state.seed_success(healthy());
        state.mark_stopped("worker stopped");

        // Simulate a health request that completed after shutdown began.
        state.record_health(Ok(healthy()));

        let snapshot = state.snapshot();
        assert!(!state.is_ready());
        assert!(!snapshot.running);
        assert_eq!(snapshot.last_error.as_deref(), Some("worker stopped"));
    }

    #[tokio::test]
    async fn dropping_monitor_marks_shared_state_stopped() {
        let state = PineReadinessState::new("pineworker-1");
        state.seed_success(healthy());
        let probe =
            GrpcPineReadinessProbe::new(None, Duration::from_millis(20), Duration::from_millis(20))
                .expect("probe");
        let monitor = PineReadinessMonitor::spawn_with_interval(
            Arc::clone(&state),
            probe,
            WorkerProcessSpec {
                worker_id: "pineworker-1".to_owned(),
                host: "127.0.0.1".parse().expect("loopback"),
                port: 9,
            },
            Duration::from_millis(5),
        );

        drop(monitor);
        tokio::task::yield_now().await;

        assert!(!state.is_ready());
        assert_eq!(
            state.snapshot().last_error.as_deref(),
            Some("pine worker health monitor dropped")
        );
    }

    #[tokio::test]
    async fn monitor_shutdown_marks_state_and_joins_task() {
        let state = PineReadinessState::new("pineworker-1");
        state.seed_success(healthy());
        let probe =
            GrpcPineReadinessProbe::new(None, Duration::from_millis(20), Duration::from_millis(20))
                .expect("probe");
        let monitor = PineReadinessMonitor::spawn_with_interval(
            Arc::clone(&state),
            probe,
            WorkerProcessSpec {
                worker_id: "pineworker-1".to_owned(),
                host: "127.0.0.1".parse().expect("loopback"),
                port: 9,
            },
            Duration::from_millis(5),
        );
        monitor.shutdown().await;
        assert!(!state.is_ready());
        assert!(
            state
                .snapshot()
                .last_error
                .as_deref()
                .is_some_and(|error| error.contains("monitor stopped"))
        );
    }

    #[test]
    fn compute_pine_backoff_doubles_and_caps_at_max() {
        let initial = Duration::from_millis(500);
        let max = Duration::from_millis(10000);
        let multiplier = 2.0;

        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 0),
            Duration::from_millis(500)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 1),
            Duration::from_millis(500)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 2),
            Duration::from_millis(1000)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 3),
            Duration::from_millis(2000)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 4),
            Duration::from_millis(4000)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 5),
            Duration::from_millis(8000)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 6),
            Duration::from_millis(10000)
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, 10),
            Duration::from_millis(10000)
        );
    }

    #[test]
    fn record_health_with_restarts_tracks_restarts_and_consecutive_failures() {
        let state = PineReadinessState::new("pineworker-test");
        state.seed_success(healthy());
        assert_eq!(state.snapshot().restarts, 0);
        assert_eq!(state.snapshot().consecutive_failures, 0);

        // First failure
        state.record_health(Err("connection dropped".to_owned()));
        assert_eq!(state.snapshot().consecutive_failures, 1);
        assert!(!state.is_ready());

        // Second failure
        state.record_health(Err("connection refused".to_owned()));
        assert_eq!(state.snapshot().consecutive_failures, 2);

        // Recovery with restart count 1
        state.record_health_with_restarts(Ok(healthy()), Some(1));
        assert!(state.is_ready());
        assert_eq!(state.snapshot().restarts, 1);
        assert_eq!(state.snapshot().consecutive_failures, 0);

        // Manual record_restart increment
        state.record_restart();
        assert_eq!(state.snapshot().restarts, 2);
    }
}
