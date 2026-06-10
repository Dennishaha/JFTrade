// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import type { ADKRun } from "@/contracts";

import { monitorADKRunContinuation } from "../src/composables/adkRunContinuation";

afterEach(() => {
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
});

function buildRun(
  status = "RUNNING",
  toolCalls: ADKRun["toolCalls"] = [
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
  ],
  failureReason = "",
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
