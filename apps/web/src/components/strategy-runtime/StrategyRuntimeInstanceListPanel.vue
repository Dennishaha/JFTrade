<script setup lang="ts">
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
} from "@/contracts";

const props = defineProps<{
    isCreateMenuOpen: boolean;
    isLoadingStrategies: boolean;
    listError: string;
    strategies: StrategyInstanceItem[];
    selectedStrategyId: string;
    displayStrategyStatus: (strategy: StrategyInstanceItem) => StrategyInstanceItem["status"];
    strategyStatusBadgeClass: (strategy: StrategyInstanceItem) => string;
    strategyStatusCardClass: (strategy: StrategyInstanceItem) => string;
    formatStrategyStatus: (status: StrategyInstanceItem["status"] | string) => string;
    formatStrategyDefinitionSyncSummary: (sync: StrategyDefinitionSyncStatus | null | undefined) => string;
    formatStrategySymbols: (strategy: StrategyInstanceItem) => string;
    formatStrategyInterval: (strategy: StrategyInstanceItem) => string;
    formatBrokerAccountSummary: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => string;
    readStrategyBinding: (strategy: StrategyInstanceItem) => StrategyInstanceBindingDocument;
    isCurrentBrokerAccountBinding: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => boolean;
    formatTimestamp: (value: unknown) => string;
    formatTimestampTooltip: (value: unknown) => string;
    formatStrategyRuntime: (runtime: unknown) => string;
    formatSourceFormat: (sourceFormat: string | null | undefined) => string;
    formatStrategyEligibility: (strategy: StrategyInstanceItem) => string;
    formatStrategyExecutionMode: (mode: StrategyExecutionMode | string | null | undefined) => string;
}>();

const emit = defineEmits<{
    "toggle-create-menu": [];
    "open-create-definition": [];
    "open-create-instance": [];
    "refresh-strategies": [];
    "select-strategy": [strategyId: string];
}>();
</script>

<template>
    <div class="min-w-0 rounded-[28px] border border-slate-200 bg-white p-4">
        <div class="mb-4 flex items-center justify-between gap-3">
            <div class="text-xl font-semibold text-slate-900">策略实例</div>
            <div class="flex flex-wrap items-center gap-2">
                <div class="relative">
                    <button
                        class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                        data-testid="strategy-create-menu-toggle"
                        type="button"
                        :aria-expanded="isCreateMenuOpen ? 'true' : 'false'"
                        @click="emit('toggle-create-menu')"
                    >
                        新增
                    </button>
                    <div
                        v-if="isCreateMenuOpen"
                        data-testid="strategy-create-menu"
                        class="absolute right-0 z-10 mt-2 grid min-w-[12rem] gap-1 rounded-3xl border border-slate-200 bg-white p-2 shadow-lg"
                    >
                        <button
                            class="rounded-2xl px-3 py-2 text-left text-sm font-medium text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                            data-testid="strategy-new-definition"
                            type="button"
                            @click="emit('open-create-definition')"
                        >
                            新增策略
                        </button>
                        <button
                            class="rounded-2xl px-3 py-2 text-left text-sm font-medium text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                            data-testid="strategy-new-instance"
                            type="button"
                            @click="emit('open-create-instance')"
                        >
                            新增实例
                        </button>
                    </div>
                </div>
                <button
                    class="rounded-full border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:border-slate-400 hover:text-slate-900"
                    type="button"
                    @click="emit('refresh-strategies')"
                >
                    {{ isLoadingStrategies ? "等待" : "刷新" }}
                </button>
            </div>
        </div>

        <slot />

        <div v-if="listError" class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            {{ listError }}
        </div>
        <div
            v-else-if="strategies.length === 0"
            class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500"
        >
            暂无策略实例。先从设计区保存定义并创建运行实例。
        </div>
        <div v-else class="grid gap-3">
            <button
                v-for="strategy in strategies"
                :key="strategy.id"
                :data-testid="`strategy-${strategy.id}`"
                class="strategy-list-card"
                :class="[strategyStatusCardClass(strategy), { 'is-active': strategy.id === selectedStrategyId }]"
                type="button"
                @click="emit('select-strategy', strategy.id)"
            >
                <div class="flex items-center justify-between gap-3">
                    <div class="min-w-0 break-words text-base font-semibold">{{ strategy.definition.name }}</div>
                    <div class="flex flex-wrap items-center justify-end gap-2">
                        <div
                            v-if="strategy.definitionSync && !strategy.definitionSync.isLatest"
                            :data-testid="`strategy-definition-stale-${strategy.id}`"
                            class="rounded-full border border-amber-200 bg-amber-50 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-amber-700"
                        >
                            待刷新
                        </div>
                        <div :data-testid="`strategy-status-${strategy.id}`" :class="strategyStatusBadgeClass(strategy)">
                            {{ formatStrategyStatus(displayStrategyStatus(strategy)) }}
                        </div>
                    </div>
                </div>
                <div class="mt-2 break-all text-sm text-slate-500">{{ strategy.id }}</div>
                <div v-if="strategy.definitionSync && !strategy.definitionSync.isLatest" class="mt-2 text-sm text-amber-700">
                    {{ formatStrategyDefinitionSyncSummary(strategy.definitionSync) }}
                </div>
                <div class="mt-2 text-sm text-slate-500">标的 {{ formatStrategySymbols(strategy) }}</div>
                <div class="mt-1 text-sm text-slate-500">
                    周期 {{ formatStrategyInterval(strategy) }}
                </div>
                <div class="mt-1 break-all text-sm text-slate-500">
                    {{ formatBrokerAccountSummary(readStrategyBinding(strategy).brokerAccount) }}
                </div>
                <div
                    v-if="isCurrentBrokerAccountBinding(readStrategyBinding(strategy).brokerAccount)"
                    class="mt-1 inline-flex rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em] text-emerald-700"
                >
                    当前
                </div>
                <div class="mt-2 text-sm text-slate-500">
                    创建于
                    <span class="strategy-time-display" :title="formatTimestampTooltip(strategy.createdAt)">
                        {{ formatTimestamp(strategy.createdAt) }}
                    </span>
                </div>
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
                            : 'border border-amber-200 bg-amber-50 text-amber-700'"
                    >
                        {{ formatStrategyEligibility(strategy) }}
                    </span>
                    <span
                        class="rounded-full px-2.5 py-1"
                        :class="readStrategyBinding(strategy).executionMode === 'notify_only'
                            ? 'border border-sky-200 bg-sky-50 text-sky-700'
                            : 'border border-slate-200 bg-slate-100 text-slate-600'"
                    >
                        {{ formatStrategyExecutionMode(readStrategyBinding(strategy).executionMode) }}
                    </span>
                </div>
            </button>
        </div>
    </div>
</template>

<style scoped>
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

.strategy-list-card--running {
    border-color: color-mix(in srgb, rgb(52 211 153) 58%, var(--tv-border));
    background: color-mix(in srgb, rgb(236 253 245) 90%, var(--tv-bg-surface));
}

.strategy-list-card--paused {
    border-color: color-mix(in srgb, rgb(251 191 36) 55%, var(--tv-border));
    background: color-mix(in srgb, rgb(255 251 235) 90%, var(--tv-bg-surface));
}

.strategy-list-card--stopped {
    border-color: var(--tv-border);
}

.strategy-status-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.2rem 0.65rem;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
}

.strategy-status-badge--running {
    border: 1px solid rgb(167 243 208);
    background: rgb(236 253 245);
    color: rgb(4 120 87);
}

.strategy-status-badge--paused {
    border: 1px solid rgb(253 230 138);
    background: rgb(254 249 195);
    color: rgb(161 98 7);
}

.strategy-status-badge--stopped {
    border: 1px solid rgb(226 232 240);
    background: rgb(248 250 252);
    color: rgb(71 85 105);
}
</style>