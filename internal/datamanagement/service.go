package datamanagement

import (
	"context"
	"errors"
)

var (
	ErrDatabaseMaintenanceConflict = errors.New("database maintenance conflict")
	ErrCleanupPreviewNotFound      = errors.New("cleanup preview not found or expired")
	ErrCleanupPreviewStale         = errors.New("cleanup preview is stale")
)

type OverviewRequest struct {
	SummaryOnly bool   `json:"summaryOnly"`
	DatabaseID  string `json:"databaseId"`
}

type CleanupPreviewRequest struct {
	Kind          string `json:"kind"`
	DatabaseID    string `json:"databaseId"`
	OlderThanDays int    `json:"olderThanDays,omitempty"`
	KeepLatest    int    `json:"keepLatest,omitempty"`
}

type CleanupExecuteRequest struct {
	PreviewID    string `json:"previewId"`
	Confirmation string `json:"confirmation"`
}

type CompactRequest struct {
	Confirmation string `json:"confirmation"`
}

type RebuildRequest struct {
	DatabaseIDs  []string `json:"databaseIds"`
	DatabaseID   string   `json:"databaseId"`
	Mode         string   `json:"mode"`
	Confirmation string   `json:"confirmation"`
}

type Backend interface {
	Overview(context.Context, OverviewRequest) (any, error)
	PreviewCleanup(context.Context, CleanupPreviewRequest) (any, error)
	ExecuteCleanup(context.Context, CleanupExecuteRequest) (any, error)
	Compact(context.Context, string, CompactRequest) (any, error)
	Rebuild(context.Context, RebuildRequest) (any, error)
}

type Service struct {
	backend Backend
}

func NewService(backend Backend) *Service {
	return &Service{backend: backend}
}

func (s *Service) Overview(ctx context.Context, request OverviewRequest) (any, error) {
	if s == nil || s.backend == nil {
		return map[string]any{"databases": []any{}}, nil
	}
	return s.backend.Overview(ctx, request)
}

func (s *Service) PreviewCleanup(ctx context.Context, request CleanupPreviewRequest) (any, error) {
	if s == nil || s.backend == nil {
		return nil, errors.New("database cleanup preview is unavailable")
	}
	return s.backend.PreviewCleanup(ctx, request)
}

func (s *Service) ExecuteCleanup(ctx context.Context, request CleanupExecuteRequest) (any, error) {
	if s == nil || s.backend == nil {
		return nil, errors.New("database cleanup is unavailable")
	}
	return s.backend.ExecuteCleanup(ctx, request)
}

func (s *Service) Compact(ctx context.Context, databaseID string, request CompactRequest) (any, error) {
	if s == nil || s.backend == nil {
		return nil, errors.New("database compaction is unavailable")
	}
	return s.backend.Compact(ctx, databaseID, request)
}

func (s *Service) Rebuild(ctx context.Context, request RebuildRequest) (any, error) {
	if s == nil || s.backend == nil {
		return nil, errors.New("database rebuild is unavailable")
	}
	return s.backend.Rebuild(ctx, request)
}
