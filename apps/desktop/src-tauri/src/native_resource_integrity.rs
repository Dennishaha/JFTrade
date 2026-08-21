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
