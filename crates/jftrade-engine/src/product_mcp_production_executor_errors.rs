use crate::product::product_execution_write_port::ExecutionWritePortError;
use crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPortError;
use crate::product::{
    AlertSnapshotError, BacktestReadSnapshotError, BacktestSyncReadSnapshotError,
    BrokerReadSnapshotError, ExecutionReadSnapshotError, MarketDataCatalogReadSnapshotError,
    MarketDataDerivativeReadSnapshotError, MarketDataNewsSearchReadSnapshotError,
    MarketDataOptionsReadSnapshotError, MarketDataPredictionReadSnapshotError,
    MarketDataProviderReadSnapshotError, MarketDataQuoteReadSnapshotError, PluginSnapshotError,
    PortfolioSnapshotError, RemoteWatchlistSnapshotError, ResearchReadSnapshotError,
    StrategyDefinitionSnapshotError, StrategyReadSnapshotError, SystemReadSnapshotError,
    WatchlistReadSnapshotError,
};
use jftrade_research::ScreenCatalogError;

use super::McpToolFailure;

pub(super) fn system_error(error: SystemReadSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("SYSTEM_READ_UNAVAILABLE", error.to_string())
}

pub(super) fn provider_error(error: MarketDataProviderReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataProviderReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_PROVIDER_UNAVAILABLE", message)
        }
        MarketDataProviderReadSnapshotError::Failed { code, message } => {
            McpToolFailure::failed(502, code, message)
        }
    }
}

pub(super) fn alert_error(error: AlertSnapshotError) -> McpToolFailure {
    match error {
        AlertSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("ALERTS_UNAVAILABLE", message)
        }
        AlertSnapshotError::CapabilityUnavailable(message) => {
            McpToolFailure::failed(409, "BROKER_CAPABILITY_UNAVAILABLE", message)
        }
        AlertSnapshotError::Provider {
            status: Some(status),
            message,
        } if (400..500).contains(&status) => {
            McpToolFailure::failed(status, "PROVIDER_REQUEST_FAILED", message)
        }
        AlertSnapshotError::Provider { message, .. } => {
            McpToolFailure::failed(502, "BROKER_FEATURE_FAILED", message)
        }
    }
}

pub(super) fn research_error(error: ResearchReadSnapshotError) -> McpToolFailure {
    match error {
        ResearchReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("RESEARCH_UNAVAILABLE", message)
        }
        ResearchReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        ResearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => McpToolFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

pub(super) fn news_search_error(error: MarketDataNewsSearchReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataNewsSearchReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_NEWS_SEARCH_UNAVAILABLE", message)
        }
        MarketDataNewsSearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => McpToolFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

pub(super) fn screen_catalog_error(error: ScreenCatalogError) -> McpToolFailure {
    match error {
        ScreenCatalogError::UnsupportedFutuMarket
        | ScreenCatalogError::UnsupportedEmbeddedMarket(_) => {
            McpToolFailure::invalid(error.to_string())
        }
        ScreenCatalogError::BrokerUnavailable(_) => {
            McpToolFailure::failed(409, "BROKER_CAPABILITY_UNAVAILABLE", error.to_string())
        }
        ScreenCatalogError::FixtureInvalid(_) => {
            McpToolFailure::failed(500, "RESEARCH_SCREEN_CATALOG_FAILED", error.to_string())
        }
    }
}

pub(super) fn catalog_error(error: MarketDataCatalogReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataCatalogReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_CATALOG_UNAVAILABLE", message)
        }
        MarketDataCatalogReadSnapshotError::Invalid { code, message } => {
            McpToolFailure::failed(400, code, message)
        }
        MarketDataCatalogReadSnapshotError::Failed {
            status,
            code,
            message,
        } => McpToolFailure::failed(status, code, message),
    }
}

pub(crate) fn quote_error(error: MarketDataQuoteReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataQuoteReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_QUOTE_READ_UNAVAILABLE", message)
        }
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => McpToolFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

pub(crate) fn provider_actions_error(error: MarketDataProviderActionsPortError) -> McpToolFailure {
    match error {
        MarketDataProviderActionsPortError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE", message)
        }
        MarketDataProviderActionsPortError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => McpToolFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

pub(super) fn derivative_error(error: MarketDataDerivativeReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataDerivativeReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_DERIVATIVE_UNAVAILABLE", message)
        }
        MarketDataDerivativeReadSnapshotError::Failed {
            status,
            code,
            message,
        } => McpToolFailure::failed(status, code, message),
    }
}

pub(super) fn options_error(error: MarketDataOptionsReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataOptionsReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_OPTIONS_UNAVAILABLE", message)
        }
        MarketDataOptionsReadSnapshotError::Failed {
            status,
            code,
            message,
        } => McpToolFailure::failed(status, code, message),
    }
}

pub(super) fn prediction_error(error: MarketDataPredictionReadSnapshotError) -> McpToolFailure {
    match error {
        MarketDataPredictionReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        MarketDataPredictionReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("MARKET_DATA_PREDICTION_READ_UNAVAILABLE", message)
        }
        MarketDataPredictionReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => McpToolFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

pub(super) fn watchlist_error(error: WatchlistReadSnapshotError) -> McpToolFailure {
    match error {
        WatchlistReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("WATCHLIST_UNAVAILABLE", message)
        }
        WatchlistReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        WatchlistReadSnapshotError::NotFound => McpToolFailure::failed(
            404,
            "WATCHLIST_NOT_FOUND",
            "watchlist resource was not found",
        ),
    }
}

pub(super) fn plugin_error(error: PluginSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("PLUGINS_UNAVAILABLE", error.to_string())
}

pub(super) fn remote_watchlist_error(error: RemoteWatchlistSnapshotError) -> McpToolFailure {
    match error {
        RemoteWatchlistSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        RemoteWatchlistSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("REMOTE_WATCHLIST_UNAVAILABLE", message)
        }
    }
}

pub(super) fn strategy_definition_error(error: StrategyDefinitionSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("STRATEGY_DEFINITIONS_UNAVAILABLE", error.to_string())
}

pub(super) fn strategy_read_error(error: StrategyReadSnapshotError) -> McpToolFailure {
    match error {
        StrategyReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        StrategyReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("STRATEGY_ACTIVITY_UNAVAILABLE", message)
        }
    }
}

pub(super) fn portfolio_error(error: PortfolioSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("PORTFOLIO_UNAVAILABLE", error.to_string())
}

pub(super) fn broker_error(error: BrokerReadSnapshotError) -> McpToolFailure {
    match error {
        BrokerReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("BROKER_UNAVAILABLE", message)
        }
        BrokerReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
    }
}

pub(super) fn backtest_error(error: BacktestReadSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("BACKTEST_RUNS_UNAVAILABLE", error.to_string())
}

pub(super) fn backtest_sync_error(error: BacktestSyncReadSnapshotError) -> McpToolFailure {
    McpToolFailure::unavailable("BACKTEST_SYNC_UNAVAILABLE", error.to_string())
}

pub(super) fn execution_error(error: ExecutionReadSnapshotError) -> McpToolFailure {
    match error {
        ExecutionReadSnapshotError::Unavailable(message) => {
            McpToolFailure::unavailable("EXECUTION_UNAVAILABLE", message)
        }
        ExecutionReadSnapshotError::Invalid(message) => McpToolFailure::invalid(message),
        ExecutionReadSnapshotError::NotFound => {
            McpToolFailure::failed(404, "ORDER_NOT_FOUND", "execution order was not found")
        }
        ExecutionReadSnapshotError::Failed { code, message } => {
            McpToolFailure::failed(500, code, message)
        }
    }
}

pub(super) fn execution_write_error(error: ExecutionWritePortError) -> McpToolFailure {
    match error {
        ExecutionWritePortError::Unavailable(message) => {
            McpToolFailure::unavailable("EXECUTION_WRITE_UNAVAILABLE", message)
        }
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => McpToolFailure::failed(status, code, message),
    }
}
