pub const MARKET_DATA_PREDICTION_READ_ROUTES: [(&str, &str); 12] = [
    ("GET", "/api/v1/market-data/prediction/categories"),
    (
        "GET",
        "/api/v1/market-data/prediction/combos/eligible-events",
    ),
    ("GET", "/api/v1/market-data/prediction/competitions"),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/candles",
    ),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/candles/history",
    ),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/milestones",
    ),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/order-book",
    ),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/snapshot",
    ),
    (
        "GET",
        "/api/v1/market-data/prediction/contracts/{code}/ticks",
    ),
    ("GET", "/api/v1/market-data/prediction/events"),
    (
        "GET",
        "/api/v1/market-data/prediction/events/{eventId}/contracts",
    ),
    ("GET", "/api/v1/market-data/prediction/series"),
];

pub fn market_data_prediction_read_routes() -> &'static [(&'static str, &'static str)] {
    &MARKET_DATA_PREDICTION_READ_ROUTES
}

pub fn is_market_data_prediction_read_path(path: &str) -> bool {
    if matches!(
        path,
        "/api/v1/market-data/prediction/categories"
            | "/api/v1/market-data/prediction/combos/eligible-events"
            | "/api/v1/market-data/prediction/competitions"
            | "/api/v1/market-data/prediction/events"
            | "/api/v1/market-data/prediction/series"
    ) {
        return true;
    }
    [
        "/api/v1/market-data/prediction/contracts/",
        "/api/v1/market-data/prediction/events/",
    ]
    .iter()
    .any(|prefix| is_prediction_read_variable_suffix(path, prefix))
}

fn is_prediction_read_variable_suffix(path: &str, prefix: &str) -> bool {
    let Some(suffix) = path.strip_prefix(prefix) else {
        return false;
    };
    let mut segments = suffix.split('/');
    let Some(variable) = segments.next() else {
        return false;
    };
    if variable.is_empty() {
        return false;
    }
    match prefix {
        "/api/v1/market-data/prediction/contracts/" => {
            matches!(
                segments.collect::<Vec<_>>().as_slice(),
                ["candles"]
                    | ["candles", "history"]
                    | ["milestones"]
                    | ["order-book"]
                    | ["snapshot"]
                    | ["ticks"]
            )
        }
        "/api/v1/market-data/prediction/events/" => {
            segments.collect::<Vec<_>>().as_slice() == ["contracts"]
        }
        _ => false,
    }
}
