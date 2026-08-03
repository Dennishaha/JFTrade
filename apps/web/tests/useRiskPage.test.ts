// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import {
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
} from "@/types";

const riskMocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPostPath: vi.fn(),
  apiPutPath: vi.fn(),
  disableRuntimeRiskConfig: vi.fn(),
  saveRuntimeRiskConfig: vi.fn(),
  store: null as null | Record<string, unknown>,
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGet: (...args: unknown[]) => riskMocks.apiGet(...args),
  apiPost: (...args: unknown[]) => riskMocks.apiPost(...args),
  apiPostPath: (...args: unknown[]) => riskMocks.apiPostPath(...args),
  apiPutPath: (...args: unknown[]) => riskMocks.apiPutPath(...args),
}));

vi.mock("@/composables/workspace/useConsoleData", () => ({
  useConsoleData: () => riskMocks.store,
}));

vi.mock("@/composables/trading/useRuntimeRiskConfig", () => ({
  useRuntimeRiskConfig: () => ({
    disableRuntimeRiskConfig: riskMocks.disableRuntimeRiskConfig,
    saveRuntimeRiskConfig: riskMocks.saveRuntimeRiskConfig,
  }),
}));

import { useRiskPage } from "@/composables/risk/useRiskPage";

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

function mountRiskPage() {
  let page!: ReturnType<typeof useRiskPage>;
  const wrapper = mount(
    defineComponent({
      setup() {
        page = useRiskPage();
        return () => h("div");
      },
    }),
  );
  return { page, wrapper };
}

beforeEach(() => {
  riskMocks.store = createRiskStore();
  riskMocks.apiGet.mockResolvedValue([strategyInstance()]);
  riskMocks.apiPost.mockResolvedValue({});
  riskMocks.apiPostPath.mockResolvedValue({});
  riskMocks.apiPutPath.mockImplementation(async (_template: string, path: string) => {
    if (path.includes("/runtime-risk")) {
      return {
        ...strategyInstance(),
        binding: {
          runtimeRisk: {
            ...strategyInstance().binding.runtimeRisk,
            mode: "enforce",
          },
        },
      };
    }
    return {};
  });
  riskMocks.disableRuntimeRiskConfig.mockResolvedValue(undefined);
  riskMocks.saveRuntimeRiskConfig.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useRiskPage", () => {
  it("loads the control-plane snapshot and strategy instances on mount", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    const store = riskMocks.store as ReturnType<typeof createRiskStore>;
    expect(store.loadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
    expect(riskMocks.apiGet).toHaveBeenCalledWith("/api/v1/strategies");
    expect(page.strategyInstances.value).toHaveLength(1);
    expect(page.tabBadge("strategy")).toBe(1);
    expect(page.strategyRuntimeRiskError.value).toBe("");
  });

  it("surfaces a strategy-instance load failure without dropping the page state", async () => {
    riskMocks.apiGet.mockRejectedValueOnce(new Error("策略列表不可用"));
    const { page } = mountRiskPage();
    await flushPromises();

    expect(page.strategyRuntimeRiskError.value).toBe("策略列表不可用");
    expect(page.strategyInstances.value).toEqual([]);
  });

  it("derives the overall posture from kill switch, hard stops, and limit state", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    expect(page.riskPosture.value.label).toBe("正常");

    const store = riskMocks.store as ReturnType<typeof createRiskStore>;
    store.realTradeRiskState.value = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: false,
    };
    expect(page.riskPosture.value.label).toBe("限额未配置");

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
    expect(page.riskPosture.value.label).toBe("部分阻断");
    expect(page.tabBadge("emergency")).toBe(1);

    store.realTradeKillSwitchState.value = {
      ...emptyRealTradeKillSwitchState,
      killSwitchActive: true,
    };
    expect(page.riskPosture.value.label).toBe("熔断中");
    expect(page.statusRows.value.find((row) => row.key === "kill-switch")?.tone).toBe("error");
  });

  it("persists tighter runtime limits without an extra confirmation", async () => {
    const store = riskMocks.store as ReturnType<typeof createRiskStore>;
    store.realTradeRiskState.value = {
      ...emptyRealTradeRiskState,
      realTradingEnabled: true,
      riskEnabled: true,
      effectiveMaxOrderQuantity: 20,
      effectiveMaxOrderNotional: 4000,
    };
    const { page } = mountRiskPage();
    await flushPromises();

    await page.saveRuntimeRisk({
      realTradingEnabled: true,
      maxOrderQuantity: 10,
      maxOrderNotional: 2000,
      operatorId: "risk-ops",
      reason: "tighten limits",
    });

    expect(page.pendingRuntimeRiskSave.value).toBeNull();
    expect(riskMocks.saveRuntimeRiskConfig).toHaveBeenCalledWith(
      expect.objectContaining({ maxOrderQuantity: 10 }),
    );
  });

  it("parks dangerous runtime changes behind an explicit confirmation", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    const payload = {
      realTradingEnabled: true,
      maxOrderQuantity: 10,
      maxOrderNotional: 2000,
      operatorId: "risk-ops",
      reason: "session open",
    };
    await page.saveRuntimeRisk(payload);

    expect(page.pendingRuntimeRiskSave.value).toEqual(payload);
    expect(riskMocks.saveRuntimeRiskConfig).not.toHaveBeenCalled();

    await page.confirmRuntimeRiskSave();
    expect(page.pendingRuntimeRiskSave.value).toBeNull();
    expect(riskMocks.saveRuntimeRiskConfig).toHaveBeenCalledWith(payload);

    const store = riskMocks.store as ReturnType<typeof createRiskStore>;
    expect(store.loadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
  });

  it("exposes runtime-risk persistence failures as control errors", async () => {
    riskMocks.saveRuntimeRiskConfig.mockRejectedValueOnce(new Error("写入失败"));
    const { page } = mountRiskPage();
    await flushPromises();

    await page.saveRuntimeRisk({
      realTradingEnabled: false,
      maxOrderQuantity: null,
      maxOrderNotional: null,
      operatorId: "risk-ops",
      reason: "tighten",
    });

    expect(page.realTradeControlError.value).toBe("写入失败");
    expect(page.updatingRealTradeControlAction.value).toBe("");
  });

  it("dispatches confirmed kill-switch and hard-stop actions to their endpoints", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    page.activateKillSwitch();
    expect(page.confirmationView.value?.title).toBe("激活实盘熔断");
    await page.confirmPendingAction();
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/activate",
      expect.objectContaining({
        tradingEnvironment: "REAL",
        reason: "manual activation from risk page",
      }),
    );
    expect(page.pendingConfirmation.value).toBeNull();

    page.releaseKillSwitch();
    expect(page.confirmationView.value?.title).toBe("解除实盘熔断");
    await page.confirmPendingAction();
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-kill-switch/release",
      expect.objectContaining({
        reason: "manual release from risk page",
      }),
    );

    page.releaseHardStop("stop/a");
    expect(page.confirmationView.value?.message).toContain("stop/a");
    await page.confirmPendingAction();
    expect(riskMocks.apiPostPath).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops/{hardStopId}/release",
      "/api/v1/system/real-trade-hard-stops/stop%2Fa/release",
      expect.objectContaining({ operatorId: "local" }),
    );
    expect(page.pendingConfirmation.value).toBeNull();
  });

  it("builds hard-stop confirmation copy from the requested scope", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    page.activateHardStop({
      accountId: "ACC-1",
      hardStopScope: "SYMBOL",
      market: "HK",
      symbol: "HK.00700",
      operatorId: "ops-a",
      reason: "manual hold",
    });

    const view = page.confirmationView.value;
    expect(view?.title).toBe("创建实盘硬停止");
    expect(view?.message).toContain("ACC-1 / SYMBOL / HK / HK.00700");
    expect(view?.confirmLabel).toBe("确认创建");

    await page.confirmPendingAction();
    expect(riskMocks.apiPost).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-hard-stops",
      expect.objectContaining({ accountId: "ACC-1", symbol: "HK.00700" }),
    );
  });

  it("ignores confirmation while another control action is still running", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    page.updatingRealTradeControlAction.value = "hard-stop.release.stop/a";
    page.activateKillSwitch();
    await page.confirmPendingAction();

    expect(riskMocks.apiPost).not.toHaveBeenCalled();
    expect(page.pendingConfirmation.value).not.toBeNull();
  });

  it("updates a strategy runtime risk mode and reloads the control-plane snapshot", async () => {
    const { page } = mountRiskPage();
    await flushPromises();

    expect(page.runtimeRiskForInstance("strategy/a").mode).toBe("monitor");

    await page.updateStrategyRuntimeRiskMode("strategy/a", "enforce");

    expect(riskMocks.apiPutPath).toHaveBeenCalledWith(
      "/api/v1/strategies/{instanceId}/runtime-risk",
      "/api/v1/strategies/strategy%2Fa/runtime-risk",
      expect.objectContaining({ mode: "enforce" }),
    );
    expect(page.strategyInstances.value[0]?.binding?.runtimeRisk.mode).toBe("enforce");
    expect(page.isUpdatingStrategyRuntimeRisk("strategy/a")).toBe(false);

    const store = riskMocks.store as ReturnType<typeof createRiskStore>;
    expect(store.loadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
  });

  it("clears the updating marker and surfaces a rejected mode update", async () => {
    riskMocks.apiPutPath.mockRejectedValueOnce(new Error("模式更新被拒绝"));
    const { page } = mountRiskPage();
    await flushPromises();

    await page.updateStrategyRuntimeRiskMode("strategy/a", "enforce");

    expect(page.strategyRuntimeRiskError.value).toBe("模式更新被拒绝");
    expect(page.isUpdatingStrategyRuntimeRisk("strategy/a")).toBe(false);
  });
});
