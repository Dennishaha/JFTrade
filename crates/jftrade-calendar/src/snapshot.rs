use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};

use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use tempfile::Builder;
use thiserror::Error;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarSessionWindow {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TradingDaySchedule {
    pub market_code: String,
    pub date: WireTimestamp,
    pub status: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub sessions: Vec<CalendarSessionWindow>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub reason: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub source_id: String,
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub observed: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarSnapshot {
    pub market_code: String,
    pub source_id: String,
    pub from: WireTimestamp,
    pub to: WireTimestamp,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub schedules: Vec<TradingDaySchedule>,
    pub fetched_at: WireTimestamp,
    pub valid_until: WireTimestamp,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub checksum: String,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CalendarSnapshotLoadErrorKind {
    Walk,
    Read,
    Decode,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CalendarSnapshotLoadError {
    pub path: PathBuf,
    pub kind: CalendarSnapshotLoadErrorKind,
    pub message: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct CalendarSnapshotLoadResult {
    pub snapshots: Vec<CalendarSnapshot>,
    pub errors: Vec<CalendarSnapshotLoadError>,
}

#[derive(Clone, Debug)]
pub struct CalendarSnapshotStore {
    root: PathBuf,
}

#[derive(Debug, Error)]
pub enum CalendarSnapshotStoreError {
    #[error("exchange calendar store root is empty")]
    EmptyRoot,
    #[error("snapshot marketCode and sourceId are required")]
    MissingIdentity,
    #[error("snapshot marketCode or sourceId is not a safe path component")]
    UnsafeIdentity,
    #[error("snapshot year is required")]
    MissingYear,
    #[error("create exchange calendar snapshot directory: {0}")]
    CreateDirectory(std::io::Error),
    #[error("marshal exchange calendar snapshot: {0}")]
    Encode(serde_json::Error),
    #[error("write exchange calendar snapshot: {0}")]
    Write(std::io::Error),
}

impl CalendarSnapshotStore {
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    pub fn root(&self) -> &Path {
        &self.root
    }

    pub fn snapshot_path(
        &self,
        snapshot: &CalendarSnapshot,
    ) -> Result<PathBuf, CalendarSnapshotStoreError> {
        let market = snapshot.market_code.trim().to_uppercase();
        let source = snapshot.source_id.trim();
        if market.is_empty() || source.is_empty() {
            return Err(CalendarSnapshotStoreError::MissingIdentity);
        }
        if !safe_component(&market) || !safe_component(source) {
            return Err(CalendarSnapshotStoreError::UnsafeIdentity);
        }
        let year = snapshot.from.into_inner().year();
        if year <= 0 {
            return Err(CalendarSnapshotStoreError::MissingYear);
        }
        Ok(self
            .root
            .join(market)
            .join(format!("{year:04}"))
            .join(format!("{source}.json")))
    }

    pub fn save(&self, snapshot: &CalendarSnapshot) -> Result<PathBuf, CalendarSnapshotStoreError> {
        if self.root.as_os_str().is_empty() {
            return Err(CalendarSnapshotStoreError::EmptyRoot);
        }
        let path = self.snapshot_path(snapshot)?;
        let directory = path.parent().expect("snapshot path always has a parent");
        fs::create_dir_all(directory).map_err(CalendarSnapshotStoreError::CreateDirectory)?;
        set_directory_permissions(directory)
            .map_err(CalendarSnapshotStoreError::CreateDirectory)?;
        let mut body =
            serde_json::to_vec_pretty(snapshot).map_err(CalendarSnapshotStoreError::Encode)?;
        body.push(b'\n');
        write_atomic(directory, &path, &body).map_err(CalendarSnapshotStoreError::Write)?;
        Ok(path)
    }

    pub fn load(&self) -> CalendarSnapshotLoadResult {
        let mut result = CalendarSnapshotLoadResult::default();
        if self.root.as_os_str().is_empty() {
            return result;
        }
        load_directory(&self.root, &mut result);
        result.snapshots.sort_by_key(snapshot_sort_key);
        result
            .errors
            .sort_by(|left, right| left.path.cmp(&right.path));
        result
    }
}

fn safe_component(value: &str) -> bool {
    value != "."
        && value != ".."
        && !value
            .chars()
            .any(|character| matches!(character, '/' | '\\' | ':' | '\0'))
}

fn write_atomic(directory: &Path, path: &Path, body: &[u8]) -> std::io::Result<()> {
    let mut temporary = Builder::new()
        .prefix(".calendar-snapshot-")
        .suffix(".tmp")
        .tempfile_in(directory)?;
    set_file_permissions(temporary.as_file())?;
    temporary.write_all(body)?;
    temporary.as_file().sync_all()?;
    temporary.persist(path).map_err(|error| error.error)?;
    sync_directory(directory)
}

fn load_directory(path: &Path, result: &mut CalendarSnapshotLoadResult) {
    let entries = match fs::read_dir(path) {
        Ok(entries) => entries,
        Err(error) => {
            result
                .errors
                .push(load_error(path, CalendarSnapshotLoadErrorKind::Walk, error));
            return;
        }
    };
    for entry in entries {
        let entry = match entry {
            Ok(entry) => entry,
            Err(error) => {
                result
                    .errors
                    .push(load_error(path, CalendarSnapshotLoadErrorKind::Walk, error));
                continue;
            }
        };
        let entry_path = entry.path();
        let file_type = match entry.file_type() {
            Ok(file_type) => file_type,
            Err(error) => {
                result.errors.push(load_error(
                    &entry_path,
                    CalendarSnapshotLoadErrorKind::Walk,
                    error,
                ));
                continue;
            }
        };
        if file_type.is_dir() {
            load_directory(&entry_path, result);
        } else if entry_path
            .extension()
            .is_some_and(|extension| extension == "json")
        {
            load_file(&entry_path, result);
        }
    }
}

fn load_file(path: &Path, result: &mut CalendarSnapshotLoadResult) {
    let body = match fs::read(path) {
        Ok(body) => body,
        Err(error) => {
            result
                .errors
                .push(load_error(path, CalendarSnapshotLoadErrorKind::Read, error));
            return;
        }
    };
    match serde_json::from_slice(&body) {
        Ok(snapshot) => result.snapshots.push(snapshot),
        Err(error) => result.errors.push(CalendarSnapshotLoadError {
            path: path.to_path_buf(),
            kind: CalendarSnapshotLoadErrorKind::Decode,
            message: error.to_string(),
        }),
    }
}

fn load_error(
    path: &Path,
    kind: CalendarSnapshotLoadErrorKind,
    error: std::io::Error,
) -> CalendarSnapshotLoadError {
    CalendarSnapshotLoadError {
        path: path.to_path_buf(),
        kind,
        message: error.to_string(),
    }
}

fn snapshot_sort_key(snapshot: &CalendarSnapshot) -> (String, String, WireTimestamp) {
    (
        snapshot.market_code.trim().to_uppercase(),
        snapshot.source_id.trim().to_owned(),
        snapshot.from,
    )
}

#[cfg(unix)]
fn set_directory_permissions(path: &Path) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o755))
}

#[cfg(not(unix))]
fn set_directory_permissions(_path: &Path) -> std::io::Result<()> {
    Ok(())
}

#[cfg(unix)]
fn set_file_permissions(file: &File) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;
    file.set_permissions(fs::Permissions::from_mode(0o644))
}

#[cfg(not(unix))]
fn set_file_permissions(_file: &File) -> std::io::Result<()> {
    Ok(())
}

#[cfg(unix)]
fn sync_directory(path: &Path) -> std::io::Result<()> {
    File::open(path)?.sync_all()
}

#[cfg(not(unix))]
fn sync_directory(_path: &Path) -> std::io::Result<()> {
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use serde::Deserialize;
    use tempfile::tempdir;

    use super::*;

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct FormatFixture {
        version: String,
        relative_path: String,
        file_contents: String,
        snapshot: CalendarSnapshot,
    }

    fn fixture() -> FormatFixture {
        serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/calendar-snapshot-format.json"
        ))
        .expect("decode Go-owned calendar snapshot fixture")
    }

    #[test]
    fn file_format_and_path_match_the_go_owner() {
        let fixture = fixture();
        assert_eq!(fixture.version, "stage9.calendar-snapshot-format.v1");
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let path = store.save(&fixture.snapshot).expect("save snapshot");
        assert_eq!(
            path.strip_prefix(directory.path())
                .expect("relative snapshot path")
                .to_string_lossy()
                .replace('\\', "/"),
            fixture.relative_path
        );
        assert_eq!(
            fs::read_to_string(&path).expect("read snapshot"),
            fixture.file_contents
        );
        let loaded = store.load();
        assert!(loaded.errors.is_empty());
        assert_eq!(loaded.snapshots, vec![fixture.snapshot]);
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                path.metadata()
                    .expect("snapshot metadata")
                    .permissions()
                    .mode()
                    & 0o777,
                0o644
            );
            assert_eq!(
                path.parent()
                    .expect("snapshot directory")
                    .metadata()
                    .expect("directory metadata")
                    .permissions()
                    .mode()
                    & 0o777,
                0o755
            );
        }
    }

    #[test]
    fn load_preserves_valid_snapshots_and_reports_each_bad_file_without_creating_root() {
        let directory = tempdir().expect("temporary directory");
        let missing = directory.path().join("missing");
        let missing_result = CalendarSnapshotStore::new(&missing).load();
        assert!(!missing.exists());
        assert_eq!(missing_result.errors.len(), 1);
        assert_eq!(
            missing_result.errors[0].kind,
            CalendarSnapshotLoadErrorKind::Walk
        );

        let fixture = fixture();
        let store = CalendarSnapshotStore::new(directory.path().join("snapshots"));
        let valid_path = store.save(&fixture.snapshot).expect("save valid snapshot");
        let bad_directory = valid_path.parent().expect("valid snapshot directory");
        fs::write(bad_directory.join("corrupt.json"), b"{").expect("write corrupt snapshot");
        fs::write(
            bad_directory.join("truncated.json"),
            br#"{"marketCode":"US""#,
        )
        .expect("write truncated snapshot");
        #[cfg(unix)]
        let unreadable_path = {
            use std::os::unix::fs::PermissionsExt;
            let path = bad_directory.join("unreadable.json");
            fs::write(&path, &fixture.file_contents).expect("write unreadable snapshot");
            fs::set_permissions(&path, fs::Permissions::from_mode(0o000))
                .expect("remove snapshot read permission");
            path
        };
        let loaded = store.load();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&unreadable_path, fs::Permissions::from_mode(0o600))
                .expect("restore snapshot read permission");
        }
        assert_eq!(loaded.snapshots, vec![fixture.snapshot]);
        #[cfg(unix)]
        assert_eq!(loaded.errors.len(), 3);
        #[cfg(not(unix))]
        assert_eq!(loaded.errors.len(), 2);
        assert_eq!(
            loaded
                .errors
                .iter()
                .filter(|error| error.kind == CalendarSnapshotLoadErrorKind::Decode)
                .count(),
            2
        );
        #[cfg(unix)]
        assert!(
            loaded
                .errors
                .iter()
                .any(|error| error.kind == CalendarSnapshotLoadErrorKind::Read)
        );
    }

    #[test]
    fn replacement_keeps_one_complete_snapshot_and_cleans_temporary_files() {
        let fixture = fixture();
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let path = store
            .save(&fixture.snapshot)
            .expect("save original snapshot");
        let mut replacement = fixture.snapshot;
        replacement.checksum = "replacement".to_owned();
        store.save(&replacement).expect("replace snapshot");
        let loaded = store.load();
        assert_eq!(loaded.snapshots, vec![replacement]);
        let peers = fs::read_dir(path.parent().expect("snapshot directory"))
            .expect("list snapshot directory")
            .map(|entry| entry.expect("directory entry").file_name())
            .collect::<Vec<_>>();
        assert_eq!(peers, [path.file_name().expect("snapshot filename")]);
    }

    #[test]
    fn unsafe_identity_is_rejected_before_filesystem_access() {
        let mut snapshot = fixture().snapshot;
        snapshot.source_id = "../escape".to_owned();
        let store = CalendarSnapshotStore::new("unused");
        assert!(matches!(
            store.snapshot_path(&snapshot),
            Err(CalendarSnapshotStoreError::UnsafeIdentity)
        ));
        snapshot.source_id = r"..\escape".to_owned();
        assert!(matches!(
            store.snapshot_path(&snapshot),
            Err(CalendarSnapshotStoreError::UnsafeIdentity)
        ));
        snapshot.source_id = "source".to_owned();
        snapshot.market_code = "/absolute".to_owned();
        assert!(matches!(
            store.snapshot_path(&snapshot),
            Err(CalendarSnapshotStoreError::UnsafeIdentity)
        ));
        assert!(WireTimestamp::from_str("2026-01-01T00:00:00Z").is_ok());
    }
}
