//! Original Pine wire inputs through the encoding owner and a joined gRPC fixture.

use super::*;
use crate::{GrpcPineReadinessProbe, PineReadinessProbe, WorkerProcessSpec};
use std::sync::atomic::{AtomicUsize, Ordering};

#[derive(Clone)]
struct RpcWorker {
    response: RunScriptResponse,
    status: Option<Status>,
    health: Arc<Mutex<HealthCheckResponse>>,
    calls: Arc<AtomicUsize>,
}

impl Default for RpcWorker {
    fn default() -> Self {
        Self {
            response: RunScriptResponse {
                metadata: Some(wire::WorkerMetadata {
                    duration_ms: 1,
                    ..Default::default()
                }),
                ..Default::default()
            },
            status: None,
            health: Arc::new(Mutex::new(HealthCheckResponse {
                ok: true,
                worker_id: "worker-1".into(),
                version: "0.1.0".into(),
                pinets_version: "pinets".into(),
                capabilities: vec!["run".into()],
            })),
            calls: Arc::new(AtomicUsize::new(0)),
        }
    }
}

#[tonic::async_trait]
impl PineWorker for RpcWorker {
    async fn health_check(
        &self,
        _: Request<HealthCheckRequest>,
    ) -> Result<Response<HealthCheckResponse>, Status> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        if let Some(status) = &self.status {
            return Err(status.clone());
        }
        Ok(Response::new(self.health.lock().expect("health").clone()))
    }

    async fn analyze_script(
        &self,
        _: Request<AnalyzeScriptRequest>,
    ) -> Result<Response<AnalyzeScriptResponse>, Status> {
        Err(Status::unimplemented("fixture"))
    }

    async fn run_script(
        &self,
        _: Request<RunScriptRequest>,
    ) -> Result<Response<RunScriptResponse>, Status> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        if let Some(status) = &self.status {
            return Err(status.clone());
        }
        Ok(Response::new(self.response.clone()))
    }
}

struct RpcServer {
    address: std::net::SocketAddr,
    stop: oneshot::Sender<()>,
    task: tokio::task::JoinHandle<()>,
}

impl RpcServer {
    async fn start(worker: RpcWorker) -> Self {
        let listener = tokio::net::TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
            .await
            .expect("listener");
        let address = listener.local_addr().expect("address");
        let (stop, stopped) = oneshot::channel();
        let task = tokio::spawn(async move {
            Server::builder()
                .add_service(PineWorkerServer::new(worker))
                .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                    let _ = stopped.await;
                })
                .await
                .expect("server");
        });
        Self {
            address,
            stop,
            task,
        }
    }

    fn execution(&self) -> GrpcPineExecutionPort {
        GrpcPineExecutionPort::new(PineExecutionConfig {
            endpoint: format!("http://{}", self.address),
            ..Default::default()
        })
        .expect("execution port")
    }

    fn spec(&self) -> WorkerProcessSpec {
        WorkerProcessSpec {
            worker_id: "worker-1".into(),
            host: self.address.ip(),
            port: self.address.port(),
        }
    }

    async fn shutdown(self) {
        let _ = self.stop.send(());
        tokio::time::timeout(Duration::from_secs(2), self.task)
            .await
            .expect("bounded shutdown")
            .expect("server joined");
    }
}

fn probe() -> GrpcPineReadinessProbe {
    GrpcPineReadinessProbe::new(None, Duration::from_secs(1), Duration::from_secs(1))
        .expect("probe")
}

fn reference_request() -> PineRunRequest {
    let mut input = request();
    input.source = "//@version=6\nindicator(\"worker smoke\")\nplot(close, \"close\")".into();
    input.timeframe = "1".into();
    input.chart_type.clear();
    input.params.insert("threshold".into(), "10".into());
    input
}

// Parity: go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:154 TestCandleBatchEncodingGoldenVector
#[test]
fn candle_batch_encoding_matches_original_signed_time_and_float_vector() {
    // The reference calls the encoder directly, independently of request validation.
    let batch = candles_to_proto(&[PineCandle {
        open_time: -2,
        close_time: 3,
        open: 1.5,
        high: 2.5,
        low: -0.5,
        close: 2.0,
        volume: 0.0,
    }]);
    let hex = batch
        .payload
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    assert_eq!(
        hex,
        "feffffffffffffff0300000000000000000000000000f83f0000000000000440000000000000e0bf00000000000000400000000000000000"
    );
    assert_eq!(batch.encoding_version, 1);
    assert_eq!(batch.payload.len(), 56);
}

// Parity: go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:66 TestWorkerConfigAndCandleTimeBoundaries
#[tokio::test]
async fn original_candle_time_errors_reject_before_rpc_and_keep_indexed_diagnostics() {
    let worker = RpcWorker::default();
    let calls = worker.calls.clone();
    let server = RpcServer::start(worker).await;
    let port = server.execution();
    for (open_time, close_time, message) in [
        (0, 0, "open time is required"),
        (2, 1, "close time is before open time"),
    ] {
        let mut input = reference_request();
        input.candles = vec![PineCandle {
            open_time,
            close_time,
            high: 1.0,
            low: 0.0,
            ..Default::default()
        }];
        assert!(
            matches!(port.run(input).await, Err(PineExecutionError::InvalidRequest(actual)) if actual == format!("candle 0: {message}"))
        );
        assert_eq!(calls.load(Ordering::SeqCst), 0);
    }
    server.shutdown().await;
}

// Parity: go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:79 TestClientDefaultsAndResponseIdentityBoundaries
#[tokio::test]
async fn rpc_client_defaults_timeout_and_fills_empty_job_identity_but_rejects_mismatch() {
    assert_eq!(
        PineExecutionConfig::default().request_timeout,
        Duration::from_secs(30)
    );
    let server = RpcServer::start(RpcWorker::default()).await;
    let response = server
        .execution()
        .run(reference_request())
        .await
        .expect("empty response identity fallback");
    assert_eq!(response.job_id, "job-1");
    assert_eq!(response.metadata.duration, Duration::from_millis(1));
    server.shutdown().await;
    let server = RpcServer::start(RpcWorker {
        response: RunScriptResponse {
            job_id: "other-job".into(),
            ..Default::default()
        },
        ..Default::default()
    })
    .await;
    assert!(
        matches!(server.execution().run(reference_request()).await, Err(PineExecutionError::InvalidResponse(message)) if message == "response job id \"other-job\" does not match request \"job-1\"")
    );
    server.shutdown().await;
}

// Parity: go:452dea11:pkg/strategy/pineworker/runtime_boundaries_test.go:107 TestGRPCTransportPropagatesRPCFailures
#[tokio::test]
async fn rpc_run_and_health_propagate_worker_unavailability() {
    let worker = RpcWorker {
        status: Some(Status::unavailable("worker rpc unavailable")),
        ..Default::default()
    };
    let calls = worker.calls.clone();
    let server = RpcServer::start(worker).await;
    assert!(
        matches!(server.execution().run(reference_request()).await, Err(PineExecutionError::Unavailable(message)) if message == "worker rpc unavailable")
    );
    let error = probe()
        .health(&server.spec())
        .await
        .expect_err("health RPC failure");
    assert!(error.contains("worker rpc unavailable"), "{error}");
    assert_eq!(calls.load(Ordering::SeqCst), 2);
    server.shutdown().await;
}

// Parity: go:452dea11:pkg/strategy/pineworker/proto_mapping_test.go:197 TestHealthFromProtoCopiesCapabilities
#[tokio::test]
async fn rpc_health_copies_original_capabilities_and_server_mutation_does_not_change_result() {
    let worker = RpcWorker::default();
    let original = worker.health.clone();
    let server = RpcServer::start(worker).await;
    let health = probe()
        .health(&server.spec())
        .await
        .expect("original health");
    original.lock().expect("source health").capabilities[0] = "mutated".into();
    assert!(health.ok);
    assert_eq!(health.capabilities, ["run"]);
    assert_eq!(health.version, "0.1.0");
    assert_eq!(health.pine_ts_version, "pinets");
    let changed = probe()
        .health(&server.spec())
        .await
        .expect("changed health");
    assert_eq!(changed.capabilities, ["mutated"]);
    assert_eq!(health.capabilities, ["run"]);
    server.shutdown().await;
}
