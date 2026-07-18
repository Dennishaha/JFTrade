// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  fetchFeature: vi.fn(),
}));

vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: apiMocks.fetchFeature };
});

import OptionContractAnalysisDrawer from "../src/components/product/OptionContractAnalysisDrawer.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import type { OptionChainSideModel } from "../src/composables/optionChainModel";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

function feature(entries: Record<string, unknown>[], metadata = {}) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "derivatives.option_analysis",
      capability: "available",
      selectionReason: "explicit_broker",
      resolvedAt: "2026-07-18T00:00:00Z",
      asOf: "2026-07-18T00:00:00Z",
    },
    asOf: "2026-07-18T00:00:00Z",
    entries,
    metadata,
  };
}

function contract(instrumentId: string): OptionChainSideModel {
  return {
    code: instrumentId.split(".").at(-1) ?? "",
    instrumentId,
    name: "BABA Call",
    bidPrice: 2.3,
    askPrice: 2.5,
    impliedVolatility: 0.31,
    delta: 0.5,
    gamma: 0.03,
    theta: -0.08,
    vega: 0.12,
    moneyness: "ATM",
    multiplier: 100,
  };
}

beforeEach(() => {
  apiMocks.fetchFeature.mockReset();
  resetBrokerProviderSelectionForTests();
  useBrokerProviderSelection().selectBrokerProvider("futu");
});

describe("option contract analysis drawer", () => {
  it("keeps partial contract analysis visible and reports each failed operation", async () => {
    apiMocks.fetchFeature.mockImplementation((path: string) => {
      if (path.includes("operation=quote")) {
        return Promise.resolve(
          feature([
            {
              price: 2.4,
              strike: 180,
              delta: 0.51,
              gamma: 0.02,
              theta: -0.07,
              vega: 0.11,
              expireTime: "2026-07-24",
            },
          ]),
        );
      }
      if (path.includes("operation=volatility")) {
        return Promise.reject(new Error("volatility denied"));
      }
      return Promise.reject("probability denied");
    });
    const selected = contract("US.BABA260724C180000");
    const wrapper = mount(OptionContractAnalysisDrawer, {
      props: { contract: selected, market: "US" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    await flushPromises();

    expect(wrapper.text()).toContain("2.4");
    expect(wrapper.text()).toContain("volatility denied");
    expect(wrapper.text()).toContain("probability denied");
    expect(
      apiMocks.fetchFeature.mock.calls.every(([path]) =>
        String(path).includes("/options/analysis/US.BABA260724C180000"),
      ),
    ).toBe(true);

    const state = setupState<{
      finiteValue: (value: unknown) => number | null;
      metric: (
        entry: Record<string, unknown>,
        ...keysOrValues: unknown[]
      ) => string;
      textMetric: (
        entry: Record<string, unknown>,
        ...keys: string[]
      ) => string;
    }>(wrapper);
    expect(state.finiteValue("bad")).toBeNull();
    expect(state.metric({ markPrice: 3.1 }, "price", "markPrice")).toBe("3.1");
    expect(state.metric({}, "missing")).toBe("—");
    expect(state.textMetric({ expiry: " 2026-07-24 " }, "empty", "expiry")).toBe(
      "2026-07-24",
    );
    expect(state.textMetric({}, "expiry")).toBe("—");

    await wrapper.get('button[aria-label="关闭合约分析"]').trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
    await wrapper
      .findAll("button")
      .find((button) => button.text().includes("打开合约工作区"))!
      .trigger("click");
    expect(wrapper.emitted("openWorkspace")?.[0]).toEqual([
      "US.BABA260724C180000",
    ]);
  });

  it("does not request analysis until a concrete option contract exists", async () => {
    const initialCallCount = apiMocks.fetchFeature.mock.calls.length;
    const wrapper = mount(OptionContractAnalysisDrawer, {
      props: { contract: contract(""), market: "US" },
      global: { stubs: productGlobalStubs },
    });
    await flushPromises();
    expect(apiMocks.fetchFeature).toHaveBeenCalledTimes(initialCallCount);

    apiMocks.fetchFeature.mockResolvedValue(feature([]));
    await wrapper.setProps({ contract: contract("US.BABA260724P180000") });
    await flushPromises();
    await flushPromises();
    expect(apiMocks.fetchFeature).toHaveBeenCalledTimes(initialCallCount + 3);

    const state = setupState<{ load: () => Promise<void> }>(wrapper);
    const pendingResolvers: Array<
      (value: ReturnType<typeof feature>) => void
    > = [];
    apiMocks.fetchFeature.mockImplementation(
      () =>
        new Promise((resolve) => {
          pendingResolvers.push(resolve);
        }),
    );
    const staleLoad = state.load();
    apiMocks.fetchFeature.mockResolvedValue(feature([]));
    await state.load();
    for (const resolve of pendingResolvers) resolve(feature([]));
    await staleLoad;

    await wrapper.setProps({ contract: null });
    await flushPromises();
    expect(wrapper.find('[aria-label="期权合约分析"]').exists()).toBe(false);
  });
});
