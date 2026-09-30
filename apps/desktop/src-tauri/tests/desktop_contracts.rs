use jftrade_desktop::contract::{
    DESKTOP_COMMANDS, DESKTOP_LOG_APPEND_EVENT, DESKTOP_UPDATE_AVAILABLE_EVENT,
    DESKTOP_UPDATE_COMMANDS,
};
use jftrade_desktop::lifecycle::{
    LifecycleAction, LifecycleError, ProcessRole, ProcessSupervisor, ReleaseAsset, RuntimePlan,
    SHUTDOWN_TIMEOUT_MILLIS,
};
use jftrade_desktop::links::{LinkTarget, classify_link};
use jftrade_desktop::profile::{
    DesktopChannel, DesktopPlatform, DesktopProfile, PlatformPaths, ProfileError, product_data_dir,
};

fn asset(role: ProcessRole, path: &str, digest_byte: char) -> ReleaseAsset {
    ReleaseAsset {
        role,
        relative_path: path.to_owned(),
        sha256: digest_byte.to_string().repeat(64),
        executable: true,
    }
}

fn assets() -> Vec<ReleaseAsset> {
    vec![
        asset(ProcessRole::Engine, "bin/jftrade-engine", 'a'),
        asset(ProcessRole::PineWorker, "bin/pineworker", 'b'),
        asset(
            ProcessRole::MarketdataSidecar,
            "bin/marketdata-sidecar",
            'c',
        ),
    ]
}

// Parity: go:452dea11:internal/app/apiserver/server_test.go:150 TestResolveDesktopRuntimeConfigUsesProfileBindInsteadOfPersistedInterfaceBind
#[test]
fn build_profiles_preserve_tauri_identity_and_data_isolation() {
    let macos = PlatformPaths {
        platform: DesktopPlatform::Darwin,
        home_dir: "/Users/alice".to_owned(),
        config_dir: "/Users/alice/Library/Application Support".to_owned(),
        local_app_data: String::new(),
        xdg_data_home: String::new(),
    };
    let development = DesktopProfile::resolve(DesktopChannel::Dev, &macos).unwrap();
    let release = DesktopProfile::resolve(DesktopChannel::Release, &macos).unwrap();
    assert_eq!(development.product_identifier, "com.jftrade.desktop.dev");
    assert_eq!(development.api_bind, "127.0.0.1:3008");
    assert_eq!(development.settings_path, "var/jftrade-api/settings.json");
    assert_eq!(release.product_identifier, "com.jftrade.desktop");
    assert_eq!(release.api_bind, "127.0.0.1:6699");
    assert!(release.update_checks_enabled);
    assert_eq!(
        release.settings_path,
        "/Users/alice/Library/Application Support/JFTrade/settings.json"
    );
    assert_eq!(
        release.window_state_path.as_deref(),
        Some("/Users/alice/Library/Application Support/JFTrade/desktop-state.json")
    );
}

fn platform_paths(
    platform: DesktopPlatform,
    home_dir: &str,
    config_dir: &str,
    local_app_data: &str,
    xdg_data_home: &str,
) -> PlatformPaths {
    PlatformPaths {
        platform,
        home_dir: home_dir.to_owned(),
        config_dir: config_dir.to_owned(),
        local_app_data: local_app_data.to_owned(),
        xdg_data_home: xdg_data_home.to_owned(),
    }
}

// Parity: go:452dea11:internal/desktop/runtime_path_test.go:8 TestProductDataDirByPlatform
#[test]
fn product_data_dir_covers_every_platform_base_directory() {
    let macos = platform_paths(
        DesktopPlatform::Darwin,
        "/Users/alice",
        "/Users/alice/Library/Application Support",
        "",
        "",
    );
    assert_eq!(
        product_data_dir(&macos).unwrap(),
        "/Users/alice/Library/Application Support/JFTrade"
    );

    let macos_fallback = platform_paths(DesktopPlatform::Darwin, "/Users/alice", "", "", "");
    assert_eq!(
        product_data_dir(&macos_fallback).unwrap(),
        "/Users/alice/Library/Application Support/JFTrade"
    );

    // Rust joins with `/` and keeps the platform prefix verbatim, so the Windows
    // result mixes separators; Windows accepts both forms.
    let windows_local_app_data = platform_paths(
        DesktopPlatform::Windows,
        r"C:\Users\alice",
        r"C:\Users\alice\AppData\Roaming",
        r"C:\Users\alice\AppData\Local",
        "",
    );
    assert_eq!(
        product_data_dir(&windows_local_app_data).unwrap(),
        r"C:\Users\alice\AppData\Local/JFTrade"
    );

    let windows_config_fallback = platform_paths(
        DesktopPlatform::Windows,
        r"C:\Users\alice",
        r"C:\Users\alice\AppData\Roaming",
        "",
        "",
    );
    assert_eq!(
        product_data_dir(&windows_config_fallback).unwrap(),
        r"C:\Users\alice\AppData\Roaming/JFTrade"
    );

    // Not covered by the Go table: the Go helper degrades to the relative
    // `JFTrade` directory, Rust fails closed instead of inventing one.
    let windows_without_base =
        platform_paths(DesktopPlatform::Windows, r"C:\Users\alice", "", "", "");
    assert_eq!(
        product_data_dir(&windows_without_base),
        Err(ProfileError::MissingDataDirectory(DesktopPlatform::Windows))
    );

    let linux_xdg = platform_paths(DesktopPlatform::Linux, "/home/alice", "", "", "/data/alice");
    assert_eq!(product_data_dir(&linux_xdg).unwrap(), "/data/alice/jftrade");

    let linux_fallback = platform_paths(DesktopPlatform::Linux, "/home/alice", "", "", "");
    assert_eq!(
        product_data_dir(&linux_fallback).unwrap(),
        "/home/alice/.local/share/jftrade"
    );

    let trimmed_home = platform_paths(DesktopPlatform::Linux, " /home/alice ", "", "", "");
    assert_eq!(
        product_data_dir(&trimmed_home).unwrap(),
        "/home/alice/.local/share/jftrade"
    );
}

#[test]
fn desktop_links_accept_only_docs_and_http_targets() {
    assert_eq!(
        classify_link("docs/reference/index.html#orders").unwrap(),
        LinkTarget::Docs("/docs/reference/#orders".to_owned())
    );
    assert_eq!(
        classify_link("https://example.com/releases").unwrap(),
        LinkTarget::External("https://example.com/releases".to_owned())
    );
    for rejected in [
        "javascript:alert(1)",
        "file:///tmp/readme",
        "/settings",
        "/docs/%2e%2e/settings",
        "../docs/index.html",
    ] {
        assert!(classify_link(rejected).is_err(), "accepted {rejected:?}");
    }
}

#[test]
fn runtime_plan_rejects_missing_duplicate_and_unsafe_assets() {
    let mut incomplete = assets();
    incomplete.pop();
    assert_eq!(
        RuntimePlan::new(incomplete).unwrap_err(),
        LifecycleError::IncompleteAssetSet
    );
    let mut duplicate = assets();
    duplicate.push(asset(ProcessRole::Engine, "bin/other", 'd'));
    assert_eq!(
        RuntimePlan::new(duplicate).unwrap_err(),
        LifecycleError::IncompleteAssetSet
    );
    let mut unsafe_assets = assets();
    unsafe_assets[0].relative_path = "../jftrade-engine".to_owned();
    assert!(matches!(
        RuntimePlan::new(unsafe_assets),
        Err(LifecycleError::UnsafePath(_))
    ));
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:110 TestStartForRunArgsConfiguresRuntimeAndFrontend
#[test]
fn lifecycle_starts_in_dependency_order_and_shuts_down_in_reverse() {
    let plan = RuntimePlan::new(assets()).unwrap();
    let mut supervisor = FakeSupervisor::default();
    let report = plan.start(&mut supervisor).unwrap();
    assert!(report.ready);
    assert_eq!(
        report
            .events
            .iter()
            .filter(|event| event.action == LifecycleAction::Ready)
            .map(|event| event.role)
            .collect::<Vec<_>>(),
        ProcessRole::START_ORDER
    );
    let shutdown = plan.shutdown(&mut supervisor).unwrap();
    assert_eq!(
        shutdown.iter().map(|event| event.role).collect::<Vec<_>>(),
        ProcessRole::SHUTDOWN_ORDER
    );
    assert!(
        supervisor
            .shutdowns
            .iter()
            .all(|(_, timeout)| *timeout == SHUTDOWN_TIMEOUT_MILLIS)
    );
}

// Parity: go:452dea11:internal/app/apiserver/desktop_api_startup_test.go:148 TestStartDesktopWithConfigClosesSidecarWhenReadinessTargetFails
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:616 TestStartForRunArgsStopsAtFailingStartupStage
#[test]
// Parity: go:452dea11:internal/app/apiserver/application/installers_test.go:49 TestInstallersRollbackPartialInitialization
fn readiness_failure_reclaims_every_started_process_without_starting_dependents() {
    let plan = RuntimePlan::new(assets()).unwrap();
    let mut supervisor = FakeSupervisor {
        fail_ready: Some(ProcessRole::PineWorker),
        ..FakeSupervisor::default()
    };
    let report = plan.start(&mut supervisor).unwrap();
    assert!(!report.ready);
    assert_eq!(report.failure_role, Some(ProcessRole::PineWorker));
    assert_eq!(
        supervisor.started,
        vec![ProcessRole::Engine, ProcessRole::PineWorker]
    );
    assert_eq!(
        supervisor
            .shutdowns
            .iter()
            .map(|(role, _)| *role)
            .collect::<Vec<_>>(),
        vec![ProcessRole::PineWorker, ProcessRole::Engine]
    );
}

#[test]
fn frontend_facade_contract_names_are_versioned_in_one_place() {
    assert_eq!(DESKTOP_COMMANDS.len(), 10);
    assert!(DESKTOP_COMMANDS.contains(&"desktop_startup_snapshot"));
    assert!(DESKTOP_COMMANDS.contains(&"desktop_window_open_logs"));
    assert_eq!(DESKTOP_UPDATE_COMMANDS, ["desktop_update_install"]);
    assert_eq!(DESKTOP_LOG_APPEND_EVENT, "jftrade:desktop-log:append");
    assert_eq!(
        DESKTOP_UPDATE_AVAILABLE_EVENT,
        "jftrade:desktop-update:available"
    );
}

#[test]
// Parity: go:452dea11:internal/frontendassets/dev_test.go:7 TestFileSystemReportsExternalAssetsForDevelopmentBuild
fn desktop_shell_keeps_development_frontend_external_and_embeds_staged_dist_for_release() {
    let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
    let config: serde_json::Value = serde_json::from_slice(
        &std::fs::read(manifest_dir.join("tauri.conf.json")).expect("read tauri configuration"),
    )
    .expect("decode tauri configuration");
    let build = &config["build"];
    assert_eq!(build["devUrl"], "http://127.0.0.1:3003");
    assert!(
        build["beforeDevCommand"]
            .as_str()
            .expect("beforeDevCommand")
            .contains("dev-tauri-frontend.mjs"),
        "development frontend must be served by the external dev process"
    );
    assert_eq!(build["frontendDist"], "../../web/dist");
    assert_eq!(config["bundle"]["active"], true);
}

#[derive(Default)]
struct FakeSupervisor {
    fail_ready: Option<ProcessRole>,
    started: Vec<ProcessRole>,
    shutdowns: Vec<(ProcessRole, u64)>,
}

impl ProcessSupervisor for FakeSupervisor {
    fn start(&mut self, asset: &ReleaseAsset) -> Result<(), String> {
        self.started.push(asset.role);
        Ok(())
    }

    fn wait_ready(&mut self, role: ProcessRole) -> Result<(), String> {
        if self.fail_ready == Some(role) {
            Err("not ready".to_owned())
        } else {
            Ok(())
        }
    }

    fn shutdown(&mut self, role: ProcessRole, timeout_millis: u64) -> Result<(), String> {
        self.shutdowns.push((role, timeout_millis));
        Ok(())
    }
}
