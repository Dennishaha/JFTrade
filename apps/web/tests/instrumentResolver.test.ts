// @vitest-environment jsdom

import { effectScope, nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  InstrumentResolutionCandidate,
  InstrumentResolutionResponse,
} from "../src/contracts";
import {
  resolveDirectInstrumentCandidate,
  resolveMarketInstrumentCandidates,
  useInstrumentResolver,
} from "../src/composables/instrumentResolver";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

function candidate(
  market: "SH" | "SZ" | "US" = "SH",
  patch: Partial<InstrumentResolutionCandidate> = {},
): InstrumentResolutionCandidate {
  const code = market === "SH" ? "600519" : market === "SZ" ? "000001" : "AAPL";
  return {
    market,
    resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
    instrumentId: `${market}.${code}`,
    code,
    symbol: code,
    name: market === "SH" ? "贵州茅台" : market === "SZ" ? "平安银行" : "Apple",
    securityType: "STOCK",
    lotSize: market === "US" ? 1 : 100,
    source: "test-static",
    ...patch,
  };
}

function resolution(
  patch: Partial<InstrumentResolutionResponse> = {},
): InstrumentResolutionResponse {
  return {
    requestedMarket: "CN",
    query: "000001",
    resolutionStatus: "resolved",
    totalReturned: 1,
    entries: [candidate("SZ")],
    failures: [],
    ...patch,
  };
}

describe("resolveMarketInstrumentCandidates", () => {
  it("keeps a US bare ticker as US.AAPL", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse(
        resolution({
          requestedMarket: "US",
          query: "AAPL",
          entries: [candidate("US")],
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      resolveMarketInstrumentCandidates({ market: "us", query: " AAPL " }),
    ).resolves.toMatchObject({
      requestedMarket: "US",
      resolutionStatus: "resolved",
      entries: [{ market: "US", code: "AAPL", instrumentId: "US.AAPL" }],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/instruments?market=US&query=AAPL",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("normalizes and de-duplicates parent-market candidates", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          query: "000001",
          entries: [
            {
              market: "sz",
              symbol: "000001",
              instrumentId: "sz.000001",
              name: "平安银行",
              lotSize: 100,
            },
            {
              market: "SZ",
              code: "000001",
              instrumentId: "SZ.000001",
            },
          ],
        }),
      ),
    );

    await expect(
      resolveMarketInstrumentCandidates({ market: "CN", query: "000001" }),
    ).resolves.toMatchObject({
      resolutionStatus: "resolved",
      totalReturned: 1,
      entries: [{ instrumentId: "SZ.000001" }],
    });
  });
});

describe("resolveDirectInstrumentCandidate", () => {
  it("directly resolves leaf markets and qualified instruments", () => {
    expect(resolveDirectInstrumentCandidate("US", "AAPL")).toMatchObject({
      market: "US",
      code: "AAPL",
      instrumentId: "US.AAPL",
      source: "direct-input",
    });
    expect(resolveDirectInstrumentCandidate("CN", "SZ.000001")).toMatchObject({
      market: "SZ",
      resolvedMarket: "CN",
      instrumentId: "SZ.000001",
    });
    expect(resolveDirectInstrumentCandidate("CN", "CNSH.600519")).toMatchObject({
      market: "SH",
      resolvedMarket: "CN",
      instrumentId: "SH.600519",
    });
    expect(resolveDirectInstrumentCandidate("US", "BRK.B")).toMatchObject({
      market: "US",
      code: "BRK.B",
      instrumentId: "US.BRK.B",
    });
  });

  it("keeps a bare parent-market code for subset lookup", () => {
    expect(resolveDirectInstrumentCandidate("CN", "000001")).toBeNull();
  });

  it("does not bypass server validation for invalid or mismatched market prefixes", () => {
    expect(resolveDirectInstrumentCandidate("CN", "CN.600519")).toBeNull();
    expect(resolveDirectInstrumentCandidate("CN", "US.AAPL")).toBeNull();
    expect(resolveDirectInstrumentCandidate("US", "HK.00700")).toBeNull();
    expect(resolveDirectInstrumentCandidate("US", "US.")).toBeNull();
    expect(resolveDirectInstrumentCandidate("", "MARS.123")).toBeNull();
  });
});

describe("useInstrumentResolver", () => {
  it("resolves a unique US ticker without opening candidate UI", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const market = ref("US");
    const query = ref("AAPL");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    await resolver.resolve();

    expect(onResolved).toHaveBeenCalledWith(
      candidate("US", {
        name: null,
        securityType: null,
        lotSize: null,
        source: "direct-input",
      }),
    );
    expect(resolver.panelOpen.value).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    scope.stop();
  });

  it("keeps ambiguous and incomplete candidates explicit", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        createResponse(
          resolution({
            resolutionStatus: "ambiguous",
            totalReturned: 2,
            entries: [candidate("SH"), candidate("SZ")],
          }),
        ),
      )
      .mockResolvedValueOnce(
        createResponse(
          resolution({
            resolutionStatus: "incomplete",
            entries: [candidate("SH")],
            failures: [{ market: "SZ", code: "600519", message: "查询超时" }],
          }),
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const market = ref("CN");
    const query = ref("000001");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    await resolver.resolve();
    expect(resolver.panelOpen.value).toBe(true);
    expect(resolver.candidates.value).toHaveLength(2);
    expect(onResolved).not.toHaveBeenCalled();
    resolver.moveActiveCandidate(1);
    expect(resolver.activeCandidateIndex.value).toBe(1);
    expect(resolver.selectActiveCandidate()).toBe(true);
    expect(onResolved).toHaveBeenCalledWith(candidate("SZ"));

    query.value = "600519";
    await nextTick();
    await resolver.resolve();
    expect(resolver.resolutionStatus.value).toBe("incomplete");
    expect(resolver.failures.value).toEqual([
      { market: "SZ", code: "600519", message: "查询超时" },
    ]);
    expect(onResolved).toHaveBeenCalledTimes(1);
    scope.stop();
  });

  it("aborts stale requests when market or query changes", async () => {
    let resolveFirst: ((value: Response) => void) | null = null;
    const firstResponse = new Promise<Response>((resolve) => {
      resolveFirst = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => firstResponse)
      .mockResolvedValueOnce(
        createResponse(
          resolution({
            requestedMarket: "US",
            query: "AAPL",
            entries: [candidate("US")],
          }),
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const market = ref("CN");
    const query = ref("600519");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    const first = resolver.resolve();
    const firstSignal = (fetchMock.mock.calls[0]?.[1] as RequestInit).signal;
    market.value = "US";
    query.value = "AAPL";
    await nextTick();
    expect(firstSignal?.aborted).toBe(true);
    await resolver.resolve();
    resolveFirst?.(createResponse(resolution({ entries: [candidate("SH")] })));
    await first;

    expect(onResolved).toHaveBeenCalledTimes(1);
    expect(onResolved).toHaveBeenCalledWith(
      candidate("US", {
        name: null,
        securityType: null,
        lotSize: null,
        source: "direct-input",
      }),
    );
    scope.stop();
  });
});
