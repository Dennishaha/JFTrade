// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, ref } from "vue";

import {
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptySystemStatus,
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

const passthroughStub = defineComponent({
  template: "<section><slot /></section>",
});

const buttonStub = defineComponent({
  props: ["loading", "disabled", "variant", "color", "size"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' @click='$emit(\"click\", $event)'><slot /></button>",
});

const switchStub = defineComponent({
  props: ["modelValue", "label"],
  emits: ["update:modelValue"],
  template:
    "<label><input type='checkbox' :aria-label='label' :checked='modelValue' @change='$emit(\"update:modelValue\", $event.target.checked)' />{{ label }}</label>",
});

const textFieldStub = defineComponent({
  props: ["modelValue", "label"],
  emits: ["update:modelValue"],
  template:
    "<label><span>{{ label }}</span><input :aria-label='label' :value='modelValue ?? \"\"' @input='$emit(\"update:modelValue\", $event.target.value)' /></label>",
});

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
    systemStatus: ref(emptySystemStatus),
  };
}

function mountRiskPage() {
  const wrapper = mount(RiskPage, {
    global: {
      stubs: {
        "v-alert": passthroughStub,
        "v-btn": buttonStub,
        "v-card": passthroughStub,
        "v-card-text": passthroughStub,
        "v-chip": passthroughStub,
        "v-empty-state": defineComponent({
          props: ["text"],
          template: "<div>{{ text }}</div>",
        }),
        "v-switch": switchStub,
        "v-text-field": textFieldStub,
      },
    },
  });
  return wrapper;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("RiskPage", () => {
  it("colors header status cards by real-trade safety state", async () => {
    const store = createRiskStore();
    testState.store = store;
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    const headerTones = () =>
      Object.fromEntries(
        wrapper.findAll(".page-header-stat").map((stat) => [
          stat.text(),
          {
            tone: stat.attributes("data-tone"),
            classes: stat.classes(),
          },
        ]),
      );

    expect(headerTones()).toMatchObject({
      "实盘总闸未开放": {
        tone: "warn",
        classes: expect.arrayContaining(["page-header-stat--warn"]),
      },
      "单笔限额未配置": {
        tone: "warn",
        classes: expect.arrayContaining(["page-header-stat--warn"]),
      },
      "熔断未激活": {
        tone: "good",
        classes: expect.arrayContaining(["page-header-stat--good"]),
      },
      "硬停止0": {
        tone: "good",
        classes: expect.arrayContaining(["page-header-stat--good"]),
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

    expect(headerTones()).toMatchObject({
      "实盘总闸已开放": {
        tone: "good",
        classes: expect.arrayContaining(["page-header-stat--good"]),
      },
      "单笔限额已配置": {
        tone: "good",
        classes: expect.arrayContaining(["page-header-stat--good"]),
      },
      "熔断已激活": {
        tone: "danger",
        classes: expect.arrayContaining(["page-header-stat--danger"]),
      },
      "硬停止1": {
        tone: "danger",
        classes: expect.arrayContaining(["page-header-stat--danger"]),
      },
    });
  });

  it("uses theme-aware surfaces and text for hard-stop native controls", async () => {
    testState.store = createRiskStore();
    testState.fetchEnvelopeMock.mockResolvedValue([]);

    const wrapper = mountRiskPage();
    await flushPromises();

    const inputs = [
      "硬停止账户 ID",
      "硬停止市场",
      "硬停止标的",
      "硬停止原因",
    ].map((label) => wrapper.get(`input[aria-label="${label}"]`));

    for (const input of inputs) {
      expect(input.classes()).toEqual(
        expect.arrayContaining([
          "bg-white",
          "text-slate-900",
          "placeholder:text-slate-500",
        ]),
      );
    }

    expect(wrapper.get('select[aria-label="硬停止范围"]').classes()).toEqual(
      expect.arrayContaining(["bg-white", "text-slate-900"]),
    );
  });

  it("edits runtime risk config without exposing env launch controls", async () => {
    testState.store = createRiskStore();
    testState.fetchEnvelopeMock.mockResolvedValue([]);
    testState.fetchEnvelopeWithInitMock.mockResolvedValue({
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
    });
    const confirmMock = vi.fn(() => true);
    vi.stubGlobal("confirm", confirmMock);

    const wrapper = mountRiskPage();
    await flushPromises();

    expect(wrapper.text()).toContain("实盘风控");
    expect(wrapper.text()).toContain("实盘总闸与单笔限额");
    expect(wrapper.findAll(".page-header-stat")).toHaveLength(4);
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

    expect(confirmMock).toHaveBeenCalled();
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
    expect(testState.fetchEnvelopeWithInitMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-risk-limits",
      expect.objectContaining({ method: "DELETE" }),
    );
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
