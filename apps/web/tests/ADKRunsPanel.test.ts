// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import type {
  ADKAgent,
  ADKApproval,
  ADKAuditEvent,
  ADKOptimizationTask,
  ADKProvider,
  ADKRun,
} from "@/contracts";

import type {
  ADKMetricsResponse,
  PageEnvelope,
} from "../src/composables/adkSettingsApi";
import ADKRunsPanel from "../src/components/adk-settings/ADKRunsPanel.vue";
import { buttonStub, inputStub, passthroughStub, selectStub } from "./helpers";

const runTraceStub = defineComponent({
  props: ["summaryExpanded", "expandedToolCallIds"],
  emits: ["update:summary-expanded", "update:expanded-tool-call-ids"],
  template: `
    <div class="adk-run-trace-stub">
      <div class="summary-state">{{ summaryExpanded ? "expanded" : "collapsed" }}</div>
      <div class="expanded-call-ids">{{ (expandedToolCallIds ?? []).join("|") }}</div>
      <button type="button" class="collapse-trace" @click="$emit('update:summary-expanded', false)">collapse</button>
      <button type="button" class="expand-call-ids" @click="$emit('update:expanded-tool-call-ids', ['call-2'])">ids</button>
    </div>
  `,
});

describe("ADKRunsPanel", () => {
  it("renders live run operations, audit labels, and provider fallbacks from real run data", async () => {
    const cancelRun = vi.fn();
    const resumeRun = vi.fn();
    const cancelOptimizationTask = vi.fn();
    const previousRunsPage = vi.fn();
    const nextRunsPage = vi.fn();
    const previousApprovalsPage = vi.fn();
    const nextApprovalsPage = vi.fn();
    const previousAuditPage = vi.fn();
    const nextAuditPage = vi.fn();

    const wrapper = mount(ADKRunsPanel, {
      props: {
        metrics: buildMetrics(),
        pendingApprovals: [buildApproval({ id: "approval-pending" })],
        agents: [
          buildAgent(),
          buildAgent({ id: "agent-missing-provider", providerId: "" }),
        ],
        providers: [buildProvider()],
        runStatusFilter: "attention",
        runPage: buildPage({ offset: 20, hasMore: true }),
        filteredRuns: [
          buildRun({
            id: "run-running",
            status: "RUNNING",
            providerId: "",
            providerName: "",
            model: "",
            usage: { durationMs: 950 },
            toolCalls: [{ id: "call-1", status: "COMPLETED" }],
          }),
          buildRun({
            id: "run-timed-out",
            status: "TIMED_OUT",
            workMode: "loop",
            workflowStatus: "RUNNING",
            usage: { durationMs: 2_500 },
            toolCalls: [{ id: "call-2", status: "FAILED", error: "broker rejected" }],
          }),
          buildRun({
            id: "run-orphaned",
            status: "FAILED",
            errorCode: "RUN_ORPHANED",
            degraded: true,
            providerId: "",
            providerName: "",
            model: "",
            agentId: "agent-missing-provider",
            failureReason: "",
            message: "",
            toolCalls: [],
          }),
          buildRun({
            id: "run-cancelled",
            status: "CANCELLED",
            failureReason: "",
            message: "",
            toolCalls: [{ id: "call-3", status: "CANCELLED" }],
          }),
          buildRun({
            id: "run-completed",
            status: "COMPLETED",
            providerId: "provider-b",
            providerName: "Anthropic",
            model: "claude-sonnet",
            usage: { durationMs: 12_000 },
            toolCalls: [{ id: "call-4", status: "COMPLETED" }],
          }),
        ],
        approvalStatusFilter: "PENDING",
        approvalPage: buildPage({ limit: 10, offset: 10, hasMore: true }),
        approvals: [
          buildApproval({
            id: "approval-pending",
            status: "PENDING",
            functionCallId: "fc-1",
            confirmationCallId: "cc-1",
          }),
          buildApproval({ id: "approval-approved", status: "APPROVED" }),
          buildApproval({ id: "approval-denied", status: "DENIED" }),
          buildApproval({ id: "approval-other", status: "UNKNOWN" }),
        ],
        optimizationTasks: [
          buildOptimizationTask({ id: "opt-completed", status: "completed" }),
          buildOptimizationTask({ id: "opt-queued", status: "queued" }),
          buildOptimizationTask({ id: "opt-running", status: "running" }),
          buildOptimizationTask({ id: "opt-failed", status: "failed" }),
          buildOptimizationTask({ id: "opt-cancelled", status: "cancelled" }),
          buildOptimizationTask({ id: "opt-other", status: "stuck" }),
        ],
        auditKindFilter: "",
        auditPage: buildPage({ limit: 12, offset: 12, hasMore: true }),
        auditEvents: [
          buildAuditEvent({ id: "audit-task", kind: "task.created" }),
          buildAuditEvent({ id: "audit-memory", kind: "memory.saved" }),
          buildAuditEvent({ id: "audit-skill", kind: "skill.loaded" }),
          buildAuditEvent({ id: "audit-approval", kind: "approval.resolved" }),
          buildAuditEvent({ id: "audit-opt", kind: "optimization.completed" }),
          buildAuditEvent({ id: "audit-run", kind: "run.updated" }),
          buildAuditEvent({ id: "audit-custom", kind: "custom.kind" }),
        ],
        pageSummary: (page: PageEnvelope) => `offset=${page.offset}`,
        formatGenericStatusLabel: (status: string) => status,
        formatDateTime: (value: string) => `fmt:${value}`,
        toolCallStatusColor: (status: string) =>
          status === "FAILED" || status === "TIMED_OUT"
            ? "error"
            : status === "RUNNING"
              ? "info"
              : status === "PENDING_APPROVAL"
                ? "warning"
                : "default",
        preview: (value: unknown) => JSON.stringify(value),
        runTerminalMessage: (run: ADKRun) =>
          run.status === "FAILED"
            ? "run failed"
            : run.status === "TIMED_OUT"
              ? "run timed out"
              : run.status === "CANCELLED"
                ? "run cancelled"
                : "",
        cancelRun,
        resumeRun,
        cancelOptimizationTask,
        previousRunsPage,
        nextRunsPage,
        previousApprovalsPage,
        nextApprovalsPage,
        previousAuditPage,
        nextAuditPage,
      },
      global: {
        stubs: {
          ADKRunTrace: runTraceStub,
          "v-alert": { template: "<div class='v-alert-stub'><slot /></div>" },
          "v-btn": buttonStub,
          "v-card": passthroughStub,
          "v-card-text": passthroughStub,
          "v-card-title": passthroughStub,
          "v-chip": {
            props: ["color"],
            template: "<span class='v-chip-stub' :data-color='color'><slot /></span>",
          },
          "v-select": selectStub,
          "v-text-field": inputStub,
        },
      },
    });

    expect(wrapper.text()).toContain("待审批");
    expect(wrapper.text()).toContain("2.5 s");
    expect(wrapper.text()).toContain("12 s");
    expect(wrapper.text()).toContain("950 ms");
    expect(wrapper.text()).toContain("OpenAI (provider-a) · gpt-4.1");
    expect(wrapper.text()).toContain("Anthropic (provider-b) · claude-sonnet");
    expect(wrapper.text()).toContain("未绑定模型服务");
    expect(wrapper.text()).toContain("可恢复审批链路已记录");
    expect(wrapper.text()).toContain("已使用降级回复");
    expect(wrapper.text()).toContain("该运行已被标记为孤儿运行");
    expect(wrapper.text()).toContain("本次运行没有工具调用。");
    expect(wrapper.text()).toContain("任务 · task.created");
    expect(wrapper.text()).toContain("记忆 · memory.saved");
    expect(wrapper.text()).toContain("技能 · skill.loaded");
    expect(wrapper.text()).toContain("审批 · approval.resolved");
    expect(wrapper.text()).toContain("优化 · optimization.completed");
    expect(wrapper.text()).toContain("运行 · run.updated");
    expect(wrapper.text()).toContain("custom.kind");

    expect(wrapper.findAll(".summary-state")[0]?.text()).toBe("expanded");
    await wrapper.find(".collapse-trace").trigger("click");
    expect(wrapper.findAll(".summary-state")[0]?.text()).toBe("collapsed");
    await wrapper.find(".expand-call-ids").trigger("click");
    expect(wrapper.findAll(".expanded-call-ids")[1]?.text()).toBe("call-2");

    const buttons = wrapper.findAll("button");
    const runningRunCard = wrapper
      .findAll("div")
      .find((node) => node.text().includes("run-running"));
    const queuedOptimizationCard = wrapper
      .findAll("div")
      .find((node) => node.text().includes("opt-queued"));
    await runningRunCard!.find("button").trigger("click");
    await queuedOptimizationCard!.find("button").trigger("click");
    await buttons.find((button) => button.text() === "继续")!.trigger("click");
    const previousButtons = buttons.filter((button) => button.text() === "上一页");
    const nextButtons = buttons.filter((button) => button.text() === "下一页");
    await previousButtons[0]!.trigger("click");
    await nextButtons[0]!.trigger("click");
    const approvalsCard = wrapper
      .findAll("div")
      .find((node) => node.text().includes("审批动作"));
    const approvalsButtons = approvalsCard!
      .findAll("button")
      .filter((button) => button.text() === "上一页" || button.text() === "下一页");
    await approvalsButtons[0]!.trigger("click");
    await approvalsButtons[1]!.trigger("click");
    const auditCard = wrapper
      .findAll("div")
      .find((node) => node.text().includes("审计流"));
    const auditButtons = auditCard!
      .findAll("button")
      .filter((button) => button.text() === "上一页" || button.text() === "下一页");
    await auditButtons[0]!.trigger("click");
    await auditButtons[1]!.trigger("click");

    expect(cancelRun).toHaveBeenCalledWith(
      expect.objectContaining({ id: "run-running" }),
    );
    expect(resumeRun).toHaveBeenCalledWith(
      expect.objectContaining({ id: "run-timed-out" }),
    );
    expect(cancelOptimizationTask).toHaveBeenCalledWith(
      expect.objectContaining({ id: "opt-queued" }),
    );
    expect(previousRunsPage).toHaveBeenCalled();
    expect(nextRunsPage).toHaveBeenCalled();
    expect(previousApprovalsPage).toHaveBeenCalled();
    expect(nextApprovalsPage).toHaveBeenCalled();
    expect(previousAuditPage).toHaveBeenCalled();
    expect(nextAuditPage).toHaveBeenCalled();

    const selects = wrapper.findAll("select");
    await selects[0]!.setValue("RUNNING");
    await selects[1]!.setValue("APPROVED");
    expect(wrapper.emitted("update:runStatusFilter")?.[0]).toEqual(["RUNNING"]);
    expect(wrapper.emitted("update:approvalStatusFilter")?.[0]).toEqual([
      "APPROVED",
    ]);

    const auditInput = wrapper.find("input");
    await auditInput.setValue("memory.");
    expect(wrapper.emitted("update:auditKindFilter")?.[0]).toEqual([
      "memory.",
    ]);
  });

  it("shows empty observation states when no runs, approvals, optimization tasks, or audits match", () => {
    const wrapper = mount(ADKRunsPanel, {
      props: {
        metrics: null,
        pendingApprovals: [],
        agents: [],
        providers: [],
        runStatusFilter: "",
        runPage: buildPage(),
        filteredRuns: [],
        approvalStatusFilter: "",
        approvalPage: buildPage({ limit: 10 }),
        approvals: [],
        optimizationTasks: [],
        auditKindFilter: "",
        auditPage: buildPage({ limit: 12 }),
        auditEvents: [],
        pageSummary: () => "0/0",
        formatGenericStatusLabel: (status: string) => status,
        formatDateTime: (value: string) => value,
        toolCallStatusColor: () => "default",
        preview: (value: unknown) => JSON.stringify(value),
        runTerminalMessage: () => "",
        cancelRun: vi.fn(),
        resumeRun: vi.fn(),
        cancelOptimizationTask: vi.fn(),
        previousRunsPage: vi.fn(),
        nextRunsPage: vi.fn(),
        previousApprovalsPage: vi.fn(),
        nextApprovalsPage: vi.fn(),
        previousAuditPage: vi.fn(),
        nextAuditPage: vi.fn(),
      },
      global: {
        stubs: {
          ADKRunTrace: runTraceStub,
          "v-btn": buttonStub,
          "v-card": passthroughStub,
          "v-card-text": passthroughStub,
          "v-card-title": passthroughStub,
          "v-chip": passthroughStub,
          "v-select": selectStub,
          "v-text-field": inputStub,
        },
      },
    });

    expect(wrapper.text()).toContain("暂无匹配的运行记录。");
    expect(wrapper.text()).toContain("当前筛选下暂无审批记录。");
    expect(wrapper.text()).toContain("当前筛选下暂无审计事件。");
    expect(wrapper.text()).not.toContain("优化任务");
  });
});

function buildPage(overrides: Partial<PageEnvelope> = {}): PageEnvelope {
  return {
    limit: 20,
    offset: 0,
    total: 20,
    returned: 20,
    hasMore: false,
    ...overrides,
  };
}

function buildMetrics(
  overrides: Partial<ADKMetricsResponse> = {},
): ADKMetricsResponse {
  return {
    runs: {
      total: 4,
      byStatus: {},
      byAgent: {},
      byProvider: {},
      lifecycle: {
        failed: 1,
        timedOut: 1,
        cancelled: 1,
        resumed: 1,
        orphaned: 1,
      },
    },
    tools: {
      total: 5,
      successful: 4,
      averageDurationMs: 900,
      byName: {},
      byStatus: {},
    },
    approvals: {
      pending: 2,
      total: 3,
      approved: 1,
      denied: 0,
      recoverablePending: 1,
      pendingWaitMs: { average: 1_200, max: 2_500 },
      resolutionWaitMs: { average: 1_000, max: 2_000, count: 2 },
    },
    usage: {
      samples: 2,
      tokensInTotal: 100,
      tokensOutTotal: 150,
      tokensInAverage: 50,
      tokensOutAverage: 75,
    },
    ...overrides,
  };
}

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
  return {
    id: "provider-a",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4.1",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildAgent(overrides: Partial<ADKAgent> = {}): ADKAgent {
  return {
    id: "agent-a",
    name: "交易助手",
    instruction: "",
    providerId: "provider-a",
    model: "gpt-4.1",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-a",
    sessionId: "session-a",
    agentId: "agent-a",
    providerId: "provider-a",
    providerName: "",
    model: "",
    status: "RUNNING",
    workMode: "chat",
    workflowStatus: "",
    userMessage: "检查账户",
    message: "",
    failureReason: "failed",
    errorCode: "",
    degraded: false,
    toolCalls: [],
    usage: { durationMs: undefined },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:01:00Z",
    ...overrides,
  } as ADKRun;
}

function buildApproval(overrides: Partial<ADKApproval> = {}): ADKApproval {
  return {
    id: "approval-a",
    runId: "run-a",
    agentId: "agent-a",
    toolName: "trading.submit_order",
    reason: "需要确认",
    status: "PENDING",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  } as ADKApproval;
}

function buildOptimizationTask(
  overrides: Partial<ADKOptimizationTask> = {},
): ADKOptimizationTask {
  return {
    id: "opt-a",
    status: "running",
    progress: { completed: 2, total: 8 },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  } as ADKOptimizationTask;
}

function buildAuditEvent(
  overrides: Partial<ADKAuditEvent> = {},
): ADKAuditEvent {
  return {
    id: "audit-a",
    kind: "run.updated",
    detail: "事件详情",
    createdAt: "2026-07-01T00:00:00Z",
    subjectId: "run-a",
    metadata: { status: "RUNNING" },
    ...overrides,
  } as ADKAuditEvent;
}
