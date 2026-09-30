//! Shared in-memory active market-data provider state.

use jftrade_settings::{MarketDataProvider, MarketDataProviderRuntimePort};
use std::sync::{Arc, Mutex, RwLock};

type Activation =
    dyn Fn(MarketDataProvider, Option<MarketDataProvider>) -> Result<(), String> + Send + Sync;
type ReadinessReader = dyn Fn() -> (bool, bool, bool) + Send + Sync;

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct ProviderRuntimeSnapshot {
    pub provider: Option<MarketDataProvider>,
    pub generation: u64,
    pub helper_ready: bool,
    pub opend_ready: bool,
    pub router_ready: bool,
    pub closing: bool,
    pub activated: bool,
}

#[derive(Clone, Default)]
pub struct ActiveProviderState {
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
                activated: false,
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

    pub fn snapshot(&self) -> ProviderRuntimeSnapshot {
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
        let snap = self.snapshot();
        if snap.provider != Some(provider) || !snap.activated {
            return true;
        }
        if provider == MarketDataProvider::Futu && !snap.opend_ready {
            return true;
        }
        false
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
        if previous == Some(provider)
            && self.snapshot().activated
            && (provider != MarketDataProvider::Futu || self.snapshot().opend_ready)
        {
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
        snapshot.activated = true;
        snapshot.generation = snapshot.generation.saturating_add(1);
        // `opend_ready` describes the physical OpenD session, not which
        // market-data provider currently owns catalog/quote reads.  The
        // production composition deliberately keeps a Futu trade session
        // alive while yfinance/AKShare is active, so deriving this bit from
        // the provider selection would make reconciliation and broker reads
        // disappear after a helper-provider switch.  Keep the last observed
        // readiness here; the dynamic readiness reader (or an explicit
        // `set_readiness` update) remains the sole source of physical state.
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

    #[test]
    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_akshare_test.go:11 TestRuntimeReusesSharedSidecarAcrossPythonProviders
    fn helper_provider_switch_preserves_observed_opend_trade_readiness() {
        // OpenD is a separate trade owner in production.  Switching market
        // data from yfinance to AKShare must not clear a healthy physical
        // OpenD session, otherwise reconciliation and broker projections are
        // incorrectly downgraded until the next external probe.
        let state = ActiveProviderState::new(Some(MarketDataProvider::Yfinance));
        state.set_readiness(true, true, true);

        state
            .activate(MarketDataProvider::Akshare)
            .expect("helper provider activation");

        let snapshot = state.snapshot();
        assert_eq!(snapshot.provider, Some(MarketDataProvider::Akshare));
        assert!(snapshot.opend_ready);
        assert_eq!(snapshot.generation, 1);
    }

    #[test]
    fn futu_switch_without_physical_probe_does_not_claim_opend_ready() {
        // A successful provider callback alone is not proof that OpenD's
        // physical session is connected.  Readiness must remain false until
        // the runtime probe/reader publishes it explicitly.
        let state = ActiveProviderState::new(Some(MarketDataProvider::Yfinance));
        state.set_readiness(true, false, true);

        state
            .activate(MarketDataProvider::Futu)
            .expect("futu provider activation");

        assert!(!state.snapshot().opend_ready);
    }
    #[test]
    fn rejected_activation_never_publishes_a_new_provider_snapshot() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:143
        // TestRuntimeRollsBackYFinanceWhenNonFutuProviderRetirementFails
        let attempts = Arc::new(AtomicUsize::new(0));
        let attempts_cb = Arc::clone(&attempts);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(
            Arc::new(move |provider, _previous| {
                attempts_cb.fetch_add(1, Ordering::SeqCst);
                if provider == MarketDataProvider::Yfinance {
                    Err("retirement of the previous provider failed".to_owned())
                } else {
                    Ok(())
                }
            }),
        );

        // A failure raised while retiring the previous owner must leave the
        // committed snapshot untouched; retrying the same rejected target has
        // to run the transition again instead of reporting a bogus success.
        assert!(state.activate(MarketDataProvider::Yfinance).is_err());
        assert_eq!(state.get(), Some(MarketDataProvider::Futu));
        assert_eq!(state.snapshot().generation, 0);
        assert!(!state.snapshot().activated);
        assert!(state.activate(MarketDataProvider::Yfinance).is_err());
        assert_eq!(attempts.load(Ordering::SeqCst), 2);
        assert_eq!(state.get(), Some(MarketDataProvider::Futu));

        // The rejected attempts must not wedge the state: committing the
        // already-selected provider re-runs the transition once and publishes
        // exactly one new snapshot generation.
        state
            .activate(MarketDataProvider::Futu)
            .expect("committing the current provider succeeds");
        let snapshot = state.snapshot();
        assert_eq!(snapshot.provider, Some(MarketDataProvider::Futu));
        assert!(snapshot.activated);
        assert_eq!(snapshot.generation, 1);
        assert_eq!(attempts.load(Ordering::SeqCst), 3);
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:191 TestRuntimeSidecarFailureAndCloseKeepSelectionStable
    #[test]
    fn activation_after_shutdown_is_rejected_and_leaves_the_snapshot_committed() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:414
        // TestRuntimeCannotRestartManagedSidecarAfterClose and
        // runtime_test.go:437 TestRuntimeCloseWinsConcurrentActivationWithoutRestartingSidecar
        let started = Arc::new(AtomicUsize::new(0));
        let started_cb = Arc::clone(&started);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(
            Arc::new(move |_provider, _previous| {
                started_cb.fetch_add(1, Ordering::SeqCst);
                Ok(())
            }),
        );

        state.begin_shutdown();
        assert!(state.activate(MarketDataProvider::Yfinance).is_err());
        assert!(state.activate(MarketDataProvider::Futu).is_err());
        assert_eq!(
            started.load(Ordering::SeqCst),
            0,
            "a closed runtime must never start another provider transition"
        );
        let snapshot = state.snapshot();
        assert_eq!(snapshot.provider, Some(MarketDataProvider::Futu));
        assert!(snapshot.closing);
        assert_eq!(snapshot.generation, 0);
    }

    #[test]
    fn queued_transitions_observe_the_committed_provider_as_their_previous_owner() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:476
        // TestRuntimeDoesNotCommitActivationCanceledWhileQueued and
        // runtime_test.go:337 TestRuntimeRestoresPreviousSubscriptionsAfterActivationFailure
        let seen = Arc::new(Mutex::new(Vec::new()));
        let seen_cb = Arc::clone(&seen);
        let state = Arc::new(
            ActiveProviderState::new(Some(MarketDataProvider::Yfinance)).with_activation(Arc::new(
                move |next, previous| {
                    seen_cb
                        .lock()
                        .unwrap_or_else(|error| error.into_inner())
                        .push((previous, next));
                    if next == MarketDataProvider::Akshare {
                        return Err("akshare rejected desired subscriptions".to_owned());
                    }
                    Ok(())
                },
            )),
        );

        let first = {
            let state = Arc::clone(&state);
            thread::spawn(move || state.activate(MarketDataProvider::Akshare))
        };
        let second = {
            let state = Arc::clone(&state);
            thread::spawn(move || state.activate(MarketDataProvider::Futu))
        };
        assert!(first.join().expect("first transition").is_err());
        second
            .join()
            .expect("second transition")
            .expect("futu activation");

        let transitions = seen.lock().unwrap_or_else(|error| error.into_inner());
        assert_eq!(transitions.len(), 2, "transitions = {transitions:?}");
        // Whichever thread wins the transition guard, every callback observes
        // the provider that was committed before it ran.  A rejected
        // transition never becomes the previous owner of the next one.
        // The first transition always starts from the committed provider.
        assert_eq!(transitions[0].0, Some(MarketDataProvider::Yfinance));
        // The second transition starts from whatever the first one committed:
        // Futu when the Futu transition won the guard, otherwise the original
        // provider because the rejected AKShare transition published nothing.
        let expected_previous = if transitions[0].1 == MarketDataProvider::Futu {
            MarketDataProvider::Futu
        } else {
            MarketDataProvider::Yfinance
        };
        assert_eq!(
            transitions[1].0,
            Some(expected_previous),
            "a rejected transition must never become the next previous owner"
        );
        assert_eq!(state.get(), Some(MarketDataProvider::Futu));
        assert_eq!(state.snapshot().generation, 1);
    }
    #[test]
    fn switch_retires_the_previous_provider_before_publishing_the_next_one() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:168
        // TestRuntimeRetiresPreviousSubscriptionsThroughBrokerReconciliation
        //
        // The previous owner is handed to the transition callback from the
        // committed snapshot, so reconciliation always retires the provider
        // that is actually live instead of a stale request target.
        let retired = Arc::new(Mutex::new(Vec::new()));
        let retired_cb = Arc::clone(&retired);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(
            Arc::new(move |next, previous| {
                if let Some(previous) = previous
                    && previous != next
                {
                    retired_cb
                        .lock()
                        .unwrap_or_else(|error| error.into_inner())
                        .push(previous);
                }
                Ok(())
            }),
        );

        state
            .activate(MarketDataProvider::Yfinance)
            .expect("switch to helper provider");
        assert_eq!(
            retired
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .as_slice(),
            &[MarketDataProvider::Futu],
            "the transition must hand over the previously committed provider"
        );
        assert_eq!(state.get(), Some(MarketDataProvider::Yfinance));
        assert_eq!(state.snapshot().generation, 1);

        // Re-selecting the already committed provider must not retire it
        // again: the reconciler would otherwise drop live subscriptions.
        state
            .activate(MarketDataProvider::Yfinance)
            .expect("same provider stays a no-op");
        assert_eq!(
            retired
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .len(),
            1,
            "re-selecting the committed provider must not retire it"
        );
        assert_eq!(state.snapshot().generation, 1);
    }

    #[test]
    fn provider_change_cannot_restore_subscriptions_after_shutdown() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:504
        // TestProviderChangeCannotRestoreSubscriptionsAfterRuntimeClose
        let reconciled = Arc::new(AtomicUsize::new(0));
        let reconciled_cb = Arc::clone(&reconciled);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(
            Arc::new(move |_next, _previous| {
                reconciled_cb.fetch_add(1, Ordering::SeqCst);
                Ok(())
            }),
        );

        state.begin_shutdown();
        assert!(state.activate(MarketDataProvider::Yfinance).is_err());
        assert_eq!(
            reconciled.load(Ordering::SeqCst),
            0,
            "a closed runtime must not restore or retire any subscription set"
        );
        assert_eq!(state.get(), Some(MarketDataProvider::Futu));
    }

    #[test]
    fn concurrent_close_and_activation_commit_exactly_one_outcome() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:437
        // TestRuntimeCloseWinsConcurrentActivationWithoutRestartingSidecar
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let barrier = Arc::new(std::sync::Barrier::new(3));
        let closer = {
            let state = Arc::clone(&state);
            let barrier = Arc::clone(&barrier);
            thread::spawn(move || {
                barrier.wait();
                state.begin_shutdown();
            })
        };
        let activator = {
            let state = Arc::clone(&state);
            let barrier = Arc::clone(&barrier);
            thread::spawn(move || {
                barrier.wait();
                state.activate(MarketDataProvider::Yfinance)
            })
        };
        barrier.wait();
        closer.join().expect("close thread");
        let activation = activator.join().expect("activation thread");

        let snapshot = state.snapshot();
        assert!(snapshot.closing, "close must win the race");
        match activation {
            Ok(()) => assert_eq!(
                snapshot.provider,
                Some(MarketDataProvider::Yfinance),
                "an activation that won before closing must stay published"
            ),
            Err(error) => {
                assert!(
                    error.contains("shutting down"),
                    "a rejected activation must report the closed runtime, got {error}"
                );
                assert_eq!(snapshot.provider, Some(MarketDataProvider::Futu));
            }
        }
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:140 TestApplyProviderSettingsRollsBackFailedFutuDemandRestore
    #[test]
    fn failed_provider_change_restores_the_previous_subscription_owner() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:337
        // TestRuntimeRestoresPreviousSubscriptionsAfterActivationFailure
        let restored = Arc::new(Mutex::new(Vec::new()));
        let restored_cb = Arc::clone(&restored);
        let state = Arc::new(
            ActiveProviderState::new(Some(MarketDataProvider::Yfinance)).with_activation(Arc::new(
                move |next, previous| {
                    if next == MarketDataProvider::Futu {
                        // The new provider rejected its desired subscriptions.
                        // Nothing may be published, and the previously
                        // committed owner stays authoritative.
                        return Err("new provider rejected subscriptions".to_owned());
                    }
                    if let Some(previous) = previous {
                        restored_cb
                            .lock()
                            .unwrap_or_else(|error| error.into_inner())
                            .push(previous);
                    }
                    Ok(())
                },
            )),
        );

        assert!(state.activate(MarketDataProvider::Futu).is_err());
        assert_eq!(state.get(), Some(MarketDataProvider::Yfinance));
        assert_eq!(state.snapshot().generation, 0);
        assert_eq!(
            restored
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .as_slice(),
            &[],
            "a rejected switch must not retire the previous owner"
        );

        // The kept owner can still serve its own transitions afterwards.
        state
            .activate(MarketDataProvider::Akshare)
            .expect("previous provider stays usable");
        assert_eq!(
            restored
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .as_slice(),
            &[MarketDataProvider::Yfinance]
        );
        assert_eq!(state.get(), Some(MarketDataProvider::Akshare));
    }

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_forwarding_test.go:166 TestRuntimeSameProviderActivationDoesNotReleasePhysicalSubscriptions
    #[test]
    fn same_provider_activation_is_idempotent_after_a_rejected_switch() {
        // Parity: go:452dea11:internal/app/apiserver/marketdataapp/runtime_test.go:14
        // TestRuntimeSwitchesStableDataPlaneBetweenFutuAndYFinance (same-provider no-op)
        let calls = Arc::new(AtomicUsize::new(0));
        let calls_cb = Arc::clone(&calls);
        let state = ActiveProviderState::new(Some(MarketDataProvider::Futu)).with_activation(
            Arc::new(move |next, _previous| {
                calls_cb.fetch_add(1, Ordering::SeqCst);
                if next == MarketDataProvider::Yfinance {
                    Err("helper runtime is not installed".to_owned())
                } else {
                    Ok(())
                }
            }),
        );

        assert!(state.activate(MarketDataProvider::Yfinance).is_err());
        let rejected = state.snapshot();
        assert!(!rejected.activated, "a rejected switch must not activate");
        assert_eq!(rejected.provider, Some(MarketDataProvider::Futu));
        assert_eq!(rejected.generation, 0);

        // The committed provider is still usable: activating it prepares the
        // runtime exactly once and publishes a single new generation.
        state
            .activate(MarketDataProvider::Futu)
            .expect("committed provider activation");
        let committed = state.snapshot();
        assert_eq!(committed.provider, Some(MarketDataProvider::Futu));
        assert!(committed.activated);
        assert_eq!(committed.generation, 1);
        assert_eq!(calls.load(Ordering::SeqCst), 2);

        // Once a helper provider is published, re-selecting it is a real
        // no-op: no new transition and no new generation.  (Futu is excluded
        // from this half because it re-probes OpenD while the physical
        // session is not proven ready, which is asserted separately by
        // needs_activation / futu_switch_without_physical_probe_*.)
        state
            .activate(MarketDataProvider::Akshare)
            .expect("switch to second helper provider");
        assert_eq!(state.snapshot().generation, 2);
        assert_eq!(calls.load(Ordering::SeqCst), 3);
        state
            .activate(MarketDataProvider::Akshare)
            .expect("re-selecting the committed helper provider is a no-op");
        assert_eq!(state.snapshot().generation, 2);
        assert_eq!(calls.load(Ordering::SeqCst), 3);
    }
}
