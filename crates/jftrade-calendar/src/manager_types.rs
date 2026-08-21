use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

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

#[derive(Clone, Debug, Default)]
pub struct CalendarCancellationToken(Arc<AtomicBool>);

impl CalendarCancellationToken {
    pub fn cancel(&self) {
        self.0.store(true, Ordering::Release);
    }

    pub fn is_cancelled(&self) -> bool {
        self.0.load(Ordering::Acquire)
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
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum ManagerLifecycleState {
    #[default]
    New,
    Running,
    Closed,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct CalendarRefreshResult {
    pub market: String,
    pub updated: i32,
    pub failures: i32,
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
    pub health_state: String,
}
