use std::fs;

use tempfile::tempdir;

use super::*;

#[tokio::test]
async fn data_management_overview_is_authenticated_and_does_not_create_missing_databases() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let before = fs::read(&settings_path).expect("read seeded settings");
    let token = "data-management-read-token-012345678901234567890";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let address = handle.startup_record().address;

    let (unauthorized_status, unauthorized) = request_json_with_status(
        address,
        "GET",
        "/api/v1/settings/data-management/databases",
        None,
        &[],
    )
    .await;
    assert_eq!(unauthorized_status, 401);
    assert_eq!(unauthorized["ok"], false);

    let authorization = format!("Bearer {token}");
    for path in [
        "/api/v1/settings/data-management/databases",
        "/api/v1/settings/data-management/databases?summaryOnly=TRUE",
        "/api/v1/settings/data-management/databases?databaseId=%20strategy%20",
    ] {
        let (status, response) = request_json_with_status(
            address,
            "GET",
            path,
            None,
            &[("Authorization", authorization.as_str())],
        )
        .await;
        assert_eq!(status, 200, "status for {path}: {response}");
        assert_eq!(response["ok"], true, "envelope for {path}");
        assert!(
            response["data"]["databases"].is_array(),
            "projection for {path}"
        );
    }

    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/settings/data-management/databases?databaseId=unknown",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "DATABASE_STATUS_REJECTED");
    handle.shutdown().await.expect("shutdown shadow");

    assert_eq!(fs::read(&settings_path).expect("read settings"), before);
    let entries = fs::read_dir(directory.path())
        .expect("read data directory")
        .map(|entry| entry.expect("directory entry").file_name())
        .collect::<Vec<_>>();
    assert_eq!(
        entries,
        vec![settings_path.file_name().expect("settings name")]
    );
}
