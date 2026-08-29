fn research_read_snapshot_failure(error: ResearchReadSnapshotError) -> ApiFailure {
    match error {
        ResearchReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
        ResearchReadSnapshotError::Failed { status, code, message } => {
            ApiFailure::new(status, code, message)
        }
        ResearchReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "RESEARCH_UNAVAILABLE", message)
        }
    }
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
