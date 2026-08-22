fn provider_descriptor_wire(
    descriptor: jftrade_marketdata::ProviderDescriptor,
) -> serde_json::Value {
    let mut value = serde_json::to_value(descriptor)
        .expect("validated provider descriptor must be serializable");
    let Some(capabilities) = value
        .get_mut("capabilities")
        .and_then(serde_json::Value::as_object_mut)
    else {
        return value;
    };
    if capabilities
        .get("orderBookLevels")
        .and_then(serde_json::Value::as_array)
        .is_some_and(Vec::is_empty)
    {
        capabilities.insert("orderBookLevels".to_owned(), serde_json::Value::Null);
    }
    if capabilities
        .get("historicalLookbackDays")
        .and_then(serde_json::Value::as_object)
        .is_some_and(serde_json::Map::is_empty)
    {
        capabilities.remove("historicalLookbackDays");
    }
    value
}
