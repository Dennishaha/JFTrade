#![forbid(unsafe_code)]

//! Empirical adversarial stress testing of sidecar process supervisor,
//! exponential backoff progression, boundary/overflow edge cases,
//! and process reaping under repeated SIGKILL and abnormal exits.

use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener};
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use jftrade_engine::product_runtime::product_runtime_helper_health::{
    HelperHealthMonitor, HelperRestartPolicy, compute_helper_backoff,
};
use jftrade_integration_marketdata_helper::{
    HelperClient, HelperClientConfig, HelperProcess, HelperProcessConfig,
};
use jftrade_integration_pine::{
    GrpcPineReadinessProbe, PineProcess, PineProcessConfig, PineReadinessMonitor,
    PineReadinessPolicy, PineReadinessState, PineRestartPolicy, compute_pine_backoff,
    spawn_mock_pine_worker, wait_until_listening,
};

/// Check OS process state via `ps -p <pid> -o stat=`.
/// Returns:
/// - `Some(state)` if process exists in the process table (e.g. "S", "R", "Z", "Z+")
/// - `None` if process does not exist at all in the OS process table
fn query_os_process_state(pid: u32) -> Option<String> {
    let output = std::process::Command::new("ps")
        .args(["-p", &pid.to_string(), "-o", "stat="])
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }
    let text = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if text.is_empty() { None } else { Some(text) }
}

/// Send SIGKILL (kill -9) to a process using `/bin/kill -9 <pid>`.
fn send_sigkill(pid: u32) {
    let _ = std::process::Command::new("kill")
        .args(["-9", &pid.to_string()])
        .status();
}

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

#[test]
fn stress_test_backoff_extreme_inputs_and_overflow_resistance() {
    let initial = Duration::from_millis(500);
    let max = Duration::from_millis(10000);
    let multiplier = 2.0;

    // Test attempts from 0 up to 100
    for attempt in 0..=100 {
        let h = compute_helper_backoff(initial, max, multiplier, attempt);
        let p = compute_pine_backoff(initial, max, multiplier, attempt);
        assert_eq!(
            h, p,
            "Helper and Pine backoff must match at attempt {attempt}"
        );
        assert!(h >= initial, "Backoff must be at least initial");
        assert!(h <= max, "Backoff must not exceed max");
        if attempt >= 6 {
            assert_eq!(
                h, max,
                "Backoff at attempt {attempt} must be strictly clamped to max (10000ms)"
            );
        }
    }

    // Test extreme attempts: large integers, boundary conditions
    let extreme_attempts = [1000, 1_000_000, i32::MAX as u32, u32::MAX - 1, u32::MAX];

    for &attempt in &extreme_attempts {
        let h = compute_helper_backoff(initial, max, multiplier, attempt);
        let p = compute_pine_backoff(initial, max, multiplier, attempt);
        assert_eq!(
            h, max,
            "Helper backoff at attempt {attempt} must cap at max"
        );
        assert_eq!(p, max, "Pine backoff at attempt {attempt} must cap at max");
    }

    // Test boundary initial and max configurations
    assert_eq!(
        compute_helper_backoff(Duration::ZERO, Duration::from_millis(5000), 2.0, 1),
        Duration::ZERO
    );
    assert_eq!(
        compute_helper_backoff(
            Duration::from_millis(1000),
            Duration::from_millis(1000),
            2.0,
            10
        ),
        Duration::from_millis(1000)
    );
}

#[tokio::test]
async fn stress_test_helper_repeated_sigkill_reaping_and_zombie_prevention() {
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

    let mut proc = HelperProcess::new(process_config).expect("helper process");
    proc.start_until_ready(
        &client,
        Duration::from_secs(3),
        Duration::from_millis(20),
        Duration::from_millis(50),
    )
    .await
    .expect("initial start");

    let mut killed_pids = Vec::new();

    // Adversarially perform 10 consecutive SIGKILL cycles
    for cycle in 1..=10 {
        let pid = proc.snapshot().pid.expect("process pid");
        assert!(
            proc.is_alive(),
            "Cycle {cycle}: process must be alive before SIGKILL"
        );

        // Send SIGKILL externally
        send_sigkill(pid);

        // Allow OS kernel to deliver signal
        tokio::time::sleep(Duration::from_millis(30)).await;

        // Verify that is_alive() detects termination immediately
        assert!(
            !proc.is_alive(),
            "Cycle {cycle}: is_alive() must report dead after SIGKILL"
        );

        // Restart process: restart_until_ready() internally stops and reaps the dead child before spawning
        proc.restart_until_ready(
            &client,
            Duration::from_secs(3),
            Duration::from_millis(20),
            Duration::from_millis(50),
        )
        .await
        .expect("restart until ready");

        let new_pid = proc.snapshot().pid.expect("new pid");
        assert_ne!(
            pid, new_pid,
            "Cycle {cycle}: new PID must differ from killed PID"
        );
        killed_pids.push(pid);

        // Verify that the old PID is NOT a zombie in the OS process table
        let old_state = query_os_process_state(pid);
        if let Some(ref st) = old_state {
            assert!(
                !st.starts_with('Z'),
                "Cycle {cycle}: PID {pid} was left as a zombie process! State: {st}"
            );
        }
    }

    // Clean shutdown
    proc.stop().await.expect("clean stop");

    // Re-verify all killed PIDs are non-zombies
    for (i, &old_pid) in killed_pids.iter().enumerate() {
        let state = query_os_process_state(old_pid);
        if let Some(ref st) = state {
            assert!(
                !st.starts_with('Z'),
                "Post-shutdown check {i}: PID {old_pid} is a zombie! State: {st}"
            );
        }
    }
}

#[tokio::test]
async fn stress_test_pine_worker_repeated_sigkill_reaping_and_zombie_prevention() {
    let token = "d".repeat(32);
    let pine_mock = spawn_mock_pine_worker("pineworker-stress", Some(&token))
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

    let spec = pine_mock.spec("pineworker-stress");
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let pine_wrapper = temp_dir.path().join("pine-worker-stress.sh");
    std::fs::write(&pine_wrapper, "#!/bin/sh\nexec sleep 300\n").expect("write pine wrapper");

    let mut proc = PineProcess::start(
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

    let health = proc
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
    assert!(health.ok);

    let mut killed_pids = Vec::new();

    // Adversarially perform 10 consecutive SIGKILL cycles
    for cycle in 1..=10 {
        let pid = proc.pid().expect("process pid");
        assert!(
            proc.is_alive(),
            "Cycle {cycle}: Pine process must be alive before SIGKILL"
        );

        send_sigkill(pid);
        tokio::time::sleep(Duration::from_millis(30)).await;

        assert!(
            !proc.is_alive(),
            "Cycle {cycle}: is_alive() must report dead after SIGKILL"
        );

        // Restart process
        let restart_health = proc
            .restart_until_ready(
                &probe,
                PineReadinessPolicy {
                    timeout: Duration::from_secs(3),
                    initial_retry_delay: Duration::from_millis(20),
                    max_retry_delay: Duration::from_millis(50),
                },
            )
            .await
            .expect("restart until ready");
        assert!(restart_health.ok);

        let new_pid = proc.pid().expect("new pid");
        assert_ne!(
            pid, new_pid,
            "Cycle {cycle}: new PID must differ from killed PID"
        );
        killed_pids.push(pid);

        // Verify old PID is not a zombie
        let old_state = query_os_process_state(pid);
        if let Some(ref st) = old_state {
            assert!(
                !st.starts_with('Z'),
                "Cycle {cycle}: Pine PID {pid} was left as a zombie process! State: {st}"
            );
        }
    }

    // Clean shutdown
    proc.stop().await.expect("clean stop");

    for (i, &old_pid) in killed_pids.iter().enumerate() {
        let state = query_os_process_state(old_pid);
        if let Some(ref st) = state {
            assert!(
                !st.starts_with('Z'),
                "Post-shutdown check {i}: Pine PID {old_pid} is a zombie! State: {st}"
            );
        }
    }
}

#[tokio::test]
async fn stress_test_cancellation_during_backoff_sleep_is_immediate() {
    // When supervisor is sleeping during a 10-second backoff, calling stop() or shutdown()
    // MUST NOT block for 10 seconds. It must return immediately (< 200ms).
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

    let mut proc = HelperProcess::new(process_config).expect("helper process");
    proc.start_until_ready(
        &client,
        Duration::from_secs(3),
        Duration::from_millis(20),
        Duration::from_millis(50),
    )
    .await
    .expect("initial start");

    let managed = Arc::new(Mutex::new(Some(proc)));

    // Configure a very long backoff (e.g. 10000ms)
    let restart_policy = HelperRestartPolicy {
        initial_backoff: Duration::from_millis(10000),
        max_backoff: Duration::from_millis(10000),
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

    let handle = monitor.spawn();

    // Kill process so monitor enters 10s backoff sleep
    {
        let mut guard = managed.lock().unwrap();
        guard.as_mut().unwrap().terminate();
    }

    // Wait 100ms for monitor to detect death and enter sleep_with_cancellation(10000ms)
    tokio::time::sleep(Duration::from_millis(100)).await;

    // Now issue stop(): must complete well before 10 seconds!
    let start = Instant::now();
    monitor.stop();
    let join_res = tokio::time::timeout(Duration::from_secs(1), handle).await;
    let elapsed = start.elapsed();

    assert!(
        join_res.is_ok(),
        "Monitor task failed to exit within 1 second during 10s backoff sleep"
    );
    assert!(
        elapsed < Duration::from_millis(200),
        "Cancellation took too long: {elapsed:?} (expected < 200ms)"
    );

    // Reap child
    let mut p = managed.lock().unwrap().take().unwrap();
    let _ = p.stop().await;
}

#[tokio::test]
async fn stress_test_rapid_consecutive_crash_on_startup_no_spinloop() {
    // Test a script that immediately exits with code 1 upon startup.
    // The supervisor must handle consecutive immediate crashes, increase backoff,
    // and NOT enter an infinite tight spinloop.
    let (addr, _server_handle, _healthy) = build_mock_helper_server();

    let client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://127.0.0.1:{}", addr.port()),
        bearer_token: None,
        request_timeout: Duration::from_millis(200),
        max_attempts: 1,
        retry_delay: Duration::from_millis(10),
    })
    .expect("helper client");

    // Script exits immediately with error 1
    let process_config = HelperProcessConfig {
        executable: PathBuf::from("/bin/sh"),
        host: "127.0.0.1".parse().unwrap(),
        port: addr.port(),
        bearer_token: None,
        prefix_args: vec!["-c".to_owned(), "exit 1".to_owned()],
        extra_args: Vec::new(),
        environment: Default::default(),
        log_path: None,
        stop_timeout: Duration::from_millis(200),
    };

    let proc = HelperProcess::new(process_config).expect("helper process");
    let managed = Arc::new(Mutex::new(Some(proc)));

    let restart_policy = HelperRestartPolicy {
        initial_backoff: Duration::from_millis(50),
        max_backoff: Duration::from_millis(200),
        multiplier: 2.0,
        startup_timeout: Duration::from_millis(100),
        initial_retry_delay: Duration::from_millis(10),
        max_retry_delay: Duration::from_millis(20),
    };

    let monitor = Arc::new(HelperHealthMonitor::with_managed_process(
        client.clone(),
        Duration::from_millis(20),
        Duration::from_millis(100),
        Arc::clone(&managed),
        restart_policy,
    ));

    let handle = monitor.spawn();

    // Let it run for 500ms under constant crash condition
    tokio::time::sleep(Duration::from_millis(500)).await;

    let snap = monitor.snapshot();
    assert!(!snap.healthy, "Snapshot must remain unhealthy");
    assert!(
        snap.consecutive_failures >= 1,
        "Consecutive failures must be tracked"
    );

    // Stop cleanly
    monitor.stop();
    let _ = tokio::time::timeout(Duration::from_secs(1), handle).await;

    let mut p = managed.lock().unwrap().take().unwrap();
    let _ = p.stop().await;
}

#[tokio::test]
async fn stress_test_live_helper_monitor_multi_crash_recovery() {
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

    let mut initial_proc = HelperProcess::new(process_config).expect("helper process");
    initial_proc
        .start_until_ready(
            &client,
            Duration::from_secs(3),
            Duration::from_millis(20),
            Duration::from_millis(50),
        )
        .await
        .expect("initial start");

    let managed = Arc::new(Mutex::new(Some(initial_proc)));

    let restart_policy = HelperRestartPolicy {
        initial_backoff: Duration::from_millis(30),
        max_backoff: Duration::from_millis(200),
        multiplier: 2.0,
        startup_timeout: Duration::from_secs(3),
        initial_retry_delay: Duration::from_millis(20),
        max_retry_delay: Duration::from_millis(50),
    };

    let monitor = Arc::new(HelperHealthMonitor::with_managed_process(
        client.clone(),
        Duration::from_millis(20),
        Duration::from_millis(500),
        Arc::clone(&managed),
        restart_policy,
    ));
    monitor.seed_success();

    let handle = monitor.spawn();
    let mut observed_pids = Vec::new();

    // Kill the process 3 times in a row while the monitor is actively running
    for cycle in 1..=3 {
        let pid = {
            let mut guard = managed.lock().unwrap();
            let proc = guard.as_mut().unwrap();
            proc.snapshot().pid.expect("pid")
        };
        observed_pids.push(pid);

        // SIGKILL while monitor is supervising
        send_sigkill(pid);

        // Wait for monitor to detect, back off, reap, and restart
        let deadline = Instant::now() + Duration::from_secs(5);
        let mut recovered = false;
        while Instant::now() < deadline {
            tokio::time::sleep(Duration::from_millis(40)).await;
            let snap = monitor.snapshot();
            if snap.healthy && snap.restarts >= cycle {
                recovered = true;
                break;
            }
        }
        assert!(
            recovered,
            "Cycle {cycle}: monitor failed to auto-recover helper process"
        );

        // Verify old PID was reaped and is not a zombie
        let old_state = query_os_process_state(pid);
        if let Some(ref st) = old_state {
            assert!(
                !st.starts_with('Z'),
                "Cycle {cycle}: PID {pid} was left as a zombie! State: {st}"
            );
        }
    }

    monitor.stop();
    let _ = handle.await;

    let mut p = managed.lock().unwrap().take().unwrap();
    let _ = p.stop().await;
}

#[tokio::test]
async fn stress_test_live_pine_monitor_multi_crash_recovery() {
    let token = "e".repeat(32);
    let pine_mock = spawn_mock_pine_worker("pineworker-multi", Some(&token))
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

    let spec = pine_mock.spec("pineworker-multi");
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let pine_wrapper = temp_dir.path().join("pine-worker-multi.sh");
    std::fs::write(&pine_wrapper, "#!/bin/sh\nexec sleep 300\n").expect("write pine wrapper");

    let mut proc = PineProcess::start(
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

    let initial_health = proc
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

    let readiness = PineReadinessState::new(spec.worker_id.clone());
    readiness.seed_success(initial_health);

    let process_arc = Arc::new(tokio::sync::Mutex::new(proc));
    let restart_policy = PineRestartPolicy {
        initial_backoff: Duration::from_millis(30),
        max_backoff: Duration::from_millis(200),
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
        Duration::from_millis(20),
        restart_policy,
    );

    // Kill the process 3 times in a row while the monitor is actively running
    for cycle in 1..=3 {
        let pid = {
            let proc_guard = process_arc.lock().await;
            proc_guard.pid().expect("pid")
        };

        send_sigkill(pid);

        // Wait for monitor to detect, back off, reap, and restart
        let deadline = Instant::now() + Duration::from_secs(5);
        let mut recovered = false;
        while Instant::now() < deadline {
            tokio::time::sleep(Duration::from_millis(40)).await;
            let snap = readiness.snapshot();
            if snap.healthy && snap.running && snap.restarts >= cycle {
                recovered = true;
                break;
            }
        }
        assert!(
            recovered,
            "Cycle {cycle}: monitor failed to auto-recover pine process"
        );

        // Verify old PID was reaped and is not a zombie
        let old_state = query_os_process_state(pid);
        if let Some(ref st) = old_state {
            assert!(
                !st.starts_with('Z'),
                "Cycle {cycle}: Pine PID {pid} was left as a zombie! State: {st}"
            );
        }
    }

    monitor.shutdown().await;
    let mut p = process_arc.lock().await;
    let _ = p.stop().await;
}
