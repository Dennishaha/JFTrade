import { PineTS } from "pinets";
import type { PineTSExecutor, PineTSRunResult, PreparedRunScriptRequest } from "./types";

type PineTSModule = {
  PineTS: new (candles: unknown[], symbol?: string, timeframe?: string, periods?: number) => PineTSRuntime;
};

type PineTSExecutionContext = {
  idx?: number;
  strategy?: {
    pending_orders?: unknown[];
    opentrades?: unknown[];
    position_size?: unknown;
  };
};

type PineTSIteration = (context: PineTSExecutionContext) => unknown | Promise<unknown>;

type PineTSRuntime = {
  setAlertMode?: (mode: "all" | "realtime") => void;
  run(source: string, periods?: number): Promise<PineTSRunResult>;
  _executeIterations?: (
    context: PineTSExecutionContext,
    transpiledFn: PineTSIteration,
    startIdx: number,
    endIdx: number,
  ) => Promise<void>;
};

type PendingOrderRecord = Record<string, unknown>;

type TrackedPendingOrder = {
  ref: PendingOrderRecord;
  identity: string;
  semantics: unknown[];
};

type OrderIntentCapture = {
  supported: boolean;
  intents: Record<string, unknown>[];
  previous: TrackedPendingOrder[];
};

type ResolvedPendingOrder = {
  direction: "long" | "short";
  quantity?: number;
  quantityPct?: number;
};

type ExitTarget = {
  direction: "long" | "short";
  quantity: number;
};

const pendingOrderSemanticFields = [
  "id",
  "category",
  "from_entry",
  "direction",
  "qty",
  "qty_percent",
  "type",
  "limit",
  "stop",
  "profit",
  "loss",
  "trail_price",
  "trail_points",
  "trail_offset",
  "oca_name",
  "oca_type",
  "comment",
  "alert_message",
  "disable_alert",
  "immediately",
] as const;

export class NativePineTSExecutor implements PineTSExecutor {
  constructor(private readonly module: PineTSModule, private readonly pineTSVersion = "unknown") {}

  version(): string {
    return this.pineTSVersion;
  }

  async run(request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    const periods = Math.max(1, request.candles.length);
    const pineTS = new this.module.PineTS(
      request.candles as Record<string, number>[],
      request.symbol,
      request.timeframe,
      periods,
    );
    pineTS.setAlertMode?.("all");
    const orderCapture = installOrderIntentCapture(pineTS, request);
    const result = await pineTS.run(normalizePineSourceForPineTS(request.source), periods);
    if (result.strategy !== undefined) {
      if (!orderCapture.supported) {
        throw new Error("PineTS runtime does not expose the per-bar execution hook required for safe strategy order capture");
      }
      if (result.orderIntents === undefined) {
        result.orderIntents = orderCapture.intents;
      }
    }
    return compactPineTSResult(result, request.includePlots !== false);
  }
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  return new NativePineTSExecutor({ PineTS: PineTS as unknown as PineTSModule["PineTS"] }, version);
}

function compactPineTSResult(result: PineTSRunResult, includePlots: boolean): PineTSRunResult {
  const compact: PineTSRunResult = {};
  if (includePlots && result.plots !== undefined) compact.plots = result.plots;
  if (result.alerts !== undefined) compact.alerts = result.alerts;
  if (result.visualOutputs !== undefined) compact.visualOutputs = result.visualOutputs;
  if (result.drawings !== undefined) compact.drawings = result.drawings;
  if (result.logs !== undefined) compact.logs = result.logs;
  if (result.warnings !== undefined) compact.warnings = result.warnings;
  if (result.diagnostics !== undefined) compact.diagnostics = result.diagnostics;
  if (result.orderIntents !== undefined) compact.orderIntents = result.orderIntents;
  if (result.strategy !== undefined) compact.strategy = compactStrategyResult(result.strategy);
  return compact;
}

function compactStrategyResult(value: unknown): unknown {
  if (typeof value !== "object" || value === null) {
    return value;
  }
  const source = value as Record<string, unknown>;
  return {
    closedtrades: compactTrades(source.closedtrades, true),
    opentrades: compactTrades(source.opentrades, false),
    buy_and_hold_pnl: source.buy_and_hold_pnl ?? source.buyAndHoldPnl,
    buy_and_hold_per_gain: source.buy_and_hold_per_gain ?? source.buyAndHoldPerGain,
    strategy_outperformance: source.strategy_outperformance ?? source.strategyOutperformance,
  };
}

function compactTrades(value: unknown, includeExit: boolean): unknown[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => {
    if (typeof item !== "object" || item === null) {
      return [];
    }
    const source = item as Record<string, unknown>;
    const trade: Record<string, unknown> = {
      entry_id: source.entry_id,
      entry_bar_index: source.entry_bar_index,
      size: source.size,
    };
    if (includeExit) {
      trade.exit_id = source.exit_id;
      trade.exit_bar_index = source.exit_bar_index;
    }
    return [trade];
  });
}

function installOrderIntentCapture(pineTS: PineTSRuntime, request: PreparedRunScriptRequest): OrderIntentCapture {
  const capture: OrderIntentCapture = { supported: false, intents: [], previous: [] };
  const executeIterations = pineTS._executeIterations;
  if (typeof executeIterations !== "function") {
    return capture;
  }

  capture.supported = true;
  // PineTS deletes filled orders from its final strategy state. Capture after
  // each script bar, while preserving the runtime's normal execution path.
  pineTS._executeIterations = async (context, transpiledFn, startIdx, endIdx) => {
    const capturingFn: PineTSIteration = async (iterationContext) => {
      const result = await transpiledFn(iterationContext);
      capturePendingOrderEvents(iterationContext, request, capture);
      return result;
    };
    await executeIterations.call(pineTS, context, capturingFn, startIdx, endIdx);
  };
  return capture;
}

function capturePendingOrderEvents(
  context: PineTSExecutionContext,
  request: PreparedRunScriptRequest,
  capture: OrderIntentCapture,
): void {
  if (context.strategy === undefined) {
    return;
  }
  const barIndex = integerOr(context.idx, request.candles.length - 1);
  const current = pendingOrders(context).map((order) => trackPendingOrder(order, context));
  const matchedPrevious = new Set<number>();
  const cancellations = new Map<string, Record<string, unknown>>();
  const placements: Record<string, unknown>[] = [];

  for (const order of current) {
    const exactIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) && previous.ref === order.ref,
    );
    if (exactIndex >= 0) {
      matchedPrevious.add(exactIndex);
      const previous = capture.previous[exactIndex]!;
      if (!sameOrderSemantics(previous.semantics, order.semantics)) {
        addCancellation(cancellations, previous.ref, request, barIndex);
        placements.push(orderIntentFromPendingOrder(order.ref, context, request, barIndex));
      }
      continue;
    }

    const unchangedIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) &&
      pendingOrderStatus(previous.ref) === "pending" &&
      previous.identity === order.identity &&
      sameOrderSemantics(previous.semantics, order.semantics),
    );
    if (unchangedIndex >= 0) {
      matchedPrevious.add(unchangedIndex);
      continue;
    }

    const modifiedIndex = capture.previous.findIndex((previous, index) =>
      !matchedPrevious.has(index) &&
      pendingOrderStatus(previous.ref) === "pending" &&
      previous.identity === order.identity,
    );
    if (modifiedIndex >= 0) {
      matchedPrevious.add(modifiedIndex);
      addCancellation(cancellations, capture.previous[modifiedIndex]!.ref, request, barIndex);
    }
    placements.push(orderIntentFromPendingOrder(order.ref, context, request, barIndex));
  }

  for (let index = 0; index < capture.previous.length; index++) {
    if (matchedPrevious.has(index)) {
      continue;
    }
    const previous = capture.previous[index]!;
    if (pendingOrderStatus(previous.ref) !== "filled") {
      addCancellation(cancellations, previous.ref, request, barIndex);
    }
  }

  capture.intents.push(...cancellations.values(), ...placements);
  capture.previous = current;
}

function pendingOrders(context: PineTSExecutionContext): PendingOrderRecord[] {
  const orders = context.strategy?.pending_orders;
  if (!Array.isArray(orders)) {
    return [];
  }
  return orders.filter((order): order is PendingOrderRecord =>
    typeof order === "object" && order !== null && pendingOrderStatus(order as PendingOrderRecord) === "pending",
  );
}

function trackPendingOrder(order: PendingOrderRecord, context: PineTSExecutionContext): TrackedPendingOrder {
  const category = optionalString(order.category) ?? "entry";
  const resolved = resolvePendingOrder(order, category, context);
  return {
    ref: order,
    identity: [
      optionalString(order.category) ?? "entry",
      optionalString(order.id) ?? "",
      optionalString(order.from_entry) ?? "",
    ].join("\u0000"),
    // Deliberately excludes placement bar/time and PineTS bookkeeping so an
    // unchanged strategy.exit refresh does not become a duplicate broker order.
    semantics: [
      ...pendingOrderSemanticFields.map((field) => order[field]),
      resolved.direction,
      resolved.quantity,
      resolved.quantityPct,
    ],
  };
}

function sameOrderSemantics(left: unknown[], right: unknown[]): boolean {
  return left.length === right.length && left.every((value, index) => Object.is(value, right[index]));
}

function pendingOrderStatus(order: PendingOrderRecord): string {
  return optionalString(order.status) ?? "pending";
}

function addCancellation(
  cancellations: Map<string, Record<string, unknown>>,
  order: PendingOrderRecord,
  request: PreparedRunScriptRequest,
  barIndex: number,
): void {
  const id = optionalString(order.id);
  if (id === undefined || cancellations.has(id)) {
    return;
  }
  cancellations.set(id, {
    kind: "cancel",
    id,
    barIndex,
    time: candleOpenTime(request, barIndex),
  });
}

function orderIntentFromPendingOrder(
  order: PendingOrderRecord,
  context: PineTSExecutionContext,
  request: PreparedRunScriptRequest,
  barIndex: number,
): Record<string, unknown> {
  const id = optionalString(order.id) ?? "";
  rejectUnsupportedConditionalExit(order, id);

  const category = optionalString(order.category) ?? "entry";
  validatePendingOrderPriceFields(order, id);
  if (category !== "exit" && !(typeof order.qty === "number" && Number.isFinite(order.qty) && order.qty > 0)) {
    throw new Error(`Pine strategy entry order ${JSON.stringify(id)} has no positive quantity`);
  }
  const resolved = resolvePendingOrder(order, category, context);
  const intent: Record<string, unknown> = {
    kind: category === "exit" ? "exit" : "entry",
    id,
    direction: resolved.direction,
    barIndex,
    time: candleOpenTime(request, barIndex),
  };
  setIntentString(intent, "fromEntry", order.from_entry);
  setPositiveIntentNumber(intent, "quantity", resolved.quantity);
  setPositiveIntentNumber(intent, "quantityPct", resolved.quantityPct);
  setIntentNumber(intent, "limitPrice", order.limit);
  setIntentNumber(intent, "stopPrice", order.stop);
  setIntentString(intent, "comment", order.comment);
  setIntentString(intent, "alertMessage", order.alert_message);
  if (typeof order.disable_alert === "boolean") {
    intent.disableAlert = order.disable_alert;
  }
  return intent;
}

function rejectUnsupportedConditionalExit(order: PendingOrderRecord, id: string): void {
  if ((optionalString(order.category) ?? "entry") !== "exit") {
    return;
  }
  const unsupported = ["profit", "loss", "trail_price", "trail_points", "trail_offset"]
    .filter((field) => order[field] !== undefined);
  if (unsupported.length > 0) {
    throw new Error(
      `Pine strategy exit ${JSON.stringify(id)} uses unsupported conditional fields: ${unsupported.join(", ")}; ` +
      "the worker cannot safely convert tick-based or trailing exits to broker prices",
    );
  }
}

function validatePendingOrderPriceFields(order: PendingOrderRecord, id: string): void {
  const type = optionalString(order.type) ?? "market";
  const hasPositivePrice = (value: unknown): boolean =>
    typeof value === "number" && Number.isFinite(value) && value > 0;
  if ((type === "limit" || type === "stop-limit") && !hasPositivePrice(order.limit)) {
    throw new Error(`Pine strategy order ${JSON.stringify(id)} has a ${type} type without a valid limit price`);
  }
  if ((type === "stop" || type === "stop-limit") && !hasPositivePrice(order.stop)) {
    throw new Error(`Pine strategy order ${JSON.stringify(id)} has a ${type} type without a valid stop price`);
  }
}

function resolvePendingOrder(
  order: PendingOrderRecord,
  category: string,
  context: PineTSExecutionContext,
): ResolvedPendingOrder {
  if (category === "exit") {
    const target = resolveExitTarget(order, context);
    const rawQuantity = optionalPositiveNumber(order.qty);
    if (order.qty !== undefined && order.qty !== 0 && rawQuantity === undefined) {
      throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid quantity`);
    }
    const rawQuantityPct = optionalPositiveNumber(order.qty_percent);
    if (order.qty_percent !== undefined && rawQuantityPct === undefined) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid quantity percent`,
      );
    }
    const requestedQuantity = rawQuantity ?? (
      rawQuantityPct === undefined ? target.quantity : target.quantity * Math.min(rawQuantityPct, 100) / 100
    );
    const quantity = Math.min(requestedQuantity, target.quantity);
    if (!(Number.isFinite(quantity) && quantity > 0)) {
      throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no positive target quantity`);
    }
    return { direction: target.direction, quantity };
  }
  const value = order.direction;
  if (value === 1 || value === "long" || value === "buy") {
    return resolvedEntryOrder("long", order);
  }
  if (value === -1 || value === "short" || value === "sell") {
    return resolvedEntryOrder("short", order);
  }
  throw new Error(`Pine strategy entry order has unsupported direction: ${String(value)}`);
}

function resolvedEntryOrder(direction: "long" | "short", order: PendingOrderRecord): ResolvedPendingOrder {
  const resolved: ResolvedPendingOrder = { direction };
  const quantity = optionalPositiveNumber(order.qty);
  const quantityPct = optionalPositiveNumber(order.qty_percent);
  if (quantity !== undefined) resolved.quantity = quantity;
  if (quantityPct !== undefined) resolved.quantityPct = quantityPct;
  return resolved;
}

function resolveExitTarget(order: PendingOrderRecord, context: PineTSExecutionContext): ExitTarget {
  const strategy = context.strategy;
  const openTrades = recordArray(strategy?.opentrades);
  const intendedTradeIDs = stringArray(order._intended_trade_ids);
  if (intendedTradeIDs.length > 0) {
    const intendedIDs = new Set(intendedTradeIDs);
    const intendedTrades = openTrades.filter((trade) => intendedIDs.has(optionalString(trade.id) ?? ""));
    const matchedIDs = new Set(intendedTrades.flatMap((trade) => {
      const id = optionalString(trade.id);
      return id === undefined ? [] : [id];
    }));
    if (matchedIDs.size !== intendedIDs.size) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has unresolved intended trades`,
      );
    }
    return exitTargetFromSignedQuantities(intendedTrades.map((trade) => trade.size), order);
  }

  const fromEntry = optionalString(order.from_entry);
  if (fromEntry !== undefined) {
    const matchingPendingEntries = pendingEntryOrders(context)
      .filter((pending) => optionalString(pending.id) === fromEntry);
    if (matchingPendingEntries.length > 0) {
      throwUnsupportedPendingEntryExit(order);
    }
    const signedQuantities = openTrades
      .filter((trade) => optionalString(trade.entry_id) === fromEntry)
      .map((trade) => trade.size);
    if (signedQuantities.length > 0) {
      return exitTargetFromSignedQuantities(signedQuantities, order);
    }
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references entry ` +
      `${JSON.stringify(fromEntry)} without a matching open or pending trade`,
    );
  }

  if (pendingEntryOrders(context).length > 0) {
    throwUnsupportedPendingEntryExit(order);
  }
  const positionSize = strategy?.position_size;
  if (directionFromSignedNumber(positionSize) !== undefined) {
    return exitTargetFromSignedQuantities([positionSize], order);
  }
  throw new Error(
    `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no provable long/short position direction`,
  );
}

function exitTargetFromSignedQuantities(values: unknown[], order: PendingOrderRecord): ExitTarget {
  const signedQuantities = values.filter((value): value is number =>
    typeof value === "number" && Number.isFinite(value) && value !== 0,
  );
  if (signedQuantities.length !== values.length) {
    throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has invalid target size`);
  }
  const directions = new Set(signedQuantities.map((value) => value > 0 ? "long" : "short"));
  if (directions.size !== 1) {
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has ambiguous target direction`,
    );
  }
  const quantity = signedQuantities.reduce((sum, value) => sum + Math.abs(value), 0);
  if (!(Number.isFinite(quantity) && quantity > 0)) {
    throw new Error(`Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} has no positive target quantity`);
  }
  return { direction: directions.values().next().value!, quantity };
}

function pendingEntryOrders(context: PineTSExecutionContext): PendingOrderRecord[] {
  return pendingOrders(context)
    .filter((pending) => (optionalString(pending.category) ?? "entry") !== "exit");
}

function throwUnsupportedPendingEntryExit(order: PendingOrderRecord): never {
  throw new Error(
    `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} depends on an unfilled entry; ` +
    "the order protocol cannot atomically express a parent-linked or reduce-only protective exit",
  );
}

function directionFromSignedNumber(value: unknown): "long" | "short" | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value === 0) {
    return undefined;
  }
  return value > 0 ? "long" : "short";
}

function optionalPositiveNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

function recordArray(value: unknown): PendingOrderRecord[] {
  return Array.isArray(value)
    ? value.filter((item): item is PendingOrderRecord => typeof item === "object" && item !== null)
    : [];
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item !== "") : [];
}

function setIntentString(intent: Record<string, unknown>, key: string, value: unknown): void {
  const normalized = optionalString(value);
  if (normalized !== undefined) {
    intent[key] = normalized;
  }
}

function setPositiveIntentNumber(intent: Record<string, unknown>, key: string, value: unknown): void {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    intent[key] = value;
  }
}

function setIntentNumber(intent: Record<string, unknown>, key: string, value: unknown): void {
  if (typeof value === "number" && Number.isFinite(value)) {
    intent[key] = value;
  }
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function integerOr(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isInteger(value) ? value : fallback;
}

function candleOpenTime(request: PreparedRunScriptRequest, barIndex: number): number {
  return request.candles[barIndex]?.openTime ?? 0;
}

export function normalizePineSourceForPineTS(source: string): string {
  let output = "";
  let index = 0;
  let state: "code" | "line_comment" | "block_comment" | "single_quote" | "double_quote" = "code";
  while (index < source.length) {
    const char = source[index];
    const next = source[index + 1];
    if (state === "code") {
      if (char === "/" && next === "/") {
        output += "//";
        index += 2;
        state = "line_comment";
        continue;
      }
      if (char === "/" && next === "*") {
        output += "/*";
        index += 2;
        state = "block_comment";
        continue;
      }
      if (char === "'") {
        output += char;
        index++;
        state = "single_quote";
        continue;
      }
      if (char === "\"") {
        output += char;
        index++;
        state = "double_quote";
        continue;
      }
      if (source.startsWith("timenow", index) && !isIdentifierChar(source[index - 1]) && !isIdentifierChar(source[index + "timenow".length])) {
        output += "time_close";
        index += "timenow".length;
        continue;
      }
      output += char;
      index++;
      continue;
    }
    output += char;
    index++;
    if (state === "line_comment" && char === "\n") {
      state = "code";
    } else if (state === "block_comment" && char === "*" && next === "/") {
      output += next;
      index++;
      state = "code";
    } else if (state === "single_quote" && char === "'" && source[index - 2] !== "\\") {
      state = "code";
    } else if (state === "double_quote" && char === "\"" && source[index - 2] !== "\\") {
      state = "code";
    }
  }
  return output;
}

function isIdentifierChar(value: string | undefined): boolean {
  return value !== undefined && /[A-Za-z0-9_]/.test(value);
}
