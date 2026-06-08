<script setup lang="ts">
import type { ADKToolVisualization } from "../../composables/adkToolVisualizations";

defineProps<{
  visualization: ADKToolVisualization;
}>();

function toneClass(tone: string | undefined): string {
  return tone ? `is-${tone}` : "is-muted";
}
</script>

<template>
  <section class="adk-tool-visual">
    <div class="adk-tool-visual__header">
      <div class="adk-tool-visual__title">{{ visualization.title }}</div>
      <div v-if="visualization.subtitle" class="adk-tool-visual__subtitle">{{ visualization.subtitle }}</div>
    </div>

    <div v-if="visualization.kind === 'summary'" class="adk-tool-visual__summary">
      <div class="adk-tool-visual__cards">
        <div
          v-for="card in visualization.cards"
          :key="`${card.label}:${card.value}`"
          class="adk-tool-visual-card"
          :class="toneClass(card.tone)"
        >
          <span class="adk-tool-visual-card__label">{{ card.label }}</span>
          <strong class="adk-tool-visual-card__value">{{ card.value }}</strong>
        </div>
      </div>
      <dl v-if="visualization.rows?.length" class="adk-tool-visual__rows">
        <template v-for="row in visualization.rows" :key="row.label">
          <dt>{{ row.label }}</dt>
          <dd>{{ row.value }}</dd>
        </template>
      </dl>
    </div>

    <div v-else-if="visualization.kind === 'table'" class="adk-tool-visual-table-wrap">
      <table class="adk-tool-visual-table">
        <thead>
          <tr>
            <th v-for="column in visualization.columns" :key="column.key">{{ column.label }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, rowIndex) in visualization.rows" :key="rowIndex">
            <td v-for="column in visualization.columns" :key="column.key">{{ row[column.key] ?? "-" }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-else-if="visualization.kind === 'depth'" class="adk-tool-visual-depth">
      <div class="adk-tool-visual-depth__side">
        <div class="adk-tool-visual-depth__label">Bids</div>
        <div v-for="(row, index) in visualization.bids" :key="`bid-${index}`" class="adk-tool-visual-depth-row is-bid">
          <span>{{ row.price }}</span>
          <span>{{ row.quantity }}</span>
          <i :style="{ width: `${row.percent}%` }" />
        </div>
      </div>
      <div class="adk-tool-visual-depth__side">
        <div class="adk-tool-visual-depth__label">Asks</div>
        <div v-for="(row, index) in visualization.asks" :key="`ask-${index}`" class="adk-tool-visual-depth-row is-ask">
          <span>{{ row.price }}</span>
          <span>{{ row.quantity }}</span>
          <i :style="{ width: `${row.percent}%` }" />
        </div>
      </div>
    </div>

    <ol v-else-if="visualization.kind === 'timeline'" class="adk-tool-visual-timeline">
      <li
        v-for="(event, index) in visualization.events"
        :key="`${event.label}:${event.time ?? index}`"
        class="adk-tool-visual-timeline__item"
        :class="toneClass(event.tone)"
      >
        <span class="adk-tool-visual-timeline__dot" />
        <span class="adk-tool-visual-timeline__main">
          <strong>{{ event.label }}</strong>
          <small v-if="event.time">{{ event.time }}</small>
          <span v-if="event.detail">{{ event.detail }}</span>
        </span>
      </li>
    </ol>
  </section>
</template>
