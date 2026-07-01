<script setup lang="ts">
import type { ADKWorkflowTriggerLog } from "@/contracts";
import type { PageEnvelope } from "@/composables/adkWorkflowsApi";
import {
  formatDurationMs,
  formatJson,
  logStatusOptions,
  logTone,
  nodeTypeLabel,
  projectedNodeRuns,
  runDurationLabel,
  statusLabel,
  triggerTypeLabel,
} from "@/features/adkWorkflowStudio";

defineProps<{
  workflowName: string;
  selectedNodeId: string;
  selectedLog: ADKWorkflowTriggerLog | null;
  visibleLogs: ADKWorkflowTriggerLog[];
  workflowStats: {
    total: number;
    successRate: number;
    avgMs: number;
    recent: number;
  };
  logTriggerOptions: Array<{ title: string; value: string }>;
  logStatusFilter: string;
  logTriggerFilter: string;
  logKeywordFilter: string;
  logFromFilter: string;
  logToFilter: string;
  logLoading: boolean;
  logPage: PageEnvelope;
  logPageSummary: string;
  formatDateTime: (value: string) => string;
  runLink: (log: ADKWorkflowTriggerLog) => string;
}>();

defineEmits<{
  refreshLogs: [];
  selectLog: [logId: string];
  selectNode: [nodeId: string];
  copyResultMarkdown: [];
  previousLogPage: [];
  nextLogPage: [];
  "update:logStatusFilter": [value: string];
  "update:logTriggerFilter": [value: string];
  "update:logKeywordFilter": [value: string];
  "update:logFromFilter": [value: string];
  "update:logToFilter": [value: string];
}>();
</script>

<template>
  <section class="adk-inspector-section adk-monitor-panel">
    <div class="adk-inspector-heading">
      <h3>运行监控</h3>
      <v-btn size="small" variant="text" :loading="logLoading" @click="$emit('refreshLogs')">
        刷新
      </v-btn>
    </div>
    <div class="adk-workflow-stat-grid">
      <div class="adk-workflow-stat">
        <strong>{{ workflowStats.total }}</strong>
        <span>运行次数</span>
      </div>
      <div class="adk-workflow-stat">
        <strong>{{ workflowStats.successRate }}%</strong>
        <span>成功率</span>
      </div>
      <div class="adk-workflow-stat">
        <strong>{{ formatDurationMs(workflowStats.avgMs) }}</strong>
        <span>平均耗时</span>
      </div>
      <div class="adk-workflow-stat">
        <strong>{{ workflowStats.recent }}</strong>
        <span>24 小时触发</span>
      </div>
    </div>
    <div class="adk-monitor-filters">
      <v-select
        :model-value="logStatusFilter"
        :items="logStatusOptions"
        label="状态"
        density="compact"
        hide-details
        @update:model-value="$emit('update:logStatusFilter', String($event ?? ''))"
      />
      <v-select
        :model-value="logTriggerFilter"
        :items="logTriggerOptions"
        label="触发器"
        density="compact"
        hide-details
        @update:model-value="$emit('update:logTriggerFilter', String($event ?? ''))"
      />
      <v-text-field
        :model-value="logKeywordFilter"
        label="关键词"
        density="compact"
        hide-details
        @update:model-value="$emit('update:logKeywordFilter', String($event ?? ''))"
      />
      <v-text-field
        :model-value="logFromFilter"
        label="开始日期"
        type="date"
        density="compact"
        hide-details
        @update:model-value="$emit('update:logFromFilter', String($event ?? ''))"
      />
      <v-text-field
        :model-value="logToFilter"
        label="结束日期"
        type="date"
        density="compact"
        hide-details
        @update:model-value="$emit('update:logToFilter', String($event ?? ''))"
      />
    </div>
    <div class="adk-monitor-layout">
      <div class="adk-log-list">
        <button
          v-for="log in visibleLogs"
          :key="log.id"
          type="button"
          class="adk-log-item"
          :class="{ 'is-active': log.id === selectedLog?.id }"
          @click="$emit('selectLog', log.id)"
        >
          <span class="adk-workflow-pill" :class="logTone(log.status)">
            {{ statusLabel(log.status) }}
          </span>
          <strong>{{ triggerTypeLabel(log.triggerType) }}</strong>
          <small>
            {{ log.startedAt ? formatDateTime(log.startedAt) : formatDateTime(log.createdAt) }}
            · {{ runDurationLabel(log) }}
          </small>
          <p v-if="log.error">{{ log.error }}</p>
        </button>
        <div v-if="visibleLogs.length === 0" class="adk-workflow-muted">暂无触发日志</div>
      </div>
      <div class="adk-run-detail">
        <div class="adk-run-detail__head">
          <span class="adk-workflow-pill" :class="logTone(selectedLog?.status || '')">
            {{ statusLabel(selectedLog?.status || '') }}
          </span>
          <a
            v-if="selectedLog && (selectedLog.runId || selectedLog.sessionId)"
            class="adk-workflow-link"
            :href="runLink(selectedLog)"
          >
            打开对话运行
          </a>
        </div>
        <div class="adk-run-kv">
          <span>运行编号</span><strong>{{ selectedLog?.runId || selectedLog?.id || '-' }}</strong>
          <span>触发</span><strong>{{ selectedLog ? triggerTypeLabel(selectedLog.triggerType) : '-' }}</strong>
          <span>耗时</span><strong>{{ runDurationLabel(selectedLog) }}</strong>
        </div>
        <h3>节点轨迹</h3>
        <div class="adk-node-trace">
          <button
            v-for="node in selectedLog ? projectedNodeRuns(selectedLog, workflowName) : []"
            :key="node.nodeId"
            type="button"
            class="adk-node-trace__item"
            :class="{ 'is-active': node.nodeId === selectedNodeId }"
            @click="$emit('selectNode', node.nodeId)"
          >
            <span class="adk-workflow-pill" :class="logTone(node.status)">
              {{ statusLabel(node.status) }}
            </span>
            <strong>{{ node.title || node.nodeId }}</strong>
            <small>{{ nodeTypeLabel(node.nodeType) }} · {{ runDurationLabel(node) }}</small>
          </button>
        </div>
        <h3>运行结果</h3>
        <div class="adk-result-panel">
          <p v-if="selectedLog?.result?.markdown">{{ selectedLog.result.markdown }}</p>
          <p v-else-if="selectedLog?.error">{{ selectedLog.error }}</p>
          <p v-else class="adk-workflow-muted">暂无结果输出</p>
          <div class="adk-inspector-actions">
            <v-btn
              size="x-small"
              variant="text"
              :disabled="!selectedLog?.result?.markdown"
              @click="$emit('copyResultMarkdown')"
            >
              复制结果
            </v-btn>
          </div>
          <details v-if="selectedLog?.result?.rawResponse">
            <summary>原始响应</summary>
            <pre>{{ formatJson(selectedLog.result.rawResponse) }}</pre>
          </details>
        </div>
      </div>
    </div>
    <div class="adk-workflow-studio__pager">
      <span>{{ logPageSummary }}</span>
      <v-btn
        size="x-small"
        variant="text"
        :disabled="logPage.offset === 0"
        @click="$emit('previousLogPage')"
      >
        上一页
      </v-btn>
      <v-btn
        size="x-small"
        variant="text"
        :disabled="!logPage.hasMore"
        @click="$emit('nextLogPage')"
      >
        下一页
      </v-btn>
    </div>
  </section>
</template>
