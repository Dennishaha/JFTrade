use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use std::time::Duration;

async fn uninstall_router(port: Arc<ProductionAdkPort>) -> crate::product::ProductHandle {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone())
    .with_adk_mutation_port(port);
    let runtime = crate::product_runtime::ProductRuntimeState::product_only(&config);
    let prepared = crate::product::prepare_product_with_runtime_state(config, runtime, None)
        .await
        .unwrap();
    crate::product::expose_prepared_product(prepared).unwrap()
}

fn installed_external(port: &ProductionAdkPort) -> Value {
    let source = "https://example.com/uninstall-skill.md";
    install_skill_document(
        port,
        source,
        &reqwest::Url::parse(source).unwrap(),
        b"---\nname: uninstall-skill\ndescription: Uninstall control\nallowed-tools: [http.fetch]\n---\nKeep the document until uninstall succeeds.",
        "text/markdown",
    )
    .unwrap()
}

fn client() -> reqwest::Client {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap()
}

#[cfg(unix)]
struct ReadonlyDirectory {
    path: std::path::PathBuf,
    permissions: Option<std::fs::Permissions>,
}

#[cfg(unix)]
impl ReadonlyDirectory {
    fn new(path: std::path::PathBuf) -> Self {
        use std::os::unix::fs::PermissionsExt;
        let permissions = std::fs::metadata(&path).unwrap().permissions();
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o555)).unwrap();
        Self {
            path,
            permissions: Some(permissions),
        }
    }

    fn restore(&mut self) {
        if let Some(permissions) = self.permissions.take() {
            std::fs::set_permissions(&self.path, permissions).unwrap();
        }
    }
}

#[cfg(unix)]
impl Drop for ReadonlyDirectory {
    fn drop(&mut self) {
        self.restore();
    }
}

// Supplemental failure control for the original Uninstall owner in
// go:452dea11:internal/assistant/engine/skillsruntime/install.go.
#[cfg(unix)]
#[tokio::test]
async fn production_skill_uninstall_reports_file_removal_failure_and_keeps_the_record_for_retry() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let installed = installed_external(&port);
    let document = std::path::PathBuf::from(installed["installPath"].as_str().unwrap());
    let bytes = std::fs::read(&document).unwrap();
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let mut fault = ReadonlyDirectory::new(document.parent().unwrap().to_path_buf());
    let handle = uninstall_router(port.clone()).await;
    let url = format!(
        "http://{}/api/v1/adk/skills",
        handle.startup_record().address
    );
    let client = client();
    let failed = client
        .delete(format!("{url}/uninstall-skill"))
        .send()
        .await
        .unwrap();
    let status = failed.status().as_u16();
    let failed: Value = failed.json().await.unwrap();
    let failed_rows = port.store.list_skills().unwrap();
    let failed_audit = port.store.list_audit_events().unwrap();
    fault.restore();
    let retained_bytes = std::fs::read(&document).unwrap();
    let retained: Value = client.get(&url).send().await.unwrap().json().await.unwrap();
    let retry = client
        .delete(format!("{url}/uninstall-skill"))
        .send()
        .await
        .unwrap();
    let retry_status = retry.status().as_u16();
    let missing: Value = client.get(&url).send().await.unwrap().json().await.unwrap();
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(status, 500, "{failed}");
    assert_eq!(failed["error"]["code"], "ADK_SKILL_UNINSTALL_FAILED");
    assert!(
        failed["error"]["message"]
            .as_str()
            .unwrap()
            .contains("Permission denied")
    );
    assert_eq!(failed_rows, rows);
    assert_eq!(failed_audit, audit);
    assert_eq!(retained_bytes, bytes);
    assert!(
        retained["data"]["skills"]
            .as_array()
            .unwrap()
            .iter()
            .any(|skill| skill["id"] == "uninstall-skill")
    );
    assert_eq!(retry_status, 200);
    assert!(!document.parent().unwrap().exists());
    assert!(port.store.get_skill("uninstall-skill").unwrap().is_none());
    assert!(
        !missing["data"]["skills"]
            .as_array()
            .unwrap()
            .iter()
            .any(|skill| skill["id"] == "uninstall-skill")
    );
}

// Supplemental SQLite failure control: deletion must fail before removing
// the installed files when the authoritative writer rejects its DELETE.
#[tokio::test]
async fn production_skill_uninstall_preserves_files_and_rows_when_the_sqlite_delete_is_rejected() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let installed = installed_external(&port);
    let document = std::path::PathBuf::from(installed["installPath"].as_str().unwrap());
    let bytes = std::fs::read(&document).unwrap();
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let connection = rusqlite::Connection::open(root.path().join("adk.db")).unwrap();
    connection.execute_batch("CREATE TRIGGER reject_skill_delete BEFORE DELETE ON adk_skills BEGIN SELECT RAISE(ABORT, 'skill delete blocked'); END;").unwrap();
    let handle = uninstall_router(port.clone()).await;
    let client = client();
    let url = format!(
        "http://{}/api/v1/adk/skills/uninstall-skill",
        handle.startup_record().address
    );
    let failed = client.delete(&url).send().await.unwrap();
    let status = failed.status().as_u16();
    let failed: Value = failed.json().await.unwrap();
    let failed_rows = port.store.list_skills().unwrap();
    let failed_audit = port.store.list_audit_events().unwrap();
    let retained_bytes = std::fs::read(&document).unwrap();
    connection
        .execute_batch("DROP TRIGGER reject_skill_delete;")
        .unwrap();
    let retry = client.delete(&url).send().await.unwrap();
    let retry_status = retry.status().as_u16();
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(status, 500, "{failed}");
    assert_eq!(failed["error"]["code"], "ADK_SKILL_UNINSTALL_FAILED");
    assert!(
        failed["error"]["message"]
            .as_str()
            .unwrap()
            .contains("skill delete blocked")
    );
    assert_eq!(failed_rows, rows);
    assert_eq!(failed_audit, audit);
    assert_eq!(retained_bytes, bytes);
    assert_eq!(retry_status, 200);
    assert!(!document.parent().unwrap().exists());
    assert!(port.store.get_skill("uninstall-skill").unwrap().is_none());
}
