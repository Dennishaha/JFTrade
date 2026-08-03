<script setup lang="ts">
type RiskTone = "success" | "warning" | "error";

defineProps<{
  posture: { label: string; tone: RiskTone; hint: string };
  statusRows: Array<{ key: string; label: string; value: string; tone: RiskTone }>;
  facts: Array<{ label: string; value: string }>;
}>();

defineEmits<{ refresh: [] }>();
</script>

<template>
  <aside class="risk-sidebar" aria-label="风控态势摘要">
    <div class="risk-sidebar__head">
      <div class="risk-sidebar__name">风控中心</div>
      <span class="risk-sidebar__posture-dot" :class="`tv-status--${posture.tone}`">
        <i class="tv-state-dot"></i>{{ posture.label }}
      </span>
    </div>
    <div
      class="risk-sidebar__posture"
      :class="`tv-status--${posture.tone}`"
      data-testid="risk-posture"
    >
      <div class="risk-sidebar__posture-label">整体风险态势</div>
      <div class="risk-sidebar__posture-value">{{ posture.label }}</div>
      <div class="risk-sidebar__posture-hint">{{ posture.hint }}</div>
    </div>
    <div class="risk-sidebar__rows">
      <div
        v-for="row in statusRows"
        :key="row.key"
        class="risk-sidebar__row"
        :class="`tv-status--${row.tone}`"
        :data-status-key="row.key"
      >
        <span>{{ row.label }}</span><b>{{ row.value }}</b>
      </div>
    </div>
    <div class="risk-sidebar__facts">
      <div v-for="fact in facts" :key="fact.label" class="risk-sidebar__fact">
        <span>{{ fact.label }}</span><b :title="fact.value">{{ fact.value }}</b>
      </div>
    </div>
    <div class="risk-sidebar__footer">
      <button
        type="button"
        class="tv-btn tv-btn-ghost risk-sidebar__refresh"
        @click="$emit('refresh')"
      >
        刷新风控状态
      </button>
    </div>
  </aside>
</template>

<style scoped>
.risk-sidebar {
  display: flex;
  width: 264px;
  flex: 0 0 auto;
  flex-direction: column;
  overflow: hidden auto;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, var(--jf-shadow-color) 8%, transparent);
  scrollbar-width: thin;
}
.risk-sidebar__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}
.risk-sidebar__name {
  overflow: hidden;
  color: var(--tv-text);
  font-size: var(--jf-text-7);
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.risk-sidebar__posture-dot {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-dim));
  font-size: var(--jf-text-4);
}
.risk-sidebar__posture { padding: 14px; border-bottom: 1px solid var(--tv-border); }
.risk-sidebar__posture-label { color: var(--tv-text-muted); font-size: var(--jf-text-5); }
.risk-sidebar__posture-value {
  margin-top: 4px;
  color: var(--tv-status-fg, var(--tv-text));
  font-size: var(--jf-text-15);
  font-weight: 680;
  letter-spacing: var(--jf-tracking-tight);
}
.risk-sidebar__posture-hint {
  margin-top: 6px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
  line-height: 1.6;
}
.risk-sidebar__rows {
  display: grid;
  gap: 2px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--tv-border);
}
.risk-sidebar__row,
.risk-sidebar__fact {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 0;
}
.risk-sidebar__row { font-size: var(--jf-text-6); }
.risk-sidebar__row span { color: var(--tv-text-muted); }
.risk-sidebar__row b { color: var(--tv-status-fg, var(--tv-text)); font-weight: 550; }
.risk-sidebar__facts { display: grid; gap: 2px; padding: 10px 14px 14px; }
.risk-sidebar__fact { gap: 10px; font-size: var(--jf-text-5); }
.risk-sidebar__fact span { flex: 0 0 auto; color: var(--tv-text-dim); }
.risk-sidebar__fact b {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.risk-sidebar__footer { margin-top: auto; padding: 10px 14px 14px; }
.risk-sidebar__refresh { width: 100%; height: 30px; font-size: var(--jf-text-6); }
@media (max-width: 1180px) {
  .risk-sidebar { width: 100%; flex: 0 0 auto; }
  .risk-sidebar__facts { grid-template-columns: 1fr 1fr; column-gap: 16px; }
}
</style>
