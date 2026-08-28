#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductStartupRecord {
    pub event: &'static str,
    pub address: SocketAddr,
    pub owner: &'static str,
    pub owned_routes: usize,
    pub ready_routes: usize,
    pub external_unavailable_routes: usize,
    pub protocol_version: &'static str,
    pub route_profile: &'static str,
    pub route_profile_digest: String,
    pub capabilities: Vec<String>,
    pub resource_sha256: String,
    pub runtime_readiness: &'static str,
    pub database_lease_status: &'static str,
    pub provider_status: &'static str,
    pub opend_status: &'static str,
    pub worker_status: &'static str,
    pub websocket_status: &'static str,
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
    server: Option<ProductServerOwner>,
    calendar_manager: Option<Arc<CalendarManager>>,
    live_hub: Arc<LiveHub>,
    active_provider_state: Option<Arc<ActiveProviderState>>,
    pub(crate) production_ports:
        Option<crate::product::product_production_ports::ProductionPortBundle>,
}

struct ProductServerOwner {
    shutdown_tx: Option<oneshot::Sender<()>>,
    thread: Option<std::thread::JoinHandle<Result<(), std::io::Error>>>,
}

impl ProductServerOwner {
    fn start(listener: StdTcpListener, router: axum::Router) -> Result<Self, ProductError> {
        listener.set_nonblocking(true).map_err(ProductError::Bind)?;
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(ProductError::Bind)?;
        let (shutdown_tx, shutdown_rx) = oneshot::channel();
        let thread = std::thread::Builder::new()
            .name("jftrade-product-http".to_owned())
            .spawn(move || {
                runtime.block_on(async move {
                    let listener = tokio::net::TcpListener::from_std(listener)?;
                    axum::serve(listener, router)
                        .with_graceful_shutdown(async move {
                            let _ = shutdown_rx.await;
                        })
                        .await
                })
            })
            .map_err(ProductError::Bind)?;
        Ok(Self {
            shutdown_tx: Some(shutdown_tx),
            thread: Some(thread),
        })
    }

    fn shutdown_blocking(&mut self) -> Result<(), ProductError> {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        let Some(thread) = self.thread.take() else {
            return Ok(());
        };
        thread
            .join()
            .map_err(|_| ProductError::ServerThreadPanicked)??;
        Ok(())
    }

    async fn shutdown(mut self) -> Result<(), ProductError> {
        tokio::task::spawn_blocking(move || self.shutdown_blocking())
            .await
            .map_err(ProductError::Join)?
    }
}

impl Drop for ProductServerOwner {
    fn drop(&mut self) {
        let _ = self.shutdown_blocking();
    }
}

#[derive(Debug)]
struct ProductLiveMarketDataStatus {
    runtime: Option<Arc<dyn MarketDataRuntimeStatusPort>>,
}

impl jftrade_api::LiveMarketDataStatusPort for ProductLiveMarketDataStatus {
    fn snapshot(&self) -> jftrade_api::LiveMarketDataStatus {
        jftrade_api::LiveMarketDataStatus {
            connected: self
                .runtime
                .as_ref()
                .is_some_and(|runtime| runtime.snapshot().connected),
        }
    }
}

impl ProductHandle {
    pub const fn startup_record(&self) -> &ProductStartupRecord {
        &self.startup_record
    }

    pub fn live_hub(&self) -> Arc<LiveHub> {
        Arc::clone(&self.live_hub)
    }

    pub(crate) fn take_production_ports(
        &mut self,
    ) -> Option<crate::product::product_production_ports::ProductionPortBundle> {
        self.production_ports.take()
    }

    pub(crate) fn sync_terminate(&mut self) {
        // Shutdown must first stop accepting new websocket connections; the
        // HTTP drain below then completes without serving fresh upgrades.
        self.live_hub.begin_shutdown();
        if let Some(state) = &self.active_provider_state {
            state.begin_shutdown();
        }
        if let Some(mut server) = self.server.take() {
            let _ = server.shutdown_blocking();
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            let _ = manager.close();
        }
        drop(self.production_ports.take());
    }

    pub async fn shutdown(mut self) -> Result<(), ProductError> {
        self.live_hub.begin_shutdown();
        if let Some(state) = &self.active_provider_state {
            state.begin_shutdown();
        }
        if let Some(server) = self.server.take() {
            server.shutdown().await?;
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            manager.close().map_err(ProductError::Calendar)?;
        }
        drop(self.production_ports.take());
        Ok(())
    }
}

impl Drop for ProductHandle {
    fn drop(&mut self) {
        self.live_hub.begin_shutdown();
        if let Some(state) = &self.active_provider_state {
            state.begin_shutdown();
        }
        if let Some(mut server) = self.server.take() {
            let _ = server.shutdown_blocking();
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            let _ = manager.close();
        }
        drop(self.production_ports.take());
    }
}

pub async fn start_product(config: ProductConfig) -> Result<ProductHandle, ProductError> {
    let runtime = ProductRuntimeState::product_only(&config);
    start_product_with_runtime_state(config, runtime).await
}

/// Fully prepared but not yet exposed product server: production ports are
/// constructed and held, every route is bound, yet no socket accepts traffic.
/// Exposing happens in [`expose_prepared_product`], which starts the HTTP
/// listener and marks the live hub `serving`.
pub(crate) struct PreparedProduct {
    pub(crate) handle: ProductHandle,
    listener: StdTcpListener,
    router: axum::Router,
    live_hub: Arc<LiveHub>,
    production: bool,
    production_runtime_core_ready: bool,
}

pub(crate) async fn start_product_with_runtime_state(
    config: ProductConfig,
    runtime: Arc<ProductRuntimeState>,
) -> Result<ProductHandle, ProductError> {
    let prepared = prepare_product_with_runtime_state(config, runtime).await?;
    expose_prepared_product(prepared)
}

/// Move the prepared product into actual service: start the HTTP server
/// thread, then mark the live hub `serving` so startup readiness and the
/// websocket status derive from the real hub lifecycle.
pub(crate) fn expose_prepared_product(
    mut prepared: PreparedProduct,
) -> Result<ProductHandle, ProductError> {
    let server = ProductServerOwner::start(prepared.listener, prepared.router)?;
    prepared.live_hub.mark_serving();
    let lifecycle = prepared.live_hub.lifecycle();
    let websocket_ready = lifecycle == LiveHubLifecycle::Serving;
    prepared.handle.startup_record.websocket_status = lifecycle.as_str();
    prepared.handle.startup_record.runtime_readiness = if !prepared.production {
        "rehearsal"
    } else if prepared.production_runtime_core_ready && websocket_ready {
        "ready"
    } else {
        "degraded"
    };
    prepared.handle.server = Some(server);
    Ok(prepared.handle)
}

pub(crate) async fn prepare_product_with_runtime_state(
    mut config: ProductConfig,
    runtime: Arc<ProductRuntimeState>,
) -> Result<PreparedProduct, ProductError> {
    if config.production {
        product_data_management::initialize_production_databases(config.settings_path())
            .map_err(ProductError::Storage)?;
    }
    let data_management = product_data_management::overview_service(config.settings_path());
    let cleanup_preview = product_data_management::cleanup_preview_service(config.settings_path());
    let maintenance = product_data_management::maintenance_service_with_profile(
        config.settings_path(),
        Arc::clone(&cleanup_preview),
        if config.production {
            PRODUCT_PRODUCTION_ROUTE_PROFILE
        } else {
            PRODUCT_TEST_CUTOVER_ROUTE_PROFILE
        },
    );
    let settings_store = Arc::new(
        if config.capabilities.requires_writable_settings() {
            SettingsFileStore::open(config.settings_path.clone())
        } else {
            SettingsFileStore::open_read_only(config.settings_path.clone())
        }
        .map_err(ProductError::Settings)?,
    );
    let metrics = Arc::new(TransportMetrics::default());
    let interface_settings = settings_store
        .load_interface_settings()
        .map_err(ProductError::Settings)?;
    let live_connections = Arc::new(LiveConnectionMetrics::new(
        normalize_live_websocket_connection_limit(interface_settings.as_ref()),
    ));
    let real_trade_control = RealTradeControlReader::new(config.real_trade_control_path.clone());
    let security_service = SecuritySettingsService::new(settings_store.clone());
    let initial_active_provider = settings_store
        .load_active_market_data_provider()
        .map_err(ProductError::Settings)?
        .and_then(|s| jftrade_settings::parse_market_data_provider(&s).ok());
    let active_provider_state = config
        .active_provider_state
        .clone()
        .unwrap_or_else(|| Arc::new(ActiveProviderState::new(initial_active_provider)));
    config.active_provider_state = Some(Arc::clone(&active_provider_state));
    let production_ports = config
        .production
        .then(|| product_production_ports::production_ports(&config, &security_service))
        .transpose()?;
    let production_calendar_manager = production_ports
        .as_ref()
        .map(|ports| ports.calendar_manager.clone());
    let calendar_manager = config
        .calendar_manager
        .clone()
        .or(production_calendar_manager);
    let production_registry = production_ports
        .as_ref()
        .map(product_production_route_registry::ProductionRouteRegistry::bind)
        .transpose()?;
    let routes = if let Some(registry) = production_registry.as_ref() {
        registry.catalog().clone()
    } else {
        product_routes(&config.capabilities, configured_route_ports(&config))?
    };
    let route_count = production_registry
        .as_ref()
        .map_or_else(|| routes.routes().len(), |registry| registry.bindings().len());
    let ready_route_count = production_registry.as_ref().map_or(route_count, |registry| {
        registry
            .bindings()
            .iter()
            .filter(|binding| {
                binding.adapter_binding
                    == product_production_ports::ProductionAdapterBinding::Ready
            })
            .count()
    });
    let external_unavailable_route_count = production_registry.as_ref().map_or(0, |registry| {
        registry
            .bindings()
            .iter()
            .filter(|binding| {
                binding.adapter_binding
                    == product_production_ports::ProductionAdapterBinding::ExternalUnavailable
            })
            .count()
    });
    let route_capabilities = routes
        .routes()
        .iter()
        .map(|route| format!("{} {}", route.method, route.path))
        .collect::<Vec<_>>();
    let route_profile_digest = production_registry.as_ref().map_or_else(
        || route_profile_digest(&route_capabilities),
        |registry| registry.digest().to_owned(),
    );
    let resource_sha256 = current_executable_sha256()?;
    let owner = if config.production {
        "rust"
    } else if config.capabilities.is_empty() {
        "rust-read-only-shadow"
    } else {
        "rust-cutover"
    };
    let route_profile = if config.production {
        PRODUCT_PRODUCTION_ROUTE_PROFILE
    } else if config.capabilities.is_empty() {
        PRODUCT_READ_ONLY_ROUTE_PROFILE
    } else {
        PRODUCT_TEST_CUTOVER_ROUTE_PROFILE
    };
    let mut access_policy = config.access.clone();
    if access_policy.session_validator.is_none()
        && let Some(ports) = production_ports.as_ref()
    {
        access_policy =
            access_policy.with_session_validator(ports.auth_session_validator.clone());
    }
    if let Some(manager) = &calendar_manager {
        manager.start().map_err(ProductError::Calendar)?;
    }
    let listener = StdTcpListener::bind(config.bind_address).map_err(ProductError::Bind)?;
    let address = listener.local_addr().map_err(ProductError::LocalAddress)?;
    let production_runtime_core_ready = production_ports.as_ref().is_some_and(|ports| {
        ports.provider_status == "ready"
            && ports.opend_status == "ready"
            && ports.worker_status == "ready"
    });
    active_provider_state.set_readiness(
        config.market_data_helper.is_some(),
        config.market_data_runtime_status_port.is_some(),
        config.market_data_router.is_some(),
    );
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
            security: security_service,
            market_data_provider: if let Some(state) = config.active_provider_state.as_ref() {
                MarketDataProviderSettingsService::new(settings_store.clone())
                    .with_runtime(Arc::clone(state) as Arc<dyn jftrade_settings::MarketDataProviderRuntimePort>)
            } else {
                MarketDataProviderSettingsService::new(settings_store.clone())
            },
            backtest_market_data_provider: BacktestMarketDataProviderSettingsService::new(
                settings_store.clone(),
            ),
            mcp_server: McpServerSettingsService::new(settings_store.clone()),
            exchange_calendars: ExchangeCalendarSettingsService::new(settings_store),
            data_management,
            cleanup_preview,
            maintenance,
        },
        Arc::clone(&metrics),
        Arc::clone(&live_connections),
        runtime,
        real_trade_control,
        ProductOptionalPorts {
            notification: config.notification_port.clone(),
            calendar_manager: calendar_manager.clone(),
            watchlist_membership_snapshot: config
                .watchlist_membership_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.watchlist_memberships.clone())),
            watchlist_read_snapshot: config
                .watchlist_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.watchlist.clone())),
            portfolio_snapshot: config
                .portfolio_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.portfolio.clone())),
            research_read_snapshot: config
                .research_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.research_read.clone())),
            research_preset_read_snapshot: config
                .research_preset_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.research_preset_read.clone())),
            execution_read_snapshot: config
                .execution_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.execution_read.clone())),
            market_data_provider_read_snapshot: config
                .market_data_provider_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.provider.clone())),
            market_data_catalog_read_snapshot: config
                .market_data_catalog_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.catalog.clone())),
            market_data_derivative_read_snapshot: config
                .market_data_derivative_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_derivative.clone())),
            market_data_options_read_snapshot: config
                .market_data_options_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_options.clone())),
            market_data_news_actions_read_snapshot: config
                .market_data_news_actions_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_news_actions.clone())),
            market_data_news_search_read_snapshot: config
                .market_data_news_search_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_news_search.clone())),
            adk_read_snapshot: config
                .adk_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.adk_read.clone())),
            market_data_quote_read_snapshot: config
                .market_data_quote_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_quote.clone())),
            market_data_prediction_read_snapshot: config
                .market_data_prediction_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_prediction.clone())),
            market_data_runtime_status: config.market_data_runtime_status_port.clone(),
            broker_read_snapshot: config
                .broker_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.broker.clone())),
            system_read_snapshot: config
                .system_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.system_read.clone())),
            remote_watchlist_snapshot: config
                .remote_watchlist_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.remote_watchlist.clone())),
            remote_watchlist_write: config
                .remote_watchlist_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.remote_watchlist_write.clone())),
            watchlist_write: config
                .watchlist_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.watchlist_write.clone())),
            plugin_uninstall_guidance_snapshot: config
                .plugin_uninstall_guidance_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.plugin_guidance.clone())),
            plugin_snapshot: config
                .plugin_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.plugins.clone())),
            plugin_write: config
                .plugin_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.plugin_write.clone())),
            research_preset_write: config
                .research_preset_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.research_preset_write.clone())),
            strategy_definition_write: config
                .strategy_definition_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_definition_write.clone())),
            market_data_provider_actions: config
                .market_data_provider_actions_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_provider_actions.clone())),
            adk_chat_stream: config
                .adk_chat_stream_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.adk_chat_stream.clone())),
            adk_mutation: config
                .adk_mutation_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.adk_mutation.clone())),
            alert_snapshot: config
                .alert_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.alert_snapshot.clone())),
            alert_write: config
                .alert_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.alert_write.clone())),
            strategy_definition_snapshot: config
                .strategy_definition_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_definition.clone())),
            strategy_pine_analyze_snapshot: config
                .strategy_pine_analyze_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_pine_analyze.clone())),
            backtest_read_snapshot: config
                .backtest_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.backtest_read.clone())),
            backtest_sync_read_snapshot: config
                .backtest_sync_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.backtest_sync.clone())),
            backtests_write: config
                .backtests_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.backtests_write.clone())),
            strategy_read_snapshot: config
                .strategy_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_read.clone())),
            strategy_runtime_status: config
                .strategy_runtime_status_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_runtime_status.clone())),
            strategy_runtime_write: config
                .strategy_runtime_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.strategy_runtime_write.clone())),
            auth_session_snapshot: config
                .auth_session_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.auth_session.clone())),
            auth_session_write: config
                .auth_session_write_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.auth_session_write.clone())),
            auth_session_invalidation: production_ports
                .as_ref()
                .map(|ports| ports.auth_session_invalidation.clone()),
            stage9_write_ports: ProductStage9WritePorts {
                execution: config
                    .stage9_write_ports
                    .execution
                    .clone()
                    .or_else(|| production_ports.as_ref().map(|ports| ports.execution_write.clone())),
                brokers: config
                    .stage9_write_ports
                    .brokers
                    .clone()
                    .or_else(|| production_ports.as_ref().map(|ports| ports.brokers_write.clone())),
                system: config
                    .stage9_write_ports
                    .system
                    .clone()
                    .or_else(|| production_ports.as_ref().map(|ports| ports.system_write.clone())),
                market_data_subscription_mutation: config
                    .stage9_write_ports
                    .market_data_subscription_mutation
                    .clone()
                    .or_else(|| production_ports.as_ref().map(|ports| ports.market_data_subscription_mutation.clone())),
                research_screen: config
                    .stage9_write_ports
                    .research_screen
                    .clone()
                    .or_else(|| production_ports.as_ref().map(|ports| ports.research_screen_write.clone())),
            },
        },
    ));
    let live_hub = config
        .live_hub
        .clone()
        .unwrap_or_else(|| Arc::new(LiveHub::default()));
    if let Some(router) = &config.market_data_router {
        live_hub.set_demand_listener(Arc::new(RouterDemandListener {
            router: Arc::clone(router),
        }));
    }
    let mut state = ApiState::new(routes, access_policy, port).with_live_hub(Arc::clone(&live_hub));
    state.metrics = metrics;
    state.live_connections = live_connections;
    if config.production || config.market_data_runtime_status_port.is_some() {
        state = state.with_live_market_data_status(Arc::new(ProductLiveMarketDataStatus {
            runtime: config.market_data_runtime_status_port.clone(),
        }));
    }
    let router = build_router(state);
    let startup_record = ProductStartupRecord {
        event: "ready",
        address,
        owner,
        owned_routes: route_count,
        ready_routes: ready_route_count,
        external_unavailable_routes: external_unavailable_route_count,
        protocol_version: PRODUCT_REHEARSAL_PROTOCOL_VERSION,
        route_profile,
        route_profile_digest,
        capabilities: route_capabilities,
        resource_sha256,
        runtime_readiness: if !config.production {
            "rehearsal"
        } else if production_runtime_core_ready {
            "ready"
        } else {
            "degraded"
        },
        database_lease_status: production_ports
            .as_ref()
            .map_or("none", |ports| ports.database_lease_status),
        provider_status: production_ports
            .as_ref()
            .map_or("not-started", |ports| ports.provider_status),
        opend_status: production_ports
            .as_ref()
            .map_or("not-started", |ports| ports.opend_status),
        worker_status: production_ports
            .as_ref()
            .map_or("not-started", |ports| ports.worker_status),
        websocket_status: live_hub.lifecycle().as_str(),
    };
    Ok(PreparedProduct {
        handle: ProductHandle {
            startup_record,
            server: None,
            calendar_manager,
            live_hub: Arc::clone(&live_hub),
            active_provider_state: config.active_provider_state,
            production_ports,
        },
        listener,
        router,
        live_hub,
        production: config.production,
        production_runtime_core_ready,
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

#[derive(Debug)]
struct RouterDemandListener {
    router: Arc<Mutex<jftrade_marketdata::ProviderRouter>>,
}

impl jftrade_api::LiveDemandListener for RouterDemandListener {
    fn on_subscription_change(
        &self,
        connection_id: u64,
        provider_broker_id: &str,
        instruments: &[String],
    ) {
        let consumer_id = format!("ws-client-{}", connection_id);
        let mut router = self.router.lock().unwrap_or_else(|e| e.into_inner());

        let provider_id = provider_broker_id.trim();
        let is_futu = provider_id.is_empty() || provider_id.eq_ignore_ascii_case("futu");
        if !is_futu {
            router.release_demand_consumer(&consumer_id);
            return;
        }

        let refs = instruments
            .iter()
            .map(|inst| {
                let upper = inst.trim().to_ascii_uppercase();
                let (market, symbol) = if upper.contains('.') {
                    let mut parts = upper.splitn(2, '.');
                    let m = parts.next().unwrap_or("US");
                    let s = parts.next().unwrap_or(&upper);
                    (m.to_owned(), s.to_owned())
                } else {
                    ("US".to_owned(), upper)
                };
                jftrade_marketdata::InstrumentRef {
                    channel: "SNAPSHOT".to_owned(),
                    market,
                    symbol,
                    interval: None,
                }
            })
            .collect::<Vec<_>>();

        if refs.is_empty() {
            router.release_demand_consumer(&consumer_id);
        } else {
            let now_ms = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .ok()
                .and_then(|d| i64::try_from(d.as_millis()).ok())
                .unwrap_or_default();
            let _ = router.replace_demand(&consumer_id, refs, false, now_ms);
        }
    }

    fn on_disconnect(&self, connection_id: u64) {
        let consumer_id = format!("ws-client-{}", connection_id);
        let mut router = self.router.lock().unwrap_or_else(|e| e.into_inner());
        router.release_demand_consumer(&consumer_id);
    }
}
