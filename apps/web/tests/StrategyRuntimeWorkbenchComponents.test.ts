// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { defineComponent } from "vue";

import RuntimeWorkbenchAlert from "../src/components/strategy-runtime/RuntimeWorkbenchAlert.vue";
import StrategyRuntimeBindingTab from "../src/components/strategy-runtime/StrategyRuntimeBindingTab.vue";
import StrategyRuntimeEmptyWorkbench from "../src/components/strategy-runtime/StrategyRuntimeEmptyWorkbench.vue";
import StrategyRuntimePanelHeader from "../src/components/strategy-runtime/StrategyRuntimePanelHeader.vue";
import StrategyRuntimeRunTab from "../src/components/strategy-runtime/StrategyRuntimeRunTab.vue";
import StrategyRuntimeWorkbenchHeader from "../src/components/strategy-runtime/StrategyRuntimeWorkbenchHeader.vue";
import StrategyRuntimeWorkbenchShell from "../src/components/strategy-runtime/StrategyRuntimeWorkbenchShell.vue";

describe("Strategy runtime workbench components", () => {
  it("renders dismissible alerts with tone semantics and close events", async () => {
    const wrapper = mount(RuntimeWorkbenchAlert, {
      props: {
        tone: "warning",
        closeLabel: "关闭最近异常",
        closeTestId: "runtime-alert-close",
        role: "alert",
      },
      slots: {
        default: "broker rejected order",
      },
    });

    expect(wrapper.classes()).toContain("runtime-workbench-alert--warning");
    expect(wrapper.attributes("role")).toBe("alert");
    expect(wrapper.text()).toContain("broker rejected order");

    await wrapper.get('[data-testid="runtime-alert-close"]').trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
  });

  it("renders page stats, shell slots, and empty workbench state", async () => {
    const header = mount(StrategyRuntimePanelHeader, {
      props: {
        activeStrategyCount: 2,
        definitionsCount: 5,
        defaultTradingEnvironment: "SIMULATE",
        runtimeRealTradingLabel: "已关闭",
        isKillSwitchActive: true,
        runtimeKillSwitchLabel: "已启用",
        runtimeRiskSummary: "观察 / 仅平仓",
      },
    });

    expect(header.text()).toContain("策略运行");
    expect(header.text()).toContain("活跃实例");
    expect(header.text()).toContain("2");
    expect(header.text()).toContain("观察 / 仅平仓");
    expect(header.find(".runtime-workbench-text-danger").exists()).toBe(true);

    const shell = mount(StrategyRuntimeWorkbenchShell, {
      props: {
        runtimePaneSizes: [30, 70],
      },
      slots: {
        messages: "<div data-testid='messages'>notice</div>",
        list: "<div data-testid='list'>instances</div>",
        detail: "<div data-testid='detail'>workbench</div>",
      },
      global: {
        stubs: {
          SplitPane: defineComponent({
            emits: ["resized"],
            template:
              "<div><button data-testid='resize' @click=\"$emit('resized', { panes: [{ size: 34 }, { size: 66 }] })\">resize</button><slot /></div>",
          }),
          SplitPaneItem: defineComponent({
            props: ["size"],
            template: "<section class='split-item'><slot /></section>",
          }),
        },
      },
    });

    expect(shell.get('[data-testid="messages"]').text()).toBe("notice");
    expect(shell.get('[data-testid="list"]').text()).toBe("instances");
    expect(shell.get('[data-testid="detail"]').text()).toBe("workbench");
    await shell.get('[data-testid="resize"]').trigger("click");
    expect(shell.emitted("resized")?.[0]?.[0]).toMatchObject({
      panes: [{ size: 34 }, { size: 66 }],
    });

    const empty = mount(StrategyRuntimeEmptyWorkbench);
    expect(empty.get('[data-testid="strategy-runtime-empty-workbench"]').text()).toContain(
      "从左侧选择策略实例",
    );
  });

  it("switches the runtime shell to non-resizable compact and mobile sections", async () => {
    const compact = mount(StrategyRuntimeWorkbenchShell, {
      props: {
        runtimePaneSizes: [30, 70],
        layout: "compact",
        hasSelectedDetail: true,
      },
      slots: {
        messages: "<div data-testid='messages'>notice</div>",
        list: "<div data-testid='list'>instances</div>",
        detail: "<div data-testid='detail'>workbench</div>",
      },
      global: {
        stubs: {
          SplitPane: defineComponent({
            emits: ["resized"],
            template:
              "<div><button data-testid='resize' @click=\"$emit('resized', { panes: [{ size: 34 }, { size: 66 }] })\">resize</button><slot /></div>",
          }),
          SplitPaneItem: defineComponent({
            props: ["size"],
            template: "<section class='split-item'><slot /></section>",
          }),
        },
      },
    });

    expect(compact.find(".runtime-panel__split").exists()).toBe(false);
    expect(compact.get(".runtime-panel__compact-stack").exists()).toBe(true);
    expect(compact.get('[data-testid="list"]').text()).toBe("instances");
    expect(compact.get('[data-testid="detail"]').text()).toBe("workbench");
    expect(compact.find('[data-testid="resize"]').exists()).toBe(false);
    expect(compact.emitted("resized")).toBeUndefined();

    const mobile = mount(StrategyRuntimeWorkbenchShell, {
      props: {
        runtimePaneSizes: [30, 70],
        layout: "mobile",
        mobileSection: "instances",
        hasSelectedDetail: true,
      },
      slots: {
        list: "<div data-testid='mobile-list'>instances</div>",
        detail: "<div data-testid='mobile-detail'>workbench</div>",
      },
    });

    expect(mobile.get('[data-testid="strategy-runtime-mobile-section-instances"]').classes()).toContain("is-active");
    await mobile.get('[data-testid="strategy-runtime-mobile-section-workbench"]').trigger("click");
    expect(mobile.emitted("update:mobile-section")).toEqual([["workbench"]]);
  });

  it("emits selected workbench header actions without owning business state", async () => {
    const wrapper = mount(StrategyRuntimeWorkbenchHeader, {
      props: {
        selectedStrategy: buildStrategy(),
        selectedRuntimeStatus: "RUNNING",
        selectedRuntimeStatusLabel: "运行中",
        selectedStrategyRuntimeLabel: "PineTS worker",
        isRefreshingStrategyContent: false,
        canStartSelectedStrategy: true,
        canPauseSelectedStrategy: true,
        canStopSelectedStrategy: true,
        formatStrategySymbols: () => "HK.00700",
        formatStrategyInterval: () => "5m",
      },
    });

    expect(wrapper.text()).toContain("Mean Revert");
    expect(wrapper.text()).toContain("HK.00700");
    expect(wrapper.get('[data-testid="strategy-start"]').attributes("disabled")).toBeUndefined();

    await wrapper.get('[data-testid="strategy-refresh-content"]').trigger("click");
    await wrapper.get('[data-testid="strategy-start"]').trigger("click");
    await wrapper.get('[data-testid="strategy-pause"]').trigger("click");
    await wrapper.get('[data-testid="strategy-stop"]').trigger("click");

    expect(wrapper.emitted("refresh-content")).toHaveLength(1);
    expect(wrapper.emitted("change-status")).toEqual([["start"], ["pause"], ["stop"]]);
  });

  it("renders runtime tab observations and locally dismisses runtime alerts", async () => {
    const wrapper = mount(StrategyRuntimeRunTab, {
      props: buildRunTabProps(),
    });

    expect(wrapper.get('[data-testid="strategy-runtime-start-hint"]').text()).toContain("可启动");
    expect(wrapper.get('[data-testid="strategy-runtime-observation"]').text()).toContain("HK.00700");
    expect(wrapper.text()).toContain("broker rejected order");
    expect(wrapper.text()).toContain("运行日志读取失败");

    await wrapper.get('[data-testid="strategy-runtime-last-error-close"]').trigger("click");
    expect(wrapper.text()).not.toContain("broker rejected order");

    await wrapper.get('[data-testid="strategy-runtime-details-error-close"]').trigger("click");
    expect(wrapper.text()).not.toContain("运行日志读取失败");

    await wrapper.setProps({
      detailsError: "",
      selectedStrategyRuntimeObservation: {
        ...buildRuntimeObservation(),
        lastError: "",
      },
    });
    await wrapper.setProps({
      detailsError: "运行日志再次失败",
      selectedStrategyRuntimeObservation: {
        ...buildRuntimeObservation(),
        lastError: "broker rejected order again",
        lastErrorAt: "2026-07-03T03:00:00.000Z",
      },
    });

    expect(wrapper.text()).toContain("运行日志再次失败");
    expect(wrapper.text()).toContain("broker rejected order again");
  });

  it("renders binding sync states and emits edit, refresh, and risk controls", async () => {
    const wrapper = mount(StrategyRuntimeBindingTab, {
      props: buildBindingTabProps(),
    });

    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').classes()).toContain(
      "runtime-workbench-sync-badge--stale",
    );
    expect(wrapper.text()).toContain("待刷新 v0.1.0 -> v0.2.0");
    expect(wrapper.text()).toContain("当前绑定摘要");
    expect(wrapper.text()).toContain("富途模拟账户");

    await wrapper.get('[data-testid="strategy-refresh-definition"]').trigger("click");
    await wrapper.get('[data-testid="strategy-runtime-risk-quick-mode"]').setValue("monitor");
    await wrapper.get('[data-testid="strategy-runtime-risk-quick-close-only"]').setValue(true);
    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");

    expect(wrapper.emitted("refresh-definition")).toHaveLength(1);
    expect(wrapper.emitted("update-runtime-risk")).toEqual([
      [{ mode: "monitor" }],
      [{ closeOnly: true }],
    ]);
    expect(wrapper.emitted("open-edit")).toHaveLength(1);

    await wrapper.setProps({
      selectedStrategyDefinitionSync: {
        isLatest: true,
        appliedVersion: "0.2.0",
        latestVersion: "0.2.0",
        canApplyLatest: false,
        blockedReason: "",
      },
    });

    expect(wrapper.get('[data-testid="strategy-definition-sync-badge"]').classes()).toContain(
      "runtime-workbench-sync-badge--latest",
    );
  });
});

function buildStrategy(patch: Record<string, unknown> = {}) {
  return {
    id: "instance-1",
    definition: {
      strategyId: "mean-revert",
      name: "Mean Revert",
      version: "0.1.0",
    },
    status: "STOPPED",
    startable: true,
    createdAt: "2026-07-03T01:00:00.000Z",
    runtime: "pinets-worker",
    sourceFormat: "pine-v6",
    params: {},
    logs: [],
    ...patch,
  } as never;
}

function buildBinding() {
  return {
    instruments: [{ market: "HK", code: "00700" }],
    symbols: ["HK.00700"],
    interval: "5m",
    executionMode: "live",
    brokerAccount: {
      brokerId: "futu",
      accountId: "SIM-1",
      tradingEnvironment: "SIMULATE",
      market: "HK",
    },
    runtimeRisk: {
      mode: "off",
      closeOnly: false,
      pauseOnReject: false,
      maxOrderQuantity: null,
      maxOrderNotional: null,
      dailyMaxOrders: null,
    },
  } as never;
}

function buildRuntimeObservation() {
  return {
    actualStatus: "RUNNING",
    activeSymbols: ["HK.00700"],
    lastClosedKlineAt: "2026-07-03T01:00:00.000Z",
    lastSignalAt: "2026-07-03T01:05:00.000Z",
    lastOrderAt: "2026-07-03T01:06:00.000Z",
    lastErrorAt: "2026-07-03T02:00:00.000Z",
    lastError: "broker rejected order",
    updatedAt: "2026-07-03T02:05:00.000Z",
  } as never;
}

function buildRunTabProps() {
  return {
    selectedStrategy: buildStrategy(),
    selectedStrategyBinding: buildBinding(),
    selectedStrategyRuntimeObservation: buildRuntimeObservation(),
    selectedStrategyRuntimeLabel: "PineTS worker",
    selectedStrategySourceFormatLabel: "Pine v6",
    selectedStrategyStartHint: "当前实例可启动。",
    selectedStrategyCompiledSummary: "已完成 Pine v6 主路径编译规划。",
    detailsError: "运行日志读取失败",
    formatStrategyEligibility: () => "可启动",
    formatStrategyExecutionMode: () => "确认执行",
    formatStrategyStatus: (status: string) => status,
    formatRuntimeObservationSymbols: (symbols: string[] | null | undefined) => symbols?.join(", ") ?? "暂无",
    formatTimestamp: (value: unknown) => String(value ?? "暂无"),
    formatTimestampTooltip: (value: unknown) => String(value ?? "暂无"),
  };
}

function buildBindingTabProps() {
  return {
    selectedStrategy: buildStrategy(),
    selectedStrategyBinding: buildBinding(),
    selectedStrategyDefinitionSync: {
      isLatest: false,
      appliedVersion: "0.1.0",
      latestVersion: "0.2.0",
      canApplyLatest: true,
      blockedReason: "",
    },
    isRefreshingStrategyDefinition: false,
    canRefreshSelectedStrategyDefinition: true,
    selectedStrategyDefinitionRefreshHint: "可刷新到最新设计 v0.2.0。",
    isUpdatingStrategyRuntimeRisk: false,
    formatStrategyDefinitionSyncSummary: () => "待刷新 v0.1.0 -> v0.2.0",
    formatStrategySymbols: () => "HK.00700",
    formatStrategyInterval: () => "5m",
    formatStrategyExecutionMode: () => "确认执行",
    formatStrategyRuntimeRiskSummary: () => "关闭",
    formatBrokerAccountSummary: () => "富途模拟账户",
    isCurrentBrokerAccountBinding: () => true,
  };
}
