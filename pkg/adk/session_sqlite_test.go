package adk

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestValidateSQLiteSessionServiceAcceptsCurrentSchema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	service, err := NewSQLiteSessionService(dir + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		jftradeErr1 := service.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})

	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("first ValidateSQLiteSessionService: %v", err)
	}
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("second ValidateSQLiteSessionService: %v", err)
	}

	ready, err := sqliteSessionSchemaReady(service.db)
	if err != nil {
		t.Fatalf("sqliteSessionSchemaReady: %v", err)
	}
	if !ready {
		t.Fatal("expected sqlite session schema to be ready")
	}
}

func TestSQLiteSessionServiceReopenPreservesADKEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "adk-session.db")
	service, err := NewSQLiteSessionService(path)
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		jftradeErr1 := service.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})

	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName:   "app",
		UserID:    googleADKUserID,
		SessionID: "session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	event := adksession.NewEvent(ctx, "run")
	event.Author = "user"
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("hello", genai.RoleUser),
	}
	if err := service.AppendEvent(ctx, created.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	reopened, err := NewSQLiteSessionService(path)
	if err != nil {
		t.Fatalf("reopen NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		jftradeErr2 := reopened.Close()
		jftradeCheckTestError(t, jftradeErr2)
	})
	response, err := reopened.Get(ctx, &adksession.GetRequest{
		AppName:   "app",
		UserID:    googleADKUserID,
		SessionID: "session",
	})
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got := response.Session.Events().Len(); got != 1 {
		t.Fatalf("events after reopen = %d, want 1", got)
	}
}

func TestSQLiteSessionServiceBoundaries(t *testing.T) {
	t.Parallel()

	if service, err := NewSQLiteSessionService(" "); err == nil || service != nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("NewSQLiteSessionService(blank) = %#v/%v, want path error", service, err)
	}
	var nilService *SQLiteSessionService
	if got := nilService.DatabasePath(); got != "" {
		t.Fatalf("nil DatabasePath = %q, want empty", got)
	}
	if err := CompactSQLiteSessionService(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("CompactSQLiteSessionService(nil) err = %v, want unavailable error", err)
	}
	if err := ValidateSQLiteSessionService(nil); err == nil || !strings.Contains(err.Error(), "schema is unavailable") {
		t.Fatalf("ValidateSQLiteSessionService(nil) err = %v, want unavailable error", err)
	}

	path := filepath.Join(t.TempDir(), "adk-session.db")
	service, err := NewSQLiteSessionService(path)
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		jftradeErr1 := service.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})
	if err := CompactSQLiteSessionService(context.Background(), service); err != nil {
		t.Fatalf("CompactSQLiteSessionService: %v", err)
	}
}
