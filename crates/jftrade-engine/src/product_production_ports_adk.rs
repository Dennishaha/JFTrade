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

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};
use crate::product::{
    AdkReadEvent, AdkReadSnapshot, AdkReadSnapshotError, AdkReadSnapshotPort, AdkReadStream,
};
use super::ProductionAdapterBinding;
use crate::product::product_production_route_registry::ProductionRouteAdapter;

#[path = "product_production_ports_adk_mutation.rs"]
mod mutation;
#[path = "product_production_ports_adk_metrics.rs"]
mod metrics;
#[path = "product_production_ports_adk_projection.rs"]
mod projection;

use projection::{
    builtin_agent, builtin_skills, composer_state_value, dynamic_id, invalid_payload,
    is_deleted_payload, normalize_memory_key, not_found, page, payload, put_string,
    query_param, session_entity_value, timeline_value, workflow_trigger_value,
};

#[derive(Debug)]
pub(crate) struct ProductionAdkPort {
    pub(crate) store: Arc<AdkStore>,
    pub(crate) session_store: Arc<AdkSessionStore>,
    pub(crate) artifact_store: Arc<AdkArtifactStore>,
    pub(crate) tool_catalog: Arc<ProductionToolCatalog>,
    pub(crate) settings_path: PathBuf,
}

#[derive(Clone, Debug)]
pub(crate) struct ProductionToolCatalog {
    tools: Vec<Value>,
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
        Ok(Self { tools })
    }

    fn values(&self) -> Vec<Value> {
        self.tools.clone()
    }

    fn ids(&self) -> Vec<String> {
        self.tools
            .iter()
            .filter_map(|tool| tool.get("id").and_then(Value::as_str))
            .map(str::to_owned)
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
    ProductionToolDefinition { id: "interaction.request_user", category: "interaction", display_name: "向用户提问", adapter: ProductionRouteAdapter::AdkChat, research_operation: None },
    ProductionToolDefinition { id: "workflow.wait", category: "workflow", display_name: "等待工作流", adapter: ProductionRouteAdapter::AdkChat, research_operation: None },
    ProductionToolDefinition { id: "tools.search", category: "system", display_name: "搜索工具", adapter: ProductionRouteAdapter::AdkChat, research_operation: None },
    ProductionToolDefinition { id: "models.list", category: "system", display_name: "查询可调用模型", adapter: ProductionRouteAdapter::AdkRead, research_operation: None },
    ProductionToolDefinition { id: "system.status", category: "system", display_name: "查询系统状态", adapter: ProductionRouteAdapter::SystemCore, research_operation: None },
    ProductionToolDefinition { id: "system.futu_opend", category: "system", display_name: "查询 OpenD 状态", adapter: ProductionRouteAdapter::SystemRead, research_operation: None },
    ProductionToolDefinition { id: "plugins.catalog", category: "plugins", display_name: "查询插件目录", adapter: ProductionRouteAdapter::PluginsRead, research_operation: None },
    ProductionToolDefinition { id: "market.capabilities", category: "market", display_name: "查询行情能力", adapter: ProductionRouteAdapter::MarketDataProviderRead, research_operation: None },
    ProductionToolDefinition { id: "market.search", category: "market", display_name: "搜索标的", adapter: ProductionRouteAdapter::MarketDataSearchRead, research_operation: None },
    ProductionToolDefinition { id: "market.snapshot", category: "market", display_name: "查询行情快照", adapter: ProductionRouteAdapter::MarketDataSnapshotsRead, research_operation: None },
    ProductionToolDefinition { id: "market.snapshots", category: "market", display_name: "批量查询行情快照", adapter: ProductionRouteAdapter::MarketDataBatchSnapshotsWrite, research_operation: None },
    ProductionToolDefinition { id: "market.candles", category: "market", display_name: "查询 K 线", adapter: ProductionRouteAdapter::MarketDataCandlesRead, research_operation: None },
    ProductionToolDefinition { id: "market.intraday", category: "market", display_name: "查询分时行情", adapter: ProductionRouteAdapter::MarketDataIntradayRead, research_operation: None },
    ProductionToolDefinition { id: "market.subscriptions", category: "market", display_name: "查询行情订阅", adapter: ProductionRouteAdapter::MarketDataSubscriptionRead, research_operation: None },
    ProductionToolDefinition { id: "watchlist.list", category: "watchlist", display_name: "查询自选列表", adapter: ProductionRouteAdapter::WatchlistRead, research_operation: None },
    ProductionToolDefinition { id: "research.instrument", category: "research", display_name: "查询标的信息", adapter: ProductionRouteAdapter::ResearchRead, research_operation: Some("instrument") },
    ProductionToolDefinition { id: "research.financials", category: "research", display_name: "查询财务数据", adapter: ProductionRouteAdapter::ResearchRead, research_operation: Some("financials") },
    ProductionToolDefinition { id: "research.valuation", category: "research", display_name: "查询估值数据", adapter: ProductionRouteAdapter::ResearchRead, research_operation: Some("valuation") },
    ProductionToolDefinition { id: "research.news", category: "research", display_name: "查询研究新闻", adapter: ProductionRouteAdapter::ResearchRead, research_operation: Some("news") },
    ProductionToolDefinition { id: "research.screen", category: "research", display_name: "执行研究筛选", adapter: ProductionRouteAdapter::ResearchScreenWrite, research_operation: None },
    ProductionToolDefinition { id: "portfolio.accounts", category: "portfolio", display_name: "查询账户", adapter: ProductionRouteAdapter::PortfolioRead, research_operation: None },
    ProductionToolDefinition { id: "portfolio.overview", category: "portfolio", display_name: "查询组合概览", adapter: ProductionRouteAdapter::PortfolioRead, research_operation: None },
    ProductionToolDefinition { id: "portfolio.positions", category: "portfolio", display_name: "查询持仓", adapter: ProductionRouteAdapter::PortfolioRead, research_operation: None },
    ProductionToolDefinition { id: "account.orders", category: "account", display_name: "查询订单", adapter: ProductionRouteAdapter::ExecutionRead, research_operation: None },
    ProductionToolDefinition { id: "risk.state", category: "risk", display_name: "查询风控状态", adapter: ProductionRouteAdapter::SystemCore, research_operation: None },
    ProductionToolDefinition { id: "strategy.definitions", category: "strategy", display_name: "查询策略定义", adapter: ProductionRouteAdapter::StrategyDefinitionRead, research_operation: None },
    ProductionToolDefinition { id: "strategy.validate_pine", category: "strategy", display_name: "校验 Pine 策略", adapter: ProductionRouteAdapter::StrategyPine, research_operation: None },
    ProductionToolDefinition { id: "strategy.research_backtest", category: "strategy", display_name: "执行策略回测", adapter: ProductionRouteAdapter::BacktestStart, research_operation: None },
    ProductionToolDefinition { id: "backtest.runs", category: "backtest", display_name: "查询回测运行", adapter: ProductionRouteAdapter::BacktestRead, research_operation: None },
    ProductionToolDefinition { id: "backtest.result_view", category: "backtest", display_name: "查询回测结果", adapter: ProductionRouteAdapter::BacktestRead, research_operation: None },
    ProductionToolDefinition { id: "backtest.kline_sync_status", category: "backtest", display_name: "查询 K 线同步状态", adapter: ProductionRouteAdapter::BacktestSyncRead, research_operation: None },
];

impl From<AdkStoreError> for AdkReadSnapshotError {
    fn from(error: AdkStoreError) -> Self {
        Self::Unavailable(error.to_string())
    }
}

impl From<AdkSessionStoreError> for AdkReadSnapshotError {
    fn from(error: AdkSessionStoreError) -> Self {
        Self::Unavailable(error.to_string())
    }
}

impl AdkReadSnapshotPort for ProductionAdkPort {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        match path {
            "/api/v1/adk" => self.snapshot(),
            "/api/v1/adk/agents" => self.agents(query),
            "/api/v1/adk/providers" => self.providers(),
            "/api/v1/adk/skills" => self.skills(),
            "/api/v1/adk/tasks" => self.tasks(query),
            "/api/v1/adk/workflows" => self.workflows(query),
            "/api/v1/adk/approvals" => self.approvals(query),
            "/api/v1/adk/runs" => self.runs(query),
            "/api/v1/adk/sessions" => self.sessions(query),
            "/api/v1/adk/memory" => self.memories(query),
            "/api/v1/adk/audit" => self.audit(query),
            "/api/v1/adk/optimization-tasks" => self.optimization_tasks(query),
            "/api/v1/adk/workflow-trigger-logs" => self.workflow_logs(query),
            "/api/v1/adk/metrics" => self.metrics(),
            "/api/v1/adk/tools" => Ok(AdkReadSnapshot::Json(json!({"tools": self.tool_catalog.values()}))),
            _ => self.dynamic(path),
        }
    }
}

impl ProductionAdkPort {
    fn snapshot(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut agents = self.entities(self.store.list_agents()?, "agent")?;
        if agents.is_empty() {
            agents.push(builtin_agent(&self.tool_catalog));
        }
        let providers = self.entities(self.store.list_providers()?, "provider")?;
        let mut skills = self.entities(self.store.list_skills()?, "skill")?;
        if skills.is_empty() {
            skills = builtin_skills(&self.tool_catalog);
        }
        Ok(AdkReadSnapshot::Json(json!({
            "agents": agents,
            "providers": providers,
            "skills": skills,
            "tools": self.tool_catalog.values(),
            "runtimeSettings": self.runtime_settings()?,
        })))
    }

    fn runtime_settings(&self) -> Result<Value, AdkReadSnapshotError> {
        let store = SettingsFileStore::open_read_only(&self.settings_path)
            .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
        let settings = store
            .load_assistant_runtime()
            .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?
            .map(|settings| normalize_assistant_runtime_settings(&settings))
            .unwrap_or_default();
        serde_json::to_value(settings).map_err(|error| {
            AdkReadSnapshotError::Unavailable(format!(
                "encode ADK runtime settings: {error}"
            ))
        })
    }

    fn entities(
        &self,
        rows: Vec<jftrade_store_sqlite::StoredAdkEntity>,
        kind: &str,
    ) -> Result<Vec<Value>, AdkReadSnapshotError> {
        rows.into_iter()
            .map(|row| {
                let mut value: Value = serde_json::from_str(&row.payload_json)
                    .map_err(|error| invalid_payload(kind, error))?;
                put_string(&mut value, "id", row.id);
                put_string(&mut value, "createdAt", row.created_at);
                put_string(&mut value, "updatedAt", row.updated_at);
                Ok(value)
            })
            .collect()
    }

    fn agents(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut items = self.entities(self.store.list_agents()?, "agent")?;
        if items.is_empty() {
            items.push(builtin_agent(&self.tool_catalog));
        }
        Ok(AdkReadSnapshot::Json(page("agents", items, query, 100)))
    }

    fn providers(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        Ok(AdkReadSnapshot::Json(json!({
            "providers": self.entities(self.store.list_providers()?, "provider")?
        })))
    }

    fn skills(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let mut skills = self.entities(self.store.list_skills()?, "skill")?;
        if skills.is_empty() {
            skills = builtin_skills(&self.tool_catalog);
        }
        Ok(AdkReadSnapshot::Json(json!({"skills": skills})))
    }

    fn tasks(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_tasks()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "task", [
                ("id", row.id), ("status", row.status), ("agentId", row.agent_id),
                ("runId", row.run_id), ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        let status = query_param(query, "status")
            .map(|value| value.trim().to_ascii_uppercase())
            .filter(|value| !value.is_empty());
        if let Some(status) = status.as_deref()
            && !matches!(status, "TODO" | "IN_PROGRESS" | "BLOCKED" | "DONE" | "CANCELLED")
        {
            return Err(AdkReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid tasks query".to_owned(),
                retry_after_seconds: None,
            });
        }
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let run_id = query_param(query, "runId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let values = values
            .into_iter()
            .filter(|value| {
                let matches_status = status.as_deref().is_none_or(|expected| {
                    value.get("status").and_then(Value::as_str) == Some(expected)
                });
                let matches_agent = agent_id.as_deref().is_none_or(|expected| {
                    value.get("agentId").and_then(Value::as_str) == Some(expected)
                });
                let matches_run = run_id.as_deref().is_none_or(|expected| {
                    value.get("runId").and_then(Value::as_str) == Some(expected)
                });
                matches_status && matches_agent && matches_run
            })
            .collect();
        Ok(AdkReadSnapshot::Json(page("tasks", values, query, 20)))
    }

    fn workflows(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_workflows()?
            .into_iter()
            .map(|row| {
                if is_deleted_payload(&row.payload_json, "workflow")? {
                    return Ok(None);
                }
                payload(
                    &row.payload_json,
                    "workflow",
                    [
                        ("id", row.id),
                        ("status", row.status),
                        ("createdAt", row.created_at),
                        ("updatedAt", row.updated_at),
                    ],
                )
                .map(Some)
            })
            .collect::<Result<Vec<_>, _>>()?
            .into_iter()
            .flatten()
            .collect::<Vec<_>>();
        Ok(AdkReadSnapshot::Json(page("workflows", values, query, 100)))
    }

    fn approvals(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_approvals()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "approval", [
                ("id", row.id), ("runId", row.run_id), ("agentId", row.agent_id),
                ("status", row.status), ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("approvals", values, query, 100)))
    }

    fn runs(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_runs()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "run", [
                ("id", row.id), ("sessionId", row.session_id), ("agentId", row.agent_id),
                ("status", row.status), ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("runs", values, query, 100)))
    }

    fn sessions(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_sessions()
            ?
            .into_iter()
            .map(session_entity_value)
            .collect::<Result<Vec<_>, _>>()?;
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let title_query = query_param(query, "query")
            .map(|value| value.trim().to_lowercase())
            .filter(|value| !value.is_empty());
        let values = values
            .into_iter()
            .filter(|value| {
                let matches_agent = agent_id.as_deref().is_none_or(|agent| {
                    value.get("agentId").and_then(Value::as_str) == Some(agent)
                });
                let matches_title = title_query.as_deref().is_none_or(|needle| {
                    value
                        .get("title")
                        .and_then(Value::as_str)
                        .is_some_and(|title| title.to_lowercase().contains(needle))
                });
                matches_agent && matches_title
            })
            .collect();
        Ok(AdkReadSnapshot::Json(page("sessions", values, query, 100)))
    }

    fn memories(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let scope = query_param(query, "scope")
            .map(|value| value.trim().to_ascii_lowercase())
            .filter(|value| !value.is_empty());
        if let Some(scope) = scope.as_deref()
            && scope != "workspace" && scope != "agent"
        {
            return Err(AdkReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "memory scope must be workspace or agent".to_owned(),
                retry_after_seconds: None,
            });
        }
        let agent_id = query_param(query, "agentId")
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty());
        let key = query_param(query, "key")
            .map(|value| normalize_memory_key(&value))
            .filter(|value| !value.is_empty());
        let values = self
            .store
            .list_memories()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "memory", [
                ("id", row.id), ("agentId", row.agent_id), ("scope", row.scope),
                ("key", row.memory_key), ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        let values = values
            .into_iter()
            .filter(|value| {
                let row_scope = value.get("scope").and_then(Value::as_str).unwrap_or_default();
                let row_agent = value.get("agentId").and_then(Value::as_str).unwrap_or_default();
                let row_key = value.get("key").and_then(Value::as_str).unwrap_or_default();
                let matches_scope = scope.as_deref().is_none_or(|expected| row_scope == expected);
                let matches_agent = match scope.as_deref() {
                    Some("agent") => agent_id.as_deref().is_none_or(|expected| row_agent == expected),
                    Some("workspace") => row_agent.is_empty(),
                    _ => agent_id
                        .as_deref()
                        .is_none_or(|expected| row_scope == "workspace" || row_agent == expected),
                };
                let matches_key = key.as_deref().is_none_or(|expected| row_key == expected);
                matches_scope && matches_agent && matches_key
            })
            .collect::<Vec<_>>();
        Ok(AdkReadSnapshot::Json(json!({"entries": values})))
    }

    fn audit(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_audit_events()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "audit event", [
                ("id", row.id), ("kind", row.kind), ("subjectId", row.subject_id),
                ("createdAt", row.created_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("events", values, query, 100)))
    }

    fn optimization_tasks(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_optimization_tasks()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "optimization task", [
                ("id", row.id), ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("tasks", values, query, 100)))
    }

    fn workflow_logs(&self, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let values = self
            .store
            .list_workflow_trigger_logs()?
            .into_iter()
            .map(|row| payload(&row.payload_json, "workflow trigger log", [
                ("id", row.id), ("workflowId", row.workflow_id), ("triggerId", row.trigger_id),
                ("triggerType", row.trigger_type), ("status", row.status), ("runId", row.run_id),
                ("createdAt", row.created_at), ("updatedAt", row.updated_at),
            ]))
            .collect::<Result<Vec<_>, _>>()?;
        Ok(AdkReadSnapshot::Json(page("logs", values, query, 100)))
    }

    fn metrics(&self) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        metrics::read(self)
    }

    fn dynamic(&self, path: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        if let Some(id) = dynamic_id(path, "/api/v1/adk/optimization-tasks/", "") {
            let Some(row) = self.store.get_optimization_task(id)? else { return Err(not_found("optimization task not found")); };
            return Ok(AdkReadSnapshot::Json(payload(&row.payload_json, "optimization task", [("id", row.id), ("createdAt", row.created_at), ("updatedAt", row.updated_at)])?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/runs/", "/stream") {
            let Some(row) = self.store.get_run(id)? else { return Err(not_found("stream not found")); };
            let value: Value = serde_json::from_str(&row.payload_json).map_err(|e| invalid_payload("run", e))?;
            let Some(events) = value.get("streamEvents").and_then(Value::as_array) else { return Err(not_found("stream not found")); };
            return Ok(AdkReadSnapshot::Stream(AdkReadStream { headers: vec![("X-ADK-Stream-ID".into(), id.into())], events: events.iter().enumerate().map(|(index, data)| AdkReadEvent { id: Some(index.to_string()), data: data.clone() }).collect() }));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/runs/", "") {
            let Some(row) = self.store.get_run(id)? else { return Err(not_found("run not found")); };
            return Ok(AdkReadSnapshot::Json(payload(&row.payload_json, "run", [("id", row.id), ("sessionId", row.session_id), ("agentId", row.agent_id), ("status", row.status), ("createdAt", row.created_at), ("updatedAt", row.updated_at)])?));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/sessions/", "/context") {
            if self.store.get_session(id)?.is_none() {
                return Err(not_found("session not found"));
            }
            if let Some(state) = self.store.get_session_context(id)? { return Ok(AdkReadSnapshot::Json(payload(&state.payload_json, "session context", [("sessionId", id.into())])?)); }
            return Err(AdkReadSnapshotError::Unavailable(
                "session context is not persisted".to_owned(),
            ));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/sessions/", "") {
            let Some(session) = self.store.get_session(id)? else { return Err(not_found("session not found")); };
            let timeline = self.session_store.list_events(id).map_err(|e| AdkReadSnapshotError::Unavailable(e.to_string()))?.into_iter().enumerate().map(|(sequence, event)| timeline_value(event, sequence)).collect::<Vec<_>>();
            let runs = self.store.list_runs()?.into_iter().filter(|run| run.session_id == id).map(|run| payload(&run.payload_json, "run", [("id", run.id), ("sessionId", run.session_id), ("agentId", run.agent_id), ("status", run.status), ("createdAt", run.created_at), ("updatedAt", run.updated_at)])).collect::<Result<Vec<_>, _>>()?;
            let artifacts = self.artifact_store.list_session_artifacts(id).map_err(|e| AdkReadSnapshotError::Unavailable(e.to_string()))?.into_iter().map(|artifact| { let part: Value = serde_json::from_str(&artifact.part_json).map_err(|e| invalid_payload("artifact", e))?; Ok(json!({"appName": artifact.app_name, "userId": artifact.user_id, "sessionId": artifact.session_id, "fileName": artifact.file_name, "version": artifact.version, "part": part, "mimeType": artifact.mime_type, "customMetadata": artifact.custom_metadata_json.as_deref().map(serde_json::from_str::<Value>).transpose().map_err(|e| invalid_payload("artifact metadata", e))?, "createdAt": artifact.created_at, "updatedAt": artifact.updated_at})) }).collect::<Result<Vec<_>, AdkReadSnapshotError>>()?;
            return Ok(AdkReadSnapshot::Json(json!({"session": session_entity_value(session)?, "timeline": timeline, "runs": runs, "artifacts": artifacts, "composerState": composer_state_value(id, self.store.get_session_composer_state(id)?)?})));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/tasks/", "") { let Some(row) = self.store.get_task(id)? else { return Err(not_found("task not found")); }; return Ok(AdkReadSnapshot::Json(payload(&row.payload_json, "task", [("id", row.id), ("status", row.status), ("agentId", row.agent_id), ("runId", row.run_id), ("createdAt", row.created_at), ("updatedAt", row.updated_at)])?)); }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/workflows/", "/triggers") {
            if self.store.get_workflow(id)?.is_none() {
                return Err(not_found("workflow not found"));
            }
            let values = self
                .store
                .list_workflow_triggers(id)?
                .into_iter()
                .map(|row| {
                    let deleted = is_deleted_payload(&row.payload_json, "workflow trigger")?;
                    if deleted {
                        return Ok(None);
                    }
                    workflow_trigger_value(
                        &row.payload_json,
                        [
                            ("id", row.id),
                            ("workflowId", row.workflow_id),
                            ("type", row.trigger_type),
                            ("status", row.status),
                            ("nextRunAt", row.next_run_at),
                            ("createdAt", row.created_at),
                            ("updatedAt", row.updated_at),
                        ],
                    )
                    .map(Some)
                })
                .collect::<Result<Vec<_>, _>>()?
                .into_iter()
                .flatten()
                .collect::<Vec<_>>();
            return Ok(AdkReadSnapshot::Json(json!({"triggers": values})));
        }
        if let Some(id) = dynamic_id(path, "/api/v1/adk/workflows/", "") {
            let Some(row) = self.store.get_workflow(id)? else {
                return Err(not_found("workflow not found"));
            };
            if is_deleted_payload(&row.payload_json, "workflow")? {
                return Err(not_found("workflow not found"));
            }
            return Ok(AdkReadSnapshot::Json(payload(
                &row.payload_json,
                "workflow",
                [
                    ("id", row.id),
                    ("status", row.status),
                    ("createdAt", row.created_at),
                    ("updatedAt", row.updated_at),
                ],
            )?));
        }
        if dynamic_id(path, "/api/v1/adk/streams/", "").is_some() { return Err(not_found("stream not found")); }
        Err(not_found("path not found"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tool_catalog_marks_external_unavailable_tools_non_callable() {
        let mut bindings = PRODUCTION_TOOL_DEFINITIONS
            .iter()
            .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
            .collect::<BTreeMap<_, _>>();
        bindings.insert(
            ProductionRouteAdapter::MarketDataSearchRead,
            ProductionAdapterBinding::ExternalUnavailable,
        );

        let catalog = ProductionToolCatalog::from_bindings(&bindings).expect("complete bindings");
        let market_search = catalog
            .tools
            .iter()
            .find(|tool| tool["id"] == "market.search")
            .expect("market search tool");
        assert_eq!(market_search["allowedModes"], json!([]));

        let system_status = catalog
            .tools
            .iter()
            .find(|tool| tool["id"] == "system.status")
            .expect("system status tool");
        assert_eq!(
            system_status["allowedModes"],
            json!(["approval", "less_approval", "all"])
        );
    }

    #[test]
    fn research_tools_use_operation_specific_readiness() {
        let bindings = PRODUCTION_TOOL_DEFINITIONS
            .iter()
            .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
            .collect::<BTreeMap<_, _>>();
        let research = BTreeMap::from([
            ("instrument", ProductionAdapterBinding::Ready),
            ("financials", ProductionAdapterBinding::Ready),
            ("valuation", ProductionAdapterBinding::ExternalUnavailable),
            ("news", ProductionAdapterBinding::ExternalUnavailable),
        ]);
        let catalog = ProductionToolCatalog::from_bindings_with_research(&bindings, &research)
            .expect("complete bindings");

        for (id, callable) in [
            ("research.instrument", true),
            ("research.financials", true),
            ("research.valuation", false),
            ("research.news", false),
        ] {
            let tool = catalog
                .tools
                .iter()
                .find(|tool| tool["id"] == id)
                .expect("research tool");
            assert_eq!(tool["allowedModes"].as_array().is_some_and(|modes| !modes.is_empty()), callable, "{id}");
        }
    }
}

impl AdkChatStreamPort for ProductionAdkPort {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        _input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        Err(AdkChatPortError::Unavailable(
            "assistant model runtime is not configured; configure a model provider in ADK settings"
                .to_owned(),
        ))
    }
}
