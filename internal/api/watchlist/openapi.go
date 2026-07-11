package watchlist

import (
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

// Stable public schema names used by generated OpenAPI clients.
type WatchlistGroup domain.Group
type WatchlistItem domain.Item
type WatchlistMemberships domain.Memberships
type WatchlistQuote domain.Quote
type WatchlistQuoteError domain.QuoteError
type WatchlistSource domain.Source
type WatchlistRemoteGroup domain.RemoteGroup
type WatchlistBinding domain.Binding
type WatchlistImportPreview domain.ImportPreview
type WatchlistImportRun domain.ImportRun

type WatchlistGroupsData struct {
	Groups []WatchlistGroup `json:"groups"`
}
type WatchlistItemsData struct {
	Items      []WatchlistItem `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}
type WatchlistQuotesData struct {
	Quotes     []WatchlistQuote      `json:"quotes"`
	Errors     []WatchlistQuoteError `json:"errors"`
	ObservedAt string                `json:"observedAt"`
}
type WatchlistSourcesData struct {
	Sources []WatchlistSource `json:"sources"`
}
type WatchlistRemoteGroupsData struct {
	Groups []WatchlistRemoteGroup `json:"groups"`
}
type WatchlistBindingsData struct {
	Bindings []WatchlistBinding `json:"bindings"`
}
type WatchlistImportRunsData struct {
	Items      []WatchlistImportRun `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
}
type WatchlistDeleteData struct {
	Deleted bool `json:"deleted"`
}
type WatchlistQuoteBatchRequest struct {
	InstrumentIDs []string `json:"instrumentIds"`
}

// referenceOpenAPIDocumentation keeps the swag-only endpoint declarations
// reachable without exporting documentation helpers as production API.
func referenceOpenAPIDocumentation() {
	_ = [...]func(){
		documentListGroups,
		documentCreateGroup,
		documentUpdateGroup,
		documentDeleteGroup,
		documentListItems,
		documentGetMemberships,
		documentReplaceMemberships,
		documentBatchQuotes,
		documentListSources,
		documentListSourceGroups,
		documentListBindings,
		documentDeleteBinding,
		documentPreviewImport,
		documentCommitImport,
		documentListImportRuns,
	}
}

// documentListGroups godoc
// @Summary 查询本地自选分组
// @Tags watchlist
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=WatchlistGroupsData}
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/groups [get]
func documentListGroups() { _ = httpserver.Envelope{} }

// documentCreateGroup godoc
// @Summary 创建本地自选分组
// @Tags watchlist
// @Accept json
// @Produce json
// @Param input body domain.CreateGroupInput true "分组"
// @Success 200 {object} httpserver.Envelope{data=WatchlistGroup}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/groups [post]
func documentCreateGroup() {}

// documentUpdateGroup godoc
// @Summary 修改本地自选分组
// @Tags watchlist
// @Accept json
// @Produce json
// @Param groupId path string true "分组 ID"
// @Param input body domain.UpdateGroupInput true "分组变更"
// @Success 200 {object} httpserver.Envelope{data=WatchlistGroup}
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/groups/{groupId} [patch]
func documentUpdateGroup() {}

// documentDeleteGroup godoc
// @Summary 删除本地自选分组
// @Tags watchlist
// @Produce json
// @Param groupId path string true "分组 ID"
// @Success 200 {object} httpserver.Envelope{data=WatchlistDeleteData}
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/groups/{groupId} [delete]
func documentDeleteGroup() {}

// documentListItems godoc
// @Summary 分页查询自选标的
// @Tags watchlist
// @Produce json
// @Param groupId query string false "分组 ID，留空表示全部"
// @Param cursor query string false "游标"
// @Param limit query int false "页大小"
// @Param query query string false "代码或名称"
// @Param market query string false "市场"
// @Success 200 {object} httpserver.Envelope{data=WatchlistItemsData}
// @Failure 400 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/items [get]
func documentListItems() {}

// documentGetMemberships godoc
// @Summary 查询标的自选分组归属
// @Tags watchlist
// @Produce json
// @Param market path string true "市场"
// @Param symbol path string true "代码"
// @Success 200 {object} httpserver.Envelope{data=WatchlistMemberships}
// @Failure 400 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/instruments/{market}/{symbol}/memberships [get]
func documentGetMemberships() {}

// documentReplaceMemberships godoc
// @Summary 原子替换标的多分组归属
// @Tags watchlist
// @Accept json
// @Produce json
// @Param market path string true "市场"
// @Param symbol path string true "代码"
// @Param input body domain.ReplaceMembershipsInput true "目标分组与 revision"
// @Success 200 {object} httpserver.Envelope{data=WatchlistMemberships}
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/instruments/{market}/{symbol}/memberships [put]
func documentReplaceMemberships() {}

// documentBatchQuotes godoc
// @Summary 批量读取自选行情快照
// @Tags watchlist
// @Accept json
// @Produce json
// @Param input body WatchlistQuoteBatchRequest true "可见标的"
// @Success 200 {object} httpserver.Envelope{data=WatchlistQuotesData}
// @Failure 400 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/quotes/batch [post]
func documentBatchQuotes() {}

// documentListSources godoc
// @Summary 查询可用自选导入来源
// @Tags watchlist
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=WatchlistSourcesData}
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/sources [get]
func documentListSources() {}

// documentListSourceGroups godoc
// @Summary 发现券商自选分组
// @Tags watchlist
// @Produce json
// @Param sourceId path string true "来源 ID"
// @Success 200 {object} httpserver.Envelope{data=WatchlistRemoteGroupsData}
// @Failure 404 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/sources/{sourceId}/groups [get]
func documentListSourceGroups() {}

// documentListBindings godoc
// @Summary 查询券商分组绑定
// @Tags watchlist
// @Produce json
// @Param sourceId query string false "来源 ID"
// @Success 200 {object} httpserver.Envelope{data=WatchlistBindingsData}
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/bindings [get]
func documentListBindings() {}

// documentDeleteBinding godoc
// @Summary 解除券商分组绑定
// @Tags watchlist
// @Produce json
// @Param bindingId query string true "绑定 ID"
// @Success 200 {object} httpserver.Envelope{data=WatchlistDeleteData}
// @Failure 404 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/bindings [delete]
func documentDeleteBinding() {}

// documentPreviewImport godoc
// @Summary 预览券商自选导入差异
// @Tags watchlist
// @Accept json
// @Produce json
// @Param input body domain.ImportPreviewRequest true "导入目标"
// @Success 200 {object} httpserver.Envelope{data=WatchlistImportPreview}
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/imports/preview [post]
func documentPreviewImport() {}

// documentCommitImport godoc
// @Summary 提交已校验的券商自选导入
// @Tags watchlist
// @Accept json
// @Produce json
// @Param previewId path string true "预览 ID"
// @Param input body domain.CommitImportInput false "选择性删除的本地独有标的"
// @Success 200 {object} httpserver.Envelope{data=WatchlistImportRun}
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/imports/{previewId}/commit [post]
func documentCommitImport() {}

// documentListImportRuns godoc
// @Summary 查询自选导入审计记录
// @Tags watchlist
// @Produce json
// @Param sourceId query string false "来源 ID"
// @Param cursor query string false "游标"
// @Param limit query int false "页大小"
// @Success 200 {object} httpserver.Envelope{data=WatchlistImportRunsData}
// @Failure 400 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/watchlist/import-runs [get]
func documentListImportRuns() {}
