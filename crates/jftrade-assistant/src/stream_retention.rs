/// Frozen Go assistantmodel.DefaultRunTimeout.
pub const DEFAULT_RUN_TIMEOUT_MS: i64 = 1_800_000;

const STREAM_RETENTION_MS: i64 = 1_800_000;

/// Reconnect visibility for an active run with a valid start timestamp.
/// The caller owns timestamp decoding. Missing timestamps and terminal event
/// retention need their own record metadata and are not decided here.
pub fn active_run_stream_retention_expired(
    now_unix_ns: i128,
    started_unix_ns: i128,
    max_duration_ms: i64,
) -> bool {
    let timeout = if max_duration_ms > 0 {
        max_duration_ms
    } else {
        DEFAULT_RUN_TIMEOUT_MS
    };
    let deadline =
        started_unix_ns + (i128::from(timeout) + i128::from(STREAM_RETENTION_MS)) * 1_000_000;
    now_unix_ns > deadline
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn active_stream_retention_uses_frozen_timeout_and_strict_nanosecond_boundary() {
        let started = 1_000_000_000;
        let custom_ms = 10_000;
        let deadline = started + i128::from(custom_ms + STREAM_RETENTION_MS) * 1_000_000;
        assert!(!active_run_stream_retention_expired(
            deadline - 1,
            started,
            custom_ms
        ));
        assert!(!active_run_stream_retention_expired(
            deadline, started, custom_ms
        ));
        assert!(active_run_stream_retention_expired(
            deadline + 1,
            started,
            custom_ms
        ));
        for max_ms in [0, -1] {
            let deadline =
                started + i128::from(DEFAULT_RUN_TIMEOUT_MS + STREAM_RETENTION_MS) * 1_000_000;
            assert!(!active_run_stream_retention_expired(
                deadline, started, max_ms
            ));
            assert!(active_run_stream_retention_expired(
                deadline + 1,
                started,
                max_ms
            ));
        }
        assert!(!active_run_stream_retention_expired(
            deadline + 1,
            started,
            i64::MAX
        ));
    }
}
