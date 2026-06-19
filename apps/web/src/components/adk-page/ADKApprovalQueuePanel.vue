<script setup lang="ts">
import { computed } from "vue";

import type { ADKApproval, ADKToolDescriptor } from "@/contracts";

import {
  type ADKApprovalQueueItem,
} from "../../composables/useADKWorkflowQueueState";
import ADKQueuePanel from "./ADKQueuePanel.vue";

const props = defineProps<{
  items: ADKApprovalQueueItem[];
  approvalsBusy: boolean;
  approvalTool: (approval: ADKApproval) => ADKToolDescriptor | undefined;
  preview: (value: unknown) => string;
  resolveApprovalGroup: (
    approvals: ADKApproval[],
    approved: boolean,
  ) => void | Promise<void>;
  resolveApproval: (
    approval: ADKApproval,
    approved: boolean,
  ) => void | Promise<void>;
}>();

const approvals = computed(() => props.items.map((item) => item.approval));
const summary = computed(() => {
  const first = props.items[0];
  if (!first) return "";
  const source = first.childIndex ? `子智能体 #${first.childIndex} · ` : "";
  return `${source}${first.approval.toolName}`;
});

function sourceLabel(item: ADKApprovalQueueItem): string {
  if (item.childIndex) {
    return `子智能体 #${item.childIndex}${item.stepTitle ? ` · ${item.stepTitle}` : ""}`;
  }
  return "父 Agent";
}
</script>

<template>
  <ADKQueuePanel
    v-if="items.length > 0"
    title="待审批"
    :count="items.length"
    status="PENDING_APPROVAL"
    :summary="summary"
  >
    <div class="adk-approval-queue__bulk">
      <button
        type="button"
        class="adk-workspace-queue-button"
        :disabled="approvalsBusy"
        @click="resolveApprovalGroup(approvals, true)"
      >
        全部批准
      </button>
      <button
        type="button"
        class="adk-workspace-queue-button is-danger"
        :disabled="approvalsBusy"
        @click="resolveApprovalGroup(approvals, false)"
      >
        全部拒绝
      </button>
    </div>

    <div class="adk-workspace-queue-list">
      <div
        v-for="(item, index) in items"
        :key="item.approval.id"
        class="adk-workspace-queue-item adk-approval-queue__item"
      >
        <span class="adk-workspace-queue-item__index">#{{ index + 1 }}</span>
        <span class="adk-workspace-queue-status is-warning">PENDING</span>
        <div class="adk-workspace-queue-item__main">
          <div class="adk-workspace-queue-item__title">
            {{ item.approval.toolName }}
            <span v-if="approvalTool(item.approval)" class="adk-approval-queue__risk">
              {{ approvalTool(item.approval)?.riskLevel ?? "unknown" }} risk
            </span>
          </div>
          <div class="adk-workspace-queue-item__meta">
            <span>{{ sourceLabel(item) }}</span>
            <span v-if="item.approval.reason"> · {{ item.approval.reason }}</span>
            <span> · Run {{ item.runId }}</span>
          </div>
          <pre v-if="item.approval.input" class="adk-approval-queue__input">{{ preview(item.approval.input) }}</pre>
        </div>
        <div class="adk-workspace-queue-item__actions">
          <button
            type="button"
            class="adk-workspace-queue-button"
            :disabled="approvalsBusy"
            @click="resolveApproval(item.approval, true)"
          >
            批准
          </button>
          <button
            type="button"
            class="adk-workspace-queue-button is-danger"
            :disabled="approvalsBusy"
            @click="resolveApproval(item.approval, false)"
          >
            拒绝
          </button>
        </div>
      </div>
    </div>
  </ADKQueuePanel>
</template>

<style scoped>
.adk-approval-queue__bulk {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.adk-approval-queue__item {
  align-items: start;
}

.adk-approval-queue__risk {
  margin-left: 6px;
  color: var(--adk-warning-fg);
  font-size: 11px;
  font-weight: 500;
}

.adk-approval-queue__input {
  max-height: 120px;
  overflow: auto;
  margin: 6px 0 0;
  padding: 8px;
  border-radius: 8px;
  border: 1px solid var(--tv-border);
  background: color-mix(in srgb, var(--tv-bg-surface-2) 92%, transparent);
  color: var(--tv-text-muted);
  font-size: 11px;
  white-space: pre-wrap;
}
</style>
