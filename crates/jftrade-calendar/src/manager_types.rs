use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration as StdDuration, Instant};

use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::{
    CalendarSnapshot, CalendarSnapshotLoadResult, CalendarSnapshotStore,
    CalendarSnapshotStoreError, CalendarSourceDescriptor,
};

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CalendarManagerSettings {
    pub auto_refresh_enabled: bool,
    pub error_notifications_enabled: bool,
    pub refresh_interval_hours: i32,
    pub warmup_markets: Vec<String>,
    pub source_policies: Vec<CalendarSourcePolicy>,
    pub manual_overrides: Vec<CalendarManualOverride>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CalendarSourcePolicy {
    pub market: String,
    pub preferred_source_ids: Vec<String>,
    pub enabled_source_ids: Vec<String>,
    pub fallback_to_builtin: bool,
    pub require_official: bool,
    pub stale_after_hours: i32,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CalendarManualOverride {
    pub market: String,
    pub date: String,
    pub status: String,
    pub sessions: Vec<CalendarSessionOverride>,
    pub reason: String,
    pub observed: bool,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CalendarSessionOverride {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

#[derive(Debug)]
struct CalendarCancellationState {
    cancelled: AtomicBool,
    parent: Option<Arc<CalendarCancellationState>>,
    deadline: Option<Instant>,
}

#[derive(Clone, Debug)]
pub struct CalendarCancellationToken(Arc<CalendarCancellationState>);

impl Default for CalendarCancellationToken {
    fn default() -> Self {
        Self(Arc::new(CalendarCancellationState {
            cancelled: AtomicBool::new(false),
            parent: None,
            deadline: None,
        }))
    }
}

impl CalendarCancellationToken {
    pub fn cancel(&self) {
        self.0.cancelled.store(true, Ordering::Release);
    }

    pub fn is_cancelled(&self) -> bool {
        self.0.cancelled.load(Ordering::Acquire)
            || self
                .0
                .parent
                .as_ref()
                .is_some_and(|parent| CalendarCancellationToken(Arc::clone(parent)).is_cancelled())
            || self
                .0
                .deadline
                .is_some_and(|deadline| Instant::now() >= deadline)
    }

    pub(crate) fn child_with_timeout(&self, timeout: StdDuration) -> Self {
        Self(Arc::new(CalendarCancellationState {
            cancelled: AtomicBool::new(false),
            parent: Some(Arc::clone(&self.0)),
            deadline: Instant::now().checked_add(timeout),
        }))
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum CalendarSourceError {
    #[error("calendar source operation was cancelled")]
    Cancelled,
    #[error("{0}")]
    Failed(String),
}

pub trait CalendarSourcePort: Send + Sync {
    fn descriptor(&self) -> CalendarSourceDescriptor;

    fn start(&self, _cancellation: &CalendarCancellationToken) -> Result<(), CalendarSourceError> {
        Ok(())
    }

    fn fetch(
        &self,
        market: &str,
        from: WireTimestamp,
        to: WireTimestamp,
        cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError>;

    fn close(&self) -> Result<(), CalendarSourceError> {
        Ok(())
    }
}

pub trait CalendarPersistencePort: Send + Sync {
    fn load(&self) -> CalendarSnapshotLoadResult;
    fn save(&self, snapshot: &CalendarSnapshot) -> Result<(), String>;

    /// Remove a persisted snapshot that failed validation during restore.
    ///
    /// Implementations that cannot mutate their backing store may keep the
    /// default no-op; the manager still isolates the invalid value from its
    /// in-memory cache.  The separate `delete_snapshot` hook keeps the port
    /// compatible with adapters that use the store's Go-derived naming.
    fn delete(&self, snapshot: &CalendarSnapshot) -> Result<(), String> {
        self.delete_snapshot(snapshot)
    }

    fn delete_snapshot(&self, _snapshot: &CalendarSnapshot) -> Result<(), String> {
        Ok(())
    }
}

impl CalendarPersistencePort for CalendarSnapshotStore {
    fn load(&self) -> CalendarSnapshotLoadResult {
        CalendarSnapshotStore::load(self)
    }

    fn save(&self, snapshot: &CalendarSnapshot) -> Result<(), String> {
        CalendarSnapshotStore::save(self, snapshot)
            .map(|_| ())
            .map_err(|error: CalendarSnapshotStoreError| error.to_string())
    }

    fn delete(&self, snapshot: &CalendarSnapshot) -> Result<(), String> {
        CalendarSnapshotStore::delete(self, snapshot)
            .map_err(|error: CalendarSnapshotStoreError| error.to_string())
    }
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum ManagerLifecycleState {
    #[default]
    New,
    Running,
    Closed,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarRefreshResult {
    pub accepted: bool,
    pub market: String,
    pub updated: i32,
    pub failures: i32,
    #[serde(skip_serializing)]
    pub skipped_backoff: i32,
    pub requested_at: String,
    pub warmup_markets: Vec<String>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct CalendarSourceRuntimeStatus {
    pub source_id: String,
    pub enabled: bool,
    pub last_success_at: Option<String>,
    pub last_failure_at: Option<String>,
    pub last_error: String,
    pub consecutive_failures: i32,
    pub next_refresh_at: Option<String>,
    pub last_snapshot_fetched_at: Option<String>,
    pub last_probe_at: Option<String>,
    pub last_probe_success_at: Option<String>,
    pub last_probe_failure_at: Option<String>,
    pub last_probe_status: String,
    pub last_probe_error: String,
    pub last_probe_market: String,
    pub last_probe_schedules: i32,
    pub health_state: String,
    pub health_fingerprint: String,
    pub last_alert_at: Option<String>,
    pub last_alert_status: String,
    pub last_alert_fingerprint: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarProbeResult {
    pub accepted: bool,
    pub market: String,
    pub checked_at: String,
    pub healthy: i32,
    pub failures: i32,
    pub results: Vec<CalendarProbeItem>,
    pub probe_scope: Vec<String>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarProbeItem {
    pub source_id: String,
    pub market: String,
    pub status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fetched_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub valid_until: Option<String>,
    #[serde(skip_serializing_if = "is_zero_i32")]
    pub schedules_parsed: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub checksum: String,
}

const fn is_zero_i32(value: &i32) -> bool {
    *value == 0
}
