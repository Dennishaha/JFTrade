<script setup lang="ts">
import { Splitpanes, Pane } from "splitpanes";

defineOptions({ inheritAttrs: false });

withDefaults(
  defineProps<{
    horizontal?: boolean;
    pushOtherPanes?: boolean;
    maximizePanes?: boolean;
    rtl?: boolean;
    firstSplitter?: boolean;
    keyboardStep?: number;
    paneMinSize?: number;
    paneMaxSize?: number;
  }>(),
  {
    horizontal: false,
    pushOtherPanes: true,
    maximizePanes: true,
    rtl: false,
    firstSplitter: false,
    keyboardStep: 5,
    paneMinSize: 8,
    paneMaxSize: 100,
  },
);

defineSlots<{
  default(): unknown;
}>();
</script>

<template>
  <Splitpanes
    class="tv-splitpanes"
    :class="{ 'tv-splitpanes--horizontal': horizontal, 'tv-splitpanes--vertical': !horizontal }"
    v-bind="$attrs"
    :horizontal="horizontal"
    :push-other-panes="pushOtherPanes"
    :maximize-panes="maximizePanes"
    :rtl="rtl"
    :first-splitter="firstSplitter"
    :keyboard-step="keyboardStep"
    :pane-min-size="paneMinSize"
    :pane-max-size="paneMaxSize"
  >
    <slot />
  </Splitpanes>
</template>
