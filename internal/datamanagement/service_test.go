package datamanagement

import (
	"context"
	"errors"
	"testing"
)

type fakeBackend struct {
	called map[string]bool
	err    error
}

func (b *fakeBackend) Overview(_ context.Context, request OverviewRequest) (any, error) {
	b.called["overview"] = request.SummaryOnly && request.DatabaseID == "strategy"
	return "overview", b.err
}

func (b *fakeBackend) PreviewCleanup(_ context.Context, request CleanupPreviewRequest) (any, error) {
	b.called["preview"] = request.Kind == "soft-deleted" && request.DatabaseID == "strategy"
	return "preview", b.err
}

func (b *fakeBackend) ExecuteCleanup(_ context.Context, request CleanupExecuteRequest) (any, error) {
	b.called["execute"] = request.PreviewID == "p" && request.Confirmation != ""
	return "execute", b.err
}

func (b *fakeBackend) Compact(_ context.Context, databaseID string, request CompactRequest) (any, error) {
	b.called["compact"] = databaseID == "strategy" && request.Confirmation != ""
	return "compact", b.err
}

func (b *fakeBackend) Backup(_ context.Context, request BackupRequest) (any, error) {
	b.called["backup"] = request.DatabaseID == "watchlist" && request.Confirmation == "BACKUP watchlist"
	return "backup", b.err
}

func (b *fakeBackend) Rebuild(_ context.Context, request RebuildRequest) (any, error) {
	b.called["rebuild"] = request.DatabaseID == "strategy" && request.Mode == "single"
	return "rebuild", b.err
}

func TestServiceFallbacks(t *testing.T) {
	svc := NewService(nil)
	status, err := svc.Overview(t.Context(), OverviewRequest{})
	if err != nil {
		t.Fatalf("Overview default: %v", err)
	}
	got, ok := status.(map[string]any)
	if !ok || len(got["databases"].([]any)) != 0 {
		t.Fatalf("default overview = %#v", status)
	}
	if _, err := svc.PreviewCleanup(t.Context(), CleanupPreviewRequest{}); err == nil {
		t.Fatal("PreviewCleanup fallback succeeded")
	}
	if _, err := svc.ExecuteCleanup(t.Context(), CleanupExecuteRequest{}); err == nil {
		t.Fatal("ExecuteCleanup fallback succeeded")
	}
	if _, err := svc.Compact(t.Context(), "strategy", CompactRequest{}); err == nil {
		t.Fatal("Compact fallback succeeded")
	}
	if _, err := svc.Backup(t.Context(), BackupRequest{}); err == nil {
		t.Fatal("Backup fallback succeeded")
	}
	if _, err := svc.Rebuild(t.Context(), RebuildRequest{}); err == nil {
		t.Fatal("Rebuild fallback succeeded")
	}
}

func TestServiceDelegatesTypedRequests(t *testing.T) {
	backend := &fakeBackend{called: map[string]bool{}}
	svc := NewService(backend)
	ctx := t.Context()

	if _, err := svc.Overview(ctx, OverviewRequest{SummaryOnly: true, DatabaseID: "strategy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewCleanup(ctx, CleanupPreviewRequest{Kind: "soft-deleted", DatabaseID: "strategy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteCleanup(ctx, CleanupExecuteRequest{PreviewID: "p", Confirmation: "CLEANUP strategy 1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Compact(ctx, "strategy", CompactRequest{Confirmation: "COMPACT strategy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Backup(ctx, BackupRequest{DatabaseID: "watchlist", Confirmation: "BACKUP watchlist"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rebuild(ctx, RebuildRequest{DatabaseID: "strategy", Mode: "single"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"overview", "preview", "execute", "compact", "backup", "rebuild"} {
		if !backend.called[name] {
			t.Fatalf("backend method %s was not called with typed request", name)
		}
	}
}

func TestServicePreservesBackendErrors(t *testing.T) {
	wantErr := errors.New("backend failed")
	svc := NewService(&fakeBackend{called: map[string]bool{}, err: wantErr})
	if _, err := svc.Overview(t.Context(), OverviewRequest{}); !errors.Is(err, wantErr) {
		t.Fatalf("Overview error = %v, want %v", err, wantErr)
	}
}
