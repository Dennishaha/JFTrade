<script setup lang="ts">
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
    <div class="runtime-workbench-stats runtime-panel__top-stats">
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">活跃实例</div>
            <div class="runtime-workbench-stat__value">{{ activeStrategyCount }}</div>
        </div>
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">策略定义</div>
            <div class="runtime-workbench-stat__value">{{ definitionsCount }}</div>
        </div>
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">交易环境</div>
            <div class="runtime-workbench-stat__value">{{ defaultTradingEnvironment }}</div>
        </div>
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">实盘开关</div>
            <div class="runtime-workbench-stat__value">{{ runtimeRealTradingLabel }}</div>
        </div>
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">急停开关</div>
            <div
                class="runtime-workbench-stat__value"
                :class="isKillSwitchActive ? 'runtime-workbench-text-danger' : 'runtime-workbench-text-good'"
            >
                {{ runtimeKillSwitchLabel }}
            </div>
        </div>
        <div class="runtime-workbench-stat">
            <div class="runtime-workbench-stat__label">动态风控</div>
            <div class="runtime-workbench-stat__value">
                {{ runtimeRiskSummary }}
            </div>
        </div>
    </div>
</template>
