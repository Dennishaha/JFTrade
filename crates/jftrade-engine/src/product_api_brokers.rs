impl ProductApi {
    fn broker_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.broker_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "BROKER_READ_UNAVAILABLE",
                "broker read snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(broker_read_snapshot_failure)
    }
}

fn is_broker_read_path(path: &str) -> bool {
    if path == "/api/v1/brokers/capabilities" {
        return true;
    }
    const RESOURCES: [&str; 12] = [
        "runtime",
        "funds",
        "positions",
        "orders",
        "fills",
        "cash-flows",
        "order-fees",
        "margin-ratios",
        "max-trade-qtys",
        "quote",
        "klines",
        "securities",
    ];
    let Some(suffix) = path.strip_prefix("/api/v1/brokers/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    let broker_id = parts.next().unwrap_or_default();
    let resource = parts.next().unwrap_or_default();
    !broker_id.is_empty()
        && !broker_id.contains('/')
        && RESOURCES.contains(&resource)
        && parts.next().is_none()
}
