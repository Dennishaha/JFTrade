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
