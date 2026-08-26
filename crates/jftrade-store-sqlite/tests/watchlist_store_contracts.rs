use std::path::Path;
use std::sync::{Arc, Barrier};

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    WATCHLIST_TEST_CUTOVER_PROFILE, WatchlistStoreError, WatchlistTestCutoverStore,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-24T04:00:00Z";
const TIMESTAMP_2: &str = "2026-08-24T04:01:00Z";
const TIMESTAMP_3: &str = "2026-08-24T04:02:00Z";

#[test]
fn watchlist_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing_path = directory.path().join("missing-watchlist.db");
    assert!(matches!(
        WatchlistTestCutoverStore::open_existing(&missing_path, WATCHLIST_TEST_CUTOVER_PROFILE),
        Err(WatchlistStoreError::NotRegularFile(_))
    ));

    let drifted_path = directory.path().join("drifted-watchlist.db");
    let connection = Connection::open(&drifted_path).expect("create drifted db");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('watchlist', 1, '2026-08-24T04:00:00Z');
            CREATE TABLE watchlist_groups (
                group_id TEXT PRIMARY KEY,
                rogue_column TEXT NOT NULL
            );",
        )
        .expect("seed rogue table");
    drop(connection);

    let error =
        WatchlistTestCutoverStore::open_existing(&drifted_path, WATCHLIST_TEST_CUTOVER_PROFILE)
            .expect_err("drifted schema must fail");
    assert!(matches!(error, WatchlistStoreError::Schema(_)));
}

#[test]
fn watchlist_group_mutations_are_revision_fenced_and_survive_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("watchlist.db");
    seed_go_watchlist_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);

    let conflict = WatchlistTestCutoverStore::open_existing(&path, WATCHLIST_TEST_CUTOVER_PROFILE)
        .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        WatchlistStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    let initial_groups = store.list_groups().expect("list initial groups");
    assert_eq!(initial_groups.len(), 1);
    assert_eq!(initial_groups[0].name, "自选股");
    assert!(initial_groups[0].is_default);
    assert!(initial_groups[0].protected);

    let created = store
        .create_group("Tech Growth", TIMESTAMP_1)
        .expect("create group");
    assert_eq!(created.name, "Tech Growth");
    assert_eq!(created.revision, 1);
    assert!(!created.protected);
    assert!(!created.is_default);

    let duplicate = store.create_group(" tech growth ", TIMESTAMP_1);
    assert!(matches!(duplicate, Err(WatchlistStoreError::Conflict)));

    let updated = store
        .update_group(&created.group_id, "Tech Growth High", 1, TIMESTAMP_2)
        .expect("update group");
    assert_eq!(updated.name, "Tech Growth High");
    assert_eq!(updated.revision, 2);

    let stale_update = store.update_group(&created.group_id, "Tech Growth Stale", 1, TIMESTAMP_2);
    assert!(matches!(stale_update, Err(WatchlistStoreError::Conflict)));

    let default_group_update = store.update_group("default", "Renamed Default", 1, TIMESTAMP_2);
    assert!(matches!(
        default_group_update,
        Err(WatchlistStoreError::ProtectedGroup)
    ));

    let default_group_delete = store.delete_group("default");
    assert!(matches!(
        default_group_delete,
        Err(WatchlistStoreError::ProtectedGroup)
    ));

    drop(store);

    let reopened = open_store(&path);
    let groups = reopened.list_groups().expect("list after restart");
    assert_eq!(groups.len(), 2);
    assert_eq!(groups[0].group_id, "default");
    assert_eq!(groups[1].group_id, created.group_id);
    assert_eq!(groups[1].name, "Tech Growth High");
    assert_eq!(groups[1].revision, 2);

    reopened
        .delete_group(&created.group_id)
        .expect("delete group");
    let after_delete = reopened.list_groups().expect("list after delete");
    assert_eq!(after_delete.len(), 1);
    assert_eq!(after_delete[0].group_id, "default");
}

#[test]
fn watchlist_membership_mutations_and_preview_commit_lifecycle() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("watchlist.db");
    seed_go_watchlist_schema(&path);

    let store = open_store(&path);

    let g1 = store
        .create_group("Sector A", TIMESTAMP_1)
        .expect("create g1");
    let g2 = store
        .create_group("Sector B", TIMESTAMP_1)
        .expect("create g2");

    let memberships = store
        .replace_memberships(
            "US.AAPL",
            std::slice::from_ref(&g1.group_id),
            &["Auto Created Sector".to_owned()],
            0,
            TIMESTAMP_1,
        )
        .expect("replace memberships 1");
    assert_eq!(memberships.instrument_id, "US.AAPL");
    assert_eq!(memberships.revision, 1);
    assert_eq!(memberships.groups.len(), 2);

    let read_memberships = store.get_memberships("us.aapl").expect("get memberships");
    assert_eq!(read_memberships.instrument_id, "US.AAPL");
    assert_eq!(read_memberships.revision, 1);
    assert_eq!(read_memberships.groups.len(), 2);

    let memberships_rev2 = store
        .replace_memberships(
            "US.AAPL",
            std::slice::from_ref(&g2.group_id),
            &[],
            1,
            TIMESTAMP_2,
        )
        .expect("replace memberships 2");
    assert_eq!(memberships_rev2.revision, 2);
    assert_eq!(memberships_rev2.groups.len(), 1);
    assert_eq!(memberships_rev2.groups[0].group_id, g2.group_id);

    let conflict_rev = store.replace_memberships(
        "US.AAPL",
        std::slice::from_ref(&g1.group_id),
        &[],
        1,
        TIMESTAMP_3,
    );
    assert!(matches!(conflict_rev, Err(WatchlistStoreError::Conflict)));

    let preview = store
        .create_import_preview(
            "futu:default",
            "remote_grp_1",
            Some(&g1.group_id),
            None,
            TIMESTAMP_1,
        )
        .expect("create preview");
    assert_eq!(preview.source_id, "futu:default");
    assert_eq!(preview.status, "pending");

    let run = store
        .commit_import_preview(&preview.preview_id, &[], TIMESTAMP_2)
        .expect("commit preview");
    assert_eq!(run.status, "completed");
    assert_eq!(run.preview_id, preview.preview_id);

    let commit_again = store.commit_import_preview(&preview.preview_id, &[], TIMESTAMP_3);
    assert!(matches!(commit_again, Err(WatchlistStoreError::Conflict)));
}

#[test]
fn concurrent_group_updates_commit_with_revision_fence() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("watchlist.db");
    seed_go_watchlist_schema(&path);

    let store = Arc::new(open_store(&path));
    let group = store
        .create_group("Race Base", TIMESTAMP_1)
        .expect("create group");

    let barrier = Arc::new(Barrier::new(3));
    let handles = ["First Name", "Second Name"].map(|name| {
        let store = Arc::clone(&store);
        let barrier = Arc::clone(&barrier);
        let gid = group.group_id.clone();
        std::thread::spawn(move || {
            barrier.wait();
            store.update_group(&gid, name, 1, TIMESTAMP_2)
        })
    });

    barrier.wait();
    let results: Vec<_> = handles.into_iter().map(|h| h.join().unwrap()).collect();

    let success_count = results.iter().filter(|r| r.is_ok()).count();
    let conflict_count = results
        .iter()
        .filter(|r| matches!(r, Err(WatchlistStoreError::Conflict)))
        .count();

    assert_eq!(success_count, 1, "exactly one update must succeed");
    assert_eq!(conflict_count, 1, "competing update must see Conflict");
}

fn open_store(path: &Path) -> WatchlistTestCutoverStore {
    WatchlistTestCutoverStore::open_existing(path, WATCHLIST_TEST_CUTOVER_PROFILE)
        .expect("open watchlist test-cutover store")
}

fn seed_go_watchlist_schema(path: &Path) {
    let connection = Connection::open(path).expect("create watchlist fixture");
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
                VALUES ('default', '自选股', '自选股', 1, 1, 1, '2026-08-24T04:00:00Z', '2026-08-24T04:00:00Z');",
        )
        .expect("seed Go-compatible watchlist schema");
}
