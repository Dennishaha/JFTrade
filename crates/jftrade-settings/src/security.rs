use std::sync::{Arc, Mutex};

use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::SettingsStoreError;
use crate::password_hash::{hash_argon2id, verify_argon2id};

pub const DEFAULT_WEB_ACCESS_PORT: u16 = 6688;
const MIN_WEB_ACCESS_PORT: i32 = 1024;
const MAX_WEB_ACCESS_PORT: i32 = 65_535;
const MIN_WEB_ACCESS_PASSWORD_CHARS: usize = 15;
const MAX_WEB_ACCESS_PASSWORD_BYTES: usize = 1024;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct SecuritySettings {
    pub web_access_enabled: bool,
    pub public_access_enabled: bool,
    pub web_port: u16,
    pub password_configured: bool,
}

impl Default for SecuritySettings {
    fn default() -> Self {
        Self {
            web_access_enabled: false,
            public_access_enabled: false,
            web_port: DEFAULT_WEB_ACCESS_PORT,
            password_configured: false,
        }
    }
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct SecuritySettingsUpdate {
    pub web_access_enabled: bool,
    pub public_access_enabled: bool,
    pub web_port: i32,
    pub new_password: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SecuritySettingsRecord {
    web_access_enabled: bool,
    public_access_enabled: bool,
    web_port: u16,
    password_hash: String,
}

impl Default for SecuritySettingsRecord {
    fn default() -> Self {
        Self {
            web_access_enabled: false,
            public_access_enabled: false,
            web_port: DEFAULT_WEB_ACCESS_PORT,
            password_hash: String::new(),
        }
    }
}

impl SecuritySettingsRecord {
    pub fn new(
        web_access_enabled: bool,
        public_access_enabled: bool,
        web_port: u16,
        password_hash: impl Into<String>,
    ) -> Self {
        normalize_security_record(Self {
            web_access_enabled,
            public_access_enabled,
            web_port,
            password_hash: password_hash.into(),
        })
    }

    pub fn web_access_enabled(&self) -> bool {
        self.web_access_enabled
    }

    pub fn public_access_enabled(&self) -> bool {
        self.public_access_enabled
    }

    pub fn web_port(&self) -> u16 {
        self.web_port
    }

    pub fn password_hash(&self) -> &str {
        &self.password_hash
    }

    pub fn public_settings(&self) -> SecuritySettings {
        SecuritySettings {
            web_access_enabled: self.web_access_enabled,
            public_access_enabled: self.public_access_enabled,
            web_port: self.web_port,
            password_configured: !self.password_hash.is_empty(),
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum SecuritySettingsError {
    #[error("a Web access password is required before Web access can be enabled")]
    PasswordRequired,
    #[error("web access password must contain at least 15 characters")]
    PasswordTooShort,
    #[error("web access password must contain at most 1024 bytes")]
    PasswordTooLong,
    #[error("web access port must be between 1024 and 65535")]
    InvalidPort,
    #[error("Web access password hashing failed: {0}")]
    PasswordHash(String),
    #[error("security settings store failed: {0}")]
    Store(#[from] SettingsStoreError),
    #[error("could not apply Web access listener settings: {message}")]
    Runtime { message: String },
    #[error(
        "could not apply Web access listener settings: {message}; settings rollback failed: {rollback}"
    )]
    RuntimeRollback { message: String, rollback: String },
}

pub trait SecuritySettingsStorePort: Send + Sync {
    fn load_security_record(&self) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError>;

    fn save_security_record(
        &self,
        record: &SecuritySettingsRecord,
    ) -> Result<(), SettingsStoreError>;
}

pub trait SecurityRuntimePort: Send + Sync {
    fn apply(&self, record: &SecuritySettingsRecord) -> Result<(), String>;
}

pub trait SecurityPasswordPort: Send + Sync {
    fn hash(&self, password: &str) -> Result<String, String>;
}

#[derive(Default)]
pub struct SystemSecurityPasswords;

impl SecurityPasswordPort for SystemSecurityPasswords {
    fn hash(&self, password: &str) -> Result<String, String> {
        hash_argon2id(password)
    }
}

#[derive(Clone)]
pub struct SecuritySettingsService {
    store: Arc<dyn SecuritySettingsStorePort>,
    runtime: Option<Arc<dyn SecurityRuntimePort>>,
    passwords: Arc<dyn SecurityPasswordPort>,
    write_lock: Arc<Mutex<()>>,
}

impl SecuritySettingsService {
    pub fn new(store: Arc<dyn SecuritySettingsStorePort>) -> Self {
        Self::with_ports(store, None, Arc::new(SystemSecurityPasswords))
    }

    pub fn with_ports(
        store: Arc<dyn SecuritySettingsStorePort>,
        runtime: Option<Arc<dyn SecurityRuntimePort>>,
        passwords: Arc<dyn SecurityPasswordPort>,
    ) -> Self {
        Self {
            store,
            runtime,
            passwords,
            write_lock: Arc::new(Mutex::new(())),
        }
    }

    pub fn settings(&self) -> Result<SecuritySettings, SecuritySettingsError> {
        Ok(self.record()?.public_settings())
    }

    pub fn save(
        &self,
        input: &SecuritySettingsUpdate,
    ) -> Result<SecuritySettings, SecuritySettingsError> {
        let _guard = self.lock_writes()?;
        let current = self.record()?;
        let web_port = if input.web_port == 0 {
            i32::from(current.web_port())
        } else {
            input.web_port
        };
        if !(MIN_WEB_ACCESS_PORT..=MAX_WEB_ACCESS_PORT).contains(&web_port) {
            return Err(SecuritySettingsError::InvalidPort);
        }
        let mut password_hash = current.password_hash().to_owned();
        if !input.new_password.is_empty() {
            validate_web_access_password(&input.new_password)?;
            password_hash = self
                .passwords
                .hash(&input.new_password)
                .map_err(SecuritySettingsError::PasswordHash)?;
        }
        if input.web_access_enabled && password_hash.is_empty() {
            return Err(SecuritySettingsError::PasswordRequired);
        }
        let next = SecuritySettingsRecord::new(
            input.web_access_enabled,
            input.web_access_enabled && input.public_access_enabled,
            u16::try_from(web_port).map_err(|_| SecuritySettingsError::InvalidPort)?,
            password_hash,
        );
        self.persist_and_apply(&current, &next)
            .map(|record| record.public_settings())
    }

    fn record(&self) -> Result<SecuritySettingsRecord, SecuritySettingsError> {
        Ok(self
            .store
            .load_security_record()?
            .map(normalize_security_record)
            .unwrap_or_default())
    }

    fn persist_and_apply(
        &self,
        current: &SecuritySettingsRecord,
        next: &SecuritySettingsRecord,
    ) -> Result<SecuritySettingsRecord, SecuritySettingsError> {
        self.store.save_security_record(next)?;
        let Some(runtime) = &self.runtime else {
            return Ok(next.clone());
        };
        if let Err(message) = runtime.apply(next) {
            return match self.store.save_security_record(current) {
                Ok(()) => Err(SecuritySettingsError::Runtime { message }),
                Err(rollback) => Err(SecuritySettingsError::RuntimeRollback {
                    message,
                    rollback: rollback.to_string(),
                }),
            };
        }
        Ok(next.clone())
    }

    fn lock_writes(&self) -> Result<std::sync::MutexGuard<'_, ()>, SecuritySettingsError> {
        self.write_lock.lock().map_err(|_| {
            SecuritySettingsError::Store(SettingsStoreError::new(
                "security settings write lock is poisoned",
            ))
        })
    }
}

pub fn normalize_security_settings(input: &SecuritySettings) -> SecuritySettings {
    let web_access_enabled = input.web_access_enabled && input.password_configured;
    SecuritySettings {
        web_access_enabled,
        public_access_enabled: web_access_enabled && input.public_access_enabled,
        web_port: if input.web_port == 0 {
            DEFAULT_WEB_ACCESS_PORT
        } else {
            input.web_port
        },
        password_configured: input.password_configured,
    }
}

pub fn verify_web_access_password(password_hash: &str, password: &str) -> bool {
    verify_argon2id(password_hash, password)
}

fn normalize_security_record(mut record: SecuritySettingsRecord) -> SecuritySettingsRecord {
    if record.web_port == 0 {
        record.web_port = DEFAULT_WEB_ACCESS_PORT;
    }
    record.password_hash = record.password_hash.trim().to_owned();
    if !record.web_access_enabled || record.password_hash.is_empty() {
        record.web_access_enabled = false;
        record.public_access_enabled = false;
    }
    record
}

fn validate_web_access_password(password: &str) -> Result<(), SecuritySettingsError> {
    if password.trim().is_empty() || password.chars().count() < MIN_WEB_ACCESS_PASSWORD_CHARS {
        return Err(SecuritySettingsError::PasswordTooShort);
    }
    if password.len() > MAX_WEB_ACCESS_PASSWORD_BYTES {
        return Err(SecuritySettingsError::PasswordTooLong);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    #[derive(Default)]
    struct Store(RwLock<Option<SecuritySettingsRecord>>);

    impl SecuritySettingsStorePort for Store {
        fn load_security_record(
            &self,
        ) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError> {
            self.0
                .read()
                .map(|record| record.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_security_record(
            &self,
            record: &SecuritySettingsRecord,
        ) -> Result<(), SettingsStoreError> {
            *self
                .0
                .write()
                .map_err(|_| SettingsStoreError::new("poisoned"))? = Some(record.clone());
            Ok(())
        }
    }

    struct FixedPasswords;

    impl SecurityPasswordPort for FixedPasswords {
        fn hash(&self, _password: &str) -> Result<String, String> {
            Ok("new-verifier".to_owned())
        }
    }

    struct FailingRuntime;

    impl SecurityRuntimePort for FailingRuntime {
        fn apply(&self, _record: &SecuritySettingsRecord) -> Result<(), String> {
            Err("port occupied".to_owned())
        }
    }

    #[test]
    fn access_fails_closed_without_a_configured_password() {
        let service = SecuritySettingsService::new(Arc::new(Store(RwLock::new(Some(
            SecuritySettingsRecord {
                web_access_enabled: true,
                public_access_enabled: true,
                web_port: 0,
                password_hash: String::new(),
            },
        )))));
        assert_eq!(
            service.settings().expect("settings"),
            SecuritySettings::default()
        );
    }

    #[test]
    fn writes_validate_password_port_and_public_access_like_go() {
        let service = SecuritySettingsService::with_ports(
            Arc::new(Store::default()),
            None,
            Arc::new(FixedPasswords),
        );
        assert_eq!(
            service.save(&SecuritySettingsUpdate {
                web_port: 80,
                ..SecuritySettingsUpdate::default()
            }),
            Err(SecuritySettingsError::InvalidPort)
        );
        assert_eq!(
            service.save(&SecuritySettingsUpdate {
                web_access_enabled: true,
                ..SecuritySettingsUpdate::default()
            }),
            Err(SecuritySettingsError::PasswordRequired)
        );
        assert_eq!(
            service.save(&SecuritySettingsUpdate {
                new_password: "short".to_owned(),
                ..SecuritySettingsUpdate::default()
            }),
            Err(SecuritySettingsError::PasswordTooShort)
        );
        let disabled = service
            .save(&SecuritySettingsUpdate {
                public_access_enabled: true,
                new_password: "a sufficiently long password".to_owned(),
                ..SecuritySettingsUpdate::default()
            })
            .expect("save disabled settings");
        assert!(!disabled.web_access_enabled);
        assert!(!disabled.public_access_enabled);
        assert!(disabled.password_configured);
    }

    #[test]
    fn listener_failure_rolls_back_password_and_port_together() {
        let original = SecuritySettingsRecord::new(true, false, 6688, "stored-verifier");
        let store = Arc::new(Store(RwLock::new(Some(original.clone()))));
        let service = SecuritySettingsService::with_ports(
            store.clone(),
            Some(Arc::new(FailingRuntime)),
            Arc::new(FixedPasswords),
        );
        let error = service
            .save(&SecuritySettingsUpdate {
                web_access_enabled: true,
                web_port: 7443,
                ..SecuritySettingsUpdate::default()
            })
            .expect_err("runtime failure");
        assert!(matches!(error, SecuritySettingsError::Runtime { .. }));
        assert_eq!(
            store.0.read().expect("read store").as_ref(),
            Some(&original)
        );
    }

    #[test]
    fn system_password_hash_is_go_compatible_and_rejects_wrong_password() {
        let password = "a sufficiently long password";
        let verifier = SystemSecurityPasswords
            .hash(password)
            .expect("hash password");
        assert!(verify_web_access_password(&verifier, password));
        assert!(!verify_web_access_password(&verifier, "wrong password"));
    }
}
