use std::collections::BTreeMap;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use jftrade_api::{SESSION_COOKIE, WebSessionValidator};
use jftrade_settings::SecuritySettingsService;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use crate::product::product_auth_session_write_port::{
    AuthSessionWriteInput, AuthSessionWritePort, AuthSessionWritePortError,
    AuthSessionWritePortResult,
};
use crate::product::{
    AuthSessionSnapshotError, AuthSessionSnapshotPort, AuthSessionSnapshotRequest,
};

const SESSION_TTL: Duration = Duration::from_secs(7 * 24 * 3600);
const MAX_FAILED_ATTEMPTS: usize = 5;
const RATE_LIMIT_WINDOW: Duration = Duration::from_secs(60);
const RATE_LIMIT_LOCKOUT: Duration = Duration::from_secs(60);
const SESSION_STORE_VERSION: &str = "jftrade.web-sessions.v1";
const SESSION_STORE_FILENAME: &str = "web-sessions.json";

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct StoredSession {
    token_hash: String,
    csrf_hash: String,
    expires_at_unix: i64,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct StoredSessionDocument {
    version: String,
    sessions: Vec<StoredSession>,
}

#[derive(Clone, Debug)]
pub struct ProductionAuthSessionManager {
    sessions: Arc<RwLock<BTreeMap<String, StoredSession>>>,
    failed_attempts: Arc<Mutex<(usize, Instant)>>,
    security: SecuritySettingsService,
    session_path: Arc<PathBuf>,
}

pub(crate) trait AuthSessionInvalidationPort: Send + Sync + std::fmt::Debug {
    fn invalidate_all_sessions(&self) -> Result<(), String>;
}

impl ProductionAuthSessionManager {
    pub fn open(security: SecuritySettingsService, settings_path: &Path) -> Result<Self, String> {
        let session_path = settings_path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
            .map_or_else(
                || PathBuf::from(SESSION_STORE_FILENAME),
                |parent| parent.join(SESSION_STORE_FILENAME),
            );
        Self::open_at(security, session_path)
    }

    fn open_at(security: SecuritySettingsService, session_path: PathBuf) -> Result<Self, String> {
        let mut sessions = load_sessions(&session_path)?;
        sessions.retain(|_, session| !session_expired(session));
        let manager = Self {
            sessions: Arc::new(RwLock::new(sessions)),
            failed_attempts: Arc::new(Mutex::new((0, Instant::now()))),
            security,
            session_path: Arc::new(session_path),
        };
        manager.persist()?;
        Ok(manager)
    }

    pub fn invalidate_all(&self) -> Result<(), String> {
        let mut guard = self
            .sessions
            .write()
            .map_err(|_| "Web session state lock is poisoned".to_owned())?;
        let previous = guard.clone();
        guard.clear();
        if let Err(error) = persist_sessions(&self.session_path, &guard) {
            *guard = previous;
            return Err(error);
        }
        Ok(())
    }

    fn generate_random_token() -> Result<String, String> {
        let mut bytes = [0u8; 32];
        getrandom::fill(&mut bytes)
            .map_err(|error| format!("generate Web session token: {error}"))?;
        Ok(hex::encode(bytes))
    }

    fn persist(&self) -> Result<(), String> {
        let guard = self
            .sessions
            .read()
            .map_err(|_| "Web session state lock is poisoned".to_owned())?;
        persist_sessions(&self.session_path, &guard)
    }
}

impl AuthSessionInvalidationPort for ProductionAuthSessionManager {
    fn invalidate_all_sessions(&self) -> Result<(), String> {
        self.invalidate_all()
    }
}

impl WebSessionValidator for ProductionAuthSessionManager {
    fn is_session_valid(&self, session_cookie: &str) -> bool {
        self.sessions
            .read()
            .ok()
            .and_then(|sessions| sessions.get(&token_hash(session_cookie)).cloned())
            .is_some_and(|session| !session_expired(&session))
    }

    fn is_csrf_valid(&self, session_cookie: &str, csrf_header: &str) -> bool {
        let Ok(guard) = self.sessions.read() else {
            return false;
        };
        let Some(session) = guard.get(&token_hash(session_cookie)) else {
            return false;
        };
        !session_expired(session) && constant_time_eq(&session.csrf_hash, &token_hash(csrf_header))
    }
}

impl AuthSessionSnapshotPort for ProductionAuthSessionManager {
    fn session(
        &self,
        request: AuthSessionSnapshotRequest,
    ) -> Result<Value, AuthSessionSnapshotError> {
        let browser_authenticated = request.browser_authenticated
            && request
                .session_cookie
                .as_deref()
                .is_some_and(|cookie| self.is_session_valid(cookie));
        let authenticated = request.desktop_trusted || browser_authenticated;
        let csrf_token = if browser_authenticated {
            request
                .session_cookie
                .as_deref()
                .map(derive_csrf_token)
                .map(Value::String)
                .unwrap_or(Value::Null)
        } else {
            Value::Null
        };

        Ok(json!({
            "authenticated": authenticated,
            "desktop": request.desktop_trusted,
            "browser": browser_authenticated,
            "csrfToken": csrf_token
        }))
    }
}

impl AuthSessionWritePort for ProductionAuthSessionManager {
    fn login_rate_limit(&self) -> Option<AuthSessionWritePortError> {
        if let Ok(guard) = self.failed_attempts.lock() {
            let (count, last_attempt) = *guard;
            if count >= MAX_FAILED_ATTEMPTS && last_attempt.elapsed() < RATE_LIMIT_LOCKOUT {
                let remaining = RATE_LIMIT_LOCKOUT.saturating_sub(last_attempt.elapsed());
                return Some(AuthSessionWritePortError::RateLimited {
                    retry_after: remaining.as_secs().max(1),
                    message: "too many failed login attempts".to_owned(),
                });
            }
        }
        None
    }

    fn mutate(
        &self,
        input: &AuthSessionWriteInput,
    ) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError> {
        match input {
            AuthSessionWriteInput::Login { password } => {
                if let Some(rate_limit) = self.login_rate_limit() {
                    return Err(rate_limit);
                }

                let enabled = self
                    .security
                    .is_web_access_enabled()
                    .map_err(|e| AuthSessionWritePortError::Unavailable(e.to_string()))?;
                if !enabled {
                    return Err(AuthSessionWritePortError::Unavailable(
                        "Web access is disabled; enable it in the desktop settings".to_owned(),
                    ));
                }

                let valid = self
                    .security
                    .verify_password(password)
                    .map_err(|e| AuthSessionWritePortError::Failed(e.to_string()))?;

                if !valid {
                    if let Ok(mut guard) = self.failed_attempts.lock() {
                        let (count, last_attempt) = *guard;
                        if last_attempt.elapsed() > RATE_LIMIT_WINDOW {
                            *guard = (1, Instant::now());
                        } else {
                            *guard = (count + 1, Instant::now());
                        }
                    }
                    return Err(AuthSessionWritePortError::InvalidPassword(
                        "invalid Web access password".to_owned(),
                    ));
                }

                // Reset failed attempts on success
                if let Ok(mut guard) = self.failed_attempts.lock() {
                    *guard = (0, Instant::now());
                }

                let session_token = Self::generate_random_token()
                    .map_err(AuthSessionWritePortError::Unavailable)?;
                let csrf_token = derive_csrf_token(&session_token);
                let stored = StoredSession {
                    token_hash: token_hash(&session_token),
                    csrf_hash: token_hash(&csrf_token),
                    expires_at_unix: unix_timestamp()
                        .saturating_add(i64::try_from(SESSION_TTL.as_secs()).unwrap_or(i64::MAX)),
                };
                let mut guard = self.sessions.write().map_err(|_| {
                    AuthSessionWritePortError::Unavailable(
                        "Web session state lock is poisoned".to_owned(),
                    )
                })?;
                let previous = guard.insert(stored.token_hash.clone(), stored.clone());
                if let Err(error) = persist_sessions(&self.session_path, &guard) {
                    if let Some(previous) = previous {
                        guard.insert(stored.token_hash, previous);
                    } else {
                        guard.remove(&stored.token_hash);
                    }
                    return Err(AuthSessionWritePortError::Unavailable(error));
                }
                drop(guard);

                let cookie = format!(
                    "{}={}; Path=/; HttpOnly; SameSite=Lax; Max-Age={}",
                    SESSION_COOKIE,
                    session_token,
                    SESSION_TTL.as_secs()
                );

                Ok(AuthSessionWritePortResult {
                    data: json!({
                        "authenticated": true,
                        "desktop": false,
                        "browser": true,
                        "csrfToken": csrf_token,
                    }),
                    set_cookie: Some(cookie),
                })
            }
            AuthSessionWriteInput::Logout { session_cookie } => {
                if let Some(token) = session_cookie {
                    let token_hash = token_hash(token);
                    let mut guard = self.sessions.write().map_err(|_| {
                        AuthSessionWritePortError::Unavailable(
                            "Web session state lock is poisoned".to_owned(),
                        )
                    })?;
                    let removed = guard.remove(&token_hash);
                    if let Err(error) = persist_sessions(&self.session_path, &guard) {
                        if let Some(removed) = removed {
                            guard.insert(token_hash, removed);
                        }
                        return Err(AuthSessionWritePortError::Unavailable(error));
                    }
                }

                let cookie = format!(
                    "{}=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0",
                    SESSION_COOKIE
                );

                Ok(AuthSessionWritePortResult {
                    data: json!({
                        "authenticated": false,
                        "desktop": false,
                        "browser": false,
                        "csrfToken": null,
                    }),
                    set_cookie: Some(cookie),
                })
            }
        }
    }
}

fn load_sessions(path: &Path) -> Result<BTreeMap<String, StoredSession>, String> {
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(BTreeMap::new()),
        Err(error) => return Err(format!("read Web session store: {error}")),
    };
    let document: StoredSessionDocument = serde_json::from_slice(&bytes)
        .map_err(|error| format!("decode Web session store: {error}"))?;
    if document.version != SESSION_STORE_VERSION {
        return Err(format!(
            "unsupported Web session store version {}",
            document.version
        ));
    }
    let mut sessions = BTreeMap::new();
    for session in document.sessions {
        if session.token_hash.len() != 64 || session.csrf_hash.len() != 64 {
            return Err("Web session store contains an invalid token hash".to_owned());
        }
        if sessions
            .insert(session.token_hash.clone(), session)
            .is_some()
        {
            return Err("Web session store contains a duplicate token hash".to_owned());
        }
    }
    Ok(sessions)
}

fn persist_sessions(path: &Path, sessions: &BTreeMap<String, StoredSession>) -> Result<(), String> {
    let parent = path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(|error| format!("create Web session directory: {error}"))?;
    let document = StoredSessionDocument {
        version: SESSION_STORE_VERSION.to_owned(),
        sessions: sessions.values().cloned().collect(),
    };
    let bytes = serde_json::to_vec_pretty(&document)
        .map_err(|error| format!("encode Web session store: {error}"))?;
    let mut file = tempfile::NamedTempFile::new_in(parent)
        .map_err(|error| format!("create Web session temporary file: {error}"))?;
    file.write_all(&bytes)
        .map_err(|error| format!("write Web session store: {error}"))?;
    file.write_all(b"\n")
        .map_err(|error| format!("write Web session store newline: {error}"))?;
    file.as_file()
        .sync_all()
        .map_err(|error| format!("sync Web session store: {error}"))?;
    file.persist(path)
        .map_err(|error| format!("replace Web session store: {}", error.error))?;
    Ok(())
}

fn unix_timestamp() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_secs()).ok())
        .unwrap_or_default()
}

fn session_expired(session: &StoredSession) -> bool {
    unix_timestamp() >= session.expires_at_unix
}

fn derive_csrf_token(session_token: &str) -> String {
    token_hash(&format!("jftrade.csrf.v1:{session_token}"))
}

fn token_hash(token: &str) -> String {
    let mut digest = Sha256::new();
    digest.update(token.as_bytes());
    hex::encode(digest.finalize())
}

fn constant_time_eq(a: &str, b: &str) -> bool {
    let a_bytes = a.as_bytes();
    let b_bytes = b.as_bytes();
    if a_bytes.len() != b_bytes.len() {
        return false;
    }
    let mut diff = 0;
    for (x, y) in a_bytes.iter().zip(b_bytes.iter()) {
        diff |= x ^ y;
    }
    diff == 0
}

mod hex {
    pub fn encode(bytes: impl AsRef<[u8]>) -> String {
        const HEX_CHARS: &[u8; 16] = b"0123456789abcdef";
        let bytes = bytes.as_ref();
        let mut hex = String::with_capacity(bytes.len() * 2);
        for &byte in bytes {
            hex.push(HEX_CHARS[(byte >> 4) as usize] as char);
            hex.push(HEX_CHARS[(byte & 0x0f) as usize] as char);
        }
        hex
    }
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use jftrade_settings::{
        SecurityPasswordPort, SecuritySettingsRecord, SecuritySettingsStorePort, SettingsStoreError,
    };

    use super::*;

    struct MockSecurityStore(RwLock<Option<SecuritySettingsRecord>>);

    impl SecuritySettingsStorePort for MockSecurityStore {
        fn load_security_record(
            &self,
        ) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError> {
            Ok(self.0.read().unwrap().clone())
        }

        fn save_security_record(
            &self,
            record: &SecuritySettingsRecord,
        ) -> Result<(), SettingsStoreError> {
            *self.0.write().unwrap() = Some(record.clone());
            Ok(())
        }
    }

    #[test]
    fn auth_manager_login_validate_and_logout_flow() {
        let directory = tempfile::tempdir().expect("temporary directory");
        // Create Argon2 hash of "correct-password"
        let hash = jftrade_settings::SystemSecurityPasswords
            .hash("correct-password")
            .expect("hash password");

        let store = Arc::new(MockSecurityStore(RwLock::new(Some(
            SecuritySettingsRecord::new(true, false, 3000, hash),
        ))));
        let security = SecuritySettingsService::new(store);
        let manager =
            ProductionAuthSessionManager::open(security, &directory.path().join("settings.json"))
                .expect("open auth manager");

        // 1. Initial snapshot with unauthenticated request
        let snap = manager
            .session(AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: false,
                session_cookie: None,
                origin_provided: false,
                origin_allowed: false,
            })
            .expect("snapshot");
        assert_eq!(snap["authenticated"], false);

        let invalid_browser = manager
            .session(AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: true,
                session_cookie: Some("stale-cookie".to_owned()),
                origin_provided: true,
                origin_allowed: true,
            })
            .expect("snapshot with stale cookie");
        assert_eq!(invalid_browser["authenticated"], false);
        assert_eq!(invalid_browser["browser"], false);
        assert!(invalid_browser["csrfToken"].is_null());

        // 2. Login with wrong password
        let err = manager.mutate(&AuthSessionWriteInput::Login {
            password: "wrong-password".to_owned(),
        });
        assert!(matches!(
            err,
            Err(AuthSessionWritePortError::InvalidPassword(_))
        ));

        // 3. Login with correct password
        let res = manager
            .mutate(&AuthSessionWriteInput::Login {
                password: "correct-password".to_owned(),
            })
            .expect("login successful");
        assert_eq!(res.data["authenticated"], true);
        let csrf_token = res.data["csrfToken"]
            .as_str()
            .expect("csrf token")
            .to_owned();
        let cookie_header = res.set_cookie.expect("set cookie header");
        assert!(cookie_header.starts_with("jftrade_web_session="));
        let session_token = cookie_header
            .split(';')
            .next()
            .unwrap()
            .strip_prefix("jftrade_web_session=")
            .unwrap()
            .to_owned();

        // 4. Validate session and CSRF
        assert!(manager.is_session_valid(&session_token));
        assert!(manager.is_csrf_valid(&session_token, &csrf_token));
        assert!(!manager.is_csrf_valid(&session_token, "wrong-csrf"));
        assert!(!manager.is_session_valid("invalid-session"));

        // 5. Snapshot with authenticated session
        let snap2 = manager
            .session(AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: true,
                session_cookie: Some(session_token.clone()),
                origin_provided: true,
                origin_allowed: true,
            })
            .expect("snapshot authenticated");
        assert_eq!(snap2["authenticated"], true);
        assert_eq!(snap2["browser"], true);
        assert_eq!(snap2["csrfToken"], csrf_token);

        // 6. Logout
        let logout_res = manager
            .mutate(&AuthSessionWriteInput::Logout {
                session_cookie: Some(session_token.clone()),
            })
            .expect("logout successful");
        assert_eq!(logout_res.data["authenticated"], false);
        assert!(!manager.is_session_valid(&session_token));
    }

    #[test]
    fn auth_manager_rate_limits_after_max_attempts() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let hash = jftrade_settings::SystemSecurityPasswords
            .hash("my-secret")
            .expect("hash password");

        let store = Arc::new(MockSecurityStore(RwLock::new(Some(
            SecuritySettingsRecord::new(true, false, 3000, hash),
        ))));
        let security = SecuritySettingsService::new(store);
        let manager =
            ProductionAuthSessionManager::open(security, &directory.path().join("settings.json"))
                .expect("open auth manager");

        for _ in 0..5 {
            let _ = manager.mutate(&AuthSessionWriteInput::Login {
                password: "bad".to_owned(),
            });
        }

        let rate_limited = manager.mutate(&AuthSessionWriteInput::Login {
            password: "bad".to_owned(),
        });
        assert!(matches!(
            rate_limited,
            Err(AuthSessionWritePortError::RateLimited { .. })
        ));
    }

    #[test]
    fn auth_sessions_are_cookie_bound_hashed_and_restart_durable() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let hash = jftrade_settings::SystemSecurityPasswords
            .hash("restart-secret")
            .expect("hash password");
        let store = Arc::new(MockSecurityStore(RwLock::new(Some(
            SecuritySettingsRecord::new(true, false, 3000, hash),
        ))));
        let security = SecuritySettingsService::new(store);
        let manager = ProductionAuthSessionManager::open(security.clone(), &settings_path)
            .expect("open manager");
        let first = manager
            .mutate(&AuthSessionWriteInput::Login {
                password: "restart-secret".to_owned(),
            })
            .expect("first login");
        let second = manager
            .mutate(&AuthSessionWriteInput::Login {
                password: "restart-secret".to_owned(),
            })
            .expect("second login");
        let first_token = cookie_token(first.set_cookie.as_deref().expect("first cookie"));
        let second_token = cookie_token(second.set_cookie.as_deref().expect("second cookie"));
        let first_csrf = first.data["csrfToken"].as_str().expect("first csrf");
        let second_csrf = second.data["csrfToken"].as_str().expect("second csrf");
        assert_ne!(first_csrf, second_csrf);

        let persisted = fs::read_to_string(directory.path().join(SESSION_STORE_FILENAME))
            .expect("read session store");
        assert!(!persisted.contains(&first_token));
        assert!(!persisted.contains(first_csrf));
        drop(manager);

        let restarted =
            ProductionAuthSessionManager::open(security, &settings_path).expect("restart manager");
        assert!(restarted.is_session_valid(&first_token));
        assert!(restarted.is_csrf_valid(&first_token, first_csrf));
        let snapshot = restarted
            .session(AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: true,
                session_cookie: Some(second_token),
                origin_provided: true,
                origin_allowed: true,
            })
            .expect("cookie-bound snapshot");
        assert_eq!(snapshot["csrfToken"], second_csrf);
        restarted.invalidate_all().expect("invalidate sessions");
        assert!(!restarted.is_session_valid(&first_token));
    }

    #[test]
    fn auth_session_store_corruption_fails_closed_without_rewrite() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let session_path = directory.path().join(SESSION_STORE_FILENAME);
        fs::write(&session_path, b"{").expect("seed corrupt store");
        let before = fs::read(&session_path).expect("read corrupt bytes");
        let store = Arc::new(MockSecurityStore(RwLock::new(None)));
        let security = SecuritySettingsService::new(store);
        let error =
            ProductionAuthSessionManager::open(security, &directory.path().join("settings.json"))
                .expect_err("corrupt session store must fail closed");
        assert!(error.contains("decode Web session store"));
        assert_eq!(fs::read(session_path).expect("read original bytes"), before);
    }

    fn cookie_token(cookie: &str) -> String {
        cookie
            .split(';')
            .next()
            .and_then(|value| value.strip_prefix("jftrade_web_session="))
            .expect("session cookie token")
            .to_owned()
    }
}
