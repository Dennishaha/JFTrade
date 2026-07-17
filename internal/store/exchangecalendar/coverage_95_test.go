package exchangecalendar

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSnapshotsReportsWalkAndReadFailures(t *testing.T) {
	missing := New(filepath.Join(t.TempDir(), "missing"))
	if snapshots, errs := missing.LoadSnapshots(); len(snapshots) != 0 || len(errs) != 1 {
		t.Fatalf("LoadSnapshots(missing root) = %#v, %#v", snapshots, errs)
	}

	root := t.TempDir()
	broken := filepath.Join(root, "broken.json")
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), broken); err != nil {
		t.Fatalf("create broken snapshot symlink: %v", err)
	}
	if snapshots, errs := New(root).LoadSnapshots(); len(snapshots) != 0 || len(errs) != 1 {
		t.Fatalf("LoadSnapshots(broken snapshot) = %#v, %#v", snapshots, errs)
	}

	// Logging a best-effort error must not change the caller-visible result.
	jftradeLogError(errors.New("calendar cache unavailable"), "not-an-error", nil)
}
