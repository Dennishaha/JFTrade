import type { Ref } from "vue";

import type { ADKRun } from "@/types";
import {
  isTerminalRunStatus,
} from "./adkChatPresentation";
import { shouldWaitForRunContinuation } from "./adkChatRuntime";
import { normalizeADKRun } from "./adkNormalization";
import { requireADKRun } from "./adkApiMappers";
import { apiPatchPath, apiPostPathAction } from "../shared/apiClient";

interface ADKRunActionsInput {
  abortActiveChatStream: (reason?: string) => void;
  abortReconnectStream: () => void;
  activeGoalRun: Ref<ADKRun | null | undefined>;
  activeGoalRunId: Ref<string>;
  activeRunControlId: Ref<string>;
  clearSessionRuntimeState: (sessionId: string) => void;
  errorMessage: Ref<string>;
  goalLifecycleBusy: Ref<boolean>;
  goalObjectiveDraft: Ref<string>;
  goalObjectiveError: Ref<string>;
  goalObjectiveSaving: Ref<boolean>;
  goalObjectiveTouched: Ref<boolean>;
  handleTerminalRun: (run: ADKRun) => Promise<void>;
  interruptingRunId: Ref<string>;
  reloadSessionTimeline: (sessionId: string) => Promise<void>;
  selectedSessionId: Ref<string>;
  syncActiveRun: (
    run: ADKRun | undefined,
    waitingForContinuation?: boolean,
  ) => ADKRun | undefined;
  waitForRunContinuation: (run: ADKRun | undefined) => Promise<void>;
}

export function useADKRunActions(input: ADKRunActionsInput) {
  async function cancelActiveRun(
    runId = input.activeRunControlId.value,
  ): Promise<void> {
    if (!runId) return;
    try {
      const run = normalizeADKRun(
        requireADKRun(
          await apiPostPathAction(
            "/api/v1/adk/runs/{runId}/cancel",
            `/api/v1/adk/runs/${encodeURIComponent(runId)}/cancel`,
          ),
        ),
      );
      input.syncActiveRun(run, !isTerminalRunStatus(run.status));
      await input.reloadSessionTimeline(
        run.sessionId || input.selectedSessionId.value,
      );
      if (isTerminalRunStatus(run.status)) {
        await input.handleTerminalRun(run);
      } else {
        await input.waitForRunContinuation(run);
      }
    } catch (error) {
      input.errorMessage.value =
        error instanceof Error ? error.message : "取消运行失败";
    } finally {
      if (input.interruptingRunId.value === runId) {
        input.interruptingRunId.value = "";
      }
    }
  }

  async function pauseGoalRun(): Promise<void> {
    const runId = input.activeGoalRunId.value;
    if (!runId || input.goalLifecycleBusy.value) return;
    input.goalLifecycleBusy.value = true;
    try {
      const run = normalizeADKRun(
        requireADKRun(
          await apiPostPathAction(
            "/api/v1/adk/runs/{runId}/pause",
            `/api/v1/adk/runs/${encodeURIComponent(runId)}/pause`,
          ),
        ),
      );
      input.abortActiveChatStream("goal_pause");
      input.abortReconnectStream();
      input.clearSessionRuntimeState(
        run.sessionId || input.selectedSessionId.value,
      );
      input.syncActiveRun(run, shouldWaitForRunContinuation(run));
      await input.reloadSessionTimeline(
        run.sessionId || input.selectedSessionId.value,
      );
      if (shouldWaitForRunContinuation(run)) {
        void input.waitForRunContinuation(run);
      }
    } catch (error) {
      input.errorMessage.value =
        error instanceof Error ? error.message : "暂停目标失败";
    } finally {
      input.goalLifecycleBusy.value = false;
    }
  }

  async function resumeGoalRun(): Promise<void> {
    const runId = input.activeGoalRunId.value;
    if (!runId || input.goalLifecycleBusy.value) return;
    input.goalLifecycleBusy.value = true;
    try {
      const run = normalizeADKRun(
        requireADKRun(
          await apiPostPathAction(
            "/api/v1/adk/runs/{runId}/resume",
            `/api/v1/adk/runs/${encodeURIComponent(runId)}/resume`,
          ),
        ),
      );
      input.syncActiveRun(run, true);
      await input.reloadSessionTimeline(
        run.sessionId || input.selectedSessionId.value,
      );
      await input.waitForRunContinuation(run);
    } catch (error) {
      input.errorMessage.value =
        error instanceof Error ? error.message : "运行目标失败";
    } finally {
      input.goalLifecycleBusy.value = false;
    }
  }

  function updateGoalObjectiveDraft(value: string): void {
    input.goalObjectiveTouched.value = true;
    input.goalObjectiveDraft.value = value;
    input.goalObjectiveError.value = "";
  }

  async function updateGoalObjective(): Promise<void> {
    const run = input.activeGoalRun.value;
    const objective = input.goalObjectiveDraft.value.trim();
    if (!run || objective === "" || input.goalObjectiveSaving.value) return;
    input.goalObjectiveSaving.value = true;
    input.goalObjectiveError.value = "";
    try {
      const updated = normalizeADKRun(
        requireADKRun(
          await apiPatchPath(
            "/api/v1/adk/runs/{runId}/objective",
            `/api/v1/adk/runs/${encodeURIComponent(run.id)}/objective`,
            { objective },
          ),
        ),
      );
      input.syncActiveRun(updated, !isTerminalRunStatus(updated.status));
      input.goalObjectiveDraft.value = updated.objective ?? objective;
      input.goalObjectiveTouched.value = false;
    } catch (error) {
      input.goalObjectiveError.value =
        error instanceof Error ? error.message : "目标保存失败";
      input.errorMessage.value = input.goalObjectiveError.value;
    } finally {
      input.goalObjectiveSaving.value = false;
    }
  }

  return {
    cancelActiveRun,
    pauseGoalRun,
    resumeGoalRun,
    updateGoalObjective,
    updateGoalObjectiveDraft,
  };
}
