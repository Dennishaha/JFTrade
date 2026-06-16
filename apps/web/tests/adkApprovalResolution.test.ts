import { afterEach, describe, expect, it, vi } from "vitest";

import type { ADKApproval, ADKApprovalResolution } from "@/contracts";

import {
  resetADKApprovalInFlightForTest,
  resolveADKApprovalBatchOnce,
  uniqueADKApprovalsById,
} from "../src/composables/adkApprovalResolution";

afterEach(() => {
  resetADKApprovalInFlightForTest();
});

describe("adkApprovalResolution", () => {
  it("deduplicates approval ids before submission", () => {
    const approval = buildApproval("approval-1");

    expect(uniqueADKApprovalsById([approval, { ...approval }])).toEqual([
      approval,
    ]);
  });

  it("shares in-flight approval guards across callers", async () => {
    const approval = buildApproval("approval-shared");
    let finishFirstSubmit!: () => void;
    const firstSubmit = vi.fn(
      () =>
        new Promise<ADKApprovalResolution>((resolve) => {
          finishFirstSubmit = () =>
            resolve({
              approval: { ...approval, status: "APPROVED" },
              run: buildRun(),
            });
        }),
    );
    const secondSubmit = vi.fn(async () => ({
      approval: { ...approval, status: "APPROVED" },
      run: buildRun(),
    }));

    const first = resolveADKApprovalBatchOnce({
      approvals: [approval],
      action: "approve",
      submit: firstSubmit,
    });
    const second = await resolveADKApprovalBatchOnce({
      approvals: [approval],
      action: "approve",
      submit: secondSubmit,
    });

    expect(firstSubmit).toHaveBeenCalledTimes(1);
    expect(secondSubmit).not.toHaveBeenCalled();
    expect(second.resolutions).toEqual([]);

    finishFirstSubmit();
    await expect(first).resolves.toMatchObject({
      resolutions: [{ approval: { id: "approval-shared" } }],
      errors: [],
    });
  });
});

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

function buildRun(): ADKApprovalResolution["run"] {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "completed",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-09T00:00:00Z",
    updatedAt: "2026-06-09T00:00:00Z",
  };
}
