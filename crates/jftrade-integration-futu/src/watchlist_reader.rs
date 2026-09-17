//! Cached, rate-gated remote watchlist reads over the OpenD 3222/3213 pair.
//!
//! Parity source: `go:452dea11:pkg/futu/watchlist_reader.go`.
//! Go wraps the raw `GetUserSecurityGroups`/`GetUserSecurities` calls in a
//! reader that owns three behaviors the raw port does not:
//!
//! * a 30s TTL cache per group/member set, returned as deep copies so callers
//!   cannot mutate cached state;
//! * a rolling 10-calls-per-30s gate on physical 3213/3222 reads, consulted
//!   only on a cache miss so repeated discovery does not burn the allowance;
//! * normalized duplicate-group detection, which both flags ambiguous groups
//!   and fails the member read before it issues a 3213 request.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime};

use serde_json::{Value, json};

use crate::customization::CustomizationError;
use crate::{FutuRemoteWatchlistReader, RemoteWatchlistReadPort};

/// Go `futuWatchlistReadLimit`.
pub const WATCHLIST_READ_LIMIT: usize = 10;
/// Go `futuWatchlistReadWindow`.
pub const WATCHLIST_READ_WINDOW: Duration = Duration::from_secs(30);
/// Go `futuWatchlistCacheTTL`.
pub const WATCHLIST_CACHE_TTL: Duration = Duration::from_secs(30);

#[derive(Debug, Clone, PartialEq, Eq)]
struct GroupEntry {
    expires_at: SystemTime,
    value: Vec<Value>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct MemberEntry {
    expires_at: SystemTime,
    value: Vec<Value>,
}

/// Rolling-window reservation gate, mirroring Go `futuWatchlistReadGate`.
#[derive(Debug)]
pub struct WatchlistReadGate {
    calls: Mutex<Vec<SystemTime>>,
    limit: usize,
    window: Duration,
}

impl Default for WatchlistReadGate {
    fn default() -> Self {
        Self::new(WATCHLIST_READ_LIMIT, WATCHLIST_READ_WINDOW)
    }
}

impl WatchlistReadGate {
    pub fn new(limit: usize, window: Duration) -> Self {
        Self {
            calls: Mutex::new(Vec::new()),
            limit,
            window,
        }
    }

    /// Reserves one call at `now`.
    ///
    /// Go prunes reservations at or before `now-window` (the comparison is
    /// `!call.After(cutoff)`), so a call exactly on the window boundary has
    /// already expired and is admitted.
    pub fn allow(&self, now: SystemTime) -> bool {
        let mut calls = self.calls.lock().unwrap_or_else(|error| error.into_inner());
        if self.limit == 0 || self.window.is_zero() {
            return false;
        }
        let cutoff = now
            .checked_sub(self.window)
            .unwrap_or(SystemTime::UNIX_EPOCH);
        calls.retain(|call| *call > cutoff);
        if calls.len() >= self.limit {
            return false;
        }
        calls.push(now);
        true
    }
}

/// Clock seam so cache TTL and the gate can be exercised deterministically.
pub type WatchlistClock = Arc<dyn Fn() -> SystemTime + Send + Sync>;

/// Cached remote watchlist reader.
///
/// The inner `RemoteWatchlistReadPort` performs the physical reads; this
/// wrapper owns cache, gate and ambiguity semantics.
pub struct CachedRemoteWatchlistReader {
    inner: Arc<dyn RemoteWatchlistReadPort>,
    now: WatchlistClock,
    ttl: Duration,
    fetch: Mutex<()>,
    groups: Mutex<Option<GroupEntry>>,
    members: Mutex<HashMap<String, MemberEntry>>,
    gate: WatchlistReadGate,
}

impl std::fmt::Debug for CachedRemoteWatchlistReader {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("CachedRemoteWatchlistReader")
            .finish_non_exhaustive()
    }
}

impl CachedRemoteWatchlistReader {
    pub fn new(coordinator: Arc<Mutex<crate::OpenDSessionCoordinator>>) -> Self {
        Self::with_inner(
            Arc::new(FutuRemoteWatchlistReader::new(coordinator)),
            Arc::new(SystemTime::now),
            WATCHLIST_CACHE_TTL,
        )
    }

    pub fn with_inner(
        inner: Arc<dyn RemoteWatchlistReadPort>,
        now: WatchlistClock,
        ttl: Duration,
    ) -> Self {
        Self {
            inner,
            now,
            ttl,
            fetch: Mutex::new(()),
            groups: Mutex::new(None),
            members: Mutex::new(HashMap::new()),
            gate: WatchlistReadGate::default(),
        }
    }

    /// Bypasses and replaces both caches, matching
    /// `ListWatchlistGroupsFresh`/`ListWatchlistGroupSecuritiesFresh`.
    pub fn groups_fresh(&self) -> Result<Vec<Value>, CustomizationError> {
        self.list_groups(true)
    }

    pub fn members_fresh(&self, group_name: &str) -> Result<Vec<Value>, CustomizationError> {
        self.list_members(group_name, true)
    }

    fn list_groups(&self, fresh: bool) -> Result<Vec<Value>, CustomizationError> {
        let now = (self.now)();
        if !fresh && let Some(cached) = self.cached_groups(now) {
            return Ok(cached);
        }
        let _fetch = self.fetch.lock().unwrap_or_else(|error| error.into_inner());
        // Re-check under the fetch lock: a concurrent read may have filled the
        // cache while this caller was waiting, and Go performs the same second
        // lookup before reserving a gate slot.
        let now = (self.now)();
        if !fresh && let Some(cached) = self.cached_groups(now) {
            return Ok(cached);
        }
        if !self.gate.allow(now) {
            return Err(rate_limited());
        }
        let groups = self.inner.groups()?;
        let converted = convert_groups(&groups);
        *self
            .groups
            .lock()
            .unwrap_or_else(|error| error.into_inner()) = Some(GroupEntry {
            expires_at: now + self.ttl,
            value: converted.clone(),
        });
        Ok(converted)
    }

    fn list_members(
        &self,
        group_name: &str,
        fresh: bool,
    ) -> Result<Vec<Value>, CustomizationError> {
        let requested = group_name.trim();
        if requested.is_empty() {
            return Err(CustomizationError::Invalid(
                "futu: watchlist group name is required".to_owned(),
            ));
        }
        // Group resolution always runs (fresh when requested) so a rename or a
        // newly duplicated remote group is observed before any member read.
        let groups = self.list_groups(fresh)?;
        let normalized = normalize_group_name(requested);
        let matches = groups
            .iter()
            .filter(|group| {
                group
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| normalize_group_name(name) == normalized)
            })
            .collect::<Vec<_>>();
        match matches.len() {
            0 => {
                return Err(CustomizationError::Invalid(format!(
                    "futu: watchlist group {requested:?} not found"
                )));
            }
            1 => {}
            _ => {
                return Err(CustomizationError::Invalid(format!(
                    "futu: watchlist group {requested:?} is ambiguous; rename duplicate groups in Futu first"
                )));
            }
        }
        let matched_name = matches[0]
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or(requested)
            .to_owned();

        let now = (self.now)();
        if !fresh && let Some(cached) = self.cached_members(&normalized, now) {
            return Ok(cached);
        }
        let _fetch = self.fetch.lock().unwrap_or_else(|error| error.into_inner());
        let now = (self.now)();
        if !fresh && let Some(cached) = self.cached_members(&normalized, now) {
            return Ok(cached);
        }
        if !self.gate.allow(now) {
            return Err(rate_limited());
        }
        let securities = self.inner.members(&matched_name)?;
        let converted = convert_members(&securities);
        self.members
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .insert(
                normalized,
                MemberEntry {
                    expires_at: now + self.ttl,
                    value: converted.clone(),
                },
            );
        Ok(converted)
    }

    fn cached_groups(&self, now: SystemTime) -> Option<Vec<Value>> {
        let guard = self
            .groups
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        guard
            .as_ref()
            .filter(|entry| now < entry.expires_at)
            .map(|entry| entry.value.clone())
    }

    fn cached_members(&self, name: &str, now: SystemTime) -> Option<Vec<Value>> {
        let guard = self
            .members
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        guard
            .get(name)
            .filter(|entry| now < entry.expires_at)
            .map(|entry| entry.value.clone())
    }
}

impl RemoteWatchlistReadPort for CachedRemoteWatchlistReader {
    fn groups(&self) -> Result<Vec<Value>, CustomizationError> {
        self.list_groups(false)
    }

    fn members(&self, group_name: &str) -> Result<Vec<Value>, CustomizationError> {
        self.list_members(group_name, false)
    }
}

fn rate_limited() -> CustomizationError {
    CustomizationError::Invalid(
        "futu: watchlist read rate limit exceeded (10 calls per 30 seconds)".to_owned(),
    )
}

/// Go `normalizeWatchlistGroupName`: trim and lower-case for matching.
pub fn normalize_group_name(name: &str) -> String {
    name.trim().to_ascii_lowercase()
}

/// Go `convertFutuWatchlistGroups`: mark every normalization-equal duplicate
/// ambiguous, keep the trimmed display name and map the group type.
pub fn convert_groups(groups: &[Value]) -> Vec<Value> {
    let mut counts: HashMap<String, usize> = HashMap::new();
    for group in groups {
        let Some(name) = group.get("name").and_then(Value::as_str) else {
            continue;
        };
        let name = name.trim();
        if name.is_empty() {
            continue;
        }
        *counts.entry(normalize_group_name(name)).or_default() += 1;
    }
    let mut converted = Vec::with_capacity(groups.len());
    for group in groups {
        let Some(name) = group.get("name").and_then(Value::as_str) else {
            continue;
        };
        let name = name.trim();
        if name.is_empty() {
            continue;
        }
        let kind = group
            .get("type")
            .and_then(Value::as_str)
            .unwrap_or("unknown")
            .to_owned();
        converted.push(json!({
            "name": name,
            "type": kind,
            "ambiguous": counts.get(&normalize_group_name(name)).copied().unwrap_or(0) > 1,
        }));
    }
    converted
}

/// Go `convertFutuWatchlistSecurities`: keep the canonical instrument id and
/// the broker's own code/security id as distinct aliases.
pub fn convert_members(securities: &[Value]) -> Vec<Value> {
    securities
        .iter()
        .filter_map(|security| {
            let instrument = security.get("instrumentId").and_then(Value::as_str)?.trim();
            if instrument.is_empty() {
                return None;
            }
            let mut converted = security.clone();
            let object = converted.as_object_mut()?;
            object.insert("instrumentId".to_owned(), json!(instrument));
            object.entry("name".to_owned()).or_insert(Value::Null);
            object
                .entry("securityType".to_owned())
                .or_insert(Value::Null);
            Some(Value::Object(object.clone()))
        })
        .collect()
}

#[cfg(test)]
#[path = "watchlist_reader_tests.rs"]
mod tests;
