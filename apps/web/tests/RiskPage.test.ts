// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import {
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
} from "@/contracts";

const testState = vi.hoisted(() => ({
  store: null as null | Record<string, any>,
  fetchEnvelopeMock: vi.fn(),
  fetchEnvelopeWithInitMock: vi.fn(),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => testState.store,
}));

vi.mock("../src/composables/apiClient", () => ({
  apiPost: vi.fn(async () => ({})),
  apiPostPath: vi.fn(async () => ({})),
  fetchEnvelope: (...args: unknown[]) => testState.fetchEnvelopeMock(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) =>
    testState.fetchEnvelopeWithInitMock(...args),
}));

import RiskPage from "../src/pages/RiskPage.vue";
import InstrumentSearchBox from "../src/components/domain/market-data/InstrumentSearchBox.vue";
import RealTradeEmergencyPanel from "../src/components/risk/RealTradeEmergencyPanel.vue";

function createRiskStore(
  riskState = emptyRealTradeRiskState,
) {
  return {
    loadSystemState: vi.fn(async () => undefined),
    realTradeHardStops: ref(emptyRealTradeHardStops),
    realTradeKillSwitchEvents: ref(emptyRealTradeKillSwitchEvents),
    realTradeKillSwitchState: ref(emptyRealTradeKillSwitchState),
    realTradeRiskEvents: ref(emptyRealTradeRiskEvents),
    realTradeRiskState: ref(riskState),
    selectedBrokerAccount: ref(null),
  };
}

function mountRiskPage() {
  return mount(RiskPage);
}

async function switchToTab(
  wrapper: ReturnType<typeof mountRiskPage>,
  label: string,
) {
  await wrapper
    .findAll(".risk-main__tabs button")
    .find((button) => button.text().includes(label))!
    .trigger("click");
  await flushPromises();
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("RiskPage", () => {
  it("emits both emergency actions and exposes their loading states", async () => {
    const wrapper = mount(RealTradeEmergencyPanel, {
      props: {
        killSwitch: {
          ...emptyRealTradeKillSwitchState,
          killSwitchActive: false,
          allowsCancel: true,
          blockedOperations: ["PLACE", "MODIFY"],
        },
        loadingAction: "",
      },
    });

    expect(wrapper.text()).toContain("下单与改单未被熔断阻断");
    const buttons = wrapper.findAll(".emergency-panel__actions button");
    await buttons[0]?.trigger("click");
    await buttons[1]?.trigger("click");
    expect(wrapper.emitted("activate")).toHaveLength(1);
    expect(wrapper.emitted("release")).toHaveLength(1);

    await wrapper.setProps({
      killSwitch: {
        ...emptyRealTradeKillSwitchState,
        killSwitchActive: true,
        allowsCancel: false,
        blockedOperations: ["PLACE"],
      },
      loadingAction: "kill-switch.release",
    });

    expect(wrapper.text()).toContain("下单与改单已被阻断");
    expect(wrapper.text()).toContain("撤单阻断");
    expect(buttons[1]?.attributes("disabled")).toBeDefined();
    expect(buttons[1]?.text()).toBe("解除中...");
  });

  it("colors status blocks by real-trade safety state", async () => {
    const store = createRiskStore();
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    const blocksByLabel = () =>
      Object.fromEntries(
        wrapper.findAll(".risk-sidebar__row").map((block) => [
          block.get("span").text(),
          {
            value: block.get("b").text(),
            classes: block.classes(),
          },
        ]),
      );

    expect(blocksByLabel()).toMatchObject({
      实盘总闸: {
        value: "未开放",
        classes: expect.arrayContaining(["tv-status--warning"]),
      },
      单笔限额: {
        value: "未配置",
        classes: expect.arrayContaining(["tv-status--warning"]),
      },
      紧急熔断: {
        value: "未激活",
        classes: expect.arrayContaining(["tv-status--success"]),
      },
      硬停止: {
        value: "0 条",
        classes: expect.arrayContaining(["tv-status--success"]),
      },
    });

    store.realTradeRiskState.value = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
    };
    store.realTradeKillSwitchState.value = {
      ...emptyRealTradeKillSwitchState,
      killSwitchActive: true,
    };
    store.realTradeHardStops.value = {
      ...emptyRealTradeHardStops,
      entries: [
        {
          id: "hard-stop-1",
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: null,
          symbol: null,
          operatorId: "ops-a",
          reason: "manual freeze",
          activatedAt: "2026-07-04T00:00:00.000Z",
          updatedAt: "2026-07-04T00:00:00.000Z",
        },
      ],
    };
    await flushPromises();

    expect(blocksByLabel()).toMatchObject({
      实盘总闸: {
        value: "已开放",
        classes: expect.arrayContaining(["tv-status--success"]),
      },
      单笔限额: {
        value: "已配置",
        classes: expect.arrayContaining(["tv-status--success"]),
      },
      紧急熔断: {
        value: "已激活",
        classes: expect.arrayContaining(["tv-status--error"]),
      },
      硬停止: {
        value: "1 条",
        classes: expect.arrayContaining(["tv-status--error"]),
      },
    });
  });

  it("derives an overall risk posture badge from the safety state", async () => {
    const store = createRiskStore();
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    const posture = () => wrapper.get('[data-testid="risk-posture"]');
    expect(posture().text()).toContain("正常");

    store.realTradeRiskState.value = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: false,
    };
    await flushPromises();
    expect(posture().text()).toContain("限额未配置");

    store.realTradeHardStops.value = {
      ...emptyRealTradeHardStops,
      entries: [
        {
          id: "hard-stop-1",
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: null,
          symbol: null,
          operatorId: "ops-a",
          reason: "manual freeze",
          activatedAt: "2026-07-04T00:00:00.000Z",
          updatedAt: "2026-07-04T00:00:00.000Z",
        },
      ],
    };
    await flushPromises();
    expect(posture().text()).toContain("部分阻断");

    store.realTradeKillSwitchState.value = {
      ...emptyRealTradeKillSwitchState,
      killSwitchActive: true,
    };
    await flushPromises();
    expect(posture().text()).toContain("熔断中");
    expect(posture().classes()).toEqual(
      expect.arrayContaining(["tv-status--error"]),
    );
  });

  it("uses terminal-style native controls for the hard-stop form", async () => {
    testState.store = createRiskStore();
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    const inputs = [
      "硬停止账户 ID",
      "硬停止市场",
      "硬停止原因",
    ].map((label) => wrapper.get(`input[aria-label="${label}"]`));

    for (const input of inputs) {
      expect(input.classes()).toEqual(expect.arrayContaining(["tv-input"]));
    }

    expect(wrapper.get('select[aria-label="硬停止范围"]').classes()).toEqual(
      expect.arrayContaining(["tv-select"]),
    );
    expect(wrapper.get('[data-testid="hardstop-symbol-search"]').exists()).toBe(
      true,
    );
  });

  it("fills market, symbol, and scope from the instrument search selector", async () => {
    testState.store = createRiskStore();
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    wrapper.findComponent(InstrumentSearchBox).vm.$emit("select", {
      market: "US",
      resolvedMarket: "US",
      instrumentId: "US.AAPL",
      code: "AAPL",
      symbol: "AAPL",
      name: "Apple Inc.",
      securityType: "STK",
      lotSize: 1,
      source: "test",
      isWatched: false,
      selectable: true,
      unavailableReason: null,
    });
    await flushPromises();

    expect(
      (wrapper.get('input[aria-label="硬停止市场"]').element as HTMLInputElement)
        .value,
    ).toBe("US");
    expect(
      (
        wrapper.get('select[aria-label="硬停止范围"]')
          .element as HTMLSelectElement
      ).value,
    ).toBe("SYMBOL");
    expect(
      (
        wrapper.get('[data-testid="hardstop-symbol-input"]')
          .element as HTMLInputElement
      ).value,
    ).toBe("US.AAPL");
  });

  it("prefills the hard-stop form only from a selected real account", async () => {
    const store = createRiskStore();
    store.selectedBrokerAccount.value = {
      brokerId: "futu",
      accountId: "SIM-9",
      displayName: "Margin Account",
      tradingEnvironment: "SIMULATE",
      market: "HK",
    };
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    expect(
      (wrapper.get('input[aria-label="硬停止账户 ID"]').element as HTMLInputElement)
        .value,
    ).toBe("");
    expect(wrapper.text()).not.toContain("SIM-9");
    expect(wrapper.text()).toContain("预填账户全部账户");

    store.selectedBrokerAccount.value = {
      brokerId: "futu",
      accountId: "REAL-9",
      displayName: "Real Margin Account",
      tradingEnvironment: "REAL",
      market: "HK",
    };
    await flushPromises();

    expect(
      (wrapper.get('input[aria-label="硬停止账户 ID"]').element as HTMLInputElement)
        .value,
    ).toBe("REAL-9");
    expect(wrapper.text()).toContain("预填账户REAL-9");
  });

  it("edits runtime risk config behind a confirmation dialog", async () => {
    testState.store = createRiskStore();
    testState.fetchEnvelopeMock.mockResolvedValue([]);
    testState.fetchEnvelopeWithInitMock.mockResolvedValue({
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
    });

    const wrapper = mountRiskPage();
    await flushPromises();

    await switchToTab(wrapper, "运行时限额");

    expect(wrapper.text()).toContain("实盘总闸与单笔限额");
    expect(wrapper.findAll(".risk-sidebar__row")).toHaveLength(4);
    expect(wrapper.text()).not.toContain("现在的实盘状态");
    expect(wrapper.text()).not.toContain("JFTRADE");
    expect(wrapper.text()).not.toContain("环境变量");
    expect(wrapper.text()).not.toContain("启动配置");

    await wrapper.get('input[aria-label="允许实盘下单"]').setValue(true);
    await wrapper.get('input[aria-label="单笔最大数量"]').setValue("10");
    await wrapper.get('input[aria-label="原因"]').setValue("session open");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "保存运行时配置")!
      .trigger("click");

    // 开放实盘属于危险操作：先弹确认框，确认后才写入。
    expect(wrapper.text()).toContain("本次变更会开放实盘交易或放宽单笔限额");
    expect(testState.fetchEnvelopeWithInitMock).not.toHaveBeenCalled();
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    await flushPromises();

    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-risk-limits",
      expect.objectContaining({ method: "PUT" }),
    );
    const putInit = testState.fetchEnvelopeWithInitMock.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(String(putInit.body))).toEqual(
      expect.objectContaining({
        realTradingEnabled: true,
        maxOrderQuantity: 10,
        operatorId: "local",
        reason: "session open",
      }),
    );

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "关闭实盘配置")!
      .trigger("click");
    await flushPromises();
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-risk-limits",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("saves tighter limits directly and reports a disable failure", async () => {
    const store = createRiskStore({
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
      runtimeRiskConfigured: true,
      runtimeConfiguredMaxOrderQuantity: 20,
      runtimeConfiguredMaxOrderNotional: 4000,
      effectiveMaxOrderQuantity: 20,
      effectiveMaxOrderNotional: 4000,
    });
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);
    testState.fetchEnvelopeWithInitMock.mockResolvedValue({});

    const wrapper = mountRiskPage();
    await flushPromises();

    await (
      wrapper.vm as unknown as {
        saveRuntimeRisk: (payload: {
          realTradingEnabled: boolean;
          maxOrderQuantity: number | null;
          maxOrderNotional: number | null;
          operatorId: string;
          reason: string;
        }) => Promise<void>;
      }
    ).saveRuntimeRisk({
      realTradingEnabled: true,
      maxOrderQuantity: 10,
      maxOrderNotional: 2000,
      operatorId: "risk-ops",
      reason: "tighten limits",
    });
    await flushPromises();

    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-risk-limits",
      expect.objectContaining({ method: "PUT" }),
    );

    testState.fetchEnvelopeWithInitMock.mockRejectedValueOnce("offline");
    await switchToTab(wrapper, "运行时限额");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "关闭实盘配置")!
      .trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("关闭运行时风控配置失败");
  });

  it("shows persisted values beside controls and marks only semantic changes", async () => {
    const persistedRiskState = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: false,
      riskEnabled: true,
      runtimeRiskConfigured: true,
      runtimeConfiguredMaxOrderQuantity: 10,
      runtimeConfiguredMaxOrderNotional: 1000,
      effectiveMaxOrderQuantity: 10,
      effectiveMaxOrderNotional: 1000,
    };
    const store = createRiskStore(persistedRiskState);
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    await switchToTab(wrapper, "运行时限额");

    const enabledStatus = () =>
      wrapper.get('[data-status-for="real-trading-enabled"]');
    const quantityStatus = () =>
      wrapper.get('[data-status-for="max-order-quantity"]');
    const notionalStatus = () =>
      wrapper.get('[data-status-for="max-order-notional"]');

    expect(enabledStatus().text()).toBe("当前：关闭");
    expect(quantityStatus().text()).toBe("当前：10");
    expect(notionalStatus().text()).toBe("当前：1000");

    const enabledInput = wrapper.get('input[aria-label="允许实盘下单"]');
    await enabledInput.setValue(true);
    expect(enabledStatus().text()).toContain("已修改");
    await enabledInput.setValue(false);
    expect(enabledStatus().text()).not.toContain("已修改");

    const quantityInput = wrapper.get('input[aria-label="单笔最大数量"]');
    await quantityInput.setValue("10.0");
    expect(quantityStatus().text()).not.toContain("已修改");
    await quantityInput.setValue("12");
    expect(quantityStatus().text()).toContain("已修改");
    await quantityInput.setValue("invalid");
    expect(quantityStatus().text()).toContain("已修改");
    expect(wrapper.text()).toContain("单笔最大数量需要是正数。");
    await quantityInput.setValue("10");
    expect(quantityStatus().text()).not.toContain("已修改");

    const notionalInput = wrapper.get('input[aria-label="单笔最大金额"]');
    await notionalInput.setValue("2000");
    expect(notionalStatus().text()).toContain("已修改");

    store.realTradeRiskState.value = {
      ...persistedRiskState,
      runtimeConfiguredMaxOrderNotional: 2000,
      effectiveMaxOrderNotional: 2000,
    };
    await flushPromises();

    expect(notionalStatus().text()).toBe("当前：2000");
  });
});
