//! Durable prediction RFQ (Parlay combo quote) ledger.
//!
//! Go keeps the `execution_prediction_quotes` table inside the execution-order
//! SQLite database and exposes three operations through
//! `broker.PredictionQuoteStore`: persist a quote returned by OpenD, validate
//! it against the exact broker/account/environment/MVC/legs binding before a
//! parlay is priced, and consume it exactly once when an order is submitted.
//! That table and those three operations are the only durable guard that keeps
//! a 30 second server-issued RFQ from being replayed for a different account, a
//! changed leg set, an expired window, or a second client order id.

use rusqlite::{OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::execution_order::{ExecutionOrderStore, ExecutionOrderStoreError};

/// Persisted prediction RFQ exactly as Go's `broker.PredictionQuoteRecord`
/// serializes it.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredPredictionQuote {
    pub quote_id: String,
    pub broker_id: String,
    pub account_id: String,
    pub trading_environment: String,
    pub mvc: String,
    pub legs_hash: String,
    pub bid_price: Option<f64>,
    pub ask_price: Option<f64>,
    pub should_retry: bool,
    pub received_at: String,
    pub expires_at: String,
    pub expiry_source: String,
    pub status: String,
    pub consumed_at: Option<String>,
    pub consumed_preview_id: Option<String>,
    pub consumed_client_order_id: Option<String>,
}

impl ExecutionOrderStore {
    /// Persist one RFQ that the market-data service just received.
    pub fn save_prediction_quote(
        &self,
        quote: &StoredPredictionQuote,
    ) -> Result<(), ExecutionOrderStoreError> {
        validate_prediction_quote(quote)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .execute(
                "INSERT INTO execution_prediction_quotes (
                    quote_id, broker_id, account_id, trading_environment, mvc,
                    legs_hash, bid_price, ask_price, should_retry, received_at,
                    expires_at, expiry_source, status, consumed_at,
                    consumed_preview_id, consumed_client_order_id
                 ) VALUES (
                    ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10,
                    ?11, ?12, ?13, ?14, ?15, ?16
                 )",
                params![
                    quote.quote_id,
                    quote.broker_id,
                    quote.account_id,
                    quote.trading_environment,
                    quote.mvc,
                    quote.legs_hash,
                    quote.bid_price,
                    quote.ask_price,
                    i64::from(quote.should_retry),
                    quote.received_at,
                    quote.expires_at,
                    quote.expiry_source,
                    quote.status,
                    quote.consumed_at,
                    quote.consumed_preview_id,
                    quote.consumed_client_order_id,
                ],
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)
    }

    /// Read an RFQ only when the caller presents the same binding Go requires.
    ///
    /// A mismatch, a missing row, an unparsable timestamp and an expired window
    /// are all hard failures: Go's `predictionQuote` returns an error for every
    /// one of them and the trading service turns it into a caller-visible
    /// `prediction RFQ is invalid` response.
    #[allow(clippy::too_many_arguments)]
    pub fn validate_prediction_quote(
        &self,
        quote_id: &str,
        broker_id: &str,
        account_id: &str,
        trading_environment: &str,
        mvc: &str,
        legs_hash: &str,
        now: &str,
    ) -> Result<StoredPredictionQuote, ExecutionOrderStoreError> {
        let now = parse_timestamp(now, "prediction RFQ timestamp")?;
        let quote_id = quote_id.trim();
        if quote_id.is_empty() {
            return Err(ExecutionOrderStoreError::NotFound(String::new()));
        }
        let connection = self.lock()?;
        let quote = connection
            .query_row(
                "SELECT quote_id, broker_id, account_id, trading_environment, mvc,
                        legs_hash, bid_price, ask_price, should_retry, received_at,
                        expires_at, expiry_source, status, consumed_at,
                        consumed_preview_id, consumed_client_order_id
                 FROM execution_prediction_quotes WHERE quote_id = ?1 LIMIT 1",
                params![quote_id],
                |row| {
                    Ok(StoredPredictionQuote {
                        quote_id: row.get(0)?,
                        broker_id: row.get(1)?,
                        account_id: row.get(2)?,
                        trading_environment: row.get(3)?,
                        mvc: row.get(4)?,
                        legs_hash: row.get(5)?,
                        bid_price: row.get(6)?,
                        ask_price: row.get(7)?,
                        should_retry: row.get::<_, i64>(8)? != 0,
                        received_at: row.get(9)?,
                        expires_at: row.get(10)?,
                        expiry_source: row.get(11)?,
                        status: row.get(12)?,
                        consumed_at: row.get(13)?,
                        consumed_preview_id: row.get(14)?,
                        consumed_client_order_id: row.get(15)?,
                    })
                },
            )
            .optional()
            .map_err(ExecutionOrderStoreError::Query)?
            .ok_or_else(|| ExecutionOrderStoreError::NotFound(quote_id.to_owned()))?;
        if !quote.broker_id.eq_ignore_ascii_case(broker_id.trim())
            || quote.account_id != account_id.trim()
            || !quote
                .trading_environment
                .eq_ignore_ascii_case(trading_environment.trim())
            || quote.mvc != mvc.trim()
            || quote.legs_hash != legs_hash.trim()
        {
            return Err(ExecutionOrderStoreError::Validation(
                "prediction RFQ broker, account, environment, MVC, or legs changed".to_owned(),
            ));
        }
        let expires_at = parse_timestamp(&quote.expires_at, "prediction RFQ expiry")?;
        let _ = parse_timestamp(&quote.received_at, "prediction RFQ received time")?;
        if expires_at <= now {
            return Err(ExecutionOrderStoreError::Validation(
                "prediction RFQ expired; request a new quote".to_owned(),
            ));
        }
        Ok(quote)
    }

    /// Mark an RFQ consumed exactly once for one preview/client-order pair.
    ///
    /// Replaying the identical pair is idempotent, matching Go's
    /// `consumePredictionQuote`; a different pair on an already-consumed quote
    /// and a concurrent second consumer both fail closed.
    #[allow(clippy::too_many_arguments)]
    pub fn consume_prediction_quote(
        &self,
        quote_id: &str,
        broker_id: &str,
        account_id: &str,
        trading_environment: &str,
        mvc: &str,
        legs_hash: &str,
        preview_id: &str,
        client_order_id: &str,
        now: &str,
    ) -> Result<(), ExecutionOrderStoreError> {
        let quote = self.validate_prediction_quote(
            quote_id,
            broker_id,
            account_id,
            trading_environment,
            mvc,
            legs_hash,
            now,
        )?;
        if quote.status == "consumed" {
            if quote.consumed_preview_id.as_deref() == Some(preview_id)
                && quote.consumed_client_order_id.as_deref() == Some(client_order_id)
            {
                return Ok(());
            }
            return Err(ExecutionOrderStoreError::Conflict(
                "prediction RFQ already consumed".to_owned(),
            ));
        }
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(ExecutionOrderStoreError::Query)?;
        let changed = transaction
            .execute(
                "UPDATE execution_prediction_quotes
                 SET status = 'consumed', consumed_at = ?2,
                     consumed_preview_id = ?3, consumed_client_order_id = ?4
                 WHERE quote_id = ?1 AND status = 'active' AND expires_at > ?2",
                params![quote_id.trim(), now, preview_id, client_order_id],
            )
            .map_err(ExecutionOrderStoreError::Query)?;
        if changed != 1 {
            return Err(ExecutionOrderStoreError::Conflict(
                "prediction RFQ expired or already consumed".to_owned(),
            ));
        }
        transaction
            .commit()
            .map_err(ExecutionOrderStoreError::Query)
    }
}

fn validate_prediction_quote(
    quote: &StoredPredictionQuote,
) -> Result<(), ExecutionOrderStoreError> {
    for (field, value) in [
        ("quote_id", quote.quote_id.as_str()),
        ("broker_id", quote.broker_id.as_str()),
        ("account_id", quote.account_id.as_str()),
        ("trading_environment", quote.trading_environment.as_str()),
        ("mvc", quote.mvc.as_str()),
        ("legs_hash", quote.legs_hash.as_str()),
        ("expiry_source", quote.expiry_source.as_str()),
    ] {
        if value.trim().is_empty() {
            return Err(ExecutionOrderStoreError::Validation(format!(
                "prediction RFQ {field} is required"
            )));
        }
    }
    if quote.status != "active" && quote.status != "consumed" {
        return Err(ExecutionOrderStoreError::Validation(
            "prediction RFQ status must be active or consumed".to_owned(),
        ));
    }
    // Go writes these columns through `time.Time.Format(time.RFC3339Nano)`, so
    // a row can never carry an unparsable timestamp. Reject one here instead of
    // discovering it on the next validate call.
    parse_timestamp(&quote.received_at, "prediction RFQ received time")?;
    parse_timestamp(&quote.expires_at, "prediction RFQ expiry")?;
    if let Some(consumed_at) = quote.consumed_at.as_deref() {
        parse_timestamp(consumed_at, "prediction RFQ consumed time")?;
    }
    Ok(())
}

fn parse_timestamp(value: &str, label: &str) -> Result<OffsetDateTime, ExecutionOrderStoreError> {
    OffsetDateTime::parse(value, &Rfc3339).map_err(|error| {
        ExecutionOrderStoreError::Validation(format!("{label} is invalid: {error}"))
    })
}
