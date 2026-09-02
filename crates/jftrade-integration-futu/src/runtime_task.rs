use std::io;
use std::sync::{Arc, Mutex, RwLock, mpsc};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::{
    HealthStatus, InstrumentRef, ProviderReadiness, ProviderRouter, TickCache,
};
use thiserror::Error;

use crate::{
    OpenDSessionCoordinator, OpenDSessionCoordinatorError, OpenDSessionCoordinatorOutcome,
};

/// Configuration for the explicitly composed OpenD runtime task.
///
/// The task is never started by the default product profile. A caller that
/// owns an authenticated coordinator may opt in to a bounded polling thread
/// Listener for real-time events and errors emitted by OpenDSessionRuntime.
pub trait OpenDSessionEventListener: Send + Sync + std::fmt::Debug {
    fn on_event(&self, outcome: &OpenDSessionCoordinatorOutcome);
    fn on_error(&self, error: &str);
}

/// Configuration for the explicitly composed OpenD runtime task.
///
/// The task is never started by the default product profile. A caller that
/// owns an authenticated coordinator may opt in to a bounded polling thread
/// and update its demand through [`OpenDSessionRuntime::set_demand`].
#[derive(Clone, Debug)]
pub struct OpenDSessionRuntimeConfig {
    pub poll_interval: Duration,
    pub event_timeout: Duration,
    pub cache_capacity_per_instrument: usize,
    pub reconnect_initial_delay: Duration,
    pub reconnect_max_delay: Duration,
    pub quota_refresh_enabled: bool,
    pub event_listener: Option<Arc<dyn OpenDSessionEventListener>>,
}

impl Default for OpenDSessionRuntimeConfig {
    fn default() -> Self {
        Self {
            poll_interval: Duration::from_millis(100),
            event_timeout: Duration::from_millis(10),
            cache_capacity_per_instrument: 2,
            reconnect_initial_delay: Duration::from_millis(250),
            reconnect_max_delay: Duration::from_secs(5),
            quota_refresh_enabled: false,
            event_listener: None,
        }
    }
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct OpenDSessionRuntimeStatus {
    pub iterations: u64,
    pub snapshot_polls: u64,
    pub reconnects: u64,
    pub last_error: Option<String>,
}

#[derive(Debug, Error)]
pub enum OpenDSessionRuntimeError {
    #[error("OpenD runtime task is already stopped")]
    Stopped,
    #[error("OpenD runtime task panicked")]
    WorkerPanicked,
    #[error("start OpenD runtime task: {0}")]
    WorkerStart(#[source] io::Error),
    #[error("OpenD runtime coordinator failed: {0}")]
    Coordinator(#[from] OpenDSessionCoordinatorError),
    #[error("OpenD runtime and ProviderRouter must share one MarketDataRuntimeRecorder")]
    SharedRecorderMismatch,
}

/// Explicit owner for OpenD cadence, dynamic demand and snapshot cache.
///
/// This type is deliberately separate from `OpenDSessionCoordinator`: the
/// coordinator remains a synchronous protocol boundary, while this owner
/// supplies the runtime task and the mutable demand source. ProductRuntime
/// only stores it when a composition root opts in.
pub struct OpenDSessionRuntime {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    demand: Arc<RwLock<Vec<InstrumentRef>>>,
    cache: Arc<Mutex<TickCache>>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    status: Arc<Mutex<OpenDSessionRuntimeStatus>>,
    stop_tx: Option<mpsc::Sender<()>>,
    worker: Option<JoinHandle<()>>,
}

impl std::fmt::Debug for OpenDSessionRuntime {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("OpenDSessionRuntime")
            .field("demand_count", &self.demand().len())
            .field("status", &self.status())
            .field("running", &self.worker.is_some())
            .finish()
    }
}

impl OpenDSessionRuntime {
    pub fn start(
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
        config: OpenDSessionRuntimeConfig,
    ) -> Result<Self, OpenDSessionRuntimeError> {
        let cache = Arc::new(Mutex::new(TickCache::new(
            config.cache_capacity_per_instrument,
        )));
        Self::start_inner(coordinator, cache, None, config)
    }

    /// Starts the task with a ProviderRouter as the sole demand owner and its
    /// cache as the sole quote-snapshot owner. The task only consumes the
    /// router's normalized demand; callers must continue to mutate demand via
    /// `ProviderRouter::acquire_demand`/`release_demand`.
    pub fn start_with_provider_router(
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
        router: Arc<Mutex<ProviderRouter>>,
        config: OpenDSessionRuntimeConfig,
    ) -> Result<Self, OpenDSessionRuntimeError> {
        let coordinator_recorder = coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .lifecycle()
            .recorder();
        let router_recorder = router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .runtime_recorder();
        if !Arc::ptr_eq(&coordinator_recorder, &router_recorder) {
            return Err(OpenDSessionRuntimeError::SharedRecorderMismatch);
        }
        let cache = router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .cache_handle();
        Self::start_inner(coordinator, cache, Some(router), config)
    }

    fn start_inner(
        coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
        cache: Arc<Mutex<TickCache>>,
        router: Option<Arc<Mutex<ProviderRouter>>>,
        config: OpenDSessionRuntimeConfig,
    ) -> Result<Self, OpenDSessionRuntimeError> {
        let initial_demand = coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .desired();
        let demand = Arc::new(RwLock::new(initial_demand));
        let status = Arc::new(Mutex::new(OpenDSessionRuntimeStatus::default()));
        let (stop_tx, stop_rx) = mpsc::channel();
        let worker_demand = Arc::clone(&demand);
        let worker_cache = Arc::clone(&cache);
        let worker_status = Arc::clone(&status);
        let worker_coordinator = Arc::clone(&coordinator);
        let worker_router = router.clone();
        let provider_id = router.as_ref().and_then(|router| {
            let provider_id = router
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .runtime()
                .active_provider;
            (!provider_id.is_empty()).then_some(provider_id)
        });
        let recorder = coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .recorder();
        let worker_provider_id = provider_id.clone();
        let worker = thread::Builder::new()
            .name("jftrade-opend-runtime".to_owned())
            .spawn(move || {
                run_task(RuntimeTaskContext {
                    coordinator: worker_coordinator,
                    demand: worker_demand,
                    cache: worker_cache,
                    status: worker_status,
                    router: worker_router,
                    provider_id: worker_provider_id,
                    recorder,
                    stop_rx,
                    config,
                })
            })
            .map_err(OpenDSessionRuntimeError::WorkerStart)?;
        Ok(Self {
            coordinator,
            demand,
            cache,
            router,
            status,
            stop_tx: Some(stop_tx),
            worker: Some(worker),
        })
    }

    pub fn coordinator(&self) -> Arc<Mutex<OpenDSessionCoordinator>> {
        Arc::clone(&self.coordinator)
    }

    pub fn demand(&self) -> Vec<InstrumentRef> {
        if let Some(router) = self.router.as_ref() {
            return router
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .demand()
                .active;
        }
        self.demand
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub fn set_demand(&self, demand: Vec<InstrumentRef>) {
        if self.router.is_some() {
            return;
        }
        *self
            .demand
            .write()
            .unwrap_or_else(|error| error.into_inner()) = demand;
    }

    pub fn cache(&self) -> Arc<Mutex<TickCache>> {
        Arc::clone(&self.cache)
    }

    pub fn uses_provider_router(&self) -> bool {
        self.router.is_some()
    }

    pub fn status(&self) -> OpenDSessionRuntimeStatus {
        self.status
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub fn shutdown(&mut self) -> Result<(), OpenDSessionRuntimeError> {
        if self.stop_tx.is_none() && self.worker.is_none() {
            return Err(OpenDSessionRuntimeError::Stopped);
        }
        if let Some(stop_tx) = self.stop_tx.take() {
            let _ = stop_tx.send(());
        }
        if let Some(worker) = self.worker.take()
            && worker.join().is_err()
        {
            return Err(OpenDSessionRuntimeError::WorkerPanicked);
        }
        self.coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .close()?;
        Ok(())
    }
}

impl Drop for OpenDSessionRuntime {
    fn drop(&mut self) {
        if let Some(stop_tx) = self.stop_tx.take() {
            let _ = stop_tx.send(());
        }
        if let Some(worker) = self.worker.take() {
            let _ = worker.join();
        }
        let _ = self
            .coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .close();
    }
}

struct RuntimeTaskContext {
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    demand: Arc<RwLock<Vec<InstrumentRef>>>,
    cache: Arc<Mutex<TickCache>>,
    status: Arc<Mutex<OpenDSessionRuntimeStatus>>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    provider_id: Option<String>,
    recorder: Arc<jftrade_marketdata::MarketDataRuntimeRecorder>,
    stop_rx: mpsc::Receiver<()>,
    config: OpenDSessionRuntimeConfig,
}

fn run_task(context: RuntimeTaskContext) {
    let RuntimeTaskContext {
        coordinator,
        demand,
        cache,
        status,
        router,
        provider_id,
        recorder,
        stop_rx,
        config,
    } = context;
    let poll_interval = positive_duration(config.poll_interval, Duration::from_millis(100));
    let event_timeout = positive_duration(config.event_timeout, Duration::from_millis(10));
    let reconnect_initial_delay =
        positive_duration(config.reconnect_initial_delay, Duration::from_millis(250));
    let reconnect_max_delay = positive_duration(config.reconnect_max_delay, Duration::from_secs(5));
    let reconnect_max_delay = reconnect_max_delay.max(reconnect_initial_delay);
    let mut reconnect_failures = 0u32;
    let mut reconnect_not_before = None;
    let mut quota_refresh_pending = config.quota_refresh_enabled;
    while stop_rx.recv_timeout(poll_interval).is_err() {
        if let Some(not_before) = reconnect_not_before
            && std::time::Instant::now() < not_before
        {
            continue;
        }
        reconnect_not_before = None;
        let now = WireTimestamp::from_offset_datetime(time::OffsetDateTime::now_utc());
        let desired = router
            .as_ref()
            .map(|router| {
                router
                    .lock()
                    .unwrap_or_else(|error| error.into_inner())
                    .demand()
                    .active
            })
            .unwrap_or_else(|| {
                demand
                    .read()
                    .unwrap_or_else(|error| error.into_inner())
                    .clone()
            });
        let mut coordinator = coordinator
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let mut iteration_error = None;
        if coordinator.desired() != desired {
            match coordinator.reconcile_topology(&desired, now.unix_millis().unwrap_or_default()) {
                Ok(()) => quota_refresh_pending = config.quota_refresh_enabled,
                Err(error) => iteration_error = Some(error.to_string()),
            }
        }
        if iteration_error.is_none() {
            match coordinator.poll_once(now, event_timeout) {
                Ok(outcome) => {
                    if let Some(listener) = config.event_listener.as_ref() {
                        listener.on_event(&outcome);
                    }
                    if let OpenDSessionCoordinatorOutcome::Reconnected { .. } = outcome {
                        let mut state = status.lock().unwrap_or_else(|error| error.into_inner());
                        state.reconnects = state.reconnects.saturating_add(1);
                        reconnect_failures = 0;
                        quota_refresh_pending = config.quota_refresh_enabled;
                    }
                }
                Err(error) => iteration_error = Some(error.to_string()),
            }
        }
        if iteration_error.is_none() && quota_refresh_pending {
            let checked_at_ms = now.unix_millis().unwrap_or_default();
            match coordinator.refresh_quota(checked_at_ms) {
                Ok(()) => quota_refresh_pending = false,
                Err(error) => {
                    coordinator.record_quota_error(checked_at_ms, error.to_string());
                    iteration_error = Some(error.to_string());
                }
            }
        }
        if iteration_error.is_none() {
            let mut cache = cache.lock().unwrap_or_else(|error| error.into_inner());
            if let Err(error) = coordinator.poll_snapshot(&mut cache, now) {
                iteration_error = Some(error.to_string());
            } else {
                let mut state = status.lock().unwrap_or_else(|error| error.into_inner());
                state.snapshot_polls = state.snapshot_polls.saturating_add(1);
            }
        }
        drop(coordinator);
        if let Some(error) = iteration_error.as_deref()
            && let Some(listener) = config.event_listener.as_ref()
        {
            listener.on_error(error);
        }
        if let (Some(router), Some(provider_id)) = (router.as_ref(), provider_id.as_deref()) {
            sync_provider_health(router, provider_id, &recorder);
        }
        let mut state = status.lock().unwrap_or_else(|error| error.into_inner());
        state.iterations = state.iterations.saturating_add(1);
        state.last_error = iteration_error;
        if state.last_error.is_some() {
            reconnect_failures = reconnect_failures.saturating_add(1);
            reconnect_not_before = Some(
                std::time::Instant::now()
                    .checked_add(reconnect_delay(
                        reconnect_initial_delay,
                        reconnect_max_delay,
                        reconnect_failures,
                    ))
                    .unwrap_or_else(std::time::Instant::now),
            );
        } else {
            reconnect_failures = 0;
        }
    }
}

fn sync_provider_health(
    router: &Arc<Mutex<ProviderRouter>>,
    provider_id: &str,
    recorder: &jftrade_marketdata::MarketDataRuntimeRecorder,
) {
    let state = recorder.snapshot();
    if state.active_count == 0 && !state.closed {
        return;
    }
    let error = state
        .stream_last_error
        .clone()
        .or(state.quote_last_error.clone())
        .or_else(|| state.closed.then(|| "OpenD session closed".to_owned()));
    let readiness = if state.closed || error.is_some() {
        ProviderReadiness::Failed
    } else if state.connected {
        ProviderReadiness::Ready
    } else {
        ProviderReadiness::Warming
    };
    let _ = router
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
        .update_health(
            provider_id,
            HealthStatus {
                connected: state.connected && !state.closed,
                readiness,
                stream_mode: "streaming".to_owned(),
                active_count: state.active_count,
                last_error: error,
            },
        );
}

fn positive_duration(value: Duration, fallback: Duration) -> Duration {
    if value.is_zero() { fallback } else { value }
}

fn reconnect_delay(initial: Duration, maximum: Duration, failures: u32) -> Duration {
    let shift = failures.saturating_sub(1).min(31);
    initial
        .checked_mul(1u32 << shift)
        .unwrap_or(maximum)
        .min(maximum)
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::{Arc, mpsc};
    use std::time::Instant;

    use super::*;
    use crate::{OpenDTcpProbeConfig, PROTO_INIT_CONNECT};
    use prost::Message;

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    #[test]
    fn runtime_task_updates_dynamic_demand_and_shuts_down_its_coordinator() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = crate::transport::read_framed_frame(&mut stream).expect("init");
            assert_eq!(init.header.proto_id, PROTO_INIT_CONNECT);
            let body = InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 1,
                }),
            }
            .encode_to_vec();
            stream
                .write_all(
                    &crate::encode_frame(PROTO_INIT_CONNECT, init.header.serial_no, &body)
                        .expect("response frame"),
                )
                .expect("write response");
            std::thread::sleep(Duration::from_millis(100));
        });
        let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
        let coordinator = Arc::new(Mutex::new(
            OpenDSessionCoordinator::connect(
                OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
                Arc::clone(&recorder),
                vec![],
                0,
            )
            .expect("coordinator"),
        ));
        let mut runtime = OpenDSessionRuntime::start(
            Arc::clone(&coordinator),
            OpenDSessionRuntimeConfig {
                poll_interval: Duration::from_millis(5),
                event_timeout: Duration::from_millis(1),
                ..OpenDSessionRuntimeConfig::default()
            },
        )
        .expect("runtime task");
        runtime.set_demand(vec![InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }]);
        let deadline = std::time::Instant::now() + Duration::from_millis(1000);
        while runtime.status().iterations == 0 && std::time::Instant::now() < deadline {
            std::thread::sleep(Duration::from_millis(5));
        }
        assert_eq!(runtime.demand().len(), 1);
        assert!(runtime.status().iterations > 0);
        runtime.shutdown().expect("shutdown");
        assert!(
            !coordinator
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .close()
                .expect("idempotent close")
        );
        server.join().expect("server");
    }

    #[test]
    fn provider_router_task_uses_router_demand_and_cache_without_a_second_owner() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = crate::transport::read_framed_frame(&mut stream).expect("init");
            let body = InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 1,
                }),
            }
            .encode_to_vec();
            stream
                .write_all(
                    &crate::encode_frame(PROTO_INIT_CONNECT, init.header.serial_no, &body)
                        .expect("response frame"),
                )
                .expect("write response");
            std::thread::sleep(Duration::from_millis(50));
        });
        let router = Arc::new(Mutex::new(jftrade_marketdata::ProviderRouter::new(2)));
        let recorder = router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .runtime_recorder();
        let coordinator = Arc::new(Mutex::new(
            OpenDSessionCoordinator::connect(
                OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
                recorder,
                vec![],
                0,
            )
            .expect("coordinator"),
        ));
        let mut runtime = OpenDSessionRuntime::start_with_provider_router(
            Arc::clone(&coordinator),
            Arc::clone(&router),
            OpenDSessionRuntimeConfig {
                poll_interval: Duration::from_millis(5),
                event_timeout: Duration::from_millis(1),
                ..OpenDSessionRuntimeConfig::default()
            },
        )
        .expect("router task");
        assert!(runtime.uses_provider_router());
        assert!(Arc::ptr_eq(
            &runtime.cache(),
            &router.lock().unwrap().cache_handle()
        ));
        runtime.set_demand(vec![InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }]);
        assert!(runtime.demand().is_empty());
        runtime.shutdown().expect("shutdown");
        server.join().expect("server");
    }

    #[test]
    fn provider_health_sync_replays_recorder_failure_and_recovery() {
        let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
        let router = Arc::new(Mutex::new(jftrade_marketdata::ProviderRouter::new(2)));
        let descriptor = jftrade_marketdata::ProviderDescriptor {
            selection_id: "futu".to_owned(),
            provider_id: "futu".to_owned(),
            display_name: "Futu OpenD".to_owned(),
            broker_id: Some("futu".to_owned()),
            source: "futu".to_owned(),
            default_market: "US".to_owned(),
            supported_markets: vec!["US".to_owned()],
            transports: vec!["stream".to_owned()],
            capabilities: jftrade_marketdata::ProviderCapabilities {
                snapshots: true,
                streaming_quotes: true,
                ..Default::default()
            },
            constraints: jftrade_marketdata::ProviderConstraints::default(),
            notes: Vec::new(),
        };
        {
            let mut router_guard = router.lock().expect("router");
            router_guard
                .register(
                    descriptor,
                    jftrade_marketdata::HealthStatus {
                        connected: true,
                        readiness: ProviderReadiness::Ready,
                        stream_mode: "streaming".to_owned(),
                        ..Default::default()
                    },
                )
                .expect("register");
            router_guard
                .activate("futu", jftrade_marketdata::ActivationMode::Explicit)
                .expect("activate");
        }
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let now: WireTimestamp = "2026-08-25T00:00:00Z".parse().expect("timestamp");
        recorder.record_stream_failure(generation, now, "socket closed");
        sync_provider_health(&router, "futu", &recorder);
        assert_eq!(
            router.lock().expect("router").runtime().readiness,
            ProviderReadiness::Failed
        );
        recorder.record_stream_connected(generation);
        sync_provider_health(&router, "futu", &recorder);
        assert_eq!(
            router.lock().expect("router").runtime().readiness,
            ProviderReadiness::Ready
        );
    }

    #[test]
    fn reconnect_delay_is_bounded_and_recovers_from_zero_or_overflowing_inputs() {
        assert_eq!(
            reconnect_delay(Duration::from_millis(100), Duration::from_secs(1), 1),
            Duration::from_millis(100)
        );
        assert_eq!(
            reconnect_delay(Duration::from_millis(100), Duration::from_secs(1), 4),
            Duration::from_millis(800)
        );
        assert_eq!(
            reconnect_delay(Duration::from_millis(100), Duration::from_secs(1), 5),
            Duration::from_secs(1)
        );
        assert_eq!(
            reconnect_delay(Duration::from_secs(10), Duration::from_secs(1), 1),
            Duration::from_secs(1)
        );
    }

    #[test]
    fn runtime_task_backoff_replays_after_a_failed_reconnect_attempt() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let (release_initial_tx, release_initial_rx) = mpsc::channel();
        let (accept_tx, accept_rx) = mpsc::channel();
        let server = std::thread::spawn(move || {
            let (mut initial, _) = listener.accept().expect("initial accept");
            accept_tx.send(Instant::now()).expect("initial timestamp");
            let init = crate::transport::read_framed_frame(&mut initial).expect("initial init");
            initial
                .write_all(
                    &crate::encode_frame(
                        PROTO_INIT_CONNECT,
                        init.header.serial_no,
                        &InitResponse {
                            ret_type: Some(0),
                            s2c: Some(InitState {
                                server_ver: 1009,
                                conn_id: 1,
                            }),
                        }
                        .encode_to_vec(),
                    )
                    .expect("initial response"),
                )
                .expect("write initial response");
            release_initial_rx.recv().expect("release initial");
            drop(initial);

            let (mut failed, _) = listener.accept().expect("failed reconnect accept");
            accept_tx.send(Instant::now()).expect("failed timestamp");
            let _ = crate::transport::read_framed_frame(&mut failed);
            drop(failed);

            let (mut recovered, _) = listener.accept().expect("recovery accept");
            accept_tx.send(Instant::now()).expect("recovery timestamp");
            let init = crate::transport::read_framed_frame(&mut recovered).expect("recovery init");
            recovered
                .write_all(
                    &crate::encode_frame(
                        PROTO_INIT_CONNECT,
                        init.header.serial_no,
                        &InitResponse {
                            ret_type: Some(0),
                            s2c: Some(InitState {
                                server_ver: 1009,
                                conn_id: 2,
                            }),
                        }
                        .encode_to_vec(),
                    )
                    .expect("recovery response"),
                )
                .expect("write recovery response");
            let mut byte = [0_u8; 1];
            let _ = recovered.read(&mut byte);
        });

        let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
        let coordinator = Arc::new(Mutex::new(
            OpenDSessionCoordinator::connect(
                OpenDTcpProbeConfig::new(address, Duration::from_secs(1)),
                Arc::clone(&recorder),
                vec![],
                0,
            )
            .expect("coordinator"),
        ));
        let initial_accept = accept_rx.recv().expect("initial accepted");
        let mut runtime = OpenDSessionRuntime::start(
            Arc::clone(&coordinator),
            OpenDSessionRuntimeConfig {
                poll_interval: Duration::from_millis(5),
                event_timeout: Duration::from_millis(1),
                reconnect_initial_delay: Duration::from_millis(40),
                reconnect_max_delay: Duration::from_millis(40),
                ..OpenDSessionRuntimeConfig::default()
            },
        )
        .expect("runtime task");
        release_initial_tx.send(()).expect("release initial");

        let failed_accept = accept_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("failed reconnect accepted");
        let recovered_accept = accept_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("recovery accepted");
        assert!(failed_accept.duration_since(initial_accept) < Duration::from_secs(1));
        assert!(recovered_accept.duration_since(failed_accept) >= Duration::from_millis(30));

        for _ in 0..100 {
            if runtime.status().reconnects > 0 {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }
        assert_eq!(runtime.status().reconnects, 1);
        runtime.shutdown().expect("shutdown");
        server.join().expect("server");
    }
}
