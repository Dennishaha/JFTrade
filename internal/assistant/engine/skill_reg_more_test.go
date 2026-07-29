package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
)

func zipArchiveOrderedForSkillBoundary(t *testing.T, entries []struct {
	name    string
	content string
	mode    os.FileMode
}) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		header.SetMode(entry.mode)
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader(%s): %v", entry.name, err)
		}
		if entry.mode.IsDir() {
			continue
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatalf("Write(%s): %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	return archive.Bytes()
}

func TestSkillRegistryAdditionalFilesystemAndArchiveBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("source uninstall and filtered frontmatter boundaries", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, registry.skillsPath, "builtin-skill", "---\nname: builtin-skill\ndescription: Builtin skill\nmetadata:\n  source: builtin\n---\nBuiltin.")
		if err := registry.Uninstall(ctx, "missing-skill"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Uninstall missing err = %v, want os.ErrNotExist", err)
		}
		if err := registry.Uninstall(ctx, "builtin-skill"); err == nil || !strings.Contains(err.Error(), "builtin skills cannot be uninstalled") {
			t.Fatalf("Uninstall builtin err = %v", err)
		}

		bad := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, bad.skillsPath, "bad", "---\nname: bad\nmetadata: [\n---\nBroken.")
		if _, err := bad.source(ctx); err == nil {
			t.Fatal("source() accepted malformed frontmatter")
		}

		base := &googleADKFakeSkillSource{
			frontmatters: []*adkskill.Frontmatter{{Name: "allowed", Description: "ok"}},
		}
		filtered := &filteredSkillSource{base: base, allowed: map[string]struct{}{"allowed": {}}}
		fm, err := filtered.LoadFrontmatter(ctx, "allowed")
		if err != nil || fm == nil || fm.Name != "allowed" {
			t.Fatalf("filtered LoadFrontmatter = %#v err=%v, want allowed frontmatter", fm, err)
		}
	})

	t.Run("archive installation surfaces ordered path conflicts and parse errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		registry := runtime.Skills()

		fileThenDir := zipArchiveOrderedForSkillBoundary(t, []struct {
			name    string
			content string
			mode    os.FileMode
		}{
			{name: "broken/file", content: "x", mode: 0o644},
			{name: "broken/file/", mode: os.ModeDir | 0o755},
		})
		if _, err := registry.installArchive(ctx, "https://example.com/file-then-dir.zip", fileThenDir); err == nil {
			t.Fatal("installArchive accepted file-then-dir conflict")
		}

		dirThenFile := zipArchiveOrderedForSkillBoundary(t, []struct {
			name    string
			content string
			mode    os.FileMode
		}{
			{name: "broken-dir/", mode: os.ModeDir | 0o755},
			{name: "broken-dir", content: "x", mode: 0o644},
		})
		if _, err := registry.installArchive(ctx, "https://example.com/dir-then-file.zip", dirThenFile); err == nil {
			t.Fatal("installArchive accepted dir-then-file conflict")
		}

		badDoc := zipArchiveOrderedForSkillBoundary(t, []struct {
			name    string
			content string
			mode    os.FileMode
		}{
			{name: "bad/SKILL.md", content: "---\nname: bad\nmetadata: [\n---\nBroken.", mode: 0o644},
		})
		if _, err := registry.installArchive(ctx, "https://example.com/bad-doc.zip", badDoc); err == nil {
			t.Fatal("installArchive accepted malformed SKILL.md")
		}
	})

	t.Run("copy and bundle helpers surface open read and conflict errors", func(t *testing.T) {
		root := t.TempDir()

		if runtime.GOOS != "windows" {
			openSource := filepath.Join(root, "open-source")
			if err := os.MkdirAll(openSource, 0o755); err != nil {
				t.Fatalf("MkdirAll open-source: %v", err)
			}
			if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(openSource, "broken-link")); err != nil {
				t.Fatalf("Symlink broken-link: %v", err)
			}
			if err := copyDirectoryContents(openSource, filepath.Join(root, "open-target")); err == nil {
				t.Fatal("copyDirectoryContents accepted broken symlink source")
			}
		}

		targetConflictSource := filepath.Join(root, "target-conflict-source")
		if err := os.MkdirAll(targetConflictSource, 0o755); err != nil {
			t.Fatalf("MkdirAll target-conflict-source: %v", err)
		}
		if err := os.WriteFile(filepath.Join(targetConflictSource, "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile source file: %v", err)
		}
		targetConflictTarget := filepath.Join(root, "target-conflict-target")
		if err := os.MkdirAll(filepath.Join(targetConflictTarget, "file.txt"), 0o755); err != nil {
			t.Fatalf("MkdirAll conflicting target dir: %v", err)
		}
		if err := copyDirectoryContents(targetConflictSource, targetConflictTarget); err == nil {
			t.Fatal("copyDirectoryContents accepted output path that is already a directory")
		}

		if runtime.GOOS != "windows" {
			readConflictSource := filepath.Join(root, "read-conflict-source")
			realDir := filepath.Join(root, "real-dir")
			if err := os.MkdirAll(realDir, 0o755); err != nil {
				t.Fatalf("MkdirAll real-dir: %v", err)
			}
			if err := os.MkdirAll(readConflictSource, 0o755); err != nil {
				t.Fatalf("MkdirAll read-conflict-source: %v", err)
			}
			if err := os.Symlink(realDir, filepath.Join(readConflictSource, "dir-link")); err != nil {
				t.Fatalf("Symlink dir-link: %v", err)
			}
			if err := copyDirectoryContents(readConflictSource, filepath.Join(root, "read-conflict-target")); err == nil {
				t.Fatal("copyDirectoryContents accepted symlink-to-directory file copy")
			}

			bundleRoot := filepath.Join(root, "bundle-root")
			if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
				t.Fatalf("MkdirAll bundle-root: %v", err)
			}
			if err := os.Symlink(realDir, filepath.Join(bundleRoot, "dir-link")); err != nil {
				t.Fatalf("Symlink bundle dir-link: %v", err)
			}
			if directoryMatchesBundle(bundleRoot, map[string]string{"dir-link": "x"}) {
				t.Fatal("directoryMatchesBundle accepted unreadable symlinked directory entry")
			}
		}
	})

	t.Run("sync builtin skill surfaces unreadable installed document", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		skillDir := filepath.Join(registry.skillsPath, "broken-sync")
		if err := os.MkdirAll(filepath.Join(skillDir, "SKILL.md"), 0o755); err != nil {
			t.Fatalf("MkdirAll broken-sync/SKILL.md: %v", err)
		}
		if err := registry.syncBuiltinSkill("broken-sync", map[string]string{"SKILL.md": "---\nname: broken-sync\nmetadata:\n  source: builtin\n---\nFixed."}); err == nil {
			t.Fatal("syncBuiltinSkill accepted SKILL.md directory")
		}
	})
}
