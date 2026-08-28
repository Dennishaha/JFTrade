use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST, DatabaseDescriptor,
};

use super::RuntimeResourceDescriptor;
use crate::product::ProductConfig;

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
