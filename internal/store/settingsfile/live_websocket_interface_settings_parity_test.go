package settingsfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestLiveWebSocketInterfaceSettingsMatchRustMigrationCorpus(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "tests", "fixtures", "rust-migration", "stage9", "live-websocket-interface-settings.json")
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	var corpus struct {
		Version string `json:"version"`
		Cases   []struct {
			Name          string          `json:"name"`
			Document      json.RawMessage `json:"document"`
			ExpectedLimit int             `json:"expectedLimit"`
			ExpectedError bool            `json:"expectedError"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatalf("Unmarshal fixture: %v", err)
	}
	if corpus.Version != "stage9.live-websocket-interface-settings.v1" {
		t.Fatalf("version = %q", corpus.Version)
	}
	for _, testCase := range corpus.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, testCase.Document, 0o600); err != nil {
				t.Fatalf("WriteFile settings: %v", err)
			}
			store, err := New(path)
			if testCase.ExpectedError {
				if err == nil {
					t.Fatal("New succeeded for malformed interface settings")
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			actual := store.InterfaceSettings(jfsettings.LaunchDefaults{}).LiveWebSocketConnectionLimit
			if actual != testCase.ExpectedLimit {
				t.Fatalf("limit = %d, want %d", actual, testCase.ExpectedLimit)
			}
		})
	}
}
