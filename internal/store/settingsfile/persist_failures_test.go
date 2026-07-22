package settingsfile

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

type failingSettingsTemporaryFile struct {
	path     string
	chmodErr error
	writeErr error
	syncErr  error
	closeErr error
}

func (f *failingSettingsTemporaryFile) Name() string            { return f.path }
func (f *failingSettingsTemporaryFile) Chmod(fs.FileMode) error { return f.chmodErr }
func (f *failingSettingsTemporaryFile) Write(data []byte) (int, error) {
	return len(data), f.writeErr
}
func (f *failingSettingsTemporaryFile) Sync() error  { return f.syncErr }
func (f *failingSettingsTemporaryFile) Close() error { return f.closeErr }

func TestPersistLockedPropagatesTemporaryFileDurabilityFailures(t *testing.T) {
	sentinel := errors.New("settings temporary file failure")
	tests := []struct {
		name      string
		configure func(*failingSettingsTemporaryFile)
	}{
		{name: "chmod", configure: func(file *failingSettingsTemporaryFile) { file.chmodErr = sentinel }},
		{name: "write", configure: func(file *failingSettingsTemporaryFile) { file.writeErr = sentinel }},
		{name: "sync", configure: func(file *failingSettingsTemporaryFile) { file.syncErr = sentinel }},
		{name: "close", configure: func(file *failingSettingsTemporaryFile) { file.closeErr = sentinel }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := New(filepath.Join(t.TempDir(), "settings.json"))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			file := &failingSettingsTemporaryFile{path: filepath.Join(filepath.Dir(store.path), "temporary")}
			test.configure(file)
			store.createTemp = func(string, string) (settingsTemporaryFile, error) { return file, nil }
			if err := store.persistLocked(); !errors.Is(err, sentinel) {
				t.Fatalf("persistLocked() error = %v, want %v", err, sentinel)
			}
		})
	}

	store, err := New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	store.createTemp = func(string, string) (settingsTemporaryFile, error) { return nil, sentinel }
	if err := store.persistLocked(); !errors.Is(err, sentinel) {
		t.Fatalf("persistLocked(create) error = %v, want %v", err, sentinel)
	}
	store.createTemp = nil
	store.replaceFile = nil
	if err := store.persistLocked(); err != nil {
		t.Fatalf("persistLocked(default hooks): %v", err)
	}
}
