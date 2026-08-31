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
    web_runtime: Option<Arc<ProductWebServerRuntime>>,
    pub(crate) mcp_server_runtime: Option<Arc<product_mcp_server::ProductMcpServerRuntime>>,
    calendar_manager: Option<Arc<CalendarManager>>,
    live_hub: Arc<LiveHub>,
    active_provider_state: Option<Arc<ActiveProviderState>>,
    pub(crate) production_ports:
        Option<crate::product::product_production_ports::ProductionPortBundle>,
}

#[derive(Debug)]
struct ProductServerOwner {
    shutdown_tx: Option<oneshot::Sender<()>>,
    thread: Option<std::thread::JoinHandle<Result<(), std::io::Error>>>,
}

impl ProductServerOwner {
    fn start(listener: StdTcpListener, router: axum::Router) -> Result<Self, ProductError> {
        Self::start_named(listener, router, "jftrade-product-http")
    }

    fn start_named(
        listener: StdTcpListener,
        router: axum::Router,
        thread_name: &'static str,
    ) -> Result<Self, ProductError> {
        listener.set_nonblocking(true).map_err(ProductError::Bind)?;
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(ProductError::Bind)?;
        let (shutdown_tx, shutdown_rx) = oneshot::channel();
        let thread = std::thread::Builder::new()
            .name(thread_name.to_owned())
            .spawn(move || {
                runtime.block_on(async move {
                    let listener = tokio::net::TcpListener::from_std(listener)?;
                    axum::serve(
                        listener,
                        router.into_make_service_with_connect_info::<SocketAddr>(),
                    )
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

/// Listener-backed security runtime for the optional browser Web surface.
///
/// The API sidecar keeps its existing bind and bearer policy. This runtime
/// owns a second listener whose bind is derived from persisted security
/// settings (`127.0.0.1` for local access and `0.0.0.0` for public access).
/// Reconfiguration binds a new port before retiring the old one; a host-only
/// change briefly closes and restores the previous listener if the new bind
/// fails. The router is cleared during shutdown to break the intentional
/// ProductApi -> SecuritySettingsService -> runtime -> router reference cycle.
#[derive(Clone, Debug)]
struct ProductWebServerRuntime {
    inner: Arc<Mutex<ProductWebServerState>>,
}

#[derive(Debug, Default)]
struct ProductWebServerState {
    router: Option<axum::Router>,
    server: Option<ProductServerOwner>,
    bind: Option<String>,
}

impl ProductWebServerRuntime {
    fn new() -> Arc<Self> {
        Arc::new(Self {
            inner: Arc::new(Mutex::new(ProductWebServerState::default())),
        })
    }

    fn install_router(&self, router: axum::Router) {
        self.inner
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .router = Some(router);
    }

    fn shutdown_blocking(&self) -> Result<(), String> {
        let mut state = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        state.bind = None;
        state.router = None;
        let Some(mut server) = state.server.take() else {
            return Ok(());
        };
        server
            .shutdown_blocking()
            .map_err(|error| error.to_string())
    }

    fn desired_bind(record: &SecuritySettingsRecord) -> Option<String> {
        if !record.web_access_enabled() {
            return None;
        }
        let host = if record.public_access_enabled() {
            "0.0.0.0"
        } else {
            "127.0.0.1"
        };
        Some(format!("{host}:{}", record.web_port()))
    }

    fn start_locked(
        state: &mut ProductWebServerState,
        bind: &str,
    ) -> Result<ProductServerOwner, String> {
        let router = state
            .router
            .clone()
            .ok_or_else(|| "Web access router is not installed".to_owned())?;
        let listener = StdTcpListener::bind(bind)
            .map_err(|error| format!("Web access port conflict on {bind}: {error}"))?;
        ProductServerOwner::start_named(listener, router, "jftrade-product-web")
            .map_err(|error| error.to_string())
    }

    fn allows_origin(&self, origin: &str) -> bool {
        let Some((scheme, authority)) = origin.split_once("://") else {
            return false;
        };
        if !scheme.eq_ignore_ascii_case("http") {
            return false;
        }
        let state = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        let Some(bind) = state.bind.as_deref() else {
            return false;
        };
        let Some((bind_host, bind_port)) = bind.rsplit_once(':') else {
            return false;
        };
        let Some((origin_host, origin_port)) = authority.rsplit_once(':') else {
            return false;
        };
        if origin_port != bind_port {
            return false;
        }
        let origin_host = origin_host.trim_matches(['[', ']']);
        let bind_host = bind_host.trim_matches(['[', ']']);
        if bind_host == "0.0.0.0" {
            // Public Web access may be reached through the machine's LAN
            // address. Restrict the dynamic grant to this listener's port;
            // fixed Tauri origins remain governed by `allowed_origins`.
            return !origin_host.is_empty();
        }
        bind_host == "127.0.0.1" && matches!(origin_host, "127.0.0.1" | "localhost")
    }
}

impl jftrade_api::AccessOriginProvider for ProductWebServerRuntime {
    fn allows_origin(&self, origin: &str) -> bool {
        Self::allows_origin(self, origin)
    }
}

impl jftrade_settings::SecurityRuntimePort for ProductWebServerRuntime {
    fn apply(&self, record: &SecuritySettingsRecord) -> Result<(), String> {
        let desired = Self::desired_bind(record);
        let mut state = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        let current = state.bind.clone();
        if current == desired.as_ref().map(ToString::to_string)
            && (desired.is_none() || state.server.is_some())
        {
            return Ok(());
        }
        let Some(desired_bind) = desired else {
            state.bind = None;
            if let Some(mut server) = state.server.take() {
                server
                    .shutdown_blocking()
                    .map_err(|error| error.to_string())?;
            }
            return Ok(());
        };

        // Port changes can be staged safely while the old listener remains
        // active. This is the common rebind path and preserves availability
        // if the new bind fails.
        let same_port = current
            .as_deref()
            .and_then(|value| value.rsplit_once(':').map(|(_, port)| port))
            == desired_bind.rsplit_once(':').map(|(_, port)| port);
        if state.server.is_some() && same_port {
            let old_bind = current.clone().unwrap_or_default();
            let old_server = state.server.take();
            state.bind = None;
            if let Some(mut server) = old_server {
                server
                    .shutdown_blocking()
                    .map_err(|error| error.to_string())?;
            }
            match Self::start_locked(&mut state, &desired_bind) {
                Ok(server) => {
                    state.bind = Some(desired_bind);
                    state.server = Some(server);
                    Ok(())
                }
                Err(error) => {
                    if old_bind.is_empty() {
                        return Err(error);
                    }
                    match Self::start_locked(&mut state, &old_bind) {
                        Ok(server) => {
                            state.bind = Some(old_bind);
                            state.server = Some(server);
                            Err(error)
                        }
                        Err(restore) => Err(format!(
                            "{error}; restoring {old_bind} also failed: {restore}"
                        )),
                    }
                }
            }
        } else {
            let server = Self::start_locked(&mut state, &desired_bind)?;
            let old_server = state.server.replace(server);
            state.bind = Some(desired_bind);
            if let Some(mut old_server) = old_server {
                old_server
                    .shutdown_blocking()
                    .map_err(|error| error.to_string())?;
            }
            Ok(())
        }
    }

    fn status(&self, record: &SecuritySettingsRecord) -> Result<bool, String> {
        let desired = Self::desired_bind(record);
        let state = self.inner.lock().unwrap_or_else(|error| error.into_inner());
        Ok(desired.is_some()
            && state.server.is_some()
            && state.bind.as_deref() == desired.as_deref())
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

    /// Stop runtime tasks that are owned by the production port bundle before
    /// its stores (and their WriterLeases) are released.  This mirrors the
    /// shutdown supervisor's reverse-of-construction order for the direct
    /// `start_product` entrypoint, where the bundle remains on this handle.
    async fn shutdown_production_runtime(&self) {
        let Some(ports) = self.production_ports.as_ref() else {
            return;
        };
        ports.shutdown_strategy_runtime();
        ports.shutdown_adk_runtime();
        if let Some(worker) = ports.execution_reconciliation_worker() {
            worker.shutdown().await;
        }
    }

    fn terminate_production_runtime(&self) {
        let Some(ports) = self.production_ports.as_ref() else {
            return;
        };
        ports.shutdown_strategy_runtime();
        ports.shutdown_adk_runtime();
        if let Some(worker) = ports.execution_reconciliation_worker() {
            worker.terminate();
        }
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
        if let Some(runtime) = self.web_runtime.take() {
            let _ = runtime.shutdown_blocking();
        }
        if let Some(runtime) = self.mcp_server_runtime.take() {
            let _ = runtime.shutdown_blocking();
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            let _ = manager.close();
        }
        self.terminate_production_runtime();
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
        if let Some(runtime) = self.web_runtime.take() {
            runtime
                .shutdown_blocking()
                .map_err(|message| ProductError::SecurityRuntime { message })?;
        }
        if let Some(runtime) = self.mcp_server_runtime.take() {
            runtime
                .shutdown_blocking()
                .map_err(|message| ProductError::SecurityRuntime {
                    message: format!("MCP server: {message}"),
                })?;
        }
        self.live_hub.mark_stopped();
        if let Some(manager) = self.calendar_manager.take() {
            manager.close().map_err(ProductError::Calendar)?;
        }
        self.shutdown_production_runtime().await;
        drop(self.production_ports.take());
        Ok(())
    }
}

impl Drop for ProductHandle {
    fn drop(&mut self) {
        self.sync_terminate();
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
    let web_runtime = config.production.then(ProductWebServerRuntime::new);
    let security_service = web_runtime.as_ref().map_or_else(
        || SecuritySettingsService::new(settings_store.clone()),
        |runtime| {
            SecuritySettingsService::with_ports(
                settings_store.clone(),
                Some(Arc::clone(runtime) as Arc<dyn SecurityRuntimePort>),
                Arc::new(jftrade_settings::SystemSecurityPasswords),
            )
        },
    );
    let security_service_for_runtime = security_service.clone();
    // Mirror Go's one-time settings upgrade in the production owner. Legacy
    // files inherit activeMarketDataProvider exactly once, then keep the
    // module-specific value independent from live-provider switches.
    if config.production {
        settings_store
            .ensure_backtest_market_data_provider()
            .map_err(ProductError::Settings)?;
    }
    let initial_active_provider = settings_store
        .load_active_market_data_provider()
        .map_err(ProductError::Settings)?
        .map(|value| {
            jftrade_settings::parse_market_data_provider(&value).map_err(|error| {
                ProductError::Settings(jftrade_settings::SettingsStoreError::new(format!(
                    "invalid active market-data provider: {error}"
                )))
            })
        })
        .transpose()?;
    let active_provider_state = config
        .active_provider_state
        .clone()
        .unwrap_or_else(|| Arc::new(ActiveProviderState::new(initial_active_provider)));
    config.active_provider_state = Some(Arc::clone(&active_provider_state));
    let initial_backtest_provider = settings_store
        .load_backtest_market_data_provider()
        .map_err(ProductError::Settings)?
        .as_deref()
        .map(jftrade_settings::parse_market_data_provider)
        .transpose()
        .map_err(|error| {
            ProductError::Storage(format!(
                "invalid backtest market-data provider settings: {error}"
            ))
        })?
        .unwrap_or_default();
    let backtest_market_data_provider_state = config
        .backtest_market_data_provider_state
        .clone()
        .unwrap_or_else(|| {
            Arc::new(crate::product::BacktestMarketDataProviderState::new(
                initial_backtest_provider,
            ))
        });
    config.backtest_market_data_provider_state =
        Some(Arc::clone(&backtest_market_data_provider_state));
    if config.production && !config.backtest_execution_port_verified {
        // A production backtest adapter is trusted only when the runtime
        // installed it after a successful Pine readiness probe.  Direct
        // ProductConfig injection remains a rehearsal seam and is ignored.
        config.backtest_execution_port = None;
    }
    let production_ports = config
        .production
        .then(|| product_production_ports::production_ports(&config, &security_service))
        .transpose()?;
    let mcp_server_runtime = production_ports.as_ref().map(|ports| {
        product_mcp_server::ProductMcpServerRuntime::from_production_ports(Arc::new(ports.clone()))
    });
    let mut mcp_runtime_ready = true;
    // Reconcile persisted MCP settings before exposing the product API. A
    // bind conflict keeps the API alive in a truthful degraded state; the
    // settings endpoint then reports the listener error and allows a retry.
    if let Some(runtime) = mcp_server_runtime.as_ref()
        && let Some(record) = settings_store
            .load_mcp_server_record()
            .map_err(ProductError::Settings)?
        && let Err(error) =
            jftrade_settings::McpServerRuntimePort::apply(runtime.as_ref(), &record)
    {
        mcp_runtime_ready = false;
        tracing::error!(
            error = %error,
            "persisted MCP listener settings could not be applied; product runtime is degraded"
        );
        }
    // Production composition is fenced to the concrete bundle built above.
    // This prevents an embedding/test port supplied on `ProductConfig` from
    // taking precedence over a production adapter in `ProductOptionalPorts`.
    config.fence_production_route_ports();
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
    // Keep a shared view of the concrete production composition in the API.
    // The ProductHandle retains its own value for teardown, while dispatch
    // consults this clone's shared runtime state on every request.
    let production_ports_for_api = production_ports
        .as_ref()
        .map(|ports| Arc::new(ports.clone()));
    let routes = if let Some(registry) = production_registry.as_ref() {
        registry.catalog().clone()
    } else {
        product_routes(&config.capabilities, configured_route_ports(&config))?
    };
    let route_count = production_registry.as_ref().map_or_else(
        || routes.routes().len(),
        |registry| registry.bindings().len(),
    );
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
        access_policy = access_policy.with_session_validator(ports.auth_session_validator.clone());
    }
    let listener = StdTcpListener::bind(config.bind_address).map_err(ProductError::Bind)?;
    let address = listener.local_addr().map_err(ProductError::LocalAddress)?;
    // Start the calendar worker only after every fallible listener setup step
    // has succeeded.  If startup fails before the prepared product is handed
    // to the shutdown supervisor, no background calendar thread is left
    // running without an owner.  A source/worker start failure is closed
    // immediately for the same fail-closed guarantee.
    if let Some(manager) = &calendar_manager
        && let Err(error) = manager.start()
    {
        let _ = manager.close();
        return Err(ProductError::Calendar(error));
    }
    let production_runtime_core_ready = production_ports.as_ref().is_some_and(|ports| {
        ports.provider_status == "ready"
            && ports.opend_status == "ready"
            && ports.worker_status == "ready"
    }) && mcp_runtime_ready;
    active_provider_state.set_readiness(
        config.market_data_helper.is_some(),
        config.market_data_runtime_status_port.is_some(),
        config.market_data_router.is_some(),
    );
    // Report the same live readiness that dispatch will use.  The canonical
    // route count and digest remain immutable, while these status counters
    // reflect the provider/helper/router state published above.
    let ready_route_count = production_registry
        .as_ref()
        .map_or(route_count, |registry| {
            production_ports.as_ref().map_or(0, |ports| {
                registry
                    .bindings()
                    .iter()
                    .filter(|binding| {
                        registry.current_binding(binding, ports)
                            == product_production_ports::ProductionAdapterBinding::Ready
                    })
                    .count()
            })
        });
    let external_unavailable_route_count = production_registry.as_ref().map_or(0, |registry| {
        production_ports.as_ref().map_or(0, |ports| {
            registry
                .bindings()
                .iter()
                .filter(|binding| {
                    registry.current_binding(binding, ports)
                        == product_production_ports::ProductionAdapterBinding::ExternalUnavailable
                })
                .count()
        })
    });
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
                    .with_runtime(Arc::clone(state)
                        as Arc<dyn jftrade_settings::MarketDataProviderRuntimePort>)
            } else {
                MarketDataProviderSettingsService::new(settings_store.clone())
            },
            backtest_market_data_provider: BacktestMarketDataProviderSettingsService::new(
                settings_store.clone(),
            )
            .with_runtime(Arc::clone(&backtest_market_data_provider_state)
                as Arc<dyn jftrade_settings::MarketDataProviderRuntimePort>),
            mcp_server: {
                let service = if let Some(runtime) = mcp_server_runtime.as_ref() {
                    McpServerSettingsService::with_ports(
                        settings_store.clone(),
                        Some(Arc::clone(runtime) as Arc<dyn jftrade_settings::McpServerRuntimePort>),
                        Arc::new(jftrade_settings::SystemMcpServerSecrets),
                    )
                } else {
                    McpServerSettingsService::new(settings_store.clone())
                };
                if config.production {
                    service.require_runtime()
                } else {
                    service
                }
            },
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
            production_routes: production_registry.clone().map(Arc::new),
            production_ports: production_ports_for_api,
            notification: config.notification_port.clone(),
            calendar_manager: calendar_manager.clone(),
            watchlist_membership_snapshot: config
                .watchlist_membership_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.watchlist_memberships.clone())
                }),
            watchlist_read_snapshot: config.watchlist_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.watchlist.clone())
            }),
            portfolio_snapshot: config.portfolio_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.portfolio.clone())
            }),
            research_read_snapshot: config.research_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.research_read.clone())
            }),
            research_preset_read_snapshot: config
                .research_preset_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.research_preset_read.clone())
                }),
            execution_read_snapshot: config.execution_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.execution_read.clone())
            }),
            market_data_provider_read_snapshot: config
                .market_data_provider_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.provider.clone())
                }),
            market_data_catalog_read_snapshot: config
                .market_data_catalog_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.catalog.clone())),
            market_data_derivative_read_snapshot: config
                .market_data_derivative_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_derivative.clone())
                }),
            market_data_options_read_snapshot: config
                .market_data_options_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_options.clone())
                }),
            market_data_news_actions_read_snapshot: config
                .market_data_news_actions_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_news_actions.clone())
                }),
            market_data_news_search_read_snapshot: config
                .market_data_news_search_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_news_search.clone())
                }),
            adk_read_snapshot: config.adk_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.adk_read.clone())
            }),
            market_data_quote_read_snapshot: config
                .market_data_quote_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_quote.clone())
                }),
            market_data_prediction_read_snapshot: config
                .market_data_prediction_read_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_prediction.clone())
                }),
            market_data_runtime_status: config.market_data_runtime_status_port.clone(),
            broker_read_snapshot: config
                .broker_read_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.broker.clone())),
            system_read_snapshot: config.system_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.system_read.clone())
            }),
            remote_watchlist_snapshot: config.remote_watchlist_snapshot_port.clone().or_else(
                || {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.remote_watchlist.clone())
                },
            ),
            remote_watchlist_write: config.remote_watchlist_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.remote_watchlist_write.clone())
            }),
            watchlist_write: config.watchlist_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.watchlist_write.clone())
            }),
            plugin_uninstall_guidance_snapshot: config
                .plugin_uninstall_guidance_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.plugin_guidance.clone())
                }),
            plugin_snapshot: config
                .plugin_snapshot_port
                .clone()
                .or_else(|| production_ports.as_ref().map(|ports| ports.plugins.clone())),
            plugin_write: config.plugin_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.plugin_write.clone())
            }),
            research_preset_write: config.research_preset_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.research_preset_write.clone())
            }),
            strategy_definition_write: config.strategy_definition_write_port.clone().or_else(
                || {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.strategy_definition_write.clone())
                },
            ),
            market_data_provider_actions: config.market_data_provider_actions_port.clone().or_else(
                || {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.market_data_provider_actions.clone())
                },
            ),
            adk_chat_stream: config.adk_chat_stream_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.adk_chat_stream.clone())
            }),
            adk_mutation: config.adk_mutation_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.adk_mutation.clone())
            }),
            alert_snapshot: config.alert_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.alert_snapshot.clone())
            }),
            alert_write: config.alert_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.alert_write.clone())
            }),
            strategy_definition_snapshot: config.strategy_definition_snapshot_port.clone().or_else(
                || {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.strategy_definition.clone())
                },
            ),
            strategy_pine_analyze_snapshot: config
                .strategy_pine_analyze_snapshot_port
                .clone()
                .or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.strategy_pine_analyze.clone())
                }),
            backtest_read_snapshot: config.backtest_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.backtest_read.clone())
            }),
            backtest_sync_read_snapshot: config.backtest_sync_read_snapshot_port.clone().or_else(
                || {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.backtest_sync.clone())
                },
            ),
            backtests_write: config.backtests_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.backtests_write.clone())
            }),
            strategy_read_snapshot: config.strategy_read_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.strategy_read.clone())
            }),
            strategy_runtime_status: config.strategy_runtime_status_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.strategy_runtime_status.clone())
            }),
            strategy_runtime_write: config.strategy_runtime_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.strategy_runtime_write.clone())
            }),
            auth_session_snapshot: config.auth_session_snapshot_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.auth_session.clone())
            }),
            auth_session_write: config.auth_session_write_port.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.auth_session_write.clone())
            }),
            auth_session_invalidation: production_ports
                .as_ref()
                .map(|ports| ports.auth_session_invalidation.clone()),
            stage9_write_ports: ProductStage9WritePorts {
                execution: config.stage9_write_ports.execution.clone().or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.execution_write.clone())
                }),
                brokers: config.stage9_write_ports.brokers.clone().or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.brokers_write.clone())
                }),
                system: config.stage9_write_ports.system.clone().or_else(|| {
                    production_ports
                        .as_ref()
                        .map(|ports| ports.system_write.clone())
                }),
                market_data_subscription_mutation: config
                    .stage9_write_ports
                    .market_data_subscription_mutation
                    .clone()
                    .or_else(|| {
                        production_ports
                            .as_ref()
                            .map(|ports| ports.market_data_subscription_mutation.clone())
                    }),
                research_screen: config
                    .stage9_write_ports
                    .research_screen
                    .clone()
                    .or_else(|| {
                        production_ports
                            .as_ref()
                            .map(|ports| ports.research_screen_write.clone())
                    }),
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
    let web_router = web_runtime.as_ref().map(|web_runtime| {
        let mut web_state = state.clone();
        let web_settings = security_service_for_runtime
            .settings()
            .unwrap_or_else(|_| jftrade_settings::SecuritySettings::default());
        let web_access = AccessPolicy::web()
            .with_allowed_origins([
                "http://127.0.0.1:3003".to_owned(),
                "http://localhost:3003".to_owned(),
                "http://127.0.0.1:3000".to_owned(),
                "http://localhost:3000".to_owned(),
                format!("http://127.0.0.1:{}", web_settings.web_port),
                format!("http://localhost:{}", web_settings.web_port),
            ])
            .with_dynamic_origin_provider(
                Arc::clone(web_runtime) as Arc<dyn jftrade_api::AccessOriginProvider>
            );
        web_state.access = if let Some(validator) =
            state.access.session_validator.clone().or_else(|| {
                production_ports
                    .as_ref()
                    .map(|ports| ports.auth_session_validator.clone())
            }) {
            web_access.with_session_validator(validator)
        } else {
            // Production composition normally supplies the concrete auth
            // session manager above. Keeping the policy without a validator
            // fails closed if that invariant is ever violated.
            web_access
        };
        build_router(web_state)
    });
    let router = build_router(state);
    if let (Some(web_runtime), Some(web_router)) = (web_runtime.as_ref(), web_router) {
        web_runtime.install_router(web_router);
        security_service_for_runtime
            .apply_runtime()
            .map_err(|error| ProductError::SecurityRuntime {
                message: error.to_string(),
            })?;
    }
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
            web_runtime: web_runtime.clone(),
            mcp_server_runtime,
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

#[cfg(test)]
mod web_listener_tests {
    use super::*;
    use std::io::Read;
    use std::net::TcpStream;

    fn available_port() -> u16 {
        let listener = StdTcpListener::bind("127.0.0.1:0").expect("reserve test port");
        listener.local_addr().expect("test port address").port()
    }

    fn enabled_record(port: u16) -> SecuritySettingsRecord {
        SecuritySettingsRecord::new(true, false, port, "fixture-verifier")
    }

    fn router() -> axum::Router {
        axum::Router::new().fallback(|| async { "ok" })
    }

    #[test]
    fn disabled_web_access_does_not_start_a_listener() {
        let runtime = ProductWebServerRuntime::new();
        runtime.install_router(router());
        runtime
            .apply(&SecuritySettingsRecord::default())
            .expect("disable Web access");
        assert!(
            !runtime
                .status(&SecuritySettingsRecord::default())
                .expect("Web status")
        );
        runtime.shutdown_blocking().expect("shutdown Web runtime");
    }

    #[test]
    fn enabled_web_access_binds_and_shutdown_releases_port() {
        let runtime = ProductWebServerRuntime::new();
        runtime.install_router(router());
        let port = available_port();
        let record = enabled_record(port);
        runtime.apply(&record).expect("start Web listener");
        assert!(runtime.status(&record).expect("Web status"));

        let mut stream = TcpStream::connect(("127.0.0.1", port)).expect("connect Web listener");
        stream
            .set_read_timeout(Some(Duration::from_secs(1)))
            .expect("set read timeout");
        std::io::Write::write_all(
            &mut stream,
            b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n",
        )
        .expect("write Web request");
        let mut response = String::new();
        stream
            .read_to_string(&mut response)
            .expect("read Web response");
        assert!(
            response.starts_with("HTTP/1.1 200"),
            "response = {response}"
        );

        runtime.shutdown_blocking().expect("shutdown Web runtime");
        let listener = StdTcpListener::bind(("127.0.0.1", port))
            .expect("Web listener port released after shutdown");
        drop(listener);
    }

    #[test]
    fn port_conflict_keeps_the_previous_listener_running() {
        let runtime = ProductWebServerRuntime::new();
        runtime.install_router(router());
        let first_port = available_port();
        let second_port = available_port();
        let first = enabled_record(first_port);
        runtime.apply(&first).expect("start initial Web listener");
        let occupied = StdTcpListener::bind(("127.0.0.1", second_port)).expect("occupy port");
        let second = enabled_record(second_port);
        let error = runtime.apply(&second).expect_err("conflicting Web bind");
        assert!(
            error.contains("Web access port conflict"),
            "error = {error}"
        );
        assert!(runtime.status(&first).expect("previous Web status"));
        drop(occupied);
        runtime.shutdown_blocking().expect("shutdown Web runtime");
    }

    #[test]
    fn dynamic_origin_allowlist_tracks_the_current_web_port() {
        let runtime = ProductWebServerRuntime::new();
        runtime.install_router(router());
        let first_port = available_port();
        let second_port = available_port();
        let first = enabled_record(first_port);
        runtime.apply(&first).expect("start initial Web listener");
        assert!(runtime.allows_origin(&format!("http://127.0.0.1:{first_port}")));
        assert!(!runtime.allows_origin(&format!("http://127.0.0.1:{second_port}")));

        let second = enabled_record(second_port);
        runtime.apply(&second).expect("rebind Web listener");
        assert!(!runtime.allows_origin(&format!("http://127.0.0.1:{first_port}")));
        assert!(runtime.allows_origin(&format!("http://127.0.0.1:{second_port}")));
        runtime.shutdown_blocking().expect("shutdown Web runtime");
    }
}
