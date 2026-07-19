<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import HardStopControlPanel from "../components/risk/HardStopControlPanel.vue";
import RealTradeEmergencyPanel from "../components/risk/RealTradeEmergencyPanel.vue";
import RiskEventTimeline from "../components/risk/RiskEventTimeline.vue";
import RuntimeRiskConfigPanel from "../components/risk/RuntimeRiskConfigPanel.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import StrategyRuntimeRiskSection from "../components/risk/StrategyRuntimeRiskSection.vue";
import { normalizeStrategyRuntimeRiskSettings } from "../components/strategy-runtime/strategyRuntimeInstanceBinding";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useRuntimeRiskConfig } from "../composables/useRuntimeRiskConfig";
import { useConsoleData } from "../composables/useConsoleData";
import type {
  RealTradeHardStopsResponse,
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "../contracts";

const {
  loadSystemState,
  realTradeHardStops,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  systemStatus,
} = useConsoleData();

const { disableRuntimeRiskConfig, saveRuntimeRiskConfig } =
  useRuntimeRiskConfig();

const strategyInstances = ref<StrategyInstanceItem[]>([]);
const strategyRuntimeRiskError = ref("");
const updatingStrategyRuntimeRiskIds = ref<string[]>([]);
const realTradeControlError = ref("");
const updatingRealTradeControlAction = ref("");
const pendingHardStopConfirmation = ref<
  | { kind: "activate"; payload: Record<string, unknown> }
  | { kind: "release"; id: string }
  | null
>(null);

type RealTradeHardStopEntry = RealTradeHardStopsResponse["entries"][number];
type RiskHeaderTone = "good" | "warn" | "danger";

function arrayOrEmpty<T>(items: T[] | null | undefined): T[] {
  return Array.isArray(items) ? items : [];
}

const strategyInstancesById = computed(
  () => new Map(strategyInstances.value.map((item) => [item.id, item])),
);
const realTradeHardStopEntries = computed<RealTradeHardStopEntry[]>(() =>
  arrayOrEmpty(realTradeHardStops.value.entries),
);
const riskHeaderStats = computed<Array<{
  label: string;
  value: string;
  tone: RiskHeaderTone;
}>>(() => [
  {
    label: "实盘总闸",
    value: realTradeRiskState.value.realTradingEnabled ? "已开放" : "未开放",
    tone: realTradeRiskState.value.realTradingEnabled ? "good" : "warn",
  },
  {
    label: "单笔限额",
    value: realTradeRiskState.value.riskEnabled ? "已配置" : "未配置",
    tone: realTradeRiskState.value.riskEnabled ? "good" : "warn",
  },
  {
    label: "熔断",
    value: realTradeKillSwitchState.value.killSwitchActive ? "已激活" : "未激活",
    tone: realTradeKillSwitchState.value.killSwitchActive ? "danger" : "good",
  },
  {
    label: "硬停止",
    value: `${realTradeHardStopEntries.value.length}`,
    tone: realTradeHardStopEntries.value.length ? "danger" : "good",
  },
]);

onMounted(() => {
  void Promise.all([loadSystemState({ bypassCooldown: true }), loadStrategyInstances()]);
});

async function loadStrategyInstances(): Promise<void> {
  try {
    strategyInstances.value = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
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
    const updated = await fetchEnvelopeWithInit<StrategyInstanceItem>(
      `/api/v1/strategies/${encodeURIComponent(instanceId)}/runtime-risk`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(runtimeRisk),
      },
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
  await loadSystemState({ bypassCooldown: true });
}

function confirmDangerous(message: string): boolean {
  if (typeof window === "undefined" || typeof window.confirm !== "function") {
    return true;
  }
  return window.confirm(message);
}

function isLooseningLimit(
  next: number | null,
  current: number | null,
): boolean {
  if (next == null) return current != null;
  if (current == null) return true;
  return next > current;
}

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
  if (
    (enablingRealTrading || loosening) &&
    !confirmDangerous("确认保存这次实盘运行时风控配置吗？")
  ) {
    return;
  }

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
  path: string,
  body: Record<string, unknown>,
): Promise<void> {
  realTradeControlError.value = "";
  updatingRealTradeControlAction.value = action;
  try {
    await fetchEnvelopeWithInit(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    await refreshRiskState();
  } catch (error) {
    realTradeControlError.value =
      error instanceof Error ? error.message : "更新实盘控制失败。";
  } finally {
    updatingRealTradeControlAction.value = "";
  }
}

function activateKillSwitch(): Promise<void> | void {
  if (!confirmDangerous("确认激活实盘熔断吗？")) return;
  return runRealTradeControlAction(
    "kill-switch.activate",
    "/api/v1/system/real-trade-kill-switch/activate",
    {
      tradingEnvironment: "REAL",
      operatorId: "local",
      reason: "manual activation from RiskPage",
    },
  );
}

function releaseKillSwitch(): Promise<void> | void {
  if (!confirmDangerous("确认解除实盘熔断吗？")) return;
  return runRealTradeControlAction(
    "kill-switch.release",
    "/api/v1/system/real-trade-kill-switch/release",
    {
      tradingEnvironment: "REAL",
      operatorId: "local",
      reason: "manual release from RiskPage",
    },
  );
}

const hardStopConfirmationMessage = computed(() => {
  const pending = pendingHardStopConfirmation.value;
  if (pending == null) return "";
  if (pending.kind === "release") {
    return `确认解除实盘硬停止 ${pending.id}？请仅在风险已处置且可以恢复下单时继续。`;
  }
  const accountId = String(pending.payload.accountId ?? "").trim() || "全部账户";
  const scope = String(pending.payload.hardStopScope ?? "ACCOUNT").trim();
  const market = String(pending.payload.market ?? "").trim();
  const symbol = String(pending.payload.symbol ?? "").trim();
  const target = [accountId, scope, market, symbol].filter(Boolean).join(" / ");
  return `确认创建实盘硬停止（${target}）？生效后会立即阻断匹配范围内的新实盘订单。`;
});

function activateHardStop(payload: Record<string, unknown>): void {
  pendingHardStopConfirmation.value = { kind: "activate", payload };
}

function releaseHardStop(id: string): void {
  pendingHardStopConfirmation.value = { kind: "release", id };
}

async function confirmHardStopAction(): Promise<void> {
  const pending = pendingHardStopConfirmation.value;
  if (pending == null || updatingRealTradeControlAction.value !== "") return;
  if (pending.kind === "activate") {
    await runRealTradeControlAction(
      "hard-stop.activate",
      "/api/v1/system/real-trade-hard-stops",
      pending.payload,
    );
  } else {
    await runRealTradeControlAction(
      `hard-stop.release.${pending.id}`,
      `/api/v1/system/real-trade-hard-stops/${encodeURIComponent(pending.id)}/release`,
      {
        operatorId: "local",
        reason: "manual release from RiskPage",
      },
    );
  }
  pendingHardStopConfirmation.value = null;
}
</script>

<template>
  <div class="risk-page grid min-w-0 gap-5">
    <PageHeader
      eyebrow="风控"
      title="实盘风控"
      description="从下单用户的角度确认实盘是否可用，运行时配置保存后立即生效。"
      :stats="riskHeaderStats"
    />

    <v-alert
      v-if="realTradeControlError"
      type="warning"
      variant="tonal"
      density="compact"
    >
      {{ realTradeControlError }}
    </v-alert>

    <section class="grid gap-5 xl:grid-cols-[1fr_1fr]">
      <RuntimeRiskConfigPanel
        :loading="updatingRealTradeControlAction === 'runtime-risk.save'"
        :risk-state="realTradeRiskState"
        @disable="disableRuntimeRisk"
        @save="saveRuntimeRisk"
      />

      <div class="grid gap-5">
        <RealTradeEmergencyPanel
          :kill-switch="realTradeKillSwitchState"
          :loading-action="updatingRealTradeControlAction"
          @activate="activateKillSwitch"
          @release="releaseKillSwitch"
        />
        <HardStopControlPanel
          :entries="realTradeHardStopEntries"
          :loading-action="updatingRealTradeControlAction"
          @activate="activateHardStop"
          @release="releaseHardStop"
        />
      </div>
    </section>

    <RiskEventTimeline
      :kill-switch-events="realTradeKillSwitchEvents.entries"
      :risk-events="realTradeRiskEvents.entries"
    />

    <StrategyRuntimeRiskSection
      :error="strategyRuntimeRiskError"
      :instances="strategyInstances"
      :is-updating="isUpdatingStrategyRuntimeRisk"
      :runtime-risk-for-instance="runtimeRiskForInstance"
      @refresh="loadStrategyInstances"
      @update-mode="updateStrategyRuntimeRiskMode"
    />
    <ActionConfirmDialog
      :open="pendingHardStopConfirmation != null"
      :title="pendingHardStopConfirmation?.kind === 'release' ? '解除实盘硬停止' : '创建实盘硬停止'"
      :message="hardStopConfirmationMessage"
      :confirm-label="pendingHardStopConfirmation?.kind === 'release' ? '确认解除' : '确认创建'"
      :busy="updatingRealTradeControlAction.startsWith('hard-stop.')"
      @close="pendingHardStopConfirmation = null"
      @confirm="confirmHardStopAction"
    />
  </div>
</template>
