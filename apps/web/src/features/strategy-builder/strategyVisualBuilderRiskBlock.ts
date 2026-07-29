export type RiskRuleType = "allowEntryIn" | "maxDrawdown" | "maxIntradayLoss" | "maxIntradayFilledOrders" | "maxPositionSize" | "maxConsLossDays";
export type RiskAmountType = "strategy.percent_of_equity" | "strategy.cash";

export interface RiskRuleBlockProperties {
  blockKind: "riskRule";
  riskRuleType?: RiskRuleType;
  riskAllowedDirection?: "all" | "long" | "short";
  riskValue?: number;
  riskAmountType?: RiskAmountType;
  riskCount?: number;
  riskContracts?: number;
  alert_message?: string;
}

export const RISK_RULE_TYPE_OPTIONS: Array<{ value: RiskRuleType; label: string }> = [
  { value: "allowEntryIn", label: "允许入场方向" },
  { value: "maxDrawdown", label: "最大回撤" },
  { value: "maxIntradayLoss", label: "日内最大亏损" },
  { value: "maxIntradayFilledOrders", label: "日内成交上限" },
  { value: "maxPositionSize", label: "最大持仓" },
  { value: "maxConsLossDays", label: "连续亏损天数" },
];

export const RISK_AMOUNT_TYPE_OPTIONS: Array<{ value: RiskAmountType; label: string }> = [
  { value: "strategy.percent_of_equity", label: "权益百分比" },
  { value: "strategy.cash", label: "现金金额" },
];

export const RISK_RULE_BLOCK_DEFINITION = {
  kind: "riskRule",
  label: "策略风控",
  description: "声明 strategy.risk.allow_entry_in 与 strategy.risk.max_* 风控约束。",
  shape: "rect",
  text: "最大回撤 10%",
  properties: {
    blockKind: "riskRule", riskRuleType: "maxDrawdown", riskAllowedDirection: "all",
    riskValue: 10, riskAmountType: "strategy.percent_of_equity", riskCount: 3, riskContracts: 10,
  },
  accent: "#7c2d12",
} as const;

export function normalizeRiskRuleType(value: unknown): RiskRuleType {
  return RISK_RULE_TYPE_OPTIONS.some((option) => option.value === value)
    ? (value as RiskRuleType)
    : "maxDrawdown";
}

export function normalizeRiskAmountType(value: unknown): RiskAmountType {
  return value === "strategy.cash" ? "strategy.cash" : "strategy.percent_of_equity";
}

export function normalizeRiskRuleDirection(value: unknown): "all" | "long" | "short" {
  return value === "long" || value === "short" ? value : "all";
}

export function normalizeRiskRuleBlockProperties(
  properties: Record<string, unknown>,
): RiskRuleBlockProperties {
  return {
    blockKind: "riskRule",
    riskRuleType: normalizeRiskRuleType(properties.riskRuleType),
    riskAllowedDirection: normalizeRiskRuleDirection(properties.riskAllowedDirection),
    riskValue: normalizeDecimal(properties.riskValue, 10),
    riskAmountType: normalizeRiskAmountType(properties.riskAmountType),
    riskCount: normalizeInteger(properties.riskCount, 3),
    riskContracts: normalizeDecimal(properties.riskContracts, 10),
    ...(typeof properties.alert_message === "string" && properties.alert_message.trim() !== ""
      ? { alert_message: properties.alert_message.trim() }
      : {}),
  };
}

export function riskRuleTypeLabel(value: RiskRuleType): string {
  return RISK_RULE_TYPE_OPTIONS.find((option) => option.value === value)?.label ?? "策略风控";
}

export function riskAmountTypeLabel(value: RiskAmountType): string {
  return RISK_AMOUNT_TYPE_OPTIONS.find((option) => option.value === value)?.label ?? "权益百分比";
}

export function nextRiskRuleNodeText(rawProperties: Record<string, unknown>): string {
  const properties = normalizeRiskRuleBlockProperties(rawProperties);
  const amountSuffix = properties.riskAmountType === "strategy.cash" ? " 现金" : "%";
  switch (properties.riskRuleType) {
    case "allowEntryIn":
      return `允许入场 · ${properties.riskAllowedDirection === "long" ? "仅多头" : properties.riskAllowedDirection === "short" ? "仅空头" : "多空都可"}`;
    case "maxPositionSize":
      return `最大持仓 · ${properties.riskContracts ?? 10}`;
    case "maxIntradayFilledOrders":
      return `日内成交上限 · ${properties.riskCount ?? 3}`;
    case "maxConsLossDays":
      return `连续亏损天数 · ${properties.riskCount ?? 3}`;
    case "maxIntradayLoss":
      return `日内最大亏损 · ${properties.riskValue ?? 10}${amountSuffix}`;
    case "maxDrawdown":
    default:
      return `最大回撤 · ${properties.riskValue ?? 10}${amountSuffix}`;
  }
}

function normalizeInteger(value: unknown, fallback: number): number {
  const parsed = readNumber(value);
  return parsed === null ? fallback : Math.max(1, Math.round(parsed));
}

function normalizeDecimal(value: unknown, fallback: number): number {
  return readNumber(value) ?? fallback;
}

function readNumber(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return null;
}
