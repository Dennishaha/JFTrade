package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestSkillInstallAdditionalBoundaryBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("installArchive and archive helpers surface tempdir and fs errors", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		blockedTmp := filepath.Join(t.TempDir(), "tmp-file")
		if err := os.WriteFile(blockedTmp, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile blocked TMPDIR: %v", err)
		}
		t.Setenv("TMPDIR", blockedTmp)
		if _, err := registry.installArchive(ctx, "https://example.com/tmp-fail.zip", zipArchive(t, map[string]string{
			"tmp-fail/SKILL.md": "---\nname: tmp-fail\n---\nBody.",
		})); err == nil {
			t.Fatal("installArchive accepted non-directory TMPDIR")
		}

		archive := zipArchiveWithDirectories(t, map[string]string{
			"nested/SKILL.md": "---\nname: nested\n---\nBody.",
		})
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		tempFile := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(tempFile, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile temp file: %v", err)
		}
		var extracted uint64
		if err := extractSkillArchiveFile(reader.File[0], tempFile, &extracted); err == nil {
			t.Fatal("extractSkillArchiveFile accepted temp root that is already a file")
		}

		reader.File[0].Method = 999
		if err := copySkillArchiveFile(reader.File[0], filepath.Join(t.TempDir(), "copy.out")); err == nil {
			t.Fatal("copySkillArchiveFile accepted empty zip file descriptor")
		}
	})

	t.Run("installArchive rejects a zip whose payload was corrupted after download", func(t *testing.T) {
		archive := zipArchiveWithDirectories(t, map[string]string{
			"corrupt-skill/SKILL.md": "---\nname: corrupt-skill\n---\nThis body must fail checksum validation.",
		})
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatalf("zip.NewReader: %v", err)
		}
		offset, err := reader.File[0].DataOffset()
		if err != nil {
			t.Fatalf("zip data offset: %v", err)
		}
		if offset < 0 || offset >= int64(len(archive)) {
			t.Fatalf("zip data offset = %d, archive len = %d", offset, len(archive))
		}
		corrupted := append([]byte(nil), archive...)
		corrupted[offset] ^= 0xff

		registry := &SkillRegistry{skillsPath: t.TempDir()}
		if _, err := registry.installArchive(ctx, "https://example.com/corrupt-skill.zip", corrupted); err == nil {
			t.Fatal("installArchive accepted a zip whose extracted payload fails integrity verification")
		}
		if _, err := os.Stat(filepath.Join(registry.skillsPath, "corrupt-skill")); !os.IsNotExist(err) {
			t.Fatalf("corrupt archive left an installed skill behind, stat err=%v", err)
		}
	})

	t.Run("rewriteArchiveSkillDocument and installed archive loading surface read write and lookup errors", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			readRoot := t.TempDir()
			if err := os.Symlink(filepath.Join(readRoot, "missing-skill"), filepath.Join(readRoot, "SKILL.md")); err != nil {
				t.Fatalf("Symlink unreadable SKILL.md: %v", err)
			}
			if _, _, err := rewriteArchiveSkillDocument(readRoot, "https://example.com/read-error"); err == nil {
				t.Fatal("rewriteArchiveSkillDocument accepted unreadable SKILL.md symlink")
			}

			writeRoot := t.TempDir()
			writeSkillDocument(t, writeRoot, "writable", "---\nname: writable\ndescription: Writable\n---\nBody.")
			docPath := filepath.Join(writeRoot, "writable", "SKILL.md")
			if err := os.Chmod(docPath, 0o444); err != nil {
				t.Fatalf("Chmod readonly SKILL.md: %v", err)
			}
			if _, _, err := rewriteArchiveSkillDocument(filepath.Join(writeRoot, "writable"), "https://example.com/write-error"); err == nil {
				t.Fatal("rewriteArchiveSkillDocument accepted readonly SKILL.md")
			}
		}

		registry := &SkillRegistry{skillsPath: t.TempDir()}
		if _, err := registry.loadInstalledArchiveSkill(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "installed skill not found") {
			t.Fatalf("loadInstalledArchiveSkill missing err = %v", err)
		}
		writeSkillDocument(t, registry.skillsPath, "broken", "---\nname: broken\nmetadata: [\n---\nBroken.")
		if _, err := registry.loadInstalledArchiveSkill(ctx, "broken"); err == nil {
			t.Fatal("loadInstalledArchiveSkill accepted malformed installed skill")
		}
	})

	t.Run("builtin cleanup and copy helpers surface additional filesystem failures", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		legacyDir := filepath.Join(registry.skillsPath, strategypinespec.LegacyBuiltinSkillName)
		if err := os.MkdirAll(filepath.Join(legacyDir, "SKILL.md"), 0o755); err != nil {
			t.Fatalf("MkdirAll legacy SKILL.md directory: %v", err)
		}
		if err := registry.removeLegacyBuiltinSkills(); err == nil {
			t.Fatal("removeLegacyBuiltinSkills accepted unreadable legacy SKILL.md")
		}
		if err := registry.ensureBuiltins(); err == nil {
			t.Fatal("ensureBuiltins accepted unreadable legacy SKILL.md")
		}

		root := t.TempDir()
		source := filepath.Join(root, "source")
		if err := os.MkdirAll(source, 0o755); err != nil {
			t.Fatalf("MkdirAll source: %v", err)
		}
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("skill"), 0o644); err != nil {
			t.Fatalf("WriteFile source SKILL.md: %v", err)
		}
		if err := copyDirectoryContents(source, filepath.Join(root, "missing-target")); err == nil {
			t.Fatal("copyDirectoryContents accepted missing target root for file copy")
		}

		if runtime.GOOS != "windows" {
			bundleRoot := t.TempDir()
			if err := os.Symlink(filepath.Join(bundleRoot, "missing"), filepath.Join(bundleRoot, "broken.md")); err != nil {
				t.Fatalf("Symlink broken bundle file: %v", err)
			}
			if directoryMatchesBundle(bundleRoot, map[string]string{"broken.md": "body"}) {
				t.Fatal("directoryMatchesBundle accepted broken symlink file entry")
			}
		}
	})

	t.Run("replaceDirectoryWithBundle surfaces tempdir stat and bundle conflict errors", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			noWriteRoot := filepath.Join(t.TempDir(), "no-write")
			if err := os.MkdirAll(noWriteRoot, 0o500); err != nil {
				t.Fatalf("MkdirAll no-write root: %v", err)
			}
			if err := replaceDirectoryWithBundle(filepath.Join(noWriteRoot, "skill"), map[string]string{"SKILL.md": "body"}); err == nil {
				t.Fatal("replaceDirectoryWithBundle accepted non-writable parent directory")
			}

			loopRoot := t.TempDir()
			loopPath := filepath.Join(loopRoot, "loop")
			if err := os.Symlink("loop", loopPath); err != nil {
				t.Fatalf("Symlink loop path: %v", err)
			}
			if err := replaceDirectoryWithBundle(loopPath, map[string]string{"SKILL.md": "body"}); err == nil {
				t.Fatal("replaceDirectoryWithBundle accepted symlink loop target")
			}
		}

		if err := replaceDirectoryWithBundle(filepath.Join(t.TempDir(), "conflict-skill"), map[string]string{
			"a":   "file",
			"a/b": "child",
		}); err == nil {
			t.Fatal("replaceDirectoryWithBundle accepted conflicting file and directory bundle paths")
		}
	})
}
