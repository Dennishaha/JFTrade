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
