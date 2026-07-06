<script setup lang="ts">
import StrategyRuntimeSummaryStats from "./StrategyRuntimeSummaryStats.vue";

defineProps<{
    activeStrategyCount: number;
    definitionsCount: number;
    defaultTradingEnvironment: string | null | undefined;
    runtimeRealTradingLabel: string;
    isKillSwitchActive: boolean;
    runtimeKillSwitchLabel: string;
    runtimeRiskSummary: string;
}>();
</script>

<template>
    <div class="runtime-panel__bar">
        <div class="runtime-panel__intro">
            <div class="runtime-panel__eyebrow">策略运行时</div>
            <div class="runtime-panel__title-row">
                <div class="runtime-panel__title">策略运行</div>
            </div>
        </div>
        <div class="runtime-panel__bar-actions">
            <StrategyRuntimeSummaryStats
                :active-strategy-count="activeStrategyCount"
                :definitions-count="definitionsCount"
                :default-trading-environment="defaultTradingEnvironment"
                :runtime-real-trading-label="runtimeRealTradingLabel"
                :is-kill-switch-active="isKillSwitchActive"
                :runtime-kill-switch-label="runtimeKillSwitchLabel"
                :runtime-risk-summary="runtimeRiskSummary"
            />
        </div>
    </div>
</template>
