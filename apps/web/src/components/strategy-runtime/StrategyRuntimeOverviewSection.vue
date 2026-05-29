<script setup lang="ts">
import type { StrategyInstanceItem, SystemStatusResponse } from "@jftrade/ui-contracts";

defineProps<{
    activeStrategyCount: number;
    selectedStrategy: StrategyInstanceItem | null;
    selectedStrategyRuntimeLabel: string;
    systemStatus: SystemStatusResponse;
}>();
</script>

<template>
    <div class="grid gap-4 lg:grid-cols-[1fr_1fr]">
        <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
            <div class="flex items-center justify-between gap-3">
                <div class="text-xl font-semibold text-slate-900">运行总览</div>
                <span
                    class="rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-slate-600"
                >
                    {{ activeStrategyCount }} 个运行中
                </span>
            </div>
            <div class="mt-4 grid gap-3 md:grid-cols-3">
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">交易环境</div>
                    <div class="mt-2 text-2xl font-semibold text-slate-900">
                        {{ systemStatus.defaultTradingEnvironment }}
                    </div>
                </div>
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">当前策略</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ selectedStrategy?.definition.name ?? "暂无" }}
                    </div>
                </div>
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">运行形态</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900" data-testid="strategy-runtime-mode">
                        {{ selectedStrategyRuntimeLabel }}
                    </div>
                </div>
            </div>
            <div class="mt-4 text-sm text-slate-600">
                运行面板负责实例控制、日志和审计；新策略统一使用 DSL 编译计划生命周期。
            </div>
        </div>

        <div class="rounded-[28px] border border-slate-200 bg-slate-50/70 p-4">
            <div class="text-xl font-semibold text-slate-900">运行保护</div>
            <div class="mt-4 grid gap-3 md:grid-cols-3">
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">实盘开关</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ systemStatus.realTradingEnabled ? "已开启" : "已关闭" }}
                    </div>
                </div>
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">急停开关</div>
                    <div
                        class="mt-2 text-xl font-semibold"
                        :class="systemStatus.realTradingKillSwitch.active ? 'text-red-600' : 'text-teal-700'"
                    >
                        {{ systemStatus.realTradingKillSwitch.active ? "已启用" : "未启用" }}
                    </div>
                </div>
                <div class="rounded-3xl bg-white px-4 py-4">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最大下单数量</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ systemStatus.realTradingRisk.maxOrderQuantity ?? "暂无" }}
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>