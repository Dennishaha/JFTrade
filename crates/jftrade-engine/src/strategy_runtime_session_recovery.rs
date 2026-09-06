//! Close an uncertain remote session before resetting the local revision.
use std::future::Future;

use jftrade_integration_pine::{PineExecutionError, PineRunRequest, PineRunResult};

pub(super) async fn run_session_request<F, Fut>(
    run: F,
    request: PineRunRequest,
    revision: &mut u64,
) -> Result<PineRunResult, PineExecutionError>
where
    F: Fn(PineRunRequest) -> Fut,
    Fut: Future<Output = Result<PineRunResult, PineExecutionError>>,
{
    let close = PineRunRequest {
        job_id: format!("recovery-close:{}", request.job_id),
        script_id: request.script_id.clone(),
        source: request.source.clone(),
        symbol: request.symbol.clone(),
        timeframe: request.timeframe.clone(),
        chart_type: request.chart_type.clone(),
        mode: request.mode.clone(),
        params: request.params.clone(),
        session_id: request.session_id.clone(),
        session_operation: "close".to_owned(),
        expected_revision: 0,
        candles: Vec::new(),
    };
    match run(request).await {
        Ok(result) => {
            *revision = result.session_revision;
            Ok(result)
        }
        Err(error) => {
            if *revision > 0 {
                // Validation failures preserve the remote session; transport
                // failures may have advanced its revision or indicate the worker crashed.
                // Reset revision to 0 even if recovery close fails (e.g. worker process
                // died) so that subsequent cycles cleanly reopen and warm up.
                let close_result = run(close).await;
                *revision = 0;
                if let Err(close_error) = close_result {
                    return Err(PineExecutionError::Remote(format!(
                        "append failed: {error}; recovery close unconfirmed: {close_error}"
                    )));
                }
            }
            Err(error)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    #[tokio::test]
    async fn failed_append_closes_unknown_remote_revision_before_reset() {
        let calls = Mutex::new(Vec::new());
        let mut revision = 3;
        let result = run_session_request(
            |request| {
                let operation = request.session_operation.clone();
                calls.lock().unwrap().push(request);
                async move {
                    if operation == "append" {
                        Err(PineExecutionError::Timeout)
                    } else {
                        Ok(PineRunResult::default())
                    }
                }
            },
            PineRunRequest {
                job_id: "bar-4".to_owned(),
                session_id: "strategy:one:US.AAPL".to_owned(),
                session_operation: "append".to_owned(),
                expected_revision: revision,
                ..Default::default()
            },
            &mut revision,
        )
        .await;
        assert_eq!(result.unwrap_err(), PineExecutionError::Timeout);
        assert_eq!(revision, 0);
        let calls = calls.lock().unwrap();
        assert_eq!(calls.len(), 2);
        assert_eq!(calls[1].session_operation, "close");
        assert_eq!(calls[1].session_id, calls[0].session_id);
        assert_eq!(calls[1].expected_revision, 0);
        assert!(calls[1].candles.is_empty());
    }

    #[tokio::test]
    async fn unconfirmed_close_resets_revision_to_zero_for_clean_reopen() {
        let operations = Mutex::new(Vec::new());
        let mut revision = 3;
        let result = run_session_request(
            |request| {
                operations.lock().unwrap().push(request.session_operation);
                async { Err(PineExecutionError::Timeout) }
            },
            PineRunRequest {
                session_operation: "append".to_owned(),
                ..Default::default()
            },
            &mut revision,
        )
        .await;
        assert!(
            result
                .unwrap_err()
                .to_string()
                .contains("close unconfirmed")
        );
        assert_eq!(revision, 0);
        assert_eq!(*operations.lock().unwrap(), ["append", "close"]);
    }

    #[tokio::test]
    async fn successful_append_advances_revision_without_close() {
        let mut revision = 3;
        run_session_request(
            |request| async move {
                assert_eq!(request.session_operation, "append");
                Ok(PineRunResult {
                    session_revision: 4,
                    ..Default::default()
                })
            },
            PineRunRequest {
                session_operation: "append".to_owned(),
                ..Default::default()
            },
            &mut revision,
        )
        .await
        .unwrap();
        assert_eq!(revision, 4);
    }

    #[tokio::test]
    async fn subsequent_cycle_after_unconfirmed_close_initiates_open_operation() {
        let operations = Mutex::new(Vec::new());
        let mut revision = 3;
        // First cycle: append fails and close fails (worker crash)
        let _ = run_session_request(
            |request| {
                operations.lock().unwrap().push(request.session_operation);
                async { Err(PineExecutionError::Timeout) }
            },
            PineRunRequest {
                session_operation: "append".to_owned(),
                ..Default::default()
            },
            &mut revision,
        )
        .await;
        assert_eq!(revision, 0);

        // Next cycle: since revision is 0, caller sends open request
        let open_req = PineRunRequest {
            session_operation: if revision == 0 {
                "open".to_owned()
            } else {
                "append".to_owned()
            },
            expected_revision: revision,
            ..Default::default()
        };
        assert_eq!(open_req.session_operation, "open");
        let result = run_session_request(
            |request| {
                operations.lock().unwrap().push(request.session_operation);
                async {
                    Ok(PineRunResult {
                        session_revision: 1,
                        ..Default::default()
                    })
                }
            },
            open_req,
            &mut revision,
        )
        .await;
        assert!(result.is_ok());
        assert_eq!(revision, 1);
        assert_eq!(*operations.lock().unwrap(), ["append", "close", "open"]);
    }
}
