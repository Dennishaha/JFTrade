use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::fs::OpenOptions;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, OnceLock, RwLock};

use jftrade_engine::product::{
    ProductConfig, ProductError, ProductNotificationDelivery, ProductNotificationPort,
    ProductNotificationRequest,
};
use jftrade_engine::product_runtime::{
    DesktopMarketDataRuntimeConfig, DesktopPineRuntimeConfig, DesktopRetainedRuntimeConfig,
    ProductRuntimeConfig, ProductRuntimeError, ProductRuntimeHandle, start_product_runtime,
};
use tauri::menu::MenuBuilder;
use tauri::tray::TrayIconBuilder;
use tauri::{
    AppHandle, Emitter, Manager, RunEvent, Url, WebviewUrl, WebviewWindowBuilder, WindowEvent, Wry,
};
use tauri_plugin_notification::{NotificationExt, PermissionState};
use tauri_plugin_updater::{Update, UpdaterExt};
use thiserror::Error;
use time::format_description::well_known::Rfc3339;
use time::{Date, Month, OffsetDateTime};

use crate::contract::{
    DESKTOP_SECOND_INSTANCE_EVENT, DesktopFacade, DesktopFailure, DesktopLogDay, DesktopLogLine,
    DesktopLogPage, DesktopPort, DesktopRuntimeConfig, DesktopStartupSnapshot, DesktopUpdateResult,
};
use crate::links::{LinkTarget, classify_link};
use crate::profile::{
    DesktopChannel, DesktopPlatform, DesktopProfile, PlatformPaths, ProfileError,
};
use crate::resource_integrity::verify_release_resources;
use crate::tauri_adapter::with_desktop_facade;
use crate::window_state::WindowStateStore;

const DEFAULT_LOG_LIMIT: usize = 500;
const MAX_LOG_LIMIT: usize = 2_000;
const LATEST_LOG_OFFSET: i64 = -1;
const UPDATER_ENDPOINT_ENV: &str = "JFTRADE_TAURI_UPDATER_ENDPOINT";
const UPDATER_PUBLIC_KEY_ENV: &str = "JFTRADE_TAURI_UPDATER_PUBKEY";
include!("native_lifecycle.rs");
include!("native_notification_updater.rs");
include!("native_window_tray.rs");
include!("native_resource_integrity.rs");
include!("native_logs.rs");

#[cfg(test)]
include!("native_tests.rs");
