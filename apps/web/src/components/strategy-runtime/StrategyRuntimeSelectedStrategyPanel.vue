<script setup lang="ts">
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
    StrategyRuntimeObservation,
} from "@/contracts";

type StrategyAction = "start" | "pause" | "stop";

const props = defineProps<{
    selectedStrategy: StrategyInstanceItem;
    selectedStrategyBinding: StrategyInstanceBindingDocument | null;
    selectedStrategyDefinitionSync: StrategyDefinitionSyncStatus | null;
    selectedStrategyRuntimeObservation: StrategyRuntimeObservation | null;
    isRefreshingStrategyDefinition: boolean;
    canRefreshSelectedStrategyDefinition: boolean;
    selectedStrategyDefinitionRefreshHint: string;
    selectedStrategyRuntimeLabel: string;
    selectedStrategySourceFormatLabel: string;
    selectedStrategyStartHint: string;
    selectedStrategyCompiledSummary: string;
    isRefreshingStrategyContent: boolean;
    isUpdatingStrategyRuntimeRisk: boolean;
    canStartSelectedStrategy: boolean;
    canPauseSelectedStrategy: boolean;
    canStopSelectedStrategy: boolean;
    detailsError: string;
    formatStrategyDefinitionSyncSummary: (sync: StrategyDefinitionSyncStatus | null | undefined) => string;
    formatStrategySymbols: (strategy: StrategyInstanceItem) => string;
    formatStrategyInterval: (strategy: StrategyInstanceItem) => string;
    formatStrategyExecutionMode: (mode: StrategyExecutionMode | string | null | undefined) => string;
    formatStrategyRuntimeRiskSummary: (settings: StrategyInstanceBindingDocument["runtimeRisk"] | null | undefined) => string;
    formatBrokerAccountSummary: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => string;
    isCurrentBrokerAccountBinding: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => boolean;
    formatStrategyEligibility: (strategy: StrategyInstanceItem) => string;
    formatStrategyStatus: (status: StrategyInstanceItem["status"] | string) => string;
    formatRuntimeObservationSymbols: (symbols: string[] | null | undefined) => string;
    formatTimestamp: (value: unknown) => string;
    formatTimestampTooltip: (value: unknown) => string;
}>();

const emit = defineEmits<{
    "open-edit": [];
    "refresh-content": [];
    "refresh-definition": [];
    "update-runtime-risk": [patch: Partial<StrategyRuntimeRiskSettings>];
    "change-status": [action: StrategyAction];
}>();

function handleRuntimeRiskModeChange(event: Event): void {
    const value = (event.target as HTMLSelectElement).value;
    emit("update-runtime-risk", { mode: value === "monitor" || value === "enforce" ? value : "off" });
}

function handleRuntimeRiskCloseOnlyChange(event: Event): void {
    emit("update-runtime-risk", { closeOnly: (event.target as HTMLInputElement).checked });
}
</script>

<template>
    <div class="grid gap-4 2xl:grid-cols-[minmax(19rem,22rem)_minmax(0,1fr)]">
        <div
            class="strategy-binding-summary min-w-0 rounded-[28px] border border-slate-200 bg-white p-4 text-left cursor-pointer"
            data-testid="strategy-current-binding-summary"
            @click="emit('open-edit')"
        >
            <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                    <div class="text-xl font-semibold text-slate-900">当前绑定摘要</div>
                    <div class="mt-1 text-sm text-slate-500">
                        点击卡片即可编辑绑定、更新执行模式或删除实例。
                    </div>
                </div>
                <div class="flex flex-wrap items-center justify-end gap-2">
                    <button
                        v-if="selectedStrategyDefinitionSync !== null && !selectedStrategyDefinitionSync.isLatest"
                        class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-50"
                        data-testid="strategy-refresh-definition"
                        :disabled="!canRefreshSelectedStrategyDefinition"
                        :title="selectedStrategyDefinitionSync.canApplyLatest
                            ? `刷新到 v${selectedStrategyDefinitionSync.latestVersion}`
                            : (selectedStrategyDefinitionSync.blockedReason ?? '')"
                        type="button"
                        @click.stop="emit('refresh-definition')"
                    >
                        {{ isRefreshingStrategyDefinition ? "刷新中" : "刷新到最新策略" }}
                    </button>
                    <span
                        class="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500"
                    >
                        仅 STOPPED 可编辑
                    </span>
                </div>
            </div>
            <div
                v-if="selectedStrategyDefinitionSync !== null"
                class="mt-4 rounded-3xl border px-4 py-3"
                :class="selectedStrategyDefinitionSync.isLatest
                    ? 'border-emerald-200 bg-emerald-50/70'
                    : 'border-amber-200 bg-amber-50/70'"
            >
                <div class="flex flex-wrap items-center gap-2">
                    <span
                        data-testid="strategy-definition-sync-badge"
                        class="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
                        :class="selectedStrategyDefinitionSync.isLatest
                            ? 'border border-emerald-200 bg-white text-emerald-700'
                            : 'border border-amber-200 bg-white text-amber-700'"
                    >
                        {{ formatStrategyDefinitionSyncSummary(selectedStrategyDefinitionSync) }}
                    </span>
                </div>
                <div
                    class="mt-2 text-sm"
                    :class="selectedStrategyDefinitionSync.isLatest ? 'text-emerald-700' : 'text-amber-700'"
                >
                    {{ selectedStrategyDefinitionRefreshHint }}
                </div>
            </div>
            <div class="mt-4 grid gap-3 text-sm text-slate-600">
                <div>
                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">策略定义</div>
                    <div class="mt-1 break-words font-medium text-slate-900">
                        {{ selectedStrategy.definition.name }} / v{{ selectedStrategy.definition.version }}
                    </div>
                </div>
                <div>
                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">交易代码</div>
                    <div class="mt-1 break-words font-medium text-slate-900">
                        {{ formatStrategySymbols(selectedStrategy) }}
                    </div>
                </div>
                <div class="grid gap-3 sm:grid-cols-2">
                    <div>
                        <div class="text-xs uppercase tracking-[0.16em] text-slate-400">周期</div>
                        <div class="mt-1 font-medium text-slate-900">
                            {{ formatStrategyInterval(selectedStrategy) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-xs uppercase tracking-[0.16em] text-slate-400">执行模式</div>
                        <div class="mt-1 font-medium text-slate-900">
                            {{ formatStrategyExecutionMode(selectedStrategyBinding?.executionMode) }}
                        </div>
                    </div>
                </div>
                <div>
                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">券商账号</div>
                    <div class="mt-1 break-all font-medium text-slate-900">
                        {{ formatBrokerAccountSummary(selectedStrategyBinding?.brokerAccount) }}
                    </div>
                    <div
                        v-if="isCurrentBrokerAccountBinding(selectedStrategyBinding?.brokerAccount)"
                        class="mt-2 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700"
                    >
                        当前
                    </div>
                </div>
                <div>
                    <div class="text-xs uppercase tracking-[0.16em] text-slate-400">动态风控</div>
                    <div class="mt-1 break-words font-medium text-slate-900" data-testid="strategy-runtime-risk-summary">
                        {{ formatStrategyRuntimeRiskSummary(selectedStrategyBinding?.runtimeRisk) }}
                    </div>
                    <div v-if="selectedStrategyBinding !== null" class="mt-3 grid gap-2 sm:grid-cols-[minmax(0,10rem)_minmax(0,1fr)]">
                        <select
                            :value="selectedStrategyBinding.runtimeRisk.mode"
                            class="rounded-2xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-slate-500 disabled:opacity-60"
                            data-testid="strategy-runtime-risk-quick-mode"
                            :disabled="isUpdatingStrategyRuntimeRisk"
                            @change="handleRuntimeRiskModeChange"
                        >
                            <option value="off">关闭</option>
                            <option value="monitor">观察</option>
                            <option value="enforce">执行</option>
                        </select>
                        <label class="inline-flex items-center gap-2 rounded-2xl bg-slate-50 px-3 py-2 text-sm text-slate-600">
                            <input
                                :checked="selectedStrategyBinding.runtimeRisk.closeOnly"
                                data-testid="strategy-runtime-risk-quick-close-only"
                                :disabled="isUpdatingStrategyRuntimeRisk"
                                type="checkbox"
                                @change="handleRuntimeRiskCloseOnlyChange"
                            >
                            <span>仅平仓</span>
                        </label>
                    </div>
                </div>
            </div>
            <div v-if="selectedStrategy.status !== 'STOPPED'" class="mt-4 text-xs text-amber-700">
                当前实例不是 STOPPED，先停止后才能修改绑定或删除。
            </div>
        </div>

        <div class="rounded-[28px] border border-slate-200 bg-white p-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                    <div class="text-xl font-semibold text-slate-900">运行控制</div>
                    <div class="mt-1 text-sm text-slate-500">
                        启动、暂停、停止都会同步刷新日志与审计视图；页面也会定时补刷新当前内容。
                    </div>
                </div>
                <div class="flex flex-wrap items-center justify-end gap-3">
                    <v-button
                        class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-50"
                        data-testid="strategy-refresh-content"
                        :disabled="isRefreshingStrategyContent"
                        type="button"
                        :loading="isRefreshingStrategyContent"
                        @click="emit('refresh-content')"
                    >
                        {{ isRefreshingStrategyContent ? "等待" : "刷新" }}
                    </v-button>
                    <div class="rounded-3xl bg-slate-50 px-4 py-4">
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
                                : 'border border-amber-200 bg-amber-50 text-amber-700'"
                        >
                            {{ formatStrategyEligibility(selectedStrategy) }}
                        </span>
                        <span
                            v-if="selectedStrategyBinding !== null"
                            class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em]"
                            :class="selectedStrategyBinding.executionMode === 'notify_only'
                                ? 'border border-sky-200 bg-sky-50 text-sky-700'
                                : 'border border-slate-200 bg-white text-slate-600'"
                        >
                            {{ formatStrategyExecutionMode(selectedStrategyBinding.executionMode) }}
                        </span>
                    </div>
                    <div class="mt-3 text-sm text-slate-600" data-testid="strategy-runtime-start-hint">
                        {{ selectedStrategyStartHint }}
                    </div>
                    <div v-if="selectedStrategyCompiledSummary" class="mt-2 text-xs text-slate-500">
                        {{ selectedStrategyCompiledSummary }}
                    </div>
                    </div>
                </div>
            </div>

            <div
                v-if="selectedStrategyRuntimeObservation !== null"
                class="mt-4 rounded-3xl border border-slate-200 bg-white/80 px-4 py-4"
                data-testid="strategy-runtime-observation"
            >
                <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">实际运行态</div>
                <div class="mt-3 grid gap-3 text-sm text-slate-600 sm:grid-cols-2 xl:grid-cols-3">
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">运行状态</div>
                        <div class="mt-1 font-medium text-slate-900">
                            {{ formatStrategyStatus(selectedStrategyRuntimeObservation.actualStatus) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">活跃标的</div>
                        <div class="mt-1 font-medium text-slate-900">
                            {{ formatRuntimeObservationSymbols(selectedStrategyRuntimeObservation.activeSymbols) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近闭合 K 线</div>
                        <div class="mt-1 font-medium text-slate-900 strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastClosedKlineAt)">
                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastClosedKlineAt) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近信号</div>
                        <div class="mt-1 font-medium text-slate-900 strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastSignalAt)">
                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastSignalAt) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近下单</div>
                        <div class="mt-1 font-medium text-slate-900 strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastOrderAt)">
                            {{ formatTimestamp(selectedStrategyRuntimeObservation.lastOrderAt) }}
                        </div>
                    </div>
                    <div>
                        <div class="text-[11px] uppercase tracking-[0.16em] text-slate-400">最近更新</div>
                        <div class="mt-1 font-medium text-slate-900 strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.updatedAt)">
                            {{ formatTimestamp(selectedStrategyRuntimeObservation.updatedAt) }}
                        </div>
                    </div>
                </div>
                <div v-if="selectedStrategyRuntimeObservation.lastError" class="mt-3 rounded-2xl border border-amber-200 bg-amber-50 px-3 py-3 text-xs text-amber-700">
                    最近异常：{{ selectedStrategyRuntimeObservation.lastError }}
                    <span class="strategy-time-display text-amber-600" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastErrorAt)">
                        （{{ formatTimestamp(selectedStrategyRuntimeObservation.lastErrorAt) }}）
                    </span>
                </div>
            </div>
            <div v-else class="mt-4 text-xs text-slate-500">
                实例未运行时暂无实时观测信息。
            </div>
            <div class="mt-4 flex flex-wrap gap-2">
                <button
                    class="strategy-runtime-start-button rounded-full border border-emerald-300 px-4 py-2 text-sm font-medium text-emerald-700 transition hover:border-emerald-400 hover:text-emerald-900 disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="strategy-start"
                    :disabled="!canStartSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'start')"
                >
                    启动
                </button>
                <button
                    class="rounded-full border border-amber-300 px-4 py-2 text-sm font-medium text-amber-700 transition hover:border-amber-400 hover:text-amber-900 disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="strategy-pause"
                    :disabled="!canPauseSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'pause')"
                >
                    暂停
                </button>
                <button
                    class="rounded-full border border-rose-300 px-4 py-2 text-sm font-medium text-rose-700 transition hover:border-rose-400 hover:text-rose-900 disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="strategy-stop"
                    :disabled="!canStopSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'stop')"
                >
                    停止
                </button>
            </div>
            <div v-if="detailsError" class="mt-4 rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                {{ detailsError }}
            </div>
        </div>
    </div>
</template>

<style scoped>
.strategy-binding-summary {
    cursor: pointer;
    border-color: var(--card-border);
    background: var(--card-surface);
    color: var(--card-text-1);
    transition: border-color 140ms ease, background-color 140ms ease, transform 140ms ease, box-shadow 140ms ease;
}

.strategy-binding-summary:hover {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    box-shadow: 0 18px 40px rgb(15 23 42 / 0.08);
    transform: translateY(-1px);
}

.strategy-binding-summary:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--tv-accent) 70%, var(--card-surface));
    outline-offset: 3px;
}

.strategy-binding-summary .text-slate-900,
.strategy-binding-summary .text-slate-800,
.strategy-binding-summary .text-slate-700 {
    color: var(--card-text-1);
}

.strategy-binding-summary .text-slate-600,
.strategy-binding-summary .text-slate-500 {
    color: var(--card-text-2);
}

.strategy-binding-summary .text-slate-400 {
    color: var(--card-text-3);
}

.strategy-binding-summary .border-slate-200,
.strategy-binding-summary .border-slate-300 {
    border-color: var(--card-border);
}

.strategy-binding-summary .bg-white {
    background: var(--card-surface);
}

.strategy-binding-summary .bg-slate-50 {
    background: var(--card-surface-raised);
}

.strategy-runtime-start-button {
    border-color: var(--card-teal-border);
    background: color-mix(in srgb, var(--card-teal-surface) 74%, var(--tv-bg-surface) 26%);
    color: var(--card-teal-text);
    box-shadow: 0 8px 20px rgb(15 23 42 / 0.04);
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, box-shadow 140ms ease, transform 140ms ease;
}

.strategy-runtime-start-button:hover {
    border-color: color-mix(in srgb, var(--card-teal-border) 60%, var(--tv-accent));
    background: color-mix(in srgb, var(--card-teal-surface) 84%, var(--tv-accent) 8%);
    color: var(--tv-text);
    box-shadow: 0 12px 24px rgb(15 23 42 / 0.08);
    transform: translateY(-1px);
}

.strategy-runtime-start-button:disabled {
    border-color: var(--tv-border);
    background: var(--tv-bg-surface-2);
    color: var(--tv-text-dim);
    box-shadow: none;
    transform: none;
}
</style>
