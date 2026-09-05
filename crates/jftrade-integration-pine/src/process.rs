use std::collections::BTreeMap;
use std::fs::{self, OpenOptions};
use std::future::Future;
use std::net::IpAddr;
use std::path::PathBuf;
use std::pin::Pin;
use std::process::Stdio;
use std::time::Duration;

use thiserror::Error;
use tokio::process::{Child, Command};
use tonic::Request;
use tonic::metadata::MetadataValue;
use tonic::transport::Endpoint;

use crate::pool::WorkerHealth;

mod wire {
    tonic::include_proto!("jftrade.strategy.pineworker.v1");
}

use wire::HealthCheckRequest;
use wire::pine_worker_client::PineWorkerClient;

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
    pub log_path: Option<PathBuf>,
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

impl Default for PineReadinessPolicy {
    fn default() -> Self {
        Self::go_compatibility()
    }
}

pub trait PineReadinessProbe {
    fn health<'a>(
        &'a self,
        spec: &'a WorkerProcessSpec,
    ) -> Pin<Box<dyn Future<Output = Result<WorkerHealth, String>> + Send + 'a>>;
}

#[derive(Clone, Debug)]
pub struct GrpcPineReadinessProbe {
    bearer_token: Option<String>,
    connect_timeout: Duration,
    request_timeout: Duration,
}

impl GrpcPineReadinessProbe {
    pub fn new(
        bearer_token: Option<String>,
        connect_timeout: Duration,
        request_timeout: Duration,
    ) -> Result<Self, PineProcessError> {
        if bearer_token
            .as_ref()
            .is_some_and(|token| token.trim().len() < 32)
        {
            return Err(PineProcessError::WeakToken);
        }
        Ok(Self {
            bearer_token: bearer_token.map(|token| token.trim().to_owned()),
            connect_timeout,
            request_timeout,
        })
    }
}

impl PineReadinessProbe for GrpcPineReadinessProbe {
    fn health<'a>(
        &'a self,
        spec: &'a WorkerProcessSpec,
    ) -> Pin<Box<dyn Future<Output = Result<WorkerHealth, String>> + Send + 'a>> {
        Box::pin(async move {
            let endpoint = Endpoint::from_shared(format!("http://{}", spec.address()))
                .map_err(|error| error.to_string())?
                .connect_timeout(self.connect_timeout)
                .timeout(self.request_timeout);
            let channel = endpoint
                .connect()
                .await
                .map_err(|error| error.to_string())?;
            let mut client = PineWorkerClient::new(channel);
            let mut request = Request::new(HealthCheckRequest {});
            if let Some(token) = &self.bearer_token {
                let authorization = MetadataValue::try_from(format!("Bearer {token}"))
                    .map_err(|error| error.to_string())?;
                request
                    .metadata_mut()
                    .insert("authorization", authorization);
            }
            let response = client
                .health_check(request)
                .await
                .map_err(|error| error.to_string())?
                .into_inner();
            if response.worker_id != spec.worker_id {
                return Err(format!(
                    "pine worker endpoint identity mismatch: expected {}, received {}",
                    spec.worker_id, response.worker_id
                ));
            }
            Ok(WorkerHealth {
                ok: response.ok,
                version: response.version,
                pine_ts_version: response.pinets_version,
                capabilities: response.capabilities,
            })
        })
    }
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
    child: Option<Child>,
    restarts: u32,
}

impl PineProcess {
    pub fn start(
        spec: WorkerProcessSpec,
        config: PineProcessConfig,
    ) -> Result<Self, PineProcessError> {
        validate(&spec, &config)?;
        let (stdout, stderr) = child_stdio(config.log_path.as_deref())?;
        let mut command = Command::new(&config.runtime);
        command
            .arg(&config.bundle_path)
            .arg("--address")
            .arg(spec.address())
            .arg("--worker-id")
            .arg(&spec.worker_id)
            .envs(&config.environment)
            .stdin(Stdio::null())
            .stdout(stdout)
            .stderr(stderr)
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
            child: Some(child),
            restarts: 0,
        })
    }

    pub fn worker_id(&self) -> &str {
        &self.spec.worker_id
    }

    pub fn restarts(&self) -> u32 {
        self.restarts
    }

    pub fn spec(&self) -> &WorkerProcessSpec {
        &self.spec
    }

    pub fn config(&self) -> &PineProcessConfig {
        &self.config
    }

    pub fn pid(&self) -> Option<u32> {
        self.child.as_ref().and_then(Child::id)
    }

    pub fn is_alive(&mut self) -> bool {
        self.child
            .as_mut()
            .and_then(|c| c.try_wait().ok())
            .is_some_and(|status| status.is_none())
    }

    pub async fn wait_until_ready<P: PineReadinessProbe>(
        &mut self,
        probe: &P,
        policy: PineReadinessPolicy,
    ) -> Result<WorkerHealth, PineProcessError> {
        let Some(child) = self.child.as_mut() else {
            return Err(PineProcessError::Exited(format!(
                "{} has no running process",
                self.spec.worker_id
            )));
        };
        let deadline = tokio::time::Instant::now() + policy.timeout;
        let mut attempt = 0_u32;
        loop {
            if let Some(status) = child.try_wait()? {
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
                let _ = child.start_kill();
                let _ = tokio::time::timeout(self.config.stop_timeout, child.wait()).await;
                return Err(PineProcessError::ReadinessTimeout(last_error));
            }
            tokio::time::sleep(policy.retry_delay(attempt)).await;
            attempt = attempt.saturating_add(1);
        }
    }

    pub async fn stop(&mut self) -> Result<(), PineProcessError> {
        let Some(mut child) = self.child.take() else {
            return Ok(());
        };
        if child.try_wait()?.is_some() {
            return Ok(());
        }
        child.start_kill()?;
        match tokio::time::timeout(self.config.stop_timeout, child.wait()).await {
            Ok(result) => {
                result?;
                Ok(())
            }
            Err(_) => Err(PineProcessError::StopTimeout),
        }
    }

    pub async fn restart(&mut self) -> Result<(), PineProcessError> {
        let _ = self.stop().await;
        let (stdout, stderr) = child_stdio(self.config.log_path.as_deref())?;
        let mut command = Command::new(&self.config.runtime);
        command
            .arg(&self.config.bundle_path)
            .arg("--address")
            .arg(self.spec.address())
            .arg("--worker-id")
            .arg(&self.spec.worker_id)
            .envs(&self.config.environment)
            .stdin(Stdio::null())
            .stdout(stdout)
            .stderr(stderr)
            .kill_on_drop(true);
        if let Some(proto_path) = &self.config.proto_path {
            command.arg("--proto").arg(proto_path);
        }
        if let Some(max_message_bytes) = self.config.max_message_bytes {
            command
                .arg("--max-message-bytes")
                .arg(max_message_bytes.to_string());
        }
        if let Some(version) = &self.config.pine_ts_version {
            command.arg("--pinets-version").arg(version);
        }
        if let Some(token) = &self.config.bearer_token {
            command.env("JFTRADE_PINEWORKER_TOKEN", token);
        }
        let child = command.spawn()?;
        self.child = Some(child);
        self.restarts = self.restarts.saturating_add(1);
        Ok(())
    }

    pub async fn restart_until_ready<P: PineReadinessProbe>(
        &mut self,
        probe: &P,
        policy: PineReadinessPolicy,
    ) -> Result<WorkerHealth, PineProcessError> {
        self.restart().await?;
        self.wait_until_ready(probe, policy).await
    }

    pub fn terminate(&mut self) {
        if let Some(child) = self.child.as_mut()
            && child.try_wait().ok().flatten().is_none()
        {
            let _ = child.start_kill();
        }
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

    use tokio::net::TcpListener;
    use tokio::sync::oneshot;
    use tokio_stream::wrappers::TcpListenerStream;
    use tonic::{Response, Status};

    use super::*;
    use wire::pine_worker_server::{PineWorker, PineWorkerServer};
    use wire::{
        AnalyzeScriptRequest, AnalyzeScriptResponse, HealthCheckResponse, RunScriptRequest,
        RunScriptResponse,
    };

    struct TestPineWorker {
        worker_id: &'static str,
        token: &'static str,
    }

    #[tonic::async_trait]
    impl PineWorker for TestPineWorker {
        async fn health_check(
            &self,
            request: Request<HealthCheckRequest>,
        ) -> Result<Response<HealthCheckResponse>, Status> {
            let authorization = request
                .metadata()
                .get("authorization")
                .and_then(|value| value.to_str().ok());
            if authorization != Some(self.token) {
                return Err(Status::unauthenticated("invalid token"));
            }
            Ok(Response::new(HealthCheckResponse {
                ok: true,
                worker_id: self.worker_id.to_owned(),
                version: "1.0.0".to_owned(),
                pinets_version: "0.82.4".to_owned(),
                capabilities: vec!["run".to_owned()],
            }))
        }

        async fn analyze_script(
            &self,
            _request: Request<AnalyzeScriptRequest>,
        ) -> Result<Response<AnalyzeScriptResponse>, Status> {
            Err(Status::unimplemented("test"))
        }

        async fn run_script(
            &self,
            _request: Request<RunScriptRequest>,
        ) -> Result<Response<RunScriptResponse>, Status> {
            Err(Status::unimplemented("test"))
        }
    }

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
            log_path: None,
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

    #[tokio::test]
    async fn grpc_probe_authenticates_and_rejects_endpoint_identity_mismatch() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
            .await
            .expect("listener");
        let address = listener.local_addr().expect("address");
        let (shutdown_tx, shutdown_rx) = oneshot::channel();
        let server = tokio::spawn(async move {
            tonic::transport::Server::builder()
                .add_service(PineWorkerServer::new(TestPineWorker {
                    worker_id: "pineworker-1",
                    token: "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
                }))
                .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                    let _ = shutdown_rx.await;
                })
                .await
        });
        let probe = GrpcPineReadinessProbe::new(
            Some("a".repeat(32)),
            Duration::from_secs(1),
            Duration::from_secs(1),
        )
        .expect("probe");
        let mut spec = WorkerProcessSpec {
            worker_id: "pineworker-1".to_owned(),
            host: address.ip(),
            port: address.port(),
        };
        let health = probe.health(&spec).await.expect("health");
        assert!(health.ok);
        assert_eq!(health.version, "1.0.0");
        assert_eq!(health.pine_ts_version, "0.82.4");

        spec.worker_id = "wrong-worker".to_owned();
        let error = probe.health(&spec).await.expect_err("identity mismatch");
        assert!(error.contains("endpoint identity mismatch"));
        let _ = shutdown_tx.send(());
        server.await.expect("join server").expect("serve");
    }
}
