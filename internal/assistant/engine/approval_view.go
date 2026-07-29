package adk

import "strings"

func pendingApprovalsOnly(approvals []Approval) []Approval {
	if len(approvals) == 0 {
		return []Approval{}
	}
	filtered := make([]Approval, 0, len(approvals))
	seen := map[string]struct{}{}
	for _, approval := range approvals {
		if isPendingApprovalStatus(approval.Status) {
			if key := pendingApprovalKey(approval); key != "" {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}
			filtered = append(filtered, approval)
		}
	}
	return filtered
}

func pendingApprovalKey(approval Approval) string {
	if id := strings.TrimSpace(approval.ID); id != "" {
		return "id:" + id
	}
	if id := strings.TrimSpace(approval.ConfirmationCallID); id != "" {
		return "confirmation:" + id
	}
	if id := strings.TrimSpace(approval.FunctionCallID); id != "" {
		return "function:" + id
	}
	return ""
}

func isPendingApprovalStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), ApprovalStatusPending)
}
