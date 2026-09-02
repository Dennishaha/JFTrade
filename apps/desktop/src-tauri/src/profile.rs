use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const DEVELOPMENT_API_BIND: &str = "127.0.0.1:3008";
pub const RELEASE_API_BIND: &str = "127.0.0.1:6699";

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DesktopChannel {
    Dev,
    Release,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DesktopPlatform {
    Darwin,
    Linux,
    Windows,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DesktopProfile {
    pub channel: DesktopChannel,
    pub application_name: &'static str,
    pub product_identifier: &'static str,
    pub single_instance_id: &'static str,
    pub api_bind: &'static str,
    pub update_checks_enabled: bool,
    pub settings_path: String,
    pub backtest_db_path: String,
    pub window_state_path: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct PlatformPaths {
    pub platform: DesktopPlatform,
    pub home_dir: String,
    #[serde(default)]
    pub config_dir: String,
    #[serde(default)]
    pub local_app_data: String,
    #[serde(default)]
    pub xdg_data_home: String,
}

#[derive(Debug, Error, Eq, PartialEq)]
pub enum ProfileError {
    #[error("desktop home directory is required")]
    MissingHome,
    #[error("desktop data directory is unavailable for {0:?}")]
    MissingDataDirectory(DesktopPlatform),
}

impl DesktopProfile {
    pub fn resolve(channel: DesktopChannel, paths: &PlatformPaths) -> Result<Self, ProfileError> {
        match channel {
            DesktopChannel::Dev => Ok(Self {
                channel,
                application_name: "JFTrade Dev",
                product_identifier: "com.jftrade.desktop.dev",
                single_instance_id: "com.jftrade.desktop.dev",
                api_bind: DEVELOPMENT_API_BIND,
                update_checks_enabled: false,
                settings_path: "var/jftrade-api/settings.json".to_owned(),
                backtest_db_path: "var/jftrade-api/backtest.db".to_owned(),
                window_state_path: None,
            }),
            DesktopChannel::Release => {
                let root = product_data_dir(paths)?;
                Ok(Self {
                    channel,
                    application_name: "JFTrade",
                    product_identifier: "com.jftrade.desktop",
                    single_instance_id: "com.jftrade.desktop",
                    api_bind: RELEASE_API_BIND,
                    update_checks_enabled: true,
                    settings_path: join_path(&root, "settings.json"),
                    backtest_db_path: join_path(&root, "backtest.db"),
                    window_state_path: Some(join_path(&root, "desktop-state.json")),
                })
            }
        }
    }
}

pub fn product_data_dir(paths: &PlatformPaths) -> Result<String, ProfileError> {
    let home = paths.home_dir.trim();
    if home.is_empty() {
        return Err(ProfileError::MissingHome);
    }
    let root = match paths.platform {
        DesktopPlatform::Darwin => {
            let base = if paths.config_dir.trim().is_empty() {
                join_path(home, "Library/Application Support")
            } else {
                paths.config_dir.trim().to_owned()
            };
            join_path(&base, "JFTrade")
        }
        DesktopPlatform::Windows => {
            let base = if paths.local_app_data.trim().is_empty() {
                paths.config_dir.trim()
            } else {
                paths.local_app_data.trim()
            };
            if base.is_empty() {
                return Err(ProfileError::MissingDataDirectory(paths.platform));
            }
            join_path(base, "JFTrade")
        }
        DesktopPlatform::Linux => {
            let base = if paths.xdg_data_home.trim().is_empty() {
                join_path(home, ".local/share")
            } else {
                paths.xdg_data_home.trim().to_owned()
            };
            join_path(&base, "jftrade")
        }
    };
    Ok(root)
}

fn join_path(base: &str, child: &str) -> String {
    let base = base.trim_end_matches(['/', '\\']);
    let child = child.trim_start_matches(['/', '\\']);
    format!("{base}/{child}")
}
