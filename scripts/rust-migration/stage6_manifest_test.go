package rustmigration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type stage6Manifest struct {
	Version                  int    `json:"version"`
	Owner                    string `json:"owner"`
	ImmutableAfterStageClose bool   `json:"immutableAfterStageClose"`
	Files                    []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func stage6FixtureDirectory(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 6 manifest test source")
	}
	return filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage6")
}

func TestStage6ManifestPinsAssistantRigEvidence(t *testing.T) {
	directory := stage6FixtureDirectory(t)
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatalf("read stage 6 manifest: %v", err)
	}
	var manifest stage6Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode stage 6 manifest: %v", err)
	}
	if manifest.Version != 1 || manifest.Owner == "" || !manifest.ImmutableAfterStageClose || len(manifest.Files) != 3 {
		t.Fatalf(
			"stage 6 manifest is incomplete: version=%d owner=%q immutable=%t files=%d",
			manifest.Version,
			manifest.Owner,
			manifest.ImmutableAfterStageClose,
			len(manifest.Files),
		)
	}
	for _, file := range manifest.Files {
		contents, err := os.ReadFile(filepath.Join(directory, file.Path))
		if err != nil {
			t.Fatalf("read pinned fixture %q: %v", file.Path, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != file.SHA256 {
			t.Errorf("fixture %q sha256 = %s, want %s", file.Path, got, file.SHA256)
		}
	}
}
