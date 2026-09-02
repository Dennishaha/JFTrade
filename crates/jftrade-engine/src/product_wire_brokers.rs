fn broker_read_snapshot_failure(error: BrokerReadSnapshotError) -> ApiFailure {
    match error {
        BrokerReadSnapshotError::Invalid(message) => ApiFailure::new(400, "BAD_REQUEST", message),
        BrokerReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "BROKER_READ_UNAVAILABLE", message)
        }
    }
}
