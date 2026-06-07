package jftradeapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func writeADKListOrError[T any](s *Server, c *gin.Context, code string, key string, items []T, err error) {
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, code, err.Error())
		return
	}
	s.writeOK(c, map[string]any{key: items})
}

func writeADKPagedListOrError[T any](s *Server, c *gin.Context, code string, key string, items []T, err error) {
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, code, err.Error())
		return
	}
	var query adkPageQuery
	_ = c.ShouldBindQuery(&query)
	limit, offset := adkPageBounds(query)
	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	s.writeOK(c, map[string]any{
		key:    items[offset:end],
		"page": map[string]any{"limit": limit, "offset": offset, "total": total, "returned": end - offset, "hasMore": end < total},
	})
}

func writeADKPageOrError[T any](s *Server, c *gin.Context, code string, key string, items []T, total int, limit int, offset int, err error) {
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, code, err.Error())
		return
	}
	if offset > total {
		offset = total
	}
	returned := len(items)
	s.writeOK(c, map[string]any{
		key: items,
		"page": map[string]any{
			"limit": limit, "offset": offset, "total": total, "returned": returned, "hasMore": offset+returned < total,
		},
	})
}

func adkPageBounds(query adkPageQuery) (int, int) {
	return normalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 100, 500)
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
