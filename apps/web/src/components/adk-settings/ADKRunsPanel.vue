<script setup lang="ts">
import { ref } from "vue";

import type { ADKAgent, ADKApproval, ADKAuditEvent, ADKOptimizationTask, ADKProvider, ADKRun } from "@/contracts";
import type { PageEnvelope, ADKMetricsResponse } from "../../composables/adkSettingsApi";

import ADKRunTrace from "../shared/ADKRunTrace.vue";

const props = defineProps<{
  metrics: ADKMetricsResponse | null;
  pendingApprovals: ADKApproval[];
  agents: ADKAgent[];
  providers: ADKProvider[];
  runStatusFilter: string;
  runPage: PageEnvelope;
  filteredRuns: ADKRun[];
  approvalStatusFilter: string;
  approvalPage: PageEnvelope;
  approvals: ADKApproval[];
  optimizationTasks: ADKOptimizationTask[];
  auditKindFilter: string;
  auditPage: PageEnvelope;
  auditEvents: ADKAuditEvent[];
  pageSummary: (page: PageEnvelope) => string;
  formatGenericStatusLabel: (status: string) => string;
  formatDateTime: (value: string) => string;
  toolCallStatusColor: (status: string) => string;
  preview: (value: unknown) => string;
  runTerminalMessage: (run: ADKRun) => string;
  cancelRun: (run: ADKRun) => void | Promise<void>;
  cancelOptimizationTask: (task: ADKOptimizationTask) => void | Promise<void>;
  previousRunsPage: () => void | Promise<void>;
  nextRunsPage: () => void | Promise<void>;
  previousApprovalsPage: () => void | Promise<void>;
  nextApprovalsPage: () => void | Promise<void>;
  previousAuditPage: () => void | Promise<void>;
  nextAuditPage: () => void | Promise<void>;
}>();

const emit = defineEmits<{
  "update:runStatusFilter": [value: string];
  "update:approvalStatusFilter": [value: string];
  "update:auditKindFilter": [value: string];
}>();

const expandedRunSummaries = ref<Record<string, boolean>>({});
const expandedToolCallIdsByRun = ref<Record<string, string[]>>({});

function isRunSummaryExpanded(runId: string): boolean {
  return expandedRunSummaries.value[runId] ?? true;
}

function setRunSummaryExpanded(runId: string, expanded: boolean): void {
  expandedRunSummaries.value = { ...expandedRunSummaries.value, [runId]: expanded };
}

function expandedToolCallIdsForRun(runId: string): string[] {
  return expandedToolCallIdsByRun.value[runId] ?? [];
}

function setExpandedToolCallIds(runId: string, ids: string[]): void {
  expandedToolCallIdsByRun.value = { ...expandedToolCallIdsByRun.value, [runId]: ids };
}

function agentLabel(agentId: string): string {
  const agent = props.agents.find((item) => item.id === agentId);
  return agent ? `${agent.name} (${agent.id})` : agentId || "未绑定智能体";
}

function providerLabel(run: ADKRun): string {
  if ((run.providerName ?? "").trim() !== "" || (run.model ?? "").trim() !== "") {
    const providerPart = (run.providerName ?? "").trim() !== ""
      ? `${run.providerName}${run.providerId ? ` (${run.providerId})` : ""}`
      : (run.providerId ?? "").trim();
    const modelPart = (run.model ?? "").trim();
    return [providerPart, modelPart].filter(Boolean).join(" · ");
  }
  const providerId = run.providerId ?? props.agents.find((agent) => agent.id === run.agentId)?.providerId ?? "";
  if (providerId === "") return "未绑定模型服务";
  const provider = props.providers.find((item) => item.id === providerId);
  return provider ? `${provider.displayName} (${provider.id}) · ${provider.model}` : providerId;
}

function runStatusColor(run: ADKRun): string {
  if (run.errorCode === "RUN_ORPHANED") return "error";
  switch (run.status) {
    case "COMPLETED":
      return "success";
    case "RUNNING":
      return "info";
    case "PENDING_APPROVAL":
      return "warning";
    case "FAILED":
    case "TIMED_OUT":
      return "error";
    case "CANCELLED":
      return "grey";
    default:
      return "default";
  }
}

function approvalColor(approval: ADKApproval): string {
  switch (approval.status) {
    case "PENDING":
      return "warning";
    case "APPROVED":
      return "success";
    case "DENIED":
      return "error";
    default:
      return "default";
  }
}

function isRecoverableApproval(approval: ADKApproval): boolean {
  return Boolean(approval.functionCallId && approval.confirmationCallId);
}

function optimizationTaskColor(task: ADKOptimizationTask): string {
  switch (task.status) {
    case "completed":
      return "success";
    case "running":
    case "queued":
      return "info";
    case "failed":
      return "error";
    case "cancelled":
      return "grey";
    default:
      return "default";
  }
}

function formatDuration(ms: number | undefined): string {
  if (ms == null || Number.isNaN(ms)) return "无数据";
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)} s`;
}

function auditKindLabel(kind: string): string {
  if (kind.startsWith("task.")) return `任务 · ${kind}`;
  if (kind.startsWith("memory.")) return `记忆 · ${kind}`;
  if (kind.startsWith("skill.")) return `技能 · ${kind}`;
  if (kind.startsWith("approval.")) return `审批 · ${kind}`;
  if (kind.startsWith("optimization.")) return `优化 · ${kind}`;
  if (kind.startsWith("run.")) return `运行 · ${kind}`;
  return kind;
}
</script>

<template>
  <section class="grid gap-3">
    <div v-if="metrics" class="grid gap-3 md:grid-cols-4">
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">待审批</div>
          <div class="text-2xl font-semibold">{{ metrics.approvals.pending }}</div>
          <div class="mt-1 text-xs text-slate-500">可恢复 {{ metrics.approvals.recoverablePending }} 项</div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">最近异常运行</div>
          <div class="text-2xl font-semibold">{{ metrics.runs.lifecycle.failed + metrics.runs.lifecycle.timedOut }}</div>
          <div class="mt-1 text-xs text-slate-500">
            超时 {{ metrics.runs.lifecycle.timedOut }} · 孤儿运行 {{ metrics.runs.lifecycle.orphaned }}
          </div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">平均工具耗时</div>
          <div class="text-2xl font-semibold">{{ formatDuration(metrics.tools.averageDurationMs) }}</div>
          <div class="mt-1 text-xs text-slate-500">
            成功率 {{ metrics.tools.total ? Math.round((metrics.tools.successful / metrics.tools.total) * 100) : 0 }}%
          </div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">恢复情况</div>
          <div class="text-2xl font-semibold">{{ metrics.runs.lifecycle.resumed }}</div>
          <div class="mt-1 text-xs text-slate-500">
            审批平均等待 {{ formatDuration(metrics.approvals.pendingWaitMs.average) }}
          </div>
        </v-card-text>
      </v-card>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <v-select
        :model-value="runStatusFilter"
        :items="[
          { title: '优先看待处理/异常', value: 'attention' },
          { title: '全部状态', value: '' },
          { title: '运行中', value: 'RUNNING' },
          { title: '等待审批', value: 'PENDING_APPROVAL' },
          { title: '已完成', value: 'COMPLETED' },
          { title: '失败', value: 'FAILED' },
          { title: '已取消', value: 'CANCELLED' },
          { title: '超时', value: 'TIMED_OUT' },
        ]"
        label="运行状态"
        density="compact"
        hide-details
        class="max-w-xs"
        @update:model-value="emit('update:runStatusFilter', String($event ?? ''))"
      />
      <v-chip v-if="pendingApprovals.length > 0" color="warning" variant="tonal">
        {{ pendingApprovals.length }} 项待审批
      </v-chip>
      <div class="ml-auto flex items-center gap-2 text-xs text-slate-500">
        <span>运行 {{ pageSummary(runPage) }}</span>
        <v-btn size="x-small" variant="outlined" :disabled="runPage.offset === 0" @click="previousRunsPage">上一页</v-btn>
        <v-btn size="x-small" variant="outlined" :disabled="!runPage.hasMore" @click="nextRunsPage">下一页</v-btn>
      </div>
    </div>

    <div v-if="filteredRuns.length === 0" class="text-sm text-slate-500">暂无匹配的运行记录。</div>
    <v-card v-for="run in filteredRuns" :key="run.id" flat class="card-shell border-0">
      <v-card-text class="grid gap-3">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="truncate text-sm font-medium text-slate-900">{{ run.id }}</div>
            <div class="text-xs text-slate-500">
              {{ formatGenericStatusLabel(run.status) }} · 更新 {{ formatDateTime(run.updatedAt) }}
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <v-chip size="small" :color="runStatusColor(run)" variant="tonal">{{ run.status }}</v-chip>
            <v-chip size="small" :color="toolCallStatusColor(run.status)" variant="tonal">
              {{ run.toolCalls.length }} 次调用
            </v-chip>
            <v-btn
              v-if="run.status === 'RUNNING' || run.status === 'PENDING_APPROVAL'"
              size="small"
              variant="outlined"
              color="error"
              @click="cancelRun(run)"
            >
              取消
            </v-btn>
          </div>
        </div>

        <div class="grid gap-2 rounded border border-slate-100 bg-slate-50 p-3 text-xs text-slate-600 md:grid-cols-2">
          <div><span class="text-slate-400">智能体</span> · {{ agentLabel(run.agentId) }}</div>
          <div><span class="text-slate-400">模型服务</span> · {{ providerLabel(run) }}</div>
          <div><span class="text-slate-400">创建</span> · {{ formatDateTime(run.createdAt) }}</div>
          <div><span class="text-slate-400">耗时</span> · {{ formatDuration(run.usage?.durationMs) }}</div>
          <div v-if="run.errorCode"><span class="text-slate-400">错误码</span> · {{ run.errorCode }}</div>
          <div v-if="run.failureReason"><span class="text-slate-400">失败原因</span> · {{ run.failureReason }}</div>
          <div v-if="run.optimizationTaskId"><span class="text-slate-400">优化任务</span> · {{ run.optimizationTaskId }}</div>
          <div v-if="run.resumeState"><span class="text-slate-400">恢复状态</span> · {{ run.resumeState }}</div>
        </div>

        <v-alert v-if="run.degraded" type="warning" variant="tonal" density="compact">
          已使用降级回复 · {{ run.errorCode ?? run.failureReason }}
        </v-alert>
        <v-alert v-if="run.errorCode === 'RUN_ORPHANED'" type="error" variant="tonal" density="compact">
          该运行已被标记为孤儿运行，需要检查模型服务响应或流式中断。
        </v-alert>
        <div v-if="runTerminalMessage(run)" class="text-xs text-slate-600">{{ runTerminalMessage(run) }}</div>

        <ADKRunTrace
          v-if="run.toolCalls.length > 0"
          :run="run"
          :busy="false"
          :summary-expanded="isRunSummaryExpanded(run.id)"
          :expanded-tool-call-ids="expandedToolCallIdsForRun(run.id)"
          @update:summary-expanded="setRunSummaryExpanded(run.id, $event)"
          @update:expanded-tool-call-ids="setExpandedToolCallIds(run.id, $event)"
        />
        <div v-else class="text-xs text-slate-500">本次运行没有工具调用。</div>
      </v-card-text>
    </v-card>

    <v-card flat class="card-shell border-0">
      <v-card-title class="flex flex-wrap items-center justify-between gap-3">
        <span>审批动作</span>
        <div class="flex items-center gap-2">
          <v-select
            :model-value="approvalStatusFilter"
            :items="[
              { title: '仅待审批', value: 'PENDING' },
              { title: '全部状态', value: '' },
              { title: '已批准', value: 'APPROVED' },
              { title: '已拒绝', value: 'DENIED' },
            ]"
            density="compact"
            hide-details
            class="min-w-[10rem]"
            @update:model-value="emit('update:approvalStatusFilter', String($event ?? ''))"
          />
          <span class="text-xs text-slate-500">审批 {{ pageSummary(approvalPage) }}</span>
          <v-btn size="x-small" variant="outlined" :disabled="approvalPage.offset === 0" @click="previousApprovalsPage">上一页</v-btn>
          <v-btn size="x-small" variant="outlined" :disabled="!approvalPage.hasMore" @click="nextApprovalsPage">下一页</v-btn>
        </div>
      </v-card-title>
      <v-card-text class="grid gap-3">
        <div v-if="approvals.length === 0" class="text-sm text-slate-500">当前筛选下暂无审批记录。</div>
        <div
          v-for="approval in approvals"
          :key="approval.id"
          class="rounded border p-3"
          :class="approval.status === 'PENDING' ? 'border-amber-200 bg-amber-50/40' : 'border-slate-200'"
        >
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div>
              <div class="font-medium">{{ approval.toolName }}</div>
              <div class="text-xs text-slate-500">{{ approval.reason }}</div>
            </div>
            <v-chip size="small" :color="approvalColor(approval)" variant="tonal">{{ approval.status }}</v-chip>
          </div>
          <div class="mt-2 grid gap-1 text-xs text-slate-500 md:grid-cols-2">
            <div>运行：{{ approval.runId }}</div>
            <div>智能体：{{ agentLabel(approval.agentId) }}</div>
            <div>更新：{{ formatDateTime(approval.updatedAt) }}</div>
            <div v-if="isRecoverableApproval(approval)">可恢复审批链路已记录</div>
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-card v-if="optimizationTasks.length > 0" flat class="card-shell border-0">
      <v-card-title>优化任务</v-card-title>
      <v-card-text class="grid gap-3">
        <div v-for="task in optimizationTasks" :key="task.id" class="rounded border border-slate-200 p-3">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <div class="font-medium">{{ task.id }}</div>
              <div class="text-xs text-slate-500">
                {{ task.status }} · {{ task.progress.completed }}/{{ task.progress.total }} 完成
              </div>
            </div>
            <div class="flex items-center gap-2">
              <v-chip size="small" :color="optimizationTaskColor(task)" variant="tonal">{{ task.status }}</v-chip>
              <v-btn
                v-if="task.status === 'queued' || task.status === 'running'"
                size="small"
                variant="outlined"
                color="error"
                @click="cancelOptimizationTask(task)"
              >
                取消
              </v-btn>
            </div>
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-card flat class="card-shell border-0">
      <v-card-title class="flex flex-wrap items-center justify-between gap-3">
        <span>审计流</span>
        <div class="flex items-center gap-2">
          <v-text-field
            :model-value="auditKindFilter"
            label="按事件类型过滤"
            density="compact"
            hide-details
            class="min-w-[12rem]"
            @update:model-value="emit('update:auditKindFilter', String($event ?? ''))"
          />
          <span class="text-xs text-slate-500">审计 {{ pageSummary(auditPage) }}</span>
          <v-btn size="x-small" variant="outlined" :disabled="auditPage.offset === 0" @click="previousAuditPage">上一页</v-btn>
          <v-btn size="x-small" variant="outlined" :disabled="!auditPage.hasMore" @click="nextAuditPage">下一页</v-btn>
        </div>
      </v-card-title>
      <v-card-text class="grid gap-2">
        <div v-if="auditEvents.length === 0" class="text-sm text-slate-500">当前筛选下暂无审计事件。</div>
        <div v-for="event in auditEvents" :key="event.id" class="border-b border-slate-100 pb-2">
          <div class="text-sm font-medium">{{ auditKindLabel(event.kind) }}</div>
          <div class="text-xs text-slate-500">
            {{ event.detail }} · {{ formatDateTime(event.createdAt) }}
            <span v-if="event.subjectId"> · 对象：{{ event.subjectId }}</span>
          </div>
          <pre v-if="event.metadata" class="adk-json mt-2">{{ preview(event.metadata) }}</pre>
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>
