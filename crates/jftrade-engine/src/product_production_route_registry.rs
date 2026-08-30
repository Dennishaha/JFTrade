use std::collections::BTreeMap;

use serde::Deserialize;

use super::product_production_ports::{
    runtime_scoped_adapter, ProductionAdapterBinding, ProductionPortBundle,
    OPTION_ANALYSIS_OPERATIONS,
};
use super::*;

const EXPECTED_PRODUCTION_ROUTE_COUNT: usize = 278;
const EXPECTED_PRODUCTION_ROUTE_DIGEST: &str =
    "afa112435ed280dd24d43bb4acaa0f7ca2ab45c01e4e5701efc5ce149e5b85b2";
const PRODUCTION_ROUTE_MANIFEST: &str = include_str!("product_production_route_manifest.json");

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
    /// Futu/OpenD future-contract catalogue.  Warrants intentionally keep a
    /// separate adapter so an unavailable warrant reader cannot mask a ready
    /// futures reader (or vice versa).
    MarketDataFuturesRead,
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
            Self::MarketDataFuturesRead => "market-data-futures-read",
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
    pub(crate) dispatch_target: ProductionRouteAdapter,
    pub(crate) adapter_binding: ProductionAdapterBinding,
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

    pub(crate) const fn dispatch_target(&self) -> ProductionRouteAdapter {
        self.dispatch_target
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
        let ledger: RouteLedger = serde_json::from_str(PRODUCTION_ROUTE_MANIFEST).map_err(|error| {
            ProductError::RouteRegistry(format!("invalid canonical ledger: {error}"))
        })?;
        if ledger.version.as_deref() != Some("production.v1") {
            return Err(ProductError::RouteRegistry(
                "production route manifest version is not production.v1".to_owned(),
            ));
        }
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
        if ledger.route_digest.as_deref() != Some(canonical_digest.as_str()) {
            return Err(ProductError::RouteRegistry(format!(
                "production route manifest digest {:?} does not match computed {canonical_digest}",
                ledger.route_digest
            )));
        }
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
            let adapter_binding = if adapter == ProductionRouteAdapter::ResearchRead {
                // ResearchRead is a compatibility umbrella for several
                // operations. Resolve readiness from the concrete path so a
                // helper-backed profile/financials route (or Futu valuation)
                // is not hidden behind the umbrella's conservative default.
                ports.research_operation_binding(&path)
            } else if adapter == ProductionRouteAdapter::ExecutionWrite {
                // Order placement/cancellation and product-rule previews do
                // not share the same readiness boundary. Keep the generic
                // writer status for ordinary execution routes, while letting
                // buying-power/combo previews require their real readers.
                ports
                    .execution_operation_binding(&path)
                    .or_else(|| ports.adapter_binding(adapter))
            } else {
                Some(ports.adapter_binding_or_missing(adapter))
            }
            .unwrap_or(ProductionAdapterBinding::MissingInternalAdapter);
            if adapter_binding == ProductionAdapterBinding::MissingInternalAdapter {
                return Err(ProductError::MissingProductionAdapter {
                    method: method.clone(),
                    path: path.clone(),
                    adapter: adapter.name().to_owned(),
                });
            }
            let operation_bindings = if adapter == ProductionRouteAdapter::MarketDataOptionsEventsRead
            {
                OPTION_EVENT_OPERATION_ADAPTERS
                    .iter()
                    .map(|(operation, operation_adapter)| {
                        let binding = ports.adapter_binding_or_missing(*operation_adapter);
                        if binding == ProductionAdapterBinding::MissingInternalAdapter {
                            return Err(ProductError::MissingProductionAdapter {
                                method: method.clone(),
                                path: path.clone(),
                                adapter: operation_adapter.name().to_owned(),
                            });
                        }
                        Ok(((*operation).to_owned(), binding))
                    })
                    .collect::<Result<BTreeMap<_, _>, ProductError>>()?
            } else if adapter == ProductionRouteAdapter::MarketDataOptionsAnalysisRead {
                OPTION_ANALYSIS_OPERATIONS
                    .iter()
                    .map(|operation| {
                        let binding = ports
                            .option_analysis_operation_binding(operation)
                            .unwrap_or(ProductionAdapterBinding::MissingInternalAdapter);
                        if binding == ProductionAdapterBinding::MissingInternalAdapter {
                            return Err(ProductError::MissingProductionAdapter {
                                method: method.clone(),
                                path: path.clone(),
                                adapter: adapter.name().to_owned(),
                            });
                        }
                        Ok(((*operation).to_owned(), binding))
                    })
                    .collect::<Result<BTreeMap<_, _>, ProductError>>()?
            } else if adapter == ProductionRouteAdapter::ResearchRead {
                let binding = ports
                    .research_operation_binding(&path)
                    .unwrap_or(ProductionAdapterBinding::MissingInternalAdapter);
                if binding == ProductionAdapterBinding::MissingInternalAdapter {
                    return Err(ProductError::MissingProductionAdapter {
                        method: method.clone(),
                        path: path.clone(),
                        adapter: adapter.name().to_owned(),
                    });
                }
                research_operation_bindings(&path, binding)
            } else {
                BTreeMap::new()
            };
            bindings.push(ProductionRouteBinding {
                method,
                path,
                route_group: operation.capability,
                adapter,
                dispatch_target: adapter,
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

    pub(crate) fn resolve(
        &self,
        method: &str,
        concrete_path: &str,
    ) -> Option<&ProductionRouteBinding> {
        let method = method.trim().to_ascii_uppercase();
        self.bindings.iter().find(|binding| {
            binding.method == method && template_matches(&binding.path, concrete_path)
        })
    }

    /// Return the live readiness for a previously validated route binding.
    ///
    /// Route registration is intentionally immutable: the canonical 278
    /// operations and their digest must not change while the process runs.
    /// Adapter capability, however, follows the shared production runtime and
    /// may change when a provider reconnects, is switched, or begins teardown.
    /// Re-evaluate that capability at dispatch time instead of retaining the
    /// startup snapshot in `ProductionRouteBinding`.
    pub(crate) fn current_binding(
        &self,
        binding: &ProductionRouteBinding,
        ports: &ProductionPortBundle,
    ) -> ProductionAdapterBinding {
        if !ports.installed_adapters.contains(&binding.adapter)
            || !ports.bound_adapters.contains_key(&binding.adapter)
        {
            return ProductionAdapterBinding::MissingInternalAdapter;
        }
        if ports.bound_adapters.get(&binding.adapter)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return ProductionAdapterBinding::MissingInternalAdapter;
        }
        if binding.adapter == ProductionRouteAdapter::MarketDataOptionsEventsRead
            && OPTION_EVENT_OPERATION_ADAPTERS.iter().any(|(_, adapter)| {
                ports.adapter_binding_or_missing(*adapter)
                    == ProductionAdapterBinding::MissingInternalAdapter
            })
        {
            return ProductionAdapterBinding::MissingInternalAdapter;
        }
        let snapshot = ports.active_provider_state.snapshot();
        if snapshot.closing {
            return ProductionAdapterBinding::ExternalUnavailable;
        }
        let dynamic = if binding.adapter == ProductionRouteAdapter::ResearchRead {
            ports.research_operation_binding(&binding.path)
        } else if binding.adapter == ProductionRouteAdapter::ExecutionWrite {
            ports
                .execution_operation_binding(&binding.path)
                .or_else(|| ports.adapter_binding(binding.adapter))
        } else {
            ports.adapter_binding(binding.adapter)
        };
        dynamic.unwrap_or_else(|| {
            if runtime_scoped_adapter(binding.adapter) {
                ProductionAdapterBinding::ExternalUnavailable
            } else {
                ProductionAdapterBinding::MissingInternalAdapter
            }
        })
    }
}

fn template_matches(template: &str, concrete: &str) -> bool {
    let template = template.split('/').collect::<Vec<_>>();
    let concrete = concrete.split('/').collect::<Vec<_>>();
    template.len() == concrete.len()
        && template.iter().zip(concrete).all(|(expected, actual)| {
            if expected.starts_with('{') && expected.ends_with('}') {
                !actual.is_empty()
            } else {
                *expected == actual
            }
        })
}

fn research_operation_bindings(
    path: &str,
    binding: ProductionAdapterBinding,
) -> BTreeMap<String, ProductionAdapterBinding> {
    let mut bindings = BTreeMap::from([(path.to_owned(), binding)]);
    let aliases: &[&str] = if path.starts_with("/api/v1/research/instruments/") {
        &["profile"]
    } else if path.starts_with("/api/v1/research/financials/") {
        &["statements"]
    } else if path.starts_with("/api/v1/research/valuation/") {
        &["valuation", "detail"]
    } else if path.starts_with("/api/v1/research/analyst/") {
        &["consensus"]
    } else if path.starts_with("/api/v1/research/ownership/") {
        &["overview"]
    } else if path.starts_with("/api/v1/research/corporate-actions/") {
        &["dividends"]
    } else if path.starts_with("/api/v1/research/short-interest/") {
        &["daily_volume", "short_interest"]
    } else if path.starts_with("/api/v1/research/technical-indicators/") {
        &["technical", "technical_indicators"]
    } else {
        &[]
    };
    for alias in aliases {
        bindings.insert((*alias).to_owned(), binding);
    }
    bindings
}

#[derive(Debug, Deserialize)]
struct RouteLedger {
    version: Option<String>,
    #[serde(rename = "routeDigest")]
    route_digest: Option<String>,
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

include!("product_production_route_registry_adapters.rs");
#[cfg(test)]
#[path = "product_production_route_registry_tests.rs"]
mod tests;
