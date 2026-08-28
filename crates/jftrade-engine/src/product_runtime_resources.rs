use std::sync::Arc;

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST, DatabaseDescriptor,
};
use serde::Serialize;

use super::ProductRuntimeConfig;
use crate::product::ProductConfig;

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
    pub production: bool,
}

pub(crate) struct ProductRuntimeState {
    resources: Vec<RuntimeResourceDescriptor>,
    production: bool,
}

impl ProductRuntimeState {
    pub(crate) fn product_only(config: &ProductConfig) -> Arc<Self> {
        Arc::new(Self {
            resources: product_resources(config),
            production: config.is_production(),
        })
    }

    pub(crate) fn configured(config: &ProductRuntimeConfig) -> Arc<Self> {
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
        if config.market_data_opend.is_some() {
            resources.push(RuntimeResourceDescriptor {
                id: "futu-opend-session".to_owned(),
                owner: "marketdata".to_owned(),
                kind: "managed-opend-session".to_owned(),
                path: "loopback OpenD API socket".to_owned(),
                initialized_by: "jftrade-engine composition root".to_owned(),
                schema_owner: "Futu OpenD protocol".to_owned(),
                close_owner: "jftrade-engine".to_owned(),
                health_provider: "OpenDSessionCoordinator".to_owned(),
                environment_override: String::new(),
                critical: false,
            });
        }
        if config.market_data_opend_task.is_some() {
            resources.push(RuntimeResourceDescriptor {
                id: "futu-opend-runtime-task".to_owned(),
                owner: "marketdata".to_owned(),
                kind: "managed-marketdata-task".to_owned(),
                path: "OpenD poll/reconnect/demand task".to_owned(),
                initialized_by: "jftrade-engine composition root".to_owned(),
                schema_owner: "Futu OpenD runtime lifecycle".to_owned(),
                close_owner: "jftrade-engine".to_owned(),
                health_provider: "OpenDSessionRuntime".to_owned(),
                environment_override: String::new(),
                critical: false,
            });
        }
        if config.market_data_opend_provider.is_some() {
            resources.push(RuntimeResourceDescriptor {
                id: "futu-opend-provider-runtime".to_owned(),
                owner: "marketdata".to_owned(),
                kind: "provider-router-opend-bridge".to_owned(),
                path: "loopback OpenD API socket".to_owned(),
                initialized_by: "jftrade-engine composition root".to_owned(),
                schema_owner: "Futu OpenD provider runtime".to_owned(),
                close_owner: "jftrade-engine".to_owned(),
                health_provider: "OpenDProviderRuntime".to_owned(),
                environment_override: String::new(),
                critical: false,
            });
        }
        Arc::new(Self {
            resources,
            production: config.product.is_production(),
        })
    }

    pub(crate) fn snapshot(&self) -> ProductRuntimeSnapshot {
        ProductRuntimeSnapshot {
            resources: self.resources.clone(),
            production: self.production,
        }
    }
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

pub(super) fn product_resources(config: &ProductConfig) -> Vec<RuntimeResourceDescriptor> {
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
