import { computed, ref, type ComputedRef, type Ref } from "vue";

import type {
  ADKApproval,
  ADKRun,
  ADKSessionContextSnapshot,
  ADKWorkflowStepState,
} from "@/contracts";

import { uniqueADKApprovalsById } from "./adkApprovalResolution";
import { runStatusTone } from "./adkChatPresentation";
import { normalizeADKRun } from "./adkNormalization";
import {
  approvalsForGroup,
  sortTimelineEntries,
  type ADKTimelineEntryState,
} from "./adkTimeline";
import { fetchEnvelope } from "./apiClient";

export interface ADKChildRunQueueItem {
  id: string;
  index: number;
  stepIndex?: number | undefined;
  stepTitle?: string | undefined;
  stepMessage?: string | undefined;
  run?: ADKRun | undefined;
  status: string;
  updatedAt?: string | undefined;
  pendingApprovalCount: number;
}

export interface ADKApprovalQueueItem {
  approval: ADKApproval;
  runId: string;
  childRunId?: string | undefined;
  childIndex?: number | undefined;
  stepTitle?: string | undefined;
}

export interface ADKWorkflowQueueState {
  activeChildRunId: Ref<string>;
  childRunItems: ComputedRef<ADKChildRunQueueItem[]>;
  childRunSnapshots: Ref<Record<string, ADKRun>>;
  childTimelineEntries: ComputedRef<ADKTimelineEntryState[]>;
  childViewContext: ComputedRef<{
    title: string;
    runId: string;
    message: string;
  } | null>;
  clearWorkflowQueues: () => void;
  parentApprovalQueue: ComputedRef<ADKApprovalQueueItem[]>;
  parentChildRunItems: ComputedRef<ADKChildRunQueueItem[]>;
  parentTimelineEntries: ComputedRef<ADKTimelineEntryState[]>;
  parentWorkflowPlanRun: ComputedRef<ADKRun | null>;
  selectedApprovalQueue: ComputedRef<ADKApprovalQueueItem[]>;
  setActiveChildRunId: (runId: string) => void;
  syncWorkflowRun: (run: ADKRun | undefined) => Promise<void>;
  visibleTimelineEntries: ComputedRef<ADKTimelineEntryState[]>;
  visibleWorkflowPlanRun: ComputedRef<ADKRun | null>;
}

export function useADKWorkflowQueueState(options: {
  timelineEntries: Ref<ADKTimelineEntryState[]>;
  selectedSessionId: Ref<string>;
}): ADKWorkflowQueueState {
  const workflowPlanRun = ref<ADKRun | null>(null);
  const childRunSnapshots = ref<Record<string, ADKRun>>({});
  const activeChildRunId = ref("");

  const parentWorkflowPlanRun = computed(() => {
    const run = workflowPlanRun.value;
    if (!isDisplayableWorkflowPlanRun(run)) return null;
    const selectedSessionId = options.selectedSessionId.value.trim();
    if (
      selectedSessionId !== "" &&
      run?.sessionId &&
      run.sessionId !== selectedSessionId
    ) {
      return null;
    }
    return run;
  });

  const activeChildRun = computed(() => {
    const id = activeChildRunId.value.trim();
    return id === "" ? null : childRunSnapshots.value[id] ?? null;
  });

  const conversationRun = computed(() => activeChildRun.value ?? parentWorkflowPlanRun.value);

  const visibleWorkflowPlanRun = computed(() => {
    const run = conversationRun.value;
    return isDisplayableWorkflowPlanRun(run) ? run : null;
  });

  const parentChildRunItems = computed<ADKChildRunQueueItem[]>(() =>
    buildChildRunItems(parentWorkflowPlanRun.value, childRunSnapshots.value),
  );

  const childRunItems = computed<ADKChildRunQueueItem[]>(() => {
    const run = visibleWorkflowPlanRun.value;
    if (activeChildRunId.value.trim() === "") {
      return parentChildRunItems.value;
    }
    return buildChildRunItems(run, childRunSnapshots.value);
  });

  function buildChildRunItems(
    run: ADKRun | null,
    snapshots: Record<string, ADKRun>,
  ): ADKChildRunQueueItem[] {
    const ids = workflowChildRunIds(run);
    const stepByChildRunId = workflowStepByChildRunId(run);
    return ids.map((id, index) => {
      const runSnapshot = snapshots[id];
      const stepMeta = stepByChildRunId.get(id);
      const stepStatus = effectiveWorkflowStepStatus(run, stepMeta?.step);
      return {
        id,
        index: index + 1,
        stepIndex: stepMeta ? stepMeta.index + 1 : undefined,
        stepTitle: stepMeta?.step.title,
        stepMessage: stepMeta?.step.message || stepMeta?.step.description,
        run: runSnapshot,
        status: effectiveChildRunStatus(run, runSnapshot, stepStatus),
        updatedAt: runSnapshot?.updatedAt,
        pendingApprovalCount: pendingApprovals(runSnapshot?.pendingApprovals)
          .length,
      };
    });
  }

  const parentApprovalQueue = computed<ADKApprovalQueueItem[]>(() =>
    buildApprovalQueue({
      activeChildRunId: "",
      childItems: parentChildRunItems.value,
      childRunSnapshots: childRunSnapshots.value,
      timelineEntries: options.timelineEntries.value,
      workflowRun: parentWorkflowPlanRun.value,
    }),
  );

  const selectedApprovalQueue = computed<ADKApprovalQueueItem[]>(() =>
    buildApprovalQueue({
      activeChildRunId: activeChildRunId.value,
      childItems: activeChildRunId.value ? childRunItems.value : parentChildRunItems.value,
      childRunSnapshots: childRunSnapshots.value,
      timelineEntries: options.timelineEntries.value,
      workflowRun: conversationRun.value,
    }),
  );

  const childTimelineEntries = computed(() => {
    const childRunId = activeChildRunId.value.trim();
    if (childRunId === "") return options.timelineEntries.value;
    return options.timelineEntries.value.filter(
      (entry) =>
        String(entry.runId ?? "").trim() === childRunId &&
        entry.kind !== "approval_group",
    );
  });

  const parentTimelineEntries = computed(() =>
    buildParentTimelineEntries({
      childItems: parentChildRunItems.value,
      parentRun: parentWorkflowPlanRun.value,
      selectedSessionId: options.selectedSessionId.value,
      timelineEntries: options.timelineEntries.value,
    }),
  );

  const visibleTimelineEntries = computed(() =>
    activeChildRunId.value.trim() === ""
      ? parentTimelineEntries.value
      : childTimelineEntries.value,
  );

  const childViewContext = computed(() => {
    const childRunId = activeChildRunId.value.trim();
    if (childRunId === "") return null;
    const item = parentChildRunItems.value.find((candidate) => candidate.id === childRunId);
    return {
      title: `子智能体 #${item?.index ?? "?"}`,
      runId: childRunId,
      message:
        item?.run?.userMessage ||
        item?.stepMessage ||
        item?.stepTitle ||
        item?.run?.message ||
        "子智能体上下文",
    };
  });

  async function syncWorkflowRun(run: ADKRun | undefined): Promise<void> {
    if (!run) return;
    const normalizedRun = normalizeADKRun(run);
    if (normalizedRun.parentRunId) {
      childRunSnapshots.value = {
        ...childRunSnapshots.value,
        [normalizedRun.id]: normalizedRun,
      };
      return;
    }
    if (isDisplayableWorkflowPlanRun(normalizedRun)) {
      workflowPlanRun.value = normalizedRun;
      await refreshChildRunSnapshots(normalizedRun);
      return;
    }
    const current = workflowPlanRun.value;
    if (
      current &&
      normalizedRun.id !== current.id &&
      normalizedRun.sessionId === current.sessionId
    ) {
      clearWorkflowQueues();
    }
  }

  function setActiveChildRunId(runId: string): void {
    const normalized = runId.trim();
    activeChildRunId.value = parentChildRunItems.value.some((item) => item.id === normalized)
      ? normalized
      : "";
  }

  function clearWorkflowQueues(): void {
    workflowPlanRun.value = null;
    childRunSnapshots.value = {};
    activeChildRunId.value = "";
  }

  async function refreshChildRunSnapshots(run: ADKRun): Promise<void> {
    const ids = workflowChildRunIds(run);
    if (ids.length === 0) {
      childRunSnapshots.value = {};
      activeChildRunId.value = "";
      return;
    }
    const retained: Record<string, ADKRun> = {};
    const stepByChildRunId = workflowStepByChildRunId(run);
    for (const id of ids) {
      const existing = childRunSnapshots.value[id];
      const stepMeta = stepByChildRunId.get(id);
      const stepStatus = effectiveWorkflowStepStatus(run, stepMeta?.step);
      retained[id] = existing ?? ({
        id,
        sessionId: run.sessionId,
        agentId: run.agentId,
        status: childStatusFromWorkflowStepStatus(stepStatus) || "PENDING",
        message: "",
        toolCalls: [],
        pendingApprovals: [],
        createdAt: run.createdAt,
        updatedAt: run.updatedAt,
        parentRunId: run.id,
      } satisfies ADKRun);
    }
    childRunSnapshots.value = retained;
    if (
      activeChildRunId.value &&
      !ids.includes(activeChildRunId.value)
    ) {
      activeChildRunId.value = "";
    }
    await Promise.all(
      ids.map(async (id) => {
        try {
          const latest = normalizeADKRun(
            await fetchEnvelope<ADKRun>(
              `/api/v1/adk/runs/${encodeURIComponent(id)}`,
            ),
          );
          if (latest.id !== id) return;
          childRunSnapshots.value = {
            ...childRunSnapshots.value,
            [id]: latest,
          };
        } catch {
          // Child snapshots are best-effort; the parent plan remains authoritative.
        }
      }),
    );
  }

  return {
    activeChildRunId,
    childRunItems,
    childRunSnapshots,
    childTimelineEntries,
    childViewContext,
    clearWorkflowQueues,
    parentApprovalQueue,
    parentChildRunItems,
    parentTimelineEntries,
    parentWorkflowPlanRun,
    selectedApprovalQueue,
    setActiveChildRunId,
    syncWorkflowRun,
    visibleTimelineEntries,
    visibleWorkflowPlanRun,
  };
}

export function workflowQueueTone(status: string | undefined): string {
  switch (String(status ?? "").trim().toUpperCase()) {
    case "DONE":
    case "COMPLETED":
    case "SUCCEEDED":
      return "success";
    case "BLOCKED":
    case "PENDING_APPROVAL":
    case "PENDING":
      return "warning";
    case "IN_PROGRESS":
    case "RUNNING":
      return "info";
    case "FAILED":
    case "TIMED_OUT":
    case "DENIED":
      return "error";
    case "TODO":
    case "CANCELLED":
    case "CANCELED":
      return "muted";
    default:
      return runStatusTone(status);
  }
}

export function sessionContextFromRunUsage(
  run: ADKRun | null | undefined,
  fallbackContext: ADKSessionContextSnapshot | null | undefined,
): ADKSessionContextSnapshot | null {
  const tokensIn = Math.max(0, Math.round(run?.usage?.tokensIn ?? 0));
  const tokensOut = Math.max(0, Math.round(run?.usage?.tokensOut ?? 0));
  const totalTokens = tokensIn + tokensOut;
  if (totalTokens <= 0) {
    return fallbackContext ?? null;
  }
  const contextWindowTokens = Math.max(
    0,
    Math.round(fallbackContext?.contextWindowTokens ?? 0),
  );
  const usageRatio =
    contextWindowTokens > 0 ? totalTokens / contextWindowTokens : 0;
  const status = contextWindowTokens > 0
    ? contextStatusFromUsageRatio(usageRatio)
    : "unknown";
  const breakdown = {
    instructionTokens: tokensIn,
    handoffTokens: 0,
    recentUserTokens: 0,
    protectedTailTokens: 0,
    otherVisibleTokens: tokensOut,
    pendingUserTokens: 0,
    toolDeclarationTokens: 0,
  };
  return {
    ...(fallbackContext ?? {}),
    sessionId: run?.sessionId || fallbackContext?.sessionId || "",
    currentInputTokens: totalTokens,
    projectedNextTurnTokens: totalTokens,
    rawCurrentInputTokens: totalTokens,
    rawProjectedNextTurnTokens: totalTokens,
    contextWindowTokens,
    usageRatio,
    status,
    recentUserWindow: fallbackContext?.recentUserWindow ?? 0,
    retainedRecentUserCount: fallbackContext?.retainedRecentUserCount ?? 0,
    activeHandoffCount: fallbackContext?.activeHandoffCount ?? 0,
    summaryPreview: `子智能体运行用量：输入 ${tokensIn}，输出 ${tokensOut}`,
    breakdown,
    rawBreakdown: breakdown,
    trimmedToolResponseCount: 0,
    autoCompacted: fallbackContext?.autoCompacted ?? false,
    degradedSummary: fallbackContext?.degradedSummary ?? false,
  };
}

function contextStatusFromUsageRatio(ratio: number): ADKSessionContextSnapshot["status"] {
  if (ratio >= 0.95) return "critical";
  if (ratio >= 0.85) return "near_limit";
  if (ratio >= 0.7) return "warning";
  return "healthy";
}

function isDisplayableWorkflowPlanRun(
  run: ADKRun | null | undefined,
): boolean {
  if (!run) return false;
  if (String(run.workMode ?? "").trim().toLowerCase() === "chat") return false;
  return (run.workflowPlan ?? []).length > 0;
}

function workflowChildRunIds(run: ADKRun | null | undefined): string[] {
  const ids = new Set<string>();
  for (const id of run?.childRunIds ?? []) {
    const normalized = id.trim();
    if (normalized !== "") ids.add(normalized);
  }
  for (const step of run?.workflowPlan ?? []) {
    const normalized = String(step.childRunId ?? "").trim();
    if (normalized !== "") ids.add(normalized);
  }
  return [...ids];
}

function workflowStepByChildRunId(
  run: ADKRun | null | undefined,
): Map<string, { step: ADKWorkflowStepState; index: number }> {
  const steps = new Map<string, { step: ADKWorkflowStepState; index: number }>();
  (run?.workflowPlan ?? []).forEach((step, index) => {
    const childRunId = String(step.childRunId ?? "").trim();
    if (childRunId !== "" && !steps.has(childRunId)) {
      steps.set(childRunId, { step, index });
    }
  });
  return steps;
}

function effectiveWorkflowStepStatus(
  run: ADKRun | null | undefined,
  step: ADKWorkflowStepState | undefined,
): string {
  const status = String(step?.status ?? "").trim();
  const runStatus = String(run?.status ?? "").trim().toUpperCase();
  const workflowStatus = String(run?.workflowStatus ?? "").trim().toUpperCase();
  if (
    String(step?.childRunId ?? "").trim() !== "" &&
    (runStatus === "COMPLETED" ||
      workflowStatus === "COMPLETED" ||
      workflowStatus === "COMPLETE")
  ) {
    return "DONE";
  }
  return status;
}

function effectiveChildRunStatus(
  parentRun: ADKRun | null | undefined,
  childRun: ADKRun | undefined,
  stepStatus: string,
): string {
  const normalizedStepStatus = String(stepStatus).trim().toUpperCase();
  if (normalizedStepStatus === "DONE") {
    return "COMPLETED";
  }
  const runStatus = String(parentRun?.status ?? "").trim().toUpperCase();
  const workflowStatus = String(parentRun?.workflowStatus ?? "").trim().toUpperCase();
  if (
    runStatus === "COMPLETED" ||
    workflowStatus === "COMPLETED" ||
    workflowStatus === "COMPLETE"
  ) {
    return "COMPLETED";
  }
  if (
    normalizedStepStatus === "BLOCKED" &&
    pendingApprovals(childRun?.pendingApprovals).length > 0
  ) {
    return "PENDING_APPROVAL";
  }
  const childStatus = String(childRun?.status ?? "").trim();
  return childStatus || childStatusFromWorkflowStepStatus(stepStatus) || "PENDING";
}

function childStatusFromWorkflowStepStatus(status: string): string {
  switch (String(status).trim().toUpperCase()) {
    case "DONE":
      return "COMPLETED";
    case "IN_PROGRESS":
      return "RUNNING";
    case "BLOCKED":
      return "PENDING_APPROVAL";
    case "TODO":
      return "PENDING";
    default:
      return String(status ?? "").trim();
  }
}

function buildApprovalQueue(options: {
  activeChildRunId: string;
  childItems: ADKChildRunQueueItem[];
  childRunSnapshots: Record<string, ADKRun>;
  timelineEntries: ADKTimelineEntryState[];
  workflowRun: ADKRun | null;
}): ADKApprovalQueueItem[] {
  const childByRunId = new Map(options.childItems.map((item) => [item.id, item]));
  const approvals: ADKApproval[] = [];
  approvals.push(...pendingApprovals(options.workflowRun?.pendingApprovals));
  for (const run of Object.values(options.childRunSnapshots)) {
    approvals.push(...pendingApprovals(run.pendingApprovals));
  }
  for (const entry of options.timelineEntries) {
    if (entry.kind === "approval_group") {
      approvals.push(...approvalsForGroup(entry));
    }
  }

  return uniqueADKApprovalsById(approvals)
    .filter((approval) => {
      if (options.activeChildRunId === "") return true;
      return String(approval.runId ?? "").trim() === options.activeChildRunId;
    })
    .map((approval) => {
      const runId = String(approval.runId ?? "").trim();
      const child = childByRunId.get(runId);
      return {
        approval,
        runId,
        childRunId: child?.id,
        childIndex: child?.index,
        stepTitle: child?.stepTitle,
      };
    });
}

function buildParentTimelineEntries(options: {
  childItems: ADKChildRunQueueItem[];
  parentRun: ADKRun | null;
  selectedSessionId: string;
  timelineEntries: ADKTimelineEntryState[];
}): ADKTimelineEntryState[] {
  if (!options.parentRun || options.childItems.length === 0) {
    return options.timelineEntries;
  }
  const childRunIds = new Set(options.childItems.map((item) => item.id));
  const filteredEntries = options.timelineEntries.flatMap((entry) => {
    const projected = projectParentTimelineEntry(entry, childRunIds);
    return projected ? [projected] : [];
  });
  const lifecycleEntries = buildChildLifecycleEntries({
    childItems: options.childItems,
    parentRun: options.parentRun,
    selectedSessionId: options.selectedSessionId,
    timelineEntries: options.timelineEntries,
  });
  return sortTimelineEntries([...filteredEntries, ...lifecycleEntries]);
}

function projectParentTimelineEntry(
  entry: ADKTimelineEntryState,
  childRunIds: Set<string>,
): ADKTimelineEntryState | null {
  const runId = String(entry.runId ?? "").trim();
  if (runId !== "" && childRunIds.has(runId)) {
    return null;
  }
  if (entry.kind === "tool_group" && entry.toolCalls) {
    const toolCalls = entry.toolCalls.filter(
      (toolCall) => !childRunIds.has(String(toolCall.runId ?? "").trim()),
    );
    if (toolCalls.length === 0) {
      return null;
    }
    if (toolCalls.length !== entry.toolCalls.length) {
      const retainedIds = new Set(toolCalls.map((toolCall) => toolCall.id));
      const projected: ADKTimelineEntryState = {
        ...entry,
        toolCalls,
      };
      if (entry.expandedToolCallIds) {
        projected.expandedToolCallIds = entry.expandedToolCallIds.filter((id) =>
          retainedIds.has(id),
        );
      }
      return projected;
    }
  }
  if (entry.kind === "approval_group" && entry.approvals) {
    const approvals = entry.approvals.filter(
      (approval) => !childRunIds.has(String(approval.runId ?? "").trim()),
    );
    if (approvals.length === 0) {
      return null;
    }
    if (approvals.length !== entry.approvals.length) {
      return {
        ...entry,
        approvals,
      };
    }
  }
  return entry;
}

function buildChildLifecycleEntries(options: {
  childItems: ADKChildRunQueueItem[];
  parentRun: ADKRun;
  selectedSessionId: string;
  timelineEntries: ADKTimelineEntryState[];
}): ADKTimelineEntryState[] {
  const baseSequence = Math.max(
    options.timelineEntries.length,
    ...options.timelineEntries.map((entry) => entry.sequence ?? 0),
  );
  return options.childItems.flatMap((item) => {
    const label = childLifecycleLabel(item);
    const sessionId =
      options.parentRun.sessionId ||
      item.run?.sessionId ||
      options.selectedSessionId ||
      "";
    const runId = options.parentRun.id;
    const startTime =
      item.run?.createdAt ||
      options.parentRun.createdAt ||
      options.timelineEntries[0]?.createdAt ||
      "";
    const statusTime =
      item.run?.updatedAt ||
      item.run?.createdAt ||
      options.parentRun.updatedAt ||
      options.parentRun.createdAt ||
      startTime;
    return [
      {
        id: `workflow-child-start:${options.parentRun.id}:${item.id}`,
        sessionId,
        runId,
        kind: "assistant_message",
        createdAt: startTime,
        sequence: baseSequence + item.index * 2 - 1,
        status: "final",
        text: `启动子智能体 #${item.index}：${label}（${item.id}）`,
      },
      {
        id: `workflow-child-status:${options.parentRun.id}:${item.id}`,
        sessionId,
        runId,
        kind: "assistant_message",
        createdAt: statusTime,
        sequence: baseSequence + item.index * 2,
        status: "final",
        text: childLifecycleStatusText(item, label),
      },
    ];
  });
}

function childLifecycleLabel(item: ADKChildRunQueueItem): string {
  return (
    item.stepTitle ||
    item.stepMessage ||
    item.run?.userMessage ||
    item.run?.message ||
    "子智能体"
  );
}

function childLifecycleStatusText(
  item: ADKChildRunQueueItem,
  label: string,
): string {
  const prefix = `子智能体 #${item.index}`;
  const suffix = `${label}（${item.id}）`;
  const status = String(item.status ?? "").trim().toUpperCase();
  if (item.pendingApprovalCount > 0 || status === "PENDING_APPROVAL" || status === "BLOCKED") {
    return `${prefix} 等待审批：${suffix}`;
  }
  switch (status) {
    case "DONE":
    case "COMPLETED":
    case "SUCCEEDED":
      return `${prefix} 已结束：已完成 · ${suffix}`;
    case "FAILED":
    case "TIMED_OUT":
    case "DENIED":
      return `${prefix} 已结束：失败 · ${suffix}`;
    case "CANCELLED":
    case "CANCELED":
      return `${prefix} 已结束：已取消 · ${suffix}`;
    case "IN_PROGRESS":
    case "RUNNING":
      return `${prefix} 正在运行：${suffix}`;
    case "TODO":
    case "PENDING":
      return `${prefix} 等待启动：${suffix}`;
    default:
      return status === ""
        ? `${prefix} 状态未知：${suffix}`
        : `${prefix} 状态 ${status}：${suffix}`;
  }
}

function pendingApprovals(
  approvals: ADKApproval[] | null | undefined,
): ADKApproval[] {
  return (approvals ?? []).filter((approval) => {
    const status = String(approval.status ?? "").trim().toUpperCase();
    return status === "" || status === "PENDING" || status === "PENDING_APPROVAL";
  });
}
