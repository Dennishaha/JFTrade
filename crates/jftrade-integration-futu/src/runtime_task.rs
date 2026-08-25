use std::io;
use std::sync::{Arc, Mutex, RwLock, mpsc};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::{InstrumentRef, ProviderRouter, TickCache};
use thiserror::Error;

use crate::{
    OpenDSessionCoordinator, OpenDSessionCoordinatorError, OpenDSessionCoordinatorOutcome,
};

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
}

impl Default for OpenDSessionRuntimeConfig {
    fn default() -> Self {
        Self {
            poll_interval: Duration::from_millis(100),
            event_timeout: Duration::from_millis(10),
            cache_capacity_per_instrument: 2,
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
        let worker = thread::Builder::new()
            .name("jftrade-opend-runtime".to_owned())
            .spawn(move || {
                run_task(
                    worker_coordinator,
                    worker_demand,
                    worker_cache,
                    worker_status,
                    worker_router,
                    stop_rx,
                    config,
                )
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

fn run_task(
    coordinator: Arc<Mutex<OpenDSessionCoordinator>>,
    demand: Arc<RwLock<Vec<InstrumentRef>>>,
    cache: Arc<Mutex<TickCache>>,
    status: Arc<Mutex<OpenDSessionRuntimeStatus>>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    stop_rx: mpsc::Receiver<()>,
    config: OpenDSessionRuntimeConfig,
) {
    let poll_interval = positive_duration(config.poll_interval, Duration::from_millis(100));
    let event_timeout = positive_duration(config.event_timeout, Duration::from_millis(10));
    while stop_rx.recv_timeout(poll_interval).is_err() {
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
        if coordinator.desired() != desired
            && let Err(error) =
                coordinator.reconcile_topology(&desired, now.unix_millis().unwrap_or_default())
        {
            iteration_error = Some(error.to_string());
        }
        if iteration_error.is_none() {
            match coordinator.poll_once(now, event_timeout) {
                Ok(OpenDSessionCoordinatorOutcome::Reconnected { .. }) => {
                    let mut state = status.lock().unwrap_or_else(|error| error.into_inner());
                    state.reconnects = state.reconnects.saturating_add(1);
                }
                Ok(_) => {}
                Err(error) => iteration_error = Some(error.to_string()),
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
        let mut state = status.lock().unwrap_or_else(|error| error.into_inner());
        state.iterations = state.iterations.saturating_add(1);
        state.last_error = iteration_error;
    }
}

fn positive_duration(value: Duration, fallback: Duration) -> Duration {
    if value.is_zero() { fallback } else { value }
}

#[cfg(test)]
mod tests {
    use std::io::Write;
    use std::net::TcpListener;
    use std::sync::Arc;

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
        std::thread::sleep(Duration::from_millis(30));
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
}
