//! Test-support mock PineTS worker.
//!
//! Composition tests must prove the real Pine worker lifecycle — a real child
//! process spawned through [`PineProcess::start`], a real gRPC readiness
//! probe, and real stop/terminate — without depending on Node or a real PineTS
//! bundle.  This module serves the generated `PineWorker` HealthCheck service
//! over loopback gRPC with a controllable readiness flag and bearer-token
//! validation, so engine composition tests can drive the full lifecycle with
//! a fixture child process (e.g. `sh -c "exec sleep N"`) standing in for the
//! Node runtime.

use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio_stream::wrappers::TcpListenerStream;
use tonic::transport::Server;
use tonic::{Request, Response, Status};

use crate::process::WorkerProcessSpec;

mod wire {
    tonic::include_proto!("jftrade.strategy.pineworker.v1");
}

use wire::pine_worker_server::{PineWorker, PineWorkerServer};
use wire::{
    AnalyzeScriptRequest, AnalyzeScriptResponse, HealthCheckRequest, HealthCheckResponse,
    RunScriptRequest, RunScriptResponse,
};

/// Handle for one running mock pine worker gRPC server.  Dropping the handle
/// requests shutdown; [`MockPineWorker::shutdown`] awaits the server task.
pub struct MockPineWorker {
    address: SocketAddr,
    ok: Arc<AtomicBool>,
    shutdown_tx: Option<oneshot::Sender<()>>,
    server: tokio::task::JoinHandle<()>,
}

impl MockPineWorker {
    pub fn address(&self) -> SocketAddr {
        self.address
    }

    pub fn spec(&self, worker_id: &str) -> WorkerProcessSpec {
        WorkerProcessSpec {
            worker_id: worker_id.to_owned(),
            host: IpAddr::V4(Ipv4Addr::LOCALHOST),
            port: self.address.port(),
        }
    }

    /// Flip the controllable readiness flag served by HealthCheck.
    pub fn set_ok(&self, ok: bool) {
        self.ok.store(ok, Ordering::Release);
    }

    pub async fn shutdown(mut self) {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        let _ = self.server.await;
    }
}

struct MockPineWorkerService {
    worker_id: Arc<str>,
    expected_authorization: Option<String>,
    ok: Arc<AtomicBool>,
}

fn unauthorized(token: Option<&str>) -> Status {
    Status::unauthenticated(format!(
        "mock pine worker rejected bearer token: {}",
        token.unwrap_or("<missing>")
    ))
}

#[tonic::async_trait]
impl PineWorker for MockPineWorkerService {
    async fn health_check(
        &self,
        request: Request<HealthCheckRequest>,
    ) -> Result<Response<HealthCheckResponse>, Status> {
        let authorization = request
            .metadata()
            .get("authorization")
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);
        let authorization = authorization.as_deref();
        match &self.expected_authorization {
            Some(expected) if authorization != Some(expected.as_str()) => {
                return Err(unauthorized(authorization));
            }
            None if authorization.is_some_and(|value| !value.is_empty()) => {
                return Err(unauthorized(authorization));
            }
            _ => {}
        }
        Ok(Response::new(HealthCheckResponse {
            ok: self.ok.load(Ordering::Acquire),
            worker_id: self.worker_id.to_string(),
            version: "0.0.0-mock".to_owned(),
            pinets_version: "0.0.0-mock".to_owned(),
            capabilities: vec!["health".to_owned()],
        }))
    }

    async fn analyze_script(
        &self,
        _request: Request<AnalyzeScriptRequest>,
    ) -> Result<Response<AnalyzeScriptResponse>, Status> {
        Err(Status::unimplemented("mock pine worker"))
    }

    async fn run_script(
        &self,
        _request: Request<RunScriptRequest>,
    ) -> Result<Response<RunScriptResponse>, Status> {
        Err(Status::unimplemented("mock pine worker"))
    }
}

/// Serve the mock PineWorker HealthCheck service on a loopback port.  When
/// `bearer_token` is provided the mock requires `Bearer <token>` metadata,
/// proving the probe path actually authenticates.
pub async fn spawn_mock_pine_worker(
    worker_id: &str,
    bearer_token: Option<&str>,
) -> Result<MockPineWorker, String> {
    let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
        .await
        .map_err(|error| format!("mock pine worker bind: {error}"))?;
    let address = listener
        .local_addr()
        .map_err(|error| format!("mock pine worker address: {error}"))?;
    let expected_authorization = bearer_token.map(|token| format!("Bearer {}", token.trim()));
    let ok = Arc::new(AtomicBool::new(true));
    let service = MockPineWorkerService {
        worker_id: Arc::from(worker_id),
        expected_authorization,
        ok: Arc::clone(&ok),
    };
    let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
    let server = tokio::spawn(async move {
        let _ = Server::builder()
            .add_service(PineWorkerServer::new(service))
            .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                let _ = shutdown_rx.await;
            })
            .await;
    });
    Ok(MockPineWorker {
        address,
        ok,
        shutdown_tx: Some(shutdown_tx),
        server,
    })
}

/// Wait until the mock worker's port accepts TCP connections, so the spawned
/// fixture child cannot lose the readiness race.
pub async fn wait_until_listening(address: SocketAddr, timeout: Duration) -> Result<(), String> {
    let deadline = tokio::time::Instant::now() + timeout;
    while tokio::time::Instant::now() < deadline {
        if tokio::net::TcpStream::connect(address).await.is_ok() {
            return Ok(());
        }
        tokio::time::sleep(Duration::from_millis(5)).await;
    }
    Err(format!(
        "mock pine worker {address} never started listening"
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::process::GrpcPineReadinessProbe;
    use crate::process::PineReadinessProbe;

    #[tokio::test]
    async fn health_probe_authenticates_and_reports_controllable_ok_flag() {
        let token = "a".repeat(32);
        let worker = spawn_mock_pine_worker("pineworker-1", Some(&token))
            .await
            .expect("spawn mock worker");
        wait_until_listening(worker.address(), Duration::from_secs(2))
            .await
            .expect("listening");

        let spec = worker.spec("pineworker-1");
        let probe = GrpcPineReadinessProbe::new(
            Some(token.clone()),
            Duration::from_secs(1),
            Duration::from_secs(1),
        )
        .expect("probe");
        let health = probe.health(&spec).await.expect("healthy");
        assert!(health.ok);
        assert_eq!(health.version, "0.0.0-mock");

        worker.set_ok(false);
        let degraded = probe.health(&spec).await.expect("reachable");
        assert!(!degraded.ok);

        // A wrong token must be rejected by the mock itself.
        let wrong_probe = GrpcPineReadinessProbe::new(
            Some("b".repeat(32)),
            Duration::from_secs(1),
            Duration::from_secs(1),
        )
        .expect("probe");
        assert!(wrong_probe.health(&spec).await.is_err());

        worker.shutdown().await;
    }
}
