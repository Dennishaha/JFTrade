//! Portfolio projections for ADK tools.
//!
//! Provides isolated per-account projections for `portfolio.accounts`,
//! `portfolio.overview`, and `portfolio.positions` without cross-account
//! aggregation, with suffix matching, market authority filtering, and
//! structured broker diagnostics.

use jftrade_integration_futu::{TradeAccountSnapshot, TradeFunds, TradeReadPort, trade_header};
use serde_json::{Value, json};

use crate::product::product_backtest_execution::now_timestamp;
use crate::product::product_production_ports::ProductionPortBundle;
use crate::product::product_production_ports::product_production_ports_trade::ResolvedTradeRequest;
use crate::product::product_production_ports::product_production_ports_trade::market_code;
use crate::product::product_production_ports::product_production_ports_trade::trade_projection::{
    account_value, position_value,
};

fn extract_query_params(arguments: &Value) -> Result<(&str, Option<&str>, Option<&str>), String> {
    let env = arguments
        .get("tradingEnvironment")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .ok_or_else(|| "tradingEnvironment is required".to_owned())?;
    let env_canonical = match env.to_ascii_uppercase().as_str() {
        "REAL" => "REAL",
        "SIMULATE" => "SIMULATE",
        other => return Err(format!("invalid tradingEnvironment: {other}")),
    };
    let market = arguments
        .get("market")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty());
    if let Some(m) = market
        && market_code(m).is_err()
    {
        return Err(format!("unsupported market: {m}"));
    }
    let account_id = arguments
        .get("accountId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|s| !s.is_empty());
    Ok((env_canonical, market, account_id))
}

fn account_supports_market(account: &Value, market: &str) -> bool {
    let empty_vec = Vec::new();
    let authorities = account
        .get("marketAuthorities")
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec);
    authorities.iter().any(|auth| {
        auth.as_str()
            .is_some_and(|a| a.eq_ignore_ascii_case(market))
    })
}

fn select_account_market(
    account: &Value,
    snapshot: &TradeAccountSnapshot,
    explicit_market: Option<&str>,
) -> Result<(String, i32), String> {
    if let Some(market) = explicit_market {
        let code = market_code(market).map_err(|_| format!("unsupported market: {market}"))?;
        return Ok((market.to_ascii_uppercase(), code));
    }
    let empty_vec = Vec::new();
    let authorities = account
        .get("marketAuthorities")
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec);
    if let Some(first) = authorities.first().and_then(Value::as_str)
        && let Ok(code) = market_code(first)
    {
        return Ok((first.to_ascii_uppercase(), code));
    }
    if let Some(&first_code) = snapshot.trd_market_auth_list.first() {
        let label = match first_code {
            1 => "HK",
            2 => "US",
            3 => "CN",
            _ => "HK",
        };
        return Ok((label.to_owned(), first_code));
    }
    Ok(("HK".to_owned(), 1))
}

struct PortfolioResolution {
    status: String,
    mode: String,
    message: String,
    requested_id: Option<String>,
    environment: String,
    market: Option<String>,
    candidates: Vec<Value>,
    target_indices: Vec<usize>,
    selected_account_ids: Vec<String>,
}

fn resolve_portfolio_selection(
    accounts: &[Value],
    target_env: &str,
    target_market: Option<&str>,
    requested_id: Option<&str>,
) -> PortfolioResolution {
    let mut candidate_indices = Vec::new();
    let mut candidate_values = Vec::new();

    for (index, acc) in accounts.iter().enumerate() {
        let env = acc
            .get("tradingEnvironment")
            .and_then(Value::as_str)
            .unwrap_or_default();
        if !env.eq_ignore_ascii_case(target_env) {
            continue;
        }
        if let Some(market) = target_market
            && !account_supports_market(acc, market)
        {
            continue;
        }
        candidate_indices.push(index);
        candidate_values.push(acc.clone());
    }

    if let Some(req_id) = requested_id {
        let exact_match = candidate_values.iter().enumerate().find_map(|(i, c)| {
            let acc_id = c
                .get("accountId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            if acc_id == req_id {
                Some((i, acc_id.to_owned()))
            } else {
                None
            }
        });
        if let Some((i, acc_id)) = exact_match {
            return PortfolioResolution {
                status: "resolved".to_owned(),
                mode: "exact".to_owned(),
                message: String::new(),
                requested_id: Some(req_id.to_owned()),
                environment: target_env.to_owned(),
                market: target_market.map(str::to_owned),
                candidates: candidate_values,
                target_indices: vec![candidate_indices[i]],
                selected_account_ids: vec![acc_id],
            };
        }
        let mut suffix_matches = Vec::new();
        let mut suffix_target_indices = Vec::new();
        let mut suffix_account_ids = Vec::new();
        for (i, c) in candidate_values.iter().enumerate() {
            let acc_id = c
                .get("accountId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            if acc_id.ends_with(req_id) {
                suffix_matches.push(c.clone());
                suffix_target_indices.push(candidate_indices[i]);
                suffix_account_ids.push(acc_id.to_owned());
            }
        }
        if suffix_matches.len() == 1 {
            return PortfolioResolution {
                status: "resolved".to_owned(),
                mode: "unique_suffix".to_owned(),
                message: String::new(),
                requested_id: Some(req_id.to_owned()),
                environment: target_env.to_owned(),
                market: target_market.map(str::to_owned),
                candidates: candidate_values,
                target_indices: suffix_target_indices,
                selected_account_ids: suffix_account_ids,
            };
        }
        if suffix_matches.len() > 1 {
            return PortfolioResolution {
                status: "ambiguous".to_owned(),
                mode: "suffix".to_owned(),
                message: format!(
                    "accountId suffix \"{req_id}\" matched multiple discovered accounts"
                ),
                requested_id: Some(req_id.to_owned()),
                environment: target_env.to_owned(),
                market: target_market.map(str::to_owned),
                candidates: suffix_matches,
                target_indices: Vec::new(),
                selected_account_ids: Vec::new(),
            };
        }
        return PortfolioResolution {
            status: "not_found".to_owned(),
            mode: "none".to_owned(),
            message: format!("accountId \"{req_id}\" did not match a discovered account"),
            requested_id: Some(req_id.to_owned()),
            environment: target_env.to_owned(),
            market: target_market.map(str::to_owned),
            candidates: candidate_values,
            target_indices: Vec::new(),
            selected_account_ids: Vec::new(),
        };
    }

    if candidate_values.is_empty() {
        return PortfolioResolution {
            status: "not_found".to_owned(),
            mode: "none".to_owned(),
            message: "no broker accounts matched the requested environment and market".to_owned(),
            requested_id: None,
            environment: target_env.to_owned(),
            market: target_market.map(str::to_owned),
            candidates: Vec::new(),
            target_indices: Vec::new(),
            selected_account_ids: Vec::new(),
        };
    }

    let all_ids = candidate_values
        .iter()
        .filter_map(|c| {
            c.get("accountId")
                .and_then(Value::as_str)
                .map(str::to_owned)
        })
        .collect();

    PortfolioResolution {
        status: "resolved".to_owned(),
        mode: "all_matching_accounts".to_owned(),
        message: String::new(),
        requested_id: None,
        environment: target_env.to_owned(),
        market: target_market.map(str::to_owned),
        candidates: candidate_values,
        target_indices: candidate_indices,
        selected_account_ids: all_ids,
    }
}

fn load_managed_broker_settings(
    ports: &ProductionPortBundle,
) -> (Vec<Value>, bool, Option<String>) {
    use jftrade_settings::BrokerSettingsStorePort;
    match ports.settings_store.load_broker_settings_inputs() {
        Ok(inputs) => {
            let enabled = inputs
                .saved_integration
                .as_ref()
                .map(|i| i.enabled)
                .unwrap_or(false);
            let managed = inputs
                .accounts
                .into_iter()
                .filter_map(|acc| serde_json::to_value(acc).ok())
                .collect();
            (managed, enabled, None)
        }
        Err(err) => (
            Vec::new(),
            false,
            Some(format!("failed to load broker settings: {err}")),
        ),
    }
}

fn funds_have_assets(funds: &TradeFunds) -> bool {
    funds.total_assets.abs() > 1e-6
        || funds.cash.abs() > 1e-6
        || funds.market_val.abs() > 1e-6
        || funds.power.abs() > 1e-6
        || funds.securities_assets.is_some_and(|v| v.abs() > 1e-6)
        || funds.fund_assets.is_some_and(|v| v.abs() > 1e-6)
        || funds.bond_assets.is_some_and(|v| v.abs() > 1e-6)
        || funds.net_cash_power.is_some_and(|v| v.abs() > 1e-6)
        || funds
            .cash_info_list
            .iter()
            .any(|c| c.cash.is_some_and(|v| v.abs() > 1e-6))
        || funds
            .market_info_list
            .iter()
            .any(|m| m.assets.is_some_and(|v| v.abs() > 1e-6))
}

fn portfolio_base_payload(
    resolution: &PortfolioResolution,
    discovered: &[Value],
    managed: &[Value],
    broker_enabled: bool,
    connectivity: &str,
    last_error: &str,
) -> serde_json::Map<String, Value> {
    let mut base = serde_json::Map::new();
    base.insert("accounts".to_owned(), Value::Array(managed.to_vec()));
    base.insert("managedAccounts".to_owned(), Value::Array(managed.to_vec()));
    base.insert("brokerEnabled".to_owned(), json!(broker_enabled));
    base.insert("checkedAt".to_owned(), json!(now_timestamp()));
    base.insert(
        "discoveredAccounts".to_owned(),
        Value::Array(discovered.to_vec()),
    );
    base.insert(
        "brokerRuntime".to_owned(),
        json!({
            "connectivity": connectivity,
            "lastError": last_error,
        }),
    );
    let mut selection = json!({
        "status": resolution.status,
        "mode": resolution.mode,
        "tradingEnvironment": resolution.environment,
        "candidateAccounts": resolution.candidates,
        "selectedAccountIds": resolution.selected_account_ids,
    });
    if let Some(ref m) = resolution.market {
        selection["market"] = Value::String(m.clone());
    }
    if let Some(ref id) = resolution.requested_id {
        selection["requestedAccountId"] = Value::String(id.clone());
    }
    if !resolution.message.is_empty() {
        selection["message"] = Value::String(resolution.message.clone());
    }
    base.insert("selection".to_owned(), selection);
    base.insert("partial".to_owned(), json!(false));
    base.insert("warnings".to_owned(), json!([]));
    base
}

fn discovery_failed_payload(
    env: &str,
    market: Option<&str>,
    account_id: Option<&str>,
    err: &str,
    managed: &[Value],
    broker_enabled: bool,
) -> Value {
    let mut base = serde_json::Map::new();
    base.insert("accounts".to_owned(), Value::Array(managed.to_vec()));
    base.insert("managedAccounts".to_owned(), Value::Array(managed.to_vec()));
    base.insert("brokerEnabled".to_owned(), json!(broker_enabled));
    base.insert("checkedAt".to_owned(), json!(now_timestamp()));
    base.insert("discoveredAccounts".to_owned(), json!([]));
    base.insert(
        "brokerRuntime".to_owned(),
        json!({
            "connectivity": "disconnected",
            "lastError": err,
        }),
    );
    let mut selection = json!({
        "status": "discovery_failed",
        "mode": "none",
        "message": err,
        "tradingEnvironment": env,
        "candidateAccounts": [],
        "selectedAccountIds": [],
    });
    if let Some(m) = market {
        selection["market"] = Value::String(m.to_owned());
    }
    if let Some(id) = account_id {
        selection["requestedAccountId"] = Value::String(id.to_owned());
    }
    base.insert("selection".to_owned(), selection);
    base.insert("partial".to_owned(), json!(true));
    base.insert("warnings".to_owned(), json!([err]));
    Value::Object(base)
}

pub(crate) fn execute_portfolio_accounts(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let (env, market, requested_id) = extract_query_params(arguments)?;
    let (managed, broker_enabled, settings_error) = load_managed_broker_settings(ports);
    let reader = ports
        .trade_read_port
        .as_ref()
        .ok_or_else(|| "broker trade reader is unavailable".to_owned())?;

    let (_snapshots, discovered) = match reader.read_accounts(0, None, None) {
        Ok(s) => {
            let d = s.iter().cloned().map(account_value).collect::<Vec<_>>();
            (s, d)
        }
        Err(e) => {
            let mut failed = discovery_failed_payload(
                env,
                market,
                requested_id,
                &e.to_string(),
                &managed,
                broker_enabled,
            );
            if let Some(err) = settings_error {
                failed["partial"] = json!(true);
                failed["brokerEnabled"] = json!(false);
                if let Some(w) = failed.get_mut("warnings").and_then(Value::as_array_mut) {
                    w.push(json!(err));
                }
            }
            return Ok(failed);
        }
    };

    let resolution = resolve_portfolio_selection(&discovered, env, market, requested_id);
    let mut base = portfolio_base_payload(
        &resolution,
        &discovered,
        &managed,
        broker_enabled,
        "connected",
        "",
    );
    let mut warnings = Vec::new();
    if resolution.status != "resolved" {
        warnings.push(resolution.message);
    }
    if let Some(err) = settings_error {
        base.insert("brokerEnabled".to_owned(), json!(false));
        warnings.push(err);
    }
    let is_partial = resolution.status != "resolved" || !warnings.is_empty();
    base.insert("partial".to_owned(), json!(is_partial));
    base.insert("warnings".to_owned(), json!(warnings));
    Ok(Value::Object(base))
}

fn read_account_overview_item(
    reader: &dyn TradeReadPort,
    snapshot: &TradeAccountSnapshot,
    account: &Value,
    market: Option<&str>,
) -> (Value, bool, Vec<String>) {
    let mut errors = Vec::new();
    let (query_market, mkt_code) = match select_account_market(account, snapshot, market) {
        Ok(m) => m,
        Err(e) => {
            errors.push(e);
            ("HK".to_owned(), 1)
        }
    };
    let header = trade_header(snapshot.trd_env, snapshot.acc_id, mkt_code);
    let mut position_count = 0;
    let mut order_count = 0;
    let mut has_assets = false;

    match reader.read_funds(header.clone(), None, None, None) {
        Ok(funds) => has_assets = funds_have_assets(&funds.funds),
        Err(e) => errors.push(format!("funds: {e}")),
    }

    match reader.read_positions(header.clone(), None, None, None, None, None, None, None) {
        Ok(positions) => position_count = positions.len(),
        Err(e) => errors.push(format!("positions: {e}")),
    }

    match reader.read_orders(header, None, Vec::new(), None) {
        Ok(orders) => order_count = orders.len(),
        Err(e) => errors.push(format!("orders: {e}")),
    }

    let partial = !errors.is_empty();
    let has_assets_or_positions = has_assets || position_count > 0;
    let item = json!({
        "account": account,
        "queryMarket": query_market,
        "positionCount": position_count,
        "orderCount": order_count,
        "hasAssetsOrPositions": has_assets_or_positions,
        "partial": partial,
        "errors": errors,
    });
    (item, partial, errors)
}

pub(crate) fn execute_portfolio_overview(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let (env, market, requested_id) = extract_query_params(arguments)?;
    let (managed, broker_enabled, settings_error) = load_managed_broker_settings(ports);
    let reader = ports
        .trade_read_port
        .as_ref()
        .ok_or_else(|| "broker trade reader is unavailable".to_owned())?;

    let (snapshots, discovered) = match reader.read_accounts(0, None, None) {
        Ok(s) => {
            let d = s.iter().cloned().map(account_value).collect::<Vec<_>>();
            (s, d)
        }
        Err(e) => {
            let mut failed = discovery_failed_payload(
                env,
                market,
                requested_id,
                &e.to_string(),
                &managed,
                broker_enabled,
            );
            failed["accountOverviews"] = json!([]);
            if let Some(err) = settings_error {
                failed["partial"] = json!(true);
                failed["brokerEnabled"] = json!(false);
                if let Some(w) = failed.get_mut("warnings").and_then(Value::as_array_mut) {
                    w.push(json!(err));
                }
            }
            return Ok(failed);
        }
    };

    let resolution = resolve_portfolio_selection(&discovered, env, market, requested_id);
    let mut base = portfolio_base_payload(
        &resolution,
        &discovered,
        &managed,
        broker_enabled,
        "connected",
        "",
    );
    if resolution.status != "resolved" {
        base.insert("accountOverviews".to_owned(), json!([]));
        base.insert("partial".to_owned(), json!(true));
        let mut warnings = vec![resolution.message];
        if let Some(err) = settings_error {
            base.insert("brokerEnabled".to_owned(), json!(false));
            warnings.push(err);
        }
        base.insert("warnings".to_owned(), json!(warnings));
        return Ok(Value::Object(base));
    }

    let mut overviews = Vec::new();
    let mut any_partial = false;
    let mut warnings = Vec::new();

    for &idx in &resolution.target_indices {
        let (item, partial, errors) =
            read_account_overview_item(reader.as_ref(), &snapshots[idx], &discovered[idx], market);
        if partial {
            any_partial = true;
            let acc_id = discovered[idx]
                .get("accountId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            for err in errors {
                warnings.push(format!("{acc_id}: {err}"));
            }
        }
        overviews.push(item);
    }

    overviews.sort_by(|a, b| {
        let a_has = a
            .get("hasAssetsOrPositions")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let b_has = b
            .get("hasAssetsOrPositions")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        match b_has.cmp(&a_has) {
            std::cmp::Ordering::Equal => {
                let a_id = a
                    .get("account")
                    .and_then(|acc| acc.get("accountId"))
                    .and_then(Value::as_str)
                    .unwrap_or("");
                let b_id = b
                    .get("account")
                    .and_then(|acc| acc.get("accountId"))
                    .and_then(Value::as_str)
                    .unwrap_or("");
                a_id.cmp(b_id)
            }
            other => other,
        }
    });

    if let Some(err) = settings_error {
        base.insert("brokerEnabled".to_owned(), json!(false));
        warnings.push(err);
    }
    let is_partial = any_partial || !warnings.is_empty();
    base.insert("accountOverviews".to_owned(), Value::Array(overviews));
    base.insert("partial".to_owned(), json!(is_partial));
    base.insert("warnings".to_owned(), json!(warnings));
    Ok(Value::Object(base))
}

fn read_account_positions_item(
    reader: &dyn TradeReadPort,
    snapshot: &TradeAccountSnapshot,
    account: &Value,
    market: Option<&str>,
    env: &str,
) -> (Value, bool, Vec<String>) {
    let mut errors = Vec::new();
    let (query_market, mkt_code) = match select_account_market(account, snapshot, market) {
        Ok(m) => m,
        Err(e) => {
            errors.push(e);
            ("HK".to_owned(), 1)
        }
    };
    let header = trade_header(snapshot.trd_env, snapshot.acc_id, mkt_code);
    let mut positions_json = Vec::new();

    match reader.read_positions(header.clone(), None, None, None, None, None, None, None) {
        Ok(positions) => {
            let trade_req = ResolvedTradeRequest {
                account_id: snapshot.acc_id.to_string(),
                environment: env.to_owned(),
                market: query_market.clone(),
                header,
            };
            for pos in positions {
                positions_json.push(position_value(&trade_req, pos));
            }
        }
        Err(e) => errors.push(format!("positions: {e}")),
    }

    let partial = !errors.is_empty();
    let position_count = positions_json.len();
    let has_assets_or_positions = position_count > 0;
    let item = json!({
        "account": account,
        "queryMarket": query_market,
        "positions": positions_json,
        "positionCount": position_count,
        "hasAssetsOrPositions": has_assets_or_positions,
        "partial": partial,
        "errors": errors,
    });
    (item, partial, errors)
}

pub(crate) fn execute_portfolio_positions(
    ports: &ProductionPortBundle,
    arguments: &Value,
) -> Result<Value, String> {
    let (env, market, requested_id) = extract_query_params(arguments)?;
    let (managed, broker_enabled, settings_error) = load_managed_broker_settings(ports);
    let reader = ports
        .trade_read_port
        .as_ref()
        .ok_or_else(|| "broker trade reader is unavailable".to_owned())?;

    let (snapshots, discovered) = match reader.read_accounts(0, None, None) {
        Ok(s) => {
            let d = s.iter().cloned().map(account_value).collect::<Vec<_>>();
            (s, d)
        }
        Err(e) => {
            let mut failed = discovery_failed_payload(
                env,
                market,
                requested_id,
                &e.to_string(),
                &managed,
                broker_enabled,
            );
            failed["accountPositions"] = json!([]);
            if let Some(err) = settings_error {
                failed["partial"] = json!(true);
                failed["brokerEnabled"] = json!(false);
                if let Some(w) = failed.get_mut("warnings").and_then(Value::as_array_mut) {
                    w.push(json!(err));
                }
            }
            return Ok(failed);
        }
    };

    let resolution = resolve_portfolio_selection(&discovered, env, market, requested_id);
    let mut base = portfolio_base_payload(
        &resolution,
        &discovered,
        &managed,
        broker_enabled,
        "connected",
        "",
    );
    if resolution.status != "resolved" {
        base.insert("accountPositions".to_owned(), json!([]));
        base.insert("partial".to_owned(), json!(true));
        let mut warnings = vec![resolution.message];
        if let Some(err) = settings_error {
            base.insert("brokerEnabled".to_owned(), json!(false));
            warnings.push(err);
        }
        base.insert("warnings".to_owned(), json!(warnings));
        return Ok(Value::Object(base));
    }

    let mut account_positions = Vec::new();
    let mut any_partial = false;
    let mut warnings = Vec::new();

    for &idx in &resolution.target_indices {
        let (item, partial, errors) = read_account_positions_item(
            reader.as_ref(),
            &snapshots[idx],
            &discovered[idx],
            market,
            env,
        );
        if partial {
            any_partial = true;
            let acc_id = discovered[idx]
                .get("accountId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            for err in errors {
                warnings.push(format!("{acc_id}: {err}"));
            }
        }
        account_positions.push(item);
    }

    account_positions.sort_by(|a, b| {
        let a_has = a
            .get("hasAssetsOrPositions")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let b_has = b
            .get("hasAssetsOrPositions")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        match b_has.cmp(&a_has) {
            std::cmp::Ordering::Equal => {
                let a_id = a
                    .get("account")
                    .and_then(|acc| acc.get("accountId"))
                    .and_then(Value::as_str)
                    .unwrap_or("");
                let b_id = b
                    .get("account")
                    .and_then(|acc| acc.get("accountId"))
                    .and_then(Value::as_str)
                    .unwrap_or("");
                a_id.cmp(b_id)
            }
            other => other,
        }
    });

    if let Some(err) = settings_error {
        base.insert("brokerEnabled".to_owned(), json!(false));
        warnings.push(err);
    }
    let is_partial = any_partial || !warnings.is_empty();
    base.insert(
        "accountPositions".to_owned(),
        Value::Array(account_positions),
    );
    base.insert("partial".to_owned(), json!(is_partial));
    base.insert("warnings".to_owned(), json!(warnings));
    Ok(Value::Object(base))
}
