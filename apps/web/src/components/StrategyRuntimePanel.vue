<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { StrategyInstanceItem, StrategySourceFormat } from "@jftrade/ui-contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { useConsoleData } from "../composables/useConsoleData";

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

type StrategyAction = "start" | "pause" | "stop";

const props = defineProps<{
    /** 设计阶段当前选中的定义数量，供头部统计展示 */
    definitionsCount?: number;
}>();

const emit = defineEmits<{
    "switch-to-design": [payload?: { mode?: "existing" | "new" }];
}>();

const { systemStatus } = useConsoleData();

const strategies = ref<StrategyInstanceItem[]>([]);
const selectedStrategyId = ref("");
const strategyLogs = ref<string[]>([]);
const strategyAuditEntries = ref<StrategyAuditEntry[]>([]);
const isLoadingStrategies = ref(false);
const isLoadingDetails = ref(false);
const listError = ref("");
const detailsError = ref("");

const selectedStrategy = computed(
    () => strategies.value.find((item) => item.id === selectedStrategyId.value) ?? null,
);

const activeStrategyCount = computed(
    () => strategies.value.filter((item) => item.status !== "STOPPED").length,
);

const selectedStrategyParamsJson = computed(() => {
    if (selectedStrategy.value === null) return "";
    return JSON.stringify(selectedStrategy.value.params, null, 2);
});

const selectedStrategyRuntimeLabel = computed(() => {
    if (selectedStrategy.value === null) return "暂无";
    return formatStrategyRuntime(selectedStrategy.value.runtime);
});

const selectedStrategySourceFormatLabel = computed(() => {
    if (selectedStrategy.value === null) return "暂无";
    return formatSourceFormat(selectedStrategy.value.sourceFormat);
});

const selectedStrategyStartHint = computed(() => {
    if (selectedStrategy.value === null) return "请选择策略实例。";
    if (selectedStrategy.value.startable) {
        return "当前实例已接入策略控制面生命周期，可启动、暂停、停止。";
    }
    if (selectedStrategy.value.runtime === "dsl-go-plan") {
        return "当前实例已完成 DSL 编译与 requirements 规划，但暂不可启动。";
    }
    return "当前实例暂不可启动。";
});

const selectedStrategyCompiledSummary = computed(() => {
    if (selectedStrategy.value === null || selectedStrategy.value.runtime !== "dsl-go-plan") {
        return "";
    }
    const hookCount = readCompiledHookCount(selectedStrategy.value);
    const indicatorCount = readCompiledIndicatorCount(selectedStrategy.value);
    const parts: string[] = [];
    if (hookCount !== null) parts.push(`${hookCount} 个 hook`);
    if (indicatorCount !== null) parts.push(`${indicatorCount} 项依赖`);
    if (parts.length === 0) return "已完成 DSL 编译计划。";
    return `已完成 DSL 编译计划，包含 ${parts.join(" / ")}。`;
});

const canStartSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status !== "RUNNING",
);

const canPauseSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status === "RUNNING",
);

const canStopSelectedStrategy = computed(
    () =>
        selectedStrategy.value !== null
        && !isLoadingDetails.value
        && selectedStrategy.value.startable
        && selectedStrategy.value.status !== "STOPPED",
);

onMounted(() => {
    void loadStrategies();
});

function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
}

function formatTimestamp(value: unknown): string {
    const normalized = normalizeText(value);
    if (normalized === "") return "暂无";
    return normalized.replace("T", " ").replace(".000Z", "Z");
}

function formatStrategyStatus(status: StrategyInstanceItem["status"] | string): string {
    switch (status) {
        case "RUNNING":
            return "运行中";
        case "PAUSED":
            return "已暂停";
        case "STOPPED":
            return "已停止";
        default:
            return normalizeText(status) || "未知";
    }
}

function formatStrategyRuntime(runtime: unknown): string {
    switch (normalizeText(runtime)) {
        case "dsl-go-plan":
            return "DSL 编译计划";
        default:
            return "未知 / 受限";
    }
}

function formatSourceFormat(sourceFormat: StrategySourceFormat | string | null | undefined): string {
    switch (normalizeText(sourceFormat)) {
        case "dsl-v1":
            return "DSL v1";
        default:
            return "未知 / 受限";
    }
}

function formatExecutionMode(strategy: StrategyInstanceItem): string {
    if (strategy.startable) return "可启动";
    if (strategy.runtime === "dsl-go-plan") return "待启用";
    return "受限";
}

function readCompiledIndicatorCount(strategy: StrategyInstanceItem): number | null {
    const compiledRequirements = asRecord(strategy.params.compiledRequirements);
    if (compiledRequirements === null) return null;
    return Array.isArray(compiledRequirements.indicators)
        ? compiledRequirements.indicators.length
        : null;
}

function readCompiledHookCount(strategy: StrategyInstanceItem): number | null {
    return Array.isArray(strategy.params.compiledHooks)
        ? strategy.params.compiledHooks.length
        : null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
        return null;
    }
    return value as Record<string, unknown>;
}

function formatAuditKind(kind: unknown): string {
    switch (normalizeText(kind).toLowerCase()) {
        case "created":
            return "已创建";
        case "started":
            return "已启动";
        case "running":
            return "运行中";
        case "paused":
            return "已暂停";
        case "stopped":
            return "已停止";
        case "failed":
            return "执行失败";
        default:
            return normalizeText(kind) || "未知";
    }
}

function formatActionLabel(action: StrategyAction): string {
    switch (action) {
        case "start":
            return "启动";
        case "pause":
            return "暂停";
        case "stop":
            return "停止";
        default:
            return action;
    }
}

function clearRuntimeDetails(): void {
    strategyLogs.value = [];
    strategyAuditEntries.value = [];
}

async function loadStrategies(preferredId = selectedStrategyId.value): Promise<void> {
    isLoadingStrategies.value = true;
    listError.value = "";

    try {
        const items = await fetchEnvelope<StrategyInstanceItem[]>("/api/v1/strategies");
        strategies.value = items;

        if (items.length === 0) {
            selectedStrategyId.value = "";
            clearRuntimeDetails();
            return;
        }

        const nextId =
            items.find((item) => item.id === preferredId)?.id ?? items[0]?.id ?? "";

        if (nextId !== "") {
            void loadStrategyDetails(nextId);
        }
    } catch (error) {
        strategies.value = [];
        selectedStrategyId.value = "";
        clearRuntimeDetails();
        listError.value =
            error instanceof Error ? error.message : "加载策略实例失败。";
    } finally {
        isLoadingStrategies.value = false;
    }
}

async function loadStrategyDetails(instanceId: string): Promise<void> {
    selectedStrategyId.value = instanceId;
    detailsError.value = "";
    isLoadingDetails.value = true;

    try {
        const [logs, audit] = await Promise.all([
            fetchEnvelope<StrategyLogsResponse>(
                `/api/v1/strategies/${encodeURIComponent(instanceId)}/logs`,
            ),
            fetchEnvelope<StrategyAuditResponse>(
                `/api/v1/strategies/${encodeURIComponent(instanceId)}/audit`,
            ),
        ]);

        strategyLogs.value = logs.logs;
        strategyAuditEntries.value = audit.entries;
    } catch (error) {
        clearRuntimeDetails();
        detailsError.value =
            error instanceof Error ? error.message : "加载策略明细失败。";
    } finally {
        isLoadingDetails.value = false;
    }
}

async function changeStrategyStatus(action: StrategyAction): Promise<void> {
    detailsError.value = "";

    if (selectedStrategy.value === null) {
        detailsError.value = "请先选择策略实例。";
        return;
    }

    isLoadingDetails.value = true;

    try {
        await fetchEnvelopeWithInit<StrategyInstanceItem>(
            `/api/v1/strategies/${encodeURIComponent(selectedStrategy.value.id)}/${action}`,
            {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({}),
            },
        );
        await loadStrategies(selectedStrategy.value.id);
    } catch (error) {
        detailsError.value =
            error instanceof Error ? error.message : `执行${formatActionLabel(action)}失败。`;
    } finally {
        isLoadingDetails.value = false;
    }
}
</script>

<template>
    <div class="runtime-panel">
        <!-- 头部 -->
        <div class="runtime-panel__bar">
            <div class="runtime-panel__intro">
                <div class="runtime-panel__eyebrow">策略运行时</div>
                <div class="runtime-panel__title-row">
                    <div class="runtime-panel__title">策略运行</div>
                </div>
            </div>

            <div class="runtime-panel__bar-actions">
                <div class="inline-flex rounded-full border border-slate-200 bg-slate-50 p-1">
                    <button
                        class="rounded-full px-4 py-2 text-sm font-medium text-slate-600 transition hover:text-slate-900"
                        data-testid="strategy-workspace-tab-design" type="button"
                        @click="emit('switch-to-design', { mode: 'existing' })">
                        策略设计
                    </button>
                    <button
                        class="rounded-full bg-slate-900 px-4 py-2 text-sm font-medium text-white shadow-sm transition"
                        data-testid="strategy-workspace-tab-runtime" type="button">
                        策略运行
                    </button>
                </div>

                <div class="runtime-panel__metrics">
                    <div class="runtime-panel__metric-chip">{{ activeStrategyCount }} 个活跃实例</div>
                    <div class="runtime-panel__metric-chip">
                        {{ props.definitionsCount ?? 0 }} 个策略定义
                    </div>
                </div>
            </div>
        </div>

        <!-- 内容区 -->
        <div class="runtime-panel__scroll">
            <div class="grid gap-4 lg:grid-cols-[1fr_1fr]">
                <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
                    <div class="flex items-center justify-between gap-3">
                        <div class="text-xl font-semibold text-slate-900">运行总览</div>
                        <span
                            class="rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-slate-600">
                            {{ activeStrategyCount }} 个运行中
                        </span>
                    </div>
                    <div class="mt-4 grid gap-3 md:grid-cols-3">
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">交易环境</div>
                            <div class="mt-2 text-2xl font-semibold text-slate-900">
                                {{ systemStatus.defaultTradingEnvironment }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">当前策略</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ selectedStrategy?.definition.name ?? "暂无" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">运行形态</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900" data-testid="strategy-runtime-mode">
                                {{ selectedStrategyRuntimeLabel }}
                            </div>
                        </div>
                    </div>
                    <div class="mt-4 text-sm text-slate-600">
                        运行面板负责实例控制、日志和审计；新策略统一使用 DSL 编译计划生命周期。
                    </div>
                </div>

                <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
                    <div class="text-xl font-semibold text-slate-900">运行保护</div>
                    <div class="mt-4 grid gap-3 md:grid-cols-3">
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">实盘开关</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ systemStatus.realTradingEnabled ? "已开启" : "已关闭" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">急停开关</div>
                            <div class="mt-2 text-xl font-semibold"
                                :class="systemStatus.realTradingKillSwitch.active ? 'text-red-600' : 'text-teal-700'">
                                {{ systemStatus.realTradingKillSwitch.active ? "已启用" : "未启用" }}
                            </div>
                        </div>
                        <div class="rounded-3xl bg-white px-4 py-4">
                            <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最大下单数量</div>
                            <div class="mt-2 text-xl font-semibold text-slate-900">
                                {{ systemStatus.realTradingRisk.maxOrderQuantity ?? "暂无" }}
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
                <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                    <div class="mb-4 flex items-center justify-between gap-3">
                        <div class="text-xl font-semibold text-slate-900">策略实例</div>
                        <div class="flex flex-wrap items-center gap-2">
                            <button
                                class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                data-testid="strategy-new-definition" type="button"
                                @click="emit('switch-to-design', { mode: 'new' })">
                                新增
                            </button>
                            <button
                                class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                                type="button" @click="loadStrategies()">
                                {{ isLoadingStrategies ? "等待" : "刷新" }}
                            </button>
                        </div>
                    </div>
                    <div v-if="listError"
                        class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                        {{ listError }}
                    </div>
                    <div v-else-if="strategies.length === 0"
                        class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                        暂无策略实例。先从设计区保存定义并创建运行实例。
                    </div>
                    <div v-else class="grid gap-3">
                        <button v-for="strategy in strategies" :key="strategy.id"
                            :data-testid="`strategy-${strategy.id}`" class="strategy-list-card"
                            :class="{ 'is-active': strategy.id === selectedStrategyId }" type="button"
                            @click="loadStrategyDetails(strategy.id)">
                            <div class="flex items-center justify-between gap-3">
                                <div class="text-base font-semibold">{{ strategy.definition.name }}</div>
                                <div class="text-xs font-semibold uppercase tracking-[0.18em]">{{
                                    formatStrategyStatus(strategy.status) }}</div>
                            </div>
                            <div class="mt-2 text-sm text-slate-500">{{ strategy.id }}</div>
                            <div class="mt-2 text-sm text-slate-500">创建于 {{ formatTimestamp(strategy.createdAt) }}</div>
                            <div class="mt-3 flex flex-wrap gap-2 text-[11px] font-semibold uppercase tracking-[0.16em]">
                                <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
                                    {{ formatStrategyRuntime(strategy.runtime) }}
                                </span>
                                <span class="rounded-full border border-slate-200 bg-slate-100 px-2.5 py-1 text-slate-600">
                                    {{ formatSourceFormat(strategy.sourceFormat) }}
                                </span>
                                <span
                                    class="rounded-full px-2.5 py-1"
                                    :class="strategy.startable
                                        ? 'border border-emerald-200 bg-emerald-50 text-emerald-700'
                                        : 'border border-amber-200 bg-amber-50 text-amber-700'">
                                    {{ formatExecutionMode(strategy) }}
                                </span>
                            </div>
                        </button>
                    </div>
                </div>

                <div class="grid gap-4">
                    <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                        <div class="flex flex-wrap items-center justify-between gap-3">
                            <div>
                                <div class="text-xl font-semibold text-slate-900">运行控制</div>
                                <div class="mt-1 text-sm text-slate-500">
                                    启动、暂停、停止都会同步刷新日志与审计视图。
                                </div>
                            </div>
                            <div v-if="selectedStrategy !== null" class="rounded-3xl bg-slate-50 px-4 py-4">
                                <div class="flex flex-wrap gap-2">
                                    <span class="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-600">
                                        {{ selectedStrategyRuntimeLabel }}
                                    </span>
                                    <span class="rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-600">
                                        {{ selectedStrategySourceFormatLabel }}
                                    </span>
                                    <span
                                        class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em]"
                                        :class="selectedStrategy.startable
                                            ? 'border border-emerald-200 bg-emerald-50 text-emerald-700'
                                            : 'border border-amber-200 bg-amber-50 text-amber-700'">
                                        {{ formatExecutionMode(selectedStrategy) }}
                                    </span>
                                </div>
                                <div class="mt-3 text-sm text-slate-600" data-testid="strategy-runtime-start-hint">
                                    {{ selectedStrategyStartHint }}
                                </div>
                                <div v-if="selectedStrategyCompiledSummary" class="mt-2 text-xs text-slate-500">
                                    {{ selectedStrategyCompiledSummary }}
                                </div>
                            </div>
                            <div class="flex flex-wrap gap-2">
                                <button
                                    class="rounded-full border border-emerald-300 px-4 py-2 text-sm font-medium text-emerald-700 transition hover:border-emerald-400 hover:text-emerald-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-start"
                                    :disabled="!canStartSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('start')">
                                    启动
                                </button>
                                <button
                                    class="rounded-full border border-amber-300 px-4 py-2 text-sm font-medium text-amber-700 transition hover:border-amber-400 hover:text-amber-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-pause"
                                    :disabled="!canPauseSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('pause')">
                                    暂停
                                </button>
                                <button
                                    class="rounded-full border border-rose-300 px-4 py-2 text-sm font-medium text-rose-700 transition hover:border-rose-400 hover:text-rose-900 disabled:cursor-not-allowed disabled:opacity-50"
                                    data-testid="strategy-stop"
                                    :disabled="!canStopSelectedStrategy" type="button"
                                    @click="changeStrategyStatus('stop')">
                                    停止
                                </button>
                            </div>
                        </div>
                        <div v-if="detailsError"
                            class="mt-4 rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                            {{ detailsError }}
                        </div>
                    </div>

                    <div class="grid gap-4 xl:grid-cols-2">
                        <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                            <div class="text-xl font-semibold text-slate-900">运行参数</div>
                            <div class="mt-3">
                                <div v-if="selectedStrategy === null"
                                    class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                    请选择策略查看参数。
                                </div>
                                <div v-else class="rounded-3xl bg-slate-50 px-4 py-4">
                                    <pre
                                        class="overflow-x-auto whitespace-pre-wrap text-xs leading-6 text-slate-700">{{ selectedStrategyParamsJson }}</pre>
                                </div>
                            </div>
                        </div>

                        <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                            <div class="text-xl font-semibold text-slate-900">运行日志</div>
                            <div class="mt-3">
                                <div v-if="selectedStrategy === null"
                                    class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                    请选择策略查看日志。
                                </div>
                                <div v-else-if="isLoadingDetails"
                                    class="rounded-3xl border border-slate-200 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                    正在加载运行明细…
                                </div>
                                <ul v-else-if="strategyLogs.length > 0" class="grid gap-3 text-sm text-slate-700">
                                    <li v-for="entry in strategyLogs" :key="entry"
                                        class="rounded-3xl bg-slate-50 px-4 py-3 font-mono leading-6">
                                        {{ entry }}
                                    </li>
                                </ul>
                                <div v-else
                                    class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                    暂无日志。
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="rounded-[28px] border border-slate-200 bg-white p-4">
                        <div class="text-xl font-semibold text-slate-900">运行审计</div>
                        <div class="mt-3">
                            <div v-if="selectedStrategy === null"
                                class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                请选择策略查看审计。
                            </div>
                            <ul v-else-if="strategyAuditEntries.length > 0" class="grid gap-3 text-sm text-slate-700">
                                <li v-for="entry in strategyAuditEntries"
                                    :key="`${entry.at}-${entry.kind}-${entry.detail ?? ''}`"
                                    class="rounded-3xl bg-slate-50 px-4 py-4">
                                    <div class="flex flex-wrap items-center justify-between gap-3">
                                        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">{{
                                            formatAuditKind(entry.kind) }}</span>
                                        <span class="text-xs text-slate-500">{{ formatTimestamp(entry.at) }}</span>
                                    </div>
                                    <div class="mt-2 text-sm font-medium text-slate-900">
                                        {{ entry.detail ?? "生命周期变更" }}
                                    </div>
                                </li>
                            </ul>
                            <div v-else
                                class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500">
                                暂无审计记录。
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.runtime-panel {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    overflow: hidden;
}

.runtime-panel__bar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    flex-shrink: 0;
}

.runtime-panel__eyebrow {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.22em;
    text-transform: uppercase;
    color: var(--tv-text-muted);
}

.runtime-panel__title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.2rem;
}

.runtime-panel__title {
    font-size: 1.35rem;
    line-height: 1.2;
    font-weight: 700;
    color: var(--tv-text);
}

.runtime-panel__bar-actions {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.75rem;
}

.runtime-panel__metrics {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.runtime-panel__metric-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.2rem;
    padding: 0.45rem 0.85rem;
    border-radius: 999px;
    border: 1px solid var(--tv-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 84%, transparent);
    color: var(--tv-text-muted);
    font-size: 0.78rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
}

.runtime-panel__scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    overscroll-behavior: contain;
    display: grid;
    gap: 1rem;
    align-content: start;
    padding-bottom: 1rem;
}

/* 复用主题 strategy-list-card */
.strategy-list-card {
    display: block;
    width: 100%;
    text-align: left;
    border-radius: 1.5rem;
    border: 1px solid var(--tv-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 90%, transparent);
    padding: 0.9rem 1rem;
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease;
}

.strategy-list-card:hover {
    border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

.strategy-list-card.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 18%, transparent);
}
</style>
