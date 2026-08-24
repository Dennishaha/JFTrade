use std::env;
use std::ffi::OsString;
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tokio::process::Command;

const NODE_MINIMUM_VERSION: SemanticVersion = SemanticVersion {
    major: 22,
    minor: 0,
    patch: 0,
};
const NODE_VERSION_TIMEOUT: Duration = Duration::from_secs(2);

#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
struct SemanticVersion {
    major: u64,
    minor: u64,
    patch: u64,
}

impl std::fmt::Display for SemanticVersion {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(formatter, "{}.{}.{}", self.major, self.minor, self.patch)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct RuntimeDependencies {
    pub checked_at: String,
    pub all_required_satisfied: bool,
    pub dependencies: Vec<RuntimeDependency>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct RuntimeDependency {
    pub id: &'static str,
    pub display_name: &'static str,
    pub required: bool,
    pub configurable: bool,
    pub status: DependencyStatus,
    pub minimum_version: &'static str,
    pub detected_version: String,
    pub configured_path: String,
    pub effective_path: String,
    pub resolved_path: String,
    pub attempted_paths: Vec<String>,
    pub source: String,
    pub homepage_url: &'static str,
    pub message: String,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum DependencyStatus {
    Ok,
    Missing,
    Outdated,
    Error,
}

impl DependencyStatus {
    const fn satisfied(self) -> bool {
        matches!(self, Self::Ok)
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct Candidate {
    path: PathBuf,
    source: String,
}

#[derive(Debug)]
struct Resolution {
    configured_path: String,
    effective_path: String,
    source: String,
    resolved_path: Option<PathBuf>,
    attempted_paths: Vec<String>,
    last_error: Option<String>,
}

pub(crate) async fn inspect(checked_at: String, configured_path: &str) -> RuntimeDependencies {
    let dependency = inspect_node(configured_path).await;
    RuntimeDependencies {
        checked_at,
        all_required_satisfied: !dependency.required || dependency.status.satisfied(),
        dependencies: vec![dependency],
    }
}

async fn inspect_node(configured_path: &str) -> RuntimeDependency {
    let resolution = resolve_node(configured_path);
    let mut dependency = base_node_dependency(&resolution);
    let Some(resolved_path) = resolution.resolved_path else {
        dependency.status = DependencyStatus::Missing;
        dependency.message = missing_message(
            &resolution.configured_path,
            &resolution.attempted_paths,
            resolution
                .last_error
                .as_deref()
                .unwrap_or("executable not found"),
        );
        return dependency;
    };

    let mut command = Command::new(&resolved_path);
    command
        .arg("--version")
        .stdin(Stdio::null())
        .kill_on_drop(true);
    let command = command.output();
    let output = match tokio::time::timeout(NODE_VERSION_TIMEOUT, command).await {
        Err(_) => {
            dependency.status = DependencyStatus::Error;
            dependency.message = "Node.js version check timed out.".to_owned();
            return dependency;
        }
        Ok(Err(error)) => {
            dependency.status = DependencyStatus::Error;
            dependency.message = format!("Node.js version check failed: {error}");
            return dependency;
        }
        Ok(Ok(output)) if !output.status.success() => {
            dependency.status = DependencyStatus::Error;
            dependency.message = format!(
                "Node.js version check failed: {}",
                summarize_command_error(&output.stderr, output.status.to_string())
            );
            return dependency;
        }
        Ok(Ok(output)) => output,
    };
    let version_text = String::from_utf8_lossy(&output.stdout).trim().to_owned();
    apply_version_output(&mut dependency, &version_text);
    dependency
}

fn apply_version_output(dependency: &mut RuntimeDependency, version_text: &str) {
    let version = match parse_version(version_text) {
        Ok(version) => version,
        Err(()) => {
            dependency.status = DependencyStatus::Error;
            dependency.detected_version = version_text.to_owned();
            dependency.message =
                format!("Node.js returned an unrecognized version: {version_text}");
            return;
        }
    };
    dependency.detected_version = version.to_string();
    if version < NODE_MINIMUM_VERSION {
        dependency.status = DependencyStatus::Outdated;
        dependency.message =
            format!("Node.js {version} is below the required {NODE_MINIMUM_VERSION}.");
        return;
    }
    dependency.status = DependencyStatus::Ok;
    dependency.message = format!("Node.js {version} is available.");
}

fn base_node_dependency(resolution: &Resolution) -> RuntimeDependency {
    RuntimeDependency {
        id: "node",
        display_name: "Node.js",
        required: true,
        configurable: true,
        status: DependencyStatus::Error,
        minimum_version: "22.0.0",
        detected_version: String::new(),
        configured_path: resolution.configured_path.clone(),
        effective_path: resolution.effective_path.clone(),
        resolved_path: resolution
            .resolved_path
            .as_ref()
            .map(|path| path.to_string_lossy().into_owned())
            .unwrap_or_default(),
        attempted_paths: resolution.attempted_paths.clone(),
        source: resolution.source.clone(),
        homepage_url: "https://nodejs.org/",
        message: String::new(),
    }
}

fn resolve_node(configured_path: &str) -> Resolution {
    let configured_path = normalize_executable_path(OsString::from(configured_path))
        .map(|path| path.to_string_lossy().into_owned())
        .unwrap_or_default();
    let candidates = node_candidates(&configured_path);
    let mut attempted_paths = Vec::with_capacity(candidates.len());
    let mut last_error = None;
    for candidate in &candidates {
        attempted_paths.push(candidate.path.to_string_lossy().into_owned());
        match resolve_executable(&candidate.path) {
            Ok(path) => {
                return Resolution {
                    configured_path,
                    effective_path: candidate.path.to_string_lossy().into_owned(),
                    source: candidate.source.clone(),
                    resolved_path: Some(path),
                    attempted_paths,
                    last_error: None,
                };
            }
            Err(error) => last_error = Some(error),
        }
    }
    let first = candidates.first().expect("node candidates are never empty");
    Resolution {
        configured_path,
        effective_path: first.path.to_string_lossy().into_owned(),
        source: first.source.clone(),
        resolved_path: None,
        attempted_paths,
        last_error,
    }
}

fn node_candidates(configured_path: &str) -> Vec<Candidate> {
    if !configured_path.is_empty() {
        return vec![Candidate {
            path: configured_path.into(),
            source: "settings".to_owned(),
        }];
    }
    for (name, source) in [
        (
            "JFTRADE_PINEWORKER_RUNTIME",
            "env:JFTRADE_PINEWORKER_RUNTIME",
        ),
        ("JFTRADE_NODE_BINARY", "env:JFTRADE_NODE_BINARY"),
    ] {
        if let Some(path) = env::var_os(name).and_then(normalize_executable_path) {
            return vec![Candidate {
                path: path.into(),
                source: source.to_owned(),
            }];
        }
    }
    let mut candidates = vec![Candidate {
        path: "node".into(),
        source: "path".to_owned(),
    }];
    if cfg!(target_os = "macos") {
        for path in [
            "/opt/homebrew/bin/node",
            "/usr/local/bin/node",
            "/opt/homebrew/opt/node/bin/node",
            "/usr/local/opt/node/bin/node",
        ] {
            candidates.push(Candidate {
                path: path.into(),
                source: format!("common:{path}"),
            });
        }
    }
    candidates
}

fn normalize_executable_path(value: OsString) -> Option<OsString> {
    let mut value = value.to_string_lossy().trim().to_owned();
    while value.len() >= 2 {
        let first = value.as_bytes()[0];
        let last = *value.as_bytes().last()?;
        if !matches!(first, b'\'' | b'"') || first != last {
            break;
        }
        value = value[1..value.len() - 1].trim().to_owned();
    }
    (!value.is_empty()).then(|| value.into())
}

fn resolve_executable(candidate: &Path) -> Result<PathBuf, String> {
    if candidate.components().count() > 1 || candidate.is_absolute() {
        return executable_file(candidate)
            .then(|| candidate.to_path_buf())
            .ok_or_else(|| format!("{} was not found or is not executable", candidate.display()));
    }
    let path = env::var_os("PATH").unwrap_or_default();
    for directory in env::split_paths(&path) {
        for name in executable_names(candidate) {
            let path = directory.join(name);
            if executable_file(&path) {
                return Ok(path);
            }
        }
    }
    Err(format!("{} was not found in PATH", candidate.display()))
}

#[cfg(unix)]
fn executable_file(path: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;

    path.metadata()
        .is_ok_and(|metadata| metadata.is_file() && metadata.permissions().mode() & 0o111 != 0)
}

#[cfg(not(unix))]
fn executable_file(path: &Path) -> bool {
    path.is_file()
}

fn executable_names(candidate: &Path) -> Vec<PathBuf> {
    if cfg!(windows) && candidate.extension().is_none() {
        let extensions = env::var("PATHEXT").unwrap_or_else(|_| ".COM;.EXE;.BAT;.CMD".to_owned());
        extensions
            .split(';')
            .filter(|extension| !extension.trim().is_empty())
            .map(|extension| PathBuf::from(format!("{}{}", candidate.display(), extension.trim())))
            .collect()
    } else {
        vec![candidate.to_path_buf()]
    }
}

fn missing_message(configured: &str, attempted: &[String], error: &str) -> String {
    if !configured.trim().is_empty() {
        return format!("Configured Node.js binary was not found or is not executable: {error}");
    }
    let attempted = if attempted.is_empty() {
        "node".to_owned()
    } else {
        attempted.join(", ")
    };
    format!(
        "Node.js was not found from the app environment: {error}. macOS apps launched from Finder do not inherit your shell PATH. Tried: {attempted}. Set the Node.js binary path in Settings if Node is installed in a custom location."
    )
}

fn summarize_command_error(output: &[u8], error: String) -> String {
    let text = String::from_utf8_lossy(output);
    let text = text.trim();
    if text.is_empty() {
        return error;
    }
    let tail = if text.len() > 500 {
        text.get(text.len() - 500..).unwrap_or(text)
    } else {
        text
    };
    format!("{error}: {tail}")
}

fn parse_version(raw: &str) -> Result<SemanticVersion, ()> {
    let first = raw.split_whitespace().next().ok_or(())?;
    let mut parts = first.trim_start_matches('v').split('.');
    let major = parts.next().ok_or(())?.parse().map_err(|_| ())?;
    let minor = parts
        .next()
        .map(str::parse)
        .transpose()
        .map_err(|_| ())?
        .unwrap_or(0);
    let patch = parts
        .next()
        .map(str::parse)
        .transpose()
        .map_err(|_| ())?
        .unwrap_or(0);
    if parts.next().is_some() {
        return Err(());
    }
    Ok(SemanticVersion {
        major,
        minor,
        patch,
    })
}

#[cfg(test)]
mod tests {
    use serde::Deserialize;

    use super::*;

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Stage9Corpus {
        version: String,
        node_version_cases: Vec<NodeVersionCase>,
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct NodeVersionCase {
        name: String,
        output: String,
        expected_status: DependencyStatus,
        expected_detected_version: String,
        expected_message: String,
    }

    #[test]
    fn node_version_parser_matches_go_compatibility_rules() {
        assert_eq!(
            parse_version("v22.1.2\n"),
            Ok(SemanticVersion {
                major: 22,
                minor: 1,
                patch: 2,
            })
        );
        assert_eq!(
            parse_version("22"),
            Ok(SemanticVersion {
                major: 22,
                minor: 0,
                patch: 0,
            })
        );
        for invalid in ["", "v", "1..2", "1.2.3.4", "not-a-version"] {
            assert_eq!(parse_version(invalid), Err(()), "{invalid}");
        }
    }

    #[test]
    fn executable_path_normalization_removes_balanced_nested_quotes() {
        assert_eq!(
            normalize_executable_path(OsString::from("  '\"/opt/node\"'  ")),
            Some(OsString::from("/opt/node"))
        );
        assert_eq!(normalize_executable_path(OsString::from("  ")), None);
        assert_eq!(
            normalize_executable_path(OsString::from("\"unbalanced'")),
            Some(OsString::from("\"unbalanced'"))
        );
    }

    #[test]
    fn configured_node_candidate_has_go_settings_precedence_and_wire_source() {
        assert_eq!(
            node_candidates("/configured/node"),
            vec![Candidate {
                path: "/configured/node".into(),
                source: "settings".to_owned(),
            }]
        );
        let resolution = resolve_node(" '\"/definitely/missing/node\"' ");
        assert_eq!(resolution.configured_path, "/definitely/missing/node");
        assert_eq!(resolution.effective_path, "/definitely/missing/node");
        assert_eq!(resolution.source, "settings");
        assert_eq!(resolution.attempted_paths, ["/definitely/missing/node"]);
        assert!(resolution.resolved_path.is_none());
    }

    #[test]
    fn stage9_node_version_corpus_matches_go() {
        let fixture = Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../tests/fixtures/rust-migration/stage9/product-slice-corpus.json");
        let corpus: Stage9Corpus = serde_json::from_slice(
            &std::fs::read(&fixture)
                .unwrap_or_else(|error| panic!("read {}: {error}", fixture.display())),
        )
        .expect("decode Stage 9 product corpus");
        assert_eq!(corpus.version, "stage9.product-slice.v10");
        assert!(corpus.node_version_cases.len() >= 4);
        for test_case in corpus.node_version_cases {
            let resolution = Resolution {
                configured_path: "/fixture/node".to_owned(),
                effective_path: "/fixture/node".to_owned(),
                source: "settings".to_owned(),
                resolved_path: Some("/fixture/node".into()),
                attempted_paths: vec!["/fixture/node".to_owned()],
                last_error: None,
            };
            let mut dependency = base_node_dependency(&resolution);
            apply_version_output(&mut dependency, test_case.output.trim());
            assert_eq!(
                dependency.status, test_case.expected_status,
                "case {} status",
                test_case.name
            );
            assert_eq!(
                dependency.detected_version, test_case.expected_detected_version,
                "case {} detected version",
                test_case.name
            );
            assert_eq!(
                dependency.message, test_case.expected_message,
                "case {} message",
                test_case.name
            );
        }
    }

    #[tokio::test]
    async fn actual_node_probe_reports_the_public_contract() {
        let dependency = inspect_node("").await;
        assert_eq!(dependency.id, "node");
        assert_eq!(dependency.minimum_version, "22.0.0");
        assert!(!dependency.attempted_paths.is_empty());
        if dependency.status == DependencyStatus::Ok {
            assert!(parse_version(&dependency.detected_version).is_ok());
            assert!(!dependency.resolved_path.is_empty());
        }
    }
}
