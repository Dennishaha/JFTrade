use super::*;

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
