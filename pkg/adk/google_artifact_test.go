package adk

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	adkartifact "google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

func TestGoogleADKArtifactServiceStoresVersionedArtifacts(t *testing.T) {
	ctx := context.Background()
	service := newGoogleADKArtifactService()
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
	versionOne, err := service.Load(ctx, &adkartifact.LoadRequest{
		AppName: googleADKAppName("artifact-agent"), UserID: googleADKUserID, SessionID: "session-artifact", FileName: "report.txt", Version: first.Version,
	})
	if err != nil {
		t.Fatalf("Load version one: %v", err)
	}
	if versionOne.Part == nil || versionOne.Part.Text != "first" {
		t.Fatalf("version one artifact = %#v", versionOne.Part)
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
