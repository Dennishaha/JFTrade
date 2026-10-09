use super::super::super::super::skills::filesystem_skill;
use super::super::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPortError, install_skill_with_resolver,
    uninstall_registered_skill, uninstall_skill,
};
use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;
use serde_json::json;
use std::collections::BTreeMap;
use std::io::{BufRead, BufReader};
use std::net::{SocketAddr, TcpListener};
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread::JoinHandle;
use std::time::Duration;

type DownloadRoute = (u16, Option<String>, Vec<u8>);

struct DownloadServer {
    address: SocketAddr,
    stop: Arc<AtomicBool>,
    worker: Option<JoinHandle<()>>,
    requests: Arc<Mutex<Vec<String>>>,
    resolutions: Arc<Mutex<Vec<String>>>,
}

impl DownloadServer {
    fn start(routes: BTreeMap<String, DownloadRoute>) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        listener.set_nonblocking(true).unwrap();
        let stop = Arc::new(AtomicBool::new(false));
        let worker_stop = stop.clone();
        let requests = Arc::new(Mutex::new(Vec::new()));
        let worker_requests = requests.clone();
        let worker = std::thread::spawn(move || {
            while !worker_stop.load(Ordering::Acquire) {
                let (mut stream, _) = match listener.accept() {
                    Ok(socket) => socket,
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        std::thread::sleep(Duration::from_millis(2));
                        continue;
                    }
                    Err(error) => panic!("accept skill request: {error}"),
                };
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(3)))
                    .unwrap();
                stream
                    .set_write_timeout(Some(Duration::from_secs(3)))
                    .unwrap();
                let mut reader = BufReader::new(stream.try_clone().unwrap());
                let mut request = String::new();
                reader.read_line(&mut request).unwrap();
                let path = request.split_whitespace().nth(1).unwrap().to_owned();
                worker_requests.lock().unwrap().push(path.clone());
                loop {
                    let mut line = String::new();
                    assert!(reader.read_line(&mut line).unwrap() > 0);
                    if line == "\r\n" {
                        break;
                    }
                }
                let (status, location, body) = routes.get(&path).unwrap();
                let reason = match status {
                    200 => "OK",
                    302 => "Found",
                    404 => "Not Found",
                    _ => panic!("unsupported fixture status"),
                };
                let location = location
                    .as_ref()
                    .map_or(String::new(), |value| format!("Location: {value}\r\n"));
                let content_type = if path.ends_with(".zip") {
                    "application/zip"
                } else {
                    "text/markdown"
                };
                write!(stream,"HTTP/1.1 {status} {reason}\r\n{location}Content-Type: {content_type}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",body.len()).unwrap();
                stream.write_all(body).unwrap();
            }
        });
        Self {
            address,
            stop,
            worker: Some(worker),
            requests,
            resolutions: Default::default(),
        }
    }

    fn url(&self, path: &str) -> String {
        format!("http://skills.example:{}{path}", self.address.port())
    }

    fn install(
        &self,
        port: &ProductionAdkPort,
        path: &str,
    ) -> Result<Value, super::super::AdkMutationPortError> {
        let address = self.address;
        let resolutions = self.resolutions.clone();
        let resolve = Arc::new(move |url: reqwest::Url| {
            resolutions
                .lock()
                .unwrap()
                .push(url.host_str().unwrap_or_default().to_owned());
            match url.host_str() {
                Some("skills.example") => Ok(address),
                Some("blocked.example") => Err("blocked host".to_owned()),
                _ => Err("unexpected fixture host".to_owned()),
            }
        });
        install_skill_with_resolver(
            port,
            &AdkMutationInput {
                operation: AdkMutationOperation::InstallSkill,
                identifiers: BTreeMap::new(),
                body: json!({"url":self.url(path)}),
                webhook_secret: None,
            },
            resolve,
        )
    }
}

impl Drop for DownloadServer {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        self.worker.take().unwrap().join().unwrap();
    }
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_test.go:235 TestSkillRegistryInstallURLPlainDocumentAndRedirectSafety
#[test]
fn production_skill_url_install_downloads_plain_document_rejects_duplicates_and_blocks_unsafe_redirects()
 {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let server = DownloadServer::start(BTreeMap::from([
        ("/plain.md".to_owned(),(200,None,b"---\nname: plain-skill\ndescription: Plain Skill\nallowed-tools: [http.fetch]\n---\nUse the plain downloaded skill.".to_vec())),
        ("/redirect".to_owned(),(302,Some("http://blocked.example/skill.md".to_owned()),Vec::new())),
    ]));
    let skill = server.install(&port, "/plain.md").unwrap();
    assert_eq!(skill["id"], "plain-skill");
    assert_eq!(skill["source"], server.url("/plain.md"));
    assert_eq!(skill["tools"], json!(["http.fetch"]));
    let path = root.path().join("skills/plain-skill/SKILL.md");
    let raw = std::fs::read_to_string(&path).unwrap();
    assert!(raw.contains(&format!("source: {}", server.url("/plain.md"))));
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let duplicate = server.install(&port, "/plain.md").unwrap_err();
    assert!(duplicate.to_string().contains("already installed"));
    let redirect = server.install(&port, "/redirect").unwrap_err();
    port.shutdown_with_error().unwrap();
    assert!(
        matches!(&duplicate, AdkMutationPortError::Failed { status: 400, code, .. } if code=="ADK_SKILL_INSTALL_FAILED"),
        "{duplicate}; redirect={redirect}"
    );
    assert!(
        redirect.to_string().contains("redirect to unsafe host"),
        "{redirect}"
    );
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(std::fs::read_to_string(&path).unwrap(), raw);
    assert_eq!(
        *server.requests.lock().unwrap(),
        ["/plain.md", "/plain.md", "/redirect"]
    );
    assert_eq!(
        *server.resolutions.lock().unwrap(),
        [
            "skills.example",
            "skills.example",
            "skills.example",
            "blocked.example"
        ]
    );
}

// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:224 TestInstallSkillURLInstallsNeodataFinancialSearch
#[test]
fn production_skill_url_install_downloads_neodata_and_gets_registered_metadata() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let path = "/skills/neodata-financial-search/SKILL.md";
    let server = DownloadServer::start(BTreeMap::from([(path.to_owned(),(200,None,b"---\nname: neodata-financial-search\ndescription: Search NeoData financial filings and earnings materials.\nallowed-tools: [http.fetch]\nmetadata:\n  version: 2026.06\n---\nUse NeoData search results as reference material and cite the source URL.".to_vec()))]));
    let skill = server.install(&port, path).unwrap();
    assert_eq!(skill["id"], "neodata-financial-search");
    assert_eq!(skill["source"], server.url(path));
    assert_eq!(skill["version"], "2026.06");
    assert_eq!(skill["tools"], json!(["http.fetch"]));
    assert!(
        root.path()
            .join("skills/neodata-financial-search/SKILL.md")
            .exists()
    );
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let stored = filesystem_skill(&port, "neodata-financial-search")
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert!(!stored["contentHash"].as_str().unwrap().is_empty());
    assert_eq!(stored["validationStatus"], "VALID");
    assert_eq!(stored["tools"], json!(["http.fetch"]));
    assert_eq!(stored["source"], server.url(path));
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_test.go:162 TestSkillRegistryInstallURLAndDirectoryBoundaries
#[test]
fn production_skill_url_install_rejects_http_404_and_redirect_loops_without_writes() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let server = DownloadServer::start(BTreeMap::from([
        (
            "/missing/SKILL.md".to_owned(),
            (404, None, b"missing".to_vec()),
        ),
        (
            "/loop".to_owned(),
            (302, Some("/loop".to_owned()), Vec::new()),
        ),
    ]));
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let missing = server.install(&port, "/missing/SKILL.md").unwrap_err();
    assert!(missing.to_string().contains("returned 404"), "{missing}");
    let loop_error = server.install(&port, "/loop").unwrap_err();
    port.shutdown_with_error().unwrap();
    assert!(
        matches!(&missing, AdkMutationPortError::Failed { status: 400, code, .. } if code=="ADK_SKILL_INSTALL_FAILED"),
        "{missing}; loop={loop_error}"
    );
    assert!(
        loop_error.to_string().contains("too many redirects"),
        "{loop_error}"
    );
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert!(!root.path().join("skills").exists());
    assert_eq!(
        *server.requests.lock().unwrap(),
        [
            "/missing/SKILL.md",
            "/loop",
            "/loop",
            "/loop",
            "/loop",
            "/loop"
        ]
    );
    assert_eq!(
        *server.resolutions.lock().unwrap(),
        vec!["skills.example"; 7]
    );
}

// Production redirect control in skillsruntime/registry.go: validate each
// hop, follow allowed hosts, and keep the original installation source URL.
#[test]
fn production_skill_url_install_follows_a_validated_relative_redirect_and_keeps_the_original_source()
 {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let server = DownloadServer::start(BTreeMap::from([
        ("/allowed".to_owned(),(302,Some("/redirected.md".to_owned()),Vec::new())),
        ("/redirected.md".to_owned(),(200,None,b"---\nname: redirected-skill\ndescription: Redirected skill\n---\nFollow the validated redirect.".to_vec())),
    ]));
    let installed = server.install(&port, "/allowed");
    port.shutdown_with_error().unwrap();
    let installed = installed.unwrap();
    assert_eq!(installed["id"], "redirected-skill");
    assert_eq!(installed["source"], server.url("/allowed"));
    let stored = filesystem_skill(&port, "redirected-skill")
        .unwrap()
        .unwrap();
    assert_eq!(stored["source"], server.url("/allowed"));
    assert_eq!(
        *server.requests.lock().unwrap(),
        ["/allowed", "/redirected.md"]
    );
    assert_eq!(
        *server.resolutions.lock().unwrap(),
        ["skills.example", "skills.example"]
    );
}

// Following redirects must preserve the production URL-shape guard before
// invoking any address resolver or connecting to the next target.
#[test]
fn production_skill_url_install_rejects_private_redirect_targets_before_resolution_or_connection() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let targets = [
        ("/private", "http://127.0.0.1:9/SKILL.md"),
        ("/local", "http://localhost/SKILL.md"),
        ("/metadata", "http://169.254.169.254/SKILL.md"),
        ("/scheme", "ftp://skills.example/SKILL.md"),
    ];
    let server = DownloadServer::start(
        targets
            .iter()
            .map(|(path, target)| {
                (
                    path.to_string(),
                    (302, Some(target.to_string()), Vec::new()),
                )
            })
            .collect(),
    );
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    for (path, _) in targets {
        let error = server.install(&port, path).unwrap_err();
        assert!(
            matches!(&error,AdkMutationPortError::Failed{status:400,code,..} if code=="ADK_SKILL_INSTALL_FAILED"),
            "{error}"
        );
        assert!(
            error.to_string().contains("redirect to unsafe host"),
            "{error}"
        );
    }
    port.shutdown_with_error().unwrap();
    assert_eq!(
        *server.requests.lock().unwrap(),
        ["/private", "/local", "/metadata", "/scheme"]
    );
    assert_eq!(
        *server.resolutions.lock().unwrap(),
        vec!["skills.example"; 4]
    );
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert!(!root.path().join("skills").exists());
}

fn stored_archive(files: &[(&str, &[u8])]) -> Vec<u8> {
    let mut buffer = std::io::Cursor::new(Vec::new());
    let mut writer = zip::ZipWriter::new(&mut buffer);
    let options =
        zip::write::SimpleFileOptions::default().compression_method(zip::CompressionMethod::Stored);
    for (name, bytes) in files {
        writer.start_file(*name, options).unwrap();
        writer.write_all(bytes).unwrap();
    }
    writer.finish().unwrap();
    buffer.into_inner()
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_test.go:286 TestSkillRegistryInstallURLSupportsArchivesAndUninstallProtections
#[test]
fn production_skill_url_install_preserves_archive_size_windows_resources_and_uninstall_protections()
{
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let archive = skill_archive(&[
        (
            "archive-skill/SKILL.md",
            "---\nname: archive-skill\ndescription: Archive Skill\nallowed-tools: [http.fetch]\n---\nUse the bundled archive instructions.",
        ),
        ("archive-skill/references/checklist.md", "archive checklist"),
    ]);
    let a = vec![b'a'; 300 << 10];
    let b = vec![b'b'; 300 << 10];
    let large = stored_archive(&[
        ("large-archive-skill/SKILL.md",b"---\nname: large-archive-skill\ndescription: Large Archive Skill\n---\nUse the bundled archive instructions."),
        ("large-archive-skill/references/a",&a),
        ("large-archive-skill/references/b",&b),
    ]);
    assert!(large.len() > 512 << 10 && large.len() <= 4 << 20);
    let server = DownloadServer::start(BTreeMap::from([
        ("/archive.zip".to_owned(), (200, None, archive)),
        ("/large-archive.zip".to_owned(), (200, None, large)),
        (
            "/too-large.md".to_owned(),
            (200, None, vec![b'x'; (512 << 10) + 1]),
        ),
    ]));
    let skill = server.install(&port, "/archive.zip").unwrap();
    assert_eq!(skill["id"], "archive-skill");
    assert_eq!(skill["source"], server.url("/archive.zip"));
    assert_eq!(
        std::fs::read(
            root.path()
                .join("skills/archive-skill/references/checklist.md")
        )
        .unwrap(),
        b"archive checklist"
    );
    let large = server.install(&port, "/large-archive.zip").unwrap();
    assert_eq!(large["id"], "large-archive-skill");
    let oversized = server.install(&port, "/too-large.md").unwrap_err();
    assert!(
        oversized.to_string().contains("skill file exceeds"),
        "{oversized}"
    );
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let document_path = root.path().join("skills/archive-skill/SKILL.md");
    let document_bytes = std::fs::read(&document_path).unwrap();
    let registry_missing = uninstall_registered_skill(&port, "missing-skill").unwrap_err();
    let missing_source = std::error::Error::source(&registry_missing)
        .and_then(|source| source.downcast_ref::<std::io::Error>());
    assert!(
        missing_source.is_some_and(|source| source.kind() == std::io::ErrorKind::NotFound),
        "missing registry uninstall must preserve its typed NotFound source: {registry_missing}"
    );
    let missing = uninstall_skill(&port, "missing-skill").unwrap_err();
    assert!(matches!(
        &missing,
        AdkMutationPortError::Failed { status: 500, code, .. } if code == "ADK_SKILL_UNINSTALL_FAILED"
    ));
    assert!(missing.to_string().contains("file does not exist"));
    let builtin = uninstall_skill(&port, "jftrade-market").unwrap_err();
    assert!(
        builtin
            .to_string()
            .contains("builtin skills cannot be uninstalled")
    );
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(std::fs::read(&document_path).unwrap(), document_bytes);
    uninstall_skill(&port, "archive-skill").unwrap();
    port.shutdown_with_error().unwrap();
    assert!(filesystem_skill(&port, "archive-skill").unwrap().is_none());
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_test.go:115
// TestSkillRegistryArchiveRejectsUnsafeOrAmbiguousBundles
#[test]
fn production_skill_url_install_preserves_archive_error_classification_and_writes_nothing() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let missing = stored_archive(&[("pack/references/guide.md", b"guide")]);
    let ambiguous = stored_archive(&[
        ("one/SKILL.md", b"---\nname: one\n---\nOne."),
        ("two/SKILL.md", b"---\nname: two\n---\nTwo."),
    ]);
    let oversized_skill = stored_archive(&[(
        "huge-skill/SKILL.md",
        &[b'x'; (512 << 10) + 1],
    )]);
    let oversized_archive = vec![b'x'; (4 << 20) + 1];
    let server = DownloadServer::start(BTreeMap::from([
        ("/missing.zip".to_owned(), (200, None, missing)),
        ("/ambiguous.zip".to_owned(), (200, None, ambiguous)),
        (
            "/corrupt.zip".to_owned(),
            (200, None, b"not a zip".to_vec()),
        ),
        ("/huge.zip".to_owned(), (200, None, oversized_archive)),
        (
            "/huge-skill.zip".to_owned(),
            (200, None, oversized_skill),
        ),
    ]));
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();

    let missing_error = server.install(&port, "/missing.zip").unwrap_err();
    assert!(
        missing_error.to_string().contains("does not contain SKILL.md"),
        "missing archive error = {missing_error}"
    );
    let ambiguous_error = server.install(&port, "/ambiguous.zip").unwrap_err();
    assert!(
        ambiguous_error
            .to_string()
            .contains("exactly one SKILL.md"),
        "ambiguous archive error = {ambiguous_error}"
    );
    let corrupt_error = server.install(&port, "/corrupt.zip").unwrap_err();
    assert!(
        corrupt_error.to_string().contains("parse skill archive"),
        "corrupt archive error = {corrupt_error}"
    );
    let oversized_archive_error = server.install(&port, "/huge.zip").unwrap_err();
    assert!(
        oversized_archive_error
            .to_string()
            .contains("skill archive exceeds"),
        "oversized archive error = {oversized_archive_error}"
    );
    let oversized_skill_error = server.install(&port, "/huge-skill.zip").unwrap_err();
    assert!(
        oversized_skill_error
            .to_string()
            .contains("skill file exceeds 512 KiB"),
        "oversized skill error = {oversized_skill_error}"
    );

    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert!(!root.path().join("skills").exists());
}

// Parity: go:452dea11:internal/assistant/engine/skill_reg_fs_test.go:89 TestSkillRegistryFilesystemFailureBoundaries
#[test]
fn production_skill_url_install_rejects_a_file_at_the_registry_root() {
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let registry_root = root.path().join("skills");
    std::fs::write(&registry_root, b"not a directory").unwrap();
    let server = DownloadServer::start(BTreeMap::from([(
        "/blocked.md".to_owned(),
        (200, None, b"---\nname: blocked\ndescription: Valid download\n---\nUse the downloaded skill.".to_vec()),
    )]));
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let error = server.install(&port, "/blocked.md").unwrap_err();
    port.shutdown_with_error().unwrap();
    assert!(matches!(&error, AdkMutationPortError::Failed { status: 400, code, .. } if code == "ADK_SKILL_INSTALL_FAILED"), "{error}");
    assert_eq!(std::fs::read(&registry_root).unwrap(), b"not a directory");
    assert_eq!(port.store.list_skills().unwrap(), rows);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(*server.requests.lock().unwrap(), ["/blocked.md"]);
    assert_eq!(*server.resolutions.lock().unwrap(), ["skills.example"]);
}

// Supplemental production regression for Go EnsureBuiltins ->
// InstallSkillDocument/InstallSkillDirectory's existing-directory protection.
#[test]
fn production_skill_url_install_preserves_builtin_ids_against_documents_and_archives() {
    use crate::product::{AdkReadSnapshot, AdkReadSnapshotPort};
    let root = tempdir().unwrap();
    let port = test_port(root.path());
    let AdkReadSnapshot::Json(catalog) = port.read("/api/v1/adk/skills", "").unwrap() else {
        panic!("expected skill catalog")
    };
    let rows = port.store.list_skills().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let mut routes = BTreeMap::new();
    for skill in catalog["skills"].as_array().unwrap() {
        let id = skill["id"].as_str().unwrap();
        assert_eq!(skill["source"], "builtin");
        let document = format!("---\nname: {id}\ndescription: Remote replacement\nallowed-tools: [http.fetch]\n---\nReplace the builtin instructions.");
        routes.insert(format!("/{id}.md"), (200, None, document.as_bytes().to_vec()));
        routes.insert(format!("/{id}.zip"), (200, None, skill_archive(&[("SKILL.md", &document),("references/replacement.md", "remote resource")])));
    }
    let server = DownloadServer::start(routes);
    for skill in catalog["skills"].as_array().unwrap() {
        let id = skill["id"].as_str().unwrap();
        for extension in ["md", "zip"] {
            let result = server.install(&port, &format!("/{id}.{extension}"));
            assert!(result.is_err(), "remote {extension} must not replace builtin {id}: {result:?}");
            let error = result.unwrap_err();
            assert!(matches!(&error, AdkMutationPortError::Failed { status: 400, code, .. } if code == "ADK_SKILL_INSTALL_FAILED"), "{error}");
            assert!(error.to_string().contains("already installed"), "{error}");
            assert_eq!(port.store.list_skills().unwrap(), rows);
            assert_eq!(port.store.list_audit_events().unwrap(), audit);
            assert!(!root.path().join("skills").exists());
            let AdkReadSnapshot::Json(current) = port.read("/api/v1/adk/skills", "").unwrap() else { panic!("expected catalog") };
            assert_eq!(current, catalog);
        }
    }
    port.shutdown_with_error().unwrap();
    assert_eq!(server.requests.lock().unwrap().len(), catalog["skills"].as_array().unwrap().len() * 2);
}
