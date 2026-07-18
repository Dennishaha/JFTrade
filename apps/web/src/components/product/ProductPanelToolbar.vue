<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string;
    description?: string;
  }>(),
  { description: "" },
);
</script>

<template>
  <header class="product-panel-toolbar">
    <div class="product-panel-toolbar__identity">
      <strong>{{ title }}</strong>
      <span v-if="description">{{ description }}</span>
    </div>
    <div v-if="$slots.default" class="product-panel-toolbar__content">
      <slot />
    </div>
  </header>
</template>

<style scoped>
.product-panel-toolbar {
  display: flex;
  min-height: 38px;
  flex: 0 0 auto;
  align-items: center;
  gap: 10px;
  padding: 4px 10px;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.product-panel-toolbar__identity,
.product-panel-toolbar__content {
  display: flex;
  min-width: 0;
  align-items: center;
}

.product-panel-toolbar__identity {
  flex: 0 1 auto;
  gap: 7px;
  white-space: nowrap;
}

.product-panel-toolbar__identity strong {
  overflow: hidden;
  font-size: 11px;
  font-weight: 700;
  text-overflow: ellipsis;
}

.product-panel-toolbar__identity span {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 9px;
  text-overflow: ellipsis;
}

.product-panel-toolbar__content {
  flex: 1;
  justify-content: flex-end;
  gap: 8px;
  overflow-x: auto;
}

@media (max-width: 720px) {
  .product-panel-toolbar {
    gap: 7px;
    padding-inline: 8px;
  }

  .product-panel-toolbar__identity span {
    display: none;
  }
}
</style>
