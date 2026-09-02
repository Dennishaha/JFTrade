#![forbid(unsafe_code)]

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub mod contract;
pub mod lifecycle;
pub mod links;
pub mod native;
pub mod profile;
mod resource_integrity;
pub mod tauri_adapter;
mod window_state;

use contract::{
    DESKTOP_COMMANDS, DESKTOP_LOG_APPEND_EVENT, DESKTOP_MENU_SETTINGS_EVENT,
    DESKTOP_SECOND_INSTANCE_EVENT, DESKTOP_UPDATE_AVAILABLE_EVENT,
};
use lifecycle::{
    LifecycleError, LifecycleReport, ProcessRole, ProcessSupervisor, ReleaseAsset, RuntimePlan,
};
use links::{LinkTarget, classify_link};
use profile::{DesktopChannel, DesktopProfile, PlatformPaths, ProfileError};

pub const STAGE8_CONTRACT_VERSION: &str = "stage8.v1";
pub const TAURI_CRATE_VERSION: &str = "2.11.5";
pub const TAURI_UPDATER_CRATE_VERSION: &str = "2.10.1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Stage8Input {
    pub version: String,
    pub platforms: Vec<PlatformPaths>,
    pub links: Vec<String>,
    pub assets: Vec<ReleaseAsset>,
    pub failure_role: ProcessRole,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Stage8Output {
    pub version: String,
    pub tauri_version: &'static str,
    pub engine_contract: EngineContract,
    pub profiles: Vec<ProfileProjection>,
    pub links: Vec<LinkProjection>,
    pub runtime_plan: RuntimePlan,
    pub successful_start: LifecycleReport,
    pub successful_shutdown: Vec<lifecycle::LifecycleEvent>,
    pub failed_start: LifecycleReport,
    pub commands: Vec<&'static str>,
    pub events: Vec<&'static str>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EngineContract {
    pub protocol_version: &'static str,
    pub health_service: &'static str,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProfileProjection {
    pub platform: profile::DesktopPlatform,
    pub development: DesktopProfile,
    pub release: DesktopProfile,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LinkProjection {
    pub input: String,
    pub accepted: bool,
    pub target: Option<LinkTarget>,
    pub error: Option<String>,
}

#[derive(Debug, Error)]
pub enum Stage8Error {
    #[error("unsupported Stage 8 contract version {0:?}")]
    UnsupportedVersion(String),
    #[error(transparent)]
    Profile(#[from] ProfileError),
    #[error(transparent)]
    Lifecycle(#[from] LifecycleError),
}

pub fn evaluate_stage8(input: Stage8Input) -> Result<Stage8Output, Stage8Error> {
    if input.version != STAGE8_CONTRACT_VERSION {
        return Err(Stage8Error::UnsupportedVersion(input.version));
    }
    let profiles = input
        .platforms
        .iter()
        .map(|paths| {
            Ok(ProfileProjection {
                platform: paths.platform,
                development: DesktopProfile::resolve(DesktopChannel::Dev, paths)?,
                release: DesktopProfile::resolve(DesktopChannel::Release, paths)?,
            })
        })
        .collect::<Result<Vec<_>, ProfileError>>()?;
    let links = input
        .links
        .into_iter()
        .map(|link| match classify_link(&link) {
            Ok(target) => LinkProjection {
                input: link,
                accepted: true,
                target: Some(target),
                error: None,
            },
            Err(error) => LinkProjection {
                input: link,
                accepted: false,
                target: None,
                error: Some(error.to_string()),
            },
        })
        .collect();
    let runtime_plan = RuntimePlan::new(input.assets)?;
    let mut successful = RecordingSupervisor::default();
    let successful_start = runtime_plan.start(&mut successful)?;
    let successful_shutdown = runtime_plan.shutdown(&mut successful)?;
    let mut failing = RecordingSupervisor {
        fail_ready: Some(input.failure_role),
    };
    let failed_start = runtime_plan.start(&mut failing)?;
    Ok(Stage8Output {
        version: STAGE8_CONTRACT_VERSION.to_owned(),
        tauri_version: TAURI_CRATE_VERSION,
        engine_contract: EngineContract {
            protocol_version: jftrade_engine::PROTOCOL_VERSION,
            health_service: jftrade_engine::HEALTH_SERVICE_NAME,
        },
        profiles,
        links,
        runtime_plan,
        successful_start,
        successful_shutdown,
        failed_start,
        commands: DESKTOP_COMMANDS.to_vec(),
        events: vec![
            DESKTOP_LOG_APPEND_EVENT,
            DESKTOP_UPDATE_AVAILABLE_EVENT,
            DESKTOP_SECOND_INSTANCE_EVENT,
            DESKTOP_MENU_SETTINGS_EVENT,
        ],
    })
}

#[derive(Default)]
struct RecordingSupervisor {
    fail_ready: Option<ProcessRole>,
}

impl ProcessSupervisor for RecordingSupervisor {
    fn start(&mut self, _asset: &ReleaseAsset) -> Result<(), String> {
        Ok(())
    }

    fn wait_ready(&mut self, role: ProcessRole) -> Result<(), String> {
        if self.fail_ready == Some(role) {
            return Err("injected readiness failure".to_owned());
        }
        Ok(())
    }

    fn shutdown(&mut self, _role: ProcessRole, _timeout_millis: u64) -> Result<(), String> {
        Ok(())
    }
}
