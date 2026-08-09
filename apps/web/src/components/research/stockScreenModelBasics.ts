import type {
  StockScreenCatalog,
  StockScreenBoundary,
  StockScreenColumn,
  StockScreenDraft,
  StockScreenEditorFilter,
  StockScreenEntry,
  StockScreenFactor,
  StockScreenFactorRef,
  StockScreenConditionV2,
  StockScreenDefinitionV2,
  StockScreenFactorParameter,
  StockScreenFactorParams,
  StockScreenFilter,
  StockScreenInterval,
  StockScreenSort,
  StockScreenValue,
} from "./stockScreenTypes";

export function factorRefKey(ref: Pick<StockScreenFactorRef, "factor" | "factorKey">): string {
  return ref.factorKey?.trim() || ref.factor;
}

export function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, item]) => [key, stableValue(item)]),
    );
  }
  return value;
}

export function stockScreenFactorRefSignature(ref: StockScreenFactorRef): string {
  return JSON.stringify({
    factorKey: factorRefKey(ref),
    params: stableValue(ref.params ?? {}),
  });
}

export function sameStockScreenFactorRef(
  left: StockScreenFactorRef,
  right: StockScreenFactorRef,
): boolean {
  return stockScreenFactorRefSignature(left) === stockScreenFactorRefSignature(right);
}

export function stockScreenFactorInstanceId(
  ref: Pick<StockScreenFactorRef, "factor" | "factorKey" | "instanceId">,
  fallback = "",
): string {
  return ref.instanceId?.trim() || fallback || factorRefKey(ref);
}

const PARAMETER_LABELS: Record<string, string> = {
  days: "统计天数",
  periodAverage: "周期平均",
  term: "财报周期",
  duration: "持续时长",
  year: "年份",
  futureDuration: "预测区间",
  period: "K 线周期",
  rangePeriod: "区间周期",
  firstCustomParam: "自定义参数",
  indicatorParams: "指标参数",
  brokerParam: "经纪商参数",
  optionParam: "期权参数",
  optionHvPeriod: "历史波动率周期",
};

export function normalizeScreenMarket(market: string): "HK" | "US" | "SH" | "SZ" {
  const normalized = market.trim().toUpperCase();
  if (normalized === "HK" || normalized === "US" || normalized === "SZ") {
    return normalized;
  }
  return "SH";
}

export function parameterLabel(parameter: StockScreenFactorParameter): string {
  return PARAMETER_LABELS[parameter.name] ?? parameter.name;
}

export function factorEnumName(factor: StockScreenFactor): string {
  if (factor.valueEnum) return factor.valueEnum;
  if (factor.key === "field.market") return "market";
  if (factor.category === "kline_shape") return "kline_shape_type";
  if (factor.key.includes("cash_flow")) return "cash_flow_period";
  return "";
}

export function defaultParameterValue(
  parameter: StockScreenFactorParameter,
  catalog: StockScreenCatalog,
): string | number | number[] {
  if (Array.isArray(parameter.default)) {
    return parameter.default.map(Number).filter(Number.isFinite);
  }
  if (typeof parameter.default === "number" || typeof parameter.default === "string") {
    return parameter.default;
  }
  if (parameter.enum) {
    const options = catalog.enums[parameter.enum] ?? [];
    const preferred =
      options.find((option) => option.key === "day") ??
      options.find((option) => option.key !== "unknown") ??
      options[0];
    return preferred?.value ?? 0;
  }
  if (parameter.type === "integer_array") return "";
  if (parameter.name === "days") return 1;
  if (parameter.name === "period") return 11;
  return parameter.minimum ?? 0;
}

export function initialParams(
  factor: StockScreenFactor,
  catalog: StockScreenCatalog,
): StockScreenFactorParams | undefined {
  const params: Record<string, unknown> = {};
  for (const parameter of factor.parameters ?? []) {
    if (parameter.name === "optionParam") continue;
    if (
      !parameter.required &&
      parameter.name !== "days" &&
      parameter.name !== "period"
    ) {
      continue;
    }
    params[parameter.name] = defaultParameterValue(parameter, catalog);
  }
  return Object.keys(params).length
    ? (params as StockScreenFactorParams)
    : undefined;
}

export function defaultSetValues(
  factor: StockScreenFactor,
  catalog: StockScreenCatalog,
  market: string,
): number[] {
  const enumName = factorEnumName(factor);
  const options = enumName ? (catalog.enums[enumName] ?? []) : [];
  if (factor.key === "field.market") {
    const key =
      normalizeScreenMarket(market) === "HK"
        ? "hk"
        : normalizeScreenMarket(market) === "US"
          ? "us"
          : "cn";
    return [options.find((option) => option.key === key)?.value ?? 0];
  }
  return options.length ? [options[0]!.value] : [0];
}

export function createStockScreenFilter(
  factor: StockScreenFactor,
  serial: number,
  catalog: StockScreenCatalog,
  market: string,
  instanceId?: string,
): StockScreenEditorFilter {
  const filter: StockScreenEditorFilter = {
    id: `${factor.key}-${serial}`,
    factor: factor.key,
  };
  if (instanceId) {
    filter.instanceId = instanceId;
    filter.factorKey = factor.key;
  }
  const params = initialParams(factor, catalog);
  if (params) filter.params = params;

  switch (factor.filterKind) {
    case "enum":
    case "set":
      filter.values = defaultSetValues(factor, catalog, market);
      break;
    case "position":
      filter.position = 1;
      filter.continuousPeriod = 1;
      break;
    case "pattern":
      filter.match = true;
      filter.continuousPeriod = 1;
      break;
    default:
      break;
  }
  return filter;
}

export function stockScreenValueData(
  wrapped: StockScreenValue | undefined,
): string | number | number[] | null {
  if (!wrapped || wrapped.type === "missing") return null;
  switch (wrapped.type) {
    case "string":
      return wrapped.string ?? null;
    case "integer":
      return wrapped.integer ?? null;
    case "integer_array":
      return wrapped.integers ?? null;
    case "number":
      return wrapped.number ?? null;
    default:
      return null;
  }
}

export function formatStockScreenValue(
  wrapped: StockScreenValue | undefined,
  factor?: StockScreenFactor,
  entry?: StockScreenEntry,
): string {
  if (wrapped?.enumName) return wrapped.enumName;
  const value = stockScreenValueData(wrapped);
  if (value == null || value === "") return "—";
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "number") {
    const unit = factor?.unit ?? wrapped?.unit ?? "";
    const displayFormat =
      factor?.displayFormat ??
      (unit === "currency"
        ? factor?.key.includes("price")
          ? "price"
          : "compact_amount"
        : unit === "percent"
          ? "percent"
          : unit === "timestamp"
            ? "timestamp"
            : factor?.valueType === "integer"
              ? "integer"
              : "number");
    let text: string;
    switch (displayFormat) {
      case "price":
        text = formatStockScreenNumber(value, 2, 4);
        break;
      case "compact_amount":
        text = formatStockScreenCompactAmount(value);
        break;
      case "percent":
        return `${formatStockScreenNumber(value, 0, 2)}%`;
      case "integer":
        text = formatStockScreenNumber(value, 0, 0);
        break;
      case "timestamp":
        return formatStockScreenTimestamp(value);
      default:
        text = formatStockScreenNumber(value, 0, 4);
        break;
    }
    if (unit === "currency") {
      const basis =
        factor?.currencyBasis ??
        (factor?.category === "financial" ? "reporting" : "quote");
      if (basis === "quote" && entry?.quoteCurrency) {
        return `${entry.quoteCurrency} ${text}`;
      }
      return text;
    }
    const suffix =
      unit === "shares" ? "股" : unit === "days" ? "天" : "";
    return `${text}${suffix}`;
  }
  return String(value);
}

export function stockScreenValueTitle(
  wrapped: StockScreenValue | undefined,
  factor?: StockScreenFactor,
  entry?: StockScreenEntry,
): string | undefined {
  if (stockScreenValueData(wrapped) == null) return undefined;
  const unit = factor?.unit ?? wrapped?.unit ?? "";
  if (unit !== "currency") return undefined;
  const basis =
    factor?.currencyBasis ??
    (factor?.category === "financial" ? "reporting" : "quote");
  if (basis === "reporting") return "OpenD 未提供报表币种";
  return entry?.quoteCurrency ? undefined : "无法可靠确定报价币种";
}

export function formatStockScreenNumber(
  value: number,
  minimumFractionDigits: number,
  maximumFractionDigits: number,
): string {
  return new Intl.NumberFormat("zh-CN", {
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(value);
}

export function formatStockScreenCompactAmount(value: number): string {
  const absolute = Math.abs(value);
  const units = [
    { threshold: 1_000_000_000_000, divisor: 1_000_000_000_000, suffix: "万亿" },
    { threshold: 100_000_000, divisor: 100_000_000, suffix: "亿" },
    { threshold: 10_000, divisor: 10_000, suffix: "万" },
  ];
  const unit = units.find((candidate) => absolute >= candidate.threshold);
  if (!unit) return formatStockScreenNumber(value, 0, 2);
  return `${formatStockScreenNumber(value / unit.divisor, 0, 2)}${unit.suffix}`;
}

export function formatStockScreenTimestamp(value: number): string {
  const milliseconds = Math.abs(value) < 1_000_000_000_000 ? value * 1000 : value;
  const date = new Date(milliseconds);
  if (Number.isNaN(date.getTime())) {
    return formatStockScreenNumber(value, 0, 0);
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

export function cloneParams(
  params: StockScreenFactorParams | undefined,
): StockScreenFactorParams | undefined {
  if (!params) return undefined;
  return {
    ...params,
    ...(params.indicatorParams
      ? { indicatorParams: [...params.indicatorParams] }
      : {}),
    ...(params.optionParamIntegers
      ? { optionParamIntegers: [...params.optionParamIntegers] }
      : {}),
  };
}

export function cloneStockScreenFilter(
  filter: StockScreenFilter,
): StockScreenFilter {
  const params = cloneParams(filter.params);
  const secondParams = cloneParams(filter.secondFactor?.params);
  return {
    ...filter,
    ...(params ? { params } : {}),
    ...(filter.min ? { min: { ...filter.min } } : {}),
    ...(filter.max ? { max: { ...filter.max } } : {}),
    ...(filter.intervals
      ? {
          intervals: filter.intervals.map((interval) => ({
            ...interval,
            ...(interval.min ? { min: { ...interval.min } } : {}),
            ...(interval.max ? { max: { ...interval.max } } : {}),
          })),
        }
      : {}),
    ...(filter.values ? { values: [...filter.values] } : {}),
    ...(filter.secondFactor
      ? {
          secondFactor: {
            ...filter.secondFactor,
            ...(secondParams ? { params: secondParams } : {}),
          },
        }
      : {}),
  };
}

export function cloneStockScreenColumn(
  column: StockScreenColumn,
): StockScreenColumn {
  const params = cloneParams(column.params);
  return {
    ...column,
    ...(params ? { params } : {}),
  };
}

export function cloneStockScreenSort(sort: StockScreenSort): StockScreenSort {
  const params = cloneParams(sort.params);
  return {
    ...sort,
    ...(params ? { params } : {}),
  };
}

export function toStockScreenDraftFilter(
  filter: StockScreenEditorFilter,
): StockScreenFilter {
  const { id: _id, ...wire } = filter;
  return cloneStockScreenFilter({ ...wire, conditionId: filter.id });
}

export function cloneStockScreenDraft(query: StockScreenDraft): StockScreenDraft {
  return {
    ...(query.brokerId ? { brokerId: query.brokerId } : {}),
    market: normalizeScreenMarket(query.market),
    ...(query.pool
      ? {
          pool: {
            ...(query.pool.watchlistStockIds
              ? { watchlistStockIds: [...query.pool.watchlistStockIds] }
              : {}),
            ...(query.pool.plates
              ? {
                  plates: query.pool.plates.map((group) => ({
                    ...group,
                    plateIds: [...group.plateIds],
                  })),
                }
              : {}),
          },
        }
      : {}),
    filters: (query.filters ?? []).map(cloneStockScreenFilter),
    columns: (query.columns ?? []).map(cloneStockScreenColumn),
    sort: (query.sort ?? []).map(cloneStockScreenSort),
  };
}


