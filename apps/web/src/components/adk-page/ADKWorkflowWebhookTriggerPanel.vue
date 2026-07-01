<script setup lang="ts">
import type { ADKWorkflowTrigger, ADKWorkflowTriggerLog } from "@/contracts";
import type { TriggerFormModel } from "@/features/adkWorkflowForms";
import { statusLabel } from "@/features/adkWorkflowStudio";

defineProps<{
  triggerForm: TriggerFormModel;
  selectedTrigger: ADKWorkflowTrigger | null;
  triggerRunSummary: {
    total: number;
    latest: ADKWorkflowTriggerLog | null;
    failures: number;
  } | null;
  webhookEndpoint: string;
  webhookCurlSample: string;
  formatDateTime: (value: string) => string;
}>();
</script>

<template>
  <div class="adk-inspector-section is-inner">
    <h3>网络回调</h3>
    <v-text-field :model-value="webhookEndpoint" label="回调地址" density="comfortable" readonly />
    <v-switch
      v-model="triggerForm.resetSecret"
      label="重置回调密钥"
      color="primary"
      hide-details
    />
    <div class="adk-trigger-health">
      <strong>请求样例</strong>
      <pre>{{ webhookCurlSample }}</pre>
      <small>
        {{ triggerForm.id ? (selectedTrigger?.hasSecret ? "密钥已生成" : "密钥未生成") : "保存后生成回调地址和密钥" }}
      </small>
      <small v-if="triggerRunSummary?.latest">
        最近请求：{{ statusLabel(triggerRunSummary.latest.status) }} ·
        {{ formatDateTime(triggerRunSummary.latest.createdAt) }}
      </small>
    </div>
  </div>
</template>
