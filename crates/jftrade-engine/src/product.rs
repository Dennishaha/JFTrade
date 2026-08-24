use std::env;
use std::fs::File;
use std::io::{BufReader, Read};
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

use crate::product_data_management;
use crate::product_runtime::ProductRuntimeState;
use crate::real_trade_control::{
    REAL_TRADE_CONTROL_PATH_ENV, RealTradeControlReader, derive_real_trade_control_path,
};
use crate::runtime_dependencies;
use jftrade_api::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, ApiState, Clock, PortFuture,
    RouteCatalog, RouteCatalogError, RouteSpec, SseEvent, SystemClock, TransportMetrics,
    build_router,
};
use jftrade_calendar::CalendarManager;
use jftrade_datamanagement::{
    BackupRequest, CleanupExecuteRequest, CleanupPreviewError, CleanupPreviewRequest,
    CleanupPreviewService, CompactRequest, MaintenanceOperationError, MaintenanceService,
    OverviewError, OverviewRequest, OverviewService, RebuildRequest,
};
use jftrade_research::ScreenCatalogError;
use jftrade_settings::{
    AppearanceService, AssistantRuntimeService, AssistantRuntimeSettings,
    BacktestMarketDataProviderSettingsService, BrokerIntegration, BrokerSettingsError,
    BrokerSettingsService, ExchangeCalendarSettings, ExchangeCalendarSettingsService,
    ExecutionService, ExecutionSettings, FutuOpenDInstallSettingsService, ManagedBrokerAccount,
    MarketDataProviderSettingsError, MarketDataProviderSettingsService, McpServerSettingsError,
    McpServerSettingsService, McpServerSettingsUpdate, OnboardingSettingsService,
    OnboardingWriteRequest, PineWorkerSettings, PineWorkerSettingsService, SecuritySettingsError,
    SecuritySettingsService, SecuritySettingsUpdate, SystemNotificationService,
    SystemNotificationSettings, UiAppearanceSettings, should_forward_system_notification,
};
use jftrade_store_settings_file::SettingsFileStore;
#[cfg(test)]
use jftrade_store_sqlite::ResearchPresetStoreError;
use jftrade_strategy::PluginUninstallGuidance;
use jftrade_watchlist::{Memberships, WatchlistError, normalize_instrument_id};
use percent_encoding::percent_decode_str;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use thiserror::Error;
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
pub const PRODUCT_BIND_ENV: &str = "JFTRADE_RUST_API_BIND";
pub const PRODUCT_SETTINGS_PATH_ENV: &str = "JFTRADE_SETTINGS_PATH";
pub const PRODUCT_DESKTOP_TOKEN_ENV: &str = "JFTRADE_DESKTOP_TOKEN";
pub const PRODUCT_REHEARSAL_PROTOCOL_VERSION: &str = "jftrade-product-rehearsal.v1";
pub const PRODUCT_READ_ONLY_ROUTE_PROFILE: &str = "read-only-shadow.v1";
pub const PRODUCT_TEST_CUTOVER_ROUTE_PROFILE: &str = "cutover-test-only.v1";
const DEFAULT_PRODUCT_BIND: &str = "127.0.0.1:3000";
const DEFAULT_SETTINGS_PATH: &str = "var/jftrade-api/settings.json";
include!("product_research_preset_port.rs");
include!("product_execution_read_port.rs");
include!("product_market_data_provider_read_port.rs");
include!("product_market_data_catalog_read_port.rs");
include!("product_market_data_derivative_read_port.rs");
include!("product_market_data_options_read_port.rs");
include!("product_market_data_news_actions_read_port.rs");
include!("product_market_data_news_search_read_port.rs");
include!("product_market_data_quote_read_port.rs");
include!("product_market_data_prediction_read_port.rs");
include!("product_adk_read_port.rs");
include!("product_market_data_prediction_read_routes.rs");
include!("product_stage9_write_ports.rs");
include!("product_auth_session_port.rs");
#[path = "product_auth_session_write_port.rs"]
mod product_auth_session_write_port;
use product_auth_session_write_port::{
    AuthSessionWritePort, AuthSessionWriteRequest, AuthSessionWriteResponse,
    auth_session_write_routes, dispatch_auth_session_write,
};
include!("product_snapshot_errors.rs");
#[path = "product_alerts_write_port.rs"]
mod product_alerts_write_port;
use product_alerts_write_port::{
    AlertWritePort, AlertWriteRequest, AlertWriteResponse, dispatch_alert_write,
};
#[path = "product_plugins_write_port.rs"]
mod product_plugins_write_port;
use product_plugins_write_port::{
    PluginWritePort, PluginWriteRequest, PluginWriteResponse, dispatch_plugin_write,
};
#[path = "product_watchlist_remote_write_port.rs"]
mod product_watchlist_remote_write_port;
use product_watchlist_remote_write_port::{
    RemoteWatchlistWritePort, RemoteWatchlistWriteRequest, RemoteWatchlistWriteResponse,
    dispatch_remote_watchlist_write, remote_watchlist_write_routes,
};
#[path = "product_watchlist_write_port.rs"]
mod product_watchlist_write_port;
use product_watchlist_write_port::{
    WatchlistWritePort, WatchlistWriteRequest, WatchlistWriteResponse, dispatch_watchlist_write,
    watchlist_write_routes,
};
#[path = "product_backtests_write_port.rs"]
mod product_backtests_write_port;
use product_backtests_write_port::{
    BacktestsWritePort, BacktestsWriteRequest, BacktestsWriteResponse, backtests_write_routes,
    dispatch_backtests_write,
};
#[path = "product_research_preset_write_port.rs"]
mod product_research_preset_write_port;
#[cfg(test)]
use product_research_preset_write_port::ResearchPresetSqliteTestCutoverPort;
use product_research_preset_write_port::{
    ResearchPresetWritePort, ResearchPresetWriteRequest, ResearchPresetWriteResponse,
    dispatch_research_preset_write, research_preset_write_routes,
};
#[path = "product_strategy_definition_write_port.rs"]
mod product_strategy_definition_write_port;
use product_strategy_definition_write_port::{
    StrategyDefinitionWritePort, StrategyDefinitionWriteResponse,
    dispatch_strategy_definition_write, strategy_definition_write_routes,
};
#[path = "product_adk_chat_stream_port.rs"]
mod product_adk_chat_stream_port;
use product_adk_chat_stream_port::{ADK_CHAT_PATH, ADK_CHAT_STREAM_PATH, AdkChatStreamPort};
#[path = "product_adk_mutation_port.rs"]
mod product_adk_mutation_port;
use product_adk_mutation_port::{
    AdkMutationPort, AdkMutationRequest, AdkMutationResponse, adk_mutation_routes,
    dispatch_adk_mutation,
};
#[path = "product_strategy_runtime_write_port.rs"]
mod product_strategy_runtime_write_port;
use product_strategy_runtime_write_port::{
    StrategyRuntimeWritePort, StrategyRuntimeWriteResponse, dispatch_strategy_runtime_write,
    strategy_runtime_write_routes,
};
#[path = "strategy_pine.rs"]
mod strategy_pine;
use strategy_pine::{
    STRATEGY_PINE_ANALYZE_PATH, StrategyPineAnalyzeSnapshotPort, dispatch_strategy_pine_analyze,
};
const WS_LIVE_ROUTE: (&str, &str) = ("GET", "/api/v1/ws/live");
/// Test-cutover gate for the existing authenticated loopback WebSocket
/// transport. Go remains the owner of live backend and subscription state.
pub trait WsLiveSnapshotPort: Send + Sync + std::fmt::Debug {
    fn enabled(&self) -> bool;
}
/// Consumer-owned read port for local watchlist membership projections.  The
/// port is accepted only in test-cutover wiring until the Rust store adapter
/// owns the same SQLite lifecycle as the Go watchlist service.
pub trait WatchlistMembershipSnapshotPort: Send + Sync + std::fmt::Debug {
    fn memberships(
        &self,
        instrument_id: &str,
    ) -> Result<Memberships, WatchlistMembershipSnapshotError>;
}
/// Consumer-owned read-only projections for the watchlist catalog. The Go
/// service remains responsible for SQLite access, normalization, pagination,
/// and source lifecycle; Rust only exposes the captured wire projection in
/// explicit test-cutover wiring.
pub trait WatchlistReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, WatchlistReadSnapshotError>;
}
/// Consumer-owned broker portfolio projections. The Go broker runtime remains
/// the only provider/OpenD owner; this port is test-cutover-only.
pub trait PortfolioSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str, query: &str) -> Result<serde_json::Value, PortfolioSnapshotError>;
}
/// Consumer-owned provider research projections. The Go provider runtime
/// remains the only production owner; this port is test-cutover-only.
pub trait ResearchReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str, query: &str)
    -> Result<serde_json::Value, ResearchReadSnapshotError>;
}
/// Consumer-owned broker read projections. The Go broker runtime remains the
/// only production owner; this port is test-cutover-only.
pub trait BrokerReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str, query: &str) -> Result<serde_json::Value, BrokerReadSnapshotError>;
}
/// Consumer-owned read port for the current Go plugin catalog's uninstall
/// guidance. The port carries the complete wire projection so Rust does not
/// duplicate platform-specific path normalization or shell quoting.
pub trait PluginUninstallGuidanceSnapshotPort: Send + Sync + std::fmt::Debug {
    fn guidance(
        &self,
        plugin_id: &str,
    ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError>;
}
/// Consumer-owned read-only projection for the Go plugin catalog and its
/// persisted operation status. The port carries complete wire values so Rust
/// does not reproduce catalog normalization or activate the plugin runtime.
pub trait PluginSnapshotPort: Send + Sync + std::fmt::Debug {
    fn catalog(&self) -> Result<serde_json::Value, PluginSnapshotError>;
    fn operation(
        &self,
        operation_id: &str,
    ) -> Result<Option<serde_json::Value>, PluginSnapshotError>;
}
/// Consumer-owned read port for Go's customization alert projections. The
/// port carries the complete wire value so the Rust shadow does not connect to
/// OpenD or duplicate the Futu alert adapter before ownership is cut over.
pub trait AlertSnapshotPort: Send + Sync + std::fmt::Debug {
    fn snapshot(
        &self,
        kind: AlertKind,
        raw_query: &str,
    ) -> Result<serde_json::Value, AlertSnapshotError>;
}
/// Consumer-owned read-only projection for strategy definition routes.
///
/// The port carries the complete JSON projection because the Go owner still
/// owns definition normalization, immutable history, and preview warmup
/// derivation. Rust only dispatches the existing wire contract and never
/// opens or mutates the strategy SQLite database.
pub trait StrategyDefinitionSnapshotPort: Send + Sync + std::fmt::Debug {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError>;

    fn get(
        &self,
        definition_id: &str,
        preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError>;

    fn versions(
        &self,
        definition_id: &str,
    ) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError>;

    fn version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError>;
}
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct StrategyDefinitionPreview {
    pub interval: Option<String>,
    pub symbol: Option<String>,
    pub use_extended_hours: bool,
}
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AlertKind {
    Price,
    OptionEvents,
}
#[derive(Clone)]
pub struct ProductConfig {
    bind_address: SocketAddr,
    settings_path: PathBuf,
    real_trade_control_path: PathBuf,
    access: AccessPolicy,
    notification_port: Option<Arc<dyn ProductNotificationPort>>,
    calendar_manager: Option<Arc<CalendarManager>>,
    watchlist_membership_snapshot_port: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    watchlist_read_snapshot_port: Option<Arc<dyn WatchlistReadSnapshotPort>>,
    portfolio_snapshot_port: Option<Arc<dyn PortfolioSnapshotPort>>,
    research_read_snapshot_port: Option<Arc<dyn ResearchReadSnapshotPort>>,
    research_preset_read_snapshot_port: Option<Arc<dyn ResearchPresetReadSnapshotPort>>,
    execution_read_snapshot_port: Option<Arc<dyn ExecutionReadSnapshotPort>>,
    market_data_provider_read_snapshot_port: Option<Arc<dyn MarketDataProviderReadSnapshotPort>>,
    market_data_catalog_read_snapshot_port: Option<Arc<dyn MarketDataCatalogReadSnapshotPort>>,
    market_data_derivative_read_snapshot_port:
        Option<Arc<dyn MarketDataDerivativeReadSnapshotPort>>,
    market_data_options_read_snapshot_port: Option<Arc<dyn MarketDataOptionsReadSnapshotPort>>,
    market_data_news_actions_read_snapshot_port:
        Option<Arc<dyn MarketDataNewsActionsReadSnapshotPort>>,
    market_data_news_search_read_snapshot_port:
        Option<Arc<dyn MarketDataNewsSearchReadSnapshotPort>>,
    adk_read_snapshot_port: Option<Arc<dyn AdkReadSnapshotPort>>,
    market_data_quote_read_snapshot_port: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    market_data_prediction_read_snapshot_port:
        Option<Arc<dyn MarketDataPredictionReadSnapshotPort>>,
    broker_read_snapshot_port: Option<Arc<dyn BrokerReadSnapshotPort>>,
    system_read_snapshot_port: Option<Arc<dyn SystemReadSnapshotPort>>,
    remote_watchlist_snapshot_port: Option<Arc<dyn RemoteWatchlistSnapshotPort>>,
    remote_watchlist_write_port: Option<Arc<dyn RemoteWatchlistWritePort>>,
    watchlist_write_port: Option<Arc<dyn WatchlistWritePort>>,
    plugin_uninstall_guidance_snapshot_port: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    plugin_snapshot_port: Option<Arc<dyn PluginSnapshotPort>>,
    plugin_write_port: Option<Arc<dyn PluginWritePort>>,
    research_preset_write_port: Option<Arc<dyn ResearchPresetWritePort>>,
    strategy_definition_write_port: Option<Arc<dyn StrategyDefinitionWritePort>>,
    market_data_provider_actions_port: Option<Arc<dyn MarketDataProviderActionsPort>>,
    adk_chat_stream_port: Option<Arc<dyn AdkChatStreamPort>>,
    adk_mutation_port: Option<Arc<dyn AdkMutationPort>>,
    alert_snapshot_port: Option<Arc<dyn AlertSnapshotPort>>,
    alert_write_port: Option<Arc<dyn AlertWritePort>>,
    strategy_definition_snapshot_port: Option<Arc<dyn StrategyDefinitionSnapshotPort>>,
    strategy_pine_analyze_snapshot_port: Option<Arc<dyn StrategyPineAnalyzeSnapshotPort>>,
    ws_live_snapshot_port: Option<Arc<dyn WsLiveSnapshotPort>>,
    backtest_read_snapshot_port: Option<Arc<dyn BacktestReadSnapshotPort>>,
    backtest_sync_read_snapshot_port: Option<Arc<dyn BacktestSyncReadSnapshotPort>>,
    backtests_write_port: Option<Arc<dyn BacktestsWritePort>>,
    strategy_read_snapshot_port: Option<Arc<dyn StrategyReadSnapshotPort>>,
    strategy_runtime_write_port: Option<Arc<dyn StrategyRuntimeWritePort>>,
    auth_session_snapshot_port: Option<Arc<dyn AuthSessionSnapshotPort>>,
    auth_session_write_port: Option<Arc<dyn AuthSessionWritePort>>,
    stage9_write_ports: ProductStage9WritePorts,
    capabilities: ProductCapabilities,
}
impl std::fmt::Debug for ProductConfig {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductConfig")
            .field("bind_address", &self.bind_address)
            .field("settings_path", &self.settings_path)
            .field("real_trade_control_path", &self.real_trade_control_path)
            .field("access", &"<redacted>")
            .field("calendar_manager", &self.calendar_manager.is_some())
            .field("capabilities", &self.capabilities)
            .finish_non_exhaustive()
    }
}
const PRODUCT_INTERNAL_PROXY_PROTOCOL_ENV: &str = "JFTRADE_RUST_INTERNAL_PROXY_PROTOCOL";

impl ProductConfig {
    pub(crate) fn new(
        bind_address: SocketAddr,
        settings_path: impl Into<PathBuf>,
        access: AccessPolicy,
    ) -> Result<Self, ProductError> {
        if !bind_address.ip().is_loopback() {
            return Err(ProductError::NonLoopbackBind);
        }
        let settings_path = settings_path.into();
        if settings_path.as_os_str().is_empty() {
            return Err(ProductError::MissingSettingsPath);
        }
        let real_trade_control_path = env::var(REAL_TRADE_CONTROL_PATH_ENV)
            .ok()
            .filter(|value| !value.trim().is_empty())
            .map(PathBuf::from)
            .unwrap_or_else(|| derive_real_trade_control_path(&settings_path));
        Ok(Self {
            bind_address,
            settings_path,
            real_trade_control_path,
            access,
            notification_port: None,
            calendar_manager: None,
            watchlist_membership_snapshot_port: None,
            watchlist_read_snapshot_port: None,
            portfolio_snapshot_port: None,
            research_read_snapshot_port: None,
            research_preset_read_snapshot_port: None,
            execution_read_snapshot_port: None,
            market_data_provider_read_snapshot_port: None,
            market_data_catalog_read_snapshot_port: None,
            market_data_derivative_read_snapshot_port: None,
            market_data_options_read_snapshot_port: None,
            market_data_news_actions_read_snapshot_port: None,
            market_data_news_search_read_snapshot_port: None,
            adk_read_snapshot_port: None,
            market_data_quote_read_snapshot_port: None,
            market_data_prediction_read_snapshot_port: None,
            broker_read_snapshot_port: None,
            system_read_snapshot_port: None,
            remote_watchlist_snapshot_port: None,
            remote_watchlist_write_port: None,
            watchlist_write_port: None,
            plugin_uninstall_guidance_snapshot_port: None,
            plugin_snapshot_port: None,
            plugin_write_port: None,
            research_preset_write_port: None,
            strategy_definition_write_port: None,
            market_data_provider_actions_port: None,
            adk_chat_stream_port: None,
            adk_mutation_port: None,
            alert_snapshot_port: None,
            alert_write_port: None,
            strategy_definition_snapshot_port: None,
            strategy_pine_analyze_snapshot_port: None,
            ws_live_snapshot_port: None,
            backtest_read_snapshot_port: None,
            backtest_sync_read_snapshot_port: None,
            backtests_write_port: None,
            strategy_read_snapshot_port: None,
            strategy_runtime_write_port: None,
            auth_session_snapshot_port: None,
            auth_session_write_port: None,
            stage9_write_ports: ProductStage9WritePorts::default(),
            capabilities: ProductCapabilities::default(),
        })
    }

    #[cfg(test)]
    fn test_cutover(
        bind_address: SocketAddr,
        settings_path: impl Into<PathBuf>,
    ) -> Result<Self, ProductError> {
        let mut config = Self::new(
            bind_address,
            settings_path,
            AccessPolicy {
                enforce_access: false,
                desktop_mode: true,
                ..AccessPolicy::default()
            }
            .with_allowed_origins([
                "http://127.0.0.1:3003".to_owned(),
                "http://localhost:3003".to_owned(),
                "tauri://localhost".to_owned(),
                "http://tauri.localhost".to_owned(),
                "https://tauri.localhost".to_owned(),
            ]),
        )?;
        config.capabilities = ProductCapabilities::test_cutover();
        Ok(config)
    }

    pub fn desktop_shadow(
        bind_address: SocketAddr,
        settings_path: impl Into<PathBuf>,
        desktop_token: impl Into<String>,
    ) -> Result<Self, ProductError> {
        let desktop_token = desktop_token.into();
        if desktop_token.trim().len() < 32 {
            return Err(ProductError::WeakDesktopToken);
        }
        Self::new(
            bind_address,
            settings_path,
            AccessPolicy {
                desktop_token: Some(desktop_token),
                enforce_access: true,
                desktop_mode: true,
                ..AccessPolicy::default()
            },
        )
    }

    pub fn from_process_env() -> Result<Self, ProductError> {
        let bind_address = env::var(PRODUCT_BIND_ENV)
            .unwrap_or_else(|_| DEFAULT_PRODUCT_BIND.to_owned())
            .parse()
            .map_err(ProductError::InvalidBindAddress)?;
        let settings_path = env::var_os(PRODUCT_SETTINGS_PATH_ENV)
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from(DEFAULT_SETTINGS_PATH));
        let desktop_token = env::var(PRODUCT_DESKTOP_TOKEN_ENV)
            .ok()
            .filter(|value| value.trim().len() >= 32)
            .ok_or(ProductError::MissingDesktopToken)?;
        let access = AccessPolicy {
            enforce_access: true,
            desktop_mode: true,
            desktop_token: Some(desktop_token),
            internal_proxy_protocol: env::var(PRODUCT_INTERNAL_PROXY_PROTOCOL_ENV)
                .ok()
                .filter(|value| !value.trim().is_empty()),
            ..AccessPolicy::default()
        };
        Self::new(bind_address, settings_path, access)
    }
}

include!("product_config_ports.rs");

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductStartupRecord {
    pub event: &'static str,
    pub address: SocketAddr,
    pub owner: &'static str,
    pub owned_routes: usize,
    pub protocol_version: &'static str,
    pub route_profile: &'static str,
    pub route_profile_digest: String,
    pub capabilities: Vec<String>,
    pub resource_sha256: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProductNotificationRequest {
    pub title: String,
    pub body: String,
    pub sound_enabled: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductNotificationDelivery {
    pub delivered: bool,
    pub status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub message: String,
}

pub trait ProductNotificationPort: Send + Sync + std::fmt::Debug {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery;
}

pub struct ProductHandle {
    startup_record: ProductStartupRecord,
    shutdown_tx: Option<oneshot::Sender<()>>,
    task: Option<JoinHandle<Result<(), std::io::Error>>>,
    calendar_manager: Option<Arc<CalendarManager>>,
}

impl ProductHandle {
    pub const fn startup_record(&self) -> &ProductStartupRecord {
        &self.startup_record
    }

    pub async fn shutdown(mut self) -> Result<(), ProductError> {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        if let Some(task) = self.task.take() {
            task.await.map_err(ProductError::Join)??;
        }
        if let Some(manager) = self.calendar_manager.take() {
            manager.close().map_err(ProductError::Calendar)?;
        }
        Ok(())
    }
}

impl Drop for ProductHandle {
    fn drop(&mut self) {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        if let Some(manager) = self.calendar_manager.take() {
            let _ = manager.close();
        }
    }
}

pub async fn start_product(config: ProductConfig) -> Result<ProductHandle, ProductError> {
    let runtime =
        ProductRuntimeState::product_only(config.settings_path().to_string_lossy().into_owned());
    start_product_with_runtime_state(config, runtime).await
}

pub(crate) async fn start_product_with_runtime_state(
    config: ProductConfig,
    runtime: Arc<ProductRuntimeState>,
) -> Result<ProductHandle, ProductError> {
    let listener = TcpListener::bind(config.bind_address)
        .await
        .map_err(ProductError::Bind)?;
    let address = listener.local_addr().map_err(ProductError::LocalAddress)?;
    let route_ports = product_route_ports(&config);
    let routes = product_routes(&config.capabilities, route_ports)?;
    let route_count = routes.routes().len();
    let route_capabilities = routes
        .routes()
        .iter()
        .map(|route| format!("{} {}", route.method, route.path))
        .collect::<Vec<_>>();
    let route_profile_digest = route_profile_digest(&route_capabilities);
    let resource_sha256 = current_executable_sha256()?;
    let owner = if config.capabilities.is_empty() {
        "rust-read-only-shadow"
    } else {
        "rust-cutover"
    };
    let route_profile = if config.capabilities.is_empty() {
        PRODUCT_READ_ONLY_ROUTE_PROFILE
    } else {
        PRODUCT_TEST_CUTOVER_ROUTE_PROFILE
    };
    let data_management = product_data_management::overview_service(config.settings_path());
    let cleanup_preview = product_data_management::cleanup_preview_service(config.settings_path());
    let maintenance = product_data_management::maintenance_service(
        config.settings_path(),
        Arc::clone(&cleanup_preview),
    );
    let settings_store = Arc::new(
        if config.capabilities.requires_writable_settings() {
            SettingsFileStore::open(config.settings_path)
        } else {
            SettingsFileStore::open_read_only(config.settings_path)
        }
        .map_err(ProductError::Settings)?,
    );
    let metrics = Arc::new(TransportMetrics::default());
    let real_trade_control = RealTradeControlReader::new(config.real_trade_control_path.clone());
    if let Some(manager) = &config.calendar_manager {
        manager.start().map_err(ProductError::Calendar)?;
    }
    let port = Arc::new(ProductApi::new(
        address.port(),
        ProductSettingsServices {
            appearance: AppearanceService::new(settings_store.clone()),
            brokers: BrokerSettingsService::new(settings_store.clone()),
            onboarding: OnboardingSettingsService::new(settings_store.clone()),
            futu_install: FutuOpenDInstallSettingsService::new(settings_store.clone()),
            execution: ExecutionService::new(settings_store.clone()),
            assistant_runtime: AssistantRuntimeService::new(settings_store.clone()),
            system_notifications: SystemNotificationService::new(settings_store.clone()),
            pine_worker: PineWorkerSettingsService::new(settings_store.clone()),
            security: SecuritySettingsService::new(settings_store.clone()),
            market_data_provider: MarketDataProviderSettingsService::new(settings_store.clone()),
            backtest_market_data_provider: BacktestMarketDataProviderSettingsService::new(
                settings_store.clone(),
            ),
            mcp_server: McpServerSettingsService::new(settings_store.clone()),
            exchange_calendars: ExchangeCalendarSettingsService::new(settings_store),
            data_management,
            cleanup_preview,
            maintenance,
        },
        runtime,
        real_trade_control,
        ProductOptionalPorts {
            notification: config.notification_port.clone(),
            calendar_manager: config.calendar_manager.clone(),
            watchlist_membership_snapshot: config.watchlist_membership_snapshot_port.clone(),
            watchlist_read_snapshot: config.watchlist_read_snapshot_port.clone(),
            portfolio_snapshot: config.portfolio_snapshot_port.clone(),
            research_read_snapshot: config.research_read_snapshot_port.clone(),
            research_preset_read_snapshot: config.research_preset_read_snapshot_port.clone(),
            execution_read_snapshot: config.execution_read_snapshot_port.clone(),
            market_data_provider_read_snapshot: config
                .market_data_provider_read_snapshot_port
                .clone(),
            market_data_catalog_read_snapshot: config
                .market_data_catalog_read_snapshot_port
                .clone(),
            market_data_derivative_read_snapshot: config
                .market_data_derivative_read_snapshot_port
                .clone(),
            market_data_options_read_snapshot: config
                .market_data_options_read_snapshot_port
                .clone(),
            market_data_news_actions_read_snapshot: config
                .market_data_news_actions_read_snapshot_port
                .clone(),
            market_data_news_search_read_snapshot: config
                .market_data_news_search_read_snapshot_port
                .clone(),
            adk_read_snapshot: config.adk_read_snapshot_port.clone(),
            market_data_quote_read_snapshot: config.market_data_quote_read_snapshot_port.clone(),
            market_data_prediction_read_snapshot: config
                .market_data_prediction_read_snapshot_port
                .clone(),
            broker_read_snapshot: config.broker_read_snapshot_port.clone(),
            system_read_snapshot: config.system_read_snapshot_port.clone(),
            remote_watchlist_snapshot: config.remote_watchlist_snapshot_port.clone(),
            remote_watchlist_write: config.remote_watchlist_write_port.clone(),
            watchlist_write: config.watchlist_write_port.clone(),
            plugin_uninstall_guidance_snapshot: config
                .plugin_uninstall_guidance_snapshot_port
                .clone(),
            plugin_snapshot: config.plugin_snapshot_port.clone(),
            plugin_write: config.plugin_write_port.clone(),
            research_preset_write: config.research_preset_write_port.clone(),
            strategy_definition_write: config.strategy_definition_write_port.clone(),
            market_data_provider_actions: config.market_data_provider_actions_port.clone(),
            adk_chat_stream: config.adk_chat_stream_port.clone(),
            adk_mutation: config.adk_mutation_port.clone(),
            alert_snapshot: config.alert_snapshot_port.clone(),
            alert_write: config.alert_write_port.clone(),
            strategy_definition_snapshot: config.strategy_definition_snapshot_port.clone(),
            strategy_pine_analyze_snapshot: config.strategy_pine_analyze_snapshot_port.clone(),
            backtest_read_snapshot: config.backtest_read_snapshot_port.clone(),
            backtest_sync_read_snapshot: config.backtest_sync_read_snapshot_port.clone(),
            backtests_write: config.backtests_write_port.clone(),
            strategy_read_snapshot: config.strategy_read_snapshot_port.clone(),
            strategy_runtime_write: config.strategy_runtime_write_port.clone(),
            auth_session_snapshot: config.auth_session_snapshot_port.clone(),
            auth_session_write: config.auth_session_write_port.clone(),
            stage9_write_ports: config.stage9_write_ports.clone(),
        },
    ));
    let mut state = ApiState::new(routes, config.access, port);
    state.metrics = metrics;
    let router = build_router(state);
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    let task = tokio::spawn(async move {
        axum::serve(listener, router)
            .with_graceful_shutdown(async move {
                let _ = shutdown_rx.await;
            })
            .await
    });
    Ok(ProductHandle {
        startup_record: ProductStartupRecord {
            event: "ready",
            address,
            owner,
            owned_routes: route_count,
            protocol_version: PRODUCT_REHEARSAL_PROTOCOL_VERSION,
            route_profile,
            route_profile_digest,
            capabilities: route_capabilities,
            resource_sha256,
        },
        shutdown_tx: Some(shutdown_tx),
        task: Some(task),
        calendar_manager: config.calendar_manager,
    })
}

fn route_profile_digest(capabilities: &[String]) -> String {
    let mut digest = Sha256::new();
    for capability in capabilities {
        digest.update(capability.as_bytes());
        digest.update(b"\n");
    }
    encode_sha256(digest.finalize())
}

include!("product_resource_integrity.rs");
include!("product_route_assembly.rs");
include!("product_api.rs");
include!("product_api_pine_worker.rs");
include!("product_api_runtime_dependencies.rs");
include!("product_api_system_status.rs");
include!("product_api_system_read.rs");
include!("product_api_backtests.rs");
include!("product_api_backtests_write.rs");
include!("product_api_strategies.rs");
include!("product_api_watchlist.rs");
include!("product_api_portfolio.rs");
include!("product_api_research.rs");
include!("product_api_execution.rs");
include!("product_api_stage9_writes.rs");
include!("product_api_brokers_write.rs");
include!("product_api_stage9_helpers.rs");
include!("product_api_market_data_provider_read.rs");
include!("product_api_market_data_catalog_read.rs");
include!("product_api_market_data_derivative_read.rs");
include!("product_api_market_data_options_read.rs");
include!("product_market_data_news_actions_read_api.rs");
include!("product_market_data_news_search_read_api.rs");
include!("product_market_data_quote_read_api.rs");
include!("product_market_data_prediction_read_api.rs");
include!("product_adk_read_api.rs");
include!("product_api_adk_chat_stream.rs");
include!("product_api_adk_mutations.rs");
include!("product_api_strategy_runtime_write.rs");
include!("product_api_brokers.rs");
include!("product_api_watchlists.rs");
include!("product_api_watchlists_write.rs");
include!("product_api_plugins.rs");
include!("product_api_strategy_definitions.rs");
include!("product_api_strategy_pine.rs");
include!("product_api_auth_session.rs");
include!("product_api_auth_session_write.rs");
include!("product_api_alerts_write.rs");
include!("product_api_plugins_write.rs");
include!("product_api_strategy_research_writes.rs");
include!("product_wire.rs");
include!("product_wire_strategy_definitions.rs");
include!("product_wire_stage9.rs");
include!("product_wire_helpers.rs");
include!("product_provider_wire.rs");
include!("product_wire_watchlist.rs");
include!("product_wire_portfolio.rs");
include!("product_wire_research.rs");
include!("product_wire_brokers.rs");

#[derive(Debug, Error)]
pub enum ProductError {
    #[error("invalid Rust product API bind address")]
    InvalidBindAddress(#[source] std::net::AddrParseError),
    #[error("Rust product API may only bind to loopback until Web security ownership moves")]
    NonLoopbackBind,
    #[error("Rust product settings path is required")]
    MissingSettingsPath,
    #[error("{PRODUCT_DESKTOP_TOKEN_ENV} must contain at least 32 characters")]
    MissingDesktopToken,
    #[error("Rust desktop API token must contain at least 32 non-whitespace characters")]
    WeakDesktopToken,
    #[error("resolve the Rust product executable for resource integrity")]
    CurrentExecutable(#[source] std::io::Error),
    #[error("read Rust product executable {path} for resource integrity")]
    ReadExecutable {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("failed to bind Rust product API")]
    Bind(#[source] std::io::Error),
    #[error("failed to inspect Rust product API listener")]
    LocalAddress(#[source] std::io::Error),
    #[error("invalid Rust product route catalog")]
    Routes(#[from] RouteCatalogError),
    #[error("failed to open Rust product settings")]
    Settings(#[source] jftrade_settings::SettingsStoreError),
    #[error("Rust exchange-calendar manager failed")]
    Calendar(#[source] jftrade_calendar::CalendarManagerError),
    #[error("Rust product API task failed")]
    Join(#[source] tokio::task::JoinError),
    #[error("Rust product API transport failed")]
    Transport(#[from] std::io::Error),
}

#[cfg(test)]
#[path = "product_tests.rs"]
mod tests;
