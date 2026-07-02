package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adkskill "google.golang.org/adk/tool/skilltoolset/skill"
)

func TestSkillRegistryFilteredSourceExposesOnlyAllowedResources(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), "resource-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll resource skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: resource-skill
description: Resource backed skill
allowed-tools: [http.fetch]
---
Use the bundled guide before answering.`), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "guide.md"), []byte("guide content"), 0o644); err != nil {
		t.Fatalf("WriteFile resource: %v", err)
	}

	source, err := runtime.Skills().Source(ctx, []string{"resource-skill"})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	frontmatters, err := source.ListFrontmatters(ctx)
	if err != nil || len(frontmatters) != 1 || frontmatters[0].Name != "resource-skill" {
		t.Fatalf("ListFrontmatters = %#v, err=%v", frontmatters, err)
	}
	if _, err := source.LoadFrontmatter(ctx, "missing-skill"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("LoadFrontmatter missing err = %v", err)
	}
	instructions, err := source.LoadInstructions(ctx, "resource-skill")
	if err != nil || !strings.Contains(instructions, "bundled guide") {
		t.Fatalf("LoadInstructions = %q, err=%v", instructions, err)
	}
	if _, err := source.LoadInstructions(ctx, "missing-skill"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("LoadInstructions missing err = %v", err)
	}
	resources, err := source.ListResources(ctx, "resource-skill", "references")
	if err != nil || len(resources) == 0 {
		t.Fatalf("ListResources = %#v, err=%v", resources, err)
	}
	reader, err := source.LoadResource(ctx, "resource-skill", "references/guide.md")
	if err != nil {
		t.Fatalf("LoadResource: %v", err)
	}
	raw, err := io.ReadAll(reader)
	jftradeCheckTestError(t, reader.Close())
	if err != nil || string(raw) != "guide content" {
		t.Fatalf("LoadResource content = %q, err=%v", string(raw), err)
	}
	if _, err := source.LoadResource(ctx, "missing-skill", "references/guide.md"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("LoadResource missing err = %v", err)
	}

	emptySource, err := runtime.Skills().Source(ctx, nil)
	if err != nil || emptySource != nil {
		t.Fatalf("empty Source = %#v, err=%v", emptySource, err)
	}
	if _, err := runtime.Skills().Source(ctx, []string{"missing-skill"}); err == nil || !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("missing Source err = %v", err)
	}
}

func TestSkillRegistryArchiveRejectsUnsafeOrAmbiguousBundles(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	_, err := runtime.Skills().installArchive(ctx, "https://example.com/unsafe.zip", zipArchive(t, map[string]string{
		"../SKILL.md": "---\nname: unsafe\n---\nNope.",
	}))
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe archive err = %v", err)
	}

	_, err = runtime.Skills().installArchive(ctx, "https://example.com/missing.zip", zipArchive(t, map[string]string{
		"pack/references/guide.md": "guide",
	}))
	if err == nil || !strings.Contains(err.Error(), "does not contain SKILL.md") {
		t.Fatalf("missing skill archive err = %v", err)
	}

	_, err = runtime.Skills().installArchive(ctx, "https://example.com/ambiguous.zip", zipArchive(t, map[string]string{
		"one/SKILL.md": "---\nname: one\n---\nOne.",
		"two/SKILL.md": "---\nname: two\n---\nTwo.",
	}))
	if err == nil || !strings.Contains(err.Error(), "exactly one SKILL.md") {
		t.Fatalf("ambiguous archive err = %v", err)
	}

	_, err = runtime.Skills().installArchive(ctx, "https://example.com/bad.zip", []byte("not a zip"))
	if err == nil || !strings.Contains(err.Error(), "parse skill archive") {
		t.Fatalf("bad archive err = %v", err)
	}

	_, err = runtime.Skills().installArchive(ctx, "https://example.com/huge.zip", zipArchive(t, map[string]string{
		"huge/SKILL.md": "---\nname: huge\n---\nHuge.",
		"huge/blob.bin": strings.Repeat("x", maxSkillArchiveSize+1),
	}))
	if err == nil || !strings.Contains(err.Error(), "after extraction") {
		t.Fatalf("huge archive err = %v", err)
	}

	_, err = runtime.Skills().installArchive(ctx, "https://example.com/huge-skill.zip", zipArchive(t, map[string]string{
		"huge-skill/SKILL.md": "---\nname: huge-skill\n---\n" + strings.Repeat("x", maxSkillFileSize+1),
	}))
	if err == nil || !strings.Contains(err.Error(), "skill file exceeds") {
		t.Fatalf("huge skill document archive err = %v", err)
	}
}

func TestSkillRegistryInstallURLAndDirectoryBoundaries(t *testing.T) {
	ctx := context.Background()
	if _, err := (*SkillRegistry)(nil).InstallURL(ctx, "https://example.com/skill.md"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil registry InstallURL err = %v", err)
	}
	runtime := newTestRuntime(t)
	if _, err := runtime.Skills().InstallURL(ctx, "http://%zz"); err == nil || !strings.Contains(err.Error(), "valid http/https") {
		t.Fatalf("malformed URL err = %v", err)
	}
	if _, err := runtime.Skills().InstallURL(ctx, "file:///tmp/SKILL.md"); err == nil || !strings.Contains(err.Error(), "valid http/https") {
		t.Fatalf("invalid URL err = %v", err)
	}

	originalValidator := skillInstallHostValidator
	skillInstallHostValidator = func(_ context.Context, host string) error {
		if host == "blocked.example" {
			return errors.New("blocked initial host")
		}
		return nil
	}
	t.Cleanup(func() { skillInstallHostValidator = originalValidator })
	if _, err := runtime.Skills().InstallURL(ctx, "https://blocked.example/SKILL.md"); err == nil || !strings.Contains(err.Error(), "blocked initial host") {
		t.Fatalf("blocked host err = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", http.StatusFound)
			return
		}
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	if _, err := runtime.Skills().InstallURL(ctx, server.URL+"/missing/SKILL.md"); err == nil || !strings.Contains(err.Error(), "returned 404") {
		t.Fatalf("404 InstallURL err = %v", err)
	}
	if _, err := runtime.Skills().InstallURL(ctx, server.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("redirect loop err = %v", err)
	}

	root := t.TempDir()
	registry := &SkillRegistry{skillsPath: filepath.Join(root, "skills")}
	sourceDir := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceDir, "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: copied-skill\n---\nCopy me."), 0o644); err != nil {
		t.Fatalf("WriteFile source skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "references", "guide.md"), []byte("copied guide"), 0o644); err != nil {
		t.Fatalf("WriteFile source resource: %v", err)
	}
	installPath, existed, err := registry.installSkillDirectory("copied-skill", sourceDir)
	if err != nil || existed || !strings.HasSuffix(installPath, filepath.Join("copied-skill", "SKILL.md")) {
		t.Fatalf("installSkillDirectory path=%q existed=%v err=%v", installPath, existed, err)
	}
	copied, err := os.ReadFile(filepath.Join(registry.skillsPath, "copied-skill", "references", "guide.md"))
	if err != nil || string(copied) != "copied guide" {
		t.Fatalf("copied resource = %q, err=%v", string(copied), err)
	}
	if _, existed, err := registry.installSkillDirectory("copied-skill", sourceDir); err == nil || !existed {
		t.Fatalf("duplicate installSkillDirectory existed=%v err=%v", existed, err)
	}
	if _, _, err := registry.installSkillDirectory("   ", sourceDir); err == nil || !strings.Contains(err.Error(), "skill name is required") {
		t.Fatalf("blank installSkillDirectory err = %v", err)
	}
	if _, _, err := registry.installSkillDirectory("empty-source", "   "); err == nil || !strings.Contains(err.Error(), "source directory is required") {
		t.Fatalf("blank source installSkillDirectory err = %v", err)
	}
	if _, _, err := registry.installSkillDocument("", []byte("bad")); err == nil || !strings.Contains(err.Error(), "skill name is required") {
		t.Fatalf("blank installSkillDocument err = %v", err)
	}
}

func TestSkillRegistryInstallURLPlainDocumentAndRedirectSafety(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	originalValidator := skillInstallHostValidator
	skillInstallHostValidator = func(_ context.Context, host string) error {
		if host == "blocked.example" {
			return errors.New("blocked host")
		}
		return nil
	}
	t.Cleanup(func() { skillInstallHostValidator = originalValidator })

	plainDoc := []byte(`---
name: plain-skill
description: Plain Skill
allowed-tools: [http.fetch]
---
Use the plain downloaded skill.`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plain.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, err := w.Write(plainDoc)
			jftradeCheckTestError(t, err)
		case "/redirect":
			http.Redirect(w, r, "http://blocked.example/skill.md", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	skill, err := runtime.Skills().InstallURL(ctx, server.URL+"/plain.md")
	if err != nil {
		t.Fatalf("InstallURL plain markdown: %v", err)
	}
	if skill.ID != "plain-skill" || skill.Source != server.URL+"/plain.md" || len(skill.Tools) != 1 || skill.Tools[0] != "http.fetch" {
		t.Fatalf("plain installed skill = %+v", skill)
	}
	raw, err := os.ReadFile(filepath.Join(runtime.Store().SkillsPath(), "plain-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(raw), "source: "+server.URL+"/plain.md") {
		t.Fatalf("plain installed document = %q err=%v", string(raw), err)
	}
	if _, err := runtime.Skills().InstallURL(ctx, server.URL+"/plain.md"); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("duplicate plain InstallURL err = %v", err)
	}
	if _, err := runtime.Skills().InstallURL(ctx, server.URL+"/redirect"); err == nil || !strings.Contains(err.Error(), "redirect to unsafe host") {
		t.Fatalf("redirect InstallURL err = %v", err)
	}
}

func TestSkillRegistryInstallURLSupportsArchivesAndUninstallProtections(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	originalValidator := skillInstallHostValidator
	skillInstallHostValidator = func(context.Context, string) error { return nil }
	t.Cleanup(func() { skillInstallHostValidator = originalValidator })

	archive := zipArchive(t, map[string]string{
		"archive-skill/SKILL.md":                "---\nname: archive-skill\ndescription: Archive Skill\nallowed-tools: [http.fetch]\n---\nUse the bundled archive instructions.",
		"archive-skill/references/checklist.md": "archive checklist",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/archive.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, jftradeErr1 := w.Write(archive)
			jftradeCheckTestError(t, jftradeErr1)
		case "/too-large.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, jftradeErr2 := w.Write(bytes.Repeat([]byte("x"), maxSkillFileSize+1))
			jftradeCheckTestError(t, jftradeErr2)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	skill, err := runtime.Skills().InstallURL(ctx, server.URL+"/archive.zip")
	if err != nil {
		t.Fatalf("InstallURL archive: %v", err)
	}
	if skill.ID != "archive-skill" || skill.Source != server.URL+"/archive.zip" {
		t.Fatalf("archive skill = %+v", skill)
	}
	raw, err := os.ReadFile(filepath.Join(runtime.Store().SkillsPath(), "archive-skill", "references", "checklist.md"))
	if err != nil || string(raw) != "archive checklist" {
		t.Fatalf("archive resource = %q err=%v", string(raw), err)
	}

	if _, err := runtime.Skills().InstallURL(ctx, server.URL+"/too-large.md"); err == nil || !strings.Contains(err.Error(), "skill file exceeds") {
		t.Fatalf("InstallURL too-large err = %v", err)
	}
	if err := runtime.Skills().Uninstall(ctx, "missing-skill"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Uninstall missing err = %v", err)
	}
	if err := runtime.Skills().Uninstall(ctx, "jftrade-market"); err == nil || !strings.Contains(err.Error(), "builtin skills cannot be uninstalled") {
		t.Fatalf("Uninstall builtin err = %v", err)
	}
	if err := runtime.Skills().Uninstall(ctx, "archive-skill"); err != nil {
		t.Fatalf("Uninstall archive skill: %v", err)
	}
	if _, ok, err := runtime.Skills().Get(ctx, "archive-skill"); err != nil || ok {
		t.Fatalf("archive skill after uninstall ok=%v err=%v", ok, err)
	}
}

func TestSkillRegistryFileHelpersDetectArchiveAndBundleBoundaries(t *testing.T) {
	if !isZipSkillArchive("https://example.com/Skill.ZIP", "text/plain", nil) {
		t.Fatal("zip extension should be treated as archive")
	}
	if !isZipSkillArchive("https://example.com/skill", "application/zip", nil) {
		t.Fatal("zip content type should be treated as archive")
	}
	if !isZipSkillArchive("https://example.com/skill", "text/plain", []byte{'P', 'K', 0x03, 0x04, 'x'}) {
		t.Fatal("zip magic should be treated as archive")
	}
	if isZipSkillArchive("https://example.com/skill", "text/plain", []byte("plain")) {
		t.Fatal("plain file should not be treated as archive")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "__MACOSX"), 0o755); err != nil {
		t.Fatalf("MkdirAll __MACOSX: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "__MACOSX", "SKILL.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("WriteFile ignored skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pack"), 0o755); err != nil {
		t.Fatalf("MkdirAll pack: %v", err)
	}
	wantDoc := filepath.Join(root, "pack", "SKILL.md")
	if err := os.WriteFile(wantDoc, []byte("real"), 0o644); err != nil {
		t.Fatalf("WriteFile real skill: %v", err)
	}
	if got, err := locateSkillDocument(root); err != nil || got != wantDoc {
		t.Fatalf("locateSkillDocument = %q, err=%v", got, err)
	}

	bundleDir := t.TempDir()
	if !directoryMatchesBundle(bundleDir, map[string]string{}) {
		t.Fatal("empty directory should match empty bundle")
	}
	if directoryMatchesBundle(bundleDir, map[string]string{"SKILL.md": "expected"}) {
		t.Fatal("empty directory should not match non-empty bundle")
	}
	if err := replaceDirectoryWithBundle(filepath.Join(t.TempDir(), "skill"), map[string]string{"../bad": "x"}); err == nil || !strings.Contains(err.Error(), "unsafe builtin skill bundle path") {
		t.Fatalf("unsafe replaceDirectoryWithBundle err = %v", err)
	}
}

func zipArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create zip entry %s: %v", name, err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("Write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	return archive.Bytes()
}
