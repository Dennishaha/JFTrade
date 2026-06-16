package assistant

import "github.com/jftrade/jftrade-main/internal/api/httpserver"

type servicePage[T any] struct {
	Items  []T
	Total  int
	Limit  int
	Offset int
}

func pageEnvelope(limit int, offset int, total int, returned int) map[string]any {
	return map[string]any{
		"limit": limit, "offset": offset, "total": total,
		"returned": returned, "hasMore": offset+returned < total,
	}
}

func adkPageBounds(query adkPageQuery) (int, int) {
	return httpserver.NormalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 100, 500)
}

func defaultString(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
