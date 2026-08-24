use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

pub const DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT: usize = 20;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InterfaceSettings {
    #[serde(default)]
    pub api_bind: String,
    #[serde(default)]
    pub gui_bind: String,
    #[serde(default, rename = "liveWebSocketConnectionLimit")]
    pub live_websocket_connection_limit: i64,
}

impl Default for InterfaceSettings {
    fn default() -> Self {
        Self {
            api_bind: String::new(),
            gui_bind: String::new(),
            live_websocket_connection_limit: DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT as i64,
        }
    }
}

pub trait InterfaceSettingsStorePort: Send + Sync {
    fn load_interface_settings(&self) -> Result<Option<InterfaceSettings>, SettingsStoreError>;
}

pub fn normalize_live_websocket_connection_limit(settings: Option<&InterfaceSettings>) -> usize {
    settings
        .map(|settings| settings.live_websocket_connection_limit)
        .filter(|limit| *limit > 0)
        .and_then(|limit| usize::try_from(limit).ok())
        .unwrap_or(DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn live_websocket_limit_matches_go_default_and_positive_value_rules() {
        assert_eq!(
            normalize_live_websocket_connection_limit(None),
            DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT
        );
        for limit in [0, -1] {
            assert_eq!(
                normalize_live_websocket_connection_limit(Some(&InterfaceSettings {
                    live_websocket_connection_limit: limit,
                    ..InterfaceSettings::default()
                })),
                DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT
            );
        }
        assert_eq!(
            normalize_live_websocket_connection_limit(Some(&InterfaceSettings {
                live_websocket_connection_limit: 7,
                ..InterfaceSettings::default()
            })),
            7
        );
    }
}
