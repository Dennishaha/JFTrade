<script setup lang="ts">
import type {
  StrategyInstanceItem,
  StrategyRuntimeRiskMode,
  StrategyRuntimeRiskSettings,
} from "@/types";
import { formatStrategyRuntimeRiskSummary } from "@/components/strategy-runtime/strategyRuntimeInstanceBinding";
import { formatStrategyRuntimeStatus } from "@/composables/shared/consoleDataFormatting";

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

function statusClass(status: string): string {
  if (status === "RUNNING") return "tv-status--success";
  if (status === "PAUSED") return "tv-status--warning";
  return "tv-status--info";
}
</script>

<template>
  <section class="strategy-risk jf-panel" aria-label="策略实例动态风控">
    <header class="strategy-risk__head jf-panel__header">
      <span class="strategy-risk__title jf-panel__title">策略实例动态风控</span>
      <span class="strategy-risk__desc jf-panel__description">切换观察或执行模式，不需要停止实例。</span>
      <button
        type="button"
        class="tv-btn tv-btn-ghost strategy-risk__refresh"
        @click="emit('refresh')"
      >
        刷新
      </button>
    </header>

    <div class="strategy-risk__body jf-panel__body">
      <div
        v-if="error"
        class="strategy-risk__error tv-status--warning tv-status-surface"
        role="alert"
      >
        {{ error }}
      </div>

      <div v-if="instances.length" class="strategy-risk__grid">
        <div
          v-for="instance in instances"
          :key="instance.id"
          class="strategy-risk__instance"
        >
          <div class="strategy-risk__instance-info">
            <div class="strategy-risk__instance-head">
              <b>{{ instance.definition.name }}</b>
              <span
                class="strategy-risk__status tv-status-surface"
                :class="statusClass(instance.status)"
              >
                {{ formatStrategyRuntimeStatus(instance.status) }}
              </span>
            </div>
            <div class="strategy-risk__instance-id">{{ instance.id }}</div>
            <div class="strategy-risk__instance-summary">
              {{ formatStrategyRuntimeRiskSummary(runtimeRiskForInstance(instance.id)) }}
            </div>
          </div>
          <select
            :value="runtimeRiskForInstance(instance.id).mode"
            :disabled="isUpdating(instance.id)"
            class="tv-select strategy-risk__mode"
            :aria-label="`${instance.definition.name} 动态风控模式`"
            @change="emit('updateMode', instance.id, (($event.target as HTMLSelectElement).value as StrategyRuntimeRiskMode))"
          >
            <option value="off">关闭</option>
            <option value="monitor">观察</option>
            <option value="enforce">执行</option>
          </select>
        </div>
      </div>
      <div v-else class="strategy-risk__empty">
        当前没有策略实例。创建策略实例后可在这里控制动态风控。
      </div>
    </div>
  </section>
</template>

<style scoped>
.strategy-risk__head {
  gap: 10px;
  padding: 9px 12px;
}

.strategy-risk__refresh {
  height: 24px;
  padding: 0 10px;
  font-size: var(--jf-text-5);
}

.strategy-risk__body {
  gap: 10px;
  padding: 12px;
}

.strategy-risk__error {
  padding: 7px 10px;
  border: 1px solid;
  border-radius: 6px;
  font-size: var(--jf-text-5);
}

.strategy-risk__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.strategy-risk__instance {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 8.5rem;
  gap: 10px;
  align-items: center;
  padding: 10px 12px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.strategy-risk__instance-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.strategy-risk__instance-head b {
  color: var(--tv-text);
  font-size: var(--jf-text-6);
  font-weight: 600;
}

.strategy-risk__status {
  padding: 2px 8px;
  border: 1px solid;
  border-radius: 999px;
  font-size: var(--jf-text-4);
  white-space: nowrap;
}

.strategy-risk__instance-id {
  overflow: hidden;
  margin-top: 2px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-risk__instance-summary {
  margin-top: 4px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.strategy-risk__mode:disabled {
  cursor: wait;
  opacity: 0.6;
}

.strategy-risk__empty {
  padding: 14px 2px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-5);
}

@media (max-width: 1180px) {
  .strategy-risk__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
