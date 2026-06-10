package adk

import "strings"

func pendingApprovalsOnly(approvals []Approval) []Approval {
	if len(approvals) == 0 {
		return []Approval{}
	}
	filtered := make([]Approval, 0, len(approvals))
	for _, approval := range approvals {
		if isPendingApprovalStatus(approval.Status) {
			filtered = append(filtered, approval)
		}
	}
	return filtered
}

func isPendingApprovalStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), ApprovalStatusPending)
}
