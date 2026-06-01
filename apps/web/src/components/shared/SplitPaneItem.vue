<script setup lang="ts">
import { computed } from "vue";
import { Pane } from "splitpanes";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    minSize?: number;
    maxSize?: number;
    size?: number | null;
  }>(),
  {
    minSize: 8,
    maxSize: 100,
    size: null,
  },
);

defineSlots<{
  default(): unknown;
}>();

const paneBind = computed(() => {
  const bindings: Record<string, number> = {
    "min-size": props.minSize ?? 8,
    "max-size": props.maxSize ?? 100,
  };
  if (props.size != null) {
    bindings["size"] = props.size;
  }
  return bindings;
});
</script>

<template>
  <Pane class="tv-pane" v-bind="paneBind">
    <slot />
  </Pane>
</template>
