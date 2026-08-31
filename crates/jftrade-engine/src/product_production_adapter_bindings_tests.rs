use super::*;
use crate::product::product_production_ports::production_ports;
use crate::product::{
    ActiveProviderState, MarketDataRuntimeState, MarketDataRuntimeStatusPort, ProductCapabilities,
    ProductConfig, product_data_management,
};
use jftrade_api::AccessPolicy;
use jftrade_integration_futu::{
    PredictionComboQuotePort, PredictionMarketReadError, PredictionMarketReadPort,
    PredictionMarketSubscriptionPort,
};
use jftrade_settings::MarketDataProviderRuntimePort;
use jftrade_settings::{MarketDataProvider, SecuritySettingsService};
use jftrade_store_settings_file::SettingsFileStore;
use serde_json::{Value, json};
use std::fs;
use std::sync::Arc;
use tempfile::TempDir;

#[derive(Debug)]
struct ReadyRuntimeStatus;

impl MarketDataRuntimeStatusPort for ReadyRuntimeStatus {
    fn snapshot(&self) -> MarketDataRuntimeState {
        MarketDataRuntimeState {
            connected: true,
            generation: 1,
            ..Default::default()
        }
    }
}

#[derive(Debug)]
struct PredictionReadFixture;

impl PredictionMarketReadPort for PredictionReadFixture {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, PredictionMarketReadError> {
        Ok(json!({"entries": []}))
    }
}

#[derive(Debug)]
struct PredictionSubscriptionFixture;

impl PredictionMarketSubscriptionPort for PredictionSubscriptionFixture {
    fn subscribe(
        &self,
        _code: &str,
        _data_types: &[String],
    ) -> Result<Value, PredictionMarketReadError> {
        Ok(json!({"subscribed": true}))
    }

    fn unsubscribe(&self, _code: &str) -> Result<Value, PredictionMarketReadError> {
        Ok(json!({"subscribed": false}))
    }
}

#[derive(Debug)]
struct PredictionComboFixture;

impl PredictionComboQuotePort for PredictionComboFixture {
    fn quote(&self, _payload: &Value) -> Result<Value, PredictionMarketReadError> {
        Ok(json!({"entries": []}))
    }
}

fn prediction_bundle(
    read: bool,
    subscription: bool,
    combo: bool,
) -> (TempDir, ProductionPortBundle) {
    let directory = tempfile::tempdir().expect("prediction binding temp directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}").expect("write settings");
    product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");
    let settings = Arc::new(SettingsFileStore::open(&settings_path).expect("settings store"));
    let security = SecuritySettingsService::new(settings);
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    runtime.set_prediction_adapters(
        read.then(|| Arc::new(PredictionReadFixture) as Arc<dyn PredictionMarketReadPort>),
        subscription.then(|| {
            Arc::new(PredictionSubscriptionFixture) as Arc<dyn PredictionMarketSubscriptionPort>
        }),
        combo.then(|| Arc::new(PredictionComboFixture) as Arc<dyn PredictionComboQuotePort>),
    );
    let mut config = ProductConfig::new(
        "127.0.0.1:0".parse().expect("bind address"),
        &settings_path,
        AccessPolicy::default(),
    )
    .expect("product config")
    .with_active_provider_state(active)
    .with_trade_runtime(runtime)
    .with_market_data_runtime_status_port(Arc::new(ReadyRuntimeStatus));
    config.capabilities = ProductCapabilities::all();
    config.production = true;
    let ports = production_ports(&config, &security).expect("production ports");
    (directory, ports)
}

#[test]
fn news_actions_binding_requires_yfinance_helper_readiness() {
    let ready = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
        Some("yfinance"),
        true,
        false,
    ));
    assert_eq!(
        ready.get(&ProductionRouteAdapter::MarketDataNewsActionsRead),
        Some(&ProductionAdapterBinding::Ready)
    );
    let akshare_ready = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
        Some("akshare"),
        true,
        false,
    ));
    assert_eq!(
        akshare_ready.get(&ProductionRouteAdapter::MarketDataNewsActionsRead),
        Some(&ProductionAdapterBinding::Ready)
    );

    for matrix in [
        MarketDataCapabilityMatrix::new(Some("yfinance"), false, false),
        MarketDataCapabilityMatrix::new(Some("akshare"), false, false),
        MarketDataCapabilityMatrix::new(Some("futu"), false, true),
    ] {
        let bindings = production_adapter_bindings(&matrix);
        assert_eq!(
            bindings.get(&ProductionRouteAdapter::MarketDataNewsActionsRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
    }
}

#[test]
fn option_chain_binding_defaults_to_external_unavailable() {
    let bindings =
        production_adapter_bindings(&MarketDataCapabilityMatrix::new(Some("futu"), false, true));
    assert_eq!(
        bindings.get(&ProductionRouteAdapter::MarketDataOptionsChainRead),
        Some(&ProductionAdapterBinding::ExternalUnavailable)
    );
    assert_eq!(
        bindings.get(&ProductionRouteAdapter::MarketDataOptionsScreenRead),
        Some(&ProductionAdapterBinding::ExternalUnavailable)
    );
    assert_eq!(
        bindings.get(&ProductionRouteAdapter::MarketDataOptionsAnalysisRead),
        Some(&ProductionAdapterBinding::ExternalUnavailable)
    );
    assert_eq!(
        bindings.get(&ProductionRouteAdapter::MarketDataOptionsEventsRead),
        Some(&ProductionAdapterBinding::ExternalUnavailable)
    );
}

#[test]
fn prediction_readiness_is_independent_per_operation_adapter() {
    let cases = [
        (
            true,
            false,
            false,
            ProductionRouteAdapter::MarketDataPredictionRead,
            vec![
                ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite,
                ProductionRouteAdapter::MarketDataPredictionSubscriptionReleaseWrite,
                ProductionRouteAdapter::MarketDataPredictionCombosWrite,
            ],
        ),
        (
            false,
            true,
            false,
            ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite,
            vec![
                ProductionRouteAdapter::MarketDataPredictionRead,
                ProductionRouteAdapter::MarketDataPredictionCombosWrite,
            ],
        ),
        (
            false,
            false,
            true,
            ProductionRouteAdapter::MarketDataPredictionCombosWrite,
            vec![
                ProductionRouteAdapter::MarketDataPredictionRead,
                ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite,
            ],
        ),
    ];
    for (read, subscription, combo, ready_adapter, unavailable_adapters) in cases {
        let (_directory, ports) = prediction_bundle(read, subscription, combo);
        assert_eq!(
            ports.adapter_binding(ready_adapter),
            Some(ProductionAdapterBinding::Ready),
            "installed prediction adapter should become ready when its own reader exists"
        );
        for adapter in unavailable_adapters {
            assert_eq!(
                ports.adapter_binding(adapter),
                Some(ProductionAdapterBinding::ExternalUnavailable),
                "prediction readiness must not leak between operation adapters"
            );
        }
    }
}

#[test]
fn prediction_readiness_requires_futu_and_opend() {
    let (_directory, ports) = prediction_bundle(true, true, true);
    let state = Arc::clone(&ports.active_provider_state);
    assert_eq!(
        ports.adapter_binding(ProductionRouteAdapter::MarketDataPredictionRead),
        Some(ProductionAdapterBinding::Ready)
    );

    state
        .activate(MarketDataProvider::Yfinance)
        .expect("provider switch");
    assert_eq!(
        ports.adapter_binding(ProductionRouteAdapter::MarketDataPredictionRead),
        Some(ProductionAdapterBinding::ExternalUnavailable),
        "prediction is an OpenD/Futu capability"
    );

    state
        .activate(MarketDataProvider::Futu)
        .expect("provider switch back");
    state.set_readiness(false, false, false);
    assert_eq!(
        ports.adapter_binding(ProductionRouteAdapter::MarketDataPredictionRead),
        Some(ProductionAdapterBinding::ExternalUnavailable),
        "a disconnected OpenD runtime must not advertise prediction"
    );
}

#[test]
fn prediction_readiness_reports_missing_uninstalled_adapter() {
    let (_directory, mut ports) = prediction_bundle(true, false, false);
    let adapter = ProductionRouteAdapter::MarketDataPredictionRead;
    ports.installed_adapters.remove(&adapter);
    ports.bound_adapters.remove(&adapter);
    assert_eq!(ports.adapter_binding(adapter), None);
    assert_eq!(
        ports.adapter_binding_or_missing(adapter),
        ProductionAdapterBinding::MissingInternalAdapter
    );
}
