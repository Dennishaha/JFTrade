use super::*;
use super::strategy_runtime_port::ProductionStrategyRuntimePort;

#[derive(Debug)]
pub(super) struct StrategyActivityQuery {
    pub(super) limit: usize,
    pub(super) offset: usize,
    pub(super) selector: String,
    pub(super) from_ms: Option<i64>,
    pub(super) to_ms: Option<i64>,
}

impl StrategyActivityQuery {
    pub(super) fn includes(&self, at_ms: i64) -> bool {
        self.from_ms.is_none_or(|from| at_ms >= from) && self.to_ms.is_none_or(|to| at_ms <= to)
    }
}

pub(super) fn strategy_activity_path(path: &str) -> Option<(&str, &str)> {
    let suffix = path.strip_prefix("/api/v1/strategies/")?;
    let (instance_id, activity) = suffix.split_once('/')?;
    (!instance_id.is_empty() && matches!(activity, "logs" | "audit"))
        .then_some((instance_id, activity))
}

pub(super) fn parse_activity_query(
    raw_query: &str,
    activity: &str,
) -> Result<StrategyActivityQuery, StrategyReadSnapshotError> {
    let invalid = || StrategyReadSnapshotError::Invalid(format!("invalid {activity} query"));
    let mut query = StrategyActivityQuery {
        limit: 500,
        offset: 0,
        selector: String::new(),
        from_ms: None,
        to_ms: None,
    };
    for pair in raw_query.split('&').filter(|pair| !pair.is_empty()) {
        let (raw_name, raw_value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_value(raw_name).map_err(|_| invalid())?;
        let value = decode_query_value(raw_value).map_err(|_| invalid())?;
        match name.as_str() {
            "limit" => {
                let value = value.parse::<i64>().map_err(|_| invalid())?;
                query.limit = value.clamp(1, 5000) as usize;
            }
            "offset" => {
                let value = value.parse::<i64>().map_err(|_| invalid())?;
                query.offset = value.max(0) as usize;
            }
            "level" if activity == "logs" => query.selector = value.trim().to_lowercase(),
            "kind" if activity == "audit" => query.selector = value.trim().to_owned(),
            "fromTime" => query.from_ms = Some(parse_timestamp_millis(&value, invalid())?),
            "toTime" => query.to_ms = Some(parse_timestamp_millis(&value, invalid())?),
            _ => {}
        }
    }
    Ok(query)
}

fn decode_query_value(value: &str) -> Result<String, ()> {
    percent_encoding::percent_decode_str(&value.replace('+', " "))
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| ())
}

fn parse_timestamp_millis(
    value: &str,
    invalid: StrategyReadSnapshotError,
) -> Result<i64, StrategyReadSnapshotError> {
    let timestamp =
        time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339)
            .map_err(|_| invalid)?;
    i64::try_from(timestamp.unix_timestamp_nanos() / 1_000_000).map_err(|_| {
        StrategyReadSnapshotError::Invalid("strategy activity timestamp is out of range".to_owned())
    })
}

pub(super) fn timestamp_from_millis(value: i64) -> Result<String, StrategyReadSnapshotError> {
    let timestamp = time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(value) * 1_000_000)
        .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
    timestamp
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))
}

pub(super) fn page_values<T>(values: Vec<T>, query: &StrategyActivityQuery) -> (Vec<T>, Value) {
    let total = values.len();
    let values = values
        .into_iter()
        .skip(query.offset)
        .take(query.limit)
        .collect::<Vec<_>>();
    let returned = values.len();
    let page = json!({
        "limit": query.limit,
        "offset": query.offset,
        "returned": returned,
        "total": total,
        "hasMore": query.offset.saturating_add(returned) < total,
    });
    (values, page)
}

impl StrategyRuntimeStatusPort for ProductionStrategyRuntimePort {
    fn snapshot(&self) -> StrategyRuntimeSummary {
        let Ok(instances) = self.store.list_instances() else {
            return StrategyRuntimeSummary {
                status: "failed".to_owned(),
                active_strategies: 0,
                supports_backtest_parity: true,
                active_instances: Vec::new(),
            };
        };
        let active_strategies = instances
            .iter()
            .filter(|i| i.runtime_active || i.status.eq_ignore_ascii_case("RUNNING"))
            .count();
        let status = if active_strategies > 0 {
            "active".to_owned()
        } else {
            "idle".to_owned()
        };
        let mut active_instances = Vec::new();
        for instance in instances.into_iter().filter(|instance| {
            instance.runtime_active || instance.status.eq_ignore_ascii_case("RUNNING")
        }) {
            let observation = match self.store.get_observation(&instance.id) {
                Ok(observation) => observation,
                Err(_) => {
                    return StrategyRuntimeSummary {
                        status: "failed".to_owned(),
                        active_strategies: 0,
                        supports_backtest_parity: true,
                        active_instances: Vec::new(),
                    };
                }
            };
            let binding_definition_name = instance.definition_name.clone().unwrap_or_else(|| {
                binding_string(&instance.binding, &["definitionName", "strategyName"])
            });
            let binding_symbols = binding_symbols(&instance.binding);
            let actual_status = observation
                .as_ref()
                .map(|item| item.actual_status.trim())
                .filter(|status| !status.is_empty())
                .unwrap_or(instance.status.trim())
                .to_ascii_lowercase();
            active_instances.push(crate::product::StrategyRuntimeActiveInstance {
                instance_id: instance.id,
                definition_name: binding_definition_name,
                actual_status,
                active_symbols: observation
                    .as_ref()
                    .map(|item| item.active_symbols.clone())
                    .or(binding_symbols),
                last_closed_kline_at: observation
                    .as_ref()
                    .and_then(|item| item.last_closed_kline_at.clone()),
                last_signal_at: observation
                    .as_ref()
                    .and_then(|item| item.last_signal_at.clone()),
                last_order_at: observation
                    .as_ref()
                    .and_then(|item| item.last_order_at.clone()),
                last_error_at: observation
                    .as_ref()
                    .and_then(|item| item.last_error_at.clone()),
                last_error: observation
                    .as_ref()
                    .and_then(|item| item.last_error.clone()),
                updated_at: observation
                    .as_ref()
                    .and_then(|item| item.updated_at.clone())
                    .or_else(|| {
                        (!instance.updated_at.is_empty()).then_some(instance.updated_at.clone())
                    }),
            });
        }
        StrategyRuntimeSummary {
            status,
            active_strategies,
            supports_backtest_parity: true,
            active_instances,
        }
    }
}

pub(super) fn binding_string(binding: &Value, keys: &[&str]) -> String {
    keys.iter()
        .filter_map(|key| binding.get(*key).and_then(Value::as_str))
        .map(str::trim)
        .find(|value| !value.is_empty())
        .unwrap_or_default()
        .to_owned()
}

pub(super) fn binding_string_opt(binding: &Value, keys: &[&str]) -> Option<String> {
    let value = binding_string(binding, keys);
    (!value.is_empty()).then_some(value)
}

pub(super) fn binding_symbols(binding: &Value) -> Option<Vec<String>> {
    ["activeSymbols", "symbols"]
        .iter()
        .find_map(|key| {
            let values = binding.get(*key)?.as_array()?;
            Some(
                values
                    .iter()
                    .filter_map(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(ToOwned::to_owned)
                    .collect(),
            )
        })
        .or_else(|| {
            binding
                .get("symbol")
                .and_then(Value::as_str)
                .map(|symbol| vec![symbol.trim().to_owned()])
                .filter(|symbols| !symbols[0].is_empty())
        })
}

pub(super) fn binding_sessions(binding: &Value) -> Vec<String> {
    let Some(value) = binding.get("sessions") else {
        return vec!["regular".to_owned(), "extended".to_owned()];
    };
    let mut sessions = match value {
        Value::Array(values) => values
            .iter()
            .filter_map(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
            .collect::<Vec<_>>(),
        Value::String(value) => value
            .split(',')
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
            .collect::<Vec<_>>(),
        _ => Vec::new(),
    };
    sessions.sort();
    sessions.dedup();
    if sessions.is_empty() {
        vec!["regular".to_owned()]
    } else {
        sessions
    }
}

pub(super) fn binding_params(binding: &Value) -> BTreeMap<String, String> {
    let Some(Value::Object(params)) = binding.get("params") else {
        return BTreeMap::new();
    };
    params
        .iter()
        .filter_map(|(key, value)| {
            let value = match value {
                Value::String(value) => value.clone(),
                Value::Bool(value) => value.to_string(),
                Value::Number(value) => value.to_string(),
                _ => return None,
            };
            Some((key.clone(), value))
        })
        .collect()
}

pub(super) fn normalize_symbol_string(value: &str) -> String {
    let trimmed = value.trim();
    if let Some((market, code)) = trimmed.split_once(':') {
        format!("{}.{}", market.trim().to_ascii_uppercase(), code.trim().to_ascii_uppercase())
    } else if let Some((market, code)) = trimmed.split_once('.') {
        format!("{}.{}", market.trim().to_ascii_uppercase(), code.trim().to_ascii_uppercase())
    } else {
        trimmed.to_ascii_uppercase()
    }
}

pub(super) fn split_strategy_symbol(value: &str, default_market: &str) -> (String, String) {
    value
        .split_once('.')
        .or_else(|| value.split_once(':'))
        .map(|(market, symbol)| (market.trim().to_owned(), symbol.trim().to_owned()))
        .unwrap_or_else(|| (default_market.to_owned(), value.trim().to_owned()))
}

pub(super) fn normalize_strategy_binding(
    binding: &mut Value,
) -> Result<(), StrategyRuntimeWritePortError> {
    let object = binding
        .as_object_mut()
        .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "strategy binding must be an object".to_owned(),
        })?;

    let raw_mode = object.get("executionMode").and_then(Value::as_str).map(str::trim);
    let raw_exec = object.get("executeOrders").and_then(Value::as_bool);

    let (final_mode, final_exec) = match (raw_mode, raw_exec) {
        (None, None) | (Some(""), None) => ("live", true),
        (None, Some(true)) | (Some(""), Some(true)) => ("live", true),
        (None, Some(false)) | (Some(""), Some(false)) => ("notify_only", false),
        (Some("live"), None) => ("live", true),
        (Some("notify_only"), None) => ("notify_only", false),
        (Some("live"), Some(true)) => ("live", true),
        (Some("notify_only"), Some(false)) => ("notify_only", false),
        (Some("live"), Some(false)) => {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "conflicting executionMode 'live' and executeOrders false".to_owned(),
            });
        }
        (Some("notify_only"), Some(true)) => {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "conflicting executionMode 'notify_only' and executeOrders true".to_owned(),
            });
        }
        (Some(other), _) => {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: format!("unsupported executionMode {other:?}"),
            });
        }
    };
    object.insert("executionMode".to_owned(), Value::String(final_mode.to_owned()));
    object.insert("executeOrders".to_owned(), Value::Bool(final_exec));

    let interval = object
        .get("interval")
        .or_else(|| object.get("timeframe"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("5m")
        .to_owned();
    object.insert("interval".to_owned(), Value::String(interval.clone()));
    object.insert("timeframe".to_owned(), Value::String(interval));

    let chart_type = object
        .get("chartType")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("standard")
        .to_owned();
    object.insert("chartType".to_owned(), Value::String(chart_type));

    let default_market = object
        .get("market")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("US")
        .to_owned();

    let mut extracted_symbols: Vec<String> = Vec::new();
    if let Some(symbols_arr) = object
        .get("symbols")
        .or_else(|| object.get("activeSymbols"))
        .and_then(Value::as_array)
    {
        for s in symbols_arr.iter().filter_map(Value::as_str) {
            let normalized = normalize_symbol_string(s);
            if !normalized.is_empty() && !extracted_symbols.contains(&normalized) {
                extracted_symbols.push(normalized);
            }
        }
    } else if let Some(instruments_arr) = object.get("instruments").and_then(Value::as_array) {
        for item in instruments_arr {
            if let Some(s) = item.as_str() {
                let normalized = normalize_symbol_string(s);
                if !normalized.is_empty() && !extracted_symbols.contains(&normalized) {
                    extracted_symbols.push(normalized);
                }
            } else if let Some(obj) = item.as_object() {
                let market = obj.get("market").and_then(Value::as_str).unwrap_or(&default_market);
                if let Some(code) = obj.get("code").and_then(Value::as_str) {
                    let normalized = normalize_symbol_string(&format!("{market}.{code}"));
                    if !normalized.is_empty() && !extracted_symbols.contains(&normalized) {
                        extracted_symbols.push(normalized);
                    }
                }
            }
        }
    } else if let Some(s) = object.get("symbol").and_then(Value::as_str) {
        let normalized = normalize_symbol_string(s);
        if !normalized.is_empty() {
            extracted_symbols.push(normalized);
        }
    }

    if !extracted_symbols.is_empty() {
        object.insert("symbols".to_owned(), json!(extracted_symbols));
        let instruments_json: Vec<Value> = extracted_symbols
            .iter()
            .map(|sym| {
                let (market, code) = split_strategy_symbol(sym, &default_market);
                json!({
                    "market": market,
                    "code": code,
                })
            })
            .collect();
        object.insert("instruments".to_owned(), Value::Array(instruments_json));
    }

    Ok(())
}
