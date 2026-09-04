impl ProductionExecutionPort {
    fn apply_broker_snapshot(
        &self,
        current: &StoredExecutionOrder,
        snapshot: &jftrade_integration_futu::TradeOrderSnapshot,
        expected_revision: u64,
    ) -> Result<bool, ExecutionWritePortError> {
        let incoming = canonical_broker_status(order_status_label(snapshot.order_status));
        if incoming == OrderStatus::Unknown {
            return Err(failed(
                502,
                "BROKER_STATUS_UNKNOWN",
                format!(
                    "OpenD returned unknown order status {} for broker order {}",
                    snapshot.order_status, snapshot.order_id
                ),
            ));
        }
        let stored_current = canonical_stored_status(&current.status);
        let accepted = if current.status.eq_ignore_ascii_case("CANCEL_SUBMITTED") {
            matches!(
                incoming,
                OrderStatus::Filled
                    | OrderStatus::Cancelled
                    | OrderStatus::Rejected
                    | OrderStatus::Expired
            )
        } else {
            reconcile_status(stored_current, incoming).1
        };
        if !accepted {
            return Ok(false);
        }
        let mut next = current.clone();
        next.status = storage_status(incoming, current.status.as_str());
        if next.broker_id.trim().is_empty() {
            next.broker_id = "futu".to_owned();
        }
        next.raw_broker_status = Some(snapshot.order_status.to_string());
        if snapshot.order_id > 0 {
            next.broker_order_id = Some(snapshot.order_id.to_string());
        }
        if !snapshot.order_id_ex.trim().is_empty() {
            next.broker_order_id_ex = Some(snapshot.order_id_ex.clone());
        }
        if snapshot
            .fill_qty
            .is_some_and(|value| value.is_finite() && value >= 0.0)
            && snapshot
                .fill_qty
                .zip(next.filled_quantity)
                .is_none_or(|(incoming, existing)| incoming >= existing)
        {
            next.filled_quantity = snapshot.fill_qty;
        }
        if snapshot
            .fill_avg_price
            .is_some_and(|value| value.is_finite() && value >= 0.0)
        {
            next.filled_average_price = snapshot.fill_avg_price;
        }
        if snapshot.qty.is_finite() && snapshot.qty > 0.0 {
            next.requested_quantity = Some(snapshot.qty);
        }
        if snapshot
            .price
            .is_some_and(|value| value.is_finite() && value >= 0.0)
        {
            next.requested_price = snapshot.price;
        }
        if !snapshot.code.trim().is_empty() {
            next.symbol = Some(if snapshot.code.contains('.') {
                snapshot.code.trim().to_ascii_uppercase()
            } else {
                format!(
                    "{}.{}",
                    next.market,
                    snapshot.code.trim().to_ascii_uppercase()
                )
            });
        }
        if !snapshot
            .remark
            .as_deref()
            .unwrap_or_default()
            .trim()
            .is_empty()
        {
            next.remark = snapshot.remark.clone();
        }
        next.side = Some(execution_order_parse::side_label(snapshot.trd_side).to_owned());
        next.order_type =
            Some(execution_order_parse::order_type_label(snapshot.order_type).to_owned());
        next.last_error = snapshot.last_err_msg.clone();
        next.last_error_code = None;
        next.last_error_source = snapshot.last_err_msg.as_ref().map(|_| "opend".to_owned());
        if next.status == current.status
            && next.broker_id == current.broker_id
            && next.broker_order_id == current.broker_order_id
            && next.broker_order_id_ex == current.broker_order_id_ex
            && next.raw_broker_status == current.raw_broker_status
            && next.symbol == current.symbol
            && next.side == current.side
            && next.order_type == current.order_type
            && next.requested_quantity == current.requested_quantity
            && next.requested_price == current.requested_price
            && next.filled_quantity == current.filled_quantity
            && next.filled_average_price == current.filled_average_price
            && next.remark == current.remark
            && next.last_error == current.last_error
        {
            return Ok(false);
        }
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        next.updated_at = now.clone();
        let event_id = format!(
            "{}-reconcile-{}",
            current.internal_order_id,
            self.store.next_sequence("order-event").map_err(store_error)?
        );
        let next_status = next.status.clone();
        let payload_json = json!({
            "brokerOrderId": snapshot.order_id,
            "brokerOrderIdEx": snapshot.order_id_ex,
            "brokerStatus": snapshot.order_status,
            "filledQuantity": snapshot.fill_qty,
            "filledAveragePrice": snapshot.fill_avg_price,
            "updatedAt": snapshot.update_time,
        })
        .to_string();
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &current.internal_order_id,
            event_type: "reconciled",
            previous_status: Some(current.status.as_str()),
            next_status: &next_status,
            payload_json: &payload_json,
            created_at: &now,
        };
        self.store
            .transition_order_and_event_fenced(
                next,
                &now,
                &event,
                current.status.as_str(),
                current.updated_at.as_str(),
                Some(expected_revision),
            )
            .map(|_| true)
            .map_err(map_transition_store_error)
    }

    fn writer(&self) -> Result<Arc<dyn TradeWritePort>, ExecutionWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if !snapshot.opend_ready {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu OpenD runtime is not ready".to_owned(),
            ));
        }
        let trade_logged_in = self
            .trade_runtime
            .as_ref()
            .map_or(self.trade_logged_in, |runtime| {
                runtime.snapshot().trade_logged_in
            });
        if trade_logged_in != Some(true) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu trade account is not logged in".to_owned(),
            ));
        }
        if let Some(runtime) = self.trade_runtime.as_ref() {
            return runtime.writer_snapshot().ok_or_else(|| {
                ExecutionWritePortError::Unavailable(
                    "OpenD trade runtime is unavailable".to_owned(),
                )
            });
        }
        self.trade_write_port.clone().ok_or_else(|| {
            ExecutionWritePortError::Unavailable("OpenD trade runtime is unavailable".to_owned())
        })
    }

    fn place_order(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let default_env = self
            .default_trading_environment
            .as_ref()
            .map(|getter| getter());
        let parsed = parse_order_with_defaults(payload, default_env.as_deref())
            .map_err(|message| failed(400, "BAD_REQUEST", message))?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        if requires_locked_preview(&parsed) {
            if parsed.preview_id.is_none() {
                return Err(failed(
                    400,
                    "BAD_REQUEST",
                    "previewId is required for derivative and event-contract orders",
                ));
            }
            if parsed.client_order_id.is_none() {
                return Err(failed(
                    400,
                    "BAD_REQUEST",
                    "clientOrderId is required for idempotent derivative and event-contract submission",
                ));
            }
        } else if parsed.preview_id.is_some() && parsed.client_order_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "clientOrderId is required when previewId is supplied",
            ));
        }
        let request_hash = preview_request_hash(payload, &parsed, None)?;
        if let Some(client_order_id) = parsed.client_order_id.as_deref()
            && let Some(existing) = self
                .store
                .find_order_by_client_identity(
                    &parsed.broker_id,
                    if parsed.header.trd_env == 1 { "REAL" } else { "SIMULATE" },
                    &parsed.header.acc_id.to_string(),
                    client_order_id,
                )
                .map_err(store_error)?
        {
            return replay_or_conflict(existing, &request_hash);
        }
        let risk_order = build_pre_trade_risk_order(&parsed);
        // Probe readiness before consuming a preview. A runtime that becomes
        // unavailable after this probe is still fenced as UNKNOWN below.
        let writer = self.writer()?;
        let capability_version = execution_order_previews::jftrade_broker_capability_version();
        let sequence = self
            .store
            .next_sequence("internal-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-order-{sequence}");
        let mut order = new_order(&internal_id, &parsed, &now);
        order.normalized_request = canonical_execution_request(payload, &parsed, None)?;
        let reservation = self
            .store
            .reserve_order_with_preview_checked(
                order.clone(),
                &request_hash,
                &now,
                Some(capability_version.as_str()),
            )
            .map_err(map_reservation_error)?;
        if let ExecutionOrderReservation::Existing(existing) = reservation {
            return replay_or_conflict(existing, &request_hash);
        }
        let result = match self.execute_order_under_guard(&risk_order, || {
            writer.place_order(parsed.to_trade_request()).map_err(map_trade_error)
        }) {
            Ok(result) => result,
            Err(error) => {
                if is_risk_rejection(&error) {
                    let _ = self.persist_rejected(&mut order, &error, &now);
                    return Err(error);
                }
                self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
                return Err(error);
            }
        };
        if result.order_id.is_none() && result.order_id_ex.as_deref().is_none_or(str::is_empty) {
            let error = failed(
                502,
                "BROKER_INVALID_RESPONSE",
                "OpenD response did not include an order id",
            );
            self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
            return Err(error);
        }
        let previous_status = order.status.clone();
        order.status = "SUBMITTED".to_owned();
        order.broker_order_id = result.order_id.map(|id| id.to_string());
        order.broker_order_id_ex = result.order_id_ex;
        order.submitted_at = Some(now.clone());
        order.updated_at = now.clone();
        self.persist_external_success(&order, "submitted", &previous_status, &now, &now)?;
        order_value(&order)
    }

    fn place_combo(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let default_env = self
            .default_trading_environment
            .as_ref()
            .map(|getter| getter());
        let parsed = parse_combo_with_defaults(payload, default_env.as_deref())
            .map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if parsed.order.preview_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "previewId is required for combo orders",
            ));
        }
        let legs = execution_order_previews::canonical_combo_legs(&parsed);
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let mut risk_order = build_pre_trade_risk_combo_order(&parsed);
        if parsed.order.header.trd_env == 1
            && !parsed.order.quantity_mode.eq_ignore_ascii_case("amount")
            && risk_order.price.is_none()
            && risk_order.legs.iter().any(|l| l.price.is_none())
        {
            prefetch_combo_leg_quotes(
                self.trade_runtime.as_deref(),
                &mut risk_order,
                payload,
                &now,
            )?;
        }
        let request_hash = preview_request_hash(payload, &parsed.order, Some(legs.clone()))?;
        if let Some(client_order_id) = parsed.order.client_order_id.as_deref()
            && let Some(existing) = self
                .store
                .find_order_by_client_identity(
                    &parsed.order.broker_id,
                    if parsed.order.header.trd_env == 1 {
                        "REAL"
                    } else {
                        "SIMULATE"
                    },
                    &parsed.order.header.acc_id.to_string(),
                    client_order_id,
                )
                .map_err(store_error)?
        {
            return replay_or_conflict(existing, &request_hash);
        }
        // Keep the preview credential untouched when OpenD is already known
        // to be unavailable. A race after this probe is persisted as UNKNOWN.
        let writer = self.writer()?;
        let capability_version = execution_order_previews::jftrade_broker_capability_version();
        let sequence = self
            .store
            .next_sequence("internal-combo-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-combo-{sequence}");
        let mut order = new_order(&internal_id, &parsed.order, &now);
        order.requested_quantity = Some(parsed.combo_quantity());
        order.normalized_request =
            canonical_execution_request(payload, &parsed.order, Some(legs))?;
        let reservation = self
            .store
            .reserve_order_with_preview_checked(
                order.clone(),
                &request_hash,
                &now,
                Some(capability_version.as_str()),
            )
            .map_err(map_reservation_error)?;
        if let ExecutionOrderReservation::Existing(existing) = reservation {
            return replay_or_conflict(existing, &request_hash);
        }
        let result = match self.execute_order_under_guard(&risk_order, || {
            writer.place_combo_order(parsed.to_trade_request()).map_err(map_trade_error)
        }) {
            Ok(result) => result,
            Err(error) => {
                if is_risk_rejection(&error) {
                    let _ = self.persist_rejected(&mut order, &error, &now);
                    return Err(error);
                }
                self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
                return Err(error);
            }
        };
        let Some(order_id_ex) = result.order_id_ex.filter(|value| !value.trim().is_empty()) else {
            let error = failed(
                502,
                "BROKER_INVALID_RESPONSE",
                "OpenD combo response did not include orderIDEx",
            );
            self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
            return Err(error);
        };
        let previous_status = order.status.clone();
        order.status = "SUBMITTED".to_owned();
        order.broker_order_id_ex = Some(order_id_ex);
        order.submitted_at = Some(now.clone());
        order.updated_at = now.clone();
        self.persist_external_success(&order, "submitted", &previous_status, &now, &now)?;
        order_value(&order)
    }

    fn cancel_order(&self, internal_id: &str) -> Result<Value, ExecutionWritePortError> {
        let mut order = self
            .store
            .get_order(internal_id)
            .map_err(store_error)?
            .ok_or_else(|| {
                failed(
                    404,
                    "EXECUTION_ORDER_NOT_FOUND",
                    "execution order not found",
                )
            })?;
        let _guard = CancelInFlightGuard::acquire(Arc::clone(&self.cancel_inflight), internal_id)?;
        if matches!(
            order.status.to_ascii_uppercase().as_str(),
            "FILLED" | "CANCELLED" | "FAILED" | "UNKNOWN"
        ) {
            return Err(failed(
                400,
                "EXECUTION_ORDER_TERMINAL",
                "execution order is already terminal",
            ));
        }
        if order.status.eq_ignore_ascii_case("CANCEL_SUBMITTED") {
            return Err(failed(
                409,
                "EXECUTION_ORDER_CANCEL_IN_PROGRESS",
                "execution order cancellation is already in progress",
            ));
        }
        let broker_order_id = order
            .broker_order_id
            .as_deref()
            .and_then(|value| value.parse::<u64>().ok())
            .unwrap_or(0);
        if broker_order_id == 0
            && order
                .broker_order_id_ex
                .as_deref()
                .is_none_or(|value| value.trim().is_empty())
        {
            return Err(failed(
                400,
                "BROKER_ORDER_ID_MISSING",
                "execution order has no broker order id or orderIDEx",
            ));
        }
        let writer = self.writer()?;
        let previous_status = order.status.clone();
        let expected_updated_at = order.updated_at.clone();
        let expected_revision = self
            .store
            .order_revision(internal_id)
            .map_err(store_error)?;
        let fence_now = crate::product::product_production_ports::provider_now_rfc3339();
        order.status = "CANCEL_SUBMITTED".to_owned();
        order.updated_at = fence_now.clone();
        self.persist_transition_with_revision(
            &order,
            "cancel_submitted",
            &previous_status,
            &expected_updated_at,
            &fence_now,
            expected_revision,
        )?;
        let modify_result = writer.modify_order(TradeModifyOrderRequest {
            header: header_from_order(&order)?,
            order_id: broker_order_id,
            operation: 2,
            for_all: None,
            trd_market: None,
            quantity: None,
            price: None,
            adjust_price: None,
            adjust_side_and_limit: None,
            aux_price: None,
            trail_type: None,
            trail_value: None,
            trail_spread: None,
            order_id_ex: order.broker_order_id_ex.clone(),
        });
        if let Err(error) = modify_result {
            let mapped = map_trade_error(error);
            let now = crate::product::product_production_ports::provider_now_rfc3339();
            self.persist_unknown(&mut order, &mapped, "cancel_failed", &now)?;
            return Err(mapped);
        }
        // The durable CANCEL_SUBMITTED fence was committed before the
        // external call.  A successful acknowledgement needs no second
        // state write; reconciliation will apply the broker terminal state.
        order_value(&order)
    }

    /// Resolve the public broker cancellation item to the durable local order
    /// identity.  The broker API accepts the numeric `orderId` plus optional
    /// broker/external identifiers; treating those fields as strings (or as a
    /// local id) silently cancels the wrong order or reports a false 400.
    fn resolve_broker_cancel_target(
        &self,
        item: &Value,
    ) -> Result<String, ExecutionWritePortError> {
        let object = item.as_object().ok_or_else(|| {
            failed(
                400,
                "BAD_REQUEST",
                "each order cancellation item must be an object",
            )
        })?;
        let internal_id = value_identifier(object.get("internalOrderId"));
        let broker_id = value_identifier(object.get("orderId"));
        let broker_order_id = value_identifier(object.get("brokerOrderId"));
        let broker_order_id_ex = value_identifier(object.get("brokerOrderIdEx"));
        if internal_id.is_none()
            && broker_id.is_none()
            && broker_order_id.is_none()
            && broker_order_id_ex.is_none()
        {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "orderId, brokerOrderId, or internalOrderId is required",
            ));
        }
        let symbol = value_identifier(object.get("symbol"));
        let orders = self.store.list_orders().map_err(store_error)?;
        let mut matches = orders
            .iter()
            .filter(|order| {
                internal_id
                    .as_deref()
                    .is_some_and(|id| order.internal_order_id == id)
                    || broker_id
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id.as_deref() == Some(id))
                    || broker_order_id
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id.as_deref() == Some(id))
                    || broker_order_id_ex
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id_ex.as_deref() == Some(id))
            })
            .collect::<Vec<_>>();
        if let Some(symbol) = symbol.as_deref() {
            let narrowed = matches
                .iter()
                .copied()
                .filter(|order| order.symbol.as_deref().is_some_and(|value| value == symbol))
                .collect::<Vec<_>>();
            if !narrowed.is_empty() {
                matches = narrowed;
            }
        }
        match matches.as_slice() {
            [order] => Ok(order.internal_order_id.clone()),
            [] => Err(failed(
                404,
                "EXECUTION_ORDER_NOT_FOUND",
                "execution order not found for supplied broker order identity",
            )),
            _ => Err(failed(
                409,
                "EXECUTION_ORDER_AMBIGUOUS",
                "supplied broker order identity matches multiple execution orders",
            )),
        }
    }

    fn persist_transition(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        previous_status: Option<&str>,
        expected_updated_at: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let expected_status = previous_status.ok_or_else(|| {
            failed(
                500,
                "EXECUTION_STATE_ERROR",
                "state transitions require a previous status",
            )
        })?;
        self.persist_transition_with_payload(
            order,
            event_type,
            expected_status,
            expected_updated_at,
            timestamp,
            &order.normalized_request,
        )
    }

    fn persist_transition_with_revision(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        expected_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
        expected_revision: u64,
    ) -> Result<(), ExecutionWritePortError> {
        let event_id = format!(
            "{}-{event_type}-{}",
            order.internal_order_id,
            self.store
                .next_sequence("order-event")
                .map_err(store_error)?
        );
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &order.internal_order_id,
            event_type,
            previous_status: Some(expected_status),
            next_status: &order.status,
            payload_json: &order.normalized_request,
            created_at: timestamp,
        };
        self.store
            .transition_order_and_event_fenced(
                order.clone(),
                timestamp,
                &event,
                expected_status,
                expected_updated_at,
                Some(expected_revision),
            )
            .map(|_| ())
            .map_err(map_transition_store_error)
    }

    /// Persist the local acknowledgement after an external broker command
    /// succeeded.  If the CAS/event transaction fails, the broker side effect
    /// is already irreversible; best-effortly fence the durable order as
    /// `UNKNOWN` so callers do not mistake a stale `SUBMITTING`/
    /// `CANCEL_SUBMITTED` row for a safely retryable command.
    fn persist_external_success(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        previous_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        if let Err(error) = self.persist_transition(
            order,
            event_type,
            Some(previous_status),
            expected_updated_at,
            timestamp,
        ) {
            let mut unknown = order.clone();
            unknown.status = previous_status.to_owned();
            unknown.updated_at = expected_updated_at.to_owned();
            let detail = execution_error_details(&error).0;
            let unknown_error = failed(
                502,
                "EXECUTION_STATE_UNKNOWN",
                format!(
                    "broker accepted {event_type}, but local state could not be persisted: {detail}"
                ),
            );
            let _ = self.persist_unknown(&mut unknown, &unknown_error, "state_unknown", timestamp);
            return Err(unknown_error);
        }
        Ok(())
    }

    fn persist_transition_with_payload(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        expected_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
        payload_json: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let event_id = format!(
            "{}-{event_type}-{}",
            order.internal_order_id,
            self.store
                .next_sequence("order-event")
                .map_err(store_error)?
        );
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &order.internal_order_id,
            event_type,
            previous_status: Some(expected_status),
            next_status: &order.status,
            payload_json,
            created_at: timestamp,
        };
        self.store
            .transition_order_and_event(
                order.clone(),
                timestamp,
                &event,
                expected_status,
                expected_updated_at,
            )
            .map(|_| ())
            .map_err(map_transition_store_error)
    }

    fn execute_order_under_guard<T>(
        &self,
        risk_order: &PreTradeRiskOrder,
        submit_fn: impl FnOnce() -> Result<T, ExecutionWritePortError>,
    ) -> Result<T, ExecutionWritePortError> {
        match self.risk_coordinator.as_ref() {
            Some(coordinator) => coordinator.execute_with_risk_guard(risk_order, submit_fn),
            None if risk_order.trading_environment == TradingEnvironment::Real => Err(failed(
                403,
                "PRE_TRADE_RISK_UNAVAILABLE",
                "pre-trade risk gateway is unavailable; REAL orders are blocked",
            )),
            None => submit_fn(),
        }
    }

    fn persist_rejected(
        &self,
        order: &mut StoredExecutionOrder,
        error: &ExecutionWritePortError,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let previous_status = order.status.clone();
        let expected_updated_at = order.updated_at.clone();
        order.status = "REJECTED".to_owned();
        let (message, code) = execution_error_details(error);
        order.last_error = Some(message);
        order.last_error_code = code;
        order.last_error_source = Some("risk".to_owned());
        order.updated_at = timestamp.to_owned();
        self.persist_transition(
            order,
            "risk_rejected",
            Some(&previous_status),
            &expected_updated_at,
            timestamp,
        )
    }

    fn persist_unknown(
        &self,
        order: &mut StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let previous_status = order.status.clone();
        let expected_updated_at = order.updated_at.clone();
        order.status = "UNKNOWN".to_owned();
        let (message, code) = execution_error_details(error);
        order.last_error = Some(message);
        order.last_error_code = code;
        order.last_error_source = Some("opend".to_owned());
        order.updated_at = timestamp.to_owned();
        self.persist_transition(
            order,
            event_type,
            Some(&previous_status),
            &expected_updated_at,
            timestamp,
        )
    }
}

fn is_risk_rejection(error: &ExecutionWritePortError) -> bool {
    match error {
        ExecutionWritePortError::Failed { status: 403, .. } => true,
        ExecutionWritePortError::Failed { status: 400, code, .. } => {
            code == "INVALID_ORDER_RISK_SHAPE"
                || code.starts_with("PRE_TRADE_RISK_")
                || code.contains("RISK_")
        }
        _ => false,
    }
}
