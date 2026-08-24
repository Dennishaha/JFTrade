#![forbid(unsafe_code)]

use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::RwLock;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease};
use jftrade_settings::{
    AssistantRuntimeSettings, AssistantRuntimeSettingsStorePort,
    BacktestMarketDataProviderSettingsStorePort, BrokerIntegration, BrokerSettingsInputs,
    BrokerSettingsStorePort, ExchangeCalendarManualOverride, ExchangeCalendarSettings,
    ExchangeCalendarSettingsStorePort, ExchangeCalendarSourcePolicy, ExecutionSettings,
    ExecutionSettingsStorePort, FutuIntegrationConfig, FutuOpenDInstallSettings,
    FutuOpenDInstallSettingsStorePort, InterfaceSettings, InterfaceSettingsStorePort,
    ManagedBrokerAccount, MarketDataProviderSettingsStorePort, McpServerSettingsRecord,
    McpServerSettingsStorePort, OnboardingInputs, OnboardingSettings, OnboardingSettingsStorePort,
    PineWorkerSettings, PineWorkerSettingsStorePort, SecuritySettingsRecord,
    SecuritySettingsStorePort, SettingsStoreError, SettingsStorePort, SystemNotificationSettings,
    SystemNotificationSettingsStorePort, UiAppearanceSettings, build_managed_account_id,
    same_managed_account_scope,
};
use serde::Deserialize;
use serde_json::{Map, Value};
use tempfile::Builder;

pub struct SettingsFileStore {
    path: PathBuf,
    document: RwLock<Map<String, Value>>,
    read_only: bool,
}

impl SettingsFileStore {
    pub fn open(path: impl Into<PathBuf>) -> Result<Self, SettingsStoreError> {
        Self::open_with_mode(path.into(), false)
    }

    pub fn open_read_only(path: impl Into<PathBuf>) -> Result<Self, SettingsStoreError> {
        Self::open_with_mode(path.into(), true)
    }

    fn open_with_mode(path: PathBuf, read_only: bool) -> Result<Self, SettingsStoreError> {
        if path.as_os_str().is_empty() {
            return Err(SettingsStoreError::new("settings path is required"));
        }
        let document = load_document(&path, !read_only)?;
        Ok(Self {
            path,
            document: RwLock::new(document),
            read_only,
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }
}

impl SettingsStorePort for SettingsFileStore {
    fn load_appearance(&self) -> Result<Option<UiAppearanceSettings>, SettingsStoreError> {
        self.load_field("appearance")
    }

    fn save_appearance(&self, appearance: &UiAppearanceSettings) -> Result<(), SettingsStoreError> {
        self.ensure_writable()?;
        let encoded = serde_json::to_value(appearance)
            .map_err(|error| SettingsStoreError::new(format!("encode appearance: {error}")))?;
        let mut document = self
            .document
            .write()
            .map_err(|_| SettingsStoreError::new("settings write lock is poisoned"))?;
        let mut next = document.clone();
        next.insert("appearance".to_owned(), encoded);
        persist_document(&self.path, &next)?;
        *document = next;
        Ok(())
    }
}

impl InterfaceSettingsStorePort for SettingsFileStore {
    fn load_interface_settings(&self) -> Result<Option<InterfaceSettings>, SettingsStoreError> {
        self.load_field("interfaces")
    }
}

impl ExecutionSettingsStorePort for SettingsFileStore {
    fn load_execution(&self) -> Result<Option<ExecutionSettings>, SettingsStoreError> {
        self.load_field("execution")
    }

    fn save_execution(&self, settings: &ExecutionSettings) -> Result<(), SettingsStoreError> {
        self.save_field("execution", settings)
    }
}

impl AssistantRuntimeSettingsStorePort for SettingsFileStore {
    fn load_assistant_runtime(
        &self,
    ) -> Result<Option<AssistantRuntimeSettings>, SettingsStoreError> {
        self.load_field("adk")
    }

    fn save_assistant_runtime(
        &self,
        settings: &AssistantRuntimeSettings,
    ) -> Result<(), SettingsStoreError> {
        self.save_field("adk", settings)
    }
}

impl SystemNotificationSettingsStorePort for SettingsFileStore {
    fn load_system_notifications(
        &self,
    ) -> Result<Option<SystemNotificationSettings>, SettingsStoreError> {
        self.load_field("systemNotifications")
    }

    fn save_system_notifications(
        &self,
        settings: &SystemNotificationSettings,
    ) -> Result<(), SettingsStoreError> {
        self.save_field("systemNotifications", settings)
    }
}

impl PineWorkerSettingsStorePort for SettingsFileStore {
    fn load_pine_worker(&self) -> Result<Option<PineWorkerSettings>, SettingsStoreError> {
        self.load_field("pineWorker")
    }

    fn save_pine_worker(&self, settings: &PineWorkerSettings) -> Result<(), SettingsStoreError> {
        self.save_field("pineWorker", settings)
    }
}

impl SecuritySettingsStorePort for SettingsFileStore {
    fn load_security_record(&self) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError> {
        let stored: Option<StoredSecuritySettings> = self.load_field("security")?;
        Ok(stored.map(|settings| {
            SecuritySettingsRecord::new(
                settings.web_access_enabled,
                settings.public_access_enabled,
                settings.web_port,
                settings.password_hash,
            )
        }))
    }

    fn save_security_record(
        &self,
        record: &SecuritySettingsRecord,
    ) -> Result<(), SettingsStoreError> {
        self.save_field(
            "security",
            &StoredSecuritySettings {
                web_access_enabled: record.web_access_enabled(),
                public_access_enabled: record.public_access_enabled(),
                web_port: record.web_port(),
                password_hash: record.password_hash().to_owned(),
            },
        )
    }
}

impl MarketDataProviderSettingsStorePort for SettingsFileStore {
    fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
        self.load_field("activeMarketDataProvider")
    }

    fn save_active_market_data_provider(
        &self,
        provider: jftrade_settings::MarketDataProvider,
    ) -> Result<(), SettingsStoreError> {
        let provider = jftrade_settings::provider_id(provider);
        self.save_field("activeMarketDataProvider", &provider)
    }
}

impl BacktestMarketDataProviderSettingsStorePort for SettingsFileStore {
    fn load_backtest_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
        let provider = self.load_field("backtestMarketDataProvider")?;
        if provider.is_some() {
            return Ok(provider);
        }
        self.load_field("activeMarketDataProvider")
    }

    fn save_backtest_market_data_provider(
        &self,
        provider: jftrade_settings::MarketDataProvider,
    ) -> Result<(), SettingsStoreError> {
        let provider = jftrade_settings::provider_id(provider);
        self.save_field("backtestMarketDataProvider", &provider)
    }
}

impl McpServerSettingsStorePort for SettingsFileStore {
    fn load_mcp_server_record(
        &self,
    ) -> Result<Option<McpServerSettingsRecord>, SettingsStoreError> {
        let stored: Option<StoredMcpServerSettings> = self.load_field("mcpServer")?;
        Ok(stored.map(|settings| {
            McpServerSettingsRecord::new(
                settings.enabled,
                settings.port,
                settings.auth_mode,
                settings.token_hash,
            )
        }))
    }

    fn save_mcp_server_record(
        &self,
        record: &McpServerSettingsRecord,
    ) -> Result<(), SettingsStoreError> {
        self.save_field(
            "mcpServer",
            &StoredMcpServerSettings {
                enabled: record.enabled(),
                port: record.port(),
                auth_mode: record.auth_mode().to_owned(),
                token_hash: record.token_hash().to_owned(),
            },
        )
    }
}

impl OnboardingSettingsStorePort for SettingsFileStore {
    fn load_onboarding_inputs(&self) -> Result<OnboardingInputs, SettingsStoreError> {
        let state = self
            .load_field::<OnboardingSettings>("onboarding")?
            .unwrap_or_default();
        let integration = self.load_field::<StoredBrokerIntegration>("integration")?;
        let accounts = self
            .load_field::<Vec<StoredManagedBrokerAccount>>("accounts")?
            .unwrap_or_default();
        Ok(OnboardingInputs {
            state,
            broker_enabled: integration.as_ref().is_some_and(|value| value.enabled),
            broker_configured: integration.is_some(),
            enabled_accounts: accounts.iter().filter(|account| account.enabled).count(),
        })
    }

    fn save_onboarding_settings(
        &self,
        settings: &OnboardingSettings,
    ) -> Result<OnboardingSettings, SettingsStoreError> {
        let normalized = jftrade_settings::normalize_onboarding_settings(settings);
        self.save_field("onboarding", &normalized)?;
        Ok(normalized)
    }
}

impl FutuOpenDInstallSettingsStorePort for SettingsFileStore {
    fn load_futu_open_d_install_settings(
        &self,
    ) -> Result<Option<FutuOpenDInstallSettings>, SettingsStoreError> {
        let integration = self.load_field::<StoredBrokerIntegration>("integration")?;
        Ok(integration.map(|integration| {
            let config = integration.config.unwrap_or_default();
            FutuOpenDInstallSettings {
                host: config.host,
                api_port: config.api_port,
                websocket_port: config.websocket_port,
                max_websocket_connections: config.max_websocket_connections,
                use_encryption: config.use_encryption,
                websocket_key_required: !config.websocket_key.trim().is_empty(),
            }
        }))
    }
}

impl BrokerSettingsStorePort for SettingsFileStore {
    fn load_broker_settings_inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError> {
        let document = self.document_snapshot()?;
        let saved_integration = decode_field::<BrokerIntegration>(&document, "integration")?;
        let accounts =
            decode_field::<Vec<ManagedBrokerAccount>>(&document, "accounts")?.unwrap_or_default();
        let effective_config = saved_integration
            .as_ref()
            .map(|integration| integration.config.clone())
            .unwrap_or_else(FutuIntegrationConfig::current_default);
        Ok(BrokerSettingsInputs {
            saved_integration,
            effective_config,
            accounts,
        })
    }

    fn save_broker_integration(
        &self,
        input: &BrokerIntegration,
        now: &str,
    ) -> Result<BrokerIntegration, SettingsStoreError> {
        let mut result = input.clone();
        result.updated_at = now.to_owned();
        self.mutate_document(|document| {
            if result.created_at.is_empty() {
                let existing = decode_field::<BrokerIntegration>(document, "integration")?;
                result.created_at = existing
                    .map(|integration| integration.created_at)
                    .filter(|created_at| !created_at.is_empty())
                    .unwrap_or_else(|| now.to_owned());
            }
            document.insert(
                "integration".to_owned(),
                encode_field(&result, "integration")?,
            );
            document.entry("interfaces".to_owned()).or_insert_with(|| {
                serde_json::json!({
                    "apiBind": "127.0.0.1:3000",
                    "liveWebSocketConnectionLimit": 20,
                })
            });
            Ok(result.clone())
        })
    }

    fn create_managed_broker_account(
        &self,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<ManagedBrokerAccount, SettingsStoreError> {
        self.mutate_document(|document| {
            let mut accounts = decode_field::<Vec<ManagedBrokerAccount>>(document, "accounts")?
                .unwrap_or_default();
            let mut result = input.clone();
            result.updated_at = now.to_owned();
            if let Some(index) = accounts
                .iter()
                .position(|account| same_managed_account_scope(account, &result))
            {
                result.id.clone_from(&accounts[index].id);
                result.created_at.clone_from(&accounts[index].created_at);
                if result.created_at.is_empty() {
                    result.created_at = now.to_owned();
                }
                accounts[index] = result.clone();
            } else {
                result.id = build_managed_account_id(&result);
                result.created_at = now.to_owned();
                accounts.push(result.clone());
            }
            document.insert("accounts".to_owned(), encode_field(&accounts, "accounts")?);
            Ok(result)
        })
    }

    fn update_managed_broker_account(
        &self,
        id: &str,
        input: &ManagedBrokerAccount,
        now: &str,
    ) -> Result<Option<ManagedBrokerAccount>, SettingsStoreError> {
        self.ensure_writable()?;
        let mut document = self
            .document
            .write()
            .map_err(|_| SettingsStoreError::new("settings write lock is poisoned"))?;
        let mut accounts =
            decode_field::<Vec<ManagedBrokerAccount>>(&document, "accounts")?.unwrap_or_default();
        let Some(index) = accounts.iter().position(|account| account.id == id) else {
            return Ok(None);
        };
        let mut result = input.clone();
        result.id.clone_from(&accounts[index].id);
        result.created_at.clone_from(&accounts[index].created_at);
        result.updated_at = now.to_owned();
        accounts[index] = result.clone();
        let mut next = document.clone();
        next.insert("accounts".to_owned(), encode_field(&accounts, "accounts")?);
        persist_document(&self.path, &next)?;
        *document = next;
        Ok(Some(result))
    }

    fn delete_managed_broker_account(&self, id: &str) -> Result<bool, SettingsStoreError> {
        self.ensure_writable()?;
        let mut document = self
            .document
            .write()
            .map_err(|_| SettingsStoreError::new("settings write lock is poisoned"))?;
        let mut accounts =
            decode_field::<Vec<ManagedBrokerAccount>>(&document, "accounts")?.unwrap_or_default();
        let Some(index) = accounts.iter().position(|account| account.id == id) else {
            return Ok(false);
        };
        accounts.remove(index);
        let mut next = document.clone();
        next.insert("accounts".to_owned(), encode_field(&accounts, "accounts")?);
        persist_document(&self.path, &next)?;
        *document = next;
        Ok(true)
    }
}

impl ExchangeCalendarSettingsStorePort for SettingsFileStore {
    fn load_exchange_calendars(
        &self,
    ) -> Result<Option<ExchangeCalendarSettings>, SettingsStoreError> {
        let stored: Option<StoredExchangeCalendarSettings> =
            self.load_field("exchangeCalendars")?;
        Ok(stored.map(|settings| ExchangeCalendarSettings {
            auto_refresh_enabled: settings.auto_refresh_enabled,
            error_notifications_enabled: settings.error_notifications_enabled.unwrap_or(true),
            refresh_interval_hours: settings.refresh_interval_hours,
            warmup_markets: settings.warmup_markets,
            source_policies: settings.source_policies,
            manual_overrides: settings.manual_overrides,
        }))
    }

    fn save_exchange_calendars(
        &self,
        settings: &ExchangeCalendarSettings,
    ) -> Result<ExchangeCalendarSettings, SettingsStoreError> {
        let normalized = jftrade_settings::normalize_exchange_calendar_settings(settings.clone());
        self.save_field("exchangeCalendars", &normalized)?;
        Ok(normalized)
    }
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredExchangeCalendarSettings {
    auto_refresh_enabled: bool,
    error_notifications_enabled: Option<bool>,
    refresh_interval_hours: i32,
    warmup_markets: Vec<String>,
    source_policies: Vec<ExchangeCalendarSourcePolicy>,
    manual_overrides: Vec<ExchangeCalendarManualOverride>,
}

#[derive(Debug, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredSecuritySettings {
    web_access_enabled: bool,
    public_access_enabled: bool,
    web_port: u16,
    password_hash: String,
}

#[derive(Debug, Default, Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredMcpServerSettings {
    enabled: bool,
    port: i32,
    auth_mode: String,
    token_hash: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredBrokerIntegration {
    enabled: bool,
    config: Option<StoredFutuIntegrationConfig>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredFutuIntegrationConfig {
    host: String,
    api_port: i32,
    websocket_port: i32,
    #[serde(rename = "maxWebSocketConnections")]
    max_websocket_connections: i32,
    use_encryption: bool,
    websocket_key: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct StoredManagedBrokerAccount {
    enabled: bool,
}

impl Default for StoredSecuritySettings {
    fn default() -> Self {
        Self {
            web_access_enabled: false,
            public_access_enabled: false,
            web_port: jftrade_settings::DEFAULT_WEB_ACCESS_PORT,
            password_hash: String::new(),
        }
    }
}

impl SettingsFileStore {
    fn load_field<T: serde::de::DeserializeOwned>(
        &self,
        field: &str,
    ) -> Result<Option<T>, SettingsStoreError> {
        decode_field(&self.document_snapshot()?, field)
    }

    fn document_snapshot(&self) -> Result<Map<String, Value>, SettingsStoreError> {
        if self.read_only {
            return load_document(&self.path, false);
        }
        self.document
            .read()
            .map(|document| document.clone())
            .map_err(|_| SettingsStoreError::new("settings read lock is poisoned"))
    }

    fn mutate_document<T>(
        &self,
        mutate: impl FnOnce(&mut Map<String, Value>) -> Result<T, SettingsStoreError>,
    ) -> Result<T, SettingsStoreError> {
        self.ensure_writable()?;
        let mut document = self
            .document
            .write()
            .map_err(|_| SettingsStoreError::new("settings write lock is poisoned"))?;
        let mut next = document.clone();
        let result = mutate(&mut next)?;
        persist_document(&self.path, &next)?;
        *document = next;
        Ok(result)
    }

    fn save_field<T: serde::Serialize>(
        &self,
        field: &str,
        value: &T,
    ) -> Result<(), SettingsStoreError> {
        self.ensure_writable()?;
        let encoded = serde_json::to_value(value)
            .map_err(|error| SettingsStoreError::new(format!("encode {field}: {error}")))?;
        let mut document = self
            .document
            .write()
            .map_err(|_| SettingsStoreError::new("settings write lock is poisoned"))?;
        let mut next = document.clone();
        next.insert(field.to_owned(), encoded);
        persist_document(&self.path, &next)?;
        *document = next;
        Ok(())
    }

    fn ensure_writable(&self) -> Result<(), SettingsStoreError> {
        if self.read_only {
            return Err(SettingsStoreError::new(
                "settings store is read-only while Go owns production writes",
            ));
        }
        Ok(())
    }
}

fn decode_field<T: serde::de::DeserializeOwned>(
    document: &Map<String, Value>,
    field: &str,
) -> Result<Option<T>, SettingsStoreError> {
    let Some(value) = document.get(field) else {
        return Ok(None);
    };
    if value.is_null() {
        return Ok(None);
    }
    serde_json::from_value(value.clone())
        .map(Some)
        .map_err(|error| SettingsStoreError::new(format!("decode {field}: {error}")))
}

fn encode_field<T: serde::Serialize>(value: &T, field: &str) -> Result<Value, SettingsStoreError> {
    serde_json::to_value(value)
        .map_err(|error| SettingsStoreError::new(format!("encode {field}: {error}")))
}

fn load_document(
    path: &Path,
    harden_permissions: bool,
) -> Result<Map<String, Value>, SettingsStoreError> {
    let contents = match fs::read(path) {
        Ok(contents) => contents,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Map::new()),
        Err(error) => {
            return Err(SettingsStoreError::new(format!(
                "read {}: {error}",
                path.display()
            )));
        }
    };
    if harden_permissions {
        harden_existing_path(path)?;
    }
    if contents.iter().all(u8::is_ascii_whitespace) {
        return Ok(Map::new());
    }
    let value: Value = serde_json::from_slice(&contents)
        .map_err(|error| SettingsStoreError::new(format!("decode {}: {error}", path.display())))?;
    let document = value
        .as_object()
        .cloned()
        .ok_or_else(|| SettingsStoreError::new("settings document must be a JSON object"))?;
    validate_supported_fields(&document)?;
    Ok(document)
}

fn validate_supported_fields(document: &Map<String, Value>) -> Result<(), SettingsStoreError> {
    validate_field::<UiAppearanceSettings>(document, "appearance")?;
    validate_field::<InterfaceSettings>(document, "interfaces")?;
    validate_field::<ExecutionSettings>(document, "execution")?;
    validate_field::<AssistantRuntimeSettings>(document, "adk")?;
    validate_field::<SystemNotificationSettings>(document, "systemNotifications")?;
    validate_field::<PineWorkerSettings>(document, "pineWorker")?;
    validate_field::<StoredSecuritySettings>(document, "security")?;
    validate_field::<String>(document, "activeMarketDataProvider")?;
    validate_field::<String>(document, "backtestMarketDataProvider")?;
    validate_field::<StoredMcpServerSettings>(document, "mcpServer")?;
    validate_field::<OnboardingSettings>(document, "onboarding")?;
    validate_field::<BrokerIntegration>(document, "integration")?;
    validate_field::<Vec<ManagedBrokerAccount>>(document, "accounts")?;
    validate_field::<StoredExchangeCalendarSettings>(document, "exchangeCalendars")
}

fn validate_field<T: serde::de::DeserializeOwned>(
    document: &Map<String, Value>,
    field: &str,
) -> Result<(), SettingsStoreError> {
    let Some(value) = document.get(field) else {
        return Ok(());
    };
    if value.is_null() {
        return Ok(());
    }
    serde_json::from_value::<T>(value.clone())
        .map(|_| ())
        .map_err(|error| SettingsStoreError::new(format!("decode {field}: {error}")))
}

fn persist_document(path: &Path, document: &Map<String, Value>) -> Result<(), SettingsStoreError> {
    let directory = path
        .parent()
        .filter(|value| !value.as_os_str().is_empty())
        .unwrap_or(Path::new("."));
    fs::create_dir_all(directory).map_err(|error| {
        SettingsStoreError::new(format!(
            "create settings directory {}: {error}",
            directory.display()
        ))
    })?;
    harden_directory(directory)?;
    let profile = std::env::var("JFTRADE_RUST_REHEARSAL_PROFILE")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| "rust-standalone".to_owned());
    let _lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile)).map_err(
        |error| SettingsStoreError::new(format!("acquire settings writer lease: {error}")),
    )?;
    let encoded = serde_json::to_vec_pretty(document)
        .map_err(|error| SettingsStoreError::new(format!("encode settings: {error}")))?;
    let mut temporary = Builder::new()
        .prefix(".settings-")
        .suffix(".tmp")
        .tempfile_in(directory)
        .map_err(|error| {
            SettingsStoreError::new(format!("create settings temporary file: {error}"))
        })?;
    harden_file(temporary.as_file())?;
    temporary
        .write_all(&encoded)
        .and_then(|()| temporary.as_file().sync_all())
        .map_err(|error| {
            SettingsStoreError::new(format!("write settings temporary file: {error}"))
        })?;
    temporary.persist(path).map_err(|error| {
        SettingsStoreError::new(format!("replace {}: {}", path.display(), error.error))
    })?;
    sync_directory(directory)?;
    Ok(())
}

#[cfg(unix)]
fn harden_directory(path: &Path) -> Result<(), SettingsStoreError> {
    use std::os::unix::fs::PermissionsExt;

    fs::set_permissions(path, fs::Permissions::from_mode(0o700)).map_err(|error| {
        SettingsStoreError::new(format!(
            "secure settings directory {}: {error}",
            path.display()
        ))
    })
}

#[cfg(not(unix))]
fn harden_directory(_path: &Path) -> Result<(), SettingsStoreError> {
    Ok(())
}

#[cfg(unix)]
fn harden_file(file: &File) -> Result<(), SettingsStoreError> {
    use std::os::unix::fs::PermissionsExt;

    file.set_permissions(fs::Permissions::from_mode(0o600))
        .map_err(|error| SettingsStoreError::new(format!("secure settings file: {error}")))
}

#[cfg(not(unix))]
fn harden_file(_file: &File) -> Result<(), SettingsStoreError> {
    Ok(())
}

fn harden_existing_path(path: &Path) -> Result<(), SettingsStoreError> {
    if let Some(directory) = path.parent().filter(|value| !value.as_os_str().is_empty()) {
        harden_directory(directory)?;
    }
    let file = File::open(path).map_err(|error| {
        SettingsStoreError::new(format!("open settings file {}: {error}", path.display()))
    })?;
    harden_file(&file)
}

#[cfg(unix)]
fn sync_directory(path: &Path) -> Result<(), SettingsStoreError> {
    File::open(path)
        .and_then(|directory| directory.sync_all())
        .map_err(|error| {
            SettingsStoreError::new(format!(
                "sync settings directory {}: {error}",
                path.display()
            ))
        })
}

#[cfg(not(unix))]
fn sync_directory(_path: &Path) -> Result<(), SettingsStoreError> {
    Ok(())
}
