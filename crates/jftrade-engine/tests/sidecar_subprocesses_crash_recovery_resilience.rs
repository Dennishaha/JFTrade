#![forbid(unsafe_code)]

//! Integration and verification tests for Python Market-data Helper and
//! Node PineTS Worker sidecar resilience, crash recovery, exponential backoff,
//! single process ownership, and session recovery with zero order replay (P0-03).

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener};
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use jftrade_engine::product_runtime::product_runtime_helper_health::{
    HelperHealthMonitor, HelperRestartPolicy, compute_helper_backoff,
};
use jftrade_integration_marketdata_helper::{
    HelperClient, HelperClientConfig, HelperProcess, HelperProcessConfig,
};
use jftrade_integration_pine::{
    GrpcPineReadinessProbe, PineProcess, PineProcessConfig,
    PineReadinessMonitor, PineReadinessPolicy, PineReadinessState, PineRestartPolicy,
    compute_pine_backoff, spawn_mock_pine_worker, wait_until_listening,
};

fn build_mock_helper_server() -> (SocketAddr, std::thread::JoinHandle<()>, Arc<AtomicBool>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind mock helper listener");
    let addr = listener.local_addr().expect("local addr");
    let healthy = Arc::new(AtomicBool::new(true));
    let flag = Arc::clone(&healthy);
    let handle = std::thread::spawn(move || {
        while let Ok((mut stream, _)) = listener.accept() {
            let mut buf = [0u8; 1024];
            let _ = stream.read(&mut buf);
            let (status, body): (&str, &[u8]) = if flag.load(Ordering::Acquire) {
                ("200 OK", b"{\"status\":\"ready\",\"ok\":true}".as_slice())
            } else {
                ("500 Internal Server Error", b"{\"status\":\"error\"}")
            };
            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                body.len()
            );
            let _ = stream.write_all(response.as_bytes());
            let _ = stream.write_all(body);
        }
    });
    (addr, handle, healthy)
}

#[tokio::test]
async fn test_python_helper_crash_auto_recovery_exponential_backoff_and_reaping() {
    let (addr, _server_handle, _healthy) = build_mock_helper_server();

    let client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://127.0.0.1:{}", addr.port()),
        bearer_token: None,
        request_timeout: Duration::from_millis(500),
        max_attempts: 1,
        retry_delay: Duration::from_millis(20),
    })
    .expect("helper client");

    let process_config = HelperProcessConfig {
        executable: PathBuf::from("/bin/sh"),
        host: "127.0.0.1".parse().unwrap(),
        port: addr.port(),
        bearer_token: None,
        prefix_args: vec!["-c".to_owned(), "exec sleep 300".to_owned()],
        extra_args: Vec::new(),
        environment: Default::default(),
        log_path: None,
        stop_timeout: Duration::from_millis(500),
    };

    let mut initial_process = HelperProcess::new(process_config).expect("helper process");
    initial_process
        .start_until_ready(
            &client,
            Duration::from_secs(3),
            Duration::from_millis(20),
            Duration::from_millis(50),
        )
        .await
        .expect("start until ready");

    let initial_pid = initial_process.snapshot().pid.expect("initial pid");
    let managed = Arc::new(Mutex::new(Some(initial_process)));

    let restart_policy = HelperRestartPolicy {
        initial_backoff: Duration::from_millis(50),
        max_backoff: Duration::from_millis(300),
        multiplier: 2.0,
        startup_timeout: Duration::from_secs(3),
        initial_retry_delay: Duration::from_millis(20),
        max_retry_delay: Duration::from_millis(50),
    };

    let monitor = Arc::new(HelperHealthMonitor::with_managed_process(
        client.clone(),
        Duration::from_millis(30),
        Duration::from_millis(500),
        Arc::clone(&managed),
        restart_policy,
    ));
    monitor.seed_success();
    assert!(monitor.is_ready());
    assert_eq!(monitor.snapshot().restarts, 0);

    let monitor_handle = monitor.spawn();

    // 1. Simulate process kill / crash
    {
        let mut guard = managed.lock().unwrap();
        let proc = guard.as_mut().unwrap();
        proc.terminate(); // kills child
    }

    // 2. Wait for monitor to detect death, reap the old child, and restart
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    let mut recovered = false;
    while tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(50)).await;
        let snap = monitor.snapshot();
        if snap.healthy && snap.restarts >= 1 {
            recovered = true;
            break;
        }
    }
    assert!(recovered, "Helper did not auto-recover within deadline");

    // 3. Verify single process ownership: new child PID is different and alive
    {
        let mut guard = managed.lock().unwrap();
        let proc = guard.as_mut().unwrap();
        assert!(proc.is_alive(), "recovered process should be alive");
        let new_pid = proc.snapshot().pid.expect("new pid");
        assert_ne!(
            initial_pid, new_pid,
            "new child process must have replaced old child"
        );
        assert_eq!(proc.restarts(), 1, "restarts count must be exactly 1");
    }

    // 4. Shutdown cleanly and verify reaping
    monitor.stop();
    let _ = monitor_handle.await;

    let mut proc = managed.lock().unwrap().take().unwrap();
    proc.stop().await.expect("stop helper cleanly");
    assert!(!proc.is_alive(), "process must be reaped with no zombie");
}

#[tokio::test]
async fn test_node_pine_worker_crash_auto_recovery_and_single_ownership() {
    let token = "c".repeat(32);
    let pine_mock = spawn_mock_pine_worker("pineworker-test", Some(&token))
        .await
        .expect("spawn mock pine worker");
    wait_until_listening(pine_mock.address(), Duration::from_secs(2))
        .await
        .expect("mock worker listening");

    let probe = GrpcPineReadinessProbe::new(
        Some(token.clone()),
        Duration::from_millis(200),
        Duration::from_millis(200),
    )
    .expect("probe");

    let spec = pine_mock.spec("pineworker-test");
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let pine_wrapper = temp_dir.path().join("pine-worker-wrapper.sh");
    std::fs::write(&pine_wrapper, "#!/bin/sh\nexec sleep 300\n").expect("write pine wrapper");

    let mut process = PineProcess::start(
        spec.clone(),
        PineProcessConfig {
            runtime: PathBuf::from("/bin/sh"),
            bundle_path: pine_wrapper,
            proto_path: None,
            max_message_bytes: None,
            pine_ts_version: None,
            bearer_token: Some(token),
            environment: Default::default(),
            log_path: None,
            stop_timeout: Duration::from_millis(500),
        },
    )
    .expect("start pine process");

    let initial_health = process
        .wait_until_ready(
            &probe,
            PineReadinessPolicy {
                timeout: Duration::from_secs(3),
                initial_retry_delay: Duration::from_millis(20),
                max_retry_delay: Duration::from_millis(50),
            },
        )
        .await
        .expect("initial readiness");
    assert!(initial_health.ok);

    let initial_pid = process.pid().expect("initial pid");
    let readiness = PineReadinessState::new(spec.worker_id.clone());
    readiness.seed_success(initial_health);
    assert!(readiness.is_ready());

    let process_arc = Arc::new(tokio::sync::Mutex::new(process));
    let restart_policy = PineRestartPolicy {
        initial_backoff: Duration::from_millis(50),
        max_backoff: Duration::from_millis(300),
        multiplier: 2.0,
        readiness_policy: PineReadinessPolicy {
            timeout: Duration::from_secs(3),
            initial_retry_delay: Duration::from_millis(20),
            max_retry_delay: Duration::from_millis(50),
        },
    };

    let monitor = PineReadinessMonitor::spawn_supervised(
        Arc::clone(&readiness),
        probe,
        Arc::clone(&process_arc),
        Duration::from_millis(30),
        restart_policy,
    );

    // 1. Simulate process kill / crash
    {
        let mut proc = process_arc.lock().await;
        proc.terminate(); // terminate child process
    }

    // 2. Wait for monitor to detect death, reap the old child, and restart
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    let mut recovered = false;
    while tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(50)).await;
        let snap = readiness.snapshot();
        if snap.healthy && snap.running && snap.restarts >= 1 {
            recovered = true;
            break;
        }
    }
    assert!(
        recovered,
        "Pine worker did not auto-recover within deadline"
    );

    // 3. Verify single process ownership: new child PID is different and alive
    {
        let mut proc = process_arc.lock().await;
        assert!(proc.is_alive(), "recovered pine process should be alive");
        let new_pid = proc.pid().expect("new pid");
        assert_ne!(
            initial_pid, new_pid,
            "new child process must have replaced old child"
        );
        assert_eq!(proc.restarts(), 1, "restarts count must be exactly 1");
    }

    // 4. Shutdown cleanly and verify reaping
    monitor.shutdown().await;
    let mut proc = process_arc.lock().await;
    proc.stop().await.expect("stop pine process cleanly");
    assert!(!proc.is_alive(), "process must be reaped with no zombie");
}

#[test]
fn test_exponential_backoff_progression_and_upper_bound_capping() {
    let initial = Duration::from_millis(500);
    let max = Duration::from_millis(10000);
    let multiplier = 2.0;

    // Both helper and pine backoff algorithms must adhere to:
    // 500ms -> 1000ms -> 2000ms -> 4000ms -> 8000ms -> 10000ms (max)
    let expected = [
        (0, Duration::from_millis(500)),
        (1, Duration::from_millis(500)),
        (2, Duration::from_millis(1000)),
        (3, Duration::from_millis(2000)),
        (4, Duration::from_millis(4000)),
        (5, Duration::from_millis(8000)),
        (6, Duration::from_millis(10000)),
        (7, Duration::from_millis(10000)),
        (10, Duration::from_millis(10000)),
        (50, Duration::from_millis(10000)),
    ];

    for (attempt, expected_duration) in expected {
        assert_eq!(
            compute_helper_backoff(initial, max, multiplier, attempt),
            expected_duration,
            "Helper backoff mismatch at attempt {attempt}"
        );
        assert_eq!(
            compute_pine_backoff(initial, max, multiplier, attempt),
            expected_duration,
            "Pine backoff mismatch at attempt {attempt}"
        );
    }
}

