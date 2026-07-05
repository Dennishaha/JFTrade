package adk

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestSkillRegistryArchiveAndFilesystemBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := (&SkillRegistry{}).InstallURL(ctx, "ftp://example.com/skill.md"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable InstallURL err = %v, want unavailable", err)
	}
	dir := t.TempDir()
	registry := &SkillRegistry{skillsPath: dir}
	if _, err := registry.InstallURL(ctx, "ftp://example.com/skill.md"); err == nil || !strings.Contains(err.Error(), "valid http/https") {
		t.Fatalf("invalid InstallURL err = %v, want URL validation", err)
	}
	rawBundle, err := buildSingleFileBuiltinSkill("zip-skill", "Zip skill", "Use zip skill.", []string{"tool.one"}, "1")
	if err != nil {
		t.Fatalf("buildSingleFileBuiltinSkill: %v", err)
	}
	skill, err := registry.installArchive(ctx, "https://example.com/zip-skill.zip", zipSkillArchive(t, map[string]string{
		"bundle/SKILL.md": rawBundle["SKILL.md"],
		"bundle/data.txt": "resource",
	}))
	if err != nil {
		t.Fatalf("installArchive success: %v", err)
	}
	if skill.ID != "zip-skill" || skill.Source != "https://example.com/zip-skill.zip" {
		t.Fatalf("installed archive skill = %+v", skill)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/unsafe.zip", zipSkillArchive(t, map[string]string{"../SKILL.md": rawBundle["SKILL.md"]})); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe archive err = %v, want unsafe path", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/missing.zip", zipSkillArchive(t, map[string]string{"README.md": "missing"})); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("missing archive err = %v, want no SKILL.md", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/dupe.zip", zipSkillArchive(t, map[string]string{
		"a/SKILL.md": rawBundle["SKILL.md"],
		"b/SKILL.md": rawBundle["SKILL.md"],
	})); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("duplicate archive err = %v, want exactly one", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/bad.zip", []byte("not zip")); err == nil || !strings.Contains(err.Error(), "parse skill archive") {
		t.Fatalf("bad archive err = %v, want parse error", err)
	}

	source := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(t.TempDir(), "dst")
	if err := copyDirectoryContents(source, target); err != nil {
		t.Fatalf("copyDirectoryContents success: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(target, "nested", "file.txt")); err != nil || string(raw) != "content" {
		t.Fatalf("copied file raw=%q err=%v", raw, err)
	}
	blockDirTarget := filepath.Join(t.TempDir(), "block-dir")
	if err := os.MkdirAll(blockDirTarget, 0o755); err != nil {
		t.Fatalf("mkdir block target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockDirTarget, "nested"), []byte("file blocks dir"), 0o644); err != nil {
		t.Fatalf("write block file: %v", err)
	}
	if err := copyDirectoryContents(source, blockDirTarget); err == nil {
		t.Fatal("copyDirectoryContents with target file blocking directory err = nil, want error")
	}
	sourceFile := filepath.Join(t.TempDir(), "src-file")
	if err := os.WriteFile(filepath.Join(sourceFile), []byte("not dir"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := copyDirectoryContents(sourceFile, filepath.Join(t.TempDir(), "dst-file")); err != nil {
		t.Fatalf("copyDirectoryContents source root file should be ignored: %v", err)
	}

	bundle := map[string]string{"SKILL.md": rawBundle["SKILL.md"], "resources/info.txt": "info"}
	replaceTarget := filepath.Join(t.TempDir(), "replace-skill")
	if err := replaceDirectoryWithBundle(replaceTarget, bundle); err != nil {
		t.Fatalf("replaceDirectoryWithBundle new: %v", err)
	}
	if !directoryMatchesBundle(replaceTarget, bundle) {
		t.Fatal("new replaced directory should match bundle")
	}
	changed := map[string]string{"SKILL.md": rawBundle["SKILL.md"], "resources/info.txt": "changed"}
	if err := replaceDirectoryWithBundle(replaceTarget, changed); err != nil {
		t.Fatalf("replaceDirectoryWithBundle existing: %v", err)
	}
	if !directoryMatchesBundle(replaceTarget, changed) || directoryMatchesBundle(replaceTarget, bundle) {
		t.Fatal("existing replaced directory should match changed bundle only")
	}
	if err := replaceDirectoryWithBundle(filepath.Join(t.TempDir(), "unsafe-bundle"), map[string]string{"../bad": "x"}); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe bundle err = %v, want unsafe", err)
	}
	if directoryMatchesBundle(filepath.Join(t.TempDir(), "missing"), bundle) {
		t.Fatal("missing directory should not match bundle")
	}

	if _, _, err := registry.installSkillDocument("", []byte("x")); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("blank installSkillDocument err = %v, want required", err)
	}
	docPath, existed, err := registry.installSkillDocument("doc-skill", []byte(rawBundle["SKILL.md"]))
	if err != nil || existed || !strings.HasSuffix(docPath, "SKILL.md") {
		t.Fatalf("installSkillDocument success path=%q existed=%v err=%v", docPath, existed, err)
	}
	if _, existed, err := registry.installSkillDocument("doc-skill", []byte(rawBundle["SKILL.md"])); err == nil || !existed {
		t.Fatalf("duplicate installSkillDocument existed=%v err=%v, want existed error", existed, err)
	}
	invalidLegacyDir := filepath.Join(dir, strategypinespec.LegacyBuiltinSkillName)
	if err := os.MkdirAll(invalidLegacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidLegacyDir, "SKILL.md"), []byte("not frontmatter"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := registry.removeLegacyBuiltinSkill(strategypinespec.LegacyBuiltinSkillName); err != nil {
		t.Fatalf("invalid legacy removal should be ignored: %v", err)
	}
	if err := registry.syncBuiltinSkill("custom-preserved", map[string]string{"SKILL.md": rawBundle["SKILL.md"]}); err != nil {
		t.Fatalf("syncBuiltinSkill missing: %v", err)
	}
	customDir := filepath.Join(dir, "custom-preserved")
	customDoc := filepath.Join(customDir, "SKILL.md")
	customRaw := strings.Replace(rawBundle["SKILL.md"], "source: builtin", "source: user", 1)
	if err := os.WriteFile(customDoc, []byte(customRaw), 0o644); err != nil {
		t.Fatalf("write custom doc: %v", err)
	}
	if err := registry.syncBuiltinSkill("custom-preserved", map[string]string{"SKILL.md": rawBundle["SKILL.md"]}); err != nil {
		t.Fatalf("syncBuiltinSkill custom should be preserved: %v", err)
	}
	if raw, err := os.ReadFile(customDoc); err != nil || !strings.Contains(string(raw), "source: user") {
		t.Fatalf("custom doc raw=%q err=%v, want preserved user source", raw, err)
	}
}

func zipSkillArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
