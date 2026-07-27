import type {
  StrategyInstanceBindingDocument,
  StrategyRuntimeRiskSettings,
} from "@/types";
import type { components } from "@/generated/openapi";

type StrategyBindingRequest =
  components["schemas"]["strategy.StrategyBindingRequest"];
type StrategyRuntimeRiskRequest =
  components["schemas"]["strategy.RuntimeRiskSettings"];

export function mapStrategyRuntimeRiskRequest(
  value: StrategyRuntimeRiskSettings,
): StrategyRuntimeRiskRequest {
  return {
    mode: value.mode,
    closeOnly: value.closeOnly,
    pauseOnReject: value.pauseOnReject,
    ...(value.maxOrderQuantity === undefined
      ? {}
      : { maxOrderQuantity: value.maxOrderQuantity }),
    ...(value.maxOrderNotional === undefined
      ? {}
      : { maxOrderNotional: value.maxOrderNotional }),
    ...(value.dailyMaxOrders === undefined
      ? {}
      : { dailyMaxOrders: value.dailyMaxOrders }),
  };
}

export function mapStrategyBindingRequest(
  value: StrategyInstanceBindingDocument,
): StrategyBindingRequest {
  const brokerAccount = value.brokerAccount;
  return {
    symbols: value.symbols,
    interval: value.interval,
    chartType: value.chartType ?? "standard",
    executionMode: value.executionMode,
    runtimeRisk: mapStrategyRuntimeRiskRequest(value.runtimeRisk),
    ...(value.instruments == null ? {} : { instruments: value.instruments }),
    ...(brokerAccount == null ? {} : { brokerAccount }),
  };
}
