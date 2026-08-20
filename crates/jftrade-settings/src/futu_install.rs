use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct FutuOpenDInstallSettings {
    pub host: String,
    pub api_port: i32,
    pub websocket_port: i32,
    #[serde(rename = "maxWebSocketConnections")]
    pub max_websocket_connections: i32,
    pub use_encryption: bool,
    pub websocket_key_required: bool,
}

impl Default for FutuOpenDInstallSettings {
    fn default() -> Self {
        Self {
            host: "127.0.0.1".to_owned(),
            api_port: 11_110,
            websocket_port: 11_111,
            max_websocket_connections: 20,
            use_encryption: false,
            websocket_key_required: false,
        }
    }
}

pub trait FutuOpenDInstallSettingsStorePort: Send + Sync {
    fn load_futu_open_d_install_settings(
        &self,
    ) -> Result<Option<FutuOpenDInstallSettings>, SettingsStoreError>;
}

#[derive(Clone)]
pub struct FutuOpenDInstallSettingsService {
    store: Arc<dyn FutuOpenDInstallSettingsStorePort>,
}

impl FutuOpenDInstallSettingsService {
    pub fn new(store: Arc<dyn FutuOpenDInstallSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<FutuOpenDInstallSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_futu_open_d_install_settings()?
            .unwrap_or_default())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    struct Store(RwLock<Option<FutuOpenDInstallSettings>>);

    impl FutuOpenDInstallSettingsStorePort for Store {
        fn load_futu_open_d_install_settings(
            &self,
        ) -> Result<Option<FutuOpenDInstallSettings>, SettingsStoreError> {
            self.0
                .read()
                .map(|settings| settings.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    #[test]
    fn missing_persisted_integration_uses_current_go_defaults() {
        let service = FutuOpenDInstallSettingsService::new(Arc::new(Store(RwLock::new(None))));
        assert_eq!(
            service.settings().expect("install settings"),
            FutuOpenDInstallSettings::default()
        );
    }
}
