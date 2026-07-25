// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { defineComponent } from "vue";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
  getWatchlistMembership: vi.fn(),
}));

vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/apiClient")>();
  return {
    ...actual,
    fetchEnvelope: mocks.fetchEnvelope,
    fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
  };
});

vi.mock("../../src/composables/watchlistApi", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/watchlistApi")>();
  return {
    ...actual,
    getWatchlistMembership: mocks.getWatchlistMembership,
  };
});

import QuoteDetailRail from "../../src/components/research/QuoteDetailRail.vue";
import {
  researchQuoteSeedFromEntry,
  researchQuoteTargetFromEntry,
  type ResearchQuoteTarget,
} from "../../src/components/research/researchQuote";
import { flushPromises } from "../productTestUtils";

const LightweightChartStub = defineComponent({
  name: "LightweightChart",
  props: {
    target: { type: Object, default: null },
    period: { type: String, default: "1d" },
    variant: { type: String, default: "workspace" },
    minHeight: { type: Number, default: 0 },
  },
  emits: ["update:period"],
  template: `
    <div
      class="lightweight-chart-stub"
      :data-market="target?.market ?? ''"
      :data-symbol="target?.symbol ?? ''"
      :data-period="period"
      :data-variant="variant"
      :data-min-height="minHeight"
    >
      <button class="chart-period-1w" @click="$emit('update:period', '1w')">1W</button>
    </div>
  `,
});

const WatchlistDialogStub = defineComponent({
  name: "WatchlistMembershipDialog",
  props: {
    modelValue: Boolean,
    market: { type: String, default: "" },
    symbol: { type: String, default: "" },
    name: { type: String, default: "" },
  },
  emits: ["update:modelValue", "saved"],
  template: `
    <div class="watchlist-dialog-stub" :data-market="market" :data-symbol="symbol">
      {{ name }}
    </div>
  `,
});

const appleEntry = {
  instrumentId: "US.AAPL",
  name: "Apple",
  productClass: "equity",
  curPrice: 199,
  lastClosePrice: 198,
};

function instrumentParts(instrumentId: string) {
  const [market = "", ...symbolParts] = instrumentId.split(".");
  return { market, symbol: symbolParts.join("."), instrumentId };
}

function queryMeta(instrumentId: string) {
  return {
    instrumentId,
    source: "opend.snapshot",
    brokerId: "futu",
    resolvedAt: "2026-07-23T01:30:00Z",
    fromCache: false,
  };
}

function snapshotResponse(instrumentId: string, price = 201.5) {
  return {
    request: instrumentParts(instrumentId),
    snapshot: {
      price,
      bid: price - 0.05,
      ask: price + 0.05,
      openPrice: 200.5,
      highPrice: 203,
      lowPrice: 199,
      previousClosePrice: 200,
      lastClosePrice: 200,
      volume: 52_000_000,
      turnover: 10_400_000_000,
      at: "2026-07-23T01:30:00Z",
      session: "closed",
    },
    meta: queryMeta(instrumentId),
  };
}

function securityResponse(
  instrumentId: string,
  overrides: Record<string, unknown> = {},
) {
  const parts = instrumentParts(instrumentId);
  return {
    request: parts,
    security: {
      ...parts,
      name: instrumentId === "US.AAPL" ? "Apple Inc." : instrumentId,
      securityType: "Stock",
      productClass: "equity",
      currentPrice: 201.5,
      openPrice: 200.5,
      highPrice: 203,
      lowPrice: 199,
      lastClosePrice: 200,
      volume: 52_000_000,
      turnover: 10_400_000_000,
      updateTime: "2026-07-23T01:30:00Z",
      highest52WeeksPrice: 237,
      lowest52WeeksPrice: 164,
      equity: { peRate: 31.2, pbRate: 8.1, dividendRatioTTM: 0.48 },
      ...overrides,
    },
    meta: queryMeta(instrumentId),
  };
}

function idFromPath(path: string): string {
  const match = path.match(/market-data\/(?:snapshots|securities)\/([^/?]+)\/([^?]+)/);
  return match == null
    ? "US.AAPL"
    : `${decodeURIComponent(match[1]!)}.${decodeURIComponent(match[2]!)}`;
}

function installDefaultResponses(): void {
  mocks.fetchEnvelope.mockImplementation((path: string) => {
    const instrumentId = idFromPath(path);
    if (path.includes("/snapshots/")) {
      return Promise.resolve(snapshotResponse(instrumentId));
    }
    if (path.includes("/securities/")) {
      return Promise.resolve(securityResponse(instrumentId));
    }
    if (path.includes("operation=plate_members")) {
      return Promise.resolve({
        entries: [
          {
            instrumentId: "HK.00700",
            name: "腾讯控股",
            productClass: "equity",
          },
        ],
      });
    }
    if (path.includes("/market-data/news?")) {
      return Promise.resolve({
        entries: [
          {
            title: "Apple 发布新产品",
            source: "OpenD",
            publishTime: "2026-07-23T01:30:00Z",
            newsType: "news",
            summary: "只在切换到资讯页签后加载。",
          },
        ],
        warnings: [],
        partialErrors: [],
      });
    }
    return Promise.reject(new Error(`unexpected path: ${path}`));
  });
  mocks.fetchEnvelopeWithInit.mockResolvedValue({ entries: [] });
}

function mountRail(props: Record<string, unknown> = {}): VueWrapper {
  const entry = "entry" in props ? props.entry : appleEntry;
  const market = typeof props.market === "string" ? props.market : "";
  const target =
    "target" in props
      ? props.target
      : researchQuoteTargetFromEntry(
          entry as Record<string, unknown> | null | undefined,
          market,
        );
  const seed =
    "seed" in props
      ? props.seed
      : researchQuoteSeedFromEntry(
          entry as Record<string, unknown> | null | undefined,
        );
  const { entry: _entry, market: _market, ...componentProps } = props;
  return mount(QuoteDetailRail, {
    props: { target, seed, brokerId: "futu", ...componentProps },
    global: {
      stubs: {
        LightweightChart: LightweightChartStub,
        WatchlistMembershipDialog: WatchlistDialogStub,
      },
    },
  });
}

beforeEach(() => {
  installDefaultResponses();
  mocks.getWatchlistMembership.mockResolvedValue({
    instrumentId: "US.AAPL",
    revision: 1,
    groups: [{ id: "default", name: "默认" }],
    groupIds: ["default"],
  });
});

afterEach(() => {
  mocks.fetchEnvelope.mockReset();
  mocks.fetchEnvelopeWithInit.mockReset();
  mocks.getWatchlistMembership.mockReset();
  vi.useRealTimers();
});

describe("QuoteDetailRail", () => {
  it("renders a placeholder without issuing requests when selection is empty", async () => {
    const wrapper = mountRail({ entry: null, target: null });
    await flushPromises();

    expect(wrapper.text()).toContain("点击左侧榜单查看行情详情");
    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();
    expect(mocks.getWatchlistMembership).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("loads quote details and mounts the shared controlled K-line component", async () => {
    const wrapper = mountRail();
    await flushPromises();

    const paths = mocks.fetchEnvelope.mock.calls.map(([path]) => String(path));
    expect(paths).toEqual(
      expect.arrayContaining([
        expect.stringMatching(/\/snapshots\/US\/AAPL\?.*brokerId=futu/),
        expect.stringMatching(/\/securities\/US\/AAPL\?.*brokerId=futu/),
      ]),
    );
    expect(paths.some((path) => path.includes("/candles/"))).toBe(false);
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("Apple Inc.");
    expect(wrapper.get(".quote-summary__price").text()).toBe("201.50");
    expect(
      wrapper.get(".quote-summary__change.tv-up").classes(),
    ).toContain("tv-up");
    expect(wrapper.text()).toContain("+1.50");
    expect(wrapper.text()).toContain("+0.75%");
    expect(wrapper.text()).toContain("已收盘");

    const chart = wrapper.get(".lightweight-chart-stub");
    expect(chart.attributes("data-market")).toBe("US");
    expect(chart.attributes("data-symbol")).toBe("AAPL");
    expect(chart.attributes("data-period")).toBe("1d");
    expect(chart.attributes("data-variant")).toBe("embedded");
    expect(chart.attributes("data-min-height")).toBe("320");

    expect(
      wrapper.get('[data-testid="quote-detail-rail-favorite"]').classes(),
    ).toContain(
      "is-active",
    );
    await wrapper
      .get('[data-testid="quote-detail-rail-favorite"]')
      .trigger("click");
    const dialog = wrapper.get(".watchlist-dialog-stub");
    expect(dialog.attributes("data-market")).toBe("US");
    expect(dialog.attributes("data-symbol")).toBe("AAPL");
    wrapper.unmount();
  });

  it("preserves the exact SH market when the surrounding UI market is CN", async () => {
    const wrapper = mountRail({
      market: "CN",
      entry: {
        instrumentId: "SH.600519",
        name: "贵州茅台",
        productClass: "equity",
      },
    });
    await flushPromises();

    const paths = mocks.fetchEnvelope.mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.includes("/snapshots/SH/600519"))).toBe(true);
    expect(paths.some((path) => path.includes("/CN/"))).toBe(false);
    expect(mocks.getWatchlistMembership).toHaveBeenCalledWith("SH", "600519");
    wrapper.unmount();
  });

  it("forwards canonical period changes from the shared K-line component", async () => {
    const wrapper = mountRail();
    await flushPromises();

    await wrapper.get(".chart-period-1w").trigger("click");
    expect(wrapper.emitted("update:period")?.[0]).toEqual(["1w"]);
    expect(
      mocks.fetchEnvelope.mock.calls.some(([path]) =>
        String(path).includes("/candles/"),
      ),
    ).toBe(false);
    wrapper.unmount();
  });

  it("leaves candle loading to the shared chart when brokerId is absent", async () => {
    const wrapper = mountRail({ brokerId: "" });
    await flushPromises();

    expect(
      mocks.fetchEnvelope.mock.calls.some(([path]) =>
        String(path).includes("/candles/"),
      ),
    ).toBe(false);
    expect(wrapper.find(".lightweight-chart-stub").exists()).toBe(true);
    wrapper.unmount();
  });

  it("keeps snapshot and details independent from the shared chart data path", async () => {
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      const instrumentId = idFromPath(path);
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse(instrumentId));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(securityResponse(instrumentId));
      }
      return Promise.reject(new Error("unexpected"));
    });
    const wrapper = mountRail();
    await flushPromises();

    expect(wrapper.get(".quote-summary__price").text()).toBe("201.50");
    expect(wrapper.find(".lightweight-chart-stub").exists()).toBe(true);
    wrapper.unmount();
  });

  it("loads plate members without requesting candles and emits exact member target", async () => {
    const target: ResearchQuoteTarget = {
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "恒生科技",
      productClass: "plate",
    };
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001", 4_500));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            name: "恒生科技",
            productClass: "plate",
            equity: null,
            plate: { raiseCount: 21, fallCount: 7, equalCount: 2 },
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return Promise.resolve({
          entries: [
            {
              instrumentId: "HK.00700",
              name: "腾讯控股",
              productClass: "equity",
            },
            {
              instrumentId: "SH.920117",
              name: "北交所伪装标的",
              productClass: "equity",
            },
          ],
        });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    mocks.fetchEnvelopeWithInit.mockResolvedValue({
      entries: [
        { instrumentId: "HK.00700", lastPrice: 450, previousClose: 440 },
      ],
    });

    const wrapper = mountRail({ target, entry: null });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("腾讯控股");
    });

    const paths = mocks.fetchEnvelope.mock.calls.map(([path]) => String(path));
    expect(paths.some((path) => path.includes("/candles/"))).toBe(false);
    expect(paths).toContainEqual(
      expect.stringMatching(
        /research\/industries\?.*operation=plate_members.*instrumentId=HK(?:\.|%2E)BK1001.*pageSize=200.*brokerId=futu/,
      ),
    );
    expect(wrapper.find(".quote-detail-rail__chart-section").exists()).toBe(false);
    expect(wrapper.find(".vertical-quote-workbench__tabs").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("打开工作台");
    expect(wrapper.text()).toContain("腾讯控股");
    expect(wrapper.text()).not.toContain("北交所伪装标的");
    expect(wrapper.text()).toContain("前 1 只");
    expect(wrapper.text()).toMatch(/上涨\s*21/);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledWith(
      expect.stringContaining("brokerId=futu"),
      expect.objectContaining({ body: expect.stringContaining("HK.00700") }),
    );
    expect(String(mocks.fetchEnvelopeWithInit.mock.calls[0]?.[1]?.body)).not.toContain(
      "SH.920117",
    );
    await wrapper.get(".quote-detail-rail__member").trigger("click");
    expect(wrapper.emitted("select")?.[0]).toEqual([
      {
        kind: "instrument",
        instrumentId: "HK.00700",
        name: "腾讯控股",
        productClass: "equity",
      },
    ]);
    wrapper.unmount();
  });

  it("keeps plate members when auxiliary snapshot enrichment fails", async () => {
    const target: ResearchQuoteTarget = {
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "恒生科技",
      productClass: "plate",
    };
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001", 4_500));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            name: "恒生科技",
            productClass: "plate",
            equity: null,
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return Promise.resolve({
          entries: [
            {
              instrumentId: "HK.00700",
              name: "腾讯控股",
              productClass: "equity",
            },
            {
              instrumentId: "SH.830001",
              name: "北交所旧代码",
              productClass: "equity",
            },
          ],
        });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    mocks.fetchEnvelopeWithInit.mockRejectedValue(new Error("批量行情不可用"));

    const wrapper = mountRail({ target, entry: null });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("腾讯控股");
    });

    expect(wrapper.text()).toContain("腾讯控股");
    expect(wrapper.text()).not.toContain("北交所旧代码");
    expect(wrapper.findAll(".quote-detail-rail__member")).toHaveLength(1);
    expect(wrapper.text()).not.toContain("板块成分股加载失败");
    wrapper.unmount();
  });

  it("caps oversized plate responses before snapshot and display", async () => {
    const target: ResearchQuoteTarget = {
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "大型板块",
      productClass: "plate",
    };
    const entries = Array.from({ length: 230 }, (_, index) => ({
      instrumentId: `HK.${String(index + 1).padStart(5, "0")}`,
      name: `成分股 ${index + 1}`,
      productClass: "equity",
    }));
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001", 4_500));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            name: "大型板块",
            productClass: "plate",
            equity: null,
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return Promise.resolve({ entries, total: entries.length });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    mocks.fetchEnvelopeWithInit.mockImplementation(
      (_path: string, init: RequestInit) => {
        const instrumentIds = JSON.parse(String(init.body)).instrumentIds as string[];
        return Promise.resolve({
          entries: instrumentIds.map((instrumentId) => ({
            instrumentId,
            lastPrice: 2,
            previousClose: 1,
          })),
        });
      },
    );

    const wrapper = mountRail({ target, entry: null });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.findAll(".quote-detail-rail__member")).toHaveLength(50);
    });

    expect(wrapper.findAll(".quote-detail-rail__member")).toHaveLength(50);
    expect(wrapper.text()).toContain("前 50 只");
    expect(wrapper.text()).toContain("统计范围");
    expect(wrapper.text()).toContain("200 / 230 只");
    const requestedIds = JSON.parse(
      String(mocks.fetchEnvelopeWithInit.mock.calls[0]?.[1]?.body),
    ).instrumentIds as string[];
    expect(requestedIds).toHaveLength(200);
    wrapper.unmount();
  });

  it("ignores late responses from a previously selected instrument", async () => {
    let resolveApple: ((value: unknown) => void) | undefined;
    let resolveMicrosoft: ((value: unknown) => void) | undefined;
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      const instrumentId = idFromPath(path);
      if (path.includes("/snapshots/")) {
        return new Promise((resolve) => {
          if (instrumentId === "US.AAPL") resolveApple = resolve;
          else resolveMicrosoft = resolve;
        });
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(securityResponse(instrumentId));
      }
      return Promise.reject(new Error("unexpected"));
    });
    const wrapper = mountRail();
    await flushPromises();

    await wrapper.setProps({
      target: {
        kind: "instrument",
        instrumentId: "US.MSFT",
        name: "Microsoft",
        productClass: "equity",
      },
      seed: { name: "Microsoft" },
    });
    await flushPromises();
    resolveMicrosoft?.(snapshotResponse("US.MSFT", 510));
    await flushPromises();
    expect(wrapper.get(".quote-summary__price").text()).toBe("510.00");

    resolveApple?.(snapshotResponse("US.AAPL", 99));
    await flushPromises();
    expect(wrapper.text()).toContain("US.MSFT");
    expect(wrapper.get(".quote-summary__price").text()).toBe("510.00");
    wrapper.unmount();
  });

  it("polls snapshots every three seconds only while visible and stops on unmount", async () => {
    vi.useFakeTimers();
    const originalVisibility = Object.getOwnPropertyDescriptor(
      document,
      "visibilityState",
    );
    let visibility: DocumentVisibilityState = "visible";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });

    const wrapper = mountRail();
    await flushPromises();
    const snapshotCalls = () =>
      mocks.fetchEnvelope.mock.calls.filter(([path]) =>
        String(path).includes("/snapshots/"),
      ).length;
    expect(snapshotCalls()).toBe(1);

    await vi.advanceTimersByTimeAsync(3_000);
    await flushPromises();
    expect(snapshotCalls()).toBe(2);

    visibility = "hidden";
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(6_000);
    expect(snapshotCalls()).toBe(2);

    visibility = "visible";
    document.dispatchEvent(new Event("visibilitychange"));
    await flushPromises();
    expect(snapshotCalls()).toBe(3);

    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(6_000);
    expect(snapshotCalls()).toBe(3);

    if (originalVisibility == null) {
      Reflect.deleteProperty(document, "visibilityState");
    } else {
      Object.defineProperty(document, "visibilityState", originalVisibility);
    }
  });

  it("rejects a bare CN code rather than inventing a non-routable CN instrument", async () => {
    const wrapper = mountRail({
      market: "CN",
      entry: { code: "600519", name: "贵州茅台" },
    });
    await flushPromises();

    expect(wrapper.text()).toContain("缺少精确的 OpenD 标的代码");
    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("normalizes broker product classes at the research boundary and emits workspace navigation", async () => {
    const wrapper = mountRail({
      entry: {
        instrumentId: "US.AAPL",
        name: "Apple",
        productClass: "stock",
      },
    });
    await flushPromises();

    const openWorkspace = wrapper.get(".vertical-quote-workbench__open");
    expect(openWorkspace.classes()).toContain("is-outlined");
    await openWorkspace.trigger("click");
    expect(wrapper.emitted("openWorkspace")?.[0]).toEqual([
      {
        kind: "instrument",
        instrumentId: "US.AAPL",
        name: "Apple",
        productClass: "equity",
      },
    ]);
    wrapper.unmount();
  });

  it("keeps period and tab controlled and lazy-loads compact news", async () => {
    const wrapper = mountRail();
    await flushPromises();

    expect(
      mocks.fetchEnvelope.mock.calls.some(([path]) =>
        String(path).includes("/market-data/news?"),
      ),
    ).toBe(false);

    const newsTab = wrapper
      .findAll(".vertical-quote-workbench__tabs button")
      .find((button) => button.text() === "资讯");
    expect(newsTab).toBeDefined();
    await newsTab!.trigger("click");
    expect(wrapper.emitted("update:tab")?.[0]).toEqual(["news"]);
    expect(
      mocks.fetchEnvelope.mock.calls.some(([path]) =>
        String(path).includes("/market-data/news?"),
      ),
    ).toBe(false);

    await wrapper.setProps({ tab: "news" });
    await flushPromises();
    const newsPath = mocks.fetchEnvelope.mock.calls
      .map(([path]) => String(path))
      .find((path) => path.includes("/market-data/news?"));
    expect(newsPath).toMatch(
      /market=US&code=US(?:\.|%2E)AAPL&operation=search&pageSize=30.*brokerId=futu/,
    );
    expect(wrapper.text()).toContain("相关资讯");
    expect(wrapper.text()).toContain("关键词搜索");
    expect(wrapper.text()).toContain("Apple 发布新产品");

    await wrapper.setProps({ tab: "quote", period: "1w" });
    await flushPromises();
    expect(wrapper.get(".lightweight-chart-stub").attributes("data-period")).toBe(
      "1w",
    );
    wrapper.unmount();
  });

  it("uses the warrant owner for news and never requests heavy quote feeds", async () => {
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      const instrumentId = idFromPath(path);
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse(instrumentId));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse(instrumentId, {
            productClass: "warrant",
            equity: null,
            warrant: {
              owner: {
                instrumentId: "HK.00700",
                market: "HK",
                symbol: "00700",
              },
              strikePrice: 500,
              maturityTime: "2026-12-31",
              leverage: 5,
              premium: 2,
              impliedVolatility: 30,
            },
          }),
        );
      }
      if (path.includes("/market-data/news?")) {
        return Promise.resolve({ entries: [] });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });

    const wrapper = mountRail({
      target: {
        kind: "instrument",
        instrumentId: "HK.12345",
        name: "腾讯认购证",
        productClass: "warrant",
      },
      entry: null,
      tab: "news",
    });
    await flushPromises();

    const paths = mocks.fetchEnvelope.mock.calls.map(([path]) => String(path));
    expect(paths).toContainEqual(
      expect.stringMatching(
        /market-data\/news\?.*market=HK.*code=HK(?:\.|%2E)00700.*brokerId=futu/,
      ),
    );
    expect(
      paths.some((path) =>
        /subscribe|ticker|ticks|order-?book|depth|trade|margin|alert/i.test(path),
      ),
    ).toBe(false);
    wrapper.unmount();
  });

  it("preserves rendered quote and shared chart target when local refresh fails", async () => {
    let refreshing = false;
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      const instrumentId = idFromPath(path);
      if (refreshing) return Promise.reject(new Error("刷新暂时失败"));
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse(instrumentId));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(securityResponse(instrumentId));
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    const wrapper = mountRail();
    await flushPromises();
    expect(wrapper.get(".quote-summary__price").text()).toBe("201.50");
    expect(wrapper.get(".lightweight-chart-stub").attributes("data-symbol")).toBe(
      "AAPL",
    );

    refreshing = true;
    await wrapper.get('[aria-label="刷新当前标的"]').trigger("click");
    await flushPromises();

    expect(wrapper.get(".quote-summary__price").text()).toBe("201.50");
    expect(wrapper.get(".lightweight-chart-stub").attributes("data-symbol")).toBe(
      "AAPL",
    );
    expect(wrapper.text()).toContain("刷新暂时失败");
    wrapper.unmount();
  });

  it("supports drawer controls, controlled tab events, news selection, and saved favorites", async () => {
    const wrapper = mountRail({ drawer: true, tab: "news" });
    await flushPromises();

    await wrapper.get('[aria-label="关闭行情详情"]').trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
    const quoteTab = wrapper
      .findAll(".vertical-quote-workbench__tabs button")
      .find((button) => button.text() === "行情")!;
    await quoteTab.trigger("click");
    expect(wrapper.emitted("update:tab")).toContainEqual(["quote"]);

    const vertical = wrapper.findComponent({ name: "VerticalQuoteWorkbench" });
    const compactNews = wrapper.findComponent({ name: "CompactInstrumentNews" });
    compactNews.vm.$emit("selectTarget", {
      kind: "instrument",
      instrumentId: "US.MSFT",
      name: "Microsoft",
      productClass: "equity",
    });
    await flushPromises();
    expect(wrapper.emitted("select")?.at(-1)?.[0]).toMatchObject({
      instrumentId: "US.MSFT",
    });

    await wrapper
      .get('[data-testid="quote-detail-rail-favorite"]')
      .trigger("click");
    const dialog = wrapper.findComponent(WatchlistDialogStub);
    dialog.vm.$emit("saved", {
      instrumentId: "US.AAPL",
      revision: 2,
      groups: [],
      groupIds: [],
    });
    dialog.vm.$emit("update:modelValue", false);
    await flushPromises();
    expect(
      wrapper.get('[data-testid="quote-detail-rail-favorite"]').classes(),
    ).not.toContain("is-active");
    expect(vertical.exists()).toBe(true);
    wrapper.unmount();
  });

  it("renders index and trust metric branches", async () => {
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      const instrumentId = idFromPath(path);
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse(instrumentId));
      }
      if (path.includes("/securities/")) {
        if (instrumentId === "HK.800000") {
          return Promise.resolve(
            securityResponse(instrumentId, {
              name: "恒生指数",
              productClass: "index",
              equity: null,
              index: { raiseCount: 10, fallCount: 7, equalCount: 2 },
            }),
          );
        }
        return Promise.resolve(
          securityResponse(instrumentId, {
            name: "恒生 ETF",
            productClass: "fund",
            equity: null,
            trust: {
              assetClass: " Equity ",
              aum: 2_000_000_000,
              netAssetValue: 12.5,
              dividendYield: 1.8,
            },
          }),
        );
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    const indexWrapper = mountRail({
      target: {
        kind: "instrument",
        instrumentId: "HK.800000",
        name: "恒生指数",
        productClass: "index",
      },
      entry: null,
    });
    await flushPromises();
    expect(indexWrapper.text()).toContain("上涨");
    expect(indexWrapper.text()).toContain("下跌");
    indexWrapper.unmount();

    const fundWrapper = mountRail({
      target: {
        kind: "instrument",
        instrumentId: "HK.02800",
        name: "恒生 ETF",
        productClass: "fund",
      },
      entry: null,
    });
    await flushPromises();
    expect(fundWrapper.text()).toContain("资产类别");
    expect(fundWrapper.text()).toContain("Equity");
    expect(fundWrapper.text()).toContain("20.00亿");
    fundWrapper.unmount();
  });

  it("shows plate member loading, request errors, and empty responses", async () => {
    const target: ResearchQuoteTarget = {
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "恒生科技",
      productClass: "plate",
    };
    let rejectMembers!: (reason: unknown) => void;
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001"));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            productClass: "plate",
            equity: null,
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return new Promise((_, reject) => {
          rejectMembers = reject;
        });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    const wrapper = mountRail({ target, entry: null });
    await flushPromises();
    expect(wrapper.text()).toContain("成分股加载中");
    rejectMembers("plate unavailable");
    await flushPromises();
    expect(wrapper.text()).toContain("板块成分股加载失败");
    wrapper.unmount();

    installDefaultResponses();
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001"));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            productClass: "plate",
            equity: null,
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return Promise.resolve({});
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    const emptyWrapper = mountRail({ target, entry: null });
    await flushPromises();
    expect(emptyWrapper.text()).toContain("暂无成分股");
    emptyWrapper.unmount();
  });

  it("counts rising, falling, flat, and invalid plate snapshots", async () => {
    const target: ResearchQuoteTarget = {
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "综合板块",
      productClass: "plate",
    };
    const members = ["HK.UP", "HK.DOWN", "HK.FLAT", "HK.BAD"].map(
      (instrumentId) => ({
        instrumentId,
        name: instrumentId,
        productClass: "equity",
      }),
    );
    mocks.fetchEnvelope.mockImplementation((path: string) => {
      if (path.includes("/snapshots/")) {
        return Promise.resolve(snapshotResponse("HK.BK1001"));
      }
      if (path.includes("/securities/")) {
        return Promise.resolve(
          securityResponse("HK.BK1001", {
            productClass: "plate",
            equity: null,
          }),
        );
      }
      if (path.includes("operation=plate_members")) {
        return Promise.resolve({ entries: members });
      }
      return Promise.reject(new Error(`unexpected: ${path}`));
    });
    mocks.fetchEnvelopeWithInit.mockResolvedValue({
      entries: [
        { instrumentId: "HK.UP", lastPrice: "2", previousClose: "1" },
        { instrumentId: "HK.DOWN", price: 1, previousClosePrice: 2 },
        { instrumentId: "HK.FLAT", lastPrice: 1, previousClose: 1 },
        { instrumentId: "HK.BAD", lastPrice: "bad", previousClose: 1 },
      ],
    });
    const wrapper = mountRail({ target, entry: null });
    await flushPromises();
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain("统计范围");
    });
    expect(wrapper.text()).toMatch(/上涨\s*1/);
    expect(wrapper.text()).toMatch(/下跌\s*1/);
    expect(wrapper.text()).toMatch(/平盘\s*1/);
    wrapper.unmount();
  });
});
