import { computed, onMounted, ref } from "vue";

import { normalizeStrategyRuntimeRiskSettings } from "@/components/strategy-runtime/strategyRuntimeInstanceBinding";
import {
  apiGet,
  apiPost,
  apiPostPath,
  apiPutPath,
} from "@/composables/shared/apiClient";
import {
  mapStrategyInstance,
  mapStrategyInstances,
} from "@/composables/strategy/strategyContract";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { useRuntimeRiskConfig } from "@/composables/trading/useRuntimeRiskConfig";
import type {
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "@/types";
import type {
  RealTradeHardStopCommandPayload,
  RealTradeHardStopsResponse,
} from "@/contracts";

export type RiskTab = "emergency" | "limits" | "strategy" | "events";

export const RISK_TABS: ReadonlyArray<{ value: RiskTab; label: string }> = [
  { value: "emergency", label: "紧急控制" },
  { value: "limits", label: "运行时限额" },
  { value: "strategy", label: "策略实例" },
  { value: "events", label: "风控事件" },
];

export function useRiskPage() {
  const {
    loadSystemState,
    realTradeHardStops,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
    selectedBrokerAccount,
  } = useConsoleData();

  const { disableRuntimeRiskConfig, saveRuntimeRiskConfig } =
    useRuntimeRiskConfig();

  const activeTab = ref<RiskTab>("emergency");

  const strategyInstances = ref<StrategyInstanceItem[]>([]);
  const strategyRuntimeRiskError = ref("");
  const updatingStrategyRuntimeRiskIds = ref<string[]>([]);
  const realTradeControlError = ref("");
  const updatingRealTradeControlAction = ref("");

  type PendingConfirmation =
    | { kind: "kill-switch-activate" }
    | { kind: "kill-switch-release" }
    | { kind: "hard-stop-activate"; payload: RealTradeHardStopCommandPayload }
    | { kind: "hard-stop-release"; id: string };

  const pendingConfirmation = ref<PendingConfirmation | null>(null);

  type RealTradeHardStopEntry = RealTradeHardStopsResponse["entries"][number];
  type RiskTone = "success" | "warning" | "error";

  function arrayOrEmpty<T>(items: T[] | null | undefined): T[] {
    return Array.isArray(items) ? items : [];
  }

  const strategyInstancesById = computed(
    () => new Map(strategyInstances.value.map((item) => [item.id, item])),
  );
  const realTradeHardStopEntries = computed<RealTradeHardStopEntry[]>(() =>
    arrayOrEmpty(realTradeHardStops.value.entries),
  );

  const hardStopPrefill = computed(() => {
    const selected = selectedBrokerAccount.value;
    if (
      selected == null ||
      selected.tradingEnvironment.trim().toUpperCase() !== "REAL"
    ) {
      return null;
    }
    return {
      brokerId: selected.brokerId,
      accountId: selected.accountId,
      tradingEnvironment: "REAL",
    };
  });

  const riskPosture = computed<{ label: string; tone: RiskTone; hint: string }>(
    () => {
      if (realTradeKillSwitchState.value.killSwitchActive) {
        return {
          label: "熔断中",
          tone: "error",
          hint: "实盘下单与改单已被紧急熔断阻断",
        };
      }
      if (realTradeHardStopEntries.value.length) {
        return {
          label: "部分阻断",
          tone: "warning",
          hint: `${realTradeHardStopEntries.value.length} 条硬停止正在生效`,
        };
      }
      if (
        realTradeRiskState.value.realTradingEnabled &&
        !realTradeRiskState.value.riskEnabled
      ) {
        return {
          label: "限额未配置",
          tone: "warning",
          hint: "实盘已开放，但尚未配置单笔限额",
        };
      }
      return { label: "正常", tone: "success", hint: "未触发任何阻断" };
    },
  );

  const statusRows = computed<
    Array<{ key: string; label: string; value: string; tone: RiskTone }>
  >(() => [
    {
      key: "real-trading",
      label: "实盘总闸",
      value: realTradeRiskState.value.realTradingEnabled ? "已开放" : "未开放",
      tone: realTradeRiskState.value.realTradingEnabled ? "success" : "warning",
    },
    {
      key: "limits",
      label: "单笔限额",
      value: realTradeRiskState.value.riskEnabled ? "已配置" : "未配置",
      tone: realTradeRiskState.value.riskEnabled ? "success" : "warning",
    },
    {
      key: "kill-switch",
      label: "紧急熔断",
      value: realTradeKillSwitchState.value.killSwitchActive ? "已激活" : "未激活",
      tone: realTradeKillSwitchState.value.killSwitchActive ? "error" : "success",
    },
    {
      key: "hard-stops",
      label: "硬停止",
      value: `${realTradeHardStopEntries.value.length} 条`,
      tone: realTradeHardStopEntries.value.length ? "error" : "success",
    },
  ]);

  function formatEffectiveLimit(value: number | null | undefined): string {
    return value == null ? "未设置" : String(value);
  }

  const stripSections = computed<
    Array<{
      title: string;
      items: Array<{ label: string; value: string; tone?: RiskTone }>;
    }>
  >(() => [
    {
      title: "实盘总闸",
      items: [
        {
          label: "状态",
          value: realTradeRiskState.value.realTradingEnabled ? "已开放" : "未开放",
          tone: realTradeRiskState.value.realTradingEnabled
            ? "success"
            : "warning",
        },
        {
          label: "运行时限额",
          value: realTradeRiskState.value.riskEnabled ? "已配置" : "未配置",
          tone: realTradeRiskState.value.riskEnabled ? "success" : "warning",
        },
      ],
    },
    {
      title: "单笔限额（当前生效）",
      items: [
        {
          label: "数量",
          value: formatEffectiveLimit(
            realTradeRiskState.value.effectiveMaxOrderQuantity,
          ),
        },
        {
          label: "金额",
          value: formatEffectiveLimit(
            realTradeRiskState.value.effectiveMaxOrderNotional,
          ),
        },
      ],
    },
    {
      title: "紧急熔断",
      items: [
        {
          label: "状态",
          value: realTradeKillSwitchState.value.killSwitchActive
            ? "正在阻断"
            : "未阻断",
          tone: realTradeKillSwitchState.value.killSwitchActive
            ? "error"
            : "success",
        },
        {
          label: "撤单",
          value: realTradeKillSwitchState.value.allowsCancel ? "允许" : "阻断",
        },
      ],
    },
    {
      title: "硬停止",
      items: [
        {
          label: "生效",
          value: `${realTradeHardStopEntries.value.length} 条`,
          tone: realTradeHardStopEntries.value.length ? "error" : "success",
        },
        {
          label: "预填账户",
          value: hardStopPrefill.value?.accountId ?? "全部账户",
        },
      ],
    },
  ]);

  const sidebarFacts = computed(() => [
    {
      label: "生效数量限额",
      value: formatEffectiveLimit(realTradeRiskState.value.effectiveMaxOrderQuantity),
    },
    {
      label: "生效金额限额",
      value: formatEffectiveLimit(realTradeRiskState.value.effectiveMaxOrderNotional),
    },
    { label: "策略实例", value: `${strategyInstances.value.length} 个` },
    {
      label: "风控事件",
      value: `${arrayOrEmpty(realTradeRiskEvents.value.entries).length + arrayOrEmpty(realTradeKillSwitchEvents.value.entries).length} 条`,
    },
  ]);

  function tabBadge(tab: RiskTab): number {
    if (tab === "emergency") return realTradeHardStopEntries.value.length;
    if (tab === "strategy") return strategyInstances.value.length;
    return 0;
  }

  function setActiveTab(tab: RiskTab): void {
    if (activeTab.value === tab) return;
    activeTab.value = tab;
  }

  onMounted(() => {
    void Promise.all([
      loadSystemState({ bypassCooldown: true }),
      loadStrategyInstances(),
    ]);
  });

  async function loadStrategyInstances(): Promise<void> {
    try {
      strategyInstances.value = mapStrategyInstances(
        await apiGet("/api/v1/strategies"),
      );
      strategyRuntimeRiskError.value = "";
    } catch (error) {
      strategyRuntimeRiskError.value =
        error instanceof Error ? error.message : "加载策略实例动态风控失败。";
    }
  }

  function runtimeRiskForInstance(instanceId: string): StrategyRuntimeRiskSettings {
    return normalizeStrategyRuntimeRiskSettings(
      strategyInstancesById.value.get(instanceId)?.binding?.runtimeRisk,
    );
  }

  function isUpdatingStrategyRuntimeRisk(instanceId: string): boolean {
    return updatingStrategyRuntimeRiskIds.value.includes(instanceId);
  }

  async function updateStrategyRuntimeRiskMode(
    instanceId: string,
    mode: StrategyRuntimeRiskMode,
  ): Promise<void> {
    const runtimeRisk = normalizeStrategyRuntimeRiskSettings({
      ...runtimeRiskForInstance(instanceId),
      mode,
    });

    strategyRuntimeRiskError.value = "";
    updatingStrategyRuntimeRiskIds.value = [
      ...updatingStrategyRuntimeRiskIds.value,
      instanceId,
    ];
    try {
      const updated = mapStrategyInstance(
        await apiPutPath(
          "/api/v1/strategies/{instanceId}/runtime-risk",
        `/api/v1/strategies/${encodeURIComponent(instanceId)}/runtime-risk`,
          runtimeRisk,
        ),
      );
      strategyInstances.value = strategyInstances.value.map((item) =>
        item.id === updated.id ? updated : item,
      );
      await loadSystemState({ bypassCooldown: true });
    } catch (error) {
      strategyRuntimeRiskError.value =
        error instanceof Error ? error.message : "更新策略实例动态风控失败。";
    } finally {
      updatingStrategyRuntimeRiskIds.value =
        updatingStrategyRuntimeRiskIds.value.filter((id) => id !== instanceId);
    }
  }

  async function refreshRiskState(): Promise<void> {
    await Promise.all([
      loadSystemState({ bypassCooldown: true }),
      loadStrategyInstances(),
    ]);
  }

  function isLooseningLimit(
    next: number | null | undefined,
    current: number | null | undefined,
  ): boolean {
    if (next == null) return current != null;
    if (current == null) return true;
    return next > current;
  }

  const pendingRuntimeRiskSave = ref<{
    realTradingEnabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    operatorId: string;
    reason: string;
  } | null>(null);

  async function saveRuntimeRisk(payload: {
    realTradingEnabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    operatorId: string;
    reason: string;
  }): Promise<void> {
    const enablingRealTrading =
      payload.realTradingEnabled && !realTradeRiskState.value.realTradingEnabled;
    const loosening =
      isLooseningLimit(
        payload.maxOrderQuantity,
        realTradeRiskState.value.effectiveMaxOrderQuantity,
      ) ||
      isLooseningLimit(
        payload.maxOrderNotional,
        realTradeRiskState.value.effectiveMaxOrderNotional,
      );
    if (enablingRealTrading || loosening) {
      pendingRuntimeRiskSave.value = payload;
      return;
    }
    await persistRuntimeRisk(payload);
  }

  async function persistRuntimeRisk(payload: {
    realTradingEnabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    operatorId: string;
    reason: string;
  }): Promise<void> {
    realTradeControlError.value = "";
    updatingRealTradeControlAction.value = "runtime-risk.save";
    try {
      await saveRuntimeRiskConfig(payload);
      await refreshRiskState();
    } catch (error) {
      realTradeControlError.value =
        error instanceof Error ? error.message : "保存运行时风控配置失败。";
    } finally {
      updatingRealTradeControlAction.value = "";
    }
  }

  async function confirmRuntimeRiskSave(): Promise<void> {
    const payload = pendingRuntimeRiskSave.value;
    pendingRuntimeRiskSave.value = null;
    if (payload != null) await persistRuntimeRisk(payload);
  }

  async function disableRuntimeRisk(payload: {
    operatorId: string;
    reason: string;
  }): Promise<void> {
    realTradeControlError.value = "";
    updatingRealTradeControlAction.value = "runtime-risk.save";
    try {
      await disableRuntimeRiskConfig(payload);
      await refreshRiskState();
    } catch (error) {
      realTradeControlError.value =
        error instanceof Error ? error.message : "关闭运行时风控配置失败。";
    } finally {
      updatingRealTradeControlAction.value = "";
    }
  }

  async function runRealTradeControlAction(
    action: string,
    request: () => Promise<unknown>,
  ): Promise<void> {
    realTradeControlError.value = "";
    updatingRealTradeControlAction.value = action;
    try {
      await request();
      await refreshRiskState();
    } catch (error) {
      realTradeControlError.value =
        error instanceof Error ? error.message : "更新实盘控制失败。";
    } finally {
      updatingRealTradeControlAction.value = "";
    }
  }

  function activateKillSwitch(): void {
    pendingConfirmation.value = { kind: "kill-switch-activate" };
  }

  function releaseKillSwitch(): void {
    pendingConfirmation.value = { kind: "kill-switch-release" };
  }

  function activateHardStop(payload: RealTradeHardStopCommandPayload): void {
    pendingConfirmation.value = { kind: "hard-stop-activate", payload };
  }

  function releaseHardStop(id: string): void {
    pendingConfirmation.value = { kind: "hard-stop-release", id };
  }

  const confirmationView = computed(() => {
    const pending = pendingConfirmation.value;
    if (pending == null) return null;
    switch (pending.kind) {
      case "kill-switch-activate":
        return {
          title: "激活实盘熔断",
          message:
            "确认激活实盘熔断吗？生效后立即阻断所有实盘下单与改单，撤单不受影响。请仅在出现紧急情况时继续。",
          confirmLabel: "确认激活",
        };
      case "kill-switch-release":
        return {
          title: "解除实盘熔断",
          message:
            "确认解除实盘熔断吗？解除后实盘下单与改单立即恢复。请确认风险已处置后再继续。",
          confirmLabel: "确认解除",
        };
      case "hard-stop-release":
        return {
          title: "解除实盘硬停止",
          message: `确认解除实盘硬停止 ${pending.id}？请仅在风险已处置且可以恢复下单时继续。`,
          confirmLabel: "确认解除",
        };
      case "hard-stop-activate": {
        const accountId =
          String(pending.payload.accountId ?? "").trim() || "全部账户";
        const scope = String(pending.payload.hardStopScope ?? "ACCOUNT").trim();
        const market = String(pending.payload.market ?? "").trim();
        const symbol = String(pending.payload.symbol ?? "").trim();
        const target = [accountId, scope, market, symbol]
          .filter(Boolean)
          .join(" / ");
        return {
          title: "创建实盘硬停止",
          message: `确认创建实盘硬停止（${target}）？生效后会立即阻断匹配范围内的新实盘订单。`,
          confirmLabel: "确认创建",
        };
      }
    }
  });

  const confirmationBusy = computed(() =>
    updatingRealTradeControlAction.value.startsWith("kill-switch.") ||
    updatingRealTradeControlAction.value.startsWith("hard-stop."),
  );

  async function confirmPendingAction(): Promise<void> {
    const pending = pendingConfirmation.value;
    if (pending == null || updatingRealTradeControlAction.value !== "") return;
    switch (pending.kind) {
      case "kill-switch-activate":
        await runRealTradeControlAction("kill-switch.activate", () =>
          apiPost("/api/v1/system/real-trade-kill-switch/activate", {
            tradingEnvironment: "REAL",
            operatorId: "local",
            reason: "manual activation from risk page",
          }),
        );
        break;
      case "kill-switch-release":
        await runRealTradeControlAction("kill-switch.release", () =>
          apiPost("/api/v1/system/real-trade-kill-switch/release", {
            tradingEnvironment: "REAL",
            operatorId: "local",
            reason: "manual release from risk page",
          }),
        );
        break;
      case "hard-stop-activate":
        await runRealTradeControlAction("hard-stop.activate", () =>
          apiPost("/api/v1/system/real-trade-hard-stops", pending.payload),
        );
        break;
      case "hard-stop-release":
        await runRealTradeControlAction(`hard-stop.release.${pending.id}`, () =>
          apiPostPath(
            "/api/v1/system/real-trade-hard-stops/{hardStopId}/release",
            `/api/v1/system/real-trade-hard-stops/${encodeURIComponent(pending.id)}/release`,
            {
              operatorId: "local",
              reason: "manual release from risk page",
            },
          ),
        );
        break;
    }
    pendingConfirmation.value = null;
  }

  return {
    RISK_TABS,
    activeTab,
    strategyInstances,
    strategyRuntimeRiskError,
    updatingStrategyRuntimeRiskIds,
    realTradeControlError,
    updatingRealTradeControlAction,
    pendingConfirmation,
    pendingRuntimeRiskSave,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
    strategyInstancesById,
    realTradeHardStopEntries,
    hardStopPrefill,
    riskPosture,
    statusRows,
    stripSections,
    sidebarFacts,
    confirmationView,
    confirmationBusy,
    tabBadge,
    setActiveTab,
    loadStrategyInstances,
    runtimeRiskForInstance,
    isUpdatingStrategyRuntimeRisk,
    updateStrategyRuntimeRiskMode,
    refreshRiskState,
    saveRuntimeRisk,
    confirmRuntimeRiskSave,
    disableRuntimeRisk,
    activateKillSwitch,
    releaseKillSwitch,
    activateHardStop,
    releaseHardStop,
    confirmPendingAction,
  };
}
