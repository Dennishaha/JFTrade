//! ADK / Assistant read and stream production adapter.

use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::Arc;

use jftrade_settings::{AssistantRuntimeSettingsStorePort, normalize_assistant_runtime_settings};
use jftrade_store_settings_file::SettingsFileStore;
use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkSessionStoreError, AdkStore, AdkStoreError,
};
use serde_json::{Value, json};

use super::{ProductionAdapterBinding, SharedTradeReadRuntime};
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};
use crate::product::product_production_route_registry::ProductionRouteAdapter;
use crate::product::{
    ActiveProviderState, AdkReadEvent, AdkReadSnapshot, AdkReadSnapshotError, AdkReadSnapshotPort,
    AdkReadStream,
};

#[path = "product_production_ports_adk_metrics.rs"]
mod metrics;
#[path = "product_production_ports_adk_mutation.rs"]
mod mutation;
#[path = "product_production_ports_adk_projection.rs"]
mod projection;

use projection::{
    builtin_agent, builtin_skills, composer_state_value, dynamic_id, invalid_payload,
    is_deleted_payload, normalize_memory_key, not_found, page, payload, put_string, query_param,
    session_entity_value, timeline_value, workflow_trigger_value,
};

#[derive(Debug)]
pub(crate) struct ProductionAdkPort {
    pub(crate) store: Arc<AdkStore>,
    pub(crate) session_store: Arc<AdkSessionStore>,
    pub(crate) artifact_store: Arc<AdkArtifactStore>,
    pub(crate) tool_catalog: Arc<ProductionToolCatalog>,
    pub(crate) settings_path: PathBuf,
    /// Optional runtime-owned chat adapter.
    ///
    /// The production composition root must inject the concrete assistant
    /// runtime here once its provider/secret lifecycle is available.  Keeping
    /// the seam on the production port (rather than branching in the HTTP
    /// layer) ensures configured runtimes can execute both chat and stream
    /// requests while an unconfigured runtime remains explicitly unavailable.
    pub(crate) chat_runtime: Option<Arc<dyn AdkChatStreamPort>>,
}

#[derive(Clone, Debug)]
pub(crate) struct ProductionToolCatalog {
    tools: Vec<Value>,
    /// The startup binding snapshot is retained for fail-closed validation and
    /// for adapters whose capability is independent of the selected provider.
    /// Provider-backed tools additionally keep the shared runtime state so
    /// their descriptor is projected from the current provider on every read.
    bindings: BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    research_bindings: BTreeMap<&'static str, ProductionAdapterBinding>,
    active_provider_state: Option<Arc<ActiveProviderState>>,
    trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    backtest_execution_ready: bool,
}

impl ProductionToolCatalog {
    #[cfg(test)]
    pub(crate) fn from_bindings(
        bindings: &BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    ) -> Result<Self, String> {
        // Callers that do not have the composition root's operation matrix
        // must fail closed for the finer-grained research tools.  The legacy
        // umbrella binding is intentionally not used as a readiness fallback.
        let unavailable = ProductionAdapterBinding::ExternalUnavailable;
        let research = BTreeMap::from([
            ("instrument", unavailable),
            ("financials", unavailable),
            ("valuation", unavailable),
            ("news", unavailable),
        ]);
        Self::from_bindings_with_research(bindings, &research)
    }

    /// Build the tool catalog with operation-level research readiness.
    ///
    /// Research HTTP routes share the `ResearchRead` adapter for compatibility,
    /// but ADK tools are finer grained: helper-backed instrument/financials
    /// operations may be callable while valuation (Futu-only) or news are not.
    /// The composition root supplies those concrete readiness decisions here so
    /// the catalog never advertises an umbrella adapter as universally ready.
    pub(crate) fn from_bindings_with_research(
        bindings: &BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
        research_bindings: &BTreeMap<&'static str, ProductionAdapterBinding>,
    ) -> Result<Self, String> {
        let mut tools = Vec::with_capacity(PRODUCTION_TOOL_DEFINITIONS.len());
        for definition in PRODUCTION_TOOL_DEFINITIONS {
            let binding = match definition.research_operation {
                Some(operation) => research_bindings.get(operation).copied(),
                None => bindings.get(&definition.adapter).copied(),
            };
            let Some(binding) = binding else {
                return Err(format!(
                    "missing production adapter for ADK tool {}: {}",
                    definition.id,
                    definition.adapter.name()
                ));
            };
            // Keep the descriptor in the catalog for wire compatibility, but
            // do not advertise unavailable provider-backed operations as
            // callable. An empty allowedModes set is the existing descriptor
            // shape's explicit "not currently available" representation and
            // avoids adding a new public field.
            let allowed_modes = if binding == ProductionAdapterBinding::Ready {
                json!(["approval", "less_approval", "all"])
            } else {
                json!([])
            };
            tools.push(json!({
                "id": definition.id,
                "name": definition.id,
                "category": definition.category,
                "displayName": definition.display_name,
                "allowedModes": allowed_modes,
                "requiresApprovalIn": [],
                "riskLevel": "low",
                "idempotencyMode": "replay_safe",
            }));
        }
        Ok(Self {
            tools,
            bindings: bindings.clone(),
            research_bindings: research_bindings.clone(),
            active_provider_state: None,
            trade_runtime: None,
            backtest_execution_ready: false,
        })
    }

    /// Attach the shared runtime provider state to an already validated
    /// catalog.  This is deliberately a separate step from construction so
    /// tests and non-production callers can continue to build a deterministic
    /// static catalog, while the production composition can opt into live
    /// readiness projection without changing the public ADK wire shape.
    pub(crate) fn with_active_provider_state(
        mut self,
        active_provider_state: Arc<ActiveProviderState>,
    ) -> Self {
        self.active_provider_state = Some(active_provider_state);
        self
    }

    /// Attach the shared trade/OpenD runtime used by provider-backed tools.
    ///
    /// OpenD connectivity alone is not sufficient for news or corporate
    /// actions: those operations require their concrete reader to be wired
    /// into the runtime.  Keeping the runtime handle on the catalog lets the
    /// descriptor be re-evaluated after provider activation without turning a
    /// missing reader into a callable tool.
    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    ) -> Self {
        self.trade_runtime = trade_runtime;
        self
    }

    /// Attach the verified PineTS execution readiness used by the
    /// `strategy.research_backtest` descriptor. Provider state remains
    /// dynamic; this flag only records that the composition root installed a
    /// worker that passed its real startup probe.
    pub(crate) fn with_backtest_execution_ready(mut self, ready: bool) -> Self {
        self.backtest_execution_ready = ready;
        self
    }

    fn values(&self) -> Vec<Value> {
        let Some(active_provider_state) = self.active_provider_state.as_ref() else {
            return self.tools.clone();
        };
        let snapshot = active_provider_state.snapshot();
        self.tools
            .iter()
            .zip(PRODUCTION_TOOL_DEFINITIONS.iter())
            .map(|(tool, definition)| {
                let mut tool = tool.clone();
                let binding = self.binding_for(definition, &snapshot);
                if let Some(object) = tool.as_object_mut() {
                    object.insert("allowedModes".to_owned(), allowed_modes(binding));
                }
                tool
            })
            .collect()
    }

    fn ids(&self) -> Vec<String> {
        self.tools
            .iter()
            .filter_map(|tool| tool.get("id").and_then(Value::as_str))
            .map(str::to_owned)
            .collect()
    }

    /// Return the descriptors that are currently callable after provider and
    /// runtime readiness projection.  This is intentionally derived from
    /// `values()` on every read rather than exposing the startup snapshot.
    pub(crate) fn callable_tools(&self) -> Vec<Value> {
        self.values()
            .into_iter()
            .filter(|tool| {
                tool.get("allowedModes")
                    .and_then(Value::as_array)
                    .is_some_and(|modes| !modes.is_empty())
            })
            .collect()
    }

    /// Resolve readiness for the small native MCP surface.  Most MCP names
    /// map directly to an ADK descriptor, while `market.providers` and
    /// `portfolio.summary` are compatibility aliases whose names predate the
    /// production ADK catalog.  Keep those aliases on the same binding map so
    /// `tools/list` and `tools/call` cannot disagree about availability.
    pub(crate) fn binding_for_mcp_tool(&self, name: &str) -> Option<ProductionAdapterBinding> {
        let snapshot = self
            .active_provider_state
            .as_ref()
            .map(|state| state.snapshot());
        if let Some(definition) = PRODUCTION_TOOL_DEFINITIONS
            .iter()
            .find(|definition| definition.id == name)
        {
            return Some(snapshot.as_ref().map_or_else(
                || {
                    self.bindings
                        .get(&definition.adapter)
                        .copied()
                        .unwrap_or(ProductionAdapterBinding::ExternalUnavailable)
                },
                |snapshot| self.binding_for(definition, snapshot),
            ));
        }
        let adapter = match name {
            "plugins.catalog" => ProductionRouteAdapter::PluginsRead,
            "market.providers" | "market.capabilities" => {
                ProductionRouteAdapter::MarketDataProviderRead
            }
            "market.instrument_profile" => ProductionRouteAdapter::MarketDataProfileRead,
            "market.intraday" => ProductionRouteAdapter::MarketDataIntradayRead,
            "market.ticks" => ProductionRouteAdapter::MarketDataTicksRead,
            "market.depth" => ProductionRouteAdapter::MarketDataDepthRead,
            "market.broker_queue" => ProductionRouteAdapter::MarketDataBrokerQueueRead,
            "market.capital_flow" => ProductionRouteAdapter::MarketDataCapitalFlowRead,
            "derivatives.warrants" => ProductionRouteAdapter::MarketDataDerivativeRead,
            "derivatives.futures" => ProductionRouteAdapter::MarketDataFuturesRead,
            "derivatives.option_chain" => ProductionRouteAdapter::MarketDataOptionsChainRead,
            "derivatives.option_screen" => ProductionRouteAdapter::MarketDataOptionsScreenRead,
            "derivatives.option_analysis" => {
                ProductionRouteAdapter::MarketDataOptionsAnalysisRead
            },
            "derivatives.option_events" => ProductionRouteAdapter::MarketDataOptionsEventsRead,
            "prediction.discover"
            | "prediction.snapshot"
            | "prediction.depth"
            | "prediction.history"
            | "prediction.combo_eligible" => ProductionRouteAdapter::MarketDataPredictionRead,
            "prediction.combo_quote" => ProductionRouteAdapter::MarketDataPredictionCombosWrite,
            "system.runtime_dependencies" => ProductionRouteAdapter::SystemRead,
            "watchlist.remote.list" => ProductionRouteAdapter::RemoteWatchlistRead,
            "portfolio.summary" => ProductionRouteAdapter::PortfolioRead,
            "account.orders" => ProductionRouteAdapter::ExecutionRead,
            "broker.orders" | "broker.fills" => ProductionRouteAdapter::BrokerRead,
            "broker.cash_flows" | "broker.fees" | "broker.margin_ratios" => {
                ProductionRouteAdapter::BrokerRead
            }
            "execution.order_events" => ProductionRouteAdapter::ExecutionRead,
            "execution.buying_power" => ProductionRouteAdapter::ExecutionWrite,
            "strategy.definition_versions.list" | "strategy.definition_versions.get" => {
                ProductionRouteAdapter::StrategyDefinitionRead
            }
            "strategy.instance_activity" => ProductionRouteAdapter::StrategyRuntimeRead,
            "risk.state" | "risk.events" => ProductionRouteAdapter::SystemRead,
            _ => return None,
        };
        let startup_binding = self
            .bindings
            .get(&adapter)
            .copied()
            .unwrap_or(ProductionAdapterBinding::ExternalUnavailable);
        if adapter != ProductionRouteAdapter::PortfolioRead {
            return Some(startup_binding);
        }
        let Some(snapshot) = snapshot else {
            return Some(startup_binding);
        };
        let trade_ready = self
            .trade_runtime
            .as_ref()
            .is_some_and(|runtime| runtime.snapshot().is_ready());
        Some(
            if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && trade_ready
            {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            },
        )
    }

    /// Convert the currently callable catalog into the OpenAI Responses
    /// function-tool shape.  Provider-backed tools whose readiness is empty
    /// are intentionally omitted so the model cannot invoke an unavailable
    /// capability.  Argument schemas remain object-shaped for compatibility
    /// with the existing Go tool contract; dispatch validates concrete input
    /// before any side effect.
    pub(crate) fn openai_tools(&self) -> Vec<Value> {
        self.callable_tools()
            .into_iter()
            .filter_map(|tool| {
                let name = tool.get("id").and_then(Value::as_str)?.to_owned();
                let description = tool
                    .get("displayName")
                    .and_then(Value::as_str)
                    .unwrap_or(&name)
                    .to_owned();
                Some(json!({
                    "type": "function",
                    "name": name,
                    "description": description,
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "instrumentId": {"type": "string"},
                            "market": {"type": "string"},
                            "symbol": {"type": "string"},
                            "query": {"type": "string"},
                            "period": {"type": "string"},
                            "limit": {"type": "integer"},
                            "operation": {"type": "string"},
                            "brokerId": {"type": "string"},
                            "accountId": {"type": "string"},
                            "tradingEnvironment": {"type": "string"},
                            "payload": {"type": "object"},
                            "internalOrderId": {"type": "string"}
                        },
                        "additionalProperties": false
                    }
                }))
            })
            .collect()
    }

    fn ids_for_categories(&self, categories: &[&str]) -> Vec<String> {
        self.tools
            .iter()
            .filter(|tool| {
                let category = tool.get("category").and_then(Value::as_str);
                category.is_some_and(|category| categories.contains(&category))
            })
            .filter_map(|tool| tool.get("id").and_then(Value::as_str))
            .map(str::to_owned)
            .collect()
    }

    fn binding_for(
        &self,
        definition: &ProductionToolDefinition,
        snapshot: &crate::product::product_active_provider_state::ProviderRuntimeSnapshot,
    ) -> ProductionAdapterBinding {
        if let Some(operation) = definition.research_operation {
            return self.research_binding_for(operation, snapshot);
        }
        let Some(startup_binding) = self.bindings.get(&definition.adapter).copied() else {
            // Construction validates every definition. Keep this defensive
            // branch fail-closed if a future definition is added without
            // updating the binding table.
            return ProductionAdapterBinding::ExternalUnavailable;
        };
        if definition.adapter == ProductionRouteAdapter::BacktestStart {
            // Backtests consume the verified PineTS worker and local candle
            // store.  Live helper/OpenD/router availability is checked only
            // by other market-data routes; an empty local range is mapped by
            // the backtest handler to BACKTESTS_WRITE_UNAVAILABLE.
            return if self.backtest_execution_ready && snapshot.provider.is_some() {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            };
        }
        if !is_provider_dynamic_adapter(definition.adapter) {
            return startup_binding;
        }
        match definition.adapter {
            ProductionRouteAdapter::MarketDataSearchRead
            | ProductionRouteAdapter::MarketDataCandlesRead
            | ProductionRouteAdapter::MarketDataSecuritiesRead => {
                let helper_ready = snapshot.helper_ready && helper_provider(snapshot.provider);
                if helper_ready {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            ProductionRouteAdapter::MarketDataNewsSearchRead => {
                let helper_ready = snapshot.helper_ready && helper_provider(snapshot.provider);
                let futu_ready = snapshot.provider
                    == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.news_reader_available());
                if helper_ready || futu_ready {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            ProductionRouteAdapter::MarketDataNewsActionsRead => {
                let helper_ready = snapshot.helper_ready && helper_provider(snapshot.provider);
                let futu_ready = snapshot.provider
                    == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.corporate_actions_reader_available());
                if helper_ready || futu_ready {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            ProductionRouteAdapter::ResearchScreenWrite => {
                // The POST screen adapter is backed by the embedded helper;
                // Futu's OpenD stock-filter reader is not wired here. Keep
                // this dynamic so an activation/reconnect cannot retain a
                // stale Ready descriptor from the startup provider.
                if snapshot.helper_ready && helper_provider(snapshot.provider) {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            ProductionRouteAdapter::MarketDataMarketsRead => {
                if helper_provider(snapshot.provider) && snapshot.helper_ready
                    || snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            ProductionRouteAdapter::MarketDataSnapshotsRead
            | ProductionRouteAdapter::MarketDataBatchSnapshotsWrite
            | ProductionRouteAdapter::MarketDataSubscriptionRead
            | ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite
            | ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite
            | ProductionRouteAdapter::MarketDataSubscriptionClearWrite
            | ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite => {
                let helper_ready = helper_provider(snapshot.provider) && snapshot.helper_ready;
                let futu_ready = snapshot.provider
                    == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.router_ready
                    && snapshot.opend_ready;
                if helper_ready || futu_ready {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            _ => startup_binding,
        }
    }

    fn research_binding_for(
        &self,
        operation: &'static str,
        snapshot: &crate::product::product_active_provider_state::ProviderRuntimeSnapshot,
    ) -> ProductionAdapterBinding {
        match operation {
            "instrument" | "financials" => {
                if snapshot.helper_ready && helper_provider(snapshot.provider) {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            "news" => {
                let helper_ready = snapshot.helper_ready && helper_provider(snapshot.provider);
                let futu_ready = snapshot.provider
                    == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.news_reader_available());
                if helper_ready || futu_ready {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            // Futu valuation has an additional trade-runtime capability that
            // is captured by the startup binding. Preserve that decision when
            // Futu remains active, but never advertise it for helper modes.
            "valuation" => {
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                {
                    self.research_bindings
                        .get(operation)
                        .copied()
                        .unwrap_or(ProductionAdapterBinding::ExternalUnavailable)
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            _ => ProductionAdapterBinding::ExternalUnavailable,
        }
    }
}

fn allowed_modes(binding: ProductionAdapterBinding) -> Value {
    if binding == ProductionAdapterBinding::Ready {
        json!(["approval", "less_approval", "all"])
    } else {
        json!([])
    }
}

fn helper_provider(provider: Option<jftrade_settings::MarketDataProvider>) -> bool {
    matches!(
        provider,
        Some(jftrade_settings::MarketDataProvider::Yfinance)
            | Some(jftrade_settings::MarketDataProvider::Akshare)
    )
}

fn is_provider_dynamic_adapter(adapter: ProductionRouteAdapter) -> bool {
    matches!(
        adapter,
        ProductionRouteAdapter::MarketDataSearchRead
            | ProductionRouteAdapter::MarketDataCandlesRead
            | ProductionRouteAdapter::MarketDataSecuritiesRead
            | ProductionRouteAdapter::MarketDataMarketsRead
            | ProductionRouteAdapter::MarketDataSnapshotsRead
            | ProductionRouteAdapter::MarketDataBatchSnapshotsWrite
            | ProductionRouteAdapter::MarketDataSubscriptionRead
            | ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite
            | ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite
            | ProductionRouteAdapter::MarketDataSubscriptionClearWrite
            | ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite
            | ProductionRouteAdapter::MarketDataNewsActionsRead
            | ProductionRouteAdapter::MarketDataNewsSearchRead
            | ProductionRouteAdapter::ResearchScreenWrite
    )
}

#[derive(Clone, Copy)]
struct ProductionToolDefinition {
    id: &'static str,
    category: &'static str,
    display_name: &'static str,
    adapter: ProductionRouteAdapter,
    /// Optional operation-level readiness key.  This is used for research
    /// tools whose public HTTP routes share the `ResearchRead` umbrella.
    research_operation: Option<&'static str>,
}

const PRODUCTION_TOOL_DEFINITIONS: &[ProductionToolDefinition] = &[
    ProductionToolDefinition {
        id: "interaction.request_user",
        category: "interaction",
        display_name: "向用户提问",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "workflow.wait",
        category: "workflow",
        display_name: "等待工作流",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "tools.search",
        category: "system",
        display_name: "搜索工具",
        adapter: ProductionRouteAdapter::AdkChat,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "models.list",
        category: "system",
        display_name: "查询可调用模型",
        adapter: ProductionRouteAdapter::AdkRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "system.status",
        category: "system",
        display_name: "查询系统状态",
        adapter: ProductionRouteAdapter::SystemCore,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "system.futu_opend",
        category: "system",
        display_name: "查询 OpenD 状态",
        adapter: ProductionRouteAdapter::SystemRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "plugins.catalog",
        category: "plugins",
        display_name: "查询插件目录",
        adapter: ProductionRouteAdapter::PluginsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.capabilities",
        category: "market",
        display_name: "查询行情能力",
        adapter: ProductionRouteAdapter::MarketDataProviderRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.search",
        category: "market",
        display_name: "搜索标的",
        adapter: ProductionRouteAdapter::MarketDataSearchRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.snapshot",
        category: "market",
        display_name: "查询行情快照",
        adapter: ProductionRouteAdapter::MarketDataSnapshotsRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.snapshots",
        category: "market",
        display_name: "批量查询行情快照",
        adapter: ProductionRouteAdapter::MarketDataBatchSnapshotsWrite,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.candles",
        category: "market",
        display_name: "查询 K 线",
        adapter: ProductionRouteAdapter::MarketDataCandlesRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.intraday",
        category: "market",
        display_name: "查询分时行情",
        adapter: ProductionRouteAdapter::MarketDataIntradayRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "market.subscriptions",
        category: "market",
        display_name: "查询行情订阅",
        adapter: ProductionRouteAdapter::MarketDataSubscriptionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "watchlist.list",
        category: "watchlist",
        display_name: "查询自选列表",
        adapter: ProductionRouteAdapter::WatchlistRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "research.instrument",
        category: "research",
        display_name: "查询标的信息",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("instrument"),
    },
    ProductionToolDefinition {
        id: "research.financials",
        category: "research",
        display_name: "查询财务数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("financials"),
    },
    ProductionToolDefinition {
        id: "research.valuation",
        category: "research",
        display_name: "查询估值数据",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("valuation"),
    },
    ProductionToolDefinition {
        id: "research.news",
        category: "research",
        display_name: "查询研究新闻",
        adapter: ProductionRouteAdapter::ResearchRead,
        research_operation: Some("news"),
    },
    ProductionToolDefinition {
        id: "research.screen",
        category: "research",
        display_name: "执行研究筛选",
        adapter: ProductionRouteAdapter::ResearchScreenWrite,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.accounts",
        category: "portfolio",
        display_name: "查询账户",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.overview",
        category: "portfolio",
        display_name: "查询组合概览",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "portfolio.positions",
        category: "portfolio",
        display_name: "查询持仓",
        adapter: ProductionRouteAdapter::PortfolioRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "account.orders",
        category: "account",
        display_name: "查询订单",
        adapter: ProductionRouteAdapter::ExecutionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "risk.state",
        category: "risk",
        display_name: "查询风控状态",
        adapter: ProductionRouteAdapter::SystemCore,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.definitions",
        category: "strategy",
        display_name: "查询策略定义",
        adapter: ProductionRouteAdapter::StrategyDefinitionRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.validate_pine",
        category: "strategy",
        display_name: "校验 Pine 策略",
        adapter: ProductionRouteAdapter::StrategyPine,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "strategy.research_backtest",
        category: "strategy",
        display_name: "执行策略回测",
        adapter: ProductionRouteAdapter::BacktestStart,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.runs",
        category: "backtest",
        display_name: "查询回测运行",
        adapter: ProductionRouteAdapter::BacktestRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.result_view",
        category: "backtest",
        display_name: "查询回测结果",
        adapter: ProductionRouteAdapter::BacktestRead,
        research_operation: None,
    },
    ProductionToolDefinition {
        id: "backtest.kline_sync_status",
        category: "backtest",
        display_name: "查询 K 线同步状态",
        adapter: ProductionRouteAdapter::BacktestSyncRead,
        research_operation: None,
    },
];

#[path = "product_production_ports_adk_read.rs"]
mod read;

include!("product_production_ports_adk_stream.rs");

#[cfg(test)]
#[path = "product_production_ports_adk_tests.rs"]
mod tests;
