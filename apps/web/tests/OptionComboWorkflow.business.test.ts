// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const api = vi.hoisted(() => ({ fetchWithInit: vi.fn() }));
const consoleState = {
  selectedBrokerAccount: ref({
    brokerId: "futu",
    accountId: "option-acc",
    displayName: "保证金账户",
    tradingEnvironment: "SIMULATE",
  }),
  systemStatus: ref({
    defaultBroker: "futu",
    defaultTradingEnvironment: "SIMULATE",
  }),
  realTradeApprovals: ref({
    requiredConfirmationText: "ENABLE_REAL_TRADING",
  }),
};

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: api.fetchWithInit,
}));
vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleState,
}));

import OptionChainTable from "../src/components/product/OptionChainTable.vue";
import OptionComboBuilder from "../src/components/product/OptionComboBuilder.vue";
import OptionComboConfirmDialog from "../src/components/product/OptionComboConfirmDialog.vue";
import OptionComboLegEditor from "../src/components/product/OptionComboLegEditor.vue";
import type {
  OptionComboDraftStore,
  OptionContractChoice,
} from "../src/composables/optionComboDraft";
import type { OptionChainRowModel } from "../src/composables/optionChainModel";
import { setupState } from "./productTestUtils";

function choice(
  code: string,
  strike: number,
  bidPrice: number,
  askPrice: number,
): OptionContractChoice {
  return {
    instrumentId: `US.${code}`,
    code,
    name: code,
    label: `07/24 Call ${strike}`,
    optionType: "call",
    strike,
    multiplier: 100,
    expiry: "2026-07-24",
    bidPrice,
    askPrice,
  };
}

const contracts = [
  choice("BABA-C100", 100, 6.2, 6.4),
  choice("BABA-C110", 110, 2.1, 2.3),
  choice("BABA-C120", 120, 0.6, 0.8),
];

beforeEach(() => {
  api.fetchWithInit.mockReset();
  consoleState.selectedBrokerAccount.value = {
    brokerId: "futu",
    accountId: "option-acc",
    displayName: "保证金账户",
    tradingEnvironment: "SIMULATE",
  };
});

describe("option combo workflow", () => {
  it("edits legs with compact controls and enforces the real confirmation phrase", async () => {
    const editor = mount(OptionComboLegEditor, {
      props: {
        contracts,
        legs: [{ ...contracts[0]!, side: "BUY", ratio: 2 }],
      },
    });
    setupState<{ addSelected: () => void }>(editor).addSelected();
    await editor
      .get('select[aria-label="搜索并选择期权合约"]')
      .setValue("US.BABA-C110");
    await editor
      .findAll("button")
      .find((button) => button.text().trim() === "添加")!
      .trigger("click");
    expect(editor.emitted("add")?.[0]).toEqual(["US.BABA-C110"]);
    await editor.get('button[aria-label*="当前买入"]').trigger("click");
    expect(editor.emitted("update")?.[0]).toEqual([
      "US.BABA-C100",
      { side: "SELL" },
    ]);
    await editor.get('button[aria-label="减少比例"]').trigger("click");
    await editor.get('button[aria-label="增加比例"]').trigger("click");
    await editor.get('button[aria-label="移除 BABA-C100"]').trigger("click");
    expect(editor.emitted("update")?.slice(1)).toEqual([
      ["US.BABA-C100", { ratio: 1 }],
      ["US.BABA-C100", { ratio: 3 }],
    ]);
    expect(editor.emitted("remove")?.[0]).toEqual(["US.BABA-C100"]);

    const dialog = mount(OptionComboConfirmDialog, {
      props: {
        open: true,
        mode: "place",
        accountLabel: "",
        environment: "REAL",
        strategyLabel: "垂直价差",
        legs: [{ ...contracts[0]!, side: "BUY", ratio: 1 }],
        price: 4.1,
        quantity: 1,
        realConfirmationRequired: true,
        requiredConfirmationText: "ENABLE_REAL_TRADING",
      },
    });
    expect(dialog.text()).toContain("默认账户");
    const confirmButton = dialog
      .findAll("button")
      .find((button) => button.text() === "确认提交")!;
    expect(confirmButton.attributes("disabled")).toBeDefined();
    await dialog.get("input").setValue("ENABLE_REAL_TRADING");
    expect(confirmButton.attributes("disabled")).toBeUndefined();
    await confirmButton.trigger("click");
    expect(dialog.emitted("confirm")).toHaveLength(1);
    await dialog.get('button[aria-label="关闭确认弹窗"]').trigger("click");
    await dialog.get(".combo-confirm").trigger("click");
    expect(dialog.emitted("close")).toHaveLength(2);
    await dialog.setProps({ open: false });
    await dialog.setProps({ open: true, mode: "cancel" });
    await dialog
      .findAll("button")
      .find((button) => button.text() === "返回检查")!
      .trigger("click");
    expect(dialog.emitted("close")).toHaveLength(3);
  });

  it("selects BUY from ask and SELL from bid without conflicting with analysis click", async () => {
    const side = {
      code: "BABA-C110",
      instrumentId: "US.BABA-C110",
      name: "BABA Call 110",
      bidPrice: 2.1,
      askPrice: 2.3,
      impliedVolatility: 0.4,
      delta: 0.5,
      gamma: 0.1,
      theta: -0.02,
      vega: 0.2,
      moneyness: "ATM" as const,
      multiplier: 100,
    };
    const put = {
      ...side,
      code: "BABA-P110",
      instrumentId: "US.BABA-P110",
      name: "BABA Put 110",
    };
    const rows: OptionChainRowModel[] = [
      { key: "110-0", strike: 110, isAtm: true, call: side, put },
    ];
    const wrapper = mount(OptionChainTable, {
      props: {
        rows,
        underlyingInstrumentId: "US.BABA",
        underlyingPrice: 110,
        selectedLegs: [
          {
            ...contracts[0]!,
            instrumentId: "US.BABA-P110",
            code: "BABA-P110",
            side: "BUY",
            ratio: 1,
          },
        ],
      },
    });
    await wrapper.get('button[aria-label*="买价，加入卖出腿"]').trigger("click");
    expect(wrapper.emitted("selectLeg")?.[0]).toEqual([side, "SELL"]);
    await wrapper.get('button[aria-label*="卖价，加入买入腿"]').trigger("click");
    expect(wrapper.emitted("selectLeg")?.[1]).toEqual([side, "BUY"]);
    await wrapper.get('button[title="BABA Call 110"]').trigger("click");
    expect(wrapper.emitted("openContract")?.[0]).toEqual([side]);
    await wrapper.get('button[aria-label*="BABA-P110 买价"]').trigger("click");
    await wrapper.get('button[aria-label*="BABA-P110 卖价"]').trigger("click");
    expect(wrapper.emitted("selectLeg")?.slice(2)).toEqual([
      [put, "SELL"],
      [put, "BUY"],
    ]);
  });

  it("previews account impact, confirms once, and submits locked option legs", async () => {
    api.fetchWithInit.mockImplementation(async (path: string) => {
      if (path.endsWith("/previews")) {
        return {
          previewId: "preview-baba",
          expiresAt: new Date(Date.now() + 60_000).toISOString(),
          buyingPowerImpact: 480,
          accountImpact: {
            initialMarginChange: 320,
            maintenanceMarginChange: 260,
            optionBuyingPower: 8_000,
            maxWithdrawalChange: -480,
            buyingPowerDecrease: 480,
          },
          warnings: ["组合报价可能变化"],
          optionAnalysis: {
            maxProfitUnlimited: true,
            maxLoss: 100,
            breakevenPoints: [104.8],
            probability: 0.61,
            delta: 0.22,
            theta: -0.03,
          },
        };
      }
      return {
        accepted: true,
        internalOrderId: "combo-baba-1",
        brokerOrderId: "broker-combo-1",
        orderStatus: "BROKER_ACCEPTED",
      };
    });
    vi.stubGlobal("crypto", { randomUUID: () => "workflow-id" });
    const wrapper = mount(OptionComboBuilder, {
      props: {
        instrumentId: "US.BABA",
        market: "US",
        contracts,
        underlyingPrice: 110,
      },
    });
    const state = setupState<{
      draft: OptionComboDraftStore;
      previewCombo: () => Promise<void>;
    }>(wrapper);
    state.draft.replaceLegs([
      { ...contracts[0]!, side: "BUY", ratio: 1 },
      { ...contracts[1]!, side: "SELL", ratio: 1 },
    ]);
    await nextTick();
    expect(wrapper.find(".option-combo-ticket__tools").exists()).toBe(true);
    expect(wrapper.find(".combo-leg-editor__columns").exists()).toBe(true);
    expect(wrapper.get('input[aria-label="组合限价"]').element).toHaveProperty(
      "value",
      "4.1",
    );
    await state.previewCombo();
    await nextTick();
    expect(wrapper.text()).toContain("初始保证金 320");
    expect(wrapper.text()).toContain("最大盈利 无限");
    expect(wrapper.text()).toContain("组合报价可能变化");

    await wrapper.get("button.is-primary").trigger("click");
    expect(wrapper.text()).toContain("确认组合期权下单");
    const confirm = wrapper
      .findAll(".combo-confirm button")
      .find((button) => button.text() === "确认提交");
    await confirm!.trigger("click");
    await nextTick();
    expect(api.fetchWithInit).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("BROKER_ACCEPTED");

    const placeCall = api.fetchWithInit.mock.calls[1]!;
    const payload = JSON.parse(String(placeCall[1].body));
    expect(payload).toMatchObject({
      clientOrderId: "jftrade-option-combo-workflow-id",
      previewId: "preview-baba",
      optionStrategy: "vertical",
      price: 4.1,
    });
    expect(payload.legs).toEqual([
      expect.objectContaining({
        instrumentId: "US.BABA-C100",
        side: "BUY",
        ratio: 1,
      }),
      expect.objectContaining({
        instrumentId: "US.BABA-C110",
        side: "SELL",
        ratio: 1,
      }),
    ]);
    await wrapper.find("button.is-primary").trigger("click");
    expect(api.fetchWithInit).toHaveBeenCalledTimes(2);
    await wrapper
      .findAll(".option-combo-ticket__execution button")
      .find((button) => button.text() === "撤单")!
      .trigger("click");
    await wrapper.get('button[aria-label="关闭确认弹窗"]').trigger("click");
  });

  it("invalidates an existing preview when quantity or account changes", async () => {
    api.fetchWithInit.mockResolvedValue({
      previewId: "preview-1",
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });
    const wrapper = mount(OptionComboBuilder, {
      props: { instrumentId: "US.BABA", market: "US", contracts },
    });
    const state = setupState<{
      draft: OptionComboDraftStore;
      preview: { previewId: string } | null;
      previewCombo: () => Promise<void>;
    }>(wrapper);
    state.draft.replaceLegs([
      { ...contracts[0]!, side: "BUY", ratio: 1 },
      { ...contracts[1]!, side: "SELL", ratio: 1 },
    ]);
    await nextTick();
    await state.previewCombo();
    expect(state.preview?.previewId).toBe("preview-1");
    state.draft.quantity.value = 2;
    await nextTick();
    expect(state.preview).toBeNull();
    state.draft.quantity.value = 1;
    await nextTick();
    await state.previewCombo();
    expect(state.preview?.previewId).toBe("preview-1");
    consoleState.selectedBrokerAccount.value = {
      ...consoleState.selectedBrokerAccount.value,
      accountId: "another-account",
      displayName: "另一个账户",
    };
    await nextTick();
    expect(state.preview).toBeNull();
    expect(wrapper.text()).toContain("另一个账户");
  });

  it("supports template generation, manual leg search, anchors, history, and quantity controls", async () => {
    const wrapper = mount(OptionComboBuilder, {
      props: {
        instrumentId: "US.BABA",
        market: "US",
        contracts,
        defaultExpiry: "2026-07-24",
        underlyingPrice: 110,
      },
    });
    await wrapper.get('select[aria-label="组合策略模板"]').setValue("vertical");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "生成")!
      .trigger("click");
    expect(wrapper.text()).toContain("垂直价差");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "全部反向")!
      .trigger("click");
    await wrapper.get('button[aria-label="撤销选腿"]').trigger("click");
    await wrapper.get('button[aria-label="重做选腿"]').trigger("click");
    await wrapper
      .findAll(".option-combo-ticket__anchors button")[0]!
      .trigger("click");
    await wrapper
      .findAll(".option-combo-ticket__anchors button")[1]!
      .trigger("click");
    await wrapper
      .findAll(".option-combo-ticket__anchors button")[2]!
      .trigger("click");
    await wrapper.get('input[aria-label="组合限价"]').setValue("3.75");
    expect(
      (wrapper.get('input[aria-label="组合限价"]').element as HTMLInputElement)
        .value,
    ).toBe("3.75");
    await wrapper.get('button[aria-label="增加组合数量"]').trigger("click");
    await wrapper.get('input[aria-label="组合数量"]').setValue("3");
    await wrapper.get('button[aria-label="减少组合数量"]').trigger("click");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "清空")!
      .trigger("click");
    expect(wrapper.text()).toContain("自定义选腿");

    await wrapper
      .get('select[aria-label="搜索并选择期权合约"]')
      .setValue("US.BABA-C100");
    await wrapper
      .findAll(".combo-leg-editor__add button")
      .at(0)!
      .trigger("click");
    expect(wrapper.text()).toContain("BABA-C100");

    await wrapper.get('select[aria-label="组合策略模板"]').setValue("straddle");
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "生成")!
      .trigger("click");
    expect(wrapper.text()).toContain(
      "当前期权链没有足够的合约生成该策略",
    );
  });

  it("surfaces preview, place, and cancel failures without enabling duplicate submission", async () => {
    api.fetchWithInit
      .mockRejectedValueOnce(new Error("预检失败"))
      .mockResolvedValueOnce({
        previewId: "preview-retry",
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
      })
      .mockRejectedValueOnce("提交失败")
      .mockRejectedValueOnce(new Error("撤单失败"))
      .mockResolvedValueOnce({
        accepted: true,
        internalOrderId: "combo-retry",
        orderStatus: "CANCELLED",
      });
    const wrapper = mount(OptionComboBuilder, {
      props: { instrumentId: "US.BABA", market: "US", contracts },
    });
    const state = setupState<{
      draft: OptionComboDraftStore;
      preview: { previewId: string } | null;
      execution: {
        accepted: boolean;
        internalOrderId?: string;
        orderStatus?: string;
      } | null;
      previewCombo: () => Promise<void>;
      placeCombo: () => Promise<void>;
      cancelCombo: () => Promise<void>;
      remainingSeconds: number | null;
    }>(wrapper);
    expect(state.remainingSeconds).toBeNull();
    await state.placeCombo();
    await state.cancelCombo();
    await state.previewCombo();
    expect(api.fetchWithInit).not.toHaveBeenCalled();
    state.draft.replaceLegs([
      { ...contracts[0]!, side: "BUY", ratio: 1 },
      { ...contracts[1]!, side: "SELL", ratio: 1 },
    ]);
    await nextTick();
    await state.previewCombo();
    expect(wrapper.text()).toContain("预检失败");
    await state.previewCombo();
    expect(state.preview?.previewId).toBe("preview-retry");
    await state.placeCombo();
    expect(wrapper.text()).toContain("提交失败");
    state.execution = {
      accepted: true,
      internalOrderId: "combo-retry",
      orderStatus: "SUBMITTED",
    };
    await state.cancelCombo();
    expect(wrapper.text()).toContain("撤单失败");
    await state.cancelCombo();
    expect(state.execution?.orderStatus).toBe("CANCELLED");
    wrapper.unmount();
  });
});
