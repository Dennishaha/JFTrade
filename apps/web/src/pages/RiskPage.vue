<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import type {
  RealTradeHardStopsResponse,
  RealTradeKillSwitchEventsResponse,
  RealTradeRiskEventsResponse,
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "../contracts";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import RuntimeRiskGateSection from "../components/risk/RuntimeRiskGateSection.vue";
import {
  formatDateTime,
  formatGenericStatusLabel,
  formatMarketLabel,
  formatRealTradeEventTypeLabel,
  formatRealTradeHardStopScope,
  formatRealTradeKillSwitchSource,
  formatRealTradeRiskSource,
  formatTradingEnvironment,
  resolveRealTradeHardStopScopeTagType,
  resolveRealTradeKillSwitchEventTagType,
  resolveRealTradeRiskEventTagType,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import {
  formatStrategyRuntimeRiskSummary,
  normalizeStrategyRuntimeRiskSettings,
} from "../components/strategy-runtime/strategyRuntimeInstanceBinding";

const {
  loadSystemState,
  realTradeHardStops,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  systemStatus,
} = useConsoleData();

onMounted(() => {
  void Promise.all([loadSystemState({ bypassCooldown: true }), loadStrategyInstances()]);
});

const strategyInstances = ref<StrategyInstanceItem[]>([]);
const strategyRuntimeRiskError = ref("");
const updatingStrategyRuntimeRiskIds = ref<string[]>([]);
const realTradeControlError = ref("");
const updatingRealTradeControlAction = ref("");
const hardStopForm = ref({
  brokerId: "futu",
  tradingEnvironment: "REAL",
  accountId: "",
  market: "",
  symbol: "",
  hardStopScope: "ACCOUNT",
  operatorId: "local",
  reason: "",
});

type RealTradeHardStopEntry = RealTradeHardStopsResponse["entries"][number];
type RealTradeKillSwitchEventEntry =
  RealTradeKillSwitchEventsResponse["entries"][number];
type RealTradeRiskEventEntry = RealTradeRiskEventsResponse["entries"][number];

function arrayOrEmpty<T>(items: T[] | null | undefined): T[] {
  return Array.isArray(items) ? items : [];
}

const strategyInstancesById = computed(
  () => new Map(strategyInstances.value.map((item) => [item.id, item])),
);
const realTradeHardStopEntries = computed<RealTradeHardStopEntry[]>(() =>
  arrayOrEmpty(realTradeHardStops.value.entries),
);
const realTradeKillSwitchEventEntries =
  computed<RealTradeKillSwitchEventEntry[]>(() =>
    arrayOrEmpty(realTradeKillSwitchEvents.value.entries),
  );
const realTradeRiskEventEntries = computed<RealTradeRiskEventEntry[]>(() =>
  arrayOrEmpty(realTradeRiskEvents.value.entries),
);
const riskHeaderStats = computed(() => [
  {
    label: "实盘总闸",
    value: systemStatus.value.realTradingEnabled ? "已开放" : "未开放",
  },
  {
    label: "风控限额",
    value: realTradeRiskState.value.riskEnabled ? "已配置" : "未配置",
  },
  {
    label: "熔断",
    value: realTradeKillSwitchState.value.killSwitchActive ? "已激活" : "未激活",
  },
  {
    label: "硬停止",
    value: `${realTradeHardStopEntries.value.length}`,
  },
]);
const runtimeGateItems = computed(() => [
  {
    key: "JFTRADE_ALLOW_REAL_TRADING",
    label: "实盘总闸",
    value: systemStatus.value.realTradingEnabled ? "true" : "false",
    status: systemStatus.value.realTradingEnabled ? "已开启" : "未开启",
    active: systemStatus.value.realTradingEnabled,
  },
  {
    key: "JFTRADE_REAL_TRADE_MAX_ORDER_QUANTITY",
    label: "最大订单数量",
    value:
      realTradeRiskState.value.envConfiguredMaxOrderQuantity == null
        ? "未设置"
        : String(realTradeRiskState.value.envConfiguredMaxOrderQuantity),
    status:
      realTradeRiskState.value.envConfiguredMaxOrderQuantity == null
        ? "未配置"
        : "已配置",
    active: realTradeRiskState.value.envConfiguredMaxOrderQuantity != null,
  },
  {
    key: "JFTRADE_REAL_TRADE_MAX_ORDER_NOTIONAL",
    label: "最大订单名义金额",
    value:
      realTradeRiskState.value.envConfiguredMaxOrderNotional == null
        ? "未设置"
        : String(realTradeRiskState.value.envConfiguredMaxOrderNotional),
    status:
      realTradeRiskState.value.envConfiguredMaxOrderNotional == null
        ? "未配置"
        : "已配置",
    active: realTradeRiskState.value.envConfiguredMaxOrderNotional != null,
  },
]);
const launchExample = computed(() => {
  const quantity =
    realTradeRiskState.value.envConfiguredMaxOrderQuantity ??
    realTradeRiskState.value.effectiveMaxOrderQuantity ??
    100;
  const notional =
    realTradeRiskState.value.envConfiguredMaxOrderNotional ??
    realTradeRiskState.value.effectiveMaxOrderNotional ??
    10000;
  return [
    "JFTRADE_ALLOW_REAL_TRADING=true \\",
    `JFTRADE_REAL_TRADE_MAX_ORDER_QUANTITY=${quantity} \\`,
    `JFTRADE_REAL_TRADE_MAX_ORDER_NOTIONAL=${notional} \\`,
    "./start.sh",
  ].join("\n");
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
  value: unknown,
): Promise<void> {
  const mode: StrategyRuntimeRiskMode =
    value === "monitor" || value === "enforce" ? value : "off";
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
    await loadSystemState({ bypassCooldown: true });
  } catch (error) {
    realTradeControlError.value =
      error instanceof Error ? error.message : "更新实盘控制面失败。";
  } finally {
    updatingRealTradeControlAction.value = "";
  }
}

function activateKillSwitch(): Promise<void> {
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

function releaseKillSwitch(): Promise<void> {
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

function activateHardStop(): Promise<void> {
  const payload = {
    ...hardStopForm.value,
    market: hardStopForm.value.market.trim().toUpperCase(),
    symbol: hardStopForm.value.symbol.trim().toUpperCase(),
  };
  return runRealTradeControlAction(
    "hard-stop.activate",
    "/api/v1/system/real-trade-hard-stops",
    payload,
  );
}

function releaseHardStop(id: string): Promise<void> {
  return runRealTradeControlAction(
    `hard-stop.release.${id}`,
    `/api/v1/system/real-trade-hard-stops/${encodeURIComponent(id)}/release`,
    {
      operatorId: "local",
      reason: "manual release from RiskPage",
    },
  );
}

function isRealTradeControlUpdating(action: string): boolean {
  return updatingRealTradeControlAction.value === action;
}

function formatStrategyRuntimeStatus(status: string): string {
  switch (status) {
    case "RUNNING":
      return "运行中";
    case "PAUSED":
      return "已暂停";
    case "STOPPED":
      return "已停止";
    default:
      return status || "未知";
  }
}
</script>

<template>
  <div class="risk-page grid min-w-0 gap-6">
    <PageHeader
      eyebrow="风控"
      title="运行时风控"
      description="实盘总闸和订单限额只由 API 进程启动时的运行时配置决定；页面提供状态核对、熔断和硬停止操作。"
      :stats="riskHeaderStats"
    />

    <RuntimeRiskGateSection
      :gates="runtimeGateItems"
      :launch-example="launchExample"
    />

    <section class="mb-5">
      <v-card flat class="card-shell border-0">
        <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
          <div>
            <div class="text-xl font-semibold text-slate-900">策略实例动态风控</div>
            <div class="mt-1 text-sm text-slate-500">
              可直接切换关闭、观察或执行模式，运行中的实例无需停止。
            </div>
          </div>
          <v-btn variant="text" color="primary" size="small" @click="loadStrategyInstances()">
            刷新
          </v-btn>
        </div>
        <v-card-text>
          <v-alert
            v-if="strategyRuntimeRiskError"
            type="warning"
            variant="tonal"
            density="compact"
            :closable="false"
            class="mb-3"
          >
            {{ strategyRuntimeRiskError }}
          </v-alert>
          <div v-if="strategyInstances.length" class="grid gap-3 lg:grid-cols-2">
            <div
              v-for="instance in strategyInstances"
              :key="instance.id"
              class="grid gap-3 rounded-lg border border-slate-200 bg-white px-4 py-4 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center"
            >
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <div class="font-semibold text-slate-900">{{ instance.definition.name }}</div>
                  <v-chip
                    :color="instance.status === 'RUNNING' ? 'success' : instance.status === 'PAUSED' ? 'warning' : undefined"
                    variant="outlined"
                    size="small"
                  >
                    {{ formatStrategyRuntimeStatus(instance.status) }}
                  </v-chip>
                </div>
                <div class="mt-1 truncate text-xs text-slate-500">{{ instance.id }}</div>
                <div class="mt-2 text-xs font-medium text-slate-700">
                  {{ formatStrategyRuntimeRiskSummary(runtimeRiskForInstance(instance.id)) }}
                </div>
              </div>
              <select
                :value="runtimeRiskForInstance(instance.id).mode"
                :disabled="isUpdatingStrategyRuntimeRisk(instance.id)"
                class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none disabled:cursor-wait disabled:opacity-60"
                :aria-label="`${instance.definition.name} 动态风控模式`"
                @change="updateStrategyRuntimeRiskMode(instance.id, ($event.target as HTMLSelectElement).value)"
              >
                <option value="off">关闭</option>
                <option value="monitor">观察</option>
                <option value="enforce">执行</option>
              </select>
            </div>
          </div>
          <v-empty-state v-else text="当前没有策略实例。创建策略实例后可在这里控制动态风控。" />
        </v-card-text>
      </v-card>
    </section>

    <section class="grid gap-5 lg:grid-cols-3">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">实盘熔断开关</div>
          <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
            {{ realTradeKillSwitchState.killSwitchActive ? '熔断开启' : '熔断关闭' }}
          </v-chip>
        </div>
        <v-card-text>
          <div class="rounded-lg bg-slate-50 px-3 py-3">
            <div class="flex items-center justify-between gap-3">
              <div class="font-medium text-slate-900">
                {{ formatRealTradeKillSwitchSource(realTradeKillSwitchState.killSwitchSource) }}
              </div>
              <v-chip :color="realTradeKillSwitchState.killSwitchActive ? 'error' : undefined" variant="outlined" size="small">
                {{ formatGenericStatusLabel(realTradeKillSwitchState.killSwitchActive ? 'ACTIVE' : 'CLEAR') }}
              </v-chip>
            </div>
            <div class="mt-1 text-xs text-slate-500">
              环境变量 {{ formatGenericStatusLabel(realTradeKillSwitchState.envConfiguredActive ? 'ON' : 'OFF') }} / 控制面 {{ formatGenericStatusLabel(realTradeKillSwitchState.controlPlaneActive ? 'ON' : 'OFF') }}
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <v-btn
              color="error"
              size="small"
              variant="outlined"
              :loading="isRealTradeControlUpdating('kill-switch.activate')"
              @click="activateKillSwitch"
            >
              激活控制面熔断
            </v-btn>
            <v-btn
              size="small"
              variant="outlined"
              :loading="isRealTradeControlUpdating('kill-switch.release')"
              @click="releaseKillSwitch"
            >
              解除控制面熔断
            </v-btn>
          </div>
          <div v-if="realTradeKillSwitchEventEntries.length" class="mt-3 grid gap-2">
            <div
              v-for="item in realTradeKillSwitchEventEntries.slice(0, 3)"
              :key="item.id"
              class="rounded-lg bg-slate-50 px-3 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium text-slate-900">{{ formatRealTradeEventTypeLabel(item.eventType) }} / {{ item.brokerId }}</div>
                <v-chip :color="resolveRealTradeKillSwitchEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeKillSwitchEventTagType(item.eventType)" variant="outlined" size="small">
                  {{ formatRealTradeEventTypeLabel(item.eventType) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">{{ formatDateTime(item.createdAt) }}</div>
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">实盘订单限额</div>
          <v-chip :color="realTradeRiskState.riskEnabled ? 'warning' : undefined" variant="outlined" size="small">
            {{ realTradeRiskState.riskEnabled ? '限额开启' : '限额关闭' }}
          </v-chip>
        </div>
        <v-card-text>
          <div class="rounded-lg bg-slate-50 px-3 py-3">
            <div class="font-medium text-slate-900">{{ formatRealTradeRiskSource(realTradeRiskState.riskConfigSource) }}</div>
            <div class="mt-1 text-xs text-slate-500">
              有效数量 {{ realTradeRiskState.effectiveMaxOrderQuantity ?? '暂无' }} / 有效金额 {{ realTradeRiskState.effectiveMaxOrderNotional ?? '暂无' }}
            </div>
            <div class="mt-1 text-xs text-slate-500">
              ENV 数量 {{ realTradeRiskState.envConfiguredMaxOrderQuantity ?? '暂无' }} / ENV 金额 {{ realTradeRiskState.envConfiguredMaxOrderNotional ?? '暂无' }}
            </div>
          </div>
          <div v-if="realTradeRiskEventEntries.length" class="mt-3 grid gap-2">
            <div
              v-for="item in realTradeRiskEventEntries.slice(0, 3)"
              :key="item.id"
              class="rounded-lg bg-slate-50 px-3 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium text-slate-900">{{ formatRealTradeEventTypeLabel(item.eventType) }} / {{ item.brokerId }}</div>
                <v-chip :color="resolveRealTradeRiskEventTagType(item.eventType) === 'danger' ? 'error' : resolveRealTradeRiskEventTagType(item.eventType)" variant="outlined" size="small">
                  {{ formatRealTradeEventTypeLabel(item.eventType) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">{{ item.reason ?? '暂无' }}</div>
            </div>
          </div>
        </v-card-text>
      </v-card>

      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">实盘硬停止</div>
          <v-chip :color="realTradeHardStopEntries.length ? 'error' : undefined" variant="outlined" size="small">
            {{ formatGenericStatusLabel(realTradeHardStopEntries.length ? 'ACTIVE' : 'CLEAR') }}
          </v-chip>
        </div>
        <v-card-text>
          <v-alert
            v-if="realTradeControlError"
            type="warning"
            variant="tonal"
            density="compact"
            class="mb-3"
          >
            {{ realTradeControlError }}
          </v-alert>
          <div class="mb-3 grid gap-2 rounded-lg border border-slate-200 bg-white px-3 py-3 text-sm">
            <div class="grid gap-2 sm:grid-cols-2">
              <input
                v-model="hardStopForm.accountId"
                class="rounded-lg border border-slate-300 px-3 py-2 outline-none"
                placeholder="账户 ID，空为全部"
                aria-label="硬停止账户 ID"
              />
              <select
                v-model="hardStopForm.hardStopScope"
                class="rounded-lg border border-slate-300 bg-white px-3 py-2 outline-none"
                aria-label="硬停止范围"
              >
                <option value="ACCOUNT">账户</option>
                <option value="MARKET">市场</option>
                <option value="SYMBOL">标的</option>
              </select>
              <input
                v-model="hardStopForm.market"
                class="rounded-lg border border-slate-300 px-3 py-2 uppercase outline-none"
                placeholder="市场，如 US"
                aria-label="硬停止市场"
              />
              <input
                v-model="hardStopForm.symbol"
                class="rounded-lg border border-slate-300 px-3 py-2 uppercase outline-none"
                placeholder="标的，如 AAPL"
                aria-label="硬停止标的"
              />
            </div>
            <input
              v-model="hardStopForm.reason"
              class="rounded-lg border border-slate-300 px-3 py-2 outline-none"
              placeholder="原因"
              aria-label="硬停止原因"
            />
            <div>
              <v-btn
                color="error"
                size="small"
                variant="outlined"
                :loading="isRealTradeControlUpdating('hard-stop.activate')"
                @click="activateHardStop"
              >
                创建硬停止
              </v-btn>
            </div>
          </div>
          <div v-if="realTradeHardStopEntries.length" class="grid gap-2">
            <div
              v-for="item in realTradeHardStopEntries.slice(0, 3)"
              :key="item.id"
              class="rounded-lg bg-slate-50 px-3 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium text-slate-900">{{ item.brokerId }} / {{ item.accountId }}</div>
                <v-chip :color="resolveRealTradeHardStopScopeTagType(item) === 'danger' ? 'error' : resolveRealTradeHardStopScopeTagType(item)" variant="outlined" size="small">
                  {{ formatRealTradeHardStopScope(item) }}
                </v-chip>
              </div>
              <div class="mt-1 text-xs text-slate-500">
                {{ formatTradingEnvironment(item.tradingEnvironment) }} / {{ formatMarketLabel(item.market ?? '') }} / 操作员 {{ item.operatorId }}
              </div>
              <div class="mt-1 text-xs text-slate-700">{{ item.reason }}</div>
              <div class="mt-2">
                <v-btn
                  size="small"
                  variant="outlined"
                  :loading="isRealTradeControlUpdating(`hard-stop.release.${item.id}`)"
                  @click="releaseHardStop(item.id)"
                >
                  解除硬停止
                </v-btn>
              </div>
            </div>
          </div>
          <div v-else class="text-sm text-slate-500">暂无活跃实盘硬停止。</div>
        </v-card-text>
      </v-card>
    </section>
  </div>
</template>
