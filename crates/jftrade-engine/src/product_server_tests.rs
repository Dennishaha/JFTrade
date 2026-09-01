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
