<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
    StrategyDefinitionSyncStatus,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskSettings,
    StrategyRuntimeObservation,
} from "@/types";
import type {
    StrategyAuditEntryDocument,
    StrategyBrokerAccountBinding,
} from "@/contracts";
import AppTabs from "@/components/shared/AppTabs.vue";

import StrategyRuntimeActivityPanel from "./StrategyRuntimeActivityPanel.vue";
import StrategyRuntimeBindingTab from "./StrategyRuntimeBindingTab.vue";
import StrategyRuntimeRunTab from "./StrategyRuntimeRunTab.vue";
import StrategyRuntimeWorkbenchHeader from "./StrategyRuntimeWorkbenchHeader.vue";

type StrategyAction = "start" | "pause" | "stop";
type StrategyRuntimeWorkbenchTab = "runtime" | "binding" | "activity";

const props = defineProps<{
    selectedStrategy: StrategyInstanceItem;
    selectedStrategyBinding: StrategyInstanceBindingDocument | null;
    selectedStrategyDefinitionSync: StrategyDefinitionSyncStatus | null;
    selectedStrategyRuntimeObservation: StrategyRuntimeObservation | null;
    isLoadingDetails: boolean;
    strategyLogs: string[];
    strategyAuditEntries: StrategyAuditEntryDocument[];
    selectedStrategyParamsJson: string;
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

const activeRuntimeTab = ref<StrategyRuntimeWorkbenchTab>("runtime");

const runtimeTabs = computed(() => [
    { value: "runtime" as const, label: "运行", testId: "strategy-runtime-tab-runtime" },
    { value: "binding" as const, label: "绑定", testId: "strategy-runtime-tab-binding" },
    {
        value: "activity" as const,
        label: `活动 ${props.strategyLogs.length + props.strategyAuditEntries.length}`,
        testId: "strategy-runtime-tab-activity",
    },
]);

const selectedRuntimeStatus = computed(
    () => props.selectedStrategyRuntimeObservation?.actualStatus ?? props.selectedStrategy.status,
);

const selectedRuntimeStatusLabel = computed(() => props.formatStrategyStatus(selectedRuntimeStatus.value));

watch(
    () => props.selectedStrategy.id,
    () => {
        activeRuntimeTab.value = "runtime";
    },
);

function selectRuntimeTab(tab: string): void {
    if (tab !== "runtime" && tab !== "binding" && tab !== "activity") return;
    activeRuntimeTab.value = tab;
}
</script>

<template>
    <section class="runtime-workbench-panel flex min-h-0 flex-1 flex-col overflow-hidden">
        <StrategyRuntimeWorkbenchHeader
            :selected-strategy="selectedStrategy"
            :selected-runtime-status="selectedRuntimeStatus"
            :selected-runtime-status-label="selectedRuntimeStatusLabel"
            :selected-strategy-runtime-label="selectedStrategyRuntimeLabel"
            :is-refreshing-strategy-content="isRefreshingStrategyContent"
            :can-start-selected-strategy="canStartSelectedStrategy"
            :can-pause-selected-strategy="canPauseSelectedStrategy"
            :can-stop-selected-strategy="canStopSelectedStrategy"
            :format-strategy-symbols="formatStrategySymbols"
            :format-strategy-interval="formatStrategyInterval"
            @refresh-content="emit('refresh-content')"
            @change-status="emit('change-status', $event)"
        />

        <AppTabs
            class="runtime-workbench-tabs border-b px-4"
            :model-value="activeRuntimeTab"
            :items="runtimeTabs"
            label="策略运行视图"
            @update:model-value="selectRuntimeTab"
        />

        <div class="min-h-0 flex-1 overflow-auto p-4">
            <StrategyRuntimeRunTab
                v-show="activeRuntimeTab === 'runtime'"
                :selected-strategy="selectedStrategy"
                :selected-strategy-binding="selectedStrategyBinding"
                :selected-strategy-runtime-observation="selectedStrategyRuntimeObservation"
                :selected-strategy-runtime-label="selectedStrategyRuntimeLabel"
                :selected-strategy-source-format-label="selectedStrategySourceFormatLabel"
                :selected-strategy-start-hint="selectedStrategyStartHint"
                :selected-strategy-compiled-summary="selectedStrategyCompiledSummary"
                :details-error="detailsError"
                :format-strategy-eligibility="formatStrategyEligibility"
                :format-strategy-execution-mode="formatStrategyExecutionMode"
                :format-strategy-status="formatStrategyStatus"
                :format-runtime-observation-symbols="formatRuntimeObservationSymbols"
                :format-timestamp="formatTimestamp"
                :format-timestamp-tooltip="formatTimestampTooltip"
            />

            <StrategyRuntimeBindingTab
                v-show="activeRuntimeTab === 'binding'"
                :selected-strategy="selectedStrategy"
                :selected-strategy-binding="selectedStrategyBinding"
                :selected-strategy-definition-sync="selectedStrategyDefinitionSync"
                :is-refreshing-strategy-definition="isRefreshingStrategyDefinition"
                :can-refresh-selected-strategy-definition="canRefreshSelectedStrategyDefinition"
                :selected-strategy-definition-refresh-hint="selectedStrategyDefinitionRefreshHint"
                :is-updating-strategy-runtime-risk="isUpdatingStrategyRuntimeRisk"
                :format-strategy-definition-sync-summary="formatStrategyDefinitionSyncSummary"
                :format-strategy-symbols="formatStrategySymbols"
                :format-strategy-interval="formatStrategyInterval"
                :format-strategy-execution-mode="formatStrategyExecutionMode"
                :format-strategy-runtime-risk-summary="formatStrategyRuntimeRiskSummary"
                :format-broker-account-summary="formatBrokerAccountSummary"
                :is-current-broker-account-binding="isCurrentBrokerAccountBinding"
                @open-edit="emit('open-edit')"
                @refresh-definition="emit('refresh-definition')"
                @update-runtime-risk="emit('update-runtime-risk', $event)"
            />

            <div v-show="activeRuntimeTab === 'activity'" class="min-h-0">
                <StrategyRuntimeActivityPanel
                    :key="selectedStrategy.id"
                    :is-loading-details="isLoadingDetails"
                    :strategy-logs="strategyLogs"
                    :strategy-audit-entries="strategyAuditEntries"
                    :selected-strategy-params-json="selectedStrategyParamsJson"
                />
            </div>
        </div>
    </section>
</template>
