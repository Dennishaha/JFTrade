// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const apiMocks = vi.hoisted(() => ({
  fetchWithInit: vi.fn(),
  fetchFeature: vi.fn(),
}));
const consoleState = {
  selectedBrokerAccount: ref<{
    brokerId: string;
    accountId: string;
    tradingEnvironment: string;
  } | null>({
    brokerId: "futu",
    accountId: "acc-1",
    tradingEnvironment: "SIMULATE",
  }),
  systemStatus: ref({
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
  }),
};

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: apiMocks.fetchWithInit,
}));
vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: apiMocks.fetchFeature };
});
vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleState,
}));

import OptionChainTable from "../src/components/product/OptionChainTable.vue";
import OptionWorkspacePanel from "../src/components/product/OptionWorkspacePanel.vue";
import { buildOptionComboTemplate } from "../src/composables/optionComboStrategies";
import type { OptionContractChoice } from "../src/composables/optionComboDraft";
import {
  buildOptionChainRows,
  formatOptionMetric,
  normalizeOptionExpirations,
  optionCode,
  optionInstrumentId,
  optionStrike,
} from "../src/composables/optionChainModel";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

const contracts = [
  {
    code: "AAPL-C190",
    label: "CALL 190",
    optionType: "call" as const,
    strike: 190,
    multiplier: 100,
    expiry: "2026-07-17",
  },
  {
    code: "AAPL-P210",
    label: "PUT 210",
    optionType: "put" as const,
    strike: 210,
    multiplier: 100,
    expiry: "2026-07-17",
  },
  {
    code: "AAPL-C220",
    label: "CALL 220",
    optionType: "call" as const,
    strike: 220,
    multiplier: 100,
    expiry: "2026-07-17",
  },
];

function feature(entries: Record<string, unknown>[], metadata = {}) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "test",
      capability: "available",
      selectionReason: "default",
      resolvedAt: "2026-07-17T00:00:00Z",
      asOf: "2026-07-17T00:00:00Z",
    },
    asOf: "2026-07-17T00:00:00Z",
    entries,
    metadata,
  };
}

function expiration(
  strikeTime: string,
  optionExpiryDateDistance: number,
  cycle = 2,
) {
  return { strikeTime, optionExpiryDateDistance, cycle };
}

beforeEach(() => {
  apiMocks.fetchWithInit.mockReset();
  apiMocks.fetchFeature.mockReset();
  consoleState.selectedBrokerAccount.value = {
    brokerId: "futu",
    accountId: "acc-1",
    tradingEnvironment: "SIMULATE",
  };
  consoleState.systemStatus.value = {
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
  };
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("option workspace", () => {
  it("does not turn a quote response into a recursive snapshot request loop", async () => {
    apiMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/expirations/")) {
        return feature([
          expiration("2026-08-21", 33),
          expiration("2026-09-18", 61),
        ]);
      }
      if (path.includes("beginTime=2026-08-21")) {
        return feature([
          {
            strikeTime: "2026-08-21",
            option: [
              {
                call: {
                  basic: {
                    name: "BABA Call 100",
                    lotSize: 100,
                    security: { code: "BABA-C100" },
                  },
                  optionExData: { strikePrice: 100 },
                },
                put: {
                  basic: {
                    name: "BABA Put 100",
                    lotSize: 100,
                    security: { code: "BABA-P100" },
                  },
                  optionExData: { strikePrice: 100 },
                },
              },
            ],
          },
        ]);
      }
      return feature([{ strikeTime: "2026-09-18", option: [] }]);
    });
    apiMocks.fetchWithInit.mockImplementation(
      async (_path: string, init: RequestInit) => {
        const request = JSON.parse(String(init.body)) as {
          instrumentIds: string[];
        };
        return feature(
          request.instrumentIds.map((instrumentId) => ({
            instrumentId,
            lastPrice: instrumentId === "US.BABA" ? 100 : 2,
            bidPrice: 1.9,
            askPrice: 2.1,
          })),
        );
      },
    );
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.BABA", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });

    await flushPromises();
    await flushPromises();
    await flushPromises();
    const settledRequestCount = apiMocks.fetchWithInit.mock.calls.length;
    expect(settledRequestCount).toBeGreaterThan(0);

    await flushPromises();
    await flushPromises();
    await nextTick();
    expect(apiMocks.fetchWithInit).toHaveBeenCalledTimes(settledRequestCount);
    wrapper.unmount();
  });

  it("keeps background quote polling out of the chain loading indicator and serializes refreshes", async () => {
    let resolveChain!: (value: ReturnType<typeof feature>) => void;
    const snapshotResolvers: Array<
      (value: ReturnType<typeof feature>) => void
    > = [];
    apiMocks.fetchFeature.mockImplementation(
      (path: string) => {
        if (path.includes("/expirations/")) {
          return Promise.resolve(feature([expiration("2026-08-21", 33)]));
        }
        return new Promise((resolve) => {
          resolveChain = resolve;
        });
      },
    );
    apiMocks.fetchWithInit.mockImplementation(
      () =>
        new Promise((resolve) => {
          snapshotResolvers.push(resolve);
        }),
    );
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.BABA", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          OptionComboBuilder: { template: "<div />" },
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });
    await nextTick();
    await flushPromises();
    expect(wrapper.find(".option-workspace__chain-progress").exists()).toBe(
      true,
    );

    resolveChain(
      feature([
        {
          strikeTime: "2026-08-21",
          option: [
            {
              call: {
                basic: { security: { code: "BABA-C100" } },
                optionExData: { strikePrice: 100 },
              },
            },
          ],
        },
      ]),
    );
    await flushPromises();
    await flushPromises();
    expect(apiMocks.fetchWithInit).toHaveBeenCalledTimes(1);
    expect(wrapper.find(".option-workspace__chain-progress").exists()).toBe(
      false,
    );

    const state = setupState<{
      loadVisibleSnapshots: () => Promise<void>;
    }>(wrapper);
    void state.loadVisibleSnapshots();
    void state.loadVisibleSnapshots();
    await flushPromises();
    expect(apiMocks.fetchWithInit).toHaveBeenCalledTimes(1);

    snapshotResolvers.shift()!(feature([]));
    await flushPromises();
    await flushPromises();
    expect(apiMocks.fetchWithInit).toHaveBeenCalledTimes(2);
    snapshotResolvers.shift()!(feature([]));
    await flushPromises();
    wrapper.unmount();
  });

  it("clears an in-flight chain indicator when leaving the chain view", async () => {
    let resolveChain!: (value: ReturnType<typeof feature>) => void;
    apiMocks.fetchFeature.mockImplementation(
      (path: string) => {
        if (path.includes("/expirations/")) {
          return Promise.resolve(feature([expiration("2026-08-21", 33)]));
        }
        return new Promise((resolve) => {
          resolveChain = resolve;
        });
      },
    );
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.BABA", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          OptionComboBuilder: { template: "<div />" },
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });
    await nextTick();
    await flushPromises();
    expect(wrapper.find(".option-workspace__chain-progress").exists()).toBe(
      true,
    );
    const state = setupState<{ section: string }>(wrapper);
    state.section = "analysis";
    await nextTick();
    expect(wrapper.find(".option-workspace__chain-progress").exists()).toBe(
      false,
    );
    resolveChain(feature([]));
    await flushPromises();
    expect(wrapper.find(".option-workspace__chain-progress").exists()).toBe(
      false,
    );
    wrapper.unmount();
  });

  it("keeps the selected chain when next-expiry prefetch fails and retries on selection", async () => {
    let nextExpiryAttempts = 0;
    apiMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/expirations/")) {
        return feature([
          expiration("2026-08-21", 33),
          expiration("2026-09-18", 61),
        ]);
      }
      if (path.includes("beginTime=2026-08-21")) {
        return feature([
          {
            strikeTime: "2026-08-21",
            option: [
              {
                call: {
                  basic: { security: { code: "TME-C9" } },
                  optionExData: { strikePrice: 9 },
                },
              },
            ],
          },
        ]);
      }
      nextExpiryAttempts += 1;
      if (nextExpiryAttempts === 1) throw new Error("预取失败");
      return feature([
        {
          strikeTime: "2026-09-18",
          option: [
            {
              call: {
                basic: { security: { code: "TME-C10" } },
                optionExData: { strikePrice: 10 },
              },
            },
          ],
        },
      ]);
    });
    apiMocks.fetchWithInit.mockResolvedValue(feature([]));
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.TME", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });

    await flushPromises();
    await flushPromises();
    expect(wrapper.text()).not.toContain("预取失败");
    expect(
      setupState<{ optionRows: unknown[] }>(wrapper).optionRows,
    ).toHaveLength(1);

    await wrapper
      .findAll(".option-workspace__expiry-list button")[1]!
      .trigger("click");
    await flushPromises();
    await flushPromises();
    const state = setupState<{
      selectedExpiry: string;
      optionRows: unknown[];
    }>(wrapper);
    expect(nextExpiryAttempts).toBe(2);
    expect(state.selectedExpiry).toBe("2026-09-18");
    expect(state.optionRows).toHaveLength(1);
    wrapper.unmount();
  });

  it("shows a distinct empty state when the catalog has no active expiry", async () => {
    apiMocks.fetchFeature.mockResolvedValue(
      feature([expiration("2026-07-17", -2)]),
    );
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.TME", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });

    await flushPromises();
    expect(wrapper.text()).toContain("当前标的暂无未到期期权合约");
    expect(
      apiMocks.fetchFeature.mock.calls.some((call) =>
        String(call[0]).includes("/options/chains/"),
      ),
    ).toBe(false);
    wrapper.unmount();
  });

  it("distinguishes an empty selected expiry from an empty catalog", async () => {
    apiMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/expirations/")) {
        return feature([expiration("2026-08-21", 33)]);
      }
      return feature([
        {
          strikeTime: "2026-09-18",
          option: [
            {
              call: {
                basic: { security: { code: "TME-C10" } },
                optionExData: { strikePrice: 10 },
              },
            },
          ],
        },
      ]);
    });
    apiMocks.fetchWithInit.mockResolvedValue(feature([]));
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.TME", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });

    await flushPromises();
    await flushPromises();
    expect(wrapper.text()).toContain("该到期日暂无期权合约");
    expect(wrapper.text()).not.toContain("当前标的暂无未到期期权合约");
    wrapper.unmount();
  });

  it("renders a paged symmetric chain with quota-safe snapshots, moneyness, and Greeks", async () => {
    vi.useFakeTimers();
    const rows = Array.from({ length: 22 }, (_, index) => {
      const strike = 190 + index;
      return {
        call: {
          basic: {
            name: `Call ${strike}`,
            lotSize: 100,
            security: { code: `C${strike}` },
          },
          optionExData: { strikePrice: strike },
        },
        put: {
          basic: {
            name: `Put ${strike}`,
            lotSize: 100,
            security: { code: `P${strike}` },
          },
          optionExData: { strikePrice: strike },
        },
      };
    });
    rows.push({
      call: { basic: { name: "invalid", security: {} }, optionExData: {} },
      put: { basic: { name: "invalid", security: {} }, optionExData: {} },
    } as (typeof rows)[number]);
    apiMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/expirations/")) {
        return feature([
          expiration("2026-07-17", -2),
          expiration("2026-08-21", 33),
          expiration("2026-08-21", 33),
          expiration("2026-09-18", 61),
          expiration("2028-01-21", 551),
          expiration("2028-06-16", 698),
          expiration("2029-01-19", 915),
        ]);
      }
      if (path.includes("beginTime=2026-08-21")) {
        return feature([{ strikeTime: "2026-08-21", option: rows }]);
      }
      if (path.includes("beginTime=2026-09-18")) {
        return feature([
          {
            strikeTime: "2026-09-18",
            option: [
              {
                call: {
                  basic: {
                    name: "Far Call 200",
                    lotSize: 100,
                    security: { code: "C200-FAR" },
                  },
                  optionExData: { strikePrice: 200 },
                },
                put: {
                  basic: {
                    name: "Far Put 200",
                    lotSize: 100,
                    security: { code: "P200-FAR" },
                  },
                  optionExData: { strikePrice: 200 },
                },
              },
            ],
          },
        ]);
      }
      return feature([]);
    });
    apiMocks.fetchWithInit.mockImplementation(
      async (_path: string, init: RequestInit) => {
        const request = JSON.parse(String(init.body)) as {
          instrumentIds: string[];
        };
        return feature(
          request.instrumentIds.map((symbol) => ({
            symbol,
            lastPrice: symbol === "US.AAPL" ? 200.2 : 3.1,
            bidPrice: 3,
            askPrice: 3.2,
            option:
              symbol === "US.AAPL"
                ? undefined
                : {
                    impliedVolatility: 30,
                    delta: symbol.includes(".C") ? 0.5 : -0.5,
                    gamma: 0.03,
                    theta: -0.08,
                    vega: 0.12,
                  },
          })),
        );
      },
    );
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "US.AAPL", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          OptionComboBuilder: { template: "<div class='combo-stub' />" },
          ProductFeaturePanel: {
            emits: ["openInstrument"],
            template:
              "<button class='feature-stub' @click=\"$emit('openInstrument', 'US.MSFT')\">feature</button>",
          },
        },
      },
    });
    await flushPromises();
    await flushPromises();
    const state = setupState<{
      optionRows: Record<string, unknown>[];
      visibleOptionRows: Record<string, unknown>[];
      chainRows: Array<{
        call: {
          instrumentId: string;
          moneyness: string;
          bidPrice: number | null;
          delta: number | null;
          gamma: number | null;
        };
        put: { moneyness: string };
      }>;
      chainPage: number;
      chainPageCount: number;
      atmStrike: number;
      rangedChainRows: unknown[];
      expirations: Array<{
        date: string;
        daysToExpiry: number;
        cycleLabel: string;
      }>;
      comboContracts: OptionContractChoice[];
      selectedExpiry: string;
      strikeRange: string;
      section: string;
      analysisOperation: string;
      eventOperation: string;
      strategyType: string;
      featurePath: string;
      selectedContract: { instrumentId: string } | null;
      formatExpiry: (value: string) => string;
      loadSelectedChain: () => Promise<void>;
      selectComboLeg: (
        contract: { instrumentId: string },
        side: "buy" | "sell",
      ) => void;
    }>(wrapper);

    expect(state.expirations.map((expiry) => expiry.date)).toEqual([
      "2026-08-21",
      "2026-09-18",
      "2028-01-21",
      "2028-06-16",
      "2029-01-19",
    ]);
    expect(state.expirations[0]).toMatchObject({
      daysToExpiry: 33,
      cycleLabel: "月",
    });
    expect(state.comboContracts.map((contract) => contract.expiry)).toEqual(
      expect.arrayContaining(["2026-08-21", "2026-09-18"]),
    );
    expect(
      buildOptionComboTemplate(
        "calendar",
        state.comboContracts,
        "2026-08-21",
        200,
      ).map((leg) => leg.expiry),
    ).toEqual(["2026-08-21", "2026-09-18"]);
    expect(state.optionRows).toHaveLength(23);
    expect(state.visibleOptionRows).toHaveLength(20);
    expect(state.chainPageCount).toBe(2);
    expect(state.atmStrike).toBe(200);
    expect(state.chainRows[0]!.call.moneyness).toBe("ITM");
    expect(state.chainRows[0]!.put.moneyness).toBe("OTM");
    expect(state.chainRows[10]!.call.moneyness).toBe("ATM");
    expect(state.chainRows[0]!.call.bidPrice).toBe(3);
    expect(state.chainRows[0]!.call.delta).toBe(0.5);
    expect(state.chainRows[0]!.call.gamma).toBe(0.03);
    expect(formatOptionMetric(null)).toBe("—");
    expect(wrapper.text()).toContain("ITM");
    await wrapper.get(".option-workspace__expiry-list button").trigger("click");
    const contractButtons = wrapper
      .getComponent(OptionChainTable)
      .findAll(".is-contract button")
      .filter((button) => !button.attributes("disabled"));
    await contractButtons[0]!.trigger("click");
    await contractButtons[1]!.trigger("click");
    await flushPromises();
    expect(state.selectedContract?.instrumentId).toBe("US.P190");
    const contractAnalysisPaths = apiMocks.fetchFeature.mock.calls
      .map((call) => String(call[0]))
      .filter((path) => path.includes("/options/analysis/"));
    expect(contractAnalysisPaths).toEqual(
      expect.arrayContaining([
        expect.stringContaining(
          "/options/analysis/US.P190?market=US&operation=quote",
        ),
        expect.stringContaining(
          "/options/analysis/US.P190?market=US&operation=volatility",
        ),
        expect.stringContaining(
          "/options/analysis/US.P190?market=US&operation=exercise_probability",
        ),
      ]),
    );
    expect(
      contractAnalysisPaths.some(
        (path) =>
          path.includes("/US.AAPL") &&
          (path.includes("operation=volatility") ||
            path.includes("operation=exercise_probability")),
      ),
    ).toBe(false);
    expect(wrapper.text()).toContain("合约分析");
    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("打开合约工作区"))!
      .trigger("click");
    expect(wrapper.emitted("openInstrument")?.[0]).toEqual(["US.P190"]);
    await wrapper.get('button[aria-label="关闭合约分析"]').trigger("click");

    const rangeButtons = wrapper.findAll(
      ".option-workspace__range-toggle button",
    );
    await rangeButtons[1]!.trigger("click");
    expect(state.strikeRange).toBe("near_atm");
    expect(state.rangedChainRows).toHaveLength(20);
    await rangeButtons[0]!.trigger("click");
    expect(state.rangedChainRows).toHaveLength(23);
    expect(state.formatExpiry("invalid-date")).toBe("invalid-date");
    expect(state.formatExpiry("2026-08-21")).toBe("2026/08/21");

    await wrapper.get(".pagination-next").trigger("click");
    await flushPromises();
    expect(state.visibleOptionRows).toHaveLength(3);
    const body = JSON.parse(
      String(apiMocks.fetchWithInit.mock.calls.at(-1)?.[1]?.body),
    );
    expect(body.instrumentIds).toHaveLength(5);

    expect(wrapper.find(".option-workspace__date-range").exists()).toBe(false);
    const chainPaths = apiMocks.fetchFeature.mock.calls
      .map((call) => String(call[0]))
      .filter((path) => path.includes("/options/chains/"));
    expect(chainPaths).toEqual(
      expect.arrayContaining([
        expect.stringContaining(
          "beginTime=2026-08-21&endTime=2026-08-21",
        ),
        expect.stringContaining(
          "beginTime=2026-09-18&endTime=2026-09-18",
        ),
      ]),
    );
    expect(
      chainPaths.every((path) => {
        const query = new URL(path, "http://localhost").searchParams;
        return query.get("beginTime") === query.get("endTime");
      }),
    ).toBe(true);
    state.selectComboLeg(state.chainRows[0]!.call, "buy");
    await state.loadSelectedChain();
    const expiryExpand = wrapper.get(
      'button[aria-label="展开全部到期日"]',
    );
    await expiryExpand.trigger("click");
    expect(wrapper.get('[aria-label="其余全部到期日"]').text()).toContain(
      "2029/01/19",
    );
    await wrapper
      .get('[aria-label="其余全部到期日"] button:last-child')
      .trigger("click");
    await flushPromises();
    expect(state.selectedExpiry).toBe("2029-01-19");
    expect(wrapper.find('[aria-label="其余全部到期日"]').exists()).toBe(false);
    expect(wrapper.get(".option-workspace__expiry-list").text()).toContain(
      "2029/01/19",
    );
    wrapper.getComponent({ name: "VBtnToggle" }).vm.$emit(
      "update:modelValue",
      "analysis",
    );
    await nextTick();
    expect(wrapper.text()).not.toContain("行权概率");
    await wrapper.get("select").setValue("historical_statistics");
    expect(state.featurePath).toContain("operation=historical_statistics");

    state.section = "analysis";
    state.analysisOperation = "historical_volatility";
    await nextTick();
    expect(state.featurePath).toContain("operation=historical_volatility");
    state.section = "events";
    state.eventOperation = "seller";
    await nextTick();
    await wrapper.get("select").setValue("earnings");
    await flushPromises();
    expect(state.featurePath).toBe("");
    expect(apiMocks.fetchFeature).toHaveBeenCalledWith(
      expect.stringContaining("underlying=US.AAPL"),
    );
    state.section = "strategy";
    state.strategyType = "4";
    await nextTick();
    await wrapper.get("select").setValue("5");
    expect(state.featurePath).toContain("option_strategy=5");
    await wrapper.get(".feature-stub").trigger("click");
    expect(wrapper.emitted("openInstrument")?.at(-1)).toEqual(["US.MSFT"]);
    await vi.advanceTimersByTimeAsync(3_000);
    wrapper.unmount();
  });

  it("clears stale chains on errors and tolerates missing contract fields", async () => {
    apiMocks.fetchFeature
      .mockRejectedValueOnce(new Error("目录失败"))
      .mockRejectedValueOnce("字符串失败");
    const wrapper = mount(OptionWorkspacePanel, {
      props: { instrumentId: "HK.00700", market: "HK" },
      global: {
        stubs: {
          ...productGlobalStubs,
          OptionComboBuilder: { template: "<div />" },
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("目录失败");
    expect(wrapper.text()).not.toContain("当前标的暂无未到期期权合约");
    const state = setupState<{
      loadExpirationCatalog: () => Promise<void>;
      loadVisibleSnapshots: () => Promise<void>;
      section: string;
      expirationError: string;
      snapshotError: string;
    }>(wrapper);
    await state.loadExpirationCatalog();
    expect(wrapper.text()).toContain("字符串失败");
    expect(optionCode({}, "call")).toBe("");
    expect(optionInstrumentId("US", "")).toBe("");
    expect(optionStrike({})).toBeNull();
    expect(
      normalizeOptionExpirations([
        expiration("2026-07-17", -2),
        expiration("2026-08-21", 33),
        expiration("2026-08-21", 33, 0),
        { strikeTime: "invalid", optionExpiryDateDistance: 1 },
      ]),
    ).toEqual([
      {
        date: "2026-08-21",
        daysToExpiry: 33,
        cycle: 2,
        cycleLabel: "月",
      },
    ]);
    expect(buildOptionChainRows([{}], {}, "US", null)[0]).toMatchObject({
      strike: null,
      isAtm: false,
      call: { moneyness: "" },
    });
    state.section = "analysis";
    await state.loadVisibleSnapshots();
    state.section = "chain";
    apiMocks.fetchFeature.mockImplementation(async (path: string) => {
      if (path.includes("/expirations/")) {
        return feature([expiration("2026-08-21", 33)]);
      }
      return feature([
        {
          strikeTime: "2026-08-21",
          option: [
            {
              call: {
                basic: { security: { code: "C200" } },
                optionExData: { strikePrice: 200 },
              },
            },
          ],
        },
      ]);
    });
    apiMocks.fetchWithInit.mockRejectedValue(new Error("快照失败"));
    await state.loadExpirationCatalog();
    await flushPromises();
    await flushPromises();
    await state.loadVisibleSnapshots();
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain(
        "实时合约价格暂不可用，期权链与行权价仍可查看。",
      );
    });
    apiMocks.fetchWithInit.mockRejectedValue("快照字符串失败");
    await state.loadVisibleSnapshots();
    await vi.waitFor(() => {
      expect(state.snapshotError).toContain("实时合约价格暂不可用");
    });

    const empty = mount(OptionWorkspacePanel, {
      props: { instrumentId: "", market: "US" },
      global: {
        stubs: {
          ...productGlobalStubs,
          OptionComboBuilder: { template: "<div />" },
          ProductFeaturePanel: { template: "<div />" },
        },
      },
    });
    const emptyState = setupState<{
      loadSelectedChain: () => Promise<void>;
      loadVisibleSnapshots: () => Promise<void>;
      section: string;
      featurePath: string;
    }>(empty);
    await emptyState.loadSelectedChain();
    await emptyState.loadVisibleSnapshots();
    emptyState.section = "analysis";
    await nextTick();
    expect(emptyState.featurePath).toBe("");
  });
});
