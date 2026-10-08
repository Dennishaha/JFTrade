use super::*;

#[derive(Debug)]
struct AcceptingSessionPassword;

impl SecurityPasswordPort for AcceptingSessionPassword {
    fn hash(&self, _: &str) -> Result<String, String> {
        Ok("test-session-verifier".to_owned())
    }
    fn verify(&self, _: &str, _: &str) -> bool {
        true
    }
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:235 TestWebAuthRemainingSessionAndPruningErrors
#[test]
fn auth_login_failed_persistence_preserves_sessions_before_capacity_eviction() {
    assert_failed_login_preserves_sessions(false);
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:235 TestWebAuthRemainingSessionAndPruningErrors
#[test]
fn auth_login_failed_persistence_preserves_sessions_before_expired_pruning() {
    assert_failed_login_preserves_sessions(true);
}

fn assert_failed_login_preserves_sessions(expired_entry: bool) {
    let directory = tempfile::tempdir().unwrap();
    let settings = directory.path().join("settings.json");
    let path = directory.path().join(SESSION_STORE_FILENAME);
    let store = Arc::new(MockSecurityStore(RwLock::new(Some(
        SecuritySettingsRecord::new(true, false, 3000, "test-session-verifier".to_owned()),
    ))));
    let security =
        SecuritySettingsService::with_ports(store, None, Arc::new(AcceptingSessionPassword));
    let manager = ProductionAuthSessionManager::open(security.clone(), &settings).unwrap();
    {
        let mut sessions = manager.sessions.write().unwrap();
        let now = unix_timestamp();
        for index in 0..MAX_SESSIONS {
            let hash = token_hash(&format!("retained-session-{index}"));
            sessions.insert(
                hash.clone(),
                StoredSession {
                    token_hash: hash,
                    csrf_hash: token_hash(&format!("csrf-{index}")),
                    expires_at_unix: if expired_entry && index == MAX_SESSIONS - 1 {
                        now - 1
                    } else {
                        now + 600 + index as i64 * 60
                    },
                },
            );
        }
    }
    manager.persist().unwrap();
    let before_memory = serde_json::to_value(&*manager.sessions.read().unwrap()).unwrap();
    let before_bytes = fs::read(&path).unwrap();
    let backup = directory.path().join("sessions-backup.json");
    fs::rename(&path, &backup).unwrap();
    fs::create_dir(&path).unwrap();
    let result = manager.mutate(&AuthSessionWriteInput::Login {
        password: "correct fixture password".to_owned(),
    });
    assert!(
        matches!(result, Err(AuthSessionWritePortError::Unavailable(_))),
        "{result:?}"
    );
    let after_memory = serde_json::to_value(&*manager.sessions.read().unwrap()).unwrap();
    assert!(
        after_memory == before_memory,
        "failed persistence changed sessions: before={} after={} expired={expired_entry}",
        before_memory.as_object().unwrap().len(),
        after_memory.as_object().unwrap().len(),
    );
    assert!(manager.is_session_valid("retained-session-0"));
    assert!(manager.is_csrf_valid("retained-session-0", "csrf-0"));
    assert_eq!(fs::read(&backup).unwrap(), before_bytes);
    fs::remove_dir(&path).unwrap();
    fs::rename(&backup, &path).unwrap();
    drop(manager);
    let restarted = ProductionAuthSessionManager::open(security, &settings).unwrap();
    assert!(restarted.is_session_valid("retained-session-0"));
    let successful = restarted
        .mutate(&AuthSessionWriteInput::Login {
            password: "correct fixture password".to_owned(),
        })
        .unwrap();
    assert_eq!(successful.data["authenticated"], true);
    let cookie = successful.set_cookie.unwrap();
    let new_token = cookie
        .split(';')
        .next()
        .unwrap()
        .strip_prefix("jftrade_web_session=")
        .unwrap();
    assert!(restarted.is_session_valid(new_token));
    assert!(restarted.is_csrf_valid(new_token, successful.data["csrfToken"].as_str().unwrap()));
    assert_eq!(restarted.sessions.read().unwrap().len(), MAX_SESSIONS);
    assert_eq!(
        restarted.is_session_valid("retained-session-0"),
        expired_entry
    );
    let persisted = load_sessions(&path).unwrap();
    assert_eq!(persisted.len(), MAX_SESSIONS);
    assert_eq!(
        persisted.contains_key(&token_hash("retained-session-0")),
        expired_entry
    );
    assert!(persisted.contains_key(&token_hash(new_token)));
    let persisted_value = serde_json::to_value(&persisted).unwrap();
    assert_eq!(
        persisted_value,
        serde_json::to_value(&*restarted.sessions.read().unwrap()).unwrap(),
        "durable table must match every published session field"
    );
    let removed_index = if expired_entry { MAX_SESSIONS - 1 } else { 0 };
    for index in 0..MAX_SESSIONS {
        let hash = token_hash(&format!("retained-session-{index}"));
        if index == removed_index {
            assert!(!persisted.contains_key(&hash));
        } else {
            assert_eq!(persisted_value[&hash], before_memory[&hash]);
        }
    }
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:235 TestWebAuthRemainingSessionAndPruningErrors
#[test]
fn auth_rate_limit_removes_attempts_at_the_expiry_boundary() {
    let directory = tempfile::tempdir().unwrap();
    let security = SecuritySettingsService::new(Arc::new(MockSecurityStore(RwLock::new(None))));
    let manager =
        ProductionAuthSessionManager::open(security, &directory.path().join("settings.json"))
            .unwrap();
    {
        let mut attempts = manager.failed_attempts.lock().unwrap();
        attempts.insert(
            "expired".to_owned(),
            LoginAttempt {
                failures: MAX_FAILED_ATTEMPTS,
                window_start: Instant::now() - RATE_LIMIT_WINDOW,
            },
        );
        attempts.insert(
            "active".to_owned(),
            LoginAttempt {
                failures: MAX_FAILED_ATTEMPTS,
                window_start: Instant::now(),
            },
        );
    }
    let expired = AuthSessionRequestContext {
        client_key: "expired".to_owned(),
        secure: false,
    };
    assert!(manager.login_rate_limit_with_context(&expired).is_none());
    assert!(
        !manager
            .failed_attempts
            .lock()
            .unwrap()
            .contains_key("expired")
    );
    let active = AuthSessionRequestContext {
        client_key: "active".to_owned(),
        secure: false,
    };
    assert!(matches!(
        manager.login_rate_limit_with_context(&active),
        Some(AuthSessionWritePortError::RateLimited { .. })
    ));
    assert_eq!(manager.failed_attempts.lock().unwrap().len(), 1);
}
