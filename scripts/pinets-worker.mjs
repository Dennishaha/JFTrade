#!/usr/bin/env node
import { createInterface } from "node:readline";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";

const require = createRequire(new URL("../workers/pineworker/package.json", import.meta.url));
const pinetsEntry = require.resolve("pinets");
const { PineTS } = await import(pathToFileURL(pinetsEntry).href);
const pinetsPackage = JSON.parse(readFileSync(join(dirname(dirname(pinetsEntry)), "package.json"), "utf8"));

const DEFAULT_TIMEOUT_MS = 10_000;

function normalizeCandle(raw, index, timeframe) {
  const openTime = Number(raw?.openTime ?? raw?.time ?? 0);
  const closeTime = Number(raw?.closeTime ?? (openTime + timeframeDurationMs(timeframe)));
  return {
    openTime,
    closeTime,
    open: Number(raw?.open),
    high: Number(raw?.high),
    low: Number(raw?.low),
    close: Number(raw?.close),
    volume: Number(raw?.volume ?? 0),
    quoteAssetVolume: Number(raw?.quoteAssetVolume ?? 0),
    numberOfTrades: Number(raw?.numberOfTrades ?? 0),
    takerBuyBaseAssetVolume: Number(raw?.takerBuyBaseAssetVolume ?? 0),
    takerBuyQuoteAssetVolume: Number(raw?.takerBuyQuoteAssetVolume ?? 0),
    ignore: raw?.ignore ?? index,
  };
}

function timeframeDurationMs(value) {
  const normalized = String(value || "1m").trim().toLowerCase();
  const match = normalized.match(/^(\d+)?([smhdw])$/);
  if (!match) return 60_000;
  const amount = Number(match[1] || 1);
  switch (match[2]) {
    case "s":
      return amount * 1_000;
    case "m":
      return amount * 60_000;
    case "h":
      return amount * 3_600_000;
    case "d":
      return amount * 86_400_000;
    case "w":
      return amount * 7 * 86_400_000;
    default:
      return 60_000;
  }
}

function normalizePlotValue(value) {
  if (value == null) return value;
  if (typeof value !== "object") return value;
  if (Object.prototype.hasOwnProperty.call(value, "value")) return value.value;
  return value;
}

function normalizePlots(plots) {
  const out = {};
  for (const [name, plot] of Object.entries(plots ?? {})) {
    if (name.startsWith("__")) continue;
    const data = Array.isArray(plot?.data) ? plot.data : [];
    out[name] = {
      title: plot?.title ?? name,
      data: data.map(normalizePlotValue),
    };
  }
  return out;
}

function lastSignals(plots) {
  const out = {};
  for (const [name, plot] of Object.entries(plots)) {
    const data = Array.isArray(plot.data) ? plot.data : [];
    out[name] = data.length ? data[data.length - 1] : null;
  }
  return out;
}

function diagnosticsFromError(error) {
  return [
    {
      severity: "error",
      code: error?.name || "PINETS_RUNTIME_ERROR",
      message: String(error?.message || error),
      line: 1,
      column: 1,
      endLine: 1,
      endColumn: 1,
    },
  ];
}

function sampleCandles(timeframe) {
  const start = 1_704_067_200_000;
  const step = timeframeDurationMs(timeframe);
  return Array.from({ length: 80 }, (_, index) => {
    const close = 100 + index + Math.sin(index / 3);
    return normalizeCandle(
      {
        openTime: start + index * step,
        closeTime: start + (index + 1) * step,
        open: close - 0.4,
        high: close + 1,
        low: close - 1,
        close,
        volume: 1_000 + index,
      },
      index,
      timeframe,
    );
  });
}

async function withTimeout(promise, timeoutMs) {
  let timeout;
  const timeoutPromise = new Promise((_, reject) => {
    timeout = setTimeout(() => reject(new Error(`pinets worker timed out after ${timeoutMs}ms`)), timeoutMs);
  });
  try {
    return await Promise.race([promise, timeoutPromise]);
  } finally {
    clearTimeout(timeout);
  }
}

async function runIndicator(params = {}) {
  const timeframe = String(params.timeframe || "1m");
  if (Array.isArray(params.candles) && params.candles.length === 0) {
    throw new Error("pinets worker requires at least one candle when candles is provided");
  }
  const rawCandles = Array.isArray(params.candles) && params.candles.length ? params.candles : sampleCandles(timeframe);
  const candles = rawCandles.map((item, index) => normalizeCandle(item, index, timeframe));
  const requestedPeriods = Number(params.periods || 0);
  const periods = Math.max(1, Math.min(requestedPeriods > 0 ? requestedPeriods : candles.length, candles.length));
  const pineTS = new PineTS(candles, String(params.symbol || "JFTRADE.SAMPLE"), timeframe, periods);
  pineTS.setAlertMode?.("all");
  const context = await withTimeout(
    pineTS.run(String(params.script || ""), periods),
    Number(params.timeoutMs || DEFAULT_TIMEOUT_MS),
  );
  const plots = normalizePlots(context?.plots);
  return {
    ok: true,
    engine: "pinets-shadow",
    engineVersion: pinetsPackage.version,
    license: pinetsPackage.license,
    diagnostics: [],
    plots,
    signals: lastSignals(plots),
    metadata: {
      symbol: String(params.symbol || "JFTRADE.SAMPLE"),
      timeframe,
      candles: candles.length,
      mode: params.mode || "shadow",
    },
    runtimeMs: 0,
  };
}

async function handle(request) {
  const startedAt = performance.now();
  if (request.method === "engineInfo") {
    return {
      engine: "pinets-shadow",
      engineVersion: pinetsPackage.version,
      packageName: pinetsPackage.name,
      license: pinetsPackage.license,
      repository: pinetsPackage.repository?.url ?? "",
      runtime: "node",
    };
  }
  if (request.method === "runIndicator") {
    const result = await runIndicator(request.params);
    result.runtimeMs = Math.max(0, Math.round(performance.now() - startedAt));
    return result;
  }
  throw new Error(`unsupported pinets worker method ${request.method}`);
}

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });

rl.on("line", async (line) => {
	let request;
	try {
		request = JSON.parse(line.replace(/^\uFEFF/, ""));
    const result = await handle(request);
    process.stdout.write(`${JSON.stringify({ id: request.id, ok: true, result })}\n`);
  } catch (error) {
    const id = request?.id ?? null;
    process.stdout.write(
      `${JSON.stringify({
        id,
        ok: false,
        error: {
          code: error?.name || "PINETS_WORKER_ERROR",
          message: String(error?.message || error),
          diagnostics: diagnosticsFromError(error),
        },
      })}\n`,
    );
  }
});
