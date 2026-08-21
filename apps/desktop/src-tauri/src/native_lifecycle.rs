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
