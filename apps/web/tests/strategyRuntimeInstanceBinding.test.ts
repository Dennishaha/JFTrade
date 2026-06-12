import { describe, expect, it } from "vitest";

import type { StrategyInstanceItem } from "@/contracts";

import {
  buildStrategyBindingPayload,
  readStrategyBinding,
} from "../src/components/strategy-runtime/strategyRuntimeInstanceBinding";

describe("strategyRuntimeInstanceBinding", () => {
  it("builds payloads from normalized instruments", () => {
    const payload = buildStrategyBindingPayload({
      brokerAccountOptions: [],
      instruments: [
        { market: "US", code: "AAPL" },
        { market: "HK", code: "00700" },
      ],
      interval: "15m",
      executionMode: "notify_only",
      brokerAccountKey: "",
    });

    expect(payload.symbols).toEqual(["US.AAPL", "HK.00700"]);
    expect(payload.instruments).toEqual([
      { market: "US", code: "AAPL" },
      { market: "HK", code: "00700" },
    ]);
  });

  it("prefers explicit instruments when reading strategy bindings", () => {
    const strategy: StrategyInstanceItem = {
      id: "instance-1",
      definition: {
        strategyId: "dsl-breakout",
        name: "Pine Breakout",
        version: "0.1.0",
      },
      runtime: "pine-go-plan",
      sourceFormat: "pine-v6",
      startable: true,
      binding: {
        instruments: [
          { market: "US", code: "AAPL" },
          { market: "HK", code: "00700" },
        ],
        symbols: ["US.MSFT"],
        interval: "5m",
        executionMode: "live",
      },
      params: {},
      status: "STOPPED",
      createdAt: "2026-05-29T00:00:00.000Z",
      logs: [],
    };

    expect(readStrategyBinding(strategy)).toEqual({
      instruments: [
        { market: "US", code: "AAPL" },
        { market: "HK", code: "00700" },
      ],
      symbols: ["US.AAPL", "HK.00700"],
      interval: "5m",
      executionMode: "live",
      brokerAccount: null,
    });
  });

});
