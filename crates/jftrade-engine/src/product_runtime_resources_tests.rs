//! Inventory coverage for the composition-root resource projection.
//!
//! The Go baseline declares these descriptors in
//! `internal/app/apiserver/runtime/resources.go`; the projection must expose the
//! same owners, paths and environment overrides because the desktop console
//! renders the resulting `system.runtimeResources` payload.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use super::*;
use jftrade_api::AccessPolicy;

fn test_config(settings_path: &Path) -> ProductConfig {
    ProductConfig::new(
        "127.0.0.1:3000".parse().expect("loopback bind"),
        settings_path,
        AccessPolicy::default(),
    )
    .expect("product config")
}

// Parity: go:452dea11:internal/app/apiserver/servercore/server_test.go:50 TestNewServerUsesStrategyRuntimeDBEnvOverride
// Parity: go:452dea11:internal/app/apiserver/runtime/research_runtime_test.go:9 TestResearchDatabasePathAndRuntimeResource
// Parity: go:452dea11:internal/app/apiserver/runtime/resources_test.go:9 TestRuntimeResourcesDeclareOwnersAndDerivedPaths
#[test]
fn runtime_resources_declare_go_owners_and_derived_paths() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let runtime_dir = directory.path().join("runtime");
    let settings_path = runtime_dir.join("settings.json");
    let config = test_config(&settings_path);

    let resources = product_resources(&config);
    let by_id = resources
        .iter()
        .map(|resource| (resource.id.as_str(), resource))
        .collect::<BTreeMap<_, _>>();

    for resource in &resources {
        assert!(!resource.id.is_empty(), "{resource:?}");
        assert!(!resource.owner.is_empty(), "{resource:?}");
        assert!(!resource.kind.is_empty(), "{resource:?}");
        assert!(!resource.path.is_empty(), "{resource:?}");
        assert!(
            !resource.health_provider.starts_with("data-migration/"),
            "obsolete health provider: {resource:?}"
        );
        if resource.kind == "sqlite" && resource.critical {
            assert!(
                resource.health_provider.starts_with("data-management/"),
                "critical sqlite resource keeps data-management health: {resource:?}"
            );
        }
    }

    let settings = by_id["settings-file"];
    assert_eq!(settings.owner, "settings");
    assert_eq!(PathBuf::from(&settings.path), settings_path);
    assert!(settings.critical);

    let backtest = by_id["backtest-kline-db"];
    assert_eq!(backtest.owner, "backtest");
    assert_eq!(
        PathBuf::from(&backtest.path),
        runtime_dir.join("backtest.db")
    );
    assert_eq!(backtest.environment_override, "JFTRADE_BACKTEST_DB");

    let orders = by_id["execution-orders-db"];
    assert_eq!(orders.owner, "trading");
    assert_eq!(
        PathBuf::from(&orders.path),
        runtime_dir.join("execution-orders.db")
    );

    let catalog = by_id["strategy-catalog"];
    assert_eq!(catalog.owner, "strategy");
    assert_eq!(catalog.kind, "sqlite");
    assert_eq!(
        PathBuf::from(&catalog.path),
        runtime_dir.join("strategy-runtime.db")
    );
    assert_eq!(catalog.environment_override, "JFTRADE_STRATEGY_RUNTIME_DB");
    assert_eq!(catalog.schema_owner, "strategy catalog tables");
    assert!(catalog.critical);

    let designs = by_id["strategy-designs"];
    assert_eq!(designs.kind, "sqlite");
    assert_eq!(
        PathBuf::from(&designs.path),
        runtime_dir.join("strategy-runtime.db")
    );
    assert_eq!(designs.schema_owner, "strategy design tables");

    let session = by_id["adk-session-db"];
    assert_eq!(session.owner, "assistant/runtime");
    assert_eq!(session.environment_override, "JFTRADE_ADK_SESSION_DB");
    assert!(!session.critical);

    let watchlist = by_id["watchlist-db"];
    assert_eq!(watchlist.owner, "watchlist");
    assert_eq!(
        PathBuf::from(&watchlist.path),
        runtime_dir.join("watchlists.db")
    );
    assert_eq!(watchlist.environment_override, "JFTRADE_WATCHLIST_DB");

    let research = by_id["research-db"];
    assert_eq!(research.owner, "research");
    assert_eq!(
        PathBuf::from(&research.path),
        runtime_dir.join("research.db")
    );
    assert_eq!(research.environment_override, "JFTRADE_RESEARCH_DB");
    assert!(research.critical);

    let control = by_id["real-trade-control"];
    assert_eq!(control.owner, "trading");
    assert_eq!(
        PathBuf::from(&control.path),
        runtime_dir.join("real-trade-control.json")
    );
    assert_eq!(
        control.environment_override,
        "JFTRADE_REAL_TRADE_CONTROL_PATH"
    );

    let plugins = by_id["strategy-plugin-dir"];
    assert_eq!(plugins.kind, "directory");
    assert_eq!(PathBuf::from(&plugins.path), runtime_dir.join("plugins"));
    assert!(!plugins.critical);

    let calendars = by_id["exchange-calendar-dir"];
    assert_eq!(calendars.owner, "system/exchange-calendar");
    assert_eq!(
        PathBuf::from(&calendars.path),
        runtime_dir.join("exchange-calendars")
    );
    assert_eq!(
        calendars.environment_override,
        "JFTRADE_EXCHANGE_CALENDAR_DIR"
    );
}

// Parity: go:452dea11:internal/app/apiserver/runtime/resources_test.go:9 TestRuntimeResourcesDeclareOwnersAndDerivedPaths
#[test]
fn runtime_resources_group_critical_entries_before_assistant_and_directory_entries() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("runtime").join("settings.json");
    let config = test_config(&settings_path);

    let ids = product_resources(&config)
        .iter()
        .map(|resource| resource.id.clone())
        .collect::<Vec<_>>();
    assert_eq!(
        ids,
        vec![
            "settings-file",
            "backtest-kline-db",
            "backtest-run-db",
            "strategy-catalog",
            "strategy-designs",
            "strategy-runtime-db",
            "execution-orders-db",
            "watchlist-db",
            "research-db",
            "real-trade-control",
            "adk-db",
            "adk-session-db",
            "adk-artifact-db",
            "adk-secrets",
            "adk-skills-dir",
            "exchange-calendar-dir",
            "strategy-plugin-dir",
        ],
        "runtime inventory keeps Go grouping plus the Rust artifact database"
    );
}

// Parity: go:452dea11:internal/app/apiserver/runtime/runtime_test.go:116 TestRuntimePathDerivationFallsBackForRelativeSettings
#[test]
fn runtime_resources_keep_the_settings_directory_for_relative_paths() {
    let config = test_config(Path::new("settings.json"));
    let resources = product_resources(&config);
    let by_id = resources
        .iter()
        .map(|resource| (resource.id.as_str(), resource))
        .collect::<BTreeMap<_, _>>();

    assert_eq!(
        PathBuf::from(&by_id["settings-file"].path),
        PathBuf::from("settings.json")
    );
    assert_eq!(
        PathBuf::from(&by_id["real-trade-control"].path),
        PathBuf::from("real-trade-control.json")
    );
    assert_eq!(
        PathBuf::from(&by_id["strategy-plugin-dir"].path),
        PathBuf::from("plugins")
    );
    assert_eq!(
        PathBuf::from(&by_id["exchange-calendar-dir"].path),
        PathBuf::from("exchange-calendars")
    );
}

// Parity: go:452dea11:internal/app/apiserver/runtime/resources_test.go:78 TestRuntimeResourceSummaryIncludesCountAndItems
#[test]
fn runtime_resource_summary_count_matches_projected_items() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let settings_path = directory.path().join("runtime").join("settings.json");
    let config = test_config(&settings_path);

    let state = ProductRuntimeState::product_only(&config);
    let snapshot = state.snapshot();
    assert_eq!(snapshot.resources.len(), 17);
    assert_eq!(
        snapshot
            .resources
            .iter()
            .map(|resource| resource.id.clone())
            .collect::<Vec<_>>()
            .len(),
        snapshot.resources.len()
    );
}
