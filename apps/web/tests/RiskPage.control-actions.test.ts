// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const riskMocks = vi.hoisted(() => ({
  apiPost: vi.fn(),
  apiPostPath: vi.fn(),
  disableRuntimeRiskConfig: vi.fn(),
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
  saveRuntimeRiskConfig: vi.fn(),
  store: null as null | Record<string, unknown>,
}));

vi.mock("../src/composables/apiClient", () => ({
  apiPost: (...args: unknown[]) => riskMocks.apiPost(...args),
  apiPostPath: (...args: unknown[]) => riskMocks.apiPostPath(...args),
  fetchEnvelope: (...args: unknown[]) => riskMocks.fetchEnvelope(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => riskMocks.fetchEnvelopeWithInit(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => riskMocks.store,
}));

vi.mock("../src/composables/useRuntimeRiskConfig", () => ({
  useRuntimeRiskConfig: () => ({
    disableRuntimeRiskConfig: riskMocks.disableRuntimeRiskConfig,
    saveRuntimeRiskConfig: riskMocks.saveRuntimeRiskConfig,
  }),
}));

import RiskPage from "../src/pages/RiskPage.vue";

const riskConfigStub = {
  emits: ["save", "disable"],
  template: `
    <div class="risk-config-stub">
      <button class="save-risk-config" @click="$emit('save', { realTradingEnabled: true, maxOrderQuantity: 10, maxOrderNotional: 2000, operatorId: 'ops-a', reason: 'open session' })">save</button>
      <button class="disable-risk-config" @click="$emit('disable', { operatorId: 'ops-a', reason: 'pause' })">disable</button>
    </div>
  `,
};

const emergencyStub = {
  emits: ["activate", "release"],
  template: `
    <div>
      <button class="activate-kill-switch" @click="$emit('activate')">activate</button>
      <button class="release-kill-switch" @click="$emit('release')">release</button>
    </div>
  `,
};

const hardStopStub = {
  emits: ["activate", "release"],
  template: `
    <div>
      <button class="activate-hard-stop" @click="$emit('activate', { accountId: 'ACC-1', reason: 'manual hold' })">hold</button>
      <button class="release-hard-stop" @click="$emit('release', 'stop/a')">release hold</button>
    </div>
  `,
};

const strategyRiskStub = {
  props: ["error"],
  emits: ["refresh", "updateMode"],
  template: `
    <div class="strategy-risk-stub">
      <span class="strategy-risk-error">{{ error }}</span>
      <button class="refresh-strategy-risk" @click="$emit('refresh')">refresh</button>
      <button class="update-strategy-risk" @click="$emit('updateMode', 'strategy/a', 'enforce')">enforce</button>
    </div>
  `,
};

const stubs = {
  HardStopControlPanel: hardStopStub,
  PageHeader: { template: "<header><slot /></header>" },
  RealTradeEmergencyPanel: emergencyStub,
  RiskEventTimeline: { template: "<div />" },
  RuntimeRiskConfigPanel: riskConfigStub,
  StrategyRuntimeRiskSection: strategyRiskStub,
  "v-alert": { template: "<p><slot /></p>" },
};

function createStore() {
  return {
    loadSystemState: vi.fn().mockResolvedValue(undefined),
    realTradeHardStops: ref({ entries: [] }),
    realTradeKillSwitchEvents: ref({ entries: [] }),
    realTradeKillSwitchState: ref({ killSwitchActive: false }),
    realTradeRiskEvents: ref({ entries: [] }),
    realTradeRiskState: ref({
      realTradingEnabled: false,
      riskEnabled: false,
      effectiveMaxOrderQuantity: 5,
      effectiveMaxOrderNotional: 1000,
    }),
    systemStatus: ref({}),
  };
}

function strategyInstance() {
  return {
    id: "strategy/a",
    definition: { id: "def-a", name: "均线策略" },
    binding: {
      runtimeRisk: {
        mode: "monitor",
        closeOnly: false,
        maxOrderQuantity: 5,
        maxOrderNotional: null,
        dailyMaxOrders: null,
        pauseOnReject: false,
      },
    },
  };
}

beforeEach(() => {
  riskMocks.store = createStore();
  riskMocks.apiPost.mockResolvedValue({});
  riskMocks.apiPostPath.mockResolvedValue({});
  riskMocks.disableRuntimeRiskConfig.mockResolvedValue(undefined);
  riskMocks.saveRuntimeRiskConfig.mockResolvedValue(undefined);
  riskMocks.fetchEnvelope.mockResolvedValue([strategyInstance()]);
  riskMocks.fetchEnvelopeWithInit.mockImplementation(async (path: string) => {
    if (path.includes("/runtime-risk")) {
      return {
        ...strategyInstance(),
        binding: { runtimeRisk: { ...strategyInstance().binding.runtimeRisk, mode: "enforce" } },
      };
    }
    return {};
  });
});

afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("RiskPage control actions", () => {
  it("executes confirmed runtime, emergency, hard-stop, and per-strategy controls", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const store = riskMocks.store as ReturnType<typeof createStore>;
    const wrapper = mount(RiskPage, { global: { stubs } });
    await flushPromises();

    await wrapper.find(".save-risk-config").trigger("click");
    await flushPromises();
    expect(riskMocks.saveRuntimeRiskConfig).toHaveBeenCalledWith(expect.objectContaining({
      realTradingEnabled: true,
      maxOrderQuantity: 10,
      operatorId: "ops-a",
    }));

    await wrapper.find(".disable-risk-config").trigger("click");
    expect(riskMocks.disableRuntimeRiskConfig).toHaveBeenCalledWith({ operatorId: "ops-a", reason: "pause" });

    await wrapper.find(".activate-kill-switch").trigger("click");
    await wrapper.find(".release-kill-switch").trigger("click");
    await wrapper.find(".activate-hard-stop").trigger("click");
    expect(wrapper.text()).toContain("确认创建实盘硬停止");
    expect(riskMocks.apiPost).not.toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops",
      expect.anything(),
    );
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    await flushPromises();
    await wrapper.find(".release-hard-stop").trigger("click");
    expect(wrapper.text()).toContain("确认解除实盘硬停止 stop/a");
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    await wrapper.find(".update-strategy-risk").trigger("click");
    await wrapper.find(".refresh-strategy-risk").trigger("click");
    await flushPromises();

    expect(confirmSpy).toHaveBeenCalledWith("确认保存这次实盘运行时风控配置吗？");
    expect(confirmSpy).toHaveBeenCalledWith("确认激活实盘熔断吗？");
    expect(confirmSpy).toHaveBeenCalledWith("确认解除实盘熔断吗？");
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/activate",
      expect.objectContaining({ operatorId: "local" }),
    );
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/release",
      expect.objectContaining({ operatorId: "local" }),
    );
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops",
      expect.objectContaining({ accountId: "ACC-1" }),
    );
    expect(riskMocks.apiPostPath).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops/{hardStopId}/release",
      "/api/v1/system/real-trade-hard-stops/stop%2Fa/release",
      expect.objectContaining({ operatorId: "local" }),
    );
    expect(riskMocks.fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/strategies/strategy%2Fa/runtime-risk",
      expect.objectContaining({ method: "PUT" }),
    );
    expect(store.loadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
  });

  it("does not execute declined dangerous actions and exposes operational failures", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const wrapper = mount(RiskPage, { global: { stubs } });
    await flushPromises();
    await wrapper.find(".save-risk-config").trigger("click");
    await wrapper.find(".activate-kill-switch").trigger("click");
    await wrapper.find(".release-kill-switch").trigger("click");
    expect(riskMocks.saveRuntimeRiskConfig).not.toHaveBeenCalled();
    expect(riskMocks.apiPost).not.toHaveBeenCalled();
    expect(riskMocks.apiPostPath).not.toHaveBeenCalled();
    expect(riskMocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();

    await wrapper.find(".activate-hard-stop").trigger("click");
    await wrapper.get(".action-confirm__actions button").trigger("click");
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);

    riskMocks.saveRuntimeRiskConfig.mockRejectedValueOnce(new Error("风控设置写入失败"));
    riskMocks.apiPost.mockRejectedValueOnce(new Error("硬停止接口失败"));
    confirmSpy.mockReturnValue(true);
    await wrapper.find(".save-risk-config").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("风控设置写入失败");
    await wrapper.find(".activate-hard-stop").trigger("click");
    expect(riskMocks.apiPost).not.toHaveBeenCalled();
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("硬停止接口失败");
  });

  it("ignores hard-stop confirmation when there is no pending action", async () => {
    const wrapper = mount(RiskPage, { global: { stubs } });
    await flushPromises();

    await (
      wrapper.vm as unknown as { confirmHardStopAction: () => Promise<void> }
    ).confirmHardStopAction();
    expect(riskMocks.apiPost).not.toHaveBeenCalled();
    expect(riskMocks.apiPostPath).not.toHaveBeenCalled();
    expect(riskMocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();
  });

  it("shows strategy loading and update failures from the corresponding contract calls", async () => {
    riskMocks.fetchEnvelope.mockRejectedValueOnce(new Error("策略列表不可用"));
    const wrapper = mount(RiskPage, { global: { stubs } });
    await flushPromises();
    expect(wrapper.find(".strategy-risk-error").text()).toContain("策略列表不可用");

    riskMocks.fetchEnvelope.mockResolvedValue([strategyInstance()]);
    riskMocks.fetchEnvelopeWithInit.mockRejectedValueOnce(new Error("模式更新被拒绝"));
    await wrapper.find(".update-strategy-risk").trigger("click");
    await flushPromises();
    expect(wrapper.find(".strategy-risk-error").text()).toContain("模式更新被拒绝");
  });
});
