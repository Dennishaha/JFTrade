package pineworkerassets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestSelectFromFSReturnsEmbeddedBundleMetadata(t *testing.T) {
	data := []byte("export default 'pineworker'")
	asset, available, err := selectFromFS(fstest.MapFS{
		"bin/worker.mjs": &fstest.MapFile{Data: data},
	})
	if err != nil {
		t.Fatalf("selectFromFS() error = %v", err)
	}
	if !available {
		t.Fatal("selectFromFS() available = false, want true")
	}
	sum := sha256.Sum256(data)
	if asset.Name != BundleName() || string(asset.Data) != string(data) || asset.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("selectFromFS() asset = %#v", asset)
	}
}

func TestSelectFromFSTreatsMissingAndEmptyBundlesAsUnavailable(t *testing.T) {
	tests := []struct {
		name  string
		files fs.FS
	}{
		{name: "missing", files: fstest.MapFS{}},
		{name: "empty", files: fstest.MapFS{"bin/worker.mjs": &fstest.MapFile{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asset, available, err := selectFromFS(test.files)
			if err != nil {
				t.Fatalf("selectFromFS() error = %v", err)
			}
			if available || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
				t.Fatalf("selectFromFS() = (%#v, %v), want empty unavailable asset", asset, available)
			}
		})
	}
}

func TestSelectFromFSReturnsUnexpectedReadError(t *testing.T) {
	wantErr := errors.New("asset storage unavailable")
	asset, available, err := selectFromFS(failingAssetFS{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("selectFromFS() error = %v, want %v", err, wantErr)
	}
	if available || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
		t.Fatalf("selectFromFS() = (%#v, %v), want empty unavailable asset", asset, available)
	}
}

func TestIsMissingAssetRecognizesOnlyNotFoundErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil},
		{name: "fs not exist", err: fs.ErrNotExist, want: true},
		{name: "platform no such file", err: errors.New("no such file or directory"), want: true},
		{name: "other error", err: errors.New("permission denied")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isMissingAsset(test.err); got != test.want {
				t.Fatalf("isMissingAsset(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

type failingAssetFS struct {
	err error
}

func (files failingAssetFS) Open(string) (fs.File, error) {
	return nil, files.err
}
