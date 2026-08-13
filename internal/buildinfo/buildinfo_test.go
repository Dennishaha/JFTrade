package buildinfo

import (
	"runtime"
	"testing"
)

func TestSnapshotTrimsBuildMetadataAndDefaultsBuildTime(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})

	Version = " 1.2.3 "
	Commit = " abc123 "
	BuildTime = " "

	snapshot := Snapshot()
	if snapshot.Version != "1.2.3" || snapshot.Commit != "abc123" || snapshot.BuildTime != "dev" {
		t.Fatalf("Snapshot metadata = %#v", snapshot)
	}
	if snapshot.GOOS != runtime.GOOS || snapshot.GOARCH != runtime.GOARCH {
		t.Fatalf("Snapshot runtime = %#v, want %s/%s", snapshot, runtime.GOOS, runtime.GOARCH)
	}

	BuildTime = "2026-07-02T00:00:00Z"
	if got := Snapshot().BuildTime; got != "2026-07-02T00:00:00Z" {
		t.Fatalf("Snapshot buildTime = %#v", got)
	}
}
