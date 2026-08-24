use std::sync::RwLock;

use jftrade_kernel::WireTimestamp;

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct CollectorRuntimeState {
    pub connected: bool,
    pub closed: bool,
    pub generation: u64,
    pub active_count: usize,
    pub last_refresh_at: Option<WireTimestamp>,
    pub quote_retry_at: Option<WireTimestamp>,
    pub quote_failures: usize,
    pub quote_last_error: Option<String>,
    pub stream_retry_at: Option<WireTimestamp>,
    pub stream_failures: usize,
    pub stream_last_error: Option<String>,
}

#[derive(Debug, Default)]
pub struct MarketDataRuntimeRecorder {
    inner: RwLock<RecorderState>,
}

#[derive(Clone, Debug, Default)]
struct RecorderState {
    runtime: CollectorRuntimeState,
    active_instruments: Vec<String>,
}

impl MarketDataRuntimeRecorder {
    pub fn reconcile(&self, instruments: impl IntoIterator<Item = String>) -> u64 {
        let instruments = normalize_instruments(instruments);
        let mut state = write_state(&self.inner);
        if state.runtime.closed || state.active_instruments == instruments {
            return state.runtime.generation;
        }
        state.active_instruments = instruments;
        let active_count = state.active_instruments.len();
        state.runtime.generation = state.runtime.generation.saturating_add(1);
        reset_generation(&mut state.runtime, active_count);
        state.runtime.generation
    }

    pub fn reset(&self) -> u64 {
        let mut state = write_state(&self.inner);
        if state.runtime.closed {
            return state.runtime.generation;
        }
        state.active_instruments.clear();
        state.runtime.generation = state.runtime.generation.saturating_add(1);
        reset_generation(&mut state.runtime, 0);
        state.runtime.generation
    }

    pub fn record_poll_started(&self, generation: u64, now: WireTimestamp) -> bool {
        update_generation(&self.inner, generation, |state| {
            state.last_refresh_at = Some(utc(now));
        })
    }

    pub fn record_quote_success(&self, generation: u64) -> bool {
        update_generation(&self.inner, generation, |state| {
            state.quote_failures = 0;
            state.quote_retry_at = None;
            state.quote_last_error = None;
        })
    }

    pub fn record_quote_failure(
        &self,
        generation: u64,
        now: WireTimestamp,
        error: impl Into<String>,
    ) -> bool {
        update_generation(&self.inner, generation, |state| {
            state.quote_retry_at = Some(retry_at(now, state.quote_failures));
            state.quote_failures = state.quote_failures.saturating_add(1);
            state.quote_last_error = Some(error.into());
        })
    }

    pub fn record_stream_connected(&self, generation: u64) -> bool {
        update_generation(&self.inner, generation, |state| {
            state.connected = true;
            state.stream_failures = 0;
            state.stream_retry_at = None;
            state.stream_last_error = None;
        })
    }

    pub fn record_stream_failure(
        &self,
        generation: u64,
        now: WireTimestamp,
        error: impl Into<String>,
    ) -> bool {
        update_generation(&self.inner, generation, |state| {
            state.connected = false;
            state.stream_retry_at = Some(retry_at(now, state.stream_failures));
            state.stream_failures = state.stream_failures.saturating_add(1);
            state.stream_last_error = Some(error.into());
        })
    }

    pub fn close(&self) -> u64 {
        let mut state = write_state(&self.inner);
        if !state.runtime.closed {
            state.runtime.closed = true;
            state.runtime.connected = false;
            state.runtime.generation = state.runtime.generation.saturating_add(1);
        }
        state.runtime.generation
    }

    pub fn snapshot(&self) -> CollectorRuntimeState {
        read_state(&self.inner).runtime.clone()
    }
}

fn update_generation(
    inner: &RwLock<RecorderState>,
    generation: u64,
    update: impl FnOnce(&mut CollectorRuntimeState),
) -> bool {
    let mut state = write_state(inner);
    if state.runtime.closed || state.runtime.generation != generation {
        return false;
    }
    update(&mut state.runtime);
    true
}

fn reset_generation(state: &mut CollectorRuntimeState, active_count: usize) {
    let generation = state.generation;
    *state = CollectorRuntimeState {
        generation,
        active_count,
        ..CollectorRuntimeState::default()
    };
}

fn normalize_instruments(instruments: impl IntoIterator<Item = String>) -> Vec<String> {
    let mut instruments = instruments
        .into_iter()
        .map(|instrument| instrument.trim().to_ascii_uppercase())
        .filter(|instrument| !instrument.is_empty())
        .collect::<Vec<_>>();
    instruments.sort();
    instruments.dedup();
    instruments
}

fn retry_at(now: WireTimestamp, failures: usize) -> WireTimestamp {
    const RETRY_SECONDS: [i64; 4] = [5, 10, 20, 30];
    let delay = RETRY_SECONDS[failures.min(RETRY_SECONDS.len() - 1)];
    WireTimestamp::from_offset_datetime(
        now.into_inner().to_offset(time::UtcOffset::UTC) + time::Duration::seconds(delay),
    )
}

fn utc(timestamp: WireTimestamp) -> WireTimestamp {
    WireTimestamp::from_offset_datetime(timestamp.into_inner().to_offset(time::UtcOffset::UTC))
}

fn read_state(inner: &RwLock<RecorderState>) -> std::sync::RwLockReadGuard<'_, RecorderState> {
    inner.read().unwrap_or_else(|error| error.into_inner())
}

fn write_state(inner: &RwLock<RecorderState>) -> std::sync::RwLockWriteGuard<'_, RecorderState> {
    inner.write().unwrap_or_else(|error| error.into_inner())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recorder_matches_generation_retry_recovery_and_close_rules() {
        let recorder = MarketDataRuntimeRecorder::default();
        let first = recorder.reconcile([
            " us.aapl ".to_owned(),
            "HK.00700".to_owned(),
            "US.AAPL".to_owned(),
        ]);
        assert_eq!(first, 1);
        assert_eq!(recorder.snapshot().active_count, 2);
        assert_eq!(
            recorder.reconcile(["HK.00700".to_owned(), "US.AAPL".to_owned()]),
            first
        );

        let now: WireTimestamp = "2026-08-24T09:00:00+08:00".parse().expect("timestamp");
        assert!(recorder.record_poll_started(first, now));
        assert!(recorder.record_quote_failure(first, now, " quote down "));
        let failed = recorder.snapshot();
        assert_eq!(
            failed.last_refresh_at.expect("refresh").to_string(),
            "2026-08-24T01:00:00Z"
        );
        assert_eq!(
            failed.quote_retry_at.expect("retry").to_string(),
            "2026-08-24T01:00:05Z"
        );
        assert_eq!(failed.quote_failures, 1);

        let second = recorder.reconcile(["CN.600000".to_owned()]);
        assert_eq!(second, 2);
        assert!(!recorder.record_stream_connected(first));
        assert!(recorder.record_stream_failure(second, now, "stream down"));
        assert!(recorder.record_stream_connected(second));
        let recovered = recorder.snapshot();
        assert!(recovered.connected);
        assert_eq!(recovered.stream_failures, 0);
        assert_eq!(recovered.stream_last_error, None);

        assert_eq!(recorder.close(), 3);
        assert_eq!(recorder.close(), 3);
        assert!(!recorder.record_quote_success(second));
        let closed = recorder.snapshot();
        assert!(closed.closed);
        assert!(!closed.connected);
    }

    #[test]
    fn retry_delay_is_capped_after_four_failures() {
        let recorder = MarketDataRuntimeRecorder::default();
        let generation = recorder.reconcile(["US.AAPL".to_owned()]);
        let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
        for expected in [5, 10, 20, 30, 30] {
            assert!(recorder.record_quote_failure(generation, now, "down"));
            let retry = recorder.snapshot().quote_retry_at.expect("retry");
            assert_eq!(
                retry.into_inner().unix_timestamp(),
                now.into_inner().unix_timestamp() + expected
            );
        }
    }
}
