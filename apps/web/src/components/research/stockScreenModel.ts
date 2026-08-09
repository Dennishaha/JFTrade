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

import {
  cloneParams,
  cloneStockScreenColumn,
  cloneStockScreenDraft,
  cloneStockScreenFilter,
  cloneStockScreenSort,
  factorRefKey,
  normalizeScreenMarket,
  parameterLabel,
  stableValue,
  stockScreenFactorRefSignature,
  stockScreenFactorInstanceId,
  stockScreenValueData,
} from "./stockScreenModelBasics";

export interface StockScreenValidationError {
  path: string;
  message: string;
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

export {
  cloneStockScreenColumn,
  cloneStockScreenDraft,
  cloneStockScreenFilter,
  cloneStockScreenSort,
  createStockScreenFilter,
  defaultParameterValue,
  factorEnumName,
  factorRefKey,
  formatStockScreenValue,
  normalizeScreenMarket,
  parameterLabel,
  sameStockScreenFactorRef,
  stockScreenFactorRefSignature,
  stockScreenFactorInstanceId,
  stockScreenValueData,
  stockScreenValueTitle,
  toStockScreenDraftFilter,
} from "./stockScreenModelBasics";
