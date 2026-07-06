<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
} from "@/contracts";

import RuntimeWorkbenchAlert from "./RuntimeWorkbenchAlert.vue";
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

type StrategyInstanceStatusFilter = "all" | "running" | "paused" | "stopped" | "other";

const instanceSearchQuery = ref("");
const instanceStatusFilter = ref<StrategyInstanceStatusFilter>("all");
const dismissedListError = ref("");

const instanceStatusFilterOptions: Array<{ value: StrategyInstanceStatusFilter; label: string }> = [
    { value: "all", label: "全部状态" },
    { value: "running", label: "运行中" },
    { value: "paused", label: "已暂停" },
    { value: "stopped", label: "已停止" },
    { value: "other", label: "其他" },
];

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

const filteredStrategyCardModels = computed(() => {
    const query = normalizeSearchText(instanceSearchQuery.value);
    return strategyCardModels.value.filter((model) => {
        if (!matchesStatusFilter(model.status, instanceStatusFilter.value)) {
            return false;
        }
        if (query === "") {
            return true;
        }
        return [
            model.id,
            model.name,
            model.statusLabel,
            model.symbols,
            model.interval,
            model.brokerAccountSummary,
            model.runtimeLabel,
            model.sourceFormatLabel,
            model.eligibilityLabel,
            model.executionModeLabel,
        ].some((value) => normalizeSearchText(value).includes(query));
    });
});

const hasInstanceFilters = computed(
    () => instanceSearchQuery.value.trim() !== "" || instanceStatusFilter.value !== "all",
);

const visibleListError = computed(() => {
    const message = props.listError.trim();
    return message !== "" && dismissedListError.value !== props.listError;
});

const emit = defineEmits<{
    "toggle-create-menu": [];
    "open-create-definition": [];
    "open-create-instance": [];
    "refresh-strategies": [];
    "select-strategy": [strategyId: string];
}>();

function normalizeSearchText(value: unknown): string {
    return String(value ?? "").trim().toLowerCase();
}

function matchesStatusFilter(status: string, filter: StrategyInstanceStatusFilter): boolean {
    if (filter === "all") {
        return true;
    }
    const normalizedStatus = status.trim().toUpperCase();
    if (filter === "running") {
        return normalizedStatus === "RUNNING";
    }
    if (filter === "paused") {
        return normalizedStatus === "PAUSED";
    }
    if (filter === "stopped") {
        return normalizedStatus === "STOPPED";
    }
    return normalizedStatus !== "RUNNING" && normalizedStatus !== "PAUSED" && normalizedStatus !== "STOPPED";
}

function resetInstanceFilters(): void {
    instanceSearchQuery.value = "";
    instanceStatusFilter.value = "all";
}

function closeListError(): void {
    dismissedListError.value = props.listError;
}

watch(
    () => props.listError,
    (message) => {
        if (message.trim() === "") {
            dismissedListError.value = "";
        }
    },
);
</script>

<template>
    <div class="runtime-workbench-panel flex min-h-0 flex-1 flex-col overflow-hidden">
        <div class="runtime-workbench-panel__header flex items-center justify-between gap-3 border-b px-3 py-3">
            <div class="min-w-0">
                <div class="text-sm font-semibold runtime-workbench-text-strong">策略实例</div>
                <div class="text-xs runtime-workbench-text-muted">
                    {{ strategies.length }} 个实例
                    <span v-if="filteredStrategyCardModels.length !== strategies.length">
                        · {{ filteredStrategyCardModels.length }} 个匹配
                    </span>
                </div>
            </div>
            <div class="flex flex-wrap items-center gap-2">
                <div class="relative">
                    <button
                        class="runtime-workbench-button runtime-workbench-button--primary"
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
                        class="runtime-workbench-menu absolute right-0 z-10 mt-2 grid min-w-[12rem] gap-1 rounded-lg border p-2 shadow-lg"
                    >
                        <button
                            class="runtime-workbench-menu__item"
                            data-testid="strategy-new-definition"
                            type="button"
                            @click="emit('open-create-definition')"
                        >
                            新增策略
                        </button>
                        <button
                            class="runtime-workbench-menu__item"
                            data-testid="strategy-new-instance"
                            type="button"
                            @click="emit('open-create-instance')"
                        >
                            新增实例
                        </button>
                    </div>
                </div>
                <button
                    class="runtime-workbench-button"
                    type="button"
                    @click="emit('refresh-strategies')"
                >
                    {{ isLoadingStrategies ? "等待" : "刷新" }}
                </button>
            </div>
        </div>

        <slot />

        <div class="grid gap-2 border-b px-3 py-3">
            <input
                v-model="instanceSearchQuery"
                class="runtime-workbench-input"
                data-testid="strategy-instance-search"
                placeholder="搜索策略、实例 ID、标的或券商"
                type="search"
            >
            <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
                <select
                    v-model="instanceStatusFilter"
                    class="runtime-workbench-input"
                    data-testid="strategy-instance-status-filter"
                >
                    <option
                        v-for="option in instanceStatusFilterOptions"
                        :key="option.value"
                        :value="option.value"
                    >
                        {{ option.label }}
                    </option>
                </select>
                <button
                    class="runtime-workbench-button"
                    :disabled="!hasInstanceFilters"
                    type="button"
                    @click="resetInstanceFilters"
                >
                    清空
                </button>
            </div>
        </div>

        <div class="min-h-0 flex-1 overflow-auto p-3">
            <RuntimeWorkbenchAlert
                v-if="visibleListError"
                close-label="关闭错误"
                close-test-id="strategy-list-error-close"
                role="alert"
                tone="error"
                @close="closeListError"
            >
                {{ listError }}
            </RuntimeWorkbenchAlert>
            <div
                v-else-if="isLoadingStrategies && strategies.length === 0"
                class="runtime-workbench-empty p-5 text-sm"
                data-state="loading"
                aria-live="polite"
            >
                正在加载策略实例…
            </div>
            <div
                v-else-if="strategies.length === 0"
                class="runtime-workbench-empty runtime-workbench-empty--dashed p-5 text-sm"
            >
                暂无策略实例。先从设计区保存定义并创建运行实例。
            </div>
            <div
                v-else-if="filteredStrategyCardModels.length === 0"
                class="runtime-workbench-empty p-5 text-sm"
            >
                没有匹配当前搜索或筛选条件的实例。
            </div>
            <div v-else class="grid gap-2">
                <StrategyInstanceCard
                    v-for="model in filteredStrategyCardModels"
                    :key="model.id"
                    :model="model"
                    @select="emit('select-strategy', $event)"
                />
            </div>
        </div>
    </div>
</template>
