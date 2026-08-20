use std::env;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

use jftrade_api::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, ApiState, Clock, PortFuture,
    RouteCatalog, RouteCatalogError, RouteSpec, SystemClock, TransportMetrics, build_router,
};
use jftrade_calendar::{CalendarSourcesSnapshot, CalendarStatusSnapshot};
use jftrade_datamanagement::{
    CleanupPreviewError, CleanupPreviewRequest, CleanupPreviewService, OverviewError,
    OverviewRequest, OverviewService,
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
use jftrade_strategy::PluginUninstallGuidance;
use jftrade_watchlist::{Memberships, WatchlistError, normalize_instrument_id};
use percent_encoding::percent_decode_str;
use serde::{Deserialize, Serialize};
use serde_json::json;
use thiserror::Error;
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

use crate::product_data_management;
use crate::product_runtime::{ProductRuntimeSnapshot, ProductRuntimeState};
use crate::real_trade_control::{
    REAL_TRADE_CONTROL_PATH_ENV, RealTradeControlReader, derive_real_trade_control_path,
};
use crate::runtime_dependencies;

pub const PRODUCT_BIND_ENV: &str = "JFTRADE_RUST_API_BIND";
pub const PRODUCT_SETTINGS_PATH_ENV: &str = "JFTRADE_SETTINGS_PATH";
pub const PRODUCT_DESKTOP_TOKEN_ENV: &str = "JFTRADE_DESKTOP_TOKEN";

const DEFAULT_PRODUCT_BIND: &str = "127.0.0.1:3000";
const DEFAULT_SETTINGS_PATH: &str = "var/jftrade-api/settings.json";

/// Consumer-owned read port for the live Go exchange-calendar manager
/// projection.  The Rust product only accepts this port in test cutover
/// wiring until a runtime adapter can supply the same dynamic registry,
/// status, cache, and health semantics.
pub trait CalendarSourceSnapshotPort: Send + Sync + std::fmt::Debug {
    fn snapshot(&self) -> Result<CalendarSourcesSnapshot, CalendarSourceSnapshotError>;
}

/// Consumer-owned read port for the complete live Go exchange-calendar
/// manager status projection.  It is intentionally separate from the source
/// list port because status also contains effective market selection and
/// cached snapshot summaries.  Until a runtime adapter supplies those
/// semantics, the product only accepts this port in test-cutover wiring.
pub trait CalendarStatusSnapshotPort: Send + Sync + std::fmt::Debug {
    fn snapshot(&self) -> Result<CalendarStatusSnapshot, CalendarStatusSnapshotError>;
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

/// Consumer-owned read port for the current Go plugin catalog's uninstall
/// guidance. The port carries the complete wire projection so Rust does not
/// duplicate platform-specific path normalization or shell quoting.
pub trait PluginUninstallGuidanceSnapshotPort: Send + Sync + std::fmt::Debug {
    fn guidance(
        &self,
        plugin_id: &str,
    ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum CalendarSourceSnapshotError {
    #[error("calendar source snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum CalendarStatusSnapshotError {
    #[error("calendar status snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum WatchlistMembershipSnapshotError {
    #[error("watchlist membership snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum PluginUninstallGuidanceSnapshotError {
    #[error("plugin uninstall guidance snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug)]
pub struct ProductConfig {
    bind_address: SocketAddr,
    settings_path: PathBuf,
    real_trade_control_path: PathBuf,
    access: AccessPolicy,
    notification_port: Option<Arc<dyn ProductNotificationPort>>,
    calendar_source_snapshot_port: Option<Arc<dyn CalendarSourceSnapshotPort>>,
    calendar_status_snapshot_port: Option<Arc<dyn CalendarStatusSnapshotPort>>,
    watchlist_membership_snapshot_port: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    plugin_uninstall_guidance_snapshot_port: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    write_owner: bool,
}

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
            calendar_source_snapshot_port: None,
            calendar_status_snapshot_port: None,
            watchlist_membership_snapshot_port: None,
            plugin_uninstall_guidance_snapshot_port: None,
            write_owner: false,
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
        config.write_owner = true;
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
            ..AccessPolicy::default()
        };
        Self::new(bind_address, settings_path, access)
    }

    pub fn settings_path(&self) -> &std::path::Path {
        &self.settings_path
    }

    pub fn real_trade_control_path(&self) -> &std::path::Path {
        &self.real_trade_control_path
    }

    pub fn with_notification_port(mut self, port: Arc<dyn ProductNotificationPort>) -> Self {
        self.notification_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_calendar_source_snapshot_port(
        mut self,
        port: Arc<dyn CalendarSourceSnapshotPort>,
    ) -> Self {
        self.calendar_source_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_calendar_status_snapshot_port(
        mut self,
        port: Arc<dyn CalendarStatusSnapshotPort>,
    ) -> Self {
        self.calendar_status_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_watchlist_membership_snapshot_port(
        mut self,
        port: Arc<dyn WatchlistMembershipSnapshotPort>,
    ) -> Self {
        self.watchlist_membership_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_plugin_uninstall_guidance_snapshot_port(
        mut self,
        port: Arc<dyn PluginUninstallGuidanceSnapshotPort>,
    ) -> Self {
        self.plugin_uninstall_guidance_snapshot_port = Some(port);
        self
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductStartupRecord {
    pub event: &'static str,
    pub address: SocketAddr,
    pub owner: &'static str,
    pub owned_routes: usize,
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
        Ok(())
    }
}

impl Drop for ProductHandle {
    fn drop(&mut self) {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
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
    let routes = product_routes(
        config.write_owner,
        config.calendar_source_snapshot_port.is_some(),
        config.calendar_status_snapshot_port.is_some(),
        config.watchlist_membership_snapshot_port.is_some(),
        config.plugin_uninstall_guidance_snapshot_port.is_some(),
    )?;
    let route_count = routes.routes().len();
    let owner = if config.write_owner {
        "rust-cutover"
    } else {
        "rust-read-only-shadow"
    };
    let data_management = product_data_management::overview_service(config.settings_path());
    let cleanup_preview = product_data_management::cleanup_preview_service(config.settings_path());
    let settings_store = Arc::new(
        if config.write_owner {
            SettingsFileStore::open(config.settings_path)
        } else {
            SettingsFileStore::open_read_only(config.settings_path)
        }
        .map_err(ProductError::Settings)?,
    );
    let metrics = Arc::new(TransportMetrics::default());
    let real_trade_control = RealTradeControlReader::new(config.real_trade_control_path.clone());
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
        },
        Arc::clone(&metrics),
        runtime,
        real_trade_control,
        ProductOptionalPorts {
            notification: config.notification_port.clone(),
            calendar_source_snapshot: config.calendar_source_snapshot_port.clone(),
            calendar_status_snapshot: config.calendar_status_snapshot_port.clone(),
            watchlist_membership_snapshot: config.watchlist_membership_snapshot_port.clone(),
            plugin_uninstall_guidance_snapshot: config
                .plugin_uninstall_guidance_snapshot_port
                .clone(),
        },
        config.write_owner,
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
        },
        shutdown_tx: Some(shutdown_tx),
        task: Some(task),
    })
}

fn product_routes(
    write_owner: bool,
    calendar_source_snapshot_port: bool,
    calendar_status_snapshot_port: bool,
    watchlist_membership_snapshot_port: bool,
    plugin_uninstall_guidance_snapshot_port: bool,
) -> Result<RouteCatalog, RouteCatalogError> {
    let mut routes = vec![
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/status".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/runtime-dependencies".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/futu-opend/install-guide".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/storage/overview".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-approvals".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-hard-stops".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-hard-stop-events".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-kill-switch".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-kill-switch-events".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-risk-limits".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/real-trade-risk-events".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/ui".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/brokers".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/onboarding".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/execution".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/adk".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/adk/agent-templates".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/adk/mcp".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/system-notifications".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/pine-worker".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/security".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/market-data-provider".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/backtest-market-data-provider".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/exchange-calendars".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/data-management/databases".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/research/screens/catalog".into(),
        },
    ];
    if write_owner && calendar_source_snapshot_port {
        routes.push(RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/exchange-calendars/sources".into(),
        });
    }
    if write_owner && calendar_status_snapshot_port {
        routes.push(RouteSpec {
            method: "GET".into(),
            path: "/api/v1/system/exchange-calendars/status".into(),
        });
    }
    if write_owner && watchlist_membership_snapshot_port {
        routes.push(RouteSpec {
            method: "GET".into(),
            path: "/api/v1/watchlist/instruments/{market}/{symbol}/memberships".into(),
        });
    }
    if write_owner && plugin_uninstall_guidance_snapshot_port {
        routes.push(RouteSpec {
            method: "GET".into(),
            path: "/api/v1/plugins/{pluginId}/uninstall-guidance".into(),
        });
    }
    if write_owner {
        routes.extend([
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/ui".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/onboarding".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/exchange-calendars".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/market-data-provider".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/backtest-market-data-provider".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/execution".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/adk".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/adk/mcp".into(),
            },
            RouteSpec {
                method: "POST".into(),
                path: "/api/v1/settings/adk/mcp/token/reset".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/system-notifications".into(),
            },
            RouteSpec {
                method: "POST".into(),
                path: "/api/v1/settings/system-notifications/test".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/pine-worker".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/security".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/brokers/{brokerId}/integration".into(),
            },
            RouteSpec {
                method: "POST".into(),
                path: "/api/v1/settings/broker-accounts".into(),
            },
            RouteSpec {
                method: "PUT".into(),
                path: "/api/v1/settings/broker-accounts/{accountRecordId}".into(),
            },
            RouteSpec {
                method: "DELETE".into(),
                path: "/api/v1/settings/broker-accounts/{accountRecordId}".into(),
            },
            RouteSpec {
                method: "POST".into(),
                path: "/api/v1/settings/data-management/cleanup/preview".into(),
            },
        ]);
    }
    RouteCatalog::new(routes)
}

struct ProductApi {
    api_port: u16,
    settings: ProductSettingsServices,
    metrics: Arc<TransportMetrics>,
    started_at: String,
    started: Instant,
    runtime: Arc<ProductRuntimeState>,
    real_trade_control: RealTradeControlReader,
    notification_port: Option<Arc<dyn ProductNotificationPort>>,
    calendar_source_snapshot_port: Option<Arc<dyn CalendarSourceSnapshotPort>>,
    calendar_status_snapshot_port: Option<Arc<dyn CalendarStatusSnapshotPort>>,
    watchlist_membership_snapshot_port: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    plugin_uninstall_guidance_snapshot_port: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
    notification_sequence: AtomicU64,
    write_owner: bool,
}

struct ProductOptionalPorts {
    notification: Option<Arc<dyn ProductNotificationPort>>,
    calendar_source_snapshot: Option<Arc<dyn CalendarSourceSnapshotPort>>,
    calendar_status_snapshot: Option<Arc<dyn CalendarStatusSnapshotPort>>,
    watchlist_membership_snapshot: Option<Arc<dyn WatchlistMembershipSnapshotPort>>,
    plugin_uninstall_guidance_snapshot: Option<Arc<dyn PluginUninstallGuidanceSnapshotPort>>,
}

struct ProductSettingsServices {
    appearance: AppearanceService,
    brokers: BrokerSettingsService,
    onboarding: OnboardingSettingsService,
    futu_install: FutuOpenDInstallSettingsService,
    execution: ExecutionService,
    assistant_runtime: AssistantRuntimeService,
    system_notifications: SystemNotificationService,
    pine_worker: PineWorkerSettingsService,
    security: SecuritySettingsService,
    market_data_provider: MarketDataProviderSettingsService,
    backtest_market_data_provider: BacktestMarketDataProviderSettingsService,
    mcp_server: McpServerSettingsService,
    exchange_calendars: ExchangeCalendarSettingsService,
    data_management: OverviewService,
    cleanup_preview: CleanupPreviewService,
}

impl ProductApi {
    fn new(
        api_port: u16,
        settings: ProductSettingsServices,
        metrics: Arc<TransportMetrics>,
        runtime: Arc<ProductRuntimeState>,
        real_trade_control: RealTradeControlReader,
        optional_ports: ProductOptionalPorts,
        write_owner: bool,
    ) -> Self {
        Self {
            api_port,
            settings,
            metrics,
            started_at: SystemClock.now_rfc3339(),
            started: Instant::now(),
            runtime,
            real_trade_control,
            notification_port: optional_ports.notification,
            calendar_source_snapshot_port: optional_ports.calendar_source_snapshot,
            calendar_status_snapshot_port: optional_ports.calendar_status_snapshot,
            watchlist_membership_snapshot_port: optional_ports.watchlist_membership_snapshot,
            plugin_uninstall_guidance_snapshot_port: optional_ports
                .plugin_uninstall_guidance_snapshot,
            notification_sequence: AtomicU64::new(0),
            write_owner,
        }
    }

    fn system_status(&self) -> ApiOutput {
        let requests = self.metrics.snapshot();
        let uptime = duration_millis(self.started.elapsed());
        let runtime = self.runtime.snapshot();
        let real_trade = self.real_trade_control.snapshot();
        let helper_ready = runtime.helper_state
            == Some(jftrade_integration_marketdata_helper::ProcessState::Ready);
        let message = runtime_message(&runtime);
        let checked_at = SystemClock.now_rfc3339();
        ApiOutput::Json(json!({
            "name": "JFTrade",
            "apiPort": self.api_port,
            "defaultBroker": "futu",
            "defaultTradingEnvironment": "SIMULATE",
            "realTradingEnabled": real_trade.real_trading_enabled,
            "realTradingKillSwitch": {
                "active": real_trade.kill_switch_active,
                "runtimeActive": real_trade.runtime_kill_switch_active,
                "blockedOperations": real_trade.blocked_operations,
                "allowsCancel": real_trade.allows_cancel
            },
            "realTradingRisk": {
                "enabled": real_trade.risk_enabled,
                "maxOrderQuantity": real_trade.effective_max_order_quantity,
                "maxOrderNotional": real_trade.effective_max_order_notional,
                "runtimeConfiguredMaxOrderQuantity": real_trade.runtime_configured_max_order_quantity,
                "runtimeConfiguredMaxOrderNotional": real_trade.runtime_configured_max_order_notional,
                "runtimeRiskConfigured": real_trade.runtime_risk_configured
            },
            "realTradeAccess": {
                "approverAllowlistEnabled": false,
                "approverCount": 0,
                "adminAllowlistEnabled": false,
                "adminCount": 0
            },
            "build": {
                "version": env!("CARGO_PKG_VERSION"),
                "commit": option_env!("JFTRADE_BUILD_COMMIT").unwrap_or("rust-development"),
                "buildTime": option_env!("JFTRADE_BUILD_TIME").unwrap_or("development"),
                "goos": std::env::consts::OS,
                "goarch": std::env::consts::ARCH
            },
            "persistence": {
                "engine": "rust-settings-file",
                "databasePath": "",
                "status": "partial",
                "migrated": false,
                "pendingMigrations": ["remaining capability stores"],
                "tables": [],
                "checkedAt": self.started_at
            },
            "observability": {
                "api": { "startedAt": self.started_at, "uptimeMs": uptime },
                "live": { "connected": 0, "limit": 100, "atLimit": false, "activeInstruments": [] },
                "marketdata": {
                    "status": if helper_ready { "helper-ready" } else { "not-owned" },
                    "connected": helper_ready, "closed": !helper_ready,
                    "generation": 0, "activeCount": 0, "lastRefreshAt": null,
                    "quoteRetryAt": null, "quoteFailures": 0, "quoteLastError": null,
                    "streamRetryAt": null, "streamFailures": 0, "streamLastError": null
                },
                "exchangeCalendars": null,
                "broker": null,
                "strategyRuntime": null,
                "requests": {
                    "started": requests.started,
                    "completed": requests.completed,
                    "failures": requests.failures,
                    "inFlight": requests.in_flight
                }
            },
            "runtimeResources": {
                "checkedAt": checked_at,
                "count": runtime.resources.len(),
                "items": runtime.resources
            },
            "broker": null,
            "strategyRuntime": { "activeStrategies": 0, "activeInstances": [] },
            "message": message,
            "migrationOwner": if self.write_owner { "cutover" } else { "read-only-shadow" }
        }))
    }

    fn appearance(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .appearance
            .appearance()
            .map(|appearance| ApiOutput::Json(json!({ "appearance": appearance })))
            .map_err(settings_failure)
    }

    fn broker_settings(&self) -> Result<ApiOutput, ApiFailure> {
        let inputs = self
            .settings
            .brokers
            .inputs()
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(broker_settings_wire(inputs)))
    }

    fn save_broker_integration(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: BrokerIntegration = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid integration payload"))?;
        self.settings
            .brokers
            .save_integration(&input, &SystemClock.now_rfc3339())
            .map(|integration| ApiOutput::Json(json!(integration)))
            .map_err(broker_settings_failure)
    }

    fn create_managed_broker_account(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: ManagedBrokerAccount = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account payload"))?;
        self.settings
            .brokers
            .create_account(&input, &SystemClock.now_rfc3339())
            .map(|account| ApiOutput::Json(json!(account)))
            .map_err(broker_settings_failure)
    }

    fn update_managed_broker_account(
        &self,
        id: &str,
        body: &[u8],
    ) -> Result<ApiOutput, ApiFailure> {
        let input: ManagedBrokerAccount = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account payload"))?;
        self.settings
            .brokers
            .update_account(id, &input, &SystemClock.now_rfc3339())
            .map(|account| ApiOutput::Json(json!(account)))
            .map_err(broker_settings_failure)
    }

    fn delete_managed_broker_account(&self, id: &str) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .brokers
            .delete_account(id)
            .map(|()| ApiOutput::Json(json!({"deleted": true, "id": id})))
            .map_err(broker_settings_failure)
    }

    async fn onboarding(&self) -> Result<ApiOutput, ApiFailure> {
        let dependencies =
            runtime_dependencies::inspect(SystemClock.now_rfc3339(), self.runtime.node_runtime())
                .await;
        let readiness = self
            .settings
            .onboarding
            .readiness(dependencies.all_required_satisfied)
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(json!({
            "state": readiness.state,
            "shouldShowOobe": readiness.should_show_oobe,
            "reasons": readiness.reasons,
            "recommendedBrokerId": "futu",
            "brokers": [{
                "descriptor": jftrade_integration_futu::broker_descriptor(),
                "enabled": readiness.broker_enabled,
                "available": true,
                "configured": readiness.broker_configured,
            }]
        })))
    }

    async fn save_onboarding(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: OnboardingWriteRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid onboarding payload"))?;
        self.settings
            .onboarding
            .save(&request, &SystemClock.now_rfc3339())
            .map_err(settings_failure)?;
        self.onboarding().await
    }

    fn save_appearance(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: AppearanceWriteRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid appearance payload"))?;
        self.settings
            .appearance
            .save_appearance(&payload.appearance)
            .map(|appearance| ApiOutput::Json(json!({ "appearance": appearance })))
            .map_err(settings_failure)
    }

    async fn runtime_dependencies(&self) -> ApiOutput {
        let dependencies =
            runtime_dependencies::inspect(SystemClock.now_rfc3339(), self.runtime.node_runtime())
                .await;
        ApiOutput::Json(
            serde_json::to_value(dependencies)
                .expect("runtime dependency projection must be serializable"),
        )
    }

    fn futu_open_d_install_guide(&self) -> Result<ApiOutput, ApiFailure> {
        let settings = self
            .settings
            .futu_install
            .settings()
            .map_err(settings_read_failure)?;
        Ok(ApiOutput::Json(json!({
            "brokerId": "futu",
            "title": "Futu OpenD",
            "description": "Configure Futu OpenD. Current market data reaches OpenD through the bbgo exchange adapter and the native API port; WebSocket settings remain available for compatibility and future push-stream support.",
            "options": [],
            "nextSteps": [
                format!("安装或升级至 Futu OpenD {} 或更高版本。", jftrade_integration_futu::MINIMUM_OPEND_VERSION),
                "确认 OpenD 已登录，并先保证 API Port 可从本机访问。",
                "保存 Host 和 API Port；WebSocket Port / Key 目前主要用于兼容配置与诊断。",
                "保存后刷新 OpenD 健康状态，确认 API 侧连接正常。"
            ],
            "settings": {
                "host": settings.host,
                "apiPort": settings.api_port,
                "websocketPort": settings.websocket_port,
                "maxWebSocketConnections": settings.max_websocket_connections,
                "useEncryption": settings.use_encryption,
                "websocketKeyRequired": settings.websocket_key_required,
                "marketDataTransport": "bbgo-opend-tcp-api",
                "minimumVersion": jftrade_integration_futu::MINIMUM_OPEND_VERSION,
            }
        })))
    }

    fn storage_overview(&self) -> ApiOutput {
        ApiOutput::Json(json!({
            "pendingOutbox": [],
            "recentJobs": [],
            "recentAuditLogs": [],
            "recentExecutionCommands": [],
        }))
    }

    fn database_overview(&self, query: &str) -> Result<ApiOutput, ApiFailure> {
        let request = parse_database_overview_query(query);
        self.settings
            .data_management
            .overview(request, SystemClock.now_rfc3339())
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(database_overview_failure)
    }

    fn research_screen_catalog(&self, query: &str) -> Result<ApiOutput, ApiFailure> {
        let (broker_id, market) = parse_research_screen_catalog_query(query);
        jftrade_research::screen_catalog(&broker_id, &market)
            .map(ApiOutput::Json)
            .map_err(research_screen_catalog_failure)
    }

    fn calendar_source_snapshot(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.calendar_source_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_SOURCES_UNAVAILABLE",
                "exchange calendar source snapshot is not configured",
            )
        })?;
        let snapshot = port.snapshot().map_err(|error| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_SOURCES_UNAVAILABLE",
                error.to_string(),
            )
        })?;
        Ok(ApiOutput::Json(json!({ "sources": snapshot.sources })))
    }

    fn calendar_status_snapshot(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.calendar_status_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_STATUS_UNAVAILABLE",
                "exchange calendar status snapshot is not configured",
            )
        })?;
        let snapshot = port.snapshot().map_err(|error| {
            ApiFailure::new(
                503,
                "EXCHANGE_CALENDAR_STATUS_UNAVAILABLE",
                error.to_string(),
            )
        })?;
        Ok(ApiOutput::Json(json!(snapshot)))
    }

    fn watchlist_memberships(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let instrument_id = watchlist_membership_instrument_id(path)?;
        let port = self
            .watchlist_membership_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "WATCHLIST_UNAVAILABLE",
                    "watchlist membership snapshot is not configured",
                )
            })?;
        let memberships = port
            .memberships(&instrument_id)
            .map_err(|error| ApiFailure::new(503, "WATCHLIST_UNAVAILABLE", error.to_string()))?;
        Ok(ApiOutput::Json(json!(memberships)))
    }

    fn plugin_uninstall_guidance(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let plugin_id = plugin_uninstall_guidance_plugin_id(path)?;
        let port = self
            .plugin_uninstall_guidance_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE",
                    "plugin uninstall guidance snapshot is not configured",
                )
            })?;
        let guidance = port
            .guidance(&plugin_id)
            .map_err(|error| {
                ApiFailure::new(
                    503,
                    "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE",
                    error.to_string(),
                )
            })?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "plugin not found"))?;
        Ok(ApiOutput::Json(json!(guidance)))
    }

    fn cleanup_preview(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let request: CleanupPreviewRequest = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid cleanup preview payload"))?;
        self.settings
            .cleanup_preview
            .preview(request)
            .map(|response| ApiOutput::Json(json!(response)))
            .map_err(cleanup_preview_failure)
    }

    fn real_trade_approvals(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().approvals()))
    }

    fn real_trade_hard_stops(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().hard_stops()))
    }

    fn real_trade_hard_stop_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().hard_stop_events()))
    }

    fn real_trade_kill_switch(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().kill_switch()))
    }

    fn real_trade_kill_switch_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(
            self.real_trade_control.snapshot().kill_switch_events()
        ))
    }

    fn real_trade_risk_limits(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().risk_limits()))
    }

    fn real_trade_risk_events(&self) -> ApiOutput {
        ApiOutput::Json(json!(self.real_trade_control.snapshot().risk_events()))
    }

    fn execution_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .execution
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn save_execution_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: ExecutionSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid execution payload"))?;
        self.settings
            .execution
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn assistant_runtime_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .assistant_runtime
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn mcp_server_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .mcp_server
            .stopped_snapshot()
            .map(|snapshot| ApiOutput::Json(json!(snapshot)))
            .map_err(mcp_server_read_failure)
    }

    fn save_mcp_server_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: McpServerSettingsUpdate = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid MCP server payload"))?;
        self.settings
            .mcp_server
            .save(&input)
            .map_err(mcp_server_save_failure)?;
        self.mcp_server_settings()
    }

    fn reset_mcp_server_token(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .mcp_server
            .reset_token()
            .map(|result| ApiOutput::Json(json!(result)))
            .map_err(mcp_server_token_reset_failure)
    }

    fn save_assistant_runtime_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: AssistantRuntimeSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid adk payload"))?;
        self.settings
            .assistant_runtime
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn system_notification_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .system_notifications
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn save_system_notification_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: SystemNotificationSettings = serde_json::from_slice(body).map_err(|_| {
            ApiFailure::new(400, "BAD_REQUEST", "invalid system notification payload")
        })?;
        self.settings
            .system_notifications
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }

    fn test_system_notification(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.notification_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                500,
                "SYSTEM_NOTIFICATION_TEST_FAILED",
                "desktop system notifications are not available",
            )
        })?;
        let settings = self
            .settings
            .system_notifications
            .settings()
            .map_err(settings_read_failure)?;
        let sequence = self.notification_sequence.fetch_add(1, Ordering::SeqCst) + 1;
        let level = "warn";
        let category = "system.notification.test";
        let delivery = if should_forward_system_notification(&settings, level, category) {
            port.deliver(ProductNotificationRequest {
                title: "JFTrade 系统通知测试".to_owned(),
                body: "系统通知通道已连接。".to_owned(),
                sound_enabled: settings.sound_enabled,
            })
        } else {
            ProductNotificationDelivery {
                delivered: false,
                status: "filtered".to_owned(),
                message: "notification filtered by desktop settings".to_owned(),
            }
        };
        Ok(ApiOutput::Json(json!({
            "event": {
                "type": "system.notification",
                "id": format!("system-notification-{sequence}"),
                "at": SystemClock.now_rfc3339(),
                "level": level,
                "title": "JFTrade 系统通知测试",
                "message": "系统通知通道已连接。",
                "source": "desktop",
                "brokerId": "",
                "category": category
            },
            "delivery": delivery
        })))
    }

    fn pine_worker_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .pine_worker
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_read_failure)
    }

    fn security_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .security
            .settings()
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(security_settings_read_failure)
    }

    fn save_security_settings(
        &self,
        body: &[u8],
        desktop_trusted: bool,
    ) -> Result<ApiOutput, ApiFailure> {
        if !desktop_trusted {
            return Err(ApiFailure::new(
                403,
                "WEB_ACCESS_SETTINGS_DESKTOP_ONLY",
                "Web access settings can only be changed from the JFTrade desktop app",
            ));
        }
        let input: SecuritySettingsUpdate = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid security payload"))?;
        self.settings
            .security
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(security_settings_save_failure)
    }

    fn active_market_data_provider(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .market_data_provider
            .active_provider()
            .map(|active_provider| ApiOutput::Json(json!({ "activeProvider": active_provider })))
            .map_err(settings_read_failure)
    }

    fn backtest_market_data_provider(&self) -> Result<ApiOutput, ApiFailure> {
        let active_provider = self
            .settings
            .backtest_market_data_provider
            .active_provider()
            .map_err(settings_read_failure)?;
        let mut descriptors = vec![jftrade_integration_futu::provider_descriptor()];
        descriptors.extend(jftrade_integration_marketdata_helper::provider_descriptors());
        let available_providers = descriptors
            .into_iter()
            .map(provider_descriptor_wire)
            .collect::<Vec<_>>();
        Ok(ApiOutput::Json(json!({
            "activeProvider": active_provider,
            "availableProviders": available_providers,
        })))
    }

    fn save_active_market_data_provider(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: MarketDataProviderWriteRequest =
            serde_json::from_slice(body).map_err(|_| {
                ApiFailure::new(400, "BAD_REQUEST", "invalid market-data provider payload")
            })?;
        self.settings
            .market_data_provider
            .save(&payload.active_provider)
            .map(|active_provider| ApiOutput::Json(json!({"activeProvider": active_provider})))
            .map_err(|error| market_data_provider_failure(error, "MARKET_DATA_PROVIDER_INVALID"))
    }

    fn save_backtest_market_data_provider(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: MarketDataProviderWriteRequest =
            serde_json::from_slice(body).map_err(|_| {
                ApiFailure::new(400, "BAD_REQUEST", "invalid market-data provider payload")
            })?;
        self.settings
            .backtest_market_data_provider
            .save(&payload.active_provider)
            .map_err(|error| {
                market_data_provider_failure(error, "BACKTEST_MARKET_DATA_PROVIDER_INVALID")
            })?;
        self.backtest_market_data_provider()
    }

    fn exchange_calendar_settings(&self) -> Result<ApiOutput, ApiFailure> {
        self.settings
            .exchange_calendars
            .settings()
            .map(|settings| ApiOutput::Json(json!({ "exchangeCalendars": settings })))
            .map_err(settings_read_failure)
    }

    fn save_exchange_calendar_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let payload: ExchangeCalendarWriteRequest = serde_json::from_slice(body).map_err(|_| {
            ApiFailure::new(400, "BAD_REQUEST", "invalid exchange calendar payload")
        })?;
        self.settings
            .exchange_calendars
            .save(payload.exchange_calendars.into())
            .map(|settings| ApiOutput::Json(json!({"exchangeCalendars": settings})))
            .map_err(settings_failure)
    }

    fn save_pine_worker_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: PineWorkerSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid Pine worker payload"))?;
        self.settings
            .pine_worker
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }
}

fn provider_descriptor_wire(
    descriptor: jftrade_marketdata::ProviderDescriptor,
) -> serde_json::Value {
    let mut value = serde_json::to_value(descriptor)
        .expect("validated provider descriptor must be serializable");
    let Some(capabilities) = value
        .get_mut("capabilities")
        .and_then(serde_json::Value::as_object_mut)
    else {
        return value;
    };
    if capabilities
        .get("orderBookLevels")
        .and_then(serde_json::Value::as_array)
        .is_some_and(Vec::is_empty)
    {
        capabilities.insert("orderBookLevels".to_owned(), serde_json::Value::Null);
    }
    if capabilities
        .get("historicalLookbackDays")
        .and_then(serde_json::Value::as_object)
        .is_some_and(serde_json::Map::is_empty)
    {
        capabilities.remove("historicalLookbackDays");
    }
    value
}

fn broker_settings_wire(inputs: jftrade_settings::BrokerSettingsInputs) -> serde_json::Value {
    json!({
        "brokers": [{
            "descriptor": jftrade_integration_futu::broker_descriptor(),
            "integration": inputs.saved_integration,
            "defaults": inputs.effective_config,
        }],
        "accounts": inputs.accounts,
    })
}

const BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

const BUILTIN_AGENT_TOOLS: &[&str] = &[
    "interaction.request_user",
    "workflow.wait",
    "tools.search",
    "models.list",
    "system.status",
    "system.futu_opend",
    "plugins.catalog",
    "market.capabilities",
    "market.search",
    "market.snapshot",
    "market.snapshots",
    "market.candles",
    "market.intraday",
    "market.subscriptions",
    "watchlist.list",
    "research.instrument",
    "research.financials",
    "research.valuation",
    "research.news",
    "research.screen",
    "portfolio.accounts",
    "portfolio.overview",
    "portfolio.positions",
    "account.orders",
    "risk.state",
    "strategy.definitions",
    "strategy.validate_pine",
    "strategy.research_backtest",
    "backtest.runs",
    "backtest.result_view",
    "backtest.kline_sync_status",
];

const BUILTIN_AGENT_SKILLS: &[&str] = &[
    "jftrade-workflow-management",
    "jftrade-operations",
    "jftrade-market",
    "jftrade-derivatives",
    "jftrade-research",
    "jftrade-prediction",
    "jftrade-trading",
    "jftrade-portfolio",
    "jftrade-strategy-research",
    "jftrade-strategy-publish",
    "external-http",
];

fn agent_templates_wire() -> serde_json::Value {
    json!({
        "templates": [{
            "id": "jftrade-default",
            "name": "默认助手",
            "instruction": BUILTIN_AGENT_INSTRUCTION,
            "providerId": "",
            "tools": BUILTIN_AGENT_TOOLS,
            "toolAccessMode": "selected",
            "skills": BUILTIN_AGENT_SKILLS,
            "permissionMode": "approval",
            "memoryEnabled": true,
            "workMode": "chat",
            "loopMaxIterations": 5,
            "status": "ENABLED"
        }]
    })
}

fn runtime_message(runtime: &ProductRuntimeSnapshot) -> String {
    if let Some(error) = &runtime.last_error {
        return format!("Rust retained runtime failed: {error}");
    }
    let helper = match runtime.helper_state {
        Some(jftrade_integration_marketdata_helper::ProcessState::Ready) => "ready",
        Some(_) => "not-ready",
        None => "not-configured",
    };
    format!(
        "Rust read-only product shadow reports system status and settings projections; PineTS workers {}/{} ready; market-data helper {helper}",
        runtime.pine_ready, runtime.pine_total
    )
}

impl ApiPort for ProductApi {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            match (request.method.as_str(), request.path.as_str()) {
                ("GET", "/api/v1/system/status") => Ok(self.system_status()),
                ("GET", "/api/v1/system/runtime-dependencies") => {
                    Ok(self.runtime_dependencies().await)
                }
                ("GET", "/api/v1/system/futu-opend/install-guide") => {
                    self.futu_open_d_install_guide()
                }
                ("GET", "/api/v1/system/storage/overview") => Ok(self.storage_overview()),
                ("GET", "/api/v1/system/real-trade-approvals") => Ok(self.real_trade_approvals()),
                ("GET", "/api/v1/system/real-trade-hard-stops") => Ok(self.real_trade_hard_stops()),
                ("GET", "/api/v1/system/real-trade-hard-stop-events") => {
                    Ok(self.real_trade_hard_stop_events())
                }
                ("GET", "/api/v1/system/real-trade-kill-switch") => {
                    Ok(self.real_trade_kill_switch())
                }
                ("GET", "/api/v1/system/real-trade-kill-switch-events") => {
                    Ok(self.real_trade_kill_switch_events())
                }
                ("GET", "/api/v1/system/real-trade-risk-limits") => {
                    Ok(self.real_trade_risk_limits())
                }
                ("GET", "/api/v1/system/real-trade-risk-events") => {
                    Ok(self.real_trade_risk_events())
                }
                ("GET", "/api/v1/adk/agent-templates") => {
                    Ok(ApiOutput::Json(agent_templates_wire()))
                }
                ("GET", "/api/v1/settings/ui") => self.appearance(),
                ("GET", "/api/v1/settings/brokers") => self.broker_settings(),
                ("GET", "/api/v1/settings/onboarding") => self.onboarding().await,
                ("PUT", "/api/v1/settings/onboarding") => self.save_onboarding(&request.body).await,
                ("PUT", "/api/v1/settings/ui") => self.save_appearance(&request.body),
                ("GET", "/api/v1/settings/execution") => self.execution_settings(),
                ("PUT", "/api/v1/settings/execution") => {
                    self.save_execution_settings(&request.body)
                }
                ("GET", "/api/v1/settings/adk") => self.assistant_runtime_settings(),
                ("GET", "/api/v1/settings/adk/mcp") => self.mcp_server_settings(),
                ("PUT", "/api/v1/settings/adk/mcp") => self.save_mcp_server_settings(&request.body),
                ("POST", "/api/v1/settings/adk/mcp/token/reset") => self.reset_mcp_server_token(),
                ("PUT", "/api/v1/settings/adk") => {
                    self.save_assistant_runtime_settings(&request.body)
                }
                ("GET", "/api/v1/settings/system-notifications") => {
                    self.system_notification_settings()
                }
                ("GET", "/api/v1/settings/pine-worker") => self.pine_worker_settings(),
                ("GET", "/api/v1/settings/security") => self.security_settings(),
                ("PUT", "/api/v1/settings/security") => {
                    self.save_security_settings(&request.body, request.desktop_trusted)
                }
                ("GET", "/api/v1/settings/market-data-provider") => {
                    self.active_market_data_provider()
                }
                ("PUT", "/api/v1/settings/market-data-provider") => {
                    self.save_active_market_data_provider(&request.body)
                }
                ("GET", "/api/v1/settings/backtest-market-data-provider") => {
                    self.backtest_market_data_provider()
                }
                ("PUT", "/api/v1/settings/backtest-market-data-provider") => {
                    self.save_backtest_market_data_provider(&request.body)
                }
                ("GET", "/api/v1/settings/exchange-calendars") => self.exchange_calendar_settings(),
                ("GET", "/api/v1/settings/data-management/databases") => {
                    self.database_overview(&request.query)
                }
                ("GET", "/api/v1/research/screens/catalog") => {
                    self.research_screen_catalog(&request.query)
                }
                ("GET", "/api/v1/system/exchange-calendars/sources") => {
                    self.calendar_source_snapshot()
                }
                ("GET", "/api/v1/system/exchange-calendars/status") => {
                    self.calendar_status_snapshot()
                }
                ("GET", path) if is_watchlist_membership_path(path) => {
                    self.watchlist_memberships(path)
                }
                ("GET", path) if is_plugin_uninstall_guidance_path(path) => {
                    self.plugin_uninstall_guidance(path)
                }
                ("POST", "/api/v1/settings/data-management/cleanup/preview") => {
                    self.cleanup_preview(&request.body)
                }
                ("PUT", "/api/v1/settings/exchange-calendars") => {
                    self.save_exchange_calendar_settings(&request.body)
                }
                ("PUT", "/api/v1/settings/system-notifications") => {
                    self.save_system_notification_settings(&request.body)
                }
                ("POST", "/api/v1/settings/system-notifications/test") => {
                    self.test_system_notification()
                }
                ("PUT", "/api/v1/settings/pine-worker") => {
                    self.save_pine_worker_settings(&request.body)
                }
                ("PUT", path) if is_broker_integration_path(path) => {
                    self.save_broker_integration(&request.body)
                }
                ("POST", "/api/v1/settings/broker-accounts") => {
                    self.create_managed_broker_account(&request.body)
                }
                ("PUT", path) if is_managed_account_path(path) => {
                    let id = managed_account_id(path)?;
                    self.update_managed_broker_account(&id, &request.body)
                }
                ("DELETE", path) if is_managed_account_path(path) => {
                    let id = managed_account_id(path)?;
                    self.delete_managed_broker_account(&id)
                }
                _ => Err(ApiFailure::new(
                    501,
                    "RUST_OWNER_NOT_IMPLEMENTED",
                    format!("Rust product owner has not implemented {}", request.path),
                )),
            }
        })
    }
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AppearanceWriteRequest {
    #[serde(default)]
    appearance: UiAppearanceSettings,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExchangeCalendarWriteRequest {
    #[serde(default)]
    exchange_calendars: ExchangeCalendarWriteInput,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct MarketDataProviderWriteRequest {
    active_provider: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct ExchangeCalendarWriteInput {
    auto_refresh_enabled: bool,
    error_notifications_enabled: Option<bool>,
    refresh_interval_hours: i32,
    warmup_markets: Vec<String>,
    source_policies: Vec<jftrade_settings::ExchangeCalendarSourcePolicy>,
    manual_overrides: Vec<jftrade_settings::ExchangeCalendarManualOverride>,
}

impl From<ExchangeCalendarWriteInput> for ExchangeCalendarSettings {
    fn from(input: ExchangeCalendarWriteInput) -> Self {
        Self {
            auto_refresh_enabled: input.auto_refresh_enabled,
            error_notifications_enabled: input.error_notifications_enabled.unwrap_or(true),
            refresh_interval_hours: input.refresh_interval_hours,
            warmup_markets: input.warmup_markets,
            source_policies: input.source_policies,
            manual_overrides: input.manual_overrides,
        }
    }
}

fn settings_failure(error: jftrade_settings::SettingsStoreError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_SAVE_FAILED", error.to_string())
}

fn settings_read_failure(error: jftrade_settings::SettingsStoreError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn mcp_server_read_failure(error: McpServerSettingsError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn mcp_server_save_failure(error: McpServerSettingsError) -> ApiFailure {
    let message = error.to_string();
    match error {
        McpServerSettingsError::InvalidPort
        | McpServerSettingsError::InvalidAuthMode
        | McpServerSettingsError::TokenRequired => {
            ApiFailure::new(400, "MCP_SERVER_SETTINGS_REJECTED", message)
        }
        _ => ApiFailure::new(500, "MCP_SERVER_SETTINGS_FAILED", message),
    }
}

fn mcp_server_token_reset_failure(error: McpServerSettingsError) -> ApiFailure {
    ApiFailure::new(500, "MCP_SERVER_TOKEN_RESET_FAILED", error.to_string())
}

fn security_settings_read_failure(error: SecuritySettingsError) -> ApiFailure {
    ApiFailure::new(500, "SETTINGS_READ_FAILED", error.to_string())
}

fn security_settings_save_failure(error: SecuritySettingsError) -> ApiFailure {
    let message = error.to_string();
    match error {
        SecuritySettingsError::InvalidPort => {
            ApiFailure::new(400, "INVALID_WEB_ACCESS_PORT", message)
        }
        SecuritySettingsError::PasswordRequired
        | SecuritySettingsError::PasswordTooShort
        | SecuritySettingsError::PasswordTooLong => {
            ApiFailure::new(400, "INVALID_WEB_ACCESS_PASSWORD", message)
        }
        SecuritySettingsError::Runtime { .. } | SecuritySettingsError::RuntimeRollback { .. } => {
            ApiFailure::new(409, "WEB_ACCESS_LISTENER_UPDATE_FAILED", message)
        }
        SecuritySettingsError::PasswordHash(_) | SecuritySettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", message)
        }
    }
}

fn broker_settings_failure(error: BrokerSettingsError) -> ApiFailure {
    match error {
        BrokerSettingsError::MissingAccountId => {
            ApiFailure::new(400, "BAD_REQUEST", error.to_string())
        }
        BrokerSettingsError::AccountNotFound => {
            ApiFailure::new(404, "NOT_FOUND", error.to_string())
        }
        BrokerSettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", error.to_string())
        }
    }
}

fn market_data_provider_failure(
    error: MarketDataProviderSettingsError,
    invalid_code: &'static str,
) -> ApiFailure {
    let message = error.to_string();
    match error {
        MarketDataProviderSettingsError::Invalid => ApiFailure::new(400, invalid_code, message),
        MarketDataProviderSettingsError::Runtime(_) => {
            ApiFailure::new(409, "MARKET_DATA_PROVIDER_UPDATE_FAILED", message)
        }
        MarketDataProviderSettingsError::Store(_) => {
            ApiFailure::new(500, "SETTINGS_SAVE_FAILED", message)
        }
    }
}

fn database_overview_failure(error: OverviewError) -> ApiFailure {
    let message = error.to_string();
    match error {
        OverviewError::UnknownDatabase(_) => {
            ApiFailure::new(400, "DATABASE_STATUS_REJECTED", message)
        }
        OverviewError::RebuildMarker(_) => ApiFailure::new(500, "DATABASE_STATUS_FAILED", message),
    }
}

fn cleanup_preview_failure(error: CleanupPreviewError) -> ApiFailure {
    ApiFailure::new(400, "DATABASE_CLEANUP_PREVIEW_REJECTED", error.to_string())
}

fn research_screen_catalog_failure(error: ScreenCatalogError) -> ApiFailure {
    match error {
        ScreenCatalogError::UnsupportedFutuMarket
        | ScreenCatalogError::UnsupportedEmbeddedMarket(_) => {
            ApiFailure::new(400, "BAD_REQUEST", error.to_string())
        }
        ScreenCatalogError::BrokerUnavailable(_) => {
            ApiFailure::new(409, "BROKER_CAPABILITY_UNAVAILABLE", error.to_string())
        }
        ScreenCatalogError::FixtureInvalid(_) => {
            ApiFailure::new(500, "RESEARCH_SCREEN_CATALOG_FAILED", error.to_string())
        }
    }
}

fn parse_database_overview_query(query: &str) -> OverviewRequest {
    let mut request = OverviewRequest::default();
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(name);
        let value = decode_query_component(value);
        match name.as_str() {
            "summaryOnly" => request.summary_only = value.eq_ignore_ascii_case("true"),
            "databaseId" => request.database_id = value.trim().to_owned(),
            _ => {}
        }
    }
    request
}

fn parse_research_screen_catalog_query(query: &str) -> (String, String) {
    let mut broker_id = None;
    let mut market = None;
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (name, value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_component(name);
        let value = decode_query_component(value);
        match name.as_str() {
            "brokerId" if broker_id.is_none() => broker_id = Some(value),
            "market" if market.is_none() => market = Some(value),
            _ => {}
        }
    }
    (broker_id.unwrap_or_default(), market.unwrap_or_default())
}

fn is_watchlist_membership_path(path: &str) -> bool {
    watchlist_membership_path_parts(path).is_some()
}

fn watchlist_membership_instrument_id(path: &str) -> Result<String, ApiFailure> {
    let (market, symbol) = watchlist_membership_path_parts(path)
        .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "unknown watchlist endpoint"))?;
    let raw = format!("{market}.{symbol}");
    normalize_instrument_id(&raw)
        .map_err(|error| ApiFailure::new(400, "WATCHLIST_INVALID", watchlist_error_message(error)))
}

fn watchlist_membership_path_parts(path: &str) -> Option<(String, String)> {
    let suffix = path.strip_prefix("/api/v1/watchlist/instruments/")?;
    let suffix = suffix.strip_suffix("/memberships")?;
    let mut parts = suffix.split('/');
    let market = percent_decode_str(parts.next()?)
        .decode_utf8()
        .ok()?
        .into_owned();
    let symbol = percent_decode_str(parts.next()?)
        .decode_utf8()
        .ok()?
        .into_owned();
    if parts.next().is_some() || market.is_empty() || symbol.is_empty() {
        return None;
    }
    Some((market, symbol))
}

fn watchlist_error_message(error: WatchlistError) -> String {
    error.to_string()
}

fn is_plugin_uninstall_guidance_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/plugins/")
        .and_then(|suffix| suffix.strip_suffix("/uninstall-guidance"))
        .is_some_and(|plugin_id| !plugin_id.contains('/'))
}

fn plugin_uninstall_guidance_plugin_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/plugins/")
        .and_then(|suffix| suffix.strip_suffix("/uninstall-guidance"))
        .filter(|plugin_id| !plugin_id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"))?;
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"))?;
    let plugin_id = decoded.trim();
    if plugin_id.is_empty() || plugin_id.contains('/') {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "pluginId is invalid"));
    }
    Ok(plugin_id.to_owned())
}

fn decode_query_component(value: &str) -> String {
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8_lossy()
        .into_owned()
}

fn is_broker_integration_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/brokers/")
        .and_then(|value| value.strip_suffix("/integration"))
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn is_managed_account_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/broker-accounts/")
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn managed_account_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/settings/broker-accounts/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))?;
    percent_decode_str(encoded)
        .decode_utf8()
        .map(|id| id.into_owned())
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))
}

fn duration_millis(duration: Duration) -> u64 {
    u64::try_from(duration.as_millis()).unwrap_or(u64::MAX)
}

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
    #[error("failed to bind Rust product API")]
    Bind(#[source] std::io::Error),
    #[error("failed to inspect Rust product API listener")]
    LocalAddress(#[source] std::io::Error),
    #[error("invalid Rust product route catalog")]
    Routes(#[from] RouteCatalogError),
    #[error("failed to open Rust product settings")]
    Settings(#[source] jftrade_settings::SettingsStoreError),
    #[error("Rust product API task failed")]
    Join(#[source] tokio::task::JoinError),
    #[error("Rust product API transport failed")]
    Transport(#[from] std::io::Error),
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use jftrade_settings::SettingsStorePort;
    use rusqlite::Connection;
    use serde_json::Value;
    use tempfile::tempdir;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpStream;

    use super::*;

    #[test]
    fn static_provider_catalog_matches_current_go_wire_fixture() {
        let mut descriptors = vec![jftrade_integration_futu::provider_descriptor()];
        descriptors.extend(jftrade_integration_marketdata_helper::provider_descriptors());
        let actual = serde_json::Value::Array(
            descriptors
                .into_iter()
                .map(provider_descriptor_wire)
                .collect(),
        );
        let expected: serde_json::Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/provider-descriptors.json"
        ))
        .expect("provider descriptor fixture");
        assert_eq!(actual, expected);
    }

    #[test]
    fn stage9_assistant_agent_templates_match_current_go_owner() {
        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE")
        else {
            return;
        };
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go agent-template reference"),
        )
        .expect("decode Go agent-template reference");
        assert_eq!(expected["version"], "stage9.assistant-agent-templates.v1");
        assert_eq!(
            agent_templates_wire(),
            json!({"templates": expected["templates"].clone()})
        );
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct OnboardingSettingsWriteCorpus {
        version: String,
        cases: Vec<OnboardingSettingsWriteCase>,
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct OnboardingSettingsWriteCase {
        name: String,
        seed_document: Value,
        input: OnboardingWriteRequest,
    }

    #[test]
    fn stage9_onboarding_settings_writes_match_current_go_owner() {
        let corpus: OnboardingSettingsWriteCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/onboarding-settings-write-corpus.json"
        ))
        .expect("onboarding settings write corpus");
        assert_eq!(corpus.version, "stage9.onboarding-settings-write.v1");
        assert!(corpus.cases.len() >= 4);
        let directory = tempdir().expect("temporary directory");
        let mut results = Vec::with_capacity(corpus.cases.len());
        for (index, test_case) in corpus.cases.iter().enumerate() {
            let path = directory.path().join(format!("onboarding-{index}.json"));
            std::fs::write(
                &path,
                serde_json::to_vec(&test_case.seed_document)
                    .expect("encode onboarding seed document"),
            )
            .expect("seed onboarding settings write document");
            let store = Arc::new(SettingsFileStore::open(&path).expect("open onboarding store"));
            let service = OnboardingSettingsService::new(store);
            let saved = service
                .save(&test_case.input, "2026-08-20T04:00:00Z")
                .expect("save onboarding settings");
            let persisted: Value = serde_json::from_slice(
                &std::fs::read(&path).expect("read persisted onboarding settings"),
            )
            .expect("decode persisted onboarding settings");
            results.push(json!({
                "name": test_case.name,
                "saved": saved,
                "persisted": persisted,
            }));
        }
        let mut actual = json!({"version": corpus.version, "results": results});
        normalize_broker_timestamps(&mut actual);
        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE")
        else {
            return;
        };
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go onboarding write reference"),
        )
        .expect("decode Go onboarding write reference");
        assert_eq!(actual, expected);
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct ProviderSettingsWriteCorpus {
        version: String,
        seed_document: Value,
        active_inputs: Vec<String>,
        backtest_inputs: Vec<String>,
    }

    #[test]
    fn stage9_provider_settings_writes_match_current_go_owner() {
        let corpus: ProviderSettingsWriteCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/provider-settings-write-corpus.json"
        ))
        .expect("provider settings write corpus");
        assert_eq!(corpus.version, "stage9.provider-settings-write.v1");
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("settings.json");
        std::fs::write(
            &path,
            serde_json::to_vec(&corpus.seed_document).expect("encode provider seed document"),
        )
        .expect("seed provider settings write document");
        let store = Arc::new(SettingsFileStore::open(&path).expect("open provider store"));
        let active = MarketDataProviderSettingsService::new(store.clone());
        let backtest = BacktestMarketDataProviderSettingsService::new(store);
        let mut active_results = Vec::with_capacity(corpus.active_inputs.len());
        for input in &corpus.active_inputs {
            let result = active.save(input);
            let error = result.is_err();
            let provider = result.unwrap_or_else(|_| {
                active
                    .active_provider()
                    .expect("active provider after rejection")
            });
            active_results.push(json!({
                "input": input,
                "provider": provider,
                "error": error,
            }));
        }
        let mut backtest_results = Vec::with_capacity(corpus.backtest_inputs.len());
        for input in &corpus.backtest_inputs {
            let result = backtest.save(input);
            let error = result.is_err();
            let provider = result.unwrap_or_else(|_| {
                backtest
                    .active_provider()
                    .expect("backtest provider after rejection")
            });
            backtest_results.push(json!({
                "input": input,
                "provider": provider,
                "error": error,
            }));
        }
        let persisted: Value = serde_json::from_slice(
            &std::fs::read(&path).expect("read persisted provider settings"),
        )
        .expect("decode persisted provider settings");
        let actual = json!({
            "version": corpus.version,
            "activeResults": active_results,
            "backtestResults": backtest_results,
            "persisted": persisted,
        });
        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE")
        else {
            return;
        };
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go provider write reference"),
        )
        .expect("decode Go provider write reference");
        assert_eq!(actual, expected);
    }

    #[derive(Deserialize)]
    struct BrokerSettingsCorpus {
        version: String,
        cases: Vec<BrokerSettingsCase>,
    }

    #[derive(Deserialize)]
    struct BrokerSettingsCase {
        name: String,
        document: Value,
    }

    #[test]
    fn stage9_broker_settings_reads_match_current_go_owner() {
        let corpus: BrokerSettingsCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/broker-settings-corpus.json"
        ))
        .expect("broker settings corpus");
        assert_eq!(corpus.version, "stage9.broker-settings-read.v1");
        assert!(corpus.cases.len() >= 4);

        let directory = tempdir().expect("temporary directory");
        let mut results = Vec::with_capacity(corpus.cases.len());
        for (index, test_case) in corpus.cases.iter().enumerate() {
            let path = directory.path().join(format!("broker-{index}.json"));
            std::fs::write(
                &path,
                serde_json::to_vec(&test_case.document).expect("encode broker settings case"),
            )
            .expect("seed broker settings case");
            let service = BrokerSettingsService::new(Arc::new(
                SettingsFileStore::open_read_only(&path).expect("open broker settings case"),
            ));
            let projection = broker_settings_wire(service.inputs().expect("broker inputs"));
            results.push(json!({"name": test_case.name, "projection": projection}));
        }
        assert_eq!(
            results[1]["projection"]["brokers"][0]["integration"]["config"]["websocketKey"],
            "fixture-secret"
        );

        let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE")
        else {
            return;
        };
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go broker settings reference"),
        )
        .expect("decode Go broker settings reference");
        assert_eq!(
            json!({"version": corpus.version, "results": results}),
            expected
        );
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct BrokerSettingsWriteCorpus {
        version: String,
        seed_document: Value,
        integration: BrokerIntegration,
        create_first: ManagedBrokerAccount,
        upsert_first: ManagedBrokerAccount,
        create_second: ManagedBrokerAccount,
        update_second: ManagedBrokerAccount,
        delete_id: String,
        missing_id: String,
    }

    #[test]
    fn stage9_broker_settings_writes_match_current_go_owner() {
        let corpus: BrokerSettingsWriteCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/broker-settings-write-corpus.json"
        ))
        .expect("broker settings write corpus");
        assert_eq!(corpus.version, "stage9.broker-settings-write.v1");
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("settings.json");
        std::fs::write(
            &path,
            serde_json::to_vec(&corpus.seed_document).expect("encode seed document"),
        )
        .expect("seed broker settings write document");
        let store = Arc::new(SettingsFileStore::open(&path).expect("open broker write store"));
        let service = BrokerSettingsService::new(store);
        let now = "2026-08-20T04:00:00Z";
        let integration = service
            .save_integration(&corpus.integration, now)
            .expect("save integration");
        let created_first = service
            .create_account(&corpus.create_first, now)
            .expect("create first account");
        let upserted_first = service
            .create_account(&corpus.upsert_first, now)
            .expect("upsert first account");
        let created_second = service
            .create_account(&corpus.create_second, now)
            .expect("create second account");
        let updated_second = service
            .update_account(&created_second.id, &corpus.update_second, now)
            .expect("update second account");
        service
            .delete_account(&corpus.delete_id)
            .expect("delete first account");
        let update_missing = matches!(
            service.update_account(&corpus.missing_id, &corpus.update_second, now),
            Err(BrokerSettingsError::AccountNotFound)
        );
        let delete_missing = matches!(
            service.delete_account(&corpus.missing_id),
            Err(BrokerSettingsError::AccountNotFound)
        );
        let projection = broker_settings_wire(service.inputs().expect("final broker inputs"));
        let persisted: Value =
            serde_json::from_slice(&std::fs::read(&path).expect("read persisted broker settings"))
                .expect("decode persisted broker settings");
        let mut actual = json!({
            "version": corpus.version,
            "integration": integration,
            "createdFirst": created_first,
            "upsertedFirst": upserted_first,
            "createdSecond": created_second,
            "updatedSecond": updated_second,
            "updateMissing": update_missing,
            "deleteMissing": delete_missing,
            "projection": projection,
            "persisted": persisted,
        });
        normalize_broker_timestamps(&mut actual);
        assert_eq!(
            actual["persisted"]["interfaces"]["liveWebSocketConnectionLimit"],
            20
        );

        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE")
        else {
            return;
        };
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go broker write reference"),
        )
        .expect("decode Go broker write reference");
        assert_eq!(actual, expected);
    }

    fn normalize_broker_timestamps(value: &mut Value) {
        match value {
            Value::Object(fields) => {
                for (key, value) in fields {
                    if matches!(
                        key.as_str(),
                        "createdAt" | "updatedAt" | "completedAt" | "dismissedAt"
                    ) && value.as_str().is_some_and(|value| !value.is_empty())
                    {
                        *value = Value::String("<timestamp>".to_owned());
                    } else {
                        normalize_broker_timestamps(value);
                    }
                }
            }
            Value::Array(items) => items.iter_mut().for_each(normalize_broker_timestamps),
            Value::Null | Value::Bool(_) | Value::Number(_) | Value::String(_) => {}
        }
    }

    #[derive(Debug)]
    struct DeliveredNotification;

    impl ProductNotificationPort for DeliveredNotification {
        fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery {
            assert_eq!(request.title, "JFTrade 系统通知测试");
            assert!(request.sound_enabled);
            ProductNotificationDelivery {
                delivered: true,
                status: "delivered".to_owned(),
                message: "sent".to_owned(),
            }
        }
    }

    #[derive(Debug)]
    struct FixtureCalendarSourceSnapshotPort {
        snapshot: jftrade_calendar::CalendarSourcesSnapshot,
    }

    impl CalendarSourceSnapshotPort for FixtureCalendarSourceSnapshotPort {
        fn snapshot(
            &self,
        ) -> Result<jftrade_calendar::CalendarSourcesSnapshot, CalendarSourceSnapshotError>
        {
            Ok(self.snapshot.clone())
        }
    }

    #[derive(Debug)]
    struct FailingCalendarSourceSnapshotPort;

    impl CalendarSourceSnapshotPort for FailingCalendarSourceSnapshotPort {
        fn snapshot(
            &self,
        ) -> Result<jftrade_calendar::CalendarSourcesSnapshot, CalendarSourceSnapshotError>
        {
            Err(CalendarSourceSnapshotError::Unavailable(
                "Go exchange-calendar manager fixture unavailable".to_owned(),
            ))
        }
    }

    #[derive(Debug)]
    struct FixtureCalendarStatusSnapshotPort {
        snapshot: jftrade_calendar::CalendarStatusSnapshot,
    }

    impl CalendarStatusSnapshotPort for FixtureCalendarStatusSnapshotPort {
        fn snapshot(
            &self,
        ) -> Result<jftrade_calendar::CalendarStatusSnapshot, CalendarStatusSnapshotError> {
            Ok(self.snapshot.clone())
        }
    }

    #[derive(Debug)]
    struct FailingCalendarStatusSnapshotPort;

    impl CalendarStatusSnapshotPort for FailingCalendarStatusSnapshotPort {
        fn snapshot(
            &self,
        ) -> Result<jftrade_calendar::CalendarStatusSnapshot, CalendarStatusSnapshotError> {
            Err(CalendarStatusSnapshotError::Unavailable(
                "Go exchange-calendar manager status fixture unavailable".to_owned(),
            ))
        }
    }

    #[derive(Debug)]
    struct FixtureWatchlistMembershipSnapshotPort {
        memberships: std::collections::BTreeMap<String, jftrade_watchlist::Memberships>,
    }

    impl WatchlistMembershipSnapshotPort for FixtureWatchlistMembershipSnapshotPort {
        fn memberships(
            &self,
            instrument_id: &str,
        ) -> Result<jftrade_watchlist::Memberships, WatchlistMembershipSnapshotError> {
            Ok(self
                .memberships
                .get(instrument_id)
                .cloned()
                .unwrap_or_else(|| jftrade_watchlist::Memberships {
                    instrument_id: instrument_id.to_owned(),
                    revision: 0,
                    groups: Vec::new(),
                }))
        }
    }

    #[derive(Debug)]
    struct FailingWatchlistMembershipSnapshotPort;

    impl WatchlistMembershipSnapshotPort for FailingWatchlistMembershipSnapshotPort {
        fn memberships(
            &self,
            _instrument_id: &str,
        ) -> Result<jftrade_watchlist::Memberships, WatchlistMembershipSnapshotError> {
            Err(WatchlistMembershipSnapshotError::Unavailable(
                "Go watchlist membership fixture unavailable".to_owned(),
            ))
        }
    }

    #[derive(Debug)]
    struct FixturePluginUninstallGuidanceSnapshotPort {
        guidance: std::collections::BTreeMap<String, PluginUninstallGuidance>,
    }

    impl PluginUninstallGuidanceSnapshotPort for FixturePluginUninstallGuidanceSnapshotPort {
        fn guidance(
            &self,
            plugin_id: &str,
        ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError> {
            Ok(self.guidance.get(plugin_id).cloned())
        }
    }

    #[derive(Debug)]
    struct FailingPluginUninstallGuidanceSnapshotPort;

    impl PluginUninstallGuidanceSnapshotPort for FailingPluginUninstallGuidanceSnapshotPort {
        fn guidance(
            &self,
            _plugin_id: &str,
        ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError> {
            Err(PluginUninstallGuidanceSnapshotError::Unavailable(
                "Go plugin catalog fixture unavailable".to_owned(),
            ))
        }
    }

    #[tokio::test]
    async fn product_server_persists_ui_settings_and_reports_actual_port() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let real_trade_control_path = directory.path().join("real-trade-control.json");
        std::fs::write(
            &real_trade_control_path,
            r#"{
                "riskConfig": {
                    "id": "risk-1",
                    "tradingEnvironment": "REAL",
                    "realTradingEnabled": true,
                    "maxOrderQuantity": 12.5,
                    "maxOrderNotional": 2500,
                    "operatorId": "operator",
                    "reason": "market open",
                    "activatedAt": "2026-08-20T01:00:00Z",
                    "updatedAt": "2026-08-20T01:00:00Z"
                },
                "killSwitch": {
                    "id": "kill-switch-control-plane",
                    "tradingEnvironment": "REAL",
                    "operatorId": "operator",
                    "reason": "incident",
                    "activatedAt": "2026-08-20T01:01:00Z",
                    "updatedAt": "2026-08-20T01:01:00Z"
                },
                "hardStops": [{
                    "id": "hard-stop-1",
                    "brokerId": "futu",
                    "tradingEnvironment": "REAL",
                    "accountId": "ACC-1",
                    "market": "US",
                    "symbol": "AAPL",
                    "hardStopScope": "SYMBOL",
                    "operatorId": "operator",
                    "reason": "incident",
                    "activatedAt": "2026-08-20T01:02:00Z",
                    "updatedAt": "2026-08-20T01:02:00Z"
                }],
                "events": [
                    {"id":"event-risk","eventType":"updated","action":"RISK_CONFIG_UPDATE","brokerId":"*","createdAt":"2026-08-20T01:03:00Z"},
                    {"id":"event-kill","eventType":"activated","action":"KILL_SWITCH_ACTIVATE","brokerId":"*","createdAt":"2026-08-20T01:02:00Z"},
                    {"id":"event-hard-stop","eventType":"activated","action":"HARD_STOP_ACTIVATE","brokerId":"futu","createdAt":"2026-08-20T01:01:00Z"}
                ]
            }"#,
        )
        .expect("seed real-trade control state");
        let handle = start_product(
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_notification_port(Arc::new(DeliveredNotification)),
        )
        .await
        .expect("start product");
        let address = handle.startup_record().address;
        assert_eq!(handle.startup_record().owned_routes, 44);

        let status = request_json(address, "GET", "/api/v1/system/status", None).await;
        assert_eq!(status["ok"], true);
        assert_eq!(status["data"]["apiPort"], address.port());
        assert_eq!(status["data"]["name"], "JFTrade");
        assert_eq!(status["data"]["realTradingEnabled"], true);
        assert_eq!(status["data"]["realTradingKillSwitch"]["active"], true);
        assert_eq!(status["data"]["realTradingRisk"]["maxOrderQuantity"], 12.5);

        let agent_templates =
            request_json(address, "GET", "/api/v1/adk/agent-templates", None).await;
        assert_eq!(agent_templates["ok"], true);
        assert_eq!(agent_templates["data"], agent_templates_wire());

        let mcp_rejected = request_json(
            address,
            "PUT",
            "/api/v1/settings/adk/mcp",
            Some(r#"{"enabled":true,"port":6697,"authMode":"token"}"#),
        )
        .await;
        assert_eq!(
            mcp_rejected["error"]["code"],
            "MCP_SERVER_SETTINGS_REJECTED"
        );
        let mcp_reset = request_json(
            address,
            "POST",
            "/api/v1/settings/adk/mcp/token/reset",
            None,
        )
        .await;
        let mcp_token = mcp_reset["data"]["token"]
            .as_str()
            .expect("one-time MCP token");
        assert!(mcp_token.starts_with("jft_mcp_"));
        assert_eq!(mcp_reset["data"]["settings"]["tokenConfigured"], true);
        let mcp_saved = request_json(
            address,
            "PUT",
            "/api/v1/settings/adk/mcp",
            Some(r#"{"enabled":true,"port":6697,"authMode":"token"}"#),
        )
        .await;
        assert_eq!(mcp_saved["data"]["settings"]["enabled"], true);
        assert!(!mcp_saved.to_string().contains(mcp_token));
        let mcp_readback = request_json(address, "GET", "/api/v1/settings/adk/mcp", None).await;
        assert!(!mcp_readback.to_string().contains(mcp_token));
        assert!(!mcp_readback.to_string().contains("tokenHash"));
        let persisted_settings = std::fs::read_to_string(&settings_path).expect("settings file");
        assert!(!persisted_settings.contains(mcp_token));
        assert!(persisted_settings.contains("$argon2id$v=19$m=65536,t=3,p=1$"));

        let approvals =
            request_json(address, "GET", "/api/v1/system/real-trade-approvals", None).await;
        assert_eq!(approvals["data"]["realTradingEnabled"], true);
        assert_eq!(approvals["data"]["entries"], json!([]));

        let hard_stops =
            request_json(address, "GET", "/api/v1/system/real-trade-hard-stops", None).await;
        assert_eq!(hard_stops["data"]["entries"][0]["id"], "hard-stop-1");

        let hard_stop_events = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-hard-stop-events",
            None,
        )
        .await;
        assert_eq!(
            hard_stop_events["data"]["entries"][0]["id"],
            "event-hard-stop"
        );

        let kill_switch = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-kill-switch",
            None,
        )
        .await;
        assert_eq!(kill_switch["data"]["killSwitchSource"], "RUNTIME");
        assert_eq!(
            kill_switch["data"]["entry"]["id"],
            "kill-switch-control-plane"
        );

        let kill_switch_events = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-kill-switch-events",
            None,
        )
        .await;
        assert_eq!(kill_switch_events["data"]["entries"][0]["id"], "event-kill");

        let risk_limits = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-risk-limits",
            None,
        )
        .await;
        assert_eq!(risk_limits["data"]["entry"]["id"], "risk-1");
        assert_eq!(risk_limits["data"]["effectiveMaxOrderNotional"], 2500.0);

        let risk_events = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-risk-events",
            None,
        )
        .await;
        assert_eq!(risk_events["data"]["entries"][0]["id"], "event-risk");
        assert_eq!(risk_events["data"]["maxOrderQuantity"], 12.5);

        let dependencies =
            request_json(address, "GET", "/api/v1/system/runtime-dependencies", None).await;
        assert_eq!(dependencies["ok"], true);
        assert_eq!(dependencies["data"]["dependencies"][0]["id"], "node");
        assert_eq!(
            dependencies["data"]["dependencies"][0]["minimumVersion"],
            "22.0.0"
        );

        let install_guide = request_json(
            address,
            "GET",
            "/api/v1/system/futu-opend/install-guide",
            None,
        )
        .await;
        assert_eq!(install_guide["data"]["settings"]["host"], "127.0.0.1");
        assert_eq!(
            install_guide["data"]["settings"]["minimumVersion"],
            "10.9.6908"
        );
        assert!(
            install_guide["data"]["settings"]
                .get("websocketKey")
                .is_none()
        );

        let storage = request_json(address, "GET", "/api/v1/system/storage/overview", None).await;
        assert_eq!(storage["data"]["pendingOutbox"], json!([]));
        assert_eq!(storage["data"]["recentExecutionCommands"], json!([]));

        let databases = request_json(
            address,
            "GET",
            "/api/v1/settings/data-management/databases?summaryOnly=TRUE&databaseId=%20strategy%20",
            None,
        )
        .await;
        assert_eq!(databases["data"]["databases"][0]["id"], "strategy");
        assert_eq!(databases["data"]["databases"][0]["expectedVersion"], 2);
        assert_eq!(
            databases["data"]["databases"][0]["storage"]["totalBytes"],
            0
        );
        assert_eq!(databases["data"]["databases"][0]["cleanable"], Value::Null);

        let security = request_json(address, "GET", "/api/v1/settings/security", None).await;
        assert_eq!(security["data"]["webAccessEnabled"], false);
        assert_eq!(security["data"]["publicAccessEnabled"], false);
        assert_eq!(security["data"]["webPort"], 6688);
        assert_eq!(security["data"]["passwordConfigured"], false);
        assert!(security["data"].get("passwordHash").is_none());
        let invalid_security = request_json(
            address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"publicAccessEnabled":true,"webPort":6688,"newPassword":"short"}"#,
            ),
        )
        .await;
        assert_eq!(
            invalid_security["error"]["code"],
            "INVALID_WEB_ACCESS_PASSWORD"
        );
        let saved_security = request_json(
            address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"publicAccessEnabled":true,"webPort":6688,"newPassword":"a sufficiently long password"}"#,
            ),
        )
        .await;
        assert_eq!(saved_security["data"]["webAccessEnabled"], true);
        assert_eq!(saved_security["data"]["publicAccessEnabled"], true);
        assert_eq!(saved_security["data"]["passwordConfigured"], true);
        assert!(saved_security["data"].get("passwordHash").is_none());
        assert!(
            !saved_security
                .to_string()
                .contains("a sufficiently long password")
        );

        let onboarding = request_json(address, "GET", "/api/v1/settings/onboarding", None).await;
        assert_eq!(onboarding["data"]["state"]["completed"], false);
        assert_eq!(onboarding["data"]["recommendedBrokerId"], "futu");
        assert_eq!(onboarding["data"]["brokers"][0]["descriptor"]["id"], "futu");
        assert_eq!(onboarding["data"]["brokers"][0]["configured"], false);

        let saved_onboarding = request_json(
            address,
            "PUT",
            "/api/v1/settings/onboarding",
            Some(r#"{"completed":true,"lastBrokerId":" futu "}"#),
        )
        .await;
        assert_eq!(saved_onboarding["data"]["state"]["completed"], true);
        assert_eq!(saved_onboarding["data"]["state"]["lastBrokerId"], "futu");
        assert!(
            saved_onboarding["data"]["state"]["completedAt"]
                .as_str()
                .is_some_and(|value| !value.is_empty())
        );

        let brokers = request_json(address, "GET", "/api/v1/settings/brokers", None).await;
        assert_eq!(brokers["data"]["brokers"][0]["descriptor"]["id"], "futu");
        assert_eq!(
            brokers["data"]["brokers"][0]["integration"],
            serde_json::Value::Null
        );
        assert_eq!(brokers["data"]["brokers"][0]["defaults"]["apiPort"], 11110);
        assert_eq!(brokers["data"]["accounts"], json!([]));

        let integration = request_json(
            address,
            "PUT",
            "/api/v1/settings/brokers/ignored/integration",
            Some(
                r#"{"enabled":true,"config":{"host":" ","apiPort":0,"websocketPort":0,"maxWebSocketConnections":0,"useEncryption":true,"websocketKey":"secret"}}"#,
            ),
        )
        .await;
        assert_eq!(integration["data"]["brokerId"], "futu");
        assert_eq!(integration["data"]["config"]["host"], "127.0.0.1");
        assert_eq!(integration["data"]["config"]["useEncryption"], false);
        assert_eq!(integration["data"]["config"]["websocketKey"], "secret");
        assert!(
            integration["data"]["createdAt"]
                .as_str()
                .is_some_and(|value| !value.is_empty())
        );

        let invalid_account = request_json(
            address,
            "POST",
            "/api/v1/settings/broker-accounts",
            Some(r#"{"accountId":" "}"#),
        )
        .await;
        assert_eq!(invalid_account["ok"], false);
        assert_eq!(invalid_account["error"]["code"], "BAD_REQUEST");

        let created_account = request_json(
            address,
            "POST",
            "/api/v1/settings/broker-accounts",
            Some(
                r#"{"id":"client-id","brokerId":" FUTU ","accountId":" ACC-1 ","displayName":" ","tradingEnvironment":" real ","market":" us ","securityFirm":" ","enabled":true,"createdAt":"client","updatedAt":"client"}"#,
            ),
        )
        .await;
        assert_eq!(created_account["data"]["id"], "futu|REAL|ACC-1|US");
        assert_eq!(created_account["data"]["displayName"], "ACC-1");
        assert_eq!(created_account["data"]["securityFirm"], Value::Null);

        let updated_account = request_json(
            address,
            "PUT",
            "/api/v1/settings/broker-accounts/futu%7CREAL%7CACC-1%7CUS",
            Some(
                r#"{"accountId":"ACC-1","displayName":"Updated","tradingEnvironment":"real","market":"us","enabled":false}"#,
            ),
        )
        .await;
        assert_eq!(updated_account["data"]["id"], "futu|REAL|ACC-1|US");
        assert_eq!(updated_account["data"]["displayName"], "Updated");
        assert_eq!(updated_account["data"]["enabled"], false);

        let persisted_brokers =
            request_json(address, "GET", "/api/v1/settings/brokers", None).await;
        assert_eq!(
            persisted_brokers["data"]["brokers"][0]["integration"]["config"]["websocketKey"],
            "secret"
        );
        assert_eq!(
            persisted_brokers["data"]["accounts"][0]["displayName"],
            "Updated"
        );

        let deleted_account = request_json(
            address,
            "DELETE",
            "/api/v1/settings/broker-accounts/futu%7CREAL%7CACC-1%7CUS",
            None,
        )
        .await;
        assert_eq!(deleted_account["data"]["deleted"], true);
        assert_eq!(deleted_account["data"]["id"], "futu|REAL|ACC-1|US");

        let mcp = request_json(address, "GET", "/api/v1/settings/adk/mcp", None).await;
        assert_eq!(mcp["data"]["settings"]["port"], 6697);
        assert_eq!(mcp["data"]["settings"]["authMode"], "token");
        assert_eq!(mcp["data"]["settings"]["tokenConfigured"], true);
        assert_eq!(mcp["data"]["status"]["running"], false);
        assert!(mcp["data"]["settings"].get("tokenHash").is_none());

        let provider = request_json(
            address,
            "GET",
            "/api/v1/settings/market-data-provider",
            None,
        )
        .await;
        assert_eq!(provider["data"]["activeProvider"], "akshare");

        let backtest_provider = request_json(
            address,
            "GET",
            "/api/v1/settings/backtest-market-data-provider",
            None,
        )
        .await;
        assert_eq!(backtest_provider["data"]["activeProvider"], "akshare");
        assert_eq!(
            backtest_provider["data"]["availableProviders"][0]["selectionId"],
            "futu"
        );
        assert_eq!(
            backtest_provider["data"]["availableProviders"][1]["selectionId"],
            "yfinance"
        );
        assert_eq!(
            backtest_provider["data"]["availableProviders"][2]["selectionId"],
            "akshare"
        );

        let saved_provider = request_json(
            address,
            "PUT",
            "/api/v1/settings/market-data-provider",
            Some(r#"{"activeProvider":" YFINANCE "}"#),
        )
        .await;
        assert_eq!(saved_provider["data"]["activeProvider"], "yfinance");
        let invalid_provider = request_json(
            address,
            "PUT",
            "/api/v1/settings/market-data-provider",
            Some(r#"{"activeProvider":"invalid"}"#),
        )
        .await;
        assert_eq!(invalid_provider["ok"], false);
        assert_eq!(
            invalid_provider["error"]["code"],
            "MARKET_DATA_PROVIDER_INVALID"
        );

        let saved_backtest_provider = request_json(
            address,
            "PUT",
            "/api/v1/settings/backtest-market-data-provider",
            Some(r#"{"activeProvider":" futu "}"#),
        )
        .await;
        assert_eq!(saved_backtest_provider["data"]["activeProvider"], "futu");
        assert_eq!(
            saved_backtest_provider["data"]["availableProviders"][0]["selectionId"],
            "futu"
        );

        let calendars =
            request_json(address, "GET", "/api/v1/settings/exchange-calendars", None).await;
        assert_eq!(
            calendars["data"]["exchangeCalendars"]["refreshIntervalHours"],
            24
        );
        assert_eq!(
            calendars["data"]["exchangeCalendars"]["warmupMarkets"],
            json!(["US", "HK", "CN"])
        );

        let saved_calendars = request_json(
            address,
            "PUT",
            "/api/v1/settings/exchange-calendars",
            Some(
                r#"{"exchangeCalendars":{"autoRefreshEnabled":false,"errorNotificationsEnabled":false,"refreshIntervalHours":999,"warmupMarkets":[" us ","US"," hk "]}}"#,
            ),
        )
        .await;
        assert_eq!(
            saved_calendars["data"]["exchangeCalendars"]["refreshIntervalHours"],
            720
        );
        assert_eq!(
            saved_calendars["data"]["exchangeCalendars"]["warmupMarkets"],
            json!(["US", "HK"])
        );
        assert_eq!(
            saved_calendars["data"]["exchangeCalendars"]["errorNotificationsEnabled"],
            false
        );

        let execution = request_json(
            address,
            "PUT",
            "/api/v1/settings/execution",
            Some(
                r#"{"defaultTradingEnvironment":" real ","brokerOrderHistoryLookbackDays":999,"seenFillRetentionDays":0}"#,
            ),
        )
        .await;
        assert_eq!(execution["data"]["defaultTradingEnvironment"], "REAL");
        assert_eq!(execution["data"]["brokerOrderHistoryLookbackDays"], 365);
        assert_eq!(execution["data"]["seenFillRetentionDays"], 90);

        let adk = request_json(
            address,
            "PUT",
            "/api/v1/settings/adk",
            Some(r#"{"runTimeoutMs":1,"streamIdleTimeoutMs":9999999}"#),
        )
        .await;
        assert_eq!(adk["data"]["runTimeoutMs"], 60_000);
        assert_eq!(adk["data"]["streamIdleTimeoutMs"], 900_000);

        let notification_test = request_json(
            address,
            "POST",
            "/api/v1/settings/system-notifications/test",
            None,
        )
        .await;
        assert_eq!(
            notification_test["data"]["event"]["id"],
            "system-notification-1"
        );
        assert_eq!(notification_test["data"]["delivery"]["status"], "delivered");

        let notifications = request_json(
            address,
            "PUT",
            "/api/v1/settings/system-notifications",
            Some(
                r#"{"enabled":true,"mode":"custom","levels":[" WARN ","warn","Error"],"categories":["a"," a ","B"],"soundEnabled":false}"#,
            ),
        )
        .await;
        assert_eq!(notifications["data"]["levels"], json!(["warn", "error"]));
        assert_eq!(notifications["data"]["categories"], json!(["a", "B"]));

        let saved = request_json(
            address,
            "PUT",
            "/api/v1/settings/ui",
            Some(r#"{"appearance":{"upColor":" #ABCDEF ","downColor":"bad"}}"#),
        )
        .await;
        assert_eq!(saved["data"]["appearance"]["upColor"], "#abcdef");
        assert_eq!(saved["data"]["appearance"]["downColor"], "#ea3943");

        std::fs::write(&real_trade_control_path, "{").expect("corrupt real-trade control state");
        let fail_closed = request_json(
            address,
            "GET",
            "/api/v1/system/real-trade-kill-switch",
            None,
        )
        .await;
        assert_eq!(fail_closed["data"]["realTradingEnabled"], true);
        assert_eq!(fail_closed["data"]["killSwitchActive"], true);
        assert_eq!(fail_closed["data"]["entry"], serde_json::Value::Null);
        handle.shutdown().await.expect("shutdown product");

        let reloaded = SettingsFileStore::open(&settings_path).expect("reload settings");
        assert_eq!(
            reloaded
                .load_appearance()
                .expect("appearance")
                .expect("saved appearance")
                .up_color,
            "#abcdef"
        );
    }

    #[tokio::test]
    async fn cleanup_preview_route_returns_candidates_and_rejects_bad_payloads() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let database_path = directory.path().join("backtest-runs.db");
        let connection = Connection::open(&database_path).expect("open backtest database");
        connection
            .execute_batch(
                r#"PRAGMA journal_mode = DELETE;
                 CREATE TABLE backtest_runs (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL DEFAULT '',
                    request_json TEXT NOT NULL DEFAULT '',
                    result_json TEXT NOT NULL DEFAULT '',
                    created_at TEXT NOT NULL DEFAULT '',
                    updated_at TEXT NOT NULL DEFAULT ''
                 );
                 CREATE INDEX idx_backtest_runs_updated_at ON backtest_runs (updated_at DESC, id ASC);
                 CREATE INDEX idx_backtest_runs_status ON backtest_runs (status, updated_at DESC);
                 CREATE TABLE jftrade_schema_meta (
                    component_id TEXT PRIMARY KEY,
                    version INTEGER NOT NULL,
                    created_at TEXT NOT NULL
                 );
                 INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                 VALUES ('backtest-runs', 1, 'test');
                 INSERT INTO backtest_runs
                    (id, status, request_json, result_json, created_at, updated_at)
                 VALUES
                    ('latest', 'completed', '{}', '{}', '2999-01-01T00:00:00Z', '2999-01-01T00:00:00Z'),
                    ('expired', 'failed', '{"x":1}', '{"y":2}', '2000-01-01T00:00:00Z', '2000-01-01T00:00:00Z');"#,
            )
            .expect("seed backtest database");
        drop(connection);

        let handle = start_product(
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config"),
        )
        .await
        .expect("start product");
        let address = handle.startup_record().address;

        let preview = request_json(
            address,
            "POST",
            "/api/v1/settings/data-management/cleanup/preview",
            Some(
                r#"{"kind":"backtest-history","databaseId":"backtest-runs","olderThanDays":1,"keepLatest":1}"#,
            ),
        )
        .await;
        assert_eq!(preview["ok"], true);
        assert_eq!(preview["data"]["databaseId"], "backtest-runs");
        assert_eq!(preview["data"]["candidateCount"], 1);
        assert_eq!(preview["data"]["items"][0]["kind"], "回测结果");
        assert_eq!(
            preview["data"]["confirmationText"],
            "CLEANUP backtest-runs 1"
        );
        assert_eq!(preview["data"]["willCompact"], true);

        let malformed = request_json(
            address,
            "POST",
            "/api/v1/settings/data-management/cleanup/preview",
            Some(r#"{"kind":"#),
        )
        .await;
        assert_eq!(malformed["error"]["code"], "BAD_REQUEST");

        let rejected = request_json(
            address,
            "POST",
            "/api/v1/settings/data-management/cleanup/preview",
            Some(r#"{"kind":"unknown","databaseId":"backtest-runs"}"#),
        )
        .await;
        assert_eq!(
            rejected["error"]["code"],
            "DATABASE_CLEANUP_PREVIEW_REJECTED"
        );
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn research_screen_catalog_route_matches_static_catalog_variants() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let handle = start_product(
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config"),
        )
        .await
        .expect("start product");
        let address = handle.startup_record().address;

        let futu = request_json(
            address,
            "GET",
            "/api/v1/research/screens/catalog?brokerId=futu&market=US",
            None,
        )
        .await;
        assert_eq!(futu["ok"], true);
        assert_eq!(futu["data"]["version"], "futu-stock-screen-v1");
        assert_eq!(futu["data"]["provider"], "futu");
        assert_eq!(futu["data"]["market"], "US");
        assert_eq!(futu["data"]["factors"].as_array().map(Vec::len), Some(402));
        assert!(futu.to_string().find("providerId").is_none());

        let embedded = request_json(
            address,
            "GET",
            "/api/v1/research/screens/catalog?brokerId=yfinance",
            None,
        )
        .await;
        assert_eq!(embedded["ok"], true);
        assert_eq!(embedded["data"]["version"], "embedded-stock-screen-v1");
        assert_eq!(embedded["data"]["provider"], "yfinance");
        assert_eq!(
            embedded["data"]["factors"].as_array().map(Vec::len),
            Some(9)
        );

        let unsupported_market = request_json(
            address,
            "GET",
            "/api/v1/research/screens/catalog?brokerId=yfinance&market=HK",
            None,
        )
        .await;
        assert_eq!(unsupported_market["error"]["code"], "BAD_REQUEST");
        let unavailable = request_json(
            address,
            "GET",
            "/api/v1/research/screens/catalog?brokerId=unknown",
            None,
        )
        .await;
        assert_eq!(
            unavailable["error"]["code"],
            "BROKER_CAPABILITY_UNAVAILABLE"
        );
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn research_screen_catalog_route_matches_go_fixture_for_all_variants() {
        let fixture: Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/research-screen-catalogs.json"
        ))
        .expect("research screen catalog fixture");
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let handle = start_product(
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config"),
        )
        .await
        .expect("start product");
        let address = handle.startup_record().address;
        for (broker, market) in [
            ("futu", ""),
            ("futu", "HK"),
            ("futu", "US"),
            ("futu", "SH"),
            ("futu", "SZ"),
            ("yfinance", ""),
            ("yfinance", "US"),
            ("akshare", ""),
            ("akshare", "SH"),
            ("akshare", "SZ"),
            ("akshare", "CN"),
            ("akshare", "HK"),
            ("akshare", "US"),
        ] {
            let query =
                format!("/api/v1/research/screens/catalog?brokerId={broker}&market={market}");
            let actual = request_json(address, "GET", &query, None).await;
            assert_eq!(actual["ok"], true, "catalog query {query}");
            let key = format!("{broker}|{market}");
            assert_eq!(
                actual["data"], fixture["catalogs"][&key],
                "catalog query {query}"
            );
        }
        for (query, code, message) in [
            (
                "brokerId=futu&market=SG",
                "BAD_REQUEST",
                "unsupported stock-screen market",
            ),
            (
                "brokerId=yfinance&market=HK",
                "BAD_REQUEST",
                "unsupported stock-screen market for yfinance",
            ),
            (
                "brokerId=akshare&market=MO",
                "BAD_REQUEST",
                "unsupported stock-screen market for akshare",
            ),
            (
                "brokerId=unknown",
                "BROKER_CAPABILITY_UNAVAILABLE",
                "the stock-screen factor catalog is not available for broker unknown",
            ),
        ] {
            let actual = request_json(
                address,
                "GET",
                &format!("/api/v1/research/screens/catalog?{query}"),
                None,
            )
            .await;
            assert_eq!(actual["ok"], false, "catalog error query {query}");
            assert_eq!(actual["error"]["code"], code, "catalog error {query}");
            assert_eq!(actual["error"]["message"], message, "catalog error {query}");
        }
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn calendar_sources_route_matches_go_manager_fixture_in_cutover_only() {
        let fixture: Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/calendar-sources.json"
        ))
        .expect("calendar source fixture");
        let sources = serde_json::from_value(fixture["defaultSources"].clone())
            .expect("decode calendar source fixture rows");
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_calendar_source_snapshot_port(Arc::new(FixtureCalendarSourceSnapshotPort {
                    snapshot: jftrade_calendar::CalendarSourcesSnapshot { sources },
                }));
        let handle = start_product(config).await.expect("start product");
        assert_eq!(handle.startup_record().owned_routes, 45);
        let actual = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/exchange-calendars/sources",
            None,
        )
        .await;
        assert_eq!(actual["ok"], true);
        assert_eq!(actual["data"]["sources"], fixture["defaultSources"]);
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn calendar_sources_route_fails_closed_when_snapshot_port_is_unavailable() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_calendar_source_snapshot_port(Arc::new(FailingCalendarSourceSnapshotPort));
        let handle = start_product(config).await.expect("start product");
        let actual = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/exchange-calendars/sources",
            None,
        )
        .await;
        assert_eq!(actual["ok"], false);
        assert_eq!(
            actual["error"]["code"],
            "EXCHANGE_CALENDAR_SOURCES_UNAVAILABLE"
        );
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn calendar_status_route_matches_go_manager_fixture_in_cutover_only() {
        let fixture: Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/calendar-status.json"
        ))
        .expect("calendar status fixture");
        let status = serde_json::from_value(fixture["status"].clone())
            .expect("decode calendar status fixture");
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_calendar_status_snapshot_port(Arc::new(FixtureCalendarStatusSnapshotPort {
                    snapshot: status,
                }));
        let handle = start_product(config).await.expect("start product");
        assert_eq!(handle.startup_record().owned_routes, 45);
        let actual = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/exchange-calendars/status",
            None,
        )
        .await;
        assert_eq!(actual["ok"], true);
        assert_eq!(actual["data"], fixture["status"]);
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn calendar_status_route_fails_closed_when_snapshot_port_is_unavailable() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_calendar_status_snapshot_port(Arc::new(FailingCalendarStatusSnapshotPort));
        let handle = start_product(config).await.expect("start product");
        let actual = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/exchange-calendars/status",
            None,
        )
        .await;
        assert_eq!(actual["ok"], false);
        assert_eq!(
            actual["error"]["code"],
            "EXCHANGE_CALENDAR_STATUS_UNAVAILABLE"
        );
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn watchlist_memberships_route_matches_go_fixture_in_cutover_only() {
        let fixture: Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/watchlist-memberships.json"
        ))
        .expect("watchlist membership fixture");
        let mut memberships = std::collections::BTreeMap::new();
        for case in fixture["cases"].as_array().expect("membership cases") {
            if let Some(response) = case.get("response") {
                let decoded: jftrade_watchlist::Memberships =
                    serde_json::from_value(response.clone()).expect("membership response");
                memberships.insert(decoded.instrument_id.clone(), decoded);
            }
        }
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_watchlist_membership_snapshot_port(Arc::new(
                    FixtureWatchlistMembershipSnapshotPort { memberships },
                ));
        let handle = start_product(config).await.expect("start product");
        assert_eq!(handle.startup_record().owned_routes, 45);
        let address = handle.startup_record().address;
        for case in fixture["cases"].as_array().expect("membership cases") {
            let market = case["market"].as_str().expect("case market");
            let symbol = case["symbol"].as_str().expect("case symbol");
            let response = request_json(
                address,
                "GET",
                &format!("/api/v1/watchlist/instruments/{market}/{symbol}/memberships"),
                None,
            )
            .await;
            if case.get("response").is_some() {
                assert_eq!(response["ok"], true, "case {}", case["name"]);
                assert_eq!(response["data"], case["response"], "case {}", case["name"]);
            } else {
                assert_eq!(response["ok"], false, "case {}", case["name"]);
                assert_eq!(
                    response["error"]["code"], "WATCHLIST_INVALID",
                    "case {}",
                    case["name"]
                );
            }
        }
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn watchlist_memberships_route_fails_closed_when_snapshot_port_is_unavailable() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_watchlist_membership_snapshot_port(Arc::new(
                    FailingWatchlistMembershipSnapshotPort,
                ));
        let handle = start_product(config).await.expect("start product");
        let response = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/watchlist/instruments/US/AAPL/memberships",
            None,
        )
        .await;
        assert_eq!(response["ok"], false);
        assert_eq!(response["error"]["code"], "WATCHLIST_UNAVAILABLE");
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn plugin_uninstall_guidance_route_matches_go_fixture_in_cutover_only() {
        let fixture: Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/plugin-uninstall-guidance.json"
        ))
        .expect("plugin uninstall guidance fixture");
        let mut guidance = std::collections::BTreeMap::new();
        for case in fixture["cases"].as_array().expect("plugin guidance cases") {
            if let Some(response) = case.get("response") {
                let decoded: PluginUninstallGuidance =
                    serde_json::from_value(response.clone()).expect("plugin guidance response");
                guidance.insert(decoded.plugin_id.clone(), decoded);
            }
        }
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_plugin_uninstall_guidance_snapshot_port(Arc::new(
                    FixturePluginUninstallGuidanceSnapshotPort { guidance },
                ));
        let handle = start_product(config).await.expect("start product");
        assert_eq!(handle.startup_record().owned_routes, 45);
        let address = handle.startup_record().address;
        for case in fixture["cases"].as_array().expect("plugin guidance cases") {
            let request_path = case["requestPath"].as_str().expect("request path");
            let response = request_json(address, "GET", request_path, None).await;
            if let Some(expected) = case.get("response") {
                assert_eq!(response["ok"], true, "case {}", case["name"]);
                assert_eq!(response["data"], *expected, "case {}", case["name"]);
            } else {
                assert_eq!(response["ok"], false, "case {}", case["name"]);
                assert_eq!(
                    response["error"]["code"], case["errorCode"],
                    "case {}",
                    case["name"]
                );
            }
        }
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn plugin_uninstall_guidance_route_fails_closed_when_snapshot_port_is_unavailable() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_plugin_uninstall_guidance_snapshot_port(Arc::new(
                    FailingPluginUninstallGuidanceSnapshotPort,
                ));
        let handle = start_product(config).await.expect("start product");
        let response = request_json(
            handle.startup_record().address,
            "GET",
            "/api/v1/plugins/pine-plan/uninstall-guidance",
            None,
        )
        .await;
        assert_eq!(response["ok"], false);
        assert_eq!(
            response["error"]["code"],
            "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE"
        );
        handle.shutdown().await.expect("shutdown product");
    }

    #[tokio::test]
    async fn browser_authenticated_request_cannot_change_desktop_only_security_settings() {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let mut config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config");
        config.access = AccessPolicy {
            session_token: Some("browser-session".to_owned()),
            csrf_token: Some("browser-csrf".to_owned()),
            enforce_access: true,
            desktop_mode: false,
            ..AccessPolicy::default()
        }
        .with_allowed_origins(["https://jftrade.local".to_owned()]);
        let handle = start_product(config).await.expect("start product");
        let response = request_json_with_headers(
            handle.startup_record().address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"webPort":6688,"newPassword":"a sufficiently long password"}"#,
            ),
            &[
                ("Cookie", "jftrade_web_session=browser-session"),
                ("Origin", "https://jftrade.local"),
                ("X-CSRF-Token", "browser-csrf"),
            ],
        )
        .await;
        assert_eq!(
            response["error"]["code"],
            "WEB_ACCESS_SETTINGS_DESKTOP_ONLY"
        );
        handle.shutdown().await.expect("shutdown product");
        assert!(!settings_path.exists());
    }

    #[test]
    fn product_config_rejects_public_bind_and_missing_path() {
        assert!(matches!(
            ProductConfig::test_cutover("0.0.0.0:3000".parse().expect("address"), "settings.json"),
            Err(ProductError::NonLoopbackBind)
        ));
        assert!(matches!(
            ProductConfig::test_cutover("127.0.0.1:3000".parse().expect("address"), Path::new("")),
            Err(ProductError::MissingSettingsPath)
        ));
        assert!(matches!(
            ProductConfig::desktop_shadow(
                "127.0.0.1:3000".parse().expect("address"),
                "settings.json",
                "weak"
            ),
            Err(ProductError::WeakDesktopToken)
        ));
    }

    #[test]
    fn read_only_shadow_catalog_never_registers_write_or_notification_routes() {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct RouteOwnership {
            shadow_routes: Vec<OwnedRoute>,
            cutover_test_routes: Vec<OwnedRoute>,
        }

        #[derive(Deserialize)]
        struct OwnedRoute {
            method: String,
            path: String,
        }

        fn pairs(routes: &[RouteSpec]) -> Vec<(String, String)> {
            routes
                .iter()
                .map(|route| (route.method.clone(), route.path.clone()))
                .collect()
        }

        fn owned_pairs(routes: &[OwnedRoute]) -> Vec<(String, String)> {
            let mut pairs = routes
                .iter()
                .map(|route| (route.method.clone(), route.path.clone()))
                .collect::<Vec<_>>();
            pairs.sort();
            pairs
        }

        let ownership: RouteOwnership = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/route-ownership.json"
        ))
        .expect("route ownership ledger");
        let shadow = product_routes(false, false, false, false, false).expect("shadow routes");
        assert_eq!(shadow.routes().len(), 26);
        assert!(shadow.routes().iter().all(|route| route.method == "GET"));
        assert_eq!(
            pairs(shadow.routes()),
            owned_pairs(&ownership.shadow_routes)
        );
        let shadow_with_calendar_port = product_routes(false, true, true, true, true)
            .expect("shadow routes with unavailable cutover ports");
        assert_eq!(shadow_with_calendar_port.routes().len(), 26);
        assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
        }));
        assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
        }));
        assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
            route.method == "GET"
                && route.path == "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
        }));
        assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/plugins/{pluginId}/uninstall-guidance"
        }));
        let cutover_without_calendar_port = product_routes(true, false, false, false, false)
            .expect("cutover routes without calendar ports");
        assert_eq!(cutover_without_calendar_port.routes().len(), 44);
        assert!(!cutover_without_calendar_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
        }));
        assert!(!cutover_without_calendar_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
        }));
        let cutover_with_source_port = product_routes(true, true, false, false, false)
            .expect("cutover routes with source port");
        assert_eq!(cutover_with_source_port.routes().len(), 45);
        assert!(cutover_with_source_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
        }));
        assert!(!cutover_with_source_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
        }));
        let cutover_with_status_port = product_routes(true, false, true, false, false)
            .expect("cutover routes with status port");
        assert_eq!(cutover_with_status_port.routes().len(), 45);
        assert!(!cutover_with_status_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
        }));
        assert!(cutover_with_status_port.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
        }));
        let cutover =
            product_routes(true, true, true, true, true).expect("cutover routes with all ports");
        assert_eq!(cutover.routes().len(), 48);
        let mut expected_cutover = ownership
            .shadow_routes
            .iter()
            .chain(&ownership.cutover_test_routes)
            .map(|route| (route.method.clone(), route.path.clone()))
            .collect::<Vec<_>>();
        expected_cutover.sort();
        assert_eq!(pairs(cutover.routes()), expected_cutover);
        assert!(
            cutover
                .routes()
                .iter()
                .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/ui" })
        );
        assert!(cutover.routes().iter().any(|route| {
            route.method == "POST" && route.path == "/api/v1/settings/system-notifications/test"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "POST" && route.path == "/api/v1/settings/adk/mcp/token/reset"
        }));
        assert!(
            cutover.routes().iter().any(|route| {
                route.method == "PUT" && route.path == "/api/v1/settings/security"
            })
        );
        assert!(
            cutover.routes().iter().any(|route| {
                route.method == "PUT" && route.path == "/api/v1/settings/onboarding"
            })
        );
        assert!(cutover.routes().iter().any(|route| {
            route.method == "PUT" && route.path == "/api/v1/settings/exchange-calendars"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "DELETE"
                && route.path == "/api/v1/settings/broker-accounts/{accountRecordId}"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "GET"
                && route.path == "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
        }));
        assert!(cutover.routes().iter().any(|route| {
            route.method == "GET" && route.path == "/api/v1/plugins/{pluginId}/uninstall-guidance"
        }));
    }

    async fn request_json(
        address: SocketAddr,
        method: &str,
        path: &str,
        body: Option<&str>,
    ) -> Value {
        request_json_with_headers(address, method, path, body, &[]).await
    }

    async fn request_json_with_headers(
        address: SocketAddr,
        method: &str,
        path: &str,
        body: Option<&str>,
        headers: &[(&str, &str)],
    ) -> Value {
        let body = body.unwrap_or_default();
        let mut stream = TcpStream::connect(address)
            .await
            .expect("connect product API");
        let extra_headers = headers
            .iter()
            .map(|(name, value)| format!("{name}: {value}\r\n"))
            .collect::<String>();
        let request = format!(
            "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\n{extra_headers}Content-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        stream
            .write_all(request.as_bytes())
            .await
            .expect("write request");
        let mut response = Vec::new();
        stream
            .read_to_end(&mut response)
            .await
            .expect("read response");
        let response = String::from_utf8(response).expect("UTF-8 response");
        let (_, body) = response.split_once("\r\n\r\n").expect("HTTP body");
        serde_json::from_str(body).expect("JSON response")
    }
}
