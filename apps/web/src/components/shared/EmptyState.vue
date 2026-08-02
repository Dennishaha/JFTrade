<script setup lang="ts">
import { computed } from "vue";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    loading?: boolean;
    error?: string | null;
    empty?: boolean;
    loadingLabel?: string;
    emptyLabel?: string;
    bordered?: boolean;
    grow?: boolean;
    minHeight?: number;
  }>(),
  {
    loading: false,
    error: null,
    empty: false,
    loadingLabel: "加载中…",
    emptyLabel: "暂无数据",
    bordered: false,
    grow: false,
    minHeight: 120,
  },
);

const active = computed(() =>
  Boolean(props.loading || props.error || props.empty),
);
const label = computed(() => {
  if (props.loading) return props.loadingLabel;
  if (props.error) return props.error;
  return props.emptyLabel;
});
</script>

<template>
  <div
    v-if="active"
    v-bind="$attrs"
    class="empty-state"
    :class="{
      'empty-state--bordered': bordered,
      'empty-state--grow': grow,
    }"
    :style="{ minHeight: `${minHeight}px` }"
  >
    {{ label }}
  </div>
  <slot v-else />
</template>

<style scoped>
.empty-state {
  display: grid;
  place-items: center;
  color: var(--tv-text-dim);
}

.empty-state--bordered {
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-md);
  background: var(--tv-bg-surface);
}

.empty-state--grow {
  flex: 1;
}
</style>
