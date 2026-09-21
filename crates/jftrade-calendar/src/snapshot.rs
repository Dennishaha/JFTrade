use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};

use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use tempfile::Builder;
use thiserror::Error;
use time::{Month, Time, UtcOffset};

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
    /// Injectable failure points for the durability chain.
    ///
    /// Go's `Store` carries `createTemp`/`replaceFile` function-valued fields
    /// that its tests overwrite to prove each step of the atomic write
    /// propagates its error; the remaining steps are only reachable that way
    /// because a real filesystem will not fail on demand. The Rust store keeps
    /// the same shape with an internal fault switch: production never sets it
    /// (it stays all-`None`), and the module tests drive each edge.
    faults: StoreFaults,
}

#[derive(Clone, Debug, Default)]
struct StoreFaults {
    create_temp: Option<std::io::ErrorKind>,
    permissions: Option<std::io::ErrorKind>,
    write: Option<std::io::ErrorKind>,
    sync: Option<std::io::ErrorKind>,
    persist: Option<std::io::ErrorKind>,
    sync_directory: Option<std::io::ErrorKind>,
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
    #[error("delete exchange calendar snapshot: {0}")]
    Delete(std::io::Error),
}

impl CalendarSnapshotStore {
    /// Build a store rooted at `root`.
    ///
    /// Go's `New` runs `strings.TrimSpace` over the configured path, so a
    /// settings value that carries stray whitespace must not silently create a
    /// differently named directory. Non-UTF-8 paths are kept verbatim because
    /// there is no lossless way to trim them.
    pub fn new(root: impl Into<PathBuf>) -> Self {
        let root = root.into();
        let trimmed = root
            .to_str()
            .map_or_else(|| root.clone(), |value| PathBuf::from(value.trim()));
        Self {
            root: trimmed,
            faults: StoreFaults::default(),
        }
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
        let Some(year) = snapshot_year(snapshot) else {
            return Err(CalendarSnapshotStoreError::MissingYear);
        };
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
        write_atomic(directory, &path, &body, &self.faults)
            .map_err(CalendarSnapshotStoreError::Write)?;
        Ok(path)
    }

    /// Remove a snapshot file if it exists.  Restore uses this after rejecting
    /// a decoded value so a corrupt cache cannot be reconsidered on every
    /// startup; a malformed identity is reported rather than traversed.
    pub fn delete(&self, snapshot: &CalendarSnapshot) -> Result<(), CalendarSnapshotStoreError> {
        if self.root.as_os_str().is_empty() {
            return Ok(());
        }
        // Go's `DeleteSnapshot` returns nil when the snapshot carries no year,
        // and the manager reaches this call for cache entries that failed
        // validation — which very much includes identity-less values. A cache
        // entry that cannot name a file it owns is nothing to delete, so the
        // restore path must not turn it into a second recorded failure.
        let Ok(path) = self.snapshot_path(snapshot) else {
            return Ok(());
        };
        match fs::remove_file(path) {
            Ok(()) => Ok(()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(error) => Err(CalendarSnapshotStoreError::Delete(error)),
        }
    }

    pub fn delete_snapshot(
        &self,
        snapshot: &CalendarSnapshot,
    ) -> Result<(), CalendarSnapshotStoreError> {
        self.delete(snapshot)
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

fn snapshot_year(snapshot: &CalendarSnapshot) -> Option<i32> {
    [snapshot.from, snapshot.to]
        .into_iter()
        .chain(snapshot.schedules.iter().map(|schedule| schedule.date))
        .find_map(|timestamp| {
            (!is_zero_timestamp(timestamp)).then_some(timestamp.into_inner().year())
        })
        .filter(|year| *year > 0)
}

fn is_zero_timestamp(value: WireTimestamp) -> bool {
    let value = value.into_inner();
    value.year() == 1
        && value.month() == Month::January
        && value.day() == 1
        && value.time() == Time::MIDNIGHT
        && value.offset() == UtcOffset::UTC
}

fn injected(kind: Option<std::io::ErrorKind>, message: &str) -> std::io::Result<()> {
    match kind {
        Some(kind) => Err(std::io::Error::new(kind, message.to_owned())),
        None => Ok(()),
    }
}

/// Write the snapshot body to a temporary sibling and rename it into place.
///
/// The rename is the commit point: a failure anywhere earlier leaves the
/// previous snapshot untouched, and the temporary file is removed on every
/// failing path (a partially written `.calendar-snapshot-*.tmp` must never be
/// left for the loader to trip over).
fn write_atomic(
    directory: &Path,
    path: &Path,
    body: &[u8],
    faults: &StoreFaults,
) -> std::io::Result<()> {
    injected(faults.create_temp, "temporary snapshot creation failed")?;
    let mut temporary = Builder::new()
        .prefix(".calendar-snapshot-")
        .suffix(".tmp")
        .tempfile_in(directory)?;
    injected(faults.permissions, "temporary snapshot permissions failed")?;
    set_file_permissions(temporary.as_file())?;
    injected(faults.write, "temporary snapshot write failed")?;
    temporary.write_all(body)?;
    injected(faults.sync, "temporary snapshot sync failed")?;
    temporary.as_file().sync_all()?;
    injected(faults.persist, "temporary snapshot replacement failed")?;
    temporary.persist(path).map_err(|error| error.error)?;
    injected(faults.sync_directory, "snapshot directory sync failed")?;
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

/// Flush a directory so the rename written by [`write_atomic`] is durable.
///
/// Go exposes `syncSnapshotDirectory` as a package-level function and asserts
/// that a missing directory is a real error; this stays directly testable for
/// the same reason.
pub(crate) fn sync_directory(path: &Path) -> std::io::Result<()> {
    File::open(path)?.sync_all()
}

#[cfg(not(unix))]
pub(crate) fn sync_directory(_path: &Path) -> std::io::Result<()> {
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
            "../../../tests/fixtures/compatibility/api-transport/calendar-snapshot-format.json"
        ))
        .expect("decode Go-owned calendar snapshot fixture")
    }

    #[test]
    fn trading_day_schedule_json_omits_zero_updated_at() {
        // Parity: go:452dea11:pkg/market/calendar/types_json_test.go:10 TestTradingDayScheduleJSONOmitsZeroUpdatedAt
        use time::OffsetDateTime;
        use time::format_description::well_known::Rfc3339;

        let date = WireTimestamp::from_offset_datetime(
            OffsetDateTime::parse("2026-06-23T00:00:00Z", &Rfc3339).expect("schedule date"),
        );
        let zero = serde_json::to_value(TradingDaySchedule {
            market_code: "US".to_owned(),
            date,
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: String::new(),
            observed: false,
            updated_at: None,
        })
        .expect("serialize zero schedule");
        assert!(
            zero.get("updatedAt").is_none(),
            "zero schedule leaked updatedAt: {zero}"
        );

        let updated_at = WireTimestamp::from_offset_datetime(
            OffsetDateTime::parse("2026-06-23T09:30:00Z", &Rfc3339).expect("updated at"),
        );
        let nonzero = serde_json::to_value(TradingDaySchedule {
            market_code: "US".to_owned(),
            date,
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: String::new(),
            observed: false,
            updated_at: Some(updated_at),
        })
        .expect("serialize nonzero schedule");
        assert_eq!(nonzero["updatedAt"], "2026-06-23T09:30:00Z");
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

    #[test]
    fn test_calendar_store_empty_load_and_delete_are_idempotent() {
        // Parity: internal/store/exchangecalendar/store_boundaries_test.go:45 TestCalendarStoreEmptyLoadAndDeleteAreIdempotent
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let loaded = store.load();
        assert!(loaded.snapshots.is_empty());
        assert!(loaded.errors.is_empty());

        // Store with non-existent root
        let non_existent = CalendarSnapshotStore::new(directory.path().join("non_existent"));
        let loaded_empty = non_existent.load();
        assert!(loaded_empty.snapshots.is_empty());

        let sample_snapshot = CalendarSnapshot {
            market_code: "US".to_string(),
            source_id: "nyse_official".to_string(),
            from: WireTimestamp::from_str("2026-01-01T00:00:00Z").unwrap(),
            to: WireTimestamp::from_str("2026-12-31T00:00:00Z").unwrap(),
            schedules: Vec::new(),
            fetched_at: WireTimestamp::from_str("2026-01-01T00:00:00Z").unwrap(),
            valid_until: WireTimestamp::from_str("2026-12-31T00:00:00Z").unwrap(),
            checksum: "chk".to_string(),
        };

        // Delete with empty store root succeeds idempotently
        let empty_store = CalendarSnapshotStore::new(Path::new(""));
        assert!(empty_store.delete(&sample_snapshot).is_ok());

        // Delete with non-existent snapshot file returns Ok(())
        assert!(store.delete(&sample_snapshot).is_ok());
    }

    fn store_snapshot(market: &str, source: &str) -> CalendarSnapshot {
        CalendarSnapshot {
            market_code: market.to_owned(),
            source_id: source.to_owned(),
            from: WireTimestamp::from_str("2026-01-01T00:00:00Z").expect("from"),
            to: WireTimestamp::from_str("2026-12-31T23:59:59Z").expect("to"),
            schedules: Vec::new(),
            fetched_at: WireTimestamp::from_str("2026-01-02T00:00:00Z").expect("fetchedAt"),
            valid_until: WireTimestamp::from_str("2026-02-01T00:00:00Z").expect("validUntil"),
            checksum: String::new(),
        }
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_test.go:12
    /// TestStoreRoundTripsSnapshotsAndIsolatesCorruption.
    ///
    /// A written snapshot reads back verbatim, a corrupt neighbour is reported
    /// as one decode error without hiding the healthy value, and deleting the
    /// snapshot removes exactly its own file.
    #[test]
    fn store_round_trips_snapshots_and_isolates_corruption() {
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let mut snapshot = store_snapshot("US", "nyse_official");
        snapshot.schedules = vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: WireTimestamp::from_str("2026-06-19T00:00:00Z").expect("schedule date"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "juneteenth".to_owned(),
            source_id: String::new(),
            observed: false,
            updated_at: None,
        }];
        let path = store.save(&snapshot).expect("save snapshot");
        fs::write(
            path.parent()
                .expect("snapshot directory")
                .join("broken.json"),
            b"{",
        )
        .expect("write broken neighbour");

        let loaded = store.load();
        assert_eq!(loaded.snapshots.len(), 1, "errors = {:?}", loaded.errors);
        assert_eq!(loaded.errors.len(), 1, "errors = {:?}", loaded.errors);
        assert_eq!(loaded.errors[0].kind, CalendarSnapshotLoadErrorKind::Decode);
        assert_eq!(loaded.snapshots[0].source_id, "nyse_official");
        assert_eq!(loaded.snapshots[0].schedules[0].reason, "juneteenth");

        store.delete(&snapshot).expect("delete snapshot");
        assert!(
            !path.exists(),
            "the snapshot file itself must be removed: {}",
            path.display()
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_test.go:58
    /// TestStoreUsesSnapshotLocalYearForPositiveOffsetMarkets.
    ///
    /// The HK feed is published in +08:00; the cache key must use the market's
    /// local year, otherwise a January 1st instant stored as UTC would land in
    /// the previous year's directory.
    #[test]
    fn store_uses_the_snapshot_local_year_for_positive_offset_markets() {
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let mut snapshot = store_snapshot("HK", "hk_gov_1823_ical");
        snapshot.from = WireTimestamp::from_str("2026-01-01T00:00:00+08:00").expect("from");
        snapshot.to = WireTimestamp::from_str("2027-12-31T23:59:59+08:00").expect("to");
        snapshot.schedules = vec![TradingDaySchedule {
            market_code: "HK".to_owned(),
            date: WireTimestamp::from_str("2026-06-19T00:00:00+08:00").expect("schedule date"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "tuen_ng_festival".to_owned(),
            source_id: String::new(),
            observed: false,
            updated_at: None,
        }];
        let path = store.save(&snapshot).expect("save HK snapshot");
        assert!(
            path.ends_with("HK/2026/hk_gov_1823_ical.json"),
            "path = {}",
            path.display()
        );
        assert!(path.exists());
        assert_eq!(store.load().snapshots, vec![snapshot]);
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:13
    /// TestCalendarStoreRejectsInvalidSnapshotPersistence.
    ///
    /// Every invalid input is refused by its own message before any filesystem
    /// access: empty root, missing market/source, and a snapshot with no year
    /// anywhere (from, to or schedule dates).
    #[test]
    fn store_rejects_invalid_snapshot_persistence_with_named_errors() {
        let directory = tempdir().expect("temporary directory");
        let empty_root = CalendarSnapshotStore::new("");
        let error = empty_root
            .save(&store_snapshot("US", "nyse_official"))
            .expect_err("an empty root is rejected");
        assert_eq!(
            error.to_string(),
            "exchange calendar store root is empty",
            "the empty root keeps Go's message"
        );

        let store = CalendarSnapshotStore::new(directory.path());
        let error = store
            .save(&store_snapshot("US", ""))
            .expect_err("a missing source is rejected");
        assert_eq!(
            error.to_string(),
            "snapshot marketCode and sourceId are required"
        );

        let yearless = CalendarSnapshot {
            market_code: "US".to_owned(),
            source_id: "source".to_owned(),
            from: WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero from"),
            to: WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero to"),
            schedules: Vec::new(),
            fetched_at: WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero fetchedAt"),
            valid_until: WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero validUntil"),
            checksum: String::new(),
        };
        let error = store
            .save(&yearless)
            .expect_err("a yearless snapshot is rejected");
        assert_eq!(error.to_string(), "snapshot year is required");
        assert!(
            fs::read_dir(directory.path())
                .expect("list root")
                .next()
                .is_none(),
            "a rejected snapshot must not touch the filesystem"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_boundaries_test.go:31
    /// TestCalendarStoreReportsUnavailableSnapshotDirectory.
    ///
    /// A root path that is already a regular file cannot host a snapshot
    /// directory, and the failure must name the directory creation step.
    #[test]
    fn store_reports_an_unavailable_snapshot_directory() {
        let directory = tempdir().expect("temporary directory");
        let occupied = directory.path().join("occupied");
        fs::write(&occupied, b"file").expect("occupy the root path");
        let mut snapshot = store_snapshot("US", "source");
        snapshot.from = WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero from");
        snapshot.to = WireTimestamp::from_str("0001-01-01T00:00:00Z").expect("zero to");
        snapshot.schedules = vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: WireTimestamp::from_str("2026-07-02T00:00:00Z").expect("schedule date"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: String::new(),
            observed: false,
            updated_at: None,
        }];

        let error = CalendarSnapshotStore::new(&occupied)
            .save(&snapshot)
            .expect_err("a file cannot host snapshots");
        assert!(
            error
                .to_string()
                .starts_with("create exchange calendar snapshot directory"),
            "the failing step is named: {error}"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:31
    /// TestSaveSnapshotUsesAtomicReplacement.
    ///
    /// A failed replacement leaves the previously stored snapshot byte-identical
    /// and cleans up the temporary file, so a reader never observes a partial
    /// or empty cache entry.
    #[test]
    fn save_uses_atomic_replacement_that_keeps_the_previous_snapshot_on_failure() {
        let directory = tempdir().expect("temporary directory");
        let mut store = CalendarSnapshotStore::new(directory.path());
        let original = store_snapshot("US", "nyse_official");
        let path = store.save(&original).expect("save the original snapshot");
        let before = fs::read(&path).expect("read the original body");

        store.faults.persist = Some(std::io::ErrorKind::Other);
        let mut replacement = original.clone();
        replacement.checksum = "replacement".to_owned();
        let error = store
            .save(&replacement)
            .expect_err("the refused rename is reported");
        assert!(
            error
                .to_string()
                .starts_with("write exchange calendar snapshot"),
            "the failing step is named: {error}"
        );
        assert_eq!(
            fs::read(&path).expect("read after the failed replacement"),
            before,
            "the committed snapshot is untouched by a failed replacement"
        );

        let leftovers = fs::read_dir(path.parent().expect("snapshot directory"))
            .expect("list the snapshot directory")
            .map(|entry| entry.expect("directory entry").file_name())
            .filter(|name| name.to_string_lossy().starts_with(".calendar-snapshot-"))
            .collect::<Vec<_>>();
        assert!(
            leftovers.is_empty(),
            "temporary files must be cleaned up: {leftovers:?}"
        );
        assert_eq!(store.load().snapshots, vec![original]);
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:73
    /// TestStoreRootAndNilSafety.
    ///
    /// Go's `New` trims surrounding whitespace from the configured root and
    /// `Root()` is nil-safe. Rust has no nil receiver, so the boundary is the
    /// empty root: it stays usable for reads/deletes and refuses writes.
    #[test]
    fn store_root_is_trimmed_and_the_empty_root_stays_safe() {
        let directory = tempdir().expect("temporary directory");
        let configured = format!("  {}  ", directory.path().join("calendars").display());
        let store = CalendarSnapshotStore::new(configured);
        assert_eq!(
            store.root(),
            directory.path().join("calendars"),
            "surrounding whitespace is trimmed like Go's New"
        );

        let empty = CalendarSnapshotStore::new("  ");
        assert!(empty.root().as_os_str().is_empty());
        let loaded = empty.load();
        assert!(loaded.snapshots.is_empty() && loaded.errors.is_empty());
        empty
            .delete(&store_snapshot("US", "source"))
            .expect("deleting from an empty root is a no-op");
        assert!(
            empty.save(&store_snapshot("US", "source")).is_err(),
            "saving into an empty root fails"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:136
    /// TestDeleteSnapshotIgnoresMissingFilesAndReturnsRealRemoveErrors.
    ///
    /// A missing snapshot and a yearless snapshot are both no-ops, while a real
    /// removal failure (here: the snapshot path is a non-empty directory) is
    /// reported with the owning path.
    #[test]
    fn delete_ignores_missing_files_and_reports_real_remove_errors() {
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());
        let snapshot = store_snapshot("US", "nyse_official");
        store
            .delete(&snapshot)
            .expect("deleting a missing snapshot is a no-op");
        store
            .delete(&CalendarSnapshot {
                market_code: String::new(),
                source_id: String::new(),
                ..snapshot.clone()
            })
            .expect("deleting a yearless/identity-less snapshot is a no-op");

        let path = store.snapshot_path(&snapshot).expect("snapshot path");
        fs::create_dir_all(&path).expect("create a directory in the snapshot's place");
        fs::write(path.join("nested"), b"x").expect("make the directory non-empty");
        let error = store
            .delete(&snapshot)
            .expect_err("removing a directory is a real failure");
        assert!(
            error
                .to_string()
                .starts_with("delete exchange calendar snapshot"),
            "the failing step is named: {error}"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:169
    /// TestWriteSnapshotPropagatesTemporaryFileDurabilityFailures.
    ///
    /// Each step of the durability chain (create, chmod, write, fsync, rename,
    /// directory fsync) propagates its own failure instead of silently
    /// committing a partial file.
    #[test]
    fn write_snapshot_propagates_each_temporary_file_durability_failure() {
        type FaultStep = (&'static str, fn(&mut StoreFaults));
        let steps: [FaultStep; 6] = [
            ("create_temp", |faults| {
                faults.create_temp = Some(std::io::ErrorKind::Other);
            }),
            ("permissions", |faults| {
                faults.permissions = Some(std::io::ErrorKind::Other);
            }),
            ("write", |faults| {
                faults.write = Some(std::io::ErrorKind::Other);
            }),
            ("sync", |faults| {
                faults.sync = Some(std::io::ErrorKind::Other);
            }),
            ("persist", |faults| {
                faults.persist = Some(std::io::ErrorKind::Other);
            }),
            ("sync_directory", |faults| {
                faults.sync_directory = Some(std::io::ErrorKind::Other);
            }),
        ];
        for (step, configure) in steps {
            let directory = tempdir().expect("temporary directory");
            let mut store = CalendarSnapshotStore::new(directory.path());
            configure(&mut store.faults);
            let error = match store.save(&store_snapshot("US", "nyse_official")) {
                Ok(path) => panic!(
                    "{step} must fail the save, but committed {}",
                    path.display()
                ),
                Err(error) => error,
            };
            assert!(
                error
                    .to_string()
                    .starts_with("write exchange calendar snapshot"),
                "{step} must surface through the write step: {error}"
            );
            let loaded = store.load();
            if step == "sync_directory" {
                // The rename is the commit point, so the body is in place; only
                // its durability guarantee failed. Everything else must not.
                assert_eq!(loaded.snapshots.len(), 1, "{step}: {loaded:?}");
            } else {
                assert!(
                    loaded.snapshots.is_empty(),
                    "{step} must not commit a snapshot: {loaded:?}"
                );
            }
            let leftovers = fs::read_dir(
                store
                    .snapshot_path(&store_snapshot("US", "nyse_official"))
                    .expect("snapshot path")
                    .parent()
                    .expect("snapshot directory"),
            )
            .expect("list the snapshot directory")
            .map(|entry| entry.expect("directory entry").file_name())
            .filter(|name| name.to_string_lossy().starts_with(".calendar-snapshot-"))
            .collect::<Vec<_>>();
            assert!(
                leftovers.is_empty(),
                "{step} must not leave a temporary file behind: {leftovers:?}"
            );
        }
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:199
    /// TestWriteSnapshotDefaultHooksAndDirectorySyncErrors.
    ///
    /// With no fault injected the full chain runs against the real filesystem,
    /// and `sync_directory` reports a missing directory (Go asserts the same on
    /// its package-level helper).
    #[test]
    fn write_snapshot_default_hooks_and_directory_sync_errors() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("snapshot.json");
        write_atomic(directory.path(), &path, b"{}\n", &StoreFaults::default())
            .expect("the default chain commits the file");
        assert_eq!(fs::read(&path).expect("read committed file"), b"{}\n");

        let missing = directory.path().join("missing");
        assert!(
            sync_directory(&missing).is_err(),
            "syncing a missing directory is a real error"
        );
        assert!(
            sync_directory(directory.path()).is_ok(),
            "syncing an existing directory succeeds"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/store_snapshot_failures_test.go:214
    /// TestSaveSnapshotReturnsDirectoryCreationError.
    ///
    /// When the configured root is a regular file, snapshot saving surfaces the
    /// directory-creation failure rather than a later encode/write error.
    #[test]
    fn save_returns_the_directory_creation_error() {
        let directory = tempdir().expect("temporary directory");
        let root = directory.path().join("calendar-root");
        fs::write(&root, b"not a directory").expect("occupy the root");
        let error = CalendarSnapshotStore::new(&root)
            .save(&store_snapshot("US", "nyse_official"))
            .expect_err("a file root cannot hold snapshots");
        assert!(
            error
                .to_string()
                .starts_with("create exchange calendar snapshot directory"),
            "the failing step is named: {error}"
        );
    }

    /// Parity: go:452dea11:internal/store/exchangecalendar/snapshot_load_failures_test.go:12
    /// TestLoadSnapshotsReportsWalkAndReadFailures.
    ///
    /// A missing root is one walk error rather than a panic or an empty result,
    /// and a snapshot path that cannot be read is one read error while other
    /// snapshots still load.
    #[test]
    fn load_reports_walk_and_read_failures_without_losing_valid_snapshots() {
        let directory = tempdir().expect("temporary directory");
        let missing = CalendarSnapshotStore::new(directory.path().join("missing"));
        let loaded = missing.load();
        assert!(loaded.snapshots.is_empty());
        assert_eq!(loaded.errors.len(), 1);
        assert_eq!(loaded.errors[0].kind, CalendarSnapshotLoadErrorKind::Walk);
        assert!(
            !missing.root().exists(),
            "a failed walk must not create the root"
        );

        let store = CalendarSnapshotStore::new(directory.path().join("snapshots"));
        let valid = store
            .save(&store_snapshot("US", "nyse_official"))
            .expect("save");
        let broken = valid.parent().expect("directory").join("broken.json");
        #[cfg(unix)]
        {
            std::os::unix::fs::symlink(
                valid.parent().expect("directory").join("does-not-exist"),
                &broken,
            )
            .expect("create a broken snapshot symlink");
            let loaded = store.load();
            assert_eq!(loaded.snapshots.len(), 1, "errors = {:?}", loaded.errors);
            assert_eq!(loaded.errors.len(), 1, "errors = {:?}", loaded.errors);
            assert_eq!(loaded.errors[0].kind, CalendarSnapshotLoadErrorKind::Read);
            assert_eq!(loaded.errors[0].path, broken);
        }
    }

    #[test]
    fn test_save_snapshot_validates_inputs_and_resolves_year_fallbacks() {
        // Parity: internal/store/exchangecalendar/store_snapshot_failures_test.go:95 TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks
        let directory = tempdir().expect("temporary directory");
        let store = CalendarSnapshotStore::new(directory.path());

        // Empty market / source fails validation
        let empty = CalendarSnapshot {
            market_code: "".to_string(),
            source_id: "".to_string(),
            from: WireTimestamp::from_str("2026-01-01T00:00:00Z").unwrap(),
            to: WireTimestamp::from_str("2026-12-31T00:00:00Z").unwrap(),
            schedules: Vec::new(),
            fetched_at: WireTimestamp::from_str("2026-01-01T00:00:00Z").unwrap(),
            valid_until: WireTimestamp::from_str("2026-12-31T00:00:00Z").unwrap(),
            checksum: "".to_string(),
        };
        assert!(store.save(&empty).is_err());

        // Valid snapshot resolves path by year
        let mut valid = empty;
        valid.market_code = "US".to_string();
        valid.source_id = "nyse_official".to_string();
        let path = store.save(&valid).expect("save valid snapshot");
        assert!(path.ends_with("US/2026/nyse_official.json"));
        assert!(path.exists());
    }
}
