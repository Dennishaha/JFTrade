use std::env;
use std::fs::File;
use std::io::{BufReader, Read};
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
use sha2::{Digest, Sha256};
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
pub const PRODUCT_REHEARSAL_PROTOCOL_VERSION: &str = "jftrade-product-rehearsal.v1";
pub const PRODUCT_READ_ONLY_ROUTE_PROFILE: &str = "read-only-shadow.v1";
pub const PRODUCT_TEST_CUTOVER_ROUTE_PROFILE: &str = "cutover-test-only.v1";

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
    capabilities: ProductCapabilities,
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
            calendar_source_snapshot_port: None,
            calendar_status_snapshot_port: None,
            watchlist_membership_snapshot_port: None,
            plugin_uninstall_guidance_snapshot_port: None,
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
    let route_ports = ProductRoutePorts {
        calendar_sources: config.calendar_source_snapshot_port.is_some(),
        calendar_status: config.calendar_status_snapshot_port.is_some(),
        watchlist_memberships: config.watchlist_membership_snapshot_port.is_some(),
        plugin_uninstall_guidance: config.plugin_uninstall_guidance_snapshot_port.is_some(),
    };
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
        config.capabilities.clone(),
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

fn current_executable_sha256() -> Result<String, ProductError> {
    let path = env::current_exe().map_err(ProductError::CurrentExecutable)?;
    let file = File::open(&path).map_err(|source| ProductError::ReadExecutable {
        path: path.clone(),
        source,
    })?;
    let mut reader = BufReader::new(file);
    let mut buffer = [0_u8; 64 * 1024];
    let mut digest = Sha256::new();
    loop {
        let count = reader
            .read(&mut buffer)
            .map_err(|source| ProductError::ReadExecutable {
                path: path.clone(),
                source,
            })?;
        if count == 0 {
            break;
        }
        digest.update(&buffer[..count]);
    }
    Ok(encode_sha256(digest.finalize()))
}

fn encode_sha256(digest: impl IntoIterator<Item = u8>) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut encoded = String::with_capacity(64);
    for byte in digest {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    encoded
}

include!("product_route_assembly.rs");

include!("product_api.rs");

include!("product_wire.rs");

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
    #[error("Rust product API task failed")]
    Join(#[source] tokio::task::JoinError),
    #[error("Rust product API transport failed")]
    Transport(#[from] std::io::Error),
}

#[cfg(test)]
#[path = "product_tests.rs"]
mod tests;
