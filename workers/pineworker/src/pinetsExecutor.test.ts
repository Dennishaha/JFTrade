import { describe, expect, test } from "vitest";
import { createNativePineTSExecutor, NativePineTSExecutor, normalizePineSourceForPineTS } from "./pinetsExecutor";
import { prepareCandleBatch, prepareRunScriptRequest } from "./preparedRequest";
import type { PineTSPlot, PineTSRunResult, PreparedRunScriptRequest, RunScriptRequest } from "./types";

describe("NativePineTSExecutor", () => {
  test("passes custom candles and Pine source to PineTS", async () => {
    const calls: unknown[] = [];
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[], symbol?: string, timeframe?: string, periods?: number) {
          calls.push({ candles, symbol, timeframe, periods });
        }

        setAlertMode(mode: string) {
          calls.push({ alertMode: mode });
        }

        async run(source: string, periods?: number) {
          calls.push({ source, periods });
          return { plots: { close: [11] } };
        }
      },
    }, "pinets-test");

    const result = await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    }));

    expect(executor.version()).toBe("pinets-test");
    expect(calls[0]).toEqual({
      candles: [{ openTime: 1, closeTime: 1, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
      symbol: "US.AAPL",
      timeframe: "1",
      periods: 1,
    });
    expect(calls[1]).toEqual({ alertMode: "all" });
    expect(calls[2]).toEqual({ source: expect.stringContaining("indicator"), periods: 1 });
    expect(result.plots?.close).toEqual([11]);
  });

  test("reuses compatible candle arrays without remapping", async () => {
    const candles = [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }];
    let receivedCandles: unknown[] | undefined;
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[]) {
          receivedCandles = candles;
        }

        async run() {
          return { plots: { close: [11] } };
        }
      },
    });

    const request = preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    });
    await executor.run(request);

    expect(receivedCandles).toBe(request.candles);
  });

  test("remaps candles with extra runtime fields before passing to PineTS", async () => {
    const candles = [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100, extra: 1 }];
    let receivedCandles: unknown[] | undefined;
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor(candles: unknown[]) {
          receivedCandles = candles;
        }

        async run() {
          return { plots: { close: [11] } };
        }
      },
    });

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(receivedCandles).not.toBe(candles);
    expect(receivedCandles).toEqual([{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }]);
  });

  test("does not let native PineTS mutate reused request candles", async () => {
    const candles = [
      { openTime: 1_700_000_000_000, closeTime: 1_700_000_059_999, open: 10, high: 12, low: 9, close: 11, volume: 100 },
      { openTime: 1_700_000_060_000, closeTime: 1_700_000_119_999, open: 11, high: 13, low: 10, close: 12, volume: 110 },
    ];
    const before = JSON.stringify(candles);
    const executor = await createNativePineTSExecutor("pinets-test");

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nindicator("x")\nplot(close)`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(JSON.stringify(candles)).toBe(before);
  });

  test("captures a filled stop entry at placement without reconstructing it from the trade", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "stop-entry",
      source: [
        `//@version=6`,
        `strategy("stop entry", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("StopLong", strategy.long, qty=2, stop=105)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents).toEqual([{
      kind: "entry",
      id: "StopLong",
      direction: "long",
      quantity: 2,
      stopPrice: 105,
      barIndex: 0,
      time: candles[0]!.openTime,
    }]);
  });

  test("captures a filled limit exit and keeps the preceding market entry", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "limit-exit",
      source: [
        `//@version=6`,
        `strategy("limit exit", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.exit("TakeProfit", from_entry="Long", limit=110)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents).toEqual([
      {
        kind: "entry",
        id: "Long",
        direction: "long",
        quantity: 1,
        barIndex: 0,
        time: candles[0]!.openTime,
      },
      {
        kind: "exit",
        id: "TakeProfit",
        direction: "long",
        fromEntry: "Long",
        quantity: 1,
        limitPrice: 110,
        barIndex: 1,
        time: candles[1]!.openTime,
      },
    ]);
  });

  test("captures a filled short stop exit with the position direction needed to buy it back", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "short-stop-exit",
      source: [
        `//@version=6`,
        `strategy("short stop exit", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Short", strategy.short, qty=2)`,
        `if bar_index == 1 and strategy.position_size < 0`,
        `    strategy.exit("ShortStop", from_entry="Short", stop=110)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents).toEqual([
      {
        kind: "entry",
        id: "Short",
        direction: "short",
        quantity: 2,
        barIndex: 0,
        time: candles[0]!.openTime,
      },
      {
        kind: "exit",
        id: "ShortStop",
        direction: "short",
        fromEntry: "Short",
        quantity: 2,
        stopPrice: 110,
        barIndex: 1,
        time: candles[1]!.openTime,
      },
    ]);
  });

  test("scopes a default exit quantity to its from_entry across multiple entry IDs", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles().map((candle, index) =>
      index === 3 ? { ...candle, low: 94 } : candle,
    );

    const result = await executor.run(preparedRequest({
      jobId: "scoped-exit-quantity",
      source: [
        `//@version=6`,
        `strategy("scoped exit", initial_capital=100000, pyramiding=10)`,
        `if bar_index == 0`,
        `    strategy.entry("A", strategy.long, qty=1)`,
        `    strategy.entry("B", strategy.long, qty=4)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.entry("A", strategy.long, qty=2)`,
        `if bar_index == 2 and strategy.position_size > 0`,
        `    strategy.exit("ExitA", from_entry="A", stop=95)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents?.find((intent) => (intent as Record<string, unknown>).id === "ExitA")).toEqual({
      kind: "exit",
      id: "ExitA",
      direction: "long",
      fromEntry: "A",
      quantity: 3,
      stopPrice: 95,
      barIndex: 2,
      time: candles[2]!.openTime,
    });
  });

  test("converts close qty_percent against only the targeted entry ID", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "scoped-close-percent",
      source: [
        `//@version=6`,
        `strategy("scoped close", initial_capital=100000, pyramiding=10)`,
        `if bar_index == 0`,
        `    strategy.entry("A", strategy.long, qty=1)`,
        `    strategy.entry("B", strategy.long, qty=4)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.entry("A", strategy.long, qty=2)`,
        `if bar_index == 2 and strategy.position_size > 0`,
        `    strategy.close("A", qty_percent=50)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents?.find((intent) => (intent as Record<string, unknown>).id === "close_A")).toEqual({
      kind: "exit",
      id: "close_A",
      direction: "long",
      fromEntry: "A",
      quantity: 1.5,
      barIndex: 2,
      time: candles[2]!.openTime,
    });
  });

  test("uses the full multi-entry position quantity for close_all", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "close-all-quantity",
      source: [
        `//@version=6`,
        `strategy("close all", initial_capital=100000, pyramiding=10)`,
        `if bar_index == 0`,
        `    strategy.entry("A", strategy.long, qty=1)`,
        `    strategy.entry("B", strategy.long, qty=4)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.entry("A", strategy.long, qty=2)`,
        `if bar_index == 2 and strategy.position_size > 0`,
        `    strategy.close_all()`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents?.find((intent) => (intent as Record<string, unknown>).id === "close_all")).toEqual({
      kind: "exit",
      id: "close_all",
      direction: "long",
      quantity: 7,
      barIndex: 2,
      time: candles[2]!.openTime,
    });
  });

  test("captures ordinary market entry and scoped close without conditional prices", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles();

    const result = await executor.run(preparedRequest({
      jobId: "market-entry-close",
      source: [
        `//@version=6`,
        `strategy("market entry and close", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.close("Long")`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents).toEqual([
      {
        kind: "entry",
        id: "Long",
        direction: "long",
        quantity: 1,
        barIndex: 0,
        time: candles[0]!.openTime,
      },
      {
        kind: "exit",
        id: "close_Long",
        direction: "long",
        fromEntry: "Long",
        quantity: 1,
        barIndex: 1,
        time: candles[1]!.openTime,
      },
    ]);
  });

  test("preserves short direction for market close and close_all", async () => {
    const candles = conditionalOrderCandles();
    for (const close of [
      { call: `strategy.close("Short")`, id: "close_Short", fromEntry: "Short" },
      { call: `strategy.close_all()`, id: "close_all", fromEntry: undefined },
    ]) {
      const executor = await createNativePineTSExecutor("pinets-test");
      const result = await executor.run(preparedRequest({
        jobId: close.id,
        source: [
          `//@version=6`,
          `strategy("short market close", initial_capital=100000)`,
          `if bar_index == 0`,
          `    strategy.entry("Short", strategy.short, qty=1)`,
          `if bar_index == 1 and strategy.position_size < 0`,
          `    ${close.call}`,
        ].join("\n"),
        symbol: "US.AAPL",
        timeframe: "1",
        candles,
      }));

      expect(result.orderIntents?.[1]).toEqual({
        kind: "exit",
        id: close.id,
        direction: "short",
        ...(close.fromEntry === undefined ? {} : { fromEntry: close.fromEntry }),
        quantity: 1,
        barIndex: 1,
        time: candles[1]!.openTime,
      });
    }
  });

  test("does not resend an unchanged persistent stop exit on every bar", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles().map((candle) => ({ ...candle, low: Math.max(candle.low, 98) }));

    const result = await executor.run(preparedRequest({
      jobId: "stable-stop-exit",
      source: [
        `//@version=6`,
        `strategy("stable stop", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if strategy.position_size > 0`,
        `    strategy.exit("StopLoss", from_entry="Long", stop=95)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles,
    }));

    expect(result.orderIntents?.filter((intent) => (intent as Record<string, unknown>).id === "StopLoss")).toEqual([{
      kind: "exit",
      id: "StopLoss",
      direction: "long",
      fromEntry: "Long",
      quantity: 1,
      stopPrice: 95,
      barIndex: 1,
      time: candles[1]!.openTime,
    }]);
  });

  test("emits cancel then replacement when a working stop price changes", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");
    const candles = conditionalOrderCandles().map((candle) => ({ ...candle, low: Math.max(candle.low, 98) }));

    const result = await executor.run(preparedRequest({
      jobId: "moving-stop-exit",
      source: [
        `//@version=6`,
        `strategy("moving stop", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if strategy.position_size > 0`,
        `    strategy.exit("StopLoss", from_entry="Long", stop=94 + bar_index)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: candles.slice(0, 3),
    }));

    expect(result.orderIntents?.filter((intent) => (intent as Record<string, unknown>).id === "StopLoss")).toEqual([
      expect.objectContaining({ kind: "exit", stopPrice: 95, barIndex: 1 }),
      expect.objectContaining({ kind: "cancel", barIndex: 2 }),
      expect.objectContaining({ kind: "exit", stopPrice: 96, barIndex: 2 }),
    ]);
  });

  test("fails closed when a strategy result cannot be captured per bar", async () => {
    const executor = new NativePineTSExecutor({
      PineTS: class {
        async run() {
          return { strategy: { closedtrades: [{ entry_id: "ambiguous" }] } };
        }
      },
    });

    await expect(executor.run(preparedRequest({
      jobId: "missing-hook",
      source: `//@version=6\nstrategy("x")`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: conditionalOrderCandles(),
    }))).rejects.toThrow("per-bar execution hook");
  });

  test("fails closed instead of applying an unknown from_entry exit to another position", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");

    await expect(executor.run(preparedRequest({
      jobId: "unknown-from-entry",
      source: [
        `//@version=6`,
        `strategy("unknown from entry", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if bar_index == 1 and strategy.position_size > 0`,
        `    strategy.exit("WrongExit", from_entry="Missing", stop=95)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: conditionalOrderCandles(),
    }))).rejects.toThrow("without a matching open or pending trade");
  });

  test("fails the whole run when a protective exit depends on an unfilled same-bar entry", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");

    await expect(executor.run(preparedRequest({
      jobId: "non-atomic-bracket",
      source: [
        `//@version=6`,
        `strategy("non atomic bracket", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `    strategy.exit("StopLoss", from_entry="Long", stop=95)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: conditionalOrderCandles(),
    }))).rejects.toThrow("cannot atomically express a parent-linked or reduce-only protective exit");
  });

  test("fails closed for tick-based exits that the order protocol cannot express", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");

    await expect(executor.run(preparedRequest({
      jobId: "profit-exit",
      source: [
        `//@version=6`,
        `strategy("profit exit", initial_capital=100000)`,
        `if bar_index == 0`,
        `    strategy.entry("Long", strategy.long, qty=1)`,
        `if strategy.position_size > 0`,
        `    strategy.exit("ProfitTicks", from_entry="Long", profit=10)`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: conditionalOrderCandles(),
    }))).rejects.toThrow("unsupported conditional fields: profit");
  });

  test("keeps PineTS integer division semantics at the adapter boundary", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");

    const result = await executor.run(preparedRequest({
      jobId: "job-1",
      source: [
        `//@version=6`,
        `indicator("division")`,
        `plot(11 / 2, "intDiv")`,
        `plot(11 / 2.0, "floatDiv")`,
        `plot(-11 / 2, "negIntDiv")`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: pineTSBoundaryCandles(),
    }));

    expect(plotValues(result, "intDiv")).toEqual([5, 5, 5, 5, 5, 5]);
    expect(plotValues(result, "floatDiv")).toEqual([5.5, 5.5, 5.5, 5.5, 5.5, 5.5]);
    expect(plotValues(result, "negIntDiv")).toEqual([-5, -5, -5, -5, -5, -5]);
  });

  test("keeps PineTS user-function history access semantics at the adapter boundary", async () => {
    const executor = await createNativePineTSExecutor("pinets-test");

    const result = await executor.run(preparedRequest({
      jobId: "job-1",
      source: [
        `//@version=6`,
        `indicator("function history")`,
        `fromParam(src, len) => src[len]`,
        `fromClose(len) => close[len]`,
        `fromTuple(src, len) => [src[len], close[len]]`,
        `[tupleParam, tupleClose] = fromTuple(close, 1)`,
        `plot(fromParam(close, 1), "paramHistory")`,
        `plot(fromClose(1), "closeHistory")`,
        `plot(tupleParam, "tupleParamHistory")`,
        `plot(tupleClose, "tupleCloseHistory")`,
      ].join("\n"),
      symbol: "US.AAPL",
      timeframe: "1",
      candles: pineTSBoundaryCandles(),
    }));

    const expected = [NaN, 10, 11, 12, 13, 14];
    expect(plotValues(result, "paramHistory")).toEqual(expected);
    expect(plotValues(result, "closeHistory")).toEqual(expected);
    expect(plotValues(result, "tupleParamHistory")).toEqual(expected);
    expect(plotValues(result, "tupleCloseHistory")).toEqual(expected);
  });

  test("normalizes timenow to PineTS-supported time_close", async () => {
    const calls: unknown[] = [];
    const executor = new NativePineTSExecutor({
      PineTS: class {
        constructor() {}

        async run(source: string, periods?: number) {
          calls.push({ source, periods });
          return { plots: { now: [1] } };
        }
      },
    });

    await executor.run(preparedRequest({
      jobId: "job-1",
      source: `//@version=6\nstrategy("x")\nplot(timenow)`,
      symbol: "US.AAPL",
      timeframe: "1",
      candles: [{ openTime: 1, closeTime: 2, open: 10, high: 12, low: 9, close: 11, volume: 100 }],
    }));

    expect(calls[0]).toEqual({ source: expect.stringContaining("plot(time_close)"), periods: 1 });
  });

  test("does not normalize timenow inside comments, strings, or longer identifiers", () => {
    const source = [
      `plot(timenow)`,
      `plot(my_timenow_value)`,
      `// timenow stays in comment`,
      `label.new(bar_index, close, "timenow stays in string")`,
      `/* timenow stays in block */`,
    ].join("\n");

    expect(normalizePineSourceForPineTS(source)).toBe([
      `plot(time_close)`,
      `plot(my_timenow_value)`,
      `// timenow stays in comment`,
      `label.new(bar_index, close, "timenow stays in string")`,
      `/* timenow stays in block */`,
    ].join("\n"));
  });
});

function preparedRequest(request: RunScriptRequest): PreparedRunScriptRequest {
  const { candles, ...fields } = request;
  return prepareRunScriptRequest(fields, prepareCandleBatch(candles));
}

function pineTSBoundaryCandles(): RunScriptRequest["candles"] {
  return Array.from({ length: 6 }, (_, index) => ({
    openTime: 1_700_000_000_000 + index * 60_000,
    closeTime: 1_700_000_059_999 + index * 60_000,
    open: 10 + index,
    high: 11 + index,
    low: 9 + index,
    close: 10 + index,
    volume: 100 + index,
  }));
}

function conditionalOrderCandles(): RunScriptRequest["candles"] {
  return [
    { openTime: 1_700_000_000_000, closeTime: 1_700_000_059_999, open: 100, high: 101, low: 99, close: 100, volume: 100 },
    { openTime: 1_700_000_060_000, closeTime: 1_700_000_119_999, open: 100, high: 106, low: 99, close: 105, volume: 100 },
    { openTime: 1_700_000_120_000, closeTime: 1_700_000_179_999, open: 105, high: 111, low: 99, close: 109, volume: 100 },
    { openTime: 1_700_000_180_000, closeTime: 1_700_000_239_999, open: 109, high: 112, low: 100, close: 110, volume: 100 },
  ];
}

function plotValues(result: PineTSRunResult, name: string): number[] {
  const plot = result.plots?.[name];
  expect(plot).toBeDefined();
  const data = (plot as PineTSPlot).data;
  expect(data).toBeDefined();
  return data!.map((point) => typeof point === "number" ? point : Number(point.value));
}
