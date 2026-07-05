package adk

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	adkartifact "google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

func TestGoogleADKArtifactServiceStoresVersionedArtifacts(t *testing.T) {
	ctx := context.Background()
	service, err := newGoogleADKArtifactService(filepath.Join(t.TempDir(), "adk-artifact.db"))
	if err != nil {
		t.Fatalf("newGoogleADKArtifactService: %v", err)
	}
	t.Cleanup(func() {
		if err := CloseArtifactService(service); err != nil {
			t.Fatalf("CloseArtifactService: %v", err)
		}
	})
	first, err := service.Save(ctx, &adkartifact.SaveRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
		Part: genai.NewPartFromText("first"),
	})
	if err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second, err := service.Save(ctx, &adkartifact.SaveRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
		Part: genai.NewPartFromText("second"),
	})
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("versions = %d/%d, want 1/2", first.Version, second.Version)
	}
	latest, err := service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	})
	if err != nil {
		t.Fatalf("Load latest: %v", err)
	}
	if latest.Part == nil || latest.Part.Text != "second" {
		t.Fatalf("latest artifact = %#v", latest.Part)
	}
	versions, err := service.Versions(ctx, &adkartifact.VersionsRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions.Versions) != 2 || versions.Versions[0] != 2 || versions.Versions[1] != 1 {
		t.Fatalf("Versions = %#v, want [2 1]", versions.Versions)
	}
	versionOne, err := service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt", Version: first.Version,
	})
	if err != nil {
		t.Fatalf("Load version one: %v", err)
	}
	if versionOne.Part == nil || versionOne.Part.Text != "first" {
		t.Fatalf("version one artifact = %#v", versionOne.Part)
	}
	meta, err := service.GetArtifactVersion(ctx, &adkartifact.GetArtifactVersionRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	})
	if err != nil {
		t.Fatalf("GetArtifactVersion latest: %v", err)
	}
	if meta.ArtifactVersion == nil || meta.ArtifactVersion.Version != second.Version || meta.ArtifactVersion.MimeType != "text/plain" || meta.ArtifactVersion.CanonicalURI == "" {
		t.Fatalf("latest artifact metadata = %#v", meta.ArtifactVersion)
	}
	list, err := service.List(ctx, &adkartifact.ListRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.FileNames) != 1 || list.FileNames[0] != "report.txt" {
		t.Fatalf("List filenames = %#v", list.FileNames)
	}
	if err := service.Delete(ctx, &adkartifact.DeleteRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt", Version: first.Version,
	}); err != nil {
		t.Fatalf("Delete version one: %v", err)
	}
	if _, err := service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt", Version: first.Version,
	}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load deleted version one err = %v, want fs.ErrNotExist", err)
	}
	latest, err = service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	})
	if err != nil || latest.Part == nil || latest.Part.Text != "second" {
		t.Fatalf("latest after deleting version one = %#v/%v", latest, err)
	}
	if err := service.Delete(ctx, &adkartifact.DeleteRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt",
	}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load deleted err = %v, want fs.ErrNotExist", err)
	}
}

func TestGoogleADKArtifactServicePersistsAcrossRestartAndUserScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "adk-artifact.db")
	service, err := newGoogleADKArtifactService(path)
	if err != nil {
		t.Fatalf("newGoogleADKArtifactService: %v", err)
	}
	if _, err := service.Save(ctx, &adkartifact.SaveRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-a", FileName: "user:profile.txt",
		Part: genai.NewPartFromBytes([]byte("profile"), "text/custom"),
	}); err != nil {
		t.Fatalf("Save user artifact: %v", err)
	}
	if _, err := service.Save(ctx, &adkartifact.SaveRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-a", FileName: "local.txt",
		Part: genai.NewPartFromText("local"),
	}); err != nil {
		t.Fatalf("Save session artifact: %v", err)
	}
	if err := CloseArtifactService(service); err != nil {
		t.Fatalf("Close first service: %v", err)
	}

	reopened, err := newGoogleADKArtifactService(path)
	if err != nil {
		t.Fatalf("reopen artifact service: %v", err)
	}
	t.Cleanup(func() {
		if err := CloseArtifactService(reopened); err != nil {
			t.Fatalf("Close reopened service: %v", err)
		}
	})
	list, err := reopened.List(ctx, &adkartifact.ListRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-b",
	})
	if err != nil {
		t.Fatalf("List session-b: %v", err)
	}
	if len(list.FileNames) != 1 || list.FileNames[0] != "user:profile.txt" {
		t.Fatalf("session-b filenames = %#v, want only user-scoped artifact", list.FileNames)
	}
	loaded, err := reopened.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-b", FileName: "user:profile.txt",
	})
	if err != nil {
		t.Fatalf("Load user artifact from other session: %v", err)
	}
	if loaded.Part == nil || loaded.Part.InlineData == nil || string(loaded.Part.InlineData.Data) != "profile" {
		t.Fatalf("loaded user artifact = %#v", loaded.Part)
	}
	meta, err := reopened.GetArtifactVersion(ctx, &adkartifact.GetArtifactVersionRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-b", FileName: "user:profile.txt", Version: 1,
	})
	if err != nil {
		t.Fatalf("GetArtifactVersion user artifact: %v", err)
	}
	if meta.ArtifactVersion == nil || meta.ArtifactVersion.MimeType != "text/custom" || meta.ArtifactVersion.CreateTime.IsZero() {
		t.Fatalf("user artifact metadata = %#v", meta.ArtifactVersion)
	}
}

func TestGoogleADKArtifactServiceBoundaries(t *testing.T) {
	if service, err := newGoogleADKArtifactService(" "); err != nil || service == nil {
		t.Fatalf("blank artifact path service = %#v/%v", service, err)
	}
	if _, err := newGoogleADKArtifactService(filepath.Join(t.TempDir(), "missing", "adk-artifact.db")); err == nil {
		t.Fatal("newGoogleADKArtifactService with missing parent err = nil")
	}
	if err := CloseArtifactService(nil); err != nil {
		t.Fatalf("CloseArtifactService(nil): %v", err)
	}
	if err := (*googleADKArtifactService)(nil).Close(); err != nil {
		t.Fatalf("nil service Close: %v", err)
	}
	if err := (&googleADKArtifactService{}).Close(); err != nil {
		t.Fatalf("empty service Close: %v", err)
	}
	if err := (*googleADKArtifactService)(nil).init(context.Background()); err == nil {
		t.Fatal("nil service init err = nil")
	}

	path := filepath.Join(t.TempDir(), "adk-artifact.db")
	service, err := newGoogleADKArtifactService(path)
	if err != nil {
		t.Fatalf("newGoogleADKArtifactService: %v", err)
	}
	typed := service.(*googleADKArtifactService)
	explicit, err := typed.Save(context.Background(), &adkartifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "explicit.txt", Version: 7, Part: genai.NewPartFromText("explicit"),
	})
	if err != nil || explicit.Version != 7 {
		t.Fatalf("explicit Save = %#v/%v, want version 7", explicit, err)
	}
	if _, err := typed.Save(context.Background(), &adkartifact.SaveRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid Save err = %v, want validation error", err)
	}
	if _, err := (*googleADKArtifactService)(nil).Save(context.Background(), &adkartifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "nil.txt", Part: genai.NewPartFromText("nil"),
	}); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("nil Save err = %v, want unavailable error", err)
	}
	if _, err := typed.Load(context.Background(), &adkartifact.LoadRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid Load err = %v, want validation error", err)
	}
	if _, err := (*googleADKArtifactService)(nil).loadRecord(context.Background(), "app", "user", "session", "nil.txt", 1); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("nil loadRecord err = %v, want unavailable error", err)
	}
	if _, err := typed.loadRecord(context.Background(), "app", "user", "session", "missing.txt", 0); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("load missing err = %v, want fs.ErrNotExist", err)
	}
	if _, err := typed.db.ExecContext(context.Background(), `INSERT INTO artifacts
		(app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"app", "user", "session", "bad-part.txt", 1, "{bad", "text/plain", "null", "now", "now",
	); err != nil {
		t.Fatalf("insert bad part artifact: %v", err)
	}
	if _, err := typed.Load(context.Background(), &adkartifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "bad-part.txt", Version: 1,
	}); err == nil || !strings.Contains(err.Error(), "unmarshal ADK artifact part") {
		t.Fatalf("bad part Load err = %v, want unmarshal error", err)
	}
	if _, err := typed.db.ExecContext(context.Background(), `INSERT INTO artifacts
		(app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"app", "user", "session", "bad-meta.txt", 1, `{"text":"ok"}`, "text/plain", "{bad", "now", "now",
	); err != nil {
		t.Fatalf("insert bad metadata artifact: %v", err)
	}
	if _, err := typed.Load(context.Background(), &adkartifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "bad-meta.txt", Version: 1,
	}); err == nil || !strings.Contains(err.Error(), "unmarshal ADK artifact metadata") {
		t.Fatalf("bad metadata Load err = %v, want metadata unmarshal error", err)
	}
	if err := typed.Delete(context.Background(), &adkartifact.DeleteRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid Delete err = %v, want validation error", err)
	}
	if err := (*googleADKArtifactService)(nil).Delete(context.Background(), &adkartifact.DeleteRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "nil.txt",
	}); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("nil Delete err = %v, want unavailable error", err)
	}
	if _, err := typed.List(context.Background(), &adkartifact.ListRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid List err = %v, want validation error", err)
	}
	if _, err := (*googleADKArtifactService)(nil).List(context.Background(), &adkartifact.ListRequest{
		AppName: "app", UserID: "user", SessionID: "session",
	}); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("nil List err = %v, want unavailable error", err)
	}
	if _, err := typed.Versions(context.Background(), &adkartifact.VersionsRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid Versions err = %v, want validation error", err)
	}
	if _, err := (*googleADKArtifactService)(nil).Versions(context.Background(), &adkartifact.VersionsRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "nil.txt",
	}); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("nil Versions err = %v, want unavailable error", err)
	}
	if _, err := typed.GetArtifactVersion(context.Background(), &adkartifact.GetArtifactVersionRequest{}); err == nil || !strings.Contains(err.Error(), "request validation failed") {
		t.Fatalf("invalid GetArtifactVersion err = %v, want validation error", err)
	}
	if err := typed.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := typed.Save(context.Background(), &adkartifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed.txt", Part: genai.NewPartFromText("closed"),
	}); err == nil {
		t.Fatal("Save on closed artifact service err = nil")
	}
	if _, err := typed.Save(context.Background(), &adkartifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed-explicit.txt", Version: 1, Part: genai.NewPartFromText("closed"),
	}); err == nil {
		t.Fatal("explicit-version Save on closed artifact service err = nil")
	}
	if _, err := typed.Load(context.Background(), &adkartifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed.txt",
	}); err == nil {
		t.Fatal("Load on closed artifact service err = nil")
	}
	if err := typed.Delete(context.Background(), &adkartifact.DeleteRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed.txt",
	}); err == nil {
		t.Fatal("Delete on closed artifact service err = nil")
	}
	if _, err := typed.List(context.Background(), &adkartifact.ListRequest{
		AppName: "app", UserID: "user", SessionID: "session",
	}); err == nil {
		t.Fatal("List on closed artifact service err = nil")
	}
	if _, err := typed.Versions(context.Background(), &adkartifact.VersionsRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed.txt",
	}); err == nil {
		t.Fatal("Versions on closed artifact service err = nil")
	}
	if _, err := typed.GetArtifactVersion(context.Background(), &adkartifact.GetArtifactVersionRequest{
		AppName: "app", UserID: "user", SessionID: "session", FileName: "closed.txt",
	}); err == nil {
		t.Fatal("GetArtifactVersion on closed artifact service err = nil")
	}
}

func TestGoogleADKArtifactPathDerivesFromSQLiteSessionService(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "adk-session.db")
	service, err := NewSQLiteSessionService(sessionPath)
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Fatalf("Close session service: %v", err)
		}
	})
	got := deriveGoogleADKArtifactPathFromSessionService(service)
	want := filepath.Join(filepath.Dir(sessionPath), "adk-artifact.db")
	if got != want {
		t.Fatalf("artifact path = %q, want %q", got, want)
	}
	if got := deriveGoogleADKArtifactPathFromSessionService(adkartifact.InMemoryService()); got != "" {
		t.Fatalf("artifact path from non-path provider = %q, want empty", got)
	}
	if got := deriveGoogleADKArtifactPathFromSessionService(emptyADKSessionPathProvider{}); got != "" {
		t.Fatalf("artifact path from empty path provider = %q, want empty", got)
	}
}

type emptyADKSessionPathProvider struct{}

func (emptyADKSessionPathProvider) DatabasePath() string {
	return " "
}
