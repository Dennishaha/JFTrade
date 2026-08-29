use std::collections::BTreeMap;

use serde::Deserialize;

use super::product_production_ports::{ProductionAdapterBinding, ProductionPortBundle};
use super::*;

const EXPECTED_PRODUCTION_ROUTE_COUNT: usize = 278;
const EXPECTED_PRODUCTION_ROUTE_DIGEST: &str =
    "afa112435ed280dd24d43bb4acaa0f7ca2ab45c01e4e5701efc5ce149e5b85b2";
const CANONICAL_ROUTE_LEDGER: &str =
    include_str!("../../../tests/fixtures/rust-migration/stage9/route-ownership.json");

const OPTION_EVENT_OPERATION_ADAPTERS: &[(&str, ProductionRouteAdapter)] = &[
    ("unusual", ProductionRouteAdapter::MarketDataOptionsUnusualRead),
    ("zero_dte", ProductionRouteAdapter::MarketDataOptionsZeroDteRead),
    (
        "zero_dte_contract",
        ProductionRouteAdapter::MarketDataOptionsZeroDteContractRead,
    ),
    ("earnings", ProductionRouteAdapter::MarketDataOptionsEarningsRead),
    ("seller", ProductionRouteAdapter::MarketDataOptionsSellerRead),
];

#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub(crate) enum ProductionRouteAdapter {
    AuthSessionRead,
    AuthSessionWrite,
    Settings,
    DataManagement,
    SystemCore,
    SystemRead,
    SystemOpenDWrite,
    RealTradeControlWrite,
    Calendar,
    WatchlistMemberships,
    WatchlistRead,
    WatchlistWrite,
    RemoteWatchlistRead,
    RemoteWatchlistWrite,
    StrategyDefinitionRead,
    StrategyDefinitionWrite,
    StrategyRuntimeRead,
    StrategyRuntimeWrite,
    StrategyPine,
    ResearchCatalog,
    ResearchRead,
    ResearchRankingsRead,
    ResearchIndustriesRead,
    ResearchCalendarRead,
    ResearchMacroRead,
    ResearchPresetRead,
    ResearchPresetWrite,
    ResearchScreenWrite,
    BacktestRead,
    BacktestSyncRead,
    BacktestStart,
    BacktestDelete,
    BacktestSyncStart,
    BacktestSyncCancel,
    ExecutionRead,
    ExecutionWrite,
    BrokerRead,
    BrokerWrite,
    PortfolioRead,
    MarketDataProviderRead,
    MarketDataMarketsRead,
    MarketDataSearchRead,
    MarketDataSubscriptionRead,
    MarketDataSecuritiesRead,
    MarketDataSnapshotsRead,
    MarketDataCandlesRead,
    MarketDataDepthRead,
    MarketDataTicksRead,
    MarketDataBrokerQueueRead,
    MarketDataCapitalFlowRead,
    MarketDataIntradayRead,
    MarketDataProfileRead,
    MarketDataDerivativeRead,
    MarketDataOptionsRead,
    MarketDataOptionsChainRead,
    MarketDataOptionsExpirationsRead,
    MarketDataOptionsScreenRead,
    MarketDataOptionsAnalysisRead,
    MarketDataOptionsEventsRead,
    /// Operation-level readers behind GET /market-data/options/events.
    ///
    /// The public HTTP route is intentionally shared for compatibility, but
    /// each query operation has an independent production capability.  Keep
    /// these adapters distinct so a missing reader cannot be hidden by the
    /// presence of the shared trade runtime.
    MarketDataOptionsUnusualRead,
    MarketDataOptionsZeroDteRead,
    MarketDataOptionsZeroDteContractRead,
    MarketDataOptionsEarningsRead,
    MarketDataOptionsSellerRead,
    MarketDataNewsActionsRead,
    MarketDataNewsSearchRead,
    MarketDataPredictionRead,
    MarketDataSubscriptionAcquireWrite,
    MarketDataSubscriptionReleaseWrite,
    MarketDataSubscriptionClearWrite,
    MarketDataSubscriptionHeartbeatWrite,
    MarketDataPredictionSubscriptionAcquireWrite,
    MarketDataPredictionSubscriptionReleaseWrite,
    MarketDataInstrumentsNormalizeWrite,
    MarketDataBatchSnapshotsWrite,
    MarketDataOptionsAnalysisWrite,
    MarketDataZeroDteWrite,
    MarketDataPredictionCombosWrite,
    PluginsRead,
    PluginsWrite,
    PluginGuidanceRead,
    AlertsRead,
    AlertsWrite,
    AdkTemplatesRead,
    AdkRead,
    AdkMutation,
    AdkChat,
    WebSocketLive,
}

impl ProductionRouteAdapter {
    pub(crate) const fn name(self) -> &'static str {
        match self {
            Self::AuthSessionRead => "auth-session-read",
            Self::AuthSessionWrite => "auth-session-write",
            Self::Settings => "settings",
            Self::DataManagement => "data-management",
            Self::SystemCore => "system-core",
            Self::SystemRead => "system-read",
            Self::SystemOpenDWrite => "system-opend-write",
            Self::RealTradeControlWrite => "real-trade-control-write",
            Self::Calendar => "exchange-calendar",
            Self::WatchlistMemberships => "watchlist-memberships",
            Self::WatchlistRead => "watchlist-read",
            Self::WatchlistWrite => "watchlist-write",
            Self::RemoteWatchlistRead => "remote-watchlist-read",
            Self::RemoteWatchlistWrite => "remote-watchlist-write",
            Self::StrategyDefinitionRead => "strategy-definition-read",
            Self::StrategyDefinitionWrite => "strategy-definition-write",
            Self::StrategyRuntimeRead => "strategy-runtime-read",
            Self::StrategyRuntimeWrite => "strategy-runtime-write",
            Self::StrategyPine => "strategy-pine",
            Self::ResearchCatalog => "research-catalog",
            Self::ResearchRead => "research-read",
            Self::ResearchRankingsRead => "research-rankings-read",
            Self::ResearchIndustriesRead => "research-industries-read",
            Self::ResearchCalendarRead => "research-calendar-read",
            Self::ResearchMacroRead => "research-macro-read",
            Self::ResearchPresetRead => "research-preset-read",
            Self::ResearchPresetWrite => "research-preset-write",
            Self::ResearchScreenWrite => "research-screen-write",
            Self::BacktestRead => "backtest-read",
            Self::BacktestSyncRead => "backtest-sync-read",
            Self::BacktestStart => "backtest-start",
            Self::BacktestDelete => "backtest-delete",
            Self::BacktestSyncStart => "backtest-sync-start",
            Self::BacktestSyncCancel => "backtest-sync-cancel",
            Self::ExecutionRead => "execution-read",
            Self::ExecutionWrite => "execution-write",
            Self::BrokerRead => "broker-read",
            Self::BrokerWrite => "broker-write",
            Self::PortfolioRead => "portfolio-read",
            Self::MarketDataProviderRead => "market-data-provider-read",
            Self::MarketDataMarketsRead => "market-data-markets-read",
            Self::MarketDataSearchRead => "market-data-search-read",
            Self::MarketDataSubscriptionRead => "market-data-subscription-read",
            Self::MarketDataSecuritiesRead => "market-data-securities-read",
            Self::MarketDataSnapshotsRead => "market-data-snapshots-read",
            Self::MarketDataCandlesRead => "market-data-candles-read",
            Self::MarketDataDepthRead => "market-data-depth-read",
            Self::MarketDataTicksRead => "market-data-ticks-read",
            Self::MarketDataBrokerQueueRead => "market-data-broker-queue-read",
            Self::MarketDataCapitalFlowRead => "market-data-capital-flow-read",
            Self::MarketDataIntradayRead => "market-data-intraday-read",
            Self::MarketDataProfileRead => "market-data-profile-read",
            Self::MarketDataDerivativeRead => "market-data-derivative-read",
            Self::MarketDataOptionsRead => "market-data-options-read",
            Self::MarketDataOptionsChainRead => "market-data-options-chain-read",
            Self::MarketDataOptionsExpirationsRead => "market-data-options-expirations-read",
            Self::MarketDataOptionsScreenRead => "market-data-options-screen-read",
            Self::MarketDataOptionsAnalysisRead => "market-data-options-analysis-read",
            Self::MarketDataOptionsEventsRead => "market-data-options-events-read",
            Self::MarketDataOptionsUnusualRead => "market-data-options-unusual-read",
            Self::MarketDataOptionsZeroDteRead => "market-data-options-zero-dte-read",
            Self::MarketDataOptionsZeroDteContractRead => {
                "market-data-options-zero-dte-contract-read"
            },
            Self::MarketDataOptionsEarningsRead => "market-data-options-earnings-read",
            Self::MarketDataOptionsSellerRead => "market-data-options-seller-read",
            Self::MarketDataNewsActionsRead => "market-data-news-actions-read",
            Self::MarketDataNewsSearchRead => "market-data-news-search-read",
            Self::MarketDataPredictionRead => "market-data-prediction-read",
            Self::MarketDataSubscriptionAcquireWrite => "market-data-subscription-acquire-write",
            Self::MarketDataSubscriptionReleaseWrite => "market-data-subscription-release-write",
            Self::MarketDataSubscriptionClearWrite => "market-data-subscription-clear-write",
            Self::MarketDataSubscriptionHeartbeatWrite => "market-data-subscription-heartbeat-write",
            Self::MarketDataPredictionSubscriptionAcquireWrite => "market-data-prediction-subscription-acquire-write",
            Self::MarketDataPredictionSubscriptionReleaseWrite => "market-data-prediction-subscription-release-write",
            Self::MarketDataInstrumentsNormalizeWrite => "market-data-instruments-normalize-write",
            Self::MarketDataBatchSnapshotsWrite => "market-data-batch-snapshots-write",
            Self::MarketDataOptionsAnalysisWrite => "market-data-options-analysis-write",
            Self::MarketDataZeroDteWrite => "market-data-zero-dte-write",
            Self::MarketDataPredictionCombosWrite => "market-data-prediction-combos-write",
            Self::PluginsRead => "plugins-read",
            Self::PluginsWrite => "plugins-write",
            Self::PluginGuidanceRead => "plugin-guidance-read",
            Self::AlertsRead => "alerts-read",
            Self::AlertsWrite => "alerts-write",
            Self::AdkTemplatesRead => "adk-templates-read",
            Self::AdkRead => "adk-read",
            Self::AdkMutation => "adk-mutation",
            Self::AdkChat => "adk-chat",
            Self::WebSocketLive => "websocket-live",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) struct ProductionRouteBinding {
    pub(crate) method: String,
    pub(crate) path: String,
    pub(crate) route_group: String,
    pub(crate) adapter: ProductionRouteAdapter,
    pub(crate) adapter_binding: ProductionAdapterBinding,
    /// Readiness for operations selected by query on a shared public route.
    /// Currently populated for `/market-data/options/events` only.
    pub(crate) operation_bindings: BTreeMap<String, ProductionAdapterBinding>,
}

impl ProductionRouteBinding {
    #[allow(dead_code)]
    pub(crate) fn operation_binding(
        &self,
        operation: &str,
    ) -> Option<ProductionAdapterBinding> {
        let normalized = operation.trim().to_ascii_lowercase();
        self.operation_bindings.get(&normalized).copied()
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) struct ProductionRouteRegistry {
    bindings: Vec<ProductionRouteBinding>,
    catalog: RouteCatalog,
    digest: String,
}

impl ProductionRouteRegistry {
    pub(crate) fn bind(ports: &ProductionPortBundle) -> Result<Self, ProductError> {
        let ledger: RouteLedger = serde_json::from_str(CANONICAL_ROUTE_LEDGER).map_err(|error| {
            ProductError::RouteRegistry(format!("invalid canonical ledger: {error}"))
        })?;
        let canonical_routes = ledger
            .operations
            .iter()
            .map(|operation| {
                format!(
                    "{} {}",
                    operation.method.trim().to_uppercase(),
                    operation.path.trim()
                )
            })
            .collect::<Vec<_>>();
        let canonical_digest = route_profile_digest(&canonical_routes);
        if canonical_routes.len() != EXPECTED_PRODUCTION_ROUTE_COUNT {
            return Err(ProductError::RouteRegistry(format!(
                "canonical ledger contains {} routes, expected {EXPECTED_PRODUCTION_ROUTE_COUNT}",
                canonical_routes.len()
            )));
        }
        if canonical_digest != EXPECTED_PRODUCTION_ROUTE_DIGEST {
            return Err(ProductError::RouteRegistry(format!(
                "canonical route digest {canonical_digest} does not match {EXPECTED_PRODUCTION_ROUTE_DIGEST}"
            )));
        }
        let mut bindings = Vec::with_capacity(ledger.operations.len());
        for operation in ledger.operations {
            let method = operation.method.trim().to_uppercase();
            let path = operation.path.trim().to_owned();
            let adapter = adapter_for(&operation.capability, &method, &path).ok_or_else(|| {
                ProductError::MissingProductionAdapter {
                    method: method.clone(),
                    path: path.clone(),
                    adapter: "unclassified-route".to_owned(),
                }
            })?;
            let adapter_binding = ports.adapter_binding(adapter).ok_or_else(|| {
                ProductError::MissingProductionAdapter {
                    method: method.clone(),
                    path: path.clone(),
                    adapter: adapter.name().to_owned(),
                }
            })?;
            let operation_bindings = if adapter == ProductionRouteAdapter::MarketDataOptionsEventsRead
            {
                OPTION_EVENT_OPERATION_ADAPTERS
                    .iter()
                    .map(|(operation, operation_adapter)| {
                        let binding = ports.adapter_binding(*operation_adapter).ok_or_else(|| {
                            ProductError::MissingProductionAdapter {
                                method: method.clone(),
                                path: path.clone(),
                                adapter: operation_adapter.name().to_owned(),
                            }
                        })?;
                        Ok(((*operation).to_owned(), binding))
                    })
                    .collect::<Result<BTreeMap<_, _>, ProductError>>()?
            } else {
                BTreeMap::new()
            };
            bindings.push(ProductionRouteBinding {
                method,
                path,
                route_group: operation.capability,
                adapter,
                adapter_binding,
                operation_bindings,
            });
        }
        Self::finish(bindings, canonical_digest)
    }

    fn finish(
        bindings: Vec<ProductionRouteBinding>,
        canonical_digest: String,
    ) -> Result<Self, ProductError> {
        if bindings.len() != EXPECTED_PRODUCTION_ROUTE_COUNT {
            return Err(ProductError::RouteRegistry(format!(
                "expected {EXPECTED_PRODUCTION_ROUTE_COUNT} bindings, got {}",
                bindings.len()
            )));
        }
        let catalog = RouteCatalog::new(bindings.iter().map(|binding| RouteSpec {
            method: binding.method.clone(),
            path: binding.path.clone(),
        }))?;
        let capabilities = catalog
            .routes()
            .iter()
            .map(|route| format!("{} {}", route.method, route.path))
            .collect::<Vec<_>>();
        let digest = route_profile_digest(&capabilities);
        if digest != canonical_digest {
            return Err(ProductError::RouteRegistry(format!(
                "route digest mismatch: bound={digest}, canonical={canonical_digest}"
            )));
        }
        Ok(Self {
            bindings,
            catalog,
            digest,
        })
    }

    pub(crate) const fn catalog(&self) -> &RouteCatalog {
        &self.catalog
    }

    pub(crate) fn bindings(&self) -> &[ProductionRouteBinding] {
        &self.bindings
    }

    pub(crate) fn digest(&self) -> &str {
        &self.digest
    }
}

#[derive(Debug, Deserialize)]
struct RouteLedger {
    operations: Vec<RouteLedgerOperation>,
}

#[derive(Debug, Deserialize)]
struct RouteLedgerOperation {
    method: String,
    path: String,
    capability: String,
}

fn adapter_for(
    capability: &str,
    method: &str,
    path: &str,
) -> Option<ProductionRouteAdapter> {
    match capability {
        "auth" => Some(if method == "GET" {
            ProductionRouteAdapter::AuthSessionRead
        } else {
            ProductionRouteAdapter::AuthSessionWrite
        }),
        "settings" => Some(if path.starts_with("/api/v1/settings/data-management/") {
            ProductionRouteAdapter::DataManagement
        } else {
            ProductionRouteAdapter::Settings
        }),
        "system" => system_adapter(method, path),
        "watchlist" => Some(watchlist_adapter(method, path)),
        "watchlists" => Some(if method == "GET" {
            ProductionRouteAdapter::RemoteWatchlistRead
        } else {
            ProductionRouteAdapter::RemoteWatchlistWrite
        }),
        "strategy-definitions" => Some(if method == "GET" {
            ProductionRouteAdapter::StrategyDefinitionRead
        } else {
            ProductionRouteAdapter::StrategyDefinitionWrite
        }),
        "strategies" => Some(if method == "GET" {
            ProductionRouteAdapter::StrategyRuntimeRead
        } else {
            ProductionRouteAdapter::StrategyRuntimeWrite
        }),
        "strategy-pine" => Some(ProductionRouteAdapter::StrategyPine),
        "research" => research_adapter(method, path),
        "backtests" => Some(backtest_adapter(method, path)),
        "execution" => Some(if method == "GET" {
            ProductionRouteAdapter::ExecutionRead
        } else {
            ProductionRouteAdapter::ExecutionWrite
        }),
        "brokers" => Some(if method == "GET" {
            ProductionRouteAdapter::BrokerRead
        } else {
            ProductionRouteAdapter::BrokerWrite
        }),
        "portfolio" => Some(ProductionRouteAdapter::PortfolioRead),
        "market-data" => market_data_adapter(method, path),
        "plugins" => Some(plugin_adapter(method, path)),
        "alerts" => Some(if method == "GET" {
            ProductionRouteAdapter::AlertsRead
        } else {
            ProductionRouteAdapter::AlertsWrite
        }),
        "adk" => Some(adk_adapter(method, path)),
        "ws" => Some(ProductionRouteAdapter::WebSocketLive),
        _ => None,
    }
}

fn system_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    if path.starts_with("/api/v1/system/exchange-calendars/") {
        return Some(ProductionRouteAdapter::Calendar);
    }
    if method != "GET" {
        return Some(if path == "/api/v1/system/futu-opend/manual-retry" {
            ProductionRouteAdapter::SystemOpenDWrite
        } else {
            ProductionRouteAdapter::RealTradeControlWrite
        });
    }
    if matches!(
        path,
        "/api/v1/system/futu-opend" | "/api/v1/system/worker/broker-order-updates"
    ) {
        return Some(ProductionRouteAdapter::SystemRead);
    }
    Some(ProductionRouteAdapter::SystemCore)
}

fn watchlist_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if method != "GET" {
        return ProductionRouteAdapter::WatchlistWrite;
    }
    if path.ends_with("/memberships") {
        ProductionRouteAdapter::WatchlistMemberships
    } else {
        ProductionRouteAdapter::WatchlistRead
    }
}

fn research_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    if path == "/api/v1/research/screens/catalog" {
        return Some(ProductionRouteAdapter::ResearchCatalog);
    }
    if path.starts_with("/api/v1/research/screens/presets") {
        return Some(if method == "GET" {
            ProductionRouteAdapter::ResearchPresetRead
        } else {
            ProductionRouteAdapter::ResearchPresetWrite
        });
    }
    if method == "POST" && path == "/api/v1/research/screens" {
        return Some(ProductionRouteAdapter::ResearchScreenWrite);
    }
    if method == "GET" && path == "/api/v1/research/rankings" {
        return Some(ProductionRouteAdapter::ResearchRankingsRead);
    }
    if method == "GET" && path == "/api/v1/research/industries" {
        return Some(ProductionRouteAdapter::ResearchIndustriesRead);
    }
    if method == "GET" && path == "/api/v1/research/calendars" {
        return Some(ProductionRouteAdapter::ResearchCalendarRead);
    }
    if method == "GET" && path == "/api/v1/research/macro" {
        return Some(ProductionRouteAdapter::ResearchMacroRead);
    }
    (method == "GET").then_some(ProductionRouteAdapter::ResearchRead)
}

fn backtest_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    match (method, path) {
        ("GET", "/api/v1/backtests/sync/{taskId}") => {
            ProductionRouteAdapter::BacktestSyncRead
        }
        ("GET", _) => ProductionRouteAdapter::BacktestRead,
        ("POST", "/api/v1/backtests/sync") => ProductionRouteAdapter::BacktestSyncStart,
        ("DELETE", "/api/v1/backtests/sync/{taskId}") => {
            ProductionRouteAdapter::BacktestSyncCancel
        }
        ("DELETE", _) => ProductionRouteAdapter::BacktestDelete,
        _ => ProductionRouteAdapter::BacktestStart,
    }
}

fn market_data_adapter(method: &str, path: &str) -> Option<ProductionRouteAdapter> {
    match (method, path) {
        ("POST", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite)
        }
        ("POST", "/api/v1/market-data/subscriptions/release") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite)
        }
        ("DELETE", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionClearWrite)
        }
        ("POST", "/api/v1/market-data/subscriptions/heartbeat") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite)
        }
        ("POST", p)
            if p.starts_with("/api/v1/market-data/prediction/contracts/")
                && p.ends_with("/subscriptions") =>
        {
            Some(ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite)
        }
        ("DELETE", p)
            if p.starts_with("/api/v1/market-data/prediction/contracts/")
                && (p.ends_with("/subscriptions") || p.contains("/subscriptions/")) =>
        {
            Some(ProductionRouteAdapter::MarketDataPredictionSubscriptionReleaseWrite)
        }
        ("POST", "/api/v1/market-data/instruments/normalize") => {
            Some(ProductionRouteAdapter::MarketDataInstrumentsNormalizeWrite)
        }
        ("POST", "/api/v1/market-data/snapshots") => {
            Some(ProductionRouteAdapter::MarketDataBatchSnapshotsWrite)
        }
        ("POST", p) if p.starts_with("/api/v1/market-data/options/analysis") => {
            Some(ProductionRouteAdapter::MarketDataOptionsAnalysisWrite)
        }
        ("POST", p)
            if p.starts_with("/api/v1/market-data/options/zero-dte")
                || p.starts_with("/api/v1/market-data/options/events/zero-dte") =>
        {
            Some(ProductionRouteAdapter::MarketDataZeroDteWrite)
        }
        ("POST", p) if p.starts_with("/api/v1/market-data/prediction/combos") => {
            Some(ProductionRouteAdapter::MarketDataPredictionCombosWrite)
        }
        ("GET", "/api/v1/market-data/provider") => {
            Some(ProductionRouteAdapter::MarketDataProviderRead)
        }
        ("GET", "/api/v1/market-data/markets") => {
            Some(ProductionRouteAdapter::MarketDataMarketsRead)
        }
        ("GET", "/api/v1/market-data/instruments") => {
            Some(ProductionRouteAdapter::MarketDataSearchRead)
        }
        ("GET", "/api/v1/market-data/subscriptions") => {
            Some(ProductionRouteAdapter::MarketDataSubscriptionRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/securities/") => {
            Some(ProductionRouteAdapter::MarketDataSecuritiesRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/snapshots/") => {
            Some(ProductionRouteAdapter::MarketDataSnapshotsRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/candles/") => {
            Some(ProductionRouteAdapter::MarketDataCandlesRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/depth/") => {
            Some(ProductionRouteAdapter::MarketDataDepthRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/ticks/") => {
            Some(ProductionRouteAdapter::MarketDataTicksRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/broker-queue/") => {
            Some(ProductionRouteAdapter::MarketDataBrokerQueueRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/capital-flow/") => {
            Some(ProductionRouteAdapter::MarketDataCapitalFlowRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/intraday/") => {
            Some(ProductionRouteAdapter::MarketDataIntradayRead)
        }
        ("GET", p)
            if p.starts_with("/api/v1/market-data/instruments/") && p.ends_with("/profile") =>
        {
            Some(ProductionRouteAdapter::MarketDataProfileRead)
        }
        ("GET", "/api/v1/market-data/warrants") | ("GET", "/api/v1/market-data/futures") => {
            Some(ProductionRouteAdapter::MarketDataDerivativeRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/options/") => {
            Some(if p.starts_with("/api/v1/market-data/options/chains/") {
                ProductionRouteAdapter::MarketDataOptionsChainRead
            } else if p.starts_with("/api/v1/market-data/options/expirations/") {
                ProductionRouteAdapter::MarketDataOptionsExpirationsRead
            } else if p == "/api/v1/market-data/options/screens" {
                ProductionRouteAdapter::MarketDataOptionsScreenRead
            } else if p.starts_with("/api/v1/market-data/options/analysis/") {
                ProductionRouteAdapter::MarketDataOptionsAnalysisRead
            } else if p == "/api/v1/market-data/options/events" {
                ProductionRouteAdapter::MarketDataOptionsEventsRead
            } else {
                ProductionRouteAdapter::MarketDataOptionsRead
            })
        }
        ("GET", "/api/v1/market-data/news") => {
            Some(ProductionRouteAdapter::MarketDataNewsSearchRead)
        }
        ("GET", p)
            if p.starts_with("/api/v1/market-data/news/")
                || p.starts_with("/api/v1/market-data/corporate-actions/") =>
        {
            Some(ProductionRouteAdapter::MarketDataNewsActionsRead)
        }
        ("GET", p) if p.starts_with("/api/v1/market-data/prediction/") => {
            Some(ProductionRouteAdapter::MarketDataPredictionRead)
        }
        _ => None,
    }
}

fn plugin_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if method != "GET" {
        ProductionRouteAdapter::PluginsWrite
    } else if path.ends_with("/uninstall-guidance") {
        ProductionRouteAdapter::PluginGuidanceRead
    } else {
        ProductionRouteAdapter::PluginsRead
    }
}

fn adk_adapter(method: &str, path: &str) -> ProductionRouteAdapter {
    if path == "/api/v1/adk/agent-templates" {
        return ProductionRouteAdapter::AdkTemplatesRead;
    }
    if method == "GET" {
        return ProductionRouteAdapter::AdkRead;
    }
    if matches!(path, ADK_CHAT_PATH | ADK_CHAT_STREAM_PATH) {
        ProductionRouteAdapter::AdkChat
    } else {
        ProductionRouteAdapter::AdkMutation
    }
}
