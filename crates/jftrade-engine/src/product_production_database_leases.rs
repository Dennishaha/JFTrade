//! Production database lease evidence shared by the composition root and
//! readiness projections.

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST,
};

pub const PRODUCTION_DATABASE_IDS: [&str; 9] = [
    DATABASE_WATCHLIST,
    DATABASE_STRATEGY,
    DATABASE_RESEARCH,
    DATABASE_BACKTEST_RUNS,
    DATABASE_BACKTEST,
    DATABASE_EXECUTION,
    DATABASE_ADK,
    DATABASE_ADK_SESSION,
    DATABASE_ADK_ARTIFACT,
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProductionDatabaseLeaseSnapshot {
    pub expected: usize,
    pub acquired: usize,
    pub databases: Vec<String>,
    pub status: &'static str,
}

impl ProductionDatabaseLeaseSnapshot {
    pub fn new(acquired_databases: Vec<String>) -> Self {
        let expected = PRODUCTION_DATABASE_IDS.len();
        let acquired = acquired_databases.len();
        let status = if acquired == expected && expected > 0 {
            "acquired"
        } else if acquired == 0 {
            "none"
        } else {
            "partial"
        };
        Self {
            expected,
            acquired,
            databases: acquired_databases,
            status,
        }
    }
}

use std::path::PathBuf;
use std::sync::Arc;

use jftrade_store_sqlite::{
    ADK_ARTIFACT_PRODUCTION_PROFILE, ADK_PRODUCTION_PROFILE, ADK_SESSION_PRODUCTION_PROFILE,
    AdkArtifactStore, AdkSessionStore, AdkStore, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
    BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestRunStore,
    BacktestSyncTaskStore, EXECUTION_ORDERS_PRODUCTION_PROFILE, ExecutionOrderStore,
    RESEARCH_PRESET_PRODUCTION_PROFILE, ResearchPresetStore,
    STRATEGY_DEFINITION_PRODUCTION_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    WATCHLIST_PRODUCTION_PROFILE, WatchlistStore,
};

use crate::product::{ProductConfig, ProductError, product_data_management};

pub(crate) struct ProductionStores {
    pub watchlist_store: Arc<WatchlistStore>,
    pub strategy_def_store: Arc<StrategyDefinitionStore>,
    pub strategy_runtime_store: Arc<StrategyRuntimeStore>,
    pub research_store: Arc<ResearchPresetStore>,
    pub backtest_store: Arc<BacktestRunStore>,
    pub backtest_sync_tasks: Arc<BacktestSyncTaskStore>,
    pub backtest_market_data_store: Arc<BacktestMarketDataStore>,
    pub execution_store: Arc<ExecutionOrderStore>,
    pub adk_store: Arc<AdkStore>,
    pub adk_session_store: Arc<AdkSessionStore>,
    pub adk_artifact_store: Arc<AdkArtifactStore>,
    pub database_leases: ProductionDatabaseLeaseSnapshot,
}

pub(crate) fn open_production_stores(
    config: &ProductConfig,
) -> Result<ProductionStores, ProductError> {
    let descriptors =
        product_data_management::managed_database_runtime_descriptors(config.settings_path());
    let get_path = |key: &str| -> Result<PathBuf, ProductError> {
        descriptors
            .iter()
            .find(|d| d.id == key)
            .map(|d| PathBuf::from(&d.path))
            .ok_or_else(|| {
                ProductError::Storage(format!("missing managed database descriptor for {key}"))
            })
    };

    let mut acquired_databases = Vec::with_capacity(PRODUCTION_DATABASE_IDS.len());

    let watchlist_path = get_path(DATABASE_WATCHLIST)?;
    let watchlist_store = Arc::new(
        WatchlistStore::open_existing(&watchlist_path, WATCHLIST_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open watchlist production store: {e}")),
        )?,
    );
    acquired_databases.push(DATABASE_WATCHLIST.to_owned());

    let strategy_path = get_path(DATABASE_STRATEGY)?;
    let strategy_def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strategy_path,
            STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .map_err(|e| {
            ProductError::Storage(format!(
                "failed to open strategy definitions production store: {e}"
            ))
        })?,
    );
    let strategy_runtime_store = Arc::new(StrategyRuntimeStore::from_definition_store(
        &strategy_def_store,
    ));
    acquired_databases.push(DATABASE_STRATEGY.to_owned());

    let research_path = get_path(DATABASE_RESEARCH)?;
    let research_store = Arc::new(
        ResearchPresetStore::open_existing(&research_path, RESEARCH_PRESET_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open research preset production store: {e}"
                ))
            })?,
    );
    acquired_databases.push(DATABASE_RESEARCH.to_owned());

    let backtest_path = get_path(DATABASE_BACKTEST_RUNS)?;
    let backtest_store = Arc::new(
        BacktestRunStore::open_existing(&backtest_path, BACKTEST_RUNS_PRODUCTION_PROFILE).map_err(
            |e| {
                ProductError::Storage(format!(
                    "failed to open backtest runs production store: {e}"
                ))
            },
        )?,
    );
    let backtest_sync_tasks = Arc::new(BacktestSyncTaskStore::new(Arc::clone(&backtest_store)));
    acquired_databases.push(DATABASE_BACKTEST_RUNS.to_owned());

    let backtest_market_data_path = get_path(DATABASE_BACKTEST)?;
    let backtest_market_data_store = Arc::new(
        BacktestMarketDataStore::open_existing(
            &backtest_market_data_path,
            BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
        )
        .map_err(|e| {
            ProductError::Storage(format!(
                "failed to open backtest market-data production store: {e}"
            ))
        })?,
    );
    acquired_databases.push(DATABASE_BACKTEST.to_owned());

    let execution_path = get_path(DATABASE_EXECUTION)?;
    let execution_store = Arc::new(
        ExecutionOrderStore::open_existing(&execution_path, EXECUTION_ORDERS_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!(
                    "failed to open execution orders production store: {e}"
                ))
            })?,
    );
    acquired_databases.push(DATABASE_EXECUTION.to_owned());

    let adk_path = get_path(DATABASE_ADK)?;
    let adk_store = Arc::new(
        AdkStore::open_existing(&adk_path, ADK_PRODUCTION_PROFILE).map_err(|e| {
            ProductError::Storage(format!("failed to open ADK production store: {e}"))
        })?,
    );
    acquired_databases.push(DATABASE_ADK.to_owned());

    let adk_session_path = get_path(DATABASE_ADK_SESSION)?;
    let adk_session_store = Arc::new(
        AdkSessionStore::open_existing(&adk_session_path, ADK_SESSION_PRODUCTION_PROFILE).map_err(
            |e| ProductError::Storage(format!("failed to open ADK session production store: {e}")),
        )?,
    );
    acquired_databases.push(DATABASE_ADK_SESSION.to_owned());

    let adk_artifact_path = get_path(DATABASE_ADK_ARTIFACT)?;
    let adk_artifact_store = Arc::new(
        AdkArtifactStore::open_existing(&adk_artifact_path, ADK_ARTIFACT_PRODUCTION_PROFILE)
            .map_err(|e| {
                ProductError::Storage(format!("failed to open ADK artifact production store: {e}"))
            })?,
    );
    acquired_databases.push(DATABASE_ADK_ARTIFACT.to_owned());

    let database_leases = ProductionDatabaseLeaseSnapshot::new(acquired_databases);

    Ok(ProductionStores {
        watchlist_store,
        strategy_def_store,
        strategy_runtime_store,
        research_store,
        backtest_store,
        backtest_sync_tasks,
        backtest_market_data_store,
        execution_store,
        adk_store,
        adk_session_store,
        adk_artifact_store,
        database_leases,
    })
}
