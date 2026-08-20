use serde::{Deserialize, Serialize};

use crate::CalendarSourceProjection;

/// Complete read-only projection returned by the Go exchange-calendar
/// manager's system status endpoint.  The snapshot is intentionally owned by
/// the consumer port: this crate describes the wire and does not know how a
/// manager, registry, cache, or provider is run.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarStatusSnapshot {
    pub auto_refresh_enabled: bool,
    pub refresh_interval_hours: i32,
    pub warmup_markets: Vec<String>,
    pub markets: Vec<CalendarMarketStatus>,
    pub sources: Vec<CalendarSourceProjection>,
    pub snapshots: Vec<CalendarSnapshotSummary>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarMarketStatus {
    pub market: String,
    pub effective_source: String,
    pub effective_mode: String,
    pub effective_reason: String,
    pub fallback_chain: Vec<String>,
    pub checked_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSnapshotSummary {
    pub market: String,
    pub source_id: String,
    pub from: String,
    pub to: String,
    pub fetched_at: String,
    pub valid_until: String,
    pub schedules_parsed: i32,
    pub checksum: String,
    pub sample_schedules: Vec<CalendarSampleSchedule>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSampleSchedule {
    pub market: String,
    pub date: String,
    pub status: String,
    pub reason: String,
    pub source_id: String,
    pub observed: bool,
    pub sessions: Option<Vec<CalendarSampleSession>>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSampleSession {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}
