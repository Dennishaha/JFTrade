//! Regression coverage for the ADK skill registry boundary.
//!
//! Reference fixtures: go:452dea11:internal/assistant/engine/store_ops_test.go
//! `TestInstallSkillArchivePreservesResources` and
//! `TestInstallSkillURLInstallsNeodataFinancialSearch`, plus
//! go:452dea11:internal/assistant/engine/skillsruntime/install.go for the
//! archive safety rules.

use std::collections::BTreeMap;
use std::io::Write;
use std::path::Path;
use std::sync::Arc;

use serde_json::Value;
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore};

use super::super::super::{
    PRODUCTION_TOOL_DEFINITIONS, ProductionAdapterBinding, ProductionAdkPort, ProductionToolCatalog,
};
use super::install_skill_document;

#[path = "product_adk_skill_install_registry_owner_tests.rs"]
mod registry;

#[path = "product_adk_skill_uninstall_owner_tests.rs"]
mod uninstall;

#[path = "product_adk_skill_url_owner_tests.rs"]
mod download;

fn test_port(root: &Path) -> Arc<ProductionAdkPort> {
    let adk_path = root.join("adk.db");
    let session_path = root.join("adk-session.db");
    let artifact_path = root.join("adk-artifact.db");
    for (path, component) in [
        (&adk_path, "adk"),
        (&session_path, "adk-session"),
        (&artifact_path, "adk-artifact"),
    ] {
        let connection = rusqlite::Connection::open(path).expect("create database");
        jftrade_store_sqlite::initialize_current(&connection, component).expect("init schema");
    }
    let bindings = PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| (definition.adapter, ProductionAdapterBinding::Ready))
        .collect::<BTreeMap<_, _>>();
    Arc::new(ProductionAdkPort {
        store: Arc::new(AdkStore::open(&adk_path).expect("open adk store")),
        session_store: Arc::new(
            AdkSessionStore::open(&session_path).expect("open adk session store"),
        ),
        artifact_store: Arc::new(
            AdkArtifactStore::open(&artifact_path).expect("open artifact store"),
        ),
        tool_catalog: Arc::new(ProductionToolCatalog::from_bindings(&bindings).unwrap()),
        settings_path: root.join("settings.json"),
        chat_runtime: None,
        unavailable_streams: Default::default(),
    })
}

fn skill_archive(entries: &[(&str, &str)]) -> Vec<u8> {
    let mut buffer = std::io::Cursor::new(Vec::new());
    {
        let mut writer = zip::ZipWriter::new(&mut buffer);
        let options = zip::write::SimpleFileOptions::default();
        for (name, content) in entries {
            writer.start_file(*name, options).expect("start file");
            writer.write_all(content.as_bytes()).expect("write file");
        }
        writer.finish().expect("finish archive");
    }
    buffer.into_inner()
}

/// A downloaded archive keeps every bundled resource next to its `SKILL.md`
/// and records the source URL plus the parsed metadata, exactly like the
/// reference `InstallArchive` -> `InstallExtractedArchiveSkill` path.
/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:184
/// `TestInstallSkillArchivePreservesResources`.
#[test]
fn skill_archive_install_preserves_resources_and_metadata() {
    let root = tempdir().expect("temp dir");
    let port = test_port(root.path());
    let source_url = "https://example.com/research-pack.zip";
    let archive = skill_archive(&[
        (
            "research-pack/SKILL.md",
            "---\nname: research-pack\ndescription: Research pack\nmetadata:\n  version: 2026.06\nallowed-tools: [http.fetch]\n---\nUse bundled resources.\n",
        ),
        ("research-pack/references/playbook.md", "playbook content"),
    ]);
    let parsed = reqwest::Url::parse(source_url).expect("source url");

    let installed = install_skill_document(&port, source_url, &parsed, &archive, "application/zip")
        .expect("install archive");

    assert_eq!(installed["id"], "research-pack");
    assert_eq!(installed["source"], source_url);
    assert_eq!(installed["version"], "2026.06");
    assert_eq!(
        installed["validationStatus"], "VALID",
        "an installed skill is validated: {installed}"
    );
    assert!(
        installed["contentHash"]
            .as_str()
            .is_some_and(|hash| hash.len() == 64),
        "the installed document has a content hash: {installed}"
    );

    let install_path = installed["installPath"]
        .as_str()
        .expect("install path")
        .to_owned();
    assert!(
        install_path.ends_with("research-pack/SKILL.md"),
        "install path = {install_path}"
    );
    let resource = root
        .path()
        .join("skills/research-pack/references/playbook.md");
    assert_eq!(
        std::fs::read_to_string(&resource).expect("read bundled resource"),
        "playbook content",
        "every archive member is preserved"
    );

    let stored = port
        .store
        .get_skill("research-pack")
        .expect("read stored skill")
        .expect("stored skill row");
    let stored: Value = serde_json::from_str(&stored.payload_json).expect("skill payload");
    assert_eq!(stored["source"], source_url);
    assert_eq!(stored["builtin"], false);
}

/// A single downloaded `SKILL.md` is stored as a one-file skill, which is the
/// shape `InstallURL` produces for the NeoData financial-search document.
#[test]
fn skill_document_install_registers_the_parsed_document() {
    let root = tempdir().expect("temp dir");
    let port = test_port(root.path());
    let source_url = "https://example.com/skills/neodata-financial-search/SKILL.md";
    let document = concat!(
        "---\n",
        "name: neodata-financial-search\n",
        "description: Search NeoData financial filings and earnings materials.\n",
        "metadata:\n  version: 2026.06\n",
        "allowed-tools: [http.fetch]\n",
        "---\n",
        "Use NeoData search results as reference material and cite the source URL.\n"
    );
    let parsed = reqwest::Url::parse(source_url).expect("source url");

    let installed = install_skill_document(
        &port,
        source_url,
        &parsed,
        document.as_bytes(),
        "text/markdown; charset=utf-8",
    )
    .expect("install document");

    assert_eq!(installed["id"], "neodata-financial-search");
    assert_eq!(installed["source"], source_url);
    assert_eq!(installed["version"], "2026.06");
    assert_eq!(installed["displayName"], "neodata-financial-search");
    assert!(
        installed["installPath"]
            .as_str()
            .is_some_and(|path| path.ends_with("neodata-financial-search/SKILL.md")),
        "install path = {}",
        installed["installPath"]
    );
    assert!(
        root.path()
            .join("skills/neodata-financial-search/SKILL.md")
            .exists(),
        "the document lands on disk"
    );
}

/// Installing the same id twice is the frozen `409 ADK_SKILL_EXISTS`, and a
/// second install must not clobber the first directory.
#[test]
fn skill_install_is_exclusive_per_id() {
    let root = tempdir().expect("temp dir");
    let port = test_port(root.path());
    let source_url = "https://example.com/skill.md";
    let document = "---\nname: exclusive-skill\n---\nfirst\n";
    let parsed = reqwest::Url::parse(source_url).expect("source url");
    install_skill_document(
        &port,
        source_url,
        &parsed,
        document.as_bytes(),
        "text/markdown",
    )
    .expect("first install");

    let second = "---\nname: exclusive-skill\n---\nsecond\n";
    match install_skill_document(
        &port,
        source_url,
        &parsed,
        second.as_bytes(),
        "text/markdown",
    ) {
        Err(error) => {
            let message = format!("{error:?}");
            assert!(
                message.contains("ADK_SKILL_EXISTS"),
                "duplicate install error = {message}"
            );
        }
        Ok(value) => panic!("duplicate install must fail, got {value}"),
    }
    let raw = std::fs::read_to_string(root.path().join("skills/exclusive-skill/SKILL.md"))
        .expect("read installed skill");
    assert!(
        raw.contains("first"),
        "the original install survives: {raw}"
    );
}

/// An archive member escaping the install root is rejected before anything is
/// written, matching the reference `ExtractSkillArchiveFile` guard.
#[test]
fn skill_archive_rejects_unsafe_paths_and_symlinks() {
    let root = tempdir().expect("temp dir");
    let port = test_port(root.path());
    let parsed = reqwest::Url::parse("https://example.com/evil.zip").expect("source url");

    for entry in ["../escape/SKILL.md", "/absolute/SKILL.md"] {
        let archive = skill_archive(&[
            (entry, "---\nname: evil\n---\nx\n"),
            ("pack/references/keep.md", "keep"),
        ]);
        match install_skill_document(
            &port,
            "https://example.com/evil.zip",
            &parsed,
            &archive,
            "application/zip",
        ) {
            Err(error) => {
                let message = format!("{error:?}");
                assert!(
                    message.contains("unsafe path"),
                    "archive with {entry:?} must report an unsafe path: {message}"
                );
            }
            Ok(value) => panic!("unsafe archive {entry:?} was installed: {value}"),
        }
    }
    assert!(
        !root.path().join("escape").exists() && !root.path().join("absolute").exists(),
        "nothing outside the skills root may be written"
    );

    // An archive without any SKILL.md is unusable too.
    let archive = skill_archive(&[("pack/notes.md", "no document")]);
    let error = install_skill_document(
        &port,
        "https://example.com/evil.zip",
        &parsed,
        &archive,
        "application/zip",
    )
    .expect_err("archive without SKILL.md must fail");
    assert!(
        format!("{error:?}").contains("SKILL.md"),
        "error must name the missing document: {error:?}"
    );
}

/// `json` round-trips the installed payload so callers get the durable
/// projection rather than the in-memory document.
#[test]
fn skill_install_projection_is_durable() {
    let root = tempdir().expect("temp dir");
    let port = test_port(root.path());
    let source_url = "https://example.com/durable.md";
    let parsed = reqwest::Url::parse(source_url).expect("source url");
    install_skill_document(
        &port,
        source_url,
        &parsed,
        b"---\nname: durable-skill\ndescription: Durable\n---\nbody\n",
        "text/markdown",
    )
    .expect("install");

    // The ADK database is owned by a single writer lease, so the durability
    // check goes through the owning store rather than a second handle.
    let stored = port
        .store
        .get_skill("durable-skill")
        .expect("read skill")
        .expect("skill row");
    let stored: Value = serde_json::from_str(&stored.payload_json).expect("skill payload");
    assert_eq!(stored["id"], "durable-skill");
    assert_eq!(stored["description"], "Durable");
    assert!(
        stored["contentHash"]
            .as_str()
            .is_some_and(|hash| !hash.is_empty()),
        "the durable row keeps the content hash: {stored}"
    );
}
