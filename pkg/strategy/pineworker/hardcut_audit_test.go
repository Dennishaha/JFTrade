package pineworker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPineTSHardCutDoesNotExposeGoPineRuntime(t *testing.T) {
	root := pineWorkerRepoRoot(t)
	assertNoLegacyRuntimeInCurrentSpecDocs(t, root)
	assertNoLegacyRuntimeInFrontendSurfaces(t, root)
	assertNoUnexpectedPineRuntimeImports(t, root)
}

func pineWorkerRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root with go.mod not found")
		}
		dir = parent
	}
}

func assertNoLegacyRuntimeInCurrentSpecDocs(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"pkg/strategy/pinespec/spec.go",
		"docs/reference/generated/pine-v6-support.md",
		"docs/frontend/strategy-authoring.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", rel, err)
		}
		text := string(data)
		if strings.Contains(text, "pine-go-plan") || strings.Contains(text, "pkg/strategy/pineruntime") {
			t.Fatalf("%s still exposes legacy Go Pine runtime", rel)
		}
	}
}

func assertNoLegacyRuntimeInFrontendSurfaces(t *testing.T, root string) {
	t.Helper()
	allowed := []string{
		"apps/web/src/components/strategy-runtime/strategyRuntimeIdentity.ts",
	}
	var offenders []string
	for _, dir := range []string{"apps/web/src", "apps/web/tests"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "node_modules", "dist":
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".ts", ".vue":
			default:
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if slices.Contains(allowed, rel) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), "pine-go-plan") {
				offenders = append(offenders, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("frontend surfaces still expose legacy Go Pine runtime: %s", strings.Join(offenders, ", "))
	}
}

func assertNoUnexpectedPineRuntimeImports(t *testing.T, root string) {
	t.Helper()
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "release_assets":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "pkg/strategy/pineruntime/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), `"github.com/jftrade/jftrade-main/pkg/strategy/pineruntime"`) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("unexpected Go Pine runtime imports outside hard-cut allowlist: %s", strings.Join(offenders, ", "))
	}
}
