<script setup lang="ts">
import type {
    StrategyBrokerAccountBinding,
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
} from "@/contracts";

defineProps<{
    selectedStrategy: StrategyInstanceItem;
    selectedStrategyBinding: StrategyInstanceBindingDocument | null;
    selectedStrategyDefinitionSync: StrategyDefinitionSyncStatus | null;
    isRefreshingStrategyDefinition: boolean;
    canRefreshSelectedStrategyDefinition: boolean;
    selectedStrategyDefinitionRefreshHint: string;
    isUpdatingStrategyRuntimeRisk: boolean;
    formatStrategyDefinitionSyncSummary: (sync: StrategyDefinitionSyncStatus | null | undefined) => string;
    formatStrategySymbols: (strategy: StrategyInstanceItem) => string;
    formatStrategyInterval: (strategy: StrategyInstanceItem) => string;
    formatStrategyExecutionMode: (mode: StrategyExecutionMode | string | null | undefined) => string;
    formatStrategyRuntimeRiskSummary: (settings: StrategyInstanceBindingDocument["runtimeRisk"] | null | undefined) => string;
    formatBrokerAccountSummary: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => string;
    isCurrentBrokerAccountBinding: (brokerAccount: StrategyBrokerAccountBinding | null | undefined) => boolean;
}>();

const emit = defineEmits<{
    "open-edit": [];
    "refresh-definition": [];
    "update-runtime-risk": [patch: Partial<StrategyRuntimeRiskSettings>];
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
    <div class="grid gap-4">
        <section
            class="strategy-binding-summary runtime-workbench-section cursor-pointer text-left"
            data-testid="strategy-current-binding-summary"
            @click="emit('open-edit')"
        >
            <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                    <div class="text-sm font-semibold runtime-workbench-text-strong">当前绑定摘要</div>
                    <div class="mt-1 text-xs runtime-workbench-text-muted">
                        点击卡片即可编辑绑定、更新执行模式或删除实例。
                    </div>
                </div>
                <div class="flex flex-wrap items-center justify-end gap-2">
                    <button
                        v-if="selectedStrategyDefinitionSync !== null && !selectedStrategyDefinitionSync.isLatest"
                        class="runtime-workbench-button"
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
                    <span class="runtime-workbench-pill">
                        仅 STOPPED 可编辑
                    </span>
                </div>
            </div>

            <div
                v-if="selectedStrategyDefinitionSync !== null"
                class="runtime-workbench-sync-panel mt-4"
                :class="selectedStrategyDefinitionSync.isLatest
                    ? 'runtime-workbench-sync-panel--latest'
                    : 'runtime-workbench-sync-panel--stale'"
            >
                <div class="flex flex-wrap items-center justify-between gap-2">
                    <span
                        data-testid="strategy-definition-sync-badge"
                        class="runtime-workbench-sync-badge"
                        :class="selectedStrategyDefinitionSync.isLatest
                            ? 'runtime-workbench-sync-badge--latest'
                            : 'runtime-workbench-sync-badge--stale'"
                    >
                        {{ formatStrategyDefinitionSyncSummary(selectedStrategyDefinitionSync) }}
                    </span>
                    <div
                        class="runtime-workbench-sync-panel__hint min-w-[14rem] flex-1"
                        :class="selectedStrategyDefinitionSync.isLatest
                            ? 'runtime-workbench-sync-panel__hint--latest'
                            : 'runtime-workbench-sync-panel__hint--stale'"
                    >
                        {{ selectedStrategyDefinitionRefreshHint }}
                    </div>
                </div>
            </div>

            <div class="strategy-binding-summary__details mt-4 text-sm">
                <div class="strategy-binding-summary__detail strategy-binding-summary__detail--wide">
                    <div class="runtime-workbench-field-label">策略定义</div>
                    <div class="runtime-workbench-field-value">
                        {{ selectedStrategy.definition.name }} / v{{ selectedStrategy.definition.version }}
                    </div>
                </div>
                <div class="strategy-binding-summary__detail strategy-binding-summary__detail--wide">
                    <div class="runtime-workbench-field-label">交易代码</div>
                    <div class="runtime-workbench-field-value">
                        {{ formatStrategySymbols(selectedStrategy) }}
                    </div>
                </div>
                <div class="strategy-binding-summary__detail">
                    <div class="runtime-workbench-field-label">周期</div>
                    <div class="runtime-workbench-field-value">
                        {{ formatStrategyInterval(selectedStrategy) }}
                    </div>
                </div>
                <div class="strategy-binding-summary__detail">
                    <div class="runtime-workbench-field-label">执行模式</div>
                    <div class="runtime-workbench-field-value">
                        {{ formatStrategyExecutionMode(selectedStrategyBinding?.executionMode) }}
                    </div>
                </div>
                <div class="strategy-binding-summary__detail strategy-binding-summary__detail--wide">
                    <div class="runtime-workbench-field-label">券商账号</div>
                    <div class="mt-1 flex flex-wrap items-center gap-2">
                        <div class="runtime-workbench-field-value mt-0 break-all">
                            {{ formatBrokerAccountSummary(selectedStrategyBinding?.brokerAccount) }}
                        </div>
                        <div
                            v-if="isCurrentBrokerAccountBinding(selectedStrategyBinding?.brokerAccount)"
                            class="runtime-workbench-pill runtime-workbench-pill--success"
                        >
                            当前
                        </div>
                    </div>
                </div>
                <div class="strategy-binding-summary__detail strategy-binding-summary__detail--risk">
                    <div class="runtime-workbench-field-label">动态风控</div>
                    <div class="strategy-binding-summary__risk-row">
                        <div class="runtime-workbench-field-value" data-testid="strategy-runtime-risk-summary">
                            {{ formatStrategyRuntimeRiskSummary(selectedStrategyBinding?.runtimeRisk) }}
                        </div>
                        <div v-if="selectedStrategyBinding !== null" class="strategy-binding-summary__risk-controls">
                            <select
                                :value="selectedStrategyBinding.runtimeRisk.mode"
                                class="runtime-workbench-input"
                                data-testid="strategy-runtime-risk-quick-mode"
                                :disabled="isUpdatingStrategyRuntimeRisk"
                                @click.stop
                                @change="handleRuntimeRiskModeChange"
                            >
                                <option value="off">关闭</option>
                                <option value="monitor">观察</option>
                                <option value="enforce">执行</option>
                            </select>
                            <label class="runtime-workbench-checkbox" @click.stop>
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
            </div>

            <div v-if="selectedStrategy.status !== 'STOPPED'" class="mt-4 text-xs text-amber-700 dark:text-amber-200">
                当前实例不是 STOPPED，先停止后才能修改绑定或删除。
            </div>
        </section>
    </div>
</template>

<style scoped>
.strategy-binding-summary {
    transition: border-color 140ms ease, background-color 140ms ease, box-shadow 140ms ease, transform 140ms ease;
}

.strategy-binding-summary:hover {
    border-color: color-mix(in srgb, var(--tv-accent) 38%, var(--tv-border));
    background: var(--card-surface-raised);
    box-shadow: 0 16px 32px rgb(15 23 42 / 0.08);
    transform: translateY(-1px);
}

.strategy-binding-summary:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--tv-accent) 70%, var(--card-surface));
    outline-offset: 3px;
}

.strategy-binding-summary__details {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
    gap: 0.85rem 1rem;
    align-items: start;
}

.strategy-binding-summary__detail {
    min-width: 0;
}

.strategy-binding-summary__detail--wide,
.strategy-binding-summary__detail--risk {
    grid-column: span 2;
}

.strategy-binding-summary__risk-row {
    display: grid;
    grid-template-columns: minmax(12rem, 1fr) minmax(16rem, auto);
    gap: 0.75rem;
    align-items: center;
}

.strategy-binding-summary__risk-controls {
    display: grid;
    grid-template-columns: minmax(8rem, 10rem) auto;
    gap: 0.5rem;
    align-items: center;
}

@media (max-width: 900px) {
    .strategy-binding-summary__detail--wide,
    .strategy-binding-summary__detail--risk {
        grid-column: span 1;
    }

    .strategy-binding-summary__risk-row {
        grid-template-columns: 1fr;
    }

    .strategy-binding-summary__risk-controls {
        grid-template-columns: minmax(0, 1fr) auto;
    }
}

@media (max-width: 560px) {
    .strategy-binding-summary__risk-controls {
        grid-template-columns: 1fr;
    }
}
</style>
