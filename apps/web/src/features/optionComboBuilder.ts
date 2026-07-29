import type { ExecutionComboRequest } from "@/contracts";

import type {
  OptionComboLegDraft,
  OptionComboStrategy,
} from "../composables/optionComboDraft";

interface ComboAccountImpact {
  nlvChange?: number | null;
  initialMarginChange?: number | null;
  maintenanceMarginChange?: number | null;
  optionBuyingPower?: number | null;
  maxWithdrawalChange?: number | null;
  buyingPowerDecrease?: number | null;
}

interface ComboAnalysis {
  bid?: number | null;
  ask?: number | null;
  maxProfit?: number | null;
  maxLoss?: number | null;
  maxProfitUnlimited?: boolean;
  maxLossUnlimited?: boolean;
  breakevenPoints?: number[];
  probability?: number | null;
  delta?: number | null;
  theta?: number | null;
}

export interface ComboPreview {
  previewId: string;
  allowed?: boolean;
  buyingPowerImpact?: number | null;
  accountImpact?: ComboAccountImpact | null;
  warnings?: string[];
  expiresAt?: string;
  optionAnalysis?: ComboAnalysis | null;
}

export const optionComboStrategyItems: Array<{
  value: OptionComboStrategy;
  label: string;
}> = [
  { value: "vertical", label: "垂直价差" },
  { value: "straddle", label: "跨式" },
  { value: "strangle", label: "宽跨式" },
  { value: "calendar", label: "日历价差" },
  { value: "butterfly", label: "蝶式" },
];

export function createOptionComboClientOrderId(): string {
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `jftrade-option-combo-${suffix}`;
}

export function buildOptionComboExecutionLegs(
  legs: OptionComboLegDraft[],
  quantity: number,
): ExecutionComboRequest["legs"] {
  return legs.map((leg) => ({
    instrumentId: leg.instrumentId,
    productClass: "option",
    side: leg.side,
    ratio: leg.ratio,
    quantity: quantity * leg.ratio,
  }));
}
