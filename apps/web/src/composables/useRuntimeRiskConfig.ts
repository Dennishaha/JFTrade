import type { RealTradeRiskStateResponse } from "@/contracts";
import { fetchEnvelopeWithInit } from "./apiClient";

export interface RuntimeRiskConfigPayload {
  realTradingEnabled: boolean;
  maxOrderQuantity: number | null;
  maxOrderNotional: number | null;
  operatorId: string;
  reason: string;
}

const runtimeRiskPath = "/api/v1/system/real-trade-risk-limits";

export function useRuntimeRiskConfig() {
  async function saveRuntimeRiskConfig(
    payload: RuntimeRiskConfigPayload,
  ): Promise<RealTradeRiskStateResponse> {
    return fetchEnvelopeWithInit<RealTradeRiskStateResponse>(runtimeRiskPath, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
  }

  async function disableRuntimeRiskConfig(
    payload: Pick<RuntimeRiskConfigPayload, "operatorId" | "reason">,
  ): Promise<RealTradeRiskStateResponse> {
    return fetchEnvelopeWithInit<RealTradeRiskStateResponse>(runtimeRiskPath, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
  }

  return {
    disableRuntimeRiskConfig,
    saveRuntimeRiskConfig,
  };
}
