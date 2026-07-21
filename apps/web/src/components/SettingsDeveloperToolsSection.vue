<script setup lang="ts">
import type {
  ObservabilityEvent,
  RequestObservabilitySummary,
} from "../contracts";
import { formatDateTime } from "../composables/consoleDataFormatting";

defineProps<{
  observability: RequestObservabilitySummary;
}>();

function correlationLabels(event: ObservabilityEvent): string[] {
  return [
    event.requestId ? `request ${event.requestId}` : "",
    event.sessionId ? `session ${event.sessionId}` : "",
    event.runId ? `run ${event.runId}` : "",
    event.taskId ? `task ${event.taskId}` : "",
    event.instrumentId ? `instrument ${event.instrumentId}` : "",
    event.providerId ? `provider ${event.providerId}` : "",
  ].filter(Boolean);
}

function observabilityEventTarget(event: ObservabilityEvent): string | null {
  if (
    event.source === "adk" ||
    event.sessionId ||
    event.runId?.startsWith("run-")
  ) {
    return "/adk/agents";
  }
  if (
    event.source === "backtest" ||
    event.runId?.startsWith("bt-") ||
    event.taskId?.startsWith("sync-")
  ) {
    return "/backtest";
  }
  return null;
}

function observabilityEventTargetLabel(event: ObservabilityEvent): string {
  return observabilityEventTarget(event) === "/adk/agents"
    ? "ADK 运行"
    : "回测任务";
}

function formatObservabilityImportance(
  importance: ObservabilityEvent["importance"],
): string {
  switch (importance) {
    case "low":
      return "低重要性";
    case "normal":
      return "普通重要性";
    case "high":
      return "高重要性";
    case "critical":
      return "关键重要性";
  }
}
</script>

<template>
  <section data-testid="request-observability-summary">
    <v-card flat class="card-shell border-0">
      <div
        class="flex flex-col items-start gap-3 px-4 pt-4 sm:flex-row sm:items-center sm:justify-between"
      >
        <div>
          <div class="text-xl font-semibold text-slate-900">链路观测</div>
          <div class="mt-1 text-sm text-slate-500">
            查看近期请求错误、慢请求和 OpenD 调用情况。
          </div>
        </div>
        <div class="flex flex-wrap justify-start gap-2 sm:justify-end">
          <v-chip variant="outlined" size="small">
            记录阈值
            {{ formatObservabilityImportance(observability.minimumImportance) }}
          </v-chip>
          <v-chip variant="outlined" size="small">
            慢请求阈值 {{ observability.slowThresholdMs }}ms
          </v-chip>
        </div>
      </div>

      <v-card-text>
        <div class="grid gap-3 sm:grid-cols-3">
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
            <div class="text-xs uppercase text-slate-500">最近错误</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ observability.recentErrors.length }}
            </div>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
            <div class="text-xs uppercase text-slate-500">最近慢请求</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ observability.recentSlowRequests.length }}
            </div>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
            <div class="text-xs uppercase text-slate-500">OpenD 调用</div>
            <div class="mt-2 text-2xl font-semibold text-slate-900">
              {{ observability.openD.totalCalls - observability.openD.failedCalls }}
              / {{ observability.openD.totalCalls }}
            </div>
            <div
              v-if="observability.openD.lastOperation"
              class="mt-1 text-xs text-slate-500"
            >
              {{ observability.openD.lastOperation }}
            </div>
          </div>
        </div>

        <div v-if="observability.recentErrors.length" class="mt-4 grid gap-2">
          <div
            v-for="event in observability.recentErrors.slice(0, 5)"
            :key="`${event.at}-${event.requestId ?? event.runId ?? event.taskId ?? event.message}`"
            class="rounded-lg border border-red-100 bg-red-50 px-3 py-3"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div>
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-medium text-red-800">
                    {{ event.message }}
                  </span>
                  <v-chip variant="outlined" size="x-small">
                    {{ formatObservabilityImportance(event.importance) }}
                  </v-chip>
                </div>
                <div v-if="event.error" class="mt-1 text-xs text-red-700">
                  {{ event.error }}
                </div>
              </div>
              <v-btn
                v-if="observabilityEventTarget(event)"
                :to="observabilityEventTarget(event) ?? undefined"
                variant="text"
                size="small"
              >
                {{ observabilityEventTargetLabel(event) }}
              </v-btn>
            </div>
            <div
              v-if="correlationLabels(event).length"
              class="mt-2 flex flex-wrap gap-1"
            >
              <v-chip
                v-for="label in correlationLabels(event)"
                :key="label"
                variant="outlined"
                size="x-small"
              >
                {{ label }}
              </v-chip>
            </div>
            <div class="mt-2 text-xs text-slate-500">
              {{ formatDateTime(event.at) }}
            </div>
          </div>
        </div>
        <div v-else class="mt-4 text-sm text-slate-500">
          当前没有近期链路错误。
        </div>

        <div
          v-if="observability.recentSlowRequests.length"
          class="mt-4 border-t border-slate-200 pt-4"
        >
          <div class="text-xs uppercase text-slate-500">慢请求</div>
          <div class="mt-2 grid gap-2">
            <div
              v-for="event in observability.recentSlowRequests.slice(0, 5)"
              :key="`${event.at}-${event.requestId ?? event.path}`"
              class="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-slate-50 px-3 py-2 text-sm"
            >
              <span class="font-medium text-slate-800">
                {{ event.method }} {{ event.path }}
              </span>
              <span class="text-slate-600">
                {{ formatObservabilityImportance(event.importance) }} ·
                {{ event.latencyMs }}ms · {{ event.requestId ?? "无 request id" }}
              </span>
            </div>
          </div>
        </div>
      </v-card-text>
    </v-card>
  </section>
</template>
