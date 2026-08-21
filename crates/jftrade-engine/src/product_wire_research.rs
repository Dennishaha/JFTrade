fn research_read_snapshot_failure(error: ResearchReadSnapshotError) -> ApiFailure {
    match error {
        ResearchReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
        ResearchReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "RESEARCH_UNAVAILABLE", message)
        }
    }
}
