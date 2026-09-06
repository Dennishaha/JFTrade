//! Backtests, Execution Orders, Brokers, and ADK production ports.

use super::BacktestSyncWorkerRegistry;
use crate::product::product_backtest_execution::{
    BacktestExecutionPort, BacktestExecutionTaskRegistry,
};
use crate::product::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};
use crate::product::{
    BacktestReadSnapshotError, BacktestReadSnapshotPort, BacktestResultViewError,
    BacktestResultViewRequest, BacktestResultViewSnapshot, BacktestSyncReadSnapshotError,
    BacktestSyncReadSnapshotPort,
};
use jftrade_integration_marketdata_helper::HelperClient;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestRunStore, BacktestSyncTaskStore, StrategyDefinitionStore,
};
use serde_json::{Value, json};
use std::sync::Arc;

#[path = "product_backtest_sync_request.rs"]
mod product_backtest_sync_request;
#[path = "product_production_ports_backtest_parse.rs"]
mod product_production_ports_backtest_parse;
#[path = "product_production_ports_backtest_sync_projection.rs"]
mod product_production_ports_backtest_sync_projection;
use product_production_ports_backtest_sync_projection::sync_task_projection;
#[path = "product_production_ports_execution_orders.rs"]
mod product_production_ports_execution_orders;
pub(crate) use product_production_ports_execution_orders::{
    ExecutionReconciliationWorker, ProductionExecutionPort,
};

#[path = "product_production_ports_backtest_strategy.rs"]
mod product_production_ports_backtest_strategy;
#[path = "product_production_ports_backtest_sync.rs"]
mod product_production_ports_backtest_sync;
#[path = "product_production_ports_backtest_task.rs"]
mod product_production_ports_backtest_task;
pub(crate) use crate::product::product_backtest_provider::BacktestMarketDataProviderState;

pub(crate) struct ProductionBacktestPort {
    pub(crate) store: Arc<BacktestRunStore>,
    pub(crate) sync_tasks: Arc<BacktestSyncTaskStore>,
    pub(crate) _market_data_store: Arc<BacktestMarketDataStore>,
    pub(crate) helper: Option<HelperClient>,
    /// Shared runtime lookup for the live OpenD historical reader. The
    /// runtime can replace this reader during provider activation, so the
    /// sync worker resolves it per request instead of freezing startup state.
    pub(crate) trade_runtime: Option<Arc<super::SharedTradeReadRuntime>>,
    pub(crate) backtest_market_data_provider_state: Arc<BacktestMarketDataProviderState>,
    pub(crate) sync_workers: Arc<BacktestSyncWorkerRegistry>,
    pub(crate) execution: Option<Arc<dyn BacktestExecutionPort>>,
    /// Shared readiness for the Pine worker that backs production execution.
    /// The execution adapter also checks this state at task time; keeping it
    /// on the write port lets BacktestStart fail closed before persisting or
    /// queueing a run when the worker has gone unhealthy.
    pub(crate) pine_readiness: Option<Arc<jftrade_integration_pine::PineReadinessState>>,
    pub(crate) execution_workers: Arc<BacktestExecutionTaskRegistry>,
    pub(crate) strategy_definitions: Arc<StrategyDefinitionStore>,
}

impl std::fmt::Debug for ProductionBacktestPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionBacktestPort")
            .field("has_helper", &self.helper.is_some())
            .field("has_execution", &self.execution.is_some())
            .finish_non_exhaustive()
    }
}

impl BacktestReadSnapshotPort for ProductionBacktestPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        self.execution_workers.reap_finished();
        self.recover_orphaned_runs()?;
        let runs = self
            .store
            .list_runs()
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        let items = runs
            .into_iter()
            .map(|r| {
                let (request, request_provider) = decode_request_metadata(&r.request_json)?;
                // Go's ListLightweight validates the persisted result while
                // omitting the potentially large result payload from the
                // response.  Empty result_json is the normal representation
                // for queued/running runs and must not turn the whole list
                // into a store failure.
                let result = decode_optional_json_field(&r.result_json, "backtest result")?;
                let market_data_provider = result
                    .as_ref()
                    .and_then(|value| value.get("marketDataProvider"))
                    .and_then(Value::as_str)
                    .or(request_provider.as_deref())
                    .unwrap_or_default();
                Ok(json!({
                    "id": r.id,
                    "status": r.status,
                    "request": request,
                    "createdAt": r.created_at,
                    "updatedAt": r.updated_at,
                    "marketDataProvider": market_data_provider,
                }))
            })
            .collect::<Result<Vec<_>, BacktestReadSnapshotError>>()?;
        if items.is_empty() {
            Ok(json!({ "runs": Value::Null }))
        } else {
            Ok(json!({ "runs": items }))
        }
    }

    fn status(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        self.execution_workers.reap_finished();
        self.recover_orphaned_runs()?;
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        Ok(run.map(|r| {
            json!({
                "id": r.id,
                "status": r.status,
                "createdAt": r.created_at,
                "updatedAt": r.updated_at,
            })
        }))
    }

    fn result(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        self.execution_workers.reap_finished();
        self.recover_orphaned_runs()?;
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        run.map(|r| {
            let (request, request_provider) = decode_request_metadata(&r.request_json)?;
            let result = decode_optional_json_field(&r.result_json, "backtest result")?;
            let market_data_provider = result
                .as_ref()
                .and_then(|value| value.get("marketDataProvider"))
                .and_then(Value::as_str)
                .or(request_provider.as_deref())
                .unwrap_or_default();
            let mut response = json!({
                "id": r.id,
                "status": r.status,
                "request": request,
                "createdAt": r.created_at,
                "updatedAt": r.updated_at,
                "marketDataProvider": market_data_provider,
            });
            if let Some(result) = result {
                response["result"] = result;
            }
            Ok(response)
        })
        .transpose()
    }

    fn result_view(
        &self,
        request: &BacktestResultViewRequest,
    ) -> Result<Option<BacktestResultViewSnapshot>, BacktestResultViewError> {
        self.execution_workers.reap_finished();
        self.recover_orphaned_runs()
            .map_err(|e| BacktestResultViewError::Unavailable(e.to_string()))?;
        let run = self
            .store
            .get_run(&request.run_id)
            .map_err(|e| BacktestResultViewError::Unavailable(e.to_string()))?;
        let Some(r) = run else {
            return Ok(None);
        };
        let (req_json, request_provider) = decode_request_metadata(&r.request_json)
            .map_err(|e| BacktestResultViewError::Unavailable(e.to_string()))?;
        let result_json = decode_optional_json_field(&r.result_json, "backtest result")
            .map_err(|e| BacktestResultViewError::Unavailable(e.to_string()))?;
        let market_data_provider = result_json
            .as_ref()
            .and_then(|v| v.get("marketDataProvider"))
            .and_then(Value::as_str)
            .or(request_provider.as_deref())
            .unwrap_or("futu");

        let raw_req: Value = serde_json::from_str(&r.request_json).unwrap_or(Value::Null);
        let seed_candles = raw_req
            .get("__resultViewSeed")
            .and_then(|s| s.get("formal_candles"))
            .and_then(Value::as_array)
            .cloned();

        let (candles, candle_missing) = if let Some(c) = seed_candles {
            (Some(c), false)
        } else {
            match self.reconstruct_candles_from_store(&raw_req, market_data_provider) {
                Some(c) => (Some(c), false),
                None => (None, true),
            }
        };

        let view = request.view.as_deref().unwrap_or("summary");
        let requests_candles = view == "chart"
            && request
                .include
                .as_ref()
                .map(|inc| inc.iter().any(|s| s == "candles"))
                .unwrap_or(true);

        let has_result_candles = result_json
            .as_ref()
            .and_then(|r| r.get("candles").or_else(|| r.get("result").and_then(|res| res.get("candles"))))
            .and_then(Value::as_array)
            .is_some_and(|arr| !arr.is_empty());

        if candle_missing && requests_candles && !has_result_candles {
            return Err(BacktestResultViewError::Unavailable(
                "backtest candle seed is missing and candle data could not be reconstructed from store".to_owned(),
            ));
        }

        let mut run_val = json!({
            "id": r.id,
            "status": r.status,
            "request": req_json,
            "createdAt": r.created_at,
            "updatedAt": r.updated_at,
            "marketDataProvider": market_data_provider,
        });
        if let Some(res) = result_json {
            run_val["result"] = res;
        }

        let view_val = crate::product::product_research_backtest_projection::project_authoritative_result_view(
            &run_val,
            candles.as_deref(),
            request,
        )?;

        Ok(Some(BacktestResultViewSnapshot { data: view_val }))
    }
}

fn decode_json_field(raw: &str, field: &str) -> Result<Value, BacktestReadSnapshotError> {
    serde_json::from_str(raw).map_err(|error| {
        BacktestReadSnapshotError::Unavailable(format!("stored {field} is invalid JSON: {error}"))
    })
}

/// The provider is private run metadata.  It is persisted beside the request
/// so queued/running rows retain their frozen source across a restart, while
/// the public request projection remains byte-for-byte compatible with Go.
pub(crate) fn decode_request_metadata(
    raw: &str,
) -> Result<(Value, Option<String>), BacktestReadSnapshotError> {
    let mut request = decode_json_field(raw, "backtest request")?;
    let provider = request.as_object_mut().and_then(|object| {
        [
            "__marketDataProvider",
            "marketDataProvider",
            "marketDataProviderOverride",
        ]
        .into_iter()
        .find_map(|field| object.remove(field))
        .and_then(|value| value.as_str().map(str::to_owned))
    });
    // Provider selection is an application/runtime concern and is excluded
    // from Go's public StartRequest JSON. Remove all accepted aliases from
    // the projection even when an older row persisted one directly.
    if let Some(object) = request.as_object_mut() {
        object.remove("__marketDataProvider");
        object.remove("marketDataProvider");
        object.remove("marketDataProviderOverride");
        object.remove("__resultViewSeed");
    }
    Ok((request, provider))
}

pub(crate) fn persist_request_with_provider(payload: &Value, provider: &str) -> String {
    let mut request = payload.clone();
    if let Some(object) = request.as_object_mut() {
        object.insert(
            "__marketDataProvider".to_owned(),
            Value::String(provider.to_owned()),
        );
    }
    request.to_string()
}

pub(crate) fn requested_provider(
    payload: &Value,
) -> Result<Option<&'static str>, BacktestsWritePortError> {
    let Some(value) = ["marketDataProvider", "marketDataProviderOverride"]
        .iter()
        .find_map(|field| payload.get(*field))
    else {
        return Ok(None);
    };
    let text = value.as_str().ok_or_else(|| {
        BacktestsWritePortError::BadRequest("marketDataProvider must be a string".to_owned())
    })?;
    let normalized = text.trim().to_ascii_lowercase();
    if normalized.is_empty() {
        return Ok(None);
    }
    match normalized.as_str() {
        "futu" => Ok(Some("futu")),
        "yfinance" => Ok(Some("yfinance")),
        "akshare" => Ok(Some("akshare")),
        _ => Err(BacktestsWritePortError::BadRequest(format!(
            "unsupported marketDataProvider {text:?}"
        ))),
    }
}

impl ProductionBacktestPort {
    pub(crate) fn recover_orphaned_runs(&self) -> Result<(), BacktestReadSnapshotError> {
        let runs = self.store.list_runs().map_err(|error| {
            BacktestReadSnapshotError::Unavailable(format!(
                "failed to scan backtest runs for recovery: {error}"
            ))
        })?;
        for run in runs {
            if !matches!(run.status.as_str(), "queued" | "running")
                || self.execution_workers.has_worker(&run.id)
            {
                continue;
            }
            let timestamp = crate::product::product_backtest_execution::now_timestamp();
            let failed = jftrade_store_sqlite::StoredBacktestRun {
                status: "failed".to_owned(),
                // A restart has no execution result. Preserve the explicit
                // terminal state without fabricating a result payload.
                result_json: String::new(),
                updated_at: timestamp.clone(),
                ..run.clone()
            };
            self.store
                .update_run_if_status(&run.id, &run.status, failed, &timestamp)
                .map_err(|error| {
                    BacktestReadSnapshotError::Unavailable(format!(
                        "backtest {} restart recovery failed: {error}",
                        run.id
                    ))
                })?;
        }
        Ok(())
    }

    fn reconstruct_candles_from_store(
        &self,
        raw_req: &Value,
        provider: &str,
    ) -> Option<Vec<Value>> {
        let symbol = raw_req.get("symbol")?.as_str()?;
        let interval = raw_req
            .get("interval")
            .or_else(|| raw_req.get("period"))?
            .as_str()?;
        let rehab = raw_req
            .get("rehabType")
            .and_then(Value::as_str)
            .unwrap_or("forward");
        let session = raw_req
            .get("sessionScope")
            .and_then(Value::as_str)
            .unwrap_or("regular");
        let start_ms = raw_req
            .get("startTimeMs")
            .and_then(Value::as_i64)
            .or_else(|| {
                raw_req
                    .get("startTime")
                    .or_else(|| raw_req.get("startDate"))
                    .and_then(Value::as_str)
                    .and_then(|s| {
                        product_production_ports_backtest_parse::parse_start_timestamp(s).ok()
                    })
            })?;
        let end_ms = raw_req
            .get("endTimeMs")
            .and_then(Value::as_i64)
            .or_else(|| {
                raw_req
                    .get("endTime")
                    .or_else(|| raw_req.get("endDate"))
                    .and_then(Value::as_str)
                    .and_then(|s| {
                        product_production_ports_backtest_parse::parse_end_timestamp(s).ok()
                    })
            })?;
        let stored = self
            ._market_data_store
            .read_candles(
                provider, symbol, interval, rehab, session, start_ms, end_ms,
            )
            .map_err(|e| {
                tracing::warn!(error = ?e, "failed to read stored candles for result view");
                e
            })
            .ok()?;
        if stored.is_empty() {
            return None;
        }
        Some(
            stored
                .into_iter()
                .map(|c| {
                    let rfc = time::OffsetDateTime::from_unix_timestamp_nanos(
                        (c.end_time as i128) * 1_000_000,
                    )
                    .ok()
                    .and_then(|dt| {
                        dt.format(&time::format_description::well_known::Rfc3339).ok()
                    });
                    json!({
                        "time": rfc.unwrap_or_else(|| c.end_time.to_string()),
                        "open": c.open,
                        "high": c.high,
                        "low": c.low,
                        "close": c.close,
                        "volume": c.volume,
                    })
                })
                .collect(),
        )
    }
}

fn decode_optional_json_field(
    raw: &str,
    field: &str,
) -> Result<Option<Value>, BacktestReadSnapshotError> {
    let trimmed = raw.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Ok(None);
    }
    decode_json_field(trimmed, field).map(Some)
}

impl BacktestSyncReadSnapshotPort for ProductionBacktestPort {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        self.sync_workers.reap_finished();
        self.sync_tasks
            .get(task_id)
            .map_err(|error| BacktestSyncReadSnapshotError::Unavailable(error.to_string()))?
            .map(|task| sync_task_projection(&task))
            .transpose()
    }

    fn active_tasks(&self) -> Result<Vec<Value>, BacktestSyncReadSnapshotError> {
        self.sync_workers.reap_finished();
        let tasks = self
            .sync_tasks
            .list_active()
            .map_err(|error| BacktestSyncReadSnapshotError::Unavailable(error.to_string()))?;
        tasks.iter().map(sync_task_projection).collect()
    }

    fn check_coverage(
        &self,
        request: &crate::product::BacktestDataCoverageRequest,
    ) -> Result<bool, BacktestSyncReadSnapshotError> {
        let formal = self
            ._market_data_store
            .read_candles(
                &request.provider,
                &request.symbol,
                &request.interval,
                &request.rehab_type,
                &request.session_scope,
                request.start_time_ms,
                request.end_time_ms,
            )
            .map_err(|e| BacktestSyncReadSnapshotError::Unavailable(e.to_string()))?;
        if formal.is_empty() {
            return Ok(false);
        }

        if request.warmup_bars > 0 {
            let interval_ms: i64 = match request.interval.trim().to_ascii_lowercase().as_str() {
                "1m" | "1min" => 60_000,
                "5m" | "5min" => 300_000,
                "15m" | "15min" => 900_000,
                "30m" | "30min" => 1_800_000,
                "60m" | "60min" | "1h" => 3_600_000,
                "1d" | "d" => 86_400_000,
                "1w" | "w" | "week" => 7 * 86_400_000,
                "1mo" | "1mon" | "1month" | "mo" | "month" => 30 * 86_400_000,
                _ => 60_000,
            };
            let mut found = false;
            for &m in &[3, 7, 14, 30, 60] {
                let bounded_start = request.start_time_ms.saturating_sub(
                    interval_ms.saturating_mul((request.warmup_bars as i64).saturating_mul(m)),
                );
                if self
                    ._market_data_store
                    .query_candles(
                        &request.provider,
                        &request.symbol,
                        &request.interval,
                        &request.rehab_type,
                        &request.session_scope,
                        bounded_start,
                        request.start_time_ms,
                        "DESC",
                        request.warmup_bars,
                    )
                    .map(|w| w.len() >= request.warmup_bars)
                    .unwrap_or(false)
                {
                    found = true;
                    break;
                }
            }
            if !found {
                return Ok(false);
            }
        }
        Ok(true)
    }
}

impl BacktestsWritePort for ProductionBacktestPort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { payload } => self.start_backtest(payload),
            BacktestsWriteInput::Sync { payload } => self.start_sync_task(payload),
            BacktestsWriteInput::CancelSync { task_id } => self.cancel_sync_task(task_id),
            BacktestsWriteInput::Delete { run_id } => match self.store.delete_run(run_id) {
                Ok(true) => Ok(BacktestsWritePortResult::RunDeleted(
                    BacktestsWriteDeleteResult::Deleted,
                )),
                Ok(false) => Ok(BacktestsWritePortResult::RunDeleted(
                    BacktestsWriteDeleteResult::Missing,
                )),
                Err(jftrade_store_sqlite::BacktestRunStoreError::NotTerminal(_)) => Ok(
                    BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::NotTerminal),
                ),
                Err(e) => Err(BacktestsWritePortError::Failed(e.to_string())),
            },
        }
    }
}

#[cfg(test)]
#[path = "product_backtest_sync_start_tests.rs"]
mod sync_start_tests;
