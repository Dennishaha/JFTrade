use std::sync::{Arc, RwLock};

use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::SettingsStoreError;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum MarketDataProvider {
    Futu,
    Yfinance,
    #[default]
    Akshare,
}

pub trait MarketDataProviderSettingsStorePort: Send + Sync {
    fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError>;

    fn save_active_market_data_provider(
        &self,
        provider: MarketDataProvider,
    ) -> Result<(), SettingsStoreError>;
}

pub trait BacktestMarketDataProviderSettingsStorePort: Send + Sync {
    fn load_backtest_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError>;

    fn save_backtest_market_data_provider(
        &self,
        provider: MarketDataProvider,
    ) -> Result<(), SettingsStoreError>;
}

pub trait MarketDataProviderRuntimePort: Send + Sync {
    fn needs_activation(&self, provider: MarketDataProvider) -> bool;
    fn activate(&self, provider: MarketDataProvider) -> Result<(), String>;
    fn prepare_backtest(&self, provider: MarketDataProvider) -> Result<(), String>;
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum MarketDataProviderSettingsError {
    #[error("active market-data provider must be futu, yfinance, or akshare")]
    Invalid,
    #[error("could not apply market-data provider settings: {0}")]
    Runtime(String),
    #[error(transparent)]
    Store(#[from] SettingsStoreError),
}

#[derive(Clone)]
pub struct MarketDataProviderSettingsService {
    store: Arc<dyn MarketDataProviderSettingsStorePort>,
    runtime: Option<Arc<dyn MarketDataProviderRuntimePort>>,
    /// Serializes provider transitions and keeps reads out of the window
    /// between a durable write and the runtime rollback that may undo it.
    provider_lock: Arc<RwLock<()>>,
}

#[derive(Clone)]
pub struct BacktestMarketDataProviderSettingsService {
    store: Arc<dyn BacktestMarketDataProviderSettingsStorePort>,
    runtime: Option<Arc<dyn MarketDataProviderRuntimePort>>,
    /// Same fencing contract as the active-provider service: the owner of the
    /// write holds the lock until the runtime agrees with the durable value.
    backtest_provider_lock: Arc<RwLock<()>>,
}

impl BacktestMarketDataProviderSettingsService {
    pub fn new(store: Arc<dyn BacktestMarketDataProviderSettingsStorePort>) -> Self {
        Self {
            store,
            runtime: None,
            backtest_provider_lock: Arc::new(RwLock::new(())),
        }
    }

    pub fn with_runtime(mut self, runtime: Arc<dyn MarketDataProviderRuntimePort>) -> Self {
        self.runtime = Some(runtime);
        self
    }

    pub fn active_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        let _guard = self
            .backtest_provider_lock
            .read()
            .unwrap_or_else(|error| error.into_inner());
        self.stored_provider()
    }

    fn stored_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        Ok(self
            .store
            .load_backtest_market_data_provider()?
            .as_deref()
            .map(normalize_market_data_provider)
            .unwrap_or_default())
    }

    pub fn save(&self, input: &str) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
        let _guard = self
            .backtest_provider_lock
            .write()
            .unwrap_or_else(|error| error.into_inner());
        let current = self.stored_provider()?;
        let next = parse_market_data_provider(input)?;
        if next == current {
            return Ok(next);
        }
        // Prepare the backtest runtime before the atomic persistence step.
        // Preparation can probe provider health, so it must not run against a
        // durable selection the runtime has not accepted: a failed preparation
        // leaves the stored provider untouched instead of needing an undo.
        if let Some(runtime) = &self.runtime
            && let Err(error) = runtime.prepare_backtest(next)
        {
            return Err(MarketDataProviderSettingsError::Runtime(error));
        }
        self.store.save_backtest_market_data_provider(next)?;
        Ok(next)
    }
}

impl MarketDataProviderSettingsService {
    pub fn new(store: Arc<dyn MarketDataProviderSettingsStorePort>) -> Self {
        Self {
            store,
            runtime: None,
            provider_lock: Arc::new(RwLock::new(())),
        }
    }

    pub fn with_runtime(mut self, runtime: Arc<dyn MarketDataProviderRuntimePort>) -> Self {
        self.runtime = Some(runtime);
        self
    }

    pub fn active_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        let _guard = self
            .provider_lock
            .read()
            .unwrap_or_else(|error| error.into_inner());
        self.stored_provider()
    }

    fn stored_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        Ok(self
            .store
            .load_active_market_data_provider()?
            .as_deref()
            .map(normalize_market_data_provider)
            .unwrap_or_default())
    }

    pub fn save(&self, input: &str) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
        let _guard = self
            .provider_lock
            .write()
            .unwrap_or_else(|error| error.into_inner());
        let current = self.stored_provider()?;
        let next = parse_market_data_provider(input)?;
        self.store.save_active_market_data_provider(next)?;
        let Some(runtime) = &self.runtime else {
            return Ok(next);
        };
        if next == current && !runtime.needs_activation(next) {
            return Ok(next);
        }
        if let Err(error) = runtime.activate(next) {
            if next != current
                && let Err(rollback_error) = self.store.save_active_market_data_provider(current)
            {
                return Err(MarketDataProviderSettingsError::Runtime(format!(
                    "{error}; settings rollback failed: {rollback_error}"
                )));
            }
            return Err(MarketDataProviderSettingsError::Runtime(error));
        }
        Ok(next)
    }
}

pub fn normalize_market_data_provider(input: &str) -> MarketDataProvider {
    match input.trim().to_ascii_lowercase().as_str() {
        "futu" => MarketDataProvider::Futu,
        "yfinance" => MarketDataProvider::Yfinance,
        "akshare" => MarketDataProvider::Akshare,
        _ => MarketDataProvider::default(),
    }
}

pub fn parse_market_data_provider(
    input: &str,
) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
    match input.trim().to_ascii_lowercase().as_str() {
        "futu" => Ok(MarketDataProvider::Futu),
        "yfinance" => Ok(MarketDataProvider::Yfinance),
        "akshare" => Ok(MarketDataProvider::Akshare),
        _ => Err(MarketDataProviderSettingsError::Invalid),
    }
}

pub const fn provider_id(provider: MarketDataProvider) -> &'static str {
    match provider {
        MarketDataProvider::Futu => "futu",
        MarketDataProvider::Yfinance => "yfinance",
        MarketDataProvider::Akshare => "akshare",
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::sync::mpsc::{Receiver, Sender, channel};
    use std::sync::{Mutex, RwLock};
    use std::time::Duration;

    use super::*;

    struct Store(RwLock<Option<String>>);

    struct BacktestStore(RwLock<Option<String>>);

    struct Runtime {
        calls: Mutex<Vec<(String, MarketDataProvider)>>,
        fail: bool,
    }

    #[derive(Default)]
    struct DegradedRuntime {
        activations: Mutex<Vec<MarketDataProvider>>,
        degraded: AtomicBool,
        fail: AtomicBool,
    }

    impl DegradedRuntime {
        fn activations(&self) -> Vec<MarketDataProvider> {
            self.activations
                .lock()
                .expect("recorded activations")
                .clone()
        }

        fn mark_degraded(&self) {
            self.degraded.store(true, Ordering::SeqCst);
        }

        fn mark_healthy(&self) {
            self.degraded.store(false, Ordering::SeqCst);
        }

        fn start_failing(&self) {
            self.fail.store(true, Ordering::SeqCst);
        }
    }

    impl MarketDataProviderRuntimePort for DegradedRuntime {
        fn needs_activation(&self, provider: MarketDataProvider) -> bool {
            self.degraded.load(Ordering::SeqCst) && provider == MarketDataProvider::Akshare
        }

        fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
            self.activations
                .lock()
                .expect("recorded activations")
                .push(provider);
            if self.fail.load(Ordering::SeqCst) {
                Err("retry failed".to_owned())
            } else {
                Ok(())
            }
        }

        fn prepare_backtest(&self, _provider: MarketDataProvider) -> Result<(), String> {
            Ok(())
        }
    }

    struct RecordingPreparation {
        store: Arc<BacktestStore>,
        observed: Mutex<Vec<Option<String>>>,
        fail: bool,
    }

    impl MarketDataProviderRuntimePort for RecordingPreparation {
        fn needs_activation(&self, _provider: MarketDataProvider) -> bool {
            true
        }

        fn activate(&self, _provider: MarketDataProvider) -> Result<(), String> {
            Ok(())
        }

        fn prepare_backtest(&self, _provider: MarketDataProvider) -> Result<(), String> {
            self.observed
                .lock()
                .expect("observed")
                .push(self.store.0.read().expect("observed provider").clone());
            if self.fail {
                Err("provider health failed".to_owned())
            } else {
                Ok(())
            }
        }
    }

    struct ScriptedActiveStore {
        provider: RwLock<Option<String>>,
        save_results: Mutex<Vec<Result<(), SettingsStoreError>>>,
    }

    impl ScriptedActiveStore {
        fn new(provider: Option<&str>) -> Self {
            Self {
                provider: RwLock::new(provider.map(ToOwned::to_owned)),
                save_results: Mutex::new(Vec::new()),
            }
        }

        fn script(&self, results: Vec<Result<(), SettingsStoreError>>) {
            *self.save_results.lock().expect("save script") = results;
        }

        fn provider(&self) -> Option<String> {
            self.provider.read().expect("stored provider").clone()
        }
    }

    impl MarketDataProviderSettingsStorePort for ScriptedActiveStore {
        fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
            Ok(self.provider())
        }

        fn save_active_market_data_provider(
            &self,
            provider: MarketDataProvider,
        ) -> Result<(), SettingsStoreError> {
            let mut results = self.save_results.lock().expect("save script");
            if !results.is_empty() {
                results.remove(0)?;
            }
            *self.provider.write().expect("stored provider") =
                Some(provider_id(provider).to_owned());
            Ok(())
        }
    }

    struct BlockingRuntime {
        started: Sender<()>,
        release: Mutex<Receiver<()>>,
    }

    impl MarketDataProviderRuntimePort for BlockingRuntime {
        fn needs_activation(&self, _provider: MarketDataProvider) -> bool {
            true
        }

        fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
            assert_eq!(provider, MarketDataProvider::Futu);
            self.started.send(()).expect("signal side effect");
            self.release
                .lock()
                .expect("release receiver")
                .recv()
                .expect("release side effect");
            Err("provider switch failed".to_owned())
        }

        fn prepare_backtest(&self, _provider: MarketDataProvider) -> Result<(), String> {
            Ok(())
        }
    }

    impl MarketDataProviderSettingsStorePort for Store {
        fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
            self.0
                .read()
                .map(|value| value.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_active_market_data_provider(
            &self,
            provider: MarketDataProvider,
        ) -> Result<(), SettingsStoreError> {
            self.0
                .write()
                .map(|mut value| *value = Some(provider_id(provider).to_owned()))
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    impl BacktestMarketDataProviderSettingsStorePort for BacktestStore {
        fn load_backtest_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
            self.0
                .read()
                .map(|value| value.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_backtest_market_data_provider(
            &self,
            provider: MarketDataProvider,
        ) -> Result<(), SettingsStoreError> {
            self.0
                .write()
                .map(|mut value| *value = Some(provider_id(provider).to_owned()))
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    impl MarketDataProviderRuntimePort for Runtime {
        fn needs_activation(&self, _provider: MarketDataProvider) -> bool {
            true
        }

        fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
            self.calls
                .lock()
                .expect("runtime calls")
                .push(("activate".to_owned(), provider));
            if self.fail {
                Err("activation failed".to_owned())
            } else {
                Ok(())
            }
        }

        fn prepare_backtest(&self, provider: MarketDataProvider) -> Result<(), String> {
            self.calls
                .lock()
                .expect("runtime calls")
                .push(("prepare".to_owned(), provider));
            if self.fail {
                Err("preparation failed".to_owned())
            } else {
                Ok(())
            }
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/servercore/settings_market_data_test.go:11 TestServerSettingsStoreDefaultsAndPersistsMarketDataSelection
    #[test]
    fn provider_normalization_matches_current_go_defaults() {
        assert_eq!(
            normalize_market_data_provider(" FUTU "),
            MarketDataProvider::Futu
        );
        assert_eq!(
            normalize_market_data_provider(" yfinance "),
            MarketDataProvider::Yfinance
        );
        assert_eq!(
            normalize_market_data_provider("unknown"),
            MarketDataProvider::Akshare
        );
        let service = MarketDataProviderSettingsService::new(Arc::new(Store(RwLock::new(None))));
        assert_eq!(
            service.active_provider().expect("active provider"),
            MarketDataProvider::Akshare
        );
        assert_eq!(
            service.save(" YFINANCE ").expect("save provider"),
            MarketDataProvider::Yfinance
        );
        assert_eq!(
            service.save("invalid"),
            Err(MarketDataProviderSettingsError::Invalid)
        );
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/assistant_provider_test.go:114 TestSelectAssistantMarketProviderPersistsScopeAndReturnsBeforeAfter
    /// A blank or whitespace-only provider selection is rejected before the
    /// durable write, so the stored selection keeps its previous value.
    #[test]
    fn blank_provider_selection_is_rejected_before_persistence() {
        let store = Arc::new(Store(RwLock::new(Some("futu".to_owned()))));
        let service = MarketDataProviderSettingsService::new(
            Arc::clone(&store) as Arc<dyn MarketDataProviderSettingsStorePort>
        );
        for input in ["", "   ", "\t"] {
            assert_eq!(
                service.save(input),
                Err(MarketDataProviderSettingsError::Invalid),
                "input {input:?}"
            );
        }
        assert_eq!(
            store.0.read().expect("stored provider").as_deref(),
            Some("futu"),
            "blank selections must not overwrite the stored provider"
        );
    }

    #[test]
    fn active_failure_rolls_back_but_backtest_failure_never_persists() {
        let active_store = Arc::new(Store(RwLock::new(Some("yfinance".to_owned()))));
        let runtime = Arc::new(Runtime {
            calls: Mutex::new(Vec::new()),
            fail: true,
        });
        let active = MarketDataProviderSettingsService::new(active_store.clone())
            .with_runtime(runtime.clone());
        assert!(matches!(
            active.save("futu"),
            Err(MarketDataProviderSettingsError::Runtime(_))
        ));
        assert_eq!(
            active_store.0.read().expect("active store").as_deref(),
            Some("yfinance")
        );

        let backtest_store = Arc::new(BacktestStore(RwLock::new(Some("akshare".to_owned()))));
        let backtest = BacktestMarketDataProviderSettingsService::new(backtest_store.clone())
            .with_runtime(runtime);
        assert!(matches!(
            backtest.save("futu"),
            Err(MarketDataProviderSettingsError::Runtime(_))
        ));
        assert_eq!(
            backtest_store.0.read().expect("backtest store").as_deref(),
            Some("akshare")
        );
    }

    // Parity: go:452dea11:internal/settings/market_data_test.go:99 TestMarketDataProviderRetriesDegradedCurrentSelection
    #[test]
    fn degraded_current_selection_is_reactivated_only_while_degraded() {
        let store = Arc::new(Store(RwLock::new(Some("akshare".to_owned()))));
        let runtime = Arc::new(DegradedRuntime::default());
        runtime.mark_degraded();
        let service =
            MarketDataProviderSettingsService::new(store.clone()).with_runtime(runtime.clone());

        assert_eq!(
            service.save("akshare").expect("retry current provider"),
            MarketDataProvider::Akshare
        );
        assert_eq!(runtime.activations(), vec![MarketDataProvider::Akshare]);

        // A healthy current selection is idempotent and never restarts the
        // runtime again.
        runtime.mark_healthy();
        assert_eq!(
            service.save("akshare").expect("healthy provider save"),
            MarketDataProvider::Akshare
        );
        assert_eq!(runtime.activations(), vec![MarketDataProvider::Akshare]);

        runtime.mark_degraded();
        runtime.start_failing();
        let error = service.save("akshare").expect_err("failed provider retry");
        assert!(error.to_string().contains("retry failed"));
        assert_eq!(
            store.0.read().expect("active store").as_deref(),
            Some("akshare")
        );
    }

    // Parity: go:452dea11:internal/settings/market_data_test.go:143 TestMarketDataProviderSettingsAcceptAKShare
    #[test]
    fn akshare_selection_is_accepted_and_applied() {
        let store = Arc::new(Store(RwLock::new(Some("yfinance".to_owned()))));
        let runtime = Arc::new(Runtime {
            calls: Mutex::new(Vec::new()),
            fail: false,
        });
        let service =
            MarketDataProviderSettingsService::new(store.clone()).with_runtime(runtime.clone());

        assert_eq!(
            service.save(" AKSHARE ").expect("AKShare provider save"),
            MarketDataProvider::Akshare
        );
        assert_eq!(
            store.0.read().expect("active store").as_deref(),
            Some("akshare")
        );
        assert_eq!(
            runtime.calls.lock().expect("runtime calls").clone(),
            vec![("activate".to_owned(), MarketDataProvider::Akshare)]
        );
    }

    // Parity: go:452dea11:internal/settings/market_data_test.go:161 TestBacktestProviderIsPreparedBeforeAtomicPersistence
    #[test]
    fn backtest_provider_is_prepared_before_atomic_persistence() {
        let store = Arc::new(BacktestStore(RwLock::new(Some("yfinance".to_owned()))));
        let failing = Arc::new(RecordingPreparation {
            store: Arc::clone(&store),
            observed: Mutex::new(Vec::new()),
            fail: true,
        });
        let service = BacktestMarketDataProviderSettingsService::new(store.clone())
            .with_runtime(failing.clone());

        let error = service.save("akshare").expect_err("failed preparation");
        assert!(error.to_string().contains("provider health failed"));
        assert_eq!(
            store.0.read().expect("backtest store").as_deref(),
            Some("yfinance")
        );
        assert_eq!(
            failing.observed.lock().expect("observed").clone(),
            vec![Some("yfinance".to_owned())]
        );

        let succeeding = Arc::new(RecordingPreparation {
            store: Arc::clone(&store),
            observed: Mutex::new(Vec::new()),
            fail: false,
        });
        let service = BacktestMarketDataProviderSettingsService::new(store.clone())
            .with_runtime(succeeding.clone());
        assert_eq!(
            service.save("akshare").expect("prepared provider save"),
            MarketDataProvider::Akshare
        );
        assert_eq!(
            store.0.read().expect("backtest store").as_deref(),
            Some("akshare")
        );
        assert_eq!(
            succeeding.observed.lock().expect("observed").clone(),
            vec![Some("yfinance".to_owned())]
        );
    }

    // Parity: go:452dea11:internal/settings/market_data_test.go:224 TestMarketDataProviderReportsPersistenceAndRollbackFailures
    #[test]
    fn persistence_and_rollback_failures_are_reported() {
        let store = Arc::new(ScriptedActiveStore::new(Some("yfinance")));
        store.script(vec![Err(SettingsStoreError::new("persist failed"))]);
        let service = MarketDataProviderSettingsService::new(store.clone());
        let error = service.save("futu").expect_err("persistence failure");
        assert!(error.to_string().contains("persist failed"));
        assert_eq!(store.provider(), Some("yfinance".to_owned()));

        let store = Arc::new(ScriptedActiveStore::new(Some("yfinance")));
        store.script(vec![
            Ok(()),
            Err(SettingsStoreError::new("rollback failed")),
        ]);
        let service =
            MarketDataProviderSettingsService::new(store.clone()).with_runtime(Arc::new(Runtime {
                calls: Mutex::new(Vec::new()),
                fail: true,
            }));
        let error = service.save("futu").expect_err("rollback failure");
        let message = error.to_string();
        assert!(message.contains("activation failed"), "{message}");
        assert!(message.contains("settings rollback failed"), "{message}");
        assert!(message.contains("rollback failed"), "{message}");
        assert_eq!(store.provider(), Some("futu".to_owned()));
    }

    // Parity: go:452dea11:internal/settings/market_data_test.go:252 TestMarketDataProviderReadsWaitForRuntimeRollback
    #[test]
    fn reads_wait_for_the_runtime_rollback_window() {
        let store = Arc::new(Store(RwLock::new(Some("yfinance".to_owned()))));
        let (started_tx, started_rx) = channel();
        let (release_tx, release_rx) = channel();
        let service = Arc::new(
            MarketDataProviderSettingsService::new(store.clone()).with_runtime(Arc::new(
                BlockingRuntime {
                    started: started_tx,
                    release: Mutex::new(release_rx),
                },
            )),
        );

        let saving = {
            let service = Arc::clone(&service);
            std::thread::spawn(move || service.save("futu"))
        };
        started_rx.recv().expect("side effect started");

        let (read_tx, read_rx) = channel();
        let reading = {
            let service = Arc::clone(&service);
            std::thread::spawn(move || {
                let provider = service.active_provider();
                let _ = read_tx.send(provider.as_ref().ok().copied());
                provider
            })
        };
        assert!(
            read_rx.recv_timeout(Duration::from_millis(50)).is_err(),
            "provider read completed while the runtime rollback was in flight"
        );

        release_tx.send(()).expect("release side effect");
        let save_error = saving
            .join()
            .expect("save thread")
            .expect_err("runtime failure");
        assert!(save_error.to_string().contains("provider switch failed"));
        assert_eq!(
            read_rx
                .recv_timeout(Duration::from_secs(5))
                .expect("provider read after rollback"),
            Some(MarketDataProvider::Yfinance)
        );
        assert_eq!(
            reading.join().expect("read thread").expect("read provider"),
            MarketDataProvider::Yfinance
        );
    }
}
