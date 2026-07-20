<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import HardStopControlPanel from "../components/risk/HardStopControlPanel.vue";
import RealTradeEmergencyPanel from "../components/risk/RealTradeEmergencyPanel.vue";
import RiskEventTimeline from "../components/risk/RiskEventTimeline.vue";
import RuntimeRiskConfigPanel from "../components/risk/RuntimeRiskConfigPanel.vue";
import StrategyRuntimeRiskSection from "../components/risk/StrategyRuntimeRiskSection.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import { normalizeStrategyRuntimeRiskSettings } from "../components/strategy-runtime/strategyRuntimeInstanceBinding";
import {
  apiPost,
  apiPostPath,
  fetchEnvelope,
  fetchEnvelopeWithInit,
} from "../composables/apiClient";
import { useConsoleData } from "../composables/useConsoleData";
import { useRuntimeRiskConfig } from "../composables/useRuntimeRiskConfig";
import type {
  RealTradeHardStopCommandPayload,
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
  selectedBrokerAccount,
} = useConsoleData();

const { disableRuntimeRiskConfig, saveRuntimeRiskConfig } =
  useRuntimeRiskConfig();

type RiskTab = "emergency" | "limits" | "strategy" | "events";

const RISK_TABS: ReadonlyArray<{ value: RiskTab; label: string }> = [
  { value: "emergency", label: "紧急控制" },
  { value: "limits", label: "运行时限额" },
  { value: "strategy", label: "策略实例" },
  { value: "events", label: "风控事件" },
];

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
    strategyInstances.value = await fetchEnvelope<StrategyInstanceItem[]>(
      "/api/v1/strategies",
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
</script>

<template>
  <div class="risk-page">
    <aside class="risk-sidebar" aria-label="风控态势摘要">
      <div class="risk-sidebar__head">
        <div class="risk-sidebar__name">风控中心</div>
        <span
          class="risk-sidebar__posture-dot"
          :class="`tv-status--${riskPosture.tone}`"
        >
          <i class="tv-state-dot"></i>{{ riskPosture.label }}
        </span>
      </div>

      <div
        class="risk-sidebar__posture"
        :class="`tv-status--${riskPosture.tone}`"
        data-testid="risk-posture"
      >
        <div class="risk-sidebar__posture-label">整体风险态势</div>
        <div class="risk-sidebar__posture-value">
          {{ riskPosture.label }}
        </div>
        <div class="risk-sidebar__posture-hint">{{ riskPosture.hint }}</div>
      </div>

      <div class="risk-sidebar__rows">
        <div
          v-for="row in statusRows"
          :key="row.key"
          class="risk-sidebar__row"
          :class="`tv-status--${row.tone}`"
          :data-status-key="row.key"
        >
          <span>{{ row.label }}</span>
          <b>{{ row.value }}</b>
        </div>
      </div>

      <div class="risk-sidebar__facts">
        <div
          v-for="fact in sidebarFacts"
          :key="fact.label"
          class="risk-sidebar__fact"
        >
          <span>{{ fact.label }}</span>
          <b :title="fact.value">{{ fact.value }}</b>
        </div>
      </div>

      <div class="risk-sidebar__footer">
        <button
          type="button"
          class="tv-btn tv-btn-ghost risk-sidebar__refresh"
          @click="refreshRiskState"
        >
          刷新风控状态
        </button>
      </div>
    </aside>

    <section class="risk-main">
      <div class="risk-strip" aria-label="风控指标">
        <section
          v-for="section in stripSections"
          :key="section.title"
          class="risk-strip__section"
        >
          <header class="risk-strip__title">
            {{ section.title }}
            <i
              v-if="section.title === '实盘总闸'"
              class="tv-state-dot"
              :class="`tv-status--${riskPosture.tone}`"
              :title="riskPosture.hint"
            ></i>
          </header>
          <div class="risk-strip__grid">
            <div
              v-for="item in section.items"
              :key="item.label"
              class="risk-strip__item"
            >
              <span>{{ item.label }}</span>
              <b :class="item.tone ? `tv-status--${item.tone}` : undefined">
                {{ item.value }}
              </b>
            </div>
          </div>
        </section>
      </div>

      <div class="risk-main__tabs-row">
        <div class="risk-main__tabs" role="tablist" aria-label="风控视图">
          <button
            v-for="tab in RISK_TABS"
            :key="tab.value"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.value"
            :class="{ 'is-active': activeTab === tab.value }"
            @click="setActiveTab(tab.value)"
          >
            {{ tab.label }}
            <span v-if="tabBadge(tab.value) > 0">{{ tabBadge(tab.value) }}</span>
          </button>
        </div>
      </div>

      <div class="risk-main__content">
        <div
          v-if="realTradeControlError"
          class="risk-main__error tv-status--warning tv-status-surface"
          role="alert"
        >
          {{ realTradeControlError }}
        </div>

        <div
          v-if="activeTab === 'emergency'"
          class="risk-main__danger-grid"
        >
          <RealTradeEmergencyPanel
            :kill-switch="realTradeKillSwitchState"
            :loading-action="updatingRealTradeControlAction"
            @activate="activateKillSwitch"
            @release="releaseKillSwitch"
          />
          <HardStopControlPanel
            :entries="realTradeHardStopEntries"
            :loading-action="updatingRealTradeControlAction"
            :prefill="hardStopPrefill"
            @activate="activateHardStop"
            @release="releaseHardStop"
          />
        </div>

        <RuntimeRiskConfigPanel
          v-else-if="activeTab === 'limits'"
          :loading="updatingRealTradeControlAction === 'runtime-risk.save'"
          :risk-state="realTradeRiskState"
          @disable="disableRuntimeRisk"
          @save="saveRuntimeRisk"
        />

        <StrategyRuntimeRiskSection
          v-else-if="activeTab === 'strategy'"
          :error="strategyRuntimeRiskError"
          :instances="strategyInstances"
          :is-updating="isUpdatingStrategyRuntimeRisk"
          :runtime-risk-for-instance="runtimeRiskForInstance"
          @refresh="loadStrategyInstances"
          @update-mode="updateStrategyRuntimeRiskMode"
        />

        <RiskEventTimeline
          v-else
          :kill-switch-events="realTradeKillSwitchEvents.entries"
          :risk-events="realTradeRiskEvents.entries"
        />
      </div>
    </section>

    <ActionConfirmDialog
      :open="pendingConfirmation != null"
      :title="confirmationView?.title ?? ''"
      :message="confirmationView?.message ?? ''"
      :confirm-label="confirmationView?.confirmLabel ?? '确认'"
      :busy="confirmationBusy"
      @close="pendingConfirmation = null"
      @confirm="confirmPendingAction"
    />

    <ActionConfirmDialog
      :open="pendingRuntimeRiskSave != null"
      title="保存运行时风控配置"
      message="本次变更会开放实盘交易或放宽单笔限额，确认保存吗？"
      confirm-label="确认保存"
      :busy="updatingRealTradeControlAction === 'runtime-risk.save'"
      @close="pendingRuntimeRiskSave = null"
      @confirm="confirmRuntimeRiskSave"
    />
  </div>
</template>

<style scoped>
.risk-page {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  gap: 12px;
  padding: 14px;
  overflow: hidden;
  background:
    radial-gradient(circle at 92% -20%, color-mix(in srgb, var(--tv-accent) 9%, transparent), transparent 36%),
    var(--tv-bg-app);
}

/* ── 左侧态势摘要栏（对齐账户摘要侧栏） ───────────────────── */

.risk-sidebar {
  display: flex;
  width: 264px;
  flex: 0 0 auto;
  flex-direction: column;
  overflow: hidden auto;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, #000 8%, transparent);
  scrollbar-width: thin;
}

.risk-sidebar__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-sidebar__name {
  overflow: hidden;
  color: var(--tv-text);
  font-size: 13px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-sidebar__posture-dot {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-dim));
  font-size: 10px;
}

.risk-sidebar__posture {
  padding: 14px;
  border-bottom: 1px solid var(--tv-border);
}

.risk-sidebar__posture-label {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.risk-sidebar__posture-value {
  margin-top: 4px;
  color: var(--tv-status-fg, var(--tv-text));
  font-size: 26px;
  font-weight: 680;
  letter-spacing: -0.02em;
}

.risk-sidebar__posture-hint {
  margin-top: 6px;
  color: var(--tv-text-dim);
  font-size: 10px;
  line-height: 1.6;
}

.risk-sidebar__rows {
  display: grid;
  gap: 2px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.risk-sidebar__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 0;
  font-size: 12px;
}

.risk-sidebar__row span {
  color: var(--tv-text-muted);
}

.risk-sidebar__row b {
  color: var(--tv-status-fg, var(--tv-text));
  font-weight: 550;
}

.risk-sidebar__facts {
  display: grid;
  gap: 2px;
  padding: 10px 14px 14px;
}

.risk-sidebar__fact {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 3px 0;
  font-size: 11px;
}

.risk-sidebar__fact span {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
}

.risk-sidebar__fact b {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-sidebar__footer {
  margin-top: auto;
  padding: 10px 14px 14px;
}

.risk-sidebar__refresh {
  width: 100%;
  height: 30px;
  font-size: 12px;
}

/* ── 右侧主面板（对齐账户主面板） ───────────────────────── */

.risk-main {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, #000 8%, transparent);
}

.risk-strip {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-strip__section {
  min-width: 0;
  padding: 10px 14px 12px;
  border-left: 1px solid var(--tv-border);
}

.risk-strip__section:first-child {
  border-left: 0;
}

.risk-strip__title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--tv-text-muted);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.risk-strip__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 12px;
}

.risk-strip__item {
  min-width: 0;
}

.risk-strip__item span {
  display: block;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.risk-strip__item b {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  color: var(--tv-status-fg, var(--tv-text));
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risk-main__tabs-row {
  display: flex;
  flex: 0 0 auto;
  align-items: stretch;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-main__tabs {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 2px;
  padding: 5px 7px 0;
  overflow-x: auto;
  scrollbar-width: thin;
}

.risk-main__tabs button {
  position: relative;
  flex: 0 0 auto;
  padding: 8px 14px 9px;
  border: 0;
  border-radius: 6px 6px 0 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 11px;
}

.risk-main__tabs button span {
  margin-left: 4px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.risk-main__tabs button.is-active {
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 650;
}

.risk-main__tabs button.is-active::after {
  position: absolute;
  right: 8px;
  bottom: -1px;
  left: 8px;
  height: 2px;
  background: var(--tv-accent);
  content: "";
}

.risk-main__content {
  display: grid;
  min-height: 0;
  flex: 1;
  align-content: start;
  gap: 12px;
  overflow: auto;
  padding: 12px;
  scrollbar-width: thin;
}

.risk-main__error {
  padding: 8px 11px;
  border: 1px solid;
  border-radius: 6px;
  font-size: 11px;
}

.risk-main__danger-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  padding: 12px;
  border: 1px solid var(--tv-status-error-border);
  border-radius: 10px;
  background: color-mix(
    in srgb,
    var(--tv-status-error-bg) 22%,
    var(--tv-bg-surface)
  );
}

@media (max-width: 1180px) {
  .risk-page {
    flex-direction: column;
    overflow: auto;
  }

  .risk-sidebar {
    width: 100%;
    flex: 0 0 auto;
  }

  .risk-sidebar__facts {
    grid-template-columns: 1fr 1fr;
    column-gap: 16px;
  }

  .risk-main {
    flex: 1 0 auto;
    min-height: 480px;
  }

  .risk-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .risk-strip__section:nth-child(odd) {
    border-left: 0;
  }

  .risk-strip__section:nth-child(n + 3) {
    border-top: 1px solid var(--tv-border);
  }

  .risk-main__danger-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
