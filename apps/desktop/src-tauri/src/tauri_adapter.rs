use tauri::{Builder, Runtime, State};

use crate::contract::{
    DesktopFacade, DesktopFailure, DesktopLogDay, DesktopLogPage, DesktopStartupSnapshot,
    DesktopUpdateResult,
};

pub fn with_desktop_facade<R: Runtime>(builder: Builder<R>, facade: DesktopFacade) -> Builder<R> {
    builder
        .manage(facade)
        .invoke_handler(tauri::generate_handler![
            desktop_startup_snapshot,
            desktop_startup_quit,
            desktop_open_link,
            desktop_log_list_days,
            desktop_log_read_page,
            desktop_log_open_folder,
            desktop_update_check,
            desktop_window_show_main,
            desktop_window_hide_main,
            desktop_window_open_logs,
        ])
}

#[tauri::command]
fn desktop_startup_snapshot(
    facade: State<'_, DesktopFacade>,
) -> Result<DesktopStartupSnapshot, DesktopFailure> {
    facade.port().startup_snapshot()
}

#[tauri::command]
fn desktop_startup_quit(facade: State<'_, DesktopFacade>) -> Result<(), DesktopFailure> {
    facade.port().startup_quit()
}

#[tauri::command]
fn desktop_open_link(facade: State<'_, DesktopFacade>, link: String) -> Result<(), DesktopFailure> {
    facade.port().open_link(&link)
}

#[tauri::command]
fn desktop_log_list_days(
    facade: State<'_, DesktopFacade>,
) -> Result<Vec<DesktopLogDay>, DesktopFailure> {
    facade.port().log_list_days()
}

#[tauri::command(rename_all = "camelCase")]
fn desktop_log_read_page(
    facade: State<'_, DesktopFacade>,
    day: String,
    level: String,
    query: String,
    offset: i64,
    limit: usize,
) -> Result<DesktopLogPage, DesktopFailure> {
    facade
        .port()
        .log_read_page(&day, &level, &query, offset, limit)
}

#[tauri::command]
fn desktop_log_open_folder(facade: State<'_, DesktopFacade>) -> Result<(), DesktopFailure> {
    facade.port().log_open_folder()
}

#[tauri::command]
fn desktop_update_check(
    facade: State<'_, DesktopFacade>,
) -> Result<DesktopUpdateResult, DesktopFailure> {
    facade.port().update_check()
}

#[tauri::command]
fn desktop_window_show_main(facade: State<'_, DesktopFacade>) -> Result<(), DesktopFailure> {
    facade.port().window_show_main()
}

#[tauri::command]
fn desktop_window_hide_main(facade: State<'_, DesktopFacade>) -> Result<(), DesktopFailure> {
    facade.port().window_hide_main()
}

#[tauri::command]
fn desktop_window_open_logs(facade: State<'_, DesktopFacade>) -> Result<(), DesktopFailure> {
    facade.port().window_open_logs()
}
