use std::fmt;
use std::sync::Arc;

use serde::{Deserialize, Serialize};

pub const DESKTOP_LOG_APPEND_EVENT: &str = "jftrade:desktop-log:append";
pub const DESKTOP_UPDATE_AVAILABLE_EVENT: &str = "jftrade:desktop-update:available";
pub const DESKTOP_SECOND_INSTANCE_EVENT: &str = "jftrade:desktop-second-instance";
pub const DESKTOP_MENU_SETTINGS_EVENT: &str = "jftrade:desktop-menu:settings";

pub const DESKTOP_COMMANDS: [&str; 10] = [
    "desktop_startup_snapshot",
    "desktop_startup_quit",
    "desktop_open_link",
    "desktop_log_list_days",
    "desktop_log_read_page",
    "desktop_log_open_folder",
    "desktop_update_check",
    "desktop_window_show_main",
    "desktop_window_hide_main",
    "desktop_window_open_logs",
];

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DesktopStartupSnapshot {
    pub state: String,
    pub phase: String,
    pub message: String,
    pub started_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DesktopLogDay {
    pub day: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DesktopLogLine {
    pub level: String,
    pub text: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DesktopLogPage {
    pub day: String,
    pub items: Vec<DesktopLogLine>,
    pub offset: i64,
    pub limit: usize,
    pub total: usize,
    pub log_dir: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DesktopUpdateResult {
    pub current_version: String,
    pub available: bool,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub latest_version: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub release_url: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub published_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub notes: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DesktopFailure {
    pub code: String,
    pub message: String,
}

impl DesktopFailure {
    pub fn new(code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            code: code.into(),
            message: message.into(),
        }
    }
}

impl fmt::Display for DesktopFailure {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(formatter, "{}: {}", self.code, self.message)
    }
}

impl std::error::Error for DesktopFailure {}

pub trait DesktopPort: Send + Sync {
    fn startup_snapshot(&self) -> Result<DesktopStartupSnapshot, DesktopFailure>;
    fn startup_quit(&self) -> Result<(), DesktopFailure>;
    fn open_link(&self, link: &str) -> Result<(), DesktopFailure>;
    fn log_list_days(&self) -> Result<Vec<DesktopLogDay>, DesktopFailure>;
    fn log_read_page(
        &self,
        day: &str,
        level: &str,
        query: &str,
        offset: i64,
        limit: usize,
    ) -> Result<DesktopLogPage, DesktopFailure>;
    fn log_open_folder(&self) -> Result<(), DesktopFailure>;
    fn update_check(&self) -> Result<DesktopUpdateResult, DesktopFailure>;
    fn window_show_main(&self) -> Result<(), DesktopFailure>;
    fn window_hide_main(&self) -> Result<(), DesktopFailure>;
    fn window_open_logs(&self) -> Result<(), DesktopFailure>;
}

#[derive(Clone)]
pub struct DesktopFacade {
    port: Arc<dyn DesktopPort>,
}

impl DesktopFacade {
    pub fn new(port: Arc<dyn DesktopPort>) -> Self {
        Self { port }
    }

    pub fn port(&self) -> &dyn DesktopPort {
        self.port.as_ref()
    }
}
