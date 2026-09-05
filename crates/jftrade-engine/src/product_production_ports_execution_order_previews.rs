//! Production execution preview operations backed by OpenD trade readers.

use std::sync::Arc;

use jftrade_integration_futu::{
    TradeComboMaxTradeQuantityRequest, TradeHeader, TradeMaxTradeQuantityRequest, TradeReadPort,
    TradeSessionError,
};
use jftrade_store_sqlite::StoredExecutionOrderPreview;
use serde_json::{Value, json};

use super::execution_order_hash::preview_request_hash;
use super::execution_order_helpers::{parse_product_rule_request, product_rule_rejection};
use super::execution_order_parse::{
    ParsedCombo, ParsedOrder, market_label, order_type_label, parse_combo, parse_order,
    parse_order_type, parse_session, quote_market_label, sec_market, side_label, trade_market,
};
use super::*;

#[path = "product_production_ports_execution_order_preview_helpers.rs"]
mod preview_helpers;

use preview_helpers::{
    add_five_minutes, map_trade_error, normalize_event_contract_code, option_strategy_legs,
    option_strategy_name, prediction_side_label, preview_symbol, same_option_strategy_legs,
    underlying_security,
};
pub(super) use preview_helpers::{canonical_combo_legs, jftrade_broker_capability_version};

#[derive(Debug)]
enum EventContractValidationError {
    /// OpenD answered successfully, but the requested contract is absent or
    /// not tradable. ProductRuleProvider exposes this as an allowed=false
    /// decision rather than an infrastructure failure.
    Denied(String),
    /// The prediction reader/session could not answer. Keep this distinct so
    /// callers return the baseline 503 instead of claiming a business denial.
    Unavailable(String),
}

impl ProductionExecutionPort {
    /// Validate and persist a single-order preview after obtaining the broker's
    /// max-trade-quantity snapshot.  The snapshot is intentionally not exposed
    /// on the public wire shape; it is the external evidence that makes the
    /// preview safe to consume later.
    pub(super) fn order_preview(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        // Order previews use the same normalization/validation boundary as
        // placement.  In particular, a missing quantity must remain invalid
        // (the buying-power query has its own optional quantity semantics).
        let parsed = parse_order(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if requires_locked_preview(&parsed) && parsed.client_order_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "clientOrderId is required for derivative and event-contract previews",
            ));
        }
        if parsed.order_kind == "event_single" {
            self.ensure_futu_runtime()?;
            match self.validate_active_event_contracts(std::slice::from_ref(&parsed.symbol)) {
                Ok(()) => {}
                Err(EventContractValidationError::Denied(message)) => {
                    return Err(failed(400, "BAD_REQUEST", message));
                }
                Err(EventContractValidationError::Unavailable(message)) => {
                    return Err(ExecutionWritePortError::Unavailable(message));
                }
            }
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
            response["predictionSide"] =
                Value::String(prediction_side_label(prediction_side).to_owned());
        }
        Ok(response)
    }

    pub(super) fn buying_power_preview(
        &self,
        payload: &Value,
    ) -> Result<Value, ExecutionWritePortError> {
        let default_env = self.default_trading_environment.as_ref().map(|getter| getter());
        let request = parse_product_rule_request(payload, default_env.as_deref())?;
        if let Some((code, message)) = product_rule_rejection(&request) {
            // Product-rule denials are a valid response, but only after the
            // request has crossed the same local validation boundary as Go.
            return Ok(json!({"allowed": false, "reasonCode": code, "reason": message}));
        }
        self.ensure_futu_runtime()?;
        // Product-rule success is only meaningful after the live trade
        // reader has answered.  In particular, do not project the local
        // validation result as `{allowed:true}` merely because OpenD is
        // configured: the max-trade-quantity response is the broker-owned
        // evidence for an ordinary buying-power query.
        let reader = self.reader()?;
        if request.order_kind == "event_single" {
            let instrument = request.instrument_id.clone().unwrap_or_default();
            match self.validate_active_event_contracts(std::slice::from_ref(&instrument)) {
                Ok(()) => {}
                Err(EventContractValidationError::Denied(message)) => {
                    return Ok(json!({
                        "allowed": false,
                        "reasonCode": "EVENT_NOT_TRADABLE",
                        "reason": message,
                    }));
                }
                Err(EventContractValidationError::Unavailable(message)) => {
                    return Err(ExecutionWritePortError::Unavailable(message));
                }
            }
        } else if let Some(request) = product_rule_max_trade_request(&request)? {
            reader
                .read_max_trade_quantity(request)
                .map_err(map_trade_error)?;
        }
        Ok(json!({"allowed": true}))
    }

    pub(super) fn combo_preview(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_combo(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if parsed.order.order_kind == "event_parlay" {
            self.ensure_futu_runtime()?;
            // The RFQ id and expiry are caller-visible metadata, but they are
            // not proof that OpenD can price this parlay.  Require the same
            // real combo-RFQ adapter used by the market-data quote endpoint;
            // otherwise a syntactically valid/fake RFQ would be persisted as
            // an allowed preview.
            let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
                ExecutionWritePortError::Unavailable(
                    "Futu prediction combo RFQ runtime is unavailable".to_owned(),
                )
            })?;
            if !runtime.prediction_combo_quote_available() {
                return Err(ExecutionWritePortError::Unavailable(
                    "Futu prediction combo RFQ adapter is unavailable".to_owned(),
                ));
            }
            let instrument_ids = parsed
                .leg_payloads
                .iter()
                .filter_map(|leg| {
                    leg.get("instrumentId")
                        .or_else(|| leg.get("symbol"))
                        .or_else(|| leg.get("code"))
                        .and_then(Value::as_str)
                        .map(str::to_owned)
                })
                .collect::<Vec<_>>();
            match self.validate_active_event_contracts(&instrument_ids) {
                Ok(()) => {}
                Err(EventContractValidationError::Denied(message)) => {
                    return Err(failed(400, "BAD_REQUEST", message));
                }
                Err(EventContractValidationError::Unavailable(message)) => {
                    return Err(ExecutionWritePortError::Unavailable(message));
                }
            }
            return self.persist_event_combo_preview(payload, &parsed);
        }
        // The Go Futu adapter validates the selected option strategy against
        // OpenD before reading combo buying power.  Keep that same fail-closed
        // boundary here: an installed trade reader without the strategy
        // readers must not manufacture an {allowed:true} combo preview.
        self.validate_option_combo_legality(payload, &parsed)?;
        let maximum = self.read_combo_max_trade_quantity(&parsed)?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let request_hash =
            preview_request_hash(payload, &parsed.order, Some(canonical_combo_legs(&parsed)))?;
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
        if matches!(
            strategy_name.as_str(),
            "vertical" | "strangle" | "butterfly"
        ) {
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
        if !snapshot
            .items
            .iter()
            .any(|item| same_option_strategy_legs(&item.multi_legs, &legs))
        {
            return Err(failed(
                400,
                "ILLEGAL_OPTION_COMBINATION",
                "the selected contracts are not a legal OpenD option strategy for the requested expiries",
            ));
        }
        Ok(())
    }

    /// Validate event-contract lifecycle state through the same typed
    /// prediction reader used by the market-data routes. OpenD's snapshot
    /// response is the source of truth for active/tradable state; an empty or
    /// terminal snapshot is a business denial, while a missing reader/session
    /// remains an infrastructure-unavailable response.
    fn validate_active_event_contracts(
        &self,
        instrument_ids: &[String],
    ) -> Result<(), EventContractValidationError> {
        let runtime = self.trade_runtime.as_ref().ok_or_else(|| {
            EventContractValidationError::Unavailable(
                "Futu prediction market-data runtime is unavailable".to_owned(),
            )
        })?;
        if !runtime.prediction_reader_available() {
            return Err(EventContractValidationError::Unavailable(
                "Futu prediction market-data reader is unavailable".to_owned(),
            ));
        }
        for instrument_id in instrument_ids {
            let code = normalize_event_contract_code(instrument_id).ok_or_else(|| {
                EventContractValidationError::Denied(
                    "futu: event contract code is required".to_owned(),
                )
            })?;
            let path = format!("/api/v1/market-data/prediction/contracts/{code}/snapshot");
            let snapshot = runtime.prediction_read(&path, "").map_err(|error| {
                EventContractValidationError::Unavailable(format!(
                    "Futu event-contract snapshot failed: {error}"
                ))
            })?;
            let entries = snapshot
                .get("entries")
                .and_then(Value::as_array)
                .ok_or_else(|| {
                    EventContractValidationError::Unavailable(
                        "Futu event-contract snapshot response is missing entries".to_owned(),
                    )
                })?;
            let Some(item) = entries.iter().find(|item| {
                item.get("code")
                    .and_then(Value::as_object)
                    .and_then(|value| value.get("code"))
                    .and_then(Value::as_str)
                    .is_some_and(|value| value.trim().eq_ignore_ascii_case(&code))
            }) else {
                return Err(EventContractValidationError::Denied(format!(
                    "futu: event contract snapshot missing {code}"
                )));
            };
            let active = item
                .get("status")
                .and_then(Value::as_i64)
                .is_some_and(|value| value == 2)
                || item
                    .get("status")
                    .and_then(Value::as_str)
                    .is_some_and(|value| value.eq_ignore_ascii_case("EC_STATUS_ACTIVE"));
            if !active {
                let status = item
                    .get("status")
                    .map(Value::to_string)
                    .unwrap_or_else(|| "null".to_owned());
                return Err(EventContractValidationError::Denied(format!(
                    "futu: event contract {code} is not active ({status})"
                )));
            }
        }
        Ok(())
    }

    fn persist_event_combo_preview(
        &self,
        payload: &Value,
        parsed: &ParsedCombo,
    ) -> Result<Value, ExecutionWritePortError> {
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let legs = canonical_combo_legs(parsed);
        let request_hash = preview_request_hash(payload, &parsed.order, Some(legs.clone()))?;
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
                rfq_id: parsed.quote_id.clone(),
                normalized_request: payload.to_string(),
                created_at: now.clone(),
                consumed_at: None,
            })
            .map_err(store_error)?;
        Ok(json!({
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
            "legs": legs,
            "allowed": true,
        }))
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
        analysis.insert(
            "strategy".to_owned(),
            json!(option_strategy_name(snapshot.option_strategy)),
        );
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
            analysis.insert(
                "breakevenPoints".to_owned(),
                json!(snapshot.breakeven_points),
            );
        }
        Ok(Some(Value::Object(analysis)))
    }

    pub(super) fn ensure_futu_runtime(&self) -> Result<(), ExecutionWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
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
    let parsed =
        time::OffsetDateTime::parse(&value, &time::format_description::well_known::Rfc3339)
            .map_err(|error| {
                failed(
                    400,
                    "BAD_REQUEST",
                    format!("quoteExpiresAt is invalid: {error}"),
                )
            })?;
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
    let fallback =
        time::OffsetDateTime::parse(&default, &time::format_description::well_known::Rfc3339)
            .map_err(|error| failed(500, "EXECUTION_TIME_ERROR", error.to_string()))?;
    quote
        .min(fallback)
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

/// Convert the public ProductRuleQuery into the neutral OpenD max-quantity
/// request.  The ProductRule wire shape keeps account identifiers opaque;
/// Futu's typed trade header is numeric, so unresolved account ids are
/// unavailable locally, so the optional probe is skipped instead of sending
/// an unbound request. Queries without an instrument id likewise retain Go's
/// local validation semantics and do not issue a malformed OpenD request.
fn product_rule_max_trade_request(
    request: &execution_order_helpers::ProductRuleRequest,
) -> Result<Option<TradeMaxTradeQuantityRequest>, ExecutionWritePortError> {
    let Some(instrument_id) = request.instrument_id.as_deref() else {
        return Ok(None);
    };
    // ProductRuleQuery deliberately keeps accountId opaque because the Go
    // broker resolves aliases through account discovery.  The neutral Futu
    // TradeReadPort accepts only a numeric OpenD account id, so skip this
    // optional evidence probe when the embedding uses an alias we cannot
    // resolve locally; the live reader was still required above.
    let Some(account_id) = request.account_id.as_deref() else {
        return Ok(None);
    };
    let Ok(account_id) = account_id.parse::<u64>() else {
        return Ok(None);
    };
    let code = instrument_id
        .trim()
        .rsplit_once('.')
        .map_or(instrument_id.trim(), |(_, code)| code)
        .trim()
        .to_ascii_uppercase();
    if code.is_empty() {
        return Err(failed(400, "BAD_REQUEST", "instrumentId is required"));
    }
    let order_type = parse_order_type(&request.order_type)
        .map_err(|message| failed(400, "BAD_REQUEST", message))?;
    let trd_market = trade_market(&request.market);
    if trd_market == 0 {
        return Err(failed(
            400,
            "BAD_REQUEST",
            format!("unsupported market {:?}", request.market),
        ));
    }
    let trading_environment = match request.trading_environment.as_str() {
        "REAL" => 1,
        "SIMULATE" | "SIMULATION" | "PAPER" | "" => 0,
        value => {
            return Err(failed(
                400,
                "BAD_REQUEST",
                format!("unsupported tradingEnvironment {value:?}"),
            ));
        }
    };
    Ok(Some(TradeMaxTradeQuantityRequest {
        header: TradeHeader {
            trd_env: trading_environment,
            acc_id: account_id,
            trd_market,
            jp_acc_type: None,
        },
        order_type,
        code,
        price: request.price.unwrap_or(0.0),
        order_id: None,
        adjust_price: None,
        adjust_side_and_limit: None,
        sec_market: Some(sec_market(trd_market)),
        order_id_ex: None,
        session: parse_session(request.session.as_deref())
            .map_err(|message| failed(400, "BAD_REQUEST", message))?,
        position_id: None,
    }))
}
