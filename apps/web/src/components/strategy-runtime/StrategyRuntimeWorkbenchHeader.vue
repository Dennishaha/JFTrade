<script setup lang="ts">
import { computed } from "vue";
import type { StrategyInstanceItem } from "@/contracts";

import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import RuntimeHealthBadge from "../domain/runtime/RuntimeHealthBadge.vue";
import { parseStrategyInstrumentIdsText } from "./strategyRuntimeInstanceBinding";

type StrategyAction = "start" | "pause" | "stop";

const props = defineProps<{
    selectedStrategy: StrategyInstanceItem;
    selectedRuntimeStatus: StrategyInstanceItem["status"] | string;
    selectedRuntimeStatusLabel: string;
    selectedStrategyRuntimeLabel: string;
    isRefreshingStrategyContent: boolean;
    canStartSelectedStrategy: boolean;
    canPauseSelectedStrategy: boolean;
    canStopSelectedStrategy: boolean;
    formatStrategySymbols: (strategy: StrategyInstanceItem) => string;
    formatStrategyInterval: (strategy: StrategyInstanceItem) => string;
}>();

const strategyInstrumentIds = computed(() =>
    parseStrategyInstrumentIdsText(props.formatStrategySymbols(props.selectedStrategy)),
);

const emit = defineEmits<{
    "refresh-content": [];
    "change-status": [action: StrategyAction];
}>();
</script>

<template>
    <header class="runtime-workbench-panel__header border-b px-4 py-3">
        <div class="runtime-workbench-header-row">
            <div class="runtime-workbench-header-main min-w-0">
                <div class="text-xs uppercase tracking-[0.16em] runtime-workbench-text-muted">运行操作台</div>
                <div class="mt-1 break-words text-lg font-semibold runtime-workbench-text-strong">
                    {{ selectedStrategy.definition.name }}
                </div>
                <div class="mt-1 flex flex-wrap items-center gap-2 text-xs runtime-workbench-text-muted">
                    <span>{{ selectedStrategy.id }}</span>
                    <span class="inline-flex flex-wrap items-center gap-1">
                        <template v-if="strategyInstrumentIds.length > 0">
                            <InstrumentIdentity
                                v-for="symbol in strategyInstrumentIds"
                                :key="symbol"
                                :instrument-id="symbol"
                                compact
                            />
                        </template>
                        <template v-else>{{ formatStrategySymbols(selectedStrategy) }}</template>
                    </span>
                    <span>{{ formatStrategyInterval(selectedStrategy) }}</span>
                    <span>{{ selectedStrategyRuntimeLabel }}</span>
                </div>
            </div>

            <div class="runtime-workbench-header-actions">
                <RuntimeHealthBadge
                    :status="selectedRuntimeStatus"
                    :label="selectedRuntimeStatusLabel"
                />
                <button
                    class="runtime-workbench-button"
                    data-testid="strategy-refresh-content"
                    :disabled="isRefreshingStrategyContent"
                    type="button"
                    @click="emit('refresh-content')"
                >
                    {{ isRefreshingStrategyContent ? "等待" : "刷新" }}
                </button>
                <button
                    class="runtime-workbench-action runtime-workbench-action--start"
                    data-testid="strategy-start"
                    :disabled="!canStartSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'start')"
                >
                    启动
                </button>
                <button
                    class="runtime-workbench-action runtime-workbench-action--pause"
                    data-testid="strategy-pause"
                    :disabled="!canPauseSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'pause')"
                >
                    暂停
                </button>
                <button
                    class="runtime-workbench-action runtime-workbench-action--stop"
                    data-testid="strategy-stop"
                    :disabled="!canStopSelectedStrategy"
                    type="button"
                    @click="emit('change-status', 'stop')"
                >
                    停止
                </button>
            </div>
        </div>
    </header>
</template>
