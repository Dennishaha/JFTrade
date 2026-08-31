//! Durable production projection for `GET /api/v1/system/storage/overview`.
//!
//! The historical Go endpoint returned four arrays even though its queues
//! were disabled.  The production Rust endpoint keeps that wire shape while
//! projecting rows from the already-leased stores that own the work.  No
//! process-local queue, fixture, or synthetic success value is introduced.

use std::fmt::Display;

use jftrade_store_sqlite::{
    StoredAdkRun, StoredAdkTask, StoredBacktestSyncTask, StoredExecutionOrder,
    StoredExecutionOrderEventRecord, StoredStrategyAuditEvent,
};
use serde_json::{Value, json};

use super::product_production_ports_system::ProductionSystemPort;
use crate::product::SystemReadSnapshotError;

const STORAGE_PROJECTION_LIMIT: usize = 100;

impl ProductionSystemPort {
    /// Read all four storage-overview projections from the durable stores
    /// already owned by the production composition.  Every store error is
    /// surfaced as unavailable instead of being hidden behind an empty list.
    pub(crate) fn storage_overview_snapshot(&self) -> Result<Value, SystemReadSnapshotError> {
        let sync_tasks = self
            .backtest_sync_tasks
            .list_all()
            .map_err(|error| storage_error("backtest sync tasks", error))?;
        let backtest_runs = self
            .backtest_store
            .list_runs()
            .map_err(|error| storage_error("backtest runs", error))?;
        let adk_tasks = self
            .adk_store
            .list_tasks()
            .map_err(|error| storage_error("ADK tasks", error))?;
        let adk_runs = self
            .adk_store
            .list_runs()
            .map_err(|error| storage_error("ADK runs", error))?;
        let execution_orders = self
            .execution_store
            .list_orders()
            .map_err(|error| storage_error("execution orders", error))?;

        let mut pending_outbox = Vec::new();
        for order in &execution_orders {
            if is_pending_status(&order.status) {
                pending_outbox.push(project_execution_order(order)?);
            }
        }
        for task in &sync_tasks {
            if is_pending_status(&task.status) {
                pending_outbox.push(project_sync_task(task)?);
            }
        }
        for task in &adk_tasks {
            if is_pending_status(&task.status) {
                pending_outbox.push(project_adk_task(task)?);
            }
        }
        for run in &adk_runs {
            if is_pending_status(&run.status) {
                pending_outbox.push(project_adk_run(run)?);
            }
        }

        let mut recent_jobs = Vec::new();
        recent_jobs.extend(
            sync_tasks
                .iter()
                .map(project_sync_task)
                .collect::<Result<Vec<_>, _>>()?,
        );
        recent_jobs.extend(
            backtest_runs
                .iter()
                .map(project_backtest_run)
                .collect::<Result<Vec<_>, _>>()?,
        );
        recent_jobs.extend(
            adk_tasks
                .iter()
                .map(project_adk_task)
                .collect::<Result<Vec<_>, _>>()?,
        );
        recent_jobs.extend(
            adk_runs
                .iter()
                .map(project_adk_run)
                .collect::<Result<Vec<_>, _>>()?,
        );
        sort_recent(&mut pending_outbox);
        sort_recent(&mut recent_jobs);

        let mut recent_audit_logs = self.adk_audit_logs()?;
        recent_audit_logs.extend(self.strategy_audit_logs()?);
        sort_recent(&mut recent_audit_logs);

        let mut recent_execution_commands = Vec::new();
        for order in &execution_orders {
            let events = self
                .execution_store
                .list_order_events(&order.internal_order_id)
                .map_err(|error| storage_error("execution order events", error))?;
            if events.is_empty() {
                recent_execution_commands.push(project_execution_order(order)?);
            } else {
                recent_execution_commands.extend(
                    events
                        .iter()
                        .map(project_execution_event)
                        .collect::<Result<Vec<_>, _>>()?,
                );
            }
        }
        sort_recent(&mut recent_execution_commands);

        Ok(json!({
            "pendingOutbox": limited(pending_outbox),
            "recentJobs": limited(recent_jobs),
            "recentAuditLogs": limited(recent_audit_logs),
            "recentExecutionCommands": limited(recent_execution_commands),
        }))
    }

    fn adk_audit_logs(&self) -> Result<Vec<Value>, SystemReadSnapshotError> {
        self.adk_store
            .list_audit_events()
            .map_err(|error| storage_error("ADK audit events", error))?
            .iter()
            .map(|event| {
                Ok(json!({
                    "id": event.id,
                    "kind": event.kind,
                    "subjectId": event.subject_id,
                    "payload": parse_json_field(&event.payload_json, "ADK audit payload")?,
                    "createdAt": event.created_at,
                }))
            })
            .collect()
    }

    fn strategy_audit_logs(&self) -> Result<Vec<Value>, SystemReadSnapshotError> {
        let instances = self
            .strategy_runtime_store
            .list_instances()
            .map_err(|error| storage_error("strategy runtime instances", error))?;
        let mut events = Vec::new();
        for instance in instances {
            let rows = self
                .strategy_runtime_store
                .list_audit_events(&instance.id)
                .map_err(|error| storage_error("strategy audit events", error))?;
            events.extend(rows.iter().map(project_strategy_audit));
        }
        Ok(events)
    }
}

fn project_sync_task(task: &StoredBacktestSyncTask) -> Result<Value, SystemReadSnapshotError> {
    Ok(json!({
        "id": task.task_id,
        "kind": "backtest-sync",
        "status": task.status,
        "symbol": task.symbol,
        "provider": task.market_data_provider,
        "totalIntervals": task.total_intervals,
        "completedIntervals": task.completed_intervals,
        "totalBatches": task.total_batches,
        "completedBatches": task.completed_batches,
        "currentInterval": task.current_interval,
        "retries": task.retries,
        "error": task.error,
        "createdAt": task.started_at,
        "updatedAt": task.updated_at,
        "revision": task.revision,
    }))
}

fn project_backtest_run(
    run: &jftrade_store_sqlite::StoredBacktestRun,
) -> Result<Value, SystemReadSnapshotError> {
    Ok(json!({
        "id": run.id,
        "kind": "backtest-run",
        "status": run.status,
        "request": parse_json_field(&run.request_json, "backtest request")?,
        "result": parse_json_field(&run.result_json, "backtest result")?,
        "createdAt": run.created_at,
        "updatedAt": run.updated_at,
    }))
}

fn project_adk_task(task: &StoredAdkTask) -> Result<Value, SystemReadSnapshotError> {
    Ok(json!({
        "id": task.id,
        "kind": "adk-task",
        "status": task.status,
        "agentId": task.agent_id,
        "runId": task.run_id,
        "payload": parse_json_field(&task.payload_json, "ADK task payload")?,
        "createdAt": task.created_at,
        "updatedAt": task.updated_at,
    }))
}

fn project_adk_run(run: &StoredAdkRun) -> Result<Value, SystemReadSnapshotError> {
    Ok(json!({
        "id": run.id,
        "kind": "adk-run",
        "status": run.status,
        "sessionId": run.session_id,
        "agentId": run.agent_id,
        "clientRequestId": run.client_request_id,
        "requestFingerprint": run.request_fingerprint,
        "payload": parse_json_field(&run.payload_json, "ADK run payload")?,
        "createdAt": run.created_at,
        "updatedAt": run.updated_at,
    }))
}

fn project_execution_order(
    order: &StoredExecutionOrder,
) -> Result<Value, SystemReadSnapshotError> {
    // `normalizedRequest` is persisted JSON, so validate it even though the
    // compatibility projection keeps the original string untouched.
    let normalized_request = parse_json_field(&order.normalized_request, "normalized order")?;
    Ok(json!({
        "id": order.internal_order_id,
        "kind": "execution-order",
        "brokerId": order.broker_id,
        "accountId": order.account_id,
        "market": order.market,
        "symbol": order.symbol,
        "side": order.side,
        "orderType": order.order_type,
        "status": order.status,
        "requestedQuantity": order.requested_quantity,
        "requestedPrice": order.requested_price,
        "filledQuantity": order.filled_quantity,
        "filledAveragePrice": order.filled_average_price,
        "normalizedRequest": normalized_request,
        "lastError": order.last_error,
        "createdAt": order.created_at,
        "updatedAt": order.updated_at,
    }))
}

fn project_execution_event(
    event: &StoredExecutionOrderEventRecord,
) -> Result<Value, SystemReadSnapshotError> {
    Ok(json!({
        "id": event.id,
        "kind": "execution-command",
        "orderId": event.internal_order_id,
        "eventType": event.event_type,
        "previousStatus": event.previous_status,
        "nextStatus": event.next_status,
        "payload": parse_json_field(&event.payload_json, "execution event payload")?,
        "createdAt": event.created_at,
    }))
}

fn project_strategy_audit(event: &StoredStrategyAuditEvent) -> Value {
    json!({
        "id": format!("{}:{}:{}", event.instance_id, event.at_ms, event.kind),
        "kind": event.kind,
        "subjectId": event.instance_id,
        "detail": event.detail,
        "atMs": event.at_ms,
    })
}

fn parse_json_field(raw: &str, field: &str) -> Result<Value, SystemReadSnapshotError> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Ok(Value::Null);
    }
    serde_json::from_str(trimmed).map_err(|error| {
        storage_error(field, format_args!("invalid JSON: {error}"))
    })
}

fn storage_error(context: &str, error: impl Display) -> SystemReadSnapshotError {
    SystemReadSnapshotError::Unavailable(format!("storage overview {context} unavailable: {error}"))
}

fn is_pending_status(status: &str) -> bool {
    !matches!(
        status.trim().to_ascii_lowercase().as_str(),
        "completed" | "complete" | "failed" | "cancelled" | "canceled" | "rejected" | "done"
    )
}

fn sort_recent(values: &mut [Value]) {
    values.sort_by(|left, right| {
        recency_key(right)
            .cmp(&recency_key(left))
            .then_with(|| id_key(right).cmp(id_key(left)))
    });
}

fn recency_key(value: &Value) -> String {
    value
        .get("updatedAt")
        .or_else(|| value.get("createdAt"))
        .and_then(Value::as_str)
        .map(str::to_owned)
        .or_else(|| {
            value
                .get("atMs")
                .and_then(Value::as_i64)
                .map(|at_ms| format!("{at_ms:020}"))
        })
        .unwrap_or_default()
}

fn id_key(value: &Value) -> &str {
    value.get("id").and_then(Value::as_str).unwrap_or_default()
}

fn limited(mut values: Vec<Value>) -> Vec<Value> {
    values.truncate(STORAGE_PROJECTION_LIMIT);
    values
}
