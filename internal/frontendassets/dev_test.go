//go:build !release_assets

package frontendassets

import "testing"

func TestFileSystemReportsExternalAssetsForDevelopmentBuild(t *testing.T) {
	frontendFS, available, err := FileSystem()
	if err != nil {
		t.Fatalf("FileSystem() error = %v", err)
	}
	if frontendFS != nil || available {
		t.Fatalf("FileSystem() = (%#v, %v), want (nil, false)", frontendFS, available)
	}
}
