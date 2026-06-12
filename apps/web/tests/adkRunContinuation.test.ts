// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import type { ADKRun } from "@/contracts";

import { monitorADKRunContinuation } from "../src/composables/adkRunContinuation";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("monitorADKRunContinuation", () => {
  it("refetches once after timeout and returns a failed tool snapshot", async () => {
    const failedSnapshot = buildRun("RUNNING", [
      {
        id: "tool-1",
        runId: "run-1",
        toolName: "strategy.save_draft",
        permission: "write_strategy",
        status: "FAILED",
        error: "disk full",
        requiresUser: false,
        createdAt: "2026-06-10T00:00:00Z",
        updatedAt: "2026-06-10T00:00:01Z",
      },
    ]);

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createEnvelopeResponse(failedSnapshot)),
    );

    const onProgress = vi.fn();
    const latestRun = await monitorADKRunContinuation(buildRun(), {
      timeoutMs: 0,
      onProgress,
    });

    expect(latestRun?.toolCalls[0]?.status).toBe("FAILED");
    expect(onProgress).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("runs terminal callback for the final refetch when the run has already failed", async () => {
    const failedRun = buildRun(
      "FAILED",
      [
        {
          id: "tool-1",
          runId: "run-1",
          toolName: "portfolio.summary",
          permission: "read",
          status: "FAILED",
          error: "backend unavailable",
          requiresUser: false,
          createdAt: "2026-06-10T00:00:00Z",
          updatedAt: "2026-06-10T00:00:01Z",
        },
      ],
      "backend unavailable",
    );

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createEnvelopeResponse(failedRun)),
    );

    const onTerminal = vi.fn();
    const latestRun = await monitorADKRunContinuation(buildRun(), {
      timeoutMs: 0,
      onTerminal,
    });

    expect(latestRun?.status).toBe("FAILED");
    expect(onTerminal).toHaveBeenCalledWith(failedRun);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("continues past the old 15s window and returns when a new approval is pending", async () => {
    vi.useFakeTimers();
    const initialRun = buildRun("RUNNING", defaultToolCalls(), "", {
      maxDurationMs: 16_000,
    });
    const pendingApproval = buildApproval("approval-2");
    const pendingRun = buildRun(
      "PENDING_APPROVAL",
      [
        {
          ...defaultToolCalls()[0]!,
          status: "PENDING_APPROVAL",
          requiresUser: true,
          updatedAt: "2026-06-10T00:00:16Z",
        },
      ],
      "",
      {
        maxDurationMs: 16_000,
        pendingApprovals: [pendingApproval],
        resumeState: "waiting_approval",
        updatedAt: "2026-06-10T00:00:16Z",
      },
    );
    let calls = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        calls += 1;
        return createEnvelopeResponse(calls >= 16 ? pendingRun : initialRun);
      }),
    );

    const onProgress = vi.fn();
    const onTerminal = vi.fn();
    const promise = monitorADKRunContinuation(initialRun, {
      pollIntervalMs: 1_000,
      onProgress,
      onTerminal,
    });

    await vi.advanceTimersByTimeAsync(16_000);
    const latestRun = await promise;

    expect(latestRun?.status).toBe("PENDING_APPROVAL");
    expect(latestRun?.pendingApprovals).toEqual([pendingApproval]);
    expect(onProgress).toHaveBeenCalledWith(pendingRun, initialRun);
    expect(onTerminal).not.toHaveBeenCalled();
    expect(fetch).toHaveBeenCalledTimes(16);
  });

  it("treats pendingApprovals changes as progress and stops polling immediately", async () => {
    vi.useFakeTimers();
    const initialRun = buildRun("RUNNING");
    const pendingApproval = buildApproval("approval-running");
    const pendingRun = buildRun("RUNNING", defaultToolCalls(), "", {
      pendingApprovals: [pendingApproval],
      resumeState: "waiting_approval",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createEnvelopeResponse(pendingRun)),
    );

    const onProgress = vi.fn();
    const onTerminal = vi.fn();
    const promise = monitorADKRunContinuation(initialRun, {
      pollIntervalMs: 10,
      timeoutMs: 1_000,
      onProgress,
      onTerminal,
    });

    await vi.advanceTimersByTimeAsync(10);
    const latestRun = await promise;

    expect(latestRun).toEqual(pendingRun);
    expect(onProgress).toHaveBeenCalledWith(pendingRun, initialRun);
    expect(onTerminal).not.toHaveBeenCalled();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});

function buildRun(
  status = "RUNNING",
  toolCalls: ADKRun["toolCalls"] = defaultToolCalls(),
  failureReason = "",
  overrides: Partial<ADKRun> = {},
): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status,
    message: failureReason,
    failureReason,
    toolCalls,
    pendingApprovals: [],
    createdAt: "2026-06-10T00:00:00Z",
    updatedAt: "2026-06-10T00:00:00Z",
    ...overrides,
  };
}

function defaultToolCalls(): ADKRun["toolCalls"] {
  return [
    {
      id: "tool-1",
      runId: "run-1",
      toolName: "portfolio.summary",
      permission: "read",
      status: "RUNNING",
      requiresUser: false,
      createdAt: "2026-06-10T00:00:00Z",
      updatedAt: "2026-06-10T00:00:00Z",
    },
  ];
}

function buildApproval(id: string): ADKRun["pendingApprovals"][number] {
  return {
    id,
    runId: "run-1",
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { name: "draft" },
    status: "PENDING",
    reason: "needs approval",
    createdAt: "2026-06-10T00:00:16Z",
    updatedAt: "2026-06-10T00:00:16Z",
  };
}

function createEnvelopeResponse(run: ADKRun): Response {
  return new Response(
    JSON.stringify({
      ok: true,
      data: run,
    }),
    {
      headers: {
        "Content-Type": "application/json",
      },
    },
  );
}
