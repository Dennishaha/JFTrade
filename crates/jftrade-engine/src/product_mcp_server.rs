//! Rust-owned local MCP Streamable HTTP runtime: stateless, loopback-only,
//! and fail-closed when a concrete runtime is unavailable.
use std::net::{SocketAddr, TcpListener as StdTcpListener};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, Weak};

use super::product_mcp_production_executor::ProductionMcpToolExecutor;
use super::product_mcp_protocol::{
    PRODUCTION_MCP_EXECUTABLE_TOOLS, REVIEWED_READ_ONLY_TOOLS, decode_messages, is_invalid_params,
    is_invalid_request, is_method_not_found, is_modern_protocol, known_method,
    mcp_tool_availability, model_search_text, negotiate_initialize_version, optional_bool,
    optional_integer, optional_string, provider_model, requires_object_params, reviewed_tool_name,
    rpc_request_id, tool_descriptors_with_ports, validate_call_shape, validate_headers,
    validate_message, validate_standard_headers,
};
use super::product_production_ports::ProductionToolCatalog;
use axum::body::{Body, to_bytes};
use axum::extract::{ConnectInfo, State};
use axum::http::{Method, Request, StatusCode, header};
use axum::response::{IntoResponse, Response};
use axum::routing::any;
use axum::{Json, Router};
use jftrade_settings::{
    McpServerRuntimePort, McpServerSettingsRecord, McpServerStatus, verify_mcp_server_token,
};
use jftrade_store_sqlite::AdkStore;
use serde_json::{Value, json};
use tokio::sync::oneshot;

#[cfg(test)]
use super::product_mcp_protocol::tool_descriptors;
const MCP_PATH: &str = "/mcp";
const MCP_RUNTIME_STATUS_URI: &str = "jftrade://runtime/status";
const MCP_MAX_REQUEST_BYTES: usize = 1 << 20;
const MCP_SERVER_NAME: &str = "jftrade";
const MCP_SERVER_VERSION: &str = "1.0";
pub(crate) trait McpToolExecutor: Send + Sync + std::fmt::Debug {
    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String>;
    /// Test executors default to accepting every reviewed name. The production
    /// executor overrides this to its explicit native Rust allowlist.
    fn supports(&self, _name: &str) -> bool {
        true
    }
    fn execute_enveloped(&self, name: &str, arguments: &Value) -> Result<Value, Value> {
        self.execute(name, arguments).map_err(|message| {
            json!({
                "ok": false,
                "error": {"code": "MCP_TOOL_EXECUTION_FAILED", "message": message},
                "status": 503,
            })
        })
    }
}
impl McpToolExecutor for ProductionMcpToolExecutor {
    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String> {
        match name {
            "tools.search" => {
                let query = optional_string(arguments, "query")
                    .unwrap_or_default()
                    .to_ascii_lowercase();
                let tools = self
                    .catalog
                    .callable_tools()
                    .into_iter()
                    .filter(|tool| reviewed_tool_name(tool).is_some())
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
            "models.list" => {
                let query = optional_string(arguments, "query")
                    .unwrap_or_default()
                    .to_ascii_lowercase();
                let provider_id = optional_string(arguments, "providerId").unwrap_or_default();
                let callable_only = optional_bool(arguments, "callableOnly", true);
                let limit = optional_integer(arguments, "limit", 50).clamp(1, 100) as usize;
                let models = self
                    .store
                    .list_providers()
                    .map_err(|error| format!("list model providers: {error}"))?
                    .into_iter()
                    .map(provider_model)
                    .collect::<Result<Vec<_>, _>>()?
                    .into_iter()
                    .filter(|model| {
                        let callable = model
                            .get("callable")
                            .and_then(Value::as_bool)
                            .unwrap_or(false);
                        let provider_matches = provider_id.is_empty()
                            || model.get("providerId").and_then(Value::as_str)
                                == Some(provider_id.as_str());
                        provider_matches
                            && (!callable_only || callable)
                            && (query.is_empty() || model_search_text(model).contains(&query))
                    })
                    .take(limit)
                    .collect::<Vec<_>>();
                let total_returned = models.len();
                Ok(json!({
                    "query": query,
                    "providerId": provider_id,
                    "callableOnly": callable_only,
                    "models": models,
                    "totalReturned": total_returned
                }))
            }
            _ => self
                .execute_production(name, arguments)
                .map_err(|error| error.message),
        }
    }

    fn execute_enveloped(&self, name: &str, arguments: &Value) -> Result<Value, Value> {
        match name {
            "tools.search" | "models.list" => self.execute(name, arguments).map_err(|message| {
                json!({
                    "ok": false,
                    "error": {"code": "MCP_TOOL_EXECUTION_FAILED", "message": message},
                    "status": 503,
                })
            }),
            _ => self
                .execute_production(name, arguments)
                .map_err(|error| error.envelope()),
        }
    }

    fn supports(&self, name: &str) -> bool {
        PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name)
    }
}

struct McpServerState {
    router: Router,
    server: Option<McpServerOwner>,
    bind: Option<String>,
    settings: ActiveMcpSettings,
    last_error: String,
    generation: u64,
    closed: bool,
}
#[derive(Clone, Debug, Eq, PartialEq)]
struct ActiveMcpSettings {
    enabled: bool,
    port: i32,
    auth_mode: String,
    token_hash: String,
}
impl ActiveMcpSettings {
    fn from_record(record: &McpServerSettingsRecord) -> Self {
        Self {
            enabled: record.enabled(),
            port: record.port(),
            auth_mode: record.auth_mode().to_owned(),
            token_hash: record.token_hash().to_owned(),
        }
    }
}
struct McpServerOwner {
    shutdown_tx: Option<oneshot::Sender<()>>,
    thread: Option<std::thread::JoinHandle<Result<(), String>>>,
    running: Arc<AtomicBool>,
}
struct McpShutdownFailure {
    owner: McpServerOwner,
    message: String,
}
impl McpServerOwner {
    fn start(
        listener: StdTcpListener,
        router: Router,
        state: Weak<Mutex<McpServerState>>,
        generation: u64,
    ) -> Result<Self, String> {
        listener
            .set_nonblocking(true)
            .map_err(|error| format!("configure MCP listener: {error}"))?;
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| format!("create MCP runtime: {error}"))?;
        let (shutdown_tx, shutdown_rx) = oneshot::channel();
        let running = Arc::new(AtomicBool::new(true));
        let running_for_thread = Arc::clone(&running);
        let thread = std::thread::Builder::new()
            .name("jftrade-mcp-server".to_owned())
            .spawn(move || {
                let result = runtime.block_on(async move {
                    let listener = tokio::net::TcpListener::from_std(listener)
                        .map_err(|error| error.to_string())?;
                    axum::serve(
                        listener,
                        router.into_make_service_with_connect_info::<SocketAddr>(),
                    )
                    .with_graceful_shutdown(async move {
                        let _ = shutdown_rx.await;
                    })
                    .await
                    .map_err(|error| error.to_string())
                });
                running_for_thread.store(false, Ordering::Release);
                if let Err(error) = &result
                    && let Some(state) = state.upgrade()
                    && let Ok(mut guard) = state.lock()
                    && guard.generation == generation
                {
                    guard.last_error = format!("MCP listener stopped unexpectedly: {error}");
                }
                result
            })
            .map_err(|error| format!("start MCP listener thread: {error}"))?;
        Ok(Self {
            shutdown_tx: Some(shutdown_tx),
            thread: Some(thread),
            running,
        })
    }

    fn is_running(&self) -> bool {
        self.running.load(Ordering::Acquire)
    }

    fn shutdown(mut self) -> Result<(), McpShutdownFailure> {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        let Some(thread) = self.thread.take() else {
            return Ok(());
        };
        match thread.join() {
            Ok(Ok(())) => Ok(()),
            Ok(Err(message)) => Err(McpShutdownFailure {
                owner: self,
                message,
            }),
            Err(_) => Err(McpShutdownFailure {
                owner: self,
                message: "MCP listener thread panicked".to_owned(),
            }),
        }
    }
}
impl Drop for McpServerOwner {
    fn drop(&mut self) {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        if let Some(thread) = self.thread.take()
            && thread.thread().id() != std::thread::current().id()
        {
            let _ = thread.join();
        }
    }
}
struct McpRequestContext {
    state: Weak<Mutex<McpServerState>>,
    catalog: Arc<ProductionToolCatalog>,
    executor: Arc<dyn McpToolExecutor>,
    production_ports: Option<Arc<super::product_production_ports::ProductionPortBundle>>,
}
pub(crate) struct ProductMcpServerRuntime {
    state: Arc<Mutex<McpServerState>>,
}
impl ProductMcpServerRuntime {
    #[allow(dead_code)]
    pub(crate) fn new(catalog: Arc<ProductionToolCatalog>, store: Arc<AdkStore>) -> Arc<Self> {
        Self::with_executor(
            Arc::clone(&catalog),
            Arc::new(ProductionMcpToolExecutor::new(catalog, store)),
        )
    }
    pub(crate) fn from_production_ports(
        ports: Arc<super::product_production_ports::ProductionPortBundle>,
    ) -> Arc<Self> {
        let catalog = Arc::clone(&ports.mcp_catalog);
        Self::with_production_executor(
            catalog,
            Arc::new(ProductionMcpToolExecutor::from_production_ports(
                Arc::clone(&ports),
            )),
            ports,
        )
    }
    fn with_executor(
        catalog: Arc<ProductionToolCatalog>,
        executor: Arc<dyn McpToolExecutor>,
    ) -> Arc<Self> {
        Self::build(catalog, executor, None)
    }

    fn with_production_executor(
        catalog: Arc<ProductionToolCatalog>,
        executor: Arc<dyn McpToolExecutor>,
        ports: Arc<super::product_production_ports::ProductionPortBundle>,
    ) -> Arc<Self> {
        Self::build(catalog, executor, Some(ports))
    }

    fn build(
        catalog: Arc<ProductionToolCatalog>,
        executor: Arc<dyn McpToolExecutor>,
        production_ports: Option<Arc<super::product_production_ports::ProductionPortBundle>>,
    ) -> Arc<Self> {
        let state = Arc::new(Mutex::new(McpServerState {
            router: Router::new(),
            server: None,
            bind: None,
            settings: ActiveMcpSettings {
                enabled: false,
                port: jftrade_settings::DEFAULT_MCP_SERVER_PORT,
                auth_mode: "token".to_owned(),
                token_hash: String::new(),
            },
            last_error: String::new(),
            generation: 0,
            closed: false,
        }));
        let context = Arc::new(McpRequestContext {
            state: Arc::downgrade(&state),
            catalog,
            executor,
            production_ports,
        });
        let router = Router::new()
            .route(MCP_PATH, any(dispatch::handle_mcp_request))
            .with_state(context);
        state
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .router = router;
        Arc::new(Self { state })
    }
    pub(crate) fn shutdown_blocking(&self) -> Result<(), String> {
        let owner = {
            let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            state.closed = true;
            state.settings.enabled = false;
            state.bind = None;
            state.generation = state.generation.wrapping_add(1);
            state.server.take()
        };
        owner.map_or(Ok(()), |owner| {
            owner.shutdown().map_err(|failure| failure.message)
        })
    }
    fn endpoint(port: i32) -> String {
        let port = if port > 0 {
            port
        } else {
            jftrade_settings::DEFAULT_MCP_SERVER_PORT
        };
        format!("http://127.0.0.1:{port}{MCP_PATH}")
    }
}
impl McpServerRuntimePort for ProductMcpServerRuntime {
    fn apply(&self, record: &McpServerSettingsRecord) -> Result<(), String> {
        let next = ActiveMcpSettings::from_record(record);
        let (old_server, rollback) = {
            let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            if state.closed {
                return Err("MCP server runtime is closed".to_owned());
            }
            let previous = (state.settings.clone(), state.bind.clone(), state.generation);
            if !next.enabled {
                state.settings = next;
                state.bind = None;
                state.last_error.clear();
                state.generation = state.generation.wrapping_add(1);
                (state.server.take(), Some(previous))
            } else {
                if next.auth_mode != "none" && next.token_hash.trim().is_empty() {
                    let message = "MCP server token is not configured";
                    state.last_error = message.to_owned();
                    return Err(message.to_owned());
                }
                let desired_bind = format!("127.0.0.1:{}", next.port);
                let same_port = state.bind.as_deref() == Some(desired_bind.as_str());
                if same_port
                    && state
                        .server
                        .as_ref()
                        .is_some_and(McpServerOwner::is_running)
                {
                    state.settings = next;
                    state.last_error.clear();
                    return Ok(());
                }
                let listener = StdTcpListener::bind(&desired_bind).map_err(|error| {
                    let message = format!("MCP server port conflict on {desired_bind}: {error}");
                    state.last_error = message.clone();
                    message
                })?;
                let generation = state.generation.wrapping_add(1);
                let owner = McpServerOwner::start(
                    listener,
                    state.router.clone(),
                    Arc::downgrade(&self.state),
                    generation,
                )
                .inspect_err(|error| {
                    state.last_error = error.clone();
                })?;
                let old_server = state.server.replace(owner);
                state.settings = next;
                state.bind = Some(desired_bind);
                state.last_error.clear();
                state.generation = generation;
                (old_server, Some(previous))
            }
        };
        let Some(old_server) = old_server else {
            return Ok(());
        };
        if let Err(failure) = old_server.shutdown() {
            let error = failure.message.clone();
            let new_server = {
                let mut state = self
                    .state
                    .lock()
                    .unwrap_or_else(|poisoned| poisoned.into_inner());
                let new_server = state.server.take();
                state.server = Some(failure.owner);
                if let Some((settings, bind, generation)) = rollback {
                    state.settings = settings;
                    state.bind = bind;
                    state.generation = generation;
                }
                state.last_error = error.clone();
                new_server
            };
            if let Some(new_server) = new_server
                && let Err(new_failure) = new_server.shutdown()
            {
                return Err(format!(
                    "{error}; rollback listener shutdown failed: {}",
                    new_failure.message
                ));
            }
            return Err(error);
        }
        Ok(())
    }
    fn status(&self, record: &McpServerSettingsRecord) -> Result<McpServerStatus, String> {
        let state = self
            .state
            .lock()
            .map_err(|_| "MCP runtime state poisoned".to_owned())?;
        Ok(McpServerStatus {
            running: record.enabled()
                && state
                    .server
                    .as_ref()
                    .is_some_and(McpServerOwner::is_running),
            endpoint: Self::endpoint(record.port()),
            last_error: state.last_error.clone(),
        })
    }
}
#[path = "product_mcp_server_dispatch.rs"]
mod dispatch;
use dispatch::{is_loopback_host, is_loopback_remote, mcp_origin_allowed};
#[cfg(test)]
#[path = "product_mcp_server_tests.rs"]
mod tests;
