import type { components } from "@/generated/openapi";

export type ExecutionOrderDto =
  components["schemas"]["trading.ExecutionOrder"];

export type ExecutionCommandResponse =
  components["schemas"]["trading.ExecutionCommandResponse"];

export type ExecutionComboRequest =
  components["schemas"]["trading.ExecutionComboRequest"];

export type RealTradeApprovalsResponse =
  components["schemas"]["system.RealTradeApprovalsResponse"];

export type RealTradeRiskEventsResponse =
  components["schemas"]["system.RealTradeRiskEventsResponse"];

export type RealTradeRiskStateResponse =
  components["schemas"]["system.RealTradeRiskLimitsResponse"];

export type RealTradeKillSwitchEventsResponse =
  components["schemas"]["system.RealTradeKillSwitchEventsResponse"];

export type RealTradeKillSwitchStateResponse =
  components["schemas"]["system.RealTradeKillSwitchStateResponse"];

export type RealTradeHardStopsResponse =
  components["schemas"]["system.RealTradeHardStopsResponse"];

export type RealTradeHardStopEventsResponse =
  components["schemas"]["system.RealTradeHardStopEventsResponse"];

export type RealTradeRiskSnapshot =
  components["schemas"]["trading.RealTradeRiskSnapshot"];

export type RealTradeKillSwitchCommandPayload =
  components["schemas"]["system.RealTradeKillSwitchRequest"];

export type RealTradeHardStopCommandPayload =
  components["schemas"]["system.RealTradeHardStopRequest"];

export type RealTradeRuntimeRiskCommandPayload =
  components["schemas"]["system.RealTradeRuntimeRiskRequest"];

export type BrokerRuntimeResponse =
  components["schemas"]["trading.BrokerRuntimeResponse"];

export type BrokerPositionsResponse =
  components["schemas"]["trading.BrokerPositionsResponse"];

export type BrokerFundsResponse =
  components["schemas"]["trading.BrokerFundsResponse"];

export type BrokerCashFlowsResponse =
  components["schemas"]["trading.BrokerCashFlowsResponse"];

export type BrokerOrderFeesResponse =
  components["schemas"]["trading.BrokerOrderFeesResponse"];

export type BrokerFillsResponse =
  components["schemas"]["trading.BrokerFillsResponse"];

export type BrokerMarginRatiosResponse =
  components["schemas"]["trading.BrokerMarginRatiosResponse"];

export type BrokerMaxTradeQuantityResponse =
  components["schemas"]["trading.BrokerMaxTradeQuantityResponse"];

export type BrokerOrdersResponse =
  components["schemas"]["trading.BrokerOrdersResponse"];

export type PortfolioPositionsResponse =
  components["schemas"]["trading.PortfolioPositionsResponse"];

export type PortfolioCashBalancesResponse =
  components["schemas"]["trading.PortfolioCashBalancesResponse"];

export type ExecutionOrderEventResponse =
  components["schemas"]["trading.ExecutionOrderEvent"];

export type ExecutionOrdersDto =
  components["schemas"]["trading.ExecutionOrders"];

export type ExecutionOrderDetailsDto =
  components["schemas"]["trading.ExecutionOrderDetails"];

export type ExecutionOrderEventsDto =
  components["schemas"]["trading.ExecutionOrderEvents"];
