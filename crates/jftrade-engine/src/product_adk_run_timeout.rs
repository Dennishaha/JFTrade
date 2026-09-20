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
pub(crate) const DEFAULT_RUN_TIMEOUT_MS: i64 = 1_800_000;

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
