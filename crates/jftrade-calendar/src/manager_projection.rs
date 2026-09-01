use std::collections::BTreeMap;

use crate::manager::{
    normalize_market, normalized_markets, policy_for_market, policy_source_ids, wire_text,
};
use crate::{
    BUILTIN_SOURCE_ID, CalendarManager, CalendarManagerError, CalendarMarketStatus,
    CalendarSampleSchedule, CalendarSampleSession, CalendarSnapshot, CalendarSnapshotSummary,
    CalendarSourceDescriptor, CalendarSourceRuntimeStatus, CalendarSourceStatus,
    CalendarSourcesSnapshot, CalendarStatusSnapshot, MANUAL_OVERRIDE_SOURCE_ID,
    default_source_descriptors, project_sources,
};

const ZERO_TIME: &str = "0001-01-01T00:00:00Z";
const DEFAULT_MARKETS: &[&str] = &["US", "HK", "CN"];

impl CalendarManager {
    pub fn sources_snapshot(&self) -> Result<CalendarSourcesSnapshot, CalendarManagerError> {
        let settings = self.inner.settings()?;
        let descriptors = self.source_descriptors();
        let statuses = self.source_status_map(false)?;
        let enabled = settings
            .source_policies
            .into_iter()
            .flat_map(|policy| policy.enabled_source_ids)
            .collect::<Vec<_>>();
        Ok(CalendarSourcesSnapshot {
            sources: project_sources(descriptors, enabled, &statuses),
        })
    }

    pub fn status_snapshot(&self) -> Result<CalendarStatusSnapshot, CalendarManagerError> {
        let settings = self.inner.settings()?;
        let now = self.inner.now();
        let warmup_markets = normalized_markets(settings.warmup_markets.clone());
        let markets = normalized_markets(if warmup_markets.is_empty() {
            DEFAULT_MARKETS
                .iter()
                .map(|market| (*market).to_owned())
                .collect()
        } else {
            warmup_markets.clone()
        });
        let descriptors = self.source_descriptors();
        let mut statuses = self.source_status_map(true)?;
        for descriptor in &descriptors {
            statuses.entry(descriptor.id.clone()).or_insert_with(|| {
                projected_status(
                    &CalendarSourceRuntimeStatus {
                        source_id: descriptor.id.clone(),
                        ..CalendarSourceRuntimeStatus::default()
                    },
                    true,
                )
            });
        }
        let enabled = settings
            .source_policies
            .iter()
            .flat_map(|policy| policy.enabled_source_ids.clone())
            .collect::<Vec<_>>();
        let sources = project_sources(descriptors, enabled, &statuses);
        let snapshots = self.snapshots()?;
        let market_rows = markets
            .iter()
            .map(|market| market_status(self, market, &settings, snapshots.iter(), wire_text(now)))
            .collect::<Result<Vec<_>, _>>()?;
        let snapshot_rows = snapshot_summaries(snapshots.iter());
        Ok(CalendarStatusSnapshot {
            auto_refresh_enabled: settings.auto_refresh_enabled,
            refresh_interval_hours: settings.refresh_interval_hours,
            warmup_markets,
            markets: market_rows,
            sources,
            snapshots: snapshot_rows,
        })
    }

    fn source_descriptors(&self) -> Vec<CalendarSourceDescriptor> {
        let mut descriptors = default_source_descriptors()
            .into_iter()
            .filter(|descriptor| {
                matches!(
                    descriptor.id.as_str(),
                    BUILTIN_SOURCE_ID | MANUAL_OVERRIDE_SOURCE_ID
                )
            })
            .collect::<Vec<_>>();
        descriptors.extend(self.inner.registry.descriptors());
        let mut unique = BTreeMap::new();
        for descriptor in descriptors {
            unique.insert(descriptor.id.clone(), descriptor);
        }
        unique.into_values().collect()
    }

    fn source_status_map(
        &self,
        include_zero_times: bool,
    ) -> Result<BTreeMap<String, CalendarSourceStatus>, CalendarManagerError> {
        let statuses = self
            .inner
            .statuses
            .read()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        Ok(statuses
            .iter()
            .map(|(source_id, status)| {
                (
                    source_id.clone(),
                    projected_status(status, include_zero_times),
                )
            })
            .collect())
    }
}

fn projected_status(
    status: &CalendarSourceRuntimeStatus,
    include_zero_times: bool,
) -> CalendarSourceStatus {
    let zero = || include_zero_times.then(|| ZERO_TIME.to_owned());
    CalendarSourceStatus {
        source_id: status.source_id.clone(),
        enabled: status.enabled,
        last_success_at: status.last_success_at.clone().or_else(zero),
        last_failure_at: status.last_failure_at.clone().or_else(zero),
        last_error: status.last_error.clone(),
        consecutive_failures: status.consecutive_failures,
        next_refresh_at: status.next_refresh_at.clone().or_else(zero),
        last_snapshot_fetched_at: status.last_snapshot_fetched_at.clone().or_else(zero),
        last_probe_at: status.last_probe_at.clone().or_else(zero),
        last_probe_success_at: status.last_probe_success_at.clone().or_else(zero),
        last_probe_failure_at: status.last_probe_failure_at.clone().or_else(zero),
        last_probe_status: status.last_probe_status.clone(),
        last_probe_error: status.last_probe_error.clone(),
        last_probe_market: status.last_probe_market.clone(),
        last_probe_schedules: status.last_probe_schedules,
        health_state: status.health_state.clone(),
        health_fingerprint: status.health_fingerprint.clone(),
        last_alert_at: status.last_alert_at.clone().or_else(zero),
        last_alert_status: status.last_alert_status.clone(),
        last_alert_fingerprint: status.last_alert_fingerprint.clone(),
    }
}

fn market_status<'a>(
    manager: &CalendarManager,
    market: &str,
    settings: &crate::CalendarManagerSettings,
    snapshots: impl Iterator<Item = &'a CalendarSnapshot>,
    checked_at: String,
) -> Result<CalendarMarketStatus, CalendarManagerError> {
    let schedule = manager.schedule(
        market,
        jftrade_kernel::WireTimestamp::from_offset_datetime(manager.inner.now()),
    )?;
    let policy = policy_for_market(settings, market);
    let covered = snapshots
        .filter(|snapshot| normalize_market(&snapshot.market_code) == market)
        .find(|snapshot| {
            let now = manager.inner.now();
            snapshot.from.into_inner() <= now
                && snapshot.to.into_inner() >= now
                && crate::manager::snapshot_fresh(snapshot, &policy, now)
        });
    let (source, mode) = match schedule.as_ref().map(|value| value.source_id.as_str()) {
        Some(MANUAL_OVERRIDE_SOURCE_ID) => {
            (MANUAL_OVERRIDE_SOURCE_ID.to_owned(), "manual_override")
        }
        Some(source) if source != BUILTIN_SOURCE_ID => (source.to_owned(), "remote_override"),
        _ if covered.is_some() => (
            covered.expect("covered snapshot").source_id.clone(),
            "remote_covered_day",
        ),
        _ => (BUILTIN_SOURCE_ID.to_owned(), "builtin_fallback"),
    };
    let mut fallback_chain = vec![MANUAL_OVERRIDE_SOURCE_ID.to_owned()];
    fallback_chain.extend(manager.inner.registry.ordered_source_ids(market, &policy));
    for source_id in policy_source_ids(&policy) {
        if !fallback_chain
            .iter()
            .any(|candidate| candidate == &source_id)
        {
            fallback_chain.push(source_id);
        }
    }
    if (policy.fallback_to_builtin || fallback_chain.len() == 1)
        && !fallback_chain
            .iter()
            .any(|source| source == BUILTIN_SOURCE_ID)
    {
        fallback_chain.push(BUILTIN_SOURCE_ID.to_owned());
    }
    let enabled_external = fallback_chain.iter().any(|source| {
        !matches!(
            source.as_str(),
            MANUAL_OVERRIDE_SOURCE_ID | BUILTIN_SOURCE_ID
        )
    });
    Ok(CalendarMarketStatus {
        market: market.to_owned(),
        effective_source: source,
        effective_mode: mode.to_owned(),
        effective_reason: effective_reason(mode, enabled_external),
        fallback_chain,
        checked_at,
    })
}

fn effective_reason(mode: &str, enabled_external: bool) -> String {
    match mode {
        "manual_override" => "manual override is active for the checked trading day",
        "remote_override" => "a fresh source snapshot covers the checked trading day",
        "remote_covered_day" => "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override",
        _ if !enabled_external => "current policy uses builtin_rules because no external source is enabled for this market",
        _ => "builtin_rules is serving this market because no fresh external snapshot currently covers the checked trading day",
    }
    .to_owned()
}

fn snapshot_summaries<'a>(
    snapshots: impl Iterator<Item = &'a CalendarSnapshot>,
) -> Vec<CalendarSnapshotSummary> {
    let mut unique = BTreeMap::new();
    for snapshot in snapshots {
        if snapshot.source_id.trim().is_empty() || snapshot.source_id == BUILTIN_SOURCE_ID {
            continue;
        }
        let key = format!(
            "{}|{}|{}|{}|{}",
            normalize_market(&snapshot.market_code),
            snapshot.source_id.trim(),
            snapshot.from,
            snapshot.to,
            snapshot.checksum
        );
        unique.insert(key, snapshot_summary(snapshot));
    }
    unique.into_values().collect()
}

fn snapshot_summary(snapshot: &CalendarSnapshot) -> CalendarSnapshotSummary {
    let sample_schedules = snapshot
        .schedules
        .iter()
        .filter(|schedule| schedule.status != "open")
        .take(8)
        .map(|schedule| CalendarSampleSchedule {
            market: normalize_market(&schedule.market_code),
            date: schedule.date.into_inner().date().to_string(),
            status: schedule.status.clone(),
            reason: schedule.reason.clone(),
            source_id: schedule.source_id.trim().to_owned(),
            observed: schedule.observed,
            sessions: (!schedule.sessions.is_empty()).then(|| {
                schedule
                    .sessions
                    .iter()
                    .map(|session| CalendarSampleSession {
                        kind: session.kind.clone(),
                        start_minute: session.start_minute,
                        end_minute: session.end_minute,
                    })
                    .collect()
            }),
        })
        .collect();
    CalendarSnapshotSummary {
        market: normalize_market(&snapshot.market_code),
        source_id: snapshot.source_id.trim().to_owned(),
        from: snapshot.from.to_string(),
        to: snapshot.to.to_string(),
        fetched_at: snapshot.fetched_at.to_string(),
        valid_until: snapshot.valid_until.to_string(),
        schedules_parsed: i32::try_from(snapshot.schedules.len()).unwrap_or(i32::MAX),
        checksum: snapshot.checksum.clone(),
        sample_schedules,
    }
}
