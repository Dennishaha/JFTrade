// @vitest-environment jsdom

import { effectScope, nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  InstrumentResolutionCandidate,
  InstrumentResolutionResponse,
} from "../src/contracts";
import {
  resolveMarketInstrumentCandidates,
  useInstrumentResolver,
} from "../src/composables/instrumentResolver";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

function candidate(
  market: string = "SH",
  patch: Partial<InstrumentResolutionCandidate> = {},
): InstrumentResolutionCandidate {
  const code = market === "SH" ? "600519" : market === "SZ" ? "000001" : market === "US" ? "AAPL" : "7203";
  return {
    market,
    resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
    instrumentId: `${market}.${code}`,
    code,
    symbol: code,
    name: market === "SH" ? "贵州茅台" : market === "SZ" ? "平安银行" : market === "US" ? "Apple" : "Toyota",
    securityType: "Eqty",
    lotSize: market === "US" ? 1 : 100,
    source: "test-search",
    isWatched: false,
    selectable: ["HK", "US", "SH", "SZ"].includes(market),
    unavailableReason: ["HK", "US", "SH", "SZ"].includes(market)
      ? null
      : `当前版本暂不支持 ${market} 市场`,
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
  it("submits every code or name to the backend and supports all markets", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse(
        resolution({
          requestedMarket: "",
          query: "Apple",
          entries: [candidate("US")],
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      resolveMarketInstrumentCandidates({ market: "", query: " Apple " }),
    ).resolves.toMatchObject({
      requestedMarket: "",
      resolutionStatus: "resolved",
      entries: [{ market: "US", instrumentId: "US.AAPL", selectable: true }],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/instruments?query=Apple&limit=20",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("sends an optional market filter and normalizes disabled candidates", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse({
        requestedMarket: "JP",
        query: "Toyota",
        resolutionStatus: "unavailable",
        entries: [
          {
            market: "jp",
            symbol: "7203",
            instrumentId: "jp.7203",
            name: "Toyota",
            securityType: "Eqty",
            selectable: false,
            unavailableReason: "当前版本暂不支持 JP 市场",
          },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      resolveMarketInstrumentCandidates({ market: "jp", query: "Toyota", limit: 5 }),
    ).resolves.toMatchObject({
      resolutionStatus: "unavailable",
      entries: [
        {
          instrumentId: "JP.7203",
          selectable: false,
          unavailableReason: "当前版本暂不支持 JP 市场",
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/market-data/instruments?query=Toyota&limit=5&market=JP",
      expect.any(Object),
    );
  });

  it("normalizes and de-duplicates server candidates", async () => {
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
              selectable: true,
            },
            {
              market: "SZ",
              code: "000001",
              instrumentId: "SZ.000001",
              selectable: true,
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

describe("useInstrumentResolver", () => {
  it("resolves a unique result through the backend without opening candidate UI", async () => {
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
    const market = ref("US");
    const query = ref("AAPL");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    await resolver.resolve();

    expect(onResolved).toHaveBeenCalledWith(candidate("US"));
    expect(resolver.panelOpen.value).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    scope.stop();
  });

  it("skips disabled candidates during keyboard navigation and selection", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse(
          resolution({
            requestedMarket: "",
            resolutionStatus: "ambiguous",
            totalReturned: 3,
            entries: [candidate("JP"), candidate("SH"), candidate("SZ")],
          }),
        ),
      ),
    );
    const market = ref("");
    const query = ref("000001");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    await resolver.resolve();
    expect(resolver.panelOpen.value).toBe(true);
    expect(resolver.activeCandidateIndex.value).toBe(1);
    resolver.moveActiveCandidate(1);
    expect(resolver.activeCandidateIndex.value).toBe(2);
    resolver.moveActiveCandidate(1);
    expect(resolver.activeCandidateIndex.value).toBe(1);
    resolver.selectCandidate(candidate("JP"));
    expect(onResolved).not.toHaveBeenCalled();
    expect(resolver.selectActiveCandidate()).toBe(true);
    expect(onResolved).toHaveBeenCalledWith(candidate("SH"));
    scope.stop();
  });

  it("shows unavailable results without allowing automatic resolution", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse(
          resolution({
            requestedMarket: "",
            resolutionStatus: "unavailable",
            entries: [candidate("JP")],
          }),
        ),
      ),
    );
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market: ref(""), query: ref("Toyota"), onResolved }),
    )!;

    await resolver.resolve();
    expect(resolver.statusMessage.value).toContain("暂未开放");
    expect(resolver.activeCandidateIndex.value).toBe(-1);
    expect(resolver.selectActiveCandidate()).toBe(false);
    expect(onResolved).not.toHaveBeenCalled();
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
            query: "Apple",
            entries: [candidate("US")],
          }),
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const market = ref("");
    const query = ref("苹果");
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market, query, onResolved }),
    )!;

    const first = resolver.resolve();
    const firstSignal = (fetchMock.mock.calls[0]?.[1] as RequestInit).signal;
    market.value = "US";
    query.value = "Apple";
    await nextTick();
    expect(firstSignal?.aborted).toBe(true);
    await resolver.resolve();
    resolveFirst?.(createResponse(resolution({ entries: [candidate("SH")] })));
    await first;

    expect(onResolved).toHaveBeenCalledTimes(1);
    expect(onResolved).toHaveBeenCalledWith(candidate("US"));
    scope.stop();
  });
});
