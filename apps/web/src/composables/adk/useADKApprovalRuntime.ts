import { ref, type Ref } from "vue";

import type {
  ADKApproval,
  ADKApprovalResolution,
  ADKInputAnswer,
  ADKInputRequest,
  ADKRun,
} from "@/types";
import {
  resolveADKApprovalBatchOnce,
  type ADKApprovalAction,
} from "./adkApprovalResolution";
import {
  isTerminalRunStatus,
  runErrorDisplayMessage,
} from "./adkChatPresentation";
import { waitForGoalAwareRunContinuation } from "./adkChatRuntime";
import {
  normalizeADKApprovalResolution,
  normalizeADKRun,
} from "./adkNormalization";
import { monitorADKRunContinuation } from "./adkRunContinuation";
import { applyApprovalResolutions, type ADKTimelineEntryState } from "./adkTimeline";
import type { ADKWorkflowQueueState } from "./useADKWorkflowQueueState";
import {
  requireADKApprovalResolution,
  requireADKInputResolution,
} from "./adkApiMappers";
import { apiPostPath, apiPostPathAction } from "../shared/apiClient";

interface ApprovalRuntimeInput {
  dispatchQueuedMessagesIfIdle: () => Promise<void>;
  errorMessage: Ref<string>;
  interruptingRunId: Ref<string>;
  refreshAll: () => Promise<void>;
  refreshSessionContext: () => Promise<void>;
  reloadSessionTimeline: (sessionId: string) => Promise<void>;
  resolvingApprovalIds: Ref<Set<string>>;
  selectedSessionId: Ref<string>;
  syncActiveRun: (
    run: ADKRun | undefined,
    waitingForContinuation?: boolean,
  ) => ADKRun | undefined;
  timelineEntries: Ref<ADKTimelineEntryState[]>;
  workflowQueues: ADKWorkflowQueueState;
}

export function useADKApprovalRuntime(input: ApprovalRuntimeInput) {
  const approvalsBusy = ref(false);
  const resolvingInputIds = ref<Set<string>>(new Set());

  async function submitApproval(
    approval: ADKApproval,
    action: ADKApprovalAction,
  ): Promise<ADKApprovalResolution> {
    return normalizeADKApprovalResolution(
      requireADKApprovalResolution(
        await apiPostPathAction(
          action === "approve"
            ? "/api/v1/adk/approvals/{approvalId}/approve"
            : "/api/v1/adk/approvals/{approvalId}/deny",
          `/api/v1/adk/approvals/${encodeURIComponent(approval.id)}/${action}`,
        ),
      ),
    );
  }

  async function resolveApprovalsBatch(
    approvals: ADKApproval[],
    action: ADKApprovalAction,
  ): Promise<void> {
    if (approvals.length === 0 || approvalsBusy.value) return;
    const resolvingIds = approvals
      .map((approval) => String(approval.id ?? "").trim())
      .filter((id) => id !== "");
    input.resolvingApprovalIds.value = new Set([
      ...input.resolvingApprovalIds.value,
      ...resolvingIds,
    ]);
    approvalsBusy.value = true;
    try {
      const { resolutions, errors } = await resolveADKApprovalBatchOnce({
        approvals,
        action,
        submit: submitApproval,
        onResolution: (resolution) => {
          input.timelineEntries.value = applyApprovalResolutions(
            input.timelineEntries.value,
            [resolution],
          );
          if (!resolution.run) return;
          const run = resolution.parentRun ?? resolution.run;
          void input.workflowQueues.syncWorkflowRun(resolution.run);
          void input.workflowQueues.syncWorkflowRun(resolution.parentRun);
          input.syncActiveRun(run, !isTerminalRunStatus(run.status));
        },
      });
      if (resolutions.length > 0) await finalizeApprovalBatch(resolutions);
      if (errors.length > 0) {
        input.errorMessage.value =
          errors.length === 1 ? errors[0]! : `批量审批部分失败：${errors[0]}`;
      }
    } finally {
      const remaining = new Set(input.resolvingApprovalIds.value);
      resolvingIds.forEach((id) => remaining.delete(id));
      input.resolvingApprovalIds.value = remaining;
      approvalsBusy.value = false;
    }
  }

  async function finalizeApprovalBatch(
    resolutions: ADKApprovalResolution[],
  ): Promise<void> {
    await input.refreshAll();
    await input.refreshSessionContext();
    await input.reloadSessionTimeline(input.selectedSessionId.value);
    const runs = Array.from(
      new Map(
        resolutions
          .map((resolution) => resolution.parentRun ?? resolution.run)
          .filter((run): run is ADKRun => run != null)
          .map((run) => [run.id, run]),
      ).values(),
    );
    for (const run of runs) {
      input.syncActiveRun(run, !isTerminalRunStatus(run.status));
      if (isTerminalRunStatus(run.status)) {
        await handleTerminalRun(run);
      } else {
        void waitForRunContinuation(run);
      }
    }
  }

  async function waitForRunContinuation(
    run: ADKRun | undefined,
  ): Promise<void> {
    if (!run) return;
    const sessionId = run.sessionId || input.selectedSessionId.value;
    if (!sessionId) return;
    await waitForGoalAwareRunContinuation({
      run,
      monitorRun: monitorADKRunContinuation,
      syncActiveRun: input.syncActiveRun,
      reloadTimeline: () => input.reloadSessionTimeline(sessionId),
      handleTerminalRun,
      setErrorMessage: (message) => {
        input.errorMessage.value = message;
      },
    });
  }

  async function handleTerminalRun(run: ADKRun): Promise<void> {
    input.syncActiveRun(run);
    const failMessage = runErrorDisplayMessage(run);
    if (failMessage) input.errorMessage.value = failMessage;
    if (input.interruptingRunId.value === run.id) {
      input.interruptingRunId.value = "";
    }
    await input.dispatchQueuedMessagesIfIdle();
  }

  async function resolveApproval(approval: ADKApproval): Promise<void> {
    await resolveApprovalsBatch([approval], "approve");
  }

  async function denyApproval(approval: ADKApproval): Promise<void> {
    await resolveApprovalsBatch([approval], "deny");
  }

  async function resolveAllApprovals(approvals: ADKApproval[]): Promise<void> {
    await resolveApprovalsBatch(approvals, "approve");
  }

  async function denyAllApprovals(approvals: ADKApproval[]): Promise<void> {
    await resolveApprovalsBatch(approvals, "deny");
  }

  function inputRequestBusy(requestId: string): boolean {
    return resolvingInputIds.value.has(requestId);
  }

  async function submitInputResponse(
    request: ADKInputRequest,
    answers: ADKInputAnswer[],
  ): Promise<void> {
    if (inputRequestBusy(request.id)) return;
    resolvingInputIds.value = new Set([...resolvingInputIds.value, request.id]);
    try {
      const resolution = requireADKInputResolution(
        await apiPostPath(
          "/api/v1/adk/runs/{runId}/input-response",
          `/api/v1/adk/runs/${encodeURIComponent(request.runId)}/input-response`,
          { requestId: request.id, answers },
        ),
      );
      const run = resolution.parentRun ?? resolution.run;
      if (resolution.run) void input.workflowQueues.syncWorkflowRun(resolution.run);
      if (resolution.parentRun) {
        void input.workflowQueues.syncWorkflowRun(resolution.parentRun);
      }
      if (run) input.syncActiveRun(normalizeADKRun(run), true);
      await input.refreshAll();
      await input.reloadSessionTimeline(input.selectedSessionId.value);
      if (run) void waitForRunContinuation(normalizeADKRun(run));
    } catch (error) {
      input.errorMessage.value =
        error instanceof Error ? error.message : "提交回答失败";
    } finally {
      const next = new Set(resolvingInputIds.value);
      next.delete(request.id);
      resolvingInputIds.value = next;
    }
  }

  return {
    approvalsBusy,
    denyAllApprovals,
    denyApproval,
    handleTerminalRun,
    inputRequestBusy,
    resolveAllApprovals,
    resolveApproval,
    submitInputResponse,
    waitForRunContinuation,
  };
}
