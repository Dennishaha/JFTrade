import type { PreparedRunScriptRequest } from "./types";

type PendingOrderRecord = Record<string, unknown>;

type PineTSOrderExecutionContext = {
  idx?: number;
  strategy?: {
    pending_orders?: unknown[];
    opentrades?: unknown[];
    position_size?: unknown;
  };
};

type PineTSIteration = (context: PineTSOrderExecutionContext) => unknown | Promise<unknown>;

type PineTSOrderRuntime = {
  _executeIterations?: (
    context: PineTSOrderExecutionContext,
    transpiledFn: PineTSIteration,
    startIdx: number,
    endIdx: number,
  ) => Promise<void>;
};

type TrackedPendingOrder = {
  ref: PendingOrderRecord;
  identity: string;
  semantics: unknown[];
};

export type OrderIntentCapture = {
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

export function installOrderIntentCapture(
  pineTS: PineTSOrderRuntime,
  request: PreparedRunScriptRequest,
): OrderIntentCapture {
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
  context: PineTSOrderExecutionContext,
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

function pendingOrders(context: PineTSOrderExecutionContext): PendingOrderRecord[] {
  const orders = context.strategy?.pending_orders;
  if (!Array.isArray(orders)) {
    return [];
  }
  return orders.filter((order): order is PendingOrderRecord =>
    typeof order === "object" && order !== null && pendingOrderStatus(order as PendingOrderRecord) === "pending",
  );
}

function trackPendingOrder(order: PendingOrderRecord, context: PineTSOrderExecutionContext): TrackedPendingOrder {
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
  context: PineTSOrderExecutionContext,
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
  annotateAtomicOrderSemantics(intent, order, category, context, request, barIndex);
  return intent;
}

function annotateAtomicOrderSemantics(
  intent: Record<string, unknown>,
  order: PendingOrderRecord,
  category: string,
  context: PineTSOrderExecutionContext,
  request: PreparedRunScriptRequest,
  barIndex: number,
): void {
  const id = optionalString(order.id) ?? "";
  if (category === "exit") {
    intent.reduceOnly = true;
    const fromEntry = optionalString(order.from_entry);
    if (fromEntry !== undefined) {
      intent.parentId = fromEntry;
      if (pendingEntryOrders(context).some((entry) => optionalString(entry.id) === fromEntry)) {
        intent.atomicGroupId = atomicGroupID(request, barIndex, fromEntry);
      }
    }
    if (optionalPositiveNumber(order.limit) !== undefined && optionalPositiveNumber(order.stop) !== undefined) {
      intent.ocoGroupId = ocoGroupID(request, barIndex, id);
      if (intent.atomicGroupId === undefined) {
        intent.atomicGroupId = intent.ocoGroupId;
      }
    }
    return;
  }

  if (id !== "" && pendingOrders(context).some((candidate) =>
    (optionalString(candidate.category) ?? "entry") === "exit" && optionalString(candidate.from_entry) === id
  )) {
    intent.atomicGroupId = atomicGroupID(request, barIndex, id);
  }
}

function atomicGroupID(request: PreparedRunScriptRequest, barIndex: number, entryId: string): string {
  return ["pine", request.symbol, String(barIndex), "parent", entryId].join(":");
}

function ocoGroupID(request: PreparedRunScriptRequest, barIndex: number, exitId: string): string {
  return ["pine", request.symbol, String(barIndex), "oco", exitId].join(":");
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
  context: PineTSOrderExecutionContext,
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

function resolveExitTarget(order: PendingOrderRecord, context: PineTSOrderExecutionContext): ExitTarget {
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
      return exitTargetFromPendingEntries(matchingPendingEntries, order);
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

function exitTargetFromPendingEntries(entries: PendingOrderRecord[], order: PendingOrderRecord): ExitTarget {
  const signedQuantities = entries.map((entry) => {
    const quantity = optionalPositiveNumber(entry.qty);
    if (quantity === undefined) {
      throw new Error(
        `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references a pending entry without a positive quantity`,
      );
    }
    const direction = entry.direction;
    if (direction === 1 || direction === "long" || direction === "buy") return quantity;
    if (direction === -1 || direction === "short" || direction === "sell") return -quantity;
    throw new Error(
      `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} references a pending entry with an unsupported direction`,
    );
  });
  return exitTargetFromSignedQuantities(signedQuantities, order);
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

function pendingEntryOrders(context: PineTSOrderExecutionContext): PendingOrderRecord[] {
  return pendingOrders(context)
    .filter((pending) => (optionalString(pending.category) ?? "entry") !== "exit");
}

function throwUnsupportedPendingEntryExit(order: PendingOrderRecord): never {
  throw new Error(
    `Pine strategy exit ${JSON.stringify(optionalString(order.id) ?? "")} depends on an unfilled entry; ` +
    "the worker cannot bind this exit to one unique parent entry for atomic placement",
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
