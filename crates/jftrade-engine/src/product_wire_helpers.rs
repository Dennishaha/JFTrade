fn is_managed_account_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/settings/broker-accounts/")
        .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn managed_account_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/settings/broker-accounts/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))?;
    percent_decode_str(encoded)
        .decode_utf8()
        .map(|id| id.into_owned())
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid account id"))
}
