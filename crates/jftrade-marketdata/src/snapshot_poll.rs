use std::collections::{BTreeMap, BTreeSet};

use jftrade_kernel::WireTimestamp;

use crate::{CacheLookup, InstrumentRef, MarketDataRuntimeRecorder, Tick, TickCache};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct SnapshotPollPolicy {
    pub interval_ms: i64,
    pub freshness_ms: i64,
}

impl Default for SnapshotPollPolicy {
    fn default() -> Self {
        Self {
            interval_ms: 1_000,
            freshness_ms: 1_500,
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SnapshotPollSkipReason {
    EmptyDemand,
    Fresh,
    Cadence,
    RetryWindow,
    InactiveGeneration,
    Closed,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum SnapshotPollOutcome {
    Skipped(SnapshotPollSkipReason),
    Applied {
        requested: usize,
        inserted: usize,
    },
    Failed {
        requested: usize,
        error: String,
    },
    StaleGeneration {
        requested_generation: u64,
        active_generation: u64,
    },
}

/// Executes one broker-neutral snapshot poll against an injected query.
///
/// This seam owns no timer, task, provider activation, transport or product
/// composition. The caller supplies the active generation and query operation;
/// the executor only applies Go-compatible cadence, freshness, retry and stale
/// completion fencing to the shared runtime recorder and tick cache.
#[derive(Clone, Copy, Debug, Default)]
pub struct SnapshotPollExecutor {
    policy: SnapshotPollPolicy,
}

impl SnapshotPollExecutor {
    pub fn new(policy: SnapshotPollPolicy) -> Self {
        let defaults = SnapshotPollPolicy::default();
        Self {
            policy: SnapshotPollPolicy {
                interval_ms: positive_or(policy.interval_ms, defaults.interval_ms),
                freshness_ms: positive_or(policy.freshness_ms, defaults.freshness_ms),
            },
        }
    }

    pub fn execute(
        &self,
        recorder: &MarketDataRuntimeRecorder,
        cache: &mut TickCache,
        demand: &[InstrumentRef],
        generation: u64,
        now: WireTimestamp,
        query: impl FnOnce(&[String]) -> Result<Vec<Tick>, String>,
    ) -> SnapshotPollOutcome {
        let instruments = normalized_instruments(demand);
        if instruments.is_empty() {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::EmptyDemand);
        }
        let runtime = recorder.snapshot();
        let now_ms = timestamp_ms(now);
        if runtime.closed {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Closed);
        }
        if runtime.generation != generation {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::InactiveGeneration);
        }
        if instruments.iter().all(|instrument| {
            matches!(
                cache.lookup_for_generation(
                    instrument,
                    now_ms,
                    self.policy.freshness_ms,
                    runtime.generation,
                ),
                CacheLookup::Fresh(_)
            )
        }) {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Fresh);
        }
        if runtime
            .quote_retry_at
            .is_some_and(|retry_at| retry_at.into_inner() > now.into_inner())
        {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::RetryWindow);
        }
        if runtime
            .last_refresh_at
            .is_some_and(|last| now_ms.saturating_sub(timestamp_ms(last)) < self.policy.interval_ms)
        {
            return SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Cadence);
        }
        if !recorder.record_poll_started(generation, now) {
            return stale_outcome(recorder, generation);
        }

        match query(&instruments) {
            Ok(ticks) => self.apply_success(recorder, cache, &instruments, generation, now, ticks),
            Err(error) => {
                if recorder.snapshot().generation != generation {
                    return stale_outcome(recorder, generation);
                }
                if !recorder.record_quote_failure(generation, now, error.clone()) {
                    return stale_outcome(recorder, generation);
                }
                SnapshotPollOutcome::Failed {
                    requested: instruments.len(),
                    error,
                }
            }
        }
    }

    fn apply_success(
        &self,
        recorder: &MarketDataRuntimeRecorder,
        cache: &mut TickCache,
        instruments: &[String],
        generation: u64,
        now: WireTimestamp,
        ticks: Vec<Tick>,
    ) -> SnapshotPollOutcome {
        if recorder.snapshot().generation != generation {
            return stale_outcome(recorder, generation);
        }
        let demanded = instruments.iter().cloned().collect::<BTreeSet<_>>();
        let mut returned = BTreeMap::new();
        for mut tick in ticks {
            let instrument = tick.instrument_id.trim().to_ascii_uppercase();
            if demanded.contains(&instrument) {
                tick.instrument_id = instrument.clone();
                returned.insert(instrument, tick);
            }
        }
        if returned
            .values()
            .any(|tick| tick.provider_generation != generation)
        {
            return stale_outcome(recorder, generation);
        }
        let mut next_cache = cache.clone();
        let mut inserted = 0;
        for tick in returned.into_values() {
            if let Err(error) = next_cache.insert(tick, generation) {
                let error = error.to_string();
                let _ = recorder.record_quote_failure(generation, now, error.clone());
                return SnapshotPollOutcome::Failed {
                    requested: instruments.len(),
                    error,
                };
            }
            inserted += 1;
        }
        if !recorder.record_quote_success(generation) {
            return stale_outcome(recorder, generation);
        }
        *cache = next_cache;
        SnapshotPollOutcome::Applied {
            requested: instruments.len(),
            inserted,
        }
    }
}

fn normalized_instruments(demand: &[InstrumentRef]) -> Vec<String> {
    let mut instruments = demand
        .iter()
        .filter_map(|reference| reference.clone().normalize().ok())
        .map(|reference| reference.instrument_id())
        .collect::<Vec<_>>();
    instruments.sort();
    instruments.dedup();
    instruments
}

fn stale_outcome(
    recorder: &MarketDataRuntimeRecorder,
    requested_generation: u64,
) -> SnapshotPollOutcome {
    SnapshotPollOutcome::StaleGeneration {
        requested_generation,
        active_generation: recorder.snapshot().generation,
    }
}

fn timestamp_ms(timestamp: WireTimestamp) -> i64 {
    let value = timestamp.into_inner();
    value
        .unix_timestamp()
        .saturating_mul(1_000)
        .saturating_add(i64::from(value.nanosecond() / 1_000_000))
}

fn positive_or(value: i64, fallback: i64) -> i64 {
    if value > 0 { value } else { fallback }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use jftrade_kernel::Fixed8;

    use super::*;

    fn reference(market: &str, symbol: &str) -> InstrumentRef {
        InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: market.to_owned(),
            symbol: symbol.to_owned(),
            interval: None,
        }
    }

    fn tick(instrument_id: &str, observed_at_ms: i64, generation: u64) -> Tick {
        Tick {
            instrument_id: instrument_id.to_owned(),
            price: Fixed8::from_scaled(18_850_000_000),
            volume: "10".parse().expect("volume"),
            observed_at_ms,
            provider_generation: generation,
        }
    }

    #[test]
    fn poll_normalizes_demand_writes_only_requested_ticks_and_skips_fresh_cache() {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let generation = recorder.reconcile(["US.AAPL".to_owned(), "HK.00700".to_owned()]);
        let mut cache = TickCache::new(2);
        let executor = SnapshotPollExecutor::default();
        let demand = [
            reference(" us ", "aapl"),
            reference("HK", "00700"),
            reference("US", "AAPL"),
        ];
        let now: WireTimestamp = "2026-08-24T00:00:01Z".parse().expect("timestamp");
        let now_ms = timestamp_ms(now);

        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &demand,
                generation,
                now,
                |instruments| {
                    assert_eq!(instruments, ["HK.00700", "US.AAPL"]);
                    Ok(vec![
                        tick("us.aapl", now_ms, generation),
                        tick("US.MSFT", now_ms, generation),
                    ])
                },
            ),
            SnapshotPollOutcome::Applied {
                requested: 2,
                inserted: 1,
            }
        );
        assert_eq!(cache.instrument_count(), 1);
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &[reference("US", "AAPL")],
                generation,
                now,
                |_| panic!("fresh cache must suppress the query"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Fresh)
        );
    }

    #[test]
    fn missing_cache_poll_respects_one_second_cadence() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let mut cache = TickCache::new(1);
        let executor = SnapshotPollExecutor::default();
        let demand = [reference("US", "AAPL")];
        let first: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, first, |_| Ok(
                Vec::new()
            ),),
            SnapshotPollOutcome::Applied {
                requested: 1,
                inserted: 0,
            }
        );
        let early: WireTimestamp = "2026-08-24T00:00:00.999Z".parse().expect("timestamp");
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &demand,
                generation,
                early,
                |_| panic!("cadence must suppress the query"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Cadence)
        );
        let due: WireTimestamp = "2026-08-24T00:00:01Z".parse().expect("timestamp");
        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, due, |_| Ok(
                Vec::new()
            ),),
            SnapshotPollOutcome::Applied {
                requested: 1,
                inserted: 0,
            }
        );
    }

    #[test]
    fn non_positive_policy_values_retain_collector_defaults() {
        assert_eq!(
            SnapshotPollExecutor::new(SnapshotPollPolicy {
                interval_ms: 0,
                freshness_ms: -1,
            })
            .policy,
            SnapshotPollPolicy::default()
        );
    }

    #[test]
    fn failure_preserves_cache_and_enforces_capped_recorder_retry_window() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let mut cache = TickCache::new(2);
        cache
            .insert(tick("US.AAPL", 0, generation), generation)
            .expect("seed cache");
        let executor = SnapshotPollExecutor::default();
        let now: WireTimestamp = "2026-08-24T00:00:02Z".parse().expect("timestamp");
        let demand = [reference("US", "AAPL")];

        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, now, |_| Err(
                "quote unavailable".to_owned()
            ),),
            SnapshotPollOutcome::Failed {
                requested: 1,
                error: "quote unavailable".to_owned(),
            }
        );
        assert_eq!(cache.instrument_count(), 1);
        let before_retry: WireTimestamp = "2026-08-24T00:00:06.999Z".parse().expect("timestamp");
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &demand,
                generation,
                before_retry,
                |_| panic!("retry window must suppress the query"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::RetryWindow)
        );
        let at_retry: WireTimestamp = "2026-08-24T00:00:07Z".parse().expect("timestamp");
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &demand,
                generation,
                at_retry,
                |_| Ok(vec![tick("US.AAPL", timestamp_ms(at_retry), generation)]),
            ),
            SnapshotPollOutcome::Applied {
                requested: 1,
                inserted: 1,
            }
        );
        assert_eq!(recorder.snapshot().quote_failures, 0);
    }

    #[test]
    fn completion_after_generation_change_cannot_mutate_cache_or_failure_state() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let mut cache = TickCache::new(2);
        let executor = SnapshotPollExecutor::default();
        let now: WireTimestamp = "2026-08-24T00:00:02Z".parse().expect("timestamp");

        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &[reference("US", "AAPL")],
                generation,
                now,
                |_| {
                    recorder.reconfigure();
                    Ok(vec![tick("US.AAPL", timestamp_ms(now), generation)])
                },
            ),
            SnapshotPollOutcome::StaleGeneration {
                requested_generation: generation,
                active_generation: generation + 1,
            }
        );
        assert_eq!(cache.instrument_count(), 0);
        assert_eq!(recorder.snapshot().quote_failures, 0);
    }

    #[test]
    fn stale_caller_is_rejected_even_when_the_active_generation_cache_is_fresh() {
        let recorder = MarketDataRuntimeRecorder::default();
        let stale_generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let active_generation = recorder.reconfigure();
        let now: WireTimestamp = "2026-08-24T00:00:02Z".parse().expect("timestamp");
        let mut cache = TickCache::new(1);
        cache
            .insert(
                tick("US.AAPL", timestamp_ms(now), active_generation),
                active_generation,
            )
            .expect("active-generation cache");

        assert_eq!(
            SnapshotPollExecutor::default().execute(
                &recorder,
                &mut cache,
                &[reference("US", "AAPL")],
                stale_generation,
                now,
                |_| panic!("stale caller must not query"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::InactiveGeneration)
        );
    }

    #[test]
    fn empty_invalid_closed_and_inactive_demand_never_calls_provider() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let mut cache = TickCache::new(1);
        let executor = SnapshotPollExecutor::default();
        let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        let invalid = [reference("", "")];
        assert_eq!(
            executor.execute(&recorder, &mut cache, &invalid, generation, now, |_| {
                panic!("invalid demand must not call provider")
            }),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::EmptyDemand)
        );
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &[reference("US", "AAPL")],
                generation + 1,
                now,
                |_| panic!("inactive generation must not call provider"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::InactiveGeneration)
        );
        recorder.close();
        assert_eq!(
            executor.execute(
                &recorder,
                &mut cache,
                &[reference("US", "AAPL")],
                recorder.snapshot().generation,
                now,
                |_| panic!("closed recorder must not call provider"),
            ),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Closed)
        );
    }

    #[test]
    fn fresh_cache_precedes_retry_while_closed_and_generation_guards_stay_authoritative() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let mut cache = TickCache::new(1);
        let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        cache
            .insert(tick("US.AAPL", timestamp_ms(now), generation), generation)
            .expect("seed cache");
        let executor = SnapshotPollExecutor::default();
        let demand = [reference("US", "AAPL")];

        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, now, |_| {
                panic!("fresh cache must suppress the query")
            }),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Fresh)
        );
        assert!(recorder.record_quote_failure(generation, now, "temporary"));
        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, now, |_| {
                panic!("fresh cache must precede retry window")
            }),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Fresh)
        );
        recorder.close();
        assert_eq!(
            executor.execute(&recorder, &mut cache, &demand, generation, now, |_| {
                panic!("closed recorder must suppress the query")
            }),
            SnapshotPollOutcome::Skipped(SnapshotPollSkipReason::Closed)
        );
    }
}
