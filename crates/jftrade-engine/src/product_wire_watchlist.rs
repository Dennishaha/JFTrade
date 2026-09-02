fn is_watchlist_membership_path(path: &str) -> bool {
    watchlist_membership_path_parts(path).is_some()
}

fn watchlist_membership_instrument_id(path: &str) -> Result<String, ApiFailure> {
    let (market, symbol) = watchlist_membership_path_parts(path)
        .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "unknown watchlist endpoint"))?;
    let raw = format!("{market}.{symbol}");
    normalize_instrument_id(&raw)
        .map_err(|error| ApiFailure::new(400, "WATCHLIST_INVALID", watchlist_error_message(error)))
}

fn watchlist_membership_path_parts(path: &str) -> Option<(String, String)> {
    let suffix = path.strip_prefix("/api/v1/watchlist/instruments/")?;
    let suffix = suffix.strip_suffix("/memberships")?;
    let mut parts = suffix.split('/');
    let market = percent_decode_str(parts.next()?)
        .decode_utf8()
        .ok()?
        .into_owned();
    let symbol = percent_decode_str(parts.next()?)
        .decode_utf8()
        .ok()?
        .into_owned();
    if parts.next().is_some() || market.is_empty() || symbol.is_empty() {
        return None;
    }
    Some((market, symbol))
}

fn watchlist_error_message(error: WatchlistError) -> String {
    error.to_string()
}

fn is_watchlist_read_path(path: &str) -> bool {
    matches!(
        path,
        "/api/v1/watchlist/groups"
            | "/api/v1/watchlist/items"
            | "/api/v1/watchlist/sources"
            | "/api/v1/watchlist/bindings"
            | "/api/v1/watchlist/import-runs"
    ) || path
        .strip_prefix("/api/v1/watchlist/sources/")
        .is_some_and(|source_id| {
            source_id.ends_with("/groups")
                && !source_id.trim_end_matches("/groups").is_empty()
                && !source_id.trim_end_matches("/groups").contains('/')
        })
}

fn watchlist_read_snapshot_failure(error: WatchlistReadSnapshotError) -> ApiFailure {
    match error {
        WatchlistReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
        WatchlistReadSnapshotError::NotFound => {
            ApiFailure::new(404, "WATCHLIST_NOT_FOUND", "watchlist resource not found")
        }
        WatchlistReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "WATCHLIST_UNAVAILABLE", message)
        }
    }
}
