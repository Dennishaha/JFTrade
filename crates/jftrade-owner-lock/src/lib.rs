#![forbid(unsafe_code)]

//! Cross-process exclusive writer leases for migration persistence resources.

use std::fs::{File, OpenOptions};
use std::io::{Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::Serialize;
use thiserror::Error;

pub const LOCK_SUFFIX: &str = ".jftrade-owner.lock";

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OwnerDiagnostic {
    pub owner: String,
    pub pid: u32,
    pub start: u64,
    pub profile: String,
}

impl OwnerDiagnostic {
    pub fn current(owner: impl Into<String>, profile: impl Into<String>) -> Self {
        Self {
            owner: owner.into(),
            pid: std::process::id(),
            start: u64::try_from(
                SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_millis(),
            )
            .unwrap_or(u64::MAX),
            profile: profile.into(),
        }
    }
}

#[derive(Debug, Error)]
pub enum WriterLeaseError {
    #[error("writer lease target path is required")]
    EmptyTarget,
    #[error("open writer lease {path}: {source}")]
    Open {
        path: String,
        #[source]
        source: std::io::Error,
    },
    #[error("writer lease is already held for {path}")]
    Held { path: String },
    #[error("lock writer lease {path}: {source}")]
    Lock {
        path: String,
        #[source]
        source: std::io::Error,
    },
    #[error("write writer lease diagnostic {path}: {source}")]
    Diagnostic {
        path: String,
        #[source]
        source: std::io::Error,
    },
    #[error("encode writer lease diagnostic {path}: {source}")]
    Encode {
        path: String,
        #[source]
        source: serde_json::Error,
    },
}

pub struct WriterLease {
    file: File,
    path: PathBuf,
}

impl WriterLease {
    pub fn acquire(
        target: impl AsRef<Path>,
        diagnostic: &OwnerDiagnostic,
    ) -> Result<Self, WriterLeaseError> {
        let target = target.as_ref();
        if target.as_os_str().is_empty() {
            return Err(WriterLeaseError::EmptyTarget);
        }
        let path = lock_path(target);
        let display = path.display().to_string();
        let mut file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(false)
            .open(&path)
            .map_err(|source| WriterLeaseError::Open {
                path: display.clone(),
                source,
            })?;
        secure_permissions(&file).map_err(|source| WriterLeaseError::Open {
            path: display.clone(),
            source,
        })?;
        if let Err(source) = file.try_lock() {
            return match source {
                std::fs::TryLockError::WouldBlock => Err(WriterLeaseError::Held { path: display }),
                std::fs::TryLockError::Error(source) => Err(WriterLeaseError::Lock {
                    path: display,
                    source,
                }),
            };
        }
        if let Err(error) = write_diagnostic(&mut file, &path, diagnostic) {
            let _ = file.unlock();
            return Err(error);
        }
        Ok(Self { file, path })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for WriterLease {
    fn drop(&mut self) {
        let _ = self.file.unlock();
    }
}

pub fn lock_path(target: impl AsRef<Path>) -> PathBuf {
    let mut value = target.as_ref().as_os_str().to_owned();
    value.push(LOCK_SUFFIX);
    PathBuf::from(value)
}

fn write_diagnostic(
    file: &mut File,
    path: &Path,
    diagnostic: &OwnerDiagnostic,
) -> Result<(), WriterLeaseError> {
    let display = path.display().to_string();
    file.set_len(0)
        .and_then(|()| file.seek(SeekFrom::Start(0)).map(|_| ()))
        .map_err(|source| WriterLeaseError::Diagnostic {
            path: display.clone(),
            source,
        })?;
    serde_json::to_writer(&mut *file, diagnostic).map_err(|source| WriterLeaseError::Encode {
        path: display.clone(),
        source,
    })?;
    file.write_all(b"\n")
        .and_then(|()| file.sync_all())
        .map_err(|source| WriterLeaseError::Diagnostic {
            path: display,
            source,
        })
}

#[cfg(unix)]
fn secure_permissions(file: &File) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    file.set_permissions(std::fs::Permissions::from_mode(0o600))
}

#[cfg(windows)]
fn secure_permissions(_file: &File) -> std::io::Result<()> {
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exclusive_lock_conflicts_and_file_survives_release() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let target = directory.path().join("settings.json");
        let diagnostic = OwnerDiagnostic::current("rust-test", "rehearsal");
        let first = WriterLease::acquire(&target, &diagnostic).expect("first lease");
        let error = WriterLease::acquire(&target, &diagnostic)
            .err()
            .expect("conflicting lease");
        assert!(matches!(error, WriterLeaseError::Held { .. }));
        let path = first.path().to_owned();
        let persisted: serde_json::Value =
            serde_json::from_slice(&std::fs::read(&path).expect("read diagnostic"))
                .expect("decode diagnostic");
        assert_eq!(persisted["owner"], "rust-test");
        assert_eq!(persisted["profile"], "rehearsal");
        assert!(persisted["start"].is_u64());
        drop(first);
        assert!(path.exists(), "lock file must not be deleted on release");
        WriterLease::acquire(&target, &diagnostic).expect("lease after release");
    }
}
