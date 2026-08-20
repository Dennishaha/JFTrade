use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use serde::{Deserialize, Serialize};
use tauri::WebviewWindow;
use tempfile::Builder;
use thiserror::Error;

const STATE_VERSION: u8 = 1;
const MIN_WIDTH: u32 = 1_024;
const MIN_HEIGHT: u32 = 700;
const MAX_DIMENSION: u32 = 32_768;
const MIN_COORDINATE: i32 = -100_000;
const MAX_COORDINATE: i32 = 100_000;
const VISIBLE_INTERSECTION: i32 = 64;

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DesktopWindowState {
    version: u8,
    x: i32,
    y: i32,
    width: u32,
    height: u32,
    maximised: bool,
}

impl DesktopWindowState {
    fn valid(self) -> bool {
        self.version == STATE_VERSION
            && (MIN_WIDTH..=MAX_DIMENSION).contains(&self.width)
            && (MIN_HEIGHT..=MAX_DIMENSION).contains(&self.height)
            && (MIN_COORDINATE..=MAX_COORDINATE).contains(&self.x)
            && (MIN_COORDINATE..=MAX_COORDINATE).contains(&self.y)
    }

    fn rect(self) -> DesktopRect {
        DesktopRect {
            x: self.x,
            y: self.y,
            width: i32::try_from(self.width).unwrap_or(i32::MAX),
            height: i32::try_from(self.height).unwrap_or(i32::MAX),
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct DesktopRect {
    x: i32,
    y: i32,
    width: i32,
    height: i32,
}

impl DesktopRect {
    fn intersects(self, other: Self) -> bool {
        let width = self
            .x
            .saturating_add(self.width)
            .min(other.x.saturating_add(other.width))
            .saturating_sub(self.x.max(other.x));
        let height = self
            .y
            .saturating_add(self.height)
            .min(other.y.saturating_add(other.height))
            .saturating_sub(self.y.max(other.y));
        width >= VISIBLE_INTERSECTION && height >= VISIBLE_INTERSECTION
    }
}

pub(crate) struct WindowStateStore {
    path: Option<PathBuf>,
    current: Mutex<Option<DesktopWindowState>>,
}

impl WindowStateStore {
    pub(crate) fn load(path: Option<&str>) -> Self {
        let path = path
            .filter(|value| !value.trim().is_empty())
            .map(PathBuf::from);
        let current = path
            .as_deref()
            .and_then(|state_path| match load_state(state_path) {
                Ok(value) => value,
                Err(error) => {
                    eprintln!("JFTrade desktop window state ignored: {error}");
                    None
                }
            });
        Self {
            path,
            current: Mutex::new(current),
        }
    }

    pub(crate) fn apply(&self, window: &WebviewWindow) -> Result<(), WindowStateError> {
        let state = self.current.lock().map_err(|_| WindowStateError::Lock)?;
        let Some(mut state) = *state else {
            return Ok(());
        };
        if !state.valid() {
            return Ok(());
        }
        let monitors = window.available_monitors()?;
        let visible = monitors.iter().any(|monitor| {
            let work = monitor.work_area();
            state.rect().intersects(DesktopRect {
                x: work.position.x,
                y: work.position.y,
                width: i32::try_from(work.size.width).unwrap_or(i32::MAX),
                height: i32::try_from(work.size.height).unwrap_or(i32::MAX),
            })
        });
        if !visible && let Some(primary) = window.primary_monitor()? {
            let work = primary.work_area();
            state.x = work.position.x.saturating_add(
                (i32::try_from(work.size.width).unwrap_or(i32::MAX)
                    - i32::try_from(state.width).unwrap_or(i32::MAX))
                .max(0)
                    / 2,
            );
            state.y = work.position.y.saturating_add(
                (i32::try_from(work.size.height).unwrap_or(i32::MAX)
                    - i32::try_from(state.height).unwrap_or(i32::MAX))
                .max(0)
                    / 2,
            );
        }
        window.set_size(tauri::PhysicalSize::new(state.width, state.height))?;
        window.set_position(tauri::PhysicalPosition::new(state.x, state.y))?;
        if state.maximised {
            window.maximize()?;
        }
        Ok(())
    }

    pub(crate) fn capture_and_save(
        &self,
        window: Option<&WebviewWindow>,
    ) -> Result<(), WindowStateError> {
        let Some(path) = &self.path else {
            return Ok(());
        };
        let Some(window) = window else {
            return Ok(());
        };
        let maximised = window.is_maximized()?;
        let mut current = self.current.lock().map_err(|_| WindowStateError::Lock)?;
        let next = if maximised {
            current.map(|mut value| {
                value.maximised = true;
                value
            })
        } else {
            let position = window.outer_position()?;
            let size = window.outer_size()?;
            Some(DesktopWindowState {
                version: STATE_VERSION,
                x: position.x,
                y: position.y,
                width: size.width,
                height: size.height,
                maximised: false,
            })
        };
        if let Some(next) = next.filter(|value| value.valid()) {
            save_state(path, next)?;
            *current = Some(next);
        }
        Ok(())
    }
}

fn load_state(path: &Path) -> Result<Option<DesktopWindowState>, WindowStateError> {
    let contents = match fs::read(path) {
        Ok(contents) => contents,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(source) => {
            return Err(WindowStateError::Read {
                path: path.into(),
                source,
            });
        }
    };
    let state: DesktopWindowState = serde_json::from_slice(&contents)?;
    if !state.valid() {
        return Err(WindowStateError::Invalid);
    }
    Ok(Some(state))
}

fn save_state(path: &Path, state: DesktopWindowState) -> Result<(), WindowStateError> {
    let directory = path
        .parent()
        .filter(|value| !value.as_os_str().is_empty())
        .unwrap_or(Path::new("."));
    fs::create_dir_all(directory).map_err(|source| WindowStateError::Write {
        path: directory.into(),
        source,
    })?;
    let encoded = serde_json::to_vec_pretty(&state)?;
    let mut temporary = Builder::new()
        .prefix(".desktop-state-")
        .suffix(".tmp")
        .tempfile_in(directory)
        .map_err(|source| WindowStateError::Write {
            path: path.into(),
            source,
        })?;
    secure_file(temporary.as_file())?;
    temporary
        .write_all(&encoded)
        .and_then(|()| temporary.write_all(b"\n"))
        .and_then(|()| temporary.as_file().sync_all())
        .map_err(|source| WindowStateError::Write {
            path: path.into(),
            source,
        })?;
    temporary
        .persist(path)
        .map_err(|error| WindowStateError::Write {
            path: path.into(),
            source: error.error,
        })?;
    Ok(())
}

#[cfg(unix)]
fn secure_file(file: &File) -> Result<(), WindowStateError> {
    use std::os::unix::fs::PermissionsExt;
    file.set_permissions(fs::Permissions::from_mode(0o600))
        .map_err(|source| WindowStateError::Write {
            path: PathBuf::from("desktop-state temporary file"),
            source,
        })
}

#[cfg(not(unix))]
fn secure_file(_file: &File) -> Result<(), WindowStateError> {
    Ok(())
}

#[derive(Debug, Error)]
pub(crate) enum WindowStateError {
    #[error("read desktop window state {path}")]
    Read {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("decode desktop window state")]
    Decode(#[from] serde_json::Error),
    #[error("desktop window state is invalid")]
    Invalid,
    #[error("write desktop window state {path}")]
    Write {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("desktop window state lock is unavailable")]
    Lock,
    #[error(transparent)]
    Tauri(#[from] tauri::Error),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn state_round_trip_is_atomic_and_rejects_invalid_bounds() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("desktop-state.json");
        let state = DesktopWindowState {
            version: STATE_VERSION,
            x: -1_200,
            y: 50,
            width: 1_400,
            height: 900,
            maximised: true,
        };
        save_state(&path, state).expect("save state");
        assert_eq!(load_state(&path).expect("load state"), Some(state));

        fs::write(
            &path,
            br#"{"version":1,"x":0,"y":0,"width":100,"height":100,"maximised":false}"#,
        )
        .expect("write invalid state");
        assert!(matches!(load_state(&path), Err(WindowStateError::Invalid)));
    }

    #[test]
    fn visibility_requires_a_real_intersection() {
        let window = DesktopRect {
            x: -1_200,
            y: 50,
            width: 1_400,
            height: 900,
        };
        let primary = DesktopRect {
            x: 0,
            y: 0,
            width: 1_920,
            height: 1_080,
        };
        assert!(window.intersects(primary));
        assert!(
            !DesktopRect {
                x: -2_000,
                ..window
            }
            .intersects(primary)
        );
    }
}
