<script setup lang="ts">
type RiskTone = "success" | "warning" | "error";

defineProps<{
  posture: { tone: RiskTone; hint: string };
  sections: Array<{
    title: string;
    items: Array<{ label: string; value: string; tone?: RiskTone }>;
  }>;
}>();
</script>

<template>
  <div class="risk-strip" aria-label="风控指标">
    <section v-for="section in sections" :key="section.title" class="risk-strip__section">
      <header class="risk-strip__title">
        {{ section.title }}
        <i
          v-if="section.title === '实盘总闸'"
          class="tv-state-dot"
          :class="`tv-status--${posture.tone}`"
          :title="posture.hint"
        ></i>
      </header>
      <div class="risk-strip__grid">
        <div v-for="item in section.items" :key="item.label" class="risk-strip__item">
          <span>{{ item.label }}</span>
          <b :class="item.tone ? `tv-status--${item.tone}` : undefined">{{ item.value }}</b>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.risk-strip {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}
.risk-strip__section {
  min-width: 0;
  padding: 10px 14px 12px;
  border-left: 1px solid var(--tv-border);
}
.risk-strip__section:first-child { border-left: 0; }
.risk-strip__title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-4);
  font-weight: 650;
  letter-spacing: var(--jf-tracking-5);
  text-transform: uppercase;
}
.risk-strip__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 12px;
}
.risk-strip__item { min-width: 0; }
.risk-strip__item span { display: block; color: var(--tv-text-dim); font-size: var(--jf-text-4); }
.risk-strip__item b {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  color: var(--tv-status-fg, var(--tv-text));
  font-size: var(--jf-text-6);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (max-width: 1180px) {
  .risk-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .risk-strip__section:nth-child(odd) { border-left: 0; }
  .risk-strip__section:nth-child(n + 3) { border-top: 1px solid var(--tv-border); }
}
</style>
