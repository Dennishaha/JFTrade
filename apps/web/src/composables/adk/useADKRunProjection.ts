import { computed, type Ref } from "vue";

import type { ADKAgent, ADKInputRequest, ADKRun } from "@/types";
import {
  buildQueueSessionKey,
  isBlockingRunStatus,
  isResumableTimedOutGoalRun,
  isUserPauseRequestedGoalRun,
  selectActiveGoalRun,
  selectPrimaryRootRun,
  type ActiveChatRunState,
  type QueuedChatMessage,
} from "./adkChatRuntime";
import { isUserPausedGoalRun } from "./adkChatPresentation";
import type { ADKWorkflowQueueState } from "./useADKWorkflowQueueState";

export interface SlashCommandItem {
  id: "context" | "compact" | "compact-aggressive";
  command: "/context" | "/compact" | "/compact-aggressive";
  title: string;
  description: string;
  disabled?: boolean;
}

interface ADKRunProjectionInput {
  activeGoalRunSnapshot: Ref<ADKRun | null>;
  activeRun: Ref<ActiveChatRunState | null>;
  activeRunSnapshot: Ref<ADKRun | null>;
  agents: Ref<ADKAgent[]>;
  chatDraft: Ref<string>;
  composerBlockMessage: Ref<string>;
  goalLifecycleBusy: Ref<boolean>;
  goalObjectiveDraft: Ref<string>;
  goalObjectiveSaving: Ref<boolean>;
  queuedChatMessages: Ref<QueuedChatMessage[]>;
  selectedAgentId: Ref<string>;
  selectedSessionId: Ref<string>;
  sendingChat: Ref<boolean>;
  workModeOverride: Ref<string>;
  workflowQueues: ADKWorkflowQueueState;
}

export function useADKRunProjection(input: ADKRunProjectionInput) {
  const activeRunId = computed(() => input.activeRun.value?.runId ?? "");
  const activeRunStatus = computed(() => input.activeRun.value?.status ?? "");
  const activeGoalRun = computed(() =>
    selectActiveGoalRun({
      activeRunSnapshot: input.activeRunSnapshot.value,
      activeGoalRunSnapshot: input.activeGoalRunSnapshot.value,
      workflowRun: input.workflowQueues.parentWorkflowPlanRun.value,
    }),
  );
  const primaryRootRun = computed(() =>
    selectPrimaryRootRun({
      activeRunSnapshot: input.activeRunSnapshot.value,
      activeGoalRunSnapshot: input.activeGoalRunSnapshot.value,
      workflowRun: input.workflowQueues.parentWorkflowPlanRun.value,
    }),
  );
  const pendingInputRequest = computed<ADKInputRequest | null>(() => {
    const request = primaryRootRun.value?.inputRequest;
    return request?.status === "PENDING" ? request : null;
  });
  const selectedAgentDefaultWorkMode = computed(() => {
    const agent = input.agents.value.find(
      (candidate) => candidate.id === input.selectedAgentId.value,
    );
    return String(agent?.workMode ?? "").trim() === "loop" ? "loop" : "chat";
  });
  const effectiveWorkMode = computed(() =>
    String(
      input.workModeOverride.value || selectedAgentDefaultWorkMode.value,
    ).trim() === "loop"
      ? "loop"
      : "chat",
  );
  const showGoalObjectiveEditor = computed(
    () => activeGoalRun.value != null || input.goalObjectiveDraft.value.trim() !== "",
  );
  const canSaveGoalObjective = computed(() => {
    const run = activeGoalRun.value;
    return (
      !!run &&
      !input.goalObjectiveSaving.value &&
      input.goalObjectiveDraft.value.trim() !== "" &&
      input.goalObjectiveDraft.value.trim() !== String(run.objective ?? "").trim()
    );
  });
  const activeGoalRunId = computed(() => activeGoalRun.value?.id ?? "");
  const goalPaused = computed(() =>
    isUserPausedGoalRun(activeGoalRun.value ?? undefined),
  );
  const goalTimedOut = computed(() =>
    isResumableTimedOutGoalRun(activeGoalRun.value ?? undefined),
  );
  const goalPauseRequested = computed(() =>
    isUserPauseRequestedGoalRun(activeGoalRun.value ?? undefined),
  );
  const canPauseGoal = computed(() => {
    const run = activeGoalRun.value;
    return (
      !!run &&
      run.status === "RUNNING" &&
      !goalPauseRequested.value &&
      !input.goalLifecycleBusy.value
    );
  });
  const canResumeGoal = computed(
    () =>
      (goalPaused.value || goalTimedOut.value) &&
      !input.goalLifecycleBusy.value,
  );
  const hasBlockingRun = computed(() =>
    primaryRootRun.value
      ? isBlockingRunStatus(primaryRootRun.value.status)
      : isBlockingRunStatus(input.activeRun.value?.status),
  );
  const activeRunControlId = computed(() => {
    if (
      primaryRootRun.value &&
      isBlockingRunStatus(primaryRootRun.value.status)
    ) {
      return primaryRootRun.value.id;
    }
    return input.activeRun.value?.runId ?? "";
  });
  const currentQueueSessionKey = computed(() =>
    buildQueueSessionKey(input.selectedSessionId.value),
  );
  const queuedMessages = computed(() =>
    input.queuedChatMessages.value.filter(
      (message) => message.sessionKey === currentQueueSessionKey.value,
    ),
  );
  const canSendChat = computed(
    () =>
      input.chatDraft.value.trim() !== "" &&
      input.selectedAgentId.value !== "" &&
      input.composerBlockMessage.value === "",
  );
  const canInterruptChat = computed(
    () => canSendChat.value && hasBlockingRun.value,
  );
  const activityIndicator = computed<"idle" | "typing" | "child_finished">(
    () => {
      if (!input.sendingChat.value && !hasBlockingRun.value) return "idle";
      const parent = input.workflowQueues.parentWorkflowPlanRun.value;
      const children = input.workflowQueues.parentChildRunItems.value;
      const parentActive =
        !!parent &&
        !["COMPLETED", "FAILED", "CANCELLED", "DENIED", "TIMED_OUT"].includes(
          parent.status,
        );
      const childrenFinished =
        children.length > 0 &&
        children.every((child) =>
          [
            "COMPLETED",
            "DONE",
            "FAILED",
            "CANCELLED",
            "DENIED",
            "TIMED_OUT",
          ].includes(String(child.status).trim().toUpperCase()),
        );
      return parentActive && childrenFinished ? "child_finished" : "typing";
    },
  );
  const slashCommands = computed<SlashCommandItem[]>(() => {
    const hasSession = input.selectedSessionId.value.trim() !== "";
    return [
      {
        id: "context",
        command: "/context",
        title: "查看上下文占用",
        description: hasSession
          ? "打开当前会话的上下文详情"
          : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
      {
        id: "compact",
        command: "/compact",
        title: "压缩当前会话",
        description: hasSession ? "执行标准上下文压缩" : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
      {
        id: "compact-aggressive",
        command: "/compact-aggressive",
        title: "激进压缩当前会话",
        description: hasSession ? "执行更强的摘要压缩" : "需要先创建或选择一个会话",
        disabled: !hasSession,
      },
    ];
  });

  return {
    activeGoalRun,
    activeGoalRunId,
    activeRunControlId,
    activeRunId,
    activeRunStatus,
    activityIndicator,
    canInterruptChat,
    canPauseGoal,
    canResumeGoal,
    canSaveGoalObjective,
    canSendChat,
    currentQueueSessionKey,
    effectiveWorkMode,
    goalPaused,
    goalPauseRequested,
    goalTimedOut,
    hasBlockingRun,
    pendingInputRequest,
    primaryRootRun,
    queuedMessages,
    showGoalObjectiveEditor,
    slashCommands,
  };
}
