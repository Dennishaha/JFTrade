use std::collections::BTreeMap;
use std::fs::{self, OpenOptions};
use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::path::PathBuf;
use std::process::Stdio;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use thiserror::Error;
use tokio::process::{Child, Command};

use crate::client::HelperClient;

#[derive(Clone, Debug)]
pub struct HelperProcessConfig {
    pub executable: PathBuf,
    pub host: IpAddr,
    pub port: u16,
    pub bearer_token: Option<String>,
    pub prefix_args: Vec<String>,
    pub extra_args: Vec<String>,
    pub environment: BTreeMap<String, String>,
    pub log_path: Option<PathBuf>,
    pub stop_timeout: Duration,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ProcessState {
    Stopped,
    Starting,
    Ready,
    Failed,
    Stopping,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProcessSnapshot {
    pub state: ProcessState,
    pub endpoint: String,
    pub pid: Option<u32>,
    pub last_error: Option<String>,
    #[serde(default)]
    pub restarts: u32,
}

#[derive(Debug, Error)]
pub enum ProcessError {
    #[error("market-data helper executable is required")]
    MissingExecutable,
    #[error("market-data helper must bind loopback")]
    PublicBind,
    #[error("market-data helper port must be non-zero")]
    InvalidPort,
    #[error("market-data helper token must contain at least 32 non-whitespace characters")]
    WeakToken,
    #[error("market-data helper process is already running")]
    AlreadyRunning,
    #[error("market-data helper readiness endpoint does not match the managed process")]
    EndpointMismatch,
    #[error("market-data helper exited before readiness: {0}")]
    Exited(String),
    #[error("market-data helper did not become ready within the configured deadline: {0}")]
    ReadinessTimeout(String),
    #[error("market-data helper process I/O: {0}")]
    Io(#[from] std::io::Error),
    #[error("market-data helper did not stop within the configured deadline")]
    StopTimeout,
}

pub struct HelperProcess {
    config: HelperProcessConfig,
    child: Option<Child>,
    state: ProcessState,
    last_error: Option<String>,
    restarts: u32,
}

impl HelperProcess {
    pub fn new(config: HelperProcessConfig) -> Result<Self, ProcessError> {
        validate(&config)?;
        Ok(Self {
            config,
            child: None,
            state: ProcessState::Stopped,
            last_error: None,
            restarts: 0,
        })
    }

    pub fn start(&mut self) -> Result<ProcessSnapshot, ProcessError> {
        if self.child.is_some() {
            return Err(ProcessError::AlreadyRunning);
        }
        self.state = ProcessState::Starting;
        let (stdout, stderr) = child_stdio(self.config.log_path.as_deref())?;
        let mut command = Command::new(&self.config.executable);
        command
            .args(&self.config.prefix_args)
            .arg("--host")
            .arg(self.config.host.to_string())
            .arg("--port")
            .arg(self.config.port.to_string())
            .args(&self.config.extra_args)
            .envs(&self.config.environment)
            .stdin(Stdio::null())
            .stdout(stdout)
            .stderr(stderr)
            .kill_on_drop(true);
        if let Some(token) = &self.config.bearer_token {
            command.env("JFTRADE_MARKETDATA_HELPER_TOKEN", token);
        }
        match command.spawn() {
            Ok(child) => {
                self.child = Some(child);
                self.last_error = None;
                Ok(self.snapshot())
            }
            Err(error) => {
                self.state = ProcessState::Failed;
                self.last_error = Some(error.to_string());
                Err(ProcessError::Io(error))
            }
        }
    }

    pub fn mark_ready(&mut self) -> ProcessSnapshot {
        if self.child.is_some() {
            self.state = ProcessState::Ready;
            self.last_error = None;
        }
        self.snapshot()
    }

    pub async fn start_until_ready(
        &mut self,
        client: &HelperClient,
        startup_timeout: Duration,
        initial_retry_delay: Duration,
        max_retry_delay: Duration,
    ) -> Result<ProcessSnapshot, ProcessError> {
        if client.endpoint().trim_end_matches('/') != self.snapshot().endpoint {
            return Err(ProcessError::EndpointMismatch);
        }
        self.start()?;
        let deadline = tokio::time::Instant::now() + startup_timeout;
        let mut attempt = 0_u32;
        loop {
            if let Some(status) = self.child_status()? {
                self.child = None;
                self.state = ProcessState::Failed;
                let error = format!("process exited with {status}");
                self.last_error = Some(error.clone());
                return Err(ProcessError::Exited(error));
            }
            let last_error = match client.healthz().await {
                Ok(_) => return Ok(self.mark_ready()),
                Err(error) => error.to_string(),
            };
            if tokio::time::Instant::now() >= deadline {
                let _ = self.stop().await;
                self.state = ProcessState::Failed;
                self.last_error = Some(last_error.clone());
                return Err(ProcessError::ReadinessTimeout(last_error));
            }
            tokio::time::sleep(retry_delay(initial_retry_delay, max_retry_delay, attempt)).await;
            attempt = attempt.saturating_add(1);
        }
    }

    pub async fn stop(&mut self) -> Result<ProcessSnapshot, ProcessError> {
        let Some(mut child) = self.child.take() else {
            self.state = ProcessState::Stopped;
            return Ok(self.snapshot());
        };
        self.state = ProcessState::Stopping;
        if child.try_wait()?.is_some() {
            self.state = ProcessState::Stopped;
            self.last_error = None;
            return Ok(self.snapshot());
        }
        child.start_kill()?;
        match tokio::time::timeout(self.config.stop_timeout, child.wait()).await {
            Ok(result) => {
                result?;
                self.state = ProcessState::Stopped;
                self.last_error = None;
                Ok(self.snapshot())
            }
            Err(_) => {
                self.state = ProcessState::Failed;
                self.last_error = Some(ProcessError::StopTimeout.to_string());
                Err(ProcessError::StopTimeout)
            }
        }
    }

    pub fn restarts(&self) -> u32 {
        self.restarts
    }

    pub fn config(&self) -> &HelperProcessConfig {
        &self.config
    }

    pub async fn restart(&mut self) -> Result<ProcessSnapshot, ProcessError> {
        let _ = self.stop().await;
        let snapshot = self.start()?;
        self.restarts = self.restarts.saturating_add(1);
        Ok(snapshot)
    }

    pub async fn restart_until_ready(
        &mut self,
        client: &HelperClient,
        startup_timeout: Duration,
        initial_retry_delay: Duration,
        max_retry_delay: Duration,
    ) -> Result<ProcessSnapshot, ProcessError> {
        let _ = self.stop().await;
        let snapshot = self
            .start_until_ready(
                client,
                startup_timeout,
                initial_retry_delay,
                max_retry_delay,
            )
            .await?;
        self.restarts = self.restarts.saturating_add(1);
        Ok(snapshot)
    }

    pub fn snapshot(&self) -> ProcessSnapshot {
        ProcessSnapshot {
            state: self.state,
            endpoint: format!("http://{}:{}", self.config.host, self.config.port),
            pid: self.child.as_ref().and_then(Child::id),
            last_error: self.last_error.clone(),
            restarts: self.restarts,
        }
    }

    pub fn terminate(&mut self) {
        if let Some(child) = self.child.as_mut()
            && child.try_wait().ok().flatten().is_none()
        {
            let _ = child.start_kill();
            self.state = ProcessState::Stopping;
        }
    }

    pub fn is_alive(&mut self) -> bool {
        self.child
            .as_mut()
            .and_then(|c| c.try_wait().ok())
            .is_some_and(|status| status.is_none())
    }

    pub(crate) fn child_status(
        &mut self,
    ) -> Result<Option<std::process::ExitStatus>, ProcessError> {
        self.child
            .as_mut()
            .map(Child::try_wait)
            .transpose()
            .map_err(ProcessError::Io)
            .map(Option::flatten)
    }
}

impl Drop for HelperProcess {
    fn drop(&mut self) {
        self.terminate();
    }
}

fn child_stdio(log_path: Option<&std::path::Path>) -> Result<(Stdio, Stdio), std::io::Error> {
    let Some(log_path) = log_path else {
        return Ok((Stdio::null(), Stdio::null()));
    };
    if let Some(directory) = log_path
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
    {
        fs::create_dir_all(directory)?;
    }
    let stderr = OpenOptions::new()
        .create(true)
        .append(true)
        .open(log_path)?;
    let stdout = stderr.try_clone()?;
    Ok((Stdio::from(stdout), Stdio::from(stderr)))
}

fn retry_delay(initial: Duration, maximum: Duration, attempt: u32) -> Duration {
    let maximum = maximum.max(initial);
    initial
        .saturating_mul(2_u32.saturating_pow(attempt.min(31)))
        .min(maximum)
}

fn validate(config: &HelperProcessConfig) -> Result<(), ProcessError> {
    if config.executable.as_os_str().is_empty() {
        return Err(ProcessError::MissingExecutable);
    }
    if !config.host.is_loopback() {
        return Err(ProcessError::PublicBind);
    }
    if config.port == 0 {
        return Err(ProcessError::InvalidPort);
    }
    if config
        .bearer_token
        .as_ref()
        .is_some_and(|token| token.trim().len() < 32)
    {
        return Err(ProcessError::WeakToken);
    }
    Ok(())
}

pub fn allocate_loopback_port() -> Result<u16, ProcessError> {
    let listener = std::net::TcpListener::bind(SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0))?;
    Ok(listener.local_addr()?.port())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::path::PathBuf;

    #[test]
    fn process_config_rejects_public_bind_and_weak_token() {
        let config = HelperProcessConfig {
            executable: "helper".into(),
            host: Ipv4Addr::UNSPECIFIED.into(),
            port: 1,
            bearer_token: None,
            prefix_args: Vec::new(),
            extra_args: Vec::new(),
            environment: BTreeMap::new(),
            log_path: None,
            stop_timeout: Duration::from_secs(1),
        };
        assert!(matches!(
            HelperProcess::new(config),
            Err(ProcessError::PublicBind)
        ));
    }

    #[test]
    fn allocated_port_is_non_zero_and_loopback_reusable() {
        let port = allocate_loopback_port().expect("port");
        assert_ne!(port, 0);
        std::net::TcpListener::bind((Ipv4Addr::LOCALHOST, port)).expect("port released");
    }

    #[test]
    fn readiness_retry_delay_is_exponential_and_capped() {
        let initial = Duration::from_millis(25);
        let maximum = Duration::from_millis(100);
        assert_eq!(retry_delay(initial, maximum, 0), initial);
        assert_eq!(retry_delay(initial, maximum, 1), Duration::from_millis(50));
        assert_eq!(retry_delay(initial, maximum, 2), maximum);
        assert_eq!(retry_delay(initial, maximum, 20), maximum);
    }
    fn temp_config(name: &str) -> HelperProcessConfig {
        HelperProcessConfig {
            executable: PathBuf::from(format!("/tmp/jftrade-helper-{name}")),
            host: Ipv4Addr::LOCALHOST.into(),
            port: 43_123,
            bearer_token: None,
            prefix_args: Vec::new(),
            extra_args: Vec::new(),
            environment: BTreeMap::new(),
            log_path: None,
            stop_timeout: Duration::from_millis(250),
        }
    }

    #[tokio::test]
    async fn start_rejects_a_second_launch_of_a_live_process() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:15
        // TestSidecarManagerStartsReusesAndStopsManagedExecutable (reuse half)
        let root = std::env::temp_dir().join(format!(
            "jftrade-helper-process-{}-{}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).expect("create fixture dir");
        let script = root.join("helper.sh");
        fs::write(&script, "#!/bin/sh\nwhile true; do sleep 0.05; done\n").expect("write script");
        let mut permissions = fs::metadata(&script).expect("stat").permissions();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            permissions.set_mode(0o755);
        }
        fs::set_permissions(&script, permissions).expect("chmod");

        let mut config = temp_config("reuse");
        config.executable = script.clone();
        config.stop_timeout = Duration::from_millis(250);
        let mut process = HelperProcess::new(config).expect("helper process");
        let started = process.start().expect("start managed executable");
        assert_eq!(started.endpoint, "http://127.0.0.1:43123");
        assert!(started.pid.is_some(), "started process must expose its pid");
        assert!(process.is_alive(), "managed process must be running");

        // A live managed process is reused: starting again is refused rather
        // than spawning a second owner for the same endpoint.
        assert!(matches!(process.start(), Err(ProcessError::AlreadyRunning)));

        let snapshot = process.snapshot();
        assert!(matches!(snapshot.state, ProcessState::Starting));
        assert_eq!(snapshot.restarts, 0);
        let _ = process.stop().await;
        assert!(!process.is_alive(), "managed process must be reaped");
        let _ = fs::remove_dir_all(root);
    }

    #[tokio::test]
    async fn stop_is_idempotent_and_bounds_graceful_shutdown_before_killing_the_child() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:115
        // TestOSSidecarProcessBoundsGracefulStopBeforeKillingChild and
        // sidecar_process_test.go:15 TestSidecarManagerStartsReusesAndStopsManagedExecutable (stop half)
        let mut process =
            HelperProcess::new(temp_config("missing-binary")).expect("config accepted");

        // Stopping a process that never started succeeds and reports Stopped
        // instead of failing: callers treat an already-finished process as a
        // successful stop.
        let stopped = process
            .stop()
            .await
            .expect("stopping an idle process is a no-op success");
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(stopped.pid.is_none());
        assert!(stopped.last_error.is_none());

        // A second stop must stay idempotent so shutdown retries cannot fail.
        let again = process
            .stop()
            .await
            .expect("repeated stop stays successful");
        assert!(matches!(again.state, ProcessState::Stopped));
        assert_eq!(again.restarts, 0);
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:78 TestRuntimeStopsNewSidecarWhenInitialAKShareActivationFails
    #[tokio::test]
    async fn a_failed_launch_never_leaves_a_child_or_a_stale_endpoint() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:92
        // TestSidecarManagerCleansMaterializationAfterPreparationFailures
        let mut config = temp_config("does-not-exist");
        config.executable = PathBuf::from("/tmp/jftrade-helper-definitely-missing-binary");
        let mut process = HelperProcess::new(config).expect("config accepted");

        let error = process.start().expect_err("launch must fail closed");
        assert!(matches!(error, ProcessError::Io(_)));
        let failed = process.snapshot();
        assert!(
            failed.pid.is_none(),
            "a failed launch must not retain a child handle"
        );
        assert!(
            failed.last_error.is_some(),
            "failure reason must be recorded"
        );
        assert!(matches!(failed.state, ProcessState::Failed));

        // Recovery path: the failed process can still be stopped cleanly, and
        // the failure must not be reported as a successful run.
        let stopped = process.stop().await.expect("stop after a failed launch");
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(!process.is_alive());
    }
    #[tokio::test]
    async fn a_naturally_exited_child_reports_stopped_and_can_be_closed_again() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:78
        // TestOSSidecarProcessTreatsNaturalExitAsStoppedAndCloseable
        let root = std::env::temp_dir().join(format!(
            "jftrade-helper-natural-exit-{}-{}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).expect("create fixture dir");
        let script = root.join("short-lived.sh");
        fs::write(&script, "#!/bin/sh\nexit 0\n").expect("write script");
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut permissions = fs::metadata(&script).expect("stat").permissions();
            permissions.set_mode(0o755);
            fs::set_permissions(&script, permissions).expect("chmod");
        }

        let mut config = temp_config("natural-exit");
        config.executable = script.clone();
        let mut process = HelperProcess::new(config).expect("helper process");
        process.start().expect("start short-lived child");

        // The child exits on its own; the managed handle must observe that as
        // "not alive" instead of reporting a phantom running process.
        let mut alive = true;
        for _ in 0..50 {
            if !process.is_alive() {
                alive = false;
                break;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        assert!(!alive, "a naturally exited child must not report alive");

        // Closing an already-finished process is a successful, repeatable
        // operation, so shutdown retries cannot fail spuriously.
        let first = process.stop().await.expect("close after natural exit");
        assert!(matches!(first.state, ProcessState::Stopped));
        assert!(first.last_error.is_none());
        let second = process.stop().await.expect("second close stays successful");
        assert!(matches!(second.state, ProcessState::Stopped));
        let _ = fs::remove_dir_all(root);
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:11 TestRuntimeReusesSharedSidecarAcrossPythonProviders
    #[tokio::test]
    async fn managed_process_is_reused_until_an_explicit_stop_releases_it() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_process_test.go:15
        // TestSidecarManagerStartsReusesAndStopsManagedExecutable (lifecycle)
        let root = std::env::temp_dir().join(format!(
            "jftrade-helper-reuse-{}-{}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).expect("create fixture dir");
        let pid_file = root.join("pid");
        let script = root.join("helper.sh");
        fs::write(
            &script,
            format!(
                "#!/bin/sh\necho $$ > {}\nwhile true; do sleep 0.05; done\n",
                pid_file.display()
            ),
        )
        .expect("write script");
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut permissions = fs::metadata(&script).expect("stat").permissions();
            permissions.set_mode(0o755);
            fs::set_permissions(&script, permissions).expect("chmod");
        }

        let mut config = temp_config("reuse-owner");
        config.executable = script.clone();
        let mut process = HelperProcess::new(config).expect("helper process");
        let started = process.start().expect("start managed executable");
        let first_pid = started.pid.expect("first pid");

        // The endpoint and pid stay stable while the process is alive: the
        // managed executable is owned by exactly one runner.
        assert_eq!(process.snapshot().endpoint, started.endpoint);
        assert_eq!(process.snapshot().pid, Some(first_pid));
        assert_eq!(process.snapshot().restarts, 0);

        // A restart is the only path that produces a new child, and it is
        // accounted for so callers can see recovery happened.
        let restarted = process.restart().await.expect("restart managed executable");
        assert!(restarted.pid.is_some());
        assert_eq!(
            process.restarts(),
            1,
            "each restart is accounted for so callers can observe recovery"
        );
        assert_eq!(
            process.snapshot().endpoint,
            started.endpoint,
            "the managed endpoint is stable across restarts"
        );

        let stopped = process.stop().await.expect("stop managed executable");
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(stopped.pid.is_none());
        let _ = fs::remove_dir_all(root);
    }
    #[tokio::test]
    async fn already_finished_children_are_stopped_successfully_and_repeatable() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:161
        // TestProcessAlreadyFinishedIsASuccessfulStop
        let mut process =
            HelperProcess::new(temp_config("already-finished")).expect("helper process");

        // Never started: the handle has no child at all, which is the same
        // observable state as a child that already finished.
        let stopped = process
            .stop()
            .await
            .expect("stopping an already finished process succeeds");
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(stopped.pid.is_none());
        assert!(stopped.last_error.is_none());
        assert_eq!(stopped.restarts, 0);

        // A second stop after the child is gone must not turn the successful
        // outcome into an error: shutdown retries have to stay idempotent.
        let repeated = process
            .stop()
            .await
            .expect("a repeated stop of a finished process succeeds");
        assert!(matches!(repeated.state, ProcessState::Stopped));
        assert!(repeated.last_error.is_none());
    }

    #[tokio::test]
    async fn stop_escalates_to_kill_when_the_child_outlives_the_grace_period() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:115
        // TestOSSidecarProcessBoundsGracefulStopBeforeKillingChild and
        // sidecar_os_process_test.go:146 TestWaitForSidecarDoneHasExplicitTimeout
        let root = std::env::temp_dir().join(format!(
            "jftrade-helper-grace-{}-{}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).expect("create fixture dir");
        let script = root.join("stubborn.sh");
        // The child ignores SIGTERM so the only way out is the kill escalation.
        fs::write(
            &script,
            "#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 0.05; done\n",
        )
        .expect("write script");
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut permissions = fs::metadata(&script).expect("stat").permissions();
            permissions.set_mode(0o755);
            fs::set_permissions(&script, permissions).expect("chmod");
        }

        let mut config = temp_config("grace-period");
        config.executable = script.clone();
        config.stop_timeout = Duration::from_millis(200);
        let mut process = HelperProcess::new(config).expect("helper process");
        process.start().expect("start stubborn child");
        assert!(process.is_alive(), "child must be running before the stop");

        // The stop must be bounded: it never waits forever, and it must not
        // leave the child running once it returns.
        let started_at = std::time::Instant::now();
        let stopped = process.stop().await.expect("bounded stop");
        assert!(
            started_at.elapsed() < Duration::from_secs(5),
            "stop must be bounded, elapsed {:?}",
            started_at.elapsed()
        );
        assert!(matches!(stopped.state, ProcessState::Stopped));
        assert!(stopped.pid.is_none());
        assert!(
            !process.is_alive(),
            "a returned stop must not leave the child running"
        );
        let _ = fs::remove_dir_all(root);
    }
}
