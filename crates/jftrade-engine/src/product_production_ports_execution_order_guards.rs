// Pre-trade risk wiring for the production execution port.
//
// Go runs the same gateway twice for every placement: once before the preview
// credential is spent (`evaluatePlaceExecutionOrderRisk`) and once inside the
// submission window (`executePlaceOrderWithRisk`). The `precheck_order` half
// reads the state the coordinator already owns; the guard half holds the
// submission gate and re-reads the control file.

impl ProductionExecutionPort {
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

    fn precheck_order(
        &self,
        risk_order: &PreTradeRiskOrder,
    ) -> Result<(), ExecutionWritePortError> {
        match self.risk_coordinator.as_ref() {
            Some(coordinator) => coordinator.precheck(risk_order),
            None if risk_order.trading_environment == TradingEnvironment::Real => Err(failed(
                403,
                "PRE_TRADE_RISK_UNAVAILABLE",
                "pre-trade risk gateway is unavailable; REAL orders are blocked",
            )),
            None => Ok(()),
        }
    }
}
