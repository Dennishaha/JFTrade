use std::net::{IpAddr, SocketAddr};
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use jftrade_integration_futu::{
    OpenDProviderRuntime, OpenDProviderRuntimeConfig, OpenDSessionCoordinator, OpenDTcpProbeConfig,
    provider_descriptor,
};
use jftrade_marketdata::{
    PhysicalSubscriptionSnapshot, PhysicalSubscriptionSnapshotPort, ProviderRouter,
};
use jftrade_settings::{
    FutuOpenDInstallSettingsStorePort, MarketDataProvider, MarketDataProviderSettingsStorePort,
    parse_market_data_provider,
};
use jftrade_store_settings_file::SettingsFileStore;

use super::{ProductRuntimeConfig, ProductRuntimeError};

pub(crate) fn compose_market_data_runtime(
    config: &mut ProductRuntimeConfig,
) -> Result<(), ProductRuntimeError> {
    let settings_path = config.product.settings_path();
    if !std::path::Path::new(settings_path).exists() {
        return Ok(());
    }
    let store = SettingsFileStore::open_read_only(settings_path)
        .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?;
    let active_provider = store
        .load_active_market_data_provider()
        .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?
        .map(|p| {
            parse_market_data_provider(&p)
                .map_err(|error| ProductRuntimeError::Settings(error.to_string()))
        })
        .transpose()?;
    let router = config
        .market_data_router
        .clone()
        .unwrap_or_else(|| Arc::new(Mutex::new(ProviderRouter::new(512))));

    if active_provider == Some(MarketDataProvider::Futu) {
        let provider_config = opend_provider_config(settings_path, Arc::clone(&router))?;
        config.market_data_opend_provider = Some(provider_config);
        // Keep the single router in the composition even while Futu owns the
        // physical runtime.  Provider activation can then move Futu -> helper
        // without manufacturing a second DemandBook/router.
        config.market_data_router = Some(router);
    } else {
        config.market_data_router = Some(router);
        config.market_data_opend_provider = None;
    }
    Ok(())
}

pub(crate) fn opend_provider_config(
    settings_path: &std::path::Path,
    router: Arc<Mutex<ProviderRouter>>,
) -> Result<OpenDProviderRuntimeConfig, ProductRuntimeError> {
    let store = SettingsFileStore::open_read_only(settings_path)
        .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?;
    let futu_settings = store
        .load_futu_open_d_install_settings()
        .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?;
    let host = std::env::var("JFTRADE_FUTU_OPEND_HOST")
        .ok()
        .or_else(|| futu_settings.as_ref().map(|s| s.host.clone()))
        .unwrap_or_else(|| "127.0.0.1".to_owned());
    let port = match std::env::var("JFTRADE_FUTU_OPEND_PORT") {
        Ok(value) => value.parse::<u16>().map_err(|_| {
            ProductRuntimeError::Settings(format!("invalid JFTRADE_FUTU_OPEND_PORT: {value}"))
        })?,
        Err(_) => futu_settings.as_ref().map_or(Ok(11111), |settings| {
            u16::try_from(settings.api_port).map_err(|_| {
                ProductRuntimeError::Settings(format!(
                    "invalid futu open d api_port: {}",
                    settings.api_port
                ))
            })
        })?,
    };
    let ip = host.parse::<IpAddr>().map_err(|_| {
        ProductRuntimeError::Settings(format!("invalid Futu OpenD host IP: {host}"))
    })?;
    let now_ms = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .unwrap_or_default();
    let mut configuration = OpenDProviderRuntimeConfig::with_defaults(
        router,
        provider_descriptor(),
        OpenDTcpProbeConfig::new(SocketAddr::new(ip, port), Duration::from_millis(500)),
        Vec::new(),
        now_ms,
    );
    configuration.task.quota_refresh_enabled = true;
    Ok(configuration)
}

pub(crate) type SharedOpenDProviderRuntime = Arc<Mutex<Option<OpenDProviderRuntime>>>;

#[derive(Debug)]
pub(crate) struct DynamicOpenDPhysicalSubscriptionAdapter {
    pub(crate) runtime: SharedOpenDProviderRuntime,
}

impl PhysicalSubscriptionSnapshotPort for DynamicOpenDPhysicalSubscriptionAdapter {
    fn physical_subscription_snapshot(
        &self,
    ) -> Result<Option<PhysicalSubscriptionSnapshot>, String> {
        let runtime = self
            .runtime
            .lock()
            .map_err(|error| format!("failed to acquire OpenD runtime lock: {error}"))?;
        runtime
            .as_ref()
            .map_or(Ok(None), OpenDProviderRuntime::physical_snapshot)
    }
}

#[derive(Debug)]
pub(crate) struct OpenDPhysicalSubscriptionAdapter {
    pub(crate) coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
}

impl PhysicalSubscriptionSnapshotPort for OpenDPhysicalSubscriptionAdapter {
    fn physical_subscription_snapshot(
        &self,
    ) -> Result<Option<PhysicalSubscriptionSnapshot>, String> {
        let coordinator = self
            .coordinator
            .lock()
            .map_err(|error| format!("failed to acquire coordinator lock: {error}"))?;
        Ok(coordinator.physical_snapshot())
    }
}
