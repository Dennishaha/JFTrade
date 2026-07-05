package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestSkillRegistryListSortsBySourceAndDefaultsFilesystemMetadata(t *testing.T) {
	ctx := context.Background()
	registry := &SkillRegistry{skillsPath: t.TempDir()}

	writeSkillDocument(t, registry.skillsPath, "z-local", "---\nname: z-local\ndescription: Local filesystem skill\nallowed-tools: [local.tool]\n---\nUse the local skill.")
	writeSkillDocument(t, registry.skillsPath, "a-remote", "---\nname: a-remote\ndescription: Remote skill\nmetadata:\n  source: https://example.com/a-remote/SKILL.md\n  version: \"7\"\n---\nUse the remote skill.")
	writeSkillDocument(t, registry.skillsPath, "b-builtin", "---\nname: b-builtin\ndescription: Builtin skill\nmetadata:\n  source: builtin\n---\nUse the builtin skill.")

	skills, err := registry.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("skills len = %d, want 3: %+v", len(skills), skills)
	}
	if got := []string{skills[0].ID, skills[1].ID, skills[2].ID}; strings.Join(got, ",") != "b-builtin,z-local,a-remote" {
		t.Fatalf("sorted skill IDs = %v, want source then name order", got)
	}
	if !skills[0].Builtin || skills[0].Source != "builtin" {
		t.Fatalf("builtin skill metadata = %+v", skills[0])
	}
	if skills[1].Source != "filesystem" || skills[1].Builtin || len(skills[1].Tools) != 1 || skills[1].Tools[0] != "local.tool" {
		t.Fatalf("filesystem skill metadata = %+v", skills[1])
	}
	if skills[2].Version != "7" || skills[2].Source != "https://example.com/a-remote/SKILL.md" {
		t.Fatalf("remote skill metadata = %+v", skills[2])
	}
}

func TestSkillRegistryArchiveInstallsBundlesWithDirectoryEntries(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	archive := zipArchiveWithDirectories(t, map[string]string{
		".":                                 "",
		"dir-skill/":                        "",
		"dir-skill/references/":             "",
		"dir-skill/SKILL.md":                "---\nname: dir-skill\ndescription: Directory archive skill\n---\nUse explicit archive directories.",
		"dir-skill/references/checklist.md": "checklist",
	})

	skill, err := runtime.Skills().installArchive(ctx, "https://example.com/dir-skill.zip", archive)
	if err != nil {
		t.Fatalf("installArchive with directory entries: %v", err)
	}
	if skill.ID != "dir-skill" || skill.Source != "https://example.com/dir-skill.zip" {
		t.Fatalf("installed archive skill = %+v", skill)
	}
	raw, err := os.ReadFile(filepath.Join(runtime.Store().SkillsPath(), "dir-skill", "references", "checklist.md"))
	if err != nil || string(raw) != "checklist" {
		t.Fatalf("installed archive resource = %q err=%v", string(raw), err)
	}
}

func TestSkillRegistryFilesystemFailureBoundaries(t *testing.T) {
	ctx := context.Background()
	var nilRegistry *SkillRegistry
	if err := nilRegistry.Uninstall(ctx, "skill"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil Uninstall err = %v", err)
	}
	if err := nilRegistry.ensureBuiltins(); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil ensureBuiltins err = %v", err)
	}
	if err := nilRegistry.syncBuiltinSkill("skill", map[string]string{"SKILL.md": "body"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil syncBuiltinSkill err = %v", err)
	}

	root := t.TempDir()
	registry := &SkillRegistry{skillsPath: filepath.Join(root, "skills")}
	parentFile := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("file"), 0o644); err != nil {
		t.Fatalf("WriteFile parent: %v", err)
	}
	fileBackedRegistry := &SkillRegistry{skillsPath: parentFile}
	if _, _, err := fileBackedRegistry.installSkillDocument("blocked", []byte("body")); err == nil {
		t.Fatal("installSkillDocument should fail when skills path is a file")
	}
	if _, _, err := fileBackedRegistry.installSkillDirectory("blocked-dir", root); err == nil {
		t.Fatal("installSkillDirectory should fail when skills path is a file")
	}
	if err := replaceDirectoryWithBundle(filepath.Join(parentFile, "child"), map[string]string{"SKILL.md": "body"}); err == nil {
		t.Fatal("replaceDirectoryWithBundle should fail when parent path is a file")
	}

	if _, _, err := registry.installSkillDirectory("missing-source", filepath.Join(root, "missing")); err == nil {
		t.Fatal("installSkillDirectory should fail when source directory is missing")
	}
	if _, err := os.Stat(filepath.Join(registry.skillsPath, "missing-source")); !os.IsNotExist(err) {
		t.Fatalf("failed directory install should be cleaned up, stat err=%v", err)
	}
	if err := copyDirectoryContents(filepath.Join(root, "missing-copy"), filepath.Join(root, "target")); err == nil {
		t.Fatal("copyDirectoryContents should surface missing source errors")
	}
	if directoryMatchesBundle(filepath.Join(root, "missing-bundle"), map[string]string{}) {
		t.Fatal("directoryMatchesBundle should not match when root cannot be walked")
	}
	if _, err := locateSkillDocument(filepath.Join(root, "missing-archive-root")); err == nil {
		t.Fatal("locateSkillDocument should surface missing archive root errors")
	}
	if _, err := registry.skillFromFrontmatter(&adkskill.Frontmatter{Name: "missing"}); err == nil {
		t.Fatal("skillFromFrontmatter should surface missing installed document")
	}
}

func TestSkillRegistryAdditionalBoundaryBranches(t *testing.T) {
	ctx := context.Background()
	var nilRegistry *SkillRegistry
	if _, err := nilRegistry.List(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil List err = %v, want unavailable", err)
	}
	if _, ok, err := nilRegistry.Get(ctx, "skill"); err == nil || ok {
		t.Fatalf("nil Get ok=%v err=%v, want unavailable", ok, err)
	}
	if source, err := nilRegistry.Source(ctx, []string{"skill"}); err != nil || source != nil {
		t.Fatalf("nil Source source=%#v err=%v, want nil nil", source, err)
	}
	if _, err := (&SkillRegistry{}).Source(ctx, []string{"skill"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("empty registry Source err = %v, want unavailable", err)
	}

	base := &googleADKFakeSkillSource{
		frontmatters: []*adkskill.Frontmatter{{Name: "allowed"}, {Name: "denied"}},
		instructions: map[string]string{"allowed": "allowed instructions"},
		resources:    map[string]map[string]string{"allowed": {"guide.md": "guide"}},
	}
	filtered := &filteredSkillSource{base: base, allowed: map[string]struct{}{"allowed": {}}}
	frontmatters, err := filtered.ListFrontmatters(ctx)
	if err != nil || len(frontmatters) != 1 || frontmatters[0].Name != "allowed" {
		t.Fatalf("filtered ListFrontmatters = %#v err=%v", frontmatters, err)
	}
	if _, err := filtered.ListResources(ctx, "denied", ""); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("filtered denied ListResources err = %v", err)
	}
	reader, err := filtered.LoadResource(ctx, "allowed", "guide.md")
	if err != nil {
		t.Fatalf("filtered allowed LoadResource: %v", err)
	}
	raw, err := io.ReadAll(reader)
	jftradeCheckTestError(t, reader.Close())
	if err != nil || string(raw) != "guide" {
		t.Fatalf("filtered resource = %q err=%v", string(raw), err)
	}
	filteredErr := &filteredSkillSource{
		base:    &googleADKFakeSkillSource{frontmatterErr: errors.New("frontmatter list failed")},
		allowed: map[string]struct{}{"allowed": {}},
	}
	if _, err := filteredErr.ListFrontmatters(ctx); err == nil || !strings.Contains(err.Error(), "frontmatter list failed") {
		t.Fatalf("filtered ListFrontmatters error = %v", err)
	}
	set := sliceToSet([]string{"", "alpha", "alpha", "beta"})
	if _, ok := set[""]; ok || len(set) != 2 {
		t.Fatalf("sliceToSet = %#v, want alpha/beta only", set)
	}

	root := t.TempDir()
	registry := &SkillRegistry{skillsPath: root}
	dirSkill := filepath.Join(root, "dir-skill")
	if err := os.MkdirAll(filepath.Join(dirSkill, "SKILL.md"), 0o755); err != nil {
		t.Fatalf("MkdirAll dir SKILL.md: %v", err)
	}
	if _, err := registry.skillFromFrontmatter(&adkskill.Frontmatter{Name: "dir-skill"}); err == nil {
		t.Fatal("skillFromFrontmatter directory read err = nil, want error")
	}

	legacyDir := filepath.Join(root, strategypinespec.LegacyBuiltinSkillName)
	if err := os.MkdirAll(filepath.Join(legacyDir, "SKILL.md"), 0o755); err != nil {
		t.Fatalf("MkdirAll legacy SKILL.md: %v", err)
	}
	if err := registry.removeLegacyBuiltinSkill(strategypinespec.LegacyBuiltinSkillName); err == nil {
		t.Fatal("removeLegacyBuiltinSkill directory read err = nil, want error")
	}

	originalBuiltinSpecs := builtinSkillSpecs
	builtinSkillSpecs = []builtinSkillSpec{{Name: "broken-builtin", BuildBundle: func() (map[string]string, error) {
		return nil, errors.New("build bundle failed")
	}}}
	t.Cleanup(func() { builtinSkillSpecs = originalBuiltinSpecs })
	if err := (&SkillRegistry{skillsPath: t.TempDir()}).ensureBuiltins(); err == nil || !strings.Contains(err.Error(), "build bundle failed") {
		t.Fatalf("ensureBuiltins build error = %v", err)
	}
}

func TestSkillRegistryMalformedBuiltinSyncFailsWithoutReplacingExternalState(t *testing.T) {
	registry := &SkillRegistry{skillsPath: t.TempDir()}
	skillDir := filepath.Join(registry.skillsPath, "broken")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll broken skill: %v", err)
	}
	brokenDoc := []byte("---\nname: broken\nmetadata: [\n---\nBroken.")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), brokenDoc, 0o644); err != nil {
		t.Fatalf("WriteFile broken skill: %v", err)
	}

	if err := registry.syncBuiltinSkill("broken", map[string]string{"SKILL.md": "---\nname: broken\nmetadata:\n  source: builtin\n---\nFixed."}); err == nil {
		t.Fatal("syncBuiltinSkill should reject malformed existing frontmatter")
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || string(raw) != string(brokenDoc) {
		t.Fatalf("broken skill should remain untouched after failed sync: %q err=%v", string(raw), err)
	}
}

func writeSkillDocument(t *testing.T, skillsPath, name, content string) {
	t.Helper()
	dir := filepath.Join(skillsPath, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
}

func zipArchiveWithDirectories(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range entries {
		header := &zip.FileHeader{Name: name}
		if strings.HasSuffix(name, "/") {
			header.SetMode(os.ModeDir | 0o755)
		} else {
			header.SetMode(0o644)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("Create zip entry %s: %v", name, err)
		}
		if !strings.HasSuffix(name, "/") {
			if _, err := file.Write([]byte(content)); err != nil {
				t.Fatalf("Write zip entry %s: %v", name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	return archive.Bytes()
}
