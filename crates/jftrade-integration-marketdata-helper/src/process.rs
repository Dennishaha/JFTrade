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
}

impl HelperProcess {
    pub fn new(config: HelperProcessConfig) -> Result<Self, ProcessError> {
        validate(&config)?;
        Ok(Self {
            config,
            child: None,
            state: ProcessState::Stopped,
            last_error: None,
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

    pub fn snapshot(&self) -> ProcessSnapshot {
        ProcessSnapshot {
            state: self.state,
            endpoint: format!("http://{}:{}", self.config.host, self.config.port),
            pid: self.child.as_ref().and_then(Child::id),
            last_error: self.last_error.clone(),
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

    fn child_status(&mut self) -> Result<Option<std::process::ExitStatus>, ProcessError> {
        self.child
            .as_mut()
            .map(Child::try_wait)
            .transpose()
            .map_err(ProcessError::Io)
            .map(Option::flatten)
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
}
