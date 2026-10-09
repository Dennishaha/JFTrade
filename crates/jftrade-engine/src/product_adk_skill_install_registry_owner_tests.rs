use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use reqwest::Client;
use std::time::Duration;

async fn installed_skill_http(port: Arc<ProductionAdkPort>, id: &str) -> Value {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone());
    let runtime = crate::product_runtime::ProductRuntimeState::product_only(&config);
    let prepared = crate::product::prepare_product_with_runtime_state(config, runtime, None)
        .await
        .unwrap();
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let response = Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap()
        .get(format!(
            "http://{}/api/v1/adk/skills",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    let body: Value = response.json().await.unwrap();
    let skill = body["data"]["skills"]
        .as_array()
        .unwrap()
        .iter()
        .find(|skill| skill["id"] == id)
        .unwrap()
        .clone();
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    skill
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_test.go:235 TestSkillRegistryInstallURLPlainDocumentAndRedirectSafety
#[tokio::test]
async fn production_skill_install_persists_download_source_and_tools_in_the_file_and_http_projection()
 {
    use sha2::Digest;
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let source = "https://example.com/plain.md";
    let raw = b"---\nname: plain-skill\ndescription: Plain Skill\nallowed-tools: [http.fetch]\n---\nUse the plain downloaded skill.";
    let installed = install_skill_document(
        &port,
        source,
        &reqwest::Url::parse(source).unwrap(),
        raw,
        "text/markdown",
    )
    .unwrap();
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let skill = installed_skill_http(port.clone(), "plain-skill").await;
    let bytes = std::fs::read(root.path().join("skills/plain-skill/SKILL.md")).unwrap();
    let document = String::from_utf8(bytes.clone()).unwrap();
    let digest = sha2::Sha256::digest(&bytes)
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect::<String>();
    port.shutdown_with_error().unwrap();
    assert_eq!(installed["source"], source);
    assert_eq!(skill["source"], source);
    assert_eq!(installed["tools"], serde_json::json!(["http.fetch"]));
    assert_eq!(skill["tools"], installed["tools"]);
    assert!(document.contains(&format!("source: {source}")));
    assert!(document.ends_with("Use the plain downloaded skill."));
    assert_eq!(installed["contentHash"], digest);
    assert_eq!(skill["contentHash"], installed["contentHash"]);
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:48 TestSkillRegistryArchiveInstallsBundlesWithDirectoryEntries
#[tokio::test]
async fn production_skill_install_handles_archive_directory_entries_and_keeps_source_and_resources()
{
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let source = "https://example.com/dir-skill.zip";
    let mut buffer = std::io::Cursor::new(Vec::new());
    {
        let mut writer = zip::ZipWriter::new(&mut buffer);
        let options = zip::write::SimpleFileOptions::default();
        writer.start_file(".", options).unwrap();
        for directory in ["dir-skill/", "dir-skill/references/"] {
            writer.add_directory(directory, options).unwrap();
        }
        writer.start_file("dir-skill/SKILL.md", options).unwrap();
        writer.write_all(b"---\nname: dir-skill\ndescription: Directory archive skill\n---\nUse explicit archive directories.").unwrap();
        writer
            .start_file("dir-skill/references/checklist.md", options)
            .unwrap();
        writer.write_all(b"checklist").unwrap();
        writer.finish().unwrap();
    }
    let installed = install_skill_document(
        &port,
        source,
        &reqwest::Url::parse(source).unwrap(),
        &buffer.into_inner(),
        "application/zip",
    )
    .unwrap();
    assert_eq!(installed["id"], "dir-skill");
    assert_eq!(installed["source"], source);
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let skill = installed_skill_http(port.clone(), "dir-skill").await;
    port.shutdown_with_error().unwrap();
    assert_eq!(skill["source"], source);
    assert_eq!(
        std::fs::read(root.path().join("skills/dir-skill/references/checklist.md")).unwrap(),
        b"checklist"
    );
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}
