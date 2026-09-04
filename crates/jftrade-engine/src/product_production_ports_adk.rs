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
#[path = "product_production_ports_adk_mcp.rs"]
mod mcp;
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

impl Drop for ProductionAdkPort {
    fn drop(&mut self) {
        // A direct production-port bundle drop is a supported lifecycle path
        // in tests and in startup rollback.  Stop the concrete runtime before
        // releasing its last Arc so its recovery scanner cannot retain the
        // ADK writer lease for one polling interval after the bundle is gone.
        if let Some(runtime) = self.chat_runtime.as_deref() {
            runtime.shutdown();
        }
    }
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
    pine_readiness: Option<Arc<jftrade_integration_pine::PineReadinessState>>,
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
            ("institutions", unavailable),
            ("analyst", unavailable),
            ("ownership", unavailable),
            ("corporate_actions", unavailable),
            ("short_interest", unavailable),
            ("technical_indicators", unavailable),
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
            let binding = if definition.id == "strategy.validate_pine" {
                // Pine validation is implemented by the reviewed native Rust
                // subset and does not call the managed PineTS worker.  Keep it
                // callable even when the worker-backed analysis/backtest
                // adapter is unavailable.
                Some(ProductionAdapterBinding::Ready)
            } else {
                match definition.research_operation {
                    Some(operation) => research_bindings
                        .get(operation)
                        .copied()
                        .or_else(|| bindings.get(&definition.adapter).copied()),
                    None => bindings.get(&definition.adapter).copied(),
                }
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
            pine_readiness: None,
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

    pub(crate) fn with_pine_readiness(
        mut self,
        readiness: Option<Arc<jftrade_integration_pine::PineReadinessState>>,
    ) -> Self {
        self.pine_readiness = readiness;
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
                if name == "interaction.request_user" {
                    return Some(json!({
                        "type": "function",
                        "name": name,
                        "description": "向用户提问以解决关键阻塞问题（缺少必要信息、重大取舍、越界授权）。用户回答后将恢复执行并继续完成原始请求。",
                        "parameters": {
                            "type": "object",
                            "properties": {
                                "title": {"type": "string", "description": "提问标题"},
                                "decisionKind": {
                                    "type": "string",
                                    "enum": ["missing_required_context", "material_tradeoff", "scope_boundary"],
                                    "description": "阻塞类型"
                                },
                                "blockingReason": {"type": "string", "description": "为什么必须由用户决策的原因"},
                                "questions": {
                                    "type": "array",
                                    "items": {
                                        "type": "object",
                                        "properties": {
                                            "question": {"type": "string"},
                                            "options": {
                                                "type": "array",
                                                "items": {
                                                    "type": "object",
                                                    "properties": {
                                                        "label": {"type": "string"},
                                                        "description": {"type": "string"},
                                                        "recommended": {"type": "boolean"}
                                                    },
                                                    "required": ["label"]
                                                }
                                            },
                                            "allowOther": {"type": "boolean"}
                                        },
                                        "required": ["question"]
                                    }
                                }
                            },
                            "required": ["decisionKind", "blockingReason", "questions"]
                        }
                    }));
                }
                if let Some(schema) = crate::product::product_mcp_protocol::try_schema_for(&name) {
                    return Some(json!({
                        "type": "function",
                        "name": name,
                        "description": description,
                        "parameters": schema,
                    }));
                }
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
        if definition.id == "strategy.validate_pine" {
            // This tool compiles and validates the supported Pine subset
            // entirely in-process.  Its readiness is intentionally independent
            // of the external PineTS execution worker.
            return ProductionAdapterBinding::Ready;
        }
        if let Some(operation) = definition.research_operation {
            return self.research_binding_for(operation, snapshot);
        }
        let Some(startup_binding) = self.bindings.get(&definition.adapter).copied() else {
            // Construction validates every definition. Keep this defensive
            // branch fail-closed if a future definition is added without
            // updating the binding table.
            return ProductionAdapterBinding::ExternalUnavailable;
        };
        if definition.id == "strategy.research_backtest" {
            // Backtests consume the verified PineTS worker and local candle
            // store.  Live helper/OpenD/router availability is checked only
            // by other market-data routes; an empty local range is mapped by
            // the backtest handler to BACKTESTS_WRITE_UNAVAILABLE.
            // The backtest provider is persisted independently from the live
            // quote provider, so a missing live-provider selection must not
            // hide an otherwise verified execution worker.
            return if self.backtest_execution_ready
                && self
                    .pine_readiness
                    .as_ref()
                    .is_some_and(|readiness| readiness.is_ready())
            {
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
            "institutions" => {
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.institution_reader_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            "short_interest" => {
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.short_interest_reader_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                }
            }
            "technical_indicators" => {
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.technical_indicator_reader_available())
                {
                    ProductionAdapterBinding::Ready
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

include!("product_production_ports_adk_catalog.rs");


#[path = "product_production_ports_adk_read.rs"]
mod read;

include!("product_production_ports_adk_stream.rs");

#[cfg(test)]
#[path = "product_production_ports_adk_tests.rs"]
mod tests;
