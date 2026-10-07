use super::*;

#[path = "protocol_parity_tests.rs"]
mod protocol_parity_tests;

#[test]
fn worker_chart_type_normalization_matches_the_pkg_chart_table() {
    // Parity: go:452dea11:pkg/chart/chart_type_test.go:5 TestNormalizeChartType
    for (value, want) in [
        ("", "standard"),
        ("standard", "standard"),
        ("  HEIKINASHI ", "heikinashi"),
        ("renko", "standard"),
    ] {
        assert_eq!(
            normalize_chart_type(value),
            want,
            "NormalizeChartType({value:?})"
        );
    }
}

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
    fixture_port_with_limit(worker, DEFAULT_MAX_MESSAGE_BYTES).await
}

async fn fixture_port_with_limit(
    worker: FixtureWorker,
    max_message_bytes: usize,
) -> (GrpcPineExecutionPort, oneshot::Sender<()>) {
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
        max_message_bytes,
        ..PineExecutionConfig::default()
    })
    .expect("port");
    (port, shutdown_tx)
}

#[tokio::test]
async fn grpc_request_message_limit_has_exact_encoded_boundaries() {
    let request = request();
    let probe = GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: "http://127.0.0.1:50051".to_owned(),
        ..PineExecutionConfig::default()
    })
    .expect("probe port");
    let encoded_len = probe
        .request_to_proto(&request)
        .expect("request encoding")
        .encoded_len();
    assert!(encoded_len > 1);

    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port_with_limit(worker, encoded_len).await;
    assert_eq!(
        port.run(request.clone()).await.expect("exact limit").job_id,
        "job-1"
    );
    let _ = shutdown.send(());

    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port_with_limit(worker, encoded_len - 1).await;
    assert!(matches!(
        port.run(request.clone()).await,
        Err(PineExecutionError::InvalidRequest(message))
            if message.contains("encoded request exceeds")
    ));
    let _ = shutdown.send(());

    let worker = FixtureWorker {
        response: RunScriptResponse {
            job_id: "job-1".to_owned(),
            ..RunScriptResponse::default()
        },
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port_with_limit(worker, encoded_len + 1).await;
    assert_eq!(
        port.run(request).await.expect("limit plus one").job_id,
        "job-1"
    );
    let _ = shutdown.send(());
}

#[tokio::test]
async fn grpc_response_message_limit_keeps_multibyte_visuals_and_order_intents_atomic() {
    let response = RunScriptResponse {
        job_id: "job-1".to_owned(),
        order_intents: vec![wire::OrderIntent {
            kind: "entry".to_owned(),
            id: "intent-完整-🌟".to_owned(),
            direction: "long".to_owned(),
            quantity: 3.0,
            has_quantity: true,
            ..wire::OrderIntent::default()
        }],
        visual_outputs: vec![wire::VisualOutput {
            kind: "line".to_owned(),
            name: "收盘线-📈".to_owned(),
            payload_json: format!(
                r#"{{"label":"多字节图形🌏","points":[{}]}}"#,
                (0..128)
                    .map(|index| index.to_string())
                    .collect::<Vec<_>>()
                    .join(",")
            ),
        }],
        logs: vec!["保持完整，不截断".to_owned()],
        ..RunScriptResponse::default()
    };
    let response_len = response.encoded_len();
    let request_len = GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: "http://127.0.0.1:50051".to_owned(),
        ..PineExecutionConfig::default()
    })
    .expect("probe port")
    .request_to_proto(&request())
    .expect("request encoding")
    .encoded_len();
    assert!(response_len > request_len);

    let worker = FixtureWorker {
        response: response.clone(),
        status: None,
        delay: Duration::ZERO,
        captured: None,
    };
    let (port, shutdown) = fixture_port_with_limit(worker, response_len - 1).await;
    let boundary_error = port.run(request()).await;
    assert!(matches!(
        boundary_error,
        Err(PineExecutionError::Transport(message))
            if message.contains("decoded message length too large")
                && message.contains("limit is")
    ));
    let _ = shutdown.send(());

    for max_message_bytes in [response_len, response_len + 1] {
        let worker = FixtureWorker {
            response: response.clone(),
            status: None,
            delay: Duration::ZERO,
            captured: None,
        };
        let (port, shutdown) = fixture_port_with_limit(worker, max_message_bytes).await;
        let result = port.run(request()).await.expect("response at boundary");
        assert_eq!(result.order_intents.len(), 1);
        assert_eq!(result.order_intents[0].id, "intent-完整-🌟");
        assert_eq!(result.order_intents[0].quantity, 3.0);
        assert_eq!(result.visual_outputs.len(), 1);
        assert_eq!(result.visual_outputs[0].name, "收盘线-📈");
        assert!(
            result.visual_outputs[0]
                .payload_json
                .contains("多字节图形🌏")
        );
        let _ = shutdown.send(());
    }
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
// Parity: go:452dea11:pkg/strategy/pineworker/client_test.go:41 TestClientRunScriptRejectsInvalidRequestBeforeTransport
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
    request.candles.clear();
    let error = port
        .request_to_proto(&request)
        .expect_err("missing candles");
    assert!(error.to_string().contains("candles are required"));
    request.candles.push(PineCandle {
        open_time: 1_700_000_000_000,
        close_time: 1_700_000_060_000,
        open: 10.0,
        high: 12.0,
        low: 9.0,
        close: 11.0,
        volume: 100.0,
    });
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

type RequestMutation = fn(&mut PineRunRequest);
type ValidationCase = (&'static str, RequestMutation, &'static str);
type SessionCase = (&'static str, &'static str, RequestMutation);

fn validation_port(max_candles: usize) -> GrpcPineExecutionPort {
    GrpcPineExecutionPort::new(PineExecutionConfig {
        endpoint: "http://127.0.0.1:50051".to_owned(),
        max_candles,
        ..PineExecutionConfig::default()
    })
    .expect("validation port")
}

/// Parity: go:452dea11:pkg/strategy/pineworker/types_test.go:41 TestValidateRunScriptRequest
#[test]
fn request_validation_rejects_every_incomplete_or_inconsistent_field() {
    let port = validation_port(0);
    port.request_to_proto(&request()).expect("valid request");

    let cases: [ValidationCase; 9] = [
        (
            "missing job",
            |request| request.job_id.clear(),
            "job id is required",
        ),
        (
            "missing source",
            |request| request.source.clear(),
            "source is required",
        ),
        (
            "missing symbol",
            |request| request.symbol.clear(),
            "symbol is required",
        ),
        (
            "missing timeframe",
            |request| request.timeframe.clear(),
            "timeframe is required",
        ),
        (
            "unsupported mode",
            |request| request.mode = "scan".to_owned(),
            "unsupported pine worker mode",
        ),
        (
            "missing candles",
            |request| request.candles.clear(),
            "candles are required",
        ),
        (
            "high below low",
            |request| request.candles[0].high = 8.0,
            "high is below low",
        ),
        (
            "open outside range",
            |request| request.candles[0].open = 99.0,
            "open is outside high/low range",
        ),
        (
            "negative volume",
            |request| request.candles[0].volume = -1.0,
            "volume is negative",
        ),
    ];
    for (name, mutate, message) in cases {
        let mut invalid = request();
        mutate(&mut invalid);
        let error = port.request_to_proto(&invalid).expect_err(name).to_string();
        assert!(
            error.contains(message),
            "{name}: error {error:?} must contain {message:?}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pineworker/types_test.go:92 TestValidateRunScriptRequestAnalyzeModeAllowsNoCandles
#[test]
fn analyze_mode_accepts_an_empty_candle_list() {
    let port = validation_port(0);
    let mut analyze = request();
    analyze.mode = "analyze".to_owned();
    analyze.candles.clear();
    port.request_to_proto(&analyze)
        .expect("analyze mode runs without candles");

    let mut backtest = request();
    backtest.candles.clear();
    let error = port
        .request_to_proto(&backtest)
        .expect_err("backtest still needs candles")
        .to_string();
    assert!(error.contains("candles are required"), "error {error:?}");
}

/// Parity: go:452dea11:pkg/strategy/pineworker/types_test.go:105 TestValidateRunScriptRequestLiveSessionContract
#[test]
fn live_session_contract_requires_identity_mode_and_revisions() {
    let port = validation_port(0);
    let mut open = request();
    open.mode = "live".to_owned();
    open.session_id = "session-1".to_owned();
    open.session_operation = "open".to_owned();
    port.request_to_proto(&open).expect("open");

    let mut append = open.clone();
    append.session_operation = "append".to_owned();
    append.expected_revision = 1;
    port.request_to_proto(&append).expect("append");

    let mut close = append.clone();
    close.session_operation = "close".to_owned();
    port.request_to_proto(&close).expect("close");

    let cases: [SessionCase; 5] = [
        (
            "unsupported operation",
            "unsupported pine worker session operation",
            |request| {
                request.session_operation = "replace".to_owned();
            },
        ),
        (
            "session requires live mode",
            "require live mode",
            |request| {
                request.mode = "backtest".to_owned();
            },
        ),
        (
            "open starts at zero",
            "requires expected revision 0",
            |request| {
                request.expected_revision = 1;
            },
        ),
        (
            "append needs a revision",
            "requires a positive expected revision",
            |request| {
                request.session_operation = "append".to_owned();
            },
        ),
        (
            "operation needs an id",
            "session id is required",
            |request| {
                request.session_id.clear();
            },
        ),
    ];
    for (name, message, mutate) in cases {
        let mut invalid = open.clone();
        mutate(&mut invalid);
        let error = port.request_to_proto(&invalid).expect_err(name).to_string();
        assert!(
            error.contains(message),
            "{name}: error {error:?} must contain {message:?}"
        );
    }
}

/// Parity: go:452dea11:pkg/strategy/pineworker/types_test.go:147 TestValidateRunScriptRequestRejectsTooManyCandles
#[test]
fn request_validation_enforces_the_candle_limit() {
    let port = validation_port(1);
    let mut two_candles = request();
    two_candles.candles.push(two_candles.candles[0].clone());
    let error = port
        .request_to_proto(&two_candles)
        .expect_err("candle limit")
        .to_string();
    assert!(error.contains("too many candles"), "error {error:?}");
    port.request_to_proto(&request()).expect("one candle fits");
}

/// Parity: go:452dea11:pkg/strategy/pineworker/types_test.go:164 TestRunScriptPayloadSizeRejectsNonFiniteCandle
#[test]
fn non_finite_candle_values_are_rejected_before_transport() {
    let port = validation_port(0);
    let mut nan_open = request();
    nan_open.candles[0].open = f64::NAN;
    let mut infinite_high = request();
    infinite_high.candles[0].high = f64::INFINITY;
    let mut negative_infinite_volume = request();
    negative_infinite_volume.candles[0].volume = f64::NEG_INFINITY;
    for invalid in [nan_open, infinite_high, negative_infinite_volume] {
        let error = port
            .request_to_proto(&invalid)
            .expect_err("non-finite candle value")
            .to_string();
        assert!(error.contains("must be finite"), "error {error:?}");
    }
}
