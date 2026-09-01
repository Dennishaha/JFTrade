//! Product runtime startup orchestration.

use super::*;

pub async fn start_product_runtime(
    mut config: ProductRuntimeConfig,
) -> Result<ProductRuntimeHandle, ProductRuntimeError> {
    // A production caller cannot smuggle a fixture/embedding execution port
    // into the API.  Backtest execution is bound below only after a real Pine
    // worker passes its readiness probe; without one the route remains
    // reachable and fails closed with the baseline 503 response.
    if config.product.is_production() {
        config.backtest_execution_port = None;
    }
    if config.market_data_opend_provider.is_none()
        && config.market_data_opend.is_none()
        && config.market_data_opend_task.is_none()
    {
        compose_market_data_runtime(&mut config)?;
    }
    let opend_configured = config.market_data_opend.is_some()
        || config.market_data_opend_task.is_some()
        || config.market_data_opend_provider.is_some();
    let opend_task_configured = config.market_data_opend_task.is_some();
    let helper_configured = config.marketdata_helper.is_some();
    let pine_workers_configured = !config.pine_workers.is_empty();
    let provider_configured = opend_configured
        || config.market_data_router.is_some()
        || config.market_data_runtime_recorder.is_some()
        || config.marketdata_helper.is_some();
    let worker_configured = pine_workers_configured || helper_configured;
    if config.market_data_opend_provider.is_some()
        && (config.market_data_runtime_recorder.is_some()
            || config.market_data_opend.is_some()
            || config.market_data_opend_task.is_some())
    {
        return Err(ProductRuntimeError::ConflictingMarketDataOwners);
    }
    if config.market_data_opend_task.is_some() && config.market_data_opend.is_none() {
        return Err(ProductRuntimeError::MissingOpenDSession);
    }
    // Validate/create the production schema before any external worker or
    // provider is started.  A migration failure therefore cannot leave an
    // external process running while the API is unable to serve its stores.
    if config.product.is_production() {
        crate::product_data_management::initialize_production_databases(
            config.product.settings_path(),
        )
        .map_err(ProductError::Storage)?;
    }
    let live_hub = config
        .product
        .live_hub
        .clone()
        .unwrap_or_else(|| Arc::new(jftrade_api::LiveHub::default()));
    config.product = config.product.with_live_hub(Arc::clone(&live_hub));
    let trade_runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    config.product = config
        .product
        .with_trade_runtime(Arc::clone(&trade_runtime));

    let market_data_router = config.market_data_router.take();
    let market_data_opend = config.market_data_opend.take();
    let mut market_data_opend_task = config.market_data_opend_task.take();
    if let Some(task) = market_data_opend_task.as_mut()
        && task.event_listener.is_none()
    {
        task.event_listener = Some(Arc::new(
            LiveHubOpenDEventListener::with_reconciliation_wake(
                Arc::clone(&live_hub),
                trade_runtime.reconciliation_wake(),
            ),
        ));
    }
    let (market_data_opend_provider, market_data_router) = if let Some(mut provider) =
        config.market_data_opend_provider.clone()
    {
        let shared_router = Arc::clone(&provider.router);
        if provider.task.event_listener.is_none() {
            provider.task.event_listener = Some(Arc::new(
                LiveHubOpenDEventListener::with_reconciliation_wake(
                    Arc::clone(&live_hub),
                    trade_runtime.reconciliation_wake(),
                ),
            ));
        }
        match OpenDProviderRuntime::start(provider) {
            Ok(runtime) => {
                let trade_logged_in = runtime.trade_logged_in();
                let trade_client = runtime
                    .coordinator()
                    .lock()
                    .ok()
                    .and_then(|coordinator| {
                        OpenDTradeReadClient::from_coordinator(&coordinator).ok()
                    })
                    .map(Arc::new);
                let trade_read_port = trade_client
                    .clone()
                    .map(|client| client as Arc<dyn TradeReadPort>);
                let trade_write_port = trade_client.map(|client| client as Arc<dyn TradeWritePort>);
                let historical_reader = {
                    let coordinator = runtime.coordinator();
                    Arc::new(jftrade_integration_futu::OpenDHistoricalKlineReader::new(
                        coordinator,
                    ))
                        as Arc<dyn jftrade_integration_futu::HistoricalKlineReadPort>
                };
                config.product = config
                    .product
                    .clone()
                    .with_trade_read_port(trade_read_port, trade_logged_in);
                config.product = config
                    .product
                    .clone()
                    .with_trade_write_port(trade_write_port);
                trade_runtime.set(config.product.trade_read_port.clone(), trade_logged_in);
                trade_runtime.set_writer(config.product.trade_write_port.clone());
                trade_runtime.set_historical_klines(Some(historical_reader));
                let customization_reader = Arc::new(
                    jftrade_integration_futu::FutuRemoteWatchlistReader::new(runtime.coordinator()),
                );
                let alert_reader = Arc::new(jftrade_integration_futu::FutuAlertQuery {
                    coordinator: runtime.coordinator(),
                });
                let alert_writer = Arc::new(jftrade_integration_futu::FutuAlertWrite {
                    coordinator: runtime.coordinator(),
                });
                trade_runtime.set_customization_readers(
                    Some(customization_reader.clone()),
                    Some(alert_reader.clone()),
                );
                trade_runtime
                    .set_customization_writers(Some(customization_reader), Some(alert_writer));
                trade_runtime.set_news_reader(Some(Arc::new(
                    jftrade_integration_futu::OpenDNewsReader::new(runtime.coordinator()),
                )));
                let prediction_reader =
                    Arc::new(OpenDPredictionMarketReader::new(runtime.coordinator()));
                trade_runtime.set_prediction_adapters(
                    Some(Arc::clone(&prediction_reader)
                        as Arc<
                            dyn jftrade_integration_futu::PredictionMarketReadPort,
                        >),
                    Some(Arc::clone(&prediction_reader)
                        as Arc<
                            dyn jftrade_integration_futu::PredictionMarketSubscriptionPort,
                        >),
                    Some(
                        prediction_reader
                            as Arc<dyn jftrade_integration_futu::PredictionComboQuotePort>,
                    ),
                );
                trade_runtime.set_market_microstructure(Some(Arc::new(
                    jftrade_integration_futu::OpenDMarketMicrostructureReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_corporate_actions_reader(Some(Arc::new(
                    jftrade_integration_futu::OpenDCorporateActionsReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_future_info(Some(Arc::new(
                    jftrade_integration_futu::OpenDFutureInfoReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_valuation_detail(Some(Arc::new(
                    jftrade_integration_futu::OpenDValuationDetailReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_institution_reader(Some(Arc::new(
                    jftrade_integration_futu::OpenDInstitutionReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_short_interest_reader(Some(Arc::new(
                    jftrade_integration_futu::OpenDShortInterestReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_technical_indicator_reader(Some(Arc::new(
                    jftrade_integration_futu::FutuTechnicalIndicatorReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_expirations(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionExpirationReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_chains(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionChainReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_option_screens(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionScreenReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_option_quotes(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionQuoteReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_option_volatility(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionVolatilityReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_exercise_probability(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionExerciseProbabilityReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_underlying_overview(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionUnderlyingOverviewReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_underlying_his_volatility(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionUnderlyingHisVolatilityReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_market_statistic(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionMarketStatisticReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_underlying_his_statistic(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionUnderlyingHisStatisticReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_strategy_spread(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionStrategySpreadReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_strategy(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionStrategyReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_option_strategy_analysis(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionStrategyAnalysisReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_underlying_rank(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionUnderlyingRankReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_contract_rank(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionContractRankReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_events(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionEventReader::new(runtime.coordinator()),
                )));
                trade_runtime.set_option_zero_dte_screener(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionZeroDteScreenerReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_earnings_screener(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionEarningsScreenerReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_zero_dte_contract(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionZeroDteContractReader::new(
                        runtime.coordinator(),
                    ),
                )));
                trade_runtime.set_option_seller_screener(Some(Arc::new(
                    jftrade_integration_futu::OpenDOptionSellerScreenerReader::new(
                        runtime.coordinator(),
                    ),
                )));
                (Some(runtime), Some(shared_router))
            }
            Err(error) => {
                eprintln!("Warning: OpenD provider runtime failed to connect: {error}");
                (None, Some(shared_router))
            }
        }
    } else {
        (None, market_data_router)
    };
    let market_data_runtime_recorder = if let Some(provider) = market_data_opend_provider.as_ref() {
        Some(
            provider
                .router()
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .runtime_recorder(),
        )
    } else if let Some(router) = market_data_router.as_ref() {
        Some(
            router
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .runtime_recorder(),
        )
    } else if let Some(coordinator) = market_data_opend.as_ref() {
        Some(
            coordinator
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .lifecycle()
                .recorder(),
        )
    } else {
        config.market_data_runtime_recorder.take()
    };
    if let Some(coordinator) = market_data_opend.as_ref()
        && let Ok(guard) = coordinator.lock()
        && let Ok(client) = OpenDTradeReadClient::from_coordinator(&guard)
    {
        let client = Arc::new(client);
        if config.product.trade_read_port.is_none() {
            config.product = config
                .product
                .clone()
                .with_trade_read_port(Some(client.clone() as Arc<dyn TradeReadPort>), None);
        }
        if config.product.trade_write_port.is_none() {
            config.product = config
                .product
                .clone()
                .with_trade_write_port(Some(client.clone() as Arc<dyn TradeWritePort>));
        }
        trade_runtime.set_writer(config.product.trade_write_port.clone());
    }
    if let Some(coordinator) = market_data_opend.as_ref() {
        let customization_reader = Arc::new(
            jftrade_integration_futu::FutuRemoteWatchlistReader::new(Arc::clone(coordinator)),
        );
        let alert_reader = Arc::new(jftrade_integration_futu::FutuAlertQuery {
            coordinator: Arc::clone(coordinator),
        });
        let alert_writer = Arc::new(jftrade_integration_futu::FutuAlertWrite {
            coordinator: Arc::clone(coordinator),
        });
        trade_runtime
            .set_customization_readers(Some(customization_reader.clone()), Some(alert_reader));
        trade_runtime.set_customization_writers(Some(customization_reader), Some(alert_writer));
        trade_runtime.set_future_info(Some(Arc::new(
            jftrade_integration_futu::OpenDFutureInfoReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_news_reader(Some(Arc::new(
            jftrade_integration_futu::OpenDNewsReader::new(Arc::clone(coordinator)),
        )));
        let prediction_reader = Arc::new(OpenDPredictionMarketReader::new(Arc::clone(coordinator)));
        trade_runtime.set_prediction_adapters(
            Some(Arc::clone(&prediction_reader)
                as Arc<
                    dyn jftrade_integration_futu::PredictionMarketReadPort,
                >),
            Some(Arc::clone(&prediction_reader)
                as Arc<
                    dyn jftrade_integration_futu::PredictionMarketSubscriptionPort,
                >),
            Some(prediction_reader as Arc<dyn jftrade_integration_futu::PredictionComboQuotePort>),
        );
        trade_runtime.set_market_microstructure(Some(Arc::new(
            jftrade_integration_futu::OpenDMarketMicrostructureReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_corporate_actions_reader(Some(Arc::new(
            jftrade_integration_futu::OpenDCorporateActionsReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_valuation_detail(Some(Arc::new(
            jftrade_integration_futu::OpenDValuationDetailReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_institution_reader(Some(Arc::new(
            jftrade_integration_futu::OpenDInstitutionReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_short_interest_reader(Some(Arc::new(
            jftrade_integration_futu::OpenDShortInterestReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_technical_indicator_reader(Some(Arc::new(
            jftrade_integration_futu::FutuTechnicalIndicatorReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_expirations(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionExpirationReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_chains(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionChainReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_screens(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionScreenReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_quotes(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionQuoteReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_volatility(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionVolatilityReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_exercise_probability(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionExerciseProbabilityReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_underlying_overview(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionUnderlyingOverviewReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_underlying_his_volatility(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionUnderlyingHisVolatilityReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_market_statistic(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionMarketStatisticReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_underlying_his_statistic(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionUnderlyingHisStatisticReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_strategy_spread(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionStrategySpreadReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_strategy(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionStrategyReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_strategy_analysis(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionStrategyAnalysisReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_underlying_rank(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionUnderlyingRankReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_contract_rank(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionContractRankReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_events(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionEventReader::new(Arc::clone(coordinator)),
        )));
        trade_runtime.set_option_zero_dte_screener(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionZeroDteScreenerReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_earnings_screener(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionEarningsScreenerReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_zero_dte_contract(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionZeroDteContractReader::new(Arc::clone(
                coordinator,
            )),
        )));
        trade_runtime.set_option_seller_screener(Some(Arc::new(
            jftrade_integration_futu::OpenDOptionSellerScreenerReader::new(Arc::clone(coordinator)),
        )));
    }
    if let Some(recorder) = market_data_runtime_recorder.as_ref() {
        config.product = config
            .product
            .with_market_data_runtime_status_port(recorder.clone());
    }
    if let Some(router) = market_data_router.as_ref() {
        config.product = config.product.with_market_data_router(Arc::clone(router));
    }
    let dynamic_opend: SharedOpenDProviderRuntime =
        Arc::new(Mutex::new(market_data_opend_provider));
    if market_data_router.is_some() {
        config.product = config.product.with_physical_subscription_port(Arc::new(
            DynamicOpenDPhysicalSubscriptionAdapter {
                runtime: Arc::clone(&dynamic_opend),
            },
        ));
    }
    if let Some(coordinator) = market_data_opend.as_ref()
        && config.product.physical_subscription_port.is_none()
    {
        config.product = config.product.with_physical_subscription_port(Arc::new(
            OpenDPhysicalSubscriptionAdapter {
                coordinator: Arc::clone(coordinator),
            },
        ));
    }
    // The production composition owns the strategy runtime status port that
    // is built from the production SQLite stores.  An injected registry is a
    // test/embedding seam and must never replace that owner in production;
    // retaining it in `config` is harmless and it is dropped with the config.
    if !config.product.is_production()
        && let Some(registry) = config.strategy_runtime_registry.take()
    {
        config.product = config.product.with_strategy_runtime_status_port(registry);
    }
    let state = ProductRuntimeState::configured(&config);
    let mut supervisor = if let Some(recorder) = config.shutdown_recorder.take() {
        ProductShutdownSupervisor::with_recorder(recorder)
    } else {
        ProductShutdownSupervisor::new()
    };
    supervisor.market_data_dynamic_opend = Some(Arc::clone(&dynamic_opend));
    supervisor.market_data_opend = market_data_opend.clone();

    if let (Some(coordinator), Some(task_config)) = (
        supervisor.market_data_opend.as_ref(),
        market_data_opend_task,
    ) {
        match OpenDSessionRuntime::start(Arc::clone(coordinator), task_config) {
            Ok(task) => supervisor.market_data_opend_runtime = Some(task),
            Err(error) => {
                eprintln!("Warning: OpenD session runtime failed to connect: {error}");
            }
        }
    }

    let mut healthy_pine_execution_config = None;
    let mut backtest_execution_verified = false;
    for worker in std::mem::take(&mut config.pine_workers) {
        let execution_config = (
            worker.spec.clone(),
            worker.process.bearer_token.clone(),
            worker.process.max_message_bytes,
            worker.connect_timeout,
            worker.request_timeout,
        );
        let result = start_pine_worker(worker).await;
        match result {
            Ok((process, _health)) => {
                supervisor.pine_workers.push(process);
                if healthy_pine_execution_config.is_none() {
                    healthy_pine_execution_config = Some(execution_config);
                }
            }
            Err(error) => {
                // PineTS is an optional external worker.  Keep the API alive
                // and expose backtest/strategy-Pine routes as 503 while the
                // worker is unavailable; internal store/schema failures still
                // fail before this point.
                eprintln!("Warning: PineTS worker unavailable: {error}");
            }
        }
    }

    // A healthy retained Pine worker is the only production composition that
    // can satisfy backtest execution.  Non-production test/embedding ports are
    // still accepted for rehearsal; production is always bound to the first
    // worker that passed the real gRPC readiness probe.  No worker means no
    // execution port, preserving the HTTP layer's 503 fail-closed result.
    if config.backtest_execution_port.is_none()
        && let Some((spec, bearer_token, max_message_bytes, connect_timeout, request_timeout)) =
            healthy_pine_execution_config
    {
        let mut execution =
            PineExecutionConfig::for_worker(&spec, bearer_token, connect_timeout, request_timeout);
        if let Some(max_message_bytes) = max_message_bytes {
            execution.max_message_bytes = max_message_bytes;
        }
        match GrpcPineExecutionPort::new(execution) {
            Ok(port) => {
                let port = Arc::new(port);
                // The same verified gRPC client backs both backtest execution
                // and the strategy-pine AnalyzeScript route.  Keeping one
                // client per worker avoids a second unprobed endpoint.
                config.product = config
                    .product
                    .with_strategy_pine_worker_port(Arc::clone(&port));
                config.backtest_execution_port =
                    Some(Arc::new(PineBacktestExecutionAdapter::new(port)));
                backtest_execution_verified = true;
            }
            Err(error) => {
                eprintln!("Warning: PineTS execution adapter unavailable: {error}");
            }
        }
    }
    if let Some(port) = config.backtest_execution_port.take() {
        config.product = if backtest_execution_verified {
            config.product.with_verified_backtest_execution_port(port)
        } else {
            config.product.with_backtest_execution_port(port)
        };
    }

    let helper_process = if let Some(helper) = config.marketdata_helper.take() {
        match start_marketdata_helper(helper).await {
            Ok((process, client, monitor)) => {
                config.product = config.product.with_market_data_helper(client);
                supervisor.helper_health = Some(Arc::clone(&monitor));
                Some(Arc::new(Mutex::new(Some(process))))
            }
            Err(error) => {
                // The helper is an optional external process.  Preserve the
                // API and let capability/readiness projection return 502/503
                // for helper-backed routes instead of failing the whole
                // product startup.
                eprintln!("Warning: market-data helper unavailable: {error}");
                None
            }
        }
    } else {
        None
    };
    supervisor.marketdata_helper = helper_process.clone();

    let settings_file = std::path::Path::new(config.product.settings_path());
    let configured_provider = if settings_file.exists() {
        let store = SettingsFileStore::open_read_only(config.product.settings_path())
            .map_err(|error| ProductRuntimeError::Settings(error.to_string()))?;
        store
            .load_active_market_data_provider()
            .map_err(|error| ProductRuntimeError::Settings(error.to_string()))?
            .map(|provider| {
                jftrade_settings::parse_market_data_provider(&provider)
                    .map_err(|error| ProductRuntimeError::Settings(error.to_string()))
            })
            .transpose()?
    } else {
        None
    };
    let dynamic_opend_ready = dynamic_opend
        .lock()
        .unwrap_or_else(|error| error.into_inner())
        .is_some()
        || market_data_opend.is_some();
    // OpenD may be configured as the authenticated Futu trade owner while a
    // helper-backed provider (yfinance/AKShare) owns market-data reads.  Keep
    // both runtimes composed; execution reconciliation resolves trade
    // readiness from `SharedTradeReadRuntime`, independently of this market
    // provider selection.
    let initial_provider =
        configured_provider.or_else(|| dynamic_opend_ready.then_some(MarketDataProvider::Futu));
    let settings_path = config.product.settings_path().to_owned();
    let dynamic_readiness = product_runtime_provider_activation::dynamic_provider_readiness(
        &helper_process,
        supervisor.helper_health.clone(),
        &dynamic_opend,
        &market_data_opend,
        &market_data_router,
    );
    let activation = product_runtime_provider_activation::provider_activation(
        &helper_process,
        supervisor.helper_health.clone(),
        &dynamic_opend,
        &market_data_router,
        &live_hub,
        &settings_path,
        Arc::clone(&trade_runtime),
    )?;
    let active_provider_state = Arc::new(
        ActiveProviderState::new(initial_provider)
            .with_dynamic_readiness(dynamic_readiness.clone())
            .with_activation(activation),
    );
    supervisor.active_provider_state = Some(Arc::clone(&active_provider_state));
    active_provider_state.set_readiness(
        config.product.market_data_helper.is_some() || supervisor.marketdata_helper.is_some(),
        dynamic_readiness().1,
        market_data_router.is_some(),
    );
    config.product = config
        .product
        .with_active_provider_state(active_provider_state);

    let provider_status = if (helper_configured && supervisor.marketdata_helper.is_none())
        || (opend_task_configured && supervisor.market_data_opend_runtime.is_none())
    {
        ProductionRuntimeStatus::Unavailable
    } else {
        production_provider_status(provider_configured, market_data_runtime_recorder.as_deref())
    };

    let opend_status = if opend_configured {
        if opend_task_configured && supervisor.market_data_opend_runtime.is_none() {
            ProductionRuntimeStatus::Unavailable
        } else {
            provider_status
        }
    } else {
        ProductionRuntimeStatus::Unavailable
    };

    let worker_status = if worker_configured {
        let helper_ok = if helper_configured {
            supervisor.marketdata_helper.is_some()
        } else {
            true
        };
        let pine_ok = if pine_workers_configured {
            !supervisor.pine_workers.is_empty()
        } else {
            true
        };
        if helper_ok && pine_ok {
            ProductionRuntimeStatus::Ready
        } else if supervisor.marketdata_helper.is_some() || !supervisor.pine_workers.is_empty() {
            ProductionRuntimeStatus::Degraded
        } else {
            ProductionRuntimeStatus::Unavailable
        }
    } else {
        ProductionRuntimeStatus::Unavailable
    };
    config.product = config.product.with_production_runtime_statuses(
        provider_status,
        opend_status,
        worker_status,
    );

    // Production ports, the 9 SQLite WriterLeases and every route adapter are
    // constructed inside `prepare_product_with_runtime_state`, while the HTTP
    // listener is not yet accepting traffic.  A fault between the two is
    // recovered by the supervisor's reverse-order rollback below, which must
    // release the provider/OpenD/helper/Pine resources and the port bundle so
    // every WriterLease can be re-acquired afterwards.
    let prepared =
        match prepare_product_with_runtime_state(config.product, Arc::clone(&state)).await {
            Ok(prepared) => prepared,
            Err(error) => {
                let _ = supervisor.execute_shutdown().await;
                return Err(error.into());
            }
        };

    #[cfg(test)]
    if config.inject_startup_failure {
        let ports = {
            let mut prepared = prepared;
            prepared.handle.take_production_ports()
        };
        supervisor.backtest_sync_workers = ports.as_ref().map(
            crate::product::product_production_ports::ProductionPortBundle::backtest_sync_workers,
        );
        supervisor.backtest_execution_workers = ports.as_ref().map(
            crate::product::product_production_ports::ProductionPortBundle::backtest_execution_workers,
        );
        supervisor.execution_reconciliation_worker = ports.as_ref().and_then(
            crate::product::product_production_ports::ProductionPortBundle::execution_reconciliation_worker,
        );
        supervisor.production_ports = ports;
        let _ = supervisor.execute_shutdown().await;
        return Err(ProductError::RouteRegistry(
            "injected startup fault after production lease acquisition, before HTTP exposure"
                .to_owned(),
        )
        .into());
    }

    match expose_prepared_product(prepared) {
        Ok(mut product) => {
            let ports = product.take_production_ports();
            supervisor.backtest_sync_workers = ports
                .as_ref()
                .map(crate::product::product_production_ports::ProductionPortBundle::backtest_sync_workers);
            supervisor.backtest_execution_workers = ports
                .as_ref()
                .map(crate::product::product_production_ports::ProductionPortBundle::backtest_execution_workers);
            supervisor.execution_reconciliation_worker = ports.as_ref().and_then(
                crate::product::product_production_ports::ProductionPortBundle::execution_reconciliation_worker,
            );
            supervisor.production_ports = ports;
            supervisor.product = Some(product);
        }
        Err(error) => {
            let _ = supervisor.execute_shutdown().await;
            return Err(error.into());
        }
    }
    Ok(ProductRuntimeHandle {
        supervisor,
        market_data_router,
    })
}

fn production_provider_status(
    configured: bool,
    recorder: Option<&MarketDataRuntimeRecorder>,
) -> ProductionRuntimeStatus {
    let Some(state) = recorder.map(MarketDataRuntimeRecorder::snapshot) else {
        return if configured {
            ProductionRuntimeStatus::Degraded
        } else {
            ProductionRuntimeStatus::Unavailable
        };
    };
    if state.closed {
        ProductionRuntimeStatus::Failed
    } else if state.connected {
        ProductionRuntimeStatus::Ready
    } else {
        ProductionRuntimeStatus::Degraded
    }
}
