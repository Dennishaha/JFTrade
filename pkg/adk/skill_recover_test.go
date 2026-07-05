package adk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillRegistrySourceAndFrontmatterFailureBoundaries(t *testing.T) {
	ctx := context.Background()
	if _, err := (*SkillRegistry)(nil).List(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil registry List err = %v", err)
	}
	if _, _, err := (*SkillRegistry)(nil).Get(ctx, "skill"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil registry Get err = %v", err)
	}
	if _, err := (*SkillRegistry)(nil).Source(ctx, []string{"skill"}); err != nil {
		t.Fatalf("nil Source should be a nil source without error, got %v", err)
	}

	registry := &SkillRegistry{skillsPath: t.TempDir()}
	badDir := filepath.Join(registry.skillsPath, "bad-skill")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("MkdirAll bad skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("---\nname: bad-skill\nmetadata: [\n---\nBad."), 0o644); err != nil {
		t.Fatalf("WriteFile bad skill: %v", err)
	}
	if _, err := registry.List(ctx); err == nil {
		t.Fatal("List should surface malformed skill frontmatter")
	}
	if _, _, err := registry.Get(ctx, "bad-skill"); err == nil {
		t.Fatal("Get should surface malformed skill frontmatter")
	}
}

func TestSkillRegistryBuiltinSyncAndLegacyCleanupBoundaries(t *testing.T) {
	registry := &SkillRegistry{skillsPath: t.TempDir()}

	legacyDir := filepath.Join(registry.skillsPath, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte("---\nname: legacy\ndescription: Legacy builtin\nmetadata:\n  source: builtin\n---\nOld builtin."), 0o644); err != nil {
		t.Fatalf("WriteFile legacy builtin: %v", err)
	}
	if err := registry.removeLegacyBuiltinSkill("legacy"); err != nil {
		t.Fatalf("removeLegacyBuiltinSkill builtin: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("builtin legacy dir still exists: %v", err)
	}

	externalDir := filepath.Join(registry.skillsPath, "external")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll external: %v", err)
	}
	externalDoc := []byte("---\nname: external\ndescription: External skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nExternal.")
	if err := os.WriteFile(filepath.Join(externalDir, "SKILL.md"), externalDoc, 0o644); err != nil {
		t.Fatalf("WriteFile external: %v", err)
	}
	if err := registry.removeLegacyBuiltinSkill("external"); err != nil {
		t.Fatalf("removeLegacyBuiltinSkill external: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(externalDir, "SKILL.md")); err != nil || string(raw) != string(externalDoc) {
		t.Fatalf("external legacy skill was modified: %q err=%v", string(raw), err)
	}
	if err := registry.syncBuiltinSkill("external", map[string]string{"SKILL.md": "---\nname: external\ndescription: Builtin skill\nmetadata:\n  source: builtin\n---\nBuiltin."}); err != nil {
		t.Fatalf("syncBuiltinSkill external: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(externalDir, "SKILL.md")); err != nil || string(raw) != string(externalDoc) {
		t.Fatalf("external skill should not be overwritten by builtin sync: %q err=%v", string(raw), err)
	}

	if err := registry.syncBuiltinSkill("fresh-builtin", map[string]string{
		"SKILL.md":            "---\nname: fresh-builtin\ndescription: Fresh builtin\nmetadata:\n  source: builtin\n---\nFresh.",
		"references/guide.md": "guide",
	}); err != nil {
		t.Fatalf("syncBuiltinSkill fresh: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(registry.skillsPath, "fresh-builtin", "references", "guide.md")); err != nil || string(raw) != "guide" {
		t.Fatalf("fresh builtin resource = %q err=%v", string(raw), err)
	}

	matchedBundle := map[string]string{
		"SKILL.md":            "---\nname: matched\ndescription: Matched builtin\nmetadata:\n  source: builtin\n---\nMatched.",
		"references/guide.md": "same guide",
	}
	if err := replaceDirectoryWithBundle(filepath.Join(registry.skillsPath, "matched"), matchedBundle); err != nil {
		t.Fatalf("replace matched builtin: %v", err)
	}
	before, err := os.Stat(filepath.Join(registry.skillsPath, "matched", "SKILL.md"))
	if err != nil {
		t.Fatalf("stat matched before: %v", err)
	}
	if err := registry.syncBuiltinSkill("matched", matchedBundle); err != nil {
		t.Fatalf("syncBuiltinSkill matched: %v", err)
	}
	after, err := os.Stat(filepath.Join(registry.skillsPath, "matched", "SKILL.md"))
	if err != nil {
		t.Fatalf("stat matched after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("matched builtin was rewritten: before=%v after=%v", before.ModTime(), after.ModTime())
	}

	outdatedDir := filepath.Join(registry.skillsPath, "outdated")
	if err := os.MkdirAll(filepath.Join(outdatedDir, "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll outdated: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outdatedDir, "SKILL.md"), []byte("---\nname: outdated\ndescription: Old builtin\nmetadata:\n  source: builtin\n---\nOld."), 0o644); err != nil {
		t.Fatalf("WriteFile outdated skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outdatedDir, "references", "stale.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile stale resource: %v", err)
	}
	newBundle := map[string]string{
		"SKILL.md":            "---\nname: outdated\ndescription: New builtin\nmetadata:\n  source: builtin\n---\nNew.",
		"references/guide.md": "fresh",
	}
	if err := registry.syncBuiltinSkill("outdated", newBundle); err != nil {
		t.Fatalf("syncBuiltinSkill outdated: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(outdatedDir, "SKILL.md")); err != nil || !strings.Contains(string(raw), "New builtin") {
		t.Fatalf("outdated skill after sync = %q err=%v", string(raw), err)
	}
	if _, err := os.Stat(filepath.Join(outdatedDir, "references", "stale.md")); !os.IsNotExist(err) {
		t.Fatalf("stale resource should be removed after builtin refresh: %v", err)
	}
}

func TestSkillRegistryCopyAndReplaceDirectoryBoundaries(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "guide.md"), []byte("guide"), 0o640); err != nil {
		t.Fatalf("WriteFile guide: %v", err)
	}
	if err := copyDirectoryContents(source, target); err != nil {
		t.Fatalf("copyDirectoryContents: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(target, "nested", "guide.md")); err != nil || string(raw) != "guide" {
		t.Fatalf("copied guide = %q err=%v", string(raw), err)
	}

	bundleTarget := filepath.Join(root, "bundle-skill")
	if err := replaceDirectoryWithBundle(bundleTarget, map[string]string{"SKILL.md": "old"}); err != nil {
		t.Fatalf("replaceDirectoryWithBundle initial: %v", err)
	}
	if err := replaceDirectoryWithBundle(bundleTarget, map[string]string{"SKILL.md": "new", "references/guide.md": "guide"}); err != nil {
		t.Fatalf("replaceDirectoryWithBundle replace: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(bundleTarget, "SKILL.md")); err != nil || string(raw) != "new" {
		t.Fatalf("replaced bundle skill = %q err=%v", string(raw), err)
	}
	if _, err := os.Stat(bundleTarget + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup dir should be removed after successful replace: %v", err)
	}

	if !directoryMatchesBundle(bundleTarget, map[string]string{"SKILL.md": "new", "references/guide.md": "guide"}) {
		t.Fatal("bundle directory should match expected contents")
	}
	if directoryMatchesBundle(bundleTarget, map[string]string{"SKILL.md": "new", "references/guide.md": "changed"}) {
		t.Fatal("content drift should not match expected bundle")
	}
	if directoryMatchesBundle(bundleTarget, map[string]string{"SKILL.md": "new"}) {
		t.Fatal("extra files should not match smaller expected bundle")
	}
}
