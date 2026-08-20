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

pub fn run() -> Result<(), NativeError> {
    let bootstrap = NativeBootstrap::resolve()?;
    let updater = NativeUpdaterConfig::from_environment(bootstrap.profile.update_checks_enabled)?;
    let updater_plugin = updater.clone();
    let log_path = desktop_log_path(Path::new(&bootstrap.profile.settings_path));
    append_native_log(&log_path, "INFO", "Rust native desktop bootstrap started");
    let token = random_token()?;
    let startup = DesktopStartupSnapshot {
        state: "starting".to_owned(),
        phase: "rust-native-bootstrap".to_owned(),
        message: "Rust native product runtime is starting".to_owned(),
        started_at: now_rfc3339(),
    };
    let product = Arc::new(Mutex::new(None));
    let quit_requested = Arc::new(AtomicBool::new(false));
    let install_product = Arc::clone(&product);
    let install_quit_requested = Arc::clone(&quit_requested);
    let port = Arc::new(NativeDesktopPort::new(
        startup,
        Path::new(&bootstrap.profile.settings_path),
        bootstrap.profile.application_name,
        updater,
        Arc::new(move || {
            install_quit_requested.store(true, Ordering::SeqCst);
            stop_product(&install_product);
        }),
    ));
    let window_state = Arc::new(WindowStateStore::load(
        bootstrap.profile.window_state_path.as_deref(),
    ));

    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(
            |app, _arguments, _cwd| {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
                let _ = app.emit(DESKTOP_SECOND_INSTANCE_EVENT, ());
            },
        ))
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_notification::init());
    let builder = match updater_plugin {
        NativeUpdaterConfig::Ready { public_key, .. } => builder.plugin(
            tauri_plugin_updater::Builder::new()
                .pubkey(public_key)
                .build(),
        ),
        NativeUpdaterConfig::Disabled | NativeUpdaterConfig::Unconfigured => builder,
    };
    let setup_port = Arc::clone(&port);
    let setup_product = Arc::clone(&product);
    let setup_log_path = log_path.clone();
    let setup_quit_requested = Arc::clone(&quit_requested);
    let setup_window_state = Arc::clone(&window_state);
    let builder = with_desktop_facade(builder, DesktopFacade::new(port)).setup(move |app| {
        setup_port.attach(app.handle().clone());
        let main_window = app
            .get_webview_window("main")
            .ok_or(NativeError::MissingMainWindow)?;
        if let Err(error) = setup_window_state.apply(&main_window) {
            append_native_log(
                &setup_log_path,
                "WARN",
                &format!("Rust desktop window state ignored: {error}"),
            );
        }
        let close_window = main_window.clone();
        let close_quit_requested = Arc::clone(&setup_quit_requested);
        let close_window_state = Arc::clone(&setup_window_state);
        main_window.on_window_event(move |event| {
            if let WindowEvent::CloseRequested { api, .. } = event
                && !close_quit_requested.load(Ordering::SeqCst)
            {
                api.prevent_close();
                let _ = close_window_state.capture_and_save(Some(&close_window));
                let _ = close_window.hide();
            }
        });
        configure_system_tray(
            app.handle(),
            bootstrap.profile.application_name,
            Arc::clone(&setup_quit_requested),
        )?;
        let resource_root = app
            .path()
            .resource_dir()
            .map_err(NativeError::ResourceDirectory)?;
        let runtime = start_native_product(
            &bootstrap,
            &resource_root,
            &token,
            Some(setup_log_path.clone()),
            Arc::new(TauriNotificationPort {
                app: app.handle().clone(),
            }),
        )
        .inspect_err(|error| {
            append_native_log(
                &setup_log_path,
                "ERROR",
                &format!("Rust retained runtime failed to start: {error}"),
            );
        })?;
        let runtime_config = DesktopRuntimeConfig {
            api_base_url: format!("http://{}", runtime.startup_record().address),
            auth_required: true,
            desktop_mode: true,
            desktop_api_token: token.clone(),
        };
        setup_port.mark_ready(runtime_config)?;
        *setup_product
            .lock()
            .map_err(|_| NativeError::RuntimeState)? = Some(runtime);
        append_native_log(
            &setup_log_path,
            "INFO",
            "Rust API, PineTS worker, and market-data helper are ready",
        );
        setup_port.show_main()?;
        if setup_port.automatic_update_check_enabled() {
            let update_port = Arc::clone(&setup_port);
            let update_app = app.handle().clone();
            let update_log_path = setup_log_path.clone();
            tauri::async_runtime::spawn(async move {
                match update_port.update_check().await {
                    Ok(result) if result.available => {
                        let _ = update_app
                            .emit(crate::contract::DESKTOP_UPDATE_AVAILABLE_EVENT, result);
                    }
                    Ok(_) => append_native_log(
                        &update_log_path,
                        "INFO",
                        "Tauri updater found no newer signed release",
                    ),
                    Err(error) => append_native_log(
                        &update_log_path,
                        "WARN",
                        &format!("Tauri updater check failed: {error}"),
                    ),
                }
            });
        }
        Ok(())
    });
    let application = builder.build(tauri::generate_context!())?;
    let signal_app = application.handle().clone();
    let signal_quit_requested = Arc::clone(&quit_requested);
    tauri::async_runtime::spawn(async move {
        if tokio::signal::ctrl_c().await.is_ok() {
            signal_quit_requested.store(true, Ordering::SeqCst);
            signal_app.exit(0);
        }
    });
    let shutdown_product = Arc::clone(&product);
    let shutdown_log_path = log_path.clone();
    let shutdown_window_state = Arc::clone(&window_state);
    application.run(move |app, event| {
        if matches!(event, RunEvent::Exit) {
            append_native_log(
                &shutdown_log_path,
                "INFO",
                "Rust native desktop shutdown started",
            );
            if let Err(error) =
                shutdown_window_state.capture_and_save(app.get_webview_window("main").as_ref())
            {
                append_native_log(
                    &shutdown_log_path,
                    "WARN",
                    &format!("Rust desktop window state save failed: {error}"),
                );
            }
            stop_product(&shutdown_product);
            append_native_log(&shutdown_log_path, "INFO", "Rust retained runtime stopped");
        }
    });
    stop_product(&product);
    Ok(())
}

struct NativeBootstrap {
    profile: DesktopProfile,
    repository_root: Option<PathBuf>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
enum NativeUpdaterConfig {
    Disabled,
    Unconfigured,
    Ready { endpoint: Url, public_key: String },
}

impl NativeUpdaterConfig {
    fn from_environment(enabled: bool) -> Result<Self, NativeError> {
        Self::from_values(
            enabled,
            updater_environment_value(UPDATER_ENDPOINT_ENV)?
                .or_else(|| option_env!("JFTRADE_TAURI_UPDATER_ENDPOINT").map(str::to_owned)),
            updater_environment_value(UPDATER_PUBLIC_KEY_ENV)?
                .or_else(|| option_env!("JFTRADE_TAURI_UPDATER_PUBKEY").map(str::to_owned)),
        )
    }

    fn from_values(
        enabled: bool,
        endpoint: Option<String>,
        public_key: Option<String>,
    ) -> Result<Self, NativeError> {
        if !enabled {
            return Ok(Self::Disabled);
        }
        let endpoint = endpoint.unwrap_or_default();
        let public_key = public_key.unwrap_or_default();
        let endpoint = endpoint.trim();
        let public_key = public_key.trim();
        if endpoint.is_empty() && public_key.is_empty() {
            return Ok(Self::Unconfigured);
        }
        if endpoint.is_empty() || public_key.is_empty() {
            return Err(NativeError::UpdaterConfiguration(
                "updater endpoint and signing public key must be configured together".to_owned(),
            ));
        }
        let endpoint = endpoint.parse::<Url>().map_err(|error| {
            NativeError::UpdaterConfiguration(format!("invalid updater endpoint: {error}"))
        })?;
        if endpoint.scheme() != "https"
            || endpoint.host_str().is_none()
            || !endpoint.username().is_empty()
            || endpoint.password().is_some()
        {
            return Err(NativeError::UpdaterConfiguration(
                "updater endpoint must be an HTTPS URL without credentials".to_owned(),
            ));
        }
        Ok(Self::Ready {
            endpoint,
            public_key: public_key.to_owned(),
        })
    }
}

fn updater_environment_value(name: &'static str) -> Result<Option<String>, NativeError> {
    match env::var(name) {
        Ok(value) => Ok(Some(value)),
        Err(env::VarError::NotPresent) => Ok(None),
        Err(env::VarError::NotUnicode(_)) => Err(NativeError::UpdaterConfiguration(format!(
            "{name} must be valid UTF-8"
        ))),
    }
}

impl NativeBootstrap {
    fn resolve() -> Result<Self, NativeError> {
        let channel = if cfg!(debug_assertions) {
            DesktopChannel::Dev
        } else {
            DesktopChannel::Release
        };
        let mut profile = DesktopProfile::resolve(channel, &platform_paths()?)?;
        let repository_root = if channel == DesktopChannel::Dev {
            let root = development_repository_root()?;
            absolutize_development_profile(&mut profile, &root);
            Some(root)
        } else {
            None
        };
        Ok(Self {
            profile,
            repository_root,
        })
    }
}

struct TauriNotificationPort {
    app: AppHandle<Wry>,
}

impl std::fmt::Debug for TauriNotificationPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("TauriNotificationPort")
    }
}

impl ProductNotificationPort for TauriNotificationPort {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery {
        let notification = self.app.notification();
        let permission = notification
            .permission_state()
            .and_then(|state| match state {
                PermissionState::Granted | PermissionState::Denied => Ok(state),
                PermissionState::Prompt | PermissionState::PromptWithRationale => {
                    notification.request_permission()
                }
            });
        match permission {
            Ok(PermissionState::Granted) => {
                let mut builder = notification
                    .builder()
                    .title(request.title)
                    .body(request.body);
                if !request.sound_enabled {
                    builder = builder.silent();
                }
                match builder.show() {
                    Ok(()) => ProductNotificationDelivery {
                        delivered: true,
                        status: "delivered".to_owned(),
                        message: "sent to operating system notification center".to_owned(),
                    },
                    Err(error) => ProductNotificationDelivery {
                        delivered: false,
                        status: "failed".to_owned(),
                        message: error.to_string(),
                    },
                }
            }
            Ok(_) => ProductNotificationDelivery {
                delivered: false,
                status: "unauthorized".to_owned(),
                message: "operating system notification permission is not authorized".to_owned(),
            },
            Err(error) => ProductNotificationDelivery {
                delivered: false,
                status: "failed".to_owned(),
                message: error.to_string(),
            },
        }
    }
}

struct NativeDesktopPort {
    app: OnceLock<AppHandle<Wry>>,
    runtime_config: RwLock<Option<DesktopRuntimeConfig>>,
    startup: RwLock<DesktopStartupSnapshot>,
    log_dir: PathBuf,
    application_name: &'static str,
    updater: NativeUpdaterConfig,
    pending_update: Arc<Mutex<Option<Update>>>,
    before_update_install: Arc<dyn Fn() + Send + Sync>,
}

impl NativeDesktopPort {
    fn new(
        startup: DesktopStartupSnapshot,
        settings_path: &Path,
        application_name: &'static str,
        updater: NativeUpdaterConfig,
        before_update_install: Arc<dyn Fn() + Send + Sync>,
    ) -> Self {
        let log_dir = settings_path
            .parent()
            .filter(|path| !path.as_os_str().is_empty())
            .unwrap_or(Path::new("."))
            .join("logs");
        Self {
            app: OnceLock::new(),
            runtime_config: RwLock::new(None),
            startup: RwLock::new(startup),
            log_dir,
            application_name,
            updater,
            pending_update: Arc::new(Mutex::new(None)),
            before_update_install,
        }
    }

    fn attach(&self, app: AppHandle<Wry>) {
        let _ = self.app.set(app);
    }

    fn app(&self) -> Result<&AppHandle<Wry>, DesktopFailure> {
        self.app.get().ok_or_else(|| {
            DesktopFailure::new("DESKTOP_NOT_READY", "Tauri application is not ready")
        })
    }

    fn mark_ready(&self, runtime_config: DesktopRuntimeConfig) -> Result<(), NativeError> {
        *self
            .runtime_config
            .write()
            .map_err(|_| NativeError::RuntimeState)? = Some(runtime_config);
        *self.startup.write().map_err(|_| NativeError::RuntimeState)? = DesktopStartupSnapshot {
            state: "ready".to_owned(),
            phase: "rust-native-retained-runtime".to_owned(),
            message: "Rust API, PineTS worker, and market-data helper are ready; unported route groups remain unavailable".to_owned(),
            started_at: now_rfc3339(),
        };
        Ok(())
    }

    fn show_main(&self) -> Result<(), NativeError> {
        let window = self.app()?.get_webview_window("main").ok_or_else(|| {
            DesktopFailure::new("DESKTOP_WINDOW_MISSING", "main window is missing")
        })?;
        window
            .show()
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))?;
        window
            .set_focus()
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))?;
        Ok(())
    }

    fn automatic_update_check_enabled(&self) -> bool {
        matches!(self.updater, NativeUpdaterConfig::Ready { .. })
    }
}

impl DesktopPort for NativeDesktopPort {
    fn runtime_config(&self) -> Result<DesktopRuntimeConfig, DesktopFailure> {
        self.runtime_config
            .read()
            .map_err(|_| {
                DesktopFailure::new("DESKTOP_STATE_FAILED", "runtime config is unavailable")
            })?
            .clone()
            .ok_or_else(|| {
                DesktopFailure::new("DESKTOP_NOT_READY", "product runtime is still starting")
            })
    }

    fn startup_snapshot(&self) -> Result<DesktopStartupSnapshot, DesktopFailure> {
        self.startup.read().map(|value| value.clone()).map_err(|_| {
            DesktopFailure::new("DESKTOP_STATE_FAILED", "startup state is unavailable")
        })
    }

    fn startup_quit(&self) -> Result<(), DesktopFailure> {
        self.app()?.exit(0);
        Ok(())
    }

    fn open_link(&self, link: &str) -> Result<(), DesktopFailure> {
        match classify_link(link).map_err(desktop_error("DESKTOP_LINK_INVALID"))? {
            LinkTarget::External(url) => tauri_plugin_opener::open_url(url, None::<&str>)
                .map_err(desktop_error("DESKTOP_LINK_OPEN_FAILED")),
            LinkTarget::Docs(path) => {
                open_route_window(self.app()?, "docs", "JFTrade 文档", &path, 1_120.0, 760.0)
            }
        }
    }

    fn log_list_days(&self) -> Result<Vec<DesktopLogDay>, DesktopFailure> {
        let entries = match fs::read_dir(&self.log_dir) {
            Ok(entries) => entries,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(error) => return Err(desktop_error("DESKTOP_LOG_READ_FAILED")(error)),
        };
        let mut days = Vec::new();
        for entry in entries {
            let entry = entry.map_err(desktop_error("DESKTOP_LOG_READ_FAILED"))?;
            if !entry
                .file_type()
                .map_err(desktop_error("DESKTOP_LOG_READ_FAILED"))?
                .is_file()
            {
                continue;
            }
            if let Some(day) = log_day(entry.file_name().to_string_lossy().as_ref()) {
                days.push(DesktopLogDay { day });
            }
        }
        days.sort_by(|left, right| right.day.cmp(&left.day));
        Ok(days)
    }

    fn log_read_page(
        &self,
        day: &str,
        level: &str,
        query: &str,
        offset: i64,
        limit: usize,
    ) -> Result<DesktopLogPage, DesktopFailure> {
        let day = normalized_day(day)?;
        let path = self.log_dir.join(format!("desktop-{day}.log"));
        let contents = match fs::read(path) {
            Ok(contents) => String::from_utf8_lossy(&contents).into_owned(),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => String::new(),
            Err(error) => return Err(desktop_error("DESKTOP_LOG_READ_FAILED")(error)),
        };
        let level = level.trim().to_ascii_uppercase();
        let query = query.trim().to_ascii_lowercase();
        let filtered = contents
            .lines()
            .map(str::trim_end)
            .filter(|line| !line.trim().is_empty())
            .map(|text| DesktopLogLine {
                level: parse_log_level(text).to_owned(),
                text: text.to_owned(),
            })
            .filter(|line| level.is_empty() || level == "ALL" || line.level == level)
            .filter(|line| query.is_empty() || line.text.to_ascii_lowercase().contains(&query))
            .collect::<Vec<_>>();
        let limit = if limit == 0 {
            DEFAULT_LOG_LIMIT
        } else {
            limit.min(MAX_LOG_LIMIT)
        };
        let total = filtered.len();
        let offset = if offset == LATEST_LOG_OFFSET && total > 0 {
            ((total - 1) / limit) * limit
        } else {
            usize::try_from(offset.max(0)).unwrap_or(usize::MAX)
        };
        let items = filtered.into_iter().skip(offset).take(limit).collect();
        Ok(DesktopLogPage {
            day,
            items,
            offset: i64::try_from(offset).unwrap_or(i64::MAX),
            limit,
            total,
            log_dir: self.log_dir.to_string_lossy().into_owned(),
        })
    }

    fn log_open_folder(&self) -> Result<(), DesktopFailure> {
        fs::create_dir_all(&self.log_dir).map_err(desktop_error("DESKTOP_LOG_OPEN_FAILED"))?;
        tauri_plugin_opener::open_path(&self.log_dir, None::<&str>)
            .map_err(desktop_error("DESKTOP_LOG_OPEN_FAILED"))
    }

    fn update_check(&self) -> crate::contract::DesktopFuture<DesktopUpdateResult> {
        let app = match self.app() {
            Ok(app) => app.clone(),
            Err(error) => return Box::pin(async move { Err(error) }),
        };
        let updater = self.updater.clone();
        let pending_update = Arc::clone(&self.pending_update);
        Box::pin(async move {
            let current_version = app.package_info().version.to_string();
            let NativeUpdaterConfig::Ready {
                endpoint,
                public_key,
            } = updater
            else {
                return match updater {
                    NativeUpdaterConfig::Disabled => Ok(no_update(current_version)),
                    NativeUpdaterConfig::Unconfigured => Err(DesktopFailure::new(
                        "DESKTOP_UPDATE_NOT_CONFIGURED",
                        "release updater requires an HTTPS endpoint and signing public key",
                    )),
                    NativeUpdaterConfig::Ready { .. } => unreachable!(),
                };
            };
            let updater = app
                .updater_builder()
                .pubkey(public_key)
                .endpoints(vec![endpoint])
                .and_then(|builder| builder.build())
                .map_err(desktop_error("DESKTOP_UPDATE_CONFIGURATION_INVALID"))?;
            let update = updater
                .check()
                .await
                .map_err(desktop_error("DESKTOP_UPDATE_CHECK_FAILED"))?;
            let Some(update) = update else {
                *pending_update.lock().map_err(|_| {
                    DesktopFailure::new(
                        "DESKTOP_UPDATE_STATE_FAILED",
                        "pending updater state is unavailable",
                    )
                })? = None;
                return Ok(no_update(current_version));
            };
            let result = update_result(&update);
            *pending_update.lock().map_err(|_| {
                DesktopFailure::new(
                    "DESKTOP_UPDATE_STATE_FAILED",
                    "pending updater state is unavailable",
                )
            })? = Some(update);
            Ok(result)
        })
    }

    fn update_install(&self) -> crate::contract::DesktopFuture<()> {
        let pending_update = Arc::clone(&self.pending_update);
        let before_install = Arc::clone(&self.before_update_install);
        let app = match self.app() {
            Ok(app) => app.clone(),
            Err(error) => return Box::pin(async move { Err(error) }),
        };
        Box::pin(async move {
            let update = pending_update
                .lock()
                .map_err(|_| {
                    DesktopFailure::new(
                        "DESKTOP_UPDATE_STATE_FAILED",
                        "pending updater state is unavailable",
                    )
                })?
                .take()
                .ok_or_else(|| {
                    DesktopFailure::new(
                        "DESKTOP_UPDATE_NOT_READY",
                        "check for a signed update before installing",
                    )
                })?;
            let bytes = match update.download(|_, _| {}, || {}).await {
                Ok(bytes) => bytes,
                Err(error) => {
                    *pending_update.lock().map_err(|_| {
                        DesktopFailure::new(
                            "DESKTOP_UPDATE_STATE_FAILED",
                            "pending updater state is unavailable",
                        )
                    })? = Some(update);
                    return Err(desktop_error("DESKTOP_UPDATE_DOWNLOAD_FAILED")(error));
                }
            };
            before_install();
            if let Err(error) = update.install(bytes) {
                app.exit(1);
                return Err(desktop_error("DESKTOP_UPDATE_INSTALL_FAILED")(error));
            }
            app.exit(0);
            Ok(())
        })
    }

    fn window_show_main(&self) -> Result<(), DesktopFailure> {
        let window = self.app()?.get_webview_window("main").ok_or_else(|| {
            DesktopFailure::new("DESKTOP_WINDOW_MISSING", "main window is missing")
        })?;
        window
            .show()
            .and_then(|()| window.set_focus())
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))
    }

    fn window_hide_main(&self) -> Result<(), DesktopFailure> {
        self.app()?
            .get_webview_window("main")
            .ok_or_else(|| DesktopFailure::new("DESKTOP_WINDOW_MISSING", "main window is missing"))?
            .hide()
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))
    }

    fn window_open_logs(&self) -> Result<(), DesktopFailure> {
        open_route_window(
            self.app()?,
            "desktop-logs",
            &format!("{} 日志", self.application_name),
            "/desktop-logs",
            1_040.0,
            720.0,
        )
    }
}

fn no_update(current_version: String) -> DesktopUpdateResult {
    DesktopUpdateResult {
        current_version,
        available: false,
        latest_version: String::new(),
        release_url: String::new(),
        published_at: String::new(),
        notes: String::new(),
    }
}

fn update_result(update: &Update) -> DesktopUpdateResult {
    let release_url = ["releaseUrl", "release_url", "html_url"]
        .into_iter()
        .find_map(|key| update.raw_json.get(key).and_then(serde_json::Value::as_str))
        .filter(|value| value.starts_with("https://"))
        .unwrap_or_default()
        .to_owned();
    DesktopUpdateResult {
        current_version: update.current_version.clone(),
        available: true,
        latest_version: update.version.clone(),
        release_url,
        published_at: update
            .date
            .and_then(|value| value.format(&Rfc3339).ok())
            .unwrap_or_default(),
        notes: update.body.clone().unwrap_or_default(),
    }
}

fn open_route_window(
    app: &AppHandle<Wry>,
    label: &str,
    title: &str,
    route: &str,
    width: f64,
    height: f64,
) -> Result<(), DesktopFailure> {
    if let Some(window) = app.get_webview_window(label) {
        window
            .show()
            .and_then(|()| window.set_focus())
            .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))?;
        return Ok(());
    }
    let url = WebviewUrl::App(PathBuf::from(route.trim_start_matches('/')));
    WebviewWindowBuilder::new(app, label, url)
        .title(title)
        .inner_size(width, height)
        .min_inner_size(760.0, 520.0)
        .center()
        .build()
        .map(|_| ())
        .map_err(desktop_error("DESKTOP_WINDOW_FAILED"))
}

fn configure_system_tray(
    app: &AppHandle<Wry>,
    application_name: &str,
    quit_requested: Arc<AtomicBool>,
) -> Result<(), NativeError> {
    const OPEN: &str = "desktop-tray-open";
    const SETTINGS: &str = "desktop-tray-settings";
    const DOCS: &str = "desktop-tray-docs";
    const LOGS: &str = "desktop-tray-logs";
    const QUIT: &str = "desktop-tray-quit";

    let menu = MenuBuilder::new(app)
        .text(OPEN, format!("打开 {application_name}"))
        .separator()
        .text(SETTINGS, "设置")
        .text(DOCS, "文档")
        .text(LOGS, "查看日志")
        .separator()
        .text(QUIT, "退出")
        .build()?;
    let icon = app
        .default_window_icon()
        .cloned()
        .ok_or(NativeError::MissingTrayIcon)?;
    TrayIconBuilder::with_id("jftrade-main")
        .menu(&menu)
        .icon(icon)
        .icon_as_template(cfg!(target_os = "macos"))
        .tooltip(application_name)
        .show_menu_on_left_click(true)
        .on_menu_event(move |app, event| match event.id().as_ref() {
            OPEN => show_and_focus_main(app),
            SETTINGS => {
                show_and_focus_main(app);
                let _ = app.emit(crate::contract::DESKTOP_MENU_SETTINGS_EVENT, ());
            }
            DOCS => {
                let _ = open_route_window(app, "docs", "JFTrade 文档", "/docs/", 1_120.0, 760.0);
            }
            LOGS => {
                let _ = open_route_window(
                    app,
                    "desktop-logs",
                    "JFTrade 日志",
                    "/desktop-logs",
                    1_040.0,
                    720.0,
                );
            }
            QUIT => {
                quit_requested.store(true, Ordering::SeqCst);
                app.exit(0);
            }
            _ => {}
        })
        .build(app)?;
    Ok(())
}

fn show_and_focus_main(app: &AppHandle<Wry>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
    }
}

fn platform_paths() -> Result<PlatformPaths, NativeError> {
    let home_dir = env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
        .filter(|path| !path.as_os_str().is_empty())
        .ok_or(NativeError::MissingHome)?;
    let platform = if cfg!(target_os = "macos") {
        DesktopPlatform::Darwin
    } else if cfg!(target_os = "windows") {
        DesktopPlatform::Windows
    } else {
        DesktopPlatform::Linux
    };
    Ok(PlatformPaths {
        platform,
        home_dir: home_dir.to_string_lossy().into_owned(),
        config_dir: env::var("APPDATA").unwrap_or_default(),
        local_app_data: env::var("LOCALAPPDATA").unwrap_or_default(),
        xdg_data_home: env::var("XDG_DATA_HOME").unwrap_or_default(),
    })
}

fn start_native_product(
    bootstrap: &NativeBootstrap,
    resource_root: &Path,
    token: &str,
    log_path: Option<PathBuf>,
    notification_port: Arc<dyn ProductNotificationPort>,
) -> Result<ProductRuntimeHandle, NativeError> {
    if bootstrap.repository_root.is_none() {
        verify_release_resources(resource_root)
            .map_err(|error| NativeError::ResourceIntegrity(error.to_string()))?;
    }
    let bind_address = bootstrap
        .profile
        .api_bind
        .parse()
        .map_err(NativeError::Bind)?;
    let product_config = ProductConfig::desktop_shadow(
        bind_address,
        &bootstrap.profile.settings_path,
        token.to_owned(),
    )?
    .with_notification_port(notification_port);
    let retained = retained_runtime_config(
        bootstrap.repository_root.as_deref(),
        resource_root,
        log_path,
    )?;
    let runtime_config = ProductRuntimeConfig::desktop(product_config, retained)?;
    tauri::async_runtime::block_on(start_product_runtime(runtime_config)).map_err(Into::into)
}

fn retained_runtime_config(
    repository_root: Option<&Path>,
    resource_root: &Path,
    log_path: Option<PathBuf>,
) -> Result<DesktopRetainedRuntimeConfig, NativeError> {
    let release_asset = |path: &str| repository_root.is_none().then(|| resource_root.join(path));
    let pine_bundle = required_asset(
        "JFTRADE_PINEWORKER_BUNDLE",
        repository_root
            .map(|root| root.join("var/pineworker/worker.mjs"))
            .or_else(|| release_asset("runtime/pineworker/worker.mjs")),
        "PineTS worker bundle",
    )?;
    let pine_proto = required_asset(
        "JFTRADE_PINEWORKER_PROTO",
        repository_root
            .map(|root| root.join("pkg/strategy/pineworker/proto/pineworker.proto"))
            .or_else(|| release_asset("runtime/pineworker/proto/pineworker.proto")),
        "PineTS protobuf contract",
    )?;
    let pine_runtime = env::var_os("JFTRADE_PINEWORKER_RUNTIME")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            repository_root.map_or_else(
                || resource_root.join(release_node_path()),
                |_| PathBuf::from("node"),
            )
        });
    if repository_root.is_none() && !pine_runtime.is_file() {
        return Err(NativeError::MissingAsset {
            name: "managed Node runtime",
            path: pine_runtime.to_string_lossy().into_owned(),
        });
    }
    let worker_count = env::var("JFTRADE_PINEWORKER_WORKERS")
        .ok()
        .map(|value| value.trim().parse::<usize>())
        .transpose()
        .map_err(NativeError::WorkerCount)?
        .unwrap_or(1);
    let helper = marketdata_helper_command(repository_root, resource_root)?;

    Ok(DesktopRetainedRuntimeConfig {
        pine: Some(DesktopPineRuntimeConfig {
            runtime_path: pine_runtime,
            bundle_path: pine_bundle,
            proto_path: pine_proto,
            bearer_token: random_token()?,
            worker_count,
            log_path: log_path.clone(),
        }),
        marketdata: Some(DesktopMarketDataRuntimeConfig {
            executable: helper.executable,
            prefix_args: helper.prefix_args,
            environment: helper.environment,
            bearer_token: random_token()?,
            log_path,
        }),
    })
}

fn development_repository_root() -> Result<PathBuf, NativeError> {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .find(|candidate| {
            candidate.join("Cargo.toml").is_file() && candidate.join("workers/pineworker").is_dir()
        })
        .map(Path::to_path_buf)
        .ok_or(NativeError::MissingRepositoryRoot)
}

fn absolutize_development_profile(profile: &mut DesktopProfile, repository_root: &Path) {
    if profile.channel != DesktopChannel::Dev {
        return;
    }
    profile.settings_path = repository_root
        .join(&profile.settings_path)
        .to_string_lossy()
        .into_owned();
    profile.backtest_db_path = repository_root
        .join(&profile.backtest_db_path)
        .to_string_lossy()
        .into_owned();
    profile.window_state_path = profile
        .window_state_path
        .as_ref()
        .map(|path| repository_root.join(path).to_string_lossy().into_owned());
}

fn required_asset(
    environment_key: &'static str,
    fallback: Option<PathBuf>,
    name: &'static str,
) -> Result<PathBuf, NativeError> {
    let path = env::var_os(environment_key)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .or(fallback)
        .ok_or_else(|| NativeError::MissingAsset {
            name,
            path: environment_key.to_owned(),
        })?;
    if !path.is_file() {
        return Err(NativeError::MissingAsset {
            name,
            path: path.to_string_lossy().into_owned(),
        });
    }
    Ok(path)
}

struct HelperCommand {
    executable: PathBuf,
    prefix_args: Vec<String>,
    environment: BTreeMap<String, String>,
}

fn marketdata_helper_command(
    repository_root: Option<&Path>,
    resource_root: &Path,
) -> Result<HelperCommand, NativeError> {
    if let Some(executable) = env::var_os("JFTRADE_MARKETDATA_SIDECAR")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
    {
        if !executable.is_file() {
            return Err(NativeError::MissingAsset {
                name: "market-data helper",
                path: executable.to_string_lossy().into_owned(),
            });
        }
        return Ok(HelperCommand {
            executable,
            prefix_args: Vec::new(),
            environment: BTreeMap::new(),
        });
    }
    let executable = if let Some(root) = repository_root {
        root.join(if cfg!(target_os = "windows") {
            "workers/marketdata-sidecar/.venv/Scripts/python.exe"
        } else {
            "workers/marketdata-sidecar/.venv/bin/python"
        })
    } else {
        resource_root.join(release_marketdata_helper_path())
    };
    if !executable.is_file() {
        return Err(NativeError::MissingAsset {
            name: "market-data Python runtime",
            path: executable.to_string_lossy().into_owned(),
        });
    }
    if repository_root.is_none() {
        return Ok(HelperCommand {
            executable,
            prefix_args: Vec::new(),
            environment: BTreeMap::new(),
        });
    }
    let source = repository_root
        .expect("repository root checked above")
        .join("workers/marketdata-sidecar/src");
    let mut environment = BTreeMap::new();
    environment.insert(
        "PYTHONPATH".to_owned(),
        source.to_string_lossy().into_owned(),
    );
    Ok(HelperCommand {
        executable,
        prefix_args: vec!["-m".to_owned(), "marketdata_sidecar.main".to_owned()],
        environment,
    })
}

fn release_node_path() -> &'static str {
    if cfg!(target_os = "windows") {
        "runtime/node/node.exe"
    } else {
        "runtime/node/node"
    }
}

fn release_marketdata_helper_path() -> PathBuf {
    let platform = if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "linux"
    };
    let architecture = if cfg!(target_arch = "aarch64") {
        "arm64"
    } else {
        "amd64"
    };
    let base = format!("marketdata-sidecar-{platform}-{architecture}");
    let executable = if cfg!(target_os = "windows") {
        format!("{base}.exe")
    } else {
        base.clone()
    };
    PathBuf::from("runtime/marketdata")
        .join(base)
        .join(executable)
}

fn random_token() -> Result<String, NativeError> {
    let mut bytes = [0_u8; 32];
    getrandom::fill(&mut bytes).map_err(NativeError::Random)?;
    let mut token = String::with_capacity(bytes.len() * 2);
    const HEX: &[u8; 16] = b"0123456789abcdef";
    for byte in bytes {
        token.push(char::from(HEX[usize::from(byte >> 4)]));
        token.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    Ok(token)
}

fn normalized_day(value: &str) -> Result<String, DesktopFailure> {
    let value = value.trim();
    if value.is_empty() {
        return Ok(jiff::Zoned::now().date().to_string());
    }
    let bytes = value.as_bytes();
    let valid_shape = bytes.len() == 10
        && bytes[4] == b'-'
        && bytes[7] == b'-'
        && bytes
            .iter()
            .enumerate()
            .all(|(index, byte)| matches!(index, 4 | 7) || byte.is_ascii_digit());
    let valid_date = valid_shape
        && value[0..4]
            .parse::<i32>()
            .ok()
            .zip(value[5..7].parse::<u8>().ok())
            .zip(value[8..10].parse::<u8>().ok())
            .and_then(|((year, month), day)| {
                Month::try_from(month)
                    .ok()
                    .and_then(|month| Date::from_calendar_date(year, month, day).ok())
            })
            .is_some();
    if valid_date {
        Ok(value.to_owned())
    } else {
        Err(DesktopFailure::new(
            "DESKTOP_LOG_DAY_INVALID",
            "desktop log day must use YYYY-MM-DD",
        ))
    }
}

fn log_day(file_name: &str) -> Option<String> {
    file_name
        .strip_prefix("desktop-")
        .and_then(|value| value.strip_suffix(".log"))
        .and_then(|value| normalized_day(value).ok())
}

fn parse_log_level(line: &str) -> &'static str {
    let upper = line.to_ascii_uppercase();
    for (level, tokens) in [
        (
            "ERROR",
            &[
                "LEVEL=ERROR",
                "\"LEVEL\":\"ERROR\"",
                " ERROR ",
                "[ERROR]",
                " ERROR:",
                "ERROR ",
            ][..],
        ),
        (
            "WARN",
            &[
                "LEVEL=WARN",
                "LEVEL=WARNING",
                "\"LEVEL\":\"WARN\"",
                "\"LEVEL\":\"WARNING\"",
                " WARN ",
                " WARNING ",
                "[WARN]",
                "[WARNING]",
                "WARN ",
                "WARNING ",
            ][..],
        ),
        (
            "DEBUG",
            &[
                "LEVEL=DEBUG",
                "\"LEVEL\":\"DEBUG\"",
                " DEBUG ",
                "[DEBUG]",
                "DEBUG ",
            ][..],
        ),
        (
            "INFO",
            &[
                "LEVEL=INFO",
                "\"LEVEL\":\"INFO\"",
                " INFO ",
                "[INFO]",
                "INFO ",
            ][..],
        ),
    ] {
        if tokens.iter().any(|token| upper.contains(token)) {
            return level;
        }
    }
    "INFO"
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn desktop_log_path(settings_path: &Path) -> PathBuf {
    settings_path
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
        .unwrap_or(Path::new("."))
        .join("logs")
        .join(format!("desktop-{}.log", jiff::Zoned::now().date()))
}

fn append_native_log(path: &Path, level: &str, message: &str) {
    let result = (|| -> Result<(), std::io::Error> {
        if let Some(directory) = path.parent().filter(|value| !value.as_os_str().is_empty()) {
            fs::create_dir_all(directory)?;
        }
        let mut file = OpenOptions::new().create(true).append(true).open(path)?;
        writeln!(file, "{} {} {}", now_rfc3339(), level, message)
    })();
    if let Err(error) = result {
        eprintln!("JFTrade desktop log append failed: {error}");
    }
}

fn desktop_error<E: std::fmt::Display>(code: &'static str) -> impl FnOnce(E) -> DesktopFailure {
    move |error| DesktopFailure::new(code, error.to_string())
}

fn stop_product(product: &Mutex<Option<ProductRuntimeHandle>>) {
    let handle = product.lock().ok().and_then(|mut product| product.take());
    if let Some(handle) = handle {
        let _ = tauri::async_runtime::block_on(handle.shutdown());
    }
}

#[derive(Debug, Error)]
pub enum NativeError {
    #[error("desktop home directory is unavailable")]
    MissingHome,
    #[error("invalid desktop API bind address")]
    Bind(#[source] std::net::AddrParseError),
    #[error("invalid JFTRADE_PINEWORKER_WORKERS value")]
    WorkerCount(#[source] std::num::ParseIntError),
    #[error("development repository root is unavailable")]
    MissingRepositoryRoot,
    #[error("required {name} is unavailable at {path}")]
    MissingAsset { name: &'static str, path: String },
    #[error("generate desktop API token")]
    Random(#[source] getrandom::Error),
    #[error(transparent)]
    Profile(#[from] ProfileError),
    #[error(transparent)]
    Product(#[from] ProductError),
    #[error(transparent)]
    ProductRuntime(#[from] ProductRuntimeError),
    #[error("release resource integrity failed: {0}")]
    ResourceIntegrity(String),
    #[error("Tauri runtime failed")]
    Tauri(#[from] tauri::Error),
    #[error("resolve Tauri resource directory")]
    ResourceDirectory(#[source] tauri::Error),
    #[error("native desktop runtime state is unavailable")]
    RuntimeState,
    #[error("Tauri main window is unavailable")]
    MissingMainWindow,
    #[error("Tauri tray icon is unavailable")]
    MissingTrayIcon,
    #[error("Tauri updater configuration is invalid: {0}")]
    UpdaterConfiguration(String),
    #[error(transparent)]
    Desktop(#[from] DesktopFailure),
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_port(root: &Path) -> NativeDesktopPort {
        NativeDesktopPort::new(
            DesktopStartupSnapshot {
                state: "ready".to_owned(),
                phase: "test".to_owned(),
                message: String::new(),
                started_at: "2026-08-19T00:00:00Z".to_owned(),
            },
            &root.join("settings.json"),
            "JFTrade Dev",
            NativeUpdaterConfig::Disabled,
            Arc::new(|| {}),
        )
    }

    #[test]
    fn updater_requires_complete_https_release_configuration() {
        assert_eq!(
            NativeUpdaterConfig::from_values(false, None, None).expect("development disabled"),
            NativeUpdaterConfig::Disabled
        );
        assert_eq!(
            NativeUpdaterConfig::from_values(true, None, None).expect("release unconfigured"),
            NativeUpdaterConfig::Unconfigured
        );
        assert!(
            NativeUpdaterConfig::from_values(
                true,
                Some("https://updates.jftrade.example/{{target}}/{{arch}}".to_owned()),
                None,
            )
            .is_err()
        );
        for endpoint in [
            "http://updates.jftrade.example/latest",
            "https://user:password@updates.jftrade.example/latest",
        ] {
            assert!(
                NativeUpdaterConfig::from_values(
                    true,
                    Some(endpoint.to_owned()),
                    Some("test-public-key".to_owned()),
                )
                .is_err(),
                "accepted {endpoint}"
            );
        }
        assert!(matches!(
            NativeUpdaterConfig::from_values(
                true,
                Some("https://updates.jftrade.example/{{target}}/{{arch}}".to_owned()),
                Some("test-public-key".to_owned()),
            )
            .expect("valid release updater"),
            NativeUpdaterConfig::Ready { .. }
        ));
    }

    #[test]
    fn log_reader_matches_go_filter_paging_and_day_order() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let port = test_port(directory.path());
        fs::create_dir_all(&port.log_dir).expect("create logs");
        fs::write(port.log_dir.join("desktop-2026-08-18.log"), "INFO older\n").expect("older log");
        let mut current = String::new();
        for index in 0..501 {
            let level = if index % 2 == 0 { "WARN" } else { "INFO" };
            current.push_str(&format!("{level} item-{index}\n"));
        }
        fs::write(
            port.log_dir.join("desktop-2026-08-19.log"),
            current.as_bytes(),
        )
        .expect("current log");
        fs::write(port.log_dir.join("desktop-2026-99-99.log"), b"ignored")
            .expect("invalid log name");

        let days = port.log_list_days().expect("list days");
        assert_eq!(
            days.into_iter().map(|value| value.day).collect::<Vec<_>>(),
            ["2026-08-19", "2026-08-18"]
        );
        let page = port
            .log_read_page("2026-08-19", "WARN", "item", LATEST_LOG_OFFSET, 100)
            .expect("read last page");
        assert_eq!(page.total, 251);
        assert_eq!(page.offset, 200);
        assert_eq!(page.items.len(), 51);
        assert!(page.items[0].text.contains("item-400"));
        let default_page = port
            .log_read_page("2026-08-18", "ALL", "", 0, 0)
            .expect("default page");
        assert_eq!(default_page.limit, DEFAULT_LOG_LIMIT);
    }

    #[test]
    fn native_runtime_events_append_to_the_existing_daily_log_contract() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let log_path = desktop_log_path(&settings_path);
        append_native_log(&log_path, "INFO", "runtime ready");
        append_native_log(&log_path, "ERROR", "worker stopped");
        let contents = fs::read_to_string(&log_path).expect("read desktop log");
        assert!(contents.contains(" INFO runtime ready"));
        assert!(contents.contains(" ERROR worker stopped"));
        assert_eq!(
            log_path.parent(),
            Some(directory.path().join("logs").as_path())
        );
    }

    #[test]
    fn development_profile_paths_are_anchored_to_the_repository_not_process_cwd() {
        let root = Path::new("/fixture/jftrade-main");
        let mut profile = DesktopProfile::resolve(
            DesktopChannel::Dev,
            &PlatformPaths {
                platform: DesktopPlatform::Darwin,
                home_dir: "/fixture/home".to_owned(),
                config_dir: String::new(),
                local_app_data: String::new(),
                xdg_data_home: String::new(),
            },
        )
        .expect("development profile");
        absolutize_development_profile(&mut profile, root);
        assert_eq!(
            profile.settings_path,
            "/fixture/jftrade-main/var/jftrade-api/settings.json"
        );
        assert_eq!(
            profile.backtest_db_path,
            "/fixture/jftrade-main/var/jftrade-api/backtest.db"
        );
        assert_eq!(profile.window_state_path, None);
    }

    #[test]
    fn native_boundaries_reject_invalid_days_and_generate_strong_tokens() {
        for invalid in ["2026-02-30", "2026-13-01", "../2026-08-19", "20260819"] {
            assert!(normalized_day(invalid).is_err(), "accepted {invalid}");
        }
        let token = random_token().expect("random token");
        assert_eq!(token.len(), 64);
        assert!(token.bytes().all(|byte| byte.is_ascii_hexdigit()));
    }
}
