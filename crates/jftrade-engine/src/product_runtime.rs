use std::collections::BTreeMap;
use std::net::Ipv4Addr;
use std::path::PathBuf;
use std::sync::{Arc, RwLock};
use std::time::Duration;

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST, DatabaseDescriptor,
};
use jftrade_integration_marketdata_helper::{
    HelperClient, HelperClientConfig, HelperProcess, HelperProcessConfig, ProcessError,
    ProcessState, allocate_loopback_port,
};
use jftrade_integration_pine::{
    GrpcPineReadinessProbe, PineProcess, PineProcessConfig, PineProcessError, PineReadinessPolicy,
    WorkerHealth, WorkerProcessSpec,
};
use serde::Serialize;
use thiserror::Error;

use crate::product::{
    ProductConfig, ProductError, ProductHandle, start_product_with_runtime_state,
};

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
pub struct ProductRuntimeConfig {
    pub product: ProductConfig,
    pub pine_workers: Vec<PineWorkerRuntimeConfig>,
    pub marketdata_helper: Option<MarketDataHelperRuntimeConfig>,
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

impl ProductRuntimeConfig {
    pub fn desktop(
        product: ProductConfig,
        retained: DesktopRetainedRuntimeConfig,
    ) -> Result<Self, ProductRuntimeError> {
        let pine_workers = retained
            .pine
            .map(desktop_pine_workers)
            .transpose()?
            .unwrap_or_default();
        let marketdata_helper = retained
            .marketdata
            .map(desktop_marketdata_helper)
            .transpose()?;
        Ok(Self {
            product,
            pine_workers,
            marketdata_helper,
        })
    }
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeResourceDescriptor {
    pub id: String,
    pub owner: String,
    pub kind: String,
    pub path: String,
    pub initialized_by: String,
    pub schema_owner: String,
    pub close_owner: String,
    pub health_provider: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub environment_override: String,
    pub critical: bool,
}

#[derive(Clone, Debug)]
pub(crate) struct ProductRuntimeSnapshot {
    pub resources: Vec<RuntimeResourceDescriptor>,
    pub helper_state: Option<ProcessState>,
    pub last_error: Option<String>,
}

pub(crate) struct ProductRuntimeState {
    snapshot: RwLock<ProductRuntimeSnapshot>,
}

impl ProductRuntimeState {
    pub(crate) fn product_only(config: &ProductConfig) -> Arc<Self> {
        Arc::new(Self {
            snapshot: RwLock::new(ProductRuntimeSnapshot {
                resources: product_resources(config),
                helper_state: None,
                last_error: None,
            }),
        })
    }

    fn configured(config: &ProductRuntimeConfig) -> Arc<Self> {
        let mut resources = product_resources(&config.product);
        resources.extend(
            config
                .pine_workers
                .iter()
                .map(|worker| RuntimeResourceDescriptor {
                    id: worker.spec.worker_id.clone(),
                    owner: "strategy".to_owned(),
                    kind: "managed-node-process".to_owned(),
                    path: worker.process.bundle_path.to_string_lossy().into_owned(),
                    initialized_by: "jftrade-engine".to_owned(),
                    schema_owner: "workers/pineworker".to_owned(),
                    close_owner: "jftrade-engine".to_owned(),
                    health_provider: "PineWorker.HealthCheck".to_owned(),
                    environment_override: "JFTRADE_PINEWORKER_BUNDLE".to_owned(),
                    critical: false,
                }),
        );
        if let Some(helper) = &config.marketdata_helper {
            resources.push(RuntimeResourceDescriptor {
                id: "marketdata-sidecar".to_owned(),
                owner: "marketdata".to_owned(),
                kind: "managed-python-process".to_owned(),
                path: helper.process.executable.to_string_lossy().into_owned(),
                initialized_by: "jftrade-engine".to_owned(),
                schema_owner: "workers/marketdata-sidecar".to_owned(),
                close_owner: "jftrade-engine".to_owned(),
                health_provider: "marketdata-sidecar /healthz".to_owned(),
                environment_override: "JFTRADE_MARKETDATA_SIDECAR".to_owned(),
                critical: false,
            });
        }
        Arc::new(Self {
            snapshot: RwLock::new(ProductRuntimeSnapshot {
                resources,
                helper_state: config
                    .marketdata_helper
                    .as_ref()
                    .map(|_| ProcessState::Stopped),
                last_error: None,
            }),
        })
    }

    pub(crate) fn snapshot(&self) -> ProductRuntimeSnapshot {
        self.snapshot
            .read()
            .map(|snapshot| snapshot.clone())
            .unwrap_or_else(|_| ProductRuntimeSnapshot {
                resources: Vec::new(),
                helper_state: Some(ProcessState::Failed),
                last_error: Some("runtime status lock is unavailable".to_owned()),
            })
    }

    fn pine_ready(&self, health: &WorkerHealth) {
        if let Ok(mut snapshot) = self.snapshot.write()
            && health.ok
        {
            snapshot.last_error = None;
        }
    }

    fn helper_state(&self, state: ProcessState) {
        if let Ok(mut snapshot) = self.snapshot.write() {
            snapshot.helper_state = Some(state);
            if state == ProcessState::Ready {
                snapshot.last_error = None;
            }
        }
    }

    fn failed(&self, error: &impl std::fmt::Display) {
        if let Ok(mut snapshot) = self.snapshot.write() {
            snapshot.last_error = Some(error.to_string());
        }
    }
}

pub struct ProductRuntimeHandle {
    product: Option<ProductHandle>,
    pine_workers: Vec<PineProcess>,
    marketdata_helper: Option<HelperProcess>,
    state: Arc<ProductRuntimeState>,
}

impl ProductRuntimeHandle {
    pub fn startup_record(&self) -> &crate::product::ProductStartupRecord {
        self.product
            .as_ref()
            .expect("running product runtime must own its product handle")
            .startup_record()
    }

    pub async fn shutdown(mut self) -> Result<(), ProductRuntimeError> {
        let mut failures = Vec::new();
        if let Some(mut helper) = self.marketdata_helper.take() {
            self.state.helper_state(ProcessState::Stopping);
            if let Err(error) = helper.stop().await {
                failures.push(error.to_string());
                self.state.failed(&error);
            } else {
                self.state.helper_state(ProcessState::Stopped);
            }
        }
        while let Some(worker) = self.pine_workers.pop() {
            if let Err(error) = worker.stop().await {
                failures.push(error.to_string());
                self.state.failed(&error);
            }
        }
        if let Some(product) = self.product.take()
            && let Err(error) = product.shutdown().await
        {
            failures.push(error.to_string());
        }
        if failures.is_empty() {
            Ok(())
        } else {
            Err(ProductRuntimeError::Shutdown(failures.join("; ")))
        }
    }
}

impl Drop for ProductRuntimeHandle {
    fn drop(&mut self) {
        if let Some(helper) = self.marketdata_helper.as_mut() {
            helper.terminate();
        }
        for worker in self.pine_workers.iter_mut().rev() {
            worker.terminate();
        }
        drop(self.product.take());
    }
}

pub async fn start_product_runtime(
    config: ProductRuntimeConfig,
) -> Result<ProductRuntimeHandle, ProductRuntimeError> {
    let state = ProductRuntimeState::configured(&config);
    let product = start_product_with_runtime_state(config.product, Arc::clone(&state)).await?;
    let mut runtime = ProductRuntimeHandle {
        product: Some(product),
        pine_workers: Vec::new(),
        marketdata_helper: None,
        state,
    };

    for worker in config.pine_workers {
        let result = start_pine_worker(worker).await;
        match result {
            Ok((process, health)) => {
                runtime.state.pine_ready(&health);
                runtime.pine_workers.push(process);
            }
            Err(error) => {
                runtime.state.failed(&error);
                let _ = runtime.shutdown().await;
                return Err(ProductRuntimeError::Pine(error));
            }
        }
    }

    if let Some(helper) = config.marketdata_helper {
        runtime.state.helper_state(ProcessState::Starting);
        match start_marketdata_helper(helper).await {
            Ok(process) => {
                runtime.state.helper_state(ProcessState::Ready);
                runtime.marketdata_helper = Some(process);
            }
            Err(error) => {
                runtime.state.failed(&error);
                let _ = runtime.shutdown().await;
                return Err(error);
            }
        }
    }
    Ok(runtime)
}

async fn start_pine_worker(
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

async fn start_marketdata_helper(
    config: MarketDataHelperRuntimeConfig,
) -> Result<HelperProcess, ProductRuntimeError> {
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
    Ok(process)
}

fn desktop_pine_workers(
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

fn desktop_marketdata_helper(
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

fn settings_resource(path: String) -> RuntimeResourceDescriptor {
    RuntimeResourceDescriptor {
        id: "settings-file".to_owned(),
        owner: "settings".to_owned(),
        kind: "json-file".to_owned(),
        path,
        initialized_by: "jftrade-engine".to_owned(),
        schema_owner: "jftrade-settings".to_owned(),
        close_owner: "jftrade-engine".to_owned(),
        health_provider: "jftrade-store-settings-file".to_owned(),
        environment_override: "JFTRADE_SETTINGS_PATH".to_owned(),
        critical: true,
    }
}

fn product_resources(config: &ProductConfig) -> Vec<RuntimeResourceDescriptor> {
    let mut resources = vec![settings_resource(
        config.settings_path().to_string_lossy().into_owned(),
    )];
    resources.extend(
        crate::product_data_management::managed_database_runtime_descriptors(
            config.settings_path(),
        )
        .iter()
        .map(database_resource),
    );
    resources.push(real_trade_control_resource(
        config
            .real_trade_control_path()
            .to_string_lossy()
            .into_owned(),
    ));
    resources
}

fn database_resource(database: &DatabaseDescriptor) -> RuntimeResourceDescriptor {
    let (id, owner, schema_owner, health_provider, environment_override, critical) =
        match database.id.as_str() {
            DATABASE_BACKTEST => (
                "backtest-kline-db",
                "backtest",
                "pkg/backtest storage",
                "data-management/backtest",
                "JFTRADE_BACKTEST_DB",
                true,
            ),
            DATABASE_BACKTEST_RUNS => (
                "backtest-run-db",
                "backtest",
                "backtest run store",
                "data-management/backtest-runs",
                "JFTRADE_BACKTEST_RUN_DB",
                true,
            ),
            DATABASE_STRATEGY => (
                "strategy-runtime-db",
                "strategy",
                "strategy runtime store",
                "data-management/strategy",
                "JFTRADE_STRATEGY_RUNTIME_DB",
                true,
            ),
            DATABASE_EXECUTION => (
                "execution-orders-db",
                "trading",
                "execution order store",
                "data-management/execution",
                "JFTRADE_EXECUTION_ORDER_DB",
                true,
            ),
            DATABASE_ADK => (
                "adk-db",
                "assistant/runtime",
                "adk store",
                "system.runtime-dependencies/adk",
                "JFTRADE_ADK_DB",
                false,
            ),
            DATABASE_ADK_SESSION => (
                "adk-session-db",
                "assistant/runtime",
                "adk session store",
                "system.runtime-dependencies/adk",
                "JFTRADE_ADK_SESSION_DB",
                false,
            ),
            DATABASE_ADK_ARTIFACT => (
                "adk-artifact-db",
                "assistant/runtime",
                "adk artifact store",
                "system.runtime-dependencies/adk",
                "JFTRADE_ADK_SESSION_DB",
                false,
            ),
            DATABASE_WATCHLIST => (
                "watchlist-db",
                "watchlist",
                "internal/store/watchlist migrations",
                "data-management/watchlist",
                "JFTRADE_WATCHLIST_DB",
                true,
            ),
            DATABASE_RESEARCH => (
                "research-db",
                "research",
                "internal/store/research migrations",
                "data-management/research",
                "JFTRADE_RESEARCH_DB",
                true,
            ),
            _ => (
                database.id.as_str(),
                "data-management",
                "jftrade-datamanagement",
                "data-management/databases",
                "",
                false,
            ),
        };
    RuntimeResourceDescriptor {
        id: id.to_owned(),
        owner: owner.to_owned(),
        kind: "sqlite".to_owned(),
        path: database.path.clone(),
        initialized_by: "jftrade-engine data-management inventory".to_owned(),
        schema_owner: schema_owner.to_owned(),
        close_owner: "jftrade-store-sqlite".to_owned(),
        health_provider: health_provider.to_owned(),
        environment_override: environment_override.to_owned(),
        critical,
    }
}

fn real_trade_control_resource(path: String) -> RuntimeResourceDescriptor {
    RuntimeResourceDescriptor {
        id: "real-trade-control".to_owned(),
        owner: "trading".to_owned(),
        kind: "json-file".to_owned(),
        path,
        initialized_by: "jftrade-engine".to_owned(),
        schema_owner: "real-trade control plane".to_owned(),
        close_owner: "jftrade-engine".to_owned(),
        health_provider: "system.real-trade-risk".to_owned(),
        environment_override: "JFTRADE_REAL_TRADE_CONTROL_PATH".to_owned(),
        critical: true,
    }
}

#[derive(Debug, Error)]
pub enum ProductRuntimeError {
    #[error(transparent)]
    Product(#[from] ProductError),
    #[error("start PineTS worker: {0}")]
    Pine(#[source] PineProcessError),
    #[error("configure market-data helper client: {0}")]
    HelperClient(#[from] jftrade_integration_marketdata_helper::HttpAdapterError),
    #[error("manage market-data helper: {0}")]
    HelperProcess(#[from] ProcessError),
    #[error("desktop PineTS worker count must be greater than zero")]
    InvalidWorkerCount,
    #[error("stop Rust product runtime: {0}")]
    Shutdown(String),
}

#[cfg(test)]
mod tests {
    use std::net::{Ipv4Addr, SocketAddr};

    use jftrade_api::AccessPolicy;
    use tempfile::tempdir;

    use super::*;

    #[tokio::test]
    async fn product_runtime_without_optional_workers_starts_and_stops_cleanly() {
        let directory = tempdir().expect("temporary directory");
        let product = ProductConfig::new(
            SocketAddr::new(Ipv4Addr::LOCALHOST.into(), 0),
            directory.path().join("settings.json"),
            AccessPolicy::default(),
        )
        .expect("product config");
        let runtime = start_product_runtime(ProductRuntimeConfig {
            product,
            pine_workers: Vec::new(),
            marketdata_helper: None,
        })
        .await
        .expect("start runtime");
        assert_eq!(runtime.startup_record().owned_routes, 26);
        let snapshot = runtime.state.snapshot();
        assert_eq!(snapshot.resources.len(), 11);
        assert_eq!(snapshot.resources[0].id, "settings-file");
        assert_eq!(snapshot.resources[1].id, "backtest-kline-db");
        assert_eq!(snapshot.resources[9].id, "research-db");
        assert_eq!(snapshot.resources[10].id, "real-trade-control");
        assert!(
            snapshot.resources[1..10]
                .iter()
                .all(|resource| resource.kind == "sqlite")
        );
        runtime.shutdown().await.expect("shutdown");
    }
}
