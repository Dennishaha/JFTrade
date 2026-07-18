<script setup lang="ts">
withDefaults(
  defineProps<{
    loading?: boolean;
  }>(),
  {
    loading: false,
  },
);

defineEmits<{
  refresh: [];
}>();
</script>

<template>
  <v-btn
    class="product-toolbar-refresh"
    icon="fa-solid fa-rotate"
    size="x-small"
    variant="text"
    :loading="loading"
    title="刷新"
    aria-label="刷新"
    @click="$emit('refresh')"
  />
</template>

<style scoped>
.product-toolbar-refresh {
  width: 24px !important;
  min-width: 24px !important;
  height: 24px !important;
  flex: 0 0 24px;
  padding: 0 !important;
  color: var(--tv-text-dim);
}

.product-toolbar-refresh:hover {
  color: var(--tv-text);
}

.product-toolbar-refresh :deep(.v-icon) {
  font-size: 10px;
}
</style>
