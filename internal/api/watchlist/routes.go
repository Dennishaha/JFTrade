package watchlist

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

type groupURI struct {
	GroupID string `uri:"groupId" binding:"required"`
}

type instrumentURI struct {
	Market string `uri:"market" binding:"required"`
	Symbol string `uri:"symbol" binding:"required"`
}

type sourceURI struct {
	SourceID string `uri:"sourceId" binding:"required"`
}

type previewURI struct {
	PreviewID string `uri:"previewId" binding:"required"`
}

type itemPageQuery struct {
	GroupID string                      `form:"groupId"`
	Cursor  string                      `form:"cursor"`
	Limit   httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Query   string                      `form:"query"`
	Market  string                      `form:"market"`
}

type importRunPageQuery struct {
	SourceID string                      `form:"sourceId"`
	Cursor   string                      `form:"cursor"`
	Limit    httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
}

type bindingQuery struct {
	SourceID  string `form:"sourceId"`
	BindingID string `form:"bindingId"`
}

// RegisterRoutes registers the JFTrade-owned watchlist API under /api/v1/watchlist.
func RegisterRoutes(api *gin.RouterGroup, service *domain.Service) {
	referenceOpenAPIDocumentation()
	if service == nil {
		service = domain.NewService(nil)
	}
	routes := api.Group("/watchlist")
	registerGroupRoutes(routes, service)
	registerItemRoutes(routes, service)
	registerQuoteRoute(routes, service)
	registerSourceRoutes(routes, service)
	registerImportRoutes(routes, service)
}

func registerGroupRoutes(routes *gin.RouterGroup, service *domain.Service) {
	routes.GET("/groups", func(c *gin.Context) {
		groups, err := service.ListGroups(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"groups": groups})
	})
	routes.POST("/groups", func(c *gin.Context) {
		var input domain.CreateGroupInput
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist group payload")
			return
		}
		group, err := service.CreateGroup(c.Request.Context(), input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, group)
	})
	routes.PATCH("/groups/:groupId", func(c *gin.Context) {
		var uri groupURI
		if !bindURI(c, &uri, "invalid watchlist group id") {
			return
		}
		var input domain.UpdateGroupInput
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist group payload")
			return
		}
		group, err := service.UpdateGroup(c.Request.Context(), uri.GroupID, input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, group)
	})
	routes.DELETE("/groups/:groupId", func(c *gin.Context) {
		var uri groupURI
		if !bindURI(c, &uri, "invalid watchlist group id") {
			return
		}
		if err := service.DeleteGroup(c.Request.Context(), uri.GroupID); err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"deleted": true})
	})
}

func registerItemRoutes(routes *gin.RouterGroup, service *domain.Service) {
	routes.GET("/items", func(c *gin.Context) {
		var query itemPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist items query")
			return
		}
		limit, err := boundLimit(query.Limit)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		page, err := service.ListItems(c.Request.Context(), domain.ListItemsOptions{
			GroupID: query.GroupID, Cursor: query.Cursor, Limit: limit, Query: query.Query, Market: query.Market,
		})
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, page)
	})
	routes.GET("/instruments/:market/:symbol/memberships", func(c *gin.Context) {
		var uri instrumentURI
		if !bindURI(c, &uri, "invalid watchlist instrument") {
			return
		}
		memberships, err := service.GetMemberships(c.Request.Context(), pathInstrumentID(uri))
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, memberships)
	})
	routes.PUT("/instruments/:market/:symbol/memberships", func(c *gin.Context) {
		var uri instrumentURI
		if !bindURI(c, &uri, "invalid watchlist instrument") {
			return
		}
		var input domain.ReplaceMembershipsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist memberships payload")
			return
		}
		input.InstrumentID = pathInstrumentID(uri)
		memberships, err := service.ReplaceMemberships(c.Request.Context(), input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, memberships)
	})
}

func registerQuoteRoute(routes *gin.RouterGroup, service *domain.Service) {
	routes.POST("/quotes/batch", func(c *gin.Context) {
		var input struct {
			InstrumentIDs []string `json:"instrumentIds" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist quote payload")
			return
		}
		quotes, err := service.BatchQuotes(c.Request.Context(), input.InstrumentIDs)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, quotes)
	})
}

func registerSourceRoutes(routes *gin.RouterGroup, service *domain.Service) {
	routes.GET("/sources", func(c *gin.Context) {
		sources, err := service.ListSources(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"sources": sources})
	})
	routes.GET("/sources/:sourceId/groups", func(c *gin.Context) {
		var uri sourceURI
		if !bindURI(c, &uri, "invalid watchlist source id") {
			return
		}
		groups, err := service.ListSourceGroups(c.Request.Context(), uri.SourceID)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"groups": groups})
	})
	routes.GET("/bindings", func(c *gin.Context) {
		var query bindingQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist binding query")
			return
		}
		bindings, err := service.ListBindings(c.Request.Context(), query.SourceID)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"bindings": bindings})
	})
	routes.DELETE("/bindings", func(c *gin.Context) {
		var query bindingQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist binding query")
			return
		}
		deleteBinding(c, service, query.BindingID)
	})
}

func registerImportRoutes(routes *gin.RouterGroup, service *domain.Service) {
	routes.POST("/imports/preview", func(c *gin.Context) {
		var input domain.ImportPreviewRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist import preview payload")
			return
		}
		preview, err := service.PreviewImport(c.Request.Context(), input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, preview)
	})
	routes.POST("/imports/:previewId/commit", func(c *gin.Context) {
		var uri previewURI
		if !bindURI(c, &uri, "invalid watchlist preview id") {
			return
		}
		var input domain.CommitImportInput
		if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist import commit payload")
			return
		}
		input.PreviewID = uri.PreviewID
		run, err := service.CommitImport(c.Request.Context(), input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, run)
	})
	routes.GET("/import-runs", func(c *gin.Context) {
		var query importRunPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid watchlist import runs query")
			return
		}
		limit, err := boundLimit(query.Limit)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		page, err := service.ListImportRuns(c.Request.Context(), query.SourceID, query.Cursor, limit)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, page)
	})
}

func pathInstrumentID(uri instrumentURI) string {
	return strings.TrimSpace(uri.Market) + "." + strings.TrimSpace(uri.Symbol)
}

func boundLimit(value httpserver.OptionalIntValue) (int, error) {
	if value.Set && (!value.Valid || value.Int() < 1) {
		return 0, errors.New("limit must be a positive integer")
	}
	limit, _ := httpserver.NormalizeBoundPage(value.Int(), 0, domain.DefaultPageLimit, domain.MaxPageLimit)
	return limit, nil
}

func bindURI(c *gin.Context, target any, message string) bool {
	if err := httpserver.BindURI(c, target); err != nil {
		httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", message)
		return false
	}
	return true
}

func deleteBinding(c *gin.Context, service *domain.Service, bindingID string) {
	if err := service.DeleteBinding(c.Request.Context(), bindingID); err != nil {
		writeError(c, err)
		return
	}
	httpserver.WriteOK(c, map[string]any{"deleted": true})
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		httpserver.WriteError(c, http.StatusServiceUnavailable, "WATCHLIST_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpserver.WriteError(c, http.StatusNotFound, "WATCHLIST_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrValidation):
		httpserver.WriteError(c, http.StatusBadRequest, "WATCHLIST_INVALID", err.Error())
	case errors.Is(err, domain.ErrAmbiguousRemoteGroup):
		httpserver.WriteError(c, http.StatusConflict, "WATCHLIST_REMOTE_GROUP_AMBIGUOUS", err.Error())
	case errors.Is(err, domain.ErrProtectedGroup):
		httpserver.WriteError(c, http.StatusConflict, "WATCHLIST_GROUP_PROTECTED", err.Error())
	case errors.Is(err, domain.ErrPreviewExpired):
		httpserver.WriteError(c, http.StatusConflict, "WATCHLIST_PREVIEW_EXPIRED", err.Error())
	case errors.Is(err, domain.ErrStalePreview):
		httpserver.WriteError(c, http.StatusConflict, "WATCHLIST_PREVIEW_STALE", err.Error())
	case errors.Is(err, domain.ErrConflict):
		httpserver.WriteError(c, http.StatusConflict, "WATCHLIST_CONFLICT", err.Error())
	default:
		httpserver.WriteError(c, http.StatusInternalServerError, "WATCHLIST_FAILED", err.Error())
	}
}
