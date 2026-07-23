// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { effectScope, nextTick, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchEnvelopeWithInit: vi.fn(),
}));

vi.mock("../../src/composables/apiClient", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/apiClient")>();
  return {
    ...actual,
    fetchEnvelopeWithInit: mocks.fetchEnvelopeWithInit,
  };
});

import WatchlistFavoriteButton from "../../src/components/domain/watchlist/WatchlistFavoriteButton.vue";
import {
  isQuoteWorkbenchPeriod,
  isQuoteWorkbenchTab,
  normalizeQuoteWorkbenchProductClass,
} from "../../src/components/domain/market-data/quoteWorkbench";
import {
  directionClass,
  entryDayKey,
  formatCompactNumber,
  formatSigned,
  pickNumber,
} from "../../src/components/research/researchEntry";
import {
  isResearchQuoteEntry,
  isResearchQuoteTarget,
  normalizeResearchQuoteTarget,
  parseResearchInstrumentId,
  researchQuoteTargetFromEntry,
} from "../../src/components/research/researchQuote";
import {
  fetchResearchSnapshots,
  mergeResearchSnapshot,
  useResearchSnapshots,
} from "../../src/components/research/researchSnapshots";
import AppNavigationControls from "../../src/layout/AppNavigationControls.vue";

afterEach(() => {
  mocks.fetchEnvelopeWithInit.mockReset();
});

describe("research utility edge coverage", () => {
  it("normalizes workbench enums and rejects unsupported values", () => {
    expect(normalizeQuoteWorkbenchProductClass(null)).toBe("unknown");
    expect(normalizeQuoteWorkbenchProductClass(" ETF ")).toBe("fund");
    expect(isQuoteWorkbenchPeriod("five-day")).toBe(true);
    expect(isQuoteWorkbenchPeriod("year")).toBe(false);
    expect(isQuoteWorkbenchTab("quote")).toBe(true);
    expect(isQuoteWorkbenchTab("news")).toBe(true);
    expect(isQuoteWorkbenchTab("chart")).toBe(false);
  });

  it("formats alternate numeric and date wire values", () => {
    expect(pickNumber({ value: " 12.5 " }, ["value"])).toBe(12.5);
    expect(pickNumber({ value: "bad" }, ["value"])).toBeNull();
    expect(formatSigned(-2, "%")).toBe("-2.00%");
    expect(formatCompactNumber(1_200_000_000_000)).toBe("1.20万亿");
    expect(directionClass(-1)).toBe("tv-down");
    expect(entryDayKey({ reportDate: "2026-07-23T12:00:00Z" })).toBe(
      "2026-07-23",
    );
  });

  it("normalizes quote targets from raw, nested, and invalid entries", () => {
    expect(parseResearchInstrumentId("US.")).toBeNull();
    expect(parseResearchInstrumentId("CN.600000")).toBeNull();
    expect(parseResearchInstrumentId(" .AAPL")).toBeNull();
    expect(normalizeResearchQuoteTarget(null)).toBeNull();
    expect(
      normalizeResearchQuoteTarget({
        kind: "plate",
        instrumentId: "HK.BK1001",
        productClass: "equity",
      }),
    ).toMatchObject({ kind: "plate", instrumentId: "HK.BK1001" });
    expect(
      researchQuoteTargetFromEntry(
        {
          basic: {
            security: {
              code: "AAPL",
              name: "Apple",
              securityType: "equity",
            },
          },
        },
        "US",
      ),
    ).toMatchObject({
      kind: "instrument",
      instrumentId: "US.AAPL",
      name: "Apple",
      productClass: "equity",
    });
    expect(
      researchQuoteTargetFromEntry(
        {
          plate: {
            instrumentId: "HK.BK1001",
            plateName: "科技",
          },
        },
        "HK",
      ),
    ).toMatchObject({
      kind: "plate",
      instrumentId: "HK.BK1001",
      name: "科技",
    });
    expect(
      researchQuoteTargetFromEntry({ code: "600000" }, "CN"),
    ).toBeNull();
    expect(
      researchQuoteTargetFromEntry({ instrumentId: "SH.430001" }),
    ).toBeNull();
    expect(isResearchQuoteTarget(null)).toBe(false);
    expect(isResearchQuoteTarget("US.AAPL")).toBe(false);
    expect(
      isResearchQuoteTarget({
        instrumentId: "US.AAPL",
        productClass: "equity",
      }),
    ).toBe(true);
    expect(
      isResearchQuoteEntry({ instrumentId: "US.AAPL", productClass: "bond" }),
    ).toBe(false);
  });

  it("covers empty, single, and multi-batch snapshot responses", async () => {
    expect(await fetchResearchSnapshots(["bad"], "", true)).toEqual([]);

    mocks.fetchEnvelopeWithInit.mockResolvedValueOnce({});
    expect(await fetchResearchSnapshots([" us.aapl "], "", true)).toEqual([]);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenLastCalledWith(
      "/api/v1/market-data/snapshots?refresh=true",
      expect.any(Object),
    );

    mocks.fetchEnvelopeWithInit.mockResolvedValue({});
    const many = Array.from({ length: 201 }, (_, index) => `US.TEST${index}`);
    expect(await fetchResearchSnapshots(many, "futu")).toEqual([]);
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledTimes(3);
  });

  it("keeps only the latest snapshot request and exposes function sources", async () => {
    const ids = ref(["US.SLOW"]);
    const brokerId = ref("futu");
    let rejectSlow: ((reason: unknown) => void) | undefined;
    mocks.fetchEnvelopeWithInit
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectSlow = reject;
          }),
      )
      .mockResolvedValueOnce({
        entries: [{ symbol: "US.FAST", lastPrice: 12 }],
      });

    const scope = effectScope();
    const state = scope.run(() =>
      useResearchSnapshots(() => ids.value, () => brokerId.value),
    )!;
    await nextTick();
    ids.value = ["US.FAST"];
    await nextTick();
    await flushPromises();
    rejectSlow?.("stale failure");
    await flushPromises();

    expect(state.error.value).toBe("");
    expect(state.byInstrumentId.value["US.FAST"]).toMatchObject({
      lastPrice: 12,
    });
    scope.stop();
  });

  it("surfaces current snapshot failures and resets an empty source", async () => {
    const ids = ref(["US.FAIL"]);
    mocks.fetchEnvelopeWithInit.mockRejectedValueOnce("snapshot failed");
    const scope = effectScope();
    const state = scope.run(() => useResearchSnapshots(ids, ref("")))!;
    await flushPromises();
    expect(state.error.value).toBe("snapshot failed");

    ids.value = [];
    await nextTick();
    await flushPromises();
    expect(state.entries.value).toEqual([]);
    expect(state.loading.value).toBe(false);
    expect(state.error.value).toBe("");
    scope.stop();
  });

  it("merges partial snapshots without inventing invalid price deltas", () => {
    const entry = { instrumentId: "us.aapl", name: "Pinned", price: 10 };
    expect(mergeResearchSnapshot(entry, undefined)).toBe(entry);
    expect(
      mergeResearchSnapshot(entry, {
        symbol: "US.AAPL",
        name: "Snapshot",
        previousClose: 0,
        lastPrice: 12,
        fund: { assetClass: "Equity" },
      }),
    ).toMatchObject({
      instrumentId: "US.AAPL",
      name: "Pinned",
      price: 12,
      assetClass: "Equity",
      changeAmount: 12,
      changeRate: undefined,
    });
    expect(
      mergeResearchSnapshot(
        { price: 7, changeAmount: 1, changeRate: 2 },
        {
          instrumentId: "US.BAD",
          previousClose: "bad",
          lastPrice: "bad",
          fund: "not-an-object",
        },
      ),
    ).toMatchObject({
      instrumentId: "US.BAD",
      price: 7,
      changeAmount: 1,
      changeRate: 2,
    });
  });

  it("renders navigation and favorite controls in both states", async () => {
    const navigation = mount(AppNavigationControls, {
      props: { canGoBack: true, canGoForward: false, compact: true },
    });
    expect(
      navigation.get('[data-testid="topbar-navigation-back"]').attributes(
        "disabled",
      ),
    ).toBeUndefined();
    expect(
      navigation.get('[data-testid="topbar-navigation-forward"]').attributes(
        "disabled",
      ),
    ).toBeDefined();
    await navigation.get('[data-testid="topbar-navigation-back"]').trigger("click");
    await navigation.get('[data-testid="topbar-navigation-refresh"]').trigger(
      "click",
    );
    expect(navigation.emitted("back")).toHaveLength(1);
    expect(navigation.emitted("refresh")).toHaveLength(1);
    await navigation.setProps({
      canGoBack: false,
      canGoForward: true,
      compact: false,
    });
    expect(navigation.classes()).not.toContain(
      "app-navigation-controls--compact",
    );
    await navigation
      .get('[data-testid="topbar-navigation-forward"]')
      .trigger("click");
    expect(navigation.emitted("forward")).toHaveLength(1);
    const disabledNavigation = mount(AppNavigationControls, {
      props: { canGoBack: false, canGoForward: false },
    });
    expect(
      disabledNavigation
        .get('[data-testid="topbar-navigation-back"]')
        .attributes("disabled"),
    ).toBeDefined();
    const enabledNavigation = mount(AppNavigationControls, {
      props: { canGoBack: true, canGoForward: true },
    });
    expect(
      enabledNavigation
        .get('[data-testid="topbar-navigation-forward"]')
        .attributes("disabled"),
    ).toBeUndefined();

    const favorite = mount(WatchlistFavoriteButton);
    expect(favorite.get("button").attributes("aria-label")).toBe("加入自选");
    expect(favorite.text()).toBe("☆");
    await favorite.setProps({ active: true, title: "管理入口", disabled: true });
    expect(favorite.get("button").attributes("aria-label")).toBe("管理入口");
    expect(favorite.get("button").attributes("disabled")).toBeDefined();
    const activeFavorite = mount(WatchlistFavoriteButton, {
      props: { active: true },
    });
    expect(activeFavorite.get("button").attributes("aria-label")).toBe(
      "管理自选分组",
    );
    expect(activeFavorite.text()).toBe("★");
    await favorite.setProps({
      active: true,
      title: "自选标题",
      ariaLabel: "自定义标签",
      testId: "favorite-edge",
    });
    expect(favorite.get("button").attributes("title")).toBe("自选标题");
    expect(favorite.get("button").attributes("aria-label")).toBe("自定义标签");
    expect(favorite.get("button").attributes("data-testid")).toBe(
      "favorite-edge",
    );
    expect(favorite.text()).toBe("★");
  });
});
