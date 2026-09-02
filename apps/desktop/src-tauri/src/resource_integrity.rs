use std::fs::File;
use std::io::{BufReader, Read};
use std::path::{Component, Path};

use serde::Deserialize;
use sha2::{Digest, Sha256};
use thiserror::Error;

const MANIFEST_SCHEMA: &str = "jftrade.tauri-runtime.v1";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ResourceManifest {
    schema_version: String,
    target: ResourceTarget,
    node_version: String,
    files: Vec<ResourceFile>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ResourceTarget {
    architecture: String,
    platform: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ResourceFile {
    resource: String,
    sha256: String,
}

pub(crate) fn verify_release_resources(resource_root: &Path) -> Result<(), ResourceIntegrityError> {
    let manifest_path = resource_root.join("runtime/node/manifest.json");
    let manifest: ResourceManifest =
        serde_json::from_reader(File::open(&manifest_path).map_err(|source| {
            ResourceIntegrityError::Read {
                path: manifest_path.clone(),
                source,
            }
        })?)?;
    if manifest.schema_version != MANIFEST_SCHEMA {
        return Err(ResourceIntegrityError::Schema(manifest.schema_version));
    }
    if manifest.target.platform != release_platform()
        || manifest.target.architecture != release_architecture()
    {
        return Err(ResourceIntegrityError::Target {
            actual: format!(
                "{}/{}",
                manifest.target.platform, manifest.target.architecture
            ),
            expected: format!("{}/{}", release_platform(), release_architecture()),
        });
    }
    if manifest.node_version.trim().is_empty() || manifest.files.is_empty() {
        return Err(ResourceIntegrityError::Incomplete);
    }
    for entry in manifest.files {
        let relative = Path::new(&entry.resource);
        if relative.is_absolute()
            || relative
                .components()
                .any(|component| !matches!(component, Component::Normal(_)))
        {
            return Err(ResourceIntegrityError::UnsafePath(entry.resource));
        }
        let path = resource_root.join(relative);
        let actual = sha256_file(&path)?;
        if actual != entry.sha256 {
            return Err(ResourceIntegrityError::Hash {
                path,
                expected: entry.sha256,
                actual,
            });
        }
    }
    Ok(())
}

fn sha256_file(path: &Path) -> Result<String, ResourceIntegrityError> {
    let file = File::open(path).map_err(|source| ResourceIntegrityError::Read {
        path: path.to_owned(),
        source,
    })?;
    let mut reader = BufReader::new(file);
    let mut buffer = [0_u8; 64 * 1024];
    let mut digest = Sha256::new();
    loop {
        let count = reader
            .read(&mut buffer)
            .map_err(|source| ResourceIntegrityError::Read {
                path: path.to_owned(),
                source,
            })?;
        if count == 0 {
            break;
        }
        digest.update(&buffer[..count]);
    }
    let digest = digest.finalize();
    let mut encoded = String::with_capacity(digest.len() * 2);
    const HEX: &[u8; 16] = b"0123456789abcdef";
    for byte in digest {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    Ok(encoded)
}

fn release_platform() -> &'static str {
    if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "linux"
    }
}

fn release_architecture() -> &'static str {
    if cfg!(target_arch = "aarch64") {
        "arm64"
    } else {
        "amd64"
    }
}

#[derive(Debug, Error)]
pub(crate) enum ResourceIntegrityError {
    #[error("read release resource {path}")]
    Read {
        path: std::path::PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("decode release resource manifest")]
    Decode(#[from] serde_json::Error),
    #[error("unsupported release resource schema {0:?}")]
    Schema(String),
    #[error("release resource target is {actual}, expected {expected}")]
    Target { actual: String, expected: String },
    #[error("release resource manifest is incomplete")]
    Incomplete,
    #[error("release resource path is unsafe: {0}")]
    UnsafePath(String),
    #[error("release resource hash mismatch for {path}: expected {expected}, got {actual}")]
    Hash {
        path: std::path::PathBuf,
        expected: String,
        actual: String,
    },
}

#[cfg(test)]
mod tests {
    use std::fs;

    use serde_json::json;

    use super::*;

    #[test]
    fn accepts_exact_resources_and_rejects_tampering() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let resource = directory.path().join("runtime/node/node");
        fs::create_dir_all(resource.parent().expect("resource parent")).expect("create resource");
        fs::write(&resource, b"node").expect("write resource");
        let manifest = json!({
            "schemaVersion": MANIFEST_SCHEMA,
            "target": {"platform": release_platform(), "architecture": release_architecture()},
            "nodeVersion": "v24.0.0",
            "files": [{"resource": "runtime/node/node", "sha256": sha256_file(&resource).expect("hash")}]
        });
        fs::write(
            directory.path().join("runtime/node/manifest.json"),
            serde_json::to_vec(&manifest).expect("manifest"),
        )
        .expect("write manifest");
        verify_release_resources(directory.path()).expect("verify exact resources");

        fs::write(resource, b"tampered").expect("tamper resource");
        assert!(matches!(
            verify_release_resources(directory.path()),
            Err(ResourceIntegrityError::Hash { .. })
        ));
    }

    #[test]
    fn rejects_manifest_path_escape_before_opening_it() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let manifest_directory = directory.path().join("runtime/node");
        fs::create_dir_all(&manifest_directory).expect("create manifest directory");
        let manifest = json!({
            "schemaVersion": MANIFEST_SCHEMA,
            "target": {"platform": release_platform(), "architecture": release_architecture()},
            "nodeVersion": "v24.0.0",
            "files": [{"resource": "../outside", "sha256": "0".repeat(64)}]
        });
        fs::write(
            manifest_directory.join("manifest.json"),
            serde_json::to_vec(&manifest).expect("manifest"),
        )
        .expect("write manifest");
        assert!(matches!(
            verify_release_resources(directory.path()),
            Err(ResourceIntegrityError::UnsafePath(_))
        ));
    }
}
