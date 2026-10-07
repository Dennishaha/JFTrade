use super::*;

#[derive(Clone, Debug)]
pub struct AdkStreamCursor {
    pub run_id: String,
    pub stream_id: String,
    pub replay_until: u64,
}

#[derive(Debug)]
pub struct AdkStreamPage {
    pub status: String,
    pub events: Vec<(u64, Value)>,
}

impl AdkStore {
    /// Resolve the reconnect identity and watermark without returning run payloads.
    pub fn open_stream_cursor(&self, id: &str) -> Result<Option<AdkStreamCursor>, AdkStoreError> {
        let connection = self.lock_connection()?;
        connection
            .query_row(
                "SELECT r.id, COALESCE(json_extract(r.payload_json, '$.streamId'), r.id),
                COALESCE((SELECT MAX(CASE WHEN json_type(e.value, '$.sequence') = 'integer'
                    AND json_extract(e.value, '$.sequence') >= 0
                    THEN json_extract(e.value, '$.sequence') ELSE CAST(e.key AS INTEGER) + 1 END)
                    FROM json_each(r.payload_json, '$.streamEvents') e), 0)
             FROM adk_runs r WHERE (r.id = ?1 OR json_extract(r.payload_json, '$.streamId') = ?1)
                AND json_type(r.payload_json, '$.streamEvents') = 'array'
             ORDER BY (r.id = ?1) DESC, r.created_at DESC LIMIT 1",
                params![id],
                |row| {
                    Ok(AdkStreamCursor {
                        run_id: row.get(0)?,
                        stream_id: row.get(1)?,
                        replay_until: row.get::<_, i64>(2)? as u64,
                    })
                },
            )
            .optional()
            .map_err(AdkStoreError::Query)
    }

    /// Decode at most 64 event values in Rust. SQLite still scans the embedded
    /// JSON array; this does not claim indexed event reads or constant SQL work.
    pub fn read_stream_page(
        &self,
        run_id: &str,
        after: u64,
        limit: usize,
    ) -> Result<AdkStreamPage, AdkStoreError> {
        let connection = self.lock_connection()?;
        let status = connection
            .query_row(
                "SELECT status FROM adk_runs WHERE id = ?1",
                params![run_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(AdkStoreError::Query)?
            .ok_or_else(|| AdkStoreError::NotFound(run_id.to_owned()))?;
        let mut statement = connection.prepare(
            "SELECT sequence, value FROM (
                SELECT CASE WHEN json_type(e.value, '$.sequence') = 'integer'
                    AND json_extract(e.value, '$.sequence') >= 0
                    THEN json_extract(e.value, '$.sequence') ELSE CAST(e.key AS INTEGER) + 1 END AS sequence,
                    e.value AS value
                FROM adk_runs r, json_each(r.payload_json, '$.streamEvents') e WHERE r.id = ?1
             ) WHERE sequence > ?2 ORDER BY sequence LIMIT ?3"
        ).map_err(AdkStoreError::Query)?;
        let rows = statement
            .query_map(
                params![
                    run_id,
                    i64::try_from(after).unwrap_or(i64::MAX),
                    limit.clamp(1, 64) as i64
                ],
                |row| Ok((row.get::<_, i64>(0)? as u64, row.get::<_, String>(1)?)),
            )
            .map_err(AdkStoreError::Query)?;
        let events = rows
            .map(|row| {
                let (sequence, value) = row.map_err(AdkStoreError::Query)?;
                let value = serde_json::from_str(&value)
                    .map_err(|error| AdkStoreError::Validation(error.to_string()))?;
                Ok((sequence, value))
            })
            .collect::<Result<Vec<_>, AdkStoreError>>()?;
        Ok(AdkStreamPage { status, events })
    }
}
