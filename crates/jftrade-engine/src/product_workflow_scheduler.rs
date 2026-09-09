//! Background workflow scheduler engine.
//!
//! Periodically polls due schedule triggers (`cron`) and evaluates market
//! threshold triggers against live/snapshot market data, launching background
//! workflow invocations and updating `lastRunAt` and `nextRunAt` states.

use std::sync::{Arc, Mutex, Weak};
use std::time::Duration;

use jftrade_store_sqlite::{AdkStore, StoredAdkWorkflowTrigger as WorkflowTriggerRow};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

use crate::product::MarketDataQuoteReadSnapshotPort;
use crate::product::product_production_ports::ProductionAdkPort;
use crate::product_workflow_cron::next_run_at_string;
use crate::product_workflow_threshold::evaluate_market_threshold_trigger;

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowSchedulerStatus {
    pub state: String,
    pub ticks: u64,
    pub schedule_triggers_fired: u64,
    pub threshold_triggers_fired: u64,
    pub last_tick_at: Option<String>,
    pub last_error: Option<String>,
}

#[derive(Clone, Debug, Default)]
pub struct SchedulerTickResult {
    pub schedule_triggers_evaluated: usize,
    pub schedule_triggers_fired: usize,
    pub threshold_triggers_evaluated: usize,
    pub threshold_triggers_fired: usize,
    pub errors: Vec<String>,
}

pub struct WorkflowScheduler {
    jobs: crate::product_workflow_jobs::WorkflowJobs,
    store: Arc<AdkStore>,
    adk_port: Weak<ProductionAdkPort>,
    quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    interval: Duration,
    stop_tx: Mutex<Option<oneshot::Sender<()>>>,
    handle: Mutex<Option<JoinHandle<()>>>,
    status: Arc<Mutex<WorkflowSchedulerStatus>>,
}

impl std::fmt::Debug for WorkflowScheduler {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WorkflowScheduler")
            .field("interval", &self.interval)
            .field("status", &self.status())
            .finish_non_exhaustive()
    }
}

impl WorkflowScheduler {
    pub fn start(
        store: Arc<AdkStore>,
        adk_port: Arc<ProductionAdkPort>,
        quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
        interval: Duration,
    ) -> Arc<Self> {
        let weak_port = Arc::downgrade(&adk_port);
        let status = Arc::new(Mutex::new(WorkflowSchedulerStatus {
            state: "ready".to_owned(),
            ..Default::default()
        }));
        let (stop_tx, mut stop_rx) = oneshot::channel();
        let scheduler = Arc::new(Self {
            jobs: Default::default(),
            store,
            adk_port: weak_port,
            quote_port,
            interval,
            stop_tx: Mutex::new(Some(stop_tx)),
            handle: Mutex::new(None),
            status: Arc::clone(&status),
        });

        let weak_scheduler = Arc::downgrade(&scheduler);
        let weak_status = Arc::downgrade(&status);
        let handle = tokio::spawn(async move {
            let mut ticker = tokio::time::interval(interval);
            loop {
                tokio::select! {
                    _ = &mut stop_rx => {
                        break;
                    }
                    _ = ticker.tick() => {
                        let Some(scheduler_ref) = weak_scheduler.upgrade() else {
                            break;
                        };
                        let now = OffsetDateTime::now_utc();
                        scheduler_ref.tick(now).await;
                    }
                }
            }
            if let Some(status_ref) = weak_status.upgrade()
                && let Ok(mut s) = status_ref.lock()
            {
                s.state = "stopped".to_owned();
            }
        });

        if let Ok(mut h) = scheduler.handle.lock() {
            *h = Some(handle);
        }

        scheduler
    }

    pub fn new_for_test(
        store: Arc<AdkStore>,
        adk_port: Arc<ProductionAdkPort>,
        quote_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    ) -> Self {
        Self {
            jobs: Default::default(),
            store,
            adk_port: Arc::downgrade(&adk_port),
            quote_port,
            interval: Duration::from_secs(30),
            stop_tx: Mutex::new(None),
            handle: Mutex::new(None),
            status: Arc::new(Mutex::new(WorkflowSchedulerStatus {
                state: "ready".to_owned(),
                ..Default::default()
            })),
        }
    }

    pub fn stop(&self) {
        self.jobs.stop();
        if let Ok(mut tx_guard) = self.stop_tx.lock()
            && let Some(tx) = tx_guard.take()
        {
            let _ = tx.send(());
        }
        if let Ok(mut handle_guard) = self.handle.lock()
            && let Some(handle) = handle_guard.take()
        {
            handle.abort();
        }
        if let Ok(mut s) = self.status.lock() {
            s.state = "stopping".to_owned();
        }
    }

    pub fn join_invocations(&self, timeout: Duration) -> bool {
        let stopped = self.jobs.join(timeout);
        if let Ok(mut s) = self.status.lock() {
            s.state = if stopped { "stopped" } else { "stopping" }.to_owned();
        }
        stopped
    }

    pub fn status(&self) -> WorkflowSchedulerStatus {
        self.status.lock().map(|s| s.clone()).unwrap_or_default()
    }

    pub async fn tick(&self, now: OffsetDateTime) -> SchedulerTickResult {
        let mut result = SchedulerTickResult::default();
        if self.jobs.is_stopping() {
            return result;
        }
        self.resume_pending_workflows(&mut result);
        self.poll_schedule_triggers(now, &mut result);
        self.poll_market_threshold_triggers(now, &mut result).await;

        let now_str = now.format(&Rfc3339).unwrap_or_default();
        if let Ok(mut state) = self.status.lock() {
            state.ticks = state.ticks.saturating_add(1);
            state.schedule_triggers_fired = state
                .schedule_triggers_fired
                .saturating_add(result.schedule_triggers_fired as u64);
            state.threshold_triggers_fired = state
                .threshold_triggers_fired
                .saturating_add(result.threshold_triggers_fired as u64);
            state.last_tick_at = Some(now_str);
            if let Some(err) = result.errors.last() {
                state.state = "degraded".to_owned();
                state.last_error = Some(err.clone());
            } else {
                state.state = "ready".to_owned();
            }
        }

        result
    }

    fn poll_schedule_triggers(&self, now: OffsetDateTime, result: &mut SchedulerTickResult) {
        let now_iso = now.format(&Rfc3339).unwrap_or_default();
        let triggers = match self.store.list_due_workflow_schedule_triggers(&now_iso, 50) {
            Ok(triggers) => triggers,
            Err(error) => {
                result
                    .errors
                    .push(format!("list due triggers failed: {error}"));
                return;
            }
        };

        result.schedule_triggers_evaluated = triggers.len();
        for trigger in triggers {
            self.process_schedule_trigger(trigger, now, &now_iso, result);
        }
    }

    fn process_schedule_trigger(
        &self,
        trigger: WorkflowTriggerRow,
        now: OffsetDateTime,
        now_iso: &str,
        result: &mut SchedulerTickResult,
    ) {
        let config = match serde_json::from_str::<Value>(&trigger.payload_json) {
            Ok(val) => val.get("config").cloned().unwrap_or_else(|| json!({})),
            Err(e) => {
                result
                    .errors
                    .push(format!("decode trigger payload {}: {e}", trigger.id));
                return;
            }
        };

        let workflow_valid = match self.store.get_workflow(&trigger.workflow_id) {
            Ok(Some(wf)) => workflow_enabled(&wf.status, &wf.payload_json),
            _ => false,
        };

        let next_run = match next_run_at_string(&config, now) {
            Ok(next) => next,
            Err(error) => {
                result
                    .errors
                    .push(format!("trigger {}: {error}", trigger.id));
                return;
            }
        };
        if !workflow_valid {
            let mut payload: Value =
                serde_json::from_str(&trigger.payload_json).unwrap_or_default();
            payload["lastRunAt"] = json!(now_iso);
            payload["nextRunAt"] = json!(next_run);
            payload["lastError"] = json!("workflow is disabled or missing");
            if let Err(error) =
                self.store
                    .enqueue_workflow_trigger_invocations(&trigger, &payload, &next_run, &[])
            {
                result.errors.push(error.to_string());
            }
            return;
        }

        let mut payload: Value = match serde_json::from_str(&trigger.payload_json) {
            Ok(payload) => payload,
            Err(error) => {
                result.errors.push(error.to_string());
                return;
            }
        };
        payload["lastRunAt"] = json!(now_iso);
        payload["nextRunAt"] = json!(next_run);
        let id = crate::product_id::generate_prefixed_id("workflow-log");
        let queued = [(id.clone(), json!({"scheduledAt":now_iso}))];
        match self
            .store
            .enqueue_workflow_trigger_invocations(&trigger, &payload, &next_run, &queued)
        {
            Ok(true) => {
                self.launch_queued(&id);
                result.schedule_triggers_fired += 1;
            }
            Ok(false) => {}
            Err(error) => result.errors.push(error.to_string()),
        }
    }

    async fn poll_market_threshold_triggers(
        &self,
        now: OffsetDateTime,
        result: &mut SchedulerTickResult,
    ) {
        let triggers = match self
            .store
            .list_enabled_workflow_triggers_by_type("market_threshold")
        {
            Ok(triggers) => triggers,
            Err(error) => {
                result
                    .errors
                    .push(format!("list threshold triggers failed: {error}"));
                return;
            }
        };

        result.threshold_triggers_evaluated = triggers.len();
        for trigger in triggers {
            self.process_market_threshold_trigger(trigger, now, result)
                .await;
        }
    }

    async fn process_market_threshold_trigger(
        &self,
        trigger: WorkflowTriggerRow,
        now: OffsetDateTime,
        result: &mut SchedulerTickResult,
    ) {
        let mut payload = match serde_json::from_str::<Value>(&trigger.payload_json) {
            Ok(val) => val,
            Err(e) => {
                result
                    .errors
                    .push(format!("decode threshold payload {}: {e}", trigger.id));
                return;
            }
        };

        let mut config = payload.get("config").cloned().unwrap_or_else(|| json!({}));
        let events = self.fetch_market_events(&config, now).await;
        let (matches, changed) = evaluate_market_threshold_trigger(&mut config, &events, now);

        if !changed {
            return;
        }
        let enabled = self
            .store
            .get_workflow(&trigger.workflow_id)
            .ok()
            .flatten()
            .is_some_and(|w| workflow_enabled(&w.status, &w.payload_json));
        let queued: Vec<_> = if enabled {
            matches
                .iter()
                .map(|m| {
                    (
                        crate::product_id::generate_prefixed_id("workflow-log"),
                        json!({
                            "event":m.event, "threshold":m.threshold,
                        }),
                    )
                })
                .collect()
        } else {
            Vec::new()
        };
        payload["config"] = config;
        if !queued.is_empty() {
            payload["lastRunAt"] = json!(now.format(&Rfc3339).unwrap_or_default());
        }
        match self.store.enqueue_workflow_trigger_invocations(
            &trigger,
            &payload,
            &trigger.next_run_at,
            &queued,
        ) {
            Ok(true) => {
                for (id, _) in &queued {
                    self.launch_queued(id);
                }
                result.threshold_triggers_fired += queued.len();
            }
            Ok(false) => {}
            Err(error) => result.errors.push(error.to_string()),
        }
    }

    async fn fetch_market_events(&self, config: &Value, now: OffsetDateTime) -> Vec<Value> {
        let Some(quote_port) = &self.quote_port else {
            return Vec::new();
        };

        let instrument_ids = extract_instrument_ids(config);
        let now_iso = now.format(&Rfc3339).unwrap_or_default();
        let mut events = Vec::with_capacity(instrument_ids.len());

        for id in instrument_ids {
            let path = instrument_to_snapshot_path(&id);
            if let Ok(snapshot) = quote_port.read(&path, "").await {
                events.push(json!({
                    "type": "market-data.tick",
                    "source": "workflow.poll",
                    "entityId": id.to_ascii_uppercase(),
                    "at": now_iso,
                    "instrument": { "instrumentId": id.to_ascii_uppercase() },
                    "payload": snapshot,
                }));
            }
        }

        events
    }

    fn launch_queued(&self, log_id: &str) {
        let Some(port) = self.adk_port.upgrade() else {
            return;
        };
        let id = log_id.to_owned();
        self.jobs.spawn(format!("invocation:{id}"), move || {
            if let Err(error) = port.run_queued_workflow(&id) {
                tracing::warn!(%error, "queued workflow failed");
            }
        });
    }

    fn resume_pending_workflows(&self, result: &mut SchedulerTickResult) {
        let logs = match self.store.list_workflow_trigger_logs() {
            Ok(logs) => logs,
            Err(e) => {
                result.errors.push(e.to_string());
                return;
            }
        };
        for log in logs {
            if !matches!(
                log.status.as_str(),
                "RUNNING" | "PENDING_APPROVAL" | "PENDING_INPUT" | "QUEUED"
            ) {
                continue;
            }
            let Ok(payload) = serde_json::from_str::<Value>(&log.payload_json) else {
                continue;
            };
            if log.status == "QUEUED"
                && payload
                    .get("schedulerInvocation")
                    .is_some_and(Value::is_object)
            {
                self.launch_queued(&log.id);
                continue;
            }
            if !payload.get("canvasExecution").is_some_and(Value::is_object) {
                continue;
            }
            let Some(port) = self.adk_port.upgrade() else {
                break;
            };
            self.jobs.spawn(format!("resume:{}", log.id), move || {
                if let Err(error) = port.resume_workflow(&log.id) {
                    tracing::warn!(%error, "workflow resume failed");
                }
            });
        }
    }
}

impl Drop for WorkflowScheduler {
    fn drop(&mut self) {
        self.stop();
    }
}

fn extract_instrument_ids(config: &Value) -> Vec<String> {
    let mut list = Vec::new();
    let Some(raw) = config.get("instrumentIds") else {
        return list;
    };
    match raw {
        Value::Array(items) => {
            for item in items {
                if let Some(s) = item.as_str() {
                    let trimmed = s.trim();
                    if !trimmed.is_empty() {
                        list.push(trimmed.to_owned());
                    }
                }
            }
        }
        Value::String(s) => {
            for part in s.split(',') {
                let trimmed = part.trim();
                if !trimmed.is_empty() {
                    list.push(trimmed.to_owned());
                }
            }
        }
        _ => {}
    }
    list
}

fn instrument_to_snapshot_path(instrument_id: &str) -> String {
    let trimmed = instrument_id.trim();
    if let Some((market, symbol)) = trimmed.split_once('.') {
        format!("/api/v1/market-data/snapshots/{market}/{symbol}")
    } else if let Some((market, symbol)) = trimmed.split_once('/') {
        format!("/api/v1/market-data/snapshots/{market}/{symbol}")
    } else {
        format!("/api/v1/market-data/snapshots/US/{trimmed}")
    }
}

fn workflow_enabled(status: &str, raw: &str) -> bool {
    status.eq_ignore_ascii_case("ENABLED")
        && serde_json::from_str::<Value>(raw)
            .ok()
            .is_some_and(|value| {
                value
                    .get("deletedAt")
                    .is_none_or(|v| v.is_null() || v.as_str() == Some(""))
            })
}
