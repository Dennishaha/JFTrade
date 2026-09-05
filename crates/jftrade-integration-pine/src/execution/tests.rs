use super::*;

#[test]
fn close_acknowledges_the_existing_revision_without_incrementing_it() {
    for (expected, actual) in [(3, 3), (0, 3), (0, 0)] {
        let request = PineRunRequest {
            session_id: "session".to_owned(),
            session_operation: "close".to_owned(),
            expected_revision: expected,
            ..Default::default()
        };
        let result = PineRunResult {
            session_id: "session".to_owned(),
            session_revision: actual,
            ..Default::default()
        };
        assert!(validate_session_result(&request, &result).is_ok());
    }
}

#[test]
fn close_rejects_a_different_session_or_a_stale_explicit_revision() {
    let request = PineRunRequest {
        session_id: "session".to_owned(),
        session_operation: "close".to_owned(),
        expected_revision: 3,
        ..Default::default()
    };
    for (session_id, session_revision) in [("other", 3), ("session", 4)] {
        let result = PineRunResult {
            session_id: session_id.to_owned(),
            session_revision,
            ..Default::default()
        };
        assert!(validate_session_result(&request, &result).is_err());
    }
}
use std::net::Ipv4Addr;
use std::sync::{Arc, Mutex};
use tokio::sync::oneshot;
use tokio_stream::wrappers::TcpListenerStream;
use tonic::transport::Server;
use tonic::{Response, Status};

use wire::pine_worker_server::{PineWorker, PineWorkerServer};
use wire::{AnalyzeScriptRequest, AnalyzeScriptResponse, HealthCheckRequest, HealthCheckResponse};

#[derive(Clone)]
struct FixtureWorker {
    response: RunScriptResponse,
    status: Option<Status>,
    delay: Duration,
    captured: Option<Arc<Mutex<Option<RunScriptRequest>>>>,
}

#[tonic::async_trait]
impl PineWorker for FixtureWorker {
    async fn health_check(
        &self,
        _request: Request<HealthCheckRequest>,
    ) -> Result<Response<HealthCheckResponse>, Status> {
        Ok(Response::new(HealthCheckResponse::default()))
    }

    async fn analyze_script(
        &self,
        _request: Request<AnalyzeScriptRequest>,
    ) -> Result<Response<AnalyzeScriptResponse>, Status> {
        Err(Status::unimplemented("fixture"))
    }

    async fn run_script(
        &self,
        request: Request<RunScriptRequest>,
    ) -> Result<Response<RunScriptResponse>, Status> {
        if let Some(captured) = &self.captured {
            *captured.lock().expect("capture lock") = Some(request.get_ref().clone());
        }
        tokio::time::sleep(self.delay).await;
        if let Some(status) = &self.status {
            return Err(status.clone());
        }
        Ok(Response::new(self.response.clone()))
    }
}

async fn fixture_port(worker: FixtureWorker) -> (GrpcPineExecutionPort, oneshot::Sender<()>) {
    let listener = tokio::net::TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
        .await
        .expect("bind");
    let address = listener.local_addr().expect("address");
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    tokio::spawn(async move {
        Server::builder()
            .add_service(PineWorkerServer::new(worker))
            .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                let _ = shutdown_rx.await;
            })
            .await
            .expect("fixture server");
    });
    let port = GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: format!("http://{address}"),
        bearer_token: None,
        ..PineExecutionConfig::default()
    })
    .expect("port");
    (port, shutdown_tx)
}

fn request() -> PineRunRequest {
    PineRunRequest {
        job_id: "job-1".to_owned(),
        script_id: "script-1".to_owned(),
        source: "//@version=6\nstrategy(\"x\")".to_owned(),
        symbol: "US.AAPL".to_owned(),
        timeframe: "1m".to_owned(),
        chart_type: "standard".to_owned(),
        mode: "backtest".to_owned(),
        candles: vec![PineCandle {
            open_time: 1_700_000_000_000,
            close_time: 1_700_000_060_000,
            open: 10.0,
            high: 12.0,
            low: 9.0,
            close: 11.0,
            volume: 100.0,
        }],
        params: BTreeMap::new(),
        session_id: String::new(),
        session_operation: String::new(),
        expected_revision: 0,
    }
}

#[tokio::test]
async fn run_script_maps_binary_request_and_order_intent_response() {
    let captured = Arc::new(Mutex::new(None));
    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            plots: vec![wire::PlotOutput {
                name: "close".to_owned(),
                values: vec![11.0],
            }],
            order_intents: vec![wire::OrderIntent {
                kind: "entry".to_owned(),
                id: "entry-1".to_owned(),
                direction: "long".to_owned(),
                quantity: 1.0,
                ..wire::OrderIntent::default()
            }],
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: Some(Arc::clone(&captured)),
    };
    let (port, shutdown) = fixture_port(worker).await;
    let result = port.run(request()).await.expect("run");
    assert_eq!(result.job_id, "job-1");
    assert_eq!(result.plots[0].values, [11.0]);
    assert_eq!(result.order_intents[0].id, "entry-1");
    let captured = captured
        .lock()
        .expect("capture lock")
        .clone()
        .expect("captured request");
    let batch = captured.candles.expect("candle batch");
    assert_eq!(batch.encoding_version, CANDLE_BATCH_ENCODING_VERSION);
    assert_eq!(batch.payload.len(), CANDLE_BATCH_RECORD_BYTES);
    assert_eq!(
        i64::from_le_bytes(batch.payload[..8].try_into().expect("open time")),
        1_700_000_000_000
    );
    let _ = shutdown.send(());
}

#[tokio::test]
async fn run_script_maps_remote_unavailable_timeout_and_cancellation() {
    let unavailable = FixtureWorker {
        response: RunScriptResponse::default(),
        status: Some(Status::unavailable("worker offline")),
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port(unavailable).await;
    assert!(matches!(
        port.run(request()).await,
        Err(PineExecutionError::Unavailable(_))
    ));
    let _ = shutdown.send(());

    let timeout_worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::from_millis(50),
        captured: None,
    };
    let listener = tokio::net::TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
        .await
        .expect("bind");
    let address = listener.local_addr().expect("address");
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    tokio::spawn(async move {
        Server::builder()
            .add_service(PineWorkerServer::new(timeout_worker))
            .serve_with_incoming_shutdown(TcpListenerStream::new(listener), async {
                let _ = shutdown_rx.await;
            })
            .await
            .expect("fixture server");
    });
    let port = GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: format!("http://{address}"),
        request_timeout: Duration::from_millis(5),
        ..PineExecutionConfig::default()
    })
    .expect("port");
    let timeout_result = port.run(request()).await;
    assert!(
        matches!(timeout_result, Err(PineExecutionError::Timeout)),
        "result={timeout_result:?}"
    );
    let (cancel_tx, cancel_rx) = oneshot::channel();
    let cancel_task = tokio::spawn(async move {
        port.run_with_cancellation(request(), async move {
            let _ = cancel_rx.await;
        })
        .await
    });
    let _ = cancel_tx.send(());
    assert!(matches!(
        cancel_task.await.expect("join"),
        Err(PineExecutionError::Cancelled)
    ));
    let _ = shutdown_tx.send(());
}

#[tokio::test]
async fn run_script_rejects_worker_error_and_identity_mismatch() {
    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            error: "script failed".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port(worker).await;
    assert!(matches!(
        port.run(request()).await,
        Err(PineExecutionError::Remote(message)) if message == "script failed"
    ));
    let _ = shutdown.send(());

    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "other-job".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port(worker).await;
    assert!(matches!(
        port.run(request()).await,
        Err(PineExecutionError::InvalidResponse(message)) if message.contains("does not match")
    ));
    let _ = shutdown.send(());
}

#[test]
fn request_validation_rejects_oversized_source_and_invalid_candle() {
    let mut request = request();
    request.source = "x".repeat(DEFAULT_MAX_SOURCE_BYTES + 1);
    let port = GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: "http://127.0.0.1:50051".to_owned(),
        ..PineExecutionConfig::default()
    })
    .expect("port");
    let error = port.request_to_proto(&request).expect_err("source limit");
    assert!(error.to_string().contains("source bytes exceed limit"));
    request.source = "source".to_owned();
    request.candles[0].high = 8.0;
    let error = port.request_to_proto(&request).expect_err("candle range");
    assert!(error.to_string().contains("high is below low"));
}

#[test]
fn endpoint_and_token_boundaries_fail_closed() {
    assert!(matches!(
        GrpcPineExecutionPort::new(PineExecutionConfig {
            endpoint: "https://127.0.0.1:50051".to_owned(),
            ..PineExecutionConfig::default()
        }),
        Err(PineExecutionError::InvalidEndpoint(_))
    ));
    assert!(matches!(
        GrpcPineExecutionPort::new(PineExecutionConfig {
            endpoint: "http://127.0.0.1:50051".to_owned(),
            bearer_token: Some("short".to_owned()),
            ..PineExecutionConfig::default()
        }),
        Err(PineExecutionError::WeakToken)
    ));
}
