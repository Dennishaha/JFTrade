use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const SHUTDOWN_TIMEOUT_MILLIS: u64 = 5_000;

#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum ProcessRole {
    Engine,
    PineWorker,
    MarketdataSidecar,
}

impl ProcessRole {
    pub const START_ORDER: [Self; 3] = [Self::Engine, Self::PineWorker, Self::MarketdataSidecar];

    pub const SHUTDOWN_ORDER: [Self; 3] = [Self::MarketdataSidecar, Self::PineWorker, Self::Engine];
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ReleaseAsset {
    pub role: ProcessRole,
    pub relative_path: String,
    pub sha256: String,
    pub executable: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimePlan {
    pub assets: Vec<ReleaseAsset>,
    pub start_order: Vec<ProcessRole>,
    pub shutdown_order: Vec<ProcessRole>,
    pub shutdown_timeout_millis: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LifecycleEvent {
    pub role: ProcessRole,
    pub action: LifecycleAction,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum LifecycleAction {
    Start,
    Ready,
    StartFailed,
    Shutdown,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LifecycleReport {
    pub ready: bool,
    pub failure_role: Option<ProcessRole>,
    pub events: Vec<LifecycleEvent>,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum LifecycleError {
    #[error("desktop runtime requires exactly one asset for every managed process")]
    IncompleteAssetSet,
    #[error("desktop asset path must be relative and traversal-free: {0}")]
    UnsafePath(String),
    #[error("desktop asset sha256 must contain exactly 64 lowercase hexadecimal characters")]
    InvalidDigest,
    #[error("desktop asset for {0:?} must be executable")]
    NotExecutable(ProcessRole),
    #[error("managed process {role:?} failed during {operation}: {message}")]
    Supervisor {
        role: ProcessRole,
        operation: &'static str,
        message: String,
    },
}

pub trait ProcessSupervisor {
    fn start(&mut self, asset: &ReleaseAsset) -> Result<(), String>;
    fn wait_ready(&mut self, role: ProcessRole) -> Result<(), String>;
    fn shutdown(&mut self, role: ProcessRole, timeout_millis: u64) -> Result<(), String>;
}

impl RuntimePlan {
    pub fn new(assets: Vec<ReleaseAsset>) -> Result<Self, LifecycleError> {
        let mut roles = BTreeSet::new();
        for asset in &assets {
            if !roles.insert(asset.role) {
                return Err(LifecycleError::IncompleteAssetSet);
            }
            validate_asset(asset)?;
        }
        if roles != ProcessRole::START_ORDER.into_iter().collect() {
            return Err(LifecycleError::IncompleteAssetSet);
        }
        let by_role: BTreeMap<_, _> = assets
            .into_iter()
            .map(|asset| (asset.role, asset))
            .collect();
        let assets = ProcessRole::START_ORDER
            .iter()
            .filter_map(|role| by_role.get(role).cloned())
            .collect();
        Ok(Self {
            assets,
            start_order: ProcessRole::START_ORDER.to_vec(),
            shutdown_order: ProcessRole::SHUTDOWN_ORDER.to_vec(),
            shutdown_timeout_millis: SHUTDOWN_TIMEOUT_MILLIS,
        })
    }

    pub fn start<S: ProcessSupervisor>(
        &self,
        supervisor: &mut S,
    ) -> Result<LifecycleReport, LifecycleError> {
        let mut started = Vec::new();
        let mut events = Vec::new();
        for asset in &self.assets {
            events.push(LifecycleEvent {
                role: asset.role,
                action: LifecycleAction::Start,
            });
            if let Err(_message) = supervisor.start(asset) {
                events.push(LifecycleEvent {
                    role: asset.role,
                    action: LifecycleAction::StartFailed,
                });
                shutdown_started(supervisor, &mut events, &started)?;
                return Ok(LifecycleReport {
                    ready: false,
                    failure_role: Some(asset.role),
                    events,
                });
            }
            started.push(asset.role);
            if let Err(message) = supervisor.wait_ready(asset.role) {
                events.push(LifecycleEvent {
                    role: asset.role,
                    action: LifecycleAction::StartFailed,
                });
                shutdown_started(supervisor, &mut events, &started)?;
                if message.is_empty() {
                    return Err(LifecycleError::Supervisor {
                        role: asset.role,
                        operation: "readiness",
                        message: "readiness failed without a reason".to_owned(),
                    });
                }
                return Ok(LifecycleReport {
                    ready: false,
                    failure_role: Some(asset.role),
                    events,
                });
            }
            events.push(LifecycleEvent {
                role: asset.role,
                action: LifecycleAction::Ready,
            });
        }
        Ok(LifecycleReport {
            ready: true,
            failure_role: None,
            events,
        })
    }

    pub fn shutdown<S: ProcessSupervisor>(
        &self,
        supervisor: &mut S,
    ) -> Result<Vec<LifecycleEvent>, LifecycleError> {
        let mut events = Vec::new();
        shutdown_started(supervisor, &mut events, &self.start_order)?;
        Ok(events)
    }
}

fn validate_asset(asset: &ReleaseAsset) -> Result<(), LifecycleError> {
    let path = asset.relative_path.trim();
    let unsafe_path = path.is_empty()
        || path.starts_with('/')
        || path.starts_with('\\')
        || path.contains(':')
        || path.split(['/', '\\']).any(|segment| segment == "..");
    if unsafe_path {
        return Err(LifecycleError::UnsafePath(asset.relative_path.clone()));
    }
    if asset.sha256.len() != 64
        || !asset
            .sha256
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
    {
        return Err(LifecycleError::InvalidDigest);
    }
    if !asset.executable {
        return Err(LifecycleError::NotExecutable(asset.role));
    }
    Ok(())
}

fn shutdown_started<S: ProcessSupervisor>(
    supervisor: &mut S,
    events: &mut Vec<LifecycleEvent>,
    started: &[ProcessRole],
) -> Result<(), LifecycleError> {
    for role in ProcessRole::SHUTDOWN_ORDER {
        if !started.contains(&role) {
            continue;
        }
        supervisor
            .shutdown(role, SHUTDOWN_TIMEOUT_MILLIS)
            .map_err(|message| LifecycleError::Supervisor {
                role,
                operation: "shutdown",
                message,
            })?;
        events.push(LifecycleEvent {
            role,
            action: LifecycleAction::Shutdown,
        });
    }
    Ok(())
}
