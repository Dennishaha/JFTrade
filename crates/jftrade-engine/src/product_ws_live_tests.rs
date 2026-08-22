use std::sync::Arc;

use tempfile::tempdir;

use super::*;

#[derive(Debug)]
struct EnabledWsLiveSnapshotPort;

impl WsLiveSnapshotPort for EnabledWsLiveSnapshotPort {
    fn enabled(&self) -> bool {
        true
    }
}

#[tokio::test]
async fn ws_live_route_is_registered_only_with_explicit_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let without_port =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(without_port).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    assert!(
        !handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "GET /api/v1/ws/live")
    );
    handle.shutdown().await.expect("shutdown product");

    let with_port =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(with_port).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "GET /api/v1/ws/live")
    );
    handle.shutdown().await.expect("shutdown product");
}
