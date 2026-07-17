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

  it("rejects malformed rows and derives useful statuses from entries and partial failures", async () => {
    const responses = [
      {
        requestedMarket: "",
        query: " 0700 ",
        entries: [
          { market: "", code: "", instrumentId: "", name: "broken" },
          { market: "hk", code: "00700", name: "腾讯控股", lotSize: Number.POSITIVE_INFINITY, source: null },
          { market: "HK", code: "00700", selectable: true },
        ],
        failures: [{ market: "us", code: "aapl", message: "" }],
      },
      {
        entries: [
          { market: "jp", code: "7203", selectable: false },
          { market: "kr", code: "005930", selectable: false },
        ],
      },
      { entries: [], failures: [{ market: "US", code: "TSLA", message: "timeout" }] },
      { entries: [] },
    ];
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createResponse(responses.shift() ?? { entries: [] })),
    );

    await expect(
      resolveMarketInstrumentCandidates({ market: "", query: " 0700 " }),
    ).resolves.toMatchObject({
      requestedMarket: "",
      query: "0700",
      resolutionStatus: "incomplete",
      entries: [{ instrumentId: "HK.00700", lotSize: null, source: "", selectable: true }],
      failures: [{ market: "US", code: "AAPL", message: "查询失败" }],
    });
    await expect(
      resolveMarketInstrumentCandidates({ market: "", query: "Asia" }),
    ).resolves.toMatchObject({ resolutionStatus: "unavailable", totalReturned: 2 });
    await expect(
      resolveMarketInstrumentCandidates({ market: "", query: "TSLA" }),
    ).resolves.toMatchObject({ resolutionStatus: "incomplete" });
    await expect(
      resolveMarketInstrumentCandidates({ market: "", query: "missing" }),
    ).resolves.toMatchObject({ resolutionStatus: "not_found" });
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

  it("ignores an AbortError reported by the browser transport", async () => {
    let rejectFirst: ((reason?: unknown) => void) | null = null;
    const firstResponse = new Promise<Response>((_resolve, reject) => {
      rejectFirst = reject;
    });
    vi.stubGlobal("fetch", vi.fn(() => firstResponse));
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market: ref("US"), query: ref("AAPL"), onResolved: vi.fn() }),
    )!;

    const request = resolver.resolve();
    rejectFirst?.(new DOMException("cancelled", "AbortError"));

    await expect(request).resolves.toBeNull();
    expect(resolver.resolutionError.value).toBe("");
    scope.stop();
  });

  it("reports input, malformed resolved results, and non-Error transport failures", async () => {
    const inputScope = effectScope();
    const onInputError = vi.fn();
    const emptyResolver = inputScope.run(() =>
      useInstrumentResolver({ market: ref("US"), query: ref("   "), onResolved: vi.fn(), onError: onInputError }),
    )!;
    await expect(emptyResolver.resolve()).resolves.toBeNull();
    expect(emptyResolver.resolutionError.value).toBe("请输入标的代码或名称。");
    expect(emptyResolver.panelOpen.value).toBe(true);
    expect(onInputError).toHaveBeenCalledWith(expect.any(Error));
    emptyResolver.closePanel();
    expect(emptyResolver.panelOpen.value).toBe(false);
    inputScope.stop();

    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(createResponse({ resolutionStatus: "resolved", entries: [] }))
        .mockRejectedValueOnce("offline"),
    );
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market: ref("US"), query: ref("AAPL"), onResolved: vi.fn() }),
    )!;
    await resolver.resolve();
    expect(resolver.resolutionError.value).toBe("标的解析响应缺少唯一候选。");
    await resolver.resolve();
    expect(resolver.resolutionError.value).toBe("标的查询失败，请稍后重试。");
    scope.stop();
  });

  it("handles keyboard navigation, selection, cancellation, and composition safely", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createResponse(resolution({
        resolutionStatus: "ambiguous",
        entries: [candidate("JP"), candidate("SH"), candidate("SZ")],
      }))),
    );
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market: ref(""), query: ref("000001"), onResolved }),
    )!;
    await resolver.resolve();

    const arrowDown = new KeyboardEvent("keydown", { key: "ArrowDown", cancelable: true });
    expect(resolver.handleKeydown(arrowDown)).toBe(true);
    expect(arrowDown.defaultPrevented).toBe(true);
    const arrowUp = new KeyboardEvent("keydown", { key: "ArrowUp", cancelable: true });
    expect(resolver.handleKeydown(arrowUp)).toBe(true);
    const composing = new KeyboardEvent("keydown", { key: "Enter", isComposing: true });
    expect(resolver.handleKeydown(composing)).toBe(false);

    const enter = new KeyboardEvent("keydown", { key: "Enter", cancelable: true });
    expect(resolver.handleKeydown(enter)).toBe(true);
    expect(onResolved).toHaveBeenCalledWith(candidate("SH"));
    expect(resolver.panelOpen.value).toBe(false);

    await resolver.resolve();
    const escape = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    expect(resolver.handleKeydown(escape)).toBe(true);
    expect(resolver.panelOpen.value).toBe(false);
    resolver.moveActiveCandidate(1);
    expect(resolver.handleKeydown(new KeyboardEvent("keydown", { key: "z" }))).toBe(false);
    scope.stop();
  });

  it("uses Enter to start a fresh resolution when no candidate menu is open", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse(
        resolution({ requestedMarket: "US", query: "AAPL", entries: [candidate("US")] }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const onResolved = vi.fn();
    const scope = effectScope();
    const resolver = scope.run(() =>
      useInstrumentResolver({ market: ref("US"), query: ref("AAPL"), onResolved }),
    )!;

    expect(resolver.statusMessage.value).toBe("");
    const enter = new KeyboardEvent("keydown", { key: "Enter", cancelable: true });
    expect(resolver.handleKeydown(enter)).toBe(true);
    expect(enter.defaultPrevented).toBe(true);
    await vi.waitFor(() => expect(onResolved).toHaveBeenCalledWith(candidate("US")));
    expect(fetchMock).toHaveBeenCalledOnce();
    scope.stop();
  });
});
