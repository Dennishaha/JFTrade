<script setup lang="ts">
import type { ADKRun } from "@/types";

import ADKWorkflowQueuePanel from "./ADKWorkflowQueuePanel.vue";

defineProps<{
  run?: ADKRun | null | undefined;
}>();
</script>

<template>
  <ADKWorkflowQueuePanel :run="run" />
</template>
