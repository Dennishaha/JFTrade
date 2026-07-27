package servercore

import (
	"context"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
)

func (s *serverApplication) workflowWatchedInstruments() []string {
	if s == nil {
		return nil
	}
	assistantRuntime := s.runtimes.Assistant()
	if assistantRuntime == nil {
		return nil
	}
	return assistantRuntime.WatchedWorkflowInstruments(context.Background())
}

func (s *serverApplication) emitWorkflowEvent(event assistantassembly.WorkflowEvent) {
	if s == nil {
		return
	}
	assistantRuntime := s.runtimes.Assistant()
	if assistantRuntime == nil {
		return
	}
	go assistantRuntime.HandleWorkflowEvent(context.Background(), event)
}
