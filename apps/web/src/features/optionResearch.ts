export type OptionResearchEntry = Record<string, unknown>;
export type OptionResearchOperation =
  | "unusual"
  | "zero_dte"
  | "earnings"
  | "seller";
export type OptionSellerStrategy = "covered_call" | "cash_secured_put";

export interface OptionResearchColumn {
  key: string;
  label: string;
}

export interface OptionResearchDrilldownContext {
  underlyingInstrumentId: string;
  expiryTimestamp: number;
  chain: {
    productCode: string;
    multiplier?: number;
    contractSize?: number;
    expirationType?: number;
  };
}

export const optionResearchColumns: Record<
  OptionResearchOperation,
  OptionResearchColumn[]
> = {
  unusual: [
    { key: "fillTime", label: "时间" },
    { key: "owner", label: "标的" },
    { key: "option", label: "期权合约" },
    { key: "strikePrice", label: "行权价" },
    { key: "price", label: "成交价" },
    { key: "volume", label: "成交量" },
    { key: "iv", label: "IV" },
    { key: "sentiment", label: "情绪" },
  ],
  zero_dte: [
    { key: "owner", label: "标的" },
    { key: "name", label: "名称" },
    { key: "price", label: "最新价" },
    { key: "changeRate", label: "涨跌幅" },
    { key: "iv", label: "IV" },
    { key: "hv", label: "HV" },
    { key: "volume", label: "成交量" },
    { key: "openInterest", label: "持仓量" },
  ],
  earnings: [
    { key: "owner", label: "标的" },
    { key: "name", label: "名称" },
    { key: "earningsTime", label: "财报日" },
    { key: "iv", label: "IV" },
    { key: "hv", label: "HV" },
    { key: "expectedMoveRatio", label: "预期波动" },
    { key: "volume", label: "成交量" },
    { key: "openInterest", label: "持仓量" },
  ],
  seller: [
    { key: "owner", label: "标的" },
    { key: "option", label: "期权合约" },
    { key: "strikePrice", label: "行权价" },
    { key: "strikeTime", label: "到期日" },
    { key: "optionPrice", label: "期权价" },
    { key: "premium", label: "权利金" },
    { key: "annualizedReturn", label: "年化收益" },
    { key: "itmProbability", label: "行权概率" },
  ],
};

export function optionSecurityInstrumentId(value: unknown): string {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    return "";
  }
  const entry = value as OptionResearchEntry;
  const direct = String(entry.instrumentId ?? "").trim().toUpperCase();
  if (direct) return direct;
  const market = String(entry.market ?? "").trim().toUpperCase();
  const code = String(entry.code ?? "").trim().toUpperCase();
  return market && code ? `${market}.${code}` : "";
}

export function optionEntryInstrumentId(
  entry: OptionResearchEntry,
  kind: "option" | "equity",
): string {
  const nested = kind === "option" ? entry.option : entry.owner;
  return optionSecurityInstrumentId(nested) || optionSecurityInstrumentId(entry);
}

export function formatOptionResearchCell(value: unknown): string {
  const instrumentId = optionSecurityInstrumentId(value);
  if (instrumentId) return instrumentId;
  if (value == null || value === "") return "—";
  if (typeof value === "number") {
    return new Intl.NumberFormat("zh-CN", {
      maximumFractionDigits: 4,
    }).format(value);
  }
  if (typeof value === "boolean") return value ? "是" : "否";
  return typeof value === "string" ? value : "—";
}
