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
    BrokerSettingsStorePort, FutuOpenDInstallSettingsStorePort, MarketDataProvider,
    MarketDataProviderSettingsStorePort, parse_market_data_provider,
};
use jftrade_store_settings_file::SettingsFileStore;

use super::{ProductRuntimeConfig, ProductRuntimeError};

pub(crate) fn compose_market_data_runtime(
    config: &mut ProductRuntimeConfig,
) -> Result<(), ProductRuntimeError> {
    let settings_path = config.product.settings_path();
    let settings_exists = std::path::Path::new(settings_path).exists();
    let futu_env_override = std::env::var_os("JFTRADE_FUTU_OPEND_HOST").is_some()
        || std::env::var_os("JFTRADE_FUTU_OPEND_PORT").is_some();
    if !settings_exists && !futu_env_override {
        return Ok(());
    }
    let store = settings_exists
        .then(|| SettingsFileStore::open_read_only(settings_path))
        .transpose()
        .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?;
    let active_provider = store
        .as_ref()
        .map(|store| {
            store
                .load_active_market_data_provider()
                .map_err(|e| ProductRuntimeError::Settings(e.to_string()))
        })
        .transpose()?
        .flatten()
        .map(|p| {
            parse_market_data_provider(&p)
                .map_err(|error| ProductRuntimeError::Settings(error.to_string()))
        })
        .transpose()?;
    // Futu's OpenD session is a shared trade owner, not merely a market-data
    // provider.  When a broker integration is enabled, compose it even if
    // yfinance/AKShare currently owns market-data reads; reconciliation then
    // keeps account/order/history/fill/fee visibility across provider switches.
    let futu_trade_enabled = store
        .as_ref()
        .map(|store| {
            store
                .load_broker_settings_inputs()
                .map_err(|error| ProductRuntimeError::Settings(error.to_string()))
        })
        .transpose()?
        .and_then(|inputs| inputs.saved_integration)
        .is_some_and(|integration| integration.enabled);
    let router = config
        .market_data_router
        .clone()
        .unwrap_or_else(|| Arc::new(Mutex::new(ProviderRouter::new(512))));

    if active_provider == Some(MarketDataProvider::Futu) || futu_trade_enabled || futu_env_override
    {
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
    let futu_settings = if settings_path.exists() {
        let store = SettingsFileStore::open_read_only(settings_path)
            .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?;
        store
            .load_futu_open_d_install_settings()
            .map_err(|e| ProductRuntimeError::Settings(e.to_string()))?
    } else {
        None
    };
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
