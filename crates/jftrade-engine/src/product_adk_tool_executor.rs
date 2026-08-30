//! Runtime-owned execution for ADK function calls.
//!
//! The model adapter deliberately does not treat a function call as a final
//! answer.  Calls are staged for approval and, once approved, are executed by
//! this process-local executor.  Only capabilities with a concrete local
//! implementation are handled here; every other call fails closed with a
//! 503-compatible error instead of returning synthetic data.

use std::sync::Arc;

use serde_json::{Value, json};

use crate::product::product_production_ports::ProductionToolCatalog;
use jftrade_store_sqlite::AdkStore;

pub(crate) trait AdkToolExecutor: Send + Sync + std::fmt::Debug {
    /// Whether this process owns a concrete implementation for the named
    /// capability.  The model request is filtered with this predicate so a
    /// production route can never be advertised as executable merely because
    /// a descriptor exists in the catalog.
    fn supports(&self, name: &str) -> bool;
    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String>;
}

#[derive(Debug)]
pub(crate) struct ProductionAdkToolExecutor {
    catalog: Arc<ProductionToolCatalog>,
    store: Arc<AdkStore>,
}

impl ProductionAdkToolExecutor {
    pub(crate) fn new(catalog: Arc<ProductionToolCatalog>, store: Arc<AdkStore>) -> Self {
        Self { catalog, store }
    }
}

impl AdkToolExecutor for ProductionAdkToolExecutor {
    fn supports(&self, name: &str) -> bool {
        matches!(name, "tools.search" | "models.list")
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
            _ => Err(format!("tool executor is unavailable for {name}")),
        }
    }
}
