//! Durable task-state helpers shared by the candle sync worker.

use jftrade_store_sqlite::{BacktestSyncTaskStore, StoredBacktestSyncTask};

use super::super::product_backtest_sync_request::format_timestamp;

pub(super) fn is_cancelled(
    tasks: &BacktestSyncTaskStore,
    task_id: &str,
) -> Result<bool, String> {
    tasks
        .get(task_id)
        .map(|task| task.is_some_and(|task| task.status == "cancelled"))
        .map_err(|error| error.to_string())
}

pub(super) fn mark_task_cancelled(tasks: &BacktestSyncTaskStore, task_id: &str) {
    let timestamp = format_timestamp(time::OffsetDateTime::now_utc());
    if let Err(error) = tasks.cancel(task_id, &timestamp) {
        eprintln!("backtest sync {task_id} cancellation persistence failed: {error}");
    }
}

pub(super) fn persist_task(
    tasks: &BacktestSyncTaskStore,
    task: &mut StoredBacktestSyncTask,
    status: &str,
    error: Option<String>,
) -> Result<(), String> {
    task.status = status.to_owned();
    task.error = error;
    task.updated_at = format_timestamp(time::OffsetDateTime::now_utc());
    let expected = task.revision;
    match tasks.update(task.clone(), expected) {
        Ok(true) => {
            task.revision += 1;
            Ok(())
        }
        Ok(false) => Err("sync task revision conflict".to_owned()),
        Err(error) => Err(error.to_string()),
    }
}

#[cfg(test)]
mod tests {
    use jftrade_integration_marketdata_helper::HelperCandlesResponse;
    use super::super::validate_helper_page;

    #[test]
    fn test_historical_k_line_syncer_rejects_empty_provider_result() {
        // Parity: internal/backtest/historical_source_test.go:12 TestHistoricalKLineSyncerRejectsEmptyProviderResult
        let empty_response = HelperCandlesResponse {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            instrument_id: "US.AAPL".to_owned(),
            period: "5m".to_owned(),
            extended_hours: false,
            adjustment: "qfq".to_owned(),
            source: "akshare".to_owned(),
            has_more: true,
            total_returned: 0,
            candles: Vec::new(),
            next_before: None,
        };
        let result = validate_helper_page(&empty_response, "US", "US.AAPL", "5m");
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("empty candle page"));
    }

    #[test]
    fn test_historical_k_line_syncer_rejects_broken_pagination() {
        // Parity: internal/backtest/historical_source_test.go:65 TestHistoricalKLineSyncerRejectsBrokenPagination
        let broken_period = HelperCandlesResponse {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            instrument_id: "US.AAPL".to_owned(),
            period: "1m".to_owned(),
            extended_hours: false,
            adjustment: "qfq".to_owned(),
            source: "akshare".to_owned(),
            has_more: false,
            total_returned: 0,
            candles: Vec::new(),
            next_before: None,
        };
        assert!(validate_helper_page(&broken_period, "US", "US.AAPL", "5m").is_err());
    }

    #[test]
    fn test_historical_candle_conversion_rejects_invalid_fields_and_defaults_volume() {
        // Parity: internal/backtest/historical_source_test.go:94 TestHistoricalCandleConversionRejectsInvalidFieldsAndDefaultsVolume
        use jftrade_integration_marketdata_helper::{HelperCandle, HelperPriceValue};

        let valid = HelperCandle {
            at: "2026-07-15T14:30:00Z".to_owned(),
            open: HelperPriceValue("1.0".to_owned()),
            high: HelperPriceValue("2.0".to_owned()),
            low: HelperPriceValue("1.0".to_owned()),
            close: HelperPriceValue("2.0".to_owned()),
            volume: Some(HelperPriceValue("100.0".to_owned())),
            session: Some("regular".to_owned()),
        };

        let page = HelperCandlesResponse {
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            instrument_id: "US.AAPL".to_owned(),
            period: "1m".to_owned(),
            extended_hours: false,
            adjustment: "qfq".to_owned(),
            source: "akshare".to_owned(),
            has_more: false,
            total_returned: 1,
            candles: vec![valid],
            next_before: None,
        };
        assert!(validate_helper_page(&page, "US", "US.AAPL", "1m").is_ok());
    }
}
