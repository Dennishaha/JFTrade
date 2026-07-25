import { describe, expect, test } from "vitest";
import { NativePineTSExecutor } from "./pinetsExecutor";
import { prepareCandleBatch, prepareRunScriptRequest } from "./preparedRequest";
import type { PreparedRunScriptRequest, RunScriptRequest } from "./types";

describe("NativePineTSExecutor request.security preflight", () => {
  test("lexes comments, quoted text, whitespace, nested tickers, and direct input timeframes", async () => {
    const { executor, constructed } = createExecutor();
    await expect(executor.run(preparedRequest([
      `indicator("preflight")`,
      `ignored = "request.security(\\\"US.MSFT\\\", \\\"1\\\", close)"`,
      `// request.security("US.MSFT", "1", close)`,
      `/* request.security("US.MSFT", "1", close) */`,
      `value = request.security (`,
      `  ticker.inherit(/* preserve */ ticker.heikinashi(syminfo.tickerid), syminfo.tickerid),`,
      `  input.timeframe(defval = "3", title = "MTF"),`,
      `  close // value`,
      `)`,
    ].join("\n")))).resolves.toMatchObject({ plots: {} });

    expect(constructed).toHaveLength(1);
  });

  test("resolves typed input aliases and empty ticker/timeframe values", async () => {
    const { executor, constructed } = createExecutor();
    await expect(executor.run(preparedRequest([
      `indicator("preflight aliases")`,
      `var string tf = input.timeframe(defval = "", title = "MTF")`,
      `standard = request.security((ticker.standard()), (tf), close)`,
      `current = request.security("", "", close)`,
    ].join("\n")))).resolves.toMatchObject({ plots: {} });

    expect(constructed).toHaveLength(1);
  });

  test("rejects malformed, dynamic, and unsupported static request.security forms before construction", async () => {
    const cases: ReadonlyArray<readonly [string, string]> = [
      [
        `value = request.security(syminfo.tickerid)`,
        "request.security() requires symbol, timeframe, and expression arguments",
      ],
      [
        `value = request.security(ticker.heikinashi(), "1", close)`,
        "ticker.heikinashi() requires one static symbol argument",
      ],
      [
        `value = request.security(ticker.standard(syminfo.tickerid, syminfo.tickerid), "1", close)`,
        "ticker.standard() accepts at most one static symbol argument",
      ],
      [
        `value = request.security(ticker.inherit(syminfo.tickerid), "1", close)`,
        "ticker.inherit() requires static source and symbol arguments",
      ],
      [
        `value = request.security(ticker.renko(syminfo.tickerid), "1", close)`,
        "requires a supported static ticker expression",
      ],
      [
        `value = request.security(syminfo.tickerid, dynamicTimeframe, close)`,
        "requires a static timeframe string",
      ],
      [
        `value = request.security(syminfo.tickerid, "1", close`,
        "could not parse request.security() call",
      ],
    ];

    for (const [source, message] of cases) {
      const { executor, constructed } = createExecutor();
      await expect(executor.run(preparedRequest(source))).rejects.toThrow(message);
      expect(constructed).toHaveLength(0);
    }
  });

  test("drops non-static input aliases before validating their request.security use", async () => {
    const { executor, constructed } = createExecutor();
    await expect(executor.run(preparedRequest([
      `indicator("invalid alias")`,
      `tf = input.timeframe(dynamicDefault)`,
      `value = request.security(syminfo.tickerid, tf, close)`,
    ].join("\n")))).rejects.toThrow("requires a static timeframe string");
    expect(constructed).toHaveLength(0);
  });

  test("initializes and incrementally stabilizes cached secondary contexts", async () => {
    const secondaryContext = {
      data: { openTime: { data: [1, 2] } },
      params: { retained: [0, 1, 2] },
    };
    let updateCalls = 0;
    const secondaryRuntime = {
      async updateTail(context: typeof secondaryContext): Promise<boolean> {
        updateCalls++;
        context.data.openTime.data = [3];
        context.params.retained = [0, 1, 2, 3];
        return true;
      },
    };
    const entry: {
      pineTS: typeof secondaryRuntime;
      context: typeof secondaryContext;
      dataVersion?: number;
    } = { pineTS: secondaryRuntime, context: secondaryContext };
    const { executor } = createExecutor({
      plots: {},
      cache: { ignored: null, primitive: 1, secondary: entry },
    });

    await executor.run(preparedRequest(`plot(close)`));

    expect(entry.dataVersion).toBe(0);
    expect(secondaryContext.params.retained).toEqual([1, 2]);
    await expect(secondaryRuntime.updateTail(secondaryContext)).resolves.toBe(true);
    expect(updateCalls).toBe(1);
    expect(secondaryContext.params.retained).toEqual([3]);
  });
});

function createExecutor(result: Record<string, unknown> = { plots: {} }): {
  executor: NativePineTSExecutor;
  constructed: unknown[];
} {
  const constructed: unknown[] = [];
  const executor = new NativePineTSExecutor({
    PineTS: class {
      constructor(source: unknown) {
        constructed.push(source);
      }

      async run(): Promise<Record<string, unknown>> {
        return result;
      }
    },
  });
  return { executor, constructed };
}

function preparedRequest(source: string): PreparedRunScriptRequest {
  const request: RunScriptRequest = {
    jobId: "preflight",
    source,
    symbol: "US.AAPL",
    timeframe: "1",
    candles: [{
      openTime: 1_700_000_000_000,
      closeTime: 1_700_000_059_999,
      open: 10,
      high: 12,
      low: 9,
      close: 11,
      volume: 100,
    }],
  };
  const { candles, ...fields } = request;
  return prepareRunScriptRequest(fields, prepareCandleBatch(candles));
}
