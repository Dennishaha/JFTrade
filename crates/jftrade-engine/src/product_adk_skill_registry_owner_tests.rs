use super::*;

async fn registry_list(port: Arc<ProductionAdkPort>) -> Value {
    let handle = skill_router(port, false).await;
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap();
    let response = client
        .get(format!(
            "http://{}/api/v1/adk/skills",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    let body: Value = response.json().await.unwrap();
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    body["data"]["skills"].clone()
}

fn write_registry_document(port: &ProductionAdkPort, id: &str, raw: &str) {
    let path = port.settings_path.parent().unwrap().join("skills").join(id);
    fs::create_dir_all(&path).unwrap();
    fs::write(path.join("SKILL.md"), raw).unwrap();
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:19 TestSkillRegistryListSortsBySourceAndDefaultsFilesystemMetadata
#[tokio::test]
async fn production_skill_list_sorts_by_source_then_name_and_preserves_filesystem_defaults() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    write_registry_document(
        &port,
        "z-local",
        "---\nname: z-local\ndescription: Local filesystem skill\nallowed-tools: [local.tool]\n---\nUse the local skill.",
    );
    write_registry_document(
        &port,
        "a-remote",
        "---\nname: a-remote\ndescription: Remote skill\nmetadata:\n  source: https://example.com/a-remote/SKILL.md\n  version: '7'\n---\nUse the remote skill.",
    );
    write_registry_document(
        &port,
        "b-builtin",
        "---\nname: b-builtin\ndescription: Builtin skill\nmetadata:\n  source: builtin\n---\nUse the builtin skill.",
    );
    let files = crate::product::product_production_ports::product_production_ports_adk::skills::filesystem_skills(&port).unwrap();
    assert_eq!(files.len(), 3);
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let skills = registry_list(port.clone()).await;
    let selected = skills
        .as_array()
        .unwrap()
        .iter()
        .filter(|skill| {
            ["z-local", "a-remote", "b-builtin"].contains(&skill["id"].as_str().unwrap())
        })
        .collect::<Vec<_>>();
    assert_eq!(selected.len(), 3);
    assert_eq!(
        selected
            .iter()
            .map(|skill| skill["id"].as_str().unwrap())
            .collect::<Vec<_>>(),
        ["b-builtin", "z-local", "a-remote"]
    );
    assert_eq!(selected[0]["builtin"], true);
    assert_eq!(selected[0]["source"], "builtin");
    assert_eq!(selected[1]["source"], "filesystem");
    assert_eq!(selected[1]["builtin"], false);
    assert_eq!(selected[1]["tools"], json!(["local.tool"]));
    assert_eq!(selected[2]["version"], "7");
    assert_eq!(
        selected[2]["source"],
        "https://example.com/a-remote/SKILL.md"
    );
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:72 TestSkillRegistryWarnsWhenExternalSkillReferencesUnknownTools
#[tokio::test]
async fn production_skill_list_keeps_unknown_external_tools_as_a_warning() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    write_registry_document(
        &port,
        "future-skill",
        "---\nname: future-skill\ndescription: Skill for a future tool\nallowed-tools: [future.tool]\n---\nUse the future tool when available.",
    );
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let skills = registry_list(port.clone()).await;
    let skill = skills
        .as_array()
        .unwrap()
        .iter()
        .find(|skill| skill["id"] == "future-skill")
        .unwrap();
    let direct = crate::product::product_production_ports::product_production_ports_adk::skills::filesystem_skill(&port, "future-skill").unwrap().unwrap();
    assert_eq!(&direct, skill);
    assert_eq!(skill["tools"], json!(["future.tool"]));
    assert!(
        skill["validationError"]
            .as_str()
            .unwrap()
            .contains("future.tool")
    );
    assert_eq!(skill["validationStatus"], "WARNING");
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/assistant/engine/skill_recover_test.go:11 TestSkillRegistrySourceAndFrontmatterFailureBoundaries
#[tokio::test]
async fn production_skill_list_and_get_propagate_corrupt_frontmatter_without_writes() {
    use crate::product::product_production_ports::product_production_ports_adk::skills;
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    write_registry_document(
        &port,
        "bad-skill",
        "---\nname: bad-skill\nmetadata: [\n---\nBad.",
    );
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    assert!(matches!(
        skills::filesystem_skill(&port, "bad-skill"),
        Err(skills::SkillRegistryError::Document(_))
    ));
    assert!(matches!(
        skills::filesystem_skills(&port),
        Err(skills::SkillRegistryError::Document(_))
    ));
    let handle = skill_router(port.clone(), false).await;
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .build()
        .unwrap();
    let response = client
        .get(format!(
            "http://{}/api/v1/adk/skills",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 500);
    assert_eq!(
        response.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_SKILL_LIST_FAILED"
    );
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}
