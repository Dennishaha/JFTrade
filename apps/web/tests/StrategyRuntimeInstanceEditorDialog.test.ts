// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { defineComponent } from "vue";

import StrategyRuntimeInstanceEditorDialog from "../src/components/strategy-runtime/StrategyRuntimeInstanceEditorDialog.vue";

const dialogStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ["update:model-value"],
  template: "<div class='dialog-stub'><slot /></div>",
});

describe("StrategyRuntimeInstanceEditorDialog", () => {
  it("emits create-mode editing events and empty-definition recovery actions", async () => {
    const emptyWrapper = mount(StrategyRuntimeInstanceEditorDialog, {
      props: buildProps({
        mode: "create",
        strategyDefinitions: [],
      }),
      global: {
        stubs: {
          "v-dialog": dialogStub,
        },
      },
    });
    const emptyButtons = emptyWrapper.findAll("button");
    await emptyButtons.find((button) => button.text().includes("去设计区创建"))!.trigger("click");
    expect(emptyWrapper.emitted("switch-to-design")).toHaveLength(1);

    const wrapper = mount(StrategyRuntimeInstanceEditorDialog, {
      props: buildProps({
        mode: "create",
        filteredBrokerAccountOptions: [],
        isBrokerAccountPickerOpen: true,
        symbolValidationMessage: "请输入合法代码",
      }),
      global: {
        stubs: {
          "v-dialog": dialogStub,
        },
      },
    });

    expect(wrapper.get('[data-testid="strategy-instance-symbols-validation"]').text()).toContain("请输入合法代码");
    expect(wrapper.get('[data-testid="strategy-instance-account-search"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-instance-account-option-none"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("没有匹配的券商账号");

    await wrapper.get('[data-testid="strategy-create-instance-close"]').trigger("click");
    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("blur");
    await wrapper.get('[data-testid="strategy-instance-symbol-add"]').trigger("click");
    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("keydown", { key: "Enter" });
    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("paste");
    await wrapper.findComponent(dialogStub).vm.$emit("update:model-value", false);

    expect(wrapper.emitted("update:open")).toEqual([[false], [false]]);
    expect(wrapper.emitted("commit-symbol-draft")).toHaveLength(2);
    expect(wrapper.emitted("symbol-draft-keydown")).toHaveLength(1);
    expect(wrapper.emitted("symbol-draft-paste")).toHaveLength(1);
  });

  it("renders edit-mode helpers and emits binding, broker, and runtime-risk interactions", async () => {
    const wrapper = mount(StrategyRuntimeInstanceEditorDialog, {
      props: buildProps({
        mode: "edit",
        selectedStrategy: {
          id: "instance-1",
          definition: { name: "Mean Revert", version: "0.1.0" },
          status: "RUNNING",
        },
        symbolTags: ["HK.00700"],
        selectedBrokerAccountKey: "futu|REAL|ACC-1|HK",
        currentBrokerAccountSelectionKey: "futu|REAL|ACC-1|HK",
        selectedBrokerAccountOption: buildBrokerOption(),
        filteredBrokerAccountOptions: [buildBrokerOption()],
        isBrokerAccountPickerOpen: true,
        executionMode: "notify_only",
        runtimeRisk: {
          mode: "enforce",
          closeOnly: true,
          pauseOnReject: false,
          maxOrderQuantity: 12,
          maxOrderNotional: 25000,
          dailyMaxOrders: 9,
        },
      }),
      global: {
        stubs: {
          "v-dialog": dialogStub,
        },
      },
    });

    expect(wrapper.get('[data-testid="strategy-edit-account-current-tag"]').text()).toContain("当前");
    expect(wrapper.text()).toContain("仅通知模式会发送准备下单提示");
    expect(wrapper.text()).toContain("当前实例不是 STOPPED");

    await wrapper.get('[data-testid="strategy-edit-symbol-market"]').setValue("US");
    await wrapper.get('[data-testid="strategy-edit-symbols"]').setValue("AAPL");
    await wrapper.get('[data-testid="strategy-edit-symbols"]').trigger("paste");
    await wrapper.get('[data-testid="strategy-edit-symbols"]').trigger("blur");
    await wrapper.get('[data-testid="strategy-edit-symbols"]').trigger("keydown", { key: "Tab" });
    await wrapper.get('[data-testid="strategy-edit-symbol-add"]').trigger("click");
    await wrapper.get(".strategy-tag-chip").trigger("click");
    await wrapper.get('[data-testid="strategy-edit-interval"]').setValue("15m");
    await wrapper.get('[data-testid="strategy-edit-execution-mode"]').setValue("live");
    await wrapper.get('[data-testid="strategy-edit-account"]').trigger("click");
    await wrapper.get('[data-testid="strategy-edit-account-search"]').setValue("REAL");
    await wrapper.get('[data-testid="strategy-edit-account-option-none"]').trigger("click");
    await wrapper.get('[data-testid="strategy-edit-account-option-ACC-1"]').trigger("click");
    await wrapper.get('[data-testid="strategy-runtime-risk-mode"]').setValue("monitor");
    await wrapper.get('[data-testid="strategy-runtime-risk-close-only"]').setValue(false);
    await wrapper.get('[data-testid="strategy-runtime-risk-pause-on-reject"]').setValue(true);
    await wrapper.get('[data-testid="strategy-runtime-risk-max-quantity"]').setValue("20");
    await wrapper.get('[data-testid="strategy-runtime-risk-max-notional"]').setValue("30000");
    await wrapper.get('[data-testid="strategy-runtime-risk-daily-max-orders"]').setValue("11");

    expect(wrapper.emitted("update:symbol-market")).toEqual([["US"]]);
    expect(wrapper.emitted("update:symbol-draft")).toEqual([["AAPL"]]);
    expect(wrapper.emitted("symbol-draft-paste")).toHaveLength(1);
    expect(wrapper.emitted("commit-symbol-draft")).toHaveLength(2);
    expect(wrapper.emitted("symbol-draft-keydown")).toHaveLength(1);
    expect(wrapper.emitted("remove-symbol")).toEqual([["HK.00700"]]);
    expect(wrapper.emitted("update:interval")).toEqual([["15m"]]);
    expect(wrapper.emitted("update:execution-mode")).toEqual([["live"]]);
    expect(wrapper.emitted("toggle-broker-picker")).toHaveLength(1);
    expect(wrapper.emitted("update:broker-query")).toEqual([["REAL"]]);
    expect(wrapper.emitted("clear-broker-selection")).toHaveLength(1);
    expect(wrapper.emitted("select-broker-selection")).toEqual([["futu|REAL|ACC-1|HK"]]);
    expect(wrapper.emitted("update:runtime-risk-mode")).toEqual([["monitor"]]);
    expect(wrapper.emitted("update:runtime-risk-close-only")).toEqual([[false]]);
    expect(wrapper.emitted("update:runtime-risk-pause-on-reject")).toEqual([[true]]);
    expect(wrapper.emitted("update:runtime-risk-number")).toEqual([
      ["maxOrderQuantity", "20"],
      ["maxOrderNotional", "30000"],
      ["dailyMaxOrders", "11"],
    ]);
  });

  it("surfaces live-execution compatibility warnings before creating an instance", () => {
    const wrapper = mount(StrategyRuntimeInstanceEditorDialog, {
      props: buildProps({
        mode: "create",
        executionMode: "live",
        createDefinition: {
          id: "percent-cancel",
          name: "Percent Cancel",
          version: "1.0.0",
          script: `
            strategy("Percent Cancel", default_qty_type=strategy.percent_of_equity)
            strategy.entry("L", strategy.long, qty_percent=10)
            strategy.cancel_all()
          `,
        },
      }),
      global: {
        stubs: {
          "v-dialog": dialogStub,
        },
      },
    });

    const warning = wrapper.get('[data-testid="strategy-live-compatibility-warning"]');
    expect(warning.text()).toContain("确认执行前检查");
    expect(warning.text()).not.toContain("QuantityPct");
    expect(warning.text()).not.toContain("cancel/cancel_all");
    expect(warning.text()).toContain("kill switch");
  });
});

function buildProps(overrides: Record<string, unknown> = {}) {
  return {
    open: true,
    mode: "create",
    title: "创建实例",
    hint: "把策略定义绑定到真实运行实例。",
    isLoadingDefinitions: false,
    definitionsError: "",
    strategyDefinitions: [
      {
        id: "mean-revert",
        name: "Mean Revert",
        version: "0.1.0",
      },
    ],
    createDefinitionId: "mean-revert",
    createDefinition: {
      id: "mean-revert",
      name: "Mean Revert",
      version: "0.1.0",
    },
    selectedStrategy: null,
    symbolTags: [],
    symbolMarket: "HK",
    symbolDraft: "",
    symbolValidationMessage: "",
    marketOptions: [
      { value: "HK", title: "港股" },
      { value: "US", title: "美股" },
    ],
    intervalValue: "5m",
    executionMode: "live",
    runtimeRisk: {
      mode: "off",
      closeOnly: false,
      pauseOnReject: false,
      maxOrderQuantity: null,
      maxOrderNotional: null,
      dailyMaxOrders: null,
    },
    selectedBrokerAccountOption: null,
    selectedBrokerAccountKey: "",
    currentBrokerAccountSelectionKey: "",
    isBrokerAccountPickerOpen: false,
    brokerAccountQuery: "",
    filteredBrokerAccountOptions: [],
    previewDefinitionLabel: "Mean Revert / v0.1.0",
    symbolsSummary: "未绑定交易代码",
    brokerAccountSummary: "暂不绑定账号",
    canCreateStrategyInstance: true,
    canUpdateSelectedStrategyBinding: true,
    canDeleteSelectedStrategy: true,
    isCreatingStrategyInstance: false,
    isUpdatingStrategyBinding: false,
    isDeletingStrategy: false,
    ...overrides,
  };
}

function buildBrokerOption() {
  return {
    selectionKey: "futu|REAL|ACC-1|HK",
    source: "managed" as const,
    brokerId: "futu",
    accountId: "ACC-1",
    displayName: "主交易账号",
    tradingEnvironment: "REAL",
    market: "HK",
    securityFirm: null,
  };
}
