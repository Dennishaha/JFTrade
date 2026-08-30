//! Production execution preview operations backed by OpenD trade readers.

use std::sync::Arc;

use jftrade_integration_futu::{
    TradeComboMaxTradeQuantityRequest, TradeMaxTradeQuantityRequest, TradeReadPort,
    TradeSessionError,
};
use jftrade_store_sqlite::StoredExecutionOrderPreview;
use serde_json::{Value, json};

use super::execution_order_helpers::{
    parse_product_rule_request, product_rule_rejection,
};
use super::execution_order_hash::preview_request_hash;
use super::execution_order_parse::{
    ParsedCombo, ParsedOrder, market_label, order_type_label, parse_order, quote_market_label,
    sec_market, side_label,
};
use super::*;

impl ProductionExecutionPort {
    /// Validate and persist a single-order preview after obtaining the broker's
    /// max-trade-quantity snapshot.  The snapshot is intentionally not exposed
    /// on the public wire shape; it is the external evidence that makes the
    /// preview safe to consume later.
    pub(super) fn order_preview(
        &self,
        payload: &Value,
    ) -> Result<Value, ExecutionWritePortError> {
        // Order previews use the same normalization/validation boundary as
        // placement.  In particular, a missing quantity must remain invalid
        // (the buying-power query has its own optional quantity semantics).
        let parsed = parse_order(payload)
            .map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if requires_locked_preview(&parsed) && parsed.client_order_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "clientOrderId is required for derivative and event-contract previews",
            ));
        }
        if parsed.order_kind == "event_single" {
            self.ensure_futu_runtime()?;
            return Err(ExecutionWritePortError::Unavailable(
                "Futu event-contract product-rule adapter is unavailable".to_owned(),
            ));
        }

        // Derivative previews retain the OpenD max-quantity read as external
        // evidence.  Ordinary equity/fund/etc. previews are intentionally
        // local, matching Go's PreviewExecutionOrder which only invokes the
        // ProductRuleProvider for locked derivative/event previews.
        if matches!(parsed.product_class.as_str(), "option" | "future") {
            let _maximum = self.read_max_trade_quantity(&parsed)?;
        }
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let request_hash = preview_request_hash(payload, &parsed, None)?;
        let preview_id = format!("preview-{}", &request_hash[..20]);
        let expires_at = add_five_minutes(&now)?;
        self.store
            .save_preview(&StoredExecutionOrderPreview {
                preview_id: preview_id.clone(),
                request_hash: request_hash.clone(),
                broker_id: parsed.broker_id.clone(),
                capability_version: jftrade_broker_capability_version(),
                account_id: parsed.header.acc_id.to_string(),
                expires_at: expires_at.clone(),
                quote_expires_at: None,
                rfq_id: None,
                normalized_request: payload.to_string(),
                created_at: now.clone(),
                consumed_at: None,
            })
            .map_err(store_error)?;

        let mut response = json!({
            "previewId": preview_id,
            "previewAt": now,
            "expiresAt": expires_at,
            "capabilityVersion": jftrade_broker_capability_version(),
            "brokerId": parsed.broker_id,
            "symbol": preview_symbol(&parsed),
            "side": side_label(parsed.side),
            "orderType": order_type_label(parsed.order_type),
            "quantity": parsed.quantity,
            "price": parsed.price,
            "amount": parsed.amount,
            "productClass": parsed.product_class,
            "orderKind": parsed.order_kind,
            "quantityMode": parsed.quantity_mode,
            "requestHash": request_hash,
            "tradingEnvironment": if parsed.header.trd_env == 1 { "REAL" } else { "SIMULATE" },
            "accountId": parsed.header.acc_id.to_string(),
            "market": parsed.market,
            "previewValid": true,
        });
        if let Some(prediction_side) = parsed.prediction_side {
            response["predictionSide"] = Value::String(prediction_side_label(prediction_side).to_owned());
        }
        Ok(response)
    }

    pub(super) fn buying_power_preview(
        &self,
        payload: &Value,
    ) -> Result<Value, ExecutionWritePortError> {
        let request = parse_product_rule_request(payload)?;
        if let Some((code, message)) = product_rule_rejection(&request) {
            // Product-rule denials are a valid response, but only after the
            // request has crossed the same local validation boundary as Go.
            return Ok(json!({"allowed": false, "reasonCode": code, "reason": message}));
        }
        // A successful approval requires the broker ProductRuleProvider.  Do
        // not manufacture {allowed:true} from local parsing when Futu/OpenD
        // or its product-rule reader is not installed.
        self.ensure_futu_runtime()?;
        Err(ExecutionWritePortError::Unavailable(
            "Futu product-rule adapter is unavailable".to_owned(),
        ))
    }

    pub(super) fn combo_preview(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_combo(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if parsed.order.order_kind == "event_parlay" {
            self.ensure_futu_runtime()?;
            return Err(ExecutionWritePortError::Unavailable(
                "Futu event-contract combo product-rule adapter is unavailable".to_owned(),
            ));
        }
        // The Go Futu adapter validates the selected option strategy against
        // OpenD before reading combo buying power.  Keep that same fail-closed
        // boundary here: an installed trade reader without the strategy
        // readers must not manufacture an {allowed:true} combo preview.
        self.validate_option_combo_legality(payload, &parsed)?;
        let maximum = self.read_combo_max_trade_quantity(&parsed)?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let request_hash = preview_request_hash(
            payload,
            &parsed.order,
            Some(canonical_combo_legs(&parsed)),
        )?;
        let preview_id = format!("preview-{}", &request_hash[..20]);
        let quote_expires_at = combo_quote_expires_at(payload)?;
        let expires_at = preview_expires_at(&now, quote_expires_at.as_deref())?;
        self.store
            .save_preview(&StoredExecutionOrderPreview {
                preview_id: preview_id.clone(),
                request_hash: request_hash.clone(),
                broker_id: parsed.order.broker_id.clone(),
                capability_version: jftrade_broker_capability_version(),
                account_id: parsed.order.header.acc_id.to_string(),
                expires_at: expires_at.clone(),
                quote_expires_at,
                rfq_id: None,
                normalized_request: payload.to_string(),
                created_at: now.clone(),
                consumed_at: None,
            })
            .map_err(store_error)?;
        let mut account_impact = serde_json::Map::new();
        for (name, value) in [
            ("nlvChange", maximum.nlv_change),
            ("initialMarginChange", maximum.initial_margin_change),
            ("maintenanceMarginChange", maximum.maintenance_margin_change),
            ("optionBuyingPower", maximum.option_buy_power),
            ("maxWithdrawalChange", maximum.max_withdraw_change),
            ("buyingPowerDecrease", maximum.buying_power_decrease),
        ] {
            if let Some(value) = value {
                account_impact.insert(name.to_owned(), json!(value));
            }
        }
        let mut response = json!({
            "previewId": preview_id,
            "requestHash": request_hash,
            "previewAt": now,
            "expiresAt": expires_at,
            "capabilityVersion": jftrade_broker_capability_version(),
            "brokerId": parsed.order.broker_id,
            "accountId": parsed.order.header.acc_id.to_string(),
            "market": parsed.order.market,
            "orderKind": parsed.order.order_kind,
            "productClass": parsed.order.product_class,
            "legs": canonical_combo_legs(&parsed),
            "allowed": true,
        });
        if let Some(value) = maximum.buying_power_decrease {
            response["buyingPowerImpact"] = json!(value);
        }
        if !account_impact.is_empty() {
            response["accountImpact"] = Value::Object(account_impact);
        }
        if let Some(option_analysis) = self.option_combo_analysis(&parsed)? {
            response["optionAnalysis"] = option_analysis;
        }
        Ok(response)
    }

    fn validate_option_combo_legality(
        &self,
        payload: &Value,
        parsed: &ParsedCombo,
    ) -> Result<(), ExecutionWritePortError> {
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            ExecutionWritePortError::Unavailable(
                "Futu option strategy legality reader is unavailable".to_owned(),
            )
        })?;
        let strategy = option_strategy_code(payload)?;
        let (market, code) = underlying_security(payload, parsed)?;
        let legs = option_strategy_legs(parsed);
        let strategy_name = payload
            .get("optionStrategy")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase();
        if matches!(strategy_name.as_str(), "vertical" | "strangle" | "butterfly") {
            if !runtime.option_strategy_spread_available() {
                return Err(ExecutionWritePortError::Unavailable(
                    "Futu option strategy spread reader is unavailable".to_owned(),
                ));
            }
            let query = jftrade_integration_futu::OptionStrategySpreadQuery {
                market,
                code: code.clone(),
                option_strategy: strategy,
                expire_time: required_text(payload, "nearExpiry")?,
                far_expire_time: optional_text(payload, "farExpiry"),
                index_option_type: None,
            };
            let snapshot = runtime.option_strategy_spread(&query).map_err(|error| {
                ExecutionWritePortError::Unavailable(format!(
                    "Futu option strategy spread reader failed: {error}"
                ))
            })?;
            let requested = payload
                .get("spread")
                .and_then(Value::as_f64)
                .ok_or_else(|| failed(400, "BAD_REQUEST", "option combo spread is required"))?;
            if !snapshot
                .items
                .iter()
                .any(|item| (item.spread - requested).abs() <= 1e-8)
            {
                return Err(failed(
                    400,
                    "ILLEGAL_OPTION_SPREAD",
                    format!(
                        "spread {requested:.6} is not legal for the selected strategy and expiry"
                    ),
                ));
            }
            return Ok(());
        }
        if !runtime.option_strategy_available() {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu option strategy reader is unavailable".to_owned(),
            ));
        }
        let query = jftrade_integration_futu::OptionStrategyQuery {
            market,
            code,
            option_strategy: strategy,
            expire_time: optional_text(payload, "nearExpiry"),
            far_expire_time: optional_text(payload, "farExpiry"),
            spread: payload.get("spread").and_then(Value::as_f64),
            option_type: None,
            strike_price: None,
            index_option_type: None,
        };
        let snapshot = runtime.option_strategy(&query).map_err(|error| {
            ExecutionWritePortError::Unavailable(format!(
                "Futu option strategy reader failed: {error}"
            ))
        })?;
        if !snapshot.items.iter().any(|item| same_option_strategy_legs(&item.multi_legs, &legs)) {
            return Err(failed(
                400,
                "ILLEGAL_OPTION_COMBINATION",
                "the selected contracts are not a legal OpenD option strategy for the requested expiries",
            ));
        }
        Ok(())
    }

    fn option_combo_analysis(
        &self,
        parsed: &ParsedCombo,
    ) -> Result<Option<Value>, ExecutionWritePortError> {
        let Some(runtime) = self.trade_runtime.as_ref() else {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu option strategy analysis reader is unavailable".to_owned(),
            ));
        };
        if !runtime.option_strategy_analysis_available() {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu option strategy analysis reader is unavailable".to_owned(),
            ));
        }
        let snapshot = runtime
            .option_strategy_analysis(&jftrade_integration_futu::OptionStrategyAnalysisQuery {
                multi_legs: option_strategy_legs(parsed),
            })
            .map_err(|error| {
                ExecutionWritePortError::Unavailable(format!(
                    "Futu option strategy analysis reader failed: {error}"
                ))
            })?;
        let mut analysis = serde_json::Map::new();
        analysis.insert("strategy".to_owned(), json!(option_strategy_name(snapshot.option_strategy)));
        for (key, value) in [
            ("bid", snapshot.bid1),
            ("ask", snapshot.ask1),
            ("maxProfit", snapshot.max_profit),
            ("maxLoss", snapshot.max_loss),
            ("probability", snapshot.prob_of_profit),
            ("delta", snapshot.delta),
            ("theta", snapshot.theta),
        ] {
            if let Some(value) = value {
                analysis.insert(key.to_owned(), json!(value));
            }
        }
        if !snapshot.breakeven_points.is_empty() {
            analysis.insert("breakevenPoints".to_owned(), json!(snapshot.breakeven_points));
        }
        Ok(Some(Value::Object(analysis)))
    }

    pub(super) fn ensure_futu_runtime(&self) -> Result<(), ExecutionWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu is not the active market-data provider".to_owned(),
            ));
        }
        if !snapshot.opend_ready {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu OpenD runtime is not ready".to_owned(),
            ));
        }
        Ok(())
    }

    fn reader(&self) -> Result<Arc<dyn TradeReadPort>, ExecutionWritePortError> {
        self.ensure_futu_runtime()?;
        if let Some(runtime) = self.trade_runtime.as_ref() {
            let snapshot = runtime.snapshot();
            if snapshot.trade_logged_in != Some(true) {
                return Err(ExecutionWritePortError::Unavailable(
                    "Futu trade session login/account not ready".to_owned(),
                ));
            }
            return snapshot.client.ok_or_else(|| {
                ExecutionWritePortError::Unavailable(
                    "Futu OpenD trade read client is unavailable".to_owned(),
                )
            });
        }
        if self.trade_logged_in != Some(true) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu trade session login/account not ready".to_owned(),
            ));
        }
        self.trade_read_port.clone().ok_or_else(|| {
            ExecutionWritePortError::Unavailable(
                "Futu OpenD trade read client is unavailable".to_owned(),
            )
        })
    }

    pub(super) fn read_max_trade_quantity(
        &self,
        parsed: &ParsedOrder,
    ) -> Result<jftrade_integration_futu::TradeMaxTradeQuantitySnapshot, ExecutionWritePortError>
    {
        self.reader()?
            .read_max_trade_quantity(TradeMaxTradeQuantityRequest {
                header: parsed.header.clone(),
                order_type: parsed.order_type,
                code: parsed.code.clone(),
                price: parsed.price.unwrap_or(0.0),
                order_id: None,
                adjust_price: None,
                adjust_side_and_limit: None,
                sec_market: Some(sec_market(parsed.header.trd_market)),
                order_id_ex: None,
                session: parsed.session,
                position_id: None,
            })
            .map_err(map_trade_error)
    }

    pub(super) fn read_combo_max_trade_quantity(
        &self,
        parsed: &ParsedCombo,
    ) -> Result<jftrade_integration_futu::TradeComboMaxTradeQuantitySnapshot, ExecutionWritePortError>
    {
        self.reader()?
            .read_combo_max_trade_quantity(TradeComboMaxTradeQuantityRequest {
                header: parsed.order.header.clone(),
                combo_legs: parsed.legs.clone(),
                quantity: parsed.combo_quantity(),
                price: parsed.order.price,
                order_type: parsed.order.order_type,
                order_id_ex: None,
            })
            .map_err(map_trade_error)
    }
}

fn required_text(payload: &Value, key: &str) -> Result<String, ExecutionWritePortError> {
    payload
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .ok_or_else(|| failed(400, "BAD_REQUEST", format!("{key} is required")))
}

fn optional_text(payload: &Value, key: &str) -> Option<String> {
    payload
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn combo_quote_expires_at(payload: &Value) -> Result<Option<String>, ExecutionWritePortError> {
    let Some(value) = optional_text(payload, "quoteExpiresAt") else {
        return Ok(None);
    };
    let parsed = time::OffsetDateTime::parse(
        &value,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| failed(400, "BAD_REQUEST", format!("quoteExpiresAt is invalid: {error}")))?;
    parsed
        .format(&time::format_description::well_known::Rfc3339)
        .map(Some)
        .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))
}

fn preview_expires_at(
    now: &str,
    quote_expires_at: Option<&str>,
) -> Result<String, ExecutionWritePortError> {
    let default = add_five_minutes(now)?;
    let Some(quote_expires_at) = quote_expires_at else {
        return Ok(default);
    };
    let quote = time::OffsetDateTime::parse(
        quote_expires_at,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))?;
    let fallback = time::OffsetDateTime::parse(
        &default,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))?;
    quote.min(fallback)
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))
}

fn option_strategy_code(payload: &Value) -> Result<i32, ExecutionWritePortError> {
    let strategy = payload
        .get("optionStrategy")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_ascii_lowercase();
    match strategy.as_str() {
        "vertical" => Ok(4),
        "straddle" => Ok(6),
        "strangle" => Ok(7),
        "butterfly" => Ok(9),
        "calendar" => Ok(15),
        _ => Err(failed(
            400,
            "BAD_REQUEST",
            format!("unsupported optionStrategy {strategy:?}"),
        )),
    }
}

fn underlying_security(
    payload: &Value,
    parsed: &ParsedCombo,
) -> Result<(i32, String), ExecutionWritePortError> {
    let instrument = payload
        .get("underlyingInstrumentId")
        .and_then(Value::as_str)
        .unwrap_or(&parsed.order.symbol);
    let (market, code) = instrument.trim().rsplit_once('.').map_or_else(
        || (market_label(parsed.order.header.trd_market), instrument.trim().to_owned()),
        |(market, code)| (market.to_owned(), code.to_owned()),
    );
    let market = match market.trim().to_ascii_uppercase().as_str() {
        "US" => 11,
        "HK" => 1,
        _ => {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "option combo underlying market must be US or HK",
            ));
        }
    };
    let code = code.trim().to_ascii_uppercase();
    if code.is_empty() {
        return Err(failed(400, "BAD_REQUEST", "option combo underlying code is required"));
    }
    Ok((market, code))
}

fn option_strategy_legs(
    parsed: &ParsedCombo,
) -> Vec<jftrade_integration_futu::OptionStrategyLeg> {
    parsed
        .legs
        .iter()
        .map(|leg| {
            let market = quote_market_label(leg.market);
            let code = leg.code.trim().to_ascii_uppercase();
            jftrade_integration_futu::OptionStrategyLeg {
                security: jftrade_integration_futu::OptionStrategySecurity {
                    market: market.to_owned(),
                    code: code.clone(),
                    quote_market: market.to_owned(),
                    trade_market: market.to_owned(),
                    instrument_id: format!("{market}.{code}"),
                },
                side: leg.side,
                qty_ratio: leg.qty_ratio,
                position_id: leg.position_id,
                pred_side: leg.pred_side,
            }
        })
        .collect()
}

fn same_option_strategy_legs(
    left: &[jftrade_integration_futu::OptionStrategyLeg],
    right: &[jftrade_integration_futu::OptionStrategyLeg],
) -> bool {
    if left.len() != right.len() {
        return false;
    }
    let mut left = left
        .iter()
        .map(option_strategy_leg_key)
        .collect::<Vec<_>>();
    let mut right = right
        .iter()
        .map(option_strategy_leg_key)
        .collect::<Vec<_>>();
    left.sort();
    right.sort();
    left == right
}

fn option_strategy_leg_key(leg: &jftrade_integration_futu::OptionStrategyLeg) -> String {
    format!(
        "{}|{}|{}",
        leg.security.code.trim().to_ascii_uppercase(),
        leg.side.unwrap_or_default(),
        leg.qty_ratio.unwrap_or_default(),
    )
}

fn option_strategy_name(strategy: i32) -> &'static str {
    match strategy {
        4 => "vertical",
        6 => "straddle",
        7 => "strangle",
        9 => "butterfly",
        15 => "calendar",
        _ => "",
    }
}

pub(super) fn canonical_combo_legs(parsed: &ParsedCombo) -> Value {
    Value::Array(
        parsed
            .legs
            .iter()
            .enumerate()
            .map(|(index, leg)| {
                // Combo normalization in Go upper-cases the supplied
                // instrument id but does not rewrite it to a market prefix.
                // Preserve that exact public value (including SH/SZ and
                // unqualified symbols) instead of the lossy OpenD market enum.
                let raw = parsed
                    .leg_payloads
                    .get(index)
                    .and_then(Value::as_object);
                let instrument = raw
                    .and_then(|object| {
                        ["instrumentId", "symbol", "code"]
                            .into_iter()
                            .find_map(|key| {
                                object
                                    .get(key)
                                    .and_then(Value::as_str)
                                    .map(str::trim)
                                    .filter(|value| !value.is_empty())
                            })
                    })
                    .map(|value| value.to_ascii_uppercase())
                    .unwrap_or_else(|| {
                        format!("{}.{}", quote_market_label(leg.market), leg.code.trim())
                            .to_ascii_uppercase()
                    });
                let ratio = leg
                    .qty_ratio
                    .filter(|value| value.is_finite() && value.fract() == 0.0)
                    .map(|value| value as i64);
                let mut value = json!({
                    "instrumentId": instrument,
                    "productClass": parsed.order.product_class.clone(),
                    "side": leg.side.map(side_label).unwrap_or("UNKNOWN"),
                    "ratio": ratio,
                });
                if let Some(object) = raw {
                    for key in ["quantity", "amount", "price"] {
                        if let Some(number) = object.get(key).filter(|value| !value.is_null()) {
                            value[key] = number.clone();
                        }
                    }
                    if let Some(prediction_side) = object
                        .get("predictionSide")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                    {
                        value["predictionSide"] =
                            Value::String(prediction_side.to_ascii_uppercase());
                    }
                } else if let Some(prediction_side) = leg.pred_side {
                    value["predictionSide"] =
                        Value::String(prediction_side_label(prediction_side).to_owned());
                }
                value
            })
            .collect(),
    )
}

fn preview_symbol(parsed: &ParsedOrder) -> String {
    parsed.symbol.trim().to_ascii_uppercase()
}

fn prediction_side_label(value: i32) -> &'static str {
    match value {
        1 => "YES",
        2 => "NO",
        _ => "UNKNOWN",
    }
}

pub(super) fn add_five_minutes(timestamp: &str) -> Result<String, ExecutionWritePortError> {
    let parsed =
        time::OffsetDateTime::parse(timestamp, &time::format_description::well_known::Rfc3339)
            .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))?;
    (parsed + time::Duration::minutes(5))
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))
}

pub(super) fn jftrade_broker_capability_version() -> String {
    "2026-07-17.opend-10.9.6908".to_owned()
}

fn map_trade_error(error: TradeSessionError) -> ExecutionWritePortError {
    super::execution_order_helpers::map_trade_error(error)
}
