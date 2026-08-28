//! Managed worker and helper process configurations and lifecycle functions.

use std::collections::BTreeMap;
use std::net::Ipv4Addr;
use std::path::PathBuf;
use std::time::Duration;

use jftrade_integration_marketdata_helper::{
    HelperClient, HelperClientConfig, HelperProcess, HelperProcessConfig, allocate_loopback_port,
};
use jftrade_integration_pine::{
    GrpcPineReadinessProbe, PineProcess, PineProcessConfig, PineProcessError, PineReadinessPolicy,
    WorkerHealth, WorkerProcessSpec,
};

use super::ProductRuntimeError;

#[derive(Clone, Debug)]
pub struct PineWorkerRuntimeConfig {
    pub spec: WorkerProcessSpec,
    pub process: PineProcessConfig,
    pub readiness: PineReadinessPolicy,
    pub connect_timeout: Duration,
    pub request_timeout: Duration,
}

#[derive(Clone, Debug)]
pub struct MarketDataHelperRuntimeConfig {
    pub process: HelperProcessConfig,
    pub startup_timeout: Duration,
    pub initial_retry_delay: Duration,
    pub max_retry_delay: Duration,
    pub request_timeout: Duration,
}

#[derive(Clone, Debug)]
pub struct DesktopPineRuntimeConfig {
    pub runtime_path: PathBuf,
    pub bundle_path: PathBuf,
    pub proto_path: PathBuf,
    pub bearer_token: String,
    pub worker_count: usize,
    pub log_path: Option<PathBuf>,
}

#[derive(Clone, Debug)]
pub struct DesktopMarketDataRuntimeConfig {
    pub executable: PathBuf,
    pub prefix_args: Vec<String>,
    pub environment: BTreeMap<String, String>,
    pub bearer_token: String,
    pub log_path: Option<PathBuf>,
}

#[derive(Clone, Debug, Default)]
pub struct DesktopRetainedRuntimeConfig {
    pub pine: Option<DesktopPineRuntimeConfig>,
    pub marketdata: Option<DesktopMarketDataRuntimeConfig>,
}

impl DesktopRetainedRuntimeConfig {
    pub fn from_process_env() -> Self {
        let pine = if let (Ok(bundle), Ok(proto)) = (
            std::env::var("JFTRADE_PINEWORKER_BUNDLE"),
            std::env::var("JFTRADE_PINEWORKER_PROTO"),
        ) {
            let runtime = std::env::var("JFTRADE_PINEWORKER_RUNTIME")
                .map(PathBuf::from)
                .unwrap_or_else(|_| PathBuf::from("node"));
            let worker_count = std::env::var("JFTRADE_PINEWORKER_WORKERS")
                .ok()
                .and_then(|v| v.trim().parse::<usize>().ok())
                .unwrap_or(1);
            let token = std::env::var("JFTRADE_PINEWORKER_BEARER_TOKEN")
                .unwrap_or_else(|_| "pineworker-token".to_owned());
            Some(DesktopPineRuntimeConfig {
                runtime_path: runtime,
                bundle_path: PathBuf::from(bundle),
                proto_path: PathBuf::from(proto),
                bearer_token: token,
                worker_count,
                log_path: None,
            })
        } else {
            None
        };

        let marketdata = if let Ok(sidecar) = std::env::var("JFTRADE_MARKETDATA_SIDECAR") {
            let token = std::env::var("JFTRADE_MARKETDATA_BEARER_TOKEN")
                .unwrap_or_else(|_| "marketdata-token".to_owned());
            Some(DesktopMarketDataRuntimeConfig {
                executable: PathBuf::from(sidecar),
                prefix_args: Vec::new(),
                environment: BTreeMap::new(),
                bearer_token: token,
                log_path: None,
            })
        } else {
            None
        };

        Self { pine, marketdata }
    }
}

pub(crate) async fn start_pine_worker(
    config: PineWorkerRuntimeConfig,
) -> Result<(PineProcess, WorkerHealth), PineProcessError> {
    let probe = GrpcPineReadinessProbe::new(
        config.process.bearer_token.clone(),
        config.connect_timeout,
        config.request_timeout,
    )?;
    let mut process = PineProcess::start(config.spec, config.process)?;
    let health = process.wait_until_ready(&probe, config.readiness).await?;
    Ok((process, health))
}

pub(crate) async fn start_marketdata_helper(
    config: MarketDataHelperRuntimeConfig,
) -> Result<(HelperProcess, HelperClient), ProductRuntimeError> {
    let endpoint = format!("http://{}:{}", config.process.host, config.process.port);
    let client = HelperClient::new(HelperClientConfig {
        base_url: endpoint,
        bearer_token: config.process.bearer_token.clone(),
        request_timeout: config.request_timeout,
        max_attempts: 1,
        retry_delay: config.initial_retry_delay,
    })?;
    let mut process = HelperProcess::new(config.process)?;
    process
        .start_until_ready(
            &client,
            config.startup_timeout,
            config.initial_retry_delay,
            config.max_retry_delay,
        )
        .await?;
    Ok((process, client))
}

pub(crate) fn desktop_pine_workers(
    config: DesktopPineRuntimeConfig,
) -> Result<Vec<PineWorkerRuntimeConfig>, ProductRuntimeError> {
    if config.worker_count == 0 {
        return Err(ProductRuntimeError::InvalidWorkerCount);
    }
    let mut workers = Vec::with_capacity(config.worker_count);
    for index in 0..config.worker_count {
        workers.push(PineWorkerRuntimeConfig {
            spec: WorkerProcessSpec {
                worker_id: format!("pineworker-{}", index + 1),
                host: Ipv4Addr::LOCALHOST.into(),
                port: allocate_loopback_port()?,
            },
            process: PineProcessConfig {
                runtime: config.runtime_path.clone(),
                bundle_path: config.bundle_path.clone(),
                proto_path: Some(config.proto_path.clone()),
                max_message_bytes: None,
                pine_ts_version: None,
                bearer_token: Some(config.bearer_token.clone()),
                environment: BTreeMap::new(),
                log_path: config.log_path.clone(),
                stop_timeout: Duration::from_secs(5),
            },
            readiness: PineReadinessPolicy::go_compatibility(),
            connect_timeout: Duration::from_secs(1),
            request_timeout: Duration::from_secs(3),
        });
    }
    Ok(workers)
}

pub(crate) fn desktop_marketdata_helper(
    config: DesktopMarketDataRuntimeConfig,
) -> Result<MarketDataHelperRuntimeConfig, ProductRuntimeError> {
    Ok(MarketDataHelperRuntimeConfig {
        process: HelperProcessConfig {
            executable: config.executable,
            host: Ipv4Addr::LOCALHOST.into(),
            port: allocate_loopback_port()?,
            bearer_token: Some(config.bearer_token),
            prefix_args: config.prefix_args,
            extra_args: Vec::new(),
            environment: config.environment,
            log_path: config.log_path,
            stop_timeout: Duration::from_secs(5),
        },
        startup_timeout: Duration::from_secs(15),
        initial_retry_delay: Duration::from_millis(100),
        max_retry_delay: Duration::from_secs(1),
        request_timeout: Duration::from_secs(3),
    })
}
