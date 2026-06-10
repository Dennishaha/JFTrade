import { describe, expect, it } from "vitest";

import type { StrategyInstanceItem } from "@/contracts";

import {
  buildStrategyBindingPayload,
  invalidSymbolsFromTextWithFallbackMarket,
  parseValidatedSymbolsTextWithFallbackMarket,
  readStrategyBinding,
} from "../src/components/strategy-runtime/strategyRuntimeInstanceBinding";

describe("strategyRuntimeInstanceBinding", () => {
  it("builds explicit instruments alongside legacy symbols", () => {
    const payload = buildStrategyBindingPayload({
      brokerAccountOptions: [],
      symbolsText: "us:aapl\nhk:00700",
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
        name: "DSL Breakout",
        version: "0.1.0",
      },
      runtime: "dsl-go-plan",
      sourceFormat: "dsl-v1",
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

  it("parses bare codes with the selected fallback market", () => {
    expect(parseValidatedSymbolsTextWithFallbackMarket("tme\n00700", "US")).toEqual([
      "US.TME",
      "US.00700",
    ]);
    expect(invalidSymbolsFromTextWithFallbackMarket("bad input", "US")).toEqual(["US.BAD INPUT"]);
  });
});