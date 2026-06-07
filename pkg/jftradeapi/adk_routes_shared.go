package jftradeapi

import (
	"net/http"
	"strings"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func writeADKListOrError[T any](s *Server, w http.ResponseWriter, code string, key string, items []T, err error) {
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, code, err.Error())
		return
	}
	s.writeOK(w, map[string]any{key: items})
}

func writeADKPagedListOrError[T any](s *Server, w http.ResponseWriter, code string, key string, items []T, err error, r *http.Request) {
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, code, err.Error())
		return
	}
	limit, offset := adkPageBounds(r)
	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	s.writeOK(w, map[string]any{
		key:    items[offset:end],
		"page": map[string]any{"limit": limit, "offset": offset, "total": total, "returned": end - offset, "hasMore": end < total},
	})
}

func writeADKPageOrError[T any](s *Server, w http.ResponseWriter, code string, key string, items []T, total int, limit int, offset int, err error) {
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, code, err.Error())
		return
	}
	if offset > total {
		offset = total
	}
	returned := len(items)
	s.writeOK(w, map[string]any{
		key: items,
		"page": map[string]any{
			"limit": limit, "offset": offset, "total": total, "returned": returned, "hasMore": offset+returned < total,
		},
	})
}

func adkPageBounds(r *http.Request) (int, int) {
	limit := queryIntDefault(r, "limit", 100)
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	offset := queryIntDefault(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func filterADKAgents(items []jfadk.Agent, status string) []jfadk.Agent {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return items
	}
	out := make([]jfadk.Agent, 0, len(items))
	for _, item := range items {
		if item.Status == status {
			out = append(out, item)
		}
	}
	return out
}

func filterADKAudit(items []jfadk.AuditEvent, kind string, subjectID string) []jfadk.AuditEvent {
	kind = strings.TrimSpace(kind)
	subjectID = strings.TrimSpace(subjectID)
	out := make([]jfadk.AuditEvent, 0, len(items))
	for _, item := range items {
		if kind != "" && item.Kind != kind {
			continue
		}
		if subjectID != "" && item.SubjectID != subjectID {
			continue
		}
		out = append(out, item)
	}
	return out
}
