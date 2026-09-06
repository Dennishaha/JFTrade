//! Runtime-owned execution for ADK function calls.
//!
//! The model adapter deliberately does not treat a function call as a final
//! answer.  Calls are staged for approval and, once approved, are executed by
//! this process-local executor.  Only capabilities with a concrete local
//! implementation are handled here; every other call fails closed with a
//! 503-compatible error instead of returning synthetic data.

use std::sync::{Arc, RwLock};

use serde_json::{Value, json};

use crate::product::product_mcp_production_executor::ProductionMcpToolExecutor;
use crate::product::product_production_ports::{ProductionPortBundle, ProductionToolCatalog};
use jftrade_store_sqlite::AdkStore;

pub(crate) trait AdkToolExecutor: Send + Sync + std::fmt::Debug {
    /// Whether this process owns a concrete implementation for the named
    /// capability.  The model request is filtered with this predicate so a
    /// production route can never be advertised as executable merely because
    /// a descriptor exists in the catalog.
    fn supports(&self, name: &str) -> bool;
    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String>;
    fn attach_ports(&self, _ports: Arc<ProductionPortBundle>) {}
    fn detach_ports(&self) {}
}

pub(crate) struct ProductionAdkToolExecutor {
    catalog: Arc<ProductionToolCatalog>,
    store: Arc<AdkStore>,
    mcp_executor: Arc<RwLock<Option<ProductionMcpToolExecutor>>>,
    ports: Arc<RwLock<Option<Arc<ProductionPortBundle>>>>,
}

impl std::fmt::Debug for ProductionAdkToolExecutor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProductionAdkToolExecutor")
            .field("catalog", &self.catalog)
            .field("store", &self.store)
            .finish()
    }
}

impl ProductionAdkToolExecutor {
    pub(crate) fn new(catalog: Arc<ProductionToolCatalog>, store: Arc<AdkStore>) -> Self {
        Self {
            catalog,
            store,
            mcp_executor: Arc::new(RwLock::new(None)),
            ports: Arc::new(RwLock::new(None)),
        }
    }

    #[allow(dead_code)]
    pub(crate) fn with_ports(
        catalog: Arc<ProductionToolCatalog>,
        store: Arc<AdkStore>,
        ports: Arc<ProductionPortBundle>,
    ) -> Self {
        let mcp = ProductionMcpToolExecutor::from_production_ports(ports.clone());
        Self {
            catalog,
            store,
            mcp_executor: Arc::new(RwLock::new(Some(mcp))),
            ports: Arc::new(RwLock::new(Some(ports))),
        }
    }

    pub(crate) fn attach_ports(&self, ports: Arc<ProductionPortBundle>) {
        if let Ok(mut guard) = self.ports.write() {
            *guard = Some(ports.clone());
        }
        if let Ok(mut guard) = self.mcp_executor.write() {
            *guard = Some(ProductionMcpToolExecutor::from_production_ports(ports));
        }
    }

    pub(crate) fn detach_ports(&self) {
        if let Ok(mut guard) = self.ports.write() {
            *guard = None;
        }
        if let Ok(mut guard) = self.mcp_executor.write() {
            *guard = None;
        }
    }
}

impl AdkToolExecutor for ProductionAdkToolExecutor {
    fn supports(&self, name: &str) -> bool {
        if matches!(
            name,
            "tools.search" | "models.list" | "strategy.validate_pine" | "strategy.pine_spec"
        ) {
            return true;
        }
        if matches!(
            name,
            "portfolio.accounts"
                | "portfolio.overview"
                | "portfolio.positions"
                | "strategy.research_backtest"
        ) {
            return self.ports.read().map(|g| g.is_some()).unwrap_or(false);
        }
        if let Ok(guard) = self.mcp_executor.read()
            && let Some(mcp) = guard.as_ref()
        {
            return mcp.supports(name);
        }
        false
    }

    fn attach_ports(&self, ports: Arc<ProductionPortBundle>) {
        ProductionAdkToolExecutor::attach_ports(self, ports);
    }

    fn detach_ports(&self) {
        ProductionAdkToolExecutor::detach_ports(self);
    }

    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String> {
        match name {
            // This is a real local operation: the result is projected from
            // the same catalog exposed by GET /api/v1/adk/tools.
            "tools.search" => {
                let query = arguments
                    .get("query")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .unwrap_or_default()
                    .to_ascii_lowercase();
                let tools = self
                    .catalog
                    .callable_tools()
                    .into_iter()
                    .filter(|tool| {
                        query.is_empty()
                            || ["id", "name", "displayName"]
                                .into_iter()
                                .filter_map(|field| tool.get(field).and_then(Value::as_str))
                                .any(|value| value.to_ascii_lowercase().contains(&query))
                    })
                    .collect::<Vec<_>>();
                Ok(json!({"tools": tools, "total": tools.len()}))
            }
            // Providers are durable ADK entities.  Return their persisted
            // payloads rather than a fabricated list or a process-local cache.
            "models.list" => {
                let providers = self
                    .store
                    .list_providers()
                    .map_err(|error| format!("list model providers: {error}"))?
                    .into_iter()
                    .map(|provider| {
                        serde_json::from_str::<Value>(&provider.payload_json)
                            .map(|mut value| {
                                if let Some(object) = value.as_object_mut() {
                                    // Provider credentials must never be
                                    // exposed to the model through a tool
                                    // result.  The HTTP provider projection
                                    // follows the same redaction rule.
                                    object.remove("apiKey");
                                    object.insert("id".to_owned(), Value::String(provider.id));
                                    object.insert(
                                        "createdAt".to_owned(),
                                        Value::String(provider.created_at),
                                    );
                                    object.insert(
                                        "updatedAt".to_owned(),
                                        Value::String(provider.updated_at),
                                    );
                                }
                                value
                            })
                            .map_err(|error| format!("invalid model provider payload: {error}"))
                    })
                    .collect::<Result<Vec<_>, _>>()?;
                Ok(json!({"providers": providers, "total": providers.len()}))
            }
            "strategy.validate_pine" | "strategy.pine_spec" => {
                crate::product::strategy_pine_mcp::dispatch_strategy_pine_mcp(name, arguments)
                    .map_err(|error| error.message)
            }
            "portfolio.accounts" => {
                let guard = self
                    .ports
                    .read()
                    .map_err(|_| "tool executor lock poisoned".to_owned())?;
                let ports = guard
                    .as_ref()
                    .ok_or_else(|| format!("tool executor is unavailable for {name}"))?;
                crate::product::product_portfolio_projection::execute_portfolio_accounts(
                    ports, arguments,
                )
            }
            "portfolio.overview" => {
                let guard = self
                    .ports
                    .read()
                    .map_err(|_| "tool executor lock poisoned".to_owned())?;
                let ports = guard
                    .as_ref()
                    .ok_or_else(|| format!("tool executor is unavailable for {name}"))?;
                crate::product::product_portfolio_projection::execute_portfolio_overview(
                    ports, arguments,
                )
            }
            "portfolio.positions" => {
                let guard = self
                    .ports
                    .read()
                    .map_err(|_| "tool executor lock poisoned".to_owned())?;
                let ports = guard
                    .as_ref()
                    .ok_or_else(|| format!("tool executor is unavailable for {name}"))?;
                crate::product::product_portfolio_projection::execute_portfolio_positions(
                    ports, arguments,
                )
            }
            "strategy.research_backtest" => {
                let guard = self
                    .ports
                    .read()
                    .map_err(|_| "tool executor lock poisoned".to_owned())?;
                let ports = guard
                    .as_ref()
                    .ok_or_else(|| format!("tool executor is unavailable for {name}"))?;
                crate::product::product_research_backtest_projection::execute_research_backtest(
                    ports, arguments,
                )
            }
            _ => {
                let guard = self
                    .mcp_executor
                    .read()
                    .map_err(|_| "tool executor lock poisoned".to_owned())?;
                if let Some(mcp) = guard.as_ref() {
                    mcp.execute_production(name, arguments)
                        .map_err(|error| error.message)
                } else {
                    Err(format!("tool executor is unavailable for {name}"))
                }
            }
        }
    }
}
