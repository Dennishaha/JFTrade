<script setup lang="ts">
import type { ADKWorkflowNodeRun } from "@/contracts";
import {
  logTone,
  nodeRunDetails,
  runDurationLabel,
  statusLabel,
} from "@/features/adkWorkflowStudio";

defineProps<{
  run: ADKWorkflowNodeRun | null;
}>();
</script>

<template>
  <div v-if="run" class="adk-node-run-preview">
    <h3>最近节点运行</h3>
    <span class="adk-workflow-pill" :class="logTone(run.status)">
      {{ statusLabel(run.status) }}
    </span>
    <small>{{ runDurationLabel(run) }}</small>
    <pre>{{ nodeRunDetails(run) }}</pre>
  </div>
</template>
