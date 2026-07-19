import type { RealTradeRiskSnapshot } from "@/contracts";
import { fetchEnvelopeWithInit } from "./apiClient";

export interface RuntimeRiskConfigPayload {
  realTradingEnabled: boolean;
  maxOrderQuantity: number | null;
  maxOrderNotional: number | null;
  operatorId: string;
  reason: string;
}

const runtimeRiskPath = "/api/v1/system/real-trade-risk-limits";

// PUT/DELETE 的 200 响应是完整控制面快照（trading.RealTradeRiskSnapshot），
// 不是 GET 的风控限额读取模型。payload 中显式的 null 用于清空限额，
// 生成类型不含 null 成员，因此这里保留显式类型参数而非 body 推导。
export function useRuntimeRiskConfig() {
  async function saveRuntimeRiskConfig(
    payload: RuntimeRiskConfigPayload,
  ): Promise<RealTradeRiskSnapshot> {
    return fetchEnvelopeWithInit<RealTradeRiskSnapshot>(runtimeRiskPath, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
  }

  async function disableRuntimeRiskConfig(
    payload: Pick<RuntimeRiskConfigPayload, "operatorId" | "reason">,
  ): Promise<RealTradeRiskSnapshot> {
    return fetchEnvelopeWithInit<RealTradeRiskSnapshot>(runtimeRiskPath, {
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
