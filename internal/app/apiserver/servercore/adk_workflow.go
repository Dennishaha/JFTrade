package servercore

import (
	"context"
	"fmt"
	"strings"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func (s *Server) workflowMarketSnapshot(ctx context.Context, instrumentID string) (map[string]any, error) {
	if s == nil || s.marketdataSvc == nil {
		return nil, fmt.Errorf("market data service is unavailable")
	}
	market, symbol, ok := splitWorkflowInstrumentID(instrumentID)
	if !ok {
		return nil, fmt.Errorf("invalid instrumentId %q", instrumentID)
	}
	snapshot, err := s.marketdataSvc.GetSnapshot(ctx, market, symbol, false)
	if err != nil {
		return nil, err
	}
	return map[string]any(snapshot), nil
}

func (s *Server) workflowWatchedInstruments() []string {
	if s == nil || s.assistantSvc == nil {
		return nil
	}
	return s.assistantSvc.WatchedWorkflowInstruments(context.Background())
}

func (s *Server) emitWorkflowEvent(event jfadk.WorkflowEvent) {
	if s == nil || s.assistantSvc == nil {
		return
	}
	go s.assistantSvc.HandleWorkflowEvent(context.Background(), event)
}

func splitWorkflowInstrumentID(instrumentID string) (string, string, bool) {
	parts := strings.SplitN(strings.ToUpper(strings.TrimSpace(instrumentID)), ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
