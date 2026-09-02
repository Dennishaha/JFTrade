use std::sync::Arc;

use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::SettingsStoreError;

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct FutuIntegrationConfig {
    #[serde(rename = "type")]
    pub integration_type: String,
    pub host: String,
    pub api_port: i32,
    pub websocket_port: i32,
    #[serde(rename = "maxWebSocketConnections")]
    pub max_websocket_connections: i32,
    pub use_encryption: bool,
    pub websocket_key: String,
    pub trade_market: String,
    pub security_firm: String,
}

impl FutuIntegrationConfig {
    pub fn current_default() -> Self {
        Self {
            integration_type: "futu".to_owned(),
            host: "127.0.0.1".to_owned(),
            api_port: 11_110,
            websocket_port: 11_111,
            max_websocket_connections: 20,
            use_encryption: false,
            websocket_key: String::new(),
            trade_market: "HK".to_owned(),
            security_firm: "FUTUSECURITIES".to_owned(),
        }
    }
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct BrokerIntegration {
    pub broker_id: String,
    pub enabled: bool,
    pub config: FutuIntegrationConfig,
    pub updated_at: String,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ManagedBrokerAccount {
    pub id: String,
    pub broker_id: String,
    pub account_id: String,
    pub display_name: String,
    pub trading_environment: String,
    pub market: String,
    pub security_firm: Option<String>,
    pub enabled: bool,
    pub updated_at: String,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct BrokerSettingsInputs {
    pub saved_integration: Option<BrokerIntegration>,
    pub effective_config: FutuIntegrationConfig,
    pub accounts: Vec<ManagedBrokerAccount>,
}

pub trait BrokerSettingsStorePort: Send + Sync {
    fn load_broker_settings_inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError>;

    fn save_broker_integration(
        &self,
        input: &BrokerIntegration,
        now: &str,
    ) -> Result<BrokerIntegration, SettingsStoreError>;

    fn create_managed_broker_account(
        &self,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<ManagedBrokerAccount, SettingsStoreError>;

    fn update_managed_broker_account(
        &self,
        id: &str,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<Option<ManagedBrokerAccount>, SettingsStoreError>;

    fn delete_managed_broker_account(&self, id: &str) -> Result<bool, SettingsStoreError>;
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum BrokerSettingsError {
    #[error("accountId is required")]
    MissingAccountId,
    #[error("managed broker account not found")]
    AccountNotFound,
    #[error(transparent)]
    Store(#[from] SettingsStoreError),
}

#[derive(Clone)]
pub struct BrokerSettingsService {
    store: Arc<dyn BrokerSettingsStorePort>,
}

impl BrokerSettingsService {
    pub fn new(store: Arc<dyn BrokerSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError> {
        self.store.load_broker_settings_inputs()
    }

    pub fn save_integration(
        &self,
        input: &BrokerIntegration,
        now: &str,
    ) -> Result<BrokerIntegration, BrokerSettingsError> {
        let mut normalized = input.clone();
        normalized.broker_id = "futu".to_owned();
        normalized.config = normalize_futu_integration_config(&normalized.config);
        Ok(self.store.save_broker_integration(&normalized, now)?)
    }

    pub fn create_account(
        &self,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<ManagedBrokerAccount, BrokerSettingsError> {
        if input.account_id.trim().is_empty() {
            return Err(BrokerSettingsError::MissingAccountId);
        }
        let mut normalized = normalize_managed_broker_account(input);
        normalized.id.clear();
        normalized.created_at.clear();
        normalized.updated_at.clear();
        Ok(self.store.create_managed_broker_account(&normalized, now)?)
    }

    pub fn update_account(
        &self,
        id: &str,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<ManagedBrokerAccount, BrokerSettingsError> {
        let normalized = normalize_managed_broker_account(input);
        self.store
            .update_managed_broker_account(id, &normalized, now)?
            .ok_or(BrokerSettingsError::AccountNotFound)
    }

    pub fn delete_account(&self, id: &str) -> Result<(), BrokerSettingsError> {
        if self.store.delete_managed_broker_account(id)? {
            Ok(())
        } else {
            Err(BrokerSettingsError::AccountNotFound)
        }
    }
}

pub fn normalize_futu_integration_config(input: &FutuIntegrationConfig) -> FutuIntegrationConfig {
    let mut config = input.clone();
    if config.integration_type.is_empty() {
        config.integration_type = "futu".to_owned();
    }
    if config.host.trim().is_empty() {
        config.host = "127.0.0.1".to_owned();
    }
    if config.api_port <= 0 {
        config.api_port = 11_110;
    }
    if config.websocket_port <= 0 {
        config.websocket_port = 11_111;
    }
    if config.max_websocket_connections <= 0 {
        config.max_websocket_connections = 20;
    }
    if config.trade_market.trim().is_empty() {
        config.trade_market = "HK".to_owned();
    }
    if config.security_firm.trim().is_empty() {
        config.security_firm = "FUTUSECURITIES".to_owned();
    }
    config.use_encryption = false;
    config
}

pub fn normalize_managed_broker_account(input: &ManagedBrokerAccount) -> ManagedBrokerAccount {
    let mut account = input.clone();
    account.broker_id = account.broker_id.trim().to_ascii_lowercase();
    if account.broker_id.is_empty() {
        account.broker_id = "futu".to_owned();
    }
    account.account_id = account.account_id.trim().to_owned();
    account.display_name = account.display_name.trim().to_owned();
    if account.display_name.is_empty() {
        account.display_name.clone_from(&account.account_id);
    }
    account.trading_environment = account.trading_environment.trim().to_ascii_uppercase();
    if account.trading_environment.is_empty() {
        account.trading_environment = "SIMULATE".to_owned();
    }
    account.market = account.market.trim().to_ascii_uppercase();
    if account.market.is_empty() {
        account.market = "HK".to_owned();
    }
    account.security_firm = account
        .security_firm
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned);
    account
}

pub fn same_managed_account_scope(
    left: &ManagedBrokerAccount,
    right: &ManagedBrokerAccount,
) -> bool {
    left.broker_id == right.broker_id
        && left.account_id == right.account_id
        && left.trading_environment == right.trading_environment
        && left.market == right.market
}

pub fn build_managed_account_id(input: &ManagedBrokerAccount) -> String {
    [
        input.broker_id.as_str(),
        input.trading_environment.as_str(),
        input.account_id.as_str(),
        input.market.as_str(),
    ]
    .join("|")
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    struct Store(RwLock<BrokerSettingsInputs>);

    impl BrokerSettingsStorePort for Store {
        fn load_broker_settings_inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError> {
            self.0
                .read()
                .map(|inputs| inputs.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_broker_integration(
            &self,
            input: &BrokerIntegration,
            _now: &str,
        ) -> Result<BrokerIntegration, SettingsStoreError> {
            Ok(input.clone())
        }

        fn create_managed_broker_account(
            &self,
            input: &ManagedBrokerAccount,
            _now: &str,
        ) -> Result<ManagedBrokerAccount, SettingsStoreError> {
            Ok(input.clone())
        }

        fn update_managed_broker_account(
            &self,
            _id: &str,
            input: &ManagedBrokerAccount,
            _now: &str,
        ) -> Result<Option<ManagedBrokerAccount>, SettingsStoreError> {
            Ok(Some(input.clone()))
        }

        fn delete_managed_broker_account(&self, _id: &str) -> Result<bool, SettingsStoreError> {
            Ok(true)
        }
    }

    #[test]
    fn service_preserves_persisted_broker_wire_without_normalizing_secrets() {
        let expected = BrokerSettingsInputs {
            saved_integration: Some(BrokerIntegration {
                broker_id: "futu".to_owned(),
                enabled: true,
                config: FutuIntegrationConfig {
                    websocket_key: "secret".to_owned(),
                    ..FutuIntegrationConfig::default()
                },
                ..BrokerIntegration::default()
            }),
            effective_config: FutuIntegrationConfig::current_default(),
            accounts: vec![ManagedBrokerAccount {
                id: "managed-1".to_owned(),
                enabled: true,
                ..ManagedBrokerAccount::default()
            }],
        };
        let service = BrokerSettingsService::new(Arc::new(Store(RwLock::new(expected.clone()))));
        assert_eq!(service.inputs().expect("broker inputs"), expected);
    }

    #[test]
    fn write_normalization_matches_current_go_owner() {
        let service = BrokerSettingsService::new(Arc::new(Store(RwLock::new(
            BrokerSettingsInputs::default(),
        ))));
        let integration = service
            .save_integration(
                &BrokerIntegration {
                    broker_id: "other".to_owned(),
                    enabled: true,
                    config: FutuIntegrationConfig {
                        host: "  ".to_owned(),
                        use_encryption: true,
                        ..FutuIntegrationConfig::default()
                    },
                    ..BrokerIntegration::default()
                },
                "2026-08-20T00:00:00Z",
            )
            .expect("normalize integration");
        assert_eq!(integration.broker_id, "futu");
        assert_eq!(integration.config.host, "127.0.0.1");
        assert!(!integration.config.use_encryption);

        let account = service
            .create_account(
                &ManagedBrokerAccount {
                    broker_id: " FUTU ".to_owned(),
                    account_id: " ACC-1 ".to_owned(),
                    security_firm: Some(" ".to_owned()),
                    enabled: true,
                    ..ManagedBrokerAccount::default()
                },
                "2026-08-20T00:00:00Z",
            )
            .expect("normalize account");
        assert_eq!(account.display_name, "ACC-1");
        assert_eq!(account.trading_environment, "SIMULATE");
        assert_eq!(account.market, "HK");
        assert_eq!(account.security_firm, None);
    }
}
