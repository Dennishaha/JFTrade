package adk

import jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"

func PendingApprovalsOnly(approvals []Approval) []Approval {
	return jfadkmodel.PendingApprovalsOnly(approvals)
}
