//! Cross-service failure-path parity for the Go `internal/settings` owner.
//!
//! These tests exercise the public services with scripted stores whose durable
//! writes fail on demand, so the reported error type and the preserved
//! on-disk/on-memory projection stay aligned with the Go baseline.

use std::sync::{Arc, Mutex};

use jftrade_settings::{
    BrokerIntegration, BrokerSettingsError, BrokerSettingsInputs, BrokerSettingsService,
    BrokerSettingsStorePort, DEFAULT_MCP_SERVER_PORT, ExchangeCalendarSettings,
    ExchangeCalendarSettingsService, ExchangeCalendarSettingsStorePort, ExecutionService,
    ExecutionSettings, ExecutionSettingsStorePort, McpServerRuntimePort, McpServerSecretPort,
    McpServerSettingsError, McpServerSettingsRecord, McpServerSettingsService,
    McpServerSettingsStorePort, McpServerSettingsUpdate, PineWorkerSettings,
    PineWorkerSettingsService, PineWorkerSettingsStorePort, SecurityPasswordPort,
    SecurityRuntimePort, SecuritySettingsError, SecuritySettingsRecord, SecuritySettingsService,
    SecuritySettingsStorePort, SecuritySettingsUpdate, SettingsStoreError,
};

struct ScriptedMcpStore {
    record: Mutex<Option<McpServerSettingsRecord>>,
    save_failures: Mutex<Vec<Option<String>>>,
}

impl ScriptedMcpStore {
    fn new(record: Option<McpServerSettingsRecord>, save_failures: Vec<Option<String>>) -> Self {
        Self {
            record: Mutex::new(record),
            save_failures: Mutex::new(save_failures),
        }
    }
}

impl McpServerSettingsStorePort for ScriptedMcpStore {
    fn load_mcp_server_record(
        &self,
    ) -> Result<Option<McpServerSettingsRecord>, SettingsStoreError> {
        Ok(self.record.lock().expect("stored record").clone())
    }

    fn save_mcp_server_record(
        &self,
        record: &McpServerSettingsRecord,
    ) -> Result<(), SettingsStoreError> {
        let mut failures = self.save_failures.lock().expect("save script");
        if !failures.is_empty()
            && let Some(message) = failures.remove(0)
        {
            return Err(SettingsStoreError::new(message));
        }
        *self.record.lock().expect("stored record") = Some(record.clone());
        Ok(())
    }
}

struct FailingMcpRuntime;

impl McpServerRuntimePort for FailingMcpRuntime {
    fn apply(&self, _record: &McpServerSettingsRecord) -> Result<(), String> {
        Err("port occupied".to_owned())
    }
}

struct WorkingSecrets;

impl McpServerSecretPort for WorkingSecrets {
    fn issue(&self) -> Result<(String, String), String> {
        Ok(("issued-token".to_owned(), "issued-verifier".to_owned()))
    }
}

struct BrokenSecrets;

impl McpServerSecretPort for BrokenSecrets {
    fn issue(&self) -> Result<(String, String), String> {
        Err("token generator unavailable".to_owned())
    }
}

struct ScriptedSecurityStore {
    record: Mutex<Option<SecuritySettingsRecord>>,
    save_failures: Mutex<Vec<Option<String>>>,
}

impl ScriptedSecurityStore {
    fn new(record: Option<SecuritySettingsRecord>, save_failures: Vec<Option<String>>) -> Self {
        Self {
            record: Mutex::new(record),
            save_failures: Mutex::new(save_failures),
        }
    }
}

impl SecuritySettingsStorePort for ScriptedSecurityStore {
    fn load_security_record(&self) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError> {
        Ok(self.record.lock().expect("stored record").clone())
    }

    fn save_security_record(
        &self,
        record: &SecuritySettingsRecord,
    ) -> Result<(), SettingsStoreError> {
        let mut failures = self.save_failures.lock().expect("save script");
        if !failures.is_empty()
            && let Some(message) = failures.remove(0)
        {
            return Err(SettingsStoreError::new(message));
        }
        *self.record.lock().expect("stored record") = Some(record.clone());
        Ok(())
    }
}

struct FailingSecurityRuntime;

impl SecurityRuntimePort for FailingSecurityRuntime {
    fn apply(&self, _record: &SecuritySettingsRecord) -> Result<(), String> {
        Err("listener unavailable".to_owned())
    }
}

struct FailingSecurityPasswords;

impl SecurityPasswordPort for FailingSecurityPasswords {
    fn hash(&self, _password: &str) -> Result<String, String> {
        Err("hasher unavailable".to_owned())
    }
}

struct FailingExecutionStore;

impl ExecutionSettingsStorePort for FailingExecutionStore {
    fn load_execution(&self) -> Result<Option<ExecutionSettings>, SettingsStoreError> {
        Ok(None)
    }

    fn save_execution(&self, _settings: &ExecutionSettings) -> Result<(), SettingsStoreError> {
        Err(SettingsStoreError::new("execution store failed"))
    }
}

struct FailingPineStore;

impl PineWorkerSettingsStorePort for FailingPineStore {
    fn load_pine_worker(&self) -> Result<Option<PineWorkerSettings>, SettingsStoreError> {
        Ok(None)
    }

    fn save_pine_worker(&self, _settings: &PineWorkerSettings) -> Result<(), SettingsStoreError> {
        Err(SettingsStoreError::new("pine worker store failed"))
    }
}

struct FailingCalendarStore;

impl ExchangeCalendarSettingsStorePort for FailingCalendarStore {
    fn load_exchange_calendars(
        &self,
    ) -> Result<Option<ExchangeCalendarSettings>, SettingsStoreError> {
        Ok(None)
    }

    fn save_exchange_calendars(
        &self,
        _settings: &ExchangeCalendarSettings,
    ) -> Result<ExchangeCalendarSettings, SettingsStoreError> {
        Err(SettingsStoreError::new("calendar store failed"))
    }
}

struct FailingBrokerStore;

impl BrokerSettingsStorePort for FailingBrokerStore {
    fn load_broker_settings_inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError> {
        Ok(BrokerSettingsInputs::default())
    }

    fn save_broker_integration(
        &self,
        _input: &BrokerIntegration,
        _now: &str,
    ) -> Result<BrokerIntegration, SettingsStoreError> {
        Err(SettingsStoreError::new("broker store failed"))
    }

    fn create_managed_broker_account(
        &self,
        _input: &jftrade_settings::ManagedBrokerAccount,
        _now: &str,
    ) -> Result<jftrade_settings::ManagedBrokerAccount, SettingsStoreError> {
        Err(SettingsStoreError::new("broker store failed"))
    }

    fn update_managed_broker_account(
        &self,
        _id: &str,
        _input: &jftrade_settings::ManagedBrokerAccount,
        _now: &str,
    ) -> Result<Option<jftrade_settings::ManagedBrokerAccount>, SettingsStoreError> {
        Err(SettingsStoreError::new("broker store failed"))
    }

    fn delete_managed_broker_account(&self, _id: &str) -> Result<bool, SettingsStoreError> {
        Err(SettingsStoreError::new("broker store failed"))
    }
}

/// Parity: go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:125 TestServicePreservesSecurityAndMCPFallbacks
#[test]
fn rollback_and_disabled_fallbacks_preserve_the_stored_projection() {
    // Security: enabling Web access without a password fails closed.
    let empty_security = Arc::new(ScriptedSecurityStore::new(None, Vec::new()));
    let security = SecuritySettingsService::new(empty_security);
    assert_eq!(
        security.save(&SecuritySettingsUpdate {
            web_access_enabled: true,
            ..SecuritySettingsUpdate::default()
        }),
        Err(SecuritySettingsError::PasswordRequired)
    );

    // Security: a listener failure whose rollback also fails reports both.
    let security_store = Arc::new(ScriptedSecurityStore::new(
        Some(SecuritySettingsRecord::new(false, false, 6697, "")),
        vec![None, Some("settings rollback unavailable".to_owned())],
    ));
    let security = SecuritySettingsService::with_ports(
        security_store.clone(),
        Some(Arc::new(FailingSecurityRuntime)),
        Arc::new(jftrade_settings::SystemSecurityPasswords),
    );
    let error = security
        .save(&SecuritySettingsUpdate {
            web_access_enabled: true,
            new_password: "a password long enough".to_owned(),
            ..SecuritySettingsUpdate::default()
        })
        .expect_err("listener and rollback failure");
    let message = error.to_string();
    assert!(message.contains("listener unavailable"), "{message}");
    assert!(message.contains("settings rollback failed"), "{message}");
    assert!(
        message.contains("settings rollback unavailable"),
        "{message}"
    );

    // MCP: a disabled update keeps the stored port and auth mode instead of
    // falling back to process defaults.
    let mcp_store = Arc::new(ScriptedMcpStore::new(
        Some(McpServerSettingsRecord::new(false, 0, "none", "")),
        Vec::new(),
    ));
    let mcp =
        McpServerSettingsService::with_ports(mcp_store.clone(), None, Arc::new(WorkingSecrets));
    let fallback = mcp
        .save(&McpServerSettingsUpdate {
            enabled: false,
            port: 0,
            auth_mode: String::new(),
        })
        .expect("disabled fallback");
    assert_eq!(fallback.port, DEFAULT_MCP_SERVER_PORT);
    assert_eq!(fallback.auth_mode, "none");
    assert!(!fallback.token_configured);

    // MCP: a secret generator failure surfaces before any durable write.
    let broken_store = Arc::new(ScriptedMcpStore::new(
        Some(McpServerSettingsRecord::new(false, 6697, "token", "")),
        Vec::new(),
    ));
    let broken = McpServerSettingsService::with_ports(broken_store, None, Arc::new(BrokenSecrets));
    let error = broken.reset_token().expect_err("secret generator failure");
    assert!(matches!(error, McpServerSettingsError::Secret(_)));
    assert!(error.to_string().contains("token generator unavailable"));
}

/// Parity: go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:90 TestServiceRollsBackMCPOnSaveFailure
#[test]
fn store_and_rollback_failures_propagate_across_settings_writes() {
    // MCP: the runtime listener fails and the settings rollback fails too.
    let mcp_store = Arc::new(ScriptedMcpStore::new(
        Some(McpServerSettingsRecord::new(false, 6697, "none", "")),
        vec![None, Some("rollback failed".to_owned())],
    ));
    let mcp = McpServerSettingsService::with_ports(
        mcp_store.clone(),
        Some(Arc::new(FailingMcpRuntime)),
        Arc::new(WorkingSecrets),
    );
    let error = mcp
        .save(&McpServerSettingsUpdate {
            enabled: true,
            port: 6697,
            auth_mode: "none".to_owned(),
        })
        .expect_err("listener rollback failure");
    assert!(matches!(
        error,
        McpServerSettingsError::RuntimeRollback { .. }
    ));
    let message = error.to_string();
    assert!(message.contains("port occupied"), "{message}");
    assert!(message.contains("settings rollback failed"), "{message}");
    assert!(message.contains("rollback failed"), "{message}");

    // MCP: a token reset whose durable write fails reports the store error.
    let token_store = Arc::new(ScriptedMcpStore::new(
        Some(McpServerSettingsRecord::new(
            true,
            6697,
            "token",
            "old-verifier",
        )),
        vec![Some("token persist failed".to_owned())],
    ));
    let token_service =
        McpServerSettingsService::with_ports(token_store, None, Arc::new(WorkingSecrets));
    let error = token_service
        .reset_token()
        .expect_err("token persistence failure");
    assert!(matches!(error, McpServerSettingsError::Store(_)));
    assert!(error.to_string().contains("token persist failed"));

    // The simple settings owners report their store failures unchanged.
    let error = PineWorkerSettingsService::new(Arc::new(FailingPineStore))
        .save(&PineWorkerSettings::default())
        .expect_err("pine worker persistence failure");
    assert!(error.message().contains("pine worker store failed"));

    let error = ExchangeCalendarSettingsService::new(Arc::new(FailingCalendarStore))
        .save(ExchangeCalendarSettings::default())
        .expect_err("calendar persistence failure");
    assert!(error.message().contains("calendar store failed"));

    let error = BrokerSettingsService::new(Arc::new(FailingBrokerStore))
        .save_integration(&BrokerIntegration::default(), "2026-08-20T00:00:00Z")
        .expect_err("broker persistence failure");
    assert!(matches!(error, BrokerSettingsError::Store(_)));
    assert!(error.to_string().contains("broker store failed"));
}

/// The default projection used when nothing has been persisted.
#[test]
fn default_mcp_settings_match_the_go_service_defaults() {
    let settings = jftrade_settings::McpServerSettings::default();
    assert_eq!(settings.port, DEFAULT_MCP_SERVER_PORT);
    assert_eq!(settings.auth_mode, "token");
    assert!(!settings.enabled);
    assert!(!settings.token_configured);

    let store = Arc::new(ScriptedMcpStore::new(None, Vec::new()));
    let service = McpServerSettingsService::new(store);
    assert_eq!(service.settings().expect("default settings"), settings);
}

/// Parity: go:452dea11:internal/settings/persistence_and_mcp_failures_test.go:55 TestServiceReportsPersistenceAndMCPFailures
#[test]
fn execution_and_security_write_failures_surface_to_callers() {
    let error = ExecutionService::new(Arc::new(FailingExecutionStore))
        .save(&ExecutionSettings::default())
        .expect_err("execution persistence failure");
    assert!(error.message().contains("execution store failed"));

    let hashing = SecuritySettingsService::with_ports(
        Arc::new(ScriptedSecurityStore::new(None, Vec::new())),
        None,
        Arc::new(FailingSecurityPasswords),
    );
    let error = hashing
        .save(&SecuritySettingsUpdate {
            new_password: "a password long enough".to_owned(),
            ..SecuritySettingsUpdate::default()
        })
        .expect_err("password hashing failure");
    assert!(matches!(error, SecuritySettingsError::PasswordHash(_)));
    assert!(error.to_string().contains("hasher unavailable"));

    let storing = SecuritySettingsService::with_ports(
        Arc::new(ScriptedSecurityStore::new(
            Some(SecuritySettingsRecord::new(false, false, 6697, "")),
            vec![Some("security persist failed".to_owned())],
        )),
        None,
        Arc::new(jftrade_settings::SystemSecurityPasswords),
    );
    let error = storing
        .save(&SecuritySettingsUpdate {
            new_password: "a password long enough".to_owned(),
            ..SecuritySettingsUpdate::default()
        })
        .expect_err("security persistence failure");
    assert!(matches!(error, SecuritySettingsError::Store(_)));
    assert!(error.to_string().contains("security persist failed"));
}
