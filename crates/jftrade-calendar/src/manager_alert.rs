use time::Duration;

use crate::manager::{ManagerInner, normalize_market, source_alert_fingerprint};
use crate::manager_calendar::wire_text;
use crate::{CalendarManagerError, CalendarSnapshot, CalendarSourceAlert};

impl ManagerInner {
    /// Go's `recordSuccess`: clears the failure state and publishes a recovery
    /// alert when the source was previously unhealthy.
    pub(crate) fn record_success(
        &self,
        snapshot: &CalendarSnapshot,
    ) -> Result<(), CalendarManagerError> {
        let now = wire_text(self.now());
        let mut statuses = self
            .statuses
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let status = statuses.entry(snapshot.source_id.clone()).or_default();
        status.source_id = snapshot.source_id.clone();
        status.last_success_at = Some(now.clone());
        status.last_failure_at = None;
        status.last_error.clear();
        status.consecutive_failures = 0;
        status.next_refresh_at = None;
        status.last_snapshot_fetched_at = Some(snapshot.fetched_at.to_string());
        status.last_probe_error.clear();
        let previous_fingerprint = std::mem::take(&mut status.health_fingerprint);
        let recovered = status.health_state == "unhealthy";
        status.health_state = "healthy".to_owned();
        if recovered {
            status.last_alert_at = Some(now);
            status.last_alert_status = "recovered".to_owned();
            status.last_alert_fingerprint = previous_fingerprint;
        }
        let alert = recovered.then(|| {
            recovery_alert(
                &snapshot.source_id,
                &snapshot.market_code,
                &status.last_alert_fingerprint,
            )
        });
        drop(statuses);
        self.emit_alert(alert);
        Ok(())
    }

    /// Go's `recordOperationFailure`: a durable-store or restore problem. It
    /// grows the retry ladder and records the error, but deliberately leaves the
    /// provider health state and alert bookkeeping alone.
    pub(crate) fn record_operation_failure(
        &self,
        source_id: &str,
        error: String,
    ) -> Result<(), CalendarManagerError> {
        self.record_failure_state(source_id, Some(error), None)
    }

    /// Go's `recordSourceFailure`: a provider fetch/parse failure. It upgrades
    /// the source to `unhealthy` and publishes a fingerprint-deduplicated alert
    /// through `sourceFailureAlert` (`kind` is `fetch_failed` or
    /// `structure_changed`).
    pub(crate) fn record_source_failure(
        &self,
        source_id: &str,
        market: &str,
        error: String,
        kind: &str,
    ) -> Result<(), CalendarManagerError> {
        self.record_failure_state(source_id, Some(error.clone()), Some((market, error, kind)))
    }

    pub(crate) fn record_failure_state(
        &self,
        source_id: &str,
        error: Option<String>,
        alert: Option<(&str, String, &str)>,
    ) -> Result<(), CalendarManagerError> {
        let now = self.now();
        let market = alert
            .as_ref()
            .map_or_else(String::new, |(market, _, _)| normalize_market(market));
        let fingerprint = alert
            .as_ref()
            .map(|(_, message, kind)| source_alert_fingerprint(source_id, &market, kind, message));
        let mut statuses = self
            .statuses
            .write()
            .map_err(|_| CalendarManagerError::StateUnavailable)?;
        let status = statuses.entry(source_id.trim().to_owned()).or_default();
        status.source_id = source_id.trim().to_owned();
        status.last_failure_at = Some(wire_text(now));
        if let Some(error) = error {
            status.last_error = error;
        }
        status.consecutive_failures = status.consecutive_failures.saturating_add(1);
        let hours = i64::from(status.consecutive_failures.clamp(1, 24));
        status.next_refresh_at = now.checked_add(Duration::hours(hours)).map(wire_text);
        let mut notification = None;
        if let Some(fingerprint) = fingerprint {
            let should_alert =
                status.health_state != "unhealthy" || status.health_fingerprint != fingerprint;
            status.health_state = "unhealthy".to_owned();
            status.health_fingerprint = fingerprint.clone();
            if should_alert {
                status.last_alert_at = status.last_failure_at.clone();
                status.last_alert_status = "triggered".to_owned();
                status.last_alert_fingerprint = fingerprint;
                if let Some((market, message, kind)) = alert {
                    notification = Some(failure_alert(source_id, market, kind, &message));
                }
            }
        }
        drop(statuses);
        self.emit_alert(notification);
        Ok(())
    }

    pub(crate) fn emit_alert(&self, alert: Option<CalendarSourceAlert>) {
        if let Some(sink) = &self.alert_sink
            && let Some(alert) = alert
        {
            sink(alert);
        }
    }
}

pub(crate) fn recovery_alert(
    source_id: &str,
    market: &str,
    fingerprint: &str,
) -> CalendarSourceAlert {
    let market = normalize_market(market);
    CalendarSourceAlert {
        source_id: source_id.to_owned(),
        market: market.clone(),
        level: "success".to_owned(),
        kind: "recovered".to_owned(),
        title: "交易所日历源已恢复".to_owned(),
        message: format!("{market} 市场日历源 {source_id} 已恢复正常解析。"),
        fingerprint: fingerprint.to_owned(),
    }
}

pub(crate) fn failure_alert(
    source_id: &str,
    market: &str,
    kind: &str,
    message: &str,
) -> CalendarSourceAlert {
    let market = normalize_market(market);
    let structure_changed = kind == "structure_changed";
    let detail = if message.trim().is_empty() {
        "unknown error"
    } else {
        message.trim()
    };
    CalendarSourceAlert {
        source_id: source_id.trim().to_owned(),
        market: market.clone(),
        level: if structure_changed { "error" } else { "warn" }.to_owned(),
        kind: if structure_changed {
            "structure_changed"
        } else {
            "fetch_failed"
        }
        .to_owned(),
        title: if structure_changed {
            "交易所日历源解析异常"
        } else {
            "交易所日历源抓取失败"
        }
        .to_owned(),
        message: if structure_changed {
            format!(
                "{market} 市场日历源 {source_id} 抓取成功但未解析到有效交易日，可能是官网结构发生变化。系统将继续回退到内置日历。"
            )
        } else {
            format!(
                "{market} 市场日历源 {source_id} 抓取失败：{detail}。系统将继续回退到内置日历。"
            )
        },
        fingerprint: source_alert_fingerprint(source_id, &market, kind, message),
    }
}
