package catalog

import (
	"context"
	"log"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

func (s *Service) GetLogs(instanceID string, query stratsrv.LogQuery) (stratsrv.LogsResult, bool) {
	if _, ok := s.GetInstance(instanceID); !ok {
		return stratsrv.LogsResult{}, false
	}
	activityQuery := runtimeactivity.NormalizeLogQuery(runtimeactivity.LogQuery{
		InstanceID: instanceID,
		Limit:      query.Limit,
		Offset:     query.Offset,
		Level:      query.Level,
		FromAt:     query.FromAt,
		ToAt:       query.ToAt,
	})
	result := stratsrv.LogsResult{
		InstanceID: instanceID,
		Logs:       []string{},
		Page: stratsrv.ActivityPage{
			Limit:  activityQuery.Limit,
			Offset: activityQuery.Offset,
		},
	}
	if s.activity == nil {
		return result, true
	}
	total, err := s.activity.CountLogs(context.Background(), activityQuery)
	if err != nil {
		log.Printf("JFTrade strategy log count degraded: %v", err)
		return result, true
	}
	result.Page.Total = total
	persisted, err := s.activity.ListLogs(context.Background(), activityQuery)
	if err != nil {
		log.Printf("JFTrade strategy log query degraded: %v", err)
		return result, true
	}
	result.Logs = make([]string, 0, len(persisted))
	for _, event := range persisted {
		result.Logs = append(result.Logs, event.Raw)
	}
	result.Page.Returned = len(result.Logs)
	result.Page.HasMore = result.Page.Offset+result.Page.Returned < result.Page.Total
	return result, true
}

func (s *Service) GetAudit(instanceID string, query stratsrv.AuditQuery) (stratsrv.AuditResult, bool) {
	if _, ok := s.GetInstance(instanceID); !ok {
		return stratsrv.AuditResult{}, false
	}
	activityQuery := runtimeactivity.NormalizeAuditQuery(runtimeactivity.AuditQuery{
		InstanceID: instanceID,
		Limit:      query.Limit,
		Offset:     query.Offset,
		Kind:       query.Kind,
		FromAt:     query.FromAt,
		ToAt:       query.ToAt,
	})
	result := stratsrv.AuditResult{
		InstanceID: instanceID,
		Entries:    []stratsrv.AuditEntry{},
		Page: stratsrv.ActivityPage{
			Limit:  activityQuery.Limit,
			Offset: activityQuery.Offset,
		},
	}
	if s.activity == nil {
		return result, true
	}
	total, err := s.activity.CountAudit(context.Background(), activityQuery)
	if err != nil {
		log.Printf("JFTrade strategy audit count degraded: %v", err)
		return result, true
	}
	result.Page.Total = total
	persisted, err := s.activity.ListAudit(context.Background(), activityQuery)
	if err != nil {
		log.Printf("JFTrade strategy audit query degraded: %v", err)
		return result, true
	}
	result.Entries = make([]stratsrv.AuditEntry, 0, len(persisted))
	for _, event := range persisted {
		result.Entries = append(result.Entries, stratsrv.AuditEntry{
			InstanceID: event.InstanceID,
			Kind:       event.Kind,
			Detail:     event.Detail,
			At:         event.At.UTC().Format(time.RFC3339Nano),
		})
	}
	result.Page.Returned = len(result.Entries)
	result.Page.HasMore = result.Page.Offset+result.Page.Returned < result.Page.Total
	return result, true
}

func (s *Service) recordEventsLocked(
	instance *stratsrv.ManagedInstance,
	at time.Time,
	message, level, source, kind, detail string,
) {
	if instance == nil || s.activity == nil {
		return
	}
	if raw := buildRuntimeLogEntry(at, message); raw != "" {
		if err := s.activity.AppendLog(context.Background(), runtimeactivity.LogEvent{
			InstanceID: instance.ID,
			At:         at,
			Raw:        raw,
			Level:      strings.ToLower(strings.TrimSpace(level)),
			Source:     strings.ToLower(strings.TrimSpace(source)),
		}); err != nil {
			log.Printf("JFTrade persist strategy runtime log degraded: %v", err)
		}
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return
	}
	if err := s.activity.AppendAudit(context.Background(), runtimeactivity.AuditEvent{
		InstanceID: instance.ID,
		Kind:       kind,
		Detail:     strings.TrimSpace(detail),
		At:         at,
	}); err != nil {
		log.Printf("JFTrade persist strategy runtime audit degraded: %v", err)
	}
}
