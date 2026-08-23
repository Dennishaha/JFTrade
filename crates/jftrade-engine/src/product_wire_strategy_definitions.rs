fn is_strategy_definition_detail_path(path: &str) -> bool {
    path.strip_prefix("/api/v1/strategy-definitions/")
        .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
}

fn is_strategy_definition_versions_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/strategy-definitions/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|id| !id.is_empty())
        && parts.next() == Some("versions")
        && parts.next().is_none()
}

fn is_strategy_definition_version_path(path: &str) -> bool {
    let Some(suffix) = path.strip_prefix("/api/v1/strategy-definitions/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|id| !id.is_empty())
        && parts.next() == Some("versions")
        && parts.next().is_some_and(|version| !version.is_empty())
        && parts.next().is_none()
}

fn strategy_definition_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .filter(|id| !id.is_empty() && !id.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let decoded = decode_strategy_path_segment(encoded, "invalid definition id")?;
    let id = decoded.trim();
    if id.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    Ok(id.to_owned())
}

fn strategy_definition_versions_id(path: &str) -> Result<String, ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    let mut parts = suffix.split('/');
    let encoded_id = parts
        .next()
        .filter(|id| !id.is_empty())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"))?;
    if parts.next() != Some("versions") || parts.next().is_some() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    let id = decode_strategy_path_segment(encoded_id, "invalid definition id")?;
    let id = id.trim();
    if id.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition id"));
    }
    Ok(id.to_owned())
}

fn strategy_definition_version_path(path: &str) -> Result<(String, String), ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/strategy-definitions/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    let mut parts = suffix.split('/');
    let encoded_id = parts
        .next()
        .filter(|id| !id.is_empty())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    if parts.next() != Some("versions") {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"));
    }
    let encoded_version = parts
        .next()
        .filter(|version| !version.is_empty())
        .filter(|_| parts.next().is_none())
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"))?;
    let decode = |encoded: &str| {
        decode_strategy_path_segment(encoded, "invalid definition version")
            .map(|value| value.trim().to_owned())
    };
    let id = decode(encoded_id)?;
    let version = decode(encoded_version)?;
    if id.is_empty() || version.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "invalid definition version"));
    }
    Ok((id, version))
}

fn parse_strategy_definition_preview(
    query: &str,
) -> Result<StrategyDefinitionPreview, ApiFailure> {
    let mut preview = StrategyDefinitionPreview::default();
    for pair in query.split('&').filter(|value| !value.is_empty()) {
        let (encoded_name, encoded_value) = pair.split_once('=').unwrap_or((pair, ""));
        let (Some(name), Some(value)) = (
            decode_strategy_query_component(encoded_name),
            decode_strategy_query_component(encoded_value),
        ) else {
            continue;
        };
        match name.as_str() {
            "interval" => preview.interval = Some(value),
            "symbol" => preview.symbol = Some(value),
            "useExtendedHours" => {
                preview.use_extended_hours = match value.to_ascii_lowercase().as_str() {
                    "true" | "1" => true,
                    "false" | "0" | "" => false,
                    _ => {
                        return Err(ApiFailure::new(
                            400,
                            "BAD_REQUEST",
                            "invalid strategy definition query",
                        ));
                    }
                };
            }
            _ => {}
        }
    }
    Ok(preview)
}

fn decode_strategy_path_segment(encoded: &str, message: &str) -> Result<String, ApiFailure> {
    if has_invalid_percent_escape(encoded) {
        return Err(ApiFailure::new(400, "BAD_REQUEST", message));
    }
    percent_decode_str(encoded)
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", message))
}

fn decode_strategy_query_component(value: &str) -> Option<String> {
    if has_invalid_percent_escape(value) {
        return None;
    }
    Some(
        percent_decode_str(&value.replace('+', " "))
            .decode_utf8_lossy()
            .into_owned(),
    )
}

fn has_invalid_percent_escape(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len()
            || !bytes[index + 1].is_ascii_hexdigit()
            || !bytes[index + 2].is_ascii_hexdigit()
        {
            return true;
        }
        index += 3;
    }
    false
}
