use std::sync::RwLock;
use std::sync::mpsc::{Receiver, Sender, channel};

use jftrade_settings::{
    SecurityPasswordPort, SecuritySettingsRecord, SecuritySettingsStorePort, SettingsStoreError,
};

use super::*;

#[path = "product_auth_session_persistence_tests.rs"]
mod persistence_tests;

struct MockSecurityStore(RwLock<Option<SecuritySettingsRecord>>);

impl SecuritySettingsStorePort for MockSecurityStore {
    fn load_security_record(&self) -> Result<Option<SecuritySettingsRecord>, SettingsStoreError> {
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

struct BlockingPasswordVerifier {
    started: Mutex<Option<Sender<()>>>,
    release: Mutex<Receiver<()>>,
    hash_verifiers: Mutex<Vec<String>>,
}

impl jftrade_settings::SecurityPasswordPort for BlockingPasswordVerifier {
    fn hash(&self, _password: &str) -> Result<String, String> {
        Ok(self
            .hash_verifiers
            .lock()
            .expect("hash verifier lock")
            .pop()
            .unwrap_or_else(|| "replacement-verifier".to_owned()))
    }

    fn verify(&self, _password_hash: &str, _password: &str) -> bool {
        if let Some(started) = self.started.lock().expect("started lock").take() {
            started.send(()).expect("signal verification start");
        }
        self.release
            .lock()
            .expect("release lock")
            .recv()
            .expect("release verification");
        true
    }
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:379 TestWebLogoutClearsSessionCookie
// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:400 TestWebLoginCookieIsHttpOnlyAndSameSiteStrict
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
    // Parity: internal/app/apiserver/webaccess/security_integration_test.go:400 TestWebLoginCookieIsHttpOnlyAndSameSiteStrict
    assert!(cookie_header.contains("HttpOnly"));
    assert!(cookie_header.contains("SameSite=Strict"));
    assert!(cookie_header.contains("Path=/"));
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
    // Parity: internal/app/apiserver/webaccess/security_integration_test.go:379 TestWebLogoutClearsSessionCookie
    let clear_cookie = logout_res
        .set_cookie
        .expect("logout must provide expired clear cookie");
    assert!(clear_cookie.starts_with("jftrade_web_session=;"));
    assert!(clear_cookie.contains("Max-Age=0"));
    assert!(clear_cookie.contains("HttpOnly"));
    assert!(clear_cookie.contains("SameSite=Strict"));
    assert!(clear_cookie.contains("Path=/"));
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:335 TestWebLoginRejectsWrongPasswordAndRateLimits
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

    for _ in 0..MAX_FAILED_ATTEMPTS {
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

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:295 TestWebAuthStateMapsStayBounded
#[test]
fn auth_state_maps_stay_bounded_and_evict_oldest_entries() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let hash = jftrade_settings::SystemSecurityPasswords
        .hash("bounded-state-secret")
        .expect("hash password");
    let store = Arc::new(MockSecurityStore(RwLock::new(Some(
        SecuritySettingsRecord::new(true, false, 3000, hash),
    ))));
    let security = SecuritySettingsService::new(store);
    let manager =
        ProductionAuthSessionManager::open(security, &directory.path().join("settings.json"))
            .expect("open auth manager");

    for index in 0..(MAX_LOGIN_ATTEMPTS + 20) {
        manager.record_login_failure(&format!("192.0.2.{index}"));
    }
    let attempts = manager
        .failed_attempts
        .lock()
        .expect("failed attempts lock");
    assert_eq!(attempts.len(), MAX_LOGIN_ATTEMPTS);
    assert!(
        !attempts.contains_key("192.0.2.0"),
        "oldest login attempt was not evicted"
    );
    assert!(attempts.contains_key("192.0.2.1043"));
    drop(attempts);

    let oldest_session_hash = token_hash("seed-session-0");
    let newest_session_hash = token_hash(&format!("seed-session-{}", MAX_SESSIONS - 1));
    {
        let mut sessions = manager.sessions.write().expect("session lock");
        let now = unix_timestamp();
        for index in 0..(MAX_SESSIONS + 20) {
            let token = format!("seed-session-{index}");
            let token_hash_value = token_hash(&token);
            sessions.insert(
                token_hash_value.clone(),
                StoredSession {
                    token_hash: token_hash_value,
                    csrf_hash: token_hash(&format!("seed-csrf-{index}")),
                    expires_at_unix: now + i64::try_from(index + 1).unwrap() * 60,
                },
            );
        }
    }

    let login = manager
        .mutate(&AuthSessionWriteInput::Login {
            password: "bounded-state-secret".to_owned(),
        })
        .expect("login after session map reaches its bound");
    assert!(login.set_cookie.is_some());
    let sessions = manager.sessions.read().expect("session lock");
    assert_eq!(sessions.len(), MAX_SESSIONS);
    assert!(
        !sessions.contains_key(&oldest_session_hash),
        "oldest session was not evicted"
    );
    assert!(
        sessions.contains_key(&newest_session_hash),
        "newest retained seed session was unexpectedly evicted"
    );
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:195 TestPasswordChangeDuringLoginCannotCreateOldPasswordSession
#[test]
fn password_change_during_login_rejects_the_stale_verification() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = Arc::new(MockSecurityStore(RwLock::new(Some(
        SecuritySettingsRecord::new(true, false, 3000, "old-verifier"),
    ))));
    let (started_tx, started_rx) = channel();
    let (release_tx, release_rx) = channel();
    let passwords = Arc::new(BlockingPasswordVerifier {
        started: Mutex::new(Some(started_tx)),
        release: Mutex::new(release_rx),
        hash_verifiers: Mutex::new(vec!["replacement-verifier".to_owned()]),
    });
    let security = SecuritySettingsService::with_ports(store, None, passwords);
    let manager = ProductionAuthSessionManager::open(
        security.clone(),
        &directory.path().join("settings.json"),
    )
    .expect("open auth manager");

    let login_manager = manager.clone();
    let login = std::thread::spawn(move || {
        login_manager.mutate(&AuthSessionWriteInput::Login {
            password: "old-password".to_owned(),
        })
    });
    started_rx
        .recv_timeout(Duration::from_secs(5))
        .expect("password verification started");

    security
        .save(&jftrade_settings::SecuritySettingsUpdate {
            web_access_enabled: true,
            new_password: "replacement password".to_owned(),
            ..Default::default()
        })
        .expect("replace password");
    release_tx.send(()).expect("release password verification");

    let result = login.join().expect("login thread");
    assert!(
        matches!(
            result,
            Err(AuthSessionWritePortError::ConfigurationChanged(_))
        ),
        "login result = {result:?}"
    );
    assert!(manager.sessions.read().expect("session lock").is_empty());
}

#[test]
fn password_restore_during_login_rejects_aba_stale_verification() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = Arc::new(MockSecurityStore(RwLock::new(Some(
        SecuritySettingsRecord::new(true, false, 3000, "old-verifier"),
    ))));
    let (started_tx, started_rx) = channel();
    let (release_tx, release_rx) = channel();
    let passwords = Arc::new(BlockingPasswordVerifier {
        started: Mutex::new(Some(started_tx)),
        release: Mutex::new(release_rx),
        // `save` pops from the end: first change to replacement, then
        // restore the exact original verifier to exercise the ABA case.
        hash_verifiers: Mutex::new(vec![
            "old-verifier".to_owned(),
            "replacement-verifier".to_owned(),
        ]),
    });
    let security = SecuritySettingsService::with_ports(store, None, passwords);
    let manager = ProductionAuthSessionManager::open(
        security.clone(),
        &directory.path().join("settings.json"),
    )
    .expect("open auth manager");

    let login_manager = manager.clone();
    let login = std::thread::spawn(move || {
        login_manager.mutate(&AuthSessionWriteInput::Login {
            password: "old-password".to_owned(),
        })
    });
    started_rx
        .recv_timeout(Duration::from_secs(5))
        .expect("password verification started");

    security
        .save(&jftrade_settings::SecuritySettingsUpdate {
            web_access_enabled: true,
            new_password: "replacement password".to_owned(),
            ..Default::default()
        })
        .expect("replace password");
    security
        .save(&jftrade_settings::SecuritySettingsUpdate {
            web_access_enabled: true,
            new_password: "restored password".to_owned(),
            ..Default::default()
        })
        .expect("restore original verifier");
    release_tx.send(()).expect("release password verification");

    let result = login.join().expect("login thread");
    assert!(
        matches!(
            result,
            Err(AuthSessionWritePortError::ConfigurationChanged(_))
        ),
        "login result = {result:?}"
    );
    assert!(manager.sessions.read().expect("session lock").is_empty());
}

// Parity: go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:116 TestDisablingWebImmediatelyInvalidatesBrowserButNotDesktop; go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:352 TestPasswordChangesInvalidateWebSessions
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
    let manager =
        ProductionAuthSessionManager::open(security.clone(), &settings_path).expect("open manager");
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
    // Parity: internal/app/apiserver/webaccess/security_integration_test.go:352 TestPasswordChangesInvalidateWebSessions
    restarted.invalidate_all().expect("invalidate sessions");
    assert!(!restarted.is_session_valid(&first_token));
    let invalidated_snapshot = restarted
        .session(AuthSessionSnapshotRequest {
            desktop_trusted: false,
            browser_authenticated: true,
            session_cookie: Some(first_token.clone()),
            origin_provided: true,
            origin_allowed: true,
        })
        .expect("invalidated snapshot");
    assert_eq!(invalidated_snapshot["authenticated"], false);
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

#[test]
fn session_revocation_is_published_only_after_persistence_succeeds() {
    let directory = tempfile::tempdir().expect("directory");
    let session_path = directory.path().join(SESSION_STORE_FILENAME);
    let security = SecuritySettingsService::new(Arc::new(MockSecurityStore(RwLock::new(None))));
    let manager =
        ProductionAuthSessionManager::open_at(security, session_path.clone()).expect("manager");
    let session = StoredSession {
        token_hash: token_hash("existing-session"),
        csrf_hash: token_hash("csrf"),
        expires_at_unix: unix_timestamp() + 100,
    };
    manager
        .sessions
        .write()
        .expect("sessions")
        .insert(session.token_hash.clone(), session);
    manager.persist().expect("persist session");
    let mut subscribed = manager.subscribe_revocation().expect("subscription");
    let backup = directory.path().join("sessions-backup.json");
    fs::rename(&session_path, &backup).expect("backup session file");
    fs::create_dir(&session_path).expect("block atomic rename");
    assert!(manager.invalidate_all().is_err());
    assert!(manager.is_session_valid("existing-session"));
    assert!(!subscribed.has_changed().expect("revocation state"));

    fs::remove_dir(&session_path).expect("unblock persistence");
    fs::rename(backup, session_path).expect("restore session file");
    manager.invalidate_all().expect("invalidate sessions");
    assert!(!manager.is_session_valid("existing-session"));
    assert!(subscribed.has_changed().expect("successful revocation"));
    subscribed.borrow_and_update();
    let fresh = manager.subscribe_revocation().expect("new subscription");
    assert!(!fresh.has_changed().expect("new generation"));
    manager.invalidate_all().expect("next security change");
    assert!(fresh.has_changed().expect("new subscriber revoked"));
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:101 TestWebAuthRemainingAuthenticationStates
#[test]
fn expired_browser_sessions_fail_validation_and_are_pruned_on_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let store = Arc::new(MockSecurityStore(RwLock::new(Some(
        SecuritySettingsRecord::new(true, true, 3000, "configured-verifier"),
    ))));
    let security = SecuritySettingsService::new(store);
    let manager = ProductionAuthSessionManager::open(security.clone(), &settings_path)
        .expect("open session manager");
    let now = unix_timestamp();
    {
        let mut sessions = manager.sessions.write().expect("sessions");
        for (token, expires_at_unix) in [("expired", now), ("active", now + 3600)] {
            sessions.insert(
                token_hash(token),
                StoredSession {
                    token_hash: token_hash(token),
                    csrf_hash: token_hash("csrf"),
                    expires_at_unix,
                },
            );
        }
    }
    assert!(!manager.is_session_valid("expired"));
    assert!(!manager.is_csrf_valid("expired", "csrf"));
    assert!(manager.is_session_valid("active"));
    assert!(manager.is_csrf_valid("active", "csrf"));
    let snapshot = manager
        .session(AuthSessionSnapshotRequest {
            desktop_trusted: false,
            browser_authenticated: true,
            session_cookie: Some("expired".to_owned()),
            origin_provided: false,
            origin_allowed: false,
        })
        .expect("expired snapshot");
    assert_eq!(snapshot["authenticated"], false);
    assert_eq!(snapshot["csrfToken"], Value::Null);
    assert_eq!(snapshot["expiresAt"], Value::Null);
    manager.persist().expect("persist expiration fixture");
    drop(manager);

    let restarted = ProductionAuthSessionManager::open(security, &settings_path)
        .expect("restart session manager");
    assert!(!restarted.is_session_valid("expired"));
    assert!(restarted.is_session_valid("active"));
    let document: StoredSessionDocument = serde_json::from_slice(
        &fs::read(directory.path().join(SESSION_STORE_FILENAME)).expect("session bytes"),
    )
    .expect("persisted session document");
    assert_eq!(document.sessions.len(), 1);
    assert_eq!(document.sessions[0].token_hash, token_hash("active"));
}

fn cookie_token(cookie: &str) -> String {
    cookie
        .split(';')
        .next()
        .and_then(|value| value.strip_prefix("jftrade_web_session="))
        .expect("session cookie token")
        .to_owned()
}
