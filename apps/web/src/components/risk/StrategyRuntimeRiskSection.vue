<script setup lang="ts">
import type {
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "@/contracts";
import { formatStrategyRuntimeRiskSummary } from "@/components/strategy-runtime/strategyRuntimeInstanceBinding";
import { formatStrategyRuntimeStatus } from "@/composables/consoleDataFormatting";

defineProps<{
  error: string;
  instances: StrategyInstanceItem[];
  isUpdating: (instanceId: string) => boolean;
  runtimeRiskForInstance: (instanceId: string) => StrategyRuntimeRiskSettings;
}>();

const emit = defineEmits<{
  refresh: [];
  updateMode: [instanceId: string, mode: StrategyRuntimeRiskMode];
}>();

</script>

<template>
  <v-card flat class="card-shell border-0">
    <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
      <div>
        <div class="text-xl font-semibold text-slate-900">策略实例动态风控</div>
        <div class="mt-1 text-sm text-slate-500">切换观察或执行模式，不需要停止实例。</div>
      </div>
      <v-btn variant="text" color="primary" size="small" @click="emit('refresh')">
        刷新
      </v-btn>
    </div>

    <v-card-text>
      <v-alert
        v-if="error"
        type="warning"
        variant="tonal"
        density="compact"
        :closable="false"
        class="mb-3"
      >
        {{ error }}
      </v-alert>

      <div v-if="instances.length" class="grid gap-3 lg:grid-cols-2">
        <div
          v-for="instance in instances"
          :key="instance.id"
          class="grid gap-3 rounded-lg border border-slate-200 bg-white px-4 py-4 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <div class="font-semibold text-slate-900">{{ instance.definition.name }}</div>
              <v-chip
                :color="instance.status === 'RUNNING' ? 'success' : instance.status === 'PAUSED' ? 'warning' : undefined"
                variant="outlined"
                size="small"
              >
                {{ formatStrategyRuntimeStatus(instance.status) }}
              </v-chip>
            </div>
            <div class="mt-1 truncate text-xs text-slate-500">{{ instance.id }}</div>
            <div class="mt-2 text-xs font-medium text-slate-700">
              {{ formatStrategyRuntimeRiskSummary(runtimeRiskForInstance(instance.id)) }}
            </div>
          </div>
          <select
            :value="runtimeRiskForInstance(instance.id).mode"
            :disabled="isUpdating(instance.id)"
            class="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none disabled:cursor-wait disabled:opacity-60"
            :aria-label="`${instance.definition.name} 动态风控模式`"
            @change="emit('updateMode', instance.id, (($event.target as HTMLSelectElement).value as StrategyRuntimeRiskMode))"
          >
            <option value="off">关闭</option>
            <option value="monitor">观察</option>
            <option value="enforce">执行</option>
          </select>
        </div>
      </div>
      <v-empty-state v-else text="当前没有策略实例。创建策略实例后可在这里控制动态风控。" />
    </v-card-text>
  </v-card>
</template>
