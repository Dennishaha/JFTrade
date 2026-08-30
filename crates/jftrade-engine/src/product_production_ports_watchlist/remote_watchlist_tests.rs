use super::*;

#[test]
fn remote_read_query_defaults_to_groups() {
    let query =
        parse_remote_read_query("brokerId=futu&sourceId=futu%3Adefault").expect("groups query");
    assert_eq!(query.operation, "groups");
    assert!(query.remote_group_id.is_empty());
}

#[test]
fn remote_read_query_requires_group_for_members() {
    assert!(matches!(
        parse_remote_read_query("operation=members"),
        Err(RemoteWatchlistSnapshotError::Invalid(message))
            if message == "remoteGroupId is required for members operation"
    ));
    let query = parse_remote_read_query("operation=MEMBERS&remoteGroupId=futu-group%3Aabc")
        .expect("members query");
    assert_eq!(query.operation, "members");
    assert_eq!(query.remote_group_id, "futu-group:abc");
}

#[test]
fn remote_read_query_rejects_unknown_operation_and_bad_encoding() {
    assert!(matches!(
        parse_remote_read_query("operation=modify"),
        Err(RemoteWatchlistSnapshotError::Invalid(message))
            if message == "operation must be groups or members"
    ));
    assert!(matches!(
        parse_remote_read_query("operation=%FF"),
        Err(RemoteWatchlistSnapshotError::Invalid(message))
            if message == "invalid remote watchlist query encoding"
    ));
}
