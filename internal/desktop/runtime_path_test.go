package desktop

import (
	"path/filepath"
	"testing"
)

func TestProductDataDirByPlatform(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		home      string
		config    string
		env       map[string]string
		wantParts []string
	}{
		{name: "macOS", goos: "darwin", home: "/Users/alice", config: "/Users/alice/Library/Application Support", wantParts: []string{"/Users/alice/Library/Application Support", "JFTrade"}},
		{name: "Windows local app data", goos: "windows", home: `C:\\Users\\alice`, config: `C:\\Users\\alice\\AppData\\Roaming`, env: map[string]string{"LOCALAPPDATA": `C:\\Users\\alice\\AppData\\Local`}, wantParts: []string{`C:\\Users\\alice\\AppData\\Local`, "JFTrade"}},
		{name: "Linux XDG", goos: "linux", home: "/home/alice", env: map[string]string{"XDG_DATA_HOME": "/data/alice"}, wantParts: []string{"/data/alice", "jftrade"}},
		{name: "Linux fallback", goos: "linux", home: "/home/alice", wantParts: []string{"/home/alice", ".local", "share", "jftrade"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := productDataDir(tt.goos, tt.home, tt.config, func(key string) string { return tt.env[key] })
			want := filepath.Join(tt.wantParts...)
			if got != want {
				t.Fatalf("productDataDir() = %q, want %q", got, want)
			}
		})
	}
}
