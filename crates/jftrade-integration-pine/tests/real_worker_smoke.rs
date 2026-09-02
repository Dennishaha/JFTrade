use std::collections::BTreeMap;
use std::env;
use std::net::{IpAddr, Ipv4Addr, TcpListener};
use std::path::PathBuf;
use std::time::Duration;

use jftrade_integration_pine::{
    GrpcPineExecutionPort, GrpcPineReadinessProbe, PineCandle, PineExecutionConfig, PineProcess,
    PineProcessConfig, PineReadinessPolicy, PineRunRequest, WorkerProcessSpec,
};

fn unused_loopback_port() -> u16 {
    let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).expect("reserve loopback port");
    listener.local_addr().expect("loopback address").port()
}

fn required_path(name: &str) -> PathBuf {
    let value = env::var(name).unwrap_or_else(|_| panic!("{name} must be set"));
    let path = PathBuf::from(value);
    assert!(path.is_absolute(), "{name} must be absolute");
    assert!(path.is_file(), "{name} must identify a file");
    path
}

#[tokio::test]
#[ignore = "requires the bundled native PineTS worker"]
async fn rust_client_executes_bundled_pinets_worker() {
    let bundle_path = required_path("JFTRADE_PINEWORKER_BUNDLE");
    let proto_path = required_path("JFTRADE_PINEWORKER_PROTO");
    let runtime = required_path("JFTRADE_PINEWORKER_RUNTIME");
    let token = "p".repeat(32);
    let spec = WorkerProcessSpec {
        worker_id: "pineworker-real-smoke".to_owned(),
        host: IpAddr::V4(Ipv4Addr::LOCALHOST),
        port: unused_loopback_port(),
    };
    let mut process = PineProcess::start(
        spec.clone(),
        PineProcessConfig {
            runtime,
            bundle_path,
            proto_path: Some(proto_path),
            max_message_bytes: Some(4 << 20),
            pine_ts_version: Some("0.9.31".to_owned()),
            bearer_token: Some(token.clone()),
            environment: BTreeMap::new(),
            log_path: None,
            stop_timeout: Duration::from_secs(5),
        },
    )
    .expect("start PineTS worker");

    let probe = GrpcPineReadinessProbe::new(
        Some(token.clone()),
        Duration::from_secs(1),
        Duration::from_secs(1),
    )
    .expect("readiness probe");
    let health = process
        .wait_until_ready(
            &probe,
            PineReadinessPolicy {
                timeout: Duration::from_secs(10),
                initial_retry_delay: Duration::from_millis(50),
                max_retry_delay: Duration::from_millis(250),
            },
        )
        .await
        .expect("worker readiness");
    assert_eq!(health.pine_ts_version, "0.9.31");

    let port = GrpcPineExecutionPort::new(PineExecutionConfig::for_worker(
        &spec,
        Some(token),
        Duration::from_secs(2),
        Duration::from_secs(20),
    ))
    .expect("execution port");
    let result = port
        .run(PineRunRequest {
            job_id: "real-pine-smoke".to_owned(),
            script_id: "real-pine-smoke".to_owned(),
            source: "//@version=6\nindicator(\"smoke\")\nplot(close)".to_owned(),
            symbol: "US.AAPL".to_owned(),
            timeframe: "1m".to_owned(),
            chart_type: "standard".to_owned(),
            // The production port intentionally suppresses plot payloads in
            // backtest mode. Analyze mode exercises the same native PineTS
            // process while keeping plots enabled for this protocol smoke.
            mode: "analyze".to_owned(),
            candles: vec![
                candle(1_700_000_000_000, 10.0),
                candle(1_700_000_060_000, 11.0),
                candle(1_700_000_120_000, 12.0),
            ],
            params: BTreeMap::new(),
            session_id: String::new(),
            session_operation: String::new(),
            expected_revision: 0,
        })
        .await
        .expect("execute native PineTS request");
    assert_eq!(result.job_id, "real-pine-smoke");
    assert_eq!(result.metadata.worker_id, "pineworker-real-smoke");
    assert_eq!(result.metadata.pine_ts_version, "0.9.31");
    assert!(
        !result.plots.is_empty(),
        "native PineTS should return the close plot"
    );

    process.stop().await.expect("stop PineTS worker");
}

fn candle(open_time: i64, close: f64) -> PineCandle {
    PineCandle {
        open_time,
        close_time: open_time + 60_000,
        open: close - 0.5,
        high: close + 0.5,
        low: close - 1.0,
        close,
        volume: 100.0,
    }
}
