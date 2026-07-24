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

export interface StockScreenValidationError {
  path: string;
  message: string;
}

export function factorRefKey(ref: Pick<StockScreenFactorRef, "factor" | "factorKey">): string {
  return ref.factorKey?.trim() || ref.factor;
}

function stableValue(value: unknown): unknown {
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

function initialParams(
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

function defaultSetValues(
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

function formatStockScreenNumber(
  value: number,
  minimumFractionDigits: number,
  maximumFractionDigits: number,
): string {
  return new Intl.NumberFormat("zh-CN", {
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(value);
}

function formatStockScreenCompactAmount(value: number): string {
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

function formatStockScreenTimestamp(value: number): string {
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

function cloneParams(
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

export function stockScreenDraftFromDefinitionV2(
  definition: StockScreenDefinitionV2,
): StockScreenDraft {
  const draftRef = (
    ref: StockScreenDefinitionV2["columns"][number]["factor"],
  ): StockScreenFactorRef => ({
    factor: ref.factorKey,
    factorKey: ref.factorKey,
    instanceId: ref.instanceId,
    ...(ref.params ? { params: cloneParams(ref.params)! } : {}),
  });
  const asObject = (value: unknown): Record<string, unknown> =>
    value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  const asBoundary = (
    value: Record<string, unknown>,
    key: "min" | "max",
  ): StockScreenBoundary | undefined => {
    const number = value[key];
    if (typeof number !== "number" || !Number.isFinite(number)) return undefined;
    return {
      value: number,
      includes: value[`${key}Includes`] !== false,
    };
  };
  const asIntervals = (
    value: Record<string, unknown>,
  ): StockScreenInterval[] | undefined => {
    if (!Array.isArray(value.intervals)) return undefined;
    return value.intervals.map((item) => {
      const interval = asObject(item);
      const minimum = asBoundary(interval, "min");
      const maximum = asBoundary(interval, "max");
      return {
        ...(minimum ? { min: minimum } : {}),
        ...(maximum ? { max: maximum } : {}),
        ...(typeof interval.unit === "number" ? { unit: interval.unit } : {}),
      };
    });
  };
  return {
    ...(definition.brokerId ? { brokerId: definition.brokerId } : {}),
    market: normalizeScreenMarket(definition.market),
    ...(definition.pool
      ? {
          pool: {
            ...(definition.pool.watchlistStockIds
              ? { watchlistStockIds: [...definition.pool.watchlistStockIds] }
              : {}),
            ...(definition.pool.plates
              ? {
                  plates: definition.pool.plates.map((group) => ({
                    ...group,
                    plateIds: [...group.plateIds],
                  })),
                }
              : {}),
          },
        }
      : {}),
    filters: definition.conditions.map((condition) => {
      const value = asObject(condition.value);
      const filter: StockScreenFilter = {
        ...draftRef(condition.factor),
        conditionId: condition.id,
        ...(condition.secondFactor
          ? { secondFactor: draftRef(condition.secondFactor) }
          : {}),
      };
      if (condition.operator === "in" && Array.isArray(condition.value)) {
        filter.values = condition.value.map(Number).filter(Number.isFinite);
      } else if (condition.operator === "position") {
        if (typeof value.position === "number") filter.position = value.position;
        if (typeof value.secondValue === "number") {
          filter.secondValue = value.secondValue;
        }
        if (typeof value.continuousPeriod === "number") {
          filter.continuousPeriod = value.continuousPeriod;
        }
        const intervals = asIntervals(value);
        if (intervals) filter.intervals = intervals;
      } else if (condition.operator === "pattern") {
        if (typeof value.match === "boolean") filter.match = value.match;
        if (Array.isArray(value.values)) {
          filter.values = value.values.map(Number).filter(Number.isFinite);
        }
        if (typeof value.continuousPeriod === "number") {
          filter.continuousPeriod = value.continuousPeriod;
        }
      } else {
        const minimum = asBoundary(value, "min");
        const maximum = asBoundary(value, "max");
        const intervals = asIntervals(value);
        if (minimum) filter.min = minimum;
        if (maximum) filter.max = maximum;
        if (intervals) filter.intervals = intervals;
        if (typeof value.continuousPeriod === "number") {
          filter.continuousPeriod = value.continuousPeriod;
        }
      }
      return filter;
    }),
    columns: definition.columns.map((column) => ({
      ...draftRef(column.factor),
      columnId: column.columnId,
    })),
    sort: definition.sorts.map((sort) => ({
      ...draftRef(sort.factor),
      ...(sort.sortId ? { sortId: sort.sortId } : {}),
      direction: sort.direction,
    })),
  };
}

function csvCell(value: string): string {
  if (!/[",\r\n]/.test(value)) return value;
  return `"${value.replaceAll('"', '""')}"`;
}

export function stockScreenCSV(
  entries: StockScreenEntry[],
  factors: Map<string, StockScreenFactor>,
  columns: StockScreenColumn[],
): string {
  const headers = [
    "市场",
    "代码",
    "名称",
    ...columns.map((column) =>
      factors.get(column.factor)?.label ?? column.factor
    ),
  ];
  const lines = entries.map((entry) => [
    entry.market ?? "",
    entry.symbol ?? "",
    entry.name ?? "",
    ...columns.map((column) => {
      const value = stockScreenValueData(stockScreenEntryValue(entry, column));
      if (value == null) return "";
      return Array.isArray(value) ? value.join("|") : String(value);
    }),
  ]);
  return `\uFEFF${[headers, ...lines]
    .map((row) => row.map((cell) => csvCell(String(cell))).join(","))
    .join("\r\n")}`;
}

/** Resolve a result cell by its response column identity. */
export function stockScreenEntryValue(
  entry: StockScreenEntry,
  column: Pick<StockScreenColumn, "factor" | "factorKey" | "instanceId" | "columnId">,
): StockScreenValue | undefined {
  const columnId = column.columnId?.trim();
  return columnId ? stockScreenCellValue(entry.cells[columnId]) : undefined;
}

function stockScreenCellValue(
  cell: { value: StockScreenValue } | undefined,
): StockScreenValue | undefined {
  return cell?.value;
}

export function resultColumnFor(
  entry: StockScreenEntry,
  column: StockScreenColumn,
  resultColumns?: Array<{ columnId: string; instanceId?: string; factorKey: string }>,
): StockScreenValue | undefined {
  const exactResultColumn = resultColumns?.find(
    (candidate) =>
      candidate.columnId === column.columnId ||
      (column.instanceId != null &&
        candidate.instanceId === column.instanceId),
  );
  if (exactResultColumn)
    return stockScreenCellValue(entry.cells[exactResultColumn.columnId]);
  return stockScreenEntryValue(entry, column);
}

function finiteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function parameterValue(
  ref: StockScreenFactorRef,
  parameter: StockScreenFactorParameter,
): unknown {
  return (ref.params as Record<string, unknown> | undefined)?.[parameter.name];
}

function factorRefValidation(
  ref: StockScreenFactorRef,
  factor: StockScreenFactor,
  path: string,
): StockScreenValidationError[] {
  const errors: StockScreenValidationError[] = [];
  for (const parameter of factor.parameters ?? []) {
    const value = parameterValue(ref, parameter);
    const missing =
      parameter.required &&
      (value == null || value === "" || (Array.isArray(value) && value.length === 0));
    if (missing) {
      errors.push({
        path: `${path}.params.${parameter.name}`,
        message: `${parameterLabel(parameter)}为必填项`,
      });
      continue;
    }
    if (value == null || value === "") continue;
    const values = Array.isArray(value) ? value : [value];
    for (const item of values) {
      if (parameter.type === "integer" || parameter.type === "number" || parameter.enum) {
        const numeric = typeof item === "number" ? item : Number(item);
        if (!finiteNumber(numeric)) {
          errors.push({
            path: `${path}.params.${parameter.name}`,
            message: `${parameterLabel(parameter)}必须是数字`,
          });
          continue;
        }
        if (parameter.minimum != null && numeric < parameter.minimum) {
          errors.push({
            path: `${path}.params.${parameter.name}`,
            message: `${parameterLabel(parameter)}不能小于 ${parameter.minimum}`,
          });
        }
        if (parameter.maximum != null && numeric > parameter.maximum) {
          errors.push({
            path: `${path}.params.${parameter.name}`,
            message: `${parameterLabel(parameter)}不能大于 ${parameter.maximum}`,
          });
        }
      }
    }
  }
  return errors;
}

export function validateStockScreenQuery(
  query: StockScreenDraft,
  catalog?: StockScreenCatalog | null,
): StockScreenValidationError[] {
  const errors: StockScreenValidationError[] = [];
  const normalizedMarket = normalizeScreenMarket(query.market);
  if (!query.market || normalizedMarket !== query.market.trim().toUpperCase() && query.market.trim().toUpperCase() !== "CN") {
    errors.push({ path: "market", message: "请选择有效市场" });
  }
  const factors = new Map((catalog?.factors ?? []).map((factor) => [factor.key, factor]));
  const validateRef = (ref: StockScreenFactorRef, path: string, role: "filter" | "retrieve" | "sort") => {
    const key = factorRefKey(ref);
    const factor = factors.get(key);
    if (!factor) {
      if (catalog) errors.push({ path: `${path}.factor`, message: "因子不在当前市场目录中" });
      return;
    }
    if (factor.availability === "unsupported") {
      errors.push({ path: `${path}.factor`, message: factor.reason || "当前市场不可用" });
    }
    if ((role === "filter" && !factor.filter) || (role === "retrieve" && !factor.retrieve) || (role === "sort" && !factor.sort)) {
      errors.push({ path: `${path}.factor`, message: "该因子不能用于此位置" });
    }
    errors.push(...factorRefValidation(ref, factor, path));
  };
  const filters = query.filters ?? [];
  filters.forEach((filter, index) => {
    const path = `conditions.${index}`;
    validateRef(filter, path, "filter");
    const factor = factors.get(factorRefKey(filter));
    if (filter.min && !finiteNumber(filter.min.value)) errors.push({ path: `${path}.min`, message: "下限必须是数字" });
    if (filter.max && !finiteNumber(filter.max.value)) errors.push({ path: `${path}.max`, message: "上限必须是数字" });
    if (filter.min && filter.max && filter.min.value > filter.max.value) errors.push({ path: `${path}.max`, message: "上限不能小于下限" });
    switch (factor?.filterKind) {
      case "enum":
      case "set":
        if (!filter.values?.length) {
          errors.push({ path: `${path}.values`, message: "请选择至少一个条件值" });
        }
        break;
      case "interval":
        if (!filter.min && !filter.max) {
          errors.push({ path: `${path}.min`, message: "请至少填写一个边界" });
        }
        break;
      case "interval_or_set":
        if (filter.values != null) {
          if (!filter.values.length) {
            errors.push({ path: `${path}.values`, message: "请选择至少一个条件值" });
          }
        } else if (!filter.min && !filter.max && !filter.intervals?.length) {
          errors.push({ path: `${path}.min`, message: "请至少填写一个边界" });
        }
        break;
      case "position":
        if (!Number.isInteger(filter.position) || (filter.position ?? 0) < 1 || (filter.position ?? 0) > 4) {
          errors.push({ path: `${path}.position`, message: "请选择有效的位置关系" });
        }
        if (!filter.secondFactor && !finiteNumber(filter.secondValue)) {
          errors.push({ path: `${path}.secondValue`, message: "请填写比较值或选择比较指标" });
        }
        break;
      default:
        break;
    }
    if (filter.secondFactor) {
      validateRef(filter.secondFactor, `${path}.secondFactor`, "filter");
      const second = factors.get(factorRefKey(filter.secondFactor));
      if (second && second.category !== "indicator") {
        errors.push({ path: `${path}.secondFactor`, message: "比较因子必须是技术指标" });
      }
    }
    if (filter.secondFactor && !filter.secondFactor.instanceId && filter.secondFactor.factorKey) {
      errors.push({ path: `${path}.secondFactor.instanceId`, message: "比较因子缺少实例标识" });
    }
  });
  const seenFilters = new Map<string, number>();
  filters.forEach((filter, index) => {
    const signature = stockScreenFactorRefSignature(filter);
    const previous = seenFilters.get(signature);
    if (previous != null) errors.push({ path: `conditions.${index}.factor`, message: `与条件 ${previous + 1} 完全重复` });
    else seenFilters.set(signature, index);
  });
  const columns = query.columns ?? [];
  columns.forEach((column, index) => validateRef(column, `columns.${index}`, "retrieve"));
  const seenColumns = new Map<string, number>();
  columns.forEach((column, index) => {
    const signature = stockScreenFactorRefSignature(column);
    const previous = seenColumns.get(signature);
    if (previous != null) errors.push({ path: `columns.${index}.factor`, message: `与结果列 ${previous + 1} 完全重复` });
    else seenColumns.set(signature, index);
  });
  const sorts = query.sort ?? [];
  sorts.forEach((sort, index) => {
    validateRef(sort, `sorts.${index}`, "sort");
    if (!sort.direction) errors.push({ path: `sorts.${index}.direction`, message: "请选择排序方向" });
  });
  return errors;
}

export function stockScreenQueryFingerprint(query: StockScreenDraft): string {
  const normalized = cloneStockScreenDraft(query);
  return JSON.stringify(stableValue({
    brokerId: normalized.brokerId ?? "",
    market: normalized.market,
    pool: normalized.pool ?? {},
    filters: normalized.filters ?? [],
    columns: normalized.columns ?? [],
    sort: normalized.sort ?? [],
  }));
}

export function toStockScreenDefinitionV2(
  query: StockScreenDraft,
  catalogVersion: string,
): StockScreenDefinitionV2 {
  const conditions: StockScreenConditionV2[] = (query.filters ?? []).map((filter, index) => {
    const factor = {
      instanceId: stockScreenFactorInstanceId(filter, `condition-${index + 1}`),
      factorKey: factorRefKey(filter),
      ...(filter.params ? { params: cloneParams(filter.params)! } : {}),
    };
    const serializedIntervals = filter.intervals?.length
      ? filter.intervals.map((interval) => ({
          ...(interval.min
            ? {
                min: interval.min.value,
                minIncludes: interval.min.includes !== false,
              }
            : {}),
          ...(interval.max
            ? {
                max: interval.max.value,
                maxIncludes: interval.max.includes !== false,
              }
            : {}),
          ...(interval.unit != null ? { unit: interval.unit } : {}),
        }))
      : undefined;
    const rangeValue = () => ({
      ...(filter.min
        ? {
            min: filter.min.value,
            minIncludes: filter.min.includes !== false,
          }
        : {}),
      ...(filter.max
        ? {
            max: filter.max.value,
            maxIncludes: filter.max.includes !== false,
          }
        : {}),
      ...(serializedIntervals
        ? { intervals: serializedIntervals }
        : {}),
      ...(filter.continuousPeriod != null
        ? { continuousPeriod: filter.continuousPeriod }
        : {}),
    });
    let operator = "between";
    let value: unknown = rangeValue();
    if (filter.position != null) {
      operator = "position";
      value = {
        position: filter.position,
        ...(filter.secondValue != null
          ? { secondValue: filter.secondValue }
          : {}),
        ...(filter.continuousPeriod != null
          ? { continuousPeriod: filter.continuousPeriod }
          : {}),
        ...(serializedIntervals ? { intervals: serializedIntervals } : {}),
      };
    } else if (filter.match != null) {
      operator = "pattern";
      value = {
        match: filter.match,
        values: filter.values ?? [],
        ...(filter.continuousPeriod != null
          ? { continuousPeriod: filter.continuousPeriod }
          : {}),
      };
    } else if (filter.values != null) {
      operator = "in";
      value = filter.values;
    }
    const secondFactor = filter.secondFactor
      ? {
          instanceId: stockScreenFactorInstanceId(filter.secondFactor, `second-${index + 1}`),
          factorKey: factorRefKey(filter.secondFactor),
          ...(filter.secondFactor.params
            ? { params: cloneParams(filter.secondFactor.params)! }
            : {}),
        }
      : undefined;
    return {
      id:
        filter.conditionId ??
        (filter as StockScreenEditorFilter).id ??
        `condition-${index + 1}`,
      factor,
      operator,
      value,
      ...(secondFactor ? { secondFactor } : {}),
    };
  });
  return {
    ...(query.brokerId ? { brokerId: query.brokerId } : {}),
    market: normalizeScreenMarket(query.market),
    ...(query.pool ? { pool: query.pool } : {}),
    conditions,
    columns: (query.columns ?? []).map((column, index) => ({
      columnId: column.columnId ?? `column-${index + 1}`,
      factor: {
        instanceId: stockScreenFactorInstanceId(column, `column-${index + 1}`),
        factorKey: factorRefKey(column),
        ...(column.params ? { params: cloneParams(column.params)! } : {}),
      },
    })),
    sorts: (query.sort ?? []).map((sort, index) => ({
      ...(sort.sortId ? { sortId: sort.sortId } : {}),
      factor: {
        instanceId: stockScreenFactorInstanceId(sort, `sort-${index + 1}`),
        factorKey: factorRefKey(sort),
        ...(sort.params ? { params: cloneParams(sort.params)! } : {}),
      },
      direction: sort.direction,
    })),
    catalogVersion,
    querySchemaVersion: 2,
  };
}

export function moveItem<T>(items: T[], index: number, delta: number): T[] {
  const target = index + delta;
  if (target < 0 || target >= items.length) return items;
  const result = [...items];
  [result[index], result[target]] = [result[target]!, result[index]!];
  return result;
}

export function sameSort(
  left: StockScreenSort[],
  right: StockScreenSort[],
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}
