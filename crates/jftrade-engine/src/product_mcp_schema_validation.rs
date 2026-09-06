pub(crate) fn validate_arguments(name: &str, arguments: &Value) -> Result<(), String> {
    validate_schema(&schema_for(name), arguments, "arguments")
}

fn validate_schema(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    validate_composition(schema, value, path)?;
    validate_type(schema, value, path)?;
    validate_enum_and_const(schema, value, path)?;
    validate_object(schema, value, path)?;
    validate_array(schema, value, path)?;
    validate_string(schema, value, path)?;
    validate_number(schema, value, path)
}

fn validate_composition(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    if let Some(condition) = schema.get("if")
        && validate_schema(condition, value, path).is_ok()
        && let Some(then_schema) = schema.get("then")
    {
        validate_schema(then_schema, value, path)?;
    } else if schema.get("if").is_some()
        && let Some(else_schema) = schema.get("else")
    {
        validate_schema(else_schema, value, path)?;
    }
    if let Some(branches) = schema.get("anyOf").and_then(Value::as_array)
        && !branches
            .iter()
            .any(|branch| validate_schema(branch, value, path).is_ok())
    {
        return Err(format!("{path} does not match any allowed schema"));
    }
    if let Some(branches) = schema.get("oneOf").and_then(Value::as_array) {
        let count = branches
            .iter()
            .filter(|branch| validate_schema(branch, value, path).is_ok())
            .count();
        if count != 1 {
            return Err(format!("{path} does not match exactly one allowed schema"));
        }
    }
    if let Some(branches) = schema.get("allOf").and_then(Value::as_array) {
        for branch in branches {
            validate_schema(branch, value, path)?;
        }
    }
    Ok(())
}

fn validate_type(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    let Some(expected) = schema.get("type").and_then(Value::as_str) else {
        return Ok(());
    };
    let matches = match expected {
        "object" => value.is_object(),
        "array" => value.is_array(),
        "string" => value.is_string(),
        "integer" => value.as_i64().is_some() || value.as_u64().is_some(),
        "number" => value.is_number(),
        "boolean" => value.is_boolean(),
        "null" => value.is_null(),
        _ => false,
    };
    if matches {
        Ok(())
    } else {
        Err(format!("{path} must be {expected}"))
    }
}

fn validate_enum_and_const(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    if let Some(expected) = schema.get("const")
        && value != expected
    {
        return Err(format!("{path} must equal {expected}"));
    }
    if let Some(values) = schema.get("enum").and_then(Value::as_array)
        && !values.contains(value)
    {
        return Err(format!("{path} is not an allowed value"));
    }
    Ok(())
}

fn validate_object(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    let Some(object) = value.as_object() else {
        return Ok(());
    };
    if let Some(required) = schema.get("required").and_then(Value::as_array) {
        for key in required.iter().filter_map(Value::as_str) {
            if !object.contains_key(key) {
                return Err(format!("{path}.{key} is required"));
            }
        }
    }
    let properties = schema.get("properties").and_then(Value::as_object);
    if schema.get("additionalProperties") == Some(&Value::Bool(false)) {
        for key in object.keys() {
            if !properties.is_some_and(|known| known.contains_key(key)) {
                return Err(format!("{path}.{key} is not allowed"));
            }
        }
    }
    if let Some(properties) = properties {
        for (key, property_schema) in properties {
            if let Some(property_value) = object.get(key) {
                validate_schema(property_schema, property_value, &format!("{path}.{key}"))?;
            }
        }
    }
    Ok(())
}

fn validate_array(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    let Some(values) = value.as_array() else {
        return Ok(());
    };
    validate_size(schema, values.len(), path, "minItems", "maxItems")?;
    if let Some(item_schema) = schema.get("items") {
        for (index, item) in values.iter().enumerate() {
            validate_schema(item_schema, item, &format!("{path}[{index}]"))?;
        }
    }
    Ok(())
}

fn validate_string(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    let Some(value) = value.as_str() else {
        return Ok(());
    };
    validate_size(
        schema,
        value.chars().count(),
        path,
        "minLength",
        "maxLength",
    )
}

fn validate_size(
    schema: &Value,
    actual: usize,
    path: &str,
    minimum_key: &str,
    maximum_key: &str,
) -> Result<(), String> {
    if let Some(minimum) = schema.get(minimum_key).and_then(Value::as_u64)
        && actual < minimum as usize
    {
        return Err(format!("{path} must contain at least {minimum} items"));
    }
    if let Some(maximum) = schema.get(maximum_key).and_then(Value::as_u64)
        && actual > maximum as usize
    {
        return Err(format!("{path} must contain at most {maximum} items"));
    }
    Ok(())
}

fn validate_number(schema: &Value, value: &Value, path: &str) -> Result<(), String> {
    let Some(actual) = value.as_f64() else {
        return Ok(());
    };
    if let Some(minimum) = schema.get("minimum").and_then(Value::as_f64)
        && actual < minimum
    {
        return Err(format!("{path} must be at least {minimum}"));
    }
    if let Some(maximum) = schema.get("maximum").and_then(Value::as_f64)
        && actual > maximum
    {
        return Err(format!("{path} must be at most {maximum}"));
    }
    if let Some(minimum) = schema.get("exclusiveMinimum").and_then(Value::as_f64)
        && actual <= minimum
    {
        return Err(format!("{path} must be greater than {minimum}"));
    }
    Ok(())
}
