use std::time::Duration as StdDuration;

use crate::manager::{normalize_market, normalized_markets, policy_for_market, supported_market};
use crate::manager_calendar::{fetch_window_for_market, wire_text};
use crate::{
    CalendarManager, CalendarManagerError, CalendarProbeItem, CalendarProbeResult,
    CalendarSourceRuntimeStatus,
};

const DEFAULT_PROBE_TIMEOUT: StdDuration = StdDuration::from_secs(30);
const DEFAULT_MARKETS: &[&str] = &["US", "HK", "CN"];

impl CalendarManager {
    pub fn probe_market(&self, market: &str) -> Result<CalendarProbeResult, CalendarManagerError> {
        self.probe_market_with_timeout(market, DEFAULT_PROBE_TIMEOUT)
    }

    pub fn probe_all(&self) -> Result<CalendarProbeResult, CalendarManagerError> {
        self.probe_all_with_timeout(DEFAULT_PROBE_TIMEOUT)
    }

    pub fn probe_market_with_timeout(
        &self,
        market: &str,
        timeout: StdDuration,
    ) -> Result<CalendarProbeResult, CalendarManagerError> {
        self.require_running()?;
        let market = normalize_market(market);
        let scope = if matches!(market.as_str(), "CN" | "SH" | "SZ") {
            vec!["CN".to_owned()]
        } else {
            vec![market.clone()]
        };
        self.probe(&market, scope, timeout)
    }

    pub fn probe_all_with_timeout(
        &self,
        timeout: StdDuration,
    ) -> Result<CalendarProbeResult, CalendarManagerError> {
        self.require_running()?;
        let settings = self.inner.settings()?;
        let scope = normalized_scope(if settings.warmup_markets.is_empty() {
            DEFAULT_MARKETS
                .iter()
                .map(|market| (*market).to_owned())
                .collect()
        } else {
            settings.warmup_markets
        });
        self.probe("", scope, timeout)
    }

    fn probe(
        &self,
        target_market: &str,
        scope: Vec<String>,
        timeout: StdDuration,
    ) -> Result<CalendarProbeResult, CalendarManagerError> {
        let checked_at = self.inner.now();
        let settings = self.inner.settings()?;
        let operation = self.inner.cancellation.child_with_timeout(timeout);
        let mut result = CalendarProbeResult {
            accepted: true,
            market: target_market.to_owned(),
            checked_at: wire_text(checked_at),
            probe_scope: scope.clone(),
            ..CalendarProbeResult::default()
        };
        for market in scope {
            if !supported_market(&market) {
                continue;
            }
            let policy = policy_for_market(&settings, &market);
            let (from, to) = fetch_window_for_market(checked_at, &market)?;
            for source in self.inner.registry.ordered_sources(&market, &policy) {
                let source_id = source.descriptor().id.trim().to_owned();
                let fetched = source.fetch(&market, from, to, &operation);
                let item = match fetched {
                    Ok(snapshot) if snapshot.schedules.is_empty() => CalendarProbeItem {
                        source_id: source_id.clone(),
                        market: market.clone(),
                        status: "unhealthy".to_owned(),
                        error: "no schedules parsed".to_owned(),
                        fetched_at: Some(snapshot.fetched_at.to_string()),
                        valid_until: Some(snapshot.valid_until.to_string()),
                        ..CalendarProbeItem::default()
                    },
                    Ok(snapshot) => CalendarProbeItem {
                        source_id: source_id.clone(),
                        market: market.clone(),
                        status: "healthy".to_owned(),
                        fetched_at: Some(snapshot.fetched_at.to_string()),
                        valid_until: Some(snapshot.valid_until.to_string()),
                        schedules_parsed: i32::try_from(snapshot.schedules.len())
                            .unwrap_or(i32::MAX),
                        checksum: snapshot.checksum,
                        ..CalendarProbeItem::default()
                    },
                    Err(error) => CalendarProbeItem {
                        source_id: source_id.clone(),
                        market: market.clone(),
                        status: "unhealthy".to_owned(),
                        error: if operation.is_cancelled() {
                            "calendar source operation timed out or was cancelled".to_owned()
                        } else {
                            error.to_string()
                        },
                        ..CalendarProbeItem::default()
                    },
                };
                if item.status == "healthy" {
                    result.healthy = result.healthy.saturating_add(1);
                    self.record_probe_success(&item)?;
                } else {
                    result.failures = result.failures.saturating_add(1);
                    self.record_probe_failure(&item)?;
                }
                result.results.push(item);
                if operation.is_cancelled() {
                    return Ok(result);
                }
            }
        }
        Ok(result)
    }

    fn record_probe_success(&self, item: &CalendarProbeItem) -> Result<(), CalendarManagerError> {
        self.update_probe_status(item, true)
    }

    fn record_probe_failure(&self, item: &CalendarProbeItem) -> Result<(), CalendarManagerError> {
        self.update_probe_status(item, false)
    }

    fn update_probe_status(
        &self,
        item: &CalendarProbeItem,
        healthy: bool,
    ) -> Result<(), CalendarManagerError> {
        let now = wire_text(self.inner.now());
        let mut statuses = self
            .inner
            .statuses
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let status = statuses
            .entry(item.source_id.clone())
            .or_insert_with(CalendarSourceRuntimeStatus::default);
        status.source_id = item.source_id.clone();
        status.last_probe_at = Some(now.clone());
        status.last_probe_market = item.market.clone();
        status.last_probe_schedules = item.schedules_parsed;
        if healthy {
            let recovered = status.health_state == "unhealthy";
            let previous_fingerprint = status.health_fingerprint.clone();
            status.health_state = "healthy".to_owned();
            status.health_fingerprint.clear();
            status.last_error.clear();
            status.consecutive_failures = 0;
            status.next_refresh_at = None;
            status.last_probe_success_at = Some(now);
            status.last_probe_status = "healthy".to_owned();
            status.last_probe_error.clear();
            if recovered {
                status.last_alert_at = status.last_probe_at.clone();
                status.last_alert_status = "recovered".to_owned();
                status.last_alert_fingerprint = previous_fingerprint;
            }
        } else {
            let kind = if item.error == "no schedules parsed" {
                "structure_changed"
            } else {
                "fetch_failed"
            };
            let detail = if kind == "structure_changed" {
                "structure_changed"
            } else if item.error.contains("timed out") || item.error.contains("cancelled") {
                "network_timeout_or_cancelled"
            } else {
                item.error.trim()
            };
            let fingerprint = format!("{}|{}|{kind}|{detail}", item.source_id, item.market);
            let should_alert =
                status.health_state != "unhealthy" || status.health_fingerprint != fingerprint;
            status.health_state = "unhealthy".to_owned();
            status.health_fingerprint = fingerprint.clone();
            status.last_probe_failure_at = Some(now);
            status.last_probe_status = "unhealthy".to_owned();
            status.last_probe_error = item.error.clone();
            if should_alert {
                status.last_alert_at = status.last_probe_at.clone();
                status.last_alert_status = "triggered".to_owned();
                status.last_alert_fingerprint = fingerprint;
            }
        }
        Ok(())
    }
}

fn normalized_scope(markets: Vec<String>) -> Vec<String> {
    let mut normalized = Vec::new();
    for market in normalized_markets(markets) {
        let market = if matches!(market.as_str(), "SH" | "SZ") {
            "CN".to_owned()
        } else {
            market
        };
        if !normalized.contains(&market) {
            normalized.push(market);
        }
    }
    normalized
}
