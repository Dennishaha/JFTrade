fn research_read_snapshot_failure(error: ResearchReadSnapshotError) -> ApiFailure {
    match error {
        ResearchReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
        ResearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let failure = ApiFailure::new(status, code, message);
            match retry_after_seconds {
                Some(seconds) => failure.with_retry_after(seconds),
                None => failure,
            }
        }
        ResearchReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "RESEARCH_UNAVAILABLE", message)
        }
    }
}

#[cfg(test)]
#[test]
fn research_read_failure_preserves_remote_error_and_retry_after() {
    let failure = research_read_snapshot_failure(ResearchReadSnapshotError::Failed {
        status: 429,
        code: "RATE_LIMITED".to_owned(),
        message: "research helper is busy".to_owned(),
        retry_after_seconds: Some(7),
    });
    assert_eq!(failure.status, 429);
    assert_eq!(failure.code, "RATE_LIMITED");
    assert_eq!(failure.message, "research helper is busy");
    assert_eq!(failure.retry_after_seconds, Some(7));
}

fn research_preset_read_snapshot_failure(error: ResearchPresetReadSnapshotError) -> ApiFailure {
    match error {
        ResearchPresetReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "RESEARCH_PRESET_INVALID", message)
        }
        ResearchPresetReadSnapshotError::NotFound => ApiFailure::new(
            404,
            "RESEARCH_PRESET_NOT_FOUND",
            "research screen preset not found",
        ),
        ResearchPresetReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "RESEARCH_PRESET_UNAVAILABLE", message)
        }
    }
}
