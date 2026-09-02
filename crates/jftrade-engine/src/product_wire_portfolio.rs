fn is_portfolio_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/portfolio/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    let broker_id = parts.next().unwrap_or_default();
    let resource = parts.next().unwrap_or_default();
    !broker_id.is_empty()
        && !broker_id.contains('/')
        && matches!(resource, "cash-balances" | "positions")
        && parts.next().is_none()
}
