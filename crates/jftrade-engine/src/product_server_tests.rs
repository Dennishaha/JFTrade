use super::*;
use std::io::Read;
use std::net::TcpStream;
use std::sync::mpsc;

fn available_port() -> u16 {
    let listener = StdTcpListener::bind("127.0.0.1:0").expect("reserve test port");
    listener.local_addr().expect("test port address").port()
}

fn enabled_record(port: u16) -> SecuritySettingsRecord {
    SecuritySettingsRecord::new(true, false, port, "fixture-verifier")
}

fn router() -> axum::Router {
    axum::Router::new().fallback(|| async { "ok" })
}

fn web_response(port: u16) -> String {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("Web connection");
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .expect("read deadline");
    std::io::Write::write_all(
        &mut stream,
        b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n",
    )
    .expect("request");
    let mut response = String::new();
    stream.read_to_string(&mut response).expect("response");
    response
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:222 TestWebAccessListenerBindUsesIndependentConfiguredPort
#[test]
fn web_bind_matrix_preserves_independent_local_and_public_ports() {
    assert_eq!(
        ProductWebServerRuntime::desired_bind(&SecuritySettingsRecord::default()),
        None
    );
    assert_eq!(
        ProductWebServerRuntime::desired_bind(&enabled_record(7443)),
        Some("127.0.0.1:7443".to_owned())
    );
    let public = SecuritySettingsRecord::new(true, true, 7443, "fixture-verifier");
    assert_eq!(
        ProductWebServerRuntime::desired_bind(&public),
        Some("0.0.0.0:7443".to_owned())
    );
    // Rust rejects a public desktop API bind rather than coercing it as the
    // frozen Go helper does. This row must remain partial for that input.
    assert!(matches!(
        ProductConfig::new(
            "0.0.0.0:6699".parse().expect("bind"),
            "settings.json",
            AccessPolicy::default()
        ),
        Err(ProductError::NonLoopbackBind)
    ));
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
#[test]
fn web_listener_hot_rebind_preserves_service_and_rolls_back_port_conflict() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    let first_port = available_port();
    runtime
        .apply(&enabled_record(first_port))
        .expect("first bind");
    assert!(web_response(first_port).starts_with("HTTP/1.1 200"));
    let second_port = available_port();
    assert_ne!(first_port, second_port);
    let second = enabled_record(second_port);
    runtime.apply(&second).expect("hot rebind");
    assert!(web_response(second_port).starts_with("HTTP/1.1 200"));
    assert!(runtime.status(&second).expect("new status"));
    assert!(
        StdTcpListener::bind(("127.0.0.1", first_port)).is_ok(),
        "old port released"
    );
    let occupied = StdTcpListener::bind("127.0.0.1:0").expect("occupy conflict port");
    let conflict_port = occupied.local_addr().expect("conflict address").port();
    let error = runtime
        .apply(&enabled_record(conflict_port))
        .expect_err("conflict");
    assert!(error.contains("Web access port conflict"), "{error}");
    assert!(web_response(second_port).starts_with("HTTP/1.1 200"));
    assert!(runtime.status(&second).expect("surviving status"));
    runtime.shutdown_blocking().expect("shutdown");
}

#[test]
fn disabled_web_access_does_not_start_a_listener() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    runtime
        .apply(&SecuritySettingsRecord::default())
        .expect("disable Web access");
    assert!(
        !runtime
            .status(&SecuritySettingsRecord::default())
            .expect("Web status")
    );
    runtime.shutdown_blocking().expect("shutdown Web runtime");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:188 TestBrowserNavigationGetsFriendlyDisabledWebPage
#[test]
fn web_runtime_reports_browser_access_state_for_the_desktop_listener() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    assert_eq!(
        runtime.web_access_state(),
        jftrade_api::WebAccessState::Disabled,
        "a fresh runtime has no browser surface"
    );
    runtime
        .apply(&SecuritySettingsRecord::default())
        .expect("keep Web access disabled");
    assert_eq!(
        runtime.web_access_state(),
        jftrade_api::WebAccessState::Disabled
    );

    runtime
        .apply(&enabled_record(available_port()))
        .expect("enable Web access");
    assert_eq!(
        runtime.web_access_state(),
        jftrade_api::WebAccessState::Available
    );

    runtime
        .apply(&SecuritySettingsRecord::default())
        .expect("disable Web access");
    assert_eq!(
        runtime.web_access_state(),
        jftrade_api::WebAccessState::Disabled
    );
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:243 TestSeparateWebListenerStartsAlongsideLoopbackDesktopSidecar
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:222 TestWebAccessListenerBindUsesIndependentConfiguredPort
#[test]
fn enabled_web_access_binds_and_shutdown_releases_port() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    let port = available_port();
    let record = enabled_record(port);
    runtime.apply(&record).expect("start Web listener");
    assert!(runtime.status(&record).expect("Web status"));

    let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect Web listener");
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .expect("set read timeout");
    std::io::Write::write_all(
        &mut stream,
        b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n",
    )
    .expect("write Web request");
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .expect("read Web response");
    assert!(
        response.starts_with("HTTP/1.1 200"),
        "response = {response}"
    );

    runtime.shutdown_blocking().expect("shutdown Web runtime");
    let listener = StdTcpListener::bind(("127.0.0.1", port))
        .expect("Web listener port released after shutdown");
    drop(listener);
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:378 TestWebAccessServerManagerRestoresOldBindAfterHostSwitchFailure
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
#[test]
fn port_conflict_keeps_the_previous_listener_running() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    let first_port = available_port();
    let second_port = available_port();
    let first = enabled_record(first_port);
    runtime.apply(&first).expect("start initial Web listener");
    let occupied = StdTcpListener::bind(("127.0.0.1", second_port)).expect("occupy port");
    let second = enabled_record(second_port);
    let error = runtime.apply(&second).expect_err("conflicting Web bind");
    assert!(
        error.contains("Web access port conflict"),
        "error = {error}"
    );
    assert!(runtime.status(&first).expect("previous Web status"));
    drop(occupied);
    runtime.shutdown_blocking().expect("shutdown Web runtime");
}

// Parity: go:452dea11:internal/app/apiserver/server_test.go:85 TestStartDesktopDoesNotMutatePersistedWebAccessSettings
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:331 TestWebAccessServerManagerCoversLiveReconfigurationLifecycle
#[test]
fn dynamic_origin_allowlist_tracks_the_current_web_port() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    let first_port = available_port();
    let second_port = available_port();
    let first = enabled_record(first_port);
    runtime.apply(&first).expect("start initial Web listener");
    assert!(runtime.allows_origin(&format!("http://127.0.0.1:{first_port}")));
    assert!(!runtime.allows_origin(&format!("http://127.0.0.1:{second_port}")));

    let second = enabled_record(second_port);
    runtime.apply(&second).expect("rebind Web listener");
    assert!(!runtime.allows_origin(&format!("http://127.0.0.1:{first_port}")));
    assert!(runtime.allows_origin(&format!("http://127.0.0.1:{second_port}")));
    runtime.shutdown_blocking().expect("shutdown Web runtime");
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:331 TestWebAccessServerManagerCoversLiveReconfigurationLifecycle
#[test]
fn web_runtime_reconfigures_same_bind_public_host_and_disable_idempotently() {
    let runtime = ProductWebServerRuntime::new();
    runtime.install_router(router());
    let port = available_port();
    let local = enabled_record(port);
    runtime.apply(&local).expect("initial bind");
    runtime.apply(&local).expect("same-bind reconfigure");
    assert!(runtime.status(&local).expect("local status"));
    assert!(web_response(port).starts_with("HTTP/1.1 200"));

    let public = SecuritySettingsRecord::new(true, true, port, "fixture-verifier");
    runtime
        .apply(&public)
        .expect("same-port public host switch");
    assert!(runtime.status(&public).expect("public status"));
    assert_eq!(
        runtime.inner.lock().unwrap().bind.as_deref(),
        Some(format!("0.0.0.0:{port}").as_str())
    );
    assert!(web_response(port).starts_with("HTTP/1.1 200"));

    runtime
        .apply(&SecuritySettingsRecord::default())
        .expect("disable");
    {
        let state = runtime.inner.lock().unwrap();
        assert!(state.server.is_none());
        assert!(state.bind.is_none());
    }
    assert!(!runtime.allows_origin(&format!("http://127.0.0.1:{port}")));
    assert!(StdTcpListener::bind(("127.0.0.1", port)).is_ok());
    runtime
        .shutdown_blocking()
        .expect("shutdown disabled runtime");
    runtime.shutdown_blocking().expect("repeat shutdown");
}

#[test]
fn web_shutdown_does_not_hold_runtime_lock_while_joining_server() {
    let runtime = ProductWebServerRuntime::new();
    let (lock_result_tx, lock_result_rx) = mpsc::channel();
    let inner = Arc::clone(&runtime.inner);
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    let thread = std::thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .expect("server probe runtime");
        runtime.block_on(async {
            let _ = shutdown_rx.await;
        });
        let acquired = inner.try_lock().is_ok();
        let _ = lock_result_tx.send(acquired);
        Ok(())
    });
    {
        let mut state = runtime.inner.lock().expect("runtime state lock");
        state.bind = Some("127.0.0.1:1".to_owned());
        state.server = Some(ProductServerOwner {
            shutdown_tx: Some(shutdown_tx),
            connection_shutdown: tokio::sync::watch::channel(false).0,
            thread: Some(thread),
        });
    }

    runtime
        .shutdown_blocking()
        .expect("shutdown Web runtime without lock held");
    assert!(
        lock_result_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("server thread lock probe"),
        "server join must happen after releasing the runtime mutex"
    );
}
