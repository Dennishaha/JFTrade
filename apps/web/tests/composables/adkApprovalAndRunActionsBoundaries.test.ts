// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref, type Ref } from "vue";

import type {
  ADKApproval,
  ADKInputAnswer,
  ADKInputRequest,
  ADKRun,
} from "@/types";
import type { ADKTimelineEntryState } from "@/composables/adk/adkTimeline";
import type { ADKWorkflowQueueState } from "@/composables/adk/useADKWorkflowQueueState";
import { useADKApprovalRuntime } from "@/composables/adk/useADKApprovalRuntime";
import { useADKRunActions } from "@/composables/adk/useADKRunActions";

import { createResponse } from "../helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("adk approval runtime boundaries", () => {
  it("keeps approval flows alive when a resolution carries no run", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          approval: buildApproval("approval-no-run"),
        }),
      ),
    );
    const host = mountApprovalRuntime();

    await host.api.resolveApproval(buildApproval("approval-no-run"));

    expect(host.input.refreshAll).toHaveBeenCalledTimes(1);
    expect(host.input.syncActiveRun).not.toHaveBeenCalled();
  });

  it("surfaces a single failed approval without finalizing a batch", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("approval failed");
    }));
    const host = mountApprovalRuntime();

    await host.api.resolveAllApprovals([
      buildApproval("approval-fails"),
    ]);

    expect(host.input.errorMessage.value).toBe("approval failed");
    expect(host.input.refreshAll).not.toHaveBeenCalled();
  });

  it("tolerates approvals without ids when resolving the whole queue", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("missing approval id");
    }));
    const host = mountApprovalRuntime();

    await host.api.resolveAllApprovals([approvalWithoutId()]);

    expect(host.input.errorMessage.value).toBe("missing approval id");
    expect(host.input.resolvingApprovalIds.value).toEqual(new Set());
  });

  it("stops continuation waits when the run or session is absent", async () => {
    const host = mountApprovalRuntime();

    await host.api.waitForRunContinuation(undefined);
    await host.api.waitForRunContinuation(buildRun({ sessionId: "" }));

    expect(host.input.reloadSessionTimeline).not.toHaveBeenCalled();
  });

  it("submits input responses without a run and surfaces error messages", async () => {
    const request = buildInputRequest();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createResponse({ request })),
    );
    const host = mountApprovalRuntime();

    await host.api.submitInputResponse(request, []);
    expect(host.input.syncActiveRun).not.toHaveBeenCalled();
    expect(host.input.refreshAll).toHaveBeenCalledTimes(1);

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("input failed");
      }),
    );
    await host.api.submitInputResponse(request, []);
    expect(host.input.errorMessage.value).toBe("input failed");
  });
});

describe("adk run actions boundaries", () => {
  it("falls back to the selected session when cancel responses omit it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse(buildRun({ id: "run-cancel", sessionId: "" })),
      ),
    );
    const host = mountRunActions();
    host.input.selectedSessionId.value = "session-fallback";
    host.input.activeRunControlId.value = "run-cancel";

    await host.api.cancelActiveRun();

    expect(host.input.reloadSessionTimeline).toHaveBeenCalledWith(
      "session-fallback",
    );
  });

  it("surfaces cancel, pause, and objective failures with real errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("cancel failed");
    }));
    const host = mountRunActions();
    host.input.activeRunControlId.value = "run-cancel";
    await host.api.cancelActiveRun();
    expect(host.input.errorMessage.value).toBe("cancel failed");

    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("pause failed");
    }));
    host.input.activeGoalRunId.value = "run-pause";
    await host.api.pauseGoalRun();
    expect(host.input.errorMessage.value).toBe("pause failed");

    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("objective failed");
    }));
    host.input.activeGoalRun.value = buildRun({ id: "run-objective" });
    host.input.goalObjectiveDraft.value = "new objective";
    await host.api.updateGoalObjective();
    expect(host.input.goalObjectiveError.value).toBe("objective failed");
  });

  it("falls back to the selected session for pause and resume lifecycle calls", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse(buildRun({ id: "run-pause", sessionId: "" })),
      ),
    );
    const host = mountRunActions();
    host.input.selectedSessionId.value = "session-fallback";
    host.input.activeGoalRunId.value = "run-pause";

    await host.api.pauseGoalRun();

    expect(host.input.clearSessionRuntimeState).toHaveBeenCalledWith(
      "session-fallback",
    );
    expect(host.input.reloadSessionTimeline).toHaveBeenCalledWith(
      "session-fallback",
    );

    host.input.activeGoalRunId.value = "run-resume";
    await host.api.resumeGoalRun();
    expect(host.input.reloadSessionTimeline).toHaveBeenCalledWith(
      "session-fallback",
    );
  });

  it("falls back to the draft objective and treats non-error resume failures as generic", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createResponse(buildRun({ id: "run-objective" }))),
    );
    const host = mountRunActions();
    host.input.activeGoalRun.value = buildRun({ id: "run-objective" });
    host.input.goalObjectiveDraft.value = "draft objective";

    await host.api.updateGoalObjective();

    expect(host.input.goalObjectiveDraft.value).toBe("draft objective");
    expect(host.input.goalObjectiveTouched.value).toBe(false);

    vi.stubGlobal("fetch", vi.fn(async () => {
      throw "resume failed";
    }));
    host.input.activeGoalRunId.value = "run-resume";
    await host.api.resumeGoalRun();
    expect(host.input.errorMessage.value).toBe("运行目标失败");
  });
});

function mountApprovalRuntime() {
  const errorMessage = ref("");
  const interruptingRunId = ref("");
  const resolvingApprovalIds = ref<Set<string>>(new Set());
  const selectedSessionId = ref("");
  const timelineEntries = ref<ADKTimelineEntryState[]>([]);
  const input = {
    dispatchQueuedMessagesIfIdle: vi.fn(async () => {}),
    errorMessage,
    interruptingRunId,
    refreshAll: vi.fn(async () => {}),
    refreshSessionContext: vi.fn(async () => {}),
    reloadSessionTimeline: vi.fn(async () => {}),
    resolvingApprovalIds,
    selectedSessionId,
    syncActiveRun: vi.fn(() => undefined),
    timelineEntries,
    workflowQueues: {
      syncWorkflowRun: vi.fn(),
    } as unknown as ADKWorkflowQueueState,
  };
  let api!: ReturnType<typeof useADKApprovalRuntime>;
  mount(
    defineComponent({
      setup() {
        api = useADKApprovalRuntime(input);
        return () => h("div");
      },
    }),
  );
  return { api, input };
}

function mountRunActions() {
  const input = {
    abortActiveChatStream: vi.fn(),
    abortReconnectStream: vi.fn(),
    activeGoalRun: ref<ADKRun | null | undefined>(undefined),
    activeGoalRunId: ref(""),
    activeRunControlId: ref(""),
    clearSessionRuntimeState: vi.fn(),
    errorMessage: ref(""),
    goalLifecycleBusy: ref(false),
    goalObjectiveDraft: ref(""),
    goalObjectiveError: ref(""),
    goalObjectiveSaving: ref(false),
    goalObjectiveTouched: ref(false),
    handleTerminalRun: vi.fn(async () => {}),
    interruptingRunId: ref(""),
    reloadSessionTimeline: vi.fn(async () => {}),
    selectedSessionId: ref(""),
    syncActiveRun: vi.fn(() => undefined),
    waitForRunContinuation: vi.fn(async () => {}),
  };
  let api!: ReturnType<typeof useADKRunActions>;
  mount(
    defineComponent({
      setup() {
        api = useADKRunActions(input);
        return () => h("div");
      },
    }),
  );
  return { api, input };
}

function buildApproval(id: string): ADKApproval {
  return {
    id,
    runId: "run-1",
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { prompt: "save" },
    status: "PENDING",
    reason: "Needs review",
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}

function approvalWithoutId(): ADKApproval {
  return {
    ...buildApproval("ignored"),
    id: undefined as unknown as string,
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "RUNNING",
    message: "running",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function buildInputRequest(): ADKInputRequest {
  return {
    id: "input-1",
    runId: "run-1",
    agentId: "agent-1",
    functionCallId: "call-1",
    status: "PENDING_INPUT",
    questions: [
      {
        id: "question-1",
        question: "Continue?",
        options: [],
        allowOther: false,
      },
    ],
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}
