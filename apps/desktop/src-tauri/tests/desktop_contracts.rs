use jftrade_desktop::contract::{
    DESKTOP_COMMANDS, DESKTOP_LOG_APPEND_EVENT, DESKTOP_UPDATE_AVAILABLE_EVENT,
    STAGE9_DESKTOP_COMMANDS,
};
use jftrade_desktop::lifecycle::{
    LifecycleAction, LifecycleError, ProcessRole, ProcessSupervisor, ReleaseAsset, RuntimePlan,
    SHUTDOWN_TIMEOUT_MILLIS,
};
use jftrade_desktop::links::{LinkTarget, classify_link};
use jftrade_desktop::profile::{DesktopChannel, DesktopPlatform, DesktopProfile, PlatformPaths};

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

#[test]
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
    assert_eq!(STAGE9_DESKTOP_COMMANDS, ["desktop_update_install"]);
    assert_eq!(DESKTOP_LOG_APPEND_EVENT, "jftrade:desktop-log:append");
    assert_eq!(
        DESKTOP_UPDATE_AVAILABLE_EVENT,
        "jftrade:desktop-update:available"
    );
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
