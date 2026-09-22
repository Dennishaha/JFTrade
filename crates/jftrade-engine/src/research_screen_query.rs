//! Futu stock-screen query parsing and projection bridge.

use super::*;

// The legacy GET route carries the V2 definition as a URL-encoded JSON query
// parameter.  Keep it on the same typed OpenD reader as POST so pagination,
// identity resolution and error mapping cannot diverge between entry points.
pub(super) fn read_futu_screen(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    query: &str,
) -> Result<Value, ResearchReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        ResearchReadSnapshotError::Unavailable(
            "Futu stock-screen research runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.stock_screen_reader_available() {
        return Err(ResearchReadSnapshotError::Unavailable(
            "Futu OpenD stock-screen reader is not ready".to_owned(),
        ));
    }
    let query_map = QueryMap::parse(query)
        .map_err(|_| ResearchReadSnapshotError::Invalid("invalid URL escape".to_owned()))?;
    let raw_definition = query_map
        .get_first("researchScreenDefinition")
        .ok_or_else(|| {
            ResearchReadSnapshotError::Invalid(
                "researchScreenDefinition is required for stock screen".to_owned(),
            )
        })?;
    let definition = serde_json::from_str::<Value>(raw_definition).map_err(|error| {
        ResearchReadSnapshotError::Invalid(format!(
            "researchScreenDefinition must be JSON: {error}"
        ))
    })?;
    let definition = normalize_definition_v2(definition)
        .map_err(|error| ResearchReadSnapshotError::Invalid(error.to_string()))?;
    let object = definition.as_object().ok_or_else(|| {
        ResearchReadSnapshotError::Invalid("stock-screen definition must be an object".to_owned())
    })?;
    let market = object
        .get("market")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim()
        .to_ascii_uppercase();
    let columns = object
        .get("columns")
        .and_then(Value::as_array)
        .map_or(&[] as &[Value], |values| values)
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let column = value.as_object().ok_or_else(|| {
                ResearchReadSnapshotError::Invalid(format!("columns[{index}] must be an object"))
            })?;
            let factor = column.get("factor").and_then(Value::as_object).ok_or_else(|| {
                ResearchReadSnapshotError::Invalid(format!(
                    "columns[{index}].factor is required"
                ))
            })?;
            let factor_key = factor
                .get("factorKey")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_ascii_lowercase();
            let column_id = column
                .get("columnId")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned();
            let instance_id = factor
                .get("instanceId")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned();
            if factor_key.is_empty() || column_id.is_empty() || instance_id.is_empty() {
                return Err(ResearchReadSnapshotError::Invalid(format!(
                    "columns[{index}] has incomplete identity"
                )));
            }
            let unit = jftrade_research::screen_catalog("futu", &market)
                .ok()
                .and_then(|catalog| catalog.get("factors").and_then(Value::as_array).cloned())
                .and_then(|factors| {
                    factors
                        .into_iter()
                        .find(|factor| factor.get("key").and_then(Value::as_str) == Some(factor_key.as_str()))
                })
                .and_then(|factor| factor.get("unit").and_then(Value::as_str).map(str::to_owned))
                .unwrap_or_default();
            Ok(crate::product::product_research_screen_write_port::ResearchScreenColumn {
                column_id,
                instance_id,
                factor_key,
                label: column
                    .get("label")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned(),
                unit,
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let offset = parse_screen_query_i64(&query_map, &["cursor", "pageFrom", "offset"])?.unwrap_or(0);
    let limit = parse_screen_query_i64(&query_map, &["pageSize", "pageCount", "limit"])?.unwrap_or(50);
    if offset < 0 || !(1..=100).contains(&limit) {
        return Err(ResearchReadSnapshotError::Invalid(
            "stock-screen pagination is out of range".to_owned(),
        ));
    }
    let request = crate::product::product_research_screen_write_port::ResearchScreenWriteQuery {
        broker_id: query_map
            .get_first("brokerId")
            .unwrap_or("futu")
            .trim()
            .to_ascii_lowercase(),
        account_id: query_map.get_first("accountId").unwrap_or_default().to_owned(),
        trading_environment: query_map
            .get_first("tradingEnvironment")
            .unwrap_or_default()
            .trim()
            .to_ascii_uppercase(),
        market,
        offset,
        limit,
        definition,
        columns,
    };
    screen::query_futu_runtime(runtime, &request).map_err(map_futu_screen_write_error)
}

fn parse_screen_query_i64(
    query: &QueryMap,
    keys: &[&str],
) -> Result<Option<i64>, ResearchReadSnapshotError> {
    let Some((key, value)) = keys
        .iter()
        .find_map(|key| query.get_first(key).map(|value| (*key, value)))
    else {
        return Ok(None);
    };
    value
        .trim()
        .parse::<i64>()
        .map(Some)
        .map_err(|_| ResearchReadSnapshotError::Invalid(format!("{key} must be an integer")))
}

fn map_futu_screen_write_error(
    error: ResearchScreenWritePortError,
) -> ResearchReadSnapshotError {
    match error {
        ResearchScreenWritePortError::Unavailable
        | ResearchScreenWritePortError::ProviderWarming
        | ResearchScreenWritePortError::ProviderBusy => {
            ResearchReadSnapshotError::Unavailable(error.to_string())
        }
        ResearchScreenWritePortError::RateLimited {
            message,
            retry_after,
        } => ResearchReadSnapshotError::Failed {
            status: 429,
            code: "RESEARCH_SCREEN_RATE_LIMITED".to_owned(),
            message,
            retry_after_seconds: Some(retry_after),
        },
        ResearchScreenWritePortError::Capability(message) => ResearchReadSnapshotError::Failed {
            status: 409,
            code: "BROKER_CAPABILITY_UNAVAILABLE".to_owned(),
            message,
            retry_after_seconds: None,
        },
        ResearchScreenWritePortError::Failed(message) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
            retry_after_seconds: None,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:355
    /// TestResearchScreenPostServesEmbeddedProvider (capability branch)
    ///
    /// The typed screen write path renders a broker capability rejection as 409
    /// `BROKER_CAPABILITY_UNAVAILABLE` — the code the console's
    /// ProviderUnsupportedState fallback keys on (frozen `research-screens`
    /// fixture case `capability-error`).
    #[test]
    fn futu_screen_capability_errors_use_the_broker_code() {
        let error = map_futu_screen_write_error(ResearchScreenWritePortError::Capability(
            "requested broker does not match active provider \"futu\"".to_owned(),
        ));
        match error {
            ResearchReadSnapshotError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            } => {
                assert_eq!(status, 409);
                assert_eq!(code, "BROKER_CAPABILITY_UNAVAILABLE");
                assert!(message.contains("does not match active provider"));
                assert_eq!(retry_after_seconds, None);
            }
            other => panic!("expected a capability failure, got {other:?}"),
        }
    }

    /// The other screen-write failures keep their own contract: a rate limit
    /// carries the rounded Retry-After, warming/busy stay 503 lifecycle errors,
    /// and a provider failure is the generic 502.
    // Parity: go:452dea11:internal/api/productfeatures/research_screen_test.go:70 TestWriteResearchScreenErrorReturnsStructured429
    #[test]
    fn futu_screen_write_errors_keep_their_transport_contract() {
        let rate_limited = map_futu_screen_write_error(ResearchScreenWritePortError::RateLimited {
            message: "research stock screen rate limited; retry after 2.5s".to_owned(),
            retry_after: 3,
        });
        assert!(matches!(
            rate_limited,
            ResearchReadSnapshotError::Failed {
                status: 429,
                ref code,
                retry_after_seconds: Some(3),
                ..
            } if code == "RESEARCH_SCREEN_RATE_LIMITED"
        ));

        for lifecycle in [
            ResearchScreenWritePortError::Unavailable,
            ResearchScreenWritePortError::ProviderWarming,
            ResearchScreenWritePortError::ProviderBusy,
        ] {
            assert!(matches!(
                map_futu_screen_write_error(lifecycle),
                ResearchReadSnapshotError::Unavailable(_)
            ));
        }

        let failed = map_futu_screen_write_error(ResearchScreenWritePortError::Failed(
            "fixture broker failed".to_owned(),
        ));
        assert!(matches!(
            failed,
            ResearchReadSnapshotError::Failed {
                status: 502,
                ref code,
                ..
            } if code == "BAD_GATEWAY"
        ));
    }
}
