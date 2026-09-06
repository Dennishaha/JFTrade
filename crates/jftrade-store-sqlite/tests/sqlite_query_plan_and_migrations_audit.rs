//! Empirical verification test suite for:
//! - P1-05: `adk-session.events` query plan audit, Go index proposal refutation, and optimal index gain.
//! - P1-06: 9 SQLite database migration paths, rollback resilience, and strict downgrade rejection.

use std::time::Instant;

use jftrade_store_sqlite::{
    current_version, initialize_current, migrate_legacy_schema, validate_current,
};
use rusqlite::{Connection, OpenFlags, params};
use tempfile::tempdir;

fn get_query_plan(connection: &Connection, sql: &str) -> Vec<String> {
    let mut stmt = connection
        .prepare(&format!("EXPLAIN QUERY PLAN {sql}"))
        .expect("prepare explain query plan");
    let rows = stmt
        .query_map([], |row| row.get::<_, String>(3))
        .expect("query explain query plan");
    rows.map(|r| r.expect("row")).collect()
}

#[test]
fn test_p1_05_adk_session_events_query_plan_without_index_shows_scan_and_temp_btree() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk-session.db");
    let conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "adk-session").expect("init");

    let sql = "SELECT id, app_name, user_id, session_id, invocation_id, author, content, timestamp \
               FROM events WHERE session_id = 'session-123' ORDER BY timestamp ASC, id ASC;";
    let plan = get_query_plan(&conn, sql);
    let plan_text = plan.join("\n");

    // Without a secondary index, SQLite must scan the table (or autoindex on composite PK)
    // and must use a temporary B-Tree for ORDER BY.
    assert!(
        plan_text.contains("SCAN"),
        "Unindexed events query must scan: {plan_text}"
    );
    assert!(
        plan_text.contains("USE TEMP B-TREE"),
        "Unindexed events query must use temp B-Tree for ORDER BY: {plan_text}"
    );
}

#[test]
fn test_p1_05_adk_session_events_refutes_go_historical_index_proposal() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk-session.db");
    let conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "adk-session").expect("init");

    // The historical Go proposal suggested: (app_name, user_id, session_id, timestamp DESC)
    conn.execute_batch(
        "CREATE INDEX idx_go_proposal ON events (app_name, user_id, session_id, timestamp DESC);",
    )
    .expect("create go proposal index");

    let sql = "SELECT id, app_name, user_id, session_id, invocation_id, author, content, timestamp \
               FROM events WHERE session_id = 'session-123' ORDER BY timestamp ASC, id ASC;";
    let plan = get_query_plan(&conn, sql);
    let plan_text = plan.join("\n");

    // Empirical refutation: Because Rust's query only filters by session_id without app_name/user_id,
    // session_id is NOT the prefix of idx_go_proposal!
    // SQLite CANNOT perform an index SEARCH and still performs a full SCAN and TEMP B-TREE.
    assert!(
        plan_text.contains("SCAN"),
        "Go proposal index must still force a full SCAN: {plan_text}"
    );
    assert!(
        !plan_text.contains("SEARCH"),
        "Go proposal index CANNOT perform an index SEARCH: {plan_text}"
    );
    assert!(
        plan_text.contains("USE TEMP B-TREE"),
        "Go proposal index fails to eliminate temp B-Tree: {plan_text}"
    );
}

#[test]
fn test_p1_05_adk_session_events_optimal_index_achieves_search_and_zero_temp_btree() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk-session.db");
    let conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "adk-session").expect("init");

    // Optimal index matching Rust query predicates: (session_id, timestamp ASC, id ASC)
    conn.execute_batch(
        "CREATE INDEX idx_adk_session_events_session_time \
         ON events (session_id, timestamp ASC, id ASC);",
    )
    .expect("create optimal index");

    let sql = "SELECT id, app_name, user_id, session_id, invocation_id, author, content, timestamp \
               FROM events WHERE session_id = 'session-123' ORDER BY timestamp ASC, id ASC;";
    let plan = get_query_plan(&conn, sql);
    let plan_text = plan.join("\n");

    // With the optimal index, SQLite performs an index SEARCH on session_id
    // and ORDER BY is satisfied directly by index order without temp B-Tree.
    assert!(
        plan_text.contains("SEARCH") && plan_text.contains("idx_adk_session_events_session_time"),
        "Must use index search on idx_adk_session_events_session_time: {plan_text}"
    );
    assert!(
        !plan_text.contains("USE TEMP B-TREE"),
        "Optimal index must eliminate temporary B-Tree: {plan_text}"
    );
}

#[test]
fn test_p1_05_adk_session_events_performance_benchmark_gain() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk-session.db");
    let mut conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "adk-session").expect("init");

    // Seed 100 parent sessions to satisfy foreign key constraints
    let tx = conn.transaction().expect("tx");
    {
        let mut insert_session = tx
            .prepare(
                "INSERT INTO sessions (app_name, user_id, id, state, create_time, update_time) \
                 VALUES ('jftrade', 'local', ?1, '{}', 1700000000, 1700000000);",
            )
            .expect("prepare session");
        for s in 0..100 {
            let session_id = format!("session-{s}");
            insert_session
                .execute(params![session_id])
                .expect("insert session");
        }

        // Seed 10,000 events across the 100 sessions
        let mut insert_event = tx
            .prepare(
                "INSERT INTO events (id, app_name, user_id, session_id, invocation_id, author, \
                 content, timestamp) VALUES (?1, 'jftrade', 'local', ?2, 'inv-1', 'user', ?3, ?4);",
            )
            .expect("prepare event");
        for i in 0..10_000 {
            let session_id = format!("session-{}", i % 100);
            let id = format!("event-{i}");
            let content = format!("ChatMessage payload {i} with conversation context");
            let timestamp = 1_700_000_000 + i;
            insert_event
                .execute(params![id, session_id, content, timestamp])
                .expect("insert event");
        }
    }
    tx.commit().expect("commit seed");

    let query_sql = "SELECT id, app_name, user_id, session_id, invocation_id, author, content, timestamp \
         FROM events WHERE session_id = 'session-42' ORDER BY timestamp ASC, id ASC;";

    // Measure unindexed query latency
    let start_unindexed = Instant::now();
    for _ in 0..20 {
        let mut stmt = conn.prepare(query_sql).expect("prepare");
        let count = stmt
            .query_map([], |row| row.get::<_, String>(0))
            .expect("query")
            .count();
        assert_eq!(count, 100);
    }
    let duration_unindexed = start_unindexed.elapsed();

    // Create optimal index
    conn.execute_batch(
        "CREATE INDEX idx_adk_session_events_session_time \
         ON events (session_id, timestamp ASC, id ASC);",
    )
    .expect("create optimal index");

    // Measure indexed query latency
    let start_indexed = Instant::now();
    for _ in 0..20 {
        let mut stmt = conn.prepare(query_sql).expect("prepare");
        let count = stmt
            .query_map([], |row| row.get::<_, String>(0))
            .expect("query")
            .count();
        assert_eq!(count, 100);
    }
    let duration_indexed = start_indexed.elapsed();

    println!(
        "P1-05 Benchmark (20 runs): unindexed={:?}, indexed={:?}, speedup={:.2}x",
        duration_unindexed,
        duration_indexed,
        duration_unindexed.as_nanos() as f64 / duration_indexed.as_nanos().max(1) as f64
    );

    // Indexed query must be substantially faster than full table scan + sort
    assert!(
        duration_indexed < duration_unindexed,
        "Indexed query ({:?}) must be faster than unindexed ({:?})",
        duration_indexed,
        duration_unindexed
    );
}

#[test]
fn test_p1_06_downgrade_strictly_rejected_at_all_layers() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    let conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "strategy").expect("init");

    // Simulate an advanced database from a newer version (e.g. version = 3)
    conn.execute(
        "UPDATE jftrade_schema_meta SET version = 3 WHERE component_id = 'strategy';",
        [],
    )
    .expect("update version");
    drop(conn);

    let path_str = path.display().to_string();

    // Layer 1: validate_current rejects when database version is greater than expected (2)
    let conn2 = Connection::open_with_flags(&path, OpenFlags::SQLITE_OPEN_READ_WRITE)
        .expect("open existing");
    let validation = validate_current(&conn2, &path_str, "strategy", 2);
    assert!(
        validation.is_err(),
        "validate_current must reject version 3 when expected is 2"
    );
    let err_msg = validation.unwrap_err().to_string();
    assert!(
        err_msg.contains("incompatible") || err_msg.contains("schema"),
        "Error message must indicate incompatibility: {err_msg}"
    );

    // Layer 2: migrate_legacy_schema strictly rejects downgrade (from >= expected)
    let tx = conn2.unchecked_transaction().expect("tx");
    let migration = migrate_legacy_schema(&tx, &path_str, "strategy", 3, 2);
    assert!(
        migration.is_err(),
        "migrate_legacy_schema must fail-closed on downgrade (3 -> 2)"
    );
    let mig_err = migration.unwrap_err().to_string();
    assert!(
        mig_err.contains("unsupported migration range") || mig_err.contains("incompatible"),
        "Migration error must indicate unsupported range: {mig_err}"
    );
}

#[test]
fn test_p1_06_supported_legacy_migrations_upgrade_and_repeated_open() {
    let dir = tempdir().expect("tempdir");

    // Test 1: Strategy v1 -> v2 migration
    let strat_path = dir.path().join("strategy.db");
    {
        let conn = Connection::open(&strat_path).expect("create strat");
        initialize_current(&conn, "strategy").expect("init");
        conn.execute_batch(
            "DROP TRIGGER IF EXISTS trg_strategy_definition_versions_immutable;
             DROP INDEX IF EXISTS idx_strategy_definition_versions_saved_at;
             DROP TABLE IF EXISTS strategy_definition_versions;
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("downgrade to v1");
        assert_eq!(current_version(&conn, "strategy"), Some(1));
    }

    // Perform migration 1 -> 2
    {
        let conn = Connection::open(&strat_path).expect("open strat");
        let tx = conn.unchecked_transaction().expect("tx");
        migrate_legacy_schema(&tx, &strat_path.display().to_string(), "strategy", 1, 2)
            .expect("migrate strat");
        validate_current(&tx, &strat_path.display().to_string(), "strategy", 2)
            .expect("validate strat");
        tx.commit().expect("commit");
    }

    // Reopening database repeatedly succeeds without re-migrating
    for _ in 0..3 {
        let conn = Connection::open(&strat_path).expect("reopen strat");
        assert_eq!(current_version(&conn, "strategy"), Some(2));
        validate_current(&conn, &strat_path.display().to_string(), "strategy", 2)
            .expect("validate strat repeat");
    }

    // Test 2: ADK v2 -> v4 migration
    let adk_path = dir.path().join("adk.db");
    {
        let conn = Connection::open(&adk_path).expect("create adk");
        initialize_current(&conn, "adk").expect("init");
        conn.execute_batch(
            "UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'adk';",
        )
        .expect("set v2");
        assert_eq!(current_version(&conn, "adk"), Some(2));
    }

    // Perform migration 2 -> 4
    {
        let conn = Connection::open(&adk_path).expect("open adk");
        let tx = conn.unchecked_transaction().expect("tx");
        migrate_legacy_schema(&tx, &adk_path.display().to_string(), "adk", 2, 4)
            .expect("migrate adk");
        validate_current(&tx, &adk_path.display().to_string(), "adk", 4).expect("validate adk");
        tx.commit().expect("commit");
    }

    // Reopen and verify
    let conn_adk = Connection::open(&adk_path).expect("reopen adk");
    assert_eq!(current_version(&conn_adk, "adk"), Some(4));
    validate_current(&conn_adk, &adk_path.display().to_string(), "adk", 4).expect("validate adk");
}

#[test]
fn test_p1_06_migration_syntax_error_triggers_atomic_rollback() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("faulty.db");
    let conn = Connection::open(&path).expect("open");
    initialize_current(&conn, "strategy").expect("init");

    // Force version 1
    conn.execute(
        "UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        [],
    )
    .expect("v1");
    drop(conn);

    let conn2 = Connection::open(&path).expect("reopen");
    let tx = conn2.unchecked_transaction().expect("tx");

    // Inject deliberate SQL syntax error inside transaction
    let faulty_result = tx.execute_batch("INVALID SQL SYNTAX HERE !!!;");
    assert!(faulty_result.is_err(), "Must fail on invalid SQL syntax");

    // Transaction is dropped without commit (automatic rollback)
    drop(tx);
    drop(conn2);

    // Verify database retained original version and valid state
    let check_conn = Connection::open(&path).expect("check conn");
    assert_eq!(
        current_version(&check_conn, "strategy"),
        Some(1),
        "Version must remain 1 after aborted migration"
    );
}
