use std::collections::BTreeMap;
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

/// Go declares the runtime inventory as `settings` -> critical resources ->
/// optional resources, so the projection keeps the same grouping instead of
/// exposing raw database order.
const CRITICAL_DATABASE_RESOURCES: [&str; 6] = [
    "backtest-kline-db",
    "backtest-run-db",
    "strategy-runtime-db",
    "execution-orders-db",
    "watchlist-db",
    "research-db",
];

const ASSISTANT_DATABASE_RESOURCES: [&str; 3] = ["adk-db", "adk-session-db", "adk-artifact-db"];

pub(super) fn product_resources(config: &ProductConfig) -> Vec<RuntimeResourceDescriptor> {
    let settings_path = config.settings_path();
    let mut managed = managed_databases_by_id(settings_path);
    let mut resources = vec![settings_resource(
        settings_path.to_string_lossy().into_owned(),
    )];

    push_managed(&mut resources, &mut managed, "backtest-kline-db");
    push_managed(&mut resources, &mut managed, "backtest-run-db");
    let strategy_path = resources
        .iter()
        .find(|resource| resource.id == "strategy-runtime-db")
        .map(|resource| resource.path.clone())
        .or_else(|| managed.get("strategy-runtime-db").map(|r| r.path.clone()))
        .unwrap_or_default();
    resources.push(strategy_table_resource(
        "strategy-catalog",
        "strategy catalog tables",
        &strategy_path,
    ));
    resources.push(strategy_table_resource(
        "strategy-designs",
        "strategy design tables",
        &strategy_path,
    ));
    push_managed(&mut resources, &mut managed, "strategy-runtime-db");
    for id in CRITICAL_DATABASE_RESOURCES {
        push_managed(&mut resources, &mut managed, id);
    }
    resources.push(real_trade_control_resource(
        config
            .real_trade_control_path()
            .to_string_lossy()
            .into_owned(),
    ));
    for id in ASSISTANT_DATABASE_RESOURCES {
        push_managed(&mut resources, &mut managed, id);
    }
    resources.extend(optional_path_resources(settings_path));
    // A future managed database keeps its descriptor instead of vanishing
    // from the projection when it is not in the Go-derived grouping above.
    resources.extend(managed.into_values());
    resources
}

fn managed_databases_by_id(
    settings_path: &std::path::Path,
) -> BTreeMap<String, RuntimeResourceDescriptor> {
    crate::product_data_management::managed_database_runtime_descriptors(settings_path)
        .iter()
        .map(database_resource)
        .map(|descriptor| (descriptor.id.clone(), descriptor))
        .collect()
}

fn push_managed(
    resources: &mut Vec<RuntimeResourceDescriptor>,
    managed: &mut BTreeMap<String, RuntimeResourceDescriptor>,
    id: &str,
) {
    if let Some(descriptor) = managed.remove(id) {
        resources.push(descriptor);
    }
}

/// The strategy catalog and design tables share the strategy runtime database
/// but stay separate inventory entries, matching Go's `strategy-catalog` and
/// `strategy-designs` descriptors.
fn strategy_table_resource(id: &str, schema_owner: &str, path: &str) -> RuntimeResourceDescriptor {
    RuntimeResourceDescriptor {
        id: id.to_owned(),
        owner: "strategy".to_owned(),
        kind: "sqlite".to_owned(),
        path: path.to_owned(),
        initialized_by: "jftrade-engine data-management inventory".to_owned(),
        schema_owner: schema_owner.to_owned(),
        close_owner: "jftrade-store-sqlite".to_owned(),
        health_provider: "data-management/strategy".to_owned(),
        environment_override: "JFTRADE_STRATEGY_RUNTIME_DB".to_owned(),
        critical: true,
    }
}

fn optional_path_resources(settings_path: &std::path::Path) -> Vec<RuntimeResourceDescriptor> {
    let directory = settings_path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty());
    let secrets = std::env::var("JFTRADE_ADK_SECRETS")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| {
            directory.map_or_else(
                || std::path::PathBuf::from("secrets/adk-secrets.json"),
                |parent| parent.join("secrets").join("adk-secrets.json"),
            )
        });
    let skills = std::env::var("JFTRADE_ADK_SKILLS")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| {
            directory.map_or_else(
                || std::path::PathBuf::from("skills"),
                |parent| parent.join("skills"),
            )
        });
    let calendars =
        crate::product::product_production_ports::product_production_calendar::exchange_calendar_snapshot_root(settings_path);
    let plugins = directory.map_or_else(
        || std::path::PathBuf::from("plugins"),
        |parent| parent.join("plugins"),
    );
    vec![
        RuntimeResourceDescriptor {
            id: "adk-secrets".to_owned(),
            owner: "assistant/admin".to_owned(),
            kind: "json-file".to_owned(),
            path: secrets.to_string_lossy().into_owned(),
            initialized_by: "jftrade-engine ADK projection".to_owned(),
            schema_owner: "adk secrets store".to_owned(),
            close_owner: "jftrade-engine".to_owned(),
            health_provider: "system.runtime-dependencies/adk".to_owned(),
            environment_override: "JFTRADE_ADK_SECRETS".to_owned(),
            critical: false,
        },
        RuntimeResourceDescriptor {
            id: "adk-skills-dir".to_owned(),
            owner: "assistant/admin".to_owned(),
            kind: "directory".to_owned(),
            path: skills.to_string_lossy().into_owned(),
            initialized_by: "jftrade-engine ADK mutation port".to_owned(),
            schema_owner: "filesystem".to_owned(),
            close_owner: "jftrade-engine".to_owned(),
            health_provider: "system.runtime-dependencies/adk".to_owned(),
            environment_override: "JFTRADE_ADK_SKILLS".to_owned(),
            critical: false,
        },
        RuntimeResourceDescriptor {
            id: "exchange-calendar-dir".to_owned(),
            owner: "system/exchange-calendar".to_owned(),
            kind: "directory".to_owned(),
            path: calendars.to_string_lossy().into_owned(),
            initialized_by: "jftrade-calendar snapshot store".to_owned(),
            schema_owner: "exchange calendar store".to_owned(),
            close_owner: "jftrade-engine".to_owned(),
            health_provider: "system.exchange-calendars".to_owned(),
            environment_override: "JFTRADE_EXCHANGE_CALENDAR_DIR".to_owned(),
            critical: false,
        },
        RuntimeResourceDescriptor {
            id: "strategy-plugin-dir".to_owned(),
            owner: "strategy".to_owned(),
            kind: "directory".to_owned(),
            path: plugins.to_string_lossy().into_owned(),
            initialized_by: "jftrade-engine production plugin port".to_owned(),
            schema_owner: "filesystem".to_owned(),
            close_owner: "jftrade-engine".to_owned(),
            health_provider: "strategy plugin catalog".to_owned(),
            environment_override: String::new(),
            critical: false,
        },
    ]
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

#[cfg(test)]
#[path = "product_runtime_resources_tests.rs"]
mod tests;
