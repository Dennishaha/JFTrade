package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveFrontendAssetsPreservesRelativePathsAndTimestamp(t *testing.T) {
	srcDir := t.TempDir()
	nestedDir := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatalf("write child file: %v", err)
	}

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	if err := archiveFrontendAssets(srcDir, zipWriter); err != nil {
		t.Fatalf("archive frontend assets: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatalf("open zip reader: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("zip file count = %d, want 2", len(reader.File))
	}
	if reader.File[0].Name != "nested/child.txt" || reader.File[1].Name != "root.txt" {
		t.Fatalf("zip names = %q, %q", reader.File[0].Name, reader.File[1].Name)
	}
	for _, file := range reader.File {
		if !file.Modified.Equal(fixedArchiveTimestamp) {
			t.Fatalf("file %q modified = %v, want %v", file.Name, file.Modified, fixedArchiveTimestamp)
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip file %q: %v", file.Name, err)
		}
		content := new(bytes.Buffer)
		if _, err := content.ReadFrom(rc); err != nil {
			t.Fatalf("read zip file %q: %v", file.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close zip file %q: %v", file.Name, err)
		}
		if got, want := content.String(), expectedArchiveContent(file.Name); got != want {
			t.Fatalf("content for %q = %q, want %q", file.Name, got, want)
		}
	}
}

func expectedArchiveContent(name string) string {
	switch name {
	case "nested/child.txt":
		return "child"
	case "root.txt":
		return "root"
	default:
		return ""
	}
}
