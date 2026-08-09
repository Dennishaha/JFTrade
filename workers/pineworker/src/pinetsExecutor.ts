import { PineTS } from "pinets";
import { chartTicker, ExtendedTickerProvider } from "./extendedTickerProvider";
import { normalizePineSourceForPineTS } from "./pinetsSource";
import { preflightStaticRequestSecurityRoutes } from "./pinetsStaticPreflight";
import {
  assertSameLiveSessionDefinition,
  compactPineTSResult,
  incrementalResult,
  resultMarker,
} from "./pinetsResult";
import {
  normalizeChartType,
  type PineTSExecutor,
  type PineTSPlot,
  type PineTSRunResult,
  type PreparedRunScriptRequest,
} from "./types";
type PineTSModule = {
  PineTS: new (source: any, symbol?: string, timeframe?: string, periods?: number) => PineTSRuntime;
};
export type PineTSExecutionContext = {
  idx?: number;
  length?: number;
  dataVersion?: number;
  cache?: Record<string, unknown>;
  data?: {
    openTime?: { data?: unknown[] };
  };
  params?: Record<string, unknown>;
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
  updateTail?: (context: PineTSExecutionContext) => Promise<boolean>;
};

type LivePineTSRuntime = PineTSRuntime & {
  updateTail(context: PineTSExecutionContext): Promise<boolean>;
};

type PendingOrderRecord = Record<string, unknown>;

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

type NativeLiveSession = {
  runtime: LivePineTSRuntime;
  context: PineTSExecutionContext & PineTSRunResult;
  capture: OrderIntentCapture;
  request: PreparedRunScriptRequest;
  provider: ExtendedTickerProvider;
  revision: number;
  queue: Promise<void>;
  failed: boolean;
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

const secondaryRefreshPatched = Symbol("secondaryRefreshPatched");

export class NativePineTSExecutor implements PineTSExecutor {
  private readonly liveSessions = new Map<string, NativeLiveSession>();

  constructor(private readonly module: PineTSModule, private readonly pineTSVersion = "unknown") {}

  version(): string {
    return this.pineTSVersion;
  }

  async run(request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    const execution = await this.createExecution(request);
    return compactPineTSResult(execution.context, request.includePlots !== false);
  }

  async openLiveSession(sessionId: string, request: PreparedRunScriptRequest): Promise<PineTSRunResult> {
    if (this.liveSessions.has(sessionId)) {
      throw new Error(`PineTS live session ${JSON.stringify(sessionId)} already exists`);
    }
    if ((request.expectedRevision ?? 0) !== 0) {
      throw new Error("PineTS live session open requires expected revision 0");
    }
    const execution = await this.createExecution(request);
    if (!isLivePineTSRuntime(execution.runtime)) {
      throw new Error("PineTS runtime does not expose the update-tail hook required for stateful live sessions");
    }
    this.liveSessions.set(sessionId, {
      ...execution,
      runtime: execution.runtime,
      revision: 1,
      queue: Promise.resolve(),
      failed: false,
    });
    // Warmup establishes state only. Historical intents must never escape as
    // live orders when a session is opened.
    const result = compactPineTSResult(execution.context, request.includePlots !== false);
    result.orderIntents = [];
    return result;
  }

  async appendLiveSession(
    sessionId: string,
    expectedRevision: number,
    request: PreparedRunScriptRequest,
  ): Promise<{ result: PineTSRunResult; revision: number }> {
    const session = this.liveSessions.get(sessionId);
    if (session === undefined || session.failed) {
      throw new Error(`PineTS live session ${JSON.stringify(sessionId)} is not available`);
    }
    return this.withLiveSessionLock(session, async () => {
      if (this.liveSessions.get(sessionId) !== session || session.failed) {
        throw new Error(`PineTS live session ${JSON.stringify(sessionId)} is not available`);
      }
      if (session.revision !== expectedRevision) {
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} revision mismatch: expected ${expectedRevision}, current ${session.revision}`,
        );
      }
      assertSameLiveSessionDefinition(session.request, request);
      const lastOpenTime = session.request.candles[session.request.candles.length - 1]?.openTime ?? 0;
      let previousOpenTime = lastOpenTime;
      for (const candle of request.candles) {
        if (candle.openTime <= previousOpenTime) {
          throw new Error(
            `PineTS live session ${JSON.stringify(sessionId)} requires strictly increasing closed candle open times`,
          );
        }
        previousOpenTime = candle.openTime;
      }

      const marker = resultMarker(session.context, session.capture);
      try {
        for (const candle of request.candles) {
          session.provider.append(candle);
          session.request.candles.push({ ...candle });
          const updated = await session.runtime.updateTail(session.context);
          if (!updated) {
            throw new Error("Pineworker runtime did not update its tail after a closed candle append");
          }
          stabilizeSecondaryContexts(session.context);
        }
      } catch (error) {
        session.failed = true;
        this.liveSessions.delete(sessionId);
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} was invalidated after an append failure: ${String(error instanceof Error ? error.message : error)}`,
        );
      }
      session.revision++;
      return {
        result: incrementalResult(
          session.context,
          session.capture,
          marker,
          request.includePlots !== false,
          request.candles.length,
        ),
        revision: session.revision,
      };
    });
  }

  async closeLiveSession(sessionId: string, expectedRevision: number): Promise<number> {
    const session = this.liveSessions.get(sessionId);
    if (session === undefined) {
      return expectedRevision;
    }
    return this.withLiveSessionLock(session, async () => {
      if (this.liveSessions.get(sessionId) !== session) {
        return expectedRevision;
      }
      if (expectedRevision !== 0 && session.revision !== expectedRevision) {
        throw new Error(
          `PineTS live session ${JSON.stringify(sessionId)} revision mismatch: expected ${expectedRevision}, current ${session.revision}`,
        );
      }
      this.liveSessions.delete(sessionId);
      return session.revision;
    });
  }

  private async createExecution(request: PreparedRunScriptRequest): Promise<{
    runtime: PineTSRuntime;
    context: PineTSExecutionContext & PineTSRunResult;
    capture: OrderIntentCapture;
    request: PreparedRunScriptRequest;
    provider: ExtendedTickerProvider;
  }> {
    const periods = Math.max(1, request.candles.length);
    const provider = new ExtendedTickerProvider(request.symbol, request.timeframe, request.candles);
    const mainTicker = chartTicker(request.symbol, normalizeChartType(request.chartType));
    provider.assertCanServe(mainTicker, request.timeframe);
    preflightStaticRequestSecurityRoutes(request.source, request.timeframe, mainTicker, provider);
    const pineTS = new this.module.PineTS(
      provider,
      mainTicker,
      request.timeframe,
      periods,
    );
    pineTS.setAlertMode?.("all");
    const orderCapture = installOrderIntentCapture(pineTS, request);
    const result = await pineTS.run(normalizePineSourceForPineTS(request.source), periods);
    initializeContextDataVersion(result as PineTSExecutionContext);
    stabilizeSecondaryContexts(result as PineTSExecutionContext);
    if (result.strategy !== undefined) {
      if (!orderCapture.supported) {
        throw new Error("PineTS runtime does not expose the per-bar execution hook required for safe strategy order capture");
      }
      if (result.orderIntents === undefined) {
        result.orderIntents = orderCapture.intents;
      }
    }
    return {
      runtime: pineTS,
      context: result as PineTSExecutionContext & PineTSRunResult,
      capture: orderCapture,
      request,
      provider,
    };
  }

  private async withLiveSessionLock<T>(session: NativeLiveSession, operation: () => Promise<T>): Promise<T> {
    const previous = session.queue;
    let release!: () => void;
    session.queue = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    try {
      return await operation();
    } finally {
      release();
    }
  }
}

function isLivePineTSRuntime(runtime: PineTSRuntime): runtime is LivePineTSRuntime {
  return typeof runtime.updateTail === "function";
}

function initializeContextDataVersion(context: PineTSExecutionContext): void {
  context.dataVersion ??= 0;
  for (const cached of Object.values(context.cache ?? {})) {
    if (typeof cached !== "object" || cached === null) {
      continue;
    }
    const entry = cached as { pineTS?: unknown; context?: unknown; dataVersion?: unknown };
    if (entry.pineTS !== undefined && entry.context !== undefined && entry.dataVersion === undefined) {
      entry.dataVersion = context.dataVersion;
    }
  }
}

function stabilizeSecondaryContexts(context: PineTSExecutionContext): void {
  for (const cached of Object.values(context.cache ?? {})) {
    if (typeof cached !== "object" || cached === null) {
      continue;
    }
    const entry = cached as { pineTS?: PineTSRuntime; context?: PineTSExecutionContext };
    const runtime = entry.pineTS;
    const secondaryContext = entry.context;
    if (runtime === undefined || secondaryContext === undefined) {
      continue;
    }
    alignSecondaryParams(secondaryContext);
    const markedRuntime = runtime as PineTSRuntime & { [secondaryRefreshPatched]?: boolean };
    if (markedRuntime[secondaryRefreshPatched] || runtime.updateTail === undefined) {
      continue;
    }
    const updateTail = runtime.updateTail;
    runtime.updateTail = async (updatedContext) => {
      const changed = await updateTail.call(runtime, updatedContext);
      if (changed) {
        alignSecondaryParams(updatedContext);
      }
      return changed;
    };
    markedRuntime[secondaryRefreshPatched] = true;
  }
}

// PineTS restores a tail snapshot when it refreshes a secondary context. Its
// request parameter arrays remain cumulative, while its timestamp arrays are
// tail-aligned. Keep those indices aligned for request.security lookups.
function alignSecondaryParams(context: PineTSExecutionContext): void {
  const dataLength = context.data?.openTime?.data?.length ?? 0;
  if (dataLength === 0) {
    return;
  }
  for (const value of Object.values(context.params ?? {})) {
    if (Array.isArray(value) && value.length > dataLength) {
      value.splice(0, value.length - dataLength);
    }
  }
}

export async function createNativePineTSExecutor(version = "unknown"): Promise<NativePineTSExecutor> {
  return new NativePineTSExecutor({ PineTS: PineTS as unknown as PineTSModule["PineTS"] }, version);
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
  annotateAtomicOrderSemantics(intent, order, category, context, request, barIndex);
  return intent;
}

function annotateAtomicOrderSemantics(
  intent: Record<string, unknown>,
  order: PendingOrderRecord,
  category: string,
  context: PineTSExecutionContext,
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

function pendingEntryOrders(context: PineTSExecutionContext): PendingOrderRecord[] {
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

export { normalizePineSourceForPineTS } from "./pinetsSource";
