package adk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
)

func TestSkillRegistryHTTPAndSourceAdditionalBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("unavailable registry methods return stable errors", func(t *testing.T) {
		if skills, err := (*SkillRegistry)(nil).List(ctx); err == nil || skills != nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("nil List skills=%+v err=%v, want unavailable error", skills, err)
		}
		if skill, ok, err := (*SkillRegistry)(nil).Get(ctx, "skill"); err == nil || ok || skill.ID != "" || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("nil Get skill=%+v ok=%v err=%v, want unavailable error", skill, ok, err)
		}
		if source, err := (*SkillRegistry)(nil).Source(ctx, []string{"skill"}); err != nil || source != nil {
			t.Fatalf("nil Source source=%+v err=%v, want nil source and nil error", source, err)
		}
		empty := &SkillRegistry{}
		if skills, err := empty.List(ctx); err == nil || skills != nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("empty List skills=%+v err=%v, want unavailable error", skills, err)
		}
		if skill, ok, err := empty.Get(ctx, "skill"); err == nil || ok || skill.ID != "" || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("empty Get skill=%+v ok=%v err=%v, want unavailable error", skill, ok, err)
		}
		if source, err := empty.Source(ctx, []string{"skill"}); err == nil || source != nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("empty Source source=%+v err=%v, want unavailable error", source, err)
		}
		if _, err := empty.InstallURL(ctx, "https://example.com/SKILL.md"); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("empty InstallURL err=%v, want unavailable error", err)
		}
	})

	t.Run("List Get and Source surface malformed frontmatter and install path errors", func(t *testing.T) {
		malformedRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, malformedRegistry.skillsPath, "malformed-list", "---\nname: malformed-list\nmetadata: [\n---\nBroken.")
		if _, err := malformedRegistry.List(ctx); err == nil {
			t.Fatal("List accepted malformed frontmatter")
		}
		if _, ok, err := malformedRegistry.Get(ctx, "malformed-list"); err == nil || ok {
			t.Fatalf("Get malformed ok=%v err=%v, want error", ok, err)
		}
		if _, err := malformedRegistry.Source(ctx, []string{"malformed-list"}); err == nil {
			t.Fatal("Source accepted malformed selected skill")
		}

		pathRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, pathRegistry.skillsPath, "carrier", "---\nname: path-target\ndescription: Carrier\n---\nBody.")
		if err := os.MkdirAll(filepath.Join(pathRegistry.skillsPath, "path-target", "SKILL.md"), 0o755); err != nil {
			t.Fatalf("MkdirAll path-target/SKILL.md: %v", err)
		}
		if _, err := pathRegistry.List(ctx); err == nil {
			t.Fatal("List accepted skill install path where SKILL.md is a directory")
		}
		if _, ok, err := pathRegistry.Get(ctx, "path-target"); err == nil || ok {
			t.Fatalf("Get path-target ok=%v err=%v, want install path error", ok, err)
		}
	})

	t.Run("source filters missing and malformed skills", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, registry.skillsPath, "valid-edge", "---\nname: valid-edge\ndescription: Valid\n---\nBody.")
		if source, err := registry.Source(ctx, nil); err != nil || source != nil {
			t.Fatalf("Source(nil) = %#v, %v; want nil, nil", source, err)
		}
		if _, err := registry.Source(ctx, []string{"missing-edge"}); err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("Source missing err = %v, want skill not found", err)
		}

		malformedRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, malformedRegistry.skillsPath, "malformed-edge", "---\nname: malformed-edge\nmetadata: [\n---\nBroken.")
		if _, _, err := malformedRegistry.Get(ctx, "malformed-edge"); err == nil {
			t.Fatal("Get accepted malformed skill frontmatter")
		}
		if _, err := malformedRegistry.Source(ctx, []string{"malformed-edge"}); err == nil {
			t.Fatal("Source accepted malformed selected skill")
		}

		source, err := registry.Source(ctx, []string{"valid-edge"})
		if err != nil {
			t.Fatalf("Source valid: %v", err)
		}
		if _, err := source.LoadFrontmatter(ctx, "other-edge"); !errors.Is(err, adkskill.ErrSkillNotFound) {
			t.Fatalf("filtered LoadFrontmatter err = %v, want ErrSkillNotFound", err)
		}
		if _, err := source.ListResources(ctx, "other-edge", ""); !errors.Is(err, adkskill.ErrSkillNotFound) {
			t.Fatalf("filtered ListResources err = %v, want ErrSkillNotFound", err)
		}
	})

	t.Run("InstallURL validates hosts statuses payload size and installed content", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		if _, err := registry.InstallURL(ctx, "ftp://example.com/SKILL.md"); err == nil || !strings.Contains(err.Error(), "valid http/https") {
			t.Fatalf("InstallURL invalid scheme err = %v", err)
		}

		oldValidator := skillInstallHostValidator
		skillInstallHostValidator = func(context.Context, string) error { return errors.New("blocked host") }
		if _, err := registry.InstallURL(ctx, "https://example.com/SKILL.md"); err == nil || !strings.Contains(err.Error(), "blocked host") {
			t.Fatalf("InstallURL host validator err = %v, want blocked host", err)
		}
		skillInstallHostValidator = oldValidator
		t.Cleanup(func() { skillInstallHostValidator = oldValidator })
		skillInstallHostValidator = func(context.Context, string) error { return nil }

		statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no skill", http.StatusTeapot)
		}))
		defer statusServer.Close()
		if _, err := registry.InstallURL(ctx, statusServer.URL+"/SKILL.md"); err == nil || !strings.Contains(err.Error(), "418") {
			t.Fatalf("InstallURL status err = %v, want 418", err)
		}

		largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", maxSkillFileSize+1)))
		}))
		defer largeServer.Close()
		if _, err := registry.InstallURL(ctx, largeServer.URL+"/SKILL.md"); err == nil || !strings.Contains(err.Error(), "skill file exceeds") {
			t.Fatalf("InstallURL oversized skill err = %v, want size error", err)
		}

		validServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("---\nname: installed-edge\ndescription: Installed\n---\nUse carefully."))
		}))
		defer validServer.Close()
		skill, err := registry.InstallURL(ctx, validServer.URL+"/SKILL.md")
		if err != nil {
			t.Fatalf("InstallURL valid: %v", err)
		}
		if skill.ID != "installed-edge" || skill.Source != validServer.URL+"/SKILL.md" {
			t.Fatalf("installed skill = %+v, want installed-edge with source URL", skill)
		}

		truncatedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			connection, buffered, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack truncated skill response: %v", err)
				return
			}
			defer func() { jftradeLogError(connection.Close()) }()
			_, _ = fmt.Fprint(buffered, "HTTP/1.1 200 OK\r\nContent-Length: 256\r\nConnection: close\r\n\r\n---\nname: partial")
			jftradeLogError(buffered.Flush())
		}))
		defer truncatedServer.Close()
		if _, err := registry.InstallURL(ctx, truncatedServer.URL+"/SKILL.md"); err == nil {
			t.Fatal("InstallURL accepted an interrupted skill download")
		}
	})

	t.Run("InstallURL surfaces archive size parse build and post-install lookup errors", func(t *testing.T) {
		oldValidator := skillInstallHostValidator
		skillInstallHostValidator = func(context.Context, string) error { return nil }
		t.Cleanup(func() { skillInstallHostValidator = oldValidator })

		parseRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		parseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not a skill document"))
		}))
		defer parseServer.Close()
		if _, err := parseRegistry.InstallURL(ctx, parseServer.URL+"/SKILL.md"); err == nil {
			t.Fatal("InstallURL accepted invalid skill document")
		}

		buildRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		buildServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("---\ndescription: missing name\n---\nBody."))
		}))
		defer buildServer.Close()
		if _, err := buildRegistry.InstallURL(ctx, buildServer.URL+"/SKILL.md"); err == nil {
			t.Fatal("InstallURL accepted skill document without a name")
		}

		postInstallLookupRegistry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, postInstallLookupRegistry.skillsPath, "malformed-installed", "---\nname: malformed-installed\nmetadata: [\n---\nBroken.")
		validServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("---\nname: post-install-lookup\ndescription: Valid\n---\nBody."))
		}))
		defer validServer.Close()
		if _, err := postInstallLookupRegistry.InstallURL(ctx, validServer.URL+"/SKILL.md"); err == nil {
			t.Fatal("InstallURL accepted registry whose post-install lookup should fail")
		}
	})
}

func TestSkillInstallAdditionalDeterministicErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("archive directory install reports existing installed skill", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, registry.skillsPath, "collision-skill", "---\nname: collision-skill\ndescription: Already installed\n---\nAlready installed.")

		extracted := t.TempDir()
		writeSkillDocument(t, extracted, "collision-skill", "---\nname: collision-skill\ndescription: Incoming skill\n---\nIncoming.")
		if _, err := registry.installExtractedArchiveSkill(ctx, "https://example.com/collision.zip", extracted); err == nil || !strings.Contains(err.Error(), "already installed") {
			t.Fatalf("installExtractedArchiveSkill collision err = %v, want already installed", err)
		}
	})

	t.Run("uninstall surfaces frontmatter preload errors", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, registry.skillsPath, "bad-uninstall", "---\nname: bad-uninstall\nmetadata: [\n---\nBroken.")
		if err := registry.Uninstall(ctx, "bad-uninstall"); err == nil {
			t.Fatal("Uninstall accepted malformed skill registry")
		}
	})

	t.Run("builtin bundle builders reject invalid generated frontmatter", func(t *testing.T) {
		if _, err := buildSingleFileBuiltinSkill("", "description", "instructions", nil, "1"); err == nil {
			t.Fatal("buildSingleFileBuiltinSkill accepted empty skill name")
		}
	})

	t.Run("document and directory installers reject empty and conflicting inputs", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		if _, _, err := registry.installSkillDocument(" ", []byte("body")); err == nil || !strings.Contains(err.Error(), "skill name is required") {
			t.Fatalf("installSkillDocument blank name err = %v", err)
		}
		if _, _, err := registry.installSkillDirectory("skill", " "); err == nil || !strings.Contains(err.Error(), "source directory is required") {
			t.Fatalf("installSkillDirectory blank source err = %v", err)
		}
		writeSkillDocument(t, registry.skillsPath, "already-dir", "---\nname: already-dir\n---\nBody.")
		if _, _, err := registry.installSkillDirectory("already-dir", t.TempDir()); err == nil || !strings.Contains(err.Error(), "already installed") {
			t.Fatalf("installSkillDirectory existing err = %v, want already installed", err)
		}

		fileRoot := filepath.Join(t.TempDir(), "skills-file")
		if err := os.WriteFile(fileRoot, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("WriteFile skills-file: %v", err)
		}
		fileRegistry := &SkillRegistry{skillsPath: fileRoot}
		if _, _, err := fileRegistry.installSkillDirectory("child", t.TempDir()); err == nil {
			t.Fatal("installSkillDirectory accepted skillsPath that is a file")
		}
	})
}
