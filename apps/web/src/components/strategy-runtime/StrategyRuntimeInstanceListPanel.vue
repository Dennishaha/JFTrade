<script setup lang="ts">
import { computed } from "vue";
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
} from "@/contracts";

import StrategyInstanceCard from "../domain/strategy/StrategyInstanceCard.vue";
import type { StrategyInstanceCardModel } from "../domain/strategy/strategyInstanceCard";

const props = defineProps<{
    isCreateMenuOpen: boolean;
    isLoadingStrategies: boolean;
    listError: string;
    strategies: StrategyInstanceItem[];
    selectedStrategyId: string;
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

const strategyCardModels = computed<StrategyInstanceCardModel[]>(() => props.strategies.map((strategy) => {
    const binding = props.readStrategyBinding(strategy);
    const status = strategy.runtimeObservation?.actualStatus ?? strategy.status;
    return {
        id: strategy.id,
        name: strategy.definition.name,
        status,
        statusLabel: props.formatStrategyStatus(status),
        selected: strategy.id === props.selectedStrategyId,
        definitionStale: strategy.definitionSync != null && !strategy.definitionSync.isLatest,
        definitionSyncSummary: props.formatStrategyDefinitionSyncSummary(strategy.definitionSync),
        symbols: props.formatStrategySymbols(strategy),
        interval: props.formatStrategyInterval(strategy),
        brokerAccountSummary: props.formatBrokerAccountSummary(binding.brokerAccount),
        currentBrokerAccount: props.isCurrentBrokerAccountBinding(binding.brokerAccount),
        createdAt: props.formatTimestamp(strategy.createdAt),
        createdAtTooltip: props.formatTimestampTooltip(strategy.createdAt),
        runtimeLabel: props.formatStrategyRuntime(strategy.runtime),
        sourceFormatLabel: props.formatSourceFormat(strategy.sourceFormat),
        eligibilityLabel: props.formatStrategyEligibility(strategy),
        startable: strategy.startable,
        executionModeLabel: props.formatStrategyExecutionMode(binding.executionMode),
        notifyOnly: binding.executionMode === "notify_only",
    };
}));

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

        <div v-if="listError" class="rounded-3xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
            {{ listError }}
        </div>
        <div
            v-else-if="isLoadingStrategies && strategies.length === 0"
            class="rounded-3xl border border-slate-200 bg-slate-50 px-4 py-5 text-sm text-slate-500"
            data-state="loading"
            aria-live="polite"
        >
            正在加载策略实例…
        </div>
        <div
            v-else-if="strategies.length === 0"
            class="rounded-3xl border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-500"
        >
            暂无策略实例。先从设计区保存定义并创建运行实例。
        </div>
        <div v-else class="grid gap-3">
            <StrategyInstanceCard
                v-for="model in strategyCardModels"
                :key="model.id"
                :model="model"
                @select="emit('select-strategy', $event)"
            />
        </div>
    </div>
</template>
