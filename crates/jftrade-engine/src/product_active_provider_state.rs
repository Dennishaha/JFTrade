//! Shared in-memory active market-data provider state.

use jftrade_settings::{MarketDataProvider, MarketDataProviderRuntimePort};
use std::sync::{Arc, Mutex, RwLock};

type Activation =
    dyn Fn(MarketDataProvider, Option<MarketDataProvider>) -> Result<(), String> + Send + Sync;
type ReadinessReader = dyn Fn() -> (bool, bool, bool) + Send + Sync;

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub(crate) struct ProviderRuntimeSnapshot {
    pub provider: Option<MarketDataProvider>,
    pub generation: u64,
    pub helper_ready: bool,
    pub opend_ready: bool,
    pub router_ready: bool,
    pub closing: bool,
}

#[derive(Clone, Default)]
pub(crate) struct ActiveProviderState {
    activation: Arc<Mutex<Option<Arc<Activation>>>>,
    readiness_reader: Arc<Mutex<Option<Arc<ReadinessReader>>>>,
    /// Serializes the physical transition and publication of the snapshot.
    /// The callback may stop one runtime and start another, so holding this
    /// guard across the callback is what prevents two concurrent settings
    /// writes from creating overlapping runtime owners.
    transition: Arc<Mutex<()>>,
    snapshot: Arc<RwLock<ProviderRuntimeSnapshot>>,
}

impl std::fmt::Debug for ActiveProviderState {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ActiveProviderState")
            .field("provider", &self.get())
            .field("closing", &self.snapshot().closing)
            .finish()
    }
}

impl ActiveProviderState {
    pub(crate) fn new(initial: Option<MarketDataProvider>) -> Self {
        Self {
            activation: Arc::new(Mutex::new(None)),
            readiness_reader: Arc::new(Mutex::new(None)),
            transition: Arc::new(Mutex::new(())),
            snapshot: Arc::new(RwLock::new(ProviderRuntimeSnapshot {
                provider: initial,
                generation: 0,
                helper_ready: false,
                opend_ready: false,
                router_ready: false,
                closing: false,
            })),
        }
    }

    pub(crate) fn with_activation(self, activation: Arc<Activation>) -> Self {
        *self.activation.lock().unwrap_or_else(|e| e.into_inner()) = Some(activation);
        self
    }

    pub(crate) fn with_dynamic_readiness(self, reader: Arc<ReadinessReader>) -> Self {
        *self
            .readiness_reader
            .lock()
            .unwrap_or_else(|e| e.into_inner()) = Some(reader);
        self
    }

    pub(crate) fn begin_shutdown(&self) {
        self.snapshot
            .write()
            .unwrap_or_else(|e| e.into_inner())
            .closing = true;
    }

    pub(crate) fn snapshot(&self) -> ProviderRuntimeSnapshot {
        let mut snapshot = self
            .snapshot
            .read()
            .unwrap_or_else(|e| e.into_inner())
            .clone();
        if let Some(reader) = self
            .readiness_reader
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .as_ref()
        {
            let (helper_ready, opend_ready, router_ready) = reader();
            snapshot.helper_ready = helper_ready;
            snapshot.opend_ready = opend_ready;
            snapshot.router_ready = router_ready;
        }
        snapshot
    }

    pub(crate) fn set_readiness(&self, helper_ready: bool, opend_ready: bool, router_ready: bool) {
        let mut snapshot = self.snapshot.write().unwrap_or_else(|e| e.into_inner());
        snapshot.helper_ready = helper_ready;
        snapshot.opend_ready = opend_ready;
        snapshot.router_ready = router_ready;
    }

    pub(crate) fn get(&self) -> Option<MarketDataProvider> {
        self.snapshot().provider
    }
}

impl MarketDataProviderRuntimePort for ActiveProviderState {
    fn needs_activation(&self, provider: MarketDataProvider) -> bool {
        self.get() != Some(provider)
    }

    fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
        let _transition = self
            .transition
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        if self.snapshot().closing {
            return Err("market-data provider runtime is shutting down".to_owned());
        }
        let previous = self.get();
        if previous == Some(provider) {
            return Ok(());
        }
        let activation = self
            .activation
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .clone();
        if let Some(activation) = activation {
            activation(provider, previous)?;
        }
        let mut snapshot = self.snapshot.write().unwrap_or_else(|e| e.into_inner());
        snapshot.provider = Some(provider);
        snapshot.generation = snapshot.generation.saturating_add(1);
        snapshot.opend_ready = provider == MarketDataProvider::Futu;
        Ok(())
    }

    fn prepare_backtest(&self, _provider: MarketDataProvider) -> Result<(), String> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::thread;
    use std::time::Duration;

    #[test]
    fn activation_publishes_only_after_runtime_prepare_and_rejects_shutdown() {
        let calls = Arc::new(AtomicUsize::new(0));
        let calls_for_activation = Arc::clone(&calls);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Yfinance)).with_activation(
            Arc::new(move |provider, _previous| {
                calls_for_activation.fetch_add(1, Ordering::SeqCst);
                if provider == MarketDataProvider::Futu {
                    Err("OpenD unavailable".to_owned())
                } else {
                    Ok(())
                }
            }),
        );
        assert!(state.activate(MarketDataProvider::Futu).is_err());
        assert_eq!(state.get(), Some(MarketDataProvider::Yfinance));
        assert_eq!(calls.load(Ordering::SeqCst), 1);
        state.begin_shutdown();
        assert!(state.activate(MarketDataProvider::Akshare).is_err());
        assert_eq!(state.get(), Some(MarketDataProvider::Yfinance));
    }

    #[test]
    fn provider_transitions_are_serialized_and_publish_one_snapshot() {
        let in_flight = Arc::new(AtomicUsize::new(0));
        let max_in_flight = Arc::new(AtomicUsize::new(0));
        let seen = Arc::new(Mutex::new(Vec::new()));
        let in_flight_cb = Arc::clone(&in_flight);
        let max_cb = Arc::clone(&max_in_flight);
        let seen_cb = Arc::clone(&seen);
        let configured = Arc::new(
            ActiveProviderState::new(Some(MarketDataProvider::Yfinance)).with_activation(Arc::new(
                move |next, previous| {
                    let current = in_flight_cb.fetch_add(1, Ordering::SeqCst) + 1;
                    max_cb.fetch_max(current, Ordering::SeqCst);
                    seen_cb
                        .lock()
                        .unwrap_or_else(|error| error.into_inner())
                        .push((previous, next));
                    thread::sleep(Duration::from_millis(5));
                    in_flight_cb.fetch_sub(1, Ordering::SeqCst);
                    Ok(())
                },
            )),
        );
        let first = {
            let state = Arc::clone(&configured);
            thread::spawn(move || state.activate(MarketDataProvider::Akshare))
        };
        let second = {
            let state = Arc::clone(&configured);
            thread::spawn(move || state.activate(MarketDataProvider::Futu))
        };
        first
            .join()
            .expect("first transition")
            .expect("first activation");
        second
            .join()
            .expect("second transition")
            .expect("second activation");

        assert_eq!(max_in_flight.load(Ordering::SeqCst), 1);
        let transitions = seen.lock().unwrap_or_else(|error| error.into_inner());
        assert_eq!(transitions.len(), 2);
        assert_eq!(transitions[0].0, Some(MarketDataProvider::Yfinance));
        assert_eq!(transitions[1].0, Some(transitions[0].1));
        assert_eq!(configured.get(), Some(transitions[1].1));
        assert_eq!(configured.snapshot().generation, 2);
    }
}
