use rusqlite::{OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::execution_order::{ExecutionOrderStore, ExecutionOrderStoreError};

/// Durable credential issued by an execution preview.  A preview is bound to
/// one broker/account/request hash and can be consumed at most once.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredExecutionOrderPreview {
    pub preview_id: String,
    pub request_hash: String,
    pub broker_id: String,
    pub capability_version: String,
    pub account_id: String,
    pub expires_at: String,
    pub quote_expires_at: Option<String>,
    pub rfq_id: Option<String>,
    pub normalized_request: String,
    pub created_at: String,
    pub consumed_at: Option<String>,
}

impl ExecutionOrderStore {
    pub fn save_preview(
        &self,
        preview: &StoredExecutionOrderPreview,
    ) -> Result<(), ExecutionOrderStoreError> {
        validate_preview(preview)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let existing = transaction
            .query_row(
                "SELECT request_hash, broker_id, capability_version, account_id,
                        expires_at, quote_expires_at, rfq_id, normalized_request,
                        created_at, consumed_at
                 FROM execution_order_previews WHERE preview_id = ?1",
                params![preview.preview_id],
                |row| {
                    Ok(StoredExecutionOrderPreview {
                        preview_id: preview.preview_id.clone(),
                        request_hash: row.get(0)?,
                        broker_id: row.get(1)?,
                        capability_version: row.get(2)?,
                        account_id: row.get(3)?,
                        expires_at: row.get(4)?,
                        quote_expires_at: row.get(5)?,
                        rfq_id: row.get(6)?,
                        normalized_request: row.get(7)?,
                        created_at: row.get(8)?,
                        consumed_at: row.get(9)?,
                    })
                },
            )
            .optional()
            .map_err(ExecutionOrderStoreError::Query)?;
        if let Some(existing) = existing {
            if existing.request_hash != preview.request_hash
                || existing.broker_id != preview.broker_id
                || existing.account_id != preview.account_id
            {
                return Err(ExecutionOrderStoreError::Validation(
                    "preview id is already bound to a different request".to_owned(),
                ));
            }
        } else {
            transaction
                .execute(
                    "INSERT INTO execution_order_previews (
                        preview_id, request_hash, broker_id, capability_version,
                        account_id, expires_at, quote_expires_at, rfq_id,
                        normalized_request, created_at, consumed_at
                     ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, NULL)",
                    params![
                        preview.preview_id,
                        preview.request_hash,
                        preview.broker_id,
                        preview.capability_version,
                        preview.account_id,
                        preview.expires_at,
                        preview.quote_expires_at,
                        preview.rfq_id,
                        preview.normalized_request,
                        preview.created_at,
                    ],
                )
                .map_err(ExecutionOrderStoreError::Query)?;
        }
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)
    }

    pub fn consume_preview(
        &self,
        preview_id: &str,
        broker_id: &str,
        account_id: &str,
        request_hash: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionOrderStoreError> {
        validate_timestamp(timestamp)?;
        let now = OffsetDateTime::parse(timestamp, &Rfc3339).map_err(|error| {
            ExecutionOrderStoreError::Validation(format!("invalid preview timestamp: {error}"))
        })?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        consume_preview_in_transaction(
            &transaction,
            preview_id,
            broker_id,
            account_id,
            request_hash,
            timestamp,
            now,
            None,
        )?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)
    }
}

/// Validate and consume a preview using a caller-owned transaction.  Keeping
/// the read/expiry checks and the conditional consumed_at update together lets
/// execution-order reservation atomically bind the preview credential to the
/// durable client-order identity fence.
#[allow(clippy::too_many_arguments)]
pub(super) fn consume_preview_in_transaction(
    transaction: &rusqlite::Transaction<'_>,
    preview_id: &str,
    broker_id: &str,
    account_id: &str,
    request_hash: &str,
    timestamp: &str,
    now: OffsetDateTime,
    expected_capability_version: Option<&str>,
) -> Result<(), ExecutionOrderStoreError> {
    let row = transaction
        .query_row(
            "SELECT request_hash, broker_id, capability_version, account_id,
                    expires_at, quote_expires_at, consumed_at
             FROM execution_order_previews WHERE preview_id = ?1",
            params![preview_id],
            |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, Option<String>>(5)?,
                    row.get::<_, Option<String>>(6)?,
                ))
            },
        )
        .optional()
        .map_err(ExecutionOrderStoreError::Query)?
        .ok_or_else(|| ExecutionOrderStoreError::NotFound(preview_id.to_owned()))?;
    if !row.1.eq_ignore_ascii_case(broker_id) || row.3 != account_id {
        return Err(ExecutionOrderStoreError::Validation(
            "execution preview does not match the order request".to_owned(),
        ));
    }
    if row.0 != request_hash {
        return Err(ExecutionOrderStoreError::Validation(
            "execution preview does not match the order request".to_owned(),
        ));
    }
    if let Some(expected) = expected_capability_version
        && (expected.trim().is_empty() || row.2.trim() != expected.trim())
    {
        return Err(ExecutionOrderStoreError::Validation(
            "execution preview capability version changed".to_owned(),
        ));
    }
    if row.6.is_some() {
        // Identical replays are safe: the durable order identity fence
        // returns the original submission without touching OpenD.
        return Ok(());
    }
    let expires_at = OffsetDateTime::parse(&row.4, &Rfc3339).map_err(|error| {
        ExecutionOrderStoreError::Validation(format!("stored preview expiry is invalid: {error}"))
    })?;
    if expires_at <= now {
        return Err(ExecutionOrderStoreError::Validation(
            "execution preview has expired".to_owned(),
        ));
    }
    if let Some(quote_expires_at) = row.5.as_deref() {
        let quote_expires_at =
            OffsetDateTime::parse(quote_expires_at, &Rfc3339).map_err(|error| {
                ExecutionOrderStoreError::Validation(format!(
                    "stored quote expiry is invalid: {error}"
                ))
            })?;
        if quote_expires_at <= now {
            return Err(ExecutionOrderStoreError::Validation(
                "broker quote expired; request a new RFQ".to_owned(),
            ));
        }
    }
    let changed = transaction
        .execute(
            "UPDATE execution_order_previews SET consumed_at = ?2
             WHERE preview_id = ?1 AND consumed_at IS NULL",
            params![preview_id, timestamp],
        )
        .map_err(ExecutionOrderStoreError::Query)?;
    if changed != 1 {
        return Err(ExecutionOrderStoreError::Conflict(
            "execution preview was consumed concurrently".to_owned(),
        ));
    }
    Ok(())
}

fn validate_preview(preview: &StoredExecutionOrderPreview) -> Result<(), ExecutionOrderStoreError> {
    for (field, value) in [
        ("preview_id", preview.preview_id.as_str()),
        ("request_hash", preview.request_hash.as_str()),
        ("broker_id", preview.broker_id.as_str()),
        ("capability_version", preview.capability_version.as_str()),
        ("account_id", preview.account_id.as_str()),
        ("normalized_request", preview.normalized_request.as_str()),
    ] {
        if value.trim().is_empty() {
            return Err(ExecutionOrderStoreError::Validation(format!(
                "preview {field} is required"
            )));
        }
    }
    validate_timestamp(&preview.expires_at)?;
    validate_timestamp(&preview.created_at)
}

fn validate_timestamp(timestamp: &str) -> Result<(), ExecutionOrderStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            ExecutionOrderStoreError::Validation(format!("invalid RFC3339 timestamp: {error}"))
        })
}
