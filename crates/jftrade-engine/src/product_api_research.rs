impl ProductApi {
    fn research_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.research_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "RESEARCH_UNAVAILABLE",
                "research read snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(research_read_snapshot_failure)
    }
}

fn is_research_read_path(path: &str) -> bool {
    const EXACT: [&str; 6] = [
        "/api/v1/research/screens",
        "/api/v1/research/calendars",
        "/api/v1/research/macro",
        "/api/v1/research/rankings",
        "/api/v1/research/institutions",
        "/api/v1/research/industries",
    ];
    if EXACT.contains(&path) {
        return true;
    }
    const PREFIXES: [&str; 8] = [
        "/api/v1/research/instruments/",
        "/api/v1/research/financials/",
        "/api/v1/research/valuation/",
        "/api/v1/research/analyst/",
        "/api/v1/research/ownership/",
        "/api/v1/research/corporate-actions/",
        "/api/v1/research/short-interest/",
        "/api/v1/research/technical-indicators/",
    ];
    PREFIXES.iter().any(|prefix| {
        path.strip_prefix(prefix)
            .is_some_and(|instrument| !instrument.is_empty() && !instrument.contains('/'))
    })
}
