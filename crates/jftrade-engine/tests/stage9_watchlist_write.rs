#[path = "../src/product_watchlist_write_port.rs"]
mod product_watchlist_write_port;
#[path = "../src/product_watchlist_write_test_cutover.rs"]
mod product_watchlist_write_test_cutover;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use product_watchlist_write_port::{
    WATCHLIST_WRITE_ROUTES, WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
    WatchlistWriteRequest, dispatch_watchlist_write, watchlist_write_routes,
};
use product_watchlist_write_test_cutover::WatchlistSqliteTestCutoverPort;
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T00:00:00Z";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Fixture {
    version: String,
    timestamp: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    requests: Vec<FixtureRequest>,
    expected: Vec<FixtureExpected>,
    #[serde(default)]
    calls: Vec<Value>,
    #[serde(default)]
    port_mode: String,
    #[serde(default)]
    concurrent: bool,
    expected_observation: Value,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    path: String,
    body: Option<String>,
    #[serde(default)]
    context: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    envelope: Value,
    port_call: bool,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, WatchlistWritePortError>>>,
    calls: Mutex<Vec<Value>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .expected
            .iter()
            .filter(|expected| expected.port_call)
            .map(|expected| {
                if expected.envelope["ok"] == true {
                    Ok(expected.envelope["data"].clone())
                } else {
                    Err(error_from_envelope(expected))
                }
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("watchlist write response lock")
                .is_empty(),
            "fixture port responses remain for {case_name}"
        );
    }
}

impl WatchlistWritePort for FixturePort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        self.calls
            .lock()
            .expect("watchlist write calls lock")
            .push(mutation.value.clone());
        self.responses
            .lock()
            .expect("watchlist write response lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(WatchlistWritePortError {
                    status: 500,
                    code: "WATCHLIST_FAILED".to_owned(),
                    message: "fixture response missing".to_owned(),
                })
            })
    }
}

#[test]
fn watchlist_write_fixture_matches_go_owner_for_all_eight_routes() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.watchlist-write.v1");
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 45);
    assert_eq!(watchlist_write_routes(), &WATCHLIST_WRITE_ROUTES);

    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        assert!(
            !case.expected_observation.is_null(),
            "case {} lacks rollback/state observation",
            case.name
        );
        let port = FixturePort::from_case(case);
        let port_ref = if case.port_mode == "no-port" {
            None
        } else {
            Some(&port as &dyn WatchlistWritePort)
        };

        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let _ = &request.context;
            let response = dispatch_watchlist_write(
                &WatchlistWriteRequest {
                    method: request.method.clone(),
                    path: request.path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                port_ref,
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }

        let calls = port.calls.lock().expect("watchlist write calls lock");
        assert_eq!(
            calls.as_slice(),
            case.calls.as_slice(),
            "case {} calls",
            case.name
        );
        assert_eq!(
            calls.len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port-call count",
            case.name
        );
        drop(calls);
        port.assert_drained(&case.name);
    }
}

#[test]
fn watchlist_write_fixture_covers_concurrency_and_every_route() {
    let fixture = fixture();
    assert!(
        fixture.cases.iter().any(|case| case.concurrent),
        "revision-fence fixture case is missing"
    );
    let covered = fixture
        .cases
        .iter()
        .flat_map(|case| {
            case.requests
                .iter()
                .map(|request| (request.method.as_str(), request.path.as_str()))
        })
        .collect::<Vec<_>>();
    for (method, path) in WATCHLIST_WRITE_ROUTES {
        let found = covered.iter().any(|(covered_method, covered_path)| {
            *covered_method == method && route_template_matches(covered_path, path)
        });
        assert!(found, "fixture does not cover {method} {path}");
    }
}

#[test]
fn sqlite_test_cutover_preserves_revision_fencing_import_commit_and_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let database_path = directory.path().join("watchlist-test-cutover.db");
    seed_go_watchlist_schema(&database_path);
    let port =
        Arc::new(WatchlistSqliteTestCutoverPort::open(&database_path).expect("open adapter"));
    let create = WatchlistWriteMutation {
        value: json!({"route":"create-group","name":"Value"}),
    };
    assert_eq!(port.mutate(&create).expect("create")["revision"], 1);
    let update = WatchlistWriteMutation {
        value: json!({"route":"update-group","groupId":"group-1","name":"Growth 2","expectedRevision":1}),
    };
    let results = (0..2)
        .map(|_| {
            let port = Arc::clone(&port);
            let update = update.clone();
            std::thread::spawn(move || port.mutate(&update))
        })
        .collect::<Vec<_>>();
    let mut successes = 0;
    let mut conflicts = 0;
    for result in results {
        match result.join().expect("join update") {
            Ok(value) => {
                successes += 1;
                assert_eq!(value["revision"], 2);
            }
            Err(error) => {
                conflicts += 1;
                assert_eq!(error.status, 409);
            }
        }
    }
    assert_eq!((successes, conflicts), (1, 1));
    let group = port.store().get_group("group-1").expect("load group");
    assert_eq!(group.revision, 2);

    let preview = WatchlistWriteMutation {
        value: json!({"route":"preview-import","sourceId":"source-1","remoteGroupId":"remote-1"}),
    };
    let preview_id = port.mutate(&preview).expect("preview")["id"]
        .as_str()
        .expect("id")
        .to_owned();
    let commit = WatchlistWriteMutation {
        value: json!({"route":"commit-import","previewId":preview_id}),
    };
    assert_eq!(port.mutate(&commit).expect("commit")["status"], "completed");
    assert_eq!(
        port.mutate(&commit).expect_err("repeat fencing").status,
        409
    );
    drop(port);
    let reopened = WatchlistSqliteTestCutoverPort::open(&database_path).expect("reopen adapter");
    let group_after_restart = reopened
        .store()
        .get_group("group-1")
        .expect("reopened group");
    assert_eq!(group_after_restart.revision, 2);
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

fn route_template_matches(path: &str, template: &str) -> bool {
    let path = path.split('?').next().unwrap_or(path);
    let path_parts = path.split('/').collect::<Vec<_>>();
    let template_parts = template.split('/').collect::<Vec<_>>();
    path_parts.len() == template_parts.len()
        && path_parts
            .iter()
            .zip(template_parts)
            .all(|(actual, expected)| expected.starts_with('{') || actual == &expected)
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/watchlist-write.json"
    ))
    .expect("watchlist-write fixture")
}

fn error_from_envelope(expected: &FixtureExpected) -> WatchlistWritePortError {
    WatchlistWritePortError {
        status: expected.status,
        code: expected.envelope["error"]["code"]
            .as_str()
            .unwrap_or("WATCHLIST_FAILED")
            .to_owned(),
        message: expected.envelope["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
    }
}
