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
	assertNoLegacyRuntimeInCurrentMaintenanceDocs(t, root)
	assertNoLegacyRuntimeInFrontendSurfaces(t, root)
	assertLegacyRuntimeIDOnlyInMigrationShims(t, root)
	assertLegacyRuntimePackageRemoved(t, root)
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

func assertNoLegacyRuntimeInCurrentMaintenanceDocs(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"docs/architecture.md",
		"docs/troubleshooting/backtest-performance.md",
		"docs/troubleshooting/pinets-worker-release.md",
		"docs/pine-completion-roadmap.md",
		"docs/frontend/strategy-authoring.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", rel, err)
		}
		text := string(data)
		for _, legacy := range []string{
			"pine-go-plan",
			"pkg/strategy/pineruntime",
			"pkg/backtest.Run`",
			"pkg/backtest.Run ",
		} {
			if strings.Contains(text, legacy) {
				t.Fatalf("%s still references legacy runtime surface %q", rel, legacy)
			}
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

func assertLegacyRuntimeIDOnlyInMigrationShims(t *testing.T, root string) {
	t.Helper()
	allowed := []string{
		"apps/web/src/components/strategy-runtime/strategyRuntimeIdentity.ts",
		"docs/pinets-hardcut-migration.md",
		"docs/release-pine-v08-closeout.md",
		"pkg/strategy/pineworker/hardcut_audit_test.go",
		"pkg/strategy/pineworker/types.go",
		"pkg/strategy/pineworker/types_test.go",
	}
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", "release_assets", "var":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".md", ".ts", ".vue":
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
		t.Fatalf("walk repository: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("legacy runtime ID appears outside migration shims: %s", strings.Join(offenders, ", "))
	}
}

func assertLegacyRuntimePackageRemoved(t *testing.T, root string) {
	t.Helper()
	rel := "pkg/strategy/pineruntime"
	if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
		t.Fatalf("legacy Go Pine runtime package still exists: %s", rel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", rel, err)
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
			case ".git", "node_modules", "dist", "release_assets", "var":
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
