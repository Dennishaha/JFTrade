use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_watchlist_write_port::{
    WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
};
use super::super::product_watchlist_write_test_cutover::WatchlistSqliteTestCutoverPort;
use super::*;

#[derive(Debug)]
struct FixtureWatchlistWritePort;

impl WatchlistWritePort for FixtureWatchlistWritePort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        Ok(json!({
            "accepted": true,
            "route": mutation.value["route"].clone(),
        }))
    }
}

#[derive(Debug)]
struct SequencedWatchlistWritePort {
    responses: Mutex<VecDeque<Result<Value, WatchlistWritePortError>>>,
    calls: Mutex<Vec<WatchlistWriteMutation>>,
}

impl SequencedWatchlistWritePort {
    fn new(responses: impl IntoIterator<Item = Result<Value, WatchlistWritePortError>>) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<WatchlistWriteMutation> {
        self.calls
            .lock()
            .expect("watchlist write calls lock")
            .clone()
    }
}

impl WatchlistWritePort for SequencedWatchlistWritePort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        self.calls
            .lock()
            .expect("watchlist write calls lock")
            .push(mutation.clone());
        self.responses
            .lock()
            .expect("watchlist write responses lock")
            .pop_front()
            .expect("watchlist write rehearsal response")
    }
}

#[tokio::test]
async fn watchlist_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_write_port(Arc::new(FixtureWatchlistWritePort));
    let handle = start_product(config)
        .await
        .expect("start watchlist product");
    assert_eq!(handle.startup_record().owned_routes, 56);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/watchlist/groups" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["route"], "create-group");
    handle.shutdown().await.expect("shutdown watchlist product");
}

#[tokio::test]
async fn watchlist_write_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"watchlist-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let success =
        |route: &str| Ok(json!({"accepted": true, "source": "rust-product", "route": route}));
    let port = Arc::new(SequencedWatchlistWritePort::new([
        Err(WatchlistWritePortError {
            status: 503,
            code: "WATCHLIST_WRITE_UNAVAILABLE".to_owned(),
            message: "fixture watchlist unavailable".to_owned(),
        }),
        success("create-group"),
        Err(WatchlistWritePortError {
            status: 409,
            code: "WATCHLIST_BUSY".to_owned(),
            message: "fixture revision conflict".to_owned(),
        }),
        success("delete-group"),
        success("delete-binding"),
        success("update-group"),
        success("preview-import"),
        success("commit-import"),
        success("batch-quotes"),
        success("replace-memberships"),
        success("create-group"),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_write_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config)
        .await
        .expect("start watchlist product");
    assert_eq!(handle.startup_record().owned_routes, 56);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/watchlist"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "watchlist-write-product"),
    ];

    let unauthorized = request_json_with_status(
        address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/watchlist"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);

    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 503);
    assert_eq!(response["error"]["code"], "WATCHLIST_WRITE_UNAVAILABLE");
    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["route"], "create-group");

    let (status, response) = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/watchlist/groups/group-1",
        None,
        &browser_headers,
    )
    .await;
    assert_eq!(status, 409);
    assert_eq!(response["error"]["code"], "WATCHLIST_BUSY");
    let (status, response) = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/watchlist/groups/group-1",
        None,
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["route"], "delete-group");

    for (method, path, body, route) in [
        (
            "DELETE",
            "/api/v1/watchlist/bindings?bindingId=binding-1",
            None,
            "delete-binding",
        ),
        (
            "PATCH",
            "/api/v1/watchlist/groups/group-1",
            Some(r#"{"name":"Growth","expectedRevision":2}"#),
            "update-group",
        ),
        (
            "POST",
            "/api/v1/watchlist/imports/preview",
            Some(r#"{"sourceId":"source-1","remoteGroupId":"remote-1"}"#),
            "preview-import",
        ),
        (
            "POST",
            "/api/v1/watchlist/imports/preview-1/commit",
            Some(r#"{"deleteInstrumentIds":["US:AAPL"]}"#),
            "commit-import",
        ),
        (
            "POST",
            "/api/v1/watchlist/quotes/batch",
            Some(r#"{"instrumentIds":["US:AAPL"]}"#),
            "batch-quotes",
        ),
        (
            "PUT",
            "/api/v1/watchlist/instruments/US/AAPL/memberships",
            Some(r#"{"groupIds":["group-1"],"newGroupNames":[],"expectedRevision":2}"#),
            "replace-memberships",
        ),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["route"], route, "{method} {path}");
    }
    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["route"], "create-group");
    assert_eq!(port.calls().len(), 11);
    handle.shutdown().await.expect("shutdown watchlist product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_watchlist_write_port(Arc::new(FixtureWatchlistWritePort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart watchlist product");
    let (status, response) = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Restarted"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["route"], "create-group");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted watchlist product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read restarted settings"),
        settings_before
    );
}

#[tokio::test]
async fn watchlist_sqlite_test_cutover_replays_transport_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("watchlist-test-cutover.db");
    std::fs::write(&settings_path, b"{\"seed\":\"watchlist-durable\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("settings");
    seed_go_watchlist_schema(&database_path);
    let port =
        Arc::new(WatchlistSqliteTestCutoverPort::open(&database_path).expect("open adapter"));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_write_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    for (method, path, body, route) in [
        (
            "POST",
            "/api/v1/watchlist/groups",
            Some(r#"{"name":"Value"}"#),
            "create-group",
        ),
        (
            "PATCH",
            "/api/v1/watchlist/groups/group-1",
            Some(r#"{"name":"Growth 2","expectedRevision":1}"#),
            "update-group",
        ),
        (
            "POST",
            "/api/v1/watchlist/imports/preview",
            Some(r#"{"sourceId":"source-1","remoteGroupId":"remote-1"}"#),
            "preview-import",
        ),
        (
            "POST",
            "/api/v1/watchlist/quotes/batch",
            Some(r#"{"instrumentIds":["US:AAPL"]}"#),
            "batch-quotes",
        ),
        (
            "PUT",
            "/api/v1/watchlist/instruments/US/AAPL/memberships",
            Some(r#"{"groupIds":["group-1"],"expectedRevision":0}"#),
            "replace-memberships",
        ),
        (
            "DELETE",
            "/api/v1/watchlist/bindings?bindingId=binding-1",
            None,
            "delete-binding",
        ),
        (
            "DELETE",
            "/api/v1/watchlist/groups/group-1",
            None,
            "delete-group",
        ),
    ] {
        let response = request_json_with_status(address, method, path, body, &[]).await;
        assert_eq!(response.0, 200, "{method} {path}");
        assert_eq!(
            response.1["data"]["route"],
            Value::String(route.to_owned()),
            "{method} {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
    drop(port);
    let reopened =
        Arc::new(WatchlistSqliteTestCutoverPort::open(&database_path).expect("reopen adapter"));
    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restart address"),
        &settings_path,
    )
    .expect("restart config")
    .with_watchlist_write_port(reopened.clone());
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Restarted"}"#),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["route"], "create-group");
    restarted.shutdown().await.expect("shutdown restart");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after restart"),
        settings_before
    );
}

fn seed_go_watchlist_schema(path: &std::path::Path) {
    let connection = rusqlite::Connection::open(path).expect("create watchlist fixture");
    connection
        .execute_batch(
            "CREATE TABLE watchlist_groups (
                group_id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                name_key TEXT NOT NULL UNIQUE,
                is_default INTEGER NOT NULL DEFAULT 0,
                protected INTEGER NOT NULL DEFAULT 0,
                revision INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE UNIQUE INDEX watchlist_groups_one_default
                ON watchlist_groups(is_default) WHERE is_default = 1;
            CREATE TABLE watchlist_instruments (
                instrument_id TEXT PRIMARY KEY,
                market TEXT NOT NULL,
                symbol TEXT NOT NULL,
                name TEXT NOT NULL DEFAULT '',
                instrument_type TEXT NOT NULL DEFAULT '',
                membership_revision INTEGER NOT NULL DEFAULT 0,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE TABLE watchlist_memberships (
                group_id TEXT NOT NULL,
                instrument_id TEXT NOT NULL,
                created_at TEXT NOT NULL,
                PRIMARY KEY (group_id, instrument_id)
            );
            CREATE INDEX watchlist_memberships_instrument
                ON watchlist_memberships(instrument_id, group_id);
            CREATE TABLE watchlist_sources (
                source_id TEXT PRIMARY KEY,
                broker TEXT NOT NULL,
                display_name TEXT NOT NULL,
                status TEXT NOT NULL,
                last_error TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL
            );
            CREATE TABLE watchlist_remote_groups (
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                name TEXT NOT NULL,
                group_type TEXT NOT NULL,
                ambiguous INTEGER NOT NULL DEFAULT 0,
                member_count INTEGER NOT NULL DEFAULT 0,
                remote_hash TEXT NOT NULL DEFAULT '',
                observed_at TEXT NOT NULL,
                PRIMARY KEY (source_id, remote_group_id)
            );
            CREATE TABLE watchlist_bindings (
                binding_id TEXT PRIMARY KEY,
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                remote_name TEXT NOT NULL,
                local_group_id TEXT NOT NULL,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                UNIQUE (source_id, remote_group_id)
            );
            CREATE INDEX watchlist_bindings_local_group
                ON watchlist_bindings(local_group_id);
            CREATE TABLE watchlist_remote_memberships (
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                instrument_id TEXT NOT NULL,
                remote_hash TEXT NOT NULL,
                observed_at TEXT NOT NULL,
                PRIMARY KEY (source_id, remote_group_id, instrument_id)
            );
            CREATE TABLE watchlist_membership_origins (
                group_id TEXT NOT NULL,
                instrument_id TEXT NOT NULL,
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                last_imported_at TEXT NOT NULL,
                PRIMARY KEY (group_id, instrument_id, source_id, remote_group_id)
            );
            CREATE INDEX watchlist_membership_origins_instrument
                ON watchlist_membership_origins(instrument_id, group_id);
            CREATE TABLE watchlist_instrument_aliases (
                source_id TEXT NOT NULL,
                alias_kind TEXT NOT NULL,
                alias_value TEXT NOT NULL,
                instrument_id TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                PRIMARY KEY (source_id, alias_kind, alias_value)
            );
            CREATE TABLE watchlist_import_previews (
                preview_id TEXT PRIMARY KEY,
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                remote_group_name TEXT NOT NULL,
                local_group_id TEXT NOT NULL DEFAULT '',
                new_group_name TEXT NOT NULL DEFAULT '',
                remote_hash TEXT NOT NULL,
                local_group_revision INTEGER NOT NULL,
                added_json TEXT NOT NULL,
                unchanged_json TEXT NOT NULL,
                local_only_json TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'pending',
                created_at TEXT NOT NULL,
                expires_at TEXT NOT NULL
            );
            CREATE INDEX watchlist_import_previews_expiry
                ON watchlist_import_previews(status, expires_at);
            CREATE TABLE watchlist_import_runs (
                run_id TEXT PRIMARY KEY,
                preview_id TEXT NOT NULL,
                source_id TEXT NOT NULL,
                remote_group_id TEXT NOT NULL,
                remote_group_name TEXT NOT NULL,
                local_group_id TEXT NOT NULL,
                status TEXT NOT NULL,
                added_count INTEGER NOT NULL,
                removed_count INTEGER NOT NULL,
                unchanged_count INTEGER NOT NULL,
                remote_hash TEXT NOT NULL,
                created_at TEXT NOT NULL,
                completed_at TEXT NOT NULL
            );
            CREATE INDEX watchlist_import_runs_source
                ON watchlist_import_runs(source_id, run_id DESC);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('watchlist', 1, '2026-08-24T04:00:00Z');
            INSERT INTO watchlist_groups (group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
                VALUES ('default', '自选股', '自选股', 1, 1, 1, '2026-08-24T04:00:00Z', '2026-08-24T04:00:00Z');
            INSERT INTO watchlist_groups (group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
                VALUES ('group-1', 'Growth', 'growth', 0, 0, 1, '2026-08-24T04:00:00Z', '2026-08-24T04:00:00Z');",
        )
        .expect("seed Go-compatible watchlist schema");
}
