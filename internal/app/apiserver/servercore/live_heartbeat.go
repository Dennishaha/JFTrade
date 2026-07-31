package servercore

import (
	"context"
	"sort"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
)

func (s *serverApplication) liveHeartbeatEvent(heartbeatInterval time.Duration, clients apilive.ClientStats, webSocketInstrumentIDs []string) map[string]any {
	return marketdataapp.LiveHeartbeat(
		s.marketdataSvc,
		marketdataapp.LiveClientStats{
			Connected: clients.Connected,
			Limit:     clients.Limit,
			AtLimit:   clients.AtLimit,
		},
		s.activeLiveStreamInstrumentIDs(webSocketInstrumentIDs),
		heartbeatInterval,
		liveHeartbeatStaleThreshold,
		tickCacheRetention,
	)
}

func (s *serverApplication) activeMarketInstrumentIDs() []string {
	if s.marketdataSvc == nil {
		return nil
	}
	instrumentIDs, err := s.marketdataSvc.GetActiveInstruments(context.Background())
	if err != nil {
		return nil
	}
	return instrumentIDs
}

func (s *serverApplication) activeLiveStreamInstrumentIDs(webSocketInstrumentIDs []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, instrumentID := range s.activeMarketInstrumentIDs() {
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		result = append(result, instrumentID)
	}
	if s.strategySvc != nil {
		for _, instrumentID := range s.strategySvc.ActiveInstrumentIDs() {
			if _, exists := seen[instrumentID]; exists {
				continue
			}
			seen[instrumentID] = struct{}{}
			result = append(result, instrumentID)
		}
	}
	if liveWebSocket := s.runtimes.LiveWebSocket(); webSocketInstrumentIDs == nil && liveWebSocket != nil {
		webSocketInstrumentIDs = liveWebSocket.ActiveInstrumentIDs()
	}
	for _, instrumentID := range webSocketInstrumentIDs {
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		result = append(result, instrumentID)
	}
	sort.Strings(result)
	return result
}
