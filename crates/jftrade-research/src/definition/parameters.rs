use super::*;

pub(super) fn validate_params(
    path: &str,
    catalog: &Value,
    factor: &Map<String, Value>,
    params: &FactorParams,
) -> Result<(), DefinitionFieldError> {
    let values = params_value(params);
    let parameters = factor
        .get("parameters")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or_default();
    for parameter in parameters {
        let Some(name) = parameter.get("name").and_then(Value::as_str) else {
            continue;
        };
        if parameter.get("type").and_then(Value::as_str) == Some("union") {
            validate_union_parameter(&format!("{path}.{name}"), name, &values)?;
            continue;
        }
        let value = values.get(name);
        if value.is_none_or(is_missing_json_value) {
            let required = parameter.get("required").and_then(Value::as_bool) == Some(true);
            let has_default = parameter
                .get("default")
                .is_some_and(|value| !value.is_null());
            if required && !has_default {
                return Err(issue(
                    format!("{path}.{name}"),
                    "required",
                    "parameter is required",
                ));
            }
            continue;
        }
        validate_parameter_value(
            &format!("{path}.{name}"),
            parameter,
            value.expect("checked above"),
            catalog,
        )?;
    }
    Ok(())
}

fn validate_parameter_value(
    path: &str,
    parameter: &Value,
    value: &Value,
    catalog: &Value,
) -> Result<(), DefinitionFieldError> {
    let parameter_type = parameter
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if parameter_type == "string" {
        return if value.is_string() {
            Ok(())
        } else {
            Err(issue(path, "invalid_type", "must be a string"))
        };
    }
    let values: Vec<&Value> = if matches!(parameter_type, "integer_array" | "number_array") {
        value
            .as_array()
            .map(|array| array.iter().collect())
            .ok_or_else(|| issue(path, "invalid_type", "must be an array"))?
    } else {
        vec![value]
    };
    for item in values {
        let Some(number) = item.as_f64().filter(|number| number.is_finite()) else {
            return Err(issue(
                path,
                "invalid_type",
                "must contain only finite numbers",
            ));
        };
        let enum_name = parameter
            .get("enum")
            .and_then(Value::as_str)
            .unwrap_or_default();
        if (matches!(parameter_type, "integer" | "integer_array") || !enum_name.is_empty())
            && number.fract() != 0.0
        {
            return Err(issue(path, "invalid_type", "must contain only integers"));
        }
        if let Some(minimum) = parameter.get("minimum").and_then(Value::as_f64)
            && number < minimum
        {
            return Err(issue(
                path,
                "minimum",
                format!("must be at least {}", minimum as i64),
            ));
        }
        if let Some(maximum) = parameter.get("maximum").and_then(Value::as_f64)
            && number > maximum
        {
            return Err(issue(
                path,
                "maximum",
                format!("must be at most {}", maximum as i64),
            ));
        }
        if let Some(step) = parameter.get("step").and_then(Value::as_f64)
            && step > 0.0
        {
            let base = parameter
                .get("minimum")
                .and_then(Value::as_f64)
                .unwrap_or(0.0);
            let remainder = (number - base).abs() % step;
            if remainder > 1e-9 && (remainder - step).abs() > 1e-9 {
                return Err(issue(path, "step", format!("must use step {step}")));
            }
        }
        if !enum_name.is_empty() && !enum_contains(catalog, enum_name, number as i64) {
            return Err(issue(
                path,
                "invalid_enum",
                format!("must be a valid {enum_name} value"),
            ));
        }
    }
    Ok(())
}

fn validate_union_parameter(
    path: &str,
    name: &str,
    values: &Map<String, Value>,
) -> Result<(), DefinitionFieldError> {
    if name != "optionParam" {
        return Err(issue(
            path,
            "unsupported_union",
            "unsupported union parameter",
        ));
    }
    let Some(parameter_type) = values.get("optionParamType") else {
        if values.contains_key("optionParamString")
            || values.contains_key("optionParamInteger")
            || values.contains_key("optionParamIntegers")
        {
            return Err(issue(
                format!("{path}.type"),
                "required",
                "union type is required",
            ));
        }
        return Ok(());
    };
    let Some(parameter_type) = parameter_type.as_f64().filter(|value| value.fract() == 0.0) else {
        return Err(issue(
            format!("{path}.type"),
            "invalid_type",
            "union type must be an integer",
        ));
    };
    match parameter_type as i64 {
        1 if values
            .get("optionParamString")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty()) =>
        {
            Ok(())
        }
        1 => Err(issue(
            format!("{path}.string"),
            "required",
            "string value is required for union type 1",
        )),
        2 if values
            .get("optionParamInteger")
            .and_then(Value::as_f64)
            .is_some() =>
        {
            Ok(())
        }
        2 => Err(issue(
            format!("{path}.integer"),
            "required",
            "integer value is required for union type 2",
        )),
        3 => {
            let array = values.get("optionParamIntegers").and_then(Value::as_array);
            if array.is_none_or(Vec::is_empty) {
                return Err(issue(
                    format!("{path}.integers"),
                    "required",
                    "integer array is required for union type 3",
                ));
            }
            if array
                .expect("checked above")
                .iter()
                .any(|item| item.as_f64().is_none_or(|number| number.fract() != 0.0))
            {
                return Err(issue(
                    format!("{path}.integers"),
                    "invalid_type",
                    "integer array must contain only integers",
                ));
            }
            Ok(())
        }
        _ => Err(issue(
            format!("{path}.type"),
            "invalid_enum",
            "union type must be 1, 2 or 3",
        )),
    }
}

pub(super) fn normalize_factor_params(factor_ref: &FactorRef) -> FactorParams {
    let Ok(catalog) = normalization_catalog(FUTU_CATALOG_VERSION, "") else {
        return factor_ref.params.clone();
    };
    let key = factor_ref.factor_key.trim().to_ascii_lowercase();
    let Some(factor) = find_factor(catalog, &key) else {
        return factor_ref.params.clone();
    };
    let mut values = params_value(&factor_ref.params);
    if let Some(parameters) = factor.get("parameters").and_then(Value::as_array) {
        for parameter in parameters {
            let Some(name) = parameter.get("name").and_then(Value::as_str) else {
                continue;
            };
            let default = parameter.get("default").filter(|value| !value.is_null());
            if !values.contains_key(name)
                && let Some(default) = default
            {
                values.insert(name.to_owned(), default.clone());
            }
        }
    }
    serde_json::from_value(Value::Object(values)).unwrap_or_else(|_| factor_ref.params.clone())
}
