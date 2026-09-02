//! Durable backtests test-cutover adapter backed by `jftrade-store-sqlite`.
//!
//! This module is compiled only for Rust tests. Its SQLite schema connects to
//! the real `backtest-runs` component with schema validation and single-writer lease.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use jftrade_store_sqlite::{
    BACKTEST_RUNS_TEST_CUTOVER_PROFILE, BacktestRunTestCutoverStore, StoredBacktestRun,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};

#[derive(Default, Serialize, Deserialize)]
struct BacktestCompanionState {
    next_sequence: u64,
    tasks: BTreeMap<String, String>,
    events: Vec<(String, String)>,
}

pub struct BacktestsSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<BacktestRunTestCutoverStore>,
    companion_path: PathBuf,
    state: Mutex<BacktestCompanionState>,
    reject_start: Mutex<bool>,
}

impl std::fmt::Debug for BacktestsSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestsSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl BacktestsSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store =
            BacktestRunTestCutoverStore::open_existing(&path, BACKTEST_RUNS_TEST_CUTOVER_PROFILE)
                .map_err(|err| err.to_string())?;
        let companion_path = path.with_extension("tasks.json");
        let state = if companion_path.exists() {
            let bytes = std::fs::read(&companion_path).map_err(|e| e.to_string())?;
            serde_json::from_slice(&bytes).unwrap_or_default()
        } else {
            BacktestCompanionState {
                next_sequence: 1,
                ..Default::default()
            }
        };
        Ok(Self {
            path,
            store: Arc::new(store),
            companion_path,
            state: Mutex::new(state),
            reject_start: Mutex::new(false),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &BacktestRunTestCutoverStore {
        &self.store
    }

    fn persist_companion(&self, state: &BacktestCompanionState) -> Result<(), String> {
        let bytes = serde_json::to_vec_pretty(state).map_err(|e| e.to_string())?;
        std::fs::write(&self.companion_path, bytes).map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn seed_run(&self, id: &str, status: &str) -> Result<(), String> {
        let timestamp = now_rfc3339();
        let run = StoredBacktestRun {
            id: id.to_owned(),
            status: status.to_owned(),
            request_json: "{}".to_owned(),
            result_json: "".to_owned(),
            created_at: timestamp.clone(),
            updated_at: timestamp.clone(),
        };
        self.store
            .save_run(run, &timestamp)
            .map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn seed_task(&self, id: &str, status: &str) -> Result<(), String> {
        let mut state = self.state.lock().map_err(|_| "poisoned".to_owned())?;
        state.tasks.insert(id.to_owned(), status.to_owned());
        self.persist_companion(&state)?;
        Ok(())
    }

    pub fn run_count(&self) -> Result<u64, String> {
        self.store.run_count().map_err(|e| e.to_string())
    }

    pub fn run_exists(&self, id: &str) -> Result<bool, String> {
        let run = self.store.get_run(id).map_err(|e| e.to_string())?;
        Ok(run.is_some())
    }

    pub fn task_status(&self, id: &str) -> Result<Option<String>, String> {
        let state = self.state.lock().map_err(|_| "poisoned".to_owned())?;
        Ok(state.tasks.get(id).cloned())
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        let state = self.state.lock().map_err(|_| "poisoned".to_owned())?;
        let count = state
            .events
            .iter()
            .filter(|(op, _)| op == operation)
            .count();
        Ok(count as u64)
    }

    pub fn reject_start_event(&self) -> Result<(), String> {
        let mut reject = self
            .reject_start
            .lock()
            .map_err(|_| "poisoned".to_owned())?;
        *reject = true;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let mut reject = self
            .reject_start
            .lock()
            .map_err(|_| "poisoned".to_owned())?;
        *reject = false;
        Ok(())
    }
}

impl BacktestsWritePort for BacktestsSqliteTestCutoverPort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { payload } => {
                let should_reject = *self
                    .reject_start
                    .lock()
                    .map_err(|_| failed("lock poisoned"))?;
                if should_reject {
                    return Err(failed("test-cutover start rejection"));
                }
                let mut state = self.state.lock().map_err(|_| failed("lock poisoned"))?;
                if state.next_sequence == 0 {
                    state.next_sequence = 1;
                }
                let seq = state.next_sequence;
                state.next_sequence += 1;
                let id = format!("run-test-{seq}");
                let timestamp = now_rfc3339();
                let run = StoredBacktestRun {
                    id: id.clone(),
                    status: "queued".to_owned(),
                    request_json: payload.to_string(),
                    result_json: "".to_owned(),
                    created_at: timestamp.clone(),
                    updated_at: timestamp.clone(),
                };
                self.store
                    .save_run(run, &timestamp)
                    .map_err(|e| failed(&e.to_string()))?;
                state.events.push(("start".to_owned(), id.clone()));
                self.persist_companion(&state)
                    .map_err(|e| failed(&e.to_string()))?;
                Ok(BacktestsWritePortResult::Data(json!({
                    "id": id,
                    "status": "queued",
                    "message": "backtest queued",
                })))
            }
            BacktestsWriteInput::Sync { payload: _ } => {
                let mut state = self.state.lock().map_err(|_| failed("lock poisoned"))?;
                if state.next_sequence == 0 {
                    state.next_sequence = 1;
                }
                let seq = state.next_sequence;
                state.next_sequence += 1;
                let task_id = format!("task-test-{seq}");
                state.tasks.insert(task_id.clone(), "running".to_owned());
                state.events.push(("sync".to_owned(), task_id.clone()));
                self.persist_companion(&state)
                    .map_err(|e| failed(&e.to_string()))?;
                Ok(BacktestsWritePortResult::Data(json!({
                    "taskId": task_id,
                    "status": "running",
                })))
            }
            BacktestsWriteInput::CancelSync { task_id } => {
                let mut state = self.state.lock().map_err(|_| failed("lock poisoned"))?;
                let cancelled = if let Some(status) = state.tasks.get_mut(task_id) {
                    if status == "running" {
                        *status = "cancelled".to_owned();
                        true
                    } else {
                        false
                    }
                } else {
                    false
                };
                if cancelled {
                    state
                        .events
                        .push(("cancel-sync".to_owned(), task_id.clone()));
                    self.persist_companion(&state)
                        .map_err(|e| failed(&e.to_string()))?;
                }
                Ok(BacktestsWritePortResult::SyncCancelled(cancelled))
            }
            BacktestsWriteInput::Delete { run_id } => {
                let run = self
                    .store
                    .get_run(run_id)
                    .map_err(|e| failed(&e.to_string()))?;
                let Some(existing) = run else {
                    return Ok(BacktestsWritePortResult::RunDeleted(
                        BacktestsWriteDeleteResult::Missing,
                    ));
                };
                if existing.status == "running" || existing.status == "queued" {
                    return Ok(BacktestsWritePortResult::RunDeleted(
                        BacktestsWriteDeleteResult::NotTerminal,
                    ));
                }
                self.store
                    .delete_run(run_id)
                    .map_err(|e| failed(&e.to_string()))?;
                let mut state = self.state.lock().map_err(|_| failed("lock poisoned"))?;
                state.events.push(("delete".to_owned(), run_id.clone()));
                self.persist_companion(&state)
                    .map_err(|e| failed(&e.to_string()))?;
                Ok(BacktestsWritePortResult::RunDeleted(
                    BacktestsWriteDeleteResult::Deleted,
                ))
            }
        }
    }
}

fn failed(message: &str) -> BacktestsWritePortError {
    BacktestsWritePortError::Failed(message.to_owned())
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "2026-08-26T00:00:00Z".to_owned())
}
