//! Startup helpers for OpenD session hooks and provider status.

use super::*;

pub(super) fn configure_opend_hooks(
    resolver: Option<&Arc<dyn jftrade_integration_futu::QuoteSessionResolver>>,
    coordinator: Option<&Arc<Mutex<OpenDSessionCoordinator>>>,
    task: &mut Option<OpenDSessionRuntimeConfig>,
    live_hub: &Arc<jftrade_api::LiveHub>,
    trade_runtime: &Arc<crate::product::product_production_ports::SharedTradeReadRuntime>,
) {
    if let Some(resolver) = resolver
        && let Some(coordinator) = coordinator
    {
        coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .set_session_resolver(Some(Arc::clone(resolver)));
    }
    if let Some(resolver) = resolver
        && let Some(task) = task.as_mut()
    {
        task.session_resolver = Some(Arc::clone(resolver));
    }
    if let Some(task) = task.as_mut()
        && task.event_listener.is_none()
    {
        task.event_listener = Some(Arc::new(
            LiveHubOpenDEventListener::with_reconciliation_wake(
                Arc::clone(live_hub),
                trade_runtime.reconciliation_wake(),
            ),
        ));
    }
}

pub(super) fn configure_provider_hooks(
    provider: &mut OpenDProviderRuntimeConfig,
    resolver: Option<&Arc<dyn jftrade_integration_futu::QuoteSessionResolver>>,
    live_hub: &Arc<jftrade_api::LiveHub>,
    trade_runtime: &Arc<crate::product::product_production_ports::SharedTradeReadRuntime>,
) {
    if let Some(resolver) = resolver {
        provider.task.session_resolver = Some(Arc::clone(resolver));
    }
    if provider.task.event_listener.is_none() {
        provider.task.event_listener = Some(Arc::new(
            LiveHubOpenDEventListener::with_reconciliation_wake(
                Arc::clone(live_hub),
                trade_runtime.reconciliation_wake(),
            ),
        ));
    }
}
pub(super) fn production_provider_status(
    configured: bool,
    recorder: Option<&MarketDataRuntimeRecorder>,
) -> ProductionRuntimeStatus {
    let Some(state) = recorder.map(MarketDataRuntimeRecorder::snapshot) else {
        return match configured {
            true => ProductionRuntimeStatus::Degraded,
            false => ProductionRuntimeStatus::Unavailable,
        };
    };
    match (state.closed, state.connected) {
        (true, _) => ProductionRuntimeStatus::Failed,
        (false, true) => ProductionRuntimeStatus::Ready,
        (false, false) => ProductionRuntimeStatus::Degraded,
    }
}
