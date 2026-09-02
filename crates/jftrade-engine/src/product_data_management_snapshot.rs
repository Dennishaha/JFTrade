use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug)]
pub(super) struct DatabaseFileSnapshot {
    path: PathBuf,
    backup: Option<tempfile::NamedTempFile>,
}

fn snapshot_database_files(
    descriptor: &jftrade_datamanagement::DatabaseDescriptor,
) -> Result<Vec<DatabaseFileSnapshot>, String> {
    let mut snapshots = Vec::with_capacity(4);
    for suffix in ["", "-wal", "-shm", "-journal"] {
        let path = PathBuf::from(format!("{}{suffix}", descriptor.path));
        let backup = tempfile::NamedTempFile::new().map_err(|error| {
            format!("create temporary snapshot for {}: {error}", path.display())
        })?;
        let backup = match fs::copy(&path, backup.path()) {
            Ok(_) => {
                backup.as_file().sync_all().map_err(|error| {
                    format!("sync temporary snapshot for {}: {error}", path.display())
                })?;
                Some(backup)
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => None,
            Err(error) => {
                return Err(format!(
                    "snapshot managed database {}: {error}",
                    path.display()
                ));
            }
        };
        snapshots.push(DatabaseFileSnapshot { path, backup });
    }
    Ok(snapshots)
}

pub(super) fn snapshot_all_database_files(
    descriptors: &[jftrade_datamanagement::DatabaseDescriptor],
    order: &[usize],
) -> Result<Vec<Option<Vec<DatabaseFileSnapshot>>>, String> {
    let mut snapshots = (0..descriptors.len())
        .map(|_| None)
        .collect::<Vec<Option<Vec<DatabaseFileSnapshot>>>>();
    for &index in order {
        snapshots[index] = Some(snapshot_database_files(&descriptors[index])?);
    }
    Ok(snapshots)
}

fn restore_database_files(snapshots: &[DatabaseFileSnapshot]) -> Result<(), String> {
    let mut errors = Vec::new();
    for snapshot in snapshots {
        match snapshot.backup.as_ref() {
            Some(backup) => {
                if let Err(error) = restore_database_file(&snapshot.path, backup) {
                    errors.push(error);
                }
            }
            None => match fs::remove_file(&snapshot.path) {
                Ok(()) => {}
                Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
                Err(error) => errors.push(format!(
                    "remove newly-created database sidecar {}: {error}",
                    snapshot.path.display()
                )),
            },
        }
    }
    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("; "))
    }
}

pub(super) fn restore_all_database_files(
    snapshots: &[Option<Vec<DatabaseFileSnapshot>>],
    order: &[usize],
) -> Result<(), String> {
    let mut errors = Vec::new();
    for &index in order.iter().rev() {
        if let Some(files) = snapshots[index].as_deref()
            && let Err(error) = restore_database_files(files)
        {
            errors.push(error);
        }
    }
    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("; "))
    }
}

fn restore_database_file(path: &Path, backup: &tempfile::NamedTempFile) -> Result<(), String> {
    let parent = path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new("."));
    let mut temporary = tempfile::NamedTempFile::new_in(parent).map_err(|error| {
        format!(
            "create temporary restore file for {}: {error}",
            path.display()
        )
    })?;
    let mut source = fs::File::open(backup.path())
        .map_err(|error| format!("open snapshot for {}: {error}", path.display()))?;
    std::io::copy(&mut source, temporary.as_file_mut())
        .and_then(|_| temporary.as_file().sync_all())
        .map_err(|error| {
            format!(
                "write temporary restore file for {}: {error}",
                path.display()
            )
        })?;
    temporary
        .persist(path)
        .map_err(|error| format!("atomically restore {}: {error}", path.display()))?;
    Ok(())
}
