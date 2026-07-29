import type { RealTradeRiskSnapshot } from "@/contracts";
import { apiDeleteBody, apiPut } from "@/composables/shared/apiClient";

export interface RuntimeRiskConfigPayload {
  realTradingEnabled: boolean;
  maxOrderQuantity: number | null;
  maxOrderNotional: number | null;
  operatorId: string;
  reason: string;
}

const runtimeRiskPath = "/api/v1/system/real-trade-risk-limits";

// PUT/DELETE 的 200 响应是完整控制面快照（trading.RealTradeRiskSnapshot），
// 不是 GET 的风控限额读取模型。payload 中显式的 null 用于清空限额。
export function useRuntimeRiskConfig() {
  async function saveRuntimeRiskConfig(
    payload: RuntimeRiskConfigPayload,
  ): Promise<RealTradeRiskSnapshot> {
    return apiPut(runtimeRiskPath, payload);
  }

  async function disableRuntimeRiskConfig(
    payload: Pick<RuntimeRiskConfigPayload, "operatorId" | "reason">,
  ): Promise<RealTradeRiskSnapshot> {
    return apiDeleteBody(runtimeRiskPath, payload);
  }

  return {
    disableRuntimeRiskConfig,
    saveRuntimeRiskConfig,
  };
}
