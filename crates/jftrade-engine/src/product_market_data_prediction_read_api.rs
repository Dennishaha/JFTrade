pub struct MarketDataPredictionReadApi {
    port: Option<std::sync::Arc<dyn MarketDataPredictionReadSnapshotPort>>,
}

impl MarketDataPredictionReadApi {
    pub fn new(
        port: Option<std::sync::Arc<dyn MarketDataPredictionReadSnapshotPort>>,
    ) -> Self {
        Self { port }
    }

    pub fn dispatch(
        &self,
        request: &jftrade_api::ApiRequest,
    ) -> Result<jftrade_api::ApiOutput, jftrade_api::ApiFailure> {
        if request.method != "GET" {
            return Err(unknown_prediction_read_endpoint(&request.path));
        }
        dispatch_market_data_prediction_read(self.port.as_deref(), &request.path, &request.query)
    }
}

pub fn dispatch_market_data_prediction_read(
    port: Option<&dyn MarketDataPredictionReadSnapshotPort>,
    path: &str,
    query: &str,
) -> Result<jftrade_api::ApiOutput, jftrade_api::ApiFailure> {
    if !is_market_data_prediction_read_path(path) {
        return Err(unknown_prediction_read_endpoint(path));
    }
    let Some(port) = port else {
        return Err(jftrade_api::ApiFailure::new(
            503,
            "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            "market-data prediction snapshot is not configured",
        ));
    };
    port.read(path, query)
        .map(jftrade_api::ApiOutput::Json)
        .map_err(market_data_prediction_read_snapshot_failure)
}

fn unknown_prediction_read_endpoint(path: &str) -> jftrade_api::ApiFailure {
    jftrade_api::ApiFailure::new(404, "NOT_FOUND", format!("unknown endpoint {path}"))
}

fn market_data_prediction_read_snapshot_failure(
    error: MarketDataPredictionReadSnapshotError,
) -> jftrade_api::ApiFailure {
    match error {
        MarketDataPredictionReadSnapshotError::Invalid(message) => {
            jftrade_api::ApiFailure::new(400, "MARKET_DATA_PREDICTION_READ_INVALID", message)
        }
        MarketDataPredictionReadSnapshotError::Unavailable(message) => {
            jftrade_api::ApiFailure::new(503, "MARKET_DATA_PREDICTION_READ_UNAVAILABLE", message)
        }
        MarketDataPredictionReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let failure = jftrade_api::ApiFailure::new(status, code, message);
            match retry_after_seconds {
                Some(seconds) => failure.with_retry_after(seconds),
                None => failure,
            }
        }
    }
}
