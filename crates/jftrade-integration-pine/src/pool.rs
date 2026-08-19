use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkerHealth {
    pub ok: bool,
    pub version: String,
    pub pine_ts_version: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkerSnapshot {
    pub worker_id: String,
    pub address: String,
    pub healthy: bool,
    pub busy: bool,
    pub restarts: u32,
    pub last_error: Option<String>,
    pub version: String,
    pub pine_ts_version: String,
    pub capabilities: Vec<String>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SessionOperation {
    None,
    Open,
    Append,
    Close,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WorkerReservation {
    pub worker_id: String,
    pub session_id: Option<String>,
    operation: SessionOperation,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum PoolError {
    #[error("pine worker pool requires at least one worker")]
    Empty,
    #[error("pine worker capacity exceeded")]
    CapacityExceeded,
    #[error("pine worker live session id is required")]
    MissingSession,
    #[error("pine worker live session {0} is already open")]
    SessionAlreadyOpen(String),
    #[error("pine worker live session {0} is not pinned to an active worker")]
    SessionNotFound(String),
    #[error("pine worker {0} is not registered")]
    WorkerNotFound(String),
}

#[derive(Clone, Debug)]
struct WorkerSlot {
    snapshot: WorkerSnapshot,
}

#[derive(Clone, Debug)]
pub struct WorkerPool {
    workers: Vec<WorkerSlot>,
    sessions: BTreeMap<String, String>,
    next: usize,
}

impl WorkerPool {
    pub fn new(workers: impl IntoIterator<Item = (String, String)>) -> Result<Self, PoolError> {
        let workers = workers
            .into_iter()
            .map(|(worker_id, address)| WorkerSlot {
                snapshot: WorkerSnapshot {
                    worker_id,
                    address,
                    healthy: false,
                    busy: false,
                    restarts: 0,
                    last_error: None,
                    version: String::new(),
                    pine_ts_version: String::new(),
                    capabilities: Vec::new(),
                },
            })
            .collect::<Vec<_>>();
        if workers.is_empty() {
            return Err(PoolError::Empty);
        }
        Ok(Self {
            workers,
            sessions: BTreeMap::new(),
            next: 0,
        })
    }

    pub fn record_health(
        &mut self,
        worker_id: &str,
        health: Result<WorkerHealth, String>,
    ) -> Result<(), PoolError> {
        let slot = self
            .workers
            .iter_mut()
            .find(|slot| slot.snapshot.worker_id == worker_id)
            .ok_or_else(|| PoolError::WorkerNotFound(worker_id.to_owned()))?;
        match health {
            Ok(health) if health.ok => {
                slot.snapshot.healthy = true;
                slot.snapshot.last_error = None;
                slot.snapshot.version = health.version;
                slot.snapshot.pine_ts_version = health.pine_ts_version;
                slot.snapshot.capabilities = health.capabilities;
            }
            Ok(_) => {
                slot.snapshot.healthy = false;
                slot.snapshot.last_error = Some("worker health returned ok=false".to_owned());
            }
            Err(error) => {
                slot.snapshot.healthy = false;
                slot.snapshot.last_error = Some(error);
            }
        }
        Ok(())
    }

    pub fn reserve(
        &mut self,
        operation: SessionOperation,
        session_id: Option<&str>,
    ) -> Result<WorkerReservation, PoolError> {
        let session_id = session_id.map(str::trim).filter(|value| !value.is_empty());
        let pinned = match operation {
            SessionOperation::None => None,
            SessionOperation::Open => {
                let id = session_id.ok_or(PoolError::MissingSession)?;
                if self.sessions.contains_key(id) {
                    return Err(PoolError::SessionAlreadyOpen(id.to_owned()));
                }
                None
            }
            SessionOperation::Append | SessionOperation::Close => {
                let id = session_id.ok_or(PoolError::MissingSession)?;
                Some(
                    self.sessions
                        .get(id)
                        .cloned()
                        .ok_or_else(|| PoolError::SessionNotFound(id.to_owned()))?,
                )
            }
        };
        let worker_index = match pinned {
            Some(worker_id) => self
                .workers
                .iter()
                .position(|slot| slot.snapshot.worker_id == worker_id && !slot.snapshot.busy)
                .ok_or(PoolError::CapacityExceeded)?,
            None => self.pick_available().ok_or(PoolError::CapacityExceeded)?,
        };
        let worker_id = self.workers[worker_index].snapshot.worker_id.clone();
        self.workers[worker_index].snapshot.busy = true;
        if operation == SessionOperation::Open {
            self.sessions.insert(
                session_id.expect("validated session").to_owned(),
                worker_id.clone(),
            );
        }
        Ok(WorkerReservation {
            worker_id,
            session_id: session_id.map(str::to_owned),
            operation,
        })
    }

    pub fn release(
        &mut self,
        reservation: WorkerReservation,
        succeeded: bool,
    ) -> Result<(), PoolError> {
        let slot = self
            .workers
            .iter_mut()
            .find(|slot| slot.snapshot.worker_id == reservation.worker_id)
            .ok_or_else(|| PoolError::WorkerNotFound(reservation.worker_id.clone()))?;
        slot.snapshot.busy = false;
        if let Some(session_id) = reservation.session_id
            && (reservation.operation == SessionOperation::Close
                || (reservation.operation == SessionOperation::Open && !succeeded))
        {
            self.sessions.remove(&session_id);
        }
        Ok(())
    }

    pub fn record_restart(&mut self, worker_id: &str) -> Result<(), PoolError> {
        let slot = self
            .workers
            .iter_mut()
            .find(|slot| slot.snapshot.worker_id == worker_id)
            .ok_or_else(|| PoolError::WorkerNotFound(worker_id.to_owned()))?;
        slot.snapshot.restarts = slot.snapshot.restarts.saturating_add(1);
        slot.snapshot.healthy = false;
        slot.snapshot.busy = false;
        self.sessions.retain(|_, pinned| pinned != worker_id);
        Ok(())
    }

    pub fn snapshot(&self) -> Vec<WorkerSnapshot> {
        self.workers
            .iter()
            .map(|slot| slot.snapshot.clone())
            .collect()
    }

    fn pick_available(&mut self) -> Option<usize> {
        for offset in 0..self.workers.len() {
            let index = (self.next + offset) % self.workers.len();
            let worker = &self.workers[index].snapshot;
            if worker.healthy && !worker.busy {
                self.next = (index + 1) % self.workers.len();
                return Some(index);
            }
        }
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn healthy() -> WorkerHealth {
        WorkerHealth {
            ok: true,
            version: "1".to_owned(),
            pine_ts_version: "fixture".to_owned(),
            capabilities: vec!["backtest".to_owned(), "live_incremental".to_owned()],
        }
    }

    #[test]
    fn live_sessions_are_pinned_and_open_failure_rolls_back() {
        let mut pool = WorkerPool::new([
            ("pineworker-1".to_owned(), "127.0.0.1:1".to_owned()),
            ("pineworker-2".to_owned(), "127.0.0.1:2".to_owned()),
        ])
        .expect("pool");
        for id in ["pineworker-1", "pineworker-2"] {
            pool.record_health(id, Ok(healthy())).expect("health");
        }
        let open = pool
            .reserve(SessionOperation::Open, Some("session-a"))
            .expect("open");
        let worker = open.worker_id.clone();
        pool.release(open, true).expect("release");
        let append = pool
            .reserve(SessionOperation::Append, Some("session-a"))
            .expect("append");
        assert_eq!(append.worker_id, worker);
        pool.release(append, true).expect("release");

        let failed = pool
            .reserve(SessionOperation::Open, Some("session-b"))
            .expect("open");
        pool.release(failed, false).expect("rollback");
        assert!(matches!(
            pool.reserve(SessionOperation::Append, Some("session-b")),
            Err(PoolError::SessionNotFound(_))
        ));
    }

    #[test]
    fn restart_drops_pinned_sessions_and_requires_new_health() {
        let mut pool =
            WorkerPool::new([("pineworker-1".to_owned(), "127.0.0.1:1".to_owned())]).expect("pool");
        pool.record_health("pineworker-1", Ok(healthy()))
            .expect("health");
        let open = pool
            .reserve(SessionOperation::Open, Some("session"))
            .expect("open");
        pool.release(open, true).expect("release");
        pool.record_restart("pineworker-1").expect("restart");
        assert!(matches!(
            pool.reserve(SessionOperation::Append, Some("session")),
            Err(PoolError::SessionNotFound(_))
        ));
        assert_eq!(pool.snapshot()[0].restarts, 1);
    }
}
