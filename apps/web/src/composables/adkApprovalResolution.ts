import type { ADKApproval, ADKApprovalResolution } from "@/contracts";

export type ADKApprovalAction = "approve" | "deny";

interface ApprovalBatchOptions {
  approvals: ADKApproval[];
  action: ADKApprovalAction;
  submit: (
    approval: ADKApproval,
    action: ADKApprovalAction,
  ) => Promise<ADKApprovalResolution>;
  onResolution?: (resolution: ADKApprovalResolution) => void | Promise<void>;
}

interface ApprovalBatchResult {
  resolutions: ADKApprovalResolution[];
  errors: string[];
}

const inFlightApprovalIds = new Set<string>();

export function uniqueADKApprovalsById(
  approvals: ADKApproval[],
): ADKApproval[] {
  const seen = new Set<string>();
  const unique: ADKApproval[] = [];
  for (const approval of approvals) {
    const id = approvalId(approval);
    if (id !== "") {
      if (seen.has(id)) {
        continue;
      }
      seen.add(id);
    }
    unique.push(approval);
  }
  return unique;
}

export async function resolveADKApprovalBatchOnce(
  options: ApprovalBatchOptions,
): Promise<ApprovalBatchResult> {
  const claimed = claimApprovals(options.approvals);
  const resolutions: ADKApprovalResolution[] = [];
  const errors: string[] = [];
  try {
    for (const approval of claimed.approvals) {
      try {
        const resolution = await options.submit(approval, options.action);
        resolutions.push(resolution);
        await options.onResolution?.(resolution);
      } catch (error) {
        errors.push(error instanceof Error ? error.message : "审批处理失败");
      }
    }
    return { resolutions, errors };
  } finally {
    claimed.release();
  }
}

export function resetADKApprovalInFlightForTest(): void {
  inFlightApprovalIds.clear();
}

function claimApprovals(approvals: ADKApproval[]): {
  approvals: ADKApproval[];
  release: () => void;
} {
  const claimedIds: string[] = [];
  const claimedApprovals: ADKApproval[] = [];
  for (const approval of uniqueADKApprovalsById(approvals)) {
    const id = approvalId(approval);
    if (id !== "") {
      if (inFlightApprovalIds.has(id)) {
        continue;
      }
      inFlightApprovalIds.add(id);
      claimedIds.push(id);
    }
    claimedApprovals.push(approval);
  }
  return {
    approvals: claimedApprovals,
    release: () => {
      for (const id of claimedIds) {
        inFlightApprovalIds.delete(id);
      }
    },
  };
}

function approvalId(approval: ADKApproval): string {
  return String(approval.id ?? "").trim();
}
