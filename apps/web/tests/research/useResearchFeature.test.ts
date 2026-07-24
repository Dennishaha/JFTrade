// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
}));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import {
  researchFeaturePaths,
  useResearchFeature,
} from "../../src/composables/useResearchFeature";
import { flushPromises } from "../productTestUtils";

function featureResult(
  entries: Record<string, unknown>[],
  envelope: Record<string, unknown> = {},
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.test",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    ...envelope,
  };
}

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("useResearchFeature", () => {
  it("loads entries on creation and exposes asOf/provider", async () => {
    mocks.fetch.mockResolvedValue(featureResult([{ name: "Apple" }]));
    const path = ref("/api/research/rankings?market=US");
    const state = useResearchFeature(path);
    expect(state.loading.value).toBe(true);

    await flushPromises();
    expect(mocks.fetch).toHaveBeenCalledWith("/api/research/rankings?market=US");
    expect(state.loading.value).toBe(false);
    expect(state.error.value).toBe("");
    expect(state.entries.value).toEqual([{ name: "Apple" }]);
    expect(state.asOf.value).toBe("2026-07-17T00:00:00Z");
    expect(state.provider.value?.brokerId).toBe("futu");
  });

  it("refetches when the path changes", async () => {
    mocks.fetch.mockResolvedValue(featureResult([]));
    const path = ref("/api/a");
    useResearchFeature(path);
    await flushPromises();

    path.value = "/api/b";
    await nextTick();
    await flushPromises();
    expect(mocks.fetch).toHaveBeenLastCalledWith("/api/b");
    expect(mocks.fetch).toHaveBeenCalledTimes(2);
  });

  it("supports function path sources", async () => {
    mocks.fetch.mockResolvedValue(featureResult([]));
    const market = ref("US");
    useResearchFeature(() => `/api/rankings?market=${market.value}`);
    await flushPromises();
    expect(mocks.fetch).toHaveBeenCalledWith("/api/rankings?market=US");
  });

  it("refresh appends refresh=true", async () => {
    mocks.fetch.mockResolvedValue(featureResult([]));
    const state = useResearchFeature(ref("/api/a?x=1"));
    await flushPromises();

    await state.refresh();
    expect(mocks.fetch).toHaveBeenLastCalledWith("/api/a?x=1&refresh=true");
  });

  it("only applies the latest request when requests race", async () => {
    let resolveFirst: ((value: unknown) => void) | null = null;
    mocks.fetch
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce(featureResult([{ name: "latest" }]));

    const path = ref("/api/slow");
    const state = useResearchFeature(path);
    await nextTick();

    path.value = "/api/fast";
    await nextTick();
    await flushPromises();
    expect(state.entries.value).toEqual([{ name: "latest" }]);

    // 第一个请求最后才返回，其结果必须被丢弃
    resolveFirst?.(featureResult([{ name: "stale" }]));
    await flushPromises();
    expect(state.entries.value).toEqual([{ name: "latest" }]);
    expect(state.loading.value).toBe(false);
  });

  it("surfaces fetch errors and clears entries", async () => {
    mocks.fetch.mockRejectedValue(new Error("网络失败"));
    const state = useResearchFeature(ref("/api/broken"));
    await flushPromises();
    expect(state.error.value).toBe("网络失败");
    expect(state.entries.value).toEqual([]);
    expect(state.loading.value).toBe(false);
  });

  it("exposes the full feature envelope and propagates brokerId", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([{ instrumentId: "US.AAPL" }], {
        metadata: { currency: "USD" },
        total: 42,
        nextCursor: "next-page",
        hasMore: true,
        warnings: ["limited"],
        partialErrors: [{ scope: "US.BAD", code: "DENIED", message: "无权限" }],
      }),
    );
    const brokerId = ref("futu");
    const state = useResearchFeature(ref("/api/research?market=US"), { brokerId });
    await flushPromises();

    expect(mocks.fetch).toHaveBeenCalledWith(
      "/api/research?market=US&brokerId=futu",
    );
    expect(state.metadata.value).toMatchObject({ currency: "USD" });
    expect(state.total.value).toBe(42);
    expect(state.nextCursor.value).toBe("next-page");
    expect(state.hasMore.value).toBe(true);
    expect(state.warnings.value).toEqual(["limited"]);
    expect(state.partialErrors.value).toEqual([
      { scope: "US.BAD", code: "DENIED", message: "无权限" },
    ]);
  });

  it("appends single-market cursor pages", async () => {
    mocks.fetch.mockImplementation((path: string) =>
      Promise.resolve(
        path.includes("cursor=next-page")
          ? featureResult([{ instrumentId: "US.MSFT" }], {
              total: 2,
              hasMore: false,
            })
          : featureResult([{ instrumentId: "US.AAPL" }], {
              total: 2,
              nextCursor: "next-page",
              hasMore: true,
            }),
      ),
    );
    const state = useResearchFeature(ref("/api/research?market=US"), {
      brokerId: "futu",
    });
    await flushPromises();
    await state.loadMore();

    expect(mocks.fetch).toHaveBeenLastCalledWith(
      "/api/research?market=US&brokerId=futu&cursor=next-page",
    );
    expect(state.entries.value.map((entry) => entry.instrumentId)).toEqual([
      "US.AAPL",
      "US.MSFT",
    ]);
    expect(state.total.value).toBe(2);
    expect(state.hasMore.value).toBe(false);
  });

  it("expands CN to SH/SZ, deduplicates exact IDs and globally sorts movers", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const market = new URLSearchParams(path.split("?")[1]).get("market");
      return Promise.resolve(
        market === "SH"
          ? featureResult([
              { instrumentId: "SH.600000", market: "SH", symbol: "600000", changeRate: 2 },
            ])
          : featureResult([
              { instrumentId: "SZ.000001", market: "SZ", symbol: "000001", changeRate: 5 },
              { instrumentId: "SH.600000", market: "SH", symbol: "600000", changeRate: 99 },
            ]),
      );
    });
    const state = useResearchFeature(
      ref("/api/research?market=CN&operation=top_movers&direction=up"),
      { brokerId: "futu" },
    );
    await flushPromises();

    const calls = mocks.fetch.mock.calls.map(([path]) => String(path));
    expect(calls).toEqual(
      expect.arrayContaining([
        expect.stringContaining("market=SH"),
        expect.stringContaining("market=SZ"),
        expect.stringContaining("brokerId=futu"),
      ]),
    );
    expect(calls.some((path) => path.includes("market=CN"))).toBe(false);
    expect(state.entries.value.map((entry) => entry.instrumentId)).toEqual([
      "SZ.000001",
      "SH.600000",
    ]);
    expect(state.total.value).toBe(2);
    expect(state.metadata.value).toMatchObject({
      logicalMarket: "CN",
      sourceMarkets: ["SH", "SZ"],
    });
  });

  it("keeps separate calendar events for the same instrument on different dates", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([
        { instrumentId: "US.AAPL", eventDate: "2026-07-23" },
        { instrumentId: "US.AAPL", eventDate: "2026-07-24" },
        { instrumentId: "US.AAPL", eventDate: "2026-07-24" },
      ]),
    );
    const state = useResearchFeature(
      ref("/api/research/calendars?market=US&operation=earnings"),
    );
    await flushPromises();

    expect(state.entries.value).toEqual([
      { instrumentId: "US.AAPL", eventDate: "2026-07-23" },
      { instrumentId: "US.AAPL", eventDate: "2026-07-24" },
    ]);
  });

  it("keeps successful CN cursor pages when a sibling branch fails", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      const market = params.get("market");
      const cursor = params.get("cursor");
      if (market === "SZ" && cursor === "sz-next") {
        return Promise.reject(new Error("SZ 下一页失败"));
      }
      if (market === "SH" && cursor === "sh-next") {
        return Promise.resolve(
          featureResult([{ instrumentId: "SH.600001" }], {
            total: 2,
            hasMore: false,
          }),
        );
      }
      return Promise.resolve(
        market === "SH"
          ? featureResult([{ instrumentId: "SH.600000" }], {
              total: 2,
              nextCursor: "sh-next",
              hasMore: true,
            })
          : featureResult([{ instrumentId: "SZ.000001" }], {
              total: 2,
              nextCursor: "sz-next",
              hasMore: true,
            }),
      );
    });
    const state = useResearchFeature(ref("/api/research?market=CN"));
    await flushPromises();
    await state.loadMore();

    expect(state.error.value).toBe("");
    expect(state.entries.value.map((entry) => entry.instrumentId)).toEqual([
      "SH.600000",
      "SH.600001",
      "SZ.000001",
    ]);
    expect(state.partialErrors.value).toContainEqual({
      scope: "SZ",
      code: "QUERY_FAILED",
      message: "SZ 下一页失败",
    });
    expect(state.total.value).toBe(4);
    expect(state.hasMore.value).toBe(true);
  });

  it("supports disabled CN expansion, empty paths, and hash-safe refresh queries", async () => {
    expect(researchFeaturePaths("/api/research")).toEqual([
      { market: "", path: "/api/research" },
    ]);
    expect(
      researchFeaturePaths("/api/research?market=CN#table", {
        expandCN: false,
      }),
    ).toEqual([
      { market: "CN", path: "/api/research?market=CN#table" },
    ]);

    const path = ref("");
    const state = useResearchFeature(path, { brokerId: () => " futu " });
    await flushPromises();
    expect(mocks.fetch).not.toHaveBeenCalled();
    expect(state.entries.value).toEqual([]);

    mocks.fetch.mockResolvedValue(featureResult([]));
    path.value = "/api/research#table";
    await nextTick();
    await flushPromises();
    await state.refresh();
    expect(mocks.fetch).toHaveBeenLastCalledWith(
      "/api/research?brokerId=futu&refresh=true#table",
    );
    await state.loadMore();
    expect(mocks.fetch).toHaveBeenCalledTimes(2);
  });

  it("merges identifier variants, warnings, metadata, and custom ordering", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const market = new URLSearchParams(path.split("?")[1]).get("market");
      return Promise.resolve(
        market === "SH"
          ? featureResult(
              [
                { plateId: "plate-1", name: "B" },
                { institutionId: 7, name: "A" },
                { market: "SH", symbol: "600000", name: "C" },
                { title: "无标识一" },
              ],
              {
                total: 4,
                warnings: ["延迟", "延迟"],
                partialErrors: [
                  { scope: "SH", code: "P", message: "部分" },
                ],
                metadata: { sh: true },
              },
            )
          : featureResult(
              [
                { plateId: "PLATE-1", name: "重复" },
                { eventId: "event-1", name: "D" },
                { title: "无标识二" },
              ],
              {
                warnings: ["另一提示"],
                partialErrors: [
                  { scope: "SH", code: "P", message: "部分" },
                ],
                metadata: { sz: true },
              },
            ),
      );
    });
    const state = useResearchFeature(
      ref("/api/research?market=CN&operation=custom"),
      {
        mergeComparator: (left, right) =>
          String(left.name ?? left.title).localeCompare(
            String(right.name ?? right.title),
          ),
      },
    );
    await flushPromises();
    expect(state.entries.value).toHaveLength(6);
    expect(state.entries.value[0]?.name).toBe("A");
    expect(state.total.value).toBe(6);
    expect(state.warnings.value).toEqual(["延迟", "另一提示"]);
    expect(state.partialErrors.value).toHaveLength(1);
    expect(state.metadata.value).toMatchObject({
      logicalMarket: "CN",
      byMarket: { SH: { sh: true }, SZ: { sz: true } },
    });
  });

  it("sorts missing rank values and keeps partial initial branch failures", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      if (params.get("market") === "SZ") return Promise.reject("SZ unavailable");
      return Promise.resolve(
        featureResult([
          { instrumentId: "SH.NULL1" },
          { instrumentId: "SH.NULL2", averageHeat: "bad" },
          { instrumentId: "SH.HOT", averageHeat: 10 },
        ]),
      );
    });
    const state = useResearchFeature(
      ref("/api/research?market=CN&operation=hot"),
    );
    await flushPromises();
    expect(state.entries.value.map((entry) => entry.instrumentId)).toEqual([
      "SH.NULL1",
      "SH.NULL2",
      "SH.HOT",
    ]);
    expect(state.partialErrors.value).toContainEqual({
      scope: "SZ",
      code: "QUERY_FAILED",
      message: "SZ unavailable",
    });

    mocks.fetch.mockImplementation((path: string) => {
      const market = new URLSearchParams(path.split("?")[1]).get("market");
      return Promise.resolve(
        featureResult(
          market === "SH"
            ? [
                { instrumentId: "SH.NULL1" },
                { instrumentId: "SH.HOT", averageHeat: 10 },
              ]
            : [{ instrumentId: "SZ.NULL2", averageHeat: "bad" }],
        ),
      );
    });
    const sorted = useResearchFeature(
      ref("/api/research?market=CN&operation=hot"),
    );
    await flushPromises();
    expect(sorted.entries.value.map((entry) => entry.instrumentId)).toEqual([
      "SH.HOT",
      "SH.NULL1",
      "SZ.NULL2",
    ]);
  });

  it("merges cursor warnings and metadata while preserving a branch without a cursor", async () => {
    mocks.fetch.mockImplementation((path: string) => {
      const params = new URLSearchParams(path.split("?")[1]);
      const market = params.get("market");
      if (params.get("cursor") === "sh-next") {
        return Promise.resolve(
          featureResult([{ instrumentId: "SH.600001" }], {
            warnings: ["page warning"],
            partialErrors: [
              { scope: "SH", code: "PARTIAL", message: "page partial" },
            ],
            metadata: { page: 2 },
          }),
        );
      }
      return Promise.resolve(
        market === "SH"
          ? featureResult([{ instrumentId: "SH.600000" }], {
              nextCursor: "sh-next",
              warnings: ["first warning"],
              metadata: { page: 1 },
            })
          : featureResult([{ instrumentId: "SZ.000001" }]),
      );
    });
    const state = useResearchFeature(ref("/api/research?market=CN"));
    await flushPromises();
    await state.loadMore();
    expect(state.entries.value).toHaveLength(3);
    expect(state.warnings.value).toEqual(["first warning", "page warning"]);
    expect(state.partialErrors.value).toContainEqual({
      scope: "SH",
      code: "PARTIAL",
      message: "page partial",
    });
    expect(state.metadata.value).toMatchObject({
      byMarket: { SH: { page: 2 } },
    });
  });
});
