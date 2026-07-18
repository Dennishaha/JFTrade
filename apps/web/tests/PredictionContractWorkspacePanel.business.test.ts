// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const predictionWorkspaceMocks = vi.hoisted(() => ({
  fetchFeature: vi.fn(),
  fetchWithInit: vi.fn(),
}));

const consoleState = {
  selectedBrokerAccount: ref<{
    brokerId: string;
    accountId: string;
    tradingEnvironment: string;
  } | null>({
    brokerId: "futu",
    accountId: "moomoo-us",
    tradingEnvironment: "REAL",
  }),
  systemStatus: ref({
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
  }),
};

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: predictionWorkspaceMocks.fetchWithInit,
}));
vi.mock("../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/composables/productFeatures")>();
  return {
    ...actual,
    fetchProductFeature: predictionWorkspaceMocks.fetchFeature,
  };
});
vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleState,
}));

import PredictionContractWorkspacePanel from "../src/components/product/PredictionContractWorkspacePanel.vue";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";
import {
  flushPromises,
  productGlobalStubs,
  setupState,
} from "./productTestUtils";

function feature(
  entries: Record<string, unknown>[],
  warnings: string[] = [],
) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "prediction.snapshot",
      capability: "available",
      selectionReason: "explicit",
      resolvedAt: "2026-07-18T00:00:00Z",
      asOf: "2026-07-18T00:00:00Z",
    },
    resolvedInstrument: {
      instrumentId: "US.EC.HOME",
      code: "EC.HOME",
      name: "主队获胜",
      productClass: "event_contract",
      marketSegment: "prediction",
      quoteMarket: "US",
      quantityMode: "amount",
    },
    asOf: "2026-07-18T00:00:00Z",
    entries,
    warnings,
  };
}

function setVisibility(value: "visible" | "hidden"): void {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value,
  });
  document.dispatchEvent(new Event("visibilitychange"));
}

async function settleWorkspace(): Promise<void> {
  await flushPromises();
  await flushPromises();
  await nextTick();
}

beforeEach(() => {
  vi.useFakeTimers();
  predictionWorkspaceMocks.fetchFeature.mockReset();
  predictionWorkspaceMocks.fetchWithInit.mockReset();
  resetBrokerProviderSelectionForTests();
  useBrokerProviderSelection().selectBrokerProvider("futu");
  consoleState.selectedBrokerAccount.value = {
    brokerId: "futu",
    accountId: "moomoo-us",
    tradingEnvironment: "REAL",
  };
  consoleState.systemStatus.value.defaultBroker = "futu";
  setVisibility("visible");
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  resetBrokerProviderSelectionForTests();
});

describe("prediction contract workspace", () => {
  it("renders every contract view and leases only the visible realtime view", async () => {
    predictionWorkspaceMocks.fetchFeature.mockImplementation(
      async (path: string) => {
        if (path.includes("/snapshot")) {
          return feature(
            [
              {
                status: "OPEN",
                price: 0.62,
                yesBid: 0.61,
                yesAsk: 0.63,
                noBid: 0.37,
                noAsk: 0.39,
                volume24h: 12500,
                openInterest: 8000,
              },
            ],
            ["报价由可见合约实时更新"],
          );
        }
        if (path.includes("/order-book")) {
          return feature([
            {
              yesBids: [
                { price: 0.61, size: 120 },
                { price: 0.6, size: 80 },
              ],
              yesAsks: [{ price: 0.63, size: 90 }],
              noBids: [{ price: 0.37, size: 60 }],
              noAsks: [{ price: 0.39, size: 70 }],
            },
          ]);
        }
        if (path.includes("/candles")) {
          return feature([
            {
              klineList: [
                {
                  timeKey: "10:00",
                  open: 0.58,
                  high: 0.63,
                  low: 0.57,
                  close: 0.62,
                  volume: 500,
                },
              ],
            },
          ]);
        }
        if (path.includes("/ticks")) {
          return feature([
            {
              tickerList: [
                {
                  time: "10:01:02",
                  side: "BUY",
                  yesPrice: 0.62,
                  noPrice: 0.38,
                  volume: 15,
                  sequence: "88",
                },
              ],
            },
          ]);
        }
        if (path.includes("/milestones")) {
          return feature([
            {
              instrumentId: "US.EVENT.1",
              title: "官方结果确认",
              type: "SETTLEMENT",
              startDate: "2026-07-18",
              endDate: "2026-07-19",
              notificationMessage: "以联盟官方结果为准",
            },
          ]);
        }
        throw new Error(`unexpected prediction path ${path}`);
      },
    );
    let leaseSequence = 0;
    predictionWorkspaceMocks.fetchWithInit.mockImplementation(
      async (path: string, init?: RequestInit) => {
        if (init?.method === "DELETE") return { released: true };
        leaseSequence += 1;
        return {
          leaseId: `lease-${leaseSequence}`,
          instrumentId: "EC.HOME",
        };
      },
    );

    const wrapper = mount(PredictionContractWorkspacePanel, {
      props: { instrumentId: "us.ec.home", view: "contract" },
      global: { stubs: productGlobalStubs },
    });
    await settleWorkspace();
    expect(wrapper.text()).toContain("合约行情");
    expect(wrapper.get(".prediction-contract__header").text()).not.toContain(
      "主队获胜",
    );
    expect(wrapper.text()).toContain("OPEN");
    expect(wrapper.text()).toContain("12,500");
    expect(wrapper.text()).toContain("报价由可见合约实时更新");
    expect(predictionWorkspaceMocks.fetchFeature.mock.calls[0]?.[0]).toContain(
      "/contracts/EC.HOME/snapshot?brokerId=futu",
    );
    expect(predictionWorkspaceMocks.fetchWithInit).not.toHaveBeenCalled();

    await wrapper.setProps({ view: "depth" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("YES 买");
    expect(wrapper.text()).toContain("120");
    let subscribeCall = predictionWorkspaceMocks.fetchWithInit.mock.calls.find(
      ([path, init]) =>
        String(path).includes("/subscriptions?") && init?.method === "POST",
    );
    expect(subscribeCall?.[0]).toContain(
      "brokerId=futu&accountId=moomoo-us",
    );
    expect(JSON.parse(String(subscribeCall?.[1]?.body))).toEqual({
      dataTypes: ["ORDER_BOOK"],
    });

    await wrapper.setProps({ view: "chart" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("10:00");
    expect(wrapper.text()).toContain("500");
    subscribeCall = predictionWorkspaceMocks.fetchWithInit.mock.calls.at(-1);
    expect(JSON.parse(String(subscribeCall?.[1]?.body))).toEqual({
      dataTypes: ["KLINE"],
    });

    await wrapper.setProps({ view: "ticks" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("10:01:02");
    expect(wrapper.text()).toContain("88");
    subscribeCall = predictionWorkspaceMocks.fetchWithInit.mock.calls.at(-1);
    expect(JSON.parse(String(subscribeCall?.[1]?.body))).toEqual({
      dataTypes: ["TICKER"],
    });

    await wrapper.setProps({ view: "rules" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("官方结果确认");
    expect(wrapper.text()).toContain("以联盟官方结果为准");
    expect(
      predictionWorkspaceMocks.fetchWithInit.mock.calls.some(
        ([, init]) => init?.method === "DELETE",
      ),
    ).toBe(true);

    const state = setupState<{
      lease: { leaseId: string } | null;
      visible: boolean;
    }>(wrapper);
    await wrapper.setProps({ view: "depth" });
    await settleWorkspace();
    expect(state.lease?.leaseId).toBeTruthy();
    setVisibility("hidden");
    await settleWorkspace();
    expect(state.visible).toBe(false);
    expect(state.lease).toBeNull();
    setVisibility("visible");
    await settleWorkspace();
    expect(state.visible).toBe(true);
    expect(state.lease?.leaseId).toBeTruthy();

    const loadCount = predictionWorkspaceMocks.fetchFeature.mock.calls.length;
    await vi.advanceTimersByTimeAsync(3_000);
    expect(predictionWorkspaceMocks.fetchFeature.mock.calls.length).toBeGreaterThan(
      loadCount,
    );
    wrapper.unmount();
    await flushPromises();
    expect(
      predictionWorkspaceMocks.fetchWithInit.mock.calls.at(-1)?.[1]?.method,
    ).toBe("DELETE");
  });

  it("shows empty and failure states and tolerates release failure", async () => {
    predictionWorkspaceMocks.fetchWithInit.mockResolvedValue({
      leaseId: "lease-fail",
      instrumentId: "EC.FAIL",
    });
    const wrapper = mount(PredictionContractWorkspacePanel, {
      props: { instrumentId: "", view: "contract" },
      global: { stubs: productGlobalStubs },
    });
    await settleWorkspace();
    expect(wrapper.text()).toContain("当前没有可展示的数据");

    predictionWorkspaceMocks.fetchFeature.mockRejectedValueOnce(
      new Error("预测行情权限不足"),
    );
    await wrapper.setProps({ instrumentId: "US.EC.FAIL" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("预测行情权限不足");

    predictionWorkspaceMocks.fetchFeature.mockRejectedValueOnce("行情失败");
    await wrapper.setProps({ view: "depth" });
    await settleWorkspace();
    expect(wrapper.text()).toContain("行情失败");

    predictionWorkspaceMocks.fetchWithInit.mockRejectedValueOnce(
      new Error("release failed"),
    );
    wrapper.unmount();
    await nextTick();
  });
});
