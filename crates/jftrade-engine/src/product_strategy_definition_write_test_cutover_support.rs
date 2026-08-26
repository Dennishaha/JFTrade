fn allocate_id(
    transaction: &Transaction<'_>,
    prefix: &str,
) -> Result<String, StrategyDefinitionWritePortError> {
    let value = transaction
        .query_row(
            "SELECT next_value FROM strategy_definition_test_cutover_ids
             WHERE singleton = 1",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| strategy_write_failure("strategy definition id allocation failed"))?;
    let next_value = value
        .checked_add(1)
        .ok_or_else(|| strategy_write_failure("strategy definition id allocation failed"))?;
    transaction
        .execute(
            "UPDATE strategy_definition_test_cutover_ids
             SET next_value = ?1 WHERE singleton = 1",
            rusqlite::params![next_value],
        )
        .map_err(|_| strategy_write_failure("strategy definition id allocation failed"))?;
    let value = u64::try_from(value)
        .map_err(|_| strategy_write_failure("strategy definition id allocation failed"))?;
    Ok(format!("{prefix}-test-{value}"))
}

fn reconcile_linked_instances(
    transaction: &Transaction<'_>,
    definition_id: &str,
    linked_ids: &[String],
) -> Result<(), String> {
    let existing = instance_ids_in_transaction(transaction, definition_id)?;
    for instance_id in &existing {
        if !linked_ids.iter().any(|linked_id| linked_id == instance_id) {
            transaction
                .execute(
                    "DELETE FROM strategy_definition_test_cutover_instances
                     WHERE id = ?1",
                    rusqlite::params![instance_id],
                )
                .map_err(|error| error.to_string())?;
        }
    }
    let version: String = transaction
        .query_row(
            "SELECT version FROM strategy_definition_test_cutover WHERE id = ?1",
            rusqlite::params![definition_id],
            |row| row.get(0),
        )
        .map_err(|error| error.to_string())?;
    for instance_id in linked_ids {
        if existing
            .iter()
            .any(|existing_id| existing_id == instance_id)
        {
            continue;
        }
        transaction
            .execute(
                "INSERT OR IGNORE INTO strategy_definition_test_cutover_instances
                    (id, definition_id, definition_version, payload, binding, status)
                 VALUES (?1, ?2, ?3, '{}', '{}', 'STOPPED')",
                rusqlite::params![instance_id, definition_id, &version],
            )
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

fn instance_ids_in_transaction(
    transaction: &Transaction<'_>,
    definition_id: &str,
) -> Result<Vec<String>, String> {
    let mut statement = transaction
        .prepare(
            "SELECT id FROM strategy_definition_test_cutover_instances
             WHERE definition_id = ?1 ORDER BY rowid",
        )
        .map_err(|error| error.to_string())?;
    statement
        .query_map(rusqlite::params![definition_id], |row| row.get(0))
        .map_err(|error| error.to_string())?
        .collect::<Result<Vec<String>, _>>()
        .map_err(|error| error.to_string())
}

fn linked_ids_for(
    transaction: &Transaction<'_>,
    definition_id: &str,
    stored_linked_ids: &str,
) -> Result<Vec<String>, StrategyDefinitionWritePortError> {
    let mut linked_ids: Vec<String> = serde_json::from_str(stored_linked_ids)
        .map_err(|_| strategy_write_failure("failed to load linked strategy instances"))?;
    for instance_id in instance_ids_in_transaction(transaction, definition_id)
        .map_err(|_| strategy_write_failure("failed to load linked strategy instances"))?
    {
        if !linked_ids.iter().any(|linked_id| linked_id == &instance_id) {
            linked_ids.push(instance_id);
        }
    }
    Ok(linked_ids)
}

fn load_instances(
    transaction: &Transaction<'_>,
    definition_id: &str,
) -> Result<Vec<(String, String, String)>, StrategyDefinitionWritePortError> {
    let mut statement = transaction
        .prepare(
            "SELECT id, definition_version, status
             FROM strategy_definition_test_cutover_instances
             WHERE definition_id = ?1 ORDER BY rowid",
        )
        .map_err(|_| strategy_write_failure("failed to load linked strategy instances"))?;
    statement
        .query_map(rusqlite::params![definition_id], |row| {
            Ok((row.get(0)?, row.get(1)?, row.get(2)?))
        })
        .map_err(|_| strategy_write_failure("failed to load linked strategy instances"))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|_| strategy_write_failure("failed to load linked strategy instances"))
}

fn acquire_writer_lease(path: &Path) -> Result<File, String> {
    let mut lock_path = path.as_os_str().to_owned();
    lock_path.push(".jftrade-owner.lock");
    let lock_path = std::path::PathBuf::from(lock_path);
    let mut file = OpenOptions::new()
        .create(true)
        .read(true)
        .write(true)
        .truncate(false)
        .open(&lock_path)
        .map_err(|error| format!("open writer lease {}: {error}", lock_path.display()))?;
    match file.try_lock() {
        Ok(()) => {}
        Err(std::fs::TryLockError::WouldBlock) => {
            return Err(format!(
                "writer lease is already held for {}",
                lock_path.display()
            ));
        }
        Err(std::fs::TryLockError::Error(error)) => {
            return Err(format!("lock writer lease {}: {error}", lock_path.display()));
        }
    }
    file.set_len(0)
        .and_then(|()| file.seek(SeekFrom::Start(0)).map(|_| ()))
        .and_then(|()| {
            writeln!(
                file,
                "{{\"owner\":\"rust\",\"profile\":\"{TEST_CUTOVER_PROFILE}\"}}"
            )
        })
        .map_err(|error| format!("write writer lease {}: {error}", lock_path.display()))?;
    Ok(file)
}
