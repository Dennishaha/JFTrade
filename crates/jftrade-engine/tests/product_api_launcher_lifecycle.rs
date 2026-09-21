//! End-to-end launcher contracts for the API sidecar binary.
//!
//! The previous launcher handled Ctrl-C **and** SIGTERM through
//! `signal.NotifyContext`, and always ran the signal `stop()` before the API
//! server returned. The Rust launcher is a process entry point, so the
//! equivalent guarantee is proven by spawning the real binary, observing its
//! ready record, and stopping it with the signals a service supervisor sends.
//!
//! Anchor exception: the Go package path of this launcher is banned text for
//! `scripts/check-zero-go.mjs`, so these tests carry no `// Parity:` marker and
//! record the mapping in `docs/history/go-to-rust/cmd-jftrade-api-batch-scope.md`.
#![cfg(unix)]

use std::io::Read;
use std::net::{Ipv4Addr, SocketAddr, TcpListener, TcpStream};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use serde_json::Value;

const READY_TIMEOUT: Duration = Duration::from_secs(60);
const EXIT_TIMEOUT: Duration = Duration::from_secs(30);

struct Launcher {
    child: Child,
}

impl Launcher {
    fn spawn(settings_path: &Path, bind_address: SocketAddr) -> Self {
        let binary = std::env::var_os("NEXTEST_BIN_EXE_jftrade_api_rust")
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from(env!("CARGO_BIN_EXE_jftrade-api-rust")));
        let mut command = Command::new(binary);
        for (key, _) in std::env::vars() {
            if key.starts_with("JFTRADE_") {
                command.env_remove(&key);
            }
        }
        let child = command
            .env("JFTRADE_RUST_API_BIND", bind_address.to_string())
            .env("JFTRADE_SETTINGS_PATH", settings_path)
            .env("RUST_LOG", "error")
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("spawn the API launcher");
        Self { child }
    }

    fn signal(&self, signal: &str) {
        let status = Command::new("kill")
            .args([format!("-{signal}"), self.child.id().to_string()])
            .status()
            .expect("send a signal to the API launcher");
        assert!(status.success(), "kill -{signal} must reach the launcher");
    }

    fn wait_for_exit(&mut self, timeout: Duration) -> ExitStatus {
        let deadline = Instant::now() + timeout;
        loop {
            if let Some(status) = self.child.try_wait().expect("poll the launcher") {
                return status;
            }
            if Instant::now() >= deadline {
                let _ = self.child.kill();
                let _ = self.child.wait();
                panic!("the API launcher did not exit within {timeout:?}");
            }
            thread::sleep(Duration::from_millis(50));
        }
    }

    fn wait_until_serving(&mut self, address: SocketAddr) {
        let deadline = Instant::now() + READY_TIMEOUT;
        loop {
            if let Some(status) = self.child.try_wait().expect("poll the launcher") {
                let (stdout, stderr) = self.take_output();
                panic!(
                    "launcher exited before serving: {status:?}\nstdout: {stdout}\nstderr: {stderr}"
                );
            }
            if TcpStream::connect_timeout(&address, Duration::from_millis(250)).is_ok() {
                return;
            }
            if Instant::now() >= deadline {
                let _ = self.child.kill();
                let _ = self.child.wait();
                panic!("the API launcher never accepted connections on {address}");
            }
            thread::sleep(Duration::from_millis(100));
        }
    }

    fn take_output(&mut self) -> (String, String) {
        let mut stdout = String::new();
        let mut stderr = String::new();
        if let Some(mut handle) = self.child.stdout.take() {
            handle
                .read_to_string(&mut stdout)
                .expect("read launcher stdout");
        }
        if let Some(mut handle) = self.child.stderr.take() {
            handle
                .read_to_string(&mut stderr)
                .expect("read launcher stderr");
        }
        (stdout, stderr)
    }
}

impl Drop for Launcher {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

fn reserve_loopback_address() -> SocketAddr {
    let listener =
        TcpListener::bind(SocketAddr::from((Ipv4Addr::LOCALHOST, 0))).expect("reserve a port");
    let address = listener.local_addr().expect("reserved address");
    drop(listener);
    address
}

fn startup_record(stdout: &str) -> Value {
    let line = stdout.lines().next().expect("launcher startup record");
    serde_json::from_str(line).expect("startup record JSON")
}

#[test]
fn api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let address = reserve_loopback_address();

    let mut launcher = Launcher::spawn(&settings_path, address);
    launcher.wait_until_serving(address);

    launcher.signal("TERM");
    let status = launcher.wait_for_exit(EXIT_TIMEOUT);
    let (stdout, stderr) = launcher.take_output();

    assert_eq!(
        status.code(),
        Some(0),
        "an operator stop request must run the shutdown path instead of killing the \
         sidecar\nstderr: {stderr}"
    );
    let record = startup_record(&stdout);
    assert_eq!(record["event"], "ready");
    assert_eq!(
        record["address"],
        address.to_string(),
        "the launcher must serve the configured address instead of replacing it with a default"
    );
    assert_eq!(
        record["databaseLeaseStatus"], "acquired",
        "the ready record must report the acquired single-writer leases"
    );
}

#[test]
fn api_launcher_reports_startup_failure_when_the_configured_address_is_taken() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let listener = TcpListener::bind(SocketAddr::from((Ipv4Addr::LOCALHOST, 0)))
        .expect("hold a loopback port");
    let address = listener.local_addr().expect("held address");

    let mut blocked = Launcher::spawn(&settings_path, address);
    let status = blocked.wait_for_exit(EXIT_TIMEOUT);
    let (stdout, stderr) = blocked.take_output();
    assert_eq!(
        status.code(),
        Some(1),
        "an unavailable address must fail the launcher\nstdout: {stdout}\nstderr: {stderr}"
    );
    assert!(
        stdout.is_empty(),
        "a failed launcher must not claim readiness: {stdout}"
    );
    assert!(
        stderr.contains("Error: ") && stderr.contains("AddrInUse"),
        "startup failures must stay visible with their cause: {stderr}"
    );

    // Releasing the port and starting again proves the failed attempt left no
    // writer lease behind on the settings database set.
    drop(listener);
    let mut retry = Launcher::spawn(&settings_path, address);
    retry.wait_until_serving(address);
    retry.signal("TERM");
    let status = retry.wait_for_exit(EXIT_TIMEOUT);
    assert_eq!(
        status.code(),
        Some(0),
        "the retry must start and shut down cleanly"
    );
}
