package servercore

import (
	"context"
	"log"
	"strings"
	"time"
)

func (s *strategyCatalogStore) strategyLogs(instanceID string) (strategyLogsResponse, bool) {
	return s.strategyLogsPage(instanceID, strategyRuntimeLogQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyLogsPage(instanceID string, query strategyRuntimeLogQuery) (strategyLogsResponse, bool) {
	s.mu.RLock()
	var normalized managedStrategyInstance
	var found bool
	for _, strategy := range s.data.Strategies {
		normalized = s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			found = true
			break
		}
	}
	s.mu.RUnlock()
	if !found {
		return strategyLogsResponse{}, false
	}
	if s.runtimeStore == nil {
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountLogs(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListLogs(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy log count degraded: %v", countErr)
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy log query degraded: %v", listErr)
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	logs := make([]string, 0, len(persisted))
	for _, event := range persisted {
		logs = append(logs, event.Raw)
	}
	return strategyLogsResponse{InstanceID: instanceID, Logs: logs, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(logs), HasMore: offset+len(logs) < total}}, true
}

func (s *strategyCatalogStore) strategyAudit(instanceID string) (strategyAuditResponse, bool) {
	return s.strategyAuditPage(instanceID, strategyRuntimeAuditQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyAuditPage(instanceID string, query strategyRuntimeAuditQuery) (strategyAuditResponse, bool) {
	s.mu.RLock()
	var normalized managedStrategyInstance
	var found bool
	for _, strategy := range s.data.Strategies {
		normalized = s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			found = true
			break
		}
	}
	s.mu.RUnlock()
	if !found {
		return strategyAuditResponse{}, false
	}
	if s.runtimeStore == nil {
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountAudit(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListAudit(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy audit count degraded: %v", countErr)
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy audit query degraded: %v", listErr)
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	entries := make([]strategyAuditEntry, 0, len(persisted))
	for _, event := range persisted {
		entries = append(entries, strategyAuditEntry{InstanceID: event.InstanceID, Kind: event.Kind, Detail: event.Detail, At: event.At.UTC().Format(time.RFC3339Nano)})
	}
	return strategyAuditResponse{InstanceID: instanceID, Entries: entries, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(entries), HasMore: offset+len(entries) < total}}, true
}

func (s *strategyCatalogStore) recordStrategyEventsLocked(strategy *managedStrategyInstance, at time.Time, logMessage string, logLevel string, logSource string, kind string, detail string) {
	rawLog := buildStrategyRuntimeLogEntry(at, logMessage)
	if rawLog != "" {
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendLog(context.Background(), strategyRuntimeLogEvent{
				InstanceID: strategy.ID,
				At:         at,
				Raw:        rawLog,
				Level:      strings.ToLower(strings.TrimSpace(logLevel)),
				Source:     strings.ToLower(strings.TrimSpace(logSource)),
			}); err != nil {
				log.Printf("JFTrade persist strategy runtime log degraded: %v", err)
			}
		}
	}

	kind = strings.TrimSpace(kind)
	if kind != "" {
		auditEntry := strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       kind,
			Detail:     strings.TrimSpace(detail),
			At:         at.UTC().Format(time.RFC3339Nano),
		}
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendAudit(context.Background(), strategyRuntimeAuditEvent{
				InstanceID: strategy.ID,
				Kind:       auditEntry.Kind,
				Detail:     auditEntry.Detail,
				At:         at,
			}); err != nil {
				log.Printf("JFTrade persist strategy runtime audit degraded: %v", err)
			}
		}
	}

}
