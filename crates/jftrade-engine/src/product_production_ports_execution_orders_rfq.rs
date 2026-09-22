// Prediction-RFQ consumption for event-parlay combo submissions.

impl ProductionExecutionPort {
    /// Consume the stored prediction RFQ that prices this parlay.
    ///
    /// Go's `CreateExecutionCombo` consumes the market-data-issued RFQ after
    /// the preview credential and before the final risk window, so one RFQ can
    /// fund exactly one preview/client-order pair and an RFQ the server never
    /// issued can never price a parlay. The store keeps Go's idempotent replay
    /// rule for the identical pair, which the client-order identity fence above
    /// already returns before this runs.
    fn consume_prediction_rfq(
        &self,
        payload: &Value,
        parsed: &execution_order_parse::ParsedCombo,
        now: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let quote_id = parsed
            .quote_id
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| failed(400, "BAD_REQUEST", "event parlay requires rfqId"))?;
        let (mvc, legs_hash) = crate::product::product_production_ports::product_production_ports_market_data::product_production_ports_market_data_actions::product_prediction_combo_quote::prediction_quote_binding(payload)
            .map_err(|message| failed(400, "BAD_REQUEST", message))?;
        self.store
            .consume_prediction_quote(
                quote_id,
                &parsed.order.broker_id,
                &parsed.order.header.acc_id.to_string(),
                if parsed.order.header.trd_env == 1 {
                    "REAL"
                } else {
                    "SIMULATE"
                },
                &mvc,
                &legs_hash,
                parsed.order.preview_id.as_deref().unwrap_or_default(),
                parsed.order.client_order_id.as_deref().unwrap_or_default(),
                now,
            )
            .map_err(|error| match error {
                // Go reports every RFQ failure as one request error:
                // `prediction RFQ is invalid: <detail>`.
                ExecutionOrderStoreError::NotFound(detail)
                | ExecutionOrderStoreError::Validation(detail)
                | ExecutionOrderStoreError::Conflict(detail) => failed(
                    400,
                    "BAD_REQUEST",
                    format!("prediction RFQ is invalid: {detail}"),
                ),
                other => store_error(other),
            })
    }
}
