//! Composition of the shared active-provider state for the product runtime.
//!
//! The dynamic readiness reader and the activation callback are the two
//! composition-root seams that decide when a provider may activate and what
//! "ready" means at runtime.  Both read the same live evidence: the managed
//! helper process plus its `/healthz` monitor (one source of truth for helper
//! readiness, also consumed by the capability matrix through the shared
//! snapshot), the dynamic OpenD provider runtime recorder, and the configured
//! provider router.

use std::sync::{Arc, Mutex};

use jftrade_integration_futu::{OpenDProviderRuntime, OpenDProviderRuntimeError};
use jftrade_integration_marketdata_helper::HelperProcess;
use jftrade_marketdata::ProviderRouter;
use jftrade_settings::MarketDataProvider;

use super::product_runtime_composition::{SharedOpenDProviderRuntime, opend_provider_config};
use super::product_runtime_helper_health::HelperHealthMonitor;
use super::product_runtime_opend_listener::LiveHubOpenDEventListener;
use crate::product::product_production_ports::SharedTradeReadRuntime;

pub(super) type DynamicReadiness = Arc<dyn Fn() -> (bool, bool, bool) + Send + Sync>;

/// Build the dynamic readiness reader: helper process + health monitor,
/// OpenD provider recorder/physical state, router presence.
pub(super) fn dynamic_provider_readiness(
    helper_process: &Option<Arc<Mutex<Option<HelperProcess>>>>,
    helper_health: Option<Arc<HelperHealthMonitor>>,
    dynamic_opend: &SharedOpenDProviderRuntime,
    market_data_router: &Option<Arc<Mutex<ProviderRouter>>>,
) -> DynamicReadiness {
    let dyn_helper_for_readiness = helper_process.clone();
    let dyn_opend_for_readiness = Arc::clone(dynamic_opend);
    let dyn_router_for_readiness = market_data_router.clone();
    Arc::new(move || {
        let helper_ready = if let Some(ref proc_arc) = dyn_helper_for_readiness {
            if let Ok(mut proc_opt) = proc_arc.lock() {
                if let Some(ref mut proc) = *proc_opt {
                    proc.is_alive()
                        && proc.snapshot().state
                            == jftrade_integration_marketdata_helper::ProcessState::Ready
                        && helper_health
                            .as_ref()
                            .is_some_and(|monitor| monitor.is_ready())
                } else {
                    false
                }
            } else {
                false
            }
        } else {
            false
        };
        let opend_ready = if let Some(ref provider) = *dyn_opend_for_readiness
            .lock()
            .unwrap_or_else(|error| error.into_inner())
        {
            let coordinator_lock = provider.coordinator();
            if let Ok(coordinator) = coordinator_lock.lock() {
                let recorder_snap = coordinator.recorder().snapshot();
                let physical_snap = coordinator.physical_snapshot();
                recorder_snap.connected
                    && recorder_snap.generation > 0
                    && recorder_snap
                        .quote_last_error
                        .as_deref()
                        .unwrap_or_default()
                        .is_empty()
                    && recorder_snap
                        .stream_last_error
                        .as_deref()
                        .unwrap_or_default()
                        .is_empty()
                    && physical_snap
                        .as_ref()
                        .and_then(|snapshot| snapshot.last_error.as_deref())
                        .unwrap_or_default()
                        .is_empty()
            } else {
                false
            }
        } else {
            false
        };
        let router_ready = dyn_router_for_readiness.is_some();
        (helper_ready, opend_ready, router_ready)
    })
}

type Activation =
    Arc<dyn Fn(MarketDataProvider, Option<MarketDataProvider>) -> Result<(), String> + Send + Sync>;

/// Build the activation callback: starting the Futu provider runtime on
/// demand, or failing closed when a helper-backed provider is requested while
/// the helper is not ready.
pub(super) fn provider_activation(
    helper_process: &Option<Arc<Mutex<Option<HelperProcess>>>>,
    helper_health: Option<Arc<HelperHealthMonitor>>,
    dynamic_opend: &SharedOpenDProviderRuntime,
    market_data_router: &Option<Arc<Mutex<ProviderRouter>>>,
    live_hub: &Arc<jftrade_api::LiveHub>,
    settings_path: &std::path::Path,
    trade_runtime: Arc<SharedTradeReadRuntime>,
) -> Result<Activation, OpenDProviderRuntimeError> {
    let dyn_helper_for_activation = helper_process.clone();
    let activation_runtime = Arc::clone(dynamic_opend);
    let activation_router = market_data_router.clone();
    let activation_hub = Arc::clone(live_hub);
    let settings_path = settings_path.to_owned();
    let trade_runtime_for_activation = Arc::clone(&trade_runtime);
    Ok(Arc::new(move |provider, previous| {
        let mut runtime = activation_runtime
            .lock()
            .map_err(|error| format!("failed to lock provider runtime: {error}"))?;
        match provider {
            MarketDataProvider::Futu => {
                if runtime.is_none() {
                    let router = activation_router.as_ref().ok_or_else(|| {
                        "market-data provider router is not configured".to_owned()
                    })?;
                    let mut configuration =
                        opend_provider_config(&settings_path, Arc::clone(router))
                            .map_err(|error| error.to_string())?;
                    configuration.task.event_listener = Some(Arc::new(
                        LiveHubOpenDEventListener::with_reconciliation_wake(
                            Arc::clone(&activation_hub),
                            trade_runtime_for_activation.reconciliation_wake(),
                        ),
                    ));
                    let provider = OpenDProviderRuntime::start(configuration)
                        .map_err(|error| error.to_string())?;
                    let trade_logged_in = provider.trade_logged_in();
                    let client = provider
                        .coordinator()
                        .lock()
                        .ok()
                        .and_then(|coordinator| {
                            jftrade_integration_futu::OpenDTradeReadClient::from_coordinator(
                                &coordinator,
                            )
                            .ok()
                        })
                        .map(Arc::new);
                    let read_client = client
                        .clone()
                        .map(|client| client as Arc<dyn jftrade_integration_futu::TradeReadPort>);
                    let write_client = client
                        .map(|client| client as Arc<dyn jftrade_integration_futu::TradeWritePort>);
                    let historical_reader = {
                        let coordinator = provider.coordinator();
                        Arc::new(jftrade_integration_futu::OpenDHistoricalKlineReader::new(
                            coordinator,
                        ))
                            as Arc<dyn jftrade_integration_futu::HistoricalKlineReadPort>
                    };
                    let security_snapshot_reader = {
                        let coordinator = provider.coordinator();
                        Arc::new(jftrade_integration_futu::OpenDSecuritySnapshotReader::new(
                            coordinator
                                .lock()
                                .map_err(|error| {
                                    format!("failed to lock OpenD coordinator: {error}")
                                })?
                                .session_clone()
                                .map_err(|error| error.to_string())?,
                        ))
                            as Arc<dyn jftrade_integration_futu::SecuritySnapshotReadPort>
                    };
                    trade_runtime_for_activation.set(read_client, trade_logged_in);
                    trade_runtime_for_activation.set_writer(write_client);
                    trade_runtime_for_activation.set_historical_klines(Some(historical_reader));
                    trade_runtime_for_activation.set_news_reader(Some(Arc::new(
                        jftrade_integration_futu::OpenDNewsReader::new(provider.coordinator()),
                    )));
                    trade_runtime_for_activation.set_corporate_actions_reader(Some(Arc::new(
                        jftrade_integration_futu::OpenDCorporateActionsReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_future_info(Some(Arc::new(
                        jftrade_integration_futu::OpenDFutureInfoReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_valuation_detail(Some(Arc::new(
                        jftrade_integration_futu::OpenDValuationDetailReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_expirations(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionExpirationReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_chains(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionChainReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_screens(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionScreenReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_quotes(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionQuoteReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_volatility(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionVolatilityReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_exercise_probability(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionExerciseProbabilityReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_underlying_overview(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionUnderlyingOverviewReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_underlying_his_volatility(Some(
                        Arc::new(
                            jftrade_integration_futu::OpenDOptionUnderlyingHisVolatilityReader::new(
                                provider.coordinator(),
                            ),
                        ),
                    ));
                    trade_runtime_for_activation.set_option_market_statistic(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionMarketStatisticReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_underlying_his_statistic(Some(
                        Arc::new(
                            jftrade_integration_futu::OpenDOptionUnderlyingHisStatisticReader::new(
                                provider.coordinator(),
                            ),
                        ),
                    ));
                    trade_runtime_for_activation.set_option_strategy_spread(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionStrategySpreadReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_strategy(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionStrategyReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_strategy_analysis(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionStrategyAnalysisReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_underlying_rank(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionUnderlyingRankReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_contract_rank(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionContractRankReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_events(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionEventReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_zero_dte_screener(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionZeroDteScreenerReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_earnings_screener(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionEarningsScreenerReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_zero_dte_contract(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionZeroDteContractReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation.set_option_seller_screener(Some(Arc::new(
                        jftrade_integration_futu::OpenDOptionSellerScreenerReader::new(
                            provider.coordinator(),
                        ),
                    )));
                    trade_runtime_for_activation
                        .set_security_snapshots(Some(security_snapshot_reader));
                    *runtime = Some(provider);
                    // Provider activation is a readiness transition even
                    // when the session has not yet emitted a reconnect event.
                    // Wake reconciliation once the live trade readers have
                    // been installed so pending orders are retried promptly.
                    trade_runtime_for_activation
                        .reconciliation_wake()
                        .notify_one();
                }
            }
            MarketDataProvider::Yfinance | MarketDataProvider::Akshare => {
                let is_helper_ready = if let Some(ref proc_arc) = dyn_helper_for_activation {
                    if let Ok(mut proc_opt) = proc_arc.lock() {
                        if let Some(ref mut proc) = *proc_opt {
                            proc.is_alive()
                                && proc.snapshot().state
                                    == jftrade_integration_marketdata_helper::ProcessState::Ready
                                && helper_health
                                    .as_ref()
                                    .is_some_and(|monitor| monitor.is_ready())
                        } else {
                            false
                        }
                    } else {
                        false
                    }
                } else {
                    true
                };
                if !is_helper_ready && previous == Some(MarketDataProvider::Futu) {
                    return Err("market-data helper is not ready".to_owned());
                }
                // OpenD is also the authenticated Futu trade owner.  Keep it
                // alive while helper-backed market data is active so account,
                // order, history, fill and fee reconciliation can continue.
                // The active-provider snapshot gates market-data routes, while
                // the execution reconciliation port reads this independent
                // trade runtime.  The production shutdown supervisor remains
                // the sole owner responsible for closing it.
            }
        }
        Ok(())
    }))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn readiness_without_composed_runtimes_reports_all_false() {
        let dynamic_opend: SharedOpenDProviderRuntime = Arc::new(Mutex::new(None));
        let readiness = dynamic_provider_readiness(&None, None, &dynamic_opend, &None);
        assert_eq!(readiness(), (false, false, false));
    }
}
