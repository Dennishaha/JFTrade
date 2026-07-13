<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeObservation,
} from "@/contracts";

import RuntimeHealthBadge from "../domain/runtime/RuntimeHealthBadge.vue";
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import RuntimeWorkbenchAlert from "./RuntimeWorkbenchAlert.vue";
import { normalizeStrategyInstrumentIds } from "./strategyRuntimeInstanceBinding";

const props = defineProps<{
    selectedStrategy: StrategyInstanceItem;
    selectedStrategyBinding: StrategyInstanceBindingDocument | null;
    selectedStrategyRuntimeObservation: StrategyRuntimeObservation | null;
    selectedStrategyRuntimeLabel: string;
    selectedStrategySourceFormatLabel: string;
    selectedStrategyStartHint: string;
    selectedStrategyCompiledSummary: string;
    detailsError: string;
    formatStrategyEligibility: (strategy: StrategyInstanceItem) => string;
    formatStrategyExecutionMode: (mode: StrategyExecutionMode | string | null | undefined) => string;
    formatStrategyStatus: (status: StrategyInstanceItem["status"] | string) => string;
    formatRuntimeObservationSymbols: (symbols: string[] | null | undefined) => string;
    formatTimestamp: (value: unknown) => string;
    formatTimestampTooltip: (value: unknown) => string;
}>();

const dismissedRuntimeLastErrorKey = ref("");
const dismissedDetailsError = ref("");

const runtimeLastErrorMessage = computed(
    () => props.selectedStrategyRuntimeObservation?.lastError?.trim() ?? "",
);

const runtimeLastErrorKey = computed(() => [
    props.selectedStrategy.id,
    runtimeLastErrorMessage.value,
    props.selectedStrategyRuntimeObservation?.lastErrorAt ?? "",
].join("\n"));

const visibleRuntimeLastError = computed(
    () => runtimeLastErrorMessage.value !== "" && dismissedRuntimeLastErrorKey.value !== runtimeLastErrorKey.value,
);

const visibleDetailsError = computed(
    () => props.detailsError.trim() !== "" && dismissedDetailsError.value !== props.detailsError,
);
const activeInstrumentIds = computed(() =>
    normalizeStrategyInstrumentIds(
        props.selectedStrategyRuntimeObservation?.activeSymbols,
    ),
);

watch(
    () => props.detailsError,
    (message) => {
        if (message.trim() === "") {
            dismissedDetailsError.value = "";
        }
    },
);

watch(runtimeLastErrorMessage, (message) => {
    if (message === "") {
        dismissedRuntimeLastErrorKey.value = "";
    }
});

function closeRuntimeLastError(): void {
    dismissedRuntimeLastErrorKey.value = runtimeLastErrorKey.value;
}

function closeDetailsError(): void {
    dismissedDetailsError.value = props.detailsError;
}
</script>

<template>
    <div class="grid gap-4">
        <section class="runtime-workbench-section">
            <div class="flex flex-wrap items-start justify-between gap-3">
                <div>
                    <div class="text-sm font-semibold runtime-workbench-text-strong">运行控制</div>
                    <div class="mt-1 text-xs runtime-workbench-text-muted">
                        启动、暂停、停止都会同步刷新日志与审计视图。
                    </div>
                </div>
                <div class="flex flex-wrap gap-2">
                    <span class="runtime-workbench-pill">
                        {{ selectedStrategyRuntimeLabel }}
                    </span>
                    <span class="runtime-workbench-pill">
                        {{ selectedStrategySourceFormatLabel }}
                    </span>
                    <span
                        class="runtime-workbench-pill"
                        :class="selectedStrategy.startable
                            ? 'runtime-workbench-pill--success'
                            : 'runtime-workbench-pill--warning'"
                    >
                        {{ formatStrategyEligibility(selectedStrategy) }}
                    </span>
                    <span
                        v-if="selectedStrategyBinding !== null"
                        class="runtime-workbench-pill"
                        :class="selectedStrategyBinding.executionMode === 'notify_only'
                            ? 'runtime-workbench-pill--info'
                            : ''"
                    >
                        {{ formatStrategyExecutionMode(selectedStrategyBinding.executionMode) }}
                    </span>
                </div>
            </div>
            <div class="mt-3 text-sm runtime-workbench-text" data-testid="strategy-runtime-start-hint">
                {{ selectedStrategyStartHint }}
            </div>
            <div v-if="selectedStrategyCompiledSummary" class="mt-2 text-xs runtime-workbench-text-muted">
                {{ selectedStrategyCompiledSummary }}
            </div>
        </section>

        <section
            v-if="selectedStrategyRuntimeObservation !== null"
            class="runtime-workbench-section"
            data-testid="strategy-runtime-observation"
        >
            <div class="text-[11px] uppercase tracking-[0.18em] runtime-workbench-text-muted">实际运行态</div>
            <div class="mt-3 grid gap-3 text-sm sm:grid-cols-2 xl:grid-cols-3">
                <div>
                    <div class="runtime-workbench-field-label">运行状态</div>
                    <RuntimeHealthBadge
                        class="mt-1"
                        :status="selectedStrategyRuntimeObservation.actualStatus"
                        :label="formatStrategyStatus(selectedStrategyRuntimeObservation.actualStatus)"
                    />
                </div>
                <div>
                    <div class="runtime-workbench-field-label">活跃标的</div>
                    <div class="runtime-workbench-field-value flex flex-wrap items-center gap-1.5">
                        <template v-if="activeInstrumentIds.length > 0">
                            <InstrumentIdentity
                                v-for="symbol in activeInstrumentIds"
                                :key="symbol"
                                :instrument-id="symbol"
                                compact
                            />
                        </template>
                        <template v-else>
                            {{ formatRuntimeObservationSymbols(selectedStrategyRuntimeObservation.activeSymbols) }}
                        </template>
                    </div>
                </div>
                <div>
                    <div class="runtime-workbench-field-label">最近闭合 K 线</div>
                    <div class="runtime-workbench-field-value strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastClosedKlineAt)">
                        {{ formatTimestamp(selectedStrategyRuntimeObservation.lastClosedKlineAt) }}
                    </div>
                </div>
                <div>
                    <div class="runtime-workbench-field-label">最近信号</div>
                    <div class="runtime-workbench-field-value strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastSignalAt)">
                        {{ formatTimestamp(selectedStrategyRuntimeObservation.lastSignalAt) }}
                    </div>
                </div>
                <div>
                    <div class="runtime-workbench-field-label">最近下单</div>
                    <div class="runtime-workbench-field-value strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastOrderAt)">
                        {{ formatTimestamp(selectedStrategyRuntimeObservation.lastOrderAt) }}
                    </div>
                </div>
                <div>
                    <div class="runtime-workbench-field-label">最近更新</div>
                    <div class="runtime-workbench-field-value strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.updatedAt)">
                        {{ formatTimestamp(selectedStrategyRuntimeObservation.updatedAt) }}
                    </div>
                </div>
            </div>
            <RuntimeWorkbenchAlert
                v-if="visibleRuntimeLastError"
                class="mt-3 text-xs"
                close-label="关闭最近异常"
                close-test-id="strategy-runtime-last-error-close"
                tone="warning"
                @close="closeRuntimeLastError"
            >
                最近异常：{{ selectedStrategyRuntimeObservation.lastError }}
                <span class="strategy-time-display" :title="formatTimestampTooltip(selectedStrategyRuntimeObservation.lastErrorAt)">
                    （{{ formatTimestamp(selectedStrategyRuntimeObservation.lastErrorAt) }}）
                </span>
            </RuntimeWorkbenchAlert>
        </section>
        <section v-else class="runtime-workbench-empty p-5 text-sm">
            实例未运行时暂无实时观测信息。
        </section>

        <RuntimeWorkbenchAlert
            v-if="visibleDetailsError"
            close-label="关闭错误"
            close-test-id="strategy-runtime-details-error-close"
            tone="error"
            @close="closeDetailsError"
        >
            {{ detailsError }}
        </RuntimeWorkbenchAlert>
    </div>
</template>
