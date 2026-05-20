<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import SectionHeader from "../components/SectionHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

interface StrategyDefinitionSummary {
  strategyId: string;
  name: string;
  version: string;
}

interface StrategyInstanceItem {
  id: string;
  definition: StrategyDefinitionSummary;
  params: Record<string, unknown>;
  status: "RUNNING" | "PAUSED" | "STOPPED";
  createdAt: string;
  logs: string[];
}

interface StrategyLogsResponse {
  instanceId: string;
  logs: string[];
}

interface StrategyAuditEntry {
  instanceId: string;
  kind: string;
  detail?: string;
  at: string;
}

interface StrategyAuditResponse {
  instanceId: string;
  entries: StrategyAuditEntry[];
}

interface ApiSuccessEnvelope<T> {
  ok: true;
  data: T;
}

interface ApiErrorEnvelope {
  ok: false;
  error: {
    message: string;
  };
}

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");

function buildApiUrl(path: string): string {
  return apiBaseUrl ? `${apiBaseUrl}${path}` : `http://127.0.0.1:3000${path}`;
}

async function fetchEnvelope<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path));
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "Unknown API error" : body.error.message);
  }

  return body.data;
}

const { systemStatus } = useConsoleData();
const strategies = ref<StrategyInstanceItem[]>([]);
const selectedStrategyId = ref("");
const strategyActiveTab = ref("params");
const strategyLogs = ref<string[]>([]);
const strategyAuditEntries = ref<StrategyAuditEntry[]>([]);
const isLoadingStrategies = ref(false);
const isLoadingDetails = ref(false);
const listError = ref("");
const detailsError = ref("");

const selectedStrategy = computed(
  () =>
    strategies.value.find(
      (strategy) => strategy.id === selectedStrategyId.value,
    ) ?? null,
);

const activeStrategyCount = computed(
  () =>
    strategies.value.filter(
      (strategy) =>
        strategy.status === "RUNNING" || strategy.status === "PAUSED",
    ).length,
);

const selectedStrategyParamsJson = computed(() => {
  if (selectedStrategy.value === null) {
    return "{}";
  }

  return JSON.stringify(selectedStrategy.value.params, null, 2);
});

function formatTimestamp(value: string): string {
  return value.replace("T", " ").replace(".000Z", "Z");
}

async function loadStrategyDetails(instanceId: string): Promise<void> {
  selectedStrategyId.value = instanceId;
  detailsError.value = "";
  isLoadingDetails.value = true;

  try {
    const [logsResponse, auditResponse] = await Promise.all([
      fetchEnvelope<StrategyLogsResponse>(
        `/api/v1/strategies/${encodeURIComponent(instanceId)}/logs`,
      ),
      fetchEnvelope<StrategyAuditResponse>(
        `/api/v1/strategies/${encodeURIComponent(instanceId)}/audit`,
      ),
    ]);

    strategyLogs.value = logsResponse.logs;
    strategyAuditEntries.value = auditResponse.entries;
  } catch (error) {
    strategyLogs.value = [];
    strategyAuditEntries.value = [];
    detailsError.value =
      error instanceof Error
        ? error.message
        : "Failed to load strategy details.";
  } finally {
    isLoadingDetails.value = false;
  }
}

async function loadStrategies(): Promise<void> {
  listError.value = "";
  isLoadingStrategies.value = true;

  try {
    const items =
      await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
    strategies.value = items;

    if (items.length === 0) {
      selectedStrategyId.value = "";
      strategyLogs.value = [];
      strategyAuditEntries.value = [];
      return;
    }

    const fallbackStrategy = items[0];

    if (fallbackStrategy === undefined) {
      selectedStrategyId.value = "";
      strategyLogs.value = [];
      strategyAuditEntries.value = [];
      return;
    }

    const nextSelectedId = items.some(
      (strategy) => strategy.id === selectedStrategyId.value,
    )
      ? selectedStrategyId.value
      : fallbackStrategy.id;

    await loadStrategyDetails(nextSelectedId);
  } catch (error) {
    strategies.value = [];
    selectedStrategyId.value = "";
    strategyLogs.value = [];
    strategyAuditEntries.value = [];
    listError.value =
      error instanceof Error ? error.message : "Failed to load strategies.";
  } finally {
    isLoadingStrategies.value = false;
  }
}

onMounted(() => {
  void loadStrategies();
});
</script>

<template>
  <div class="grid gap-6">
    <SectionHeader
      title="Strategy / Runtime"
      description="把策略实例、参数、日志和审计整理成一个运行时控制台，减少只读信息在多个小卡片之间跳转。"
    />

    <!-- Top stats section -->
    <div class="grid gap-5 lg:grid-cols-[1fr_1fr]">
      <div class="rounded-lg border border-slate-200 bg-white p-4">
        <div class="flex items-center justify-between gap-3">
          <div class="text-xl font-semibold text-slate-900">Strategy Runtime</div>
          <v-chip variant="outlined" size="small">{{ activeStrategyCount }} 活跃</v-chip>
        </div>

        <div class="mt-4 grid gap-4 md:grid-cols-2">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Active Strategies</div>
            <div class="mt-2 text-3xl font-semibold text-slate-900">
              {{ activeStrategyCount }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Selected Strategy</div>
            <div class="mt-2 text-xl font-semibold text-slate-900">
              {{ selectedStrategy?.definition.name ?? 'N/A' }}
            </div>
          </div>
        </div>

        <div class="mt-4 grid gap-3 text-sm text-slate-600">
          <div>
            运行模式：<span class="font-medium text-slate-900">{{ systemStatus.defaultTradingEnvironment }}</span>
          </div>
          <div>
            当前已接通最小只读路径：实例列表、日志与审计明细可查询。
          </div>
        </div>
      </div>

      <div class="rounded-lg border border-slate-200 bg-white p-4">
        <div class="text-xl font-semibold text-slate-900">Strategy Configuration</div>

        <div class="mt-4 grid gap-4">
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Real Trade Enabled</div>
            <div class="mt-2 text-xl font-semibold text-slate-900">
              {{ systemStatus.realTradingEnabled ? 'YES' : 'NO' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Kill Switch</div>
            <div class="mt-2 text-xl font-semibold" :class="systemStatus.realTradingKillSwitch.active ? 'text-red-600' : 'text-teal-700'">
              {{ systemStatus.realTradingKillSwitch.active ? 'ACTIVE' : 'INACTIVE' }}
            </div>
          </div>
          <div class="rounded-3xl bg-slate-50 px-4 py-4">
            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Max Order Qty</div>
            <div class="mt-2 text-xl font-semibold text-slate-900">
              {{ systemStatus.realTradingRisk.maxOrderQuantity ?? 'N/A' }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Master-detail layout -->
    <div class="grid gap-4 grid-cols-12">
      <!-- Left: Strategy list (col-span-4 lg:col-span-3) -->
      <div class="col-span-12 lg:col-span-3 rounded-lg border border-slate-200 bg-white p-4">
        <div class="flex items-center justify-between gap-3 mb-4">
          <div class="text-xl font-semibold text-slate-900">Strategy Instances</div>
          <button
            class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
            type="button"
            @click="loadStrategies"
          >
            {{ isLoadingStrategies ? '刷新中…' : '刷新列表' }}
          </button>
        </div>

        <div v-if="listError" class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {{ listError }}
        </div>

        <v-empty-state v-else-if="strategies.length === 0" text="暂无策略实例。" />

        <div v-else class="grid gap-3">
          <button
            v-for="strategy in strategies"
            :key="strategy.id"
            :data-testid="`strategy-${strategy.id}`"
            class="rounded-3xl border px-4 py-4 text-left transition"
            :class="strategy.id === selectedStrategyId
              ? 'border-teal-400 bg-teal-50 text-teal-950'
              : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300'"
            type="button"
            @click="loadStrategyDetails(strategy.id)"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="text-base font-semibold">{{ strategy.definition.name }}</div>
              <div class="text-xs uppercase tracking-[0.2em]">{{ strategy.status }}</div>
            </div>
            <div class="mt-2 text-sm text-slate-500">{{ strategy.id }}</div>
            <div class="mt-2 text-sm text-slate-500">创建于 {{ formatTimestamp(strategy.createdAt) }}</div>
          </button>
        </div>
      </div>

      <!-- Right: Detail view in tabs (col-span-8 lg:col-span-9) -->
      <div class="col-span-12 lg:col-span-9 rounded-lg border border-slate-200 bg-white p-4">
        <v-tabs v-model="strategyActiveTab" bg-color="transparent">
          <v-tab value="params">参数 / Parameters</v-tab>
          <v-tab value="logs">运行日志 / Run Log</v-tab>
          <v-tab value="audit">Strategy Audit / 关联订单</v-tab>
        </v-tabs>

        <v-window v-model="strategyActiveTab" class="mt-2">
          <!-- Tab 1: Parameters -->
          <v-window-item value="params">
            <div v-if="detailsError" class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {{ detailsError }}
            </div>

            <v-empty-state v-else-if="selectedStrategy === null" text="请选择策略查看参数。" />

            <div v-else class="rounded-3xl bg-slate-50 px-4 py-4">
              <pre class="overflow-x-auto whitespace-pre-wrap text-xs leading-6 text-slate-700">{{ selectedStrategyParamsJson }}</pre>
            </div>
          </v-window-item>

          <!-- Tab 2: Run Log -->
          <v-window-item value="logs">
            <div v-if="detailsError" class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {{ detailsError }}
            </div>

            <v-empty-state v-else-if="selectedStrategy === null" text="请选择策略查看日志。" />

            <v-skeleton-loader v-else-if="isLoadingDetails" type="list-item-three-line" />

            <ul v-else-if="strategyLogs.length > 0" class="grid gap-3 text-sm text-slate-700">
              <li v-for="entry in strategyLogs" :key="entry" class="rounded-3xl bg-slate-50 px-4 py-3 font-mono leading-6">
                {{ entry }}
              </li>
            </ul>

            <v-empty-state v-else text="暂无数据" />
          </v-window-item>

          <!-- Tab 3: Linked Orders / Audit -->
          <v-window-item value="audit">
            <v-empty-state v-if="selectedStrategy === null" text="请选择策略查看审计。" />

            <ul v-else-if="strategyAuditEntries.length > 0" class="grid gap-3 text-sm text-slate-700">
              <li
                v-for="entry in strategyAuditEntries"
                :key="`${entry.at}-${entry.kind}-${entry.detail ?? ''}`"
                class="rounded-3xl bg-slate-50 px-4 py-4"
              >
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <span class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">{{ entry.kind }}</span>
                  <span class="text-xs text-slate-500">{{ formatTimestamp(entry.at) }}</span>
                </div>
                <div class="mt-2 text-sm font-medium text-slate-900">{{ entry.detail ?? 'lifecycle transition' }}</div>
              </li>
            </ul>

            <v-empty-state v-else text="暂无数据" />
          </v-window-item>
        </v-window>
      </div>
    </div>
  </div>
</template>
