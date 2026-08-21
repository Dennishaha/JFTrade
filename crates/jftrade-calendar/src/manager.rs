use std::collections::{BTreeMap, BTreeSet};
use std::sync::mpsc::{self, Receiver, SyncSender, TrySendError};
use std::sync::{Arc, Mutex, RwLock};
use std::thread::{self, JoinHandle};
use std::time::Duration as StdDuration;

use jftrade_kernel::WireTimestamp;
use thiserror::Error;
use time::{Date, Duration, Month, OffsetDateTime, Time};

use crate::manager_policy::{builtin_schedule, manual_schedule};
use crate::{
    BUILTIN_SOURCE_ID, CalendarCancellationToken, CalendarManagerSettings, CalendarPersistencePort,
    CalendarRefreshResult, CalendarSnapshot, CalendarSourcePolicy, CalendarSourcePort,
    CalendarSourceRegistry, CalendarSourceRuntimeStatus, ManagerLifecycleState, TradingDaySchedule,
};

const DEFAULT_MARKETS: &[&str] = &["US", "HK", "CN"];

#[derive(Debug, Error)]
pub enum CalendarManagerError {
    #[error("calendar manager settings are invalid: {0}")]
    InvalidSettings(String),
    #[error("calendar manager is closed")]
    Closed,
    #[error("calendar manager is not running")]
    NotRunning,
    #[error("unsupported calendar market {0:?}")]
    UnsupportedMarket(String),
    #[error("calendar source {source_id} failed to start: {message}")]
    SourceStart { source_id: String, message: String },
    #[error("calendar manager worker failed to start: {0}")]
    WorkerStart(std::io::Error),
    #[error("calendar manager worker panicked")]
    WorkerPanicked,
    #[error("calendar manager state is unavailable")]
    StateUnavailable,
}

pub struct CalendarManager {
    inner: Arc<ManagerInner>,
    lifecycle: Mutex<Lifecycle>,
}

struct ManagerInner {
    registry: Arc<CalendarSourceRegistry>,
    persistence: Option<Arc<dyn CalendarPersistencePort>>,
    settings: RwLock<CalendarManagerSettings>,
    snapshots: RwLock<BTreeMap<String, CalendarSnapshot>>,
    statuses: RwLock<BTreeMap<String, CalendarSourceRuntimeStatus>>,
    clock: Arc<dyn Fn() -> OffsetDateTime + Send + Sync>,
    cancellation: CalendarCancellationToken,
}

struct Lifecycle {
    state: ManagerLifecycleState,
    commands: Option<SyncSender<ManagerCommand>>,
    worker: Option<JoinHandle<()>>,
    started_sources: Vec<Arc<dyn CalendarSourcePort>>,
}

enum ManagerCommand {
    Reload,
    Stop,
}

impl CalendarManager {
    pub fn new(
        registry: CalendarSourceRegistry,
        persistence: Option<Arc<dyn CalendarPersistencePort>>,
        settings: CalendarManagerSettings,
    ) -> Result<Self, CalendarManagerError> {
        Self::with_clock(
            registry,
            persistence,
            settings,
            Arc::new(OffsetDateTime::now_utc),
        )
    }

    pub fn with_clock(
        registry: CalendarSourceRegistry,
        persistence: Option<Arc<dyn CalendarPersistencePort>>,
        settings: CalendarManagerSettings,
        clock: Arc<dyn Fn() -> OffsetDateTime + Send + Sync>,
    ) -> Result<Self, CalendarManagerError> {
        validate_settings(&settings)?;
        Ok(Self {
            inner: Arc::new(ManagerInner {
                registry: Arc::new(registry),
                persistence,
                settings: RwLock::new(settings),
                snapshots: RwLock::new(BTreeMap::new()),
                statuses: RwLock::new(BTreeMap::new()),
                clock,
                cancellation: CalendarCancellationToken::default(),
            }),
            lifecycle: Mutex::new(Lifecycle {
                state: ManagerLifecycleState::New,
                commands: None,
                worker: None,
                started_sources: Vec::new(),
            }),
        })
    }

    pub fn start(&self) -> Result<(), CalendarManagerError> {
        let mut lifecycle = self
            .lifecycle
            .lock()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        match lifecycle.state {
            ManagerLifecycleState::Running => return Ok(()),
            ManagerLifecycleState::Closed => return Err(CalendarManagerError::Closed),
            ManagerLifecycleState::New => {}
        }
        let mut started = Vec::new();
        for source in self.inner.registry.lifecycle_sources() {
            if let Err(error) = source.start(&self.inner.cancellation) {
                close_sources(&started);
                self.inner.cancellation.cancel();
                lifecycle.state = ManagerLifecycleState::Closed;
                return Err(CalendarManagerError::SourceStart {
                    source_id: source.descriptor().id,
                    message: error.to_string(),
                });
            }
            started.push(source);
        }
        if let Err(error) = self.inner.restore_snapshots() {
            self.inner.cancellation.cancel();
            close_sources(&started);
            lifecycle.state = ManagerLifecycleState::Closed;
            return Err(error);
        }
        let (sender, receiver) = mpsc::sync_channel(1);
        let inner = Arc::clone(&self.inner);
        let worker = match thread::Builder::new()
            .name("jftrade-calendar-manager".to_owned())
            .spawn(move || run_manager(inner, receiver))
        {
            Ok(worker) => worker,
            Err(error) => {
                self.inner.cancellation.cancel();
                close_sources(&started);
                lifecycle.state = ManagerLifecycleState::Closed;
                return Err(CalendarManagerError::WorkerStart(error));
            }
        };
        lifecycle.state = ManagerLifecycleState::Running;
        lifecycle.commands = Some(sender.clone());
        lifecycle.worker = Some(worker);
        lifecycle.started_sources = started;
        let _ = sender.try_send(ManagerCommand::Reload);
        Ok(())
    }

    pub fn close(&self) -> Result<(), CalendarManagerError> {
        let (worker, started) = {
            let mut lifecycle = self
                .lifecycle
                .lock()
                .map_err(|_| CalendarManagerError::StateUnavailable)?;
            if lifecycle.state == ManagerLifecycleState::Closed {
                return Ok(());
            }
            lifecycle.state = ManagerLifecycleState::Closed;
            self.inner.cancellation.cancel();
            if let Some(sender) = lifecycle.commands.take() {
                let _ = sender.try_send(ManagerCommand::Stop);
            }
            (
                lifecycle.worker.take(),
                std::mem::take(&mut lifecycle.started_sources),
            )
        };
        let worker_result = worker.map(JoinHandle::join);
        close_sources(&started);
        match worker_result {
            Some(Err(_)) => Err(CalendarManagerError::WorkerPanicked),
            _ => Ok(()),
        }
    }

    pub fn lifecycle_state(&self) -> Result<ManagerLifecycleState, CalendarManagerError> {
        self.lifecycle
            .lock()
            .map(|lifecycle| lifecycle.state)
            .map_err(|_| CalendarManagerError::StateUnavailable)
    }

    pub fn reload_settings(
        &self,
        settings: CalendarManagerSettings,
    ) -> Result<(), CalendarManagerError> {
        validate_settings(&settings)?;
        let lifecycle = self
            .lifecycle
            .lock()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        if lifecycle.state == ManagerLifecycleState::Closed {
            return Err(CalendarManagerError::Closed);
        }
        *self
            .inner
            .settings
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)? = settings;
        if let Some(sender) = &lifecycle.commands {
            match sender.try_send(ManagerCommand::Reload) {
                Ok(()) | Err(TrySendError::Full(_)) => {}
                Err(TrySendError::Disconnected(_)) => {
                    return Err(CalendarManagerError::NotRunning);
                }
            }
        }
        Ok(())
    }

    pub fn refresh_market(
        &self,
        market: &str,
    ) -> Result<CalendarRefreshResult, CalendarManagerError> {
        self.require_running()?;
        self.inner.refresh_market(market)
    }

    pub fn refresh_all(&self) -> Result<CalendarRefreshResult, CalendarManagerError> {
        self.require_running()?;
        self.inner.refresh_all()
    }

    pub fn schedule(
        &self,
        market: &str,
        at: WireTimestamp,
    ) -> Result<Option<TradingDaySchedule>, CalendarManagerError> {
        self.inner.schedule(market, at)
    }

    pub fn source_statuses(
        &self,
    ) -> Result<Vec<CalendarSourceRuntimeStatus>, CalendarManagerError> {
        let settings = self.inner.settings()?;
        let statuses = self
            .inner
            .statuses
            .read()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        Ok(self
            .inner
            .registry
            .descriptors()
            .into_iter()
            .map(|descriptor| {
                let mut status = statuses.get(&descriptor.id).cloned().unwrap_or_default();
                status.source_id = descriptor.id.clone();
                status.enabled = source_enabled(&settings, &descriptor.id);
                status
            })
            .collect())
    }

    pub fn snapshots(&self) -> Result<Vec<CalendarSnapshot>, CalendarManagerError> {
        let snapshots = self
            .inner
            .snapshots
            .read()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let mut unique = BTreeMap::new();
        for snapshot in snapshots.values() {
            unique.insert(snapshot_identity(snapshot), snapshot.clone());
        }
        Ok(unique.into_values().collect())
    }

    fn require_running(&self) -> Result<(), CalendarManagerError> {
        match self.lifecycle_state()? {
            ManagerLifecycleState::Running => Ok(()),
            ManagerLifecycleState::Closed => Err(CalendarManagerError::Closed),
            ManagerLifecycleState::New => Err(CalendarManagerError::NotRunning),
        }
    }
}

impl Drop for CalendarManager {
    fn drop(&mut self) {
        let _ = self.close();
    }
}

impl ManagerInner {
    fn settings(&self) -> Result<CalendarManagerSettings, CalendarManagerError> {
        self.settings
            .read()
            .map(|settings| settings.clone())
            .map_err(|_| CalendarManagerError::StateUnavailable)
    }

    fn now(&self) -> OffsetDateTime {
        (self.clock)()
    }

    fn restore_snapshots(&self) -> Result<(), CalendarManagerError> {
        let Some(persistence) = &self.persistence else {
            return Ok(());
        };
        let loaded = persistence.load();
        for snapshot in loaded.snapshots {
            match validate_snapshot(&snapshot) {
                Ok(()) => {
                    self.cache_snapshot(snapshot.clone())?;
                    self.record_success(&snapshot)?;
                }
                Err(error) => self.record_failure(&snapshot.source_id, error)?,
            }
        }
        for error in loaded.errors {
            self.record_failure(BUILTIN_SOURCE_ID, error.message)?;
        }
        Ok(())
    }

    fn refresh_all(&self) -> Result<CalendarRefreshResult, CalendarManagerError> {
        let settings = self.settings()?;
        let markets = normalized_markets(if settings.warmup_markets.is_empty() {
            DEFAULT_MARKETS
                .iter()
                .map(|market| (*market).to_owned())
                .collect()
        } else {
            settings.warmup_markets.clone()
        });
        let mut aggregate = CalendarRefreshResult {
            requested_at: wire_text(self.now()),
            warmup_markets: markets.clone(),
            ..CalendarRefreshResult::default()
        };
        for market in markets {
            let result = self.refresh_market(&market)?;
            aggregate.updated = aggregate.updated.saturating_add(result.updated);
            aggregate.failures = aggregate.failures.saturating_add(result.failures);
            aggregate.skipped_backoff = aggregate
                .skipped_backoff
                .saturating_add(result.skipped_backoff);
        }
        Ok(aggregate)
    }

    fn refresh_market(&self, market: &str) -> Result<CalendarRefreshResult, CalendarManagerError> {
        let market = normalize_market(market);
        if !supported_market(&market) {
            return Err(CalendarManagerError::UnsupportedMarket(market));
        }
        let settings = self.settings()?;
        let policy = policy_for_market(&settings, &market);
        let now = self.now();
        let (from, to) = fetch_window(now)?;
        let mut result = CalendarRefreshResult {
            market: market.clone(),
            requested_at: wire_text(now),
            warmup_markets: vec![market.clone()],
            ..CalendarRefreshResult::default()
        };
        for source in self.registry.ordered_sources(&market, &policy) {
            let source_id = source.descriptor().id.trim().to_owned();
            if self.in_backoff(&source_id, now)? {
                result.skipped_backoff = result.skipped_backoff.saturating_add(1);
                continue;
            }
            if self.cancellation.is_cancelled() {
                return Ok(result);
            }
            let mut snapshot = match source.fetch(&market, from, to, &self.cancellation) {
                Ok(snapshot) => snapshot,
                Err(error) => {
                    result.failures = result.failures.saturating_add(1);
                    self.record_failure(&source_id, error.to_string())?;
                    continue;
                }
            };
            if snapshot.source_id.trim().is_empty() {
                snapshot.source_id = source_id.clone();
            }
            if snapshot.market_code.trim().is_empty() {
                snapshot.market_code = market.clone();
            }
            if let Err(error) = validate_snapshot(&snapshot) {
                result.failures = result.failures.saturating_add(1);
                self.record_failure(&source_id, error)?;
                continue;
            }
            if let Some(persistence) = &self.persistence
                && let Err(error) = persistence.save(&snapshot)
            {
                result.failures = result.failures.saturating_add(1);
                self.record_failure(&source_id, error)?;
                continue;
            }
            self.cache_snapshot(snapshot.clone())?;
            self.record_success(&snapshot)?;
            result.updated = result.updated.saturating_add(1);
        }
        Ok(result)
    }

    fn cache_snapshot(&self, snapshot: CalendarSnapshot) -> Result<(), CalendarManagerError> {
        let start_year = snapshot.from.into_inner().year();
        let end_year = snapshot.to.into_inner().year().max(start_year);
        let mut snapshots = self
            .snapshots
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        for year in start_year..=end_year {
            snapshots.insert(
                snapshot_key(&snapshot.source_id, &snapshot.market_code, year),
                snapshot.clone(),
            );
        }
        Ok(())
    }

    fn record_success(&self, snapshot: &CalendarSnapshot) -> Result<(), CalendarManagerError> {
        let now = wire_text(self.now());
        let mut statuses = self
            .statuses
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let status = statuses.entry(snapshot.source_id.clone()).or_default();
        status.source_id = snapshot.source_id.clone();
        status.last_success_at = Some(now);
        status.last_failure_at = None;
        status.last_error.clear();
        status.consecutive_failures = 0;
        status.next_refresh_at = None;
        status.last_snapshot_fetched_at = Some(snapshot.fetched_at.to_string());
        status.health_state = "healthy".to_owned();
        Ok(())
    }

    fn record_failure(&self, source_id: &str, error: String) -> Result<(), CalendarManagerError> {
        let now = self.now();
        let mut statuses = self
            .statuses
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let status = statuses.entry(source_id.trim().to_owned()).or_default();
        status.source_id = source_id.trim().to_owned();
        status.last_failure_at = Some(wire_text(now));
        status.last_error = error;
        status.consecutive_failures = status.consecutive_failures.saturating_add(1);
        let hours = i64::from(status.consecutive_failures.clamp(1, 24));
        status.next_refresh_at = now.checked_add(Duration::hours(hours)).map(wire_text);
        status.health_state = "unhealthy".to_owned();
        Ok(())
    }

    fn in_backoff(
        &self,
        source_id: &str,
        now: OffsetDateTime,
    ) -> Result<bool, CalendarManagerError> {
        let statuses = self
            .statuses
            .read()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let Some(next) = statuses
            .get(source_id)
            .and_then(|status| status.next_refresh_at.as_deref())
        else {
            return Ok(false);
        };
        Ok(next
            .parse::<WireTimestamp>()
            .is_ok_and(|next| next.into_inner() > now))
    }

    fn schedule(
        &self,
        market: &str,
        at: WireTimestamp,
    ) -> Result<Option<TradingDaySchedule>, CalendarManagerError> {
        let market = normalize_market(market);
        if !supported_market(&market) {
            return Err(CalendarManagerError::UnsupportedMarket(market));
        }
        let settings = self.settings()?;
        if let Some(schedule) = manual_schedule(&settings, &market, at) {
            return Ok(Some(schedule));
        }
        let policy = policy_for_market(&settings, &market);
        let year = at.into_inner().year();
        let snapshots = self
            .snapshots
            .read()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        for source in self.registry.ordered_sources(&market, &policy) {
            let source_id = source.descriptor().id;
            for candidate in candidate_markets(&market) {
                let Some(snapshot) = snapshots.get(&snapshot_key(&source_id, candidate, year))
                else {
                    continue;
                };
                if !snapshot_fresh(snapshot, &policy, self.now()) {
                    continue;
                }
                if let Some(schedule) = snapshot.schedules.iter().find(|schedule| {
                    same_date(schedule.date, at) && market_matches(&schedule.market_code, candidate)
                }) {
                    let mut schedule = schedule.clone();
                    schedule.market_code = market.clone();
                    schedule.source_id = source_id.clone();
                    return Ok(Some(schedule));
                }
            }
        }
        if policy.fallback_to_builtin {
            return Ok(Some(builtin_schedule(&market, at)));
        }
        Ok(None)
    }
}

fn run_manager(inner: Arc<ManagerInner>, receiver: Receiver<ManagerCommand>) {
    loop {
        let interval = inner
            .settings()
            .map(|settings| settings.refresh_interval_hours.max(1))
            .unwrap_or(24);
        match receiver.recv_timeout(StdDuration::from_secs(
            u64::try_from(interval)
                .unwrap_or(24)
                .saturating_mul(60 * 60),
        )) {
            Ok(ManagerCommand::Stop) | Err(mpsc::RecvTimeoutError::Disconnected) => return,
            Ok(ManagerCommand::Reload) | Err(mpsc::RecvTimeoutError::Timeout) => {
                if inner.cancellation.is_cancelled() {
                    return;
                }
                if inner
                    .settings()
                    .map(|settings| settings.auto_refresh_enabled)
                    .unwrap_or(false)
                {
                    let _ = inner.refresh_all();
                }
            }
        }
    }
}

fn close_sources(sources: &[Arc<dyn CalendarSourcePort>]) {
    for source in sources.iter().rev() {
        let _ = source.close();
    }
}

fn validate_settings(settings: &CalendarManagerSettings) -> Result<(), CalendarManagerError> {
    if settings.refresh_interval_hours < 0 {
        return Err(CalendarManagerError::InvalidSettings(
            "refreshIntervalHours must not be negative".to_owned(),
        ));
    }
    for market in &settings.warmup_markets {
        if !supported_market(&normalize_market(market)) {
            return Err(CalendarManagerError::UnsupportedMarket(market.clone()));
        }
    }
    Ok(())
}

fn normalize_market(market: &str) -> String {
    market.trim().to_uppercase()
}

fn normalized_markets(markets: Vec<String>) -> Vec<String> {
    markets
        .into_iter()
        .map(|market| normalize_market(&market))
        .filter(|market| !market.is_empty())
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect()
}

fn supported_market(market: &str) -> bool {
    matches!(market, "US" | "HK" | "CN" | "SH" | "SZ")
}

fn source_enabled(settings: &CalendarManagerSettings, source_id: &str) -> bool {
    settings.source_policies.iter().any(|policy| {
        policy
            .enabled_source_ids
            .iter()
            .any(|id| id.trim() == source_id)
    })
}

fn policy_for_market(settings: &CalendarManagerSettings, market: &str) -> CalendarSourcePolicy {
    settings
        .source_policies
        .iter()
        .find(|policy| normalize_market(&policy.market) == market)
        .or_else(|| {
            matches!(market, "SH" | "SZ")
                .then(|| {
                    settings
                        .source_policies
                        .iter()
                        .find(|policy| normalize_market(&policy.market) == "CN")
                })
                .flatten()
        })
        .cloned()
        .unwrap_or_else(|| CalendarSourcePolicy {
            market: market.to_owned(),
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        })
}

fn fetch_window(
    now: OffsetDateTime,
) -> Result<(WireTimestamp, WireTimestamp), CalendarManagerError> {
    let offset = now.offset();
    let from_date = Date::from_calendar_date(now.year(), Month::January, 1)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let to_date = Date::from_calendar_date(now.year() + 1, Month::December, 31)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    Ok((
        WireTimestamp::from_offset_datetime(
            from_date.with_time(Time::MIDNIGHT).assume_offset(offset),
        ),
        WireTimestamp::from_offset_datetime(
            to_date
                .with_time(Time::from_hms(23, 59, 59).expect("valid end-of-day time"))
                .assume_offset(offset),
        ),
    ))
}

fn validate_snapshot(snapshot: &CalendarSnapshot) -> Result<(), String> {
    if snapshot.source_id.trim().is_empty() || snapshot.market_code.trim().is_empty() {
        return Err("snapshot marketCode and sourceId are required".to_owned());
    }
    if !supported_market(&normalize_market(&snapshot.market_code)) {
        return Err(format!(
            "unsupported snapshot market {:?}",
            snapshot.market_code
        ));
    }
    if snapshot.to < snapshot.from {
        return Err("snapshot range is invalid".to_owned());
    }
    if snapshot.schedules.is_empty() {
        return Err("no schedules parsed".to_owned());
    }
    if snapshot.schedules.iter().any(|schedule| {
        schedule.date < snapshot.from
            || schedule.date > snapshot.to
            || !market_matches(&schedule.market_code, &snapshot.market_code)
    }) {
        return Err("snapshot schedule is outside its market or range".to_owned());
    }
    Ok(())
}

fn snapshot_key(source_id: &str, market: &str, year: i32) -> String {
    format!(
        "{}|{}|{year:04}",
        source_id.trim(),
        normalize_market(market)
    )
}

fn snapshot_identity(snapshot: &CalendarSnapshot) -> String {
    format!(
        "{}|{}|{}|{}",
        snapshot.source_id.trim(),
        normalize_market(&snapshot.market_code),
        snapshot.from,
        snapshot.to
    )
}

fn candidate_markets(market: &str) -> &'static [&'static str] {
    match market {
        "SH" => &["SH", "CN"],
        "SZ" => &["SZ", "CN"],
        "CN" => &["CN", "SH", "SZ"],
        "HK" => &["HK"],
        _ => &["US"],
    }
}

fn market_matches(left: &str, right: &str) -> bool {
    normalize_market(left) == normalize_market(right)
}

fn same_date(left: WireTimestamp, right: WireTimestamp) -> bool {
    left.into_inner().date() == right.into_inner().date()
}

fn snapshot_fresh(
    snapshot: &CalendarSnapshot,
    policy: &CalendarSourcePolicy,
    now: OffsetDateTime,
) -> bool {
    if snapshot.valid_until.into_inner() < now {
        return false;
    }
    if policy.stale_after_hours > 0
        && snapshot
            .fetched_at
            .into_inner()
            .checked_add(Duration::hours(i64::from(policy.stale_after_hours)))
            .is_none_or(|expiry| expiry < now)
    {
        return false;
    }
    true
}

fn wire_text(at: OffsetDateTime) -> String {
    WireTimestamp::from_offset_datetime(at).to_string()
}
