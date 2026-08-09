package model

import (
	"strings"
	"sync"
)

// WorkflowGoalDecision is the in-memory decision state for a loop-mode goal
// workflow turn. It is shared between the goal orchestrator toolset and the
// workflow runtime.
type WorkflowGoalDecision struct {
	mu      sync.Mutex
	phase   string
	status  string
	summary string
	reason  string
}

// WorkflowGoalDecisionSnapshot is an immutable copy of the decision state.
type WorkflowGoalDecisionSnapshot struct {
	Status  string
	Summary string
	Reason  string
}

// Reset returns the decision to the working phase.
func (d *WorkflowGoalDecision) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = "work"
	d.status = ""
	d.summary = ""
	d.reason = ""
}

// BeginDecision moves the decision into the decision phase.
func (d *WorkflowGoalDecision) BeginDecision() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = "decision"
	d.status = ""
	d.summary = ""
	d.reason = ""
}

// DecisionPhase reports whether the decision phase is active.
func (d *WorkflowGoalDecision) DecisionPhase() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.phase == "decision"
}

// SetComplete records a completed goal decision with a summary.
func (d *WorkflowGoalDecision) SetComplete(summary string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = "complete"
	d.summary = strings.TrimSpace(summary)
	d.reason = ""
}

// SetContinue records a continue decision with a reason.
func (d *WorkflowGoalDecision) SetContinue(reason string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = "continue"
	d.summary = ""
	d.reason = strings.TrimSpace(reason)
}

// Snapshot returns an immutable copy of the decision state.
func (d *WorkflowGoalDecision) Snapshot() WorkflowGoalDecisionSnapshot {
	if d == nil {
		return WorkflowGoalDecisionSnapshot{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return WorkflowGoalDecisionSnapshot{Status: d.status, Summary: d.summary, Reason: d.reason}
}
