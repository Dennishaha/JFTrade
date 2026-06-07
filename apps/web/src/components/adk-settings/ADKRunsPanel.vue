<script setup lang="ts">
import type { ADKApproval, ADKAuditEvent, ADKOptimizationTask, ADKRun } from "@jftrade/ui-contracts";
import type { PageEnvelope, ADKMetricsResponse } from "../../composables/adkSettingsApi";

defineProps<{
  metrics: ADKMetricsResponse | null;
  pendingApprovals: ADKApproval[];
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
</script>

<template>
  <section class="grid gap-3">
    <div v-if="metrics" class="grid gap-3 md:grid-cols-4">
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">待审批</div>
          <div class="text-2xl font-semibold">{{ metrics.approvals.pending }}</div>
          <div class="mt-1 text-xs text-slate-500">
            可恢复 {{ metrics.approvals.recoverablePending }} 项
          </div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">最近失败 Runs</div>
          <div class="text-2xl font-semibold">{{ metrics.runs.lifecycle.failed + metrics.runs.lifecycle.timedOut }}</div>
          <div class="mt-1 text-xs text-slate-500">
            超时 {{ metrics.runs.lifecycle.timedOut }} · orphaned {{ metrics.runs.lifecycle.orphaned }}
          </div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">平均工具耗时</div>
          <div class="text-2xl font-semibold">
            {{ metrics.tools.averageDurationMs }} ms
          </div>
          <div class="mt-1 text-xs text-slate-500">
            成功率 {{ metrics.tools.total ? Math.round(metrics.tools.successful / metrics.tools.total * 100) : 0 }}%
          </div>
        </v-card-text>
      </v-card>
      <v-card flat class="card-shell border-0">
        <v-card-text>
          <div class="text-xs text-slate-500">恢复情况</div>
          <div class="text-2xl font-semibold">{{ metrics.runs.lifecycle.resumed }}</div>
          <div class="mt-1 text-xs text-slate-500">
            审批平均等待 {{ metrics.approvals.pendingWaitMs.average }} ms
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
        @update:model-value="$emit('update:runStatusFilter', $event)"
      />
      <v-chip v-if="pendingApprovals.length > 0" color="warning" variant="tonal">
        {{ pendingApprovals.length }} 项待审批
      </v-chip>
      <div class="ml-auto flex items-center gap-2 text-xs text-slate-500">
        <span>Runs {{ pageSummary(runPage) }}</span>
        <v-btn size="x-small" variant="outlined" :disabled="runPage.offset === 0" @click="previousRunsPage">
          上一页
        </v-btn>
        <v-btn size="x-small" variant="outlined" :disabled="!runPage.hasMore" @click="nextRunsPage">
          下一页
        </v-btn>
      </div>
    </div>
    <div v-if="filteredRuns.length === 0" class="text-sm text-slate-500">暂无匹配的运行记录。</div>
    <v-card v-for="run in filteredRuns" :key="run.id" flat class="card-shell border-0">
      <v-card-text>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="truncate text-sm font-medium text-slate-900">{{ run.id }}</div>
            <div class="text-xs text-slate-500">
              {{ formatGenericStatusLabel(run.status) }} · {{ formatDateTime(run.createdAt) }}
            </div>
          </div>
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
        <div v-if="run.degraded" class="mt-2 text-xs text-amber-700">
          已使用降级回复 · {{ run.errorCode ?? run.failureReason }}
        </div>
        <div v-if="runTerminalMessage(run)" class="mt-2 text-xs text-slate-600">
          {{ runTerminalMessage(run) }}
        </div>
        <div v-if="run.optimizationTaskId" class="mt-2 text-xs text-slate-500">
          优化任务：{{ run.optimizationTaskId }}
        </div>
        <pre v-if="run.toolCalls.length > 0" class="adk-json mt-3">{{ preview(run.toolCalls) }}</pre>
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
            @update:model-value="$emit('update:approvalStatusFilter', $event)"
          />
          <span class="text-xs text-slate-500">Approvals {{ pageSummary(approvalPage) }}</span>
          <v-btn size="x-small" variant="outlined" :disabled="approvalPage.offset === 0" @click="previousApprovalsPage">
            上一页
          </v-btn>
          <v-btn size="x-small" variant="outlined" :disabled="!approvalPage.hasMore" @click="nextApprovalsPage">
            下一页
          </v-btn>
        </div>
      </v-card-title>
      <v-card-text class="grid gap-3">
        <div v-if="approvals.length === 0" class="text-sm text-slate-500">
          当前筛选下暂无审批记录。
        </div>
        <div
          v-for="approval in approvals"
          :key="approval.id"
          class="rounded border p-3"
          :class="approval.status === 'PENDING' ? 'border-amber-200' : 'border-slate-200'"
        >
          <div class="font-medium">{{ approval.toolName }}</div>
          <div class="text-xs text-slate-500">{{ approval.reason }}</div>
          <div class="mt-1 text-xs text-slate-500">
            {{ approval.status }} · Run: {{ approval.runId }} · {{ formatDateTime(approval.updatedAt) }}
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
            @update:model-value="$emit('update:auditKindFilter', $event)"
          />
          <span class="text-xs text-slate-500">Audit {{ pageSummary(auditPage) }}</span>
          <v-btn size="x-small" variant="outlined" :disabled="auditPage.offset === 0" @click="previousAuditPage">
            上一页
          </v-btn>
          <v-btn size="x-small" variant="outlined" :disabled="!auditPage.hasMore" @click="nextAuditPage">
            下一页
          </v-btn>
        </div>
      </v-card-title>
      <v-card-text class="grid gap-2">
        <div v-if="auditEvents.length === 0" class="text-sm text-slate-500">
          当前筛选下暂无审计事件。
        </div>
        <div v-for="event in auditEvents" :key="event.id" class="border-b border-slate-100 pb-2">
          <div class="text-sm font-medium">{{ event.kind }}</div>
          <div class="text-xs text-slate-500">{{ event.detail }} · {{ formatDateTime(event.createdAt) }}</div>
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>
