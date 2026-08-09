import { PineTS } from "pinets";
import { chartTicker, ExtendedTickerProvider } from "./extendedTickerProvider";
import { normalizePineSourceForPineTS } from "./pinetsSource";
import { installOrderIntentCapture, type OrderIntentCapture } from "./pinetsOrderIntents";
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
export type { OrderIntentCapture } from "./pinetsOrderIntents";
export { normalizePineSourceForPineTS } from "./pinetsSource";
