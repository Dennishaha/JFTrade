package servercore

import (
	"context"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"log"
	"strings"
	"time"
)

func (s *strategyCatalogStore) strategyLogs(instanceID string) (stratsrv.LogsResult, bool) {
	return s.strategyLogsPage(instanceID, runtimeactivity.LogQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyLogsPage(instanceID string, query runtimeactivity.LogQuery) (stratsrv.LogsResult, bool) {
	s.mu.RLock()
	var normalized stratsrv.ManagedInstance
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
		return stratsrv.LogsResult{}, false
	}
	if s.runtimeStore == nil {
		return stratsrv.LogsResult{InstanceID: instanceID, Logs: []string{}, Page: stratsrv.ActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountLogs(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListLogs(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy log count degraded: %v", countErr)
		return stratsrv.LogsResult{InstanceID: instanceID, Logs: []string{}, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy log query degraded: %v", listErr)
		return stratsrv.LogsResult{InstanceID: instanceID, Logs: []string{}, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	logs := make([]string, 0, len(persisted))
	for _, event := range persisted {
		logs = append(logs, event.Raw)
	}
	return stratsrv.LogsResult{InstanceID: instanceID, Logs: logs, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(logs), HasMore: offset+len(logs) < total}}, true
}

func (s *strategyCatalogStore) strategyAudit(instanceID string) (stratsrv.AuditResult, bool) {
	return s.strategyAuditPage(instanceID, runtimeactivity.AuditQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyAuditPage(instanceID string, query runtimeactivity.AuditQuery) (stratsrv.AuditResult, bool) {
	s.mu.RLock()
	var normalized stratsrv.ManagedInstance
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
		return stratsrv.AuditResult{}, false
	}
	if s.runtimeStore == nil {
		return stratsrv.AuditResult{InstanceID: instanceID, Entries: []stratsrv.AuditEntry{}, Page: stratsrv.ActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountAudit(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListAudit(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy audit count degraded: %v", countErr)
		return stratsrv.AuditResult{InstanceID: instanceID, Entries: []stratsrv.AuditEntry{}, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy audit query degraded: %v", listErr)
		return stratsrv.AuditResult{InstanceID: instanceID, Entries: []stratsrv.AuditEntry{}, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	entries := make([]stratsrv.AuditEntry, 0, len(persisted))
	for _, event := range persisted {
		entries = append(entries, stratsrv.AuditEntry{InstanceID: event.InstanceID, Kind: event.Kind, Detail: event.Detail, At: event.At.UTC().Format(time.RFC3339Nano)})
	}
	return stratsrv.AuditResult{InstanceID: instanceID, Entries: entries, Page: stratsrv.ActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(entries), HasMore: offset+len(entries) < total}}, true
}

func (s *strategyCatalogStore) recordStrategyEventsLocked(strategy *stratsrv.ManagedInstance, at time.Time, logMessage string, logLevel string, logSource string, kind string, detail string) {
	rawLog := buildStrategyRuntimeLogEntry(at, logMessage)
	if rawLog != "" {
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendLog(context.Background(), runtimeactivity.LogEvent{
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
		auditEntry := stratsrv.AuditEntry{
			InstanceID: strategy.ID,
			Kind:       kind,
			Detail:     strings.TrimSpace(detail),
			At:         at.UTC().Format(time.RFC3339Nano),
		}
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendAudit(context.Background(), runtimeactivity.AuditEvent{
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
