#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireQuery {
    #[serde(default)]
    broker_id: Option<String>,
    #[serde(default)]
    market: Option<String>,
    #[serde(default)]
    pool: Option<WirePool>,
    #[serde(default)]
    conditions: Option<Vec<WireCondition>>,
    #[serde(default)]
    columns: Option<Vec<WireColumn>>,
    #[serde(default)]
    sorts: Option<Vec<WireSort>>,
    #[serde(default)]
    catalog_version: Option<String>,
    #[serde(default)]
    query_schema_version: Option<i64>,
    #[serde(default)]
    account_id: Option<String>,
    #[serde(default)]
    trading_environment: Option<String>,
    #[serde(default)]
    page: Option<WirePage>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePool {
    #[serde(default)]
    watchlist_stock_ids: Option<Vec<String>>,
    #[serde(default)]
    plates: Option<Vec<WirePlate>>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePlate {
    #[allow(dead_code)]
    #[serde(default)]
    parent_plate_id: Option<String>,
    #[serde(default)]
    plate_ids: Option<Vec<String>>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireFactorRef {
    #[serde(default)]
    instance_id: Option<String>,
    #[serde(default)]
    factor_key: Option<String>,
    #[serde(default)]
    params: Option<WireFactorParams>,
}

#[derive(Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireFactorParams {
    #[serde(default)]
    days: Option<i64>,
    #[serde(default)]
    period_average: Option<i64>,
    #[serde(default)]
    term: Option<i64>,
    #[serde(default)]
    duration: Option<i64>,
    #[serde(default)]
    year: Option<i64>,
    #[serde(default)]
    future_duration: Option<i64>,
    #[serde(default)]
    period: Option<i64>,
    #[serde(default)]
    range_period: Option<i64>,
    #[serde(default)]
    first_custom_param: Option<i64>,
    #[serde(default)]
    indicator_params: Option<Vec<i64>>,
    #[serde(default)]
    broker_param: Option<String>,
    #[serde(default)]
    option_param_type: Option<i64>,
    #[serde(default)]
    option_param_string: Option<String>,
    #[serde(default)]
    option_param_integer: Option<i64>,
    #[serde(default)]
    option_param_integers: Option<Vec<i64>>,
    #[serde(default)]
    option_hv_period: Option<i64>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireCondition {
    #[serde(default)]
    id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    operator: Option<String>,
    #[serde(default)]
    value: Option<Value>,
    #[serde(default)]
    second_factor: Option<WireFactorRef>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireColumn {
    #[serde(default)]
    column_id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    label: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WireSort {
    #[serde(default)]
    sort_id: Option<String>,
    #[serde(default)]
    column_id: Option<String>,
    #[serde(default)]
    factor: Option<WireFactorRef>,
    #[serde(default)]
    direction: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct WirePage {
    #[serde(default)]
    offset: Option<i64>,
    #[serde(default)]
    limit: Option<i64>,
}
