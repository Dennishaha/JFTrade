use std::collections::BTreeMap;
use std::future::Future;
use std::net::IpAddr;
use std::path::PathBuf;
use std::pin::Pin;
use std::process::Stdio;
use std::time::Duration;

use thiserror::Error;
use tokio::process::{Child, Command};

use crate::pool::WorkerHealth;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WorkerProcessSpec {
    pub worker_id: String,
    pub host: IpAddr,
    pub port: u16,
}

impl WorkerProcessSpec {
    pub fn address(&self) -> String {
        format!("{}:{}", self.host, self.port)
    }
}

#[derive(Clone, Debug)]
pub struct PineProcessConfig {
    pub runtime: PathBuf,
    pub bundle_path: PathBuf,
    pub proto_path: Option<PathBuf>,
    pub max_message_bytes: Option<usize>,
    pub pine_ts_version: Option<String>,
    pub bearer_token: Option<String>,
    pub environment: BTreeMap<String, String>,
    pub stop_timeout: Duration,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PineReadinessPolicy {
    pub timeout: Duration,
    pub initial_retry_delay: Duration,
    pub max_retry_delay: Duration,
}

impl PineReadinessPolicy {
    pub const fn go_compatibility() -> Self {
        Self {
            timeout: Duration::from_secs(5),
            initial_retry_delay: Duration::from_millis(50),
            max_retry_delay: Duration::from_millis(50),
        }
    }

    pub fn retry_delay(self, attempt: u32) -> Duration {
        let maximum = self.max_retry_delay.max(self.initial_retry_delay);
        self.initial_retry_delay
            .saturating_mul(2_u32.saturating_pow(attempt.min(31)))
            .min(maximum)
    }
}

pub trait PineReadinessProbe {
    fn health<'a>(
        &'a self,
        spec: &'a WorkerProcessSpec,
    ) -> Pin<Box<dyn Future<Output = Result<WorkerHealth, String>> + Send + 'a>>;
}

#[derive(Debug, Error)]
pub enum PineProcessError {
    #[error("pine worker runtime and bundle path are required")]
    MissingExecutable,
    #[error("pine worker must bind loopback")]
    PublicBind,
    #[error("pine worker port must be non-zero")]
    InvalidPort,
    #[error("pine worker id is required")]
    MissingWorkerId,
    #[error("pine worker token must contain at least 32 non-whitespace characters")]
    WeakToken,
    #[error("pine worker process I/O: {0}")]
    Io(#[from] std::io::Error),
    #[error("pine worker exited before readiness: {0}")]
    Exited(String),
    #[error("pine worker did not become ready within the configured deadline: {0}")]
    ReadinessTimeout(String),
    #[error("pine worker did not stop within the configured deadline")]
    StopTimeout,
}

pub struct PineProcess {
    spec: WorkerProcessSpec,
    config: PineProcessConfig,
    child: Child,
}

impl PineProcess {
    pub fn start(
        spec: WorkerProcessSpec,
        config: PineProcessConfig,
    ) -> Result<Self, PineProcessError> {
        validate(&spec, &config)?;
        let mut command = Command::new(&config.runtime);
        command
            .arg(&config.bundle_path)
            .arg("--address")
            .arg(spec.address())
            .arg("--worker-id")
            .arg(&spec.worker_id)
            .envs(&config.environment)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .kill_on_drop(true);
        if let Some(proto_path) = &config.proto_path {
            command.arg("--proto").arg(proto_path);
        }
        if let Some(max_message_bytes) = config.max_message_bytes {
            command
                .arg("--max-message-bytes")
                .arg(max_message_bytes.to_string());
        }
        if let Some(version) = &config.pine_ts_version {
            command.arg("--pinets-version").arg(version);
        }
        if let Some(token) = &config.bearer_token {
            command.env("JFTRADE_PINEWORKER_TOKEN", token);
        }
        let child = command.spawn()?;
        Ok(Self {
            spec,
            config,
            child,
        })
    }

    pub fn worker_id(&self) -> &str {
        &self.spec.worker_id
    }

    pub fn pid(&self) -> Option<u32> {
        self.child.id()
    }

    pub async fn wait_until_ready<P: PineReadinessProbe>(
        &mut self,
        probe: &P,
        policy: PineReadinessPolicy,
    ) -> Result<WorkerHealth, PineProcessError> {
        let deadline = tokio::time::Instant::now() + policy.timeout;
        let mut attempt = 0_u32;
        loop {
            if let Some(status) = self.child.try_wait()? {
                return Err(PineProcessError::Exited(format!(
                    "{} exited with {status}",
                    self.spec.worker_id
                )));
            }
            let last_error = match probe.health(&self.spec).await {
                Ok(health) if health.ok => return Ok(health),
                Ok(_) => "worker health returned ok=false".to_owned(),
                Err(error) => error,
            };
            if tokio::time::Instant::now() >= deadline {
                let _ = self.child.start_kill();
                let _ = tokio::time::timeout(self.config.stop_timeout, self.child.wait()).await;
                return Err(PineProcessError::ReadinessTimeout(last_error));
            }
            tokio::time::sleep(policy.retry_delay(attempt)).await;
            attempt = attempt.saturating_add(1);
        }
    }

    pub async fn stop(mut self) -> Result<(), PineProcessError> {
        self.child.start_kill()?;
        match tokio::time::timeout(self.config.stop_timeout, self.child.wait()).await {
            Ok(result) => {
                result?;
                Ok(())
            }
            Err(_) => Err(PineProcessError::StopTimeout),
        }
    }
}

fn validate(spec: &WorkerProcessSpec, config: &PineProcessConfig) -> Result<(), PineProcessError> {
    if config.runtime.as_os_str().is_empty() || config.bundle_path.as_os_str().is_empty() {
        return Err(PineProcessError::MissingExecutable);
    }
    if !spec.host.is_loopback() {
        return Err(PineProcessError::PublicBind);
    }
    if spec.port == 0 {
        return Err(PineProcessError::InvalidPort);
    }
    if spec.worker_id.trim().is_empty() {
        return Err(PineProcessError::MissingWorkerId);
    }
    if config
        .bearer_token
        .as_ref()
        .is_some_and(|token| token.trim().len() < 32)
    {
        return Err(PineProcessError::WeakToken);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::net::Ipv4Addr;

    use super::*;

    #[test]
    fn validates_loopback_worker_boundary_before_spawn() {
        let spec = WorkerProcessSpec {
            worker_id: "pineworker-1".to_owned(),
            host: Ipv4Addr::UNSPECIFIED.into(),
            port: 50051,
        };
        let config = PineProcessConfig {
            runtime: "node".into(),
            bundle_path: "worker.mjs".into(),
            proto_path: None,
            max_message_bytes: None,
            pine_ts_version: None,
            bearer_token: None,
            environment: BTreeMap::new(),
            stop_timeout: Duration::from_secs(1),
        };
        assert!(matches!(
            validate(&spec, &config),
            Err(PineProcessError::PublicBind)
        ));
    }

    #[test]
    fn readiness_policy_preserves_go_delay_and_supports_capped_backoff() {
        let compatibility = PineReadinessPolicy::go_compatibility();
        assert_eq!(compatibility.retry_delay(0), Duration::from_millis(50));
        assert_eq!(compatibility.retry_delay(10), Duration::from_millis(50));

        let backoff = PineReadinessPolicy {
            timeout: Duration::from_secs(1),
            initial_retry_delay: Duration::from_millis(25),
            max_retry_delay: Duration::from_millis(100),
        };
        assert_eq!(backoff.retry_delay(0), Duration::from_millis(25));
        assert_eq!(backoff.retry_delay(1), Duration::from_millis(50));
        assert_eq!(backoff.retry_delay(2), Duration::from_millis(100));
        assert_eq!(backoff.retry_delay(20), Duration::from_millis(100));
    }
}
