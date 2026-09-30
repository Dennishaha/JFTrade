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
        Err(_) => default_opend_port(futu_settings.as_ref())?,
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

/// Returns the TCP API port for a missing or persisted OpenD integration.
///
/// Parity: `go:452dea11:pkg/futu/exchange.go:45` defines
/// `DefaultOpenDAddr = "127.0.0.1:11110"`. Port `11111` is the separate
/// OpenD WebSocket port and cannot answer the TCP API handshake, so the
/// fallback must stay on `11110` whenever no integration is persisted.
fn default_opend_port(
    settings: Option<&jftrade_settings::FutuOpenDInstallSettings>,
) -> Result<u16, ProductRuntimeError> {
    settings.map_or(Ok(11_110), |settings| {
        u16::try_from(settings.api_port).map_err(|_| {
            ProductRuntimeError::Settings(format!(
                "invalid futu open d api_port: {}",
                settings.api_port
            ))
        })
    })
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

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_marketdata::PhysicalSubscriptionSnapshotPort;

    #[test]
    // Parity: go:452dea11:pkg/futu/exchange_test.go:36 TestConstructorFallsBackToDefaultAddress
    fn opend_provider_config_defaults_to_the_go_tcp_api_port() {
        // Parity: go:452dea11:pkg/futu/exchange.go:45 DefaultOpenDAddr and
        // pkg/futu/exchange_test.go:36 TestConstructorFallsBackToDefaultAddress.
        // Without a persisted integration the fallback must be the TCP API
        // port 11110, not the adjacent WebSocket port 11111.
        assert_eq!(default_opend_port(None).expect("default port"), 11_110);
        assert_eq!(
            default_opend_port(Some(&jftrade_settings::FutuOpenDInstallSettings {
                api_port: 12_345,
                ..Default::default()
            }))
            .expect("persisted port"),
            12_345
        );
        let invalid = default_opend_port(Some(&jftrade_settings::FutuOpenDInstallSettings {
            api_port: -1,
            ..Default::default()
        }));
        assert!(matches!(invalid, Err(ProductRuntimeError::Settings(_))));
    }

    #[test]
    fn dynamic_opend_adapter_reports_no_snapshot_without_a_live_runtime() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:196
        // TestMarketDataRuntimeNilAndClosedLifecycleBoundaries and
        // internal/integration/futu/marketdata_runtime_opend_test.go:154
        // TestMarketDataRuntimeQueryAndSubscriptionWrappers.
        // The empty slot is the Rust analogue of Go's "runtime without an
        // exchange": the port must answer `None` instead of erroring or
        // fabricating an empty physical snapshot that would let a caller
        // believe OpenD had released every subscription.
        let adapter = DynamicOpenDPhysicalSubscriptionAdapter {
            runtime: Arc::new(Mutex::new(None)),
        };
        assert_eq!(
            adapter.physical_subscription_snapshot(),
            Ok(None),
            "an uncomposed OpenD runtime must not report a physical snapshot"
        );
    }
}
