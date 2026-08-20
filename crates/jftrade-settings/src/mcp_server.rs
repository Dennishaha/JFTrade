use std::sync::{Arc, Mutex};

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::SettingsStoreError;
use crate::password_hash::{hash_argon2id, verify_argon2id};

pub const DEFAULT_MCP_SERVER_PORT: i32 = 6697;
const MIN_MCP_SERVER_PORT: i32 = 1024;
const MAX_MCP_SERVER_PORT: i32 = 65_535;
const MCP_SERVER_TOKEN_BYTES: usize = 32;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct McpServerSettings {
    pub enabled: bool,
    pub port: i32,
    pub auth_mode: String,
    pub token_configured: bool,
}

impl Default for McpServerSettings {
    fn default() -> Self {
        Self {
            enabled: false,
            port: DEFAULT_MCP_SERVER_PORT,
            auth_mode: "token".to_owned(),
            token_configured: false,
        }
    }
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct McpServerSettingsUpdate {
    pub enabled: bool,
    pub port: i32,
    pub auth_mode: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct McpServerSettingsRecord {
    enabled: bool,
    port: i32,
    auth_mode: String,
    token_hash: String,
}

impl Default for McpServerSettingsRecord {
    fn default() -> Self {
        Self {
            enabled: false,
            port: DEFAULT_MCP_SERVER_PORT,
            auth_mode: "token".to_owned(),
            token_hash: String::new(),
        }
    }
}

impl McpServerSettingsRecord {
    pub fn new(
        enabled: bool,
        port: i32,
        auth_mode: impl Into<String>,
        token_hash: impl Into<String>,
    ) -> Self {
        normalize_mcp_server_record(Self {
            enabled,
            port,
            auth_mode: auth_mode.into(),
            token_hash: token_hash.into(),
        })
    }

    pub fn enabled(&self) -> bool {
        self.enabled
    }

    pub fn port(&self) -> i32 {
        self.port
    }

    pub fn auth_mode(&self) -> &str {
        &self.auth_mode
    }

    pub fn token_hash(&self) -> &str {
        &self.token_hash
    }

    pub fn public_settings(&self) -> McpServerSettings {
        McpServerSettings {
            enabled: self.enabled,
            port: self.port,
            auth_mode: self.auth_mode.clone(),
            token_configured: !self.token_hash.trim().is_empty(),
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpServerStatus {
    pub running: bool,
    pub endpoint: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_error: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpServerSettingsSnapshot {
    pub settings: McpServerSettings,
    pub status: McpServerStatus,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpServerTokenResetResult {
    pub settings: McpServerSettings,
    pub status: McpServerStatus,
    pub token: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum McpServerSettingsError {
    #[error("MCP server port must be between 1024 and 65535")]
    InvalidPort,
    #[error("MCP server auth mode must be token or none")]
    InvalidAuthMode,
    #[error("an MCP server token is required before token authentication can be enabled")]
    TokenRequired,
    #[error("MCP server secret generation failed: {0}")]
    Secret(String),
    #[error("MCP server settings store failed: {0}")]
    Store(#[from] SettingsStoreError),
    #[error("could not apply MCP server listener settings: {message}")]
    Runtime { message: String },
    #[error(
        "could not apply MCP server listener settings: {message}; settings rollback failed: {rollback}"
    )]
    RuntimeRollback { message: String, rollback: String },
}

pub trait McpServerSettingsStorePort: Send + Sync {
    fn load_mcp_server_record(&self)
    -> Result<Option<McpServerSettingsRecord>, SettingsStoreError>;

    fn save_mcp_server_record(
        &self,
        record: &McpServerSettingsRecord,
    ) -> Result<(), SettingsStoreError>;
}

pub trait McpServerRuntimePort: Send + Sync {
    fn apply(&self, record: &McpServerSettingsRecord) -> Result<(), String>;
}

pub trait McpServerSecretPort: Send + Sync {
    fn issue(&self) -> Result<(String, String), String>;
}

#[derive(Default)]
pub struct SystemMcpServerSecrets;

impl McpServerSecretPort for SystemMcpServerSecrets {
    fn issue(&self) -> Result<(String, String), String> {
        let mut token_bytes = [0_u8; MCP_SERVER_TOKEN_BYTES];
        getrandom::fill(&mut token_bytes).map_err(|error| error.to_string())?;
        let token = format!("jft_mcp_{}", URL_SAFE_NO_PAD.encode(token_bytes));

        let token_hash = hash_argon2id(&token)?;
        Ok((token, token_hash))
    }
}

#[derive(Clone)]
pub struct McpServerSettingsService {
    store: Arc<dyn McpServerSettingsStorePort>,
    runtime: Option<Arc<dyn McpServerRuntimePort>>,
    secrets: Arc<dyn McpServerSecretPort>,
    write_lock: Arc<Mutex<()>>,
}

impl McpServerSettingsService {
    pub fn new(store: Arc<dyn McpServerSettingsStorePort>) -> Self {
        Self::with_ports(store, None, Arc::new(SystemMcpServerSecrets))
    }

    pub fn with_ports(
        store: Arc<dyn McpServerSettingsStorePort>,
        runtime: Option<Arc<dyn McpServerRuntimePort>>,
        secrets: Arc<dyn McpServerSecretPort>,
    ) -> Self {
        Self {
            store,
            runtime,
            secrets,
            write_lock: Arc::new(Mutex::new(())),
        }
    }

    pub fn settings(&self) -> Result<McpServerSettings, McpServerSettingsError> {
        Ok(self.record()?.public_settings())
    }

    pub fn stopped_snapshot(&self) -> Result<McpServerSettingsSnapshot, McpServerSettingsError> {
        let settings = self.settings()?;
        Ok(McpServerSettingsSnapshot {
            status: stopped_status(settings.port),
            settings,
        })
    }

    pub fn save(
        &self,
        input: &McpServerSettingsUpdate,
    ) -> Result<McpServerSettings, McpServerSettingsError> {
        let _guard = self.lock_writes()?;
        let current = self.record()?;
        let port = if input.port == 0 {
            current.port()
        } else {
            input.port
        };
        if !(MIN_MCP_SERVER_PORT..=MAX_MCP_SERVER_PORT).contains(&port) {
            return Err(McpServerSettingsError::InvalidPort);
        }
        let auth_mode = if input.auth_mode.trim().is_empty() {
            current.auth_mode().to_owned()
        } else {
            input.auth_mode.trim().to_ascii_lowercase()
        };
        if !matches!(auth_mode.as_str(), "token" | "none") {
            return Err(McpServerSettingsError::InvalidAuthMode);
        }
        let next = McpServerSettingsRecord::new(
            input.enabled,
            port,
            auth_mode,
            current.token_hash().to_owned(),
        );
        if next.enabled() && next.auth_mode() == "token" && next.token_hash().trim().is_empty() {
            return Err(McpServerSettingsError::TokenRequired);
        }
        self.persist_and_apply(&current, &next)
            .map(|record| record.public_settings())
    }

    pub fn reset_token(&self) -> Result<McpServerTokenResetResult, McpServerSettingsError> {
        let _guard = self.lock_writes()?;
        let current = self.record()?;
        let (token, token_hash) = self
            .secrets
            .issue()
            .map_err(McpServerSettingsError::Secret)?;
        let next = McpServerSettingsRecord::new(
            current.enabled(),
            current.port(),
            current.auth_mode(),
            token_hash,
        );
        let settings = self.persist_and_apply(&current, &next)?.public_settings();
        Ok(McpServerTokenResetResult {
            status: stopped_status(settings.port),
            settings,
            token,
        })
    }

    fn record(&self) -> Result<McpServerSettingsRecord, McpServerSettingsError> {
        Ok(self
            .store
            .load_mcp_server_record()?
            .map(normalize_mcp_server_record)
            .unwrap_or_default())
    }

    fn persist_and_apply(
        &self,
        current: &McpServerSettingsRecord,
        next: &McpServerSettingsRecord,
    ) -> Result<McpServerSettingsRecord, McpServerSettingsError> {
        self.store.save_mcp_server_record(next)?;
        let Some(runtime) = &self.runtime else {
            return Ok(next.clone());
        };
        if let Err(message) = runtime.apply(next) {
            return match self.store.save_mcp_server_record(current) {
                Ok(()) => Err(McpServerSettingsError::Runtime { message }),
                Err(rollback) => Err(McpServerSettingsError::RuntimeRollback {
                    message,
                    rollback: rollback.to_string(),
                }),
            };
        }
        Ok(next.clone())
    }

    fn lock_writes(&self) -> Result<std::sync::MutexGuard<'_, ()>, McpServerSettingsError> {
        self.write_lock.lock().map_err(|_| {
            McpServerSettingsError::Store(SettingsStoreError::new(
                "MCP settings write lock is poisoned",
            ))
        })
    }
}

pub fn normalize_mcp_server_settings(input: &McpServerSettings) -> McpServerSettings {
    let record = McpServerSettingsRecord::new(
        input.enabled,
        input.port,
        input.auth_mode.clone(),
        if input.token_configured {
            "configured"
        } else {
            ""
        },
    );
    record.public_settings()
}

pub fn verify_mcp_server_token(token_hash: &str, token: &str) -> bool {
    verify_argon2id(token_hash, token)
}

fn normalize_mcp_server_record(mut record: McpServerSettingsRecord) -> McpServerSettingsRecord {
    if !(MIN_MCP_SERVER_PORT..=MAX_MCP_SERVER_PORT).contains(&record.port) {
        record.port = DEFAULT_MCP_SERVER_PORT;
    }
    record.auth_mode = if record.auth_mode.trim().eq_ignore_ascii_case("none") {
        "none".to_owned()
    } else {
        "token".to_owned()
    };
    record
}

fn stopped_status(port: i32) -> McpServerStatus {
    McpServerStatus {
        running: false,
        endpoint: format!("http://127.0.0.1:{port}/mcp"),
        last_error: String::new(),
    }
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use argon2::password_hash::PasswordHash;

    use super::*;

    #[derive(Default)]
    struct Store(RwLock<Option<McpServerSettingsRecord>>);

    impl McpServerSettingsStorePort for Store {
        fn load_mcp_server_record(
            &self,
        ) -> Result<Option<McpServerSettingsRecord>, SettingsStoreError> {
            self.0
                .read()
                .map(|settings| settings.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_mcp_server_record(
            &self,
            record: &McpServerSettingsRecord,
        ) -> Result<(), SettingsStoreError> {
            *self
                .0
                .write()
                .map_err(|_| SettingsStoreError::new("poisoned"))? = Some(record.clone());
            Ok(())
        }
    }

    struct FixedSecrets;

    impl McpServerSecretPort for FixedSecrets {
        fn issue(&self) -> Result<(String, String), String> {
            Ok(("one-time-secret".to_owned(), "stored-verifier".to_owned()))
        }
    }

    struct FailingRuntime;

    impl McpServerRuntimePort for FailingRuntime {
        fn apply(&self, _record: &McpServerSettingsRecord) -> Result<(), String> {
            Err("port occupied".to_owned())
        }
    }

    #[test]
    fn snapshot_normalizes_public_settings_and_reports_unowned_listener_stopped() {
        let service = McpServerSettingsService::new(Arc::new(Store(RwLock::new(Some(
            McpServerSettingsRecord {
                enabled: true,
                port: 80,
                auth_mode: " other ".to_owned(),
                token_hash: "stored-verifier".to_owned(),
            },
        )))));
        let snapshot = service.stopped_snapshot().expect("MCP snapshot");
        assert_eq!(snapshot.settings.port, DEFAULT_MCP_SERVER_PORT);
        assert_eq!(snapshot.settings.auth_mode, "token");
        assert!(snapshot.settings.token_configured);
        assert!(!snapshot.status.running);
        assert_eq!(snapshot.status.endpoint, "http://127.0.0.1:6697/mcp");
    }

    #[test]
    fn writes_validate_like_go_and_never_accept_a_caller_supplied_token() {
        let store = Arc::new(Store::default());
        let service = McpServerSettingsService::with_ports(store, None, Arc::new(FixedSecrets));
        assert_eq!(
            service.save(&McpServerSettingsUpdate {
                enabled: true,
                port: 80,
                auth_mode: "token".to_owned(),
            }),
            Err(McpServerSettingsError::InvalidPort)
        );
        assert_eq!(
            service.save(&McpServerSettingsUpdate {
                enabled: true,
                port: DEFAULT_MCP_SERVER_PORT,
                auth_mode: "token".to_owned(),
            }),
            Err(McpServerSettingsError::TokenRequired)
        );
        assert_eq!(
            service.save(&McpServerSettingsUpdate {
                auth_mode: "invalid".to_owned(),
                ..McpServerSettingsUpdate::default()
            }),
            Err(McpServerSettingsError::InvalidAuthMode)
        );
    }

    #[test]
    fn token_reset_returns_secret_once_and_persists_only_its_verifier() {
        let store = Arc::new(Store::default());
        let service =
            McpServerSettingsService::with_ports(store.clone(), None, Arc::new(FixedSecrets));
        let result = service.reset_token().expect("reset token");
        assert_eq!(result.token, "one-time-secret");
        assert!(result.settings.token_configured);
        let encoded = serde_json::to_string(&result.settings).expect("encode settings");
        assert!(!encoded.contains("one-time-secret"));
        assert!(!encoded.contains("stored-verifier"));
        let stored = store.0.read().expect("read store").clone().expect("record");
        assert_eq!(stored.token_hash(), "stored-verifier");
    }

    #[test]
    fn listener_failure_rolls_back_the_persisted_settings() {
        let original = McpServerSettingsRecord::new(false, 6697, "none", "old-verifier");
        let store = Arc::new(Store(RwLock::new(Some(original.clone()))));
        let service = McpServerSettingsService::with_ports(
            store.clone(),
            Some(Arc::new(FailingRuntime)),
            Arc::new(FixedSecrets),
        );
        let error = service
            .save(&McpServerSettingsUpdate {
                enabled: false,
                port: 7443,
                auth_mode: "none".to_owned(),
            })
            .expect_err("runtime failure");
        assert!(matches!(error, McpServerSettingsError::Runtime { .. }));
        assert_eq!(
            store.0.read().expect("read store").as_ref(),
            Some(&original)
        );
    }

    #[test]
    fn system_secret_uses_go_compatible_token_and_argon2id_verifier() {
        let (token, verifier) = SystemMcpServerSecrets.issue().expect("issue secret");
        assert!(token.starts_with("jft_mcp_"));
        assert_eq!(token.len(), 51);
        let parsed = PasswordHash::new(&verifier).expect("parse PHC verifier");
        assert_eq!(parsed.algorithm.as_str(), "argon2id");
        assert_eq!(parsed.version, Some(19));
        assert_eq!(parsed.params.get_decimal("m"), Some(65_536));
        assert_eq!(parsed.params.get_decimal("t"), Some(3));
        assert_eq!(parsed.params.get_decimal("p"), Some(1));
        assert!(verify_mcp_server_token(&verifier, &token));
        assert!(!verify_mcp_server_token(&verifier, "wrong-token"));
    }
}
