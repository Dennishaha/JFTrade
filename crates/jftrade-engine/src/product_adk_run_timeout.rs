//! Assistant run-timeout lookup shared by every ADK execution path.
//!
//! Go's `ApplicationAdapter.runtimeLimits` reads the persisted
//! `RunTimeoutMs` from the settings port on every `startRun` and
//! `ResumeGoalRun`, and `Runtime.runtimeLimits` falls back to
//! `assistantmodel.DefaultRunTimeout` whenever that snapshot is absent or not
//! positive.  Rust keeps the single implementation here so the chat entry, the
//! goal-resume mutation and any later expiry reconciliation agree on the same
//! value.

use std::path::Path;

use jftrade_settings::{AssistantRuntimeSettingsStorePort, normalize_assistant_runtime_settings};
use jftrade_store_settings_file::SettingsFileStore;

/// Go `assistantmodel.DefaultRunTimeout`.
pub(crate) use jftrade_assistant::DEFAULT_RUN_TIMEOUT_MS;

/// The frozen `run.MaxDurationMs` for a new or resumed goal run.
///
/// An unreadable or absent settings file behaves exactly like Go's
/// `RuntimeLimits{}` snapshot: the default timeout wins instead of failing the
/// chat request.
pub(crate) fn assistant_run_timeout_ms(settings_path: &Path) -> i64 {
    let Ok(store) = SettingsFileStore::open_read_only(settings_path) else {
        return DEFAULT_RUN_TIMEOUT_MS;
    };
    let settings = store
        .load_assistant_runtime()
        .ok()
        .flatten()
        .map(|settings| normalize_assistant_runtime_settings(&settings))
        .unwrap_or_default();
    let timeout_ms = i64::from(settings.run_timeout_ms);
    if timeout_ms > 0 {
        timeout_ms
    } else {
        DEFAULT_RUN_TIMEOUT_MS
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::path::PathBuf;

    fn settings_file(body: &str) -> (tempfile::TempDir, PathBuf) {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("settings.json");
        fs::write(&path, body).expect("write assistant runtime settings");
        (directory, path)
    }

    /// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:294
    /// `TestRuntimeFacadeBoundaryBranches`: a nil runtime reports
    /// `RuntimeLimits{}`, so `runtimeLimits()` answers
    /// `assistantmodel.DefaultRunTimeout` for an absent, unreadable or
    /// non-positive snapshot, while a configured provider value wins.
    #[test]
    // Parity: go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:71 TestApplicationAdapterUsesConfiguredRuntimeAndSettings
    fn assistant_run_timeout_falls_back_to_the_reference_default_window() {
        assert_eq!(
            assistant_run_timeout_ms(Path::new("missing-settings.json")),
            DEFAULT_RUN_TIMEOUT_MS
        );
        assert_eq!(DEFAULT_RUN_TIMEOUT_MS, 1_800_000);

        for body in [
            "{}",
            r#"{"adk":{}}"#,
            r#"{"adk":{"runTimeoutMs":0}}"#,
            r#"{"adk":{"runTimeoutMs":-1}}"#,
            "{",
        ] {
            let (_directory, path) = settings_file(body);
            assert_eq!(
                assistant_run_timeout_ms(&path),
                DEFAULT_RUN_TIMEOUT_MS,
                "settings {body} must fall back to the reference default"
            );
        }

        let (_directory, configured) = settings_file(r#"{"adk":{"runTimeoutMs":660000}}"#);
        assert_eq!(assistant_run_timeout_ms(&configured), 660_000);
    }
}
